import { Clock3 } from "lucide-react";
import type { CSSProperties, ReactNode } from "react";
import {
  type EventKindPair,
  type ObservationKind,
  offsetCarriesInstant,
  type RecordField,
  type Timestamp,
  type TimestampInterpretation,
} from "../contracts/common";
import type { GraphEvidence } from "../contracts/graph";
import { toVisibleRawText } from "../lib/rawText";
import { readRecordFieldRawText } from "../lib/recordField";
import {
  isCoarserThanSecond,
  observationStatusLabels,
  timestampPrecisionLabels,
} from "../lib/recordLabels";
import {
  describeRecordLocation,
  recordPositionAbsent,
  recordPositionValue,
  recordRefKey,
} from "../lib/recordPosition";
import { contextTimeFilter } from "../lib/timeFilter";
import { localTextOf, utcTextOf } from "../lib/timestampInstant";
import { fieldConditions, type ValueCondition } from "../lib/valueCondition";
import { useDisplayOffset } from "./DisplayOffset";
import { Highlighted, PeriodMark } from "./Highlighted";
import { Hint } from "./Hint";
import { MissingValue } from "./MissingValue";
import {
  inferredMeaningDescription,
  observationStatusAbsent,
} from "./ObservationKindView";
import { RawText } from "./RawText";
import {
  interpretationOriginLabel,
  offsetUnknownDescription,
  offsetUnknownLabel,
} from "./TimestampOffsetNote";
import { ValueLink } from "./ValueLink";
import { type ValueMenuActions, ValueMenuButton } from "./ValueMenu";

/** イベントの種類のうち、Event ID の列に表示するフィールドの名前。 */
const eventIdFieldPattern = /^event_?id$/i;

/** 根拠のレコードの表の列。見出しは短い名前にし、説明は見出しの tooltip に入れる。 */
const evidenceColumns = [
  { name: "Artifact", description: "収集元の file 名" },
  { name: "時刻", description: "イベントの UTC 時刻" },
  { name: "Event ID", description: "イベントの種類の Event ID" },
  {
    name: "位置",
    description:
      "収集元の中のレコードの位置\n位置: byte の始まり-終わり\nID: 収集元の中のレコードの ID\n行: 行番号",
  },
  { name: "原文の時刻", description: "時刻の原文" },
  {
    name: "イベントの種類",
    description: "Event ID 以外のイベントの種類",
  },
] as const;

const terminalColumn = { name: "端末", description: "レコードを記録した端末" };

/** 根拠のレコードの表の列の数。`showsTerminal` のときは端末の列を足した数である。 */
export function evidenceColumnCount(showsTerminal = false): number {
  return evidenceColumns.length + (showsTerminal ? 1 : 0);
}

/** 根拠のレコードの表の見出しの行。 */
export function EvidenceTableHead({
  cellStyle,
  showsTerminal = false,
}: {
  /** 見出しの欄の見た目。スクロールする表では上端に留める指定を渡す。 */
  cellStyle?: CSSProperties;
  /** 端末の列を足す。 */
  showsTerminal?: boolean;
}) {
  const columns = showsTerminal
    ? [...evidenceColumns, terminalColumn]
    : evidenceColumns;
  return (
    <thead>
      <tr aria-rowindex={1}>
        {columns.map((column) => (
          <th key={column.name} scope="col" style={cellStyle}>
            <Hint text={column.description}>{column.name}</Hint>
          </th>
        ))}
      </tr>
    </thead>
  );
}

/**
 * 観測の種別のフィールド 1 つの値。押すと値のメニューを開く。レコードがイベントの種類の組を
 * 持つときは、その組の条件を先にする。文字列を持たないときは持たない理由を出す。
 */
function FieldValueView({
  rowKey,
  position,
  field,
  eventKind,
  actions,
}: {
  rowKey: string;
  /** 観測の種別の中のフィールドの並びの位置。同じ名前と値のフィールドを見分ける。 */
  position: number;
  field: RecordField;
  /** backend がレコードに付けたイベントの種類の組。 */
  eventKind: EventKindPair | undefined;
  actions: ValueMenuActions | undefined;
}) {
  const value = readRecordFieldRawText(field);
  if (!("text" in value)) {
    return <MissingValue description={value.absence} />;
  }
  return (
    <ValueMenuButton
      className="font-mono"
      target={{
        key: `${rowKey}\u0000${position}\u0000${field.name}\u0000${value.text}`,
        name: field.name,
        text: value.text,
        conditions: [
          ...(eventKind === undefined
            ? []
            : [{ kind: "eventKind" as const, ...eventKind }]),
          ...fieldConditions(field),
        ],
        copies: [{ what: "原文の文字列", text: value.text }],
      }}
      actions={actions}
    >
      <Highlighted text={value.text} field={field} />
    </ValueMenuButton>
  );
}

/**
 * イベントの種類の意味の状態を、推定と不明のときだけ「意味: 推定」の組で出す。確定した状態は
 * 呼び出し側が title に入れる。イベントの種類のフィールドを持つのに状態が無いときは、無い印を出す
 * (ObservationKindView と同じ扱い)。
 */
function ObservationStatusMark({
  observationKind,
}: {
  observationKind: ObservationKind;
}) {
  const { raw, status } = observationKind;
  if (status === undefined) {
    return raw.length === 0 ? null : (
      <MissingValue description={observationStatusAbsent} />
    );
  }
  return status === "determined" ? null : (
    <span className="text-xs text-muted">
      {`意味: ${observationStatusLabels[status]}`}
    </span>
  );
}

/** タイムゾーンを与えた人の短い名前。分析者の記録は「分析者」、取り込みの起動の指定は「指定」である。 */
function interpretationShortLabel(interpretation: TimestampInterpretation) {
  return interpretation.assertionId === undefined ? "指定" : "分析者";
}

/** 時刻の UTC と、表示のタイムゾーンの時刻と、タイムゾーンの出どころを、名前と値の組で返す。 */
function timeFacts(time: Timestamp | undefined, offset: string): string[] {
  if (time === undefined) return [];
  const utc = utcTextOf(time);
  const local = utc === undefined ? undefined : localTextOf(utc, offset);
  return [
    ...(utc === undefined ? [] : [`UTC: ${utc}`]),
    ...(local === undefined ? [] : [`UTC${offset}: ${local}`]),
    ...(time.interpretation === undefined
      ? []
      : [
          `${interpretationOriginLabel(time.interpretation)}: UTC${time.interpretation.offset}`,
        ]),
    ...(time.rawText === undefined
      ? []
      : [`原文: ${toVisibleRawText(time.rawText)}`]),
    `精度: ${timestampPrecisionLabels[time.precision]}`,
  ];
}

/**
 * 時刻を UTC の 1 行で出す。精度は秒より粗いときだけ添える。UTC に直せない時刻は原文と
 * 「タイムゾーン不明」の印を出す。
 *
 * **与えたタイムゾーン (分析者の記録と取り込みの起動の指定) から求めた UTC には、画面の文字で
 * 印を付ける。** 原文が UTC を持つ時刻と同じ見た目にすると、keyboard と読み上げの利用者が
 * 読み分けられない。
 */
function TimeCell({
  time,
  offset,
  rowKey,
  actions,
}: {
  time: Timestamp | undefined;
  offset: string;
  rowKey: string;
  actions: ValueMenuActions | undefined;
}) {
  if (time === undefined) {
    return <MissingValue description="時刻なし" />;
  }
  const utc = utcTextOf(time);
  const title = timeFacts(time, offset).join("\n");
  // 精度は title に入れ、秒より粗いときだけ画面の文字でも出す。
  const precision = isCoarserThanSecond(time.precision) ? (
    <span className="text-xs text-muted">
      {`精度: ${timestampPrecisionLabels[time.precision]}`}
    </span>
  ) : null;
  if (utc !== undefined) {
    // 期間の条件は時刻の値から作る。表示の文字列 (T と Z を外した UTC) を読み直さない。
    const filter = contextTimeFilter(time, 0);
    const conditions: ValueCondition[] =
      filter === undefined ? [] : [{ kind: "period", filter }];
    return (
      <span className="inline-flex items-center gap-2">
        <ValueMenuButton
          className="font-mono tabular-nums"
          hover={title}
          target={{
            key: `${rowKey}\u0000time`,
            name: "時刻",
            text: utc,
            conditions,
            copies: [
              { what: "UTC 時刻", text: utc },
              ...(time.rawText === undefined
                ? []
                : [{ what: "時刻の原文", text: time.rawText }]),
            ],
          }}
          actions={actions}
        >
          <PeriodMark timestamp={time}>
            {utc.replace("T", " ").replace(/Z$/, "")}
          </PeriodMark>
        </ValueMenuButton>
        {time.interpretation === undefined ? null : (
          <Hint
            className="inline-flex items-center gap-0.5 text-xs"
            text={`${interpretationOriginLabel(time.interpretation)}: UTC${time.interpretation.offset}`}
          >
            <Clock3 className="size-3" aria-hidden="true" />
            <span>{`${interpretationShortLabel(time.interpretation)}: UTC${time.interpretation.offset}`}</span>
          </Hint>
        )}
        {precision}
      </span>
    );
  }
  if (time.rawText === undefined) {
    return <MissingValue description="原文なし" />;
  }
  return (
    <span className="inline-flex items-center gap-2">
      <Hint text={title}>
        <PeriodMark timestamp={time}>
          <RawText text={time.rawText} />
        </PeriodMark>
      </Hint>
      {offsetCarriesInstant(time.offsetState) ? null : (
        <strong className="text-xs">
          <Hint text={offsetUnknownDescription}>{offsetUnknownLabel}</Hint>
        </strong>
      )}
      {precision}
    </span>
  );
}

/**
 * 根拠のレコード 1 件を 1 行で出す。値を 2 行目へ回さず、表が区画に収まらないときは表を横に
 * スクロールさせる。
 *
 * - 位置を押すと Record にレコードを出す (ValueLink)。hover で収集元・位置・時刻を出す。
 * - Event ID と観測の種別の値を押すと、条件に追加するメニューを開く。hover でフィールドの名前を出す。
 *
 * `rowIndex` を渡すと、行に `data-row-index` と、見出しの行を 1 行目とした `aria-rowindex` を
 * 付ける。行を入れ替える表が行の高さを測る。
 */
export function EvidenceRecordRow({
  evidence,
  termActions,
  rowIndex,
  terminal,
  current = false,
}: {
  evidence: GraphEvidence;
  /** 値のメニューを開く操作。出ない場合は値を押せない文字列で出す。 */
  termActions: ValueMenuActions | undefined;
  rowIndex?: number;
  /** 端末の列の中身。渡すと端末の列を出す。 */
  terminal?: ReactNode;
  /** 開いているレコードの行であるか。 */
  current?: boolean;
}) {
  const offset = useDisplayOffset();
  const { recordRef } = evidence;
  const location = toVisibleRawText(describeRecordLocation(recordRef));
  const rowKey = recordRefKey(recordRef);
  const { observationKind } = evidence;
  const eventId = observationKind.raw.find((field) =>
    eventIdFieldPattern.test(field.name),
  );
  // Event ID の列に出したフィールドは、観測の種別の列に重ねて出さない。
  const otherObservation = observationKind.raw.filter(
    (field) => field !== eventId,
  );
  const time = evidence.eventTime;
  const recordFacts = [location, ...timeFacts(time, offset)].join("\n");
  return (
    <tr
      data-row-index={rowIndex}
      aria-rowindex={rowIndex === undefined ? undefined : rowIndex + 2}
      aria-current={current ? "true" : undefined}
    >
      <td>
        <Hint text={recordFacts}>
          <RawText text={recordRef.sourceFileName} />
        </Hint>
      </td>
      <td>
        <TimeCell
          time={time}
          offset={offset}
          rowKey={rowKey}
          actions={termActions}
        />
      </td>
      <td>
        {eventId === undefined ? (
          <MissingValue description="Event ID なし" />
        ) : (
          <FieldValueView
            rowKey={rowKey}
            position={observationKind.raw.indexOf(eventId)}
            field={eventId}
            eventKind={evidence.eventKind}
            actions={termActions}
          />
        )}
      </td>
      <td className="font-mono">
        <ValueLink
          target={{ kind: "record", ref: recordRef }}
          label={`Record に表示: ${location}`}
          hover={<span className="whitespace-pre-line">{recordFacts}</span>}
        >
          {recordPositionValue(recordRef) ?? recordPositionAbsent}
        </ValueLink>
      </td>
      <td>
        {time?.rawText === undefined ? (
          <MissingValue description="原文なし" />
        ) : (
          <PeriodMark timestamp={time}>
            <RawText text={time.rawText} />
          </PeriodMark>
        )}
      </td>
      <td>
        <span
          className="inline-flex items-center gap-2"
          // 意味が確定した状態は画面の文字にせず、title にだけ入れる。
          title={
            observationKind.status === "determined"
              ? `意味: ${observationStatusLabels.determined}`
              : undefined
          }
        >
          {otherObservation.length === 0 ? (
            <MissingValue description="値なし" />
          ) : (
            otherObservation.map((field) => {
              const position = observationKind.raw.indexOf(field);
              return (
                <FieldValueView
                  key={`${position}\u0000${field.name}`}
                  rowKey={rowKey}
                  position={position}
                  field={field}
                  eventKind={evidence.eventKind}
                  actions={termActions}
                />
              );
            })
          )}
          <ObservationStatusMark observationKind={observationKind} />
          {observationKind.meaning === undefined ? null : (
            <Hint text={inferredMeaningDescription}>
              <RawText text={observationKind.meaning} />
            </Hint>
          )}
        </span>
      </td>
      {terminal === undefined ? null : <td>{terminal}</td>}
    </tr>
  );
}
