// cosmos.gl の GPU の力学で図の配置を決める設定と、cosmos.gl へ渡す値を作る関数。
import type { GraphConfig } from "@cosmos.gl/graph";
import Graph from "graphology";
import louvain from "graphology-communities-louvain";

/**
 * 配置の入力。応答から識別子と端点だけを抜き出した形にする。
 *
 * **並びを応答の並びに保つ。** 初期の座標と cosmos.gl の点の番号は並びで決まる。
 */
export type LayoutInput = {
  /** ノードの識別子。 */
  ids: string[];
  /** エッジの識別子と端点。 */
  links: { id: string; source: string; target: string }[];
};

/**
 * 黄金角。応答の並びの隣どうしを図の上で離し、円板を等間隔で埋める。
 * 値は `π * (3 - √5)` である。
 */
const goldenAngle = Math.PI * (3 - Math.sqrt(5));

/**
 * 並びの位置から、半径 1 の円板の中の初期の座標を決める。
 * 同じ入力からは同じ座標が出る。乱数と時刻を材料にしない。
 */
function placeOnDisc(index: number, total: number) {
  const radius = total === 0 ? 0 : Math.sqrt((index + 0.5) / total);
  const angle = index * goldenAngle;
  return { x: radius * Math.cos(angle), y: radius * Math.sin(angle) };
}

/** cosmos の座標の空間の一辺。 */
const spaceSize = 4096;

/** 同じ入力から同じ配置を出すための乱数の種。 */
const randomSeed = 703;

/**
 * 分析者が画面から変える cosmos.gl の設定。名前と意味は cosmos.gl の `GraphConfig` と同じである。
 * `simulationCluster` が 0 より大きいときだけ、Louvain 法で分けた番号を `setPointClusters` で渡す。
 */
export type CosmosLayoutSettings = Required<
  Pick<
    GraphConfig,
    "simulationGravity" | "simulationLinkDistance" | "simulationCluster"
  >
>;

/** 設定が取る値の範囲と刻み。画面の入力欄が使う。 */
export const layoutSettingRanges = {
  simulationGravity: { min: 0, max: 10, step: 0.25 },
  simulationLinkDistance: { min: 1, max: 100, step: 1 },
  simulationCluster: { min: 0, max: 1, step: 0.05 },
} as const satisfies Record<
  keyof CosmosLayoutSettings,
  { min: number; max: number; step: number }
>;

// 既知の制限: simulationGravity を 5、simulationRepulsion を 40、simulationLinkDistance を 10、
// simulationCluster を 0 にする,
// ハブに多数の葉が付く数百点の図で、配置の終了後の画面の座標から、点の円が重なる組と
// エッジの交差を数えた。反発を強めて塊を広げ、重力を強めて離れた成分を寄せると、図の範囲が
// 詰まって重なりが 1/10 以下になり、交差も減った。値は機能の issue に記録した,
// 数千点を超える図で塊が画面の外へ広がると報告されたときに見直す
export const defaultLayoutSettings: CosmosLayoutSettings = {
  simulationGravity: 5,
  simulationLinkDistance: 10,
  simulationCluster: 0,
};

/** 初期配置の力。円板から中央への急な移動を抑える。 */
export const initialSimulationAlpha = 0.1;

/**
 * 通常の点と選んでいる点の大きさ (px)。cosmos.gl は `setPointSizes` の大きさから衝突の半径も
 * 求めるため、描画と配置で同じ値を使う。
 */
export const pointSize = 10;
export const selectedPointSize = 18;

/** simulation の設定。配置の設定 (`CosmosLayoutSettings`) の値は描画の component が上書きする。 */
export const cosmosSimulation: GraphConfig = {
  spaceSize,
  randomSeed,
  pointDefaultSize: pointSize,
  simulationDecay: 1000,
  ...defaultLayoutSettings,
  simulationRepulsion: 40,
  simulationLinkSpring: 1,
  simulationFriction: 0.85,
  simulationCollision: 1,
  simulationCollisionPadding: 4,
  transitionDuration: 0,
  fitViewOnInit: true,
  fitViewDelay: 0,
};

// 既知の制限: 点を既定の大きさで描く拡大率の上限を 2.5、倍率の上限を 3 にする,
// 利用者が配置した実資料でノードの数を変え、図の範囲に合わせた後の拡大率を測った。ノードが多い図は
// 2.5 未満になり、倍率 1 の今の大きさで描いた,
// 分析者が小さな図の点を大きすぎる・小さすぎると報告したときに見直す
const referenceZoom = 2.5;
const maxSizeScale = 3;

/**
 * 図の範囲に合わせた後の拡大率から、点の大きさとエッジの太さの倍率を決める。
 * ノードが少ない図は大きく拡大され、点が画面の px で固定のままだと間隔だけが広がる。
 */
export function sizeScaleForZoom(zoom: number): number {
  return Math.min(maxSizeScale, Math.max(1, Math.sqrt(zoom / referenceZoom)));
}

/**
 * 初期の座標を、cosmos の空間の中央に置く。
 *
 * focus を与えないときは、エッジでつながる点の組 (連結成分) ごとに円板を分ける。大きい成分を
 * 中央に置き、残りの成分の中心を黄金角の螺旋に沿って外へ並べる。成分の円板の面積は点の数に
 * 比例させ、成分の中は並びで円板を埋める。別の成分を混ぜた状態から始めると、配置の後も成分が
 * 重なる。
 *
 * focus を与えたときは、focus の点だけで中央の円板を埋め、残りの点をその外側の輪に置く。
 * 部分グラフを中央に集め、それ以外を外側に出すためである。
 */
export function initialPositions(
  input: LayoutInput,
  focus?: ReadonlySet<string>,
): Float32Array {
  const positions = new Float32Array(input.ids.length * 2);
  const radius = spaceSize / 8;
  const place = (index: number, x: number, y: number) => {
    positions[index * 2] = spaceSize / 2 + x;
    positions[index * 2 + 1] = spaceSize / 2 + y;
  };
  if (focus === undefined || focus.size === 0) {
    // 全体の円板の面積を成分に割り当てる単位。点 1 つの円板の半径である。
    const unit = radius / Math.sqrt(Math.max(1, input.ids.length));
    let placed = 0;
    connectedComponents(input).forEach((members, order) => {
      const distance =
        order === 0 ? 0 : unit * Math.sqrt(2 * (placed + members.length / 2));
      const cx = distance * Math.cos(order * goldenAngle);
      const cy = distance * Math.sin(order * goldenAngle);
      const discRadius = unit * Math.sqrt(members.length);
      members.forEach((index, position) => {
        const point = placeOnDisc(position, members.length);
        place(index, cx + point.x * discRadius, cy + point.y * discRadius);
      });
      placed += members.length;
    });
    return positions;
  }
  const inner = input.ids.flatMap((id, index) =>
    focus.has(id) ? [index] : [],
  );
  const outer = input.ids.flatMap((id, index) =>
    focus.has(id) ? [] : [index],
  );
  inner.forEach((index, order) => {
    const point = placeOnDisc(order, inner.length);
    place(index, point.x * radius, point.y * radius);
  });
  outer.forEach((index, order) => {
    const angle = (2 * Math.PI * order) / outer.length;
    place(index, Math.cos(angle) * radius * 3, Math.sin(angle) * radius * 3);
  });
  return positions;
}

/** エッジでつながる点の組ごとに、点の並びの位置を返す。組は大きい順、同じ大きさは先に現れた順。 */
function connectedComponents(input: LayoutInput): number[][] {
  const index = new Map(input.ids.map((id, i) => [id, i]));
  const parent = input.ids.map((_, i) => i);
  const root = (i: number): number => {
    let r = i;
    while (parent[r] !== r) r = parent[r] as number;
    while (parent[i] !== r) {
      const next = parent[i] as number;
      parent[i] = r;
      i = next;
    }
    return r;
  };
  for (const link of input.links) {
    const s = index.get(link.source);
    const t = index.get(link.target);
    if (s !== undefined && t !== undefined) parent[root(s)] = root(t);
  }
  const groups = new Map<number, number[]>();
  input.ids.forEach((_, i) => {
    const r = root(i);
    const group = groups.get(r);
    if (group === undefined) groups.set(r, [i]);
    else group.push(i);
  });
  return [...groups.values()].sort((a, b) => b.length - a.length);
}

/**
 * エッジを、端点の並びの位置の対にする。返す対の並びはエッジの並びと同じである。
 *
 * **点の識別子の重複と、端点が図に無いエッジで例外を投げる。** 重複を通すと点とエッジの
 * 対応がずれた図を描き、関係を通知せずに消すと分析者は欠けた図を完全な図と読む。
 * 例外は図の error boundary が失敗として出す。
 */
export function linkPairs(input: LayoutInput): Float32Array {
  const index = new Map(input.ids.map((id, i) => [id, i]));
  if (index.size !== input.ids.length) {
    throw new Error("the graph carries a point identifier more than once");
  }
  const pairs = new Float32Array(input.links.length * 2);
  input.links.forEach((link, i) => {
    const s = index.get(link.source);
    const t = index.get(link.target);
    if (s === undefined || t === undefined) {
      throw new Error(
        `the edge ${link.id} refers to an endpoint outside the graph`,
      );
    }
    pairs[i * 2] = s;
    pairs[i * 2 + 1] = t;
  });
  return pairs;
}

/**
 * 種から 0 以上 1 未満の数を順に返す線形合同法の乱数を作る。法は 2^31 である。
 * 掛け算を `Math.imul` の 32 bit で行い、安全な整数の範囲を超えて下位の桁が丸まるのを避ける。
 */
export function seededRandom(seed: number): () => number {
  let state = seed;
  return () => {
    state = (Math.imul(state, 1103515245) + 12345) & 0x7fffffff;
    return state / 2147483648;
  };
}

/**
 * `setPointClusters` に渡す点ごとの cluster の番号を、エッジのつながりから Louvain 法で求める。
 * 乱数の種を固定し、同じ入力から同じ番号を出す。
 */
export function pointClusters(input: LayoutInput): number[] {
  const graph = new Graph({ type: "undirected" });
  for (const id of input.ids) graph.addNode(id);
  for (const link of input.links) {
    if (
      link.source !== link.target &&
      graph.hasNode(link.source) &&
      graph.hasNode(link.target) &&
      !graph.hasEdge(link.source, link.target)
    ) {
      graph.addEdge(link.source, link.target);
    }
  }
  const rng = seededRandom(randomSeed);
  const clusters = louvain(graph, { rng });
  return input.ids.map((id) => clusters[id] ?? 0);
}

/** `#rrggbb` を cosmos の 0 から 1 の RGBA にする。 */
export function rgba(hex: string, alpha = 1): [number, number, number, number] {
  const value = Number.parseInt(hex.replace("#", "").slice(0, 6), 16);
  if (Number.isNaN(value)) return [0.5, 0.5, 0.5, alpha];
  return [
    ((value >> 16) & 255) / 255,
    ((value >> 8) & 255) / 255,
    (value & 255) / 255,
    alpha,
  ];
}
