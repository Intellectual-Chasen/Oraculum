import { expect, test } from "vitest";
import type { Timestamp } from "../contracts/common";
import {
  type GraphTimeFilter,
  instantInPeriod,
  matchRangesOf,
  periodJudgementOf,
  rangeOverlapsPeriod,
  searchHighlightOf,
} from "./searchHighlight";
import { noSearchTerms } from "./searchTerms";

const secondPeriod: GraphTimeFilter = {
  from: { text: "2031-10-08T10:20:30+09:00" },
  to: { text: "2031-10-08T10:20:35+09:00" },
  unit: "second",
};

function absolute(normalized: string): Timestamp {
  return {
    rawText: normalized,
    normalized,
    normalizedForm: "rfc3339_absolute",
    precision: "millisecond",
    offsetState: "in_value",
    clock: "terminal_local",
    meaning: "event",
    valueState: "present",
  };
}

const local: Timestamp = {
  rawText: "2031/10/08 10:20:32",
  normalized: "2031-10-08T10:20:32",
  normalizedForm: "local_without_offset",
  precision: "second",
  offsetState: "item_absent",
  clock: "terminal_local",
  meaning: "event",
  valueState: "present",
};

const partial: Timestamp = {
  rawText: "2031-10-08T10:20",
  normalized: "2031-10-08T10:20",
  normalizedForm: "partial_date_time",
  precision: "minute",
  offsetState: "item_absent",
  clock: "terminal_local",
  meaning: "event",
  valueState: "present",
};

test("秒の単位では、上端の秒の中の時点を期間の内とし、次の秒を外とする", () => {
  expect(instantInPeriod("2031-10-08T10:20:35.900+09:00", secondPeriod)).toBe(
    true,
  );
  expect(instantInPeriod("2031-10-08T01:20:36Z", secondPeriod)).toBe(false);
  expect(instantInPeriod("2031-10-08T10:20:30+09:00", secondPeriod)).toBe(true);
  expect(instantInPeriod("2031-10-08T10:20:29.999+09:00", secondPeriod)).toBe(
    false,
  );
});

test("ミリ秒の単位では、上端より 1 ms 後の時点を外とし、ms より下の桁は切り捨てる", () => {
  const period: GraphTimeFilter = {
    to: { text: "2031-10-08T10:20:35.500+09:00" },
    unit: "millisecond",
  };
  expect(instantInPeriod("2031-10-08T10:20:35.500999+09:00", period)).toBe(
    true,
  );
  expect(instantInPeriod("2031-10-08T10:20:35.501+09:00", period)).toBe(false);
});

test("地方時と部分精度の時刻は判定できず、ずれを与えた地方時は期間で判定する", () => {
  expect(periodJudgementOf(local, secondPeriod)).toBe("unjudged");
  expect(periodJudgementOf(partial, secondPeriod)).toBe("unjudged");
  expect(
    periodJudgementOf(
      { ...local, interpretation: { offset: "+09:00" } },
      secondPeriod,
    ),
  ).toBe("inside");
  expect(periodJudgementOf(local, undefined)).toBeUndefined();
  expect(
    periodJudgementOf(absolute("2031-10-08T02:00:00Z"), secondPeriod),
  ).toBe("outside");
});

test("適用期間は、両端を含めて期間と重なるときだけ重なる", () => {
  const range = (from: string, to: string) => ({
    from: absolute(from),
    to: absolute(to),
  });
  expect(
    rangeOverlapsPeriod(
      range("2031-10-08T01:20:35.900Z", "2031-10-08T02:00:00Z"),
      secondPeriod,
    ),
  ).toBe(true);
  expect(
    rangeOverlapsPeriod(
      range("2031-10-08T01:20:36Z", "2031-10-08T02:00:00Z"),
      secondPeriod,
    ),
  ).toBe(false);
  expect(
    rangeOverlapsPeriod(
      { from: local, to: absolute("2031-10-08T02:00:00Z") },
      secondPeriod,
    ),
  ).toBe(false);
  expect(
    rangeOverlapsPeriod(
      range("2031-10-08T01:20:35Z", "2031-10-08T02:00:00Z"),
      undefined,
    ),
  ).toBe(false);
});

test("ASCII の文字列は大文字と小文字をそろえ、tab を含む値でも原文の位置を返す", () => {
  const highlight = searchHighlightOf(
    { ...noSearchTerms, contains: ["run"] },
    undefined,
  );
  expect(matchRangesOf(highlight, "a\tRUN b run")).toEqual([
    { start: 2, end: 5 },
    { start: 8, end: 11 },
  ]);
});

test("ASCII の外の文字を含む文字列は、完全に一致した部分だけを返す", () => {
  const highlight = searchHighlightOf(
    { ...noSearchTerms, contains: ["İx", "ß"] },
    undefined,
  );
  // `İ` は toLowerCase で 2 文字になる。大小の違う `ix` には当てない。
  expect(matchRangesOf(highlight, "İx ix ẞ ß")).toEqual([
    { start: 0, end: 2 },
    { start: 8, end: 9 },
  ]);
});

test("欄の指定がある文字列と欄の組は、その欄の値だけに当てる", () => {
  const highlight = searchHighlightOf(
    {
      ...noSearchTerms,
      contains: ["cmd"],
      field: "process.command_line",
      fieldContains: ["NewProcessName=exe"],
      fieldEquals: ["user.name=Alice"],
    },
    undefined,
  );
  expect(matchRangesOf(highlight, "cmd.exe")).toEqual([]);
  expect(
    matchRangesOf(highlight, "cmd.exe", {
      name: "CommandLine",
      semantic: "process.command_line",
    }),
  ).toEqual([{ start: 0, end: 3 }]);
  expect(
    matchRangesOf(highlight, "cmd.exe", {
      name: "EventData.NewProcessName",
    }),
  ).toEqual([{ start: 4, end: 7 }]);
  expect(
    matchRangesOf(highlight, "alice", { name: "u", semantic: "user.name" }),
  ).toEqual([{ start: 0, end: 5 }]);
  expect(
    matchRangesOf(highlight, "alice2", { name: "u", semantic: "user.name" }),
  ).toEqual([]);
});

test("重なる一致を 1 つの範囲にまとめ、検索式と除く文字列には当てない", () => {
  const highlight = searchHighlightOf(
    {
      contains: ["abc", "bcd"],
      excludes: ["e"],
      expression: 'x = "e"',
    },
    undefined,
  );
  expect(matchRangesOf(highlight, "abcde")).toEqual([{ start: 0, end: 4 }]);
});
