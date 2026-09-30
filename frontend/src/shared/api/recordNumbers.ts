import {
  decodeRecordNumbersResponse,
  type RecordNumbersResponse,
} from "../contracts/recordNumbers";
import type { SourceIdentity } from "../contracts/sources";
import { type ApiResult, requestJson } from "./httpClient";

const failureSummary = "レコードの番号の抜けの取得";

/**
 * `GET /api/v0/record-numbers` を実行する。`compared` を与えると、2 つの収集元の
 * 突き合わせも受け取る。
 */
export async function fetchRecordNumbers(
  source: SourceIdentity,
  compared: SourceIdentity | undefined,
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<RecordNumbersResponse>> {
  return requestJson({
    path: "/record-numbers",
    searchParams: {
      sourceId: source.sourceId,
      sourceContentSha256: source.contentSha256,
      comparedSourceId: compared?.sourceId,
      comparedSourceContentSha256: compared?.contentSha256,
    },
    decode: decodeRecordNumbersResponse,
    failureSummary,
    signal: options.signal,
  });
}
