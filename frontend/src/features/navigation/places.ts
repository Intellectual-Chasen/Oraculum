import type { NodeRef } from "@/shared/api/graph";
import type { RecordLocator } from "@/shared/contracts/common";
import { describeRecordLocation } from "@/shared/lib/recordPosition";

/**
 * 分析者が見た場所。開いたレコードか、グラフで選んだノードである。レコードの `eventId` は、
 * 開いたレコードを読み込んだ後に分かった Event ID である。
 */
export type Place =
  | { kind: "record"; ref: RecordLocator; eventId?: string }
  | { kind: "node"; node: NodeRef };

/** 同じ場所を同じ文字列にする。 */
export function placeKey(place: Place): string {
  if (place.kind === "node") return `node:${place.node.id}`;
  const { ref } = place;
  return [
    "record",
    ref.sourceId,
    ref.sequenceNumber,
    ref.lineNumber,
    ref.byteOffset,
  ].join(":");
}

/** 場所の表示名。 */
export function placeLabel(place: Place): string {
  return place.kind === "node"
    ? `ノード ${place.node.label}`
    : describeRecordLocation(place.ref, place.eventId);
}

/** 戻る・進むの履歴。`index` は今の場所の位置である。 */
export type PlaceHistory = { places: Place[]; index: number };

export const emptyHistory: PlaceHistory = { places: [], index: -1 };

/**
 * 今の場所の後に place を足し、進む先を捨てる。今の場所と同じ場所は足さずに、表示名だけを
 * place のものへ置き換える。グラフは応答を読む前に識別子を表示名として知らせ、読んだ後に
 * 表示名を知らせ直す。
 */
export function visit(history: PlaceHistory, place: Place): PlaceHistory {
  const current = history.places[history.index];
  if (current !== undefined && placeKey(current) === placeKey(place)) {
    const places = [...history.places];
    places[history.index] = place;
    return { ...history, places };
  }
  const places = [...history.places.slice(0, history.index + 1), place];
  return { places, index: places.length - 1 };
}

/**
 * 今の場所が place と同じ場所のときだけ、表示名を place のものへ置き換える。ほかの場所へ
 * 移った後に届いた表示名は捨てる。
 */
export function relabelCurrent(
  history: PlaceHistory,
  place: Place,
): PlaceHistory {
  const current = history.places[history.index];
  return current !== undefined && placeKey(current) === placeKey(place)
    ? visit(history, place)
    : history;
}

/** step だけ戻るか進む。範囲の外へは動かない。 */
export function move(history: PlaceHistory, step: -1 | 1): PlaceHistory {
  const index = history.index + step;
  return index < 0 || index >= history.places.length
    ? history
    : { ...history, index };
}
