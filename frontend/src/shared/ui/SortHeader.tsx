import { ArrowDown, ArrowUp, ArrowUpDown } from "lucide-react";
import { type ReactNode, useMemo, useState } from "react";
import { type SortDirection, type SortValue, sortRows } from "../lib/sortValue";

/** 並べ替えの状態。押した列と向きを持つ。並べ替える前は undefined。 */
export type TableSort<Key extends string = string> = {
  key: Key;
  direction: SortDirection;
};

/** 列の見出しを押した後の並べ替えの状態。同じ列を押し直すと向きを変え、別の列は昇順から始める。 */
export function nextSort<Key extends string>(
  sort: TableSort<Key> | undefined,
  key: Key,
): TableSort<Key> {
  return {
    key,
    direction:
      sort?.key === key && sort.direction === "ascending"
        ? "descending"
        : "ascending",
  };
}

/**
 * 素の table の行を、見出しで選んだ列の値で並べ替える。`values` は列の key ごとの並べ替えの値で、
 * 描画をまたいで同じ組を渡す。`header(key)` を `SortHeader` に渡す。
 */
export function useSortedRows<Row, Key extends string>(
  rows: readonly Row[],
  values: Record<Key, (row: Row) => SortValue>,
) {
  const [sort, setSort] = useState<TableSort<Key>>();
  const sorted = useMemo(
    () =>
      sort === undefined
        ? rows
        : sortRows(rows, values[sort.key], sort.direction).map(
            ({ row }) => row,
          ),
    [rows, values, sort],
  );
  const header = (key: Key) => ({
    direction: sort?.key === key ? sort.direction : undefined,
    onPress: () => setSort(nextSort(sort, key)),
  });
  return { sorted, header };
}

/**
 * 並べ替えられる列の見出し。押すと `onPress` を呼ぶ。今の並びの向きを `aria-sort` と矢印の印で示す。
 * button は表の文字と同じ見た目で描く。
 */
export function SortHeader({
  direction,
  onPress,
  title,
  className,
  children,
}: {
  /** この列で並べているときの向き。並べていない列は undefined。 */
  direction: SortDirection | undefined;
  onPress: () => void;
  title?: string;
  className?: string;
  children: ReactNode;
}) {
  const Icon =
    direction === "ascending"
      ? ArrowUp
      : direction === "descending"
        ? ArrowDown
        : ArrowUpDown;
  return (
    <th scope="col" aria-sort={direction ?? "none"} className={className}>
      <button
        type="button"
        className="value-link sort-header"
        title={title}
        onClick={onPress}
      >
        {children}
        <Icon
          size={11}
          aria-hidden="true"
          className={direction === undefined ? "sort-idle" : undefined}
        />
      </button>
    </th>
  );
}
