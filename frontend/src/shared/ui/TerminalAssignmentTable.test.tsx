// @vitest-environment jsdom
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import type { Timestamp } from "../contracts/common";
import type { TerminalAssignment } from "../contracts/terminalAssignments";
import { DisplayOffsetContext } from "./DisplayOffset";
import { TerminalAssignmentTable } from "./TerminalAssignmentTable";

afterEach(cleanup);

function localTime(rawText: string): Timestamp {
  return {
    rawText,
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
    from: localTime("2031/01/02 03:04:05"),
    to: localTime("2031/01/02 04:05:06"),
  },
  origin: "import_specified",
};

test("適用期間を原文の「始まり – 終わり」で出し、見出しに件数の組を出す", () => {
  render(
    <TerminalAssignmentTable
      assignments={[assignment]}
      caption="端末の割り当て"
      sourceFileNames={undefined}
    />,
  );

  expect(
    screen.getByRole("table", { name: "端末の割り当て: 1 件" }),
  ).toBeTruthy();
  const row = within(screen.getByRole("table")).getAllByRole("row")[1];
  expect(row).toHaveTextContent("2031/01/02 03:04:05 – 2031/01/02 04:05:06");
  // 期間の両端を「—」で繋がない。「—」は値の無い欄の印だけに使う。
  expect(row).not.toHaveTextContent(/\d—\d/);
});

test("UTC に直せない適用期間には、タイムゾーン不明の印を添える", () => {
  const utc: Timestamp = {
    rawText: "2031-01-02T03:04:05Z",
    normalized: "2031-01-02T03:04:05Z",
    normalizedForm: "rfc3339_absolute",
    precision: "second",
    offsetState: "in_value",
    clock: "terminal_local",
    meaning: "event",
    valueState: "present",
  };
  render(
    <TerminalAssignmentTable
      assignments={[
        assignment,
        { ...assignment, assignmentValidRange: { from: utc, to: utc } },
      ]}
      caption="端末の割当"
      sourceFileNames={undefined}
    />,
  );

  const [, local, dated] = within(screen.getByRole("table")).getAllByRole(
    "row",
  );
  expect(local).toHaveTextContent("タイムゾーン不明");
  expect(dated).not.toHaveTextContent("タイムゾーン不明");
});

test("期間を取った収集元に時刻の解釈があるときは、そのずれで読んだ UTC の期間を出す", () => {
  const wallClock = (text: string, normalized: string): Timestamp => ({
    ...localTime(text),
    normalized,
    normalizedForm: "local_without_offset",
  });
  render(
    <TerminalAssignmentTable
      assignments={[
        {
          ...assignment,
          assignmentValidRange: {
            from: wallClock("2031/01/02 03:04:05", "2031-01-02T03:04:05"),
            to: wallClock("2031/01/02 04:05:06", "2031-01-02T04:05:06"),
          },
        },
      ]}
      caption="端末の割当"
      sourceFileNames={undefined}
      sourceInterpretations={
        new Map([["source-a", { offset: "+09:00", assertionId: "as-1" }]])
      }
    />,
  );

  const row = within(screen.getByRole("table")).getAllByRole("row")[1];
  expect(row).toHaveTextContent("2031-01-01T18:04:05Z");
  expect(row).not.toHaveTextContent("タイムゾーン不明");
  // 与えたタイムゾーンで読んだ期間は、原文の事実と読まれないよう、タイムゾーンを添える。
  expect(row).toHaveTextContent("分析者の記録: UTC+09:00");
});

test("表示のタイムゾーンを選ぶと、与えたタイムゾーンで読んだ期間の両端の title にその時刻と原文を入れる", () => {
  const wallClock = (text: string, normalized: string): Timestamp => ({
    ...localTime(text),
    normalized,
    normalizedForm: "local_without_offset",
  });
  render(
    <DisplayOffsetContext.Provider value="+09:00">
      <TerminalAssignmentTable
        assignments={[
          {
            ...assignment,
            assignmentValidRange: {
              from: wallClock("2031/01/02 03:04:05", "2031-01-02T03:04:05"),
              to: wallClock("2031/01/02 04:05:06", "2031-01-02T04:05:06"),
            },
          },
        ]}
        caption="端末の割当"
        sourceFileNames={undefined}
        sourceInterpretations={new Map([["source-a", { offset: "+09:00" }]])}
      />
    </DisplayOffsetContext.Provider>,
  );

  const header = within(screen.getByRole("table")).getAllByRole("columnheader");
  const periodColumn = header.findIndex(
    (cell) => cell.textContent === "適用期間",
  );
  const cell = within(screen.getByRole("table")).getAllByRole("row")[1]
    ?.children[periodColumn];
  expect(cell?.textContent).toBe(
    "2031-01-01T18:04:05Z – 2031-01-01T19:05:06Z" +
      // メモの識別子を持たないタイムゾーンは、取り込みの起動で指定したタイムゾーンである。
      "起動時の指定: UTC+09:00",
  );
  const titles = [...(cell?.querySelectorAll("[title]") ?? [])].map((node) =>
    node.getAttribute("title"),
  );
  expect(titles).toEqual([
    "UTC+09:00: 2031-01-02T03:04:05+09:00\n原文: 2031/01/02 03:04:05",
    "UTC+09:00: 2031-01-02T04:05:06+09:00\n原文: 2031/01/02 04:05:06",
  ]);
});
