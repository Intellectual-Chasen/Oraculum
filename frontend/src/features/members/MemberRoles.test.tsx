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
import type { MemberRole } from "@/shared/contracts/members";
import { SignedInContext } from "@/shared/ui/SignedInAccount";
import { jsonResponse } from "@/testdata/http";
import { memberJson, signedInAlice } from "@/testdata/session";
import { MemberRoles } from "./MemberRoles";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type Route = (init: RequestInit | undefined) => Response | Promise<Response>;

/**
 * 役割を持つ利用者を server の代わりに保ち、一覧・変更・外す要求に答える。
 * `failures` に method と path の組を渡すと、その要求に渡した応答を返す。
 */
function stubMembers(failures: Record<string, Route> = {}) {
  const members = new Map<string, MemberRole>([
    ["alice", "admin"],
    ["bob", "viewer"],
  ]);
  const mock = vi.fn(async (input: string, init?: RequestInit) => {
    const method = init?.method ?? "GET";
    const failure = failures[`${method} ${input}`];
    if (failure !== undefined) {
      return failure(init);
    }
    if (method === "GET" && input === "/api/v0/members") {
      return jsonResponse(200, {
        members: [...members].map(([login, role]) => memberJson(role, login)),
      });
    }
    const login = decodeURIComponent(input.replace("/api/v0/members/", ""));
    if (method === "PUT") {
      const { role } = JSON.parse(String(init?.body));
      members.set(login, role);
      return jsonResponse(200, memberJson(role, login));
    }
    if (method === "DELETE") {
      members.delete(login);
      return new Response(null, { status: 204 });
    }
    throw new Error(`unexpected request: ${method} ${input}`);
  });
  vi.stubGlobal("fetch", mock);
  return mock;
}

function rowOf(login: string): HTMLElement {
  const row = screen.getByLabelText(`${login} の役割`).closest("tr");
  if (row === null) {
    throw new Error(`row for ${login} was not found`);
  }
  return row;
}

/** login の行の役割を削除する button。 */
async function revokeButton(login: string): Promise<HTMLElement> {
  await screen.findByLabelText(`${login} の役割`);
  return within(rowOf(login)).getByRole("button", {
    name: `${login} の役割を削除`,
  });
}

test("役割を持つ利用者の一覧に、ログイン名・表示名・役割・与えた利用者・時刻を出す", async () => {
  stubMembers();
  render(<MemberRoles />);

  await screen.findByRole("table", { name: "利用者" });
  const row = rowOf("bob");
  expect(row.textContent).toContain("石橋");
  expect(row.textContent).toContain("root");
  expect(row.textContent).toContain("2026-09-01T00:00:00Z");
  expect(within(row).getByLabelText("bob の役割")).toHaveValue("viewer");
});

test("役割を選び直すと役割を記録し、一覧を取り直す", async () => {
  const mock = stubMembers();
  render(<MemberRoles />);

  fireEvent.change(await screen.findByLabelText("bob の役割"), {
    target: { value: "editor" },
  });

  await waitFor(() =>
    expect(screen.getByLabelText("bob の役割")).toHaveValue("editor"),
  );
  const put = mock.mock.calls.find(([, init]) => init?.method === "PUT");
  expect(put?.[0]).toBe("/api/v0/members/bob");
  expect(JSON.parse(String(put?.[1]?.body))).toEqual({ role: "editor" });
});

test("削除の button で役割を削除し、一覧から消す", async () => {
  stubMembers();
  render(<MemberRoles />);

  fireEvent.click(await revokeButton("bob"));

  await waitFor(() => expect(screen.queryByLabelText("bob の役割")).toBeNull());
});

test("ログイン名と役割を入力して追加すると、一覧に足して入力欄を空へ戻す", async () => {
  stubMembers();
  render(<MemberRoles />);
  await screen.findByLabelText("bob の役割");

  fireEvent.change(screen.getByLabelText("ログイン名"), {
    target: { value: "carol" },
  });
  fireEvent.change(screen.getByLabelText("役割"), {
    target: { value: "editor" },
  });
  fireEvent.click(screen.getByRole("button", { name: "利用者を追加" }));

  expect(await screen.findByLabelText("carol の役割")).toHaveValue("editor");
  expect(screen.getByLabelText("ログイン名")).toHaveValue("");
});

test("最後の管理者を削除する要求が拒否されたら、失敗を出して一覧を保つ", async () => {
  stubMembers({
    "DELETE /api/v0/members/alice": () =>
      jsonResponse(400, { code: "invalid_request", message: "x" }),
  });
  render(<MemberRoles />);

  fireEvent.click(await revokeButton("alice"));

  expect(await screen.findByText("役割の削除")).toBeTruthy();
  expect(screen.getByRole("alert").textContent).toContain(
    "コード: invalid_request",
  );
  expect(screen.getByLabelText("alice の役割")).toHaveValue("admin");
});

test("アカウントの無いログイン名を追加する要求が退けられたら、失敗を出して入力を残す", async () => {
  stubMembers({
    "PUT /api/v0/members/nobody": () =>
      jsonResponse(400, { code: "invalid_request", message: "x" }),
  });
  render(<MemberRoles />);
  await screen.findByLabelText("bob の役割");

  fireEvent.change(screen.getByLabelText("ログイン名"), {
    target: { value: "nobody" },
  });
  fireEvent.click(screen.getByRole("button", { name: "利用者を追加" }));

  expect(await screen.findByText("役割の記録")).toBeTruthy();
  expect(screen.getByLabelText("ログイン名")).toHaveValue("nobody");
});

test("ログインした利用者の行は、役割の変更と削除の操作を止めて理由を結び付ける", async () => {
  stubMembers();
  render(
    <SignedInContext.Provider value={{ ...signedInAlice, role: "admin" }}>
      <MemberRoles />
    </SignedInContext.Provider>,
  );

  const own = await screen.findByLabelText("alice の役割");
  const revoke = await revokeButton("alice");
  expect(own).toBeDisabled();
  expect(revoke).toHaveAttribute("aria-disabled", "true");
  const describedText = (element: HTMLElement) =>
    (element.getAttribute("aria-describedby") ?? "")
      .split(" ")
      .map((id) => document.getElementById(id)?.textContent ?? "")
      .join(" ");
  expect(describedText(own)).toContain("ほかの管理者が変更");
  expect(describedText(revoke)).toContain("ほかの管理者が変更");
  expect(screen.getByLabelText("bob の役割")).toBeEnabled();
});

test("役割の変更が退けられたら、失敗を出して選択を元の役割に戻す", async () => {
  stubMembers({
    "PUT /api/v0/members/bob": () =>
      jsonResponse(500, { code: "internal_error", message: "x" }),
  });
  render(<MemberRoles />);

  fireEvent.change(await screen.findByLabelText("bob の役割"), {
    target: { value: "admin" },
  });

  expect(await screen.findByText("役割の記録")).toBeTruthy();
  expect(screen.getByLabelText("bob の役割")).toHaveValue("viewer");
});

test("要求の応答を待つ間は、記録していることを出す", async () => {
  stubMembers({
    "DELETE /api/v0/members/bob": () => new Promise<Response>(() => {}),
  });
  render(<MemberRoles />);

  fireEvent.click(await revokeButton("bob"));

  expect(await screen.findByText("記録中")).toBeTruthy();
});

test("一覧を読めないときは失敗を出す", async () => {
  stubMembers({
    "GET /api/v0/members": () =>
      jsonResponse(403, { code: "permission_denied", message: "x" }),
  });
  render(<MemberRoles />);

  expect(await screen.findByText("利用者と役割の一覧の取得")).toBeTruthy();
  expect(screen.getByRole("alert").textContent).toContain(
    "次の操作: 管理者に役割の変更を依頼",
  );
});
