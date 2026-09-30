import { expect, test } from "vitest";
import {
  decodeObservationKind,
  decodeRecordField,
  decodeRecordLocator,
  decodeTimestamp,
} from "./common";

test("所見を持たない時刻の解釈を、所見の識別子を持たない解釈として読む", () => {
  const decoded = decodeTimestamp(
    {
      rawText: "2001/02/03 04:05:06",
      normalized: "2001-02-03T04:05:06",
      normalizedForm: "local_without_offset",
      precision: "second",
      offsetState: "item_absent",
      clock: "terminal_local",
      meaning: "event",
      valueState: "present",
      interpretation: { offset: "+09:00" },
    },
    "$",
  );
  expect(decoded.interpretation).toEqual({
    offset: "+09:00",
    assertionId: undefined,
  });
});

// 期待する項目は `backend/core/value_state.go` の `RecordField` が定める。

const byteRangeLocator = {
  sourceId: "source-auditd",
  sourceContentSha256: "c".repeat(64),
  sourceFileName: "host-g.log",
  positionKind: "byte_range",
  lineNumber: 2048,
  lineCount: 4,
  byteOffset: 7321,
  byteLength: 300,
  recordRawTextRef: "/api/v0/records",
} as const;

test("byte 範囲で指すレコードの位置を読む", () => {
  expect(decodeRecordLocator(byteRangeLocator, "$")).toEqual(byteRangeLocator);
});

test("byte 位置を持たない byte 範囲のレコードの位置を読まない", () => {
  const { byteOffset: _dropped, ...withoutOffset } = byteRangeLocator;
  expect(() => decodeRecordLocator(withoutOffset, "$")).toThrowError(
    "$.byteOffset",
  );
});

const textValue = {
  rawText: "203.0.113.21",
  valueState: "present",
} as const;

test("語彙の項目を持つ 1 項目から semantic を読む", () => {
  const field = decodeRecordField(
    {
      name: "dstIP",
      semantic: "connection.destination_address",
      kind: "text",
      text: textValue,
    },
    "$",
  );

  expect(field).toEqual({
    name: "dstIP",
    semantic: "connection.destination_address",
    kind: "text",
    text: { rawText: "203.0.113.21", valueState: "present" },
  });
});

test("semantic を持たない 1 項目を読み、項目を補わない", () => {
  const field = decodeRecordField(
    { name: "squidStatus", kind: "text", text: textValue },
    "$",
  );

  expect(field).toEqual({
    name: "squidStatus",
    kind: "text",
    text: { rawText: "203.0.113.21", valueState: "present" },
  });
  expect("semantic" in field).toBe(false);
});

test("時刻を持つ 1 項目からも semantic を読む", () => {
  const field = decodeRecordField(
    {
      name: "headerTime",
      semantic: "event.time",
      kind: "timestamp",
      timestamp: {
        rawText: "10/08/2031 10:20:35.100 +0900",
        normalized: "2031-10-08T10:20:35.100+09:00",
        normalizedForm: "rfc3339_absolute",
        precision: "millisecond",
        offsetState: "in_value",
        offsetText: "+0900",
        clock: "terminal_local",
        meaning: "event",
        valueState: "present",
      },
    },
    "$",
  );

  expect(field.semantic).toBe("event.time");
});

test("semantic に文字列以外を持つ組を読まない", () => {
  expect(() =>
    decodeRecordField(
      { name: "dstIP", semantic: 7, kind: "text", text: textValue },
      "$",
    ),
  ).toThrowError("$.semantic");
});

// 意味の状態と推定した意味の対応は `backend/core/observation_kind.go` の
// `validateMeaning` が定める。`meaning` が出るのは `inferred` のときだけであり、
// `inferred` では必須である。

const netKindRaw = (subEvt: string) => [
  {
    name: "evt",
    kind: "text",
    text: { rawText: "net", valueState: "present" },
  },
  {
    name: "subEvt",
    kind: "text",
    text: { rawText: subEvt, valueState: "present" },
  },
];

const inferredMeaning = "TCP 接続が確立したときに出力される";

test("推定した種別から意味を読む", () => {
  const kind = decodeObservationKind(
    { raw: netKindRaw("est"), status: "inferred", meaning: inferredMeaning },
    "$",
  );

  expect(kind.status).toBe("inferred");
  expect(kind.meaning).toBe(inferredMeaning);
});

test("意味を持たない推定した種別を読まない", () => {
  expect(() =>
    decodeObservationKind({ raw: netKindRaw("est"), status: "inferred" }, "$"),
  ).toThrowError("$.meaning");
});

test("仕様書が定めた種別に意味がある組を読まない", () => {
  expect(() =>
    decodeObservationKind(
      {
        raw: netKindRaw("con"),
        status: "determined",
        meaning: inferredMeaning,
      },
      "$",
    ),
  ).toThrowError("$.meaning");
});

test("意味を確定できない種別から意味を読まない", () => {
  const kind = decodeObservationKind(
    { raw: netKindRaw("exampleSubEvent"), status: "undetermined" },
    "$",
  );

  expect(kind.status).toBe("undetermined");
  expect(kind.meaning).toBeUndefined();
});

// 状態の欄が出ない応答は、意味の欄も持たない。
// 種別の欄を持たない入力形式と、文字列に `present` 以外がある種別が該当する。

const absentKindRaw = [
  {
    name: "evt",
    kind: "text",
    text: { valueState: "item_absent" },
  },
];

test("状態の欄が出ない応答を、状態も意味も持たない組として読む", () => {
  const kind = decodeObservationKind({ raw: absentKindRaw }, "$");

  expect(kind.status).toBeUndefined();
  expect(kind.meaning).toBeUndefined();
});

test("状態の欄が出ない応答に意味がある組を読まない", () => {
  expect(() =>
    decodeObservationKind(
      { raw: absentKindRaw, meaning: inferredMeaning },
      "$",
    ),
  ).toThrowError("$.meaning");
});

test("種別の欄を持たない入力形式の応答に意味がある組を読まない", () => {
  expect(() =>
    decodeObservationKind({ raw: [], meaning: inferredMeaning }, "$"),
  ).toThrowError("$.meaning");
});
