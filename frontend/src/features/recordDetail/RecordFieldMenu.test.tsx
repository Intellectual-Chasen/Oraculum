// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { RecordField } from "@/shared/contracts/common";
import type { ValueCondition } from "@/shared/lib/valueCondition";
import { restoreClipboard, stubClipboard } from "@/testdata/clipboard";
import { ValueActionsForTest } from "@/testdata/valueActions";
import { RecordFieldTable } from "./RecordFieldTable";

afterEach(() => {
  cleanup();
  restoreClipboard();
});

/** 前後の空白を持つ原資料の文字列と、正規化値を持つ欄。 */
const commandLine: RecordField = {
  name: "CommandLine",
  kind: "text",
  text: {
    rawText: " tool.exe -x ",
    normalized: "tool.exe -x",
    derivation: "trim",
    valueState: "present",
  },
};

/** 名前が `=` を含み、正規化値を持たない欄。 */
const equalsName: RecordField = {
  name: "Query=Name",
  kind: "text",
  text: { rawText: "value-01", valueState: "present" },
};

/** 原資料の文字列を持たない欄。 */
const missingKey: RecordField = {
  name: "clientPort",
  kind: "text",
  text: { valueState: "item_absent" },
};

/** 時刻のフィールド。ずれを持つ秒の精度の時刻である。 */
const loggedAt: RecordField = {
  name: "LoggedAt",
  kind: "timestamp",
  timestamp: {
    rawText: "2031/04/05 06:07:08",
    normalized: "2031-04-05T06:07:08+09:00",
    normalizedForm: "rfc3339_absolute",
    precision: "second",
    offsetState: "in_value",
    clock: "terminal_local",
    meaning: "event",
    valueState: "present",
  },
};

const fields = [commandLine, equalsName, missingKey, loggedAt];

/** 表を描く。`onAddCondition` を渡すと、表の値の操作の provider の中に描く。 */
function renderTable(onAddCondition?: (condition: ValueCondition) => void) {
  const table = <RecordFieldTable fields={fields} />;
  render(
    onAddCondition === undefined ? (
      table
    ) : (
      <ValueActionsForTest onAddCondition={onAddCondition}>
        {table}
      </ValueActionsForTest>
    ),
  );
}

function openFieldMenu(name: string) {
  fireEvent.click(
    screen.getByRole("button", { name: `フィールド ${name} の操作` }),
  );
  return screen.getByRole("menu", { name: `フィールド ${name} の操作` });
}

function itemNames(menu: HTMLElement) {
  return within(menu)
    .getAllByRole("menuitem")
    .map((item) => item.textContent);
}

function isDisabled(name: string) {
  return (
    screen.getByRole("menuitem", { name }).getAttribute("aria-disabled") ===
    "true"
  );
}

function choose(name: string) {
  fireEvent.click(screen.getByRole("menuitem", { name }));
}

test("欄のメニューは、フィールドの値が等しい条件を先に出し、どの条件も前後の空白を持つ原文で作る", () => {
  const onAddCondition = vi.fn();
  renderTable(onAddCondition);

  expect(itemNames(openFieldMenu("CommandLine"))).toEqual([
    "フィールドの値が等しい条件に追加",
    "フィールドの値が文字列を含む条件に追加",
    "含む文字列に追加",
    "含まない文字列に追加",
    "原文の文字列をコピー",
    "正規化値をコピー",
    "フィールドの名前をコピー",
  ]);
  const expected: [string, ValueCondition][] = [
    [
      "フィールドの値が等しい条件に追加",
      {
        kind: "field",
        field: "CommandLine",
        text: " tool.exe -x ",
        whole: true,
      },
    ],
    [
      "フィールドの値が文字列を含む条件に追加",
      {
        kind: "field",
        field: "CommandLine",
        text: " tool.exe -x ",
        whole: false,
      },
    ],
    [
      "含む文字列に追加",
      { kind: "text", mode: "contains", text: " tool.exe -x " },
    ],
    [
      "含まない文字列に追加",
      { kind: "text", mode: "excludes", text: " tool.exe -x " },
    ],
  ];
  for (const [index, [item, condition]] of expected.entries()) {
    // 最初の項目は、項目の並びを確かめたときに開いたメニューから選ぶ。
    if (index > 0) openFieldMenu("CommandLine");
    choose(item);
    expect(onAddCondition).toHaveBeenLastCalledWith(condition);
  }
  expect(onAddCondition).toHaveBeenCalledTimes(expected.length);
});

test("時刻の欄のメニューは、時刻の値から作った期間の条件を先に出す", () => {
  const onAddCondition = vi.fn();
  renderTable(onAddCondition);
  expect(itemNames(openFieldMenu("LoggedAt"))[0]).toBe("期間の条件に追加");
  choose("期間の条件に追加");
  expect(onAddCondition).toHaveBeenCalledWith({
    kind: "period",
    filter: {
      from: { text: "2031-04-05T06:07:08+09:00", precision: "second" },
      to: { text: "2031-04-05T06:07:08+09:00", precision: "second" },
      unit: "second",
    },
  });
});

test("イベントの種類の欄のメニューは、応答の組のままイベントの種類の条件を先に出す", () => {
  const onAddCondition = vi.fn();
  const eventId: RecordField = {
    name: "EventID",
    semantic: "windows_event.id",
    kind: "text",
    text: { rawText: "9001", valueState: "present" },
  };
  render(
    <ValueActionsForTest onAddCondition={onAddCondition}>
      <RecordFieldTable
        fields={[eventId]}
        eventKind={{ category: "synthetic-provider", action: "9001" }}
      />
    </ValueActionsForTest>,
  );
  expect(itemNames(openFieldMenu("EventID"))[0]).toBe(
    "イベントの種類の条件に追加",
  );
  choose("イベントの種類の条件に追加");
  expect(onAddCondition).toHaveBeenCalledWith({
    kind: "eventKind",
    category: "synthetic-provider",
    action: "9001",
  });
});

test("名前が = を含む欄はフィールドの条件を出さず、正規化値の無い欄は正規化値を写す操作を使えなくする", () => {
  renderTable(vi.fn());
  const names = itemNames(openFieldMenu("Query=Name"));

  expect(names).not.toContain("フィールドの値が等しい条件に追加");
  expect(names).not.toContain("フィールドの値が文字列を含む条件に追加");
  expect(names).toContain("含む文字列に追加");
  expect(isDisabled("正規化値をコピー")).toBe(true);
});

test("原資料の文字列を持たない欄は、文字列の条件を出さず、写す操作を使えなくし、欄の名前は写せる", async () => {
  const clipboard = stubClipboard();
  renderTable(vi.fn());
  expect(itemNames(openFieldMenu("clientPort"))).toEqual([
    "原文の文字列をコピー",
    "正規化値をコピー",
    "フィールドの名前をコピー",
  ]);

  for (const name of ["原文の文字列をコピー", "正規化値をコピー"]) {
    expect(isDisabled(name)).toBe(true);
  }
  choose("フィールドの名前をコピー");
  expect(
    await screen.findByText("コピー済み: フィールドの名前"),
  ).toBeInTheDocument();
  expect(clipboard.writeText).toHaveBeenCalledWith("clientPort");
});

test("条件へ足す操作を渡さない表は、写す操作だけを出す", () => {
  renderTable();
  expect(itemNames(openFieldMenu("CommandLine"))).toEqual([
    "原文の文字列をコピー",
    "正規化値をコピー",
    "フィールドの名前をコピー",
  ]);
});

test("原資料の文字列と正規化値を、それぞれ整形せずに写す", async () => {
  const clipboard = stubClipboard();
  renderTable();
  openFieldMenu("CommandLine");
  choose("原文の文字列をコピー");
  expect(
    await screen.findByText("コピー済み: 原文の文字列"),
  ).toBeInTheDocument();
  expect(clipboard.writeText).toHaveBeenLastCalledWith(" tool.exe -x ");

  openFieldMenu("CommandLine");
  choose("正規化値をコピー");
  expect(await screen.findByText("コピー済み: 正規化値")).toBeInTheDocument();
  expect(clipboard.writeText).toHaveBeenLastCalledWith("tool.exe -x");
});

test("欄の行の右クリックでも同じメニューを開く", () => {
  renderTable();
  const row = screen
    .getByRole("button", { name: "フィールド CommandLine の操作" })
    .closest("tr");
  if (row === null) throw new Error("the field row is missing");
  expect(fireEvent.contextMenu(row)).toBe(false);
  expect(
    screen.getByRole("menu", { name: "フィールド CommandLine の操作" }),
  ).toBeInTheDocument();
});
