// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { expect, test } from "vitest";
import { KeyValueList, type KeyValuePair } from "@/shared/ui/KeyValueList";
import {
  candidateTallyPairs,
  type TimeWindowCondition,
  timeWindowPairs,
} from "./EdgeRecordPairs";

/** 値の無い組を除き、「名前: 値」の文字列に並べる。 */
function pairTexts(pairs: KeyValuePair[]): string[] {
  return pairs
    .filter((pair) => pair.value !== undefined)
    .map((pair) => `${pair.name}: ${String(pair.value)}`);
}

test.each([
  [
    "上位の候補を持つ候補",
    { candidateCount: 3, precedingCandidateCount: 1 },
    ["候補: 3", "上位の候補: 1"],
  ],
  [
    "最も上の順位の候補",
    { candidateCount: 3, precedingCandidateCount: 0 },
    ["候補: 3", "上位の候補: 0"],
  ],
])("%s の件数を組にする", (_name, tally, texts) => {
  expect(pairTexts(candidateTallyPairs(tally))).toEqual(texts);
});

const timeCondition: TimeWindowCondition = {
  conditionKey: "time_proximity",
  leftValue: [],
  rightValue: [],
  windowSeconds: 2,
};

test.each([
  [
    "後の差",
    { differenceSeconds: 0.25 },
    ["許容幅: ±2 秒", "時刻の差: +0.25 秒"],
  ],
  [
    "前の差",
    { differenceSeconds: -0.000521 },
    ["許容幅: ±2 秒", "時刻の差: −0.000521 秒"],
  ],
  ["差が 0", { differenceSeconds: 0 }, ["許容幅: ±2 秒", "時刻の差: 0 秒"]],
  [
    "丸めて 0 になる差",
    { differenceSeconds: -0.0000004 },
    ["許容幅: ±2 秒", "時刻の差: 0.000001 秒未満"],
  ],
  ["差が無い", {}, ["許容幅: ±2 秒", "時刻の差: —UTC 時刻にできない時刻"]],
  [
    "秒の単位で比べた差",
    {
      windowSeconds: 60,
      differenceSeconds: 60,
      differenceInWholeSeconds: true,
    },
    ["許容幅: ±60 秒", "時刻の差: +60 秒", "秒未満: 切り捨て"],
  ],
  [
    "秒の単位で比べた差が 0",
    { windowSeconds: 60, differenceSeconds: 0, differenceInWholeSeconds: true },
    ["許容幅: ±60 秒", "時刻の差: 1 秒未満", "秒未満: 切り捨て"],
  ],
])("%s を組にする", (_name, fields, texts) => {
  const { container } = render(
    <KeyValueList pairs={timeWindowPairs({ ...timeCondition, ...fields })} />,
  );
  expect(
    [...container.querySelectorAll("li")].map((item) => item.textContent),
  ).toEqual(texts);
  cleanup();
});
