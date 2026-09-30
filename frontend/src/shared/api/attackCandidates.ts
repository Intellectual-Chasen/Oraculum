import {
  type AttackCandidatesResponse,
  decodeAttackCandidatesResponse,
} from "../contracts/attackCandidates";
import { buildFetchFailure } from "./apiFailure";
import {
  type GraphRequest,
  graphRequestParams,
  isValidGraphRequest,
} from "./graph";
import { type ApiResult, requestJson } from "./httpClient";

const failureSummary = "ATT&CK 候補の取得";

/** graphと同じ条件で一時的なATT&CK候補を取得する。 */
export async function fetchAttackCandidates(
  request: GraphRequest,
  signal?: AbortSignal,
): Promise<ApiResult<AttackCandidatesResponse>> {
  if (!isValidGraphRequest(request)) {
    return {
      ok: false,
      failure: buildFetchFailure("request_rejected", failureSummary),
    };
  }
  return requestJson({
    path: "/attack-candidates",
    searchParams: graphRequestParams(request),
    decode: decodeAttackCandidatesResponse,
    failureSummary,
    signal,
  });
}
