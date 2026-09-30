import type { RecordField } from "../contracts/common";
import { type FieldValue, readRecordFieldRawText } from "../lib/recordField";
import { Highlighted, PeriodMark } from "./Highlighted";
import { MissingValue } from "./MissingValue";
import { RawText } from "./RawText";

/** 項目の名前と値を 1 行ずつ並べる。 */
export function RecordFieldList({
  fields,
  readValue,
}: {
  fields: RecordField[];
  readValue: (field: RecordField) => FieldValue;
}) {
  return (
    <ul>
      {fields.map((field) => {
        const value = readValue(field);
        return (
          <li key={field.name}>
            <RawText text={field.name} />:{" "}
            {"text" in value ? (
              <PeriodMark
                timestamp={
                  field.kind === "timestamp" ? field.timestamp : undefined
                }
              >
                <Highlighted text={value.text} field={field} />
              </PeriodMark>
            ) : (
              <MissingValue description={value.absence} />
            )}
          </li>
        );
      })}
    </ul>
  );
}

/** 観測の種別の文字列だけを並べる。 */
export function ObservationFieldList({ fields }: { fields: RecordField[] }) {
  return <RecordFieldList fields={fields} readValue={readRecordFieldRawText} />;
}
