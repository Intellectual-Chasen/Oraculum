import type { EventKindPair, RecordField } from "../contracts/common";
import type { GraphTimeFilter } from "../contracts/graph";
import { readRecordFieldRawText } from "./recordField";
import type { SearchTermKind } from "./searchTerms";
import { contextTimeFilter } from "./timeFilter";

/**
 * 表の値から検索へ足す条件。値を表示の文字列から読み直さず、値が持つ構造から作る。
 *
 * - `text`: どれかのフィールドが文字列を含む・含まない条件。
 * - `field`: フィールドの値が文字列と等しい (`whole`)、または文字列を含む条件。
 * - `eventKind`: イベントの種類の条件。分類と動作を組で持つ。
 * - `period`: 期間の条件。
 */
export type ValueCondition =
  | { kind: "text"; mode: SearchTermKind; text: string }
  | { kind: "field"; field: string; text: string; whole: boolean }
  | { kind: "eventKind"; category: string; action: string }
  | { kind: "period"; filter: GraphTimeFilter };

/** 値のメニューの、写す文字列 1 つ。`what` は写した後の通知に書く、何を写したかの名前である。 */
export type ValueCopy = { what: string; text: string };

/** 条件のメニューの項目の名前。 */
export function conditionLabel(condition: ValueCondition): string {
  switch (condition.kind) {
    case "text":
      return condition.mode === "contains"
        ? "含む文字列に追加"
        : "含まない文字列に追加";
    case "field":
      return condition.whole
        ? "フィールドの値が等しい条件に追加"
        : "フィールドの値が文字列を含む条件に追加";
    case "eventKind":
      return "イベントの種類の条件に追加";
    case "period":
      return "期間の条件に追加";
    default: {
      const unreachable: never = condition;
      throw new Error(`unknown condition: ${JSON.stringify(unreachable)}`);
    }
  }
}

/** イベントの種類を決める語彙の項目。定義元は `backend/core/semantic_key.go` である。 */
const eventKindSemantics: ReadonlySet<string> = new Set([
  "event.category",
  "event.action",
  "windows_event.provider",
  "windows_event.id",
]);

/**
 * フィールドの値から足せる条件を、適した順に返す。時刻のフィールドは期間、イベントの種類を
 * 決めるフィールドはイベントの種類、ほかはフィールドの値が等しい条件を先にする。フィールドの
 * 条件は原文で作る。`eventKind` は応答がレコードに付けたイベントの種類の組である。
 *
 * フィールドの名前が `=` を含むフィールドは、名前と値の組の条件を作らない。組は最初の `=` で
 * 名前と値に分けて読まれる。
 */
export function fieldConditions(
  field: RecordField,
  eventKind?: EventKindPair,
): ValueCondition[] {
  const conditions: ValueCondition[] = [];
  if (field.kind === "timestamp") {
    const filter = contextTimeFilter(field.timestamp, 0);
    if (filter !== undefined) conditions.push({ kind: "period", filter });
  }
  if (
    eventKind !== undefined &&
    field.semantic !== undefined &&
    eventKindSemantics.has(field.semantic)
  ) {
    conditions.push({ kind: "eventKind", ...eventKind });
  }
  const raw = readRecordFieldRawText(field);
  if (!("text" in raw)) return conditions;
  if (field.name.trim() !== "" && !field.name.includes("=")) {
    conditions.push(
      { kind: "field", field: field.name, text: raw.text, whole: true },
      { kind: "field", field: field.name, text: raw.text, whole: false },
    );
  }
  conditions.push(
    { kind: "text", mode: "contains", text: raw.text },
    { kind: "text", mode: "excludes", text: raw.text },
  );
  return conditions;
}

/** 期間の条件を、両端の文字列を `/` で結んだ ISO 8601 の区間の文字列にする。 */
export function periodText(filter: GraphTimeFilter): string {
  return `${filter.from?.text ?? ""}/${filter.to?.text ?? ""}`;
}
