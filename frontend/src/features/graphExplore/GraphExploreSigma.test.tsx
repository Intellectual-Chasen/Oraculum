// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { openDetails } from "@/testdata/details";
import { ValueActionsForTest } from "@/testdata/valueActions";
import {
  GraphExploreHarness,
  initialFigureRequest,
  renderGraphExplore,
  sentRequest,
  stubFetch,
} from "./graphExploreTestHarness";
import type { GraphHighlight } from "./SubgraphCanvas";

// WebGL の renderer は jsdom で動かない。本 test は図を stub に置き換え、Sigma の候補の tab の取得を確かめる。
vi.mock("./SubgraphCanvas", () => ({
  SubgraphCanvas: () => <p>部分グラフの図</p>,
}));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

/** Detection の中の tab を選ぶ。 */
function selectDetectionTab(name: string | RegExp) {
  fireEvent.click(screen.getByRole("tab", { name }));
}

test("Sigma の候補は tab を選んだときに図と同じ条件で取得し、別の tab を選んでも中身を保つ", async () => {
  const mock = stubFetch();
  renderGraphExplore();
  const sigmaRequests = () =>
    mock.mock.calls
      .map(([input]) => input)
      .filter((input) => input.startsWith("/api/v0/sigma-rule-candidates"));
  await screen.findByText("部分グラフの図");
  expect(sigmaRequests()).toHaveLength(0);

  selectDetectionTab("Sigma");
  await screen.findByText("評価したルール:");
  expect(sigmaRequests()).toEqual([
    sentRequest(initialFigureRequest).replace(
      "/api/v0/graph?",
      "/api/v0/sigma-rule-candidates?",
    ),
  ]);

  // 選んでいない tab の中身は描き続け、操作を受けない状態 (inert) にして隠す。
  const sigmaInert = () =>
    screen.getByText("評価したルール:").closest("[data-inert='true']") !== null;
  selectDetectionTab(/^ATT&CK/);
  await waitFor(() => expect(sigmaInert()).toBe(true));
  selectDetectionTab("Sigma");
  await waitFor(() => expect(sigmaInert()).toBe(false));
  expect(sigmaRequests()).toHaveLength(1);
});

test("ルールの強調は、開いたレコードの強調を取得し直しても保ち、レコードを開き直すと開いたレコードの強調に戻る", async () => {
  stubFetch();
  const view = (highlight: GraphHighlight, opening: object) => (
    <ValueActionsForTest onSelectRecord={() => {}}>
      <GraphExploreHarness
        onSelectRecord={() => {}}
        selectedEdgeId={undefined}
        onSelectEdge={() => {}}
        assertions={() => null}
        highlight={highlight}
        highlightOpening={opening}
      />
    </ValueActionsForTest>
  );
  const recordHighlight = () => ({
    nodeIds: new Set<string>(),
    edgeIds: new Set<string>(),
  });
  const opened = {};
  const { rerender } = render(view(recordHighlight(), opened));
  await screen.findByText("部分グラフの図");
  selectDetectionTab("Sigma");
  await screen.findByText("評価したルール:");
  openDetails(/Synthetic Process Rule/);
  fireEvent.click(screen.getByRole("button", { name: "Graph で強調" }));
  expect(
    screen.getByRole("button", { name: "Graph の強調を解除" }),
  ).toBeDefined();

  // 端末の割当などで、同じレコードの強調を取得し直した。
  rerender(view(recordHighlight(), opened));
  expect(
    screen.getByRole("button", { name: "Graph の強調を解除" }),
  ).toBeDefined();

  // 別のレコードを開いた。
  rerender(view(recordHighlight(), {}));
  expect(screen.getByRole("button", { name: "Graph で強調" })).toBeDefined();
});
