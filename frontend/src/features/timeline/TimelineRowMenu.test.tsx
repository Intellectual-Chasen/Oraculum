// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { NodeRef, SourceRef } from "@/shared/api/graph";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import type { RecordLocator } from "@/shared/contracts/common";
import { restoreClipboard, stubClipboard } from "@/testdata/clipboard";
import { jsonResponse } from "@/testdata/http";
import { hostALogSourceId } from "@/testdata/sources/sourcesResponse";
import {
  timelineResponseJson,
  timelineTerminalNodeId,
} from "@/testdata/timeline/timelineResponse";
import { Timeline } from "./Timeline";
import type { TimelineRowBookmark } from "./timelineRowMenu";

const matchConditions: MatchConditionSelection = {
  conditions: [{ conditionKey: "destination_ip" }],
};

/** 応答の先頭の行の位置の文字列。 */
const firstRowLocation = "収集元: host-a.log、ID: 2204、行: 1022";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  restoreClipboard();
});

function renderTimeline(
  props: {
    terminal?: string;
    sources?: readonly string[];
    onNarrowToTerminal?: (terminal: NodeRef) => void;
    onNarrowToSource?: (source: SourceRef) => void;
    rowBookmark?: TimelineRowBookmark;
  } = {},
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse(200, timelineResponseJson())),
  );
  const onSelectRecord = vi.fn<(recordRef: RecordLocator) => void>();
  const onPreviewRecord = vi.fn<(recordRef: RecordLocator) => void>();
  render(
    <Timeline
      matchConditions={matchConditions}
      timeFilter={undefined}
      eventCategory={undefined}
      eventAction={undefined}
      caseId={undefined}
      terminal={props.terminal}
      sources={props.sources}
      onSelectRecord={onSelectRecord}
      onPreviewRecord={onPreviewRecord}
      onNarrowToTerminal={props.onNarrowToTerminal}
      onNarrowToSource={props.onNarrowToSource}
      rowBookmark={props.rowBookmark}
      dataVersion={0}
    />,
  );
  return { onSelectRecord, onPreviewRecord };
}

async function openFirstRowMenu() {
  fireEvent.click(
    await screen.findByRole("button", { name: `${firstRowLocation} の操作` }),
  );
  return screen.getByRole("menu", { name: `${firstRowLocation} の操作` });
}

function itemNames(menu: HTMLElement) {
  return within(menu)
    .getAllByRole("menuitem")
    .map((item) => item.textContent);
}

function choose(name: string) {
  fireEvent.click(screen.getByRole("menuitem", { name }));
}

const firstRecord = expect.objectContaining({
  sourceId: hostALogSourceId,
  sequenceNumber: 2204,
});

test("行のメニューは、Record に表示する操作と行の端末・収集元でフィルタする操作を、行の値で呼ぶ", async () => {
  const onNarrowToTerminal = vi.fn();
  const onNarrowToSource = vi.fn();
  const { onSelectRecord, onPreviewRecord } = renderTimeline({
    onNarrowToTerminal,
    onNarrowToSource,
  });

  expect(itemNames(await openFirstRowMenu())).toEqual([
    "Record に表示",
    "Record を前面に出さずに表示",
    "この端末でフィルタ",
    "この収集元でフィルタ",
    "位置をコピー",
  ]);
  choose("Record に表示");
  expect(onSelectRecord).toHaveBeenCalledWith(firstRecord);
  expect(onPreviewRecord).not.toHaveBeenCalled();

  await openFirstRowMenu();
  choose("Record を前面に出さずに表示");
  expect(onPreviewRecord).toHaveBeenCalledWith(firstRecord);

  await openFirstRowMenu();
  choose("この端末でフィルタ");
  expect(onNarrowToTerminal).toHaveBeenCalledWith({
    id: timelineTerminalNodeId,
    label: "HOST-C",
    kind: "terminal",
  });

  await openFirstRowMenu();
  choose("この収集元でフィルタ");
  expect(onNarrowToSource).toHaveBeenCalledWith({
    id: hostALogSourceId,
    label: "host-a.log",
  });
});

test("今と同じ端末だけ・同じ収集元だけのフィルタを適用しているときは、その項目を使えなくする", async () => {
  const onNarrowToTerminal = vi.fn();
  const onNarrowToSource = vi.fn();
  renderTimeline({
    terminal: timelineTerminalNodeId,
    sources: [hostALogSourceId],
    onNarrowToTerminal,
    onNarrowToSource,
  });
  await openFirstRowMenu();

  for (const name of ["この端末でフィルタ", "この収集元でフィルタ"]) {
    expect(screen.getByRole("menuitem", { name })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    choose(name);
  }
  expect(onNarrowToTerminal).not.toHaveBeenCalled();
  expect(onNarrowToSource).not.toHaveBeenCalled();
});

test("ほかの収集元も選んでいるときは、行の収集元でフィルタする項目を使える", async () => {
  const onNarrowToSource = vi.fn();
  renderTimeline({
    sources: [hostALogSourceId, "source:access-log"],
    onNarrowToSource,
  });
  await openFirstRowMenu();
  choose("この収集元でフィルタ");
  expect(onNarrowToSource).toHaveBeenCalledWith({
    id: hostALogSourceId,
    label: "host-a.log",
  });
});

test("フィルタを適用する操作を渡さないときは項目を出さず、位置をコピーは接頭辞の無い位置の値をコピーする", async () => {
  const clipboard = stubClipboard();
  renderTimeline();
  expect(itemNames(await openFirstRowMenu())).toEqual([
    "Record に表示",
    "Record を前面に出さずに表示",
    "位置をコピー",
  ]);
  choose("位置をコピー");

  expect(
    await screen.findByText("コピー済み: レコードの位置"),
  ).toBeInTheDocument();
  expect(clipboard.writeText).toHaveBeenCalledWith("2204");
});

test("行の右クリックと Shift+F10 でメニューを開き、Escape で開いた元へ focus を戻す", async () => {
  renderTimeline();
  await openFirstRowMenu();
  fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
  const [openButton] = screen.getAllByRole("button", {
    name: "Record に表示",
  });
  const row = openButton?.closest("tr");
  if (openButton === undefined || row === null || row === undefined) {
    throw new Error("the timeline row is missing");
  }

  expect(fireEvent.contextMenu(row)).toBe(false);
  fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });

  openButton.focus();
  fireEvent.keyDown(openButton, { key: "F10", shiftKey: true });
  expect(screen.getByRole("menuitem", { name: "Record に表示" })).toHaveFocus();
  fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
  expect(openButton).toHaveFocus();
});

test("行のメニューは、開いた時点で付けてあるかを読んで、ブックマークを付ける項目か外す項目を出す", async () => {
  let marked = false;
  const onToggle = vi.fn(() => {
    marked = !marked;
  });
  renderTimeline({ rowBookmark: { isMarked: () => marked, onToggle } });

  await openFirstRowMenu();
  choose("ブックマークに追加");
  expect(onToggle).toHaveBeenCalledWith(
    expect.objectContaining({
      recordRef: firstRecord,
      eventTime: expect.objectContaining({
        rawText: "2031/10/08 10:20:35.100",
      }),
    }),
  );

  await openFirstRowMenu();
  choose("ブックマークから削除");
  expect(onToggle).toHaveBeenCalledTimes(2);
});

test("ブックマークを渡さないときは、行のメニューにブックマークの項目を出さない", async () => {
  renderTimeline();
  const menu = await openFirstRowMenu();
  expect(itemNames(menu)).not.toContain("ブックマークに追加");
});
