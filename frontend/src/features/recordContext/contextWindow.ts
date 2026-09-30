import type { RecordField, Timestamp } from "@/shared/contracts/common";

/** 前後を取る幅の選択肢。秒で数える。 */
export const contextWindowSeconds = [10, 60, 300] as const;
export type ContextWindowSeconds = (typeof contextWindowSeconds)[number];

/** 事象の時刻を持つ語彙の項目。定義元は `backend/core/semantic_key.go` の `SemanticKeyEventTime` である。 */
const eventTimeSemantic = "event.time";

/** レコードの項目から事象の時刻を探す。項目が無い場合は `undefined` を返す。 */
export function findEventTime(
  fields: readonly RecordField[],
): Timestamp | undefined {
  for (const field of fields) {
    if (field.kind === "timestamp" && field.semantic === eventTimeSemantic) {
      return field.timestamp;
    }
  }
  return undefined;
}
