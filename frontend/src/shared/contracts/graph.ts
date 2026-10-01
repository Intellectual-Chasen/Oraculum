import {
  type CaseEvidenceCount,
  type CaseId,
  decodeCaseId,
  optionalEvidenceByCase,
} from "./cases";
import {
  decodeEventKindPair,
  decodeObservationKind,
  decodeRawAndNormalized,
  decodeRecordLocator,
  decodeRequestedTime,
  decodeTimeRange,
  decodeTimestamp,
  type EventKindPair,
  type ObservationKind,
  type RawAndNormalized,
  type RecordLocator,
  type RequestedTime,
  requireCountEqualsElements,
  type SemanticKey,
  type TimeRange,
  type Timestamp,
  type TimestampPrecision,
} from "./common";
import {
  DecodeFailure,
  type Decoder,
  decodeString,
  optionalArray,
  optionalBoolean,
  optionalCount,
  optionalEnum,
  optionalMember,
  optionalString,
  readObject,
  rejectMember,
  requireArray,
  requireCount,
  requireEnum,
  requireMember,
  requireString,
} from "./decoding";
import {
  type TerminalAssignmentOrigin,
  terminalAssignmentOrigins,
} from "./terminalAssignments";

/**
 * ノードの種別。定義元は `backend/core/graph.go` の `NodeKind` である。
 * 値は機械処理用の key であり、画面の文言は feature の `labels.ts` が持つ。
 */
export const nodeKinds = [
  "terminal",
  "process",
  "file",
  "registry_value",
  "account",
  "ip",
  "domain",
  "record",
] as const;
export type NodeKind = (typeof nodeKinds)[number];

/** 集合の要素としてノードの種別を読む。 */
export const decodeNodeKind: Decoder<NodeKind> = (input, path) => {
  const found = nodeKinds.find((kind) => kind === input);
  if (found === undefined) {
    throw new DecodeFailure(path, `expected one of ${nodeKinds.join(" / ")}`);
  }
  return found;
};

/**
 * 部分グラフにレコードのノードを出すか、対象のノードだけを出すか。定義元は
 * `backend/core/graph_granularity.go` の `GraphGranularity` である。
 */
export const graphGranularities = ["record", "object"] as const;
export type GraphGranularity = (typeof graphGranularities)[number];

/**
 * 関係の種別。定義元は `backend/core/graph.go` の `EdgeKind` である。
 */
export const edgeKinds = [
  "ran_on",
  "process_parent_child",
  "process_injection",
  "file_operation",
  "file_copy",
  "process_executable",
  "registry_operation",
  "process_communication",
  "terminal_address",
  "terminal_remote_session",
  "terminal_account",
  "http_request",
  "file_content_match",
  "cross_source_connection_match",
  "record_names_object",
  "record_subject_account",
  "record_target_account",
  "logon_session_operation",
  "argument_names_object",
  "task_registration_run",
  "linked_logon",
  "ticket_request_logon",
  "reverse_lookup_name",
  "connection_logon_match",
  "process_identity_match",
  "explicit_credential_logon",
  "unidentified_source_remote_session",
  "terminal_outbound_connection",
  "account_identity_match",
  "inbound_connection_match",
  "same_connection_match",
  "requested_session_logon",
  "logon_chain",
] as const;
export type EdgeKind = (typeof edgeKinds)[number];

/** 識別鍵の形。定義元は `backend/core/graph.go` の `NodeKeyForm` である。 */
export const nodeKeyForms = [
  "terminal_id",
  "terminal_id_process_id",
  "terminal_id_file_path",
  "terminal_id_registry_value_key_path",
  "account_sid",
  "account_domain_name",
  "address",
  "hostname",
  "source_content_sha256_position",
  "terminal_id_process_pid_interval",
  "recording_source_content_sha256",
  "recording_source_content_sha256_hostname",
  "terminal_id_account_name",
  "terminal_id_address",
  "collection_content_sha256",
] as const;
export type NodeKeyForm = (typeof nodeKeyForms)[number];

/** 応答がノードを持つ理由。定義元は `backend/core/graph_response.go` の `NodeSelection` である。 */
export const nodeSelections = ["matched", "edge_endpoint"] as const;
export type NodeSelection = (typeof nodeSelections)[number];

/**
 * ノードを記録したレコードの種類。定義元は `backend/core/graph.go` の `NodeObservation` である。
 * その対象を記録したレコードがあるかを表す。生成のレコードの有無は `creationRecord` が表す。
 */
export const nodeObservations = ["observed", "referenced"] as const;
export type NodeObservation = (typeof nodeObservations)[number];

/**
 * 対象の生成を記録したレコードが根拠にあるか。
 * 定義元は `backend/core/graph.go` の `NodeCreationRecord` である。
 * `item_absent` は生成を記録した根拠を持てない種別である。
 */
export const nodeCreationRecords = [
  "present",
  "absent",
  "item_absent",
] as const;
export type NodeCreationRecord = (typeof nodeCreationRecords)[number];

/** 関係の状態。定義元は `backend/core/candidate.go` の `RelationState` である。 */
export const relationStates = [
  "observed",
  "candidate",
  "uncertain_chain",
] as const;
export type RelationState = (typeof relationStates)[number];

/** ノードから見たエッジの向き。定義元は `backend/core/graph_response.go` の `EdgeDirection` である。 */
export const edgeDirections = ["outgoing", "incoming"] as const;
export type EdgeDirection = (typeof edgeDirections)[number];

/** 絞り込みに使う時刻の比較の単位。定義元は `backend/api/request_items.go` の `filterUnit` である。 */
export const filterUnits = ["second", "millisecond", "microsecond"] as const;
export type FilterUnit = (typeof filterUnits)[number];

/**
 * 期間の端の精度。`timeFromPrecision` と `timeToPrecision` が取る値。
 * 日付と UTC からのずれを持つ文字列の正規化値が `rfc3339_absolute` になるのは、精度が
 * `second`、`millisecond`、`microsecond` のときである。絶対時刻として比較できる正規化値は
 * `rfc3339_absolute` である (定義元は `backend/core/timestamp.go` の
 * `expectedNormalizedForm`)。
 */
export const graphBoundPrecisions = [
  "second",
  "millisecond",
  "microsecond",
] as const satisfies readonly TimestampPrecision[];
export type GraphBoundPrecision = (typeof graphBoundPrecisions)[number];

/** 期間の端 1 つ。文字列と精度を組で持つ。 */
export type GraphTimeBound = {
  /** 日付と UTC からのずれを含む文字列。 */
  text: string;
  precision: GraphBoundPrecision;
};

/**
 * 根拠のレコードを絞る期間。
 * `backend/api/graph_request.go` は時刻の文字列が 1 つ以上ある要求に `filterUnit` を
 * 必須とするため、単位を組に含める。
 */
export type GraphTimeFilter = {
  from?: GraphTimeBound;
  to?: GraphTimeBound;
  unit: FilterUnit;
};

/** `nodeCount` が 0 になった理由。 */
export const graphEmptyReasons = [
  "no_record_in_filter",
  "no_value_match",
  "value_match_outside_filter",
  "no_field_observed",
  "record_match_without_object",
] as const;
export type GraphEmptyReason = (typeof graphEmptyReasons)[number];

/** 値ごとの件数が 0 件になった理由。 */
export const valueCountsEmptyReasons = [
  "no_field_observed",
  "no_readable_value",
  "field_on_other_node_kind",
  "no_value_in_filter",
] as const;
export type ValueCountsEmptyReason = (typeof valueCountsEmptyReasons)[number];

/** 識別鍵の 1 項目の値。 */
export type NodeIdentityValue = {
  /** 出ない場合は、鍵の形が語彙の項目を 1 つに定めない。 */
  semantic?: SemanticKey;
  value: string;
};

/** `NodeIdentityValue` を検証する。 */
export const decodeNodeIdentityValue: Decoder<NodeIdentityValue> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    semantic: optionalString(source, "semantic", path),
    value: requireString(source, "value", path),
  };
};

/** グラフのノード 1 つ。`id` は backend が作る不透明な文字列である。 */
export type GraphNode = {
  id: string;
  kind: NodeKind;
  keyForm: NodeKeyForm;
  /** 識別鍵の値。要素数は 1 以上である。 */
  identity: NodeIdentityValue[];
  /** 表示名。値を持たないノードは `valueState` が `item_absent` である。 */
  label: RawAndNormalized;
  /** ノードを記録したレコードの種類。参照だけのノードは `referenced` である。 */
  observation: NodeObservation;
  /** 生成を記録したレコードが根拠にあるか。`observation` と別の軸である。 */
  creationRecord: NodeCreationRecord;
  /**
   * ファイルのノードが、端末を名乗らない収集元の名前の分からない端末に置かれたか。ファイル以外の
   * 種別のノードでは出ない (`backend/core/graph_response.go` の `GraphNode.OnUnknownTerminal`)。
   */
  onUnknownTerminal?: boolean;
  /**
   * PID の区間のノードの区間を開いたレコードの時刻。収集元の時刻の解釈を持つ。ほかの形のノードでは
   * 出ない (`backend/core/graph_response.go` の `GraphNode.IntervalStart`)。
   */
  intervalStart?: Timestamp;
  /**
   * 同じアカウントとしてまとめる鍵。アカウントのノードのうち、鍵を決められたノードだけが持つ
   * (`backend/core/graph_response.go` の `GraphNode.AccountName`)。
   */
  accountName?: AccountNameKey;
  /**
   * アカウントのノードが `accountName` を持たない理由
   * (`backend/core/graph_response.go` の `GraphNode.AccountNameWithheld`)。
   */
  accountNameWithheld?: AccountNameWithheldReason;
};

/** 同じアカウントとしてまとめる鍵 (`backend/core/graph_response.go` の `AccountNameKey`)。 */
export type AccountNameKey = {
  /** 出ない場合は、案件を区別しない取り込みである。 */
  caseId?: string;
  /** 小文字にした `ドメイン\ログイン名`。 */
  name: string;
  /** グラフ全体で同じ鍵を持つノードの数。1 以上である。 */
  nodeCount: number;
};

/** `AccountNameKey` を検証する。 */
export const decodeAccountNameKey: Decoder<AccountNameKey> = (input, path) => {
  const source = readObject(input, path);
  const nodeCount = requireCount(source, "nodeCount", path);
  if (nodeCount < 1) {
    throw new DecodeFailure(`${path}.nodeCount`, "expected 1 or more");
  }
  return {
    caseId: optionalString(source, "caseId", path),
    name: requireString(source, "name", path),
    nodeCount,
  };
};

/** アカウントのノードが鍵を持たない理由 (`backend/core/graph_response.go` の `AccountNameWithheldReason`)。 */
export const accountNameWithheldReasons = [
  "no_name_recorded",
  "multiple_names",
  "multiple_cases",
] as const;
export type AccountNameWithheldReason =
  (typeof accountNameWithheldReasons)[number];

function requireIdentity(
  source: Record<string, unknown>,
  path: string,
): NodeIdentityValue[] {
  const identity = requireArray(
    source,
    "identity",
    path,
    decodeNodeIdentityValue,
  );
  if (identity.length === 0) {
    throw new DecodeFailure(`${path}.identity`, "expected 1 or more elements");
  }
  return identity;
}

/** `GraphNode` を検証する。 */
export const decodeGraphNode: Decoder<GraphNode> = (input, path) => {
  const source = readObject(input, path);
  return {
    id: requireString(source, "id", path),
    kind: requireEnum(source, "kind", path, nodeKinds),
    keyForm: requireEnum(source, "keyForm", path, nodeKeyForms),
    identity: requireIdentity(source, path),
    label: requireMember(source, "label", path, decodeRawAndNormalized),
    observation: requireEnum(source, "observation", path, nodeObservations),
    creationRecord: requireEnum(
      source,
      "creationRecord",
      path,
      nodeCreationRecords,
    ),
    onUnknownTerminal: optionalBoolean(source, "onUnknownTerminal", path),
    intervalStart: optionalMember(
      source,
      "intervalStart",
      path,
      decodeTimestamp,
    ),
    accountName: optionalMember(
      source,
      "accountName",
      path,
      decodeAccountNameKey,
    ),
    accountNameWithheld: optionalEnum(
      source,
      "accountNameWithheld",
      path,
      accountNameWithheldReasons,
    ),
  };
};

/** 検索の文字列が一致した値の形。定義元は `backend/core/graph_response.go` の `ValueMatchForm` である。 */
export const valueMatchForms = ["raw_text", "normalized"] as const;
export type ValueMatchForm = (typeof valueMatchForms)[number];

/**
 * 値の検索が 1 つの欄に一致したこと。
 * 欄の名前は語彙の項目と原資料の key のどちらか一方が持つ。
 */
export type NodeValueMatch = {
  semantic?: SemanticKey;
  name?: string;
  form: ValueMatchForm;
  /** この欄とこの形で一致した値のうち、最初の値。`form` の形の文字列である。 */
  value: string;
};

/** `NodeValueMatch` を検証する。 */
export const decodeNodeValueMatch: Decoder<NodeValueMatch> = (input, path) => {
  const source = readObject(input, path);
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
    form: requireEnum(source, "form", path, valueMatchForms),
    value: requireString(source, "value", path),
  };
};

/**
 * 部分グラフが持つノード 1 つ。持つ理由を `selection` が持つ。
 * `valueMatches` は検索の文字列を与えた要求でだけ出る。
 */
export type SubgraphNode = GraphNode & {
  selection: NodeSelection;
  valueMatches?: NodeValueMatch[];
  /**
   * 根拠のレコードが名乗った端末。
   *
   * **母集団は `selection` ごとに違う。** `matched` のノードは絞り込みを通った根拠が
   * 名乗った端末、`edge_endpoint` のノードは根拠の全件が名乗った端末である。
   * **空であることを「端末が無い」と読まない。**
   */
  terminals?: GraphNode[];
  /**
   * レコードのノードが指すレコードの位置と時刻と事象の種別。要求が `recordSummary` を与えたときの、
   * `matched` のレコードのノードだけが持つ (`backend/core/graph_response.go` の `RecordSummary`)。
   */
  record?: RecordSummary;
};

/** レコード 1 件の位置と時刻と事象の種別 (`backend/core/graph_response.go` の `RecordSummary`)。 */
export type RecordSummary = {
  recordRef: RecordLocator;
  /** 出ない場合は、そのレコードの時刻を読めていない。 */
  eventTime?: Timestamp;
  /** Windows イベントログのレコード (`windowsEvent`) ではプロバイダの名前。 */
  eventCategory?: string;
  /** Windows イベントログのレコード (`windowsEvent`) ではイベント ID。 */
  eventAction?: string;
  windowsEvent: boolean;
  /** Windows イベントログのチャネルの名前。 */
  channel?: string;
  /** EVTX のレコードの見出しが記録したレコードの番号の文字列。 */
  recordHeaderId?: string;
  /** XML の EventRecordID の文字列。 */
  eventRecordId?: string;
};

/** `RecordSummary` を検証する。 */
export const decodeRecordSummary: Decoder<RecordSummary> = (input, path) => {
  const source = readObject(input, path);
  return {
    recordRef: requireMember(source, "recordRef", path, decodeRecordLocator),
    eventTime: optionalMember(source, "eventTime", path, decodeTimestamp),
    eventCategory: optionalString(source, "eventCategory", path),
    eventAction: optionalString(source, "eventAction", path),
    windowsEvent: optionalBoolean(source, "windowsEvent", path) ?? false,
    channel: optionalString(source, "channel", path),
    recordHeaderId: optionalString(source, "recordHeaderId", path),
    eventRecordId: optionalString(source, "eventRecordId", path),
  };
};

/** `SubgraphNode` を検証する。 */
export const decodeSubgraphNode: Decoder<SubgraphNode> = (input, path) => {
  const source = readObject(input, path);
  const node = decodeGraphNode(input, path);
  const selection = requireEnum(source, "selection", path, nodeSelections);
  const record = optionalMember(source, "record", path, decodeRecordSummary);
  if (
    record !== undefined &&
    (node.kind !== "record" || selection !== "matched")
  ) {
    throw new DecodeFailure(
      `${path}.record`,
      "expected a record summary on a matched record node only",
    );
  }
  return {
    ...node,
    selection,
    valueMatches: optionalArray(
      source,
      "valueMatches",
      path,
      decodeNodeValueMatch,
    ),
    terminals: optionalArray(source, "terminals", path, decodeGraphNode),
    record,
  };
};

/** 絞り込みに合ったノードの、種別 1 つ分の件数。件数 0 の種別は応答に出ない。 */
export type MatchedKind = {
  kind: NodeKind;
  /** 1 以上。 */
  count: number;
};

/** `MatchedKind` を検証する。 */
export const decodeMatchedKind: Decoder<MatchedKind> = (input, path) => {
  const source = readObject(input, path);
  return {
    kind: requireEnum(source, "kind", path, nodeKinds),
    count: requireCount(source, "count", path),
  };
};

/** 図に描くはずだったエッジの、関係の種別 1 つ分の本数。 */
export type EdgeKindCount = {
  kind: EdgeKind;
  count: number;
};

/** `EdgeKindCount` を検証する。 */
const decodeEdgeKindCount: Decoder<EdgeKindCount> = (input, path) => {
  const source = readObject(input, path);
  return {
    kind: requireEnum(source, "kind", path, edgeKinds),
    count: requireCount(source, "count", path),
  };
};

/** 関係の種別 1 つを検証する。 */
export const decodeEdgeKind: Decoder<EdgeKind> = (element, elementPath) => {
  if (!(edgeKinds as readonly string[]).includes(element as string)) {
    throw new DecodeFailure(elementPath, "expected an edge kind");
  }
  return element as EdgeKind;
};

/** 関係またはノードの根拠のレコード 1 件。原資料の値を持たず、位置だけを持つ。 */
export type GraphEvidence = {
  recordRef: RecordLocator;
  /** 出ない場合は、そのレコードの時刻を読めていない。 */
  eventTime?: Timestamp;
  observationKind: ObservationKind;
  /**
   * レコードの事象の分類と動作の組。根拠のレコードを絞る条件の `eventCategory` と `eventAction` に
   * そのまま渡せる。出ない場合は、そのレコードは分類を持たない (`backend/core/graph_response.go` の
   * `EventKindPair`)。
   */
  eventKind?: EventKindPair;
};

/** `GraphEvidence` を検証する。 */
export const decodeGraphEvidence: Decoder<GraphEvidence> = (input, path) => {
  const source = readObject(input, path);
  return {
    recordRef: requireMember(source, "recordRef", path, decodeRecordLocator),
    eventTime: optionalMember(source, "eventTime", path, decodeTimestamp),
    observationKind: requireMember(
      source,
      "observationKind",
      path,
      decodeObservationKind,
    ),
    eventKind: optionalMember(source, "eventKind", path, decodeEventKindPair),
  };
};

/** グラフのエッジ 1 本。同じ種別と両端の観測を 1 本にまとめ、件数を `evidenceCount` が持つ。 */
export type GraphEdge = {
  id: string;
  kind: EdgeKind;
  state: RelationState;
  sourceNodeId: string;
  targetNodeId: string;
  /**
   * 出ない場合は、絶対時刻として読める根拠を 1 件も持たない。根拠のレコードを持たず、
   * 端末の割当から作ったエッジでは、割当の適用期間である
   * (`backend/core/graph_response.go` の `GraphEdge.ApplicableRange`)。
   */
  applicableRange?: TimeRange;
  /** エッジを作った端末の割当の由来。割当から作っていないエッジでは出ない。 */
  assignmentOrigins?: TerminalAssignmentOrigin[];
  /**
   * 根拠のレコードの件数。
   * **根拠の中身をこの型が持たない。** 中身は操作 11 が返す。
   */
  evidenceCount: number;
  /**
   * `evidenceCount` を案件ごとに分けた件数。案件を区別しない取り込みでは出ない。
   * 定義元は `backend/core/graph_response.go` の `GraphEdge.EvidenceByCase` である。
   */
  evidenceByCase?: CaseEvidenceCount[];
  /**
   * 関連付けがこのエッジの候補を、互いに区別できない候補の組に入れた起点での、組の候補が指す
   * ノードの数の最大。このエッジの終点を含み、2 以上だけが出る。定義元は
   * `backend/core/graph_response.go` の `GraphEdge.IndistinguishableCandidateCount` である。
   */
  indistinguishableCandidateCount?: number;
  /**
   * 関連付けが終点のレコードに挙がった候補を並べた、このエッジの区分。ログオンの連鎖だけが持つ。
   * 応答のエッジの並びは、区分を持たないエッジの後に区分の番号の順で続く。定義元は
   * `backend/core/graph_response.go` の `GraphEdge.CandidateTier` である。
   */
  candidateTier?: EdgeCandidateTier;
};

/** 候補の区分を決める条件。`edgeRecordPairs.ts` の `edgePairConditionKeys` の一部である。 */
export const candidateTierConditionKeys = [
  "session_account_match",
  "session_account_different",
  "session_logon_interactive",
  "session_logon_network",
  "session_logon_other",
] as const;

export type CandidateTierConditionKey =
  (typeof candidateTierConditionKeys)[number];

/**
 * 候補のエッジ 1 本を並べた区分。定義元は `backend/core/graph_response.go` の
 * `EdgeCandidateTier` である。
 */
export type EdgeCandidateTier = {
  /** 区分の番号。1 が並びの最も上の区分である。 */
  tier: number;
  /** 区分を決めた条件。アカウントの条件、ログオンの種別の条件の順である。 */
  conditions: CandidateTierConditionKey[];
};

/** `EdgeCandidateTier` を検証する。 */
const decodeEdgeCandidateTier: Decoder<EdgeCandidateTier> = (input, path) => {
  const source = readObject(input, path);
  const tier = requireCount(source, "tier", path);
  if (tier < 1) {
    throw new DecodeFailure(`${path}.tier`, "expected 1 or more");
  }
  const conditions = requireArray(
    source,
    "conditions",
    path,
    (condition, conditionPath) => {
      const value = decodeString(condition, conditionPath);
      const found = candidateTierConditionKeys.find((key) => key === value);
      if (found === undefined) {
        throw new DecodeFailure(
          conditionPath,
          `expected one of ${candidateTierConditionKeys.join(" / ")}`,
        );
      }
      return found;
    },
  );
  if (conditions.length === 0) {
    throw new DecodeFailure(`${path}.conditions`, "expected 1 or more");
  }
  return { tier, conditions };
};

/** 割当の由来 1 つを検証する。 */
const decodeAssignmentOrigin: Decoder<TerminalAssignmentOrigin> = (
  input,
  path,
) => {
  const value = decodeString(input, path);
  const found = terminalAssignmentOrigins.find((origin) => origin === value);
  if (found === undefined) {
    throw new DecodeFailure(
      path,
      `expected one of ${terminalAssignmentOrigins.join(" / ")}`,
    );
  }
  return found;
};

/** `GraphEdge` を検証する。 */
export const decodeGraphEdge: Decoder<GraphEdge> = (input, path) => {
  const source = readObject(input, path);
  const evidenceCount = requireCount(source, "evidenceCount", path);
  const indistinguishableCandidateCount = optionalCount(
    source,
    "indistinguishableCandidateCount",
    path,
  );
  if (
    indistinguishableCandidateCount !== undefined &&
    indistinguishableCandidateCount < 2
  ) {
    throw new DecodeFailure(
      `${path}.indistinguishableCandidateCount`,
      "expected 2 or more",
    );
  }
  return {
    indistinguishableCandidateCount,
    candidateTier: optionalMember(
      source,
      "candidateTier",
      path,
      decodeEdgeCandidateTier,
    ),
    id: requireString(source, "id", path),
    kind: requireEnum(source, "kind", path, edgeKinds),
    state: requireEnum(source, "state", path, relationStates),
    sourceNodeId: requireString(source, "sourceNodeId", path),
    targetNodeId: requireString(source, "targetNodeId", path),
    applicableRange: optionalMember(
      source,
      "applicableRange",
      path,
      decodeTimeRange,
    ),
    assignmentOrigins: optionalArray(
      source,
      "assignmentOrigins",
      path,
      decodeAssignmentOrigin,
    ),
    evidenceCount,
    evidenceByCase: optionalEvidenceByCase(source, path, evidenceCount),
  };
};

/**
 * 操作 11 が返すエッジ。絞り込みを通った根拠の中身を全件持つ。
 * 操作 9 のエッジは `GraphEdge` であり、根拠の中身を持たない。
 */
export type EdgeDetailEdge = GraphEdge & {
  evidence: GraphEvidence[];
};

/** `EdgeDetailEdge` を検証する。 */
export const decodeEdgeDetailEdge: Decoder<EdgeDetailEdge> = (input, path) => {
  const source = readObject(input, path);
  const evidence = requireArray(source, "evidence", path, decodeGraphEvidence);
  const edge = decodeGraphEdge(input, path);
  requireCountEqualsElements(
    edge.evidenceCount,
    evidence.length,
    `${path}.evidenceCount`,
  );
  return { ...edge, evidence };
};

/**
 * 数える欄に観測した値 1 件と、その件数と時刻の両端。
 * **値は外部由来の文字列であり、表示の境界で無害化する。**
 */
export type ValueCount = {
  value: string;
  /** この値を観測したレコードの件数。 */
  recordCount: number;
  /** 時刻を読める根拠を 1 件も持たない値では出ない。2 つは揃って出る。 */
  firstEventTime?: Timestamp;
  lastEventTime?: Timestamp;
  /** 時点を持つレコードが 2 件未満の値では出ない。 */
  intervals?: EventIntervals;
};

/**
 * `EventIntervals.binCounts` の階級の境界 (ミリ秒)。定義元は `backend/core/graph_response.go` の
 * `IntervalBoundsMilliseconds` である。階級 i は [境界 i-1, 境界 i) であり、最初の階級の下端は 0、
 * 最後の階級は上端を持たない。
 */
export const intervalBoundsMilliseconds = [
  1_000, 5_000, 10_000, 30_000, 60_000, 300_000, 600_000, 1_800_000, 3_600_000,
  21_600_000, 86_400_000,
] as const;

/** 値を観測したレコードの時刻の隣り合う差の分布。定義元は `core.EventIntervals` である。 */
export type EventIntervals = {
  timedRecordCount: number;
  minMilliseconds: number;
  lowerQuartileMilliseconds: number;
  medianMilliseconds: number;
  upperQuartileMilliseconds: number;
  maxMilliseconds: number;
  binCounts: number[];
  coarsePrecision: boolean;
};

const decodeEventIntervals: Decoder<EventIntervals> = (input, path) => {
  const source = readObject(input, path);
  const count = (key: string) => requireCount(source, key, path);
  const intervals: EventIntervals = {
    timedRecordCount: count("timedRecordCount"),
    minMilliseconds: count("minMilliseconds"),
    lowerQuartileMilliseconds: count("lowerQuartileMilliseconds"),
    medianMilliseconds: count("medianMilliseconds"),
    upperQuartileMilliseconds: count("upperQuartileMilliseconds"),
    maxMilliseconds: count("maxMilliseconds"),
    binCounts: requireArray(source, "binCounts", path, (value, at) =>
      requireCount({ value }, "value", at),
    ),
    coarsePrecision: optionalBoolean(source, "coarsePrecision", path) ?? false,
  };
  const quantiles = [
    intervals.minMilliseconds,
    intervals.lowerQuartileMilliseconds,
    intervals.medianMilliseconds,
    intervals.upperQuartileMilliseconds,
    intervals.maxMilliseconds,
  ];
  if (
    quantiles.some((value, at) => at > 0 && value < (quantiles[at - 1] ?? 0))
  ) {
    throw new DecodeFailure(path, "expected ordered quantiles");
  }
  if (intervals.binCounts.length !== intervalBoundsMilliseconds.length + 1) {
    throw new DecodeFailure(`${path}.binCounts`, "expected one count per bin");
  }
  const sum = intervals.binCounts.reduce((total, value) => total + value, 0);
  if (
    intervals.timedRecordCount < 2 ||
    sum !== intervals.timedRecordCount - 1
  ) {
    throw new DecodeFailure(
      `${path}.binCounts`,
      "expected the bins to sum to one less than the timed records",
    );
  }
  return intervals;
};

/** `ValueCount` を検証する。 */
export const decodeValueCount: Decoder<ValueCount> = (input, path) => {
  const source = readObject(input, path);
  const firstEventTime = optionalMember(
    source,
    "firstEventTime",
    path,
    decodeTimestamp,
  );
  const lastEventTime = optionalMember(
    source,
    "lastEventTime",
    path,
    decodeTimestamp,
  );
  // 片方だけを持つ両端を読まない。2 つは同じ根拠の集合から決まる。
  if ((firstEventTime === undefined) !== (lastEventTime === undefined)) {
    throw new DecodeFailure(
      `${path}.firstEventTime`,
      "expected both bounds or neither",
    );
  }
  const recordCount = requireCount(source, "recordCount", path);
  const intervals = optionalMember(
    source,
    "intervals",
    path,
    decodeEventIntervals,
  );
  if (intervals !== undefined && intervals.timedRecordCount > recordCount) {
    throw new DecodeFailure(
      `${path}.intervals.timedRecordCount`,
      "expected at most the record count",
    );
  }
  return {
    value: requireString(source, "value", path),
    recordCount,
    firstEventTime,
    lastEventTime,
    ...(intervals === undefined ? {} : { intervals }),
  };
};

/**
 * 操作 9 の応答のうち、ノードの数の上限に関わる項目。上限を超えたかで項目が分かれる。
 */
export type GraphNodeLimitState =
  | {
      /** 部分グラフのノードの数 (`subgraphNodeCount`) が上限に収まった。 */
      nodeLimitExceeded: false;
      /** 要求が与えたノードの数の上限。要求が与えないときは出ない。 */
      nodeLimit?: number;
    }
  | {
      /** `subgraphNodeCount` が `nodeLimit` を超えた。 */
      nodeLimitExceeded: true;
      /** 要求が与えたノードの数の上限。 */
      nodeLimit: number;
      /**
       * 図に描くはずだったエッジの、関係の種別ごとの本数。種別の昇順に並ぶ。
       * 辿ったエッジが無いときは空である。
       */
      edgeKindCounts: EdgeKindCount[];
    };

/** 操作 9 (`GET /api/v0/graph`) の応答。 */
export type GraphResponse = GraphResponseBody & GraphNodeLimitState;

/** 上限を超えた操作 9 の応答。 */
export type WithheldGraphResponse = GraphResponse & { nodeLimitExceeded: true };

/** 操作 9 の応答のうち、ノードの数の上限に依らない項目。 */
type GraphResponseBody = {
  /** 上限を超えた応答では、絞り込みに合ったノード (`matched`) だけを持つ。 */
  nodes: SubgraphNode[];
  /** 絞り込みに合ったノードの件数。 */
  nodeCount: number;
  /** 上限を超えた応答では空である。 */
  edges: GraphEdge[];
  /** エッジの件数。`edges.length` と等しい。 */
  edgeCount: number;
  /** 図に描くノードの数。絞り込みに合ったノードと、その関係の相手を数える。 */
  subgraphNodeCount: number;
  /** 絞り込みに合うノードの種別ごとの件数。件数の和は `nodeCount` と等しい。 */
  matchedKinds: MatchedKind[];
  /**
   * 絞り込みに合うアカウントのうち、同じアカウントの候補で結ばれた SID と名前のノードの組の数。
   * `matchedKinds` はその組の 2 つのノードを別に数える。組が無いときは出ない。
   */
  matchedAccountIdentityPairCount?: number;
  /** 応答が絞り込みに用いた条件。出ない項目は要求が与えていない。 */
  nodeKinds?: NodeKind[];
  granularity?: GraphGranularity;
  /** 要求が近傍を広げる起点に指したノード。要求に書いた順に並ぶ。 */
  nodeIds?: string[];
  depth: number;
  /** 要求が与えた関係の種別。要求に書いた順に並ぶ。 */
  edgeKinds?: EdgeKind[];
  eventCategory?: string;
  eventAction?: string;
  /** 応答が根拠のレコードを絞った、事象の動作の範囲の両端。 */
  eventActionFrom?: number;
  eventActionTo?: number;
  /** 応答が文字列を一致させた欄。 */
  valueField?: string;
  /** 応答が用いた欄と文字列の組 (`欄=文字列`)。要求に書いた順に並ぶ。 */
  fieldContains?: string[];
  /** 応答が用いた完全一致の欄と文字列の組 (`欄=文字列`)。要求に書いた順に並ぶ。 */
  fieldEquals?: string[];
  /** 応答が根拠のレコードを絞った検索式。要求が与えた文字列のままである。 */
  searchExpression?: string;
  /** 応答が根拠のレコードを絞った案件。 */
  case?: CaseId;
  /** 応答が根拠のレコードを絞った端末のノードの識別子。 */
  terminal?: string;
  /** 応答が絞り込みに使った検索の文字列。要求に書いた順に並ぶ。 */
  valueContains?: string[];
  /** 応答が含まないことを求めた文字列。 */
  valueExcludes?: string[];
  /** 応答が値ごとに数えた欄。出ない場合は数えていない。 */
  countBy?: string;
  /** 数える欄を与えた応答が持つ、値ごとの件数。 */
  valueCounts?: ValueCount[];
  /** 打ち切りの前に数えた値の異なりの個数。数える欄を与えた応答では 0 件でも出る。 */
  distinctValueCount?: number;
  /** 値ごとの件数が 0 件のとき出る。 */
  valueCountsEmptyReason?: ValueCountsEmptyReason;
  /**
   * 数える欄の値を持つノードの種別。`valueCountsEmptyReason` が `field_on_other_node_kind` の
   * ときだけ出る。
   */
  valueCountsNodeKinds?: NodeKind[];
  /** 応答が根拠のレコードを絞った収集元の sourceId。要求に書いた順に並ぶ。 */
  source?: string[];
  /** IP アドレスのノードを残したアドレスの範囲。 */
  addressInCidr?: string;
  /** IP アドレスのノードを外したアドレスの範囲。 */
  addressNotInCidr?: string;
  timeFrom?: RequestedTime;
  timeTo?: RequestedTime;
  filterUnit?: FilterUnit;
  /** `nodeCount` が 0 のとき出る。 */
  emptyReason?: GraphEmptyReason;
};

/** 絞り込みに合ったノードの件数が、応答の `matched` のノードの数と一致することを確かめる。 */
function requireMatchedCount(
  nodeCount: number,
  nodes: SubgraphNode[],
  path: string,
): number {
  const matched = nodes.filter((node) => node.selection === "matched").length;
  if (nodeCount !== matched) {
    throw new DecodeFailure(
      `${path}.nodeCount`,
      `expected ${matched} to equal the matched nodes`,
    );
  }
  return nodeCount;
}

/**
 * エッジの両端が `nodes` にあることを確かめる。
 * 両端は必ず `nodes` に入る。上限の外にある端点も `edge_endpoint` として応答に入る
 * (`backend/core/graph_response.go` の `NodeSelectionEdgeEndpoint`)。画面は 2 つの集合を
 * 繋ぐ処理を持たず、繋げない応答を読むと端点を持たないエッジを描くことになる。
 */
function requireEndpointsInNodes(
  edges: GraphEdge[],
  nodes: SubgraphNode[],
  path: string,
): GraphEdge[] {
  const nodeIds = new Set(nodes.map((node) => node.id));
  edges.forEach((edge, index) => {
    for (const endpoint of ["sourceNodeId", "targetNodeId"] as const) {
      if (!nodeIds.has(edge[endpoint])) {
        throw new DecodeFailure(
          `${path}[${index}].${endpoint}`,
          "expected an id the nodes of the same response carry",
        );
      }
    }
  });
  return edges;
}

/**
 * 集合の要素の `id` が 1 つずつ別の値であることを確かめる。
 * `id` はノードとエッジを指す識別子である
 * (`backend/core/graph_response.go` の `GraphNode.Id` と `GraphEdge.Id`)。
 * 同じ値を 2 件が名乗る応答は、
 * 画面がどちらの要素を指しているかを決められない。
 */
function requireDistinctIds<Element extends { id: string }>(
  elements: Element[],
  path: string,
): Element[] {
  const seen = new Set<string>();
  elements.forEach((element, index) => {
    if (seen.has(element.id)) {
      throw new DecodeFailure(
        `${path}[${index}].id`,
        "expected an id no other element of the same set carries",
      );
    }
    seen.add(element.id);
  });
  return elements;
}

/** 操作 9 の応答を検証する。 */
export const decodeGraphResponse: Decoder<GraphResponse> = (input, path) => {
  const source = readObject(input, path);
  const nodes = requireDistinctIds(
    requireArray(source, "nodes", path, decodeSubgraphNode),
    `${path}.nodes`,
  );
  const nodeCount = requireCount(source, "nodeCount", path);
  const edges = requireEndpointsInNodes(
    requireDistinctIds(
      requireArray(source, "edges", path, decodeGraphEdge),
      `${path}.edges`,
    ),
    nodes,
    `${path}.edges`,
  );

  const countBy = optionalString(source, "countBy", path);
  let valueCounts: ValueCount[] | undefined;
  let distinctValueCount: number | undefined;
  let valueCountsEmptyReason: ValueCountsEmptyReason | undefined;
  let valueCountsNodeKinds: NodeKind[] | undefined;
  if (countBy === undefined) {
    rejectMember(
      source,
      "distinctValueCount",
      path,
      "expected no distinct value count while the response counts no field",
    );
    rejectMember(
      source,
      "valueCounts",
      path,
      "expected no value counts while the response counts no field",
    );
    rejectMember(
      source,
      "valueCountsEmptyReason",
      path,
      "expected no reason while the response counts no field",
    );
  } else {
    // 値が 1 件も無い応答は valueCounts を出さない。要素数 0 の集合として読む。
    valueCounts =
      optionalArray(source, "valueCounts", path, decodeValueCount) ?? [];
    // 0 件でも出る。数えていない状態と 0 件を分ける (`backend/api/graph.go`)。
    distinctValueCount = requireCount(source, "distinctValueCount", path);
    // **数えた個数と、この応答に入れた個数が一致する。** 応答は値の異なりを全件返す。
    if (distinctValueCount !== valueCounts.length) {
      throw new DecodeFailure(
        `${path}.distinctValueCount`,
        `expected ${valueCounts.length} to equal the returned values`,
      );
    }
    if (distinctValueCount === 0) {
      valueCountsEmptyReason = requireEnum(
        source,
        "valueCountsEmptyReason",
        path,
        valueCountsEmptyReasons,
      );
      if (valueCountsEmptyReason === "field_on_other_node_kind") {
        valueCountsNodeKinds = requireArray(
          source,
          "valueCountsNodeKinds",
          path,
          decodeNodeKind,
        );
      }
    } else {
      rejectMember(
        source,
        "valueCountsEmptyReason",
        path,
        "expected no reason while the response counts 1 or more values",
      );
    }
  }
  // 値を持つ種別は field_on_other_node_kind の理由だけが持ち、1 つ以上を持つ。
  if (valueCountsNodeKinds === undefined) {
    rejectMember(
      source,
      "valueCountsNodeKinds",
      path,
      "expected no node kinds without the field_on_other_node_kind reason",
    );
  } else if (valueCountsNodeKinds.length === 0) {
    throw new DecodeFailure(
      `${path}.valueCountsNodeKinds`,
      "expected one or more node kinds",
    );
  }

  const timeFrom = optionalMember(
    source,
    "timeFrom",
    path,
    decodeRequestedTime,
  );
  const timeTo = optionalMember(source, "timeTo", path, decodeRequestedTime);
  let filterUnit: FilterUnit | undefined;
  if (timeFrom === undefined && timeTo === undefined) {
    rejectMember(
      source,
      "filterUnit",
      path,
      "expected no filterUnit while the response carries no requested time",
    );
  } else {
    filterUnit = requireEnum(source, "filterUnit", path, filterUnits);
  }

  let emptyReason: GraphEmptyReason | undefined;
  if (nodeCount === 0) {
    emptyReason = requireEnum(source, "emptyReason", path, graphEmptyReasons);
  } else {
    rejectMember(
      source,
      "emptyReason",
      path,
      "expected no reason while nodeCount is 1 or more",
    );
  }

  const matchedKinds = requireArray(
    source,
    "matchedKinds",
    path,
    decodeMatchedKind,
  );
  if (
    matchedKinds.reduce((total, kind) => total + kind.count, 0) !== nodeCount
  ) {
    throw new DecodeFailure(
      `${path}.matchedKinds`,
      "expected the counts to sum to nodeCount",
    );
  }
  const subgraphNodeCount = requireCount(source, "subgraphNodeCount", path);
  const nodeLimit = optionalCount(source, "nodeLimit", path);
  const nodeLimitExceeded =
    optionalBoolean(source, "nodeLimitExceeded", path) ?? false;
  let limitState: GraphNodeLimitState = { nodeLimitExceeded: false, nodeLimit };
  if (nodeLimitExceeded) {
    // 上限を超えた応答は、合ったノードだけを持ち、エッジを持たない。
    if (nodeLimit === undefined || subgraphNodeCount <= nodeLimit) {
      throw new DecodeFailure(
        `${path}.nodeLimitExceeded`,
        "expected a node limit the subgraph node count exceeds",
      );
    }
    if (edges.length !== 0) {
      throw new DecodeFailure(
        `${path}.edges`,
        "expected no edge while the node limit is exceeded",
      );
    }
    limitState = {
      nodeLimitExceeded: true,
      nodeLimit,
      edgeKindCounts: requireArray(
        source,
        "edgeKindCounts",
        path,
        decodeEdgeKindCount,
      ),
    };
  } else {
    rejectMember(
      source,
      "edgeKindCounts",
      path,
      "expected no edge kind counts within the node limit",
    );
  }
  return {
    nodes,
    nodeCount: requireMatchedCount(nodeCount, nodes, path),
    subgraphNodeCount,
    ...limitState,
    matchedKinds,
    matchedAccountIdentityPairCount: optionalCount(
      source,
      "matchedAccountIdentityPairCount",
      path,
    ),
    edges,
    edgeCount: requireCountEqualsElements(
      requireCount(source, "edgeCount", path),
      edges.length,
      `${path}.edgeCount`,
    ),
    nodeKinds: optionalArray(source, "nodeKinds", path, decodeNodeKind),
    granularity: optionalEnum(source, "granularity", path, graphGranularities),
    nodeIds: optionalArray(source, "nodeIds", path, decodeString),
    depth: requireCount(source, "depth", path),
    edgeKinds: optionalArray(source, "edgeKinds", path, decodeEdgeKind),
    eventCategory: optionalString(source, "eventCategory", path),
    eventAction: optionalString(source, "eventAction", path),
    eventActionFrom: optionalCount(source, "eventActionFrom", path),
    eventActionTo: optionalCount(source, "eventActionTo", path),
    valueField: optionalString(source, "valueField", path),
    fieldContains: optionalArray(source, "fieldContains", path, decodeString),
    fieldEquals: optionalArray(source, "fieldEquals", path, decodeString),
    searchExpression: optionalString(source, "searchExpression", path),
    case: optionalMember(source, "case", path, decodeCaseId),
    terminal: optionalString(source, "terminal", path),
    valueContains: optionalArray(source, "valueContains", path, decodeString),
    valueExcludes: optionalArray(source, "valueExcludes", path, decodeString),
    countBy,
    valueCounts,
    distinctValueCount,
    valueCountsEmptyReason,
    valueCountsNodeKinds,
    source: optionalArray(source, "source", path, decodeString),
    addressInCidr: optionalString(source, "addressInCidr", path),
    addressNotInCidr: optionalString(source, "addressNotInCidr", path),
    timeFrom,
    timeTo,
    filterUnit,
    emptyReason,
  };
};
