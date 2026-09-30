// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import type { SearchExpressionFailure } from "@/shared/lib/fetchState";
import { SearchExpressionErrorView } from "./SearchExpressionForm";

afterEach(() => {
  cleanup();
});

function renderForm(expression: string, error: SearchExpressionFailure) {
  render(<SearchExpressionErrorView expression={expression} error={error} />);
}

function errorRegion(): HTMLElement {
  return screen.getByRole("region", { name: "検索式の誤り" });
}

/** 誤りの表示の「名前: 値」の組の文字列。 */
function pairsOf(region: HTMLElement): (string | null)[] {
  return Array.from(region.querySelectorAll("li"), (item) => item.textContent);
}

/** 誤りの表示が式を描く段落。式の文字列と印だけを持つ。 */
function echo(): HTMLElement {
  const paragraph = errorRegion().querySelector(".search-expression-echo");
  if (!(paragraph instanceof HTMLElement)) {
    throw new Error("the expression is not drawn");
  }
  return paragraph;
}

test("誤りの理由と位置を出し、式の誤りの範囲を mark で示す", () => {
  renderForm("LogonType = 3", {
    reason: "unexpected_character",
    offset: 10,
    length: 1,
    description: "単独の「=」があります。",
  });

  const region = errorRegion();
  expect(pairsOf(region)).toEqual([
    "検索式の誤り: 単独の「=」があります。",
    "誤りの位置: 11 文字目",
  ]);
  const marks = echo().querySelectorAll("mark");
  expect(marks).toHaveLength(1);
  expect(marks[0]?.textContent).toBe("=");
  // 印の前後に式の残りを原文の並びで描く。
  expect(echo().textContent).toBe("LogonType = 3");
});

test("長さ 0 の誤りは、式の終わりに目印を置き、位置に式の終わりであることを書く", () => {
  renderForm("(LogonType == 3", {
    reason: "unclosed_parenthesis",
    offset: 15,
    length: 0,
    description: "開いた括弧を閉じる括弧がありません。",
  });

  expect(pairsOf(errorRegion())).toContain("誤りの位置: 式の末尾");
  const point = echo().querySelector("mark.search-expression-point");
  expect(point?.textContent).toBe("誤りの位置");
  // 目印は式の文字列の後ろにある。
  expect(point?.previousSibling?.textContent).toBe("(LogonType == 3");
  expect(point?.nextSibling).toBeNull();
});

test("式の制御文字を、範囲の外と範囲の中の両方で可視の符号にして描く", () => {
  renderForm("a\u0007 == \u001b", {
    reason: "unexpected_character",
    offset: 6,
    length: 1,
    description: "どの文字列も始められない文字があります。",
  });

  const codes = Array.from(
    echo().querySelectorAll(".raw-control-code"),
    (code) => code.textContent,
  );
  expect(codes).toEqual(["U+0007", "U+001B"]);
  expect(echo().querySelector("mark")?.textContent).toBe("U+001B");
  expect(echo().textContent).not.toContain("\u0007");
});
