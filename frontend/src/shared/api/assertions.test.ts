import { afterEach, expect, test, vi } from "vitest";
import {
  assertionItemJson,
  assertionNote,
  assertionRecordRef,
  assertionsResponseJson,
  countMismatchAssertionsResponseJson,
} from "@/testdata/assertions/assertionsResponse";
import { jsonResponse } from "@/testdata/http";
import type { AssertionDraft } from "./assertions";
import {
  createAssertion,
  fetchAssertions,
  reviseAssertion,
} from "./assertions";
import type { MatchConditionSelection } from "./matchConditions";

// 本 test が渡す関連付けの条件の選択。
const matchConditions: MatchConditionSelection = {
  conditions: [{ conditionKey: "destination_ip" }],
};
const matchConditionQuery = "matchCondition=destination_ip";

afterEach(() => {
  vi.unstubAllGlobals();
});

function stubFetch(result: Response | Error) {
  const mock = vi.fn(async (_input: string, _init?: RequestInit) => {
    if (result instanceof Error) {
      throw result;
    }
    return result;
  });
  vi.stubGlobal("fetch", mock);
  return mock;
}

const noteDraft: AssertionDraft = {
  target: { kind: "record", record: assertionRecordRef },
  author: "analyst-a",
  note: assertionNote,
  recordRefs: [assertionRecordRef],
};

test("所見の一覧を取得し、メモと対象の出所を読む", async () => {
  const mock = stubFetch(jsonResponse(200, assertionsResponseJson()));

  const result = await fetchAssertions({ matchConditions });

  expect(mock.mock.calls[0]?.[0]).toBe(
    `/api/v0/assertions?${matchConditionQuery}`,
  );
  expect(result.ok).toBe(true);
  if (!result.ok) {
    return;
  }
  const first = result.value.assertions[0]?.assertion;
  expect(first?.basis.note).toBe(assertionNote);
  expect(first?.basis.recordRefs).toEqual([assertionRecordRef]);
  expect(result.value.assertions[0]?.targetOrigin).toBe("observation");
  expect(result.value.assertionCount).toBe(result.value.assertions.length);
});

test("メモを POST で記録し、本文に対象と著者と根拠だけを載せる", async () => {
  const mock = stubFetch(jsonResponse(201, assertionItemJson()));

  const result = await createAssertion(noteDraft, matchConditions);

  expect(mock.mock.calls[0]?.[0]).toBe(
    `/api/v0/assertions?${matchConditionQuery}`,
  );
  const init = mock.mock.calls[0]?.[1];
  expect(init?.method).toBe("POST");
  expect(JSON.parse(String(init?.body))).toEqual({
    target: { kind: "record", record: assertionRecordRef },
    author: "analyst-a",
    basis: {
      note: assertionNote,
      recordRefs: [assertionRecordRef],
    },
  });
  expect(result.ok).toBe(true);
  if (!result.ok) {
    return;
  }
  expect(result.value.assertion.basis.note).toBe(assertionNote);
});

test("著者を省いた入力は、本文に著者を載せずに送る", async () => {
  const mock = stubFetch(jsonResponse(201, assertionItemJson()));

  const result = await createAssertion(
    { ...noteDraft, author: undefined },
    matchConditions,
  );

  expect(result.ok).toBe(true);
  expect("author" in JSON.parse(String(mock.mock.calls[0]?.[1]?.body))).toBe(
    false,
  );
});

test("一覧の要求は条件の選択だけを query に載せる", async () => {
  const mock = stubFetch(jsonResponse(200, assertionsResponseJson()));

  await fetchAssertions({ matchConditions });

  const requestedUrl = new URL(
    String(mock.mock.calls[0]?.[0]),
    "https://example.test",
  );
  expect([...requestedUrl.searchParams.keys()]).toEqual(["matchCondition"]);
});

test("assertionCount が assertions の要素数と食い違う応答を読まない", async () => {
  stubFetch(jsonResponse(200, countMismatchAssertionsResponseJson()));

  const result = await fetchAssertions({ matchConditions });

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});

const sourceDraft: AssertionDraft = {
  target: { kind: "source", sourceContentSha256: "e".repeat(64) },
  author: "analyst-a",
  note: "同じ要求の UTC の行と比べた",
  recordRefs: [assertionRecordRef],
  timeOffset: "+09:00",
};

test("新しい改訂を PUT で送り、状態とずれと元にした revision を本文に載せる", async () => {
  const mock = stubFetch(jsonResponse(200, assertionItemJson()));

  const result = await reviseAssertion(
    "as:0001",
    { ...sourceDraft, state: "withdrawn", baseRevision: 3 },
    matchConditions,
  );

  expect(mock.mock.calls[0]?.[0]).toBe(
    `/api/v0/assertions/as%3A0001?${matchConditionQuery}`,
  );
  const init = mock.mock.calls[0]?.[1];
  expect(init?.method).toBe("PUT");
  expect(JSON.parse(String(init?.body))).toEqual({
    target: sourceDraft.target,
    author: "analyst-a",
    basis: { note: sourceDraft.note, recordRefs: [assertionRecordRef] },
    timeOffset: "+09:00",
    state: "withdrawn",
    baseRevision: 3,
  });
  expect(result.ok).toBe(true);
});

test("ずれの事前の検査は backend と同じ範囲だけを通す", async () => {
  const mock = stubFetch(jsonResponse(201, assertionItemJson()));
  for (const timeOffset of ["+09:60", "+18:30", "+19:00", "-00:00", "+9:00"]) {
    const result = await createAssertion(
      { ...sourceDraft, timeOffset },
      matchConditions,
    );
    expect({ timeOffset, ok: result.ok }).toEqual({ timeOffset, ok: false });
  }
  expect(mock).not.toHaveBeenCalled();
  for (const timeOffset of ["+18:00", "-18:00", "-05:30", "+00:00"]) {
    await createAssertion({ ...sourceDraft, timeOffset }, matchConditions);
  }
  expect(
    mock.mock.calls.map(
      (call) => JSON.parse(String(call[1]?.body)).timeOffset as string,
    ),
  ).toEqual(["+18:00", "-18:00", "-05:30", "+00:00"]);
});

test("対象が収集元で UTC からのずれを持たない所見を、読めない応答として扱う", async () => {
  const item = assertionItemJson();
  stubFetch(
    jsonResponse(200, {
      assertions: [
        {
          ...item,
          assertion: {
            ...item.assertion,
            target: { kind: "source", sourceContentSha256: "e".repeat(64) },
          },
        },
      ],
      assertionCount: 1,
    }),
  );

  const result = await fetchAssertions({ matchConditions });

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("response_unreadable");
});

test("収集元の解釈は +hh:mm の形でないずれを送らない", async () => {
  const mock = stubFetch(jsonResponse(201, assertionItemJson()));

  const result = await createAssertion(
    { ...sourceDraft, timeOffset: "9" },
    matchConditions,
  );

  expect(mock).not.toHaveBeenCalled();
  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.summary).toBe(
    "タイムゾーンの誤り: +09:00 の形、-18:00〜+18:00",
  );
});

test("空白だけの著者とメモを送らずに失敗として返す", async () => {
  const mock = stubFetch(jsonResponse(201, assertionItemJson()));

  const blankAuthor = await createAssertion(
    { ...noteDraft, author: " " },
    matchConditions,
  );
  const blankNote = await createAssertion(
    { ...noteDraft, note: "  " },
    matchConditions,
  );

  expect(mock).not.toHaveBeenCalled();
  expect(blankAuthor.ok).toBe(false);
  expect(blankNote.ok).toBe(false);
  if (blankAuthor.ok || blankNote.ok) {
    return;
  }
  expect(blankAuthor.failure.summary).toBe("分析者の名前なし");
  expect(blankNote.failure.summary).toBe("メモなし");
});
