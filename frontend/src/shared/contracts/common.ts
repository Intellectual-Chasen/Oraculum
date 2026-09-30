import {
  DecodeFailure,
  type Decoder,
  optionalCount,
  optionalEnum,
  optionalMember,
  optionalString,
  readObject,
  rejectMember,
  requireArray,
  requireEnum,
  requireMember,
  requireString,
} from "./decoding";

/** 時刻の精度。定義元は `backend/core/timestamp.go` の `Precision` である。 */
export const timestampPrecisions = [
  "year",
  "month",
  "day",
  "hour",
  "minute",
  "second",
  "millisecond",
  "microsecond",
] as const;
export type TimestampPrecision = (typeof timestampPrecisions)[number];

/** 正規化値の書式。定義元は `backend/core/timestamp.go` の `NormalizedForm` である。 */
export const normalizedForms = [
  "rfc3339_absolute",
  "local_without_offset",
  "partial_date_time",
] as const;
export type NormalizedForm = (typeof normalizedForms)[number];

/** UTC からのずれの状態。定義元は `backend/core/timestamp.go` の `OffsetState` である。 */
export const offsetStates = [
  "in_value",
  "item_absent",
  "undetermined",
  "epoch",
  "format_defined",
] as const;
export type OffsetState = (typeof offsetStates)[number];

/**
 * UTC からのずれの状態から時点が 1 つに定まるかを返す。backend の
 * `OffsetState.carriesInstant` と同じ条件である。
 */
export function offsetCarriesInstant(offsetState: OffsetState): boolean {
  return (
    offsetState === "in_value" ||
    offsetState === "epoch" ||
    offsetState === "format_defined"
  );
}

/** 時刻を刻んだ時計。定義元は `backend/core/timestamp.go` の `Clock` である。 */
export const timestampClocks = [
  "terminal_local",
  "observer_local",
  "file_property",
  "undetermined",
] as const;
export type TimestampClock = (typeof timestampClocks)[number];

/** 何の時刻か。定義元は `backend/core/timestamp.go` の `Meaning` である。 */
export const timestampMeanings = [
  "event",
  "operation_start",
  "record_output",
  "property",
] as const;
export type TimestampMeaning = (typeof timestampMeanings)[number];

/** 値の状態。定義元は `backend/core/value_state.go` の `ValueState` である。 */
export const valueStates = [
  "present",
  "absent",
  "item_absent",
  "no_body",
  "out_of_definition",
  "derived",
  "derivation_undetermined",
  "truncated",
] as const;
export type ValueState = (typeof valueStates)[number];

/**
 * UTC からのずれを持たない地方時に、分析者が所見で与えたずれ。
 * 定義元は `backend/core/time_interpretation.go` の `TimestampInterpretation` である。
 */
export type TimestampInterpretation = {
  /** `+09:00` の形の UTC からのずれ。 */
  offset: string;
  /**
   * ずれを記録した所見の識別子。取り込みの起動で収集元に指定したずれは所見を持たず、
   * この項目を持たない。
   */
  assertionId?: string;
};

const decodeTimestampInterpretation: Decoder<TimestampInterpretation> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    offset: requireString(source, "offset", path),
    assertionId: optionalString(source, "assertionId", path),
  };
};

/** 原資料のレコードから読んだ時刻。精度を値と別の項目で持つ。 */
export type Timestamp = {
  rawText?: string;
  normalized?: string;
  normalizedForm?: NormalizedForm;
  precision: TimestampPrecision;
  offsetState: OffsetState;
  offsetText?: string;
  clock: TimestampClock;
  meaning: TimestampMeaning;
  valueState: ValueState;
  /** 分析者が与えたずれ。原資料の文字列と正規化値は書き換えない。 */
  interpretation?: TimestampInterpretation;
};

/** `Timestamp` を検証する。 */
export const decodeTimestamp: Decoder<Timestamp> = (input, path) => {
  const source = readObject(input, path);
  return {
    interpretation: optionalMember(
      source,
      "interpretation",
      path,
      decodeTimestampInterpretation,
    ),
    rawText: optionalString(source, "rawText", path),
    normalized: optionalString(source, "normalized", path),
    normalizedForm: optionalEnum(
      source,
      "normalizedForm",
      path,
      normalizedForms,
    ),
    precision: requireEnum(source, "precision", path, timestampPrecisions),
    offsetState: requireEnum(source, "offsetState", path, offsetStates),
    offsetText: optionalString(source, "offsetText", path),
    clock: requireEnum(source, "clock", path, timestampClocks),
    meaning: requireEnum(source, "meaning", path, timestampMeanings),
    valueState: requireEnum(source, "valueState", path, valueStates),
  };
};

/** 時刻の両端。定義元は `backend/core/source_identity.go` の `TimeRange` である。 */
export type TimeRange = {
  from: Timestamp;
  to: Timestamp;
};

/** `TimeRange` を検証する。 */
export const decodeTimeRange: Decoder<TimeRange> = (input, path) => {
  const source = readObject(input, path);
  return {
    from: requireMember(source, "from", path, decodeTimestamp),
    to: requireMember(source, "to", path, decodeTimestamp),
  };
};

/**
 * 数えた件数が、応答へ入れた要素数と一致することを確かめる。
 *
 * **全件を返す操作の不変条件である。** 件数が要素数と食い違う応答は、数えた要素の一部を
 * 除いたまま件数だけを申告しており、分析者が「一覧に出ていない要素は無い」と読めない。
 * 定義元は `backend/core/truncation.go` の `validateCountedSet` である。
 */
export function requireCountEqualsElements(
  count: number,
  returned: number,
  path: string,
): number {
  if (count !== returned) {
    throw new DecodeFailure(path, `expected ${returned} elements`);
  }
  return count;
}

/** 範囲の表し方。定義元は `backend/core/record_locator.go` の `RangeKind` である。 */
export const rangeKinds = ["positioned", "whole_source"] as const;
export type RangeKind = (typeof rangeKinds)[number];

/** 位置の指し方。定義元は `backend/core/record_locator.go` の `PositionKind` である。 */
export const positionKinds = [
  "line_number",
  "sequence_number",
  "byte_range",
] as const;
export type PositionKind = (typeof positionKinds)[number];

/** 収集元の中のレコードの範囲。 */
export type RecordRange = {
  sourceId: string;
  sourceContentSha256: string;
  rangeKind: RangeKind;
  positionKind?: PositionKind;
  fromPosition?: number;
  toPosition?: number;
};

/** `RecordRange` を検証する。 */
export const decodeRecordRange: Decoder<RecordRange> = (input, path) => {
  const source = readObject(input, path);
  return {
    sourceId: requireString(source, "sourceId", path),
    sourceContentSha256: requireString(source, "sourceContentSha256", path),
    rangeKind: requireEnum(source, "rangeKind", path, rangeKinds),
    positionKind: optionalEnum(source, "positionKind", path, positionKinds),
    fromPosition: optionalCount(source, "fromPosition", path),
    toPosition: optionalCount(source, "toPosition", path),
  };
};

/** 両側の母数の件数と、その範囲。定義元は `backend/core/truncation.go` の `BothSideCounts` である。 */
export type BothSideCounts = {
  leftCount: number;
  /** `leftCount` が数えた範囲。 */
  leftScope: string;
  rightCount: number;
  /** `rightCount` が数えた範囲。 */
  rightScope: string;
};

/** レコードの 1 項目の原資料の文字列と正規化値。定義元は `backend/core/value_state.go` の `RawAndNormalized` である。 */
export type RawAndNormalized = {
  /** `valueState` が `item_absent` と `derived` と `derivation_undetermined` のとき出ない。 */
  rawText?: string;
  /**
   * 出ない場合、値がある項目は原資料の文字列のまま比べる。比べる値の定義元は
   * `backend/core/record_value.go` の `ComparableValue` である。
   */
  normalized?: string;
  /** `normalized` があるとき、および `valueState` が `derivation_undetermined` のとき出る。 */
  derivation?: string;
  valueState: ValueState;
};

const valueStatesWithoutRawText: readonly ValueState[] = [
  "item_absent",
  "derived",
  "derivation_undetermined",
];

/** `RawAndNormalized` を検証する。項目の出る条件は `backend/core/value_state.go` の `RawAndNormalized.Validate` が持つ。 */
export const decodeRawAndNormalized: Decoder<RawAndNormalized> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const valueState = requireEnum(source, "valueState", path, valueStates);

  let rawText: string | undefined;
  if (valueStatesWithoutRawText.includes(valueState)) {
    rejectMember(
      source,
      "rawText",
      path,
      `expected no rawText while valueState is ${valueState}`,
    );
  } else {
    rawText = requireString(source, "rawText", path);
  }

  let normalized: string | undefined;
  if (valueState === "derived") {
    normalized = requireString(source, "normalized", path);
  } else if (
    valueState === "item_absent" ||
    valueState === "derivation_undetermined"
  ) {
    rejectMember(
      source,
      "normalized",
      path,
      `expected no normalized while valueState is ${valueState}`,
    );
  } else {
    normalized = optionalString(source, "normalized", path);
  }

  let derivation: string | undefined;
  if (valueState === "item_absent") {
    rejectMember(
      source,
      "derivation",
      path,
      "expected no derivation while valueState is item_absent",
    );
  } else if (
    valueState === "derivation_undetermined" ||
    normalized !== undefined
  ) {
    derivation = requireString(source, "derivation", path);
  } else {
    derivation = optionalString(source, "derivation", path);
  }

  return { rawText, normalized, derivation, valueState };
};

/** 値の形の判別。定義元は `backend/core/value_state.go` の `RecordFieldKind` である。 */
export const recordFieldKinds = ["text", "timestamp"] as const;
export type RecordFieldKind = (typeof recordFieldKinds)[number];

/**
 * 入力形式をまたいで同じ意味を表す語彙の項目。値の一覧は `backend/core/semantic_key.go`
 * が持つ。値が語彙の中にあることは backend が確かめる。
 */
export type SemanticKey = string;

/** レコードの 1 項目。`kind` が持っている値の形を表す。 */
export type RecordField =
  | {
      name: string;
      /** 出ない項目は、この入力形式に固有である。 */
      semantic?: SemanticKey;
      kind: "text";
      text: RawAndNormalized;
    }
  | {
      name: string;
      semantic?: SemanticKey;
      kind: "timestamp";
      timestamp: Timestamp;
    };

/** `RecordField` を検証する。`text` と `timestamp` を同時に持つ組を読まない。 */
export const decodeRecordField: Decoder<RecordField> = (input, path) => {
  const source = readObject(input, path);
  const name = requireString(source, "name", path);
  const semantic = optionalString(source, "semantic", path);
  const kind = requireEnum(source, "kind", path, recordFieldKinds);
  switch (kind) {
    case "text":
      rejectMember(
        source,
        "timestamp",
        path,
        "expected no timestamp while kind is text",
      );
      return {
        name,
        ...(semantic === undefined ? {} : { semantic }),
        kind,
        text: requireMember(source, "text", path, decodeRawAndNormalized),
      };
    case "timestamp":
      rejectMember(
        source,
        "text",
        path,
        "expected no text while kind is timestamp",
      );
      return {
        name,
        ...(semantic === undefined ? {} : { semantic }),
        kind,
        timestamp: requireMember(source, "timestamp", path, decodeTimestamp),
      };
    default: {
      const exhaustive: never = kind;
      throw new DecodeFailure(`${path}.kind`, `unknown kind: ${exhaustive}`);
    }
  }
};

/** 観測の種別の意味の状態。定義元は `backend/core/observation_kind.go` の `ObservationKindStatus` である。 */
export const observationStatuses = [
  "determined",
  "inferred",
  "undetermined",
] as const;
export type ObservationStatus = (typeof observationStatuses)[number];

/** 1 レコードの観測の種別と、その意味の状態。 */
export type ObservationKind = {
  /** 入力形式が観測の種別の欄を持たないとき要素数は 0 である。 */
  raw: RecordField[];
  /** `raw` の要素数が 0 のとき、および要素の文字列に `present` 以外があるとき出ない。 */
  status?: ObservationStatus;
  /** 収集元のレコードから推定した意味。`status` が `inferred` のときだけ出る。 */
  meaning?: string;
};

/**
 * 事象の分類と動作の組。`action` が空の組は、動作の欄を持たないレコードの組である。定義元は
 * `backend/core/graph_response.go` の `EventKindPair` である。
 */
export type EventKindPair = { category: string; action: string };

/** `EventKindPair` を検証する。 */
export const decodeEventKindPair: Decoder<EventKindPair> = (input, path) => {
  const source = readObject(input, path);
  const category = requireString(source, "category", path);
  if (category === "") {
    throw new DecodeFailure(`${path}.category`, "expected a non-empty string");
  }
  return { category, action: requireString(source, "action", path) };
};

/** `ObservationKind` を検証する。 */
export const decodeObservationKind: Decoder<ObservationKind> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const raw = requireArray(source, "raw", path, decodeRecordField);
  const hasStatus =
    raw.length > 0 &&
    raw.every(
      (field) => field.kind === "text" && field.text.valueState === "present",
    );
  if (!hasStatus) {
    rejectMember(
      source,
      "status",
      path,
      "expected no status while the raw fields do not carry present values",
    );
    // **状態が出ない応答の `meaning` も拒む。** `meaning` が出るのは `status` が
    // `inferred` のときだけであり、`status` が出ない応答はその条件を満たさない。
    // エラーを出さずに捨てると、backend が拒む組み合わせを画面が受け取ったことに気付けない。
    rejectMember(
      source,
      "meaning",
      path,
      "expected no meaning while the raw fields do not carry present values",
    );
    return { raw };
  }
  const status = requireEnum(source, "status", path, observationStatuses);
  if (status !== "inferred") {
    rejectMember(
      source,
      "meaning",
      path,
      "expected no meaning while the status is not inferred",
    );
    return { raw, status };
  }
  // **`inferred` の種別は `meaning` を必ず持つ。** 必須条件の定義元は
  // `backend/core/observation_kind.go` の `validateMeaning` である。意味を持たない
  // `inferred` は、推定であることだけを伝えて推定の内容を伝えない値になる。
  return {
    raw,
    status,
    meaning: requireString(source, "meaning", path),
  };
};

/** 原資料のレコードへ到達する位置。定義元は `backend/core/record_locator.go` の `RecordLocator` である。 */
export type RecordLocator = {
  sourceId: string;
  sourceContentSha256: string;
  /** 表示に使う収集元の file 名。 */
  sourceFileName: string;
  positionKind: PositionKind;
  /** 入力形式が通番を持たないとき出ない。 */
  sequenceNumber?: number;
  /**
   * 出ない場合は行番号を数えていない。
   * `positionKind` が `byte_range` のときはレコードの先頭の行を指す。
   */
  lineNumber?: number;
  /** 収集元の先頭からのレコードの byte 位置。 */
  byteOffset?: number;
  /** レコードが占める byte 数。 */
  byteLength?: number;
  /** レコードが占める行数。1 レコードが複数行に分かれる形式で出る。 */
  lineCount?: number;
  /** レコードの原文を返す操作への参照。 */
  recordRawTextRef: string;
};

/**
 * `RecordLocator` を検証する。
 * `positionKind` に対応する位置の値が必ず出ることは、
 * `backend/core/record_locator.go` の `RecordLocator.Validate` が定める。
 */
export const decodeRecordLocator: Decoder<RecordLocator> = (input, path) => {
  const source = readObject(input, path);
  const positionKind = requireEnum(source, "positionKind", path, positionKinds);
  const sequenceNumber = optionalCount(source, "sequenceNumber", path);
  const lineNumber = optionalCount(source, "lineNumber", path);
  const byteOffset = optionalCount(source, "byteOffset", path);
  const byteLength = optionalCount(source, "byteLength", path);
  const lineCount = optionalCount(source, "lineCount", path);
  const positionOfKind = {
    sequence_number: { key: "sequenceNumber", value: sequenceNumber },
    line_number: { key: "lineNumber", value: lineNumber },
    byte_range: { key: "byteOffset", value: byteOffset },
  } as const;
  const { key: positionKey, value: position } = positionOfKind[positionKind];
  if (position === undefined) {
    throw new DecodeFailure(
      `${path}.${positionKey}`,
      `expected a position while positionKind is ${positionKind}`,
    );
  }
  return {
    sourceId: requireString(source, "sourceId", path),
    sourceContentSha256: requireString(source, "sourceContentSha256", path),
    sourceFileName: requireString(source, "sourceFileName", path),
    positionKind,
    sequenceNumber,
    lineNumber,
    byteOffset,
    byteLength,
    lineCount,
    recordRawTextRef: requireString(source, "recordRawTextRef", path),
  };
};

/** 要求が与えた時刻。定義元は `backend/core/timestamp.go` の `RequestedTime` である。 */
export type RequestedTime = {
  /** 要求が与えた文字列。原資料の文字列と別の値である。 */
  requestText: string;
  precision: TimestampPrecision;
  offsetState: OffsetState;
  /** 出ない場合は関連付けに用いる値を導く規則が未確定である。 */
  normalized?: string;
  normalizedForm?: NormalizedForm;
  derivation?: string;
};

/** `RequestedTime` を検証する。 */
export const decodeRequestedTime: Decoder<RequestedTime> = (input, path) => {
  const source = readObject(input, path);
  const normalized = optionalString(source, "normalized", path);
  return {
    requestText: requireString(source, "requestText", path),
    precision: requireEnum(source, "precision", path, timestampPrecisions),
    offsetState: requireEnum(source, "offsetState", path, offsetStates),
    normalized,
    normalizedForm:
      normalized === undefined
        ? optionalEnum(source, "normalizedForm", path, normalizedForms)
        : requireEnum(source, "normalizedForm", path, normalizedForms),
    derivation:
      normalized === undefined
        ? optionalString(source, "derivation", path)
        : requireString(source, "derivation", path),
  };
};
