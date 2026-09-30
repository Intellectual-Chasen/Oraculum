import {
  decodeSigmaRuleCandidatesResponse,
  type SigmaRuleCandidatesResponse,
} from "../contracts/sigmaRuleCandidates";
import { buildFetchFailure } from "./apiFailure";
import {
  type GraphRequest,
  graphRequestParams,
  isValidGraphRequest,
} from "./graph";
import { type ApiResult, requestJson } from "./httpClient";

const failureSummary = "Sigma のルールの候補の取得";

/**
 * Sigma のルールの候補の操作 (`GET /api/v0/sigma-rule-candidates`) を、グラフと同じ条件で実行する。
 * server は条件のうち、レコードのフィルタと文字列の条件で一致をフィルタする。
 */
export function fetchSigmaRuleCandidates(
  request: GraphRequest,
  signal?: AbortSignal,
): Promise<ApiResult<SigmaRuleCandidatesResponse>> {
  if (!isValidGraphRequest(request)) {
    return Promise.resolve({
      ok: false,
      failure: buildFetchFailure("request_rejected", failureSummary),
    });
  }
  return requestJson({
    path: "/sigma-rule-candidates",
    searchParams: graphRequestParams(request),
    decode: decodeSigmaRuleCandidatesResponse,
    failureSummary,
    signal,
  });
}
