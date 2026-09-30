import { Trash2 } from "lucide-react";
import { useState } from "react";
import { formatCount } from "@/shared/lib/format";
import { toVisibleRawText } from "@/shared/lib/rawText";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { Highlighted } from "@/shared/ui/Highlighted";
import { Hint } from "@/shared/ui/Hint";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { MissingValue } from "@/shared/ui/MissingValue";
import { SortHeader } from "@/shared/ui/SortHeader";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import { TimestampText } from "@/shared/ui/TimestampText";
import {
  arrangeBookmarks,
  type Bookmark,
  type BookmarkOrigin,
  type BookmarkSort,
  type BookmarkSortKey,
  type BookmarkTarget,
  bookmarkKey,
  bookmarkName,
  bookmarkOriginLabels,
  bookmarkOrigins,
  bookmarkPosition,
  bookmarkTimeKindLabels,
} from "./bookmarks";

const sortLabels: Record<BookmarkSortKey, string> = {
  kind: "種類",
  name: "名前",
  time: "時刻",
  addedAt: "登録日時",
};

const defaultSort: BookmarkSort = { key: "addedAt", descending: true };

/**
 * ブックマークを表にし、種類と文字列でフィルタを適用して並べ替える。一覧は上位の画面が持ち、
 * ワークスペースに保存する。並べ替えとフィルタは保存しない。
 */
export function Bookmarks({
  bookmarks,
  onChangeBookmarks,
  onOpen,
}: {
  bookmarks: readonly Bookmark[];
  onChangeBookmarks: (bookmarks: Bookmark[]) => void;
  /** 対象を開く。対象が今の解析結果に無くて開けなかったときは false を返す。 */
  onOpen: (target: BookmarkTarget) => boolean;
}) {
  const [sort, setSort] = useState(defaultSort);
  const [from, setFrom] = useState<BookmarkOrigin | undefined>(undefined);
  const [text, setText] = useState("");
  // 開けなかったブックマークの鍵。開き直して開けたら外す。
  const [missing, setMissing] = useState<ReadonlySet<string>>(new Set());
  if (bookmarks.length === 0) {
    return (
      <div className="note flex items-center gap-1">
        <KeyValueList pairs={[{ name: "ブックマーク", value: "なし" }]} />
        <HelpPopover label="ブックマークの追加">
          <KeyValueList
            stacked
            pairs={[
              {
                name: "ビューの操作",
                value: "履歴・Record・Node Detail・Edge Detail・Artifacts",
              },
              { name: "行のメニュー", value: "Timeline・Nodes・Edges" },
            ]}
          />
        </HelpPopover>
      </div>
    );
  }
  const rows = arrangeBookmarks(bookmarks, { from, text }, sort);
  const open = (target: BookmarkTarget) => {
    const key = bookmarkKey(target);
    const opened = onOpen(target);
    setMissing((current) => {
      const next = new Set(current);
      if (opened) next.delete(key);
      else next.add(key);
      return next;
    });
  };
  const remove = (target: BookmarkTarget) => {
    const key = bookmarkKey(target);
    onChangeBookmarks(
      bookmarks.filter((item) => bookmarkKey(item.target) !== key),
    );
    // 付け直した同じ対象に、開く前から対象が無いことを出さない。
    setMissing((current) => {
      const next = new Set(current);
      next.delete(key);
      return next;
    });
  };
  const sortHeader = (key: BookmarkSortKey) => (
    <SortHeader
      direction={
        sort.key !== key
          ? undefined
          : sort.descending
            ? "descending"
            : "ascending"
      }
      onPress={() =>
        setSort({
          key,
          descending: sort.key === key ? !sort.descending : false,
        })
      }
    >
      {sortLabels[key]}
    </SortHeader>
  );
  return (
    <section aria-label="ブックマーク">
      <div className="flex flex-wrap items-center gap-2">
        <label>
          種類{" "}
          <select
            value={from ?? ""}
            onChange={(event) =>
              setFrom(
                bookmarkOrigins.find((origin) => origin === event.target.value),
              )
            }
          >
            <option value="">すべて</option>
            {bookmarkOrigins.map((origin) => (
              <option key={origin} value={origin}>
                {bookmarkOriginLabels[origin]}
              </option>
            ))}
          </select>
        </label>{" "}
        <label>
          <Hint text="名前か位置に含む文字列">文字列</Hint>{" "}
          <input
            type="search"
            value={text}
            onChange={(event) => setText(event.target.value)}
          />
        </label>
        <div role="status">
          <KeyValueList
            pairs={[{ name: "件数", value: formatCount(rows.length) }]}
          />
        </div>
      </div>
      {rows.length === 0 ? null : (
        <table>
          <caption className="sr-only">ブックマーク</caption>
          <thead>
            <tr>
              <th scope="col" className="row-actions">
                <span className="sr-only">操作</span>
              </th>
              {sortHeader("kind")}
              {sortHeader("name")}
              {sortHeader("time")}
              <th scope="col">位置</th>
              {sortHeader("addedAt")}
            </tr>
          </thead>
          <tbody>
            {rows.map((bookmark) => (
              <BookmarkRow
                key={bookmarkKey(bookmark.target)}
                bookmark={bookmark}
                missing={missing.has(bookmarkKey(bookmark.target))}
                onOpen={() => open(bookmark.target)}
                onRemove={() => remove(bookmark.target)}
              />
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}

function BookmarkRow({
  bookmark,
  missing,
  onOpen,
  onRemove,
}: {
  bookmark: Bookmark;
  missing: boolean;
  onOpen: () => void;
  onRemove: () => void;
}) {
  const name = bookmarkName(bookmark.target);
  const position = bookmarkPosition(bookmark.target);
  return (
    <tr>
      <td className="row-actions">
        <IconButton
          label={`${toVisibleRawText(name)} をブックマークから削除`}
          onPress={onRemove}
        >
          <Trash2 size={14} aria-hidden="true" />
        </IconButton>
      </td>
      <td>{bookmarkOriginLabels[bookmark.from]}</td>
      <td>
        <button type="button" onClick={onOpen}>
          <Highlighted text={name} />
        </button>
        {missing ? (
          <div role="status">
            <StatusLabel status="failed" label="解析結果になし" />
          </div>
        ) : null}
      </td>
      <td>
        {bookmark.time === undefined ? (
          <MissingValue description="時刻なし" />
        ) : (
          <KeyValueList
            pairs={[
              {
                name: bookmarkTimeKindLabels[bookmark.time.kind],
                value: <TimestampText timestamp={bookmark.time.value} />,
              },
            ]}
          />
        )}
      </td>
      <td className="font-mono">
        {position ?? <MissingValue description="位置なし" />}
      </td>
      <td className="font-mono">
        {bookmark.addedAt ?? <MissingValue description="登録日時なし" />}
      </td>
    </tr>
  );
}
