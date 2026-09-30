import { expect, test } from "vitest";
import { investigationOrderResponseJson } from "@/testdata/investigationOrder/orderResponse";
import { DecodeFailure } from "./decoding";
import { decodeInvestigationOrderResponse } from "./investigationOrder";

test("種別ごとの並びと、値を持たない対象と、計算に使った入力を読む", () => {
  const decoded = decodeInvestigationOrderResponse(
    investigationOrderResponseJson(),
    "response",
  );
  expect(decoded.inputs.timeRange?.from.rawText).toBe(
    "2031-10-08T10:20:01+09:00",
  );
  const [process] = decoded.kinds;
  expect(process.kind).toBe("process");
  expect(process.entries.map((entry) => [entry.rank, entry.value])).toEqual([
    [1, 5],
    [1, 5],
    [3, null],
  ]);
});

test("時点を持つレコードが無い応答は期間を持たない", () => {
  const json = investigationOrderResponseJson();
  const { timeRange: _, ...inputs } = json.inputs;
  const decoded = decodeInvestigationOrderResponse(
    { ...json, inputs },
    "response",
  );
  expect(decoded.inputs.timeRange).toBeUndefined();
});

test("順位が 1 より小さい行と、数でない値を退ける", () => {
  const json = investigationOrderResponseJson();
  const withEntry = (entry: object) => ({
    ...json,
    kinds: [
      {
        ...json.kinds[0],
        entries: [{ ...json.kinds[0].entries[0], ...entry }],
      },
    ],
  });
  expect(() =>
    decodeInvestigationOrderResponse(withEntry({ rank: 0 }), "response"),
  ).toThrow(DecodeFailure);
  expect(() =>
    decodeInvestigationOrderResponse(withEntry({ value: "5" }), "response"),
  ).toThrow(DecodeFailure);
});
