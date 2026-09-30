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
import { buildFetchFailure } from "@/shared/api/apiFailure";
import type { NodeRef, RecordFilterCriteria } from "@/shared/api/graph";
import {
  everyMatchCondition,
  type MatchConditionSelection,
} from "@/shared/api/matchConditions";
import type { CaseId } from "@/shared/contracts/cases";
import type { EventKindsResponse } from "@/shared/contracts/eventKinds";
import type {
  FetchState,
  SearchExpressionFailure,
} from "@/shared/lib/fetchState";
import { noSearchTerms, type SearchTerms } from "@/shared/lib/searchTerms";
import {
  autoView,
  type ResolvedView,
  resolveView,
  type ViewChoice,
} from "@/shared/lib/searchView";
import { baselineCaseId } from "@/testdata/cases/caseCounts";
import { restoreClipboard, stubClipboard } from "@/testdata/clipboard";
import {
  addCondition,
  chooseConditionKind,
  chooseConditionValue,
  conditionValueOptions,
  openConditionValues,
} from "@/testdata/conditionInput";
import { SearchConditions } from "./SearchConditions";
import type { SubgraphCriteria } from "./useSubgraph";

afterEach(() => {
  cleanup();
  restoreClipboard();
});

const hostC: NodeRef = { id: "n:terminal:01", label: '"HOST-C"' };
const hostD: NodeRef = { id: "n:terminal:02", label: '"HOST-D"' };
const loadedTerminals: FetchState<NodeRef[]> = {
  status: "loaded",
  value: [hostC, hostD],
};
const sourceFileNames: ReadonlyMap<string, string> = new Map([
  ["src-a", "alpha.log"],
  ["src-b", "beta.log"],
]);

/** 事象の種別の候補。ps の 2 組と file の 1 組を持ち、分類を持たないレコードも数える。 */
function loadedEventKinds(
  uncategorizedRecordCount = 3,
): FetchState<EventKindsResponse> {
  return {
    status: "loaded",
    value: {
      kinds: [
        {
          category: "file",
          action: "close",
          recordCount: 30,
          windowsEvent: false,
        },
        {
          category: "ps",
          action: "start",
          recordCount: 20,
          windowsEvent: false,
        },
        {
          category: "ps",
          action: "stop",
          recordCount: 10,
          windowsEvent: false,
        },
      ],
      uncategorizedRecordCount,
    },
  };
}

const baseCriteria: SubgraphCriteria = {
  matchConditions: everyMatchCondition(),
  depth: 1,
};

function renderConditions(
  options: {
    terms?: SearchTerms;
    recordFilter?: RecordFilterCriteria;
    terminals?: FetchState<NodeRef[]>;
    sourceFileNames?: ReadonlyMap<string, string>;
    eventKinds?: FetchState<EventKindsResponse>;
    view?: ViewChoice;
    resolvedView?: ResolvedView;
    expressionError?: SearchExpressionFailure;
    criteria?: Partial<SubgraphCriteria>;
    matchConditions?: MatchConditionSelection;
    caseIds?: readonly CaseId[];
  } = {},
) {
  const onChangeTerms = vi.fn();
  const onApplyRecordFilter = vi.fn();
  const onChangeView = vi.fn();
  const onApplyCriteria = vi.fn();
  const onApplyEdgeKinds = vi.fn();
  const onApplyMatchConditions = vi.fn();
  const onChangeMergeSameAccount = vi.fn();
  const view = options.view ?? autoView;
  render(
    <SearchConditions
      terms={options.terms ?? noSearchTerms}
      onChangeTerms={onChangeTerms}
      expressionError={options.expressionError}
      recordFilter={options.recordFilter ?? {}}
      onApplyRecordFilter={onApplyRecordFilter}
      terminals={options.terminals ?? loadedTerminals}
      sourceFileNames={options.sourceFileNames ?? sourceFileNames}
      eventKinds={options.eventKinds ?? loadedEventKinds()}
      view={view}
      resolvedView={
        options.resolvedView ?? resolveView(view, { hasCondition: false })
      }
      onChangeView={onChangeView}
      criteria={{ ...baseCriteria, ...options.criteria }}
      onApplyCriteria={onApplyCriteria}
      onApplyEdgeKinds={onApplyEdgeKinds}
      matchConditions={options.matchConditions ?? everyMatchCondition()}
      onApplyMatchConditions={onApplyMatchConditions}
      caseIds={options.caseIds ?? []}
      mergeSameAccount={false}
      onChangeMergeSameAccount={onChangeMergeSameAccount}
    />,
  );
  return {
    onChangeMergeSameAccount,
    onChangeTerms,
    onApplyRecordFilter,
    onChangeView,
    onApplyCriteria,
    onApplyEdgeKinds,
    onApplyMatchConditions,
  };
}

function chipList(): HTMLElement {
  return screen.getByRole("list", { name: "適用している検索の条件" });
}

test("同じアカウントとしてまとめる切り替えは、要求の条件を変えずに知らせる", () => {
  const { onChangeMergeSameAccount, onApplyCriteria, onApplyRecordFilter } =
    renderConditions();
  fireEvent.click(
    screen.getByRole("checkbox", { name: "同じアカウントとしてまとめる" }),
  );
  expect(onChangeMergeSameAccount).toHaveBeenCalledWith(true);
  expect(onApplyCriteria).not.toHaveBeenCalled();
  expect(onApplyRecordFilter).not.toHaveBeenCalled();
});

test("条件が無いときは、条件の一覧を出さず、種類を選ぶ欄だけを出す", () => {
  renderConditions();

  expect(screen.getByRole("combobox", { name: "条件の種類" })).toBeTruthy();
  expect(screen.queryByText(/取り込んだ全体/)).toBeNull();
  expect(
    screen.queryByRole("list", { name: "適用している検索の条件" }),
  ).toBeNull();
});

test("「?」は条件の入力の隣の 1 つだけであり、文字列の一致と検索式の書き方を条件の入力の説明に出す", () => {
  renderConditions();

  const helps = screen.getAllByRole("button", { name: / の説明$/ });
  expect(helps.map((button) => button.getAttribute("aria-label"))).toEqual([
    "条件の入力 の説明",
  ]);
  fireEvent.click(helps[0] as HTMLElement);
  const tooltip = screen.getByRole("tooltip");
  expect(within(tooltip).getByText("文字列の一致")).toBeInTheDocument();
  expect(within(tooltip).getByText("検索式の書き方")).toBeInTheDocument();
});

test("含む・含まないの種類で入れた文字列を、その向きの条件に足す", () => {
  const { onChangeTerms } = renderConditions();

  addCondition("含む", { 含む文字列: " 203.0.113.15 " });
  expect(onChangeTerms).toHaveBeenLastCalledWith({
    contains: ["203.0.113.15"],
    excludes: [],
  });

  addCondition("含まない", { 含まない文字列: "dcon" });
  expect(onChangeTerms).toHaveBeenLastCalledWith({
    contains: [],
    excludes: ["dcon"],
  });
});

test("適用している条件を一覧に出し、外す button でその条件だけを外す", () => {
  const { onChangeTerms, onApplyRecordFilter } = renderConditions({
    terms: { contains: ["code", "tunnel"], excludes: ["dcon"] },
    recordFilter: { terminal: hostC, eventCategory: "ps" },
  });

  const list = chipList();
  expect(list.textContent).toContain("含む文字列: code");
  expect(list.textContent).toContain("含まない文字列: dcon");
  expect(list.textContent).toContain('Host: "HOST-C"');
  expect(list.textContent).toContain("イベントの分類: ps");

  fireEvent.click(
    screen.getByRole("button", { name: "含む文字列 code を削除" }),
  );
  expect(onChangeTerms).toHaveBeenLastCalledWith({
    contains: ["tunnel"],
    excludes: ["dcon"],
  });

  fireEvent.click(screen.getByRole("button", { name: 'Host "HOST-C" を削除' }));
  expect(onApplyRecordFilter).toHaveBeenLastCalledWith({
    terminal: undefined,
    eventCategory: "ps",
  });
});

test("検索式を入れて足すと、前後の空白を外した式で条件を変える", () => {
  const { onChangeTerms } = renderConditions({
    terms: { contains: ["code"], excludes: [] },
  });

  addCondition("検索式", {
    検索式: "  LogonType == 3 and not user contains admin ",
  });

  expect(onChangeTerms).toHaveBeenLastCalledWith({
    contains: ["code"],
    excludes: [],
    expression: "LogonType == 3 and not user contains admin",
  });
});

test("空白だけの検索式は、誤りを出して条件を変えない", () => {
  const { onChangeTerms } = renderConditions();

  addCondition("検索式", { 検索式: " \t " });

  expect(screen.getByText("入力なし")).toBeTruthy();
  expect(onChangeTerms).not.toHaveBeenCalled();
});

test("適用している検索式は、値の入力を開くとその式を入れた状態で出す", () => {
  renderConditions({
    terms: { contains: [], excludes: [], expression: "LogonType == 3" },
  });
  chooseConditionKind("検索式");
  expect(screen.getByLabelText("検索式")).toHaveValue("LogonType == 3");
});

test("適用している検索式を条件の一覧に出し、一覧から外すと他の条件を残す", () => {
  const { onChangeTerms } = renderConditions({
    terms: {
      contains: ["code"],
      excludes: [],
      expression: 'user == "a b"',
    },
  });

  expect(chipList().textContent).toContain('検索式: user == "a b"');

  fireEvent.click(
    screen.getByRole("button", { name: '検索式 user == "a b" を削除' }),
  );
  expect(onChangeTerms).toHaveBeenLastCalledWith({
    contains: ["code"],
    excludes: [],
  });
});

test("上位の画面が渡した検索式の誤りを、入力欄の下に出す。式を外している間は出さない", () => {
  const error: SearchExpressionFailure = {
    reason: "missing_value",
    offset: 12,
    length: 0,
    description: "演算子の後に値がありません。",
  };
  renderConditions({
    terms: { contains: [], excludes: [], expression: "LogonType ==" },
    expressionError: error,
  });

  const region = screen.getByRole("region", { name: "検索式の誤り" });
  expect(region.textContent).toContain(
    "検索式の誤り: 演算子の後に値がありません。",
  );
  cleanup();

  renderConditions({ expressionError: error });
  expect(screen.queryByRole("region", { name: "検索式の誤り" })).toBeNull();
});

test("Host を選ぶと、その端末の参照で根拠のレコードにフィルタを適用し、選び直すと置き換える", () => {
  const { onApplyRecordFilter } = renderConditions({
    recordFilter: { terminal: hostC },
  });

  chooseConditionValue("Host", '"HOST-D"');

  expect(onApplyRecordFilter).toHaveBeenLastCalledWith({ terminal: hostD });
});

test("Artifact を選ぶと足していき、選んだ収集元は候補から外れ、条件の一覧から 1 つずつ外せる", () => {
  const { onApplyRecordFilter } = renderConditions();
  chooseConditionValue("Artifact", "beta.log");
  expect(onApplyRecordFilter).toHaveBeenLastCalledWith({
    sources: [{ id: "src-b", label: "beta.log" }],
  });
  cleanup();

  const chosen = renderConditions({
    recordFilter: { sources: [{ id: "src-a", label: "alpha.log" }] },
  });
  expect(chipList().textContent).toContain("Artifact: alpha.log");
  expect(conditionValueOptions("Artifact")).toEqual(["beta.log"]);
  chooseConditionValue("Artifact", "beta.log");
  expect(chosen.onApplyRecordFilter).toHaveBeenLastCalledWith({
    sources: [
      { id: "src-a", label: "alpha.log" },
      { id: "src-b", label: "beta.log" },
    ],
  });
  fireEvent.click(
    screen.getByRole("button", { name: "Artifact alpha.log を削除" }),
  );
  expect(chosen.onApplyRecordFilter).toHaveBeenLastCalledWith({
    sources: [],
  });
});

test("端末の一覧を読み込むまで、Host の候補を選べない", () => {
  renderConditions({ terminals: { status: "loading" } });

  chooseConditionKind("Host");
  expect(screen.getByRole("combobox", { name: "Host" })).toBeDisabled();
});

/** 事象の種別の候補を開き、区分の見出しを並べる。見出しは押せない行である。 */
function eventKindSections(): string[] {
  openConditionValues("イベントの種類");
  return within(screen.getByRole("listbox"))
    .getAllByRole("option")
    .filter((option) => option.getAttribute("aria-disabled") === "true")
    .map((option) => option.textContent ?? "");
}

/** 開いている候補の一覧のうち、選べる候補の表示の文字列を並べる。 */
function choosableTexts(): string[] {
  return screen
    .getAllByRole("option")
    .filter((option) => option.getAttribute("aria-disabled") !== "true")
    .map((option) => option.firstElementChild?.textContent ?? "");
}

test("イベントの種類の候補は、応答の分類ごとの区分に、組と件数を並べる", () => {
  renderConditions();

  expect(eventKindSections()).toEqual(["file", "ps"]);
  expect(
    screen
      .getAllByRole("option")
      .map((option) => option.textContent)
      .slice(3),
  ).toEqual(["ps", "ps のすべて30", "ps / start20", "ps / stop10"]);
});

test("Windows イベントログの組は、プロバイダとイベント ID の候補として並び、選ぶとその組でフィルタを適用する", () => {
  const provider = "Microsoft-Windows-Security-Auditing";
  const { onApplyRecordFilter } = renderConditions({
    eventKinds: {
      status: "loaded",
      value: {
        kinds: [
          {
            category: provider,
            action: "4624",
            recordCount: 7,
            windowsEvent: true,
          },
          {
            category: provider,
            action: "4688",
            recordCount: 2,
            windowsEvent: true,
          },
          {
            category: "ps",
            action: "start",
            recordCount: 1,
            windowsEvent: false,
          },
        ],
        uncategorizedRecordCount: 0,
      },
    },
  });

  expect(eventKindSections()).toEqual([`プロバイダ: ${provider}`, "ps"]);
  expect(choosableTexts()).toEqual([
    `${provider} のすべて`,
    "イベント ID 4624",
    "イベント ID 4688",
    // 反対側。Windows イベントログでない組にはイベント ID と書かない。
    "ps のすべて",
    "ps / start",
  ]);

  chooseConditionValue("イベントの種類", "イベント ID 4624");
  expect(onApplyRecordFilter).toHaveBeenLastCalledWith({
    eventCategory: provider,
    eventAction: "4624",
  });
});

/** 分類 Example-Provider の組を与えた事象の種別の候補。windowsEvent は組ごとに与える。 */
function providerEventKinds(
  windowsEvents: readonly boolean[],
): FetchState<EventKindsResponse> {
  return {
    status: "loaded",
    value: {
      kinds: windowsEvents.map((windowsEvent, index) => ({
        category: "Example-Provider",
        action: String(42 + index),
        recordCount: 1,
        windowsEvent,
      })),
      uncategorizedRecordCount: 0,
    },
  };
}

test("事象の種別は分類と動作を 1 つの chip にし、Windows イベントログの組はプロバイダとイベント ID の名前で出す", () => {
  renderConditions({
    eventKinds: providerEventKinds([true]),
    recordFilter: { eventCategory: "Example-Provider", eventAction: "42" },
  });
  expect(within(chipList()).getAllByRole("listitem")).toHaveLength(1);
  expect(chipList().textContent).toContain(
    "プロバイダ / イベント ID: Example-Provider / 42",
  );
  cleanup();

  // 反対側。組が今の範囲に無いときは Windows イベントログの組と判定できず、分類と動作の名前にする。
  const { onApplyRecordFilter } = renderConditions({
    eventKinds: providerEventKinds([true]),
    recordFilter: { eventCategory: "Example-Provider", eventAction: "7" },
  });
  expect(chipList().textContent).toContain(
    "イベントの分類 / イベントの動作: Example-Provider / 7",
  );
  fireEvent.click(
    screen.getByRole("button", {
      name: "イベントの分類 / イベントの動作 Example-Provider / 7 を削除",
    }),
  );
  expect(onApplyRecordFilter).toHaveBeenLastCalledWith({
    eventCategory: undefined,
    eventAction: undefined,
  });
});

test("同じ分類に Windows イベントログの組とほかの組が混ざると、分類と動作の表示に戻す", () => {
  renderConditions({
    eventKinds: providerEventKinds([true, false]),
    recordFilter: { eventCategory: "Example-Provider" },
  });

  expect(chipList().textContent).toContain("イベントの分類: Example-Provider");
  expect(eventKindSections()).toEqual(["Example-Provider"]);
  expect(choosableTexts()).toEqual([
    "Example-Provider のすべて",
    "Example-Provider / 42",
    "Example-Provider / 43",
  ]);
});

test("適用している条件を外す button の名前は、原資料の制御文字を見える符号にする", () => {
  const bidi = "‮";
  const { onChangeTerms } = renderConditions({
    terms: { contains: [`a${bidi}b`], excludes: [] },
  });

  fireEvent.click(
    screen.getByRole("button", { name: "含む文字列 aU+202Eb を削除" }),
  );
  expect(onChangeTerms).toHaveBeenLastCalledWith({
    contains: [],
    excludes: [],
  });
});

// 原資料の文字列の制御文字を候補にそのまま描画すると、表示の並びを偽装できる。
test("事象の種別と Host の候補は、原資料の制御文字を見える符号にして描画し、選ぶと原文を渡す", () => {
  const bidi = "‮";
  const disguised: NodeRef = { id: "n:terminal:03", label: `WS${bidi}03` };
  const { onApplyRecordFilter } = renderConditions({
    terminals: { status: "loaded", value: [disguised] },
    eventKinds: {
      status: "loaded",
      value: {
        kinds: [
          {
            category: `p${bidi}s`,
            action: `st${bidi}art`,
            recordCount: 1,
            windowsEvent: false,
          },
        ],
        uncategorizedRecordCount: 0,
      },
    },
  });

  const hostTexts = conditionValueOptions("Host");
  expect(hostTexts.join("")).not.toContain(bidi);
  expect(hostTexts).toContain("WSU+202E03");

  eventKindSections();
  const listbox = screen.getByRole("listbox");
  expect(listbox.textContent).not.toContain(bidi);
  expect(listbox.textContent).toContain("pU+202Es / stU+202Eart");

  chooseConditionValue("イベントの種類", "pU+202Es / ");
  expect(onApplyRecordFilter).toHaveBeenLastCalledWith({
    eventCategory: `p${bidi}s`,
    eventAction: `st${bidi}art`,
  });
});

test("事象の種別を選ぶと、選んだ分類と動作でフィルタを適用し、分類のすべてを選ぶと動作を外す", () => {
  const { onApplyRecordFilter } = renderConditions({
    recordFilter: { terminal: hostC, eventCategory: "ps", eventAction: "stop" },
  });

  chooseConditionValue("イベントの種類", "ps / start");
  expect(onApplyRecordFilter).toHaveBeenLastCalledWith({
    terminal: hostC,
    eventCategory: "ps",
    eventAction: "start",
  });

  chooseConditionValue("イベントの種類", "ps のすべて");
  expect(onApplyRecordFilter).toHaveBeenLastCalledWith({
    terminal: hostC,
    eventCategory: "ps",
    eventAction: undefined,
  });
});

test("事象の種別の一覧を読み込むまで選べず、失敗したときは失敗を出す", () => {
  renderConditions({ eventKinds: { status: "loading" } });
  chooseConditionKind("イベントの種類");
  expect(
    screen.getByRole("combobox", { name: "イベントの種類" }),
  ).toBeDisabled();
  cleanup();

  renderConditions({
    eventKinds: {
      status: "failed",
      failure: buildFetchFailure("network", "イベントの種類の一覧の取得"),
    },
  });
  chooseConditionKind("イベントの種類");
  expect(screen.getByRole("alert").textContent).toContain(
    "イベントの種類の一覧の取得",
  );
});

test("イベントの種類の無いレコードがあるときだけ、フィルタで除外するその件数を書く", () => {
  const note = "フィルタで除外するイベントの種類の無いレコード: 3";
  renderConditions();
  chooseConditionKind("イベントの種類");
  expect(screen.getByText(note)).toBeTruthy();
  cleanup();

  renderConditions({ eventKinds: loadedEventKinds(0) });
  chooseConditionKind("イベントの種類");
  expect(screen.queryByText(/イベントの種類の無いレコード/)).toBeNull();
});

/** ノードの種類の組の文字列を返す。 */
function viewPair(): string | undefined {
  return Array.from(
    document.querySelectorAll("ul.value-pairs > li"),
    (item) => item.textContent ?? "",
  ).find((text) => text.startsWith("ノードの種類: "));
}

test("自動のときは今出している種類を書き、ノードの種類を追加すると今の種類にそれを追加した手の選択を渡す", () => {
  const { onChangeView } = renderConditions();

  expect(viewPair()).toBe("ノードの種類: 自動: 端末、IP アドレス");
  expect(
    screen.queryByRole("list", { name: "適用している検索の条件" }),
  ).toBeNull();

  chooseConditionValue("ノードの種類", "プロセス");
  expect(onChangeView).toHaveBeenLastCalledWith({
    kind: "manual",
    nodeKinds: ["terminal", "process", "ip"],
  });
});

test("手で選んだ種別を種類の色の線を持つ chip にし、最後の chip を外すと自動へ戻す", () => {
  const view: ViewChoice = { kind: "manual", nodeKinds: ["process", "file"] };
  const { onChangeView } = renderConditions({
    view,
    resolvedView: resolveView(view, { hasCondition: false }),
  });

  expect(viewPair()).toBe("ノードの種類: プロセス、ファイル");
  const remove = screen.getByRole("button", {
    name: "ノードの種類 ファイル を削除",
  });
  expect(remove.closest("li")?.style.borderLeftColor).toBe("var(--kind-file)");
  expect(conditionValueOptions("ノードの種類")).not.toContain("プロセス");
  fireEvent.click(remove);
  expect(onChangeView).toHaveBeenLastCalledWith({
    kind: "manual",
    nodeKinds: ["process"],
  });
  cleanup();

  const single = renderConditions({
    view: { kind: "manual", nodeKinds: ["process"] },
  });
  fireEvent.click(
    screen.getByRole("button", { name: "ノードの種類 プロセス を削除" }),
  );
  expect(single.onChangeView).toHaveBeenLastCalledWith(autoView);
});

test("イベント ID の範囲を入れて追加すると、最小と最大を条件に渡し、片側だけでも追加できる", () => {
  const { onApplyRecordFilter } = renderConditions({
    recordFilter: { eventCategory: "Example-Provider" },
  });
  addCondition("イベント ID の範囲", {
    "イベント ID の最小": "8000",
    "イベント ID の最大": "8999",
  });
  expect(onApplyRecordFilter).toHaveBeenLastCalledWith({
    eventCategory: "Example-Provider",
    eventActionFrom: 8000,
    eventActionTo: 8999,
  });
  addCondition("イベント ID の範囲", { "イベント ID の最小": "4000" });
  expect(onApplyRecordFilter).toHaveBeenLastCalledWith({
    eventCategory: "Example-Provider",
    eventActionFrom: 4000,
    eventActionTo: undefined,
  });
});

test("数でない文字列と、最小が最大を超える範囲は追加できない", () => {
  const { onApplyRecordFilter } = renderConditions();
  addCondition("イベント ID の範囲", {
    "イベント ID の最小": "9000",
    "イベント ID の最大": "8000",
  });
  expect(screen.getByText("最大より大きい最小")).toBeTruthy();
  fireEvent.change(screen.getByLabelText("イベント ID の最大"), {
    target: { value: "8a" },
  });
  fireEvent.click(screen.getByRole("button", { name: "条件に追加" }));
  expect(onApplyRecordFilter).not.toHaveBeenCalled();
});

test("範囲とフィールドの指定を条件の一覧に出し、外せる", () => {
  const { onApplyRecordFilter, onChangeTerms } = renderConditions({
    terms: { contains: ["example"], excludes: [], field: "CommandLine" },
    recordFilter: { eventActionFrom: 8000, eventActionTo: 8999 },
  });
  fireEvent.click(
    screen.getByRole("button", {
      name: "イベント ID の範囲 8000–8999 を削除",
    }),
  );
  expect(onApplyRecordFilter).toHaveBeenLastCalledWith({
    eventActionFrom: undefined,
    eventActionTo: undefined,
  });
  fireEvent.click(
    screen.getByRole("button", {
      name: "文字列を探すフィールド CommandLine を削除",
    }),
  );
  expect(onChangeTerms).toHaveBeenLastCalledWith({
    contains: ["example"],
    excludes: [],
  });
});

test("フィールドを指定の種類でフィールドを入れると、文字列の条件にフィールドを追加する", () => {
  const { onChangeTerms } = renderConditions({
    terms: { contains: ["example"], excludes: [] },
  });
  addCondition("フィールドを指定", {
    文字列を探すフィールド: " process.command_line ",
  });
  expect(onChangeTerms).toHaveBeenLastCalledWith({
    contains: ["example"],
    excludes: [],
    field: "process.command_line",
  });
});

test("フィールドの値の種類は、含む組と完全一致の組を追加し、「=」を含むフィールドを退ける", () => {
  const { onChangeTerms } = renderConditions();
  addCondition("フィールドの値が文字列を含む", {
    フィールド: "TargetUserName",
    値: "admin",
  });
  expect(onChangeTerms).toHaveBeenLastCalledWith({
    contains: [],
    excludes: [],
    fieldContains: ["TargetUserName=admin"],
  });

  addCondition("フィールドの値が等しい", {
    フィールド: "EventRecordID",
    値: "42",
  });
  expect(onChangeTerms).toHaveBeenLastCalledWith({
    contains: [],
    excludes: [],
    fieldEquals: ["EventRecordID=42"],
  });

  onChangeTerms.mockClear();
  addCondition("フィールドの値が文字列を含む", { フィールド: "a=b", 値: "x" });
  expect(screen.getByText("フィールド名の中の「=」")).toBeTruthy();
  expect(onChangeTerms).not.toHaveBeenCalled();
});

test("ホップ数・エッジの種類・件数を数えるフィールド・アドレスの範囲・一致ノードだけ・両端が期間内を条件に渡し、chip から外す", () => {
  const { onApplyCriteria, onApplyEdgeKinds } = renderConditions({
    criteria: {
      depth: 2,
      edgeKinds: ["process_communication"],
      countBy: "LogonType",
      addressInCidr: "198.51.100.0/24",
      conditionsOnOriginsOnly: true,
      endpointRecordsInPeriod: true,
    },
  });
  const list = chipList();
  // ホップ数の chip は数だけを出し、意味は chip の説明に置く。
  const depthChip = within(list)
    .getAllByRole("listitem")
    .find((item) => item.textContent?.startsWith("ホップ数"));
  expect(depthChip?.textContent).toBe("ホップ数: 2");
  expect(within(depthChip as HTMLElement).getByText("2").title).toBe(
    "一致ノードから辿るエッジの本数: 2",
  );
  expect(list.textContent).toContain("件数を数えるフィールド: LogonType");
  expect(list.textContent).toContain("CIDR の内のアドレス: 198.51.100.0/24");
  expect(list.textContent).toContain("条件の適用先: 一致ノードだけ");
  expect(list.textContent).toContain("表示するエッジ: 両端のレコードが期間内");

  chooseConditionValue("ホップ数", "0");
  expect(onApplyCriteria).toHaveBeenLastCalledWith(
    expect.objectContaining({ depth: 0 }),
  );
  fireEvent.click(screen.getByRole("button", { name: /^ホップ数 .* を削除$/ }));
  expect(onApplyCriteria).toHaveBeenLastCalledWith(
    expect.objectContaining({ depth: 1 }),
  );

  chooseConditionValue("エッジの種類", "ファイルの操作");
  expect(onApplyEdgeKinds).toHaveBeenLastCalledWith([
    "file_operation",
    "process_communication",
  ]);
  fireEvent.click(
    screen.getByRole("button", { name: /^エッジの種類 .* を削除$/ }),
  );
  expect(onApplyEdgeKinds).toHaveBeenLastCalledWith(undefined);

  fireEvent.click(
    screen.getByRole("button", {
      name: "条件の適用先 一致ノードだけ を削除",
    }),
  );
  expect(onApplyCriteria).toHaveBeenLastCalledWith(
    expect.objectContaining({ conditionsOnOriginsOnly: undefined }),
  );
});

test("推定条件は、すべての条件のときは chip を出さず、選び直すと chip にして外すとすべてへ戻す", () => {
  const { onApplyMatchConditions } = renderConditions();
  expect(
    screen.queryByRole("list", { name: "適用している検索の条件" }),
  ).toBeNull();
  chooseConditionKind("推定条件");
  for (const checkbox of screen.getAllByRole("checkbox")) {
    if ((checkbox as HTMLInputElement).checked) fireEvent.click(checkbox);
  }
  fireEvent.click(screen.getByLabelText("接続先 port"));
  fireEvent.click(screen.getByRole("button", { name: "推定条件を適用" }));
  expect(onApplyMatchConditions).toHaveBeenLastCalledWith({
    conditions: [{ conditionKey: "destination_port" }],
  });
  cleanup();

  const chosen = renderConditions({
    matchConditions: { conditions: [{ conditionKey: "destination_port" }] },
  });
  fireEvent.click(
    screen.getByRole("button", { name: "推定条件 接続先 port を削除" }),
  );
  expect(chosen.onApplyMatchConditions).toHaveBeenLastCalledWith(
    everyMatchCondition(),
  );
});

test("案件は、案件の付いた収集元があるときだけ種類に出し、選ぶとその案件でフィルタを適用する", () => {
  renderConditions();
  const kinds = screen.getByRole("combobox", { name: "条件の種類" });
  act(() => kinds.focus());
  expect(screen.getByRole("listbox").textContent).not.toContain("案件");
  cleanup();

  const { onApplyRecordFilter } = renderConditions({
    caseIds: [baselineCaseId],
  });
  chooseConditionValue("案件", "baseline");
  expect(onApplyRecordFilter).toHaveBeenLastCalledWith({
    caseId: baselineCaseId,
  });
});

/** 外す button の名前で chip を指し、右クリックでその chip のメニューを開く。 */
function openChipMenu(removeName: string) {
  const chip = screen.getByRole("button", { name: removeName }).closest("li");
  if (chip === null) throw new Error("the condition chip is missing");
  fireEvent.contextMenu(chip);
  return screen.getByRole("menu");
}

function menuItemNames(menu: HTMLElement): (string | null)[] {
  return Array.from(
    menu.querySelectorAll('[role="menuitem"]'),
    (item) => item.textContent,
  );
}

test("含む文字列・含まない文字列の chip のメニューは、削除と向きの変更と値のコピーの操作を出す", () => {
  const terms: SearchTerms = { contains: ["example"], excludes: ["svchost"] };
  const { onChangeTerms } = renderConditions({ terms });

  const contains = openChipMenu("含む文字列 example を削除");
  expect(contains).toHaveAccessibleName("含む文字列 example の操作");
  expect(menuItemNames(contains)).toEqual([
    "条件を削除",
    "含まない文字列に変更",
    "値をコピー",
  ]);
  fireEvent.click(
    screen.getByRole("menuitem", { name: "含まない文字列に変更" }),
  );
  expect(onChangeTerms).toHaveBeenLastCalledWith({
    contains: [],
    excludes: ["svchost", "example"],
  });

  openChipMenu("含まない文字列 svchost を削除");
  fireEvent.click(screen.getByRole("menuitem", { name: "含む文字列に変更" }));
  expect(onChangeTerms).toHaveBeenLastCalledWith({
    contains: ["example", "svchost"],
    excludes: [],
  });

  openChipMenu("含む文字列 example を削除");
  fireEvent.click(screen.getByRole("menuitem", { name: "条件を削除" }));
  expect(onChangeTerms).toHaveBeenLastCalledWith({
    contains: [],
    excludes: ["svchost"],
  });
});

test("含む文字列・含まない文字列以外の chip のメニューは削除と値のコピーの操作だけを持ち、削除は同じ条件を外す", () => {
  const { onApplyRecordFilter } = renderConditions({
    recordFilter: { terminal: hostC },
  });
  const menu = openChipMenu('Host "HOST-C" を削除');
  expect(menuItemNames(menu)).toEqual(["条件を削除", "値をコピー"]);
  fireEvent.click(screen.getByRole("menuitem", { name: "条件を削除" }));
  expect(onApplyRecordFilter).toHaveBeenLastCalledWith({
    terminal: undefined,
  });
});

test("chip の値のコピーは、表示に添えた説明を除いた条件の値をコピーする", async () => {
  const clipboard = stubClipboard();
  renderConditions({
    terms: { contains: [], excludes: [], field: "CommandLine" },
  });
  const chip = openChipMenu("文字列を探すフィールド CommandLine を削除");
  expect(chip).toBeTruthy();
  expect(screen.getByText("CommandLine").closest("span")?.title).toBe(
    "照合する文字列なし",
  );
  fireEvent.click(screen.getByRole("menuitem", { name: "値をコピー" }));

  expect(
    await screen.findByText("コピー済み: 文字列を探すフィールドの値"),
  ).toBeInTheDocument();
  expect(clipboard.writeText).toHaveBeenCalledWith("CommandLine");
});

test("chip で Shift+F10 を押すとメニューを開き、Escape で削除の button へ focus を戻す", () => {
  renderConditions({ terms: { contains: ["example"], excludes: [] } });
  const remove = screen.getByRole("button", {
    name: "含む文字列 example を削除",
  });
  remove.focus();
  fireEvent.keyDown(remove, { key: "F10", shiftKey: true });
  const menu = screen.getByRole("menu", {
    name: "含む文字列 example の操作",
  });
  expect(screen.getByRole("menuitem", { name: "条件を削除" })).toHaveFocus();
  fireEvent.keyDown(menu, { key: "Escape" });
  expect(remove).toHaveFocus();
});
