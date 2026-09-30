// @vitest-environment jsdom
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import type { Timestamp } from "@/shared/contracts/common";
import type { TerminalAssignment } from "@/shared/contracts/terminalAssignments";
import { DisplayOffsetContext } from "@/shared/ui/DisplayOffset";
import { EdgeTerminalAssignments } from "./EdgeTerminalAssignments";

afterEach(cleanup);

function wallClock(rawText: string, normalized: string): Timestamp {
  return {
    rawText,
    normalized,
    normalizedForm: "local_without_offset",
    precision: "second",
    offsetState: "undetermined",
    clock: "terminal_local",
    meaning: "event",
    valueState: "present",
  };
}

const assignment: TerminalAssignment = {
  clientIp: "192.0.2.10",
  terminalId: "host-a",
  sourceId: "source-a",
  sourceContentSha256: "0".repeat(64),
  assignmentValidRange: {
    from: wallClock("2031/01/02 03:04:05", "2031-01-02T03:04:05"),
    to: wallClock("2031/01/02 04:05:06", "2031-01-02T04:05:06"),
  },
  origin: "import_specified",
};

test("関係を作った割当の地方時の適用期間を、収集元の解釈で読んで地方時と出どころを添える", () => {
  render(
    <DisplayOffsetContext.Provider value="+09:00">
      <EdgeTerminalAssignments
        edgeKind="terminal_address"
        assignments={[assignment]}
        sourceFileNames={undefined}
        sourceInterpretations={new Map([["source-a", { offset: "+00:00" }]])}
      />
    </DisplayOffsetContext.Provider>,
  );

  const row = within(screen.getByRole("table")).getAllByRole("row")[1];
  // 表示のタイムゾーンの時刻と原文は、UTC の時刻の title に入れる。
  const from = within(row).getAllByText("2031-01-02T03:04:05Z")[0];
  expect(from).toHaveAttribute(
    "title",
    "UTC+09:00: 2031-01-02T12:04:05+09:00\n原文: 2031/01/02 03:04:05",
  );
  expect(row).toHaveTextContent("起動時の指定: UTC+00:00");
});
