// @vitest-environment jsdom
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { addCondition } from "@/testdata/conditionInput";
import { graphResponseJson } from "@/testdata/graph/graphResponse";
import { jsonResponse } from "@/testdata/http";
import {
  type SearchExpressionErrorJson,
  searchExpressionErrorResponseJson,
} from "@/testdata/searchExpression/searchExpressionErrorResponse";
import {
  type FetchMock,
  figureRequests,
  findNodeList,
  getPair,
  initialFigureRequest,
  lastFigureRequest,
  limitQuery,
  matchConditionQuery,
  objectViewQuery,
  renderGraphExplore,
  stubFetch,
} from "./graphExploreTestHarness";

// WebGL の renderer は jsdom で動かない。本 test は図を stub に置き換え、検索式の要求と
// 条件の一覧と誤りの表示を確かめる。
vi.mock("./SubgraphCanvas", () => ({
  SubgraphCanvas: () => <p>部分グラフの図</p>,
}));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

/** 図と一覧を出し、事象の種別の選択肢を読み込んだ画面を描き、fetch の mock を返す。 */
async function renderLoaded(mock: FetchMock = stubFetch()) {
  renderGraphExplore();
  await findNodeList();
  await waitFor(() =>
    expect(
      mock.mock.calls.some((call) =>
        String(call[0]).startsWith("/api/v0/event-kinds"),
      ),
    ).toBe(true),
  );
  return mock;
}

/**
 * 図の要求が検索式 `rejected` を含むときだけ、検索式の誤りを返す。他の要求には harness の
 * 既定の応答を返す。
 */
function stubRejecting(rejected: string, error: SearchExpressionErrorJson) {
  const mock = stubFetch();
  const served = mock.getMockImplementation();
  mock.mockImplementation(async (input: string, init?: RequestInit) => {
    const url = new URL(input, "http://localhost");
    if (
      url.pathname === "/api/v0/graph" &&
      url.searchParams.get("searchExpression") === rejected
    ) {
      return jsonResponse(400, searchExpressionErrorResponseJson(error));
    }
    return served?.(input, init) ?? jsonResponse(500, {});
  });
  return mock;
}

function applyExpression(text: string) {
  addCondition("検索式", { 検索式: text });
}

/** 図の要求が含む検索式。載せていない要求は null を返す。 */
function requestedExpression(path: string | undefined): string | null {
  return new URL(path ?? "", "http://localhost").searchParams.get(
    "searchExpression",
  );
}

test("検索式を適用すると、図の要求に載せて対象を図に出し、条件の一覧から外すと要求から外す", async () => {
  const mock = await renderLoaded(
    stubFetch({ ...graphResponseJson(), searchExpression: "LogonType == 3" }),
  );

  applyExpression("  LogonType == 3 ");

  // 前後の空白を外した式を載せ、端末の粒度をやめて対象を出す。
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}&depth=1` +
        `&searchExpression=LogonType+%3D%3D+3${matchConditionQuery}`,
    ),
  );
  expect(requestedExpression(lastFigureRequest(mock))).toBe("LogonType == 3");
  await waitFor(() =>
    getPair(
      "検索式: LogonType == 3",
      document.querySelector(".result-pane") as HTMLElement,
    ),
  );

  fireEvent.click(
    screen.getByRole("button", { name: "検索式 LogonType == 3 を削除" }),
  );
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(initialFigureRequest),
  );
});

test("図の要求が検索式の誤りを返すと、欄の近くに理由と位置と範囲を出し、直した式を適用すると消す", async () => {
  const mock = await renderLoaded(
    stubRejecting("LogonType = 3", {
      reason: "unexpected_character",
      offset: 10,
      length: 1,
    }),
  );

  applyExpression("LogonType = 3");

  const region = await screen.findByRole("region", { name: "検索式の誤り" });
  expect(getPair("検索式の誤り: 条件を始められない文字", region)).toBeTruthy();
  expect(getPair("誤りの位置: 11 文字目", region)).toBeTruthy();
  expect(region.querySelector("mark")?.textContent).toBe("=");
  // 取得の失敗の表示も、読めなかった理由を書く。
  expect(screen.getAllByText("条件を始められない文字")).not.toEqual([]);

  applyExpression("LogonType == 3");

  await waitFor(() =>
    expect(requestedExpression(lastFigureRequest(mock))).toBe("LogonType == 3"),
  );
  await findNodeList();
  expect(screen.queryByRole("region", { name: "検索式の誤り" })).toBeNull();
});

test("関係先を出している間も、背景の検索の要求が返した検索式の誤りを出す", async () => {
  const processOrigin = "nodeId=n%3Aprocess%3A8ab3";
  const mock = await renderLoaded(
    stubRejecting("(LogonType == 3", {
      reason: "unclosed_parenthesis",
      offset: 15,
      length: 0,
    }),
  );
  // 関係先の起点を足してから検索の結果へ戻り、起点を持ったまま式を適用する。
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の隣接ノードを追加",
    }),
  );
  await waitFor(() => expect(lastFigureRequest(mock)).toContain(processOrigin));
  fireEvent.click(screen.getByRole("button", { name: "検索の結果に戻る" }));
  applyExpression("(LogonType == 3");
  await screen.findByRole("region", { name: "検索式の誤り" });

  const beforeRestore = figureRequests(mock).length;
  fireEvent.click(
    screen.getByRole("button", { name: "隣接ノードの表示に戻る" }),
  );

  // 関係先の要求は検索式を含まず、背景の要求が式を含む。
  await waitFor(() => {
    const restored = figureRequests(mock).slice(beforeRestore);
    expect(
      restored.some(
        (path) =>
          path.includes(processOrigin) && requestedExpression(path) === null,
      ),
    ).toBe(true);
    expect(
      restored.some((path) => requestedExpression(path) === "(LogonType == 3"),
    ).toBe(true);
  });
  const region = await screen.findByRole("region", { name: "検索式の誤り" });
  expect(getPair("誤りの位置: 式の末尾", region)).toBeTruthy();
  expect(region.querySelector("mark.search-expression-point")).not.toBeNull();
});
