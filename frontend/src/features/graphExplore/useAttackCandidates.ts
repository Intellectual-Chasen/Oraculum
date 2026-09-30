import { useEffect, useMemo, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import { fetchAttackCandidates } from "@/shared/api/attackCandidates";
import type { AttackCandidatesResponse } from "@/shared/contracts/attackCandidates";
import type { FetchState } from "@/shared/lib/fetchState";
import type { SubgraphCriteria } from "./useSubgraph";

const failureSummary = "ATT&CK 候補の取得";

/** 部分グラフと同じ条件でATT&CK候補を取得する。 */
export function useAttackCandidates(
  criteria: SubgraphCriteria,
  version: number,
): FetchState<AttackCandidatesResponse> {
  const [state, setState] = useState<FetchState<AttackCandidatesResponse>>({
    status: "loading",
  });
  const request = useMemo(() => ({ criteria, version }), [criteria, version]);

  useEffect(() => {
    const controller = new AbortController();
    setState({ status: "loading" });

    const load = async () => {
      const result = await fetchAttackCandidates(
        request.criteria,
        controller.signal,
      );
      if (controller.signal.aborted) return;
      if (!result.ok) {
        setState({ status: "failed", failure: result.failure });
        return;
      }
      setState({ status: "loaded", value: result.value });
    };

    void load().catch(() => {
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
