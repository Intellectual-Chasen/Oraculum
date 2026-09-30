import { expect, test } from "vitest";
import {
  decodeGraphResponse,
  type GraphResponse,
} from "@/shared/contracts/graph";
import {
  emptyGraphResponseJson,
  graphResponseJson,
  ipNodeId,
  overLimitGraphResponseJson,
  processNodeId,
  terminalNodeId,
  valueCountJson,
} from "@/testdata/graph/graphResponse";
import { nodeSelectionLabels } from "./labels";
import {
  buildSubgraphDrawing,
  highlightCountsOf,
  lineageRootsOf,
  readNodeLabel,
  subgraphSummaryPairs,
  valueCountsSummaryPairs,
  withBackdrop,
} from "./subgraph";

function response(): GraphResponse {
  return decodeGraphResponse(graphResponseJson(), "fixture");
}

test("開いたレコードに対応するノードとエッジのうち、応答にある数と全体の数を数える", () => {
  const drawn = response();
  expect(
    highlightCountsOf(drawn, {
      nodeIds: new Set([processNodeId, ipNodeId, "n:absent"]),
      edgeIds: new Set([drawn.edges[0]?.id ?? "", "e:absent"]),
    }),
  ).toEqual({ shownNodes: 2, nodes: 3, shownEdges: 1, edges: 2 });
});

test("上限を超えた応答では、一致ノードの一覧にあるノードだけを数え、エッジは 0 本と数える", () => {
  const withheld = decodeGraphResponse(
    overLimitGraphResponseJson(201, 200),
    "fixture",
  );
  expect(
    highlightCountsOf(withheld, {
      nodeIds: new Set([processNodeId, ipNodeId]),
      edgeIds: new Set(["e:any"]),
    }),
  ).toEqual({ shownNodes: 1, nodes: 2, shownEdges: 0, edges: 1 });
});

test("背景を足すと、部分グラフの点とエッジを先に並べ、背景の点と、端点が図にあるエッジを重ねずに足す", () => {
  const whole = buildSubgraphDrawing(response());
  const [first, second] = whole.points;
  if (first === undefined || second === undefined) throw new Error("fixture");
  const focus = { ...whole, points: [second], links: [] };
  const merged = withBackdrop(focus, whole);

  expect(merged.points.map((point) => point.id)).toEqual([
    second.id,
    ...whole.points
      .filter((point) => point.id !== second.id)
      .map((point) => point.id),
  ]);
  expect(merged.links.map((link) => link.id)).toEqual(
    whole.links.map((link) => link.id),
  );
  // 端点が図に無いエッジは足さない。
  const partial = withBackdrop(focus, { ...whole, points: [first] });
  for (const link of partial.links) {
    const ids = partial.points.map((point) => point.id);
    expect(ids).toContain(link.sourceNodeId);
    expect(ids).toContain(link.targetNodeId);
  }
});

test("応答のノードを応答の並びのまま返す", () => {
  const drawing = buildSubgraphDrawing(response());

  expect(drawing.points.map((point) => point.id)).toEqual([
    terminalNodeId,
    processNodeId,
    ipNodeId,
  ]);
  expect(drawing.points.map((point) => point.selection)).toEqual([
    "matched",
    "matched",
    "edge_endpoint",
  ]);
});

test("応答のエッジを両端の識別子と根拠の件数のまま返す", () => {
  const drawing = buildSubgraphDrawing(response());

  expect(drawing.links).toEqual([
    {
      id: "e:ran_on:0001",
      kind: "ran_on",
      state: "observed",
      sourceNodeId: processNodeId,
      targetNodeId: terminalNodeId,
      evidenceCount: 4,
    },
    {
      id: "e:process_communication:0002",
      kind: "process_communication",
      state: "observed",
      sourceNodeId: processNodeId,
      targetNodeId: ipNodeId,
      evidenceCount: 1,
    },
  ]);
});

test("ノードが 1 件も無い応答から要素数 0 の図を組む", () => {
  const empty = decodeGraphResponse(emptyGraphResponseJson(), "fixture");

  const drawing = buildSubgraphDrawing(empty);

  expect(drawing.points).toEqual([]);
  expect(drawing.links).toEqual([]);
});

test("原資料の文字列を持つ表示名を、文字列と値の状態で読む", () => {
  expect(readNodeLabel({ rawText: "HOST-C", valueState: "present" })).toEqual({
    value: { text: "HOST-C" },
    valueState: "present",
  });
});

test("導いた表示名を、導いた値と導き方と値の状態で読む", () => {
  expect(
    readNodeLabel({
      normalized: "cmd.exe",
      derivation: "file.path の末尾の要素から導いた",
      valueState: "derived",
    }),
  ).toEqual({
    value: { text: "cmd.exe" },
    valueState: "derived",
    derivation: "file.path の末尾の要素から導いた",
  });
});

test("表示名を持たないノードを、値が無い理由と値の状態で読む", () => {
  expect(readNodeLabel({ valueState: "item_absent" })).toEqual({
    value: { absence: "フィールドなし" },
    valueState: "item_absent",
  });
});

test("グラフの一致ノードの件数と、エッジの端のノードの件数とエッジの本数を組で返す", () => {
  expect(subgraphSummaryPairs(response())).toEqual([
    { name: "グラフの一致ノード", value: "2" },
    { name: "エッジの端のノード", value: "1" },
    { name: "エッジ", value: "2" },
  ]);
});

// **上限を超えた応答で、エッジが 0 本と書かない。** 描画するノードの件数と上限を返す。
test("上限を超えた応答は、描画するノードの件数と描画の上限を組で返す", () => {
  const withheld = decodeGraphResponse(
    overLimitGraphResponseJson(350, 200),
    "fixture",
  );

  expect(subgraphSummaryPairs(withheld)).toEqual([
    { name: "描画するノード", value: "350" },
    { name: "描画の上限", value: "200" },
  ]);
});

test("親との関係を持たないプロセスを、親子の連鎖の上端として返す", () => {
  const lineage = response();
  lineage.edges = [
    {
      id: "e:process_parent_child:0009",
      kind: "process_parent_child",
      state: "observed",
      sourceNodeId: terminalNodeId,
      targetNodeId: processNodeId,
      evidenceCount: 1,
    },
  ];
  // 端末は種別がプロセスでないため、上端に入らない。プロセスは親を持つため入らない。
  expect(lineageRootsOf(lineage).map((node) => node.id)).toEqual([]);

  lineage.edges = [];
  expect(lineageRootsOf(lineage).map((node) => node.id)).toEqual([
    processNodeId,
  ]);
});

test("上限を超えた応答では、親子の連鎖の上端を決めない", () => {
  const withheld = decodeGraphResponse(
    overLimitGraphResponseJson(350, 200),
    "fixture",
  );

  expect(lineageRootsOf(withheld)).toEqual([]);
});

test("要約の組が、ノードを指す語を nodeSelectionLabels から組む", () => {
  const names = subgraphSummaryPairs(response()).map((pair) => pair.name);

  expect(names.join()).toContain(nodeSelectionLabels.matched);
  expect(names).toContain(nodeSelectionLabels.edge_endpoint);
});

// **エッジが 0 本の応答でも「無い」と断定しない。** 本数の値だけを返す。
test("エッジが 0 本の応答は、本数 0 の組を返す", () => {
  const empty = response();
  empty.edges = [];
  empty.edgeCount = 0;

  expect(subgraphSummaryPairs(empty)).toContainEqual({
    name: "エッジ",
    value: "0",
  });
});

/**
 * 数える欄を与えた応答を、値の異なりの件数を指定して組む。
 * 0 件の応答は理由の欄を持つ (`backend/api/graph.go` の ValueCountsEmptyReason)。
 */
function countedResponse(distinct: number): GraphResponse {
  return decodeGraphResponse(
    {
      ...graphResponseJson(),
      countBy: "http.user_agent",
      valueCounts: Array.from({ length: distinct }, (_, index) => ({
        ...valueCountJson(),
        value: `agent-${index}`,
      })),
      distinctValueCount: distinct,
      ...(distinct === 0
        ? { valueCountsEmptyReason: "no_value_in_filter" }
        : {}),
    },
    "fixture",
  );
}

test("値の種類の件数を組で返す", () => {
  expect(valueCountsSummaryPairs(countedResponse(2))).toEqual([
    { name: "値の種類", value: "2" },
  ]);
});

// 値の種類が 0 件でも、数えるフィールドを与えた応答は要約を返す。
test("値の種類が 0 件の応答でも、数えた件数を返す", () => {
  expect(valueCountsSummaryPairs(countedResponse(0))).toEqual([
    { name: "値の種類", value: "0" },
  ]);
});
