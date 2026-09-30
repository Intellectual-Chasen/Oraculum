// @vitest-environment jsdom
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { useEffect, useRef } from "react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { SignedInContext } from "@/shared/ui/SignedInAccount";
import { jsonResponse } from "@/testdata/http";
import { signedInAlice, signedInViewer } from "@/testdata/session";
import {
  changeNoticeMs,
  fitsKeepalive,
  lastWorkspaceStorageKey,
  presenceDelayMs,
  type WorkspaceAppProps,
  WorkspaceSession,
  workspaceSaveDelayMs,
} from "./WorkspaceSession";
import type { WorkspaceState } from "./workspaceState";

/** 読める画面の状態。marker を比べる相手の収集元の欄に入れ、どの状態かを見分ける。 */
function stateJson(marker: string) {
  return {
    dockLayout: { panels: {} },
    recordFilter: {},
    comparedSourceId: marker,
    searchTerms: { contains: [], excludes: [] },
    matchConditions: { conditions: [] },
    nodeScope: { enabled: false, depth: 1 },
    history: { places: [], index: -1 },
    bookmarks: [],
    graph: {
      criteria: { depth: 1 },
      view: { kind: "auto" },
      drawLimit: "all",
      mergeSameAccount: false,
    },
  };
}

type Stored = {
  id: string;
  name: string;
  owner: string;
  revision: number;
  updatedAt: string;
  updatedBy: string;
  access: "owner" | "edit" | "view";
  state: unknown;
  shares?: { login: string; access: string }[];
};

function stored(id: string, name: string, state: unknown): Stored {
  return {
    id,
    name,
    owner: "local",
    access: "owner",
    revision: 1,
    updatedAt: "2031-01-01T00:00:00Z",
    updatedBy: "local",
    state,
  };
}

/** ワークスペースの API を memory の上で真似る。 */
function stubServer(initial: Stored[]) {
  const store = new Map(initial.map((item) => [item.id, item]));
  let created = 0;
  // 「method path」ごとに、次の 1 回を 500 で失敗させる要求。
  const failsNext = new Set<string>();
  // ワークスペースごとに、欄を最後に変えた revision。"*" は全体の置き換えを表す。
  const fieldRevisions = new Map<string, Record<string, number>>();
  const results = new Map<string, unknown>();
  const presences: {
    connectionId: string;
    selection: unknown;
    following: string | null;
  }[] = [];
  /** login の利用者が欄を変えたことにする。新しい revision を返す。 */
  const changeBy = (
    id: string,
    login: string,
    patch: Record<string, unknown>,
  ) => {
    const item = store.get(id);
    if (item === undefined) throw new Error(`no workspace ${id}`);
    const revision = item.revision + 1;
    const state = { ...(item.state as Record<string, unknown>), ...patch };
    store.set(id, { ...item, state, revision, updatedBy: login });
    const revisions = fieldRevisions.get(id) ?? {};
    for (const field of Object.keys(patch)) revisions[field] = revision;
    fieldRevisions.set(id, revisions);
    return revision;
  };
  const notFound = () =>
    jsonResponse(404, { code: "record_not_found", message: "not found" });
  const mock = vi.fn(async (input: string, init?: RequestInit) => {
    const method = init?.method ?? "GET";
    if (failsNext.delete(`${method} ${input}`)) {
      return jsonResponse(500, { code: "internal_error", message: "x" });
    }
    const body =
      init?.body === undefined ? undefined : JSON.parse(`${init.body}`);
    if (input === "/api/v0/workspaces") {
      if (method === "GET") {
        return jsonResponse(200, {
          workspaces: [...store.values()].map(({ state: _s, ...rest }) => rest),
        });
      }
      created += 1;
      // 名前を持たない作成は、server が「ワークスペース N」の最大の N に 1 を足して名前を付ける。
      const numbers = [...store.values()].map((other) =>
        Number(/^ワークスペース (\d+)$/.exec(other.name)?.[1] ?? 0),
      );
      const name = body.name ?? `ワークスペース ${Math.max(0, ...numbers) + 1}`;
      const item = stored(`w-new-${created}`, name, body.state);
      store.set(item.id, item);
      return jsonResponse(201, item);
    }
    const [rawId = "", sub, rawLogin = ""] = input
      .slice("/api/v0/workspaces/".length)
      .split("/");
    const id = decodeURIComponent(rawId);
    const item = store.get(id);
    if (item === undefined) return notFound();
    if (sub === "shares") {
      const login = decodeURIComponent(rawLogin);
      const others = (item.shares ?? []).filter(
        (share) => share.login !== login,
      );
      if (method === "DELETE") {
        if (others.length === (item.shares ?? []).length) return notFound();
        store.set(id, { ...item, shares: others });
        return new Response(null, { status: 204 });
      }
      if (login === "stranger") {
        return jsonResponse(400, {
          code: "invalid_request",
          message: "the login has no role",
        });
      }
      const share = {
        login,
        access: body.access,
        grantedBy: "local",
        grantedAt: "2031-01-02T00:00:00Z",
      };
      store.set(id, { ...item, shares: [...others, share] });
      return jsonResponse(200, share);
    }
    if (sub === "presence") {
      presences.push(body);
      return new Response(null, { status: 204 });
    }
    if (sub === "changes") {
      if (item.access === "view") {
        return jsonResponse(403, { code: "permission_denied", message: "x" });
      }
      const previous = results.get(body.clientChangeId);
      if (previous !== undefined) return jsonResponse(200, previous);
      const revisions = fieldRevisions.get(id) ?? {};
      const fields = Object.keys(body.patch).filter(
        (field) => (revisions[field] ?? 0) > body.baseRevision,
      );
      if ((revisions["*"] ?? 0) > body.baseRevision) fields.splice(0, 0, "*");
      if (fields.length > 0) {
        return jsonResponse(409, {
          code: "workspace_changed",
          message: "x",
          conflict: {
            fields: fields.includes("*") ? ["*"] : fields,
            revision: item.revision,
            state: item.state,
            updatedBy: item.updatedBy,
          },
        });
      }
      const result = changeBy(id, "local", body.patch);
      const value = {
        revision: result,
        clientChangeId: body.clientChangeId,
        updatedBy: "local",
      };
      results.set(body.clientChangeId, value);
      return jsonResponse(200, value);
    }
    if (method === "GET") return jsonResponse(200, item);
    if (method === "PUT" && item.access === "view") {
      return jsonResponse(403, { code: "permission_denied", message: "x" });
    }
    if (method === "DELETE") {
      store.delete(id);
      return new Response(null, { status: 204 });
    }
    if (body.baseRevision !== item.revision) {
      return jsonResponse(409, { code: "workspace_changed", message: "x" });
    }
    const next = {
      ...item,
      name: body.name,
      state: body.state,
      revision: item.revision + 1,
    };
    store.set(id, next);
    fieldRevisions.set(id, { "*": next.revision });
    return jsonResponse(200, next);
  });
  vi.stubGlobal("fetch", mock);
  const requests = () =>
    mock.mock.calls.map(([input, init]) => `${init?.method ?? "GET"} ${input}`);
  /** 変更を送った要求の本文。 */
  const changes = () =>
    mock.mock.calls
      .filter(([input]) => input.endsWith("/changes"))
      .map(([, init]) => JSON.parse(`${init?.body}`));
  return {
    store,
    mock,
    requests,
    failsNext,
    changeBy,
    changes,
    presences,
  };
}

/** 配信の代わり。test が事象を流す。 */
class FakeEventSource {
  static instances: FakeEventSource[] = [];
  onmessage: ((message: MessageEvent) => void) | null = null;
  closed = false;
  constructor(readonly url: string) {
    FakeEventSource.instances.push(this);
  }
  close() {
    this.closed = true;
  }
  /** 接続の id。server が接続を見分けるのに使う。 */
  get connectionId() {
    return new URL(this.url, "http://localhost").searchParams.get(
      "connectionId",
    );
  }
}

/** 開いている接続へ事象を流す。 */
async function emit(event: unknown) {
  const source = FakeEventSource.instances.at(-1);
  if (source === undefined) throw new Error("no event source");
  await act(async () => {
    source.onmessage?.(
      new MessageEvent("message", { data: JSON.stringify(event) }),
    );
  });
}

function ownConnectionId() {
  return FakeEventSource.instances.at(-1)?.connectionId ?? "";
}

let changes = 0;

/**
 * 調査の画面の代わり。開いた直後に状態を 1 回知らせ、ボタンで状態の欄を 1 つ変える。開いた
 * 直後の状態は、作業場所が配置を書き出し直した値を持ち、保存した値と文字列が異なる。状態が
 * 適用し直されると、その状態を知らせる。
 */
function FakeApp(props: WorkspaceAppProps) {
  const { workspaceState, onWorkspaceStateChange } = props;
  const shown = useRef<WorkspaceState>(stateJson("") as WorkspaceState);
  const firstRevision = useRef(workspaceState?.revision);
  const change = (fields: Partial<WorkspaceState>) => {
    shown.current = { ...shown.current, ...fields };
    onWorkspaceStateChange(shown.current);
  };
  // biome-ignore lint/correctness/useExhaustiveDependencies: 開いた直後に 1 回だけ知らせる。
  useEffect(() => {
    change({
      ...(workspaceState?.state ?? {}),
      dockLayout: { panels: { search: {} } },
    });
  }, []);
  // biome-ignore lint/correctness/useExhaustiveDependencies: revision が変わったときだけ知らせる。
  useEffect(() => {
    if (
      workspaceState === undefined ||
      workspaceState.revision === firstRevision.current
    )
      return;
    // 作業場所は適用した配置を書き出し直し、受け取った値と文字列が異なる値を知らせる。
    const layout = workspaceState.state.dockLayout as Record<string, unknown>;
    shown.current = {
      ...workspaceState.state,
      dockLayout: { ...layout, written: true },
    };
    onWorkspaceStateChange(shown.current);
  }, [workspaceState?.revision]);
  return (
    <div>
      <p>適用した状態: {workspaceState?.state.comparedSourceId ?? "なし"}</p>
      <p>選んだ収集元: {workspaceState?.state.selectedSourceId ?? "なし"}</p>
      <p>選んだエッジ: {workspaceState?.state.selectedEdgeId ?? "なし"}</p>
      <button
        type="button"
        onClick={() => {
          changes += 1;
          change({ comparedSourceId: `c${changes}` });
        }}
      >
        画面を変える
      </button>
      <button
        type="button"
        onClick={() => {
          changes += 1;
          change({ selectedEdgeId: `e${changes}` });
        }}
      >
        エッジを選ぶ
      </button>
      <button
        type="button"
        onClick={() => change({ selectedEdgeId: undefined })}
      >
        エッジの選択を外す
      </button>
      <button
        type="button"
        onClick={() => {
          changes += 1;
          change({
            history: {
              places: [
                {
                  kind: "node",
                  node: { id: `n${changes}`, label: `n${changes}` },
                },
              ],
              index: 0,
            },
          });
        }}
      >
        履歴を足す
      </button>
      <button
        type="button"
        onClick={() => {
          changes += 1;
          change({
            comparedSourceId: `c${changes}`,
            selectedSourceId: `s${changes}`,
            selectedEdgeId: `e${changes}`,
            searchTerms: { contains: [`t${changes}`], excludes: [] },
            history: {
              places: [
                {
                  kind: "node",
                  node: { id: `n${changes}`, label: `n${changes}` },
                },
              ],
              index: 0,
            },
          });
        }}
      >
        五つのフィールドを変える
      </button>
      <button
        type="button"
        onClick={() => {
          changes += 1;
          change({
            dockLayout: { panels: { search: {} }, grid: "p".repeat(2000) },
            comparedSourceId: `c${changes}`,
            searchTerms: { contains: ["m".repeat(500)], excludes: [] },
          });
        }}
      >
        配置と条件を変える
      </button>
      <button
        type="button"
        onClick={() => {
          changes += 1;
          const places = Array.from({ length: 1000 }, (_, index) => ({
            kind: "node" as const,
            node: { id: `n${index}`, label: `n${index}-${"x".repeat(80)}` },
          }));
          change({
            comparedSourceId: `c${changes}`,
            history: { places, index: places.length - 1 },
          });
        }}
      >
        大きい履歴と画面を変える
      </button>
      {props.workspaceBar}
      {props.workspacePanel}
    </div>
  );
}

function renderSession() {
  return render(
    <WorkspaceSession>{(props) => <FakeApp {...props} />}</WorkspaceSession>,
  );
}

async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

/** 画面の「名前: 値」の組のうち、name の組の文字列の一覧。 */
function pairTexts(name: string): string[] {
  return Array.from(document.querySelectorAll(".value-pairs > li"))
    .map((item) => item.textContent ?? "")
    .filter((text) => text.startsWith(`${name}: `));
}

/** 他の利用者の変更の知らせを、変更した利用者と項目の組で出しているか。 */
function changeShown(who: string, fields: string): boolean {
  return (
    pairTexts("変更した利用者").includes(`変更した利用者: ${who}`) &&
    pairTexts("変更した項目").includes(`変更した項目: ${fields}`)
  );
}

/** 表か一覧の、名前 name の行の中を探す。ワークスペースの表と利用者の一覧の行は名前を持つ。 */
function row(list: string, name: string) {
  const table = screen.queryByRole("table", { name: list });
  return within(
    table === null
      ? within(screen.getByRole("list", { name: list })).getByRole("listitem", {
          name,
        })
      : within(table).getByRole("row", { name }),
  );
}

/** ワークスペースの表の、見出しを除く行。 */
function workspaceRows(table: string): HTMLElement[] {
  return within(screen.getByRole("table", { name: table }))
    .getAllByRole("row")
    .slice(1);
}

/** 状態を適用した画面を待ち、開いた直後の通知を送り終えるまで待つ。 */
async function openedWith(marker: string) {
  await screen.findByText(`適用した状態: ${marker}`);
  await act(async () => {});
}

const localKey = lastWorkspaceStorageKey("local");

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  changes = 0;
  FakeEventSource.instances = [];
  vi.stubGlobal("EventSource", FakeEventSource);
});

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

test("前回開いていたワークスペースを、ログイン名ごとの鍵から開く", async () => {
  stubServer([
    stored("w1", "一つ目", stateJson("a")),
    stored("w2", "二つ目", stateJson("b")),
  ]);
  localStorage.setItem(localKey, "w1");
  localStorage.setItem(lastWorkspaceStorageKey("alice"), "w2");
  render(
    <SignedInContext.Provider value={signedInViewer}>
      <WorkspaceSession>{(props) => <FakeApp {...props} />}</WorkspaceSession>
    </SignedInContext.Provider>,
  );
  expect(await screen.findByText("適用した状態: b")).toBeTruthy();
});

test("前回のワークスペースが無ければ一覧の先頭を開き、その id を記録する", async () => {
  stubServer([
    stored("w1", "一つ目", stateJson("a")),
    stored("w2", "二つ目", stateJson("b")),
  ]);
  localStorage.setItem(localKey, "gone");
  renderSession();
  expect(await screen.findByText("適用した状態: a")).toBeTruthy();
  expect(localStorage.getItem(localKey)).toBe("w1");
});

test("開いているワークスペースを一覧の先頭に出し、ほかは応答の順に並べる", async () => {
  stubServer([
    stored("w1", "一つ目", stateJson("a")),
    stored("w2", "二つ目", stateJson("b")),
    stored("w3", "三つ目", stateJson("c")),
  ]);
  localStorage.setItem(localKey, "w3");
  renderSession();
  expect(await screen.findByText("適用した状態: c")).toBeTruthy();
  const names = workspaceRows("自分のワークスペース").map(
    (item) => item.textContent?.match(/[一二三]つ目/)?.[0],
  );
  expect(names).toEqual(["三つ目", "一つ目", "二つ目"]);
});

test("一覧が空なら新しいワークスペースを作って開き、読み込みの間は画面を出さない", async () => {
  const { store } = stubServer([]);
  renderSession();
  expect(screen.getByRole("status").textContent).toBe(
    "ワークスペースの読み込み中",
  );
  expect(screen.queryByText(/適用した状態/)).toBeNull();
  expect(await screen.findByText("適用した状態: なし")).toBeTruthy();
  expect([...store.values()].map((item) => [item.name, item.state])).toEqual([
    ["ワークスペース 1", {}],
  ]);
});

const changesPath = "POST /api/v0/workspaces/w1/changes";

test("開いた直後の通知を保存せず、変化を待ち時間の後に変わった欄だけを送る。localStorage には開いた id だけを置く", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSession();
  await openedWith("a");
  await advance(workspaceSaveDelayMs * 2);
  expect(server.requests()).not.toContain(changesPath);

  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  await advance(workspaceSaveDelayMs / 2);
  expect(server.requests()).not.toContain(changesPath);
  await advance(workspaceSaveDelayMs / 2);
  await savedStatus();
  expect(server.changes()).toEqual([
    {
      clientChangeId: expect.any(String),
      baseRevision: 1,
      patch: { comparedSourceId: "c1" },
    },
  ]);
  expect(server.store.get("w1")).toMatchObject({
    revision: 2,
    state: { comparedSourceId: "c1" },
  });
  expect(Object.keys(localStorage)).toEqual([localKey]);
});

test("値を外した欄を、欄を消す変更として保存する", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSession();
  await openedWith("a");

  fireEvent.click(screen.getByRole("button", { name: "エッジを選ぶ" }));
  await advance(workspaceSaveDelayMs);
  await savedStatus();
  fireEvent.click(screen.getByRole("button", { name: "エッジの選択を外す" }));
  await advance(workspaceSaveDelayMs);
  await savedStatus();

  expect(server.changes().map((change) => change.patch)).toEqual([
    { selectedEdgeId: "e1" },
    { selectedEdgeId: null },
  ]);
});

test("同じ欄を別の利用者が先に変えていたら最新を適用し、競合した欄と自分の値を閉じるまで出し、自分の値で送り直せる", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSession();
  await openedWith("a");
  await emit({
    type: "presence",
    connections: [
      {
        connectionId: "bob-1",
        login: "bob",
        displayName: "田中",
        selection: null,
        following: null,
      },
    ],
  });
  server.changeBy("w1", "bob", { comparedSourceId: "other" });
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  await advance(workspaceSaveDelayMs);
  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain("変更した利用者: 田中");
  expect(alert.textContent).toContain("自分の値の保存: 未反映");
  expect(conflictRows()).toEqual([["比較する収集元", "other", "c1"]]);
  expect(screen.getByText("適用した状態: other")).toBeTruthy();
  await advance(workspaceSaveDelayMs * 10);
  expect(screen.getByRole("alert")).toBeTruthy();

  fireEvent.click(screen.getByRole("button", { name: "自分の値を再送信" }));
  await savedStatus();
  expect(screen.getByText("適用した状態: c1")).toBeTruthy();
  expect(server.store.get("w1")).toMatchObject({
    revision: 3,
    state: { comparedSourceId: "c1" },
  });
  expect(server.changes().at(-1)).toMatchObject({
    baseRevision: 2,
    patch: { comparedSourceId: "c1" },
  });
});

test("競合の通知は閉じる操作で消える", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSession();
  await openedWith("a");
  server.changeBy("w1", "bob", { comparedSourceId: "other" });
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  await advance(workspaceSaveDelayMs);
  expect((await screen.findByRole("alert")).textContent).toContain(
    "変更した利用者: bob",
  );
  fireEvent.click(screen.getByRole("button", { name: "競合の通知を閉じる" }));
  expect(screen.queryByRole("alert")).toBeNull();
});

/** 競合の通知の表の行を、フィールドの名前・相手の値・自分の値の文字列の組で返す。 */
function conflictRows() {
  const table = within(screen.getByRole("alert")).getByRole("table", {
    name: "競合したフィールド",
  });
  return within(table)
    .getAllByRole("row")
    .slice(1)
    .map((row) =>
      [...row.querySelectorAll("th, td")].map((cell) => cell.textContent),
    );
}

test("競合の通知は、配置の値を出さずにフィールドの名前だけを出し、ほかの値を詰めて出し、表を scroll の中に置く", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSession();
  await openedWith("a");
  server.changeBy("w1", "bob", {
    dockLayout: { panels: { search: {} }, grid: "q".repeat(2000) },
    comparedSourceId: "other",
    searchTerms: { contains: ["t".repeat(500)], excludes: [] },
  });
  fireEvent.click(screen.getByRole("button", { name: "配置と条件を変える" }));
  await advance(workspaceSaveDelayMs);
  const alert = await screen.findByRole("alert");
  expect(alert.textContent).not.toContain("ppp");
  expect(alert.textContent).not.toContain("qqq");
  const rows = conflictRows();
  expect(rows.map((row) => row[0])).toEqual([
    "作業場所の配置",
    "比較する収集元",
    "検索の条件",
  ]);
  expect(rows[0]).toEqual(["作業場所の配置", "", ""]);
  expect(rows[1]).toEqual(["比較する収集元", "other", "c1"]);
  for (const cell of rows[2]?.slice(1) ?? []) {
    expect(cell?.length).toBeLessThanOrEqual(201);
    expect(cell?.endsWith("…")).toBe(true);
  }
  const scroller = within(alert).getByRole("table").parentElement;
  expect(scroller?.className).toContain("overflow-auto");
  expect(scroller?.className).toContain("max-h-");
});

test("他の利用者の変更を、送っていない欄を除いて画面へ重ね、帯に知らせる", async () => {
  stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSession();
  await openedWith("a");
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  await emit({
    type: "change",
    revision: 2,
    fields: ["comparedSourceId", "selectedSourceId"],
    patch: { comparedSourceId: "x", selectedSourceId: "src" },
    actor: "bob",
  });
  expect(screen.getByText("選んだ収集元: src")).toBeTruthy();
  expect(screen.getByText("適用した状態: c1")).toBeTruthy();
  expect(changeShown("bob", "比較する収集元、選択中の収集元")).toBe(true);
  await advance(changeNoticeMs);
  expect(pairTexts("変更した利用者")).toEqual([]);
});

test("自分の変更の配信では revision だけを進め、画面へ適用せず帯にも出さない", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSession();
  await openedWith("a");
  server.changeBy("w1", "bob", { selectedSourceId: "src" });
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  await advance(workspaceSaveDelayMs);
  await savedStatus();
  const [sent] = server.changes();
  // 間の revision 2 の変更が配信で届き、続いて自分の変更 (revision 3) が届く。
  await emit({
    type: "change",
    revision: 2,
    fields: ["selectedSourceId"],
    patch: { selectedSourceId: "src" },
    actor: "bob",
  });
  expect(screen.getByText("選んだ収集元: src")).toBeTruthy();
  await advance(changeNoticeMs);
  await emit({
    type: "change",
    revision: 3,
    fields: ["comparedSourceId"],
    patch: { comparedSourceId: "stale" },
    actor: "local",
    clientChangeId: sent.clientChangeId,
  });
  expect(screen.queryByText("適用した状態: stale")).toBeNull();
  expect(pairTexts("変更した利用者")).toEqual([]);
  fireEvent.click(screen.getByRole("button", { name: "エッジを選ぶ" }));
  await advance(workspaceSaveDelayMs);
  expect(server.changes().map((change) => change.baseRevision)).toEqual([1, 3]);
  expect(server.changes().at(-1)?.patch).toEqual({ selectedEdgeId: "e2" });
});

test("他人の変更の配信より先に自分の応答が届いたら元の revision を進めず、次の送信で同じ欄の競合を知らせる", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSession();
  await openedWith("a");
  // bob の変更 (revision 2) は配信で届かない。
  server.changeBy("w1", "bob", { comparedSourceId: "other" });
  fireEvent.click(screen.getByRole("button", { name: "エッジを選ぶ" }));
  await advance(workspaceSaveDelayMs);
  await savedStatus();
  expect(server.store.get("w1")?.revision).toBe(3);

  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  await advance(workspaceSaveDelayMs);
  await screen.findByRole("alert");
  expect(conflictRows()).toEqual([["比較する収集元", "other", "c2"]]);
  expect(server.changes().map((change) => change.baseRevision)).toEqual([1, 1]);
  expect(screen.getByText("適用した状態: other")).toBeTruthy();
});

/** 画面で比べる相手の収集元を変え、送る前に bob の同じ欄の変更 (revision 2) を配信で届ける。 */
async function receiveOnUnsentField(
  server: ReturnType<typeof stubServer>,
  event: unknown = {
    type: "change",
    revision: 2,
    fields: ["comparedSourceId"],
    patch: { comparedSourceId: "x" },
    actor: "bob",
  },
) {
  renderSession();
  await openedWith("a");
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  server.changeBy("w1", "bob", { comparedSourceId: "x" });
  await emit(event);
  const alert = screen.getByRole("alert");
  expect(alert.textContent).toContain("変更した利用者: bob");
  expect(alert.textContent).toContain("自分の値の保存: 未送信");
  expect(conflictRows()).toEqual([["比較する収集元", "x", "c1"]]);
  await advance(workspaceSaveDelayMs * 2);
  expect(server.changes()).toEqual([]);
}

test("送っていない欄に他人の変更が届いたら、その場で競合を知らせて送らずに待ち、相手の値を採れる", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  await receiveOnUnsentField(server);
  fireEvent.click(screen.getByRole("button", { name: "相手の値を採用" }));
  expect(screen.getByText("適用した状態: x")).toBeTruthy();
  expect(screen.queryByRole("alert")).toBeNull();
  await advance(workspaceSaveDelayMs * 2);
  expect(server.changes()).toEqual([]);
  expect(server.store.get("w1")?.state).toMatchObject({
    comparedSourceId: "x",
  });
});

test("送っていない欄に他人の変更が届いた後、自分の値で送り直せる", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  await receiveOnUnsentField(server);
  fireEvent.click(screen.getByRole("button", { name: "自分の値を再送信" }));
  await savedStatus();
  expect(server.changes()).toEqual([
    {
      clientChangeId: expect.any(String),
      baseRevision: 2,
      patch: { comparedSourceId: "c1" },
    },
  ]);
});

test("snapshot との差が送っていない欄に重なるときも、その場で競合を知らせる", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  await receiveOnUnsentField(server, {
    type: "snapshot",
    revision: 2,
    state: stateJson("x"),
    updatedBy: "bob",
    name: "一つ目",
  });
});

test("送信中の欄に他人の変更が届いたら判定を server の 409 に任せ、通知を閉じた後の編集を送る", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSession();
  await openedWith("a");
  const fallback = server.mock.getMockImplementation();
  let release = () => {};
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  server.mock.mockImplementation(async (input, init) => {
    if (input.endsWith("/changes")) await gate;
    return fallback?.(input, init) ?? jsonResponse(500, {});
  });
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  server.changeBy("w1", "bob", { comparedSourceId: "x" });
  await advance(workspaceSaveDelayMs);
  await emit({
    type: "change",
    revision: 2,
    fields: ["comparedSourceId"],
    patch: { comparedSourceId: "x" },
    actor: "bob",
  });
  expect(screen.queryByRole("button", { name: "相手の値を採用" })).toBeNull();
  await act(async () => release());
  expect((await screen.findByRole("alert")).textContent).toContain(
    "自分の値の保存: 未反映",
  );
  fireEvent.click(screen.getByRole("button", { name: "競合の通知を閉じる" }));
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  await advance(workspaceSaveDelayMs);
  await waitFor(() =>
    expect(server.changes().at(-1)?.patch).toEqual({ comparedSourceId: "c2" }),
  );
});

test("届いた変更の通知の後に同じ欄を編集してから送り直すと、画面の最新の値を送る", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  await receiveOnUnsentField(server);
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  fireEvent.click(screen.getByRole("button", { name: "自分の値を再送信" }));
  await savedStatus();
  expect(server.changes().map((change) => change.patch)).toEqual([
    { comparedSourceId: "c2" },
  ]);
});

test("ページを離れるときに送信中の変更と後の編集が同じ欄を持てば、新しい clientChangeId で 1 件にまとめる", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  server.failsNext.add(changesPath);
  await openAndChange(server);
  await screen.findByRole("button", { name: "再保存" });
  const [failed] = server.changes();
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  window.dispatchEvent(new Event("pagehide"));
  const sent = server.mock.mock.calls
    .filter(([input, init]) => input.endsWith("/changes") && init?.keepalive)
    .map(([, init]) => JSON.parse(`${init?.body}`))
    .slice(1);
  // 中身が違う変更を同じ id で送ると、先に届いた方の再送として server がエラーを出さずに捨てる。
  expect(sent).toEqual([
    {
      ...failed,
      clientChangeId: expect.any(String),
      patch: { comparedSourceId: "c2" },
    },
  ]);
  expect(sent[0].clientChangeId).not.toBe(failed.clientChangeId);
});

/** bob が配置を変えた change を revision で届ける。 */
function layoutChange(revision: number) {
  return {
    type: "change",
    revision,
    fields: ["dockLayout"],
    patch: { dockLayout: { panels: { search: {} }, active: `t${revision}` } },
    actor: "bob",
  };
}

test("適用した配置を画面が書き出し直しても、他人の配置の change を続けて受け取って競合にならず、何も送らない", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSession();
  await openedWith("a");
  await emit(layoutChange(2));
  await emit(layoutChange(3));
  await advance(workspaceSaveDelayMs * 2);
  expect(screen.queryByRole("alert")).toBeNull();
  expect(
    server
      .requests()
      .filter((line) => line.startsWith("PUT") || line.endsWith("/changes")),
  ).toEqual([]);
});

test("閲覧だけの共有先は、配信で競合の通知を出さずに適用する", async () => {
  const server = stubServer([sharedWith("s1", "調べもの", "s", "view")]);
  localStorage.setItem(aliceKey, "s1");
  renderSignedIn();
  await openedWith("s");
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  await emit({
    type: "change",
    revision: 2,
    fields: ["comparedSourceId"],
    patch: { comparedSourceId: "x" },
    actor: "bob",
  });
  await emit(layoutChange(3));
  expect(screen.queryByRole("alert")).toBeNull();
  expect(screen.getByText("適用した状態: x")).toBeTruthy();
  expect(server.changes()).toEqual([]);
});

test("自分の別のタブの欄の変更は帯に出す", async () => {
  stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSession();
  await openedWith("a");
  await emit({
    type: "change",
    revision: 2,
    fields: ["selectedSourceId"],
    patch: { selectedSourceId: "src" },
    actor: "local",
  });
  expect(changeShown("local", "選択中の収集元")).toBe(true);
});

test("1 回の保存で競合が何回続いても、残ったフィールドを送り続けて保存する", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSession();
  await openedWith("a");
  // 送る直前に、送る変更の先頭のフィールドを bob が変える。フィールドが 1 つ残るまで競合が続く。
  const theirs: Record<string, unknown> = {
    comparedSourceId: "bob",
    selectedSourceId: "bob",
    selectedEdgeId: "bob",
    searchTerms: { contains: ["bob"], excludes: [] },
    history: { places: [], index: -1 },
  };
  const fallback = server.mock.getMockImplementation();
  // 送るたびに、保存の失敗を出しているかを記録する。
  const failedAtSend: boolean[] = [];
  server.mock.mockImplementation(async (input, init) => {
    if (input.endsWith("/changes")) {
      failedAtSend.push(
        screen.queryByRole("button", { name: "再保存" }) !== null,
      );
      const fields = Object.keys(JSON.parse(`${init?.body}`).patch);
      const [first] = fields;
      if (fields.length > 1 && first !== undefined) {
        server.changeBy("w1", "bob", { [first]: theirs[first] });
      }
    }
    return fallback?.(input, init) ?? jsonResponse(500, {});
  });
  fireEvent.click(
    screen.getByRole("button", { name: "五つのフィールドを変える" }),
  );
  await advance(workspaceSaveDelayMs);
  await savedStatus();
  expect(failedAtSend).toEqual([false, false, false, false, false]);
  expect(screen.queryByRole("button", { name: "再保存" })).toBeNull();
  expect(Object.keys(server.changes().at(-1)?.patch ?? {})).toHaveLength(1);
  // 通知の表は、4 回の競合で失ったすべてのフィールドの行を、自分の値と一緒に持つ。
  const labels: Record<string, string> = {
    comparedSourceId: "比較する収集元",
    selectedSourceId: "選択中の収集元",
    selectedEdgeId: "選択中のエッジ",
    searchTerms: "検索の条件",
    history: "見た場所の履歴",
  };
  const saved = Object.keys(server.changes().at(-1)?.patch ?? {});
  const lost = Object.keys(labels).filter((field) => !saved.includes(field));
  const rows = conflictRows();
  expect(rows.map((row) => row[0]).sort()).toEqual(
    lost.map((field) => labels[field]).sort(),
  );
  for (const row of rows) expect(row[2]).not.toContain("bob");
  // bob が変えるのをやめた後に送り直すと、失ったすべての自分の値を送る。
  if (fallback !== undefined) server.mock.mockImplementation(fallback);
  fireEvent.click(screen.getByRole("button", { name: "自分の値を再送信" }));
  await advance(workspaceSaveDelayMs);
  await waitFor(() =>
    expect(server.store.get("w1")?.state).toMatchObject({
      comparedSourceId: "c1",
      selectedSourceId: "s1",
      selectedEdgeId: "e1",
    }),
  );
});

test("ページを離れるときは、送信中の変更を同じ clientChangeId で送り、その後の編集を別の変更で送る", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  server.failsNext.add(changesPath);
  await openAndChange(server);
  await screen.findByRole("button", { name: "再保存" });
  const [failed] = server.changes();
  fireEvent.click(screen.getByRole("button", { name: "エッジを選ぶ" }));
  window.dispatchEvent(new Event("pagehide"));
  const sent = server.mock.mock.calls
    .filter(([input, init]) => input.endsWith("/changes") && init?.keepalive)
    .map(([, init]) => JSON.parse(`${init?.body}`));
  expect(sent.slice(-2)).toEqual([
    failed,
    {
      clientChangeId: expect.any(String),
      baseRevision: 1,
      patch: { selectedEdgeId: "e2" },
    },
  ]);
  expect(sent.at(-1).clientChangeId).not.toBe(failed.clientChangeId);
});

test("snapshot と全体の change が持つ名前を出し、自分の全体の change は帯に出さない", async () => {
  stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSession();
  await openedWith("a");
  await emit({
    type: "snapshot",
    revision: 1,
    state: stateJson("a"),
    updatedBy: "local",
    name: "改名 1",
  });
  expect(screen.getByText(/ワークスペース: 改名 1/)).toBeTruthy();
  await emit({
    type: "change",
    revision: 2,
    fields: ["*"],
    patch: { ...stateJson("a"), selectedSourceId: "src" },
    actor: "local",
    name: "改名 2",
  });
  expect(screen.getByText(/ワークスペース: 改名 2/)).toBeTruthy();
  expect(
    within(
      screen.getByRole("table", { name: "自分のワークスペース" }),
    ).getByRole("row", { current: true }).textContent,
  ).toContain("改名 2");
  expect(screen.getByText("選んだ収集元: src")).toBeTruthy();
  expect(pairTexts("変更した利用者")).toEqual([]);
});

/** 自分のワークスペースの一覧から name を開く。 */
function openWorkspace(name: string) {
  fireEvent.click(
    row("自分のワークスペース", name).getByRole("button", {
      name: `${name} を開く`,
    }),
  );
}

/** 自分のワークスペースの name を削除し、確認の dialog で削除を押す。 */
function deleteWorkspace(name: string) {
  fireEvent.click(
    row("自分のワークスペース", name).getByRole("button", {
      name: `${name} を削除`,
    }),
  );
  fireEvent.click(
    within(screen.getByRole("alertdialog")).getByRole("button", {
      name: "ワークスペースを削除",
    }),
  );
}

/** 接続中の利用者の行の、画面に追従する button を押す。 */
function follow(rowName: string) {
  fireEvent.click(
    row("接続中の利用者", rowName).getByRole("button", {
      name: `${rowName} の画面に追従`,
    }),
  );
}

test("追従中に積んだ見た場所の履歴を、追従を終えたら戻し、保存しない", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSignedIn();
  await openedWith("a");
  await emit({
    type: "presence",
    connections: [
      connection("bob-1", "bob", "田中", {
        selection: { selectedEdgeId: "e-bob" },
      }),
    ],
  });
  follow("田中 bob");
  fireEvent.click(screen.getByRole("button", { name: "履歴を足す" }));
  fireEvent.click(screen.getByRole("button", { name: "追従を解除" }));
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  await advance(workspaceSaveDelayMs);
  await savedStatus();
  expect(server.changes().map((change) => change.patch)).toEqual([
    { comparedSourceId: "c2" },
  ]);
});

test("つなぎ直した後の snapshot は、送っていない欄を保ち、ほかの欄を適用する", async () => {
  stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSession();
  await openedWith("a");
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  await emit({
    type: "snapshot",
    revision: 3,
    state: { ...stateJson("z"), selectedSourceId: "src" },
    updatedBy: "bob",
    name: "一つ目",
  });
  expect(screen.getByText("選んだ収集元: src")).toBeTruthy();
  expect(screen.getByText("適用した状態: c1")).toBeTruthy();
});

function connection(
  connectionId: string,
  login: string,
  displayName: string,
  fields: { selection?: unknown; following?: string | null } = {},
) {
  return {
    connectionId,
    login,
    displayName,
    selection: fields.selection ?? null,
    following: fields.following ?? null,
  };
}

test("接続が複数なら数の button で一覧を開き、自分の接続に「自分」を付け、同じ利用者の接続を分ける", async () => {
  stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSignedIn();
  await openedWith("a");
  await emit({
    type: "presence",
    connections: [
      connection(ownConnectionId(), "alice", "石橋"),
      connection("bob-1", "bob", "田中"),
      connection("bob-2", "bob", "田中", { following: ownConnectionId() }),
    ],
  });
  expect(screen.queryByRole("list", { name: "接続中の利用者" })).toBeNull();
  // 追従されていることは、一覧を開かなくても見える。
  expect(pairTexts("自分に追従")).toEqual(["自分に追従: 田中"]);
  const trigger = screen.getByRole("button", { name: "接続中の利用者: 3" });
  expect(trigger.textContent).toBe("3");
  fireEvent.click(trigger);
  const people = await screen.findByRole("list", { name: "接続中の利用者" });
  const rows = Array.from(people.querySelectorAll(":scope > li"));
  expect(rows.map((item) => item.getAttribute("aria-label"))).toEqual([
    "石橋 alice",
    "田中 bob 1",
    "田中 bob 2",
  ]);
  expect(rows[0]?.textContent).toContain("状態: 自分");
  expect(within(rows[0] as HTMLElement).queryByRole("button")).toBeNull();
  expect(
    within(rows[1] as HTMLElement).getByRole("button", {
      name: "田中 bob 1 の画面に追従",
    }),
  ).toBeTruthy();
  expect(pairTexts("自分に追従")).toEqual(["自分に追従: 田中"]);
});

test("追従中は相手の選択を適用し、選択を保存しない。相手が消えると追従を終えて自分の選択に戻る", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSignedIn();
  await openedWith("a");
  fireEvent.click(screen.getByRole("button", { name: "エッジを選ぶ" }));
  await advance(workspaceSaveDelayMs);
  await savedStatus();
  const bob = connection("bob-1", "bob", "田中", {
    selection: { selectedEdgeId: "e-bob" },
  });
  await emit({ type: "presence", connections: [bob] });
  follow("田中 bob");
  expect(screen.getByText("選んだエッジ: e-bob")).toBeTruthy();
  expect(pairTexts("追従中")).toEqual(["追従中: 田中"]);
  await advance(presenceDelayMs);
  expect(server.presences.at(-1)).toMatchObject({
    following: "bob-1",
    selection: { selectedEdgeId: "e1" },
  });

  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  await advance(workspaceSaveDelayMs);
  await savedStatus();
  expect(server.changes().at(-1)?.patch).toEqual({ comparedSourceId: "c2" });
  expect(server.store.get("w1")?.state).toMatchObject({ selectedEdgeId: "e1" });

  await emit({ type: "presence", connections: [] });
  const ended = screen.getByText("追従の終了");
  expect(ended.closest('[role="status"]')?.textContent).toContain(
    "相手の接続の切断",
  );
  expect(screen.getByText("選んだエッジ: e1")).toBeTruthy();
});

test("追従を解除する操作で追従を終える", async () => {
  stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSignedIn();
  await openedWith("a");
  await emit({
    type: "presence",
    connections: [
      connection("bob-1", "bob", "田中", {
        selection: { selectedEdgeId: "e-bob" },
      }),
    ],
  });
  follow("田中 bob");
  expect(screen.getByText("選んだエッジ: e-bob")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "追従を解除" }));
  expect(screen.getByText("選んだエッジ: なし")).toBeTruthy();
  expect(pairTexts("追従中")).toEqual([]);
});

test("配信が共有の取り消しと削除を知らせると、それぞれを出して保存を止める", async () => {
  const server = stubServer([
    sharedWith("s1", "調べもの", "s", "edit"),
    stored("w1", "一つ目", stateJson("a")),
  ]);
  localStorage.setItem(aliceKey, "s1");
  renderSignedIn();
  await openedWith("s");
  await emit({ type: "closed", reason: "revoked" });
  expect(screen.getByRole("alert").textContent).toContain("共有の解除");
  expect(FakeEventSource.instances.at(-1)?.closed).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  await advance(workspaceSaveDelayMs * 2);
  expect(server.changes()).toEqual([]);

  fireEvent.click(
    screen.getByRole("button", { name: "自分のワークスペースに移動" }),
  );
  await openedWith("a");
  await emit({ type: "closed", reason: "deleted" });
  expect(screen.getByRole("alert").textContent).toContain("所有者による削除");
});

test("読めない状態は適用も上書きもせず、理由と作る操作と別のワークスペースを開く操作を出す", async () => {
  const unreadable = { dockLayout: { panels: { unknown: {} } } };
  const { store, requests } = stubServer([
    stored("w1", "壊れた", unreadable),
    stored("w2", "二つ目", stateJson("b")),
  ]);
  renderSession();
  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain("ワークスペース: 壊れた");
  expect(alert.textContent).toContain("状態: 読み込み失敗");
  expect(screen.queryByText(/適用した状態/)).toBeNull();
  expect(
    screen.getByRole("button", { name: "ワークスペースを新規作成" }),
  ).toBeTruthy();
  openWorkspace("二つ目");
  expect(await screen.findByText("適用した状態: b")).toBeTruthy();
  expect(store.get("w1")?.state).toEqual(unreadable);
  expect(requests().filter((line) => line.startsWith("PUT"))).toEqual([]);
});

test("別のワークスペースへ切り替える前に、保存していない変更を送る", async () => {
  const { store, requests } = stubServer([
    stored("w1", "一つ目", stateJson("a")),
    stored("w2", "二つ目", stateJson("b")),
  ]);
  renderSession();
  await openedWith("a");
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  openWorkspace("二つ目");
  expect(await screen.findByText("適用した状態: b")).toBeTruthy();
  const lines = requests();
  expect(lines.indexOf(changesPath)).toBeGreaterThan(-1);
  expect(lines.indexOf(changesPath)).toBeLessThan(
    lines.indexOf("GET /api/v0/workspaces/w2"),
  );
  expect(store.get("w1")?.state).toMatchObject({ comparedSourceId: "c1" });
});

test("パネルで作成・名前の変更・削除ができ、開いているワークスペースを消すと別のものを開く", async () => {
  const { store } = stubServer([
    stored("w1", "一つ目", stateJson("a")),
    stored("w2", "二つ目", stateJson("b")),
  ]);
  renderSession();
  await openedWith("a");
  const panel = () =>
    within(screen.getByRole("region", { name: "ワークスペース" }));

  fireEvent.click(
    panel().getByRole("button", { name: "ワークスペースを新規作成" }),
  );
  expect(await screen.findByText("適用した状態: なし")).toBeTruthy();
  expect(panel().getByRole("row", { current: true }).textContent).toContain(
    "ワークスペース 1",
  );

  fireEvent.click(
    row("自分のワークスペース", "二つ目").getByRole("button", {
      name: "二つ目 の名前を変更",
    }),
  );
  fireEvent.change(
    row("自分のワークスペース", "二つ目").getByRole("textbox", {
      name: "新しい名前",
    }),
    {
      target: { value: "調べ直し" },
    },
  );
  fireEvent.click(panel().getByRole("button", { name: "名前を保存" }));
  expect(await panel().findByText("調べ直し")).toBeTruthy();
  expect(store.get("w2")).toMatchObject({
    name: "調べ直し",
    state: { comparedSourceId: "b" },
  });

  deleteWorkspace("ワークスペース 1");
  expect(await screen.findByText("適用した状態: a")).toBeTruthy();
  expect([...store.keys()]).toEqual(["w1", "w2"]);
});

/** w1 を開き、画面を 1 回変えて、待ち時間の後の保存まで進める。 */
async function openAndChange(server: ReturnType<typeof stubServer>) {
  renderSession();
  await openedWith("a");
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  await advance(workspaceSaveDelayMs);
  return server;
}

async function savedStatus() {
  await waitFor(() =>
    expect(
      screen.getAllByRole("status").map((node) => node.textContent),
    ).toContain("保存済み"),
  );
}

test("保存に失敗したら、もう一度保存する操作で同じ clientChangeId の変更を送り直せる", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  server.failsNext.add(changesPath);
  await openAndChange(server);
  fireEvent.click(await screen.findByRole("button", { name: "再保存" }));
  await savedStatus();
  expect(server.store.get("w1")?.state).toMatchObject({
    comparedSourceId: "c1",
  });
  const [first, second] = server.changes();
  expect(first.clientChangeId).toEqual(expect.any(String));
  expect(second).toEqual(first);
});

test("保存に失敗している間は切り替えず、理由を出す", async () => {
  const server = stubServer([
    stored("w1", "一つ目", stateJson("a")),
    stored("w2", "二つ目", stateJson("b")),
  ]);
  server.failsNext.add(changesPath);
  await openAndChange(server);
  await screen.findByRole("button", { name: "再保存" });
  server.failsNext.add(changesPath);
  openWorkspace("二つ目");
  const region = within(screen.getByRole("region", { name: "ワークスペース" }));
  expect(
    (await region.findByRole("alert")).textContent?.includes(
      "ワークスペースの保存",
    ),
  ).toBe(true);
  expect(screen.getByText("適用した状態: a")).toBeTruthy();
  expect(server.requests()).not.toContain("GET /api/v0/workspaces/w2");
});

test("開いているワークスペースの削除に失敗しても、変更を保存し続ける", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  server.failsNext.add("DELETE /api/v0/workspaces/w1");
  renderSession();
  await openedWith("a");
  deleteWorkspace("一つ目");
  await screen.findByRole("alert");
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  await advance(workspaceSaveDelayMs);
  await savedStatus();
  expect(server.store.get("w1")?.state).toMatchObject({
    comparedSourceId: "c1",
  });
});

test("keepalive で送れる本文は 64 KiB までである", () => {
  // JSON.stringify の引用符 2 byte を除いた長さで境界を作る。
  expect(fitsKeepalive("a".repeat(64 * 1024 - 2))).toBe(true);
  expect(fitsKeepalive("a".repeat(64 * 1024 - 1))).toBe(false);
});

test("ページを離れるときは、保存していない変更を keepalive の要求でその場で送る", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSession();
  await openedWith("a");
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  window.dispatchEvent(new Event("pagehide"));
  const sent = server.mock.mock.calls.find(([input]) =>
    input.endsWith("/changes"),
  );
  expect(sent?.[1]?.keepalive).toBe(true);
  expect(JSON.parse(`${sent?.[1]?.body}`)).toMatchObject({
    baseRevision: 1,
    patch: { comparedSourceId: "c1" },
  });
});

test("ページを離れるときに変更が keepalive の本文に収まらなければ、履歴を外して残りを送り、履歴を保存していない値として残す", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSession();
  await openedWith("a");
  fireEvent.click(
    screen.getByRole("button", { name: "大きい履歴と画面を変える" }),
  );
  window.dispatchEvent(new Event("pagehide"));
  const sent = server.mock.mock.calls
    .filter(([input]) => input.endsWith("/changes"))
    .map(([, init]) => ({
      keepalive: init?.keepalive,
      body: JSON.parse(`${init?.body}`),
    }));
  expect(sent).toEqual([
    {
      keepalive: true,
      body: {
        clientChangeId: expect.any(String),
        baseRevision: 1,
        patch: { comparedSourceId: "c1" },
      },
    },
  ]);
  // ページに戻ると、外した履歴を次の保存で送る。
  await advance(workspaceSaveDelayMs);
  await waitFor(() =>
    expect(
      (
        server.store.get("w1")?.state as
          | { history?: { places: unknown[] } }
          | undefined
      )?.history?.places,
    ).toHaveLength(1000),
  );
});

test("開いているワークスペースの名前の変更を、画面の状態と一緒に保存する", async () => {
  const server = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSession();
  await openedWith("a");
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  fireEvent.click(
    row("自分のワークスペース", "一つ目").getByRole("button", {
      name: "一つ目 の名前を変更",
    }),
  );
  const input = screen.getByRole("textbox", { name: "新しい名前" });
  fireEvent.change(input, { target: { value: "   " } });
  expect(screen.getByRole("button", { name: "名前を保存" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  fireEvent.change(input, { target: { value: " 調べ直し " } });
  fireEvent.click(screen.getByRole("button", { name: "名前を保存" }));
  await waitFor(() =>
    expect(server.store.get("w1")).toMatchObject({
      name: "調べ直し",
      revision: 3,
      state: { comparedSourceId: "c1" },
    }),
  );
  const lines = server.requests();
  expect(lines.indexOf(changesPath)).toBeGreaterThan(-1);
  expect(lines.indexOf(changesPath)).toBeLessThan(
    lines.indexOf("PUT /api/v0/workspaces/w1"),
  );
});

function sharedWith(
  id: string,
  name: string,
  marker: string,
  access: "edit" | "view",
): Stored {
  return {
    ...stored(id, name, stateJson(marker)),
    owner: "bob",
    updatedBy: "bob",
    access,
  };
}

/** 上部の帯の自分用に複製する button。一覧の行の同じ名前の button と分ける。 */
function viewOnlyCopyButton(): HTMLElement {
  const [button] = screen
    .getAllByRole("button", { name: "自分用に複製" })
    .filter((item) => item.closest("li") === null);
  if (button === undefined) throw new Error("the copy button is missing");
  return button;
}

function renderSignedIn() {
  return render(
    <SignedInContext.Provider value={signedInAlice}>
      <WorkspaceSession>{(props) => <FakeApp {...props} />}</WorkspaceSession>
    </SignedInContext.Provider>,
  );
}

const aliceKey = lastWorkspaceStorageKey("alice");

test("自分のワークスペースと共有されたワークスペースを分けて出し、共有の設定は自分の行だけに出す", async () => {
  stubServer([
    stored("w1", "一つ目", stateJson("a")),
    sharedWith("s1", "調べもの", "s", "view"),
  ]);
  renderSignedIn();
  await openedWith("a");
  const own = within(
    screen.getByRole("table", { name: "自分のワークスペース" }),
  );
  const shared = within(
    screen.getByRole("table", { name: "共有されたワークスペース" }),
  );
  expect(own.queryByText(/調べもの/)).toBeNull();
  const headers = shared
    .getAllByRole("columnheader")
    .map((cell) => cell.textContent);
  const sharedCells = Array.from(
    shared.getByRole("row", { name: "調べもの" }).querySelectorAll("th, td"),
    (cell) => cell.textContent,
  );
  expect(sharedCells[headers.indexOf("所有者")]).toBe("bob");
  expect(sharedCells[headers.indexOf("権限")]).toBe("閲覧のみ");
  expect(
    shared.queryByRole("button", {
      name: /名前を変更|を削除|共有を設定/,
    }),
  ).toBeNull();
  expect(shared.getByRole("button", { name: "調べもの を開く" })).toBeTruthy();
  expect(
    shared.getByRole("button", { name: "調べもの を自分用に複製" }),
  ).toBeTruthy();
  expect(own.getByRole("button", { name: "一つ目 の共有を設定" })).toBeTruthy();
});

test("アカウントを持たない起動では共有の設定を出さない", async () => {
  stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSession();
  await openedWith("a");
  expect(screen.queryByRole("button", { name: /共有を設定$/ })).toBeNull();
});

/** 共有先の件数 0 の組を出しているか。 */
function noShares(scope: HTMLElement): boolean {
  return Array.from(scope.querySelectorAll("li")).some(
    (item) => item.textContent === "共有先: 0",
  );
}

test("所有者は共有先を追加し、権限を変え、解除できる。役割を持たない共有先は理由を出す", async () => {
  const { store } = stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSignedIn();
  await openedWith("a");
  fireEvent.click(screen.getByRole("button", { name: "一つ目 の共有を設定" }));
  const region = await screen.findByRole("region", { name: "一つ目 の共有" });
  const shares = within(region);
  await waitFor(() => expect(noShares(region)).toBe(true));
  const login = shares.getByRole("textbox", { name: "共有先のログイン名" });

  fireEvent.change(login, { target: { value: "stranger" } });
  fireEvent.click(shares.getByRole("button", { name: "共有先に追加" }));
  expect(
    (await shares.findByRole("alert")).textContent?.includes(
      "ワークスペースの共有の記録",
    ),
  ).toBe(true);

  fireEvent.change(login, { target: { value: "carol" } });
  fireEvent.change(shares.getByRole("combobox", { name: "権限" }), {
    target: { value: "edit" },
  });
  fireEvent.click(shares.getByRole("button", { name: "共有先に追加" }));
  const carolAccess = await shares.findByRole("combobox", {
    name: "carol の権限",
  });
  expect((carolAccess as HTMLSelectElement).value).toBe("edit");
  expect(store.get("w1")?.shares).toMatchObject([
    { login: "carol", access: "edit" },
  ]);

  fireEvent.change(carolAccess, { target: { value: "view" } });
  await waitFor(() =>
    expect(store.get("w1")?.shares).toMatchObject([
      { login: "carol", access: "view" },
    ]),
  );

  fireEvent.click(
    within(
      within(shares.getByRole("table", { name: "共有先" })).getByRole("row", {
        name: "carol",
      }),
    ).getByRole("button", { name: "共有を解除" }),
  );
  await waitFor(() => expect(noShares(region)).toBe(true));
  expect(store.get("w1")?.shares).toEqual([]);
});

test("共有されたワークスペースを開くと同じ id を開き、複製すると自分の新しいワークスペースを作って開く", async () => {
  const { store, requests } = stubServer([
    stored("w1", "一つ目", stateJson("a")),
    sharedWith("s1", "調べもの", "s", "edit"),
  ]);
  renderSignedIn();
  await openedWith("a");
  fireEvent.click(
    row("共有されたワークスペース", "調べもの").getByRole("button", {
      name: "調べもの を開く",
    }),
  );
  await openedWith("s");
  expect(localStorage.getItem(aliceKey)).toBe("s1");
  expect(requests()).not.toContain("POST /api/v0/workspaces");

  fireEvent.click(
    row("共有されたワークスペース", "調べもの").getByRole("button", {
      name: "調べもの を自分用に複製",
    }),
  );
  await waitFor(() => expect(localStorage.getItem(aliceKey)).toBe("w-new-1"));
  expect(store.get("w-new-1")).toMatchObject({
    name: "調べもの のコピー",
    access: "owner",
    state: { comparedSourceId: "s" },
  });
  expect(
    within(
      screen.getByRole("table", { name: "自分のワークスペース" }),
    ).getByRole("row", { current: true }).textContent,
  ).toContain("調べもの のコピー");
});

test("閲覧だけの共有を開いている間は保存せず、ページを離れても送らない。その場で複製すると画面の状態を保存できる", async () => {
  const { store, requests } = stubServer([
    sharedWith("s1", "調べもの", "s", "view"),
  ]);
  localStorage.setItem(aliceKey, "s1");
  renderSignedIn();
  await openedWith("s");
  expect(
    screen
      .getAllByRole("status")
      .some((node) => node.textContent?.startsWith("閲覧のみ")),
  ).toBe(true);
  expect(pairTexts("保存するには")).toEqual(["保存するには: 自分用に複製"]);
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  await advance(workspaceSaveDelayMs * 2);
  window.dispatchEvent(new Event("pagehide"));
  expect(
    requests().filter(
      (line) => line.startsWith("PUT") || line.endsWith("/changes"),
    ),
  ).toEqual([]);

  fireEvent.click(viewOnlyCopyButton());
  await waitFor(() => expect(localStorage.getItem(aliceKey)).toBe("w-new-1"));
  expect(store.get("w-new-1")).toMatchObject({
    name: "調べもの のコピー",
    state: { comparedSourceId: "c1" },
  });
  expect(store.get("s1")?.revision).toBe(1);
});

test("編集の共有で保存が 403 になると、閲覧のみの扱いに切り替えて理由を出す", async () => {
  const server = stubServer([sharedWith("s1", "調べもの", "s", "edit")]);
  localStorage.setItem(aliceKey, "s1");
  renderSignedIn();
  await openedWith("s");
  const s1 = server.store.get("s1");
  if (s1 !== undefined) server.store.set("s1", { ...s1, access: "view" });
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  await advance(workspaceSaveDelayMs);
  expect(await screen.findByText("保存失敗")).toBeTruthy();
  expect(screen.getByRole("alert").textContent).toContain(
    "所有者による閲覧のみへの変更",
  );
  expect(screen.getByRole("row", { current: true }).textContent).toContain(
    "閲覧のみ",
  );
  const puts = () => server.changes().length;
  const putsAfter403 = puts();
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  await advance(workspaceSaveDelayMs * 2);
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  await advance(workspaceSaveDelayMs * 2);
  window.dispatchEvent(new Event("pagehide"));
  expect(puts() - putsAfter403).toBe(0);
});

test("閲覧のみの共有を開いたときの案内は読み上げの割り込みにしない", async () => {
  stubServer([sharedWith("s1", "調べもの", "s", "view")]);
  localStorage.setItem(aliceKey, "s1");
  renderSignedIn();
  await openedWith("s");
  expect(screen.queryByRole("alert")).toBeNull();
  expect(
    screen
      .getAllByRole("status")
      .some((node) => node.textContent?.startsWith("閲覧のみ")),
  ).toBe(true);
});

test("共有が取り消された後に、画面の今の状態を元のワークスペースを読まずに複製できる", async () => {
  const server = stubServer([sharedWith("s1", "調べもの", "s", "edit")]);
  localStorage.setItem(aliceKey, "s1");
  renderSignedIn();
  await openedWith("s");
  server.store.delete("s1");
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  await advance(workspaceSaveDelayMs);
  await screen.findByText("共有の解除");
  const readsBefore = server
    .requests()
    .filter((line) => line === "GET /api/v0/workspaces/s1").length;
  fireEvent.click(viewOnlyCopyButton());
  await waitFor(() => expect(localStorage.getItem(aliceKey)).toBe("w-new-1"));
  expect(server.store.get("w-new-1")).toMatchObject({
    name: "調べもの のコピー",
    state: { comparedSourceId: "c1" },
  });
  expect(
    server.requests().filter((line) => line === "GET /api/v0/workspaces/s1"),
  ).toHaveLength(readsBefore);
});

test("共有先を読み込む間はそのことを出す", async () => {
  stubServer([stored("w1", "一つ目", stateJson("a"))]);
  renderSignedIn();
  await openedWith("a");
  fireEvent.click(screen.getByRole("button", { name: "一つ目 の共有を設定" }));
  expect(screen.getByText("共有先の読み込み中")).toBeTruthy();
  const region = screen.getByRole("region", { name: "一つ目 の共有" });
  await waitFor(() => expect(noShares(region)).toBe(true));
  expect(screen.queryByText("共有先の読み込み中")).toBeNull();
});

test("新しいワークスペースの名前は server に任せ、続けて作っても同じ名前にならない", async () => {
  const server = stubServer([
    stored("w1", "一つ目", stateJson("a")),
    sharedWith("s1", "調べもの", "s", "view"),
  ]);
  renderSignedIn();
  await openedWith("a");
  const create = screen.getByRole("button", {
    name: "ワークスペースを新規作成",
  });
  fireEvent.click(create);
  fireEvent.click(create);
  await waitFor(() => expect(server.store.get("w-new-2")).toBeDefined());
  expect(
    server.mock.mock.calls
      .filter(
        ([input, init]) =>
          input === "/api/v0/workspaces" && init?.method === "POST",
      )
      .map(([, init]) => JSON.parse(`${init?.body}`)),
  ).toEqual([{ state: {} }, { state: {} }]);
  expect([
    server.store.get("w-new-1")?.name,
    server.store.get("w-new-2")?.name,
  ]).toEqual(["ワークスペース 1", "ワークスペース 2"]);
});

test("共有が取り消されて保存が 404 になると、そのことを出し、自分のワークスペースへ移れる", async () => {
  const server = stubServer([
    sharedWith("s1", "調べもの", "s", "edit"),
    stored("w1", "一つ目", stateJson("a")),
  ]);
  localStorage.setItem(aliceKey, "s1");
  renderSignedIn();
  await openedWith("s");
  server.store.delete("s1");
  fireEvent.click(screen.getByRole("button", { name: "画面を変える" }));
  await advance(workspaceSaveDelayMs);
  expect(await screen.findByText("共有の解除")).toBeTruthy();
  expect(
    screen.queryByRole("table", { name: "共有されたワークスペース" }),
  ).toBeNull();
  fireEvent.click(
    screen.getByRole("button", { name: "自分のワークスペースに移動" }),
  );
  await openedWith("a");
});
