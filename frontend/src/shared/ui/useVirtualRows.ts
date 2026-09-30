import {
  type RefObject,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";

/**
 * 行ごとの高さと、行の上端の位置。
 * `offsets[i]` は行 i の上端の位置で、`offsets[rowCount]` は全行の高さの和である。
 */
type RowLayout = {
  heights: Float64Array;
  offsets: Float64Array;
};

function createLayout(rowCount: number, estimatedHeight: number): RowLayout {
  const heights = new Float64Array(rowCount).fill(estimatedHeight);
  const offsets = new Float64Array(rowCount + 1);
  for (let index = 0; index < rowCount; index += 1) {
    offsets[index + 1] = (offsets[index] ?? 0) + estimatedHeight;
  }
  return { heights, offsets };
}

function rebuildOffsets(layout: RowLayout, from: number): void {
  for (let index = from; index < layout.heights.length; index += 1) {
    layout.offsets[index + 1] =
      (layout.offsets[index] ?? 0) + (layout.heights[index] ?? 0);
  }
}

/** 下端が位置 `top` より下にある最初の行を返す。無ければ行の数を返す。 */
function firstRowBelow(offsets: Float64Array, top: number): number {
  const rowCount = offsets.length - 1;
  let low = 0;
  let high = rowCount;
  while (low < high) {
    const middle = (low + high) >>> 1;
    if ((offsets[middle + 1] ?? 0) > top) {
      high = middle;
    } else {
      low = middle + 1;
    }
  }
  return low;
}

/** 上端が位置 `bottom` 以上にある最初の行を返す。無ければ行の数を返す。 */
function firstRowFrom(offsets: Float64Array, bottom: number): number {
  const rowCount = offsets.length - 1;
  let low = 0;
  let high = rowCount;
  while (low < high) {
    const middle = (low + high) >>> 1;
    if ((offsets[middle] ?? 0) >= bottom) {
      high = middle;
    } else {
      low = middle + 1;
    }
  }
  return low;
}

type RowWindow = { start: number; end: number; version: number };

/**
 * 画面の上端に見えている行を決めるときに、見えていない扱いにする行の下端の幅 (px)。
 * ブラウザーはスクロールの位置を画素へ丸めるため、上端へ出した行の 1 つ前の行の下端が、
 * 1 px 未満だけ見えている位置になる。
 */
const hiddenEdgePx = 1;

function windowAt(
  layout: RowLayout,
  scrollTop: number,
  viewportHeight: number,
  overscanPx: number,
): { start: number; end: number } {
  const rowCount = layout.heights.length;
  const total = layout.offsets[rowCount] ?? 0;
  const top = Math.min(
    Math.max(scrollTop, 0),
    Math.max(total - viewportHeight, 0),
  );
  const start = firstRowBelow(layout.offsets, top - overscanPx);
  const end = Math.max(
    firstRowFrom(layout.offsets, top + viewportHeight + overscanPx),
    Math.min(start + 1, rowCount),
  );
  return { start, end };
}

/** 見えている行とその前後だけを描くための、描く行の範囲と操作。 */
export type VirtualRows = {
  /** スクロールする要素に付ける。 */
  scrollerRef: RefObject<HTMLElement | null>;
  /** 行を並べる `tbody` に付ける。各行は `data-row-index` に行の位置を持つ。 */
  bodyRef: RefObject<HTMLTableSectionElement | null>;
  /** 描く行の範囲。`end` を含まない。 */
  start: number;
  end: number;
  /** 描かない行の高さの和。描く行の前と後に、この高さの空きを置く。 */
  spaceBefore: number;
  spaceAfter: number;
  /** スクロールする要素の `onScroll` に渡す。 */
  onScroll: () => void;
  /** 画面の上端に見えている行の位置を返す。 */
  firstVisibleRow: () => number;
  /** 行を画面の上端へ出す。 */
  scrollToRow: (index: number) => void;
};

/**
 * スクロールする要素の中で、行が始まる位置と、上端に留まる見出しの高さ。
 * `rowsOrigin` は、スクロールする要素の中身の上端から、行 0 の上端までの長さである。
 * 表の題と見出しの行の高さを含む。
 */
type ScrollFrame = { rowsOrigin: number; headerHeight: number };

/**
 * 行が始まる位置と見出しの高さを測る。スクロールする要素が画面に配置されていない
 * (高さが 0 の) ときは undefined を返し、呼ぶ側は前に測った値を使い続ける。
 */
function measureFrame(
  scroller: HTMLElement,
  body: HTMLTableSectionElement,
  renderedSpaceBefore: number,
): ScrollFrame | undefined {
  const scrollerBox = scroller.getBoundingClientRect();
  if (scrollerBox.height === 0) {
    return undefined;
  }
  const bodyTop =
    body.getBoundingClientRect().top - scrollerBox.top + scroller.scrollTop;
  const header = body.closest("table")?.tHead;
  return {
    rowsOrigin: bodyTop - renderedSpaceBefore,
    headerHeight: header?.getBoundingClientRect().height ?? 0,
  };
}

/**
 * 表の行のうち、スクロールする要素に見えている行とその前後だけを描く。
 * 表の見出しの行は、スクロールする要素の上端に留まる前提で位置を数える。
 *
 * **行の高さを描いた後に測る。** 行は原資料の文字列を折り返して出すため、高さが行ごとに
 * 違う。まだ描いていない行は `estimatedHeight` の高さとして位置を見積もり、描いた行を
 * 測って位置を直す。画面の上端に見えている行の位置がずれないよう、直した分だけ
 * スクロールの位置を動かす。スクロールする要素には `overflow-anchor: none` を付け、
 * ブラウザーによる位置の補正と重ねない。
 *
 * `rowsKey` が変わると、測った高さを捨てて見積もりから始める。
 */
export function useVirtualRows(
  rowCount: number,
  rowsKey: unknown,
  estimatedHeight: number,
  overscanPx: number,
): VirtualRows {
  const scrollerRef = useRef<HTMLElement | null>(null);
  const bodyRef = useRef<HTMLTableSectionElement | null>(null);
  const frameRef = useRef<ScrollFrame>({ rowsOrigin: 0, headerHeight: 0 });
  // rowsKey は行の並びが変わったことを知らせるだけで、見積もりの計算には使わない。
  // biome-ignore lint/correctness/useExhaustiveDependencies: 行の並びが変わったときに測った高さを捨てる。
  const layout = useMemo(
    () => createLayout(rowCount, estimatedHeight),
    [rowCount, estimatedHeight, rowsKey],
  );
  // スクロールする要素の高さは描いた後に測る。最初の範囲は画面の高さで見積もる。
  const [rowWindow, setRowWindow] = useState<RowWindow>(() => ({
    ...windowAt(layout, 0, window.innerHeight, overscanPx),
    version: 0,
  }));

  const updateWindow = useCallback(
    (measured: boolean) => {
      const scroller = scrollerRef.current;
      if (scroller === null) {
        return;
      }
      const next = windowAt(
        layout,
        scroller.scrollTop - frameRef.current.rowsOrigin,
        scroller.clientHeight,
        overscanPx,
      );
      setRowWindow((previous) =>
        !measured && previous.start === next.start && previous.end === next.end
          ? previous
          : { ...next, version: previous.version + 1 },
      );
    },
    [layout, overscanPx],
  );

  const start = Math.min(rowWindow.start, rowCount);
  const end = Math.min(Math.max(rowWindow.end, start), rowCount);
  const spaceBefore = layout.offsets[start] ?? 0;

  // **描いた後、画面に出す前に測る。** 測る前の位置で 1 度描くと、行の高さが見積もりと
  // 違う分だけ画面が跳ねて見える。依存を持たないのは、描くたびに描いた行を測るためである。
  useLayoutEffect(() => {
    const scroller = scrollerRef.current;
    const body = bodyRef.current;
    if (scroller === null || body === null) {
      return;
    }
    const frame = measureFrame(scroller, body, spaceBefore) ?? frameRef.current;
    frameRef.current = frame;
    // 見出しの行の下端に接する位置を、画面の上端に見えている行の位置として保つ。
    const scrollTop = scroller.scrollTop;
    const visibleTop = scrollTop - frame.rowsOrigin + frame.headerHeight;
    const anchor = firstRowBelow(layout.offsets, visibleTop + hiddenEdgePx);
    const anchorShift = visibleTop - (layout.offsets[anchor] ?? 0);
    let changedFrom = rowCount;
    // 1 件を複数の行で描く表は、同じ位置を持つ行の高さの和を 1 件の高さにする。
    const renderedHeights = new Map<number, number>();
    for (const row of Array.from(body.rows)) {
      const index = Number(row.dataset.rowIndex);
      if (!Number.isInteger(index) || index < 0 || index >= rowCount) {
        continue;
      }
      renderedHeights.set(
        index,
        (renderedHeights.get(index) ?? 0) + row.getBoundingClientRect().height,
      );
    }
    for (const [index, height] of renderedHeights) {
      // 高さ 0 は、要素が画面に配置されていない状態である。見積もりのまま残す。
      if (height > 0 && Math.abs(height - (layout.heights[index] ?? 0)) > 0.5) {
        layout.heights[index] = height;
        changedFrom = Math.min(changedFrom, index);
      }
    }
    if (changedFrom === rowCount) {
      updateWindow(false);
      return;
    }
    rebuildOffsets(layout, changedFrom);
    // 画面の上端に見えていた行の上端からのずれを、直した位置の上でも保つ。
    const anchoredTop =
      frame.rowsOrigin +
      (layout.offsets[anchor] ?? 0) +
      anchorShift -
      frame.headerHeight;
    if (scrollTop > 0 && Math.abs(anchoredTop - scrollTop) >= 1) {
      scroller.scrollTop = anchoredTop;
    }
    updateWindow(true);
  });

  const firstVisibleRow = useCallback(() => {
    const scroller = scrollerRef.current;
    const { rowsOrigin, headerHeight } = frameRef.current;
    const visibleTop = (scroller?.scrollTop ?? 0) - rowsOrigin + headerHeight;
    const index = firstRowBelow(layout.offsets, visibleTop + hiddenEdgePx);
    return Math.min(index, Math.max(rowCount - 1, 0));
  }, [layout, rowCount]);

  const scrollToRow = useCallback(
    (index: number) => {
      const scroller = scrollerRef.current;
      if (scroller === null) {
        return;
      }
      const bounded = Math.min(Math.max(index, 0), rowCount);
      const { rowsOrigin, headerHeight } = frameRef.current;
      // 行 0 は表の題と一緒に出す。
      scroller.scrollTop =
        bounded === 0
          ? 0
          : rowsOrigin + (layout.offsets[bounded] ?? 0) - headerHeight;
      updateWindow(false);
    },
    [layout, rowCount, updateWindow],
  );

  const onScroll = useCallback(() => updateWindow(false), [updateWindow]);

  // 画面の大きさが変わると、スクロールする要素の高さと行の折り返しが変わる。描き直して
  // 測り直す。
  useEffect(() => {
    const remeasure = () => updateWindow(true);
    window.addEventListener("resize", remeasure);
    return () => window.removeEventListener("resize", remeasure);
  }, [updateWindow]);

  return {
    scrollerRef,
    bodyRef,
    start,
    end,
    spaceBefore,
    spaceAfter: (layout.offsets[rowCount] ?? 0) - (layout.offsets[end] ?? 0),
    onScroll,
    firstVisibleRow,
    scrollToRow,
  };
}
