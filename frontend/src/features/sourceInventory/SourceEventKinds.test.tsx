// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { everyMatchCondition } from "@/shared/api/matchConditions";
import type { SourceIdentity } from "@/shared/contracts/sources";
import { jsonResponse } from "@/testdata/http";
import { SourceEventKinds } from "./SourceEventKinds";

const matchConditions = everyMatchCondition();

function source(formatKey: string): SourceIdentity {
  return {
    sourceId: "c-proxy",
    contentSha256: "c".repeat(64),
    originPath: "/data/example/access.log",
    fileName: "access.log",
    sizeBytes: 1000,
    newlineCount: 10,
    endsWithNewline: true,
    lineEnding: "lf",
    formatKey,
  };
}

function stubFetch(body: unknown) {
  const mock = vi.fn(async (_input: string) => jsonResponse(200, body));
  vi.stubGlobal("fetch", mock);
  return mock;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

test("選んだ収集元の種別と件数、取り込めなかった件数、0 件の種別の読み方、入力形式の説明を出す", async () => {
  const fetch = stubFetch({
    kinds: [{ category: "access", action: "GET", recordCount: 7 }],
    uncategorizedRecordCount: 2,
    sourceId: "c-proxy",
  });
  render(
    <SourceEventKinds
      source={source("squid_combined")}
      failedRecordCount={3}
      matchConditions={matchConditions}
      dataVersion={0}
    />,
  );
  const table = await screen.findByRole("table");
  expect(within(table).getByText("GET")).toBeTruthy();
  expect(within(table).getByText("7")).toBeTruthy();
  const counts = screen
    .getAllByRole("listitem")
    .map((item) => item.textContent);
  expect(counts).toEqual(
    expect.arrayContaining(["種類なし: 2", "取り込めなかったレコード: 3"]),
  );
  fireEvent.click(
    screen.getByRole("button", { name: "イベントの種類 の説明" }),
  );
  const help = screen.getByRole("tooltip").textContent;
  expect(help).toContain(
    "判定しないこと: 記録の設定の外か、イベントが起きなかったか",
  );
  expect(help).toContain(
    "入力形式の記録の範囲記録する要求: Proxy を経由した要求だけ",
  );
  const url = new URL(String(fetch.mock.calls[0]?.[0]), "http://localhost");
  expect(url.searchParams.get("sourceId")).toBe("c-proxy");
  expect(url.searchParams.get("sourceContentSha256")).toBe("c".repeat(64));
});

test("応答の収集元が要求と違うときは、別の収集元の種別を出さない", async () => {
  stubFetch({
    kinds: [{ category: "access", action: "GET", recordCount: 7 }],
    uncategorizedRecordCount: 0,
    sourceId: "d-other",
  });
  render(
    <SourceEventKinds
      source={source("infotrace_mark_ii")}
      failedRecordCount={undefined}
      matchConditions={matchConditions}
      dataVersion={0}
    />,
  );
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(screen.queryByRole("table")).toBeNull();
  fireEvent.click(
    screen.getByRole("button", { name: "イベントの種類 の説明" }),
  );
  expect(screen.getByRole("tooltip").textContent).not.toMatch(
    /入力形式の記録の範囲/,
  );
});
