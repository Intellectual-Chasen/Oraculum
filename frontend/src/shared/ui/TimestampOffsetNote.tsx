import {
  offsetCarriesInstant,
  type Timestamp,
  type TimestampInterpretation,
} from "@/shared/contracts/common";
import { utcTextOf } from "@/shared/lib/timestampInstant";
import { LocalTimeNote } from "./DisplayOffset";
import { Hint } from "./Hint";
import { RawText } from "./RawText";

const noteStyle = { display: "block" } as const;

/** UTC に直せない時刻の印。 */
export const offsetUnknownLabel = "タイムゾーン不明";

/** UTC に直せない時刻の印の tooltip。 */
export const offsetUnknownDescription = "UTC に直せない時刻";

/**
 * 時刻に与えたタイムゾーンの出どころの名前。メモの識別子を持つものは分析者の記録、持たない
 * ものは取り込みの起動で指定したタイムゾーンである。
 */
export function interpretationOriginLabel(
  interpretation: TimestampInterpretation,
): string {
  return interpretation.assertionId === undefined
    ? "起動時の指定"
    : "分析者の記録";
}

/**
 * 時刻に与えたタイムゾーンを「出どころ: UTC+09:00」の組で 1 行に出す。
 *
 * **起動で指定したタイムゾーンを分析者の記録と書かない。** 分析者の記録は著者・時刻・根拠を
 * 持つメモであり、起動で指定したタイムゾーンはメモを持たない。
 */
export function InterpretationNote({
  interpretation,
  prefix,
  className,
}: {
  interpretation: TimestampInterpretation;
  /** 行の先頭に置く、どの時刻の組かを示す文字列。 */
  prefix?: string;
  className?: string;
}) {
  return (
    <span className={className} style={noteStyle}>
      {prefix}
      {interpretationOriginLabel(interpretation)}: UTC
      <RawText text={interpretation.offset} />
    </span>
  );
}

/**
 * タイムゾーンを与えた時刻の UTC を 1 行で出す。タイムゾーンを与えていない時刻と、UTC に
 * 直せない時刻には何も出さない。正規化値は書き換えないため、UTC は表示の時に求める。
 */
export function InterpretedUtcNote({ timestamp }: { timestamp: Timestamp }) {
  const utc =
    timestamp.interpretation === undefined ? undefined : utcTextOf(timestamp);
  return utc === undefined ? null : (
    <span style={noteStyle}>{`UTC: ${utc}`}</span>
  );
}

/**
 * 時刻のタイムゾーンの出どころを、時刻の文字列の次の行に出す。タイムゾーンを与えた時刻は、
 * その UTC も出す。原文がタイムゾーンを持つ時刻には何も出さない。
 *
 * **与えたタイムゾーンを、原文と別に出す。** 並びの位置と期間は与えたタイムゾーンで決まった
 * ものであり、原文の事実と読まれないようにする。
 *
 * 画面全体で表示のタイムゾーンを選んでいるときは、UTC に直せる時刻にそのタイムゾーンの時刻を添える。
 */
export function TimestampOffsetNote({ timestamp }: { timestamp: Timestamp }) {
  if (timestamp.interpretation !== undefined) {
    return (
      <>
        <InterpretedUtcNote timestamp={timestamp} />
        <InterpretationNote interpretation={timestamp.interpretation} />
        <LocalTimeNote timestamp={timestamp} />
      </>
    );
  }
  if (!offsetCarriesInstant(timestamp.offsetState)) {
    return (
      <strong style={noteStyle}>
        <Hint text={offsetUnknownDescription}>{offsetUnknownLabel}</Hint>
      </strong>
    );
  }
  return <LocalTimeNote timestamp={timestamp} />;
}
