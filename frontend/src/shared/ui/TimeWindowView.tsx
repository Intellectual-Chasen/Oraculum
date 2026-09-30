import type { TimeWindow } from "../contracts/candidates";
import type { RequestedTime } from "../contracts/common";
import { formatCount } from "../lib/format";
import { windowKindLabels } from "../lib/matchLabels";
import { timestampPrecisionLabels } from "../lib/recordLabels";
import { DataTable } from "./DataTable";
import { KeyValueList } from "./KeyValueList";
import { MissingValue } from "./MissingValue";
import { RawText } from "./RawText";

const normalizedAbsent = "値を求められない";

type BoundRow = { name: string; time: RequestedTime };

/**
 * 段階がエッジの推定に使った時刻の範囲を出す。範囲の種類と幅を値の組で出し、下端・上端・中心を
 * 行、入力した文字列・使った値・精度・求め方を列にした表で出す。範囲の幅は利用者が与える引数である。
 *
 * 入力した文字列に対応する原文は無いため、原文の列を置かない (`backend/core/candidate.go`)。
 */
export function TimeWindowView({
  timeWindow,
  stageLabel,
}: {
  timeWindow: TimeWindow;
  /** 段階の名前。1 画面に 2 つ以上の表を置くため、表の名前にする。 */
  stageLabel: string;
}) {
  const { lowerBound, upperBound, centerTime, radiusSeconds } = timeWindow;
  const rows: BoundRow[] = [
    ...(lowerBound === undefined ? [] : [{ name: "下端", time: lowerBound }]),
    ...(upperBound === undefined ? [] : [{ name: "上端", time: upperBound }]),
    ...(centerTime === undefined ? [] : [{ name: "中心", time: centerTime }]),
  ];
  return (
    <>
      <KeyValueList
        pairs={[
          {
            name: "時刻の範囲",
            value: windowKindLabels[timeWindow.windowKind],
          },
          {
            name: "中心からの幅",
            value:
              radiusSeconds === undefined
                ? undefined
                : `± ${formatCount(radiusSeconds)} 秒`,
          },
        ]}
      />
      {rows.length === 0 ? null : (
        <DataTable
          label={stageLabel}
          rows={rows}
          rowKey={(row) => row.name}
          columns={[
            {
              key: "bound",
              header: "範囲",
              rowHeader: true,
              cell: (row) => row.name,
            },
            {
              key: "request",
              header: "入力",
              mono: true,
              cell: (row) => <RawText text={row.time.requestText} />,
            },
            {
              key: "normalized",
              header: "使った値",
              mono: true,
              cell: (row) =>
                row.time.normalized === undefined ? (
                  <MissingValue description={normalizedAbsent} />
                ) : (
                  <RawText text={row.time.normalized} />
                ),
            },
            {
              key: "precision",
              header: "精度",
              cell: (row) => timestampPrecisionLabels[row.time.precision],
            },
            {
              key: "derivation",
              header: "求め方",
              cell: (row) =>
                row.time.derivation === undefined ? (
                  <MissingValue description={normalizedAbsent} />
                ) : (
                  <RawText text={row.time.derivation} />
                ),
            },
          ]}
        />
      )}
    </>
  );
}
