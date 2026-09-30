// @vitest-environment jsdom
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, onTestFinished, test, vi } from "vitest";
import { decodeNodeDetailResponse } from "@/shared/contracts/graphDetail";
import { recordPositionValue } from "@/shared/lib/recordPosition";
import { searchHighlightOf } from "@/shared/lib/searchHighlight";
import { noSearchTerms } from "@/shared/lib/searchTerms";
import { DisplayOffsetContext } from "@/shared/ui/DisplayOffset";
import { SearchHighlightContext } from "@/shared/ui/Highlighted";
import {
  createdNodeDetailResponseJson,
  nodeDetailResponseJson,
  rejectedLogonNodeDetailResponseJson,
} from "@/testdata/graph/graphResponse";
import { visibleText } from "@/testdata/visibleText";
import { findPair, getPair, queryPair } from "./graphExploreTestHarness";
import { NodeDetail, type NodeDetailActions } from "./NodeDetail";

afterEach(() => {
  cleanup();
});

/** 記録したレコードの表の、レコード 1 件ずつの行。見出しの行を除く。 */
function recordsOf(table: HTMLElement): HTMLElement[] {
  return within(table).getAllByRole("row").slice(1);
}

/** 行の中の、列の番号 index の欄。 */
function cellOf(row: HTMLElement | undefined, index: number): HTMLElement {
  const cell = within(row as HTMLElement).getAllByRole("cell")[index];
  if (cell === undefined) throw new Error(`列 ${index} が無い`);
  return cell;
}

/** 時刻の欄の、説明の文 (title) を持つ要素。 */
function timeFactsOf(row: HTMLElement | undefined): string {
  return cellOf(row, 1).querySelector("[title]")?.getAttribute("title") ?? "";
}

/** fixture のノードの詳細を、種別 kind に置き換えて読む。 */
function detailOf(kind: "process" | "terminal" | "account") {
  const json = nodeDetailResponseJson();
  return decodeNodeDetailResponse(
    {
      ...json,
      node: {
        ...json.node,
        kind,
        creationRecord:
          kind === "process" ? json.node.creationRecord : "item_absent",
      },
      creationRecords: kind === "process" ? json.creationRecords : [],
      creationRecordCount: kind === "process" ? json.creationRecordCount : 0,
    },
    "fixture",
  );
}

function actionsSpy(): NodeDetailActions {
  return {
    onAddTerm: vi.fn(),
    onNarrowToTerminal: vi.fn(),
    onTraceLineage: vi.fn(),
    onShowNeighbours: vi.fn(),
  };
}

function renderDetail(
  kind: "process" | "terminal" | "account",
  actions: NodeDetailActions | undefined,
) {
  const detail = detailOf(kind);
  render(
    <NodeDetail
      state={{ status: "loaded", value: detail }}
      onSelectRecord={() => {}}
      assertions={() => null}
      actions={actions}
    />,
  );
  return detail;
}

test("アカウントから役割付き記録を開く操作へ識別子を渡す", () => {
  const actions = { ...actionsSpy(), onShowAccountRecords: vi.fn() };
  const detail = renderDetail("account", actions);
  fireEvent.click(
    screen.getByRole("button", { name: "このアカウントを名指した記録" }),
  );
  expect(actions.onShowAccountRecords).toHaveBeenCalledWith({
    id: detail.node.id,
    label: detail.node.label.rawText,
    kind: "account",
  });
});

test("プロセスのノードから、親子の連鎖をたどる操作へ、ノードの参照を渡す", () => {
  const actions = actionsSpy();
  const detail = renderDetail("process", actions);

  fireEvent.click(
    screen.getByRole("button", { name: "プロセスの親子関係を表示" }),
  );

  expect(actions.onTraceLineage).toHaveBeenCalledTimes(1);
  expect(actions.onTraceLineage).toHaveBeenCalledWith({
    id: detail.node.id,
    label: detail.node.label.rawText,
    kind: "process",
  });
  expect(
    screen.queryByRole("button", {
      name: "この端末でフィルタ",
    }),
  ).toBeNull();
});

test("端末のノードから、その端末のレコードだけを残すフィルタを適用する操作へ、ノードの参照を渡す", () => {
  const actions = actionsSpy();
  const detail = renderDetail("terminal", actions);

  fireEvent.click(
    screen.getByRole("button", {
      name: "この端末でフィルタ",
    }),
  );

  expect(actions.onNarrowToTerminal).toHaveBeenCalledWith({
    id: detail.node.id,
    label: detail.node.label.rawText,
    kind: "terminal",
  });
  expect(
    screen.queryByRole("button", { name: "プロセスの親子関係を表示" }),
  ).toBeNull();
});

test("参照だけの端末のノードには、その端末のレコードだけを残すフィルタを適用する操作を出さない", () => {
  const detail = detailOf("terminal");
  render(
    <NodeDetail
      state={{
        status: "loaded",
        value: {
          ...detail,
          node: { ...detail.node, observation: "referenced" },
        },
      }}
      onSelectRecord={() => {}}
      assertions={() => null}
      actions={actionsSpy()}
    />,
  );

  expect(
    screen.queryByRole("button", {
      name: "この端末でフィルタ",
    }),
  ).toBeNull();
});

test("関係先を足す操作を渡したときだけ、足す button を出し、ノードの参照を渡す", () => {
  const actions = { ...actionsSpy(), onAddNeighbours: vi.fn() };
  const detail = renderDetail("terminal", actions);
  const reference = {
    id: detail.node.id,
    label: detail.node.label.rawText,
    kind: "terminal",
  };

  fireEvent.click(screen.getByRole("button", { name: "隣接ノードだけを表示" }));
  expect(actions.onShowNeighbours).toHaveBeenCalledWith(reference);
  fireEvent.click(screen.getByRole("button", { name: "隣接ノードを追加" }));
  expect(actions.onAddNeighbours).toHaveBeenCalledWith(reference);
});

test("関係先を足す操作を渡さないときは、足す button を出さない", () => {
  renderDetail("terminal", actionsSpy());

  expect(
    screen.getByRole("button", { name: "隣接ノードだけを表示" }),
  ).toBeTruthy();
  expect(screen.queryByRole("button", { name: "隣接ノードを追加" })).toBeNull();
});

test("識別鍵の値と属性の原資料の文字列を、含む条件に足す", () => {
  const actions = actionsSpy();
  const detail = renderDetail("process", actions);
  const identity = detail.node.identity[0]?.value ?? "";
  const attributeText = detail.attributes[0]?.values[0]?.field;
  const rawText =
    attributeText?.kind === "text" ? attributeText.text.rawText : undefined;

  fireEvent.click(
    screen.getByRole("button", { name: `含む条件に追加: ${identity}` }),
  );
  expect(actions.onAddTerm).toHaveBeenLastCalledWith(identity);

  expect(rawText).toBeDefined();
  const [attributeButton] = screen.getAllByRole("button", {
    name: `含む条件に追加: ${rawText}`,
  });
  attributeButton?.click();
  expect(actions.onAddTerm).toHaveBeenLastCalledWith(rawText);
});

test("識別鍵の値と属性の原資料の文字列にコピーボタンを出す", () => {
  const actions = actionsSpy();
  const detail = renderDetail("process", actions);
  const identity = detail.node.identity[0]?.value ?? "";
  const attributeText = detail.attributes[0]?.values[0]?.field;
  const rawText =
    attributeText?.kind === "text" ? attributeText.text.rawText : undefined;

  expect(
    screen.getByRole("button", { name: `コピー: ${identity}` }),
  ).toBeTruthy();

  expect(rawText).toBeDefined();
  const [copyAttributeButton] = screen.getAllByRole("button", {
    name: `コピー: ${rawText}`,
  });
  expect(copyAttributeButton).toBeTruthy();
});

test("Node Detail は欄を指定した文字列の一致を属性値に強調する", () => {
  const detail = detailOf("process");
  const { container } = render(
    <SearchHighlightContext.Provider
      value={searchHighlightOf(
        {
          ...noSearchTerms,
          fieldContains: ["process.command_line=whoami"],
        },
        undefined,
      )}
    >
      <NodeDetail
        state={{ status: "loaded", value: detail }}
        onSelectRecord={() => {}}
        assertions={() => null}
      />
    </SearchHighlightContext.Provider>,
  );

  expect(
    [...container.querySelectorAll("mark")].map((mark) => mark.textContent),
  ).toEqual(["whoami"]);
});

test("操作を渡さないときは、次の検索へ移る button を出さない", () => {
  renderDetail("process", undefined);

  expect(
    screen.queryByRole("button", { name: "プロセスの親子関係を表示" }),
  ).toBeNull();
  expect(
    screen.queryByRole("button", { name: /^含む条件に追加: / }),
  ).toBeNull();
});

test("記録したレコードの表は Artifact・時刻・Event ID を出し、Event ID の欄が無い観測の種別は無いことを出す", () => {
  const json = nodeDetailResponseJson();
  const [first, second] = json.evidence;
  const withEventId = {
    ...first,
    observationKind: {
      ...first?.observationKind,
      raw: [
        {
          name: "EventID",
          kind: "text",
          text: { rawText: "9001", valueState: "present" },
        },
      ],
    },
  };
  render(
    <NodeDetail
      state={{
        status: "loaded",
        value: decodeNodeDetailResponse(
          { ...json, evidence: [withEventId, second] },
          "fixture",
        ),
      }}
      onSelectRecord={() => {}}
      assertions={() => null}
    />,
  );
  const table = screen.getByRole("table", { name: /ノードを記録したレコード/ });
  const headers = within(table)
    .getAllByRole("columnheader")
    .map((cell) => visibleText(cell));
  expect(headers).toEqual([
    "Artifact",
    "時刻",
    "Event ID",
    "位置",
    "原文の時刻",
    "イベントの種類",
  ]);
  // 見出しの説明は見出しの中の値の title に入れる。
  expect(
    within(table)
      .getByRole("columnheader", { name: /^時刻/ })
      .querySelector("[title]")
      ?.getAttribute("title"),
  ).toBe("イベントの UTC 時刻");
  const [row1, row2] = recordsOf(table);
  // Event ID の列に出し、観測の種別の列には重ねて出さない。
  expect(visibleText(cellOf(row1, 2))).toBe("9001");
  expect(cellOf(row1, 5).textContent).not.toContain("9001");
  expect(cellOf(row2, 2).textContent).toContain("Event ID なし");
});

test("記録したレコードの表の時刻は、秒より下を記録された桁のまま出し、地方時を選ぶと地方時を添える", () => {
  const json = nodeDetailResponseJson();
  const [first, second] = json.evidence;
  const precise = {
    ...first,
    eventTime: {
      rawText: "2031-10-08T01:20:35.1234567Z",
      normalized: "2031-10-08T01:20:35.123456Z",
      normalizedForm: "rfc3339_absolute",
      precision: "microsecond",
      offsetState: "in_value",
      offsetText: "Z",
      clock: "terminal_local",
      meaning: "event",
      valueState: "present",
    },
  };
  render(
    <DisplayOffsetContext.Provider value="+09:00">
      <NodeDetail
        state={{
          status: "loaded",
          value: decodeNodeDetailResponse(
            { ...json, evidence: [precise, second] },
            "fixture",
          ),
        }}
        onSelectRecord={() => {}}
        assertions={() => null}
      />
    </DisplayOffsetContext.Provider>,
  );
  const table = screen.getByRole("table", { name: /ノードを記録したレコード/ });
  const [row] = recordsOf(table);
  // UTC を日付と時刻の 1 行で出し、表示のタイムゾーンの時刻は title に入れる。
  // 秒より細かい精度は画面の文字にせず、title に入れる。
  expect(visibleText(cellOf(row, 1))).toBe("2031-10-08 01:20:35.123456");
  expect(timeFactsOf(row)).toContain("精度: マイクロ秒");
  expect(timeFactsOf(row)).toContain("UTC: 2031-10-08T01:20:35.123456Z");
  expect(timeFactsOf(row)).toContain(
    "UTC+09:00: 2031-10-08T10:20:35.123456+09:00",
  );
});

test("記録したレコードの表は、行ごとにレコード位置を出し、時刻の原文を原文の時刻の列に出す", () => {
  const response = decodeNodeDetailResponse(
    nodeDetailResponseJson(),
    "fixture",
  );
  render(
    <NodeDetail
      state={{ status: "loaded", value: response }}
      onSelectRecord={() => {}}
      assertions={() => null}
    />,
  );
  const table = screen.getByRole("table", { name: /ノードを記録したレコード/ });
  const [row] = recordsOf(table);
  const [evidence] = response.evidence;
  if (evidence === undefined) throw new Error("fixture に根拠が無い");
  // 位置の列は値だけを出す。見出しが「位置」を示す。
  expect(cellOf(row, 3).textContent).toBe(
    recordPositionValue(evidence.recordRef),
  );
  expect(cellOf(row, 3).textContent).not.toContain("ID:");
  expect(cellOf(row, 4).textContent).toBe("2031/10/08 10:20:35.100");
});

test("PID の区間の始まりの値に、読んだずれで求めた UTC と、ずれの出どころを添える", () => {
  const json = nodeDetailResponseJson();
  const start = "2031-10-08T10:20:35";
  render(
    <NodeDetail
      state={{
        status: "loaded",
        value: decodeNodeDetailResponse(
          {
            ...json,
            node: {
              ...json.node,
              kind: "process",
              keyForm: "terminal_id_process_pid_interval",
              identity: [
                { semantic: "terminal.id", value: "host-a" },
                { semantic: "process.pid", value: "4321" },
                { value: start },
              ],
              intervalStart: {
                rawText: "2031/10/08 10:20:35",
                normalized: start,
                normalizedForm: "local_without_offset",
                precision: "second",
                offsetState: "undetermined",
                clock: "terminal_local",
                meaning: "event",
                valueState: "present",
                interpretation: { offset: "+09:00" },
              },
            },
          },
          "fixture",
        ),
      }}
      onSelectRecord={() => {}}
      assertions={() => null}
    />,
  );
  const identity = within(
    screen.getByRole("table", { name: "同一性" }),
  ).getAllByRole("listitem");
  expect(identity[2]?.textContent).toContain(start);
  expect(identity[2]?.textContent).toContain("UTC: 2031-10-08T01:20:35Z");
  expect(identity[2]?.textContent).toContain("起動時の指定: UTC+09:00");
  // 端末の ID と PID の値には時刻の注記を付けない。
  expect(identity[1]?.textContent).not.toContain("起動時の指定");
});

test("記録したレコードの表の時刻は、分析者の記録したタイムゾーンで読んだことと、タイムゾーン不明であることを出す", () => {
  const json = nodeDetailResponseJson();
  const [first, second] = json.evidence;
  const local = {
    rawText: "2031/10/08 10:20:35",
    normalized: "2031-10-08T10:20:35",
    normalizedForm: "local_without_offset",
    precision: "second",
    clock: "terminal_local",
    meaning: "event",
    valueState: "present",
  };
  const interpreted = {
    ...first,
    eventTime: {
      ...local,
      offsetState: "item_absent",
      interpretation: { offset: "+09:00", assertionId: "as-synthetic" },
    },
  };
  const undetermined = {
    ...second,
    eventTime: { ...local, offsetState: "undetermined" },
  };
  render(
    <NodeDetail
      state={{
        status: "loaded",
        value: decodeNodeDetailResponse(
          { ...json, evidence: [interpreted, undetermined] },
          "fixture",
        ),
      }}
      onSelectRecord={() => {}}
      assertions={() => null}
    />,
  );
  const table = screen.getByRole("table", { name: /ノードを記録したレコード/ });
  const [row1, row2] = recordsOf(table);
  const interpretedTime = cellOf(row1, 1);
  // 解釈から求めた UTC であることを画面の文字で出す。秒の精度は title に入れる。
  expect(within(interpretedTime).getByText("2031-10-08 01:20:35")).toBeTruthy();
  expect(within(interpretedTime).getByText("分析者: UTC+09:00")).toBeTruthy();
  expect(timeFactsOf(row1)).toMatch(/精度: /);
  expect(timeFactsOf(row1)).toContain("分析者の記録: UTC+09:00");
  const undeterminedTime = cellOf(row2, 1);
  expect(within(undeterminedTime).getByText("タイムゾーン不明")).toBeTruthy();
  expect(within(undeterminedTime).queryByText(/^分析者: /)).toBeNull();
  expect(within(undeterminedTime).getByText("タイムゾーン不明").title).toBe(
    "UTC に直せない時刻",
  );
});

test("参照した値からだけ分かっている対象のときだけ、参照だけの印と参照したレコードの見出しを出し、印の focus で意味を出す", async () => {
  // fixture のノードは observed である。
  renderDetail("process", undefined);
  expect(screen.queryByText("参照だけ")).toBeNull();
  expect(
    screen.getByRole("table", { name: "ノードを記録したレコード" }),
  ).toBeTruthy();
  expect(screen.getByText("件数: 2")).toBeTruthy();
  expect(
    screen.queryByRole("table", { name: "このノードを参照したレコード" }),
  ).toBeNull();
  cleanup();

  const json = nodeDetailResponseJson();
  const renderReferenced = (creationRecord: string) =>
    render(
      <NodeDetail
        state={{
          status: "loaded",
          value: decodeNodeDetailResponse(
            {
              ...json,
              node: { ...json.node, observation: "referenced", creationRecord },
            },
            "fixture",
          ),
        }}
        onSelectRecord={() => {}}
        assertions={() => null}
      />,
    );
  renderReferenced("present");
  const origin =
    "ノードの元: 別のノードのレコードが指した SID・名前・プロセス番号";
  expect(queryPair(origin)).toBeNull();
  expect(
    screen.getByRole("table", { name: "このノードを参照したレコード" }),
  ).toBeTruthy();
  expect(
    screen.queryByRole("table", { name: "ノードを記録したレコード" }),
  ).toBeNull();
  expect(screen.getByText("参照だけ")).toBeTruthy();
  // 印は操作を持たない。意味は印の横の help を focus すると出る。
  expect(screen.queryByRole("button", { name: "参照だけ" })).toBeNull();
  const help = screen.getByRole("button", { name: "参照だけ の説明" });
  act(() => help.focus());
  // 生成を記録した根拠と矛盾しないよう、根拠が参照したレコードの中にあることを添える。
  expect(await findPair(origin)).toBeTruthy();
  expect(getPair("このノードを記録したレコード: なし")).toBeTruthy();
  const creation = "作成レコード: このノードを指したレコードの中";
  expect(getPair(creation)).toBeTruthy();
  cleanup();

  renderReferenced("absent");
  act(() => screen.getByRole("button", { name: "参照だけ の説明" }).focus());
  expect(await findPair(origin)).toBeTruthy();
  expect(queryPair(creation)).toBeNull();
});

/** byte 位置だけで区別するレコードの根拠。通番と行番号を持たない入力形式の形である。 */
function byteRangeEvidence(byteOffset: number) {
  const evidence = nodeDetailResponseJson().evidence[0];
  return {
    ...evidence,
    recordRef: {
      sourceId: evidence.recordRef.sourceId,
      sourceContentSha256: evidence.recordRef.sourceContentSha256,
      sourceFileName: "host-a.bin",
      positionKind: "byte_range",
      byteOffset,
      byteLength: 64,
      recordRawTextRef: `/api/v0/records?byteOffset=${byteOffset}`,
    },
  };
}

test("byte 位置だけが異なる根拠のレコードを、同じ key にせず 1 行ずつ出す", () => {
  const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
  onTestFinished(() => consoleError.mockRestore());
  const json = createdNodeDetailResponseJson();
  const evidence = [byteRangeEvidence(1024), byteRangeEvidence(2048)];
  render(
    <NodeDetail
      state={{
        status: "loaded",
        value: decodeNodeDetailResponse(
          {
            ...json,
            evidence,
            evidenceCount: evidence.length,
            creationRecords: evidence,
            creationRecordCount: evidence.length,
          },
          "fixture",
        ),
      }}
      onSelectRecord={() => {}}
      assertions={() => null}
    />,
  );

  expect(
    consoleError.mock.calls.filter((call) =>
      String(call[0]).includes("same key"),
    ),
  ).toEqual([]);
  expect(screen.getAllByText(/^1024-/)).toHaveLength(2);
  expect(screen.getAllByText(/^2048-/)).toHaveLength(2);
});

test("参照だけの印を描いても、console に error と warning を出さない", () => {
  const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
  const consoleWarn = vi.spyOn(console, "warn").mockImplementation(() => {});
  onTestFinished(() => {
    consoleError.mockRestore();
    consoleWarn.mockRestore();
  });
  const json = nodeDetailResponseJson();
  render(
    <NodeDetail
      state={{
        status: "loaded",
        value: decodeNodeDetailResponse(
          { ...json, node: { ...json.node, observation: "referenced" } },
          "fixture",
        ),
      }}
      onSelectRecord={() => {}}
      assertions={() => null}
    />,
  );

  expect(screen.getByText("参照だけ")).toBeTruthy();
  expect([...consoleError.mock.calls, ...consoleWarn.mock.calls]).toEqual([]);
});

test.each([
  ["名前の分からない端末に置いたファイル", "file", true, true],
  ["端末を割り当てたファイル", "file", false, false],
  ["印を持たないプロセス", "process", undefined, false],
] as const)(
  "%s に端末が不明の印を出すか",
  (_name, kind, onUnknownTerminal, shown) => {
    const json = nodeDetailResponseJson();
    render(
      <NodeDetail
        state={{
          status: "loaded",
          value: decodeNodeDetailResponse(
            {
              ...json,
              node: {
                ...json.node,
                kind,
                creationRecord:
                  kind === "process" ? json.node.creationRecord : "item_absent",
                ...(onUnknownTerminal === undefined
                  ? {}
                  : { onUnknownTerminal }),
              },
              creationRecords: kind === "process" ? json.creationRecords : [],
              creationRecordCount:
                kind === "process" ? json.creationRecordCount : 0,
            },
            "fixture",
          ),
        }}
        onSelectRecord={() => {}}
        assertions={() => null}
      />,
    );
    expect(screen.queryByText("端末が不明") !== null).toBe(shown);
  },
);

test("Logon ID が一致しながら結ばなかったログオンを理由とともに出し、元レコードを開ける", () => {
  const detail = decodeNodeDetailResponse(
    rejectedLogonNodeDetailResponseJson(),
    "fixture",
  );
  const onSelectRecord = vi.fn();
  render(
    <NodeDetail
      state={{ status: "loaded", value: detail }}
      onSelectRecord={onSelectRecord}
      assertions={() => null}
    />,
  );
  expect(
    screen.getByRole("heading", { name: "ログオンと操作の結び付け" }),
  ).toBeTruthy();
  expect(screen.getByText("件数: 1")).toBeTruthy();
  const table = screen.getByRole("table", {
    name: "エッジにしなかったログオン",
  });
  const reason = within(table).getByText("別の端末のログオン");
  expect(reason.title).toBe("Logon ID が一意な範囲: 1 台の端末");
  fireEvent.click(
    screen.getByRole("button", {
      name: /^ログオンのレコードを Record に表示: /,
    }),
  );
  expect(onSelectRecord).toHaveBeenCalledWith(
    expect.objectContaining({
      sourceFileName: "host-a.log",
      positionKind: "sequence_number",
      sequenceNumber: 112,
      lineNumber: 1022,
    }),
  );
});

test("結ばなかったログオンを持たないノードでは、その節を出さない", () => {
  renderDetail("process", undefined);
  expect(
    screen.queryByRole("table", { name: "エッジにしなかったログオン" }),
  ).toBeNull();
  expect(
    screen.queryByRole("heading", { name: "ログオンと操作の結び付け" }),
  ).toBeNull();
});

test("関連付けの結果の節は、通信の相手を関連付ける起点かを述べ、ログオンの関係と分ける", () => {
  const json = nodeDetailResponseJson();
  render(
    <NodeDetail
      state={{
        status: "loaded",
        value: decodeNodeDetailResponse(
          {
            ...json,
            node: {
              ...json.node,
              kind: "record",
              creationRecord: "item_absent",
            },
            relationDerivation: {
              outcome: "not_used_as_origin",
              basis: "not_applicable",
            },
          },
          "fixture",
        ),
      }}
      onSelectRecord={() => {}}
      assertions={() => null}
    />,
  );
  expect(screen.getByRole("heading", { name: "接続先の推定" })).toBeTruthy();
  expect(getPair("結果: 推定の元に不使用")).toBeTruthy();
  expect(getPair("判定の根拠: 該当なし")).toBeTruthy();
  expect(screen.queryByText("次の操作:")).toBeNull();
  expect(
    screen.queryByRole("heading", { name: "ログオンと操作の結び付け" }),
  ).toBeNull();
});

test("表示名を持つノードはノード名の横にコピーボタンを出し、表示名を持たないノードにはコピーボタンを出さない", () => {
  const json = nodeDetailResponseJson();
  const { rerender } = render(
    <NodeDetail
      state={{
        status: "loaded",
        value: decodeNodeDetailResponse(json, "fixture"),
      }}
      onSelectRecord={() => {}}
      assertions={() => null}
    />,
  );
  // 表示名がある場合はコピーボタンが出る
  const copyLabelButton = screen.getByRole("button", {
    name: "コピー: C:\\Windows\\System32\\cmd.exe",
  });
  expect(copyLabelButton).toBeTruthy();

  // 表示名を持たない (item_absent) ノードでは内部 ID のコピーボタンを出さない
  rerender(
    <NodeDetail
      state={{
        status: "loaded",
        value: decodeNodeDetailResponse(
          {
            ...json,
            node: {
              ...json.node,
              id: "internal-hidden-node-id",
              label: {
                valueState: "item_absent",
              },
            },
          },
          "fixture",
        ),
      }}
      onSelectRecord={() => {}}
      assertions={() => null}
    />,
  );
  expect(
    screen.queryByRole("button", {
      name: /internal-hidden-node-id/,
    }),
  ).toBeNull();
  expect(
    screen.queryByRole("button", {
      name: "コピー: C:\\Windows\\System32\\cmd.exe",
    }),
  ).toBeNull();
});
