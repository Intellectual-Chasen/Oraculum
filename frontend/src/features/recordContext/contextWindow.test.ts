import { expect, test } from "vitest";
import type { RecordField, Timestamp } from "@/shared/contracts/common";
import { contextTimeFilter } from "@/shared/lib/timeFilter";
import { findEventTime } from "./contextWindow";

function timestamp(overrides: Partial<Timestamp>): Timestamp {
  return {
    rawText: "2031/10/08 10:20:35.100",
    normalized: "2031-10-08T10:20:35.100+09:00",
    normalizedForm: "rfc3339_absolute",
    precision: "millisecond",
    offsetState: "in_value",
    clock: "terminal_local",
    meaning: "event",
    valueState: "present",
    ...overrides,
  };
}

test("ミリ秒の時刻の前後を、同じずれのミリ秒の文字列で返す", () => {
  expect(contextTimeFilter(timestamp({}), 60)).toEqual({
    from: { text: "2031-10-08T10:19:35.100+09:00", precision: "millisecond" },
    to: { text: "2031-10-08T10:21:35.100+09:00", precision: "millisecond" },
    unit: "millisecond",
  });
});

test("秒の時刻は秒の文字列で返し、日付をまたぐ", () => {
  const filter = contextTimeFilter(
    timestamp({
      normalized: "2031-10-08T23:59:55Z",
      precision: "second",
    }),
    10,
  );
  expect(filter).toEqual({
    from: { text: "2031-10-08T23:59:45Z", precision: "second" },
    to: { text: "2031-10-09T00:00:05Z", precision: "second" },
    unit: "second",
  });
});

test("マイクロ秒の時刻はミリ秒へ切り捨て、負のずれを保つ", () => {
  const filter = contextTimeFilter(
    timestamp({
      normalized: "2031-10-08T06:12:47.437789-05:30",
      precision: "microsecond",
    }),
    300,
  );
  expect(filter).toEqual({
    from: { text: "2031-10-08T06:07:47.437-05:30", precision: "millisecond" },
    to: { text: "2031-10-08T06:17:47.437-05:30", precision: "millisecond" },
    unit: "millisecond",
  });
});

test("入力形式の定義で UTC に定まる時刻は、UTC の文字列で期間を返す", () => {
  const filter = contextTimeFilter(
    timestamp({
      normalized: "2021-10-05T04:59:11Z",
      precision: "second",
      offsetState: "format_defined",
    }),
    10,
  );
  expect(filter).toEqual({
    from: { text: "2021-10-05T04:59:01Z", precision: "second" },
    to: { text: "2021-10-05T04:59:21Z", precision: "second" },
    unit: "second",
  });
});

test("分析者のずれを与えた地方時は、そのずれの文字列で期間を返す", () => {
  const filter = contextTimeFilter(
    timestamp({
      normalized: "2031-10-08T10:20:35.100",
      normalizedForm: "local_without_offset",
      offsetState: "item_absent",
      interpretation: { offset: "+09:00", assertionId: "a-1" },
    }),
    10,
  );
  expect(filter).toEqual({
    from: { text: "2031-10-08T10:20:25.100+09:00", precision: "millisecond" },
    to: { text: "2031-10-08T10:20:45.100+09:00", precision: "millisecond" },
    unit: "millisecond",
  });
});

test("絶対時刻として比べられない時刻には期間を作らない", () => {
  const unreadable: Partial<Timestamp>[] = [
    { offsetState: "item_absent", normalizedForm: "local_without_offset" },
    { valueState: "absent" },
    { normalized: undefined },
    { precision: "minute", normalizedForm: "partial_date_time" },
    { normalized: "2021-13-45T99:00:00Z" },
  ];
  for (const overrides of unreadable) {
    expect(contextTimeFilter(timestamp(overrides), 60)).toBeUndefined();
  }
});

test("事象の時刻の項目を語彙で探す", () => {
  const eventTime = timestamp({});
  const fields: RecordField[] = [
    {
      name: "written",
      semantic: "record.output_time",
      kind: "timestamp",
      timestamp: timestamp({ rawText: "other" }),
    },
    { name: "evt", kind: "text", text: { valueState: "present" } },
    {
      name: "time",
      semantic: "event.time",
      kind: "timestamp",
      timestamp: eventTime,
    },
  ];
  expect(findEventTime(fields)).toBe(eventTime);
  expect(findEventTime(fields.slice(0, 2))).toBeUndefined();
});
