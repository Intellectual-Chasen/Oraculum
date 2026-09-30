import { expect, test } from "vitest";
import type { Assertion } from "@/shared/contracts/assertions";
import { sourceInterpretationsOf } from "./sourceInterpretations";

function assertion(
  content: string,
  state: "active" | "withdrawn",
  timeOffset: string,
): Assertion {
  return {
    id: `a-${content}-${timeOffset}`,
    target: { kind: "source", sourceContentSha256: content },
    state,
    author: "analyst-a",
    recordedAt: "2031-01-02T03:04:05.000Z",
    basis: { note: "合成の根拠", recordRefs: [] },
    timeOffset,
    revisionNumber: 1,
    history: [],
    // 対象の関数が読むのは target・state・timeOffset・id だけであり、他の必須の項目を省く。
  } as Assertion;
}

function row(sourceId: string, content: string, importTimeOffset?: string) {
  return { source: { sourceId, contentSha256: content }, importTimeOffset };
}

test("主張中の解釈を 1 件持つ収集元はその解釈を、持たない収集元は起動で指定したずれを使う", () => {
  const result = sourceInterpretationsOf(
    [
      row("s-analyst", "c-analyst", "+01:00"),
      row("s-import", "c-import", "+02:00"),
      row("s-withdrawn", "c-withdrawn", "+03:00"),
      row("s-conflicted", "c-conflicted", "+04:00"),
      row("s-none", "c-none"),
    ],
    new Map([
      [
        "c-analyst",
        {
          kind: "single",
          assertion: assertion("c-analyst", "active", "+09:00"),
        },
      ],
      [
        "c-withdrawn",
        {
          kind: "single",
          assertion: assertion("c-withdrawn", "withdrawn", "+05:00"),
        },
      ],
      [
        "c-conflicted",
        {
          kind: "conflicted",
          assertions: [
            assertion("c-conflicted", "active", "+06:00"),
            assertion("c-conflicted", "active", "+07:00"),
          ],
        },
      ],
    ]),
  );

  expect(
    Object.fromEntries([...result].map(([id, value]) => [id, value.offset])),
  ).toEqual({
    "s-analyst": "+09:00",
    "s-import": "+02:00",
    "s-withdrawn": "+03:00",
    "s-conflicted": "+04:00",
  });
  expect(result.get("s-analyst")?.assertionId).toBe("a-c-analyst-+09:00");
  expect(result.get("s-import")?.assertionId).toBeUndefined();
});
