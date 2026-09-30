/**
 * Squid の要求処理の結果の文字列 (`%Ss` または `%Ss:%Sh`) が、cache からの応答を示すかを返す。
 *
 * `:` より前の結果の符号を `_` で区切り、区切った語に大文字の `HIT` があるときだけ真にする
 * (`TCP_HIT`、`TCP_MEM_HIT`、`TCP_IMS_HIT` など)。語の一部に `HIT` を含む値と小文字の値は偽である。
 */
export function isSquidCacheHit(status: string): boolean {
  const [code] = status.split(":");
  return code.split("_").includes("HIT");
}
