import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import {
  type AssistProposalAdoptionDraft,
  type AssistProposalRejectionDraft,
  adoptAssistProposal,
  fetchAssistProposals,
  rejectAssistProposal,
} from "@/shared/api/assistProposals";
import type { ApiResult } from "@/shared/api/httpClient";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import {
  type AssistProposalAdoption,
  type AssistProposalItem,
  decodeAssistProposalItem,
} from "@/shared/contracts/assistProposals";
import { DecodeFailure } from "@/shared/contracts/decoding";
import { assertionTargetKey } from "@/shared/lib/assertionTargetKey";
import type { FetchFailure, FetchState } from "@/shared/lib/fetchState";

const listFailureSummary = "AI 提案の取得";
const decisionFailureSummary = "AI 提案の採否の記録";

/** 読み込んだ AI 提案と、対象で探せる索引。 */
export type LoadedAssistProposals = {
  items: AssistProposalItem[];
  /** 対象を指す文字列から、その対象の提案を探す。採否を決めた提案も含む。 */
  byTarget: ReadonlyMap<string, AssistProposalItem[]>;
};

/**
 * 提案 1 件の採否の下書き。分析者が直した記述、却下の理由、分析者の名前を持つ。
 * `note` が無い下書きは、記述を直していない。
 */
export type AssistProposalDecisionDraft = {
  note?: string;
  reason: string;
  analyst: string;
};

/** 下書きを持たない提案の下書き。 */
export const emptyDecisionDraft: AssistProposalDecisionDraft = {
  reason: "",
  analyst: "",
};

/** AI 提案の一覧と、採否の操作と状態を持つ。 */
export type AssistProposalsView = {
  state: FetchState<LoadedAssistProposals>;
  /** 一覧を取り直す。 */
  reload: () => void;
  /** 提案の識別子から採否の下書きを探す。**一覧を取り直しても消さない。** */
  drafts: ReadonlyMap<string, AssistProposalDecisionDraft>;
  changeDraft: (proposalId: string, draft: AssistProposalDecisionDraft) => void;
  /** 採否を送っている途中の提案の識別子。 */
  deciding: ReadonlySet<string>;
  /** 提案の識別子から、採否の最後の失敗を探す。**読めた一覧を消さない。** */
  failures: ReadonlyMap<string, FetchFailure>;
  adopt: (proposalId: string, draft: AssistProposalAdoptionDraft) => void;
  reject: (proposalId: string, draft: AssistProposalRejectionDraft) => void;
};

function indexByTarget(
  items: AssistProposalItem[],
): ReadonlyMap<string, AssistProposalItem[]> {
  const index = new Map<string, AssistProposalItem[]>();
  for (const item of items) {
    const key = assertionTargetKey(item.proposal.target);
    if (key === undefined) {
      continue;
    }
    index.set(key, [...(index.get(key) ?? []), item]);
  }
  return index;
}

/** 一覧の中の同じ識別子の提案を置き換える。 */
function replaced(
  items: AssistProposalItem[],
  item: AssistProposalItem,
): AssistProposalItem[] {
  return items.map((current) =>
    current.proposal.id === item.proposal.id ? item : current,
  );
}

/** 採否の競合の応答が持つ、現在の提案を読む。読めなければ `undefined` を返す。 */
function conflictedProposal(
  failure: FetchFailure,
): AssistProposalItem | undefined {
  if (failure.failureCode !== "assist_proposal_decided") {
    return undefined;
  }
  try {
    return decodeAssistProposalItem(failure.conflict, "conflict");
  } catch (error) {
    // 読めない競合の記録は、失敗の文言だけを出して一覧を変えない。
    if (error instanceof DecodeFailure) {
      return undefined;
    }
    throw error;
  }
}

function withEntry<V>(
  map: ReadonlyMap<string, V>,
  key: string,
  value: V | undefined,
): ReadonlyMap<string, V> {
  const next = new Map(map);
  if (value === undefined) {
    next.delete(key);
  } else {
    next.set(key, value);
  }
  return next;
}

/** 採用で所見ができたことを受け取る関数の組。 */
export type AssistProposalAdoptionHandlers = {
  /** この画面の採用が成功したときに、採用の応答を受け取る。 */
  onAdopted: (adoption: AssistProposalAdoption) => void;
  /**
   * 採用の要求が `assist_proposal_decided` で退けられ、競合の応答の提案が採用済みだったときに、
   * その提案を受け取る。別の操作の採用が作った所見は、この画面の所見の一覧にまだ無い。
   */
  onAdoptedElsewhere: (item: AssistProposalItem) => void;
};

/**
 * AI 提案を読み込んで保ち、分析者の採用と却下を送る。
 *
 * **提案は分析者の所見と別の一覧に置く。** 採用で所見ができたことは `handlers` に知らせ、
 * 所見の一覧とグラフの取り直しは受け取った側が行う。
 */
export function useAssistProposals(
  matchConditions: MatchConditionSelection,
  handlers: AssistProposalAdoptionHandlers,
): AssistProposalsView {
  const [state, setState] = useState<FetchState<AssistProposalItem[]>>({
    status: "loading",
  });
  const [reloadCount, setReloadCount] = useState(0);
  const [drafts, setDrafts] = useState<
    ReadonlyMap<string, AssistProposalDecisionDraft>
  >(new Map());
  const [deciding, setDeciding] = useState<ReadonlySet<string>>(new Set());
  // **送信中の判定は描画を待たずに読む。** 同じ提案の 2 回目の操作が、1 回目の状態の更新を
  // 描画する前に届いても送らない。
  const decidingIds = useRef(new Set<string>());
  const [failures, setFailures] = useState<ReadonlyMap<string, FetchFailure>>(
    new Map(),
  );

  // reloadCount は取り直しの合図であり、値は読まない。
  // biome-ignore lint/correctness/useExhaustiveDependencies: 取り直しの合図で取得をやり直す
  useEffect(() => {
    const controller = new AbortController();
    const load = async () => {
      const result = await fetchAssistProposals(matchConditions, {
        signal: controller.signal,
      });
      if (controller.signal.aborted) {
        return;
      }
      if (!result.ok) {
        setState({ status: "failed", failure: result.failure });
        return;
      }
      const proposals = result.value.proposals;
      setState({ status: "loaded", value: proposals });
      // 採否が決まった提案の、前の採否の失敗を消す。
      setFailures((previous) => {
        const next = new Map(previous);
        for (const item of proposals) {
          if (item.proposal.state !== "proposed") {
            next.delete(item.proposal.id);
          }
        }
        return next.size === previous.size ? previous : next;
      });
    };
    void load().catch(() => {
      if (controller.signal.aborted) {
        return;
      }
      setState({
        status: "failed",
        failure: buildFetchFailure("unexpected", listFailureSummary),
      });
    });
    return () => controller.abort();
  }, [matchConditions, reloadCount]);

  const reload = useCallback(() => setReloadCount((count) => count + 1), []);

  const changeDraft = useCallback(
    (proposalId: string, draft: AssistProposalDecisionDraft) =>
      setDrafts((previous) => withEntry(previous, proposalId, draft)),
    [],
  );

  // 採否を 1 件送る。成功したら一覧の提案を置き換え、下書きを消す。
  const decide = useCallback(
    <T>(
      proposalId: string,
      send: () => Promise<ApiResult<T>>,
      decided: (value: T) => AssistProposalItem,
      afterSuccess: (value: T) => void,
      afterConflict: (current: AssistProposalItem) => void,
    ) => {
      if (decidingIds.current.has(proposalId)) {
        return;
      }
      decidingIds.current.add(proposalId);
      setDeciding(new Set(decidingIds.current));
      setFailures((previous) => withEntry(previous, proposalId, undefined));
      const finish = () => {
        decidingIds.current.delete(proposalId);
        setDeciding(new Set(decidingIds.current));
      };
      const run = async () => {
        const result = await send();
        finish();
        if (!result.ok) {
          setFailures((previous) =>
            withEntry(previous, proposalId, result.failure),
          );
          // 別の操作が先に採否を決めた提案は、応答が持つ現在の提案に置き換える。
          const current = conflictedProposal(result.failure);
          if (current !== undefined) {
            setState((previous) =>
              previous.status === "loaded"
                ? { status: "loaded", value: replaced(previous.value, current) }
                : previous,
            );
            afterConflict(current);
          }
          return;
        }
        const item = decided(result.value);
        setState((previous) =>
          previous.status === "loaded"
            ? { status: "loaded", value: replaced(previous.value, item) }
            : previous,
        );
        setDrafts((previous) => withEntry(previous, proposalId, undefined));
        afterSuccess(result.value);
      };
      void run().catch(() => {
        finish();
        setFailures((previous) =>
          withEntry(
            previous,
            proposalId,
            buildFetchFailure("unexpected", decisionFailureSummary),
          ),
        );
      });
    },
    [],
  );

  const { onAdopted, onAdoptedElsewhere } = handlers;
  // 競合の応答の提案が採用済みなら、別の操作の採用が作った所見を受け取る側へ知らせる。
  const adoptedElsewhere = useCallback(
    (current: AssistProposalItem) => {
      if (current.proposal.state === "adopted") {
        onAdoptedElsewhere(current);
      }
    },
    [onAdoptedElsewhere],
  );

  const adopt = useCallback(
    (proposalId: string, draft: AssistProposalAdoptionDraft) =>
      decide(
        proposalId,
        () => adoptAssistProposal(proposalId, draft, matchConditions),
        (adoption) => adoption.proposal,
        onAdopted,
        adoptedElsewhere,
      ),
    [decide, matchConditions, onAdopted, adoptedElsewhere],
  );

  const reject = useCallback(
    (proposalId: string, draft: AssistProposalRejectionDraft) =>
      decide(
        proposalId,
        () => rejectAssistProposal(proposalId, draft, matchConditions),
        (item) => item,
        () => undefined,
        adoptedElsewhere,
      ),
    [decide, matchConditions, adoptedElsewhere],
  );

  const loaded = useMemo<FetchState<LoadedAssistProposals>>(
    () =>
      state.status === "loaded"
        ? {
            status: "loaded",
            value: { items: state.value, byTarget: indexByTarget(state.value) },
          }
        : state,
    [state],
  );

  return {
    state: loaded,
    reload,
    drafts,
    changeDraft,
    deciding,
    failures,
    adopt,
    reject,
  };
}
