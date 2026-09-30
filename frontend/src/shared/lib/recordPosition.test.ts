import { expect, test } from "vitest";
import {
  accessLogSha256,
  accessLogSourceId,
} from "@/testdata/sources/sourcesResponse";
import type { RecordLocator, RecordRange } from "../contracts/common";
import {
  describeRecordPosition,
  describeRecordRange,
  isPositionUsable,
  recordPositionParts,
  recordPositionValue,
  recordRefKey,
} from "./recordPosition";

function locator(overrides: Partial<RecordLocator>): RecordLocator {
  return {
    sourceId: accessLogSourceId,
    sourceContentSha256: accessLogSha256,
    sourceFileName: "access.log",
    positionKind: "line_number",
    lineNumber: 37,
    recordRawTextRef: "/api/v0/records?lineNumber=37",
    ...overrides,
  };
}

function range(overrides: Partial<RecordRange>): RecordRange {
  return {
    sourceId: accessLogSourceId,
    sourceContentSha256: accessLogSha256,
    rangeKind: "positioned",
    positionKind: "line_number",
    fromPosition: 1,
    toPosition: 1200,
    ...overrides,
  };
}

test("行番号で指すレコードの位置を行番号で書く", () => {
  expect(describeRecordPosition(locator({}))).toBe("行: 37");
});

test("byte 範囲で指すレコードの位置を、先頭の行と行数と byte 範囲で書く", () => {
  const recordRef = locator({
    positionKind: "byte_range",
    lineNumber: 2048,
    lineCount: 4,
    byteOffset: 7321,
    byteLength: 300,
  });
  expect(describeRecordPosition(recordRef)).toBe(
    "行: 2048-2051、位置: 7321-7621",
  );
});

test("1 行だけのレコードの行は、番号だけを書く", () => {
  const recordRef = locator({
    positionKind: "byte_range",
    lineNumber: 30,
    lineCount: 1,
    byteOffset: 900,
    byteLength: 80,
  });
  expect(describeRecordPosition(recordRef)).toBe("行: 30、位置: 900-980");
});

test("行を数えていない byte 範囲のレコードの位置を byte 範囲だけで書く", () => {
  const recordRef = locator({
    positionKind: "byte_range",
    lineNumber: undefined,
    byteOffset: 7321,
    byteLength: 300,
  });
  expect(describeRecordPosition(recordRef)).toBe("位置: 7321-7621");
});

test("byte 範囲の長さを持たないレコードの位置を、位置なしで表す", () => {
  const recordRef = locator({ positionKind: "byte_range", byteOffset: 7321 });
  expect(recordPositionParts(recordRef)).toEqual([]);
  expect(describeRecordPosition(recordRef)).toBe("位置なし");
});

test("位置を ID・行・位置の組で返し、行の終わりは最後の行、byte の範囲の終わりは始まりに長さを足した値にする", () => {
  expect(
    recordPositionParts(
      locator({
        positionKind: "byte_range",
        lineNumber: 12,
        lineCount: 6,
        byteOffset: 634014,
        byteLength: 2360,
      }),
    ),
  ).toEqual([
    { name: "行", value: "12-17" },
    { name: "位置", value: "634014-636374" },
  ]);
  expect(
    recordPositionParts(
      locator({
        positionKind: "sequence_number",
        sequenceNumber: 4096,
        lineNumber: 1022,
      }),
    ),
  ).toEqual([
    { name: "ID", value: "4096" },
    { name: "行", value: "1022" },
  ]);
  expect(recordPositionParts(locator({}))).toEqual([
    { name: "行", value: "37" },
  ]);
});

test("byte 位置だけを持つ位置を要求に載せる", () => {
  expect(isPositionUsable({ byteOffset: 7321 })).toBe(true);
  expect(isPositionUsable({ byteOffset: 0 })).toBe(true);
  expect(isPositionUsable({ byteOffset: -1 })).toBe(false);
  expect(isPositionUsable({})).toBe(false);
});

test("先頭の行が同じ 2 つの事象は別の key になる", () => {
  const first = locator({
    positionKind: "byte_range",
    lineNumber: 2048,
    byteOffset: 7321,
    byteLength: 300,
  });
  const second = locator({
    positionKind: "byte_range",
    lineNumber: 2048,
    byteOffset: 7621,
    byteLength: 120,
  });
  expect(recordRefKey(first)).not.toBe(recordRefKey(second));
});

test("通番で指すレコードの位置を通番と行番号で書く", () => {
  const recordRef = locator({
    positionKind: "sequence_number",
    sequenceNumber: 4096,
    lineNumber: 1022,
  });

  expect(describeRecordPosition(recordRef)).toBe("ID: 4096、行: 1022");
});

test("行番号を数えていない通番のレコードの位置を通番だけで書く", () => {
  const recordRef = locator({
    positionKind: "sequence_number",
    sequenceNumber: 4096,
    lineNumber: undefined,
  });

  expect(describeRecordPosition(recordRef)).toBe("ID: 4096");
});

test("位置の値が無いレコードの位置を位置なしで書く", () => {
  expect(describeRecordPosition(locator({ lineNumber: undefined }))).toBe(
    "位置なし",
  );
  expect(
    describeRecordPosition(
      locator({ positionKind: "sequence_number", sequenceNumber: undefined }),
    ),
  ).toBe("位置なし");
});

test("位置を指定しない範囲と、両端を持つ範囲を別のラベルで書く", () => {
  expect(describeRecordRange(range({ rangeKind: "whole_source" }))).toBe(
    "収集元のすべて",
  );
  expect(describeRecordRange(range({}))).toBe("行: 1-1200");
  expect(describeRecordRange(range({ toPosition: undefined }))).toBe(
    "範囲なし",
  );
});

test("表の列に出す位置の値は、名前を付けずに値だけを返し、値が無いときは undefined を返す", () => {
  expect(
    recordPositionValue(
      locator({
        positionKind: "byte_range",
        byteOffset: 634014,
        byteLength: 2360,
      }),
    ),
  ).toBe("634014-636374");
  expect(
    recordPositionValue(
      locator({ positionKind: "sequence_number", sequenceNumber: 4096 }),
    ),
  ).toBe("4096");
  expect(recordPositionValue(locator({}))).toBe("37");
  expect(
    recordPositionValue(locator({ positionKind: "byte_range" })),
  ).toBeUndefined();
});
