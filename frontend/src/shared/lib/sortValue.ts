import type { Timestamp } from "../contracts/common";
import { utcTextOf } from "./timestampInstant";

/**
 * 表の並べ替えに使う値。件数・UTC の ms は number、IPv6 のアドレスは bigint、名前は string で
 * 持つ。undefined は値の無い行であり、並びの向きに依らず末尾に置く。
 */
export type SortValue = number | bigint | string | undefined;

export type SortDirection = "ascending" | "descending";

// 並びを分析者の環境の言語設定に依らず同じにするため、ロケールを固定する。数字の並びは数として比べる。
const collator = new Intl.Collator("en", { numeric: true });

/** 文字列を照合順で比べる。`host10` を `host9` の後に置く。 */
export function compareText(left: string, right: string): number {
  return collator.compare(left, right);
}

/** 数は文字列より前に置く。数どうし (number と bigint を含む) は大小で、文字列どうしは照合順で比べる。 */
function compareDefined(
  a: number | bigint | string,
  b: number | bigint | string,
): number {
  const aText = typeof a === "string";
  const bText = typeof b === "string";
  if (aText && bText) return compareText(a, b);
  if (aText) return 1;
  if (bText) return -1;
  return a < b ? -1 : a > b ? 1 : 0;
}

/**
 * 行を値で並べ替えた、元の位置との組を返す。同じ値の行は元の順を保ち、値の無い行は末尾に置く。
 */
export function sortRows<Row>(
  rows: readonly Row[],
  sortValueOf: (row: Row) => SortValue,
  direction: SortDirection,
): { row: Row; index: number }[] {
  const sign = direction === "ascending" ? 1 : -1;
  return rows
    .map((row, index) => ({ row, index, value: sortValueOf(row) }))
    .sort((a, b) => {
      if (a.value === undefined || b.value === undefined) {
        if (a.value === b.value) return a.index - b.index;
        return a.value === undefined ? 1 : -1;
      }
      return sign * compareDefined(a.value, b.value) || a.index - b.index;
    })
    .map(({ row, index }) => ({ row, index }));
}

/** 時刻を UTC の ms にする。UTC に直せない時刻と、時刻の無い値は値なしとする。 */
export function timestampSortValue(
  timestamp: Timestamp | undefined,
): SortValue {
  const utc = timestamp === undefined ? undefined : utcTextOf(timestamp);
  return utc === undefined ? undefined : Date.parse(utc);
}

/** IPv4 のアドレスの 4 つの数を返す。IPv4 として読めない文字列では undefined を返す。 */
function ipv4Octets(text: string): number[] | undefined {
  const v4 = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(text);
  const octets = v4?.slice(1).map(Number);
  return octets?.every((octet) => octet <= 255) ? octets : undefined;
}

/**
 * IP アドレスの文字列を、アドレスの大小で並ぶ値にする。IPv4 は IPv4 射影の IPv6 と同じ値にし、
 * 末尾を IPv4 で書いた IPv6 (`::ffff:192.0.2.1`) も同じ規則で読む。
 * アドレスとして読めない文字列は、文字列のまま返す。
 */
export function ipSortValue(text: string): SortValue {
  if (!text.includes(":")) {
    const octets = ipv4Octets(text);
    if (octets === undefined) return text;
    return octets.reduce((sum, octet) => sum * 256n + BigInt(octet), 0xffffn);
  }
  let address = text.split("%")[0] ?? text;
  const lastColon = address.lastIndexOf(":");
  const trailing = address.slice(lastColon + 1);
  if (trailing.includes(".")) {
    const octets = ipv4Octets(trailing);
    if (octets === undefined) return text;
    const [a = 0, b = 0, c = 0, d = 0] = octets;
    address = `${address.slice(0, lastColon + 1)}${(a * 256 + b).toString(16)}:${(c * 256 + d).toString(16)}`;
  }
  if (address.includes(".")) return text;
  const halves = address.split("::");
  if (halves.length > 2) return text;
  const head = halves[0] === "" ? [] : (halves[0] ?? "").split(":");
  const tail =
    halves.length === 2 && halves[1] !== "" ? (halves[1] ?? "").split(":") : [];
  const missing = 8 - head.length - tail.length;
  if (halves.length === 1 ? missing !== 0 : missing < 1) return text;
  const groups = [...head, ...Array(missing).fill("0"), ...tail];
  if (!groups.every((group) => /^[0-9a-fA-F]{1,4}$/.test(group))) return text;
  return groups.reduce(
    (sum, group) => sum * 65536n + BigInt(Number.parseInt(group, 16)),
    0n,
  );
}
