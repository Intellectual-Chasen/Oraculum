// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { decodeTimeHistogramResponse } from "@/shared/contracts/timeHistogram";
import { DisplayOffsetContext } from "@/shared/ui/DisplayOffset";
import { jsonResponse } from "@/testdata/http";
import { timelineResponseJson } from "@/testdata/timeline/timelineResponse";
import {
  columnRangeFilter,
  histogramColumns,
  histogramOf,
  TimeHistogram,
} from "./TimeHistogram";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const terminal = timelineResponseJson().entries[0]?.terminal;

/** 端末 n 台の行を持つ応答。行 i は列 i に n - i 件を持つ。最後の行は端末を持たない。 */
function responseJson(
  terminals: number,
  label = (index: number) => `HOST-${index}`,
) {
  const rows = Array.from({ length: terminals }, (_, index) => {
    const counts = new Array<number>(histogramColumns).fill(0);
    counts[index] = terminals - index;
    return {
      ...(index === terminals - 1
        ? {}
        : {
            terminal: {
              ...terminal,
              id: `n:terminal:${index}`,
              label: { rawText: label(index), valueState: "present" },
            },
          }),
      counts,
    };
  });
  return {
    start: "2031-10-08T00:00:00.000Z",
    stepMs: 2000,
    rows,
    localTimeRecordCount: 3,
    undatedRecordCount: 2,
    spanningRecordCount: 1,
  };
}

test("上位 8 行を出し、残りの端末を「その他」の行へ足す。行が無い応答は描かない", () => {
  const histogram = histogramOf(
    decodeTimeHistogramResponse(responseJson(10), "$"),
  );
  if (histogram === undefined) throw new Error("no histogram");
  expect(histogram.rows.map((row) => row.label)).toEqual([
    "HOST-0",
    "HOST-1",
    "HOST-2",
    "HOST-3",
    "HOST-4",
    "HOST-5",
    "HOST-6",
    "HOST-7",
    "その他",
  ]);
  // 9 行目 (HOST-8) の列 8 の 2 件と、端末の無い 10 行目の列 9 の 1 件。
  expect(histogram.rows[8]?.counts.slice(8, 10)).toEqual([2, 1]);
  expect(histogram.maxCount).toBe(10);
  expect(histogram.columnCount).toBe(histogramColumns);
  // 行の key は端末のノードの識別子で、表示名が同じ端末も別の行になる。
  expect(new Set(histogram.rows.map((row) => row.key)).size).toBe(
    histogram.rows.length,
  );

  expect(
    histogramOf({
      stepMs: 0,
      rows: [],
      localTimeRecordCount: 5,
      undatedRecordCount: 0,
      spanningRecordCount: 0,
    }),
  ).toBeUndefined();

  expect(columnRangeFilter(histogram, 3, 1)).toEqual({
    from: { text: "2031-10-08T00:00:02.000Z", precision: "millisecond" },
    to: { text: "2031-10-08T00:00:07.999Z", precision: "millisecond" },
    unit: "millisecond",
  });
});

test("同じ表示名の端末の行は、端末を記録した収集元の表示名を足して見分けられる", () => {
  const json = responseJson(3, () => "host01.example.test");
  json.rows.forEach((row, index) => {
    if (row.terminal === undefined) return;
    Object.assign(row.terminal, {
      keyForm: "recording_source_content_sha256_hostname",
      identity: [
        { value: String(index).repeat(64) },
        { value: "host01.example.test" },
      ],
    });
  });
  const histogram = histogramOf(
    decodeTimeHistogramResponse(json, "$"),
    new Map([
      ["0".repeat(64), "a.log"],
      ["1".repeat(64), "b.log"],
    ]),
  );
  expect(histogram?.rows.map((row) => row.label)).toEqual([
    "host01.example.test、収集元: a.log",
    "host01.example.test、収集元: b.log",
    "端末なし",
  ]);
});

test("端末の表示名の制御文字と書式文字を、可視の符号にして描く", () => {
  const histogram = histogramOf(
    decodeTimeHistogramResponse(
      responseJson(2, () => "HOST‮gnp.exe"),
      "$",
    ),
  );
  expect(histogram?.rows[0]?.label).toBe("HOSTU+202Egnp.exe");
});

function stubHistogram(json: unknown) {
  const mock = vi.fn(async (_input: string) => jsonResponse(200, json));
  vi.stubGlobal("fetch", mock);
  return mock;
}

// 同じ要求の組を渡し続け、描き直しで取り直さないようにする。
const request = { matchConditions: { conditions: [] }, sources: ["s1"] };

test("図の下に時刻の目盛りを、画面全体で選んだ表示のタイムゾーンで出す", async () => {
  stubHistogram(responseJson(2));
  const view = (offset: string) => (
    <DisplayOffsetContext.Provider value={offset}>
      <TimeHistogram
        request={request}
        version={0}
        timeFilter={undefined}
        onApplyTimeFilter={vi.fn()}
      />
    </DisplayOffsetContext.Provider>
  );
  const { rerender } = render(view(""));
  const axis = await screen.findByRole("list", { name: "時刻の目盛り" });
  const ticks = () =>
    within(axis)
      .getAllByRole("listitem")
      .map((item) => item.textContent);
  // 1 列は 2 秒であり、10 列ごとに目盛りを置く。
  expect(ticks().slice(0, 2)).toEqual(["10-08 00:00:00", "10-08 00:00:20"]);
  expect(screen.getByText("タイムゾーン: UTC")).toBeTruthy();

  rerender(view("+09:00"));
  expect(ticks().slice(0, 2)).toEqual(["10-08 09:00:00", "10-08 09:00:20"]);
  expect(screen.getByText("タイムゾーン: UTC+09:00")).toBeTruthy();
});

test("1 列が 1 秒より短いときは、目盛りにミリ秒まで書く", async () => {
  stubHistogram({ ...responseJson(2), stepMs: 73 });
  render(
    <TimeHistogram
      request={request}
      version={0}
      timeFilter={undefined}
      onApplyTimeFilter={vi.fn()}
    />,
  );
  const axis = await screen.findByRole("list", { name: "時刻の目盛り" });
  expect(
    within(axis)
      .getAllByRole("listitem")
      .slice(0, 2)
      .map((item) => item.textContent),
  ).toEqual(["10-08 00:00:00.000", "10-08 00:00:00.730"]);
});

test("列を横にドラッグすると、その範囲の期間のフィルタを適用し、適用中の列に印を付け、外す操作を出す", async () => {
  const mock = stubHistogram(responseJson(2));
  const onApply = vi.fn();
  const { container, rerender } = render(
    <TimeHistogram
      request={request}
      version={0}
      timeFilter={undefined}
      onApplyTimeFilter={onApply}
    />,
  );
  await screen.findByRole("img", { name: "時間と端末ごとの件数" });
  const params = new URL(mock.mock.calls[0]?.[0] ?? "", "http://localhost")
    .searchParams;
  expect(params.get("columns")).toBe(String(histogramColumns));
  expect(params.getAll("source")).toEqual(["s1"]);
  const column = (index: number) =>
    container.querySelector(`rect[data-column="${index}"]`) as Element;

  fireEvent.pointerDown(column(0));
  fireEvent.pointerEnter(column(1));
  fireEvent.pointerUp(column(1));

  expect(onApply).toHaveBeenCalledWith({
    from: { text: "2031-10-08T00:00:00.000Z", precision: "millisecond" },
    to: { text: "2031-10-08T00:00:03.999Z", precision: "millisecond" },
    unit: "millisecond",
  });

  rerender(
    <TimeHistogram
      request={request}
      version={0}
      timeFilter={onApply.mock.calls[0]?.[0]}
      onApplyTimeFilter={onApply}
    />,
  );
  expect(column(1).getAttribute("class")).toBe("histogram-selected");
  expect(column(2).getAttribute("class")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "期間のフィルタを解除" }));
  expect(onApply).toHaveBeenLastCalledWith(undefined);
});

/** 「名前: 値」の組の文字列の一覧。 */
function pairTexts(): string[] {
  return screen
    .queryAllByRole("listitem")
    .map((item) => item.textContent ?? "");
}

test("棒に入れなかったレコードを、精度・タイムゾーン不明・時刻なしの理由ごとの件数で知らせる", async () => {
  stubHistogram(responseJson(2));
  render(
    <TimeHistogram
      request={request}
      version={0}
      timeFilter={undefined}
      onApplyTimeFilter={() => {}}
    />,
  );
  await screen.findByRole("img", { name: "時間と端末ごとの件数" });
  const texts = pairTexts();
  const start = texts.indexOf("棒の外: 6");
  expect(texts.slice(start, start + 4)).toEqual([
    "棒の外: 6",
    "精度が棒の幅より粗い: 1",
    "タイムゾーン不明: 3",
    "時刻なし: 2",
  ]);
});

test("どの棒にも入らないときは、図を描かずに理由を知らせる", async () => {
  stubHistogram({
    start: "2031-10-08T00:00:00.000Z",
    stepMs: 10,
    rows: [],
    localTimeRecordCount: 0,
    undatedRecordCount: 0,
    spanningRecordCount: 4,
  });
  render(
    <TimeHistogram
      request={request}
      version={0}
      timeFilter={undefined}
      onApplyTimeFilter={() => {}}
    />,
  );
  expect(await screen.findByText("棒に入るレコードなし")).toBeTruthy();
  expect(pairTexts()).toEqual(
    expect.arrayContaining(["棒の外: 4", "精度が棒の幅より粗い: 4"]),
  );
  expect(
    screen.queryByRole("img", { name: "時間と端末ごとの件数" }),
  ).toBeNull();
});

test("棒の範囲をキーボードで選んで期間のフィルタを適用し、件数を表で読める", async () => {
  stubHistogram(responseJson(2));
  const onApply = vi.fn();
  render(
    <TimeHistogram
      request={request}
      version={0}
      timeFilter={undefined}
      onApplyTimeFilter={onApply}
    />,
  );
  await screen.findByRole("img", { name: "時間と端末ごとの件数" });
  fireEvent.change(screen.getByLabelText("始まりの棒"), {
    target: { value: "2" },
  });
  fireEvent.change(screen.getByLabelText("終わりの棒"), {
    target: { value: "1" },
  });
  fireEvent.click(screen.getByRole("button", { name: "期間のフィルタを適用" }));
  expect(onApply).toHaveBeenCalledWith({
    from: { text: "2031-10-08T00:00:02.000Z", precision: "millisecond" },
    to: { text: "2031-10-08T00:00:05.999Z", precision: "millisecond" },
    unit: "millisecond",
  });

  // 表は件数のある棒 (0 本目と 1 本目) だけを行に並べ、行ごとの件数と合計を出す。
  const table = screen.getByRole("table", {
    name: "棒ごとの端末別の件数",
  });
  const rows = within(table).getAllByRole("row").slice(1);
  expect(
    rows.map((row) => within(row).getByRole("rowheader").textContent),
  ).toEqual([
    "2031-10-08T00:00:00.000Z – 2031-10-08T00:00:02.000Z",
    "2031-10-08T00:00:02.000Z – 2031-10-08T00:00:04.000Z",
  ]);
  expect(
    within(rows[0] as HTMLElement)
      .getAllByRole("cell")
      .map((cell) => cell.textContent),
  ).toEqual(["2", "0", "2"]);
});
