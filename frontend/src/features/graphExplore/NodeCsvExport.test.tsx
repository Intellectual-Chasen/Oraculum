// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { everyMatchCondition } from "@/shared/api/matchConditions";
import {
  graphResponseJson,
  summarizedRecordGraphResponseJson,
} from "@/testdata/graph/graphResponse";
import { jsonResponse } from "@/testdata/http";
import { NodeCsvExport } from "./NodeCsvExport";
import type { SubgraphCriteria } from "./useSubgraph";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const criteria: SubgraphCriteria = {
  matchConditions: everyMatchCondition(),
  nodeLimit: 200,
  depth: 1,
  valueContains: ["example"],
  valueField: "CommandLine",
};

test("描画の上限を外し、ホップ数 0 で同じ条件を要求して、合致したノードだけを書き出す", async () => {
  const fetch = vi.fn(async (_input: string) =>
    jsonResponse(200, graphResponseJson()),
  );
  vi.stubGlobal("fetch", fetch);
  const blobs: Blob[] = [];
  vi.spyOn(URL, "createObjectURL").mockImplementation((blob) => {
    blobs.push(blob as Blob);
    return "blob:example";
  });
  vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});

  render(<NodeCsvExport criteria={criteria} />);
  expect(fetch).not.toHaveBeenCalled();
  fireEvent.click(
    screen.getByRole("button", { name: "一致ノードを CSV に書き出す" }),
  );
  const status = await screen.findByRole("status");
  const url = new URL(String(fetch.mock.calls[0]?.[0]), "http://localhost");
  expect(url.searchParams.has("nodeLimit")).toBe(false);
  expect(url.searchParams.get("depth")).toBe("0");
  expect(url.searchParams.get("valueField")).toBe("CommandLine");
  expect(url.searchParams.get("recordSummary")).toBe("true");
  // fixture の 3 ノードのうち、関係の相手として出た 1 つは書かない。
  expect(status).toHaveTextContent("書き出したノード: 2");
  const text = await blobs[0]?.text();
  expect(text?.split("\r\n").filter((line) => line !== "")).toHaveLength(3);
});

test("レコードのノードの行に、時刻の文字列と正規化値、イベント ID、チャネル、Provider、収集元、位置を書く", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse(200, summarizedRecordGraphResponseJson())),
  );
  const blobs: Blob[] = [];
  vi.spyOn(URL, "createObjectURL").mockImplementation((blob) => {
    blobs.push(blob as Blob);
    return "blob:example";
  });
  vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});

  render(<NodeCsvExport criteria={criteria} />);
  fireEvent.click(
    screen.getByRole("button", { name: "一致ノードを CSV に書き出す" }),
  );
  await screen.findByRole("status");
  const lines = (await blobs[0]?.text())?.split("\r\n") ?? [];
  // 値はどれも `"` を含まないため、欄の区切りは `","` である。
  const cellsOf = (line: string | undefined) =>
    line?.replace("﻿", "").slice(1, -1).split('","') ?? [];
  const header = cellsOf(lines[0]);
  const cells = cellsOf(
    lines.find((line) => line.startsWith('"host-a.log ID 112"')),
  );
  const cellOf = (name: string) => cells[header.indexOf(name)];
  expect(cellOf("時刻の原文")).toBe("2031/10/08 10:20:35.100");
  expect(cellOf("時刻の正規化した値")).toBe("2031-10-08T10:20:35.100+09:00");
  expect(cellOf("イベント ID かイベントの動作")).toBe("8001");
  expect(cellOf("チャネル")).toBe("Example-Channel");
  expect(cellOf("Provider かイベントの分類")).toBe("Example-Provider");
  expect(cellOf("収集元")).toBe("host-a.log");
  expect(cellOf("収集元の sourceId")?.length).toBeGreaterThan(0);
  expect(cellOf("位置")).toBe("ID: 112、行: 1022");
  expect(cellOf("RecordHeader.RecordID")).toBe("5112");
  expect(cellOf("EventRecordID")).toBe("4112");
});
