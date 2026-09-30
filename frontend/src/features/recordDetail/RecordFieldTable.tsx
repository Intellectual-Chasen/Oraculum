import type { ReactNode } from "react";
import type { EventKindPair, RecordField } from "@/shared/contracts/common";
import { toVisibleRawText } from "@/shared/lib/rawText";
import {
  type FieldValue,
  readRecordFieldNormalized,
  readRecordFieldRawText,
} from "@/shared/lib/recordField";
import { timestampPrecisionLabels } from "@/shared/lib/recordLabels";
import {
  conditionLabel,
  fieldConditions,
  type ValueCondition,
} from "@/shared/lib/valueCondition";
import { AddTermButton } from "@/shared/ui/AddTermButton";
import {
  type ContextMenuContent,
  RowMenuButton,
  useContextMenu,
} from "@/shared/ui/ContextMenu";
import { CopyButton } from "@/shared/ui/CopyButton";
import { Highlighted, PeriodMark } from "@/shared/ui/Highlighted";
import { type MenuEntry, menuSeparator } from "@/shared/ui/Menu";
import { MissingValue } from "@/shared/ui/MissingValue";
import { RawText } from "@/shared/ui/RawText";
import { InterpretedUtcNote } from "@/shared/ui/TimestampOffsetNote";
import { useCopyText } from "@/shared/ui/useCopyText";
import { useValueActions } from "@/shared/ui/ValueLink";

const precisionOutOfScope = "時刻のフィールド以外";

function FieldValueCell({
  field,
  value,
  action = null,
  note = null,
}: {
  field: RecordField;
  value: FieldValue;
  /** 値の後ろに置く操作。 */
  action?: ReactNode;
  /** 値の次の行に置く注記。 */
  note?: ReactNode;
}) {
  return (
    <td>
      {"text" in value ? (
        <PeriodMark
          timestamp={field.kind === "timestamp" ? field.timestamp : undefined}
        >
          <Highlighted text={value.text} field={field} />
        </PeriodMark>
      ) : (
        <MissingValue description={value.absence} />
      )}
      {action === null ? null : (
        <span className="inline-flex items-center ml-1 gap-0.5">{action}</span>
      )}
      {note}
    </td>
  );
}

function PrecisionCell({ field }: { field: RecordField }) {
  return (
    <td>
      {field.kind === "timestamp" ? (
        timestampPrecisionLabels[field.timestamp.precision]
      ) : (
        <MissingValue description={precisionOutOfScope} />
      )}
    </td>
  );
}

/**
 * 行の key を、`name` とその `name` の出現の順番から組む。
 * 原資料が同じ key を複数回書いたレコードでは応答の `name` が重複し、`name` だけでは
 * 行を一意に指せない。
 */
function keyedFields(
  fields: RecordField[],
): { key: string; field: RecordField }[] {
  const occurrences = new Map<string, number>();
  return fields.map((field) => {
    const occurrence = (occurrences.get(field.name) ?? 0) + 1;
    occurrences.set(field.name, occurrence);
    return { key: `${field.name}#${occurrence}`, field };
  });
}

/**
 * 欄 1 行のコンテキストメニューの項目を組む。条件の項目はフィールドの意味に適した順に出す
 * (時刻は期間、イベントの種類を決める欄はイベントの種類、ほかはフィールドの値が等しい条件が先)。
 * `addCondition` が無い画面では、値を写す項目だけを出す。
 */
function fieldMenuContent(
  field: RecordField | undefined,
  eventKind: EventKindPair | undefined,
  addCondition: ((condition: ValueCondition) => void) | undefined,
  copy: (text: string, what: string) => void,
): ContextMenuContent {
  if (field === undefined) {
    return { label: "フィールドの操作", entries: [] };
  }
  const rawText = readRecordFieldRawText(field);
  const normalized = readRecordFieldNormalized(field);
  const text = "text" in rawText ? rawText.text : undefined;
  const item = (
    key: string,
    label: string,
    run: (value: string) => void,
    value: string | undefined,
  ): MenuEntry => ({
    kind: "item",
    key,
    label,
    disabled: value === undefined,
    onSelect: () => {
      if (value !== undefined) run(value);
    },
  });
  const terms: MenuEntry[] =
    addCondition === undefined
      ? []
      : fieldConditions(field, eventKind).map((condition, index) => ({
          kind: "item",
          key: `condition-${index}`,
          label: conditionLabel(condition),
          onSelect: () => addCondition(condition),
        }));
  const copies: MenuEntry[] = [
    item(
      "copy-raw",
      "原文の文字列をコピー",
      (value) => copy(value, "原文の文字列"),
      text,
    ),
    item(
      "copy-normalized",
      "正規化値をコピー",
      (value) => copy(value, "正規化値"),
      "text" in normalized ? normalized.text : undefined,
    ),
    item(
      "copy-name",
      "フィールドの名前をコピー",
      (value) => copy(value, "フィールドの名前"),
      field.name,
    ),
  ];
  return {
    label: `フィールド ${toVisibleRawText(field.name)} の操作`,
    entries:
      terms.length === 0
        ? copies
        : [...terms, menuSeparator("copy"), ...copies],
  };
}

/**
 * レコードの項目を 1 行ずつ縦に並べる。
 * 行の数は応答の `fields` の要素数であり、返すレコードが持つ項目が決める。
 *
 * 行の右クリックと Shift+F10 と「…」の button から、値を条件に足す操作と写す操作のメニューを
 * 開く。
 */
export function RecordFieldTable({
  fields,
  eventKind,
}: {
  fields: RecordField[];
  /** 応答がレコードに付けたイベントの種類の組。 */
  eventKind?: EventKindPair;
}) {
  const keyed = keyedFields(fields);
  const { copy, notice } = useCopyText();
  // 条件を足す操作は表の値の操作 (ValueActions) が持つ。provider の無い画面では写す項目だけを出す。
  const addCondition = useValueActions()?.addCondition;
  // 対象は行の key で持ち、項目は描画ごとに今の欄から組む。
  const { triggers, openTarget, menu } = useContextMenu((key: string) =>
    fieldMenuContent(
      keyed.find((row) => row.key === key)?.field,
      eventKind,
      addCondition,
      copy,
    ),
  );
  const onAddTerm =
    addCondition === undefined
      ? undefined
      : (text: string) =>
          addCondition({ kind: "text", mode: "contains", text });
  return (
    <>
      <table>
        <caption className="sr-only">フィールド</caption>
        <thead>
          <tr>
            <th scope="col">フィールド</th>
            <th scope="col">原文</th>
            <th scope="col">正規化値</th>
            <th scope="col">精度</th>
            <th scope="col">操作</th>
          </tr>
        </thead>
        <tbody>
          {keyed.map(({ key, field }) => {
            const rawText = readRecordFieldRawText(field);
            const normalized = readRecordFieldNormalized(field);
            return (
              <tr
                key={key}
                onContextMenu={(event) => triggers.onContextMenu(event, key)}
                onKeyDown={(event) => triggers.onKeyDown(event, key)}
              >
                <th scope="row">
                  <RawText text={field.name} />
                  <span className="inline-flex items-center ml-1">
                    <CopyButton
                      text={field.name}
                      onCopy={(text) => copy(text, "フィールドの名前")}
                    />
                  </span>
                </th>
                <FieldValueCell
                  field={field}
                  value={rawText}
                  action={
                    !("text" in rawText) ? null : (
                      <>
                        {onAddTerm === undefined ? null : (
                          <AddTermButton
                            text={rawText.text}
                            onAdd={onAddTerm}
                          />
                        )}
                        <CopyButton
                          text={rawText.text}
                          onCopy={(text) => copy(text, "原文の文字列")}
                        />
                      </>
                    )
                  }
                />
                <FieldValueCell
                  field={field}
                  value={normalized}
                  action={
                    !("text" in normalized) ? null : (
                      <CopyButton
                        text={normalized.text}
                        onCopy={(text) => copy(text, "正規化値")}
                      />
                    )
                  }
                  note={
                    field.kind === "timestamp" ? (
                      <InterpretedUtcNote timestamp={field.timestamp} />
                    ) : field.kind === "text" &&
                      field.text.derivation !== undefined ? (
                      <span className="mt-0.5 block text-xs text-muted">
                        <RawText text={field.text.derivation} />
                      </span>
                    ) : null
                  }
                />
                <PrecisionCell field={field} />
                <td>
                  <RowMenuButton
                    label={`フィールド ${toVisibleRawText(field.name)} の操作`}
                    expanded={openTarget === key}
                    onClick={(event) => triggers.onButtonClick(event, key)}
                  />
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
      {notice}
      {menu}
    </>
  );
}
