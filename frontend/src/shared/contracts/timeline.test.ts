import { describe, expect, test } from "vitest";
import {
  emptyTimelineResponseJson,
  timelineResponseJson,
} from "@/testdata/timeline/timelineResponse";
import { DecodeFailure } from "./decoding";
import { coverageStates, decodeTimelineResponse } from "./timeline";

const path = "timeline";

function decode(json: unknown) {
  return decodeTimelineResponse(json, path);
}

test("応答の行と収集元の収録範囲を読む", () => {
  const response = decode(timelineResponseJson());

  expect(response.entryCount).toBe(response.entries.length);
  expect(response.undatedRecordCount).toBe(3);
  expect(response.entries[0]?.terminal?.label.rawText).toBe("HOST-C");
  expect(response.entries[0]?.account?.label.rawText).toBe("user01");
  // 2 行目はアカウントが現れない。値の不在を空文字列で表さない。
  expect(response.entries[1]?.account).toBeUndefined();
  expect(response.sourceCoverages.map((coverage) => coverage.state)).toEqual([
    "covered",
    "outside_recording",
    "recording_range_unknown",
  ]);
  // 収録範囲を読めない収集元は、両端を含まない。
  const unknown = response.sourceCoverages[2];
  expect(unknown?.observedRangeFirst).toBeUndefined();
  expect(unknown?.observedRangeLast).toBeUndefined();
});

test("0 件の応答は、理由と収集元の収録範囲を含む", () => {
  const response = decode(emptyTimelineResponseJson());

  expect(response.entries).toHaveLength(0);
  expect(response.emptyReason).toBe("no_record_in_filter");
  expect(response.eventCategory).toBe("no_such_category");
  expect(response.sourceCoverages).toHaveLength(2);
});

test("応答が用いた検索式を文字列のまま読み、出ない応答では検索式を持たない", () => {
  const expression = "LogonType >= 3 || process.name contains cmd";

  expect(
    decode({ ...timelineResponseJson(), searchExpression: expression })
      .searchExpression,
  ).toBe(expression);
  expect(decode(timelineResponseJson()).searchExpression).toBeUndefined();
});

describe("契約に反する応答を読まない", () => {
  test("文字列でない検索式を含む応答", () => {
    const json = { ...timelineResponseJson(), searchExpression: 3 };

    expect(() => decode(json)).toThrow("timeline.searchExpression:");
  });

  test("数えた件数と行の個数が食い違う応答", () => {
    const json = { ...timelineResponseJson(), entryCount: 5 };

    expect(() => decode(json)).toThrow(DecodeFailure);
  });

  test("理由を含まない 0 件の応答", () => {
    const json = { ...emptyTimelineResponseJson(), emptyReason: undefined };

    expect(() => decode(json)).toThrow(DecodeFailure);
  });

  test("1 件以上を含みながら理由も含む応答", () => {
    const json = {
      ...timelineResponseJson(),
      emptyReason: "no_record_in_filter",
    };

    expect(() => decode(json)).toThrow(DecodeFailure);
  });

  test("契約の外の収録範囲の状態", () => {
    const base = timelineResponseJson();
    const json = {
      ...base,
      sourceCoverages: [{ ...base.sourceCoverages[0], state: "no_record" }],
    };

    expect(() => decode(json)).toThrow(DecodeFailure);
  });

  test("収集元の file 名を含まない収録範囲", () => {
    const base = timelineResponseJson();
    const json = {
      ...base,
      sourceCoverages: [
        { ...base.sourceCoverages[0], sourceFileName: undefined },
      ],
    };

    expect(() => decode(json)).toThrow(DecodeFailure);
  });

  test("負の件数", () => {
    const json = { ...timelineResponseJson(), undatedRecordCount: -1 };

    expect(() => decode(json)).toThrow(DecodeFailure);
  });

  test.each([[[-1]], [[2]], [[0.5]], [[1, 0]], [[1, 1]]])(
    "行の位置を外れるか昇順に並ばない一致の位置 %j",
    (findMatches) => {
      const json = { ...timelineResponseJson(), findMatches };

      expect(() => decode(json)).toThrow(DecodeFailure);
    },
  );
});

test("一致の位置を昇順で読む", () => {
  const json = { ...timelineResponseJson(), findMatches: [0, 1] };

  expect(decode(json).findMatches).toEqual([0, 1]);
});

test("収録範囲の状態の一覧が、記録の不在と事象の不在を別の値で持つ", () => {
  // 記録の不在を表す 2 値と、記録があることを表す 2 値を、同じ値へまとめない。
  for (const state of [
    "covered",
    "partially_covered",
    "outside_recording",
    "recording_range_unknown",
    "range_not_requested",
    "undetermined",
  ]) {
    expect(coverageStates).toContain(state);
  }
  expect(new Set(coverageStates).size).toBe(coverageStates.length);
});
