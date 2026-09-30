// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { RowMenuButton, useContextMenu } from "./ContextMenu";
import { type MenuEntry, menuSeparator } from "./Menu";

afterEach(() => {
  cleanup();
});

/** 行ごとに同じ項目を出す一覧。実行した項目と対象を onRun へ渡す。 */
function RowList({
  rows,
  onRun,
}: {
  rows: readonly string[];
  onRun: (item: string, row: string) => void;
}) {
  const { triggers, openTarget, menu } = useContextMenu((row: string) => {
    const entries: MenuEntry[] = [
      {
        kind: "item",
        key: "detail",
        label: "詳細を開く",
        onSelect: () => onRun("detail", row),
      },
      {
        kind: "item",
        key: "disabled",
        label: "使えない項目",
        disabled: true,
        onSelect: () => onRun("disabled", row),
      },
      menuSeparator("copy"),
      {
        kind: "item",
        key: "copy-label",
        label: "表示名を写す",
        onSelect: () => onRun("copy", row),
      },
      {
        kind: "item",
        key: "add",
        label: "含む文字列に足す",
        onSelect: () => onRun("add", row),
      },
    ];
    return { label: `${row} の操作`, entries };
  });
  return (
    <>
      <table>
        <tbody>
          {rows.map((row) => (
            <tr
              key={row}
              onContextMenu={(event) => triggers.onContextMenu(event, row)}
              onKeyDown={(event) => triggers.onKeyDown(event, row)}
            >
              <td>
                <button type="button">{`${row} を選ぶ`}</button>
                <RowMenuButton
                  label={`${row} の操作`}
                  expanded={openTarget === row}
                  onClick={(event) => triggers.onButtonClick(event, row)}
                />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <button type="button">外の button</button>
      {menu}
    </>
  );
}

function renderRows(onRun = vi.fn()) {
  render(<RowList rows={["甲", "乙"]} onRun={onRun} />);
  return onRun;
}

function focusedText(): string | null | undefined {
  return document.activeElement?.textContent;
}

function press(key: string, init: Partial<KeyboardEventInit> = {}) {
  const target = document.activeElement;
  if (target === null) throw new Error("no focused element");
  fireEvent.keyDown(target, { key, ...init });
}

test("Shift+F10 で focus のある行のメニューを開き、最初の使える項目に focus を置く", () => {
  renderRows();
  const opener = screen.getByRole("button", { name: "乙 を選ぶ" });
  opener.focus();
  press("F10", { shiftKey: true });

  expect(screen.getByRole("menu", { name: "乙 の操作" })).toBeInTheDocument();
  expect(screen.getByRole("menuitem", { name: "詳細を開く" })).toHaveFocus();
});

test("ContextMenu キーで開き、修飾キーを伴う F10 と他のキーでは開かない", () => {
  renderRows();
  screen.getByRole("button", { name: "甲 を選ぶ" }).focus();
  press("F10", { shiftKey: true, ctrlKey: true });
  press("a");
  expect(screen.queryByRole("menu")).toBeNull();

  press("ContextMenu");
  expect(screen.getByRole("menu", { name: "甲 の操作" })).toBeInTheDocument();
});

test("右クリックでブラウザーのメニューを止めて開き、Escape で閉じて focus を戻す", () => {
  renderRows();
  const opener = screen.getByRole("button", { name: "甲 を選ぶ" });
  opener.focus();
  const notPrevented = fireEvent.contextMenu(opener, {
    clientX: 40,
    clientY: 60,
  });
  expect(notPrevented).toBe(false);
  const menu = screen.getByRole("menu", { name: "甲 の操作" });
  expect(menu.style.left).toBe("40px");
  expect(menu.style.top).toBe("60px");

  press("Escape");
  expect(screen.queryByRole("menu")).toBeNull();
  expect(opener).toHaveFocus();
});

test("上下の矢印キーは端で回り、Home と End は両端へ移り、使えない項目にも止まる", () => {
  renderRows();
  screen.getByRole("button", { name: "甲 を選ぶ" }).focus();
  press("F10", { shiftKey: true });

  press("ArrowDown");
  expect(focusedText()).toBe("使えない項目");
  press("ArrowDown");
  expect(focusedText()).toBe("表示名を写す");
  press("End");
  expect(focusedText()).toBe("含む文字列に足す");
  press("ArrowDown");
  expect(focusedText()).toBe("詳細を開く");
  press("ArrowUp");
  expect(focusedText()).toBe("含む文字列に足す");
  press("Home");
  expect(focusedText()).toBe("詳細を開く");
});

test("文字を入れると、その文字で始まる次の項目へ移る", () => {
  renderRows();
  screen.getByRole("button", { name: "甲 を選ぶ" }).focus();
  press("F10", { shiftKey: true });

  press("含");
  expect(focusedText()).toBe("含む文字列に足す");
  press("表");
  expect(focusedText()).toBe("表示名を写す");
  press("無");
  expect(focusedText()).toBe("表示名を写す");
});

test("Enter と Space で項目を実行して閉じ、開いた元へ focus を戻す", () => {
  const onRun = renderRows();
  const opener = screen.getByRole("button", { name: "乙 を選ぶ" });
  opener.focus();
  press("F10", { shiftKey: true });
  press("Enter");
  expect(onRun).toHaveBeenCalledWith("detail", "乙");
  expect(screen.queryByRole("menu")).toBeNull();
  expect(opener).toHaveFocus();

  press("F10", { shiftKey: true });
  press("End");
  press(" ");
  expect(onRun).toHaveBeenLastCalledWith("add", "乙");
  expect(screen.queryByRole("menu")).toBeNull();
});

test("aria-disabled の項目は Enter でも click でも実行せず、メニューを残す", () => {
  const onRun = renderRows();
  screen.getByRole("button", { name: "甲 を選ぶ" }).focus();
  press("F10", { shiftKey: true });
  const disabled = screen.getByRole("menuitem", { name: "使えない項目" });
  expect(disabled).toHaveAttribute("aria-disabled", "true");

  press("ArrowDown");
  press("Enter");
  fireEvent.click(disabled);
  expect(onRun).not.toHaveBeenCalled();
  expect(screen.getByRole("menu")).toBeInTheDocument();
});

test("Tab で閉じて開いた元へ focus を戻す", () => {
  renderRows();
  const opener = screen.getByRole("button", { name: "甲 を選ぶ" });
  opener.focus();
  press("F10", { shiftKey: true });
  press("Tab");
  expect(screen.queryByRole("menu")).toBeNull();
  expect(opener).toHaveFocus();
});

test("外側を押すと閉じ、メニューの中を押しても閉じない", () => {
  renderRows();
  screen.getByRole("button", { name: "甲 を選ぶ" }).focus();
  press("F10", { shiftKey: true });
  fireEvent.pointerDown(screen.getByRole("menuitem", { name: "表示名を写す" }));
  expect(screen.getByRole("menu")).toBeInTheDocument();

  fireEvent.pointerDown(screen.getByRole("button", { name: "外の button" }));
  expect(screen.queryByRole("menu")).toBeNull();
});

test("「…」の button はメニューを開き閉じし、開いている間だけ aria-expanded を真にする", () => {
  const onRun = renderRows();
  const button = screen.getByRole("button", { name: "乙 の操作" });
  expect(button).toHaveAttribute("aria-haspopup", "menu");
  expect(button).toHaveAttribute("aria-expanded", "false");

  fireEvent.pointerDown(button);
  fireEvent.click(button);
  expect(button).toHaveAttribute("aria-expanded", "true");
  expect(screen.getByRole("button", { name: "甲 の操作" })).toHaveAttribute(
    "aria-expanded",
    "false",
  );
  fireEvent.click(screen.getByRole("menuitem", { name: "表示名を写す" }));
  expect(onRun).toHaveBeenCalledWith("copy", "乙");
  expect(button).toHaveFocus();

  fireEvent.click(button);
  expect(screen.getByRole("menu")).toBeInTheDocument();
  fireEvent.pointerDown(button);
  fireEvent.click(button);
  expect(screen.queryByRole("menu")).toBeNull();
  expect(button).toHaveAttribute("aria-expanded", "false");
});

test("別の行を右クリックすると、その行のメニューへ替わる", () => {
  const onRun = renderRows();
  fireEvent.contextMenu(screen.getByRole("button", { name: "甲 を選ぶ" }));
  const other = screen.getByRole("button", { name: "乙 を選ぶ" });
  fireEvent.pointerDown(other);
  fireEvent.contextMenu(other);
  expect(screen.getAllByRole("menu")).toHaveLength(1);
  fireEvent.click(screen.getByRole("menuitem", { name: "詳細を開く" }));
  expect(onRun).toHaveBeenCalledWith("detail", "乙");
});
