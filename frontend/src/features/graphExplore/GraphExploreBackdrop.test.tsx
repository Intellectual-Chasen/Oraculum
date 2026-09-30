// @vitest-environment jsdom
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import {
  emptyGraphResponseJson,
  graphResponseJson,
  ipNodeId,
  limitedGraphResponseJson,
  overLimitGraphResponseJson,
  processNodeId,
  terminalNodeId,
} from "@/testdata/graph/graphResponse";
import { jsonResponse } from "@/testdata/http";
import {
  figureRequests,
  findNodeList,
  findPair,
  GraphExploreHarness,
  stubFetch,
  terminalListPrefix,
} from "./graphExploreTestHarness";
import type { GraphHighlight } from "./SubgraphCanvas";
import type { SubgraphDrawing } from "./subgraph";

// WebGL の renderer は jsdom で動かない。図に渡した値を記録する stub に置き換える。
const { canvasSpy } = vi.hoisted(() => ({ canvasSpy: vi.fn() }));

vi.mock("./SubgraphCanvas", () => ({
  SubgraphCanvas: (props: {
    drawing: SubgraphDrawing;
    highlight: GraphHighlight | undefined;
  }) => {
    canvasSpy(props);
    return <p>図に渡したノードは {props.drawing.points.length} 件です。</p>;
  },
}));

afterEach(() => {
  cleanup();
  canvasSpy.mockClear();
  vi.unstubAllGlobals();
});

/** 関係先の応答。端末とプロセスと、その間の実行のエッジだけを含む。 */
function neighboursJson() {
  const graph = graphResponseJson();
  const edges = graph.edges.filter(
    (edge) => edge.sourceNodeId !== ipNodeId && edge.targetNodeId !== ipNodeId,
  );
  return {
    ...graph,
    subgraphNodeCount: 2,
    nodes: graph.nodes.filter((node) => node.id !== ipNodeId),
    edges,
    edgeCount: edges.length,
  };
}

/** 関係先を出す操作の後に送った検索の要求 (背景) には backdrop を返す。 */
let exploring = false;

/**
 * 関係先の要求 (起点の nodeId を含む) に neighboursJson を返す。関係先を出した後の検索の
 * 要求 (背景) には backdrop を返す。
 */
function stubNeighbours(backdrop: unknown) {
  exploring = false;
  const mock = stubFetch();
  const served = mock.getMockImplementation();
  mock.mockImplementation(async (input: string, init?: RequestInit) => {
    if (input.startsWith("/api/v0/graph?") && input.includes("nodeId=")) {
      return jsonResponse(200, neighboursJson());
    }
    // 関係先を出している間の、検索の条件の図 (背景) の要求。端末の選択肢の要求を除く。
    if (
      exploring &&
      input.startsWith("/api/v0/graph?") &&
      !input.startsWith(terminalListPrefix)
    ) {
      return jsonResponse(200, backdrop);
    }
    return served?.(input, init) ?? jsonResponse(500, {});
  });
  return mock;
}

function renderHarness(highlight?: GraphHighlight) {
  return render(
    <GraphExploreHarness
      onSelectRecord={() => {}}
      selectedEdgeId={undefined}
      onSelectEdge={() => {}}
      assertions={() => null}
      highlight={highlight}
    />,
  );
}

async function showNeighbours() {
  await findNodeList();
  exploring = true;
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の隣接ノードを追加",
    }),
  );
}

type CanvasProps = {
  drawing: SubgraphDrawing;
  highlight: GraphHighlight | undefined;
};

function lastCanvas(): CanvasProps {
  return canvasSpy.mock.calls.at(-1)?.[0] as CanvasProps;
}

test("検索の結果を出す間は背景を取らず、関係先を出すと検索の条件で背景を取り、背景と合わせて描く", async () => {
  const mock = stubNeighbours(limitedGraphResponseJson());
  renderHarness();
  await findNodeList();
  const beforeExploring = figureRequests(mock).length;
  expect(figureRequests(mock).every((path) => !path.includes("nodeId="))).toBe(
    true,
  );
  expect(lastCanvas().highlight).toBeUndefined();

  await showNeighbours();
  // 背景だけが持つ IP アドレスのノードも図に渡す。
  await waitFor(() =>
    expect(
      lastCanvas()
        .drawing.points.map((point) => point.id)
        .sort(),
    ).toEqual([ipNodeId, processNodeId, terminalNodeId].sort()),
  );
  const afterExploring = figureRequests(mock).slice(beforeExploring);
  expect(afterExploring.some((path) => path.includes("nodeId="))).toBe(true);
  expect(afterExploring.some((path) => !path.includes("nodeId="))).toBe(true);
});

test("検索の結果から関係先を足した後、関係先だけを表示すると背景のノードとエッジを除く", async () => {
  stubNeighbours(limitedGraphResponseJson());
  renderHarness();
  await showNeighbours();
  await waitFor(() => expect(lastCanvas().drawing.points).toHaveLength(3));
  expect(lastCanvas().highlight).toBeUndefined();

  fireEvent.click(
    await screen.findByRole("button", { name: "隣接ノードだけを表示" }),
  );
  await waitFor(() =>
    expect(
      lastCanvas()
        .drawing.points.map((point) => point.id)
        .sort(),
    ).toEqual([processNodeId, terminalNodeId].sort()),
  );
  expect(
    lastCanvas().drawing.links.every(
      (link) =>
        link.sourceNodeId !== ipNodeId && link.targetNodeId !== ipNodeId,
    ),
  ).toBe(true);
  expect(lastCanvas().highlight).toBeUndefined();
});

test("親子の連鎖の表示から関係先を足すと、背景の検索の結果を薄くして関係先を強調する", async () => {
  stubNeighbours(limitedGraphResponseJson());
  renderHarness();
  await findNodeList();
  exploring = true;
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );
  fireEvent.click(
    await screen.findByRole("button", { name: "プロセスの親子関係を表示" }),
  );
  await findPair("表示中: プロセスの親子関係");
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の隣接ノードを追加",
    }),
  );
  await screen.findByText("隣接ノードの表示元");
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  expect(
    lastCanvas()
      .drawing.points.map((point) => point.id)
      .sort(),
  ).toEqual([ipNodeId, processNodeId, terminalNodeId].sort());
  await waitFor(() =>
    expect([...(lastCanvas().highlight?.nodeIds ?? [])].sort()).toEqual(
      [processNodeId, terminalNodeId].sort(),
    ),
  );
});

test("背景の応答が 0 件か上限を超えるときは、背景を描かずに関係先だけを描く", async () => {
  for (const backdrop of [
    emptyGraphResponseJson(),
    overLimitGraphResponseJson(5000),
  ]) {
    stubNeighbours(backdrop);
    renderHarness();
    await showNeighbours();
    await waitFor(() =>
      expect(
        lastCanvas()
          .drawing.points.map((point) => point.id)
          .sort(),
      ).toEqual([processNodeId, terminalNodeId].sort()),
    );
    expect(lastCanvas().highlight).toBeUndefined();
    cleanup();
    canvasSpy.mockClear();
  }
});

test("開いたレコードの強調を渡しているときは、関係先の強調よりそちらを図に渡す", async () => {
  stubNeighbours(limitedGraphResponseJson());
  const recordHighlight: GraphHighlight = {
    nodeIds: new Set([ipNodeId]),
    edgeIds: new Set(),
  };
  renderHarness(recordHighlight);
  await showNeighbours();
  await waitFor(() => expect(lastCanvas().drawing.points).toHaveLength(3));
  expect(lastCanvas().highlight).toBe(recordHighlight);
});
