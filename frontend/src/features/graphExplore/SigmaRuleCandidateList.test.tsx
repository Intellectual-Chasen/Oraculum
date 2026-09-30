// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { SigmaRuleCandidatesResponse } from "@/shared/contracts/sigmaRuleCandidates";
import { openDetails } from "@/testdata/details";
import {
  sigmaCandidatesResponse,
  sigmaLocator,
} from "@/testdata/sigmaRuleCandidates/candidateResponse";
import { SigmaRuleCandidateList } from "./SigmaRuleCandidateList";

afterEach(cleanup);

function renderLoaded(
  value: SigmaRuleCandidatesResponse,
  highlightedRulePath?: string,
) {
  const actions = {
    onSelectRecord: vi.fn(),
    onSelectNode: vi.fn(),
    highlightedRulePath,
    onHighlightRule: vi.fn(),
  };
  render(
    <SigmaRuleCandidateList
      state={{ status: "loaded", value }}
      actions={actions}
    />,
  );
  return actions;
}

/** aria-label が label のセクションの「名前: 値」の組の文字列を並べて返す。 */
function pairsIn(label: string): string[] {
  return Array.from(
    screen.getByRole("region", { name: label }).querySelectorAll("li"),
    (item) => item.textContent ?? "",
  );
}

/** 表の本体の行を、セルの文字列の並びで返す。 */
function bodyRows(table: HTMLElement): (string | null)[][] {
  return within(table)
    .getAllByRole("row")
    .slice(1)
    .map((row) =>
      Array.from(row.querySelectorAll("th, td"), (cell) => cell.textContent),
    );
}

test("見出しは focus を受ける要素を含まない", () => {
  renderLoaded(sigmaCandidatesResponse());
  const heading = screen.getByRole("heading", { name: "Sigma ルールの候補" });
  expect(heading.querySelector("button, [tabindex]")).toBeNull();
});

test("ルールの集合の commit・出どころ・SHA-256・件数を示す", () => {
  renderLoaded(sigmaCandidatesResponse());
  expect(pairsIn("ルールの集合")).toEqual([
    "ルールの directory: synthetic-rules",
    `commit: ${"ab12".repeat(10)}`,
    "commit の出どころ: ルールの directory の git の HEAD",
    "git の作業ツリー: /synthetic/checkout",
    `SHA-256: ${"c".repeat(64)}`,
    "ルールの file: 3",
  ]);
  expect(pairsIn("評価の件数")).toEqual([
    "評価したルール: 2",
    "対象の Windows イベントログのレコード: 9",
    "一致したルール: 1",
    "一致したレコードの延べ数: 2",
    "評価しなかったルール: 1",
    "フィールドが無く適用しなかったレコード: 3",
    "意味を解釈できなかったレコード: 2",
    "logsource に該当しないレコード: 5",
  ]);
  expect(
    within(screen.getByRole("region", { name: "評価の件数" }))
      .getByText("3")
      .closest("span")
      ?.getAttribute("title"),
  ).toBe("ルールとレコードの組: 6");
  expect(
    bodyRows(
      screen.getByRole("table", {
        name: "ルールを適用しなかったレコードのチャネルとプロバイダ",
      }),
    ),
  ).toEqual([
    ["チャネル: Example/Operational", "4"],
    // 補足の印の文字列は読み上げに渡す。
    ["プロバイダ: Example-ProviderChannel フィールドの無いレコード", "1"],
  ]);
});

test("空のチャネルと空のプロバイダと、どちらのフィールドも無いレコードを分けて示す", () => {
  renderLoaded({
    ...sigmaCandidatesResponse(),
    unevaluatedRecordGroups: [
      { channel: "", recordCount: 2 },
      { provider: "", recordCount: 1 },
      { channelAndProviderAbsent: true, recordCount: 1 },
    ],
    recordsWithoutSemantics: 0,
  });
  expect(
    bodyRows(
      screen.getByRole("table", {
        name: "ルールを適用しなかったレコードのチャネルとプロバイダ",
      }),
    ),
  ).toEqual([
    ["チャネル: —空の値", "2"],
    ["プロバイダ: —空の値Channel フィールドの無いレコード", "1"],
    ["Channel と Provider のフィールドなし", "1"],
  ]);
  expect(pairsIn("評価の件数").join()).not.toContain("意味を解釈できなかった");
});

test("ルールを適用する対象のレコードが無い応答は、対象のレコード 0 件と評価しなかったルールを示す", () => {
  renderLoaded({
    ...sigmaCandidatesResponse(),
    evaluatedRecordCount: 0,
    rules: [],
    matches: [],
    skippedPairCount: 0,
    skippedPairRecordCount: 0,
  });
  const pairs = pairsIn("評価の件数");
  expect(pairs).toContain("対象の Windows イベントログのレコード: 0");
  expect(pairs).toContain("評価しなかったルール: 1");
  expect(pairs.join()).not.toContain("評価したルール:");
  expect(pairs.join()).not.toContain("フィールドが無く");
});

test("ルールの中身は開くまで描かない", () => {
  renderLoaded(sigmaCandidatesResponse());
  expect(screen.queryByRole("table", { name: "一致したレコード" })).toBeNull();
  expect(
    screen.queryByRole("table", { name: "評価しなかったルールの一覧" }),
  ).toBeNull();
  openDetails(/Synthetic Process Rule/);
  expect(screen.getByRole("table", { name: "一致したレコード" })).toBeDefined();
});

test("候補から一致したレコードの原文とルールの条件へ戻れる", () => {
  const response = sigmaCandidatesResponse();
  const { onSelectRecord } = renderLoaded(response);

  expect(screen.getByText("候補")).toBeDefined();
  openDetails(/Synthetic Process Rule/);
  expect(pairsIn("ルール")).toEqual([
    "ID: 00000000-0000-4000-8000-000000000001",
    "著者: Synthetic Author",
    "ルールの file: process/synthetic.yml",
    "condition: selection and not filter",
  ]);
  const rules = screen.getByRole("list", { name: "一致したルール" });
  const selection = within(rules).getByRole("region", {
    name: "検索 selection",
  });
  expect(within(selection).getByText("一致")).toBeTruthy();
  // 定義の YAML は行の区切りを改行のまま描く。
  expect(selection.querySelector("pre")?.textContent).toBe(
    "Image|endswith:\n    - \\tool.exe",
  );
  expect(
    within(
      within(rules).getByRole("region", { name: "検索 filter" }),
    ).getByText("不一致"),
  ).toBeTruthy();

  const records = within(rules).getByRole("table", {
    name: "一致したレコード",
  });
  const buttons = within(records)
    .getAllByRole("button")
    .filter(
      (button) => !button.getAttribute("aria-label")?.startsWith("ノード"),
    );
  expect(buttons).toHaveLength(response.matches.length);
  fireEvent.click(buttons[1] as HTMLElement);
  expect(onSelectRecord).toHaveBeenCalledWith(sigmaLocator(8));
});

test("一致したレコードのノードを押すとノードを選び、ノードの無い一致は無いことを示す", () => {
  const { onSelectNode } = renderLoaded(sigmaCandidatesResponse());
  openDetails(/Synthetic Process Rule/);
  const records = screen.getByRole("table", { name: "一致したレコード" });
  fireEvent.click(
    within(records).getByRole("button", {
      name: "ノード synthetic-events.xml:3 を選ぶ",
    }),
  );
  expect(onSelectNode).toHaveBeenCalledWith({
    id: "n:record:0003",
    label: "synthetic-events.xml:3",
    kind: "record",
  });
  expect(within(records).getByText("グラフにノードなし")).toBeDefined();
});

test("ルールの一致のノードとエッジを Graph で強調し、強調中は解除する", () => {
  const response = sigmaCandidatesResponse();
  const { onHighlightRule } = renderLoaded(response);
  openDetails(/Synthetic Process Rule/);
  expect(screen.getByText("ノード: 2 · エッジ: 1")).toBeDefined();
  fireEvent.click(screen.getByRole("button", { name: "Graph で強調" }));
  expect(onHighlightRule).toHaveBeenCalledWith(response.rules[0]);
  cleanup();

  const highlighted = renderLoaded(response, "process/synthetic.yml");
  openDetails(/Synthetic Process Rule/);
  const release = screen.getByRole("button", { name: "Graph の強調を解除" });
  expect(release.getAttribute("aria-pressed")).toBe("true");
  fireEvent.click(release);
  expect(highlighted.onHighlightRule).toHaveBeenCalledWith(undefined);
});

test("グラフに無いレコードの一致の数を示す", () => {
  renderLoaded({ ...sigmaCandidatesResponse(), outsideGraphMatchCount: 4 });
  expect(pairsIn("評価の件数")).toContain("グラフに無いレコードの一致: 4");
});

test("一致したレコードの端末と時刻を並べ、割り当てた端末と記録の無い端末を区別する", () => {
  const response = sigmaCandidatesResponse();
  const [first, second] = response.matches;
  renderLoaded({
    ...response,
    matches: [
      {
        ...(first as (typeof response.matches)[number]),
        terminal: "ws-1.example.test",
        eventTime: {
          rawText: "2031-04-05T06:07:08Z",
          normalized: "2031-04-05T06:07:08Z",
          normalizedForm: "rfc3339_absolute",
          precision: "second",
          offsetState: "in_value",
          clock: "terminal_local",
          meaning: "event",
          valueState: "present",
        },
      },
      {
        ...(second as (typeof response.matches)[number]),
        terminal: "ws-2",
        terminalAssigned: true,
      },
    ],
  });
  openDetails(/Synthetic Process Rule/);

  const table = screen.getByRole("table", { name: "一致したレコード" });
  const headers = within(table)
    .getAllByRole("columnheader")
    .map((cell) => cell.textContent);
  const [firstRow, secondRow] = bodyRows(table);
  expect(firstRow?.[headers.indexOf("端末")]).toBe("ws-1.example.test");
  expect(
    firstRow?.[headers.indexOf("時刻")]?.startsWith("2031-04-05T06:07:08Z"),
  ).toBe(true);
  expect(screen.getByText("ws-2").closest("span")?.title).toBe(
    "評価の時に収集元に割り当てた端末",
  );
  expect(secondRow?.[headers.indexOf("時刻")]).toBe("—時刻なし");
});

test("一致したレコードの時刻に、タイムゾーンの出どころを添える", () => {
  const response = sigmaCandidatesResponse();
  const [first] = response.matches;
  renderLoaded({
    ...response,
    matches: [
      {
        // 応答は一致を 2 件持つ。配列の添字の読み取りが undefined を含む型になる。
        ...(first as (typeof response.matches)[number]),
        eventTime: {
          rawText: "2031/04/05 06:07:08",
          normalized: "2031-04-05T06:07:08",
          normalizedForm: "local_without_offset",
          precision: "second",
          offsetState: "undetermined",
          clock: "terminal_local",
          meaning: "event",
          valueState: "present",
        },
      },
    ],
  });
  openDetails(/Synthetic Process Rule/);

  const table = screen.getByRole("table", { name: "一致したレコード" });
  const headers = within(table)
    .getAllByRole("columnheader")
    .map((cell) => cell.textContent);
  const time = bodyRows(table)[0]?.[headers.indexOf("時刻")];
  expect(time).toContain("2031/04/05 06:07:08");
  expect(time).toContain("タイムゾーン不明");
});

test("評価しなかったルールを件数と理由とともに示す", () => {
  const response = sigmaCandidatesResponse();
  renderLoaded({
    ...response,
    unevaluatedRules: [
      ...response.unevaluatedRules,
      {
        path: "other/broken.yml",
        id: "",
        title: "",
        reason: "yaml_unreadable",
        detail: "synthetic parse failure",
      },
    ],
  });

  openDetails(/^評価しなかったルール/);
  expect(
    screen.getByText(/^評価しなかったルール/, { selector: "summary" })
      .textContent,
  ).toBe("評価しなかったルール 2");
  expect(
    bodyRows(
      screen.getByRole("table", { name: "評価しなかった理由ごとの件数" }),
    ),
  ).toEqual([
    ["フィールド名の無い値の検索", "1"],
    ["YAML として解析できない", "1"],
  ]);
  expect(
    bodyRows(screen.getByRole("table", { name: "評価しなかったルールの一覧" })),
  ).toEqual([
    [
      "other/keywords.yml",
      "00000000-0000-4000-8000-000000000002",
      "Synthetic Keyword Rule",
      "フィールド名の無い値の検索",
      "selection keywords searches values without a field name",
    ],
    [
      "other/broken.yml",
      "—file に id なし",
      "—file に title なし",
      "YAML として解析できない",
      "synthetic parse failure",
    ],
  ]);
});

test("一致したレコードを 50 件ずつ出す", () => {
  const response = sigmaCandidatesResponse();
  const rule = response.rules[0];
  if (rule === undefined) throw new Error("synthetic rule is required");
  const matches = Array.from({ length: 60 }, (_, index) => ({
    rulePath: rule.path,
    record: sigmaLocator(index + 1),
    matchedSelections: ["selection"],
  }));
  renderLoaded({
    ...response,
    rules: [{ ...rule, matchCount: matches.length }],
    matches,
  });

  openDetails(/Synthetic Process Rule/);
  const records = screen.getByRole("table", { name: "一致したレコード" });
  const openButtons = () =>
    Array.from(records.querySelectorAll("button.value-link"));
  expect(openButtons()).toHaveLength(50);
  expect(screen.getByText("50 / 60")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "続きを表示" }));
  expect(openButtons()).toHaveLength(matches.length);
});

test("ルールの集合を渡していない起動を示す", () => {
  const { ruleSet: _, ...withoutRuleSet } = sigmaCandidatesResponse();
  renderLoaded({ ...withoutRuleSet, rules: [], matches: [] });
  expect(screen.getByText("Sigma ルールなし")).toBeDefined();
});

test("commit を確かめられなかった集合は理由を示す", () => {
  const response = sigmaCandidatesResponse();
  renderLoaded({
    ...response,
    ruleSet: {
      directory: "synthetic-rules",
      revisionSource: "unverified",
      revisionDetail: "the rule directory is not in a git working tree",
      contentSha256: "c".repeat(64),
      ruleFileCount: 3,
    },
  });
  expect(pairsIn("ルールの集合")).toEqual([
    "ルールの directory: synthetic-rules",
    "commit の出どころ: 未確認",
    "未確認の理由: the rule directory is not in a git working tree",
    `SHA-256: ${"c".repeat(64)}`,
    "ルールの file: 3",
  ]);
});
