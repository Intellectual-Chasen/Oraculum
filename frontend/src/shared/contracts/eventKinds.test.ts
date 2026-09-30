import { expect, test } from "vitest";
import { eventKindsResponseJson } from "@/testdata/eventKinds/eventKindsResponse";
import { DecodeFailure } from "./decoding";
import { decodeEventKindsResponse } from "./eventKinds";

test("事象の種別の一覧を読み、動作を持たない組は動作を出さない", () => {
  const json = eventKindsResponseJson();
  const decoded = decodeEventKindsResponse(
    {
      ...json,
      kinds: [...json.kinds, { category: "session", recordCount: 1 }],
      terminal: "n:terminal:01",
    },
    "response",
  );

  expect(decoded.kinds.map((kind) => [kind.category, kind.action])).toEqual([
    ...json.kinds.map((kind) => [kind.category, kind.action]),
    ["session", undefined],
  ]);
  expect("action" in (decoded.kinds.at(-1) ?? {})).toBe(false);
  // windowsEvent を持たない組は、Windows イベントログの組でない。
  expect(decoded.kinds.every((kind) => kind.windowsEvent === false)).toBe(true);
  expect(decoded.uncategorizedRecordCount).toBe(json.uncategorizedRecordCount);
  expect(decoded.terminal).toBe("n:terminal:01");
  expect("case" in decoded).toBe(false);
});

test("windowsEvent は真偽だけを受け取る", () => {
  const json = eventKindsResponseJson();
  const kind = { category: "Example-Provider", action: "42", recordCount: 1 };

  expect(
    decodeEventKindsResponse(
      { ...json, kinds: [{ ...kind, windowsEvent: true }] },
      "response",
    ).kinds[0]?.windowsEvent,
  ).toBe(true);
  expect(() =>
    decodeEventKindsResponse(
      { ...json, kinds: [{ ...kind, windowsEvent: "true" }] },
      "response",
    ),
  ).toThrow(DecodeFailure);
});

test("案件は識別子の規則に合う文字列だけを受け取る", () => {
  const json = eventKindsResponseJson();

  expect(
    decodeEventKindsResponse({ ...json, case: "baseline" }, "response").case,
  ).toBe("baseline");
  expect(() =>
    decodeEventKindsResponse({ ...json, case: "base line" }, "response"),
  ).toThrow(DecodeFailure);
});

test("同じ分類と動作の組を 2 回含む応答と、件数が 0 の組を退ける", () => {
  const json = eventKindsResponseJson();
  const [first] = json.kinds;
  expect(() =>
    decodeEventKindsResponse(
      { ...json, kinds: [...json.kinds, first] },
      "response",
    ),
  ).toThrow(DecodeFailure);
  expect(() =>
    decodeEventKindsResponse(
      { ...json, kinds: [{ category: "ps", action: "start", recordCount: 0 }] },
      "response",
    ),
  ).toThrow(DecodeFailure);
});
