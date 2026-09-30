import { Graph as CosmosGraph, PointShape } from "@cosmos.gl/graph";
import { Maximize, ZoomIn, ZoomOut } from "lucide-react";
import { type ReactNode, useEffect, useMemo, useRef, useState } from "react";
import type { NodeKind } from "@/shared/contracts/graph";
import { formatCount } from "@/shared/lib/format";
import { IconButton } from "@/shared/ui/IconButton";
import {
  type CosmosLayoutSettings,
  cosmosSimulation,
  initialPositions,
  initialSimulationAlpha,
  type LayoutInput,
  linkPairs,
  pointClusters,
  pointSize,
  rgba,
  selectedPointSize,
  sizeScaleForZoom,
} from "./cosmosLayout";
import { nodeKindColors, nodeKindColorTokens, nodeKindLabels } from "./labels";
import { WebglUnavailableError } from "./SubgraphCanvasBoundary";
import type { SubgraphDrawing } from "./subgraph";
import {
  type LabelBox,
  labelledNodeIds,
  linkLabelTexts,
  nonOverlappingLabels,
  pointLabelBoxes,
  pointLabelTexts,
} from "./subgraphLabels";
import { canDrawWithGpu } from "./webglProbe";

/** 画面の配色を読めないときの、選んでいるノードと強調の色。 */
const selectedColor = "#1f6f8b";
/** 種別の色を読めないときの点の色と、凡例の見本の色。種別ごとの色と重ならない値を置く。 */
const neutralColor = "#6b7280";

/**
 * エッジの描き方。色は画面の配色 (theme.css) の変数から取り、変数を読めないときは
 * fallback を使う。
 */
const edgeStyles = {
  observed: { token: "--edge-observed", fallback: "#a3adb9", size: 1.5 },
  candidate: { token: "--edge-candidate", fallback: "#d97706", size: 2 },
  uncertain: { token: "--edge-uncertain", fallback: "#c026d3", size: 2 },
} as const;

/**
 * 図の凡例。
 *
 * **色が表すものを図の隣に出す。** エッジのラベルは指したときだけ描くため、色と太さが
 * 関係の状態を表す。候補を作った由来は、図の下のエッジの一覧と詳細が持つ。
 */
const edgeLegend: { style: keyof typeof edgeStyles; text: string }[] = [
  { style: "observed", text: "観測したエッジ" },
  { style: "candidate", text: "推定したエッジ" },
  { style: "uncertain", text: "未確定の推定エッジ" },
];

/**
 * 図の範囲に合わせるときの、枠の内側の余白の割合。点の右に描くラベルが枠の端で切れない
 * 幅を空ける。
 */
const fitViewPadding = 0.2;
/** 配置の計算の途中で図の範囲に合わせ直す間隔 (tick の数)。 */
const fitEveryTicks = 10;

/**
 * 参照だけで分かったノードの点の画像。塗りを抜いた破線の輪を描く。cosmos.gl の点は輪郭を
 * 描けず、画像は点の色で染まらないため、種別の色ごとに画像を作る。2D の canvas を作れない
 * 環境では undefined を返し、塗った点のまま描く。
 */
function dashedRing(color: string): ImageData | undefined {
  const size = 64;
  const canvas = document.createElement("canvas");
  canvas.width = size;
  canvas.height = size;
  const context = canvas.getContext("2d");
  if (context === null) return undefined;
  context.strokeStyle = color;
  context.lineWidth = 9;
  context.setLineDash([11, 7]);
  context.beginPath();
  context.arc(size / 2, size / 2, size / 2 - 6, 0, Math.PI * 2);
  context.stroke();
  return context.getImageData(0, 0, size, size);
}
/** ラベルの文字の大きさ (px)。 */
const labelSize = 12;

type SubgraphCanvasProps = {
  drawing: SubgraphDrawing;
  /** 分析者が選んだ cosmos.gl の設定。値を変えると初期の座標から配置し直す。 */
  settings: CosmosLayoutSettings;
  /** 選んでいるノード。状態の所有者は上位の画面である。 */
  selectedNodeId: string | undefined;
  onSelectNode: (nodeId: string) => void;
  /** 図の上でエッジを選んだときに、そのエッジの識別子を上位へ渡す。 */
  onSelectEdge?: (edgeId: string) => void;
  /** 図の上でノードをダブルクリックしたときに、そのノードの識別子を上位へ渡す。 */
  onDoubleClickNode?: (nodeId: string) => void;
  /**
   * 図の上でノードを右クリックしたときに、そのノードの識別子と event を上位へ渡す。
   * ブラウザーのメニューは cosmos.gl が止める。
   */
  onNodeContextMenu?: (nodeId: string, event: MouseEvent) => void;
  /** 図の上でエッジを右クリックしたときに、そのエッジの識別子と event を上位へ渡す。 */
  onEdgeContextMenu?: (edgeId: string, event: MouseEvent) => void;
  /**
   * 強調するノードとエッジ。与えると、それ以外のノードとエッジを薄く描く。出ない場合は
   * 強調しない。
   */
  highlight?: GraphHighlight | undefined;
  /** 強調するエッジが何であるか。強調するエッジを描くとき、強調の色の線と一緒に凡例に出す。 */
  highlightLegend?: string;
  /** 凡例の末尾に足す項目 (`li`)。強調の件数の値の組を渡す。 */
  legendItems?: ReactNode;
};

/** 図の上で強調するノードとエッジの識別子。 */
export type GraphHighlight = {
  nodeIds: ReadonlySet<string>;
  edgeIds: ReadonlySet<string>;
  /** 強調するノードとエッジを根拠に持つレコードの名前。凡例と件数に表示する。無いときは開いたレコードである。 */
  source?: string;
};

/** 強調しないノードとエッジを描く不透明度。 */
const dimmedOpacity = 0.12;

/** 描画の単位を、配置の入力と同じ並びにする。 */
function inputOf(drawing: SubgraphDrawing): LayoutInput {
  return {
    ids: drawing.points.map((point) => point.id),
    links: drawing.links.map((link) => ({
      id: link.id,
      source: link.sourceNodeId,
      target: link.targetNodeId,
    })),
  };
}

/** 指している点とエッジ。エッジは cosmos.gl に渡したエッジの並びの位置で持つ。 */
type Hover = { point?: number; link?: number };

/**
 * 部分グラフを cosmos.gl (WebGL) で配置して描き、図の上の選択を上位へ渡す。
 *
 * cosmos.gl の図は effect が作り、effect の破棄処理が捨てる。ラベルは図に重ねた
 * Canvas 2D の層に描く。図は読み上げの対象にならないため、ノードとエッジを選ぶ操作は一覧が持つ。
 *
 * **ラベルを描くノードを、絞り込みに合ったノード、選んでいるノードとその隣に限る。**
 * 先に置いたラベルと重なるラベルは描かない。指したノードのラベルは、板に載せて必ず描く。
 * 図に描かれないラベルの値は、ノードの一覧で読める。
 */
export function SubgraphCanvas({
  drawing,
  settings,
  selectedNodeId,
  onSelectNode,
  onSelectEdge,
  onDoubleClickNode,
  onNodeContextMenu,
  onEdgeContextMenu,
  highlight,
  highlightLegend,
  legendItems,
}: SubgraphCanvasProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const labelCanvasRef = useRef<HTMLCanvasElement>(null);
  const legendRef = useRef<HTMLUListElement>(null);
  const zoomControlsRef = useRef<HTMLDivElement>(null);
  /** 分析者が図を拡大・縮小・移動したか。真の間は図の範囲に合わせ直さない。図を作るたびに偽へ戻す。 */
  const viewMovedRef = useRef(false);
  const graphRef = useRef<CosmosGraph | undefined>(undefined);
  const selectNodeRef = useRef(onSelectNode);
  const selectEdgeRef = useRef(onSelectEdge);
  const doubleClickNodeRef = useRef(onDoubleClickNode);
  const nodeContextMenuRef = useRef(onNodeContextMenu);
  const edgeContextMenuRef = useRef(onEdgeContextMenu);
  // **WebGL2 の context を作れなかったことを、描画の中で投げて図の error boundary に届ける。**
  // cosmos.gl は初期化の失敗を `ready` の reject で返し、例外を投げない。
  const [webglUnavailable, setWebglUnavailable] = useState(false);
  const input = useMemo(() => inputOf(drawing), [drawing]);
  // 壊れた入力の例外を、cosmos.gl の図を作る前に描画の中で投げ、図の error boundary に届ける。
  const pairs = useMemo(() => linkPairs(input), [input]);
  const clusterOn = settings.simulationCluster > 0;
  const clusters = useMemo(
    () => (clusterOn ? pointClusters(input) : undefined),
    [clusterOn, input],
  );
  const pointTexts = useMemo(() => pointLabelTexts(drawing), [drawing]);
  const linkTexts = useMemo(() => linkLabelTexts(drawing), [drawing]);
  const labelled = useMemo(() => {
    const ids = labelledNodeIds(drawing, selectedNodeId);
    const selected =
      selectedNodeId === undefined ? -1 : input.ids.indexOf(selectedNodeId);
    // 選んでいるノードのラベルを先に置き、隣のラベルとの重なりで外れないようにする。
    return [
      ...(selected >= 0 ? [selected] : []),
      ...input.ids.flatMap((id, index) =>
        ids.has(id) && index !== selected ? [index] : [],
      ),
    ];
  }, [drawing, input, selectedNodeId]);
  const labelledRef = useRef(labelled);
  const selectedRef = useRef(selectedNodeId);
  const highlightRef = useRef(highlight);
  /** 選択と強調とラベルの変化を、作った図へ写す関数。図を作る effect が差し替える。 */
  const applySelectionRef = useRef<() => void>(() => {});
  // 凡例に載せる種別。描画したノードが持つ種別だけを、種別の表の並びで出す。
  const drawnKinds = useMemo(() => {
    const present = new Set(drawing.points.map((point) => point.kind));
    return (Object.keys(nodeKindLabels) as NodeKind[]).filter((kind) =>
      present.has(kind),
    );
  }, [drawing]);

  useEffect(() => {
    selectNodeRef.current = onSelectNode;
    selectEdgeRef.current = onSelectEdge;
    doubleClickNodeRef.current = onDoubleClickNode;
    nodeContextMenuRef.current = onNodeContextMenu;
    edgeContextMenuRef.current = onEdgeContextMenu;
  }, [
    onSelectNode,
    onSelectEdge,
    onDoubleClickNode,
    onNodeContextMenu,
    onEdgeContextMenu,
  ]);

  useEffect(() => {
    const container = containerRef.current;
    if (container === null) {
      return;
    }
    // GPU で描けない環境では図を作らない。CPU で描く WebGL は画面の操作を止める。
    if (!canDrawWithGpu()) {
      setWebglUnavailable(true);
      return;
    }
    // ノードとエッジとラベルの色と書体を画面の基準 (theme.css) から取り、暗い配色でも読めるようにする。
    const style = getComputedStyle(container);
    const token = (name: string, fallback: string) =>
      style.getPropertyValue(name).trim() || fallback;
    let inkColor = token("--ink", "#17202b");
    let surfaceColor = token("--surface", "#ffffff");
    const labelFont = `${labelSize}px ${token("--sans", "sans-serif")}`;
    // 点ごとの、その点を端点に持つエッジの位置。cosmos.gl の getConnectedLinkIndices は
    // 両端が渡した点の中にあるエッジを返し、1 つの点に繋がるエッジを返さない。
    const linksOfPoint = new Map<number, number[]>();
    for (let link = 0; link < pairs.length / 2; link++) {
      for (const point of [pairs[link * 2], pairs[link * 2 + 1]]) {
        if (point === undefined) continue;
        const links = linksOfPoint.get(point);
        if (links === undefined) linksOfPoint.set(point, [link]);
        else if (links.at(-1) !== link) links.push(link);
      }
    }
    const hover: Hover = {};
    // 強調するエッジの位置。指したノードを離したときに、この強調へ戻す。
    let emphasizedLinks: number[] | undefined;
    // 強調する点の位置。空でなければ、視点を図全体でなくこの点に合わせる。
    let emphasizedPoints: number[] | undefined;
    let appliedEmphasis: GraphHighlight | undefined;
    let ticks = 0;
    // 点の大きさとエッジの太さに掛ける倍率。ラベルを点から離す幅にも使う。
    let sizeScale = 1;
    viewMovedRef.current = false;
    let frame = 0;
    // **配置の計算の間は、ラベルを 10 tick ごと、指したとき、拡大・移動のときに描く。**
    // ラベルの点の座標を読むたびに main thread が GPU の計算の完了を待つ。約 9,000 ノードの図で
    // tick ごとに描き直すと、待ちが画面の操作を遅らせた。計算の終わりまで描かないと、計算の続く
    // 約 20 秒の間、図にノードの名前が出ない。
    // 読み出す点を足した直後の frame では、足した点の座標がまだ読めない。読めるまで描き直す回数。
    let retries = 0;
    // ラベルの文字列の幅。書体は図を作り直すまで変わらないため、文字列ごとに 1 回だけ測る。
    const textWidths = new Map<string, number>();
    const widthOf = (context: CanvasRenderingContext2D, text: string) => {
      let width = textWidths.get(text);
      if (width === undefined) {
        width = context.measureText(text).width;
        textWidths.set(text, width);
      }
      return width;
    };

    const drawLabels = () => {
      frame = 0;
      const canvas = labelCanvasRef.current;
      const context = canvas?.getContext("2d");
      if (canvas === null || context === null || context === undefined) {
        return;
      }
      const ratio = window.devicePixelRatio || 1;
      const width = canvas.clientWidth;
      const height = canvas.clientHeight;
      if (canvas.width !== Math.round(width * ratio)) {
        canvas.width = Math.round(width * ratio);
      }
      if (canvas.height !== Math.round(height * ratio)) {
        canvas.height = Math.round(height * ratio);
      }
      context.setTransform(ratio, 0, 0, ratio, 0, 0);
      context.clearRect(0, 0, width, height);
      context.font = labelFont;
      context.textBaseline = "middle";
      const tracked = graph.getTrackedPointPositionsMap();
      const screenOf = (index: number) => {
        const position = tracked.get(index);
        return position === undefined
          ? undefined
          : graph.spaceToScreenPosition(position);
      };

      // 描く順: 指したノードのラベル、指したエッジのラベル、ノードのラベル、指したノードに繋がる
      // エッジのラベル。先に置いたラベルと重なるラベルは描かない。
      let missing = 0;
      const offset = (pointSize * sizeScale) / 2 + 3;
      // boxes はラベルを置ける矩形の候補を、置きたい順に持つ。
      const labels: { text: string; boxes: LabelBox[]; board: boolean }[] = [];
      const pushPoint = (index: number, board: boolean) => {
        const position = screenOf(index);
        const text = pointTexts[index];
        if (position === undefined) {
          missing++;
          return;
        }
        const [x, y] = position;
        if (!text || x < 0 || x > width || y < 0 || y > height) return;
        labels.push({
          text,
          board,
          boxes: pointLabelBoxes(
            { x, y },
            { width: widthOf(context, text), height: labelSize },
            { width, height },
            // 選んでいるノードは大きく描くため、ラベルを点から離す。
            input.ids[index] === selectedRef.current
              ? (selectedPointSize * sizeScale) / 2 + 3
              : offset,
          ),
        });
      };
      const pushLink = (link: number) => {
        const source = screenOf(pairs[link * 2] ?? -1);
        const target = screenOf(pairs[link * 2 + 1] ?? -1);
        const text = linkTexts[link];
        if (source === undefined || target === undefined) {
          missing++;
          return;
        }
        if (!text) return;
        const textWidth = widthOf(context, text);
        labels.push({
          text,
          board: false,
          boxes: [
            {
              x: (source[0] + target[0]) / 2 - textWidth / 2,
              y: (source[1] + target[1]) / 2 - labelSize / 2,
              width: textWidth,
              height: labelSize,
            },
          ],
        });
      };
      if (hover.point !== undefined) pushPoint(hover.point, true);
      if (hover.link !== undefined) pushLink(hover.link);
      for (const index of labelledRef.current) {
        if (index !== hover.point) pushPoint(index, false);
      }
      if (hover.point !== undefined) {
        for (const link of linksOfPoint.get(hover.point) ?? []) pushLink(link);
      }

      context.textAlign = "left";
      context.lineWidth = 3;
      context.strokeStyle = surfaceColor;
      // 図に重ねた凡例とボタンの下にラベルを描かない。
      const canvasRect = canvas.getBoundingClientRect();
      const reserved = [legendRef.current, zoomControlsRef.current].flatMap(
        (element) => {
          if (element === null) return [];
          const rect = element.getBoundingClientRect();
          return [
            {
              x: rect.left - canvasRect.left,
              y: rect.top - canvasRect.top,
              width: rect.width,
              height: rect.height,
            },
          ];
        },
      );
      for (const { index, box } of nonOverlappingLabels(
        labels.map((label) => label.boxes),
        reserved,
      )) {
        const label = labels[index];
        if (label === undefined) continue;
        const { text } = label;
        const y = box.y + labelSize / 2;
        if (label.board) {
          context.fillStyle = surfaceColor;
          context.shadowBlur = 8;
          context.shadowColor = "rgb(0 0 0 / 35%)";
          context.fillRect(box.x - 3, box.y - 3, box.width + 6, box.height + 6);
          context.shadowBlur = 0;
        } else {
          context.strokeText(text, box.x, y);
        }
        context.fillStyle = inkColor;
        context.fillText(text, box.x, y);
      }
      // 既知の制限: 座標を読めない点のラベルを描き直すのを 30 frame で打ち切り、打ち切った後はその点のラベルを描かない,
      // 利用者が配置した実資料の画面で、ノードを指した直後の frame では足した点の座標を読めず、描き直した次の frame で
      // エッジのラベルが出た,
      // 30 frame を過ぎてもラベルが出ない点を画面で見たときに見直す
      if (missing > 0 && retries < 30) {
        retries++;
        scheduleLabels();
      } else {
        retries = 0;
      }
    };
    const scheduleLabels = () => {
      if (frame === 0) frame = requestAnimationFrame(drawLabels);
    };
    // ラベルと、指したエッジのラベルの端点の座標を、cosmos.gl が tick ごとに読み出す。
    const trackLabelled = () => {
      const indices = new Set(labelledRef.current);
      if (hover.point !== undefined) {
        indices.add(hover.point);
        for (const index of graph.getNeighboringPointIndices(hover.point)) {
          indices.add(index);
        }
      }
      if (hover.link !== undefined) {
        indices.add(pairs[hover.link * 2] ?? 0);
        indices.add(pairs[hover.link * 2 + 1] ?? 0);
      }
      graph.trackPointPositionsByIndices([...indices]);
      // 足した点の座標を読めるまで、読み直す回数を数え直す。
      retries = 0;
      scheduleLabels();
    };
    // 強調があれば、強調した点と直接つながる点が画面に収まるように、無ければ図全体に視点を
    // 合わせる。強調した点だけに合わせると、力で寄り合った数点が描画面の中央に固まり、周りが
    // 空く。それより外の点は画面の端の外へはみ出して描かれる。
    const fitToFocus = (duration: number) => {
      if (emphasizedPoints !== undefined && emphasizedPoints.length > 0) {
        graph.fitViewByPointIndices(
          [
            ...emphasizedPoints,
            ...graph.getNeighboringPointIndices(emphasizedPoints),
          ],
          duration,
          fitViewPadding,
        );
        return;
      }
      graph.fitView(duration, fitViewPadding);
    };

    const highlightColor = token("--accent", selectedColor);
    const graph = new CosmosGraph(container, {
      ...cosmosSimulation,
      ...settings,
      backgroundColor: surfaceColor,
      // 矢印は線の中央に線の太さに比例した大きさで描かれる。細い線でも向きを読める大きさにし、
      // 長い線を薄くしすぎない。
      linkDefaultArrows: true,
      linkArrowsSizeScale: 2.5,
      linkVisibilityMinTransparency: 0.8,
      pointGreyoutOpacity: dimmedOpacity,
      linkGreyoutOpacity: dimmedOpacity,
      fitViewPadding,
      hoveredPointCursor: "pointer",
      hoveredLinkCursor: "pointer",
      hoveredLinkColor: highlightColor,
      hoveredLinkWidthIncrease: 2,
      onPointClick: (index) => {
        const id = input.ids[index];
        if (id !== undefined) selectNodeRef.current(id);
      },
      onPointContextMenu: (index, _position, event) => {
        const id = input.ids[index];
        if (id !== undefined) nodeContextMenuRef.current?.(id, event);
      },
      onLinkClick: (linkIndex) => {
        const id = input.links[linkIndex]?.id;
        if (id !== undefined) selectEdgeRef.current?.(id);
      },
      onLinkContextMenu: (linkIndex, event) => {
        const id = input.links[linkIndex]?.id;
        if (id !== undefined) edgeContextMenuRef.current?.(id, event);
      },
      // 指したノードに繋がるエッジを残し、他のエッジを薄くする。
      onPointMouseOver: (index) => {
        hover.point = index;
        graph.setConfigPartial({
          highlightedLinkIndices: linksOfPoint.get(index) ?? [],
        });
        trackLabelled();
      },
      onPointMouseOut: () => {
        hover.point = undefined;
        // 指す前の強調へ戻す。強調が無ければすべてのエッジを元の濃さで描く。
        graph.setConfigPartial({ highlightedLinkIndices: emphasizedLinks });
        trackLabelled();
      },
      onLinkMouseOver: (linkIndex) => {
        hover.link = linkIndex;
        trackLabelled();
      },
      onLinkMouseOut: () => {
        hover.link = undefined;
        trackLabelled();
      },
      // **配置が縮む間も図の範囲に合わせ直す。** fitViewOnInit は初期の座標に合わせるだけで、
      // 力で縮んだノードが中央の小さな塊に見える。分析者が拡大・移動した後は合わせない。
      onSimulationTick: () => {
        ticks++;
        if (ticks % fitEveryTicks !== 0) return;
        if (!viewMovedRef.current) fitToFocus(0);
        // ラベルを動く点に合わせて描き直す。分析者が図を動かした後は fitToFocus の zoom の通知が
        // 無いため、ここで描き直す。
        scheduleLabels();
      },
      onZoomStart: (_, userDriven) => {
        // 拡大・縮小のボタンの zoom は userDriven が偽であり、ボタンの側で記録する。
        if (userDriven) viewMovedRef.current = true;
      },
      // 図の範囲に合わせた視点の拡大率から、点とエッジの倍率を決める。分析者が動かした後は保つ。
      onZoom: () => {
        if (!viewMovedRef.current) {
          const next = sizeScaleForZoom(graph.getZoomLevel());
          if (next !== sizeScale) {
            sizeScale = next;
            // 矢印は線の太さに比例して大きくなるため、線には倍率の平方根を掛ける。
            graph.setConfigPartial({
              pointSizeScale: next,
              linkWidthScale: Math.sqrt(next),
            });
          }
        }
        scheduleLabels();
      },
      onSimulationEnd: () => {
        if (!viewMovedRef.current) fitToFocus(0);
        scheduleLabels();
        container.dataset.simulation = "ended";
      },
    });
    container.dataset.simulation = "running";
    let disposed = false;
    graph.ready.catch(() => {
      if (!disposed) setWebglUnavailable(true);
    });

    // 選んでいるノードは文字の色で塗り、大きく描く。種別の色のどれとも取り違えない。
    // 強調を与えたときは、強調しないノードとエッジを cosmos.gl の強調の設定で薄く描き、
    // 強調するエッジを強調の色で太く描く。
    applySelectionRef.current = () => {
      const selected =
        selectedRef.current === undefined
          ? -1
          : input.ids.indexOf(selectedRef.current);
      const emphasis = highlightRef.current;
      const emphasisChanged = emphasis !== appliedEmphasis;
      appliedEmphasis = emphasis;
      emphasizedLinks =
        emphasis === undefined
          ? undefined
          : input.links.flatMap((link, i) =>
              emphasis.edgeIds.has(link.id) ? [i] : [],
            );
      emphasizedPoints =
        emphasis === undefined
          ? undefined
          : input.ids.flatMap((id, i) => (emphasis.nodeIds.has(id) ? [i] : []));
      // 強調を差し替えたら、分析者が図を動かしていない限り、強調へ視点を合わせ直す。
      if (emphasisChanged && ticks > 0 && !viewMovedRef.current) {
        fitToFocus(250);
      }
      graph.setConfigPartial({
        highlightedPointIndices: emphasizedPoints,
        highlightedLinkIndices:
          hover.point === undefined
            ? emphasizedLinks
            : (linksOfPoint.get(hover.point) ?? []),
      });
      const colors = new Float32Array(input.ids.length * 4);
      const sizes = new Float32Array(input.ids.length).fill(pointSize);
      drawing.points.forEach((point, i) => {
        colors.set(
          rgba(token(nodeKindColorTokens[point.kind], neutralColor)),
          i * 4,
        );
      });
      if (selected >= 0) {
        colors.set(rgba(inkColor), selected * 4);
        sizes[selected] = selectedPointSize;
      }
      // 参照だけのノードは破線の輪で描く。選んでいる点は塗った点のまま描き、選択を示す。
      const ringKinds: NodeKind[] = [];
      const shapes = new Float32Array(input.ids.length);
      const imageIndices = new Float32Array(input.ids.length).fill(-1);
      drawing.points.forEach((point, i) => {
        if (point.referenced !== true || i === selected) return;
        let index = ringKinds.indexOf(point.kind);
        if (index < 0) index = ringKinds.push(point.kind) - 1;
        shapes[i] = PointShape.None;
        imageIndices[i] = index;
      });
      const rings = ringKinds.map((kind) =>
        dashedRing(token(nodeKindColorTokens[kind], neutralColor)),
      );
      if (rings.every((ring) => ring !== undefined)) {
        graph.setImageData(rings);
        graph.setPointShapes(shapes);
        graph.setPointImageIndices(imageIndices);
        graph.setPointImageSizes(sizes);
      }
      const linkColors = new Float32Array(input.links.length * 4);
      const linkWidths = new Float32Array(input.links.length);
      input.links.forEach((link, i) => {
        const kind =
          drawing.links[i]?.state === "uncertain_chain"
            ? "uncertain"
            : drawing.links[i]?.state === "candidate"
              ? "candidate"
              : "observed";
        const style = edgeStyles[kind];
        const emphasized = emphasis?.edgeIds.has(link.id) === true;
        linkColors.set(
          rgba(
            emphasized ? highlightColor : token(style.token, style.fallback),
          ),
          i * 4,
        );
        linkWidths[i] = emphasized ? style.size + 2 : style.size;
      });
      graph.setPointColors(colors);
      graph.setPointSizes(sizes);
      graph.setLinkColors(linkColors);
      graph.setLinkWidths(linkWidths);
      trackLabelled();
    };

    graph.setPointPositions(
      initialPositions(input, highlightRef.current?.nodeIds),
    );
    // cosmos.gl は setPointClusters を呼んだときだけ cluster の力を計算する。
    if (clusters !== undefined) {
      graph.setPointClusters(clusters);
    }
    graph.setLinks(pairs);
    applySelectionRef.current();
    graph.render(initialSimulationAlpha);
    graphRef.current = graph;

    // 配色だけを更新し、分析者が調整したノードの配置と視点を保持する。
    const themeObserver = new MutationObserver(() => {
      inkColor = token("--ink", "#17202b");
      surfaceColor = token("--surface", "#ffffff");
      applySelectionRef.current();
      graph.render(undefined, 0);
      scheduleLabels();
    });
    themeObserver.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["data-theme"],
    });

    // ダブルクリックで図を拡大せず、ノードの関係先を足す操作へ渡す。
    // 捕捉の段階で止め、cosmos.gl の canvas の拡大の処理へ届けない。
    const doubleClick = (event: MouseEvent) => {
      const id = hover.point === undefined ? undefined : input.ids[hover.point];
      if (id !== undefined && doubleClickNodeRef.current !== undefined) {
        event.stopPropagation();
        doubleClickNodeRef.current(id);
      }
    };
    container.addEventListener("dblclick", doubleClick, true);
    // 枠の大きさが変わるとラベルの層の大きさも変わるため、描き直す。分析者が視点を動かして
    // いなければ、新しい枠の大きさに視点を合わせ直す。区画の配置が決まる前の小さな枠で合わせた
    // 視点のままだと、広い描画面の中央にノードが小さく固まる。
    const observer =
      typeof ResizeObserver === "undefined"
        ? undefined
        : new ResizeObserver(() => {
            // cosmos.gl が canvas の大きさを変えた後に合わせるため、次の frame で行う。
            if (!viewMovedRef.current && ticks > 0) {
              requestAnimationFrame(() => {
                if (!disposed) fitToFocus(0);
              });
            }
            scheduleLabels();
          });
    observer?.observe(container);
    return () => {
      disposed = true;
      themeObserver.disconnect();
      observer?.disconnect();
      container.removeEventListener("dblclick", doubleClick, true);
      cancelAnimationFrame(frame);
      applySelectionRef.current = () => {};
      graphRef.current = undefined;
      graph.destroy();
    };
  }, [drawing, input, pairs, clusters, settings, pointTexts, linkTexts]);

  useEffect(() => {
    // cosmos.gl は render を呼ぶまで、差し替えた色と大きさを描かない。選択と強調が変わったときだけ
    // 描き直す。配置の力の強さを変えず、色の移り変わりも待たない。
    const recolors =
      selectedRef.current !== selectedNodeId ||
      highlightRef.current !== highlight;
    labelledRef.current = labelled;
    selectedRef.current = selectedNodeId;
    highlightRef.current = highlight;
    applySelectionRef.current();
    if (recolors) graphRef.current?.render(undefined, 0);
  }, [labelled, selectedNodeId, highlight]);

  if (webglUnavailable) {
    throw new WebglUnavailableError();
  }
  return (
    <>
      <div
        ref={containerRef}
        role="img"
        aria-label={`グラフ: ノード ${formatCount(drawing.points.length)}、エッジ ${formatCount(drawing.links.length)}`}
        className="subgraph-canvas"
      >
        <canvas ref={labelCanvasRef} className="subgraph-labels" />
      </div>
      <div ref={zoomControlsRef} className="zoom-controls">
        <IconButton
          variant="secondary"
          label="グラフを拡大"
          onPress={() => {
            const graph = graphRef.current;
            viewMovedRef.current = true;
            graph?.setZoomLevel(graph.getZoomLevel() * 1.5, 200);
          }}
        >
          <ZoomIn size={14} aria-hidden="true" />
        </IconButton>
        <IconButton
          variant="secondary"
          label="グラフを縮小"
          onPress={() => {
            const graph = graphRef.current;
            viewMovedRef.current = true;
            graph?.setZoomLevel(graph.getZoomLevel() / 1.5, 200);
          }}
        >
          <ZoomOut size={14} aria-hidden="true" />
        </IconButton>
        <IconButton
          variant="secondary"
          label="全体を表示"
          onPress={() => {
            // 分析者が動かした視点を捨て、配置の計算と区画の大きさの変化に合わせ直す動きへ戻す。
            viewMovedRef.current = false;
            graphRef.current?.fitView(200, fitViewPadding);
          }}
        >
          <Maximize size={14} aria-hidden="true" />
        </IconButton>
      </div>
      <ul ref={legendRef} aria-label="グラフの凡例" className="graph-legend">
        {drawnKinds.map((kind) => (
          <li key={kind}>
            <span
              aria-hidden="true"
              className="kind-dot"
              style={{ background: nodeKindColors[kind] }}
            />
            {nodeKindLabels[kind]}
          </li>
        ))}
        {edgeLegend
          .filter(
            (entry) =>
              entry.style !== "uncertain" ||
              drawing.links.some((link) => link.state === "uncertain_chain"),
          )
          .map((entry) => (
            <li key={entry.text}>
              <span
                aria-hidden="true"
                className="legend-line"
                style={{
                  borderTopColor: `var(${edgeStyles[entry.style].token}, ${edgeStyles[entry.style].fallback})`,
                  borderTopWidth: `${edgeStyles[entry.style].size}px`,
                }}
              />
              {entry.text}
            </li>
          ))}
        {highlightLegend !== undefined &&
        drawing.links.some((link) => highlight?.edgeIds.has(link.id)) ? (
          <li>
            <span
              aria-hidden="true"
              className="legend-line"
              style={{
                borderTopColor: `var(--accent, ${selectedColor})`,
                borderTopWidth: `${edgeStyles.observed.size + 2}px`,
              }}
            />
            {highlightLegend}
          </li>
        ) : null}
        {drawing.points.some((point) => point.referenced === true) ? (
          <li>
            <span
              aria-hidden="true"
              className="kind-dot"
              style={{
                background: "transparent",
                border: `2px dashed ${neutralColor}`,
              }}
            />
            参照だけのノード
          </li>
        ) : null}
        {legendItems}
      </ul>
    </>
  );
}
