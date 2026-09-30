/** backend の接続先を設定しなかったときに使う path。開発 server と同じ origin を指す。 */
export const defaultApiBaseUrl = "/api/v0";

/**
 * backend の接続先を返す。
 * 設定は環境変数 `VITE_ORACULUM_API_BASE_URL` が持つ。値の末尾の `/` を除く。
 */
export function readApiBaseUrl(): string {
  const configured = import.meta.env.VITE_ORACULUM_API_BASE_URL;
  if (typeof configured !== "string" || configured.trim() === "") {
    return defaultApiBaseUrl;
  }
  return configured.trim().replace(/\/+$/, "");
}
