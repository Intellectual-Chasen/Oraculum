import { expect, test } from "vitest";
import type { RecordField, Timestamp } from "../contracts/common";
import { msRangeTimeFilter } from "./timeFilter";
import { conditionLabel, fieldConditions, periodText } from "./valueCondition";

function text(
  name: string,
  rawText: string | undefined,
  semantic?: string,
  normalized?: string,
): RecordField {
  return {
    name,
    ...(semantic === undefined ? {} : { semantic }),
    kind: "text",
    text:
      rawText === undefined
        ? { valueState: "item_absent" }
        : {
            rawText,
            ...(normalized === undefined
              ? {}
              : { normalized, derivation: "trim" }),
            valueState: "present",
          },
  };
}

function timestamp(overrides: Partial<Timestamp>): RecordField {
  return {
    name: "LoggedAt",
    kind: "timestamp",
    timestamp: {
      rawText: "2031/04/05 06:07:08",
      normalized: "2031-04-05T06:07:08+09:00",
      normalizedForm: "rfc3339_absolute",
      precision: "second",
      offsetState: "in_value",
      clock: "terminal_local",
      meaning: "event",
      valueState: "present",
      ...overrides,
    },
  };
}

test("一般のフィールドは、等しい・含む・全体の含む・含まないの順に原文の条件を作る", () => {
  expect(fieldConditions(text("User", " user-a "))).toEqual([
    { kind: "field", field: "User", text: " user-a ", whole: true },
    { kind: "field", field: "User", text: " user-a ", whole: false },
    { kind: "text", mode: "contains", text: " user-a " },
    { kind: "text", mode: "excludes", text: " user-a " },
  ]);
});

test("名前が = を含むフィールドと原文を持たないフィールドは、作れない条件を返さない", () => {
  expect(fieldConditions(text("a=b", "v")).map((c) => c.kind)).toEqual([
    "text",
    "text",
  ]);
  expect(fieldConditions(text("User", undefined))).toEqual([]);
});

test("時刻のフィールドは、時刻の値の精度とずれのまま期間の条件を先にする", () => {
  const [first] = fieldConditions(timestamp({}));
  expect(first).toEqual({
    kind: "period",
    filter: {
      from: { text: "2031-04-05T06:07:08+09:00", precision: "second" },
      to: { text: "2031-04-05T06:07:08+09:00", precision: "second" },
      unit: "second",
    },
  });
});

test.each([
  ["ずれが定まらない", { offsetState: "undetermined" as const }],
  ["秒より粗い精度", { precision: "minute" as const }],
])("%s時刻には期間の条件を作らない", (_, overrides) => {
  expect(
    fieldConditions(timestamp(overrides)).some((c) => c.kind === "period"),
  ).toBe(false);
});

test("イベントの種類を決めるフィールドだけが、応答の組のままイベントの種類の条件を先に持つ", () => {
  const kind = { category: "synthetic-provider", action: "9001" };
  expect(
    fieldConditions(text("EventID", "9001", "windows_event.id"), kind)[0],
  ).toEqual({ kind: "eventKind", ...kind });
  expect(
    fieldConditions(text("User", "user-a"), kind).some(
      (c) => c.kind === "eventKind",
    ),
  ).toBe(false);
  // 組を持たないレコードには、イベントの種類の条件を作らない。
  expect(
    fieldConditions(text("EventID", "9001", "windows_event.id")).some(
      (c) => c.kind === "eventKind",
    ),
  ).toBe(false);
});

test("終わりを含まない ms の区切りは、終わりの 1 ms 前までの期間にし、区間の文字列で写す", () => {
  const start = Date.UTC(2031, 3, 5, 6, 0, 0);
  const filter = msRangeTimeFilter(start, start + 60_000);
  expect(filter).toEqual({
    from: { text: "2031-04-05T06:00:00.000Z", precision: "millisecond" },
    to: { text: "2031-04-05T06:00:59.999Z", precision: "millisecond" },
    unit: "millisecond",
  });
  expect(periodText(filter)).toBe(
    "2031-04-05T06:00:00.000Z/2031-04-05T06:00:59.999Z",
  );
});

test("条件ごとにメニューの項目の名前を持つ", () => {
  expect(
    fieldConditions(timestamp({}))
      .concat([{ kind: "eventKind", category: "c", action: "a" }])
      .map(conditionLabel),
  ).toEqual([
    "期間の条件に追加",
    "フィールドの値が等しい条件に追加",
    "フィールドの値が文字列を含む条件に追加",
    "含む文字列に追加",
    "含まない文字列に追加",
    "イベントの種類の条件に追加",
  ]);
});
