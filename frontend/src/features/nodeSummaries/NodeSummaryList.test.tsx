// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { NodeSummariesRequest } from "@/shared/api/nodeSummaries";
import { DisplayOffsetContext } from "@/shared/ui/DisplayOffset";
import { ValueActionsContext } from "@/shared/ui/ValueLink";
import { jsonResponse } from "@/testdata/http";
import {
  timelineResponseJson,
  timelineTerminalNodeId,
} from "@/testdata/timeline/timelineResponse";
import { ValueActionsForTest } from "@/testdata/valueActions";
import { NodeSummaryList } from "./NodeSummaryList";

test("表示のずれを選ぶと、ノードの時刻の範囲に地方時を添える", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse(200, summariesJson())),
  );
  render(
    <DisplayOffsetContext.Provider value="+00:00">
      <NodeSummaryList
        request={request}
        version={0}
        selectedNodeId={undefined}
      />
    </DisplayOffsetContext.Provider>,
  );

  await screen.findByRole("table");
  expect(screen.getByRole("table").textContent).toContain(
    "UTC+00:00: 2031-10-08T01:20:35.100+00:00",
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function summariesJson() {
  const [first, second] = timelineResponseJson().entries;
  return {
    nodes: [
      {
        node: first?.terminal,
        recordCount: 5,
        firstTime: first?.eventTime,
        lastTime: second?.eventTime,
        localTimeRecordCount: 2,
        undatedRecordCount: 1,
      },
      {
        node: {
          ...first?.terminal,
          id: "n:terminal:2",
          label: { rawText: "HOST-D", valueState: "present" },
        },
        recordCount: 1,
        localTimeRecordCount: 0,
        undatedRecordCount: 1,
      },
    ],
    nodeCount: 2,
    nodeKind: "terminal",
  };
}

const request: NodeSummariesRequest = {
  nodeKind: "terminal",
  matchConditions: { conditions: [{ conditionKey: "destination_ip" }] },
  sources: ["s1"],
};

test("端末を件数と時刻の範囲とともに並べ、選んだ行を示し、ノードの値を押すとそのノードを開く", async () => {
  const mock = vi.fn(async (_input: string) =>
    jsonResponse(200, summariesJson()),
  );
  vi.stubGlobal("fetch", mock);
  const open = vi.fn();
  render(
    <ValueActionsContext value={{ open, addCondition: vi.fn() }}>
      <NodeSummaryList
        request={request}
        version={0}
        selectedNodeId={timelineTerminalNodeId}
      />
    </ValueActionsContext>,
  );

  const table = await screen.findByRole("table");
  const params = new URL(mock.mock.calls[0]?.[0] ?? "", "http://localhost")
    .searchParams;
  expect(params.get("nodeKind")).toBe("terminal");
  expect(params.getAll("source")).toEqual(["s1"]);

  const rows = within(table).getAllByRole("row").slice(1);
  expect(rows[0]?.getAttribute("aria-current")).toBe("true");
  expect(rows[0]?.textContent).toContain("2031/10/08 10:20:35.100");
  expect(rows[0]?.textContent).toContain("2031/10/08 10:20:36.200");
  // 時刻の範囲の外のレコードを、タイムゾーン未定と時刻なしに分けて件数に添える。
  expect(rows[0]?.textContent).toContain("範囲外・タイムゾーン未定: 2");
  expect(rows[0]?.textContent).toContain("範囲外・時刻なし: 1");
  expect(rows[0]?.textContent).toContain("精度:");
  expect(rows[1]?.getAttribute("aria-current")).toBeNull();
  // UTC 時刻を持つレコードが無いノードは、最初と最後の時刻の代わりにそのことを出す。
  expect(rows[1]?.textContent).toContain("UTC 時刻を持つレコードなし");
  expect(rows[1]?.textContent).not.toContain("タイムゾーン未定");
  expect(rows[1]?.textContent).toContain("範囲外・時刻なし: 1");

  fireEvent.click(within(table).getByRole("button", { name: "HOST-D" }));
  expect(open).toHaveBeenCalledWith({
    kind: "node",
    id: "n:terminal:2",
    label: "HOST-D",
  });
});

test("時刻のセルは時刻を 1 行で出し、精度とタイムゾーンを tooltip に入れる", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse(200, summariesJson())),
  );
  render(
    <DisplayOffsetContext.Provider value="+00:00">
      <NodeSummaryList
        request={request}
        version={0}
        selectedNodeId={undefined}
      />
    </DisplayOffsetContext.Provider>,
  );
  const table = await screen.findByRole("table");
  const firstRow = within(table).getAllByRole("row")[1] as HTMLElement;
  for (const header of ["最初の時刻", "最後の時刻"]) {
    const column = within(table)
      .getAllByRole("columnheader")
      .findIndex((cell) => cell.textContent === header);
    const cell = firstRow.children[column] as HTMLElement;
    expect(cell.querySelectorAll("li, [style*='block']")).toHaveLength(0);
    const title = cell.querySelector("[title]")?.getAttribute("title") ?? "";
    expect(title).toContain("精度: ");
    expect(title).toContain("UTC+00:00: ");
  }
});

test("押せる時刻のセルは、時刻の補足を押す操作の tooltip だけに出し、title の tooltip を重ねない", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse(200, summariesJson())),
  );
  render(
    <ValueActionsForTest>
      <DisplayOffsetContext.Provider value="+00:00">
        <NodeSummaryList
          request={request}
          version={0}
          selectedNodeId={undefined}
        />
      </DisplayOffsetContext.Provider>
    </ValueActionsForTest>,
  );
  const table = await screen.findByRole("table");
  const firstRow = within(table).getAllByRole("row")[1] as HTMLElement;
  const column = within(table)
    .getAllByRole("columnheader")
    .findIndex((cell) => cell.textContent === "最初の時刻");
  const cell = firstRow.children[column] as HTMLElement;
  expect(cell.querySelector("[title]")).toBeNull();
  expect(within(cell).getByRole("button").textContent).toContain("精度: ");
});

test("IP アドレスの一覧は、端末の範囲のアドレスの行に範囲の端末を出し、範囲を持たない行には理由を出す", async () => {
  const terminal = timelineResponseJson().entries[0]?.terminal;
  const loopback = (id: string, terminalLabel: string) => ({
    node: {
      ...terminal,
      id: `n:ip:${id}`,
      kind: "ip",
      keyForm: "terminal_id_address",
      identity: [{ value: terminalLabel }, { value: "::1" }],
      label: { rawText: "::1", valueState: "present" },
    },
    terminal: {
      ...terminal,
      id: `n:terminal:${id}`,
      label: { rawText: terminalLabel, valueState: "present" },
    },
    recordCount: 3,
    localTimeRecordCount: 0,
    undatedRecordCount: 3,
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      jsonResponse(200, {
        nodes: [
          loopback("a", "host-a.example.test"),
          loopback("b", "host-b.example.test"),
          {
            node: {
              ...terminal,
              id: "n:ip:c",
              kind: "ip",
              keyForm: "address",
              identity: [{ value: "192.0.2.1" }],
              label: { rawText: "192.0.2.1", valueState: "present" },
            },
            recordCount: 1,
            localTimeRecordCount: 0,
            undatedRecordCount: 1,
          },
        ],
        nodeCount: 3,
        nodeKind: "ip",
      }),
    ),
  );
  render(
    <NodeSummaryList
      request={{ ...request, nodeKind: "ip" }}
      version={0}
      selectedNodeId={undefined}
    />,
  );
  const table = await screen.findByRole("table");
  expect(
    within(table).getByRole("columnheader", { name: "端末" }),
  ).toBeTruthy();
  const terminalCells = within(table)
    .getAllByRole("row")
    .slice(1)
    // ノードの列は行の見出しであり、端末の列が最初の cell である。
    .map((row) => within(row).getAllByRole("cell")[0]?.textContent);
  expect(terminalCells).toEqual([
    "host-a.example.test",
    "host-b.example.test",
    "—端末の範囲なし",
  ]);
});

test("0 件の応答は、端末の件数 0 を知らせる", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      jsonResponse(200, { nodes: [], nodeCount: 0, nodeKind: "terminal" }),
    ),
  );
  render(
    <NodeSummaryList
      request={request}
      version={0}
      selectedNodeId={undefined}
    />,
  );
  expect((await screen.findByText("0")).closest("li")?.textContent).toBe(
    "端末: 0",
  );
  expect(screen.queryByRole("table")).toBeNull();
});
