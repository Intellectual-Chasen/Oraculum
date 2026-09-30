import { expect, test } from "vitest";
import { drawnNodeLabel, lastPathElement } from "./NodeLabelView";

test.each([
  ['"C:\\Windows\\System32\\OpenSSH\\ssh.exe"', "ssh.exe"],
  ["C:\\Users\\user01\\Desktop\\app.exe", "app.exe"],
  ["/usr/bin/grep", "grep"],
  ["HOST-C", "HOST-C"],
  ['"HOST-C"', '"HOST-C"'],
  ["C:\\data\\", "C:\\data\\"],
])("path の形の文字列 %s を最後の要素 %s にする", (text, expected) => {
  expect(lastPathElement(text)).toBe(expected);
});

test("図のラベルは path の最後の要素だけを描き、導いた値にも注記を添えない", () => {
  expect(
    drawnNodeLabel({
      value: { text: "C:\\Windows\\System32\\cmd.exe" },
      valueState: "present",
    }),
  ).toBe("cmd.exe");
  expect(
    drawnNodeLabel({
      value: { text: "C:\\Windows\\System32\\cmd.exe" },
      valueState: "derived",
      derivation: "file.path の末尾の要素",
    }),
  ).toBe("cmd.exe");
});
