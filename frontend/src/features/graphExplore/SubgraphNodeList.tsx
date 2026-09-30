import { PanelRightOpen, Share2 } from "lucide-react";
import { memo, useMemo, useState } from "react";
import type {
  NodeIdentityValue,
  NodeKind,
  NodeValueMatch,
  SubgraphNode,
} from "@/shared/contracts/graph";
import { toVisibleRawText } from "@/shared/lib/rawText";
import { ipSortValue } from "@/shared/lib/sortValue";
import {
  type ContextMenuTriggers,
  RowMenuButton,
  useContextMenu,
} from "@/shared/ui/ContextMenu";
import { Highlighted } from "@/shared/ui/Highlighted";
import { IconButton } from "@/shared/ui/IconButton";
import { MissingValue } from "@/shared/ui/MissingValue";
import { RawText } from "@/shared/ui/RawText";
import { SortHeader, useSortedRows } from "@/shared/ui/SortHeader";
import { useCopyText } from "@/shared/ui/useCopyText";
import { ListFilter } from "./ListFilterControl";
import {
  nodeCreationRecordLabels,
  nodeKeyFormLabelOf,
  nodeKindLabelOf,
  nodeKindLabels,
  nodeSelectionLabels,
  valueMatchFormLabels,
} from "./labels";
import { includesListText, nodeSearchValues } from "./listFilter";
import { NodeLabelView, nodeLabelName } from "./NodeLabelView";
import { NodeValueLink } from "./NodeValueLink";
import { type NodeMenuActions, nodeMenuContent } from "./nodeMenu";
import { ShowMoreRows, useShownRows } from "./ShownRows";
import { readNodeLabel } from "./subgraph";

/** 並べ替えられる列の値。IP アドレスのノードは表示名をアドレスの大小で並べる。 */
const nodeSortValues = {
  label: (node: SubgraphNode) => {
    const name = nodeLabelName(readNodeLabel(node.label));
    return node.kind === "ip" ? ipSortValue(name) : name;
  },
  kind: (node: SubgraphNode) => nodeKindLabelOf(node),
  keyForm: (node: SubgraphNode) => nodeKeyFormLabelOf(node),
};

/**
 * 識別鍵の 1 項目を並びの中で見分ける値を作る。React の再描画だけに使い、
 * 要求にも表示にも出さない。
 */
function identityRenderKey(value: NodeIdentityValue): string {
  return JSON.stringify([value.semantic ?? null, value.value]);
}

function NodeIdentityCell({ node }: { node: SubgraphNode }) {
  return (
    <ul className="value-lines">
      {node.identity.map((value) => (
        <li
          key={identityRenderKey(value)}
          title={
            value.semantic === undefined
              ? toVisibleRawText(value.value)
              : `${toVisibleRawText(value.semantic)}: ${toVisibleRawText(value.value)}`
          }
        >
          {value.semantic === undefined ? null : (
            <>
              <RawText text={value.semantic} />
              {": "}
            </>
          )}
          <Highlighted
            text={value.value}
            field={{ name: value.semantic ?? "", semantic: value.semantic }}
          />
        </li>
      ))}
    </ul>
  );
}

/**
 * 一致したフィールドを持たないノードのセルに出す印。一覧に入った経路 (`selection`) で分ける。
 *
 * **一致ノードは、ノードを指すレコードのフィールドで一致する。** ノードの属性のフィールドに
 * 一致したものが無いことがある。表示名は識別鍵から組むため、表示名が文字列を含んでも属性の
 * フィールドに一致しないことがある。
 */
const noValueMatchDescriptions: Record<SubgraphNode["selection"], string> = {
  matched: "レコード経由の一致",
  edge_endpoint: "エッジの端のノード",
};

/**
 * 検索が一致した欄 1 件を並びの中で見分ける値を作る。React の再描画だけに使う。
 */
function valueMatchRenderKey(match: NodeValueMatch): string {
  return JSON.stringify([
    match.semantic ?? null,
    match.name ?? null,
    match.form,
  ]);
}

/**
 * 検索が一致した欄と、一致した値と、原資料の文字列と正規化値のどちらで一致したかを出す。
 * 語彙に写していない欄は原資料の key の文字列で並ぶ。同じ欄に一致した値が 2 件以上あっても、
 * 最初の 1 件だけを出す。
 */
function NodeValueMatchCell({
  matches,
  selection,
}: {
  matches: NodeValueMatch[];
  selection: SubgraphNode["selection"];
}) {
  if (matches.length === 0) {
    return <MissingValue description={noValueMatchDescriptions[selection]} />;
  }
  return (
    <ul className="value-lines">
      {matches.map((match) => (
        <li
          key={valueMatchRenderKey(match)}
          title={`${toVisibleRawText(match.semantic ?? match.name ?? "")}: ${toVisibleRawText(match.value)}`}
        >
          <RawText text={match.semantic ?? match.name ?? ""} />
          {": "}
          <RawText text={match.value} />{" "}
          <span className="text-xs text-muted">
            {valueMatchFormLabels[match.form]}
          </span>
        </li>
      ))}
    </ul>
  );
}

/**
 * 根拠のレコードが記録した端末の表示名を出す。
 *
 * **識別鍵の欄と別の列にする。** 端末は根拠から解決した派生の値であり、このノードを
 * 指す識別鍵ではない。
 *
 * **0 件・1 件・2 件以上を別に読める形にする。** 2 件以上を持つノードは、複数の端末の
 * レコードが同じ対象を指すことを表す。
 */
function NodeTerminalCell({ node }: { node: SubgraphNode }) {
  const terminals = node.terminals ?? [];
  if (terminals.length === 0) {
    return <MissingValue description="端末の記録なし" />;
  }
  return (
    <ul className="value-lines">
      {terminals.map((terminal) => (
        <li
          key={terminal.id}
          title={toVisibleRawText(
            terminal.label.rawText ?? terminal.label.normalized ?? "",
          )}
        >
          <NodeLabelView label={readNodeLabel(terminal.label)} />
        </li>
      ))}
    </ul>
  );
}

/**
 * ノード 1 件の行。
 * ノードを選び直したときに、選んだ行と選びを外した行だけを描き直すよう memo にする。
 */
const SubgraphNodeRow = memo(function SubgraphNodeRow({
  node,
  isSelected,
  isOrigin,
  showsValueMatches,
  showsTerminals,
  onSelect,
  onExpand,
  menu,
  isMenuOpen,
}: {
  node: SubgraphNode;
  isSelected: boolean;
  isOrigin: boolean;
  showsValueMatches: boolean;
  showsTerminals: boolean;
  onSelect: (nodeId: string) => void;
  onExpand: (nodeId: string, kind: NodeKind) => void;
  /** 行の操作のメニューを開く操作。一覧が 1 組を持ち、描画をまたいで同じ組を渡す。 */
  menu: ContextMenuTriggers<SubgraphNode>;
  isMenuOpen: boolean;
}) {
  const kind = nodeKindLabelOf(node);
  const label = readNodeLabel(node.label);
  // **button の名前に種類と表示名を入れる。** 同じ操作の button が行の数だけ並び、読み上げで
  // どの行の button かを見分ける。
  const buttonName = `${kind} ${nodeLabelName(label)}`;
  // 文字列の条件に一致したかは backend の判定 (`selection` と `valueMatches`) だけで決める。
  const textMatched =
    node.selection === "matched" && node.valueMatches !== undefined;
  return (
    <tr
      className={textMatched ? "search-matched-row" : undefined}
      onContextMenu={(event) => menu.onContextMenu(event, node)}
      onKeyDown={(event) => menu.onKeyDown(event, node)}
    >
      <td className="row-actions">
        <IconButton
          aria-pressed={isSelected}
          label={`${buttonName} の詳細を開く`}
          onPress={() => onSelect(node.id)}
        >
          <PanelRightOpen size={14} aria-hidden="true" />
        </IconButton>
        <IconButton
          aria-pressed={isOrigin}
          label={`${buttonName} の隣接ノードを追加`}
          onPress={() => onExpand(node.id, node.kind)}
        >
          <Share2 size={14} aria-hidden="true" />
        </IconButton>
        <RowMenuButton
          label={`${buttonName} の操作`}
          expanded={isMenuOpen}
          onClick={(event) => menu.onButtonClick(event, node)}
        />
      </td>
      <td className="wrapping-cell">
        <NodeValueLink node={node} />
      </td>
      <td className="short-cell">{kind}</td>
      <td className="short-cell">{nodeKeyFormLabelOf(node)}</td>
      <td>
        <NodeIdentityCell node={node} />
      </td>
      <td className="short-cell">{nodeSelectionLabels[node.selection]}</td>
      <td className="short-cell">
        {nodeCreationRecordLabels[node.creationRecord]}
      </td>
      {showsValueMatches ? (
        <td>
          <NodeValueMatchCell
            matches={node.valueMatches ?? []}
            selection={node.selection}
          />
        </td>
      ) : null}
      {showsTerminals ? (
        <td>
          <NodeTerminalCell node={node} />
        </td>
      ) : null}
    </tr>
  );
});

type SubgraphNodeListProps = {
  nodes: SubgraphNode[];
  /** 選んでいるノード。状態の所有者は上位の画面である。 */
  selectedNodeId: string | undefined;
  /** 関係先を出す起点のノード。状態の所有者は上位の画面である。 */
  originNodeIds: readonly string[];
  onSelect: (nodeId: string) => void;
  onExpand: (nodeId: string, kind: NodeKind) => void;
  /** 行のコンテキストメニューが呼ぶ操作。描画をまたいで同じ組を渡す。 */
  menuActions: NodeMenuActions;
};

/**
 * 部分グラフのノードを 1 行ずつ出し、選んだノードと起点にするノードを上位へ渡す。
 * 図は読み上げの対象にならないため、ノードを選ぶ操作を本一覧が持つ。図の右クリックと同じ
 * メニューを、行の右クリックと Shift+F10 と「…」の button から開く。
 *
 * **渡す値が変わらない間は描き直さない。** 一覧は数千行になり、関係を選ぶたびに
 * 上位の画面が描き直される。
 */
export const SubgraphNodeList = memo(function SubgraphNodeList({
  nodes,
  selectedNodeId,
  originNodeIds,
  onSelect,
  onExpand,
  menuActions,
}: SubgraphNodeListProps) {
  const { copy, notice } = useCopyText();
  const { triggers, openTarget, menu } = useContextMenu((node: SubgraphNode) =>
    nodeMenuContent(node, menuActions, copy),
  );
  // **検索の文字列を与えた応答でだけ欄を出す。** 応答が一致した欄を含まないときに
  // 空の欄を並べると、一致しなかったことと検索していないことを読み分けられない。
  const showsValueMatches = nodes.some(
    (node) => node.valueMatches !== undefined,
  );
  // **1 件でも端末を名乗る応答でだけ欄を出す。** どのノードも名乗らないときに空の欄を
  // 並べると、名乗らなかったことと端末を含まない応答を読み分けられない。
  const showsTerminals = nodes.some((node) => node.terminals !== undefined);
  const [kind, setKind] = useState("");
  const [text, setText] = useState("");
  const kinds = useMemo(() => {
    const values = new Set<string>([kind, ...nodes.map((node) => node.kind)]);
    return Object.entries(nodeKindLabels)
      .filter(([value]) => values.has(value))
      .map(([value, label]) => ({ value, label }));
  }, [nodes, kind]);
  const filtered = useMemo(
    () =>
      nodes.filter(
        (node) =>
          (kind === "" || node.kind === kind) &&
          (text === "" || includesListText(nodeSearchValues(node), text)),
      ),
    [nodes, kind, text],
  );
  // 並べ替えは「続きを表示」で行数を切る前の全行に適用する。
  const { sorted, header } = useSortedRows(filtered, nodeSortValues);
  const rows = useShownRows(sorted);
  return (
    <>
      <ListFilter
        name="ノード"
        kinds={kinds}
        kind={kind}
        text={text}
        onKindChange={setKind}
        onTextChange={setText}
        matched={filtered.length}
        total={nodes.length}
      />
      <table>
        {/* 見える見出しは Nodes のビューが件数と一緒に出す。表の名前は読み上げに残す。 */}
        <caption className="sr-only">グラフのノードの一覧</caption>
        <thead>
          <tr>
            <th scope="col" className="row-actions">
              操作
            </th>
            <SortHeader {...header("label")}>表示名</SortHeader>
            <SortHeader {...header("kind")}>種類</SortHeader>
            <SortHeader {...header("keyForm")}>同一性の基準</SortHeader>
            <th scope="col">同一性の値</th>
            <th scope="col">表示の理由</th>
            <th scope="col">作成レコード</th>
            {showsValueMatches ? <th scope="col">一致したフィールド</th> : null}
            {showsTerminals ? <th scope="col">記録した端末</th> : null}
          </tr>
        </thead>
        <tbody>
          {rows.visible.map((node) => (
            <SubgraphNodeRow
              key={node.id}
              node={node}
              isSelected={node.id === selectedNodeId}
              isOrigin={originNodeIds.includes(node.id)}
              showsValueMatches={showsValueMatches}
              showsTerminals={showsTerminals}
              onSelect={onSelect}
              onExpand={onExpand}
              menu={triggers}
              isMenuOpen={openTarget === node}
            />
          ))}
        </tbody>
      </table>
      <ShowMoreRows {...rows} />
      {notice}
      {menu}
    </>
  );
});
