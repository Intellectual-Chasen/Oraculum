import { useEffect, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import { fetchRecord, type RecordRequest } from "@/shared/api/records";
import type { RecordResponse } from "@/shared/contracts/records";
import type { FetchState } from "@/shared/lib/fetchState";

const failureSummary = "レコードの取得";

/**
 * 元レコード 1 件を取得する (操作 3 と操作 7)。
 * 対象のレコード位置が変わるたびに取り直し、前の取得を `AbortController` で打ち切る。
 */
export function useRecordDetail(
  request: RecordRequest,
): FetchState<RecordResponse> {
  const [state, setState] = useState<FetchState<RecordResponse>>({
    status: "loading",
  });

  useEffect(() => {
    const controller = new AbortController();
    setState({ status: "loading" });

    const load = async () => {
      const result = await fetchRecord(request, { signal: controller.signal });
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
        failure: buildFetchFailure("unexpected", failureSummary),
      });
    });
    return () => controller.abort();
  }, [request]);

  return state;
}
