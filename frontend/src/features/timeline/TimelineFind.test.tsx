// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { decodeTimelineResponse } from "@/shared/contracts/timeline";
import { jsonResponse } from "@/testdata/http";
import {
  evenlySpacedEntrySpecs,
  generatedTimelineResponseJson,
  timelineResponseJson,
} from "@/testdata/timeline/timelineResponse";
import { seekIndexOf, Timeline } from "./Timeline";
import { nextMatch } from "./TimelineFind";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

test("下方は今の行より後ろの最初の一致を、上方は前の最後の一致を返し、無ければ undefined を返す", () => {
  const matches = [2, 5, 9];
  expect(nextMatch(matches, undefined, "down")).toBe(2);
  expect(nextMatch(matches, 5, "down")).toBe(9);
  expect(nextMatch(matches, 9, "down")).toBeUndefined();
  expect(nextMatch(matches, undefined, "up")).toBe(9);
  expect(nextMatch(matches, 5, "up")).toBe(2);
  expect(nextMatch(matches, 2, "up")).toBeUndefined();
});

/** 文字列を含む要求には、2 行ともに一致したと返す。 */
function stubFind() {
  const mock = vi.fn(async (input: string, _init?: RequestInit) => {
    const find = new URL(input, "http://localhost").searchParams.get("find");
    return jsonResponse(200, {
      ...timelineResponseJson(),
      ...(find === null ? {} : { findMatches: [0, 1] }),
    });
  });
  vi.stubGlobal("fetch", mock);
  return mock;
}

function lastParams(mock: ReturnType<typeof stubFind>) {
  return new URL(mock.mock.calls.at(-1)?.[0] ?? "", "http://localhost")
    .searchParams;
}

function renderTimeline(onSelectRecord = vi.fn()) {
  render(
    <Timeline
      matchConditions={{ conditions: [{ conditionKey: "destination_ip" }] }}
      timeFilter={undefined}
      eventCategory={undefined}
      eventAction={undefined}
      caseId={undefined}
      terminal={undefined}
      onSelectRecord={onSelectRecord}
      dataVersion={0}
    />,
  );
  return onSelectRecord;
}

const recordTableName = "時刻順のレコード";

function focusedRowText() {
  const table = screen.getByRole("table", { name: recordTableName });
  return within(table)
    .getAllByRole("row")
    .find((row) => row.getAttribute("aria-current") === "true")?.textContent;
}

test("文字列を探すと最初の一致の行を開いて強調し、次の一致へ進み、尽きたらそれを知らせる", async () => {
  const mock = stubFind();
  const onSelectRecord = renderTimeline();
  await screen.findByRole("table", { name: recordTableName });

  fireEvent.change(screen.getByLabelText("原文から探す文字列"), {
    target: { value: "WhoAmI" },
  });
  fireEvent.click(screen.getByRole("button", { name: "検索" }));

  await waitFor(() => expect(onSelectRecord).toHaveBeenCalledTimes(1));
  expect(lastParams(mock).get("find")).toBe("WhoAmI");
  expect(lastParams(mock).get("findCaseSensitive")).toBe("false");
  expect(focusedRowText()).toContain("2031/10/08 10:20:35.100");
  expect(screen.getByRole("status").textContent).toBe("一致した行: 2");
  // 探す対象は「?」の説明で読める。
  fireEvent.click(
    screen.getByRole("button", { name: "原文から探す文字列 の説明" }),
  );
  expect(screen.getByRole("tooltip").textContent).toContain(
    "フィールドの名前と復号した値",
  );

  fireEvent.click(screen.getByRole("button", { name: "次の一致" }));
  await waitFor(() =>
    expect(focusedRowText()).toContain("2031/10/08 10:20:36.200"),
  );
  expect(onSelectRecord).toHaveBeenCalledTimes(2);

  fireEvent.click(screen.getByRole("button", { name: "次の一致" }));
  await waitFor(() =>
    expect(screen.getByRole("status").textContent).toBe(
      "一致した行: 2 下に一致なし",
    ),
  );
  expect(onSelectRecord).toHaveBeenCalledTimes(2);
});

test("大文字と小文字の区別を要求に載せ、上へは末尾の一致から、すべては一致の一覧を出す", async () => {
  const mock = stubFind();
  const onSelectRecord = renderTimeline();
  await screen.findByRole("table", { name: recordTableName });

  fireEvent.change(screen.getByLabelText("原文から探す文字列"), {
    target: { value: "net" },
  });
  fireEvent.click(
    screen.getByRole("checkbox", { name: "大文字と小文字を区別する" }),
  );
  fireEvent.change(screen.getByRole("combobox", { name: "探す向き" }), {
    target: { value: "up" },
  });
  fireEvent.click(screen.getByRole("button", { name: "検索" }));
  await waitFor(() =>
    expect(focusedRowText()).toContain("2031/10/08 10:20:36.200"),
  );
  expect(lastParams(mock).get("findCaseSensitive")).toBe("true");

  fireEvent.change(screen.getByRole("combobox", { name: "探す向き" }), {
    target: { value: "all" },
  });
  const list = screen.getByRole("table", { name: "文字列を含む行" });
  const buttons = within(list).getAllByRole("button");
  expect(buttons).toHaveLength(2);
  fireEvent.click(buttons[0] as HTMLElement);
  await waitFor(() =>
    expect(focusedRowText()).toContain("2031/10/08 10:20:35.100"),
  );
  expect(onSelectRecord).toHaveBeenCalledTimes(2);
});

test("移動先の時刻を渡すと、一覧を読んだ後にその時刻以後の最初の行へ移り、レコードは開かずに知らせる", async () => {
  stubFind();
  const onSelectRecord = vi.fn();
  const onSeekDone = vi.fn();
  render(
    <Timeline
      matchConditions={{ conditions: [{ conditionKey: "destination_ip" }] }}
      timeFilter={undefined}
      eventCategory={undefined}
      eventAction={undefined}
      caseId={undefined}
      terminal={undefined}
      onSelectRecord={onSelectRecord}
      dataVersion={0}
      // 2 行目の時刻は 2031-10-08T10:20:36.200+09:00 である。
      seek={{ utc: "2031-10-08T01:20:36Z" }}
      onSeekDone={onSeekDone}
    />,
  );

  await waitFor(() =>
    expect(focusedRowText()).toContain("2031/10/08 10:20:36.200"),
  );
  expect(onSeekDone).toHaveBeenCalledTimes(1);
  expect(onSelectRecord).not.toHaveBeenCalled();
});

test.each([
  ["一覧の範囲より後の", "2031-10-09T00:00:00Z", "2031/10/08 10:20:36.200"],
  ["行の時刻と一致しない", "2031-10-08T01:20:36Z", "2031/10/08 10:20:36.200"],
])(
  "移動先の時刻が%sときは、一致なしと移った先の行の時刻を出す",
  async (_name, utc, rowText) => {
    stubFind();
    render(
      <Timeline
        matchConditions={{ conditions: [{ conditionKey: "destination_ip" }] }}
        timeFilter={undefined}
        eventCategory={undefined}
        eventAction={undefined}
        caseId={undefined}
        terminal={undefined}
        onSelectRecord={vi.fn()}
        dataVersion={0}
        seek={{ utc }}
        onSeekDone={vi.fn()}
      />,
    );

    await waitFor(() => expect(focusedRowText()).toContain(rowText));
    expect(screen.getByRole("status").textContent).toBe(
      "一致なし移動先: 2031-10-08T01:20:36.200Z",
    );
  },
);

test("移動先の時刻が行の時刻と一致するときは、一致なしを出さない", async () => {
  stubFind();
  render(
    <Timeline
      matchConditions={{ conditions: [{ conditionKey: "destination_ip" }] }}
      timeFilter={undefined}
      eventCategory={undefined}
      eventAction={undefined}
      caseId={undefined}
      terminal={undefined}
      onSelectRecord={vi.fn()}
      dataVersion={0}
      seek={{ utc: "2031-10-08T01:20:36.200Z" }}
      onSeekDone={vi.fn()}
    />,
  );

  await waitFor(() =>
    expect(focusedRowText()).toContain("2031/10/08 10:20:36.200"),
  );
  expect(screen.queryByText("一致なし")).toBeNull();
});

test("移動先の時刻が最後の行より後なら、時刻を持つ最後の行を返し、時刻を持つ行が無ければ undefined を返す", () => {
  const entries = decodeTimelineResponse(
    generatedTimelineResponseJson(
      evenlySpacedEntrySpecs(3, "2031-10-08T00:00:00Z", 1000),
    ),
    "fixture",
  ).entries;
  expect(seekIndexOf(entries, "2031-10-08T00:00:00.625Z")).toBe(1);
  expect(seekIndexOf(entries, "2031-10-09T00:00:00Z")).toBe(2);
  expect(seekIndexOf([], "2031-10-08T00:00:00Z")).toBeUndefined();
});
