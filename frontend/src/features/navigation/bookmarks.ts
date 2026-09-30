import type { SourceRef } from "@/shared/api/graph";
import type { Timestamp } from "@/shared/contracts/common";
import type { EdgeKind, GraphEdge, GraphNode } from "@/shared/contracts/graph";
import type { SourceIdentity } from "@/shared/contracts/sources";
import type { TimelineEntry } from "@/shared/contracts/timeline";
import { edgeKindLabels } from "@/shared/lib/graphLabels";
import { describeRecordPosition } from "@/shared/lib/recordPosition";
import { utcTextOf } from "@/shared/lib/timestampInstant";
import { type Place, placeKey, placeLabel } from "./places";

/** ブックマークしたエッジ。端点の表示名は原資料の文字列のまま持つ。 */
export type EdgeBookmarkTarget = {
  id: string;
  kind: EdgeKind;
  sourceLabel: string;
  targetLabel: string;
};

/** ブックマークできる対象。見た場所に、エッジと収集元を足したものである。 */
export type BookmarkTarget =
  | Place
  | { kind: "edge"; edge: EdgeBookmarkTarget }
  | { kind: "source"; source: SourceRef };

/** ブックマークを付けた画面。時系列の行はレコードを対象にし、付けた画面を時系列とする。 */
export const bookmarkOrigins = [
  "node",
  "edge",
  "timeline",
  "record",
  "source",
] as const;
export type BookmarkOrigin = (typeof bookmarkOrigins)[number];

export const bookmarkOriginLabels: Record<BookmarkOrigin, string> = {
  node: "ノード",
  edge: "エッジ",
  timeline: "Timeline",
  record: "レコード",
  source: "収集元",
};

/** ブックマークに添える時刻が何の時刻か。 */
export const bookmarkTimeKinds = [
  "event",
  "edgeRange",
  "observedFirst",
] as const;
export type BookmarkTimeKind = (typeof bookmarkTimeKinds)[number];

export const bookmarkTimeKindLabels: Record<BookmarkTimeKind, string> = {
  event: "イベントの時刻",
  edgeRange: "エッジの期間の始まり",
  observedFirst: "記録期間の始まり",
};

/**
 * ブックマーク 1 件。`time` は付けた画面が持っていた時刻で、同じ対象でも付けた画面で変わる。
 * `addedAt` は付けた時刻の RFC 3339 の文字列で、表にする前に保存したブックマークは持たない。
 */
export type Bookmark = {
  target: BookmarkTarget;
  from: BookmarkOrigin;
  addedAt?: string;
  time?: { kind: BookmarkTimeKind; value: Timestamp };
};

/** 付ける前のブックマーク。付けた時刻は一覧を持つ側が足す。 */
export type NewBookmark = Omit<Bookmark, "addedAt">;

export function placeBookmarkOf(place: Place): NewBookmark {
  return { target: place, from: place.kind };
}

/** 時系列の行はレコードを対象にし、行の事象の時刻を添える。 */
export function timelineBookmarkOf(entry: TimelineEntry): NewBookmark {
  return {
    target: { kind: "record", ref: entry.recordRef },
    from: "timeline",
    time:
      entry.eventTime === undefined
        ? undefined
        : { kind: "event", value: entry.eventTime },
  };
}

/** ノードの表示名の原文。原文も正規化値も持たないノードは識別子で指す。 */
export function nodeLabelTextOf(node: GraphNode): string {
  return node.label.rawText ?? node.label.normalized ?? node.id;
}

export function edgeBookmarkOf(
  edge: GraphEdge,
  sourceLabel: string,
  targetLabel: string,
): NewBookmark {
  return {
    target: {
      kind: "edge",
      edge: { id: edge.id, kind: edge.kind, sourceLabel, targetLabel },
    },
    from: "edge",
    time:
      edge.applicableRange === undefined
        ? undefined
        : { kind: "edgeRange", value: edge.applicableRange.from },
  };
}

export function sourceBookmarkOf(source: SourceIdentity): NewBookmark {
  return {
    target: {
      kind: "source",
      source: { id: source.sourceId, label: source.fileName },
    },
    from: "source",
    time:
      source.observedRangeFirst === undefined
        ? undefined
        : { kind: "observedFirst", value: source.observedRangeFirst },
  };
}

/** 識別子だけが分かるエッジの鍵。`bookmarkKey` と同じ文字列になる。 */
export function edgeBookmarkKey(edgeId: string): string {
  return `edge:${edgeId}`;
}

/** 同じ対象を同じ文字列にする。 */
export function bookmarkKey(target: BookmarkTarget): string {
  switch (target.kind) {
    case "edge":
      return edgeBookmarkKey(target.edge.id);
    case "source":
      return `source:${target.source.id}`;
    default:
      return placeKey(target);
  }
}

/** 表の「名前」の列。原文の文字列を含むため、RawText で描く。 */
export function bookmarkName(target: BookmarkTarget): string {
  switch (target.kind) {
    case "record":
      return target.eventId === undefined
        ? target.ref.sourceFileName
        : `Event ID: ${target.eventId}、${target.ref.sourceFileName}`;
    case "node":
      return target.node.label;
    case "edge":
      return `${edgeKindLabels[target.edge.kind]} ${target.edge.sourceLabel} → ${target.edge.targetLabel}`;
    case "source":
      return target.source.label;
  }
}

/** 表の「位置」の列。レコードだけが持つ。 */
export function bookmarkPosition(target: BookmarkTarget): string | undefined {
  return target.kind === "record"
    ? describeRecordPosition(target.ref)
    : undefined;
}

/** 対象を 1 つの文字列で指す名前。レコードは収集元と位置と、分かっていれば Event ID で指す。 */
export function bookmarkTitle(target: BookmarkTarget): string {
  return target.kind === "record" ? placeLabel(target) : bookmarkName(target);
}

export function isBookmarked(
  bookmarks: readonly Bookmark[],
  target: BookmarkTarget,
): boolean {
  const key = bookmarkKey(target);
  return bookmarks.some((bookmark) => bookmarkKey(bookmark.target) === key);
}

/** 対象がブックマークにあれば外し、無ければ末尾に足す。 */
export function toggleBookmark(
  bookmarks: readonly Bookmark[],
  bookmark: Bookmark,
): Bookmark[] {
  const key = bookmarkKey(bookmark.target);
  return isBookmarked(bookmarks, bookmark.target)
    ? bookmarks.filter((item) => bookmarkKey(item.target) !== key)
    : [...bookmarks, bookmark];
}

export const bookmarkSortKeys = ["kind", "name", "time", "addedAt"] as const;
export type BookmarkSortKey = (typeof bookmarkSortKeys)[number];
export type BookmarkSort = { key: BookmarkSortKey; descending: boolean };

/** 並べ替えに使う値。値を持たない行は向きによらず末尾に置く。 */
function sortValue(
  bookmark: Bookmark,
  key: BookmarkSortKey,
): string | undefined {
  switch (key) {
    case "kind":
      return String(bookmarkOrigins.indexOf(bookmark.from));
    case "name":
      return bookmarkName(bookmark.target);
    case "time":
      return bookmark.time === undefined
        ? undefined
        : utcTextOf(bookmark.time.value);
    case "addedAt":
      return bookmark.addedAt;
  }
}

export type BookmarkFilter = {
  from: BookmarkOrigin | undefined;
  /** 名前か位置にこの文字列を含む行を残す。大文字と小文字を区別しない。 */
  text: string;
};

/** フィルタを適用して並べ替える。同じ値の行は付けた順を保つ。 */
export function arrangeBookmarks(
  bookmarks: readonly Bookmark[],
  filter: BookmarkFilter,
  sort: BookmarkSort,
): Bookmark[] {
  const text = filter.text.trim().toLowerCase();
  const kept = bookmarks.filter(
    (bookmark) =>
      (filter.from === undefined || bookmark.from === filter.from) &&
      (text === "" ||
        [bookmarkName(bookmark.target), bookmarkPosition(bookmark.target)].some(
          (value) => value?.toLowerCase().includes(text),
        )),
  );
  const direction = sort.descending ? -1 : 1;
  const compare =
    sort.key === "kind"
      ? (a: string, b: string) => Number(a) - Number(b)
      : (a: string, b: string) => a.localeCompare(b);
  return kept
    .map((bookmark, index) => ({
      bookmark,
      index,
      value: sortValue(bookmark, sort.key),
    }))
    .sort((a, b) => {
      if (a.value === undefined || b.value === undefined) {
        return a.value === b.value
          ? a.index - b.index
          : a.value === undefined
            ? 1
            : -1;
      }
      return direction * compare(a.value, b.value) || a.index - b.index;
    })
    .map((item) => item.bookmark);
}
