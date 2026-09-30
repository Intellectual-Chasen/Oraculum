// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { createDockview } from "dockview-react";
import { afterEach, expect, test, vi } from "vitest";
import groupsViewWorkspaceState from "@/testdata/workspace/groupsViewWorkspaceState.json";
import {
  type DockPanel,
  DockWorkspace,
  hideClippedTabs,
  keepSizesAcrossMaximize,
  restoreLayout,
} from "./DockWorkspace";
import { decodeWorkspaceState, workspacePanelIds } from "./workspaceState";

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.restoreAllMocks();
});

const panels: DockPanel[] = [
  { id: "a", title: "左", content: <p>左の中身</p>, keepsMounted: true },
  { id: "b", title: "中央", content: <p>中央の中身</p>, keepsMounted: true },
  { id: "c", title: "右", content: <p>右の中身</p>, keepsMounted: true },
  { id: "d", title: "下", content: <p>下の中身</p> },
  { id: "e", title: "下の裏", content: <p>下の裏の中身</p> },
];

test("既定の配置で開き、隠れたビューの中身は前面に出るまで描かない", async () => {
  render(<DockWorkspace panels={panels} />);
  for (const text of ["左の中身", "中央の中身", "右の中身", "下の中身"]) {
    expect(await screen.findByText(text)).toBeTruthy();
  }
  expect(screen.queryByText("下の裏の中身")).toBeNull();
});

test("配置を知らせ、知らせた配置を渡すとその配置で開く。localStorage に書かない", async () => {
  const onLayoutChange = vi.fn();
  const { unmount } = render(
    <DockWorkspace
      panels={panels}
      reveal={{ panelId: "e" }}
      onLayoutChange={onLayoutChange}
    />,
  );
  await screen.findByText("下の裏の中身");
  const layout = onLayoutChange.mock.lastCall?.[0];
  expect(Object.keys(layout.panels).sort()).toEqual(["a", "b", "c", "d", "e"]);
  expect(localStorage.length).toBe(0);
  unmount();

  render(<DockWorkspace panels={panels} layoutRequest={{ layout }} />);
  expect(await screen.findByText("下の裏の中身")).toBeTruthy();
  await waitFor(() => expect(screen.queryByText("下の中身")).toBeNull());
});

test("読めない配置と知らないビューを含む配置は捨て、既定の配置で開く", async () => {
  for (const broken of [{ not: "layout" }, { panels: { z: {} } }]) {
    render(
      <DockWorkspace panels={panels} layoutRequest={{ layout: broken }} />,
    );
    expect(await screen.findByText("右の中身")).toBeTruthy();
    const alert = screen
      .getByText("保存した配置の読み込み失敗")
      .closest('[role="alert"]');
    expect(alert?.textContent).toContain("表示: 初期の配置");
    cleanup();
  }
});

test("まとまりのビューを開いて保存した状態の配置を、まとまりのビューを除いて開く", async () => {
  const { dockLayout } = decodeWorkspaceState(groupsViewWorkspaceState);
  const workspacePanels: DockPanel[] = workspacePanelIds.map((id) => ({
    id,
    title: id,
    content: <p>{`${id} の中身`}</p>,
  }));

  render(
    <DockWorkspace
      panels={workspacePanels}
      layoutRequest={{ layout: dockLayout }}
    />,
  );

  // まとまりのビューが前面にあった区画は、先頭のビューを前面に出す。
  expect(await screen.findByText("record の中身")).toBeTruthy();
  expect(screen.queryByText("保存した配置の読み込み失敗")).toBeNull();
  expect(screen.getByRole("tab", { name: "ips" })).toBeTruthy();
  expect(screen.queryByRole("tab", { name: "clusters" })).toBeNull();
});

test("Edge Detail と Path を持たない保存した配置を開き、reveal で Node Detail の区画に重ねて開く", async () => {
  const { dockLayout } = decodeWorkspaceState(groupsViewWorkspaceState);
  const workspacePanels: DockPanel[] = workspacePanelIds.map((id) => ({
    id,
    title: id,
    content: <p>{`${id} の中身`}</p>,
    keepsMounted: id === "edgeDetail",
    column: id === "edgeDetail" || id === "path" ? "detail" : undefined,
  }));
  const layoutRequest = { layout: dockLayout };
  const { rerender } = render(
    <DockWorkspace panels={workspacePanels} layoutRequest={layoutRequest} />,
  );
  expect(await screen.findByText("detail の中身")).toBeTruthy();
  expect(screen.queryByText("保存した配置の読み込み失敗")).toBeNull();
  expect(screen.queryByRole("tab", { name: "edgeDetail" })).toBeNull();
  expect(screen.queryByRole("tab", { name: "path" })).toBeNull();

  for (const panelId of ["edgeDetail", "path"]) {
    rerender(
      <DockWorkspace
        panels={workspacePanels}
        layoutRequest={layoutRequest}
        reveal={{ panelId }}
      />,
    );
    expect(await screen.findByText(`${panelId} の中身`)).toBeTruthy();
    const groupOf = (name: string) =>
      screen.getByRole("tab", { name }).closest(".dv-groupview");
    expect(groupOf(panelId)).toBe(groupOf("detail"));
  }
});

test("閉じているときだけ開く reveal は、開いているビューを前面に出さない", async () => {
  const withColumn: DockPanel[] = [
    ...panels,
    { id: "f", title: "右の重ね", content: <p>右の重ねの中身</p>, column: "c" },
  ];
  const { rerender } = render(
    <DockWorkspace panels={withColumn} reveal={{ panelId: "f" }} />,
  );
  expect(await screen.findByText("右の重ねの中身")).toBeTruthy();
  rerender(<DockWorkspace panels={withColumn} reveal={{ panelId: "c" }} />);
  await waitFor(() => expect(screen.queryByText("右の重ねの中身")).toBeNull());

  rerender(
    <DockWorkspace
      panels={withColumn}
      reveal={{ panelId: "f", onlyIfClosed: true }}
    />,
  );
  await waitFor(() => expect(screen.queryByText("右の重ねの中身")).toBeNull());
  expect(screen.getByText("右の中身")).toBeTruthy();
});

test("まとまりのビューだけの区画・浮かせた区画・別ウィンドウの区画を除いた配置を開く", async () => {
  // biome-ignore lint/suspicious/noExplicitAny: 保存した JSON の木を組み替える
  const json: any = structuredClone(groupsViewWorkspaceState);
  const root = json.dockLayout.grid.root;
  const below = root.data[1].data[1].data;
  below.views = below.views.filter(
    (id: string) => id !== "clusters" && id !== "ips",
  );
  below.activeView = "record";
  root.data.push({
    type: "leaf",
    data: { views: ["clusters"], activeView: "clusters", id: "5" },
    size: 100,
  });
  json.dockLayout.activeGroup = "5";
  json.dockLayout.floatingGroups = [
    {
      data: { views: ["clusters", "ips"], activeView: "clusters", id: "6" },
      position: { left: 0, top: 0, width: 300, height: 200 },
    },
  ];
  json.dockLayout.popoutGroups = [
    {
      data: { views: ["clusters"], activeView: "clusters", id: "7" },
      position: { left: 0, top: 0, width: 300, height: 200 },
    },
  ];
  const { dockLayout } = decodeWorkspaceState(json);
  const workspacePanels: DockPanel[] = workspacePanelIds.map((id) => ({
    id,
    title: id,
    content: <p>{`${id} の中身`}</p>,
  }));

  render(
    <DockWorkspace
      panels={workspacePanels}
      layoutRequest={{ layout: dockLayout }}
    />,
  );

  // 浮かせた区画は、まとまりのビューを除き、残った ips を前面に出す。
  expect(await screen.findByText("ips の中身")).toBeTruthy();
  expect(await screen.findByText("record の中身")).toBeTruthy();
  expect(screen.queryByText("保存した配置の読み込み失敗")).toBeNull();
  expect(screen.queryByRole("tab", { name: "clusters" })).toBeNull();
});

test("hidden のビューは既定の配置と開き直す操作に出さず、保存した配置が含んでいればそのまま開く", async () => {
  const withHidden: DockPanel[] = [
    ...panels,
    { id: "m", title: "管理", content: <p>管理の中身</p>, hidden: true },
  ];
  const onLayoutChange = vi.fn();
  const { unmount } = render(
    <DockWorkspace
      panels={withHidden}
      reveal={{ panelId: "m" }}
      onLayoutChange={onLayoutChange}
    />,
  );
  await screen.findByText("管理の中身");
  const layout = onLayoutChange.mock.lastCall?.[0];
  unmount();

  render(<DockWorkspace panels={withHidden} />);
  await screen.findByText("右の中身");
  expect(screen.queryByText("管理の中身")).toBeNull();
  expect(screen.queryByRole("button", { name: "管理を開く" })).toBeNull();
  cleanup();

  render(<DockWorkspace panels={withHidden} layoutRequest={{ layout }} />);
  expect(await screen.findByText("管理の中身")).toBeTruthy();
  expect(screen.queryByText("保存した配置の読み込み失敗")).toBeNull();
});

test("maximizable のビューを含む区画だけに最大にする操作を出し、押すと元の大きさに戻す操作に変わる", async () => {
  const withMaximizable = panels.map((panel) =>
    panel.id === "b" ? { ...panel, maximizable: true } : panel,
  );
  render(<DockWorkspace panels={withMaximizable} />);
  await screen.findByText("中央の中身");
  const [maximize, ...others] = screen.getAllByRole("button", {
    name: "ビューを最大化",
  });
  expect(others).toHaveLength(0);
  fireEvent.click(maximize as HTMLElement);
  expect(
    await screen.findByRole("button", { name: "ビューの大きさを復元" }),
  ).toBeTruthy();
});

/** 配置の JSON で、views に id を持つ区画の activeView を返す。 */
function activeViewOf(layout: unknown, id: string): unknown {
  const walk = (node: unknown): unknown => {
    const item = node as { type: string; data: unknown };
    if (item.type === "leaf") {
      const data = item.data as { views: string[]; activeView?: string };
      return data.views.includes(id) ? data.activeView : undefined;
    }
    for (const child of item.data as unknown[]) {
      const found = walk(child);
      if (found !== undefined) return found;
    }
    return undefined;
  };
  return walk((layout as { grid: { root: unknown } }).grid.root);
}

test("区画を最大にしている間の変更も、最大にしていない配置として知らせる", async () => {
  const onLayoutChange = vi.fn();
  const withMaximizable = panels.map((panel) =>
    panel.id === "d" ? { ...panel, maximizable: true } : panel,
  );
  const { rerender } = render(
    <DockWorkspace panels={withMaximizable} onLayoutChange={onLayoutChange} />,
  );
  await screen.findByText("下の中身");
  fireEvent.click(screen.getByRole("button", { name: "ビューを最大化" }));
  await screen.findByRole("button", { name: "ビューの大きさを復元" });
  onLayoutChange.mockClear();
  // 最大にした区画の中で、前面のタブを替える。
  rerender(
    <DockWorkspace
      panels={withMaximizable}
      onLayoutChange={onLayoutChange}
      reveal={{ panelId: "e" }}
    />,
  );
  await waitFor(() =>
    expect(activeViewOf(onLayoutChange.mock.lastCall?.[0], "e")).toBe("e"),
  );
  expect(onLayoutChange.mock.lastCall?.[0].grid.maximizedNode).toBeUndefined();
  expect(
    screen.getByRole("button", { name: "ビューの大きさを復元" }),
  ).toBeTruthy();
});

test("最大にした状態で保存した配置は、最大を解いて開く", () => {
  const make = () =>
    createDockview(document.createElement("div"), {
      createComponent: () => ({
        element: document.createElement("div"),
        init: () => {},
      }),
    });
  const saved = make();
  saved.layout(1600, 1000);
  saved.addPanel({ id: "a", component: "x" });
  saved.addPanel({
    id: "b",
    component: "x",
    position: { referencePanel: "a", direction: "right" },
  });
  saved.getPanel("b")?.api.maximize();
  const layout = saved.toJSON();
  expect("maximizedNode" in layout.grid).toBe(true);

  const opened = make();
  opened.layout(1600, 1000);
  const known = ["a", "b"].map((id) => ({ id, title: id, content: null }));
  expect(restoreLayout(opened, known, layout)).toBe(true);
  expect(opened.hasMaximizedGroup()).toBe(false);
  saved.dispose();
  opened.dispose();
});

test("最大にした区画を元に戻すと、各区画を最大にする前の大きさに戻す", () => {
  const element = document.createElement("div");
  const api = createDockview(element, {
    createComponent: () => {
      const content = document.createElement("div");
      return { element: content, init: () => {} };
    },
  });
  api.layout(1600, 1000);
  api.addPanel({ id: "left", component: "x" });
  api.addPanel({
    id: "center",
    component: "x",
    position: { referencePanel: "left", direction: "right" },
  });
  api.addPanel({
    id: "detail",
    component: "x",
    position: { referencePanel: "center", direction: "right" },
  });
  api.addPanel({
    id: "below",
    component: "x",
    position: { referencePanel: "center", direction: "below" },
  });
  api.getPanel("left")?.api.setSize({ width: 320 });
  api.getPanel("detail")?.api.setSize({ width: 420 });
  api.getPanel("below")?.api.setSize({ height: 380 });
  const kept = keepSizesAcrossMaximize(api);
  const sizes = () =>
    api.groups.map((group) => [group.api.width, group.api.height]);
  const before = sizes();
  const gridOf = (layout: unknown) => (layout as { grid: unknown }).grid;
  const layoutBefore = gridOf(kept.layout());
  for (const id of ["left", "center", "detail", "below"]) {
    api.getPanel(id)?.api.maximize();
    // 最大にしている間に保存する配置は、最大にする前の配置と同じ大きさを持つ。
    expect(gridOf(kept.layout())).toEqual(layoutBefore);
    api.getPanel(id)?.api.exitMaximized();
    expect(sizes()).toEqual(before);
  }
  kept.dispose();
  api.dispose();
});

test("保存した配置のタブは、保存した時の名前でなく今の名前で出す", async () => {
  const onLayoutChange = vi.fn();
  const { unmount } = render(
    <DockWorkspace panels={panels} onLayoutChange={onLayoutChange} />,
  );
  await screen.findByText("右の中身");
  const layout = onLayoutChange.mock.lastCall?.[0];
  unmount();

  const renamed = panels.map((panel) =>
    panel.id === "c" ? { ...panel, title: "Right" } : panel,
  );
  render(<DockWorkspace panels={renamed} layoutRequest={{ layout }} />);
  expect(await screen.findByRole("tab", { name: "Right" })).toBeTruthy();
  expect(screen.queryByRole("tab", { name: "右" })).toBeNull();
});

test("resetRequest を新しい値に変えると、配置を初めの形に戻す", async () => {
  const reveal = { panelId: "e" };
  const { rerender } = render(
    <DockWorkspace panels={panels} reveal={reveal} />,
  );
  await screen.findByText("下の裏の中身");
  rerender(<DockWorkspace panels={panels} reveal={reveal} resetRequest={{}} />);
  expect(await screen.findByText("下の中身")).toBeTruthy();
  await waitFor(() => expect(screen.queryByText("下の裏の中身")).toBeNull());
});

test("別ウィンドウを開けないときは、そのことを知らせて区画を残す", async () => {
  vi.spyOn(window, "open").mockReturnValue(null);
  render(<DockWorkspace panels={panels} />);
  await screen.findByText("右の中身");
  const [popout] = screen.getAllByRole("button", {
    name: "別ウィンドウに表示",
  });
  fireEvent.click(popout as HTMLElement);
  expect(await screen.findByText("別ウィンドウの表示失敗")).toBeTruthy();
  expect(screen.getByText("左の中身")).toBeTruthy();
});

test("タブの閉じる button の名前と、ビューを閉じたときの読み上げを日本語で出す", async () => {
  const { container } = render(<DockWorkspace panels={panels} />);
  const close = await screen.findByRole("button", { name: "下 を閉じる" });
  fireEvent.click(close);
  await waitFor(() =>
    expect(container.querySelector(".dv-live-region")?.textContent).toBe(
      "下 を閉じました",
    ),
  );
});

/** 配置の JSON から、tab で並ぶビューの識別子の組を集める。 */
function tabGroups(layout: unknown): string[][] {
  const groups: string[][] = [];
  const visit = (node: unknown) => {
    if (typeof node !== "object" || node === null) return;
    const views = (node as { views?: unknown }).views;
    if (Array.isArray(views)) groups.push(views as string[]);
    for (const child of Object.values(node)) visit(child);
  };
  visit(layout);
  return groups;
}

test("保存した配置に無い下のビューを開くと、下のビューの tab に足す", async () => {
  const onLayoutChange = vi.fn();
  const { unmount } = render(
    <DockWorkspace panels={panels} onLayoutChange={onLayoutChange} />,
  );
  await screen.findByText("下の中身");
  const layout = onLayoutChange.mock.lastCall?.[0];
  unmount();

  const added: DockPanel = {
    id: "f",
    title: "下の追加",
    content: <p>下の追加の中身</p>,
  };
  const withAdded = [...panels, added];
  const changes = vi.fn();
  const { rerender } = render(
    <DockWorkspace
      panels={withAdded}
      layoutRequest={{ layout }}
      onLayoutChange={changes}
    />,
  );
  await screen.findByText("右の中身");
  rerender(
    <DockWorkspace
      panels={withAdded}
      layoutRequest={{ layout }}
      reveal={{ panelId: "f" }}
      onLayoutChange={changes}
    />,
  );
  expect(await screen.findByText("下の追加の中身")).toBeTruthy();
  await waitFor(() =>
    expect(
      tabGroups(changes.mock.lastCall?.[0]).find((views) =>
        views.includes("f"),
      ),
    ).toEqual(expect.arrayContaining(["d", "e", "f"])),
  );
});

test("下のビューをすべて閉じた後に下のビューを開くと、中央のビューの下に区画を作る", async () => {
  const changes = vi.fn();
  const { rerender } = render(
    <DockWorkspace panels={panels} onLayoutChange={changes} />,
  );
  await screen.findByText("下の中身");
  // 下の区画の tab をすべて閉じる。
  for (const name of ["下", "下の裏"]) {
    fireEvent.click(
      await screen.findByRole("button", { name: `${name} を閉じる` }),
    );
  }
  await waitFor(() =>
    expect(tabGroups(changes.mock.lastCall?.[0]).flat()).not.toContain("d"),
  );
  // 右の区画を前面にしてから開き直す。
  rerender(
    <DockWorkspace
      panels={panels}
      reveal={{ panelId: "c" }}
      onLayoutChange={changes}
    />,
  );
  rerender(
    <DockWorkspace
      panels={panels}
      reveal={{ panelId: "e" }}
      onLayoutChange={changes}
    />,
  );
  expect(await screen.findByText("下の裏の中身")).toBeTruthy();
  await waitFor(() =>
    expect(
      tabGroups(changes.mock.lastCall?.[0]).find((views) =>
        views.includes("e"),
      ),
    ).toEqual(["e"]),
  );
});

test("reveal で指したビューを前面に出す", async () => {
  const { rerender } = render(<DockWorkspace panels={panels} />);
  await screen.findByText("下の中身");
  rerender(<DockWorkspace panels={panels} reveal={{ panelId: "e" }} />);
  expect(await screen.findByText("下の裏の中身")).toBeTruthy();
});

test("前面のタブが帯の端で切れていれば、帯を scroll して見せ、隠さない", async () => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
  vi.stubGlobal("requestAnimationFrame", (run: FrameRequestCallback) => {
    run(0);
    return 1;
  });
  // 帯は x = 0 から 100 まで見せ、タブは幅 60 で 2 つ並ぶ。前面の 2 つ目は 108 まで出て切れている。
  const root = document.createElement("div");
  const list = document.createElement("div");
  list.className = "dv-tabs-container";
  let scrollLeft = 12;
  Object.defineProperty(list, "scrollLeft", {
    get: () => scrollLeft,
    set: (value: number) => {
      scrollLeft = value;
    },
  });
  list.getBoundingClientRect = () => new DOMRect(0, 0, 100, 30);
  const tabs = [0, 60].map((offset, index) => {
    const tab = document.createElement("div");
    tab.className = index === 1 ? "dv-tab dv-active-tab" : "dv-tab";
    tab.getBoundingClientRect = () =>
      new DOMRect(offset - scrollLeft, 0, 60, 30);
    list.append(tab);
    return tab;
  });
  root.append(list);
  const stop = hideClippedTabs(root);
  await new Promise((resolve) => setTimeout(resolve, 0));
  expect(scrollLeft).toBe(20);
  expect(tabs[1]?.classList.contains("dock-tab-clipped")).toBe(false);
  expect(tabs[0]?.classList.contains("dock-tab-clipped")).toBe(true);
  stop();
  vi.unstubAllGlobals();
});

/** 幅 100 の帯に、left と width のタブを 1 つ置く。scroll すると scroll の event を送る。 */
function tabStrip(left: number, width: number, scrollStart: number) {
  const root = document.createElement("div");
  const list = document.createElement("div");
  list.className = "dv-tabs-container";
  const writes: number[] = [];
  let scrollLeft = scrollStart;
  Object.defineProperty(list, "scrollLeft", {
    get: () => scrollLeft,
    set: (value: number) => {
      scrollLeft = value;
      writes.push(value);
      list.dispatchEvent(new Event("scroll"));
    },
  });
  list.getBoundingClientRect = () => new DOMRect(0, 0, 100, 30);
  const tab = document.createElement("div");
  tab.className = "dv-tab dv-active-tab";
  tab.getBoundingClientRect = () =>
    new DOMRect(left - scrollLeft, 0, width, 30);
  list.append(tab);
  root.append(list);
  return { root, writes, scrollLeft: () => scrollLeft };
}

test("帯より広い前面のタブは左端に合わせるだけにし、scroll を繰り返さない。監視を止めると scroll しない", async () => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
  vi.stubGlobal("requestAnimationFrame", (run: FrameRequestCallback) =>
    setTimeout(() => run(0), 0),
  );
  vi.stubGlobal("cancelAnimationFrame", (id: number) => clearTimeout(id));
  const wide = tabStrip(50, 150, 12);
  const stop = hideClippedTabs(wide.root);
  for (let tick = 0; tick < 20; tick += 1) {
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
  stop();
  expect(wide.scrollLeft()).toBe(50);
  expect(wide.writes).toEqual([50]);

  // 判定の後、scroll する前に監視を止める。
  const stopped = tabStrip(60, 60, 12);
  const stopLater = hideClippedTabs(stopped.root);
  await new Promise((resolve) => setTimeout(resolve, 0));
  stopLater();
  for (let tick = 0; tick < 5; tick += 1) {
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
  expect(stopped.writes).toEqual([]);
  vi.unstubAllGlobals();
});
