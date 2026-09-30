/**
 * 収集元に案件を付けた取り込みの応答の JSON。
 * 既存の fixture に案件と案件ごとの件数を足す。案件の識別子と件数の割り振りは本 fixture が
 * 生成する値であり、各要素の件数の和は元の fixture の `evidenceCount` と等しい。
 */
import { type CaseId, decodeCaseId } from "@/shared/contracts/cases";
import {
  edgeDetailResponseJson,
  graphResponseJson,
  nodeDetailResponseJson,
} from "@/testdata/graph/graphResponse";
import {
  accessLogSource,
  hostALogSource,
  sourcesResponseJson,
} from "@/testdata/sources/sourcesResponse";

/** 平常時のログに付ける案件。 */
export const baselineCaseId: CaseId = decodeCaseId("baseline", "fixture");

/** 調べる対象のログに付ける案件。 */
export const challengeCaseId: CaseId = decodeCaseId("challenge", "fixture");

type SourcesJson = {
  sources: { source: { sourceId: string } }[];
  sourceCount: number;
};

/** `access.log` に平常時の案件、`host-a.log` に調べる対象の案件を付けた応答。 */
export function casedSourcesResponseJson(): unknown {
  const caseOf = new Map([
    [accessLogSource().sourceId, baselineCaseId],
    [hostALogSource().sourceId, challengeCaseId],
  ]);
  const whole = sourcesResponseJson() as SourcesJson;
  return {
    ...whole,
    sources: whole.sources.map((item) => ({
      ...item,
      source: { ...item.source, caseId: caseOf.get(item.source.sourceId) },
    })),
  };
}

/** `access.log` だけに案件を付け、`host-a.log` は案件を持たない応答。 */
export function partlyCasedSourcesResponseJson(): unknown {
  const whole = sourcesResponseJson() as SourcesJson;
  return {
    ...whole,
    sources: whole.sources.map((item) =>
      item.source.sourceId === accessLogSource().sourceId
        ? { ...item, source: { ...item.source, caseId: baselineCaseId } }
        : item,
    ),
  };
}

/** 関係の種別ごとの、案件ごとの件数。 */
const edgeEvidenceByCase: Record<string, unknown> = {
  "e:ran_on:0001": [
    { caseId: baselineCaseId, evidenceCount: 3 },
    { caseId: challengeCaseId, evidenceCount: 1 },
  ],
  "e:process_communication:0002": [
    { caseId: challengeCaseId, evidenceCount: 1 },
  ],
};

/** エッジに案件ごとの件数を足した操作 9 の応答。 */
export function casedGraphResponseJson() {
  const whole = graphResponseJson();
  return {
    ...whole,
    edges: whole.edges.map((edge) => ({
      ...edge,
      evidenceByCase: edgeEvidenceByCase[edge.id],
    })),
  };
}

/** 根拠とエッジの件数に案件ごとの件数を足した操作 10 の応答。 */
export function casedNodeDetailResponseJson() {
  const whole = nodeDetailResponseJson();
  const byEdgeKind: Record<string, unknown> = {
    ran_on: edgeEvidenceByCase["e:ran_on:0001"],
    process_communication: edgeEvidenceByCase["e:process_communication:0002"],
  };
  return {
    ...whole,
    evidenceByCase: [
      { caseId: baselineCaseId, evidenceCount: 1 },
      { caseId: challengeCaseId, evidenceCount: 1 },
    ],
    edgeCounts: whole.edgeCounts.map((count) => ({
      ...count,
      evidenceByCase: byEdgeKind[count.edgeKind],
    })),
  };
}

/** エッジの根拠に案件ごとの件数を足した操作 11 の応答。 */
export function casedEdgeDetailResponseJson() {
  const whole = edgeDetailResponseJson();
  return {
    ...whole,
    edge: {
      ...whole.edge,
      evidenceByCase: [
        { caseId: baselineCaseId, evidenceCount: 1 },
        { caseId: challengeCaseId, evidenceCount: 3 },
      ],
    },
  };
}
