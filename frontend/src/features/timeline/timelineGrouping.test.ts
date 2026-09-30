import { describe, expect, test } from "vitest";
import type { Timestamp } from "@/shared/contracts/common";
import {
  decodeTimelineResponse,
  type TimelineEntry,
} from "@/shared/contracts/timeline";
import {
  evenlySpacedEntrySpecs,
  type GeneratedEntrySpec,
  generatedTimelineResponseJson,
} from "@/testdata/timeline/timelineResponse";
import {
  defaultScaleKeyOf,
  defaultScaleRowLimit,
  finerScaleOf,
  groupTimeline,
  readEntryTime,
  readEntryTimes,
  timelineScaleOf,
} from "./timelineGrouping";

const secondMs = 1_000;
const minuteMs = 60 * secondMs;
const hourMs = 60 * minuteMs;

function timestamp(overrides: Partial<Timestamp>): Timestamp {
  return {
    rawText: overrides.normalized,
    normalizedForm: "rfc3339_absolute",
    precision: "millisecond",
    offsetState: "in_value",
    clock: "terminal_local",
    meaning: "event",
    valueState: "present",
    ...overrides,
  };
}

function entriesOf(specs: GeneratedEntrySpec[]): TimelineEntry[] {
  return decodeTimelineResponse(generatedTimelineResponseJson(specs), "$")
    .entries;
}

describe("readEntryTime", () => {
  test("ずれを持たない地方時はタイムゾーン未定であり、分析者のずれを与えるとそのずれで読む", () => {
    const local = timestamp({
      normalized: "2031-10-08T10:20:35.100",
      normalizedForm: "local_without_offset",
      offsetState: "item_absent",
    });
    expect(readEntryTime(local)).toEqual({ kind: "offset_undetermined" });
    expect(
      readEntryTime({
        ...local,
        interpretation: { offset: "+09:00", assertionId: "as:synthetic" },
      }),
    ).toEqual({
      kind: "placed",
      startMs: Date.parse("2031-10-08T01:20:35.100Z"),
      endMs: Date.parse("2031-10-08T01:20:35.100Z") + 1,
      precision: "millisecond",
    });
  });

  test("分析者のずれを与えた秒の精度の地方時は、その 1 秒を範囲に持つ", () => {
    expect(
      readEntryTime(
        timestamp({
          normalized: "2031-10-08T10:20:35",
          normalizedForm: "local_without_offset",
          offsetState: "undetermined",
          precision: "second",
          interpretation: { offset: "+00:00", assertionId: "as:synthetic" },
        }),
      ),
    ).toMatchObject({
      startMs: Date.parse("2031-10-08T10:20:35Z"),
      endMs: Date.parse("2031-10-08T10:20:36Z"),
    });
  });

  test("ミリ秒の精度の時刻は、その 1 ms を範囲に持つ", () => {
    const normalized = "2031-10-08T10:20:35.100+09:00";
    expect(readEntryTime(timestamp({ normalized }))).toEqual({
      kind: "placed",
      startMs: Date.parse(normalized),
      endMs: Date.parse(normalized) + 1,
      precision: "millisecond",
    });
  });

  test("分の精度の時刻は、その分の始まりから次の分の始まりの前までを範囲に持つ", () => {
    expect(
      readEntryTime(
        timestamp({
          normalized: "2031-10-08T10:20:00+09:00",
          precision: "minute",
        }),
      ),
    ).toEqual({
      kind: "placed",
      startMs: Date.parse("2031-10-08T10:20:00+09:00"),
      endMs: Date.parse("2031-10-08T10:21:00+09:00"),
      precision: "minute",
    });
  });

  test("日の精度の範囲は、値が持つ UTC からのずれの暦の 1 日である", () => {
    expect(
      readEntryTime(
        timestamp({
          normalized: "2031-10-08T00:00:00+09:00",
          precision: "day",
        }),
      ),
    ).toMatchObject({
      startMs: Date.parse("2031-10-07T15:00:00Z"),
      endMs: Date.parse("2031-10-08T15:00:00Z"),
    });
  });

  test("12 月の月の精度の範囲は、翌年の 1 月の始まりの前までである", () => {
    expect(
      readEntryTime(
        timestamp({ normalized: "2021-12-01T00:00:00Z", precision: "month" }),
      ),
    ).toMatchObject({
      startMs: Date.parse("2021-12-01T00:00:00Z"),
      endMs: Date.parse("2022-01-01T00:00:00Z"),
    });
  });

  test("0 から 99 の年を 1900 年代へ読み替えない", () => {
    const normalized = "0050-03-01T00:00:00Z";
    expect(readEntryTime(timestamp({ normalized }))).toMatchObject({
      startMs: new Date(normalized).getTime(),
    });
  });

  test("epoch と入力形式がずれを定める値は区切りへ入れられ、UTC からのずれが無い値と未確定の値は入れられない", () => {
    const normalized = "2033-04-17T03:12:44.500Z";
    for (const offsetState of ["epoch", "format_defined"] as const) {
      expect(readEntryTime(timestamp({ normalized, offsetState })).kind).toBe(
        "placed",
      );
    }
    for (const offsetState of ["item_absent", "undetermined"] as const) {
      expect(
        readEntryTime(
          timestamp({
            normalized: "2033-04-17T12:12:44.500",
            normalizedForm: "local_without_offset",
            offsetState,
          }),
        ),
      ).toEqual({ kind: "offset_undetermined" });
    }
  });

  test("時刻が無いレコード、値が出ていないレコード、暦に無い日付は時刻を読めない扱いにする", () => {
    expect(readEntryTime(undefined)).toEqual({ kind: "time_unreadable" });
    expect(
      readEntryTime(
        timestamp({
          normalized: "2031-10-08T10:20:35.100+09:00",
          valueState: "absent",
        }),
      ),
    ).toEqual({ kind: "time_unreadable" });
    expect(
      readEntryTime(timestamp({ normalized: "2021-02-31T00:00:00Z" })),
    ).toEqual({ kind: "time_unreadable" });
    expect(
      readEntryTime(timestamp({ normalized: "2031-10-08 10:20:35Z" })),
    ).toEqual({ kind: "time_unreadable" });
  });
});

/** 行が持つレコードの位置を、`groupOfEntry` から求め直す。 */
function entryIndexesOf(
  groupIndex: number,
  groupOfEntry: Int32Array,
): number[] {
  return Array.from(groupOfEntry.entries())
    .filter(([, group]) => group === groupIndex)
    .map(([entry]) => entry);
}

function sumOf(values: number[]): number {
  return values.reduce((total, value) => total + value, 0);
}

describe("groupTimeline", () => {
  test("区切りの行の件数の和と、行の種別ごとの件数の和が、レコードの件数と等しい", () => {
    const entries = entriesOf(
      evenlySpacedEntrySpecs(90, "2031-10-08T01:20:20Z", 2 * secondMs, [
        "hostA",
        "access",
      ]),
    );
    const grouping = groupTimeline(entries, readEntryTimes(entries), minuteMs);

    expect(sumOf(grouping.groups.map((group) => group.entryCount))).toBe(
      entries.length,
    );
    expect(sumOf(Object.values(grouping.entryCounts))).toBe(entries.length);
    grouping.groups.forEach((group, index) => {
      expect(sumOf([...group.sourceCounts.values()])).toBe(group.entryCount);
      expect(entryIndexesOf(index, grouping.groupOfEntry)).toHaveLength(
        group.entryCount,
      );
    });
  });

  test("区切りの行は、入れたレコードの時刻を範囲に含み、範囲の幅が区切りの幅である", () => {
    const entries = entriesOf(
      evenlySpacedEntrySpecs(40, "2031-10-08T01:20:50Z", 700),
    );
    const times = readEntryTimes(entries);
    const grouping = groupTimeline(entries, times, 10 * secondMs);

    grouping.groups.forEach((group, index) => {
      if (group.kind !== "bucket") {
        throw new Error(`unexpected group kind: ${group.kind}`);
      }
      expect(group.range.endMs - group.range.startMs).toBe(10 * secondMs);
      for (const entryIndex of entryIndexesOf(index, grouping.groupOfEntry)) {
        const time = times[entryIndex];
        if (time?.kind !== "placed") {
          throw new Error(`entry ${entryIndex} has no placed time`);
        }
        expect(time.startMs).toBeGreaterThanOrEqual(group.range.startMs);
        expect(time.endMs).toBeLessThanOrEqual(group.range.endMs);
      }
    });
  });

  test("精度が示す範囲が 2 つ以上の区切りにまたがるレコードを、区切りへ入れず別の行に出す", () => {
    const entries = entriesOf([
      { normalized: "2031-10-08T01:20:05.000Z" },
      { normalized: "2031-10-08T01:20:00Z", precision: "minute" },
      { normalized: "2031-10-08T01:20:15.000Z" },
    ]);
    const times = readEntryTimes(entries);

    const tenSeconds = groupTimeline(entries, times, 10 * secondMs);
    const separate = tenSeconds.groups.filter(
      (group) => group.kind === "coarser_than_bucket",
    );
    expect(separate).toMatchObject([
      {
        precision: "minute",
        entryCount: 1,
        range: {
          startMs: Date.parse("2031-10-08T01:20:00Z"),
          endMs: Date.parse("2031-10-08T01:21:00Z"),
        },
      },
    ]);
    expect(tenSeconds.entryCounts.coarser_than_bucket).toBe(1);
    expect(tenSeconds.entryCounts.bucket).toBe(entries.length - 1);

    // 区切りが精度の範囲を覆う段階では、同じレコードを区切りへ入れる。
    const oneMinute = groupTimeline(entries, times, minuteMs);
    expect(oneMinute.entryCounts.coarser_than_bucket).toBe(0);
    expect(oneMinute.groups.map((group) => group.kind)).toEqual(["bucket"]);
  });

  test("UTC からのずれが 30 分の端数を持つとき、時の精度のレコードは 1 時間の区切りにまたがる", () => {
    const entries = entriesOf([
      { normalized: "2031-10-08T10:00:00+05:30", precision: "hour" },
      { normalized: "2031-10-08T10:00:00+09:00", precision: "hour" },
    ]);
    const grouping = groupTimeline(entries, readEntryTimes(entries), hourMs);

    expect(grouping.groups.map((group) => group.kind)).toEqual([
      "coarser_than_bucket",
      "bucket",
    ]);
  });

  test("UTC からのずれを確認できないレコードは、応答の並びで続く分を 1 行にまとめる", () => {
    const local = {
      normalized: "2031-10-08T10:20:35.100",
      offsetState: "item_absent",
    };
    const entries = entriesOf([
      local,
      local,
      { normalized: "2031-10-08T01:20:35.100Z" },
      local,
    ]);
    const grouping = groupTimeline(entries, readEntryTimes(entries), minuteMs);

    const separate = grouping.groups.filter(
      (group) => group.kind === "offset_undetermined",
    );
    expect(separate.map((group) => group.entryCount)).toEqual([2, 1]);
    expect(grouping.entryCounts.offset_undetermined).toBe(
      sumOf(separate.map((group) => group.entryCount)),
    );
  });
});

describe("defaultScaleKeyOf", () => {
  test("行の数が上限以下のときは 1 件ずつの行を選ぶ", () => {
    const entries = entriesOf(
      evenlySpacedEntrySpecs(3, "2031-10-08T01:20:00Z", secondMs),
    );
    expect(defaultScaleKeyOf(entries, readEntryTimes(entries))).toBe("record");
  });

  test("行の数が上限を超えるときは、行の数が上限以下になる最も細かい区切りの段階を選ぶ", () => {
    const entries = entriesOf(
      evenlySpacedEntrySpecs(
        defaultScaleRowLimit + 1,
        "2031-10-08T04:00:00Z",
        secondMs,
      ),
    );
    const times = readEntryTimes(entries);
    const chosen = timelineScaleOf(defaultScaleKeyOf(entries, times));
    const finer = finerScaleOf(chosen.key);

    const rowsAt = (scale: typeof chosen | undefined) => {
      if (scale === undefined || scale.kind === "record") {
        return entries.length;
      }
      return groupTimeline(entries, times, scale.widthMs).groups.length;
    };
    expect(chosen.kind).toBe("bucket");
    expect(rowsAt(chosen)).toBeLessThanOrEqual(defaultScaleRowLimit);
    expect(rowsAt(finer)).toBeGreaterThan(defaultScaleRowLimit);
  });
});
