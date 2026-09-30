import { expect, test } from "vitest";
import { DecodeFailure } from "./decoding";
import { decodeTimeHistogramResponse } from "./timeHistogram";

const decode = (json: unknown) => decodeTimeHistogramResponse(json, "$");

const outside = {
  localTimeRecordCount: 0,
  undatedRecordCount: 0,
  spanningRecordCount: 0,
};

const valid = {
  start: "2031-10-08T00:00:00.000Z",
  stepMs: 1000,
  rows: [{ counts: [1, 0] }, { counts: [0, 2] }],
  ...outside,
};

test("行と区切りの幅と始まりの時刻を読み、行が無い応答は始まりを持たなくてよい", () => {
  expect(decode(valid).rows[1]?.counts).toEqual([0, 2]);
  const withoutRows = decode({
    stepMs: 0,
    rows: [],
    localTimeRecordCount: 4,
    undatedRecordCount: 2,
    spanningRecordCount: 0,
  });
  expect(withoutRows.localTimeRecordCount).toBe(4);
  expect(withoutRows.undatedRecordCount).toBe(2);
});

test("区切りに入れた行が無くても、区切りの始まりと幅と、収まらなかった件数を読む", () => {
  const spanning = decode({
    start: "2031-10-08T00:00:00.000Z",
    stepMs: 10,
    rows: [],
    ...outside,
    spanningRecordCount: 3,
  });
  expect(spanning.start).toBe("2031-10-08T00:00:00.000Z");
  expect(spanning.spanningRecordCount).toBe(3);
});

test.each([
  ["負の件数", { ...valid, rows: [{ counts: [-1, 0] }] }],
  ["整数でない件数", { ...valid, rows: [{ counts: [0.5, 0] }] }],
  ["数でない件数", { ...valid, rows: [{ counts: ["1", 0] }] }],
  ["始まりの無い行", { ...valid, start: undefined }],
  ["読めない始まり", { ...valid, start: "not a time" }],
  ["幅 0 の行", { ...valid, stepMs: 0 }],
  [
    "列の数が揃わない行",
    { ...valid, rows: [{ counts: [1] }, { counts: [0, 2] }] },
  ],
  ["地方時の件数の無い応答", { ...valid, localTimeRecordCount: undefined }],
  ["時刻の無い件数の無い応答", { ...valid, undatedRecordCount: undefined }],
  ["収まらない件数の無い応答", { ...valid, spanningRecordCount: undefined }],
  ["負の件数の欄", { ...valid, spanningRecordCount: -1 }],
])("契約に反する応答を読まない: %s", (_name, json) => {
  expect(() => decode(json)).toThrow(DecodeFailure);
});
