// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { SourceRef } from "@/shared/api/graph";
import type { SourceIdentity } from "@/shared/contracts/sources";
import { restoreClipboard, stubClipboard } from "@/testdata/clipboard";
import { jsonResponse } from "@/testdata/http";
import {
  accessLogSha256,
  accessLogSourceId,
  hostALogSourceId,
  sourcesResponseJson,
} from "@/testdata/sources/sourcesResponse";
import { SourceInventory } from "./SourceInventory";
import { useSourceInventory } from "./useSourceInventory";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  restoreClipboard();
});

/** 上位の画面と同じく、一覧の取得を `useSourceInventory` に任せて表を出す。 */
function InventoryUnderTest(props: {
  selectedSource: SourceIdentity | undefined;
  onSelect: (source: SourceIdentity) => void;
  narrowedSourceIds?: readonly string[];
  onNarrowToSource?: (source: SourceRef) => void;
}) {
  const { state } = useSourceInventory();
  return <SourceInventory state={state} {...props} />;
}

function renderInventory(
  props: {
    selectedSource?: SourceIdentity;
    narrowedSourceIds?: readonly string[];
    onNarrowToSource?: (source: SourceRef) => void;
  } = {},
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse(200, sourcesResponseJson())),
  );
  const onSelect = vi.fn<(source: SourceIdentity) => void>();
  render(
    <InventoryUnderTest
      selectedSource={props.selectedSource}
      onSelect={onSelect}
      narrowedSourceIds={props.narrowedSourceIds}
      onNarrowToSource={props.onNarrowToSource}
    />,
  );
  return onSelect;
}

const accessMenuName = "収集元 access.log の操作";

async function openAccessMenu() {
  fireEvent.click(await screen.findByRole("button", { name: accessMenuName }));
  return screen.getByRole("menu", { name: accessMenuName });
}

function itemNames(menu: HTMLElement) {
  return within(menu)
    .getAllByRole("menuitem")
    .map((item) => item.textContent);
}

function choose(name: string) {
  fireEvent.click(screen.getByRole("menuitem", { name }));
}

test("収集元の行のメニューは、選ぶ操作とその収集元のレコードだけを残すフィルタを適用する操作を行の収集元で呼ぶ", async () => {
  const onNarrowToSource = vi.fn();
  const onSelect = renderInventory({ onNarrowToSource });

  expect(itemNames(await openAccessMenu())).toEqual([
    "詳細を表示",
    "この収集元のフィルタを適用",
    "SHA-256 をコピー",
  ]);
  choose("詳細を表示");
  expect(onSelect).toHaveBeenCalledWith(
    expect.objectContaining({
      sourceId: accessLogSourceId,
      fileName: "access.log",
    }),
  );

  await openAccessMenu();
  choose("この収集元のフィルタを適用");
  expect(onNarrowToSource).toHaveBeenCalledWith({
    id: accessLogSourceId,
    label: "access.log",
  });
});

test("選んでいる収集元の選ぶ項目と、その収集元だけのフィルタを適用しているときのフィルタを適用する項目を使えなくする", async () => {
  const onNarrowToSource = vi.fn();
  const first = renderInventory({
    narrowedSourceIds: [accessLogSourceId],
    onNarrowToSource,
  });
  await openAccessMenu();
  expect(
    screen.getByRole("menuitem", {
      name: "この収集元のフィルタを適用",
    }),
  ).toHaveAttribute("aria-disabled", "true");
  choose("この収集元のフィルタを適用");
  expect(onNarrowToSource).not.toHaveBeenCalled();
  choose("詳細を表示");
  const [[selected]] = first.mock.calls;
  cleanup();

  // 選んだ収集元を上位の画面が持ち直して渡すと、その行の選ぶ項目を使えなくする。
  const second = renderInventory({
    selectedSource: selected,
    narrowedSourceIds: [accessLogSourceId, hostALogSourceId],
    onNarrowToSource,
  });
  await openAccessMenu();
  expect(screen.getByRole("menuitem", { name: "詳細を表示" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  choose("詳細を表示");
  expect(second).not.toHaveBeenCalled();
  choose("この収集元のフィルタを適用");
  expect(onNarrowToSource).toHaveBeenCalledWith({
    id: accessLogSourceId,
    label: "access.log",
  });
});

test("フィルタを適用する操作を渡さない一覧はフィルタを適用する項目を出さず、sha256 を写すは内容の識別をそのまま写す", async () => {
  const clipboard = stubClipboard();
  renderInventory();
  expect(itemNames(await openAccessMenu())).toEqual([
    "詳細を表示",
    "SHA-256 をコピー",
  ]);
  choose("SHA-256 をコピー");

  expect(await screen.findByText("コピー済み: SHA-256")).toBeInTheDocument();
  expect(clipboard.writeText).toHaveBeenCalledWith(accessLogSha256);
});

test("収集元の行は右クリックと Shift+F10 でも同じメニューを開く", async () => {
  renderInventory();
  const select = await screen.findByRole("button", { name: "access.log" });
  const row = select.closest("tr");
  if (row === null) throw new Error("the source row is missing");
  expect(fireEvent.contextMenu(row)).toBe(false);
  fireEvent.keyDown(screen.getByRole("menu", { name: accessMenuName }), {
    key: "Escape",
  });

  select.focus();
  fireEvent.keyDown(select, { key: "F10", shiftKey: true });
  expect(screen.getByRole("menuitem", { name: "詳細を表示" })).toHaveFocus();
  fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
  expect(select).toHaveFocus();
});
