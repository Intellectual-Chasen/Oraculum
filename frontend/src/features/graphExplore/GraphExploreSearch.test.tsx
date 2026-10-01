// @vitest-environment jsdom
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { maxGraphDepth } from "@/shared/api/graph";
import {
  addCondition,
  chooseConditionKind,
  chooseConditionValue,
  conditionValueOptions,
} from "@/testdata/conditionInput";
import { eventKindsResponseJson } from "@/testdata/eventKinds/eventKindsResponse";
import {
  accountNodeDetailResponseJson,
  accountNodeId,
  accountRecordGraphResponseJson,
  graphResponseJson,
  nodeDetailResponseJson,
  processNodeId,
  terminalIdText,
  terminalNodeId,
} from "@/testdata/graph/graphResponse";
import { jsonResponse } from "@/testdata/http";
import { apiErrorJson } from "@/testdata/sources/sourcesResponse";
import { ValueActionsForTest } from "@/testdata/valueActions";
import {
  type FetchMock,
  figureRequests,
  findNodeList,
  GraphExploreHarness,
  GraphExploreWithTerminals,
  initialFigureRequest,
  lastFigureRequest,
  limitQuery,
  matchConditionQuery,
  objectViewQuery,
  otherTerminalNodeId,
  renderGraphExplore,
  stackPanes,
  stubFetch,
  terminalListPrefix,
  terminalListResponseJson,
} from "./graphExploreTestHarness";
import { edgeKindLabels } from "./labels";

// WebGL の renderer は jsdom で動かない。本 test は図を stub に置き換え、検索の条件と
// 要求と一覧と詳細を確かめる。
vi.mock("./SubgraphCanvas", () => ({
  SubgraphCanvas: () => <p>部分グラフの図</p>,
}));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

/** 親子の連鎖で、fixture のプロセスの親にあたるプロセス。 */
const parentProcessNodeId = "n:process:9f10";

/** 適用している検索の条件の、外す button の名前を並びの順に返す。条件が無いときは空を返す。 */
function appliedConditionNames(): string[] {
  const list = screen.queryByRole("list", { name: "適用している検索の条件" });
  if (list === null) {
    return [];
  }
  // harness は推定条件を 1 つだけ選んで渡すため、その chip を常に持つ。数えない。
  return within(list)
    .getAllByRole("button")
    .map((button) => button.getAttribute("aria-label") ?? "")
    .filter((name) => !name.startsWith("推定条件"));
}

/** 「名前: 値」の組の並びの li の文字列を返す。chip の li は含めない。 */
function valuePairTexts(container: ParentNode = document): string[] {
  return Array.from(
    container.querySelectorAll("ul.value-pairs > li"),
    (item) => item.textContent ?? "",
  );
}

/** ノードの種類の組が書く、今出している種類の文字列を返す。 */
function viewNote(): string | undefined {
  const text = valuePairTexts().find((pair) =>
    pair.startsWith("ノードの種類: "),
  );
  return text?.replace(/^ノードの種類: (自動: )?/, "");
}

/** Search の結果の、応答が用いた条件の組を返す。 */
function responseConditions(): string[] {
  return valuePairTexts(document.querySelector(".result-pane") ?? document);
}

/** ノードの種類の候補から、図に出す種類を追加する。 */
function addViewKind(label: string) {
  chooseConditionValue("ノードの種類", label);
}

/** 自動が選んだ端末と IP アドレスを外し、label の種類だけを図に出す。 */
function onlyViewKind(label: string) {
  addViewKind(label);
  for (const auto of ["端末", "IP アドレス"]) {
    fireEvent.click(
      screen.getByRole("button", { name: `ノードの種類 ${auto} を削除` }),
    );
  }
}

function addTerm(text: string, kind: "含む" | "含まない") {
  addCondition(kind, { [`${kind}文字列`]: text });
}

/** イベントの種類の候補から、分類と動作の組を選ぶ。動作を与えないときは分類のすべてを選ぶ。 */
function chooseEventKind(category: string, action?: string) {
  chooseConditionValue(
    "イベントの種類",
    action === undefined ? `${category} のすべて` : `${category} / ${action}`,
  );
}

/** 図と一覧を出し、事象の種別の候補を読み込んだ画面を描き、fetch の mock を返す。 */
async function renderLoaded(mock: FetchMock = stubFetch()) {
  renderGraphExplore();
  await findNodeList();
  await waitFor(() =>
    expect(
      mock.mock.calls.some((call) =>
        String(call[0]).startsWith("/api/v0/event-kinds"),
      ),
    ).toBe(true),
  );
  // 事象の種別と端末の候補を読み込み終えるまで待つ。
  await act(async () => {});
  return mock;
}

test("アカウントから相手候補を開くと、Graph が個別レコードを初回に描かず役割付き集計を取得する", async () => {
  const mock = stubFetch(
    accountRecordGraphResponseJson(),
    accountNodeDetailResponseJson(),
  );
  const onShowAccountRecords = vi.fn();
  const account = {
    id: accountNodeId,
    label: "user02",
    kind: "account" as const,
  };
  render(
    <ValueActionsForTest onSelectRecord={() => {}}>
      <GraphExploreHarness
        focusRequest={{ node: account }}
        onShowAccountRecords={onShowAccountRecords}
        onSelectRecord={() => {}}
        selectedEdgeId={undefined}
        onSelectEdge={() => {}}
        assertions={() => null}
        layout={stackPanes}
      />
    </ValueActionsForTest>,
  );

  fireEvent.click(
    await screen.findByRole("button", {
      name: "このアカウントを名指した記録",
    }),
  );

  await waitFor(() => {
    const request = mock.mock.calls.find(([input]) =>
      String(input).startsWith("/api/v0/account-relations?"),
    );
    expect(request).toBeDefined();
    const url = new URL(String(request?.[0]), "http://localhost");
    expect(url.pathname).toBe("/api/v0/account-relations");
    expect(url.searchParams.getAll("edgeKind")).toEqual([
      "record_subject_account",
      "record_target_account",
      "record_names_object",
    ]);
    expect(url.searchParams.get("accountNodeId")).toBe(accountNodeId);
    expect(url.searchParams.get("counterpartId")).toBeNull();
  });
  expect(
    screen.getByRole("img", { name: /0 組の相手への関係図/ }),
  ).toBeInTheDocument();
  expect(onShowAccountRecords).toHaveBeenCalledWith(account);
});

test("条件が無い最初のグラフは端末と IP アドレスを出し、グラフに出す対象の自動が端末と IP アドレスを選んだことを出す", async () => {
  const mock = await renderLoaded();

  expect(figureRequests(mock)).toEqual([initialFigureRequest]);
  expect(appliedConditionNames()).toEqual([]);
  expect(valuePairTexts()).toContain("ノードの種類: 自動: 端末、IP アドレス");
});

test("含む文字列を足すと、要求の valueContains に載せ、値の入力を閉じる", async () => {
  const mock = await renderLoaded();

  addTerm(" example-note.html ", "含む");

  // 前後の空白を外した文字列を載せる。
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}&depth=1` +
        "&valueContains=example-note.html" +
        matchConditionQuery,
    ),
  );
  expect(appliedConditionNames()).toEqual([
    "含む文字列 example-note.html を削除",
  ]);
  expect(screen.queryByLabelText("含む文字列")).toBeNull();
});

test("検索する文字列が空の追加では、文字列を要求に載せず、取り直さない", async () => {
  const mock = await renderLoaded();
  const before = figureRequests(mock);

  addTerm("   ", "含む");
  addTerm("   ", "含まない");

  expect(figureRequests(mock)).toEqual(before);
  expect(lastFigureRequest(mock)).toBe(initialFigureRequest);
  expect(appliedConditionNames()).toEqual([]);
});

test("含む文字列 2 つと含まない文字列 1 つを繰り返しの項目で送り、外した文字列を次の要求から外す", async () => {
  const mock = await renderLoaded();

  addTerm("203.0.113.15", "含む");
  addTerm("HOST-D", "含む");
  addTerm("svchost.exe", "含まない");

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}&depth=1` +
        "&valueContains=203.0.113.15&valueContains=HOST-D" +
        "&valueExcludes=svchost.exe" +
        matchConditionQuery,
    ),
  );
  expect(appliedConditionNames()).toEqual([
    "含む文字列 203.0.113.15 を削除",
    "含む文字列 HOST-D を削除",
    "含まない文字列 svchost.exe を削除",
  ]);
  // 文字列だけの条件では、レコード以外のすべての種類を出す。
  expect(viewNote()).toBe("レコード以外のすべて");

  fireEvent.click(
    screen.getByRole("button", { name: "含む文字列 HOST-D を削除" }),
  );

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}&depth=1` +
        "&valueContains=203.0.113.15&valueExcludes=svchost.exe" +
        matchConditionQuery,
    ),
  );
  expect(appliedConditionNames()).toEqual([
    "含む文字列 203.0.113.15 を削除",
    "含まない文字列 svchost.exe を削除",
  ]);

  // 条件をすべて外すと、端末と IP アドレスを出す図へ戻る。
  fireEvent.click(
    screen.getByRole("button", { name: "含む文字列 203.0.113.15 を削除" }),
  );
  fireEvent.click(
    screen.getByRole("button", { name: "含まない文字列 svchost.exe を削除" }),
  );

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(initialFigureRequest),
  );
  expect(appliedConditionNames()).toEqual([]);
  expect(viewNote()).toBe("端末、IP アドレス");
});

test("Host を選ぶと terminal を送って条件の一覧に表示名を出し、chip を外すと外す", async () => {
  const mock = await renderLoaded(
    stubFetch({ ...graphResponseJson(), terminal: otherTerminalNodeId }),
  );
  // 画面が端末を選んでいないときは、応答が用いた端末を識別子で出す。
  expect(responseConditions()).toContain(`Host: ${otherTerminalNodeId}`);
  expect(responseConditions()).toContain("ホップ数: 1");

  expect(conditionValueOptions("Host")).toStrictEqual(["HOST-C", "HOST-D"]);

  chooseConditionValue("Host", "HOST-D");

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}&depth=1` +
        `&terminal=${encodeURIComponent(otherTerminalNodeId)}` +
        matchConditionQuery,
    ),
  );
  expect(appliedConditionNames()).toEqual(["Host HOST-D を削除"]);
  await waitFor(() => expect(responseConditions()).toContain("Host: HOST-D"));

  fireEvent.click(screen.getByRole("button", { name: "Host HOST-D を削除" }));

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(initialFigureRequest),
  );
  expect(appliedConditionNames()).toEqual([]);
});

test("参照だけの端末は、置いたレコードを持たないため、Host の候補に出さない", async () => {
  const listed = terminalListResponseJson();
  const [terminal] = listed.nodes;
  const referencedTerminal = {
    ...terminal,
    id: "n:terminal:9c1d",
    identity: [{ semantic: "terminal.id", value: "HOST-E-TMID" }],
    label: { rawText: "HOST-E", valueState: "present" },
    observation: "referenced",
  };
  await renderLoaded(
    stubFetch(graphResponseJson(), nodeDetailResponseJson(), {
      ...listed,
      nodes: [...listed.nodes, referencedTerminal],
      nodeCount: 3,
      matchedKinds: [{ kind: "terminal", count: 3 }],
    }),
  );

  expect(conditionValueOptions("Host")).toEqual(["HOST-C", "HOST-D"]);
});

test("端末の一覧を取れないとき、Host の値の入力に失敗を出し、図は出す", async () => {
  await renderLoaded(
    stubFetch(
      graphResponseJson(),
      nodeDetailResponseJson(),
      jsonResponse(500, apiErrorJson("internal_error")),
    ),
  );

  chooseConditionKind("Host");
  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain("端末の一覧の取得");
  expect(screen.queryByRole("combobox", { name: "Host" })).toBeNull();
  expect(screen.getByText("部分グラフの図")).toBeTruthy();
});

test("Windows イベントログの組を選ぶと、適用している条件と応答が用いた条件をプロバイダとイベント ID の名前で出す", async () => {
  await renderLoaded(
    stubFetch(
      {
        ...graphResponseJson(),
        eventCategory: "Example-Provider",
        eventAction: "42",
      },
      nodeDetailResponseJson(),
      terminalListResponseJson(),
      {
        kinds: [
          {
            category: "Example-Provider",
            action: "42",
            recordCount: 3,
            windowsEvent: true,
          },
        ],
        uncategorizedRecordCount: 0,
      },
    ),
  );

  chooseConditionValue("イベントの種類", "イベント ID 42");

  await waitFor(() =>
    expect(appliedConditionNames()).toEqual([
      "プロバイダ / イベント ID Example-Provider / 42 を削除",
    ]),
  );
  await waitFor(() =>
    expect(responseConditions()).toEqual(
      expect.arrayContaining([
        "プロバイダ: Example-Provider",
        "イベント ID: 42",
        "ホップ数: 1",
      ]),
    ),
  );
});

test("イベントの種類を選ぶと、その条件を要求に載せ、適用している条件に出す", async () => {
  const mock = await renderLoaded();

  chooseEventKind("file", "create");

  // 粒度を選ぶ対応に無い分類は、レコードを除くすべての対象を出す。
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}&depth=1` +
        "&eventCategory=file&eventAction=create" +
        matchConditionQuery,
    ),
  );
  expect(appliedConditionNames()).toEqual([
    "イベントの分類 / イベントの動作 file / create を削除",
  ]);

  // 分類のすべてを選び直すと、動作を外して分類を残す。
  chooseEventKind("file");
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}&depth=1` +
        "&eventCategory=file" +
        matchConditionQuery,
    ),
  );
});

test("イベント ID の範囲だけを適用しても対象の粒度に切り替え、範囲を要求と条件の一覧に出す", async () => {
  const mock = await renderLoaded();

  addCondition("イベント ID の範囲", {
    "イベント ID の最小": "8000",
    "イベント ID の最大": "8999",
  });

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}&depth=1` +
        "&eventActionFrom=8000&eventActionTo=8999" +
        matchConditionQuery,
    ),
  );
  expect(appliedConditionNames()).toEqual([
    "イベント ID の範囲 8000–8999 を削除",
  ]);
});

test("文字列を探すフィールドは文字列がある間だけ要求に載せ、応答が用いた条件に出す", async () => {
  const mock = await renderLoaded(
    stubFetch(
      {
        ...graphResponseJson(),
        valueContains: ["example"],
        valueField: "CommandLine",
        eventActionFrom: 8000,
      },
      nodeDetailResponseJson(),
      terminalListResponseJson(),
      eventKindsResponseJson(),
    ),
  );

  addCondition("フィールドを指定", { 文字列を探すフィールド: "CommandLine" });
  expect(lastFigureRequest(mock)).not.toContain("valueField");

  addTerm("example", "含む");
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toContain(
      "&valueContains=example&valueField=CommandLine",
    ),
  );
  await waitFor(() =>
    expect(responseConditions()).toEqual(
      expect.arrayContaining([
        "イベント ID の範囲: 8000–",
        "文字列を探すフィールド: CommandLine",
      ]),
    ),
  );
});

test("フィールドと値の組を 2 つ追加すると、両方を要求に載せ、組ごとに外せる", async () => {
  const mock = await renderLoaded(
    stubFetch(
      {
        ...graphResponseJson(),
        fieldContains: ["TargetUserName=user-a", "LogonType=3"],
      },
      nodeDetailResponseJson(),
      terminalListResponseJson(),
      eventKindsResponseJson(),
    ),
  );
  const addPair = (field: string, term: string) =>
    addCondition("フィールドの値が文字列を含む", {
      フィールド: field,
      値: term,
    });

  addPair("TargetUserName", "user-a");
  addPair("LogonType", "3");
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toContain(
      "&fieldContains=TargetUserName%3Duser-a&fieldContains=LogonType%3D3",
    ),
  );
  await waitFor(() =>
    expect(responseConditions()).toEqual(
      expect.arrayContaining([
        "フィールドと文字列: TargetUserName=user-a",
        "フィールドと文字列: LogonType=3",
      ]),
    ),
  );

  fireEvent.click(
    screen.getByRole("button", {
      name: "フィールドと文字列 TargetUserName=user-a を削除",
    }),
  );
  await waitFor(() =>
    expect(lastFigureRequest(mock)).not.toContain("TargetUserName"),
  );
  expect(lastFigureRequest(mock)).toContain("&fieldContains=LogonType%3D3");
});

test("完全一致の種類で追加した組は、完全一致の組として要求に載せ、別に外せる", async () => {
  const mock = await renderLoaded(
    stubFetch(
      { ...graphResponseJson(), fieldEquals: ["LogonType=1"] },
      nodeDetailResponseJson(),
      terminalListResponseJson(),
      eventKindsResponseJson(),
    ),
  );
  addCondition("フィールドの値が等しい", { フィールド: "LogonType", 値: "1" });

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toContain("&fieldEquals=LogonType%3D1"),
  );
  expect(lastFigureRequest(mock)).not.toContain("fieldContains");
  await waitFor(() =>
    expect(responseConditions()).toContain(
      "フィールドと文字列の完全一致: LogonType=1",
    ),
  );
  fireEvent.click(
    screen.getByRole("button", {
      name: "フィールドと文字列の完全一致 LogonType=1 を削除",
    }),
  );
  await waitFor(() =>
    expect(lastFigureRequest(mock)).not.toContain("LogonType"),
  );
});

test("イベントの種類の選択肢を、端末のフィルタを適用した範囲で取り直す", async () => {
  const mock = await renderLoaded();
  const eventKindRequests = () =>
    mock.mock.calls
      .map(([input]) => input)
      .filter((input) => input.startsWith("/api/v0/event-kinds"));
  expect(eventKindRequests()).toEqual([
    `/api/v0/event-kinds?${matchConditionQuery.slice(1)}`,
  ]);

  chooseConditionValue("Host", "HOST-D");

  await waitFor(() =>
    expect(eventKindRequests().at(-1)).toBe(
      `/api/v0/event-kinds?terminal=${encodeURIComponent(otherTerminalNodeId)}` +
        matchConditionQuery,
    ),
  );
});

test("イベントの分類 ps はプロセスを、net はプロセスと IP アドレスを図に出す", async () => {
  const mock = await renderLoaded();

  chooseEventKind("ps");
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}&nodeKind=process&granularity=object&depth=1` +
        `&eventCategory=ps${matchConditionQuery}`,
    ),
  );
  expect(viewNote()).toBe("プロセス");
  expect(appliedConditionNames()).toEqual(["イベントの分類 ps を削除"]);

  fireEvent.click(
    screen.getByRole("button", { name: "イベントの分類 ps を削除" }),
  );
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(initialFigureRequest),
  );

  chooseEventKind("net");
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}&nodeKind=process&nodeKind=ip&granularity=object` +
        `&depth=1&eventCategory=net${matchConditionQuery}`,
    ),
  );
  expect(viewNote()).toBe("プロセス、IP アドレス");
});

test("図に出す種別を足すと、選んだ種別を繰り返しの nodeKind で送る", async () => {
  const mock = await renderLoaded();

  // 自動が選んだ端末と IP アドレスに、プロセスとファイルを足す。
  addViewKind("プロセス");
  addViewKind("ファイル");

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}` +
        "&nodeKind=terminal&nodeKind=process&nodeKind=ip&nodeKind=file" +
        `&granularity=object&depth=1${matchConditionQuery}`,
    ),
  );
  expect(appliedConditionNames()).toEqual([
    "ノードの種類 端末 を削除",
    "ノードの種類 プロセス を削除",
    "ノードの種類 IP アドレス を削除",
    "ノードの種類 ファイル を削除",
  ]);
  expect(viewNote()).toBe("端末、プロセス、IP アドレス、ファイル");
});

test("図に出す種別にレコードを選ぶと、granularity=record を送る", async () => {
  const mock = await renderLoaded();

  onlyViewKind("レコード");

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}&nodeKind=record&granularity=record` +
        `&depth=1${matchConditionQuery}`,
    ),
  );
});

const processOrigin = "&nodeId=n%3Aprocess%3A8ab3";

async function addProcessOrigin() {
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の隣接ノードを追加",
    }),
  );
}

// 関係先を足す要求は、図に出す種別にレコードがあれば、レコード以外の起点からもレコードの
// 粒度で読む。
test("図に出す種別にレコードがあれば、プロセスの関係先をレコードの粒度で読む", async () => {
  const mock = await renderLoaded();

  onlyViewKind("レコード");
  await addProcessOrigin();
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}&nodeKind=record&nodeKind=process` +
        `&granularity=record${processOrigin}&depth=1${matchConditionQuery}`,
    ),
  );
});

// 足した起点は、図に出す種別の変更と、検索の結果へ戻ることで失わない。
test("関係先の起点を、図に出す種別の変更と検索の結果への戻りで保つ", async () => {
  const mock = await renderLoaded();

  await addProcessOrigin();
  await waitFor(() => expect(lastFigureRequest(mock)).toContain(processOrigin));

  addViewKind("プロセス");
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}&nodeKind=terminal&nodeKind=process` +
        `&nodeKind=ip${objectViewQuery}${processOrigin}&depth=1${matchConditionQuery}`,
    ),
  );

  fireEvent.click(screen.getByRole("button", { name: "検索の結果に戻る" }));
  await waitFor(() =>
    expect(lastFigureRequest(mock)).not.toContain(processOrigin),
  );
  fireEvent.click(
    screen.getByRole("button", { name: "隣接ノードの表示に戻る" }),
  );
  await waitFor(() => expect(lastFigureRequest(mock)).toContain(processOrigin));
});

const addressOriginButton = /203\.0\.113\.21 の隣接ノードを追加/;

function originCount(mock: FetchMock): number {
  return lastFigureRequest(mock)?.match(/nodeId=/g)?.length ?? 0;
}

async function addOriginAndGoBack(mock: FetchMock) {
  await addProcessOrigin();
  await waitFor(() => expect(lastFigureRequest(mock)).toContain(processOrigin));
  fireEvent.click(screen.getByRole("button", { name: "検索の結果に戻る" }));
  await waitFor(() =>
    expect(lastFigureRequest(mock)).not.toContain(processOrigin),
  );
}

const restoreButton = { name: "隣接ノードの表示に戻る" };

test("検索の結果へ戻った後に関係先を足すと、持っている起点に足す", async () => {
  const mock = await renderLoaded();
  await addOriginAndGoBack(mock);

  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", { name: addressOriginButton }),
  );
  await waitFor(() => expect(originCount(mock)).toBe(2));
  expect(lastFigureRequest(mock)).toContain(processOrigin);
});

test("戻った後に検索の文字列を変えると、関係先の表示から戻る先は新しい条件の検索になる", async () => {
  const mock = await renderLoaded();
  await addOriginAndGoBack(mock);

  addTerm("cmd.exe", "含む");
  await waitFor(() => expect(lastFigureRequest(mock)).toBe(cmdSearchRequest));
  fireEvent.click(screen.getByRole("button", restoreButton));
  await waitFor(() => expect(lastFigureRequest(mock)).toContain(processOrigin));
  fireEvent.click(screen.getByRole("button", { name: "検索の結果に戻る" }));
  await waitFor(() => expect(lastFigureRequest(mock)).toBe(cmdSearchRequest));
});

test("関係先の起点を全部外すと、持っている起点も無くなる", async () => {
  const mock = await renderLoaded();
  await addOriginAndGoBack(mock);

  fireEvent.click(screen.getByRole("button", restoreButton));
  fireEvent.click(
    await screen.findByRole("button", {
      name: "C:\\Windows\\System32\\cmd.exe の隣接ノードを非表示",
    }),
  );
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(initialFigureRequest),
  );
  expect(screen.queryByRole("button", restoreButton)).toBeNull();
});

test("足した関係先を捨てると、戻る button が消え、次の関係先は新しい起点で始まる", async () => {
  const mock = await renderLoaded();
  await addOriginAndGoBack(mock);

  fireEvent.click(
    screen.getByRole("button", { name: "追加した隣接ノードを削除" }),
  );
  expect(screen.queryByRole("button", restoreButton)).toBeNull();
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", { name: addressOriginButton }),
  );
  await waitFor(() => expect(originCount(mock)).toBe(1));
  expect(lastFigureRequest(mock)).not.toContain(processOrigin);
});

test("親子の連鎖をたどっても、関係先の起点を持ち、関係先の表示へ戻せる", async () => {
  const mock = stubLineageFetch();
  await renderLoaded(mock);

  await addProcessOrigin();
  await waitFor(() => expect(lastFigureRequest(mock)).toContain(processOrigin));
  fireEvent.click(
    await screen.findByRole("button", { name: "プロセスの親子関係を表示" }),
  );
  await waitFor(() => expect(lastFigureRequest(mock)).toBe(lineageRequest));
  fireEvent.click(screen.getByRole("button", restoreButton));
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}${processOrigin}&depth=1` +
        matchConditionQuery,
    ),
  );
});

test("関係先だけを表示すると、持っている起点を捨ててそのノードだけで始め直す", async () => {
  const mock = await renderLoaded();

  await addProcessOrigin();
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", { name: addressOriginButton }),
  );
  await waitFor(() => expect(originCount(mock)).toBe(2));
  fireEvent.click(
    await screen.findByRole("button", { name: "隣接ノードだけを表示" }),
  );
  await waitFor(() => expect(originCount(mock)).toBe(1));
  fireEvent.click(screen.getByRole("button", { name: "検索の結果に戻る" }));
  expect(await screen.findByRole("button", restoreButton)).toBeTruthy();
});

test("図に出す種別を手で選ぶと、関係先の要求に選んだ種別と起点の種別を載せる", async () => {
  const mock = await renderLoaded();

  onlyViewKind("プロセス");
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", { name: addressOriginButton }),
  );
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}&nodeKind=process&nodeKind=ip` +
        "&granularity=object&nodeId=n%3Aip%3A5d72&depth=1" +
        matchConditionQuery,
    ),
  );
});

test("関係の種別を選んでも外しても、関係先の起点を保ったまま関係の種別のフィルタを適用する", async () => {
  const mock = await renderLoaded();
  await addProcessOrigin();
  await waitFor(() => expect(lastFigureRequest(mock)).toContain(processOrigin));

  chooseConditionValue("エッジの種類", edgeKindLabels.process_communication);
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toContain(
      "&edgeKind=process_communication",
    ),
  );
  expect(lastFigureRequest(mock)).toContain(processOrigin);

  fireEvent.click(
    screen.getByRole("button", {
      name: `エッジの種類 ${edgeKindLabels.process_communication} を削除`,
    }),
  );
  await waitFor(() =>
    expect(lastFigureRequest(mock)).not.toContain("edgeKind="),
  );
  expect(lastFigureRequest(mock)).toContain(processOrigin);
});

test("手で選んだ図に出す種別は、事象の分類から選ぶ種別より優先し、自動に戻すと分類の種別へ戻る", async () => {
  const mock = await renderLoaded();
  const processRequest =
    `/api/v0/graph?${limitQuery}&nodeKind=process&granularity=object&depth=1` +
    `&eventCategory=ps${matchConditionQuery}`;

  chooseEventKind("ps");
  await waitFor(() => expect(lastFigureRequest(mock)).toBe(processRequest));

  // 自動が選んだプロセスにファイルを足すと、手の選択になる。
  addViewKind("ファイル");
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}&nodeKind=process&nodeKind=file` +
        `&granularity=object&depth=1&eventCategory=ps${matchConditionQuery}`,
    ),
  );
  expect(viewNote()).toBe("プロセス、ファイル");

  // 手で選んだ種別の chip をすべて外すと、自動へ戻る。
  fireEvent.click(
    screen.getByRole("button", { name: "ノードの種類 ファイル を削除" }),
  );
  fireEvent.click(
    screen.getByRole("button", { name: "ノードの種類 プロセス を削除" }),
  );
  await waitFor(() => expect(lastFigureRequest(mock)).toBe(processRequest));
});

/**
 * 親子の連鎖をたどった応答。fixture のプロセスと、その親のプロセスを含む。
 * 親から子へのエッジだけがあり、親は自身の親との関係を持たない。
 */
function lineageResponseJson() {
  const graph = graphResponseJson();
  const child = graph.nodes[1];
  return {
    ...graph,
    nodes: [
      child,
      {
        ...child,
        id: parentProcessNodeId,
        identity: [
          { semantic: "terminal.id", value: terminalIdText },
          { semantic: "process.id", value: "{P0}" },
        ],
        label: { rawText: "C:\\Windows\\explorer.exe", valueState: "present" },
      },
    ],
    nodeCount: 2,
    matchedKinds: [{ kind: "process", count: 2 }],
    edges: [
      {
        id: "e:process_parent_child:0001",
        kind: "process_parent_child",
        state: "observed",
        sourceNodeId: parentProcessNodeId,
        targetNodeId: processNodeId,
        evidenceCount: 1,
      },
    ],
    edgeCount: 1,
  };
}

/** 親子の連鎖の要求に lineageResponseJson を返し、それ以外は stubFetch と同じ応答を返す。 */
function stubLineageFetch() {
  const mock = stubFetch();
  mock.mockImplementation(async (input: string) => {
    if (input.startsWith("/api/v0/event-kinds")) {
      return jsonResponse(200, eventKindsResponseJson());
    }
    if (input.startsWith("/api/v0/nodes/")) {
      return jsonResponse(200, nodeDetailResponseJson());
    }
    if (input.startsWith(terminalListPrefix)) {
      return jsonResponse(200, terminalListResponseJson());
    }
    return jsonResponse(
      200,
      input.includes("edgeKind=process_parent_child")
        ? lineageResponseJson()
        : graphResponseJson(),
    );
  });
  return mock;
}

const lineageRoots = { name: "親の記録が無いプロセス" };

const lineageRequest =
  `/api/v0/graph?${limitQuery}&nodeKind=process&granularity=object` +
  `&nodeId=${encodeURIComponent(processNodeId)}&depth=${maxGraphDepth}` +
  `&edgeKind=process_parent_child${matchConditionQuery}`;

const cmdSearchRequest =
  `/api/v0/graph?${limitQuery}${objectViewQuery}&depth=1` +
  `&valueContains=cmd.exe${matchConditionQuery}`;

/** 文字列 cmd.exe で検索し、プロセスの詳細から親子の連鎖をたどる。 */
async function traceLineageFromSearch(mock: FetchMock) {
  await renderLoaded(mock);
  addTerm("cmd.exe", "含む");
  await waitFor(() => expect(lastFigureRequest(mock)).toBe(cmdSearchRequest));
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );
  fireEvent.click(
    await screen.findByRole("button", { name: "プロセスの親子関係を表示" }),
  );
  await waitFor(() => expect(lastFigureRequest(mock)).toBe(lineageRequest));
}

test("親子の連鎖をたどると検索の条件を載せずに要求し、親の無いプロセスを出し、戻ると検索の要求へ戻る", async () => {
  const mock = stubLineageFetch();

  await traceLineageFromSearch(mock);

  const back = screen.getByRole("button", { name: "検索の結果に戻る" });
  const status = back.closest("[role='status']") as HTMLElement;
  expect(valuePairTexts(status)).toEqual([
    "表示中: プロセスの親子関係",
    "元のプロセス: C:\\Windows\\System32\\cmd.exe",
    "検索の条件: 不使用",
  ]);
  // 親の記録が無いプロセスは Nodes のビューに、数と辿ったホップ数を添えて出す。
  const rootsNotice = await screen.findByRole("region", lineageRoots);
  expect(rootsNotice.textContent).toContain(
    `親の記録なし: 1ホップ数: ${maxGraphDepth}`,
  );
  // 子のプロセスは親とのエッジを持つため、上端に出ない。
  expect(
    within(rootsNotice)
      .getAllByRole("button")
      .map((button) => button.textContent),
  ).toEqual(["C:\\Windows\\explorer.exe"]);
  // 検索の条件は保ち、戻る先として残す。
  expect(appliedConditionNames()).toEqual(["含む文字列 cmd.exe を削除"]);

  // 上端のプロセスを選ぶと、そのノードの詳細を要求する。
  fireEvent.click(
    within(rootsNotice).getByRole("button", {
      name: "C:\\Windows\\explorer.exe",
    }),
  );
  await waitFor(() =>
    expect(mock.mock.calls.map((call) => call[0])).toContain(
      `/api/v0/nodes/${encodeURIComponent(parentProcessNodeId)}?${matchConditionQuery.slice(1)}`,
    ),
  );

  fireEvent.click(back);

  await waitFor(() => expect(lastFigureRequest(mock)).toBe(cmdSearchRequest));
  expect(screen.queryByRole("button", { name: "検索の結果に戻る" })).toBeNull();
  expect(screen.queryByRole("region", lineageRoots)).toBeNull();
});

test("検索の結果では、親子の連鎖の上端と連鎖の通知を出さない", async () => {
  await renderLoaded(stubFetch(lineageResponseJson()));

  expect(screen.queryByRole("region", lineageRoots)).toBeNull();
  expect(screen.queryByRole("button", { name: "検索の結果に戻る" })).toBeNull();
});

test("プロセスの親子関係を表示している間に文字列を追加すると、親子関係を抜けて検索の要求を送る", async () => {
  const mock = stubLineageFetch();

  await traceLineageFromSearch(mock);
  addTerm("whoami", "含む");

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}&depth=1` +
        `&valueContains=cmd.exe&valueContains=whoami${matchConditionQuery}`,
    ),
  );
  expect(screen.queryByRole("button", { name: "検索の結果に戻る" })).toBeNull();
});

// 元レコードの表示は、本機能の外から検索の文字列を足す。足した文字列を図に使わないまま
// 連鎖を出し続けない。
test("親子の連鎖をたどっている間に外から文字列が足されると、連鎖を抜けて検索の要求を送る", async () => {
  const mock = stubLineageFetch();
  const props = {
    matchConditions: {
      conditions: [{ conditionKey: "destination_ip" as const }],
    },
    onApplyMatchConditions: () => {},
    recordFilter: {},
    onApplyRecordFilter: () => {},
    onChangeSearchTerms: () => {},
    caseIds: [],
    onSelectRecord: () => {},
    selectedEdgeId: undefined,
    onSelectEdge: () => {},
    assertions: () => null,
    dataVersion: 0,
    layout: stackPanes,
  };
  const view = render(
    <GraphExploreWithTerminals
      {...props}
      searchTerms={{ contains: ["cmd.exe"], excludes: [] }}
    />,
  );
  await waitFor(() => expect(lastFigureRequest(mock)).toBe(cmdSearchRequest));
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );
  fireEvent.click(
    await screen.findByRole("button", { name: "プロセスの親子関係を表示" }),
  );
  await waitFor(() => expect(lastFigureRequest(mock)).toBe(lineageRequest));

  view.rerender(
    <GraphExploreWithTerminals
      {...props}
      searchTerms={{ contains: ["cmd.exe", "whoami"], excludes: [] }}
    />,
  );

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}&depth=1` +
        `&valueContains=cmd.exe&valueContains=whoami${matchConditionQuery}`,
    ),
  );
  expect(screen.queryByRole("button", { name: "検索の結果に戻る" })).toBeNull();
});

// 詳しい条件の操作は連鎖を抜ける。そのとき連鎖の起点・関係の種別・ホップ数を検索の条件へ
// 持ち込むと、分析者が足していない条件で検索の結果にフィルタが適用される。
test("親子の連鎖をたどっている間にホップ数を変えると、連鎖の条件を検索の要求へ持ち込まない", async () => {
  const mock = stubLineageFetch();

  await traceLineageFromSearch(mock);
  chooseConditionValue("ホップ数", "0");

  await waitFor(() =>
    expect(
      screen.queryByRole("button", { name: "検索の結果に戻る" }),
    ).toBeNull(),
  );
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}&depth=0` +
        `&valueContains=cmd.exe${matchConditionQuery}`,
    ),
  );
});

// 描画するノードの上限はグラフの描き方であり、たどっている連鎖を抜けない。連鎖の要求も同じ上限を使う。
test("親子の連鎖をたどっている間に描画するノードの上限を変えると、連鎖のまま上限だけを変える", async () => {
  const mock = stubLineageFetch();

  await traceLineageFromSearch(mock);
  fireEvent.change(screen.getByLabelText("描画するノードの上限"), {
    target: { value: "500" },
  });

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      lineageRequest.replace(limitQuery, "nodeLimit=500"),
    ),
  );
  expect(screen.getByRole("button", { name: "検索の結果に戻る" })).toBeTruthy();
});

test("ノードの詳細の値を含む条件に追加すると、条件の一覧と valueContains に載せる", async () => {
  const mock = stubFetch();

  renderGraphExplore();
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );
  fireEvent.click(
    await screen.findByRole("button", {
      name: "含む条件に追加: cmd.exe /c whoami",
    }),
  );

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}&depth=1` +
        `&valueContains=cmd.exe+%2Fc+whoami${matchConditionQuery}`,
    ),
  );
  expect(appliedConditionNames()).toEqual([
    "含む文字列 cmd.exe /c whoami を削除",
  ]);
  // 端末のレコードだけを残すフィルタを適用する操作は端末のノードだけが持つ。
  expect(
    screen.queryByRole("button", {
      name: "この端末でフィルタ",
    }),
  ).toBeNull();
});

/** 端末のノード 1 つの詳細。 */
function terminalNodeDetailResponseJson() {
  const detail = nodeDetailResponseJson();
  return {
    ...detail,
    node: {
      ...detail.node,
      id: terminalNodeId,
      kind: "terminal",
      keyForm: "terminal_id",
      identity: [{ semantic: "terminal.id", value: terminalIdText }],
      label: { rawText: "HOST-C", valueState: "present" },
      creationRecord: "item_absent",
    },
  };
}

test("端末の詳細からこの端末のレコードだけを残すフィルタを適用すると、terminal を送り、端末の欄と条件の一覧に出す", async () => {
  const mock = stubFetch(graphResponseJson(), terminalNodeDetailResponseJson());

  renderGraphExplore();
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", { name: "端末 HOST-C の詳細を開く" }),
  );
  fireEvent.click(
    await screen.findByRole("button", {
      name: "この端末でフィルタ",
    }),
  );

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}&depth=1` +
        `&terminal=${encodeURIComponent(terminalNodeId)}${matchConditionQuery}`,
    ),
  );
  expect(appliedConditionNames()).toEqual(["Host HOST-C を削除"]);
  // 親子の連鎖をたどる操作はプロセスのノードだけが持つ。
  expect(
    screen.queryByRole("button", { name: "プロセスの親子関係を表示" }),
  ).toBeNull();
});

// 1 つの取得の失敗を検索の列と図の列の両方が alert で出すと、読み上げが同じ失敗を 2 回読む。
test("部分グラフの取得の失敗を 1 つの alert で出す", async () => {
  stubFetch(jsonResponse(500, apiErrorJson("internal_error")));

  renderGraphExplore();

  const alerts = await screen.findAllByRole("alert");
  expect(alerts).toHaveLength(1);
  expect(alerts[0]?.textContent).toContain("グラフの取得");
});
