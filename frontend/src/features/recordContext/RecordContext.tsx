import { useState } from "react";
import type { GraphTimeFilter } from "@/shared/api/graph";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import type {
  RawAndNormalized,
  RecordLocator,
  Timestamp,
} from "@/shared/contracts/common";
import {
  type PeriodUnjudged,
  type TimelineEntry,
  timelineEntryKey,
} from "@/shared/contracts/timeline";
import { formatCount } from "@/shared/lib/format";
import {
  isCoarserThanSecond,
  timestampPrecisionLabels,
} from "@/shared/lib/recordLabels";
import { recordRefKey } from "@/shared/lib/recordPosition";
import {
  localTextOf,
  periodUnjudgedParts,
} from "@/shared/lib/timestampInstant";
import { DerivedLabelNote } from "@/shared/ui/DerivedLabelNote";
import { useDisplayOffset } from "@/shared/ui/DisplayOffset";
import {
  EvidenceRecordRow,
  EvidenceTableHead,
} from "@/shared/ui/EvidenceRecordRow";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { Hint } from "@/shared/ui/Hint";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { MissingValue } from "@/shared/ui/MissingValue";
import { RawText } from "@/shared/ui/RawText";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import {
  InterpretationNote,
  TimestampOffsetNote,
} from "@/shared/ui/TimestampOffsetNote";
import { useValueMenu } from "@/shared/ui/ValueMenu";
import {
  type ContextWindowSeconds,
  contextWindowSeconds,
} from "./contextWindow";
import { useRecordContext } from "./useRecordContext";

const defaultWindowSeconds: ContextWindowSeconds = 60;

const windowLabels: Record<ContextWindowSeconds, string> = {
  10: "10 秒",
  60: "60 秒",
  300: "5 分",
};

function TimeCell({ timestamp }: { timestamp: Timestamp | undefined }) {
  if (timestamp?.rawText === undefined) {
    return <MissingValue description="原文の時刻なし" />;
  }
  const precision = `精度: ${timestampPrecisionLabels[timestamp.precision]}`;
  // 精度は tooltip に入れ、秒より粗いときだけ画面の文字でも出す。
  return (
    <>
      <Hint className="font-mono whitespace-nowrap" text={precision}>
        <RawText text={timestamp.rawText} />
      </Hint>
      {isCoarserThanSecond(timestamp.precision) ? (
        <span className="ml-2 text-xs text-muted">{precision}</span>
      ) : null}
      <TimestampOffsetNote timestamp={timestamp} />
    </>
  );
}

/**
 * 端末の表示名を出す。
 * 導いた表示名は原資料の文字列を持たず、導いた値だけを持つ。導いたことと導き方を添え、原資料の
 * 文字列と読み分けられるようにする。
 */
function TerminalLabel({ label }: { label: RawAndNormalized }) {
  const text = label.rawText ?? label.normalized;
  if (text === undefined) {
    return <MissingValue description="表示名なし" />;
  }
  return (
    <>
      <RawText text={text} />
      {label.valueState === "derived" ? (
        <>
          {" "}
          <DerivedLabelNote derivation={label.derivation} />
        </>
      ) : null}
    </>
  );
}

function ContextTable({
  caption,
  entries,
  openedKey,
}: {
  caption: string;
  entries: readonly TimelineEntry[];
  openedKey: string;
}) {
  const termMenu = useValueMenu();
  if (entries.length === 0) {
    return <KeyValueList pairs={[{ name: caption, value: "0" }]} />;
  }
  return (
    <>
      <table className="evidence-table context-table">
        <caption>{`${caption}: ${formatCount(entries.length)}`}</caption>
        <EvidenceTableHead showsTerminal />
        <tbody>
          {entries.map((entry) => (
            <EvidenceRecordRow
              key={timelineEntryKey(entry)}
              evidence={entry}
              termActions={termMenu.actions}
              current={recordRefKey(entry.recordRef) === openedKey}
              terminal={
                entry.terminal === undefined ? (
                  <MissingValue description="端末なし" />
                ) : (
                  <TerminalLabel label={entry.terminal.label} />
                )
              }
            />
          ))}
        </tbody>
      </table>
      {termMenu.menu}
    </>
  );
}

function SameTerminalTable({
  entries,
  openedKey,
}: {
  entries: readonly TimelineEntry[];
  openedKey: string;
}) {
  const opened = entries.find(
    (entry) => recordRefKey(entry.recordRef) === openedKey,
  );
  if (opened === undefined) {
    return (
      <KeyValueList
        pairs={[
          {
            name: "同じ端末",
            value: (
              <StatusLabel
                status="idle"
                label="端末の決定不可"
                details="開いたレコードが期間の時系列の外"
              />
            ),
          },
        ]}
      />
    );
  }
  const terminal = opened.terminal;
  if (terminal === undefined) {
    return (
      <KeyValueList
        pairs={[{ name: "同じ端末", value: "開いたレコードに端末なし" }]}
      />
    );
  }
  return (
    <ContextTable
      caption="同じ端末"
      entries={entries.filter((entry) => entry.terminal?.id === terminal.id)}
      openedKey={openedKey}
    />
  );
}

/**
 * 前後の期間を「期間: 始まり – 終わり」の組で出す。画面全体で表示のタイムゾーンを選んでいるときは、
 * そのタイムゾーンの期間の組を添える。両端のタイムゾーンは開いたレコードの時刻のタイムゾーンであり、
 * 分析者か起動の指定から来たタイムゾーンには、その出どころを添える。
 */
function WindowSummary({
  eventTime,
  timeFilter,
  periodUnjudged,
}: {
  eventTime: Timestamp;
  timeFilter: GraphTimeFilter;
  periodUnjudged: PeriodUnjudged | undefined;
}) {
  const offset = useDisplayOffset();
  const from = timeFilter.from?.text ?? "";
  const to = timeFilter.to?.text ?? "";
  const localFrom = localTextOf(from, offset);
  const localTo = localTextOf(to, offset);
  const showsLocal =
    localFrom !== undefined &&
    localTo !== undefined &&
    (localFrom !== from || localTo !== to);
  const outside =
    (periodUnjudged?.localRecordCount ?? 0) +
    (periodUnjudged?.undatedRecordCount ?? 0);
  return (
    <>
      <div className="flex flex-wrap items-center gap-x-3">
        <KeyValueList
          className="font-mono"
          pairs={[
            { name: "期間", value: `${from} – ${to}` },
            {
              name: `UTC${offset}`,
              value: showsLocal ? `${localFrom} – ${localTo}` : undefined,
            },
          ]}
        />
        {eventTime.interpretation === undefined ? null : (
          <InterpretationNote interpretation={eventTime.interpretation} />
        )}
      </div>
      {outside === 0 ? null : (
        <KeyValueList
          pairs={[
            { name: "表の外", value: formatCount(outside) },
            ...periodUnjudgedParts(periodUnjudged),
          ]}
        />
      )}
    </>
  );
}

/**
 * 開いたレコードの前後の期間にあるレコードを、同じ端末と全端末の 2 つの一覧で出す。
 * 期間の幅は利用者が選ぶ。開いたレコードの行を `aria-current` で示す。行の値の操作 (別の
 * レコードを開く、条件に追加する) は、表の値の操作 (ValueActions) が持つ。
 */
export function RecordContext({
  recordRef,
  matchConditions,
  dataVersion,
}: {
  /** 開いたレコード。開いていないときは `undefined`。 */
  recordRef: RecordLocator | undefined;
  /** すべての要求が含む関連付けの条件の選択。 */
  matchConditions: MatchConditionSelection;
  /** backend のデータが変わるたびに増える値。変わったら取り直す。 */
  dataVersion: number;
}) {
  const [windowSeconds, setWindowSeconds] =
    useState<ContextWindowSeconds>(defaultWindowSeconds);

  return (
    <section aria-label="レコードの前後">
      <label>
        前後の幅
        <select
          value={windowSeconds}
          onChange={(event) => {
            const selected = contextWindowSeconds.find(
              (seconds) => String(seconds) === event.target.value,
            );
            if (selected !== undefined) {
              setWindowSeconds(selected);
            }
          }}
        >
          {contextWindowSeconds.map((seconds) => (
            <option key={seconds} value={seconds}>
              {windowLabels[seconds]}
            </option>
          ))}
        </select>
      </label>
      {recordRef === undefined ? (
        <KeyValueList pairs={[{ name: "レコード", value: "未選択" }]} />
      ) : (
        <RecordContextBody
          recordRef={recordRef}
          windowSeconds={windowSeconds}
          matchConditions={matchConditions}
          dataVersion={dataVersion}
        />
      )}
    </section>
  );
}

function RecordContextBody({
  recordRef,
  windowSeconds,
  matchConditions,
  dataVersion,
}: {
  recordRef: RecordLocator;
  windowSeconds: ContextWindowSeconds;
  matchConditions: MatchConditionSelection;
  dataVersion: number;
}) {
  const state = useRecordContext(
    recordRef,
    windowSeconds,
    matchConditions,
    dataVersion,
  );
  const openedKey = recordRefKey(recordRef);

  return (
    <FetchStateView
      state={state}
      loadingDescription="レコードの前後の読み込み中"
    >
      {(value) => {
        switch (value.kind) {
          case "time_unreadable":
            return (
              <KeyValueList
                pairs={[
                  {
                    name: "前後",
                    value: (
                      <StatusLabel
                        status="idle"
                        label="表示不可"
                        details="開いたレコードの時刻を UTC 時刻に解釈不可"
                      />
                    ),
                  },
                  {
                    name: "原文の時刻",
                    value:
                      value.eventTime?.rawText === undefined ? undefined : (
                        <RawText text={value.eventTime.rawText} />
                      ),
                  },
                ]}
              />
            );
          case "window":
            return (
              <>
                <KeyValueList
                  pairs={[
                    {
                      name: "開いたレコードの時刻",
                      value: <TimeCell timestamp={value.eventTime} />,
                    },
                  ]}
                />
                <WindowSummary
                  eventTime={value.eventTime}
                  timeFilter={value.timeFilter}
                  periodUnjudged={value.timeline.periodUnjudged}
                />
                <SameTerminalTable
                  entries={value.timeline.entries}
                  openedKey={openedKey}
                />
                <ContextTable
                  caption="全端末"
                  entries={value.timeline.entries}
                  openedKey={openedKey}
                />
              </>
            );
          default: {
            const exhaustive: never = value;
            throw new Error(
              `unknown record context: ${JSON.stringify(exhaustive)}`,
            );
          }
        }
      }}
    </FetchStateView>
  );
}
