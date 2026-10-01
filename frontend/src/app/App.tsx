import {
  ArrowLeft,
  ArrowRight,
  Bookmark as BookmarkIcon,
  FilterX,
  Sparkles,
  X,
} from "lucide-react";
import {
  type ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { Assertions } from "@/features/assertions/Assertions";
import { useAssertions } from "@/features/assertions/useAssertions";
import { AssistConversation } from "@/features/assistConversation/AssistConversation";
import { useAssistConversation } from "@/features/assistConversation/useAssistConversation";
import { AssistPermissions } from "@/features/assistPermissions/AssistPermissions";
import { useAssistPermissions } from "@/features/assistPermissions/useAssistPermissions";
import {
  AssistProposalList,
  AssistProposalsForTarget,
} from "@/features/assistProposals/AssistProposals";
import { useAssistProposals } from "@/features/assistProposals/useAssistProposals";
import { EdgeDetail } from "@/features/graphExplore/EdgeDetail";
import {
  GraphExplore,
  type GraphExploreState,
} from "@/features/graphExplore/GraphExplore";
import { distinctSourceLabels } from "@/features/graphExplore/labels";
import { UrlFragmentJoinView } from "@/features/graphExplore/UrlFragmentJoin";
import { useEdgeDetail } from "@/features/graphExplore/useEdgeDetail";
import { useRecordGraph } from "@/features/graphExplore/useRecordGraph";
import { useTerminalList } from "@/features/graphExplore/useTerminalList";
import {
  viewInputOf,
  withOriginKinds,
} from "@/features/graphExplore/viewInput";
import { MemberRoles } from "@/features/members/MemberRoles";
import { Bookmarks } from "@/features/navigation/BookmarksView";
import { BookmarkToggle } from "@/features/navigation/BookmarkToggle";
import {
  type BookmarkTarget,
  bookmarkTitle,
  edgeBookmarkKey,
  edgeBookmarkOf,
  isBookmarked,
  type NewBookmark,
  nodeLabelTextOf,
  placeBookmarkOf,
  sourceBookmarkOf,
  timelineBookmarkOf,
} from "@/features/navigation/bookmarks";
import {
  emptyHistory,
  move,
  type Place,
  type PlaceHistory,
  placeKey,
  relabelCurrent,
  visit,
} from "@/features/navigation/places";
import { useBookmarks } from "@/features/navigation/useBookmarks";
import { NodeSummaryList } from "@/features/nodeSummaries/NodeSummaryList";
import { RecordContext } from "@/features/recordContext/RecordContext";
import {
  RecordDetail,
  type TrailOrigin,
} from "@/features/recordDetail/RecordDetail";
import { RecordNumbers } from "@/features/recordNumbers/RecordNumbers";
import {
  inventoryCaseIds,
  type SourceInventory as SourceInventoryValue,
} from "@/features/sourceInventory/inventory";
import { SourceEventKinds } from "@/features/sourceInventory/SourceEventKinds";
import { SourceIncludeToggles } from "@/features/sourceInventory/SourceIncludeToggles";
import { SourceInventory } from "@/features/sourceInventory/SourceInventory";
import { useSourceInventory } from "@/features/sourceInventory/useSourceInventory";
import {
  type InterpretedRangeState,
  TerminalAssignments,
} from "@/features/terminalAssignments/TerminalAssignments";
import { useTerminalAssignments } from "@/features/terminalAssignments/useTerminalAssignments";
import { TerminalDetail } from "@/features/terminals/TerminalDetail";
import { TerminalList } from "@/features/terminals/TerminalList";
import { sourceInterpretationsOf } from "@/features/timeInterpretation/sourceInterpretations";
import { TimeInterpretation } from "@/features/timeInterpretation/TimeInterpretation";
import {
  type TimeInterpretationsView,
  useTimeInterpretations,
} from "@/features/timeInterpretation/useTimeInterpretations";
import { TimeHistogram } from "@/features/timeline/TimeHistogram";
import { Timeline } from "@/features/timeline/Timeline";
import {
  defaultTimelineNodeScope,
  TimelineNodeScope,
} from "@/features/timeline/TimelineNodeScope";
import {
  type AssistTurnContext,
  assistMatchConditionsOf,
} from "@/shared/api/assistRelay";
import type {
  GraphTimeBound,
  GraphTimeFilter,
  NodeRef,
  RecordFilterCriteria,
  SourceRef,
} from "@/shared/api/graph";
import {
  everyMatchCondition,
  type MatchConditionSelection,
} from "@/shared/api/matchConditions";
import type { NodeSummariesRequest } from "@/shared/api/nodeSummaries";
import {
  type AssertionRecordRef,
  type AssertionTarget,
  assertionRecordRefOf,
} from "@/shared/contracts/assertions";
import type {
  AssistProposalAdoption,
  AssistProposalItem,
} from "@/shared/contracts/assistProposals";
import type { AssistOrigin } from "@/shared/contracts/assistRelay";
import type { CaseId } from "@/shared/contracts/cases";
import type { RecordLocator } from "@/shared/contracts/common";
import type { GraphEdge, GraphNode } from "@/shared/contracts/graph";
import type { EdgeEvidenceSelector } from "@/shared/contracts/graphDetail";
import type { RecordResponse } from "@/shared/contracts/records";
import type { SearchQuery } from "@/shared/contracts/searchQuery";
import type { SourceIdentity } from "@/shared/contracts/sources";
import type { TimelineEntry } from "@/shared/contracts/timeline";
import type { FetchState } from "@/shared/lib/fetchState";
import type { GraphConditions } from "@/shared/lib/graphConditions";
import { windowsEventIdOf } from "@/shared/lib/recordField";
import { recordPositionParts } from "@/shared/lib/recordPosition";
import { searchHighlightOf } from "@/shared/lib/searchHighlight";
import {
  noSearchTerms,
  type SearchTerms,
  withFieldTerm,
  withoutExpression,
  withoutField,
  withTerm,
} from "@/shared/lib/searchTerms";
import { resolveView } from "@/shared/lib/searchView";
import type { ValueCondition } from "@/shared/lib/valueCondition";
import {
  DisplayOffsetContext,
  DisplayOffsetSelect,
} from "@/shared/ui/DisplayOffset";
import { FetchFailureNotice } from "@/shared/ui/FetchFailureNotice";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { SearchHighlightContext } from "@/shared/ui/Highlighted";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList, type KeyValuePair } from "@/shared/ui/KeyValueList";
import { type MenuEntry, menuSeparator } from "@/shared/ui/Menu";
import { MenuBar, type MenuBarMenu } from "@/shared/ui/MenuBar";
import { RawText } from "@/shared/ui/RawText";
import { useSignedIn } from "@/shared/ui/SignedInAccount";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import { Tabs } from "@/shared/ui/Tabs";
import { ThemeToggle } from "@/shared/ui/ThemeToggle";
import {
  type ValueActions,
  ValueActionsContext,
  type ValueTarget,
} from "@/shared/ui/ValueLink";
import { AccountBar } from "./AccountBar";
import {
  type DockPanel,
  type DockReveal,
  DockWorkspace,
} from "./DockWorkspace";
import { searchQueryOf, searchStateOf } from "./searchState";
import { fileNamesByContentOf } from "./sourceFileNames";
import type { WorkspaceState } from "./workspaceState";

/** 選んだ収集元の取り込めなかったレコードの件数。一覧を読めていない間は出さない。 */
function failedRecordCountOf(
  inventory: FetchState<SourceInventoryValue>,
  source: SourceIdentity,
): number | undefined {
  if (inventory.status !== "loaded") return undefined;
  const row = inventory.value.rows.find(
    (candidate) => candidate.source.sourceId === source.sourceId,
  );
  return row?.importStatus.counts.find((count) => count.category === "failed")
    ?.count;
}

/** 収集元の一覧を読めていない間の案件の選択肢。描画ごとに別の配列を作らない。 */
const noCaseIds: readonly CaseId[] = [];
const noSources: readonly SourceRef[] = [];

/**
 * 作業場所に置くビューの識別子と名前。既定の配置はこの並びで置き、メニューバーの「表示」も
 * この並びで出す。
 */
const workspaceViews = [
  { id: "search", title: "Search", keepsMounted: true },
  { id: "graph", title: "Graph", keepsMounted: true },
  { id: "detail", title: "Node Detail", keepsMounted: true },
  // エッジの詳細と影響の経路は、既定の配置に置かず、Node Detail の区画に開く。
  {
    id: "edgeDetail",
    title: "Edge Detail",
    keepsMounted: true,
    column: "detail",
  },
  // 未適用の除く根拠の種類の選択を、タブを切り替えても保つ。
  { id: "path", title: "Path", keepsMounted: true, column: "detail" },
  { id: "record", title: "Record", keepsMounted: true },
  { id: "nodes", title: "Nodes" },
  { id: "edges", title: "Edges" },
  { id: "timeline", title: "Timeline" },
  { id: "sources", title: "Artifacts" },
  { id: "terminals", title: "Hosts" },
  { id: "terminal", title: "Host Detail" },
  { id: "ips", title: "IPs" },
  { id: "histogram", title: "Histogram" },
  // 節の開閉と、開いた後に取得した候補を、タブを切り替えても保つ。
  { id: "detection", title: "Detection", keepsMounted: true },
  { id: "priority", title: "Priority", keepsMounted: true },
  { id: "bookmarks", title: "Bookmarks", keepsMounted: true },
  { id: "recording", title: "Time & Host", keepsMounted: true },
  // **会話と送信の許可のビューは隠れても描き続ける。** 応答の途中の流れと、書きかけの発言と
  // 分析者の名前を保つ。
  { id: "assist", title: "AI Chat", keepsMounted: true },
  { id: "assistPermissions", title: "AI Permissions", keepsMounted: true },
  { id: "proposals", title: "AI Proposals" },
] as const satisfies readonly Omit<DockPanel, "content">[];

type WorkspaceViewId = (typeof workspaceViews)[number]["id"];

/** ワークスペースを選ぶ画面を渡した起動だけに置くビュー。 */
const workspacesView = {
  id: "workspaces",
  title: "Workspaces",
  keepsMounted: true,
} as const satisfies Omit<DockPanel, "content">;

/** 管理者だけに出すビュー。管理者でない利用者にも、保存した配置を開けるよう隠して置く。 */
const membersView = {
  id: "members",
  title: "Members",
} as const satisfies Omit<DockPanel, "content">;

/** 文字列の条件を 1 つでも持つか。含む文字列・含まない文字列・欄と文字列の組・文字列を照合する欄を見る。 */
function hasSearchTerms(terms: SearchTerms): boolean {
  return (
    terms.contains.length > 0 ||
    terms.excludes.length > 0 ||
    (terms.fieldContains?.length ?? 0) > 0 ||
    (terms.fieldEquals?.length ?? 0) > 0 ||
    terms.field !== undefined
  );
}

/**
 * 選んだ収集元の、時刻の解釈で読んだ観測期間の状態を組む。
 *
 * **一覧を取り直している間は、前の一覧の期間を使わない。** 解釈を記録した直後の前の一覧は、
 * 記録した解釈で読んだ期間を持たない。主張中の解釈が 2 件以上ある収集元は、backend が
 * どのずれでも読まないため、期間を持たない理由を分けて出す。
 */
function interpretedRangeStateOf(
  inventory: FetchState<SourceInventoryValue>,
  refreshing: boolean,
  interpretations: TimeInterpretationsView["state"],
  selectedSource: SourceIdentity | undefined,
): InterpretedRangeState {
  if (inventory.status === "failed" || inventory.status === "empty") {
    return { status: "failed" };
  }
  if (inventory.status !== "loaded" || refreshing) {
    return { status: "loading" };
  }
  if (selectedSource === undefined) {
    return { status: "loaded", range: undefined };
  }
  if (
    interpretations.status === "loaded" &&
    interpretations.value.get(selectedSource.contentSha256)?.kind ===
      "conflicted"
  ) {
    return { status: "conflicted" };
  }
  return {
    status: "loaded",
    range: inventory.value.rows.find(
      (row) => row.source.sourceId === selectedSource.sourceId,
    )?.interpretedObservedRange,
  };
}

/** グラフの探索が知らせた状態から、図の要求だけが読む検索の条件を取り出す。 */
function graphConditionsOf(state: GraphExploreState): GraphConditions {
  const { criteria } = state;
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

/** AI 支援に開いているレコードの説明を求める発言。 */
const explainRecordText =
  "開いているレコードが何を記録しているかを、根拠の参照を添えて説明してください。";

/**
 * 画面を組み立て、収集元と関連付けの起点と開いたレコードと見ているプロセスの選択を保持する。
 *
 * 検索・グラフ・詳細・開いたレコード・全件の時系列・収集元の各ビューを、分析者が自由に
 * 配置できる作業場所に置く。
 *
 * **関連付けの起点と、原文を開いたレコードを別の状態にする。** 候補の一覧から原文を開く
 * 操作で起点が変わると、出していた候補集合が通知なしに別の起点の結果に置き換わる。
 *
 * 画面の状態を `WorkspaceState` にまとめ、変わるたびに onWorkspaceStateChange へ知らせる。
 * workspaceState の revision が変わるたびに、その状態を画面へ適用する。
 * workspaceBar は上部の帯に、workspacePanel は作業場所の「ワークスペース」のビューに出す。
 */
export function App({
  workspaceState,
  onWorkspaceStateChange,
  workspaceBar,
  workspacePanel,
}: {
  workspaceState?: { state: WorkspaceState; revision: number };
  onWorkspaceStateChange?: (state: WorkspaceState) => void;
  workspaceBar?: ReactNode;
  workspacePanel?: ReactNode;
} = {}) {
  // 選んだ収集元は sourceId で持ち、収集元の一覧から探す。一覧に無い収集元は選んでいない。
  const [selectedSourceId, setSelectedSourceId] = useState<string | undefined>(
    undefined,
  );
  // レコードの番号の欄で比べる相手の収集元。欄は収集元のビューが前面にあるときだけ描くため、
  // 隠しても選択が残るように本画面が持つ。空文字列は比べないことを表す。
  const [comparedSourceId, setComparedSourceId] = useState("");
  // 画面全体で時刻に添える地方時のずれ。空文字列は UTC だけで出すことを表す。
  const [displayOffset, setDisplayOffset] = useState("");
  // 開いたレコードと、関連付けの候補から開いたときの起点のレコード。起点があるとき、
  // 開いたレコードの欄は起点からの経路を出す。
  const [openedRecord, setOpenedRecord] = useState<
    { ref: RecordLocator; origin?: RecordLocator } | undefined
  >(undefined);
  const openedRecordRef = openedRecord?.ref;
  const [selectedEdgeId, setSelectedEdgeId] = useState<string | undefined>(
    undefined,
  );
  const [evidenceGroupSelector, setEvidenceGroupSelector] = useState<
    EdgeEvidenceSelector | undefined
  >(undefined);
  // **根拠のレコードを絞る条件を 1 か所で持つ。** グラフの探索と時系列が同じレコードの
  // 集合を絞るため、条件を 2 つ持つと 2 つの画面が別の集合を出す。
  const [recordFilter, setRecordFilter] = useState<RecordFilterCriteria>({});
  // **検索の文字列を 1 か所で持つ。** グラフの探索とノードの詳細と元レコードの表示が、
  // 同じ条件へ値を足す。
  const [searchTerms, setSearchTerms] = useState<SearchTerms>(noSearchTerms);
  // **候補を絞る条件の選択を 1 か所で持つ。** グラフの探索と時系列とノードの詳細と
  // 関係の詳細が同じ選択を読む。選択を 2 つ持つと、ノードの詳細が候補を出しながら
  // 関係の詳細が同じ関係を見つけられない状態が出る。
  //
  // 初期の選択は契約が定めるすべての条件を幅 0 で用いる。
  const [matchConditions, setMatchConditions] =
    useState<MatchConditionSelection>(everyMatchCondition);
  // 前面に出すビュー。レコードや関係を選んだときに、その詳細のビューを出す。
  const [reveal, setReveal] = useState<DockReveal | undefined>(undefined);
  // メニューバーから作業場所とグラフの探索へ渡す要求。値を替えるたびに 1 回動く。
  const [layoutResetRequest, setLayoutResetRequest] = useState<
    object | undefined
  >(undefined);
  const [leaveExplorationRequest, setLeaveExplorationRequest] = useState<
    object | undefined
  >(undefined);
  // グラフが探索 (関係先の表示、親子の連鎖) を出しているか。状態の所有者はグラフの探索である。
  const [exploring, setExploring] = useState(false);
  // グラフで選んでいるノードと、時系列をそのノードの近くに絞る設定。グラフと時系列が
  // 同じ選択を読むため、本画面が持つ。
  const [selectedNode, setSelectedNode] = useState<NodeRef | undefined>(
    undefined,
  );
  const [nodeScope, setNodeScope] = useState(defaultTimelineNodeScope);
  const [accountHistory, setAccountHistory] = useState<
    { node: NodeRef; before?: GraphTimeBound } | undefined
  >(undefined);

  const timeInterpretations = useTimeInterpretations(matchConditions);
  // **収集元の一覧を 1 か所で取得する。** 収集元の一覧の表と、グラフの探索の案件の
  // 選択肢と、端末の割当の適用期間が同じ一覧を読む。時刻の解釈を記録すると、解釈で読んだ
  // 観測期間が変わるため取り直す。
  const { state: sourceInventory, refreshing: inventoryRefreshing } =
    useSourceInventory(timeInterpretations.recordedCount);
  const selectedRow = useMemo(
    () =>
      selectedSourceId === undefined || sourceInventory.status !== "loaded"
        ? undefined
        : sourceInventory.value.rows.find(
            (row) => row.source.sourceId === selectedSourceId,
          ),
    [sourceInventory, selectedSourceId],
  );
  const selectedSource = selectedRow?.source;
  const interpretedRange = useMemo(
    () =>
      interpretedRangeStateOf(
        sourceInventory,
        inventoryRefreshing,
        timeInterpretations.state,
        selectedSource,
      ),
    [
      sourceInventory,
      inventoryRefreshing,
      timeInterpretations.state,
      selectedSource,
    ],
  );
  const caseIds = useMemo(
    () =>
      sourceInventory.status === "loaded"
        ? inventoryCaseIds(sourceInventory.value)
        : noCaseIds,
    [sourceInventory],
  );
  const inventorySources = useMemo(
    () =>
      sourceInventory.status === "loaded"
        ? sourceInventory.value.rows.map((row) => row.source)
        : [],
    [sourceInventory],
  );
  // 時系列は memo であり、描画ごとに別の配列を渡すと取り直す。
  const timelineSources = useMemo(
    () => recordFilter.sources?.map((source) => source.id),
    [recordFilter.sources],
  );
  // 端末と IP アドレスの一覧と件数の分布の要求。時系列と同じ条件 (検索式を含む) で絞る。
  const searchExpression = searchTerms.expression;
  const nodeSummaryFilter = useMemo(
    () => ({
      matchConditions,
      timeFilter: recordFilter.timeFilter,
      eventCategory: recordFilter.eventCategory,
      eventAction: recordFilter.eventAction,
      eventActionFrom: recordFilter.eventActionFrom,
      eventActionTo: recordFilter.eventActionTo,
      caseId: recordFilter.caseId,
      terminal: recordFilter.terminal?.id,
      sources: timelineSources,
      searchExpression,
    }),
    [matchConditions, recordFilter, timelineSources, searchExpression],
  );
  // 各ビューで強調する検索条件。検索条件か期間が変わったときに値を作り直す。
  const searchHighlight = useMemo(
    () => searchHighlightOf(searchTerms, recordFilter.timeFilter),
    [searchTerms, recordFilter.timeFilter],
  );
  // 端末の一覧で選んだ端末。端末の詳細のビューが読む。
  const [selectedTerminalId, setSelectedTerminalId] = useState<
    string | undefined
  >(undefined);
  const selectTerminal = useCallback((node: GraphNode) => {
    setSelectedTerminalId(node.id);
    setReveal({ panelId: "terminal" });
  }, []);
  const ipSummaries = useMemo<NodeSummariesRequest>(
    () => ({ ...nodeSummaryFilter, nodeKind: "ip" }),
    [nodeSummaryFilter],
  );
  // 一覧で選んだノードをグラフへ渡す。同じノードを選び直しても、新しい組で関係先を出し直す。
  const [focusRequest, setFocusRequest] = useState<
    { node: NodeRef } | undefined
  >(undefined);
  const focusNode = useCallback((node: NodeRef) => {
    setFocusRequest({ node });
    setReveal({ panelId: "graph" });
  }, []);
  // 見た場所の履歴。開いたレコードとグラフで選んだノードが変わるたびに足す。戻る・進むで
  // 移った場所は今の場所と同じため、足し直さない。
  const [history, setHistory] = useState<PlaceHistory>(emptyHistory);
  // 適用した画面の状態が持つ開いたレコードと選んだノード。適用した履歴に既にあるため足さない。
  const restoredPlaces = useRef<{
    record?: RecordLocator;
    nodeId?: string;
  }>({});
  useEffect(() => {
    if (
      openedRecordRef !== undefined &&
      openedRecordRef !== restoredPlaces.current.record
    ) {
      setHistory((current) =>
        visit(current, { kind: "record", ref: openedRecordRef }),
      );
    }
  }, [openedRecordRef]);
  // 読み込んだレコードの Event ID。開いたレコードの見出しと、今の場所の表示名に入れる。
  const [loadedEventId, setLoadedEventId] = useState<{
    placeKey: string;
    eventId: string | undefined;
  }>();
  const labelLoadedRecord = useCallback((record: RecordResponse) => {
    const place: Place = {
      kind: "record",
      ref: record.recordRef,
      eventId: windowsEventIdOf(record.fields),
    };
    setLoadedEventId({ placeKey: placeKey(place), eventId: place.eventId });
    setHistory((current) => relabelCurrent(current, place));
  }, []);
  // 開いたレコードの見出しの「名前: 値」の組。Event ID は読み込んだレコードのものだけを使う。
  const openedRecordPairs = (ref: RecordLocator): KeyValuePair[] => {
    const eventId =
      loadedEventId?.placeKey === placeKey({ kind: "record", ref })
        ? loadedEventId.eventId
        : undefined;
    return [
      { name: "Event ID", value: eventId },
      { name: "収集元", value: <RawText text={ref.sourceFileName} /> },
      ...recordPositionParts(ref).map((part) => ({
        name: part.name,
        value: <span className="font-mono">{part.value}</span>,
      })),
    ];
  };
  useEffect(() => {
    if (
      selectedNode !== undefined &&
      selectedNode.id !== restoredPlaces.current.nodeId
    ) {
      restoredPlaces.current.nodeId = undefined;
      setHistory((current) =>
        visit(current, { kind: "node", node: selectedNode }),
      );
    }
  }, [selectedNode]);
  // **ノードを選び直したら Node Detail を前面に出す。** Edge Detail と Path は同じ区画に重なる。
  // 適用した画面の状態が持つノードは、保存した配置の前面のビューを保つ。Path の表で選んだノードは、
  // Path を前面に保つ。
  const selectedNodeId = selectedNode?.id;
  const keepFrontNodeId = useRef<string | undefined>(undefined);
  const keepFrontViewFor = useCallback((nodeId: string) => {
    keepFrontNodeId.current = nodeId;
  }, []);
  useEffect(() => {
    if (
      selectedNodeId !== undefined &&
      selectedNodeId !== keepFrontNodeId.current
    ) {
      setReveal({ panelId: "detail" });
    }
    keepFrontNodeId.current = undefined;
  }, [selectedNodeId]);
  const {
    bookmarks,
    setBookmarks,
    isMarkedKey,
    toggle: toggleBookmark,
  } = useBookmarks();
  // 行メニューは開いたときに付けてあるかを読むため、一覧が変わっても組を替えない。
  const rowBookmarks = useMemo(
    () => ({
      timeline: {
        isMarked: (entry: TimelineEntry) =>
          isMarkedKey(placeKey({ kind: "record", ref: entry.recordRef })),
        onToggle: (entry: TimelineEntry) =>
          toggleBookmark(timelineBookmarkOf(entry)),
      },
      node: {
        isMarked: (nodeId: string) =>
          isMarkedKey(
            placeKey({ kind: "node", node: { id: nodeId, label: "" } }),
          ),
        onToggle: (node: NodeRef) =>
          toggleBookmark(placeBookmarkOf({ kind: "node", node })),
      },
      edge: {
        isMarked: (edgeId: string) => isMarkedKey(edgeBookmarkKey(edgeId)),
        onToggle: (edge: GraphEdge, sourceLabel: string, targetLabel: string) =>
          toggleBookmark(edgeBookmarkOf(edge, sourceLabel, targetLabel)),
      },
    }),
    [isMarkedKey, toggleBookmark],
  );
  const bookmarkToggle = (bookmark: NewBookmark) => (
    <BookmarkToggle
      name={bookmarkTitle(bookmark.target)}
      marked={isBookmarked(bookmarks, bookmark.target)}
      onToggle={() => toggleBookmark(bookmark)}
    />
  );
  // グラフの探索が知らせた状態と、グラフの探索と作業場所へ適用する状態。
  const [graphState, setGraphState] = useState<GraphExploreState | undefined>(
    undefined,
  );
  const [graphStateRequest, setGraphStateRequest] = useState<
    { state: GraphExploreState } | undefined
  >(undefined);
  const [layoutRequest, setLayoutRequest] = useState<
    { layout: unknown } | undefined
  >(undefined);
  // biome-ignore lint/correctness/useExhaustiveDependencies: revision が変わったときだけ適用する。
  useEffect(() => {
    if (workspaceState === undefined) return;
    const { state } = workspaceState;
    restoredPlaces.current = {
      record: state.openedRecord?.ref,
      nodeId:
        state.graph.selected?.kind === "node"
          ? state.graph.selected.node.id
          : undefined,
    };
    keepFrontNodeId.current = restoredPlaces.current.nodeId;
    setLayoutRequest({ layout: state.dockLayout });
    // 保存した配置が Edge Detail を持たないときは、選んだエッジの詳細を開く。
    if (state.selectedEdgeId !== undefined) {
      setReveal({ panelId: "edgeDetail", onlyIfClosed: true });
    }
    setRecordFilter(state.recordFilter);
    setComparedSourceId(state.comparedSourceId);
    setSelectedSourceId(state.selectedSourceId);
    setSearchTerms(state.searchTerms);
    setMatchConditions(state.matchConditions);
    setOpenedRecord(state.openedRecord);
    setSelectedEdgeId(state.selectedEdgeId);
    setEvidenceGroupSelector(state.evidenceGroupSelector);
    setNodeScope(state.nodeScope);
    setHistory(state.history);
    setBookmarks(state.bookmarks);
    setGraphStateRequest({ state: state.graph });
  }, [workspaceState?.revision]);
  // 配置は分割の幅を動かす間も変わり続けるため、画面を描き直さずに持つ。
  const dockLayout = useRef<unknown>(undefined);
  const notifyWorkspaceState = useRef(onWorkspaceStateChange);
  notifyWorkspaceState.current = onWorkspaceStateChange;
  const stateWithoutLayout = useMemo(
    () =>
      graphState === undefined
        ? undefined
        : {
            recordFilter,
            comparedSourceId,
            selectedSourceId,
            searchTerms,
            matchConditions,
            openedRecord,
            selectedEdgeId,
            evidenceGroupSelector,
            nodeScope,
            history,
            bookmarks,
            graph: graphState,
          },
    [
      recordFilter,
      comparedSourceId,
      selectedSourceId,
      searchTerms,
      matchConditions,
      openedRecord,
      selectedEdgeId,
      evidenceGroupSelector,
      nodeScope,
      history,
      bookmarks,
      graphState,
    ],
  );
  const latestState = useRef(stateWithoutLayout);
  const notifyState = useCallback(() => {
    const rest = latestState.current;
    if (rest === undefined || dockLayout.current === undefined) return;
    notifyWorkspaceState.current?.({ dockLayout: dockLayout.current, ...rest });
  }, []);
  useEffect(() => {
    latestState.current = stateWithoutLayout;
    notifyState();
  }, [stateWithoutLayout, notifyState]);
  const changeLayout = useCallback(
    (layout: unknown) => {
      dockLayout.current = layout;
      notifyState();
    },
    [notifyState],
  );
  const currentPlace = history.places[history.index];
  const openPlace = (place: Place) => {
    if (place.kind === "record") {
      setOpenedRecord({ ref: place.ref });
      setReveal({ panelId: "record" });
    } else {
      focusNode(place.node);
    }
  };
  const moveInHistory = (step: -1 | 1) => {
    const next = move(history, step);
    const place = next.places[next.index];
    if (next === history || place === undefined) return;
    setHistory(next);
    openPlace(place);
  };
  // 件数の分布は期間を外した全体を数える。
  //
  // **期間を除いた項目だけで組を作り直す。** 期間を含む条件の組から作ると、件数の分布で期間を
  // 適用するたびに同じ条件で取り直し、読み込み中の表示を挟んで範囲の欄が初めの値に戻る。
  const histogramTerminal = recordFilter.terminal?.id;
  const histogramRequest = useMemo(
    () => ({
      matchConditions,
      eventCategory: recordFilter.eventCategory,
      eventAction: recordFilter.eventAction,
      eventActionFrom: recordFilter.eventActionFrom,
      eventActionTo: recordFilter.eventActionTo,
      caseId: recordFilter.caseId,
      terminal: histogramTerminal,
      sources: timelineSources,
      searchExpression,
    }),
    [
      matchConditions,
      recordFilter.eventCategory,
      recordFilter.eventAction,
      recordFilter.eventActionFrom,
      recordFilter.eventActionTo,
      recordFilter.caseId,
      histogramTerminal,
      timelineSources,
      searchExpression,
    ],
  );
  const applyTimeFilter = useCallback(
    (timeFilter: GraphTimeFilter | undefined) =>
      setRecordFilter((current) => ({ ...current, timeFilter })),
    [],
  );
  const sourceFileNames = useMemo(
    () =>
      new Map(
        sourceInventory.status === "loaded"
          ? sourceInventory.value.rows.map(
              (row) => [row.source.sourceId, row.source.fileName] as const,
            )
          : [],
      ),
    [sourceInventory],
  );

  // 収集元ごとの今の時刻の解釈。時刻の解釈の一覧を正とし、解釈を持たない収集元は起動で指定した
  // ずれを使う。
  const sourceInterpretations = useMemo(
    () =>
      sourceInterpretationsOf(
        sourceInventory.status === "loaded" ? sourceInventory.value.rows : [],
        timeInterpretations.state.status === "loaded"
          ? timeInterpretations.state.value
          : new Map(),
      ),
    [sourceInventory, timeInterpretations.state],
  );

  const sourceFileNamesByContent = useMemo(
    () =>
      fileNamesByContentOf(
        sourceInventory.status === "loaded"
          ? sourceInventory.value.rows.map((row) => row.source)
          : [],
      ),
    [sourceInventory],
  );

  const terminalAssignments = useTerminalAssignments();
  // 関係を足す AI 提案を採用した回数。
  const [addedRelationCount, setAddedRelationCount] = useState(0);
  // **割当と時刻の解釈を記録した回数と、関係を足す AI 提案を採用した回数で、グラフを読む画面を
  // 取り直す。** 割当と時刻の解釈を記録した時点で、backend はグラフのノードとエッジと時系列の
  // 並びを組み直す。
  const dataVersion =
    terminalAssignments.recordedCount +
    timeInterpretations.recordedCount +
    addedRelationCount;
  // 端末で絞る条件の選択肢。グラフの探索と AI 支援の card が同じ一覧を読む。子の effect は親の
  // effect より先に走るため、分析者が最初に待つ図の取得が先に始まる。
  const terminals = useTerminalList(
    matchConditions,
    dataVersion,
    sourceFileNamesByContent,
  );
  const assistPermissions = useAssistPermissions();
  const conversation = useAssistConversation();
  const edgeDetail = useEdgeDetail(
    selectedEdgeId,
    evidenceGroupSelector,
    matchConditions,
    recordFilter.caseId,
    dataVersion,
  );
  const assertions = useAssertions(matchConditions);
  // **採用の応答の所見を、所見の一覧に足す。** 別の操作が先に採用していたときは、所見の一覧を
  // 取り直す。採用で関係を足す所見ができたときは、グラフを読む画面を取り直す。別の操作の採用では
  // 所見を読めていないため、提案が関係を足す提案であるかで決める。
  const { add: addAssertion, reload: reloadAssertions } = assertions;
  const adoptionHandlers = useMemo(
    () => ({
      onAdopted: (adoption: AssistProposalAdoption) => {
        addAssertion(adoption.assertion);
        if (adoption.assertion.assertion.addsRelation === true) {
          setAddedRelationCount((count) => count + 1);
        }
      },
      onAdoptedElsewhere: (item: AssistProposalItem) => {
        reloadAssertions();
        if (item.proposal.addsRelation) {
          setAddedRelationCount((count) => count + 1);
        }
      },
    }),
    [addAssertion, reloadAssertions],
  );
  const assistProposals = useAssistProposals(matchConditions, adoptionHandlers);
  // **開いたレコードを根拠に持つノードとエッジを、グラフで強調する。** 時系列や一覧で
  // レコードを選ぶと、そのレコードが図のどこにあたるかを示す。
  const recordGraph = useRecordGraph(
    openedRecordRef,
    matchConditions,
    dataVersion,
  );
  const highlight = useMemo(
    () =>
      recordGraph.status === "loaded"
        ? {
            nodeIds: new Set(recordGraph.value.nodeIds),
            edgeIds: new Set(recordGraph.value.edgeIds),
          }
        : undefined,
    [recordGraph],
  );
  // 強調を取得している間と失敗したときは、強調が無いことと区別できるように知らせる。
  const highlightNotice =
    recordGraph.status === "loading" ? (
      <p role="status" className="highlight-status">
        <StatusLabel status="running" label="強調の読み込み中" />
      </p>
    ) : recordGraph.status === "failed" ? (
      <div className="highlight-status">
        <FetchFailureNotice failure={recordGraph.failure} />
      </div>
    ) : undefined;

  // 起点から開いたレコードへの経路を求める起点。条件の選択とグラフの組み直しで取り直す。
  const openedOrigin = openedRecord?.origin;
  const trailOrigin = useMemo<TrailOrigin | undefined>(
    () =>
      openedOrigin === undefined
        ? undefined
        : { origin: openedOrigin, matchConditions, dataVersion },
    [openedOrigin, matchConditions, dataVersion],
  );

  // **描画をまたいで同じ関数を保つ。** 関連付けの一覧は memo である。
  const selectRecord = useCallback(
    (ref: RecordLocator, origin?: RecordLocator) => {
      setOpenedRecord({ ref, origin });
      setReveal({ panelId: "record" });
    },
    [],
  );
  // 表の値から選んだノードと、Timeline の移動先の時刻。選んだノードの状態の所有者はグラフの
  // 探索であり、本画面は選ぶ要求だけを渡す。移動先の時刻は Timeline が使った後に外す。
  const [selectRequest, setSelectRequest] = useState<
    { node: NodeRef } | undefined
  >(undefined);
  const [timelineSeek, setTimelineSeek] = useState<{ utc: string } | undefined>(
    undefined,
  );
  const clearTimelineSeek = useCallback(() => setTimelineSeek(undefined), []);
  const showPane = useCallback(
    (panelId: WorkspaceViewId) => setReveal({ panelId }),
    [],
  );
  // 前面のビューを変えずにレコードを開く。時系列で一致へ移る操作は、時系列を見たまま
  // 開いたレコードとグラフの強調だけを変える。
  const previewRecord = useCallback(
    (ref: RecordLocator) => setOpenedRecord({ ref }),
    [],
  );

  // **関係を選び直したら、区分の選択を外す。** 前の関係の区分で、別の関係の根拠を
  // 絞ったままにしない。
  //
  // **描画をまたいで同じ関数を保つ。** エッジの一覧は memo であり、関数が変わると
  // 関係の詳細の取得の状態が変わるたびに全行を描き直す。
  const selectEdge = useCallback((edgeId: string) => {
    setSelectedEdgeId(edgeId);
    setEvidenceGroupSelector(undefined);
    setReveal({ panelId: "edgeDetail" });
  }, []);
  // **値から作った条件を、条件の種類ごとの状態へ入れる。** 外から足す文字列は欄の指定を外す。
  // 別の欄から拾った値を、今の欄だけで判定しない。欄と文字列の組は欄の指定に左右されないため、
  // 欄の指定を残す。
  const addCondition = useCallback((condition: ValueCondition) => {
    switch (condition.kind) {
      case "text":
        setSearchTerms((current) =>
          withoutField(withTerm(current, condition.mode, condition.text)),
        );
        return;
      case "field":
        setSearchTerms((current) =>
          withFieldTerm(
            current,
            condition.field,
            condition.text,
            condition.whole,
          ),
        );
        return;
      case "eventKind":
        setRecordFilter((current) => ({
          ...current,
          eventCategory: condition.category,
          // 動作が空の組は、動作で絞らない分類の条件である。
          eventAction: condition.action === "" ? undefined : condition.action,
        }));
        return;
      case "period":
        setRecordFilter((current) => ({
          ...current,
          timeFilter: condition.filter,
        }));
        return;
      default: {
        const unreachable: never = condition;
        throw new Error(`unknown condition: ${JSON.stringify(unreachable)}`);
      }
    }
  }, []);
  // **表の値の操作。** 値を扱うビューを前面に出し、値から作った条件を検索の条件に追加する。
  // 表の行は memo であり、描画をまたいで同じ組を保つ。
  const valueActions = useMemo<ValueActions>(
    () => ({
      open: (target: ValueTarget) => {
        switch (target.kind) {
          case "node":
            setSelectRequest({ node: { id: target.id, label: target.label } });
            setReveal({ panelId: "detail" });
            return;
          case "edge":
            selectEdge(target.id);
            return;
          case "record":
            selectRecord(target.ref);
            return;
          case "time":
            setTimelineSeek({ utc: target.utc });
            setReveal({ panelId: "timeline" });
            return;
          default: {
            const unreachable: never = target;
            throw new Error(`unknown value: ${JSON.stringify(unreachable)}`);
          }
        }
      },
      addCondition,
    }),
    [selectEdge, selectRecord, addCondition],
  );

  // **収集元の一覧に無い収集元は開けない。** 選んでも一覧から探せず、何も出ない。一覧を
  // 読み込む前は開き、読み込んだ後の一覧から探す。エッジは Edge Detail が取得の失敗を出す。
  const openBookmark = (target: BookmarkTarget): boolean => {
    switch (target.kind) {
      case "edge":
        selectEdge(target.edge.id);
        return true;
      case "source":
        if (
          sourceInventory.status === "loaded" &&
          !sourceInventory.value.rows.some(
            (row) => row.source.sourceId === target.source.id,
          )
        ) {
          return false;
        }
        setSelectedSourceId(target.source.id);
        setReveal({ panelId: "sources" });
        return true;
      default:
        openPlace(target);
        return true;
    }
  };

  // **描画をまたいで同じ関数を保つ。** 時系列は memo である。
  const narrowToTerminal = useCallback(
    (terminal: NodeRef) =>
      setRecordFilter((current) => ({ ...current, terminal })),
    [],
  );
  const narrowToSource = useCallback(
    (source: SourceRef) =>
      setRecordFilter((current) => ({ ...current, sources: [source] })),
    [],
  );

  // AI 支援の card の収集元に付ける表示名。検索欄の収集元の選択肢と同じ名前を使う。
  const sourceLabels = useMemo(
    () => distinctSourceLabels(sourceFileNames),
    [sourceFileNames],
  );
  // **AI 支援の card の条件を、検索の条件のすべてへ 1 回で適用する。** 検索の文字列と根拠の
  // レコードを絞る条件は本画面が持ち、図の条件と図に出す対象の選び方はグラフの探索が持つ。
  // グラフの探索へは状態の適用として渡し、探索の表示を抜ける。返す操作は、適用する前の条件と
  // 探索の表示へ戻す。
  const applySearchQuery = (
    query: SearchQuery,
    origins: readonly AssistOrigin[] | undefined,
  ) => {
    const previousTerms = searchTerms;
    const previousFilter = recordFilter;
    const previousGraph = graphState;
    const next = searchStateOf(
      query,
      origins,
      terminals.status === "loaded" ? terminals.value : [],
      sourceLabels,
    );
    setSearchTerms(next.searchTerms);
    setRecordFilter(next.recordFilter);
    if (graphState !== undefined) {
      setGraphStateRequest({
        state: {
          ...graphState,
          criteria: { ...graphState.criteria, ...next.graphConditions },
          view: next.view,
          exploration:
            graphState.exploration === undefined
              ? undefined
              : { ...graphState.exploration, active: false },
        },
      });
    }
    return () => {
      setSearchTerms(previousTerms);
      setRecordFilter(previousFilter);
      if (previousGraph !== undefined) {
        setGraphStateRequest({ state: previousGraph });
      }
    };
  };
  const assistMatchConditions = useMemo(
    () => assistMatchConditionsOf(matchConditions),
    [matchConditions],
  );
  // 発言に添える画面の文脈。送る操作の時点の値を読む。グラフの探索が状態を知らせる前は、
  // 検索の条件を添えない。
  //
  // **図に出す対象は、グラフの探索と同じ手順で決めた種別を添える。** AI が読むグラフが、画面の
  // グラフと同じ種別で絞られる。検索の条件から決めているかも添え、AI は図に出す対象を変えない
  // card で種別を省く。
  const turnContext = (
    records: RecordLocator[] | undefined = openedRecordRef === undefined
      ? undefined
      : [openedRecordRef],
  ): AssistTurnContext => ({
    matchConditions: assistMatchConditions,
    case: recordFilter.caseId,
    ...(graphState === undefined
      ? {}
      : {
          searchQuery: searchQueryOf(
            {
              searchTerms,
              recordFilter,
              graphConditions: graphConditionsOf(graphState),
            },
            withOriginKinds(
              resolveView(
                graphState.view,
                viewInputOf(searchTerms, recordFilter, graphState.criteria),
              ),
              graphState.criteria.origins,
            ),
          ),
          nodeKindsFromConditions:
            graphState.view.kind === "auto" ? true : undefined,
        }),
    records,
    nodeIds: selectedNode === undefined ? undefined : [selectedNode.id],
    edgeIds: selectedEdgeId === undefined ? undefined : [selectedEdgeId],
  });
  const canAskAssist =
    conversation.relay.status === "available" &&
    !conversation.sending &&
    !conversation.answering;
  const explainRecord = (record: RecordLocator) => {
    setReveal({ panelId: "assist" });
    void conversation.send(explainRecordText, turnContext([record]));
  };

  // 所見に添えるレコード。対象が出している根拠に、分析者が開いたレコードを足す。
  //
  // **対象の根拠に入らないレコードを、分析者が根拠に挙げられるようにする。** 遠隔の
  // セッションの手段を読む材料は、その関係の根拠の外にもある (例: 接続先の端末での
  // `wsmprovhost.exe` の起動)。レコードを開くのは分析者の操作であり、取り込みが根拠を
  // 増やすわけではない。
  const assertionRecordRefs = (evidence: RecordLocator[]) => {
    const refs = evidence.map(assertionRecordRefOf);
    if (openedRecordRef === undefined) {
      return refs;
    }
    const opened = assertionRecordRefOf(openedRecordRef);
    const already = refs.some(
      (ref) =>
        ref.sourceContentSha256 === opened.sourceContentSha256 &&
        ref.sequenceNumber === opened.sequenceNumber &&
        ref.lineNumber === opened.lineNumber &&
        ref.byteOffset === opened.byteOffset,
    );
    return already ? refs : [...refs, opened];
  };

  // 対象 1 つへの分析者のメモと AI 提案を並べて出す。2 つの feature は App を通して連動する。
  const targetJudgements = (
    target: AssertionTarget,
    recordRefs: AssertionRecordRef[],
  ) => (
    <>
      <Assertions target={target} recordRefs={recordRefs} view={assertions} />
      <AssistProposalsForTarget
        target={target}
        view={assistProposals}
        matchConditions={matchConditions}
      />
    </>
  );

  const edgePane = (
    <section aria-label="エッジの詳細" className="pane-detail">
      {edgeDetail.status === "unselected" ? (
        <p className="note">未選択</p>
      ) : null}
      <EdgeDetail
        state={edgeDetail}
        selectedSelector={evidenceGroupSelector}
        onSelectGroup={setEvidenceGroupSelector}
        onSelectRecord={selectRecord}
        sourceFileNames={sourceFileNames}
        sourceInterpretations={sourceInterpretations}
        headerActions={(detail) =>
          bookmarkToggle(
            edgeBookmarkOf(
              detail.edge,
              nodeLabelTextOf(detail.sourceNode),
              nodeLabelTextOf(detail.targetNode),
            ),
          )
        }
        urlFragments={(detail) => (
          <UrlFragmentJoinView
            key={detail.edge.id}
            edgeId={detail.edge.id}
            caseId={recordFilter.caseId}
            matchConditions={matchConditions}
            onSelectRecord={selectRecord}
          />
        )}
        assertions={(detail) =>
          targetJudgements(
            {
              kind: "edge",
              edge: {
                kind: detail.edge.kind,
                sourceNodeId: detail.edge.sourceNodeId,
                targetNodeId: detail.edge.targetNodeId,
              },
            },
            assertionRecordRefs(
              detail.edge.evidence.map((item) => item.recordRef),
            ),
          )
        }
      />
    </section>
  );

  const recordPane =
    openedRecordRef === undefined ? (
      <div className="note flex items-center gap-1">
        <KeyValueList pairs={[{ name: "レコード", value: "未選択" }]} />
        <HelpPopover label="レコードの選択">
          Graph・Timeline・Edges のレコード
        </HelpPopover>
      </div>
    ) : (
      <section aria-labelledby="record-sheet-heading" className="record-sheet">
        <div className="sheet-head">
          <h2 id="record-sheet-heading">Record</h2>
          <KeyValueList
            className="sheet-position"
            pairs={openedRecordPairs(openedRecordRef)}
          />
          {bookmarkToggle(
            placeBookmarkOf({ kind: "record", ref: openedRecordRef }),
          )}
          {conversation.relay.status === "available" ? (
            <IconButton
              label="AI にレコードの説明を依頼"
              isDisabled={!canAskAssist}
              disabledReason={{
                title: "AI の応答中",
                text: "応答の完了を待機",
              }}
              onPress={() => explainRecord(openedRecordRef)}
            >
              <Sparkles size={14} aria-hidden="true" />
            </IconButton>
          ) : null}
          <IconButton
            label="レコードを閉じる"
            onPress={() => setOpenedRecord(undefined)}
          >
            <X size={14} aria-hidden="true" />
          </IconButton>
        </div>
        <div className="sheet-body">
          <div className="sheet-record">
            <RecordDetail
              recordRef={openedRecordRef}
              trailOrigin={trailOrigin}
              onRecordLoad={labelLoadedRecord}
            />
            {targetJudgements(
              { kind: "record", record: assertionRecordRefOf(openedRecordRef) },
              [assertionRecordRefOf(openedRecordRef)],
            )}
          </div>
          <section
            aria-labelledby="record-context-heading"
            className="sheet-context"
          >
            <h2 id="record-context-heading">前後のレコード</h2>
            <RecordContext
              recordRef={openedRecordRef}
              matchConditions={matchConditions}
              dataVersion={dataVersion}
            />
          </section>
        </div>
      </section>
    );

  // **収集元のビューは前面に出ているときだけ描く。** レコードの番号の抜けの範囲と片方にしか
  // 無いレコードの一覧は数百の行になる。
  const sourcesPane = (
    <div className="view-pane">
      <KeyValueList
        pairs={[
          {
            name: "選択中の収集元",
            value:
              selectedSource === undefined ? (
                "未選択"
              ) : (
                <span className="inline-flex items-center gap-1">
                  <RawText text={selectedSource.fileName} />
                  {bookmarkToggle(sourceBookmarkOf(selectedSource))}
                </span>
              ),
          },
        ]}
      />
      <Tabs
        label="Artifacts の節"
        items={[
          {
            id: "inventory",
            label: "収集元",
            content: (
              <>
                <SourceIncludeToggles
                  sources={inventorySources}
                  included={recordFilter.sources ?? noSources}
                  onChange={(sources) =>
                    setRecordFilter((current) => ({ ...current, sources }))
                  }
                />
                <SourceInventory
                  state={sourceInventory}
                  selectedSource={selectedSource}
                  onSelect={(source) => setSelectedSourceId(source.sourceId)}
                  onSelectRecord={selectRecord}
                  terminalAssignments={terminalAssignments.state}
                  narrowedSourceIds={timelineSources}
                  onNarrowToSource={narrowToSource}
                />
              </>
            ),
          },
          {
            id: "eventKinds",
            label: "イベントの種類",
            content:
              selectedSource === undefined ? (
                <p className="note">未選択</p>
              ) : (
                <SourceEventKinds
                  source={selectedSource}
                  failedRecordCount={failedRecordCountOf(
                    sourceInventory,
                    selectedSource,
                  )}
                  matchConditions={matchConditions}
                  dataVersion={dataVersion}
                />
              ),
          },
          {
            id: "recordNumbers",
            label: "レコードの番号",
            content: (
              <RecordNumbers
                selectedSource={selectedSource}
                sources={inventorySources}
                comparedSourceId={comparedSourceId}
                onSelectComparedSource={setComparedSourceId}
                dataVersion={dataVersion}
                onSelectRecord={selectRecord}
              />
            ),
          },
        ]}
      />
    </div>
  );

  // **記録の入力欄を持つビューは隠れても描き続ける。** 分析者は開いたレコードのビューと
  // 行き来して根拠を足す。描くのをやめると入力途中の値が消える。
  const recordingPane = (
    <div className="view-pane pane-sections">
      <div className="flex items-center gap-1">
        <KeyValueList
          pairs={[
            {
              name: "記録する収集元",
              value:
                selectedSource === undefined ? (
                  "未選択"
                ) : (
                  <RawText text={selectedSource.fileName} />
                ),
            },
          ]}
        />
        <HelpPopover label="記録する収集元">
          Artifacts で選択中の収集元
        </HelpPopover>
      </div>
      <TimeInterpretation
        view={timeInterpretations}
        selectedSource={selectedSource}
        openedRecordRef={openedRecordRef}
        sourceFileNamesByContent={sourceFileNamesByContent}
        importTimeOffset={selectedRow?.importTimeOffset}
      />
      <TerminalAssignments
        view={terminalAssignments}
        selectedSource={selectedSource}
        interpretedRange={interpretedRange}
        basisRecordRef={openedRecordRef}
        sourceFileNames={sourceFileNames}
        sourceInterpretations={sourceInterpretations}
      />
    </div>
  );

  // **時系列は前面に出ているときだけ描く。** 全件は 1 万件を超え、取得と描画に時間がかかる。
  const timelinePane = (
    <div className="view-pane">
      {accountHistory === undefined ? (
        <TimelineNodeScope
          node={selectedNode}
          value={nodeScope}
          onChange={setNodeScope}
        />
      ) : (
        <div className="flex items-center gap-2">
          <KeyValueList
            pairs={[
              {
                name: "アカウント",
                value: <RawText text={accountHistory.node.label} />,
              },
            ]}
          />
          <IconButton
            label="通常の時系列へ戻る"
            onPress={() => setAccountHistory(undefined)}
          >
            <ArrowLeft size={14} aria-hidden="true" />
          </IconButton>
          {accountHistory.before === undefined ? null : (
            <IconButton
              label="時刻の絞り込みを解除"
              onPress={() =>
                setAccountHistory((current) =>
                  current === undefined ? undefined : { node: current.node },
                )
              }
            >
              <FilterX size={14} aria-hidden="true" />
            </IconButton>
          )}
        </div>
      )}
      <Timeline
        key={accountHistory?.node.id ?? "normal"}
        matchConditions={matchConditions}
        timeFilter={
          accountHistory === undefined
            ? recordFilter.timeFilter
            : accountHistory.before === undefined
              ? undefined
              : {
                  to: accountHistory.before,
                  unit: accountHistory.before.precision,
                }
        }
        eventCategory={
          accountHistory === undefined ? recordFilter.eventCategory : undefined
        }
        eventAction={
          accountHistory === undefined ? recordFilter.eventAction : undefined
        }
        eventActionFrom={
          accountHistory === undefined
            ? recordFilter.eventActionFrom
            : undefined
        }
        eventActionTo={
          accountHistory === undefined ? recordFilter.eventActionTo : undefined
        }
        caseId={recordFilter.caseId}
        terminal={
          accountHistory === undefined ? recordFilter.terminal?.id : undefined
        }
        sources={accountHistory === undefined ? timelineSources : undefined}
        valueContains={
          accountHistory === undefined ? searchTerms.contains : undefined
        }
        valueExcludes={
          accountHistory === undefined ? searchTerms.excludes : undefined
        }
        valueField={
          accountHistory === undefined ? searchTerms.field : undefined
        }
        fieldContains={
          accountHistory === undefined ? searchTerms.fieldContains : undefined
        }
        fieldEquals={
          accountHistory === undefined ? searchTerms.fieldEquals : undefined
        }
        searchExpression={
          accountHistory === undefined ? searchTerms.expression : undefined
        }
        nearNodeId={
          accountHistory === undefined && nodeScope.enabled
            ? selectedNode?.id
            : undefined
        }
        accountNodeId={accountHistory?.node.id}
        onBefore={
          accountHistory === undefined
            ? undefined
            : (entry) => {
                const timestamp = entry.eventTime;
                if (
                  timestamp?.normalizedForm !== "rfc3339_absolute" ||
                  timestamp.normalized === undefined ||
                  (timestamp.precision !== "second" &&
                    timestamp.precision !== "millisecond" &&
                    timestamp.precision !== "microsecond")
                )
                  return;
                const before: GraphTimeBound = {
                  text: timestamp.normalized,
                  precision: timestamp.precision,
                };
                setAccountHistory((current) =>
                  current === undefined
                    ? undefined
                    : {
                        node: current.node,
                        before,
                      },
                );
              }
        }
        nearDepth={nodeScope.depth}
        onSelectRecord={selectRecord}
        onPreviewRecord={previewRecord}
        onNarrowToTerminal={narrowToTerminal}
        onNarrowToSource={narrowToSource}
        rowBookmark={rowBookmarks.timeline}
        dataVersion={dataVersion}
        seek={timelineSeek}
        onSeekDone={clearTimelineSeek}
      />
    </div>
  );

  // 利用者と役割のビューは管理者だけに出す。
  const isAdmin = useSignedIn()?.role === "admin";
  // メニューバーの「表示」は、置いたうえで隠していないビューを出す。
  const shownViews: readonly Omit<DockPanel, "content">[] = [
    ...workspaceViews,
    ...(workspacePanel === undefined ? [] : [workspacesView]),
    ...(isAdmin ? [membersView] : []),
  ];

  const ipListPane = (
    <div className="view-pane">
      <NodeSummaryList
        request={ipSummaries}
        version={dataVersion}
        selectedNodeId={selectedNode?.id}
        fileNamesByContent={sourceFileNamesByContent}
      />
    </div>
  );

  const menuItem = (
    key: string,
    label: string,
    onSelect: () => void,
    disabled = false,
  ): MenuEntry => ({ kind: "item", key, label, onSelect, disabled });
  const appMenus: MenuBarMenu[] = [
    {
      key: "view",
      label: "表示",
      entries: [
        ...shownViews.map((view) =>
          menuItem(`view-${view.id}`, view.title, () =>
            setReveal({ panelId: view.id }),
          ),
        ),
        menuSeparator("layout"),
        menuItem("reset-layout", "配置を初期化", () =>
          setLayoutResetRequest({}),
        ),
      ],
    },
    {
      key: "search",
      label: "検索",
      entries: [
        menuItem(
          "clear-expression",
          "検索式を解除",
          () => setSearchTerms(withoutExpression),
          searchTerms.expression === undefined,
        ),
        // 検索式は別の項目で外す。文字列の条件を外しても、適用している検索式は残す。
        menuItem(
          "clear-terms",
          "文字列の条件をすべて解除",
          () =>
            setSearchTerms((current) =>
              current.expression === undefined
                ? noSearchTerms
                : { ...noSearchTerms, expression: current.expression },
            ),
          !hasSearchTerms(searchTerms),
        ),
        menuItem(
          "clear-period",
          "期間のフィルタを解除",
          () => applyTimeFilter(undefined),
          recordFilter.timeFilter === undefined,
        ),
        menuItem(
          "leave-exploration",
          "探索を終了",
          () => setLeaveExplorationRequest({}),
          !exploring,
        ),
      ],
    },
    {
      key: "move",
      label: "移動",
      entries: [
        menuItem("back", "戻る", () => moveInHistory(-1), history.index <= 0),
        menuItem(
          "forward",
          "進む",
          () => moveInHistory(1),
          history.index >= history.places.length - 1,
        ),
        menuSeparator("record"),
        menuItem(
          "close-record",
          "レコードを閉じる",
          () => setOpenedRecord(undefined),
          openedRecordRef === undefined,
        ),
      ],
    },
    {
      key: "record",
      label: "記録",
      entries: [
        menuItem("recording", "Time & Host を表示", () =>
          setReveal({ panelId: "recording" }),
        ),
      ],
    },
  ];

  return (
    <DisplayOffsetContext.Provider value={displayOffset}>
      <ValueActionsContext value={valueActions}>
        <main className="app">
          <header className="appbar">
            <h1>Oraculum</h1>
            <MenuBar label="画面の操作" menus={appMenus} />
            <nav aria-label="表示の履歴" className="appbar-nav">
              <IconButton
                label="戻る"
                isDisabled={history.index <= 0}
                disabledReason={{
                  title: "戻る履歴なし",
                  text: "レコードの表示かノードの選択で記録",
                }}
                onPress={() => moveInHistory(-1)}
              >
                <ArrowLeft size={16} aria-hidden="true" />
              </IconButton>
              <IconButton
                label="進む"
                isDisabled={history.index >= history.places.length - 1}
                disabledReason={{
                  title: "進む履歴なし",
                  text: "戻る操作の後に記録",
                }}
                onPress={() => moveInHistory(1)}
              >
                <ArrowRight size={16} aria-hidden="true" />
              </IconButton>
              {currentPlace === undefined ? (
                <IconButton
                  label="今の場所をブックマーク"
                  isDisabled
                  disabledReason={{
                    title: "今の場所なし",
                    text: "レコードの表示かノードの選択で記録",
                  }}
                >
                  <BookmarkIcon size={16} aria-hidden="true" />
                </IconButton>
              ) : (
                bookmarkToggle(placeBookmarkOf(currentPlace))
              )}
            </nav>
            {workspaceBar}
            <DisplayOffsetSelect
              value={displayOffset}
              onChange={setDisplayOffset}
            />
            <AccountBar />
            <ThemeToggle />
          </header>
          <GraphExplore
            matchConditions={matchConditions}
            onApplyMatchConditions={setMatchConditions}
            recordFilter={recordFilter}
            onApplyRecordFilter={setRecordFilter}
            searchTerms={searchTerms}
            onChangeSearchTerms={setSearchTerms}
            caseIds={caseIds}
            onSelectRecord={selectRecord}
            selectedEdgeId={selectedEdgeId}
            onSelectEdge={selectEdge}
            onChangeSelectedNode={setSelectedNode}
            onSelectNodeInPath={keepFrontViewFor}
            highlight={highlight}
            highlightOpening={openedRecord}
            highlightNotice={highlightNotice}
            focusRequest={focusRequest}
            selectRequest={selectRequest}
            leaveExplorationRequest={leaveExplorationRequest}
            onChangeExploring={setExploring}
            stateRequest={graphStateRequest}
            onChangeState={setGraphState}
            dataVersion={dataVersion}
            sourceFileNames={sourceFileNames}
            terminals={terminals}
            assertions={(detail) =>
              targetJudgements(
                { kind: "node", nodeId: detail.node.id },
                assertionRecordRefs(
                  detail.evidence.map((item) => item.recordRef),
                ),
              )
            }
            nodeHeaderActions={(detail) =>
              bookmarkToggle(
                placeBookmarkOf({
                  kind: "node",
                  node: {
                    id: detail.node.id,
                    label: nodeLabelTextOf(detail.node),
                    kind: detail.node.kind,
                  },
                }),
              )
            }
            nodeBookmark={rowBookmarks.node}
            edgeBookmark={rowBookmarks.edge}
            onShowPane={showPane}
            onShowAccountRecords={(node) => {
              setAccountHistory({ node });
              showPane("timeline");
            }}
            layout={(panes) => {
              const contents: Record<WorkspaceViewId, ReactNode> = {
                search: panes.search,
                graph: panes.graph,
                detail: panes.detail,
                edgeDetail: edgePane,
                path: panes.path,
                record: recordPane,
                nodes: panes.nodes,
                edges: panes.edges,
                detection: panes.detection,
                priority: panes.priority,
                timeline: timelinePane,
                sources: sourcesPane,
                terminals: (
                  <div className="view-pane">
                    <TerminalList
                      matchConditions={matchConditions}
                      version={dataVersion}
                      selectedTerminalId={selectedTerminalId}
                      onSelectTerminal={selectTerminal}
                      sourceFileNames={sourceFileNamesByContent}
                    />
                  </div>
                ),
                terminal: (
                  <div className="view-pane">
                    <TerminalDetail
                      terminalId={selectedTerminalId}
                      matchConditions={matchConditions}
                      version={dataVersion}
                      onSelectRecord={selectRecord}
                      onSelectNode={focusNode}
                      onApplyDisplayOffset={setDisplayOffset}
                      onOpenHosts={() => setReveal({ panelId: "terminals" })}
                    />
                  </div>
                ),
                ips: ipListPane,
                bookmarks: (
                  <div className="view-pane">
                    <Bookmarks
                      onOpen={openBookmark}
                      bookmarks={bookmarks}
                      onChangeBookmarks={setBookmarks}
                    />
                  </div>
                ),
                histogram: (
                  <div className="view-pane">
                    <TimeHistogram
                      request={histogramRequest}
                      version={dataVersion}
                      timeFilter={recordFilter.timeFilter}
                      onApplyTimeFilter={applyTimeFilter}
                      fileNamesByContent={sourceFileNamesByContent}
                    />
                  </div>
                ),
                recording: recordingPane,
                assist: (
                  <div className="view-pane">
                    <AssistConversation
                      view={conversation}
                      turnContext={() => turnContext()}
                      currentMatchConditions={assistMatchConditions}
                      terminals={terminals}
                      onApplySearchQuery={applySearchQuery}
                    />
                  </div>
                ),
                assistPermissions: (
                  <div className="view-pane">
                    <AssistPermissions view={assistPermissions} />
                  </div>
                ),
                proposals: (
                  <AssistProposalList
                    view={assistProposals}
                    matchConditions={matchConditions}
                  />
                ),
              };
              return (
                <DockWorkspace
                  reveal={reveal}
                  resetRequest={layoutResetRequest}
                  layoutRequest={layoutRequest}
                  onLayoutChange={changeLayout}
                  panels={[
                    ...workspaceViews.map((view) => ({
                      ...view,
                      content: (
                        <SearchHighlightContext.Provider
                          value={searchHighlight}
                        >
                          {contents[view.id]}
                        </SearchHighlightContext.Provider>
                      ),
                      // 左右の列は幅の狭い一覧なので、中央の Graph と下の区画と Path だけを最大にできる。
                      maximizable:
                        view.id !== "search" &&
                        view.id !== "detail" &&
                        view.id !== "edgeDetail",
                    })),
                    ...(workspacePanel === undefined
                      ? []
                      : [
                          {
                            ...workspacesView,
                            content: (
                              <div className="view-pane">{workspacePanel}</div>
                            ),
                          },
                        ]),
                    {
                      ...membersView,
                      content: isAdmin ? (
                        <MemberRoles />
                      ) : (
                        <div className="view-pane">
                          <KeyValueList
                            pairs={[{ name: "必要な役割", value: "管理者" }]}
                          />
                        </div>
                      ),
                      hidden: !isAdmin,
                    },
                  ]}
                />
              );
            }}
          />
        </main>
      </ValueActionsContext>
    </DisplayOffsetContext.Provider>
  );
}
