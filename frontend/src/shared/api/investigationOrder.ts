import {
  decodeInvestigationOrderResponse,
  type InvestigationOrderMethod,
  type InvestigationOrderResponse,
  type SigmaMinLevel,
} from "../contracts/investigationOrder";
import { buildFetchFailure } from "./apiFailure";
import {
  type GraphRequest,
  graphRequestParams,
  isValidGraphRequest,
} from "./graph";
import { type ApiResult, requestJson } from "./httpClient";

const failureSummary = "調べる順序の目安の取得";

/**
 * グラフと同じ条件で、調べる順序の目安 (`GET /api/v0/investigation-order/{method}`) を取得する。
 * sigmaMinLevel は Sigma の手法が一致を数えるルールのレベルの下限で、省略は全レベルを数える。
 */
export async function fetchInvestigationOrder(
  request: GraphRequest,
  method: InvestigationOrderMethod,
  sigmaMinLevel: SigmaMinLevel | undefined,
  signal?: AbortSignal,
): Promise<ApiResult<InvestigationOrderResponse>> {
  if (!isValidGraphRequest(request)) {
    return {
      ok: false,
      failure: buildFetchFailure("request_rejected", failureSummary),
    };
  }
  return requestJson({
    path: `/investigation-order/${encodeURIComponent(method)}`,
    searchParams: { ...graphRequestParams(request), sigmaMinLevel },
    decode: decodeInvestigationOrderResponse,
    failureSummary,
    signal,
  });
}
