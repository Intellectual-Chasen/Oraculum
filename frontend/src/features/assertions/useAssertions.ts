import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import {
  type AssertionDraft,
  createAssertion,
  fetchAssertions,
} from "@/shared/api/assertions";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import type {
  AssertionItem,
  AssertionTarget,
} from "@/shared/contracts/assertions";
import { assertionTargetKey } from "@/shared/lib/assertionTargetKey";
import type { FetchFailure, FetchState } from "@/shared/lib/fetchState";

const listFailureSummary = "メモの取得";
const recordFailureSummary = "メモの記録";

/**
 * 読み込んだ所見と、対象で探せる索引である。
 *
 * **索引を一覧と一緒に返す。** 画面はノードの詳細と関係の詳細の双方で同じ一覧を読む。
 * 対象ごとに一覧を走査する形にすると、描画のたびに所見の件数と画面上の対象の個数の積
 * だけ比較が走る。
 */
export type LoadedAssertions = {
  items: AssertionItem[];
  /** 対象を指す文字列から、その対象に付いた有効な所見を探す。 */
  byTarget: ReadonlyMap<string, AssertionItem[]>;
};

/**
 * 記録が失敗したときの理由と、記録しようとした対象。
 *
 * **対象を一緒に返す。** 1 つの一覧を 2 つの欄が読むため、対象を持たないと、片方の
 * 記録の失敗がもう片方の対象の欄にも出る。
 */
export type AssertionRecordFailure = {
  failure: FetchFailure;
  target: AssertionTarget;
};

/**
 * 記録が成功した対象と、成功した回数。
 *
 * **対象と回数を一緒に返す。** 1 つの一覧を 2 つの欄が読むため、成功したことだけを
 * 返すと、片方の記録の成功がもう片方の欄の入力欄も戻す。同じ対象へ続けて記録した
 * 2 回を回数が分ける。
 */
export type AssertionRecordSuccess = {
  target: AssertionTarget;
  /** 記録が成功した通算の回数。最初の成功で 1 になる。 */
  count: number;
};

/** 所見の一覧と、記録の結果を持つ。 */
export type AssertionsView = {
  state: FetchState<LoadedAssertions>;
  /** 記録が失敗したときの理由と対象。**読めた一覧を消さない。** */
  recordFailure?: AssertionRecordFailure;
  /** 記録が成功した対象と回数。記録が 1 件も成功していない間は持たない。 */
  recordSuccess?: AssertionRecordSuccess;
  /** 記録の途中は真である。 */
  recording: boolean;
  /** 所見を 1 件記録し、成功したら一覧の末尾へ足す。 */
  record: (draft: AssertionDraft) => void;
};

/** 所見の一覧の所有者が持つ操作。画面の欄に渡す `AssertionsView` に、一覧を変える操作を加える。 */
export type AssertionsOwner = AssertionsView & {
  /**
   * 別の操作が作った所見 1 件を一覧に出す。同じ識別子の所見があれば置き換え、無ければ末尾へ
   * 足す。足した時点で途中だった取得の応答にも足す。その取得は所見を作る前の一覧を返しうるため
   * である。一覧の取得が失敗していれば、一覧を取り直す。
   */
  add: (item: AssertionItem) => void;
  /** 一覧を取り直す。 */
  reload: () => void;
};

/** 同じ識別子の所見を置き換え、無ければ末尾へ足した一覧を返す。 */
function withItem(
  items: AssertionItem[],
  item: AssertionItem,
): AssertionItem[] {
  const index = items.findIndex(
    (current) => current.assertion.id === item.assertion.id,
  );
  if (index < 0) {
    return [...items, item];
  }
  return items.map((current, at) => (at === index ? item : current));
}

/**
 * 対象を指す文字列から、その対象に付いた有効な所見を探す索引を組む。
 *
 * 状態が `active` でない所見を索引に入れない。改訂を重ねて取り下げた所見を画面に出さない。
 */
function indexByTarget(
  items: AssertionItem[],
): ReadonlyMap<string, AssertionItem[]> {
  const index = new Map<string, AssertionItem[]>();
  for (const item of items) {
    if (item.assertion.state !== "active") {
      continue;
    }
    const key = assertionTargetKey(item.assertion.target);
    if (key === undefined) {
      continue;
    }
    const found = index.get(key);
    if (found === undefined) {
      index.set(key, [item]);
      continue;
    }
    found.push(item);
  }
  return index;
}

/**
 * 分析者の所見を読み込んで保つ。一覧の操作は記録した所見を全件返す。
 *
 * **記録した所見は一覧の末尾へ足す。** 応答が記録した組をそのまま返すため、一覧を
 * 取り直さずに同じ内容を出せる。
 */
export function useAssertions(
  matchConditions: MatchConditionSelection,
): AssertionsOwner {
  const [state, setState] = useState<FetchState<AssertionItem[]>>({
    status: "loading",
  });
  const [recording, setRecording] = useState(false);
  const [recordFailure, setRecordFailure] = useState<
    AssertionRecordFailure | undefined
  >(undefined);
  const [recordSuccess, setRecordSuccess] = useState<
    AssertionRecordSuccess | undefined
  >(undefined);
  const [reloadCount, setReloadCount] = useState(0);
  // 始めた取得の通番。
  const fetchSeq = useRef(0);
  // 途中の取得があるか。
  const isFetching = useRef(false);
  // 一覧の取得が失敗しているか。
  const hasFailed = useRef(false);
  // 足した所見と、足した時点で最後に始めていた取得の通番。その通番以前の取得の応答に足す。
  const addedItems = useRef<{ item: AssertionItem; afterFetch: number }[]>([]);

  // reloadCount は取り直しの合図であり、値は読まない。
  // biome-ignore lint/correctness/useExhaustiveDependencies: 取り直しの合図で取得をやり直す
  useEffect(() => {
    const controller = new AbortController();
    fetchSeq.current += 1;
    const seq = fetchSeq.current;
    isFetching.current = true;
    const fail = (failure: FetchFailure) => {
      isFetching.current = false;
      hasFailed.current = true;
      setState({ status: "failed", failure });
    };
    const load = async () => {
      const result = await fetchAssertions(
        { matchConditions },
        { signal: controller.signal },
      );
      if (controller.signal.aborted) {
        return;
      }
      if (!result.ok) {
        fail(result.failure);
        return;
      }
      isFetching.current = false;
      hasFailed.current = false;
      // **この取得を始めた後に足した所見は、応答に無いことがある。** 応答に無い所見だけを足す。
      // 応答が持つ所見は、所見を作った後に server が組んだ値であり、応答の値を採る。
      addedItems.current = addedItems.current.filter(
        (added) => added.afterFetch >= seq,
      );
      const items = [...result.value.assertions];
      const ids = new Set(items.map((item) => item.assertion.id));
      for (const added of addedItems.current) {
        if (!ids.has(added.item.assertion.id)) {
          ids.add(added.item.assertion.id);
          items.push(added.item);
        }
      }
      setState({ status: "loaded", value: items });
    };
    void load().catch(() => {
      if (controller.signal.aborted) {
        return;
      }
      fail(buildFetchFailure("unexpected", listFailureSummary));
    });
    return () => controller.abort();
  }, [matchConditions, reloadCount]);

  const reload = useCallback(() => setReloadCount((count) => count + 1), []);

  const add = useCallback(
    (item: AssertionItem) => {
      addedItems.current.push({ item, afterFetch: fetchSeq.current });
      setState((previous) =>
        previous.status === "loaded"
          ? { status: "loaded", value: withItem(previous.value, item) }
          : previous,
      );
      if (!isFetching.current && hasFailed.current) {
        reload();
      }
    },
    [reload],
  );

  const record = useCallback(
    (draft: AssertionDraft) => {
      if (recording) {
        return;
      }
      setRecording(true);
      setRecordFailure(undefined);
      const send = async () => {
        const result = await createAssertion(draft, matchConditions);
        setRecording(false);
        if (!result.ok) {
          // **読めた一覧を残す。** 記録の失敗で、出していた所見を消さない。
          setRecordFailure({ failure: result.failure, target: draft.target });
          return;
        }
        setRecordSuccess((previous) => ({
          target: draft.target,
          count: (previous?.count ?? 0) + 1,
        }));
        add(result.value);
      };
      void send().catch(() => {
        setRecording(false);
        setRecordFailure({
          failure: buildFetchFailure("unexpected", recordFailureSummary),
          target: draft.target,
        });
      });
    },
    [recording, matchConditions, add],
  );

  // 索引は一覧が変わったときだけ組み直す。対象ごとの描画では探すだけにする。
  const loaded = useMemo<FetchState<LoadedAssertions>>(
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
    recordFailure,
    recordSuccess,
    recording,
    record,
    add,
    reload,
  };
}
