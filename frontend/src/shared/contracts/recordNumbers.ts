import { decodeRecordLocator, type RecordLocator } from "./common";
import {
  DecodeFailure,
  type Decoder,
  decodeString,
  optionalCount,
  optionalEnum,
  optionalMember,
  optionalString,
  readObject,
  requireArray,
  requireCount,
  requireEnum,
  requireMember,
  requireString,
} from "./decoding";

// 定義元は `backend/core/record_numbering.go` と `backend/api/record_numbers.go` である。
// backend は `Number.MAX_SAFE_INTEGER` を超える番号を読めない番号の件数に入れ、応答の番号は
// どれも安全な整数である (`requireCount` で読む)。

export const recordNumberExaminations = [
  "gaps_found",
  "no_gaps",
  "not_examined",
] as const;
export type RecordNumberExamination = (typeof recordNumberExaminations)[number];

export const recordNumberNotExaminedReasons = [
  "record_numbers_not_declared",
  "record_numbers_unreadable",
] as const;
export type RecordNumberNotExaminedReason =
  (typeof recordNumberNotExaminedReasons)[number];

export type RecordNumberGap = {
  firstMissingNumber: number;
  lastMissingNumber: number;
  precedingRecordRef: RecordLocator;
  followingRecordRef: RecordLocator;
  /** 抜けに結んだ取り込みの失敗のレコードの全件数。 */
  failureRecordCount: number;
  /** 結んだ失敗のレコードの位置。先頭の `listLimit` 件まで。 */
  failureRecordRefs: RecordLocator[];
};

/** 単位の中で 1 つの Computer を名乗ったレコードの件数。 */
export type RecordNumberComputer = { computer?: string; recordCount: number };

/** 番号を振った 1 つの単位 (同じ収集元の同じチャネル)。 */
export type RecordNumberStream = {
  channel?: string;
  computers: RecordNumberComputer[];
  recordCount: number;
  lowestNumber: number;
  highestNumber: number;
  missingNumberCount: number;
  duplicatedRecordCount: number;
  /** 抜けた番号の範囲の全個数。 */
  gapCount: number;
  /** 抜けた番号の範囲。先頭の `listLimit` 件まで。 */
  gaps: RecordNumberGap[];
};

export const recordNumberHeaderComparisons = [
  "agrees",
  "differs",
  "no_record_numbers",
  "header_unreadable",
] as const;
export type RecordNumberHeaderComparison =
  (typeof recordNumberHeaderComparisons)[number];

export type RecordNumberFileHeader = {
  /** 出ない場合は見出しの番号を読めなかった。 */
  nextRecordNumber?: number;
  /** 出ない場合はレコードの見出しの番号を読めたレコードが 0 件。 */
  highestRecordNumber?: number;
  unreadableRecordHeaderCount: number;
  comparison: RecordNumberHeaderComparison;
  dirty?: string;
};

export type RecordNumbers = {
  sourceId: string;
  examination: RecordNumberExamination;
  notExaminedReason?: RecordNumberNotExaminedReason;
  unreadableRecordCount?: number;
  streams: RecordNumberStream[];
  fileHeader?: RecordNumberFileHeader;
  listLimit: number;
};

export const recordComparisonKeys = [
  "channel_computer_record_number",
  "second_provider_event_id",
] as const;
export type RecordComparisonKey = (typeof recordComparisonKeys)[number];

export const recordNotComparedReasons = [
  "comparison_key_not_declared",
  "time_offset_undetermined",
] as const;
export type RecordNotComparedReason = (typeof recordNotComparedReasons)[number];

export type RecordComparison = {
  comparedSourceId: string;
  state: "compared" | "not_compared";
  notComparedReason?: RecordNotComparedReason;
  key?: RecordComparisonKey;
  oneToOneKeyCount: number;
  onlyInSourceRecordCount: number;
  onlyInComparedRecordCount: number;
  /** 片方にしか無いレコードの位置。先頭の `listLimit` 件まで。 */
  onlyInSourceRecordRefs: RecordLocator[];
  onlyInComparedRecordRefs: RecordLocator[];
  /**
   * 片方にしか無いレコードを、相手の収録範囲の外と内に分けた件数と、範囲の内のレコードの位置。
   * 外と内の和が片方にしか無い件数より少ない差は、範囲を決められなかった件数である。
   */
  onlyInSourceOutsideComparedRangeRecordCount: number;
  onlyInSourceInsideComparedRangeRecordCount: number;
  onlyInSourceInsideComparedRangeRecordRefs: RecordLocator[];
  onlyInComparedOutsideSourceRangeRecordCount: number;
  onlyInComparedInsideSourceRangeRecordCount: number;
  onlyInComparedInsideSourceRangeRecordRefs: RecordLocator[];
  listLimit: number;
  undeterminedKeyCount: number;
  undeterminedSourceRecordCount: number;
  undeterminedComparedRecordCount: number;
  unequalKeyCount: number;
  sourceSurplusRecordCount: number;
  comparedSurplusRecordCount: number;
  /** 件数の違う鍵ごとに、鍵に入れていない欄の値で分けた結果。先頭の `listLimit` 個まで。 */
  unequalKeys: RecordComparisonUnequalKey[];
  sourceUnkeyedRecordCount: number;
  comparedUnkeyedRecordCount: number;
};

export const unequalKeyOutcomes = [
  "identified",
  "no_compared_values",
  "undecided",
] as const;
export type UnequalKeyOutcome = (typeof unequalKeyOutcomes)[number];

/** 件数の違う鍵 1 つを、鍵に入れていない語彙の項目の値まで比べた結果。 */
export type RecordComparisonUnequalKey = {
  outcome: UnequalKeyOutcome;
  /** 値を比べた語彙の項目。 */
  comparedSemantics: string[];
  /** その鍵を持つレコード全件の位置。片方にだけあるレコードを特定できないときの候補。 */
  sourceRecordRefs: RecordLocator[];
  comparedRecordRefs: RecordLocator[];
  /** 比べた値の組が相手のどのレコードとも対にならないレコードの位置。 */
  onlyInSourceRecordRefs: RecordLocator[];
  onlyInComparedRecordRefs: RecordLocator[];
};

const decodeUnequalKey: Decoder<RecordComparisonUnequalKey> = (input, path) => {
  const key = readObject(input, path);
  const refs = (name: string) =>
    requireArray(key, name, path, decodeRecordLocator);
  return {
    outcome: requireEnum(key, "outcome", path, unequalKeyOutcomes),
    comparedSemantics: requireArray(
      key,
      "comparedSemantics",
      path,
      decodeString,
    ),
    sourceRecordRefs: refs("sourceRecordRefs"),
    comparedRecordRefs: refs("comparedRecordRefs"),
    onlyInSourceRecordRefs: refs("onlyInSourceRecordRefs"),
    onlyInComparedRecordRefs: refs("onlyInComparedRecordRefs"),
  };
};

export type RecordNumbersResponse = {
  recordNumbers: RecordNumbers;
  comparison?: RecordComparison;
};

const decodeGap: Decoder<RecordNumberGap> = (input, path) => {
  const gap = readObject(input, path);
  return {
    firstMissingNumber: requireCount(gap, "firstMissingNumber", path),
    lastMissingNumber: requireCount(gap, "lastMissingNumber", path),
    precedingRecordRef: requireMember(
      gap,
      "precedingRecordRef",
      path,
      decodeRecordLocator,
    ),
    followingRecordRef: requireMember(
      gap,
      "followingRecordRef",
      path,
      decodeRecordLocator,
    ),
    failureRecordCount: requireCount(gap, "failureRecordCount", path),
    failureRecordRefs: requireArray(
      gap,
      "failureRecordRefs",
      path,
      decodeRecordLocator,
    ),
  };
};

const decodeComputer: Decoder<RecordNumberComputer> = (input, path) => {
  const computer = readObject(input, path);
  return {
    computer: optionalString(computer, "computer", path),
    recordCount: requireCount(computer, "recordCount", path),
  };
};

const decodeStream: Decoder<RecordNumberStream> = (input, path) => {
  const stream = readObject(input, path);
  return {
    channel: optionalString(stream, "channel", path),
    computers: requireArray(stream, "computers", path, decodeComputer),
    recordCount: requireCount(stream, "recordCount", path),
    lowestNumber: requireCount(stream, "lowestNumber", path),
    highestNumber: requireCount(stream, "highestNumber", path),
    missingNumberCount: requireCount(stream, "missingNumberCount", path),
    duplicatedRecordCount: requireCount(stream, "duplicatedRecordCount", path),
    gapCount: requireCount(stream, "gapCount", path),
    gaps: requireArray(stream, "gaps", path, decodeGap),
  };
};

const decodeFileHeader: Decoder<RecordNumberFileHeader> = (input, path) => {
  const header = readObject(input, path);
  return {
    nextRecordNumber: optionalCount(header, "nextRecordNumber", path),
    highestRecordNumber: optionalCount(header, "highestRecordNumber", path),
    unreadableRecordHeaderCount: requireCount(
      header,
      "unreadableRecordHeaderCount",
      path,
    ),
    comparison: requireEnum(
      header,
      "comparison",
      path,
      recordNumberHeaderComparisons,
    ),
    dirty: optionalString(header, "dirty", path),
  };
};

const decodeRecordNumbers: Decoder<RecordNumbers> = (input, path) => {
  const numbers = readObject(input, path);
  return {
    sourceId: requireString(numbers, "sourceId", path),
    examination: requireEnum(
      numbers,
      "examination",
      path,
      recordNumberExaminations,
    ),
    notExaminedReason: optionalEnum(
      numbers,
      "notExaminedReason",
      path,
      recordNumberNotExaminedReasons,
    ),
    unreadableRecordCount: optionalCount(
      numbers,
      "unreadableRecordCount",
      path,
    ),
    streams: requireArray(numbers, "streams", path, decodeStream),
    fileHeader: optionalMember(numbers, "fileHeader", path, decodeFileHeader),
    listLimit: requireCount(numbers, "listLimit", path),
  };
};

/**
 * 範囲の外と内の件数の和が片方にしか無い件数を超える応答と、範囲の内の位置が件数か上限を
 * 超える応答を退ける。
 */
function checkRangeSplit(
  path: string,
  side: string,
  onlyIn: number,
  outside: number,
  inside: number,
  insideRefs: number,
  listLimit: number,
) {
  if (outside + inside > onlyIn) {
    throw new DecodeFailure(
      `${path}.${side}`,
      "expected the outside and inside counts to sum to at most the only-in count",
    );
  }
  if (insideRefs > Math.min(inside, listLimit)) {
    throw new DecodeFailure(
      `${path}.${side}`,
      "expected at most the inside count and the list limit of refs",
    );
  }
}

const decodeComparison: Decoder<RecordComparison> = (input, path) => {
  const decoded = decodeComparisonItems(input, path);
  checkRangeSplit(
    path,
    "onlyInSourceInsideComparedRangeRecordRefs",
    decoded.onlyInSourceRecordCount,
    decoded.onlyInSourceOutsideComparedRangeRecordCount,
    decoded.onlyInSourceInsideComparedRangeRecordCount,
    decoded.onlyInSourceInsideComparedRangeRecordRefs.length,
    decoded.listLimit,
  );
  checkRangeSplit(
    path,
    "onlyInComparedInsideSourceRangeRecordRefs",
    decoded.onlyInComparedRecordCount,
    decoded.onlyInComparedOutsideSourceRangeRecordCount,
    decoded.onlyInComparedInsideSourceRangeRecordCount,
    decoded.onlyInComparedInsideSourceRangeRecordRefs.length,
    decoded.listLimit,
  );
  return decoded;
};

const decodeComparisonItems: Decoder<RecordComparison> = (input, path) => {
  const comparison = readObject(input, path);
  const count = (key: string) => requireCount(comparison, key, path);
  return {
    comparedSourceId: requireString(comparison, "comparedSourceId", path),
    state: requireEnum(comparison, "state", path, [
      "compared",
      "not_compared",
    ] as const),
    notComparedReason: optionalEnum(
      comparison,
      "notComparedReason",
      path,
      recordNotComparedReasons,
    ),
    key: optionalEnum(comparison, "key", path, recordComparisonKeys),
    oneToOneKeyCount: count("oneToOneKeyCount"),
    onlyInSourceRecordCount: count("onlyInSourceRecordCount"),
    onlyInComparedRecordCount: count("onlyInComparedRecordCount"),
    listLimit: count("listLimit"),
    onlyInSourceRecordRefs: requireArray(
      comparison,
      "onlyInSourceRecordRefs",
      path,
      decodeRecordLocator,
    ),
    onlyInComparedRecordRefs: requireArray(
      comparison,
      "onlyInComparedRecordRefs",
      path,
      decodeRecordLocator,
    ),
    onlyInSourceOutsideComparedRangeRecordCount: count(
      "onlyInSourceOutsideComparedRangeRecordCount",
    ),
    onlyInSourceInsideComparedRangeRecordCount: count(
      "onlyInSourceInsideComparedRangeRecordCount",
    ),
    onlyInSourceInsideComparedRangeRecordRefs: requireArray(
      comparison,
      "onlyInSourceInsideComparedRangeRecordRefs",
      path,
      decodeRecordLocator,
    ),
    onlyInComparedOutsideSourceRangeRecordCount: count(
      "onlyInComparedOutsideSourceRangeRecordCount",
    ),
    onlyInComparedInsideSourceRangeRecordCount: count(
      "onlyInComparedInsideSourceRangeRecordCount",
    ),
    onlyInComparedInsideSourceRangeRecordRefs: requireArray(
      comparison,
      "onlyInComparedInsideSourceRangeRecordRefs",
      path,
      decodeRecordLocator,
    ),
    undeterminedKeyCount: count("undeterminedKeyCount"),
    undeterminedSourceRecordCount: count("undeterminedSourceRecordCount"),
    undeterminedComparedRecordCount: count("undeterminedComparedRecordCount"),
    unequalKeyCount: count("unequalKeyCount"),
    sourceSurplusRecordCount: count("sourceSurplusRecordCount"),
    comparedSurplusRecordCount: count("comparedSurplusRecordCount"),
    unequalKeys: requireArray(
      comparison,
      "unequalKeys",
      path,
      decodeUnequalKey,
    ),
    sourceUnkeyedRecordCount: count("sourceUnkeyedRecordCount"),
    comparedUnkeyedRecordCount: count("comparedUnkeyedRecordCount"),
  };
};

/** `GET /api/v0/record-numbers` の応答を検証する。 */
export const decodeRecordNumbersResponse: Decoder<RecordNumbersResponse> = (
  input,
  path,
) => {
  const response = readObject(input, path);
  return {
    recordNumbers: requireMember(
      response,
      "recordNumbers",
      path,
      decodeRecordNumbers,
    ),
    comparison: optionalMember(response, "comparison", path, decodeComparison),
  };
};
