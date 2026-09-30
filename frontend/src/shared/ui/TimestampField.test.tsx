// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { TimestampField } from "./TimestampField";

afterEach(cleanup);

test.each([
  ["", "+05:45", "2032-06-12T09:15:00+05:45"],
  ["", "Z", "2032-06-12T09:15:00Z"],
  ["2032-06-10T04:05:06Z", "+09:00", "2032-06-12T09:15:00Z"],
  ["2032-06-10T04:05:06.000-03:30", "Z", "2032-06-12T09:15:00.000-03:30"],
  ["2032-06-10T04:05:06.123456789", "", "2032-06-12T09:15:00.123456789"],
])(
  "日時選択が入力済みのオフセットと精度を保つ: %s",
  (value, defaultOffset, expected) => {
    const onChange = vi.fn();
    render(
      <TimestampField
        label="対象時刻"
        value={value}
        defaultOffset={defaultOffset}
        onChange={onChange}
      />,
    );
    fireEvent.change(screen.getByLabelText("対象時刻の日時選択"), {
      target: { value: "2032-06-12T09:15" },
    });
    expect(onChange).toHaveBeenCalledWith(expected);
  },
);

test("カレンダーボタンでブラウザの日時選択を開き、消去した入力を通知する", () => {
  const onChange = vi.fn();
  render(
    <TimestampField
      label="対象時刻"
      value="2032-06-10T04:05:06Z"
      defaultOffset="Z"
      onChange={onChange}
    />,
  );
  const picker = screen.getByLabelText("対象時刻の日時選択");
  const showPicker = vi.fn();
  Object.defineProperty(picker, "showPicker", { value: showPicker });
  fireEvent.click(
    screen.getByRole("button", { name: "対象時刻をカレンダーで選択" }),
  );
  expect(showPicker).toHaveBeenCalledOnce();
  expect(onChange).not.toHaveBeenCalled();
  fireEvent.change(picker, { target: { value: "" } });
  expect(onChange).toHaveBeenCalledWith("");
});
