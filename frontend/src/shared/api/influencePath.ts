import {
  decodeInfluencePathResponse,
  type InfluenceBasis,
  type InfluencePathResponse,
} from "../contracts/influencePath";
import { type ApiResult, requestJson } from "./httpClient";
import {
  type MatchConditionSelection,
  matchConditionParams,
} from "./matchConditions";

/** 影響の経路の要求 (`backend/api/influence_path.go` の `parseInfluencePathRequest`)。 */
export type InfluencePathRequest = {
  from: string;
  to: string;
  /** 除く根拠の種類。1 つにつき 1 回送る。 */
  excludedBases: readonly InfluenceBasis[];
  matchConditions: MatchConditionSelection;
};

/** 起点から終点までの影響の経路を取得する。 */
export function fetchInfluencePath(
  request: InfluencePathRequest,
  signal?: AbortSignal,
): Promise<ApiResult<InfluencePathResponse>> {
  return requestJson({
    path: "/influence-path",
    searchParams: {
      from: request.from,
      to: request.to,
      excludeBasis: [...request.excludedBases],
      matchCondition: matchConditionParams(request.matchConditions),
    },
    decode: decodeInfluencePathResponse,
    failureSummary: "影響の経路の取得",
    signal,
  });
}
