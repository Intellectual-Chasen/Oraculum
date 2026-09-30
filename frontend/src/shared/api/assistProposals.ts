import {
  type AssistProposalAdoption,
  type AssistProposalItem,
  type AssistProposalsResponse,
  decodeAssistProposalAdoption,
  decodeAssistProposalItem,
  decodeAssistProposalsResponse,
} from "../contracts/assistProposals";
import { buildFetchFailure } from "./apiFailure";
import { type ApiResult, requestJson } from "./httpClient";
import {
  type MatchConditionSelection,
  matchConditionParams,
} from "./matchConditions";

const listFailureSummary = "AI 提案の取得";
const adoptionFailureSummary = "AI 提案の採用";
const rejectionFailureSummary = "AI 提案の却下";

/** 採用の入力。 */
export type AssistProposalAdoptionDraft = {
  /** 分析者の名前。ログインしているときは省き、server がログイン名を使う。 */
  analyst?: string;
  /** 分析者が直した記述。省くと提案の記述をそのまま所見にする。 */
  note?: string;
};

/** 却下の入力。 */
export type AssistProposalRejectionDraft = {
  /** 分析者の名前。ログインしているときは省き、server がログイン名を使う。 */
  analyst?: string;
  /** 却下の理由。任意。 */
  reason?: string;
};

/** handler が退ける入力を送る前に分ける。名前と直した記述は空白だけの文字列を退ける。 */
function draftProblem(draft: {
  analyst?: string;
  note?: string;
}): string | undefined {
  if (draft.analyst?.trim() === "") {
    return "分析者の名前なし";
  }
  if (draft.note?.trim() === "") {
    return "直した記述なし";
  }
  return undefined;
}

function proposalPath(proposalId: string, operation: string): string {
  return `/assist-proposals/${encodeURIComponent(proposalId)}/${operation}`;
}

/**
 * AI 提案の一覧 (`GET /api/v0/assist-proposals`) を取得する。対象の出所の解決に、選択で組んだ
 * グラフを使う。
 */
export async function fetchAssistProposals(
  matchConditions: MatchConditionSelection,
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<AssistProposalsResponse>> {
  return requestJson({
    path: "/assist-proposals",
    searchParams: { matchCondition: matchConditionParams(matchConditions) },
    decode: decodeAssistProposalsResponse,
    failureSummary: listFailureSummary,
    signal: options.signal,
  });
}

/**
 * AI 提案を採用し、分析者を著者とする所見を作る
 * (`POST /api/v0/assist-proposals/{id}/adoption`)。先に採否を決めた提案は
 * `assist_proposal_decided` で失敗し、失敗の `conflict` が現在の提案を持つ。
 */
export async function adoptAssistProposal(
  proposalId: string,
  draft: AssistProposalAdoptionDraft,
  matchConditions: MatchConditionSelection,
): Promise<ApiResult<AssistProposalAdoption>> {
  const problem = draftProblem(draft);
  if (problem !== undefined) {
    return {
      ok: false,
      failure: buildFetchFailure("request_rejected", problem),
    };
  }
  return requestJson({
    path: proposalPath(proposalId, "adoption"),
    method: "POST",
    searchParams: { matchCondition: matchConditionParams(matchConditions) },
    body: { analyst: draft.analyst, note: draft.note },
    decode: decodeAssistProposalAdoption,
    failureSummary: adoptionFailureSummary,
  });
}

/** AI 提案を却下する (`POST /api/v0/assist-proposals/{id}/rejection`)。 */
export async function rejectAssistProposal(
  proposalId: string,
  draft: AssistProposalRejectionDraft,
  matchConditions: MatchConditionSelection,
): Promise<ApiResult<AssistProposalItem>> {
  const problem = draftProblem(draft);
  if (problem !== undefined) {
    return {
      ok: false,
      failure: buildFetchFailure("request_rejected", problem),
    };
  }
  return requestJson({
    path: proposalPath(proposalId, "rejection"),
    method: "POST",
    searchParams: { matchCondition: matchConditionParams(matchConditions) },
    body: {
      analyst: draft.analyst,
      reason:
        draft.reason === undefined || draft.reason.trim() === ""
          ? undefined
          : draft.reason,
    },
    decode: decodeAssistProposalItem,
    failureSummary: rejectionFailureSummary,
  });
}
