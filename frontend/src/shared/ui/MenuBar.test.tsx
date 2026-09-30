// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { MenuBar, type MenuBarMenu } from "./MenuBar";

afterEach(() => {
  cleanup();
});

function menusOf(onRun: (key: string) => void): MenuBarMenu[] {
  const item = (key: string, label: string, disabled = false) => ({
    kind: "item" as const,
    key,
    label,
    disabled,
    onSelect: () => onRun(key),
  });
  return [
    {
      key: "view",
      label: "表示",
      entries: [item("graph", "グラフ"), item("timeline", "全件の時系列")],
    },
    {
      key: "search",
      label: "検索",
      entries: [item("terms", "文字列を外す"), item("period", "期間を外す")],
    },
    {
      key: "move",
      label: "移動",
      entries: [item("back", "戻る", true), item("forward", "進む")],
    },
  ];
}

function renderBar(onRun = vi.fn()) {
  render(
    <>
      <button type="button">本文の button</button>
      <MenuBar label="画面の操作" menus={menusOf(onRun)} />
    </>,
  );
  return onRun;
}

function heading(name: string) {
  return screen.getByRole("menuitem", { name });
}

function press(key: string, init: Partial<KeyboardEventInit> = {}) {
  const target = document.activeElement;
  if (target === null) throw new Error("no focused element");
  fireEvent.keyDown(target, { key, ...init });
}

test("見出しは menuitem で、Tab で止まるのは 1 つだけである", () => {
  renderBar();
  expect(
    screen.getByRole("menubar", { name: "画面の操作" }),
  ).toBeInTheDocument();
  for (const name of ["表示", "検索", "移動"]) {
    expect(heading(name)).toHaveAttribute("aria-haspopup", "menu");
    expect(heading(name)).toHaveAttribute("aria-expanded", "false");
  }
  expect(heading("表示")).toHaveAttribute("tabindex", "0");
  expect(heading("検索")).toHaveAttribute("tabindex", "-1");
});

test("F10 で最初の見出しへ移り、左右の矢印キーで見出しを移って端で回る", () => {
  renderBar();
  screen.getByRole("button", { name: "本文の button" }).focus();
  press("F10");
  expect(heading("表示")).toHaveFocus();

  press("ArrowRight");
  expect(heading("検索")).toHaveFocus();
  expect(heading("検索")).toHaveAttribute("tabindex", "0");
  expect(heading("表示")).toHaveAttribute("tabindex", "-1");
  press("ArrowRight");
  press("ArrowRight");
  expect(heading("表示")).toHaveFocus();
  press("ArrowLeft");
  expect(heading("移動")).toHaveFocus();
  press("Home");
  expect(heading("表示")).toHaveFocus();
  press("End");
  expect(heading("移動")).toHaveFocus();
});

test("Shift+F10 ではメニューバーへ移らない", () => {
  renderBar();
  const body = screen.getByRole("button", { name: "本文の button" });
  body.focus();
  press("F10", { shiftKey: true });
  expect(body).toHaveFocus();
});

test("下の矢印キーで開いて先頭に、上の矢印キーで開いて末尾に focus を置く", () => {
  renderBar();
  heading("検索").focus();
  press("ArrowDown");
  expect(heading("検索")).toHaveAttribute("aria-expanded", "true");
  expect(screen.getByRole("menu", { name: "検索" })).toBeInTheDocument();
  expect(screen.getByRole("menuitem", { name: "文字列を外す" })).toHaveFocus();

  press("Escape");
  expect(screen.queryByRole("menu")).toBeNull();
  expect(heading("検索")).toHaveFocus();
  press("ArrowUp");
  expect(screen.getByRole("menuitem", { name: "期間を外す" })).toHaveFocus();
});

test("開いている間の左右の矢印キーは、隣の見出しのメニューを開き直す", () => {
  renderBar();
  heading("表示").focus();
  press("Enter");
  press("ArrowRight");
  expect(screen.getByRole("menu", { name: "検索" })).toBeInTheDocument();
  expect(screen.queryByRole("menu", { name: "表示" })).toBeNull();
  expect(screen.getByRole("menuitem", { name: "文字列を外す" })).toHaveFocus();

  press("ArrowLeft");
  press("ArrowLeft");
  // 移動のメニューは先頭の項目が使えないため、次の使える項目に focus を置く。
  expect(screen.getByRole("menu", { name: "移動" })).toBeInTheDocument();
  expect(screen.getByRole("menuitem", { name: "進む" })).toHaveFocus();

  press("Escape");
  expect(heading("移動")).toHaveFocus();
});

test("項目を実行すると閉じて見出しへ focus を戻し、使えない項目は実行しない", () => {
  const onRun = renderBar();
  heading("移動").focus();
  press(" ");
  fireEvent.click(screen.getByRole("menuitem", { name: "戻る" }));
  expect(onRun).not.toHaveBeenCalled();
  press("Enter");
  expect(onRun).toHaveBeenCalledWith("forward");
  expect(screen.queryByRole("menu")).toBeNull();
  expect(heading("移動")).toHaveFocus();
});

test("見出しを押すとメニューを開き閉じし、開いている間に指した見出しのメニューへ替える", () => {
  renderBar();
  fireEvent.click(heading("表示"));
  expect(screen.getByRole("menu", { name: "表示" })).toBeInTheDocument();
  fireEvent.pointerEnter(heading("検索"));
  expect(screen.getByRole("menu", { name: "検索" })).toBeInTheDocument();
  fireEvent.pointerDown(heading("検索"));
  fireEvent.click(heading("検索"));
  expect(screen.queryByRole("menu")).toBeNull();

  fireEvent.click(heading("表示"));
  fireEvent.pointerDown(screen.getByRole("button", { name: "本文の button" }));
  expect(screen.queryByRole("menu")).toBeNull();
});
