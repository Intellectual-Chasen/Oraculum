// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, test, vi } from "vitest";
import type { GraphEdge, SubgraphNode } from "@/shared/contracts/graph";
import type { EdgeMenuActions } from "./edgeMenu";
import {
  findEdgeList,
  findNodeList,
  GraphExploreHarness,
  stubFetch,
} from "./graphExploreTestHarness";
import { nodeLabelName } from "./NodeLabelView";
import { NodeValueLink } from "./NodeValueLink";
import { SubgraphEdgeList } from "./SubgraphEdgeList";
import type { NodeLabel } from "./subgraph";

// 一覧の行が描かれた回数を、行が呼ぶ表示名の部品と関数の呼び出しで数える。
// nodeLabelName を直に呼ぶのはノードの一覧の行 (操作の button の名前) だけである。表の値の
// 部品は表示名の文字列だけを描く stub にし、エッジの一覧の行は端点ごとに NodeValueLink を描く。
vi.mock("./NodeLabelView", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./NodeLabelView")>();
  return { ...actual, nodeLabelName: vi.fn(actual.nodeLabelName) };
});
vi.mock("./NodeValueLink", () => ({
  NodeValueLink: vi.fn(({ node }: { node: SubgraphNode }) => (
    <span>{node.label.rawText}</span>
  )),
  EdgeValueLink: ({ edge }: { edge: GraphEdge }) => <span>{edge.kind}</span>,
}));

// WebGL の renderer は jsdom で動かない。本 test は図を読まない。
vi.mock("./SubgraphCanvas", () => ({ SubgraphCanvas: () => null }));

afterEach(() => {
  cleanup();
  vi.mocked(NodeValueLink).mockClear();
  vi.mocked(nodeLabelName).mockClear();
  vi.unstubAllGlobals();
});

/** 表示名の文字列を返す。文字列を持たない表示名は、無い理由の文を返す。 */
function labelText(label: NodeLabel): string {
  return "text" in label.value ? label.value.text : label.value.absence;
}

/** エッジの一覧の行が描いた端点の表示名の文字列を、重複を除いて返す。 */
function renderedLabels() {
  return new Set(
    vi.mocked(NodeValueLink).mock.calls.map(([{ node }]) => node.label.rawText),
  );
}

/** ノードの一覧の行が button の名前に使った表示名の文字列を、重複を除いて返す。 */
function namedLabels() {
  return new Set(
    vi.mocked(nodeLabelName).mock.calls.map(([label]) => labelText(label)),
  );
}

function hostNode(name: string): SubgraphNode {
  return {
    id: `n:host:${name}`,
    kind: "terminal",
    keyForm: "terminal_id",
    identity: [{ semantic: "terminal.id", value: `${name}-TMID` }],
    label: { rawText: `${name}.example.test`, valueState: "present" },
    observation: "observed",
    creationRecord: "item_absent",
    selection: "matched",
  };
}

function hostEdge(id: string, source: string, target: string): GraphEdge {
  return {
    id,
    kind: "ran_on",
    state: "observed",
    sourceNodeId: `n:host:${source}`,
    targetNodeId: `n:host:${target}`,
    evidenceCount: 1,
  };
}

const edgeListNodes = [
  "alpha",
  "bravo",
  "charlie",
  "delta",
  "echo",
  "foxtrot",
].map(hostNode);
const edgeListEdges = [
  hostEdge("e:first", "alpha", "bravo"),
  hostEdge("e:second", "charlie", "delta"),
  hostEdge("e:third", "echo", "foxtrot"),
];
const onSelectEdge = () => {};
/** 行のメニューの操作。本 test はメニューを開かず、描画をまたいで同じ組を渡す。 */
const edgeMenuActions: EdgeMenuActions = {
  onSelectEdge: () => {},
  onOpenNodeDetail: () => {},
  onAddTerm: () => {},
};

/** 端点の表示名 label を持つエッジの行の、詳細を出す button を返す。 */
function edgeDetailButton(label: RegExp): HTMLElement {
  const table = screen.getByRole("table", { name: "グラフのエッジの一覧" });
  const row = within(table).getByRole("row", { name: label });
  return within(row).getByRole("button", { name: / の詳細を表示$/ });
}

test("関係を選び直すとき、エッジの一覧は選んだ行と選びを外した行だけを描き直す", () => {
  const view = render(
    <SubgraphEdgeList
      edges={edgeListEdges}
      nodes={edgeListNodes}
      selectedEdgeId="e:first"
      onSelect={onSelectEdge}
      menuActions={edgeMenuActions}
    />,
  );
  expect(renderedLabels()).toEqual(
    new Set(edgeListNodes.map((node) => node.label.rawText)),
  );
  vi.mocked(NodeValueLink).mockClear();

  view.rerender(
    <SubgraphEdgeList
      edges={edgeListEdges}
      nodes={edgeListNodes}
      selectedEdgeId="e:second"
      onSelect={onSelectEdge}
      menuActions={edgeMenuActions}
    />,
  );

  expect(renderedLabels()).toEqual(
    new Set([
      "alpha.example.test",
      "bravo.example.test",
      "charlie.example.test",
      "delta.example.test",
    ]),
  );
  // 表示中の行の button は focus を受けたまま押せない (aria-disabled)。
  expect(edgeDetailButton(/alpha\.example\.test/)).not.toHaveAttribute(
    "aria-disabled",
  );
  expect(edgeDetailButton(/charlie\.example\.test/)).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  expect(edgeDetailButton(/echo\.example\.test/)).not.toHaveAttribute(
    "aria-disabled",
  );
});

test("上位の画面がエッジの一覧の props を変えずに描き直すとき、エッジの一覧を描き直さない", () => {
  const view = render(
    <SubgraphEdgeList
      edges={edgeListEdges}
      nodes={edgeListNodes}
      selectedEdgeId="e:first"
      onSelect={onSelectEdge}
      menuActions={edgeMenuActions}
    />,
  );
  vi.mocked(NodeValueLink).mockClear();

  view.rerender(
    <SubgraphEdgeList
      edges={edgeListEdges}
      nodes={edgeListNodes}
      selectedEdgeId="e:first"
      onSelect={onSelectEdge}
      menuActions={edgeMenuActions}
    />,
  );

  expect(vi.mocked(NodeValueLink).mock.calls).toEqual([]);
});

/** 詳細を出している関係を上位の画面と同じく state で持ち、同じ関数で選び直す。 */
function EdgeSelectionHost({ selectedEdgeId }: { selectedEdgeId?: string }) {
  const [onSelectRecord] = useState(() => () => {});
  const [assertions] = useState(() => () => null);
  return (
    <GraphExploreHarness
      onSelectRecord={onSelectRecord}
      selectedEdgeId={selectedEdgeId}
      onSelectEdge={onSelectEdge}
      assertions={assertions}
    />
  );
}

test("ノードの一覧は操作の列を左端に置き、同一性の値を 1 行に 1 つの組で出して全体を tooltip に入れる", async () => {
  stubFetch();
  render(<EdgeSelectionHost />);
  const table = await findNodeList();
  const headers = within(table)
    .getAllByRole("columnheader")
    .map((th) => th.textContent);
  expect(headers[0]).toBe("操作");

  const [, row] = within(table).getAllByRole("row");
  const identity = row?.children[headers.indexOf("同一性の値")];
  const lines = identity?.querySelectorAll("ul.value-lines > li") ?? [];
  expect(lines.length).toBeGreaterThan(0);
  for (const line of lines) {
    expect(line.getAttribute("title")).toBe(line.textContent);
  }
});

test("関係を選び直すとき、ノードの一覧を描き直さない。ノードを選ぶと、選んだ行を描き直す", async () => {
  stubFetch();
  const view = render(<EdgeSelectionHost />);
  // 描き直しの回数は、両方の一覧を開いて行を描いた後から数える。
  await findNodeList();
  await findEdgeList();
  expect(namedLabels()).toContain("C:\\Windows\\System32\\cmd.exe");
  vi.mocked(nodeLabelName).mockClear();

  // 詳細を出す関係は、プロセス cmd.exe から端末 HOST-C への関係である。もう 1 本の
  // 関係の終点は IP であり、行の文字列で 2 本を見分ける。
  view.rerender(<EdgeSelectionHost selectedEdgeId="e:ran_on:0001" />);

  expect(edgeDetailButton(/203\.0\.113\.21/)).not.toHaveAttribute(
    "aria-disabled",
  );
  expect(edgeDetailButton(/^(?!.*203\.0\.113\.21).*HOST-C/)).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  expect(vi.mocked(nodeLabelName).mock.calls).toEqual([]);

  const processButton = screen.getByRole("button", {
    name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
  });
  fireEvent.click(processButton);

  expect(processButton).toHaveAttribute("aria-pressed", "true");
  expect(namedLabels()).toEqual(new Set(["C:\\Windows\\System32\\cmd.exe"]));
});
