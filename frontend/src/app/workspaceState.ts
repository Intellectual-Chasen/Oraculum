import type {
  GraphExploreCriteria,
  GraphExploreSelection,
  GraphExploreState,
} from "@/features/graphExplore/GraphExplore";
import {
  type Bookmark,
  type BookmarkTarget,
  bookmarkOrigins,
  bookmarkTimeKinds,
} from "@/features/navigation/bookmarks";
import type { Place, PlaceHistory } from "@/features/navigation/places";
import type { TimelineNodeScopeValue } from "@/features/timeline/TimelineNodeScope";
import {
  type GraphTimeBound,
  type GraphTimeFilter,
  maxGraphDepth,
  maxOriginNodes,
  minGraphDepth,
  type NodeRef,
  type RecordFilterCriteria,
  type SourceRef,
} from "@/shared/api/graph";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import { conditionKeys } from "@/shared/contracts/candidates";
import { decodeCaseId } from "@/shared/contracts/cases";
import {
  decodeRecordLocator,
  decodeTimestamp,
  type RecordLocator,
} from "@/shared/contracts/common";
import {
  DecodeFailure,
  type Decoder,
  decodeString,
  optionalArray,
  optionalBoolean,
  optionalCount,
  optionalEnum,
  optionalMember,
  optionalString,
  readObject,
  requireArray,
  requireBoolean,
  requireCount,
  requireEnum,
  requireMember,
  requireString,
} from "@/shared/contracts/decoding";
import {
  edgeKinds,
  filterUnits,
  graphBoundPrecisions,
  nodeKinds,
} from "@/shared/contracts/graph";
import {
  decodeEdgeEvidenceSelector,
  type EdgeEvidenceSelector,
} from "@/shared/contracts/graphDetail";
import type { SearchTerms } from "@/shared/lib/searchTerms";
import type { ViewChoice } from "@/shared/lib/searchView";

/** 作業場所に置くビューの識別子。配置がこれ以外のビューを持つとき、配置を読めないものとする。 */
export const workspacePanelIds = [
  "search",
  "graph",
  "detail",
  "edgeDetail",
  "path",
  "record",
  "nodes",
  "edges",
  "detection",
  "priority",
  "timeline",
  "sources",
  "terminals",
  "terminal",
  "ips",
  "bookmarks",
  "histogram",
  "recording",
  "proposals",
  "workspaces",
  "members",
  "assist",
  "assistPermissions",
] as const;

/**
 * 分析者が調査中に使う画面の状態。同じ作業を再開するために、画面へ適用し直せる値だけを持つ。
 * 読み込み中の状態、hover、図の視点、入力途中の下書きを持たない。
 *
 * JSON.stringify で書き出し、`decodeWorkspaceState` で読み直す。
 */
export type WorkspaceState = {
  /** 作業場所の配置。dockview の `toJSON()` の値であり、`fromJSON()` で適用する。 */
  dockLayout: unknown;
  recordFilter: RecordFilterCriteria;
  /** レコードの番号で比べる相手の収集元。空文字列は比べないことを表す。 */
  comparedSourceId: string;
  /** 選んだ収集元の sourceId。適用時に収集元の一覧から探し、見つからなければ未選択にする。 */
  selectedSourceId?: string;
  searchTerms: SearchTerms;
  matchConditions: MatchConditionSelection;
  openedRecord?: { ref: RecordLocator; origin?: RecordLocator };
  selectedEdgeId?: string;
  evidenceGroupSelector?: EdgeEvidenceSelector;
  nodeScope: TimelineNodeScopeValue;
  history: PlaceHistory;
  bookmarks: Bookmark[];
  /** グラフの探索が持つ条件・図に出す対象・描画の上限・探索・選んだノード。 */
  graph: GraphExploreState;
};

function enumDecoder<T extends string>(values: readonly T[]): Decoder<T> {
  return (input, path) => requireEnum({ value: input }, "value", path, values);
}

function requireDepth(
  source: Record<string, unknown>,
  key: string,
  path: string,
): number {
  const depth = requireCount(source, key, path);
  if (depth < minGraphDepth || depth > maxGraphDepth) {
    throw new DecodeFailure(
      `${path}.${key}`,
      `expected ${minGraphDepth} to ${maxGraphDepth}`,
    );
  }
  return depth;
}

const decodeNodeRef: Decoder<NodeRef> = (input, path) => {
  const source = readObject(input, path);
  return {
    id: requireString(source, "id", path),
    label: requireString(source, "label", path),
    kind: optionalEnum(source, "kind", path, nodeKinds),
  };
};

const decodeSourceRef: Decoder<SourceRef> = (input, path) => {
  const source = readObject(input, path);
  return {
    id: requireString(source, "id", path),
    label: requireString(source, "label", path),
  };
};

const decodeTimeBound: Decoder<GraphTimeBound> = (input, path) => {
  const source = readObject(input, path);
  return {
    text: requireString(source, "text", path),
    precision: requireEnum(source, "precision", path, graphBoundPrecisions),
  };
};

const decodeTimeFilter: Decoder<GraphTimeFilter> = (input, path) => {
  const source = readObject(input, path);
  return {
    from: optionalMember(source, "from", path, decodeTimeBound),
    to: optionalMember(source, "to", path, decodeTimeBound),
    unit: requireEnum(source, "unit", path, filterUnits),
  };
};

function optionalInteger(
  source: Record<string, unknown>,
  key: string,
  path: string,
): number | undefined {
  const value = source[key];
  if (value !== undefined && !Number.isSafeInteger(value)) {
    throw new DecodeFailure(`${path}.${key}`, "expected a safe integer");
  }
  // 上の Number.isSafeInteger が、undefined でない値を整数に限っている。
  return value as number | undefined;
}

const decodeRecordFilter: Decoder<RecordFilterCriteria> = (input, path) => {
  const source = readObject(input, path);
  return {
    timeFilter: optionalMember(source, "timeFilter", path, decodeTimeFilter),
    eventCategory: optionalString(source, "eventCategory", path),
    eventAction: optionalString(source, "eventAction", path),
    eventActionFrom: optionalInteger(source, "eventActionFrom", path),
    eventActionTo: optionalInteger(source, "eventActionTo", path),
    caseId: optionalMember(source, "caseId", path, decodeCaseId),
    terminal: optionalMember(source, "terminal", path, decodeNodeRef),
    sources: optionalArray(source, "sources", path, decodeSourceRef),
  };
};

const decodeSearchTerms: Decoder<SearchTerms> = (input, path) => {
  const source = readObject(input, path);
  return {
    contains: requireArray(source, "contains", path, decodeString),
    excludes: requireArray(source, "excludes", path, decodeString),
    field: optionalString(source, "field", path),
    fieldContains: optionalArray(source, "fieldContains", path, decodeString),
    fieldEquals: optionalArray(source, "fieldEquals", path, decodeString),
    expression: optionalString(source, "expression", path),
  };
};

const decodeMatchConditions: Decoder<MatchConditionSelection> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    conditions: requireArray(source, "conditions", path, (item, itemPath) => {
      const condition = readObject(item, itemPath);
      return {
        conditionKey: requireEnum(
          condition,
          "conditionKey",
          itemPath,
          conditionKeys,
        ),
        toleranceSeconds: optionalCount(
          condition,
          "toleranceSeconds",
          itemPath,
        ),
      };
    }),
  };
};

const decodeOpenedRecord: Decoder<
  NonNullable<WorkspaceState["openedRecord"]>
> = (input, path) => {
  const source = readObject(input, path);
  return {
    ref: requireMember(source, "ref", path, decodeRecordLocator),
    origin: optionalMember(source, "origin", path, decodeRecordLocator),
  };
};

const decodeNodeScope: Decoder<TimelineNodeScopeValue> = (input, path) => {
  const source = readObject(input, path);
  return {
    enabled: requireBoolean(source, "enabled", path),
    depth: requireDepth(source, "depth", path),
  };
};

const decodePlace: Decoder<Place> = (input, path) => {
  const source = readObject(input, path);
  return requireEnum(source, "kind", path, ["record", "node"]) === "record"
    ? {
        kind: "record",
        ref: requireMember(source, "ref", path, decodeRecordLocator),
        eventId: optionalString(source, "eventId", path),
      }
    : {
        kind: "node",
        node: requireMember(source, "node", path, decodeNodeRef),
      };
};

const decodeBookmarkTarget: Decoder<BookmarkTarget> = (input, path) => {
  const source = readObject(input, path);
  switch (
    requireEnum(source, "kind", path, ["record", "node", "edge", "source"])
  ) {
    case "edge":
      return {
        kind: "edge",
        edge: requireMember(source, "edge", path, (edge, edgePath) => {
          const value = readObject(edge, edgePath);
          return {
            id: requireString(value, "id", edgePath),
            kind: requireEnum(value, "kind", edgePath, edgeKinds),
            sourceLabel: requireString(value, "sourceLabel", edgePath),
            targetLabel: requireString(value, "targetLabel", edgePath),
          };
        }),
      };
    case "source":
      return {
        kind: "source",
        source: requireMember(source, "source", path, decodeSourceRef),
      };
    default:
      return decodePlace(input, path);
  }
};

/**
 * ブックマーク 1 件を読む。`kind` を持つ要素は、見た場所をそのまま並べていた形で保存した
 * ブックマークとして読み、付けた画面をその場所の種類とする。
 */
const decodeBookmark: Decoder<Bookmark> = (input, path) => {
  const source = readObject(input, path);
  if ("kind" in source) {
    const place = decodePlace(input, path);
    return { target: place, from: place.kind };
  }
  return {
    target: requireMember(source, "target", path, decodeBookmarkTarget),
    from: requireEnum(source, "from", path, bookmarkOrigins),
    addedAt: optionalString(source, "addedAt", path),
    time: optionalMember(source, "time", path, (time, timePath) => {
      const value = readObject(time, timePath);
      return {
        kind: requireEnum(value, "kind", timePath, bookmarkTimeKinds),
        value: requireMember(value, "value", timePath, decodeTimestamp),
      };
    }),
  };
};

const decodeHistory: Decoder<PlaceHistory> = (input, path) => {
  const source = readObject(input, path);
  const places = requireArray(source, "places", path, decodePlace);
  const index = source.index;
  if (
    typeof index !== "number" ||
    !Number.isSafeInteger(index) ||
    index < (places.length === 0 ? -1 : 0) ||
    index >= places.length
  ) {
    throw new DecodeFailure(
      `${path}.index`,
      "expected a position in places, or -1 for no places",
    );
  }
  return { places, index };
};

/**
 * 検索の条件が一致ノードを限るノードを読む。起点を持たない前の保存も読めるよう、欄が無いときと
 * 空の並びは起点を持たない条件として読む。
 */
function optionalSearchOrigins(
  source: Record<string, unknown>,
  path: string,
): NodeRef[] | undefined {
  const origins = optionalArray(source, "origins", path, decodeNodeRef);
  if (origins === undefined || origins.length === 0) {
    return undefined;
  }
  if (origins.length > maxOriginNodes) {
    throw new DecodeFailure(
      `${path}.origins`,
      `expected at most ${maxOriginNodes} origins`,
    );
  }
  return origins;
}

/** 画面の条件を読む。消した機能の欄 (`expansion`、`unfolds`、`expands`) は読み飛ばす。 */
const decodeGraphCriteria: Decoder<GraphExploreCriteria> = (input, path) => {
  const source = readObject(input, path);
  return {
    depth: requireDepth(source, "depth", path),
    origins: optionalSearchOrigins(source, path),
    edgeKinds: optionalArray(source, "edgeKinds", path, enumDecoder(edgeKinds)),
    addressInCidr: optionalString(source, "addressInCidr", path),
    addressNotInCidr: optionalString(source, "addressNotInCidr", path),
    countBy: optionalString(source, "countBy", path),
    conditionsOnOriginsOnly: optionalBoolean(
      source,
      "conditionsOnOriginsOnly",
      path,
    ),
    endpointRecordsInPeriod: optionalBoolean(
      source,
      "endpointRecordsInPeriod",
      path,
    ),
  };
};

const decodeViewChoice: Decoder<ViewChoice> = (input, path) => {
  const source = readObject(input, path);
  return requireEnum(source, "kind", path, ["auto", "manual"]) === "auto"
    ? { kind: "auto" }
    : {
        kind: "manual",
        nodeKinds: requireArray(
          source,
          "nodeKinds",
          path,
          enumDecoder(nodeKinds),
        ),
      };
};

const decodeDrawLimit = (source: Record<string, unknown>, path: string) => {
  const value = source.drawLimit;
  if (value === "all") return value;
  const limit = requireCount(source, "drawLimit", path);
  if (limit < 1) {
    throw new DecodeFailure(`${path}.drawLimit`, "expected a positive limit");
  }
  return limit;
};

type Neighbours = {
  kind: "neighbours";
  origins: NodeRef[];
  keepsSearch?: boolean;
  showOnly?: boolean;
};

function decodeOrigins(source: Record<string, unknown>, path: string) {
  const origins = requireArray(source, "origins", path, decodeNodeRef);
  if (origins.length === 0 || origins.length > maxOriginNodes) {
    throw new DecodeFailure(
      `${path}.origins`,
      `expected 1 to ${maxOriginNodes} origins`,
    );
  }
  return origins;
}

const decodeNeighbours: Decoder<Neighbours> = (input, path) => {
  const source = readObject(input, path);
  requireEnum(source, "kind", path, ["neighbours"]);
  return {
    kind: "neighbours",
    origins: decodeOrigins(source, path),
    keepsSearch: optionalBoolean(source, "keepsSearch", path),
    showOnly: optionalBoolean(source, "showOnly", path),
  };
};

const decodeExploration: Decoder<
  NonNullable<GraphExploreState["exploration"]>["exploration"]
> = (input, path) => {
  const source = readObject(input, path);
  return requireEnum(source, "kind", path, ["lineage", "neighbours"]) ===
    "lineage"
    ? {
        kind: "lineage",
        origin: requireMember(source, "origin", path, decodeNodeRef),
      }
    : decodeNeighbours(input, path);
};

/** 選んだノードを読む。消した機能のまとまりの選択 (`group`) は未選択として読む。 */
const decodeGraphSelection: Decoder<GraphExploreSelection | undefined> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return requireEnum(source, "kind", path, ["node", "group"]) === "node"
    ? { kind: "node", node: requireMember(source, "node", path, decodeNodeRef) }
    : undefined;
};

const decodeGraphState: Decoder<GraphExploreState> = (input, path) => {
  const source = readObject(input, path);
  return {
    criteria: requireMember(source, "criteria", path, decodeGraphCriteria),
    view: requireMember(source, "view", path, decodeViewChoice),
    drawLimit: decodeDrawLimit(source, path),
    exploration: optionalMember(
      source,
      "exploration",
      path,
      (item, itemPath) => {
        const exploration = readObject(item, itemPath);
        return {
          exploration: requireMember(
            exploration,
            "exploration",
            itemPath,
            decodeExploration,
          ),
          active: requireBoolean(exploration, "active", itemPath),
          kept: optionalMember(exploration, "kept", itemPath, decodeNeighbours),
        };
      },
    ),
    selected: optionalMember(source, "selected", path, decodeGraphSelection),
    mergeSameAccount:
      optionalBoolean(source, "mergeSameAccount", path) ?? false,
  };
};

/** 作業場所から消したビュー。保存した配置が持っていれば、配置から取り除いて読む。 */
const removedPanelIds: ReadonlySet<string> = new Set(["clusters", "leads"]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/**
 * 保存した配置から消したビューを取り除く。`panels` と、区画の木 (`grid.root`) と、浮かせた区画と
 * 別ウィンドウの区画から取り除き、ビューが残らない区画を消す。形を確かめるのは dockview の
 * `fromJSON` であり、形の違う値はそのまま返す。
 */
function withoutRemovedPanels(layout: Record<string, unknown>): unknown {
  const droppedGroups = new Set<unknown>();
  // 区画 1 つ (`views` と `activeView` を持つ)。ビューが残らなければ undefined を返す。
  const pruneGroup = (group: unknown): unknown => {
    if (!isRecord(group) || !Array.isArray(group.views)) return group;
    const views = group.views.filter((id) => !removedPanelIds.has(id));
    if (views.length === 0) {
      droppedGroups.add(group.id);
      return undefined;
    }
    return {
      ...group,
      views,
      activeView: views.includes(group.activeView)
        ? group.activeView
        : views[0],
    };
  };
  // 区画の木の節 1 つ。葉は区画を持ち、枝は子の節を並べる。
  const pruneNode = (node: unknown): unknown => {
    if (!isRecord(node)) return node;
    if (node.type === "leaf") {
      const data = pruneGroup(node.data);
      return data === undefined ? undefined : { ...node, data };
    }
    if (node.type === "branch" && Array.isArray(node.data)) {
      const data = node.data
        .map(pruneNode)
        .filter((child) => child !== undefined);
      return data.length === 0 ? undefined : { ...node, data };
    }
    return node;
  };
  // 浮かせた区画と別ウィンドウの区画は、`data` に区画を持つ。
  const pruneDetached = (entries: unknown): unknown =>
    Array.isArray(entries)
      ? entries.flatMap((entry) => {
          if (!isRecord(entry)) return [entry];
          const data = pruneGroup(entry.data);
          return data === undefined ? [] : [{ ...entry, data }];
        })
      : entries;

  const result: Record<string, unknown> = { ...layout };
  if (isRecord(layout.panels)) {
    result.panels = Object.fromEntries(
      Object.entries(layout.panels).filter(([id]) => !removedPanelIds.has(id)),
    );
  }
  if (isRecord(layout.grid) && isRecord(layout.grid.root)) {
    const root = layout.grid.root;
    result.grid = {
      ...layout.grid,
      root: pruneNode(root) ?? { ...root, data: [] },
    };
  }
  if ("floatingGroups" in layout) {
    result.floatingGroups = pruneDetached(layout.floatingGroups);
  }
  if ("popoutGroups" in layout) {
    result.popoutGroups = pruneDetached(layout.popoutGroups);
  }
  if (droppedGroups.has(layout.activeGroup)) {
    delete result.activeGroup;
  }
  return result;
}

/**
 * 配置を読む。消したビューを取り除き、作業場所に無いビューを持つ配置を読めないものとする。
 */
const decodeDockLayout: Decoder<unknown> = (input, path) => {
  const layout = withoutRemovedPanels(readObject(input, path));
  const panels = readObject(readObject(layout, path).panels, `${path}.panels`);
  const known: readonly string[] = workspacePanelIds;
  for (const id of Object.keys(panels)) {
    if (!known.includes(id)) {
      throw new DecodeFailure(`${path}.panels.${id}`, "unknown panel");
    }
  }
  return layout;
};

/**
 * 保存した画面の状態を読む。1 つの欄でも読めなければ `DecodeFailure` を投げ、一部だけを
 * 適用させない。
 */
export function decodeWorkspaceState(input: unknown): WorkspaceState {
  const path = "$";
  const source = readObject(input, path);
  return {
    dockLayout: requireMember(source, "dockLayout", path, decodeDockLayout),
    recordFilter: requireMember(
      source,
      "recordFilter",
      path,
      decodeRecordFilter,
    ),
    comparedSourceId: requireString(source, "comparedSourceId", path),
    selectedSourceId: optionalString(source, "selectedSourceId", path),
    searchTerms: requireMember(source, "searchTerms", path, decodeSearchTerms),
    matchConditions: requireMember(
      source,
      "matchConditions",
      path,
      decodeMatchConditions,
    ),
    openedRecord: optionalMember(
      source,
      "openedRecord",
      path,
      decodeOpenedRecord,
    ),
    selectedEdgeId: optionalString(source, "selectedEdgeId", path),
    evidenceGroupSelector: optionalMember(
      source,
      "evidenceGroupSelector",
      path,
      decodeEdgeEvidenceSelector,
    ),
    nodeScope: requireMember(source, "nodeScope", path, decodeNodeScope),
    history: requireMember(source, "history", path, decodeHistory),
    bookmarks: requireArray(source, "bookmarks", path, decodeBookmark),
    graph: requireMember(source, "graph", path, decodeGraphState),
  };
}
