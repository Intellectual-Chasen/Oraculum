import {
  decodeProxyBypassResponse,
  type ProxyBypassResponse,
} from "../contracts/proxyBypass";
import { type ApiResult, requestJson } from "./httpClient";
import {
  type MatchConditionSelection,
  matchConditionParams,
} from "./matchConditions";

/** Proxy を経由した要求と経由しない接続の比較 (`GET /api/v0/proxy-bypass`) を実行する。 */
export function fetchProxyBypass(
  request: {
    matchConditions: MatchConditionSelection;
    source: { sourceId: string; contentSha256: string };
  },
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<ProxyBypassResponse>> {
  return requestJson({
    path: "/proxy-bypass",
    searchParams: {
      sourceId: request.source.sourceId,
      sourceContentSha256: request.source.contentSha256,
      matchCondition: matchConditionParams(request.matchConditions),
    },
    decode: decodeProxyBypassResponse,
    failureSummary: "Proxy の経由と迂回の件数の取得",
    signal: options.signal,
  });
}
