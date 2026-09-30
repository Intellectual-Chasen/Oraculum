import {
  decodeRecordField,
  decodeRequestedTime,
  decodeTimeRange,
  decodeTimestamp,
  type RecordField,
  type RequestedTime,
  type TimeRange,
  type Timestamp,
} from "./common";
import {
  DecodeFailure,
  type Decoder,
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

function requireNonEmptyArray<T>(
  source: Record<string, unknown>,
  key: string,
  path: string,
  decode: Decoder<T>,
): T[] {
  const elements = requireArray(source, key, path, decode);
  if (elements.length === 0) {
    throw new DecodeFailure(`${path}.${key}`, "expected 1 or more elements");
  }
  return elements;
}

/** 段階の種別。`backend/core/candidate.go` の `CandidateStage`の 2 値。 */
export const stageKeys = ["clock_independent", "second_time_matched"] as const;
export type StageKey = (typeof stageKeys)[number];

/** 条件の種別。定義元は `backend/core/match_condition.go` の `ConditionKey` である。 */
export const conditionKeys = [
  "terminal_ip_assignment",
  "terminal_identity_matches",
  "parent_process_id_matches",
  "destination_ip",
  "destination_port",
  "destination_authority",
  "second_of_time",
  "sub_second_of_time",
  "client_port",
  "process",
  "user",
] as const;
export type ConditionKey = (typeof conditionKeys)[number];

/** 条件の使い方。定義元は `backend/core/match_condition.go` の `ConditionUse` である。 */
export const conditionUses = [
  "used",
  "not_used",
  "no_comparable_counterpart",
  "item_absent_on_counterpart",
] as const;
export type ConditionUse = (typeof conditionUses)[number];

/** 候補の側の時刻が候補 1 件ごとに変わる 2 条件。段階に 1 組の `rightValue` を置けない。 */
const perCandidateTimeConditionKeys: readonly ConditionKey[] = [
  "second_of_time",
  "sub_second_of_time",
];

/** 段階が関連付けに用いた条件、または用いなかった条件の 1 件。 */
export type MatchCondition = {
  conditionKey: ConditionKey;
  use: ConditionUse;
  /** `use` が `used` のとき必ず出る。起点の側の値は候補の有無に依らず定まる。 */
  leftValue?: RecordField[];
  /** 時刻の 2 条件、および候補が 0 件の段階と応答では出ない。要素数は `leftValue` と等しい。 */
  rightValue?: RecordField[];
  /** `conditionKey` が `terminal_ip_assignment` のとき出る。 */
  assignmentValidRange?: TimeRange;
  /** 起点のレコードが `assignmentValidRange` の外にあるか。段階に 1 件の判定である。 */
  outsideAssignmentRange?: boolean;
};

/**
 * `MatchCondition` を検証する decoder を作る。
 * `rightValue` の必須の条件が同じ段階または同じ応答の `memberCount` で決まるため、
 * 候補が 1 件以上あるかを引数で受け取る。
 */
export function matchConditionDecoder(
  hasMember: boolean,
): Decoder<MatchCondition> {
  return (input, path) => {
    const source = readObject(input, path);
    const conditionKey = requireEnum(
      source,
      "conditionKey",
      path,
      conditionKeys,
    );
    const use = requireEnum(source, "use", path, conditionUses);

    const carriesLeftValue = use === "used" || "leftValue" in source;
    const leftValue = carriesLeftValue
      ? requireNonEmptyArray(source, "leftValue", path, decodeRecordField)
      : undefined;

    // **候補の側の値を、定義元より狭い条件で拒まない。** 拒む組は、時刻の 2 条件と、
    // 候補が 1 件も無い段階だけである (`backend/core/match_condition.go` の
    // `MatchCondition.Validate` と `ValidateMatchConditions`)。必須になるのは
    // use が used の条件である。
    const mayCarryRightValue =
      hasMember && !perCandidateTimeConditionKeys.includes(conditionKey);
    let rightValue: RecordField[] | undefined;
    if (mayCarryRightValue && (use === "used" || "rightValue" in source)) {
      rightValue = requireNonEmptyArray(
        source,
        "rightValue",
        path,
        decodeRecordField,
      );
      if (leftValue !== undefined && rightValue.length !== leftValue.length) {
        throw new DecodeFailure(
          `${path}.rightValue`,
          `expected ${leftValue.length} elements to match leftValue`,
        );
      }
    } else {
      rejectMember(
        source,
        "rightValue",
        path,
        `expected no rightValue for ${conditionKey} while use is ${use}`,
      );
    }

    const carriesAssignment = conditionKey === "terminal_ip_assignment";
    if (!carriesAssignment) {
      rejectMember(
        source,
        "assignmentValidRange",
        path,
        `expected no assignmentValidRange for ${conditionKey}`,
      );
      rejectMember(
        source,
        "outsideAssignmentRange",
        path,
        `expected no outsideAssignmentRange for ${conditionKey}`,
      );
    }

    return {
      conditionKey,
      use,
      leftValue,
      rightValue,
      assignmentValidRange: carriesAssignment
        ? requireMember(source, "assignmentValidRange", path, decodeTimeRange)
        : undefined,
      outsideAssignmentRange: carriesAssignment
        ? requireBoolean(source, "outsideAssignmentRange", path)
        : undefined,
    };
  };
}

/** 前提の種別。定義元は `backend/core/assumption.go` の `AssumptionKey` である。 */
export const assumptionKeys = [
  "clock_offset_below_one_second",
  "ip_assignment_holds_in_observation_gap",
  "counterpart_connected_to_proxy",
] as const;
export type AssumptionKey = (typeof assumptionKeys)[number];

/** 前提の根拠区分。値と意味の定義元は `backend/core/assumption.go` の `EvidenceClass` である。 */
export const evidenceClasses = [
  "official",
  "measured",
  "inferred",
  "unconfirmed",
] as const;
export type EvidenceClass = (typeof evidenceClasses)[number];

/** 段階または比較が依拠する前提 1 件。 */
export type MatchAssumption = {
  assumptionKey: AssumptionKey;
  evidenceClass: EvidenceClass;
  statement: string;
  /** `evidenceClass` が `unconfirmed` のとき出る。 */
  unresolvedReason?: string;
};

/** `MatchAssumption` を検証する。 */
export const decodeMatchAssumption: Decoder<MatchAssumption> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const evidenceClass = requireEnum(
    source,
    "evidenceClass",
    path,
    evidenceClasses,
  );
  return {
    assumptionKey: requireEnum(source, "assumptionKey", path, assumptionKeys),
    evidenceClass,
    statement: requireString(source, "statement", path),
    unresolvedReason:
      evidenceClass === "unconfirmed"
        ? requireString(source, "unresolvedReason", path)
        : optionalString(source, "unresolvedReason", path),
  };
};

/** 時刻の範囲の種別。定義元は `backend/core/time_window.go` の `WindowKind` である。 */
export const windowKinds = [
  "not_compared",
  "same_second",
  "second_range",
  "symmetric_seconds",
] as const;
export type WindowKind = (typeof windowKinds)[number];

/**
 * 時刻の範囲の判定に使う時刻の比較の単位。
 * 操作 4 の仕様の 1 値。
 * `windowKind` の 4 値と `windowRadiusSeconds` の単位が秒であるため `millisecond` を取らない。
 */
export const windowComparisonUnits = ["second"] as const;

/** 関連付けに用いた時刻の範囲。既定値を持たず、値は要求が与える。 */
export type TimeWindow = {
  windowKind: WindowKind;
  /** `windowKind` が `second_range` のとき出る。 */
  lowerBound?: RequestedTime;
  /** `windowKind` が `second_range` のとき出る。 */
  upperBound?: RequestedTime;
  /** `windowKind` が `same_second` または `symmetric_seconds` のとき出る。 */
  centerTime?: RequestedTime;
  /** `windowKind` が `symmetric_seconds` のとき出る。半幅 0 は中心の秒だけを取る。 */
  radiusSeconds?: number;
};

/** `TimeWindow` を検証する。 */
export const decodeTimeWindow: Decoder<TimeWindow> = (input, path) => {
  const source = readObject(input, path);
  const windowKind = requireEnum(source, "windowKind", path, windowKinds);
  const carriesRange = windowKind === "second_range";
  const carriesCenter =
    windowKind === "same_second" || windowKind === "symmetric_seconds";
  const carriesRadius = windowKind === "symmetric_seconds";
  // **時刻の範囲の種別が求めない項目を受け取らない。** 同じ規則を backend の
  // `TimeWindow.Validate` が持つ。受け取ると、どの項目で時刻の範囲を決めたかが読めなくなる。
  const unexpected = "the window kind does not carry this item";
  if (!carriesRange) {
    rejectMember(source, "lowerBound", path, unexpected);
    rejectMember(source, "upperBound", path, unexpected);
  }
  if (!carriesCenter) {
    rejectMember(source, "centerTime", path, unexpected);
  }
  if (!carriesRadius) {
    rejectMember(source, "radiusSeconds", path, unexpected);
  }
  return {
    windowKind,
    lowerBound: carriesRange
      ? requireMember(source, "lowerBound", path, decodeRequestedTime)
      : undefined,
    upperBound: carriesRange
      ? requireMember(source, "upperBound", path, decodeRequestedTime)
      : undefined,
    centerTime: carriesCenter
      ? requireMember(source, "centerTime", path, decodeRequestedTime)
      : undefined,
    radiusSeconds: carriesRadius
      ? requireCount(source, "radiusSeconds", path)
      : undefined,
  };
};

/** 時刻を比べた単位。`backend/core/matching.go` の `TimeComparison`の 2 値。 */
export const comparisonUnits = ["second", "not_compared"] as const;
export type ComparisonUnit = (typeof comparisonUnits)[number];

/** 候補 1 件と起点の時刻の比較。比較の結果を真偽 1 個で表さない。 */
export type TimeComparison = {
  comparisonUnit: ComparisonUnit;
  /** `comparisonUnit` が `second` のとき出る。 */
  leftTime?: Timestamp;
  /** `comparisonUnit` が `second` のとき出る。同じ候補の `eventTime` と等しい。 */
  rightTime?: Timestamp;
  /** 比較が依拠する前提。`not_compared` のとき要素数は 0 である。 */
  assumptions: MatchAssumption[];
};

/** `TimeComparison` を検証する。 */
export const decodeTimeComparison: Decoder<TimeComparison> = (input, path) => {
  const source = readObject(input, path);
  const comparisonUnit = requireEnum(
    source,
    "comparisonUnit",
    path,
    comparisonUnits,
  );
  const assumptions = requireArray(
    source,
    "assumptions",
    path,
    decodeMatchAssumption,
  );
  if (comparisonUnit === "not_compared") {
    rejectMember(
      source,
      "leftTime",
      path,
      "expected no leftTime while comparisonUnit is not_compared",
    );
    rejectMember(
      source,
      "rightTime",
      path,
      "expected no rightTime while comparisonUnit is not_compared",
    );
    if (assumptions.length > 0) {
      throw new DecodeFailure(
        `${path}.assumptions`,
        "expected no assumptions while comparisonUnit is not_compared",
      );
    }
    return { comparisonUnit, assumptions };
  }
  return {
    comparisonUnit,
    leftTime: requireMember(source, "leftTime", path, decodeTimestamp),
    rightTime: requireMember(source, "rightTime", path, decodeTimestamp),
    assumptions,
  };
};
