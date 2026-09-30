import type { AssertionRecordRef } from "../contracts/assertions";
import type { TimeRange } from "../contracts/common";
import {
  decodeTerminalAssignment,
  decodeTerminalAssignmentsResponse,
  type TerminalAssignment,
  type TerminalAssignmentsResponse,
} from "../contracts/terminalAssignments";
import { buildFetchFailure } from "./apiFailure";
import { type ApiResult, requestJson } from "./httpClient";

const listFailureSummary = "端末の割り当ての取得";
const recordFailureSummary = "端末の割り当ての記録";

/**
 * 分析者が画面から記録する割当 1 件の入力。
 *
 * 由来を要求が与えない。この経路から入る割当は画面から記録した割当であり、backend が
 * 決める。
 */
export type TerminalAssignmentDraft = {
  /** 接続元 IP。空白だけの文字列は省いた項目として扱う。 */
  clientIp?: string;
  /** 端末の外部識別子。省くと `appliesToSourceId` の収集元を記録した端末を指す。 */
  terminalId?: string;
  /** 端末の表示名。空白だけの文字列は省いた項目として扱う。 */
  terminalHostname?: string;
  /** 端末が名乗るホスト名 (短い名前と FQDN)。要素数 0 の並びは送らない。 */
  terminalHostnames?: string[];
  /** 適用期間を読み取る収集元。 */
  sourceId: string;
  sourceContentSha256: string;
  assignmentValidRange: TimeRange;
  derivation: string;
  basisRecordRefs: AssertionRecordRef[];
  /** 分析者の名前。ログインしているときは省き、server がログイン名を著者にする。 */
  author?: string;
  /** レコードの全体がこの端末のものである収集元。与えないときは IP の割当だけを表す。 */
  appliesToSourceId?: string;
};

/** 前後の空白を除いた文字列を返す。空白だけの文字列は省いた項目として `undefined` を返す。 */
function presentText(value: string | undefined): string | undefined {
  const trimmed = value?.trim();
  return trimmed === undefined || trimmed === "" ? undefined : trimmed;
}

/**
 * handler が退ける入力を送る前に分け、誤りの短いラベルを返す。定義元は
 * `backend/core/assignment.go` の `Validate` である。
 *
 * **空白だけの文字列を退ける。** 理由を書かない割り当ては、根拠を持たない推定になる。
 */
function draftProblem(draft: TerminalAssignmentDraft): string | undefined {
  const terminalId = presentText(draft.terminalId);
  if (
    presentText(draft.clientIp) === undefined &&
    terminalId === undefined &&
    presentText(draft.terminalHostname) === undefined
  ) {
    return "接続元 IP・端末 ID・端末の表示名なし";
  }
  // 端末 ID を持たない割り当ては、割り当てる収集元を記録した端末を指す。
  if (terminalId === undefined && draft.appliesToSourceId !== draft.sourceId) {
    return "端末 ID か収集元の全体の指定が必要";
  }
  // 接続元 IP も割り当てる収集元もホスト名も持たない割り当ては、端末の判定にもグラフにも使われない。
  if (
    presentText(draft.clientIp) === undefined &&
    draft.appliesToSourceId === undefined &&
    presentText(draft.terminalHostname) === undefined &&
    (draft.terminalHostnames ?? []).length === 0
  ) {
    return "接続元 IP・ホスト名か収集元の全体の指定が必要";
  }
  if (draft.author?.trim() === "") {
    return "分析者の名前なし";
  }
  if (draft.derivation.trim() === "") {
    return "理由なし";
  }
  if (draft.basisRecordRefs.length === 0) {
    return "根拠のレコードなし";
  }
  return undefined;
}

/** 端末の割当の一覧 (`GET /api/v0/terminal-assignments`) を取得する。 */
export async function fetchTerminalAssignments(
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<TerminalAssignmentsResponse>> {
  return requestJson({
    path: "/terminal-assignments",
    decode: decodeTerminalAssignmentsResponse,
    failureSummary: listFailureSummary,
    signal: options.signal,
  });
}

/** 端末の割当を 1 件記録する (`POST /api/v0/terminal-assignments`)。 */
export async function createTerminalAssignment(
  draft: TerminalAssignmentDraft,
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<TerminalAssignment>> {
  const problem = draftProblem(draft);
  if (problem !== undefined) {
    return {
      ok: false,
      failure: buildFetchFailure("request_rejected", problem),
    };
  }
  return requestJson({
    path: "/terminal-assignments",
    method: "POST",
    body: {
      ...draft,
      clientIp: presentText(draft.clientIp),
      terminalId: presentText(draft.terminalId),
      terminalHostname: presentText(draft.terminalHostname),
      terminalHostnames:
        draft.terminalHostnames !== undefined &&
        draft.terminalHostnames.length > 0
          ? draft.terminalHostnames
          : undefined,
    },
    decode: decodeTerminalAssignment,
    failureSummary: recordFailureSummary,
    signal: options.signal,
  });
}
