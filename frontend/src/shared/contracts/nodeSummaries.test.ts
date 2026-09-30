import { expect, test } from "vitest";
import { timelineResponseJson } from "@/testdata/timeline/timelineResponse";
import { DecodeFailure } from "./decoding";
import { decodeNodeSummariesResponse } from "./nodeSummaries";

const decode = (json: unknown) => decodeNodeSummariesResponse(json, "$");

function responseOf(summary: Record<string, unknown>) {
  return {
    nodes: [
      {
        node: timelineResponseJson().entries[0]?.terminal,
        recordCount: 3,
        localTimeRecordCount: 1,
        undatedRecordCount: 2,
        ...summary,
      },
    ],
    nodeCount: 1,
    nodeKind: "terminal",
  };
}

test("時刻の範囲の外のレコードの件数を、地方時と時刻の無いものに分けて読む", () => {
  const [summary] = decode(responseOf({})).nodes;
  expect(summary?.recordCount).toBe(3);
  expect(summary?.localTimeRecordCount).toBe(1);
  expect(summary?.undatedRecordCount).toBe(2);
  expect(summary?.firstTime).toBeUndefined();
});

test.each([
  ["地方時の件数の無い項目", { localTimeRecordCount: undefined }],
  ["時刻の無い件数の無い項目", { undatedRecordCount: undefined }],
  ["負の件数", { undatedRecordCount: -1 }],
  ["範囲の外の件数がレコードの件数を超える項目", { undatedRecordCount: 3 }],
])("契約に反する応答を読まない: %s", (_name, summary) => {
  expect(() => decode(responseOf(summary))).toThrow(DecodeFailure);
});
