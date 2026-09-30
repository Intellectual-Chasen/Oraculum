// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { jsonResponse } from "@/testdata/http";
import {
  terminalDetailJson,
  terminalEventJson,
  terminalId,
  terminalNodeJson,
  terminalsJson,
} from "@/testdata/terminals/terminalsResponse";
import { offsetFromActiveTimeBias, TerminalDetail } from "./TerminalDetail";
import { TerminalList } from "./TerminalList";

const matchConditions = { conditions: [] };

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function textFieldJson(name: string, semantic: string, rawText: string) {
  return {
    name,
    semantic,
    kind: "text",
    text: { rawText, valueState: "present" },
  };
}

/** 欄の文字列を含むレコードの行を返す。欄の一覧の項目の外側の項目である。 */
function eventRowOf(text: RegExp): HTMLElement {
  const field = screen.getByText(text).closest("li");
  return field?.parentElement?.closest("li") as HTMLElement;
}

function urlOf(input: unknown): URL {
  return new URL(String(input), "http://localhost");
}

/** path ごとに応答を返す fetch の mock。 */
function stubApi() {
  const mock = vi.fn(async (input: unknown) => {
    const url = urlOf(input);
    if (url.pathname.endsWith("/events")) {
      const sourceIp = url.searchParams.get("sourceIp");
      if (sourceIp !== null) {
        return jsonResponse(200, {
          terminalId,
          sourceIp,
          events: [
            {
              ...terminalEventJson("logon-source.exe", 9),
              categories: ["remote_logon"],
              logonOutcome: "failure",
              fields: [
                ...terminalEventJson("logon-source.exe", 9).fields,
                textFieldJson("EventID", "windows_event.id", "4625"),
              ],
            },
          ],
        });
      }
      const first = terminalEventJson("C:\\first.exe", 1);
      return jsonResponse(200, {
        terminalId,
        category: url.searchParams.get("category"),
        events: [
          {
            ...first,
            categories: ["installation"],
            fields: [
              ...first.fields,
              textFieldJson("Data", "event.message", "Product: Example Tool"),
              textFieldJson("Data#2", "file.product_version", "9.8.7"),
            ],
          },
          terminalEventJson("C:\\renamed.exe", 2),
        ],
      });
    }
    if (url.pathname.endsWith("/terminals")) {
      return jsonResponse(200, terminalsJson());
    }
    return jsonResponse(200, terminalDetailJson());
  });
  vi.stubGlobal("fetch", mock);
  return mock;
}

function renderDetail() {
  const handlers = {
    onSelectRecord: vi.fn(),
    onSelectNode: vi.fn(),
    onApplyDisplayOffset: vi.fn(),
  };
  render(
    <TerminalDetail
      terminalId={terminalId}
      matchConditions={matchConditions}
      version={0}
      {...handlers}
    />,
  );
  return handlers;
}

test("端末の一覧は名前・OS・件数を並べ、行を選ぶと端末を渡す", async () => {
  stubApi();
  const onSelectTerminal = vi.fn();
  render(
    <TerminalList
      matchConditions={matchConditions}
      version={0}
      selectedTerminalId={undefined}
      onSelectTerminal={onSelectTerminal}
      sourceFileNames={new Map()}
    />,
  );
  const table = await screen.findByRole("table", { name: "端末" });
  const row = within(table)
    .getAllByRole("row")
    .find((item) => item.textContent?.includes("HOST-C"));
  const button = within(row as HTMLElement).getAllByRole("button")[0];
  if (button === undefined) throw new Error("端末の行に button が無い");
  expect(row?.textContent).toContain("Example OS 10 1234");
  expect(row?.textContent).toContain("5");
  fireEvent.click(button);
  expect(onSelectTerminal.mock.calls[0]?.[0].id).toBe(terminalId);
});

/** 収集元の内容の識別とホスト名で識別した端末の一覧の行。 */
function recordingTerminalRow(sha: string, hostname: string) {
  return {
    node: {
      ...terminalNodeJson,
      id: `n:terminal:${sha.slice(0, 4)}:${hostname}`,
      keyForm: "recording_source_content_sha256_hostname",
      identity: [
        { value: sha },
        { semantic: "terminal.hostname", value: hostname },
      ],
      label: { rawText: hostname, valueState: "present" },
    },
    names: [hostname],
    recordCount: 1,
    categorizedRecordCount: 1,
  };
}

test("端末の行に収集元を出し、同じ名前の端末が 2 行以上あれば名前と行数の表を出し、割り当てでまとめられることを help に置く", async () => {
  const shaA = "a".repeat(64);
  const shaB = "b".repeat(64);
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      jsonResponse(200, {
        terminals: [
          recordingTerminalRow(shaA, "pc-01.example.test"),
          recordingTerminalRow(shaB, "PC-01"),
          recordingTerminalRow(shaB, "pc-02"),
          // IP アドレスの表示名は、最初の `.` で切って組にしない。
          recordingTerminalRow(shaA, "192.0.2.10"),
          recordingTerminalRow(shaB, "192.0.2.11"),
          {
            ...recordingTerminalRow(shaA, "collected"),
            node: {
              ...recordingTerminalRow(shaA, "collected").node,
              id: "n:terminal:collection",
              keyForm: "collection_content_sha256",
              identity: [{ value: "c".repeat(64) }],
            },
          },
          {
            ...recordingTerminalRow(shaA, "HOST-T"),
            node: {
              ...recordingTerminalRow(shaA, "HOST-T").node,
              id: "n:terminal:id",
              keyForm: "terminal_id",
              identity: [{ semantic: "terminal.id", value: "T-1" }],
            },
          },
        ],
      }),
    ),
  );
  render(
    <TerminalList
      matchConditions={matchConditions}
      version={0}
      selectedTerminalId={undefined}
      onSelectTerminal={() => {}}
      sourceFileNames={
        new Map([
          [shaA, "a.evtx"],
          [shaB, "b.evtx"],
        ])
      }
    />,
  );

  const table = await screen.findByRole("table", { name: "端末" });
  const rowOf = (name: string) =>
    within(table).getByRole("button", { name }).closest("tr") as HTMLElement;
  expect(rowOf("pc-01.example.test").textContent).toContain("a.evtx");
  expect(rowOf("PC-01").textContent).toContain("b.evtx");
  // OS は registry の収集元から読み取る。
  expect(rowOf("pc-02").textContent).toContain(
    "OS 情報を読み取る registry の収集元なし",
  );
  // 収集の directory の端末は収集の単位を、端末の識別子の端末はそのことを出す。
  expect(rowOf("collected").textContent).toContain("収集の directory");
  expect(rowOf("HOST-T").textContent).toContain("端末 ID で識別");
  // FQDN と短い名前、大文字と小文字の違いも同じ名前に数える。IP アドレスは組にしない。
  const sameNames = screen.getByRole("table", { name: "同じ名前の端末" });
  expect(
    within(sameNames)
      .getAllByRole("row")
      .slice(1)
      .map((item) => item.textContent),
  ).toEqual(["pc-01.example.test2"]);
  // 「?」は表の見出しの隣に置く。
  expect(
    screen
      .getByRole("button", { name: "同じ名前の端末 の説明" })
      .closest(".help-popover")?.parentElement?.textContent,
  ).toBe("同じ名前の端末");
  // 割り当てでまとめられることと、図の端末の数と違う理由は、help で出す。
  fireEvent.click(
    screen.getByRole("button", { name: "同じ名前の端末 の説明" }),
  );
  expect(screen.getByText(/端末の割り当て/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Hosts の端末 の説明" }));
  expect(
    screen.getByText(/分類に入るレコードか registry の端末の情報を持つ端末/),
  ).toBeTruthy();
});

test("端末の一覧の取得に失敗したことを出す", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse(500, {})),
  );
  render(
    <TerminalList
      matchConditions={matchConditions}
      version={0}
      selectedTerminalId={undefined}
      onSelectTerminal={vi.fn()}
      sourceFileNames={new Map()}
    />,
  );
  expect(await screen.findByText("端末の一覧の取得")).toBeTruthy();
});

test("端末を選んでいない間は「端末: 未選択」と、Hosts を表示する button を出す", () => {
  const onOpenHosts = vi.fn();
  render(
    <TerminalDetail
      terminalId={undefined}
      matchConditions={matchConditions}
      version={0}
      onSelectRecord={vi.fn()}
      onSelectNode={vi.fn()}
      onApplyDisplayOffset={vi.fn()}
      onOpenHosts={onOpenHosts}
    />,
  );
  expect(screen.getByRole("status").textContent).toBe("端末: 未選択");
  fireEvent.click(screen.getByRole("button", { name: "Hosts を表示" }));
  expect(onOpenHosts).toHaveBeenCalledTimes(1);
});

test("端末の情報の値からレコードを開き、タイムゾーンを表示のタイムゾーンへ渡す", async () => {
  stubApi();
  const handlers = renderDetail();
  await screen.findByText("端末の情報");
  expect(screen.getByText(/192\.0\.2\.10/)).toBeTruthy();
  fireEvent.click(
    screen.getAllByRole("button", { name: "レコードを開く" })[0] as Element,
  );
  expect(handlers.onSelectRecord).toHaveBeenCalledTimes(1);

  expect(
    screen
      .getAllByRole("listitem")
      .some(
        (item) =>
          item.textContent === "ActiveTimeBias のタイムゾーン: UTC+05:45",
      ),
  ).toBe(true);
  fireEvent.click(
    screen.getByRole("button", { name: "表示のタイムゾーンに適用" }),
  );
  expect(handlers.onApplyDisplayOffset).toHaveBeenCalledWith("+05:45");
});

test("値を持たないインターフェースの期間も、値の列を値なしにした 1 行で IP アドレスの表に出す", async () => {
  const detail = terminalDetailJson();
  const [address] = detail.addresses;
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      jsonResponse(200, {
        ...detail,
        addresses: [
          ...detail.addresses,
          { ...address, interface: "{0001-synthetic}", values: [] },
        ],
      }),
    ),
  );
  renderDetail();
  const table = await screen.findByRole("table", { name: "IP アドレス" });
  const row = within(table)
    .getByText("{0001-synthetic}")
    .closest("tr") as HTMLElement;
  expect(row.textContent).toContain("値なし");
});

test("ActiveTimeBias の符号なしの値を符号付きのずれへ直す", () => {
  expect(offsetFromActiveTimeBias("4294966951 (0xfffffea7)")).toBe("+05:45");
  expect(offsetFromActiveTimeBias("300 (0x12c)")).toBe("-05:00");
  expect(offsetFromActiveTimeBias("0 (0x0)")).toBe("+00:00");
  expect(offsetFromActiveTimeBias("unreadable")).toBeUndefined();
});

test("遠隔のログオンを失敗の多い順に並べ、並びを変え、件数から接続元のレコードを出す", async () => {
  const mock = stubApi();
  renderDetail();
  const table = await screen.findByRole("table", { name: "接続元: 2" });
  const firstIp = () =>
    within(table).getAllByRole("row")[1]?.querySelector("th")?.textContent;
  expect(firstIp()).toBe("198.51.100.2");

  fireEvent.click(within(table).getByRole("button", { name: "成功" }));
  expect(firstIp()).toBe("198.51.100.1");

  // 件数の button の読み上げの名前は、接続元と何の件数かを含む。
  fireEvent.click(
    within(table).getByRole("button", { name: "198.51.100.1 の失敗: 1" }),
  );
  expect(await screen.findByText(/logon-source\.exe/)).toBeTruthy();
  const logonRow = eventRowOf(/logon-source\.exe/);
  expect(logonRow.textContent).toContain("4625");
  expect(logonRow.textContent).toContain("結果: 失敗");
  const eventsCall = mock.mock.calls
    .map((call) => urlOf(call[0]))
    .find((url) => url.searchParams.get("sourceIp") !== null);
  expect(eventsCall?.searchParams.get("sourceIp")).toBe("198.51.100.1");
});

test("レコードの無い分類は、要る収集元の有無を出す", async () => {
  stubApi();
  renderDetail();
  await screen.findByText("分類ごとのレコード");
  const pairs = () =>
    screen.getAllByRole("listitem").map((item) => item.textContent ?? "");
  await vi.waitFor(() =>
    expect(pairs()).toContain("PowerShell のスクリプト: 0"),
  );
  expect(pairs().some((text) => text.startsWith("必要な収集元: なし"))).toBe(
    true,
  );
  expect(pairs()).toContain("インストール: 0");
  expect(pairs().some((text) => text.startsWith("収集元: "))).toBe(true);
});

test("分類のレコードをすべて出し、行からレコードとノードを開く", async () => {
  const mock = stubApi();
  const handlers = renderDetail();
  expect(await screen.findByText(/C:\\first\.exe/)).toBeTruthy();
  expect(screen.queryByText(/hash-not-shown/)).toBeNull();
  const installRow = eventRowOf(/C:\\first\.exe/);
  expect(installRow.textContent).toContain("Product: Example Tool");
  expect(installRow.textContent).toContain("9.8.7");
  expect(
    screen
      .getAllByRole("listitem")
      .some((item) => item.textContent === "レコード: 2"),
  ).toBe(true);
  expect(screen.getAllByText(/ほかの実行時刻/)).toHaveLength(2);
  expect(screen.getAllByText(/C:\\renamed\.exe/)).toHaveLength(1);
  expect(screen.getByText(/元のファイル名と不一致/)).toBeTruthy();
  expect(
    mock.mock.calls
      .map((call) => urlOf(call[0]))
      .filter((url) => url.pathname.endsWith("/events"))
      .every((url) => !url.searchParams.has("cursor")),
  ).toBe(true);

  fireEvent.click(
    screen.getAllByRole("button", { name: "user-z" })[0] as Element,
  );
  expect(handlers.onSelectNode).toHaveBeenCalledWith({
    id: "n:account:synthetic",
    label: "user-z",
  });
});
