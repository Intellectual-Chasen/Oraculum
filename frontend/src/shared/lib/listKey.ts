import type { RecordLocator } from "@/shared/contracts/common";

/**
 * 一覧の要素 1 件を一意に指す key を作る。
 *
 * **分離子を NUL にする。** 材料が持つのは原資料の文字列であり、表示できる区切り文字を
 * 使うと、材料の値が同じ文字を含むときに別の要素が同じ key になる。値が無い項目は
 * 空の文字列にする。
 *
 * 表示のまとめ上げに使わない。
 */
export function listKey(parts: (string | number | undefined)[]): string {
  return parts
    .map((part) => (part === undefined ? "" : String(part)))
    .join("\0");
}

/**
 * レコード 1 件を一覧の中で指す key を作る。収集元と、入力形式が持つ位置の値のすべてを含める。
 * 通番と行番号を持たない入力形式では、byte 位置だけがレコードを区別する。
 */
export function recordLocatorKey(ref: RecordLocator): string {
  return listKey([
    ref.sourceId,
    ref.positionKind,
    ref.sequenceNumber,
    ref.lineNumber,
    ref.byteOffset,
  ]);
}
