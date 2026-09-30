import { expect, test } from "vitest";
import {
  instantDisplayText,
  localTextOf,
  periodUnjudgedParts,
} from "./timestampInstant";

test("期間で判定できないレコードの件数を、0 件の組を除いた値の組で書く", () => {
  expect(
    periodUnjudgedParts({ localRecordCount: 0, undatedRecordCount: 2 }),
  ).toEqual([{ name: "時刻なし", value: "2 件" }]);
  expect(
    periodUnjudgedParts({ localRecordCount: 1, undatedRecordCount: 3 }),
  ).toEqual([
    { name: "タイムゾーン不明", value: "1 件" },
    { name: "時刻なし", value: "3 件" },
  ]);
  expect(
    periodUnjudgedParts({ localRecordCount: 0, undatedRecordCount: 0 }),
  ).toEqual([]);
});

test("UTC の文字列を選んだずれの地方時の文字列にし、日付をまたぐ", () => {
  expect(localTextOf("2001-02-03T23:44:25Z", "+09:00")).toBe(
    "2001-02-04T08:44:25+09:00",
  );
  expect(localTextOf("2001-02-03T01:00:00.125Z", "-05:30")).toBe(
    "2001-02-02T19:30:00.125-05:30",
  );
});

test("ずれを選んでいないときと、時点の文字列でないときは地方時を返さない", () => {
  expect(localTextOf("2001-02-03T23:44:25Z", "")).toBeUndefined();
  expect(localTextOf("2001-02-03 23:44:25", "+09:00")).toBeUndefined();
});

test("epoch の ms の時点に、表示のタイムゾーンを選んだときだけその時刻の組を添える", () => {
  const ms = Date.parse("2001-02-03T23:44:25Z");
  expect(instantDisplayText(ms, "")).toBe("2001-02-03T23:44:25.000Z");
  expect(instantDisplayText(ms, "+09:00")).toBe(
    "2001-02-03T23:44:25.000Z、UTC+09:00: 2001-02-04T08:44:25.000+09:00",
  );
});
