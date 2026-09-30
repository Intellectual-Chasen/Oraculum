import { expect, test } from "vitest";
import {
  viewInputOf,
  withOriginKinds,
} from "@/features/graphExplore/viewInput";
import { noSearchTerms } from "@/shared/lib/searchTerms";
import { autoView, resolveView } from "@/shared/lib/searchView";
import { baselineCaseId } from "@/testdata/cases/caseCounts";
import { type SearchState, searchQueryOf, searchStateOf } from "./searchState";

const terminal = { id: "n:terminal:1", label: "HOST-A" };
const source = { id: "src:1", label: "host-a.log" };
const sourceLabels = new Map([[source.id, source.label]]);
const origin = {
  id: "n:process:1",
  label: "cmd.exe",
  kind: "process" as const,
};

const fullState: SearchState = {
  searchTerms: {
    contains: ["cmd.exe", "whoami"],
    excludes: ["svchost"],
    field: "process.command_line",
    fieldContains: ["TargetUserName=alice"],
    fieldEquals: ["LogonType=3"],
    expression: "process.name contains cmd",
  },
  recordFilter: {
    timeFilter: {
      from: { text: "2026-01-02T03:04:05Z", precision: "second" },
      to: { text: "2026-01-02T04:04:05.123+09:00", precision: "millisecond" },
      unit: "second",
    },
    eventCategory: "ps",
    eventAction: "start",
    eventActionFrom: 4624,
    eventActionTo: 4625,
    caseId: baselineCaseId,
    terminal,
    sources: [source],
  },
  graphConditions: {
    depth: 2,
    origins: [origin],
    edgeKinds: ["process_parent_child"],
    addressInCidr: "198.51.100.0/24",
    addressNotInCidr: "203.0.113.0/24",
    countBy: "process.name",
    conditionsOnOriginsOnly: true,
    endpointRecordsInPeriod: true,
  },
  view: { kind: "manual", nodeKinds: ["process", "ip"] },
};

/** 画面の図が出す対象を、グラフの探索と同じ手順で決める。 */
function shownOf(state: SearchState) {
  return resolveView(
    state.view,
    viewInputOf(state.searchTerms, state.recordFilter, state.graphConditions),
  );
}

test("検索の条件のすべてを SearchQuery へ写し、同じ条件へ戻す", () => {
  const query = searchQueryOf(fullState, shownOf(fullState));
  expect(query).toEqual({
    nodeKinds: ["process", "ip"],
    granularity: "object",
    depth: 2,
    nodeIds: ["n:process:1"],
    edgeKinds: ["process_parent_child"],
    valueContains: ["cmd.exe", "whoami"],
    valueExcludes: ["svchost"],
    valueField: "process.command_line",
    fieldContains: ["TargetUserName=alice"],
    fieldEquals: ["LogonType=3"],
    searchExpression: "process.name contains cmd",
    conditionsOnOriginsOnly: true,
    endpointRecordsInPeriod: true,
    countBy: "process.name",
    addressInCidr: "198.51.100.0/24",
    addressNotInCidr: "203.0.113.0/24",
    eventCategory: "ps",
    eventAction: "start",
    eventActionFrom: 4624,
    eventActionTo: 4625,
    timeFrom: "2026-01-02T03:04:05Z",
    timeFromPrecision: "second",
    timeTo: "2026-01-02T04:04:05.123+09:00",
    timeToPrecision: "millisecond",
    filterUnit: "second",
    case: baselineCaseId,
    terminal: "n:terminal:1",
    sources: ["src:1"],
  });
  expect(searchStateOf(query, [origin], [terminal], sourceLabels)).toEqual(
    fullState,
  );
});

test("起点を持たない card は、前に適用した起点を外す値を持つ", () => {
  const state = searchStateOf({ depth: 1 }, undefined, [], new Map());
  // 適用は前の図の条件に重ねるため、起点の欄を undefined で持つ。
  expect(Object.hasOwn(state.graphConditions, "origins")).toBe(true);
  expect(state.graphConditions.origins).toBeUndefined();
});

test("図に出す対象は起点の種別を含み、レコードの起点があればレコードの粒度で出す", () => {
  const withoutCondition = resolveView(autoView, {
    hasCondition: false,
  });
  expect(withOriginKinds(withoutCondition, [origin])).toEqual({
    granularity: "object",
    nodeKinds: ["terminal", "ip", "process"],
  });
  const record = {
    id: "n:record:1",
    label: "record 1",
    kind: "record" as const,
  };
  expect(withOriginKinds({ granularity: "object" }, [origin, record])).toEqual({
    granularity: "record",
    nodeKinds: undefined,
  });
  expect(
    viewInputOf(noSearchTerms, {}, { origins: [origin] }).hasCondition,
  ).toBe(true);
});

test("条件の無い自動の選び方は、画面が図に出す端末と IP アドレスの種別とホップ数を持つ SearchQuery になる", () => {
  const empty: SearchState = {
    searchTerms: noSearchTerms,
    recordFilter: {},
    graphConditions: { depth: 1 },
    view: autoView,
  };
  const query = searchQueryOf(empty, shownOf(empty));
  expect(JSON.parse(JSON.stringify(query))).toEqual({
    granularity: "object",
    nodeKinds: ["terminal", "ip"],
    depth: 1,
  });
});

test("自動の選び方は、条件に合わせて画面が選んだ種別を SearchQuery に持つ", () => {
  const state: SearchState = {
    ...fullState,
    recordFilter: { eventCategory: "net" },
    view: autoView,
  };
  expect(searchQueryOf(state, shownOf(state)).nodeKinds).toEqual([
    "process",
    "ip",
  ]);
});

test("最後の検索語を消した後の発言には検索対象の欄を添えない", () => {
  const state: SearchState = {
    ...fullState,
    searchTerms: {
      contains: [],
      excludes: [],
      field: "process.command_line",
      fieldContains: ["TargetUserName=alice"],
    },
  };
  const query = JSON.parse(
    JSON.stringify(searchQueryOf(state, shownOf(state))),
  );
  expect(query).not.toHaveProperty("valueField");
  expect(query.fieldContains).toEqual(["TargetUserName=alice"]);
});

test("種別と粒度を持たない SearchQuery は、図に出す対象を検索の条件から決める", () => {
  expect(searchStateOf({ depth: 1 }, undefined, [], new Map())).toEqual({
    searchTerms: noSearchTerms,
    recordFilter: {},
    graphConditions: { depth: 1 },
    view: autoView,
  });
});

test("手で選んだ図に出す対象を、種別と粒度の組へ写して戻す", () => {
  for (const view of [
    { kind: "manual" as const, nodeKinds: [] },
    {
      kind: "manual" as const,
      nodeKinds: ["record" as const, "process" as const],
    },
  ]) {
    const state = { ...fullState, view };
    const query = searchQueryOf(state, shownOf(state));
    expect(query.granularity).toBe(
      view.nodeKinds.length > 0 ? "record" : "object",
    );
    expect(
      searchStateOf(query, [origin], [terminal], sourceLabels).view,
    ).toEqual(view);
  }
});

test("端末の選択肢に無い端末と、表示名の無い収集元は、識別子を表示名にする", () => {
  const state = searchStateOf(
    { depth: 1, terminal: "n:terminal:9", sources: ["src:9"] },
    undefined,
    [terminal],
    sourceLabels,
  );
  expect(state.recordFilter.terminal).toEqual({
    id: "n:terminal:9",
    label: "n:terminal:9",
  });
  expect(state.recordFilter.sources).toEqual([{ id: "src:9", label: "src:9" }]);
});
