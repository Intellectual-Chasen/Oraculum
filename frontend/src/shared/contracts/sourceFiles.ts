import {
  DecodeFailure,
  type Decoder,
  optionalCount,
  optionalMember,
  optionalString,
  readObject,
  requireArray,
  requireBoolean,
  requireEnum,
  requireString,
} from "./decoding";
import type { FormatKey } from "./sources";

/** 基準の directory の下の項目の種類。定義元は `backend/core/source_file.go` の `SourceFileKind` である。 */
export const sourceFileKinds = ["directory", "file", "other"] as const;
export type SourceFileKind = (typeof sourceFileKinds)[number];

/**
 * file の入力形式の候補が無い理由。定義元は `backend/core/source_file.go` の
 * `SourceFileUndetectedReason` である。
 */
export const sourceFileUndetectedReasons = [
  "unsupported_format",
  "empty_file",
  "unreadable",
  "companion_file",
] as const;
export type SourceFileUndetectedReason =
  (typeof sourceFileUndetectedReasons)[number];

/** file の入力形式の候補が無い理由。定義元は同 file の `SourceFileUndetected` である。 */
export type SourceFileUndetected = {
  reason: SourceFileUndetectedReason;
  /** 先頭の byte 列から分かった file の種類。分からない file では出ない。 */
  detectedKind?: string;
};

/** 基準の directory の下の項目 1 つ。定義元は同 file の `SourceFileEntry` である。 */
export type SourceFileEntry = {
  /** 基準の directory からの相対 path。読み込みの要求の `originPath` にそのまま渡せる。 */
  originPath: string;
  name: string;
  kind: SourceFileKind;
  /** file の byte 数。file だけが持つ。 */
  sizeBytes?: number;
  /** file を読める入力形式の候補。候補が無い file と、file でない項目では要素数 0 である。 */
  formatCandidates: FormatKey[];
  /** file の候補が無い理由。 */
  undetected?: SourceFileUndetected;
};

/**
 * 基準の directory の下の directory 1 つの項目の一覧 (`GET /api/v0/stages/source-files`)。
 * 定義元は `backend/core/source_file.go` の `SourceFileListing` である。
 */
export type SourceFileListing = {
  /** 一覧にした directory。基準の directory そのものは `.` である。 */
  path: string;
  /** 下の directory の file も含めたか。真の一覧は directory を持たない。 */
  recursive: boolean;
  entries: SourceFileEntry[];
  /** 項目の数が上限に達し、残りの項目を含めなかったか。 */
  truncated: boolean;
};

const decodeUndetected: Decoder<SourceFileUndetected> = (input, path) => {
  const source = readObject(input, path);
  return {
    reason: requireEnum(source, "reason", path, sourceFileUndetectedReasons),
    detectedKind: optionalString(source, "detectedKind", path),
  };
};

function requireNonEmptyString(
  source: Record<string, unknown>,
  key: string,
  path: string,
): string {
  const value = requireString(source, key, path);
  if (value === "") {
    throw new DecodeFailure(`${path}.${key}`, "expected a non-empty string");
  }
  return value;
}

const decodeEntry: Decoder<SourceFileEntry> = (input, path) => {
  const source = readObject(input, path);
  const kind = requireEnum(source, "kind", path, sourceFileKinds);
  const entry: SourceFileEntry = {
    originPath: requireNonEmptyString(source, "originPath", path),
    name: requireNonEmptyString(source, "name", path),
    kind,
    sizeBytes: optionalCount(source, "sizeBytes", path),
    formatCandidates:
      source.formatCandidates === undefined
        ? []
        : requireArray(source, "formatCandidates", path, (value, at) => {
            if (typeof value !== "string" || value === "") {
              throw new DecodeFailure(at, "expected a non-empty string");
            }
            return value;
          }),
    undetected: optionalMember(source, "undetected", path, decodeUndetected),
  };
  // file は大きさを持ち、候補と候補が無い理由のどちらか一方を持つ。file でない項目はどれも持たない。
  const hasCandidates = entry.formatCandidates.length > 0;
  const hasUndetected = entry.undetected !== undefined;
  const hasSize = entry.sizeBytes !== undefined;
  if (kind === "file" && !hasSize) {
    throw new DecodeFailure(`${path}.sizeBytes`, "expected a size on a file");
  }
  if (kind === "file" && hasCandidates === hasUndetected) {
    throw new DecodeFailure(
      path,
      "expected a file to carry exactly one of formatCandidates and undetected",
    );
  }
  if (kind !== "file" && (hasCandidates || hasUndetected || hasSize)) {
    throw new DecodeFailure(path, "expected no file items on a non-file");
  }
  return entry;
};

/** 保存した資料と関連する付属資料。定義元は `backend/core/source_file.go` の `SourceUploadResult`。 */
export type SourceUploadResult = { entries: SourceFileEntry[] };

/** アップロードした資料の判定を検証する。 */
export const decodeSourceUploadResult: Decoder<SourceUploadResult> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return { entries: requireArray(source, "entries", path, decodeEntry) };
};

/** `SourceFileListing` を検証する。整合の条件は backend の `SourceFileListing.Validate` に対応する。 */
export const decodeSourceFileListing: Decoder<SourceFileListing> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const recursive = requireBoolean(source, "recursive", path);
  const entries = requireArray(source, "entries", path, decodeEntry);
  if (recursive && entries.some((entry) => entry.kind === "directory")) {
    throw new DecodeFailure(
      `${path}.entries`,
      "expected no directory in a recursive listing",
    );
  }
  return {
    path: requireNonEmptyString(source, "path", path),
    recursive,
    entries,
    truncated: requireBoolean(source, "truncated", path),
  };
};
