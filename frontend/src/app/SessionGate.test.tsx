// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { fetchStages } from "@/shared/api/stages";
import { useCanWrite, useSignedIn } from "@/shared/ui/SignedInAccount";
import { jsonResponse } from "@/testdata/http";
import {
  accountLabel,
  accountSessionJson,
  memberJson,
} from "@/testdata/session";
import { AccountBar } from "./AccountBar";
import { SessionGate } from "./SessionGate";

// 登録を外す関数の呼び出しを数える。登録そのものは本来の実装へ渡す。
const unsubscribed = vi.hoisted(() => ({ count: 0 }));
vi.mock("@/shared/api/httpClient", async (importOriginal) => {
  const original =
    await importOriginal<typeof import("@/shared/api/httpClient")>();
  return {
    ...original,
    onAuthenticationRequired: (
      listener: Parameters<typeof original.onAuthenticationRequired>[0],
    ) => {
      const unsubscribe = original.onAuthenticationRequired(listener);
      return () => {
        unsubscribed.count += 1;
        unsubscribe();
      };
    },
  };
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const authenticationRequired = () =>
  jsonResponse(401, { code: "authentication_required", message: "x" });

const authenticationRequiredLabel = "ログインが必要";

/** 子の要素に分かれた文字列を、要素の textContent の全体で探す。 */
const textIs = (text: string) => (_: string, element: Element | null) =>
  element?.textContent === text &&
  !Array.from(element.children).some((child) => child.textContent === text);

/** ヘッダーの利用者の表示。ログイン名は title に入る。 */
const accountHeader = textIs(`ログイン中: ${accountLabel}`);

type Route = (init: RequestInit | undefined) => Response | Promise<Response>;

/** 呼ばれるたびに次の応答を返す。並びの最後の応答は、それ以降も返す。 */
function inTurn(...responses: (() => Response)[]): Route {
  let served = 0;
  return () => {
    const response = responses[Math.min(served, responses.length - 1)];
    served += 1;
    if (response === undefined) {
      throw new Error("no response");
    }
    return response();
  };
}

/**
 * path と method の組ごとに応答を返す。組に無い要求は test を失敗させる。
 * `GET /api/v0/members/me` を渡さないときは、編集者の役割を返す。
 */
function stubFetch(routes: Record<string, Route>) {
  const withMember: Record<string, Route> = {
    "GET /api/v0/members/me": () => jsonResponse(200, memberJson("editor")),
    ...routes,
  };
  const mock = vi.fn(async (input: string, init?: RequestInit) => {
    const route = withMember[`${init?.method ?? "GET"} ${input}`];
    if (route === undefined) {
      throw new Error(`unexpected request: ${init?.method} ${input}`);
    }
    return route(init);
  });
  vi.stubGlobal("fetch", mock);
  return mock;
}

/** 本文のログイン名で、そのログイン名の利用者の応答を返す。 */
const loginAsRequested: Route = (init) => {
  const { login } = JSON.parse(String(init?.body));
  return jsonResponse(200, {
    authentication: "account",
    login,
    displayName: login === "alice" ? "石橋" : "別の利用者",
  });
};

/**
 * 調査の画面の代わりに、ヘッダー、役割、記録のボタン、入力欄、段階の状態を取り直すボタンを
 * 出す。
 */
function Investigation() {
  return (
    <>
      <AccountBar />
      <p>調査の画面</p>
      <p>役割: {useSignedIn()?.role ?? "なし"}</p>
      <button type="button" disabled={!useCanWrite()}>
        記録する
      </button>
      <label>
        書きかけのメモ
        <input />
      </label>
      <button type="button" onClick={() => void fetchStages()}>
        段階の状態を取り直す
      </button>
    </>
  );
}

function renderGate() {
  return render(
    <SessionGate>
      <Investigation />
    </SessionGate>,
  );
}

async function submitLogin(login = "alice") {
  fireEvent.change(await screen.findByLabelText("ログイン名"), {
    target: { value: login },
  });
  fireEvent.change(screen.getByLabelText("パスワード"), {
    target: { value: "secret-1" },
  });
  fireEvent.click(screen.getByRole("button", { name: "ログイン" }));
}

/** ログインした状態で段階の状態を取り直し、authentication_required を受け取る。 */
async function expireSessionWhileTyping() {
  fireEvent.change(await screen.findByLabelText("書きかけのメモ"), {
    target: { value: "port 5985 を読んだ" },
  });
  fireEvent.click(screen.getByRole("button", { name: "段階の状態を取り直す" }));
  await screen.findByRole("dialog", { name: "再ログイン" });
}

function memoValue(): string {
  return (screen.getByLabelText("書きかけのメモ") as HTMLInputElement).value;
}

test("アカウントを持たない起動では、ログインの画面とログインした利用者を出さない", async () => {
  stubFetch({
    "GET /api/v0/session/me": () =>
      jsonResponse(200, { authentication: "none" }),
  });

  renderGate();

  expect(await screen.findByText("調査の画面")).toBeTruthy();
  expect(screen.queryByLabelText("パスワード")).toBeNull();
  expect(screen.queryByRole("button", { name: "ログアウト" })).toBeNull();
});

test("ログインの状態を読めないときは、失敗を出して調査の画面とログインの画面を出さない", async () => {
  stubFetch({
    "GET /api/v0/session/me": () =>
      jsonResponse(500, { code: "internal_error", message: "x" }),
  });

  renderGate();

  expect(await screen.findByText("ログインの状態の取得")).toBeTruthy();
  expect(screen.getByText(textIs("コード: internal_error"))).toBeTruthy();
  expect(screen.queryByText("調査の画面")).toBeNull();
  expect(screen.queryByLabelText("パスワード")).toBeNull();
});

test("ログインしていないときはログインの画面を出し、ログインに成功したら調査の画面へ進む", async () => {
  const mock = stubFetch({
    "GET /api/v0/session/me": authenticationRequired,
    "POST /api/v0/session": () => jsonResponse(200, accountSessionJson()),
  });

  renderGate();
  expect(screen.queryByText("調査の画面")).toBeNull();
  await submitLogin();

  expect(await screen.findByText("調査の画面")).toBeTruthy();
  expect(screen.getByText(accountHeader)).toBeTruthy();
  expect(screen.getByTitle("ログイン名: alice")).toBeTruthy();
  const login = mock.mock.calls.find(([, init]) => init?.method === "POST");
  expect(login?.[0]).toBe("/api/v0/session");
  expect(JSON.parse(String(login?.[1]?.body))).toEqual({
    login: "alice",
    password: "secret-1",
  });
});

test("ログイン名とパスワードの欄は入力を必須にする", async () => {
  stubFetch({ "GET /api/v0/session/me": authenticationRequired });

  renderGate();

  expect(
    (await screen.findByLabelText("ログイン名")).hasAttribute("required"),
  ).toBe(true);
  expect(screen.getByLabelText("パスワード").hasAttribute("required")).toBe(
    true,
  );
});

test.each([
  [401, "login_rejected", "ログイン名かパスワードの誤り"],
  [429, "login_rate_limited", "ログインの一時停止"],
])(
  "ログインが status %i の %s で失敗したら、理由を出し、パスワードを消してログインの画面に留まる",
  async (status, code, description) => {
    stubFetch({
      "GET /api/v0/session/me": authenticationRequired,
      "POST /api/v0/session": () =>
        jsonResponse(status, { code, message: "x" }),
    });

    renderGate();
    await submitLogin();

    expect(await screen.findByText(description)).toBeTruthy();
    expect(screen.getByText(textIs(`コード: ${code}`))).toBeTruthy();
    expect(
      (screen.getByLabelText("パスワード") as HTMLInputElement).value,
    ).toBe("");
    expect(
      (screen.getByLabelText("ログイン名") as HTMLInputElement).value,
    ).toBe("alice");
    expect(screen.queryByText("調査の画面")).toBeNull();
  },
);

test("ログインしているときはヘッダーに利用者を出し、ログアウトしたらログインの画面に戻す", async () => {
  const mock = stubFetch({
    "GET /api/v0/session/me": () => jsonResponse(200, accountSessionJson()),
    "DELETE /api/v0/session": () => new Response(null, { status: 204 }),
  });

  renderGate();
  expect(await screen.findByText(accountHeader)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "ログアウト" }));

  expect(await screen.findByLabelText("パスワード")).toBeTruthy();
  expect(screen.queryByText("調査の画面")).toBeNull();
  expect(
    mock.mock.calls.filter(
      ([input, init]) =>
        input === "/api/v0/session" && init?.method === "DELETE",
    ),
  ).toHaveLength(1);
});

test("ログアウトが失敗したら、失敗を出してログインしている状態を保つ", async () => {
  stubFetch({
    "GET /api/v0/session/me": () => jsonResponse(200, accountSessionJson()),
    "DELETE /api/v0/session": () =>
      jsonResponse(500, { code: "internal_error", message: "x" }),
  });

  renderGate();
  fireEvent.click(await screen.findByRole("button", { name: "ログアウト" }));

  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain("ログアウト");
  expect(alert.textContent).toContain("コード: internal_error");
  expect(screen.getByText(accountHeader)).toBeTruthy();
  expect(screen.getByText("調査の画面")).toBeTruthy();
  expect(screen.queryByLabelText("パスワード")).toBeNull();
});

test("ログインの後の要求が authentication_required を受け取ったら、理由とログインの form を調査の画面に重ね、同じ利用者でログインし直すと入力を残す", async () => {
  stubFetch({
    "GET /api/v0/session/me": () => jsonResponse(200, accountSessionJson()),
    "GET /api/v0/stages": authenticationRequired,
    "POST /api/v0/session": loginAsRequested,
  });

  renderGate();
  await expireSessionWhileTyping();

  expect(
    screen.getAllByText(authenticationRequiredLabel).length,
  ).toBeGreaterThan(0);
  expect(screen.getByText("段階の状態の取得")).toBeTruthy();
  const content = screen.getByText("調査の画面").closest(".session-content");
  expect(content?.hasAttribute("inert")).toBe(true);
  expect(memoValue()).toBe("port 5985 を読んだ");
  await waitFor(() =>
    expect(document.activeElement).toBe(screen.getByLabelText("ログイン名")),
  );

  await submitLogin("alice");

  await waitFor(() =>
    expect(screen.queryByRole("dialog", { name: "再ログイン" })).toBeNull(),
  );
  expect(content?.hasAttribute("inert")).toBe(false);
  expect(memoValue()).toBe("port 5985 を読んだ");
});

test("別の利用者でログインし直すと、前の利用者の入力を残さずに調査の画面を作り直す", async () => {
  stubFetch({
    "GET /api/v0/session/me": () => jsonResponse(200, accountSessionJson()),
    "GET /api/v0/stages": authenticationRequired,
    "POST /api/v0/session": loginAsRequested,
  });

  renderGate();
  await expireSessionWhileTyping();
  await submitLogin("bob");

  expect(
    await screen.findByText(textIs("ログイン中: 別の利用者")),
  ).toBeTruthy();
  expect(screen.queryByRole("dialog", { name: "再ログイン" })).toBeNull();
  expect(memoValue()).toBe("");
});

test("unmount すると、authentication_required の通知の登録を外す", async () => {
  stubFetch({
    "GET /api/v0/session/me": () =>
      jsonResponse(200, { authentication: "none" }),
  });
  const before = unsubscribed.count;

  const { unmount } = renderGate();
  await screen.findByText("調査の画面");
  unmount();

  expect(unsubscribed.count).toBe(before + 1);
});

test("ログインした後に調査の役割を読み、調査の画面へ役割を渡す", async () => {
  stubFetch({
    "GET /api/v0/session/me": () => jsonResponse(200, accountSessionJson()),
    "GET /api/v0/members/me": () => jsonResponse(200, memberJson("viewer")),
  });

  renderGate();

  expect(await screen.findByText("役割: viewer")).toBeTruthy();
});

test("アカウントを持たない起動では、役割を読まずに調査の画面を出す", async () => {
  const mock = stubFetch({
    "GET /api/v0/session/me": () =>
      jsonResponse(200, { authentication: "none" }),
  });

  renderGate();

  expect(await screen.findByText("役割: なし")).toBeTruthy();
  expect(
    mock.mock.calls.some(([input]) => input === "/api/v0/members/me"),
  ).toBe(false);
});

/** 役割を持たないことと次の操作を、値の組で出しているか。 */
async function expectNoRole() {
  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain("役割: なし");
  expect(alert.textContent).toContain("次の操作: 管理者に役割の付与を依頼");
}

test("調査の役割を持たない利用者には、調査の画面を出さずに次の操作とログアウトのボタンを出す", async () => {
  stubFetch({
    "GET /api/v0/session/me": () => jsonResponse(200, accountSessionJson()),
    "GET /api/v0/members/me": () =>
      jsonResponse(404, { code: "investigation_not_found", message: "x" }),
    "DELETE /api/v0/session": () => new Response(null, { status: 204 }),
  });

  renderGate();

  await expectNoRole();
  expect(screen.getByText(accountHeader)).toBeTruthy();
  expect(screen.queryByText("調査の画面")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "ログアウト" }));
  expect(await screen.findByLabelText("パスワード")).toBeTruthy();
});

test("調査の役割を読めないときは、失敗を出して調査の画面を出さない", async () => {
  stubFetch({
    "GET /api/v0/session/me": () => jsonResponse(200, accountSessionJson()),
    "GET /api/v0/members/me": () =>
      jsonResponse(500, { code: "internal_error", message: "x" }),
  });

  renderGate();

  expect(await screen.findByText("調査の役割の取得")).toBeTruthy();
  expect(screen.queryByText("調査の画面")).toBeNull();
  expect(screen.getByRole("button", { name: "ログアウト" })).toBeTruthy();
});

const internalError = () =>
  jsonResponse(500, { code: "internal_error", message: "x" });
const investigationNotFound = () =>
  jsonResponse(404, { code: "investigation_not_found", message: "x" });

test("役割を読めなかった後に「役割を再確認」を押すと、役割を読み直して調査の画面を出す", async () => {
  stubFetch({
    "GET /api/v0/session/me": () => jsonResponse(200, accountSessionJson()),
    "GET /api/v0/members/me": inTurn(internalError, () =>
      jsonResponse(200, memberJson("editor")),
    ),
  });

  renderGate();
  fireEvent.click(await screen.findByRole("button", { name: "役割を再確認" }));

  expect(await screen.findByText("調査の画面")).toBeTruthy();
});

test("役割を持たない画面で「役割を再確認」を押し、役割を与えられていれば調査の画面を出す", async () => {
  stubFetch({
    "GET /api/v0/session/me": () => jsonResponse(200, accountSessionJson()),
    "GET /api/v0/members/me": inTurn(investigationNotFound, () =>
      jsonResponse(200, memberJson("viewer")),
    ),
  });

  renderGate();
  fireEvent.click(await screen.findByRole("button", { name: "役割を再確認" }));

  expect(await screen.findByText("役割: viewer")).toBeTruthy();
});

test("役割を読む要求が authentication_required を受け取ったら、同じ利用者でログインし直した後に読み直す", async () => {
  stubFetch({
    "GET /api/v0/session/me": () => jsonResponse(200, accountSessionJson()),
    "GET /api/v0/members/me": inTurn(authenticationRequired, () =>
      jsonResponse(200, memberJson("editor")),
    ),
    "POST /api/v0/session": loginAsRequested,
  });

  renderGate();
  await screen.findByRole("dialog", { name: "再ログイン" });
  await submitLogin("alice");

  expect(await screen.findByText("調査の画面")).toBeTruthy();
});

test("途中の要求が permission_denied を受け取ったら役割を読み直し、閲覧者になっていれば記録の操作を止める", async () => {
  stubFetch({
    "GET /api/v0/session/me": () => jsonResponse(200, accountSessionJson()),
    "GET /api/v0/members/me": inTurn(
      () => jsonResponse(200, memberJson("editor")),
      () => jsonResponse(200, memberJson("viewer")),
    ),
    "GET /api/v0/stages": () =>
      jsonResponse(403, { code: "permission_denied", message: "x" }),
  });

  renderGate();
  fireEvent.change(await screen.findByLabelText("書きかけのメモ"), {
    target: { value: "port 5985 を読んだ" },
  });
  expect(screen.getByRole("button", { name: "記録する" })).toBeEnabled();
  fireEvent.click(screen.getByRole("button", { name: "段階の状態を取り直す" }));

  expect(await screen.findByText("役割: viewer")).toBeTruthy();
  expect(screen.getByRole("button", { name: "記録する" })).toBeDisabled();
  expect(memoValue()).toBe("port 5985 を読んだ");
});

test("役割を読めている間の読み直しが internal_error を受け取っても、調査の画面と入力を保つ", async () => {
  let reads = 0;
  stubFetch({
    "GET /api/v0/session/me": () => jsonResponse(200, accountSessionJson()),
    "GET /api/v0/members/me": () => {
      reads += 1;
      return reads === 1
        ? jsonResponse(200, memberJson("editor"))
        : jsonResponse(500, { code: "internal_error", message: "x" });
    },
    "GET /api/v0/stages": () =>
      jsonResponse(403, { code: "permission_denied", message: "x" }),
  });

  renderGate();
  fireEvent.change(await screen.findByLabelText("書きかけのメモ"), {
    target: { value: "port 5985 を読んだ" },
  });
  fireEvent.click(screen.getByRole("button", { name: "段階の状態を取り直す" }));

  await waitFor(() => expect(reads).toBe(2));
  await new Promise((resolve) => setTimeout(resolve, 0));
  expect(screen.getByText("調査の画面")).toBeTruthy();
  expect(memoValue()).toBe("port 5985 を読んだ");
});

test("途中の要求が investigation_not_found を受け取り、役割を外されていれば役割を持たない画面へ移す", async () => {
  stubFetch({
    "GET /api/v0/session/me": () => jsonResponse(200, accountSessionJson()),
    "GET /api/v0/members/me": inTurn(
      () => jsonResponse(200, memberJson("editor")),
      investigationNotFound,
    ),
    "GET /api/v0/stages": investigationNotFound,
  });

  renderGate();
  fireEvent.click(
    await screen.findByRole("button", { name: "段階の状態を取り直す" }),
  );

  await expectNoRole();
  expect(screen.queryByText("調査の画面")).toBeNull();
});

test("別の利用者がログインし直すと、その利用者の役割を読み直す", async () => {
  let signedInLogin = "alice";
  stubFetch({
    "GET /api/v0/session/me": () => jsonResponse(200, accountSessionJson()),
    "GET /api/v0/members/me": () =>
      jsonResponse(
        200,
        memberJson(
          signedInLogin === "alice" ? "admin" : "viewer",
          signedInLogin,
        ),
      ),
    "GET /api/v0/stages": authenticationRequired,
    "POST /api/v0/session": (init) => {
      signedInLogin = JSON.parse(String(init?.body)).login;
      return loginAsRequested(init);
    },
  });

  renderGate();
  expect(await screen.findByText("役割: admin")).toBeTruthy();
  await expireSessionWhileTyping();
  await submitLogin("bob");

  expect(await screen.findByText("役割: viewer")).toBeTruthy();
  expect(screen.getByText(textIs("ログイン中: 別の利用者"))).toBeTruthy();
});

test("役割を読み込んでいる間は、調査の画面を出さない", async () => {
  stubFetch({
    "GET /api/v0/session/me": () => jsonResponse(200, accountSessionJson()),
    "GET /api/v0/members/me": () => new Promise<Response>(() => {}),
  });

  renderGate();

  expect(await screen.findByText("調査の役割の確認中")).toBeTruthy();
  expect(screen.queryByText("調査の画面")).toBeNull();
});
