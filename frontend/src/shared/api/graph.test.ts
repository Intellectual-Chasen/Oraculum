import { afterEach, expect, test, vi } from "vitest";
import { baselineCaseId, challengeCaseId } from "@/testdata/cases/caseCounts";
import {
  accountNodeId,
  attributeCountMismatchNodeDetailResponseJson,
  clientTerminalNodeId,
  edgeDetailOfPort5985Json,
  edgeDetailResponseJson,
  emptyGraphResponseJson,
  evidenceCountMismatchEdgeDetailResponseJson,
  evidenceCountMismatchNodeDetailResponseJson,
  graphResponseJson,
  ipNodeId,
  logonTypeAbsentReason,
  matchCountMismatchEdgeDetailResponseJson,
  nodeDetailResponseJson,
  processNodeId,
  remoteSessionEdgeId,
  summarizedRecordGraphResponseJson,
  terminalNodeId,
  valueCountJson,
} from "@/testdata/graph/graphResponse";
import { jsonResponse, textResponse } from "@/testdata/http";
import { searchExpressionErrorResponseJson } from "@/testdata/searchExpression/searchExpressionErrorResponse";
import { apiErrorJson } from "@/testdata/sources/sourcesResponse";
import { readObject } from "../contracts/decoding";
import {
  decodeEdgeEvidenceGroup,
  type EdgeEvidenceGroup,
} from "../contracts/graphDetail";
import {
  fetchEdgeDetail,
  fetchGraph,
  fetchNodeDetail,
  type GraphRequest,
  maxGraphDepth,
  maxOriginNodes,
  minGraphDepth,
} from "./graph";
import type { MatchConditionSelection } from "./matchConditions";

afterEach(() => {
  vi.unstubAllGlobals();
});

function stubFetch(result: Response | Error) {
  const mock = vi.fn(async (_input: string, _init?: RequestInit) => {
    if (result instanceof Error) {
      throw result;
    }
    return result;
  });
  vi.stubGlobal("fetch", mock);
  return mock;
}

// 本 test が渡す関連付けの条件の選択。条件を含む項目の期待値は、選択を変える test が挙げる。
const matchConditions: MatchConditionSelection = {
  conditions: [{ conditionKey: "destination_ip" }],
};
const matchConditionQuery = "matchCondition=destination_ip";

const baseRequest = {
  depth: 1,
  matchConditions,
} as const satisfies GraphRequest;

test("部分グラフを取得し、応答の項目を画面が扱う型へ変換する", async () => {
  const mock = stubFetch(jsonResponse(200, graphResponseJson()));

  const result = await fetchGraph(baseRequest);

  expect(mock).toHaveBeenCalledTimes(1);
  expect(mock.mock.calls[0]?.[0]).toBe(
    `/api/v0/graph?depth=1&${matchConditionQuery}`,
  );
  expect(mock.mock.calls[0]?.[1]).toEqual({
    method: "GET",
    headers: { Accept: "application/json" },
    signal: undefined,
  });

  expect(result.ok).toBe(true);
  if (!result.ok) {
    return;
  }
  expect(result.value.nodeCount).toBe(2);
  expect(result.value.depth).toBe(1);
  expect(result.value.nodes.map((node) => node.id)).toEqual([
    terminalNodeId,
    processNodeId,
    ipNodeId,
  ]);
  expect(result.value.nodes[2]?.selection).toBe("edge_endpoint");
  expect(result.value.nodes[1]?.identity).toEqual([
    { semantic: "terminal.id", value: "HOST-C-TMID" },
    { semantic: "process.id", value: "{P1}" },
  ]);
  expect(result.value.nodes[2]?.identity[0]?.semantic).toBeUndefined();
  expect(result.value.edges[0]?.kind).toBe("ran_on");
  expect(result.value.edges[0]?.evidenceCount).toBe(4);
  expect(result.value.edges[0]?.applicableRange?.from.normalized).toBe(
    "2031-10-08T10:20:35.100+09:00",
  );
  expect(result.value.edges[1]?.applicableRange).toBeUndefined();
  expect(result.value.filterUnit).toBeUndefined();
  expect(result.value.emptyReason).toBeUndefined();
});

test("イベント ID の範囲と文字列を照合する欄を query に載せ、文字列の無い欄の指定は載せない", async () => {
  const mock = stubFetch(jsonResponse(200, graphResponseJson()));
  await fetchGraph({
    ...baseRequest,
    eventActionFrom: 8000,
    eventActionTo: 8999,
    valueContains: ["example"],
    valueField: "CommandLine",
  });
  const withTerms = new URL(
    String(mock.mock.calls[0]?.[0]),
    "http://localhost",
  );
  expect(withTerms.searchParams.get("eventActionFrom")).toBe("8000");
  expect(withTerms.searchParams.get("eventActionTo")).toBe("8999");
  expect(withTerms.searchParams.get("valueField")).toBe("CommandLine");

  await fetchGraph({ ...baseRequest, valueField: "CommandLine" });
  const withoutTerms = new URL(
    String(mock.mock.calls[1]?.[0]),
    "http://localhost",
  );
  expect(withoutTerms.searchParams.has("valueField")).toBe(false);
});

test("種別と粒度と起点と期間と端末のフィルタを query に載せる", async () => {
  const mock = stubFetch(jsonResponse(200, graphResponseJson()));

  const result = await fetchGraph({
    ...baseRequest,
    nodeKinds: ["process", "ip"],
    granularity: "object",
    nodeIds: [processNodeId, terminalNodeId],
    edgeKinds: ["process_communication", "ran_on"],
    eventCategory: "net",
    eventAction: "con",
    terminal: terminalNodeId,
    timeFilter: {
      from: { text: "2031-10-08T10:20:35+09:00", precision: "second" },
      unit: "second",
    },
  });

  expect(mock.mock.calls[0]?.[0]).toBe(
    "/api/v0/graph?nodeKind=process&nodeKind=ip&granularity=object" +
      "&nodeId=n%3Aprocess%3A8ab3&nodeId=n%3Aterminal%3A1f0c&depth=1" +
      "&edgeKind=process_communication&edgeKind=ran_on&eventCategory=net&eventAction=con" +
      "&terminal=n%3Aterminal%3A1f0c" +
      "&timeFrom=2031-10-08T10%3A20%3A35%2B09%3A00&timeFromPrecision=second" +
      `&filterUnit=second&${matchConditionQuery}`,
  );
  expect(result.ok).toBe(true);
});

test("描画の上限を与えた要求は、nodeLimit を載せる", async () => {
  const mock = stubFetch(jsonResponse(200, graphResponseJson()));

  await fetchGraph({ ...baseRequest, nodeLimit: 200 });

  expect(mock.mock.calls[0]?.[0]).toBe(
    `/api/v0/graph?nodeLimit=200&depth=1&${matchConditionQuery}`,
  );
});

test.each([0, -1, 1.5])("上限 %s の要求を送らない", async (nodeLimit) => {
  const mock = stubFetch(jsonResponse(200, graphResponseJson()));

  const result = await fetchGraph({ ...baseRequest, nodeLimit });

  expect(mock).not.toHaveBeenCalled();
  expect(result.ok).toBe(false);
});

test("空の集合の条件を query に載せない", async () => {
  const mock = stubFetch(jsonResponse(200, graphResponseJson()));

  await fetchGraph({
    ...baseRequest,
    nodeKinds: [],
    valueContains: [],
    valueExcludes: [],
  });

  expect(mock.mock.calls[0]?.[0]).toBe(
    `/api/v0/graph?depth=1&${matchConditionQuery}`,
  );
});

test("検索式を文字列のまま 1 つの searchExpression に載せ、応答が用いた検索式を読む", async () => {
  const expression = 'TargetUserName contains "a b" && !(LogonType == 3)';
  const mock = stubFetch(
    jsonResponse(200, { ...graphResponseJson(), searchExpression: expression }),
  );

  const result = await fetchGraph({
    ...baseRequest,
    searchExpression: expression,
  });

  const url = new URL(String(mock.mock.calls[0]?.[0]), "http://localhost");
  expect(url.searchParams.getAll("searchExpression")).toEqual([expression]);
  expect(result.ok).toBe(true);
  if (!result.ok) {
    return;
  }
  expect(result.value.searchExpression).toBe(expression);
});

test.each([
  ["空の検索式", ""],
  ["空白だけの検索式", " \t "],
])("%s を query に載せない", async (_name, expression) => {
  const mock = stubFetch(jsonResponse(200, graphResponseJson()));

  await fetchGraph({ ...baseRequest, searchExpression: expression });

  expect(mock.mock.calls[0]?.[0]).toBe(
    `/api/v0/graph?depth=1&${matchConditionQuery}`,
  );
});

test("検索式を読めなかった失敗は、誤りの理由と位置と説明を含む", async () => {
  stubFetch(
    jsonResponse(
      400,
      searchExpressionErrorResponseJson({
        reason: "unexpected_character",
        offset: 10,
        length: 1,
      }),
    ),
  );

  const result = await fetchGraph({
    ...baseRequest,
    searchExpression: "LogonType = 3",
  });

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("request_rejected");
  expect(result.failure.failureCode).toBe("invalid_request");
  expect(result.failure.searchExpressionError).toEqual({
    reason: "unexpected_character",
    offset: 10,
    length: 1,
    description: "条件を始められない文字",
  });
});

// 続きの位置が総数と等しい応答は、要素数 0 のページを理由なしで返す。
test("数えた個数と値ごとの件数の要素数が食い違う応答を読まない", async () => {
  stubFetch(
    jsonResponse(200, {
      ...graphResponseJson(),
      countBy: "http.user_agent",
      valueCounts: [],
      distinctValueCount: 7,
    }),
  );

  const result = await fetchGraph(baseRequest);

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});
test("ホップ数とアドレスの範囲と検索の文字列を query に載せる", async () => {
  const mock = stubFetch(jsonResponse(200, graphResponseJson()));

  const result = await fetchGraph({
    ...baseRequest,
    depth: maxGraphDepth,
    addressInCidr: "198.51.100.0/24",
    addressNotInCidr: "198.51.100.0/28",
    valueContains: ["Get-ChildItem", "powershell"],
    valueExcludes: ["-enc"],
  });

  expect(mock.mock.calls[0]?.[0]).toBe(
    `/api/v0/graph?depth=${maxGraphDepth}` +
      "&addressInCidr=198.51.100.0%2F24&addressNotInCidr=198.51.100.0%2F28" +
      "&valueContains=Get-ChildItem&valueContains=powershell" +
      `&valueExcludes=-enc&${matchConditionQuery}`,
  );
  expect(result.ok).toBe(true);
});

test.each([
  [
    "時刻の両端の片方だけがある",
    {
      countBy: "http.user_agent",
      valueCounts: [{ ...valueCountJson(), lastEventTime: undefined }],
      distinctValueCount: 1,
    },
  ],
  [
    "数える欄が無いのに数えた個数がある",
    {
      distinctValueCount: 3,
    },
  ],
  [
    "数える欄が無いのに値ごとの件数がある",
    {
      valueCounts: [valueCountJson()],
    },
  ],
  [
    "数える欄が無いのに 0 件の理由がある",
    {
      valueCountsEmptyReason: "no_field_observed",
    },
  ],
  [
    "数えた個数が 0 なのに理由が無い",
    {
      countBy: "http.user_agent",
      valueCounts: [],
      distinctValueCount: 0,
    },
  ],
  [
    "数えた個数が 1 以上なのに理由がある",
    {
      countBy: "http.user_agent",
      valueCounts: [valueCountJson()],
      distinctValueCount: 1,
      valueCountsEmptyReason: "no_field_observed",
    },
  ],
  [
    "数える欄があるのに数えた個数が無い",
    {
      countBy: "http.user_agent",
      valueCounts: [valueCountJson()],
    },
  ],
  [
    "数えた個数が、応答に入れた個数を下回る",
    {
      countBy: "http.user_agent",
      valueCounts: [valueCountJson()],
      distinctValueCount: 0,
    },
  ],
  [
    "対象の種別が欄を持たない理由なのに、値を持つ種別が無い",
    {
      countBy: "http.user_agent",
      valueCounts: [],
      distinctValueCount: 0,
      valueCountsEmptyReason: "field_on_other_node_kind",
    },
  ],
  [
    "値を持つ種別が空の並びである",
    {
      countBy: "http.user_agent",
      valueCounts: [],
      distinctValueCount: 0,
      valueCountsEmptyReason: "field_on_other_node_kind",
      valueCountsNodeKinds: [],
    },
  ],
  [
    "理由が別なのに値を持つ種別がある",
    {
      countBy: "http.user_agent",
      valueCounts: [],
      distinctValueCount: 0,
      valueCountsEmptyReason: "no_value_in_filter",
      valueCountsNodeKinds: ["record"],
    },
  ],
])("応答に %s とき、読めない応答として返す", async (_name, extra) => {
  stubFetch(jsonResponse(200, { ...graphResponseJson(), ...extra }));

  const result = await fetchGraph(baseRequest);

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});

test("収集元の条件を要求の source に 1 つずつ載せ、応答が用いた収集元と値を持つ種別を読む", async () => {
  const mock = stubFetch(
    jsonResponse(200, {
      ...graphResponseJson(),
      source: ["src-a", "src-b"],
      countBy: "http.user_agent",
      valueCounts: [],
      distinctValueCount: 0,
      valueCountsEmptyReason: "field_on_other_node_kind",
      valueCountsNodeKinds: ["record"],
    }),
  );

  const result = await fetchGraph({
    ...baseRequest,
    sources: ["src-a", "src-b"],
  });

  const url = new URL(String(mock.mock.calls[0]?.[0]), "http://localhost");
  expect(url.searchParams.getAll("source")).toEqual(["src-a", "src-b"]);
  expect(result.ok).toBe(true);
  if (!result.ok) {
    return;
  }
  expect(result.value.source).toEqual(["src-a", "src-b"]);
  expect(result.value.valueCountsNodeKinds).toEqual(["record"]);
});

test("レコードでないノードがレコードの要約を持つ応答を、読めない応答として返す", async () => {
  const response = graphResponseJson();
  const summary = summarizedRecordGraphResponseJson().nodes.find(
    (node) => "record" in node,
  );
  stubFetch(
    jsonResponse(200, {
      ...response,
      nodes: response.nodes.map((node, index) =>
        index === 0 && summary !== undefined && "record" in summary
          ? { ...node, record: summary.record }
          : node,
      ),
    }),
  );

  const result = await fetchGraph(baseRequest);

  expect(result.ok).toBe(false);
});

test.each([
  [
    "語彙の項目と原資料の key の両方がある",
    { semantic: "process.command_line", name: "cmd", form: "raw_text" },
  ],
  ["語彙の項目と原資料の key のどちらも無い", { form: "raw_text" }],
  [
    "値の形が契約の外にある",
    { semantic: "process.command_line", form: "comparable_value" },
  ],
])("一致した欄に %s 応答を、読めない応答として返す", async (_name, match) => {
  const response = graphResponseJson();
  stubFetch(
    jsonResponse(200, {
      ...response,
      valueContains: ["cmd", "powershell"],
      valueExcludes: ["-enc"],
      nodes: response.nodes.map((node, index) =>
        index === 0 ? { ...node, valueMatches: [match] } : node,
      ),
    }),
  );

  const result = await fetchGraph(baseRequest);

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});

test("一致した欄と値の形を読み、文字列を与えない応答では出さない", async () => {
  const response = graphResponseJson();
  stubFetch(
    jsonResponse(200, {
      ...response,
      valueContains: ["cmd", "powershell"],
      valueExcludes: ["-enc"],
      nodes: response.nodes.map((node, index) =>
        index === 0
          ? {
              ...node,
              valueMatches: [
                { name: "psProfile", form: "normalized", value: "cmd" },
              ],
            }
          : node,
      ),
    }),
  );

  const matched = await fetchGraph(baseRequest);

  expect(matched.ok).toBe(true);
  if (!matched.ok) {
    return;
  }
  expect(matched.value.valueContains).toEqual(["cmd", "powershell"]);
  expect(matched.value.valueExcludes).toEqual(["-enc"]);
  expect(matched.value.nodes[0]?.valueMatches).toEqual([
    {
      semantic: undefined,
      name: "psProfile",
      form: "normalized",
      value: "cmd",
    },
  ]);
  expect(matched.value.nodes[1]?.valueMatches).toBeUndefined();

  stubFetch(jsonResponse(200, graphResponseJson()));
  const plain = await fetchGraph(baseRequest);
  expect(plain.ok).toBe(true);
  if (!plain.ok) {
    return;
  }
  expect(plain.value.valueContains).toBeUndefined();
  expect(plain.value.nodes[0]?.valueMatches).toBeUndefined();
});

test.each([
  [
    "depth が受け取る範囲の上を超える",
    { ...baseRequest, depth: maxGraphDepth + 1 },
  ],
  [
    "depth が受け取る範囲の下を下回る",
    { ...baseRequest, depth: minGraphDepth - 1 },
  ],
  [
    "起点のノードが上限を超える",
    {
      ...baseRequest,
      nodeIds: Array.from(
        { length: maxOriginNodes + 1 },
        (_, index) => `n:ip:${index}`,
      ),
    },
  ],
] satisfies [string, GraphRequest][])(
  "%s 要求を送らずに失敗として返す",
  async (_name, request) => {
    const mock = stubFetch(jsonResponse(200, graphResponseJson()));

    const result = await fetchGraph(request);

    expect(mock).not.toHaveBeenCalled();
    expect(result.ok).toBe(false);
    if (result.ok) {
      return;
    }
    expect(result.failure.kind).toBe("request_rejected");
    expect(result.failure.summary).toBe("グラフの取得");
  },
);

test("起点のノードが上限ちょうどの要求は送る", async () => {
  const mock = stubFetch(jsonResponse(200, graphResponseJson()));

  const result = await fetchGraph({
    ...baseRequest,
    nodeIds: Array.from(
      { length: maxOriginNodes },
      (_, index) => `n:ip:${index}`,
    ),
  });

  expect(result.ok).toBe(true);
  expect(
    new URL(
      mock.mock.calls[0]?.[0] ?? "",
      "http://example.test",
    ).searchParams.getAll("nodeId").length,
  ).toBe(maxOriginNodes);
});

test("時刻の文字列を 1 つも持たない期間のフィルタを送らずに失敗として返す", async () => {
  const mock = stubFetch(jsonResponse(200, graphResponseJson()));

  const result = await fetchGraph({
    ...baseRequest,
    timeFilter: { unit: "second" },
  });

  expect(mock).not.toHaveBeenCalled();
  expect(result.ok).toBe(false);
});

test("ノードが 1 件も無い応答は emptyReason を持つ", async () => {
  stubFetch(jsonResponse(200, emptyGraphResponseJson()));

  const result = await fetchGraph(baseRequest);

  expect(result.ok).toBe(true);
  if (!result.ok) {
    return;
  }
  expect(result.value.nodeCount).toBe(0);
  expect(result.value.nodes).toEqual([]);
  expect(result.value.emptyReason).toBe("no_record_in_filter");
});

test("nodeCount が 1 以上で emptyReason を出す応答を読まない", async () => {
  stubFetch(
    jsonResponse(200, {
      ...readObject(graphResponseJson(), "fixture"),
      emptyReason: "no_record_in_filter",
    }),
  );

  const result = await fetchGraph(baseRequest);

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});

test("nodeCount が合致したノードの件数と食い違う応答を読まない", async () => {
  stubFetch(
    jsonResponse(200, {
      ...readObject(graphResponseJson(), "fixture"),
      nodeCount: 3,
    }),
  );

  const result = await fetchGraph(baseRequest);

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});
test("エッジの端点を nodes が含んでいない応答を読まない", async () => {
  const response = graphResponseJson();
  stubFetch(
    jsonResponse(200, {
      ...response,
      nodes: response.nodes.slice(0, 2),
      nodeCount: 2,
    }),
  );

  const result = await fetchGraph(baseRequest);

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});

test("同じ id を 2 件のノードが名乗る応答を読まない", async () => {
  const response = graphResponseJson();
  const [first, ...rest] = response.nodes;
  stubFetch(
    jsonResponse(200, {
      ...response,
      nodes: [first, ...rest, first],
      nodeCount: 3,
    }),
  );

  const result = await fetchGraph(baseRequest);

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});

test("同じ id を 2 本のエッジが名乗る応答を読まない", async () => {
  const response = graphResponseJson();
  const [first] = response.edges;
  stubFetch(
    jsonResponse(200, {
      ...response,
      edges: [first, first],
      edgeCount: 2,
    }),
  );

  const result = await fetchGraph(baseRequest);

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});

test("時刻を 1 つも持たない応答が filterUnit を出したときに読まない", async () => {
  stubFetch(
    jsonResponse(200, {
      ...readObject(graphResponseJson(), "fixture"),
      filterUnit: "second",
    }),
  );

  const result = await fetchGraph(baseRequest);

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});

test("ノードの詳細を取得し、識別子を path へ符号化して送る", async () => {
  const mock = stubFetch(jsonResponse(200, nodeDetailResponseJson()));

  const result = await fetchNodeDetail({
    id: processNodeId,
    matchConditions,
  });

  expect(mock.mock.calls[0]?.[0]).toBe(
    `/api/v0/nodes/n%3Aprocess%3A8ab3?${matchConditionQuery}`,
  );
  expect(result.ok).toBe(true);
  if (!result.ok) {
    return;
  }
  expect(result.value.node.id).toBe(processNodeId);
  expect(result.value.attributeCount).toBe(1);
  expect(result.value.attributes[0]?.valueCount).toBe(2);
  expect(result.value.attributes[0]?.values).toHaveLength(2);
  expect(result.value.evidenceCount).toBe(2);
  expect(result.value.edgeCounts[0]?.edgeCount).toBe(1);
  expect(result.value.edgeCounts[0]?.evidenceCount).toBe(4);
});

/** 属性 1 つの名前を差し替えた応答を組む。 */
function nodeDetailWithAttributeNames(names: {
  semantic?: string;
  name?: string;
}) {
  const detail = nodeDetailResponseJson();
  return {
    ...detail,
    attributes: [{ ...detail.attributes[0], semantic: undefined, ...names }],
  };
}

test("語彙に写していない欄の属性を、原資料の key の文字列で読む", async () => {
  stubFetch(
    jsonResponse(200, nodeDetailWithAttributeNames({ name: "psProfile" })),
  );

  const result = await fetchNodeDetail({
    id: processNodeId,
    matchConditions,
  });

  expect(result.ok).toBe(true);
  if (!result.ok) {
    return;
  }
  expect(result.value.attributes[0]?.name).toBe("psProfile");
  expect(result.value.attributes[0]?.semantic).toBeUndefined();
});

test.each([
  ["語彙の項目と原資料の key の両方がある", { semantic: "loc", name: "loc" }],
  ["語彙の項目と原資料の key のどちらも無い", {}],
])("属性に %s 応答を、読めない応答として返す", async (_name, names) => {
  stubFetch(jsonResponse(200, nodeDetailWithAttributeNames(names)));

  const result = await fetchNodeDetail({
    id: processNodeId,
    matchConditions,
  });

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});

test("識別子が空の要求を送らずに失敗として返す", async () => {
  const mock = stubFetch(jsonResponse(200, nodeDetailResponseJson()));

  const empty = await fetchNodeDetail({ id: "", matchConditions });

  expect(mock).not.toHaveBeenCalled();
  expect(empty.ok).toBe(false);
  if (empty.ok) {
    return;
  }
  expect(empty.failure.summary).toBe("ノードの詳細の取得");
});

test("ノードの詳細の要求は条件の選択だけを query に載せる", async () => {
  const mock = stubFetch(jsonResponse(200, nodeDetailResponseJson()));

  await fetchNodeDetail({ id: processNodeId, matchConditions });

  const requestedUrl = new URL(
    String(mock.mock.calls[0]?.[0]),
    "https://example.test",
  );
  expect([...requestedUrl.searchParams.keys()]).toEqual(["matchCondition"]);
});

test("attributeCount が attributes の要素数と食い違う応答を読まない", async () => {
  stubFetch(jsonResponse(200, attributeCountMismatchNodeDetailResponseJson()));

  const result = await fetchNodeDetail({ id: processNodeId, matchConditions });

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});

test("evidenceCount が evidence の要素数と食い違う応答を読まない", async () => {
  stubFetch(jsonResponse(200, evidenceCountMismatchNodeDetailResponseJson()));

  const result = await fetchNodeDetail({ id: processNodeId, matchConditions });

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});

test("一致するノードが無い応答を分類し、code に応じた文言を返す", async () => {
  stubFetch(jsonResponse(404, apiErrorJson("record_not_found")));

  const result = await fetchNodeDetail({
    id: processNodeId,
    matchConditions,
  });

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("request_rejected");
  expect(result.failure.failureCode).toBe("record_not_found");
  expect(result.failure.failureDescription).toBe("レコードかノードなし");
});

test("失敗の応答が ApiError の形でないときも HTTP の status で分類する", async () => {
  stubFetch(textResponse(503, "service unavailable"));

  const result = await fetchGraph(baseRequest);

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("server");
  expect(result.failure.failureCode).toBeUndefined();
  expect(result.failure.failureDescription).toBeUndefined();
});

test("通信そのものが失敗したときにネットワーク断として分類する", async () => {
  stubFetch(new TypeError("Failed to fetch"));

  const result = await fetchGraph(baseRequest);

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("network");
});

const edgeDetailPath = `/api/v0/edges/${encodeURIComponent(remoteSessionEdgeId)}`;

test("関係の詳細を取得し、根拠の区分とレコードに現れたアカウントを読む", async () => {
  const mock = stubFetch(jsonResponse(200, edgeDetailResponseJson()));

  const result = await fetchEdgeDetail({
    id: remoteSessionEdgeId,
    matchConditions,
  });

  expect(mock.mock.calls[0]?.[0]).toBe(
    `${edgeDetailPath}?${matchConditionQuery}`,
  );
  expect(result.ok).toBe(true);
  if (!result.ok) {
    return;
  }
  const groups = result.value.evidenceGroups;
  const portRawTextOf = (group: EdgeEvidenceGroup) => {
    const port = group.destinationPort;
    return port?.kind === "text" ? port.text.rawText : undefined;
  };
  expect(groups.map(portRawTextOf)).toEqual([
    "5985",
    "445",
    undefined,
    undefined,
  ]);
  expect(groups.map((group) => group.evidenceCount)).toEqual([1, 1, 1, 1]);
  // **接続先 port を持たない区分は、持たない理由を持つ。** 0 で埋めない。
  // 欄が無い区分と、欄の値を比べられない区分は別の理由を持つ。
  expect(groups[2]?.destinationPortAbsence).toBe(
    "区分のレコードが接続先 port の欄を持っていません",
  );
  expect(groups[3]?.destinationPortAbsence).toBe(
    "区分のレコードの接続先 port の値を比べられません",
  );
  expect(groups[0]?.destinationPortAbsence).toBeUndefined();
  // **区分を指す値を応答が含む。** 指せない区分は指せない理由を持つ。
  expect(groups[0]?.selector).toEqual({
    eventCategory: "net",
    eventAction: "acpt",
    destinationPort: "5985",
    destinationPortAbsent: undefined,
    logonType: undefined,
    logonTypeAbsent: true,
  });
  expect(groups[2]?.selector?.destinationPortAbsent).toBe(true);
  // **ログオンの種別を持たない区分は、持たない理由を持つ。** 0 で埋めない。
  expect(groups.map((group) => group.logonType)).toEqual([
    undefined,
    undefined,
    undefined,
    undefined,
  ]);
  expect(groups[0]?.logonTypeAbsence).toBe(logonTypeAbsentReason);
  expect(groups[3]?.selector).toBeUndefined();
  expect(groups[3]?.selectorAbsence).toBe(
    "区分のレコードの接続先 port の値を比べられないため、要求で指せません",
  );
  // **レコードに現れたアカウントへ識別子で辿れる。**
  expect(groups[2]?.accounts.map((account) => account.node.id)).toEqual([
    accountNodeId,
  ]);
  expect(groups[2]?.accounts[0]?.node.label.rawText).toBe("user02");
  expect(groups[0]?.accounts).toEqual([]);
  expect(result.value.sourceNode.id).toBe(clientTerminalNodeId);
  expect(result.value.targetNode.id).toBe(terminalNodeId);
});

test("区分を指した要求は、応答が返した値をそのまま query に載せる", async () => {
  const mock = stubFetch(jsonResponse(200, edgeDetailOfPort5985Json()));

  const result = await fetchEdgeDetail({
    id: remoteSessionEdgeId,
    matchConditions,
    selector: {
      eventCategory: "net",
      eventAction: "acpt",
      destinationPort: "5985",
    },
  });

  expect(mock.mock.calls[0]?.[0]).toBe(
    `${edgeDetailPath}?eventCategory=net&eventAction=acpt` +
      `&destinationPort=5985&${matchConditionQuery}`,
  );
  expect(result.ok).toBe(true);
  if (!result.ok) {
    return;
  }
  expect(result.value.edge.evidenceCount).toBe(1);
  // **区分の一覧はフィルタで変わらない。** 他の区分の件数を同じ応答で読める。
  expect(result.value.evidenceGroups.length).toBe(4);
});

test("接続先 port の欄の不在を指す区分は、値ではなく flag を query に載せる", async () => {
  const mock = stubFetch(jsonResponse(200, edgeDetailResponseJson()));

  await fetchEdgeDetail({
    id: remoteSessionEdgeId,
    matchConditions,
    selector: {
      eventCategory: "session",
      eventAction: "loginR",
      destinationPortAbsent: true,
    },
  });

  expect(mock.mock.calls[0]?.[0]).toBe(
    `${edgeDetailPath}?eventCategory=session&eventAction=loginR` +
      `&destinationPortAbsent=true&${matchConditionQuery}`,
  );
});

test("ログオンの種別の区分は、種別のコードか種別の不在の flag を query に載せる", async () => {
  const mock = stubFetch(jsonResponse(200, edgeDetailResponseJson()));
  const selector = {
    eventCategory: "os",
    eventAction: "evtLog",
    destinationPortAbsent: true,
  };

  await fetchEdgeDetail({
    id: remoteSessionEdgeId,
    matchConditions,
    selector: { ...selector, logonType: "10" },
  });
  await fetchEdgeDetail({
    id: remoteSessionEdgeId,
    matchConditions,
    selector: { ...selector, logonTypeAbsent: true },
  });

  const base = `${edgeDetailPath}?eventCategory=os&eventAction=evtLog&destinationPortAbsent=true`;
  expect(mock.mock.calls[0]?.[0]).toBe(
    `${base}&logonType=10&${matchConditionQuery}`,
  );
  expect(mock.mock.calls[1]?.[0]).toBe(
    `${base}&logonTypeAbsent=true&${matchConditionQuery}`,
  );
});

test("区分を指さない要求は条件の選択だけを query に載せる", async () => {
  const mock = stubFetch(jsonResponse(200, edgeDetailResponseJson()));

  await fetchEdgeDetail({ id: remoteSessionEdgeId, matchConditions });

  const requestedUrl = new URL(
    String(mock.mock.calls[0]?.[0]),
    "https://example.test",
  );
  expect([...requestedUrl.searchParams.keys()]).toEqual(["matchCondition"]);
});

test("案件を指した関係の詳細の要求は、案件を query に載せる", async () => {
  const mock = stubFetch(jsonResponse(200, edgeDetailResponseJson()));

  await fetchEdgeDetail({
    id: remoteSessionEdgeId,
    matchConditions,
    caseId: challengeCaseId,
  });

  expect(mock.mock.calls[0]?.[0]).toBe(
    `${edgeDetailPath}?case=${challengeCaseId}&${matchConditionQuery}`,
  );
});

test("案件を指した部分グラフの要求は案件を query に載せ、指さない要求は載せない", async () => {
  const mock = stubFetch(jsonResponse(200, graphResponseJson()));

  await fetchGraph({ ...baseRequest, caseId: baselineCaseId });
  await fetchGraph(baseRequest);

  expect(mock.mock.calls[0]?.[0]).toBe(
    `/api/v0/graph?depth=1&case=${baselineCaseId}` + `&${matchConditionQuery}`,
  );
  const plainUrl = new URL(
    String(mock.mock.calls[1]?.[0]),
    "https://example.test",
  );
  expect(plainUrl.searchParams.has("case")).toBe(false);
});

/**
 * 区分 1 つを差し替えた応答を読み、読み込みが退けることを確かめる。
 *
 * `rejectedAt` は、区分だけを読み込んだときに退けた項目の path である。応答全体の失敗は
 * 項目を挙げないため、どの検査が退けたかを区分の読み込みで確かめる。
 */
async function expectRejectedGroup(group: object, rejectedAt: string) {
  expect(() => decodeEdgeEvidenceGroup(group, "group")).toThrow(rejectedAt);
  stubFetch(
    jsonResponse(200, { ...edgeDetailResponseJson(), evidenceGroups: [group] }),
  );

  const result = await fetchEdgeDetail({
    id: remoteSessionEdgeId,
    matchConditions,
  });

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
}

test("区分を指す値と、指せない理由をどちらも持たない応答を退ける", async () => {
  const response = edgeDetailResponseJson();
  await expectRejectedGroup(
    {
      observationKind: response.evidenceGroups[0]?.observationKind,
      destinationPort: response.evidenceGroups[0]?.destinationPort,
      logonTypeAbsence: logonTypeAbsentReason,
      accounts: [],
      authentications: [],
      evidenceCount: 1,
    },
    "group.selector",
  );
});

test("ログオンの種別と出ない理由を両方持つ区分の応答を退ける", async () => {
  const response = edgeDetailResponseJson();
  await expectRejectedGroup(
    {
      ...response.evidenceGroups[0],
      logonType: {
        name: "EventData.LogonType",
        semantic: "event.logon_type",
        kind: "text",
        text: { rawText: "3", valueState: "present" },
      },
    },
    "group.logonType",
  );
});

test("ログオンの種別と出ない理由をどちらも持たない区分の応答を退ける", async () => {
  const { logonTypeAbsence: _omitted, ...withoutLogonType } =
    edgeDetailResponseJson().evidenceGroups[0] ?? {};
  await expectRejectedGroup(withoutLogonType, "group.logonType");
});

test("ログオンの種別と種別の不在の flag を両方持つ区分を指す値の応答を退ける", async () => {
  const group = edgeDetailResponseJson().evidenceGroups[0];
  await expectRejectedGroup(
    { ...group, selector: { ...group?.selector, logonType: "3" } },
    "group.selector.logonType",
  );
});

test("ログオンの種別と種別の不在の flag をどちらも持たない区分を指す値の応答を退ける", async () => {
  const group = edgeDetailResponseJson().evidenceGroups[0];
  const { logonTypeAbsent: _omitted, ...selector } = group?.selector ?? {};
  await expectRejectedGroup({ ...group, selector }, "group.selector.logonType");
});

test("接続先 port と出ない理由を両方持つ区分の応答を退ける", async () => {
  const response = edgeDetailResponseJson();
  stubFetch(
    jsonResponse(200, {
      ...response,
      evidenceGroups: [
        {
          ...response.evidenceGroups[0],
          destinationPortAbsence: "両方を持つ区分",
        },
      ],
    }),
  );

  const result = await fetchEdgeDetail({
    id: remoteSessionEdgeId,
    matchConditions,
  });

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});

test("接続先 port と出ない理由をどちらも持たない区分の応答を退ける", async () => {
  const response = edgeDetailResponseJson();
  stubFetch(
    jsonResponse(200, {
      ...response,
      evidenceGroups: [
        {
          observationKind: response.evidenceGroups[0]?.observationKind,
          accounts: [],
          authentications: [],
          evidenceCount: 1,
        },
      ],
    }),
  );

  const result = await fetchEdgeDetail({
    id: remoteSessionEdgeId,
    matchConditions,
  });

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});

test("識別子が空の要求を送らずに失敗として返す", async () => {
  const mock = stubFetch(jsonResponse(200, edgeDetailResponseJson()));

  const empty = await fetchEdgeDetail({ id: "", matchConditions });

  expect(mock).not.toHaveBeenCalled();
  expect(empty.ok).toBe(false);
  if (empty.ok) {
    return;
  }
  expect(empty.failure.summary).toBe("エッジの詳細の取得");
});

test("matchCount が matches の要素数と食い違う応答を読まない", async () => {
  stubFetch(jsonResponse(200, matchCountMismatchEdgeDetailResponseJson()));

  const result = await fetchEdgeDetail({
    id: remoteSessionEdgeId,
    matchConditions,
  });

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});

test("エッジの evidenceCount が evidence の要素数と食い違う応答を読まない", async () => {
  stubFetch(jsonResponse(200, evidenceCountMismatchEdgeDetailResponseJson()));

  const result = await fetchEdgeDetail({
    id: remoteSessionEdgeId,
    matchConditions,
  });

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});
