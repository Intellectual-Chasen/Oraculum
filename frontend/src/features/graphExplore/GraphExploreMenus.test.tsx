// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { restoreClipboard, stubClipboard } from "@/testdata/clipboard";
import { processNodeId, terminalNodeId } from "@/testdata/graph/graphResponse";
import {
  findEdgeList,
  findNodeList,
  GraphExploreHarness,
  lastFigureRequest,
  renderGraphExplore,
  stubFetch,
} from "./graphExploreTestHarness";
import { edgeKindLabels } from "./labels";
import type { SubgraphDrawing } from "./subgraph";

// WebGL の renderer は jsdom で動かない。図は、点とエッジごとに右クリックの通知を上位へ渡す
// button を並べる stub に置き換える。cosmos.gl から通知を受ける部分は SubgraphCanvas の test が
// 確かめる。
vi.mock("./SubgraphCanvas", () => ({
  SubgraphCanvas: (props: {
    drawing: SubgraphDrawing;
    onNodeContextMenu?: (nodeId: string, event: MouseEvent) => void;
    onEdgeContextMenu?: (edgeId: string, event: MouseEvent) => void;
  }) => (
    <ul aria-label="図の点">
      {props.drawing.points.map((point) => (
        <li key={point.id}>
          <button
            type="button"
            onClick={(event) =>
              props.onNodeContextMenu?.(point.id, event.nativeEvent)
            }
          >
            {`図の ${point.id} を右クリック`}
          </button>
        </li>
      ))}
      {props.drawing.links.map((link) => (
        <li key={link.id}>
          <button
            type="button"
            onClick={(event) =>
              props.onEdgeContextMenu?.(link.id, event.nativeEvent)
            }
          >
            {`図のエッジ ${link.id} を右クリック`}
          </button>
        </li>
      ))}
    </ul>
  ),
}));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  restoreClipboard();
});

const processName = "プロセス C:\\Windows\\System32\\cmd.exe";
const processLabel = "C:\\Windows\\System32\\cmd.exe";
const leadingItems = ["詳細を開く", "隣接ノードを追加", "隣接ノードだけを表示"];
const labelItems = [
  "表示名を含む条件に追加",
  "表示名を含まない条件に追加",
  "表示名をコピー",
];

function itemNames(menu: HTMLElement) {
  return within(menu)
    .getAllByRole("menuitem")
    .map((item) => item.textContent);
}

/** ノードの一覧を開き、行の「…」の button からメニューを開く。 */
async function openRowMenu(nodeName: string) {
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", { name: `${nodeName} の操作` }),
  );
  return screen.getByRole("menu", { name: `${nodeName} の操作` });
}

function choose(name: string) {
  fireEvent.click(screen.getByRole("menuitem", { name }));
}

test("ノードの一覧の行は、プロセスに親子関係の表示を、端末にこの端末でフィルタを追加したメニューを出す", async () => {
  stubFetch();
  renderGraphExplore();

  const processMenu = await openRowMenu(processName);
  expect(itemNames(processMenu)).toEqual([
    ...leadingItems,
    "プロセスの親子関係を表示",
    ...labelItems,
  ]);
  fireEvent.keyDown(processMenu, { key: "Escape" });

  const terminalMenu = await openRowMenu("端末 HOST-C");
  expect(itemNames(terminalMenu)).toEqual([
    ...leadingItems,
    "この端末でフィルタ",
    ...labelItems,
  ]);
  fireEvent.keyDown(terminalMenu, { key: "Escape" });

  const ipMenu = await openRowMenu("IP アドレス 203.0.113.21");
  expect(itemNames(ipMenu)).toEqual([...leadingItems, ...labelItems]);
});

test("行の右クリックと Shift+F10 で同じメニューを開き、Escape で開いた元へ focus を戻す", async () => {
  stubFetch();
  renderGraphExplore();
  await findNodeList();
  const detailButton = await screen.findByRole("button", {
    name: `${processName} の詳細を開く`,
  });
  const row = detailButton.closest("tr");
  if (row === null) throw new Error("the node row is missing");

  expect(fireEvent.contextMenu(row)).toBe(false);
  const clicked = screen.getByRole("menu", { name: `${processName} の操作` });
  fireEvent.keyDown(clicked, { key: "Escape" });
  expect(screen.queryByRole("menu")).toBeNull();

  detailButton.focus();
  fireEvent.keyDown(detailButton, { key: "F10", shiftKey: true });
  expect(
    screen.getByRole("menu", { name: `${processName} の操作` }),
  ).toBeInTheDocument();
  expect(screen.getByRole("menuitem", { name: "詳細を開く" })).toHaveFocus();
  fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
  expect(detailButton).toHaveFocus();
});

test("詳細を開くはノードを選び、ノードの詳細を取得する", async () => {
  const mock = stubFetch();
  renderGraphExplore();
  await openRowMenu(processName);
  choose("詳細を開く");

  await waitFor(() =>
    expect(mock.mock.calls.map((call) => call[0])).toContainEqual(
      expect.stringContaining(
        `/api/v0/nodes/${encodeURIComponent(processNodeId)}`,
      ),
    ),
  );
  expect(
    screen.getByRole("button", { name: `${processName} の詳細を開く` }),
  ).toHaveAttribute("aria-pressed", "true");
});

test("表示名を含む条件と含まない条件に追加すると、同じ文字列を反対の向きから外して要求に載せる", async () => {
  const mock = stubFetch();
  renderGraphExplore();
  await openRowMenu("IP アドレス 203.0.113.21");
  choose("表示名を含む条件に追加");
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toContain("&valueContains=203.0.113.21"),
  );

  await openRowMenu("IP アドレス 203.0.113.21");
  choose("表示名を含まない条件に追加");
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toContain("&valueExcludes=203.0.113.21"),
  );
  expect(lastFigureRequest(mock)).not.toContain("valueContains");
});

test("隣接ノードだけを表示とプロセスの親子関係を表示は、選んだノードを表示元にした要求を出す", async () => {
  const mock = stubFetch();
  renderGraphExplore();
  const origin = `nodeId=${encodeURIComponent(processNodeId)}`;

  await openRowMenu(processName);
  choose("隣接ノードだけを表示");
  await waitFor(() => expect(lastFigureRequest(mock)).toContain(origin));
  expect(await screen.findByText("隣接ノードの表示元")).toBeInTheDocument();

  await openRowMenu(processName);
  choose("プロセスの親子関係を表示");
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toContain("process_parent_child"),
  );
  expect(lastFigureRequest(mock)).toContain(origin);
});

test("この端末でフィルタは、端末の条件を要求に載せる", async () => {
  const mock = stubFetch();
  renderGraphExplore();
  await openRowMenu("端末 HOST-C");
  choose("この端末でフィルタ");

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toContain(
      `&terminal=${encodeURIComponent(terminalNodeId)}`,
    ),
  );
});

test("表示名をコピーは原文をそのまま clipboard へ渡し、結果を知らせる", async () => {
  stubFetch();
  const clipboard = stubClipboard();
  renderGraphExplore();
  await openRowMenu(processName);
  choose("表示名をコピー");

  expect(await screen.findByText("コピー済み: 表示名")).toBeInTheDocument();
  expect(clipboard.writeText).toHaveBeenCalledWith(processLabel);
});

test("clipboard が書き込みを拒むと、コピーできなかったことを知らせる", async () => {
  stubFetch();
  stubClipboard(async () => {
    throw new DOMException("denied", "NotAllowedError");
  });
  renderGraphExplore();
  await openRowMenu(processName);
  choose("表示名をコピー");

  expect(await screen.findByRole("alert")).toHaveTextContent(
    /^コピーできません: 表示名/,
  );
});

test("図のノードの右クリックは一覧の行と同じメニューを開く", async () => {
  const mock = stubFetch();
  renderGraphExplore();
  fireEvent.click(
    await screen.findByRole("button", {
      name: `図の ${processNodeId} を右クリック`,
    }),
  );
  const menu = screen.getByRole("menu", { name: `${processName} の操作` });
  expect(itemNames(menu)).toEqual([
    ...leadingItems,
    "プロセスの親子関係を表示",
    ...labelItems,
  ]);
  choose("隣接ノードだけを表示");
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toContain(
      `nodeId=${encodeURIComponent(processNodeId)}`,
    ),
  );
});

const ranOnEdgeId = "e:ran_on:0001";
const ranOnMenuName = `${edgeKindLabels.ran_on} ${processLabel} → HOST-C の操作`;
const edgeItems = [
  "エッジの詳細を表示",
  "始点のノードの詳細を開く",
  "終点のノードの詳細を開く",
  "始点の表示名を含む条件に追加",
  "終点の表示名を含む条件に追加",
  "エッジの識別子をコピー",
];

/** エッジを選ぶ操作を記録する harness を描く。 */
function renderWithEdgeSpy(selectedEdgeId?: string) {
  const onSelectEdge = vi.fn<(edgeId: string) => void>();
  render(
    <GraphExploreHarness
      onSelectRecord={() => {}}
      selectedEdgeId={selectedEdgeId}
      onSelectEdge={onSelectEdge}
      assertions={() => null}
    />,
  );
  return onSelectEdge;
}

async function openEdgeRowMenu() {
  await findEdgeList();
  fireEvent.click(screen.getByRole("button", { name: ranOnMenuName }));
  return screen.getByRole("menu", { name: ranOnMenuName });
}

test("エッジの一覧の行のメニューは、エッジの詳細と端のノードの詳細を開き、端の表示名を条件に追加する", async () => {
  const mock = stubFetch();
  const onSelectEdge = renderWithEdgeSpy();

  expect(itemNames(await openEdgeRowMenu())).toEqual(edgeItems);
  choose("エッジの詳細を表示");
  expect(onSelectEdge).toHaveBeenCalledWith(ranOnEdgeId);

  await openEdgeRowMenu();
  choose("始点のノードの詳細を開く");
  await waitFor(() =>
    expect(mock.mock.calls.map((call) => call[0])).toContainEqual(
      expect.stringContaining(
        `/api/v0/nodes/${encodeURIComponent(processNodeId)}`,
      ),
    ),
  );

  await openEdgeRowMenu();
  choose("終点の表示名を含む条件に追加");
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toContain("&valueContains=HOST-C"),
  );
});

test("詳細を表示しているエッジのメニューは詳細を表示する項目を使えなくし、エッジの識別子をコピーする", async () => {
  stubFetch();
  const clipboard = stubClipboard();
  const onSelectEdge = renderWithEdgeSpy(ranOnEdgeId);
  await openEdgeRowMenu();
  expect(
    screen.getByRole("menuitem", { name: "エッジの詳細を表示" }),
  ).toHaveAttribute("aria-disabled", "true");
  choose("エッジの詳細を表示");
  expect(onSelectEdge).not.toHaveBeenCalled();

  choose("エッジの識別子をコピー");
  expect(
    await screen.findByText("コピー済み: エッジの識別子"),
  ).toBeInTheDocument();
  expect(clipboard.writeText).toHaveBeenCalledWith(ranOnEdgeId);
});

test("エッジの一覧の行は、右クリックと Shift+F10 でも同じメニューを開く", async () => {
  stubFetch();
  renderWithEdgeSpy();
  await findEdgeList();
  const button = screen.getByRole("button", { name: ranOnMenuName });
  const row = button.closest("tr");
  if (row === null) throw new Error("the edge row is missing");
  expect(fireEvent.contextMenu(row)).toBe(false);
  fireEvent.keyDown(screen.getByRole("menu", { name: ranOnMenuName }), {
    key: "Escape",
  });

  button.focus();
  fireEvent.keyDown(button, { key: "F10", shiftKey: true });
  expect(
    screen.getByRole("menuitem", { name: "エッジの詳細を表示" }),
  ).toHaveFocus();
});

test("図のエッジの右クリックは一覧の行と同じメニューを開き、エッジの詳細を表示する", async () => {
  stubFetch();
  const onSelectEdge = renderWithEdgeSpy();
  fireEvent.click(
    await screen.findByRole("button", {
      name: `図のエッジ ${ranOnEdgeId} を右クリック`,
    }),
  );
  expect(itemNames(screen.getByRole("menu", { name: ranOnMenuName }))).toEqual(
    edgeItems,
  );
  choose("エッジの詳細を表示");
  expect(onSelectEdge).toHaveBeenCalledWith(ranOnEdgeId);
});

test("ブックマークを渡すと、ノードとエッジのメニューは付けてあるかで項目の名前を変え、対象と端点の原文の表示名を渡す", async () => {
  stubFetch();
  const node = { isMarked: vi.fn(() => false), onToggle: vi.fn() };
  const edge = { isMarked: vi.fn(() => true), onToggle: vi.fn() };
  render(
    <GraphExploreHarness
      onSelectRecord={() => {}}
      selectedEdgeId={undefined}
      onSelectEdge={() => {}}
      assertions={() => null}
      nodeBookmark={node}
      edgeBookmark={edge}
    />,
  );

  expect(itemNames(await openRowMenu(processName))).toEqual([
    ...leadingItems,
    "プロセスの親子関係を表示",
    "ブックマークに追加",
    ...labelItems,
  ]);
  choose("ブックマークに追加");
  expect(node.onToggle).toHaveBeenCalledWith(
    expect.objectContaining({ id: processNodeId, label: processLabel }),
  );

  await openEdgeRowMenu();
  choose("ブックマークから削除");
  expect(edge.isMarked).toHaveBeenCalledWith(ranOnEdgeId);
  expect(edge.onToggle).toHaveBeenCalledWith(
    expect.objectContaining({ id: ranOnEdgeId }),
    processLabel,
    "HOST-C",
  );
});
