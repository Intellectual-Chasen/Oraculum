// tsconfig.json の `types` が読み込む vite/client の型へ、本 repo が読む環境変数を足す
// (宣言のマージ)。
interface ImportMetaEnv {
  /** backend の接続先。省略時は `shared/api/config.ts` の既定値を使う。 */
  readonly VITE_ORACULUM_API_BASE_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
