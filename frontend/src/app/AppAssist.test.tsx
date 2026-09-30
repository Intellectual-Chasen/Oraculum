// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import {
  figureRequests,
  findNodeList,
} from "@/features/graphExplore/graphExploreTestHarness";
import { assistMatchConditionsOf } from "@/shared/api/assistRelay";
import { everyMatchCondition } from "@/shared/api/matchConditions";
import { emptyAssertionsResponseJson } from "@/testdata/assertions/assertionsResponse";
import { eventKindsResponseJson } from "@/testdata/eventKinds/eventKindsResponse";
import {
  graphResponseJson,
  nodeDetailResponseJson,
} from "@/testdata/graph/graphResponse";
import { jsonResponse } from "@/testdata/http";
import { hostALogRecordResponseJson } from "@/testdata/records/recordResponse";
import {
  apiErrorJson,
  hostALogSha256,
  hostALogSourceId,
  sourcesResponseJson,
} from "@/testdata/sources/sourcesResponse";
import { App } from "./App";

// WebGL の renderer は jsdom で動かない。本 test は図を stub に置き換える。
vi.mock("@/features/graphExplore/SubgraphCanvas", () => ({
  SubgraphCanvas: () => <p>部分グラフの図</p>,
}));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const conversationId = "00112233445566778899aabbccddeeff";

/** 検索の文字列と図の条件を持つ card。 */
const searchCard = {
  sequence: 1,
  turnId: "t1",
  kind: "search_query_card",
  searchQuery: {
    depth: 2,
    valueContains: ["whoami"],
    searchExpression: "process.name contains whoami",
    nodeKinds: ["process"],
    addressInCidr: "198.51.100.0/24",
    timeFrom: "2026-01-02T03:04:05Z",
    timeFromPrecision: "second",
    filterUnit: "second",
  },
  explanation: "whoami を実行したプロセスを見ます。",
  matchConditions: assistMatchConditionsOf(everyMatchCondition()),
};

/**
 * 中継と server の応答を返す。中継は card を持つ会話を持つ。figure は図の要求への応答を
 * 替える。undefined を返した要求は既定の部分グラフを返す。
 */
function stubFetch(
  card: unknown = searchCard,
  figure: (input: string) => Response | undefined = () => undefined,
) {
  const mock = vi.fn(async (input: string, init?: RequestInit) => {
    if (input === `/assist/conversations/${conversationId}/turns`) {
      const { turnId } = JSON.parse(String(init?.body)) as { turnId: string };
      return new Response(
        [
          { sequence: 2, turnId, kind: "user_message", text: "説明して" },
          { sequence: 3, turnId, kind: "turn_end" },
        ]
          .map((event) => `${JSON.stringify(event)}\n`)
          .join(""),
        { status: 200 },
      );
    }
    if (input.startsWith("/api/v0/records")) {
      return jsonResponse(200, hostALogRecordResponseJson());
    }
    if (input.startsWith("/api/v0/nodes/")) {
      return jsonResponse(200, nodeDetailResponseJson());
    }
    if (input === "/assist/status") {
      return jsonResponse(200, {
        relay: "oraculum-assist",
        providers: ["claude"],
      });
    }
    if (input === "/assist/conversations") {
      return jsonResponse(200, {
        conversations: [
          {
            id: conversationId,
            provider: "claude",
            eventCount: 1,
            answering: false,
          },
        ],
      });
    }
    if (input.startsWith(`/assist/conversations/${conversationId}/events`)) {
      return jsonResponse(200, { answering: false, events: [card] });
    }
    if (input.startsWith("/api/v0/sources")) {
      return jsonResponse(200, sourcesResponseJson());
    }
    if (input.startsWith("/api/v0/event-kinds")) {
      return jsonResponse(200, eventKindsResponseJson());
    }
    if (input.startsWith("/api/v0/assertions")) {
      return jsonResponse(200, emptyAssertionsResponseJson());
    }
    return figure(input) ?? jsonResponse(200, graphResponseJson());
  });
  vi.stubGlobal("fetch", mock);
  return mock;
}

const conversationView = "AI Chat";

/** タブの文字列が title に一致するビューを前面に出す。 */
function openView(title: string) {
  const tab = screen.getByRole("tab", { name: title });
  fireEvent.pointerDown(tab);
  fireEvent.mouseDown(tab);
  fireEvent.click(tab);
}

/** 図の要求のうち、最後に送ったもの。端末の選択肢の要求を除く。 */
function lastFigureRequest(mock: ReturnType<typeof stubFetch>) {
  return figureRequests(mock).at(-1);
}

test("AI が勧めた検索の条件を適用すると図の要求が変わり、元の条件に戻すと前の要求へ戻る", async () => {
  const mock = stubFetch();
  render(<App />);
  await findNodeList();
  openView(conversationView);

  const card = await screen.findByRole("region", {
    name: "AI が勧めた検索の条件",
  });
  await waitFor(() => expect(lastFigureRequest(mock)).toBeDefined());
  const before = lastFigureRequest(mock);
  expect(before).toContain(
    "nodeKind=terminal&nodeKind=ip&granularity=object&depth=1",
  );

  fireEvent.click(
    within(card).getByRole("button", { name: "検索の条件に適用" }),
  );

  await waitFor(() => {
    const applied = lastFigureRequest(mock) ?? "";
    expect(applied).toContain("nodeKind=process&granularity=object&depth=2");
    expect(applied).toContain("valueContains=whoami");
    expect(applied).toContain("searchExpression=process.name+contains+whoami");
    expect(applied).toContain("addressInCidr=198.51.100.0%2F24");
  });
  // 検索欄の chip も、適用した条件を出す。
  const conditionList = screen.getByRole("list", {
    name: "適用している検索の条件",
  });
  expect(within(conditionList).getByText("whoami")).toBeTruthy();
  expect(within(conditionList).getByText("198.51.100.0/24")).toBeTruthy();
  expect(within(conditionList).getByText(/2026-01-02T03:04:05Z/)).toBeTruthy();

  fireEvent.click(
    within(card).getByRole("button", { name: "適用前の条件に復元" }),
  );
  await waitFor(() => expect(lastFigureRequest(mock)).toBe(before));
  expect(
    screen.queryByRole("list", { name: "適用している検索の条件" }),
  ).toBeNull();
});

/** 会話のビューで text を送り、中継へ送った発言の文脈を返す。 */
async function sendTurn(mock: ReturnType<typeof stubFetch>, text: string) {
  const turnsPath = `/assist/conversations/${conversationId}/turns`;
  const sentBefore = mock.mock.calls.filter(
    (call) => call[0] === turnsPath,
  ).length;
  fireEvent.change(screen.getByLabelText("AI への発言"), {
    target: { value: text },
  });
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "発言を送信" }),
    ).not.toHaveAttribute("aria-disabled"),
  );
  fireEvent.click(screen.getByRole("button", { name: "発言を送信" }));
  await waitFor(() =>
    expect(
      mock.mock.calls.filter((call) => call[0] === turnsPath).length,
    ).toBeGreaterThan(sentBefore),
  );
  const turn = mock.mock.calls.filter((call) => call[0] === turnsPath).at(-1);
  return (
    JSON.parse(String(turn?.[1]?.body)) as {
      context: {
        searchQuery?: { nodeKinds?: string[]; granularity?: string };
        nodeKindsFromConditions?: boolean;
      };
    }
  ).context;
}

test("発言の文脈は、画面が図に出している対象の種別と、種別を検索の条件から決めているかを添える", async () => {
  const mock = stubFetch();
  render(<App />);
  await findNodeList();
  openView(conversationView);
  const card = await screen.findByRole("region", {
    name: "AI が勧めた検索の条件",
  });
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toContain(
      "nodeKind=terminal&nodeKind=ip&granularity=object",
    ),
  );

  // 検索の条件から決める選び方では、画面が決めた端末と IP アドレスの種別を添える。
  const automatic = await sendTurn(mock, "今の図を説明して");
  expect(automatic.searchQuery?.nodeKinds).toEqual(["terminal", "ip"]);
  expect(automatic.searchQuery?.granularity).toBe("object");
  expect(automatic.nodeKindsFromConditions).toBe(true);

  // card を適用すると、card が決めたプロセスの種別を手で選んだ選び方になる。
  fireEvent.click(
    within(card).getByRole("button", { name: "検索の条件に適用" }),
  );
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toContain(
      "nodeKind=process&granularity=object",
    ),
  );
  const manual = await sendTurn(mock, "適用した図を説明して");
  expect(manual.searchQuery?.nodeKinds).toEqual(["process"]);
  expect(manual.nodeKindsFromConditions).toBeUndefined();
});

test("起点を持つ card を適用すると、ノードID の条件で図を要求し、発言の文脈に起点を添え、失敗しても条件を外せる", async () => {
  const originCard = {
    sequence: 1,
    turnId: "t1",
    kind: "tool_result",
    toolName: "graph_search",
    toolUseSequence: 1,
    toolResult: "{}",
    searchQuery: { depth: 2, nodeIds: ["n:process:9"] },
    origins: [{ id: "n:process:9", kind: "process", label: "whoami.exe" }],
    matchConditions: assistMatchConditionsOf(everyMatchCondition()),
  };
  const toolUse = {
    sequence: 1,
    turnId: "t1",
    kind: "tool_use",
    toolName: "graph_search",
    toolInput: {},
  };
  // 起点を持つ図の要求は、起点のノードが取り込み結果に無いものとして失敗させる。
  const mock = stubFetch(originCard, (input) =>
    input.includes("nodeId=n%3Aprocess%3A9")
      ? jsonResponse(404, apiErrorJson("record_not_found"))
      : undefined,
  );
  const relayed = mock.getMockImplementation();
  mock.mockImplementation(async (input: string, init?: RequestInit) =>
    input.startsWith(`/assist/conversations/${conversationId}/events`)
      ? jsonResponse(200, {
          answering: false,
          events: [toolUse, { ...originCard, sequence: 2 }],
        })
      : (relayed?.(input, init) ?? jsonResponse(500, {})),
  );
  render(<App />);
  await findNodeList();
  openView(conversationView);
  const conversation = await screen.findByRole("list", { name: "会話" });
  await waitFor(() => expect(lastFigureRequest(mock)).toBeDefined());
  const before = lastFigureRequest(mock);

  fireEvent.click(
    await within(conversation).findByRole("button", {
      name: "検索の条件に適用",
    }),
  );
  await waitFor(() => {
    const applied = lastFigureRequest(mock) ?? "";
    expect(applied).toContain("nodeId=n%3Aprocess%3A9");
    expect(applied).toContain("depth=2");
    // 自動の選び方は起点を条件に数え、端末と IP アドレスだけに絞らない。
    expect(applied).not.toContain("nodeKind=terminal");
  });
  const conditionList = screen.getByRole("list", {
    name: "適用している検索の条件",
  });
  expect(within(conditionList).getByText("whoami.exe")).toBeTruthy();

  const context = await sendTurn(mock, "起点から説明して");
  expect(
    (context.searchQuery as { nodeIds?: string[] } | undefined)?.nodeIds,
  ).toEqual(["n:process:9"]);

  // 図の取得が失敗したときは、ノードID の条件のノードが無いことを示し、その条件を外せる。
  await screen.findByText("ノードID のノードなし");
  fireEvent.click(
    within(conditionList).getByRole("button", {
      name: "ノードID whoami.exe を削除",
    }),
  );
  await waitFor(() => {
    const removed = lastFigureRequest(mock) ?? "";
    expect(removed).not.toContain("nodeId=");
    expect(removed).toContain("depth=2");
  });
  await waitFor(() =>
    expect(screen.queryByText("ノードID のノードなし")).toBeNull(),
  );
  expect(before).not.toContain("nodeId=");
});

test("開いたレコードの「AI に説明させる」は、会話のビューを前面に出し、そのレコードを文脈に添えて送る", async () => {
  const mock = stubFetch();
  render(<App />);

  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );
  fireEvent.click(
    await screen.findByRole("button", {
      name: "Record に表示: 収集元: host-a.log、ID: 112、行: 1022",
    }),
  );
  const explainLabel = "AI にレコードの説明を依頼";
  await screen.findByRole("button", { name: explainLabel });
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: explainLabel }),
    ).not.toHaveAttribute("aria-disabled"),
  );
  fireEvent.click(screen.getByRole("button", { name: explainLabel }));

  await waitFor(() =>
    expect(screen.getByRole("tab", { name: conversationView })).toHaveAttribute(
      "aria-selected",
      "true",
    ),
  );
  await within(await screen.findByRole("list", { name: "会話" })).findByText(
    "説明して",
  );
  const turn = mock.mock.calls.find(
    (call) => call[0] === `/assist/conversations/${conversationId}/turns`,
  );
  const body = JSON.parse(String(turn?.[1]?.body)) as {
    context: { records: { sourceId: string; sourceContentSha256: string }[] };
  };
  expect(
    body.context.records.map((record) => [
      record.sourceId,
      record.sourceContentSha256,
    ]),
  ).toEqual([[hostALogSourceId, hostALogSha256]]);
});

test("中継が無い画面は、開いたレコードに「AI に説明させる」を出さない", async () => {
  const mock = stubFetch();
  const relayed = mock.getMockImplementation();
  mock.mockImplementation(async (input: string, init?: RequestInit) =>
    input === "/assist/status"
      ? jsonResponse(404, { message: "not found" })
      : (relayed?.(input, init) ?? jsonResponse(500, {})),
  );
  render(<App />);

  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );
  fireEvent.click(
    await screen.findByRole("button", {
      name: "Record に表示: 収集元: host-a.log、ID: 112、行: 1022",
    }),
  );
  await screen.findByRole("table", { name: "フィールド" });
  expect(
    Array.from(document.querySelectorAll(".value-pairs > li")).some(
      (item) => item.textContent === "中継: 未接続",
    ),
  ).toBe(true);
  expect(
    screen.queryByRole("button", { name: "AI にレコードの説明を依頼" }),
  ).toBeNull();
});
