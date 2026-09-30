import { expect, test } from "vitest";
import { byteOrderMark, toCsv } from "./csv";

test("欄を二重引用符で囲み、引用符を重ね、行を CRLF で区切り、先頭に BOM を置く", () => {
  const { text, guardedCount } = toCsv(
    ["a", "b"],
    [
      ['say "hi"', "x,y"],
      ["line1\nline2", ""],
    ],
  );
  expect(text).toBe(
    `${byteOrderMark}"a","b"\r\n"say ""hi""","x,y"\r\n"line1\nline2",""\r\n`,
  );
  expect(guardedCount).toBe(0);
});

test("表計算の式として読まれうる値の前に ' を付け、その個数を返す", () => {
  const { text, guardedCount } = toCsv(
    ["v"],
    [["=cmd()"], ["+1"], ["-enc"], ["@sum"], ["\tx"], ["plain"]],
  );
  expect(text).toContain(`"'=cmd()"`);
  expect(text).toContain(`"'-enc"`);
  expect(text).toContain(`"plain"`);
  expect(guardedCount).toBe(5);
});
