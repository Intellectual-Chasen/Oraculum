import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import {
  type AssertionDraft,
  type AssertionRevisionDraft,
  createAssertion,
  fetchAssertions,
  reviseAssertion,
} from "@/shared/api/assertions";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import {
  type Assertion,
  decodeAssertionItem,
} from "@/shared/contracts/assertions";
import { DecodeFailure } from "@/shared/contracts/decoding";
import type { FetchFailure, FetchState } from "@/shared/lib/fetchState";

const listFailureSummary = "タイムゾーンの取得";
const recordFailureSummary = "タイムゾーンの記録";
/** 競合の応答が相手の所見を読める形で含まなかったときに、次に行う操作のラベル。 */
const unreadableConflictNextAction = "再読み込みした最新の値を確認して再度記録";

/**
 * 収集元 1 件の時刻の解釈。
 *
 * - `single`: 解釈を持つ所見 1 件。主張中の所見があればそれであり、無ければ最後に記録した
 *   取り消し済みの所見である。
 * - `conflicted`: 主張中の所見が 2 件以上ある。backend はどちらのずれでも読まず、解釈を
 *   適用しない (`backend/pipeline/time_interpretation.go` の `timeInterpretationsOf`)。
 */
export type SourceInterpretation =
  | { kind: "single"; assertion: Assertion }
  | { kind: "conflicted"; assertions: Assertion[] };

/** 記録が失敗したときの理由と、記録しようとした収集元の内容の識別。 */
export type TimeInterpretationFailure = {
  sourceContentSha256: string;
  failure: FetchFailure;
};

/**
 * 別の分析者が先に同じ解釈を記録していたため、server が退けた記録。
 *
 * `theirs` は server の現在の所見、`mine` は退けられた入力である。`mine.baseRevision` は
 * 退けられた時点の値であり、送り直すときは `theirs.revisionNumber` にする。
 */
export type TimeInterpretationConflict = {
  sourceContentSha256: string;
  theirs: Assertion;
  mine: AssertionRevisionDraft;
};

/** 収集元の時刻の解釈の一覧と、記録の結果を持つ。 */
export type TimeInterpretationsView = {
  /** 収集元の内容の識別から、その収集元の解釈を探す。 */
  state: FetchState<ReadonlyMap<string, SourceInterpretation>>;
  /** 記録が失敗したときの理由と収集元。**読めた一覧を消さない。** */
  recordFailure: TimeInterpretationFailure | undefined;
  /** 別の分析者の記録と競合して退けられた記録。次の記録を送るか、`dismissConflict` で消える。 */
  conflict: TimeInterpretationConflict | undefined;
  /** 競合の表示を消す。何も送らない。 */
  dismissConflict: () => void;
  /** 記録が成功した通算の回数。時系列とグラフを取り直す合図に使う。 */
  recordedCount: number;
  /** 最後に記録が成功した収集元の内容の識別。成功がまだ無い間は持たない。 */
  lastRecordedSource: string | undefined;
  /** 記録の途中は真である。 */
  recording: boolean;
  /** 解釈の最初の改訂を記録する。 */
  create: (draft: AssertionDraft) => void;
  /** 既存の解釈へ新しい改訂を足す。取り消しは `withdrawn` の改訂である。 */
  revise: (assertionId: string, draft: AssertionRevisionDraft) => void;
};

/** 所見の一覧から、収集元ごとの解釈を backend と同じ規則で組む。 */
function bySource(
  assertions: Assertion[],
): ReadonlyMap<string, SourceInterpretation> {
  const grouped = new Map<string, Assertion[]>();
  for (const assertion of assertions) {
    const content = assertion.target.sourceContentSha256;
    if (assertion.target.kind !== "source" || content === undefined) {
      continue;
    }
    grouped.set(content, [...(grouped.get(content) ?? []), assertion]);
  }
  const index = new Map<string, SourceInterpretation>();
  for (const [content, items] of grouped) {
    const active = items.filter((item) => item.state === "active");
    if (active.length >= 2) {
      index.set(content, { kind: "conflicted", assertions: active });
      continue;
    }
    const assertion = active[0] ?? items[items.length - 1];
    if (assertion !== undefined) {
      index.set(content, { kind: "single", assertion });
    }
  }
  return index;
}

/** `assertion_changed` の失敗から、server の現在の所見を読む。読めないときは持たない。 */
function conflictedAssertion(failure: FetchFailure): Assertion | undefined {
  if (failure.failureCode !== "assertion_changed") {
    return undefined;
  }
  try {
    return decodeAssertionItem(failure.conflict, "conflict").assertion;
  } catch (cause) {
    if (!(cause instanceof DecodeFailure)) throw cause;
    return undefined;
  }
}

/**
 * 収集元の時刻の解釈を読み、記録・変更・取り消しを送る。
 *
 * **記録が成功したら、応答の所見で一覧の同じ所見を置き換える。** 応答は履歴を含む所見の
 * 全体を返す。**記録が失敗したら一覧を取り直す。** 取り直しの間も読めた一覧を出し続ける。
 *
 * **別の分析者の記録を通知せずに置き換えない。** 改訂は画面が読んだ `revisionNumber` を
 * `baseRevision` として送る。server が `assertion_changed` で退けたら、応答の現在の所見を
 * 一覧へ入れ、`conflict` に相手の値と自分の値を持つ。
 */
export function useTimeInterpretations(
  matchConditions: MatchConditionSelection,
): TimeInterpretationsView {
  const [state, setState] = useState<FetchState<Assertion[]>>({
    status: "loading",
  });
  const [reloads, setReloads] = useState(0);
  const [recordFailure, setRecordFailure] = useState<
    TimeInterpretationFailure | undefined
  >(undefined);
  const [recordedCount, setRecordedCount] = useState(0);
  const [lastRecordedSource, setLastRecordedSource] = useState<
    string | undefined
  >(undefined);
  const [recording, setRecording] = useState(false);
  const [conflict, setConflict] = useState<
    TimeInterpretationConflict | undefined
  >(undefined);

  // biome-ignore lint/correctness/useExhaustiveDependencies: reloads は取り直しの合図であり、値を読まない。
  useEffect(() => {
    const controller = new AbortController();
    void fetchAssertions({ matchConditions }, { signal: controller.signal })
      .then((result) => {
        if (controller.signal.aborted) {
          return;
        }
        setState(
          result.ok
            ? {
                status: "loaded",
                value: result.value.assertions.map((item) => item.assertion),
              }
            : {
                status: "failed",
                failure: { ...result.failure, summary: listFailureSummary },
              },
        );
      })
      .catch(() => {
        if (!controller.signal.aborted) {
          setState({
            status: "failed",
            failure: buildFetchFailure("unexpected", listFailureSummary),
          });
        }
      });
    return () => controller.abort();
  }, [matchConditions, reloads]);

  const sending = useRef(false);
  const send = useCallback(
    (
      mine: AssertionRevisionDraft,
      request: () => ReturnType<typeof createAssertion>,
    ) => {
      // 描画を待たずに 2 回目のクリックを止めるため、state でなく ref で判定する。
      if (sending.current) {
        return;
      }
      sending.current = true;
      const sourceContentSha256 = mine.target.sourceContentSha256 ?? "";
      setRecording(true);
      setRecordFailure(undefined);
      setConflict(undefined);
      const failed = (failure: FetchFailure) => {
        setRecordFailure({ sourceContentSha256, failure });
        setReloads((count) => count + 1);
      };
      const replace = (assertion: Assertion) =>
        setState((previous) =>
          previous.status === "loaded"
            ? {
                status: "loaded",
                value: [
                  ...previous.value.filter((item) => item.id !== assertion.id),
                  assertion,
                ],
              }
            : previous,
        );
      void request()
        .then((result) => {
          sending.current = false;
          setRecording(false);
          if (!result.ok) {
            const theirs = conflictedAssertion(result.failure);
            if (theirs === undefined) {
              failed(
                result.failure.failureCode === "assertion_changed"
                  ? {
                      ...result.failure,
                      nextAction: unreadableConflictNextAction,
                    }
                  : result.failure,
              );
              return;
            }
            replace(theirs);
            setConflict({ sourceContentSha256, theirs, mine });
            return;
          }
          replace(result.value.assertion);
          setLastRecordedSource(sourceContentSha256);
          setRecordedCount((count) => count + 1);
        })
        .catch(() => {
          sending.current = false;
          setRecording(false);
          failed(buildFetchFailure("unexpected", recordFailureSummary));
        });
    },
    [],
  );

  const create = useCallback(
    (draft: AssertionDraft) =>
      // 作成が競合したときに改訂として送り直せるよう、最初の改訂の形で持つ。
      send({ ...draft, state: "active", baseRevision: 0 }, () =>
        createAssertion(draft, matchConditions),
      ),
    [send, matchConditions],
  );
  const revise = useCallback(
    (assertionId: string, draft: AssertionRevisionDraft) =>
      send(draft, () => reviseAssertion(assertionId, draft, matchConditions)),
    [send, matchConditions],
  );
  const dismissConflict = useCallback(() => setConflict(undefined), []);

  // 索引は一覧が変わったときだけ組み直す。
  const indexed = useMemo<
    FetchState<ReadonlyMap<string, SourceInterpretation>>
  >(
    () =>
      state.status === "loaded"
        ? { status: "loaded", value: bySource(state.value) }
        : state,
    [state],
  );
  return {
    state: indexed,
    recordFailure,
    conflict,
    dismissConflict,
    recordedCount,
    lastRecordedSource,
    recording,
    create,
    revise,
  };
}
