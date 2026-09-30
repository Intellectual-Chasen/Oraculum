import type { Timestamp } from "../contracts/common";
import type { GraphBoundPrecision, GraphTimeFilter } from "../contracts/graph";
import { absoluteNormalizedOf } from "./timestampInstant";

const rfc3339Pattern =
  /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d+))?(Z|[+-]\d{2}:\d{2})$/;

function boundPrecisionOf(
  precision: Timestamp["precision"],
): GraphBoundPrecision | undefined {
  switch (precision) {
    case "second":
      return "second";
    case "millisecond":
    case "microsecond":
      return "millisecond";
    case "year":
    case "month":
    case "day":
    case "hour":
    case "minute":
      return undefined;
    default: {
      const exhaustive: never = precision;
      throw new Error(`unknown precision: ${String(exhaustive)}`);
    }
  }
}

/**
 * 時刻 `timestamp` の前後 `windowSeconds` 秒の期間を、時系列の要求の絞り込みへ直す。
 * 両端は時刻と同じ UTC からのずれの RFC 3339 の文字列で書き、比較の単位より下の桁を
 * 切り捨てる。精度が秒のときは秒、ミリ秒以下のときはミリ秒の文字列にする。
 * 分析者がずれを与えた地方時は、そのずれの文字列で書く。
 * 絶対時刻として比べられない時刻 (ずれが定まらない、値が無い、秒より粗い精度) には
 * `undefined` を返す。backend の `Timestamp.Instant` が時点を返す条件に合わせる。
 */
export function contextTimeFilter(
  timestamp: Timestamp,
  windowSeconds: number,
): GraphTimeFilter | undefined {
  const normalized = absoluteNormalizedOf(timestamp);
  if (normalized === undefined) {
    return undefined;
  }
  const precision = boundPrecisionOf(timestamp.precision);
  const match = rfc3339Pattern.exec(normalized);
  if (precision === undefined || match === null) {
    return undefined;
  }
  const [, base = "", fraction = "", offset = ""] = match;
  // 時刻の文字列の壁時計をそのまま UTC の値として扱い、書き戻すときに同じずれを付ける。
  const wallClockMs =
    Date.parse(`${base}Z`) + Number(fraction.padEnd(3, "0").slice(0, 3));
  if (Number.isNaN(wallClockMs)) {
    return undefined;
  }
  const format = (ms: number) => {
    const iso = new Date(ms).toISOString();
    return `${precision === "second" ? iso.slice(0, 19) : iso.slice(0, 23)}${offset}`;
  };
  const windowMs = windowSeconds * 1000;
  return {
    from: { text: format(wallClockMs - windowMs), precision },
    to: { text: format(wallClockMs + windowMs), precision },
    unit: precision,
  };
}

/**
 * UTC の ms の区切り `[startMs, endMs)` を、両端を含む期間の絞り込みへ直す。区切りは終わりを
 * 含まないため、終わりの 1 ms 前を期間の終わりにする。両端はミリ秒の精度の UTC で書く。
 */
export function msRangeTimeFilter(
  startMs: number,
  endMs: number,
): GraphTimeFilter {
  const bound = (ms: number) => ({
    text: new Date(ms).toISOString(),
    precision: "millisecond" as const,
  });
  return { from: bound(startMs), to: bound(endMs - 1), unit: "millisecond" };
}
