// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import type { Timestamp } from "@/shared/contracts/common";
import { DisplayOffsetContext } from "@/shared/ui/DisplayOffset";
import { timelineResponseJson } from "@/testdata/timeline/timelineResponse";
import { SourceCoverageList } from "./SourceCoverageList";

afterEach(cleanup);

const tableName = "収集元ごとの記録期間";

test("起点からの経過で書かれた記録期間の端は、UTC の文字列を出して原文の数を添える", () => {
  const epoch: Timestamp = {
    rawText: "131000000000000000",
    normalized: "2016-02-19T02:13:20Z",
    normalizedForm: "rfc3339_absolute",
    precision: "microsecond",
    offsetState: "epoch",
    clock: "terminal_local",
    meaning: "event",
    valueState: "present",
  };
  render(
    <SourceCoverageList
      coverages={[
        {
          sourceId: "s1",
          sourceFileName: "run.pf",
          observedRangeFirst: epoch,
          observedRangeLast: epoch,
          state: "range_not_requested",
          matchedRecordCount: 1,
          matchedRowCount: 3,
        },
      ]}
    />,
  );

  const table = screen.getByRole("table", { name: tableName });
  // UTC の文字列を出し、原文の数は title に持つ。
  const [first] = screen.getAllByText("2016-02-19T02:13:20Z");
  expect(first?.getAttribute("title")).toBe("原文: 131000000000000000");
  // 1 件のレコードがイベントの時刻ごとに 3 行を持つ。
  const cells = table.querySelectorAll("tbody td");
  expect(cells[2]?.textContent).toBe("1");
  expect(cells[3]?.textContent).toBe("3");
});

test("タイムゾーンを持たない記録期間の端に、与えたタイムゾーンの出どころと表示のタイムゾーンの期間を添える", () => {
  const local = (rawText: string, normalized: string): Timestamp => ({
    rawText,
    normalized,
    normalizedForm: "local_without_offset",
    precision: "second",
    offsetState: "undetermined",
    clock: "terminal_local",
    meaning: "event",
    valueState: "present",
    interpretation: { offset: "+00:00" },
  });
  render(
    <DisplayOffsetContext.Provider value="+09:00">
      <SourceCoverageList
        coverages={[
          {
            sourceId: "s1",
            sourceFileName: "proxy.log",
            observedRangeFirst: local(
              "2031/01/02 03:04:05",
              "2031-01-02T03:04:05",
            ),
            observedRangeLast: local(
              "2031/01/02 04:05:06",
              "2031-01-02T04:05:06",
            ),
            state: "range_not_requested",
            matchedRecordCount: 2,
            matchedRowCount: 2,
          },
        ]}
      />
    </DisplayOffsetContext.Provider>,
  );

  const cells = screen
    .getByRole("table", { name: tableName })
    .querySelectorAll("tbody td");
  expect(cells[0]?.textContent).toBe(
    "2031/01/02 03:04:05 – 2031/01/02 04:05:06" +
      "起動時の指定: UTC+00:00" +
      "UTC+09:00: 2031-01-02T12:04:05+09:00 – 2031-01-02T13:05:06+09:00",
  );
});

test("表示のタイムゾーンを選ぶと、収集元の記録期間にそのタイムゾーンの期間を添える", () => {
  const [first, second] = timelineResponseJson().entries;
  render(
    <DisplayOffsetContext.Provider value="+00:00">
      <SourceCoverageList
        coverages={[
          {
            sourceId: "s1",
            sourceFileName: "host-a.log",
            // 応答の先頭 2 行は時刻を持つ。配列の添字の読み取りが undefined を含む型になる。
            observedRangeFirst: first?.eventTime as Timestamp,
            observedRangeLast: second?.eventTime as Timestamp,
            state: "range_not_requested",
            matchedRecordCount: 2,
            matchedRowCount: 2,
          },
        ]}
      />
    </DisplayOffsetContext.Provider>,
  );

  expect(screen.getByRole("table", { name: tableName }).textContent).toContain(
    "UTC+00:00: 2031-10-08T01:20:35.100+00:00 – ",
  );
});
