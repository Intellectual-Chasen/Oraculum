import { expect, test } from "vitest";
import { decodeCaseId } from "../contracts/cases";
import { formatEvidenceCountWithCases } from "./caseCounts";

test("案件ごとの件数を持つ根拠の件数は、先頭の合計に名前を付ける", () => {
  expect(
    formatEvidenceCountWithCases(12, [
      { caseId: decodeCaseId("baseline", "case"), evidenceCount: 10 },
      { caseId: decodeCaseId("challenge", "case"), evidenceCount: 2 },
    ]),
  ).toBe("合計: 12、baseline: 10、challenge: 2");
});

test("案件ごとの件数を持たない根拠の件数は、件数だけを返す", () => {
  expect(formatEvidenceCountWithCases(12, undefined)).toBe("12");
});
