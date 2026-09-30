// @vitest-environment jsdom
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { TextEditorPopover } from "./TextEditorPopover";

afterEach(cleanup);

function editor() {
  const onCommit = vi.fn();
  render(
    <TextEditorPopover
      label="書式を編集"
      inputLabel="ログ書式"
      value="saved"
      description="書式の説明"
      target="logs/proxy.log"
      onCommit={onCommit}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "書式を編集" }));
  return onCommit;
}

function pendingFile(name: string) {
  let finish: (value: string) => void = () => {
    throw new Error("the file was not read");
  };
  const file = new File(["spec"], name);
  Object.defineProperty(file, "text", {
    value: () =>
      new Promise<string>((resolve) => {
        finish = resolve;
      }),
  });
  fireEvent.change(screen.getByLabelText("書式のテキストファイル"), {
    target: { files: [file] },
  });
  return (value: string) =>
    act(async () => {
      finish(value);
    });
}

test("書式ファイルを読む間は古い書式の適用を止め、最新の選択結果だけを表示する", async () => {
  const onCommit = editor();
  const first = pendingFile("first.txt");
  expect(screen.getByRole("button", { name: "適用" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  const second = pendingFile("second.txt");
  await second("latest");
  await first("stale");
  expect(screen.getByRole("textbox", { name: "ログ書式" })).toHaveValue(
    "latest",
  );
  fireEvent.click(screen.getByRole("button", { name: "適用" }));
  expect(onCommit).toHaveBeenCalledWith("latest", false);
});

test("ファイル読込中に手入力した内容を遅い読み込み結果で上書きしない", async () => {
  editor();
  const finish = pendingFile("slow.txt");
  fireEvent.change(screen.getByRole("textbox", { name: "ログ書式" }), {
    target: { value: "manual" },
  });
  await finish("stale");
  expect(screen.getByRole("textbox", { name: "ログ書式" })).toHaveValue(
    "manual",
  );
});

test("編集を取り消して開き直した入力を以前のファイル読込で変更しない", async () => {
  editor();
  const finish = pendingFile("slow.txt");
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  fireEvent.click(screen.getByRole("button", { name: "書式を編集" }));
  await finish("stale");
  expect(screen.getByRole("textbox", { name: "ログ書式" })).toHaveValue(
    "saved",
  );
});

test("書式ファイルの読込失敗を表示し、手入力で修正すると失敗表示を解除する", async () => {
  const onCommit = editor();
  const file = new File(["spec"], "broken.txt");
  Object.defineProperty(file, "text", {
    value: () => Promise.reject(new Error("synthetic read failure")),
  });
  await act(async () => {
    fireEvent.change(screen.getByLabelText("書式のテキストファイル"), {
      target: { files: [file] },
    });
  });
  expect(screen.getByRole("alert")).toHaveTextContent(
    "書式ファイルを読めませんでした",
  );
  fireEvent.change(screen.getByRole("textbox", { name: "ログ書式" }), {
    target: { value: "corrected" },
  });
  expect(screen.queryByRole("alert")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "適用" }));
  expect(onCommit).toHaveBeenCalledWith("corrected", false);
});
