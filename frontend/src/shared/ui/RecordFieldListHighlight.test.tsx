// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import type { RecordField } from "@/shared/contracts/common";
import { readRecordFieldRawText } from "@/shared/lib/recordField";
import { searchHighlightOf } from "@/shared/lib/searchHighlight";
import { noSearchTerms } from "@/shared/lib/searchTerms";
import { SearchHighlightContext } from "@/shared/ui/Highlighted";
import { RecordFieldList } from "@/shared/ui/RecordFieldList";

afterEach(cleanup);

test("RecordFieldList は期間に入る timestamp の値を強調する", () => {
  const time = "2031-10-08T10:20:35Z";
  const fields: RecordField[] = [
    {
      name: "EventTime",
      kind: "timestamp",
      timestamp: {
        rawText: time,
        normalized: time,
        normalizedForm: "rfc3339_absolute",
        precision: "second",
        offsetState: "in_value",
        clock: "terminal_local",
        meaning: "event",
        valueState: "present",
      },
    },
  ];
  const { container } = render(
    <SearchHighlightContext.Provider
      value={searchHighlightOf(noSearchTerms, {
        from: { text: "2031-10-08T10:20:30Z" },
        to: { text: "2031-10-08T10:20:40Z" },
        unit: "second",
      })}
    >
      <RecordFieldList fields={fields} readValue={readRecordFieldRawText} />
    </SearchHighlightContext.Provider>,
  );

  expect(
    [...container.querySelectorAll("mark")].map((mark) => mark.textContent),
  ).toEqual([time]);
});
