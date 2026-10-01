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
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import type { CaseId } from "@/shared/contracts/cases";
import { challengeCaseId } from "@/testdata/cases/caseCounts";
import { jsonResponse } from "@/testdata/http";
import { searchExpressionErrorResponseJson } from "@/testdata/searchExpression/searchExpressionErrorResponse";
import { apiErrorJson } from "@/testdata/sources/sourcesResponse";
import {
  emptyTimelineResponseJson,
  evenlySpacedEntrySpecs,
  generatedSources,
  generatedTimelineResponseJson,
  timelineResponseJson,
} from "@/testdata/timeline/timelineResponse";
import { Timeline } from "./Timeline";

// 本 test が渡す関連付けの条件の選択。要求の URL が含む文字列を、期間と観測の種別の期待値から
// 分けて指定できるようにする。
const matchConditions: MatchConditionSelection = {
  conditions: [{ conditionKey: "destination_ip" }],
};
const matchConditionQuery = "matchCondition=destination_ip";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function stubFetch(json: unknown = timelineResponseJson(), status = 200) {
  const mock = vi.fn(async (_input: string, _init?: RequestInit) =>
    jsonResponse(status, json),
  );
  vi.stubGlobal("fetch", mock);
  return mock;
}

function renderTimeline(
  props: Partial<{
    timeFilter: undefined;
    eventCategory: string | undefined;
    eventAction: string | undefined;
    caseId: CaseId | undefined;
    terminal: string | undefined;
    valueContains: readonly string[] | undefined;
    valueExcludes: readonly string[] | undefined;
    valueField: string | undefined;
    fieldContains: readonly string[] | undefined;
    fieldEquals: readonly string[] | undefined;
    searchExpression: string | undefined;
  }> = {},
) {
  return render(
    <Timeline
      matchConditions={matchConditions}
      timeFilter={undefined}
      eventCategory={props.eventCategory}
      eventAction={props.eventAction}
      caseId={props.caseId}
      terminal={props.terminal}
      valueContains={props.valueContains}
      valueExcludes={props.valueExcludes}
      valueField={props.valueField}
      fieldContains={props.fieldContains}
      fieldEquals={props.fieldEquals}
      searchExpression={props.searchExpression}
      onSelectRecord={() => {}}
      dataVersion={0}
    />,
  );
}

const recordTableName = "時刻順のレコード";
const groupTableName = "時間の区切りごとのレコード";
const coverageTableName = "収集元ごとの記録期間";

test("アカウントの記録に役割と相手と接続元を示し、時刻以前の操作を渡す", async () => {
  const response = timelineResponseJson();
  const entry = response.entries[0];
  if (entry === undefined || entry.eventTime === undefined)
    throw new Error("fixture has no timed entry");
  const requester = {
    ...entry.account,
    id: "n:requester",
    label: { rawText: "requester", valueState: "present" },
  };
  const fetchMock = stubFetch({
    ...response,
    accountNodeId: "n:target",
    entries: [
      {
        ...entry,
        eventTime: {
          ...entry.eventTime,
          rawText: "2031-10-08T01:02:03.1234567Z",
          normalized: "2031-10-08T01:02:03.123456Z",
          precision: "microsecond",
        },
        accountRoles: ["record_target_account"],
        otherAccounts: [{ role: "record_subject_account", node: requester }],
        eventKind: { category: "Windows", action: "4769" },
        sourceAddress: "192.0.2.42",
      },
    ],
    entryCount: 1,
  });
  const onBefore = vi.fn();
  render(
    <Timeline
      matchConditions={matchConditions}
      timeFilter={undefined}
      eventCategory={undefined}
      eventAction={undefined}
      caseId={undefined}
      terminal={undefined}
      accountNodeId="n:target"
      onBefore={onBefore}
      onSelectRecord={() => {}}
      dataVersion={0}
    />,
  );
  const table = await screen.findByRole("table", { name: recordTableName });
  const request = new URL(
    String(fetchMock.mock.calls[0]?.[0]),
    "http://example.test",
  );
  expect(request.searchParams.get("accountNodeId")).toBe("n:target");
  expect(request.searchParams.has("nodeId")).toBe(false);
  expect(table.textContent).toContain("このアカウント: 操作対象");
  expect(table.textContent).toContain("requester");
  expect(table.textContent).toContain("4769");
  expect(table.textContent).toContain("192.0.2.42");
  fireEvent.click(within(table).getByRole("button", { name: "この時刻以前" }));
  expect(onBefore).toHaveBeenCalledWith(
    expect.objectContaining({ accountRoles: ["record_target_account"] }),
  );
});

test("役割を持たず名前だけを記録したアカウントも役割名を示す", async () => {
  const response = timelineResponseJson();
  const entry = response.entries[0];
  if (entry === undefined) throw new Error("fixture has no entry");
  stubFetch({
    ...response,
    accountNodeId: "n:account",
    entries: [{ ...entry, accountRoles: ["record_names_object"] }],
    entryCount: 1,
  });

  render(
    <Timeline
      matchConditions={matchConditions}
      timeFilter={undefined}
      eventCategory={undefined}
      eventAction={undefined}
      caseId={undefined}
      terminal={undefined}
      accountNodeId="n:account"
      onSelectRecord={() => {}}
      dataVersion={0}
    />,
  );

  const table = await screen.findByRole("table", { name: recordTableName });
  expect(table.textContent).toContain("このアカウント: 名前の記録");
});

/** 「名前: 値」の組のうち、名前が `name` の組の文字列を返す。無ければ undefined を返す。 */
function pairText(name: string): string | undefined {
  return screen
    .queryAllByRole("listitem")
    .map((item) => item.textContent ?? "")
    .find((text) => text.startsWith(`${name}:`));
}

test("収集元をまたいだ行を時刻順の 1 本の列として出す", async () => {
  stubFetch();

  renderTimeline();

  const table = await screen.findByRole("table", {
    name: recordTableName,
  });
  const rows = within(table).getAllByRole("row").slice(1);
  expect(rows).toHaveLength(2);
  // 行の並びが応答の並びのままである。画面が並べ替えない。
  expect(rows[0]?.textContent).toContain("2031/10/08 10:20:35.100");
  expect(rows[1]?.textContent).toContain("2031/10/08 10:20:36.200");
});

test("行から端末とアカウントとイベントの種類とレコードの位置と時刻の種類を読む", async () => {
  stubFetch();

  renderTimeline();

  const table = await screen.findByRole("table", {
    name: recordTableName,
  });
  const first = within(table).getAllByRole("row")[1];
  expect(first?.textContent).toContain("HOST-C");
  expect(first?.textContent).toContain("user01");
  expect(first?.textContent).toContain("con");
  expect(first?.textContent).toContain("ID: 2204");
  expect(first?.textContent).toContain("行: 1022");
  expect(first?.textContent).toContain("端末時刻");
  expect(first?.textContent).toContain("host-a.log");
});

test("byte 範囲で指すレコードの位置は、先頭の行番号と byte の位置を分けて出す", async () => {
  const base = timelineResponseJson();
  const [first, second] = base.entries;
  stubFetch({
    ...base,
    entries: [
      first,
      {
        ...second,
        recordRef: {
          ...second?.recordRef,
          positionKind: "byte_range",
          sequenceNumber: undefined,
          lineNumber: 7,
          lineCount: 3,
          byteOffset: 4096,
          byteLength: 512,
        },
      },
    ],
  });

  renderTimeline();

  const table = await screen.findByRole("table", {
    name: recordTableName,
  });
  const row = within(table).getAllByRole("row")[2];
  expect(row?.textContent).toContain("行: 7-9");
  expect(row?.textContent).toContain("位置: 4096-4608");
  expect(row?.textContent).not.toContain("byte 範囲 7");
});

test("同じレコードのほかの時刻の行は、別の行として時刻のフィールドの名前を添えて出す", async () => {
  const base = timelineResponseJson();
  const [first] = base.entries;
  stubFetch({
    ...base,
    entryCount: 2,
    entries: [first, { ...first, timeFieldName: "Run#2" }],
  });

  renderTimeline();

  const table = await screen.findByRole("table", {
    name: recordTableName,
  });
  const [, recorded, additional] = within(table).getAllByRole("row");
  expect(recorded?.textContent).not.toContain("フィールド");
  expect(additional?.textContent).toContain("フィールド: Run#2");
});

test("表示名が導いた値だけを持つアカウントは、導いた値に導いたことと導き方を添えて出す", async () => {
  const base = timelineResponseJson();
  const [first, second] = base.entries;
  stubFetch({
    ...base,
    entries: [
      first,
      {
        ...second,
        account: {
          ...first?.account,
          label: {
            normalized: "user02",
            derivation: "引用符を外した文字列",
            valueState: "derived",
          },
        },
      },
    ],
  });

  renderTimeline();

  const table = await screen.findByRole("table", {
    name: recordTableName,
  });
  const row = within(table).getAllByRole("row")[2];
  expect(row?.textContent).toContain(
    "user02 Oraculum が作った表示名\n作り方: 引用符を外した文字列",
  );
  expect(row?.textContent).not.toContain("表示名なし");
});

test("原文の文字列を持つ表示名には、導いたことの注記を添えない", async () => {
  stubFetch();

  renderTimeline();

  const table = await screen.findByRole("table", {
    name: recordTableName,
  });
  const first = within(table).getAllByRole("row")[1];
  expect(first?.textContent).toContain("user01");
  expect(first?.textContent).not.toContain("Oraculum が");
});

test("アカウントが現れない行は、値が無いことを出す", async () => {
  stubFetch();

  renderTimeline();

  const table = await screen.findByRole("table", {
    name: recordTableName,
  });
  const second = within(table).getAllByRole("row")[2];
  expect(second?.textContent).toContain("アカウントなし");
});

test("時刻を読み取れなかったレコードを表に混ぜず、件数で出す", async () => {
  stubFetch();

  renderTimeline();

  await waitFor(() => expect(pairText("レコード")).toBe("レコード: 2"));
  expect(pairText("表の外")).toBe("表の外: 3");
});

test("期間の判定から外れたレコードを、表の外の件数に合わせて出す", async () => {
  stubFetch({
    ...timelineResponseJson(),
    undatedRecordCount: 0,
    periodUnjudged: { localRecordCount: 0, undatedRecordCount: 1 },
  });

  renderTimeline();

  await waitFor(() => expect(pairText("表の外")).toBe("表の外: 1"));
  expect(pairText("時刻なし")).toBe("時刻なし: 1 件");
});

/** 収集元の記録期間の表の行のうち、一致したレコードの件数の列の値。 */
function matchedCountOf(row: HTMLElement | undefined): string | undefined {
  return row === undefined
    ? undefined
    : (within(row).getAllByRole("cell")[2]?.textContent ?? undefined);
}

test("記録が無い収集元と、記録はありイベントが無い収集元を別のラベルで出す", async () => {
  stubFetch();

  renderTimeline();

  const table = await screen.findByRole("table", {
    name: coverageTableName,
  });
  const rows = within(table).getAllByRole("row").slice(1);
  const outside = rows.find((row) => row.textContent?.includes("access.log"));
  const unknown = rows.find((row) => row.textContent?.includes("audit.log"));
  expect(outside?.textContent).toContain("記録期間外");
  expect(unknown?.textContent).toContain("記録期間不明");
  // 記録が無い 2 つの状態を、同じ表示で書かない。
  expect(outside?.textContent).not.toBe(unknown?.textContent);
  // 件数を状態で伏せない。記録期間の母集団は解析に失敗したレコードを含み、件数の
  // 母集団は取り込みに成功したレコードだけであるため、2 つは食い違いうる。
  for (const row of [outside, unknown]) {
    expect(matchedCountOf(row)).toBe("0");
  }
});

test("0 件の応答でも、収集元ごとの記録期間と用いた条件を出す", async () => {
  stubFetch(emptyTimelineResponseJson());

  renderTimeline({ eventCategory: "no_such_category" });

  const table = await screen.findByRole("table", {
    name: coverageTableName,
  });
  const rows = within(table).getAllByRole("row").slice(1);
  expect(rows).toHaveLength(2);
  // 記録期間が期間を覆う収集元は「記録はあり、イベントが無い」と読める。
  const covered = rows.find((row) => row.textContent?.includes("host-a.log"));
  expect(covered?.textContent).toContain("記録期間内");
  expect(matchedCountOf(covered)).toBe("0");
  // 行が 0 件でも、表そのものは出さない。
  expect(screen.queryByRole("table", { name: recordTableName })).toBeNull();
});

test("期間を指定していないとき、指定していないことを出す", async () => {
  stubFetch();

  renderTimeline();

  await waitFor(() => expect(pairText("期間")).toBe("期間: 指定なし"));
});

test("取得に失敗したときに、失敗の要約を出す", async () => {
  stubFetch(apiErrorJson("internal_error"), 500);

  renderTimeline();

  await waitFor(() =>
    expect(screen.getByRole("alert").textContent).toContain("Timeline の取得"),
  );
});

test("イベントの種類を変えると、その条件で取り直す", async () => {
  const mock = stubFetch();

  const view = renderTimeline();
  await screen.findByRole("table", { name: coverageTableName });

  view.rerender(
    <Timeline
      matchConditions={matchConditions}
      timeFilter={undefined}
      eventCategory="file"
      eventAction="create"
      caseId={undefined}
      terminal={undefined}
      onSelectRecord={() => {}}
      dataVersion={0}
    />,
  );

  await waitFor(() =>
    expect(mock.mock.calls.at(-1)?.[0]).toBe(
      `/api/v0/timeline?eventCategory=file&eventAction=create&${matchConditionQuery}`,
    ),
  );
});

test("端末を選ぶと要求に載せる", async () => {
  const mock = stubFetch();

  renderTimeline({ terminal: "n:terminal:1f0c" });

  await screen.findByRole("table", { name: coverageTableName });
  expect(mock.mock.calls.at(-1)?.[0]).toBe(
    `/api/v0/timeline?terminal=n%3Aterminal%3A1f0c&${matchConditionQuery}`,
  );
});

test("案件を選ぶと要求に載せ、応答に用いられた案件を出す", async () => {
  const mock = stubFetch({ ...timelineResponseJson(), case: challengeCaseId });

  renderTimeline({ caseId: challengeCaseId });

  await waitFor(() =>
    expect(pairText("案件")).toBe(`案件: ${challengeCaseId}`),
  );
  expect(mock.mock.calls.at(-1)?.[0]).toBe(
    `/api/v0/timeline?case=${challengeCaseId}&${matchConditionQuery}`,
  );
});

test("検索式を要求の searchExpression に文字列のまま載せ、応答に用いられた検索式を出す", async () => {
  const expression = 'user contains "a b" or LogonType != 3';
  const mock = stubFetch({
    ...timelineResponseJson(),
    searchExpression: expression,
  });

  renderTimeline({ searchExpression: expression });

  await waitFor(() => expect(pairText("検索式")).toBe(`検索式: ${expression}`));
  const url = new URL(String(mock.mock.calls.at(-1)?.[0]), "http://localhost");
  expect(url.searchParams.getAll("searchExpression")).toEqual([expression]);
});

test("文字列条件を時系列の要求に複数回の項目として載せる", async () => {
  const mock = stubFetch();
  renderTimeline({
    valueContains: ["alpha", "beta"],
    valueExcludes: ["gamma"],
    valueField: "CommandLine",
    fieldContains: ["Image=tool.exe", "Account=name"],
    fieldEquals: ["EventId=42"],
  });

  await screen.findByRole("table", { name: coverageTableName });
  const url = new URL(String(mock.mock.calls.at(-1)?.[0]), "http://localhost");
  expect(url.searchParams.getAll("valueContains")).toEqual(["alpha", "beta"]);
  expect(url.searchParams.getAll("valueExcludes")).toEqual(["gamma"]);
  expect(url.searchParams.getAll("valueField")).toEqual(["CommandLine"]);
  expect(url.searchParams.getAll("fieldContains")).toEqual([
    "Image=tool.exe",
    "Account=name",
  ]);
  expect(url.searchParams.getAll("fieldEquals")).toEqual(["EventId=42"]);
});

test("文字列条件が無いとき欄の指定だけを時系列の要求に載せない", async () => {
  const mock = stubFetch();
  renderTimeline({ valueField: "CommandLine" });

  await screen.findByRole("table", { name: coverageTableName });
  const url = new URL(String(mock.mock.calls.at(-1)?.[0]), "http://localhost");
  expect(url.searchParams.has("valueField")).toBe(false);
});

test("空白だけの検索式を要求に載せない", async () => {
  const mock = stubFetch();

  renderTimeline({ searchExpression: "  " });

  await screen.findByRole("table", { name: coverageTableName });
  expect(mock.mock.calls.at(-1)?.[0]).toBe(
    `/api/v0/timeline?${matchConditionQuery}`,
  );
  expect(pairText("検索式")).toBeUndefined();
});

test("時系列の取得が検索式の誤りを返すと、読めなかった理由を出す", async () => {
  stubFetch(
    searchExpressionErrorResponseJson({
      reason: "missing_operand",
      offset: 0,
      length: 3,
    }),
    400,
  );

  renderTimeline({ searchExpression: "and LogonType == 3" });

  // 解析できなかった理由を失敗の種類のラベルに出す。
  expect(await screen.findByText("and・or・not の条件なし")).toBeTruthy();
});

function chooseScale(label: string) {
  const select = screen.getByRole("combobox", { name: "区切りの幅" });
  const option = within(select).getByRole("option", { name: label });
  fireEvent.change(select, {
    target: { value: option.getAttribute("value") },
  });
}

function selectedScaleLabel(): string | undefined {
  const select = screen.getByRole<HTMLSelectElement>("combobox", {
    name: "区切りの幅",
  });
  return select.selectedOptions[0]?.textContent ?? undefined;
}

function dataRowsOf(table: HTMLElement): HTMLElement[] {
  return within(table).getAllByRole("row").slice(1);
}

/** 区切りの行の件数の欄の値を読む。 */
function countOf(row: HTMLElement): number {
  const cell = within(row).getAllByRole("cell")[1];
  return Number(cell?.textContent?.replace(/[^0-9]/g, ""));
}

test("区切りの段階を選ぶと、区切りごとの件数と収集元ごとの内訳を出し、件数の和が全件数と等しい", async () => {
  const specs = evenlySpacedEntrySpecs(12, "2031-10-08T01:19:30Z", 10_000, [
    "hostA",
    "hostACopy",
    "access",
  ]);
  stubFetch(generatedTimelineResponseJson(specs));
  renderTimeline();
  await screen.findByRole("table", { name: recordTableName });

  chooseScale("1 分");

  const table = screen.getByRole("table", { name: groupTableName });
  const rows = dataRowsOf(table);
  expect(rows.map(countOf).reduce((sum, count) => sum + count, 0)).toBe(
    specs.length,
  );
  expect(pairText("レコード")).toBe("レコード: 12");
  expect(pairText("行")).toBe(`行: ${rows.length}`);
  expect(rows[0]?.textContent).toContain(
    "2031-10-08 01:19:00 – 2031-10-08 01:20:00",
  );
  // file 名が同じ 2 つの収集元を、内容の識別の先頭で分けて数える。
  const breakdown = within(rows[1] ?? table).getAllByRole("listitem");
  for (const source of [generatedSources.hostA, generatedSources.hostACopy]) {
    expect(
      breakdown.some((item) =>
        item.textContent?.includes(source.contentSha256.slice(0, 12)),
      ),
    ).toBe(true);
  }
});

test("区切りを広げる・狭めるボタンで段階を 1 つずつ変え、両端の段階では先へ進むボタンを押せない", async () => {
  stubFetch();
  renderTimeline();
  await screen.findByRole("table", { name: recordTableName });

  expect(selectedScaleLabel()).toBe("1 件ずつ");
  const zoomIn = screen.getByRole<HTMLButtonElement>("button", {
    name: "区切りを狭める",
  });
  const zoomOut = screen.getByRole<HTMLButtonElement>("button", {
    name: "区切りを広げる",
  });
  expect(zoomIn).toBeDisabled();
  expect(zoomOut).toBeEnabled();

  fireEvent.click(zoomOut);
  expect(selectedScaleLabel()).toBe("1 秒");
  expect(zoomIn).toBeEnabled();

  chooseScale("1 日");
  expect(zoomOut).toBeDisabled();
  fireEvent.click(zoomIn);
  expect(selectedScaleLabel()).toBe("1 時間");
});

test("時刻の精度が示す範囲が区切りをまたぐレコードを、精度と一緒に区切りと別の行に出す", async () => {
  stubFetch(
    generatedTimelineResponseJson([
      { normalized: "2031-10-08T01:20:05.000Z" },
      { normalized: "2031-10-08T01:20:00Z", precision: "minute" },
      { normalized: "2031-10-08T01:20:15.000Z" },
    ]),
  );
  renderTimeline();
  await screen.findByRole("table", { name: recordTableName });

  chooseScale("10 秒");
  const separate = dataRowsOf(
    screen.getByRole("table", { name: groupTableName }),
  ).filter((row) => row.textContent?.includes("精度が区切りより粗い"));
  expect(separate).toHaveLength(1);
  expect(separate[0]?.querySelector('[title="精度: 分"]')).not.toBeNull();
  expect(separate[0]?.textContent).toContain(
    "2031-10-08 01:20:00 – 2031-10-08 01:21:00",
  );
  expect(pairText("精度が区切りより粗い")).toBe("精度が区切りより粗い: 1");

  // 区切りが精度の範囲を覆う段階では、同じレコードを区切りへ入れる。
  chooseScale("1 分");
  const rows = dataRowsOf(screen.getByRole("table", { name: groupTableName }));
  expect(
    rows.some((row) => row.textContent?.includes("精度が区切りより粗い")),
  ).toBe(false);
  expect(rows.map(countOf)).toEqual([3]);
});

test("UTC からのずれを確認できないレコードを、区切りと別の行に出す", async () => {
  stubFetch(
    generatedTimelineResponseJson([
      { normalized: "2031-10-08T01:20:05.000Z" },
      {
        normalized: "2031-10-08T10:20:06.000",
        offsetState: "item_absent",
      },
    ]),
  );
  renderTimeline();
  await screen.findByRole("table", { name: recordTableName });

  chooseScale("1 分");

  const rows = dataRowsOf(screen.getByRole("table", { name: groupTableName }));
  const separate = rows.filter((row) =>
    row.textContent?.includes("タイムゾーン不明"),
  );
  expect(separate.map(countOf)).toEqual([1]);
  expect(rows.map(countOf).reduce((sum, count) => sum + count, 0)).toBe(2);
});

test("1 件ずつの行は、タイムゾーン不明の時刻と分析者の記録で読んだ時刻を区別して出す", async () => {
  stubFetch(
    generatedTimelineResponseJson([
      {
        normalized: "2031-10-08T10:20:05.000",
        offsetState: "item_absent",
        interpretationOffset: "+09:00",
      },
      { normalized: "2031-10-08T01:20:06.000Z" },
      { normalized: "2031-10-08T10:20:07.000", offsetState: "undetermined" },
    ]),
  );
  renderTimeline();
  const table = await screen.findByRole("table", { name: recordTableName });

  const rows = dataRowsOf(table).map((row) => row.textContent ?? "");
  expect(rows[0]).toContain("分析者の記録: UTC+09:00");
  expect(rows[0]).not.toContain("タイムゾーン不明");
  expect(rows[1]).not.toContain("タイムゾーン不明");
  expect(rows[1]).not.toContain("分析者の記録");
  expect(rows[2]).toContain("タイムゾーン不明");

  chooseScale("1 分");
  const groups = dataRowsOf(
    screen.getByRole("table", { name: groupTableName }),
  );
  // タイムゾーンを与えた時刻は UTC の時刻と同じ区切りに入り、不明の時刻だけが別の行に出る。
  expect(groups.map(countOf)).toEqual([2, 1]);
  expect(groups[1]?.textContent).toContain("タイムゾーン不明");
});

/** レコードの列が ID を出している 1 件ずつの行が、表に描かれているか。 */
function hasRecordRow(table: HTMLElement, sequenceNumber: number): boolean {
  const pattern = new RegExp(`ID: ${sequenceNumber}(?!\\d)`);
  return within(table)
    .getAllByRole("cell")
    .some((cell) => pattern.test(cell.textContent ?? ""));
}

function scrollRegionTo(scrollTop: number) {
  const region = screen.getByRole("region", { name: "時系列の表" });
  region.scrollTop = scrollTop;
  fireEvent.scroll(region);
}

test("見えている行とその前後だけを描き、スクロールに合わせて行を入れ替える", async () => {
  const specs = evenlySpacedEntrySpecs(400, "2031-10-08T04:00:00Z", 1_000);
  stubFetch(generatedTimelineResponseJson(specs));
  renderTimeline();
  await screen.findByRole("table", { name: groupTableName });
  // 行の多い応答では、既定の段階は区切りの段階である。
  expect(selectedScaleLabel()).not.toBe("1 件ずつ");

  chooseScale("1 件ずつ");
  const table = screen.getByRole("table", { name: recordTableName });
  // 生成した行の通番は 1000 から振る。
  const first = 1000;
  const last = first + specs.length - 1;
  expect(Number(table.getAttribute("aria-rowcount")) - 1).toBe(specs.length);
  expect(dataRowsOf(table).length).toBeLessThan(specs.length);
  expect(hasRecordRow(table, first)).toBe(true);
  expect(hasRecordRow(table, last)).toBe(false);

  scrollRegionTo(Number.MAX_SAFE_INTEGER);

  await waitFor(() => expect(hasRecordRow(table, last)).toBe(true));
  expect(hasRecordRow(table, first)).toBe(false);
  expect(dataRowsOf(table).length).toBeLessThan(specs.length);
});

test("区切りの行の拡大のボタンで、1 つ細かい段階に替え、その区切りの行を描く", async () => {
  const specs = evenlySpacedEntrySpecs(400, "2031-10-08T04:00:00Z", 10_000);
  stubFetch(generatedTimelineResponseJson(specs));
  renderTimeline();
  await screen.findByRole("table", { name: groupTableName });
  chooseScale("1 分");
  scrollRegionTo(Number.MAX_SAFE_INTEGER);
  const lastMinute = "2031-10-08 05:06:00 – 2031-10-08 05:07:00";
  const lastMinuteRow = (await screen.findByText(lastMinute)).closest("tr");
  if (lastMinuteRow === null) {
    throw new Error("the last minute row is missing");
  }

  fireEvent.click(
    within(lastMinuteRow).getByRole("button", {
      name: "10 秒ごとに表示",
    }),
  );

  expect(selectedScaleLabel()).toBe("10 秒");
  const table = screen.getByRole("table", { name: groupTableName });
  expect(
    within(table).queryByText("2031-10-08 05:06:00 – 2031-10-08 05:06:10"),
  ).not.toBeNull();
  expect(
    within(table).queryByText("2031-10-08 04:00:00 – 2031-10-08 04:00:10"),
  ).toBeNull();
});

test("条件を変えて取り直しても、分析者が選んだ段階を保つ", async () => {
  stubFetch();
  const view = renderTimeline();
  await screen.findByRole("table", { name: recordTableName });
  chooseScale("1 分");

  view.rerender(
    <Timeline
      matchConditions={matchConditions}
      timeFilter={undefined}
      eventCategory="net"
      eventAction={undefined}
      caseId={undefined}
      terminal={undefined}
      onSelectRecord={() => {}}
      dataVersion={0}
    />,
  );

  await screen.findByRole("table", { name: groupTableName });
  expect(selectedScaleLabel()).toBe("1 分");
});

test("案件を選んでいないときは要求に載せず、案件の組を出さない", async () => {
  const mock = stubFetch();

  renderTimeline();

  await screen.findByRole("table", { name: coverageTableName });
  expect(mock.mock.calls.at(-1)?.[0]).toBe(
    `/api/v0/timeline?${matchConditionQuery}`,
  );
  expect(pairText("案件")).toBeUndefined();
});
