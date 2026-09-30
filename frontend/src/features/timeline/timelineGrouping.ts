import {
  offsetCarriesInstant,
  type Timestamp,
  type TimestampPrecision,
} from "@/shared/contracts/common";
import type { TimelineEntry } from "@/shared/contracts/timeline";

/** 区切りの幅の段階の識別子。 */
export type TimelineBucketScaleKey =
  | "day"
  | "hour"
  | "ten_minutes"
  | "minute"
  | "ten_seconds"
  | "second";

/** 区切りの幅の段階と、1 件ずつの行の段階を合わせた識別子。 */
export type TimelineScaleKey = TimelineBucketScaleKey | "record";

/** 時系列の表示の段階。区切りの段階は、区切りの幅を UTC の時刻の上で持つ。 */
export type TimelineScale =
  | { kind: "bucket"; key: TimelineBucketScaleKey; widthMs: number }
  | { kind: "record"; key: "record" };

const secondMs = 1_000;
const minuteMs = 60 * secondMs;
const hourMs = 60 * minuteMs;
const dayMs = 24 * hourMs;

/**
 * 表示の段階を粗い順に並べる。最後の段階は 1 件ずつのレコードの行である。
 *
 * 区切りの境界は UTC の時刻で引く。収集元ごとに UTC からのずれが違うため、1 つの
 * 収集元の地方時で境界を引くと、別の収集元のレコードの区切りが地方時の日や時と揃わない。
 */
export const timelineScales: readonly TimelineScale[] = [
  { kind: "bucket", key: "day", widthMs: dayMs },
  { kind: "bucket", key: "hour", widthMs: hourMs },
  { kind: "bucket", key: "ten_minutes", widthMs: 10 * minuteMs },
  { kind: "bucket", key: "minute", widthMs: minuteMs },
  { kind: "bucket", key: "ten_seconds", widthMs: 10 * secondMs },
  { kind: "bucket", key: "second", widthMs: secondMs },
  { kind: "record", key: "record" },
];

/** 識別子が指す段階を返す。 */
export function timelineScaleOf(key: TimelineScaleKey): TimelineScale {
  const scale = timelineScales.find((candidate) => candidate.key === key);
  if (scale === undefined) {
    throw new Error(`unknown timeline scale: ${key}`);
  }
  return scale;
}

/** 1 つ細かい段階を返す。最も細かい段階では undefined を返す。 */
export function finerScaleOf(key: TimelineScaleKey): TimelineScale | undefined {
  const index = timelineScales.findIndex((scale) => scale.key === key);
  return timelineScales[index + 1];
}

/** 1 つ粗い段階を返す。最も粗い段階では undefined を返す。 */
export function coarserScaleOf(
  key: TimelineScaleKey,
): TimelineScale | undefined {
  const index = timelineScales.findIndex((scale) => scale.key === key);
  return index <= 0 ? undefined : timelineScales[index - 1];
}

/**
 * レコード 1 件の時刻を、区切りへ入れられるかの判定に使う形で読んだ結果。
 *
 * `placed` の範囲は、時刻の精度が示す範囲である。精度が分なら、その分の始まりから
 * 次の分の始まりの前までを持つ。範囲の両端は UTC の epoch からの ms で、終わりを含まない。
 */
export type EntryTime =
  | {
      kind: "placed";
      startMs: number;
      endMs: number;
      precision: TimestampPrecision;
    }
  | { kind: "offset_undetermined" }
  | { kind: "time_unreadable" };

const rfc3339Pattern =
  /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d+))?(Z|[+-]\d{2}:\d{2})$/;

/**
 * 地方時の各欄を、UTC の同じ字面の時刻の epoch からの ms へ写す。
 * 欄の値が範囲を超えると上の欄へ繰り上げる (13 月は翌年の 1 月になる)。
 */
function wallClockMs(
  year: number,
  month: number,
  day: number,
  hour: number,
  minute: number,
  second: number,
  millisecond: number,
): number {
  // Date.UTC は 0 から 99 の年を 1900 年代へ読み替えるため、setUTCFullYear で年を入れる。
  const date = new Date(0);
  date.setUTCFullYear(year, month - 1, day);
  date.setUTCHours(hour, minute, second, millisecond);
  return date.getTime();
}

function offsetMinutesOf(designator: string): number | undefined {
  if (designator === "Z") {
    return 0;
  }
  const sign = designator.startsWith("-") ? -1 : 1;
  const hours = Number(designator.slice(1, 3));
  const minutes = Number(designator.slice(4, 6));
  if (minutes > 59) {
    return undefined;
  }
  return sign * (hours * 60 + minutes);
}

/**
 * 時刻の正規化値から、精度が示す範囲を UTC の ms で読む。
 * 正規化値を読めないときは undefined を返す。
 *
 * **欄の値が暦の範囲を外れる文字列を読めない扱いにする。** `Date.UTC` は 2 月 31 日を
 * 3 月 3 日へ繰り上げるため、読み替えた時刻で区切りへ入れると原資料と別の時刻になる。
 */
function precisionRangeOf(
  normalized: string,
  precision: TimestampPrecision,
): { startMs: number; endMs: number } | undefined {
  const match = rfc3339Pattern.exec(normalized);
  if (match === null) {
    return undefined;
  }
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  const hour = Number(match[4]);
  const minute = Number(match[5]);
  const second = Number(match[6]);
  const millisecond = Number((match[7] ?? "").padEnd(3, "0").slice(0, 3));
  const offsetMinutes = offsetMinutesOf(match[8] ?? "");
  if (offsetMinutes === undefined) {
    return undefined;
  }
  const parsed = new Date(
    wallClockMs(year, month, day, hour, minute, second, millisecond),
  );
  if (
    parsed.getUTCFullYear() !== year ||
    parsed.getUTCMonth() !== month - 1 ||
    parsed.getUTCDate() !== day ||
    parsed.getUTCHours() !== hour ||
    parsed.getUTCMinutes() !== minute ||
    parsed.getUTCSeconds() !== second
  ) {
    return undefined;
  }
  const offsetMs = offsetMinutes * minuteMs;
  const [start, end] = wallClockRangeOf(
    [year, month, day, hour, minute, second, millisecond],
    precision,
  );
  return { startMs: start - offsetMs, endMs: end - offsetMs };
}

type WallClockFields = [number, number, number, number, number, number, number];

/**
 * 地方時の欄を精度で切り捨てた始まりと、1 単位進めた終わりを返す。
 *
 * 秒より細かい精度は ms の単位で範囲を持つ。マイクロ秒の精度の範囲は、その値を含む
 * 1 ms に収まるため、1 ms の範囲として扱っても区切りへ入れるかの判定は変わらない。
 */
function wallClockRangeOf(
  [year, month, day, hour, minute, second, millisecond]: WallClockFields,
  precision: TimestampPrecision,
): [number, number] {
  switch (precision) {
    case "year":
      return [
        wallClockMs(year, 1, 1, 0, 0, 0, 0),
        wallClockMs(year + 1, 1, 1, 0, 0, 0, 0),
      ];
    case "month":
      return [
        wallClockMs(year, month, 1, 0, 0, 0, 0),
        wallClockMs(year, month + 1, 1, 0, 0, 0, 0),
      ];
    case "day":
      return [
        wallClockMs(year, month, day, 0, 0, 0, 0),
        wallClockMs(year, month, day + 1, 0, 0, 0, 0),
      ];
    case "hour":
      return [
        wallClockMs(year, month, day, hour, 0, 0, 0),
        wallClockMs(year, month, day, hour + 1, 0, 0, 0),
      ];
    case "minute":
      return [
        wallClockMs(year, month, day, hour, minute, 0, 0),
        wallClockMs(year, month, day, hour, minute + 1, 0, 0),
      ];
    case "second":
      return [
        wallClockMs(year, month, day, hour, minute, second, 0),
        wallClockMs(year, month, day, hour, minute, second + 1, 0),
      ];
    case "millisecond":
    case "microsecond": {
      const start = wallClockMs(
        year,
        month,
        day,
        hour,
        minute,
        second,
        millisecond,
      );
      return [start, start + 1];
    }
    default: {
      const unknown: never = precision;
      throw new Error(`unknown timestamp precision: ${String(unknown)}`);
    }
  }
}

/**
 * レコードの事象の時刻を、区切りへ入れられるかの判定に使う形で読む。
 *
 * 区切りへ入れられるのは、UTC からのずれの状態から時点が定まる値であり、値が
 * 出ていて、正規化値を RFC 3339 として読めるレコードである。分析者がずれを与えた
 * 地方時 (`interpretation`) は、正規化値をそのずれで読む。backend の
 * `Timestamp.Instant` と同じ条件である。
 */
export function readEntryTime(timestamp: Timestamp | undefined): EntryTime {
  if (timestamp === undefined) {
    return { kind: "time_unreadable" };
  }
  const { interpretation } = timestamp;
  if (
    !offsetCarriesInstant(timestamp.offsetState) &&
    interpretation === undefined
  ) {
    return { kind: "offset_undetermined" };
  }
  if (
    timestamp.valueState !== "present" ||
    timestamp.normalized === undefined
  ) {
    return { kind: "time_unreadable" };
  }
  const normalized =
    interpretation === undefined
      ? timestamp.normalized
      : timestamp.normalized + interpretation.offset;
  const range = precisionRangeOf(normalized, timestamp.precision);
  if (range === undefined) {
    return { kind: "time_unreadable" };
  }
  return { kind: "placed", ...range, precision: timestamp.precision };
}

/** 行の並びの各レコードの時刻を読む。 */
export function readEntryTimes(entries: readonly TimelineEntry[]): EntryTime[] {
  return entries.map((entry) => readEntryTime(entry.eventTime));
}

type GroupCommon = {
  /** 表の中で行を一意に指す key。 */
  key: string;
  /** 行に入れた最初のレコードの、応答の並びでの位置。 */
  firstEntryIndex: number;
  entryCount: number;
  /** 収集元の識別子ごとの件数。 */
  sourceCounts: Map<string, number>;
};

/**
 * 区切りの段階の表の 1 行。
 *
 * - `bucket`: 区切り 1 つ。範囲は区切りの両端である。
 * - `coarser_than_bucket`: 時刻の精度が示す範囲が 2 つ以上の区切りにまたがるレコード。
 *   範囲と精度が同じレコードを 1 行にまとめる。範囲はレコードの精度が示す範囲である。
 * - `offset_undetermined` と `time_unreadable`: 区切りへ入れる時刻を持たないレコード。
 *   応答の並びで続くレコードを 1 行にまとめる。
 */
export type TimelineGroup =
  | (GroupCommon & {
      kind: "bucket";
      range: { startMs: number; endMs: number };
    })
  | (GroupCommon & {
      kind: "coarser_than_bucket";
      range: { startMs: number; endMs: number };
      precision: TimestampPrecision;
    })
  | (GroupCommon & { kind: "offset_undetermined" | "time_unreadable" });

/** 区切りの段階で組んだ表の行と、行の種別ごとの件数。 */
export type TimelineGrouping = {
  groups: TimelineGroup[];
  /** 応答の並びでの位置から、そのレコードを入れた行の位置を求める。 */
  groupOfEntry: Int32Array;
  /** 行の種別ごとのレコードの件数。合計は応答の行の件数と等しい。 */
  entryCounts: Record<TimelineGroup["kind"], number>;
};

function addEntry(group: TimelineGroup, sourceId: string): void {
  group.entryCount += 1;
  group.sourceCounts.set(sourceId, (group.sourceCounts.get(sourceId) ?? 0) + 1);
}

/**
 * レコードを区切りの幅ごとの行へまとめる。行の並びは、行に入れた最初のレコードの、
 * 応答の並びでの位置の順である。
 *
 * **区切りへ入れるのは、時刻の精度が示す範囲が 1 つの区切りに収まるレコードだけにする。**
 * 精度が分のレコードを 10 秒の区切りへ入れると、そのレコードが区切りの中で起きたと
 * 読める。UTC からのずれが確認できないレコードと、時刻を読めないレコードも区切りへ
 * 入れず、種別を分けた行に出す。
 */
export function groupTimeline(
  entries: readonly TimelineEntry[],
  times: readonly EntryTime[],
  widthMs: number,
): TimelineGrouping {
  const groups: TimelineGroup[] = [];
  const groupAt = new Map<string, number>();
  const groupOfEntry = new Int32Array(entries.length);
  const entryCounts: TimelineGrouping["entryCounts"] = {
    bucket: 0,
    coarser_than_bucket: 0,
    offset_undetermined: 0,
    time_unreadable: 0,
  };
  const append = (group: TimelineGroup): number => {
    groupAt.set(group.key, groups.length);
    groups.push(group);
    return groups.length - 1;
  };
  let previousAt: number | undefined;
  entries.forEach((entry, index) => {
    const time = times[index] ?? { kind: "time_unreadable" };
    const common = () => ({
      firstEntryIndex: index,
      entryCount: 0,
      sourceCounts: new Map<string, number>(),
    });
    let at: number;
    if (time.kind === "placed") {
      const first = Math.floor(time.startMs / widthMs);
      const last = Math.floor((time.endMs - 1) / widthMs);
      const key =
        first === last
          ? `bucket:${first}`
          : `coarser:${time.precision}:${time.startMs}:${time.endMs}`;
      at =
        groupAt.get(key) ??
        append(
          first === last
            ? {
                ...common(),
                kind: "bucket",
                key,
                range: {
                  startMs: first * widthMs,
                  endMs: (first + 1) * widthMs,
                },
              }
            : {
                ...common(),
                kind: "coarser_than_bucket",
                key,
                range: { startMs: time.startMs, endMs: time.endMs },
                precision: time.precision,
              },
        );
    } else if (
      previousAt !== undefined &&
      groups[previousAt]?.kind === time.kind
    ) {
      at = previousAt;
    } else {
      at = append({
        ...common(),
        kind: time.kind,
        key: `${time.kind}:${index}`,
      });
    }
    const group = groups[at];
    if (group === undefined) {
      throw new Error(`timeline group ${at} is missing`);
    }
    addEntry(group, entry.recordRef.sourceId);
    entryCounts[group.kind] += 1;
    groupOfEntry[index] = at;
    previousAt = at;
  });
  return { groups, groupOfEntry, entryCounts };
}

// 既知の制限: 既定の段階は、行の数が defaultScaleRowLimit 以下になる最も細かい段階とする,
// 一覧を見渡せる行の数は分析者の読み方で決まり、上限の値の良否を数値で測れない。
// 利用者が配置した実資料で、本規則が選ぶ段階を確かめた,
// 分析者の評価で既定の段階を変える求めが出たときに見直す。
/** 既定の段階を選ぶときの、行の数の上限。 */
export const defaultScaleRowLimit = 200;

/**
 * 分析者が段階を選んでいないときの段階を返す。
 * 行の数が上限以下になる最も細かい段階を採り、どの段階も上限を超えるときは最も粗い段階を採る。
 */
export function defaultScaleKeyOf(
  entries: readonly TimelineEntry[],
  times: readonly EntryTime[],
): TimelineScaleKey {
  if (entries.length <= defaultScaleRowLimit) {
    return "record";
  }
  const bucketScales = timelineScales.filter(
    (scale): scale is Extract<TimelineScale, { kind: "bucket" }> =>
      scale.kind === "bucket",
  );
  for (const scale of [...bucketScales].reverse()) {
    const { groups } = groupTimeline(entries, times, scale.widthMs);
    if (groups.length <= defaultScaleRowLimit) {
      return scale.key;
    }
  }
  return bucketScales[0]?.key ?? "record";
}
