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
import type { RecordLocator } from "@/shared/contracts/common";
import { DisplayOffsetContext } from "@/shared/ui/DisplayOffset";
import { jsonResponse } from "@/testdata/http";
import {
  apiErrorJson,
  hostALogSha256,
  hostALogSource,
  hostALogSourceId,
} from "@/testdata/sources/sourcesResponse";
import { ValueActionsForTest } from "@/testdata/valueActions";
import { RecordContext } from "./RecordContext";

/** 時刻を等幅の span に分けて描いた段落を、段落の文字列全体で探す。 */
/** name の「名前: 値」の組の文字列。組が無いときは undefined を返す。 */
function pairText(name: string): string | undefined {
  return Array.from(document.querySelectorAll("li"))
    .map((item) => item.textContent ?? "")
    .find((text) => text.startsWith(`${name}: `));
}

const matchConditions: MatchConditionSelection = {
  conditions: [{ conditionKey: "destination_ip" }],
};

function locator(sequenceNumber: number): RecordLocator {
  return {
    sourceId: hostALogSourceId,
    sourceContentSha256: hostALogSha256,
    sourceFileName: "sample.log",
    positionKind: "sequence_number",
    sequenceNumber,
    lineNumber: sequenceNumber + 1,
    recordRawTextRef: `/api/v0/records?sequenceNumber=${sequenceNumber}`,
  };
}

const openedRef = locator(101);

function eventTime(
  rawText: string,
  normalized: string,
  offsetState = "in_value",
) {
  return {
    rawText,
    normalized,
    normalizedForm:
      offsetState === "in_value" ? "rfc3339_absolute" : "local_without_offset",
    precision: "millisecond",
    offsetState,
    clock: "terminal_local",
    meaning: "event",
    valueState: "present",
  };
}

const observationKind = {
  raw: [
    {
      name: "evt",
      kind: "text",
      text: { rawText: "net", valueState: "present" },
    },
  ],
  status: "determined",
};

function recordJson(
  time = eventTime("01/01/2021 10:00:00.500", "2021-01-01T10:00:00.500+09:00"),
) {
  return {
    recordRef: openedRef,
    rawText: "synthetic record",
    sourceIdentity: hostALogSource(),
    fields: [
      {
        name: "time",
        semantic: "event.time",
        kind: "timestamp",
        timestamp: time,
      },
    ],
    observationKind,
  };
}

function terminal(id: string, label: string) {
  return {
    id,
    kind: "terminal",
    keyForm: "terminal_id",
    identity: [{ semantic: "terminal.id", value: `${label}-ID` }],
    label: { rawText: label, valueState: "present" },
    observation: "observed",
    creationRecord: "item_absent",
  };
}

/**
 * 時系列の行 1 つ。terminal は端末のノードの識別子の末尾、label は端末の表示名である。
 * label を省いた行は terminal を表示名にする。
 */
type EntrySpec = {
  sequenceNumber: number;
  second: string;
  terminal?: string;
  label?: string;
};

// 通番 103 の端末は、表示名が通番 101 の端末と同じで、識別子が別の端末である。
const entrySpecs: EntrySpec[] = [
  { sequenceNumber: 100, second: "09:59:58.000", terminal: "WS-A" },
  { sequenceNumber: 101, second: "10:00:00.500", terminal: "WS-A" },
  { sequenceNumber: 102, second: "10:00:01.000", terminal: "WS-B" },
  {
    sequenceNumber: 103,
    second: "10:00:02.000",
    terminal: "WS-A-other",
    label: "WS-A",
  },
];

function timelineJson(specs: EntrySpec[] = entrySpecs) {
  return {
    entries: specs.map((spec) => ({
      recordRef: locator(spec.sequenceNumber),
      eventTime: eventTime(
        `01/01/2021 ${spec.second}`,
        `2021-01-01T${spec.second}+09:00`,
      ),
      observationKind,
      ...(spec.terminal === undefined
        ? {}
        : {
            terminal: terminal(
              `n:terminal:${spec.terminal}`,
              spec.label ?? spec.terminal,
            ),
          }),
    })),
    entryCount: specs.length,
    undatedRecordCount: 0,
    sourceCoverages: [],
    ...(specs.length === 0 ? { emptyReason: "no_record_in_filter" } : {}),
  };
}

function stubFetch(
  record: { status: number; json: unknown } = {
    status: 200,
    json: recordJson(),
  },
  timeline: unknown = timelineJson(),
) {
  const mock = vi.fn(async (input: string, _init?: RequestInit) =>
    input.includes("/records")
      ? jsonResponse(record.status, record.json)
      : jsonResponse(200, timeline),
  );
  vi.stubGlobal("fetch", mock);
  return mock;
}

function requestedUrls(mock: ReturnType<typeof stubFetch>): URL[] {
  return mock.mock.calls.map(([input]) => new URL(input, "http://localhost"));
}

function renderContext(
  recordRef: RecordLocator | undefined,
  onSelectRecord: (recordRef: RecordLocator) => void = () => {},
) {
  return render(
    <ValueActionsForTest onSelectRecord={onSelectRecord}>
      <RecordContext
        recordRef={recordRef}
        matchConditions={matchConditions}
        dataVersion={0}
      />
    </ValueActionsForTest>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

test("レコードを開いていないときは、未選択を出して取得しない", () => {
  const mock = stubFetch();

  renderContext(undefined);

  expect(pairText("レコード")).toBe("レコード: 未選択");
  expect(mock).not.toHaveBeenCalled();
});

test("読み込み中を出し、元レコードの時刻の前後 60 秒の時系列を要求する", async () => {
  const mock = stubFetch();

  renderContext(openedRef);

  expect(screen.getByRole("status").textContent).toBe(
    "レコードの前後の読み込み中",
  );
  await screen.findByRole("table", { name: /^全端末/ });
  const [recordUrl, timelineUrl] = requestedUrls(mock);
  expect(recordUrl?.pathname).toMatch(/\/records$/);
  expect(recordUrl?.searchParams.get("sourceId")).toBe(hostALogSourceId);
  expect(recordUrl?.searchParams.get("sequenceNumber")).toBe("101");
  expect(timelineUrl?.pathname).toMatch(/\/timeline$/);
  expect(timelineUrl?.searchParams.get("timeFrom")).toBe(
    "2021-01-01T09:59:00.500+09:00",
  );
  expect(timelineUrl?.searchParams.get("timeTo")).toBe(
    "2021-01-01T10:01:00.500+09:00",
  );
  expect(timelineUrl?.searchParams.get("timeFromPrecision")).toBe(
    "millisecond",
  );
  expect(timelineUrl?.searchParams.get("timeToPrecision")).toBe("millisecond");
  expect(timelineUrl?.searchParams.get("filterUnit")).toBe("millisecond");
  expect(timelineUrl?.searchParams.getAll("matchCondition")).toEqual([
    "destination_ip",
  ]);
  expect(pairText("期間")).toBe(
    "期間: 2021-01-01T09:59:00.500+09:00 – 2021-01-01T10:01:00.500+09:00",
  );
});

test("元レコードを取得できなければ失敗を出し、時系列を要求しない", async () => {
  const mock = stubFetch({
    status: 404,
    json: apiErrorJson("record_not_found"),
  });

  renderContext(openedRef);

  expect(await screen.findByText("レコードの取得")).toBeTruthy();
  expect(requestedUrls(mock).map((url) => url.pathname)).toEqual([
    expect.stringMatching(/\/records$/),
  ]);
});

test("時刻を UTC 時刻に解釈できないレコードは、前後の表示不可と原文の時刻を出す", async () => {
  const mock = stubFetch({
    status: 200,
    json: recordJson(
      eventTime(
        "01/01/2021 10:00:00.500",
        "2021-01-01T10:00:00.500",
        "item_absent",
      ),
    ),
  });

  renderContext(openedRef);

  await screen.findByText("表示不可");
  expect(pairText("前後")).toMatch(/^前後: 表示不可/);
  expect(pairText("原文の時刻")).toBe("原文の時刻: 01/01/2021 10:00:00.500");
  expect(mock.mock.calls.some(([input]) => input.includes("/timeline"))).toBe(
    false,
  );
});

test("分析者のずれを与えた地方時は、そのずれで前後を要求し、解釈で読んだ時刻であることを出す", async () => {
  const interpretation = { offset: "+09:00", assertionId: "a-1" };
  const local = {
    ...eventTime(
      "01/01/2021 10:00:00.500",
      "2021-01-01T10:00:00.500",
      "item_absent",
    ),
    interpretation,
  };
  const timeline = timelineJson(entrySpecs.slice(0, 2));
  timeline.entries[1] = { ...timeline.entries[1], eventTime: local };
  const mock = stubFetch({ status: 200, json: recordJson(local) }, timeline);

  renderContext(openedRef);

  const all = await screen.findByRole("table", { name: /^全端末/ });
  const timelineUrl = requestedUrls(mock).find((url) =>
    url.pathname.endsWith("/timeline"),
  );
  expect(timelineUrl?.searchParams.get("timeFrom")).toBe(
    "2021-01-01T09:59:00.500+09:00",
  );
  expect(pairText("開いたレコードの時刻")).toContain("分析者の記録: UTC+09:00");
  // 期間の両端の +09:00 も解釈から来たことを、期間の組の横に出す。
  expect(pairText("期間")).toBe(
    "期間: 2021-01-01T09:59:00.500+09:00 – 2021-01-01T10:01:00.500+09:00",
  );
  expect(
    screen.getByText(/^期間:/, { selector: ".pair-name" }).closest("div")
      ?.textContent,
  ).toContain("分析者の記録: UTC+09:00");
  // 時刻の欄は UTC を出し、読んだずれと UTC の詳細を title に入れる。
  const timeFacts = (row: HTMLElement | undefined) =>
    within(row as HTMLElement)
      .getAllByRole("cell")[1]
      ?.querySelector("[title]")
      ?.getAttribute("title") ?? "";
  const rows = within(all).getAllByRole("row").slice(1);
  expect(timeFacts(rows[0])).not.toContain("分析者の記録");
  expect(timeFacts(rows[1])).toContain("分析者の記録: UTC+09:00");
  expect(timeFacts(rows[1])).toContain("UTC: 2021-01-01T01:00:00.500Z");
  const timeCell = (row: HTMLElement | undefined) =>
    within(within(row as HTMLElement).getAllByRole("cell")[1] as HTMLElement);
  // 解釈から求めた UTC であることを画面の文字で出す。ミリ秒の精度は title に入れる。
  expect(timeCell(rows[1]).getByText("2021-01-01 01:00:00.500")).toBeTruthy();
  expect(timeCell(rows[1]).getByText("分析者: UTC+09:00")).toBeTruthy();
  expect(timeCell(rows[1]).queryByText(/^精度: /)).toBeNull();
  expect(timeFacts(rows[1])).toContain("精度: ミリ秒");
  expect(timeCell(rows[0]).queryByText(/^分析者: /)).toBeNull();
});

test("表示のタイムゾーンを選ぶと、そのタイムゾーンの期間を添え、起動の指定のタイムゾーンであることを出す", async () => {
  const local = {
    ...eventTime(
      "01/01/2021 10:00:00.500",
      "2021-01-01T10:00:00.500",
      "item_absent",
    ),
    interpretation: { offset: "+00:00" },
  };
  stubFetch(
    { status: 200, json: recordJson(local) },
    timelineJson(entrySpecs.slice(0, 2)),
  );

  render(
    <DisplayOffsetContext.Provider value="+09:00">
      <RecordContext
        recordRef={openedRef}
        matchConditions={matchConditions}
        dataVersion={0}
      />
    </DisplayOffsetContext.Provider>,
  );

  await screen.findByRole("table", { name: /^全端末/ });
  expect(pairText("期間")).toBe(
    "期間: 2021-01-01T09:59:00.500+00:00 – 2021-01-01T10:01:00.500+00:00",
  );
  expect(pairText("UTC+09:00")).toBe(
    "UTC+09:00: 2021-01-01T18:59:00.500+09:00 – 2021-01-01T19:01:00.500+09:00",
  );
  expect(
    screen.getByText(/^期間:/, { selector: ".pair-name" }).closest("div")
      ?.textContent,
  ).toContain("起動時の指定: UTC+00:00");
});

test("同じ端末の一覧は開いたレコードの端末の行だけを出し、開いた行に印を付ける", async () => {
  stubFetch();

  renderContext(openedRef);

  const same = await screen.findByRole("table", { name: /^同じ端末/ });
  const all = screen.getByRole("table", { name: /^全端末/ });
  const sameRows = within(same).getAllByRole("row").slice(1);
  const allRows = within(all).getAllByRole("row").slice(1);
  // 端末は識別子で比べる。表示名が同じ別の端末の行 (通番 103) は入らない。
  const positionsOf = (rows: HTMLElement[]) =>
    rows.map(
      (row) =>
        /ID: \d+/.exec(
          within(row)
            .getByRole("button", { name: /^Record に表示: / })
            .getAttribute("aria-label") ?? "",
        )?.[0],
    );
  expect(positionsOf(sameRows)).toEqual(["ID: 100", "ID: 101"]);
  expect(positionsOf(allRows)).toEqual([
    "ID: 100",
    "ID: 101",
    "ID: 102",
    "ID: 103",
  ]);
  expect(same.querySelector("caption")?.textContent).toBe("同じ端末: 2");
  for (const rows of [sameRows, allRows]) {
    const marked = rows.filter(
      (row) => row.getAttribute("aria-current") === "true",
    );
    expect(marked).toHaveLength(1);
    expect(positionsOf(marked)).toEqual(["ID: 101"]);
  }
});

test("開いたレコードに端末が現れないときは、同じ端末の一覧の代わりに理由を出す", async () => {
  stubFetch(
    undefined,
    timelineJson(
      entrySpecs.map((spec) =>
        spec.sequenceNumber === 101 ? { ...spec, terminal: undefined } : spec,
      ),
    ),
  );

  renderContext(openedRef);

  await screen.findByRole("table", { name: /^全端末/ });
  expect(pairText("同じ端末")).toBe("同じ端末: 開いたレコードに端末なし");
  expect(screen.queryByRole("table", { name: /^同じ端末/ })).toBeNull();
});

test("導いた表示名の端末は、導いた値に導いたことと導き方を添えて出す", async () => {
  const json = timelineJson();
  const [first, ...rest] = json.entries;
  stubFetch(undefined, {
    ...json,
    entries: [
      {
        ...first,
        terminal: {
          ...first?.terminal,
          label: {
            normalized: "host-a.example.test",
            derivation: "合成の導き方",
            valueState: "derived",
          },
        },
      },
      ...rest,
    ],
  });

  renderContext(openedRef);

  const all = await screen.findByRole("table", { name: /^全端末/ });
  const [derivedRow, presentRow] = within(all).getAllByRole("row").slice(1);
  expect(derivedRow?.textContent).toContain(
    "host-a.example.test Oraculum が作った表示名\n作り方: 合成の導き方",
  );
  expect(derivedRow?.textContent).not.toContain("表示名なし");
  // 原資料の文字列を持つ表示名には注記を添えない。
  expect(presentRow?.textContent).toContain("WS-A");
  expect(presentRow?.textContent).not.toContain("Oraculum が");
});

test("行の位置の button はその行のレコードの位置を渡す", async () => {
  stubFetch();
  const onSelectRecord = vi.fn();

  renderContext(openedRef, onSelectRecord);

  const all = await screen.findByRole("table", { name: /^全端末/ });
  const row = within(all)
    .getAllByRole("row")
    .find(
      (candidate) =>
        candidate
          .querySelector('[aria-label^="Record に表示: "]')
          ?.getAttribute("aria-label")
          ?.includes("ID: 102") === true,
    );
  if (row === undefined) {
    throw new Error("row with sequence number 102 is missing");
  }
  fireEvent.click(
    within(row).getByRole("button", { name: /^Record に表示: / }),
  );
  expect(onSelectRecord).toHaveBeenCalledWith(locator(102));
});

test("時点を持たず期間の判定から外れたレコードの件数と理由を出す", async () => {
  stubFetch(undefined, {
    ...timelineJson(),
    periodUnjudged: { localRecordCount: 2, undatedRecordCount: 1 },
  });

  renderContext(openedRef);

  const name = await screen.findByText("表の外:");
  const pairs = name.closest("ul")?.querySelectorAll(":scope > li") ?? [];
  expect([...pairs].map((pair) => pair.textContent)).toEqual([
    "表の外: 3",
    "タイムゾーン不明: 2 件",
    "時刻なし: 1 件",
  ]);
});

test("幅を変えると新しい両端で取り直す", async () => {
  const mock = stubFetch();

  renderContext(openedRef);
  await screen.findByRole("table", { name: /^全端末/ });
  fireEvent.change(screen.getByLabelText("前後の幅"), {
    target: { value: "10" },
  });
  await waitFor(() =>
    expect(pairText("期間")).toBe(
      "期間: 2021-01-01T09:59:50.500+09:00 – 2021-01-01T10:00:10.500+09:00",
    ),
  );

  const timelineUrls = requestedUrls(mock).filter((url) =>
    url.pathname.endsWith("/timeline"),
  );
  const last = timelineUrls[timelineUrls.length - 1];
  expect(last?.searchParams.get("timeFrom")).toBe(
    "2021-01-01T09:59:50.500+09:00",
  );
  expect(last?.searchParams.get("timeTo")).toBe(
    "2021-01-01T10:00:10.500+09:00",
  );
});
