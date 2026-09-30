// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import { decodeInvestigationOrderResponse } from "@/shared/contracts/investigationOrder";
import {
  investigationOrderResponseJson,
  sigmaOrderResponseJson,
} from "@/testdata/investigationOrder/orderResponse";
import { getPair } from "./graphExploreTestHarness";
import { InvestigationOrderList } from "./InvestigationOrderPane";

afterEach(cleanup);

function loadedState(processCount?: number) {
  return loadedStateOf(investigationOrderResponseJson(processCount));
}

function loadedStateOf(json: unknown) {
  return {
    status: "loaded" as const,
    value: decodeInvestigationOrderResponse(json, "response"),
  };
}

/** プロセスの表。見出しは種類の名前と行の数である。 */
function processTable(count: number) {
  return screen.getByRole("table", { name: `プロセス ${count}` });
}

/** プロセスの表の各行の、順位・表示名・値・同じ順位の数。 */
function processRows(count: number) {
  return within(processTable(count))
    .getAllByRole("row")
    .slice(1)
    .map((row) =>
      within(row)
        .getAllByRole("cell")
        .slice(0, 4)
        .map((cell) => cell.textContent),
    );
}

test("種別ごとの表に順位と値と同じ順位の数を出し、計算に使った入力を添える", () => {
  render(
    <InvestigationOrderList
      state={loadedState()}
      method="degree"
      onChangeMethod={() => {}}
      sigmaMinLevel={undefined}
      onChangeSigmaMinLevel={() => {}}
      onShowNeighbours={() => {}}
    />,
  );
  expect(
    getPair("順位の基準: エッジか同じレコードで結ばれたノードの数"),
  ).toBeDefined();
  expect(getPair("順: 多い順")).toBeDefined();
  expect(getPair("ノード: 4")).toBeDefined();
  expect(getPair("ノードの組: 3")).toBeDefined();
  expect(getPair("レコード: 12")).toBeDefined();
  expect(processRows(3)).toEqual([
    ["1", "C:\\synthetic\\tool1.exe", "5", "2"],
    ["1", "C:\\synthetic\\tool2.exe", "5", "2"],
    ["3", "C:\\synthetic\\tool3.exe", "値なし", "1"],
  ]);
});

test("Sigma の値を、最高レベルと一致したレコードの件数に分けて出す", () => {
  render(
    <InvestigationOrderList
      state={loadedStateOf(sigmaOrderResponseJson())}
      method="sigma"
      onChangeMethod={() => {}}
      sigmaMinLevel={undefined}
      onChangeSigmaMinLevel={() => {}}
      onShowNeighbours={() => {}}
    />,
  );
  expect(processRows(3)).toEqual([
    ["1", "C:\\synthetic\\tool1.exe", "high · 4", "1"],
    ["2", "C:\\synthetic\\tool2.exe", "low · 9", "1"],
    ["3", "C:\\synthetic\\tool3.exe", "一致なし", "1"],
  ]);
});

test("Sigma では数えるルールのレベルの下限を選ばせ、すべてのレベルは未指定として渡す", () => {
  const onChangeSigmaMinLevel = vi.fn();
  render(
    <InvestigationOrderList
      state={loadedStateOf(sigmaOrderResponseJson())}
      method="sigma"
      onChangeMethod={() => {}}
      sigmaMinLevel="medium"
      onChangeSigmaMinLevel={onChangeSigmaMinLevel}
      onShowNeighbours={() => {}}
    />,
  );
  const select = screen.getByRole("combobox", {
    name: "数える Sigma ルールのレベル",
  });
  expect((select as HTMLSelectElement).value).toBe("medium");
  expect(
    within(select)
      .getAllByRole("option")
      .map((option) => option.textContent),
  ).toEqual([
    "すべてのレベル",
    "informational 以上",
    "low 以上",
    "medium 以上",
    "high 以上",
    "critical 以上",
  ]);
  fireEvent.change(select, { target: { value: "high" } });
  fireEvent.change(select, { target: { value: "all" } });
  expect(onChangeSigmaMinLevel.mock.calls).toEqual([["high"], [undefined]]);
});

test("Sigma のルールの集合を渡していない起動では、表の代わりに状態を出す", () => {
  render(
    <InvestigationOrderList
      state={loadedStateOf(sigmaOrderResponseJson({ ruleSet: false }))}
      method="sigma"
      onChangeMethod={() => {}}
      sigmaMinLevel={undefined}
      onChangeSigmaMinLevel={() => {}}
      onShowNeighbours={() => {}}
    />,
  );
  expect(screen.getByText("Sigma ルールなし")).toBeDefined();
  expect(screen.queryByRole("table")).toBeNull();
});

test("Sigma のルールを適用したレコードが無いときは、表の代わりに状態を出す", () => {
  render(
    <InvestigationOrderList
      state={loadedStateOf(sigmaOrderResponseJson({ evaluatedRecordCount: 0 }))}
      method="sigma"
      onChangeMethod={() => {}}
      sigmaMinLevel={undefined}
      onChangeSigmaMinLevel={() => {}}
      onShowNeighbours={() => {}}
    />,
  );
  expect(screen.getByText("Sigma ルールを適用したレコードなし")).toBeDefined();
  expect(screen.queryByRole("table")).toBeNull();
});

test("種別ごとに上位 20 行だけを描き、残りの件数を出す", () => {
  render(
    <InvestigationOrderList
      state={loadedState(25)}
      method="degree"
      onChangeMethod={() => {}}
      sigmaMinLevel={undefined}
      onChangeSigmaMinLevel={() => {}}
      onShowNeighbours={() => {}}
    />,
  );
  const processes = processTable(25);
  expect(within(processes).getAllByRole("row")).toHaveLength(1 + 20 + 1);
  expect(getPair("表示していない行: 5", processes)).toBeDefined();
});

test("行の操作で対象を起点として渡し、手法の選択を上位へ渡す", () => {
  const onShowNeighbours = vi.fn();
  const onChangeMethod = vi.fn();
  render(
    <InvestigationOrderList
      state={loadedState()}
      method="degree"
      onChangeMethod={onChangeMethod}
      sigmaMinLevel={undefined}
      onChangeSigmaMinLevel={() => {}}
      onShowNeighbours={onShowNeighbours}
    />,
  );
  expect(
    screen.queryByRole("combobox", { name: "数える Sigma ルールのレベル" }),
  ).toBeNull();
  fireEvent.click(
    screen.getByRole("button", { name: "端末 HOST-E の隣接ノードだけを表示" }),
  );
  expect(onShowNeighbours).toHaveBeenCalledWith({
    id: "n:terminal:0e0e",
    label: "HOST-E",
    kind: "terminal",
  });
  fireEvent.change(screen.getByRole("combobox", { name: "並べ方" }), {
    target: { value: "sigma" },
  });
  expect(onChangeMethod).toHaveBeenCalledWith("sigma");
  expect(
    within(screen.getByRole("combobox", { name: "並べ方" }))
      .getAllByRole("option")
      .map((option) => option.textContent),
  ).toEqual(["隣接ノードの数", "Sigma ルールの一致"]);
});

test("取得に失敗したときは失敗の理由を出す", () => {
  render(
    <InvestigationOrderList
      state={{
        status: "failed",
        failure: buildFetchFailure(
          "unexpected",
          "調べる順序の目安を取得できませんでした。",
        ),
      }}
      method="degree"
      onChangeMethod={() => {}}
      sigmaMinLevel={undefined}
      onChangeSigmaMinLevel={() => {}}
      onShowNeighbours={() => {}}
    />,
  );
  expect(
    screen.getByText("調べる順序の目安を取得できませんでした。"),
  ).toBeDefined();
});
