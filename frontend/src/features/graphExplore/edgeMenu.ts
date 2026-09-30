import type { GraphEdge, SubgraphNode } from "@/shared/contracts/graph";
import { toVisibleRawText } from "@/shared/lib/rawText";
import type { SearchTermKind } from "@/shared/lib/searchTerms";
import type { ContextMenuContent } from "@/shared/ui/ContextMenu";
import { type MenuEntry, type MenuItem, menuSeparator } from "@/shared/ui/Menu";
import { edgeKindLabels } from "./labels";
import { menuLabelName } from "./nodeMenu";
import { readNodeLabel } from "./subgraph";

/** エッジのコンテキストメニューが呼ぶ操作。状態を持つ GraphExplore が渡す。 */
export type EdgeMenuActions = {
  /** 関係 1 本を選び、関係の詳細を出す。 */
  onSelectEdge: (edgeId: string) => void;
  /** 端点のノードを選び、ノードの詳細を出す。 */
  onOpenNodeDetail: (nodeId: string) => void;
  /** 文字列を検索の条件に足す。 */
  onAddTerm: (kind: SearchTermKind, text: string) => void;
  /** エッジのブックマークを付け外しする。出ない場合は項目を出さない。 */
  bookmark?: EdgeBookmark | undefined;
};

/**
 * エッジのブックマーク。メニューを開いたときに付けてあるかを読む。端点の表示名は原資料の
 * 文字列で、応答が端点を含まないときは識別子である。
 */
export type EdgeBookmark = {
  isMarked: (edgeId: string) => boolean;
  onToggle: (edge: GraphEdge, sourceLabel: string, targetLabel: string) => void;
};

/**
 * 端点のノードを読み上げと見出しに出す名前。表示名の文字列を可視の符号にして出し、表示名を持たない
 * ノードは持たない理由を出す。応答が端点を含まないときは識別子を出す。
 */
export function endpointName(
  node: SubgraphNode | undefined,
  nodeId: string,
): string {
  return node === undefined
    ? toVisibleRawText(nodeId)
    : menuLabelName(node.label);
}

/** エッジ 1 本を読み上げで指す名前。種類と始点と終点を挙げる。 */
export function edgeName(
  edge: GraphEdge,
  nodeOf: (nodeId: string) => SubgraphNode | undefined,
): string {
  return `${edgeKindLabels[edge.kind]} ${endpointName(nodeOf(edge.sourceNodeId), edge.sourceNodeId)} → ${endpointName(nodeOf(edge.targetNodeId), edge.targetNodeId)}`;
}

/** エッジ 1 本の行の操作のメニューの読み上げの名前。 */
export function edgeMenuName(
  edge: GraphEdge,
  nodeOf: (nodeId: string) => SubgraphNode | undefined,
): string {
  return `${edgeName(edge, nodeOf)} の操作`;
}

function bookmarkItems(
  edge: GraphEdge,
  endpoints: { nodeId: string; node: SubgraphNode | undefined }[],
  bookmark: EdgeBookmark | undefined,
): MenuItem[] {
  if (bookmark === undefined) return [];
  const [source, target] = endpoints.map(
    ({ nodeId, node }) =>
      node?.label.rawText ?? node?.label.normalized ?? nodeId,
  );
  return [
    item(
      "bookmark",
      bookmark.isMarked(edge.id)
        ? "ブックマークから削除"
        : "ブックマークに追加",
      () =>
        bookmark.onToggle(
          edge,
          source ?? edge.sourceNodeId,
          target ?? edge.targetNodeId,
        ),
      false,
    ),
  ];
}

function item(
  key: string,
  label: string,
  onSelect: () => void,
  disabled: boolean,
): MenuItem {
  return { kind: "item", key, label, onSelect, disabled };
}

/**
 * エッジ 1 本のコンテキストメニューの項目を組む。図と一覧が同じ項目を出す。
 *
 * **応答が含まない端点は、その端点を使う項目を残して使えなくする。** 選んだノードが応答に
 * 無いと、選択は次の描画で外れる。表示名を持たない端点では、文字列に足す項目を使えなくする。
 * 詳細を出している関係では、詳細を出す項目を使えなくする。
 */
export function edgeMenuContent(
  edge: GraphEdge,
  nodeOf: (nodeId: string) => SubgraphNode | undefined,
  actions: EdgeMenuActions,
  selectedEdgeId: string | undefined,
  copy: (text: string, what: string) => void,
): ContextMenuContent {
  const endpoints = [
    { key: "source", name: "始点", nodeId: edge.sourceNodeId },
    { key: "target", name: "終点", nodeId: edge.targetNodeId },
  ].map((endpoint) => {
    const node = nodeOf(endpoint.nodeId);
    const label = node === undefined ? undefined : readNodeLabel(node.label);
    return {
      ...endpoint,
      node,
      text:
        label !== undefined && "text" in label.value
          ? label.value.text
          : undefined,
    };
  });
  const entries: MenuEntry[] = [
    item(
      "edge-detail",
      "エッジの詳細を表示",
      () => actions.onSelectEdge(edge.id),
      edge.id === selectedEdgeId,
    ),
    ...endpoints.map((endpoint) =>
      item(
        `${endpoint.key}-detail`,
        `${endpoint.name}のノードの詳細を開く`,
        () => actions.onOpenNodeDetail(endpoint.nodeId),
        endpoint.node === undefined,
      ),
    ),
    ...bookmarkItems(edge, endpoints, actions.bookmark),
    menuSeparator("terms"),
    ...endpoints.map((endpoint) =>
      item(
        `${endpoint.key}-term`,
        `${endpoint.name}の表示名を含む条件に追加`,
        () => {
          if (endpoint.text !== undefined) {
            actions.onAddTerm("contains", endpoint.text);
          }
        },
        endpoint.text === undefined,
      ),
    ),
    menuSeparator("copy"),
    item(
      "copy-id",
      "エッジの識別子をコピー",
      () => copy(edge.id, "エッジの識別子"),
      false,
    ),
  ];
  return { label: edgeMenuName(edge, nodeOf), entries };
}
