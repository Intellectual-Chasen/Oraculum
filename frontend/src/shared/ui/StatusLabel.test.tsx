// @vitest-environment jsdom
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { KeyValueList } from "./KeyValueList";
import { StatusLabel } from "./StatusLabel";

afterEach(() => {
  cleanup();
});

test("詳細を持つラベルは、tooltip を開く前から詳細を読み上げに結び付け、focus で tooltip に出す", () => {
  render(
    <StatusLabel
      status="failed"
      label="収集元なし"
      details={
        <KeyValueList
          stacked
          pairs={[
            { name: "コード", value: "source_not_found" },
            { name: "収集元", value: undefined },
          ]}
        />
      }
    />,
  );
  const label = screen.getByText("収集元なし");
  const described = document.getElementById(
    label.getAttribute("aria-describedby") ?? "",
  );
  expect(described?.textContent).toBe("コード: source_not_found");
  act(() => {
    fireEvent.keyDown(document.body, { key: "Tab" });
    label.focus();
  });
  expect(screen.getByRole("tooltip").textContent).toBe(
    "コード: source_not_found",
  );
});

test("詳細を持つラベルは、focus を受ける要素の role の警告を出さない", () => {
  const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
  render(
    <StatusLabel
      status="failed"
      label="収集元なし"
      details={
        <KeyValueList pairs={[{ name: "コード", value: "source_not_found" }]} />
      }
    />,
  );
  expect(screen.getByRole("img", { name: "収集元なし" })).toBeDefined();
  expect(warn).not.toHaveBeenCalled();
  warn.mockRestore();
});

test("詳細を持たないラベルは focus を受けない", () => {
  render(<StatusLabel status="done" label="完了" />);
  expect(screen.getByText("完了").hasAttribute("tabindex")).toBe(false);
});
