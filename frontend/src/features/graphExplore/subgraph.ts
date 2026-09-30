import type { RawAndNormalized, ValueState } from "@/shared/contracts/common";
import type {
  EdgeKind,
  GraphResponse,
  NodeKind,
  NodeSelection,
  RelationState,
  SubgraphNode,
} from "@/shared/contracts/graph";
import { formatCount } from "@/shared/lib/format";
import {
  describeRawTextAbsence,
  type FieldValue,
} from "@/shared/lib/recordField";
import { nodeSelectionLabels } from "./labels";

/**
 * ノードの表示名 1 つ。
 * 原資料の文字列と、別の欄から導いた値を、読む側が見分けられる形で持つ。
 */
export type NodeLabel = {
  /** 欄に出す値。文字列も導いた値も無いノードでは、無い理由を持つ。 */
  value: FieldValue;
  valueState: ValueState;
  /** `valueState` が `derived` のとき、値の導き方を持つ。 */
  derivation?: string;
};

/** 図に置くノード 1 つ。 */
export type SubgraphPoint = {
  id: string;
  kind: NodeKind;
  selection: NodeSelection;
  /** 原資料の文字列のまま持つ。可視の符号への置き換えは描画の処理が行う。 */
  label: NodeLabel;
  /** 同じ表示名のアカウントを図で見分けるための識別鍵。 */
  accountIdentity?: string;
  /** 真の点は、別の対象のレコードが参照した値からだけ分かったノードである。破線の輪で描く。 */
  referenced?: boolean;
};

function accountIdentityOf(node: SubgraphNode): string | undefined {
  if (node.kind !== "account") return undefined;
  const value = (semantic: string) =>
    node.identity.find((part) => part.semantic === semantic)?.value;
  switch (node.keyForm) {
    case "account_sid":
      return `SID: ${value("account.sid") ?? node.identity[0]?.value ?? ""}`;
    case "account_domain_name":
      return `${value("account.domain") ?? ""}\\${value("account.name") ?? ""}`;
    default:
      return node.identity.map((part) => part.value).join(" / ");
  }
}

/** 図に引くエッジ 1 本。 */
export type SubgraphLink = {
  id: string;
  kind: EdgeKind;
  state: RelationState;
  sourceNodeId: string;
  targetNodeId: string;
  /** 打ち切りの前に数えた根拠のレコードの総数。 */
  evidenceCount: number;
};

/** 図 1 枚分。 */
export type SubgraphDrawing = {
  points: SubgraphPoint[];
  links: SubgraphLink[];
};

/**
 * ノードの表示名を読む。
 * 原資料の文字列を先に読み、文字列を持たないノードでは導いた値を読む。
 * どちらも無いノードでは、値が無い理由を持つ。
 *
 * **原資料の文字列と導いた値を同じ形にまとめない。** 読む側が `valueState` で見分ける。
 */
export function readNodeLabel(label: RawAndNormalized): NodeLabel {
  if (label.rawText !== undefined) {
    return { value: { text: label.rawText }, valueState: label.valueState };
  }
  if (label.normalized !== undefined) {
    return {
      value: { text: label.normalized },
      valueState: label.valueState,
      derivation: label.derivation,
    };
  }
  return {
    value: { absence: describeRawTextAbsence(label.valueState) },
    valueState: label.valueState,
  };
}

/**
 * 部分グラフの応答を図の入力へ写す。
 * ノードとエッジを作る処理も、同一性を決める処理も行わない。応答が含む集合を
 * そのまま並べる。座標は描画の component が cosmos.gl で決める。
 */
export function buildSubgraphDrawing(response: GraphResponse): SubgraphDrawing {
  return {
    points: response.nodes.map((node) => ({
      id: node.id,
      kind: node.kind,
      selection: node.selection,
      label: readNodeLabel(node.label),
      ...(node.kind === "account"
        ? { accountIdentity: accountIdentityOf(node) }
        : {}),
      ...(node.observation === "referenced" ? { referenced: true } : {}),
    })),
    links: response.edges.map((edge) => ({
      id: edge.id,
      kind: edge.kind,
      state: edge.state,
      sourceNodeId: edge.sourceNodeId,
      targetNodeId: edge.targetNodeId,
      evidenceCount: edge.evidenceCount,
    })),
  };
}

/**
 * 部分グラフの図に、背景のグラフの点とエッジを足す。部分グラフに同じ識別子があるときは
 * 部分グラフの値を使う。端点が図に無いエッジは足さない。
 */
export function withBackdrop(
  focus: SubgraphDrawing,
  backdrop: SubgraphDrawing,
): SubgraphDrawing {
  const points = [...focus.points];
  const present = new Set(points.map((point) => point.id));
  for (const point of backdrop.points) {
    if (!present.has(point.id)) {
      present.add(point.id);
      points.push(point);
    }
  }
  const links = [...focus.links];
  const linkIds = new Set(links.map((link) => link.id));
  for (const link of backdrop.links) {
    if (
      !linkIds.has(link.id) &&
      present.has(link.sourceNodeId) &&
      present.has(link.targetNodeId)
    ) {
      linkIds.add(link.id);
      links.push(link);
    }
  }
  return { points, links };
}

/**
 * 部分グラフの件数を「名前: 値」の組で返す。一致ノードの件数と種類ごとの件数は ResultCount が
 * 出すため、ここに入れない。
 *
 * **絞り込んだ結果を全件出す。** 応答は上限も続きを取る位置も持たない
 * (`backend/api/graph.go` の `graphResponse`)。打ち切りの組を置かない。
 *
 * **ノードを指す語を `nodeSelectionLabels` に揃える。** ノードの一覧の列が同じ語を出す。
 *
 * **上限を超えた応答は、描画するノードの件数と上限を返す。** 図とエッジの一覧が空であることを、
 * エッジが無いと読ませない。
 */
export function subgraphSummaryPairs(
  response: GraphResponse,
): { name: string; value: string }[] {
  if (response.nodeLimitExceeded) {
    return [
      {
        name: "描画するノード",
        value: formatCount(response.subgraphNodeCount),
      },
      { name: "描画の上限", value: formatCount(response.nodeLimit) },
    ];
  }
  const shown = response.nodes.filter(
    (node) => node.selection === "matched",
  ).length;
  return [
    {
      name: `グラフの${nodeSelectionLabels.matched}`,
      value: formatCount(shown),
    },
    {
      name: nodeSelectionLabels.edge_endpoint,
      value: formatCount(response.nodes.length - shown),
    },
    { name: "エッジ", value: formatCount(response.edgeCount) },
  ];
}

/** 開いたレコードに対応するノードとエッジのうち、応答にある数と、対応する全体の数。 */
export type HighlightCounts = {
  shownNodes: number;
  nodes: number;
  shownEdges: number;
  edges: number;
};

/**
 * 開いたレコードに対応するノードとエッジのうち、応答にある数を数える。応答に無いものは、条件や
 * 描画の上限で図の外にある。上限を超えた応答はエッジを持たず、ノードは一致ノードの一覧にある。
 */
export function highlightCountsOf(
  response: GraphResponse,
  highlight: {
    nodeIds: ReadonlySet<string>;
    edgeIds: ReadonlySet<string>;
  },
): HighlightCounts {
  return {
    shownNodes: response.nodes.filter((node) => highlight.nodeIds.has(node.id))
      .length,
    nodes: highlight.nodeIds.size,
    shownEdges: response.edges.filter((edge) => highlight.edgeIds.has(edge.id))
      .length,
    edges: highlight.edgeIds.size,
  };
}

/**
 * 親子の連鎖をたどった応答で、親との関係を持たないプロセスのノードを返す。
 *
 * **上限を超えた応答では何も返さない。** エッジを持たないため、親との関係の有無を決められない。
 */
export function lineageRootsOf(response: GraphResponse): SubgraphNode[] {
  if (response.nodeLimitExceeded) {
    return [];
  }
  const children = new Set(
    response.edges
      .filter((edge) => edge.kind === "process_parent_child")
      .map((edge) => edge.targetNodeId),
  );
  return response.nodes.filter(
    (node) => node.kind === "process" && !children.has(node.id),
  );
}

/**
 * 値ごとの件数の要約の組を返す。数えるフィールドを与えていない応答では返さない。
 *
 * 件数は一致ノードの全件から数えた、その値を記録したレコードの件数であり、図と一覧に出した
 * ノードの中だけを数えた値ではない (`backend/pipeline/graph_query.go` の `valueCounts`)。
 *
 * **値の不在を値の 1 つとして数えない。** 収集元が値の不在を文字列で書くフィールドでは、その
 * 記録を数から外す。
 *
 * **値ごとの件数の和と、絞り込みに一致したレコードの件数を比べない。** 値の不在で数から外す
 * 記録がある一方、1 レコードが同じフィールドへ 2 つの異なる値を持つと、そのレコードを 2 つの
 * 値が数える。Mark II の 1 レコードは端末の IPv4 と IPv6 を 1 つずつ持つ
 * (`backend/pipeline/graph_query.go` の `valueCounts` の既知の制限)。
 */
export function valueCountsSummaryPairs(
  response: GraphResponse,
): { name: string; value: string }[] | undefined {
  const distinct = response.distinctValueCount;
  if (response.countBy === undefined || distinct === undefined) {
    return undefined;
  }
  return [{ name: "値の種類", value: formatCount(distinct) }];
}
