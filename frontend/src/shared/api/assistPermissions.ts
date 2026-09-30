import {
  type AssistPermissionAction,
  type AssistPermissionsResponse,
  type AssistProvider,
  type AssistProviderPermission,
  decodeAssistPermissionsResponse,
  decodeAssistProviderPermission,
} from "../contracts/assistPermissions";
import { buildFetchFailure } from "./apiFailure";
import { type ApiResult, requestJson } from "./httpClient";

const listFailureSummary = "AI 支援の送信の許可の取得";
const recordFailureSummary = "AI 支援の送信の許可の記録";

/** 送信の許可の新しい改訂 1 つの入力。改訂の番号と時刻は backend が決める。 */
export type AssistPermissionDraft = {
  provider: AssistProvider;
  action: AssistPermissionAction;
  /** ログインしているときは省き、server がセッションの利用者を記録する。 */
  analyst?: string;
};

/** 提供者ごとの送信の許可 (`GET /api/v0/assist-permissions`) を取得する。 */
export async function fetchAssistPermissions(
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<AssistPermissionsResponse>> {
  return requestJson({
    path: "/assist-permissions",
    decode: decodeAssistPermissionsResponse,
    failureSummary: listFailureSummary,
    signal: options.signal,
  });
}

/** 送信の許可の改訂を 1 つ記録する (`POST /api/v0/assist-permissions`)。 */
export async function recordAssistPermission(
  draft: AssistPermissionDraft,
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<AssistProviderPermission>> {
  // handler が退ける入力を送る前に分ける。
  if (draft.analyst?.trim() === "") {
    return {
      ok: false,
      failure: buildFetchFailure("request_rejected", "分析者の名前なし"),
    };
  }
  return requestJson({
    path: "/assist-permissions",
    method: "POST",
    body: draft,
    decode: decodeAssistProviderPermission,
    failureSummary: recordFailureSummary,
    signal: options.signal,
  });
}
