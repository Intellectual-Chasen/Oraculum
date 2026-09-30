import {
  decodeSourceFileListing,
  decodeSourceUploadResult,
  type SourceFileListing,
  type SourceUploadResult,
} from "../contracts/sourceFiles";
import { type ApiResult, requestJson } from "./httpClient";

const failureSummary = "基準の directory の一覧の取得";

/** PC の資料をサーバーへ保存し、形式の判定結果を返す。 */
export function uploadSourceFile(
  file: File,
  path: string,
  signal?: AbortSignal,
  upload?: string,
): Promise<ApiResult<SourceUploadResult>> {
  return requestJson({
    path: "/stages/source-files",
    method: "POST",
    searchParams: { path, upload },
    rawBody: file,
    decode: decodeSourceUploadResult,
    failureSummary: "ログのアップロード",
    signal,
  });
}

/**
 * 基準の directory の下の directory `path` の項目を、file ごとの入力形式の候補とともに取得する
 * (`GET /api/v0/stages/source-files`)。`recursive` が真のときは下の directory の file も取得する。
 */
export async function fetchSourceFiles(
  path: string,
  options: { recursive?: boolean; signal?: AbortSignal } = {},
): Promise<ApiResult<SourceFileListing>> {
  return requestJson({
    path: "/stages/source-files",
    searchParams: {
      path,
      recursive: options.recursive === true ? "true" : undefined,
    },
    decode: decodeSourceFileListing,
    failureSummary,
    signal: options.signal,
  });
}
