import { expect, test } from "vitest";
import {
  candidateResponse,
  candidateRule,
  remoteServicesCandidateMatch,
  remoteServicesCandidateRule,
} from "@/testdata/attackCandidates/candidateResponse";
import { decodeAttackCandidatesResponse } from "./attackCandidates";
import { DecodeFailure } from "./decoding";

const t1055Match = candidateResponse().matches[0];
if (!t1055Match) throw new Error("synthetic T1055 match is required");
const t1055Edge = t1055Match.edges[0];
if (!t1055Edge) throw new Error("synthetic T1055 edge is required");
const remotePrimary = remoteServicesCandidateMatch.edges[0];
const remoteContextual = remoteServicesCandidateMatch.edges[1];
if (!remotePrimary || !remoteContextual) {
  throw new Error("synthetic T1021 edges are required");
}

function wireValue(value: unknown): unknown {
  return JSON.parse(JSON.stringify(value));
}

test("single-edge T1055 matchをnested responseからdecodeする", () => {
  const response = decodeAttackCandidatesResponse(
    candidateResponse([t1055Match]),
    "response",
  );

  expect(response.matches).toHaveLength(1);
  expect(response.matches[0]?.ruleId).toBe(candidateRule.id);
  expect(response.matches[0]?.edges.map((edge) => edge.role)).toEqual(["edge"]);
  expect(response.matches[0]?.edges[0]?.edgeId).toBe("e:injection:1");
  expect(response.matches[0]?.edges[0]?.sourceNode.id).toBe("n:process:source");
  expect(response.matches[0]?.edges[0]?.sinkNode.id).toBe("n:process:sink");
  expect(response.matches[0]?.edges[0]?.evidence[0]?.sourceFileName).toBe(
    "synthetic.log",
  );
});

test("T1021のprimaryとcontextual edgeを一つのmatchとしてdecodeする", () => {
  const response = decodeAttackCandidatesResponse(
    candidateResponse(
      [remoteServicesCandidateMatch],
      [remoteServicesCandidateRule],
    ),
    "response",
  );
  const edges = response.matches[0]?.edges;
  const assignmentCondition = edges?.[0]?.assignmentBases[0]?.conditions.find(
    (condition) => condition.conditionKey === "terminal_ip_assignment",
  );

  expect(response.matches[0]?.matchId).toBe("match:remote-service:1");
  expect(
    assignmentCondition,
    "terminal_ip_assignment condition is required",
  ).toBeDefined();
  if (!assignmentCondition) {
    throw new Error("terminal_ip_assignment condition is required");
  }
  expect(edges?.map((edge) => [edge.role, edge.edgeId])).toEqual([
    ["session", "e:remote-session:1"],
    ["address", "e:terminal-address:1"],
  ]);
  expect(edges?.map((edge) => [edge.sourceNode.id, edge.sinkNode.id])).toEqual([
    ["n:terminal:source", "n:terminal:remote"],
    ["n:terminal:source", "n:ip:client"],
  ]);
  expect(edges?.map((edge) => edge.evidence[0]?.sourceFileName)).toEqual([
    "synthetic-session.log",
    "synthetic-address.log",
  ]);
  expect(edges?.[0]?.assignmentBases[0]?.clientIp).toBe("192.0.2.44");
  expect(assignmentCondition.leftValue?.[0]).toMatchObject({
    semantic: "connection.source_address",
    text: { rawText: edges[1]?.sinkNode.identity[0]?.value },
  });
  expect(assignmentCondition.rightValue?.[0]).toMatchObject({
    semantic: "terminal.id",
    text: { normalized: edges[0]?.sourceNode.identity[0]?.value },
  });
  expect(edges?.[0]?.sourceNode.identity[0]?.value).toBe("host-a.example.test");
  expect(assignmentCondition.conditionKey).toBe("terminal_ip_assignment");
  expect(assignmentCondition.leftValue?.[0]).toMatchObject({
    kind: "text",
    semantic: "connection.source_address",
    text: { rawText: "192.0.2.44", valueState: "present" },
  });
  expect(edges?.[1]?.assignmentBases).toEqual([]);
});

test("matched edgeごとのkindをdecodeする", () => {
  const matchWithKinds = {
    ...remoteServicesCandidateMatch,
    edges: [
      { ...remotePrimary, kind: "terminal_remote_session" as const },
      { ...remoteContextual, kind: "terminal_address" as const },
    ],
  };
  const response = decodeAttackCandidatesResponse(
    candidateResponse([matchWithKinds], [remoteServicesCandidateRule]),
    "response",
  );

  expect(response.matches[0]?.edges).toMatchObject([
    { role: "session", kind: "terminal_remote_session" },
    { role: "address", kind: "terminal_address" },
  ]);
});

test("候補なしの空配列をdecodeする", () => {
  expect(
    decodeAttackCandidatesResponse(candidateResponse([], []), "response"),
  ).toMatchObject({ rules: [], matches: [], notEvaluated: [] });
});

test("missing_inputとevaluated zero matchesを別の状態としてdecodeする", () => {
  const response = candidateResponse([], [candidateRule]);
  const rule = response.rules[0];
  const variant = rule?.variants[0];
  if (rule === undefined || variant === undefined)
    throw new Error("missing test variant");
  variant.evaluation = {
    state: "not_evaluated",
    reason: "missing_input",
    detail: "windows.sysmon",
  };
  response.notEvaluated = [
    {
      ruleId: rule.id,
      variantId: variant.id,
      reason: "missing_input",
      detail: "windows.sysmon",
    },
  ];

  const decoded = decodeAttackCandidatesResponse(response, "response");
  expect(decoded.matches).toEqual([]);
  expect(decoded.rules[0]?.variants[0]?.evaluation.state).toBe("not_evaluated");
  expect(decoded.notEvaluated[0]?.reason).toBe("missing_input");
});

test("not_evaluated variantにmatchがある応答を拒否する", () => {
  const response = candidateResponse([t1055Match]);
  const variant = response.rules[0]?.variants[0];
  if (variant === undefined) throw new Error("missing test variant");
  variant.evaluation = { state: "not_evaluated", reason: "missing_input" };
  response.notEvaluated = [
    {
      ruleId: t1055Match.ruleId,
      variantId: t1055Match.variantId,
      reason: "missing_input",
      detail: "synthetic missing input",
    },
  ];
  expect(() => decodeAttackCandidatesResponse(response, "response")).toThrow(
    DecodeFailure,
  );
});

const invalidResponses: Array<[string, unknown, string]> = [
  [
    "重複したruleIdとmatchId",
    candidateResponse([
      t1055Match,
      { ...t1055Match, matchId: t1055Match.matchId },
    ]),
    "response.matches[1].matchId",
  ],
  [
    "edgeが空のmatch",
    candidateResponse([{ ...t1055Match, edges: [] }]),
    "response.matches[0].edges",
  ],
  [
    "match内のedge ID重複",
    candidateResponse(
      [
        {
          ...remoteServicesCandidateMatch,
          edges: [
            remotePrimary,
            {
              ...remoteContextual,
              edgeId: remotePrimary.edgeId,
            },
          ],
        },
      ],
      [remoteServicesCandidateRule],
    ),
    "response.matches[0].edges[1].edgeId",
  ],
  [
    "match内のrole重複",
    candidateResponse(
      [
        {
          ...remoteServicesCandidateMatch,
          edges: [
            remotePrimary,
            {
              ...remoteContextual,
              role: remotePrimary.role,
            },
          ],
        },
      ],
      [remoteServicesCandidateRule],
    ),
    "response.matches[0].edges[1].role",
  ],
  [
    "不明なedge kind",
    wireValue({
      ...candidateResponse([t1055Match]),
      matches: [
        {
          ...t1055Match,
          edges: [{ ...t1055Edge, kind: "unknown" }],
        },
      ],
    }),
    "response.matches[0].edges[0].kind",
  ],
  [
    "不正nodeを含むedge",
    wireValue({
      ...candidateResponse([t1055Match]),
      matches: [
        {
          ...t1055Match,
          edges: [
            {
              ...t1055Edge,
              sourceNode: { ...t1055Edge.sourceNode, kind: "unknown" },
            },
          ],
        },
      ],
    }),
    "response.matches[0].edges[0].sourceNode.kind",
  ],
  [
    "assignmentBasesの誤型",
    wireValue({
      ...candidateResponse([t1055Match]),
      matches: [
        {
          ...t1055Match,
          edges: [{ ...t1055Edge, assignmentBases: "invalid" }],
        },
      ],
    }),
    "response.matches[0].edges[0].assignmentBases",
  ],
  [
    "terminal_ip_assignmentを含まないassignment basis",
    wireValue({
      ...candidateResponse(
        [remoteServicesCandidateMatch],
        [remoteServicesCandidateRule],
      ),
      matches: [
        {
          ...remoteServicesCandidateMatch,
          edges: [
            {
              ...remotePrimary,
              assignmentBases: [
                {
                  ...remotePrimary.assignmentBases[0],
                  conditions: [],
                },
              ],
            },
            remoteContextual,
          ],
        },
      ],
    }),
    "response.matches[0].edges[0].assignmentBases[0].conditions",
  ],
  [
    "evidenceの誤型",
    wireValue({
      ...candidateResponse([t1055Match]),
      matches: [
        { ...t1055Match, edges: [{ ...t1055Edge, evidence: "invalid" }] },
      ],
    }),
    "response.matches[0].edges[0].evidence",
  ],
  [
    "rule matchCountとmatch件数の不一致",
    {
      ...candidateResponse([t1055Match]),
      rules: [{ ...candidateRule, matchCount: 2 }],
    },
    "response.rules[0].matchCount",
  ],
  [
    "未知ruleへの参照",
    candidateResponse([{ ...t1055Match, ruleId: "attack.unknown" }]),
    "response.matches[0].ruleId",
  ],
];

test.each(invalidResponses)("%sを拒否する", (_name, body, path) => {
  expect(() => decodeAttackCandidatesResponse(body, "response")).toThrow(
    DecodeFailure,
  );
  try {
    decodeAttackCandidatesResponse(body, "response");
  } catch (error) {
    expect((error as DecodeFailure).path).toBe(path);
  }
});
