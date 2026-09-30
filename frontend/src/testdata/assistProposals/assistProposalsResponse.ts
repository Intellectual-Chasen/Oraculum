/**
 * AI 提案の操作 (`/api/v0/assist-proposals`) の応答の JSON。
 * 識別子は backend が発行する不透明な値であり、本 fixture が決める。
 */

import { assertionRecordRef } from "../assertions/assertionsResponse";

/** fixture の提案が含む LLM の記述。 */
export const proposalNote = "起動の直後に外部の IP アドレスへ接続している";

/** fixture の提案を作った会話の識別子。 */
export const proposalConversationId = "0123456789abcdef0123456789abcdef";

/** fixture の提案が指すノード。 */
export const proposalNodeId = "n:process:8ab3";

/** 提案の対象の JSON。 */
export type ProposalTargetJson =
  | { kind: "node"; nodeId: string }
  | {
      kind: "edge";
      edge: { kind: string; sourceNodeId: string; targetNodeId: string };
    };

const nodeTarget: ProposalTargetJson = { kind: "node", nodeId: proposalNodeId };

/** グラフに無い関係を指す対象。 */
export const addedRelationTarget: ProposalTargetJson = {
  kind: "edge",
  edge: {
    kind: "file_copy",
    sourceNodeId: proposalNodeId,
    targetNodeId: "n:file:0c1d",
  },
};

/** 提案中の提案 1 件。 */
export function proposedItemJson(
  id = "ap:0001",
  target: ProposalTargetJson = nodeTarget,
) {
  return {
    proposal: {
      id,
      target,
      note: proposalNote,
      recordRefs: [assertionRecordRef],
      conversationId: proposalConversationId,
      turnId: "turn-1",
      provider: "claude",
      model: "model-a",
      addsRelation: false,
      matchConditions: [{ conditionKey: "destination_ip", tolerance: 0 }],
      state: "proposed",
      createdAt: "2026-01-02T03:04:05.000Z",
    },
    targetOrigin: "observation",
  };
}

/**
 * グラフに無い関係を足す、提案中の提案 1 件。関係はグラフに無いため `absent` であり、両端の
 * ノードの有無を `endpointsInGraph` が持つ。
 */
export function relationItemJson(
  endpointsInGraph: boolean,
  id = "ap:0003",
  target: ProposalTargetJson = addedRelationTarget,
) {
  const item = proposedItemJson(id, target);
  return {
    proposal: { ...item.proposal, addsRelation: true },
    targetOrigin: "absent",
    endpointsInGraph,
  };
}

/** 分析者 `analyst-b` が採用した提案 1 件。 */
export function adoptedItemJson(id = "ap:0001") {
  const item = proposedItemJson(id);
  return {
    ...item,
    proposal: {
      ...item.proposal,
      state: "adopted",
      decision: {
        analyst: "analyst-b",
        decidedAt: "2026-01-02T03:05:00.000Z",
        assertionId: "as:0100",
      },
    },
  };
}

/** 分析者 `analyst-b` が理由を付けて却下した提案 1 件。 */
export function rejectedItemJson(id = "ap:0001") {
  const item = proposedItemJson(id);
  return {
    ...item,
    proposal: {
      ...item.proposal,
      state: "rejected",
      decision: {
        analyst: "analyst-b",
        decidedAt: "2026-01-02T03:05:00.000Z",
        reason: "根拠は別の端末のもの",
      },
    },
  };
}

/** 提案の一覧。items を持つ。既定は提案中の提案 1 件である。 */
export function assistProposalsResponseJson(
  items: { proposal: { state: string } }[] = [proposedItemJson()],
) {
  return {
    proposals: items,
    proposalCount: items.length,
    pendingCount: items.filter((item) => item.proposal.state === "proposed")
      .length,
  };
}

/**
 * 採用の応答。`proposed` を採用した提案と、分析者を著者とする所見を持つ。所見は、提案が関係を
 * 足す提案なら関係を足す。
 */
export function adoptionJson(
  note = proposalNote,
  proposed: {
    proposal: { id: string; target: ProposalTargetJson; addsRelation: boolean };
    endpointsInGraph?: boolean;
  } = proposedItemJson(),
) {
  const { proposal } = proposed;
  return {
    proposal: {
      ...proposed,
      proposal: {
        ...proposal,
        state: "adopted",
        decision: {
          analyst: "analyst-b",
          decidedAt: "2026-01-02T03:05:00.000Z",
          assertionId: "as:0100",
        },
      },
    },
    assertion: {
      assertion: {
        id: "as:0100",
        target: proposal.target,
        state: "active",
        author: "analyst-b",
        recordedAt: "2026-01-02T03:05:00.000Z",
        basis: { note, recordRefs: [assertionRecordRef] },
        addsRelation: proposal.addsRelation,
        proposalId: proposal.id,
        revisionNumber: 1,
        history: [],
      },
      targetOrigin: proposal.addsRelation ? "analyst_assertion" : "observation",
    },
  };
}
