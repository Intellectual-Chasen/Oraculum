import { expect, test } from "vitest";
import { listKey, recordLocatorKey } from "./listKey";

test("区切りに使う文字を値が含んでも、別の組が同じ key にならない", () => {
  // 表示できる区切り文字を使うと、次の 2 組は同じ key になる。
  expect(listKey(["a/b", "c"])).not.toBe(listKey(["a", "b/c"]));
  expect(listKey(["a-b", "c"])).not.toBe(listKey(["a", "b-c"]));
});

test("値が無い項目を空の文字列にし、位置を保つ", () => {
  expect(listKey(["a", undefined, "b"])).toBe("a\0\0b");
  // 値が無い項目の位置が違う組は、別の key になる。
  expect(listKey(["a", undefined, "b"])).not.toBe(
    listKey([undefined, "a", "b"]),
  );
});

test("同じ組は同じ key になる", () => {
  expect(listKey(["a", 1, undefined])).toBe(listKey(["a", 1, undefined]));
});

test("byte 位置だけが異なるレコードは、別の key になる", () => {
  const ref = {
    sourceId: "src-1",
    sourceContentSha256: "0".repeat(64),
    sourceFileName: "a.bin",
    positionKind: "byte_range",
    recordRawTextRef: "/r",
  } as const;
  expect(recordLocatorKey({ ...ref, byteOffset: 0 })).not.toBe(
    recordLocatorKey({ ...ref, byteOffset: 64 }),
  );
});

test("数の項目を文字列へ直す", () => {
  expect(listKey([1, 2])).toBe("1\x002");
});
