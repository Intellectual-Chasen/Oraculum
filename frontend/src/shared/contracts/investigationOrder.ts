import { decodeTimeRange, type TimeRange } from "./common";
import {
  DecodeFailure,
  type Decoder,
  optionalMember,
  readObject,
  requireArray,
  requireCount,
  requireEnum,
  requireMember,
  requireString,
} from "./decoding";
import {
  decodeGraphNode,
  type GraphNode,
  type NodeKind,
  nodeKinds,
} from "./graph";

/**
 * 調べる順序の目安の手法。定義元は `backend/pipeline/investigation_order.go` の
 * `orderMethods` である。
 */
export const investigationOrderMethods = [
  "degree",
  "sigma",
  "record_count_descending",
  "record_count_ascending",
] as const;
export type InvestigationOrderMethod =
  (typeof investigationOrderMethods)[number];

/**
 * Sigma の手法が一致を数えるルールのレベルの下限に渡せる文字列。低い順に並ぶ。
 * 定義元は `backend/pipeline/investigation_order_sigma.go` の `sigmaLevelRanks` である。
 */
export const sigmaMinLevels = [
  "informational",
  "low",
  "medium",
  "high",
  "critical",
] as const;
export type SigmaMinLevel = (typeof sigmaMinLevels)[number];

/** Sigma の手法の値で、最高レベルの順位に掛ける倍率。定義元は `sigmaLevelRankMultiplier` である。 */
export const sigmaLevelRankMultiplier = 1_000_000_000;

/** 手法の係数 1 つ。 */
export type InvestigationOrderParameter = { name: string; value: string };

/** 計算に使った入力の大きさ。定義元は `backend/api/investigation_order.go` である。 */
export type InvestigationOrderInputs = {
  objectCount: number;
  pairCount: number;
  recordCount: number;
  timedRecordCount: number;
  /** 時点を持つレコードの時刻の両端。時点を持つレコードが無いときは無い。 */
  timeRange?: TimeRange;
};

/** 並びの 1 行。 */
export type InvestigationOrderEntry = {
  node: GraphNode;
  /** 手法の値。手法が値を決められない対象では `null` である。 */
  value: number | null;
  rank: number;
  tieCount: number;
};

/** 1 つの種別の対象の並び。 */
export type InvestigationOrderKind = {
  kind: NodeKind;
  entries: InvestigationOrderEntry[];
};

/**
 * 調べる順序の目安の応答。定義元は `backend/api/investigation_order.go` の
 * `investigationOrderResponse` である。
 *
 * 目安は関係の状態と別の属性であり、関係の成立や対象の性質を判定しない。
 */
export type InvestigationOrderResponse = {
  method: string;
  parameters: InvestigationOrderParameter[];
  inputs: InvestigationOrderInputs;
  kinds: InvestigationOrderKind[];
};

/** 有限の数か `null` を読む。 */
function requireNullableNumber(
  source: Record<string, unknown>,
  key: string,
  path: string,
): number | null {
  const value = source[key];
  if (value === null) return null;
  if (typeof value !== "number" || !Number.isFinite(value)) {
    throw new DecodeFailure(
      `${path}.${key}`,
      "expected a finite number or null",
    );
  }
  return value;
}

const decodeParameter: Decoder<InvestigationOrderParameter> = (input, path) => {
  const source = readObject(input, path);
  return {
    name: requireString(source, "name", path),
    value: requireString(source, "value", path),
  };
};

const decodeInputs: Decoder<InvestigationOrderInputs> = (input, path) => {
  const source = readObject(input, path);
  return {
    objectCount: requireCount(source, "objectCount", path),
    pairCount: requireCount(source, "pairCount", path),
    recordCount: requireCount(source, "recordCount", path),
    timedRecordCount: requireCount(source, "timedRecordCount", path),
    timeRange: optionalMember(source, "timeRange", path, decodeTimeRange),
  };
};

const decodeEntry: Decoder<InvestigationOrderEntry> = (input, path) => {
  const source = readObject(input, path);
  const rank = requireCount(source, "rank", path);
  const tieCount = requireCount(source, "tieCount", path);
  if (rank < 1 || tieCount < 1) {
    throw new DecodeFailure(
      path,
      "expected a rank and a tie count of 1 or more",
    );
  }
  return {
    node: requireMember(source, "node", path, decodeGraphNode),
    value: requireNullableNumber(source, "value", path),
    rank,
    tieCount,
  };
};

const decodeKind: Decoder<InvestigationOrderKind> = (input, path) => {
  const source = readObject(input, path);
  return {
    kind: requireEnum(source, "kind", path, nodeKinds),
    entries: requireArray(source, "entries", path, decodeEntry),
  };
};

export const decodeInvestigationOrderResponse: Decoder<
  InvestigationOrderResponse
> = (input, path) => {
  const source = readObject(input, path);
  return {
    method: requireString(source, "method", path),
    parameters: requireArray(source, "parameters", path, decodeParameter),
    inputs: requireMember(source, "inputs", path, decodeInputs),
    kinds: requireArray(source, "kinds", path, decodeKind),
  };
};
