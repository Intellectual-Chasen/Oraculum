import { expect, test } from "vitest";
import { matchedEdgeDetailResponseJson } from "@/testdata/graph/graphResponse";
import {
  decodeEdgeDetailResponse,
  edgeMatchTimeComparison,
} from "./graphDetail";

type MatchedJson = ReturnType<typeof matchedEdgeDetailResponseJson>;

// 段階と関連付けは表の位置を指す。範囲の外を指す応答は、指す先のレコードや段階を読めない。
test("表の範囲の外を指す関連付けの表を読まない", () => {
  const cases: [string, (json: MatchedJson) => unknown, RegExp][] = [
    [
      "起点",
      (json) => ({
        ...json,
        matchStages: [{ ...json.matchStages[0], origin: 3 }],
      }),
      /matchStages\[0\]\.origin/,
    ],
    [
      "区別できない候補",
      (json) => ({
        ...json,
        matchStages: [
          { ...json.matchStages[0], indistinguishableGroups: [[1, 3]] },
        ],
      }),
      /matchStages\[0\]\.indistinguishableGroups\[0\]\[1\]/,
    ],
    [
      "段階",
      (json) => ({
        ...json,
        matches: [{ stage: 1, candidate: 1, unresolvedReasons: 0 }],
      }),
      /matches\[0\]\.stage/,
    ],
    [
      "候補",
      (json) => ({
        ...json,
        matches: [{ stage: 0, candidate: 3, unresolvedReasons: 0 }],
      }),
      /matches\[0\]\.candidate/,
    ],
    [
      "確定しない理由",
      (json) => ({
        ...json,
        matches: [{ stage: 0, candidate: 1, unresolvedReasons: 1 }],
      }),
      /matches\[0\]\.unresolvedReasons/,
    ],
    [
      "空の理由",
      (json) => ({
        ...json,
        matchStages: [{ ...json.matchStages[0], unresolvedReasonSets: [[""]] }],
      }),
      /matchStages\[0\]\.unresolvedReasonSets\[0\]\[0\]/,
    ],
    [
      "関連付けが指さない段階",
      (json) => ({
        ...json,
        matchStages: [json.matchStages[0], json.matchStages[0]],
      }),
      /matchStages\[1\]/,
    ],
    [
      "段階が挙げた候補より多い関連付け",
      (json) => ({
        ...json,
        matchRecords: [...json.matchRecords, json.matchRecords[1]],
        matches: [
          { stage: 0, candidate: 1, unresolvedReasons: 0 },
          { stage: 0, candidate: 3, unresolvedReasons: 0 },
          { stage: 0, candidate: 3, unresolvedReasons: 0 },
        ],
        matchCount: 3,
      }),
      /matchStages\[0\]/,
    ],
    [
      "段階と単位が異なる比較を持つ関連付け",
      (json) => ({
        ...json,
        matches: [
          {
            stage: 0,
            candidate: 1,
            unresolvedReasons: 0,
            timeComparison: { comparisonUnit: "not_compared", assumptions: [] },
          },
        ],
      }),
      /matches\[0\]\.timeComparison\.comparisonUnit/,
    ],
    [
      "関連付けが無いのに置いたレコード",
      (json) => ({ ...json, matchStages: [], matches: [], matchCount: 0 }),
      /matchRecords/,
    ],
  ];
  for (const [name, change, path] of cases) {
    expect(
      () =>
        decodeEdgeDetailResponse(
          change(matchedEdgeDetailResponseJson()),
          "response",
        ),
      name,
    ).toThrow(path);
  }
});

// 時刻を比べた段階の関連付けは、起点と候補の時刻から比較を組む。候補が時刻を持たず、関連付けが
// 比較を持たない応答は、比べた時刻を読めない。
test("時刻を比べた段階で、比較を組めない関連付けを読まない", () => {
  const json = matchedEdgeDetailResponseJson();
  const withoutTime = {
    ...json,
    matches: [{ stage: 0, candidate: 2, unresolvedReasons: 0 }],
  };
  expect(() => decodeEdgeDetailResponse(withoutTime, "response")).toThrow(
    /matches\[0\]\.timeComparison/,
  );

  // 関連付けが比較を持てば、候補が時刻を持たなくても読める。持っている比較をそのまま採る。
  const leftTime = json.matchRecords[1].eventTime;
  const rightTime = json.matchRecords[0].eventTime;
  const carried = {
    ...json,
    matches: [
      {
        stage: 0,
        candidate: 2,
        unresolvedReasons: 0,
        timeComparison: {
          comparisonUnit: "second",
          leftTime,
          rightTime,
          assumptions: [],
        },
      },
    ],
  };
  const detail = decodeEdgeDetailResponse(carried, "response");
  const comparison = edgeMatchTimeComparison(detail, detail.matches[0]);
  expect(comparison.leftTime?.rawText).toBe("2031/10/08 10:20:35.900");
  expect(comparison.rightTime?.rawText).toBe("2031/10/08 10:20:35.100");
  expect(comparison.assumptions).toEqual([]);
});

// 関連付けが比較を持たないときは、段階の単位と前提と、両端のレコードの時刻から組む。
test("関連付けの時刻の比較を、段階と両端のレコードから組む", () => {
  const detail = decodeEdgeDetailResponse(
    matchedEdgeDetailResponseJson(),
    "response",
  );
  const match = detail.matches[0];
  const comparison = edgeMatchTimeComparison(detail, match);
  expect(comparison.comparisonUnit).toBe("second");
  // 起点は sn=112、候補は sn=115 のレコードの時刻である。
  expect(comparison.leftTime?.rawText).toBe("2031/10/08 10:20:35.100");
  expect(comparison.rightTime?.rawText).toBe("2031/10/08 10:20:35.900");
  expect(comparison.assumptions.map((each) => each.assumptionKey)).toEqual([
    "clock_offset_below_one_second",
  ]);

  const notCompared = {
    ...detail,
    matchStages: detail.matchStages.map((each) => ({
      ...each,
      comparisonUnit: "not_compared" as const,
    })),
  };
  expect(edgeMatchTimeComparison(notCompared, match)).toEqual({
    comparisonUnit: "not_compared",
    assumptions: [],
  });

  // 候補の母集合を組むときの前提 (Proxy への接続) は、段階の前提に載り、時刻の比較には載らない。
  const withProxy = {
    ...detail,
    matchStages: detail.matchStages.map((each) => ({
      ...each,
      assumptions: [
        {
          assumptionKey: "counterpart_connected_to_proxy" as const,
          evidenceClass: "inferred" as const,
          statement: "候補の接続先が Proxy の IP アドレスである",
        },
        ...each.assumptions,
      ],
    })),
  };
  expect(
    edgeMatchTimeComparison(withProxy, match).assumptions.map(
      (each) => each.assumptionKey,
    ),
  ).toEqual(["clock_offset_below_one_second"]);
});
