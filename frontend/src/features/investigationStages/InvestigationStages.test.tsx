// @vitest-environment jsdom
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { SignedInContext } from "@/shared/ui/SignedInAccount";
import { jsonResponse } from "@/testdata/http";
import { signedInViewer } from "@/testdata/session";
import {
  baseListingJson,
  logsListingJson,
  proxyListingJson,
  proxyRecursiveListingJson,
  sourceFilesRequest,
  unknownFilePath,
} from "@/testdata/stages/sourceFilesResponse";
import {
  hostLogPath,
  loadingCompletedStagesJson,
  loadingFailedStagesJson,
  loadingRunningStagesJson,
  notStartedStagesJson,
  processingFailedStagesJson,
  processingRunningStagesJson,
  proxyLogPath,
  rejectingRequestedLoadingStagesJson,
} from "@/testdata/stages/stagesResponse";
import { InvestigationStages } from "./InvestigationStages";
import { sourceDragType } from "./SourceBrowser";
import {
  stagesPollIntervalMs,
  useInvestigationStages,
} from "./useInvestigationStages";

/** 上位の画面と同じく、段階の状態を `useInvestigationStages` に任せて出す。 */
function StagesUnderTest() {
  return <InvestigationStages view={useInvestigationStages()} />;
}

/** 要求への応答。`Error` は通信の失敗として投げる。 */
type Reply = Response | Error;

/**
 * 要求の method と path ごとに、応答を並びの順で返す。並びの最後の応答は、それ以降の
 * 要求にも返す。
 */
function stubFetch(replies: Record<string, Reply[]>) {
  const served = new Map<string, number>();
  const mock = vi.fn(async (input: string, init?: RequestInit) => {
    const key = `${init?.method ?? "GET"} ${input}`;
    const queue = replies[key] ?? [];
    const index = served.get(key) ?? 0;
    served.set(key, index + 1);
    const reply = queue[Math.min(index, queue.length - 1)];
    if (reply === undefined) {
      throw new Error(`unexpected request: ${key}`);
    }
    if (reply instanceof Error) {
      throw reply;
    }
    return reply.clone();
  });
  vi.stubGlobal("fetch", mock);
  return mock;
}

const getStages = "GET /api/v0/stages";

async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

async function renderStages(replies: Record<string, Reply[]>) {
  const mock = stubFetch(replies);
  render(<StagesUnderTest />);
  await advance(0);
  return mock;
}

function stageSection(name: string): HTMLElement {
  return screen.getByRole("region", { name });
}

/** 段階の節の「状態: 値」の組の文字列。 */
function stateOf(section: HTMLElement): string | undefined {
  return Array.from(section.querySelectorAll("li"))
    .map((item) => item.textContent ?? "")
    .find((text) => text.startsWith("状態: "));
}

/** button が押せないか。理由を持つ button は aria-disabled で、持たない button は disabled で止める。 */
function isBlocked(button: HTMLElement): boolean {
  return (
    button.getAttribute("aria-disabled") === "true" ||
    (button as HTMLButtonElement).disabled
  );
}

/** 収集元の行の各列を、列の見出しから探せる形にする。 */
function rowValues(originPath: string): Map<string, string> {
  const table = screen.getByRole("table", {
    name: "収集元の読み込み",
  });
  const headers = within(table)
    .getAllByRole("columnheader")
    .map((header) => header.textContent ?? "");
  const row = within(table)
    .getByRole("rowheader", { name: originPath })
    .closest("tr");
  if (row === null) {
    throw new Error(`row for ${originPath} was not found`);
  }
  const cells = [...row.children].map((cell) => cell.textContent ?? "");
  return new Map(headers.map((header, index) => [header, cells[index] ?? ""]));
}

/** 基準の directory から `logs/proxy` までの一覧の応答。 */
const listingReplies: Record<string, Reply[]> = {
  [sourceFilesRequest(".")]: [jsonResponse(200, baseListingJson())],
  [sourceFilesRequest("logs")]: [jsonResponse(200, logsListingJson())],
  [sourceFilesRequest("logs/proxy")]: [jsonResponse(200, proxyListingJson())],
  [sourceFilesRequest("logs/proxy", true)]: [
    jsonResponse(200, proxyRecursiveListingJson()),
  ],
  [sourceFilesRequest(".", true)]: [
    jsonResponse(200, { ...proxyRecursiveListingJson(), path: "." }),
  ],
};

/** 一覧で directory を順に開く。 */
async function openDirectories(...names: string[]) {
  for (const name of names) {
    fireEvent.click(screen.getByRole("button", { name: `${name} を開く` }));
    await advance(0);
  }
}

/** 文字列が `prefix` で始まる状態の表示 (`role="status"`) を 1 つ返す。 */
function statusStartingWith(prefix: string): HTMLElement {
  const matched = screen
    .getAllByRole("status")
    .filter((element) => element.textContent?.startsWith(prefix) === true);
  const [only] = matched;
  if (only === undefined || matched.length !== 1) {
    throw new Error(
      `expected one status starting with ${prefix}, got ${matched.length}`,
    );
  }
  return only;
}

/** 読み込む収集元の表の、path の行の入力形式の選択。 */
function formatSelect(originPath: string): HTMLSelectElement {
  return screen.getByLabelText(`${originPath} の入力形式`);
}

function processingButton(): HTMLElement {
  return screen.getByRole("button", { name: "処理を開始" });
}

const refreshFailureSummary = "段階の状態の取得";
const refetchLabel = "段階の状態を再読み込み";

function refetchButton(): HTMLElement | null {
  return screen.queryByRole("button", { name: refetchLabel });
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

test("段階を始めていない server で、基準の directory の一覧と読み込む収集元の入力欄を出し、処理を始める操作を止める", async () => {
  await renderStages({
    [getStages]: [jsonResponse(200, notStartedStagesJson())],
    ...listingReplies,
  });

  const loading = stageSection("収集元の読み込み");
  expect(stateOf(loading)).toBe("状態: 未開始");
  expect(loading.textContent).toContain("収集元: 0");
  expect(
    within(loading).getByRole("table", { name: "directory の項目" }),
  ).toBeTruthy();
  const form = within(loading).getByRole("form", { name: "読み込む収集元" });
  expect(form.textContent).toContain("未選択");
  expect(
    isBlocked(within(form).getByRole("button", { name: /件の読み込みを開始/ })),
  ).toBe(true);
  expect(loading.textContent).not.toContain("--source-root");
  expect(stateOf(stageSection("処理"))).toBe("状態: 未開始");
  expect(isBlocked(processingButton())).toBe(true);
});

test("名前で絞り込んでも選択と入力を保持し、すべて外すと一覧の選択も解除する", async () => {
  await renderStages({
    [getStages]: [jsonResponse(200, notStartedStagesJson())],
    ...listingReplies,
  });
  await openDirectories("logs", "proxy");
  fireEvent.click(screen.getByLabelText("access.log を選ぶ"));
  fireEvent.change(
    screen.getByLabelText(`${proxyLogPath} を記録した端末の表示名`),
    { target: { value: "端末A" } },
  );
  fireEvent.change(
    screen.getByRole("searchbox", { name: "ファイル名で絞り込み" }),
    { target: { value: "NOTES" } },
  );
  expect(screen.queryByLabelText("access.log を選ぶ")).toBeNull();
  expect(screen.getByLabelText("notes.bin を選ぶ")).toBeTruthy();
  expect(
    screen.getByLabelText(`${proxyLogPath} を記録した端末の表示名`),
  ).toHaveProperty("value", "端末A");
  fireEvent.click(screen.getByRole("button", { name: "すべて外す" }));
  expect(screen.queryByLabelText(`${proxyLogPath} の入力形式`)).toBeNull();
  fireEvent.change(screen.getByRole("searchbox"), { target: { value: "" } });
  expect(screen.getByLabelText("access.log を選ぶ")).not.toBeChecked();
});

test("一覧のファイルをドラッグで追加し、重複追加しても入力を保持する", async () => {
  await renderStages({
    [getStages]: [jsonResponse(200, notStartedStagesJson())],
    ...listingReplies,
  });
  await openDirectories("logs", "proxy");
  const transfer = {
    types: [sourceDragType],
    setData: vi.fn(),
    effectAllowed: "",
    dropEffect: "",
  };
  const row = within(
    screen.getByRole("table", { name: "directory の項目" }),
  ).getByRole("row", { name: /access\.log/ });
  const form = screen.getByRole("form", { name: "読み込む収集元" });
  fireEvent.dragStart(row, { dataTransfer: transfer });
  fireEvent.dragOver(form, { dataTransfer: transfer });
  fireEvent.drop(form, { dataTransfer: transfer });
  expect(transfer.setData).toHaveBeenCalledWith(sourceDragType, proxyLogPath);
  expect(formatSelect(proxyLogPath)).toHaveProperty("value", "squid_combined");
  fireEvent.change(
    screen.getByLabelText(`${proxyLogPath} を記録した端末の表示名`),
    { target: { value: "端末B" } },
  );
  fireEvent.dragStart(row, { dataTransfer: transfer });
  fireEvent.drop(form, { dataTransfer: transfer });
  expect(screen.getAllByLabelText(`${proxyLogPath} の入力形式`)).toHaveLength(
    1,
  );
  expect(
    screen.getByLabelText(`${proxyLogPath} を記録した端末の表示名`),
  ).toHaveProperty("value", "端末B");
});

test("フォルダのドラッグは再帰的に判定したファイルを追加し、偽の内部ドラッグは無視する", async () => {
  await renderStages({
    [getStages]: [jsonResponse(200, notStartedStagesJson())],
    ...listingReplies,
  });
  const form = screen.getByRole("form", { name: "読み込む収集元" });
  fireEvent.drop(form, { dataTransfer: { types: [sourceDragType] } });
  expect(screen.queryByRole("combobox")).toBeNull();
  await openDirectories("logs");
  const row = within(
    screen.getByRole("table", { name: "directory の項目" }),
  ).getByRole("row", { name: /proxy/ });
  const transfer = {
    types: [sourceDragType],
    setData: vi.fn(),
    effectAllowed: "",
  };
  fireEvent.dragStart(row, { dataTransfer: transfer });
  fireEvent.drop(form, { dataTransfer: transfer });
  await advance(0);
  expect(formatSelect(proxyLogPath)).toBeTruthy();
  expect(screen.queryByLabelText(`${unknownFilePath} の入力形式`)).toBeNull();
});

test("PCからドロップしたファイルは本文をアップロードし、判定結果を追加する", async () => {
  const batch = "11111111-1111-4111-8111-111111111111";
  const randomUUID = vi.spyOn(crypto, "randomUUID").mockReturnValue(batch);
  const file = new File(["log contents"], "access.log");
  const listing = proxyListingJson();
  listing.entries = listing.entries.slice(0, 1);
  const mock = await renderStages({
    [getStages]: [jsonResponse(200, notStartedStagesJson())],
    [`POST /api/v0/stages/source-files?path=access.log&upload=${batch}`]: [
      jsonResponse(201, listing),
    ],
  });
  const form = screen.getByRole("form", { name: "読み込む収集元" });
  fireEvent.drop(form, {
    dataTransfer: { types: ["Files"], items: [], files: [file] },
  });
  await advance(0);
  expect(formatSelect(proxyLogPath)).toHaveProperty("value", "squid_combined");
  const call = mock.mock.calls.find(
    ([input, init]) =>
      input.includes("source-files?path=access.log") && init?.method === "POST",
  );
  expect(call?.[1]?.body).toBe(file);
  expect(screen.getByText(/追加: 1件/)).toBeTruthy();
  randomUUID.mockRestore();
});

test("directory を辿り、file ごとに判定した形式か判定できない理由を出す", async () => {
  await renderStages({
    [getStages]: [jsonResponse(200, notStartedStagesJson())],
    ...listingReplies,
  });

  await openDirectories("logs", "proxy");

  const table = screen.getByRole("table", { name: "directory の項目" });
  const rowText = (name: string) =>
    within(table).getByRole("rowheader", { name }).closest("tr")?.textContent;
  expect(rowText("access.log")).toContain("Squid combined");
  expect(rowText("other.log")).toContain(
    "候補 2 件: InfoTrace Mark II / Squid combined",
  );
  expect(rowText("notes.bin")).toContain("形式を判定できない (ZIP)");
  expect(rowText("SYSTEM.LOG1")).toContain("主 file と一緒に読む");
  // 付属の file は単独で選べない。
  expect(screen.getByLabelText("SYSTEM.LOG1 を選ぶ")).toHaveProperty(
    "disabled",
    true,
  );
  expect(screen.getByRole("button", { name: "/ proxy" })).toHaveAttribute(
    "aria-current",
    "location",
  );

  fireEvent.click(screen.getByRole("button", { name: "ログフォルダ" }));
  await advance(0);
  expect(screen.getByRole("button", { name: "logs を開く" })).toBeTruthy();
});

test("基準の directory の下の file をまとめて選ぶと、基準の directory を名前で出す", async () => {
  await renderStages({
    [getStages]: [jsonResponse(200, notStartedStagesJson())],
    ...listingReplies,
  });

  fireEvent.click(
    screen.getByRole("button", {
      name: "フォルダ内をすべて追加",
    }),
  );
  await advance(0);

  expect(
    screen.getByText(
      /^ログフォルダ の下の、形式を判定できた 2 件を選択に含めた。/,
    ),
  ).toBeTruthy();
});

test("directory の下の file をまとめて選ぶと、判定できた file だけを足し、含めなかった項目を理由ごとに数え、形式が 1 つの file は選んだ状態にする", async () => {
  await renderStages({
    [getStages]: [jsonResponse(200, notStartedStagesJson())],
    ...listingReplies,
  });
  await openDirectories("logs");

  fireEvent.click(
    screen.getByRole("button", {
      name: "proxy の下の、形式を判定できた file をすべて選ぶ",
    }),
  );
  await advance(0);

  expect(statusStartingWith("logs/proxy の下の").textContent).toBe(
    "logs/proxy の下の、形式を判定できた 2 件を選択に含めた。 含めていない項目: 形式を判定できない 1 件、空の file 1 件、読めない file 1 件、主 file と一緒に読む 1 件、選べない項目 1 件。",
  );
  expect(formatSelect(proxyLogPath).value).toBe("squid_combined");
  // 候補が複数の file は、分析者が選ぶまで未選択にする。
  expect(formatSelect("logs/proxy/other.log").value).toBe("");
  expect(screen.queryByLabelText(`${unknownFilePath} の入力形式`)).toBeNull();
  expect(
    screen.getByText("入力形式・ログ書式が未指定の 1 件は読み込まない"),
  ).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "1 件の読み込みを開始" }),
  ).toBeTruthy();
});

/**
 * `deferredKey` の要求だけ応答を保留し、ほかの要求には `replies` の応答を返す。返す関数で保留した
 * 要求に応答する。
 */
function stubFetchWithDeferred(
  replies: Record<string, Reply[]>,
  deferredKey: string,
): (reply: Response) => void {
  let release: (reply: Response) => void = () => {
    throw new Error(`${deferredKey} was not requested`);
  };
  const others = stubFetch(replies);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string, init?: RequestInit) => {
      if (`${init?.method ?? "GET"} ${input}` === deferredKey) {
        return new Promise<Response>((resolve) => {
          release = resolve;
        });
      }
      return others(input, init);
    }),
  );
  return (reply) => release(reply);
}

test("項目の数が上限に達した一覧と空の一覧を、それぞれ文で出す", async () => {
  await renderStages({
    [getStages]: [jsonResponse(200, notStartedStagesJson())],
    ...listingReplies,
    [sourceFilesRequest(".")]: [
      jsonResponse(200, { ...baseListingJson(), truncated: true }),
    ],
    [sourceFilesRequest("logs")]: [
      jsonResponse(200, { ...logsListingJson(), entries: [] }),
    ],
  });

  expect(
    screen.getByText(
      "項目の数が上限に達したため、上限を超えた項目を表示していない",
    ),
  ).toBeTruthy();
  await openDirectories("logs");
  expect(screen.getByText("項目なし")).toBeTruthy();
});

test("選べない項目、空の file、読めない file は、理由を出して選べなくする", async () => {
  await renderStages({
    [getStages]: [jsonResponse(200, notStartedStagesJson())],
    ...listingReplies,
    [sourceFilesRequest(".")]: [
      jsonResponse(200, {
        ...proxyRecursiveListingJson(),
        path: ".",
        recursive: false,
      }),
    ],
  });

  const table = screen.getByRole("table", { name: "directory の項目" });
  for (const [name, text] of [
    ["empty.log", "空の file"],
    ["locked.log", "読めない file"],
    ["linked", "選べない項目"],
  ] as const) {
    const row = within(table).getByRole("rowheader", { name }).closest("tr");
    expect(row?.textContent).toContain(text);
    expect(within(table).getByLabelText(`${name} を選ぶ`)).toHaveProperty(
      "disabled",
      true,
    );
  }
});

test("まとめて選ぶ取得が失敗したら、失敗した操作を出し、選択を変えない", async () => {
  await renderStages({
    [getStages]: [jsonResponse(200, notStartedStagesJson())],
    ...listingReplies,
    [sourceFilesRequest("logs/proxy", true)]: [
      jsonResponse(500, { code: "internal_error", message: "x" }),
    ],
  });
  await openDirectories("logs");

  fireEvent.click(
    screen.getByRole("button", {
      name: "proxy の下の、形式を判定できた file をすべて選ぶ",
    }),
  );
  await advance(0);

  expect(
    within(screen.getByRole("alert")).getByText(
      "基準の directory の一覧の取得",
    ),
  ).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "0 件の読み込みを開始" }),
  ).toBeTruthy();
});

test("directory を移った後に届いた前の directory の一覧を出さない", async () => {
  const releaseLogs = stubFetchWithDeferred(
    {
      [getStages]: [jsonResponse(200, notStartedStagesJson())],
      ...listingReplies,
    },
    sourceFilesRequest("logs"),
  );
  render(<StagesUnderTest />);
  await advance(0);

  fireEvent.click(screen.getByRole("button", { name: "logs を開く" }));
  await advance(0);
  fireEvent.click(screen.getByRole("button", { name: "ログフォルダ" }));
  await advance(0);
  releaseLogs(jsonResponse(200, logsListingJson()));
  await advance(0);

  expect(screen.getByRole("button", { name: "logs を開く" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "proxy を開く" })).toBeNull();
});

test("まとめて選ぶ取得の応答を待つ間は、読み込みを始めない", async () => {
  const releaseBulk = stubFetchWithDeferred(
    {
      [getStages]: [jsonResponse(200, notStartedStagesJson())],
      ...listingReplies,
    },
    sourceFilesRequest("logs/proxy", true),
  );
  render(<StagesUnderTest />);
  await advance(0);
  await openDirectories("logs", "proxy");

  fireEvent.click(screen.getByLabelText("access.log を選ぶ"));
  fireEvent.click(
    screen.getByRole("button", {
      name: "フォルダ内をすべて追加",
    }),
  );
  await advance(0);
  expect(
    isBlocked(screen.getByRole("button", { name: "1 件の読み込みを開始" })),
  ).toBe(true);

  releaseBulk(jsonResponse(200, proxyRecursiveListingJson()));
  await advance(0);
  // 候補が 2 つの file は形式を選ぶまで数えない。
  expect(
    isBlocked(screen.getByRole("button", { name: "1 件の読み込みを開始" })),
  ).toBe(false);
  expect(formatSelect("logs/proxy/other.log").value).toBe("");
});

test("閲覧者の役割では、読み込みと処理を始める操作を止め、理由を出す", async () => {
  stubFetch({ [getStages]: [jsonResponse(200, loadingCompletedStagesJson())] });
  render(
    <SignedInContext.Provider value={signedInViewer}>
      <StagesUnderTest />
    </SignedInContext.Provider>,
  );
  await advance(0);

  expect(screen.getAllByText("閲覧者は開始不可").length).toBeGreaterThan(0);
  expect(isBlocked(processingButton())).toBe(true);
});

test("閲覧者の役割では、基準の directory の一覧と読み込みの入力欄を出さない", async () => {
  const mock = stubFetch({
    [getStages]: [jsonResponse(200, notStartedStagesJson())],
  });
  render(
    <SignedInContext.Provider value={signedInViewer}>
      <StagesUnderTest />
    </SignedInContext.Provider>,
  );
  await advance(0);

  const loading = stageSection("収集元の読み込み");
  expect(within(loading).queryByRole("form")).toBeNull();
  expect(screen.getByText("閲覧者は開始不可", { selector: "p" })).toBeTruthy();
  // 一覧は編集者の役割を要るため、要求を送らない。
  expect(
    mock.mock.calls.some(([input]) => String(input).includes("source-files")),
  ).toBe(false);
});

test("要求による読み込みを受け付けない server で、入力欄の代わりに起動の指定を出す", async () => {
  await renderStages({
    [getStages]: [jsonResponse(200, rejectingRequestedLoadingStagesJson())],
  });

  const loading = stageSection("収集元の読み込み");
  expect(within(loading).queryByRole("form")).toBeNull();
  expect(loading.textContent).toContain("画面からの読み込み: 使用不可");
  fireEvent.click(
    within(loading).getByRole("button", { name: "画面からの読み込み の説明" }),
  );
  expect(document.body.textContent).toContain("起動の指定: --source-root");
});

test("読み込み中の収集元の読んだ byte 数と大きさを出し、大きさと取り込みの状態の欠測を書く", async () => {
  await renderStages({
    [getStages]: [jsonResponse(200, loadingRunningStagesJson())],
  });

  const loading = stageSection("収集元の読み込み");
  expect(stateOf(loading)).toBe("状態: 実行中");
  expect(within(loading).queryByRole("form")).toBeNull();

  const proxy = rowValues(proxyLogPath);
  expect(proxy.get("入力形式")).toBe("Squid combined");
  expect(proxy.get("読み込んだ byte")).toBe("2,048");
  expect(proxy.get("大きさ")).toBe("8,192");
  // 値の無い列は印「—」を出し、理由は tooltip と読み上げのラベルに持つ。
  expect(proxy.get("公開の状態")).toBe("—読み込みの段階の完了後に表示");

  const host = rowValues(hostLogPath);
  expect(host.get("読み込んだ byte")).toBe("0");
  expect(host.get("大きさ")).toBe("—大きさ不明");
  expect(isBlocked(processingButton())).toBe(true);
});

test("読み込みを終えた収集元の公開の状態と件数を出し、処理を始める操作を受け付ける", async () => {
  const mock = await renderStages({
    [getStages]: [jsonResponse(200, loadingCompletedStagesJson())],
    "POST /api/v0/stages/processing": [
      jsonResponse(202, processingRunningStagesJson()),
    ],
  });

  const proxy = rowValues(proxyLogPath);
  expect(proxy.get("公開の状態")).toBe("全体を公開");
  expect(proxy.get("読み込み")).toBe("40");
  expect(proxy.get("成功")).toBe("38");
  expect(proxy.get("失敗")).toBe("2");
  expect(proxy.get("失敗原因の分類")).toBe("未対応の形式: 2");
  expect(proxy.get("失敗の件数")).toBe("2");

  const host = rowValues(hostLogPath);
  expect(host.get("公開の状態")).toBe("一部を公開");
  expect(host.get("成功")).toBe("—未集計");
  expect(host.get("失敗原因の分類")).toBe("なし");

  expect(isBlocked(processingButton())).toBe(false);
  fireEvent.click(processingButton());
  await advance(0);

  expect(
    mock.mock.calls.some(
      ([input, init]) =>
        input === "/api/v0/stages/processing" && init?.method === "POST",
    ),
  ).toBe(true);
  const processing = stageSection("処理");
  expect(stateOf(processing)).toBe("状態: 実行中");
  const steps = within(processing)
    .getByRole("list", { name: "処理の手順" })
    .querySelectorAll("li");
  expect([...steps].map((step) => step.textContent)).toEqual([
    "レコードからのノードとエッジの作成: 完了",
    "収集元をまたぐエッジの推定: 実行中",
  ]);
  expect(isBlocked(processingButton())).toBe(true);
});

test("処理が失敗した理由と手順の状態を出し、処理を始め直す操作を受け付ける", async () => {
  await renderStages({
    [getStages]: [jsonResponse(200, processingFailedStagesJson())],
  });

  const processing = stageSection("処理");
  expect(stateOf(processing)).toBe("状態: 失敗");
  const alert = within(processing).getByRole("alert");
  expect(alert.textContent).toContain("失敗の理由: グラフの構築の失敗");
  // 収集元に関わらない失敗は、失敗した収集元の組を出さない。
  expect(alert.textContent).not.toContain("失敗した収集元");
  expect(
    within(processing).getByText("収集元をまたぐエッジの推定: 失敗"),
  ).toBeTruthy();
  expect(isBlocked(processingButton())).toBe(false);
});

test("選んだ収集元を読み込みと処理の要求で送り、読み込みが失敗した後の入力欄に残す", async () => {
  const mock = await renderStages({
    [getStages]: [
      jsonResponse(200, notStartedStagesJson()),
      jsonResponse(200, loadingFailedStagesJson()),
    ],
    ...listingReplies,
    "POST /api/v0/stages/loading": [
      jsonResponse(202, loadingRunningStagesJson()),
    ],
  });
  await openDirectories("logs", "proxy");

  fireEvent.click(screen.getByLabelText("access.log を選ぶ"));
  fireEvent.click(screen.getByLabelText("notes.bin を選ぶ"));
  // 判定できない file は、形式を選ぶと読み込む。
  fireEvent.change(formatSelect(unknownFilePath), {
    target: { value: "infotrace_mark_ii" },
  });
  fireEvent.change(
    screen.getByLabelText(`${proxyLogPath} を記録した端末の表示名`),
    { target: { value: "proxy01" } },
  );
  const form = screen.getByRole("form", { name: "読み込む収集元" });
  fireEvent.change(within(form).getByLabelText("案件"), {
    target: { value: "baseline" },
  });
  fireEvent.click(
    within(form).getByRole("button", { name: "2 件の読み込みを開始" }),
  );
  await advance(0);

  const post = mock.mock.calls.find(
    ([input]) => input === "/api/v0/stages/loading",
  );
  expect(JSON.parse(String(post?.[1]?.body))).toEqual({
    sources: [
      {
        originPath: proxyLogPath,
        formatKey: "squid_combined",
        caseId: "baseline",
        terminal: { hostname: "proxy01" },
      },
      {
        originPath: unknownFilePath,
        formatKey: "infotrace_mark_ii",
        caseId: "baseline",
      },
    ],
    thenProcess: true,
  });
  expect(screen.queryByRole("form")).toBeNull();

  await advance(stagesPollIntervalMs);

  const loading = stageSection("収集元の読み込み");
  expect(stateOf(loading)).toBe("状態: 失敗");
  const failure = within(loading)
    .getAllByRole("alert")
    .find((alert) => alert.textContent?.includes("失敗の理由"));
  if (failure === undefined) {
    throw new Error("the loading failure was not shown");
  }
  expect(failure.textContent).toContain(
    "失敗の理由: 収集元の読み取りか取り込みの失敗",
  );
  expect(failure.textContent).toContain(`失敗した収集元: ${proxyLogPath}`);
  expect(formatSelect(proxyLogPath).value).toBe("squid_combined");
  expect(formatSelect(unknownFilePath).value).toBe("infotrace_mark_ii");
});

test("読み込みの完了後に処理を始める指定を外すと、処理の指定を送らない", async () => {
  const mock = await renderStages({
    [getStages]: [jsonResponse(200, notStartedStagesJson())],
    ...listingReplies,
    "POST /api/v0/stages/loading": [
      jsonResponse(202, loadingRunningStagesJson()),
    ],
  });
  await openDirectories("logs", "proxy");

  fireEvent.click(screen.getByLabelText("access.log を選ぶ"));
  fireEvent.click(screen.getByLabelText("読み込みの完了後に処理を始める"));
  fireEvent.click(screen.getByRole("button", { name: "1 件の読み込みを開始" }));
  await advance(0);

  const post = mock.mock.calls.find(
    ([input]) => input === "/api/v0/stages/loading",
  );
  expect(JSON.parse(String(post?.[1]?.body))).toEqual({
    sources: [{ originPath: proxyLogPath, formatKey: "squid_combined" }],
  });
});

test("一覧の取得が退けられたら、失敗した操作と理由を出す", async () => {
  await renderStages({
    [getStages]: [jsonResponse(200, notStartedStagesJson())],
    [sourceFilesRequest(".")]: [
      jsonResponse(400, {
        code: "invalid_request",
        message: "the loading request is invalid",
        loadingRejection: "file_unreadable",
        originPath: ".",
      }),
    ],
  });

  const alert = within(stageSection("収集元の読み込み")).getByRole("alert");
  expect(within(alert).getByText("基準の directory の一覧の取得")).toBeTruthy();
  expect(within(alert).getByText("読み込めない file")).toBeTruthy();
});

test("読み込みを始める要求が退けられたら、失敗した操作と理由を出す", async () => {
  await renderStages({
    [getStages]: [jsonResponse(200, notStartedStagesJson())],
    ...listingReplies,
    "POST /api/v0/stages/loading": [
      jsonResponse(400, {
        code: "invalid_request",
        message: "the loading request is invalid",
        loadingRejection: "path_outside_base",
        originPath: proxyLogPath,
      }),
    ],
  });
  await openDirectories("logs", "proxy");

  fireEvent.click(screen.getByLabelText("access.log を選ぶ"));
  fireEvent.click(screen.getByRole("button", { name: "1 件の読み込みを開始" }));
  await advance(0);

  const alert = within(stageSection("収集元の読み込み"))
    .getAllByRole("alert")
    .find((item) => item.textContent?.includes("収集元の読み込みの開始"));
  if (alert === undefined) {
    throw new Error("the loading failure was not shown");
  }
  // 拒否された理由を失敗の種類のラベルに出し、コードと path を詳細の組に持つ。
  expect(within(alert).getByText("directory の外の path")).toBeTruthy();
  expect(alert.textContent).toContain("コード: invalid_request");
  expect(alert.textContent).toContain(`path: ${proxyLogPath}`);
  expect(within(stageSection("処理")).queryByRole("alert")).toBeNull();
});

test("段階の状態の最初の取得が失敗したら、失敗した操作と、取得し直す操作を出す", async () => {
  const mock = await renderStages({
    [getStages]: [
      jsonResponse(500, { code: "internal_error", message: "x" }),
      jsonResponse(200, notStartedStagesJson()),
    ],
  });

  expect(screen.getByText("段階の状態の取得")).toBeTruthy();
  expect(screen.queryByRole("region", { name: "収集元の読み込み" })).toBeNull();

  fireEvent.click(screen.getByRole("button", { name: refetchLabel }));
  await advance(0);

  expect(
    mock.mock.calls.filter(([input]) => input === "/api/v0/stages"),
  ).toHaveLength(2);
  expect(stageSection("収集元の読み込み")).toBeTruthy();
  expect(screen.queryByRole("button", { name: refetchLabel })).toBeNull();
});

test("開始を退けられた後の取り直しが失敗したら、最後に読めた状態を保ち、取得し直す操作を出す", async () => {
  const mock = await renderStages({
    [getStages]: [
      jsonResponse(200, loadingCompletedStagesJson()),
      new TypeError("offline"),
      jsonResponse(200, processingRunningStagesJson()),
    ],
    "POST /api/v0/stages/processing": [
      jsonResponse(409, { code: "stage_already_started", message: "x" }),
    ],
  });

  fireEvent.click(processingButton());
  await advance(0);

  expect(screen.getByText(refreshFailureSummary)).toBeTruthy();
  expect(stateOf(stageSection("処理"))).toBe("状態: 未開始");
  const refetch = refetchButton();
  if (refetch === null) {
    throw new Error("the operation to fetch the stages again was not shown");
  }
  expect(refetch).toHaveProperty("disabled", false);

  fireEvent.click(refetch);
  await advance(0);

  expect(
    mock.mock.calls.filter(([input]) => input === "/api/v0/stages"),
  ).toHaveLength(3);
  expect(stateOf(stageSection("処理"))).toBe("状態: 実行中");
  expect(screen.queryByText(refreshFailureSummary)).toBeNull();
  expect(refetchButton()).toBeNull();
});

test("実行中の段階の取り直しが失敗したあいだは、次の取り直しを予約しており、取得し直す操作を出さない", async () => {
  await renderStages({
    [getStages]: [
      jsonResponse(200, loadingRunningStagesJson()),
      new TypeError("offline"),
    ],
  });

  await advance(stagesPollIntervalMs);

  expect(screen.getByText(refreshFailureSummary)).toBeTruthy();
  expect(refetchButton()).toBeNull();
});

test("取得し直す応答を待つあいだ、取得し直す操作と処理を始める操作を止める", async () => {
  let getCount = 0;
  let resolveRefetch: (reply: Response) => void = () => {};
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        return jsonResponse(409, {
          code: "stage_already_started",
          message: "x",
        });
      }
      getCount += 1;
      if (getCount === 1) {
        return jsonResponse(200, loadingCompletedStagesJson());
      }
      if (getCount === 2) {
        throw new TypeError("offline");
      }
      return new Promise<Response>((resolve) => {
        resolveRefetch = resolve;
      });
    }),
  );
  render(<StagesUnderTest />);
  await advance(0);
  fireEvent.click(processingButton());
  await advance(0);

  const refetch = refetchButton();
  if (refetch === null) {
    throw new Error("the operation to fetch the stages again was not shown");
  }
  fireEvent.click(refetch);
  await advance(0);

  expect(refetchButton()).toHaveProperty("disabled", true);
  expect(isBlocked(processingButton())).toBe(true);

  await act(async () => {
    resolveRefetch(jsonResponse(200, loadingCompletedStagesJson()));
  });
  await advance(0);

  expect(refetchButton()).toBeNull();
  expect(isBlocked(processingButton())).toBe(false);
});
