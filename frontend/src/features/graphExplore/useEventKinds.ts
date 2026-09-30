import { useEffect, useMemo, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import { fetchEventKinds } from "@/shared/api/eventKinds";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import type { CaseId } from "@/shared/contracts/cases";
import type { EventKindsResponse } from "@/shared/contracts/eventKinds";
import type { FetchState } from "@/shared/lib/fetchState";

const failureSummary = "イベントの種類の一覧の取得";

/**
 * 事象の種別の条件に与える値の選択肢を取得する。
 * 端末と案件で絞ったときは、その範囲のレコードが持つ組だけを返す。
 */
export function useEventKinds(
  matchConditions: MatchConditionSelection,
  terminal: string | undefined,
  caseId: CaseId | undefined,
  version: number,
): FetchState<EventKindsResponse> {
  const [state, setState] = useState<FetchState<EventKindsResponse>>({
    status: "loading",
  });

  // version が変わると取り込み結果が変わりうるため、要求の組に含めて取り直す。
  const request = useMemo(
    () => ({ matchConditions, terminal, caseId, version }),
    [matchConditions, terminal, caseId, version],
  );

  useEffect(() => {
    const controller = new AbortController();
    setState({ status: "loading" });
    fetchEventKinds(request, { signal: controller.signal })
      .then((result) => {
        if (controller.signal.aborted) return;
        setState(
          result.ok
            ? { status: "loaded", value: result.value }
            : { status: "failed", failure: result.failure },
        );
      })
      .catch(() => {
        if (controller.signal.aborted) return;
        setState({
          status: "failed",
          failure: buildFetchFailure("unexpected", failureSummary),
        });
      });
    return () => controller.abort();
  }, [request]);

  return state;
}
