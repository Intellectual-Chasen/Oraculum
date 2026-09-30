// @vitest-environment jsdom

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { Plus } from "lucide-react";
import { afterEach, expect, test } from "vitest";
import { IconButton } from "./IconButton";

afterEach(() => {
  cleanup();
});

function focusByKeyboard(element: HTMLElement) {
  act(() => {
    fireEvent.keyDown(document.body, { key: "Tab" });
    element.focus();
  });
}

test("label を読み上げの名前と focus の tooltip の両方に使う", () => {
  render(
    <IconButton label="条件に追加">
      <Plus aria-hidden="true" />
    </IconButton>,
  );
  const button = screen.getByRole("button", { name: "条件に追加" });
  focusByKeyboard(button);
  expect(screen.getByRole("tooltip").textContent).toBe("条件に追加");
});

test("押せないときは、押せない理由を tooltip に出す", () => {
  render(
    <IconButton
      label="メモを記録"
      isDisabled
      disabledReason={{ title: "記録できない", text: "編集者の役割が必要" }}
    >
      <Plus aria-hidden="true" />
    </IconButton>,
  );
  const button = screen.getByRole("button", { name: "メモを記録" });
  focusByKeyboard(button);
  expect(screen.getByRole("tooltip").textContent).toContain("記録できない");
});

test("押せる状態と押せない状態を切り替えても、同じ button を保ち focus を外さない", () => {
  const view = (isDisabled: boolean) => (
    <IconButton
      label="メモを記録"
      isDisabled={isDisabled}
      disabledReason={{ title: "記録できない", text: "編集者の役割が必要" }}
    >
      <Plus aria-hidden="true" />
    </IconButton>
  );
  const { rerender } = render(view(false));
  const button = screen.getByRole("button", { name: "メモを記録" });
  focusByKeyboard(button);

  rerender(view(true));
  expect(screen.getByRole("button", { name: "メモを記録" })).toBe(button);
  expect(button).toHaveFocus();

  rerender(view(false));
  expect(screen.getByRole("button", { name: "メモを記録" })).toBe(button);
  expect(button).toHaveFocus();
});
