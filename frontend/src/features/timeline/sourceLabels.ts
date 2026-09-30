import type {
  SourceCoverage,
  TimelineEntry,
} from "@/shared/contracts/timeline";

/** 収集元 1 件を内訳の行で指す値。 */
export type SourceLabel = {
  fileName: string;
  /** 同じ file 名の収集元が他にあるときに、file 名に添えて区別する文字列。 */
  distinguisher?: string;
};

/** 内訳に並べる収集元の順と、各収集元を指す値。 */
export type TimelineSources = {
  /** 収集元の識別子の並び。収録範囲の表の並びの後に、そこに無い収集元を出た順で置く。 */
  order: string[];
  labels: Map<string, SourceLabel>;
};

const contentDigestPrefixLength = 12;

/**
 * 内訳に出す収集元の順と名前を決める。
 *
 * **file 名が同じ収集元に、内容の SHA-256 の先頭を添える。** 平常時の記録と調べる対象の
 * 記録のように、同じ file 名の収集元が 1 つの応答に並ぶ。file 名だけでは、内訳の件数が
 * どちらの収集元のものかを読めない。先頭の文字列も重なる収集元には、収集元の識別子を
 * 添える。
 */
export function timelineSourcesOf(
  entries: readonly TimelineEntry[],
  coverages: readonly SourceCoverage[],
): TimelineSources {
  const fileNames = new Map<string, string>();
  const digests = new Map<string, string>();
  for (const coverage of coverages) {
    fileNames.set(coverage.sourceId, coverage.sourceFileName);
  }
  for (const entry of entries) {
    const ref = entry.recordRef;
    if (!fileNames.has(ref.sourceId)) {
      fileNames.set(ref.sourceId, ref.sourceFileName);
    }
    if (!digests.has(ref.sourceId)) {
      digests.set(ref.sourceId, ref.sourceContentSha256);
    }
  }

  const sourcesByName = new Map<string, string[]>();
  for (const [sourceId, fileName] of fileNames) {
    sourcesByName.set(fileName, [
      ...(sourcesByName.get(fileName) ?? []),
      sourceId,
    ]);
  }

  const labels = new Map<string, SourceLabel>();
  for (const [fileName, sourceIds] of sourcesByName) {
    if (sourceIds.length === 1) {
      for (const sourceId of sourceIds) {
        labels.set(sourceId, { fileName });
      }
      continue;
    }
    const prefixes = sourceIds.map((sourceId) =>
      digests.get(sourceId)?.slice(0, contentDigestPrefixLength),
    );
    sourceIds.forEach((sourceId, index) => {
      const prefix = prefixes[index];
      const isUnique =
        prefix !== undefined &&
        prefixes.filter((other) => other === prefix).length === 1;
      labels.set(sourceId, {
        fileName,
        distinguisher: isUnique
          ? `sha256: ${prefix}`
          : `収集元 ID: ${sourceId}`,
      });
    });
  }
  return { order: [...fileNames.keys()], labels };
}
