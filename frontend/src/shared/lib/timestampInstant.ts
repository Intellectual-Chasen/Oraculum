import { offsetCarriesInstant, type Timestamp } from "../contracts/common";
import type { PeriodUnjudged } from "../contracts/timeline";
import { formatCount } from "./format";

/**
 * 時点を読む RFC 3339 の文字列を返す。分析者がずれを与えた地方時は、正規化値にそのずれを
 * 付けて読む。時点が定まらない時刻には `undefined` を返す。
 */
export function absoluteNormalizedOf(timestamp: Timestamp): string | undefined {
  if (
    timestamp.valueState !== "present" ||
    timestamp.normalized === undefined
  ) {
    return undefined;
  }
  if (
    timestamp.interpretation !== undefined &&
    timestamp.normalizedForm === "local_without_offset"
  ) {
    return timestamp.normalized + timestamp.interpretation.offset;
  }
  return offsetCarriesInstant(timestamp.offsetState) &&
    timestamp.normalizedForm === "rfc3339_absolute"
    ? timestamp.normalized
    : undefined;
}

const rfc3339Pattern =
  /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(\.\d+)?(Z|[+-]\d{2}:\d{2})$/;

/**
 * 時刻を UTC の RFC 3339 の文字列で返す。秒より下の桁は正規化値の桁のまま保つ。
 * 時点が定まらない時刻には `undefined` を返す。
 */
export function utcTextOf(timestamp: Timestamp): string | undefined {
  const absolute = absoluteNormalizedOf(timestamp);
  const match = absolute === undefined ? null : rfc3339Pattern.exec(absolute);
  if (match === null) {
    return undefined;
  }
  const [, seconds, fraction = "", designator] = match;
  const instant = Date.parse(seconds + designator);
  if (Number.isNaN(instant)) {
    return undefined;
  }
  return `${new Date(instant).toISOString().slice(0, 19)}${fraction}Z`;
}

const offsetPattern = /^([+-])(\d{2}):(\d{2})$/;

/**
 * 時点を持つ RFC 3339 の文字列 `instant` を、UTC からのずれ `offset` (例 `+09:00`) の地方時の
 * 文字列で返す。秒より下の桁は `instant` の桁のまま保つ。`offset` が空のとき (表示のずれを
 * 選んでいないとき) と、文字列を読めないときは `undefined` を返す。
 */
export function localTextOf(
  instant: string,
  offset: string,
): string | undefined {
  const match = rfc3339Pattern.exec(instant);
  const shift = offsetPattern.exec(offset);
  if (match === null || shift === null) {
    return undefined;
  }
  const [, seconds, fraction = "", designator] = match;
  const [, sign, hours, minutes] = shift;
  const shiftMs =
    (sign === "-" ? -1 : 1) * (Number(hours) * 60 + Number(minutes)) * 60_000;
  const ms = Date.parse(seconds + designator);
  if (Number.isNaN(ms)) {
    return undefined;
  }
  return `${new Date(ms + shiftMs).toISOString().slice(0, 19)}${fraction}${offset}`;
}

/**
 * 期間で判定できないレコードの件数を「名前: 値」の組で返す。件数が 0 の組は返さない。
 * 呼ぶ側は組の前に、表や棒に入れなかったレコードの合計の組 (「表の外」「棒の外」) を置く。
 */
export function periodUnjudgedParts(
  unjudged: PeriodUnjudged | undefined,
): { name: string; value: string }[] {
  if (unjudged === undefined) return [];
  return [
    ...(unjudged.localRecordCount > 0
      ? [
          {
            name: "タイムゾーン不明",
            value: `${formatCount(unjudged.localRecordCount)} 件`,
          },
        ]
      : []),
    ...(unjudged.undatedRecordCount > 0
      ? [
          {
            name: "時刻なし",
            value: `${formatCount(unjudged.undatedRecordCount)} 件`,
          },
        ]
      : []),
  ];
}

/**
 * epoch からの ms の時点を UTC の文字列で返し、表示のタイムゾーン `offset` を選んでいるときは
 * そのタイムゾーンの時刻を「、」の後に添える。時点を画面に出すときは `toISOString` を直に使わず、
 * この関数を通す。
 */
export function instantDisplayText(ms: number, offset: string): string {
  const utc = new Date(ms).toISOString();
  const local = localTextOf(utc, offset);
  return local === undefined ? utc : `${utc}、UTC${offset}: ${local}`;
}
