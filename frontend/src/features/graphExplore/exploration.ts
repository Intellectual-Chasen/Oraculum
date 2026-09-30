import {
  maxGraphDepth,
  maxOriginNodes,
  type NodeRef,
} from "@/shared/api/graph";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import type {
  EdgeKind,
  GraphGranularity,
  NodeKind,
} from "@/shared/contracts/graph";
import { type DrawLimit, nodeLimitFor } from "./drawLimit";
import type { SubgraphCriteria } from "./useSubgraph";

/**
 * 検索の結果と別に、分析者が選んだノードから広げてグラフを出す探索。
 *
 * - `lineage`: 起点のプロセスから、親と子のプロセスを上限のホップ数まで辿る。
 * - `neighbours`: 起点のノードと、それに関係するノードを出す。起点は分析者が 1 つずつ
 *   足していき、起点ごとの関係先の和を出す。
 *
 * **検索の条件を載せない。** 探索の途中のノードは、検索の文字列も端末の条件も満たすとは
 * 限らない。条件を載せると、たどる途中で関係が切れる。
 *
 * `keepsSearch` が真の関係先は、検索の結果に足した関係先である。図は検索の結果を関係先と
 * 同じ強さで描く。偽の値は背景を薄く描く。`showOnly` は背景を図から外す。
 */
export type Exploration =
  | { kind: "lineage"; origin: NodeRef }
  | {
      kind: "neighbours";
      origins: readonly NodeRef[];
      keepsSearch?: boolean;
      showOnly?: boolean;
    };

/**
 * 探索の要求を組む。edgeKinds は画面で選んでいる関係の種別であり、関係先を出す要求だけが
 * 辿る種別に使う。
 *
 * **起点にレコードのノードがあるか、「グラフに出す対象」の粒度 (viewGranularity) が
 * レコードであれば、関係先をレコードの粒度で読む。** 対象の粒度はレコードのノードを出さない
 * ため、レコードの起点と関係先のレコードが応答から消える。
 *
 * **分析者が手で選んだ「グラフに出す対象」の種別 (chosenKinds) があれば、関係先をその種別と
 * 起点の種別に限る。** 起点の種別を足さないと、起点が種別の絞り込みで応答から消える。
 */
export function explorationCriteria(
  exploration: Exploration,
  matchConditions: MatchConditionSelection,
  drawLimit: DrawLimit,
  edgeKinds: readonly EdgeKind[],
  viewGranularity: GraphGranularity,
  chosenKinds?: readonly NodeKind[],
): SubgraphCriteria {
  switch (exploration.kind) {
    case "lineage":
      return {
        matchConditions,
        nodeLimit: nodeLimitFor(drawLimit),
        depth: maxGraphDepth,
        granularity: "object",
        nodeKinds: ["process"],
        edgeKinds: ["process_parent_child"],
        nodeIds: [exploration.origin.id],
      };
    case "neighbours":
      return {
        matchConditions,
        nodeLimit: nodeLimitFor(drawLimit),
        depth: 1,
        nodeKinds:
          chosenKinds === undefined || chosenKinds.length === 0
            ? undefined
            : [
                ...new Set([
                  ...chosenKinds,
                  ...exploration.origins.flatMap((origin) =>
                    origin.kind === undefined ? [] : [origin.kind],
                  ),
                ]),
              ],
        granularity:
          viewGranularity === "record" ||
          exploration.origins.some((origin) => origin.kind === "record")
            ? "record"
            : "object",
        edgeKinds: edgeKinds.length > 0 ? edgeKinds : undefined,
        nodeIds: exploration.origins.map((origin) => origin.id),
      };
    default: {
      const unreachable: never = exploration;
      return unreachable;
    }
  }
}

/**
 * 関係先を出す起点に node を足した探索を返す。関係先を出していない探索からは、node だけを
 * 起点にした探索を始める。探索の無い検索の結果から始めた探索は、検索の結果に関係先を足す
 * (`keepsSearch`)。既に起点にあるノードと、起点が上限 (`maxOriginNodes`) に達した後の
 * ノードは足さない。
 */
export function withOrigin(
  exploration: Exploration | undefined,
  node: NodeRef,
): Exploration {
  if (exploration?.kind !== "neighbours") {
    return {
      kind: "neighbours",
      origins: [node],
      keepsSearch: exploration === undefined,
    };
  }
  if (
    exploration.origins.length >= maxOriginNodes ||
    exploration.origins.some((origin) => origin.id === node.id)
  ) {
    return exploration;
  }
  return { ...exploration, origins: [...exploration.origins, node] };
}

/** 関係先を出す起点から id を外した探索を返す。起点が無くなったら探索を終える。 */
export function withoutOrigin(
  exploration: Exploration,
  id: string,
): Exploration | undefined {
  if (exploration.kind !== "neighbours") {
    return exploration;
  }
  const origins = exploration.origins.filter((origin) => origin.id !== id);
  return origins.length === 0 ? undefined : { ...exploration, origins };
}

/** 探索が関係先を出す起点に id を持つかを返す。 */
export function hasOrigin(
  exploration: Exploration | undefined,
  id: string,
): boolean {
  return (
    exploration?.kind === "neighbours" &&
    exploration.origins.some((origin) => origin.id === id)
  );
}
