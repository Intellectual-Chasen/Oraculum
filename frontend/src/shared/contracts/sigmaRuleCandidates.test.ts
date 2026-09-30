import { expect, test } from "vitest";
import { sigmaCandidatesResponse } from "@/testdata/sigmaRuleCandidates/candidateResponse";
import { DecodeFailure } from "./decoding";
import { decodeSigmaRuleCandidatesResponse } from "./sigmaRuleCandidates";

function wireValue(value: unknown): unknown {
  return JSON.parse(JSON.stringify(value));
}

test("一致したルールと評価しなかったルールを読む", () => {
  const decoded = decodeSigmaRuleCandidatesResponse(
    wireValue(sigmaCandidatesResponse()),
    "response",
  );
  expect(decoded).toEqual(sigmaCandidatesResponse());
});

// backend の pipeline の test (emptyChannelGroupsJSON) が、空の Channel、空の Provider、
// どちらの欄も無いレコードから組む文字列と同じである。
const emptyChannelGroupsJSON =
  '[{"channel":"","recordCount":2},{"provider":"","recordCount":1},' +
  '{"channelAndProviderAbsent":true,"recordCount":1}]';

test("空の Channel と空の Provider とどちらも無いレコードの組を読み分ける", () => {
  const decoded = decodeSigmaRuleCandidatesResponse(
    {
      ...(wireValue(sigmaCandidatesResponse()) as object),
      unevaluatedRecordGroups: JSON.parse(emptyChannelGroupsJSON),
    },
    "response",
  );
  expect(decoded.unevaluatedRecordGroups).toEqual([
    { channel: "", recordCount: 2 },
    { provider: "", recordCount: 1 },
    { channelAndProviderAbsent: true, recordCount: 1 },
  ]);
  // どの分け方も持たない組を退ける。
  expect(() =>
    decodeSigmaRuleCandidatesResponse(
      {
        ...(wireValue(sigmaCandidatesResponse()) as object),
        unevaluatedRecordGroups: [{ recordCount: 1 }],
      },
      "response",
    ),
  ).toThrowError(DecodeFailure);
});

test("ルールの集合を渡していない応答は ruleSet を持たない", () => {
  const { ruleSet: _, ...withoutRuleSet } = sigmaCandidatesResponse();
  const decoded = decodeSigmaRuleCandidatesResponse(
    wireValue({ ...withoutRuleSet, rules: [], matches: [] }),
    "response",
  );
  expect(decoded.ruleSet).toBeUndefined();
});

test("一覧に無いルールを指す一致と、一致の数と食い違う件数を退ける", () => {
  const response = sigmaCandidatesResponse();
  const orphan = wireValue({
    ...response,
    matches: [
      ...response.matches,
      { ...response.matches[0], rulePath: "unknown.yml" },
    ],
  });
  expect(() =>
    decodeSigmaRuleCandidatesResponse(orphan, "response"),
  ).toThrowError(DecodeFailure);
  const miscounted = wireValue({
    ...response,
    matches: response.matches.slice(0, 1),
  });
  expect(() =>
    decodeSigmaRuleCandidatesResponse(miscounted, "response"),
  ).toThrowError(DecodeFailure);
});

test("知らない理由の分類を退ける", () => {
  const response = sigmaCandidatesResponse();
  const unknown = wireValue({
    ...response,
    unevaluatedRules: [{ ...response.unevaluatedRules[0], reason: "other" }],
  });
  expect(() =>
    decodeSigmaRuleCandidatesResponse(unknown, "response"),
  ).toThrowError(DecodeFailure);
});

test("ルールの集合と一致の食い違いを退ける", () => {
  const response = sigmaCandidatesResponse();
  const [rule] = response.rules;
  const [match] = response.matches;
  if (
    rule === undefined ||
    match === undefined ||
    response.ruleSet === undefined
  ) {
    throw new Error("synthetic rule and match are required");
  }
  const rejected: Record<string, unknown> = {
    "the selection of another rule": {
      ...response,
      matches: [
        { ...match, matchedSelections: ["unknown"] },
        ...response.matches.slice(1),
      ],
    },
    "duplicated selection names": {
      ...response,
      rules: [
        { ...rule, selections: [...rule.selections, rule.selections[0]] },
      ],
    },
    "an unknown revision source": {
      ...response,
      ruleSet: { ...response.ruleSet, revisionSource: "remote" },
    },
    "duplicated rule paths": {
      ...response,
      rules: [rule, { ...rule, matchCount: 0 }],
    },
    "a group with both channel and provider": {
      ...response,
      unevaluatedRecordGroups: [
        { channel: "Example/Operational", provider: "Example", recordCount: 1 },
      ],
    },
  };
  for (const [name, value] of Object.entries(rejected)) {
    expect(
      () => decodeSigmaRuleCandidatesResponse(wireValue(value), "response"),
      name,
    ).toThrowError(DecodeFailure);
  }
});
