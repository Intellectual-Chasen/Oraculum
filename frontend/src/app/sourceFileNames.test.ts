import { expect, test } from "vitest";
import { fileNamesByContentOf } from "./sourceFileNames";

test("同じ内容の収集元の表示名を、取り込んだ順につなぐ。同じ表示名は 1 度だけ出す", () => {
  const names = fileNamesByContentOf([
    { contentSha256: "a".repeat(64), fileName: "x.log (case-a)" },
    { contentSha256: "b".repeat(64), fileName: "y.log" },
    { contentSha256: "a".repeat(64), fileName: "x.log (case-b)" },
    { contentSha256: "a".repeat(64), fileName: "x.log (case-b)" },
  ]);
  expect(names.get("a".repeat(64))).toBe("x.log (case-a)、x.log (case-b)");
  expect(names.get("b".repeat(64))).toBe("y.log");
});
