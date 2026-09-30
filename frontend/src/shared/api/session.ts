import { decodeSession, type Session } from "../contracts/session";
import { type ApiResult, requestJson } from "./httpClient";

/** 画面を開いた利用者のセッション (`GET /api/v0/session/me`) を取得する。 */
export async function fetchSession(
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<Session>> {
  return requestJson({
    path: "/session/me",
    decode: decodeSession,
    failureSummary: "ログインの状態の取得",
    signal: options.signal,
  });
}

/** ログインする (`POST /api/v0/session`)。server がセッションの cookie を付ける。 */
export async function login(
  loginName: string,
  password: string,
): Promise<ApiResult<Session>> {
  return requestJson({
    path: "/session",
    method: "POST",
    body: { login: loginName, password },
    decode: decodeSession,
    failureSummary: "ログイン",
  });
}

/** ログアウトする (`DELETE /api/v0/session`)。server がセッションを失効させる。 */
export async function logout(): Promise<ApiResult<undefined>> {
  return requestJson({
    path: "/session",
    method: "DELETE",
    decode: () => undefined,
    failureSummary: "ログアウト",
  });
}
