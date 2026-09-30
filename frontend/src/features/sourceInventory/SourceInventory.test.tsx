// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import {
  decodeRecordLocator,
  type RecordLocator,
} from "@/shared/contracts/common";
import {
  decodeSourceIdentity,
  decodeSourcesResponse,
  memberAt,
  type SourceIdentity,
} from "@/shared/contracts/sources";
import {
  decodeTerminalAssignment,
  type TerminalAssignment,
} from "@/shared/contracts/terminalAssignments";
import type { FetchState } from "@/shared/lib/fetchState";
import { DisplayOffsetContext } from "@/shared/ui/DisplayOffset";
import {
  baselineCaseId,
  casedSourcesResponseJson,
  partlyCasedSourcesResponseJson,
} from "@/testdata/cases/caseCounts";
import { jsonResponse } from "@/testdata/http";
import {
  accessLogSha256,
  accessLogSource,
  accessLogSourceId,
  apiErrorJson,
  collectionSourcesResponseJson,
  emptySourcesResponseJson,
  sourcesResponseJson,
} from "@/testdata/sources/sourcesResponse";
import { importSpecifiedAssignmentJson } from "@/testdata/terminalAssignments/terminalAssignmentsResponse";
import { visibleText } from "@/testdata/visibleText";
import { SourceInventory } from "./SourceInventory";
import { useSourceInventory } from "./useSourceInventory";

/** 上位の画面と同じく、一覧の取得を `useSourceInventory` に任せて表を出す。 */
function InventoryUnderTest(props: {
  selectedSource: SourceIdentity | undefined;
  onSelect: (source: SourceIdentity) => void;
  onSelectRecord?: (recordRef: RecordLocator) => void;
}) {
  const { state } = useSourceInventory();
  return <SourceInventory state={state} {...props} />;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function stubFetch(result: Response | Error | Promise<Response>) {
  const mock = vi.fn(async (_input: string, _init?: RequestInit) => {
    if (result instanceof Error) {
      throw result;
    }
    return result;
  });
  vi.stubGlobal("fetch", mock);
  return mock;
}

function rowOf(table: HTMLElement, fileName: string): HTMLElement {
  const row = within(table)
    .getByRole("button", { name: fileName })
    .closest("tr");
  if (row === null) {
    throw new Error(`${fileName} の行を取得できませんでした`);
  }
  return row;
}

const inventoryTableName = "収集元";

test("読み込み中に読み込みの文を出す", async () => {
  let release: ((response: Response) => void) | undefined;
  const pending = new Promise<Response>((resolve) => {
    release = resolve;
  });
  stubFetch(pending);

  render(<InventoryUnderTest selectedSource={undefined} onSelect={vi.fn()} />);

  expect(screen.getByRole("status").textContent).toBe(
    "収集元の一覧の読み込み中",
  );
  expect(screen.queryByRole("table")).toBeNull();

  release?.(jsonResponse(200, sourcesResponseJson()));
  expect(
    await screen.findByRole("table", { name: inventoryTableName }),
  ).toBeTruthy();
});

test("成功に収集元ごとの識別と件数と公開の状態を出す", async () => {
  const mock = stubFetch(jsonResponse(200, sourcesResponseJson()));

  render(<InventoryUnderTest selectedSource={undefined} onSelect={vi.fn()} />);

  const table = await screen.findByRole("table", { name: inventoryTableName });
  expect(mock).toHaveBeenCalledTimes(1);
  expect(mock.mock.calls[0]?.[0]).toBe("/api/v0/sources");

  const accessCells = within(rowOf(table, "access.log")).getAllByRole("cell");
  expect(accessCells[1]?.textContent).toBe(accessLogSha256);
  expect(accessCells[2]?.textContent).toBe("1,200");
  expect(accessCells[3]?.textContent).toBe("1,200");
  expect(accessCells[4]?.textContent).toBe("1,200");
  expect(accessCells[5]?.textContent).toBe("—集計なし");
  expect(accessCells[8]?.textContent).toBe("全体を公開");

  const hostACells = within(rowOf(table, "host-a.log")).getAllByRole("cell");
  expect(hostACells[2]?.textContent).toBe("—未確定");
  expect(hostACells[3]?.textContent).toBe("—集計なし");
  expect(hostACells[6]?.textContent).toBe("2");
  // 公開を止めた理由は、状態のラベルの tooltip と読み上げに入れる。
  expect(hostACells[8]?.textContent).toBe("公開を停止理由: 識別子の衝突");

  expect(
    screen.getAllByRole("listitem").map((item) => item.textContent),
  ).toContain("収集元: 2");
});

test("収集の directory の取り込まなかった file を理由とともに出す", async () => {
  stubFetch(jsonResponse(200, collectionSourcesResponseJson()));

  render(<InventoryUnderTest selectedSource={undefined} onSelect={vi.fn()} />);

  const table = await screen.findByRole("table", {
    name: "取り込まなかったファイル",
  });
  const rows = within(table)
    .getAllByRole("row")
    .slice(1)
    .map((row) =>
      within(row)
        .getAllByRole("cell")
        .map((cell) => cell.textContent),
    );
  expect(rows).toEqual([
    ["triage/notes.txt", "未対応の形式", "text"],
    ["triage/empty.dat", "0 byte のファイル", "—形式なし"],
  ]);
});

test("取り込まなかった file が無い一覧に、その表を出さない", async () => {
  stubFetch(jsonResponse(200, sourcesResponseJson()));

  render(<InventoryUnderTest selectedSource={undefined} onSelect={vi.fn()} />);

  await screen.findByRole("table", { name: inventoryTableName });
  expect(
    screen.queryByRole("table", { name: "取り込まなかったファイル" }),
  ).toBeNull();
});

test("取得失敗に失敗した操作と次に行える操作を出し、空の結果として出さない", async () => {
  const mock = stubFetch(jsonResponse(500, apiErrorJson("internal_error")));

  render(<InventoryUnderTest selectedSource={undefined} onSelect={vi.fn()} />);

  const alert = await screen.findByRole("alert");
  expect(mock).toHaveBeenCalledTimes(1);
  expect(alert.textContent).toContain("収集元の一覧の取得");
  expect(alert.textContent).toContain("コード: internal_error");
  expect(screen.queryByRole("table")).toBeNull();
});

test("結果なしに取り込みが 0 件である理由を出す", async () => {
  const mock = stubFetch(jsonResponse(200, emptySourcesResponseJson()));

  render(<InventoryUnderTest selectedSource={undefined} onSelect={vi.fn()} />);

  const notice = await screen.findByText("取り込んだ収集元なし");
  expect(mock).toHaveBeenCalledTimes(1);
  expect(notice).toHaveAttribute("role", "status");
  expect(screen.queryByRole("table")).toBeNull();
  expect(screen.queryByRole("alert")).toBeNull();
});

test("行を選ぶと、選んだ収集元を上位へ渡す", async () => {
  stubFetch(jsonResponse(200, sourcesResponseJson()));
  const onSelect = vi.fn();

  render(<InventoryUnderTest selectedSource={undefined} onSelect={onSelect} />);

  const button = await screen.findByRole("button", { name: "access.log" });
  expect(button).toHaveAttribute("aria-pressed", "false");
  fireEvent.click(button);

  expect(onSelect).toHaveBeenCalledTimes(1);
  expect(onSelect.mock.calls[0]?.[0]).toMatchObject({
    sourceId: accessLogSourceId,
    fileName: "access.log",
    contentSha256: accessLogSha256,
    recordCount: 1200,
  });
});

test("選んだ収集元の path と記録期間の正規化値を出し、原文を help に入れる", async () => {
  stubFetch(jsonResponse(200, sourcesResponseJson()));
  const response = sourcesResponseJson() as { sources: unknown[] };
  const selected = decodeSourceIdentity(
    (response.sources[0] as { source: unknown }).source,
    "selected",
  );

  render(<InventoryUnderTest selectedSource={selected} onSelect={vi.fn()} />);

  const detail = await screen.findByRole("table", {
    name: "選択中の収集元: access.log",
  });
  const labels = within(detail)
    .getAllByRole("rowheader")
    .map((header) => header.textContent);
  expect(labels).toContain("記録期間");
  expect(within(detail).getByText("2031-10-08T10:20:35+09:00")).toBeTruthy();
  expect(
    within(detail).getByText("/data/example/proxy/access.log"),
  ).toBeTruthy();
  expect(within(detail).getByText("UTC 時刻なし")).toBeTruthy();
  fireEvent.click(
    within(detail).getByRole("button", { name: "記録期間 の説明" }),
  );
  expect(screen.getByRole("tooltip").textContent).toContain(
    "始まりの原文: [08/Oct/2031:10:20:35 +0900]",
  );
  expect(screen.getByRole("button", { name: "access.log" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
});

test("時刻の解釈を持つ収集元は、解釈で読んだ最初と最後の時刻を、解釈であることと一緒に出す", async () => {
  const local = (rawText: string, normalized: string) => ({
    rawText,
    normalized,
    normalizedForm: "local_without_offset",
    precision: "millisecond",
    offsetState: "item_absent",
    clock: "observer_local",
    meaning: "event",
    valueState: "present",
    interpretation: { offset: "+09:00", assertionId: "a-1" },
  });
  const response = sourcesResponseJson() as {
    sources: Array<{ source: unknown; interpretedObservedRange?: unknown }>;
  };
  response.sources[1].interpretedObservedRange = {
    from: local("2031/10/08 10:20:35.100", "2031-10-08T10:20:35.100"),
    to: local("2031/10/08 11:00:00.200", "2031-10-08T11:00:00.200"),
  };
  const selected = decodeSourceIdentity(response.sources[1].source, "selected");
  const detailName = "選択中の収集元: host-a.log";

  stubFetch(jsonResponse(200, response));
  const view = render(
    <InventoryUnderTest selectedSource={selected} onSelect={vi.fn()} />,
  );
  const detail = await screen.findByRole("table", { name: detailName });
  expect(within(detail).getByText("2031-10-08T10:20:35.100")).toBeTruthy();
  expect(within(detail).getByText("2031-10-08T11:00:00.200")).toBeTruthy();
  expect(within(detail).getAllByText("分析者の記録: UTC+09:00")).toHaveLength(
    2,
  );
  // 解釈のずれで読んだ時点を UTC で出す。
  expect(
    within(detail).getByText("UTC: 2031-10-08T01:20:35.100Z"),
  ).toBeTruthy();
  expect(
    within(detail).getByText("UTC: 2031-10-08T02:00:00.200Z"),
  ).toBeTruthy();

  // 解釈の無い一覧に取り直すと、原資料から定まる時刻だけの表示へ戻る。
  view.unmount();
  stubFetch(jsonResponse(200, sourcesResponseJson()));
  render(<InventoryUnderTest selectedSource={selected} onSelect={vi.fn()} />);
  const reverted = await screen.findByRole("table", { name: detailName });
  expect(within(reverted).queryByText(/分析者の記録/)).toBeNull();
  expect(within(reverted).getAllByText("UTC 時刻なし")).toHaveLength(2);
});

test("未知の入力形式を識別子のまま表示する", async () => {
  const response = sourcesResponseJson() as {
    sources: Array<{ source: { formatKey: string } }>;
  };
  response.sources[0].source.formatKey = "vendor_format_v1";
  stubFetch(jsonResponse(200, response));
  const selected = decodeSourceIdentity(response.sources[0].source, "selected");

  render(<InventoryUnderTest selectedSource={selected} onSelect={vi.fn()} />);

  const detail = await screen.findByRole("table", {
    name: "選択中の収集元: access.log",
  });
  expect(within(detail).getByText("vendor_format_v1")).toBeTruthy();
});

test("欄の並びの指定を、読んだ文字列のまま表示する", async () => {
  const response = sourcesResponseJson() as {
    sources: Array<{ source: { formatSpec?: string } }>;
  };
  stubFetch(jsonResponse(200, response));
  const selected = decodeSourceIdentity(response.sources[0].source, "selected");

  render(<InventoryUnderTest selectedSource={selected} onSelect={vi.fn()} />);

  const detail = await screen.findByRole("table", {
    name: "選択中の収集元: access.log",
  });
  expect(
    within(detail).getByText(
      '%>a %[ui %[un [%tl] "%rm %ru HTTP/%rv" %>Hs %<st "%{Referer}>h" "%{User-Agent}>h" %Ss:%Sh',
    ),
  ).toBeTruthy();
});

test("欄の並びの指定を取らない入力形式に、指定が無い理由を表示する", async () => {
  const response = sourcesResponseJson() as {
    sources: Array<{ source: { formatSpec?: string } }>;
  };
  delete response.sources[0].source.formatSpec;
  stubFetch(jsonResponse(200, response));
  const selected = decodeSourceIdentity(response.sources[0].source, "selected");

  render(<InventoryUnderTest selectedSource={selected} onSelect={vi.fn()} />);

  const detail = await screen.findByRole("table", {
    name: "選択中の収集元: access.log",
  });
  expect(within(detail).getByText("指定を受け付けない入力形式")).toBeTruthy();
});

test("案件を持つ収集元がある一覧は案件の列を出し、案件を持たない収集元に指定なしと出す", async () => {
  stubFetch(jsonResponse(200, partlyCasedSourcesResponseJson()));

  render(<InventoryUnderTest selectedSource={undefined} onSelect={vi.fn()} />);

  const table = await screen.findByRole("table", { name: inventoryTableName });
  expect(
    within(table).getByRole("columnheader", { name: "案件" }),
  ).toBeTruthy();
  expect(
    within(rowOf(table, "access.log")).getByRole("cell", {
      name: baselineCaseId,
    }),
  ).toBeTruthy();
  expect(
    within(rowOf(table, "host-a.log")).getByRole("cell", {
      name: "案件の指定なし",
    }),
  ).toBeTruthy();
});

test("案件を持つ収集元が無い一覧は案件の列を出さない", async () => {
  stubFetch(jsonResponse(200, sourcesResponseJson()));

  render(<InventoryUnderTest selectedSource={undefined} onSelect={vi.fn()} />);

  const table = await screen.findByRole("table", { name: inventoryTableName });
  expect(
    within(table).queryByRole("columnheader", { name: "案件" }),
  ).toBeNull();
});

test("選んだ収集元の案件を出し、案件を持たない収集元には指定なしと出す", async () => {
  stubFetch(jsonResponse(200, casedSourcesResponseJson()));
  const cased = decodeSourceIdentity(
    { ...accessLogSource(), caseId: baselineCaseId },
    "selected",
  );

  const view = render(
    <InventoryUnderTest selectedSource={cased} onSelect={vi.fn()} />,
  );

  const detail = await screen.findByRole("table", {
    name: "選択中の収集元: access.log",
  });
  const caseRow = within(detail).getByRole("rowheader", { name: "案件" });
  expect(caseRow.closest("tr")?.textContent).toBe(`案件${baselineCaseId}`);

  view.unmount();
  stubFetch(jsonResponse(200, sourcesResponseJson()));
  const plain = decodeSourceIdentity(accessLogSource(), "selected");
  render(<InventoryUnderTest selectedSource={plain} onSelect={vi.fn()} />);

  const plainDetail = await screen.findByRole("table", {
    name: "選択中の収集元: access.log",
  });
  expect(
    within(plainDetail).getByRole("rowheader", { name: "案件" }).closest("tr")
      ?.textContent,
  ).toBe("案件—案件の指定なし");
});

test("説明を組めなかったレコードの件数と端末の候補を出し、候補を持たない入力形式に理由を出す", async () => {
  const response = sourcesResponseJson() as {
    sources: Array<{
      source: Record<string, unknown>;
      messageUnrenderedRecordRefs?: unknown[];
    }>;
  };
  response.sources[0].source.messageUnrenderedCount = 2;
  response.sources[0].source.terminalCandidates = [
    { name: "HOST01$", recordCount: 3 },
    { name: "HOST02$", recordCount: 1 },
  ];
  const unrendered = {
    sourceId: accessLogSourceId,
    sourceContentSha256: accessLogSha256,
    sourceFileName: "access.log",
    positionKind: "line_number",
    lineNumber: 7,
    recordRawTextRef: "raw:access-7",
  };
  // 上限で切った応答は、件数より少ない位置を含む。
  response.sources[0].messageUnrenderedRecordRefs = [unrendered];
  stubFetch(jsonResponse(200, response));
  const counted = decodeSourceIdentity(response.sources[0].source, "selected");
  const onSelectRecord = vi.fn();

  const view = render(
    <InventoryUnderTest
      selectedSource={counted}
      onSelect={vi.fn()}
      onSelectRecord={onSelectRecord}
    />,
  );

  const detail = await screen.findByRole("table", {
    name: "選択中の収集元: access.log",
  });
  expect(
    within(detail)
      .getByRole("rowheader", { name: "説明を生成できなかったレコード" })
      .closest("tr")?.textContent,
  ).toBe("説明を生成できなかったレコード件数: 2表示: 1行: 7 ");
  fireEvent.click(
    within(
      within(detail).getByRole("list", {
        name: "説明を生成できなかったレコード",
      }),
    ).getByRole("button", { name: "レコードを開く" }),
  );
  expect(onSelectRecord).toHaveBeenCalledWith(unrendered);
  const candidates = within(
    within(detail).getByRole("table", { name: "記録した端末の候補" }),
  )
    .getAllByRole("row")
    .slice(1)
    .map((row) => row.textContent);
  expect(candidates).toEqual(["HOST01$3", "HOST02$1"]);

  view.unmount();
  stubFetch(jsonResponse(200, sourcesResponseJson()));
  const plain = decodeSourceIdentity(accessLogSource(), "selected");
  render(<InventoryUnderTest selectedSource={plain} onSelect={vi.fn()} />);
  const plainDetail = await screen.findByRole("table", {
    name: "選択中の収集元: access.log",
  });
  expect(within(plainDetail).getByText("記録しない入力形式")).toBeTruthy();
  expect(within(plainDetail).getByText("候補なし")).toBeTruthy();
});

test("説明を組めなかったレコードが 0 件の収集元には件数だけを出す", async () => {
  const response = sourcesResponseJson() as {
    sources: Array<{ source: Record<string, unknown> }>;
  };
  response.sources[0].source.messageUnrenderedCount = 0;
  stubFetch(jsonResponse(200, response));
  const selected = decodeSourceIdentity(response.sources[0].source, "selected");

  render(<InventoryUnderTest selectedSource={selected} onSelect={vi.fn()} />);

  const detail = await screen.findByRole("table", {
    name: "選択中の収集元: access.log",
  });
  expect(
    within(detail)
      .getByRole("rowheader", { name: "説明を生成できなかったレコード" })
      .closest("tr")?.textContent,
  ).toBe("説明を生成できなかったレコード0");
});

test("選んだ収集元の取り込めなかったレコードの位置と理由を出す", async () => {
  const response = sourcesResponseJson() as {
    sources: Array<{ source: unknown }>;
  };
  stubFetch(jsonResponse(200, response));
  const selected = decodeSourceIdentity(response.sources[1].source, "selected");

  render(<InventoryUnderTest selectedSource={selected} onSelect={vi.fn()} />);

  const detail = await screen.findByRole("table", {
    name: "選択中の収集元: host-a.log",
  });
  expect(
    within(detail)
      .getByRole("rowheader", { name: "取り込めなかったレコード" })
      .closest("tr")?.textContent,
  ).toBe("取り込めなかったレコード2");
  const failures = screen.getByRole("table", {
    name: "取り込めなかったレコード",
  });
  const rows = within(failures).getAllByRole("row");
  expect(rows).toHaveLength(3);
  const cells = within(rows[1]).getAllByRole("cell");
  expect(cells.map((cell) => cell.textContent)).toEqual([
    "ID: 112",
    "―",
    "ノードとエッジの作成",
    "each record identifier points at one record",
    "two records carry the same identifier",
    "未判定the source does not carry a tie-breaking item",
    "",
  ]);
  expect(
    within(cells[6]).getByRole("button", { name: "原文を開く" }),
  ).toBeTruthy();
  expect(within(rows[2]).getByText("ID: 113")).toBeTruthy();
});

test("取り込めなかったレコードの行から原文を開き、途中で切れた行は理由と読めた欄を出す", async () => {
  const response = sourcesResponseJson() as {
    sources: Array<{
      source: unknown;
      importStatus: { failures: Array<Record<string, unknown>> };
    }>;
  };
  response.sources[1].importStatus.failures[1].recordTruncated = true;
  let releaseFirst: (() => void) | undefined;
  let rawTextCalls = 0;
  const fetch = vi.fn(async (input: string) => {
    const url = String(input);
    if (!url.includes("/raw-texts")) {
      return jsonResponse(200, response);
    }
    rawTextCalls++;
    if (rawTextCalls === 1) {
      // 先に開いた行の応答を、後に開いた行の応答より遅らせる。
      await new Promise<void>((resolve) => {
        releaseFirst = resolve;
      });
    }
    if (url.includes("host-a-113")) {
      return jsonResponse(404, {
        code: "record_not_found",
        message: "no published record has the raw text reference",
      });
    }
    return jsonResponse(200, {
      rawTextRef: "raw:host-a-112",
      rawText: "synthetic line\u0007",
    });
  });
  vi.stubGlobal("fetch", fetch);
  const selected = decodeSourceIdentity(response.sources[1].source, "selected");
  const onSelectRecord = vi.fn();

  render(
    <InventoryUnderTest
      selectedSource={selected}
      onSelect={vi.fn()}
      onSelectRecord={onSelectRecord}
    />,
  );

  const failures = await screen.findByRole("table", {
    name: "取り込めなかったレコード",
  });
  const rows = within(failures).getAllByRole("row");
  expect(rows[2]).toHaveTextContent(/途中で切れた行/);
  fireEvent.click(
    within(rows[2]).getByRole("button", { name: "読み取れたフィールドを開く" }),
  );
  expect(onSelectRecord).toHaveBeenCalledWith(
    decodeRecordLocator(
      response.sources[1].importStatus.failures[1].recordRef,
      "expected",
    ),
  );
  expect(
    within(rows[1]).queryByRole("button", {
      name: "読み取れたフィールドを開く",
    }),
  ).toBeNull();

  // 1 行目を開いた応答が届く前に 2 行目を開くと、2 行目の失敗だけを出す。
  fireEvent.click(within(rows[1]).getByRole("button", { name: "原文を開く" }));
  fireEvent.click(within(rows[2]).getByRole("button", { name: "原文を開く" }));
  const opened = await screen.findByRole("region", { name: "開いた原文" });
  expect(await within(opened).findByRole("alert")).toBeTruthy();
  releaseFirst?.();
  await new Promise((resolve) => setTimeout(resolve, 20));
  expect(within(opened).queryByText(/synthetic line/)).toBeNull();

  // 開き直すと原文を出す。
  fireEvent.click(within(rows[1]).getByRole("button", { name: "原文を開く" }));
  const reopened = await screen.findByRole("region", { name: "開いた原文" });
  expect(await within(reopened).findByText(/synthetic line/)).toBeTruthy();
  const url = new URL(String(fetch.mock.calls.at(-1)?.[0]), "http://localhost");
  expect(url.pathname).toBe("/api/v0/raw-texts");
  expect(url.searchParams.get("ref")).toBe("raw:host-a-112");
});

test("行番号で指す途中で切れた行の読めた欄は、行番号だけで開く", async () => {
  const response = sourcesResponseJson() as {
    sources: Array<{
      source: unknown;
      importStatus: { failures: Array<Record<string, unknown>> };
    }>;
  };
  const failure = response.sources[1].importStatus.failures[1];
  failure.recordTruncated = true;
  // 取り込めなかった行の位置は、行の byte 範囲も含む。
  failure.recordRef = {
    ...(failure.recordRef as Record<string, unknown>),
    positionKind: "line_number",
    sequenceNumber: undefined,
    lineNumber: 7,
    byteOffset: 300,
    byteLength: 40,
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse(200, response)),
  );
  const selected = decodeSourceIdentity(response.sources[1].source, "selected");
  const onSelectRecord = vi.fn();

  render(
    <InventoryUnderTest
      selectedSource={selected}
      onSelect={vi.fn()}
      onSelectRecord={onSelectRecord}
    />,
  );

  const failures = await screen.findByRole("table", {
    name: "取り込めなかったレコード",
  });
  fireEvent.click(
    within(failures).getByRole("button", {
      name: "読み取れたフィールドを開く",
    }),
  );
  const opened = onSelectRecord.mock.calls[0]?.[0];
  expect(opened).toMatchObject({ positionKind: "line_number", lineNumber: 7 });
  expect(opened.byteOffset).toBeUndefined();
});

test("取り込めなかったレコードが 0 件の収集元に 0 件と出し、失敗の表を出さない", async () => {
  const response = sourcesResponseJson() as {
    sources: Array<{ source: unknown }>;
  };
  stubFetch(jsonResponse(200, response));
  const selected = decodeSourceIdentity(response.sources[0].source, "selected");

  render(<InventoryUnderTest selectedSource={selected} onSelect={vi.fn()} />);

  const detail = await screen.findByRole("table", {
    name: "選択中の収集元: access.log",
  });
  expect(
    within(detail)
      .getByRole("rowheader", { name: "取り込めなかったレコード" })
      .closest("tr")?.textContent,
  ).toBe("取り込めなかったレコード0");
  expect(
    screen.queryByRole("table", { name: "取り込めなかったレコード" }),
  ).toBeNull();
});

test("取り込めなかったレコードの原因の分類を持たない要素を読まない", () => {
  const response = sourcesResponseJson() as {
    sources: Array<{
      importStatus: { failures: Array<Record<string, unknown>> };
    }>;
  };
  delete response.sources[1].importStatus.failures[0].diagnosisClass;

  expect(() => decodeSourcesResponse(response, "response")).toThrow(
    "diagnosisClass",
  );
});

test("原因の分類が未判定なのに理由を持たない要素を読まない", () => {
  const response = sourcesResponseJson() as {
    sources: Array<{
      importStatus: { failures: Array<Record<string, unknown>> };
    }>;
  };
  delete response.sources[1].importStatus.failures[0].unresolvedReason;

  expect(() => decodeSourcesResponse(response, "response")).toThrow(
    "unresolvedReason",
  );
});

test("取り込めなかったレコードが 21 件以上の収集元は、失敗の表を閉じて出す", async () => {
  const response = sourcesResponseJson() as {
    sources: Array<{
      source: unknown;
      importStatus: {
        failures: unknown[];
        failureCount: number;
        diagnosisCounts: Array<{ count: number }>;
      };
    }>;
  };
  const status = response.sources[1].importStatus;
  status.failures = Array.from({ length: 21 }, () => status.failures[0]);
  status.failureCount = 21;
  status.diagnosisCounts[0].count = 21;
  stubFetch(jsonResponse(200, response));
  const selected = decodeSourceIdentity(response.sources[1].source, "selected");

  render(<InventoryUnderTest selectedSource={selected} onSelect={vi.fn()} />);

  const summary = await screen.findByText("取り込めなかったレコード: 21");
  expect(summary.closest("details")).not.toHaveAttribute("open");
  expect(
    screen.queryByRole("table", { name: "取り込めなかったレコード" }),
  ).toBeNull();

  fireEvent.click(summary);

  expect(
    await screen.findByRole("table", { name: "取り込めなかったレコード" }),
  ).toBeTruthy();
});

test("空の入力形式識別子を受け付けない", () => {
  const response = sourcesResponseJson() as {
    sources: Array<{ source: { formatKey: string } }>;
  };
  response.sources[0].source.formatKey = "";

  expect(() =>
    decodeSourceIdentity(response.sources[0].source, "source"),
  ).toThrow("source.formatKey: expected a non-empty string");
});

/** file の見出しの値の項目。 */
function headerField(name: string, rawText: string) {
  return {
    name,
    kind: "text",
    text: { rawText, valueState: "present" },
  };
}

test("file の見出しを持つ収集元は、次のレコード番号と dirty の印の行を出す", async () => {
  const response = sourcesResponseJson() as {
    sources: Array<{ source: Record<string, unknown> }>;
  };
  response.sources[0].source.fileHeader = [
    headerField("NextRecordID", "701"),
    headerField("Dirty", "true"),
  ];
  stubFetch(jsonResponse(200, response));
  const selected = decodeSourceIdentity(response.sources[0].source, "selected");
  expect(selected.fileHeader?.map((field) => field.name)).toEqual([
    "NextRecordID",
    "Dirty",
  ]);

  render(<InventoryUnderTest selectedSource={selected} onSelect={vi.fn()} />);

  const detail = await screen.findByRole("table", {
    name: "選択中の収集元: access.log",
  });
  expect(
    within(detail)
      .getByRole("rowheader", { name: "次のレコード番号" })
      .closest("tr")?.textContent,
  ).toBe("次のレコード番号701");
  expect(
    within(detail)
      .getByRole("rowheader", { name: "Dirty フラグ" })
      .closest("tr")?.textContent,
  ).toBe("Dirty フラグあり");
});

test("file の見出しを持たない収集元は、見出しの行を出さない", async () => {
  stubFetch(jsonResponse(200, sourcesResponseJson()));
  const response = sourcesResponseJson() as { sources: unknown[] };
  const selected = decodeSourceIdentity(
    (response.sources[0] as { source: unknown }).source,
    "selected",
  );
  expect(selected.fileHeader).toBeUndefined();

  render(<InventoryUnderTest selectedSource={selected} onSelect={vi.fn()} />);

  const detail = await screen.findByRole("table", {
    name: "選択中の収集元: access.log",
  });
  expect(
    within(detail).queryByRole("rowheader", { name: "次のレコード番号" }),
  ).toBeNull();
});

/** 主 file 600 byte と付属の file 424 byte の構成。 */
function syntheticMembers() {
  return [
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
}

test("構成する file を持つ収集元は、file ごとの位置と sha256 と log の適用の結果を出す", async () => {
  const response = sourcesResponseJson() as {
    sources: Array<{ source: Record<string, unknown> }>;
  };
  response.sources[0].source.members = syntheticMembers();
  response.sources[0].source.fileHeader = [
    headerField("Recovery", "not_applied"),
    headerField("Log.hive.LOG1", "1 entries, sequence 7-7; applied 7-7"),
    headerField("Log.hive.LOG2", "absent"),
    {
      name: "LastWrittenTime",
      kind: "timestamp",
      timestamp: {
        rawText: "126256467060000000",
        normalized: "2001-02-03T04:05:06.000000Z",
        normalizedForm: "rfc3339_absolute",
        precision: "microsecond",
        offsetState: "epoch",
        clock: "file_property",
        meaning: "property",
        valueState: "present",
      },
    },
  ];
  stubFetch(jsonResponse(200, response));
  const selected = decodeSourceIdentity(response.sources[0].source, "selected");

  render(<InventoryUnderTest selectedSource={selected} onSelect={vi.fn()} />);

  const detail = await screen.findByRole("table", {
    name: "選択中の収集元: access.log",
  });
  const members = within(detail).getByRole("table", {
    name: "構成するファイル",
  });
  expect(
    within(members)
      .getAllByRole("row")
      .slice(1)
      .map((row) =>
        within(row)
          .getAllByRole("cell")
          .map((cell) => cell.textContent),
      ),
  ).toEqual([
    ["logs/hive", "0-600", "a".repeat(64)],
    ["logs/hive.LOG1", "600-1024", "b".repeat(64)],
  ]);
  expect(
    within(detail)
      .getByRole("rowheader", { name: "transaction log の適用" })
      .closest("tr")?.textContent,
  ).toBe("transaction log の適用適用できない");
  fireEvent.click(
    within(detail).getByRole("button", { name: "適用できない の説明" }),
  );
  expect(screen.getByRole("tooltip").textContent).toBe(
    "key と値の状態: 主ファイルの書き込みの途中",
  );
  const rowText = (name: string) =>
    visibleText(within(detail).getByRole("rowheader", { name }).closest("tr"));
  expect(rowText("transaction log hive.LOG1 の状態")).toBe(
    "transaction log hive.LOG1 の状態1 entries, sequence 7-7; applied 7-7",
  );
  expect(rowText("transaction log hive.LOG2 の状態")).toBe(
    "transaction log hive.LOG2 の状態主ファイルと同じ directory になし",
  );
  expect(rowText("最終書き込みの時刻")).toBe(
    "最終書き込みの時刻2001-02-03T04:05:06.000000Z",
  );
  expect(
    within(detail).getByText("2001-02-03T04:05:06.000000Z").closest("[title]"),
  ).toHaveAttribute("title", "原文: 126256467060000000");
});

test("表示のずれを選ぶと、file の見出しが記録した時点に地方時を添える", async () => {
  const response = sourcesResponseJson() as {
    sources: Array<{ source: Record<string, unknown> }>;
  };
  response.sources[0].source.fileHeader = [
    {
      name: "LastWrittenTime",
      kind: "timestamp",
      timestamp: {
        rawText: "126256467060000000",
        normalized: "2001-02-03T04:05:06.000000Z",
        normalizedForm: "rfc3339_absolute",
        precision: "microsecond",
        offsetState: "epoch",
        clock: "file_property",
        meaning: "property",
        valueState: "present",
      },
    },
  ];
  stubFetch(jsonResponse(200, response));
  const selected = decodeSourceIdentity(response.sources[0].source, "selected");

  render(
    <DisplayOffsetContext.Provider value="+09:00">
      <InventoryUnderTest selectedSource={selected} onSelect={vi.fn()} />
    </DisplayOffsetContext.Provider>,
  );

  const detail = await screen.findByRole("table", {
    name: "選択中の収集元: access.log",
  });
  expect(
    within(detail)
      .getByRole("rowheader", { name: "最終書き込みの時刻" })
      .closest("tr")?.textContent,
  ).toContain("UTC+09:00: 2001-02-03T13:05:06.000000+09:00");
});

test("構成する file の一覧が空の収集元は、構成する file の行を出さない", async () => {
  const response = sourcesResponseJson() as {
    sources: Array<{ source: Record<string, unknown> }>;
  };
  response.sources[0].source.members = [];
  stubFetch(jsonResponse(200, response));
  const selected = decodeSourceIdentity(response.sources[0].source, "selected");

  render(<InventoryUnderTest selectedSource={selected} onSelect={vi.fn()} />);

  const detail = await screen.findByRole("table", {
    name: "選択中の収集元: access.log",
  });
  expect(
    within(detail).queryByRole("table", { name: "構成するファイル" }),
  ).toBeNull();
});

test("構成する file の位置を byte 位置から求める", () => {
  const members = syntheticMembers();
  expect(memberAt(members, 0)?.originPath).toBe("logs/hive");
  expect(memberAt(members, 599)?.originPath).toBe("logs/hive");
  expect(memberAt(members, 600)?.originPath).toBe("logs/hive.LOG1");
  expect(memberAt(members, 1024)).toBeUndefined();
  expect(memberAt(undefined, 0)).toBeUndefined();
});

test("構成する file の項目の形が壊れた応答を読まない", () => {
  const response = sourcesResponseJson() as {
    sources: Array<{ source: Record<string, unknown> }>;
  };
  response.sources[0].source.members = [{ originPath: "logs/hive" }];

  expect(() =>
    decodeSourceIdentity(response.sources[0].source, "source"),
  ).toThrow("source.members[0]");
});

/** 割当の取得の状態を渡して、選んだ収集元の「起動で指定した端末」の行を返す。 */
async function specifiedTerminalRow(
  terminalAssignments: FetchState<TerminalAssignment[]>,
): Promise<HTMLElement | null> {
  stubFetch(jsonResponse(200, sourcesResponseJson()));
  const selected = decodeSourceIdentity(accessLogSource(), "selected");
  function WithAssignments() {
    const { state } = useSourceInventory();
    return (
      <SourceInventory
        state={state}
        selectedSource={selected}
        onSelect={vi.fn()}
        terminalAssignments={terminalAssignments}
      />
    );
  }
  render(<WithAssignments />);
  const detail = await screen.findByRole("table", {
    name: "選択中の収集元: access.log",
  });
  return (
    within(detail)
      .queryByRole("rowheader", { name: "サーバーの起動で指定した端末" })
      ?.closest("tr") ?? null
  );
}

function specifiedAssignment(
  items: Partial<TerminalAssignment>,
): TerminalAssignment {
  return {
    ...decodeTerminalAssignment(importSpecifiedAssignmentJson(), "$"),
    sourceId: accessLogSourceId,
    appliesToSourceId: accessLogSourceId,
    ...items,
  };
}

test("起動で指定した端末を、選んだ収集元の詳細に出す", async () => {
  const specified = specifiedAssignment({ terminalId: "host-a" });
  const other = specifiedAssignment({
    terminalId: "host-z",
    appliesToSourceId: "another-source",
  });
  const row = await specifiedTerminalRow({
    status: "loaded",
    value: [specified, other],
  });
  expect(row?.textContent).toBe(
    "サーバーの起動で指定した端末端末 ID: host-aホスト名: host-a.example.test",
  );
});

test("起動で指定した端末があるときは、レコードが記録した端末の名前を端末に決めていない候補と出さない", async () => {
  await specifiedTerminalRow({
    status: "loaded",
    value: [specifiedAssignment({ terminalId: "host-a" })],
  });
  expect(screen.queryByRole("rowheader", { name: "端末の候補" })).toBeNull();
  expect(
    screen.getByRole("rowheader", { name: "レコードが記録した端末の名前" }),
  ).toBeTruthy();
});

test("ホスト名だけの割当と IP 付きの割当を、起動で指定した端末に出す", async () => {
  const hostnameOnly = specifiedAssignment({
    terminalId: undefined,
    terminalHostname: "ws-b.example.test",
    clientIp: undefined,
  });
  const withIp = specifiedAssignment({
    terminalId: "host-c",
    terminalHostname: undefined,
    clientIp: "192.0.2.30",
  });
  const row = await specifiedTerminalRow({
    status: "loaded",
    value: [hostnameOnly, withIp],
  });
  expect(row?.textContent).toBe(
    "サーバーの起動で指定した端末ホスト名: ws-b.example.test端末 ID: host-cIP: 192.0.2.30",
  );
});

test("ホスト名だけの割当が複数あっても、要素の key が重ならない", async () => {
  const errors = vi.spyOn(console, "error").mockImplementation(() => {});
  const row = await specifiedTerminalRow({
    status: "loaded",
    value: ["ws-d.example.test", "ws-e.example.test"].map((hostname) =>
      specifiedAssignment({
        terminalId: undefined,
        terminalHostname: hostname,
        clientIp: undefined,
      }),
    ),
  });
  expect(row?.textContent).toBe(
    "サーバーの起動で指定した端末ホスト名: ws-d.example.testホスト名: ws-e.example.test",
  );
  expect(errors).not.toHaveBeenCalled();
  errors.mockRestore();
});

test("割当を読み込み中・読めなかったときは、起動で指定した端末の行でそれを出す", async () => {
  const loading = await specifiedTerminalRow({ status: "loading" });
  expect(loading?.textContent).toContain("読み込み中");
  cleanup();
  const failed = await specifiedTerminalRow({
    status: "failed",
    failure: {
      kind: "server",
      summary: "割当を読めませんでした",
      nextAction: "再読み込みしてください",
    },
  });
  expect(failed?.textContent).toContain("読み込み失敗");
});

test("file の見出しの項目の形が壊れた応答を読まない", () => {
  const response = sourcesResponseJson() as {
    sources: Array<{ source: Record<string, unknown> }>;
  };
  response.sources[0].source.fileHeader = [{ name: "Dirty" }];

  expect(() =>
    decodeSourceIdentity(response.sources[0].source, "source"),
  ).toThrow("source.fileHeader[0]");
});
