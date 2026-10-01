import { Redo2, Trash2, Undo2, X } from "lucide-react";
import {
  type ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  maxGraphDepth,
  maxOriginNodes,
  type NodeRef,
  type RecordFilterCriteria,
} from "@/shared/api/graph";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import type { AccountRelationKey } from "@/shared/contracts/accountRelations";
import type { CaseId } from "@/shared/contracts/cases";
import type { RecordLocator, RequestedTime } from "@/shared/contracts/common";
import type { EventKind } from "@/shared/contracts/eventKinds";
import type {
  EdgeKind,
  GraphEdge,
  GraphResponse,
  SubgraphNode,
} from "@/shared/contracts/graph";
import type { NodeDetailResponse } from "@/shared/contracts/graphDetail";
import type { InfluenceBasis } from "@/shared/contracts/influencePath";
import type { FetchState } from "@/shared/lib/fetchState";
import { formatCount } from "@/shared/lib/format";
import { toVisibleRawText } from "@/shared/lib/rawText";
import { timestampPrecisionLabels } from "@/shared/lib/recordLabels";
import {
  type SearchTermKind,
  type SearchTerms,
  withoutField,
  withTerm,
} from "@/shared/lib/searchTerms";
import {
  autoView,
  resolveView,
  type ViewChoice,
} from "@/shared/lib/searchView";
import {
  type ContextMenuContent,
  useContextMenu,
} from "@/shared/ui/ContextMenu";
import { DerivedLabelNote } from "@/shared/ui/DerivedLabelNote";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { Hint } from "@/shared/ui/Hint";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { focusableElementOf } from "@/shared/ui/Menu";
import { anchorAtPoint } from "@/shared/ui/menuPlacement";
import { RawText } from "@/shared/ui/RawText";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import { Tabs } from "@/shared/ui/Tabs";
import { useCopyText } from "@/shared/ui/useCopyText";
import {
  AccountRelationsDetail,
  AccountRelationsFigure,
} from "./AccountRelationsView";
import { AttackCandidateList } from "./AttackCandidateList";
import { originsOf, shownNodeId, useAccountMerge } from "./accountMerge";
import {
  type CosmosLayoutSettings,
  defaultLayoutSettings,
} from "./cosmosLayout";
import { type DrawLimit, defaultDrawLimit, nodeLimitFor } from "./drawLimit";
import {
  type EdgeBookmark,
  type EdgeMenuActions,
  edgeMenuContent,
} from "./edgeMenu";
import {
  type Exploration,
  explorationCriteria,
  hasOrigin,
  withOrigin,
  withoutOrigin,
} from "./exploration";
import {
  defaultExcludedBases,
  type InfluenceEnds,
  InfluenceEndsPairs,
  InfluencePathFigure,
  InfluencePathView,
  selectedVertexKey,
  useInfluencePath,
  vertexNodeRef,
} from "./InfluencePathPane";
import { InvestigationOrderPane } from "./InvestigationOrderPane";
import { LayoutSettingsPopover } from "./LayoutSettingsInputs";
import {
  describeGraphEmptyReason,
  distinctSourceLabels,
  edgeKindLabels,
  eventKindFieldLabels,
  filterFieldLabels,
  graphEmptyReasonNextActions,
  nodeKindColors,
  nodeKindLabels,
  nodeSelectionLabels,
  timeUnitLabels,
  valueCountsEmptyReasonLabels,
  valueCountsNodeKinds,
} from "./labels";
import { MergedAccountSection } from "./MergedAccountSection";
import { NodeCsvExport } from "./NodeCsvExport";
import { NodeDetail, type NodeDetailActions } from "./NodeDetail";
import { NodeLabelValue } from "./NodeLabelView";
import {
  type NodeBookmark,
  type NodeMenuActions,
  nodeMenuContent,
} from "./nodeMenu";
import { RelatedValueCounts } from "./RelatedValueCounts";
import { SearchConditions } from "./SearchConditions";
import { SigmaRuleCandidatePane } from "./SigmaRuleCandidateList";
import { type GraphHighlight, SubgraphCanvas } from "./SubgraphCanvas";
import { SubgraphCanvasBoundary } from "./SubgraphCanvasBoundary";
import { SubgraphEdgeList } from "./SubgraphEdgeList";
import { SubgraphNodeList } from "./SubgraphNodeList";
import {
  buildSubgraphDrawing,
  type HighlightCounts,
  highlightCountsOf,
  lineageRootsOf,
  readNodeLabel,
  subgraphSummaryPairs,
  valueCountsSummaryPairs,
  withBackdrop,
} from "./subgraph";
import { useAccountRelations } from "./useAccountRelations";
import { useAttackCandidates } from "./useAttackCandidates";
import { useEventKinds } from "./useEventKinds";
import { useNodeDetail } from "./useNodeDetail";
import { type SubgraphCriteria, useSubgraph } from "./useSubgraph";
import { ValueCountList } from "./ValueCountList";
import { viewInputOf, withOriginKinds } from "./viewInput";
import {
  DrawLimitSelect,
  WithheldFigure,
  WithheldKindTables,
} from "./WithheldFigure";

// 既知の制限: 最初に出す部分グラフを、端末の粒度で上限を置いた 1 ホップにする,
// 利用者が配置した実資料で測った。端末とその間の
// エッジを 1 回の要求で返した。検索の条件を足すと、条件から選んだ粒度へ替わる (searchView.ts), 端末の数が上限を
// 超える資料を取り込んだときに見直す
const initialCriteria: LocalSubgraphCriteria = { depth: 1 };

const accountRecordEdgeKinds: readonly EdgeKind[] = [
  "record_subject_account",
  "record_target_account",
  "record_names_object",
];

/**
 * 本機能が持つ部分グラフの条件。
 * 関連付けの条件の選択と、根拠のレコードを絞る条件と、検索の文字列と、図に出す対象と、
 * 描画の上限は、上位の画面または本機能の別の状態が持つため、この型から外す。
 */
type LocalSubgraphCriteria = Omit<
  SubgraphCriteria,
  | "matchConditions"
  | "nodeLimit"
  | "timeFilter"
  | "eventCategory"
  | "eventAction"
  | "caseId"
  | "terminal"
  | "valueContains"
  | "valueExcludes"
  | "granularity"
  | "nodeKinds"
  | "nodeIds"
>;

/** 期間の端の時刻の、要求の文字列。端が無いときは空にする。 */
function requestedTimeText(value: RequestedTime | undefined): ReactNode {
  return value === undefined ? "" : <RawText text={value.requestText} />;
}

/** 応答が用いた期間を「名前: 値」の組で出す。 */
function ResponseTimeFilterSummary({ response }: { response: GraphResponse }) {
  if (response.timeFrom === undefined && response.timeTo === undefined) {
    return null;
  }
  return (
    <KeyValueList
      pairs={[
        {
          name: "期間",
          value: (
            <>
              {requestedTimeText(response.timeFrom)}
              {" – "}
              {requestedTimeText(response.timeTo)}
            </>
          ),
        },
        {
          name: "始まりの精度",
          value:
            response.timeFrom === undefined
              ? undefined
              : timestampPrecisionLabels[response.timeFrom.precision],
        },
        {
          name: "終わりの精度",
          value:
            response.timeTo === undefined
              ? undefined
              : timestampPrecisionLabels[response.timeTo.precision],
        },
        {
          name: "比較の単位",
          value:
            response.filterUnit === undefined
              ? "未取得"
              : timeUnitLabels[response.filterUnit],
        },
      ]}
    />
  );
}

/**
 * 応答が用いた絞り込みの条件と、近傍を広げたホップ数と、辿ったエッジの種類を出す。期間の条件は
 * ResponseTimeFilterSummary が出す。
 *
 * **0 件の応答でも server が用いた条件を出す。** 適用した条件が画面から消えると、
 * 分析者は 0 件がどの条件の結果かを読めない (ResponseTimeFilterSummary と同じ理由)。
 * 値は外部由来の文字列であり、`RawText` を通す。
 */
function ResponseConditionSummary({
  response,
  terminalLabel,
  sourceFileNames,
  eventKinds,
}: {
  response: GraphResponse;
  /** 応答が用いた端末の表示名。画面が識別子から表示名を求められないときは識別子を出す。 */
  terminalLabel: string | undefined;
  /** 収集元の sourceId から file 名を探す表。探せない収集元は sourceId を出す。 */
  sourceFileNames: ReadonlyMap<string, string>;
  /** 事象の種別の選択肢。事象の種別の条件の欄の名前を決める。 */
  eventKinds: readonly EventKind[];
}) {
  const eventKindLabels = eventKindFieldLabels(
    eventKinds,
    response.eventCategory,
    response.eventAction,
  );
  const applied = [
    ...(response.valueContains ?? []).map((value) => ({
      label: filterFieldLabels.valueContains,
      value,
    })),
    ...(response.valueExcludes ?? []).map((value) => ({
      label: filterFieldLabels.valueExcludes,
      value,
    })),
    {
      label: filterFieldLabels.terminal,
      value:
        response.terminal === undefined
          ? undefined
          : (terminalLabel ?? response.terminal),
    },
    ...(response.source ?? []).map((sourceId) => ({
      label: filterFieldLabels.sources,
      value: sourceFileNames.get(sourceId) ?? sourceId,
    })),
    { label: filterFieldLabels.countBy, value: response.countBy },
    { label: filterFieldLabels.addressInCidr, value: response.addressInCidr },
    {
      label: filterFieldLabels.addressNotInCidr,
      value: response.addressNotInCidr,
    },
    { label: eventKindLabels.eventCategory, value: response.eventCategory },
    { label: eventKindLabels.eventAction, value: response.eventAction },
    {
      label: "イベント ID の範囲",
      value:
        response.eventActionFrom === undefined &&
        response.eventActionTo === undefined
          ? undefined
          : `${response.eventActionFrom ?? ""}–${response.eventActionTo ?? ""}`,
    },
    { label: "文字列を探すフィールド", value: response.valueField },
    ...(response.fieldContains ?? []).map((value) => ({
      label: filterFieldLabels.fieldContains,
      value,
    })),
    ...(response.fieldEquals ?? []).map((value) => ({
      label: filterFieldLabels.fieldEquals,
      value,
    })),
    {
      label: filterFieldLabels.searchExpression,
      value: response.searchExpression,
    },
    { label: filterFieldLabels.caseId, value: response.case },
    { label: "ホップ数", value: String(response.depth) },
    ...(response.edgeKinds ?? []).map((kind) => ({
      label: "エッジの種類",
      value: edgeKindLabels[kind],
    })),
  ];
  return (
    <KeyValueList
      pairs={applied.map((item) => ({
        name: item.label,
        key: `${item.label}:${item.value}`,
        value:
          item.value === undefined ? undefined : <RawText text={item.value} />,
      }))}
    />
  );
}

/**
 * 図の入力を組み立てて描く。座標は cosmos.gl が描きながら計算する。
 *
 * **組み立てを error boundary の内側に置く。** graphology は同じ識別子のノードを
 * 2 件受け取ると例外を投げるため、組み立てを外に置くと画面全体が消える。
 */
function SubgraphFigure({
  response,
  settings,
  selectedNodeId,
  onSelectNode,
  onSelectEdge,
  onAddOrigin,
  onNodeContextMenu,
  onEdgeContextMenu,
  highlight,
  backdrop,
  emphasizesResponse,
  legendItems,
}: {
  response: GraphResponse;
  settings: CosmosLayoutSettings;
  selectedNodeId: string | undefined;
  onSelectNode: (nodeId: string) => void;
  onSelectEdge: (edgeId: string) => void;
  onAddOrigin: (nodeId: string) => void;
  onNodeContextMenu: (nodeId: string, event: MouseEvent) => void;
  onEdgeContextMenu: (edgeId: string, event: MouseEvent) => void;
  highlight: GraphHighlight | undefined;
  backdrop: GraphResponse | undefined;
  /** 背景を描くときに、応答を強調して背景を薄く描くか。偽の値は両方を同じ強さで描く。 */
  emphasizesResponse: boolean;
  legendItems: ReactNode;
}) {
  const drawing = useMemo(() => {
    const focus = buildSubgraphDrawing(response);
    return backdrop === undefined
      ? focus
      : withBackdrop(focus, buildSubgraphDrawing(backdrop));
  }, [response, backdrop]);
  // **背景を描くときは、部分グラフを強調する。** 開いたレコードの強調があればそちらを優先する。
  const focusHighlight = useMemo<GraphHighlight | undefined>(
    () =>
      backdrop === undefined || !emphasizesResponse
        ? undefined
        : {
            nodeIds: new Set(response.nodes.map((node) => node.id)),
            edgeIds: new Set(response.edges.map((edge) => edge.id)),
          },
    [response, backdrop, emphasizesResponse],
  );
  return (
    <SubgraphCanvas
      drawing={drawing}
      settings={settings}
      selectedNodeId={selectedNodeId}
      onSelectNode={onSelectNode}
      onSelectEdge={onSelectEdge}
      onDoubleClickNode={onAddOrigin}
      onNodeContextMenu={onNodeContextMenu}
      onEdgeContextMenu={onEdgeContextMenu}
      highlight={highlight ?? focusHighlight}
      highlightLegend={
        highlight === undefined
          ? focusHighlight === undefined
            ? undefined
            : "検索の結果のエッジ"
          : `${highlightSourceOf(highlight)}のエッジ`
      }
      legendItems={legendItems}
    />
  );
}

/** 図で右クリックした点またはエッジ。応答に含まれる値を持つ。 */
type FigureMenuTarget =
  | { kind: "node"; node: SubgraphNode }
  | { kind: "edge"; edge: GraphEdge };

/** 強調するノードとエッジを根拠に持つレコードの名前。 */
function highlightSourceOf(highlight: GraphHighlight): string {
  return highlight.source ?? "開いたレコード";
}

/**
 * 強調するノードとエッジの数の組 (`li`)。「応答にある数/対応する数」で出す。
 * 上限を超えた応答はエッジを持たないため、`edges` が偽のときはノードだけを出す。
 */
function HighlightPairs({
  counts,
  source,
  edges = true,
}: {
  counts: HighlightCounts;
  source: string;
  edges?: boolean;
}) {
  return (
    <>
      <li>
        <Hint text={`${source}のノード: グラフにある数/全体の数`}>
          <span className="pair-name">強調のノード:</span>{" "}
          {`${formatCount(counts.shownNodes)}/${formatCount(counts.nodes)}`}
        </Hint>
      </li>
      {edges ? (
        <li>
          <Hint text={`${source}のエッジ: グラフにある数/全体の数`}>
            <span className="pair-name">強調のエッジ:</span>{" "}
            {`${formatCount(counts.shownEdges)}/${formatCount(counts.edges)}`}
          </Hint>
        </li>
      ) : null}
    </>
  );
}

/** 図と一覧で選んでいるノード。 */
type SelectedPoint = { kind: "node"; id: string };

/**
 * 0 件の応答の理由と、server が用いた条件と期間。Search の結果に出す。
 *
 * **0 件の応答でも、server が用いた期間を出す。** 期間で絞った結果が 0 件のとき、
 * 適用した期間が画面から消えると、分析者は 0 件がどの期間の結果かを読めない。
 * 読み上げに期間を届けるため、2 つを 1 つの通知にまとめる。
 */
function EmptyGraphStatus({
  response,
  emptyReason,
  terminalLabel,
  sourceFileNames,
  eventKinds,
}: {
  response: GraphResponse;
  emptyReason: NonNullable<GraphResponse["emptyReason"]>;
  terminalLabel: string | undefined;
  sourceFileNames: ReadonlyMap<string, string>;
  eventKinds: readonly EventKind[];
}) {
  const nextAction = graphEmptyReasonNextActions[emptyReason];
  return (
    <div role="status">
      <div className="flex flex-wrap items-center gap-x-2">
        <KeyValueList
          pairs={[{ name: nodeSelectionLabels.matched, value: "0" }]}
        />
        <StatusLabel
          status="idle"
          label={describeGraphEmptyReason({ ...response, emptyReason })}
          details={
            nextAction === undefined ? undefined : (
              <KeyValueList
                stacked
                pairs={[{ name: "次の操作", value: nextAction }]}
              />
            )
          }
        />
      </div>
      <ResponseConditionSummary
        response={response}
        terminalLabel={terminalLabel}
        sourceFileNames={sourceFileNames}
        eventKinds={eventKinds}
      />
      <ResponseTimeFilterSummary response={response} />
    </div>
  );
}

/** 図を描く。0 件の応答は、図の枠に件数だけを出し、理由は Search の結果が出す。 */
function SubgraphFigurePane({
  response,
  settings,
  selectedId,
  onSelectNode,
  onSelectEdge,
  onAddOrigin,
  highlight,
  backdrop,
  selectedEdgeId,
  nodeMenuActions,
  edgeMenuActions,
  emphasizesResponse,
  legendItems,
}: {
  response: GraphResponse;
  settings: CosmosLayoutSettings;
  highlight: GraphHighlight | undefined;
  /** 部分グラフの外側に描く、検索の条件のグラフ。部分グラフを出していない間は無い。 */
  backdrop: GraphResponse | undefined;
  /** 部分グラフを強調し、背景を薄く描くか。 */
  emphasizesResponse: boolean;
  selectedId: string | undefined;
  /** 詳細を出している関係。メニューの詳細を出す項目を使えなくする。 */
  selectedEdgeId: string | undefined;
  onSelectNode: (nodeId: string) => void;
  onSelectEdge: (edgeId: string) => void;
  onAddOrigin: (nodeId: string) => void;
  nodeMenuActions: NodeMenuActions;
  edgeMenuActions: EdgeMenuActions;
  /** 凡例の末尾に足す値の組。 */
  legendItems: ReactNode;
}) {
  const { copy, notice } = useCopyText();
  // 部分グラフと背景のどちらのノードも、図に描いた端点として指す。
  const nodeOf = (id: string) =>
    response.nodes.find((node) => node.id === id) ??
    backdrop?.nodes.find((node) => node.id === id);
  const { triggers, menu } = useContextMenu(
    (target: FigureMenuTarget): ContextMenuContent => {
      switch (target.kind) {
        case "node":
          return nodeMenuContent(target.node, nodeMenuActions, copy);
        case "edge":
          // 図のエッジから詳細を出す操作は、図の上の click と同じくノードの選択を外す。
          return edgeMenuContent(
            target.edge,
            nodeOf,
            { ...edgeMenuActions, onSelectEdge },
            selectedEdgeId,
            copy,
          );
        default: {
          const exhaustive: never = target;
          throw new Error(
            `unknown figure target: ${JSON.stringify(exhaustive)}`,
          );
        }
      }
    },
  );
  if (response.emptyReason !== undefined) {
    return (
      <p className="figure-placeholder">{`${nodeSelectionLabels.matched}: 0`}</p>
    );
  }
  // 図は読み上げの対象にならず、キーボードで開く手段はノードの一覧の行が持つ。キーボードで
  // 閉じたときは、右クリックの前に focus を持っていた要素へ戻す。
  const openAt = (target: FigureMenuTarget, event: MouseEvent) =>
    triggers.openAt(
      target,
      anchorAtPoint(event.clientX, event.clientY),
      focusableElementOf(event.view?.document.activeElement ?? null),
    );
  const openPointMenu = (id: string, event: MouseEvent) => {
    const node = nodeOf(id);
    if (node !== undefined) openAt({ kind: "node", node }, event);
  };
  const openEdgeMenu = (id: string, event: MouseEvent) => {
    const edge =
      response.edges.find((candidate) => candidate.id === id) ??
      backdrop?.edges.find((candidate) => candidate.id === id);
    if (edge !== undefined) openAt({ kind: "edge", edge }, event);
  };
  return (
    <>
      <SubgraphCanvasBoundary>
        <SubgraphFigure
          response={response}
          backdrop={backdrop}
          emphasizesResponse={emphasizesResponse}
          settings={settings}
          selectedNodeId={selectedId}
          highlight={highlight}
          onSelectNode={onSelectNode}
          onSelectEdge={onSelectEdge}
          onAddOrigin={onAddOrigin}
          onNodeContextMenu={openPointMenu}
          onEdgeContextMenu={openEdgeMenu}
          legendItems={legendItems}
        />
      </SubgraphCanvasBoundary>
      {notice}
      {menu}
    </>
  );
}

/**
 * 一致ノードの数を大きく出し、種類ごとの数を添える。
 *
 * **SID と名前のアカウントのノードは別に数える。** 同じ人を 2 回数えた件数と読めるよう、同じ
 * アカウントの候補で結ばれた SID と名前の組の数を別の組で出す。
 */
function ResultCount({ response }: { response: GraphResponse }) {
  const pairCount = response.matchedAccountIdentityPairCount ?? 0;
  return (
    <div className="result-count">
      <p>
        <span className="pair-name">{nodeSelectionLabels.matched}:</span>{" "}
        <span className="result-number">{formatCount(response.nodeCount)}</span>
      </p>
      {response.matchedKinds.length === 0 ? null : (
        <ul
          aria-label={`${nodeSelectionLabels.matched}の種類`}
          className="kind-counts"
        >
          {response.matchedKinds.map((kind) => (
            <li key={kind.kind}>
              <span
                aria-hidden="true"
                className="kind-dot"
                style={{ background: nodeKindColors[kind.kind] }}
              />
              {`${nodeKindLabels[kind.kind]} ${formatCount(kind.count)}`}
            </li>
          ))}
          {pairCount === 0 ? null : (
            <li>
              <Hint text="同じアカウントの候補で結ばれた SID と名前の組">
                {`SID と名前の組 ${formatCount(pairCount)}`}
              </Hint>
            </li>
          )}
        </ul>
      )}
    </div>
  );
}

/**
 * 件数の要約と一覧を出す。
 *
 * **一覧は開いたときだけ読む。** 図と要約が結果の全体を示し、一覧は個々の要素を指すときに
 * 開く。
 */
function SubgraphSummaryPane({
  response,
  terminalLabel,
  sourceFileNames,
  eventKinds,
}: {
  response: GraphResponse;
  terminalLabel: string | undefined;
  sourceFileNames: ReadonlyMap<string, string>;
  eventKinds: readonly EventKind[];
}) {
  if (response.emptyReason !== undefined) {
    return (
      <EmptyGraphStatus
        response={response}
        emptyReason={response.emptyReason}
        terminalLabel={terminalLabel}
        sourceFileNames={sourceFileNames}
        eventKinds={eventKinds}
      />
    );
  }
  return (
    <>
      <ResultCount response={response} />
      <KeyValueList pairs={subgraphSummaryPairs(response)} />
      <ResponseConditionSummary
        response={response}
        terminalLabel={terminalLabel}
        sourceFileNames={sourceFileNames}
        eventKinds={eventKinds}
      />
      <ResponseTimeFilterSummary response={response} />
      <ValueCountSection response={response} />
    </>
  );
}

/**
 * グラフの応答を一覧のビューで出せないときの状態のラベル。理由は Search のビューの結果が出し、
 * ここでは繰り返さない。
 */
function listPlaceholder(state: FetchState<GraphResponse>): ReactNode {
  switch (state.status) {
    case "loading":
      return <StatusLabel status="running" label="グラフの読み込み中" />;
    case "failed":
    case "empty":
      return (
        <StatusLabel
          status="failed"
          label="グラフの取得に失敗"
          details="理由: Search の結果"
        />
      );
    case "loaded":
      return state.value.emptyReason === undefined ? undefined : (
        <StatusLabel
          status="idle"
          label={`${nodeSelectionLabels.matched}: 0`}
          details="理由: Search の結果"
        />
      );
    default: {
      const unreachable: never = state;
      return unreachable;
    }
  }
}

/**
 * Graph の区画が出しているグラフのノードの一覧と、合ったノードの CSV の書き出し。
 *
 * **上限を超えた応答では、合ったノードだけを並べる。** 図を描かない応答は、関係の相手を持たない。
 */
function NodesPane({
  state,
  criteria,
  selectedId,
  originNodeIds,
  onSelectNode,
  onSetOrigin,
  menuActions,
  summary,
}: {
  state: FetchState<GraphResponse>;
  criteria: SubgraphCriteria;
  selectedId: string | undefined;
  originNodeIds: readonly string[];
  onSelectNode: (nodeId: string) => void;
  onSetOrigin: (nodeId: string) => void;
  menuActions: NodeMenuActions;
  /**
   * 一覧の上に出す、ノードを種類で数えた表と親の記録が無いプロセス。Graph のビューは図の
   * ほかに一覧を持たないため、ここに出す。
   */
  summary: ReactNode;
}) {
  const placeholder = listPlaceholder(state);
  return (
    <section aria-labelledby="graph-nodes-heading" className="view-pane">
      <h2 id="graph-nodes-heading">
        {state.status === "loaded" &&
        placeholder === undefined &&
        state.value.nodeLimitExceeded
          ? nodeSelectionLabels.matched
          : "ノード"}
      </h2>
      {state.status === "loaded" && placeholder === undefined ? (
        <KeyValueList
          pairs={[
            { name: "件数", value: formatCount(state.value.nodes.length) },
          ]}
        />
      ) : null}
      {/* 書き出しは描画の上限に依らず合ったノードの全件を要求するため、図を描かない応答でも出す。 */}
      {state.status === "loaded" ? <NodeCsvExport criteria={criteria} /> : null}
      {summary}
      {placeholder !== undefined || state.status !== "loaded" ? (
        <div>{placeholder}</div>
      ) : (
        <SubgraphNodeList
          nodes={state.value.nodes}
          selectedNodeId={selectedId}
          originNodeIds={originNodeIds}
          onSelect={onSelectNode}
          onExpand={onSetOrigin}
          menuActions={menuActions}
        />
      )}
    </section>
  );
}

/** Graph の区画が出しているグラフのエッジの一覧。 */
function EdgesPane({
  state,
  selectedEdgeId,
  onSelectEdge,
  menuActions,
}: {
  state: FetchState<GraphResponse>;
  selectedEdgeId: string | undefined;
  onSelectEdge: (edgeId: string) => void;
  menuActions: EdgeMenuActions;
}) {
  const placeholder = listPlaceholder(state);
  const body = (() => {
    if (placeholder !== undefined || state.status !== "loaded") {
      return <div>{placeholder}</div>;
    }
    if (state.value.nodeLimitExceeded) {
      return (
        <StatusLabel
          status="idle"
          label="描画の上限を超過"
          details="上限の変更: Graph のビュー"
        />
      );
    }
    return (
      <SubgraphEdgeList
        edges={state.value.edges}
        nodes={state.value.nodes}
        selectedEdgeId={selectedEdgeId}
        onSelect={onSelectEdge}
        menuActions={menuActions}
      />
    );
  })();
  return (
    <section aria-labelledby="graph-edges-heading" className="view-pane">
      <h2 id="graph-edges-heading">エッジ</h2>
      {state.status === "loaded" &&
      placeholder === undefined &&
      !state.value.nodeLimitExceeded ? (
        <KeyValueList
          pairs={[
            { name: "本数", value: formatCount(state.value.edges.length) },
          ]}
        />
      ) : null}
      {body}
    </section>
  );
}

/**
 * 値ごとの件数を、値の種類の件数と一緒に出す。
 * 数えるフィールドを与えていない応答では何も出さない。
 */
function ValueCountSection({ response }: { response: GraphResponse }) {
  const summary = valueCountsSummaryPairs(response);
  if (summary === undefined || response.valueCounts === undefined) {
    return null;
  }
  return (
    <>
      <div className="flex flex-wrap items-center gap-x-2">
        <KeyValueList
          pairs={[
            ...summary,
            {
              name: "値を持つノードの種類",
              value:
                response.valueCountsNodeKinds === undefined
                  ? undefined
                  : valueCountsNodeKinds(response.valueCountsNodeKinds),
            },
          ]}
        />
        {response.valueCountsEmptyReason === undefined ? null : (
          <StatusLabel
            status="idle"
            label={
              valueCountsEmptyReasonLabels[response.valueCountsEmptyReason]
            }
          />
        )}
      </div>
      {/* 数えるフィールドが変わったら、並べ方と選んだ値を捨てる。 */}
      <ValueCountList key={response.countBy} counts={response.valueCounts} />
    </>
  );
}

/**
 * 親子の連鎖の上端のプロセスを出す。Nodes のビューに置く。
 *
 * **取り込んだ記録の中で親との関係を持たないプロセスを挙げる。** 連鎖がそこで切れていることは、
 * 親の起動が記録されていないか、親を指す欄が無いことを表す。上限のホップ数で辿りきれなかった
 * 場合と区別できないため、ホップ数を添える。
 */
function LineageRoots({
  response,
  onSelectNode,
}: {
  response: GraphResponse;
  onSelectNode: (nodeId: string) => void;
}) {
  const roots = lineageRootsOf(response);
  if (roots.length === 0) {
    return null;
  }
  return (
    <section aria-label="親の記録が無いプロセス">
      <ul className="value-pairs">
        <li>
          <span className="pair-name">親の記録なし:</span>{" "}
          {formatCount(roots.length)}
        </li>
        <li>
          <span className="pair-name">ホップ数:</span> {maxGraphDepth}
        </li>
      </ul>
      <ul>
        {roots.map((node) => (
          <li key={node.id}>
            {/* 導き方の印は button の外に置く。button の名前は表示名の値だけにする。 */}
            <button type="button" onClick={() => onSelectNode(node.id)}>
              <NodeLabelValue label={readNodeLabel(node.label)} />
            </button>
            {node.label.valueState === "derived" ? (
              <DerivedLabelNote derivation={node.label.derivation} />
            ) : null}
          </li>
        ))}
      </ul>
    </section>
  );
}

/**
 * 探索で出しているグラフの起点と、検索の結果へ戻る button。関係先を出しているときは、
 * 起点ごとに外す button を出す。
 */
function ExplorationStatus({
  exploration,
  onRemoveOrigin,
  onLeave,
}: {
  exploration: Exploration;
  onRemoveOrigin: (id: string) => void;
  onLeave: () => void;
}) {
  const leave = (
    <IconButton label="検索の結果に戻る" onPress={onLeave}>
      <Undo2 size={14} aria-hidden="true" />
    </IconButton>
  );
  if (exploration.kind === "lineage") {
    return (
      <div role="status" className="exploration-status">
        <KeyValueList
          pairs={[
            { name: "表示中", value: "プロセスの親子関係" },
            {
              name: "元のプロセス",
              value: <RawText text={exploration.origin.label} />,
            },
            { name: "検索の条件", value: "不使用" },
          ]}
        />
        {leave}
      </div>
    );
  }
  return (
    <div role="status" className="exploration-status">
      <p className="exploration-title">
        隣接ノードの表示元
        <HelpPopover label="隣接ノードの追加">
          <KeyValueList
            stacked
            pairs={[
              { name: "隣接ノードの追加", value: "ノードのダブルクリック" },
            ]}
          />
        </HelpPopover>
      </p>
      <ul className="origin-chips">
        {exploration.origins.map((origin) => (
          <li key={origin.id}>
            <RawText text={origin.label} />
            <IconButton
              label={`${toVisibleRawText(origin.label)} の隣接ノードを非表示`}
              onPress={() => onRemoveOrigin(origin.id)}
            >
              <X size={12} aria-hidden="true" />
            </IconButton>
          </li>
        ))}
      </ul>
      <KeyValueList
        className="note"
        pairs={[
          {
            name: "上限",
            value:
              exploration.origins.length >= maxOriginNodes
                ? String(maxOriginNodes)
                : undefined,
          },
          { name: "検索の条件", value: "不使用" },
        ]}
      />
      {leave}
    </div>
  );
}

/**
 * 分析者が選んだ部分グラフの条件のうち、本機能が持つもの。検索の条件と根拠のレコードを絞る条件は
 * 上位の画面が持つ。
 */
export type GraphExploreCriteria = Pick<
  SubgraphCriteria,
  | "depth"
  | "origins"
  | "edgeKinds"
  | "addressInCidr"
  | "addressNotInCidr"
  | "countBy"
  | "conditionsOnOriginsOnly"
  | "endpointRecordsInPeriod"
>;

/** 選んでいるノード。表示名と一緒に持つ。 */
export type GraphExploreSelection = { kind: "node"; node: NodeRef };

/**
 * 本機能が持つ状態のうち、画面を再開するときに戻すもの。
 *
 * `exploration.active` は探索を今の図に出しているかを表す。偽の探索は、検索の結果へ戻った後に
 * 持っている関係先の起点である。
 */
export type GraphExploreState = {
  criteria: GraphExploreCriteria;
  view: ViewChoice;
  drawLimit: DrawLimit;
  exploration?: {
    exploration: Exploration;
    active: boolean;
    kept?: Extract<Exploration, { kind: "neighbours" }>;
  };
  selected?: GraphExploreSelection;
  /** 同じアカウントのノードを図と表で 1 つにまとめるか。要求の条件に入れない。 */
  mergeSameAccount: boolean;
};

function persistedCriteria(
  criteria: LocalSubgraphCriteria,
): GraphExploreCriteria {
  return {
    depth: criteria.depth,
    origins: criteria.origins,
    edgeKinds: criteria.edgeKinds,
    addressInCidr: criteria.addressInCidr,
    addressNotInCidr: criteria.addressNotInCidr,
    countBy: criteria.countBy,
    conditionsOnOriginsOnly: criteria.conditionsOnOriginsOnly,
    endpointRecordsInPeriod: criteria.endpointRecordsInPeriod,
  };
}

/** 本機能が出すビュー。 */
export type GraphExplorePanes = {
  search: ReactNode;
  graph: ReactNode;
  detail: ReactNode;
  /** グラフのノードの一覧と CSV の書き出し。 */
  nodes: ReactNode;
  /** グラフのエッジの一覧。 */
  edges: ReactNode;
  /** Sigma のルールと ATT&CK の規則に一致した結果。 */
  detection: ReactNode;
  /** 調べる順序の目安。 */
  priority: ReactNode;
  /** 影響の経路の表と条件。図は graph が描く。 */
  path: ReactNode;
};

export type GraphExploreProps = {
  /**
   * 候補を絞るのに用いる条件の選択。時系列と関係の詳細も同じ選択を読むため、上位の
   * 画面が持つ。
   */
  matchConditions: MatchConditionSelection;
  onApplyMatchConditions: (selection: MatchConditionSelection) => void;
  /**
   * 根拠のレコードを絞る条件。時系列と同じ値で動くため、上位の画面が持つ。
   */
  recordFilter: RecordFilterCriteria;
  onApplyRecordFilter: (filter: RecordFilterCriteria) => void;
  /** 検索の文字列の条件。元レコードの表示からも足すため、上位の画面が持つ。 */
  searchTerms: SearchTerms;
  onChangeSearchTerms: (terms: SearchTerms) => void;
  /**
   * 取り込んだ収集元に付いた案件の識別子。昇順で、重複を持たない。
   * 定義元は収集元の一覧であり、上位の画面が一覧から導いて渡す。
   */
  caseIds: readonly CaseId[];
  /** 根拠のレコードを選んだときに、元レコードの表示へ渡す。 */
  onSelectRecord: (recordRef: RecordLocator) => void;
  /** 詳細を出している関係。上位の画面が持つ。 */
  selectedEdgeId: string | undefined;
  /** 関係 1 本を選んだときに、関係の詳細の表示へ渡す。 */
  onSelectEdge: (edgeId: string) => void;
  /** ノードに付いた所見の表示と記録。ノードを選んでいるときだけ出す。 */
  assertions: (detail: NodeDetailResponse) => ReactNode;
  /** ノードの詳細の見出しに並べる操作。出ない場合は出さない。 */
  nodeHeaderActions?: (detail: NodeDetailResponse) => ReactNode;
  /**
   * ノードとエッジのコンテキストメニューから付け外しするブックマーク。一覧は上位の画面が持つ。
   * 描画をまたいで同じ組を渡す。出ない場合は項目を出さない。
   */
  nodeBookmark?: NodeBookmark;
  edgeBookmark?: EdgeBookmark;
  /** 端末の割当を記録した回数。上位の画面が持ち、変わるたびにグラフと詳細を取り直す。 */
  dataVersion: number;
  /**
   * 本機能のビューを前面に出す。影響の経路を求めたときに Path を、Graph の操作で Nodes を出す。
   * 出ない場合は前面のビューを変えない。
   */
  onShowPane?: (pane: "path" | "nodes") => void;
  /** 選んだアカウントを名指した記録を表示する。 */
  onShowAccountRecords?: (node: NodeRef) => void;
  /**
   * 本機能のビューを並べる方法。上位の画面が自由に配置できる作業場所へ置く。
   */
  layout: (panes: GraphExplorePanes) => ReactNode;
  /**
   * 収集元の sourceId から file 名を探す表。ATT&CK 候補の関係を作った割当の「適用する
   * 収集元」を file 名で出す。渡さないときは sourceId をそのまま出す。
   */
  sourceFileNames?: ReadonlyMap<string, string>;
  /** 端末で絞る条件の選択肢。AI 支援の card の端末に表示名を付けるため、上位の画面が取得する。 */
  terminals: FetchState<NodeRef[]>;
  /**
   * 選んでいるノードが変わったときに知らせる。時系列を選んだノードの近くに絞るため、上位の
   * 画面が受け取る。選択を外したときは undefined を渡す。
   */
  onChangeSelectedNode?: (node: NodeRef | undefined) => void;
  /**
   * Path のビューの中でノードを選ぶ直前に、そのノードの識別子を知らせる。上位の画面は、この選択で
   * Node Detail を前面に出さない。
   */
  onSelectNodeInPath?: (nodeId: string) => void;
  /**
   * 図で強調するノードとエッジ。開いたレコードを根拠に持つものを上位の画面が渡す。出ない場合は
   * 強調しない。
   */
  highlight?: GraphHighlight | undefined;
  /**
   * 開いたレコードの操作 1 回を表す値。上位の画面がレコードを開くたびに別の値を渡し、端末の割当
   * などで強調を取得し直す間は同じ値を渡す。Detection の強調を開いたレコードの強調へ戻す判定に使う。
   */
  highlightOpening?: object | undefined;
  /**
   * 強調を取得している間と、取得に失敗したときに図の下へ出す通知。上位の画面が取得の状態から
   * 作る。出ない場合は出さない。
   */
  highlightNotice?: ReactNode;
  /**
   * 外のビューで選んだノード。組が変わるたびに、そのノードだけを起点に関係先を出して選ぶ。
   * 出ない場合は何もしない。
   */
  focusRequest?: { node: NodeRef } | undefined;
  /**
   * 表の値から選んだノード。組が変わるたびにそのノードを選ぶ。今の部分グラフに無いノードは、
   * そのノードを起点に関係先を出す。出ない場合は何もしない。
   */
  selectRequest?: { node: NodeRef } | undefined;
  /**
   * 探索 (関係先の表示、親子の連鎖) を抜けて検索の結果へ戻る要求。メニューバーのように本機能の
   * 外の操作が渡し、値が変わるたびに抜ける。
   */
  leaveExplorationRequest?: object | undefined;
  /** 探索を出しているかが変わったときに知らせる。探索を抜ける操作を使えるかを上位が決める。 */
  onChangeExploring?: (exploring: boolean) => void;
  /**
   * 外から適用する本機能の状態。組が変わるたびに、条件・図に出す対象・描画の上限・探索・選択を
   * 置き換える。探索は、そのときの検索の条件で始めたものとする。
   */
  stateRequest?: { state: GraphExploreState } | undefined;
  /** 本機能の状態が変わったときに知らせる。 */
  onChangeState?: (state: GraphExploreState) => void;
};

const noFileNames: ReadonlyMap<string, string> = new Map();

/**
 * 検索の条件と、部分グラフの図と、選んだノードの詳細を 3 つのビューとして出す (操作 9 と
 * 操作 10)。並べ方は layout が決める。
 *
 * 選んでいるノードと起点のノードと親子の連鎖の起点は本機能が持ち、選んだ根拠の
 * レコードは上位の画面が持つ。
 */
export function GraphExplore({
  matchConditions,
  onApplyMatchConditions,
  recordFilter,
  onApplyRecordFilter,
  searchTerms,
  onChangeSearchTerms,
  caseIds,
  onSelectRecord,
  selectedEdgeId,
  onSelectEdge,
  assertions,
  nodeHeaderActions,
  nodeBookmark,
  edgeBookmark,
  dataVersion,
  onShowPane,
  onShowAccountRecords,
  layout,
  sourceFileNames,
  terminals,
  onChangeSelectedNode,
  onSelectNodeInPath,
  highlight: recordHighlight,
  highlightOpening,
  highlightNotice,
  focusRequest,
  selectRequest,
  leaveExplorationRequest,
  onChangeExploring,
  stateRequest,
  onChangeState,
}: GraphExploreProps) {
  // Detection で選んだ Sigma のルールの強調。`over` は選んだ時点の開いたレコードの操作であり、
  // レコードを開き直すと開いたレコードの強調に戻る (最後の操作の強調を表示する)。
  const [ruleHighlight, setRuleHighlight] = useState<{
    rulePath: string;
    highlight: GraphHighlight;
    over: object | undefined;
  }>();
  const highlight =
    ruleHighlight !== undefined && ruleHighlight.over === highlightOpening
      ? ruleHighlight.highlight
      : recordHighlight;
  const [localCriteria, setLocalCriteria] = useState(initialCriteria);
  const [accountRelationRoot, setAccountRelationRoot] = useState<
    NodeRef | undefined
  >(undefined);
  const [selectedAccountRelation, setSelectedAccountRelation] = useState<
    AccountRelationKey | undefined
  >(undefined);
  const [view, setView] = useState<ViewChoice>(autoView);
  const [drawLimit, setDrawLimit] = useState<DrawLimit>(defaultDrawLimit);
  const [layoutSettings, setLayoutSettings] = useState(defaultLayoutSettings);
  const [mergeSameAccount, setMergeSameAccount] = useState(false);
  // 上限を変えても、たどっている親子の連鎖は抜けない。連鎖の要求も同じ上限を読む。
  const changeDrawLimit = (limit: DrawLimit) => setDrawLimit(limit);
  // **探索 (親子の連鎖、関係先) をしている間は、検索の条件と別の要求を出す。** 探索を
  // 抜けると、同じ検索の条件の結果へ戻る。
  //
  // **探索を始めたときの検索の条件を一緒に持ち、条件が変わったら探索を抜ける。** 条件は
  // 元レコードの表示のように本機能の外からも足される。足した条件をグラフに使わないまま
  // 探索を出し続けない。
  //
  // **抜けた探索を捨てずに持つ (`active` が偽)。** 関係先を足した起点は分析者が 1 つずつ
  // 選んだものであり、検索の結果へ戻った後に戻せる。次に関係先を足すと、持っている起点へ足す。
  // 親子の連鎖をたどる間は、それまでの関係先の起点を `kept` に持つ。
  const [explorationStart, setExplorationStart] = useState<
    | {
        exploration: Exploration;
        terms: SearchTerms;
        filter: RecordFilterCriteria;
        active: boolean;
        kept?: Extract<Exploration, { kind: "neighbours" }>;
      }
    | undefined
  >(undefined);
  const exploration =
    explorationStart?.active &&
    explorationStart.terms === searchTerms &&
    explorationStart.filter === recordFilter
      ? explorationStart.exploration
      : undefined;
  // 関係先を出していない間に持っている関係先の起点。
  const suspendedNeighbours =
    explorationStart === undefined || exploration?.kind === "neighbours"
      ? undefined
      : explorationStart.exploration.kind === "neighbours"
        ? explorationStart.exploration
        : explorationStart.kept;
  // 表示中か持っている関係先の起点。
  const neighbourOrigins =
    exploration?.kind === "neighbours" ? exploration : suspendedNeighbours;
  // 影響の経路の起点と、求めている影響の経路。終点を選ぶまで図を変えない。
  const [pathStart, setPathStart] = useState<NodeRef | undefined>(undefined);
  const [influence, setInfluence] = useState<InfluenceEnds | undefined>(
    undefined,
  );
  // **影響の経路の取得と、結果に使った除く根拠の種類を本機能が持つ。** Graph の図と Path の
  // 表が同じ応答を読み、Path のビューを隠しても取得をやり直さない。
  const [excludedBases, setExcludedBases] =
    useState<readonly InfluenceBasis[]>(defaultExcludedBases);
  // 影響の経路の図と表で押した要素の key。
  const [pathClickedKey, setPathClickedKey] = useState<string | undefined>(
    undefined,
  );
  // **図を変える操作は、影響の経路を閉じる。** 影響の経路を出している間、グラフのビューは
  // 探索と検索の結果を描かない。閉じないと、操作した結果が画面に現れない。
  const setExploration = useCallback(
    (
      next: Exploration | undefined,
      kept?: Extract<Exploration, { kind: "neighbours" }>,
    ) => {
      setAccountRelationRoot(undefined);
      setSelectedAccountRelation(undefined);
      setInfluence(undefined);
      setExplorationStart(
        next === undefined
          ? undefined
          : {
              exploration: next,
              terms: searchTerms,
              filter: recordFilter,
              active: true,
              kept,
            },
      );
    },
    [searchTerms, recordFilter],
  );
  // 持っている関係先の起点を捨てる。親子の連鎖をたどっている間は、連鎖の表示を保つ。
  const discardNeighbours = () =>
    setExplorationStart((current) =>
      current?.active && current.exploration.kind === "lineage"
        ? { ...current, kept: undefined }
        : undefined,
    );
  // 探索を抜けて検索の結果へ戻る。関係先の起点は持ち続ける。
  const leaveExploration = useCallback(() => {
    setInfluence(undefined);
    setExplorationStart((current) =>
      current === undefined ? undefined : { ...current, active: false },
    );
  }, []);
  useEffect(() => {
    if (leaveExplorationRequest !== undefined) {
      leaveExploration();
      setAccountRelationRoot(undefined);
    }
  }, [leaveExplorationRequest, leaveExploration]);
  const exploring = exploration !== undefined;
  useEffect(() => {
    onChangeExploring?.(exploring);
  }, [onChangeExploring, exploring]);
  // 関係先を出す起点に node を足す。関係先を出していないときは、抜けた後に持っている起点へ
  // 足す。起点を持っていなければ node だけを起点にする。親子の連鎖から始めた起点は検索の
  // 結果に足さないため、起点が無いときは表示中の探索を渡す。
  //
  // まとめたアカウントのノードは、組の全員を起点に足す。組の全員を探す関数は、応答を読んだ後に
  // 描画ごとに置き換える。
  const originsOfNode = useRef((node: NodeRef): NodeRef[] => [node]);
  const addOrigin = useCallback(
    (node: NodeRef) => {
      setExploration(
        originsOfNode
          .current(node)
          .reduce<Exploration | undefined>(
            withOrigin,
            neighbourOrigins ?? exploration,
          ),
      );
      setSelected({ kind: "node", id: node.id });
    },
    [setExploration, neighbourOrigins, exploration],
  );
  // 手で選んだ種別の組は呼ぶたびに新しい組になるため、useMemo で要求の組の identity を保つ。
  const { hasCondition, eventCategory } = viewInputOf(
    searchTerms,
    recordFilter,
    localCriteria,
  );
  const resolvedView = useMemo(
    () => resolveView(view, { hasCondition, eventCategory }),
    [view, hasCondition, eventCategory],
  );
  const searchView = useMemo(
    () => withOriginKinds(resolvedView, localCriteria.origins),
    [resolvedView, localCriteria.origins],
  );
  // **要求の組を useMemo で保つ。** useSubgraph の effect の依存は組そのものである。
  // レンダーごとに新しい組を作ると、取得 → 状態の更新 → 再レンダー → 新しい組 →
  // 取得のやり直し、という繰り返しになる。
  //
  // **検索の要求と、探索の要求を別の組に持つ。** 詳しい条件の form は検索の要求を書き換える。
  // 探索の要求を form へ渡すと、探索の起点とホップ数が検索の条件へ書き戻される。
  const searchCriteria = useMemo<SubgraphCriteria>(
    () => ({
      ...localCriteria,
      nodeLimit: nodeLimitFor(drawLimit),
      timeFilter: recordFilter.timeFilter,
      eventCategory: recordFilter.eventCategory,
      eventAction: recordFilter.eventAction,
      eventActionFrom: recordFilter.eventActionFrom,
      eventActionTo: recordFilter.eventActionTo,
      caseId: recordFilter.caseId,
      terminal: recordFilter.terminal?.id,
      sources: recordFilter.sources?.map((source) => source.id),
      valueContains: searchTerms.contains,
      valueExcludes: searchTerms.excludes,
      valueField: searchTerms.field,
      fieldContains: searchTerms.fieldContains,
      fieldEquals: searchTerms.fieldEquals,
      searchExpression: searchTerms.expression,
      granularity: searchView.granularity,
      nodeKinds: searchView.nodeKinds,
      // 検索の条件の form が書き戻した nodeIds を送らない。起点は origins だけが持つ。
      nodeIds: localCriteria.origins?.map((origin) => origin.id),
      matchConditions,
    }),
    [
      localCriteria,
      drawLimit,
      recordFilter,
      searchTerms,
      searchView,
      matchConditions,
    ],
  );
  const criteria = useMemo<SubgraphCriteria>(
    () =>
      exploration === undefined
        ? searchCriteria
        : explorationCriteria(
            exploration,
            matchConditions,
            drawLimit,
            localCriteria.edgeKinds ?? [],
            resolvedView.granularity,
            view.kind === "manual" ? view.nodeKinds : undefined,
          ),
    [
      exploration,
      searchCriteria,
      matchConditions,
      drawLimit,
      localCriteria.edgeKinds,
      resolvedView.granularity,
      view,
    ],
  );
  const accountRelationRoles = useMemo(
    () =>
      localCriteria.edgeKinds?.filter((kind) =>
        accountRecordEdgeKinds.includes(kind),
      ),
    [localCriteria.edgeKinds],
  );
  const accountRelationQuery = useAccountRelations(
    accountRelationRoot?.id,
    recordFilter,
    accountRelationRoles,
    selectedAccountRelation,
    matchConditions,
    dataVersion,
  );
  const accountRelations =
    accountRelationRoot === undefined ? undefined : accountRelationQuery.state;
  const [selected, setSelected] = useState<SelectedPoint | undefined>(
    undefined,
  );
  const selectedNodeId = selected?.id;
  // biome-ignore lint/correctness/useExhaustiveDependencies: 外のビューが新しい組を渡したときだけ動かす。
  useEffect(() => {
    if (focusRequest === undefined) return;
    setExploration({
      kind: "neighbours",
      origins: [focusRequest.node],
      showOnly: true,
    });
    setSelected({ kind: "node", id: focusRequest.node.id });
  }, [focusRequest]);
  // 図の上でエッジを選ぶと、詳細のビューに関係の詳細を出す。ノードの詳細を外し、関係の
  // 詳細がビューの先頭に来るようにする。
  const selectEdgeInFigure = (edgeId: string) => {
    setSelected(undefined);
    onSelectEdge(edgeId);
  };
  // 探索中は検索の条件のグラフも取得する。「関係先だけを表示」では図から背景を外し、
  // 関係先の追加と親子の連鎖では背景を図に足す。部分グラフの取得を後に出すため、
  // 背景の hook を先に置く。
  const backdropState = useSubgraph(
    exploration === undefined ? undefined : searchCriteria,
    dataVersion,
  );
  const state = useSubgraph(criteria, dataVersion);
  // 表の値と Detection から選んだノードが今の部分グラフに無いときは、そのノードを起点に関係先を
  // 出して選ぶ。部分グラフに無いノードだけを選ぶと、選択を外す effect が選択を外す。
  const selectOrExplore = (node: NodeRef) => {
    const drawn =
      state.status === "loaded" &&
      state.value.nodes.some((candidate) => candidate.id === node.id);
    if (!drawn)
      setExploration({ kind: "neighbours", origins: [node], showOnly: true });
    setSelected({ kind: "node", id: node.id });
  };
  // biome-ignore lint/correctness/useExhaustiveDependencies: 表の値が新しい組を渡したときだけ動かす。
  useEffect(() => {
    if (selectRequest !== undefined) selectOrExplore(selectRequest.node);
  }, [selectRequest]);
  const backdrop =
    exploration !== undefined &&
    backdropState.status === "loaded" &&
    backdropState.value.emptyReason === undefined &&
    !backdropState.value.nodeLimitExceeded
      ? backdropState.value
      : undefined;
  // 図と Nodes・Edges の表は、まとめた応答を読む。選択を外す判定と表示名はまとめる前の応答を読む。
  const { merge, shownState, shownBackdrop } = useAccountMerge(
    state,
    backdrop,
    mergeSameAccount,
  );
  const shownSelectedId = shownNodeId(merge, selectedNodeId);
  originsOfNode.current = (node) =>
    originsOf(merge, node, state.status === "loaded" ? state.value.nodes : []);
  // **検索式の誤りは、検索の条件の要求の失敗が含む。** 探索の要求は検索式を含まないため、
  // 探索の間は背景の要求の失敗を読む。
  const searchState = exploration === undefined ? state : backdropState;
  const expressionError =
    searchState.status === "failed"
      ? searchState.failure.searchExpressionError
      : undefined;
  // 表示名は今の応答から探す。応答に無いノードは、前に読んだ表示名か外のビューが渡した
  // 表示名を使い、どちらも無ければ識別子を表示名にする。条件を変えて応答が選んだノードを
  // 持たなくなっても、表示名を識別子へ変えない。
  const selectedNodeLabel =
    state.status === "loaded" && selectedNodeId !== undefined
      ? state.value.nodes.find((node) => node.id === selectedNodeId)?.label
      : undefined;
  const knownLabels = useRef(new Map<string, string>());
  const readLabel = selectedNodeLabel?.rawText ?? selectedNodeLabel?.normalized;
  if (selectedNodeId !== undefined && readLabel !== undefined) {
    knownLabels.current.set(selectedNodeId, readLabel);
  }
  for (const request of [focusRequest, selectRequest]) {
    if (request === undefined) continue;
    const { id, label } = request.node;
    if (!knownLabels.current.has(id)) knownLabels.current.set(id, label);
  }
  const selectedNodeText =
    selectedNodeId === undefined
      ? undefined
      : (knownLabels.current.get(selectedNodeId) ?? selectedNodeId);
  useEffect(() => {
    onChangeSelectedNode?.(
      selectedNodeId === undefined || selectedNodeText === undefined
        ? undefined
        : { id: selectedNodeId, label: selectedNodeText },
    );
  }, [onChangeSelectedNode, selectedNodeId, selectedNodeText]);
  // 保存した状態から開いたノードの選択。開いた後の最初の応答を読むまで、図に無くても外さない。
  const restoredSelectionId = useRef<string | undefined>(undefined);
  // biome-ignore lint/correctness/useExhaustiveDependencies: 外の画面が新しい組を渡したときだけ動かす。
  useEffect(() => {
    if (stateRequest === undefined) return;
    const { state: next } = stateRequest;
    setLocalCriteria(next.criteria);
    setView(next.view);
    setDrawLimit(next.drawLimit);
    setMergeSameAccount(next.mergeSameAccount);
    setPathStart(undefined);
    setInfluence(undefined);
    setExplorationStart(
      next.exploration === undefined
        ? undefined
        : { ...next.exploration, terms: searchTerms, filter: recordFilter },
    );
    if (next.selected !== undefined) {
      knownLabels.current.set(next.selected.node.id, next.selected.node.label);
    }
    restoredSelectionId.current = next.selected?.node.id;
    setSelected(
      next.selected === undefined
        ? undefined
        : { kind: "node", id: next.selected.node.id },
    );
  }, [stateRequest]);
  const persistedState = useMemo<GraphExploreState>(
    () => ({
      criteria: persistedCriteria(localCriteria),
      view,
      drawLimit,
      exploration:
        explorationStart === undefined
          ? undefined
          : {
              exploration: explorationStart.exploration,
              active: exploration !== undefined,
              kept: explorationStart.kept,
            },
      selected:
        selectedNodeId === undefined || selectedNodeText === undefined
          ? undefined
          : {
              kind: "node",
              node: { id: selectedNodeId, label: selectedNodeText },
            },
      mergeSameAccount,
    }),
    [
      localCriteria,
      view,
      drawLimit,
      mergeSameAccount,
      explorationStart,
      exploration,
      selectedNodeId,
      selectedNodeText,
    ],
  );
  useEffect(() => {
    onChangeState?.(persistedState);
  }, [onChangeState, persistedState]);
  const eventKinds = useEventKinds(
    matchConditions,
    recordFilter.terminal?.id,
    recordFilter.caseId,
    dataVersion,
  );
  const candidates = useAttackCandidates(criteria, dataVersion);
  const detail = useNodeDetail(selectedNodeId, matchConditions, dataVersion);
  // 図に描くノードの数が上限を超えた応答。図を描かず、件数と絞り方と、合ったノードの一覧を出す。
  const withheld =
    shownState.status === "loaded" && shownState.value.nodeLimitExceeded
      ? shownState.value
      : undefined;
  // 強調するノードとエッジのうち、応答にある数。凡例の横に出す。
  const highlightPairs = useMemo(
    () =>
      state.status === "loaded" && highlight !== undefined ? (
        <HighlightPairs
          counts={highlightCountsOf(state.value, highlight)}
          source={highlightSourceOf(highlight)}
          edges={!state.value.nodeLimitExceeded}
        />
      ) : null,
    [state, highlight],
  );
  const shownHighlight = useMemo(
    () =>
      merge === undefined || highlight === undefined
        ? highlight
        : {
            ...highlight,
            nodeIds: new Set(
              [...highlight.nodeIds].map((id) => shownNodeId(merge, id) ?? id),
            ),
          },
    [merge, highlight],
  );

  // 新しい部分グラフが選んでいるノードを含まないとき、選択を外す。
  useEffect(() => {
    // 影響の経路の図は検索の応答に無いノードを選ばせる。
    if (
      state.status !== "loaded" ||
      selected === undefined ||
      influence !== undefined
    ) {
      return;
    }
    // 保存した状態から開いた選択は別の接続の選択と同じ値であり、開いた後の最初の応答では外さない。
    // useSubgraph は条件が変わった後の応答を読むまで読み込み中を返すので、ここの応答は開いた
    // 状態の条件の応答である。
    if (selected.id === restoredSelectionId.current) {
      restoredSelectionId.current = undefined;
      return;
    }
    if (
      accountRelationRoot === undefined &&
      !state.value.nodes.some((node) => node.id === selected.id)
    ) {
      setSelected(undefined);
    }
  }, [state, selected, influence, accountRelationRoot]);

  // **setter だけを呼び、描画をまたいで同じ関数を保つ。** ノードの一覧は memo であり、
  // 関数が変わると関係を選ぶたびに全行を描き直す。
  const applyCriteria = useCallback(
    (
      next:
        | LocalSubgraphCriteria
        | ((current: LocalSubgraphCriteria) => LocalSubgraphCriteria),
    ) => {
      leaveExploration();
      setLocalCriteria(next);
    },
    [leaveExploration],
  );
  // **関係の種別は探索を抜けずに変える。** 関係先の要求も関係の種別で絞る。
  const applyEdgeKinds = useCallback(
    (edgeKinds: readonly EdgeKind[] | undefined) => {
      setLocalCriteria((current) => ({ ...current, edgeKinds }));
    },
    [],
  );

  // **検索の条件を変えたら、探索の表示を抜ける。** 分析者は条件を足した結果を見るために
  // 条件を変える。
  const changeSearchTerms = (terms: SearchTerms) => {
    setAccountRelationRoot(undefined);
    leaveExploration();
    onChangeSearchTerms(terms);
  };
  const applyRecordFilter = (filter: RecordFilterCriteria) => {
    leaveExploration();
    onApplyRecordFilter(filter);
  };

  const selectNode = useCallback(
    (nodeId: string) => setSelected({ kind: "node", id: nodeId }),
    [],
  );
  // グラフと一覧は識別子だけを渡す。今の応答から表示名を探して、関係先の起点に足す。
  // 一覧は memo であり、応答か起点が変わるときだけ関数を作り直す。
  const addOriginById = useCallback(
    (nodeId: string) => {
      const node =
        state.status === "loaded"
          ? state.value.nodes.find((candidate) => candidate.id === nodeId)
          : undefined;
      addOrigin({
        id: nodeId,
        label: node?.label.rawText ?? node?.label.normalized ?? nodeId,
        kind: node?.kind,
      });
    },
    [state, addOrigin],
  );
  const originNodeIds = useMemo(
    () =>
      exploration?.kind === "neighbours"
        ? exploration.origins.map(
            (origin) => shownNodeId(merge, origin.id) ?? origin.id,
          )
        : [],
    [exploration, merge],
  );

  const traceLineage = useCallback(
    (process: NodeRef) => {
      setExploration({ kind: "lineage", origin: process }, neighbourOrigins);
      setSelected({ kind: "node", id: process.id });
    },
    [setExploration, neighbourOrigins],
  );
  // 「関係先だけを表示」は node だけを起点に始め直す操作であり、持っている起点を捨てる。
  const showNeighbours = useCallback(
    (node: NodeRef) => {
      setExploration({
        kind: "neighbours",
        origins: originsOfNode.current(node).slice(0, maxOriginNodes),
        showOnly: true,
      });
      setSelected({ kind: "node", id: node.id });
    },
    [setExploration],
  );
  const showAccountRecords = useCallback(
    (node: NodeRef) => {
      setLocalCriteria((current) => ({
        ...current,
        edgeKinds: accountRecordEdgeKinds,
      }));
      leaveExploration();
      setAccountRelationRoot(node);
      setSelectedAccountRelation(undefined);
      setSelected({ kind: "node", id: node.id });
      onShowAccountRecords?.(node);
    },
    [onShowAccountRecords, leaveExploration],
  );
  // 外から足す文字列は、欄の指定を外す。別の欄から拾った値を、今の欄だけで判定しない。
  const addTerm = useCallback(
    (kind: SearchTermKind, text: string) => {
      leaveExploration();
      onChangeSearchTerms(withoutField(withTerm(searchTerms, kind, text)));
    },
    [leaveExploration, onChangeSearchTerms, searchTerms],
  );
  const narrowToTerminal = useCallback(
    (terminal: NodeRef) => {
      leaveExploration();
      onApplyRecordFilter({ ...recordFilter, terminal });
    },
    [leaveExploration, onApplyRecordFilter, recordFilter],
  );
  // **描画をまたいで同じ組を保つ。** ノードの一覧は memo であり、関係を選び直すたびに
  // 組が変わると全行を描き直す。
  const nodeMenuActions = useMemo<NodeMenuActions>(
    () => ({
      onOpenDetail: selectNode,
      onAddNeighbours: addOriginById,
      onShowNeighbours: showNeighbours,
      onTraceLineage: traceLineage,
      onNarrowToTerminal: narrowToTerminal,
      onAddTerm: addTerm,
      bookmark: nodeBookmark,
    }),
    [
      selectNode,
      addOriginById,
      showNeighbours,
      traceLineage,
      narrowToTerminal,
      addTerm,
      nodeBookmark,
    ],
  );
  // エッジの一覧も memo であり、同じ理由で組を保つ。
  const edgeMenuActions = useMemo<EdgeMenuActions>(
    () => ({
      onSelectEdge,
      onOpenNodeDetail: selectNode,
      onAddTerm: addTerm,
      bookmark: edgeBookmark,
    }),
    [onSelectEdge, selectNode, addTerm, edgeBookmark],
  );

  const actions: NodeDetailActions = {
    onAddTerm: (text) => addTerm("contains", text),
    onNarrowToTerminal: narrowToTerminal,
    onTraceLineage: traceLineage,
    onShowNeighbours: showNeighbours,
    onShowAccountRecords:
      onShowAccountRecords === undefined ? undefined : showAccountRecords,
    // 関係先を出している間だけ、起点に足す操作を出す。起点にあるノードと、起点が上限に
    // 達した後には出さない。
    onAddNeighbours:
      exploration?.kind === "neighbours" && selectedNodeId !== undefined
        ? hasOrigin(exploration, selectedNodeId) ||
          exploration.origins.length >= maxOriginNodes
          ? undefined
          : addOrigin
        : undefined,
    onMarkPathStart: setPathStart,
    pathStart,
    // 新しい起点と終点では、除く根拠の種類を画面を開いたときの組から始める。
    onShowPath: (to) => {
      if (pathStart !== undefined) {
        setInfluence({ from: pathStart, to });
        setExcludedBases(defaultExcludedBases);
        setPathClickedKey(undefined);
        onShowPane?.("path");
      }
    },
  };
  const pathState = useInfluencePath(
    influence,
    excludedBases,
    matchConditions,
    dataVersion,
  );
  const pathResponse =
    pathState?.status === "loaded" ? pathState.value : undefined;
  const pathSelectedKey = selectedVertexKey(
    pathResponse,
    pathClickedKey,
    selectedNodeId,
  );
  // 図の点と、表と一覧のノードの button が同じ選び方を使う。表示名も覚え、選んだノードが検索の
  // 応答に無くても表示名で出す。
  const selectPathVertex = (key: string, inPathPane = false) => {
    const node =
      pathResponse === undefined ? undefined : vertexNodeRef(pathResponse, key);
    if (node === undefined) return;
    if (inPathPane) onSelectNodeInPath?.(node.id);
    setPathClickedKey(key);
    knownLabels.current.set(node.id, node.label);
    selectNode(node.id);
  };

  const terminalLabel = recordFilter.terminal?.label;
  const sourceNames = useMemo(
    () => distinctSourceLabels(sourceFileNames ?? noFileNames),
    [sourceFileNames],
  );
  const loadedEventKinds =
    eventKinds.status === "loaded" ? eventKinds.value.kinds : [];

  const searchPane = (
    <section aria-labelledby="graph-explore-heading" className="pane-search">
      <h2 id="graph-explore-heading">Search</h2>
      <div className="pane-body">
        {exploration === undefined ? null : (
          <ExplorationStatus
            exploration={exploration}
            onRemoveOrigin={(id) =>
              setExploration(withoutOrigin(exploration, id))
            }
            onLeave={leaveExploration}
          />
        )}
        {suspendedNeighbours === undefined ? null : (
          <div className="kept-origins flex items-center gap-1">
            <KeyValueList
              pairs={[
                {
                  name: "隣接ノードの表示元",
                  value: formatCount(suspendedNeighbours.origins.length),
                },
              ]}
            />
            <IconButton
              label="隣接ノードの表示に戻る"
              onPress={() => setExploration(suspendedNeighbours)}
            >
              <Redo2 size={14} aria-hidden="true" />
            </IconButton>
            <IconButton
              label="追加した隣接ノードを削除"
              onPress={discardNeighbours}
            >
              <Trash2 size={14} aria-hidden="true" />
            </IconButton>
          </div>
        )}
        <SearchConditions
          terms={searchTerms}
          onChangeTerms={changeSearchTerms}
          expressionError={expressionError}
          recordFilter={recordFilter}
          onApplyRecordFilter={applyRecordFilter}
          terminals={terminals}
          sourceFileNames={sourceNames}
          eventKinds={eventKinds}
          view={view}
          resolvedView={resolvedView}
          onChangeView={setView}
          criteria={searchCriteria}
          onApplyCriteria={applyCriteria}
          onApplyEdgeKinds={applyEdgeKinds}
          matchConditions={matchConditions}
          onApplyMatchConditions={onApplyMatchConditions}
          caseIds={caseIds}
          mergeSameAccount={mergeSameAccount}
          onChangeMergeSameAccount={setMergeSameAccount}
        />
        <div className="result-pane">
          <h3>結果</h3>
          <FetchStateView state={state} loadingDescription="グラフの読み込み中">
            {(response) => (
              <SubgraphSummaryPane
                response={response}
                terminalLabel={terminalLabel}
                sourceFileNames={sourceNames}
                eventKinds={loadedEventKinds}
              />
            )}
          </FetchStateView>
          {/* ノードID の条件のノードが取り込み結果に無いと、検索の全体が失敗する。外す条件を示す。 */}
          {state.status === "failed" &&
          state.failure.failureCode === "record_not_found" &&
          exploration === undefined &&
          (localCriteria.origins?.length ?? 0) > 0 ? (
            <div role="note">
              <StatusLabel
                status="failed"
                label="ノードID のノードなし"
                details={
                  <KeyValueList
                    stacked
                    pairs={(localCriteria.origins ?? []).map((origin) => ({
                      key: origin.id,
                      name: "ノードID",
                      value: origin.label === "" ? origin.id : origin.label,
                    }))}
                  />
                }
              />
            </div>
          ) : null}
        </div>
      </div>
    </section>
  );

  const graphPane = (
    <section aria-label="グラフ" className="pane-graph">
      {accountRelations !== undefined ? (
        <>
          <div className="figure-bar">
            <span>相手アカウントの関係</span>
            <IconButton
              label="相手アカウントの表示を終了"
              onPress={() => setAccountRelationRoot(undefined)}
            >
              <X size={14} aria-hidden="true" />
            </IconButton>
          </div>
          <div className="figure">
            <AccountRelationsFigure
              state={accountRelations}
              selected={selectedAccountRelation}
            />
          </div>
        </>
      ) : (
        <>
          <div className="pane-head">
            <DrawLimitSelect value={drawLimit} onChange={changeDrawLimit} />
            <div className="ml-auto">
              <LayoutSettingsPopover
                value={layoutSettings}
                onChange={setLayoutSettings}
              />
            </div>
          </div>
          {influence !== undefined && pathState !== undefined ? (
            <>
              <div className="figure-bar">
                <ul className="value-pairs">
                  <InfluenceEndsPairs ends={influence} />
                </ul>
                <IconButton
                  label="経路の表示を終了"
                  onPress={() => setInfluence(undefined)}
                >
                  <X size={14} aria-hidden="true" />
                </IconButton>
              </div>
              <InfluencePathFigure
                state={pathState}
                settings={layoutSettings}
                selectedKey={pathSelectedKey}
                onSelectVertex={selectPathVertex}
              />
            </>
          ) : (
            <>
              {/* **読み込み中と失敗の通知は結果の欄だけが出す。** 同じ取得の通知を 2 か所に
            出すと、読み上げが同じ失敗を 2 回読む。 */}
              <div className="figure">
                {withheld !== undefined ? (
                  <WithheldFigure
                    response={withheld}
                    onChangeDrawLimit={changeDrawLimit}
                    onShowNodes={
                      onShowPane === undefined
                        ? undefined
                        : () => onShowPane("nodes")
                    }
                  >
                    {highlightPairs}
                  </WithheldFigure>
                ) : shownState.status === "loaded" ? (
                  <SubgraphFigurePane
                    response={shownState.value}
                    settings={layoutSettings}
                    selectedId={shownSelectedId}
                    legendItems={highlightPairs}
                    onSelectNode={selectNode}
                    onSelectEdge={selectEdgeInFigure}
                    onAddOrigin={addOriginById}
                    highlight={shownHighlight}
                    backdrop={
                      exploration?.kind === "neighbours" &&
                      exploration.showOnly === true
                        ? undefined
                        : shownBackdrop
                    }
                    selectedEdgeId={selectedEdgeId}
                    nodeMenuActions={nodeMenuActions}
                    edgeMenuActions={edgeMenuActions}
                    emphasizesResponse={
                      exploration?.kind !== "neighbours" ||
                      exploration.keepsSearch !== true
                    }
                  />
                ) : (
                  <p className="figure-placeholder">
                    {state.status === "loading" ? (
                      <StatusLabel
                        status="running"
                        label="グラフの読み込み中"
                      />
                    ) : (
                      <StatusLabel
                        status="failed"
                        label="描画なし"
                        details="理由: Search の結果"
                      />
                    )}
                  </p>
                )}
              </div>
              {highlightNotice}
            </>
          )}
        </>
      )}
    </section>
  );

  const nodesPane = (
    <NodesPane
      state={shownState}
      criteria={criteria}
      selectedId={shownSelectedId}
      originNodeIds={originNodeIds}
      onSelectNode={selectNode}
      onSetOrigin={addOriginById}
      menuActions={nodeMenuActions}
      summary={
        <>
          {withheld === undefined ? null : (
            <WithheldKindTables
              response={withheld}
              shownKinds={criteria.nodeKinds}
              onShowOnlyKind={(kind) => {
                leaveExploration();
                setView({ kind: "manual", nodeKinds: [kind] });
              }}
            />
          )}
          {state.status === "loaded" && exploration?.kind === "lineage" ? (
            <LineageRoots response={state.value} onSelectNode={selectNode} />
          ) : null}
        </>
      }
    />
  );

  const edgesPane = (
    <EdgesPane
      state={shownState}
      selectedEdgeId={selectedEdgeId}
      onSelectEdge={onSelectEdge}
      menuActions={edgeMenuActions}
    />
  );

  // 規則に一致した結果。tab ごとに分け、Sigma の候補は tab を選んだときに取得を始める。
  // ATT&CK の候補は図と同じ条件で取得しているため、先頭に置く。
  const detectionPane = (
    <section aria-label="Detection" className="view-pane attack-pane">
      <Tabs
        label="規則の種類"
        items={[
          {
            id: "attack",
            label: (
              <>
                ATT&CK
                {candidates.status === "loaded" ? (
                  <span className="tab-count">
                    {formatCount(candidates.value.matches.length)}
                  </span>
                ) : null}
              </>
            ),
            content: (
              <AttackCandidateList
                state={candidates}
                selectedEdgeId={selectedEdgeId}
                onSelectRecord={onSelectRecord}
                sourceFileNames={sourceFileNames}
              />
            ),
          },
          {
            // 最初に選んだ後は、検索の条件が変わるたびに取得し直す。**探索の間も検索の条件を
            // 使う。** 一致のノードから関係先を出しても、一致の一覧は検索の条件の結果のまま保つ。
            id: "sigma",
            label: "Sigma",
            content: (
              <SigmaRuleCandidatePane
                criteria={searchCriteria}
                version={dataVersion}
                onSelectRecord={onSelectRecord}
                onSelectNode={selectOrExplore}
                highlightedRulePath={
                  ruleHighlight !== undefined &&
                  highlight === ruleHighlight.highlight
                    ? ruleHighlight.rulePath
                    : undefined
                }
                onHighlightRule={(rule) =>
                  setRuleHighlight(
                    rule === undefined
                      ? undefined
                      : {
                          rulePath: rule.path,
                          highlight: {
                            nodeIds: new Set(rule.nodeIds),
                            edgeIds: new Set(rule.edgeIds),
                            source: "Sigma のルールに一致したレコード",
                          },
                          over: highlightOpening,
                        },
                  )
                }
              />
            ),
          },
        ]}
      />
    </section>
  );

  // 最初に表示した後は、検索の条件が変わるたびに取得し直す。
  const priorityPane = (
    <section aria-label="Priority" className="view-pane attack-pane">
      <InvestigationOrderPane
        criteria={criteria}
        version={dataVersion}
        onShowNeighbours={actions.onShowNeighbours}
      />
    </section>
  );

  const detailPane = (
    <section aria-labelledby="node-detail-heading" className="pane-detail">
      <h2 id="node-detail-heading">Node Detail</h2>
      <div className="pane-body">
        <NodeDetail
          state={detail}
          onSelectRecord={onSelectRecord}
          assertions={assertions}
          headerActions={nodeHeaderActions}
          relatedValueCounts={(loaded) => (
            <RelatedValueCounts
              key={loaded.node.id}
              detail={loaded}
              matchConditions={matchConditions}
            />
          )}
          actions={actions}
        />
        {accountRelations === undefined ? null : (
          <AccountRelationsDetail
            state={accountRelations}
            selected={selectedAccountRelation}
            onSelect={setSelectedAccountRelation}
            onSelectRecord={onSelectRecord}
            onSelectNode={selectNode}
            onLoadMore={accountRelationQuery.loadMore}
            loadingMore={accountRelationQuery.loadingMore}
            moreFailure={accountRelationQuery.moreFailure}
            sourceFileNames={sourceFileNames}
          />
        )}
        <MergedAccountSection
          enabled={mergeSameAccount}
          node={detail.status === "loaded" ? detail.value.node : undefined}
          merge={merge}
          nodes={state.status === "loaded" ? state.value.nodes : []}
        />
      </div>
    </section>
  );

  const pathPane = (
    <InfluencePathView
      // 起点と終点ごとに、選び途中の除く根拠の種類を捨てる。
      key={
        influence === undefined
          ? undefined
          : `${influence.from.id}\u0000${influence.to.id}`
      }
      ends={influence}
      state={pathState}
      excluded={excludedBases}
      onApplyExcluded={setExcludedBases}
      selectedKey={pathSelectedKey}
      onSelectVertex={(key) => selectPathVertex(key, true)}
      onSelectRecord={onSelectRecord}
      mergeSameAccount={mergeSameAccount}
    />
  );

  return layout({
    search: searchPane,
    graph: graphPane,
    detail: detailPane,
    nodes: nodesPane,
    edges: edgesPane,
    detection: detectionPane,
    priority: priorityPane,
    path: pathPane,
  });
}
