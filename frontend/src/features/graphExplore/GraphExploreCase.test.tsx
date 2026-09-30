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
import { useState } from "react";
import { afterEach, expect, test, vi } from "vitest";
import type { RecordFilterCriteria } from "@/shared/api/graph";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import type { CaseId } from "@/shared/contracts/cases";
import { noSearchTerms, type SearchTerms } from "@/shared/lib/searchTerms";
import {
  baselineCaseId,
  casedGraphResponseJson,
  casedNodeDetailResponseJson,
  challengeCaseId,
} from "@/testdata/cases/caseCounts";
import {
  chooseConditionValue,
  conditionValueOptions,
} from "@/testdata/conditionInput";
import { eventKindsResponseJson } from "@/testdata/eventKinds/eventKindsResponse";
import {
  graphResponseJson,
  nodeDetailResponseJson,
} from "@/testdata/graph/graphResponse";
import { jsonResponse } from "@/testdata/http";
import {
  findEdgeList,
  findNodeList,
  GraphExploreWithTerminals,
  figureRequests as graphPaths,
  stackPanes,
} from "./graphExploreTestHarness";

// WebGL の renderer は jsdom で動かない。本 test は図を stub に置き換え、フィルタの欄と
// 一覧と詳細を確かめる。
vi.mock("./SubgraphCanvas", () => ({
  SubgraphCanvas: () => <p>部分グラフの図</p>,
}));

const matchConditions: MatchConditionSelection = {
  conditions: [{ conditionKey: "destination_ip" }],
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

/** 根拠のレコードのフィルタの条件を上位の画面と同じく 1 か所で持ち、適用した値を画面へ戻す。 */
function Harness({
  caseIds,
  onRecordFilter,
}: {
  caseIds: readonly CaseId[];
  onRecordFilter?: (filter: RecordFilterCriteria) => void;
}) {
  const [recordFilter, setRecordFilter] = useState<RecordFilterCriteria>({});
  const [searchTerms, setSearchTerms] = useState<SearchTerms>(noSearchTerms);
  return (
    <GraphExploreWithTerminals
      matchConditions={matchConditions}
      onApplyMatchConditions={() => {}}
      recordFilter={recordFilter}
      onApplyRecordFilter={(filter) => {
        onRecordFilter?.(filter);
        setRecordFilter(filter);
      }}
      searchTerms={searchTerms}
      onChangeSearchTerms={setSearchTerms}
      layout={stackPanes}
      caseIds={caseIds}
      onSelectRecord={() => {}}
      selectedEdgeId={undefined}
      onSelectEdge={() => {}}
      assertions={() => null}
      dataVersion={0}
    />
  );
}

function stubFetch(graphJson: unknown, nodeJson: unknown) {
  const mock = vi.fn(async (input: string, _init?: RequestInit) => {
    if (input.startsWith("/api/v0/event-kinds")) {
      return jsonResponse(200, eventKindsResponseJson());
    }
    return input.startsWith("/api/v0/nodes/")
      ? jsonResponse(200, nodeJson)
      : jsonResponse(200, graphJson);
  });
  vi.stubGlobal("fetch", mock);
  return mock;
}

test("案件を持つ収集元があるとき、案件の候補に各案件を並べる", async () => {
  stubFetch(graphResponseJson(), nodeDetailResponseJson());

  render(<Harness caseIds={[baselineCaseId, challengeCaseId]} />);

  await findNodeList();
  expect(conditionValueOptions("案件")).toEqual([
    baselineCaseId,
    challengeCaseId,
  ]);
});

test("案件を持つ収集元が無いとき、案件を条件の種類に出さない", async () => {
  stubFetch(graphResponseJson(), nodeDetailResponseJson());

  render(<Harness caseIds={[]} />);

  await findNodeList();
  const kinds = screen.getByRole("combobox", { name: "条件の種類" });
  act(() => kinds.focus());
  const names = within(screen.getByRole("listbox"))
    .getAllByRole("option")
    .map((option) => option.firstElementChild?.textContent);
  expect(names).toContain("Host");
  expect(names).not.toContain("案件");
});

test("案件を選ぶと根拠のレコードのフィルタの条件に入れて取り直し、chip を外すと外す", async () => {
  const mock = stubFetch(
    { ...graphResponseJson(), case: challengeCaseId },
    nodeDetailResponseJson(),
  );
  const applied = vi.fn();

  render(
    <Harness
      caseIds={[baselineCaseId, challengeCaseId]}
      onRecordFilter={applied}
    />,
  );
  await findNodeList();
  expect(graphPaths(mock).at(-1)).not.toContain("case=");

  chooseConditionValue("案件", challengeCaseId);

  expect(applied).toHaveBeenLastCalledWith({ caseId: challengeCaseId });
  await waitFor(() =>
    expect(graphPaths(mock).at(-1)).toContain(`case=${challengeCaseId}`),
  );
  expect(
    (
      await screen.findAllByText(
        (_, element) =>
          element?.tagName === "LI" &&
          element.textContent === `案件: ${challengeCaseId}`,
      )
    ).length,
  ).toBeGreaterThan(0);

  fireEvent.click(
    screen.getByRole("button", { name: `案件 ${challengeCaseId} を削除` }),
  );

  expect(applied).toHaveBeenLastCalledWith({ caseId: undefined });
  await waitFor(() => expect(graphPaths(mock).at(-1)).not.toContain("case="));
});

test("エッジの一覧は、案件ごとの件数を持つエッジで根拠の総数の横に案件ごとの件数を並べる", async () => {
  stubFetch(casedGraphResponseJson(), nodeDetailResponseJson());

  render(<Harness caseIds={[baselineCaseId, challengeCaseId]} />);

  const table = await findEdgeList();
  const cells = within(table)
    .getAllByRole("cell")
    .map((cell) => cell.textContent);
  expect(cells).toContain(
    `合計: 4、${baselineCaseId}: 3、${challengeCaseId}: 1`,
  );
  expect(cells).toContain(`合計: 1、${challengeCaseId}: 1`);
});

test("エッジの一覧は、案件ごとの件数を持たないエッジで根拠の総数だけを出す", async () => {
  stubFetch(graphResponseJson(), nodeDetailResponseJson());

  render(<Harness caseIds={[]} />);

  const table = await findEdgeList();
  const cells = within(table)
    .getAllByRole("cell")
    .map((cell) => cell.textContent);
  expect(cells).toContain("4");
  expect(cells.some((text) => /^合計: /.test(text ?? ""))).toBe(false);
});

test("ノードの詳細は、根拠の件数とエッジの件数に案件ごとの件数を並べる", async () => {
  stubFetch(casedGraphResponseJson(), casedNodeDetailResponseJson());

  render(<Harness caseIds={[baselineCaseId, challengeCaseId]} />);

  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );

  await screen.findByRole("table", { name: "ノードを記録したレコード" });
  expect(
    screen.getByText(
      `件数: 2 · 案件ごと: ${baselineCaseId}: 1、${challengeCaseId}: 1`,
    ),
  ).toBeTruthy();
  const edgeCounts = screen.getByRole("table", {
    name: "エッジの件数",
  });
  const cells = within(edgeCounts)
    .getAllByRole("cell")
    .map((cell) => cell.textContent);
  expect(cells).toContain(
    `合計: 4、${baselineCaseId}: 3、${challengeCaseId}: 1`,
  );
  expect(cells).toContain(`合計: 1、${challengeCaseId}: 1`);
});

test("ノードの詳細は、案件ごとの件数を持たない応答で案件を出さない", async () => {
  stubFetch(graphResponseJson(), nodeDetailResponseJson());

  render(<Harness caseIds={[]} />);

  await findNodeList();
  fireEvent.click(
    await screen.findByRole("button", {
      name: "プロセス C:\\Windows\\System32\\cmd.exe の詳細を開く",
    }),
  );

  await screen.findByRole("table", { name: "ノードを記録したレコード" });
  expect(screen.getByText("件数: 2")).toBeTruthy();
  const edgeCounts = screen.getByRole("table", {
    name: "エッジの件数",
  });
  expect(edgeCounts.textContent).not.toContain("件）");
});
