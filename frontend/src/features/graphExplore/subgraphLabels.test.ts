import { expect, test } from "vitest";
import { decodeGraphResponse } from "@/shared/contracts/graph";
import {
  graphResponseJson,
  invisibleCharacterGraphResponseJson,
  ipNodeId,
  processNodeId,
  terminalNodeId,
} from "@/testdata/graph/graphResponse";
import { pointClusters, seededRandom } from "./cosmosLayout";
import { edgeKindLabels } from "./labels";
import { buildSubgraphDrawing } from "./subgraph";
import {
  labelledNodeIds,
  linkLabelTexts,
  nonOverlappingLabels,
  pointLabelBoxes,
  pointLabelTexts,
} from "./subgraphLabels";

function drawingOf(json: unknown) {
  return buildSubgraphDrawing(decodeGraphResponse(json, "fixture"));
}

/** 端点のノードを count 件足した、ノードの多い図。 */
function crowdedDrawing(count: number) {
  const drawing = drawingOf(graphResponseJson());
  const endpoint = drawing.points.find(
    (point) => point.selection === "edge_endpoint",
  );
  if (endpoint === undefined) {
    throw new Error("the fixture carries no edge endpoint");
  }
  const extra = Array.from({ length: count }, (_, index) => ({
    ...endpoint,
    id: `n:ip:extra-${index}`,
  }));
  return { ...drawing, points: [...drawing.points, ...extra] };
}

test("ノードの少ない図では、すべてのノードにラベルを出す", () => {
  expect(labelledNodeIds(drawingOf(graphResponseJson()), undefined)).toEqual(
    new Set([terminalNodeId, processNodeId, ipNodeId]),
  );
});

test("ノードの多い図では、合ったノードと、選んでいるノードとその隣にだけラベルを出す", () => {
  const labelled = labelledNodeIds(crowdedDrawing(60), processNodeId);

  // 端末とプロセスは合ったノード、IP はプロセスの process_communication の相手である。
  expect(labelled).toContain(terminalNodeId);
  expect(labelled).toContain(processNodeId);
  expect(labelled).toContain(ipNodeId);
  // 足した端点はどれとも繋がらず、合ってもいない。
  expect(labelled).not.toContain("n:ip:extra-0");
});

test("ノードの多い図で何も選んでいないときは、合ったノードにだけラベルを出す", () => {
  expect(labelledNodeIds(crowdedDrawing(60), undefined)).toEqual(
    new Set([terminalNodeId, processNodeId]),
  );
});

test("ノードのラベルの bidi 制御を可視の符号にし、種別を添えない", () => {
  expect(
    pointLabelTexts(drawingOf(invisibleCharacterGraphResponseJson()))[0],
  ).toBe("HOST-U+202EC");
  expect(pointLabelTexts(drawingOf(graphResponseJson()))[1]).toBe("cmd.exe");
});

test("エッジのラベルに、エッジの種類と根拠のレコード数を書く", () => {
  const drawing = drawingOf(graphResponseJson());

  expect(linkLabelTexts(drawing)).toHaveLength(drawing.links.length);
  expect(linkLabelTexts(drawing)[0]).toMatch(/ · \d+$/);
});

test("先に置いたラベルと重なるラベルを外す", () => {
  const box = (x: number, y: number) => ({ x, y, width: 50, height: 12 });

  const indices = (boxes: Parameters<typeof nonOverlappingLabels>[0]) =>
    nonOverlappingLabels(boxes).map((placed) => placed.index);
  expect(indices([box(0, 0), box(10, 5), box(100, 0)])).toEqual([0, 2]);
  // 格子の境目をまたぐ矩形どうしの重なりも見つける。
  expect(indices([box(60, 60), box(100, 62)])).toEqual([0]);
});

test("先に置いたラベルと重なるときは、次の候補の位置に置く", () => {
  const box = (x: number, y: number) => ({ x, y, width: 50, height: 12 });

  // 2 つ目のラベルは点の右に置くと 1 つ目と重なり、点の左には置ける。
  expect(
    nonOverlappingLabels([[box(100, 0)], [box(130, 4), box(40, 4)]]),
  ).toEqual([
    { index: 0, box: box(100, 0) },
    { index: 1, box: box(40, 4) },
  ]);
  // どの候補も重なるラベルは外す。
  expect(
    nonOverlappingLabels([[box(100, 0)], [box(130, 4), box(70, 4)]]),
  ).toEqual([{ index: 0, box: box(100, 0) }]);
});

test("凡例とボタンの矩形に重なるラベルを外し、先頭のラベルは残す", () => {
  const box = (x: number, y: number) => ({ x, y, width: 50, height: 12 });
  const legend = { x: 0, y: 300, width: 400, height: 30 };

  expect(
    nonOverlappingLabels(
      [box(10, 305), box(100, 310), box(100, 100)],
      [legend],
    ).map((placed) => placed.index),
  ).toEqual([0, 2]);
});

test("点のラベルの候補を、点の右、点の左の順に、図の枠に収まるものだけ返す", () => {
  const size = { width: 80, height: 12 };
  const frame = { width: 400, height: 300 };
  const xs = (point: { x: number; y: number }) =>
    pointLabelBoxes(point, size, frame, 7).map((box) => box.x);

  expect(pointLabelBoxes({ x: 100, y: 100 }, size, frame, 7)).toEqual([
    { x: 107, y: 94, ...size },
    { x: 13, y: 94, ...size },
  ]);
  // 右端からはみ出すときは左だけ、左端からはみ出すときは右だけを返す。
  expect(xs({ x: 380, y: 100 })).toEqual([293]);
  expect(xs({ x: 50, y: 100 })).toEqual([57]);
  // 両方からはみ出すときは右に置く。
  expect(
    pointLabelBoxes(
      { x: 50, y: 100 },
      size,
      { width: 100, height: 300 },
      7,
    ).map((box) => box.x),
  ).toEqual([57]);
  // 上端と下端では枠の中に収める。
  expect(pointLabelBoxes({ x: 100, y: 2 }, size, frame, 7)[0]?.y).toBe(0);
  expect(pointLabelBoxes({ x: 100, y: 299 }, size, frame, 7)[0]?.y).toBe(288);
});

test("乱数は、法 2^31 の線形合同法を丸めずに計算した値と 2 万回先まで一致する", () => {
  const random = seededRandom(703);
  let exact = 703n;
  for (let i = 0; i < 20_000; i++) {
    exact = (exact * 1103515245n + 12345n) % 2147483648n;
    expect(random()).toBe(Number(exact) / 2147483648);
  }
});

test("同じ入力から同じ cluster の番号を出し、繋がったノードを同じ cluster に入れる", () => {
  const input = {
    ids: ["a", "b", "c", "x", "y", "z"],
    links: [
      { id: "1", source: "a", target: "b" },
      { id: "2", source: "b", target: "c" },
      { id: "3", source: "c", target: "a" },
      { id: "4", source: "x", target: "y" },
      { id: "5", source: "y", target: "z" },
      { id: "6", source: "z", target: "x" },
    ],
  };

  const clusters = pointClusters(input);

  expect(pointClusters(input)).toEqual(clusters);
  expect(clusters[0]).toBe(clusters[1]);
  expect(clusters[0]).toBe(clusters[2]);
  expect(clusters[3]).toBe(clusters[4]);
  expect(clusters[0]).not.toBe(clusters[3]);
});

test("操作の対象のアカウントの関係の名前は、ログオンと読める文字列を持たない", () => {
  // チケットを要求したサービスのアカウントも同じ種別で結ぶため、ログオンに限る名前にしない。
  expect(edgeKindLabels.record_target_account).not.toContain("ログオン");
});
