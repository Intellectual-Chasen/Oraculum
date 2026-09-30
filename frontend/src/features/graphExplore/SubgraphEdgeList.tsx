import { PanelRightOpen } from "lucide-react";
import { memo, useMemo, useState } from "react";
import type { TimeRange } from "@/shared/contracts/common";
import type {
  CandidateTierConditionKey,
  EdgeCandidateTier,
  GraphEdge,
  SubgraphNode,
} from "@/shared/contracts/graph";
import { formatEvidenceCountWithCases } from "@/shared/lib/caseCounts";
import { formatCount } from "@/shared/lib/format";
import { toVisibleRawText } from "@/shared/lib/rawText";
import { rangeOverlapsPeriod } from "@/shared/lib/searchHighlight";
import { timestampSortValue } from "@/shared/lib/sortValue";
import { terminalAssignmentOriginLabels } from "@/shared/lib/terminalAssignmentLabels";
import {
  type ContextMenuTriggers,
  RowMenuButton,
  useContextMenu,
} from "@/shared/ui/ContextMenu";
import { Highlighted, useSearchHighlight } from "@/shared/ui/Highlighted";
import { Hint } from "@/shared/ui/Hint";
import { IconButton } from "@/shared/ui/IconButton";
import { MissingValue } from "@/shared/ui/MissingValue";
import { SortHeader, useSortedRows } from "@/shared/ui/SortHeader";
import { InterpretationNote } from "@/shared/ui/TimestampOffsetNote";
import { TimestampText } from "@/shared/ui/TimestampText";
import { useCopyText } from "@/shared/ui/useCopyText";
import { TimeValueLink } from "@/shared/ui/ValueLink";
import { type EdgeMenuActions, edgeMenuContent, edgeName } from "./edgeMenu";
import { ListFilter } from "./ListFilterControl";
import {
  edgeKindLabels,
  relationStateDescriptions,
  relationStateLabels,
} from "./labels";
import { includesListText, nodeSearchValues } from "./listFilter";
import { EdgeValueLink, NodeValueLink } from "./NodeValueLink";
import { ShowMoreRows, useShownRows } from "./ShownRows";

const applicableRangeAbsent = "UTC 時刻を持つ根拠なし";

/** 並べ替えられる列の値。期間は始まりの時刻で並べる。 */
const edgeSortValues = {
  kind: (edge: GraphEdge) => edgeKindLabels[edge.kind] ?? edge.kind,
  state: (edge: GraphEdge) => relationStateLabels[edge.state],
  indistinguishable: (edge: GraphEdge) => edge.indistinguishableCandidateCount,
  tier: (edge: GraphEdge) => edge.candidateTier?.tier,
  evidence: (edge: GraphEdge) => edge.evidenceCount,
  period: (edge: GraphEdge) => timestampSortValue(edge.applicableRange?.from),
};

/**
 * 期間の両端を読んだずれの出どころを出す。両端が同じずれと出どころで読まれたときは 1 行にまとめる。
 * それ以外は、どちらの端の注記かを「始まり:」「終わり:」で示す。原資料の文字列からずれが定まる
 * 端には何も出さない。
 */
function RangeInterpretationNotes({ range }: { range: TimeRange }) {
  const from = range.from.interpretation;
  const to = range.to.interpretation;
  if (
    from !== undefined &&
    to !== undefined &&
    from.offset === to.offset &&
    (from.assertionId === undefined) === (to.assertionId === undefined)
  ) {
    return (
      <InterpretationNote
        interpretation={from}
        className="text-xs text-muted"
      />
    );
  }
  return (
    <>
      {from === undefined ? null : (
        <InterpretationNote
          interpretation={from}
          prefix="始まり: "
          className="text-xs text-muted"
        />
      )}
      {to === undefined ? null : (
        <InterpretationNote
          interpretation={to}
          prefix="終わり: "
          className="text-xs text-muted"
        />
      )}
    </>
  );
}

/** 候補の区分を決めた条件の短い名前。 */
const candidateTierConditionLabels: Record<CandidateTierConditionKey, string> =
  {
    session_account_match: "アカウントが一致",
    session_account_different: "アカウントが一致しない",
    session_logon_interactive: "対話か画面の遠隔操作のログオン",
    session_logon_network: "ネットワークのログオン",
    session_logon_other: "その他のログオン",
  };

/** 候補の順位を決めた条件を並べる。 */
function candidateTierConditions(tier: EdgeCandidateTier): string {
  return tier.conditions
    .map((condition) => candidateTierConditionLabels[condition])
    .join("、");
}

/**
 * 端点のノードの表示名と識別鍵の値を出す。
 * エッジの両端は同じ応答の `nodes` にあり、識別子から両方を探せる。
 *
 * **表示名に識別鍵の値を添える。** 同じ表示名を持つ別のノードが同じ応答に入るため、
 * 表示名だけでは 2 本の関係がどのノードを指すかを読めない。
 */
function EndpointCell({
  nodeId,
  nodeById,
}: {
  nodeId: string;
  nodeById: Map<string, SubgraphNode>;
}) {
  const node = nodeById.get(nodeId);
  if (node === undefined) {
    return <MissingValue description="ノードなし" />;
  }
  return (
    <div className="whitespace-nowrap">
      <NodeValueLink node={node} />
      <ul className="value-lines text-xs text-muted">
        {node.identity.map((value) => (
          <li
            key={JSON.stringify([value.semantic ?? null, value.value])}
            title={toVisibleRawText(value.value)}
          >
            <Highlighted
              text={value.value}
              field={{ name: value.semantic ?? "", semantic: value.semantic }}
            />
          </li>
        ))}
      </ul>
    </div>
  );
}

/**
 * エッジ 1 本の行。
 * 関係を選び直したときに、選んだ行と選びを外した行だけを描き直すよう memo にする。
 */
const SubgraphEdgeRow = memo(function SubgraphEdgeRow({
  edge,
  nodeById,
  isSelected,
  onSelect,
  menu,
  isMenuOpen,
}: {
  edge: GraphEdge;
  nodeById: Map<string, SubgraphNode>;
  isSelected: boolean;
  onSelect: (edgeId: string) => void;
  /** 行の操作のメニューを開く操作。一覧が 1 組を持ち、描画をまたいで同じ組を渡す。 */
  menu: ContextMenuTriggers<GraphEdge>;
  isMenuOpen: boolean;
}) {
  // 割当から作ったエッジは、割当を根拠として件数に添える。
  // 由来の要素を持たない配列は、割当から作っていないエッジと同じに扱う。
  const origins =
    (edge.assignmentOrigins?.length ?? 0) > 0
      ? edge.assignmentOrigins
      : undefined;
  const name = edgeName(edge, (nodeId) => nodeById.get(nodeId));
  const { timeFilter } = useSearchHighlight();
  return (
    <tr
      className={
        rangeOverlapsPeriod(edge.applicableRange, timeFilter)
          ? "search-matched-row"
          : undefined
      }
      onContextMenu={(event) => menu.onContextMenu(event, edge)}
      onKeyDown={(event) => menu.onKeyDown(event, edge)}
    >
      <td className="row-actions">
        <IconButton
          label="エッジの詳細を表示"
          accessibleName={`${name} の詳細を表示`}
          isDisabled={isSelected}
          disabledReason={{ title: "表示中", text: "Edge Detail に表示中" }}
          onPress={() => onSelect(edge.id)}
        >
          <PanelRightOpen size={14} aria-hidden="true" />
        </IconButton>
        <RowMenuButton
          label={`${name} の操作`}
          expanded={isMenuOpen}
          onClick={(event) => menu.onButtonClick(event, edge)}
        />
      </td>
      <td className="label-cell">
        <EdgeValueLink edge={edge} nodeOf={(id) => nodeById.get(id)} />
      </td>
      <td className="label-cell">
        <Hint text={relationStateDescriptions[edge.state]}>
          {relationStateLabels[edge.state]}
        </Hint>
      </td>
      <td className="text-right tabular-nums">
        {edge.indistinguishableCandidateCount === undefined
          ? null
          : formatCount(edge.indistinguishableCandidateCount)}
      </td>
      <td className="text-right tabular-nums">
        {edge.candidateTier === undefined
          ? null
          : formatCount(edge.candidateTier.tier)}
      </td>
      <td>
        {edge.candidateTier === undefined
          ? null
          : candidateTierConditions(edge.candidateTier)}
      </td>
      <td>
        <EndpointCell nodeId={edge.sourceNodeId} nodeById={nodeById} />
      </td>
      <td>
        <EndpointCell nodeId={edge.targetNodeId} nodeById={nodeById} />
      </td>
      <td className="tabular-nums">
        {formatEvidenceCountWithCases(edge.evidenceCount, edge.evidenceByCase)}
      </td>
      <td>
        {origins === undefined
          ? null
          : origins
              .map((origin) => terminalAssignmentOriginLabels[origin])
              .join("、")}
      </td>
      <td>
        {edge.applicableRange === undefined ? (
          <MissingValue description={applicableRangeAbsent} />
        ) : (
          <>
            {origins !== undefined && edge.evidenceCount === 0 ? (
              <span className="pair-name">適用期間: </span>
            ) : null}
            <TimeValueLink timestamp={edge.applicableRange.from}>
              <TimestampText timestamp={edge.applicableRange.from} />
            </TimeValueLink>
            {" – "}
            <TimeValueLink timestamp={edge.applicableRange.to}>
              <TimestampText timestamp={edge.applicableRange.to} />
            </TimeValueLink>
            <RangeInterpretationNotes range={edge.applicableRange} />
          </>
        )}
      </td>
    </tr>
  );
});

type SubgraphEdgeListProps = {
  edges: GraphEdge[];
  nodes: SubgraphNode[];
  /** 詳細を出している関係。選んでいる行の操作を無効にする。 */
  selectedEdgeId: string | undefined;
  /** 関係 1 本を選んで詳細を出す。 */
  onSelect: (edgeId: string) => void;
  /** 行のコンテキストメニューが呼ぶ操作。描画をまたいで同じ組を渡す。 */
  menuActions: EdgeMenuActions;
};

/**
 * 部分グラフのエッジを 1 行ずつ出す。
 * 図は読み上げの対象にならないため、関係の種別と状態と根拠の件数を本一覧が出す。図の右クリックと
 * 同じメニューを、行の右クリックと Shift+F10 と「…」の button から開く。
 *
 * **同じ応答の間は、選び直した行だけを描き直す。** 一覧は数千行になり、関係の詳細の
 * 取得の状態が変わるたびに上位の画面が描き直される。
 */
export const SubgraphEdgeList = memo(function SubgraphEdgeList({
  edges,
  nodes,
  selectedEdgeId,
  onSelect,
  menuActions,
}: SubgraphEdgeListProps) {
  const nodeById = useMemo(
    () => new Map(nodes.map((node) => [node.id, node])),
    [nodes],
  );
  const { copy, notice } = useCopyText();
  const { triggers, openTarget, menu } = useContextMenu((edge: GraphEdge) =>
    edgeMenuContent(
      edge,
      (nodeId) => nodeById.get(nodeId),
      menuActions,
      selectedEdgeId,
      copy,
    ),
  );
  const [kind, setKind] = useState("");
  const [text, setText] = useState("");
  const kinds = useMemo(() => {
    const values = new Set<string>([kind, ...edges.map((edge) => edge.kind)]);
    return Object.entries(edgeKindLabels)
      .filter(([value]) => values.has(value))
      .map(([value, label]) => ({ value, label }));
  }, [edges, kind]);
  const filtered = useMemo(
    () =>
      edges.filter((edge) => {
        if (kind !== "" && edge.kind !== kind) return false;
        if (text === "") return true;
        const source = nodeById.get(edge.sourceNodeId);
        const target = nodeById.get(edge.targetNodeId);
        return includesListText(
          [
            edge.kind,
            edgeKindLabels[edge.kind],
            relationStateLabels[edge.state],
            ...(source === undefined
              ? [edge.sourceNodeId]
              : nodeSearchValues(source)),
            ...(target === undefined
              ? [edge.targetNodeId]
              : nodeSearchValues(target)),
          ],
          text,
        );
      }),
    [edges, kind, text, nodeById],
  );
  // 並べ替えは「続きを表示」で行数を切る前の全行に適用する。
  const { sorted, header } = useSortedRows(filtered, edgeSortValues);
  const rows = useShownRows(sorted);
  return (
    <>
      <ListFilter
        name="エッジ"
        kinds={kinds}
        kind={kind}
        text={text}
        onKindChange={setKind}
        onTextChange={setText}
        matched={filtered.length}
        total={edges.length}
      />
      <table>
        {/* 見える見出しは Edges のビューが件数と一緒に出す。表の名前は読み上げに残す。 */}
        <caption className="sr-only">グラフのエッジの一覧</caption>
        <thead>
          <tr>
            <th scope="col" className="row-actions">
              操作
            </th>
            <SortHeader {...header("kind")}>エッジの種類</SortHeader>
            <SortHeader {...header("state")}>作り方</SortHeader>
            <SortHeader {...header("indistinguishable")} className="text-right">
              区別できない候補
            </SortHeader>
            <SortHeader {...header("tier")} className="text-right">
              候補の順位
            </SortHeader>
            <th scope="col">順位の条件</th>
            <th scope="col">始点</th>
            <th scope="col">終点</th>
            <SortHeader {...header("evidence")}>根拠のレコード</SortHeader>
            <th scope="col">端末の割り当て</th>
            <SortHeader {...header("period")}>期間</SortHeader>
          </tr>
        </thead>
        <tbody>
          {rows.visible.map((edge) => (
            <SubgraphEdgeRow
              key={edge.id}
              edge={edge}
              nodeById={nodeById}
              isSelected={edge.id === selectedEdgeId}
              onSelect={onSelect}
              menu={triggers}
              isMenuOpen={openTarget === edge}
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
