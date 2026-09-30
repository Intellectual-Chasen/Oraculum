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
import { maxGraphDepth } from "@/shared/api/graph";
import {
  candidateMatch,
  candidateResponse,
  candidateRule,
} from "@/testdata/attackCandidates/candidateResponse";
import {
  addCondition,
  chooseConditionKind,
  chooseConditionValue,
  conditionValueOptions,
} from "@/testdata/conditionInput";
import {
  createdNodeDetailResponseJson,
  emptyGraphResponseJson,
  graphResponseJson,
  invisibleCharacterGraphResponseJson,
  nodeDetailResponseJson,
  overLimitGraphResponseJson,
  processNodeId,
  recordNodeDetailResponseJson,
  recordNodeGraphResponseJson,
  valueCountJson,
  valueCountWithoutTimeJson,
} from "@/testdata/graph/graphResponse";
import { jsonResponse } from "@/testdata/http";
import { apiErrorJson } from "@/testdata/sources/sourcesResponse";
import {
  figureRequests,
  findEdgeList,
  findNodeList,
  findPair,
  GraphExploreHarness,
  getPair,
  initialFigureRequest,
  lastFigureRequest,
  limitQuery,
  matchConditionQuery,
  objectViewQuery,
  renderGraphExplore,
  sentRequest,
  stubFetch,
  terminalIpViewQuery,
  terminalListPrefix,
} from "./graphExploreTestHarness";
import { edgeKindLabels } from "./labels";
import type { SubgraphDrawing } from "./subgraph";

// WebGL の renderer は jsdom で動かない。図に渡した値を記録する stub に置き換え、
// 選択・根拠表示・空・失敗を本 test が確かめる。実描画はブラウザーで確かめる。
const { canvasSpy } = vi.hoisted(() => ({ canvasSpy: vi.fn() }));

vi.mock("./SubgraphCanvas", () => ({
  SubgraphCanvas: (props: {
    drawing: SubgraphDrawing;
    selectedNodeId: string | undefined;
  }) => {
    canvasSpy(props);
    return (
      <p>
        図に渡したノードは {props.drawing.points.length} 件、エッジは{" "}
        {props.drawing.links.length} 本です。
      </p>
    );
  },
}));

afterEach(() => {
  cleanup();
  canvasSpy.mockClear();
  vi.unstubAllGlobals();
});

function registryGraphResponseJson() {
  const graph = graphResponseJson();
  return {
    ...graph,
    nodes: graph.nodes.map((node, index) =>
      index === 1
        ? {
            ...node,
            kind: "registry_value",
            keyForm: "terminal_id_registry_value_key_path",
            identity: [
              { semantic: "terminal.id", value: "HOST-C-TMID" },
              {
                semantic: "registry_value.key_path",
                value:
                  "HKLM\\Software\\Microsoft\\Windows\\CurrentVersion\\Run",
              },
            ],
            label: { rawText: "Run", valueState: "present" },
            creationRecord: "item_absent",
          }
        : node,
    ),
  };
}

function registryNodeDetailResponseJson(
  valueCount: number,
  includeNameAttribute = true,
) {
  const detail = nodeDetailResponseJson();
  const values = detail.attributes[0]?.values ?? [];
  return {
    ...detail,
    node: {
      ...detail.node,
      kind: "registry_value",
      keyForm: "terminal_id_registry_value_key_path",
      identity: [
        { semantic: "terminal.id", value: "HOST-C-TMID" },
        {
          semantic: "registry_value.key_path",
          value: "HKLM\\Software\\Microsoft\\Windows\\CurrentVersion\\Run",
        },
      ],
      label: { rawText: "Run", valueState: "present" },
      creationRecord: "item_absent",
    },
    attributes: includeNameAttribute
      ? [
          {
            ...detail.attributes[0],
            semantic: "registry_value.name",
            valueCount,
            values: values.slice(0, valueCount).map((value, index) => ({
              ...value,
              field: {
                ...value.field,
                name: `valueName${index + 1}`,
                semantic: "registry_value.name",
              },
            })),
          },
        ]
      : [],
    attributeCount: includeNameAttribute ? 1 : 0,
  };
}

test("部分グラフを取得し、件数と図とノードの一覧を出す", async () => {
  const mock = stubFetch();

  renderGraphExplore();

  expect(
    screen
      .getAllByRole("status")
      .map((status) => status.textContent)
      .filter((text) => text?.includes("グラフの読み込み")),
  ).toEqual(["グラフの読み込み中"]);

  await findNodeList();
  // 図の要求を、事象の種別の選択肢と ATT&CK の候補と端末の選択肢の要求より先に出す。
  // 端末の選択肢は上位の画面が取得するため、本機能の要求の後に出る。
  expect(mock.mock.calls.map((call) => call[0])).toEqual([
    sentRequest(initialFigureRequest),
    `/api/v0/event-kinds?${matchConditionQuery.slice(1)}`,
    sentRequest(initialFigureRequest).replace(
      "/api/v0/graph?",
      "/api/v0/attack-candidates?",
    ),
    `${terminalListPrefix}${matchConditionQuery.slice(1)}`,
  ]);
  const result = document.querySelector(".result-pane");
  expect(result?.textContent).toContain("一致ノード: 2");
  expect(result?.textContent).toContain("プロセス 1");
  expect(result?.textContent).toContain("端末 1");
  expect(result?.textContent).toContain("グラフの一致ノード: 2");
  expect(result?.textContent).toContain("エッジの端のノード: 1");
  expect(result?.textContent).toContain("エッジ: 2");
  expect(
    await screen.findByText("図に渡したノードは 3 件、エッジは 2 本です。"),
  ).toBeTruthy();
  expect(canvasSpy.mock.calls.at(-1)?.[0]).toMatchObject({
    selectedNodeId: undefined,
  });
  expect(canvasSpy.mock.calls.at(-1)?.[0].drawing.links).toHaveLength(2);
  expect(screen.getByText("エッジの端のノード")).toBeTruthy();
});

test("Nodes と Edges のビューは、Graph と同じ応答のノードとエッジを見出しの件数と一緒に並べる", async () => {
  stubFetch();

  renderGraphExplore();

  const nodesHeading = await screen.findByRole("heading", { name: "ノード" });
  expect(nodesHeading.closest("section")?.textContent).toContain("件数: 3");
  const table = screen.getByRole("table", { name: "グラフのノードの一覧" });
  expect(
    within(table).getByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  ).toBeTruthy();
  const edgesHeading = screen.getByRole("heading", { name: "エッジ" });
  expect(edgesHeading.closest("section")?.textContent).toMatch(/本数: \d+/);
  expect(
    screen.getByRole("table", { name: "グラフのエッジの一覧" }),
  ).toBeTruthy();
});

test("Nodes と Edges のビューは、空の応答で理由を Search の結果に任せた 1 行を出し、Graph の図は件数だけを出す", async () => {
  stubFetch(emptyGraphResponseJson());

  renderGraphExplore();

  expect(await screen.findAllByText("一致ノード: 0")).toHaveLength(3);
  expect(document.querySelector(".pane-graph .figure")?.textContent).toBe(
    "一致ノード: 0",
  );
  expect(
    screen.queryByRole("table", { name: "グラフのノードの一覧" }),
  ).toBeNull();
});

test("ノードを選ぶと詳細を取得し、属性と根拠と相互参照の件数を出す", async () => {
  const mock = stubFetch();

  renderGraphExplore();

  expect(screen.getByText("ノード未選択")).toBeTruthy();

  await findNodeList();
  const button = await screen.findByRole("button", {
    name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
  });
  fireEvent.click(button);

  await screen.findByRole("table", { name: "属性" });
  expect(
    within(document.querySelector(".node-detail") as HTMLElement).queryByRole(
      "status",
    ),
  ).toBeNull();
  expect(
    mock.mock.calls
      .map((call) => call[0])
      .filter((path) => path.startsWith("/api/v0/nodes/")),
  ).toEqual([
    `/api/v0/nodes/n%3Aprocess%3A8ab3?${matchConditionQuery.slice(1)}`,
  ]);
  expect(button).toHaveAttribute("aria-pressed", "true");
  expect(screen.getByText("cmd.exe /c whoami")).toBeTruthy();
  expect(screen.getByText("cmd.exe /c net use")).toBeTruthy();
  expect(screen.getByRole("table", { name: "エッジの件数" })).toBeTruthy();
  expect(canvasSpy.mock.calls.at(-1)?.[0]).toMatchObject({
    selectedNodeId: processNodeId,
  });
});

/** ノードの詳細を開き、プロセスのノードを選ぶ。 */
async function openProcessNodeDetail() {
  renderGraphExplore();
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );
  await screen.findByRole("table", { name: "属性" });
}

// 作成レコードを持つノードは、そのレコードを一覧に出し、Record へ進める。
test("ノードの詳細に、作成レコードを一覧で出す", async () => {
  stubFetch(graphResponseJson(), createdNodeDetailResponseJson());

  await openProcessNodeDetail();

  const table = screen.getByRole("table", { name: "作成レコード" });
  // 生成の根拠として応答が含む 1 件の位置を、収集元と位置の文字列で指定して確かめる。
  // 位置は位置の列の button に出し、button の名前が収集元と位置を持つ。
  expect(table.textContent).toContain("host-a.log");
  expect(
    within(table).getAllByRole("button", {
      name: /^Record に表示: 収集元: host-a\.log、ID: 112、/,
    }).length,
  ).toBe(1);
});

// 作成レコードが 0 件のノードは、「なし」の組を出す。
test("作成レコードが 0 件のノードに、作成レコードなしの組を出す", async () => {
  stubFetch();

  await openProcessNodeDetail();

  expect(document.querySelector(".node-detail")?.textContent).toContain(
    "作成レコード: なし",
  );
  expect(screen.queryByRole("table", { name: "作成レコード" })).toBeNull();
});

// **軸が該当しない種類ではセクションそのものを出さない。** 該当しない軸を「無い」と見せない。
test("作成レコードを持てない種類のノードに、作成レコードのセクションを出さない", async () => {
  stubFetch(registryGraphResponseJson(), registryNodeDetailResponseJson(2));

  renderGraphExplore();
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: /レジストリの値 Run の詳細を開く/,
    }),
  );
  await screen.findByRole("table", { name: "属性" });

  expect(screen.queryByRole("table", { name: "作成レコード" })).toBeNull();
  expect(document.querySelector(".node-detail")?.textContent).not.toContain(
    "作成レコード: なし",
  );
});

test("複数の値の名前を束ねたレジストリ値を詳細で区別する", async () => {
  stubFetch(registryGraphResponseJson(), registryNodeDetailResponseJson(2));

  renderGraphExplore();
  await findNodeList();

  fireEvent.click(
    await screen.findByRole("button", {
      name: "レジストリの値 Run の詳細を開く",
    }),
  );

  await waitFor(() =>
    expect(
      within(document.querySelector(".node-detail") as HTMLElement).getByRole(
        "status",
      ).textContent,
    ).toBe("まとめた値の名前: 2"),
  );
  expect(screen.getAllByText("2").length).toBeGreaterThan(0);
});

test("値の名前が 1 つのレジストリの値では束ねた案内を出さない", async () => {
  stubFetch(registryGraphResponseJson(), registryNodeDetailResponseJson(1));

  renderGraphExplore();
  await findNodeList();

  fireEvent.click(
    await screen.findByRole("button", {
      name: "レジストリの値 Run の詳細を開く",
    }),
  );

  await screen.findByRole("table", { name: "属性" });
  expect(
    within(document.querySelector(".node-detail") as HTMLElement).queryByRole(
      "status",
    ),
  ).toBeNull();
});

test("値の名前の属性が無いレジストリの値では束ねた案内を出さない", async () => {
  stubFetch(
    registryGraphResponseJson(),
    registryNodeDetailResponseJson(1, false),
  );

  renderGraphExplore();
  await findNodeList();

  fireEvent.click(
    await screen.findByRole("button", {
      name: "レジストリの値 Run の詳細を開く",
    }),
  );

  await screen.findByRole("table", { name: "属性" });
  expect(
    within(document.querySelector(".node-detail") as HTMLElement).queryByRole(
      "status",
    ),
  ).toBeNull();
});

test("根拠のレコードを選ぶと、そのレコード位置を上位へ渡す", async () => {
  stubFetch();
  const selected = vi.fn();

  renderGraphExplore(selected);
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

  expect(selected).toHaveBeenCalledTimes(1);
  expect(selected.mock.calls[0]?.[0]).toMatchObject({
    sourceFileName: "host-a.log",
    positionKind: "sequence_number",
    sequenceNumber: 112,
    lineNumber: 1022,
  });
});

test("エッジの種類の選び方は、常には出さず、エッジの種類の値の入力で読める", async () => {
  stubFetch();
  renderGraphExplore();
  await findNodeList();

  expect(screen.queryByText("未選択: すべてのエッジの種類")).toBeNull();
  chooseConditionKind("エッジの種類");
  expect(screen.getByText("未選択: すべてのエッジの種類")).toBeTruthy();
});

test("ホップ数を 0 に切り替えると、要求の depth が変わる", async () => {
  const mock = stubFetch();

  renderGraphExplore();
  await findNodeList();

  chooseConditionValue("ホップ数", "0");

  // ホップ数は検索の条件ではなく、端末を出す図のままホップ数だけが変わる。
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${terminalIpViewQuery}&depth=0${matchConditionQuery}`,
    ),
  );
});

test("ホップ数を 1 より大きい値へ切り替えると、要求の depth が変わる", async () => {
  const mock = stubFetch();

  renderGraphExplore();
  await findNodeList();

  chooseConditionValue("ホップ数", `${maxGraphDepth}`);

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${terminalIpViewQuery}&depth=${maxGraphDepth}${matchConditionQuery}`,
    ),
  );
});

test("条件を一致ノードだけに適用する選択を入れると、要求に conditionsOnOriginsOnly が載る", async () => {
  const mock = stubFetch();

  renderGraphExplore();
  await findNodeList();

  chooseConditionKind("条件を一致ノードだけに適用");

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${terminalIpViewQuery}&depth=1` +
        `&conditionsOnOriginsOnly=true${matchConditionQuery}`,
    ),
  );
});

test("アドレスの範囲を入力して適用すると、要求の項目に載せる", async () => {
  const mock = stubFetch();

  renderGraphExplore();
  await findNodeList();

  addCondition("CIDR の外のアドレス", {
    "CIDR の外のアドレス": "198.51.100.0/28",
  });

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}&depth=1` +
        "&addressNotInCidr=198.51.100.0%2F28" +
        matchConditionQuery,
    ),
  );

  addCondition("CIDR の内のアドレス", {
    "CIDR の内のアドレス": "198.51.100.0/24",
  });

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}&depth=1` +
        "&addressInCidr=198.51.100.0%2F24" +
        "&addressNotInCidr=198.51.100.0%2F28" +
        matchConditionQuery,
    ),
  );
});

test("一致したフィールドと一致した値と値の形を、ノードの一覧に出す", async () => {
  const response = graphResponseJson();
  stubFetch({
    ...response,
    valueContains: ["cmd.exe"],
    nodes: response.nodes.map((node, index) =>
      index === 0
        ? {
            ...node,
            valueMatches: [
              {
                semantic: "process.command_line",
                form: "raw_text",
                value: "cmd.exe /c whoami",
              },
              { name: "psProfile", form: "normalized", value: "cmd.exe" },
            ],
          }
        : { ...node, valueMatches: [] },
    ),
  });

  renderGraphExplore();
  await findNodeList();

  expect(
    screen.getByRole("columnheader", { name: "一致したフィールド" }),
  ).toBeTruthy();
  // 一致したフィールドの名前と、一致した値と、値の形を同じ行に出す。
  expect(getPair("process.command_line: cmd.exe /c whoami 原文")).toBeTruthy();
  expect(getPair("psProfile: cmd.exe 正規化した値")).toBeTruthy();
  // 一致したフィールドを持たないノードは、一覧に入った経路の印を出す。一致ノードは、ノードを
  // 指すレコードのフィールドが文字列を含むため入り、端のノードはエッジの端として入る。
  expect(screen.getByText("レコード経由の一致")).toBeTruthy();
  expect(
    screen.getAllByText("エッジの端のノード", { selector: ".sr-only" }),
  ).not.toHaveLength(0);
});

test("検索の文字列を与えない応答では、一致したフィールドの列を出さない", async () => {
  stubFetch();

  renderGraphExplore();
  await findNodeList();

  expect(
    screen.queryByRole("columnheader", { name: "一致したフィールド" }),
  ).toBeNull();
});

test.each([
  ["no_value_match", "読み取れたフィールドに文字列なし"],
  ["value_match_outside_filter", "他のフィルタで除外"],
])("0 件の理由 %s を別のラベルで出す", async (emptyReason, want) => {
  stubFetch({
    ...emptyGraphResponseJson(),
    valueContains: ["no-such-token"],
    valueExcludes: ["svchost.exe"],
    emptyReason,
  });

  renderGraphExplore();

  await screen.findByText(want);
  // **0 件でも server が用いた条件を出す。**
  expect(getPair("含む文字列: no-such-token")).toBeTruthy();
  expect(getPair("含まない文字列: svchost.exe")).toBeTruthy();
});

test.each([
  ["no_value_match", "読み取れたフィールドに全体が等しい値なし"],
  ["value_match_outside_filter", "他のフィルタで除外"],
])(
  "完全一致の条件だけで 0 件の理由 %s を、値の全体で比べたラベルで出す",
  async (emptyReason, want) => {
    stubFetch({
      ...emptyGraphResponseJson(),
      fieldEquals: ["LogonType=1"],
      emptyReason,
    });

    renderGraphExplore();

    await screen.findByText(want);
  },
);

test("応答が用いた条件に、ホップ数とエッジの種類を出す", async () => {
  stubFetch({
    ...emptyGraphResponseJson(),
    depth: 4,
    edgeKinds: ["process_communication"],
    emptyReason: "no_record_in_filter",
  });

  renderGraphExplore();

  expect(await findPair("ホップ数: 4")).toBeTruthy();
  expect(
    getPair(`エッジの種類: ${edgeKindLabels.process_communication}`),
  ).toBeTruthy();
});

test("一致するレコードがノードを指さない 0 件は、レコードの種類を追加する次の操作を出す", async () => {
  stubFetch({
    ...emptyGraphResponseJson(),
    emptyReason: "record_match_without_object",
  });

  renderGraphExplore();

  await screen.findByText("端末のほかのノードを指すレコードなし");
  expect(getPair("次の操作: ノードの種類にレコードを追加")).toBeTruthy();
});

test("レコードのノードを、種類と同一性の基準と表示名で一覧に出す", async () => {
  stubFetch(recordNodeGraphResponseJson());

  renderGraphExplore();
  const table = await findNodeList();
  const row = within(table)
    .getAllByRole("row")
    .find((element) => element.textContent?.includes("host-a.log ID 112"));
  if (row === undefined) {
    throw new Error("the list carries no row for the record node");
  }
  expect(within(row).getByText("レコード")).toBeTruthy();
  expect(
    within(row).getByText("収集元の file の内容とレコードの位置"),
  ).toBeTruthy();
  expect(within(row).getByText("record.sequence_number: 112")).toBeTruthy();
  // レコードは作成レコードを持てない。
  expect(within(row).getByText("対象外の種類")).toBeTruthy();
  // レコードがノードを指すエッジがエッジの一覧に出る。
  // 同じ文字列はエッジの種類を選ぶ option にもあるため、一覧の中で探す。
  const edges = await findEdgeList();
  expect(within(edges).getByText("レコードに現れたノード")).toBeTruthy();
});

test("値ごとの件数が 0 件の理由を出す", async () => {
  stubFetch({
    ...graphResponseJson(),
    countBy: "http.user_agnet",
    valueCounts: [],
    distinctValueCount: 0,
    valueCountsEmptyReason: "no_field_observed",
  });

  renderGraphExplore();
  await findNodeList();

  expect(screen.getByText("読み取れたレコードにフィールド名なし")).toBeTruthy();
  expect(getPair("件数を数えるフィールド: http.user_agnet")).toBeTruthy();
});

test("値ごとの件数が 0 件で、フィールドの値を持つノードの種類が今の種類と違うとき、値を持つ種類を示す", async () => {
  stubFetch({
    ...graphResponseJson(),
    countBy: "connection.destination_hostname",
    valueCounts: [],
    distinctValueCount: 0,
    valueCountsEmptyReason: "field_on_other_node_kind",
    valueCountsNodeKinds: ["record"],
  });

  renderGraphExplore();
  await findNodeList();

  expect(screen.getByText("表示中のノードの種類に値なし")).toBeTruthy();
  expect(getPair("値を持つノードの種類: レコード")).toBeTruthy();
});

test("数えるフィールドを入力して適用すると、要求の countBy と valueLimit に載せる", async () => {
  const mock = stubFetch();

  renderGraphExplore();
  await findNodeList();

  addCondition("件数を数えるフィールド", {
    件数を数えるフィールド: "http.user_agent",
  });

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}&depth=1` +
        "&countBy=http.user_agent" +
        matchConditionQuery,
    ),
  );
});

test("数えるフィールドが空の適用では、フィールドを要求に載せない", async () => {
  const mock = stubFetch();

  renderGraphExplore();
  await findNodeList();
  addCondition("件数を数えるフィールド", {});

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(initialFigureRequest),
  );
});

test("値ごとの件数と時刻の両端を出し、値の種類の件数を出す", async () => {
  stubFetch({
    ...graphResponseJson(),
    countBy: "http.user_agent",
    valueCounts: [valueCountJson(), valueCountWithoutTimeJson()],
    distinctValueCount: 2,
  });

  renderGraphExplore();
  await screen.findByRole("table", {
    name: "フィールドの値ごとのレコード数",
  });

  expect(getPair("値の種類: 2")).toBeTruthy();
  expect(screen.getByText("ExampleClient/1.0")).toBeTruthy();
  expect(screen.getByText("640")).toBeTruthy();
  // **根拠のレコードの列を持たない。** 図は根拠を描かず、応答も根拠の中身を含まない。
  expect(
    within(
      screen.getByRole("table", { name: "フィールドの値ごとのレコード数" }),
    ).queryByRole("columnheader", { name: "根拠のレコード" }),
  ).toBeNull();
  // 時刻を読み取れる根拠を持たない値は、既定の時刻を出さない。
  expect(screen.getAllByText("時刻なし").length).toBeGreaterThan(0);
});

// **値の種類を全件返す。** 数えた個数と表の行数が食い違う応答は読めない。
test("値の種類の件数と表の行数が一致する", async () => {
  stubFetch({
    ...graphResponseJson(),
    countBy: "http.user_agent",
    valueCounts: [valueCountJson(), valueCountWithoutTimeJson()],
    distinctValueCount: 2,
  });

  renderGraphExplore();
  const table = await screen.findByRole("table", {
    name: "フィールドの値ごとのレコード数",
  });

  expect(within(table).getAllByRole("row")).toHaveLength(3);
  expect(getPair("値の種類: 2")).toBeTruthy();
});

test("数えるフィールドを与えない応答では、値ごとの件数の表を出さない", async () => {
  stubFetch();

  renderGraphExplore();
  await findNodeList();

  expect(
    screen.queryByRole("table", { name: "フィールドの値ごとのレコード数" }),
  ).toBeNull();
});

test("値ごとの件数が 0 件で、フィールドはあるがフィルタで除外した理由を出す", async () => {
  stubFetch({
    ...graphResponseJson(),
    countBy: "http.user_agent",
    valueCounts: [],
    distinctValueCount: 0,
    valueCountsEmptyReason: "no_value_in_filter",
  });

  renderGraphExplore();
  await findNodeList();

  expect(screen.getByText("フィルタで除外")).toBeTruthy();
});

test("アドレスの範囲の文字列を拒み、直す入力欄を示す", async () => {
  const mock = stubFetch();

  renderGraphExplore();
  await findNodeList();
  const requestsBefore = mock.mock.calls.length;

  addCondition("CIDR の外のアドレス", {
    "CIDR の外のアドレス": "198.51.100.0",
  });

  const rejection = "CIDR の形式の誤り";
  expect(screen.getByText(rejection)).toBeTruthy();
  expect(screen.getByLabelText("CIDR の外のアドレス")).toHaveAttribute(
    "aria-invalid",
    "true",
  );
  // 拒んだ要求を送らない。
  expect(mock.mock.calls.length).toBe(requestsBefore);

  // 欄を直している間は欄の指摘を残さない。
  fireEvent.change(screen.getByLabelText("CIDR の外のアドレス"), {
    target: { value: "198.51.100.0/28" },
  });
  expect(screen.queryByText(rejection)).toBeNull();
});

test("応答が用いたアドレスの範囲を画面に出す", async () => {
  stubFetch({
    ...graphResponseJson(),
    addressInCidr: "198.51.100.0/24",
    addressNotInCidr: "198.51.100.0/28",
  });

  renderGraphExplore();
  await findNodeList();

  expect(getPair("CIDR の内のアドレス: 198.51.100.0/24")).toBeTruthy();
  expect(getPair("CIDR の外のアドレス: 198.51.100.0/28")).toBeTruthy();
});

test("アドレスの範囲が空の追加では、誤りを出して範囲を要求に載せない", async () => {
  const mock = stubFetch();

  renderGraphExplore();
  await findNodeList();
  addCondition("CIDR の内のアドレス", {});

  expect(screen.getByText("入力なし")).toBeTruthy();
  expect(lastFigureRequest(mock)).toBe(initialFigureRequest);
});

test("期間を入力して適用すると、要求の timeFilter に載せる", async () => {
  const mock = stubFetch();

  renderGraphExplore();
  await findNodeList();

  // 終わりの秒の小数部の 3 桁から、終わりの精度と比べる単位をミリ秒にする。
  addCondition("期間", {
    始まりの時刻: "2031-10-08T10:20:35+09:00",
    終わりの時刻: "2031-10-08T11:05:48.500+09:00",
  });

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}&depth=1` +
        "&timeFrom=2031-10-08T10%3A20%3A35%2B09%3A00&timeFromPrecision=second" +
        "&timeTo=2031-10-08T11%3A05%3A48.500%2B09%3A00&timeToPrecision=millisecond" +
        "&filterUnit=millisecond" +
        matchConditionQuery,
    ),
  );
});

test("秒の小数部の桁数が合わない時刻の文字列は送らず、どの端の文字列がどの形であるべきかを示す", async () => {
  const mock = stubFetch();

  renderGraphExplore();
  await findNodeList();
  const before = figureRequests(mock).length;

  addCondition("期間", { 始まりの時刻: "2031-10-08T10:20:35.8Z" });

  expect(screen.getByText("時刻の形式の誤り")).toBeTruthy();
  expect(screen.getByLabelText("始まりの時刻")).toHaveAttribute(
    "aria-invalid",
    "true",
  );
  expect(figureRequests(mock).length).toBe(before);

  // 文字列を直すと、古い警告を消す。
  fireEvent.change(screen.getByLabelText("始まりの時刻"), {
    target: { value: "2031-10-08T10:20:35.800Z" },
  });
  expect(screen.queryByText("時刻の形式の誤り")).toBeNull();
});

test("終わりだけを入力して適用すると、要求の timeFilter に載せる", async () => {
  const mock = stubFetch();

  renderGraphExplore();
  await findNodeList();

  addCondition("期間", { 終わりの時刻: "2031-10-08T11:05:48+09:00" });

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}&depth=1` +
        "&timeTo=2031-10-08T11%3A05%3A48%2B09%3A00&timeToPrecision=second" +
        "&filterUnit=second" +
        matchConditionQuery,
    ),
  );
});

test("期間の文字列が空の適用では timeFilter を要求に載せない", async () => {
  const mock = stubFetch();

  renderGraphExplore();
  await findNodeList();
  addCondition("期間", {});

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(initialFigureRequest),
  );
});

test("応答が用いた期間の条件を出す", async () => {
  const response = graphResponseJson();
  stubFetch({
    ...response,
    timeFrom: {
      requestText: "2031-10-08T10:20:35+09:00",
      precision: "second",
      offsetState: "in_value",
    },
    timeTo: {
      requestText: "2031-10-08T11:05:48.500+09:00",
      precision: "millisecond",
      offsetState: "in_value",
    },
    filterUnit: "millisecond",
  });

  renderGraphExplore();

  expect(
    await findPair(
      "期間: 2031-10-08T10:20:35+09:00 – 2031-10-08T11:05:48.500+09:00",
    ),
  ).toBeTruthy();
  expect(getPair("始まりの精度: 秒")).toBeTruthy();
  expect(getPair("終わりの精度: ミリ秒")).toBeTruthy();
  expect(getPair("比較の単位: ミリ秒単位")).toBeTruthy();
});

// 期間のフィルタを適用した結果が 0 件のとき、適用した期間が画面から消えると、分析者は 0 件が
// どの期間の結果かを読めない。
test("0 件の応答でも、server が用いた期間の条件を出す", async () => {
  stubFetch({
    ...emptyGraphResponseJson(),
    timeFrom: {
      requestText: "2031-10-09T00:00:00+09:00",
      precision: "second",
      offsetState: "in_value",
    },
    filterUnit: "second",
  });

  renderGraphExplore();

  expect(
    await screen.findByText("フィルタに一致するレコードなし"),
  ).toBeTruthy();
  expect(getPair("期間: 2031-10-09T00:00:00+09:00 – ")).toBeTruthy();
  expect(getPair("始まりの精度: 秒")).toBeTruthy();
  expect(getPair("比較の単位: 秒単位")).toBeTruthy();
  expect(document.body.textContent).not.toContain("終わりの精度");
  // 0 件と期間を 1 つの通知で読み上げる。
  const status = screen
    .getAllByRole("status")
    .find((element) => element.textContent?.includes("一致ノード: 0"));
  expect(status?.textContent).toContain("期間: 2031-10-09T00:00:00+09:00");
  // 図へ空の入力を渡さない。
  expect(canvasSpy).not.toHaveBeenCalled();
});

// 期間のフィルタを適用して 0 件になったとき、前の部分グラフで選んだノードの詳細を外す。古い選択に対する
// 応答を現在の詳細に出さない。0 件の応答が取得の状態へまとめられると、この解除が走らなくなる。
test("期間のフィルタを適用して 0 件になると、選んでいたノードの詳細を外す", async () => {
  const mock = stubFetch();

  renderGraphExplore();
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );
  await screen.findByRole("table", { name: "属性" });

  // 呼び出しごとに新しい Response を作る。body は 1 度しか読めない。
  mock.mockImplementation(async () =>
    jsonResponse(200, emptyGraphResponseJson()),
  );
  addCondition("期間", { 始まりの時刻: "2031-10-09T00:00:00+09:00" });

  await screen.findByText("フィルタに一致するレコードなし");
  expect(lastFigureRequest(mock)).toContain(
    "timeFrom=2031-10-09T00%3A00%3A00%2B09%3A00",
  );
  await waitFor(() => expect(screen.getByText("ノード未選択")).toBeTruthy());
  expect(screen.queryByRole("table", { name: "属性" })).toBe(null);
});

test("ノードが 0 件のときに 0 件であることを書く", async () => {
  stubFetch(emptyGraphResponseJson());

  renderGraphExplore();

  expect(
    await screen.findByText("フィルタに一致するレコードなし"),
  ).toBeTruthy();
  expect(screen.queryByRole("table", { name: "グラフのノードの一覧" })).toBe(
    null,
  );
});

test("期間のフィルタを適用した結果が 0 件のときに 0 件であることを書く", async () => {
  const mock = stubFetch(emptyGraphResponseJson());

  renderGraphExplore();
  await screen.findByText("フィルタに一致するレコードなし");
  addCondition("期間", { 始まりの時刻: "2031-10-08T10:20:35+09:00" });

  expect(
    await screen.findByText("フィルタに一致するレコードなし"),
  ).toBeTruthy();
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toContain(
      "timeFrom=2031-10-08T10%3A20%3A35%2B09%3A00",
    ),
  );

  // 適用している期間を入れた値の入力で、下端を空にして足すと期間を外す。
  addCondition("期間", { 始まりの時刻: "" });

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(initialFigureRequest),
  );
});

test("取得が失敗したときに、code に応じた文言と次に行える操作を出す", async () => {
  stubFetch(jsonResponse(500, apiErrorJson("internal_error")));

  renderGraphExplore();

  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain("グラフの取得");
  expect(alert.textContent).toContain("サーバーの内部の失敗");
  expect(alert.textContent).toContain("次の操作: 時間を空けて再実行");
  expect(alert.textContent).toContain("コード: internal_error");
  expect(screen.queryByRole("table", { name: "グラフのノードの一覧" })).toBe(
    null,
  );
  // Nodes と Edges は、失敗の状態だけを出し、理由は Search の結果が出す。
  expect(screen.getAllByText("グラフの取得に失敗")).toHaveLength(2);
});

test("ノードの詳細の取得が失敗したときに、図と一覧を残したまま失敗を出す", async () => {
  stubFetch(
    graphResponseJson(),
    jsonResponse(404, apiErrorJson("record_not_found")),
  );

  renderGraphExplore();
  await findNodeList();

  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );

  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain("ノードの詳細の取得");
  expect(alert.textContent).toContain("コード: record_not_found");
  expect(
    screen.getByRole("table", { name: "グラフのノードの一覧" }),
  ).toBeTruthy();
});

test("表示名が持つ bidi 制御を可視の符号にして出す", async () => {
  stubFetch(invisibleCharacterGraphResponseJson());

  renderGraphExplore();
  await findNodeList();

  expect(
    await screen.findByRole("button", {
      name: "端末 HOST-U+202EC の詳細を開く",
    }),
  ).toBeTruthy();

  // 隣接ノードの表示元を外す button の名前も、可視の符号にして出す。
  fireEvent.click(
    screen.getByRole("button", {
      name: "端末 HOST-U+202EC の隣接ノードを追加",
    }),
  );
  expect(
    await screen.findByRole("button", {
      name: "HOST-U+202EC の隣接ノードを非表示",
    }),
  ).toBeTruthy();
});

// レビュアが測った症状の再現である。同じ id のノードを 2 件含む応答で、画面全体が
// 消えた。decode が応答を退けるため、失敗の表示と見出しが残る。
test("同じ id のノードを 2 件含む応答で、画面を白紙にしない", async () => {
  const response = graphResponseJson();
  const [first, ...rest] = response.nodes;
  // 重ねた 1 件は合った端末である。件数と種別ごとの件数は重ねた後の値に揃え、応答の不正を
  // id の重なりだけにする。
  stubFetch({
    ...response,
    nodes: [first, ...rest, first],
    nodeCount: 3,
    matchedKinds: [
      { kind: "process", count: 1 },
      { kind: "terminal", count: 2 },
    ],
  });

  renderGraphExplore();

  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain("グラフの取得");
  expect(
    screen.getByRole("heading", { level: 2, name: "Search" }),
  ).toBeTruthy();
  expect(screen.getByRole("region", { name: "グラフ" })).toBeTruthy();
  expect(screen.getByRole("combobox", { name: "条件の種類" })).toBeTruthy();
});

// ノードを記録したレコードの有無は、参照だけで分かるノードのときに詳細だけが書く。
// fixture のプロセスは observed かつ作成レコードを持たない。
test("ノードの一覧に作成レコードの有無を出し、ノードを記録したレコードの有無は出さない", async () => {
  stubFetch();

  renderGraphExplore();

  const table = await findNodeList();
  expect(table.textContent).not.toContain("このノードを記録したレコード");
  const headers = within(table)
    .getAllByRole("columnheader")
    .map((header) => header.textContent);
  const column = headers.indexOf("作成レコード");
  const cells = within(table)
    .getAllByRole("row")
    .slice(1)
    .map((row) => row.children[column]?.textContent);
  expect(cells).toContain("なし");
  expect(cells).toContain("対象外の種類");
});

// nodeCreationRecords に無い値を含む応答を退ける。画面が知らない値を既定の表示へ寄せない。
test("生成の根拠の欄が nodeCreationRecords に無い値である応答を退ける", async () => {
  const response = graphResponseJson();
  const [first, ...rest] = response.nodes;
  stubFetch({
    ...response,
    nodes: [{ ...first, creationRecord: "undetermined" }, ...rest],
  });

  renderGraphExplore();

  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain("グラフの取得");
  expect(
    screen.queryByRole("table", { name: "グラフのノードの一覧" }),
  ).toBeNull();
});

test("エッジの一覧にエッジの種類と作り方と根拠の件数を出す", async () => {
  stubFetch();

  renderGraphExplore();

  const table = await findEdgeList();
  expect(table.textContent).toContain("端末で実行したプロセス");
  expect(within(table).getAllByText("観測").length).toBeGreaterThan(0);
  expect(table.textContent).toContain("プロセスの通信");
});

test("隣接ノードを追加するたびに要求の nodeId を重ね、表示元を外すと減らし、戻ると検索の要求へ戻る", async () => {
  const mock = stubFetch();

  renderGraphExplore();
  await findNodeList();

  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の隣接ノードを追加",
    }),
  );
  const processOrigin = "&nodeId=n%3Aprocess%3A8ab3";
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}${processOrigin}&depth=1` +
        matchConditionQuery,
    ),
  );

  // 取り直しで作り直した一覧は閉じているため、開き直す。
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: /203\.0\.113\.21 の隣接ノードを追加/,
    }),
  );
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toMatch(
      new RegExp(`${processOrigin}&nodeId=[^&]+&depth=1`),
    ),
  );
  expect(lastFigureRequest(mock)?.match(/nodeId=/g)?.length).toBe(2);

  fireEvent.click(
    screen.getByRole("button", {
      name: "C:\\Windows\\System32\\cmd.exe の隣接ノードを非表示",
    }),
  );
  await waitFor(() =>
    expect(lastFigureRequest(mock)).not.toContain(processOrigin),
  );
  expect(lastFigureRequest(mock)).toContain("&nodeId=");

  fireEvent.click(screen.getByRole("button", { name: "検索の結果に戻る" }));
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(initialFigureRequest),
  );
});

test("起点を 1 つだけ持つときに外すと、検索の要求へ戻る", async () => {
  const mock = stubFetch();

  renderGraphExplore();
  await findNodeList();

  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の隣接ノードを追加",
    }),
  );
  fireEvent.click(
    await screen.findByRole("button", {
      name: "C:\\Windows\\System32\\cmd.exe の隣接ノードを非表示",
    }),
  );
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(initialFigureRequest),
  );
});

test("エッジの種類を複数選ぶと、種類の定義元の順で要求の edgeKind に載せ、全部外すと載せない", async () => {
  const mock = stubFetch();

  renderGraphExplore();
  await findNodeList();
  // 選んだ順と逆でも、要求は種別の定義元の順に並ぶ。
  chooseConditionValue("エッジの種類", edgeKindLabels.process_communication);
  chooseConditionValue("エッジの種類", edgeKindLabels.file_operation);
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}&depth=1` +
        "&edgeKind=file_operation&edgeKind=process_communication" +
        matchConditionQuery,
    ),
  );

  // 全部外すと種別の条件が無くなり、条件の無い初めの表示に戻る。
  for (const kind of ["process_communication", "file_operation"] as const) {
    fireEvent.click(
      screen.getByRole("button", {
        name: `エッジの種類 ${edgeKindLabels[kind]} を削除`,
      }),
    );
  }
  await waitFor(() =>
    expect(lastFigureRequest(mock)).not.toContain("edgeKind="),
  );
  expect(lastFigureRequest(mock)).toContain("nodeKind=terminal");
});

test("両端のレコードが期間の中にあるエッジだけを出すを選ぶと、要求に endpointRecordsInPeriod を載せ、外すと載せない", async () => {
  const mock = stubFetch();
  renderGraphExplore();
  await findNodeList();
  chooseConditionKind("両端のレコードが期間内");
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toContain("&endpointRecordsInPeriod=true"),
  );
  fireEvent.click(
    screen.getByRole("button", {
      name: "表示するエッジ 両端のレコードが期間内 を削除",
    }),
  );
  await waitFor(() =>
    expect(lastFigureRequest(mock)).not.toContain("endpointRecordsInPeriod"),
  );
});

/** 関連付けの条件の値の入力を開いた画面を描く。 */
async function renderForMatchConditions() {
  const mock = stubFetch();
  renderGraphExplore();
  await findNodeList();
  chooseConditionKind("推定条件");
  return mock;
}

test("条件を選んで適用すると、選んだ条件だけを要求に載せる", async () => {
  const mock = await renderForMatchConditions();

  fireEvent.click(screen.getByLabelText("接続先 port"));
  fireEvent.click(screen.getByRole("button", { name: "推定条件を適用" }));

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${terminalIpViewQuery}&depth=1` +
        "&matchCondition=destination_ip&matchCondition=destination_port",
    ),
  );
  // 端末の選択肢も、同じ選択で取り直す。
  await waitFor(() =>
    expect(
      mock.mock.calls
        .map((call) => call[0])
        .filter((path) => path.startsWith(terminalListPrefix))
        .at(-1),
    ).toBe(
      `${terminalListPrefix}matchCondition=destination_ip&matchCondition=destination_port`,
    ),
  );
});

test("条件を 1 つも選ばない適用を退け、要求を送らない", async () => {
  const mock = await renderForMatchConditions();
  const before = mock.mock.calls.length;

  fireEvent.click(screen.getByLabelText("接続先 IP"));
  fireEvent.click(screen.getByRole("button", { name: "推定条件を適用" }));

  expect(
    screen.getByRole("alert").textContent?.includes("推定条件の選択なし"),
  ).toBe(true);
  expect(mock.mock.calls.length).toBe(before);
});

test("秒単位の時刻を選ぶと許容幅の入力欄が出て、許容幅を要求に載せる", async () => {
  const mock = await renderForMatchConditions();

  expect(screen.queryByLabelText("秒単位の時刻の許容幅の秒数")).toBeNull();

  fireEvent.click(screen.getByLabelText("秒単位の時刻"));
  fireEvent.change(screen.getByLabelText("秒単位の時刻の許容幅の秒数"), {
    target: { value: "3" },
  });
  fireEvent.click(screen.getByRole("button", { name: "推定条件を適用" }));

  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${terminalIpViewQuery}&depth=1` +
        "&matchCondition=destination_ip&matchCondition=second_of_time%7E3",
    ),
  );

  // 適用した選択を入れた値の入力で、秒単位の時刻を外すと、幅の欄が消える。
  chooseConditionKind("推定条件");
  expect(screen.getByLabelText("秒単位の時刻の許容幅の秒数")).toHaveValue("3");
  fireEvent.click(screen.getByLabelText("秒単位の時刻"));
  expect(screen.queryByLabelText("秒単位の時刻の許容幅の秒数")).toBeNull();
});

test("導いた表示名に、導いた値であることと導き方を添える", async () => {
  const response = graphResponseJson();
  const [terminal, ...rest] = response.nodes;
  stubFetch({
    ...response,
    nodes: [
      {
        ...terminal,
        label: {
          normalized: "report-0001.log",
          derivation: "file.path の末尾の要素から導いた",
          valueState: "derived",
        },
      },
      ...rest,
    ],
  });

  renderGraphExplore();

  const list = await findNodeList();
  expect(list.textContent).toContain(
    "Oraculum が作った表示名\n作り方: file.path の末尾の要素から導いた",
  );
  // button の名前には値と種別だけを入れる。導き方の一文を読み上げが繰り返さない。
  expect(
    screen.getByRole("button", { name: "端末 report-0001.log の詳細を開く" }),
  ).toBeTruthy();
  expect(
    screen.getByRole("button", {
      name: "端末 report-0001.log の隣接ノードを追加",
    }),
  ).toBeTruthy();
});

test("エッジの一覧の端点に、表示名と識別鍵の値を出す", async () => {
  const response = graphResponseJson();
  const [terminal, process, ip] = response.nodes;
  // 同じ表示名を持つ別のノードを 2 件返す応答にする。表示名だけでは見分けられない。
  const twin = {
    ...ip,
    id: "n:ip:twin",
    identity: [{ value: "198.51.100.7" }],
  };
  stubFetch({
    ...response,
    nodes: [terminal, process, ip, twin],
    edges: [
      ...response.edges,
      {
        ...response.edges[1],
        id: "e:process_communication:0003",
        targetNodeId: "n:ip:twin",
      },
    ],
    edgeCount: 3,
  });

  renderGraphExplore();

  const table = await findEdgeList();
  // 2 本の process_communication は同じ表示名の端点を持ち、識別鍵の値で分かれる。
  expect(table.textContent).toContain("203.0.113.21");
  expect(table.textContent).toContain("198.51.100.7");
});

test("ノードの一覧に、根拠が記録した端末を独立した列として出す", async () => {
  stubFetch();

  renderGraphExplore();

  const table = await findNodeList();
  const header = within(table).getAllByRole("row")[0];
  expect(header?.textContent).toContain("記録した端末");
  const rows = within(table).getAllByRole("row").slice(1);
  // 端末を記録したノードは表示名を出す。
  const process = rows.find((row) => row.textContent?.includes("cmd.exe"));
  expect(process?.textContent).toContain("HOST-C");
  // 記録していないノードは、記録していないことを出す。空のセルにしない。
  const endpoint = rows.find((row) =>
    row.textContent?.includes("203.0.113.21"),
  );
  expect(endpoint?.textContent).toContain("端末の記録なし");
});

/** 描画するノードの上限の選択欄。 */
function drawLimitSelect(): HTMLSelectElement {
  return screen.getByLabelText("描画するノードの上限") as HTMLSelectElement;
}

test("描画するノードの上限を選ぶと要求の nodeLimit が変わり、関係先の起点を外さない", async () => {
  const mock = stubFetch();

  renderGraphExplore();
  await findNodeList();
  // 既定は上限を置かず、図の要求に nodeLimit を載せない。
  expect(drawLimitSelect().value).toBe("all");
  expect(
    mock.mock.calls.some(([input]) => String(input).includes("nodeLimit")),
  ).toBe(false);

  fireEvent.click(
    screen.getByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の隣接ノードを追加",
    }),
  );
  const origin = `&nodeId=${encodeURIComponent(processNodeId)}`;
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}${origin}&depth=1${matchConditionQuery}`,
    ),
  );

  fireEvent.change(drawLimitSelect(), { target: { value: "500" } });
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?nodeLimit=500${objectViewQuery}${origin}` +
        `&depth=1${matchConditionQuery}`,
    ),
  );

  // すべてを選ぶと、上限を置かない要求を送る。
  fireEvent.change(drawLimitSelect(), { target: { value: "all" } });
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?${limitQuery}${objectViewQuery}${origin}` +
        `&depth=1${matchConditionQuery}`,
    ),
  );
  expect(String(mock.mock.calls.at(-1)?.[0])).not.toContain("nodeLimit");
  expect(screen.queryByRole("alert")).toBeNull();
});

/** 表の本体の行を、セルの文字列の並びで返す。 */
function bodyRows(table: HTMLElement): string[][] {
  return within(table)
    .getAllByRole("row")
    .map((row) =>
      within(row)
        .queryAllByRole("cell")
        .map((cell) => (cell.textContent ?? "").trim()),
    )
    .filter((cells) => cells.length > 0);
}

test("図に描くノードが上限を超える応答は、図を描かず、Graph に件数と上限だけを出し、Nodes に種類ごとの件数と合ったノードの一覧を出す", async () => {
  stubFetch(overLimitGraphResponseJson(350, 200));

  renderGraphExplore();

  // Graph は描画しなかった件数と上限の値の組と、上限を上げる操作だけを出す。
  const withheld = await screen.findByRole("region", { name: "描画上限超過" });
  expect(
    within(withheld)
      .getAllByRole("listitem")
      .map((item) => item.textContent),
  ).toEqual(["対象ノード: 350", "上限: 200"]);
  expect(
    within(withheld).getByRole("button", { name: "上限を上げて描画" }),
  ).toBeTruthy();
  expect(within(withheld).queryByRole("table")).toBeNull();

  // Nodes は一致ノードの種類ごとの件数と、描くはずだったエッジの種類ごとの本数を表で出す。
  const kinds = screen.getByRole("table", { name: "一致ノードの種類" });
  expect(bodyRows(kinds).map((cells) => cells.slice(0, 2))).toContainEqual([
    "プロセス",
    "1",
  ]);
  const edgeRows = bodyRows(
    screen.getByRole("table", { name: "エッジの種類" }),
  );
  expect(edgeRows).toContainEqual(["端末で実行したプロセス", "1,200"]);
  expect(edgeRows).toContainEqual(["プロセスの通信", "34"]);
  // Nodes は一致ノードの一覧を出す。応答がエッジを持たないため、Edges は上限を超えた状態を出す。
  const heading = screen.getByRole("heading", { name: "一致ノード" });
  expect(heading.closest("section")?.textContent).toContain("件数: 2");
  const nodes = await screen.findByRole("table", {
    name: "グラフのノードの一覧",
  });
  expect(within(nodes).getAllByRole("row")).toHaveLength(3);
  expect(
    screen.queryByRole("table", { name: "グラフのエッジの一覧" }),
  ).toBeNull();
  expect(screen.getByText("描画の上限を超過")).toBeTruthy();
  // 書き出しは一致ノードの全件を要求するため、図を描かない応答でも出す。
  expect(
    screen.getByRole("button", { name: "一致ノードを CSV に書き出す" }),
  ).toBeTruthy();
});

test("上限に収まる応答は、上限を超えたことを出さずに図と一覧を描く", async () => {
  stubFetch({ ...graphResponseJson(), subgraphNodeCount: 200, nodeLimit: 200 });

  renderGraphExplore();

  await findNodeList();
  expect(screen.queryByRole("region", { name: "描画上限超過" })).toBeNull();
  expect(screen.queryByRole("table", { name: "一致ノードの種類" })).toBeNull();
});

test("上限を超えた画面から、上限を上げる操作と、種別を 1 つだけグラフに出す操作で要求を変える", async () => {
  const mock = stubFetch(overLimitGraphResponseJson(350, 200));

  renderGraphExplore();
  fireEvent.change(drawLimitSelect(), { target: { value: "200" } });

  // 350 件を描ける最も小さい上限は 500 である。
  fireEvent.click(
    await screen.findByRole("button", { name: "上限を上げて描画" }),
  );
  await waitFor(() => expect(drawLimitSelect().value).toBe("500"));
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      `/api/v0/graph?nodeLimit=500${terminalIpViewQuery}` +
        `&depth=1${matchConditionQuery}`,
    ),
  );

  fireEvent.change(drawLimitSelect(), { target: { value: "200" } });
  fireEvent.click(
    await screen.findByRole("button", { name: "プロセスだけをグラフに表示" }),
  );
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      "/api/v0/graph?nodeLimit=200&nodeKind=process&granularity=object" +
        `&depth=1${matchConditionQuery}`,
    ),
  );
});

test("グラフに出す対象が 1 つの種別だけのとき、上限を超えた画面にその種別だけを出す操作を出さない", async () => {
  const mock = stubFetch(overLimitGraphResponseJson(350, 200));

  renderGraphExplore();
  fireEvent.change(drawLimitSelect(), { target: { value: "200" } });
  fireEvent.click(
    await screen.findByRole("button", { name: "プロセスだけをグラフに表示" }),
  );
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toBe(
      "/api/v0/graph?nodeLimit=200&nodeKind=process&granularity=object" +
        `&depth=1${matchConditionQuery}`,
    ),
  );

  // 押しても要求が変わらない操作を出さない。別の種別と上限を上げる操作は残す。
  await screen.findByRole("region", { name: "描画上限超過" });
  expect(
    screen.queryByRole("button", { name: "プロセスだけをグラフに表示" }),
  ).toBeNull();
  expect(
    screen.getByRole("button", { name: "端末だけをグラフに表示" }),
  ).toBeTruthy();
  expect(screen.getByRole("button", { name: "上限を上げて描画" })).toBeTruthy();
});

test("上限を超えた Graph の Nodes を表示する操作は、Nodes のビューを前面に出す", async () => {
  stubFetch(overLimitGraphResponseJson(350, 200));
  const onShowPane = vi.fn();
  render(
    <GraphExploreHarness
      onSelectRecord={() => {}}
      selectedEdgeId={undefined}
      onSelectEdge={() => {}}
      assertions={() => null}
      onShowPane={onShowPane}
    />,
  );

  const withheld = await screen.findByRole("region", { name: "描画上限超過" });
  fireEvent.click(
    within(withheld).getByRole("button", { name: "Nodes を表示" }),
  );

  expect(onShowPane).toHaveBeenCalledWith("nodes");
});

// レコードのノードから、そのレコードを元にした接続先の推定の結果を読む。
test("レコードのノードに、接続先の推定の結果と、判定の根拠を出す", async () => {
  stubFetch(graphResponseJson(), recordNodeDetailResponseJson());

  renderGraphExplore();
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );

  expect(
    await screen.findByRole("heading", { name: "接続先の推定" }),
  ).toBeTruthy();
  expect(getPair("結果: 時刻の一致で候補なし")).toBeTruthy();
  // 原資料の事実と、Oraculum が処理できなかったことを読み分ける材料を出す。
  expect(getPair("判定の根拠: 原資料の事実")).toBeTruthy();
});

// レコード以外の種類のノードは、セクションそのものを出さない。
test("レコード以外のノードに、接続先の推定のセクションを出さない", async () => {
  stubFetch();

  renderGraphExplore();
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );
  await screen.findByRole("table", { name: "エッジの件数" });

  expect(screen.queryByRole("heading", { name: "接続先の推定" })).toBeNull();
});

// 個々のノードを選ぶと、ノードの詳細を要求する。
test("個々のノードを選ぶと、ノードの詳細を要求する", async () => {
  const mock = stubFetch(graphResponseJson());

  renderGraphExplore();
  const table = await findNodeList();

  fireEvent.click(
    within(table).getAllByRole("button", {
      name: /の詳細を開く$/,
    })[0] as HTMLElement,
  );

  await waitFor(() =>
    expect(
      mock.mock.calls.filter((call) =>
        String(call[0]).startsWith("/api/v0/nodes/"),
      ).length,
    ).toBeGreaterThan(0),
  );
});

test("図と同じ条件で候補を取得し、選択edgeの根拠だけを原文表示へ渡す", async () => {
  const onSelectRecord = vi.fn();
  const assertions = vi.fn(() => null);
  const candidateEdge = candidateMatch.edges[0];
  if (candidateEdge === undefined)
    throw new Error("missing synthetic candidate edge");
  const response = candidateResponse([
    {
      ...candidateMatch,
      matchId: "match:ran_on:0001",
      edges: [{ ...candidateEdge, edgeId: "e:ran_on:0001" }],
    },
    {
      ...candidateMatch,
      matchId: "match:process_communication:0002",
      edges: [{ ...candidateEdge, edgeId: "e:process_communication:0002" }],
    },
  ]);
  const graphMock = stubFetch();
  const mock = vi.fn(async (input: string, init?: RequestInit) =>
    input.startsWith("/api/v0/attack-candidates")
      ? jsonResponse(200, response)
      : graphMock(input, init),
  );
  vi.stubGlobal("fetch", mock);
  render(
    <GraphExploreHarness
      onSelectRecord={onSelectRecord}
      selectedEdgeId="e:ran_on:0001"
      onSelectEdge={() => {}}
      assertions={assertions}
    />,
  );

  const selected = await screen.findByRole("region", {
    name: "選択中のエッジの ATT&CK 候補",
  });
  expect(selected.textContent).toContain("e:ran_on:0001");
  expect(selected.textContent).not.toContain("e:process_communication:0002");
  const candidates = mock.mock.calls
    .map(([input]) => input)
    .filter((input) => input.startsWith("/api/v0/attack-candidates?"))
    .at(-1);
  expect(candidates?.replace("/api/v0/attack-candidates?", "")).toBe(
    sentRequest(figureRequests(mock).at(-1) ?? "").replace(
      "/api/v0/graph?",
      "",
    ),
  );
  fireEvent.click(
    within(selected).getByRole("button", {
      name: /synthetic.log.*行: 7(\D|$)/,
    }),
  );
  expect(onSelectRecord).toHaveBeenCalledWith(
    candidateMatch.edges[0]?.evidence[0],
  );
  expect(assertions).not.toHaveBeenCalled();
  expect(
    mock.mock.calls.every(
      ([, init]) => init?.method === undefined || init.method === "GET",
    ),
  ).toBe(true);
});

test("図の取得が失敗しても、取得できた ATT&CK の候補を出す", async () => {
  const graphMock = stubFetch(
    jsonResponse(500, apiErrorJson("internal_error")),
  );
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string, init?: RequestInit) =>
      input.startsWith("/api/v0/attack-candidates")
        ? jsonResponse(200, candidateResponse([candidateMatch]))
        : graphMock(input, init),
    ),
  );
  renderGraphExplore();

  await screen.findByText("描画なし");
  const heading = await screen.findByRole("heading", { name: "ATT&CK 候補" });
  const rules = within(heading.closest("section") as HTMLElement).getByRole(
    "table",
    { name: "ATT&CK のルール" },
  );
  const row = within(rules)
    .getAllByRole("row")
    .find((element) => element.textContent?.includes(candidateRule.title));
  expect(row?.textContent).toContain("1");
});

test("収集元を選ぶと要求の source に載せ、応答が用いた収集元を区別できる表示名で出す", async () => {
  const mock = stubFetch({
    ...graphResponseJson(),
    source: ["src-a1", "src-unknown"],
  });
  render(
    <GraphExploreHarness
      onSelectRecord={() => {}}
      selectedEdgeId={undefined}
      onSelectEdge={() => {}}
      assertions={() => null}
      sourceFileNames={
        new Map([
          ["src-a1", "same.log"],
          ["src-b2", "same.log"],
          ["src-c3", "other.log"],
        ])
      }
    />,
  );
  await findNodeList();

  // 同じ file 名の収集元は、sourceId の先頭を添えて区別する。
  expect(conditionValueOptions("Artifact")).toEqual([
    "same.log src-a1",
    "same.log src-b2",
    "other.log",
  ]);
  chooseConditionValue("Artifact", "same.log src-a1");
  await waitFor(() =>
    expect(lastFigureRequest(mock)).toContain("&source=src-a1&"),
  );
  // 応答が用いた収集元は表示名で出し、探せない sourceId はそのまま出す。
  const result = document.querySelector(".result-pane") as HTMLElement;
  await waitFor(() =>
    expect(getPair("Artifact: same.log src-a1", result)).toBeTruthy(),
  );
  expect(getPair("Artifact: src-unknown", result)).toBeTruthy();
});
