// @vitest-environment jsdom
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { MissingValue } from "./MissingValue";

afterEach(cleanup);

test("値の位置に印だけを出し、ラベルは読み上げとマウスを重ねたときの tooltip に出す", () => {
  const { container } = render(<MissingValue description="フィールドなし" />);

  // 画面に見える文字は印「—」だけである。
  const mark = container.querySelector('[aria-hidden="true"]');
  expect(mark?.textContent).toBe("—");
  expect(container.querySelector("[title]")?.getAttribute("title")).toBe(
    "フィールドなし",
  );
  expect(screen.getByText("フィールドなし").className).toBe("sr-only");
});

test("keyboard の focus でラベルの tooltip を開き、focus が外れると閉じる", () => {
  const { container } = render(<MissingValue description="フィールドなし" />);
  const mark = container.querySelector<HTMLElement>("[tabindex]");
  if (mark === null) throw new Error("focus を受ける印が無い");

  act(() => {
    fireEvent.keyDown(document.body, { key: "Tab" });
    mark.focus();
  });
  expect(screen.getByRole("tooltip").textContent).toBe("フィールドなし");

  act(() => mark.blur());
  expect(screen.queryByRole("tooltip")).toBeNull();
});

test("押せる要素の中に置くときは、印を作らずにラベルを括弧なしでそのまま出す", () => {
  const { container } = render(<MissingValue description="表示名なし" plain />);
  expect(container.textContent).toBe("表示名なし");
  expect(container.querySelector("[title]")).toBeNull();
});
