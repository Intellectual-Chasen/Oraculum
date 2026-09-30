import { useEffect, useMemo, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import { fetchInvestigationOrder } from "@/shared/api/investigationOrder";
import type {
  InvestigationOrderMethod,
  InvestigationOrderResponse,
  SigmaMinLevel,
} from "@/shared/contracts/investigationOrder";
import type { FetchState } from "@/shared/lib/fetchState";
import type { SubgraphCriteria } from "./useSubgraph";

const failureSummary = "調べる順序の取得";

/**
 * 部分グラフと同じ条件で、調べる順序の目安を取得する。
 * sigmaMinLevel は Sigma の手法だけに渡し、ほかの手法の要求には付けない。
 */
export function useInvestigationOrder(
  criteria: SubgraphCriteria,
  method: InvestigationOrderMethod,
  sigmaMinLevel: SigmaMinLevel | undefined,
  version: number,
): FetchState<InvestigationOrderResponse> {
  const [state, setState] = useState<FetchState<InvestigationOrderResponse>>({
    status: "loading",
  });
  const minLevel = method === "sigma" ? sigmaMinLevel : undefined;
  const request = useMemo(
    () => ({ criteria, method, minLevel, version }),
    [criteria, method, minLevel, version],
  );

  useEffect(() => {
    const controller = new AbortController();
    setState({ status: "loading" });

    const load = async () => {
      const result = await fetchInvestigationOrder(
        request.criteria,
        request.method,
        request.minLevel,
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
