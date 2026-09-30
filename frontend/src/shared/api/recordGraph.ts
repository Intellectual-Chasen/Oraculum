import type { RecordLocator } from "../contracts/common";
import {
  decodeRecordGraphResponse,
  type RecordGraphResponse,
} from "../contracts/recordGraph";
import { positionText } from "../lib/recordPosition";
import { type ApiResult, requestJson } from "./httpClient";
import {
  type MatchConditionSelection,
  matchConditionParams,
} from "./matchConditions";

/**
 * 1 レコードを根拠に持つノードとエッジを取る (`GET /api/v0/record-graph`)。関連付けの条件は
 * グラフの探索と同じ選択を渡す。
 */
export function fetchRecordGraph(
  request: {
    recordRef: RecordLocator;
    matchConditions: MatchConditionSelection;
  },
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<RecordGraphResponse>> {
  const { recordRef } = request;
  return requestJson({
    path: "/record-graph",
    searchParams: {
      sourceId: recordRef.sourceId,
      sourceContentSha256: recordRef.sourceContentSha256,
      sequenceNumber: positionText(recordRef.sequenceNumber),
      lineNumber: positionText(recordRef.lineNumber),
      byteOffset: positionText(recordRef.byteOffset),
      matchCondition: matchConditionParams(request.matchConditions),
    },
    decode: decodeRecordGraphResponse,
    failureSummary: "レコードのノードとエッジの取得",
    signal: options.signal,
  });
}
