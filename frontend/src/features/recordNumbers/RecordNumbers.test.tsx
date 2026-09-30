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
  decodeRecordLocator,
  type RecordLocator,
} from "@/shared/contracts/common";
import type { SourceIdentity } from "@/shared/contracts/sources";
import { jsonResponse } from "@/testdata/http";
import { RecordNumbers } from "./RecordNumbers";

function source(sourceId: string, fileName: string): SourceIdentity {
  return {
    sourceId,
    contentSha256: sourceId.slice(0, 1).repeat(64),
    originPath: `/data/example/${fileName}`,
    fileName,
    sizeBytes: 1000,
    newlineCount: 10,
    endsWithNewline: true,
    lineEnding: "lf",
    formatKey: "windows_event_xml",
  };
}

const events = source("a-events", "events.xml");
const viewer = source("b-viewer", "viewer.csv");

function locator(of: SourceIdentity, byteOffset: number) {
  return {
    sourceId: of.sourceId,
    sourceContentSha256: of.contentSha256,
    sourceFileName: of.fileName,
    positionKind: "byte_range",
    byteOffset,
    byteLength: 10,
    lineNumber: byteOffset + 1,
    recordRawTextRef: `raw:${of.sourceId}:${byteOffset}`,
  };
}

function stream(overrides: Record<string, unknown> = {}) {
  return {
    channel: "Application",
    computers: [{ computer: "host-a.example.test", recordCount: 3 }],
    recordCount: 3,
    lowestNumber: 11,
    highestNumber: 15,
    missingNumberCount: 0,
    duplicatedRecordCount: 0,
    gapCount: 0,
    gaps: [],
    ...overrides,
  };
}

const gapNumbers = {
  sourceId: events.sourceId,
  examination: "gaps_found",
  unreadableRecordCount: 0,
  listLimit: 200,
  streams: [
    stream({
      missingNumberCount: 2,
      gapCount: 1,
      gaps: [
        {
          firstMissingNumber: 13,
          lastMissingNumber: 14,
          precedingRecordRef: locator(events, 100),
          followingRecordRef: locator(events, 300),
          failureRecordCount: 1,
          failureRecordRefs: [locator(events, 200)],
        },
      ],
    }),
  ],
  fileHeader: {
    nextRecordNumber: 12,
    highestRecordNumber: 15,
    unreadableRecordHeaderCount: 0,
    comparison: "differs",
    dirty: "true",
  },
};

function noGapNumbers(overrides: Record<string, unknown>) {
  return {
    ...gapNumbers,
    examination: "no_gaps",
    streams: [stream(overrides)],
    fileHeader: undefined,
  };
}

const viewerNumbers = {
  sourceId: viewer.sourceId,
  examination: "not_examined",
  notExaminedReason: "record_numbers_not_declared",
  streams: [],
  listLimit: 200,
};

function comparisonOf(overrides: Record<string, unknown>) {
  return {
    comparedSourceId: events.sourceId,
    state: "compared",
    key: "second_provider_event_id",
    oneToOneKeyCount: 5,
    onlyInSourceRecordCount: 0,
    onlyInComparedRecordCount: 0,
    onlyInSourceRecordRefs: [],
    onlyInComparedRecordRefs: [],
    onlyInSourceOutsideComparedRangeRecordCount: 0,
    onlyInSourceInsideComparedRangeRecordCount: 0,
    onlyInSourceInsideComparedRangeRecordRefs: [],
    onlyInComparedOutsideSourceRangeRecordCount: 0,
    onlyInComparedInsideSourceRangeRecordCount: 0,
    onlyInComparedInsideSourceRangeRecordRefs: [],
    listLimit: 200,
    undeterminedKeyCount: 2,
    undeterminedSourceRecordCount: 3,
    undeterminedComparedRecordCount: 4,
    unequalKeyCount: 1,
    sourceSurplusRecordCount: 0,
    comparedSurplusRecordCount: 1,
    unequalKeys: [],
    sourceUnkeyedRecordCount: 0,
    comparedUnkeyedRecordCount: 0,
    ...overrides,
  };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

/** 要求の順に responses を返す。最後の応答は以降の要求にも返す。 */
function stubFetch(...responses: Array<() => Response>) {
  const mock = vi.fn(async (_input: string) => {
    const next = responses.length > 1 ? responses.shift() : responses[0];
    if (next === undefined) {
      throw new Error("no response is stubbed");
    }
    return next();
  });
  vi.stubGlobal("fetch", mock);
  return mock;
}

const ok = (body: unknown) => () => jsonResponse(200, body);

function requestedUrl(fetch: ReturnType<typeof stubFetch>, call: number): URL {
  return new URL(String(fetch.mock.calls[call]?.[0]), "http://localhost");
}

/** 上位の画面と同じく、比べる相手の選択を部品の外で持つ。 */
function NumbersUnderTest(props: {
  selectedSource: SourceIdentity | undefined;
  dataVersion: number;
  onSelectRecord: (recordRef: RecordLocator) => void;
}) {
  const [comparedSourceId, setComparedSourceId] = useState("");
  return (
    <RecordNumbers
      {...props}
      sources={[events, viewer]}
      comparedSourceId={comparedSourceId}
      onSelectComparedSource={setComparedSourceId}
    />
  );
}

function renderNumbers(
  selectedSource: SourceIdentity | undefined,
  dataVersion = 0,
  onSelectRecord = vi.fn(),
) {
  const view = render(
    <NumbersUnderTest
      selectedSource={selectedSource}
      dataVersion={dataVersion}
      onSelectRecord={onSelectRecord}
    />,
  );
  return { onSelectRecord, view };
}

/** name の「名前: 値」の組の文字列。最初に見つかった組を返す。 */
function pairText(name: string): string | undefined {
  return Array.from(document.querySelectorAll("li"))
    .map((item) => item.textContent ?? "")
    .find((text) => text.startsWith(`${name}: `));
}

/** 番号の抜けの組が label になるまで待つ。 */
async function examinedAs(label: string) {
  await waitFor(() =>
    expect(pairText("番号の抜け")).toMatch(new RegExp(`^番号の抜け: ${label}`)),
  );
}

test("収集元を選ぶまで取得しない", () => {
  const fetch = stubFetch(ok({ recordNumbers: gapNumbers }));
  renderNumbers(undefined);
  expect(pairText("収集元")).toBe("収集元: 未選択");
  fireEvent.click(
    screen.getByRole("button", { name: "レコードの番号 の説明" }),
  );
  expect(screen.getByRole("tooltip").textContent).toBe(
    "収集元の一覧で選択した収集元の番号の抜け",
  );
  expect(fetch).not.toHaveBeenCalled();
});

test("収集元を選んだ直後の取得中は、未選択と出さずに確認中を出す", async () => {
  stubFetch(ok({ recordNumbers: gapNumbers }));
  renderNumbers(events);
  expect(screen.getByText("レコードの番号の確認中")).toBeDefined();
  expect(pairText("収集元")).toBeUndefined();
  await screen.findByRole("list", { name: "抜けた番号の範囲" });
});

test("抜けた範囲と失敗の記録、見出しの番号の食い違いを出し、抜けの前後のレコードを開ける", async () => {
  const fetch = stubFetch(ok({ recordNumbers: gapNumbers }));
  const { onSelectRecord } = renderNumbers(events);
  const ranges = await screen.findByRole("list", { name: "抜けた番号の範囲" });
  const item = within(ranges).getAllByRole("listitem")[0] as HTMLElement;
  expect(item.querySelector("span")?.textContent).toBe("13-14");
  expect(pairText("取り込みの失敗")).toBe("取り込みの失敗: 1");
  expect(pairText("失敗の位置")).toBe("失敗の位置: 行: 201、位置: 200-210");
  expect(pairText("ファイルの見出しの次のレコード番号")).toBe(
    "ファイルの見出しの次のレコード番号: 12",
  );
  expect(pairText("レコードの見出しの番号の最大")).toBe(
    "レコードの見出しの番号の最大: 15",
  );
  expect(pairText("2 つの番号")).toBe("2 つの番号: 不一致");
  expect(pairText("dirty の印")).toBe("dirty の印: あり");
  fireEvent.click(
    within(ranges).getByRole("button", { name: "直後のレコードを表示" }),
  );
  expect(onSelectRecord).toHaveBeenCalledWith(
    decodeRecordLocator(locator(events, 300), "expected"),
  );
  const url = requestedUrl(fetch, 0);
  expect(url.pathname).toBe("/api/v0/record-numbers");
  expect(url.searchParams.get("sourceId")).toBe(events.sourceId);
  expect(url.searchParams.get("sourceContentSha256")).toBe(
    events.contentSha256,
  );
  expect(url.searchParams.has("comparedSourceId")).toBe(false);
});

test("見出しの番号を読めないとき、dirty の印が無いときの見出しの行を出す", async () => {
  stubFetch(
    ok({
      recordNumbers: {
        ...noGapNumbers({}),
        fileHeader: {
          unreadableRecordHeaderCount: 2,
          comparison: "header_unreadable",
          dirty: "false",
        },
      },
    }),
  );
  renderNumbers(events);
  await waitFor(() =>
    expect(pairText("ファイルの見出しの次のレコード番号")).toBe(
      "ファイルの見出しの次のレコード番号: 読み取り不可",
    ),
  );
  expect(pairText("レコードの見出しの番号の最大")).toBe(
    "レコードの見出しの番号の最大: なし",
  );
  expect(pairText("2 つの番号")).toMatch(/^2 つの番号: 未比較/);
  expect(pairText("2 つの番号")).toContain(
    "ファイルの見出しの次のレコード番号の読み取り不可",
  );
  expect(pairText("レコードの見出しの番号を読み取れなかったレコード")).toBe(
    "レコードの見出しの番号を読み取れなかったレコード: 2",
  );
  expect(pairText("dirty の印")).toBe("dirty の印: なし");
});

test("抜けが無い収集元と、確認しなかった収集元を分けて出す", async () => {
  stubFetch(ok({ recordNumbers: viewerNumbers }));
  renderNumbers(viewer);
  await examinedAs("未確認");
  expect(pairText("理由")).toBe("理由: 入力形式が EventRecordID の確認対象外");
  cleanup();
  stubFetch(ok({ recordNumbers: noGapNumbers({}) }));
  renderNumbers(events);
  await examinedAs("なし");
  expect(screen.queryByText("複数の端末の混在の可能性")).toBeNull();
});

test("名前のある端末が 2 種類以上あり番号が重複する単位に、端末ごとの件数と混ざっている可能性を出す", async () => {
  stubFetch(
    ok({
      recordNumbers: noGapNumbers({
        computers: [
          { computer: "host-a.example.test", recordCount: 3 },
          { computer: "host-b.example.test", recordCount: 2 },
        ],
        duplicatedRecordCount: 2,
      }),
    }),
  );
  renderNumbers(events);
  const computers = await screen.findByRole("list", {
    name: "端末ごとの件数",
  });
  expect(
    within(computers)
      .getAllByRole("listitem")
      .map((item) => item.textContent),
  ).toEqual(["host-a.example.test: 3", "host-b.example.test: 2"]);
  expect(screen.getByText("複数の端末の混在の可能性")).toBeInTheDocument();
  expect(pairText("Computer")).toBe("Computer: 2 種類以上");
});

test("端末の欄の無いレコードと名前のある端末 1 つの組では、混ざっている可能性を出さない", async () => {
  stubFetch(
    ok({
      recordNumbers: noGapNumbers({
        computers: [
          { recordCount: 1 },
          { computer: "host-a.example.test", recordCount: 3 },
        ],
        duplicatedRecordCount: 1,
      }),
    }),
  );
  renderNumbers(events);
  await screen.findByRole("list", { name: "端末ごとの件数" });
  expect(screen.queryByText("複数の端末の混在の可能性")).toBeNull();
});

/** 突き合わせの表の、項目の行の列の値。列は鍵・収集元・相手の順である。 */
function comparisonRow(label: string): string[] {
  const table = screen.getByRole("table", { name: "突き合わせの件数" });
  const row = within(table)
    .getByRole("rowheader", { name: label })
    .closest("tr");
  if (row === null) throw new Error(`row ${label} is missing`);
  return within(row)
    .getAllByRole("cell")
    .map((cell) => cell.textContent ?? "");
}

test("比べる収集元を選ぶと、片方にしか無いレコードと鍵で決められない件数を出す", async () => {
  const comparison = comparisonOf({
    onlyInComparedRecordCount: 1,
    onlyInComparedRecordRefs: [locator(events, 400)],
  });
  const fetch = stubFetch(
    ok({ recordNumbers: viewerNumbers }),
    ok({ recordNumbers: viewerNumbers, comparison }),
  );
  const { onSelectRecord } = renderNumbers(viewer);
  await examinedAs("未確認");
  fireEvent.change(screen.getByLabelText("比べる収集元"), {
    target: { value: events.sourceId },
  });
  const onlyInEvents = await screen.findByRole("list", {
    name: "相手だけにある鍵のレコード",
  });
  expect(pairText("収集元")).toBe("収集元: viewer.csv");
  expect(pairText("相手")).toBe("相手: events.xml");
  expect(comparisonRow("1 対 1 に決まらない")).toEqual(["2", "3", "4"]);
  expect(comparisonRow("件数の違う鍵の多い分")).toEqual(["1", "0", "1"]);
  fireEvent.click(within(onlyInEvents).getByRole("button"));
  expect(onSelectRecord).toHaveBeenCalledWith(
    decodeRecordLocator(locator(events, 400), "expected"),
  );
  const url = requestedUrl(fetch, 1);
  expect(url.searchParams.get("comparedSourceId")).toBe(events.sourceId);
  expect(url.searchParams.get("comparedSourceContentSha256")).toBe(
    events.contentSha256,
  );
});

test("件数の違う鍵について、欄の値で特定した片方にだけあるレコードを開ける", async () => {
  const comparison = comparisonOf({
    unequalKeys: [
      {
        outcome: "identified",
        comparedSemantics: ["connection.source_port"],
        sourceRecordRefs: [locator(viewer, 10), locator(viewer, 20)],
        comparedRecordRefs: [
          locator(events, 100),
          locator(events, 200),
          locator(events, 300),
        ],
        onlyInSourceRecordRefs: [],
        onlyInComparedRecordRefs: [locator(events, 300)],
      },
      {
        outcome: "no_compared_values",
        comparedSemantics: [],
        sourceRecordRefs: [locator(viewer, 30)],
        comparedRecordRefs: [locator(events, 400), locator(events, 500)],
        onlyInSourceRecordRefs: [],
        onlyInComparedRecordRefs: [],
      },
    ],
  });
  stubFetch(
    ok({ recordNumbers: viewerNumbers }),
    ok({ recordNumbers: viewerNumbers, comparison }),
  );
  const { onSelectRecord } = renderNumbers(viewer);
  await examinedAs("未確認");
  fireEvent.change(screen.getByLabelText("比べる収集元"), {
    target: { value: events.sourceId },
  });
  const identified = await screen.findByRole("list", {
    name: "相手だけの対にならないレコード",
  });
  const [first] = within(
    screen.getByRole("region", {
      name: "件数の違う鍵のフィールドの値による比較",
    }),
  ).getAllByRole("listitem");
  expect(first?.textContent).toContain("収集元: 2");
  expect(first?.textContent).toContain("相手: 3");
  expect(first?.textContent).toContain("結果: 特定");
  fireEvent.click(within(identified).getByRole("button"));
  expect(onSelectRecord).toHaveBeenCalledWith(
    decodeRecordLocator(locator(events, 300), "expected"),
  );
  // 値で比べられない鍵は、鍵を持つ全レコードを候補に出す。
  const candidates = screen.getByRole("list", {
    name: "候補: 相手の鍵を持つレコード",
  });
  expect(within(candidates).getAllByRole("button")).toHaveLength(2);
});

test("片方にしか無いレコードが上限を超えるとき、全件数と先頭の 200 件を出す", async () => {
  const refs = Array.from({ length: 200 }, (_, index) =>
    locator(events, index * 10),
  );
  stubFetch(
    ok({ recordNumbers: viewerNumbers }),
    ok({
      recordNumbers: viewerNumbers,
      comparison: comparisonOf({
        onlyInComparedRecordCount: 201,
        onlyInComparedRecordRefs: refs,
      }),
    }),
  );
  renderNumbers(viewer);
  await examinedAs("未確認");
  fireEvent.change(screen.getByLabelText("比べる収集元"), {
    target: { value: events.sourceId },
  });
  const list = await screen.findByRole("list", {
    name: "相手だけにある鍵のレコード",
  });
  expect(within(list).getAllByRole("button")).toHaveLength(200);
  expect(
    screen.getByRole("heading", {
      level: 4,
      name: "相手だけにある鍵のレコード",
    }),
  ).toBeTruthy();
  expect(pairText("件数")).toBe("件数: 201");
  expect(pairText("表示")).toBe("表示: 200 / 201");
});

test("片方にしか無いレコードを相手の記録期間の外と内に分け、範囲の内のレコードを開ける", async () => {
  stubFetch(
    ok({ recordNumbers: viewerNumbers }),
    ok({
      recordNumbers: viewerNumbers,
      comparison: comparisonOf({
        onlyInComparedRecordCount: 4,
        onlyInComparedRecordRefs: [
          locator(events, 100),
          locator(events, 200),
          locator(events, 300),
          locator(events, 400),
        ],
        onlyInComparedOutsideSourceRangeRecordCount: 2,
        onlyInComparedInsideSourceRangeRecordCount: 1,
        onlyInComparedInsideSourceRangeRecordRefs: [locator(events, 300)],
      }),
    }),
  );
  const { onSelectRecord } = renderNumbers(viewer);
  await examinedAs("未確認");
  fireEvent.change(screen.getByLabelText("比べる収集元"), {
    target: { value: events.sourceId },
  });
  await screen.findByRole("table", { name: "突き合わせの件数" });
  expect(comparisonRow("片方だけ・相手の記録期間の外")[2]).toBe("2");
  expect(comparisonRow("片方だけ・相手の記録期間の内")[2]).toBe("1");
  expect(comparisonRow("片方だけ・記録期間の決定不可")[2]).toBe("1");
  fireEvent.click(
    screen.getByRole("button", { name: "相手の記録期間 の説明" }),
  );
  expect(screen.getByRole("tooltip").textContent).toContain(
    "範囲: 相手の鍵を持つレコードのプロバイダごとの最初の秒-最後の秒",
  );
  const inside = screen.getByRole("list", {
    name: "相手だけにあり収集元の記録期間の内のレコード",
  });
  fireEvent.click(within(inside).getByRole("button"));
  expect(onSelectRecord).toHaveBeenCalledWith(
    decodeRecordLocator(locator(events, 300), "expected"),
  );
});

test("範囲の外と内の件数の和が片方にしか無い件数を超える応答を退ける", async () => {
  stubFetch(
    ok({ recordNumbers: viewerNumbers }),
    ok({
      recordNumbers: viewerNumbers,
      comparison: comparisonOf({
        onlyInSourceRecordCount: 1,
        onlyInSourceRecordRefs: [locator(viewer, 10)],
        onlyInSourceOutsideComparedRangeRecordCount: 1,
        onlyInSourceInsideComparedRangeRecordCount: 1,
      }),
    }),
  );
  renderNumbers(viewer);
  await examinedAs("未確認");
  fireEvent.change(screen.getByLabelText("比べる収集元"), {
    target: { value: events.sourceId },
  });
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(screen.queryByRole("table", { name: "突き合わせの件数" })).toBeNull();
});

test("抜けの範囲と失敗の記録が上限を超えるとき、全件数と先頭の 200 件を出す", async () => {
  const failures = Array.from({ length: 200 }, (_, index) =>
    locator(events, 1000 + index * 10),
  );
  const gaps = Array.from({ length: 200 }, (_, index) => ({
    firstMissingNumber: 10 + index * 2,
    lastMissingNumber: 10 + index * 2,
    precedingRecordRef: locator(events, index * 20),
    followingRecordRef: locator(events, index * 20 + 10),
    failureRecordCount: index === 0 ? 201 : 0,
    failureRecordRefs: index === 0 ? failures : [],
  }));
  stubFetch(
    ok({
      recordNumbers: {
        ...gapNumbers,
        streams: [stream({ missingNumberCount: 201, gapCount: 201, gaps })],
      },
    }),
  );
  renderNumbers(events);
  const ranges = await screen.findByRole("list", { name: "抜けた番号の範囲" });
  // 抜けの範囲 1 件は、失敗の記録の組を入れ子の一覧に持つ。範囲の行は直下の listitem である。
  expect(ranges.querySelectorAll(":scope > li")).toHaveLength(200);
  expect(pairText("抜けた番号")).toBe("抜けた番号: 201");
  expect(pairText("範囲")).toBe("範囲: 201");
  const pairsOfGaps = Array.from(document.querySelectorAll("li"))
    .map((item) => item.textContent ?? "")
    .filter((text) => text.startsWith("表示: "));
  // 抜けの範囲の件数と、最初の抜けの失敗の件数の 2 つが上限を超える。
  expect(pairsOfGaps).toEqual(["表示: 200 / 201", "表示: 200 / 201"]);
  expect(pairText("取り込みの失敗")).toBe("取り込みの失敗: 201");
  // 位置の並びは 1 件が「位置: 始まり-終わり」を 1 つ持つ。
  expect(pairText("失敗の位置")?.match(/位置: \d+-\d+/g)).toHaveLength(200);
});

test("UTC 時刻の定まらない収集元とは未比較と出し、理由と次の操作を持つ", async () => {
  stubFetch(
    ok({ recordNumbers: gapNumbers }),
    ok({
      recordNumbers: gapNumbers,
      comparison: comparisonOf({
        comparedSourceId: viewer.sourceId,
        state: "not_compared",
        notComparedReason: "time_offset_undetermined",
      }),
    }),
  );
  renderNumbers(events);
  await screen.findByRole("list", { name: "抜けた番号の範囲" });
  fireEvent.change(screen.getByLabelText("比べる収集元"), {
    target: { value: viewer.sourceId },
  });
  await waitFor(() =>
    expect(pairText("突き合わせ")).toMatch(/^突き合わせ: 未比較/),
  );
  expect(pairText("理由")).toBe("理由: UTC 時刻を持つレコードのない収集元あり");
  expect(pairText("次の操作")).toBe("次の操作: 収集元のタイムゾーンの記録");
});

test("HTTP の失敗と、読めない応答を失敗として出す", async () => {
  stubFetch(() =>
    jsonResponse(500, { code: "internal_error", message: "failed" }),
  );
  renderNumbers(events);
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "レコードの番号の抜けの取得サーバーの内部の失敗",
  );
  cleanup();
  stubFetch(ok({ recordNumbers: { ...gapNumbers, examination: "unknown" } }));
  renderNumbers(events);
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "レコードの番号の抜けの取得サーバーと画面のバージョンの不一致",
  );
  expect(screen.queryByRole("list", { name: "抜けた番号の範囲" })).toBeNull();
});

test("dataVersion が変わると取り直す", async () => {
  const fetch = stubFetch(ok({ recordNumbers: gapNumbers }));
  const { view, onSelectRecord } = renderNumbers(events);
  await screen.findByRole("list", { name: "抜けた番号の範囲" });
  view.rerender(
    <NumbersUnderTest
      selectedSource={events}
      dataVersion={1}
      onSelectRecord={onSelectRecord}
    />,
  );
  await screen.findByRole("list", { name: "抜けた番号の範囲" });
  expect(fetch).toHaveBeenCalledTimes(2);
});

test("前の選択の応答を新しい選択の下に出さない", async () => {
  let release: ((response: Response) => void) | undefined;
  const pending = new Promise<Response>((resolve) => {
    release = resolve;
  });
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(jsonResponse(200, { recordNumbers: gapNumbers }))
    .mockReturnValueOnce(pending);
  vi.stubGlobal("fetch", fetch);
  const { view, onSelectRecord } = renderNumbers(events);
  await screen.findByRole("list", { name: "抜けた番号の範囲" });
  view.rerender(
    <NumbersUnderTest
      selectedSource={viewer}
      dataVersion={0}
      onSelectRecord={onSelectRecord}
    />,
  );
  expect(screen.getByRole("status")).toHaveTextContent(
    "レコードの番号の確認中",
  );
  expect(screen.queryByRole("list", { name: "抜けた番号の範囲" })).toBeNull();
  release?.(jsonResponse(200, { recordNumbers: viewerNumbers }));
  await examinedAs("未確認");
});
