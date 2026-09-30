import {
  DecodeFailure,
  type Decoder,
  optionalArray,
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
import {
  decodeCandidateTally,
  type EdgeCandidateTally,
  type EdgePairConditionKey,
  edgePairConditionKeys,
} from "./edgeRecordPairs";
import {
  decodeGraphEvidence,
  decodeGraphNode,
  type EdgeKind,
  edgeKinds,
  type GraphEvidence,
  type GraphNode,
  type NodeKind,
  nodeKinds,
} from "./graph";

/**
 * 影響のエッジの根拠の種類 (`backend/core/influence_path.go` の `InfluenceBasis`)。
 * **値の間に順序を付けない。** 並びは Go の定数の並びを写しただけである。
 */
export const influenceBases = [
  "specified_operation",
  "inferred_record",
  "undetermined_direction",
  "account_management",
  "argument_name",
  "requested_destination",
  "credential_use",
  "observed",
  "candidate",
  "uncertain_chain",
  "single_node",
  "equivalence",
] as const;
export type InfluenceBasis = (typeof influenceBases)[number];

/** 時刻の条件を満たした方法の種類 (`InfluenceTimeBasis`)。値の間に順序を付けない。 */
export const influenceTimeBases = [
  "same_terminal",
  "unbounded_offset",
  "precision_width",
] as const;
export type InfluenceTimeBasis = (typeof influenceTimeBases)[number];

/** 経路の集合が空であるか、求めきれなかった理由 (`InfluencePathStop`)。 */
export const influencePathStops = [
  "origin_without_timestamp",
  "destination_without_timestamp",
  "no_influence_route",
  "time_order_unsatisfied",
  "computation_limit",
  "origin_not_influence_end",
  "destination_not_influence_end",
  "truncated_route_unverified",
  "route_through_excluded_basis",
  "excluded_basis_route_limit",
] as const;
export type InfluencePathStop = (typeof influencePathStops)[number];

/** 先へ影響が進まない理由 (`InfluenceFrontierReason`)。 */
export const influenceFrontierReasons = [
  "no_outgoing_influence",
  "outgoing_time_unsatisfied",
  "outgoing_excluded",
] as const;
export type InfluenceFrontierReason = (typeof influenceFrontierReasons)[number];

/** 影響の経路のノード 1 つ (`InfluenceVertex`)。 */
export type InfluenceVertex = {
  key: string;
  node: GraphNode;
  /** 内容のバージョンが始まった時刻 (RFC 3339)。後のバージョンだけが持つ。 */
  versionStart?: string;
  influenceEdgeCount: number;
  /** 2 件以上の候補のプロセスを持つレコードに、1 つの候補から入った要素の、その候補のプロセス。 */
  enteredFrom?: GraphNode;
};

/** 経路に乗る影響のエッジ 1 本 (`InfluenceEdge`)。 */
export type InfluenceEdge = {
  id: string;
  sourceKey: string;
  targetKey: string;
  graphEdgeId: string;
  graphEdgeKind: EdgeKind;
  bases: InfluenceBasis[];
  timeBases: InfluenceTimeBasis[];
  evidence: GraphEvidence[];
  /** 不確定の連鎖のエッジだけが持つ、同じ先の要素に入る不確定の連鎖の関係の数。 */
  candidateCount?: number;
  /** ログオンの連鎖のエッジだけが持つ、同じ終点のログオンに挙がった候補の並びの件数。 */
  candidateTally?: EdgeCandidateTally;
  /** ログオンの連鎖のエッジだけが持つ、この候補の区分を決めた条件 (アカウントの一致とログオンの種別)。 */
  candidateOrder?: EdgePairConditionKey[];
};

/** 経路の起点または終点 (`InfluenceEndpoint`)。 */
export type InfluenceEndpoint = { key: string; record: GraphEvidence };

/**
 * 先へ影響が進まないノード 1 つと理由 (`InfluenceFrontier`)。このノードは経路のエッジの端でなく、
 * 応答の vertices に入らない。ノードの項目をこの型が持つ。
 */
export type InfluenceFrontier = InfluenceVertex & {
  reason: InfluenceFrontierReason;
};

/** 影響の経路の応答 (`backend/api/influence_path.go` の `influencePathResponse`)。 */
export type InfluencePathResponse = {
  from: string;
  to: string;
  excludedBases: InfluenceBasis[];
  origin?: InfluenceEndpoint;
  destination?: InfluenceEndpoint;
  vertices: InfluenceVertex[];
  edges: InfluenceEdge[];
  /** 上限を超えて載せなかった影響のエッジの本数。 */
  omittedEdgeCount: number;
  stops: InfluencePathStop[];
  /** 除いた根拠の種類を戻して求めた経路のエッジが持つ、除いた根拠の種類。 */
  routeExcludedBases: InfluenceBasis[];
  /** 起点または終点が経路の端にならないときの、その端のノードの種別。 */
  originNodeKind?: NodeKind;
  destinationNodeKind?: NodeKind;
  frontier: InfluenceFrontier[];
  frontierCount: number;
  untimedRecordCount: number;
};

function enumElement<T extends string>(values: readonly T[]): Decoder<T> {
  return (input, path) => {
    const found = values.find((value) => value === input);
    if (found === undefined) {
      throw new DecodeFailure(path, `expected one of ${values.join(" / ")}`);
    }
    return found;
  };
}

const decodeBasis = enumElement(influenceBases);

/** 空でない必須の文字列を読む。Go の `requirePresent` と同じく、空の文字列を拒む。 */
function requirePresentString(
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

const decodeVertex: Decoder<InfluenceVertex> = (input, path) => {
  const source = readObject(input, path);
  return {
    key: requirePresentString(source, "key", path),
    node: requireMember(source, "node", path, decodeGraphNode),
    versionStart: optionalString(source, "versionStart", path),
    influenceEdgeCount: requireCount(source, "influenceEdgeCount", path),
    enteredFrom: optionalMember(source, "enteredFrom", path, decodeGraphNode),
  };
};

/**
 * 影響のエッジを読む。**根拠のレコードを 1 件以上持つ。** 根拠の無い影響のエッジを経路として
 * 出さない (Go の `InfluenceEdge.Validate` と同じ条件)。
 */
const decodeEdge: Decoder<InfluenceEdge> = (input, path) => {
  const source = readObject(input, path);
  const evidence = requireArray(source, "evidence", path, decodeGraphEvidence);
  if (evidence.length === 0) {
    throw new DecodeFailure(`${path}.evidence`, "expected at least one record");
  }
  return {
    id: requirePresentString(source, "id", path),
    sourceKey: requirePresentString(source, "sourceKey", path),
    targetKey: requirePresentString(source, "targetKey", path),
    graphEdgeId: requirePresentString(source, "graphEdgeId", path),
    graphEdgeKind: requireEnum(source, "graphEdgeKind", path, edgeKinds),
    bases: requireArray(source, "bases", path, decodeBasis),
    timeBases: requireArray(
      source,
      "timeBases",
      path,
      enumElement(influenceTimeBases),
    ),
    evidence,
    candidateCount: optionalCount(source, "candidateCount", path),
    candidateTally: optionalMember(
      source,
      "candidateTally",
      path,
      decodeCandidateTally,
    ),
    candidateOrder: optionalArray(
      source,
      "candidateOrder",
      path,
      enumElement(edgePairConditionKeys),
    ),
  };
};

const decodeEndpoint: Decoder<InfluenceEndpoint> = (input, path) => {
  const source = readObject(input, path);
  return {
    key: requirePresentString(source, "key", path),
    record: requireMember(source, "record", path, decodeGraphEvidence),
  };
};

const decodeFrontier: Decoder<InfluenceFrontier> = (input, path) => ({
  ...decodeVertex(input, path),
  reason: requireEnum(
    readObject(input, path),
    "reason",
    path,
    influenceFrontierReasons,
  ),
});

/**
 * 影響の経路の応答を検証する。エッジの両端と起点・終点は vertices の key を指す。
 * frontier は frontierCount 件の先頭であり、frontierCount を超えない。
 */
export const decodeInfluencePathResponse: Decoder<InfluencePathResponse> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const response: InfluencePathResponse = {
    from: requireString(source, "from", path),
    to: requireString(source, "to", path),
    excludedBases: requireArray(source, "excludedBases", path, decodeBasis),
    origin: optionalMember(source, "origin", path, decodeEndpoint),
    destination: optionalMember(source, "destination", path, decodeEndpoint),
    vertices: requireArray(source, "vertices", path, decodeVertex),
    edges: requireArray(source, "edges", path, decodeEdge),
    omittedEdgeCount: requireCount(source, "omittedEdgeCount", path),
    stops: requireArray(source, "stops", path, enumElement(influencePathStops)),
    routeExcludedBases: requireArray(
      source,
      "routeExcludedBases",
      path,
      decodeBasis,
    ),
    originNodeKind: optionalEnum(source, "originNodeKind", path, nodeKinds),
    destinationNodeKind: optionalEnum(
      source,
      "destinationNodeKind",
      path,
      nodeKinds,
    ),
    frontier: requireArray(source, "frontier", path, decodeFrontier),
    frontierCount: requireCount(source, "frontierCount", path),
    untimedRecordCount: requireCount(source, "untimedRecordCount", path),
  };
  const keys = new Set(response.vertices.map((vertex) => vertex.key));
  for (const [index, edge] of response.edges.entries()) {
    if (!keys.has(edge.sourceKey) || !keys.has(edge.targetKey)) {
      throw new DecodeFailure(
        `${path}.edges[${index}]`,
        "expected endpoints among vertices",
      );
    }
  }
  for (const [name, endpoint] of [
    ["origin", response.origin],
    ["destination", response.destination],
  ] as const) {
    if (endpoint !== undefined && !keys.has(endpoint.key)) {
      throw new DecodeFailure(
        `${path}.${name}.key`,
        "expected a key among vertices",
      );
    }
  }
  if (response.frontier.length > response.frontierCount) {
    throw new DecodeFailure(
      `${path}.frontier`,
      "expected at most frontierCount items",
    );
  }
  return response;
};
