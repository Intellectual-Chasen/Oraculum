import { type CaseId, decodeCaseId } from "./cases";
import {
  decodeRecordField,
  decodeRecordLocator,
  decodeRecordRange,
  decodeTimeRange,
  decodeTimestamp,
  type RecordField,
  type RecordLocator,
  type RecordRange,
  requireCountEqualsElements,
  type TimeRange,
  type Timestamp,
} from "./common";
import {
  DecodeFailure,
  type Decoder,
  optionalArray,
  optionalBoolean,
  optionalCount,
  optionalEnum,
  optionalMember,
  optionalString,
  readObject,
  rejectMember,
  requireArray,
  requireBoolean,
  requireCount,
  requireEnum,
  requireMember,
  requireString,
} from "./decoding";

/** 行末の byte 列。定義元は `backend/core/source_identity.go` の `LineEnding` である。 */
export const lineEndings = ["crlf", "lf", "mixed", "undetermined"] as const;
export type LineEnding = (typeof lineEndings)[number];

/** 入力形式を指す非空の識別子。 */
export type FormatKey = string;

/** 収集元の file を指す組。 */
export type SourceIdentity = {
  sourceId: string;
  contentSha256: string;
  originPath: string;
  fileName: string;
  sizeBytes: number;
  /** 出ない場合はレコード件数を確定できない。 */
  recordCount?: number;
  newlineCount: number;
  endsWithNewline: boolean;
  lineEnding: LineEnding;
  formatKey: FormatKey;
  /** 出ない場合はバージョンを読み取る項目が原資料に無い。 */
  formatVersion?: string;
  /** 収集元を読んだ欄の並びの指定。出ない場合は入力形式が並びの指定を取らない。 */
  formatSpec?: string;
  /**
   * 取り込みを求める側が収集元に付けた案件。出ない場合は、取り込みが案件を区別しない。
   * 定義元は `backend/core/source_identity.go` の `SourceIdentity.CaseId` である。
   */
  caseId?: CaseId;
  /**
   * 収集元を取り出した収集の directory。出ない場合は、収集元の file を 1 件ずつ指定した。
   * 定義元は `backend/core/source_identity.go` の `SourceIdentity.CollectionPath` である。
   */
  collectionPath?: string;
  /** 出ない場合は、UTC からのずれが定まる時刻を持つレコードが 0 件である。 */
  observedRangeFirst?: Timestamp;
  observedRangeLast?: Timestamp;
  /**
   * 書き出した端末が説明を組めなかったレコードの件数。出ない場合は、入力形式が説明を
   * 組めたかを持たない。
   */
  messageUnrenderedCount?: number;
  /**
   * レコードに現れた、収集元を記録した端末の候補。端末に決めた値ではない。出ない場合は、
   * この収集元が端末の候補を示していない。定義元は `backend/core/source_identity.go` の `TerminalCandidate` である。
   */
  terminalCandidates?: TerminalCandidate[];
  /**
   * 真の収集元では、レコードの原文は読み取りが収集元の byte 列から組み立てた文字列である。
   * 出ない場合、原文は収集元の byte 列そのものである。
   * 定義元は `backend/core/source_identity.go` の `SourceIdentity.RawTextConverted` である。
   */
  rawTextConverted?: boolean;
  /**
   * 収集元の file の見出しが記録した値。見出しを持たない入力形式と、見出しを読めなかった
   * 収集元では出ない。定義元は `backend/core/source_identity.go` の `SourceIdentity.FileHeader`
   * である。
   */
  fileHeader?: RecordField[];
  /**
   * 収集元を構成する file。先頭が主 file で、収集元の byte 列は並びの順に file を連結した
   * byte 列である。付属の file を一緒に読む入力形式の収集元だけが持つ。
   * 定義元は `backend/core/source_identity.go` の `SourceIdentity.Members` である。
   */
  members?: SourceMember[];
};

/** 収集元を構成する file 1 つ。定義元は `backend/core/source_identity.go` の `SourceMember` である。 */
export type SourceMember = {
  originPath: string;
  contentSha256: string;
  byteOffset: number;
  sizeBytes: number;
};

/** `SourceMember` を検証する。 */
const decodeSourceMember: Decoder<SourceMember> = (input, path) => {
  const source = readObject(input, path);
  return {
    originPath: requireString(source, "originPath", path),
    contentSha256: requireString(source, "contentSha256", path),
    byteOffset: requireCount(source, "byteOffset", path),
    sizeBytes: requireCount(source, "sizeBytes", path),
  };
};

/**
 * 収集元の byte 位置を持つ構成の file を返す。構成の file を持たない収集元と、どの file の
 * 範囲にも入らない位置では undefined である。
 */
export function memberAt(
  members: readonly SourceMember[] | undefined,
  byteOffset: number,
): SourceMember | undefined {
  return members?.find(
    (member) =>
      byteOffset >= member.byteOffset &&
      byteOffset < member.byteOffset + member.sizeBytes,
  );
}

/** 収集元を記録した端末の候補 1 つ。 */
export type TerminalCandidate = { name: string; recordCount: number };

/** `TerminalCandidate` を検証する。 */
const decodeTerminalCandidate: Decoder<TerminalCandidate> = (input, path) => {
  const source = readObject(input, path);
  return {
    name: requireString(source, "name", path),
    recordCount: requireCount(source, "recordCount", path),
  };
};

/** `SourceIdentity` を検証する。 */
export const decodeSourceIdentity: Decoder<SourceIdentity> = (input, path) => {
  const source = readObject(input, path);
  const formatKey = requireString(source, "formatKey", path);
  if (formatKey === "") {
    throw new DecodeFailure(`${path}.formatKey`, "expected a non-empty string");
  }
  return {
    sourceId: requireString(source, "sourceId", path),
    contentSha256: requireString(source, "contentSha256", path),
    originPath: requireString(source, "originPath", path),
    fileName: requireString(source, "fileName", path),
    sizeBytes: requireCount(source, "sizeBytes", path),
    recordCount: optionalCount(source, "recordCount", path),
    newlineCount: requireCount(source, "newlineCount", path),
    endsWithNewline: requireBoolean(source, "endsWithNewline", path),
    lineEnding: requireEnum(source, "lineEnding", path, lineEndings),
    formatKey,
    formatVersion: optionalString(source, "formatVersion", path),
    formatSpec: optionalString(source, "formatSpec", path),
    caseId: optionalMember(source, "caseId", path, decodeCaseId),
    collectionPath: optionalString(source, "collectionPath", path),
    observedRangeFirst: optionalMember(
      source,
      "observedRangeFirst",
      path,
      decodeTimestamp,
    ),
    observedRangeLast: optionalMember(
      source,
      "observedRangeLast",
      path,
      decodeTimestamp,
    ),
    messageUnrenderedCount: optionalCount(
      source,
      "messageUnrenderedCount",
      path,
    ),
    terminalCandidates: optionalArray(
      source,
      "terminalCandidates",
      path,
      decodeTerminalCandidate,
    ),
    rawTextConverted: optionalBoolean(source, "rawTextConverted", path),
    fileHeader: optionalArray(source, "fileHeader", path, decodeRecordField),
    members: optionalArray(source, "members", path, decodeSourceMember),
  };
};

/** 取り込み結果の区分。定義元は `backend/core/import_status.go` の `ImportCategory` である。 */
export const importCategories = ["read", "succeeded", "failed"] as const;
export type ImportCategory = (typeof importCategories)[number];

/** 失敗原因の分類。同節の 4 値。 */
export const diagnosisClasses = [
  "undetermined",
  "unsupported_format",
  "inconsistent_input_confirmed",
  "implementation_defect_confirmed",
] as const;
export type DiagnosisClass = (typeof diagnosisClasses)[number];

/** 取り込み結果の区分 1 つの件数。 */
export type ImportCount = { category: ImportCategory; count: number };

/** `ImportCount` を検証する。 */
export const decodeImportCount: Decoder<ImportCount> = (input, path) => {
  const source = readObject(input, path);
  return {
    category: requireEnum(source, "category", path, importCategories),
    count: requireCount(source, "count", path),
  };
};

/** 失敗原因の分類 1 つの件数。 */
export type DiagnosisCount = { diagnosisClass: DiagnosisClass; count: number };

/** `DiagnosisCount` を検証する。 */
export const decodeDiagnosisCount: Decoder<DiagnosisCount> = (input, path) => {
  const source = readObject(input, path);
  return {
    diagnosisClass: requireEnum(
      source,
      "diagnosisClass",
      path,
      diagnosisClasses,
    ),
    count: requireCount(source, "count", path),
  };
};

/** 失敗した処理段階。定義元は `backend/core/import_status.go` の `FailureStage` である。 */
export const failureStages = [
  "read",
  "tokenize",
  "field_map",
  "normalize",
  "relate",
] as const;
export type FailureStage = (typeof failureStages)[number];

/**
 * 取り込めなかったレコード 1 件。定義元は `backend/core/import_status.go` の `ImportFailure`
 * である。画面が出す項目だけを持つ。
 */
export type ImportFailure = {
  /** 出ない場合はレコードの位置を確定できていない。 */
  recordRef?: RecordLocator;
  /** 出ない場合は行番号を確定できていない。1 起点。 */
  lineNumber?: number;
  /** 出ない場合は byte の位置を数えていない。 */
  byteOffset?: number;
  diagnosisClass: DiagnosisClass;
  stage: FailureStage;
  expectedMeaning: string;
  observedResult: string;
  /** `diagnosisClass` が `undetermined` のとき出る。 */
  unresolvedReason?: string;
  /**
   * 原文を返す操作 (`GET /api/v0/raw-texts`) への参照。`recordRef` があるとき出る。
   */
  rawTextRef?: string;
  /**
   * レコードが書き出した側の 1 行の長さの上限で終わり、途中で切れているか。真のレコードは、
   * 切れる前に読めた欄を持つレコードとしても取り込んでいる。
   */
  recordTruncated?: boolean;
};

/** `ImportFailure` を検証する。 */
const decodeImportFailure: Decoder<ImportFailure> = (input, path) => {
  const source = readObject(input, path);
  const diagnosisClass = requireEnum(
    source,
    "diagnosisClass",
    path,
    diagnosisClasses,
  );
  const unresolvedReason = optionalString(source, "unresolvedReason", path);
  if (diagnosisClass === "undetermined" && unresolvedReason === undefined) {
    throw new DecodeFailure(
      `${path}.unresolvedReason`,
      "expected a reason while diagnosisClass is undetermined",
    );
  }
  return {
    recordRef: optionalMember(source, "recordRef", path, decodeRecordLocator),
    lineNumber: optionalCount(source, "lineNumber", path),
    byteOffset: optionalCount(source, "byteOffset", path),
    diagnosisClass,
    stage: requireEnum(source, "stage", path, failureStages),
    expectedMeaning: requireString(source, "expectedMeaning", path),
    observedResult: requireString(source, "observedResult", path),
    unresolvedReason,
    rawTextRef: optionalString(source, "rawTextRef", path),
    recordTruncated: optionalBoolean(source, "recordTruncated", path),
  };
};

/** 公開の状態。定義元は `backend/core/candidate.go` の `PublicationState` である。 */
export const publicationStates = [
  "published_full",
  "published_partial",
  "withheld",
] as const;
export type PublicationState = (typeof publicationStates)[number];

/** 公開を止めた理由。同節の 3 値。 */
export const withheldReasons = [
  "identifier_collision",
  "dangling_evidence_reference",
  "mixed_analysis_run",
] as const;
export type WithheldReason = (typeof withheldReasons)[number];

/** 収集元 1 件の取り込みの状態。 */
export type ImportStatus = {
  sourceId: string;
  scope: RecordRange;
  counts: ImportCount[];
  diagnosisCounts: DiagnosisCount[];
  publicationState: PublicationState;
  /** `publicationState` が `withheld` のとき出る。 */
  withheldReason?: WithheldReason;
  /** 取り込めなかったレコードの全件。応答の `failureCount` と要素数が一致する。 */
  failures: ImportFailure[];
  analysisRunRef: string;
};

/** `ImportStatus` を検証する。 */
export const decodeImportStatus: Decoder<ImportStatus> = (input, path) => {
  const source = readObject(input, path);
  const failures = requireArray(source, "failures", path, decodeImportFailure);
  requireCountEqualsElements(
    requireCount(source, "failureCount", path),
    failures.length,
    `${path}.failureCount`,
  );
  const publicationState = requireEnum(
    source,
    "publicationState",
    path,
    publicationStates,
  );
  const withheldReason = optionalEnum(
    source,
    "withheldReason",
    path,
    withheldReasons,
  );
  if (publicationState === "withheld" && withheldReason === undefined) {
    throw new DecodeFailure(
      `${path}.withheldReason`,
      "expected a reason while publicationState is withheld",
    );
  }
  return {
    sourceId: requireString(source, "sourceId", path),
    scope: requireMember(source, "scope", path, decodeRecordRange),
    counts: requireArray(source, "counts", path, decodeImportCount),
    diagnosisCounts: requireArray(
      source,
      "diagnosisCounts",
      path,
      decodeDiagnosisCount,
    ),
    publicationState,
    withheldReason,
    failures,
    analysisRunRef: requireString(source, "analysisRunRef", path),
  };
};

/** `sourceCount` が 0 になった理由。操作 1 は 1 値だけを取る。 */
export const sourcesEmptyReasons = ["no_source_ingested"] as const;
export type SourcesEmptyReason = (typeof sourcesEmptyReasons)[number];

/**
 * 収集の directory の file を取り込まなかった理由。定義元は `backend/core/skipped_file.go` の
 * `SkippedFileReason` である。
 */
export const skippedFileReasons = [
  "unsupported_format",
  "empty_file",
  "not_regular_file",
  "companion_without_main",
] as const;
export type SkippedFileReason = (typeof skippedFileReasons)[number];

/** 収集の directory にあり、取り込まなかった file 1 つ。定義元は `backend/core/skipped_file.go` の `SkippedFile` である。 */
export type SkippedFile = {
  originPath: string;
  reason: SkippedFileReason;
  /** 先頭の byte 列から分かった file の種類。分からない file では出ない。 */
  detectedKind?: string;
};

/** `SkippedFile` を検証する。 */
const decodeSkippedFile: Decoder<SkippedFile> = (input, path) => {
  const source = readObject(input, path);
  return {
    originPath: requireString(source, "originPath", path),
    reason: requireEnum(source, "reason", path, skippedFileReasons),
    detectedKind: optionalString(source, "detectedKind", path),
  };
};

/** 操作 1 (`GET /api/v0/sources`) の応答。 */
export type SourcesResponse = {
  sources: SourceWithImportStatus[];
  /** 収集元の総数。 */
  sourceCount: number;
  /** `sourceCount` が 0 のとき出る。 */
  emptyReason?: SourcesEmptyReason;
  /** 収集の directory にあり取り込まなかった file。収集の directory を指定しなかった取り込みでは空である。 */
  skippedFiles: SkippedFile[];
};

/** 収集元と、その収集元の取り込みの状態を一組で持つ。 */
export type SourceWithImportStatus = {
  source: SourceIdentity;
  importStatus: ImportStatus;
  /**
   * 観測期間を持たない収集元を、分析者の時刻の解釈で読んだ観測期間。UTC の時点で書く。
   * 解釈を持たない収集元と、観測期間を持つ収集元では出ない。定義元は
   * `backend/api/sources.go` の `sourceResponseItem.InterpretedObservedRange` である。
   */
  interpretedObservedRange?: TimeRange;
  /**
   * 取り込みの起動で収集元に指定した UTC からのずれ (例 `+09:00`)。分析者が解釈を記録すると、
   * 時刻は記録した解釈で読み、この値は指定した値のまま出る。指定していない収集元では出ない。
   * 定義元は `backend/api/sources.go` の `sourceResponseItem.ImportTimeOffset` である。
   */
  importTimeOffset?: string;
  /**
   * 説明を組めなかったレコードの位置。収集元の中の順に並び、先頭の上限の件数までを持つ。
   * 件数の総数は `source.messageUnrenderedCount` である。定義元は `backend/api/sources.go` の
   * `sourceResponseItem.MessageUnrenderedRecordRefs` である。
   */
  messageUnrenderedRecordRefs?: RecordLocator[];
};

const decodeSourceWithImportStatus: Decoder<SourceWithImportStatus> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const sourceIdentity = requireMember(
    source,
    "source",
    path,
    decodeSourceIdentity,
  );
  const importStatus = requireMember(
    source,
    "importStatus",
    path,
    decodeImportStatus,
  );
  if (sourceIdentity.sourceId !== importStatus.sourceId) {
    throw new DecodeFailure(
      `${path}.importStatus.sourceId`,
      "expected sourceId to match source.sourceId",
    );
  }
  return {
    source: sourceIdentity,
    importStatus,
    interpretedObservedRange: optionalMember(
      source,
      "interpretedObservedRange",
      path,
      decodeTimeRange,
    ),
    importTimeOffset: optionalString(source, "importTimeOffset", path),
    messageUnrenderedRecordRefs: optionalArray(
      source,
      "messageUnrenderedRecordRefs",
      path,
      decodeRecordLocator,
    ),
  };
};

/** 操作 1 の応答を検証する。 */
export const decodeSourcesResponse: Decoder<SourcesResponse> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const sources = requireArray(
    source,
    "sources",
    path,
    decodeSourceWithImportStatus,
  );
  const sourceCount = requireCount(source, "sourceCount", path);
  let emptyReason: SourcesEmptyReason | undefined;
  if (sourceCount === 0) {
    emptyReason = requireEnum(source, "emptyReason", path, sourcesEmptyReasons);
  } else {
    rejectMember(
      source,
      "emptyReason",
      path,
      "expected no reason while sourceCount is 1 or more",
    );
  }
  requireCountEqualsElements(
    sourceCount,
    sources.length,
    `${path}.sourceCount`,
  );
  return {
    sources,
    sourceCount,
    emptyReason,
    skippedFiles: requireArray(source, "skippedFiles", path, decodeSkippedFile),
  };
};
