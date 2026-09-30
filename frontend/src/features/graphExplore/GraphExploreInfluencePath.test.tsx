// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, test, vi } from "vitest";
import {
  graphResponseJson,
  ipNodeId,
  nodeDetailResponseJson,
  processNodeId,
  terminalNodeId,
} from "@/testdata/graph/graphResponse";
import {
  graphNodeJson,
  influencePathResponseJson,
} from "@/testdata/graph/influencePathResponse";
import { jsonResponse } from "@/testdata/http";
import type { GraphExplorePanes } from "./GraphExplore";
import {
  type FetchMock,
  findNodeList,
  findPair,
  GraphExploreHarness,
  getPair,
  renderGraphExplore,
  stubFetch,
} from "./graphExploreTestHarness";
import { defaultExcludedBases } from "./InfluencePathPane";

// WebGL の renderer は jsdom で動かない。本 test は図を stub に置き換える。
vi.mock("./SubgraphCanvas", () => ({
  SubgraphCanvas: ({
    drawing,
    selectedNodeId,
    onSelectNode,
  }: {
    drawing: {
      points: { id: string }[];
      links: { id: string; state: string }[];
    };
    selectedNodeId?: string;
    onSelectNode: (id: string) => void;
  }) => (
    <div>
      <p>影響の経路の図</p>
      <p>{`選んでいる点: ${selectedNodeId ?? "無し"}`}</p>
      <p>{`点: ${drawing.points.map((point) => point.id).join(",")}`}</p>
      <p>{`線: ${drawing.links.map((link) => `${link.id}=${link.state}`).join(",")}`}</p>
      {drawing.points.map((point) => (
        <button
          key={point.id}
          type="button"
          onClick={() => onSelectNode(point.id)}
        >
          {`点 ${point.id} を押す`}
        </button>
      ))}
    </div>
  ),
}));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const graphNode = graphNodeJson;
const pathJson = influencePathResponseJson;

const evidence = nodeDetailResponseJson().evidence;

/** 経路の上の要素と、先へ進まない要素と、不確定の連鎖の影響のエッジを持つ応答。 */
function pathWithFrontierJson() {
  const base = pathJson();
  return {
    ...base,
    edges: [
      {
        ...base.edges[0],
        bases: ["specified_operation", "uncertain_chain", "single_node"],
        candidateCount: 5,
        candidateTally: { candidateCount: 3, precedingCandidateCount: 2 },
        candidateOrder: ["session_account_different", "session_logon_network"],
      },
    ],
    frontier: [
      {
        key: "v:f",
        node: graphNode(terminalNodeId),
        influenceEdgeCount: 1,
        enteredFrom: graphNode(processNodeId),
        reason: "no_outgoing_influence",
      },
    ],
    omittedEdgeCount: 12,
  };
}

function emptyPathJson() {
  return {
    ...pathJson(),
    edges: [],
    stops: ["time_order_unsatisfied"],
    frontier: [],
    frontierCount: 0,
    untimedRecordCount: 0,
  };
}

/** 起点がタイムスタンプを持たず、backend が数える前に止まった応答。起点と終点を持たない。 */
function untimedOriginJson() {
  const { origin, destination, ...rest } = emptyPathJson();
  void origin;
  void destination;
  return {
    ...rest,
    vertices: [],
    stops: ["origin_without_timestamp"],
  };
}

/**
 * 影響の経路の要求に answer を返す。端末と IP アドレスの詳細の要求には、そのノードを返す。
 */
function stubPathFetch(answer: () => Response): FetchMock {
  const mock = stubFetch(graphResponseJson());
  const fallback = mock.getMockImplementation();
  mock.mockImplementation(async (input: string, init?: RequestInit) => {
    for (const id of [terminalNodeId, ipNodeId]) {
      if (input.startsWith(`/api/v0/nodes/${encodeURIComponent(id)}`)) {
        return jsonResponse(200, {
          ...nodeDetailResponseJson(),
          node: graphNode(id),
        });
      }
    }
    if (input.startsWith("/api/v0/influence-path")) {
      return answer();
    }
    if (fallback === undefined) throw new Error("the stub has no fallback");
    return fallback(input, init);
  });
  return mock;
}

function pathRequests(mock: FetchMock): URL[] {
  return mock.mock.calls
    .map(([input]) => input)
    .filter((input) => input.startsWith("/api/v0/influence-path"))
    .map((input) => new URL(input, "http://localhost"));
}

async function showPathFromProcessToTerminal(
  mock: FetchMock,
  onSelectRecord: Parameters<typeof renderGraphExplore>[0] = () => {},
) {
  renderGraphExplore(onSelectRecord);
  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );
  fireEvent.click(
    await screen.findByRole("button", { name: "影響の経路の起点に設定" }),
  );
  expect(
    screen.getByRole("button", { name: "影響の経路の起点に設定" }),
  ).toHaveAttribute("aria-disabled", "true");
  fireEvent.click(
    screen.getByRole("button", { name: /203\.0\.113\.21 の詳細を開く$/ }),
  );
  fireEvent.click(
    await screen.findByRole("button", {
      name: "影響の経路を表示: C:\\Windows\\System32\\cmd.exe から",
    }),
  );
  await waitFor(() => expect(pathRequests(mock)).toHaveLength(1));
}

test("端末のノードには、影響の経路の起点にする操作と、経路を表示する操作を出さない", async () => {
  stubPathFetch(() => jsonResponse(200, pathJson()));
  renderGraphExplore(() => {});
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
    screen.getByRole("button", { name: "端末 HOST-C の詳細を開く" }),
  );
  await screen.findByRole("button", { name: "この端末でフィルタ" });
  expect(
    screen.queryByRole("button", { name: /^影響の経路を表示: / }),
  ).toBeNull();
  expect(
    screen.queryByRole("button", { name: "影響の経路の起点に設定" }),
  ).toBeNull();
});

test("起点と終点を選ぶと影響の経路を要求し、影響のエッジの根拠と先へ進まないノードを出す", async () => {
  const mock = stubPathFetch(() => jsonResponse(200, pathJson()));

  await showPathFromProcessToTerminal(mock);

  const [request] = pathRequests(mock);
  expect(request?.searchParams.get("from")).toBe(processNodeId);
  expect(request?.searchParams.get("to")).toBe(ipNodeId);
  // 引数に現れた名前と接続先から作った影響のエッジを、画面を開いたときから除く。アカウントの管理操作と
  // 資格情報の使用は除かない。
  expect(request?.searchParams.getAll("excludeBasis")).toEqual([
    "argument_name",
    "requested_destination",
  ]);
  const table = await screen.findByRole("table", { name: "影響のエッジ" });
  // 経路に使った条件と使っていない条件を、Path のビューの値の組で出す。
  const path = screen.getByRole("region", { name: "影響の経路" });
  expect(path.textContent).toContain("推定条件: 使用");
  expect(path.textContent).toContain("検索の条件: 未使用");
  // Graph の図の上には、起点と終点の値の組だけを出す。
  const bar = document.querySelector(".pane-graph .figure-bar");
  expect(bar?.textContent).toContain("起点: C:\\Windows\\System32\\cmd.exe");
  expect(bar?.textContent).toContain("終点: 203.0.113.21");
  expect(bar?.textContent).not.toContain("推定条件");
  expect(table.textContent).toContain("入力形式の仕様の定める操作");
  expect(table.textContent).toContain("1 つの端末の時刻");
  expect(getPair("時刻の無い根拠のレコード: 3")).toBeTruthy();
  expect(
    screen.getByRole("table", { name: "先へ影響が進まないノード" }).textContent,
  ).toContain("先への影響の記録なし");
  expect(screen.getByText("影響の経路の図")).toBeTruthy();
});

test("先へ進まないノードを図に描かずに一覧に出し、不確定の連鎖の候補の並びと候補から入った要素と出していないエッジの数を示す", async () => {
  const mock = stubPathFetch(() => jsonResponse(200, pathWithFrontierJson()));

  await showPathFromProcessToTerminal(mock);

  expect(await screen.findByText("点: v:p,v:t")).toBeTruthy();
  expect(screen.getByText("線: s:1=uncertain_chain")).toBeTruthy();
  const table = screen.getByRole("table", { name: "影響のエッジ" });
  expect(getPair("同じ終点の候補: 5", table)).toBeTruthy();
  expect(getPair("上位の候補: 2", table)).toBeTruthy();
  expect(table.textContent).toContain("アカウントが不一致");
  expect(table.textContent).toContain("ネットワークのログオン");
  const frontier = screen.getByRole("table", {
    name: "先へ影響が進まないノード",
  });
  expect(frontier.textContent).toContain("入口のプロセス: ");
  expect(getPair("経路のエッジ: 13")).toBeTruthy();
  expect(getPair("表示したエッジ: 1")).toBeTruthy();
});

test("上限で切ったエッジだけで経路が成り立つと確かめられないときは、その理由を出す", async () => {
  const mock = stubPathFetch(() =>
    jsonResponse(200, {
      ...pathWithFrontierJson(),
      stops: ["truncated_route_unverified"],
    }),
  );

  await showPathFromProcessToTerminal(mock);

  const stops = await screen.findByRole("table", { name: "止まった理由" });
  expect(stops.textContent).toContain("表示の上限で経路が未確認");
});

test.each([
  ["ip", true],
  ["terminal", false],
])(
  "端にならない種別のノードを選んだときは、たどれないことと分けて示し、IP のときだけ IP から来たログオンを案内する (%s)",
  async (kind, guided) => {
    const mock = stubPathFetch(() =>
      jsonResponse(200, {
        ...emptyPathJson(),
        stops: ["destination_not_influence_end"],
        destinationNodeKind: kind,
      }),
    );

    await showPathFromProcessToTerminal(mock);

    const stops = await screen.findByRole("table", { name: "止まった理由" });
    const row = within(stops).getAllByRole("row")[1] as HTMLElement;
    expect(row.textContent).toContain("経路の端にならないノード");
    expect(row.textContent).toContain("終点");
    expect(
      row.textContent?.includes(
        "次の操作: その IP から来たログオンのレコードを終点に選択",
      ),
    ).toBe(guided);
    // たどれないことと分け、経路なしと出さない。
    expect(screen.getAllByText("対象外").length).toBeGreaterThan(0);
    expect(screen.queryByText("経路なし")).toBeNull();
  },
);

test("根拠の除外を解除して求め直した計算が上限に達したときは、そのことを Path に出し、図の枠には短いラベルだけを出す", async () => {
  const mock = stubPathFetch(() =>
    jsonResponse(200, {
      ...emptyPathJson(),
      stops: ["no_influence_route", "excluded_basis_route_limit"],
    }),
  );

  await showPathFromProcessToTerminal(mock);

  const stops = await screen.findByRole("table", { name: "止まった理由" });
  expect(stops.textContent).toContain("除外を解除した探索が上限に到達");
  expect(document.querySelector(".pane-graph .figure")?.textContent).toBe(
    "経路なし",
  );
});

test("根拠の除外を解除すると経路があるときは、その根拠を示す", async () => {
  const mock = stubPathFetch(() =>
    jsonResponse(200, {
      ...emptyPathJson(),
      stops: ["no_influence_route", "route_through_excluded_basis"],
      routeExcludedBases: ["argument_name"],
    }),
  );

  await showPathFromProcessToTerminal(mock);

  const stops = await screen.findByRole("table", { name: "止まった理由" });
  expect(stops.textContent).toContain("除外の解除で経路あり");
  expect(
    getPair("経路のエッジの除外した根拠: プロセスの引数に現れた名前", stops),
  ).toBeTruthy();
});

test("根拠のレコードを選ぶと、元レコードの表示へ渡す", async () => {
  const mock = stubPathFetch(() => jsonResponse(200, pathJson()));
  const onSelectRecord = vi.fn();

  await showPathFromProcessToTerminal(mock, onSelectRecord);
  const table = await screen.findByRole("table", { name: "影響のエッジ" });
  const [open] = within(table)
    .getAllByRole("button")
    .filter((button) => button.classList.contains("value-link"));
  if (open === undefined) throw new Error("no record button");
  fireEvent.click(open);

  expect(onSelectRecord).toHaveBeenCalledWith(evidence[0]?.recordRef);
});

test("経路の時刻を決めた起点と終点のレコードを出し、選ぶと元レコードの表示へ渡す", async () => {
  const mock = stubPathFetch(() => jsonResponse(200, pathJson()));
  const onSelectRecord = vi.fn();

  await showPathFromProcessToTerminal(mock, onSelectRecord);
  const ends = await screen.findByRole("table", {
    name: "経路の時刻を決めたレコード",
  });
  expect(ends.textContent).toContain("起点の最初");
  expect(ends.textContent).toContain("終点の最後");
  const [openOrigin] = within(ends).getAllByRole("button");
  if (openOrigin === undefined) throw new Error("no origin button");
  fireEvent.click(openOrigin);

  expect(onSelectRecord).toHaveBeenCalledWith(evidence[0]?.recordRef);
});

test("起点か終点の時刻を決めたレコードの収集元が経路のエッジの根拠に無いときは、そのことを出す", async () => {
  const json = pathJson();
  const origin = evidence[0];
  if (origin === undefined) throw new Error("no evidence");
  const otherSource = {
    ...origin,
    recordRef: {
      ...origin.recordRef,
      sourceId: "ingest-other-1",
      sourceFileName: "other.evtx",
    },
  };
  const mock = stubPathFetch(() =>
    jsonResponse(200, {
      ...json,
      origin: { key: "v:p", record: otherSource },
    }),
  );

  await showPathFromProcessToTerminal(mock);
  const ends = await screen.findByRole("table", {
    name: "経路の時刻を決めたレコード",
  });
  const [, originItem, destinationItem] = within(ends).getAllByRole("row");
  expect(originItem?.textContent).toContain("エッジの根拠と異なる");
  // 同じ端末の別の収集元でも収集元は違う。収集元の違いだけから、端末をまたぐと言わない。
  expect(originItem?.textContent).not.toContain("端末");
  expect(originItem?.textContent).not.toContain("省いた");
  expect(destinationItem?.textContent).not.toContain("エッジの根拠と異なる");
});

test("省いた影響のエッジがあるときは、収集元を比べたのは表に出したエッジだけであることを出す", async () => {
  const json = pathWithFrontierJson();
  const origin = evidence[0];
  if (origin === undefined) throw new Error("no evidence");
  const mock = stubPathFetch(() =>
    jsonResponse(200, {
      ...json,
      origin: {
        key: "v:p",
        record: {
          ...origin,
          recordRef: { ...origin.recordRef, sourceId: "ingest-other-1" },
        },
      },
    }),
  );

  await showPathFromProcessToTerminal(mock);
  const ends = await screen.findByRole("table", {
    name: "経路の時刻を決めたレコード",
  });
  const [, originItem] = within(ends).getAllByRole("row");
  expect(originItem?.textContent).toContain(
    "エッジの根拠と異なる未比較: 省いたエッジ 12 本の根拠",
  );
});

test("探索を上限で止めて影響のエッジが無いときは、経路が無いと言わず、有るかどうか分からないと出す", async () => {
  const mock = stubPathFetch(() =>
    jsonResponse(200, { ...emptyPathJson(), stops: ["computation_limit"] }),
  );

  await showPathFromProcessToTerminal(mock);

  const path = await screen.findByRole("region", { name: "影響の経路" });
  await waitFor(() => expect(within(path).getByText("未確定")).toBeTruthy());
  expect(within(path).queryByText("経路なし")).toBeNull();
});

test.each(["time_order_unsatisfied", "no_influence_route"])(
  "広げない計算だけが上限に達し、広げた計算が経路の無いこと (%s) を確かめたときは、経路が無いと出す",
  async (ruledOut) => {
    const mock = stubPathFetch(() =>
      jsonResponse(200, {
        ...emptyPathJson(),
        stops: ["computation_limit", ruledOut],
      }),
    );

    await showPathFromProcessToTerminal(mock);

    const path = await screen.findByRole("region", { name: "影響の経路" });
    await waitFor(() =>
      expect(within(path).getByText("経路なし")).toBeTruthy(),
    );
    expect(within(path).queryByText("未確定")).toBeNull();
  },
);

test("区間を広げない計算だけが上限に達したときは、精度の幅の要否が分からないことをエッジごとに添える", async () => {
  const mock = stubPathFetch(() =>
    jsonResponse(200, { ...pathJson(), stops: ["computation_limit"] }),
  );

  await showPathFromProcessToTerminal(mock);

  expect(await screen.findByText("精度の幅で広げた区間の集合")).toBeTruthy();
  expect(
    screen.getByRole("table", { name: "影響のエッジ" }).textContent,
  ).toContain("精度の幅の条件: 未確定");
});

test("起点がタイムスタンプを持たず数えていないときは、影響のエッジにしなかったレコードの件数を出さない", async () => {
  const mock = stubPathFetch(() => jsonResponse(200, untimedOriginJson()));

  await showPathFromProcessToTerminal(mock);

  const stops = await screen.findByRole("table", { name: "止まった理由" });
  expect(stops.textContent).toContain("時刻を持つレコードなし");
  expect(stops.textContent).toContain("起点");
  expect(screen.queryByText("時刻の無い根拠のレコード:")).toBeNull();
});

test("除外する根拠は既定で閉じ、見出しに今の結果で除外した根拠の数を出す", async () => {
  const mock = stubPathFetch(() => jsonResponse(200, pathJson()));

  await showPathFromProcessToTerminal(mock);
  const summary = await screen.findByText(/^除外する根拠/, {
    selector: "summary",
  });
  expect((summary.parentElement as HTMLDetailsElement).open).toBe(false);
  expect(summary.textContent).toBe(
    `除外する根拠 ${defaultExcludedBases.length}`,
  );
});

test("結果に使った除外する根拠を出し、選び直して再計算していない間はそのことを知らせる", async () => {
  const mock = stubPathFetch(() => jsonResponse(200, pathJson()));

  await showPathFromProcessToTerminal(mock);
  await waitFor(() =>
    expect(
      screen.getByText("除外した根拠:").closest("li")?.textContent,
    ).not.toBe("除外した根拠: なし"),
  );
  expect(screen.queryByText("未適用")).toBeNull();

  fireEvent.click(screen.getByLabelText("推定したエッジ"));

  expect(screen.getByText("未適用")).toBeTruthy();
});

test("表のノードの button で、図と同じく経路のノードを選べる", async () => {
  const mock = stubPathFetch(() => jsonResponse(200, pathJson()));

  await showPathFromProcessToTerminal(mock);
  const table = await screen.findByRole("table", { name: "影響のエッジ" });
  // 終点の端末の詳細から開いたため、端末の button が選んでいる状態である。
  expect(within(table).getAllByRole("button", { pressed: true })).toHaveLength(
    1,
  );
  const [origin] = within(table).getAllByRole("button", { pressed: false });
  if (origin === undefined) throw new Error("no node button");
  expect(
    within(table).getAllByText(
      (_, element) =>
        element?.tagName === "LI" && element.textContent === "影響のエッジ: 1",
    ),
  ).toHaveLength(2);
  fireEvent.click(origin);

  expect(await screen.findByText("選んでいる点: v:p")).toBeTruthy();
});

test("表のノードの button の名前に導き方を入れず、導き方は button の横の印の tooltip に出す", async () => {
  const json = pathJson();
  const derived = {
    normalized: "4688 host-a.bin:1024",
    derivation: "Event ID と収集元の file 名とレコードの位置",
    valueState: "derived",
  };
  const withDerived = {
    ...json,
    vertices: json.vertices.map((vertex) => ({
      ...vertex,
      node: { ...vertex.node, label: derived },
    })),
  };
  const mock = stubPathFetch(() => jsonResponse(200, withDerived));

  await showPathFromProcessToTerminal(mock);
  const table = await screen.findByRole("table", { name: "影響のエッジ" });
  const buttons = within(table).getAllByRole("button", {
    name: "4688 host-a.bin:1024",
  });
  expect(buttons).toHaveLength(2);
  // 導き方は、表示名の button の外の印が持つ。印は読み上げの文と、マウスを重ねたときの tooltip に
  // 同じ文を渡す。
  const marks = table.querySelectorAll(".derived-note");
  expect(marks).toHaveLength(2);
  for (const mark of marks) {
    expect(mark.closest("button")).toBeNull();
    expect(mark.getAttribute("title")).toContain(
      "Event ID と収集元の file 名とレコードの位置",
    );
  }
});

test("影響の経路を開いたまま隣接ノードだけを表示すると、影響の経路を閉じて隣接ノードの図を出す", async () => {
  const mock = stubPathFetch(() => jsonResponse(200, pathJson()));

  await showPathFromProcessToTerminal(mock);
  await screen.findByRole("table", { name: "影響のエッジ" });
  fireEvent.click(screen.getByRole("button", { name: "隣接ノードだけを表示" }));

  await waitFor(() =>
    expect(screen.queryByRole("table", { name: "影響のエッジ" })).toBeNull(),
  );
  expect(screen.queryByText("影響の経路を閉じる")).toBeNull();
});

test("先へ進まないノードの一部だけを返したときは、出している件数を添える", async () => {
  const mock = stubPathFetch(() =>
    jsonResponse(200, { ...pathJson(), frontierCount: 7 }),
  );

  await showPathFromProcessToTerminal(mock);

  expect(await findPair("件数: 7")).toBeTruthy();
  expect(getPair("表示: 1")).toBeTruthy();
});

test("経路が空のときは、止まった理由を出す", async () => {
  const mock = stubPathFetch(() => jsonResponse(200, emptyPathJson()));

  await showPathFromProcessToTerminal(mock);

  const stops = await screen.findByRole("table", { name: "止まった理由" });
  expect(stops.textContent).toContain("時刻の順を満たす経路なし");
  // 経路が無い理由は Path のビューに出し、図の枠には短いラベルだけを出す。
  const path = screen.getByRole("region", { name: "影響の経路" });
  expect(within(path).getByText("経路なし")).toBeTruthy();
  expect(document.querySelector(".pane-graph .figure")?.textContent).toBe(
    "経路なし",
  );
  expect(screen.queryByRole("table", { name: "影響のエッジ" })).toBeNull();
});

test("除外する根拠を選んで求め直すと、選択した根拠を要求に載せる", async () => {
  const mock = stubPathFetch(() => jsonResponse(200, pathJson()));

  await showPathFromProcessToTerminal(mock);
  await screen.findByRole("table", { name: "影響のエッジ" });

  fireEvent.click(screen.getByLabelText("推定したエッジ"));
  fireEvent.click(screen.getByLabelText("決められない向き"));
  // 選んだだけでは再計算しない。
  expect(pathRequests(mock)).toHaveLength(1);
  fireEvent.click(
    screen.getByRole("button", { name: "選択した根拠を除外して再計算" }),
  );

  await waitFor(() => expect(pathRequests(mock)).toHaveLength(2));
  expect(pathRequests(mock)[1]?.searchParams.getAll("excludeBasis")).toEqual([
    "undetermined_direction",
    "argument_name",
    "requested_destination",
    "candidate",
  ]);
});

test("取得に失敗したときは、失敗を出す", async () => {
  const mock = stubPathFetch(() =>
    jsonResponse(500, { code: "internal", message: "failed" }),
  );

  await showPathFromProcessToTerminal(mock);

  expect(await screen.findByText("影響の経路の取得")).toBeTruthy();
});

test("同じノードの 2 つのバージョンの点では、押した方の点を選んでいると描く", async () => {
  const twoVersions = pathJson();
  twoVersions.vertices.push({
    key: "v:t2",
    node: graphNode(terminalNodeId),
    influenceEdgeCount: 1,
  });
  twoVersions.edges.push({
    ...twoVersions.edges[0],
    id: "s:2",
    targetKey: "v:t2",
  });
  const mock = stubPathFetch(() => jsonResponse(200, twoVersions));

  await showPathFromProcessToTerminal(mock);
  fireEvent.click(
    await screen.findByRole("button", { name: "点 v:t2 を押す" }),
  );

  expect(await screen.findByText("選んでいる点: v:t2")).toBeTruthy();
});

test("影響の経路を閉じると、検索のグラフへ戻る", async () => {
  const mock = stubPathFetch(() => jsonResponse(200, pathJson()));

  await showPathFromProcessToTerminal(mock);
  fireEvent.click(
    await screen.findByRole("button", { name: "経路の表示を終了" }),
  );

  expect(screen.queryByRole("table", { name: "影響のエッジ" })).toBeNull();
  await findNodeList();
});

test("Path のビューを隠して描き直しても、影響の経路を取得し直さず、結果に使った除外する根拠を保つ", async () => {
  const mock = stubPathFetch(() => jsonResponse(200, pathJson()));
  // Path と Node Detail を同じ区画のタブとして切り替え、隠れたビューを描かない作業場所を模す。
  function TabbedPanes(panes: GraphExplorePanes) {
    const [front, setFront] = useState<"path" | "detail">("detail");
    return (
      <>
        {panes.search}
        {panes.graph}
        {panes.nodes}
        <button type="button" onClick={() => setFront("path")}>
          Path のタブ
        </button>
        <button type="button" onClick={() => setFront("detail")}>
          Node Detail のタブ
        </button>
        {front === "path" ? panes.path : panes.detail}
      </>
    );
  }
  render(
    <GraphExploreHarness
      onSelectRecord={() => {}}
      selectedEdgeId={undefined}
      onSelectEdge={() => {}}
      assertions={() => null}
      layout={(panes) => <TabbedPanes {...panes} />}
    />,
  );
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
  fireEvent.click(screen.getByRole("button", { name: "Path のタブ" }));
  await screen.findByRole("table", { name: "影響のエッジ" });
  fireEvent.click(screen.getByLabelText("推定したエッジ"));
  fireEvent.click(
    screen.getByRole("button", { name: "選択した根拠を除外して再計算" }),
  );
  await waitFor(() => expect(pathRequests(mock)).toHaveLength(2));

  fireEvent.click(screen.getByRole("button", { name: "Node Detail のタブ" }));
  expect(screen.queryByRole("region", { name: "影響の経路" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Path のタブ" }));

  expect(
    await screen.findByRole("table", { name: "影響のエッジ" }),
  ).toBeTruthy();
  expect(
    (screen.getByLabelText("推定したエッジ") as HTMLInputElement).checked,
  ).toBe(true);
  expect(screen.queryByText("未適用")).toBeNull();
  expect(pathRequests(mock)).toHaveLength(2);
});
