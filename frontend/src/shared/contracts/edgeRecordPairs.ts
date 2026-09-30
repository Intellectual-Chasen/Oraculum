/**
 * 観測の層が候補の関係を作ったレコードの組 (操作 11 の `recordPairs`) の型と decoder。
 * 定義元は `backend/core/edge_record_pair.go` である。
 */
import { decodeRecordField, type RecordField } from "./common";
import {
  DecodeFailure,
  type Decoder,
  optionalBoolean,
  optionalCount,
  optionalMember,
  readObject,
  requireArray,
  requireCount,
  requireEnum,
} from "./decoding";
import {
  type candidateTierConditionKeys,
  decodeGraphEvidence,
  type GraphEvidence,
} from "./graph";

/** 候補の関係を 2 件のレコードの値から作った条件の種別。 */
export const edgePairConditionKeys = [
  "process_pid",
  "terminal",
  "time_order",
  "time_proximity",
  "time_overlap",
  "nearest_identity_record",
  "logon_id",
  "linked_logon_id",
  "logon_guid",
  "task_name",
  "account",
  "source_endpoint",
  "destination_endpoint",
  "destination_terminal",
  "source_unassigned",
  "source_outside_assignment_range",
  "requesting_session",
  "source_terminal",
  "session_start_logon",
  "session_start_first_operation",
  "session_start_logoff_record",
  "session_end_logoff",
  "session_end_system_start",
  "session_end_time_limit",
  "session_end_last_operation",
  "session_account_match",
  "session_account_different",
  "session_logon_interactive",
  "session_logon_network",
  "session_logon_other",
] as const;

export type EdgePairConditionKey = (typeof edgePairConditionKeys)[number];

/** 型の引数が組の条件の種別の一部であることを要求する。 */
type EdgePairConditionSubset<T extends readonly EdgePairConditionKey[]> = T;

/**
 * 候補の区分の条件 (`graph.ts` の `candidateTierConditionKeys`)。組の条件の種別に無い値を足すと、
 * 型の検査が失敗する。`graph.ts` はこのファイルを読まないため、確かめる側をこのファイルに置く。
 */
export type CandidateTierConditionKeys = EdgePairConditionSubset<
  typeof candidateTierConditionKeys
>;

/** 時刻を比べる条件。値を持たず、組の両側のレコードの時刻を比べる。 */
export const timeEdgePairConditions: ReadonlySet<EdgePairConditionKey> =
  new Set([
    "time_order",
    "time_proximity",
    "time_overlap",
    "nearest_identity_record",
  ]);

/** 組が満たした条件 1 つと、条件に用いた両側の欄。 */
export type EdgePairCondition = {
  conditionKey: EdgePairConditionKey;
  /** 起点の側のレコードの欄。欄を読めないレコードと時刻の条件では要素数 0 である。 */
  leftValue: RecordField[];
  /** 終点の側のレコードの欄。 */
  rightValue: RecordField[];
  /** 時刻の差の許容幅の秒数。関係の種別が幅を定める時刻の条件でだけ出る。 */
  windowSeconds?: number;
  /** 終点の側の時刻から起点の側の時刻を引いた秒数。`windowSeconds` を持つ条件でだけ出る。 */
  differenceSeconds?: number;
  /**
   * 照合が両側の時刻の秒未満を切り捨てて比べたか。真のとき `differenceSeconds` は切り捨てた
   * 2 つの時刻の差である。
   */
  differenceInWholeSeconds?: boolean;
};

/**
 * 終点のレコード 1 件に挙がった候補を、組の条件の区分で並べた件数。
 * 定義元は `backend/core/edge_record_pair.go` の `EdgeCandidateTally` である。
 */
export type EdgeCandidateTally = {
  candidateCount: number;
  /** そのうち、並びでこの組の起点より上の区分に入る候補の数。 */
  precedingCandidateCount: number;
};

/** 候補の関係を作ったレコードの組 1 つ。片側がレコードを持たない組がある。 */
export type EdgeRecordPair = {
  left?: GraphEvidence;
  right?: GraphEvidence;
  conditions: EdgePairCondition[];
  /** 候補を区分で並べる関係 (logon_chain) の組でだけ出る。 */
  candidateTally?: EdgeCandidateTally;
};

/** 省略可の有限の数を読む。 */
function optionalFiniteNumber(
  source: Record<string, unknown>,
  key: string,
  path: string,
): number | undefined {
  if (!(key in source)) {
    return undefined;
  }
  const value = source[key];
  if (typeof value !== "number" || !Number.isFinite(value)) {
    throw new DecodeFailure(`${path}.${key}`, "expected a finite number");
  }
  return value;
}

const decodeEdgePairCondition: Decoder<EdgePairCondition> = (input, path) => {
  const source = readObject(input, path);
  const windowSeconds = optionalCount(source, "windowSeconds", path);
  const differenceSeconds = optionalFiniteNumber(
    source,
    "differenceSeconds",
    path,
  );
  if (differenceSeconds !== undefined && windowSeconds === undefined) {
    throw new DecodeFailure(
      `${path}.differenceSeconds`,
      "expected the difference only with the window",
    );
  }
  return {
    conditionKey: requireEnum(
      source,
      "conditionKey",
      path,
      edgePairConditionKeys,
    ),
    leftValue: requireArray(source, "leftValue", path, decodeRecordField),
    rightValue: requireArray(source, "rightValue", path, decodeRecordField),
    windowSeconds,
    differenceSeconds,
    differenceInWholeSeconds: optionalBoolean(
      source,
      "differenceInWholeSeconds",
      path,
    ),
  };
};

export const decodeCandidateTally: Decoder<EdgeCandidateTally> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const candidateCount = requireCount(source, "candidateCount", path);
  const precedingCandidateCount = requireCount(
    source,
    "precedingCandidateCount",
    path,
  );
  if (precedingCandidateCount >= candidateCount) {
    throw new DecodeFailure(
      `${path}.precedingCandidateCount`,
      "expected fewer preceding candidates than the candidates",
    );
  }
  return { candidateCount, precedingCandidateCount };
};

/** `EdgeRecordPair` を検証する。両側のどちらかと、条件 1 つ以上を持つ組だけを通す。 */
export const decodeEdgeRecordPair: Decoder<EdgeRecordPair> = (input, path) => {
  const source = readObject(input, path);
  const left = optionalMember(source, "left", path, decodeGraphEvidence);
  const right = optionalMember(source, "right", path, decodeGraphEvidence);
  if (left === undefined && right === undefined) {
    throw new DecodeFailure(`${path}.left`, "expected a record on either side");
  }
  const conditions = requireArray(
    source,
    "conditions",
    path,
    decodeEdgePairCondition,
  );
  if (conditions.length === 0) {
    throw new DecodeFailure(`${path}.conditions`, "expected a condition");
  }
  const candidateTally = optionalMember(
    source,
    "candidateTally",
    path,
    decodeCandidateTally,
  );
  return { left, right, conditions, candidateTally };
};
