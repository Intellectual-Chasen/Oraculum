import { vi } from "vitest";

/**
 * navigator.clipboard を、渡された文字列を記録する stub に置き換える。writeText の動作は
 * 引数で替える。jsdom は clipboard を持たない。test の後に restoreClipboard で外す。
 */
export function stubClipboard(
  writeText: (text: string) => Promise<void> = async () => {},
) {
  const clipboard = { writeText: vi.fn(writeText) };
  Object.defineProperty(navigator, "clipboard", {
    value: clipboard,
    configurable: true,
  });
  return clipboard;
}

/** navigator.clipboard を持たない画面 (https でも localhost でもない URL) にする。 */
export function removeClipboard() {
  Object.defineProperty(navigator, "clipboard", {
    value: undefined,
    configurable: true,
  });
}

/** stubClipboard と removeClipboard が置いた値を外す。 */
export function restoreClipboard() {
  Reflect.deleteProperty(navigator, "clipboard");
}
