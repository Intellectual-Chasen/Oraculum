// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { SignedInContext } from "@/shared/ui/SignedInAccount";
import { jsonResponse } from "@/testdata/http";
import { signedInAlice } from "@/testdata/session";
import { AssistPermissions } from "./AssistPermissions";
import { useAssistPermissions } from "./useAssistPermissions";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const permissionsPath = "/api/v0/assist-permissions";

function grantRevision(revisionNumber: number) {
  return {
    provider: "claude",
    revisionNumber,
    action: "grant",
    analyst: "analyst-a",
    recordedAt: "2030-01-02T03:04:05.000Z",
  };
}

function Harness() {
  return <AssistPermissions view={useAssistPermissions()} />;
}

/** 一覧と記録の要求に応える fetch の mock。記録の要求の本文を集める。 */
function stubFetch(list: Response, recorded?: () => Response) {
  const bodies: unknown[] = [];
  const mock = vi.fn(async (input: string, init?: RequestInit) => {
    if (input !== permissionsPath) {
      throw new Error(`unexpected request: ${input}`);
    }
    if (init?.method === "POST") {
      bodies.push(JSON.parse(String(init.body)));
      if (recorded === undefined) {
        throw new Error("unexpected record request");
      }
      return recorded();
    }
    return list;
  });
  vi.stubGlobal("fetch", mock);
  return { mock, bodies };
}

const grantLabel = "送信を許可";
const revokeLabel = "送信の許可を取消";

/** 操作の IconButton を押し、確認の dialog の同じ名前の button を押す。 */
async function confirm(name: string) {
  fireEvent.click(await screen.findByRole("button", { name }));
  const dialog = await screen.findByRole("alertdialog");
  fireEvent.click(within(dialog).getByRole("button", { name }));
}

/** 提供者の表の Claude の行。 */
async function providerRow(): Promise<HTMLElement> {
  const table = await screen.findByRole("table", { name: "提供者" });
  const row = within(table).getAllByRole("row")[1];
  if (row === undefined) throw new Error("提供者の行が無い");
  return row;
}

test("許可の無い提供者に許可を記録すると、現在の状態と改訂の一覧を置き換える", async () => {
  const { bodies } = stubFetch(
    jsonResponse(200, {
      providers: [{ provider: "claude", permitted: false, revisions: [] }],
    }),
    () =>
      jsonResponse(201, {
        provider: "claude",
        permitted: true,
        revisions: [grantRevision(1)],
      }),
  );
  render(<Harness />);

  expect((await providerRow()).textContent).toContain("未許可");
  const revisions = screen.getByRole("region", { name: "Claude の改訂" });
  expect(revisions.textContent).toBe("Claude の改訂: 0");

  fireEvent.change(screen.getByLabelText("分析者の名前"), {
    target: { value: "analyst-a" },
  });
  await confirm(grantLabel);

  expect(await screen.findByRole("button", { name: revokeLabel })).toBeTruthy();
  expect((await providerRow()).textContent).toContain("許可中");
  const rows = within(
    screen.getByRole("table", { name: "Claude の改訂" }),
  ).getAllByRole("row");
  expect(rows[1]?.textContent).toContain("許可");
  expect(rows[1]?.textContent).toContain("analyst-a");
  expect(bodies).toEqual([
    { provider: "claude", action: "grant", analyst: "analyst-a" },
  ]);
});

test("確認の dialog で取消を押すと、許可を記録しない", async () => {
  const { mock } = stubFetch(
    jsonResponse(200, {
      providers: [{ provider: "claude", permitted: false, revisions: [] }],
    }),
  );
  render(<Harness />);
  fireEvent.click(await screen.findByRole("button", { name: grantLabel }));
  const dialog = await screen.findByRole("alertdialog");
  expect(dialog.textContent).toContain("提供者: Claude");
  fireEvent.click(within(dialog).getByRole("button", { name: "取消" }));

  expect(screen.queryByRole("alertdialog")).toBeNull();
  expect(mock.mock.calls.every(([, init]) => init?.method !== "POST")).toBe(
    true,
  );
});

test("分析者の名前が空のときは記録を送らずに理由を出す", async () => {
  const { mock } = stubFetch(
    jsonResponse(200, {
      providers: [{ provider: "claude", permitted: false, revisions: [] }],
    }),
  );
  render(<Harness />);
  await confirm(grantLabel);
  expect((await screen.findByRole("alert")).textContent).toContain(
    "分析者の名前なし",
  );
  expect(mock.mock.calls.every(([, init]) => init?.method !== "POST")).toBe(
    true,
  );
});

test("ログインしている分析者は名前を入力せずに送信を許可する", async () => {
  const { bodies } = stubFetch(
    jsonResponse(200, {
      providers: [{ provider: "claude", permitted: false, revisions: [] }],
    }),
    () =>
      jsonResponse(201, {
        provider: "claude",
        permitted: true,
        revisions: [{ ...grantRevision(1), analyst: "alice" }],
      }),
  );
  render(
    <SignedInContext.Provider value={signedInAlice}>
      <Harness />
    </SignedInContext.Provider>,
  );
  await screen.findByRole("button", { name: grantLabel });
  expect(screen.queryByLabelText("分析者の名前")).toBeNull();
  await confirm(grantLabel);
  expect(await screen.findByRole("button", { name: revokeLabel })).toBeTruthy();
  expect(bodies).toEqual([{ provider: "claude", action: "grant" }]);
});

test("調査の directory を渡さない起動では、使用不可の状態と起動の指定を出す", async () => {
  stubFetch(
    jsonResponse(409, {
      code: "assist_unavailable",
      message: "the launch keeps no investigation",
    }),
  );
  render(<Harness />);
  const state = await screen.findByText("使用不可");
  expect(state.closest("li")?.textContent).toMatch(/^状態: 使用不可/);
  expect(document.body.textContent).toContain("起動の指定: --investigation");
  // 見出しの help のほかに操作を出さない。
  expect(
    screen
      .queryAllByRole("button")
      .map((button) => button.getAttribute("aria-label")),
  ).toEqual(["AI 支援の送信の許可 の説明"]);
});

test("記録が失敗しても、読めた状態を残して失敗を出す", async () => {
  stubFetch(
    jsonResponse(200, {
      providers: [
        { provider: "claude", permitted: true, revisions: [grantRevision(1)] },
      ],
    }),
    () => jsonResponse(500, { code: "internal_error", message: "failed" }),
  );
  render(<Harness />);
  fireEvent.change(await screen.findByLabelText("分析者の名前"), {
    target: { value: "analyst-b" },
  });
  await confirm(revokeLabel);
  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain("AI 支援の送信の許可の記録");
  expect(alert.textContent).toContain("コード: internal_error");
  expect((await providerRow()).textContent).toContain("許可中");
});
