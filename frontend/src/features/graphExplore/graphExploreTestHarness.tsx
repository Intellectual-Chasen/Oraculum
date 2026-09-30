import { fireEvent, render, screen, within } from "@testing-library/react";
import { useState } from "react";
import { vi } from "vitest";
import type { RecordFilterCriteria } from "@/shared/api/graph";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import { noSearchTerms, type SearchTerms } from "@/shared/lib/searchTerms";
import { candidateResponse } from "@/testdata/attackCandidates/candidateResponse";
import { eventKindsResponseJson } from "@/testdata/eventKinds/eventKindsResponse";
import {
  graphResponseJson,
  limitedGraphResponseJson,
  nodeDetailResponseJson,
} from "@/testdata/graph/graphResponse";
import { jsonResponse } from "@/testdata/http";
import { investigationOrderResponseJson } from "@/testdata/investigationOrder/orderResponse";
import { sigmaCandidatesResponse } from "@/testdata/sigmaRuleCandidates/candidateResponse";
import { ValueActionsForTest } from "@/testdata/valueActions";
import {
  GraphExplore,
  type GraphExplorePanes,
  type GraphExploreProps,
} from "./GraphExplore";
import { useTerminalList } from "./useTerminalList";

/**
 * 本 harness が持つ関連付けの条件の選択。
 * 要求の URL が含む文字列を、他の絞り込みの期待値から分けて指定できるようにする。
 */
const initialMatchConditions: MatchConditionSelection = {
  conditions: [{ conditionKey: "destination_ip" }],
};

/** 上の選択を要求へ載せた文字列。 */
export const matchConditionQuery = "&matchCondition=destination_ip";

/**
 * 描画の上限を置かない既定の要求を、figureRequests が読む形の項目。既定の要求は nodeLimit を
 * 載せない。figureRequests は、nodeLimit を持たない図の要求の先頭にこの項目を置いて返す。
 */
export const limitQuery = "nodeLimit=all";

/** 端末と IP アドレスを図に出す要求の項目。 */
export const terminalIpViewQuery =
  "&nodeKind=terminal&nodeKind=ip&granularity=object";

/** レコードを除くすべての対象を図に出す要求の項目。 */
export const objectViewQuery = "&granularity=object";

/** 検索の条件を持たない図の要求。 */
export const initialFigureRequest = `/api/v0/graph?${limitQuery}${terminalIpViewQuery}&depth=1${matchConditionQuery}`;

/** 端末で絞る条件の選択肢を取る要求の接頭辞。関連付けの条件の選択が後ろに続く。 */
export const terminalListPrefix =
  "/api/v0/graph?nodeKind=terminal&granularity=object&depth=0&";

/** 端末の選択肢に出す 2 つ目の端末。 */
export const otherTerminalNodeId = "n:terminal:2e5f";

const noFileNames: ReadonlyMap<string, string> = new Map();

/**
 * 端末の選択肢は上位の画面が取得する。test はその取得を 1 つ置き、残りの props を呼び出し側
 * から受け取る。
 */
export function GraphExploreWithTerminals(
  props: Omit<GraphExploreProps, "terminals">,
) {
  const terminals = useTerminalList(
    props.matchConditions,
    props.dataVersion,
    noFileNames,
  );
  return <GraphExplore {...props} terminals={terminals} />;
}

/**
 * 根拠のレコードを絞る条件と関連付けの条件の選択と検索の文字列は上位の画面が持つ。test は
 * その所有者を 1 つ置き、適用する操作がその値を動かすことを確かめる。
 */
export function GraphExploreHarness(
  props: Omit<
    GraphExploreProps,
    | "terminals"
    | "recordFilter"
    | "onApplyRecordFilter"
    | "matchConditions"
    | "onApplyMatchConditions"
    | "searchTerms"
    | "onChangeSearchTerms"
    | "caseIds"
    | "dataVersion"
    | "layout"
  > & { layout?: GraphExploreProps["layout"] },
) {
  const [recordFilter, setRecordFilter] = useState<RecordFilterCriteria>({});
  const [matchConditions, setMatchConditions] =
    useState<MatchConditionSelection>(initialMatchConditions);
  const [searchTerms, setSearchTerms] = useState<SearchTerms>(noSearchTerms);
  return (
    <GraphExploreWithTerminals
      {...props}
      dataVersion={0}
      caseIds={[]}
      recordFilter={recordFilter}
      onApplyRecordFilter={setRecordFilter}
      matchConditions={matchConditions}
      onApplyMatchConditions={setMatchConditions}
      searchTerms={searchTerms}
      onChangeSearchTerms={setSearchTerms}
      layout={props.layout ?? stackPanes}
    />
  );
}

/**
 * test でビューを順に並べる layout。Priority は描くと目安の取得を始めるため、作業場所で
 * ビューを表示したときと同じく、`withPriority` で描く test だけが並べる。
 */
export function stackPanes(panes: GraphExplorePanes) {
  return (
    <>
      {panes.search}
      {panes.graph}
      {panes.detail}
      {panes.nodes}
      {panes.edges}
      {panes.detection}
      {panes.path}
    </>
  );
}

/** stackPanes に Priority を足した layout。 */
export function withPriority(panes: GraphExplorePanes) {
  return (
    <>
      {stackPanes(panes)}
      {panes.priority}
    </>
  );
}

/**
 * harness を描く。根拠のレコードを選んだときの受け手は onSelectRecord で差し替える。表の値の
 * 操作の provider の中に描き、表の位置の値から開くレコードも onSelectRecord へ渡す。
 */
export function renderGraphExplore(
  onSelectRecord: GraphExploreProps["onSelectRecord"] = () => {},
  layout?: GraphExploreProps["layout"],
) {
  return render(
    <ValueActionsForTest onSelectRecord={onSelectRecord}>
      <GraphExploreHarness
        layout={layout}
        onSelectRecord={onSelectRecord}
        selectedEdgeId={undefined}
        onSelectEdge={() => {}}
        assertions={() => null}
      />
    </ValueActionsForTest>,
  );
}

/** 端末の選択肢の要求に返す応答。端末のノード 2 件だけを含む。 */
export function terminalListResponseJson() {
  const graph = graphResponseJson();
  const [terminal] = graph.nodes;
  return {
    ...graph,
    nodes: [
      terminal,
      {
        ...terminal,
        id: otherTerminalNodeId,
        identity: [{ semantic: "terminal.id", value: "HOST-D-TMID" }],
        label: { rawText: "HOST-D", valueState: "present" },
      },
    ],
    nodeCount: 2,
    matchedKinds: [{ kind: "terminal", count: 2 }],
    edges: [],
    edgeCount: 0,
  };
}

function respond(json: unknown): Response {
  return json instanceof Response ? json : jsonResponse(200, json);
}

/**
 * 図の要求に graphJson、ノードの詳細に nodeJson、端末の選択肢の要求に terminalJson を返す。
 * ATT&CK の候補の要求には候補の無い応答を、Sigma の候補の要求には候補を、事象の種別の
 * 一覧の要求には eventKindsJson を返す。
 * JSON を与えた項目は、呼び出しごとに新しい Response を作る。body は 1 度しか読めない。
 *
 * 図の既定の応答は、画面が最初に送る上限を置いた要求への答えである。
 */
export function stubFetch(
  graphJson: unknown = limitedGraphResponseJson(),
  nodeJson: unknown = nodeDetailResponseJson(),
  terminalJson: unknown = terminalListResponseJson(),
  eventKindsJson: unknown = eventKindsResponseJson(),
) {
  const mock = vi.fn(async (input: string, _init?: RequestInit) => {
    if (input.startsWith("/api/v0/attack-candidates")) {
      return respond(candidateResponse([]));
    }
    if (input.startsWith("/api/v0/sigma-rule-candidates")) {
      return respond(sigmaCandidatesResponse());
    }
    if (input.startsWith("/api/v0/investigation-order/")) {
      return respond(investigationOrderResponseJson());
    }
    if (input.startsWith("/api/v0/event-kinds")) {
      return respond(eventKindsJson);
    }
    if (input.startsWith("/api/v0/nodes/")) {
      return respond(nodeJson);
    }
    if (input.startsWith(terminalListPrefix)) {
      return respond(terminalJson);
    }
    return respond(graphJson);
  });
  vi.stubGlobal("fetch", mock);
  return mock;
}

export type FetchMock = ReturnType<typeof stubFetch>;

/**
 * 図の要求の URL を、送った順に返す。端末の選択肢の要求を除く。
 * 第 1 引数に URL を受け取る fetch の mock なら、stubFetch 以外が作ったものも読める。
 *
 * nodeLimit を持たない要求は、先頭に limitQuery を置いて返す。上限を置かない要求と、上限を
 * 置いた要求を、同じ形の文字列で比べられる。
 *
 * **端末の選択肢の要求は、同じ文字列の 1 回目だけを除く。** 上限を置かない、ホップ数 0 の
 * 端末の図の要求は、端末の選択肢の要求と同じ文字列になる。端末の選択肢は、関連付けの条件が
 * 変わるたびに別の文字列で取り直す。
 */
export function figureRequests(mock: {
  mock: { calls: ReadonlyArray<readonly [string, ...unknown[]]> };
}): string[] {
  const prefix = "/api/v0/graph?";
  const terminalLists = new Set<string>();
  return mock.mock.calls
    .map((call) => call[0])
    .filter((path) => {
      if (!path.startsWith(prefix)) return false;
      if (path.startsWith(terminalListPrefix) && !terminalLists.has(path)) {
        terminalLists.add(path);
        return false;
      }
      return true;
    })
    .map((path) =>
      path.includes("nodeLimit=")
        ? path
        : `${prefix}${limitQuery}&${path.slice(prefix.length)}`,
    );
}

/** figureRequests の形の要求を、画面が送った文字列へ戻す。先頭に置いた limitQuery を外す。 */
export function sentRequest(path: string): string {
  return path.replace(`?${limitQuery}&`, "?");
}

export function lastFigureRequest(mock: FetchMock): string | undefined {
  return figureRequests(mock).at(-1);
}

/**
 * 作業場所の中で描いているときは、「表示」メニューからビューを前面に出す。隠れたビューは
 * 中身を描かない。メニューバーの無い harness では何もしない。
 */
function revealView(title: string) {
  const bar = screen.queryByRole("menubar", { name: "画面の操作" });
  if (bar === null) return;
  fireEvent.click(within(bar).getByRole("menuitem", { name: "表示" }));
  fireEvent.click(screen.getByRole("menuitem", { name: title }));
}

/**
 * 「名前: 値」の組 1 つの要素を探す。組は名前と値が別の要素に分かれるため、li の
 * textContent で比べる。
 */
function isPair(text: string) {
  return (_: string, element: Element | null) =>
    element?.tagName === "LI" && element.textContent === text;
}

/** 「名前: 値」の組を探して返す。無いときは失敗する。 */
export function getPair(text: string, container: HTMLElement = document.body) {
  return within(container).getByText(isPair(text));
}

/** 「名前: 値」の組を探して返す。無いときは null を返す。 */
export function queryPair(
  text: string,
  container: HTMLElement = document.body,
) {
  return within(container).queryByText(isPair(text));
}

/** 「名前: 値」の組が描かれるのを待って返す。 */
export function findPair(text: string) {
  return screen.findByText(isPair(text));
}

/** Nodes のビューを前面に出し、ノードの一覧の table を待って返す。 */
export async function findNodeList() {
  const name = "グラフのノードの一覧";
  if (screen.queryByRole("table", { name }) === null) revealView("Nodes");
  return screen.findByRole("table", { name });
}

/** Edges のビューを前面に出し、エッジの一覧の table を待って返す。 */
export async function findEdgeList() {
  const name = "グラフのエッジの一覧";
  if (screen.queryByRole("table", { name }) === null) revealView("Edges");
  return screen.findByRole("table", { name });
}
