import { expect, test } from "vitest";
import { decodeTimelineResponse } from "@/shared/contracts/timeline";
import {
  type GeneratedEntrySpec,
  generatedSources,
  generatedTimelineResponseJson,
} from "@/testdata/timeline/timelineResponse";
import { timelineSourcesOf } from "./sourceLabels";

function sourcesOf(specs: GeneratedEntrySpec[]) {
  const response = decodeTimelineResponse(
    generatedTimelineResponseJson(specs),
    "$",
  );
  return timelineSourcesOf(response.entries, response.sourceCoverages);
}

const at = "2031-10-08T01:20:00.000Z";

test("file 名が同じ収集元には内容の SHA-256 の先頭を添え、file 名が 1 つだけの収集元には添えない", () => {
  const { labels } = sourcesOf([
    { normalized: at, source: "hostA" },
    { normalized: at, source: "hostACopy" },
    { normalized: at, source: "access" },
  ]);

  for (const source of [generatedSources.hostA, generatedSources.hostACopy]) {
    const label = labels.get(source.sourceId);
    expect(label?.fileName).toBe(source.fileName);
    expect(label?.distinguisher).toContain(source.contentSha256.slice(0, 12));
  }
  expect(labels.get(generatedSources.access.sourceId)).toEqual({
    fileName: generatedSources.access.fileName,
  });
});

test("並びは収録範囲の表の並びに従う", () => {
  const { order } = sourcesOf([
    { normalized: at, source: "access" },
    { normalized: at, source: "hostA" },
  ]);
  // 収録範囲の表は generatedSources の順に並ぶ。行が先に出た収集元を先にしない。
  expect(order).toEqual([
    generatedSources.hostA.sourceId,
    generatedSources.access.sourceId,
  ]);
});

test("内容の識別の先頭も同じ収集元には、収集元の識別子を添える", () => {
  const response = decodeTimelineResponse(
    generatedTimelineResponseJson([
      { normalized: at, source: "hostA" },
      { normalized: at, source: "hostACopy" },
    ]),
    "$",
  );
  const sameContent = response.entries.map((entry) => ({
    ...entry,
    recordRef: {
      ...entry.recordRef,
      sourceContentSha256: generatedSources.hostA.contentSha256,
    },
  }));

  const { labels } = timelineSourcesOf(sameContent, response.sourceCoverages);

  for (const source of [generatedSources.hostA, generatedSources.hostACopy]) {
    expect(labels.get(source.sourceId)?.distinguisher).toContain(
      source.sourceId,
    );
  }
});
