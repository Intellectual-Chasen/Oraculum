import type { Timestamp } from "../contracts/common";
import { toVisibleRawText } from "../lib/rawText";
import { describeRawTextAbsence } from "../lib/recordField";
import { localTextOf, utcTextOf } from "../lib/timestampInstant";
import { useDisplayOffset } from "./DisplayOffset";
import { PeriodMark } from "./Highlighted";
import { MissingValue } from "./MissingValue";
import { RawText } from "./RawText";

/**
 * 時刻の原文を出す。原文が起点からの経過の数 (FILETIME など) で書く時刻は、UTC の文字列を出し、
 * 原文を title に入れる。原文を持たない時刻は `absence` の印を出す。
 */
export function RawTimestampText({
  timestamp,
  absence,
}: {
  timestamp: Timestamp;
  absence: string;
}) {
  if (timestamp.rawText === undefined) {
    return <MissingValue description={absence} />;
  }
  const utc =
    timestamp.offsetState === "epoch" ? utcTextOf(timestamp) : undefined;
  return (
    <PeriodMark timestamp={timestamp}>
      {utc === undefined ? (
        <RawText text={timestamp.rawText} />
      ) : (
        <span title={`原文: ${toVisibleRawText(timestamp.rawText)}`}>
          {utc}
        </span>
      )}
    </PeriodMark>
  );
}

/**
 * 時刻を出す。UTC に直せる時刻は UTC の文字列を出し、表示のタイムゾーンの時刻と原文を
 * 「名前: 値」の組で title に入れる。UTC に直せない時刻は原文だけを出す。原文を持たない時刻は、
 * 持たない印を出す。
 */
export function TimestampText({ timestamp }: { timestamp: Timestamp }) {
  const offset = useDisplayOffset();
  const raw =
    timestamp.rawText === undefined ? (
      <MissingValue
        description={describeRawTextAbsence(timestamp.valueState)}
      />
    ) : (
      <RawText text={timestamp.rawText} />
    );
  const utc = utcTextOf(timestamp);
  if (utc === undefined) {
    return <PeriodMark timestamp={timestamp}>{raw}</PeriodMark>;
  }
  const local = localTextOf(utc, offset);
  const facts = [
    ...(local === undefined ? [] : [`UTC${offset}: ${local}`]),
    ...(timestamp.rawText === undefined || timestamp.rawText === utc
      ? []
      : [`原文: ${toVisibleRawText(timestamp.rawText)}`]),
  ];
  return (
    <PeriodMark timestamp={timestamp}>
      {facts.length === 0 ? utc : <span title={facts.join("\n")}>{utc}</span>}
    </PeriodMark>
  );
}
