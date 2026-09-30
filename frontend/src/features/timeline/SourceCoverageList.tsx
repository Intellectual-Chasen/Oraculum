import type { Timestamp } from "@/shared/contracts/common";
import type { SourceCoverage } from "@/shared/contracts/timeline";
import { formatCount } from "@/shared/lib/format";
import { localTextOf, utcTextOf } from "@/shared/lib/timestampInstant";
import { DataTable } from "@/shared/ui/DataTable";
import { useDisplayOffset } from "@/shared/ui/DisplayOffset";
import { Hint } from "@/shared/ui/Hint";
import { MissingValue } from "@/shared/ui/MissingValue";
import { RawText } from "@/shared/ui/RawText";
import { InterpretationNote } from "@/shared/ui/TimestampOffsetNote";
import { RawTimestampText } from "@/shared/ui/TimestampText";
import { coverageStateDescriptions, coverageStateLabels } from "./labels";

function RangeBound({ timestamp }: { timestamp: Timestamp | undefined }) {
  return timestamp === undefined ? (
    <MissingValue description="UTC 時刻なし" />
  ) : (
    <RawTimestampText timestamp={timestamp} absence="原文なし" />
  );
}

/**
 * 記録期間を「最初 – 最後」で出す。与えたタイムゾーンと、表示のタイムゾーンの期間を次の行に
 * 「名前: 値」の組で出す。
 */
function RecordingRange({ coverage }: { coverage: SourceCoverage }) {
  const offset = useDisplayOffset();
  const first = coverage.observedRangeFirst;
  const last = coverage.observedRangeLast;
  if (first === undefined && last === undefined) {
    return <MissingValue description="記録期間なし" />;
  }
  const interpretation = first?.interpretation ?? last?.interpretation;
  const local = (timestamp: Timestamp | undefined) => {
    const utc = timestamp === undefined ? undefined : utcTextOf(timestamp);
    return utc === undefined ? undefined : localTextOf(utc, offset);
  };
  const [localFirst, localLast] = [local(first), local(last)];
  return (
    <>
      <RangeBound timestamp={first} /> – <RangeBound timestamp={last} />
      {interpretation === undefined ? null : (
        <InterpretationNote interpretation={interpretation} />
      )}
      {localFirst === undefined || localLast === undefined ? null : (
        <span className="block">{`UTC${offset}: ${localFirst} – ${localLast}`}</span>
      )}
    </>
  );
}

type SourceCoverageListProps = {
  coverages: SourceCoverage[];
};

/**
 * 収集元ごとの記録期間と、指定した期間との重なり方と、フィルタに一致した件数を並べる。
 *
 * **件数を必ず出す。** 状態で件数を伏せると、記録期間の外と判定された収集元が実際には
 * レコードを返している状態を読めない。記録期間の母集団は解析に失敗したレコードを含み、
 * 件数の母集団は取り込みに成功したレコードだけであるため、2 つは食い違いうる。
 *
 * **0 件の意味は重なりの列で読む。** 「記録期間内」で 0 件ならイベントが無く、「記録期間外」と
 * 「記録期間不明」の 0 件は記録が無いか、記録の有無を決められない。
 */
export function SourceCoverageList({ coverages }: SourceCoverageListProps) {
  if (coverages.length === 0) {
    return <p>収集元なし</p>;
  }
  return (
    <DataTable
      label="収集元ごとの記録期間"
      showCaption
      rows={coverages}
      rowKey={(coverage) => coverage.sourceId}
      columns={[
        {
          key: "source",
          header: "収集元",
          rowHeader: true,
          cell: (coverage) => <RawText text={coverage.sourceFileName} />,
        },
        {
          key: "range",
          header: "記録期間",
          mono: true,
          cell: (coverage) => <RecordingRange coverage={coverage} />,
        },
        {
          key: "state",
          header: "期間との重なり",
          cell: (coverage) => {
            const description = coverageStateDescriptions[coverage.state];
            return description === undefined ? (
              coverageStateLabels[coverage.state]
            ) : (
              <Hint text={description}>
                {coverageStateLabels[coverage.state]}
              </Hint>
            );
          },
        },
        {
          key: "records",
          header: "一致したレコード",
          numeric: true,
          cell: (coverage) => formatCount(coverage.matchedRecordCount),
        },
        {
          // イベントの時刻を複数持つレコードは時系列に複数の行を持つ。
          key: "rows",
          header: "行",
          numeric: true,
          cell: (coverage) => formatCount(coverage.matchedRowCount),
        },
      ]}
    />
  );
}
