import type { RecordField, ValueState } from "../contracts/common";

/** フィールド 1 つの値。文字列を持つ場合と、持たない理由を返す場合を分ける。 */
export type FieldValue = { text: string } | { absence: string };

/** フィールドが原文を持たない理由の短いラベルを、値の状態ごとに返す。 */
export function describeRawTextAbsence(valueState: ValueState): string {
  switch (valueState) {
    case "item_absent":
      return "フィールドなし";
    case "derived":
      return "別の収集元から求めた値";
    case "derivation_undetermined":
      return "値を求められない";
    case "present":
    case "absent":
    case "no_body":
    case "out_of_definition":
    case "truncated":
      return "原文なし";
    default: {
      const exhaustive: never = valueState;
      throw new Error(`unknown value state: ${String(exhaustive)}`);
    }
  }
}

/** フィールドが比較に使う正規化値を持たない理由の短いラベルを、値の状態ごとに返す。 */
export function describeNormalizedAbsence(valueState: ValueState): string {
  switch (valueState) {
    case "item_absent":
      return "フィールドなし";
    case "derivation_undetermined":
      return "値を求められない";
    case "derived":
      return "求めた値なし";
    // 正規化値を持たない値は、原資料の文字列で一致を比べる。値が無い状態は比べない。
    case "present":
      return "原文で比較";
    case "absent":
    case "no_body":
    case "out_of_definition":
      return "値なし";
    case "truncated":
      return "値の末尾の欠け";
    default: {
      const exhaustive: never = valueState;
      throw new Error(`unknown value state: ${String(exhaustive)}`);
    }
  }
}

/** 語彙の項目 semantic を持つ 1 件目の文字列の項目から、文字列の一致を比べる値を読む。 */
function comparableOfSemantic(
  fields: readonly RecordField[],
  semantic: string,
): string | undefined {
  const field = fields.find(
    (item) => item.semantic === semantic && item.kind === "text",
  );
  if (field?.kind !== "text") return undefined;
  const { valueState, normalized, rawText } = field.text;
  return valueState === "present" || valueState === "derived"
    ? (normalized ?? rawText)
    : undefined;
}

/**
 * Windows イベントログのレコードの Event ID を返す。backend がレコードのノードの表示名に
 * Event ID を付ける条件と同じく、事象の分類を持たず、プロバイダと Event ID の両方を持つ
 * レコードだけが持つ (定義元は `backend/pipeline/event_kinds.go` の `eventKindOf`)。
 */
export function windowsEventIdOf(
  fields: readonly RecordField[],
): string | undefined {
  if (fields.some((field) => field.semantic === "event.category")) {
    return undefined;
  }
  const provider = comparableOfSemantic(fields, "windows_event.provider");
  const eventId = comparableOfSemantic(fields, "windows_event.id");
  return provider && eventId ? eventId : undefined;
}

/** レコードの 1 項目から原資料の文字列を読む。文字列が無いときは理由を返す。 */
export function readRecordFieldRawText(field: RecordField): FieldValue {
  switch (field.kind) {
    case "text":
      return field.text.rawText === undefined
        ? { absence: describeRawTextAbsence(field.text.valueState) }
        : { text: field.text.rawText };
    case "timestamp":
      return field.timestamp.rawText === undefined
        ? { absence: describeRawTextAbsence(field.timestamp.valueState) }
        : { text: field.timestamp.rawText };
    default: {
      const exhaustive: never = field;
      throw new Error(`unknown record field: ${JSON.stringify(exhaustive)}`);
    }
  }
}

/** レコードの 1 項目から比較に用いる正規化値を読む。値が無いときは理由を返す。 */
export function readRecordFieldNormalized(field: RecordField): FieldValue {
  switch (field.kind) {
    case "text":
      return field.text.normalized === undefined
        ? { absence: describeNormalizedAbsence(field.text.valueState) }
        : { text: field.text.normalized };
    case "timestamp":
      return field.timestamp.normalized === undefined
        ? { absence: describeNormalizedAbsence(field.timestamp.valueState) }
        : { text: field.timestamp.normalized };
    default: {
      const exhaustive: never = field;
      throw new Error(`unknown record field: ${JSON.stringify(exhaustive)}`);
    }
  }
}
