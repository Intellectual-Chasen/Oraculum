// @vitest-environment jsdom
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { everyMatchCondition } from "@/shared/api/matchConditions";
import type { SourceIdentity } from "@/shared/contracts/sources";
import { jsonResponse } from "@/testdata/http";
import { SourceEventKinds } from "./SourceEventKinds";

function source(formatKey: string): SourceIdentity {
  return {
    sourceId: "c-proxy",
    contentSha256: "c".repeat(64),
    originPath: "/data/example/proxy.log",
    fileName: "proxy.log",
    sizeBytes: 1000,
    newlineCount: 10,
    endsWithNewline: true,
    lineEnding: "lf",
    formatKey,
  };
}

const eventKinds = {
  kinds: [],
  uncategorizedRecordCount: 0,
  sourceId: "c-proxy",
};

function stubFetch(bypass: unknown) {
  const mock = vi.fn(async (input: string) =>
    jsonResponse(
      200,
      String(input).includes("/proxy-bypass") ? bypass : eventKinds,
    ),
  );
  vi.stubGlobal("fetch", mock);
  return mock;
}

function renderSource(formatKey: string) {
  render(
    <SourceEventKinds
      source={source(formatKey)}
      failedRecordCount={0}
      matchConditions={everyMatchCondition()}
      dataVersion={0}
    />,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

test("Proxy のログの収集元は、接続元ごとに Proxy を経由した要求と経由しない接続を件数で並べる", async () => {
  const fetch = stubFetch({
    sourceId: "c-proxy",
    proxyAddresses: ["192.0.2.80"],
    unreadableProxyRecordCount: 0,
    clients: [
      {
        clientIp: "192.0.2.10",
        terminals: ["host-a"],
        proxyRequestCount: 12,
        proxyConnectionCount: 10,
        directConnectionCount: 5,
        directConnectionCountable: true,
        directDestinations: [
          { address: "203.0.113.50", port: "80", recordCount: 4 },
          { address: "203.0.113.51", recordCount: 1 },
        ],
      },
    ],
  });
  renderSource("squid_combined");

  const table = await screen.findByRole("table", {
    name: "接続元ごとの Proxy の経由",
  });
  const row = within(table).getAllByRole("row")[1];
  expect(
    within(row)
      .getAllByRole("cell")
      .map((cell) => cell.textContent),
  ).toEqual([
    "192.0.2.10",
    "host-a",
    "12",
    "10",
    "5",
    "203.0.113.50:80: 4203.0.113.51: 1",
  ]);
  expect(
    screen.getAllByRole("listitem").map((item) => item.textContent),
  ).toContain("Proxy のアドレス: 192.0.2.80");
  const url = new URL(
    String(
      fetch.mock.calls.find(([input]) =>
        String(input).includes("/proxy-bypass"),
      )?.[0],
    ),
    "http://localhost",
  );
  expect(url.searchParams.get("sourceId")).toBe("c-proxy");
  expect(url.searchParams.get("sourceContentSha256")).toBe("c".repeat(64));
});

test("Proxy のアドレスが無い収集元は、経由しない接続を分けられないことを出す", async () => {
  stubFetch({
    sourceId: "c-proxy",
    proxyAddresses: [],
    unreadableProxyRecordCount: 0,
    clients: [
      {
        clientIp: "192.0.2.10",
        terminals: [],
        proxyRequestCount: 12,
        proxyConnectionCount: 0,
        directConnectionCount: 0,
        directConnectionCountable: false,
        directDestinations: [],
      },
    ],
  });
  renderSource("squid_combined");

  const table = await screen.findByRole("table", {
    name: "接続元ごとの Proxy の経由",
  });
  const cells = within(within(table).getAllByRole("row")[1])
    .getAllByRole("cell")
    .map((cell) => cell.textContent ?? "");
  expect(cells[2]).toBe("12");
  for (const cell of cells.slice(3)) {
    expect(cell).not.toBe("0");
    expect(cell).toMatch(/Proxy のアドレスなし/);
  }
  expect(
    screen.getAllByRole("listitem").map((item) => item.textContent),
  ).toContain("Proxy のアドレス: —端末の IP なし");
});

test("接続元を読めない Proxy のレコードの件数を出す", async () => {
  stubFetch({
    sourceId: "c-proxy",
    proxyAddresses: ["192.0.2.80"],
    unreadableProxyRecordCount: 3,
    clients: [],
  });
  renderSource("squid_combined");

  await screen.findByRole("table", { name: "接続元ごとの Proxy の経由" });
  expect(
    screen.getAllByRole("listitem").map((item) => item.textContent),
  ).toContain("接続元が IP でないレコード: 3");
});

test("Proxy のログでない収集元は、比較を要求しない", async () => {
  const fetch = stubFetch({});
  renderSource("windows_event_xml");

  await screen.findByText("イベントの種類を持つレコードなし");
  expect(
    fetch.mock.calls.some(([input]) => String(input).includes("/proxy-bypass")),
  ).toBe(false);
});

test("接続を記録する収集元が無い接続元は、経由しない接続を 0 件と出さず数えられないと出す", async () => {
  stubFetch({
    sourceId: "c-proxy",
    proxyAddresses: ["192.0.2.80"],
    unreadableProxyRecordCount: 0,
    clients: [
      {
        clientIp: "192.0.2.20",
        terminals: [],
        proxyRequestCount: 3,
        proxyConnectionCount: 0,
        directConnectionCount: 0,
        directConnectionCountable: false,
        directDestinations: [],
      },
    ],
  });
  renderSource("squid_combined");

  const table = await screen.findByRole("table", {
    name: "接続元ごとの Proxy の経由",
  });
  const cells = within(within(table).getAllByRole("row")[1])
    .getAllByRole("cell")
    .map((cell) => cell.textContent ?? "");
  expect(cells[2]).toBe("3");
  for (const cell of cells.slice(3)) {
    expect(cell).not.toBe("0");
    expect(cell).toMatch(/接続を記録する収集元なし/);
  }
});
