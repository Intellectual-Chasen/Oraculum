import { decodeRecordLocator, type RecordLocator } from "./common";
import {
  DecodeFailure,
  type Decoder,
  decodeString,
  optionalArray,
  optionalString,
  readObject,
  requireArray,
  requireCount,
  requireEnum,
  requireMember,
  requireString,
} from "./decoding";
import {
  decodeGraphNode,
  type EdgeKind,
  edgeKinds,
  type GraphNode,
} from "./graph";
import {
  decodeEdgeAssignmentBasis,
  type EdgeAssignmentBasis,
} from "./graphDetail";
import {
  decodeTerminalAssignment,
  type TerminalAssignment,
} from "./terminalAssignments";

/** `backend/api/attack_candidates.go` のrule summary。 */
export type AttackCandidateRule = {
  id: string;
  title: string;
  description: string;
  references: string[];
  attack: AttackReference[];
  distinguishesFrom: string[];
  variants: AttackCandidateVariant[];
  matchCount: number;
};

export type AttackReference = { id: string; basis: "official" | "inferred" };
export type AttackCandidateVariant = {
  id: string;
  platforms: string[];
  rationale: string;
  evaluation: {
    state: "evaluated" | "not_evaluated";
    reason?: string;
    detail?: string;
  };
  matchCount: number;
};

/** 有向graph関係に一致したruleと根拠。 */
export type AttackCandidateMatch = {
  ruleId: string;
  variantId: string;
  matchId: string;
  edges: AttackCandidateMatchedEdge[];
};

/** ATT&CK候補を構成する役割付きedgeと各edgeの根拠。 */
export type AttackCandidateMatchedEdge = {
  role: string;
  evidenceRole: "required" | "supporting";
  kind: EdgeKind;
  edgeId: string;
  sourceNode: GraphNode;
  sinkNode: GraphNode;
  assignmentBases: EdgeAssignmentBasis[];
  /** エッジを作った端末の割当。利用者が収集元に付けた割当の IP から作ったエッジだけが持つ。 */
  terminalAssignments?: TerminalAssignment[];
  evidence: RecordLocator[];
};

/** 要求ごとに計算した候補の応答。 */
export type AttackCandidatesResponse = {
  ruleSet: {
    directory: string;
    fileCount: number;
    contentSha256: string;
    revision: string;
    revisionSource: string;
  };
  rules: AttackCandidateRule[];
  matches: AttackCandidateMatch[];
  notEvaluated: Array<{
    ruleId: string;
    variantId: string;
    reason: string;
    detail: string;
  }>;
};

const decodeAttackReference: Decoder<AttackReference> = (input, path) => {
  const source = readObject(input, path);
  return {
    id: requireString(source, "id", path),
    basis: requireEnum(source, "basis", path, ["official", "inferred"]),
  };
};

const decodeEvaluation: Decoder<AttackCandidateVariant["evaluation"]> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    state: requireEnum(source, "state", path, ["evaluated", "not_evaluated"]),
    reason: optionalString(source, "reason", path),
    detail: optionalString(source, "detail", path),
  };
};

const decodeVariant: Decoder<AttackCandidateVariant> = (input, path) => {
  const source = readObject(input, path);
  return {
    id: requireString(source, "id", path),
    platforms: requireArray(source, "platforms", path, decodeString),
    rationale: requireString(source, "rationale", path),
    evaluation: requireMember(source, "evaluation", path, decodeEvaluation),
    matchCount: requireCount(source, "matchCount", path),
  };
};

const decodeRule: Decoder<AttackCandidateRule> = (input, path) => {
  const source = readObject(input, path);
  return {
    id: requireString(source, "id", path),
    title: requireString(source, "title", path),
    description: requireString(source, "description", path),
    references: requireArray(source, "references", path, decodeString),
    attack: requireArray(source, "attack", path, decodeAttackReference),
    distinguishesFrom: requireArray(
      source,
      "distinguishesFrom",
      path,
      decodeString,
    ),
    variants: requireArray(source, "variants", path, decodeVariant),
    matchCount: requireCount(source, "matchCount", path),
  };
};

const decodeMatchedEdge: Decoder<AttackCandidateMatchedEdge> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    role: requireString(source, "role", path),
    evidenceRole: requireEnum(source, "evidenceRole", path, [
      "required",
      "supporting",
    ]),
    kind: requireEnum(source, "kind", path, edgeKinds),
    edgeId: requireString(source, "edgeId", path),
    sourceNode: requireMember(source, "sourceNode", path, decodeGraphNode),
    sinkNode: requireMember(source, "sinkNode", path, decodeGraphNode),
    assignmentBases: requireArray(
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
    evidence: requireArray(source, "evidence", path, decodeRecordLocator),
  };
};

function requireMatchedEdges(
  source: Record<string, unknown>,
  path: string,
): AttackCandidateMatchedEdge[] {
  const edges = requireArray(source, "edges", path, decodeMatchedEdge);
  if (edges.length === 0) {
    throw new DecodeFailure(`${path}.edges`, "expected 1 or more elements");
  }
  const edgeIds = new Set<string>();
  const roles = new Set<string>();
  for (const [index, edge] of edges.entries()) {
    if (edgeIds.has(edge.edgeId)) {
      throw new DecodeFailure(
        `${path}.edges[${index}].edgeId`,
        "expected unique edge IDs within a match",
      );
    }
    edgeIds.add(edge.edgeId);
    if (roles.has(edge.role)) {
      throw new DecodeFailure(
        `${path}.edges[${index}].role`,
        "expected unique roles within a match",
      );
    }
    roles.add(edge.role);
  }
  return edges;
}

const decodeMatch: Decoder<AttackCandidateMatch> = (input, path) => {
  const source = readObject(input, path);
  return {
    ruleId: requireString(source, "ruleId", path),
    variantId: requireString(source, "variantId", path),
    matchId: requireString(source, "matchId", path),
    edges: requireMatchedEdges(source, path),
  };
};

/** ATT&CK候補APIの応答を検証して画面用の値へ変換する。 */
export const decodeAttackCandidatesResponse: Decoder<
  AttackCandidatesResponse
> = (input, path) => {
  const source = readObject(input, path);
  const ruleSetSource = requireMember(source, "ruleSet", path, readObject);
  const ruleSet = {
    directory: requireString(ruleSetSource, "directory", `${path}.ruleSet`),
    fileCount: requireCount(ruleSetSource, "fileCount", `${path}.ruleSet`),
    contentSha256: requireString(
      ruleSetSource,
      "contentSha256",
      `${path}.ruleSet`,
    ),
    revision: requireString(ruleSetSource, "revision", `${path}.ruleSet`),
    revisionSource: requireString(
      ruleSetSource,
      "revisionSource",
      `${path}.ruleSet`,
    ),
  };
  const rules = requireArray(source, "rules", path, decodeRule);
  const matches = requireArray(source, "matches", path, decodeMatch);
  const notEvaluated = requireArray(
    source,
    "notEvaluated",
    path,
    (input, itemPath) => {
      const item = readObject(input, itemPath);
      return {
        ruleId: requireString(item, "ruleId", itemPath),
        variantId: requireString(item, "variantId", itemPath),
        reason: requireString(item, "reason", itemPath),
        detail: requireString(item, "detail", itemPath),
      };
    },
  );
  const counts = new Map<string, number>();
  const pairs = new Set<string>();
  for (const [index, match] of matches.entries()) {
    const pair = JSON.stringify([match.ruleId, match.matchId]);
    if (pairs.has(pair))
      throw new DecodeFailure(
        `${path}.matches[${index}].matchId`,
        "expected unique rule and match IDs",
      );
    pairs.add(pair);
    const key = JSON.stringify([match.ruleId, match.variantId]);
    counts.set(key, (counts.get(key) ?? 0) + 1);
  }
  const ruleIds = new Set<string>();
  const variantStates = new Map<string, AttackCandidateVariant["evaluation"]>();
  for (const [index, rule] of rules.entries()) {
    if (ruleIds.has(rule.id))
      throw new DecodeFailure(
        `${path}.rules[${index}].id`,
        "expected unique rule ID",
      );
    ruleIds.add(rule.id);
    const variantIds = new Set<string>();
    for (const [variantIndex, variant] of rule.variants.entries()) {
      if (variantIds.has(variant.id))
        throw new DecodeFailure(
          `${path}.rules[${index}].variants[${variantIndex}].id`,
          "expected unique variant ID within a rule",
        );
      variantIds.add(variant.id);
      const key = JSON.stringify([rule.id, variant.id]);
      variantStates.set(key, variant.evaluation);
      if (variant.matchCount !== (counts.get(key) ?? 0))
        throw new DecodeFailure(
          `${path}.rules[${index}].variants[${variantIndex}].matchCount`,
          "expected count of matches for variant",
        );
      if (
        variant.evaluation.state === "not_evaluated" &&
        variant.matchCount !== 0
      )
        throw new DecodeFailure(
          `${path}.rules[${index}].variants[${variantIndex}].matchCount`,
          "expected zero matches for a not-evaluated variant",
        );
    }
    if (
      rule.matchCount !==
      rule.variants.reduce((sum, variant) => sum + variant.matchCount, 0)
    ) {
      throw new DecodeFailure(
        `${path}.rules[${index}].matchCount`,
        "expected count of matches for rule",
      );
    }
  }
  for (const [index, match] of matches.entries()) {
    const rule = rules.find((candidate) => candidate.id === match.ruleId);
    if (
      !ruleIds.has(match.ruleId) ||
      rule?.variants.every((variant) => variant.id !== match.variantId)
    )
      throw new DecodeFailure(
        `${path}.matches[${index}].ruleId`,
        "expected a listed rule and variant",
      );
  }
  const skipped = new Set<string>();
  for (const [index, item] of notEvaluated.entries()) {
    const key = JSON.stringify([item.ruleId, item.variantId]);
    const state = variantStates.get(key);
    if (
      state?.state !== "not_evaluated" ||
      state.reason !== item.reason ||
      skipped.has(key)
    )
      throw new DecodeFailure(
        `${path}.notEvaluated[${index}]`,
        "expected one matching not-evaluated variant",
      );
    skipped.add(key);
  }
  for (const [key, state] of variantStates) {
    if ((state.state === "not_evaluated") !== skipped.has(key))
      throw new DecodeFailure(
        `${path}.notEvaluated`,
        "expected each skipped variant exactly once",
      );
  }
  return { ruleSet, rules, matches, notEvaluated };
};
