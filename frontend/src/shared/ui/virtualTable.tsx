import type { CSSProperties } from "react";

/**
 * 表の見た目。
 *
 * **列の幅を表の幅の比で決める。** 描く行がスクロールで入れ替わるため、列の幅を中身で
 * 決めると、スクロールのたびに列の幅が変わる。
 */
export const virtualTableStyle: CSSProperties = {
  width: "100%",
  tableLayout: "fixed",
  borderCollapse: "collapse",
};

/** 見出しの欄の見た目。上端に留める指定は、すべての表の見出しに当てる CSS が持つ。 */
export const stickyHeaderCellStyle: CSSProperties = {
  textAlign: "left",
  verticalAlign: "bottom",
  padding: "0.25rem 0.5rem",
};

/** 原資料の長い文字列を、列の幅で折り返す。 */
export const virtualCellStyle: CSSProperties = {
  verticalAlign: "top",
  overflowWrap: "anywhere",
  padding: "0.25rem 0.5rem",
  borderTop: "1px solid GrayText",
};

/**
 * 描かない行の高さの空き。行を並べる `tbody` の前と後に、別の `tbody` として置き、
 * 支援技術には出さない。高さが 0 のときは何も描かない。
 */
export function SpacerBody({
  height,
  columnCount,
}: {
  height: number;
  columnCount: number;
}) {
  if (height <= 0) {
    return null;
  }
  return (
    <tbody aria-hidden="true">
      <tr>
        <td colSpan={columnCount} style={{ height, padding: 0, border: 0 }} />
      </tr>
    </tbody>
  );
}
