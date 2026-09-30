import { expect, test } from "vitest";
import {
  baselineCaseId,
  casedEdgeDetailResponseJson,
  casedGraphResponseJson,
  casedNodeDetailResponseJson,
  casedSourcesResponseJson,
  challengeCaseId,
} from "@/testdata/cases/caseCounts";
import {
  edgeDetailResponseJson,
  graphResponseJson,
  nodeDetailResponseJson,
} from "@/testdata/graph/graphResponse";
import {
  accessLogSource,
  sourcesResponseJson,
} from "@/testdata/sources/sourcesResponse";
import { timelineResponseJson } from "@/testdata/timeline/timelineResponse";
import { decodeCaseId, optionalEvidenceByCase } from "./cases";
import { DecodeFailure } from "./decoding";
import { decodeGraphResponse } from "./graph";
import {
  decodeEdgeDetailResponse,
  decodeNodeDetailResponse,
} from "./graphDetail";
import { decodeSourceIdentity, decodeSourcesResponse } from "./sources";
import { decodeTimelineResponse } from "./timeline";

test("案件の識別子は英数字と . _ - の 1 文字以上 64 文字以下を受け取る", () => {
  for (const accepted of ["baseline", "case-2021_a.1", "a".repeat(64)]) {
    expect(decodeCaseId(accepted, "caseId")).toBe(accepted);
  }
});

test("案件の識別子の規則に合わない文字列を退ける", () => {
  for (const rejected of [
    "",
    "a".repeat(65),
    "base line",
    "base/line",
    "平常時",
    "case\n",
    3,
    null,
  ]) {
    expect(() => decodeCaseId(rejected, "caseId")).toThrow(DecodeFailure);
  }
});

test("案件ごとの件数が無い組では undefined を返し、0 件の集合で埋めない", () => {
  expect(optionalEvidenceByCase({}, "edge", 4)).toBeUndefined();
});

test("案件ごとの件数を昇順で読み、和が全体の件数と等しいことを確かめる", () => {
  const counts = [
    { caseId: "baseline", evidenceCount: 3 },
    { caseId: "challenge", evidenceCount: 1 },
  ];
  const total = counts.reduce(
    (running, count) => running + count.evidenceCount,
    0,
  );

  expect(
    optionalEvidenceByCase({ evidenceByCase: counts }, "edge", total),
  ).toEqual(counts);
});

test("案件ごとの件数の和が全体の件数と食い違う組を退ける", () => {
  const source = {
    evidenceByCase: [
      { caseId: "baseline", evidenceCount: 3 },
      { caseId: "challenge", evidenceCount: 1 },
    ],
  };

  expect(() => optionalEvidenceByCase(source, "edge", 5)).toThrow(
    "edge.evidenceByCase",
  );
});

test("案件の並びが昇順でない組と、同じ案件を 2 回持つ組を退ける", () => {
  const descending = {
    evidenceByCase: [
      { caseId: "challenge", evidenceCount: 1 },
      { caseId: "baseline", evidenceCount: 3 },
    ],
  };
  const repeated = {
    evidenceByCase: [
      { caseId: "baseline", evidenceCount: 1 },
      { caseId: "baseline", evidenceCount: 3 },
    ],
  };

  expect(() => optionalEvidenceByCase(descending, "edge", 4)).toThrow(
    "edge.evidenceByCase[1].caseId",
  );
  expect(() => optionalEvidenceByCase(repeated, "edge", 4)).toThrow(
    "edge.evidenceByCase[1].caseId",
  );
});

test("件数が 0 の案件と、規則に合わない案件の識別子を退ける", () => {
  const zero = {
    evidenceByCase: [
      { caseId: "baseline", evidenceCount: 0 },
      { caseId: "challenge", evidenceCount: 4 },
    ],
  };
  const malformed = {
    evidenceByCase: [{ caseId: "base line", evidenceCount: 4 }],
  };

  expect(() => optionalEvidenceByCase(zero, "edge", 4)).toThrow(
    "edge.evidenceByCase[0].evidenceCount",
  );
  expect(() => optionalEvidenceByCase(malformed, "edge", 4)).toThrow(
    "edge.evidenceByCase[0].caseId",
  );
});

test("収集元の案件を読み、案件を持たない収集元では undefined を返す", () => {
  const cased = decodeSourcesResponse(casedSourcesResponseJson(), "sources");
  const plain = decodeSourcesResponse(sourcesResponseJson(), "sources");

  expect(cased.sources.map((item) => item.source.caseId)).toEqual([
    baselineCaseId,
    challengeCaseId,
  ]);
  for (const item of plain.sources) {
    expect(item.source.caseId).toBeUndefined();
  }
});

test("規則に合わない案件を持つ収集元を退ける", () => {
  expect(() =>
    decodeSourceIdentity(
      { ...accessLogSource(), caseId: "base line" },
      "source",
    ),
  ).toThrow("source.caseId");
});

test("操作 9 のエッジが持つ案件ごとの件数を読み、持たない応答では undefined を返す", () => {
  const cased = decodeGraphResponse(casedGraphResponseJson(), "graph");
  const plain = decodeGraphResponse(graphResponseJson(), "graph");

  for (const edge of cased.edges) {
    const sum = (edge.evidenceByCase ?? []).reduce(
      (running, count) => running + count.evidenceCount,
      0,
    );
    expect(sum).toBe(edge.evidenceCount);
  }
  expect(
    cased.edges.find((edge) => edge.id === "e:ran_on:0001")?.evidenceByCase,
  ).toEqual([
    { caseId: baselineCaseId, evidenceCount: 3 },
    { caseId: challengeCaseId, evidenceCount: 1 },
  ]);
  for (const edge of plain.edges) {
    expect(edge.evidenceByCase).toBeUndefined();
  }
});

test("操作 9 と時系列の応答が返した案件を読み、要求が案件を与えない応答では undefined を返す", () => {
  expect(
    decodeGraphResponse(
      { ...graphResponseJson(), case: challengeCaseId },
      "graph",
    ).case,
  ).toBe(challengeCaseId);
  expect(
    decodeGraphResponse(graphResponseJson(), "graph").case,
  ).toBeUndefined();
  expect(
    decodeTimelineResponse(
      { ...timelineResponseJson(), case: challengeCaseId },
      "timeline",
    ).case,
  ).toBe(challengeCaseId);
  expect(
    decodeTimelineResponse(timelineResponseJson(), "timeline").case,
  ).toBeUndefined();
});

test("操作 9 のエッジの案件ごとの件数の和が evidenceCount と食い違う応答を読まない", () => {
  const whole = casedGraphResponseJson();
  const broken = {
    ...whole,
    edges: whole.edges.map((edge) =>
      edge.id === "e:ran_on:0001"
        ? {
            ...edge,
            evidenceByCase: [{ caseId: baselineCaseId, evidenceCount: 3 }],
          }
        : edge,
    ),
  };

  expect(() => decodeGraphResponse(broken, "graph")).toThrow(
    "graph.edges[0].evidenceByCase",
  );
});

test("操作 10 の根拠と、エッジの件数の案件ごとの件数を読む", () => {
  const cased = decodeNodeDetailResponse(casedNodeDetailResponseJson(), "node");
  const plain = decodeNodeDetailResponse(nodeDetailResponseJson(), "node");

  expect(cased.evidenceByCase).toEqual([
    { caseId: baselineCaseId, evidenceCount: 1 },
    { caseId: challengeCaseId, evidenceCount: 1 },
  ]);
  for (const count of cased.edgeCounts) {
    expect(count.evidenceByCase).toBeDefined();
  }
  expect(plain.evidenceByCase).toBeUndefined();
  for (const count of plain.edgeCounts) {
    expect(count.evidenceByCase).toBeUndefined();
  }
});

test("操作 10 の案件ごとの件数の和が evidenceCount と食い違う応答を読まない", () => {
  const broken = {
    ...casedNodeDetailResponseJson(),
    evidenceByCase: [{ caseId: baselineCaseId, evidenceCount: 1 }],
  };

  expect(() => decodeNodeDetailResponse(broken, "node")).toThrow(
    "node.evidenceByCase",
  );
});

test("操作 11 のエッジの案件ごとの件数を読み、持たない応答では undefined を返す", () => {
  const cased = decodeEdgeDetailResponse(casedEdgeDetailResponseJson(), "edge");
  const plain = decodeEdgeDetailResponse(edgeDetailResponseJson(), "edge");

  expect(cased.edge.evidenceByCase).toEqual([
    { caseId: baselineCaseId, evidenceCount: 1 },
    { caseId: challengeCaseId, evidenceCount: 3 },
  ]);
  expect(plain.edge.evidenceByCase).toBeUndefined();
});
