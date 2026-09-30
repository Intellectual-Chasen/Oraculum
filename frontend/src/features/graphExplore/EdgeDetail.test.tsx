// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, expect, onTestFinished, test, vi } from "vitest";
import {
  decodeEdgeDetailResponse,
  decodeEdgeEvidenceSelector,
  type EdgeDetailResponse,
  type EdgeEvidenceSelector,
} from "@/shared/contracts/graphDetail";
import {
  baselineCaseId,
  casedEdgeDetailResponseJson,
  challengeCaseId,
} from "@/testdata/cases/caseCounts";
import {
  edgeDetailResponseJson,
  logonTypeAbsentReason,
  manyEvidenceEdgeDetailResponseJson,
  manyStageEdgeDetailResponseJson,
  matchedEdgeDetailResponseJson,
  remoteSessionEdgeId,
  remoteSessionObservationKind,
  twoStageEdgeDetailResponseJson,
} from "@/testdata/graph/graphResponse";
import {
  analystAssignmentJson,
  assignmentSourceId,
} from "@/testdata/terminalAssignments/terminalAssignmentsResponse";
import { ValueActionsForTest } from "@/testdata/valueActions";
import { visibleText } from "@/testdata/visibleText";
import { EdgeDetail } from "./EdgeDetail";
import { getPair } from "./graphExploreTestHarness";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function loadedDetail(): EdgeDetailResponse {
  return decodeEdgeDetailResponse(edgeDetailResponseJson(), "response");
}

function renderDetail(
  overrides: {
    selectedSelector?: EdgeEvidenceSelector;
    onSelectGroup?: (selector: EdgeEvidenceSelector | undefined) => void;
    onSelectRecord?: () => void;
  } = {},
) {
  const onSelectRecord = overrides.onSelectRecord ?? (() => {});
  render(
    <ValueActionsForTest onSelectRecord={onSelectRecord}>
      <EdgeDetail
        state={{ status: "loaded", value: loadedDetail() }}
        selectedSelector={overrides.selectedSelector}
        onSelectGroup={overrides.onSelectGroup ?? (() => {})}
        onSelectRecord={onSelectRecord}
        assertions={() => <p>所見の欄</p>}
      />
    </ValueActionsForTest>,
  );
}

/** 区分の一覧の、区分 1 つずつの項目を返す。項目の中の一覧の項目を含めない。 */
function groupRows(): HTMLElement[] {
  const list = screen.getByRole("list", { name: "根拠のグループ" });
  return [...list.children] as HTMLElement[];
}

/** 区分 1 つの項目から、項目名 name の値を返す。 */
function groupValue(group: HTMLElement, name: string): HTMLElement {
  const term = within(group)
    .getAllByRole("term")
    .find((element) => element.textContent === name);
  const value = term?.nextElementSibling;
  if (!(value instanceof HTMLElement)) {
    throw new Error(`group has no value for ${name}`);
  }
  return value;
}

test("関係を選んでいないときは、関係の詳細の欄を出さない", () => {
  const { container } = render(
    <EdgeDetail
      state={{ status: "unselected" }}
      selectedSelector={undefined}
      onSelectGroup={() => {}}
      onSelectRecord={() => {}}
      assertions={() => null}
    />,
  );

  expect(container.textContent).toBe("");
  expect(screen.queryByRole("heading", { name: "Edge Detail" })).toBeNull();
});

test("見出しを Node Detail に揃えて Edge Detail にする", () => {
  renderDetail();
  expect(screen.getByRole("heading", { name: "Edge Detail" })).toBeTruthy();
});

/**
 * 区分を足した応答を描き、区分の行と、React が同じ key を知らせた console.error を返す。
 * React は同じ key を持つ兄弟を console.error で知らせる。
 */
function renderWithGroups(
  extraGroups: object[],
  overrides: {
    selectedSelector?: EdgeEvidenceSelector;
    onSelectGroup?: (selector: EdgeEvidenceSelector | undefined) => void;
  } = {},
) {
  const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
  onTestFinished(() => consoleError.mockRestore());
  const whole = edgeDetailResponseJson();
  const response = decodeEdgeDetailResponse(
    { ...whole, evidenceGroups: [...whole.evidenceGroups, ...extraGroups] },
    "response",
  );
  render(
    <EdgeDetail
      state={{ status: "loaded", value: response }}
      selectedSelector={overrides.selectedSelector}
      onSelectGroup={overrides.onSelectGroup ?? (() => {})}
      onSelectRecord={() => {}}
      assertions={() => null}
    />,
  );
  return {
    groupCount: response.evidenceGroups.length,
    rows: groupRows(),
    duplicateKeys: consoleError.mock.calls.filter((call) =>
      String(call[0]).includes("same key"),
    ),
  };
}

test("接続先 port の値だけが異なる、要求で指せない区分を別の行に出す", () => {
  // 観測の種別の値を読めない区分を、接続先 port の値だけを変えて 2 つ置く。backend の
  // 区分の鍵は接続先 port の値を含むため、2 つは別の区分である。
  const unselectable = (port: string) => ({
    observationKind: { raw: [] },
    destinationPort: {
      name: "dstPort",
      semantic: "connection.destination_port",
      kind: "text",
      text: { rawText: port, valueState: "present" },
    },
    logonTypeAbsence: logonTypeAbsentReason,
    selectorAbsence:
      "区分のレコードの観測の種別の値を読めないため、要求で指せません",
    accounts: [],
    authentications: [],
    evidenceCount: 1,
  });
  const { groupCount, rows, duplicateKeys } = renderWithGroups([
    unselectable("22"),
    unselectable("80"),
  ]);

  expect(rows).toHaveLength(groupCount);
  for (const port of ["22", "80"]) {
    expect(rows.some((row) => within(row).queryByText(port) !== null)).toBe(
      true,
    );
  }
  expect(duplicateKeys).toEqual([]);
});

test("観測の種別の値だけが異なる、要求で指せない区分を別の行に出す", () => {
  // 接続先 port の値を比べられない区分を、観測の種別の動作だけを変えて 2 つ置く。backend の
  // 区分の鍵は観測の種別の値を含むため、2 つは別の区分である。
  const unselectable = (action: string) => ({
    observationKind: remoteSessionObservationKind("net", action),
    destinationPortAbsence: "区分のレコードの接続先 port の値を比べられません",
    logonTypeAbsence: logonTypeAbsentReason,
    selectorAbsence:
      "区分のレコードの接続先 port の値を比べられないため、要求で指せません",
    accounts: [],
    authentications: [],
    evidenceCount: 1,
  });
  const { groupCount, rows, duplicateKeys } = renderWithGroups([
    unselectable("conn"),
    unselectable("disc"),
  ]);

  expect(rows).toHaveLength(groupCount);
  expect(rows.some((row) => within(row).queryByText(/disc/) !== null)).toBe(
    true,
  );
  expect(duplicateKeys).toEqual([]);
});

test("根拠を観測の種別と接続先 port ごとの区分に分けて出す", () => {
  renderDetail();

  const texts = groupRows().map((row) => row.textContent ?? "");
  expect(texts[0]).toContain("acpt");
  expect(texts[0]).toContain("5985");
  expect(texts[1]).toContain("acpt");
  expect(texts[1]).toContain("445");
  expect(texts[2]).toContain("loginR");
  // **接続先 port を持たない区分は、持たない理由を出す。** 0 を出さない。
  expect(texts[2]).toContain(
    "区分のレコードが接続先 port の欄を持っていません",
  );
  expect(texts[2]).not.toContain("5985");
  // 欄はあるが値を比べられない区分は、別の理由を出す。
  expect(texts[3]).toContain(
    "区分のレコードの接続先 port の値を比べられません",
  );
});

test("要求先の URL から導いた接続先 port は、URL を出さずに port の値を出す", () => {
  // Proxy の要求の port は要求先の URL から導いた値であり、原資料の文字列は URL である。
  const { rows } = renderWithGroups([
    {
      observationKind: { raw: [] },
      destinationPort: {
        name: "requestTargetPort",
        semantic: "connection.destination_port",
        kind: "text",
        text: {
          rawText: "http://203.0.113.5/test",
          normalized: "80",
          valueState: "present",
          derivation: "要求先の scheme の既定の port",
        },
      },
      logonTypeAbsence: logonTypeAbsentReason,
      selectorAbsence:
        "区分のレコードの観測の種別の値を読めないため、要求で指せません",
      accounts: [],
      authentications: [],
      evidenceCount: 1,
    },
  ]);

  expect(
    groupValue(rows[rows.length - 1] as HTMLElement, "接続先 port").textContent,
  ).toBe("80");
});

test("HTTP の要求の区分に HTTP の状態を出し、選ぶと状態を指す値を渡す", () => {
  const httpGroup = (status: string, count: number) => ({
    observationKind: { raw: [] },
    destinationPortAbsence: "区分のレコードは接続先 port の欄を持ちません",
    logonTypeAbsence: logonTypeAbsentReason,
    httpStatus: {
      name: "status",
      semantic: "http.status_code",
      kind: "text",
      text: { rawText: status, valueState: "present" },
    },
    selector: {
      eventCategory: "",
      eventAction: "",
      destinationPortAbsent: true,
      logonTypeAbsent: true,
      httpStatus: status,
    },
    accounts: [],
    authentications: [],
    evidenceCount: count,
  });
  const selected = vi.fn();
  const { rows, duplicateKeys } = renderWithGroups(
    [httpGroup("404", 2), httpGroup("200", 1)],
    { onSelectGroup: selected },
  );

  expect(duplicateKeys).toEqual([]);
  const notFound = rows[rows.length - 2] as HTMLElement;
  expect(groupValue(notFound, "HTTP の状態").textContent).toBe("404");
  fireEvent.click(
    within(notFound).getByRole("button", { name: "このグループの根拠を表示" }),
  );
  expect(selected.mock.calls[0]?.[0]).toMatchObject({ httpStatus: "404" });
});

test("遠隔ログインの区分のレコードに現れたアカウントを出し、他の区分はアカウントが現れないことを出す", () => {
  renderDetail();

  const rows = groupRows();
  expect(rows[2]?.textContent).toContain("user02");
  // 条件の両側。レコードにアカウントが現れない区分は、現れないことを出す。
  expect(
    groupValue(rows[0] as HTMLElement, "アカウント").textContent,
  ).toContain("アカウントなし");
  expect(rows[0]?.textContent).not.toContain("user02");
});

test("区分を選ぶと、応答が返した区分を指す値をそのまま上位へ渡す", () => {
  const selected = vi.fn();
  renderDetail({ onSelectGroup: selected });

  const button = within(groupRows()[0] as HTMLElement).getByRole("button", {
    name: "このグループの根拠を表示",
  });
  fireEvent.click(button);

  expect(selected).toHaveBeenCalledTimes(1);
  expect(selected.mock.calls[0]?.[0]).toEqual(
    loadedDetail().evidenceGroups[0]?.selector,
  );
});

test("接続先 port の欄を持たない区分は、欄の不在を指す値を渡す", () => {
  const selected = vi.fn();
  renderDetail({ onSelectGroup: selected });

  fireEvent.click(
    within(groupRows()[2] as HTMLElement).getByRole("button", {
      name: "このグループの根拠を表示",
    }),
  );

  expect(selected.mock.calls[0]?.[0]).toEqual({
    eventCategory: "session",
    eventAction: "loginR",
    destinationPort: undefined,
    destinationPortAbsent: true,
    logonType: undefined,
    logonTypeAbsent: true,
  });
});

/**
 * ログオンの種別のコードの欄。
 *
 * `derived` は Mark II の logonType の原資料の文字列から導いた形で、原資料の文字列を持たず正規化値だけを持つ。
 * `raw` は Windows イベントログの LogonType の形で、原資料の文字列だけを持つ。
 */
function logonTypeField(code: string, form: "derived" | "raw") {
  if (form === "raw") {
    return {
      name: "EventData.LogonType",
      semantic: "event.logon_type",
      kind: "text",
      text: { rawText: code, valueState: "present" },
    };
  }
  return {
    name: "logonTypeCode",
    semantic: "event.logon_type",
    kind: "text",
    text: {
      normalized: code,
      derivation: "logonType の値の括弧の中の 10 進の数字",
      valueState: "derived",
    },
  };
}

/** 種別のコードを持つログオンの区分。要求で指せる。 */
function logonGroup(code: string, form: "derived" | "raw" = "derived") {
  return {
    observationKind: remoteSessionObservationKind("os", "evtLog"),
    destinationPortAbsence: "区分のレコードが接続先 port の欄を持っていません",
    logonType: logonTypeField(code, form),
    selector: {
      eventCategory: "os",
      eventAction: "evtLog",
      destinationPortAbsent: true,
      logonType: code,
    },
    accounts: [],
    authentications: [],
    evidenceCount: 1,
  };
}

/** リモート対話 (10) のログオンの区分。 */
function remoteInteractiveLogonGroup() {
  return logonGroup("10");
}

/** 位置 index の区分の行の「この区分の根拠を出す」の button を返す。 */
function groupButton(rows: HTMLElement[], index: number) {
  return within(rows.at(index) as HTMLElement).getByRole("button", {
    name: "このグループの根拠を表示",
  });
}

test("ログオンの種別ごとに区分を出し、種別を持たない区分は種別の値が無いと出す", () => {
  const { rows, duplicateKeys } = renderWithGroups([
    remoteInteractiveLogonGroup(),
  ]);

  const logonRow = rows.at(-1) as HTMLElement;
  expect(logonRow.textContent).toContain("evtLog");
  expect(groupValue(logonRow, "ログオンタイプ").textContent).toBe("10");
  expect(logonRow.textContent).not.toContain(logonTypeAbsentReason);
  // **種別を持たない区分は、持たない理由を出す。** 0 を出さない。
  for (const row of rows.slice(0, -1)) {
    expect(row.textContent).toContain(logonTypeAbsentReason);
    expect(within(row).queryByText("0")).toBeNull();
  }
  expect(duplicateKeys).toEqual([]);
});

test("ログオンの種別の区分を選ぶと、種別のコードを指す値を渡す", () => {
  const selected = vi.fn();
  const whole = edgeDetailResponseJson();
  render(
    <EdgeDetail
      state={{
        status: "loaded",
        value: decodeEdgeDetailResponse(
          {
            ...whole,
            evidenceGroups: [
              ...whole.evidenceGroups,
              remoteInteractiveLogonGroup(),
            ],
          },
          "response",
        ),
      }}
      selectedSelector={undefined}
      onSelectGroup={selected}
      onSelectRecord={() => {}}
      assertions={() => null}
    />,
  );

  fireEvent.click(
    within(groupRows().at(-1) as HTMLElement).getByRole("button", {
      name: "このグループの根拠を表示",
    }),
  );

  expect(selected.mock.calls[0]?.[0]).toEqual({
    eventCategory: "os",
    eventAction: "evtLog",
    destinationPort: undefined,
    destinationPortAbsent: true,
    logonType: "10",
    logonTypeAbsent: undefined,
  });
});

test("コード 10 の区分を選んでいるとき、10 の操作だけを無効にし、3 の操作は押せる", () => {
  const ten = logonGroup("10");
  const { rows } = renderWithGroups([logonGroup("3"), ten], {
    selectedSelector: decodeEdgeEvidenceSelector(ten.selector, "selector"),
  });

  expect(within(rows.at(-1) as HTMLElement).getByText("10")).toBeTruthy();
  expect(groupButton(rows, -1)).toHaveAttribute("aria-disabled", "true");
  expect(within(rows.at(-2) as HTMLElement).getByText("3")).toBeTruthy();
  expect(groupButton(rows, -2)).not.toHaveAttribute("aria-disabled");
});

test("ログオンの種別だけが異なる、要求で指せない区分を別の行に出す", () => {
  // 観測の種別の値を読めない区分を、ログオンの種別のコードだけを変えて 2 つ置く。backend の
  // 区分の鍵はログオンの種別の値を含むため、2 つは別の区分である。
  const unselectable = (code: string) => ({
    observationKind: { raw: [] },
    destinationPortAbsence: "区分のレコードが接続先 port の欄を持っていません",
    logonType: logonTypeField(code, "raw"),
    selectorAbsence:
      "区分のレコードの観測の種別の値を読めないため、要求で指せません",
    accounts: [],
    authentications: [],
    evidenceCount: 1,
  });
  const { groupCount, rows, duplicateKeys } = renderWithGroups([
    unselectable("3"),
    unselectable("10"),
  ]);

  expect(rows).toHaveLength(groupCount);
  for (const code of ["3", "10"]) {
    expect(rows.some((row) => within(row).queryByText(code) !== null)).toBe(
      true,
    );
  }
  expect(duplicateKeys).toEqual([]);
});

test("原資料の文字列だけを持つ Windows イベントログの形のコードを出す", () => {
  const { rows } = renderWithGroups([logonGroup("10", "raw")]);

  const row = rows.at(-1) as HTMLElement;
  expect(within(row).getByText("10")).toBeTruthy();
  expect(row.textContent).not.toContain(logonTypeAbsentReason);
});

test("要求で指せない区分は、押せる操作を出さずに指せない理由を出す", () => {
  renderDetail();

  const row = groupRows()[3] as HTMLElement;
  expect(
    within(row).queryByRole("button", { name: "このグループの根拠を表示" }),
  ).toBeNull();
  expect(row.textContent).toContain(
    "区分のレコードの接続先 port の値を比べられないため、要求で指せません",
  );
});

test("選んでいる区分の操作を無効にし、すべての区分へ戻す操作を出す", () => {
  const selected = vi.fn();
  const detail = loadedDetail();
  renderDetail({
    selectedSelector: detail.evidenceGroups[0]?.selector,
    onSelectGroup: selected,
  });

  const rows = groupRows();
  const current = within(rows[0] as HTMLElement).getByRole("button", {
    name: "このグループの根拠を表示",
  });
  expect(current).toHaveAttribute("aria-disabled", "true");
  // 反対側。選んでいない区分の操作は押せる。
  const other = within(rows[1] as HTMLElement).getByRole("button", {
    name: "このグループの根拠を表示",
  });
  expect(other).not.toHaveAttribute("aria-disabled");

  fireEvent.click(screen.getByRole("button", { name: "すべての根拠を表示" }));
  expect(selected).toHaveBeenCalledWith(undefined);
});

/** 見出しが summary の開閉できるセクションの、件数の組の文字列を返す。 */
function foldNote(summary: string): string {
  const details = [
    ...document.querySelectorAll<HTMLDetailsElement>("details.fold"),
  ].find((element) =>
    element.querySelector("summary")?.textContent?.startsWith(summary),
  );
  return details?.querySelector("summary > span")?.textContent ?? "";
}

test("根拠の表は、並んだ行の件数を見出しの横に出す", () => {
  renderDetail();

  const table = screen.getByRole("table", { name: "根拠のレコード" });
  const rowCount = within(table).getAllByRole("button", {
    name: /^Record に表示: /,
  }).length;
  expect(foldNote("根拠のレコード")).toBe(`件数: ${rowCount}`);
  expect(rowCount).toBe(loadedDetail().edge.evidence.length);
  // 見出しは開閉の欄の summary だけが出す。
  expect(screen.getAllByText("根拠のレコード")).toHaveLength(1);
});

test("案件ごとの件数を持つエッジは、根拠の表の見出しの横に案件ごとの件数を並べる", () => {
  const detail = decodeEdgeDetailResponse(
    casedEdgeDetailResponseJson(),
    "response",
  );
  render(
    <EdgeDetail
      state={{ status: "loaded", value: detail }}
      selectedSelector={undefined}
      onSelectGroup={() => {}}
      onSelectRecord={() => {}}
      assertions={() => null}
    />,
  );

  expect(foldNote("根拠のレコード")).toBe(
    `件数: ${detail.edge.evidenceCount} · 案件ごと: ` +
      `${baselineCaseId}: 1、${challengeCaseId}: 3`,
  );
});

test("案件ごとの件数を持たないエッジは、根拠の表の見出しの横に案件を出さない", () => {
  renderDetail();

  expect(foldNote("根拠のレコード")).not.toContain("案件");
});

test("観測したエッジは作り方を観測と出し、推定のエッジは推定と出す", () => {
  const renderJson = (json: unknown) =>
    render(
      <EdgeDetail
        state={{
          status: "loaded",
          value: decodeEdgeDetailResponse(json, "response"),
        }}
        selectedSelector={undefined}
        onSelectGroup={() => {}}
        onSelectRecord={() => {}}
        assertions={() => null}
      />,
    );
  const base = edgeDetailResponseJson();
  const stateCell = () =>
    visibleText(
      within(screen.getByRole("table", { name: "選択中のエッジ" })).getByRole(
        "rowheader",
        { name: "作り方" },
      ).nextElementSibling,
    );
  renderJson({ ...base, edge: { ...base.edge, state: "observed" } });
  expect(stateCell()).toBe("観測");
  cleanup();

  // fixture のエッジは推定したエッジである。
  renderJson(base);
  expect(stateCell()).toBe("推定");
});

/** 応答の JSON に手を加えて関係の詳細を描く。 */
function renderDetailJson(json: object, onSelectRecord = () => {}) {
  render(
    <EdgeDetail
      state={{
        status: "loaded",
        value: decodeEdgeDetailResponse(json, "response"),
      }}
      selectedSelector={undefined}
      onSelectGroup={() => {}}
      onSelectRecord={onSelectRecord}
      assertions={() => null}
    />,
  );
}

// エッジの推定を経ていないエッジは、推定が無いことを値の組で出す。値を空にしない。
// 作った元はエッジの作り方に合わせ、推定したエッジを「レコード」から直接作ったと書かない。
test("推定を持たないエッジに、作り方に合わせて作った元を出す", () => {
  const base = edgeDetailResponseJson();
  renderDetailJson({ ...base, edge: { ...base.edge, state: "observed" } });
  expect(getPair("エッジの推定: なし")).toBeTruthy();
  expect(getPair("作った元: レコード")).toBeTruthy();
  cleanup();

  renderDetail();
  expect(getPair("作った元: レコードの値")).toBeTruthy();
});

test("推定した親子に、親と子のレコードの組と、条件ごとの両端の値と、Record に表示する値を出す", () => {
  const base = edgeDetailResponseJson();
  const evidence = base.edge.evidence[0];
  const pidField = (name: string, semantic: string) => ({
    name,
    semantic,
    kind: "text",
    text: { rawText: "0x10", valueState: "present" },
  });
  const parent = {
    ...evidence,
    recordRef: { ...evidence.recordRef, lineNumber: 7 },
    eventTime: { ...evidence.eventTime, rawText: "2001/02/03 04:05:05" },
  };
  const child = {
    ...evidence,
    eventTime: { ...evidence.eventTime, rawText: "2001/02/03 04:05:07" },
  };
  const onSelectRecord = vi.fn();
  renderDetailJson(
    {
      ...base,
      edge: { ...base.edge, kind: "process_parent_child" },
      recordPairs: [
        {
          left: parent,
          right: child,
          conditions: [
            {
              conditionKey: "process_pid",
              leftValue: [pidField("NewProcessId", "process.pid")],
              rightValue: [pidField("ProcessId", "process.parent_pid")],
            },
            { conditionKey: "time_order", leftValue: [], rightValue: [] },
          ],
        },
      ],
      recordPairCount: 1,
    },
    onSelectRecord,
  );

  expect(getPair("作った元: レコードの組の条件")).toBeTruthy();
  const table = screen.getByRole("table", { name: "レコードの組 1" });
  // 条件ごとに、始点と終点の 2 行を 1 つの行の組にまとめる。
  const conditionGroup = (label: RegExp) =>
    within(table)
      .getAllByRole("rowgroup")
      .find((group) => label.test(group.textContent ?? ""));
  const pidRows = conditionGroup(/プロセス番号の一致/);
  expect(pidRows?.textContent).toContain("NewProcessId: 0x10");
  expect(pidRows?.textContent).toContain("ProcessId: 0x10");
  const timeRows = conditionGroup(/時刻の順/);
  expect(timeRows?.textContent).toContain("2001/02/03 04:05:05");
  expect(timeRows?.textContent).toContain("2001/02/03 04:05:07");

  const [openParent, openChild] = within(table).getAllByRole("button");
  fireEvent.click(openParent as HTMLElement);
  fireEvent.click(openChild as HTMLElement);
  expect(onSelectRecord).toHaveBeenNthCalledWith(
    1,
    expect.objectContaining({ lineNumber: 7 }),
  );
  expect(onSelectRecord).toHaveBeenNthCalledWith(
    2,
    expect.objectContaining({ lineNumber: evidence.recordRef.lineNumber }),
  );
});

test("ログオンの連鎖の組に、候補の数と上位の候補の数、アカウントとログオンタイプの条件、時刻の差を出す", () => {
  const base = edgeDetailResponseJson();
  const evidence = base.edge.evidence[0];
  renderDetailJson(
    {
      ...base,
      edge: { ...base.edge, kind: "logon_chain", state: "uncertain_chain" },
      recordPairs: [
        {
          left: evidence,
          right: evidence,
          conditions: [
            {
              conditionKey: "session_end_last_operation",
              leftValue: [],
              rightValue: [],
            },
            {
              conditionKey: "session_account_match",
              leftValue: [],
              rightValue: [],
            },
            {
              conditionKey: "session_logon_network",
              leftValue: [],
              rightValue: [],
            },
            {
              conditionKey: "time_proximity",
              leftValue: [],
              rightValue: [],
              windowSeconds: 2,
              differenceSeconds: 0.25,
            },
          ],
          candidateTally: { candidateCount: 3, precedingCandidateCount: 1 },
        },
      ],
      recordPairCount: 1,
    },
    vi.fn(),
  );
  const table = screen.getByRole("table", { name: "レコードの組 1" });
  expect(getPair("候補: 3", table)).toBeTruthy();
  expect(getPair("上位の候補: 1", table)).toBeTruthy();
  expect(table).toHaveTextContent("終わり: 最後の操作の記録");
  expect(table).toHaveTextContent("アカウントが一致");
  expect(table).toHaveTextContent("ネットワークのログオン");
  expect(getPair("許容幅: ±2 秒", table)).toBeTruthy();
  expect(getPair("時刻の差: +0.25 秒", table)).toBeTruthy();
});

test.each([
  ["差だけを持つ条件", { differenceSeconds: 1 }, /differenceSeconds/],
  [
    "上の区分の候補が候補の数に届く件数",
    { tally: { candidateCount: 2, precedingCandidateCount: 2 } },
    /precedingCandidateCount/,
  ],
])("%s を退ける", (_name, variant, message) => {
  const base = edgeDetailResponseJson();
  const pair = rightOnlyPair(base.edge.evidence[0]);
  const broken =
    "tally" in variant
      ? { ...pair, candidateTally: variant.tally }
      : {
          ...pair,
          conditions: [
            {
              conditionKey: "time_proximity",
              leftValue: [],
              rightValue: [],
              ...variant,
            },
          ],
        };
  expect(() =>
    decodeEdgeDetailResponse(
      { ...base, recordPairs: [broken], recordPairCount: 1 },
      "response",
    ),
  ).toThrow(message);
});

/** 条件を 1 つ持ち、終点の側だけがレコードを持つ組。 */
function rightOnlyPair(evidence: object) {
  return {
    right: evidence,
    conditions: [{ conditionKey: "logon_guid", leftValue: [], rightValue: [] }],
  };
}

test.each([
  [
    "両側ともレコードを持たない組",
    { recordPairs: [{ conditions: [] }] },
    /left/,
  ],
  ["条件を持たない組", "no-conditions", /conditions/],
  ["組の総数を持たない応答", "no-count", /recordPairCount/],
  ["総数より多い組を持つ応答", "count-below", /recordPairCount/],
])("%s を退ける", (_name, variant, message) => {
  const base = edgeDetailResponseJson();
  const pair = rightOnlyPair(base.edge.evidence[0]);
  const overrides =
    variant === "no-conditions"
      ? { recordPairs: [{ ...pair, conditions: [] }], recordPairCount: 1 }
      : variant === "no-count"
        ? { recordPairs: [pair] }
        : variant === "count-below"
          ? { recordPairs: [pair, pair], recordPairCount: 1 }
          : { ...(variant as object), recordPairCount: 1 };
  expect(() =>
    decodeEdgeDetailResponse({ ...base, ...overrides }, "response"),
  ).toThrow(message);
  expect(() =>
    decodeEdgeDetailResponse(
      { ...base, recordPairs: [pair], recordPairCount: 1 },
      "response",
    ),
  ).not.toThrow();
});

test("総数より少ない組を持つ応答は、受け取った組の数を出す", () => {
  const base = edgeDetailResponseJson();
  renderDetailJson({
    ...base,
    recordPairs: [rightOnlyPair(base.edge.evidence[0])],
    recordPairCount: 5,
  });

  expect(getPair("組数: 5")).toBeTruthy();
  expect(getPair("受け取った組: 1")).toBeTruthy();
});

test("組は 50 組ずつ描き、続きを表示する操作で次の組を追加する", () => {
  const base = edgeDetailResponseJson();
  const pair = rightOnlyPair(base.edge.evidence[0]);
  renderDetailJson({
    ...base,
    recordPairs: Array.from({ length: 60 }, () => pair),
    recordPairCount: 60,
  });
  openFold("レコードの組");

  expect(
    screen.getAllByRole("table", { name: /^レコードの組 \d+$/ }),
  ).toHaveLength(50);
  expect(getPair("表示: 50 / 60")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "続きを表示" }));
  expect(
    screen.getAllByRole("table", { name: /^レコードの組 \d+$/ }),
  ).toHaveLength(60);
  expect(screen.queryByRole("button", { name: "続きを表示" })).toBeNull();
});

test("時刻の条件で時刻を持たない端は、時刻が無いことを出し、終点が欠けた組は始点の補足を出さない", () => {
  const base = edgeDetailResponseJson();
  const { eventTime: _omitted, ...withoutTime } = base.edge.evidence[0];
  renderDetailJson({
    ...base,
    recordPairs: [
      {
        left: withoutTime,
        conditions: [
          { conditionKey: "time_order", leftValue: [], rightValue: [] },
        ],
      },
    ],
    recordPairCount: 1,
  });

  const table = screen.getByRole("table", { name: "レコードの組 1" });
  expect(table.textContent).toContain("時刻なし");
  expect(table.textContent).toContain("レコードなし");
  expect(table.textContent).not.toContain("割り当ての無い IP アドレス");
});

test("始点にレコードを持たない組は、持たないことを出す", () => {
  const base = edgeDetailResponseJson();
  renderDetailJson({
    ...base,
    edge: { ...base.edge, kind: "unidentified_source_remote_session" },
    recordPairs: [
      {
        right: base.edge.evidence[0],
        conditions: [
          {
            conditionKey: "source_unassigned",
            leftValue: [],
            rightValue: [
              {
                name: "IpAddress",
                semantic: "connection.source_address",
                kind: "text",
                text: { rawText: "198.51.100.7", valueState: "present" },
              },
            ],
          },
        ],
      },
    ],
    recordPairCount: 1,
  });

  const table = screen.getByRole("table", { name: "レコードの組 1" });
  expect(table.textContent).toContain(
    "レコードなし: 割り当ての無い IP アドレスのノード",
  );
  expect(table.textContent).toContain("IpAddress: 198.51.100.7");
});

test("グループのアカウントに、識別した値と件数を添える", () => {
  renderDetail();

  expect(
    groupValue(groupRows()[2] as HTMLElement, "アカウント").textContent,
  ).toBe("user02 EXAMPLE \\ user02 1");
});

test("グループのレコードが記録した認証の方式を件数とともに出し、記録しないグループはそのことを出す", () => {
  renderDetail();

  const rows = groupRows();
  expect(groupValue(rows[2] as HTMLElement, "認証の方式").textContent).toBe(
    "NTLM 1",
  );
  expect(
    groupValue(rows[0] as HTMLElement, "認証の方式").textContent,
  ).toContain("認証の方式なし");
});

/** 端末の割当を持つ関係の詳細を描く。observedInRecords はレコードも直に観測したかである。 */
function renderAssignedDetail(observedInRecords: boolean) {
  const base = edgeDetailResponseJson();
  const detail = decodeEdgeDetailResponse(
    {
      ...base,
      edge: { ...base.edge, state: "observed" },
      terminalAssignments: [analystAssignmentJson()],
      observedInRecords,
    },
    "response",
  );
  render(
    <EdgeDetail
      state={{ status: "loaded", value: detail }}
      selectedSelector={undefined}
      onSelectGroup={() => {}}
      onSelectRecord={() => {}}
      assertions={() => null}
      sourceFileNames={new Map([[assignmentSourceId, "assigned-host.log"]])}
    />,
  );
}

test("端末の割り当てから作ったエッジは、割り当てとその根拠を出し、レコードから直接作ったとは出さない", () => {
  const assignment = analystAssignmentJson();
  renderAssignedDetail(false);

  const table = screen.getByRole("table", { name: /端末の割り当て: 1 件/ });
  expect(table.textContent).toContain(String(assignment.clientIp));
  expect(table.textContent).toContain("分析者");
  expect(table.textContent).toContain(String(assignment.derivation));
  // 割当を付けた収集元を file 名で出す。
  expect(table.textContent).toContain("assigned-host.log");
  // 割当を記録した分析者を出す。
  expect(table.textContent).toContain(String(assignment.author));
  expect(getPair("作った元: 端末の割り当て")).toBeTruthy();
  expect(getPair("割り当ての照合: IP アドレス")).toBeTruthy();
  expect(screen.queryByText("観測")).toBeNull();
});

test("引数が指すホスト名から割り当ての端末へ張ったエッジは、ホスト名で照合したことを出す", () => {
  const base = edgeDetailResponseJson();
  const { clientIp: _ip, ...assignment } = analystAssignmentJson();
  const detail = decodeEdgeDetailResponse(
    {
      ...base,
      edge: { ...base.edge, kind: "argument_names_object" },
      terminalAssignments: [
        {
          ...assignment,
          terminalHostname: "host-z",
          terminalHostnames: ["host-z", "host-z.example.test"],
        },
      ],
    },
    "response",
  );
  render(
    <EdgeDetail
      state={{ status: "loaded", value: detail }}
      selectedSelector={undefined}
      onSelectGroup={() => {}}
      onSelectRecord={() => {}}
      assertions={() => null}
    />,
  );

  expect(getPair("割り当ての照合: コマンドの引数のホスト名")).toBeTruthy();
  expect(screen.queryByText("IP アドレス", { selector: "li" })).toBeNull();
});

// レコード自身の IP と割り当てが同じエッジを与えたとき、観測したエッジであることと割り当ての両方を出す。
test("レコードも直に観測した割り当てのエッジは、観測と割り当ての両方を出す", () => {
  renderAssignedDetail(true);

  expect(screen.getByText("観測 · 端末の割り当て")).toBeTruthy();
  expect(getPair("作った元: レコード · 端末の割り当て")).toBeTruthy();
  expect(
    screen.getByRole("table", { name: /端末の割り当て: 1 件/ }),
  ).toBeTruthy();
});

test("根拠のレコードを選ぶと、元レコードの表示へ位置を渡す", () => {
  const opened = vi.fn();
  renderDetail({ onSelectRecord: opened });

  const evidenceTable = screen.getByRole("table", { name: /根拠のレコード/ });
  const openButtons = within(evidenceTable).getAllByRole("button", {
    name: /^Record に表示: /,
  });
  expect(openButtons).toHaveLength(loadedDetail().edge.evidence.length);
  fireEvent.click(openButtons[0] as HTMLElement);

  expect(opened).toHaveBeenCalledTimes(1);
  expect(opened.mock.calls[0]?.[0]).toEqual(
    loadedDetail().edge.evidence[0]?.recordRef,
  );
});

test("関係の詳細に所見の欄を出す", () => {
  renderDetail();

  expect(screen.getByText("所見の欄")).toBeTruthy();
});

test("推定を持たないエッジは、推定が作ったと読める見出しを出さない", () => {
  renderDetail();
  expect(loadedDetail().matchCount).toBe(0);

  expect(
    screen.queryByRole("heading", { name: "エッジを作った推定" }),
  ).toBeNull();
  expect(screen.getByRole("heading", { name: "エッジの推定" })).toBeTruthy();
  cleanup();

  renderMatchedDetail();
  expect(
    screen.getByRole("heading", { name: "エッジを作った推定" }),
  ).toBeTruthy();
});

test("エッジの識別子を要求へ渡す形で持つ", () => {
  expect(loadedDetail().edge.id).toBe(remoteSessionEdgeId);
});

function renderMatchedDetail(
  json: unknown = matchedEdgeDetailResponseJson(),
  onSelectRecord: () => void = () => {},
) {
  render(
    <ValueActionsForTest onSelectRecord={onSelectRecord}>
      <EdgeDetail
        state={{
          status: "loaded",
          value: decodeEdgeDetailResponse(json, "response"),
        }}
        selectedSelector={undefined}
        onSelectGroup={() => {}}
        onSelectRecord={onSelectRecord}
        assertions={() => <p>所見の欄</p>}
      />
    </ValueActionsForTest>,
  );
}

/** 関連付けの段階の見出し。根拠の一覧などの開閉できる欄の見出しと分ける。 */
const stageSummary = "details.match-stage > summary";

/** 関連付けの段階の一覧のうち、位置 index の段階を開く。 */
function openStage(index = 0) {
  const details = document.querySelectorAll<HTMLDetailsElement>(
    "details.match-stage",
  )[index];
  if (details === undefined) {
    throw new Error(`no stage at ${index}`);
  }
  details.open = true;
  fireEvent(details, new Event("toggle"));
  return details;
}

const candidateTableName = "エッジを作った候補";

/** 段階の中の、この関係を作った候補の表の行の文字列を返す。 */
function candidateRowsOf(stage: HTMLElement) {
  const table = within(stage).getByRole("table", { name: candidateTableName });
  return within(table)
    .getAllByRole("row")
    .slice(1)
    .map((row) => row.textContent ?? "");
}

// 段階は開くまで中身を描かない。段階が数千個あるエッジでも、一覧だけを描く。
test("推定の段階は、元のレコードと候補の件数を出し、開くまで条件を描かない", () => {
  renderMatchedDetail();

  expect(foldNote("推定の結果")).toBe("件数: 1 · 段階: 1");
  const summary = document.querySelector(stageSummary);
  expect(summary?.textContent).toContain("段階 2: 同じ秒の時刻");
  expect(summary?.textContent).toContain("ID: 112");
  expect(screen.queryByRole("table", { name: "この段階" })).toBeNull();

  openStage();
  expect(screen.getByRole("table", { name: "この段階" })).toBeTruthy();
});

test.each([
  ["TCP_MEM_HIT:HIER_NONE", "cache からの応答"],
  ["TCP_MISS:HIER_DIRECT", undefined],
])(
  "元のレコードの Proxy の行が記録した要求処理の結果 %s を段階に出す",
  (status, cache) => {
    const json = matchedEdgeDetailResponseJson();
    const origin = json.matchStages[0].origin;
    const matchRecords: unknown[] = [...json.matchRecords];
    matchRecords[origin] = {
      ...json.matchRecords[origin],
      proxyStatus: {
        name: "squidStatus",
        kind: "text",
        text: { rawText: status, valueState: "present" },
      },
    };
    renderMatchedDetail({ ...json, matchRecords });
    openStage();

    expect(getPair(`Proxy の処理結果: ${status}`)).toBeTruthy();
    if (cache === undefined) {
      expect(screen.queryByText("Proxy の cache:")).toBeNull();
    } else {
      expect(getPair(`Proxy の cache: ${cache}`)).toBeTruthy();
    }
  },
);

test("Proxy の要求処理の結果を持たない元のレコードの段階は、結果の組を出さない", () => {
  renderMatchedDetail();
  openStage();

  expect(screen.queryByText("Proxy の処理結果:")).toBeNull();
});

// 段階が挙げた候補は、候補の端点ごとに別のエッジへ分かれる。このエッジの件数だけを出すと、
// 他のエッジへ分かれた候補が無いと読める。
test("段階の見出しに、エッジを作った候補と段階が挙げた候補の件数を並べる", () => {
  const json = matchedEdgeDetailResponseJson();
  renderMatchedDetail(json);

  const stage = json.matchStages[0];
  const stageMemberCount =
    stage.stageTallies[stage.stageTallies.length - 1].memberCount;
  expect(json.matches.length).toBeLessThan(stageMemberCount);
  const summary = document.querySelector(stageSummary) as HTMLElement;
  const pairs = [...summary.querySelectorAll(".value-pairs > span")].map(
    (pair) => pair.textContent,
  );
  expect(pairs).toContain(`エッジの候補: ${json.matches.length}`);
  expect(pairs).toContain(`段階の候補: ${stageMemberCount}`);
  // summary の中身は phrasing content に限る。
  expect(summary.querySelector("ul, li, div, p")).toBeNull();
});

// 推定の結果は段階ごとに、推定の結果の並び順のまま分ける。
test("推定の結果を、結果を出した段階ごとに並び順のまま出す", () => {
  renderMatchedDetail(twoStageEdgeDetailResponseJson());

  const summaries = [...document.querySelectorAll(stageSummary)].map(
    (summary) => summary.textContent ?? "",
  );
  expect(summaries).toHaveLength(2);
  expect(summaries[0]).toContain("ID: 112");
  expect(summaries[0]).toContain("エッジの候補: 2");
  expect(summaries[1]).toContain("ID: 113");
  expect(summaries[1]).toContain("エッジの候補: 1");

  const first = candidateRowsOf(openStage(0));
  expect(first).toHaveLength(2);
  expect(first[0]).toContain("ID: 115");
  expect(first[1]).toContain("ID: 118");
  const second = candidateRowsOf(openStage(1));
  expect(second).toHaveLength(1);
  expect(second[0]).toContain("ID: 117");
});

// 関連付けの条件を画面から読む。条件が無いと、候補の関係が何で成立したかを読めない。
test("関連付けを経た関係に、用いた条件と用いなかった条件を出す", () => {
  renderMatchedDetail();
  openStage();

  // RawText は文字列を可視化のために分割して描く。行の全文で確かめる。
  const conditionRow = screen
    .getByRole("table", { name: "この段階" })
    .querySelector(":scope > tbody > tr");
  expect(conditionRow?.textContent).toContain("203.0.113.21");
});

// 段階が挙げた候補ごとに、候補のレコードと時刻の比較と確定しない理由を出す。
test("段階が挙げた候補を、時刻の比較と確定しない理由と一緒に出す", () => {
  renderMatchedDetail();
  openStage();

  const table = screen.getByRole("table", { name: candidateTableName });
  const rows = within(table).getAllByRole("row").slice(1);
  expect(rows).toHaveLength(1);
  const cells = within(rows[0]).getAllByRole("cell");
  expect(cells[0]?.textContent).toContain("ID: 115");
  expect(cells[1]?.textContent).toBe("秒");
  // 時刻を比べた段階の関連付けは、段階の前提に依拠する。
  expect(cells[2]?.textContent).toContain(
    "2 つの収集元の時計のずれは 1 秒未満である",
  );
  expect(cells[3]?.textContent).toContain(
    "段階が用いた条件だけでは候補が 1 件に定まらない",
  );
});

// 候補のレコードを開いた欄が、起点から候補への経路を出す。起点が無いと経路を求められない。
test("候補のレコードを開くと、段階の起点を添えて位置を渡す", () => {
  const json = twoStageEdgeDetailResponseJson();
  const detail = decodeEdgeDetailResponse(json, "response");
  const opened = vi.fn();
  renderMatchedDetail(json, opened);

  const table = within(openStage(1)).getByRole("table", {
    name: candidateTableName,
  });
  // 名前は行ごとの候補の file 名と位置である。
  fireEvent.click(within(table).getByRole("button", { name: /ID: 117(?!\d)/ }));
  const firstStageNames = within(
    within(openStage(0)).getByRole("table", { name: candidateTableName }),
  )
    .getAllByRole("button")
    .map((button) => button.textContent);
  expect(firstStageNames).toHaveLength(2);
  expect(new Set(firstStageNames).size).toBe(2);

  const match = detail.matches.find((candidate) => candidate.stage === 1);
  if (match === undefined) {
    throw new Error("the fixture has no match of the second stage");
  }
  expect(opened).toHaveBeenCalledTimes(1);
  expect(opened).toHaveBeenCalledWith(
    detail.matchRecords[match.candidate].ref,
    detail.matchRecords[detail.matchStages[1].origin].ref,
  );
  expect(opened.mock.calls[0]?.[1]?.sequenceNumber).toBe(113);
});

/** 段階ごとの件数の表の、段階の名前で探せる行を返す。 */
function stageTallyRow(stageLabel: string) {
  const table = screen.getByRole("table", { name: "段階ごとの候補" });
  const row = within(table)
    .getAllByRole("row")
    .find((element) => element.textContent?.startsWith(stageLabel));
  expect(row).toBeTruthy();
  return row as HTMLElement;
}

// 候補を選び出す過程を段階ごとに読む。最後の段階の件数だけでは、時刻の条件が何件を外したかを
// 読めない。
test("推定を経たエッジに、段階ごとに挙げた候補の件数を出す", () => {
  renderMatchedDetail();
  openStage();

  const clockIndependent = stageTallyRow("段階 1: 時刻を使わない条件");
  const secondTimeMatched = stageTallyRow("段階 2: 同じ秒の時刻");
  expect(clockIndependent.children[1]?.textContent).toBe("3");
  expect(secondTimeMatched.children[1]?.textContent).toBe("2");
  // 候補が 1 件以上ある段階は、候補が無い理由を持たない。
  expect(clockIndependent.textContent).toContain("候補あり");
});

// 関連付けを出した段階は候補を 1 件以上挙げる。末尾の段階が 0 件の段階を持つ応答を読まない。
test("関連付けを出した段階が候補を挙げていない応答を読まない", () => {
  const json = matchedEdgeDetailResponseJson();
  const stage = json.matchStages[0];
  expect(() =>
    decodeEdgeDetailResponse(
      {
        ...json,
        matchStages: [
          {
            ...stage,
            stageTallies: [
              { stageKey: "clock_independent", memberCount: 3 },
              {
                stageKey: "second_time_matched",
                memberCount: 0,
                emptyReason: "no_candidate_in_window",
              },
            ],
          },
        ],
      },
      "response",
    ),
  ).toThrow(/matchStages\[0\]\.stageTallies/);
});

// 確度の材料を条件と同じ画面に置く。条件だけを読んで候補を確定と読ませない。
test("推定を経たエッジに、区別できない候補の組を出す", () => {
  renderMatchedDetail();
  openStage();

  expect(foldNote("区別できない候補の組")).toBe("組数: 1");
  const fold = [...document.querySelectorAll("details.fold")].find((element) =>
    element
      .querySelector("summary")
      ?.textContent?.startsWith("区別できない候補の組"),
  );
  // 組の要素は表の位置から、候補 (通番 115) と区別できないレコード (通番 116) に戻る。
  const members = [...(fold?.querySelectorAll("li li") ?? [])].map(
    (item) => item.textContent ?? "",
  );
  expect(members).toHaveLength(2);
  expect(members[0]).toContain("ID: 115");
  expect(members[1]).toContain("ID: 116");
  expect(
    screen.getByText("段階が用いた条件だけでは候補が 1 件に定まらない"),
  ).toBeTruthy();
});

// IP アドレスから端末を決めたことを明示する。レコードが記録した端末と区別する。
test("IP アドレスから端末を決めたエッジに、その根拠を出す", () => {
  renderMatchedDetail();

  expect(
    screen.getByRole("heading", { name: "IP アドレスから端末を決めた根拠" }),
  ).toBeTruthy();
  expect(
    screen.getByRole("table", {
      name: "端末を決める判定",
    }),
  ).toBeTruthy();
  const basisRow = screen
    .getByRole("table", { name: "端末を決める判定" })
    .querySelector("tbody tr");
  expect(basisRow?.textContent).toContain("192.0.2.11");
  expect(
    screen.getByText(/観測の切れ目でも IP の割当が続いている/),
  ).toBeTruthy();
});

// 同じアドレスと端末の割当が収集元ごとに別の期間で 2 件あると、根拠も 2 件になる。
test("同じアドレスと端末の根拠を 1 つにまとめ、収集元ごとの適用期間を出す", () => {
  const json = matchedEdgeDetailResponseJson();
  const second = structuredClone(json.assignmentBases[0]);
  second.sourceId = "source-b";
  // 導いた端末の導き方は、用いた割当を指すため割当ごとに違う。
  second.conditions[0].rightValue[0].text.derivation =
    "srcIP=192.0.2.11; rule=terminal_ip_assignment; source=source-b";
  second.conditions[0].assignmentValidRange.to.rawText =
    "2031/10/08 12:00:00.000";
  second.conditions[0].assignmentValidRange.to.normalized =
    "2031-10-08T12:00:00.000+09:00";
  json.assignmentBases.push(second);
  render(
    <EdgeDetail
      state={{
        status: "loaded",
        value: decodeEdgeDetailResponse(json, "response"),
      }}
      selectedSelector={undefined}
      onSelectGroup={() => {}}
      onSelectRecord={() => {}}
      assertions={() => <p>所見の欄</p>}
      sourceFileNames={
        new Map([
          [json.assignmentBases[0].sourceId, "host-a.log"],
          ["source-b", "host-b.log"],
        ])
      }
    />,
  );

  expect(screen.getAllByText("使った IP アドレス:")).toHaveLength(1);
  const rows = within(
    screen.getByRole("table", { name: "割り当ての適用期間" }),
  ).getAllByRole("row");
  const firstDerivation =
    json.assignmentBases[0].conditions[0].rightValue[0].text.derivation;
  expect(rows.slice(1).map((row) => row.textContent)).toEqual([
    `host-a.log2031-10-08T00:40:09.600Z – 2031-10-08T02:05:53.700Z${firstDerivation}`,
    "host-b.log2031-10-08T00:40:09.600Z – 2031-10-08T03:00:00.000Z" +
      "srcIP=192.0.2.11; rule=terminal_ip_assignment; source=source-b",
  ]);
});

test("割り当ての適用期間は、正規化値が無いときも原文で出す", () => {
  const json = matchedEdgeDetailResponseJson();
  const range = json.assignmentBases[0].conditions[0].assignmentValidRange;
  const { normalized: _normalized, ...withoutNormalized } = range.from;
  range.from = withoutNormalized as typeof range.from;
  renderMatchedDetail(json);

  expect(
    screen.getByRole("table", { name: "割り当ての適用期間" }).textContent,
  ).toContain("2031/10/08 09:40:09.600");
});

/** 見出しが summary で始まる、閉じた欄を開く。 */
function openFold(summary: string) {
  const details = [
    ...document.querySelectorAll<HTMLDetailsElement>("details.fold"),
  ].find((element) =>
    element.querySelector("summary")?.textContent?.startsWith(summary),
  );
  if (details === undefined) {
    throw new Error(`no fold starting with ${summary}`);
  }
  expect(details.open).toBe(false);
  details.open = true;
  fireEvent(details, new Event("toggle"));
}

function scrollRegionTo(name: string, scrollTop: number) {
  const region = screen.getByRole("region", { name });
  region.scrollTop = scrollTop;
  fireEvent.scroll(region);
}

/** 文字列の並びのどれかが、通番 sequenceNumber の位置を出しているか。 */
function mentionsSequence(texts: string[], sequenceNumber: number): boolean {
  const pattern = new RegExp(`ID: ${sequenceNumber}(?!\\d)`);
  return texts.some((text) => pattern.test(text));
}

test("byte 位置だけが異なる根拠のレコードを、同じ key にせず 1 件ずつ出す", () => {
  const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
  onTestFinished(() => consoleError.mockRestore());
  const json = manyEvidenceEdgeDetailResponseJson(2);
  const evidence = json.edge.evidence.map((item, index) => ({
    ...item,
    recordRef: {
      sourceId: item.recordRef.sourceId,
      sourceContentSha256: item.recordRef.sourceContentSha256,
      sourceFileName: "host-a.bin",
      positionKind: "byte_range",
      byteOffset: 1024 * (index + 1),
      byteLength: 64,
      recordRawTextRef: `/api/v0/records?byteOffset=${1024 * (index + 1)}`,
    },
  }));
  renderMatchedDetail({ ...json, edge: { ...json.edge, evidence } });

  expect(
    consoleError.mock.calls.filter((call) =>
      String(call[0]).includes("same key"),
    ),
  ).toEqual([]);
  // 位置の列は値だけを出し、button の名前が「位置: 始まり-終わり」を持つ。
  expect(screen.getByText("1024-1088")).toBeTruthy();
  expect(screen.getByText("2048-2112")).toBeTruthy();
  expect(screen.getByRole("button", { name: /位置: 1024-1088$/ })).toBeTruthy();
});

test("根拠のレコードの表は、値の幅で横に広がり、ノードの根拠の表と同じ列で 1 件を 1 行に出す", () => {
  renderMatchedDetail(manyEvidenceEdgeDetailResponseJson(3));
  const table = screen.getByRole("table", { name: /根拠のレコード/ });

  expect(table.style.width).toBe("max-content");
  expect(
    within(table)
      .getAllByRole("columnheader")
      .map((cell) => visibleText(cell)),
  ).toEqual([
    "Artifact",
    "時刻",
    "Event ID",
    "位置",
    "原文の時刻",
    "イベントの種類",
  ]);
  // 見出しの行と、1 件あたり 1 行。
  expect(within(table).getAllByRole("row")).toHaveLength(1 + 3);
});

// 1 本の関係が根拠を数千件持つことがある。全行を描くと、行の配置の計算で画面が止まる。
test("根拠のレコードは、見えている行とその前後だけを描き、スクロールに合わせて行を入れ替える", async () => {
  const count = 400;
  renderMatchedDetail(manyEvidenceEdgeDetailResponseJson(count));
  // 行数の多い表は閉じた状態で出る。
  expect(screen.queryByRole("table", { name: /根拠のレコード/ })).toBeNull();
  openFold("根拠のレコード");
  const table = screen.getByRole("table", { name: /根拠のレコード/ });
  const rowTexts = () =>
    within(table)
      .getAllByRole("row")
      .slice(1)
      // 位置の列は値だけを出すため、名前と値の組は位置の button の名前から読む。
      .map((row) =>
        [
          row.textContent ?? "",
          ...within(row)
            .getAllByRole("button")
            .map((button) => button.getAttribute("aria-label") ?? ""),
        ].join(" "),
      );
  // 生成した根拠の通番は 920000 から振る。
  const first = 920000;
  const last = first + count - 1;

  expect(foldNote("根拠のレコード")).toBe(`件数: ${count}`);
  expect(table).toHaveAttribute("aria-rowcount", String(count + 1));
  // 見出しの行を 1 行目とし、1 件目のレコードの行は 2 行目である。
  expect(within(table).getAllByRole("row")[1]).toHaveAttribute(
    "aria-rowindex",
    "2",
  );
  expect(rowTexts().length).toBeLessThan(count);
  expect(mentionsSequence(rowTexts(), first)).toBe(true);
  expect(mentionsSequence(rowTexts(), last)).toBe(false);

  scrollRegionTo("根拠のレコードの表", Number.MAX_SAFE_INTEGER);

  await waitFor(() => expect(mentionsSequence(rowTexts(), last)).toBe(true));
  expect(mentionsSequence(rowTexts(), first)).toBe(false);
  expect(rowTexts().length).toBeLessThan(count);
  // 最後のレコードの行は、見出しの行を 1 行目として count + 1 行目である。
  expect(within(table).getAllByRole("row").at(-1)).toHaveAttribute(
    "aria-rowindex",
    String(count + 1),
  );
});

// 段階は数千個になる。見えている段階だけを描き、スクロールで外れた段階を作り直しても、
// 分析者が開いた段階は開いたまま戻る。
test("推定の段階は、見えている段階とその前後だけを描き、スクロールで外れた段階を開いたまま残す", async () => {
  const count = 300;
  renderMatchedDetail(manyStageEdgeDetailResponseJson(count));
  openFold("推定の結果");
  const summaryTexts = () =>
    [...document.querySelectorAll("summary")].map(
      (summary) => summary.textContent ?? "",
    );
  const conditionTableName = "この段階";
  // 生成した段階の起点の通番は 930000 から振る。
  const first = 930000;
  const last = first + count - 1;

  expect(foldNote("推定の結果")).toBe(`件数: ${count} · 段階: ${count}`);
  expect(summaryTexts().length).toBeLessThan(count);
  expect(mentionsSequence(summaryTexts(), first)).toBe(true);
  expect(mentionsSequence(summaryTexts(), last)).toBe(false);
  openStage(0);
  expect(screen.getByRole("table", { name: conditionTableName })).toBeTruthy();

  scrollRegionTo("推定の段階の一覧", Number.MAX_SAFE_INTEGER);

  await waitFor(() =>
    expect(mentionsSequence(summaryTexts(), last)).toBe(true),
  );
  expect(mentionsSequence(summaryTexts(), first)).toBe(false);
  expect(screen.queryByRole("table", { name: conditionTableName })).toBeNull();

  scrollRegionTo("推定の段階の一覧", 0);

  await waitFor(() =>
    expect(mentionsSequence(summaryTexts(), first)).toBe(true),
  );
  expect(document.querySelectorAll("details.match-stage")[0]).toHaveProperty(
    "open",
    true,
  );
  expect(screen.getByRole("table", { name: conditionTableName })).toBeTruthy();
});

test("URL の断片の欄は HTTP の要求の関係だけに、所見の欄の前に出す", () => {
  const base = loadedDetail();
  const renderOf = (detail: EdgeDetailResponse) =>
    render(
      <EdgeDetail
        state={{ status: "loaded", value: detail }}
        selectedSelector={undefined}
        onSelectGroup={() => {}}
        onSelectRecord={() => {}}
        urlFragments={() => <p>断片の欄</p>}
        assertions={() => <p>所見の欄</p>}
      />,
    );
  renderOf(base);
  expect(screen.queryByText("断片の欄")).toBeNull();
  cleanup();

  renderOf({ ...base, edge: { ...base.edge, kind: "http_request" } });
  const fragments = screen.getByText("断片の欄");
  const assertions = screen.getByText("所見の欄");
  expect(
    fragments.compareDocumentPosition(assertions) &
      Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
});
