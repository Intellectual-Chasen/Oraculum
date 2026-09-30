import { type Decoder, readObject, requireString } from "../contracts/decoding";
import {
  decodeSourcesResponse,
  type SourcesResponse,
} from "../contracts/sources";
import { type ApiResult, requestJson } from "./httpClient";

const failureSummary = "収集元の一覧の取得";

/**
 * 操作 1 (`GET /api/v0/sources`) を実行する。
 * 要求の項目を持たない。取り込んだ収集元を全件受け取る。
 */
export async function fetchSources(
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<SourcesResponse>> {
  return requestJson({
    path: "/sources",
    decode: decodeSourcesResponse,
    failureSummary,
    signal: options.signal,
  });
}

/** 原文の要求の応答。定義元は `backend/api/raw_texts.go` の `rawTextResponse` である。 */
export type RawTextResponse = { rawTextRef: string; rawText: string };

const decodeRawTextResponse: Decoder<RawTextResponse> = (input, path) => {
  const source = readObject(input, path);
  return {
    rawTextRef: requireString(source, "rawTextRef", path),
    rawText: requireString(source, "rawText", path),
  };
};

/**
 * 原文への参照 (`rawTextRef`) が指すレコードの原文を取得する (`GET /api/v0/raw-texts`)。
 * 取り込めなかったレコードの原文も返る。
 */
export async function fetchRawText(
  rawTextRef: string,
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<RawTextResponse>> {
  return requestJson({
    path: "/raw-texts",
    searchParams: { ref: rawTextRef },
    decode: decodeRawTextResponse,
    failureSummary: "原文の取得",
    signal: options.signal,
  });
}
