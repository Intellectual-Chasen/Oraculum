import {
  decodeEventKindPair,
  decodeObservationKind,
  decodeRecordField,
  decodeRecordLocator,
  type EventKindPair,
  type ObservationKind,
  type RecordField,
  type RecordLocator,
} from "./common";
import {
  DecodeFailure,
  type Decoder,
  decodeString,
  optionalMember,
  readObject,
  rejectMember,
  requireArray,
  requireEnum,
  requireMember,
  requireString,
} from "./decoding";
import { decodeSourceIdentity, type SourceIdentity } from "./sources";

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

/** 段階の入力が指す細かさの判別。定義元は `backend/core/record_locator.go` の `TrailInputKind` である。 */
export const trailInputKinds = ["record", "source"] as const;
export type TrailInputKind = (typeof trailInputKinds)[number];

/** 段階の入力 1 件。1 レコードと収集元 1 件のどちらを指すかを `kind` が表す。 */
export type TrailInputRef =
  | { kind: "record"; record: RecordLocator }
  | { kind: "source"; source: SourceIdentity };

/** `TrailInputRef` を検証する。`record` と `source` を同時に持つ組を読まない。 */
export const decodeTrailInputRef: Decoder<TrailInputRef> = (input, path) => {
  const source = readObject(input, path);
  const kind = requireEnum(source, "kind", path, trailInputKinds);
  switch (kind) {
    case "record":
      rejectMember(
        source,
        "source",
        path,
        "expected no source while kind is record",
      );
      return {
        kind,
        record: requireMember(source, "record", path, decodeRecordLocator),
      };
    case "source":
      rejectMember(
        source,
        "record",
        path,
        "expected no record while kind is source",
      );
      return {
        kind,
        source: requireMember(source, "source", path, decodeSourceIdentity),
      };
    default: {
      const exhaustive: never = kind;
      throw new DecodeFailure(
        `${path}.kind`,
        `unknown trail input kind: ${exhaustive}`,
      );
    }
  }
};

/** 到達した経路の 1 段階。定義元は `backend/core/record_locator.go` の `TrailStep` である。 */
export type TrailStep = {
  stepKey: string;
  /** 段階の入力のすべてが原資料の外にあるとき出ない。出るときの要素数は 1 以上である。 */
  inputRefs?: TrailInputRef[];
  /** 段階で用いた識別子と情報。要素数は 1 以上である。 */
  usedIdentifiers: string[];
  output: string;
};

/** `TrailStep` を検証する。 */
export const decodeTrailStep: Decoder<TrailStep> = (input, path) => {
  const source = readObject(input, path);
  return {
    stepKey: requireString(source, "stepKey", path),
    inputRefs:
      "inputRefs" in source
        ? requireNonEmptyArray(source, "inputRefs", path, decodeTrailInputRef)
        : undefined,
    usedIdentifiers: requireNonEmptyArray(
      source,
      "usedIdentifiers",
      path,
      decodeString,
    ),
    output: requireString(source, "output", path),
  };
};

// 既知の制限: 段階の順序を stepKey の code unit の昇順で確かめる, stepKey は
// backend/core/record_locator.go の TrailStep.StepKey が任意の文字列として持ち、
// いま応答に出る値は英字 1 文字と 1 桁の番号であって桁上がりの並びを測る対象が無い,
// 番号が 2 桁になる stepKey を返す操作を足すときに見直す。
function requireAscendingStepKeys(
  steps: TrailStep[],
  path: string,
): TrailStep[] {
  for (let index = 1; index < steps.length; index += 1) {
    const previous = steps[index - 1]?.stepKey ?? "";
    const current = steps[index]?.stepKey ?? "";
    if (previous >= current) {
      throw new DecodeFailure(
        `${path}[${index}].stepKey`,
        `expected ${current} to come after ${previous}`,
      );
    }
  }
  return steps;
}

/** 起点のレコードから元レコードまでの各段階。 */
export type DerivationTrail = {
  originRef: RecordLocator;
  /** 要素の並び順を段階の順序として持つ。並び順は `stepKey` の昇順と一致する。 */
  steps: TrailStep[];
  /** 出ない場合は最後の段階まで到達した。 */
  stoppedAt?: TrailStep;
};

/** `DerivationTrail` を検証する。`steps` の並び順が `stepKey` の昇順と食い違う応答を読まない。 */
export const decodeDerivationTrail: Decoder<DerivationTrail> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    originRef: requireMember(source, "originRef", path, decodeRecordLocator),
    steps: requireAscendingStepKeys(
      requireNonEmptyArray(source, "steps", path, decodeTrailStep),
      `${path}.steps`,
    ),
    stoppedAt: optionalMember(source, "stoppedAt", path, decodeTrailStep),
  };
};

/** 操作 3 と操作 7 (`GET /api/v0/records`) の応答。 */
export type RecordResponse = {
  recordRef: RecordLocator;
  /** レコード全体の原文。1 レコードの byte 列をそのまま持つ。 */
  rawText: string;
  sourceIdentity: SourceIdentity;
  /** 項目ごとの値。要素数は、返すレコードの入力形式が定める `name` の集合の要素数である。 */
  fields: RecordField[];
  observationKind: ObservationKind;
  /** レコードの事象の分類と動作の組。出ない場合は、そのレコードは分類を持たない。 */
  eventKind?: EventKindPair;
  /** 起点の項目を要求が持たないとき出ない。 */
  derivationTrail?: DerivationTrail;
};

/** 操作 3 と操作 7 の応答を検証する。 */
export const decodeRecordResponse: Decoder<RecordResponse> = (input, path) => {
  const source = readObject(input, path);
  return {
    recordRef: requireMember(source, "recordRef", path, decodeRecordLocator),
    rawText: requireString(source, "rawText", path),
    sourceIdentity: requireMember(
      source,
      "sourceIdentity",
      path,
      decodeSourceIdentity,
    ),
    fields: requireNonEmptyArray(source, "fields", path, decodeRecordField),
    observationKind: requireMember(
      source,
      "observationKind",
      path,
      decodeObservationKind,
    ),
    eventKind: optionalMember(source, "eventKind", path, decodeEventKindPair),
    derivationTrail: optionalMember(
      source,
      "derivationTrail",
      path,
      decodeDerivationTrail,
    ),
  };
};
