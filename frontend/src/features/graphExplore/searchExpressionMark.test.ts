import { expect, test } from "vitest";
import { markSearchExpression } from "./searchExpressionMark";

test("誤りの範囲の前・範囲・後に分け、位置を 1 から数えた文字の番号で書く", () => {
  const marked = markSearchExpression("LogonType = 3", {
    offset: 10,
    length: 1,
  });

  expect(marked).toEqual({
    before: "LogonType ",
    marked: "=",
    after: " 3",
    position: "11 文字目",
  });
});

// U+20BB7 は UTF-16 では 2 つの code unit で書く。範囲は code point で数える。
test("範囲を code point で数え、surrogate pair を 1 文字として扱う", () => {
  const text = "user == \u{20BB7}a && x";
  const marked = markSearchExpression(text, { offset: 8, length: 2 });

  expect(marked.before).toBe("user == ");
  expect(marked.marked).toBe("\u{20BB7}a");
  expect(marked.after).toBe(" && x");
  expect(marked.before + marked.marked + marked.after).toBe(text);
  expect(marked.position).toBe("9–10 文字目");
});

test("式の終わりの長さ 0 の誤りは、範囲を空にし、式の末尾であることを書く", () => {
  const text = "(LogonType == 3";
  const marked = markSearchExpression(text, {
    offset: Array.from(text).length,
    length: 0,
  });

  expect(marked).toEqual({
    before: text,
    marked: "",
    after: "",
    position: "式の末尾",
  });
});

test("式の途中の長さ 0 の誤りは、範囲を空にし、次の文字の前と書く", () => {
  const marked = markSearchExpression("a and  or b", { offset: 6, length: 0 });

  expect(marked.before).toBe("a and ");
  expect(marked.marked).toBe("");
  expect(marked.after).toBe(" or b");
  expect(marked.position).toBe("7 文字目の前");
});

test("式の終わりを越える範囲は式の終わりで切り、位置は応答の値のまま書く", () => {
  const marked = markSearchExpression("x ==", { offset: 3, length: 5 });

  expect(marked.before).toBe("x =");
  expect(marked.marked).toBe("=");
  expect(marked.after).toBe("");
  expect(marked.position).toBe("4–8 文字目");
});
