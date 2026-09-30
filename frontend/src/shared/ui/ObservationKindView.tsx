import type { ObservationKind } from "../contracts/common";
import { observationStatusLabels } from "../lib/recordLabels";
import { Hint } from "./Hint";
import { MissingValue } from "./MissingValue";
import { RawText } from "./RawText";
import { ObservationFieldList } from "./RecordFieldList";

const observationFieldsAbsent = "イベントの種類のフィールドなし";
/** イベントの種類の意味の状態が無いときの印。根拠のレコードの表も使う。 */
export const observationStatusAbsent = "イベントの種類を読み取れない";
/** 推定した意味の tooltip。根拠のレコードの表も使う。 */
export const inferredMeaningDescription =
  "推定した意味: 入力形式の仕様書に記述なし";

/**
 * 1 レコードのイベントの種類の文字列と、その意味の状態を「意味: 推定」の組で出す。
 * 意味の状態が `inferred` の応答は推定した意味を含み、「推定した意味: …」の組で出す。
 * 他の状態の応答は意味を持たない (`backend/core/observation_kind.go` の `validateMeaning`)。
 */
export function ObservationKindView({
  observationKind,
}: {
  observationKind: ObservationKind;
}) {
  if (observationKind.raw.length === 0) {
    return <MissingValue description={observationFieldsAbsent} />;
  }
  return (
    <>
      <ObservationFieldList fields={observationKind.raw} />
      {observationKind.status === undefined ? (
        <MissingValue description={observationStatusAbsent} />
      ) : (
        `意味: ${observationStatusLabels[observationKind.status]}`
      )}
      {observationKind.meaning === undefined ? null : (
        <span className="block">
          <Hint text={inferredMeaningDescription}>
            推定した意味: <RawText text={observationKind.meaning} />
          </Hint>
        </span>
      )}
    </>
  );
}
