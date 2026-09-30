// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import {
  decodeGraphResponse,
  type SubgraphNode,
} from "@/shared/contracts/graph";
import {
  graphResponseJson,
  processNodeId,
} from "@/testdata/graph/graphResponse";
import { ValueActionsForTest } from "@/testdata/valueActions";
import { SubgraphEdgeList } from "./SubgraphEdgeList";
import { SubgraphNodeList } from "./SubgraphNodeList";

afterEach(cleanup);
const graph = decodeGraphResponse(graphResponseJson(), "graph");
const nodeActions = {
  onOpenDetail: () => {},
  onAddNeighbours: () => {},
  onShowNeighbours: () => {},
  onTraceLineage: () => {},
  onNarrowToTerminal: () => {},
  onAddTerm: () => {},
};
const edgeActions = {
  onSelectEdge: () => {},
  onOpenNodeDetail: () => {},
  onAddTerm: () => {},
};

function nodeList(nodes: SubgraphNode[], onSelect = vi.fn()) {
  return (
    <ValueActionsForTest>
      <SubgraphNodeList
        nodes={nodes}
        selectedNodeId={undefined}
        originNodeIds={[]}
        onSelect={onSelect}
        onExpand={() => {}}
        menuActions={nodeActions}
      />
    </ValueActionsForTest>
  );
}

function tableRows(name: string) {
  return within(screen.getByRole("table", { name }))
    .queryAllByRole("row")
    .slice(1);
}

test("ノードを種類と文字列で絞り、詳細を開き、解除して全行へ戻す", () => {
  const onSelect = vi.fn();
  render(nodeList(graph.nodes, onSelect));
  fireEvent.change(
    screen.getByRole("combobox", { name: "一覧のノードの種類" }),
    {
      target: { value: "process" },
    },
  );
  fireEvent.change(screen.getByRole("searchbox", { name: "ノードの文字列" }), {
    target: { value: "CMD.EXE" },
  });
  const rows = tableRows("グラフのノードの一覧");
  expect(rows).toHaveLength(1);
  const row = rows[0];
  if (row === undefined) throw new Error("missing process row");
  expect(row).toHaveTextContent("cmd.exe");
  fireEvent.click(within(row).getByRole("button", { name: /の詳細を開く$/ }));
  expect(onSelect).toHaveBeenCalledWith(processNodeId);
  expect(
    screen.getByRole("status", { name: "ノードの一致件数" }),
  ).toHaveTextContent(`1 / ${graph.nodes.length}`);
  fireEvent.change(screen.getByRole("searchbox", { name: "ノードの文字列" }), {
    target: { value: "missing.example.test" },
  });
  expect(tableRows("グラフのノードの一覧")).toEqual([]);
  fireEvent.click(
    screen.getByRole("button", { name: "ノードの絞り込みを解除" }),
  );
  expect(tableRows("グラフのノードの一覧")).toHaveLength(graph.nodes.length);
});

test("ノードの同一性の値を探し、応答更新後も選んだ種類と空の結果を表示する", () => {
  const view = render(nodeList(graph.nodes));
  fireEvent.change(screen.getByRole("searchbox", { name: "ノードの文字列" }), {
    target: { value: "{p1}" },
  });
  expect(tableRows("グラフのノードの一覧")[0]).toHaveTextContent("cmd.exe");
  fireEvent.change(
    screen.getByRole("combobox", { name: "一覧のノードの種類" }),
    {
      target: { value: "process" },
    },
  );
  view.rerender(
    nodeList(graph.nodes.filter((node) => node.kind === "terminal")),
  );
  expect(
    screen.getByRole("combobox", { name: "一覧のノードの種類" }),
  ).toHaveValue("process");
  expect(tableRows("グラフのノードの一覧")).toEqual([]);
});

test("エッジを種類と端点の同一性の値で絞り、行の操作と条件の解除を保つ", () => {
  const onSelect = vi.fn();
  const list = (edges = graph.edges) => (
    <ValueActionsForTest>
      <SubgraphEdgeList
        edges={edges}
        nodes={graph.nodes}
        selectedEdgeId={undefined}
        onSelect={onSelect}
        menuActions={edgeActions}
      />
    </ValueActionsForTest>
  );
  const view = render(list());
  fireEvent.change(
    screen.getByRole("combobox", { name: "一覧のエッジの種類" }),
    {
      target: { value: "process_communication" },
    },
  );
  fireEvent.change(screen.getByRole("searchbox", { name: "エッジの文字列" }), {
    target: { value: "{P1}" },
  });
  const rows = tableRows("グラフのエッジの一覧");
  expect(rows).toHaveLength(1);
  const row = rows[0];
  if (row === undefined) throw new Error("missing edge row");
  expect(row).toHaveTextContent("203.0.113.21");
  fireEvent.click(within(row).getByRole("button", { name: /の詳細を表示$/ }));
  expect(onSelect).toHaveBeenCalledWith("e:process_communication:0002");
  view.rerender(list(graph.edges.filter((edge) => edge.kind === "ran_on")));
  expect(
    screen.getByRole("combobox", { name: "一覧のエッジの種類" }),
  ).toHaveValue("process_communication");
  expect(tableRows("グラフのエッジの一覧")).toEqual([]);
  fireEvent.click(
    screen.getByRole("button", { name: "エッジの絞り込みを解除" }),
  );
  expect(tableRows("グラフのエッジの一覧")[0]).toHaveTextContent("HOST-C");
});

test("エッジの作り方と端点の種類を文字列で探せる", () => {
  render(
    <ValueActionsForTest>
      <SubgraphEdgeList
        edges={graph.edges.map((edge) => ({
          ...edge,
          state: edge.kind === "ran_on" ? "candidate" : "observed",
        }))}
        nodes={graph.nodes}
        selectedEdgeId={undefined}
        onSelect={() => {}}
        menuActions={edgeActions}
      />
    </ValueActionsForTest>,
  );
  const input = screen.getByRole("searchbox", { name: "エッジの文字列" });
  fireEvent.change(input, { target: { value: "推定" } });
  expect(tableRows("グラフのエッジの一覧")).toHaveLength(1);
  expect(tableRows("グラフのエッジの一覧")[0]).toHaveTextContent("HOST-C");
  fireEvent.change(input, { target: { value: "IP アドレス" } });
  expect(tableRows("グラフのエッジの一覧")).toHaveLength(1);
  expect(tableRows("グラフのエッジの一覧")[0]).toHaveTextContent(
    "203.0.113.21",
  );
});

test("最初に表示する範囲より後のノードも文字列で見つける", () => {
  const base = graph.nodes.find((node) => node.kind === "terminal");
  if (base === undefined) throw new Error("missing terminal fixture");
  const nodes = Array.from({ length: 1100 }, (_, index) => ({
    ...base,
    id: `host-${index}`,
    label: { ...base.label, rawText: `host-${index}.example.test` },
  }));
  render(nodeList(nodes));
  expect(
    screen.queryByText("host-1099.example.test", { exact: true }),
  ).not.toBeInTheDocument();
  fireEvent.change(screen.getByRole("searchbox", { name: "ノードの文字列" }), {
    target: { value: "host-1099.example.test" },
  });
  expect(tableRows("グラフのノードの一覧")[0]).toHaveTextContent(
    "host-1099.example.test",
  );
});
