// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import {
  removeClipboard,
  restoreClipboard,
  stubClipboard,
} from "@/testdata/clipboard";
import { copyText, useCopyText } from "./useCopyText";

afterEach(() => {
  cleanup();
  restoreClipboard();
  vi.useRealTimers();
});

/** 原資料の文字列の形だけを保つテスト値。前後の空白とタブを含む。 */
const rawText = "  C:\\Temp\\tool.exe\t-x ";

function CopyHarness() {
  const { copy, notice } = useCopyText();
  return (
    <>
      <button type="button" onClick={() => copy(rawText, "原文")}>
        コピー
      </button>
      {notice}
    </>
  );
}

test("copyText は文字列をそのまま渡し、書けたことを返す", async () => {
  const writeText = vi.fn(async (_text: string) => {});
  expect(await copyText(rawText, { writeText })).toEqual({ ok: true });
  expect(writeText).toHaveBeenCalledWith(rawText);
});

test("copyText は clipboard が無いときと拒まれたときを分けて返し、投げない", async () => {
  expect(await copyText(rawText, undefined)).toEqual({
    ok: false,
    reason: "unavailable",
  });
  const writeText = vi.fn(async () => {
    throw new DOMException("denied", "NotAllowedError");
  });
  expect(await copyText(rawText, { writeText })).toEqual({
    ok: false,
    reason: "rejected",
  });
});

test("コピーできたら何をコピーしたかを知らせ、少し後に通知を消す", async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  const clipboard = stubClipboard();
  render(<CopyHarness />);
  fireEvent.click(screen.getByRole("button", { name: "コピー" }));

  expect(await screen.findByRole("status")).toHaveTextContent(
    "コピー済み: 原文",
  );
  expect(clipboard.writeText).toHaveBeenCalledWith(rawText);
  vi.advanceTimersByTime(5000);
  await waitFor(() => expect(screen.queryByRole("status")).toBeNull());
});

test("拒まれたらコピーできなかったことと理由を残し、閉じるまで消さない", async () => {
  stubClipboard(async () => {
    throw new DOMException("denied", "NotAllowedError");
  });
  render(<CopyHarness />);
  fireEvent.click(screen.getByRole("button", { name: "コピー" }));

  const alert = await screen.findByRole("alert");
  expect(alert).toHaveTextContent(/^コピーできません: 原文/);
  expect(alert).toHaveTextContent("理由: ブラウザーが書き込みを拒否");
  fireEvent.click(screen.getByRole("button", { name: "閉じる" }));
  expect(screen.queryByRole("alert")).toBeNull();
});

test("clipboard を使えない画面では、開き直す方法を知らせる", async () => {
  removeClipboard();
  render(<CopyHarness />);
  fireEvent.click(screen.getByRole("button", { name: "コピー" }));

  expect(await screen.findByRole("alert")).toHaveTextContent(
    "操作: https か localhost の URL で開く",
  );
});
