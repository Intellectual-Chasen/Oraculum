import {
  decodeRecordResponse,
  type RecordResponse,
} from "../contracts/records";
import { isPositionUsable, positionText } from "../lib/recordPosition";
import { buildFetchFailure } from "./apiFailure";
import { type ApiResult, requestJson } from "./httpClient";
import {
  type MatchConditionSelection,
  matchConditionParams,
} from "./matchConditions";

/**
 * レコード 1 件の位置。
 * handler は `sourceId` と `sourceContentSha256` の片方だけを与えた要求を失敗として
 * 返すため、2 つを 1 つの組で受け取る。位置は `sequenceNumber` と `lineNumber` と
 * `byteOffset` の 1 つ以上を持つ。
 */
export type RecordPosition = {
  sourceId: string;
  sourceContentSha256: string;
  sequenceNumber?: number;
  lineNumber?: number;
  byteOffset?: number;
  /** byte 位置で指すレコードの長さ。起点として送るとき `byteOffset` と対で送る。 */
  byteLength?: number;
};

/** 操作 3 と操作 7 の要求の項目。 */
export type RecordRequest = {
  record: RecordPosition;
  /**
   * 起点のレコードと、起点から開いたレコードへの経路を組む関連付けの条件。
   * 出さない場合は応答が `derivationTrail` を持たない。backend は起点を持つ要求に 1 つ以上の
   * 条件を求め、起点の無い要求に条件を与えると退ける。
   */
  origin?: { record: RecordPosition; matchConditions: MatchConditionSelection };
};

const failureSummary = "レコードの取得";

/**
 * 操作 3 と操作 7 (`GET /api/v0/records`) を実行する。
 * handler が失敗を返す要求 (位置を 1 つも与えない、起点の位置を 1 つも与えない) を
 * 送らずに失敗として返す。
 */
export async function fetchRecord(
  request: RecordRequest,
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<RecordResponse>> {
  const origin = request.origin?.record;
  if (
    !isPositionUsable(request.record) ||
    (origin !== undefined && !isPositionUsable(origin))
  ) {
    return {
      ok: false,
      failure: buildFetchFailure("request_rejected", failureSummary),
    };
  }
  return requestJson({
    path: "/records",
    searchParams: {
      sourceId: request.record.sourceId,
      sourceContentSha256: request.record.sourceContentSha256,
      sequenceNumber: positionText(request.record.sequenceNumber),
      lineNumber: positionText(request.record.lineNumber),
      byteOffset: positionText(request.record.byteOffset),
      originSourceId: origin?.sourceId,
      originSourceContentSha256: origin?.sourceContentSha256,
      originSequenceNumber: positionText(origin?.sequenceNumber),
      originLineNumber: positionText(origin?.lineNumber),
      originByteOffset: positionText(origin?.byteOffset),
      matchCondition:
        request.origin === undefined
          ? undefined
          : matchConditionParams(request.origin.matchConditions),
    },
    decode: decodeRecordResponse,
    failureSummary,
    signal: options.signal,
  });
}
