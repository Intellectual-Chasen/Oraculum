import { expect, test } from "vitest";
import type { AssertionTarget } from "@/shared/contracts/assertions";
import { assertionTargetKey } from "./assertionTargetKey";

const nodeTarget: AssertionTarget = { kind: "node", nodeId: "n:process:8ab3" };

const edgeRef = {
  kind: "terminal_remote_session",
  sourceNodeId: "n:terminal:9c41",
  targetNodeId: "n:terminal:1f0c",
};
const edgeTarget: AssertionTarget = { kind: "edge", edge: edgeRef };

const recordRef = {
  sourceContentSha256: "a".repeat(64),
  positionKind: "sequence_number",
  sequenceNumber: 112,
  lineNumber: 1022,
} as const;
const recordTarget: AssertionTarget = { kind: "record", record: recordRef };

test.each([
  ["ノード", nodeTarget],
  ["関係", edgeTarget],
  ["レコード", recordTarget],
])("同じ %s の対象を指す 2 つの組が同じ文字列になる", (_kind, target) => {
  const key = assertionTargetKey(target);

  expect(key).toBe(assertionTargetKey({ ...target }));
  expect(key).not.toBeUndefined();
});

test("種別が違う 3 つの対象は、互いに別の文字列になる", () => {
  const keys = [nodeTarget, edgeTarget, recordTarget].map(assertionTargetKey);

  expect(new Set(keys).size).toBe(keys.length);
});

test.each([
  [
    "ノードの識別子が違う",
    nodeTarget,
    { ...nodeTarget, nodeId: "n:process:0000" },
  ],
  [
    "関係の終点が違う",
    edgeTarget,
    {
      kind: "edge" as const,
      edge: { ...edgeRef, targetNodeId: "n:terminal:0000" },
    },
  ],
  [
    "レコードの通番が違う",
    recordTarget,
    {
      kind: "record" as const,
      record: { ...recordRef, sequenceNumber: 113 },
    },
  ],
  [
    "レコードの収集元の内容の識別が違う",
    recordTarget,
    {
      kind: "record" as const,
      record: { ...recordRef, sourceContentSha256: "b".repeat(64) },
    },
  ],
])("%s 2 つの対象は別の文字列になる", (_name, left, right) => {
  expect(assertionTargetKey(left)).not.toBe(assertionTargetKey(right));
});

test.each([
  ["ノードの識別子を欠く", { kind: "node" } as AssertionTarget],
  ["関係の参照を欠く", { kind: "edge" } as AssertionTarget],
  ["レコードの参照を欠く", { kind: "record" } as AssertionTarget],
  [
    "種別が要する参照と別の参照を持つ",
    { kind: "node", record: recordRef } as AssertionTarget,
  ],
])("%s 組は文字列を持たない", (_name, target) => {
  expect(assertionTargetKey(target)).toBeUndefined();
});
