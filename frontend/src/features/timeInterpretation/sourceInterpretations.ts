import type { TimestampInterpretation } from "@/shared/contracts/common";
import type { SourceInterpretation } from "./useTimeInterpretations";

/** 収集元の一覧の 1 行のうち、解釈を求めるのに使う項目。 */
type SourceRow = {
  source: { sourceId: string; contentSha256: string };
  importTimeOffset?: string;
};

/**
 * 収集元ごとに、今の時刻の解釈を sourceId で探す表を組む。backend が割当の期間を読む解釈
 * (`backend/pipeline/time_interpretation.go` の `withTimeInterpretations`) と同じ規則である。
 *
 * - 主張中の所見を 1 件だけ持つ収集元は、その所見のずれを使う。
 * - それ以外の収集元 (所見が無い、取り消した、2 件以上が主張中) は、取り込みの起動で指定した
 *   ずれ (`importTimeOffset`) を使う。起動で指定したずれは所見を持たず、所見の識別子を持たない
 *   解釈になる。
 */
export function sourceInterpretationsOf(
  rows: readonly SourceRow[],
  interpretations: ReadonlyMap<string, SourceInterpretation>,
): Map<string, TimestampInterpretation> {
  const bySourceId = new Map<string, TimestampInterpretation>();
  for (const row of rows) {
    const recorded = interpretations.get(row.source.contentSha256);
    if (
      recorded?.kind === "single" &&
      recorded.assertion.state === "active" &&
      recorded.assertion.timeOffset !== undefined
    ) {
      bySourceId.set(row.source.sourceId, {
        offset: recorded.assertion.timeOffset,
        assertionId: recorded.assertion.id,
      });
      continue;
    }
    if (row.importTimeOffset !== undefined) {
      bySourceId.set(row.source.sourceId, { offset: row.importTimeOffset });
    }
  }
  return bySourceId;
}
