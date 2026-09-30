// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { CopyButton } from "./CopyButton";

afterEach(() => {
  cleanup();
});

test("クリック時に onCopy を呼び出し、適切なアクセシブル名を持つ", () => {
  const onCopy = vi.fn();
  render(<CopyButton text="example-text" onCopy={onCopy} />);

  const button = screen.getByRole("button", {
    name: "コピー: example-text",
  });
  expect(button).toBeTruthy();

  fireEvent.click(button);
  expect(onCopy).toHaveBeenCalledWith("example-text");
});
