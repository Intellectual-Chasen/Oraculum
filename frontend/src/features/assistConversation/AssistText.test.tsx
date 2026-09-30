// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { AssistMarkdown } from "./AssistText";

afterEach(cleanup);

test("AI の文を Markdown として描き、見出し・リスト・表・コードを要素にする", () => {
  const { container } = render(
    <AssistMarkdown
      text={[
        "## 結果",
        "",
        "- `whoami.exe` を実行",
        "- **2 件**",
        "",
        "| 端末 | 件数 |",
        "| --- | --- |",
        "| HOST-A | 2 |",
        "",
        "```",
        "net  use",
        "second line",
        "```",
      ].join("\n")}
    />,
  );
  expect(screen.getByRole("heading", { name: "結果" })).toBeTruthy();
  expect(screen.getAllByRole("listitem")).toHaveLength(2);
  expect(screen.getByRole("table")).toBeTruthy();
  expect(screen.getByRole("cell", { name: "HOST-A" })).toBeTruthy();
  expect(container.querySelector("strong")?.textContent).toBe("2 件");
  // コードブロックは空白と改行を原文のまま持つ。
  expect(container.querySelector("pre")?.textContent).toBe(
    "net  use\nsecond line\n",
  );
});

test("リンクと画像と HTML を描かず、bidi 制御の文字を符号にする", () => {
  const { container } = render(
    <AssistMarkdown
      text={[
        "[資料](https://example.test/a) と https://example.test/b と <https://example.test/c>",
        "",
        "![代わりの文](https://example.test/x.png)",
        "",
        "<b onclick=x>太字‮c</b>",
        "",
        "<div>block‮d</div>",
        "",
        "a‮b",
      ].join("\n")}
    />,
  );
  expect(container.querySelector("a")).toBeNull();
  expect(container.querySelector("img")).toBeNull();
  expect(container.querySelector("b")).toBeNull();
  expect(container.querySelector(".assist-markdown div")).toBeNull();
  expect(screen.getByText(/資料/)).toBeTruthy();
  expect(screen.getByText("代わりの文")).toBeTruthy();
  // HTML に見える文字列も文字列として描き、その中の制御文字も符号にする。
  expect(container.textContent).toContain("<b onclick=x>");
  expect(screen.getAllByRole("img", { name: "制御文字 U+202E" })).toHaveLength(
    3,
  );
  expect(container.textContent).not.toContain("‮");
});

test("原文で表示へ切り替えると、Markdown の記号を原文のまま出す", () => {
  const { container } = render(<AssistMarkdown text={"**C:\\*_x_**"} />);
  expect(container.querySelector("strong")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "原文で表示" }));
  expect(container.querySelector("strong")).toBeNull();
  expect(container.textContent).toContain("**C:\\*_x_**");
  fireEvent.click(screen.getByRole("button", { name: "Markdown で表示" }));
  expect(container.querySelector("strong")).toBeTruthy();
});
