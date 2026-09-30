// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { Timestamp } from "../contracts/common";
import { TimeValueLink, ValueActionsContext, ValueLink } from "./ValueLink";

afterEach(cleanup);

const absolute: Timestamp = {
  rawText: "2031-10-08T10:20:35.100+09:00",
  normalized: "2031-10-08T10:20:35.100+09:00",
  normalizedForm: "rfc3339_absolute",
  precision: "millisecond",
  offsetState: "in_value",
  clock: "terminal_local",
  meaning: "event",
  valueState: "present",
};

test("provider の無い画面では、値を操作できない文字列として出す", () => {
  render(
    <ValueLink target={{ kind: "edge", id: "e:1" }} hover="種類">
      値
    </ValueLink>,
  );
  expect(screen.getByText("値")).toBeTruthy();
  expect(screen.queryByRole("button")).toBeNull();
});

test("値を押すと、provider の操作に値が指すものを渡す", () => {
  const open = vi.fn();
  render(
    <ValueActionsContext value={{ open, addCondition: () => {} }}>
      <ValueLink
        target={{ kind: "node", id: "n:1", label: "HOST-C" }}
        hover="端末"
      >
        HOST-C
      </ValueLink>
      <TimeValueLink timestamp={absolute}>時刻</TimeValueLink>
      <TimeValueLink timestamp={{ ...absolute, offsetState: "undetermined" }}>
        地方時
      </TimeValueLink>
    </ValueActionsContext>,
  );

  fireEvent.click(screen.getByRole("button", { name: "HOST-C" }));
  fireEvent.click(screen.getByRole("button", { name: "時刻" }));

  expect(open.mock.calls).toEqual([
    [{ kind: "node", id: "n:1", label: "HOST-C" }],
    [{ kind: "time", utc: "2031-10-08T01:20:35.100Z" }],
  ]);
  // 時点が定まらない時刻は操作できない。
  expect(screen.queryByRole("button", { name: "地方時" })).toBeNull();
});
