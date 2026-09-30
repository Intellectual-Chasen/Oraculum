import { ZoomIn } from "lucide-react";
import { memo, type RefObject } from "react";
import { formatCount } from "@/shared/lib/format";
import { toVisibleRawText } from "@/shared/lib/rawText";
import { timestampPrecisionLabels } from "@/shared/lib/recordLabels";
import { msRangeTimeFilter } from "@/shared/lib/timeFilter";
import { localTextOf } from "@/shared/lib/timestampInstant";
import { periodText } from "@/shared/lib/valueCondition";
import { useDisplayOffset } from "@/shared/ui/DisplayOffset";
import { Hint } from "@/shared/ui/Hint";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import {
  offsetUnknownDescription,
  offsetUnknownLabel,
} from "@/shared/ui/TimestampOffsetNote";
import {
  useValueMenu,
  type ValueMenuActions,
  ValueMenuButton,
} from "@/shared/ui/ValueMenu";
import {
  SpacerBody,
  stickyHeaderCellStyle,
  virtualCellStyle,
  virtualTableStyle,
} from "@/shared/ui/virtualTable";
import {
  coarserThanBucketLabel,
  timelineGroupColumnLabels,
  timeUnreadableLabel,
} from "./labels";
import type { TimelineSources } from "./sourceLabels";
import type { TimelineGroup } from "./timelineGrouping";

/** 列の見出しと、表の幅に対する列の幅の比。 */
const groupColumns = [
  { label: timelineGroupColumnLabels.period, width: "34%" },
  { label: timelineGroupColumnLabels.count, width: "10%" },
  { label: timelineGroupColumnLabels.sources, width: "44%" },
  { label: "操作", width: "12%" },
];

/**
 * UTC の epoch からの ms を、UTC の日付と時刻の文字列で書く。
 * ms の欄は 0 でないときだけ書く。
 */
function formatUtcTime(ms: number): string {
  const iso = new Date(ms).toISOString();
  const withoutZone = iso.slice(0, -1).replace("T", " ");
  return withoutZone.endsWith(".000") ? withoutZone.slice(0, -4) : withoutZone;
}

/**
 * 期間を UTC の「始まり – 終わり」で書く。終わりは期間に含まない。表示のタイムゾーン
 * `offset` を選んでいるときは、そのタイムゾーンの期間を title に入れる。
 */
function GroupRange({
  range,
  offset,
  actions,
}: {
  range: { startMs: number; endMs: number };
  offset: string;
  actions: ValueMenuActions | undefined;
}) {
  const local = (ms: number) => localTextOf(new Date(ms).toISOString(), offset);
  const [from, to] = [local(range.startMs), local(range.endMs)];
  const text = `${formatUtcTime(range.startMs)} – ${formatUtcTime(range.endMs)}`;
  // 期間の条件とコピーは区切りの ms から作る。表示の文字列は ms の桁を省くため読み直さない。
  const filter = msRangeTimeFilter(range.startMs, range.endMs);
  return (
    <ValueMenuButton
      className="font-mono"
      hover={
        from === undefined || to === undefined
          ? timelineGroupColumnLabels.period
          : `UTC${offset}: ${from} – ${to}`
      }
      target={{
        key: `${range.startMs}-${range.endMs}`,
        name: timelineGroupColumnLabels.period,
        text,
        conditions: [{ kind: "period", filter }],
        copies: [{ what: "期間", text: periodText(filter) }],
      }}
      actions={actions}
    >
      {text}
    </ValueMenuButton>
  );
}

/** 行の期間を出す。区切りに入れなかったレコードの行には、別の行に置いた理由のラベルを出す。 */
function GroupPeriod({
  group,
  actions,
}: {
  group: TimelineGroup;
  actions: ValueMenuActions | undefined;
}) {
  const offset = useDisplayOffset();
  switch (group.kind) {
    case "bucket":
      return (
        <GroupRange range={group.range} offset={offset} actions={actions} />
      );
    case "coarser_than_bucket":
      return (
        <>
          <GroupRange range={group.range} offset={offset} actions={actions} />
          <br />
          <strong>
            <Hint text={`精度: ${timestampPrecisionLabels[group.precision]}`}>
              {coarserThanBucketLabel}
            </Hint>
          </strong>
        </>
      );
    case "offset_undetermined":
      return (
        <strong>
          <Hint text={offsetUnknownDescription}>{offsetUnknownLabel}</Hint>
        </strong>
      );
    case "time_unreadable":
      return <strong>{timeUnreadableLabel}</strong>;
    default: {
      const unknown: never = group;
      throw new Error(`unknown timeline group: ${String(unknown)}`);
    }
  }
}

/** 行に入れたレコードの件数を、収集元ごとに「収集元: 件数」の組で出す。 */
function SourceBreakdown({
  group,
  sources,
}: {
  group: TimelineGroup;
  sources: TimelineSources;
}) {
  return (
    <KeyValueList
      stacked
      pairs={sources.order.flatMap((sourceId) => {
        const count = group.sourceCounts.get(sourceId);
        const label = sources.labels.get(sourceId);
        if (count === undefined || label === undefined) {
          return [];
        }
        const name = toVisibleRawText(label.fileName);
        return [
          {
            key: sourceId,
            name:
              label.distinguisher === undefined
                ? name
                : `${name} ${label.distinguisher}`,
            value: formatCount(count),
          },
        ];
      })}
    />
  );
}

/**
 * 区切りの段階の表の 1 行。
 * スクロールで行を入れ替えるたびに、残る行を描き直さないよう memo にする。
 */
const TimelineGroupRow = memo(function TimelineGroupRow({
  group,
  rowIndex,
  sources,
  zoomLabel,
  onZoom,
  actions,
}: {
  group: TimelineGroup;
  rowIndex: number;
  sources: TimelineSources;
  zoomLabel: string;
  onZoom: (group: TimelineGroup) => void;
  actions: ValueMenuActions | undefined;
}) {
  return (
    <tr data-row-index={rowIndex} aria-rowindex={rowIndex + 2}>
      <td style={virtualCellStyle}>
        <GroupPeriod group={group} actions={actions} />
      </td>
      <td style={virtualCellStyle} className="text-right tabular-nums">
        {formatCount(group.entryCount)}
      </td>
      <td style={virtualCellStyle}>
        <SourceBreakdown group={group} sources={sources} />
      </td>
      <td style={virtualCellStyle}>
        <IconButton label={zoomLabel} onPress={() => onZoom(group)}>
          <ZoomIn size={14} aria-hidden="true" />
        </IconButton>
      </td>
    </tr>
  );
});

type TimelineGroupTableProps = {
  groups: readonly TimelineGroup[];
  sources: TimelineSources;
  /** 区切りの行の拡大のボタンの文字列。 */
  bucketZoomLabel: string;
  /** 区切りと別の行の拡大のボタンの文字列。 */
  separateZoomLabel: string;
  /** 行の拡大のボタンを押したときに、その行を渡す。 */
  onZoom: (group: TimelineGroup) => void;
  /** 描く行の範囲。`end` を含まない。 */
  start: number;
  end: number;
  spaceBefore: number;
  spaceAfter: number;
  bodyRef: RefObject<HTMLTableSectionElement | null>;
};

/**
 * 時間の区切りごとに、件数と収集元ごとの内訳を 1 行で出す。
 * 描くのは `start` から `end` の前までの行で、他の行の高さは空きで保つ。
 */
export function TimelineGroupTable({
  groups,
  sources,
  bucketZoomLabel,
  separateZoomLabel,
  onZoom,
  start,
  end,
  spaceBefore,
  spaceAfter,
  bodyRef,
}: TimelineGroupTableProps) {
  const valueMenu = useValueMenu();
  return (
    <>
      <table style={virtualTableStyle} aria-rowcount={groups.length + 1}>
        <caption>時間の区切りごとのレコード</caption>
        <colgroup>
          {groupColumns.map((column) => (
            <col key={column.label} style={{ width: column.width }} />
          ))}
        </colgroup>
        <thead>
          <tr aria-rowindex={1}>
            {groupColumns.map((column) => (
              <th key={column.label} scope="col" style={stickyHeaderCellStyle}>
                {column.label}
              </th>
            ))}
          </tr>
        </thead>
        <SpacerBody height={spaceBefore} columnCount={groupColumns.length} />
        <tbody ref={bodyRef}>
          {groups.slice(start, end).map((group, offset) => (
            <TimelineGroupRow
              key={group.key}
              group={group}
              rowIndex={start + offset}
              sources={sources}
              zoomLabel={
                group.kind === "bucket" ? bucketZoomLabel : separateZoomLabel
              }
              onZoom={onZoom}
              actions={valueMenu.actions}
            />
          ))}
        </tbody>
        <SpacerBody height={spaceAfter} columnCount={groupColumns.length} />
      </table>
      {valueMenu.menu}
    </>
  );
}
