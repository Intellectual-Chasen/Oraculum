import {
  type AssertionItem,
  type AssertionRecordRef,
  type AssertionTarget,
  type AssertionTargetOrigin,
  assertionTargetOrigins,
  decodeAssertionItem,
  decodeAssertionRecordRef,
  decodeAssertionTarget,
} from "./assertions";
import { type ConditionKey, conditionKeys } from "./candidates";
import { requireCountEqualsElements } from "./common";
import {
  DecodeFailure,
  type Decoder,
  optionalBoolean,
  optionalMember,
  optionalString,
  readObject,
  requireArray,
  requireBoolean,
  requireCount,
  requireEnum,
  requireMember,
  requireString,
} from "./decoding";

/** AI 提案の採否の状態。定義元は `backend/core/assist_proposal.go` の `AssistProposalState` である。 */
export const assistProposalStates = [
  "proposed",
  "adopted",
  "rejected",
] as const;
export type AssistProposalState = (typeof assistProposalStates)[number];

/** LLM の提供者。定義元は `backend/core/assist_common.go` の `AssistProvider` である。 */
export const assistProviders = ["claude"] as const;
export type AssistProvider = (typeof assistProviders)[number];

/** 提案を作った発言の関連付けの条件 1 件と、その幅。 */
export type AssistMatchCondition = {
  conditionKey: ConditionKey;
  tolerance: number;
};

const decodeAssistMatchCondition: Decoder<AssistMatchCondition> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    conditionKey: requireEnum(source, "conditionKey", path, conditionKeys),
    tolerance: requireCount(source, "tolerance", path),
  };
};

/** 分析者が AI 提案の採否を決めた記録。 */
export type AssistProposalDecision = {
  analyst: string;
  decidedAt: string;
  /** 却下の理由。分析者が書いた字句。却下で任意、採用では出ない。 */
  reason?: string;
  /** 採用で作った所見の識別子。採用で必ず出る。 */
  assertionId?: string;
};

const decodeAssistProposalDecision: Decoder<AssistProposalDecision> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    analyst: requireString(source, "analyst", path),
    decidedAt: requireString(source, "decidedAt", path),
    reason: optionalString(source, "reason", path),
    assertionId: optionalString(source, "assertionId", path),
  };
};

/**
 * LLM が会話の中で挙げた所見の候補 1 件。
 * **分析者の所見 (`Assertion`) と別の型である。** 採用した時点で、分析者を著者とする所見が作られる。
 */
export type AssistProposal = {
  id: string;
  /** ノード、関係、レコードのいずれか。 */
  target: AssertionTarget;
  /** LLM が書いた記述。 */
  note: string;
  /** 根拠に挙げたレコード。1 件以上。 */
  recordRefs: AssertionRecordRef[];
  conversationId: string;
  turnId: string;
  provider: AssistProvider;
  /** 会話の model。中継が申告しなかった会話では出ない。 */
  model?: string;
  /** グラフに無い関係を足す提案であるか。採用するまでグラフに関係を足さない。 */
  addsRelation: boolean;
  /** 提案を作った発言の関連付けの条件の選択。 */
  matchConditions: AssistMatchCondition[];
  state: AssistProposalState;
  createdAt: string;
  /** 採否の記録。提案中の提案では出ない。 */
  decision?: AssistProposalDecision;
};

/** `AssistProposal` を検証する。状態と採否の記録の対応を確かめる。 */
export const decodeAssistProposal: Decoder<AssistProposal> = (input, path) => {
  const source = readObject(input, path);
  const proposal: AssistProposal = {
    id: requireString(source, "id", path),
    target: requireMember(source, "target", path, decodeAssertionTarget),
    note: requireString(source, "note", path),
    recordRefs: requireArray(
      source,
      "recordRefs",
      path,
      decodeAssertionRecordRef,
    ),
    conversationId: requireString(source, "conversationId", path),
    turnId: requireString(source, "turnId", path),
    provider: requireEnum(source, "provider", path, assistProviders),
    model: optionalString(source, "model", path),
    addsRelation: requireBoolean(source, "addsRelation", path),
    matchConditions: requireArray(
      source,
      "matchConditions",
      path,
      decodeAssistMatchCondition,
    ),
    state: requireEnum(source, "state", path, assistProposalStates),
    createdAt: requireString(source, "createdAt", path),
    decision: optionalMember(
      source,
      "decision",
      path,
      decodeAssistProposalDecision,
    ),
  };
  checkTarget(proposal, path);
  if (proposal.note.trim() === "") {
    throw new DecodeFailure(`${path}.note`, "expected a note");
  }
  if (proposal.recordRefs.length === 0) {
    throw new DecodeFailure(`${path}.recordRefs`, "expected one or more");
  }
  checkDecision(proposal, path);
  return proposal;
};

/**
 * 対象がノード・関係・レコードのいずれかで、種類が要する参照を持ち、関係を足す提案が関係を指すことを
 * 確かめる (`backend/core/assist_proposal.go` の `validateTarget`)。
 */
function checkTarget(proposal: AssistProposal, path: string): void {
  const { target } = proposal;
  const present =
    (target.kind === "node" && target.nodeId !== undefined) ||
    (target.kind === "edge" && target.edge !== undefined) ||
    (target.kind === "record" && target.record !== undefined);
  if (!present) {
    throw new DecodeFailure(
      `${path}.target`,
      "expected a node, an edge or a record with its reference",
    );
  }
  if (proposal.addsRelation && target.kind !== "edge") {
    throw new DecodeFailure(
      `${path}.addsRelation`,
      "a proposal adds a relation on an edge alone",
    );
  }
}

/** 状態と採否の記録の対応を確かめる (`backend/core/assist_proposal.go` の `validateDecision`)。 */
function checkDecision(proposal: AssistProposal, path: string): void {
  const { decision } = proposal;
  if ((proposal.state === "proposed") !== (decision === undefined)) {
    throw new DecodeFailure(
      `${path}.decision`,
      "a decided proposal alone carries the decision",
    );
  }
  if (proposal.state === "adopted") {
    if (decision?.assertionId === undefined) {
      throw new DecodeFailure(
        `${path}.decision.assertionId`,
        "required on an adopted proposal",
      );
    }
    if (decision.reason !== undefined) {
      throw new DecodeFailure(
        `${path}.decision.reason`,
        "an adoption carries no reason",
      );
    }
  }
  if (proposal.state === "rejected" && decision?.assertionId !== undefined) {
    throw new DecodeFailure(
      `${path}.decision.assertionId`,
      "a rejection names no assertion",
    );
  }
}

/** AI 提案 1 件と、その対象が現在のグラフの何から出たか。 */
export type AssistProposalItem = {
  proposal: AssistProposal;
  /**
   * 対象の出所。値の意味は所見の対象の出所と同じであり、グラフが対象を持たない提案は `absent` に
   * なる。関係を足す提案の関係は、採用するまでグラフに無い。
   */
  targetOrigin: AssertionTargetOrigin;
  /** 関係を足す提案の関係の両端のノードが、現在のグラフにあるか。関係を足す提案だけが持つ。 */
  endpointsInGraph?: boolean;
};

/** `AssistProposalItem` を検証する。 */
export const decodeAssistProposalItem: Decoder<AssistProposalItem> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const item: AssistProposalItem = {
    proposal: requireMember(source, "proposal", path, decodeAssistProposal),
    targetOrigin: requireEnum(
      source,
      "targetOrigin",
      path,
      assertionTargetOrigins,
    ),
    endpointsInGraph: optionalBoolean(source, "endpointsInGraph", path),
  };
  if (item.proposal.addsRelation !== (item.endpointsInGraph !== undefined)) {
    throw new DecodeFailure(
      `${path}.endpointsInGraph`,
      "a proposal that adds a relation alone carries it",
    );
  }
  return item;
};

/** AI 提案の一覧の応答。採否を決めた提案も含めて全件を持つ。 */
export type AssistProposalsResponse = {
  proposals: AssistProposalItem[];
  proposalCount: number;
  /** 状態が提案中の提案の件数。 */
  pendingCount: number;
  emptyReason?: string;
};

/** AI 提案の一覧の応答を検証する。 */
export const decodeAssistProposalsResponse: Decoder<AssistProposalsResponse> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const proposals = requireArray(
    source,
    "proposals",
    path,
    decodeAssistProposalItem,
  );
  return {
    proposals,
    proposalCount: requireCountEqualsElements(
      requireCount(source, "proposalCount", path),
      proposals.length,
      `${path}.proposalCount`,
    ),
    pendingCount: requireCountEqualsElements(
      requireCount(source, "pendingCount", path),
      proposals.filter((item) => item.proposal.state === "proposed").length,
      `${path}.pendingCount`,
    ),
    emptyReason: optionalString(source, "emptyReason", path),
  };
};

/** 採用の応答。採用に変えた提案と、採用で作った分析者の所見を持つ。 */
export type AssistProposalAdoption = {
  proposal: AssistProposalItem;
  assertion: AssertionItem;
};

/**
 * 採用の応答を検証する。提案が採用済みで、採用の記録と所見が互いを指すことを確かめる。
 */
export const decodeAssistProposalAdoption: Decoder<AssistProposalAdoption> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const adoption = {
    proposal: requireMember(source, "proposal", path, decodeAssistProposalItem),
    assertion: requireMember(source, "assertion", path, decodeAssertionItem),
  };
  const { proposal } = adoption.proposal;
  const { assertion } = adoption.assertion;
  if (
    proposal.state !== "adopted" ||
    proposal.decision?.assertionId !== assertion.id
  ) {
    throw new DecodeFailure(
      `${path}.proposal.proposal.decision`,
      "expected the adoption that names the assertion",
    );
  }
  if (assertion.proposalId !== proposal.id) {
    throw new DecodeFailure(
      `${path}.assertion.assertion.proposalId`,
      "expected the adopted proposal",
    );
  }
  return adoption;
};
