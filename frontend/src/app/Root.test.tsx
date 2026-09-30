// @vitest-environment jsdom
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { stagesPollIntervalMs } from "@/features/investigationStages/useInvestigationStages";
import { SignedInContext } from "@/shared/ui/SignedInAccount";
import { jsonResponse } from "@/testdata/http";
import { accountLabel, signedInAlice } from "@/testdata/session";
import { sourcesResponseJson } from "@/testdata/sources/sourcesResponse";
import {
  loadingCompletedStagesJson,
  notStartedStagesJson,
  processingCompletedStagesJson,
  processingRunningStagesJson,
} from "@/testdata/stages/stagesResponse";
import { Root } from "./Root";

// 調査の画面は処理を終えた後の応答を読む。段階に合う画面を出すことを確かめる本 test は、
// 調査の画面を stub に置き換える。調査の画面の組み立ては App.test.tsx が確かめる。
vi.mock("./App", () => ({
  App: () => <p>調査の画面</p>,
}));
// ワークスペースを開く手順は WorkspaceSession.test.tsx が確かめる。
vi.mock("./WorkspaceSession", () => ({
  WorkspaceSession: ({ children }: { children: (props: object) => unknown }) =>
    children({}),
}));

/** `/api/v0/stages` の応答を並びの順で返し、収集元の一覧の要求に一覧を返す。 */
function stubFetch(stages: unknown[]) {
  let served = 0;
  const mock = vi.fn(async (input: string, _init?: RequestInit) => {
    if (input === "/api/v0/sources") {
      return jsonResponse(200, sourcesResponseJson());
    }
    if (input === "/api/v0/stages") {
      const body = stages[Math.min(served, stages.length - 1)];
      served += 1;
      return jsonResponse(200, body);
    }
    throw new Error(`unexpected request: ${input}`);
  });
  vi.stubGlobal("fetch", mock);
  return mock;
}

function requestedPaths(mock: ReturnType<typeof stubFetch>): string[] {
  return mock.mock.calls.map(([input]) => input);
}

async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

test("処理を終えた server では、調査の画面を出す", async () => {
  const mock = stubFetch([processingCompletedStagesJson()]);

  render(<Root />);
  await advance(0);

  expect(screen.getByText("調査の画面")).toBeTruthy();
  expect(screen.queryByRole("heading", { name: "調査の段階" })).toBeNull();
  expect(requestedPaths(mock)).not.toContain("/api/v0/sources");
});

test("読み込みを始めていない server では、段階の状態を出し、収集元の一覧を取得しない", async () => {
  const mock = stubFetch([notStartedStagesJson()]);

  render(<Root />);
  await advance(0);

  expect(screen.getByRole("heading", { name: "調査の段階" })).toBeTruthy();
  expect(screen.queryByText("調査の画面")).toBeNull();
  expect(screen.queryByRole("region", { name: "収集元の一覧" })).toBeNull();
  expect(requestedPaths(mock)).not.toContain("/api/v0/sources");
});

test("読み込みを終え処理を終える前は、段階の状態と収集元の一覧を並べる", async () => {
  const mock = stubFetch([loadingCompletedStagesJson()]);

  render(<Root />);
  await advance(0);

  expect(screen.getByRole("heading", { name: "調査の段階" })).toBeTruthy();
  expect(screen.getByRole("region", { name: "収集元の一覧" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "access.log" })).toBeTruthy();
  expect(requestedPaths(mock)).toContain("/api/v0/sources");
  expect(screen.queryByText("調査の画面")).toBeNull();
});

test("ログインしているときは、段階の画面のヘッダーにログインした利用者とログアウトを出す", async () => {
  stubFetch([notStartedStagesJson()]);

  render(
    <SignedInContext.Provider value={signedInAlice}>
      <Root />
    </SignedInContext.Provider>,
  );
  await advance(0);

  expect(
    screen.getByText(
      (_, element) =>
        element?.tagName === "SPAN" &&
        element.textContent === `ログイン中: ${accountLabel}`,
    ),
  ).toBeTruthy();
  expect(screen.getByRole("button", { name: "ログアウト" })).toBeTruthy();
});

test("実行中の処理を取り直して処理を終えたら、調査の画面へ切り替える", async () => {
  stubFetch([processingRunningStagesJson(), processingCompletedStagesJson()]);

  render(<Root />);
  await advance(0);
  expect(screen.queryByText("調査の画面")).toBeNull();

  await advance(stagesPollIntervalMs);

  expect(screen.getByText("調査の画面")).toBeTruthy();
});
