import {
  type PositionKind,
  positionKinds,
  type RecordLocator,
  requireCountEqualsElements,
} from "./common";
import {
  DecodeFailure,
  type Decoder,
  optionalBoolean,
  optionalCount,
  optionalMember,
  optionalString,
  readObject,
  requireArray,
  requireCount,
  requireEnum,
  requireMember,
  requireString,
} from "./decoding";

/** 所見が指す対象の種類。 */
export const assertionTargetKinds = [
  "node",
  "edge",
  "record",
  "source",
] as const;
export type AssertionTargetKind = (typeof assertionTargetKinds)[number];

/** 所見が現在も主張されているか。 */
export const assertionStates = ["active", "withdrawn"] as const;
export type AssertionState = (typeof assertionStates)[number];

/** 所見の対象が何から出たか。 */
export const assertionTargetOrigins = [
  "observation",
  "matching_candidate",
  "analyst_assertion",
  "absent",
] as const;
export type AssertionTargetOrigin = (typeof assertionTargetOrigins)[number];

/**
 * 所見が指す原資料のレコード 1 件。
 * **`sourceId` を持たない。** 取り込みをやり直しても同じレコードに一致する材料だけを持つ。
 */
export type AssertionRecordRef = {
  sourceContentSha256: string;
  positionKind: PositionKind;
  sequenceNumber?: number;
  lineNumber?: number;
  byteOffset?: number;
};

/** レコードの位置から、取り込みをまたいで同じ値になる参照を作る。 */
export function assertionRecordRefOf(
  locator: RecordLocator,
): AssertionRecordRef {
  return {
    sourceContentSha256: locator.sourceContentSha256,
    positionKind: locator.positionKind,
    sequenceNumber: locator.sequenceNumber,
    lineNumber: locator.lineNumber,
    byteOffset: locator.byteOffset,
  };
}

/** `AssertionRecordRef` を検証する。 */
export const decodeAssertionRecordRef: Decoder<AssertionRecordRef> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    sourceContentSha256: requireString(source, "sourceContentSha256", path),
    positionKind: requireEnum(source, "positionKind", path, positionKinds),
    sequenceNumber: optionalCount(source, "sequenceNumber", path),
    lineNumber: optionalCount(source, "lineNumber", path),
    byteOffset: optionalCount(source, "byteOffset", path),
  };
};

/** 所見が指す関係 1 本。関係の種別と両端のノードの識別子で指す。 */
export type AssertionEdgeRef = {
  kind: string;
  sourceNodeId: string;
  targetNodeId: string;
};

/** `AssertionEdgeRef` を検証する。 */
export const decodeAssertionEdgeRef: Decoder<AssertionEdgeRef> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    kind: requireString(source, "kind", path),
    sourceNodeId: requireString(source, "sourceNodeId", path),
    targetNodeId: requireString(source, "targetNodeId", path),
  };
};

/** 所見が指す対象。`kind` に対応する 1 つの参照だけを持つ。 */
export type AssertionTarget = {
  kind: AssertionTargetKind;
  nodeId?: string;
  edge?: AssertionEdgeRef;
  record?: AssertionRecordRef;
  /** 収集元の内容の識別。`kind` が `source` のときに持つ。 */
  sourceContentSha256?: string;
};

/** `AssertionTarget` を検証する。 */
export const decodeAssertionTarget: Decoder<AssertionTarget> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    kind: requireEnum(source, "kind", path, assertionTargetKinds),
    nodeId: optionalString(source, "nodeId", path),
    edge: optionalMember(source, "edge", path, decodeAssertionEdgeRef),
    record: optionalMember(source, "record", path, decodeAssertionRecordRef),
    sourceContentSha256: optionalString(source, "sourceContentSha256", path),
  };
};

/** 分析者が所見に添えた根拠。 */
export type AssertionBasis = {
  /** 分析者が書いた記述。入力された言語のまま保つ。 */
  note: string;
  /** 根拠に挙げたレコード。要素数 0 の場合も集合である。 */
  recordRefs: AssertionRecordRef[];
};

/** `AssertionBasis` を検証する。 */
export const decodeAssertionBasis: Decoder<AssertionBasis> = (input, path) => {
  const source = readObject(input, path);
  return {
    note: requireString(source, "note", path),
    recordRefs: requireArray(
      source,
      "recordRefs",
      path,
      decodeAssertionRecordRef,
    ),
  };
};

/** 置き換えられた所見の改訂 1 つ。 */
export type AssertionRevision = {
  revisionNumber: number;
  state: AssertionState;
  author: string;
  recordedAt: string;
  basis: AssertionBasis;
  /** その改訂で収集元の時刻を読む UTC からのずれ。対象が収集元の所見だけが持つ。 */
  timeOffset?: string;
};

/** `AssertionRevision` を検証する。 */
export const decodeAssertionRevision: Decoder<AssertionRevision> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    revisionNumber: requireCount(source, "revisionNumber", path),
    state: requireEnum(source, "state", path, assertionStates),
    author: requireString(source, "author", path),
    recordedAt: requireString(source, "recordedAt", path),
    basis: requireMember(source, "basis", path, decodeAssertionBasis),
    timeOffset: optionalString(source, "timeOffset", path),
  };
};

/**
 * 分析者が付けた所見 1 件。現在の改訂を直接持ち、置き換えられた改訂を `history` が保つ。
 * 主張の中身は `basis.note` の自由記述である。
 */
export type Assertion = {
  id: string;
  target: AssertionTarget;
  state: AssertionState;
  author: string;
  recordedAt: string;
  basis: AssertionBasis;
  /** 現在の改訂で収集元の時刻を読む UTC からのずれ。対象が収集元の所見だけが持つ。 */
  timeOffset?: string;
  /**
   * 所見を記録した時点のグラフが対象の関係を持たなかったこと、つまり所見で関係を足したこと。
   * 関係の所見だけが真を取る。出ないときは偽である。
   */
  addsRelation?: boolean;
  /** AI 提案を採用して作った所見が持つ、元の提案の識別子。改訂で変わらない。 */
  proposalId?: string;
  revisionNumber: number;
  /** 置き換えられた改訂。古い順に並ぶ。 */
  history: AssertionRevision[];
};

/** `Assertion` を検証する。 */
export const decodeAssertion: Decoder<Assertion> = (input, path) => {
  const source = readObject(input, path);
  const assertion: Assertion = {
    id: requireString(source, "id", path),
    target: requireMember(source, "target", path, decodeAssertionTarget),
    state: requireEnum(source, "state", path, assertionStates),
    author: requireString(source, "author", path),
    recordedAt: requireString(source, "recordedAt", path),
    basis: requireMember(source, "basis", path, decodeAssertionBasis),
    timeOffset: optionalString(source, "timeOffset", path),
    addsRelation: optionalBoolean(source, "addsRelation", path),
    proposalId: optionalString(source, "proposalId", path),
    revisionNumber: requireCount(source, "revisionNumber", path),
    history: requireArray(source, "history", path, decodeAssertionRevision),
  };
  // 対象が収集元の所見は、現在の改訂と全ての履歴の改訂がずれを持つ
  // (`backend/core/assertion.go` の `validateTimeOffsets`)。
  if (assertion.target.kind === "source") {
    if (assertion.timeOffset === undefined) {
      throw new DecodeFailure(`${path}.timeOffset`, "required on a source");
    }
    assertion.history.forEach((revision, index) => {
      if (revision.timeOffset === undefined) {
        throw new DecodeFailure(
          `${path}.history[${index}].timeOffset`,
          "required on a source",
        );
      }
    });
  }
  return assertion;
};

/** 所見 1 件と、その対象が現在のグラフの何から出たか。 */
export type AssertionItem = {
  assertion: Assertion;
  targetOrigin: AssertionTargetOrigin;
};

/** `AssertionItem` を検証する。 */
export const decodeAssertionItem: Decoder<AssertionItem> = (input, path) => {
  const source = readObject(input, path);
  return {
    assertion: requireMember(source, "assertion", path, decodeAssertion),
    targetOrigin: requireEnum(
      source,
      "targetOrigin",
      path,
      assertionTargetOrigins,
    ),
  };
};

/** 所見の一覧の応答。 */
export type AssertionsResponse = {
  assertions: AssertionItem[];
  /** 所見の総数。 */
  assertionCount: number;
  /** 要素数 0 の一覧が出た理由。要素を持つ一覧では出ない。 */
  emptyReason?: string;
};

/** 所見の一覧の応答を検証する。 */
export const decodeAssertionsResponse: Decoder<AssertionsResponse> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const assertions = requireArray(
    source,
    "assertions",
    path,
    decodeAssertionItem,
  );
  return {
    assertions,
    assertionCount: requireCountEqualsElements(
      requireCount(source, "assertionCount", path),
      assertions.length,
      `${path}.assertionCount`,
    ),
    emptyReason: optionalString(source, "emptyReason", path),
  };
};
