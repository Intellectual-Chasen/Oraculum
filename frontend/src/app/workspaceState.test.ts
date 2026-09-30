import { expect, test } from "vitest";
import { maxOriginNodes } from "@/shared/api/graph";
import { DecodeFailure } from "@/shared/contracts/decoding";
import { timelineResponseJson } from "@/testdata/timeline/timelineResponse";
import groupsViewWorkspaceState from "@/testdata/workspace/groupsViewWorkspaceState.json";
import { decodeWorkspaceState } from "./workspaceState";

const recordRef = timelineResponseJson().entries[0]?.recordRef;
const eventTime = timelineResponseJson().entries[0]?.eventTime;

// biome-ignore lint/suspicious/noExplicitAny: 欄を 1 つずつ壊すために、JSON の形を型で縛らない。
type Json = Record<string, any>;

/** すべての欄に値を持つ画面の状態の JSON。 */
function fullStateJson(): Json {
  return {
    dockLayout: { grid: {}, panels: { search: {}, graph: {} } },
    recordFilter: {
      timeFilter: {
        from: { text: "2031-10-08T01:00:00+09:00", precision: "second" },
        unit: "second",
      },
      eventCategory: "net",
      eventActionFrom: 4624,
      caseId: "case-a",
      terminal: { id: "n:terminal:1", label: "HOST-A", kind: "terminal" },
      sources: [{ id: "src:1", label: "host-a.log" }],
    },
    comparedSourceId: "",
    selectedSourceId: "src:1",
    searchTerms: {
      contains: ["cmd.exe"],
      excludes: [],
      fieldContains: ["user=alice"],
      expression: "LogonType == 3",
    },
    matchConditions: {
      conditions: [
        { conditionKey: "second_of_time", toleranceSeconds: 2 },
        { conditionKey: "user" },
      ],
    },
    openedRecord: { ref: recordRef, origin: recordRef },
    selectedEdgeId: "e:1",
    evidenceGroupSelector: {
      eventCategory: "net",
      eventAction: "acpt",
      destinationPort: "5985",
      logonTypeAbsent: true,
    },
    nodeScope: { enabled: false, depth: 2 },
    history: {
      places: [
        { kind: "record", ref: recordRef },
        { kind: "node", node: { id: "n:1", label: "cmd.exe" } },
      ],
      index: 1,
    },
    bookmarks: [
      {
        target: { kind: "node", node: { id: "n:1", label: "cmd.exe" } },
        from: "node",
        addedAt: "2031-10-09T00:00:00.000Z",
      },
      {
        target: { kind: "record", ref: recordRef },
        from: "timeline",
        addedAt: "2031-10-09T00:00:01.000Z",
        time: { kind: "event", value: eventTime },
      },
      {
        target: {
          kind: "edge",
          edge: {
            id: "e:1",
            kind: "process_parent_child",
            sourceLabel: "cmd.exe",
            targetLabel: "whoami.exe",
          },
        },
        from: "edge",
        addedAt: "2031-10-09T00:00:02.000Z",
      },
      {
        target: { kind: "source", source: { id: "s:1", label: "host-a.log" } },
        from: "source",
        addedAt: "2031-10-09T00:00:03.000Z",
        time: { kind: "observedFirst", value: eventTime },
      },
      {
        target: {
          kind: "record",
          ref: { ...recordRef, sequenceNumber: 1 },
          eventId: "4624",
        },
        from: "record",
        addedAt: "2031-10-09T00:00:04.000Z",
      },
    ],
    graph: {
      criteria: {
        depth: 1,
        origins: [{ id: "n:3", label: "whoami.exe", kind: "process" }],
        edgeKinds: ["process_parent_child"],
        countBy: "user",
      },
      view: { kind: "manual", nodeKinds: ["process"] },
      drawLimit: 500,
      exploration: {
        exploration: { kind: "lineage", origin: { id: "n:1", label: "cmd" } },
        active: true,
        kept: {
          kind: "neighbours",
          origins: [{ id: "n:2", label: "HOST-B" }],
          keepsSearch: true,
        },
      },
      selected: { kind: "node", node: { id: "n:1", label: "cmd" } },
      mergeSameAccount: true,
    },
  };
}

test("同じアカウントのまとめの切り替えを保ち、欄が無い状態はまとめないものとして読む", () => {
  expect(decodeWorkspaceState(fullStateJson()).graph.mergeSameAccount).toBe(
    true,
  );
  const json = fullStateJson();
  const { mergeSameAccount: _merge, ...graph } = json.graph;
  expect(decodeWorkspaceState({ ...json, graph }).graph.mergeSameAccount).toBe(
    false,
  );
  // まとめの切り替えは要求の条件と別の欄であり、条件に入らない。
  expect(decodeWorkspaceState(json).graph.criteria).not.toHaveProperty(
    "mergeSameAccount",
  );
});

test("すべての欄を読み、JSON にして読み直しても同じ値になる", () => {
  const json = fullStateJson();
  const state = decodeWorkspaceState(json);
  expect(state.searchTerms.expression).toBe("LogonType == 3");
  expect(state.graph.selected).toEqual({
    kind: "node",
    node: { id: "n:1", label: "cmd" },
  });
  expect(JSON.parse(JSON.stringify(state))).toEqual(json);
});

test("関係先だけの表示を保存して復元しても背景を戻さない", () => {
  const json = fullStateJson();
  const saved = {
    ...json,
    graph: {
      ...json.graph,
      exploration: {
        active: true,
        exploration: {
          kind: "neighbours",
          origins: [{ id: "n:2", label: "HOST-B" }],
          showOnly: true,
        },
      },
    },
  };

  const restored = decodeWorkspaceState(saved).graph.exploration?.exploration;
  expect(restored?.kind).toBe("neighbours");
  if (restored?.kind === "neighbours") {
    expect(restored.showOnly).toBe(true);
  }
});

test("見た場所をそのまま並べていた形のブックマークを、新しい形と混ざっていても読む", () => {
  const json = fullStateJson();
  json.bookmarks = [
    { kind: "record", ref: recordRef },
    json.bookmarks[2],
    { kind: "node", node: { id: "n:1", label: "cmd.exe" } },
  ];
  const { bookmarks } = decodeWorkspaceState(json);
  expect(bookmarks.map((bookmark) => bookmark.from)).toEqual([
    "record",
    "edge",
    "node",
  ]);
  expect(bookmarks[0]).toEqual({
    target: { kind: "record", ref: recordRef },
    from: "record",
  });
});

// まとまりのビューを開いて保存した画面の状態。消した機能の欄と、まとまりのビューを持つ。
test("まとまりのビューを開いて保存した状態を読み、ビューと消した機能の欄を取り除く", () => {
  const state = decodeWorkspaceState(groupsViewWorkspaceState);
  const layout = state.dockLayout as Json;

  expect(Object.keys(layout.panels)).not.toContain("clusters");
  expect(Object.keys(layout.panels)).toHaveLength(12);
  const below = layout.grid.root.data[1].data[1].data;
  expect(below.views).not.toContain("clusters");
  expect(below.views).toHaveLength(9);
  // 前面にあったまとまりのビューに代えて、同じ区画の先頭のビューを前面に出す。
  expect(below.activeView).toBe("record");
  expect(layout.activeGroup).toBe("4");
  expect(state.graph.criteria).toEqual({ depth: 1 });
  expect(state.graph.selected).toBeUndefined();
  expect(state.graph.drawLimit).toBe(2000);
});

test("まとまりのビューだけを置いた区画と浮かせた区画を、配置から取り除く", () => {
  const json: Json = structuredClone(groupsViewWorkspaceState);
  const root = json.dockLayout.grid.root;
  const below = root.data[1].data[1].data;
  below.views = below.views.filter((id: string) => id !== "clusters");
  below.activeView = "record";
  root.data.push({
    type: "leaf",
    data: { views: ["clusters"], activeView: "clusters", id: "5" },
    size: 100,
  });
  json.dockLayout.activeGroup = "5";
  json.dockLayout.floatingGroups = [
    {
      data: { views: ["clusters"], activeView: "clusters", id: "6" },
      position: { left: 0, top: 0, width: 300, height: 200 },
    },
    {
      data: {
        views: ["clusters", "ips"],
        activeView: "clusters",
        id: "7",
      },
      position: { left: 0, top: 0, width: 300, height: 200 },
    },
  ];

  const layout = decodeWorkspaceState(json).dockLayout as Json;

  expect(layout.grid.root.data).toHaveLength(3);
  expect(JSON.stringify(layout.grid)).not.toContain("clusters");
  expect(layout.activeGroup).toBeUndefined();
  expect(layout.floatingGroups).toEqual([
    {
      data: { views: ["ips"], activeView: "ips", id: "7" },
      position: { left: 0, top: 0, width: 300, height: 200 },
    },
  ]);
});

test("完全一致の組と HTTP の状態の区分を、JSON にして読み直しても同じ値で保つ", () => {
  const json = fullStateJson();
  json.searchTerms = {
    ...json.searchTerms,
    fieldEquals: ["EventRecordID=4321"],
  };
  json.evidenceGroupSelector = {
    eventCategory: "",
    eventAction: "",
    destinationPort: "80",
    logonTypeAbsent: true,
    httpStatus: "404",
    // fullStateJson の推論した型は httpStatus を持たないため、同じ欄の組として代入する。
  } as typeof json.evidenceGroupSelector;
  const state = decodeWorkspaceState(json);
  expect(state.searchTerms.fieldEquals).toEqual(["EventRecordID=4321"]);
  expect(state.evidenceGroupSelector?.httpStatus).toBe("404");
  expect(JSON.parse(JSON.stringify(state))).toEqual(json);
});

test("省略できる欄が無い状態を読む", () => {
  const {
    selectedSourceId: _source,
    openedRecord: _record,
    selectedEdgeId: _edge,
    evidenceGroupSelector: _selector,
    ...json
  } = fullStateJson();
  expect(decodeWorkspaceState(json).openedRecord).toBeUndefined();
});

test("一致ノードを限るノードの欄が無い保存と空の並びは、ノードで限らない条件として読む", () => {
  const json = fullStateJson();
  delete json.graph.criteria.origins;
  expect(decodeWorkspaceState(json).graph.criteria.origins).toBeUndefined();
  json.graph.criteria.origins = [];
  expect(decodeWorkspaceState(json).graph.criteria.origins).toBeUndefined();
});

/** fullStateJson の一部を change で書き換えた JSON を読み、失敗した位置を返す。 */
function failurePath(change: (json: Json) => void): string {
  const json = fullStateJson();
  change(json);
  try {
    decodeWorkspaceState(json);
  } catch (error) {
    if (error instanceof DecodeFailure) return error.path;
    throw error;
  }
  throw new Error("decoded");
}

test.each([
  [
    "検索の文字列の欠け",
    (json: Json) => delete json.searchTerms,
    "$.searchTerms",
  ],
  ["グラフの状態の欠け", (json: Json) => delete json.graph, "$.graph"],
  [
    "履歴の位置の範囲外",
    (json: Json) => {
      json.history.index = 2;
    },
    "$.history.index",
  ],
  [
    "期間の単位の型違い",
    (json: Json) => {
      json.recordFilter.timeFilter.unit = 1;
    },
    "$.recordFilter.timeFilter.unit",
  ],
  [
    "ホップ数の範囲外",
    (json: Json) => {
      json.nodeScope.depth = 99;
    },
    "$.nodeScope.depth",
  ],
  [
    "描画の上限の型違い",
    (json: Json) => {
      json.graph.drawLimit = "500";
    },
    "$.graph.drawLimit",
  ],
  [
    "知らない関係の種別",
    (json: Json) => {
      json.graph.criteria.edgeKinds = ["unknown_kind"];
    },
    "$.graph.criteria.edgeKinds[0]",
  ],
  [
    "上限を超える一致ノードを限るノード",
    (json: Json) => {
      json.graph.criteria.origins = Array.from(
        { length: maxOriginNodes + 1 },
        (_, index) => ({ id: `n:${index}`, label: `node ${index}` }),
      );
    },
    "$.graph.criteria.origins",
  ],
  [
    "形の合わないブックマーク",
    (json: Json) => {
      json.bookmarks = [{ kind: "record", ref: { sourceId: "s" } }];
    },
    "$.bookmarks[0].ref",
  ],
  [
    "知らない種類のブックマーク",
    (json: Json) => {
      json.bookmarks[0].from = "unknown";
    },
    "$.bookmarks[0].from",
  ],
  [
    "知らない関係の種別のブックマーク",
    (json: Json) => {
      json.bookmarks[2].target.edge.kind = "unknown_kind";
    },
    "$.bookmarks[2].target.edge.kind",
  ],
  [
    "作業場所に無いビューを持つ配置",
    (json: Json) => {
      json.dockLayout.panels.unknownView = {};
    },
    "$.dockLayout.panels.unknownView",
  ],
  [
    "ビューを持たない配置",
    (json: Json) => {
      json.dockLayout = {};
    },
    "$.dockLayout.panels",
  ],
])("%s を読めないものとする", (_name, change, path) => {
  expect(failurePath(change)).toContain(path);
});

test("object でない値を読めないものとする", () => {
  expect(() => decodeWorkspaceState([])).toThrow(DecodeFailure);
});
