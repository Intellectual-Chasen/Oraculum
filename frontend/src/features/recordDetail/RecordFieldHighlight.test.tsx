// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import type { RecordField } from "@/shared/contracts/common";
import { searchHighlightOf } from "@/shared/lib/searchHighlight";
import { noSearchTerms } from "@/shared/lib/searchTerms";
import { SearchHighlightContext } from "@/shared/ui/Highlighted";
import { RecordFieldTable } from "./RecordFieldTable";

afterEach(cleanup);

test("Record のフィールドの表は、欄を指定した一致をその欄の値だけに付け、期間に入る時刻の欄に付ける", () => {
  const time = "2031-10-08T01:20:35.900Z";
  const fields: RecordField[] = [
    {
      name: "CommandLine",
      semantic: "process.command_line",
      kind: "text",
      text: { valueState: "present", rawText: "cmd /c run" },
    },
    {
      name: "Other",
      kind: "text",
      text: { valueState: "present", rawText: "run" },
    },
    {
      name: "Time",
      kind: "timestamp",
      timestamp: {
        rawText: time,
        normalized: time,
        normalizedForm: "rfc3339_absolute",
        precision: "millisecond",
        offsetState: "in_value",
        clock: "terminal_local",
        meaning: "event",
        valueState: "present",
      },
    },
  ];
  const { container } = render(
    <SearchHighlightContext.Provider
      value={searchHighlightOf(
        { ...noSearchTerms, contains: ["run"], field: "process.command_line" },
        {
          from: { text: "2031-10-08T10:20:30+09:00" },
          to: { text: "2031-10-08T10:20:35+09:00" },
          unit: "second",
        },
      )}
    >
      <RecordFieldTable fields={fields} />
    </SearchHighlightContext.Provider>,
  );
  // 原文と正規化値の 2 つの欄の時刻に付ける。欄の違う `Other` の値には付けない。
  expect(
    [...container.querySelectorAll("mark")].map((mark) => mark.textContent),
  ).toEqual(["run", time, time]);
});
