// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, expect, onTestFinished, test, vi } from "vitest";
import { decodeGraphResponse } from "@/shared/contracts/graph";
import {
  graphResponseJson,
  processNodeId,
  terminalNodeId,
} from "@/testdata/graph/graphResponse";
import {
  defaultLayoutSettings,
  pointSize,
  selectedPointSize,
} from "./cosmosLayout";
import { SubgraphCanvas } from "./SubgraphCanvas";
import { SubgraphCanvasBoundary } from "./SubgraphCanvasBoundary";
import { buildSubgraphDrawing, type SubgraphDrawing } from "./subgraph";

// jsdom は WebGL の context を持たないため、cosmos.gl の図だけを stub に置き換える。
// 図に渡した値と破棄処理と操作の通知を、本 test が実物の component を描画して確かめる。
// 実描画はブラウザーで確かめる。
type Config = Record<string, unknown> & {
  onPointClick?: (index: number) => void;
  onLinkClick?: (index: number) => void;
  onPointMouseOver?: (index: number) => void;
  onPointMouseOut?: () => void;
  onPointContextMenu?: (
    index: number,
    position: [number, number],
    event: MouseEvent,
  ) => void;
  onLinkContextMenu?: (index: number, event: MouseEvent) => void;
};

const { graphs, device } = vi.hoisted(() => ({
  graphs: [] as {
    container: HTMLElement;
    config: Config;
    calls: Map<string, unknown[][]>;
  }[],
  // 次に作る図の WebGL の初期化が失敗するか。gpu は GPU で WebGL2 を描けるかである。
  // zoom は図の拡大率である。
  device: { fails: false, gpu: true, zoom: 2 },
}));

vi.mock("./webglProbe", () => ({ canDrawWithGpu: () => device.gpu }));

vi.mock("@cosmos.gl/graph", () => {
  class FakeGraph {
    container: HTMLElement;
    config: Config;
    calls = new Map<string, unknown[][]>();
    ready: Promise<void>;
    constructor(container: HTMLElement, config: Config) {
      this.container = container;
      this.config = { ...config };
      this.ready = device.fails
        ? Promise.reject(new Error("Failed to create WebGL context"))
        : Promise.resolve();
      graphs.push(this);
    }
    record(name: string, args: unknown[]) {
      this.calls.set(name, [...(this.calls.get(name) ?? []), args]);
    }
    setConfigPartial(config: Config) {
      Object.assign(this.config, config);
    }
    getZoomLevel() {
      return device.zoom;
    }
    getTrackedPointPositionsMap() {
      return new Map();
    }
    spaceToScreenPosition(position: [number, number]) {
      return position;
    }
    getNeighboringPointIndices() {
      return [];
    }
  }
  for (const name of [
    "setPointPositions",
    "setPointColors",
    "setPointSizes",
    "setImageData",
    "setPointShapes",
    "setPointImageIndices",
    "setPointImageSizes",
    "setPointClusters",
    "setLinks",
    "setLinkColors",
    "setLinkWidths",
    "trackPointPositionsByIndices",
    "render",
    "fitView",
    "fitViewByPointIndices",
    "setZoomLevel",
    "destroy",
  ]) {
    Object.defineProperty(FakeGraph.prototype, name, {
      value(this: FakeGraph, ...args: unknown[]) {
        this.record(name, args);
      },
    });
  }
  return { Graph: FakeGraph, PointShape: { Circle: 0, None: 8 } };
});

afterEach(() => {
  cleanup();
  graphs.length = 0;
  device.fails = false;
  device.gpu = true;
  device.zoom = 2;
  vi.restoreAllMocks();
});

test("GPU で WebGL2 を描けない環境では、cosmos.gl の図を作らず、描かない理由を出す", async () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  device.gpu = false;
  const { findByRole } = render(
    <SubgraphCanvasBoundary>
      <SubgraphCanvas
        drawing={drawingOf(graphResponseJson())}
        settings={defaultLayoutSettings}
        selectedNodeId={undefined}
        onSelectNode={() => {}}
      />
    </SubgraphCanvasBoundary>,
  );

  const alert = await findByRole("alert");
  expect(alert.textContent).toContain("GPU の WebGL2 なし");
  expect(alert.textContent).toContain("ノードの選択: Nodes のビュー");
  expect(graphs).toHaveLength(0);
});

test("WebGL の初期化が失敗したら、図の代わりに描けない理由と一覧への案内を出す", async () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  device.fails = true;
  const { findByRole } = render(
    <SubgraphCanvasBoundary>
      <SubgraphCanvas
        drawing={drawingOf(graphResponseJson())}
        settings={defaultLayoutSettings}
        selectedNodeId={undefined}
        onSelectNode={() => {}}
      />
    </SubgraphCanvasBoundary>,
  );

  const alert = await findByRole("alert");
  expect(alert.textContent).toContain("GPU の WebGL2 なし");
  expect(alert.textContent).toContain("ノードの選択: Nodes のビュー");
  // 図の破棄は、案内を描いた後の effect の片付けで走る。
  await waitFor(() => expect(lastGraph().calls.get("destroy")).toHaveLength(1));
});

function drawingOf(json: unknown) {
  return buildSubgraphDrawing(decodeGraphResponse(json, "fixture"));
}

function lastGraph() {
  const graph = graphs.at(-1);
  if (graph === undefined) throw new Error("no graph was created");
  return graph;
}

function lastCall(name: string) {
  return lastGraph().calls.get(name)?.at(-1) ?? [];
}

/** 点の並びの中の位置。 */
function pointIndex(drawing: SubgraphDrawing, id: string) {
  return drawing.points.map((point) => point.id).indexOf(id);
}

/** エッジの並びの中の位置。 */
function linkIndex(drawing: SubgraphDrawing, id: string) {
  return drawing.links.map((link) => link.id).indexOf(id);
}

/** Float32Array の RGBA の 4 つ組 1 つを `#rrggbb` に戻す。 */
function hexAt(colors: Float32Array, index: number) {
  return `#${Array.from(colors.slice(index * 4, index * 4 + 3), (value) =>
    Math.round(value * 255)
      .toString(16)
      .padStart(2, "0"),
  ).join("")}`;
}

function renderCanvas(
  drawing: SubgraphDrawing,
  props: Partial<Parameters<typeof SubgraphCanvas>[0]> = {},
) {
  return render(
    <SubgraphCanvas
      drawing={drawing}
      settings={defaultLayoutSettings}
      selectedNodeId={undefined}
      onSelectNode={() => {}}
      {...props}
    />,
  );
}

test("図を破棄するときに cosmos.gl の図を 1 回だけ捨てる", () => {
  const { unmount } = renderCanvas(drawingOf(graphResponseJson()));

  expect(graphs).toHaveLength(1);
  expect(lastGraph().calls.get("destroy")).toBeUndefined();

  unmount();

  expect(lastGraph().calls.get("destroy")).toHaveLength(1);
});

test("設定の値を cosmos.gl へ渡し、simulationCluster が 0 のときは cluster を渡さない", () => {
  const drawing = drawingOf(graphResponseJson());
  const { rerender } = renderCanvas(drawing);

  expect(lastGraph().config).toMatchObject(defaultLayoutSettings);
  expect(lastGraph().calls.get("setPointClusters")).toBeUndefined();

  const settings = {
    simulationGravity: 2,
    simulationLinkDistance: 30,
    simulationCluster: 0.3,
  };
  rerender(
    <SubgraphCanvas
      drawing={drawing}
      settings={settings}
      selectedNodeId={undefined}
      onSelectNode={() => {}}
    />,
  );

  // 設定を変えると、前の図を捨てて初期の座標から配置し直す。
  expect(graphs).toHaveLength(2);
  expect(graphs[0]?.calls.get("destroy")).toHaveLength(1);
  expect(lastGraph().config).toMatchObject(settings);
  const [clusters] = lastCall("setPointClusters") as [number[]];
  expect(clusters).toHaveLength(drawing.points.length);
});

test("初期配置と設定変更後の配置を弱い力で始め、選択では力を戻さない", () => {
  const drawing = drawingOf(graphResponseJson());
  const { rerender } = renderCanvas(drawing);
  const [initialAlpha] = lastCall("render") as [number];
  expect(initialAlpha).toBeGreaterThan(0);
  expect(initialAlpha).toBeLessThan(1);

  rerender(
    <SubgraphCanvas
      drawing={drawing}
      settings={defaultLayoutSettings}
      selectedNodeId={terminalNodeId}
      onSelectNode={() => {}}
    />,
  );
  expect(graphs).toHaveLength(1);
  // 選択の色だけを変える render は現在の力を保つ。
  expect(lastCall("render")).toEqual([undefined, 0]);

  rerender(
    <SubgraphCanvas
      drawing={drawing}
      settings={{ ...defaultLayoutSettings, simulationGravity: 2 }}
      selectedNodeId={terminalNodeId}
      onSelectNode={() => {}}
    />,
  );
  expect(graphs).toHaveLength(2);
  expect(lastCall("render")).toEqual([initialAlpha]);
});

test("エッジの端点と色を、応答の関係の状態で分ける", () => {
  const drawing = drawingOf(graphResponseJson());
  renderCanvas(drawing);

  const [pairs] = lastCall("setLinks") as [Float32Array];
  const ranOn = linkIndex(drawing, "e:ran_on:0001");
  expect(pairs[ranOn * 2]).toBe(pointIndex(drawing, processNodeId));
  expect(pairs[ranOn * 2 + 1]).toBe(pointIndex(drawing, terminalNodeId));
  // fixture のエッジはどれも observed であり、候補の色を置かない。
  const [colors] = lastCall("setLinkColors") as [Float32Array];
  expect(hexAt(colors, ranOn)).toBe("#a3adb9");
});

test("強調を与えると他のノードとエッジを薄く描き、強調のエッジを強調の色で太く描き、外すと図を作り直さずに戻す", () => {
  const drawing = drawingOf(graphResponseJson());
  const { rerender } = renderCanvas(drawing, {
    highlight: {
      nodeIds: new Set([processNodeId]),
      edgeIds: new Set(["e:ran_on:0001"]),
    },
  });
  const process = pointIndex(drawing, processNodeId);
  const ranOn = linkIndex(drawing, "e:ran_on:0001");
  const other = linkIndex(drawing, "e:process_communication:0002");

  // 強調するものだけを cosmos.gl の強調に渡し、他を薄く描かせる。
  expect(lastGraph().config.highlightedPointIndices).toEqual([process]);
  expect(lastGraph().config.highlightedLinkIndices).toEqual([ranOn]);
  expect(lastGraph().config.pointGreyoutOpacity).toBeLessThan(1);
  const [linkColors] = lastCall("setLinkColors") as [Float32Array];
  const [linkWidths] = lastCall("setLinkWidths") as [Float32Array];
  expect(hexAt(linkColors, ranOn)).toBe("#1f6f8b");
  expect(linkWidths[ranOn]).toBeGreaterThan(linkWidths[other] ?? 0);
  const rendersBefore = lastGraph().calls.get("render")?.length ?? 0;

  rerender(
    <SubgraphCanvas
      drawing={drawing}
      settings={defaultLayoutSettings}
      selectedNodeId={undefined}
      onSelectNode={() => {}}
    />,
  );
  expect(graphs).toHaveLength(1);
  expect(lastGraph().config.highlightedPointIndices).toBeUndefined();
  expect(lastGraph().config.highlightedLinkIndices).toBeUndefined();
  const [restoredLinks] = lastCall("setLinkColors") as [Float32Array];
  expect(hexAt(restoredLinks, ranOn)).toBe("#a3adb9");
  // cosmos.gl は render を呼ぶまで、差し替えた色を描かない。配置を動かさずに描き直す。
  const renders = lastGraph().calls.get("render") ?? [];
  expect(renders.length).toBeGreaterThan(rendersBefore);
  expect(renders.at(-1)).toEqual([undefined, 0]);
});

test("強調するエッジを描くときだけ、強調の意味を凡例に出す", () => {
  const drawing = drawingOf(graphResponseJson());
  const { rerender, getByRole } = render(
    <SubgraphCanvas
      drawing={drawing}
      settings={defaultLayoutSettings}
      selectedNodeId={undefined}
      onSelectNode={() => {}}
      highlight={{
        nodeIds: new Set([processNodeId]),
        edgeIds: new Set(["e:ran_on:0001"]),
      }}
      highlightLegend="開いたレコードに対応する関係"
    />,
  );
  const legend = getByRole("list", { name: "グラフの凡例" });
  expect(legend.textContent).toContain("開いたレコードに対応する関係");

  rerender(
    <SubgraphCanvas
      drawing={drawing}
      settings={defaultLayoutSettings}
      selectedNodeId={undefined}
      onSelectNode={() => {}}
    />,
  );
  expect(legend.textContent).not.toContain("開いたレコードに対応する関係");
});

test("点の識別子が重複する図は、cosmos.gl の図を作る前に描画の中で例外を投げる", () => {
  const drawing = drawingOf(graphResponseJson());
  const [point, second, ...rest] = drawing.points;
  if (point === undefined || second === undefined)
    throw new Error("the fixture carries fewer than 2 nodes");
  const broken = {
    points: [point, { ...second, id: point.id }, ...rest],
    links: [],
  };
  vi.spyOn(console, "error").mockImplementation(() => {});

  expect(() =>
    renderCanvas(broken, {
      settings: { ...defaultLayoutSettings, simulationCluster: 0.3 },
    }),
  ).toThrow(/more than once/);
  expect(graphs).toHaveLength(0);
});

test("端点が図に無いエッジを除かず、描画の中で例外を投げる", () => {
  const drawing = drawingOf(graphResponseJson());
  const [link, ...rest] = drawing.links;
  if (link === undefined) throw new Error("the fixture carries no edge");
  const broken = {
    ...drawing,
    links: [{ ...link, sourceNodeId: "n:missing" }, ...rest],
  };
  vi.spyOn(console, "error").mockImplementation(() => {});

  expect(() => renderCanvas(broken)).toThrow(/outside the graph/);
  expect(graphs).toHaveLength(0);
});

test("図の上でノードを選ぶと、選んだ識別子を上位へ渡す", () => {
  const drawing = drawingOf(graphResponseJson());
  const selected = vi.fn();
  renderCanvas(drawing, { onSelectNode: selected });

  lastGraph().config.onPointClick?.(pointIndex(drawing, processNodeId));

  expect(selected).toHaveBeenCalledTimes(1);
  expect(selected).toHaveBeenLastCalledWith(processNodeId);
});

test("ノードを指してダブルクリックすると、図の拡大へ届けずに識別子を上位へ渡す", () => {
  const drawing = drawingOf(graphResponseJson());
  const doubleClicked = vi.fn();
  const { container } = renderCanvas(drawing, {
    onDoubleClickNode: doubleClicked,
  });
  const zoom = vi.fn();
  const canvas = container.querySelector("canvas");
  canvas?.addEventListener("dblclick", zoom);

  lastGraph().config.onPointMouseOver?.(pointIndex(drawing, processNodeId));
  canvas?.dispatchEvent(new MouseEvent("dblclick", { bubbles: true }));

  expect(doubleClicked).toHaveBeenCalledWith(processNodeId);
  expect(zoom).not.toHaveBeenCalled();
});

test("図の上でノードを右クリックすると、その識別子と event を上位へ渡す", () => {
  const drawing = drawingOf(graphResponseJson());
  const opened = vi.fn();
  renderCanvas(drawing, { onNodeContextMenu: opened });
  const event = new MouseEvent("contextmenu", { clientX: 30, clientY: 40 });

  lastGraph().config.onPointContextMenu?.(
    pointIndex(drawing, terminalNodeId),
    [0, 0],
    event,
  );

  expect(opened).toHaveBeenCalledWith(terminalNodeId, event);
});

test("図の上でエッジを右クリックすると、その識別子と event を渡す", () => {
  const event = new MouseEvent("contextmenu", { clientX: 30, clientY: 40 });
  const opened = vi.fn();
  const drawing = drawingOf(graphResponseJson());
  renderCanvas(drawing, { onEdgeContextMenu: opened });
  lastGraph().config.onLinkContextMenu?.(
    linkIndex(drawing, "e:ran_on:0001"),
    event,
  );
  expect(opened).toHaveBeenCalledTimes(1);
  expect(opened).toHaveBeenLastCalledWith("e:ran_on:0001", event);
});

test("ノードを指していないときのダブルクリックは、図の拡大へ届ける", () => {
  const drawing = drawingOf(graphResponseJson());
  const doubleClicked = vi.fn();
  const { container } = renderCanvas(drawing, {
    onDoubleClickNode: doubleClicked,
  });
  const zoom = vi.fn();
  const canvas = container.querySelector("canvas");
  canvas?.addEventListener("dblclick", zoom);

  canvas?.dispatchEvent(new MouseEvent("dblclick", { bubbles: true }));

  expect(doubleClicked).not.toHaveBeenCalled();
  expect(zoom).toHaveBeenCalledTimes(1);
});

test("拡大・縮小・全体を表示の button が、それぞれ図の操作を呼ぶ", () => {
  const { getByRole } = renderCanvas(drawingOf(graphResponseJson()));

  fireEvent.click(getByRole("button", { name: "グラフを拡大" }));
  expect(lastCall("setZoomLevel")[0]).toBe(3);
  fireEvent.click(getByRole("button", { name: "グラフを縮小" }));
  expect(lastCall("setZoomLevel")[0]).toBeCloseTo(4 / 3);
  fireEvent.click(getByRole("button", { name: "全体を表示" }));
  expect(lastGraph().calls.get("fitView")).toHaveLength(1);
});

type SimulationConfig = {
  onSimulationTick: () => void;
  onSimulationEnd: () => void;
  onZoomStart: (event: unknown, userDriven: boolean) => void;
  onZoom: () => void;
};

function tick(count: number) {
  const config = lastGraph().config as unknown as SimulationConfig;
  for (let i = 0; i < count; i++) config.onSimulationTick();
}

test("配置の計算の間は 10 tick ごとに図の範囲に合わせ、分析者が動かした後は合わせない", () => {
  renderCanvas(drawingOf(graphResponseJson()));
  const config = lastGraph().config as unknown as SimulationConfig;

  tick(20);
  expect(lastGraph().calls.get("fitView")).toEqual([
    [0, 0.2],
    [0, 0.2],
  ]);
  // cosmos.gl のプログラムからの zoom (userDriven が偽) は、分析者の操作に数えない。
  config.onZoomStart({}, false);
  tick(10);
  expect(lastGraph().calls.get("fitView")).toHaveLength(3);
  config.onZoomStart({}, true);
  tick(10);
  config.onSimulationEnd();
  expect(lastGraph().calls.get("fitView")).toHaveLength(3);
});

test("図の範囲に合わせた拡大率が大きいほど点とエッジを太く描き、分析者が動かした後は倍率を保つ", () => {
  renderCanvas(drawingOf(graphResponseJson()));
  const config = lastGraph().config as unknown as SimulationConfig;

  device.zoom = 10;
  config.onZoom();
  expect(lastGraph().config.pointSizeScale).toBeCloseTo(2);
  expect(lastGraph().config.linkWidthScale).toBeCloseTo(Math.SQRT2);

  config.onZoomStart({}, true);
  device.zoom = 1000;
  config.onZoom();
  expect(lastGraph().config.pointSizeScale).toBeCloseTo(2);
});

test("配置の計算の間は tick ごとにラベルの座標を読まず、10 tick ごと、ノードを指したとき、計算が終わったときに読み、ノードの名前を描く", () => {
  vi.spyOn(window, "requestAnimationFrame").mockImplementation((draw) => {
    draw(0);
    return 0;
  });
  const texts: string[] = [];
  // jsdom の canvas は 2D の context を持たないため、ラベルを描く処理まで進む context を渡す。
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockImplementation(
    () =>
      new Proxy({} as CanvasRenderingContext2D, {
        get: (_, name) =>
          name === "measureText"
            ? () => ({ width: 10 })
            : name === "fillText"
              ? (text: string) => texts.push(text)
              : () => {},
      }),
  );
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(400);
  vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockReturnValue(400);
  const drawing = drawingOf(graphResponseJson());
  renderCanvas(drawing);
  const graph = lastGraph();
  const reads = vi
    .spyOn(
      graph as unknown as {
        getTrackedPointPositionsMap: () => Map<number, [number, number]>;
      },
      "getTrackedPointPositionsMap",
    )
    .mockImplementation(
      () => new Map(drawing.points.map((_, i) => [i, [0, 0]] as const)),
    );
  const config = graph.config as unknown as SimulationConfig;

  tick(9);
  expect(reads).not.toHaveBeenCalled();
  tick(1);
  expect(reads).toHaveBeenCalledTimes(1);
  // 計算の終わりを待たずに、ノードの名前を描く。
  expect(texts.length).toBeGreaterThan(0);
  graph.config.onPointMouseOver?.(pointIndex(drawing, terminalNodeId));
  tick(2);
  expect(reads).toHaveBeenCalledTimes(2);
  // 分析者が図を動かした後も、ラベルを動く点に合わせる。
  config.onZoomStart({}, true);
  tick(8);
  expect(reads).toHaveBeenCalledTimes(3);
  graph.config.onPointMouseOut?.();
  reads.mockClear();
  tick(20);
  expect(reads).toHaveBeenCalledTimes(2);
  config.onSimulationEnd();
  expect(reads).toHaveBeenCalledTimes(3);
});

test("配置の計算の間に、座標を読めない点から離れて指し直すと、読み直す回数を 30 回から数え直す", () => {
  const frames: FrameRequestCallback[] = [];
  vi.spyOn(window, "requestAnimationFrame").mockImplementation((draw) => {
    frames.push(draw);
    return frames.length;
  });
  const runFrames = (count: number) => {
    for (let i = 0; i < count && frames.length > 0; i++) frames.shift()?.(0);
  };
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockImplementation(
    () =>
      new Proxy({} as CanvasRenderingContext2D, {
        get: (_, name) =>
          name === "measureText" ? () => ({ width: 10 }) : () => {},
      }),
  );
  const drawing = drawingOf(graphResponseJson());
  renderCanvas(drawing);
  runFrames(Number.POSITIVE_INFINITY);
  const graph = lastGraph();
  // 点の座標を読めない状態を続け、読み直しを打ち切るまで数える。
  const reads = vi
    .spyOn(
      graph as unknown as {
        getTrackedPointPositionsMap: () => Map<number, [number, number]>;
      },
      "getTrackedPointPositionsMap",
    )
    .mockImplementation(() => new Map());
  const terminal = pointIndex(drawing, terminalNodeId);

  graph.config.onPointMouseOver?.(terminal);
  runFrames(5);
  graph.config.onPointMouseOut?.();
  runFrames(1);
  reads.mockClear();
  graph.config.onPointMouseOver?.(terminal);
  runFrames(100);
  expect(reads).toHaveBeenCalledTimes(31);
});

test("全体を表示を押すと、配置の計算の間に図の範囲へ合わせ直す動きを戻す", () => {
  const { getByRole } = renderCanvas(drawingOf(graphResponseJson()));
  const config = lastGraph().config as unknown as SimulationConfig;
  tick(10);
  config.onZoomStart({}, true);
  tick(10);
  expect(lastGraph().calls.get("fitView")).toHaveLength(1);

  fireEvent.click(getByRole("button", { name: "全体を表示" }));
  expect(lastGraph().calls.get("fitView")).toHaveLength(2);
  tick(10);
  expect(lastGraph().calls.get("fitView")).toHaveLength(3);
});

test("強調を与えた図は強調した点を中央に置き、配置の計算の間も強調した点に視点を合わせる", () => {
  const drawing = drawingOf(graphResponseJson());
  renderCanvas(drawing, {
    highlight: { nodeIds: new Set([processNodeId]), edgeIds: new Set() },
  });
  const process = pointIndex(drawing, processNodeId);
  const terminal = pointIndex(drawing, terminalNodeId);
  const [positions] = lastCall("setPointPositions") as [Float32Array];
  const distance = (index: number) => {
    const x = (positions[index * 2] ?? 0) - 4096 / 2;
    const y = (positions[index * 2 + 1] ?? 0) - 4096 / 2;
    return Math.hypot(x, y);
  };
  expect(distance(process)).toBeLessThan(distance(terminal));

  tick(10);
  expect(lastGraph().calls.get("fitViewByPointIndices")).toEqual([
    [[process], 0, 0.2],
  ]);
  expect(lastGraph().calls.get("fitView")).toBeUndefined();
});

test("拡大・縮小の button を押した後は、配置の計算の間も終わった後も図の範囲に合わせない", () => {
  const { getByRole } = renderCanvas(drawingOf(graphResponseJson()));
  const config = lastGraph().config as unknown as SimulationConfig;

  fireEvent.click(getByRole("button", { name: "グラフを拡大" }));
  tick(20);
  config.onSimulationEnd();
  expect(lastGraph().calls.get("fitView")).toBeUndefined();
});

test("ノードを指すと、そのノードに繋がるエッジだけを残して他を薄くし、離すと戻す", () => {
  const drawing = drawingOf(graphResponseJson());
  renderCanvas(drawing);
  const graph = lastGraph();

  graph.config.onPointMouseOver?.(pointIndex(drawing, terminalNodeId));
  expect(graph.config.highlightedLinkIndices).toEqual([
    linkIndex(drawing, "e:ran_on:0001"),
  ]);

  graph.config.onPointMouseOut?.();
  expect(graph.config.highlightedLinkIndices).toBeUndefined();
});

test("ノードを指すと、そのノードの座標を読み出す対象に足す", () => {
  const drawing = drawingOf(graphResponseJson());
  renderCanvas(drawing);
  const ip = pointIndex(drawing, "n:ip:5d72");

  lastGraph().config.onPointMouseOver?.(ip);

  const [tracked] = lastCall("trackPointPositionsByIndices") as [number[]];
  expect(tracked).toContain(ip);
});

test("図の上でエッジを押すと、その識別子を渡す", () => {
  const selectEdge = vi.fn();
  const drawing = drawingOf(graphResponseJson());
  renderCanvas(drawing, { onSelectEdge: selectEdge });
  lastGraph().config.onLinkClick?.(linkIndex(drawing, "e:ran_on:0001"));
  expect(selectEdge).toHaveBeenCalledTimes(1);
  expect(selectEdge).toHaveBeenLastCalledWith("e:ran_on:0001");
});

test("ノードの色を画面の配色の変数から取り、選んでいるノードは文字の色で塗って大きく描く", () => {
  const root = document.documentElement;
  root.style.setProperty("--kind-process", "#123456");
  root.style.setProperty("--kind-terminal", "#654321");
  root.style.setProperty("--ink", "#010203");
  onTestFinished(() => root.removeAttribute("style"));
  const drawing = drawingOf(graphResponseJson());

  renderCanvas(drawing, { selectedNodeId: terminalNodeId });

  const [colors] = lastCall("setPointColors") as [Float32Array];
  const [sizes] = lastCall("setPointSizes") as [Float32Array];
  const process = pointIndex(drawing, processNodeId);
  const terminal = pointIndex(drawing, terminalNodeId);
  expect(hexAt(colors, process)).toBe("#123456");
  expect(hexAt(colors, terminal)).toBe("#010203");
  expect(sizes[terminal]).toBeGreaterThan(sizes[process] ?? 0);
});

test("配色を変えても図の配置と視点を保持し、選択した点とエッジを塗り直す", async () => {
  const root = document.documentElement;
  onTestFinished(() => {
    root.removeAttribute("style");
    delete root.dataset.theme;
  });
  root.style.setProperty("--ink", "#010203");
  root.style.setProperty("--edge-observed", "#123456");
  const drawing = drawingOf(graphResponseJson());
  const { unmount } = renderCanvas(drawing, { selectedNodeId: terminalNodeId });
  const graph = lastGraph();
  const positions = graph.calls.get("setPointPositions");
  const fit = graph.calls.get("fitView");
  const [before] = lastCall("setPointColors") as [Float32Array];
  expect(hexAt(before, pointIndex(drawing, terminalNodeId))).toBe("#010203");
  root.style.setProperty("--ink", "#f1f2f3");
  root.style.setProperty("--edge-observed", "#abcdef");
  root.dataset.theme = "dark";
  await waitFor(() => {
    const [colors] = lastCall("setPointColors") as [Float32Array];
    expect(hexAt(colors, pointIndex(drawing, terminalNodeId))).toBe("#f1f2f3");
  });
  const [links] = lastCall("setLinkColors") as [Float32Array];
  expect(hexAt(links, linkIndex(drawing, "e:ran_on:0001"))).toBe("#abcdef");
  expect(lastGraph()).toBe(graph);
  expect(graph.calls.get("setPointPositions")).toBe(positions);
  expect(graph.calls.get("fitView")).toBe(fit);
  expect(lastCall("render")).toEqual([undefined, 0]);
  unmount();
  const renders = graph.calls.get("render");
  root.dataset.theme = "light";
  await Promise.resolve();
  expect(graph.calls.get("render")).toBe(renders);
});

test("選ぶノードを変えても図を作り直さず、色と大きさだけを差し替える", () => {
  const drawing = drawingOf(graphResponseJson());
  const { rerender } = renderCanvas(drawing);

  rerender(
    <SubgraphCanvas
      drawing={drawing}
      settings={defaultLayoutSettings}
      selectedNodeId={processNodeId}
      onSelectNode={() => {}}
    />,
  );

  expect(graphs).toHaveLength(1);
  const [sizes] = lastCall("setPointSizes") as [Float32Array];
  expect(sizes[pointIndex(drawing, processNodeId)]).toBe(selectedPointSize);
  expect(selectedPointSize).toBeGreaterThan(pointSize);
});

test("描画したノードの種別だけを凡例に載せる", () => {
  const { getByRole } = renderCanvas(drawingOf(graphResponseJson()));

  const legend = within(getByRole("list", { name: "グラフの凡例" }));
  for (const kind of ["端末", "プロセス", "IP アドレス"]) {
    expect(legend.getByText(kind)).toBeTruthy();
  }
  expect(legend.queryByText("ファイル")).toBeNull();
});

test.each([{ uncertain: false }, { uncertain: true }])(
  "不確定の連鎖の線を別の見た目で描き、描いたときだけ凡例に載せる (不確定の連鎖 $uncertain)",
  ({ uncertain }) => {
    const base = drawingOf(graphResponseJson());
    const drawing: SubgraphDrawing = {
      ...base,
      links: base.links.map((link) =>
        link.id === "e:ran_on:0001" && uncertain
          ? { ...link, state: "uncertain_chain" }
          : link,
      ),
    };

    const { getByRole } = renderCanvas(drawing);

    const [linkColors] = lastCall("setLinkColors") as [Float32Array];
    const link = linkIndex(drawing, "e:ran_on:0001");
    expect(hexAt(linkColors, link)).toBe(uncertain ? "#c026d3" : "#a3adb9");
    const legend = getByRole("list", { name: "グラフの凡例" }).textContent;
    expect(legend?.includes("未確定の推定エッジ")).toBe(uncertain);
  },
);
