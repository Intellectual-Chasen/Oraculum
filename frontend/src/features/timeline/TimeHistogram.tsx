import { Filter, FilterX } from "lucide-react";
import { memo, type PointerEvent, useEffect, useMemo, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import type { GraphTimeFilter } from "@/shared/api/graph";
import {
  fetchTimeHistogram,
  type TimeHistogramRequest,
} from "@/shared/api/timeHistogram";
import type {
  TimeHistogramResponse,
  TimeHistogramRow,
} from "@/shared/contracts/timeHistogram";
import type { FetchState } from "@/shared/lib/fetchState";
import { formatCount } from "@/shared/lib/format";
import { toVisibleRawText } from "@/shared/lib/rawText";
import { distinctTerminalNames } from "@/shared/lib/terminalSource";
import { localTextOf } from "@/shared/lib/timestampInstant";
import { useDisplayOffset } from "@/shared/ui/DisplayOffset";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { Hint } from "@/shared/ui/Hint";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList, type KeyValuePair } from "@/shared/ui/KeyValueList";
import { offsetUnknownLabel } from "@/shared/ui/TimestampOffsetNote";

/** 横に並べる区切りの数。 */
export const histogramColumns = 60;
/** 縦に並べる端末の数。残りは「その他」の行へまとめる。 */
const histogramTerminalRows = 8;
const otherRow = "その他";
const noTerminalRow = "端末なし";
/** 集約の行の key。端末のノードの識別子と重ならない文字列にする。 */
const otherRowKey = "row:other";
const noTerminalRowKey = "row:no-terminal";

export type Histogram = {
  startMs: number;
  /** 1 つの区切りの幅。 */
  stepMs: number;
  /**
   * 行ごとの、区切りごとの件数。行は件数の多い端末の順で、最後に「その他」を置く。key は端末の
   * ノードの識別子か行の印で、label は制御文字を可視の符号にした表示名である。
   */
  rows: { key: string; label: string; counts: number[] }[];
  /** 区切りの数。 */
  columnCount: number;
  maxCount: number;
};

/**
 * backend が数えた応答を描く形にする。件数の多い端末から histogramTerminalRows 行を出し、
 * 残りの端末は「その他」の行へ足す。区切りに入れたレコードが無いときは undefined を返す。
 *
 * **端末の表示名を可視の符号にする。** 表示名は原資料の文字列であり、書式文字 (U+202E など) を
 * そのまま描くと、別の名前に見せかけられる。
 */
export function histogramOf(
  response: TimeHistogramResponse,
  fileNamesByContent: ReadonlyMap<string, string> = new Map(),
): Histogram | undefined {
  if (response.start === undefined || response.rows.length === 0) {
    return undefined;
  }
  // 同じ表示名の端末は、端末を記録した収集元の表示名を足して見分ける。
  const names = distinctTerminalNames(
    response.rows.flatMap((row) =>
      row.terminal === undefined ? [] : [row.terminal],
    ),
    fileNamesByContent,
  );
  const labelOf = (row: TimeHistogramRow) => {
    const text =
      row.terminal === undefined ? undefined : names.get(row.terminal.id);
    return text === undefined ? noTerminalRow : toVisibleRawText(text);
  };
  const shown = response.rows.slice(0, histogramTerminalRows).map((row) => ({
    key: row.terminal?.id ?? noTerminalRowKey,
    label: labelOf(row),
    counts: [...row.counts],
  }));
  const rest = response.rows.slice(histogramTerminalRows);
  const columnCount = response.rows[0]?.counts.length ?? 0;
  const rows =
    rest.length === 0
      ? shown
      : [
          ...shown,
          {
            key: otherRowKey,
            label: otherRow,
            counts: Array.from({ length: columnCount }, (_, column) =>
              rest.reduce((sum, row) => sum + (row.counts[column] ?? 0), 0),
            ),
          },
        ];
  return {
    startMs: Date.parse(response.start),
    stepMs: response.stepMs,
    rows,
    columnCount,
    maxCount: Math.max(...rows.flatMap((row) => row.counts)),
  };
}

/**
 * 棒 1 本の期間を「始まり – 終わり」で書く。表示のタイムゾーン `offset` を選んでいるときは
 * そのタイムゾーンの時刻で書き、選んでいないときは UTC で書く。
 */
function columnRangeText(
  histogram: Histogram,
  column: number,
  offset: string,
): string {
  const text = (ms: number) => {
    const utc = new Date(ms).toISOString();
    return localTextOf(utc, offset) ?? utc;
  };
  const start = histogram.startMs + column * histogram.stepMs;
  return `${text(start)} – ${text(start + histogram.stepMs)}`;
}

/** 区切りの範囲を、ミリ秒の精度の期間の絞り込みにする。上端は範囲の最後の 1 ms である。 */
export function columnRangeFilter(
  histogram: Histogram,
  from: number,
  to: number,
): GraphTimeFilter {
  const [first, last] = from <= to ? [from, to] : [to, from];
  const text = (ms: number) => new Date(ms).toISOString();
  return {
    from: {
      text: text(histogram.startMs + first * histogram.stepMs),
      precision: "millisecond",
    },
    to: {
      text: text(histogram.startMs + (last + 1) * histogram.stepMs - 1),
      precision: "millisecond",
    },
    unit: "millisecond",
  };
}

/** 列が、適用している期間と重なるか。期間の端の文字列を読めないときは重ならないとする。 */
function appliedColumns(
  histogram: Histogram,
  timeFilter: GraphTimeFilter | undefined,
  column: number,
): boolean {
  if (timeFilter === undefined) return false;
  const from =
    timeFilter.from === undefined
      ? Number.NEGATIVE_INFINITY
      : Date.parse(timeFilter.from.text);
  const to =
    timeFilter.to === undefined
      ? Number.POSITIVE_INFINITY
      : Date.parse(timeFilter.to.text);
  const start = histogram.startMs + column * histogram.stepMs;
  return start <= to && start + histogram.stepMs > from;
}

/**
 * 棒に入れなかったレコードの件数を、先頭の「棒の外」の合計と、理由ごとの「名前: 値」の組にする。
 * 件数が 0 の組は返さない。UTC 時刻を持たないレコードは、時系列と同じくタイムゾーン不明と
 * 時刻なしに分ける。分析者が取れる対処が違う。
 */
function outsidePairs(response: TimeHistogramResponse): KeyValuePair[] {
  const parts = [
    { name: "精度が棒の幅より粗い", count: response.spanningRecordCount },
    { name: offsetUnknownLabel, count: response.localTimeRecordCount },
    { name: "時刻なし", count: response.undatedRecordCount },
  ];
  const outside = parts.reduce((sum, { count }) => sum + count, 0);
  if (outside === 0) return [];
  return [
    { name: "棒の外", value: formatCount(outside) },
    ...parts.flatMap(({ name, count }) =>
      count > 0 ? [{ name, value: formatCount(count) }] : [],
    ),
  ];
}

const cellWidth = 12;
const cellHeight = 18;
const minLabelWidth = 120;

/**
 * 端末の名前を描く幅。11px の文字で、ASCII を 7px、ほかの文字を 12px と見積もり、最も長い
 * 名前が格子に重ならない幅にする。
 */
function labelWidthOf(histogram: Histogram): number {
  const widths = histogram.rows.map((row) =>
    [...row.label].reduce(
      (sum, char) => sum + (char.charCodeAt(0) < 0x80 ? 7 : 12),
      8,
    ),
  );
  return Math.max(minLabelWidth, ...widths);
}

/** 目盛りを置く列の間隔。 */
const tickEveryColumns = 10;

/**
 * 目盛りの時刻を、表示のタイムゾーンの「月-日 時:分:秒」で書く。1 列が 1 秒より短いときは
 * ミリ秒まで書き、隣り合う目盛りを同じ文字列にしない。
 */
function tickText(ms: number, offset: string, stepMs: number): string {
  const utc = new Date(ms).toISOString();
  const text = localTextOf(utc, offset) ?? utc;
  return text.slice(5, stepMs < 1000 ? 23 : 19).replace("T", " ");
}

/**
 * 格子の下に、列の始まりの時刻の目盛りを出す。時刻は画面全体で選んだ表示のタイムゾーンで書き、
 * そのタイムゾーンを目盛りの後に出す。
 */
function TimeAxis({
  histogram,
  labelWidth,
}: {
  histogram: Histogram;
  labelWidth: number;
}) {
  const offset = useDisplayOffset();
  const columns = Array.from(
    { length: Math.ceil(histogram.columnCount / tickEveryColumns) },
    (_, index) => index * tickEveryColumns,
  );
  return (
    <>
      <ul
        aria-label="時刻の目盛り"
        className="histogram-axis"
        style={{ width: labelWidth + histogram.columnCount * cellWidth }}
      >
        {columns.map((column) => (
          <li key={column} style={{ left: labelWidth + column * cellWidth }}>
            {tickText(
              histogram.startMs + column * histogram.stepMs,
              offset,
              histogram.stepMs,
            )}
          </li>
        ))}
      </ul>
      <p className="note">{`タイムゾーン: ${offset === "" ? "UTC" : `UTC${offset}`}`}</p>
    </>
  );
}

/**
 * 列の範囲を選んで期間の絞り込みに使う欄。図を横に引く操作と同じ期間を、キーボードと読み上げでも
 * 選べるようにする。
 */
function ColumnRangeForm({
  histogram,
  onApply,
}: {
  histogram: Histogram;
  onApply: (filter: GraphTimeFilter) => void;
}) {
  const offset = useDisplayOffset();
  const [from, setFrom] = useState(0);
  const [to, setTo] = useState(histogram.columnCount - 1);
  const options = Array.from({ length: histogram.columnCount }, (_, column) => (
    // biome-ignore lint/suspicious/noArrayIndexKey: 列の位置が区切りを指す。
    <option key={column} value={column}>
      {columnRangeText(histogram, column, offset)}
    </option>
  ));
  return (
    <form
      className="histogram-range"
      onSubmit={(event) => {
        event.preventDefault();
        onApply(columnRangeFilter(histogram, from, to));
      }}
    >
      <label>
        始まりの棒
        <select
          value={from}
          onChange={(event) => setFrom(Number(event.target.value))}
        >
          {options}
        </select>
      </label>
      <label>
        終わりの棒
        <select
          value={to}
          onChange={(event) => setTo(Number(event.target.value))}
        >
          {options}
        </select>
      </label>
      <IconButton type="submit" label="期間のフィルタを適用">
        <Filter size={14} aria-hidden="true" />
      </IconButton>
    </form>
  );
}

/** 件数のある区切りと、行ごとの件数の表。図の件数を読み上げでも読めるようにする。 */
function HistogramTable({ histogram }: { histogram: Histogram }) {
  const offset = useDisplayOffset();
  const columns = Array.from(
    { length: histogram.columnCount },
    (_, column) => column,
  ).filter((column) =>
    histogram.rows.some((row) => (row.counts[column] ?? 0) > 0),
  );
  return (
    <details>
      <summary>件数の表</summary>
      <table className="histogram-table">
        <caption>棒ごとの端末別の件数</caption>
        <thead>
          <tr>
            <th scope="col">期間</th>
            {histogram.rows.map((row) => (
              <th scope="col" key={row.key}>
                {row.label}
              </th>
            ))}
            <th scope="col">合計</th>
          </tr>
        </thead>
        <tbody>
          {columns.map((column) => (
            <tr key={column}>
              <th scope="row">{columnRangeText(histogram, column, offset)}</th>
              {histogram.rows.map((row) => (
                <td key={row.key}>{formatCount(row.counts[column] ?? 0)}</td>
              ))}
              <td>
                {formatCount(
                  histogram.rows.reduce(
                    (sum, row) => sum + (row.counts[column] ?? 0),
                    0,
                  ),
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </details>
  );
}

/**
 * pointer を押した要素が暗黙に持つ pointer capture を外す。touch と pen では押した要素が capture を
 * 持ち、指を動かしても隣の列に pointerenter が起きず、1 列しか選べない。
 */
function releaseImplicitCapture(event: PointerEvent<Element>) {
  const target = event.currentTarget;
  if (target.hasPointerCapture?.(event.pointerId)) {
    target.releasePointerCapture(event.pointerId);
  }
}

/**
 * 件数を時間と端末の格子で描く。区切りを横に引いて選ぶか、列の範囲の欄で選ぶと、その範囲を期間の
 * 絞り込みに使う。
 *
 * **期間の絞り込みを外した全体を数える。** 絞り込んだ範囲だけを描くと、範囲を広げ直す操作が
 * 描いた格子の中でできない。
 */
export const TimeHistogram = memo(function TimeHistogram({
  request,
  version,
  timeFilter,
  onApplyTimeFilter,
  fileNamesByContent,
}: {
  /** 期間を除いた絞り込みの要求。上位の画面が useMemo で保つ。 */
  request: Omit<TimeHistogramRequest, "columns">;
  version: number;
  timeFilter: GraphTimeFilter | undefined;
  onApplyTimeFilter: (filter: GraphTimeFilter | undefined) => void;
  /** 収集元の内容の識別から表示名を探す表。同じ表示名の端末の行を見分けるのに使う。 */
  fileNamesByContent?: ReadonlyMap<string, string>;
}) {
  const [state, setState] = useState<FetchState<TimeHistogramResponse>>({
    status: "loading",
  });
  // biome-ignore lint/correctness/useExhaustiveDependencies: version が変わったときに、同じ条件で取り直す。
  useEffect(() => {
    const controller = new AbortController();
    setState({ status: "loading" });
    fetchTimeHistogram(
      { ...request, columns: histogramColumns },
      { signal: controller.signal },
    ).then(
      (result) => {
        if (controller.signal.aborted) return;
        setState(
          result.ok
            ? { status: "loaded", value: result.value }
            : { status: "failed", failure: result.failure },
        );
      },
      () => {
        if (controller.signal.aborted) return;
        setState({
          status: "failed",
          failure: buildFetchFailure("unexpected", "件数の分布の取得"),
        });
      },
    );
    return () => controller.abort();
  }, [request, version]);
  const histogram = useMemo(
    () =>
      state.status === "loaded"
        ? histogramOf(state.value, fileNamesByContent)
        : undefined,
    [state, fileNamesByContent],
  );
  const labelWidth =
    histogram === undefined ? minLabelWidth : labelWidthOf(histogram);
  const [drag, setDrag] = useState<{ from: number; to: number } | undefined>(
    undefined,
  );
  return (
    <section aria-label="件数の分布" className="time-histogram">
      <FetchStateView state={state} loadingDescription="集計中">
        {(response) => (
          <>
            {histogram === undefined ? (
              <p role="status">
                {response.start === undefined
                  ? "UTC 時刻を持つレコードなし"
                  : "棒に入るレコードなし"}
              </p>
            ) : (
              <p>
                <Hint text="ドラッグで期間のフィルタを適用">
                  {`棒の幅: ${
                    histogram.stepMs >= 1000
                      ? `${formatCount(histogram.stepMs / 1000)} 秒`
                      : `${formatCount(histogram.stepMs)} ミリ秒`
                  }`}
                </Hint>
              </p>
            )}
            <KeyValueList className="note" pairs={outsidePairs(response)} />
            {histogram === undefined ? null : (
              <>
                <svg
                  role="img"
                  aria-label="時間と端末ごとの件数"
                  width={labelWidth + histogram.columnCount * cellWidth}
                  height={histogram.rows.length * cellHeight}
                  onPointerLeave={() => setDrag(undefined)}
                  onPointerUp={() => setDrag(undefined)}
                  onPointerCancel={() => setDrag(undefined)}
                >
                  {histogram.rows.map((row, rowIndex) => (
                    <g key={row.key}>
                      <text x={0} y={rowIndex * cellHeight + 13}>
                        {row.label}
                      </text>
                      {row.counts.map((count, column) => {
                        const selected =
                          drag === undefined
                            ? appliedColumns(histogram, timeFilter, column)
                            : column >= Math.min(drag.from, drag.to) &&
                              column <= Math.max(drag.from, drag.to);
                        return (
                          <rect
                            // biome-ignore lint/suspicious/noArrayIndexKey: 列の位置が区切りを指す。
                            key={column}
                            data-column={column}
                            x={labelWidth + column * cellWidth}
                            y={rowIndex * cellHeight}
                            width={cellWidth - 1}
                            height={cellHeight - 1}
                            className={
                              selected ? "histogram-selected" : undefined
                            }
                            fillOpacity={
                              count === 0
                                ? 0.05
                                : 0.2 + (0.8 * count) / histogram.maxCount
                            }
                            onPointerDown={(event) => {
                              releaseImplicitCapture(event);
                              setDrag({ from: column, to: column });
                            }}
                            onPointerEnter={() =>
                              setDrag((current) =>
                                current === undefined
                                  ? undefined
                                  : { ...current, to: column },
                              )
                            }
                            onPointerUp={() => {
                              if (drag === undefined) return;
                              onApplyTimeFilter(
                                columnRangeFilter(histogram, drag.from, column),
                              );
                            }}
                          >
                            <title>{`${row.label}: ${formatCount(count)}`}</title>
                          </rect>
                        );
                      })}
                    </g>
                  ))}
                </svg>
                <TimeAxis histogram={histogram} labelWidth={labelWidth} />
                <ColumnRangeForm
                  key={`${histogram.startMs}:${histogram.stepMs}:${histogram.columnCount}`}
                  histogram={histogram}
                  onApply={onApplyTimeFilter}
                />
                <HistogramTable histogram={histogram} />
              </>
            )}
          </>
        )}
      </FetchStateView>
      {timeFilter === undefined ? null : (
        <IconButton
          label="期間のフィルタを解除"
          onPress={() => onApplyTimeFilter(undefined)}
        >
          <FilterX size={14} aria-hidden="true" />
        </IconButton>
      )}
    </section>
  );
});
