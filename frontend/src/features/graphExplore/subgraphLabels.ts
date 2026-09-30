import { formatCount } from "@/shared/lib/format";
import { toVisibleRawText } from "@/shared/lib/rawText";
import { edgeKindLabels } from "./labels";
import { drawnNodeLabel } from "./NodeLabelView";
import type { SubgraphDrawing } from "./subgraph";

// 既知の制限: 絞り込みに合ったノードのラベルを、選んでいなくても常に描き、
// ノードの数が smallDrawingSize 以下の図ではすべてのラベルを描く,利用者が配置した実資料で
// 測った。端末と文字列で絞った検索の結果、初期表示の端末、
// プロセスからの親子の連鎖の図で、ラベルは一部が重なるが識別できた。先に置いたラベルと重なる
// ラベルは描かない (drawLabels), 合うノードのラベルが重なって読めない結果を
// 実資料で見たときに見直す
const smallDrawingSize = 40;

/**
 * ラベルを描くノードの識別子を返す。ノードの少ない図ではすべてのノード、それ以外では絞り込みに
 * 合ったノード、選んでいるノードと、そのノードに繋がるノードである。
 * ラベルを出す範囲を決める材料であり、応答の集合を変えない。
 */
export function labelledNodeIds(
  drawing: SubgraphDrawing,
  selectedNodeId: string | undefined,
): Set<string> {
  if (drawing.points.length <= smallDrawingSize) {
    return new Set(drawing.points.map((point) => point.id));
  }
  const labelled = new Set(
    drawing.points
      .filter((point) => point.selection === "matched")
      .map((point) => point.id),
  );
  if (selectedNodeId === undefined) {
    return labelled;
  }
  labelled.add(selectedNodeId);
  for (const link of drawing.links) {
    if (link.sourceNodeId === selectedNodeId) {
      labelled.add(link.targetNodeId);
    }
    if (link.targetNodeId === selectedNodeId) {
      labelled.add(link.sourceNodeId);
    }
  }
  return labelled;
}

/** 点のラベルの文字列を、点の並びで返す。 */
export function pointLabelTexts(drawing: SubgraphDrawing): string[] {
  const labels = drawing.points.map((point) => drawnNodeLabel(point.label));
  const accountLabelCounts = new Map<string, number>();
  drawing.points.forEach((point, index) => {
    if (point.kind !== "account") return;
    const label = labels[index];
    if (label !== undefined) {
      accountLabelCounts.set(label, (accountLabelCounts.get(label) ?? 0) + 1);
    }
  });
  return drawing.points.map((point, index) => {
    const label = labels[index] ?? "";
    return point.kind === "account" &&
      (accountLabelCounts.get(label) ?? 0) > 1 &&
      point.accountIdentity !== undefined
      ? `${label} · ${toVisibleRawText(point.accountIdentity)}`
      : label;
  });
}

/** エッジのラベルの文字列を、エッジの並びで返す。 */
export function linkLabelTexts(drawing: SubgraphDrawing): string[] {
  return drawing.links.map(
    (link) =>
      `${edgeKindLabels[link.kind]} · ${formatCount(link.evidenceCount)}`,
  );
}

/** 画面の上の矩形 1 つ。左上の角と幅と高さを px で持つ。 */
export type LabelBox = { x: number; y: number; width: number; height: number };

/**
 * 点のラベルを置ける矩形を、置きたい順に返す。点の右、点の左の順で、図の枠 (幅 width、高さ
 * height) の左右に収まるものだけを返す。どちらも収まらないときは点の右だけを返す。上下は枠の中に
 * 収める。offset は点の中心からラベルまでの距離である。
 */
export function pointLabelBoxes(
  point: { x: number; y: number },
  size: { width: number; height: number },
  frame: { width: number; height: number },
  offset: number,
): LabelBox[] {
  const y = Math.min(
    Math.max(point.y - size.height / 2, 0),
    Math.max(frame.height - size.height, 0),
  );
  const right = { x: point.x + offset, y, ...size };
  const left = { x: point.x - offset - size.width, y, ...size };
  const inside = [right, left].filter(
    (box) => box.x >= 0 && box.x + box.width <= frame.width,
  );
  return inside.length > 0 ? inside : [right];
}

/** 重なりを調べる格子の一辺 (px)。 */
const cellSize = 64;

/**
 * 描く順に並べたラベルを、先に置いたラベルと重ならない位置に置き、置いたラベルの位置と矩形を
 * 返す。ラベルごとに矩形の候補を置きたい順に渡すと、先に置いた矩形と重ならない最初の候補に置く。
 * どの候補も重なるラベルは描かない。先頭のラベルは最初の候補に必ず置く。reserved は図の上に
 * 重ねた凡例やボタンの矩形であり、先頭のラベルの後に置くラベルはこれと重ならない。
 */
export function nonOverlappingLabels(
  labels: readonly (LabelBox | readonly LabelBox[])[],
  reserved: readonly LabelBox[] = [],
): { index: number; box: LabelBox }[] {
  const cells = new Map<string, LabelBox[]>();
  const cellKeys = (box: LabelBox) => {
    const keys: string[] = [];
    for (
      let cx = Math.floor(box.x / cellSize);
      cx <= Math.floor((box.x + box.width) / cellSize);
      cx++
    ) {
      for (
        let cy = Math.floor(box.y / cellSize);
        cy <= Math.floor((box.y + box.height) / cellSize);
        cy++
      ) {
        keys.push(`${cx},${cy}`);
      }
    }
    return keys;
  };
  const occupy = (box: LabelBox) => {
    for (const key of cellKeys(box)) {
      const list = cells.get(key);
      if (list === undefined) {
        cells.set(key, [box]);
      } else {
        list.push(box);
      }
    }
  };
  const overlaps = (box: LabelBox) =>
    cellKeys(box).some((key) =>
      (cells.get(key) ?? []).some(
        (other) =>
          box.x < other.x + other.width &&
          other.x < box.x + box.width &&
          box.y < other.y + other.height &&
          other.y < box.y + box.height,
      ),
    );
  const shown: { index: number; box: LabelBox }[] = [];
  labels.forEach((candidates, index) => {
    const list = Array.isArray(candidates) ? candidates : [candidates];
    const box = list.find((candidate) => !overlaps(candidate));
    if (box === undefined) {
      return;
    }
    shown.push({ index, box });
    occupy(box);
    if (index === 0) {
      for (const other of reserved) occupy(other);
    }
  });
  return shown;
}
