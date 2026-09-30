import { type HTMLAttributes, type ReactNode, useState } from "react";
import { type SortValue, sortRows } from "../lib/sortValue";
import { cn } from "./cn";
import { nextSort, SortHeader, type TableSort } from "./SortHeader";

/** 表の列 1 つ。 */
export type DataTableColumn<Row> = {
  /** 列を区別する key。 */
  key: string;
  /** 列の見出し。短い名詞にする。 */
  header: string;
  cell: (row: Row) => ReactNode;
  /** 数値の列。右に揃え、桁を揃える。 */
  numeric?: boolean;
  /** 行の見出しの列。`th scope="row"` で描く。 */
  rowHeader?: boolean;
  /** 等幅の書体で描く列 (hash、時刻、位置、識別の値)。 */
  mono?: boolean;
  /** 列の幅と折り返しを決める class (`short-cell`、`label-cell`、`wrapping-cell`)。 */
  className?: string;
  /**
   * 並べ替えに使う値。渡した列は見出しを押して並べ替えられる。件数と時刻は数、IP アドレスは
   * `ipSortValue` の値、名前は文字列で返す。値の無い行は undefined を返す。
   */
  sortValue?: (row: Row) => SortValue;
};

/**
 * 1 件の値を 1 行、値を列にした表。見出しは `th scope="col"`、行の見出しの列は
 * `th scope="row"`、ほかは `td` で描く。
 *
 * `label` は表の名前であり、既定では読み上げだけに渡す。`showCaption` で見出しとして出す。
 * `sortValue` を持つ列の見出しを押すと、昇順と降順を切り替えて並べ替える。並べ替える前は `rows` の
 * 順に描く。
 */
export function DataTable<Row>({
  label,
  showCaption = false,
  columns,
  rows,
  rowKey,
  rowProps,
  className,
}: {
  label: string;
  showCaption?: boolean;
  columns: DataTableColumn<Row>[];
  rows: readonly Row[];
  /** 行の key。`index` は並べ替える前の `rows` の中の位置である。 */
  rowKey: (row: Row, index: number) => string;
  /** 行の要素に渡す属性 (選択中の `aria-current` など)。 */
  rowProps?: (row: Row) => HTMLAttributes<HTMLTableRowElement>;
  className?: string;
}) {
  const [sort, setSort] = useState<TableSort>();
  const cellClass = (column: DataTableColumn<Row>) =>
    cn(
      column.numeric && "text-right tabular-nums",
      column.mono && "font-mono",
      column.className,
    ) || undefined;
  const sortValue = columns.find(
    (column) => column.key === sort?.key,
  )?.sortValue;
  const shown =
    sort === undefined || sortValue === undefined
      ? rows.map((row, index) => ({ row, index }))
      : sortRows(rows, sortValue, sort.direction);
  return (
    <table className={className}>
      <caption className={showCaption ? undefined : "sr-only"}>{label}</caption>
      <thead>
        <tr>
          {columns.map((column) =>
            column.sortValue === undefined ? (
              <th key={column.key} scope="col" className={cellClass(column)}>
                {column.header}
              </th>
            ) : (
              <SortHeader
                key={column.key}
                className={cellClass(column)}
                direction={
                  sort?.key === column.key ? sort.direction : undefined
                }
                onPress={() => setSort(nextSort(sort, column.key))}
              >
                {column.header}
              </SortHeader>
            ),
          )}
        </tr>
      </thead>
      <tbody>
        {shown.map(({ row, index }) => (
          <tr key={rowKey(row, index)} {...rowProps?.(row)}>
            {columns.map((column) =>
              column.rowHeader ? (
                <th key={column.key} scope="row" className={cellClass(column)}>
                  {column.cell(row)}
                </th>
              ) : (
                <td key={column.key} className={cellClass(column)}>
                  {column.cell(row)}
                </td>
              ),
            )}
          </tr>
        ))}
      </tbody>
    </table>
  );
}
