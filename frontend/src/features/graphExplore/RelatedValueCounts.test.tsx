// @vitest-environment jsdom
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { everyMatchCondition } from "@/shared/api/matchConditions";
import { decodeNodeDetailResponse } from "@/shared/contracts/graphDetail";
import { decodeNodeValueCountsResponse } from "@/shared/contracts/nodeValueCounts";
import { nodeDetailResponseJson } from "@/testdata/graph/graphResponse";
import { jsonResponse } from "@/testdata/http";
import { RelatedValueCounts } from "./RelatedValueCounts";

const matchConditions = everyMatchCondition();

function detail() {
  return decodeNodeDetailResponse(nodeDetailResponseJson(), "fixture");
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

test("数える欄が空のまま form を送っても、要求を出さず、失敗も出さない", async () => {
  const fetch = stubFetch({});
  render(
    <RelatedValueCounts detail={detail()} matchConditions={matchConditions} />,
  );
  const input = screen.getByLabelText("集計するフィールド");
  fireEvent.click(screen.getByRole("button", { name: "隣接ノードを集計" }));
  const form = input.closest("form");
  if (form === null) throw new Error("集計の form が無い");
  fireEvent.submit(form);
  // 失敗は取得の結果を待ってから出るので、結果が出る時間を待ってから確かめる。
  await act(() => new Promise((resolve) => setTimeout(resolve, 50)));
  expect(fetch).not.toHaveBeenCalled();
  expect(screen.queryByRole("alert")).toBeNull();
});

test("関係と欄を選んで数えると、相手側の欄の値ごとの件数を表に出す", async () => {
  const loaded = detail();
  const fetch = stubFetch({
    nodeId: loaded.node.id,
    edgeKind: "ran_on",
    direction: "outgoing",
    countBy: "terminal.hostname",
    valueCounts: [{ value: "host01.example.test", recordCount: 4 }],
  });
  render(
    <RelatedValueCounts detail={loaded} matchConditions={matchConditions} />,
  );
  fireEvent.change(screen.getByLabelText("集計するフィールド"), {
    target: { value: "terminal.hostname" },
  });
  fireEvent.click(screen.getByRole("button", { name: "隣接ノードを集計" }));
  const table = await screen.findByRole("table", {
    name: "隣接ノードのフィールドの値ごとのレコード数",
  });
  expect(within(table).getByText("host01.example.test")).toBeTruthy();
  const url = new URL(String(fetch.mock.calls[0]?.[0]), "http://localhost");
  expect(url.pathname).toBe(
    `/api/v0/nodes/${encodeURIComponent(loaded.node.id)}/value-counts`,
  );
  expect(url.searchParams.get("edgeKind")).toBe("ran_on");
  expect(url.searchParams.get("direction")).toBe("outgoing");
  expect(url.searchParams.get("countBy")).toBe("terminal.hostname");
});

test("欄をどこにも観測していないときは、理由を出す", async () => {
  const loaded = detail();
  stubFetch({
    nodeId: loaded.node.id,
    edgeKind: "ran_on",
    direction: "outgoing",
    countBy: "absent",
    valueCounts: [],
    emptyReason: "no_field_observed",
  });
  render(
    <RelatedValueCounts detail={loaded} matchConditions={matchConditions} />,
  );
  fireEvent.change(screen.getByLabelText("集計するフィールド"), {
    target: { value: "absent" },
  });
  fireEvent.click(screen.getByRole("button", { name: "隣接ノードを集計" }));
  expect((await screen.findByRole("status")).textContent).toBe(
    "読み取れたレコードにフィールド名なし",
  );
  expect(screen.queryByRole("table")).toBeNull();
});

test("ノードが替わると、前のノードの結果を出さない", async () => {
  const loaded = detail();
  stubFetch({
    nodeId: loaded.node.id,
    edgeKind: "ran_on",
    direction: "outgoing",
    countBy: "terminal.hostname",
    valueCounts: [{ value: "host01.example.test", recordCount: 4 }],
  });
  const { rerender } = render(
    <RelatedValueCounts
      key="a"
      detail={loaded}
      matchConditions={matchConditions}
    />,
  );
  fireEvent.change(screen.getByLabelText("集計するフィールド"), {
    target: { value: "terminal.hostname" },
  });
  fireEvent.click(screen.getByRole("button", { name: "隣接ノードを集計" }));
  await screen.findByRole("table");
  rerender(
    <RelatedValueCounts
      key="b"
      detail={loaded}
      matchConditions={matchConditions}
    />,
  );
  expect(screen.queryByRole("table")).toBeNull();
});

test("取得に失敗したら、読み込み中のまま止まらずに失敗を出す", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => {
      throw new Error("network down");
    }),
  );
  render(
    <RelatedValueCounts detail={detail()} matchConditions={matchConditions} />,
  );
  fireEvent.change(screen.getByLabelText("集計するフィールド"), {
    target: { value: "terminal.hostname" },
  });
  fireEvent.click(screen.getByRole("button", { name: "隣接ノードを集計" }));
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(screen.queryByText("隣接ノードの集計中")).toBeNull();
});

test("理由と件数が食い違う応答を読まない", () => {
  expect(() =>
    decodeNodeValueCountsResponse(
      {
        nodeId: "n",
        edgeKind: "ran_on",
        direction: "outgoing",
        countBy: "x",
        valueCounts: [{ value: "v", recordCount: 1 }],
        emptyReason: "no_field_observed",
      },
      "$",
    ),
  ).toThrow();
});
