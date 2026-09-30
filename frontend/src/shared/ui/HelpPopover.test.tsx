// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { HelpPopover } from "./HelpPopover";

afterEach(() => {
  cleanup();
});

function renderHelp() {
  render(<HelpPopover label="項目">説明の文</HelpPopover>);
  return screen.getByRole("button", { name: "項目 の説明" });
}

test("閉じている間は説明を描かない", () => {
  const trigger = renderHelp();
  expect(trigger.getAttribute("aria-expanded")).toBe("false");
  expect(screen.queryByRole("tooltip")).toBeNull();
});

test("ポインタを重ねると開き、離すと閉じる", () => {
  const trigger = renderHelp();
  fireEvent.mouseEnter(trigger);
  expect(screen.getByRole("tooltip").textContent).toBe("説明の文");
  fireEvent.mouseLeave(trigger);
  expect(screen.queryByRole("tooltip")).toBeNull();
});

test("クリックで開いたままにし、ポインタが離れても閉じない。もう一度のクリックで閉じる", () => {
  const trigger = renderHelp();
  fireEvent.click(trigger);
  fireEvent.mouseLeave(trigger);
  expect(screen.getByRole("tooltip")).toBeTruthy();
  expect(trigger.getAttribute("aria-controls")).toBe(
    screen.getByRole("tooltip").id,
  );
  fireEvent.click(trigger);
  expect(screen.queryByRole("tooltip")).toBeNull();
});

test("focus で開き、Escape と blur で閉じる", () => {
  const trigger = renderHelp();
  fireEvent.focus(trigger);
  // focus したボタンの説明として吹き出しを読み上げる。
  expect(trigger.getAttribute("aria-describedby")).toBe(
    screen.getByRole("tooltip").id,
  );
  fireEvent.keyDown(trigger, { key: "Escape" });
  expect(screen.queryByRole("tooltip")).toBeNull();
  fireEvent.click(trigger);
  fireEvent.blur(trigger);
  expect(screen.queryByRole("tooltip")).toBeNull();
});
