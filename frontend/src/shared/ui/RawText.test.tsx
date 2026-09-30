// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { RawText } from "./RawText";

afterEach(cleanup);

function drawn(text: string): string {
  const { container } = render(<RawText text={text} />);
  return container.textContent ?? "";
}

test("制御文字と書式文字を持たない文字列を、そのまま描く", () => {
  expect(drawn("[08/Oct/2031:10:20:35 +0900]")).toBe(
    "[08/Oct/2031:10:20:35 +0900]",
  );
  expect(drawn("access.log")).toBe("access.log");
  expect(drawn("収集元の一覧")).toBe("収集元の一覧");
  expect(drawn("\u{1F600}")).toBe("\u{1F600}");
});

test("bidi 制御と tab と zero width を U+XXXX の符号にする", () => {
  expect(drawn("192.0.2.101‮")).toBe("192.0.2.101U+202E");
  expect(drawn("host-a	log")).toBe("host-aU+0009log");
  expect(drawn("192.0​.2.101")).toBe("192.0U+200B.2.101");
  expect(drawn("host-a\nlog")).toBe("host-aU+000Alog");
});

test("符号に置き換えた文字の他を、1 文字も変えずに描く", () => {
  const original = "192.0.2.101‮ host-a.log	";

  const output = drawn(original);

  expect(output).toBe("192.0.2.101U+202E host-a.logU+0009");
  expect(output.replaceAll("U+202E", "").replaceAll("U+0009", "")).toBe(
    "192.0.2.101 host-a.log",
  );
});

test("LF の符号の後で改行し、LF の符号を残す", () => {
  const { container } = render(<RawText text={"line-a\nline-b\r\nline-c"} />);

  expect(container.textContent).toBe("line-aU+000Aline-bU+000DU+000Aline-c");
  const breaks = container.querySelectorAll("br");
  expect(breaks).toHaveLength(2);
  for (const lineBreak of breaks) {
    expect(lineBreak.previousElementSibling?.textContent).toBe("U+000A");
  }
});

test("連続した空白を詰めずに描く枠に入れる", () => {
  const { container } = render(<RawText text="net  use" />);

  const frame = container.querySelector(".raw-text");
  expect(frame?.textContent).toBe("net  use");
});

test.each([
  // 先頭と末尾の空白 1 つは、要素の端で詰められる。
  [" net use", [" net use"]],
  ["net use ", ["net use "]],
  // tab は符号に置き換え、符号の前後の空白を保つ。
  ["net\t use", [" use"]],
  ["net \tuse", ["net "]],
])("空白を詰められうる部分 %j を枠に入れる", (text, framed) => {
  const { container } = render(<RawText text={text} />);

  expect(
    Array.from(container.querySelectorAll(".raw-text")).map(
      (frame) => frame.textContent,
    ),
  ).toEqual(framed);
});

test("詰められる空白を持たない部分は枠に入れない", () => {
  const { container } = render(<RawText text="net use" />);

  expect(container.querySelector(".raw-text")).toBeNull();
});

test("原文が持つ U+202E の 6 文字と、置き換えた符号を別の描画にする", () => {
  const literal = render(<RawText text="U+202E" />).container;
  const replaced = render(<RawText text="‮" />).container;

  expect(literal.querySelector(".raw-control-code")).toBeNull();
  const code = replaced.querySelector(".raw-control-code");
  expect(code?.textContent).toBe("U+202E");
  expect(code?.getAttribute("aria-label")).toBe("制御文字 U+202E");
});
