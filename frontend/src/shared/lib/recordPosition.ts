import type { RecordLocator, RecordRange } from "../contracts/common";
import { listKey } from "./listKey";
import { positionKindLabels } from "./recordLabels";

/**
 * 始まりと長さを持つ範囲を「名前: 始まり-終わり」で書く。終わりは始まりに長さを足した値である。
 * 長さが無いときは「名前: 始まり」で書く。
 */
export function describeSpan(
  name: string,
  start: number,
  length?: number,
): string {
  return `${name}: ${spanValue(start, length)}`;
}

/** 範囲の値だけを「始まり-終わり」で書く。長さが無いときは始まりだけを書く。 */
function spanValue(start: number, length?: number): string {
  return length === undefined ? `${start}` : `${start}-${start + length}`;
}

/** 行の範囲を「始まり-終わり」で書く。終わりは最後の行の番号であり、1 行だけのときは番号だけを書く。 */
function lineSpanValue(start: number, count?: number): string {
  return count === undefined || count <= 1
    ? `${start}`
    : `${start}-${start + count - 1}`;
}

/**
 * レコード位置の値だけを、名前を付けずに返す。CSV とコピーに使う。byte 範囲は「始まり-終わり」、
 * 通番と行番号はその番号を返す。位置の値が無いときは `undefined` を返す。
 */
export function recordPositionValue(
  recordRef: RecordLocator,
): string | undefined {
  const { positionKind, sequenceNumber, lineNumber, byteOffset, byteLength } =
    recordRef;
  switch (positionKind) {
    case "byte_range":
      return byteOffset === undefined || byteLength === undefined
        ? undefined
        : spanValue(byteOffset, byteLength);
    case "sequence_number":
      return sequenceNumber === undefined ? undefined : `${sequenceNumber}`;
    default:
      return lineNumber === undefined ? undefined : `${lineNumber}`;
  }
}

/** レコード位置の「名前: 値」の組 1 つ。 */
export type RecordPositionPart = {
  name: "ID" | "行" | "位置";
  value: string;
};

/**
 * レコード位置を「名前: 値」の組の並びで返す。画面はこの組で列と値の組を作る。
 * byte 範囲は「位置: 始まり-終わり」で、終わりは始まりに長さを足した値である。
 * 位置の値が無いときは空の配列を返す。
 */
export function recordPositionParts(
  recordRef: RecordLocator,
): RecordPositionPart[] {
  const {
    positionKind,
    sequenceNumber,
    lineNumber,
    lineCount,
    byteOffset,
    byteLength,
  } = recordRef;
  const line = (count?: number): RecordPositionPart[] =>
    lineNumber === undefined
      ? []
      : [{ name: "行", value: lineSpanValue(lineNumber, count) }];
  switch (positionKind) {
    case "sequence_number":
      return sequenceNumber === undefined
        ? []
        : [{ name: "ID", value: `${sequenceNumber}` }, ...line()];
    case "byte_range":
      // **1 レコードが複数行に分かれることを、行の範囲で画面に出す。** 先頭の行だけを出すと、
      // 6 行のイベントが 1 行のレコードと同じ見た目になる。
      return byteOffset === undefined || byteLength === undefined
        ? []
        : [
            ...line(lineCount),
            { name: "位置", value: spanValue(byteOffset, byteLength) },
          ];
    default:
      return line();
  }
}

/** 位置の値が無いレコードの表示。 */
export const recordPositionAbsent = "位置なし";

/**
 * レコード位置を 1 つの文字列に書く。`recordPositionParts` の組を「、」で並べる。
 * 列と値の組を作れない場所 (読み上げの名前、1 つのセル) だけで使う。
 */
export function describeRecordPosition(recordRef: RecordLocator): string {
  const parts = recordPositionParts(recordRef);
  return parts.length === 0
    ? recordPositionAbsent
    : parts.map((part) => `${part.name}: ${part.value}`).join("、");
}

/**
 * レコードの名前を、収集元の file 名と位置で 1 つの文字列に書く。Event ID が分かる Windows
 * イベントログのレコードは、グラフのレコードのノードの表示名と同じく Event ID を先頭に置く。
 */
export function describeRecordLocation(
  recordRef: RecordLocator,
  eventId?: string,
): string {
  return [
    ...(eventId === undefined ? [] : [`Event ID: ${eventId}`]),
    `収集元: ${recordRef.sourceFileName}`,
    describeRecordPosition(recordRef),
  ].join("、");
}

/** 収集元のすべてのレコードを指す範囲の表示。 */
export const wholeSourceRangeLabel = "収集元のすべて";

/** レコードの範囲 1 件を 1 つの文字列に書く。 */
export function describeRecordRange(range: RecordRange): string {
  if (range.rangeKind === "whole_source") {
    return wholeSourceRangeLabel;
  }
  const { positionKind, fromPosition, toPosition } = range;
  if (
    positionKind === undefined ||
    fromPosition === undefined ||
    toPosition === undefined
  ) {
    return "範囲なし";
  }
  return `${positionKindLabels[positionKind]}: ${fromPosition}-${toPosition}`;
}

/** レコード位置 1 件を一意に指す key を作る。表示のまとめ上げに使わない。 */
export function recordRefKey(recordRef: RecordLocator): string {
  return listKey([
    recordRef.sourceId,
    recordRef.sourceContentSha256,
    recordRef.positionKind,
    recordRef.sequenceNumber,
    recordRef.lineNumber,
    recordRef.byteOffset,
  ]);
}

/**
 * 位置を表す 10 進整数の下限。
 * 行番号は 1 起点であり、通番と byte 位置は符号を持たない
 * (定義元は `backend/core/record_locator.go` の `RecordLocator`)。
 */
const firstLineNumber = 1;
const lowestSequenceNumber = 0;
const lowestByteOffset = 0;

/** 収集元の中の位置。3 項目のうち 1 つ以上に値が入る。 */
export type RequestedRecordPosition = {
  sequenceNumber?: number;
  lineNumber?: number;
  byteOffset?: number;
};

function isWithinLowerBound(
  value: number | undefined,
  lowest: number,
): boolean {
  return (
    value === undefined || (Number.isSafeInteger(value) && value >= lowest)
  );
}

/**
 * 位置が要求に使える形であるかを返す。
 * **3 項目のどれも無い位置と、下限を下回る位置を要求に載せない。**
 */
export function isPositionUsable(position: RequestedRecordPosition): boolean {
  return (
    isWithinLowerBound(position.sequenceNumber, lowestSequenceNumber) &&
    isWithinLowerBound(position.lineNumber, firstLineNumber) &&
    isWithinLowerBound(position.byteOffset, lowestByteOffset) &&
    (position.sequenceNumber !== undefined ||
      position.lineNumber !== undefined ||
      position.byteOffset !== undefined)
  );
}

/** 位置の値を query の項目の文字列へ直す。値が無い項目は送らない。 */
export function positionText(value: number | undefined): string | undefined {
  return value === undefined ? undefined : String(value);
}
