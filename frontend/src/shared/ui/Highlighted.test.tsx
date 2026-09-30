// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, test } from "vitest";
import type { Timestamp } from "../contracts/common";
import {
  type GraphTimeFilter,
  searchHighlightOf,
} from "../lib/searchHighlight";
import { noSearchTerms, type SearchTerms } from "../lib/searchTerms";
import { Highlighted, SearchHighlightContext } from "./Highlighted";
import { TimestampText } from "./TimestampText";

afterEach(cleanup);

function withHighlight(
  terms: SearchTerms,
  timeFilter: GraphTimeFilter | undefined,
  children: ReactNode,
) {
  return render(
    <SearchHighlightContext.Provider
      value={searchHighlightOf(terms, timeFilter)}
    >
      {children}
    </SearchHighlightContext.Provider>,
  );
}

const period: GraphTimeFilter = {
  from: { text: "2031-10-08T10:20:30+09:00" },
  to: { text: "2031-10-08T10:20:35+09:00" },
  unit: "second",
};

const inside: Timestamp = {
  rawText: "2031-10-08T01:20:35.900Z",
  normalized: "2031-10-08T01:20:35.900Z",
  normalizedForm: "rfc3339_absolute",
  precision: "millisecond",
  offsetState: "in_value",
  clock: "terminal_local",
  meaning: "event",
  valueState: "present",
};

test("一致した部分を mark で包み、制御文字は符号のまま描く", () => {
  const { container } = withHighlight(
    { ...noSearchTerms, contains: ["run"] },
    undefined,
    <Highlighted text={"a\tRUN"} />,
  );
  expect(
    [...container.querySelectorAll("mark")].map((mark) => mark.textContent),
  ).toEqual(["RUN"]);
  expect(container.textContent).toBe("aU+0009RUN");
});

test("期間内の時刻を mark で包み、判定不能の理由に合う印を付ける", () => {
  const { container } = withHighlight(
    noSearchTerms,
    period,
    <>
      <TimestampText timestamp={inside} />
      <TimestampText
        timestamp={{
          ...inside,
          rawText: "2031/10/08 10:20:32",
          normalized: "2031-10-08T10:20:32",
          normalizedForm: "local_without_offset",
          offsetState: "item_absent",
        }}
      />
      <TimestampText
        timestamp={{
          ...inside,
          rawText: "2031-10-08T10:20",
          normalized: "2031-10-08T10:20",
          normalizedForm: "partial_date_time",
          precision: "minute",
          offsetState: "item_absent",
        }}
      />
      <TimestampText
        timestamp={{
          ...inside,
          rawText: "2031-10-08T01:20:36Z",
          normalized: "2031-10-08T01:20:36Z",
        }}
      />
    </>,
  );
  expect(
    [...container.querySelectorAll("mark")].map((mark) => mark.textContent),
  ).toEqual(["2031-10-08T01:20:35.900Z"]);
  const unjudged = [...container.querySelectorAll(".search-unjudged")];
  expect(unjudged.map((element) => element.textContent)).toEqual([
    "2031/10/08 10:20:32",
    "2031-10-08T10:20",
  ]);
  expect(unjudged.map((element) => element.getAttribute("title"))).toEqual([
    "期間の判定: タイムゾーン不明",
    "期間の判定: 時刻の精度不足",
  ]);
});
