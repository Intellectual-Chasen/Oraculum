// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { chooseConditionValue } from "@/testdata/conditionInput";
import {
  ipNodeId,
  limitedGraphResponseJson,
} from "@/testdata/graph/graphResponse";
import { jsonResponse } from "@/testdata/http";
import {
  figureRequests,
  GraphExploreHarness,
  stubFetch,
  terminalListResponseJson,
} from "./graphExploreTestHarness";

// WebGL の renderer は jsdom で動かない。
vi.mock("./SubgraphCanvas", () => ({ SubgraphCanvas: () => null }));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

test("外のビューが渡したノードを起点に関係先を取り、そのノードを選んだと知らせる", async () => {
  const mock = stubFetch();
  const onChangeSelectedNode = vi.fn();
  const props = {
    onSelectRecord: vi.fn(),
    selectedEdgeId: undefined,
    onSelectEdge: vi.fn(),
    onChangeSelectedNode,
    assertions: () => null,
  };
  const { rerender } = render(<GraphExploreHarness {...props} />);
  await waitFor(() => expect(figureRequests(mock).length).toBeGreaterThan(0));

  rerender(
    <GraphExploreHarness
      {...props}
      focusRequest={{ node: { id: ipNodeId, label: "192.0.2.10" } }}
    />,
  );

  await waitFor(() =>
    expect(
      figureRequests(mock).some((path) =>
        path.includes(`nodeId=${encodeURIComponent(ipNodeId)}`),
      ),
    ).toBe(true),
  );
  // 応答がまだそのノードを持たない間は外のビューが渡した表示名を、読んだ後は応答の表示名を
  // 知らせる。識別子を表示名として知らせない。
  await waitFor(() =>
    expect(onChangeSelectedNode).toHaveBeenLastCalledWith({
      id: ipNodeId,
      label: "203.0.113.21",
    }),
  );
  expect(onChangeSelectedNode).not.toHaveBeenCalledWith({
    id: ipNodeId,
    label: ipNodeId,
  });
  expect(screen.queryByRole("alert")).toBeNull();
});

test("外のビューが渡したノードが今の図の応答に無くても、関係先の応答が届くまで選択を外さない", async () => {
  // 最初の図は端末だけを運び、渡したノードを持たない。関係先の要求 (nodeId=) の応答だけが持つ。
  const mock = stubFetch(terminalListResponseJson());
  const fallback = mock.getMockImplementation();
  mock.mockImplementation(async (input: string, init?: RequestInit) => {
    if (input.startsWith("/api/v0/graph?") && input.includes("nodeId=")) {
      return jsonResponse(200, limitedGraphResponseJson());
    }
    if (fallback === undefined) throw new Error("no fallback implementation");
    return fallback(input, init);
  });
  const onChangeSelectedNode = vi.fn();
  const props = {
    onSelectRecord: vi.fn(),
    selectedEdgeId: undefined,
    onSelectEdge: vi.fn(),
    onChangeSelectedNode,
    assertions: () => null,
  };
  const { rerender } = render(<GraphExploreHarness {...props} />);
  await waitFor(() => expect(figureRequests(mock).length).toBeGreaterThan(0));
  await screen.findByText("一致ノード:");

  rerender(
    <GraphExploreHarness
      {...props}
      focusRequest={{ node: { id: ipNodeId, label: "192.0.2.10" } }}
    />,
  );

  await waitFor(() =>
    expect(onChangeSelectedNode).toHaveBeenLastCalledWith({
      id: ipNodeId,
      label: "203.0.113.21",
    }),
  );
  // 渡したノードを知らせた後に、選択を外したと知らせない。
  const calls = onChangeSelectedNode.mock.calls.map((call) => call[0]);
  const firstFocused = calls.findIndex((node) => node?.id === ipNodeId);
  expect(firstFocused).toBeGreaterThanOrEqual(0);
  expect(calls.slice(firstFocused)).not.toContain(undefined);
});

test("表の値から選んだノードが今の図の応答に無いときは、そのノードを起点に関係先を取り、選択を外さない", async () => {
  const mock = stubFetch(terminalListResponseJson());
  const fallback = mock.getMockImplementation();
  mock.mockImplementation(async (input: string, init?: RequestInit) => {
    if (input.startsWith("/api/v0/graph?") && input.includes("nodeId=")) {
      return jsonResponse(200, limitedGraphResponseJson());
    }
    if (fallback === undefined) throw new Error("no fallback implementation");
    return fallback(input, init);
  });
  const onChangeSelectedNode = vi.fn();
  const props = {
    onSelectRecord: vi.fn(),
    selectedEdgeId: undefined,
    onSelectEdge: vi.fn(),
    onChangeSelectedNode,
    assertions: () => null,
  };
  const { rerender } = render(<GraphExploreHarness {...props} />);
  await screen.findByText("一致ノード:");

  rerender(
    <GraphExploreHarness
      {...props}
      selectRequest={{ node: { id: ipNodeId, label: "192.0.2.10" } }}
    />,
  );

  await waitFor(() =>
    expect(
      figureRequests(mock).some((path) =>
        path.includes(`nodeId=${encodeURIComponent(ipNodeId)}`),
      ),
    ).toBe(true),
  );
  await waitFor(() =>
    expect(onChangeSelectedNode).toHaveBeenLastCalledWith({
      id: ipNodeId,
      label: "203.0.113.21",
    }),
  );
  const calls = onChangeSelectedNode.mock.calls.map((call) => call[0]);
  const firstSelected = calls.findIndex((node) => node?.id === ipNodeId);
  expect(calls.slice(firstSelected)).not.toContain(undefined);
});

test("表の値から選んだノードが今の図の応答にあるときは、図に出す対象を変えずに選ぶ", async () => {
  const mock = stubFetch(limitedGraphResponseJson());
  const onChangeSelectedNode = vi.fn();
  const props = {
    onSelectRecord: vi.fn(),
    selectedEdgeId: undefined,
    onSelectEdge: vi.fn(),
    onChangeSelectedNode,
    assertions: () => null,
  };
  const { rerender } = render(<GraphExploreHarness {...props} />);
  await screen.findByText("一致ノード:");
  const requested = figureRequests(mock).length;

  rerender(
    <GraphExploreHarness
      {...props}
      selectRequest={{ node: { id: ipNodeId, label: "192.0.2.10" } }}
    />,
  );

  await waitFor(() =>
    expect(onChangeSelectedNode).toHaveBeenLastCalledWith({
      id: ipNodeId,
      label: "203.0.113.21",
    }),
  );
  expect(figureRequests(mock).slice(requested)).toEqual([]);
});

test("保存した状態で開いた選択は、読み込んだ図に無くても外さず、外したと知らせない", async () => {
  const mock = stubFetch();
  const onChangeState = vi.fn();
  const absent = { id: "n:ip:absent", label: "198.51.100.7" };
  render(
    <GraphExploreHarness
      onSelectRecord={vi.fn()}
      selectedEdgeId={undefined}
      onSelectEdge={vi.fn()}
      assertions={() => null}
      onChangeState={onChangeState}
      stateRequest={{
        state: {
          criteria: { depth: 1 },
          view: { kind: "auto" },
          drawLimit: "all",
          selected: { kind: "node", node: absent },
          mergeSameAccount: false,
        },
      }}
    />,
  );
  await waitFor(() => expect(figureRequests(mock).length).toBeGreaterThan(0));
  await screen.findByText("一致ノード:");
  await new Promise((resolve) => setTimeout(resolve, 0));
  expect(onChangeState.mock.lastCall?.[0].selected).toEqual({
    kind: "node",
    node: absent,
  });
  // 最初の応答を読んだ後にエッジの種類を変えると、新しい図に無い選択を外す。
  const requested = figureRequests(mock).length;
  chooseConditionValue("エッジの種類", "ファイルの操作");
  await waitFor(() =>
    expect(figureRequests(mock).length).toBeGreaterThan(requested),
  );
  await waitFor(() =>
    expect(onChangeState.mock.lastCall?.[0].selected).toBeUndefined(),
  );
});
