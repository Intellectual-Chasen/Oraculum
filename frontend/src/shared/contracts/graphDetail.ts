/**
 * ノード 1 つの詳細 (操作 10) と、関係 1 本の詳細 (操作 11) の応答の型と decoder。
 * 部分グラフの応答 (操作 9) と、両方が共有するノードとエッジの型は `./graph` が持つ。
 */
import {
  type ComparisonUnit,
  comparisonUnits,
  decodeMatchAssumption,
  decodeTimeComparison,
  decodeTimeWindow,
  type MatchAssumption,
  type MatchCondition,
  matchConditionDecoder,
  type StageKey,
  stageKeys,
  type TimeComparison,
  type TimeWindow,
} from "./candidates";
import { type CaseEvidenceCount, optionalEvidenceByCase } from "./cases";
import {
  decodeObservationKind,
  decodeRecordField,
  decodeRecordLocator,
  decodeTimestamp,
  type ObservationKind,
  type RecordField,
  type RecordLocator,
  requireCountEqualsElements,
  type SemanticKey,
  type Timestamp,
} from "./common";
import {
  DecodeFailure,
  type Decoder,
  decodeString,
  optionalArray,
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
import { decodeEdgeRecordPair, type EdgeRecordPair } from "./edgeRecordPairs";
import {
  decodeEdgeDetailEdge,
  decodeGraphEvidence,
  decodeGraphNode,
  type EdgeDetailEdge,
  type EdgeDirection,
  type EdgeKind,
  edgeDirections,
  edgeKinds,
  type GraphEvidence,
  type GraphNode,
} from "./graph";
import {
  decodeTerminalAssignment,
  type TerminalAssignment,
} from "./terminalAssignments";

/** ノードの 1 つの属性に観測した値 1 件。 */
export type NodeAttributeValue = {
  field: RecordField;
  /** この値を観測したレコードの件数。 */
  observationCount: number;
  /** この値を最初に観測したレコードの位置。 */
  firstRecordRef: RecordLocator;
};

/** `NodeAttributeValue` を検証する。 */
export const decodeNodeAttributeValue: Decoder<NodeAttributeValue> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    field: requireMember(source, "field", path, decodeRecordField),
    observationCount: requireCount(source, "observationCount", path),
    firstRecordRef: requireMember(
      source,
      "firstRecordRef",
      path,
      decodeRecordLocator,
    ),
  };
};

/**
 * ノードの 1 つの意味に観測した値の集合。
 * 矛盾する値を 1 つに寄せず、`valueCount` が観測した異なる値の個数を持つ。
 */
export type NodeAttribute = {
  /** 語彙の項目。語彙に写していない欄では出ない。 */
  semantic?: SemanticKey;
  /** 原資料の key の文字列。語彙に写していない欄でだけ出る。 */
  name?: string;
  valueCount: number;
  /** 観測した値。要素数は 1 以上である。 */
  values: NodeAttributeValue[];
};

/**
 * `NodeAttribute` を検証する。
 * **語彙の項目と原資料の key のちょうど一方を求める**
 * (`backend/core/graph_response.go` の `NodeAttribute.Validate`)。画面は属性を名前で
 * 出すため、どちらも無い要素を描けない。
 */
export const decodeNodeAttribute: Decoder<NodeAttribute> = (input, path) => {
  const source = readObject(input, path);
  const values = requireArray(source, "values", path, decodeNodeAttributeValue);
  if (values.length === 0) {
    throw new DecodeFailure(`${path}.values`, "expected 1 or more elements");
  }
  const semantic = optionalString(source, "semantic", path);
  const name = optionalString(source, "name", path);
  if ((semantic === undefined) === (name === undefined)) {
    throw new DecodeFailure(
      `${path}.semantic`,
      "expected either a semantic or a name of the raw material",
    );
  }
  return {
    semantic,
    name,
    valueCount: requireCount(source, "valueCount", path),
    values,
  };
};

/** ノードに繋がるエッジの、種別と向きごとの件数。 */
export type NodeEdgeCount = {
  edgeKind: EdgeKind;
  direction: EdgeDirection;
  /** まとめたエッジの本数。 */
  edgeCount: number;
  /** そのエッジが持つ根拠のレコードの総数。 */
  evidenceCount: number;
  /** `evidenceCount` を案件ごとに分けた件数。案件を区別しない取り込みでは出ない。 */
  evidenceByCase?: CaseEvidenceCount[];
};

/** `NodeEdgeCount` を検証する。 */
export const decodeNodeEdgeCount: Decoder<NodeEdgeCount> = (input, path) => {
  const source = readObject(input, path);
  const evidenceCount = requireCount(source, "evidenceCount", path);
  return {
    edgeKind: requireEnum(source, "edgeKind", path, edgeKinds),
    direction: requireEnum(source, "direction", path, edgeDirections),
    edgeCount: requireCount(source, "edgeCount", path),
    evidenceCount,
    evidenceByCase: optionalEvidenceByCase(source, path, evidenceCount),
  };
};

/** 操作 10 (`GET /api/v0/nodes/{id}`) の応答。 */
export type NodeDetailResponse = {
  node: GraphNode;
  /** 役割が `identity` の項目を外した残りに観測した値。 */
  attributes: NodeAttribute[];
  /** 属性の意味の総数。 */
  attributeCount: number;
  /** ノードを記録したレコード。 */
  evidence: GraphEvidence[];
  /** 根拠のレコードの総数。 */
  evidenceCount: number;
  /**
   * `evidenceCount` を案件ごとに分けた件数。案件を区別しない取り込みでは出ない。
   * 定義元は `backend/api/nodes.go` の `nodeResponse.EvidenceByCase` である。
   */
  evidenceByCase?: CaseEvidenceCount[];
  edgeCounts: NodeEdgeCount[];
  /**
   * 対象の生成を記録した根拠のレコード。全件を持つ。
   * 生成を記録した根拠を持てない種別のノードでは要素数 0 である。
   */
  creationRecords: GraphEvidence[];
  /** 打ち切りの前に数えた生成の根拠のレコードの総数。 */
  creationRecordCount: number;
  /**
   * このレコードを起点にして関係を導いた結果。
   * **レコードの種別のノードだけが持つ。** 他の種別のノードは起点にならない。
   */
  relationDerivation?: RelationDerivation;
  /**
   * このレコードの操作の Logon ID が一致しながら、関係にしなかったログオンのレコードと理由。
   * 全件を持つ。定義元は `backend/core/logon_session.go` の `LogonSessionRejection` である。
   */
  logonSessionRejections: LogonSessionRejection[];
};

/**
 * Logon ID が一致したログオンと操作を関係にしなかった理由。
 * 定義元は `backend/core/logon_session.go` の `LogonSessionRejectionReason` である。
 */
export const logonSessionRejectionReasons = [
  "other_terminal",
  "other_source",
  "logon_after_operation",
  "time_not_comparable",
] as const;
export type LogonSessionRejectionReason =
  (typeof logonSessionRejectionReasons)[number];

/** 関係にしなかったログオンのレコード 1 件と理由。 */
export type LogonSessionRejection = {
  reason: LogonSessionRejectionReason;
  logon: GraphEvidence;
};

/** `LogonSessionRejection` を検証する。 */
export const decodeLogonSessionRejection: Decoder<LogonSessionRejection> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    reason: requireEnum(source, "reason", path, logonSessionRejectionReasons),
    logon: requireMember(source, "logon", path, decodeGraphEvidence),
  };
};

/**
 * 1 件のレコードを起点にして関係を導いた結果の分類。
 * 定義元は `backend/core/relation_derivation.go` の `RelationDerivationOutcome` である。
 */
export const relationDerivationOutcomes = [
  "not_used_as_origin",
  "matched",
  "destination_ip_absent",
  "destination_port_absent",
  "no_candidate_record",
  "candidate_item_unreadable",
  "terminal_undetermined",
  "terminal_id_absent",
  "proxy_address_unknown",
  "origin_item_unreadable",
  "candidate_set_failed",
  "no_candidate_matching_conditions",
  "no_candidate_in_window",
  "node_unresolved",
  "source_declaration_conflict",
  "no_compared_condition",
] as const;
export type RelationDerivationOutcome =
  (typeof relationDerivationOutcomes)[number];

/**
 * 分類が何について言えることか。
 * 定義元は `backend/core/relation_derivation.go` の `RelationDerivationBasis` である。
 */
export const relationDerivationBases = [
  "source_fact",
  "input_item_absent",
  "source_declaration_defect",
  "internal_gap",
  "analyst_selection",
  "not_applicable",
] as const;
export type RelationDerivationBasis = (typeof relationDerivationBases)[number];

/** レコード 1 件を起点にして関係を導いた結果。 */
export type RelationDerivation = {
  outcome: RelationDerivationOutcome;
  basis: RelationDerivationBasis;
};

/** `RelationDerivation` を検証する。 */
export const decodeRelationDerivation: Decoder<RelationDerivation> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    outcome: requireEnum(source, "outcome", path, relationDerivationOutcomes),
    basis: requireEnum(source, "basis", path, relationDerivationBases),
  };
};

/** 操作 10 の応答を検証する。 */
export const decodeNodeDetailResponse: Decoder<NodeDetailResponse> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const attributes = requireArray(
    source,
    "attributes",
    path,
    decodeNodeAttribute,
  );
  const evidence = requireArray(source, "evidence", path, decodeGraphEvidence);
  const creationRecords = requireArray(
    source,
    "creationRecords",
    path,
    decodeGraphEvidence,
  );
  const evidenceCount = requireCountEqualsElements(
    requireCount(source, "evidenceCount", path),
    evidence.length,
    `${path}.evidenceCount`,
  );
  const creationRecordCount = requireCount(source, "creationRecordCount", path);
  // 生成の根拠は打ち切りを持たない。総数と返った件数が食い違う応答を受け取らない。
  if (creationRecordCount !== creationRecords.length) {
    throw new DecodeFailure(
      `${path}.creationRecordCount`,
      `expected ${creationRecords.length} to match the returned creationRecords`,
    );
  }
  return {
    node: requireMember(source, "node", path, decodeGraphNode),
    attributes,
    attributeCount: requireCountEqualsElements(
      requireCount(source, "attributeCount", path),
      attributes.length,
      `${path}.attributeCount`,
    ),
    evidence,
    evidenceCount,
    evidenceByCase: optionalEvidenceByCase(source, path, evidenceCount),
    edgeCounts: requireArray(source, "edgeCounts", path, decodeNodeEdgeCount),
    creationRecords,
    creationRecordCount,
    relationDerivation: optionalMember(
      source,
      "relationDerivation",
      path,
      decodeRelationDerivation,
    ),
    logonSessionRejections: requireArray(
      source,
      "logonSessionRejections",
      path,
      decodeLogonSessionRejection,
    ),
  };
};

/** 根拠の区分のレコードに現れたアカウント 1 つ。 */
export type EdgeEvidenceAccount = {
  node: GraphNode;
  /** そのアカウントが現れた、区分の中の根拠のレコードの件数。 */
  evidenceCount: number;
};

/** `EdgeEvidenceAccount` を検証する。 */
export const decodeEdgeEvidenceAccount: Decoder<EdgeEvidenceAccount> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    node: requireMember(source, "node", path, decodeGraphNode),
    evidenceCount: requireCount(source, "evidenceCount", path),
  };
};

/**
 * 根拠の区分 1 つを要求で指す値。
 *
 * **値を組むのは backend である。** 画面は応答が返した値をそのまま要求へ返す。
 * 比べられる値を作る規則を画面が持つと、backend の規則とずれる。関係の成立を判定する規則は
 * backend だけが持つ。
 */
export type EdgeEvidenceSelector = {
  eventCategory: string;
  eventAction: string;
  /** 出ない場合は、区分のレコードが接続先 port の欄を持っていない。 */
  destinationPort?: string;
  /** 真の場合は、接続先 port の欄を持たないレコードの区分である。 */
  destinationPortAbsent?: boolean;
  /** 出ない場合は、区分のレコードがログオンの種別を持っていない。 */
  logonType?: string;
  /** 真の場合は、ログオンの種別を持たないレコードの区分である。 */
  logonTypeAbsent?: boolean;
  /**
   * HTTP の要求の区分の HTTP の状態。`httpStatusAbsent` が真の場合は、状態の値を持たない
   * HTTP の要求の区分である。HTTP の要求でない区分では、どちらも出ない。観測の種別を
   * 持たない HTTP の要求の区分では、`eventCategory` と `eventAction` が空の文字列である。
   */
  httpStatus?: string;
  httpStatusAbsent?: boolean;
};

/** `EdgeEvidenceSelector` を検証する。 */
export const decodeEdgeEvidenceSelector: Decoder<EdgeEvidenceSelector> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const destinationPort = optionalString(source, "destinationPort", path);
  const destinationPortAbsent = optionalBoolean(
    source,
    "destinationPortAbsent",
    path,
  );
  if ((destinationPort === undefined) !== (destinationPortAbsent === true)) {
    throw new DecodeFailure(
      `${path}.destinationPort`,
      "expected either the destination port or the flag saying the field is absent",
    );
  }
  const logonType = optionalString(source, "logonType", path);
  const logonTypeAbsent = optionalBoolean(source, "logonTypeAbsent", path);
  if ((logonType === undefined) !== (logonTypeAbsent === true)) {
    throw new DecodeFailure(
      `${path}.logonType`,
      "expected either the logon type or the flag saying the value is absent",
    );
  }
  const httpStatus = optionalString(source, "httpStatus", path);
  const httpStatusAbsent = optionalBoolean(source, "httpStatusAbsent", path);
  if (httpStatus !== undefined && httpStatusAbsent === true) {
    throw new DecodeFailure(
      `${path}.httpStatus`,
      "expected either the HTTP status or the flag saying the value is absent",
    );
  }
  return {
    eventCategory: requireString(source, "eventCategory", path),
    eventAction: requireString(source, "eventAction", path),
    destinationPort,
    destinationPortAbsent,
    logonType,
    logonTypeAbsent,
    httpStatus,
    httpStatusAbsent,
  };
};

/**
 * エッジ 1 本の根拠を、観測の種別と接続先 port とログオンの種別で分けた 1 区分。
 *
 * **接続先 port とログオンの種別のそれぞれで、欄と出ない理由は排他である。** どちらも
 * 持たない区分と、両方を持つ区分を読み込みが退ける。**欠測を 0 で埋めない。**
 *
 * **区分を指す値と、指せない理由も排他である。** 指せない区分は、指せない理由を持つ。
 */
export type EdgeEvidenceGroup = {
  observationKind: ObservationKind;
  /** 出ない場合は、区分のレコードが接続先 port を持っていない。 */
  destinationPort?: RecordField;
  /** 出ない場合は、区分が接続先 port を持っている。 */
  destinationPortAbsence?: string;
  /** 出ない場合は、区分のレコードがログオンの種別の比べられる値を持っていない。 */
  logonType?: RecordField;
  /** 出ない場合は、区分がログオンの種別を持っている。 */
  logonTypeAbsence?: string;
  /**
   * HTTP の要求の区分の HTTP の状態と、状態が出ない理由。2 つは排他であり、HTTP の要求で
   * ない区分では、どちらも出ない。
   */
  httpStatus?: RecordField;
  httpStatusAbsence?: string;
  /** 出ない場合は、区分を要求で指せない。 */
  selector?: EdgeEvidenceSelector;
  /** 出ない場合は、区分を要求で指せる。 */
  selectorAbsence?: string;
  /** 区分のレコードに現れたアカウント。要素数 0 の場合も集合である。 */
  accounts: EdgeEvidenceAccount[];
  /** 区分のレコードが記録した認証の方式。要素数 0 の場合も集合である。 */
  authentications: EdgeEvidenceAuthentication[];
  /** 区分に属する根拠のレコードの件数。 */
  evidenceCount: number;
};

/**
 * 根拠の区分のレコードが記録した認証の方式 1 つ。定義元は `backend/core/graph_response.go` の
 * `EdgeEvidenceAuthentication` である。
 */
export type EdgeEvidenceAuthentication = {
  /** その方式を記録した最初のレコードの欄。 */
  value: RecordField;
  /** その方式を記録した、区分の中の根拠のレコードの件数。 */
  evidenceCount: number;
};

const decodeEdgeEvidenceAuthentication: Decoder<EdgeEvidenceAuthentication> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    value: requireMember(source, "value", path, decodeRecordField),
    evidenceCount: requireCount(source, "evidenceCount", path),
  };
};

/** `EdgeEvidenceGroup` を検証する。 */
export const decodeEdgeEvidenceGroup: Decoder<EdgeEvidenceGroup> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const destinationPort = optionalMember(
    source,
    "destinationPort",
    path,
    decodeRecordField,
  );
  const destinationPortAbsence = optionalString(
    source,
    "destinationPortAbsence",
    path,
  );
  if (
    (destinationPort === undefined) ===
    (destinationPortAbsence === undefined)
  ) {
    throw new DecodeFailure(
      `${path}.destinationPort`,
      "expected either the destination port or the reason it is absent",
    );
  }
  const logonType = optionalMember(
    source,
    "logonType",
    path,
    decodeRecordField,
  );
  const logonTypeAbsence = optionalString(source, "logonTypeAbsence", path);
  if ((logonType === undefined) === (logonTypeAbsence === undefined)) {
    throw new DecodeFailure(
      `${path}.logonType`,
      "expected either the logon type or the reason it is absent",
    );
  }
  const httpStatus = optionalMember(
    source,
    "httpStatus",
    path,
    decodeRecordField,
  );
  const httpStatusAbsence = optionalString(source, "httpStatusAbsence", path);
  if (httpStatus !== undefined && httpStatusAbsence !== undefined) {
    throw new DecodeFailure(
      `${path}.httpStatus`,
      "expected either the HTTP status or the reason it is absent",
    );
  }
  const selector = optionalMember(
    source,
    "selector",
    path,
    decodeEdgeEvidenceSelector,
  );
  const selectorAbsence = optionalString(source, "selectorAbsence", path);
  if ((selector === undefined) === (selectorAbsence === undefined)) {
    throw new DecodeFailure(
      `${path}.selector`,
      "expected either the selector or the reason it is absent",
    );
  }
  return {
    observationKind: requireMember(
      source,
      "observationKind",
      path,
      decodeObservationKind,
    ),
    destinationPort,
    destinationPortAbsence,
    logonType,
    logonTypeAbsence,
    httpStatus,
    httpStatusAbsence,
    selector,
    selectorAbsence,
    accounts: requireArray(source, "accounts", path, decodeEdgeEvidenceAccount),
    authentications: requireArray(
      source,
      "authentications",
      path,
      decodeEdgeEvidenceAuthentication,
    ),
    evidenceCount: requireCount(source, "evidenceCount", path),
  };
};

/**
 * 候補が 0 件になった理由。定義元は `backend/core/candidate.go` の `EmptyReason` である。
 */
export const matchStageEmptyReasons = [
  "no_record_in_filter",
  "no_source_ingested",
  "no_candidate_in_window",
  "no_candidate_matching_conditions",
  "origin_outside_assignment_range",
  "publication_withheld",
  "counterpart_item_absent",
] as const;
export type MatchStageEmptyReason = (typeof matchStageEmptyReasons)[number];

/**
 * 1 つの起点に対して段階 1 つが挙げた候補を数えた結果。
 *
 * **候補を 0 件にした段階も要素になる。** 段階そのものが消えると、その段階を実行して 0 件
 * だったのか、その段階を実行していないのかを読み分けられない。
 */
export type MatchStageTally = {
  stageKey: StageKey;
  memberCount: number;
  /** 出ない場合は候補がプロセスの項目を持たない。 */
  distinctProcessCount?: number;
  /** 候補が 1 件以上のとき出ない。 */
  emptyReason?: MatchStageEmptyReason;
};

/** `MatchStageTally` を検証する。 */
export const decodeMatchStageTally: Decoder<MatchStageTally> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const memberCount = requireCount(source, "memberCount", path);
  const tally: MatchStageTally = {
    stageKey: requireEnum(source, "stageKey", path, stageKeys),
    memberCount,
    distinctProcessCount: optionalCount(source, "distinctProcessCount", path),
  };
  if (memberCount === 0) {
    tally.emptyReason = requireEnum(
      source,
      "emptyReason",
      path,
      matchStageEmptyReasons,
    );
  }
  return tally;
};

/**
 * 関連付けと段階が指すレコード 1 件。定義元は `backend/core/edge_match_table.go` の
 * `EdgeMatchRecord` である。
 */
export type EdgeMatchRecord = {
  ref: RecordLocator;
  /** レコードの事象の時刻。時刻を持たないレコードでは出ない。 */
  eventTime?: Timestamp;
  /** Proxy のログのレコードが記録した要求処理の結果の欄。欄を持たないレコードでは出ない。 */
  proxyStatus?: RecordField;
};

/** `EdgeMatchRecord` を検証する。 */
export const decodeEdgeMatchRecord: Decoder<EdgeMatchRecord> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    ref: requireMember(source, "ref", path, decodeRecordLocator),
    eventTime: optionalMember(source, "eventTime", path, decodeTimestamp),
    proxyStatus: optionalMember(source, "proxyStatus", path, decodeRecordField),
  };
};

/**
 * 起点 1 件に対して候補を挙げた段階と、その段階の関連付けが共有する値。定義元は
 * `backend/core/edge_match_table.go` の `EdgeMatchStage` である。
 *
 * **確度を判断する材料をこの型が持つ。** 同じ起点が候補を 1 件だけ挙げたのか
 * 5 件挙げたのかで、関連付けが表す確からしさが変わる。
 */
export type EdgeMatchStage = {
  /** 起点のレコードの、`matchRecords` での位置。 */
  origin: number;
  stageKey: StageKey;
  /** 関連付けに用いた条件と、用いなかった条件の全数。 */
  conditions: MatchCondition[];
  /** 段階が依拠する前提。要素数 0 の場合も集合である。 */
  assumptions: MatchAssumption[];
  timeWindow: TimeWindow;
  /** 関連付けが収集元の時計に依拠する箇所。 */
  clockDependencyNote: string;
  /**
   * この起点に対して関連付けが求めた段階を、段階の並び順ですべて数えた結果。
   * 要素数は 1 以上で、末尾の要素の段階がこの段階である。
   */
  stageTallies: MatchStageTally[];
  /** 段階が起点と候補の時刻を比べた単位。 */
  comparisonUnit: ComparisonUnit;
  /**
   * 段階が用いた条件だけでは互いに区別できない候補の組。要素は `matchRecords` での位置である。
   * 要素数 0 の場合も集合である。
   */
  indistinguishableGroups: number[][];
  /** 段階の関連付けが持つ確定しない理由の異なり。関連付けは位置で指す。 */
  unresolvedReasonSets: string[][];
};

/**
 * 関連付けの結果 1 件。段階とレコードと理由を位置で指す。定義元は
 * `backend/core/edge_match_table.go` の `EdgeMatchRef` である。
 */
export type EdgeMatchRef = {
  /** 関連付けを出した段階の、`matchStages` での位置。 */
  stage: number;
  /** 候補のレコードの、`matchRecords` での位置。 */
  candidate: number;
  /** 確定しない理由の、段階の `unresolvedReasonSets` での位置。 */
  unresolvedReasons: number;
  /** 段階と両端のレコードの時刻から組み直せない時刻の比較。組み直せる関連付けでは出ない。 */
  timeComparison?: TimeComparison;
};

/** 表の位置を読む。0 以上の整数だけを受け取る。 */
const decodePosition: Decoder<number> = (input, path) => {
  if (
    typeof input !== "number" ||
    !Number.isSafeInteger(input) ||
    input < 0 ||
    Object.is(input, -0)
  ) {
    throw new DecodeFailure(path, "expected a non-negative safe integer");
  }
  return input;
};

/**
 * 確定しない理由の 1 組を検証する。要素数 0 と空の文字列を通さない。
 * 理由の無い候補は、何が確定していないかを分析者が読めない。
 */
const decodeReasons: Decoder<string[]> = (input, path) => {
  if (!Array.isArray(input)) {
    throw new DecodeFailure(path, "expected an array");
  }
  if (input.length === 0) {
    throw new DecodeFailure(path, "expected 1 or more elements");
  }
  return input.map((element, index) => {
    const reason = decodeString(element, `${path}[${index}]`);
    if (reason === "") {
      throw new DecodeFailure(`${path}[${index}]`, "expected a reason");
    }
    return reason;
  });
};

/** 区別できない候補の 1 組を、位置の組として検証する。要素数は 2 以上である。 */
const decodePositionGroup: Decoder<number[]> = (input, path) => {
  if (!Array.isArray(input)) {
    throw new DecodeFailure(path, "expected an array");
  }
  if (input.length < 2) {
    throw new DecodeFailure(path, "expected 2 or more elements");
  }
  return input.map((element, index) =>
    decodePosition(element, `${path}[${index}]`),
  );
};

/** `EdgeMatchStage` を検証する。位置が表の範囲にあることは応答の検証が確かめる。 */
export const decodeEdgeMatchStage: Decoder<EdgeMatchStage> = (input, path) => {
  const source = readObject(input, path);
  const stageKey = requireEnum(source, "stageKey", path, stageKeys);
  const stageTallies = requireArray(
    source,
    "stageTallies",
    path,
    decodeMatchStageTally,
  );
  const lastTally = stageTallies[stageTallies.length - 1];
  if (lastTally === undefined) {
    throw new DecodeFailure(
      `${path}.stageTallies`,
      "expected 1 or more elements",
    );
  }
  if (lastTally.stageKey !== stageKey || lastTally.memberCount < 1) {
    throw new DecodeFailure(
      `${path}.stageTallies`,
      "expected to end at the stage with 1 or more candidates",
    );
  }
  const unresolvedReasonSets = requireArray(
    source,
    "unresolvedReasonSets",
    path,
    decodeReasons,
  );
  if (unresolvedReasonSets.length === 0) {
    throw new DecodeFailure(
      `${path}.unresolvedReasonSets`,
      "expected 1 or more elements",
    );
  }
  return {
    origin: requireMember(source, "origin", path, decodePosition),
    stageKey,
    conditions: requireArray(
      source,
      "conditions",
      path,
      matchConditionDecoder(true),
    ),
    assumptions: requireArray(
      source,
      "assumptions",
      path,
      decodeMatchAssumption,
    ),
    timeWindow: requireMember(source, "timeWindow", path, decodeTimeWindow),
    clockDependencyNote: requireString(source, "clockDependencyNote", path),
    stageTallies,
    comparisonUnit: requireEnum(
      source,
      "comparisonUnit",
      path,
      comparisonUnits,
    ),
    indistinguishableGroups: requireArray(
      source,
      "indistinguishableGroups",
      path,
      decodePositionGroup,
    ),
    unresolvedReasonSets,
  };
};

/** `EdgeMatchRef` を検証する。位置が表の範囲にあることは応答の検証が確かめる。 */
export const decodeEdgeMatchRef: Decoder<EdgeMatchRef> = (input, path) => {
  const source = readObject(input, path);
  return {
    stage: requireMember(source, "stage", path, decodePosition),
    candidate: requireMember(source, "candidate", path, decodePosition),
    unresolvedReasons: requireMember(
      source,
      "unresolvedReasons",
      path,
      decodePosition,
    ),
    timeComparison: optionalMember(
      source,
      "timeComparison",
      path,
      decodeTimeComparison,
    ),
  };
};

/**
 * 関連付けの結果 1 件の時刻の比較を返す。規則の定義元は `backend/core/edge_match_table.go` の
 * `EdgeMatchTable.Match` である。
 *
 * 関連付けが比較を持つときはその値である。持たないときは、段階の単位が `second` なら起点と
 * 候補の事象の時刻を段階の前提で比べた値であり、それ以外なら比べていない値である。
 * 段階の前提のうち、候補の母集合を組むときの前提 (`counterpart_connected_to_proxy`) は
 * 時刻の比較に載せない (`backend/core/assumption.go` の `TimeComparisonAssumptions`)。
 */
export function edgeMatchTimeComparison(
  detail: Pick<EdgeDetailResponse, "matchRecords" | "matchStages">,
  match: EdgeMatchRef,
): TimeComparison {
  if (match.timeComparison !== undefined) {
    return match.timeComparison;
  }
  const stage = detail.matchStages[match.stage];
  if (stage.comparisonUnit !== "second") {
    return { comparisonUnit: stage.comparisonUnit, assumptions: [] };
  }
  return {
    comparisonUnit: "second",
    leftTime: detail.matchRecords[stage.origin].eventTime,
    rightTime: detail.matchRecords[match.candidate].eventTime,
    assumptions: stage.assumptions.filter(
      (each) => each.assumptionKey !== "counterpart_connected_to_proxy",
    ),
  };
}

/** 位置 `position` が要素数 `length` の表の範囲にあることを確かめる。 */
function requireInTable(position: number, length: number, path: string) {
  if (position >= length) {
    throw new DecodeFailure(path, `expected a position below ${length}`);
  }
}

/**
 * 段階と関連付けが指す位置が表の範囲にあり、時刻を比べた段階の関連付けが両端の時刻を組めることを
 * 確かめる。
 *
 * **すべての段階を関連付けが 1 件以上指す。** 関連付けを持たない段階は、候補を挙げた段階の件数と
 * 食い違う。関連付けが無い表はレコードも持たない。
 */
function requireMatchTable(
  records: EdgeMatchRecord[],
  stages: EdgeMatchStage[],
  matches: EdgeMatchRef[],
  path: string,
) {
  if (matches.length === 0 && records.length > 0) {
    throw new DecodeFailure(
      `${path}.matchRecords`,
      "expected no record without a match",
    );
  }
  stages.forEach((stage, index) => {
    const at = `${path}.matchStages[${index}]`;
    requireInTable(stage.origin, records.length, `${at}.origin`);
    stage.indistinguishableGroups.forEach((group, groupIndex) => {
      group.forEach((member, memberIndex) => {
        requireInTable(
          member,
          records.length,
          `${at}.indistinguishableGroups[${groupIndex}][${memberIndex}]`,
        );
      });
    });
  });
  matches.forEach((match, index) => {
    const at = `${path}.matches[${index}]`;
    requireInTable(match.stage, stages.length, `${at}.stage`);
    requireInTable(match.candidate, records.length, `${at}.candidate`);
    const stage = stages[match.stage];
    requireInTable(
      match.unresolvedReasons,
      stage.unresolvedReasonSets.length,
      `${at}.unresolvedReasons`,
    );
    if (
      match.timeComparison === undefined &&
      stage.comparisonUnit === "second" &&
      (records[stage.origin].eventTime === undefined ||
        records[match.candidate].eventTime === undefined)
    ) {
      throw new DecodeFailure(
        `${at}.timeComparison`,
        "expected the event times of the origin and the candidate",
      );
    }
    if (
      match.timeComparison !== undefined &&
      match.timeComparison.comparisonUnit !== stage.comparisonUnit
    ) {
      throw new DecodeFailure(
        `${at}.timeComparison.comparisonUnit`,
        "expected the comparison unit of the stage",
      );
    }
  });
  // 段階の関連付けは、段階が挙げた候補 (末尾の段階の件数) のうちこの関係を作った候補である。
  const matchCounts = new Map<number, number>();
  for (const match of matches) {
    matchCounts.set(match.stage, (matchCounts.get(match.stage) ?? 0) + 1);
  }
  stages.forEach((stage, index) => {
    const count = matchCounts.get(index);
    if (count === undefined) {
      throw new DecodeFailure(
        `${path}.matchStages[${index}]`,
        "expected a match of the stage",
      );
    }
    const memberCount =
      stage.stageTallies[stage.stageTallies.length - 1].memberCount;
    if (count > memberCount) {
      throw new DecodeFailure(
        `${path}.matchStages[${index}]`,
        `expected at most ${memberCount} matches of the stage`,
      );
    }
  });
}

/** 接続元のアドレスから端末を導いて作った候補の関係の、成立の根拠。 */
export type EdgeAssignmentBasis = {
  /** 端末を導くのに用いたアドレスの文字列。 */
  clientIp: string;
  /** 用いた割当の適用期間を読み取った収集元。 */
  sourceId: string;
  /** 成立に用いた条件の全数。`terminal_ip_assignment` を必ず含む。 */
  conditions: MatchCondition[];
  assumptions: MatchAssumption[];
  clockDependencyNote: string;
};

/** `EdgeAssignmentBasis` を検証する。 */
export const decodeEdgeAssignmentBasis: Decoder<EdgeAssignmentBasis> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const conditions = requireArray(
    source,
    "conditions",
    path,
    matchConditionDecoder(true),
  );
  if (
    !conditions.some(
      ({ conditionKey }) => conditionKey === "terminal_ip_assignment",
    )
  ) {
    throw new DecodeFailure(
      `${path}.conditions`,
      "expected terminal_ip_assignment condition",
    );
  }
  return {
    clientIp: requireString(source, "clientIp", path),
    sourceId: requireString(source, "sourceId", path),
    conditions,
    assumptions: requireArray(
      source,
      "assumptions",
      path,
      decodeMatchAssumption,
    ),
    clockDependencyNote: requireString(source, "clockDependencyNote", path),
  };
};

export type EdgeDetailResponse = {
  edge: EdgeDetailEdge;
  sourceNode: GraphNode;
  targetNode: GraphNode;
  /** 関連付けと段階が指すレコード。同じレコードを 2 度置かない。 */
  matchRecords: EdgeMatchRecord[];
  /** 起点 1 件に対して候補を挙げた段階。 */
  matchStages: EdgeMatchStage[];
  /** エッジを作った関連付け。観測から直接作ったエッジでは要素数 0 である。 */
  matches: EdgeMatchRef[];
  /** 関連付けの総数。 */
  matchCount: number;
  /**
   * 接続元のアドレスから端末を導いて作った候補の関係の根拠。
   * 出ない場合は、そのエッジがアドレスからの導出に依らない。
   */
  assignmentBases?: EdgeAssignmentBasis[];
  /** エッジを作った端末の割当。利用者が収集元に付けた割当の IP から作ったエッジだけが持つ。 */
  terminalAssignments?: TerminalAssignment[];
  /**
   * `terminalAssignments` を持つエッジを、レコードも直に観測したか。`terminalAssignments` と
   * 同時に出る。定義元は `backend/pipeline/graph_edge_detail.go` の `ObservedInRecords` である。
   */
  observedInRecords?: boolean;
  /**
   * 観測の層が候補の関係を作ったレコードの組。先頭から上限までの組を持つ。組を持たない
   * 関係では出ない。
   */
  recordPairs?: EdgeRecordPair[];
  /** レコードの組の総数。`recordPairs` と同時に出る。 */
  recordPairCount?: number;
  /** 根拠を観測の種別と接続先 port で分けた区分。絞り込みの有無で変わらない。 */
  evidenceGroups: EdgeEvidenceGroup[];
};

/** 操作 11 の応答を検証する。 */
export const decodeEdgeDetailResponse: Decoder<EdgeDetailResponse> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const matchRecords = requireArray(
    source,
    "matchRecords",
    path,
    decodeEdgeMatchRecord,
  );
  const matchStages = requireArray(
    source,
    "matchStages",
    path,
    decodeEdgeMatchStage,
  );
  const matches = requireArray(source, "matches", path, decodeEdgeMatchRef);
  requireMatchTable(matchRecords, matchStages, matches, path);
  const recordPairs = optionalArray(
    source,
    "recordPairs",
    path,
    decodeEdgeRecordPair,
  );
  const recordPairCount = optionalCount(source, "recordPairCount", path);
  if (
    (recordPairs === undefined) !== (recordPairCount === undefined) ||
    (recordPairs !== undefined &&
      recordPairCount !== undefined &&
      recordPairs.length > recordPairCount)
  ) {
    throw new DecodeFailure(
      `${path}.recordPairCount`,
      "expected the count of record pairs beside at most as many pairs",
    );
  }
  return {
    edge: requireMember(source, "edge", path, decodeEdgeDetailEdge),
    sourceNode: requireMember(source, "sourceNode", path, decodeGraphNode),
    targetNode: requireMember(source, "targetNode", path, decodeGraphNode),
    matchRecords,
    matchStages,
    matches,
    matchCount: requireCountEqualsElements(
      requireCount(source, "matchCount", path),
      matches.length,
      `${path}.matchCount`,
    ),
    assignmentBases: optionalArray(
      source,
      "assignmentBases",
      path,
      decodeEdgeAssignmentBasis,
    ),
    terminalAssignments: optionalArray(
      source,
      "terminalAssignments",
      path,
      decodeTerminalAssignment,
    ),
    observedInRecords: optionalBoolean(source, "observedInRecords", path),
    recordPairs,
    recordPairCount,
    evidenceGroups: requireArray(
      source,
      "evidenceGroups",
      path,
      decodeEdgeEvidenceGroup,
    ),
  };
};
