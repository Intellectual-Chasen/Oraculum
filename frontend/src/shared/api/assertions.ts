import {
  type AssertionItem,
  type AssertionRecordRef,
  type AssertionState,
  type AssertionsResponse,
  type AssertionTarget,
  decodeAssertionItem,
  decodeAssertionsResponse,
} from "../contracts/assertions";
import { buildFetchFailure } from "./apiFailure";
import { type ApiResult, requestJson } from "./httpClient";
import {
  type MatchConditionSelection,
  matchConditionParams,
} from "./matchConditions";

const listFailureSummary = "メモの取得";
const recordFailureSummary = "メモの記録";
const revisionFailureSummary = "メモの改訂の記録";

/** 所見の一覧の要求。記録した所見を全件受け取る。 */
export type AssertionsRequest = {
  /**
   * 候補を絞るのに用いる条件の選択。要求が必ず含む。所見の対象の解決に、その選択で
   * 組んだグラフを使う (`backend/api/sources.go` の `matchSelectingHandler`)。
   */
  matchConditions: MatchConditionSelection;
};

/** 新しい所見 1 件の入力。識別子・時刻・改訂の番号・状態は backend が決める。 */
export type AssertionDraft = {
  target: AssertionTarget;
  /** 分析者の名前。ログインしているときは省き、server がログイン名を著者にする。 */
  author?: string;
  note: string;
  recordRefs: AssertionRecordRef[];
  /** 収集元の時刻を読む UTC からのずれ。対象が収集元のときに持つ。 */
  timeOffset?: string;
};

/** 既存の所見へ足す新しい改訂の入力。対象は改訂で変わらない。 */
export type AssertionRevisionDraft = AssertionDraft & {
  state: AssertionState;
  /**
   * 改訂の元にした所見の `revisionNumber`。server の現在の値と違うときは `assertion_changed` で
   * 失敗し、失敗の `conflict` が現在の所見を持つ。
   */
  baseRevision: number;
};

/** `+09:00` の形の UTC からのずれの文字列。分は 00 から 59 である。 */
const utcOffsetPattern = /^([+-])(\d{2}):([0-5]\d)$/;

/** 受け入れるずれの絶対値の上限 (分)。 */
const maxUtcOffsetMinutes = 18 * 60;

/**
 * ずれの文字列が backend の `UtcOffset.Validate` (`backend/core/time_interpretation.go`) の
 * 範囲にあるかを返す。絶対値が 18:00 以下であり、ずれが分からないことを表す `-00:00` を退ける。
 */
export function isUtcOffset(text: string): boolean {
  const match = utcOffsetPattern.exec(text);
  if (match === null) {
    return false;
  }
  const minutes = Number(match[2]) * 60 + Number(match[3]);
  if (minutes > maxUtcOffsetMinutes) {
    return false;
  }
  return !(match[1] === "-" && minutes === 0);
}

/** 要求の本文を組む。 */
function assertionBody(draft: AssertionDraft): Record<string, unknown> {
  return {
    target: draft.target,
    author: draft.author,
    basis: { note: draft.note, recordRefs: draft.recordRefs },
    timeOffset: draft.timeOffset,
  };
}

/**
 * handler が退ける入力を送る前に分け、誤りの短いラベルを返す。著者とメモは空白だけの文字列を
 * 退ける。
 */
function draftProblem(draft: AssertionDraft): string | undefined {
  if (draft.author?.trim() === "") {
    return "分析者の名前なし";
  }
  if (draft.note.trim() === "") {
    return "メモなし";
  }
  if (draft.target.kind === "source" && !isUtcOffset(draft.timeOffset ?? "")) {
    return "タイムゾーンの誤り: +09:00 の形、-18:00〜+18:00";
  }
  return undefined;
}

/** 所見の一覧 (`GET /api/v0/assertions`) を取得する。 */
export async function fetchAssertions(
  request: AssertionsRequest,
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<AssertionsResponse>> {
  return requestJson({
    path: "/assertions",
    searchParams: {
      matchCondition: matchConditionParams(request.matchConditions),
    },
    decode: decodeAssertionsResponse,
    failureSummary: listFailureSummary,
    signal: options.signal,
  });
}

/** 所見を 1 件記録する (`POST /api/v0/assertions`)。 */
export async function createAssertion(
  draft: AssertionDraft,
  matchConditions: MatchConditionSelection,
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<AssertionItem>> {
  const problem = draftProblem(draft);
  if (problem !== undefined) {
    return {
      ok: false,
      failure: buildFetchFailure("request_rejected", problem),
    };
  }
  return requestJson({
    path: "/assertions",
    method: "POST",
    searchParams: { matchCondition: matchConditionParams(matchConditions) },
    body: assertionBody(draft),
    decode: decodeAssertionItem,
    failureSummary: recordFailureSummary,
    signal: options.signal,
  });
}

/**
 * 既存の所見へ新しい改訂を足す (`PUT /api/v0/assertions/{id}`)。置き換えられた改訂は履歴に残る。
 * 取り消しは `state` を `withdrawn` にした改訂である。
 */
export async function reviseAssertion(
  assertionId: string,
  draft: AssertionRevisionDraft,
  matchConditions: MatchConditionSelection,
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<AssertionItem>> {
  const problem = draftProblem(draft);
  if (problem !== undefined) {
    return {
      ok: false,
      failure: buildFetchFailure("request_rejected", problem),
    };
  }
  return requestJson({
    path: `/assertions/${encodeURIComponent(assertionId)}`,
    method: "PUT",
    searchParams: { matchCondition: matchConditionParams(matchConditions) },
    body: {
      ...assertionBody(draft),
      state: draft.state,
      baseRevision: draft.baseRevision,
    },
    decode: decodeAssertionItem,
    failureSummary: revisionFailureSummary,
    signal: options.signal,
  });
}
