// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import type { Timestamp } from "../contracts/common";
import { DisplayOffsetContext } from "./DisplayOffset";
import { TimestampOffsetNote } from "./TimestampOffsetNote";
import { TimestampText } from "./TimestampText";

const utcTimestamp: Timestamp = {
  rawText: "2001-02-03T23:44:25.5Z",
  normalized: "2001-02-03T23:44:25.5Z",
  normalizedForm: "rfc3339_absolute",
  precision: "millisecond",
  offsetState: "in_value",
  clock: "terminal_local",
  meaning: "event",
  valueState: "present",
};

const localTimestamp: Timestamp = {
  rawText: "2001/02/04 08:44:25",
  normalized: "2001-02-04T08:44:25",
  normalizedForm: "local_without_offset",
  precision: "second",
  offsetState: "item_absent",
  clock: "terminal_local",
  meaning: "event",
  valueState: "present",
};

afterEach(cleanup);

test("表示のタイムゾーンを選ぶと、UTC に直せる時刻にそのタイムゾーンの時刻を組で添える", () => {
  render(
    <DisplayOffsetContext.Provider value="+09:00">
      <p data-testid="text">
        <TimestampText timestamp={utcTimestamp} />
      </p>
      <p data-testid="note">
        <TimestampOffsetNote timestamp={utcTimestamp} />
      </p>
    </DisplayOffsetContext.Provider>,
  );
  // 時刻の本文は UTC だけにし、表示のタイムゾーンの時刻は title に入れる。
  const text = screen.getByTestId("text");
  expect(text.textContent).toBe("2001-02-03T23:44:25.5Z");
  expect(text.querySelector("[title]")?.getAttribute("title")).toBe(
    "UTC+09:00: 2001-02-04T08:44:25.5+09:00",
  );
  expect(screen.getByTestId("note").textContent).toBe(
    "UTC+09:00: 2001-02-04T08:44:25.5+09:00",
  );
});

test("表示のタイムゾーンを選んでいないときと、UTC に直せない時刻には表示のタイムゾーンの時刻を添えない", () => {
  render(
    <>
      <p data-testid="unselected">
        <TimestampText timestamp={utcTimestamp} />
      </p>
      <DisplayOffsetContext.Provider value="+09:00">
        <p data-testid="local">
          <TimestampOffsetNote timestamp={localTimestamp} />
        </p>
      </DisplayOffsetContext.Provider>
    </>,
  );
  const unselected = screen.getByTestId("unselected");
  expect(unselected.textContent).toBe("2001-02-03T23:44:25.5Z");
  expect(unselected.querySelector("[title]")).toBeNull();
  expect(screen.getByTestId("local").textContent).toBe("タイムゾーン不明");
});
