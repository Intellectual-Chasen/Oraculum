import {
  decodeNodeSummariesResponse,
  type NodeSummariesResponse,
} from "../contracts/nodeSummaries";
import { buildFetchFailure } from "./apiFailure";
import { expressionParam, nonEmpty, numberParam } from "./graph";
import { type ApiResult, requestJson } from "./httpClient";
import { matchConditionParams } from "./matchConditions";
import type { TimelineRequest } from "./timeline";

const failureSummary = "ノードの一覧の取得";

/** ノードの一覧の要求。絞り込みの項目は時系列と同じである。 */
export type NodeSummariesRequest = Omit<TimelineRequest, "near" | "find"> & {
  nodeKind: "terminal" | "ip";
};

/**
 * 1 つの種別のノードごとに、接するレコードの件数と時刻の範囲を取る
 * (`GET /api/v0/node-summaries`)。
 */
export async function fetchNodeSummaries(
  request: NodeSummariesRequest,
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<NodeSummariesResponse>> {
  const timeFilter = request.timeFilter;
  if (
    timeFilter !== undefined &&
    timeFilter.from === undefined &&
    timeFilter.to === undefined
  ) {
    return {
      ok: false,
      failure: buildFetchFailure("request_rejected", failureSummary),
    };
  }
  return requestJson({
    path: "/node-summaries",
    searchParams: {
      nodeKind: request.nodeKind,
      eventCategory: request.eventCategory,
      eventAction: request.eventAction,
      eventActionFrom: numberParam(request.eventActionFrom),
      eventActionTo: numberParam(request.eventActionTo),
      case: request.caseId,
      terminal: request.terminal,
      source: nonEmpty(request.sources),
      searchExpression: expressionParam(request.searchExpression),
      timeFrom: timeFilter?.from?.text,
      timeFromPrecision: timeFilter?.from?.precision,
      timeTo: timeFilter?.to?.text,
      timeToPrecision: timeFilter?.to?.precision,
      filterUnit: timeFilter === undefined ? undefined : timeFilter.unit,
      matchCondition: matchConditionParams(request.matchConditions),
    },
    decode: decodeNodeSummariesResponse,
    failureSummary,
    signal: options.signal,
  });
}
