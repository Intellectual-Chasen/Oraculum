import { expect, test } from "vitest";
import {
  adoptedItemJson,
  adoptionJson,
  assistProposalsResponseJson,
  proposalNote,
  proposedItemJson,
  rejectedItemJson,
  relationItemJson,
} from "@/testdata/assistProposals/assistProposalsResponse";
import {
  decodeAssistProposalAdoption,
  decodeAssistProposalItem,
  decodeAssistProposalsResponse,
} from "./assistProposals";
import { DecodeFailure } from "./decoding";

test("提案の一覧を読み、件数と未決の件数を要素と突き合わせる", () => {
  const decoded = decodeAssistProposalsResponse(
    assistProposalsResponseJson(),
    "$",
  );
  expect(decoded.proposals[0]?.proposal.note).toBe(proposalNote);
  expect(decoded.proposalCount).toBe(decoded.proposals.length);
  expect(decoded.pendingCount).toBe(1);

  expect(() =>
    decodeAssistProposalsResponse(
      { ...assistProposalsResponseJson(), pendingCount: 0 },
      "$",
    ),
  ).toThrow(DecodeFailure);
  expect(() =>
    decodeAssistProposalsResponse(
      { ...assistProposalsResponseJson(), proposalCount: 2 },
      "$",
    ),
  ).toThrow(DecodeFailure);
});

test("採否の記録は状態と対応し、採用は所見の識別子を持つ", () => {
  expect(
    decodeAssistProposalItem(adoptedItemJson(), "$").proposal.decision
      ?.assertionId,
  ).toBe("as:0100");

  const proposed = proposedItemJson();
  const decidedWithoutRecord = {
    ...proposed,
    proposal: { ...proposed.proposal, state: "rejected" },
  };
  const proposedWithRecord = {
    ...proposed,
    proposal: {
      ...proposed.proposal,
      decision: { analyst: "a", decidedAt: "2026-01-02T03:05:00.000Z" },
    },
  };
  const adopted = adoptedItemJson();
  const adoptedWithoutAssertion = {
    ...adopted,
    proposal: {
      ...adopted.proposal,
      decision: { analyst: "a", decidedAt: "2026-01-02T03:05:00.000Z" },
    },
  };
  for (const input of [
    decidedWithoutRecord,
    proposedWithRecord,
    adoptedWithoutAssertion,
  ]) {
    expect(() => decodeAssistProposalItem(input, "$")).toThrow(DecodeFailure);
  }
});

test("収集元を指す提案と、根拠を持たない提案を退ける", () => {
  const proposed = proposedItemJson();
  const onSource = {
    ...proposed,
    proposal: {
      ...proposed.proposal,
      target: { kind: "source", sourceContentSha256: "a".repeat(64) },
    },
  };
  const withoutBasis = {
    ...proposed,
    proposal: { ...proposed.proposal, recordRefs: [] },
  };
  expect(() => decodeAssistProposalItem(onSource, "$")).toThrow(DecodeFailure);
  expect(() => decodeAssistProposalItem(withoutBasis, "$")).toThrow(
    DecodeFailure,
  );
});

test("採否の記録と対象の組が Go の Validate の条件に合わない提案を退ける", () => {
  const proposed = proposedItemJson();
  const adopted = adoptedItemJson();
  const rejected = rejectedItemJson();
  const withProposal = (
    base: { proposal: object; targetOrigin: string },
    proposal: object,
  ) => ({ ...base, proposal: { ...base.proposal, ...proposal } });
  const failing = {
    "reason on an adoption": withProposal(adopted, {
      decision: { ...adopted.proposal.decision, reason: "理由" },
    }),
    "assertion on a rejection": withProposal(rejected, {
      decision: { ...rejected.proposal.decision, assertionId: "as:1" },
    }),
    "relation added on a node": withProposal(proposed, { addsRelation: true }),
    "node without its identifier": withProposal(proposed, {
      target: { kind: "node" },
    }),
    "blank note": withProposal(proposed, { note: "  " }),
    "endpoints on a node proposal": { ...proposed, endpointsInGraph: true },
    "relation without its endpoints": {
      proposal: relationItemJson(true).proposal,
      targetOrigin: "absent",
    },
  };
  for (const [name, input] of Object.entries(failing)) {
    expect(() => decodeAssistProposalItem(input, "$"), name).toThrow(
      DecodeFailure,
    );
  }
  expect(
    decodeAssistProposalItem(rejected, "$").proposal.decision?.reason,
  ).toBe("根拠は別の端末のもの");
  expect(decodeAssistProposalItem(relationItemJson(false), "$")).toMatchObject({
    targetOrigin: "absent",
    endpointsInGraph: false,
  });
});

test("採用の応答の提案は採用済みで、採用の記録が所見を指す", () => {
  const adoption = adoptionJson();
  const notAdopted = { ...adoption, proposal: proposedItemJson() };
  const otherAssertion = {
    ...adoption,
    assertion: {
      ...adoption.assertion,
      assertion: { ...adoption.assertion.assertion, id: "as:other" },
    },
  };
  expect(() => decodeAssistProposalAdoption(notAdopted, "$")).toThrow(
    DecodeFailure,
  );
  expect(() => decodeAssistProposalAdoption(otherAssertion, "$")).toThrow(
    DecodeFailure,
  );
});

test("採用の応答の所見は、採用した提案の識別子を持つ", () => {
  const decoded = decodeAssistProposalAdoption(adoptionJson("直した記述"), "$");
  expect(decoded.assertion.assertion.proposalId).toBe(
    decoded.proposal.proposal.id,
  );
  expect(decoded.assertion.assertion.basis.note).toBe("直した記述");

  const other = adoptionJson();
  const mismatched = {
    ...other,
    assertion: {
      ...other.assertion,
      assertion: { ...other.assertion.assertion, proposalId: "ap:other" },
    },
  };
  expect(() => decodeAssistProposalAdoption(mismatched, "$")).toThrow(
    DecodeFailure,
  );
});
