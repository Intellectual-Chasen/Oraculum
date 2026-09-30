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
import type { AssistTurnContext } from "@/shared/api/assistRelay";
import type {
  AssistMatchCondition,
  AssistOrigin,
} from "@/shared/contracts/assistRelay";
import type { SearchQuery } from "@/shared/contracts/searchQuery";
import { jsonResponse, textResponse } from "@/testdata/http";
import { AssistConversation } from "./AssistConversation";
import { useAssistConversation } from "./useAssistConversation";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const conversationId = "00112233445566778899aabbccddeeff";
const currentConditions: AssistMatchCondition[] = [
  { conditionKey: "destination_ip", tolerance: 0 },
];
const context: AssistTurnContext = {
  matchConditions: currentConditions,
  searchQuery: { depth: 1 },
};
const terminals = {
  status: "loaded" as const,
  value: [{ id: "n:terminal:1", label: "HOST-A" }],
};
const cardQuery: SearchQuery = {
  depth: 2,
  valueContains: ["whoami"],
  fieldEquals: ["LogonType=3"],
  searchExpression: 'process.name == "whoami.exe"',
  terminal: "n:terminal:1",
  nodeKinds: ["process"],
};

function event(sequence: number, fields: Record<string, unknown>) {
  return { sequence, turnId: "t1", ...fields };
}

/** 改行で区切った JSON の応答を作る。 */
function ndjsonResponse(items: unknown[]): Response {
  return new Response(
    items.map((item) => `${JSON.stringify(item)}\n`).join(""),
    { status: 200, headers: { "content-type": "application/x-ndjson" } },
  );
}

type Routes = {
  status?: () => Response;
  conversations?: () => Response;
  open?: () => Response;
  events?: () => Response;
  turn?: () => Response;
};

/** 中継の path ごとに応答を返す fetch の mock。要求の本文を集める。 */
function stubRelay(routes: Routes) {
  const bodies: { path: string; body: unknown }[] = [];
  const mock = vi.fn(async (input: string, init?: RequestInit) => {
    if (init?.body !== undefined) {
      bodies.push({ path: input, body: JSON.parse(String(init.body)) });
    }
    const route =
      input === "/assist/status"
        ? routes.status
        : input === "/assist/conversations"
          ? init?.method === "POST"
            ? routes.open
            : routes.conversations
          : input.endsWith("/turns")
            ? routes.turn
            : input.includes("/events?")
              ? routes.events
              : undefined;
    if (route === undefined) {
      throw new Error(`unexpected request: ${input}`);
    }
    return route();
  });
  vi.stubGlobal("fetch", mock);
  return { mock, bodies };
}

const available = () =>
  jsonResponse(200, { relay: "oraculum-assist", providers: ["claude"] });
const noConversations = () => jsonResponse(200, { conversations: [] });

function Harness({
  onApply = () => () => {},
}: {
  onApply?: (query: SearchQuery) => () => void;
}) {
  return (
    <AssistConversation
      view={useAssistConversation()}
      turnContext={() => context}
      currentMatchConditions={currentConditions}
      terminals={terminals}
      onApplySearchQuery={onApply}
    />
  );
}

const sendLabel = "発言を送信";

/** 「中継: 状態」の組の文字列。 */
function relayState(): string | undefined {
  return Array.from(document.querySelectorAll("li"))
    .map((item) => item.textContent ?? "")
    .find((text) => text.startsWith("中継: "));
}

test("中継の状態を読めない画面は、未接続と中継の起動方法の help を出し、発言の欄を出さない", async () => {
  stubRelay({ status: () => textResponse(404, "not found") });
  render(<Harness />);
  await waitFor(() => expect(relayState()).toBe("中継: 未接続"));
  expect(
    screen.getByRole("button", { name: "中継の起動 の説明" }),
  ).toBeTruthy();
  expect(screen.getByRole("button", { name: "中継を再確認" })).toBeTruthy();
  expect(screen.queryByLabelText("AI への発言")).toBeNull();
});

test("発言を送ると会話を始め、応答の文と検索の条件の card を出し、適用と元に戻す操作を渡す", async () => {
  const { bodies } = stubRelay({
    status: available,
    conversations: noConversations,
    open: () =>
      jsonResponse(201, {
        conversation: {
          id: conversationId,
          provider: "claude",
          eventCount: 0,
          answering: false,
        },
      }),
    turn: () =>
      ndjsonResponse([
        event(1, { kind: "user_message", text: "whoami を調べて" }),
        event(2, { kind: "tool_use", toolName: "mcp__oraculum__graph_search" }),
        event(3, { kind: "text", text: "1 行目\nhttps://example.test/x" }),
        event(4, {
          kind: "search_query_card",
          searchQuery: cardQuery,
          explanation: "実行した‮プロセス",
          matchConditions: currentConditions,
        }),
        event(5, { kind: "turn_end" }),
      ]),
  });
  const restore = vi.fn();
  const onApply = vi.fn((_query: SearchQuery) => restore);
  render(<Harness onApply={onApply} />);

  fireEvent.change(await screen.findByLabelText("AI への発言"), {
    target: { value: "whoami を調べて" },
  });
  fireEvent.click(screen.getByRole("button", { name: sendLabel }));

  const conversation = await screen.findByRole("list", { name: "会話" });
  await within(conversation).findByText(/1 行目/);
  // 中継が受け付けた発言は、欄から消す。
  await waitFor(() =>
    expect(
      (screen.getByLabelText("AI への発言") as HTMLTextAreaElement).value,
    ).toBe(""),
  );
  expect(bodies.map((item) => item.path)).toEqual([
    "/assist/conversations",
    `/assist/conversations/${conversationId}/turns`,
  ]);
  expect(bodies[0]?.body).toEqual({ provider: "claude" });
  const turn = bodies[1]?.body as { turnId: string; text: string };
  expect(turn.text).toBe("whoami を調べて");
  expect(turn.turnId).toMatch(/^[0-9a-f]{32}$/);
  expect(bodies[1]?.body).toMatchObject({ context });

  // 会話は文字列として描き、URL をリンクにしない。
  expect(within(conversation).queryByRole("link")).toBeNull();
  expect(
    Array.from(conversation.querySelectorAll("li")).some(
      (item) => item.textContent === "tool: graph_search",
    ),
  ).toBe(true);

  const card = within(conversation).getByRole("region", {
    name: "AI が勧めた検索の条件",
  });
  const chips = within(card).getByRole("list", { name: "勧めた検索の条件" });
  expect(within(chips).getByText("whoami")).toBeTruthy();
  // 完全一致の組と検索式も、検索欄の chip と同じ名前で出す。
  expect(within(chips).getByText("フィールドと文字列の完全一致")).toBeTruthy();
  expect(within(chips).getByText("LogonType=3")).toBeTruthy();
  expect(within(chips).getByText("検索式")).toBeTruthy();
  expect(within(chips).getByText('process.name == "whoami.exe"')).toBeTruthy();
  // 端末は選択肢の表示名で出す。
  expect(within(chips).getByText("HOST-A")).toBeTruthy();
  expect(within(chips).getByText("プロセス")).toBeTruthy();
  // LLM の説明は印を付けた別の欄に出し、bidi 制御の文字を可視の符号にする。
  expect(within(card).getByText("AI の説明")).toBeTruthy();
  expect(within(card).getByRole("img", { name: /U\+202E/ })).toBeTruthy();
  expect(within(card).queryByRole("note")).toBeNull();

  const restoreButton = () =>
    within(card).getByRole("button", { name: "適用前の条件に復元" });
  expect(restoreButton()).toHaveAttribute("aria-disabled", "true");
  fireEvent.click(
    within(card).getByRole("button", { name: "検索の条件に適用" }),
  );
  expect(onApply).toHaveBeenCalledWith(cardQuery, undefined);
  expect(restoreButton()).not.toHaveAttribute("aria-disabled");
  fireEvent.click(restoreButton());
  expect(restore).toHaveBeenCalledTimes(1);
  expect(restoreButton()).toHaveAttribute("aria-disabled", "true");
});

/** 行を後から足せる NDJSON の応答。stream は close を呼ぶまで開いたままにする。 */
function heldNdjsonResponse() {
  let controller: ReadableStreamDefaultController<Uint8Array> | undefined;
  const body = new ReadableStream<Uint8Array>({
    start(opened) {
      controller = opened;
    },
  });
  const encoder = new TextEncoder();
  return {
    response: new Response(body, {
      status: 200,
      headers: { "content-type": "application/x-ndjson" },
    }),
    /** 渡した行を 1 つの chunk として足す。 */
    push: (...items: unknown[]) =>
      controller?.enqueue(
        encoder.encode(
          items.map((item) => `${JSON.stringify(item)}\n`).join(""),
        ),
      ),
    close: () => controller?.close(),
  };
}

/** 発言の要求の本文から、画面が作った発言の識別子を読む。 */
function turnIdOf(body: unknown): string {
  return (body as { turnId: string }).turnId;
}

const openedConversation = () =>
  jsonResponse(201, {
    conversation: {
      id: conversationId,
      provider: "claude",
      eventCount: 0,
      answering: false,
    },
  });

test("中継が発言を受け付けた時点で、応答の完了を待たずに発言の欄を空にする", async () => {
  const held = heldNdjsonResponse();
  const { bodies } = stubRelay({
    status: available,
    conversations: noConversations,
    open: openedConversation,
    turn: () => held.response,
  });
  render(<Harness />);
  const draft = (await screen.findByLabelText(
    "AI への発言",
  )) as HTMLTextAreaElement;
  fireEvent.change(draft, { target: { value: "調べて" } });
  fireEvent.click(screen.getByRole("button", { name: sendLabel }));
  await waitFor(() => expect(bodies).toHaveLength(2));
  const turnId = turnIdOf(bodies[1]?.body);
  held.push({ sequence: 1, turnId, kind: "user_message", text: "調べて" });
  await within(await screen.findByRole("list", { name: "会話" })).findByText(
    "調べて",
  );
  await waitFor(() => expect(draft.value).toBe(""));
  expect(screen.getByText("AI の応答中")).toBeTruthy();
  held.push({ sequence: 2, turnId, kind: "turn_end" });
  held.close();
  await waitFor(() => expect(screen.queryByText("AI の応答中")).toBeNull());
});

const toolQuery: SearchQuery = {
  depth: 1,
  nodeKinds: ["process"],
  nodeIds: ["n:process:1"],
};
const toolOrigins: AssistOrigin[] = [
  { id: "n:process:1", kind: "process", label: "cmd.exe" },
];

/** 中継が持つ会話 1 件と、その event を読み直す経路。 */
function storedConversation(events: unknown[]): Routes {
  return {
    status: available,
    conversations: () =>
      jsonResponse(200, {
        conversations: [
          {
            id: conversationId,
            provider: "claude",
            eventCount: events.length,
            answering: false,
          },
        ],
      }),
    events: () => jsonResponse(200, { answering: false, events }),
  };
}

test("tool の呼び出しは状態を出し、開くと入力と結果を出し、画面で表せる検索の結果を適用できる", async () => {
  stubRelay(
    storedConversation([
      event(1, { kind: "user_message", text: "調べて" }),
      event(2, {
        kind: "tool_use",
        toolName: "graph_search",
        toolInput: { searchQuery: toolQuery },
      }),
      event(3, {
        kind: "tool_result",
        toolName: "graph_search",
        toolUseSequence: 2,
        toolResult: '{"matchedNodeCount":3,"note":"a‮b"}',
        searchQuery: toolQuery,
        origins: toolOrigins,
      }),
      event(4, {
        kind: "tool_use",
        toolName: "record_text",
        toolInput: { refs: ["r1"] },
      }),
      event(5, {
        kind: "tool_use",
        toolName: "target_detail",
        toolInput: { ref: "n9" },
      }),
      event(6, {
        kind: "tool_result",
        toolName: "target_detail",
        toolUseSequence: 5,
        toolResult: "unknown reference",
        toolFailed: true,
      }),
      event(7, { kind: "turn_end" }),
    ]),
  );
  const restore = vi.fn();
  const onApply = vi.fn((_query: SearchQuery) => restore);
  render(<Harness onApply={onApply} />);
  const conversation = await screen.findByRole("list", { name: "会話" });
  await within(conversation).findByText("完了");
  // 結果が無いまま応答が終わった呼び出しと、失敗した呼び出しを分けて出す。
  expect(within(conversation).getByText("結果なし")).toBeTruthy();
  expect(within(conversation).getByText("失敗")).toBeTruthy();
  // tool の結果は呼び出しの行にまとめ、単独の行にしない。
  expect(conversation.querySelectorAll(":scope > li")).toHaveLength(4);
  // 入力と結果は開くまで描かない。
  expect(within(conversation).queryByText(/matchedNodeCount/)).toBeNull();

  const details = conversation.querySelectorAll("details");
  const search = details[0] as HTMLDetailsElement;
  search.open = true;
  fireEvent(search, new Event("toggle"));
  const body = await within(search).findByText(/"matchedNodeCount": 3/);
  expect(body).toBeTruthy();
  expect(within(search).getByText(/"depth": 1/)).toBeTruthy();
  // 字下げの改行は行として描き、制御文字の符号にしない。
  expect(search.textContent).not.toContain("U+000A");
  expect(within(search).getByRole("img", { name: /U\+202E/ })).toBeTruthy();

  // 画面で表せる条件を持つ結果だけに、適用と復元の操作を出す。
  expect(
    within(conversation).getAllByRole("button", { name: "検索の条件に適用" }),
  ).toHaveLength(1);
  fireEvent.click(
    within(conversation).getByRole("button", { name: "検索の条件に適用" }),
  );
  // 起点の条件は、起点の種別と表示名と一緒に適用する。
  expect(onApply).toHaveBeenCalledWith(toolQuery, toolOrigins);
  fireEvent.click(
    within(conversation).getByRole("button", { name: "適用前の条件に復元" }),
  );
  expect(restore).toHaveBeenCalledTimes(1);
});

test("自動で適用するモードは、有効にした後に届いた検索の条件だけを届いた時点で適用し、始める前の条件へ戻せる", async () => {
  const held = heldNdjsonResponse();
  const { bodies } = stubRelay({
    ...storedConversation([
      event(1, { kind: "search_query_card", searchQuery: cardQuery }),
    ]),
    turn: () => held.response,
  });
  const restores = [vi.fn(), vi.fn(), vi.fn()];
  let applied = 0;
  const onApply = vi.fn(
    (_query: SearchQuery) => restores[applied++] ?? vi.fn(),
  );
  render(<Harness onApply={onApply} />);
  await screen.findByRole("region", { name: "AI が勧めた検索の条件" });

  const auto = screen.getByLabelText("AI の検索を画面に自動で適用");
  fireEvent.click(auto);
  // 有効にする前に読み直した card は適用しない。
  expect(onApply).not.toHaveBeenCalled();

  fireEvent.change(screen.getByLabelText("AI への発言"), {
    target: { value: "調べて" },
  });
  fireEvent.click(screen.getByRole("button", { name: sendLabel }));
  await waitFor(() => expect(bodies).toHaveLength(1));
  const turnId = turnIdOf(bodies[0]?.body);
  const line = (sequence: number, fields: Record<string, unknown>) => ({
    sequence,
    turnId,
    ...fields,
  });
  const pushed = (sequence: number, fields: Record<string, unknown>) =>
    held.push(line(sequence, fields));
  pushed(2, { kind: "user_message", text: "調べて" });
  pushed(3, { kind: "tool_use", toolName: "graph_search", toolInput: {} });
  pushed(4, {
    kind: "tool_result",
    toolName: "graph_search",
    toolUseSequence: 3,
    toolResult: "{}",
    searchQuery: toolQuery,
    origins: toolOrigins,
  });
  await waitFor(() =>
    expect(onApply).toHaveBeenCalledWith(toolQuery, toolOrigins),
  );

  // 同時に届いた条件は、最新の 1 件だけを適用する。
  const later: SearchQuery = { depth: 3 };
  held.push(
    line(5, { kind: "search_query_card", searchQuery: { depth: 2 } }),
    line(6, { kind: "search_query_card", searchQuery: later }),
  );
  await waitFor(() => expect(onApply).toHaveBeenCalledTimes(2));
  expect(onApply).toHaveBeenLastCalledWith(later, undefined);

  pushed(7, { kind: "turn_end" });
  held.close();
  await waitFor(() => expect(screen.queryByText("AI の応答中")).toBeNull());
  expect(onApply).toHaveBeenCalledTimes(2);

  // 復元は、自動で適用を始める前の条件へ戻す。
  fireEvent.click(
    screen.getByRole("button", { name: "自動で適用を始める前の条件に復元" }),
  );
  expect(restores[0]).toHaveBeenCalledTimes(1);
  expect(restores[1]).not.toHaveBeenCalled();

  fireEvent.click(auto);
  expect((auto as HTMLInputElement).checked).toBe(false);
});

test("自動で適用するモードは、会話の欄を開き直すと無効に戻る", async () => {
  stubRelay({ status: available, conversations: noConversations });
  const first = render(<Harness />);
  fireEvent.click(await screen.findByLabelText("AI の検索を画面に自動で適用"));
  first.unmount();
  render(<Harness />);
  const auto = (await screen.findByLabelText(
    "AI の検索を画面に自動で適用",
  )) as HTMLInputElement;
  expect(auto.checked).toBe(false);
});

test("card を作った時点の関連付けの条件が今の選択と違うときと、端末が一覧に無いときは、その旨を出す", async () => {
  stubRelay({
    status: available,
    conversations: () =>
      jsonResponse(200, {
        conversations: [
          {
            id: conversationId,
            provider: "claude",
            eventCount: 1,
            answering: false,
          },
        ],
      }),
    events: () =>
      jsonResponse(200, {
        answering: false,
        events: [
          event(1, {
            kind: "search_query_card",
            searchQuery: { depth: 1, terminal: "n:terminal:9" },
            matchConditions: [{ conditionKey: "second_of_time", tolerance: 5 }],
          }),
        ],
      }),
  });
  render(<Harness />);
  // 再読み込みした画面は、中継が持つ会話を読み直して card を出し直す。
  expect(await screen.findByText("推定条件の不一致")).toBeTruthy();
  expect(screen.getByText("一覧に無い端末")).toBeTruthy();
  expect(screen.getByText("n:terminal:9")).toBeTruthy();
});

test("応答の途中で中継と通信できなくなったら、接続が切れたことを出し、読み直す操作を出す", async () => {
  stubRelay({
    status: available,
    conversations: () =>
      jsonResponse(200, {
        conversations: [
          {
            id: conversationId,
            provider: "claude",
            eventCount: 0,
            answering: false,
          },
        ],
      }),
    events: () => jsonResponse(200, { answering: false, events: [] }),
    turn: () => {
      throw new TypeError("network");
    },
  });
  render(<Harness />);
  fireEvent.change(await screen.findByLabelText("AI への発言"), {
    target: { value: "続けて" },
  });
  await waitFor(() =>
    expect(screen.getByRole("button", { name: sendLabel })).not.toHaveAttribute(
      "aria-disabled",
    ),
  );
  fireEvent.click(screen.getByRole("button", { name: sendLabel }));
  await waitFor(() => expect(relayState()).toBe("中継: 切断"));
  expect(screen.getByRole("button", { name: "中継を再確認" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "会話を再読み込み" })).toBeTruthy();
  expect(screen.queryByLabelText("AI への発言")).toBeNull();
});

test("送信の許可が無いときは、server の失敗の理由を出す", async () => {
  stubRelay({
    status: available,
    conversations: noConversations,
    open: () =>
      jsonResponse(409, {
        code: "assist_not_permitted",
        message: "the provider is not permitted",
      }),
  });
  render(<Harness />);
  fireEvent.change(await screen.findByLabelText("AI への発言"), {
    target: { value: "調べて" },
  });
  fireEvent.click(screen.getByRole("button", { name: sendLabel }));
  expect(await screen.findByText("AI 支援の会話の開始")).toBeTruthy();
  // 中継には接続できているため、発言の欄を残す。送れなかった発言は欄に残す。
  expect(
    (screen.getByLabelText("AI への発言") as HTMLTextAreaElement).value,
  ).toBe("調べて");
});

test("提供者を止めた会話は、新しい会話を始める操作で続けられる", async () => {
  const newConversationId = "ffeeddccbbaa99887766554433221100";
  let turns = 0;
  const { bodies } = stubRelay({
    status: available,
    conversations: () =>
      jsonResponse(200, {
        conversations: [
          {
            id: conversationId,
            provider: "claude",
            eventCount: 1,
            answering: false,
          },
        ],
      }),
    events: () =>
      jsonResponse(200, {
        answering: false,
        events: [
          event(1, {
            kind: "provider_error",
            text: "応答が上限の時間内に終わらなかったため、提供者を止めました。",
          }),
        ],
      }),
    open: () =>
      jsonResponse(201, {
        conversation: {
          id: newConversationId,
          provider: "claude",
          eventCount: 0,
          answering: false,
        },
      }),
    turn: () => {
      turns += 1;
      return turns === 1
        ? jsonResponse(409, {
            message:
              "assist: the provider of this conversation was stopped; open a new conversation",
          })
        : ndjsonResponse([
            event(1, { kind: "user_message", text: "続けて" }),
            event(2, { kind: "turn_end" }),
          ]);
    },
  });
  render(<Harness />);
  await screen.findByText(/提供者を止めました。/);
  fireEvent.change(screen.getByLabelText("AI への発言"), {
    target: { value: "続けて" },
  });
  fireEvent.click(screen.getByRole("button", { name: sendLabel }));
  await screen.findByText("AI 支援への発言の送信");

  fireEvent.click(screen.getByRole("button", { name: "新規会話を開始" }));
  expect(screen.queryByText(/提供者を止めました。/)).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: sendLabel }));
  await within(await screen.findByRole("list", { name: "会話" })).findByText(
    "続けて",
  );
  expect(bodies.map((item) => item.path)).toEqual([
    `/assist/conversations/${conversationId}/turns`,
    "/assist/conversations",
    `/assist/conversations/${newConversationId}/turns`,
  ]);
});
