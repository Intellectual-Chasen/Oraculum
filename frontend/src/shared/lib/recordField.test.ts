import { expect, test } from "vitest";
import {
  describeNormalizedAbsence,
  describeRawTextAbsence,
  readRecordFieldNormalized,
  readRecordFieldRawText,
  windowsEventIdOf,
} from "./recordField";

// 期待するラベルは `backend/core/value_state.go` の `ValueState` と、状態ごとに原文と
// 正規化値が出る条件から決める。

test("原文を出さない 3 状態に、状態ごとの理由のラベルを返す", () => {
  expect(describeRawTextAbsence("item_absent")).toBe("フィールドなし");
  expect(describeRawTextAbsence("derived")).toBe("別の収集元から求めた値");
  expect(describeRawTextAbsence("derivation_undetermined")).toBe(
    "値を求められない",
  );
});

test("原文を出す 4 状態に、原文が無いことのラベルを返す", () => {
  for (const valueState of [
    "present",
    "absent",
    "no_body",
    "out_of_definition",
  ] as const) {
    expect(describeRawTextAbsence(valueState)).toBe("原文なし");
  }
});

test("正規化値を出さない 3 状態に、状態ごとの理由のラベルを返す", () => {
  expect(describeNormalizedAbsence("item_absent")).toBe("フィールドなし");
  expect(describeNormalizedAbsence("derivation_undetermined")).toBe(
    "値を求められない",
  );
  expect(describeNormalizedAbsence("derived")).toBe("求めた値なし");
});

test("値があり正規化値を持たないフィールドに、原文で比べることのラベルを返す", () => {
  expect(describeNormalizedAbsence("present")).toBe("原文で比較");
});

test("値の無い 3 状態に、値が無いことのラベルを返す", () => {
  for (const valueState of [
    "absent",
    "no_body",
    "out_of_definition",
  ] as const) {
    expect(describeNormalizedAbsence(valueState)).toBe("値なし");
  }
});

test("文字列を持つ項目から原資料の文字列と正規化値を読む", () => {
  const field = {
    name: "clientIp",
    kind: "text",
    text: {
      rawText: "192.0.2.101",
      normalized: "192.0.2.101",
      derivation: "文字列をそのまま用いる",
      valueState: "present",
    },
  } as const;

  expect(readRecordFieldRawText(field)).toEqual({ text: "192.0.2.101" });
  expect(readRecordFieldNormalized(field)).toEqual({ text: "192.0.2.101" });
});

test("時刻の項目から原資料の文字列と正規化値を読む", () => {
  const field = {
    name: "eventTime",
    kind: "timestamp",
    timestamp: {
      rawText: "[08/Oct/2031:10:20:35 +0900]",
      normalized: "2031-10-08T10:20:35+09:00",
      normalizedForm: "rfc3339_absolute",
      precision: "second",
      offsetState: "in_value",
      clock: "observer_local",
      meaning: "event",
      valueState: "present",
    },
  } as const;

  expect(readRecordFieldRawText(field)).toEqual({
    text: "[08/Oct/2031:10:20:35 +0900]",
  });
  expect(readRecordFieldNormalized(field)).toEqual({
    text: "2031-10-08T10:20:35+09:00",
  });
});

test("導いた値の項目は原資料の文字列を欠測として読み、正規化値を読む", () => {
  const field = {
    name: "clientTerminal",
    kind: "text",
    text: {
      normalized: "PC-0042",
      derivation: "割当を適用した",
      valueState: "derived",
    },
  } as const;

  expect(readRecordFieldRawText(field)).toEqual({
    absence: "別の収集元から求めた値",
  });
  expect(readRecordFieldNormalized(field)).toEqual({ text: "PC-0042" });
});

test("入力形式にフィールドが無いときは原文と正規化値の両方をフィールドなしで読む", () => {
  const field = {
    name: "subEvt",
    kind: "text",
    text: { valueState: "item_absent" },
  } as const;

  expect(readRecordFieldRawText(field)).toEqual({ absence: "フィールドなし" });
  expect(readRecordFieldNormalized(field)).toEqual({
    absence: "フィールドなし",
  });
});

test("Windows イベントログのレコードの Event ID は、プロバイダと Event ID の両方を持つときだけ読む", () => {
  const text = (name: string, semantic: string, value: string) =>
    ({
      name,
      semantic,
      kind: "text",
      text: { rawText: value, valueState: "present" },
    }) as const;
  const provider = text("Provider", "windows_event.provider", "Example-Log");
  const eventId = text("EventID", "windows_event.id", "4624");
  expect(windowsEventIdOf([provider, eventId])).toBe("4624");
  expect(windowsEventIdOf([eventId])).toBeUndefined();
  // 事象の分類を持つレコードは、Windows イベントログのレコードとして扱わない。
  expect(
    windowsEventIdOf([
      text("category", "event.category", "logon"),
      provider,
      eventId,
    ]),
  ).toBeUndefined();
});
