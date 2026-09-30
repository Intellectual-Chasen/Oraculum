import { ChartColumn, Maximize2, Minimize2 } from "lucide-react";
import { type ReactNode, useEffect, useMemo, useRef, useState } from "react";
import type { Timestamp } from "@/shared/contracts/common";
import {
  type EventIntervals,
  intervalBoundsMilliseconds,
  type ValueCount,
} from "@/shared/contracts/graph";
import { formatCount } from "@/shared/lib/format";
import { compareText } from "@/shared/lib/sortValue";
import { LocalTimeNote } from "@/shared/ui/DisplayOffset";
import { Hint } from "@/shared/ui/Hint";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { MissingValue } from "@/shared/ui/MissingValue";
import { RawText } from "@/shared/ui/RawText";
import { SortHeader } from "@/shared/ui/SortHeader";

/** 時刻を読み取れる根拠を 1 件も持たない値のセルに出すラベル。 */
const eventTimeAbsent = "時刻なし";

/** 時刻の原文を持たない値のセルに出すラベル。 */
const eventTimeRawTextAbsent = "原文なし";

/**
 * 時刻の両端の 1 つを書く。
 * **時刻が無いことと、時刻の原文が無いことを別のラベルで出す。**
 */
function EventTimeCell({ timestamp }: { timestamp: Timestamp | undefined }) {
  if (timestamp === undefined) {
    return <MissingValue description={eventTimeAbsent} />;
  }
  if (timestamp.rawText === undefined) {
    return <MissingValue description={eventTimeRawTextAbsent} />;
  }
  return (
    <>
      <RawText text={timestamp.rawText} />
      <LocalTimeNote timestamp={timestamp} />
    </>
  );
}

/** 表の並べ方。 */
type ValueCountOrder =
  | "count_descending"
  | "count_ascending"
  | "value"
  | "interval_spread";

/** ばらつきを比べるのに要る、時点を持つレコードの最少の件数。差は 3 つ以上になる。 */
const minTimedRecordsForSpread = 4;

/**
 * 時刻の差のばらつき。四分位の幅を中央値で割った値である。一定の間隔で繰り返す観測 (定期的な
 * 通信) ほど小さい。
 *
 * 次の値は、ばらつきを比べられないため undefined を返す。
 * - 差を持たない値と、時点を持つレコードが 4 件未満の値。差が 1 つか 2 つでは、四分位の幅が
 *   間隔の揃い方を表さない。
 * - 差の中央値が 0 の値。同じ時刻に集まった観測であり、間隔で繰り返す観測と読めない。
 */
function intervalSpread(counted: ValueCount): number | undefined {
  const intervals = counted.intervals;
  if (
    intervals === undefined ||
    intervals.timedRecordCount < minTimedRecordsForSpread ||
    intervals.medianMilliseconds === 0
  ) {
    return undefined;
  }
  return (
    (intervals.upperQuartileMilliseconds -
      intervals.lowerQuartileMilliseconds) /
    intervals.medianMilliseconds
  );
}

/**
 * 時刻の差のばらつきの小さい順に比べる。比べられない値は後ろに置く。ばらつきが等しいときは 0 を
 * 返し、呼ぶ側が件数と値の順で決める。
 */
function compareSpread(left: ValueCount, right: ValueCount): number {
  const leftSpread = intervalSpread(left);
  const rightSpread = intervalSpread(right);
  if (leftSpread === undefined || rightSpread === undefined) {
    if (leftSpread === rightSpread) return 0;
    return leftSpread === undefined ? 1 : -1;
  }
  return leftSpread < rightSpread ? -1 : leftSpread > rightSpread ? 1 : 0;
}

function sortedCounts(
  counts: ValueCount[],
  order: ValueCountOrder,
): ValueCount[] {
  return [...counts].sort((left, right) => {
    if (order === "interval_spread") {
      const bySpread = compareSpread(left, right);
      if (bySpread !== 0) return bySpread;
      if (left.recordCount !== right.recordCount) {
        return right.recordCount - left.recordCount;
      }
      return compareText(left.value, right.value);
    }
    if (order !== "value" && left.recordCount !== right.recordCount) {
      return order === "count_descending"
        ? right.recordCount - left.recordCount
        : left.recordCount - right.recordCount;
    }
    return compareText(left.value, right.value);
  });
}

/**
 * ミリ秒の差を、読める単位の文字列にする。10 分未満の差は秒で書き、分の端数で丸めない。
 * 階級の境界は round を真にして、切りのよい単位で書く。
 */
function formatMilliseconds(value: number, round = false): string {
  if (value < 1_000) return `${formatCount(value)} ミリ秒`;
  if (value < (round ? 60_000 : 600_000))
    return `${(value / 1_000).toFixed(value % 1_000 === 0 ? 0 : 1)} 秒`;
  if (value < 3_600_000)
    return `${(value / 60_000).toFixed(value % 60_000 === 0 ? 0 : 1)} 分`;
  if (value < 86_400_000)
    return `${(value / 3_600_000).toFixed(value % 3_600_000 === 0 ? 0 : 1)} 時間`;
  return `${(value / 86_400_000).toFixed(value % 86_400_000 === 0 ? 0 : 1)} 日`;
}

/** 階級 at の範囲の文字列。 */
function binLabel(at: number): string {
  const lower = at === 0 ? 0 : intervalBoundsMilliseconds[at - 1];
  const upper = intervalBoundsMilliseconds[at];
  const from = lower === undefined ? "" : formatMilliseconds(lower, true);
  return upper === undefined
    ? `${from} 以上`
    : `${from} 以上 ${formatMilliseconds(upper, true)} 未満`;
}

/**
 * 選んだ値のレコードの時刻の隣り合う差の分布を出す。
 * **差の分布だけを示し、差の意味を名付けない。**
 */
function IntervalDistribution({ counted }: { counted: ValueCount }) {
  const intervals: EventIntervals | undefined = counted.intervals;
  if (intervals === undefined) return null;
  const untimed = counted.recordCount - intervals.timedRecordCount;
  return (
    <section aria-label="選択中の値の時刻の間隔の分布">
      <h4>
        <Hint text="端末と接続元で分けずに時刻の順に並べた、隣り合う 2 件の間隔">
          時刻の間隔の分布
        </Hint>
      </h4>
      <KeyValueList
        pairs={[
          { name: "値", value: <RawText text={counted.value} /> },
          {
            name: "対象のレコード",
            value: formatCount(intervals.timedRecordCount),
          },
          {
            name: "間隔",
            value: formatCount(intervals.timedRecordCount - 1),
          },
          {
            name: "UTC 時刻の無いレコード",
            value: untimed > 0 ? formatCount(untimed) : undefined,
          },
          {
            name: "秒単位の時刻",
            value: intervals.coarsePrecision ? (
              <Hint text="1 秒未満の間隔: 同じ秒の 2 件を含む値">含む</Hint>
            ) : undefined,
          },
        ]}
      />
      <KeyValueList
        pairs={[
          {
            name: "最小",
            value: formatMilliseconds(intervals.minMilliseconds),
          },
          {
            name: "第 1 四分位",
            value: formatMilliseconds(intervals.lowerQuartileMilliseconds),
          },
          {
            name: "中央値",
            value: formatMilliseconds(intervals.medianMilliseconds),
          },
          {
            name: "第 3 四分位",
            value: formatMilliseconds(intervals.upperQuartileMilliseconds),
          },
          {
            name: "最大",
            value: formatMilliseconds(intervals.maxMilliseconds),
          },
        ]}
      />
      <table>
        <caption>間隔の範囲ごとの件数</caption>
        <thead>
          <tr>
            <th scope="col">間隔の範囲</th>
            <th scope="col">件数</th>
          </tr>
        </thead>
        <tbody>
          {intervals.binCounts.map((count, at) =>
            count === 0 ? null : (
              // 階級は固定の並びであり、位置が階級を指す。
              // biome-ignore lint/suspicious/noArrayIndexKey: 位置が階級の識別である。
              <tr key={at}>
                <td>{binLabel(at)}</td>
                <td>{formatCount(count)}</td>
              </tr>
            ),
          )}
        </tbody>
      </table>
    </section>
  );
}

function ValueCountRow({
  counted,
  onSelect,
}: {
  counted: ValueCount;
  onSelect: (value: string) => void;
}) {
  return (
    <tr>
      <td className="wrapping-cell">
        <RawText text={counted.value} />
      </td>
      <td>{formatCount(counted.recordCount)}</td>
      <td>
        <EventTimeCell timestamp={counted.firstEventTime} />
      </td>
      <td>
        <EventTimeCell timestamp={counted.lastEventTime} />
      </td>
      <td>
        {counted.intervals === undefined ? (
          <MissingValue description="UTC 時刻が 2 件未満" />
        ) : (
          <IconButton
            label={`${counted.value} の時刻の間隔の分布を表示`}
            onPress={() => onSelect(counted.value)}
          >
            <ChartColumn size={14} aria-hidden="true" />
          </IconButton>
        )}
      </td>
    </tr>
  );
}

const noWrap = { whiteSpace: "nowrap" } as const;

/** 表を画面の幅で出す枠の大きさ。 */
const wideFrame = {
  width: "90vw",
  height: "90vh",
  maxWidth: "none",
  maxHeight: "none",
  overflow: "auto",
  padding: "12px",
  background: "var(--surface, white)",
  border: "1px solid var(--line, gray)",
} as const;

/**
 * 件数を集計する欄に観測した値を 1 行ずつ出す。
 *
 * 件数は絞り込みに合ったノードの全件から数えた、その値を観測したレコードの件数である。
 * **根拠のレコードを出さない。** 操作 9 は根拠の中身を含まない。
 *
 * 並べ方は件数の多い順から始まる。同じ件数の行は値の文字列の順に並ぶ。選んだ値は文字列で持ち、
 * 応答が変わっても同じ値の新しい分布を出す。数える欄が変わったら、呼び出し側が key を替えて
 * 選択を捨てる。
 */
export function ValueCountList({
  counts,
  caption = "フィールドの値ごとのレコード数",
  absentSelection = "フィルタの結果に無い値",
}: {
  counts: ValueCount[];
  /** 表の見出し。数えた範囲を言う。 */
  caption?: string;
  /** 選んだ値が新しい結果に無いときのラベル。 */
  absentSelection?: string;
}) {
  const [order, setOrder] = useState<ValueCountOrder>("count_descending");
  const [selected, setSelected] = useState<string | undefined>(undefined);
  const [wide, setWide] = useState(false);
  const rows = useMemo(() => sortedCounts(counts, order), [counts, order]);
  const selectedCount =
    selected === undefined
      ? undefined
      : counts.find((count) => count.value === selected);
  // 狭い区画に置いた表は、画面の幅の枠へ移して読む。
  const content = (
    <>
      <IconButton
        label={wide ? "元の幅に戻す" : "表を広げる"}
        onPress={() => setWide(!wide)}
      >
        {wide ? (
          <Minimize2 size={14} aria-hidden="true" />
        ) : (
          <Maximize2 size={14} aria-hidden="true" />
        )}
      </IconButton>
      {selected === undefined ? null : selectedCount === undefined ? (
        <p role="status">{absentSelection}</p>
      ) : (
        <IntervalDistribution counted={selectedCount} />
      )}
      {/* 狭い区画でも欄を折り返さず、1 件を 1 行で出す。区画の幅を超える分は横にずらして読む。 */}
      <table style={noWrap}>
        <caption>{caption}</caption>
        <thead>
          <tr>
            <SortHeader
              direction={order === "value" ? "ascending" : undefined}
              onPress={() => setOrder("value")}
            >
              値
            </SortHeader>
            <SortHeader
              direction={
                order === "count_descending"
                  ? "descending"
                  : order === "count_ascending"
                    ? "ascending"
                    : undefined
              }
              onPress={() =>
                setOrder(
                  order === "count_descending"
                    ? "count_ascending"
                    : "count_descending",
                )
              }
            >
              レコード数
            </SortHeader>
            <th scope="col">最初の時刻</th>
            <th scope="col">最後の時刻</th>
            <SortHeader
              direction={order === "interval_spread" ? "ascending" : undefined}
              title="間隔の揃った順: 四分位の幅 / 中央値"
              onPress={() => setOrder("interval_spread")}
            >
              時刻の間隔
            </SortHeader>
          </tr>
        </thead>
        <tbody>
          {rows.map((counted) => (
            <ValueCountRow
              key={counted.value}
              counted={counted}
              onSelect={setSelected}
            />
          ))}
        </tbody>
      </table>
    </>
  );
  return wide ? (
    <WideFrame label={caption} onClose={() => setWide(false)}>
      {content}
    </WideFrame>
  ) : (
    <div>{content}</div>
  );
}

/**
 * 表を画面の幅で出す modal の dialog。showModal で開き、ブラウザが focus の移動と Escape で
 * 閉じる操作と背面の操作の停止を行う。top layer に出るため、区画の配置の基準に縛られない。
 */
function WideFrame({
  label,
  onClose,
  children,
}: {
  label: string;
  onClose: () => void;
  children: ReactNode;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    ref.current?.showModal();
  }, []);
  return (
    <dialog ref={ref} aria-label={label} style={wideFrame} onClose={onClose}>
      {children}
    </dialog>
  );
}
