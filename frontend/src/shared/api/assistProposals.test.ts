import { afterEach, expect, test, vi } from "vitest";
import {
  adoptedItemJson,
  adoptionJson,
  assistProposalsResponseJson,
  rejectedItemJson,
} from "@/testdata/assistProposals/assistProposalsResponse";
import { jsonResponse } from "@/testdata/http";
import {
  adoptAssistProposal,
  fetchAssistProposals,
  rejectAssistProposal,
} from "./assistProposals";
import type { MatchConditionSelection } from "./matchConditions";

const matchConditions: MatchConditionSelection = {
  conditions: [{ conditionKey: "destination_ip" }],
};
const matchConditionQuery = "matchCondition=destination_ip";

afterEach(() => {
  vi.unstubAllGlobals();
});

function stubFetch(result: Response) {
  const mock = vi.fn(async (_input: string, _init?: RequestInit) => result);
  vi.stubGlobal("fetch", mock);
  return mock;
}

test("AI 提案の一覧を、関連付けの条件の選択を付けて取得する", async () => {
  const mock = stubFetch(jsonResponse(200, assistProposalsResponseJson()));

  const result = await fetchAssistProposals(matchConditions);

  expect(mock.mock.calls[0]?.[0]).toBe(
    `/api/v0/assist-proposals?${matchConditionQuery}`,
  );
  expect(result.ok && result.value.pendingCount).toBe(1);
});

test("採用は直した記述と分析者を本文に載せ、採用の応答を読む", async () => {
  const mock = stubFetch(jsonResponse(201, adoptionJson("直した記述")));

  const result = await adoptAssistProposal(
    "ap:0001",
    { analyst: "analyst-b", note: "直した記述" },
    matchConditions,
  );

  expect(mock.mock.calls[0]?.[0]).toBe(
    `/api/v0/assist-proposals/ap%3A0001/adoption?${matchConditionQuery}`,
  );
  const init = mock.mock.calls[0]?.[1];
  expect(init?.method).toBe("POST");
  expect(JSON.parse(String(init?.body))).toEqual({
    analyst: "analyst-b",
    note: "直した記述",
  });
  expect(result.ok && result.value.assertion.assertion.basis.note).toBe(
    "直した記述",
  );
});

test("記述を直さない採用と、理由の無い却下は、その項目を送らない", async () => {
  const adopted = stubFetch(jsonResponse(201, adoptionJson()));
  await adoptAssistProposal("ap:0001", {}, matchConditions);
  expect(JSON.parse(String(adopted.mock.calls[0]?.[1]?.body))).toEqual({});

  const rejected = stubFetch(jsonResponse(200, rejectedItemJson()));
  const result = await rejectAssistProposal(
    "ap:0001",
    { analyst: "analyst-b", reason: "  " },
    matchConditions,
  );
  expect(rejected.mock.calls[0]?.[0]).toBe(
    `/api/v0/assist-proposals/ap%3A0001/rejection?${matchConditionQuery}`,
  );
  expect(JSON.parse(String(rejected.mock.calls[0]?.[1]?.body))).toEqual({
    analyst: "analyst-b",
  });
  expect(result.ok && result.value.proposal.state).toBe("rejected");
});

test("空白だけの名前と直した記述は送らずに退ける", async () => {
  const mock = stubFetch(jsonResponse(201, adoptionJson()));

  const blankAnalyst = await adoptAssistProposal(
    "ap:0001",
    { analyst: " " },
    matchConditions,
  );
  const blankNote = await adoptAssistProposal(
    "ap:0001",
    { note: " " },
    matchConditions,
  );

  expect(mock).not.toHaveBeenCalled();
  expect(blankAnalyst.ok || blankAnalyst.failure.summary).toBe(
    "分析者の名前なし",
  );
  expect(blankNote.ok || blankNote.failure.summary).toBe("直した記述なし");
});

test("先に採否を決めた提案の採用は、現在の提案を競合として返す", async () => {
  stubFetch(
    jsonResponse(409, {
      code: "assist_proposal_decided",
      message: "the proposal is already decided",
      conflict: adoptedItemJson(),
    }),
  );

  const result = await adoptAssistProposal("ap:0001", {}, matchConditions);

  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.failureCode).toBe("assist_proposal_decided");
  expect(result.failure.conflict).toEqual(adoptedItemJson());
});
