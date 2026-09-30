import type { NodeRef } from "@/shared/api/graph";
import type { RawAndNormalized } from "@/shared/contracts/common";
import type { SubgraphNode } from "@/shared/contracts/graph";
import { toVisibleRawText } from "@/shared/lib/rawText";
import type { SearchTermKind } from "@/shared/lib/searchTerms";
import type { ContextMenuContent } from "@/shared/ui/ContextMenu";
import { type MenuEntry, type MenuItem, menuSeparator } from "@/shared/ui/Menu";
import { nodeKindLabels } from "./labels";
import { nodeLabelName } from "./NodeLabelView";
import { readNodeLabel } from "./subgraph";

/** ノードのコンテキストメニューが呼ぶ操作。状態を持つ GraphExplore が渡す。 */
export type NodeMenuActions = {
  /** ノードを選び、詳細のビューに出す。 */
  onOpenDetail: (nodeId: string) => void;
  /** 今のグラフにノードの関係先を足す。 */
  onAddNeighbours: (nodeId: string) => void;
  /** ノードとその関係先だけをグラフに出す。 */
  onShowNeighbours: (node: NodeRef) => void;
  /** プロセスのノードから親子の連鎖をたどる。 */
  onTraceLineage: (node: NodeRef) => void;
  /** 端末のノードで根拠のレコードを絞る。 */
  onNarrowToTerminal: (node: NodeRef) => void;
  /** 文字列を検索の条件に足す。 */
  onAddTerm: (kind: SearchTermKind, text: string) => void;
  /** ノードのブックマークを付け外しする。出ない場合は項目を出さない。 */
  bookmark?: NodeBookmark | undefined;
};

/** ノードのブックマーク。メニューを開いたときに付けてあるかを読む。 */
export type NodeBookmark = {
  isMarked: (nodeId: string) => boolean;
  onToggle: (node: NodeRef) => void;
};

function item(
  key: string,
  label: string,
  onSelect: () => void,
  disabled = false,
): MenuItem {
  return { kind: "item", key, label, onSelect, disabled };
}

/**
 * 表示名を、メニューと「…」の button の名前に入れる文字列にする。文字列を可視の符号にして出し、
 * 表示名を持たないときは持たない理由を出す。
 */
export function menuLabelName(label: RawAndNormalized): string {
  const read = readNodeLabel(label);
  return "text" in read.value
    ? toVisibleRawText(read.value.text)
    : read.value.absence;
}

/**
 * ノード 1 件のコンテキストメニューの項目を組む。図と一覧が同じ項目を出す。
 *
 * **ノードの詳細が出す操作と同じ条件で項目を出す。** 親子の連鎖はプロセスのノードだけが持つ。
 * 端末で絞る操作は、参照だけの端末に出さない。参照だけの端末には置いたレコードが無く、絞ると
 * 必ず 0 件になる。
 *
 * **表示名を持たないノードでは、表示名を使う項目を残して使えなくする。**
 */
export function nodeMenuContent(
  node: SubgraphNode,
  actions: NodeMenuActions,
  copy: (text: string, what: string) => void,
): ContextMenuContent {
  const label = readNodeLabel(node.label);
  const text = "text" in label.value ? label.value.text : undefined;
  const reference: NodeRef = {
    id: node.id,
    label: node.label.rawText ?? node.label.normalized ?? node.id,
    kind: node.kind,
  };
  const entries: MenuEntry[] = [
    item("detail", "詳細を開く", () => actions.onOpenDetail(node.id)),
    item("add-neighbours", "隣接ノードを追加", () =>
      actions.onAddNeighbours(node.id),
    ),
    item("show-neighbours", "隣接ノードだけを表示", () =>
      actions.onShowNeighbours(reference),
    ),
  ];
  if (node.kind === "process") {
    entries.push(
      item("lineage", "プロセスの親子関係を表示", () =>
        actions.onTraceLineage(reference),
      ),
    );
  }
  if (node.kind === "terminal" && node.observation !== "referenced") {
    entries.push(
      item("terminal", "この端末でフィルタ", () =>
        actions.onNarrowToTerminal(reference),
      ),
    );
  }
  const { bookmark } = actions;
  if (bookmark !== undefined) {
    entries.push(
      item(
        "bookmark",
        bookmark.isMarked(node.id)
          ? "ブックマークから削除"
          : "ブックマークに追加",
        () => bookmark.onToggle(reference),
      ),
    );
  }
  entries.push(
    menuSeparator("terms"),
    item(
      "contains",
      "表示名を含む条件に追加",
      () => {
        if (text !== undefined) actions.onAddTerm("contains", text);
      },
      text === undefined,
    ),
    item(
      "excludes",
      "表示名を含まない条件に追加",
      () => {
        if (text !== undefined) actions.onAddTerm("excludes", text);
      },
      text === undefined,
    ),
    menuSeparator("copy"),
    item(
      "copy-label",
      "表示名をコピー",
      () => {
        if (text !== undefined) copy(text, "表示名");
      },
      text === undefined,
    ),
  );
  return {
    label: `${nodeKindLabels[node.kind]} ${nodeLabelName(label)} の操作`,
    entries,
  };
}
