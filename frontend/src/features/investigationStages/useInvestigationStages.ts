import { useCallback, useEffect, useRef, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import type { ApiResult } from "@/shared/api/httpClient";
import {
  fetchStages,
  type LoadingSourceDraft,
  startLoading as requestLoading,
  startProcessing as requestProcessing,
  showsStaleStages,
} from "@/shared/api/stages";
import type { InvestigationStages } from "@/shared/contracts/stages";
import type { FetchFailure, FetchState } from "@/shared/lib/fetchState";

/** 実行中の段階の状態を取り直す間隔。 */
export const stagesPollIntervalMs = 1000;

const unexpectedFetchSummary = "段階の状態の取得";
const unexpectedLoadingSummary = "収集元の読み込みの開始";
const unexpectedProcessingSummary = "処理の開始";

/** 段階を始める要求の種類。 */
type StageStartKind = "loading" | "processing";

/** 調査の段階の状態と、段階を始める操作を持つ。 */
export type InvestigationStagesView = {
  /** 段階の状態。最初の取得を終えるまで `loading` である。 */
  state: FetchState<InvestigationStages>;
  /**
   * 段階の状態を取り直す要求 (実行中の段階の取り直し、段階を始める要求が失敗した後の取り直し、
   * `refresh` の取り直し) が失敗した理由。`state` は最後に読めた状態を保つ。次の取り直しか、
   * 段階を始める要求が成功すると `undefined` に戻る。
   */
  refreshFailure: FetchFailure | undefined;
  /** 段階を始める要求の失敗。次に同じ段階を始める要求が成功すると `undefined` に戻る。 */
  startFailure: { kind: StageStartKind; failure: FetchFailure } | undefined;
  /** 段階を始める要求の応答を待っているか。要求が失敗した後の取り直しを終えるまで真である。 */
  isStarting: boolean;
  /** 実行中の段階があり、段階の状態を `stagesPollIntervalMs` ごとに取り直す予約を持つか。 */
  isPolling: boolean;
  /** `refresh` で始めた取り直しの応答を待っているか。 */
  isRefreshing: boolean;
  /** 収集元の読み込みを始める。`thenProcess` が真のときは、読み込みの完了後に server が処理を始める。 */
  startLoading: (
    sources: LoadingSourceDraft[],
    thenProcess: boolean,
  ) => Promise<void>;
  /** 処理を始める。 */
  startProcessing: () => Promise<void>;
  /**
   * 段階の状態を取り直す。`state` は取り直しの応答を受け取るまで最後に読めた状態を保つ。
   * 取り直しが失敗したときも `state` を保ち、理由を `refreshFailure` に置く。
   */
  refresh: () => Promise<void>;
  /** 最初の取得に失敗した状態から、段階の状態をもう一度取得する。 */
  reload: () => void;
};

/** 読み込みと処理のどちらかの段階が実行中かを返す。 */
function hasRunningStage(stages: InvestigationStages): boolean {
  return (
    stages.loading.state === "running" || stages.processing.state === "running"
  );
}

/**
 * 段階を始める要求が失敗した後に、段階の状態を取り直すかを返す。
 *
 * **server が段階を始めたかを確かめられない失敗と、画面の段階の状態が古いことを表す失敗で
 * 取り直す。** 通信の失敗、server の失敗、読めない応答、想定外の失敗では、server が要求を
 * 受け取って段階を始めた後で応答だけが失敗した場合がある。409 の段階の code は、画面の状態が
 * 古いことを表す。要求の条件、権限、要求頻度を理由に退けた要求と、送る前の入力の検査で退けた
 * 要求では、server の段階は要求を送る前の状態のままである。
 */
function needsRefetchAfter(failure: FetchFailure): boolean {
  switch (failure.kind) {
    case "network":
    case "server":
    case "response_unreadable":
    case "unexpected":
      return true;
    case "request_rejected":
      return showsStaleStages(failure);
    case "authorization":
    case "rate_limited":
      return false;
    default: {
      const exhaustive: never = failure.kind;
      throw new Error(`unknown fetch failure kind: ${String(exhaustive)}`);
    }
  }
}

/**
 * 調査の段階の状態を取得し、段階を始める要求を送る。
 *
 * **実行中の段階があるあいだ、状態を `stagesPollIntervalMs` ごとに取り直す。** 次の取り直しは
 * 前の応答を受け取った後に予約する。画面を離れたときは予約を消し、取得を打ち切る。
 */
export function useInvestigationStages(): InvestigationStagesView {
  const [state, setState] = useState<FetchState<InvestigationStages>>({
    status: "loading",
  });
  const [refreshFailure, setRefreshFailure] = useState<
    FetchFailure | undefined
  >(undefined);
  const [startFailure, setStartFailure] =
    useState<InvestigationStagesView["startFailure"]>(undefined);
  const [isStarting, setIsStarting] = useState(false);
  const [isRefreshing, setIsRefreshing] = useState(false);
  // 取り直しを終えた回数。取り直しのたびに次の予約を組み直す。
  const [pollCount, setPollCount] = useState(0);
  // 最初の取得を始めた回数。reload のたびに取得をやり直す。
  const [loadAttempt, setLoadAttempt] = useState(0);
  // 画面の寿命の間だけ有効な signal。段階を始める要求の応答を、画面を離れた後に反映しない。
  const lifetime = useRef<AbortController | undefined>(undefined);

  useEffect(() => {
    const controller = new AbortController();
    lifetime.current = controller;
    return () => {
      controller.abort();
    };
  }, []);

  // biome-ignore lint/correctness/useExhaustiveDependencies: loadAttempt が変わったとき、つまり reload のたびに取得をやり直す。
  useEffect(() => {
    const controller = new AbortController();
    const load = async () => {
      const result = await fetchStages({ signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      setState(
        result.ok
          ? { status: "loaded", value: result.value }
          : { status: "failed", failure: result.failure },
      );
    };
    void load().catch(() => {
      if (controller.signal.aborted) {
        return;
      }
      setState({
        status: "failed",
        failure: buildFetchFailure("unexpected", unexpectedFetchSummary),
      });
    });
    return () => {
      controller.abort();
    };
  }, [loadAttempt]);

  const reload = useCallback(() => {
    setState({ status: "loading" });
    setLoadAttempt((attempt) => attempt + 1);
  }, []);

  const isRunning = state.status === "loaded" && hasRunningStage(state.value);

  /**
   * 段階の状態を取り直し、`state` と `refreshFailure` へ反映する。失敗したときは `state` を保つ。
   * 失敗を `refreshFailure` に置き、呼び出し元へ投げない。`signal` を打ち切った後の応答は反映しない。
   */
  const refetch = useCallback(async (signal: AbortSignal) => {
    let result: ApiResult<InvestigationStages>;
    try {
      result = await fetchStages({ signal });
    } catch {
      result = {
        ok: false,
        failure: buildFetchFailure("unexpected", unexpectedFetchSummary),
      };
    }
    if (signal.aborted) {
      return;
    }
    if (result.ok) {
      setRefreshFailure(undefined);
      setState({ status: "loaded", value: result.value });
    } else {
      setRefreshFailure(result.failure);
    }
  }, []);

  // biome-ignore lint/correctness/useExhaustiveDependencies: pollCount が変わったとき、つまり取り直しを終えるたびに次の取り直しを予約する。
  useEffect(() => {
    if (!isRunning) {
      return;
    }
    const controller = new AbortController();
    const poll = async () => {
      await refetch(controller.signal);
      if (!controller.signal.aborted) {
        setPollCount((count) => count + 1);
      }
    };
    const timer = setTimeout(() => {
      void poll();
    }, stagesPollIntervalMs);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [isRunning, pollCount, refetch]);

  const refresh = useCallback(async () => {
    const signal = lifetime.current?.signal;
    if (signal === undefined || signal.aborted) {
      return;
    }
    setIsRefreshing(true);
    await refetch(signal);
    if (!signal.aborted) {
      setIsRefreshing(false);
    }
  }, [refetch]);

  /**
   * 段階を始める要求を送り、応答の段階の状態を画面へ反映する。
   *
   * **段階を始める要求は、画面を離れても打ち切らない。** 送った要求を server が受け取ったか
   * 分からない状態を作らない。画面を離れた後の応答は画面へ反映しない。
   *
   * **要求が失敗し、server の段階が画面の状態と異なる場合があるときは、段階の状態を取り直す。**
   * 取り直した状態が実行中の段階を持てば、取り直しの予約が始まる。
   */
  const start = useCallback(
    async (
      kind: StageStartKind,
      send: () => Promise<ApiResult<InvestigationStages>>,
      unexpectedSummary: string,
    ) => {
      const signal = lifetime.current?.signal;
      if (signal === undefined || signal.aborted) {
        return;
      }
      setIsStarting(true);
      let result: ApiResult<InvestigationStages>;
      try {
        result = await send();
      } catch {
        result = {
          ok: false,
          failure: buildFetchFailure("unexpected", unexpectedSummary),
        };
      }
      if (signal.aborted) {
        return;
      }
      if (result.ok) {
        setStartFailure(undefined);
        setRefreshFailure(undefined);
        setState({ status: "loaded", value: result.value });
      } else {
        setStartFailure({ kind, failure: result.failure });
        if (needsRefetchAfter(result.failure)) {
          await refetch(signal);
          if (signal.aborted) {
            return;
          }
        }
      }
      setIsStarting(false);
    },
    [refetch],
  );

  const startLoading = useCallback(
    (sources: LoadingSourceDraft[], thenProcess: boolean) =>
      start(
        "loading",
        () => requestLoading(sources, { thenProcess }),
        unexpectedLoadingSummary,
      ),
    [start],
  );

  const startProcessing = useCallback(
    () =>
      start(
        "processing",
        () => requestProcessing(),
        unexpectedProcessingSummary,
      ),
    [start],
  );

  return {
    state,
    refreshFailure,
    startFailure,
    isStarting,
    isPolling: isRunning,
    isRefreshing,
    startLoading,
    startProcessing,
    refresh,
    reload,
  };
}
