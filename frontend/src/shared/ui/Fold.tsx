import { type ReactNode, useState } from "react";

// 既知の制限: 行数が foldedRowCount を超える欄を閉じた状態で出す, 利用者が配置した実資料で
// 測った。1 つのノードを記録したレコードは百件を超え、
// 1 本の推定される関係の根拠は数千件あり、開いたまま並べると右の列が数千行になった,
// 分析者が閉じた欄を毎回開く操作を実測したときに見直す
const foldedRowCount = 20;

/**
 * 行数の多くなりうる欄を開閉できる形で出す。
 *
 * **行数が少ないときは開いた状態で出し、多いときは閉じた状態で出す。** 閉じている間は
 * 中身を描かない。数千行の表を描いたまま隠すと、画面の応答が遅くなる。
 */
export function Fold({
  summary,
  summaryNote,
  rowCount,
  openLimit = foldedRowCount,
  children,
}: {
  /** 閉じていても見える見出し。件数を含める。 */
  summary: string;
  /** 見出しの横に薄い色で添える 1 行。 */
  summaryNote?: string;
  /** 中身の行数。開いた状態で出すかをこの数で決める。 */
  rowCount: number;
  /** 開いた状態で出す行数の上限。1 行が大きい塊の欄は小さい値を渡す。 */
  openLimit?: number;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(rowCount <= openLimit);
  return (
    <details
      className="fold"
      open={open}
      onToggle={(event) => setOpen(event.currentTarget.open)}
    >
      <summary className="fold-summary">
        {summary}
        {summaryNote === undefined ? null : (
          // 語の途中で折り返さず、入りきらないときは丸ごと次の行へ送る。
          <span className="ml-2 inline-block whitespace-nowrap text-xs font-normal text-muted">
            {summaryNote}
          </span>
        )}
      </summary>
      {open ? children : null}
    </details>
  );
}
