// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import {
  decodeValueCount,
  type EventIntervals,
  intervalBoundsMilliseconds,
  type ValueCount,
} from "@/shared/contracts/graph";
import { ValueCountList } from "./ValueCountList";

afterEach(cleanup);

// jsdom は showModal を持たないため、属性 open を付けるだけの代わりを置き、呼び出しを数える。
const showModal = vi.fn(function (this: HTMLDialogElement) {
  this.setAttribute("open", "");
});
beforeEach(() => {
  showModal.mockClear();
  HTMLDialogElement.prototype.showModal = showModal;
});

function intervals(overrides: Partial<EventIntervals> = {}): EventIntervals {
  const binCounts = intervalBoundsMilliseconds.map(() => 0);
  binCounts.push(0);
  binCounts[5] = 3;
  return {
    timedRecordCount: 4,
    minMilliseconds: 60_000,
    lowerQuartileMilliseconds: 60_000,
    medianMilliseconds: 60_000,
    upperQuartileMilliseconds: 62_000,
    maxMilliseconds: 65_000,
    binCounts,
    coarsePrecision: true,
    ...overrides,
  };
}

const counts: ValueCount[] = [
  { value: "b.example.test", recordCount: 2 },
  { value: "c.example.test", recordCount: 5, intervals: intervals() },
  { value: "a.example.test", recordCount: 2 },
];

function valueColumn(): string[] {
  const table = screen.getByRole("table", {
    name: "フィールドの値ごとのレコード数",
  });
  return within(table)
    .getAllByRole("row")
    .slice(1)
    .map((row) => row.querySelector("td")?.textContent ?? "");
}

test("件数の多い順から始め、見出しで少ない順と値の順に並べ替える。同じ件数は値の順に並ぶ", () => {
  render(<ValueCountList counts={counts} />);
  expect(valueColumn()).toEqual([
    "c.example.test",
    "a.example.test",
    "b.example.test",
  ]);
  const countHeader = screen.getByRole("columnheader", {
    name: "レコード数",
  });
  expect(countHeader.getAttribute("aria-sort")).toBe("descending");
  fireEvent.click(within(countHeader).getByRole("button"));
  expect(valueColumn()).toEqual([
    "a.example.test",
    "b.example.test",
    "c.example.test",
  ]);
  expect(countHeader.getAttribute("aria-sort")).toBe("ascending");
  fireEvent.click(screen.getByRole("button", { name: "値" }));
  expect(valueColumn()).toEqual([
    "a.example.test",
    "b.example.test",
    "c.example.test",
  ]);
  expect(
    screen.getByRole("columnheader", { name: "値" }).getAttribute("aria-sort"),
  ).toBe("ascending");
});

test("時刻の差のばらつきの小さい順に並べ、差を持たない値を後ろに置く", () => {
  // 定期的な通信は、件数が少なくても時刻の差のばらつきが小さい。
  const beacon: ValueCount = {
    value: "beacon.example.test",
    recordCount: 4,
    intervals: intervals({
      timedRecordCount: 4,
      lowerQuartileMilliseconds: 60_000,
      medianMilliseconds: 60_000,
      upperQuartileMilliseconds: 60_500,
    }),
  };
  const busy: ValueCount = {
    value: "busy.example.test",
    recordCount: 50,
    intervals: intervals({
      timedRecordCount: 50,
      minMilliseconds: 1_000,
      lowerQuartileMilliseconds: 2_000,
      medianMilliseconds: 30_000,
      upperQuartileMilliseconds: 600_000,
      maxMilliseconds: 900_000,
    }),
  };
  // 時点が 2 件だけの値は、差が 1 つで四分位の幅が 0 になる。ばらつきを比べない。
  const pair: ValueCount = {
    value: "pair.example.test",
    recordCount: 2,
    intervals: intervals({
      timedRecordCount: 2,
      minMilliseconds: 5_000,
      lowerQuartileMilliseconds: 5_000,
      medianMilliseconds: 5_000,
      upperQuartileMilliseconds: 5_000,
      maxMilliseconds: 5_000,
    }),
  };
  // 同じ時刻に集まった値は、差の中央値が 0 になる。幅の違う 2 つを並べても順が決まる。
  const burst = (value: string, recordCount: number, upper: number) => ({
    value,
    recordCount,
    intervals: intervals({
      timedRecordCount: recordCount,
      minMilliseconds: 0,
      lowerQuartileMilliseconds: 0,
      medianMilliseconds: 0,
      upperQuartileMilliseconds: upper,
      maxMilliseconds: upper,
    }),
  });
  render(
    <ValueCountList
      counts={[
        { value: "single.example.test", recordCount: 80 },
        busy,
        burst("burst-a.example.test", 6, 1_000),
        pair,
        beacon,
        burst("burst-b.example.test", 9, 0),
      ]}
    />,
  );
  expect(valueColumn()[0]).toBe("single.example.test");

  const intervalHeader = screen.getByRole("columnheader", {
    name: /時刻の間隔/,
  });
  const intervalButton = within(intervalHeader).getByRole("button");
  expect(intervalButton.getAttribute("title")).toBe(
    "間隔の揃った順: 四分位の幅 / 中央値",
  );
  fireEvent.click(intervalButton);
  // 比べられない値は、件数の多い順と値の順で後ろに並ぶ。
  expect(valueColumn()).toEqual([
    "beacon.example.test",
    "busy.example.test",
    "single.example.test",
    "burst-b.example.test",
    "burst-a.example.test",
    "pair.example.test",
  ]);
  expect(intervalHeader.getAttribute("aria-sort")).toBe("ascending");
});

test("ばらつきの等しい値は、件数の多い順と値の順で並ぶ", () => {
  const regular = (value: string, recordCount: number): ValueCount => ({
    value,
    recordCount,
    intervals: intervals({ timedRecordCount: recordCount }),
  });
  render(
    <ValueCountList
      counts={[
        regular("b.example.test", 5),
        regular("c.example.test", 9),
        regular("a.example.test", 5),
      ]}
    />,
  );
  fireEvent.click(
    within(screen.getByRole("columnheader", { name: /時刻の間隔/ })).getByRole(
      "button",
    ),
  );
  expect(valueColumn()).toEqual([
    "c.example.test",
    "a.example.test",
    "b.example.test",
  ]);
});

test("行を選ぶと、時刻の間隔の最小・四分位・最大と範囲ごとの件数と、間隔に入れなかった件数を出す", () => {
  render(<ValueCountList counts={counts} />);
  fireEvent.click(
    screen.getByRole("button", {
      name: "c.example.test の時刻の間隔の分布を表示",
    }),
  );
  const distribution = screen.getByRole("region", {
    name: "選択中の値の時刻の間隔の分布",
  });
  expect(distribution).toHaveTextContent(
    "最小: 60 秒第 1 四分位: 60 秒中央値: 60 秒第 3 四分位: 62 秒最大: 65 秒",
  );
  expect(distribution).toHaveTextContent("UTC 時刻の無いレコード: 1");
  expect(distribution).toHaveTextContent("秒単位の時刻: 含む");
  const bins = within(distribution).getByRole("table", {
    name: "間隔の範囲ごとの件数",
  });
  expect(within(bins).getAllByRole("row")).toHaveLength(2);
  expect(bins).toHaveTextContent("1 分 以上 5 分 未満3");
  // UTC 時刻を持つレコードが 2 件未満の値は、分布を選べない。
  expect(screen.getAllByText("UTC 時刻が 2 件未満")).toHaveLength(2);
});

test("秒までの時刻を含まない分布は、同じ秒の注意を出さない", () => {
  render(
    <ValueCountList
      counts={[
        {
          value: "c.example.test",
          recordCount: 4,
          intervals: intervals({ coarsePrecision: false }),
        },
      ]}
    />,
  );
  fireEvent.click(
    screen.getByRole("button", {
      name: "c.example.test の時刻の間隔の分布を表示",
    }),
  );
  expect(
    screen.getByRole("region", { name: "選択中の値の時刻の間隔の分布" }),
  ).not.toHaveTextContent("秒単位の時刻");
});

test("上端を持たない最後の範囲を、下端だけで書く", () => {
  const binCounts = intervalBoundsMilliseconds.map(() => 0);
  binCounts.push(1);
  render(
    <ValueCountList
      counts={[
        {
          value: "c.example.test",
          recordCount: 2,
          intervals: intervals({
            timedRecordCount: 2,
            minMilliseconds: 172_800_000,
            lowerQuartileMilliseconds: 172_800_000,
            medianMilliseconds: 172_800_000,
            upperQuartileMilliseconds: 172_800_000,
            maxMilliseconds: 172_800_000,
            binCounts,
          }),
        },
      ]}
    />,
  );
  fireEvent.click(
    screen.getByRole("button", {
      name: "c.example.test の時刻の間隔の分布を表示",
    }),
  );
  const bins = screen.getByRole("table", { name: "間隔の範囲ごとの件数" });
  expect(bins).toHaveTextContent("1 日 以上1");
  expect(
    screen.getByRole("region", { name: "選択中の値の時刻の間隔の分布" }),
  ).toHaveTextContent("最大: 2 日");
});

test("選んだ値が新しい応答から消えたら、分布を出さずに消えたことを出す", () => {
  const { rerender } = render(<ValueCountList counts={counts} />);
  fireEvent.click(
    screen.getByRole("button", {
      name: "c.example.test の時刻の間隔の分布を表示",
    }),
  );
  rerender(
    <ValueCountList
      counts={counts.filter((count) => count.intervals === undefined)}
    />,
  );
  expect(
    screen.queryByRole("region", { name: "選択中の値の時刻の間隔の分布" }),
  ).toBeNull();
  expect(screen.getByText("フィルタの結果に無い値")).toBeTruthy();
});

test("数える欄が変わると、選んだ値を捨てる", () => {
  const { rerender } = render(<ValueCountList key="dst" counts={counts} />);
  fireEvent.click(
    screen.getByRole("button", {
      name: "c.example.test の時刻の間隔の分布を表示",
    }),
  );
  rerender(<ValueCountList key="src" counts={counts} />);
  expect(
    screen.queryByRole("region", { name: "選択中の値の時刻の間隔の分布" }),
  ).toBeNull();
});

test("分布の整合の崩れた値を読まない", () => {
  const valid = {
    value: "c.example.test",
    recordCount: 5,
    intervals: intervals(),
  };
  expect(decodeValueCount(valid, "$").intervals?.medianMilliseconds).toBe(
    60_000,
  );
  for (const broken of [
    { ...valid, recordCount: 3 },
    { ...valid, intervals: intervals({ medianMilliseconds: 70_000 }) },
    { ...valid, intervals: intervals({ timedRecordCount: 5 }) },
    { ...valid, intervals: intervals({ binCounts: [3] }) },
  ]) {
    expect(() => decodeValueCount(broken, "$")).toThrow();
  }
});

// 狭い区画に置いても、1 行を 1 行のまま出し、表は横にずらして読む。
test("集計の表の欄を折り返さない", () => {
  render(<ValueCountList counts={counts} />);

  expect(
    screen.getByRole("table", { name: "フィールドの値ごとのレコード数" }),
  ).toHaveStyle({ whiteSpace: "nowrap" });
});

test("集計の表を画面の幅の枠で出し、枠を閉じて元の位置へ戻す", () => {
  render(<ValueCountList counts={counts} />);

  fireEvent.click(screen.getByRole("button", { name: "表を広げる" }));
  const dialog = screen.getByRole("dialog", {
    name: "フィールドの値ごとのレコード数",
  });
  expect(
    within(dialog).getByRole("table", {
      name: "フィールドの値ごとのレコード数",
    }),
  ).toBeTruthy();

  fireEvent.click(within(dialog).getByRole("button", { name: "元の幅に戻す" }));
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(
    screen.getByRole("table", { name: "フィールドの値ごとのレコード数" }),
  ).toBeTruthy();
});

test("画面の幅の枠は modal の dialog として開き、Escape で閉じる", () => {
  render(<ValueCountList counts={counts} />);

  fireEvent.click(screen.getByRole("button", { name: "表を広げる" }));
  const dialog = screen.getByRole("dialog", {
    name: "フィールドの値ごとのレコード数",
  });
  expect(dialog.tagName).toBe("DIALOG");
  expect(showModal).toHaveBeenCalledOnce();

  // Escape を押すと、ブラウザは dialog を閉じて close を送る。
  fireEvent(dialog, new Event("close"));
  expect(screen.queryByRole("dialog")).toBeNull();
});
