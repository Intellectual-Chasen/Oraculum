import { Clock3, FileText } from "lucide-react";
import { memo, type RefObject } from "react";
import type { RecordLocator, Timestamp } from "@/shared/contracts/common";
import type { GraphNode } from "@/shared/contracts/graph";
import {
  type TimelineEntry,
  timelineEntryKey,
} from "@/shared/contracts/timeline";
import { nodeKindLabels } from "@/shared/lib/graphLabels";
import { toVisibleRawText } from "@/shared/lib/rawText";
import {
  isCoarserThanSecond,
  timestampClockLabels,
  timestampPrecisionLabels,
} from "@/shared/lib/recordLabels";
import {
  describeRecordLocation,
  recordPositionParts,
} from "@/shared/lib/recordPosition";
import {
  type ContextMenuTriggers,
  RowMenuButton,
} from "@/shared/ui/ContextMenu";
import { DerivedLabelNote } from "@/shared/ui/DerivedLabelNote";
import { Highlighted } from "@/shared/ui/Highlighted";
import { Hint } from "@/shared/ui/Hint";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { MissingValue } from "@/shared/ui/MissingValue";
import { ObservationKindView } from "@/shared/ui/ObservationKindView";
import { RawText } from "@/shared/ui/RawText";
import { TimestampOffsetNote } from "@/shared/ui/TimestampOffsetNote";
import { RawTimestampText } from "@/shared/ui/TimestampText";
import { ValueLink } from "@/shared/ui/ValueLink";
import {
  SpacerBody,
  stickyHeaderCellStyle,
  virtualCellStyle,
  virtualTableStyle,
} from "@/shared/ui/virtualTable";
import { timelineColumnLabels } from "./labels";

const terminalAbsent = "端末なし";
const accountAbsent = "アカウントなし";
const timeAbsent = "時刻なし";

function accountRoleLabel(role: string): string {
  switch (role) {
    case "record_subject_account":
      return "操作主体";
    case "record_target_account":
      return "操作対象";
    default:
      return "名前の記録";
  }
}

/** 列の見出しと、表の幅に対する列の幅の比。 */
const recordColumns = [
  { label: timelineColumnLabels.eventTime, width: "16%" },
  { label: timelineColumnLabels.terminal, width: "15%" },
  { label: timelineColumnLabels.account, width: "14%" },
  { label: timelineColumnLabels.observationKind, width: "13%" },
  { label: timelineColumnLabels.clock, width: "12%" },
  { label: timelineColumnLabels.record, width: "17%" },
  { label: "操作", width: "13%" },
];

/**
 * 時刻の原文を出す。秒より粗い精度だけを「精度: 分」の組で出し、ほかの精度は tooltip に入れる。
 *
 * **正規化した値でなく原文の文字列を出す。** 分析者が原文のどこを読むかは文字列で決まる。
 */
function EventTimeCell({ timestamp }: { timestamp: Timestamp | undefined }) {
  if (timestamp === undefined) {
    return <MissingValue description={timeAbsent} />;
  }
  const precision = `精度: ${timestampPrecisionLabels[timestamp.precision]}`;
  return (
    <>
      <Hint text={precision}>
        <RawTimestampText timestamp={timestamp} absence="原文なし" />
      </Hint>
      {isCoarserThanSecond(timestamp.precision) ? (
        <span className="block">{precision}</span>
      ) : null}
      <TimestampOffsetNote timestamp={timestamp} />
    </>
  );
}

/**
 * ノードの表示名と識別鍵の値を出す。
 *
 * **表示名に識別鍵の値を添える。** 同じ表示名を持つ別のノードが同じ列に並ぶため、
 * 表示名だけでは 2 つの行がどの対象を指すかを読めない。
 */
function NodeCell({
  node,
  absence,
}: {
  node: GraphNode | undefined;
  absence: string;
}) {
  if (node === undefined) {
    return <MissingValue description={absence} />;
  }
  // 導いた表示名は原文の文字列を持たず、導いた値だけを持つ。導いたことと導き方を添え、原文の
  // 文字列と読み分けられるようにする。
  const label = node.label.rawText ?? node.label.normalized;
  return (
    <>
      {label === undefined ? (
        <MissingValue description="表示名なし" />
      ) : (
        <ValueLink
          target={{ kind: "node", id: node.id, label }}
          hover={
            <KeyValueList
              stacked
              pairs={[
                { name: "種類", value: nodeKindLabels[node.kind] },
                ...node.identity.map((value, index) => ({
                  key: `identity-${index}`,
                  name: "識別",
                  value: toVisibleRawText(value.value),
                })),
              ]}
            />
          }
        >
          <Highlighted text={label} />
        </ValueLink>
      )}
      {label !== undefined && node.label.valueState === "derived" ? (
        <>
          {" "}
          <DerivedLabelNote derivation={node.label.derivation} />
        </>
      ) : null}
      <ul>
        {node.identity.map((value) => (
          <li key={JSON.stringify([value.semantic ?? null, value.value])}>
            <Highlighted
              text={value.value}
              field={{ name: value.semantic ?? "", semantic: value.semantic }}
            />
          </li>
        ))}
      </ul>
    </>
  );
}

/**
 * レコードを収集元と位置の組で出す。click で Record に表示する。
 *
 * **時刻の種類は収集元と組で読む。** 同じ `terminal_local` でも、収集元が違えば別の端末の時刻で
 * ある。収集元はこの列に出す。
 */
function RecordCell({ entry }: { entry: TimelineEntry }) {
  const { recordRef } = entry;
  const pairs = [
    { name: "収集元", value: <RawText text={recordRef.sourceFileName} /> },
    ...recordPositionParts(recordRef).map((part) => ({
      name: part.name,
      value: part.value,
    })),
  ];
  return (
    <ValueLink
      target={{ kind: "record", ref: recordRef }}
      label={toVisibleRawText(describeRecordLocation(recordRef))}
      hover={
        <KeyValueList
          stacked
          pairs={[
            ...pairs,
            {
              name: "時刻",
              value:
                entry.eventTime?.rawText === undefined
                  ? undefined
                  : toVisibleRawText(entry.eventTime.rawText),
            },
          ]}
        />
      }
    >
      {/* button の中に置くため、ul でなく span の組で並べる。 */}
      {pairs.map((pair) => (
        <span key={pair.name} className="block text-left">
          <span className="pair-name">{pair.name}:</span> {pair.value}
        </span>
      ))}
    </ValueLink>
  );
}

/**
 * レコード 1 件の行。
 * スクロールで行を入れ替えるたびに、残る行を描き直さないよう memo にする。
 */
const TimelineRecordRow = memo(function TimelineRecordRow({
  entry,
  rowIndex,
  onSelectRecord,
  focused,
  menu,
  isMenuOpen,
  onBefore,
}: {
  entry: TimelineEntry;
  rowIndex: number;
  onSelectRecord: (recordRef: RecordLocator) => void;
  /** 探した文字列に一致して見せている行か。 */
  focused: boolean;
  /** 行の操作のメニューを開く操作。一覧が 1 組を持ち、描画をまたいで同じ組を渡す。 */
  menu: ContextMenuTriggers<TimelineEntry>;
  isMenuOpen: boolean;
  onBefore?: (entry: TimelineEntry) => void;
}) {
  return (
    <tr
      data-row-index={rowIndex}
      aria-rowindex={rowIndex + 2}
      aria-current={focused ? "true" : undefined}
      className={focused ? "timeline-row-focused" : undefined}
      onContextMenu={(event) => menu.onContextMenu(event, entry)}
      onKeyDown={(event) => menu.onKeyDown(event, entry)}
    >
      <td style={virtualCellStyle}>
        <EventTimeCell timestamp={entry.eventTime} />
        {entry.timeFieldName === undefined ? null : (
          <span className="block">
            フィールド: <RawText text={entry.timeFieldName} />
          </span>
        )}
      </td>
      <td style={virtualCellStyle}>
        <NodeCell node={entry.terminal} absence={terminalAbsent} />
      </td>
      <td style={virtualCellStyle}>
        <NodeCell node={entry.account} absence={accountAbsent} />
        {entry.accountRoles?.map((role) => (
          <span className="block" key={role}>
            このアカウント: {accountRoleLabel(role)}
          </span>
        ))}
        {entry.otherAccounts?.map(({ role, node }) => (
          <span className="block" key={`${role}:${node.id}`}>
            {accountRoleLabel(role)}:{" "}
            <NodeCell node={node} absence="相手なし" />
          </span>
        ))}
      </td>
      <td style={virtualCellStyle}>
        <ObservationKindView observationKind={entry.observationKind} />
        {/* 事象の組は、アカウントを起点にした時系列の行にだけ書く。 */}
        {entry.accountRoles === undefined ||
        entry.eventKind === undefined ? null : (
          <span className="block">
            事象:{" "}
            <RawText
              text={`${entry.eventKind.category} ${entry.eventKind.action}`}
            />
          </span>
        )}
        {entry.sourceAddress === undefined ? null : (
          <span className="block">
            接続元: <RawText text={entry.sourceAddress} />
          </span>
        )}
      </td>
      <td style={virtualCellStyle}>
        {entry.eventTime === undefined ? (
          <MissingValue description={timeAbsent} />
        ) : (
          timestampClockLabels[entry.eventTime.clock]
        )}
      </td>
      <td style={virtualCellStyle}>
        <RecordCell entry={entry} />
      </td>
      <td style={virtualCellStyle}>
        <IconButton
          label="Record に表示"
          onPress={() => onSelectRecord(entry.recordRef)}
        >
          <FileText size={14} aria-hidden="true" />
        </IconButton>
        {onBefore === undefined ||
        entry.eventTime?.normalizedForm !== "rfc3339_absolute" ||
        entry.eventTime.normalized === undefined ||
        (entry.eventTime.precision !== "second" &&
          entry.eventTime.precision !== "millisecond" &&
          entry.eventTime.precision !== "microsecond") ? null : (
          <IconButton label="この時刻以前" onPress={() => onBefore(entry)}>
            <Clock3 size={14} aria-hidden="true" />
          </IconButton>
        )}
        <RowMenuButton
          label={`${toVisibleRawText(describeRecordLocation(entry.recordRef))} の操作`}
          expanded={isMenuOpen}
          onClick={(event) => menu.onButtonClick(event, entry)}
        />
      </td>
    </tr>
  );
});

type TimelineRecordTableProps = {
  entries: readonly TimelineEntry[];
  /** 描く行の範囲。`end` を含まない。 */
  start: number;
  end: number;
  spaceBefore: number;
  spaceAfter: number;
  bodyRef: RefObject<HTMLTableSectionElement | null>;
  onSelectRecord: (recordRef: RecordLocator) => void;
  /** 行の操作のメニューを開く操作。 */
  rowMenu: ContextMenuTriggers<TimelineEntry>;
  /** メニューを開いている行。 */
  menuEntry: TimelineEntry | undefined;
  /** 強調する行の位置。出ない場合は強調しない。 */
  focusedEntry?: number | undefined;
  onBefore?: (entry: TimelineEntry) => void;
};

/**
 * 根拠のレコードを 1 件ずつの行で、時刻順に出す。
 * 描くのは `start` から `end` の前までの行で、他の行の高さは空きで保つ。
 */
export function TimelineRecordTable({
  entries,
  start,
  end,
  spaceBefore,
  spaceAfter,
  bodyRef,
  onSelectRecord,
  rowMenu,
  menuEntry,
  focusedEntry,
  onBefore,
}: TimelineRecordTableProps) {
  return (
    <table style={virtualTableStyle} aria-rowcount={entries.length + 1}>
      <caption>時刻順のレコード</caption>
      <colgroup>
        {recordColumns.map((column) => (
          <col key={column.label} style={{ width: column.width }} />
        ))}
      </colgroup>
      <thead>
        <tr aria-rowindex={1}>
          {recordColumns.map((column) => (
            <th key={column.label} scope="col" style={stickyHeaderCellStyle}>
              {column.label}
            </th>
          ))}
        </tr>
      </thead>
      <SpacerBody height={spaceBefore} columnCount={recordColumns.length} />
      <tbody ref={bodyRef}>
        {entries.slice(start, end).map((entry, offset) => (
          <TimelineRecordRow
            key={timelineEntryKey(entry)}
            entry={entry}
            rowIndex={start + offset}
            onSelectRecord={onSelectRecord}
            focused={start + offset === focusedEntry}
            menu={rowMenu}
            isMenuOpen={entry === menuEntry}
            onBefore={onBefore}
          />
        ))}
      </tbody>
      <SpacerBody height={spaceAfter} columnCount={recordColumns.length} />
    </table>
  );
}
