// @vitest-environment jsdom

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { ThemeProvider, ThemeToggle } from "./ThemeToggle";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

test.each([false, true])(
  "OS のダーク設定 %s から両方向に切り替える",
  (matches) => {
    const media = new EventTarget();
    vi.stubGlobal("matchMedia", () => Object.assign(media, { matches }));
    const { rerender } = render(
      <ThemeProvider>
        <ThemeToggle />
      </ThemeProvider>,
    );
    const initial = matches ? "dark" : "light";
    const next = matches ? "light" : "dark";
    const label = matches ? "ライトテーマに切り替え" : "ダークテーマに切り替え";
    expect(document.documentElement.dataset.theme).toBe(initial);
    expect(
      screen.getByRole("button", { name: label }).querySelector("svg"),
    ).toHaveClass(matches ? "lucide-moon" : "lucide-sun");
    fireEvent.click(screen.getByRole("button", { name: label }));
    expect(document.documentElement.dataset.theme).toBe(next);
    const reverse = matches
      ? "ダークテーマに切り替え"
      : "ライトテーマに切り替え";
    expect(screen.getByRole("button", { name: reverse })).toBeVisible();
    expect(
      screen.getByRole("button", { name: reverse }).querySelector("svg"),
    ).toHaveClass(matches ? "lucide-sun" : "lucide-moon");

    rerender(
      <ThemeProvider>
        <p>別の画面</p>
      </ThemeProvider>,
    );
    rerender(
      <ThemeProvider>
        <ThemeToggle />
      </ThemeProvider>,
    );
    expect(document.documentElement.dataset.theme).toBe(next);
    fireEvent.click(screen.getByRole("button", { name: reverse }));
    expect(document.documentElement.dataset.theme).toBe(initial);
  },
);

test("手動選択前は OS に追従し、選択後は配色を保持し、購読を解除する", () => {
  const media = Object.assign(new EventTarget(), { matches: false });
  const remove = vi.spyOn(media, "removeEventListener");
  vi.stubGlobal("matchMedia", () => media);
  const { unmount } = render(
    <ThemeProvider>
      <ThemeToggle />
    </ThemeProvider>,
  );
  act(() => {
    media.matches = true;
    media.dispatchEvent(new Event("change"));
  });
  expect(document.documentElement.dataset.theme).toBe("dark");
  fireEvent.click(
    screen.getByRole("button", { name: "ライトテーマに切り替え" }),
  );
  act(() => {
    media.matches = false;
    media.dispatchEvent(new Event("change"));
    media.matches = true;
    media.dispatchEvent(new Event("change"));
  });
  expect(document.documentElement.dataset.theme).toBe("light");
  unmount();
  expect(remove).toHaveBeenCalledWith("change", expect.any(Function));
  expect(document.documentElement.dataset.theme).toBeUndefined();
});
