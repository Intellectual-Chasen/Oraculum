import { useEffect, useMemo, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import { fetchSigmaRuleCandidates } from "@/shared/api/sigmaRuleCandidates";
import type { SigmaRuleCandidatesResponse } from "@/shared/contracts/sigmaRuleCandidates";
import type { FetchState } from "@/shared/lib/fetchState";
import type { SubgraphCriteria } from "./useSubgraph";

/**
 * 部分グラフと同じ条件で Sigma のルールの候補を取得する。条件か端末の割当の記録の回数が
 * 変わるたびに取得し直す。
 */
export function useSigmaRuleCandidates(
  criteria: SubgraphCriteria,
  version: number,
): FetchState<SigmaRuleCandidatesResponse> {
  const [state, setState] = useState<FetchState<SigmaRuleCandidatesResponse>>({
    status: "loading",
  });
  const request = useMemo(() => ({ criteria, version }), [criteria, version]);
  useEffect(() => {
    const controller = new AbortController();
    setState({ status: "loading" });
    const load = async () => {
      const result = await fetchSigmaRuleCandidates(
        request.criteria,
        controller.signal,
      );
      if (controller.signal.aborted) return;
      setState(
        result.ok
          ? { status: "loaded", value: result.value }
          : { status: "failed", failure: result.failure },
      );
    };
    void load().catch(() => {
      if (controller.signal.aborted) return;
      setState({
        status: "failed",
        failure: buildFetchFailure("unexpected", "Sigma ルールの候補の取得"),
      });
    });
    return () => controller.abort();
  }, [request]);
  return state;
}
