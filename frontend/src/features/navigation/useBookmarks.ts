import { useCallback, useRef, useState } from "react";
import {
  type Bookmark,
  bookmarkKey,
  type NewBookmark,
  toggleBookmark,
} from "./bookmarks";

/**
 * ブックマークの一覧を持つ。`isMarkedKey` と `toggle` は描画をまたいで同じ関数であり、memo の
 * 一覧の行メニューへ渡せる。`isMarkedKey` は呼んだ時点の一覧を `bookmarkKey` の鍵で探す。
 */
export function useBookmarks(
  now: () => string = () => new Date().toISOString(),
) {
  const [bookmarks, setBookmarks] = useState<Bookmark[]>([]);
  const latest = useRef(bookmarks);
  latest.current = bookmarks;
  const clock = useRef(now);
  clock.current = now;
  const isMarkedKey = useCallback(
    (key: string) =>
      latest.current.some((bookmark) => bookmarkKey(bookmark.target) === key),
    [],
  );
  const toggle = useCallback(
    (bookmark: NewBookmark) =>
      setBookmarks((list) =>
        toggleBookmark(list, { ...bookmark, addedAt: clock.current() }),
      ),
    [],
  );
  return { bookmarks, setBookmarks, isMarkedKey, toggle };
}
