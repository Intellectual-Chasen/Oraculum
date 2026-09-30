/**
 * 図に描くノードの数の上限。`all` は上限を置かない。
 *
 * **上限を超えた結果は図に描かない。** 描いた図には、合った対象と、その関係の相手が
 * すべて載る。上限を超えたときは、合ったノードの一覧と件数を出す。
 */
export type DrawLimit = number | "all";

/** 分析者が選べる数の上限のうち、最も大きい値。 */
export const maxNodeLimit = 2000;

// 既知の制限: 図に描くノードの数の既定は上限を置かない, GPU で描く環境で cosmos.gl の図を
// 測った。1,638 件の図で上限 2,000 件と上限なしの差は無く、ノード 17,516 件とエッジ
// 66,603 本の図も上限を外してから 2.8 秒で操作できた。GPU で描けない環境では図を
// 作らない (webglProbe.ts), 描画の時間が数秒を超える資料が出たとき、または図の描き方を
// 替えたときに見直す
export const defaultDrawLimit: DrawLimit = "all";

/** 分析者が選べる上限。 */
export const drawLimitChoices: readonly DrawLimit[] = [
  200,
  500,
  1000,
  maxNodeLimit,
  "all",
];

/** 上限を要求の `nodeLimit` へ直す。`all` は項目を載せない。 */
export function nodeLimitFor(limit: DrawLimit): number | undefined {
  return limit === "all" ? undefined : limit;
}

/**
 * 応答が図に描くノードの数 `count` を描ける、最も小さい上限を返す。どの数の上限にも
 * 収まらないときは `all` を返す。
 */
export function smallestLimitFor(count: number): DrawLimit {
  return (
    drawLimitChoices.find((limit) => limit !== "all" && count <= limit) ?? "all"
  );
}
