import { useEffect, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import { fetchTimeline, type TimelineRequest } from "@/shared/api/timeline";
import type { TimelineResponse } from "@/shared/contracts/timeline";
import type { FetchState } from "@/shared/lib/fetchState";

const failureSummary = "時系列の取得";

/**
 * 時系列を取得する。
 * 条件が変わるたびと、`version` が変わるたびに取り直し、前の取得を `AbortController` で
 * 打ち切る。`version` は端末の割当を記録した回数である。記録で行が名乗る端末が変わる。
 */
export function useTimeline(
  request: TimelineRequest,
  version: number,
): FetchState<TimelineResponse> {
  const [state, setState] = useState<FetchState<TimelineResponse>>({
    status: "loading",
  });

  // biome-ignore lint/correctness/useExhaustiveDependencies: version が変わったときに、同じ条件で取り直す。
  useEffect(() => {
    const controller = new AbortController();
    setState({ status: "loading" });

    const load = async () => {
      const result = await fetchTimeline(request, {
        signal: controller.signal,
      });
      if (controller.signal.aborted) {
        return;
      }
      if (!result.ok) {
        setState({ status: "failed", failure: result.failure });
        return;
      }
      // **0 件の応答も loaded で渡す。** 応答は絞り込みに用いた期間と収集元ごとの
      // 収録範囲を含むため、取得の状態へまとめると画面がその条件を出せない。
      setState({ status: "loaded", value: result.value });
    };

    void load().catch(() => {
      if (controller.signal.aborted) {
        return;
      }
      setState({
        status: "failed",
        failure: buildFetchFailure("unexpected", failureSummary),
      });
    });
    return () => controller.abort();
  }, [request, version]);

  return state;
}
