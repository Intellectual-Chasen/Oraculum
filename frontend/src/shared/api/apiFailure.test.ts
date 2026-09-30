import { expect, test } from "vitest";
import { apiErrorWithRecordRefJson } from "@/testdata/records/recordResponse";
import { searchExpressionErrorResponseJson } from "@/testdata/searchExpression/searchExpressionErrorResponse";
import {
  hostALogSha256,
  hostALogSourceId,
} from "@/testdata/sources/sourcesResponse";
import { DecodeFailure } from "../contracts/decoding";
import {
  apiFailureDescriptions,
  apiFailureHttpStatuses,
  buildFetchFailure,
  classifyHttpStatus,
  decodeApiError,
  loadingRejectionDescriptions,
  searchExpressionErrorDescriptions,
  searchExpressionErrorReasons,
} from "./apiFailure";

test("検索式を読めなかった理由の各値に、短いラベルを持ち、表に定義の外の値を持たない", () => {
  expect(Object.keys(searchExpressionErrorDescriptions).sort()).toEqual(
    [...searchExpressionErrorReasons].sort(),
  );
  for (const reason of searchExpressionErrorReasons) {
    expect(searchExpressionErrorDescriptions[reason]).not.toBe("");
  }
});

test("検索式の誤りを含む失敗から、理由と位置を読み、画面へ渡す失敗に説明を添える", () => {
  const error = decodeApiError(
    searchExpressionErrorResponseJson({
      reason: "missing_value",
      offset: 4,
      length: 2,
    }),
    "error",
  );
  expect(error.code).toBe("invalid_request");
  expect(error.searchExpressionError).toEqual({
    reason: "missing_value",
    offset: 4,
    length: 2,
  });

  const failure = buildFetchFailure("request_rejected", "グラフの取得", error);
  expect(failure.searchExpressionError).toEqual({
    reason: "missing_value",
    offset: 4,
    length: 2,
    description: "演算子の後の値なし",
  });
  expect(failure.failureCode).toBe("invalid_request");
});

test("長さ 0 の検索式の誤りを読む", () => {
  const error = decodeApiError(
    searchExpressionErrorResponseJson({
      reason: "unclosed_parenthesis",
      offset: 9,
      length: 0,
    }),
    "error",
  );

  expect(error.searchExpressionError?.length).toBe(0);
  expect(error.searchExpressionError?.offset).toBe(9);
});

test("検索式の誤りを含まない失敗は、画面へ渡す失敗にも誤りを載せない", () => {
  const failure = buildFetchFailure(
    "request_rejected",
    "グラフを取得できませんでした。",
    decodeApiError({ code: "invalid_request", message: "x" }, "error"),
  );

  expect(failure.searchExpressionError).toBeUndefined();
});

test.each([
  [
    "定義の外の理由",
    { reason: "disk_full", offset: 0, length: 1 },
    "error.searchExpressionError.reason",
  ],
  [
    "負の位置",
    { reason: "missing_value", offset: -1, length: 1 },
    "error.searchExpressionError.offset",
  ],
  [
    "整数でない長さ",
    { reason: "missing_value", offset: 0, length: 1.5 },
    "error.searchExpressionError.length",
  ],
])("%s を持つ検索式の誤りを読まない", (_name, searchExpressionError, path) => {
  expect(() =>
    decodeApiError(
      searchExpressionErrorResponseJson(searchExpressionError),
      "error",
    ),
  ).toThrow(`${path}:`);
});

test("読み込みの要求を拒否された理由の各値に、短いラベルを持つ", () => {
  expect(loadingRejectionDescriptions).toEqual({
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
  });
});

test("ApiErrorCode の各値に対応する HTTP の status を持つ", () => {
  expect(apiFailureHttpStatuses).toEqual({
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
  });
});

test.each([
  [404, "investigation_not_found", "管理者に役割の付与を依頼"],
  [403, "permission_denied", "管理者に役割の変更を依頼"],
])("status %i の %s は、管理者への依頼を案内する", (status, code, next) => {
  const failure = buildFetchFailure(
    classifyHttpStatus(status),
    "メモの記録",
    decodeApiError({ code, message: "x" }, "error"),
  );

  expect(failure.nextAction).toBe(next);
  expect(failure.failureCode).toBe(code);
});

test("許可していないアドレスで開いた画面の失敗は、開き直すアドレスを案内する", () => {
  const failure = buildFetchFailure(
    classifyHttpStatus(403),
    "グラフの取得",
    decodeApiError({ code: "request_origin_rejected", message: "x" }, "error"),
  );

  expect(failure.kind).toBe("authorization");
  expect(failure.nextAction).toBe(
    "--allowed-host のアドレスか localhost で開く",
  );
  expect(failure.failureDescription).toBe("許可されていないアドレス");
});

test("ApiErrorCode の各値に対応する失敗の種類の短いラベルを持つ", () => {
  expect(apiFailureDescriptions).toEqual({
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
  });
});

test("先に採否を決めた AI 提案の失敗は、現在の採否の確認を案内する", () => {
  const failure = buildFetchFailure(
    classifyHttpStatus(409),
    "AI 提案の採用",
    decodeApiError({ code: "assist_proposal_decided", message: "x" }, "error"),
  );

  expect(failure.nextAction).toBe("一覧の採否を確認");
  expect(failure.failureCode).toBe("assist_proposal_decided");
});

test.each([
  [401, "authorization"],
  [403, "authorization"],
  [429, "rate_limited"],
  [400, "request_rejected"],
  [404, "request_rejected"],
  [409, "request_rejected"],
  [500, "server"],
  [501, "server"],
])("HTTP の status %i を %s に分類する", (status, kind) => {
  expect(classifyHttpStatus(status)).toBe(kind);
});

test("欠けている要求の項目を含む失敗を読む", () => {
  const error = decodeApiError(
    {
      code: "invalid_request",
      message: "sortKey is required",
      missingParameters: ["sortKey"],
    },
    "error",
  );

  expect(error.code).toBe("invalid_request");
  expect(error.missingParameters).toEqual(["sortKey"]);
  expect(error.sourceId).toBeUndefined();
});

test("apiFailureCodes に無い code を読まない", () => {
  expect(() =>
    decodeApiError({ code: "unknown_code", message: "x" }, "error"),
  ).toThrow(DecodeFailure);
});

// ApiError の recordRef は省略可である。それを載せた応答を返す経路は backend に無いが、
// 読む側が省略可の項目として読めることを確かめる。
test("失敗に関わるレコード位置を含む失敗から、位置を読む", () => {
  const error = decodeApiError(apiErrorWithRecordRefJson(), "error");

  expect(error.code).toBe("internal_error");
  expect(error.recordRef?.sourceId).toBe(hostALogSourceId);
  expect(error.recordRef?.sourceContentSha256).toBe(hostALogSha256);
  expect(error.recordRef?.sourceFileName).toBe("host-a.log");
  expect(error.recordRef?.positionKind).toBe("sequence_number");
  expect(error.recordRef?.sequenceNumber).toBe(108);
  expect(error.recordRef?.lineNumber).toBe(1018);
  expect(error.missingParameters).toBeUndefined();
});

test("位置の値を欠いた recordRef を含む失敗を読まない", () => {
  expect(() =>
    decodeApiError(
      {
        code: "internal_error",
        message: "x",
        recordRef: {
          sourceId: hostALogSourceId,
          sourceContentSha256: hostALogSha256,
          sourceFileName: "host-a.log",
          positionKind: "sequence_number",
          recordRawTextRef: "/api/v0/records",
        },
      },
      "error",
    ),
  ).toThrow(DecodeFailure);
});

test("未実装の失敗は、未対応の機能のラベルを持ち、次の操作を持たない", () => {
  const failure = buildFetchFailure(
    "server",
    "レコードの取得",
    decodeApiError(
      { code: "not_implemented", message: "synthetic operation is absent" },
      "error",
    ),
  );

  expect(failure.failureDescription).toBe("未対応の機能");
  expect(failure.nextAction).toBe("なし");
  expect(failure.failureCode).toBe("not_implemented");
  expect(failure.sourceId).toBeUndefined();
});

// 画面へ渡す失敗は収集元の 2 つだけを載せる。recordRef を載せる先は画面に無い。
test("失敗に関わる収集元を、画面へ渡す失敗に載せる", () => {
  const failure = buildFetchFailure(
    "server",
    "元レコードを取得できませんでした。",
    decodeApiError(apiErrorWithRecordRefJson(), "error"),
  );

  expect(failure.failureCode).toBe("internal_error");
  expect(failure.sourceId).toBe(hostALogSourceId);
  expect(failure.sourceContentSha256).toBe(hostALogSha256);
  expect(failure.originPath).toBeUndefined();
  expect("recordRef" in failure).toBe(false);
});

test("退けた収集元の path を、画面へ渡す失敗に載せる", () => {
  const failure = buildFetchFailure(
    "request_rejected",
    "収集元の読み込みを始められませんでした。",
    decodeApiError(
      {
        code: "invalid_request",
        message: "x",
        originPath: "../outside/access.log",
      },
      "error",
    ),
  );

  expect(failure.failureCode).toBe("invalid_request");
  expect(failure.originPath).toBe("../outside/access.log");
  expect(failure.sourceId).toBeUndefined();
  expect(failure.rejectionDescription).toBeUndefined();
});

test("読み込みの要求を退けた理由を、短いラベルにして画面へ渡す失敗に載せる", () => {
  const failure = buildFetchFailure(
    "request_rejected",
    "収集元の読み込みの開始",
    decodeApiError(
      {
        code: "invalid_request",
        message: "x",
        loadingRejection: "file_absent",
        originPath: "logs/absent.log",
      },
      "error",
    ),
  );

  expect(failure.rejectionDescription).toBe("file なし");
  expect(failure.originPath).toBe("logs/absent.log");
});

test("定義の外の退けた理由を持つ失敗を読まない", () => {
  expect(() =>
    decodeApiError(
      { code: "invalid_request", message: "x", loadingRejection: "disk_full" },
      "error",
    ),
  ).toThrow("error.loadingRejection: expected one of");
});

test("未実装以外の失敗は、分類ごとの次に行える操作を書く", () => {
  const failure = buildFetchFailure(
    "request_rejected",
    "元レコードを取得できませんでした。",
    decodeApiError({ code: "record_not_found", message: "x" }, "error"),
  );

  expect(failure.nextAction).toBe("指定を直して再実行");
});
