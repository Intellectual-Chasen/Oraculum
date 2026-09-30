import { createContext, useContext } from "react";
import type { Timestamp } from "../contracts/common";
import { localTextOf, utcTextOf } from "../lib/timestampInstant";

/**
 * 画面全体で時刻に添える表示のタイムゾーン (例 `+09:00`)。空の文字列は選んでいないことを
 * 表し、時刻を UTC の文字列だけで出す。
 */
export const DisplayOffsetContext = createContext("");

/** 画面全体で選んだ表示のずれを返す。選んでいないときは空の文字列を返す。 */
export function useDisplayOffset(): string {
  return useContext(DisplayOffsetContext);
}

/** UTC からのずれの分を `+09:00` の形の文字列で返す。 */
export function offsetText(minutes: number): string {
  const sign = minutes < 0 ? "-" : "+";
  const absolute = Math.abs(minutes);
  const hours = String(Math.floor(absolute / 60)).padStart(2, "0");
  return `${sign}${hours}:${String(absolute % 60).padStart(2, "0")}`;
}

/** 選べる表示のずれ。-12:00 から +14:00 の 1 時間ごとと、30 分・45 分のずれを持つ地域の値。 */
const displayOffsets = [
  ...Array.from({ length: 27 }, (_, index) => (index - 12) * 60),
  -210,
  210,
  270,
  330,
  345,
  390,
  570,
  630,
]
  .sort((left, right) => left - right)
  .map(offsetText);

/** 画面全体の表示のずれを 1 つ選ぶ欄。 */
export function DisplayOffsetSelect({
  value,
  onChange,
}: {
  value: string;
  onChange: (offset: string) => void;
}) {
  return (
    <label className="display-offset">
      表示のタイムゾーン{" "}
      <select value={value} onChange={(event) => onChange(event.target.value)}>
        <option value="">なし</option>
        {displayOffsets.map((offset) => (
          <option key={offset} value={offset}>
            UTC{offset}
          </option>
        ))}
      </select>
    </label>
  );
}

const noteStyle = { display: "block" } as const;

/**
 * UTC に直せる時刻に、画面全体で選んだ表示のタイムゾーンの時刻を「UTC+09:00: 時刻」の組で、
 * 時刻の次の行に添える。表示のタイムゾーンを選んでいないときと、UTC に直せない時刻には
 * 何も出さない。
 */
export function LocalTimeNote({
  timestamp,
}: {
  timestamp: Timestamp | undefined;
}) {
  const offset = useDisplayOffset();
  const utc = timestamp === undefined ? undefined : utcTextOf(timestamp);
  const local = utc === undefined ? undefined : localTextOf(utc, offset);
  return local === undefined ? null : (
    <span style={noteStyle}>{`UTC${offset}: ${local}`}</span>
  );
}
