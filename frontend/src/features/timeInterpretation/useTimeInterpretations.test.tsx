// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import { jsonResponse } from "@/testdata/http";
import { apiErrorJson } from "@/testdata/sources/sourcesResponse";
import { useTimeInterpretations } from "./useTimeInterpretations";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const matchConditions: MatchConditionSelection = {
  conditions: [{ conditionKey: "destination_ip" }],
};

const localContent = "e".repeat(64);
const otherContent = "f".repeat(64);

/** 収集元を指す所見 1 件の JSON。 */
function sourceAssertionJson(
  id: string,
  content: string,
  state: "active" | "withdrawn",
  timeOffset: string,
) {
  return {
    assertion: {
      id,
      target: { kind: "source", sourceContentSha256: content },
      state,
      author: "analyst-a",
      recordedAt: "2031-01-02T03:04:05.000Z",
      basis: { note: "合成の根拠", recordRefs: [] },
      timeOffset,
      revisionNumber: 1,
      history: [],
    },
    targetOrigin: "observation",
  };
}

function listJson(items: unknown[]) {
  return { assertions: items, assertionCount: items.length };
}

const probeRecordRef = {
  sourceContentSha256: otherContent,
  positionKind: "line_number" as const,
  lineNumber: 7,
};

/** hook の結果を 1 行の文字列で描き、記録の操作を出す。 */
function Probe() {
  const view = useTimeInterpretations(matchConditions);
  const describe = (content: string) => {
    if (view.state.status !== "loaded") {
      return view.state.status;
    }
    const found = view.state.value.get(content);
    if (found === undefined) {
      return "none";
    }
    return found.kind === "conflicted"
      ? `conflicted:${found.assertions.length}`
      : `${found.assertion.state}:${found.assertion.timeOffset}`;
  };
  return (
    <>
      <p>
        local={describe(localContent)} other={describe(otherContent)} recorded=
        {view.recordedCount}
      </p>
      <p>
        failure=
        {view.recordFailure === undefined
          ? "none"
          : `${view.recordFailure.sourceContentSha256}:${view.recordFailure.failure.summary}`}
      </p>
      <p>next={view.recordFailure?.failure.nextAction ?? "none"}</p>
      <p>
        {view.state.status === "failed" ? view.state.failure.summary : "listed"}
      </p>
      <button
        type="button"
        onClick={() =>
          view.create({
            target: { kind: "source", sourceContentSha256: localContent },
            author: "analyst-a",
            note: "合成の根拠",
            recordRefs: [probeRecordRef],
            timeOffset: "+09:00",
          })
        }
      >
        記録する
      </button>
      <p>
        conflict=
        {view.conflict === undefined
          ? "none"
          : `${view.conflict.theirs.author}:${view.conflict.theirs.revisionNumber}:${view.conflict.mine.timeOffset}`}
      </p>
      <button
        type="button"
        onClick={() => {
          const conflict = view.conflict;
          if (conflict !== undefined) {
            view.revise(conflict.theirs.id, {
              ...conflict.mine,
              baseRevision: conflict.theirs.revisionNumber,
            });
          }
        }}
      >
        自分の値で改訂する
      </button>
    </>
  );
}

test("記録が 409 assertion_changed で退けられると、相手の値と自分の値を持ち、自分の値の改訂は相手の revision を元に送る", async () => {
  const theirs = sourceAssertionJson("as:7", localContent, "active", "+00:00");
  theirs.assertion.author = "analyst-b";
  theirs.assertion.revisionNumber = 4;
  const sent: { method?: string; body?: unknown }[] = [];
  const mock = vi.fn(async (_input: string, init?: RequestInit) => {
    if (init?.method === "POST") {
      return jsonResponse(409, {
        code: "assertion_changed",
        message: "the assertion changed after the base revision",
        conflict: theirs,
      });
    }
    if (init?.method === "PUT") {
      sent.push({ method: "PUT", body: JSON.parse(String(init.body)) });
      return jsonResponse(
        200,
        sourceAssertionJson("as:7", localContent, "active", "+09:00"),
      );
    }
    return jsonResponse(200, listJson([]));
  });
  vi.stubGlobal("fetch", mock);

  render(<Probe />);
  await waitFor(() => expect(screen.getByText(/local=none/)).toBeTruthy());
  fireEvent.click(screen.getByRole("button", { name: "記録する" }));

  await waitFor(() =>
    expect(screen.getByText("conflict=analyst-b:4:+09:00")).toBeTruthy(),
  );
  expect(screen.getByText("failure=none")).toBeTruthy();
  expect(screen.getByText(/local=active:\+00:00/)).toBeTruthy();

  fireEvent.click(screen.getByRole("button", { name: "自分の値で改訂する" }));
  await waitFor(() => expect(screen.getByText(/recorded=1/)).toBeTruthy());
  expect(sent).toHaveLength(1);
  expect(sent[0]?.body).toEqual({
    target: { kind: "source", sourceContentSha256: localContent },
    author: "analyst-a",
    basis: { note: "合成の根拠", recordRefs: [probeRecordRef] },
    timeOffset: "+09:00",
    state: "active",
    baseRevision: 4,
  });
  expect(screen.getByText("conflict=none")).toBeTruthy();
});

test("409 assertion_changed の conflict を読めないときは、一覧を取り直して記録し直すことを案内する", async () => {
  let listCalls = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        return jsonResponse(409, {
          code: "assertion_changed",
          message: "the assertion changed after the base revision",
          conflict: { assertion: "unreadable" },
        });
      }
      listCalls += 1;
      return jsonResponse(200, listJson([]));
    }),
  );

  render(<Probe />);
  await waitFor(() => expect(screen.getByText(/local=none/)).toBeTruthy());
  fireEvent.click(screen.getByRole("button", { name: "記録する" }));

  await waitFor(() =>
    expect(
      screen.getByText("next=再読み込みした最新の値を確認して再度記録"),
    ).toBeTruthy(),
  );
  expect(screen.getByText("conflict=none")).toBeTruthy();
  await waitFor(() => expect(listCalls).toBe(2));
});

test("記録の操作を続けて 2 回行っても、要求は 1 回だけ送る", async () => {
  let posts = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        posts += 1;
        return jsonResponse(
          201,
          sourceAssertionJson("as:1", localContent, "active", "+09:00"),
        );
      }
      return jsonResponse(200, listJson([]));
    }),
  );

  render(<Probe />);
  await waitFor(() => expect(screen.getByText(/local=none/)).toBeTruthy());
  const button = screen.getByRole("button", { name: "記録する" });
  fireEvent.click(button);
  fireEvent.click(button);

  await waitFor(() => expect(screen.getByText(/recorded=1/)).toBeTruthy());
  expect(posts).toBe(1);
});

test("主張中の解釈を優先し、同じ収集元に主張中の解釈が 2 件あれば競合として返す", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      jsonResponse(
        200,
        listJson([
          sourceAssertionJson("as:1", localContent, "active", "+09:00"),
          sourceAssertionJson("as:2", localContent, "active", "+00:00"),
          sourceAssertionJson("as:3", otherContent, "active", "+01:00"),
          sourceAssertionJson("as:4", otherContent, "withdrawn", "+02:00"),
        ]),
      ),
    ),
  );

  render(<Probe />);

  await waitFor(() =>
    expect(
      screen.getByText(/local=conflicted:2 other=active:\+01:00/),
    ).toBeTruthy(),
  );
});

test("記録が成功すると、記録した回数が 1 増え、応答の所見が一覧に入る", async () => {
  const mock = vi.fn(async (_input: string, init?: RequestInit) =>
    init?.method === "POST"
      ? jsonResponse(
          201,
          sourceAssertionJson("as:1", localContent, "active", "+09:00"),
        )
      : jsonResponse(200, listJson([])),
  );
  vi.stubGlobal("fetch", mock);

  render(<Probe />);
  await waitFor(() => expect(screen.getByText(/local=none/)).toBeTruthy());

  fireEvent.click(screen.getByRole("button", { name: "記録する" }));

  await waitFor(() =>
    expect(
      screen.getByText(/local=active:\+09:00 other=none recorded=1/),
    ).toBeTruthy(),
  );
  expect(screen.getByText("failure=none")).toBeTruthy();
});

test("記録が 400 で退けられると、読めた一覧を残して失敗を出し、一覧を取り直す", async () => {
  let listCalls = 0;
  const mock = vi.fn(async (_input: string, init?: RequestInit) => {
    if (init?.method === "POST") {
      return jsonResponse(400, apiErrorJson("invalid_request"));
    }
    listCalls += 1;
    // 取り直した一覧は、別の分析者が先に記録した解釈を含む。
    return jsonResponse(
      200,
      listJson(
        listCalls === 1
          ? []
          : [sourceAssertionJson("as:9", localContent, "active", "+00:00")],
      ),
    );
  });
  vi.stubGlobal("fetch", mock);

  render(<Probe />);
  await waitFor(() => expect(screen.getByText(/local=none/)).toBeTruthy());

  fireEvent.click(screen.getByRole("button", { name: "記録する" }));

  await waitFor(() =>
    expect(
      screen.getByText(/local=active:\+00:00 other=none recorded=0/),
    ).toBeTruthy(),
  );
  expect(listCalls).toBe(2);
  expect(screen.getByText(`failure=${localContent}:メモの記録`)).toBeTruthy();
  expect(screen.getByText("listed")).toBeTruthy();
});

test("一覧を取得できないとき、時刻の解釈を取得できなかったことを出す", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse(500, apiErrorJson("internal_error"))),
  );

  render(<Probe />);

  await waitFor(() =>
    expect(screen.getByText("タイムゾーンの取得")).toBeTruthy(),
  );
});
