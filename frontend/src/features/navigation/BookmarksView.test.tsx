// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { GraphEdge } from "@/shared/contracts/graph";
import type { TimelineEntry } from "@/shared/contracts/timeline";
import { edgeKindLabels } from "@/shared/lib/graphLabels";
import { timelineResponseJson } from "@/testdata/timeline/timelineResponse";
import { Bookmarks } from "./BookmarksView";
import {
  type Bookmark,
  edgeBookmarkOf,
  placeBookmarkOf,
  timelineBookmarkOf,
} from "./bookmarks";

afterEach(() => {
  cleanup();
  localStorage.clear();
});

const [early, late] = timelineResponseJson().entries as TimelineEntry[];
if (early === undefined || late === undefined) throw new Error("fixture");

const edge = {
  id: "e:1",
  kind: "process_parent_child",
  state: "observed",
  sourceNodeId: "n:1",
  targetNodeId: "n:2",
  applicableRange: { from: late.eventTime, to: late.eventTime },
  evidenceCount: 1,
} as GraphEdge;
const edgeName = `${edgeKindLabels.process_parent_child} cmd.exe → whoami.exe`;

/** 付けた順は node → 時系列 → edge。node は登録日時を持たない。 */
const bookmarks: Bookmark[] = [
  placeBookmarkOf({ kind: "node", node: { id: "n:9", label: "HOST-B" } }),
  { ...timelineBookmarkOf(early), addedAt: "2031-10-09T00:00:01.000Z" },
  {
    ...edgeBookmarkOf(edge, "cmd.exe", "whoami.exe"),
    addedAt: "2031-10-09T00:00:02.000Z",
  },
];

/** 表の本体の行の「名前」の欄を上から読む。 */
function names(): string[] {
  const [, body] = screen.getAllByRole("rowgroup");
  if (body === undefined) throw new Error("no body");
  return within(body)
    .getAllByRole("row")
    .map((row) => within(row).getAllByRole("cell")[2]?.textContent ?? "");
}

test("ブックマークが無いときは無いことを出し、付ける場所を「?」の説明で読める", () => {
  render(
    <Bookmarks bookmarks={[]} onOpen={vi.fn()} onChangeBookmarks={vi.fn()} />,
  );
  expect(screen.getByText("なし")).toBeDefined();
  fireEvent.click(
    screen.getByRole("button", { name: "ブックマークの追加 の説明" }),
  );
  const help = screen.getByRole("tooltip").textContent ?? "";
  expect(help).toContain("Node Detail");
  expect(help).toContain("行のメニュー");
});

test("既定は登録日時の新しい順で、登録日時の無い行は末尾に置く", () => {
  render(
    <Bookmarks
      bookmarks={bookmarks}
      onOpen={vi.fn()}
      onChangeBookmarks={vi.fn()}
    />,
  );
  expect(names()).toEqual([edgeName, early.recordRef.sourceFileName, "HOST-B"]);
  expect(screen.getByRole("columnheader", { name: "登録日時" }).ariaSort).toBe(
    "descending",
  );
});

test("見出しを押すと昇順、もう一度押すと降順にし、時刻の無い行は向きによらず末尾に置く", () => {
  render(
    <Bookmarks
      bookmarks={bookmarks}
      onOpen={vi.fn()}
      onChangeBookmarks={vi.fn()}
    />,
  );
  const time = screen.getByRole("button", { name: "時刻" });
  fireEvent.click(time);
  expect(names()).toEqual([early.recordRef.sourceFileName, edgeName, "HOST-B"]);
  fireEvent.click(time);
  expect(names()).toEqual([edgeName, early.recordRef.sourceFileName, "HOST-B"]);
  expect(screen.getByRole("columnheader", { name: "時刻" }).ariaSort).toBe(
    "descending",
  );

  fireEvent.click(screen.getByRole("button", { name: "種類" }));
  expect(names()).toEqual(["HOST-B", edgeName, early.recordRef.sourceFileName]);
});

test("時刻の欄は何の時刻かを添え、無い行は時刻なしと出す", () => {
  render(
    <Bookmarks
      bookmarks={bookmarks}
      onOpen={vi.fn()}
      onChangeBookmarks={vi.fn()}
    />,
  );
  expect(screen.getByText("イベントの時刻:")).toBeDefined();
  expect(screen.getByText("エッジの期間の始まり:")).toBeDefined();
  expect(screen.getAllByText("時刻なし").length).toBe(1);
});

test("種類と文字列でフィルタを適用し、一致する行が無いときは件数 0 を出す", () => {
  render(
    <Bookmarks
      bookmarks={bookmarks}
      onOpen={vi.fn()}
      onChangeBookmarks={vi.fn()}
    />,
  );
  fireEvent.change(screen.getByRole("combobox", { name: "種類" }), {
    target: { value: "timeline" },
  });
  expect(names()).toEqual([early.recordRef.sourceFileName]);

  fireEvent.change(screen.getByRole("combobox", { name: "種類" }), {
    target: { value: "" },
  });
  const text = screen.getByRole("searchbox", { name: "文字列" });
  fireEvent.change(text, { target: { value: "WHOAMI" } });
  expect(names()).toEqual([edgeName]);

  fireEvent.change(text, { target: { value: "無い文字列" } });
  expect(screen.getByRole("status").textContent).toBe("件数: 0");
  expect(screen.queryByRole("table")).toBeNull();
});

test("名前を押すと開き、開けなかった行は対象が無いことを出す。外すと一覧から消し、localStorage に書かない", () => {
  const onOpen = vi.fn(() => false);
  const onChangeBookmarks = vi.fn();
  render(
    <Bookmarks
      bookmarks={bookmarks}
      onOpen={onOpen}
      onChangeBookmarks={onChangeBookmarks}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "HOST-B" }));
  expect(onOpen).toHaveBeenCalledWith(bookmarks[0]?.target);
  expect(screen.getByText("解析結果になし")).toBeDefined();

  onOpen.mockReturnValue(true);
  fireEvent.click(screen.getByRole("button", { name: "HOST-B" }));
  expect(screen.queryByText("解析結果になし")).toBeNull();

  fireEvent.click(
    screen.getByRole("button", { name: "HOST-B をブックマークから削除" }),
  );
  expect(onChangeBookmarks).toHaveBeenLastCalledWith(bookmarks.slice(1));
  expect(localStorage.length).toBe(0);
});

test("開けなかった行を外して同じ対象を付け直すと、開く前には対象が無いことを出さない", () => {
  const node = bookmarks.slice(0, 1);
  const view = (list: Bookmark[]) => (
    <Bookmarks
      bookmarks={list}
      onOpen={() => false}
      onChangeBookmarks={(next) => rerender(view(next))}
    />
  );
  const { rerender } = render(view(node));
  fireEvent.click(screen.getByRole("button", { name: "HOST-B" }));
  expect(screen.getByText("解析結果になし")).toBeDefined();

  fireEvent.click(
    screen.getByRole("button", { name: "HOST-B をブックマークから削除" }),
  );
  rerender(view(node));
  expect(screen.queryByText("解析結果になし")).toBeNull();
});

test("外す button の名前は、表示名の制御文字と書式文字を可視の符号にする", () => {
  render(
    <Bookmarks
      onOpen={vi.fn()}
      bookmarks={[
        placeBookmarkOf({
          kind: "node",
          node: { id: "n:1", label: "HOST‮gnp.exe" },
        }),
      ]}
      onChangeBookmarks={vi.fn()}
    />,
  );
  expect(
    screen.getByRole("button", {
      name: "HOSTU+202Egnp.exe をブックマークから削除",
    }),
  ).toBeTruthy();
});
