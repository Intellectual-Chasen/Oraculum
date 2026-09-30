// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import {
  findNodeList,
  lastFigureRequest,
} from "@/features/graphExplore/graphExploreTestHarness";
import { emptyAssertionsResponseJson } from "@/testdata/assertions/assertionsResponse";
import { addCondition, chooseConditionKind } from "@/testdata/conditionInput";
import { eventKindsResponseJson } from "@/testdata/eventKinds/eventKindsResponse";
import {
  edgeDetailResponseJson,
  graphResponseJson,
  ipNodeId,
  nodeDetailResponseJson,
  processNodeId,
} from "@/testdata/graph/graphResponse";
import { jsonResponse } from "@/testdata/http";
import { hostALogRecordResponseJson } from "@/testdata/records/recordResponse";
import {
  accessLogSourceId,
  hostALogSourceId,
  sourcesResponseJson,
} from "@/testdata/sources/sourcesResponse";
import {
  timelineResponseJson,
  timelineTerminalNodeId,
} from "@/testdata/timeline/timelineResponse";
import { App } from "./App";

// WebGL の renderer は jsdom で動かない。画面の組み立てを確かめる本 test は図を
// stub に置き換える。実描画はブラウザーで確かめる。
vi.mock("@/features/graphExplore/SubgraphCanvas", () => ({
  SubgraphCanvas: () => <p>部分グラフの図</p>,
}));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  // ビューの配置は localStorage に残る。前の test の配置で次の test を始めない。
  localStorage.clear();
});

/** path ごとに、画面が呼ぶ操作の応答を返す。 */
function stubFetch() {
  const mock = vi.fn(async (input: string, _init?: RequestInit) => {
    if (input.startsWith("/api/v0/sources")) {
      return jsonResponse(200, sourcesResponseJson());
    }
    if (input.startsWith("/api/v0/timeline")) {
      return jsonResponse(200, timelineResponseJson());
    }
    if (input.startsWith("/api/v0/event-kinds")) {
      return jsonResponse(200, eventKindsResponseJson());
    }
    if (input.startsWith("/api/v0/records")) {
      return jsonResponse(200, hostALogRecordResponseJson());
    }
    if (input.startsWith("/api/v0/record-graph")) {
      return jsonResponse(200, {
        recordRef: (hostALogRecordResponseJson() as { recordRef: unknown })
          .recordRef,
        nodeIds: [processNodeId],
        edgeIds: [],
      });
    }
    if (input.startsWith("/api/v0/nodes/")) {
      return jsonResponse(200, nodeDetailResponseJson());
    }
    if (input.startsWith("/api/v0/edges/")) {
      return jsonResponse(200, edgeDetailResponseJson());
    }
    if (input.startsWith("/api/v0/assertions")) {
      return jsonResponse(200, emptyAssertionsResponseJson());
    }
    return jsonResponse(200, graphResponseJson());
  });
  vi.stubGlobal("fetch", mock);
  return mock;
}

const processName = "プロセス C:\\Windows\\System32\\cmd.exe";

/** メニューバーの見出しを押してメニューを開く。 */
function openMenu(heading: string) {
  const bar = screen.getByRole("menubar", { name: "画面の操作" });
  fireEvent.click(within(bar).getByRole("menuitem", { name: heading }));
  return screen.getByRole("menu", { name: heading });
}

function menuItem(name: string) {
  return screen.getByRole("menuitem", { name });
}

function choose(heading: string, item: string) {
  openMenu(heading);
  fireEvent.click(menuItem(item));
}

/** 見出しのメニューを開いて項目が使えるかを読み、メニューを閉じてから確かめる。 */
function expectDisabled(heading: string, item: string, disabled: boolean) {
  const menu = openMenu(heading);
  const isDisabled =
    within(menu)
      .getByRole("menuitem", { name: item })
      .getAttribute("aria-disabled") === "true";
  fireEvent.keyDown(menu, { key: "Escape" });
  expect(isDisabled).toBe(disabled);
}

async function expectSelectedTab(name: string, selected: boolean) {
  await waitFor(() =>
    expect(screen.getByRole("tab", { name })).toHaveAttribute(
      "aria-selected",
      String(selected),
    ),
  );
}

async function openRecordFromNodeDetail() {
  fireEvent.click(
    await screen.findByRole("button", { name: `${processName} の詳細を開く` }),
  );
  fireEvent.click(
    await screen.findByRole("button", {
      name: "Record に表示: 収集元: host-a.log、ID: 112、行: 1022",
    }),
  );
  await screen.findByRole("table", { name: "フィールド" });
}

test("メニューバーは表示・検索・移動・記録を持ち、表示は作業場所のすべてのビューを挙げる", async () => {
  stubFetch();
  render(<App />);
  await findNodeList();

  const bar = screen.getByRole("menubar", { name: "画面の操作" });
  expect(
    within(bar)
      .getAllByRole("menuitem")
      .map((heading) => heading.textContent),
  ).toEqual(["表示", "検索", "移動", "記録"]);

  const view = openMenu("表示");
  const viewItems = within(view)
    .getAllByRole("menuitem")
    .map((item) => item.textContent ?? "")
    .filter((name) => name !== "配置を初期化");
  const tabs = screen.getAllByRole("tab").map((tab) => tab.textContent ?? "");
  // Edge Detail と Path は既定の配置に置かず、開くときに Node Detail の区画に加わる。
  expect([...viewItems].sort()).toEqual(
    [...tabs, "Edge Detail", "Path"].sort(),
  );
});

test("表示の項目はビューを前面に出し、配置を初期化は前面のビューを既定に戻す", async () => {
  stubFetch();
  render(<App />);
  await findNodeList();

  choose("表示", "Histogram");
  await expectSelectedTab("Histogram", true);
  await expectSelectedTab("Record", false);

  choose("表示", "配置を初期化");
  await expectSelectedTab("Record", true);
  await expectSelectedTab("Histogram", false);

  choose("記録", "Time & Host を表示");
  await expectSelectedTab("Time & Host", true);
});

test("文字列の条件をすべて解除する項目は、文字列を追加するまで使えず、追加した文字列と組と欄をまとめて解除する", async () => {
  const mock = stubFetch();
  render(<App />);
  await findNodeList();
  const clearTerms = "文字列の条件をすべて解除";
  expectDisabled("検索", clearTerms, true);

  addCondition("含む", { 含む文字列: "example" });
  addCondition("フィールドの値が文字列を含む", {
    フィールド: "CommandLine",
    値: "tool",
  });
  addCondition("フィールドを指定", {
    文字列を探すフィールド: "CommandLine",
  });
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toContain("valueField=CommandLine"),
  );
  expect(lastFigureRequest(mock)).toContain("valueContains=example");
  expect(lastFigureRequest(mock)).toContain("fieldContains=");
  expectDisabled("検索", clearTerms, false);

  choose("検索", clearTerms);
  expect(
    screen.queryByRole("list", { name: "適用している検索の条件" }),
  ).toBeNull();
  await waitFor(() =>
    expect(lastFigureRequest(mock)).not.toContain("valueContains"),
  );
  expect(lastFigureRequest(mock)).not.toContain("fieldContains");
  expect(lastFigureRequest(mock)).not.toContain("valueField");
});

test("検索式を解除する項目は式を適用したときだけ使え、文字列の条件を解除する項目は検索式を残す", async () => {
  const mock = stubFetch();
  render(<App />);
  await findNodeList();
  expectDisabled("検索", "検索式を解除", true);

  addCondition("検索式", { 検索式: "LogonType == 3" });
  addCondition("含む", { 含む文字列: "example" });
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toContain("valueContains=example"),
  );
  expect(lastFigureRequest(mock)).toContain("searchExpression=");
  expectDisabled("検索", "検索式を解除", false);

  // 文字列の条件を解除しても、検索式は要求に残る。
  choose("検索", "文字列の条件をすべて解除");
  await waitFor(() =>
    expect(lastFigureRequest(mock)).not.toContain("valueContains"),
  );
  expect(lastFigureRequest(mock)).toContain("searchExpression=");

  choose("検索", "検索式を解除");
  await waitFor(() =>
    expect(lastFigureRequest(mock)).not.toContain("searchExpression="),
  );
  // 外した後に検索式の値の入力を開くと、空の欄から始める。
  chooseConditionKind("検索式");
  expect(screen.getByLabelText("検索式")).toHaveValue("");
});

test("期間のフィルタを解除する項目は、期間を適用したときだけ使え、要求から期間を外す", async () => {
  const mock = stubFetch();
  render(<App />);
  await findNodeList();
  expectDisabled("検索", "期間のフィルタを解除", true);

  addCondition("期間", { 始まりの時刻: "2031-10-08T10:20:35+09:00" });
  await waitFor(() => expect(lastFigureRequest(mock)).toContain("timeFrom="));
  expectDisabled("検索", "期間のフィルタを解除", false);

  choose("検索", "期間のフィルタを解除");
  await waitFor(() =>
    expect(lastFigureRequest(mock)).not.toContain("timeFrom="),
  );
});

test("探索を終了する項目は、関係先を出している間だけ使え、検索の結果へ戻す", async () => {
  const mock = stubFetch();
  render(<App />);
  await findNodeList();
  const leave = "探索を終了";
  expectDisabled("検索", leave, true);

  fireEvent.click(
    screen.getByRole("button", {
      name: `${processName} の隣接ノードを追加`,
    }),
  );
  await screen.findByText("隣接ノードの表示元");
  expectDisabled("検索", leave, false);

  choose("検索", leave);
  await waitFor(() =>
    expect(screen.queryByText("隣接ノードの表示元")).toBeNull(),
  );
  expect(lastFigureRequest(mock)).not.toContain("nodeId=");
  expect(
    screen.getByRole("button", { name: "隣接ノードの表示に戻る" }),
  ).toBeInTheDocument();
});

test("戻ると進むは見た場所の履歴を移り、端の側では使えない", async () => {
  const mock = stubFetch();
  render(<App />);
  await findNodeList();
  expectDisabled("移動", "戻る", true);
  expectDisabled("移動", "進む", true);

  fireEvent.click(
    screen.getByRole("button", { name: `${processName} の詳細を開く` }),
  );
  fireEvent.click(
    screen.getByRole("button", {
      name: "IP アドレス 203.0.113.21 の詳細を開く",
    }),
  );
  // **待つ間にメニューを開かない。** waitFor は DOM の変化で callback を呼び直すため、
  // callback がメニューを開き閉じすると止まらない。帯の button で履歴の更新を待つ。
  const nav = screen.getByRole("navigation", { name: "表示の履歴" });
  await waitFor(() =>
    expect(
      within(nav).getByRole("button", { name: "戻る" }),
    ).not.toHaveAttribute("aria-disabled"),
  );
  expectDisabled("移動", "戻る", false);

  choose("移動", "戻る");
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toContain(
      `nodeId=${encodeURIComponent(processNodeId)}`,
    ),
  );
  expectDisabled("移動", "進む", false);

  choose("移動", "進む");
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toContain(
      `nodeId=${encodeURIComponent(ipNodeId)}`,
    ),
  );
  expectDisabled("移動", "進む", true);
});

test("レコードを閉じる項目は、レコードを開いている間だけ使え、開いたレコードを閉じる", async () => {
  stubFetch();
  render(<App />);
  await findNodeList();
  const close = "レコードを閉じる";
  expectDisabled("移動", close, true);

  await openRecordFromNodeDetail();
  expectDisabled("移動", close, false);
  choose("移動", close);

  await waitFor(() =>
    expect(
      Array.from(document.querySelectorAll(".value-pairs > li")).some(
        (item) => item.textContent === "レコード: 未選択",
      ),
    ).toBe(true),
  );
  expect(screen.queryByRole("table", { name: "フィールド" })).toBeNull();
});

test("開いたレコードのフィールドのメニューから、含まない文字列とフィールドの値の条件を検索の条件に追加する", async () => {
  const mock = stubFetch();
  render(<App />);
  await findNodeList();
  await openRecordFromNodeDetail();

  fireEvent.click(
    screen.getByRole("button", { name: "フィールド evt の操作" }),
  );
  fireEvent.click(menuItem("含まない文字列に追加"));
  expect(
    await screen.findByRole("button", { name: "含まない文字列 net を削除" }),
  ).toBeInTheDocument();

  fireEvent.click(
    screen.getByRole("button", { name: "フィールド evt の操作" }),
  );
  fireEvent.click(menuItem("フィールドの値が文字列を含む条件に追加"));
  expect(
    await screen.findByRole("button", {
      name: "フィールドと文字列 evt=net を削除",
    }),
  ).toBeInTheDocument();

  fireEvent.click(
    screen.getByRole("button", { name: "フィールド evt の操作" }),
  );
  fireEvent.click(menuItem("フィールドの値が等しい条件に追加"));
  await waitFor(() => {
    const request = new URL(lastFigureRequest(mock) ?? "", "http://127.0.0.1");
    expect(request.searchParams.getAll("valueExcludes")).toEqual(["net"]);
    expect(request.searchParams.getAll("fieldEquals")).toEqual(["evt=net"]);
  });
});

test("根拠のレコードのイベントの種類の値のメニューから、分類と動作の組をイベントの種類の条件に追加する", async () => {
  const mock = stubFetch();
  render(<App />);
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", { name: `${processName} の詳細を開く` }),
  );
  const [value] = await screen.findAllByRole("button", {
    name: "値の操作: evt net",
  });
  fireEvent.click(value as HTMLElement);
  fireEvent.click(menuItem("イベントの種類の条件に追加"));
  await waitFor(() => {
    const request = new URL(lastFigureRequest(mock) ?? "", "http://127.0.0.1");
    expect(request.searchParams.get("eventCategory")).toBe("net");
    expect(request.searchParams.get("eventAction")).not.toBeNull();
  });
  // 分類と動作の組は、検索の条件の chip として表示する。
  expect(
    await screen.findByRole("button", { name: / net \/ .+ を削除$/ }),
  ).toBeInTheDocument();
});

test("時系列の行のメニューから、行の端末と収集元で根拠のレコードにフィルタを適用する", async () => {
  const mock = stubFetch();
  render(<App />);
  await findNodeList();
  choose("表示", "Timeline");
  const rowMenu = "収集元: host-a.log、ID: 2204、行: 1022 の操作";

  fireEvent.click(await screen.findByRole("button", { name: rowMenu }));
  fireEvent.click(menuItem("この端末でフィルタ"));
  expect(
    await screen.findByRole("button", { name: "Host HOST-C を削除" }),
  ).toBeInTheDocument();
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toContain(
      `terminal=${encodeURIComponent(timelineTerminalNodeId)}`,
    ),
  );

  fireEvent.click(await screen.findByRole("button", { name: rowMenu }));
  fireEvent.click(menuItem("この収集元でフィルタ"));
  expect(
    await screen.findByRole("button", { name: "Artifact host-a.log を削除" }),
  ).toBeInTheDocument();
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toContain(
      `source=${encodeURIComponent(hostALogSourceId)}`,
    ),
  );
});

test("収集元の一覧の行のメニューから、その収集元だけのフィルタを根拠のレコードに適用し、適用した後はその項目を使えなくする", async () => {
  const mock = stubFetch();
  render(<App />);
  await findNodeList();
  choose("表示", "Artifacts");
  const rowMenu = "収集元 access.log の操作";
  const narrow = "この収集元のフィルタを適用";

  fireEvent.click(await screen.findByRole("button", { name: rowMenu }));
  fireEvent.click(menuItem(narrow));
  expect(
    await screen.findByRole("button", { name: "Artifact access.log を削除" }),
  ).toBeInTheDocument();
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toContain(
      `source=${encodeURIComponent(accessLogSourceId)}`,
    ),
  );

  fireEvent.click(screen.getByRole("button", { name: rowMenu }));
  expect(menuItem(narrow)).toHaveAttribute("aria-disabled", "true");
});
