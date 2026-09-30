// @vitest-environment jsdom
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import {
  initialFigureRequest,
  renderGraphExplore,
  sentRequest,
  stubFetch,
  withPriority,
} from "./graphExploreTestHarness";

// WebGL の renderer は jsdom で動かない。本 test は図を stub に置き換え、目安の欄の取得を確かめる。
vi.mock("./SubgraphCanvas", () => ({
  SubgraphCanvas: () => <p>部分グラフの図</p>,
}));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

test("Priority は図と同じ条件で次数の目安を取得する", async () => {
  const mock = stubFetch();
  renderGraphExplore(undefined, withPriority);
  const orderRequests = () =>
    mock.mock.calls
      .map(([input]) => input)
      .filter((input) => input.startsWith("/api/v0/investigation-order/"));
  await screen.findByText("部分グラフの図");
  await screen.findByRole("table", { name: "プロセス 3" });
  const requestOf = (method: string) =>
    sentRequest(initialFigureRequest).replace(
      "/api/v0/graph?",
      `/api/v0/investigation-order/${method}?`,
    );
  expect(orderRequests()).toEqual([requestOf("degree")]);

  fireEvent.change(screen.getByRole("combobox", { name: "並べ方" }), {
    target: { value: "sigma" },
  });
  await waitFor(() =>
    expect(orderRequests()).toEqual([requestOf("degree"), requestOf("sigma")]),
  );

  fireEvent.change(
    screen.getByRole("combobox", { name: "数える Sigma ルールのレベル" }),
    { target: { value: "medium" } },
  );
  await waitFor(() =>
    expect(orderRequests()).toEqual([
      requestOf("degree"),
      requestOf("sigma"),
      `${requestOf("sigma")}&sigmaMinLevel=medium`,
    ]),
  );
});
