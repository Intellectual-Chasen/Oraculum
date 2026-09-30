import { decodeRecordLocator, type RecordLocator } from "../contracts/common";
import {
  type Decoder,
  decodeString,
  optionalMember,
  optionalString,
  readObject,
  requireArray,
  requireCount,
  requireEnum,
  requireString,
} from "../contracts/decoding";
import type {
  FetchFailure,
  FetchFailureKind,
  SearchExpressionFailure,
} from "../lib/fetchState";

/** 失敗の種別。定義元は `backend/core/api_error.go` の `ApiErrorCode` である。 */
export const apiFailureCodes = [
  "not_implemented",
  "candidate_window_missing",
  "invalid_request",
  "source_not_found",
  "source_hash_mismatch",
  "import_withheld",
  "position_outside_source",
  "record_not_found",
  "record_unreadable",
  "stage_not_ready",
  "stage_already_started",
  "source_upload_already_exists",
  "terminal_assignment_already_recorded",
  "request_origin_rejected",
  "authentication_required",
  "login_rejected",
  "login_rate_limited",
  "investigation_not_found",
  "permission_denied",
  "workspace_changed",
  "assertion_changed",
  "assist_unavailable",
  "assist_not_permitted",
  "conversation_not_found",
  "assist_proposal_decided",
  "internal_error",
] as const;
export type ApiFailureCode = (typeof apiFailureCodes)[number];

/**
 * `code` と HTTP の status の対応。
 * 定義元は `backend/api/sources.go` の `httpStatusFor` である。
 */
export const apiFailureHttpStatuses: Record<ApiFailureCode, number> = {
  not_implemented: 501,
  candidate_window_missing: 400,
  invalid_request: 400,
  source_not_found: 404,
  source_hash_mismatch: 409,
  import_withheld: 409,
  position_outside_source: 404,
  record_not_found: 404,
  record_unreadable: 404,
  stage_not_ready: 409,
  stage_already_started: 409,
  source_upload_already_exists: 409,
  terminal_assignment_already_recorded: 409,
  request_origin_rejected: 403,
  authentication_required: 401,
  login_rejected: 401,
  login_rate_limited: 429,
  investigation_not_found: 404,
  permission_denied: 403,
  workspace_changed: 409,
  assertion_changed: 409,
  assist_unavailable: 409,
  assist_not_permitted: 409,
  conversation_not_found: 404,
  assist_proposal_decided: 409,
  internal_error: 500,
};

/**
 * 読み込みの要求を退けた理由の種別。
 * 定義元は `backend/core/stage_reason.go` の `LoadingRejection` である。
 */
export const loadingRejections = [
  "no_base_directory",
  "no_source",
  "partial_case",
  "case_invalid",
  "path_outside_base",
  "path_control_character",
  "format_unknown",
  "format_spec_invalid",
  "terminal_invalid",
  "source_repeated",
  "file_absent",
  "file_unreadable",
  "file_not_regular",
  "not_directory",
] as const;
export type LoadingRejection = (typeof loadingRejections)[number];

/** 読み込みの要求を退けた理由の短いラベル。失敗の詳細の「理由」の値である。 */
export const loadingRejectionDescriptions: Record<LoadingRejection, string> = {
  no_base_directory: "収集元を読み込む directory の指定なし",
  no_source: "収集元なし",
  partial_case: "案件の指定の有無が混在",
  case_invalid: "案件の形式の誤り",
  path_outside_base: "directory の外の path",
  path_control_character: "path に制御文字",
  format_unknown: "未対応の入力形式",
  format_spec_invalid: "フィールドの並びの指定の誤り",
  terminal_invalid: "端末の指定の誤り",
  source_repeated: "同じ収集元の重複",
  file_absent: "file なし",
  file_unreadable: "読み込めない file",
  file_not_regular: "通常の file 以外",
  not_directory: "directory 以外",
};

/**
 * 検索式を読めなかった理由の種別。
 * 定義元は backend の `core.SearchExpressionErrorReason` である。
 */
export const searchExpressionErrorReasons = [
  "empty_expression",
  "too_long",
  "too_many_terms",
  "nesting_too_deep",
  "unexpected_character",
  "unterminated_string",
  "invalid_escape",
  "missing_field",
  "missing_value",
  "missing_operand",
  "unclosed_parenthesis",
  "unmatched_parenthesis",
  "value_not_ordered",
] as const;
export type SearchExpressionErrorReason =
  (typeof searchExpressionErrorReasons)[number];

/** 検索式を読めなかった理由の短いラベル。 */
export const searchExpressionErrorDescriptions: Record<
  SearchExpressionErrorReason,
  string
> = {
  empty_expression: "空の検索式",
  too_long: "長さの上限を超過",
  too_many_terms: "条件の数の上限を超過",
  nesting_too_deep: "括弧と not の入れ子の上限を超過",
  unexpected_character: "条件を始められない文字",
  unterminated_string: '閉じる「"」なし',
  invalid_escape: "「\\」の後の文字の誤り",
  missing_field: "演算子の前のフィールドなし",
  missing_value: "演算子の後の値なし",
  missing_operand: "and・or・not の条件なし",
  unclosed_parenthesis: "閉じる括弧なし",
  unmatched_parenthesis: "開く括弧なし",
  value_not_ordered: "数か RFC 3339 の時刻以外の値",
};

/**
 * 検索式を読めなかった誤り 1 件。`offset` と `length` は、式の先頭から数えた Unicode の
 * code point の個数である。`length` が 0 の誤りは、`offset` の位置に文字列が足りないことを表す。
 */
export type SearchExpressionError = {
  reason: SearchExpressionErrorReason;
  offset: number;
  length: number;
};

/** `SearchExpressionError` を検証する。 */
const decodeSearchExpressionError: Decoder<SearchExpressionError> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    reason: requireEnum(source, "reason", path, searchExpressionErrorReasons),
    offset: requireCount(source, "offset", path),
    length: requireCount(source, "length", path),
  };
};

/** 失敗を表す組。`message` は英語で、画面の文言に使わない。 */
export type ApiError = {
  code: ApiFailureCode;
  message: string;
  missingParameters?: string[];
  sourceId?: string;
  sourceContentSha256?: string;
  originPath?: string;
  /**
   * 失敗に関わるレコード位置。
   * `backend/core/api_error.go` の `RecordRef` は `code` のすべての値で省略可であり、
   * 出る条件を `code` で限定しない。出ない応答は、失敗がレコードに関わらないことを表す。
   */
  recordRef?: RecordLocator;
  importStatusRef?: string;
  /** 読み込みの要求を退けた理由。`code` が `invalid_request` のときだけ出る。 */
  loadingRejection?: LoadingRejection;
  /** 検索式を読めなかった誤り。`code` が `invalid_request` のときだけ出る。 */
  searchExpressionError?: SearchExpressionError;
  /**
   * 競合した相手の記録。`code` が `workspace_changed` と `assertion_changed` と
   * `assist_proposal_decided` のときに出る。
   * 中身の形は `code` ごとに違い、呼び出し側が読む。
   */
  conflict?: unknown;
};

/** `ApiError` を検証する。 */
export const decodeApiError: Decoder<ApiError> = (input, path) => {
  const source = readObject(input, path);
  return {
    code: requireEnum(source, "code", path, apiFailureCodes),
    message: requireString(source, "message", path),
    missingParameters:
      "missingParameters" in source
        ? requireArray(source, "missingParameters", path, decodeString)
        : undefined,
    sourceId: optionalString(source, "sourceId", path),
    sourceContentSha256: optionalString(source, "sourceContentSha256", path),
    originPath: optionalString(source, "originPath", path),
    recordRef: optionalMember(source, "recordRef", path, decodeRecordLocator),
    importStatusRef: optionalString(source, "importStatusRef", path),
    loadingRejection:
      "loadingRejection" in source
        ? requireEnum(source, "loadingRejection", path, loadingRejections)
        : undefined,
    searchExpressionError: optionalMember(
      source,
      "searchExpressionError",
      path,
      decodeSearchExpressionError,
    ),
    conflict: source.conflict,
  };
};

/** HTTP の status から失敗の分類を決める。 */
export function classifyHttpStatus(status: number): FetchFailureKind {
  if (status === 401 || status === 403) {
    return "authorization";
  }
  if (status === 429) {
    return "rate_limited";
  }
  if (status >= 500) {
    return "server";
  }
  if (status >= 400) {
    return "request_rejected";
  }
  return "server";
}

/** 失敗の分類ごとの、次に行う操作の短いラベル。 */
const nextActions: Record<FetchFailureKind, string> = {
  network: "通信を確認して再実行",
  authorization: "再ログインして権限を確認",
  request_rejected: "指定を直して再実行",
  rate_limited: "時間を空けて再実行",
  server: "時間を空けて再実行",
  response_unreadable: "サーバーと画面のバージョンを確認",
  unexpected: "画面を再読み込み",
};

/** 未対応の機能に対して次に行う操作のラベル。 */
const notImplementedNextAction = "なし";

/** 許可していないアドレスで開いた画面から、次に行う操作のラベル。 */
const requestOriginRejectedNextAction =
  "--allowed-host のアドレスか localhost で開く";

/** 別の分析者が先にメモを改訂したときに、次に行う操作のラベル。 */
const assertionChangedNextAction = "別の分析者の値と比べて選択";

/** AI 支援の失敗の種類ごとに、次に行う操作のラベル。 */
const assistNextActions = {
  assist_unavailable: "運用者に調査の directory を指定した起動を依頼",
  assist_proposal_decided: "一覧の採否を確認",
} as const;

/** ログインの失敗の種類ごとに、次に行う操作のラベル。 */
const loginNextActions = {
  authentication_required: "ログインして再実行",
  login_rejected: "ログイン名とパスワードを確認",
  login_rate_limited: "時間を空けて再ログイン",
} as const;

/** 役割の失敗の種類ごとに、次に行う操作のラベル。 */
const roleNextActions = {
  investigation_not_found: "管理者に役割の付与を依頼",
  permission_denied: "管理者に役割の変更を依頼",
} as const;

function nextActionOf(kind: FetchFailureKind, apiError?: ApiError): string {
  switch (apiError?.code) {
    case "authentication_required":
    case "login_rejected":
    case "login_rate_limited":
      return loginNextActions[apiError.code];
    case "investigation_not_found":
    case "permission_denied":
      return roleNextActions[apiError.code];
    case "not_implemented":
      return notImplementedNextAction;
    case "request_origin_rejected":
      return requestOriginRejectedNextAction;
    case "assertion_changed":
      return assertionChangedNextAction;
    case "assist_unavailable":
    case "assist_proposal_decided":
      return assistNextActions[apiError.code];
    default:
      return nextActions[kind];
  }
}

/**
 * `code` が表す失敗の種類の短いラベル。画面は StatusLabel のラベルに使う。
 * 意味は `backend/core/api_error.go` の `ApiErrorCode` の各値の doc コメントに対応する。
 */
export const apiFailureDescriptions: Record<ApiFailureCode, string> = {
  not_implemented: "未対応の機能",
  candidate_window_missing: "時刻の範囲の指定なし",
  invalid_request: "指定の誤り",
  source_not_found: "収集元なし",
  source_hash_mismatch: "取り込み後の収集元の変更",
  import_withheld: "収集元の公開の停止",
  position_outside_source: "収集元の範囲外の位置",
  record_not_found: "レコードかノードなし",
  record_unreadable: "取り込みで読み込めないレコード",
  stage_not_ready: "読み込みか処理が未完了",
  stage_already_started: "実行中か完了済みの段階",
  source_upload_already_exists: "同じ保存先にアップロード済みの資料",
  terminal_assignment_already_recorded: "記録済みの端末の割り当て",
  request_origin_rejected: "許可されていないアドレス",
  authentication_required: "ログインが必要",
  login_rejected: "ログイン名かパスワードの誤り",
  login_rate_limited: "ログインの一時停止",
  investigation_not_found: "調査の役割なし",
  permission_denied: "役割の不足",
  workspace_changed: "別の画面によるワークスペースの変更",
  assertion_changed: "別の分析者によるメモの記録",
  assist_unavailable: "AI 支援の記録の場所なし",
  assist_not_permitted: "証拠の送信が未許可",
  conversation_not_found: "AI 支援の会話なし",
  assist_proposal_decided: "採否の決定済み",
  internal_error: "サーバーの内部の失敗",
};

/**
 * 分類から、利用者に見せる失敗 1 件を組み立てる。
 *
 * `recordRef` を渡さない。`decodeApiError` は省略可の項目として読むが、
 * それを載せた失敗の応答を返す経路が backend に無く、画面に出す先が無い
 * (`FetchFailureNotice`)。
 */
export function buildFetchFailure(
  kind: FetchFailureKind,
  summary: string,
  apiError?: ApiError,
): FetchFailure {
  return {
    kind,
    summary,
    nextAction: nextActionOf(kind, apiError),
    failureCode: apiError?.code,
    failureDescription:
      apiError === undefined
        ? undefined
        : apiFailureDescriptions[apiError.code],
    sourceId: apiError?.sourceId,
    sourceContentSha256: apiError?.sourceContentSha256,
    originPath: apiError?.originPath,
    rejectionDescription:
      apiError?.loadingRejection === undefined
        ? undefined
        : loadingRejectionDescriptions[apiError.loadingRejection],
    searchExpressionError: searchExpressionFailureOf(
      apiError?.searchExpressionError,
    ),
    conflict: apiError?.conflict,
  };
}

/** 検索式の誤りに、理由の日本語の説明を添える。 */
function searchExpressionFailureOf(
  error: SearchExpressionError | undefined,
): SearchExpressionFailure | undefined {
  if (error === undefined) {
    return undefined;
  }
  return {
    reason: error.reason,
    offset: error.offset,
    length: error.length,
    description: searchExpressionErrorDescriptions[error.reason],
  };
}
