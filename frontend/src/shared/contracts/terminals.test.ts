import { expect, test } from "vitest";
import {
  terminalDetailJson,
  terminalEventJson,
  terminalsJson,
} from "@/testdata/terminals/terminalsResponse";
import { DecodeFailure } from "./decoding";
import {
  decodeTerminalDetail,
  decodeTerminalEventsResponse,
  decodeTerminalsResponse,
} from "./terminals";

test("端末の一覧・詳細・レコードの応答を読む", () => {
  const list = decodeTerminalsResponse(terminalsJson(), "response");
  expect(list.terminals[0]?.names).toEqual(["HOST-C", "HOST-C2"]);

  const detail = decodeTerminalDetail(terminalDetailJson(), "response");
  expect(detail.categories.map((count) => count.category)).toHaveLength(7);
  expect(detail.remoteLogons[1]?.firstRecordRef).toBeDefined();
  expect(detail.remoteLogons[0]?.firstRecordRef).toBeUndefined();

  const events = decodeTerminalEventsResponse(
    {
      terminalId: "t",
      category: "program_execution",
      events: [terminalEventJson("a.exe", 1)],
    },
    "response",
  );
  expect(events.category).toBe("program_execution");
  expect(events.events[0]?.fields[0]?.semantic).toBe("file.path");
});

test("契約に無い分類と欠けた項目を退ける", () => {
  const detail = terminalDetailJson();
  expect(() =>
    decodeTerminalDetail(
      {
        ...detail,
        categories: [{ ...detail.categories[0], category: "other" }],
      },
      "response",
    ),
  ).toThrow(DecodeFailure);
  const { originalFileNameDiffers: _, ...withoutFlag } = terminalEventJson(
    "a.exe",
    1,
  );
  expect(() =>
    decodeTerminalEventsResponse(
      { terminalId: "t", events: [withoutFlag] },
      "response",
    ),
  ).toThrow(DecodeFailure);
});
