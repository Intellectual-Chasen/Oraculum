import { expect, test } from "vitest";
import {
  graphResponseJson,
  nodeDetailResponseJson,
  overLimitGraphResponseJson,
  rejectedLogonNodeDetailResponseJson,
} from "@/testdata/graph/graphResponse";
import { DecodeFailure } from "./decoding";
import { decodeGraphResponse } from "./graph";
import {
  decodeEdgeAssignmentBasis,
  decodeNodeDetailResponse,
} from "./graphDetail";

test("応答が返した関係の種別を、要求に書いた順で読み、知らない種別を退ける", () => {
  const decoded = decodeGraphResponse(
    { ...graphResponseJson(), edgeKinds: ["ran_on", "file_operation"] },
    "response",
  );
  expect(decoded.edgeKinds).toEqual(["ran_on", "file_operation"]);
  expect(() =>
    decodeGraphResponse(
      { ...graphResponseJson(), edgeKinds: ["ran_on", "copied_to"] },
      "response",
    ),
  ).toThrow(DecodeFailure);
});

test("応答が用いた検索式を文字列のまま読み、出ない応答では検索式を持たない", () => {
  const expression = 'not TargetUserName == "and"';
  expect(
    decodeGraphResponse(
      { ...graphResponseJson(), searchExpression: expression },
      "response",
    ).searchExpression,
  ).toBe(expression);
  expect(
    decodeGraphResponse(graphResponseJson(), "response").searchExpression,
  ).toBeUndefined();
});

test("文字列でない検索式を含む応答を読まない", () => {
  expect(() =>
    decodeGraphResponse(
      { ...graphResponseJson(), searchExpression: ["LogonType == 3"] },
      "response",
    ),
  ).toThrow("response.searchExpression:");
});

test("上限を超えた図の応答を、合ったノードと、関係の種別ごとのエッジの本数で読む", () => {
  const decoded = decodeGraphResponse(
    overLimitGraphResponseJson(201, 200),
    "response",
  );
  expect(decoded.nodeLimitExceeded).toBe(true);
  expect(decoded.subgraphNodeCount).toBe(201);
  expect(decoded.nodeLimit).toBe(200);
  expect(decoded.nodeCount).toBe(2);
  expect(decoded.nodes.map((node) => node.selection)).toEqual([
    "matched",
    "matched",
  ]);
  expect(decoded.edges).toEqual([]);
  expect(decoded.nodeLimitExceeded && decoded.edgeKindCounts).toEqual([
    { kind: "ran_on", count: 1200 },
    { kind: "process_communication", count: 34 },
  ]);
});

test("上限に収まる応答は、上限を超えたことを持たない", () => {
  const decoded = decodeGraphResponse(
    { ...graphResponseJson(), nodeLimit: 2000 },
    "response",
  );
  expect(decoded.nodeLimitExceeded).toBe(false);
  expect(decoded.subgraphNodeCount).toBe(3);
  expect("edgeKindCounts" in decoded).toBe(false);
});

test("辿ったエッジが 0 本で上限を超えた応答を、空の edgeKindCounts で読む", () => {
  const decoded = decodeGraphResponse(
    { ...overLimitGraphResponseJson(201, 200), edgeKindCounts: [] },
    "response",
  );
  expect(decoded.nodeLimitExceeded).toBe(true);
  expect(decoded.nodeLimitExceeded && decoded.edgeKindCounts).toEqual([]);
});

test("上限を超えた応答の矛盾を退ける", () => {
  const exceeded = overLimitGraphResponseJson(201, 200);
  const spoiled: Record<string, unknown> = {
    "no limit": { ...exceeded, nodeLimit: undefined },
    "count within the limit": { ...exceeded, subgraphNodeCount: 200 },
    "edges while exceeded": { ...exceeded, edges: graphResponseJson().edges },
    // backend は上限を超えた応答に edgeKindCounts を必ず出す (0 本のときは空の配列)。
    "the edge kind counts member missing": {
      ...exceeded,
      edgeKindCounts: undefined,
    },
    "unknown edge kind": {
      ...exceeded,
      edgeKindCounts: [{ kind: "copied_to", count: 1 }],
    },
    "edge kind counts within the limit": {
      ...graphResponseJson(),
      edgeKindCounts: exceeded.edgeKindCounts,
    },
    "no subgraph node count": {
      ...graphResponseJson(),
      subgraphNodeCount: undefined,
    },
  };
  for (const [name, response] of Object.entries(spoiled)) {
    expect(() => decodeGraphResponse(response, "response"), name).toThrow(
      DecodeFailure,
    );
  }
});

const validBasis = {
  clientIp: "192.0.2.44",
  sourceId: "source-a",
  conditions: [],
  assumptions: [],
  clockDependencyNote: "Synthetic assignment basis.",
};
const basisWithoutConditions = {
  clientIp: "192.0.2.44",
  sourceId: "source-a",
  assumptions: [],
  clockDependencyNote: "Synthetic assignment basis.",
};

test.each([
  ["empty conditions", validBasis],
  ["missing conditions", basisWithoutConditions],
])("assignment basis with %s is rejected", (_description, input) => {
  expect(() => decodeEdgeAssignmentBasis(input, "basis")).toThrow(
    DecodeFailure,
  );
});

/** 関係にしなかったログオン 1 件を差し替えたノードの詳細を返す。 */
function withRejection(rejection: Record<string, unknown>) {
  return {
    ...rejectedLogonNodeDetailResponseJson(),
    logonSessionRejections: [rejection],
  };
}

const { logonSessionRejections: _omitted, ...withoutRejections } =
  nodeDetailResponseJson();
const [rejection] =
  rejectedLogonNodeDetailResponseJson().logonSessionRejections;
const { logon: _logon, ...rejectionWithoutLogon } = rejection ?? {};

test.each([
  [
    "関係にしなかったログオンの欄が無い",
    withoutRejections,
    "response.logonSessionRejections",
  ],
  [
    "契約の外の理由",
    withRejection({ ...rejection, reason: "unknown_reason" }),
    "response.logonSessionRejections[0].reason",
  ],
  [
    "ログオンの元レコードが無い",
    withRejection(rejectionWithoutLogon),
    "response.logonSessionRejections[0].logon",
  ],
])("ノードの詳細の%sを拒否する", (_name, body, path) => {
  expect(() => decodeNodeDetailResponse(body, "response")).toThrow(
    DecodeFailure,
  );
  try {
    decodeNodeDetailResponse(body, "response");
  } catch (error) {
    expect((error as DecodeFailure).path).toBe(path);
  }
});
