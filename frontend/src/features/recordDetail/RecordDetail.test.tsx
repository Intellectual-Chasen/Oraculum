// @vitest-environment jsdom
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { decodeRecordLocator } from "@/shared/contracts/common";
import { jsonResponse, textResponse } from "@/testdata/http";
import {
  accessLogRecordRawText,
  accessLogRecordResponseJson,
  clientTerminalDerivation,
  clientTerminalId,
  clientTerminalName,
  hostALogRecordResponseJson,
  htmlLikeRawText,
  htmlLikeRecordResponseJson,
  internalErrorJson,
  invisibleCharacterRecordResponseJson,
  repeatedFieldNameRecordResponseJson,
  repeatedKeyValues,
  requestTargetHost,
  requestTargetHostDerivation,
  requestTargetRawText,
  stoppedRecordResponseJson,
} from "@/testdata/records/recordResponse";
import { apiErrorJson } from "@/testdata/sources/sourcesResponse";
import { ValueActionsForTest } from "@/testdata/valueActions";
import { RecordDetail, type TrailOrigin } from "./RecordDetail";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  // spy を test ごとに戻す。assertion が失敗した test の中で戻すと、失敗した実行でだけ
  // console.error が差し替えられたまま後続の test が走り、React の警告が読めなくなる。
  vi.restoreAllMocks();
});

function stubFetch(result: Response) {
  const mock = vi.fn(async (_input: string, _init?: RequestInit) => result);
  vi.stubGlobal("fetch", mock);
  return mock;
}

/** 応答の `recordRef` を、上位の画面が渡すレコード位置として読む。 */
function recordRefOf(
  responseJson: unknown,
): ReturnType<typeof decodeRecordLocator> {
  const response = responseJson as { recordRef: unknown };
  return decodeRecordLocator(response.recordRef, "fixture.recordRef");
}

const accessLogRecordRef = recordRefOf(accessLogRecordResponseJson());
const hostALogRecordRef = recordRefOf(hostALogRecordResponseJson());

/** 「名前: 値」の組の文字列の一覧。 */
function pairTexts(): string[] {
  return Array.from(document.querySelectorAll(".value-pairs > li")).map(
    (item) => item.textContent ?? "",
  );
}

function fieldRow(name: string): HTMLElement {
  const table = screen.getByRole("table", { name: "フィールド" });
  const row = within(table).getByRole("row", { name: new RegExp(`^${name} `) });
  return row;
}

test("レコードを選ぶ前は、未選択を示す", () => {
  stubFetch(jsonResponse(200, accessLogRecordResponseJson()));

  render(<RecordDetail recordRef={undefined} />);

  expect(pairTexts()).toEqual(["レコード: 未選択"]);
  expect(screen.queryByRole("table", { name: "フィールド" })).toBeNull();
});

test("読み込み中を示してから、レコードの原文と応答の全項目を出す", async () => {
  stubFetch(jsonResponse(200, accessLogRecordResponseJson()));

  render(<RecordDetail recordRef={accessLogRecordRef} />);

  expect(screen.getByRole("status").textContent).toBe("レコードの読み込み中");

  const table = await screen.findByRole("table", { name: "フィールド" });
  expect(screen.getByText(accessLogRecordRawText)).toBeTruthy();
  // 見出しの 1 行と、応答の fields の 1 件につき 1 行である。
  expect(within(table).getAllByRole("row")).toHaveLength(18);
});

// byte 位置だけで指すレコードは、行番号も通番も持たない。位置の指定に byte 位置を載せないと、
// 要求を送れない。
test("byte 位置だけで指すレコードを、byte 位置で要求する", async () => {
  const fetchMock = stubFetch(jsonResponse(200, accessLogRecordResponseJson()));
  const byteRangeRef = {
    ...accessLogRecordRef,
    positionKind: "byte_range" as const,
    sequenceNumber: undefined,
    lineNumber: undefined,
    byteOffset: 4096,
    byteLength: 726,
  };

  render(<RecordDetail recordRef={byteRangeRef} />);

  await screen.findByRole("table", { name: "フィールド" });
  expect(fetchMock).toHaveBeenCalledTimes(1);
  const url = new URL(String(fetchMock.mock.calls[0]?.[0]), "http://test");
  expect(url.searchParams.get("byteOffset")).toBe("4096");
  expect(url.searchParams.get("lineNumber")).toBeNull();
  expect(url.searchParams.get("sourceId")).toBe(byteRangeRef.sourceId);
});

test("変換した原文を持つ収集元のレコードは、原文が変換した文字列であることを出す", async () => {
  const response = accessLogRecordResponseJson() as {
    sourceIdentity: Record<string, unknown>;
  };
  response.sourceIdentity.rawTextConverted = true;
  stubFetch(jsonResponse(200, response));

  render(<RecordDetail recordRef={accessLogRecordRef} />);

  expect(await screen.findByRole("heading", { name: "原文" })).toBeTruthy();
  expect(screen.getByText("変換した原文")).toBeTruthy();
  expect(document.body.textContent).toContain(
    "収集元の byte 列から読み取りが組み立てた文字列",
  );
  expect(screen.getByText(accessLogRecordRawText)).toBeTruthy();
});

test("複数の file を連結した収集元のレコードは、位置を持つ file とその中の byte 位置を出す", async () => {
  const response = accessLogRecordResponseJson() as {
    recordRef: Record<string, unknown>;
    sourceIdentity: Record<string, unknown>;
  };
  response.recordRef = {
    ...response.recordRef,
    positionKind: "byte_range",
    byteOffset: 700,
    byteLength: 96,
  };
  delete response.recordRef.lineNumber;
  delete response.recordRef.sequenceNumber;
  response.sourceIdentity.members = [
    {
      originPath: "logs/hive",
      contentSha256: "a".repeat(64),
      byteOffset: 0,
      sizeBytes: 600,
    },
    {
      originPath: "logs/hive.LOG1",
      contentSha256: "b".repeat(64),
      byteOffset: 600,
      sizeBytes: 424,
    },
  ];
  stubFetch(jsonResponse(200, response));

  render(<RecordDetail recordRef={accessLogRecordRef} />);

  const row = await screen.findByRole("rowheader", {
    name: "位置を持つ file",
  });
  expect(row.closest("tr")?.textContent).toBe(
    "位置を持つ filelogs/hive.LOG1 位置: 100-196",
  );
});

test("1 つの file の収集元のレコードは、位置を持つ file の行を出さない", async () => {
  stubFetch(jsonResponse(200, accessLogRecordResponseJson()));

  render(<RecordDetail recordRef={accessLogRecordRef} />);

  await screen.findByRole("table", { name: "フィールド" });
  expect(
    screen.queryByRole("rowheader", { name: "位置を持つ file" }),
  ).toBeNull();
});

test("収集元の byte 列を原文に持つレコードは、変換した原文と示さない", async () => {
  stubFetch(jsonResponse(200, accessLogRecordResponseJson()));

  render(<RecordDetail recordRef={accessLogRecordRef} />);

  expect(await screen.findByRole("heading", { name: "原文" })).toBeTruthy();
  expect(screen.queryByText("変換した原文")).toBeNull();
});

test("欄の値の button で、その値を含む条件に足す", async () => {
  stubFetch(jsonResponse(200, accessLogRecordResponseJson()));
  const onAddCondition = vi.fn();

  render(
    <ValueActionsForTest onAddCondition={onAddCondition}>
      <RecordDetail recordRef={accessLogRecordRef} />
    </ValueActionsForTest>,
  );

  await screen.findByRole("table", { name: "フィールド" });
  // fixture は要求先の文字列を 2 つの欄 (要求先と、要求の行から読んだ欄) に持つ。
  const [button] = screen.getAllByRole("button", {
    name: `含む条件に追加: ${requestTargetRawText}`,
  });
  button?.click();
  expect(onAddCondition).toHaveBeenCalledTimes(1);
  expect(onAddCondition).toHaveBeenCalledWith({
    kind: "text",
    mode: "contains",
    text: requestTargetRawText,
  });
});

test("条件に足す操作を渡さないときは、button を出さない", async () => {
  stubFetch(jsonResponse(200, accessLogRecordResponseJson()));

  render(<RecordDetail recordRef={accessLogRecordRef} />);

  await screen.findByRole("table", { name: "フィールド" });
  expect(
    screen.queryByRole("button", { name: /^含む条件に追加: / }),
  ).toBeNull();
});

// 原資料が同じ key を 2 回書いたレコードでは、応答の name が 2 件になる。
// 2 件を 1 行にまとめず、それぞれの値を対応する行に出す。
test("同じ name を 2 件持つ応答を、値の対応する 2 行に出す", async () => {
  const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
  stubFetch(jsonResponse(200, repeatedFieldNameRecordResponseJson()));

  render(<RecordDetail recordRef={hostALogRecordRef} />);
  const table = await screen.findByRole("table", { name: "フィールド" });

  const rows = within(table).getAllByRole("row", { name: /^hide / });
  expect(rows).toHaveLength(repeatedKeyValues.length);
  for (const [index, value] of repeatedKeyValues.entries()) {
    const cells = within(rows[index] as HTMLElement).getAllByRole("cell");
    expect(cells[0]?.textContent).toBe(value);
  }
  // 2 件が同じ値であれば、行と値の対応が崩れても検査が通る。
  expect(repeatedKeyValues[0]).not.toBe(repeatedKeyValues[1]);
  // React は同じ key を持つ兄弟を error で報告する。行の key が name だけだと出る。
  expect(consoleError).not.toHaveBeenCalled();
});

test("解釈のずれを与えた時刻の項目は、正規化値を書き換えずに UTC を添え、解釈を持たない時刻には添えない", async () => {
  const response = accessLogRecordResponseJson() as {
    fields: { name: string; kind: string; timestamp?: unknown }[];
  };
  response.fields.push({
    name: "localTime",
    kind: "timestamp",
    timestamp: {
      rawText: "2031/10/08 10:20:35",
      normalized: "2031-10-08T10:20:35",
      normalizedForm: "local_without_offset",
      precision: "second",
      offsetState: "item_absent",
      clock: "observer_local",
      meaning: "event",
      valueState: "present",
      interpretation: { offset: "+09:00" },
    },
  });
  stubFetch(jsonResponse(200, response));

  render(<RecordDetail recordRef={accessLogRecordRef} />);
  await screen.findByRole("table", { name: "フィールド" });

  const normalizedCell = within(fieldRow("localTime")).getAllByRole("cell")[1];
  expect(normalizedCell?.textContent).toBe(
    "2031-10-08T10:20:35UTC: 2031-10-08T01:20:35Z",
  );
  const absolute = within(fieldRow("requestTime")).getAllByRole("cell")[1];
  expect(absolute?.textContent).toBe("2031-10-08T10:20:35+09:00");
});

test("入力形式に欄が無い項目と、値の不在の文字列を持つ項目を別の文で出す", async () => {
  stubFetch(jsonResponse(200, accessLogRecordResponseJson()));

  render(<RecordDetail recordRef={accessLogRecordRef} />);
  await screen.findByRole("table", { name: "フィールド" });

  const itemAbsent = fieldRow("clientPort");
  expect(itemAbsent.textContent).toContain("フィールドなし");

  const absent = fieldRow("referer");
  expect(absent.textContent).toContain('"-"');
  expect(absent.textContent).toContain("値なし");
  expect(absent.textContent).not.toContain("フィールドなし");
});

test("導いた項目は原資料の文字列を欠測として出し、正規化値と導き方を出す", async () => {
  stubFetch(jsonResponse(200, accessLogRecordResponseJson()));

  render(<RecordDetail recordRef={accessLogRecordRef} />);
  await screen.findByRole("table", { name: "フィールド" });

  const derived = fieldRow("clientTerminal");
  expect(derived.textContent).toContain("別の収集元から求めた値");
  expect(derived.textContent).toContain(clientTerminalId);
  expect(derived.textContent).toContain(clientTerminalDerivation);
});

test("端末の識別子と表示名を別の行に出す", async () => {
  stubFetch(jsonResponse(200, accessLogRecordResponseJson()));

  render(<RecordDetail recordRef={accessLogRecordRef} />);
  await screen.findByRole("table", { name: "フィールド" });

  expect(fieldRow("clientTerminal").textContent).toContain(clientTerminalId);
  expect(fieldRow("clientTerminalName").textContent).toContain(
    clientTerminalName,
  );
  // 2 つが同じ値であれば、取り違えても両方の検査が通る。
  expect(clientTerminalName).not.toBe(clientTerminalId);
  expect(fieldRow("clientTerminal").textContent).not.toContain(
    clientTerminalName,
  );
});

test("原資料の文字列と正規化値を別の列に出し、正規化値に導き方を添える", async () => {
  stubFetch(jsonResponse(200, accessLogRecordResponseJson()));

  render(<RecordDetail recordRef={accessLogRecordRef} />);
  await screen.findByRole("table", { name: "フィールド" });

  const row = fieldRow("requestTargetHost");
  const cells = within(row).getAllByRole("cell");
  expect(cells[0]?.textContent).toContain(requestTargetRawText);
  expect(cells[1]?.textContent).toContain(requestTargetHost);
  expect(cells[1]?.textContent).toContain(requestTargetHostDerivation);
  expect(
    within(cells[0] as HTMLElement).getByRole("button", {
      name: `コピー: ${requestTargetRawText}`,
    }),
  ).toBeTruthy();
});

test("時刻の項目に精度を出し、時刻以外の項目に時刻の項目でないことを出す", async () => {
  stubFetch(jsonResponse(200, accessLogRecordResponseJson()));

  render(<RecordDetail recordRef={accessLogRecordRef} />);
  await screen.findByRole("table", { name: "フィールド" });

  expect(fieldRow("requestTime").textContent).toContain("秒");
  expect(fieldRow("clientIp").textContent).toContain("時刻のフィールド以外");
});

/** 経路を求める起点。関連付けの条件と、グラフを組み直した回数を添える。 */
function trailOriginOf(overrides: Partial<TrailOrigin> = {}): TrailOrigin {
  return {
    origin: accessLogRecordRef,
    matchConditions: {
      conditions: [
        { conditionKey: "destination_port" },
        { conditionKey: "second_of_time", toleranceSeconds: 0 },
      ],
    },
    dataVersion: 0,
    ...overrides,
  };
}

/**
 * レコードの要求と経路の要求に別の応答を返す。経路の要求は起点の項目を持つ。
 * 応答の本体は 1 回しか読めないため、要求ごとに作る。
 */
function stubRecordAndTrail(
  recordResponse: () => Response,
  trailResponse: () => Response,
) {
  const mock = vi.fn(async (input: string, _init?: RequestInit) =>
    input.includes("originSourceId") ? trailResponse() : recordResponse(),
  );
  vi.stubGlobal("fetch", mock);
  const urls = (withOrigin: boolean) =>
    mock.mock.calls
      .map((call) => new URL(String(call[0]), "http://test"))
      .filter((url) => url.searchParams.has("originSourceId") === withOrigin);
  return { recordUrls: () => urls(false), trailUrls: () => urls(true) };
}

test("起点を渡さないときは、経路を求めず、経路の欄を出さない", async () => {
  const fetchMock = stubFetch(jsonResponse(200, accessLogRecordResponseJson()));

  render(<RecordDetail recordRef={accessLogRecordRef} />);
  await screen.findByRole("table", { name: "フィールド" });

  expect(fetchMock).toHaveBeenCalledTimes(1);
  const url = new URL(String(fetchMock.mock.calls[0]?.[0]), "http://test");
  expect(url.searchParams.has("originSourceId")).toBe(false);
  expect(url.searchParams.has("matchCondition")).toBe(false);
  expect(
    screen.queryByRole("heading", { level: 3, name: "到達した経路" }),
  ).toBeNull();
});

// レコードは起点なしの要求で取り、経路は起点と条件を付けた別の要求で取る。
test("起点を渡すと、経路を起点と条件を付けた別の要求で取り、各段階を応答の並び順で出す", async () => {
  const requests = stubRecordAndTrail(
    () => jsonResponse(200, hostALogRecordResponseJson()),
    () => jsonResponse(200, hostALogRecordResponseJson()),
  );

  render(
    <RecordDetail
      recordRef={hostALogRecordRef}
      trailOrigin={trailOriginOf()}
    />,
  );

  const table = await screen.findByRole("table", {
    name: "到達した経路の段階",
  });
  expect(screen.getByRole("table", { name: "フィールド" })).toBeTruthy();
  expect(requests.recordUrls()).toHaveLength(1);
  expect(requests.recordUrls()[0]?.searchParams.has("matchCondition")).toBe(
    false,
  );
  const trailUrl = requests.trailUrls()[0];
  expect(requests.trailUrls()).toHaveLength(1);
  expect(trailUrl?.searchParams.get("sourceId")).toBe(
    hostALogRecordRef.sourceId,
  );
  expect(trailUrl?.searchParams.get("originSourceId")).toBe(
    accessLogRecordRef.sourceId,
  );
  expect(trailUrl?.searchParams.get("originLineNumber")).toBe(
    String(accessLogRecordRef.lineNumber),
  );
  expect(trailUrl?.searchParams.getAll("matchCondition")).toEqual([
    "destination_port",
    "second_of_time~0",
  ]);
  const steps = within(table)
    .getAllByRole("rowheader")
    .map((header) => header.textContent);
  expect(steps).toEqual(["A1", "A2", "A3", "A4", "A5", "A6", "A7", "A8"]);
  expect(pairTexts()).toContain("起点のレコード: 収集元: access.log、行: 37");
  expect(
    within(table).getByRole("row", { name: /^A1 / }).textContent,
  ).toContain("原文の外の入力のみ");
  expect(pairTexts()).toContain("進めなかった段階: なし");
});

test("進めなかった段階を含む応答は、その段階を出す", async () => {
  stubRecordAndTrail(
    () => jsonResponse(200, hostALogRecordResponseJson()),
    () => jsonResponse(200, stoppedRecordResponseJson()),
  );

  render(
    <RecordDetail
      recordRef={hostALogRecordRef}
      trailOrigin={trailOriginOf()}
    />,
  );
  await screen.findByRole("table", { name: "到達した経路の段階" });

  expect(pairTexts()).toContain("進めなかった段階: C5");
  expect(pairTexts()).toContain("出力: 同じ psGUID の ps / start が 0 件");
});

// 関連付けの段階の種別は、英語の enum の文字列を画面に出さない。
test("関連付けの段階で止まった経路は、段階を画面の呼び名で出す", async () => {
  const originJson = (accessLogRecordResponseJson() as { recordRef: unknown })
    .recordRef;
  const stoppedStep = {
    stepKey: "second_time_matched",
    inputRefs: [{ kind: "record", record: originJson }],
    usedIdentifiers: ["選んだ条件: 接続先 port"],
    output: "この組がどの段階で外れたかを、グラフから特定できない",
  };
  stubRecordAndTrail(
    () => jsonResponse(200, hostALogRecordResponseJson()),
    () =>
      jsonResponse(200, {
        ...(hostALogRecordResponseJson() as Record<string, unknown>),
        derivationTrail: {
          originRef: originJson,
          steps: [stoppedStep],
          stoppedAt: stoppedStep,
        },
      }),
  );

  render(
    <RecordDetail
      recordRef={hostALogRecordRef}
      trailOrigin={trailOriginOf()}
    />,
  );

  const table = await screen.findByRole("table", {
    name: "到達した経路の段階",
  });
  expect(
    within(table)
      .getAllByRole("rowheader")
      .map((header) => header.textContent),
  ).toEqual(["段階 2: 同じ秒の時刻"]);
  expect(pairTexts()).toContain("進めなかった段階: 段階 2: 同じ秒の時刻");
  expect(pairTexts()).toContain(
    "出力: この組がどの段階で外れたかを、グラフから特定できない",
  );
  expect(screen.queryByText(/second_time_matched/)).toBeNull();
});

// 経路の失敗は経路の欄に出し、起点なしの要求で読めたレコードの表示を保つ。
// 失敗の応答が code を含まない行は、失敗の識別子を出さない。
test.each<{ name: string; response: () => Response; code: string | undefined }>(
  [
    {
      name: "起点のレコードが無い",
      response: () => jsonResponse(404, apiErrorJson("record_not_found")),
      code: "record_not_found",
    },
    {
      name: "経路を組めなかった",
      response: () => jsonResponse(500, internalErrorJson()),
      code: "internal_error",
    },
    {
      name: "起点付きの応答が経路を持たない",
      response: () => jsonResponse(200, accessLogRecordResponseJson()),
      code: undefined,
    },
  ],
)(
  "経路の要求の失敗 ($name) を経路の欄に出し、レコードを出し続ける",
  async ({ response, code }) => {
    stubRecordAndTrail(
      () => jsonResponse(200, hostALogRecordResponseJson()),
      response,
    );

    render(
      <RecordDetail
        recordRef={hostALogRecordRef}
        trailOrigin={trailOriginOf()}
      />,
    );

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("到達した経路の構築");
    if (code === undefined) {
      expect(alert.textContent).not.toContain("コード:");
    } else {
      expect(alert.textContent).toContain(`コード: ${code}`);
    }
    expect(alert.textContent).not.toContain("レコードの取得");
    expect(screen.getByRole("table", { name: "フィールド" })).toBeTruthy();
  },
);

// 端末の割当や時刻の解釈を記録すると backend はグラフを組み直す。経路だけを取り直す。
test("グラフを組み直した回数か条件が変わると、経路だけを取り直す", async () => {
  const requests = stubRecordAndTrail(
    () => jsonResponse(200, hostALogRecordResponseJson()),
    () => jsonResponse(200, hostALogRecordResponseJson()),
  );
  const { rerender } = render(
    <RecordDetail
      recordRef={hostALogRecordRef}
      trailOrigin={trailOriginOf()}
    />,
  );
  await screen.findByRole("table", { name: "到達した経路の段階" });

  rerender(
    <RecordDetail
      recordRef={hostALogRecordRef}
      trailOrigin={trailOriginOf({ dataVersion: 1 })}
    />,
  );
  await waitFor(() => expect(requests.trailUrls()).toHaveLength(2));
  rerender(
    <RecordDetail
      recordRef={hostALogRecordRef}
      trailOrigin={trailOriginOf({
        dataVersion: 1,
        matchConditions: { conditions: [{ conditionKey: "user" }] },
      })}
    />,
  );
  await waitFor(() => expect(requests.trailUrls()).toHaveLength(3));

  expect(
    requests.trailUrls()[2]?.searchParams.getAll("matchCondition"),
  ).toEqual(["user"]);
  expect(requests.recordUrls()).toHaveLength(1);
});

test("レコードの原文の書式文字と制御文字を可視の符号にして出す", async () => {
  stubFetch(jsonResponse(200, invisibleCharacterRecordResponseJson()));

  render(<RecordDetail recordRef={accessLogRecordRef} />);
  await screen.findByRole("table", { name: "フィールド" });

  const rawText = screen.getByText(
    (_, element) =>
      // 原文の連続した空白をまとめない pre で出す。
      element?.tagName === "PRE" &&
      element.textContent === `${accessLogRecordRawText}U+202EU+0009`,
  );
  expect(
    Array.from(rawText.querySelectorAll(".raw-control-code")).map(
      (code) => code.textContent,
    ),
  ).toEqual(["U+202E", "U+0009"]);
});

test("レコードの原文の HTML の文字列を文字列として出す", async () => {
  stubFetch(jsonResponse(200, htmlLikeRecordResponseJson()));

  const { container } = render(<RecordDetail recordRef={accessLogRecordRef} />);
  await screen.findByRole("table", { name: "フィールド" });

  expect(screen.getByText(htmlLikeRawText)).toBeTruthy();
  expect(container.querySelector("script")).toBeNull();
  expect(container.innerHTML).toContain("&lt;script&gt;");
});

test("取得に失敗したときは失敗した操作と次に行える操作を出す", async () => {
  stubFetch(textResponse(503, "service unavailable"));

  render(<RecordDetail recordRef={accessLogRecordRef} />);

  const alert = await screen.findByRole("alert");
  expect(within(alert).getByText("レコードの取得")).toBeTruthy();
  expect(alert.textContent).toContain("次の操作: 時間を空けて再実行");
  expect(alert.textContent).not.toContain("失敗に関わるレコード");
  expect(screen.queryByRole("table", { name: "フィールド" })).toBeNull();
});

test("指すレコードが無い失敗は、従来の次に行える操作のままである", async () => {
  stubFetch(jsonResponse(404, apiErrorJson("record_not_found")));

  render(<RecordDetail recordRef={accessLogRecordRef} />);

  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain("次の操作: 指定を直して再実行");
  expect(alert.textContent).toContain("コード: record_not_found");
  expect(alert.textContent).not.toContain("失敗に関わるレコード");
});
