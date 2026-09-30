/**
 * 操作 1 (`GET /api/v0/sources`) の応答の JSON。
 * file 名・sha256・レコード件数・観測範囲の時刻・`sourceId`・byte 数・解析実行の参照は、
 * 本 fixture が決める値である。
 */

/** `access.log` の内容の識別。 */
export const accessLogSha256 = "a".repeat(64);

/** `host-a.log` の内容の識別。 */
export const hostALogSha256 = "b".repeat(64);

/** `access.log` の取り込み 1 件。 */
export const accessLogSourceId = "ingest-access-log-1";

/** `host-a.log` の取り込み 1 件。 */
export const hostALogSourceId = "ingest-host-a-log-1";

/** レコード件数と両端の位置を確定できた収集元。 */
export function accessLogSource() {
  return {
    sourceId: accessLogSourceId,
    contentSha256: accessLogSha256,
    originPath: "/data/example/proxy/access.log",
    fileName: "access.log",
    sizeBytes: 500000,
    recordCount: 1200,
    newlineCount: 1199,
    endsWithNewline: false,
    lineEnding: "lf",
    formatKey: "squid_combined",
    formatSpec:
      '%>a %[ui %[un [%tl] "%rm %ru HTTP/%rv" %>Hs %<st "%{Referer}>h" "%{User-Agent}>h" %Ss:%Sh',
    observedRangeFirst: {
      rawText: "[08/Oct/2031:10:20:35 +0900]",
      normalized: "2031-10-08T10:20:35+09:00",
      normalizedForm: "rfc3339_absolute",
      precision: "second",
      offsetState: "in_value",
      offsetText: "+0900",
      clock: "observer_local",
      meaning: "event",
      valueState: "present",
    },
  };
}

/** レコード件数を確定できず、公開を停止した収集元。 */
export function hostALogSource() {
  return {
    sourceId: hostALogSourceId,
    contentSha256: hostALogSha256,
    originPath: "/data/example/endpoint/host-a.log",
    fileName: "host-a.log",
    sizeBytes: 9000000,
    newlineCount: 4000,
    endsWithNewline: true,
    lineEnding: "crlf",
    formatKey: "infotrace_mark_ii",
  };
}

function accessLogImportStatus() {
  return {
    sourceId: accessLogSourceId,
    scope: {
      sourceId: accessLogSourceId,
      sourceContentSha256: accessLogSha256,
      rangeKind: "positioned",
      positionKind: "line_number",
      fromPosition: 1,
      toPosition: 1200,
    },
    counts: [
      { category: "read", count: 1200 },
      { category: "succeeded", count: 1200 },
    ],
    diagnosisCounts: [],
    publicationState: "published_full",
    failures: [],
    failureCount: 0,
    analysisRunRef: "analysis-run-1",
  };
}

/**
 * 識別子が衝突したレコード 1 件の取り込み失敗。
 * 項目は `backend/core/import_status.go` の `ImportFailure` が持つ。
 */
function hostALogImportFailure(sequenceNumber: number) {
  return {
    sourceId: hostALogSourceId,
    sourceContentSha256: hostALogSha256,
    recordRef: {
      sourceId: hostALogSourceId,
      sourceContentSha256: hostALogSha256,
      sourceFileName: "host-a.log",
      positionKind: "sequence_number",
      sequenceNumber,
      recordRawTextRef: `raw:host-a-${sequenceNumber}`,
    },
    rawTextRef: `raw:host-a-${sequenceNumber}`,
    diagnosisClass: "undetermined",
    stage: "relate",
    interpretation: "a record identifier repeats inside one source",
    expectedMeaning: "each record identifier points at one record",
    observedResult: "two records carry the same identifier",
    unresolvedReason: "the source does not carry a tie-breaking item",
    parserVersion: "test-parser-0",
    sanitizedMessage: "duplicate record identifier",
  };
}

function hostALogImportStatus() {
  return {
    sourceId: hostALogSourceId,
    scope: {
      sourceId: hostALogSourceId,
      sourceContentSha256: hostALogSha256,
      rangeKind: "whole_source",
    },
    counts: [],
    diagnosisCounts: [{ diagnosisClass: "undetermined", count: 2 }],
    publicationState: "withheld",
    withheldReason: "identifier_collision",
    failures: [hostALogImportFailure(112), hostALogImportFailure(113)],
    failureCount: 2,
    analysisRunRef: "analysis-run-1",
  };
}

/**
 * bidi 制御 (U+202E) を持つ収集元の file 名。
 * 原資料の file 名が書式文字を持つ場合の画面の表現を確かめるため、本 fixture が生成する。
 */
export const invisibleCharacterFileName = "access‮gol.log";

/** file 名に bidi 制御を持つ収集元 1 件を含む応答。 */
export function invisibleFileNameSourcesResponseJson(): unknown {
  return {
    sources: [
      {
        source: { ...accessLogSource(), fileName: invisibleCharacterFileName },
        importStatus: accessLogImportStatus(),
      },
    ],
    sourceCount: 1,
    skippedFiles: [],
  };
}

/** 収集元 2 件を含む応答。 */
export function sourcesResponseJson(): unknown {
  return {
    sources: [
      { source: accessLogSource(), importStatus: accessLogImportStatus() },
      { source: hostALogSource(), importStatus: hostALogImportStatus() },
    ],
    sourceCount: 2,
    skippedFiles: [],
  };
}

/** 収集の directory から取り込み、取り込まなかった file 2 つを持つ応答。 */
export function collectionSourcesResponseJson(): unknown {
  return {
    sources: [
      {
        source: { ...hostALogSource(), collectionPath: "triage" },
        importStatus: hostALogImportStatus(),
      },
    ],
    sourceCount: 1,
    skippedFiles: [
      {
        originPath: "triage/notes.txt",
        reason: "unsupported_format",
        detectedKind: "text",
      },
      { originPath: "triage/empty.dat", reason: "empty_file" },
    ],
  };
}

/**
 * 収集元 2 件を含みながら、`sourceCount` が 3 件を名乗る応答。
 * 一覧に出ていない収集元があることを、総数と要素数の食い違いが表す。
 */
export function countMismatchSourcesResponseJson(): unknown {
  return {
    sources: [
      { source: accessLogSource(), importStatus: accessLogImportStatus() },
      { source: hostALogSource(), importStatus: hostALogImportStatus() },
    ],
    sourceCount: 3,
    skippedFiles: [],
  };
}

/**
 * 取り込み失敗 2 件を含みながら、`failureCount` が 3 件を名乗る応答。
 * 収集元 1 件の取り込みの状態の中で、総数と要素数が食い違う。
 */
export function failureCountMismatchSourcesResponseJson(): unknown {
  return {
    sources: [
      {
        source: hostALogSource(),
        importStatus: { ...hostALogImportStatus(), failureCount: 3 },
      },
    ],
    sourceCount: 1,
    skippedFiles: [],
  };
}

/** 収集元を 1 件も取り込んでいない応答。 */
export function emptySourcesResponseJson(): unknown {
  return {
    sources: [],
    sourceCount: 0,
    emptyReason: "no_source_ingested",
    skippedFiles: [],
  };
}

/** 失敗の応答 1 件。 */
export function apiErrorJson(code: string, sourceId?: string): unknown {
  return sourceId === undefined
    ? { code, message: "the request could not be served" }
    : {
        code,
        message: "the request could not be served",
        sourceId,
        sourceContentSha256: accessLogSha256,
      };
}
