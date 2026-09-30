import { expect, test } from "vitest";
import { anchorAtPoint, placeMenu } from "./menuPlacement";

const viewport = { width: 800, height: 600 };
const size = { width: 200, height: 120 };
const margin = 4;

test("収まる位置では、基準の左下からそのまま出す", () => {
  expect(placeMenu(anchorAtPoint(100, 50), size, viewport, margin)).toEqual({
    left: 100,
    top: 50,
  });
});

test("右端を越えるときは、右端から余白を空けた位置まで左へ寄せる", () => {
  expect(placeMenu(anchorAtPoint(700, 50), size, viewport, margin)).toEqual({
    left: viewport.width - margin - size.width,
    top: 50,
  });
});

test("下に収まらず上に収まるときは、基準の上端にメニューの下端を合わせる", () => {
  const anchor = { x: 10, top: 540, bottom: 560 };
  expect(placeMenu(anchor, size, viewport, margin)).toEqual({
    left: 10,
    top: anchor.top - size.height,
  });
});

test("上下のどちらにも収まらないときは、画面の下端に寄せ、上端より外へ出さない", () => {
  const tall = { width: 200, height: 500 };
  expect(
    placeMenu({ x: 10, top: 300, bottom: 320 }, tall, viewport, margin),
  ).toEqual({
    left: 10,
    top: viewport.height - margin - tall.height,
  });
  const taller = { width: 900, height: 700 };
  expect(placeMenu(anchorAtPoint(10, 300), taller, viewport, margin)).toEqual({
    left: margin,
    top: margin,
  });
});

test("左端と上端より外の基準は、余白の位置まで戻す", () => {
  expect(placeMenu(anchorAtPoint(-30, -10), size, viewport, margin)).toEqual({
    left: margin,
    top: margin,
  });
});
