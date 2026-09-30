// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { ValueCondition } from "@/shared/lib/valueCondition";
import { restoreClipboard, stubClipboard } from "@/testdata/clipboard";
import { ValueActionsForTest } from "@/testdata/valueActions";
import { useContextMenu } from "./ContextMenu";
import {
  useValueMenu,
  ValueMenuButton,
  type ValueMenuTarget,
} from "./ValueMenu";

afterEach(() => {
  cleanup();
  restoreClipboard();
});

const target: ValueMenuTarget = {
  key: "user",
  name: "User",
  text: "user-a",
  conditions: [
    { kind: "field", field: "User", text: "user-a", whole: true },
    { kind: "text", mode: "contains", text: "user-a" },
  ],
  copies: [{ what: "原文の文字列", text: "user-a" }],
};

/** 行のメニューを持つ行の中に、値のメニューの button を 1 つ置いた表。 */
function RowWithValue() {
  const valueMenu = useValueMenu();
  const rowMenu = useContextMenu(() => ({
    label: "行の操作",
    entries: [
      { kind: "item", key: "row", label: "行の項目", onSelect: () => {} },
    ],
  }));
  return (
    <>
      <table>
        <tbody>
          <tr
            onContextMenu={(event) =>
              rowMenu.triggers.onContextMenu(event, "row")
            }
          >
            <td>
              <ValueMenuButton target={target} actions={valueMenu.actions}>
                user-a
              </ValueMenuButton>
            </td>
            <td>行の値</td>
          </tr>
        </tbody>
      </table>
      {valueMenu.menu}
      {rowMenu.menu}
    </>
  );
}

function renderRow(onAddCondition?: (condition: ValueCondition) => void) {
  render(
    onAddCondition === undefined ? (
      <RowWithValue />
    ) : (
      <ValueActionsForTest onAddCondition={onAddCondition}>
        <RowWithValue />
      </ValueActionsForTest>
    ),
  );
}

test("値を押すと条件とコピーの項目のメニューを開き、選んだ条件を表の値の操作に渡す", () => {
  const onAddCondition = vi.fn();
  renderRow(onAddCondition);
  fireEvent.click(
    screen.getByRole("button", { name: "値の操作: User user-a" }),
  );
  const menu = screen.getByRole("menu", { name: "値の操作: User user-a" });
  expect(
    within(menu)
      .getAllByRole("menuitem")
      .map((item) => item.textContent),
  ).toEqual([
    "フィールドの値が等しい条件に追加",
    "含む文字列に追加",
    "原文の文字列をコピー",
  ]);
  fireEvent.click(
    within(menu).getByRole("menuitem", {
      name: "フィールドの値が等しい条件に追加",
    }),
  );
  expect(onAddCondition).toHaveBeenCalledWith(target.conditions[0]);
});

test("値の上の右クリックは値のメニューだけを開き、行の右クリックは行のメニューを開く", () => {
  renderRow(vi.fn());
  fireEvent.contextMenu(
    screen.getByRole("button", { name: "値の操作: User user-a" }),
  );
  expect(
    screen.getByRole("menu", { name: "値の操作: User user-a" }),
  ).toBeTruthy();
  expect(screen.queryByRole("menu", { name: "行の操作" })).toBeNull();
  cleanup();

  renderRow(vi.fn());
  fireEvent.contextMenu(screen.getByText("行の値"));
  expect(screen.getByRole("menu", { name: "行の操作" })).toBeTruthy();
  expect(screen.queryByRole("menu", { name: /^値の操作/ })).toBeNull();
});

test("コピーの項目は値の文字列を整形せずに写す", async () => {
  const clipboard = stubClipboard();
  renderRow(vi.fn());
  fireEvent.click(
    screen.getByRole("button", { name: "値の操作: User user-a" }),
  );
  fireEvent.click(
    screen.getByRole("menuitem", { name: "原文の文字列をコピー" }),
  );
  expect(await screen.findByText("コピー済み: 原文の文字列")).toBeTruthy();
  expect(clipboard.writeText).toHaveBeenCalledWith("user-a");
});

test("表の値の操作の provider が無い画面では、値を押せない文字列で出す", () => {
  renderRow();
  expect(screen.queryByRole("button")).toBeNull();
  expect(screen.getByText("user-a")).toBeTruthy();
});
