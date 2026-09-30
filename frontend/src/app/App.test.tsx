// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, test, vi } from "vitest";
import {
  findEdgeList,
  findNodeList,
} from "@/features/graphExplore/graphExploreTestHarness";
import { histogramColumns } from "@/features/timeline/TimeHistogram";
import { conditionKeys } from "@/shared/contracts/candidates";
import { DecodeFailure } from "@/shared/contracts/decoding";
import { SignedInContext } from "@/shared/ui/SignedInAccount";
import { emptyAssertionsResponseJson } from "@/testdata/assertions/assertionsResponse";
import {
  adoptionJson,
  assistProposalsResponseJson,
  proposedItemJson,
  relationItemJson,
} from "@/testdata/assistProposals/assistProposalsResponse";
import {
  baselineCaseId,
  casedSourcesResponseJson,
  challengeCaseId,
} from "@/testdata/cases/caseCounts";
import {
  addCondition,
  chooseConditionKind,
  chooseConditionValue,
  conditionValueOptions,
} from "@/testdata/conditionInput";
import { eventKindsResponseJson } from "@/testdata/eventKinds/eventKindsResponse";
import {
  edgeDetailResponseJson,
  graphResponseJson,
  invisibleCharacterGraphResponseJson,
  ipNodeId,
  matchedEdgeDetailResponseJson,
  nodeDetailResponseJson,
  processNodeId,
  terminalNodeId,
} from "@/testdata/graph/graphResponse";
import {
  graphNodeJson,
  influencePathResponseJson,
} from "@/testdata/graph/influencePathResponse";
import { jsonResponse } from "@/testdata/http";
import {
  hostALogRecordRawText,
  hostALogRecordResponseJson,
} from "@/testdata/records/recordResponse";
import { signedInAlice, signedInViewer } from "@/testdata/session";
import {
  apiErrorJson,
  hostALogSha256,
  hostALogSourceId,
  invisibleFileNameSourcesResponseJson,
  sourcesResponseJson,
} from "@/testdata/sources/sourcesResponse";
import { timelineResponseJson } from "@/testdata/timeline/timelineResponse";
import { App } from "./App";
import { decodeWorkspaceState, type WorkspaceState } from "./workspaceState";

// WebGL の renderer は jsdom で動かない。画面の組み立てを確かめる本 test は図を
// stub に置き換える。実描画はブラウザーで確かめる。凡例に足す項目は stub も描く。
vi.mock("@/features/graphExplore/SubgraphCanvas", () => ({
  SubgraphCanvas: ({ legendItems }: { legendItems?: ReactNode }) => (
    <>
      <p>部分グラフの図</p>
      <ul aria-label="グラフの凡例">{legendItems}</ul>
    </>
  ),
}));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  // ビューの配置は localStorage に残る。前の test の配置で次の test を始めない。
  localStorage.clear();
});

/**
 * `GET /api/v0/record-numbers` の応答。番号を宣言していない形式の収集元とし、相手を与えた要求には
 * 比べなかった突き合わせを返す。
 */
function recordNumbersResponseJson(input: string): unknown {
  const query = new URL(input, "http://localhost").searchParams;
  const comparedSourceId = query.get("comparedSourceId");
  return {
    recordNumbers: {
      sourceId: query.get("sourceId"),
      examination: "not_examined",
      notExaminedReason: "record_numbers_not_declared",
      streams: [],
      listLimit: 200,
    },
    ...(comparedSourceId === null
      ? {}
      : {
          comparison: {
            comparedSourceId,
            state: "not_compared",
            notComparedReason: "comparison_key_not_declared",
            oneToOneKeyCount: 0,
            onlyInSourceRecordCount: 0,
            onlyInComparedRecordCount: 0,
            onlyInSourceRecordRefs: [],
            onlyInComparedRecordRefs: [],
            onlyInSourceOutsideComparedRangeRecordCount: 0,
            onlyInSourceInsideComparedRangeRecordCount: 0,
            onlyInSourceInsideComparedRangeRecordRefs: [],
            onlyInComparedOutsideSourceRangeRecordCount: 0,
            onlyInComparedInsideSourceRangeRecordCount: 0,
            onlyInComparedInsideSourceRangeRecordRefs: [],
            listLimit: 200,
            undeterminedKeyCount: 0,
            undeterminedSourceRecordCount: 0,
            undeterminedComparedRecordCount: 0,
            unequalKeyCount: 0,
            sourceSurplusRecordCount: 0,
            comparedSurplusRecordCount: 0,
            unequalKeys: [],
            sourceUnkeyedRecordCount: 0,
            comparedUnkeyedRecordCount: 0,
          },
        }),
  };
}

/** path ごとに、画面が呼ぶ操作の応答を返す。 */
/** 件数の分布の応答。端末を持たない 1 行が、最初の 2 列に 1 件ずつを持つ。 */
function histogramResponseJson() {
  const counts = new Array<number>(histogramColumns).fill(0);
  counts[0] = 1;
  counts[1] = 1;
  return {
    start: "2031-10-08T00:00:00.000Z",
    stepMs: 1000,
    rows: [{ counts }],
    localTimeRecordCount: 0,
    undatedRecordCount: 0,
    spanningRecordCount: 0,
  };
}

function stubFetch(
  sourcesJson: unknown = sourcesResponseJson(),
  graphJson: unknown = graphResponseJson(),
  recordJson: unknown = hostALogRecordResponseJson(),
  assertionsJson: unknown = emptyAssertionsResponseJson(),
  edgeJson: unknown = edgeDetailResponseJson(),
) {
  const mock = vi.fn(async (input: string, _init?: RequestInit) => {
    if (input.startsWith("/api/v0/sources")) {
      return jsonResponse(200, sourcesJson);
    }
    if (input.startsWith("/api/v0/record-numbers")) {
      return jsonResponse(200, recordNumbersResponseJson(input));
    }
    if (input.startsWith("/api/v0/timeline")) {
      return jsonResponse(200, timelineResponseJson());
    }
    if (input.startsWith("/api/v0/time-histogram")) {
      return jsonResponse(200, histogramResponseJson());
    }
    if (input.startsWith("/api/v0/event-kinds")) {
      return jsonResponse(200, eventKindsResponseJson());
    }
    if (input.startsWith("/api/v0/records")) {
      return jsonResponse(200, recordJson);
    }
    // 開いたレコードを根拠に持つのは、図に出る実行のエッジとその端点の 2 つと、図に出ない
    // ノード 1 つである。
    if (input.startsWith("/api/v0/record-graph")) {
      return jsonResponse(200, {
        recordRef: (recordJson as { recordRef: unknown }).recordRef,
        nodeIds: [processNodeId, terminalNodeId, "n:absent:0001"],
        edgeIds: ["e:ran_on:0001"],
      });
    }
    if (input.startsWith("/api/v0/nodes/")) {
      return jsonResponse(200, nodeDetailResponseJson());
    }
    if (input.startsWith("/api/v0/edges/")) {
      return jsonResponse(200, edgeJson);
    }
    if (input.startsWith("/api/v0/assertions")) {
      return jsonResponse(200, assertionsJson);
    }
    if (input.startsWith("/api/v0/assist-proposals")) {
      return jsonResponse(200, assistProposalsResponseJson([]));
    }
    return jsonResponse(200, graphJson);
  });
  vi.stubGlobal("fetch", mock);
  return mock;
}

const sourcesView = "Artifacts";
const recordingView = "Time & Host";
const timelineView = "Timeline";

/** 画面の「名前: 値」の組のうち、最初に見つかった name の組の文字列。 */
function pairText(name: string): string | undefined {
  return Array.from(document.querySelectorAll(".value-pairs > li"))
    .map((item) => item.textContent ?? "")
    .find((text) => text.startsWith(`${name}: `));
}

/** タブの文字列が title に一致するビューを前面に出す。 */
function openView(title: string) {
  const tab = screen.getByRole("tab", { name: title });
  fireEvent.pointerDown(tab);
  fireEvent.mouseDown(tab);
  fireEvent.click(tab);
}

function recordNumbersPaths(mock: ReturnType<typeof stubFetch>): string[] {
  return mock.mock.calls
    .map((call) => call[0])
    .filter((path) => path.startsWith("/api/v0/record-numbers"));
}

test.each([
  ["アカウントを持たない起動", undefined, false],
  ["閲覧者", signedInViewer, false],
  ["編集者", signedInAlice, false],
  ["管理者", { ...signedInAlice, role: "admin" as const }, true],
])(
  "利用者と役割のビューは管理者だけに出す (%s)",
  async (_name, signedIn, shown) => {
    stubFetch();
    render(
      <SignedInContext.Provider value={signedIn}>
        <App />
      </SignedInContext.Provider>,
    );
    await findNodeList();

    expect(screen.queryByRole("tab", { name: "Members" }) !== null).toBe(shown);
  },
);

function layoutPanelIds(state: WorkspaceState | undefined): string[] {
  return Object.keys(
    (state?.dockLayout as { panels?: object } | undefined)?.panels ?? {},
  );
}

test("管理者でない利用者も、利用者と役割のビューを含む配置をそのまま開き、配置を保つ", async () => {
  stubFetch();
  const adminChanges = vi.fn<(state: WorkspaceState) => void>();
  render(
    <SignedInContext.Provider value={{ ...signedInAlice, role: "admin" }}>
      <App onWorkspaceStateChange={adminChanges} />
    </SignedInContext.Provider>,
  );
  await findNodeList();
  const saved = await waitFor(() => {
    const state = adminChanges.mock.lastCall?.[0];
    expect(layoutPanelIds(state)).toContain("members");
    return decodeWorkspaceState(JSON.parse(JSON.stringify(state)));
  });
  cleanup();

  const changes = vi.fn<(state: WorkspaceState) => void>();
  render(
    <SignedInContext.Provider value={signedInAlice}>
      <App
        workspaceState={{ state: saved, revision: 1 }}
        onWorkspaceStateChange={changes}
      />
    </SignedInContext.Provider>,
  );
  await findNodeList();
  openView("Members");
  expect(await screen.findByText("管理者")).toBeTruthy();
  expect(pairText("必要な役割")).toBe("必要な役割: 管理者");
  expect(screen.queryByText("保存した配置の読み込み失敗")).toBeNull();
  expect(layoutPanelIds(changes.mock.lastCall?.[0])).toContain("members");
});

test("レコードを開いていないときは未選択を出し、選び方は「?」の説明で読める", async () => {
  stubFetch();
  render(<App />);
  await findNodeList();
  openView("Record");
  await waitFor(() => expect(pairText("レコード")).toBe("レコード: 未選択"));
  expect(screen.queryByText("Graph・Timeline・Edges のレコード")).toBeNull();
  fireEvent.click(
    await screen.findByRole("button", { name: "レコードの選択 の説明" }),
  );
  expect(screen.getByRole("tooltip").textContent).toBe(
    "Graph・Timeline・Edges のレコード",
  );
});

test("件数の分布で期間を適用すると、グラフに期間のフィルタを適用し、件数の分布は取り直さない", async () => {
  const mock = stubFetch();
  render(<App />);
  await findNodeList();
  openView("Histogram");
  await screen.findByRole("img", { name: "時間と端末ごとの件数" });
  const histogramRequests = () =>
    mock.mock.calls.filter((call) =>
      call[0].startsWith("/api/v0/time-histogram"),
    ).length;
  const before = histogramRequests();

  fireEvent.click(screen.getByRole("button", { name: "期間のフィルタを適用" }));

  await waitFor(() =>
    expect(
      mock.mock.calls.some(
        (call) =>
          call[0].startsWith("/api/v0/graph?") &&
          call[0].includes("timeFrom=2031-10-08T00%3A00%3A00.000Z"),
      ),
    ).toBe(true),
  );
  expect(histogramRequests()).toBe(before);
  expect(
    screen.getByRole("button", { name: "期間のフィルタを解除" }),
  ).toBeTruthy();
});

test("作業場所の既定の配置に Groups のビューを置かない", async () => {
  stubFetch();
  render(<App />);
  await findNodeList();

  expect(screen.getByRole("tab", { name: "IPs" })).toBeTruthy();
  expect(screen.queryByRole("tab", { name: "Groups" })).toBeNull();
});

test("収集元のビューでレコードの番号の tab を選ぶまで、レコードの番号を要求しない", async () => {
  const mock = stubFetch();
  render(<App />);
  await findNodeList();
  expect(recordNumbersPaths(mock)).toEqual([]);
  openView(sourcesView);
  fireEvent.click(await screen.findByRole("button", { name: "access.log" }));
  expect(recordNumbersPaths(mock)).toEqual([]);
  fireEvent.click(screen.getByRole("tab", { name: "レコードの番号" }));
  await waitFor(() => expect(pairText("番号の抜け")).toMatch(/^番号の抜け: /));
  expect(recordNumbersPaths(mock)).toHaveLength(1);
});

test("収集元のビューを隠して前面に戻しても、比べる収集元の選択が残る", async () => {
  const mock = stubFetch();
  render(<App />);
  openView(sourcesView);
  fireEvent.click(await screen.findByRole("button", { name: "access.log" }));
  fireEvent.click(screen.getByRole("tab", { name: "レコードの番号" }));
  fireEvent.change(await screen.findByLabelText("比べる収集元"), {
    target: { value: hostALogSourceId },
  });
  await waitFor(() => expect(pairText("突き合わせ")).toMatch(/^突き合わせ: /));
  openView(timelineView);
  await waitFor(() =>
    expect(screen.queryByLabelText("比べる収集元")).toBeNull(),
  );
  const requested = recordNumbersPaths(mock).length;
  openView(sourcesView);
  fireEvent.click(await screen.findByRole("tab", { name: "レコードの番号" }));
  expect(
    ((await screen.findByLabelText("比べる収集元")) as HTMLSelectElement).value,
  ).toBe(hostALogSourceId);
  await waitFor(() =>
    expect(recordNumbersPaths(mock).slice(requested)).toHaveLength(1),
  );
  expect(
    new URL(
      recordNumbersPaths(mock).at(-1) ?? "",
      "http://localhost",
    ).searchParams.get("comparedSourceId"),
  ).toBe(hostALogSourceId);
});

test("時刻の解釈の入力途中の値は、別のビューを前面に出して戻っても残る", async () => {
  stubFetch();
  render(<App />);
  openView(sourcesView);
  fireEvent.click(await screen.findByRole("button", { name: "access.log" }));
  openView(recordingView);
  const region = await screen.findByRole("region", {
    name: "収集元のタイムゾーン",
  });
  fireEvent.change(within(region).getByLabelText("分析者"), {
    target: { value: "analyst-a" },
  });
  openView("Record");
  openView(recordingView);
  expect(
    within(
      await screen.findByRole("region", { name: "収集元のタイムゾーン" }),
    ).getByLabelText("分析者"),
  ).toHaveValue("analyst-a");
});

test("グラフでノードを選ぶと、時系列をそのノードからホップ数までのレコードを残すフィルタを適用する", async () => {
  const mock = stubFetch();
  render(<App />);
  await findNodeList();
  openView(timelineView);
  const lastTimeline = () =>
    new URL(
      mock.mock.calls
        .map((call) => call[0])
        .filter((path) => path.startsWith("/api/v0/timeline"))
        .at(-1) ?? "",
      "http://localhost",
    ).searchParams;
  await screen.findByRole("table", { name: "収集元ごとの記録期間" });
  expect(lastTimeline().has("nodeId")).toBe(false);
  expect(lastTimeline().has("depth")).toBe(false);

  // ノードの一覧は Timeline と同じ区画の Nodes にある。選んだ後に Timeline へ戻す。
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );
  openView(timelineView);
  await waitFor(() => expect(lastTimeline().get("nodeId")).toBe(processNodeId));
  expect(lastTimeline().get("depth")).toBe("1");
  expect(pairText("ノード")).toBe("ノード: C:\\Windows\\System32\\cmd.exe");

  fireEvent.change(screen.getByLabelText("ホップ数"), {
    target: { value: "2" },
  });
  await waitFor(() => expect(lastTimeline().get("depth")).toBe("2"));

  fireEvent.click(
    screen.getByRole("checkbox", {
      name: "Graph の選択ノードでフィルタ",
    }),
  );
  await waitFor(() => expect(lastTimeline().has("nodeId")).toBe(false));
  expect(lastTimeline().has("depth")).toBe(false);
});

test("全件の時系列は、開くまで取得しない", async () => {
  const mock = stubFetch();

  render(<App />);
  await findNodeList();

  const timelinePaths = () =>
    mock.mock.calls
      .map((call) => call[0])
      .filter((path) => path.startsWith("/api/v0/timeline"));
  expect(timelinePaths()).toEqual([]);

  openView(timelineView);

  await screen.findByRole("table", { name: "収集元ごとの記録期間" });
  expect(timelinePaths()).toHaveLength(1);
});

test("検索の画面で適用した検索式を、時系列の要求にも載せ、外すと載せない", async () => {
  const mock = stubFetch();
  render(<App />);
  await findNodeList();
  openView(timelineView);
  const lastTimeline = () =>
    new URL(
      mock.mock.calls
        .map((call) => call[0])
        .filter((path) => path.startsWith("/api/v0/timeline"))
        .at(-1) ?? "",
      "http://localhost",
    ).searchParams;
  await screen.findByRole("table", { name: "収集元ごとの記録期間" });
  expect(lastTimeline().has("searchExpression")).toBe(false);

  addCondition("検索式", { 検索式: "LogonType == 3" });
  await waitFor(() =>
    expect(lastTimeline().get("searchExpression")).toBe("LogonType == 3"),
  );

  fireEvent.click(
    screen.getByRole("button", { name: "検索式 LogonType == 3 を削除" }),
  );
  await waitFor(() =>
    expect(lastTimeline().has("searchExpression")).toBe(false),
  );
});

test("収集元の一覧を表示し、選んだ収集元を画面の見出しの下に出す", async () => {
  const mock = stubFetch();

  render(<App />);

  expect(screen.getByRole("heading", { level: 1 }).textContent).toBe(
    "Oraculum",
  );
  openView(sourcesView);
  await waitFor(() =>
    expect(pairText("選択中の収集元")).toBe("選択中の収集元: 未選択"),
  );

  const button = await screen.findByRole("button", { name: "access.log" });
  expect(
    mock.mock.calls
      .map((call) => call[0])
      .filter((path) => path.startsWith("/api/v0/sources")),
  ).toEqual(["/api/v0/sources"]);
  expect(screen.getByRole("region", { name: "収集元の一覧" })).toBeTruthy();
  for (const name of ["Search", "Node Detail"]) {
    expect(screen.getByRole("heading", { level: 2, name })).toBeTruthy();
  }
  expect(screen.getByRole("region", { name: "グラフ" })).toBeTruthy();
  // 開いたレコードの欄は、レコードを開くまで出さない。
  expect(
    screen.queryByRole("heading", { level: 2, name: "開いたレコード" }),
  ).toBeNull();

  fireEvent.click(button);

  expect(pairText("選択中の収集元")).toBe("選択中の収集元: access.log");
  expect(button).toHaveAttribute("aria-pressed", "true");
});

test("グラフの画面がノードの一覧を出し、選んだノードの詳細を出す", async () => {
  const mock = stubFetch();

  render(<App />);

  await findNodeList();
  await screen.findByRole("button", {
    name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
  });
  addCondition("含む", { 含む文字列: "cmd.exe" });
  await findNodeList();
  expect(
    Array.from(document.querySelectorAll("mark")).some((mark) =>
      mark.textContent?.includes("cmd.exe"),
    ),
  ).toBe(true);
  const graphPaths = mock.mock.calls
    .map((call) => call[0])
    .filter((path) => path.startsWith("/api/v0/graph"));
  // 図の部分グラフと、端末のフィルタの条件の選択肢の 2 つを取得する。
  expect(graphPaths.length).toBeGreaterThanOrEqual(2);
  // 検索の条件が無い最初の図は、端末と IP アドレスを出し、描画の上限を置かない。
  const graphPath = graphPaths.find((path) => path.includes("&depth=1&")) ?? "";
  expect(graphPath).toContain(
    "/api/v0/graph?nodeKind=terminal&nodeKind=ip&granularity=object&depth=1",
  );
  expect(graphPaths.some((path) => path.includes("nodeLimit"))).toBe(false);
  expect(
    graphPaths.some((path) =>
      path.includes("?nodeKind=terminal&granularity=object&depth=0"),
    ),
  ).toBe(true);
  // 初期の選択は、契約が定めるすべての条件を幅 0 で用いる。
  for (const conditionKey of conditionKeys) {
    expect(graphPath).toContain(
      conditionKey === "second_of_time"
        ? "matchCondition=second_of_time%7E0"
        : `matchCondition=${conditionKey}`,
    );
  }

  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );

  expect(await screen.findByRole("table", { name: "同一性" })).toBeTruthy();
  // ノードにも分析者のメモを付けられる。
  expect(screen.getAllByRole("heading", { name: "メモ" }).length).toBe(1);
});

test("強調の取得に失敗したときは、強調が無いことと区別して失敗を図の下に出す", async () => {
  const mock = stubFetch();
  const served = mock.getMockImplementation();
  mock.mockImplementation(async (input: string, init?: RequestInit) =>
    input.startsWith("/api/v0/record-graph")
      ? jsonResponse(500, apiErrorJson("internal_error"))
      : (served?.(input, init) ?? jsonResponse(500, {})),
  );
  render(<App />);
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );
  fireEvent.click(
    await screen.findByRole("button", {
      name: "Record に表示: 収集元: host-a.log、ID: 112、行: 1022",
    }),
  );

  expect(
    await screen.findByText("レコードのノードとエッジの取得"),
  ).toBeTruthy();
  expect(screen.queryByText(/強調のノード/)).toBeNull();
});

test("レコードを開くと、そのレコードを根拠に持つノードとエッジを図で強調し、閉じると外す", async () => {
  const mock = stubFetch();
  render(<App />);
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );
  fireEvent.click(
    await screen.findByRole("button", {
      name: "Record に表示: 収集元: host-a.log、ID: 112、行: 1022",
    }),
  );

  // 強調の件数は、凡例の横に「応答にある数/対応する数」の値の組で出す。
  const legend = screen.getByRole("list", { name: "グラフの凡例" });
  await waitFor(() =>
    expect(
      within(legend)
        .queryAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual(["強調のノード: 2/3", "強調のエッジ: 1/1"]),
  );
  const request = new URL(
    mock.mock.calls
      .map((call) => call[0])
      .find((path) => path.startsWith("/api/v0/record-graph")) ?? "",
    "http://localhost",
  ).searchParams;
  expect(request.get("sourceId")).toBe(hostALogSourceId);
  expect(request.get("sequenceNumber")).toBe("112");
  expect(request.getAll("matchCondition").length).toBeGreaterThan(0);

  fireEvent.click(screen.getByRole("button", { name: "レコードを閉じる" }));
  await waitFor(() => expect(screen.queryByText(/強調のノード/)).toBeNull());
});

/** 通番 sequenceNumber のレコードの応答。eventId を渡すと Windows イベントログの欄を足す。 */
function windowsRecordJson(
  sequenceNumber: number,
  lineNumber: number,
  eventId?: string,
): unknown {
  const json = hostALogRecordResponseJson() as {
    recordRef: Record<string, unknown>;
    fields: unknown[];
  };
  const text = (value: string) => ({ rawText: value, valueState: "present" });
  return {
    ...json,
    recordRef: { ...json.recordRef, sequenceNumber, lineNumber },
    fields:
      eventId === undefined
        ? json.fields
        : [
            ...json.fields,
            {
              name: "Provider",
              semantic: "windows_event.provider",
              kind: "text",
              text: text("Example-Provider"),
            },
            {
              name: "EventID",
              semantic: "windows_event.id",
              kind: "text",
              text: text(eventId),
            },
          ],
  };
}

test("開いたレコードの見出しと今の場所の名前に Event ID を付け、読み込み中に移った先には古い Event ID を付けない", async () => {
  const mock = stubFetch();
  const base = mock.getMockImplementation();
  // 通番 112 の応答を止めておけるようにする。
  let holdFirst = false;
  let releaseFirst: () => void = () => {};
  let holdSecond = false;
  let releaseSecond: () => void = () => {};
  mock.mockImplementation(async (input: string, init?: RequestInit) => {
    if (input.startsWith("/api/v0/records")) {
      if (input.includes("sequenceNumber=115")) {
        if (holdSecond) {
          await new Promise<void>((resolve) => {
            releaseSecond = resolve;
          });
        }
        return jsonResponse(200, windowsRecordJson(115, 1025));
      }
      if (holdFirst) {
        await new Promise<void>((resolve) => {
          releaseFirst = resolve;
        });
      }
      return jsonResponse(200, windowsRecordJson(112, 1022, "4624"));
    }
    return (base as NonNullable<typeof base>)(input, init);
  });
  // 見出しの「名前: 値」の組を、場所の名前と同じ「、」の区切りで並べる。
  const heading = () =>
    Array.from(document.querySelectorAll(".sheet-position > li"))
      .map((item) => item.textContent)
      .join("、");
  const openRecord = async (name: string) =>
    fireEvent.click(await screen.findByRole("button", { name }));
  const first = "Record に表示: 収集元: host-a.log、ID: 112、行: 1022";
  const second = "Record に表示: 収集元: host-a.log、ID: 115、行: 1025";

  render(<App />);
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );
  await openRecord(first);
  await waitFor(() =>
    expect(heading()).toBe(
      "Event ID: 4624、収集元: host-a.log、ID: 112、行: 1022",
    ),
  );
  fireEvent.click(
    await within(
      screen.getByRole("navigation", { name: "表示の履歴" }),
    ).findByRole("button", {
      name: "Event ID: 4624、収集元: host-a.log、ID: 112、行: 1022 をブックマークに追加",
    }),
  );
  openView("Bookmarks");
  const bookmarks = await screen.findByRole("region", { name: "ブックマーク" });
  expect(
    within(bookmarks).getByRole("button", {
      name: "Event ID: 4624、host-a.log",
    }),
  ).toBeTruthy();

  // 通番 115 を読み込み終える前にも、通番 112 の Event ID を付けない。
  holdSecond = true;
  await openRecord(second);
  await waitFor(() =>
    expect(heading()).toBe("収集元: host-a.log、ID: 115、行: 1025"),
  );
  holdSecond = false;
  releaseSecond();
  // 通番 112 を読み込んでいる間に通番 115 へ移る。
  holdFirst = true;
  await openRecord(first);
  await openRecord(second);
  releaseFirst();
  await waitFor(() =>
    expect(heading()).toBe("収集元: host-a.log、ID: 115、行: 1025"),
  );
  fireEvent.click(
    await within(
      screen.getByRole("navigation", { name: "表示の履歴" }),
    ).findByRole("button", {
      name: "収集元: host-a.log、ID: 115、行: 1025 をブックマークに追加",
    }),
  );
  openView("Bookmarks");
  const list = await screen.findByRole("region", { name: "ブックマーク" });
  const row = within(list).getByText("ID: 115、行: 1025").closest("tr");
  expect(row?.textContent).toContain("host-a.log");
  expect(row?.textContent).not.toContain("Event ID");
});

test("根拠のレコードを選ぶと、元レコードを取得して原文を出す", async () => {
  const mock = stubFetch();

  render(<App />);

  expect(
    screen.queryByRole("heading", { level: 2, name: "開いたレコード" }),
  ).toBeNull();

  await findNodeList();
  // 同じ区画の別のビューを前面に出しておき、レコードを選ぶと開いたレコードが前面に戻ることを確かめる。
  openView(timelineView);
  await waitFor(() =>
    expect(screen.getByRole("tab", { name: "Record" })).toHaveAttribute(
      "aria-selected",
      "false",
    ),
  );
  openView("Nodes");
  await waitFor(() =>
    expect(screen.getByRole("tab", { name: "Nodes" })).toHaveAttribute(
      "aria-selected",
      "true",
    ),
  );
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );
  fireEvent.click(
    await screen.findByRole("button", {
      name: "Record に表示: 収集元: host-a.log、ID: 112、行: 1022",
    }),
  );

  await screen.findByRole("table", { name: "フィールド" });
  await waitFor(() =>
    expect(screen.getByRole("tab", { name: "Record" })).toHaveAttribute(
      "aria-selected",
      "true",
    ),
  );
  // 元レコードの表示と、開いたレコードの前後が、同じレコードを 1 回ずつ取得する。
  const recordPaths = () =>
    mock.mock.calls
      .map((call) => call[0])
      .filter((path) => path.startsWith("/api/v0/records"));
  await waitFor(() => expect(recordPaths()).toHaveLength(2));
  for (const path of recordPaths()) {
    expect(path).toBe(
      `/api/v0/records?sourceId=${hostALogSourceId}` +
        `&sourceContentSha256=${hostALogSha256}` +
        "&sequenceNumber=112&lineNumber=1022",
    );
  }
  expect(screen.getByText(hostALogRecordRawText)).toBeTruthy();
  // 開いているレコードにも分析者のメモを付けられる。
  const sheet = document.querySelector(".record-sheet");
  if (!(sheet instanceof HTMLElement)) throw new Error("no record sheet");
  expect(within(sheet).getByRole("heading", { name: "メモ" })).toBeTruthy();
  // 開いたレコードの欄は、原文と前後を並べ、閉じる操作で消える。
  expect(
    screen.getByRole("heading", { level: 2, name: "前後のレコード" }),
  ).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "レコードを閉じる" }));
  expect(
    screen.queryByRole("heading", { level: 2, name: "開いたレコード" }),
  ).toBeNull();
  expect(screen.queryByText(hostALogRecordRawText)).toBeNull();
});

/**
 * 関連付けを持つ関係の詳細を開き、最初の段階の最初の候補を起点付きで開く。
 * 経路を求める要求 (起点の項目を持つ要求) と、それ以外のレコードの要求の URL を返す関数を返す。
 */
async function openCandidateWithOrigin(
  recordJson: unknown = hostALogRecordResponseJson(),
) {
  const mock = stubFetch(
    sourcesResponseJson(),
    graphResponseJson(),
    recordJson,
    emptyAssertionsResponseJson(),
    matchedEdgeDetailResponseJson(),
  );
  render(<App />);
  const edgeTable = await findEdgeList();
  fireEvent.click(
    within(edgeTable).getAllByRole("button", {
      name: / の詳細を表示$/,
    })[0] as HTMLElement,
  );
  await waitFor(() =>
    expect(document.querySelector("details.match-stage")).not.toBeNull(),
  );
  const stage = document.querySelector(
    "details.match-stage",
  ) as HTMLDetailsElement;
  stage.open = true;
  fireEvent(stage, new Event("toggle"));
  fireEvent.click(
    within(stage).getAllByTitle(
      "元のレコードからの経路と一緒に Record に表示",
    )[0] as HTMLElement,
  );
  const recordUrls = (withOrigin: boolean) =>
    mock.mock.calls
      .map((call) => new URL(call[0], "http://localhost"))
      .filter(
        (url) =>
          url.pathname === "/api/v0/records" &&
          url.searchParams.has("originSourceId") === withOrigin,
      );
  return {
    trailUrls: () => recordUrls(true),
    plainRecordUrls: () => recordUrls(false),
  };
}

test("候補から開いたレコードは、起点と関連付けの条件を付けた要求で経路を取る", async () => {
  const requests = await openCandidateWithOrigin();

  await waitFor(() => expect(requests.trailUrls()).toHaveLength(1));
  const trailUrl = requests.trailUrls()[0];
  expect(trailUrl?.searchParams.get("sequenceNumber")).toBe("115");
  expect(trailUrl?.searchParams.get("originSequenceNumber")).toBe("112");
  expect(trailUrl?.searchParams.getAll("matchCondition")).toEqual(
    conditionKeys.map((key) =>
      key === "second_of_time" ? "second_of_time~0" : key,
    ),
  );
  // レコードの原文と項目は、起点なしの要求で取る。
  expect(
    requests
      .plainRecordUrls()
      .some((url) => url.searchParams.get("sequenceNumber") === "115"),
  ).toBe(true);
});

test("前後の欄から開いたレコードは、起点も関連付けの条件も付けずに取る", async () => {
  // 前後の欄は、絶対時刻で読める事象の時刻を持つレコードにだけ前後の行を出す。
  const record = hostALogRecordResponseJson() as { fields: unknown[] };
  const requests = await openCandidateWithOrigin({
    ...record,
    fields: [
      ...record.fields,
      {
        name: "time",
        semantic: "event.time",
        kind: "timestamp",
        timestamp: {
          rawText: "01/01/2021 10:00:00.500",
          normalized: "2021-01-01T10:00:00.500+09:00",
          normalizedForm: "rfc3339_absolute",
          precision: "millisecond",
          offsetState: "in_value",
          clock: "terminal_local",
          meaning: "event",
          valueState: "present",
        },
      },
    ],
  });
  await waitFor(() => expect(requests.trailUrls()).toHaveLength(1));

  const context = await screen.findByRole("region", { name: "レコードの前後" });
  fireEvent.click(
    (
      await within(context).findAllByRole("button", {
        name: /^Record に表示: /,
      })
    )[0] as HTMLElement,
  );

  await waitFor(() =>
    expect(
      requests
        .plainRecordUrls()
        .some((url) => url.searchParams.get("sequenceNumber") === "2204"),
    ).toBe(true),
  );
  expect(requests.trailUrls()).toHaveLength(1);
  expect(
    requests
      .plainRecordUrls()
      .every((url) => !url.searchParams.has("matchCondition")),
  ).toBe(true);
});

test("条件を適用し直すと、起点を保ったまま新しい条件で経路を取り直す", async () => {
  const requests = await openCandidateWithOrigin();
  await waitFor(() => expect(requests.trailUrls()).toHaveLength(1));

  chooseConditionKind("推定条件");
  fireEvent.click(screen.getByRole("checkbox", { name: "利用者" }));
  fireEvent.click(screen.getByRole("button", { name: "推定条件を適用" }));

  await waitFor(() => expect(requests.trailUrls()).toHaveLength(2));
  const reloaded = requests.trailUrls()[1];
  expect(reloaded?.searchParams.get("originSequenceNumber")).toBe("112");
  // 利用者だけを外した選択を、選択の順のまま送る。
  expect(reloaded?.searchParams.getAll("matchCondition")).toEqual(
    conditionKeys
      .filter((key) => key !== "user")
      .map((key) => (key === "second_of_time" ? "second_of_time~0" : key)),
  );
});

test("収集元とノードの表示名が持つ bidi 制御を可視の符号にして出す", async () => {
  stubFetch(
    invisibleFileNameSourcesResponseJson(),
    invisibleCharacterGraphResponseJson(),
  );

  render(<App />);

  openView(sourcesView);
  fireEvent.click(
    await screen.findByRole("button", {
      name: "access制御文字 U+202Egol.log",
    }),
  );
  expect(pairText("選択中の収集元")).toBe(
    "選択中の収集元: accessU+202Egol.log",
  );
  await findNodeList();
  expect(
    await screen.findByRole("button", {
      name: "端末 HOST-U+202EC の詳細を開く",
    }),
  ).toBeTruthy();
});

test("エッジを選ぶと関係の詳細を出し、区分とメモの欄を同じ画面に並べる", async () => {
  const mock = stubFetch();

  render(<App />);

  const edgeTable = await findEdgeList();
  fireEvent.click(
    within(edgeTable).getAllByRole("button", {
      name: / の詳細を表示$/,
    })[0] as HTMLElement,
  );

  // 操作 11 の区分と、その関係のメモの欄が出る。
  const groups = await screen.findByRole("list", { name: "根拠のグループ" });
  expect(groups.children.length).toBeGreaterThan(0);
  expect(
    screen.getAllByRole("heading", { name: "メモ" }).length,
  ).toBeGreaterThan(0);
  expect(
    Array.from(document.querySelectorAll(".value-pairs > li")).some(
      (item) => item.textContent === "メモ: 0",
    ),
  ).toBe(true);

  // 画面は操作 11 と所見の一覧の両方を呼ぶ。
  const paths = mock.mock.calls.map((call) => call[0]);
  expect(paths.some((path) => path.startsWith("/api/v0/edges/"))).toBe(true);
  expect(paths.some((path) => path.startsWith("/api/v0/assertions"))).toBe(
    true,
  );
});

test("エッジを選ぶと Node Detail の区画に Edge Detail を前面に出し、ノードを選ぶと Node Detail を前面に戻す", async () => {
  stubFetch();
  render(<App />);

  expect(screen.queryByRole("tab", { name: "Edge Detail" })).toBeNull();
  fireEvent.click(
    within(await findEdgeList()).getAllByRole("button", {
      name: / の詳細を表示$/,
    })[0] as HTMLElement,
  );
  await screen.findByRole("list", { name: "根拠のグループ" });
  const groupOf = (name: string) =>
    screen.getByRole("tab", { name }).closest(".dv-groupview");
  expect(groupOf("Edge Detail")).toBe(groupOf("Node Detail"));

  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );
  expect(await screen.findByRole("table", { name: "同一性" })).toBeTruthy();
  expect(screen.queryByRole("list", { name: "根拠のグループ" })).toBeNull();
});

test("Path の表でノードを押しても Path を前面に保ち、未適用の除外する根拠の選択を残す", async () => {
  const mock = stubFetch();
  const fallback = mock.getMockImplementation();
  mock.mockImplementation(async (input: string, init?: RequestInit) => {
    if (input.startsWith("/api/v0/influence-path")) {
      return jsonResponse(200, influencePathResponseJson());
    }
    if (input.startsWith(`/api/v0/nodes/${encodeURIComponent(ipNodeId)}`)) {
      return jsonResponse(200, {
        ...nodeDetailResponseJson(),
        node: graphNodeJson(ipNodeId),
      });
    }
    if (fallback === undefined) throw new Error("no fallback implementation");
    return fallback(input, init);
  });
  render(<App />);

  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );
  fireEvent.click(
    await screen.findByRole("button", { name: "影響の経路の起点に設定" }),
  );
  fireEvent.click(
    screen.getByRole("button", { name: /203\.0\.113\.21 の詳細を開く$/ }),
  );
  fireEvent.click(
    await screen.findByRole("button", {
      name: "影響の経路を表示: C:\\Windows\\System32\\cmd.exe から",
    }),
  );
  const table = await screen.findByRole("table", { name: "影響のエッジ" });
  const inferred = screen.getByLabelText<HTMLInputElement>("推定したエッジ");
  const chosen = !inferred.checked;
  fireEvent.click(inferred);
  const [origin] = within(table).getAllByRole("button", { pressed: false });
  if (origin === undefined) throw new Error("no node button");
  fireEvent.click(origin);

  await waitFor(() =>
    expect(within(table).getAllByRole("button", { pressed: true })).toContain(
      origin,
    ),
  );
  expect(screen.getByRole("tab", { name: "Path" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  openView("Node Detail");
  await waitFor(() =>
    expect(screen.queryByRole("table", { name: "影響のエッジ" })).toBeNull(),
  );
  openView("Path");
  expect(
    (await screen.findByLabelText<HTMLInputElement>("推定したエッジ")).checked,
  ).toBe(chosen);
});

test("Nodes と Edges の表の値を押すと、ノードは Node Detail、エッジは Edge Detail、時刻は Timeline に出す", async () => {
  stubFetch();
  render(<App />);

  const nodes = await findNodeList();
  fireEvent.click(
    within(nodes).getByRole("button", {
      name: "C:\\Windows\\System32\\cmd.exe",
    }),
  );
  expect(await screen.findByRole("table", { name: "同一性" })).toBeTruthy();

  const edges = await findEdgeList();
  const [edgeRow] = within(edges).getAllByRole("row").slice(1);
  if (edgeRow === undefined) throw new Error("no edge row");
  const [kindButton] = within(edgeRow).getAllByRole("button");
  if (kindButton === undefined) throw new Error("no edge kind button");
  fireEvent.click(kindButton);
  expect(
    await screen.findByRole("list", { name: "根拠のグループ" }),
  ).toBeTruthy();

  const time = within(await findEdgeList())
    .getAllByRole("button")
    .find((button) => /^\d{4}-\d{2}-\d{2}T/.test(button.textContent ?? ""));
  if (time === undefined) throw new Error("no time button");
  fireEvent.click(time);
  expect(
    await screen.findByRole("table", { name: "時刻順のレコード" }),
  ).toBeTruthy();
});

test("収集元の一覧の案件を選ぶと、グラフと時系列と関係の詳細の要求が同じ案件を載せる", async () => {
  const mock = stubFetch(casedSourcesResponseJson());

  render(<App />);

  await findNodeList();
  await waitFor(() =>
    expect(conditionValueOptions("案件")).toEqual([
      baselineCaseId,
      challengeCaseId,
    ]),
  );
  chooseConditionValue("案件", challengeCaseId);
  // 全件の時系列は前面に出したときだけ取得する。
  openView(timelineView);
  await screen.findByRole("table", { name: "収集元ごとの記録期間" });
  const edgeTable = await findEdgeList();
  fireEvent.click(
    within(edgeTable).getAllByRole("button", {
      name: / の詳細を表示$/,
    })[0] as HTMLElement,
  );
  await screen.findByRole("list", { name: "根拠のグループ" });

  const lastPathOf = (prefix: string) =>
    mock.mock.calls
      .map((call) => call[0])
      .filter((path) => path.startsWith(prefix))
      .at(-1);
  for (const prefix of [
    "/api/v0/graph",
    "/api/v0/timeline",
    "/api/v0/edges/",
  ]) {
    expect(lastPathOf(prefix)).toContain(`case=${challengeCaseId}`);
  }
});

test("案件を持つ収集元が無い一覧では、案件の欄を出さず要求に案件を載せない", async () => {
  const mock = stubFetch();

  render(<App />);

  openView(sourcesView);
  await screen.findByRole("button", { name: "access.log" });
  await findEdgeList();
  expect(screen.queryByLabelText("案件")).toBeNull();
  expect(mock.mock.calls.some((call) => call[0].includes("case="))).toBe(false);
});

test("時刻の解釈を記録すると収集元の一覧を取り直し、解釈で読んだ範囲を割当の欄に出す", async () => {
  const interpretation = { offset: "+09:00", assertionId: "as:recorded" };
  const localTime = (text: string) => ({
    rawText: text,
    normalized: text,
    normalizedForm: "local_without_offset",
    precision: "second",
    offsetState: "undetermined",
    clock: "terminal_local",
    meaning: "event",
    valueState: "present",
    interpretation,
  });
  const interpretedSources = sourcesResponseJson() as {
    sources: Record<string, unknown>[];
  };
  interpretedSources.sources[1] = {
    ...interpretedSources.sources[1],
    interpretedObservedRange: {
      from: localTime("2031-10-08T01:20:35"),
      to: localTime("2031-10-08T02:30:45"),
    },
  };
  const created = {
    assertion: {
      id: interpretation.assertionId,
      target: { kind: "source", sourceContentSha256: hostALogSha256 },
      state: "active",
      author: "analyst",
      recordedAt: "2031-10-09T00:00:00.000Z",
      basis: { note: "合成の根拠", recordRefs: [] },
      timeOffset: interpretation.offset,
      revisionNumber: 1,
      history: [],
    },
    targetOrigin: "observation",
  };
  let sourcesRequests = 0;
  const mock = vi.fn(async (input: string, init?: RequestInit) => {
    if (input.startsWith("/api/v0/sources")) {
      sourcesRequests += 1;
      return jsonResponse(
        200,
        sourcesRequests === 1 ? sourcesResponseJson() : interpretedSources,
      );
    }
    if (input.startsWith("/api/v0/assertions")) {
      return init?.method === "POST"
        ? jsonResponse(201, created)
        : jsonResponse(200, emptyAssertionsResponseJson());
    }
    if (input.startsWith("/api/v0/timeline")) {
      return jsonResponse(200, timelineResponseJson());
    }
    if (input.startsWith("/api/v0/event-kinds")) {
      return jsonResponse(200, eventKindsResponseJson());
    }
    return jsonResponse(200, graphResponseJson());
  });
  vi.stubGlobal("fetch", mock);

  render(<App />);
  openView(sourcesView);
  fireEvent.click(await screen.findByRole("button", { name: "host-a.log" }));
  openView(recordingView);
  const assignments = await screen.findByRole("region", {
    name: "端末の割り当て",
  });
  await within(assignments).findByText(/記録期間なし/);

  const interpretationRegion = screen.getByRole("region", {
    name: "収集元のタイムゾーン",
  });
  fireEvent.change(
    within(interpretationRegion).getByLabelText("タイムゾーン"),
    { target: { value: "+09:00" } },
  );
  fireEvent.change(within(interpretationRegion).getByLabelText("分析者"), {
    target: { value: "analyst" },
  });
  fireEvent.change(within(interpretationRegion).getByLabelText("メモ"), {
    target: { value: "合成の根拠" },
  });
  fireEvent.click(
    within(interpretationRegion).getByRole("button", {
      name: "タイムゾーンを記録",
    }),
  );

  const assignmentPairs = () =>
    Array.from(assignments.querySelectorAll(".value-pairs > li")).map(
      (item) => item.textContent ?? "",
    );
  await waitFor(() =>
    expect(assignmentPairs()).toContain(
      "適用期間: 2031-10-08T01:20:35 – 2031-10-08T02:30:45",
    ),
  );
  expect(assignmentPairs()).toContain("タイムゾーン: UTC+09:00");
  expect(assignmentPairs()).toContain("出どころ: 分析者の記録");
  // 一覧の取得、解釈の記録、一覧の取り直しの順に送る。
  expect(
    mock.mock.calls
      .filter(
        ([path, init]) =>
          path.startsWith("/api/v0/sources") ||
          (path.startsWith("/api/v0/assertions") && init?.method === "POST"),
      )
      .map(([path, init]) => `${init?.method ?? "GET"} ${path.split("?")[0]}`),
  ).toEqual([
    "GET /api/v0/sources",
    "POST /api/v0/assertions",
    "GET /api/v0/sources",
  ]);
});

test("区分を選ぶと、応答が返した値を操作 11 の query に載せる", async () => {
  const mock = stubFetch();

  render(<App />);

  const edgeTable = await findEdgeList();
  fireEvent.click(
    within(edgeTable).getAllByRole("button", {
      name: / の詳細を表示$/,
    })[0] as HTMLElement,
  );
  const groups = await screen.findByRole("list", { name: "根拠のグループ" });
  fireEvent.click(
    within(groups).getAllByRole("button", {
      name: "このグループの根拠を表示",
    })[0] as HTMLElement,
  );

  await screen.findByRole("list", { name: "根拠のグループ" });
  const narrowed = mock.mock.calls
    .map((call) => call[0])
    .filter((path) => path.startsWith("/api/v0/edges/"))
    .at(-1);
  expect(narrowed).toContain("eventCategory=net");
  expect(narrowed).toContain("eventAction=acpt");
  expect(narrowed).toContain("destinationPort=5985");
});

test("取り出した画面の状態を JSON にして別の画面へ適用すると、同じ状態が出る", async () => {
  stubFetch();
  const changes = vi.fn<(state: WorkspaceState) => void>();
  render(<App onWorkspaceStateChange={changes} />);
  await findNodeList();

  addCondition("含む", { 含む文字列: "cmd.exe" });
  addCondition("期間", { 始まりの時刻: "2031-10-08T10:20:35+09:00" });
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );
  await screen.findByRole("table", { name: "同一性" });
  const bookmark = await within(
    screen.getByRole("navigation", { name: "表示の履歴" }),
  ).findByRole("button", {
    name: "C:\\Windows\\System32\\cmd.exe をブックマークに追加",
  });
  fireEvent.click(bookmark);
  fireEvent.click(
    within(await findEdgeList()).getAllByRole("button", {
      name: / の詳細を表示$/,
    })[0] as HTMLElement,
  );
  await screen.findByRole("list", { name: "根拠のグループ" });
  // エッジの一覧は Bookmarks と同じ区画にある。Bookmarks を前面に戻した配置を保存させる。
  openView("Bookmarks");

  const saved = await waitFor(() => {
    const state = changes.mock.lastCall?.[0];
    expect(JSON.stringify(state?.dockLayout)).toContain(
      '"activeView":"bookmarks"',
    );
    expect(state?.selectedEdgeId).toBeDefined();
    expect(state?.bookmarks).toHaveLength(1);
    expect(state?.graph.selected?.kind).toBe("node");
    return JSON.parse(JSON.stringify(state));
  });
  expect(saved.searchTerms.contains).toEqual(["cmd.exe"]);
  expect(saved.recordFilter.timeFilter.from.text).toBe(
    "2031-10-08T10:20:35+09:00",
  );
  expect(saved.history.places).toHaveLength(1);
  const restored = decodeWorkspaceState(saved);
  cleanup();
  localStorage.clear();

  const restoredChanges = vi.fn<(state: WorkspaceState) => void>();
  render(
    <App
      workspaceState={{ state: restored, revision: 1 }}
      onWorkspaceStateChange={restoredChanges}
    />,
  );
  const { dockLayout: _layout, ...rest } = saved;
  await waitFor(() => {
    const { dockLayout, ...state } = JSON.parse(
      JSON.stringify(restoredChanges.mock.lastCall?.[0] ?? {}),
    );
    expect(state).toEqual(rest);
    expect(Object.keys(dockLayout.panels).sort()).toEqual(
      Object.keys(saved.dockLayout.panels).sort(),
    );
  });
  const chips = await screen.findByRole("list", {
    name: "適用している検索の条件",
  });
  expect(within(chips).getByText(/cmd\.exe/)).toBeTruthy();
  expect(
    within(chips).getByRole("button", {
      name: /^期間 2031-10-08T10:20:35\+09:00 –\s+· 秒単位 を削除$/,
    }),
  ).toBeTruthy();
  // 保存した配置は、エッジを選んだときに前面に出した Edge Detail を Node Detail の区画の前面に
  // 置いている。
  expect(
    await screen.findByRole("list", { name: "根拠のグループ" }),
  ).toBeTruthy();
  openView("Node Detail");
  expect(await screen.findByRole("table", { name: "同一性" })).toBeTruthy();
  // 保存した配置はブックマークのビューを前面に出している。
  expect(
    within(
      await screen.findByRole("region", { name: "ブックマーク" }),
    ).getByRole("button", { name: "C:\\Windows\\System32\\cmd.exe" }),
  ).toBeTruthy();
});

test("収集元のブックマークは、一覧にある収集元を Artifacts で選び、一覧に無い収集元は対象が無いことを出す", async () => {
  stubFetch();
  const changes = vi.fn<(state: WorkspaceState) => void>();
  render(<App onWorkspaceStateChange={changes} />);
  await findNodeList();
  const initial = await waitFor(() => {
    const state = changes.mock.lastCall?.[0];
    expect(state).toBeDefined();
    return state as WorkspaceState;
  });
  cleanup();

  render(
    <App
      workspaceState={{
        state: {
          ...initial,
          bookmarks: [
            {
              target: {
                kind: "source",
                source: { id: hostALogSourceId, label: "host-a.log" },
              },
              from: "source",
            },
            {
              target: {
                kind: "source",
                source: { id: "s:gone", label: "gone.log" },
              },
              from: "source",
            },
          ],
        },
        revision: 1,
      }}
    />,
  );
  await findNodeList();
  openView("Bookmarks");
  const table = await screen.findByRole("region", { name: "ブックマーク" });
  fireEvent.click(within(table).getByRole("button", { name: "gone.log" }));
  expect(within(table).getByText("解析結果になし")).toBeTruthy();

  fireEvent.click(within(table).getByRole("button", { name: "host-a.log" }));
  await waitFor(() =>
    expect(pairText("選択中の収集元")).toBe("選択中の収集元: host-a.log"),
  );
});

/**
 * AI 提案の一覧に items を返し、採用の要求に adoption を返す fetch の mock を置く。他の path は
 * stubFetch と同じ応答を返す。
 */
function stubProposals(items: unknown[], adoption: unknown, status = 201) {
  const mock = stubFetch();
  const base = mock.getMockImplementation();
  mock.mockImplementation(async (input: string, init?: RequestInit) => {
    if (input.startsWith("/api/v0/assist-proposals/")) {
      return jsonResponse(status, adoption);
    }
    if (input.startsWith("/api/v0/assist-proposals")) {
      return jsonResponse(200, assistProposalsResponseJson(items as never[]));
    }
    if (base === undefined) {
      throw new Error("stubFetch has no implementation");
    }
    return base(input, init);
  });
  return mock;
}

/** scope の中の「提案を採用」を押し、確認の dialog で採用を押す。 */
function adoptIn(scope: ReturnType<typeof within>) {
  fireEvent.click(scope.getByRole("button", { name: "提案を採用" }));
  fireEvent.click(
    within(screen.getByRole("alertdialog")).getByRole("button", {
      name: "提案を採用",
    }),
  );
}

/** AI 提案の未決の件数の組が count になるまで待つ。 */
async function pendingProposals(count: number) {
  await waitFor(() => expect(pairText("未決")).toBe(`未決: ${count}`));
}

function graphRequestCount(mock: ReturnType<typeof stubFetch>): number {
  return mock.mock.calls.filter((call) => call[0].startsWith("/api/v0/graph"))
    .length;
}

test("ノードの詳細で AI 提案を採用すると、所見をメモに出し、関係を足さない採用ではグラフを取り直さない", async () => {
  const proposed = proposedItemJson("ap:0001");
  const mock = stubProposals([proposed], adoptionJson(undefined, proposed));
  render(<App />);
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );
  const section = await screen.findByRole("region", { name: "AI 提案" });
  await pendingProposals(1);
  const graphRequests = graphRequestCount(mock);

  fireEvent.change(within(section).getByLabelText("分析者の名前"), {
    target: { value: "analyst-b" },
  });
  adoptIn(within(section));

  expect(
    await screen.findByRole("cell", { name: "AI 提案の採用" }),
  ).toBeTruthy();
  await pendingProposals(0);
  expect(graphRequestCount(mock)).toBe(graphRequests);
});

test("関係を足す AI 提案を採用すると、グラフを読む画面を取り直す", async () => {
  const relation = relationItemJson(true, "ap:0001");
  const mock = stubProposals([relation], adoptionJson(undefined, relation));
  render(<App />);
  await findNodeList();
  openView("AI Proposals");
  await pendingProposals(1);
  const graphRequests = graphRequestCount(mock);

  fireEvent.change(screen.getByLabelText("分析者の名前"), {
    target: { value: "analyst-b" },
  });
  adoptIn(screen);

  await waitFor(() =>
    expect(graphRequestCount(mock)).toBeGreaterThan(graphRequests),
  );
  await pendingProposals(0);
});

function requestCount(mock: ReturnType<typeof stubFetch>, prefix: string) {
  return mock.mock.calls.filter((call) => call[0].startsWith(prefix)).length;
}

/** 採用の要求に、別の操作が先に proposed を採用したことを返す 409 の本文。 */
function adoptedElsewhereJson(proposed: Parameters<typeof adoptionJson>[1]) {
  return {
    code: "assist_proposal_decided",
    message: "the proposal is already decided",
    conflict: adoptionJson(undefined, proposed).proposal,
  };
}

/** AI 提案のビューを開き、名前を入れて最初の提案を採用する。 */
async function adoptInProposalsView() {
  await findNodeList();
  openView("AI Proposals");
  await pendingProposals(1);
  fireEvent.change(screen.getByLabelText("分析者の名前"), {
    target: { value: "analyst-b" },
  });
  adoptIn(screen);
}

test("別の操作が先に採用していた提案は、所見の一覧を取り直し、関係を足さない提案ではグラフを取り直さない", async () => {
  const proposed = proposedItemJson("ap:0001");
  const mock = stubProposals([proposed], adoptedElsewhereJson(proposed), 409);
  render(<App />);
  await findNodeList();
  const assertionRequests = requestCount(mock, "/api/v0/assertions");
  const graphRequests = graphRequestCount(mock);

  await adoptInProposalsView();

  await pendingProposals(0);
  await waitFor(() =>
    expect(requestCount(mock, "/api/v0/assertions")).toBe(
      assertionRequests + 1,
    ),
  );
  expect(graphRequestCount(mock)).toBe(graphRequests);
});

test("別の操作が先に採用していた関係を足す提案は、グラフを読む画面も取り直す", async () => {
  const relation = relationItemJson(true, "ap:0001");
  const mock = stubProposals([relation], adoptedElsewhereJson(relation), 409);
  render(<App />);
  await findNodeList();
  const graphRequests = graphRequestCount(mock);

  await adoptInProposalsView();

  await waitFor(() =>
    expect(graphRequestCount(mock)).toBeGreaterThan(graphRequests),
  );
  await pendingProposals(0);
});

test("作業場所に無いビューを持つ配置を、画面の状態として読まない", async () => {
  stubFetch();
  const changes = vi.fn<(state: WorkspaceState) => void>();
  render(<App onWorkspaceStateChange={changes} />);
  await findNodeList();
  const saved = JSON.parse(JSON.stringify(changes.mock.lastCall?.[0]));
  expect(() => decodeWorkspaceState(saved)).not.toThrow();
  saved.dockLayout.panels.unknownView = saved.dockLayout.panels.search;
  expect(() => decodeWorkspaceState(saved)).toThrow(DecodeFailure);
});
