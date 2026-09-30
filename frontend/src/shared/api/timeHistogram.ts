import {
  decodeTimeHistogramResponse,
  type TimeHistogramResponse,
} from "../contracts/timeHistogram";
import { expressionParam, nonEmpty, numberParam } from "./graph";
import { type ApiResult, requestJson } from "./httpClient";
import { matchConditionParams } from "./matchConditions";
import type { TimelineRequest } from "./timeline";

/** 件数の分布の要求。期間を除いた絞り込みの項目は時系列と同じである。 */
export type TimeHistogramRequest = Omit<
  TimelineRequest,
  "near" | "find" | "timeFilter"
> & { columns: number };

/** 時刻の区切りと端末ごとの件数を取る (`GET /api/v0/time-histogram`)。 */
export function fetchTimeHistogram(
  request: TimeHistogramRequest,
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<TimeHistogramResponse>> {
  return requestJson({
    path: "/time-histogram",
    searchParams: {
      columns: String(request.columns),
      eventCategory: request.eventCategory,
      eventAction: request.eventAction,
      eventActionFrom: numberParam(request.eventActionFrom),
      eventActionTo: numberParam(request.eventActionTo),
      case: request.caseId,
      terminal: request.terminal,
      source: nonEmpty(request.sources),
      searchExpression: expressionParam(request.searchExpression),
      matchCondition: matchConditionParams(request.matchConditions),
    },
    decode: decodeTimeHistogramResponse,
    failureSummary: "Histogram の取得",
    signal: options.signal,
  });
}
