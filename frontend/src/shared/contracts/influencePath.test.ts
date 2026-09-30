import { expect, test } from "vitest";
import {
  graphResponseJson,
  nodeDetailResponseJson,
  processNodeId,
  terminalNodeId,
} from "@/testdata/graph/graphResponse";
import { DecodeFailure } from "./decoding";
import { decodeInfluencePathResponse } from "./influencePath";

const decode = (json: unknown) => decodeInfluencePathResponse(json, "$");

function graphNode(id: string) {
  const found = graphResponseJson().nodes.find((node) => node.id === id);
  if (found === undefined) throw new Error("fixture lacks the node");
  const { selection, ...node } = found;
  void selection;
  return node;
}

const [record] = nodeDetailResponseJson().evidence;

function valid() {
  return {
    from: processNodeId,
    to: terminalNodeId,
    excludedBases: ["candidate"],
    origin: { key: "v:p", record },
    destination: { key: "v:t", record },
    vertices: [
      { key: "v:p", node: graphNode(processNodeId), influenceEdgeCount: 1 },
      { key: "v:t", node: graphNode(terminalNodeId), influenceEdgeCount: 1 },
    ],
    edges: [
      {
        id: "s:1",
        sourceKey: "v:p",
        targetKey: "v:t",
        graphEdgeId: "e:ran_on:0001",
        graphEdgeKind: "ran_on",
        bases: ["specified_operation", "observed", "single_node"],
        timeBases: ["same_terminal"],
        evidence: [record],
      },
    ],
    omittedEdgeCount: 0,
    stops: [],
    routeExcludedBases: [],
    frontier: [
      {
        key: "v:f",
        node: graphNode(terminalNodeId),
        influenceEdgeCount: 0,
        reason: "no_outgoing_influence",
      },
    ],
    frontierCount: 4,
    untimedRecordCount: 2,
  };
}

test("影響のエッジと起点・終点と frontier を読み、frontier は vertices に無いノードの項目を持ち、frontierCount 件の先頭でよい", () => {
  const response = decode(valid());
  expect(response.edges[0]?.evidence).toHaveLength(1);
  expect(response.origin?.key).toBe("v:p");
  expect(response.frontier[0]?.node.id).toBe(terminalNodeId);
  expect(response.frontier[0]?.reason).toBe("no_outgoing_influence");
  expect(response.frontierCount).toBe(4);
});

test("不確定の連鎖のエッジの候補の並びの件数と区分の条件を読む", () => {
  const json = valid();
  json.edges[0] = {
    ...json.edges[0],
    candidateTally: { candidateCount: 2, precedingCandidateCount: 1 },
    candidateOrder: ["session_account_match", "session_logon_other"],
  } as never;
  const [edge] = decode(json).edges;
  expect(edge?.candidateTally).toEqual({
    candidateCount: 2,
    precedingCandidateCount: 1,
  });
  expect(edge?.candidateOrder).toEqual([
    "session_account_match",
    "session_logon_other",
  ]);
});

test.each([
  [
    "根拠のレコードを持たない影響のエッジ",
    (json: ReturnType<typeof valid>) => {
      json.edges[0] = { ...json.edges[0], evidence: [] } as never;
    },
    "$.edges[0].evidence",
  ],
  [
    "空の key の要素",
    (json: ReturnType<typeof valid>) => {
      json.vertices[1] = { ...json.vertices[1], key: "" } as never;
    },
    "$.vertices[1].key",
  ],
  [
    "空の識別子の影響のエッジ",
    (json: ReturnType<typeof valid>) => {
      json.edges[0] = { ...json.edges[0], id: "" } as never;
    },
    "$.edges[0].id",
  ],
  [
    "vertices に無い起点",
    (json: ReturnType<typeof valid>) => {
      json.origin = { key: "v:absent", record };
    },
    "$.origin.key",
  ],
  [
    "vertices に無い終点",
    (json: ReturnType<typeof valid>) => {
      json.destination = { key: "v:absent", record };
    },
    "$.destination.key",
  ],
  [
    "frontierCount より多い frontier",
    (json: ReturnType<typeof valid>) => {
      json.frontierCount = 0;
    },
    "$.frontier",
  ],
])("%s を拒む", (_name, change, path) => {
  const json = valid();
  change(json);
  expect(() => decode(json)).toThrow(DecodeFailure);
  try {
    decode(json);
  } catch (error) {
    expect((error as DecodeFailure).path).toBe(path);
  }
});
