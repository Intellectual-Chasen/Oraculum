import { expect, test } from "vitest";
import {
  defaultLayoutSettings,
  initialPositions,
  type LayoutInput,
  layoutSettingRanges,
  sizeScaleForZoom,
} from "./cosmosLayout";

test("点の倍率は、大きな図で 1、拡大率とともに増え、上限で止まる", () => {
  expect(sizeScaleForZoom(0.5)).toBe(1);
  expect(sizeScaleForZoom(2.5)).toBe(1);
  expect(sizeScaleForZoom(10)).toBeCloseTo(2);
  expect(sizeScaleForZoom(10)).toBeGreaterThan(sizeScaleForZoom(5));
  expect(sizeScaleForZoom(1000)).toBe(3);
});

test("配置の設定の既定値は、画面の入力欄の範囲の中にある", () => {
  for (const [key, range] of Object.entries(layoutSettingRanges)) {
    const value =
      defaultLayoutSettings[key as keyof typeof defaultLayoutSettings];
    expect(value).toBeGreaterThanOrEqual(range.min);
    expect(value).toBeLessThanOrEqual(range.max);
    // 既定値は刻みの上にあり、入力欄の値をそのまま表せる。
    expect(Number.isInteger((value - range.min) / range.step)).toBe(true);
  }
});

/** 点の数が size の連結成分を、名前 prefix の鎖で作る。 */
function chain(prefix: string, size: number): LayoutInput {
  const ids = Array.from({ length: size }, (_, i) => `${prefix}${i}`);
  return {
    ids,
    links: ids.slice(1).map((id, i) => ({
      id: `${prefix}-e${i}`,
      source: ids[i] as string,
      target: id,
    })),
  };
}

function merge(...parts: LayoutInput[]): LayoutInput {
  return {
    ids: parts.flatMap((part) => part.ids),
    links: parts.flatMap((part) => part.links),
  };
}

/** 点の並びの位置の組から、座標の重心と、重心から最も遠い点までの距離を求める。 */
function discOf(positions: Float32Array, indices: readonly number[]) {
  const xs = indices.map((i) => positions[i * 2] as number);
  const ys = indices.map((i) => positions[i * 2 + 1] as number);
  const cx = xs.reduce((sum, x) => sum + x, 0) / indices.length;
  const cy = ys.reduce((sum, y) => sum + y, 0) / indices.length;
  const r = Math.max(
    ...indices.map((_, k) =>
      Math.hypot((xs[k] as number) - cx, (ys[k] as number) - cy),
    ),
  );
  return { cx, cy, r };
}

const indicesOf = (input: LayoutInput, prefix: string) =>
  input.ids.flatMap((id, i) => (id.startsWith(prefix) ? [i] : []));

test("連結成分ごとに円板を分け、最も大きい成分を中央に、ほかの成分をその円板の外に置く", () => {
  // 応答の並びでは成分の点が交互に現れる。並びで円板を埋めると成分が混ざる。
  const big = chain("a", 40);
  const small = chain("b", 10);
  const input: LayoutInput = {
    ids: big.ids.flatMap((id, i) =>
      i < small.ids.length ? [id, small.ids[i] as string] : [id],
    ),
    links: [...big.links, ...small.links],
  };
  const positions = initialPositions(input);
  const center = 4096 / 2;
  const bigDisc = discOf(positions, indicesOf(input, "a"));
  const smallDisc = discOf(positions, indicesOf(input, "b"));
  // 円板を並びで埋めた点の重心は、点の数が有限のため円板の中心から少しずれる。
  expect(Math.hypot(bigDisc.cx - center, bigDisc.cy - center)).toBeLessThan(
    bigDisc.r / 10,
  );
  expect(
    Math.hypot(smallDisc.cx - bigDisc.cx, smallDisc.cy - bigDisc.cy),
  ).toBeGreaterThan(bigDisc.r);
  // 成分の円板の面積は点の数に比例する。
  expect(bigDisc.r / smallDisc.r).toBeCloseTo(Math.sqrt(40 / 10), 0);
});

test("同じ入力からは同じ初期の座標を出す", () => {
  const input = merge(chain("a", 7), chain("b", 3), chain("c", 1));
  expect(initialPositions(input)).toEqual(initialPositions(input));
});

test("focus を与えたときは、focus の点を中央の円板に、残りの点を外側の輪に置く", () => {
  const input = merge(chain("a", 6), chain("b", 6));
  const focus = new Set(input.ids.filter((id) => id.startsWith("a")));
  const positions = initialPositions(input, focus);
  const center = 4096 / 2;
  const distance = (i: number) =>
    Math.hypot(
      (positions[i * 2] as number) - center,
      (positions[i * 2 + 1] as number) - center,
    );
  const inner = indicesOf(input, "a").map(distance);
  const outer = indicesOf(input, "b").map(distance);
  expect(Math.max(...inner)).toBeLessThanOrEqual(4096 / 8);
  for (const d of outer) expect(d).toBeCloseTo((4096 / 8) * 3, 3);
});
