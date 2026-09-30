import { ZoomIn, ZoomOut } from "lucide-react";
import {
  type CSSProperties,
  useCallback,
  useLayoutEffect,
  useMemo,
  useRef,
} from "react";
import type { RecordLocator } from "@/shared/contracts/common";
import type {
  SourceCoverage,
  TimelineEntry,
} from "@/shared/contracts/timeline";
import { formatCount } from "@/shared/lib/format";
import { useContextMenu } from "@/shared/ui/ContextMenu";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList, type KeyValuePair } from "@/shared/ui/KeyValueList";
import { offsetUnknownLabel } from "@/shared/ui/TimestampOffsetNote";
import { useCopyText } from "@/shared/ui/useCopyText";
import { useVirtualRows } from "@/shared/ui/useVirtualRows";
import {
  coarserThanBucketLabel,
  timelineScaleLabels,
  timeUnreadableLabel,
} from "./labels";
import { timelineSourcesOf } from "./sourceLabels";
import { TimelineGroupTable } from "./TimelineGroupTable";
import { TimelineRecordTable } from "./TimelineRecordTable";
import {
  coarserScaleOf,
  defaultScaleKeyOf,
  finerScaleOf,
  groupTimeline,
  readEntryTimes,
  type TimelineGroup,
  type TimelineGrouping,
  type TimelineScaleKey,
  timelineScaleOf,
  timelineScales,
} from "./timelineGrouping";
import {
  type TimelineRowActions,
  timelineRowMenuContent,
} from "./timelineRowMenu";

/**
 * 描く前に見積もる行の高さ (px)。1 件ずつの行は端末とアカウントの識別鍵を並べるため、
 * 区切りの行より高い。描いた行は測った高さへ直す。
 */
const estimatedRecordRowHeight = 96;
const estimatedGroupRowHeight = 64;
/** 見えている範囲の前後に、先に描いておく高さ (px)。 */
const overscanPx = 800;

// 既知の制限: 全行の高さの和がブラウザーの要素の高さの上限 (Chromium で約 3,355 万 px) を
// 超えると、末尾の行までスクロールできない,
// 利用者が配置した実資料で測った。幅 1,400 px の画面に
// 1 件ずつの行で出すと、1 行あたり約 94 px だった,
// 1 回の応答の行が 30 万件を超えたときに見直す。
const scrollerStyle: CSSProperties = {
  maxHeight: "70vh",
  overflowY: "auto",
  overflowAnchor: "none",
  scrollbarGutter: "stable",
};

/**
 * 行の数と、区切りに入れたレコードと区切りと別の行に置いたレコードの件数を「名前: 値」の組で
 * 返す。件数が 0 の組は返さない。
 */
function groupingPairs(
  grouping: TimelineGrouping | undefined,
  entryCount: number,
): KeyValuePair[] {
  const pairs: KeyValuePair[] = [
    { name: "レコード", value: formatCount(entryCount) },
  ];
  if (grouping === undefined) return pairs;
  const { entryCounts } = grouping;
  const counted = (name: string, count: number) =>
    count > 0 ? [{ name, value: formatCount(count) }] : [];
  return [
    ...pairs,
    { name: "行", value: formatCount(grouping.groups.length) },
    { name: "区切りに入ったレコード", value: formatCount(entryCounts.bucket) },
    ...counted(coarserThanBucketLabel, entryCounts.coarser_than_bucket),
    ...counted(offsetUnknownLabel, entryCounts.offset_undetermined),
    ...counted(timeUnreadableLabel, entryCounts.time_unreadable),
  ];
}

type ScaleControlsProps = {
  scaleKey: TimelineScaleKey;
  onChange: (scaleKey: TimelineScaleKey) => void;
};

/** 区切りの幅を 1 段階ずつ変えるボタンと、段階を直接選ぶ欄。 */
function ScaleControls({ scaleKey, onChange }: ScaleControlsProps) {
  const coarser = coarserScaleOf(scaleKey);
  const finer = finerScaleOf(scaleKey);
  return (
    <fieldset className="m-0 flex min-w-0 items-center gap-1 border-0 p-0">
      <legend className="sr-only">区切りの幅</legend>
      <IconButton
        label="区切りを広げる"
        isDisabled={coarser === undefined}
        onPress={() => {
          if (coarser !== undefined) {
            onChange(coarser.key);
          }
        }}
      >
        <ZoomOut size={14} aria-hidden="true" />
      </IconButton>
      <label>
        区切りの幅{" "}
        <select
          value={scaleKey}
          onChange={(event) => {
            const chosen = timelineScales.find(
              (scale) => scale.key === event.target.value,
            );
            if (chosen !== undefined) {
              onChange(chosen.key);
            }
          }}
        >
          {timelineScales.map((scale) => (
            <option key={scale.key} value={scale.key}>
              {timelineScaleLabels[scale.key]}
            </option>
          ))}
        </select>
      </label>
      <IconButton
        label="区切りを狭める"
        isDisabled={finer === undefined}
        onPress={() => {
          if (finer !== undefined) {
            onChange(finer.key);
          }
        }}
      >
        <ZoomIn size={14} aria-hidden="true" />
      </IconButton>
    </fieldset>
  );
}

type TimelineListProps = {
  entries: TimelineEntry[];
  /** 内訳に並べる収集元の順と名前を決めるのに使う。 */
  sourceCoverages: SourceCoverage[];
  /** 分析者が選んだ段階。選んでいないときは、行の数から既定の段階を決める。 */
  chosenScale: TimelineScaleKey | undefined;
  onChooseScale: (scaleKey: TimelineScaleKey) => void;
  /** 行を選んだときに、元レコードの表示へ渡す。 */
  onSelectRecord: (recordRef: RecordLocator) => void;
  /** 1 件ずつの行のコンテキストメニューが呼ぶ操作。描画をまたいで同じ組を渡す。 */
  rowActions: TimelineRowActions;
  /**
   * 見せる行の `entries` での位置。変わるたびに 1 件ずつの段階でその行までスクロールし、
   * 行を強調する。出ない場合はスクロールしない。
   */
  focusedEntry?: number | undefined;
  onBefore?: (entry: TimelineEntry) => void;
};

/**
 * 根拠のレコードを時刻順の 1 本の列として出す。
 *
 * **応答が返した全件を 1 本のスクロールで辿れるようにし、見えている行とその前後だけを
 * 描く。** 縮小すると、時間の区切りごとに件数と収集元ごとの内訳を 1 行にまとめる。
 * 最も拡大した段階は 1 件ずつのレコードの行である。
 */
export function TimelineList(props: TimelineListProps) {
  if (props.entries.length === 0) {
    return null;
  }
  return <TimelineRows {...props} />;
}

function TimelineRows({
  entries,
  sourceCoverages,
  chosenScale,
  onChooseScale,
  onSelectRecord,
  rowActions,
  focusedEntry,
  onBefore,
}: TimelineListProps) {
  const { copy, notice } = useCopyText();
  const rowMenu = useContextMenu((entry: TimelineEntry) =>
    timelineRowMenuContent(entry, rowActions, copy),
  );
  const times = useMemo(() => readEntryTimes(entries), [entries]);
  const scaleKey = useMemo(
    () => chosenScale ?? defaultScaleKeyOf(entries, times),
    [chosenScale, entries, times],
  );
  const scale = timelineScaleOf(scaleKey);
  const grouping = useMemo(
    () =>
      scale.kind === "bucket"
        ? groupTimeline(entries, times, scale.widthMs)
        : undefined,
    [entries, times, scale],
  );
  const sources = useMemo(
    () => timelineSourcesOf(entries, sourceCoverages),
    [entries, sourceCoverages],
  );

  const virtual = useVirtualRows(
    grouping === undefined ? entries.length : grouping.groups.length,
    grouping ?? entries,
    grouping === undefined ? estimatedRecordRowHeight : estimatedGroupRowHeight,
    overscanPx,
  );
  const { firstVisibleRow, scrollToRow } = virtual;

  // **段階を変えても、画面の上端に見えていたレコードを上端に残す。** 段階を変えるたびに
  // 先頭へ戻ると、分析者は見ていた時刻を探し直す。
  const pendingAnchorRef = useRef<number | undefined>(undefined);
  useLayoutEffect(() => {
    const anchor = pendingAnchorRef.current;
    if (anchor === undefined) {
      return;
    }
    pendingAnchorRef.current = undefined;
    scrollToRow(
      grouping === undefined ? anchor : (grouping.groupOfEntry[anchor] ?? 0),
    );
  }, [grouping, scrollToRow]);

  const changeScale = useCallback(
    (next: TimelineScaleKey, anchorEntry?: number) => {
      if (next === scaleKey) {
        return;
      }
      const top = firstVisibleRow();
      pendingAnchorRef.current =
        anchorEntry ??
        (grouping === undefined
          ? top
          : (grouping.groups[top]?.firstEntryIndex ?? 0));
      onChooseScale(next);
    },
    [scaleKey, grouping, firstVisibleRow, onChooseScale],
  );

  // **見せる行が変わったら、1 件ずつの段階でその行までスクロールする。** まとめた段階では行が
  // 無いため、段階を 1 件ずつへ変え、変えた後の描画でスクロールする。
  // biome-ignore lint/correctness/useExhaustiveDependencies: 見せる行が変わったときだけ動かす。段階とスクロールの関数の変化では動かさない。
  useLayoutEffect(() => {
    if (focusedEntry === undefined) {
      return;
    }
    if (grouping !== undefined) {
      pendingAnchorRef.current = focusedEntry;
      onChooseScale("record");
      return;
    }
    scrollToRow(focusedEntry);
  }, [focusedEntry]);

  const finer = finerScaleOf(scaleKey);
  const zoomInto = useCallback(
    (group: TimelineGroup) => {
      const target =
        group.kind === "bucket" && finer !== undefined ? finer.key : "record";
      changeScale(target, group.firstEntryIndex);
    },
    [finer, changeScale],
  );

  return (
    <>
      <ScaleControls
        scaleKey={scaleKey}
        onChange={(next) => changeScale(next)}
      />
      <KeyValueList pairs={groupingPairs(grouping, entries.length)} />
      <section
        ref={virtual.scrollerRef}
        aria-label="時系列の表"
        // biome-ignore lint/a11y/noNoninteractiveTabindex: スクロールする領域をキーボードで動かせるようにする。
        tabIndex={0}
        onScroll={virtual.onScroll}
        style={scrollerStyle}
      >
        {grouping === undefined ? (
          <TimelineRecordTable
            entries={entries}
            start={virtual.start}
            end={virtual.end}
            spaceBefore={virtual.spaceBefore}
            spaceAfter={virtual.spaceAfter}
            bodyRef={virtual.bodyRef}
            onSelectRecord={onSelectRecord}
            rowMenu={rowMenu.triggers}
            menuEntry={rowMenu.openTarget}
            focusedEntry={focusedEntry}
            onBefore={onBefore}
          />
        ) : (
          <TimelineGroupTable
            groups={grouping.groups}
            sources={sources}
            bucketZoomLabel={
              finer === undefined || finer.kind === "record"
                ? "1 件ずつ表示"
                : `${timelineScaleLabels[finer.key]}ごとに表示`
            }
            separateZoomLabel="1 件ずつ表示"
            onZoom={zoomInto}
            start={virtual.start}
            end={virtual.end}
            spaceBefore={virtual.spaceBefore}
            spaceAfter={virtual.spaceAfter}
            bodyRef={virtual.bodyRef}
          />
        )}
      </section>
      {notice}
      {rowMenu.menu}
    </>
  );
}
