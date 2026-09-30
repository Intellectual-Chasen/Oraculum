// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, onTestFinished, test, vi } from "vitest";
import type { TerminalAssignmentDraft } from "@/shared/api/terminalAssignments";
import type {
  RecordLocator,
  TimeRange,
  Timestamp,
} from "@/shared/contracts/common";
import type { SourceIdentity } from "@/shared/contracts/sources";
import { decodeSourcesResponse } from "@/shared/contracts/sources";
import {
  decodeTerminalAssignment,
  decodeTerminalAssignmentsResponse,
  type TerminalAssignment,
} from "@/shared/contracts/terminalAssignments";
import { SignedInContext } from "@/shared/ui/SignedInAccount";
import {
  accountLabel,
  signedInAlice,
  signedInViewer,
} from "@/testdata/session";
import { sourcesResponseJson } from "@/testdata/sources/sourcesResponse";
import {
  analystAssignmentJson,
  assignmentSourceId,
  importSpecifiedAssignmentJson,
  observedAssignmentJson,
  terminalAssignmentsResponseJson,
} from "@/testdata/terminalAssignments/terminalAssignmentsResponse";
import { TerminalAssignments } from "./TerminalAssignments";
import type { TerminalAssignmentsView } from "./useTerminalAssignments";

afterEach(cleanup);

const clientIpLabel = "接続元 IP";
const terminalIdLabel = "端末の識別子";
const hostnameLabel = "端末の表示名";
const hostnamesLabel = "ホスト名";
const appliesLabel = "収集元の全体に適用";
const authorLabel = "分析者";
const derivationLabel = "理由";
const submitLabel = "割り当てを記録";

/** 記録の button。 */
function submitButton(): HTMLElement {
  return screen.getByRole("button", { name: submitLabel });
}

/** 「名前: 値」の組のうち、名前が `name` のものの文字列。無ければ undefined。 */
function pairText(name: string): string | undefined {
  return screen
    .queryAllByRole("listitem")
    .map((item) => item.textContent ?? "")
    .find((text) => text.startsWith(`${name}: `));
}

/** 「?」の説明を開き、説明の吹き出しを返す。 */
function openHelp(label: string): HTMLElement {
  fireEvent.click(screen.getByRole("button", { name: `${label} の説明` }));
  return screen.getByRole("tooltip");
}

/**
 * 観測期間の両端を持つ収集元を、操作 1 の応答の fixture から組む。
 *
 * fixture の収集元は期間の始まりだけを持つ。終わりは始まりと同じ時刻を置く。
 * 画面が読むのは両端が揃っていることだけであり、値そのものを判定に使わない。
 */
function sourceWithRange(): SourceIdentity {
  const response = decodeSourcesResponse(sourcesResponseJson(), "$");
  const source = response.sources.find(
    (item) => item.source.observedRangeFirst !== undefined,
  );
  if (source === undefined) {
    throw new Error("the fixture holds no source with an observed range");
  }
  return {
    ...source.source,
    observedRangeLast: source.source.observedRangeFirst,
  };
}

const basisRecordRef: RecordLocator = {
  sourceId: "ingest-host-g-1",
  sourceContentSha256: "c".repeat(64),
  sourceFileName: "host-g.log",
  positionKind: "byte_range",
  byteOffset: 7321,
  byteLength: 300,
  lineNumber: 2048,
  lineCount: 4,
  recordRawTextRef: "/api/v0/records",
};

/** 記録の呼び出しを数え、渡された入力を読める mock を返す。 */
function recordMock() {
  return vi.fn<(draft: TerminalAssignmentDraft) => Promise<void>>(
    async () => {},
  );
}

function viewOf(
  overrides: Partial<TerminalAssignmentsView> = {},
): TerminalAssignmentsView {
  return {
    state: { status: "loaded", value: [] },
    recordFailure: undefined,
    recordedCount: 0,
    recording: false,
    record: recordMock(),
    ...overrides,
  };
}

test("収集元を選んでいないときは、選択なしの状態を出す", () => {
  render(
    <TerminalAssignments
      view={viewOf()}
      selectedSource={undefined}
      basisRecordRef={basisRecordRef}
    />,
  );
  expect(screen.getByText("収集元の選択なし")).toBeDefined();
  expect(screen.queryByLabelText(clientIpLabel)).toBeNull();
});

test("根拠のレコードを開いていないときは、根拠のレコードなしの状態を出す", () => {
  render(
    <TerminalAssignments
      view={viewOf()}
      selectedSource={sourceWithRange()}
      basisRecordRef={undefined}
    />,
  );
  expect(screen.getByText("根拠のレコードなし")).toBeDefined();
  expect(screen.queryByLabelText(clientIpLabel)).toBeNull();
});

test("入力した割当を、適用期間と根拠のレコードとともに記録する", () => {
  const record = recordMock();
  const source = sourceWithRange();
  render(
    <TerminalAssignments
      view={viewOf({ record })}
      selectedSource={source}
      basisRecordRef={basisRecordRef}
    />,
  );
  fireEvent.change(screen.getByLabelText(clientIpLabel), {
    target: { value: "192.0.2.14" },
  });
  fireEvent.change(screen.getByLabelText(terminalIdLabel), {
    target: { value: "host-g" },
  });
  fireEvent.change(screen.getByLabelText(hostnameLabel), {
    target: { value: "host-g.example.test" },
  });
  fireEvent.change(screen.getByLabelText(hostnamesLabel), {
    target: { value: "host-g, host-g.example.test" },
  });
  fireEvent.change(screen.getByLabelText(authorLabel), {
    target: { value: "analyst" },
  });
  fireEvent.change(screen.getByLabelText(derivationLabel), {
    target: { value: "別の端末の ssh の接続先から導いた" },
  });
  fireEvent.click(submitButton());

  expect(record).toHaveBeenCalledTimes(1);
  const draft = record.mock.calls[0][0];
  expect(draft.clientIp).toBe("192.0.2.14");
  expect(draft.terminalId).toBe("host-g");
  expect(draft.terminalHostnames).toEqual(["host-g", "host-g.example.test"]);
  expect(draft.sourceId).toBe(source.sourceId);
  expect(draft.assignmentValidRange.from).toEqual(source.observedRangeFirst);
  // 根拠のレコードは byte 位置で指す。
  expect(draft.basisRecordRefs).toHaveLength(1);
  expect(draft.basisRecordRefs[0].byteOffset).toBe(7321);
  // 既定では、収集元のレコードの全体をその端末のものとしない。
  expect(draft.appliesToSourceId).toBeUndefined();
});

test("分析者が入力した適用期間の始まりと終わりで割当を記録する", () => {
  const record = recordMock();
  const source = sourceWithRange();
  render(
    <TerminalAssignments
      view={viewOf({ record })}
      selectedSource={source}
      basisRecordRef={basisRecordRef}
    />,
  );
  // 根拠のレコードを、file 名と位置の組で出す。
  expect(pairText("根拠のレコード")).toBe("根拠のレコード: host-g.log");
  expect(pairText("行")).toBe("行: 2048-2051");
  expect(pairText("位置")).toBe("位置: 7321-7621");
  // 入力欄の初期値は観測期間の両端である。
  const from = screen.getByRole("textbox", {
    name: "適用期間の始まり",
  }) as HTMLInputElement;
  const to = screen.getByRole("textbox", {
    name: "適用期間の終わり",
  }) as HTMLInputElement;
  expect(from.value).toBe(source.observedRangeFirst?.normalized);
  expect(to.value).toBe(source.observedRangeLast?.normalized);
  fireEvent.change(to, { target: { value: "2031-01-02T03:30:00Z" } });
  fireEvent.change(screen.getByLabelText(clientIpLabel), {
    target: { value: "192.0.2.14" },
  });
  fireEvent.change(screen.getByLabelText(authorLabel), {
    target: { value: "analyst" },
  });
  fireEvent.change(screen.getByLabelText(derivationLabel), {
    target: { value: "合成の筋道" },
  });
  fireEvent.click(submitButton());

  expect(record).toHaveBeenCalledTimes(1);
  const range = record.mock.calls[0][0].assignmentValidRange;
  expect(range.from).toEqual(source.observedRangeFirst);
  expect(range.to.normalized).toBe("2031-01-02T03:30:00Z");
  expect(range.to.rawText).toBe("2031-01-02T03:30:00Z");
});

test("始まりだけを変えた期間で記録し、収集元を選び直すと入力した期間を戻す", () => {
  const record = recordMock();
  const source = sourceWithRange();
  const { rerender } = render(
    <TerminalAssignments
      view={viewOf({ record })}
      selectedSource={source}
      basisRecordRef={basisRecordRef}
    />,
  );
  const from = () =>
    screen.getByRole("textbox", {
      name: "適用期間の始まり",
    }) as HTMLInputElement;
  fireEvent.change(from(), { target: { value: "2031-01-02T03:10:00Z" } });
  fireEvent.change(screen.getByLabelText(clientIpLabel), {
    target: { value: "192.0.2.14" },
  });
  fireEvent.change(screen.getByLabelText(authorLabel), {
    target: { value: "analyst" },
  });
  fireEvent.change(screen.getByLabelText(derivationLabel), {
    target: { value: "合成の筋道" },
  });
  fireEvent.click(submitButton());
  const range = record.mock.calls[0][0].assignmentValidRange;
  expect(range.from.normalized).toBe("2031-01-02T03:10:00Z");
  expect(range.to).toEqual(source.observedRangeLast);

  const other = { ...source, sourceId: "another-source" };
  rerender(
    <TerminalAssignments
      view={viewOf({ record })}
      selectedSource={other}
      basisRecordRef={basisRecordRef}
    />,
  );
  expect(from().value).toBe(other.observedRangeFirst?.normalized);
});

test("適用期間の入力欄を空にしたときは、その入力欄に誤りを出して記録しない", () => {
  const record = recordMock();
  render(
    <TerminalAssignments
      view={viewOf({ record })}
      selectedSource={sourceWithRange()}
      basisRecordRef={basisRecordRef}
    />,
  );
  fireEvent.change(screen.getByRole("textbox", { name: "適用期間の始まり" }), {
    target: { value: " " },
  });
  fireEvent.change(screen.getByLabelText(clientIpLabel), {
    target: { value: "192.0.2.14" },
  });
  fireEvent.change(screen.getByLabelText(authorLabel), {
    target: { value: "analyst" },
  });
  fireEvent.change(screen.getByLabelText(derivationLabel), {
    target: { value: "合成の筋道" },
  });
  fireEvent.click(submitButton());

  expect(record).not.toHaveBeenCalled();
  const from = screen.getByRole("textbox", { name: "適用期間の始まり" });
  expect(from.getAttribute("aria-invalid")).toBe("true");
  expect(screen.getByText("入力が必要")).toBeTruthy();
});

/** 観測期間を持たない収集元。fixture の観測期間を持つ収集元から両端を外す。 */
function sourceWithoutRange(): SourceIdentity {
  return {
    ...sourceWithRange(),
    observedRangeFirst: undefined,
    observedRangeLast: undefined,
  };
}

function localTime(text: string): Timestamp {
  return {
    rawText: text,
    normalized: text,
    normalizedForm: "local_without_offset",
    precision: "second",
    offsetState: "undetermined",
    clock: "terminal_local",
    meaning: "event",
    valueState: "present",
  };
}

/** 地方時の両端と、それを分析者のずれで読んだ観測期間。 */
const localRange: TimeRange = {
  from: localTime("2031-01-02T03:04:05"),
  to: localTime("2031-01-02T03:14:05"),
};
const interpretation = { offset: "+09:00", assertionId: "as:synthetic" };
const interpretedRange: TimeRange = {
  from: { ...localRange.from, interpretation },
  to: { ...localRange.to, interpretation },
};

/** 段落の文字列全体が text と一致する要素を返す。 */
function paragraphReading(text: string): HTMLElement {
  return screen.getByText(
    (_content, element) =>
      element?.tagName === "P" && element.textContent === text,
  );
}

test("記録期間もタイムゾーンも持たない収集元では、記録期間なしを出し、割り当ての入力欄を出さない", () => {
  render(
    <TerminalAssignments
      view={viewOf()}
      selectedSource={sourceWithoutRange()}
      interpretedRange={{ status: "loaded", range: undefined }}
      basisRecordRef={basisRecordRef}
    />,
  );
  expect(screen.getByText("記録期間なし")).toBeTruthy();
  expect(screen.queryByLabelText(clientIpLabel)).toBeNull();
});

test("収集元の一覧を読み込んでいる間は、読み込み中を出す", () => {
  render(
    <TerminalAssignments
      view={viewOf()}
      selectedSource={sourceWithoutRange()}
      interpretedRange={{ status: "loading" }}
      basisRecordRef={basisRecordRef}
    />,
  );
  expect(screen.getByText("収集元の一覧を読み込み中")).toBeTruthy();
  expect(screen.queryByText("記録期間なし")).toBeNull();
  expect(screen.queryByLabelText(clientIpLabel)).toBeNull();
});

test("収集元の一覧を読み込めなかったときは、読み込みの失敗を出す", () => {
  render(
    <TerminalAssignments
      view={viewOf()}
      selectedSource={sourceWithoutRange()}
      interpretedRange={{ status: "failed" }}
      basisRecordRef={basisRecordRef}
    />,
  );
  expect(screen.getByText("収集元の一覧の読み込みに失敗")).toBeTruthy();
  expect(screen.queryByLabelText(clientIpLabel)).toBeNull();
});

test("適用中の解釈が 2 件以上ある収集元では、タイムゾーンの記録の競合を出す", () => {
  render(
    <TerminalAssignments
      view={viewOf()}
      selectedSource={sourceWithoutRange()}
      interpretedRange={{ status: "conflicted" }}
      basisRecordRef={basisRecordRef}
    />,
  );
  expect(screen.getByText("収集元のタイムゾーンの記録が競合")).toBeTruthy();
  expect(screen.queryByLabelText(clientIpLabel)).toBeNull();
});

test("タイムゾーンを当てた記録期間を値の組で出し、タイムゾーンを外した期間で割り当てを記録する", () => {
  const record = recordMock();
  const source = sourceWithoutRange();
  render(
    <TerminalAssignments
      view={viewOf({ record })}
      selectedSource={source}
      interpretedRange={{ status: "loaded", range: interpretedRange }}
      basisRecordRef={basisRecordRef}
    />,
  );
  expect(pairText("収集元")).toBe(`収集元: ${source.fileName}`);
  expect(pairText("適用期間")).toBe(
    "適用期間: 2031-01-02T03:04:05 – 2031-01-02T03:14:05",
  );
  expect(pairText("タイムゾーン")).toBe("タイムゾーン: UTC+09:00");
  expect(pairText("出どころ")).toBe("出どころ: 分析者の記録");
  expect(openHelp("適用期間のタイムゾーン").textContent).toContain(
    "タイムゾーンの記録の取り消し: 割り当てを適用しない",
  );
  fireEvent.change(screen.getByLabelText(clientIpLabel), {
    target: { value: "192.0.2.14" },
  });
  fireEvent.change(screen.getByLabelText(authorLabel), {
    target: { value: "analyst" },
  });
  fireEvent.change(screen.getByLabelText(derivationLabel), {
    target: { value: "別の端末の ssh の接続先から導いた" },
  });
  fireEvent.click(submitButton());

  expect(record).toHaveBeenCalledTimes(1);
  expect(record.mock.calls[0][0].assignmentValidRange).toEqual(localRange);
});

test("端末の識別子を入力したときは、収集元の全体を端末のものとする選択を既定で外し、適用範囲を入力の前に示す", () => {
  const record = recordMock();
  const source = sourceWithRange();
  render(
    <TerminalAssignments
      view={viewOf({ record })}
      selectedSource={source}
      basisRecordRef={basisRecordRef}
    />,
  );
  fireEvent.change(screen.getByLabelText(terminalIdLabel), {
    target: { value: "host-a" },
  });
  const applies = screen.getByLabelText<HTMLInputElement>(appliesLabel);
  expect(applies.disabled).toBe(false);
  expect(applies.checked).toBe(false);
  // 適用範囲は目で読める値の組として出し、選択と結ぶ。
  const note = () =>
    document.getElementById(applies.getAttribute("aria-describedby") ?? "")
      ?.textContent;
  expect(note()).toBe("適用範囲: 接続元 IP と端末の対応");
  expect(openHelp("収集元の全体への適用").textContent).toContain(
    "選択: 端末の識別子を持たない収集元のレコードすべてをこの端末の記録として扱う",
  );
  fireEvent.click(submitButton());
  expect(record.mock.calls[0][0].appliesToSourceId).toBeUndefined();

  fireEvent.click(applies);
  expect(note()).toBe("適用範囲: 収集元の全体");
  fireEvent.click(submitButton());
  const draft = record.mock.calls[1][0];
  expect(draft.terminalId).toBe("host-a");
  expect(draft.appliesToSourceId).toBe(source.sourceId);
});

test("端末の識別子が空のときは、収集元の全体を端末のものとする選択を固定する", () => {
  const record = recordMock();
  const source = sourceWithRange();
  render(
    <TerminalAssignments
      view={viewOf({ record })}
      selectedSource={source}
      basisRecordRef={basisRecordRef}
    />,
  );
  const applies = screen.getByLabelText<HTMLInputElement>(appliesLabel);
  expect(applies.checked).toBe(true);
  expect(applies.disabled).toBe(true);
  expect(pairText("適用範囲")).toBe("適用範囲: 収集元の全体");

  // 識別子を入力すると選択は外れた既定値に戻り、識別子を消すと選択は固定に戻る。
  fireEvent.change(screen.getByLabelText(terminalIdLabel), {
    target: { value: "host-a" },
  });
  expect(applies.checked).toBe(false);
  fireEvent.change(screen.getByLabelText(terminalIdLabel), {
    target: { value: " " },
  });
  expect(applies.checked).toBe(true);
  expect(applies.disabled).toBe(true);

  fireEvent.change(screen.getByLabelText(hostnameLabel), {
    target: { value: "host-a.example.test" },
  });
  fireEvent.click(submitButton());
  const draft = record.mock.calls[0][0];
  expect(draft.appliesToSourceId).toBe(source.sourceId);
});

test("端末の表示名だけを入力した割当を、収集元に適用して記録する", () => {
  const record = recordMock();
  const source = sourceWithRange();
  render(
    <TerminalAssignments
      view={viewOf({ record })}
      selectedSource={source}
      basisRecordRef={basisRecordRef}
    />,
  );
  fireEvent.change(screen.getByLabelText(hostnameLabel), {
    target: { value: "host-a.example.test" },
  });
  fireEvent.change(screen.getByLabelText(authorLabel), {
    target: { value: "analyst-a" },
  });
  fireEvent.change(screen.getByLabelText(derivationLabel), {
    target: { value: "取り込んだ file の名前から導いた" },
  });
  fireEvent.click(submitButton());

  expect(record).toHaveBeenCalledTimes(1);
  const draft = record.mock.calls[0][0];
  expect(draft.terminalHostname).toBe("host-a.example.test");
  expect(draft.clientIp).toBe("");
  expect(draft.terminalId).toBe("");
  expect(draft.appliesToSourceId).toBe(source.sourceId);
});

test("閲覧者の役割では、記録のボタンを無効にして理由を出し、form を送っても記録しない", () => {
  const record = recordMock();
  render(
    <SignedInContext.Provider value={signedInViewer}>
      <TerminalAssignments
        view={viewOf({ record })}
        selectedSource={sourceWithRange()}
        basisRecordRef={basisRecordRef}
      />
    </SignedInContext.Provider>,
  );

  expect(screen.getByText("閲覧者は記録不可")).toBeTruthy();
  const submit = submitButton();
  expect(submit.getAttribute("aria-disabled")).toBe("true");
  fireEvent.change(screen.getByLabelText(hostnameLabel), {
    target: { value: "host-a.example.test" },
  });
  // 記録のボタンは割当の form の中にあり、closest("form") は null にならない。
  fireEvent.submit(submit.closest("form") as HTMLFormElement);

  expect(record).not.toHaveBeenCalled();
});

test("ログインしているときは分析者の入力欄を出さず、ログインした利用者を出して著者を送らない", () => {
  const record = recordMock();
  render(
    <SignedInContext.Provider value={signedInAlice}>
      <TerminalAssignments
        view={viewOf({ record })}
        selectedSource={sourceWithRange()}
        basisRecordRef={basisRecordRef}
      />
    </SignedInContext.Provider>,
  );

  expect(screen.queryByLabelText(authorLabel)).toBeNull();
  expect(paragraphReading(`分析者: ${accountLabel}`)).toBeTruthy();
  fireEvent.change(screen.getByLabelText(hostnameLabel), {
    target: { value: "host-a.example.test" },
  });
  fireEvent.change(screen.getByLabelText(derivationLabel), {
    target: { value: "取り込んだ file の名前から導いた" },
  });
  fireEvent.click(submitButton());

  expect(record).toHaveBeenCalledTimes(1);
  expect(record.mock.calls[0][0].author).toBeUndefined();
});

test("接続元 IP だけを入力した割当を、収集元に適用して記録する", () => {
  const record = recordMock();
  const source = sourceWithRange();
  render(
    <TerminalAssignments
      view={viewOf({ record })}
      selectedSource={source}
      basisRecordRef={basisRecordRef}
    />,
  );
  fireEvent.change(screen.getByLabelText(clientIpLabel), {
    target: { value: "192.0.2.10" },
  });
  fireEvent.click(submitButton());

  expect(record).toHaveBeenCalledTimes(1);
  const draft = record.mock.calls[0][0];
  expect(draft.clientIp).toBe("192.0.2.10");
  expect(draft.terminalHostname).toBe("");
  expect(draft.appliesToSourceId).toBe(source.sourceId);
});

test("記録した割り当てが 1 件で端末を確定させることを、「?」を開かずに読めるラベルで出す", () => {
  render(
    <TerminalAssignments
      view={viewOf()}
      selectedSource={sourceWithRange()}
      basisRecordRef={basisRecordRef}
    />,
  );
  expect(pairText("確定")).toBe("確定: 記録 1 件で端末を確定");
});

/** 割当の一覧の列の見出し。セルを列の見出しで探すために使う。 */
const listColumns = [
  "接続元 IP",
  "端末 ID",
  "端末の表示名",
  "記録した方法",
  "割り当てる収集元",
  "期間の収集元",
  "理由",
  "分析者",
  "根拠のレコード",
] as const;

/** 記録した方法の表示を持つ行の、列の見出しごとのセルの文字列を返す。 */
function rowCells(originLabel: string): Record<string, string> {
  const table = screen.getByRole("table", {
    name: /分析者が記録した割り当て/,
  });
  const headers = within(table)
    .getAllByRole("columnheader")
    .map((header) => header.textContent ?? "");
  // 記録した方法の「分析者」は列の見出しと同じ文字列なので、セルの中だけを探す。
  const row = within(table)
    .getAllByRole("cell")
    .find(
      (cell) =>
        cell.textContent === originLabel &&
        headers[
          Array.from(cell.parentElement?.children ?? []).indexOf(cell)
        ] === "記録した方法",
    )
    ?.closest("tr");
  if (row === null || row === undefined) {
    throw new Error(`no row carries ${originLabel}`);
  }
  const cells = within(row).getAllByRole("cell");
  const byColumn: Record<string, string> = {};
  for (const column of listColumns) {
    byColumn[column] = cells[headers.indexOf(column)]?.textContent ?? "";
  }
  return byColumn;
}

test("記録済みの割当を、由来ごとの表示と省いた項目の欠測とともに出す", () => {
  const { assignments } = decodeTerminalAssignmentsResponse(
    terminalAssignmentsResponseJson(),
    "$",
  );
  const observed = decodeTerminalAssignment(observedAssignmentJson(), "$");
  render(
    <TerminalAssignments
      view={viewOf({
        state: { status: "loaded", value: [...assignments, observed] },
      })}
      selectedSource={sourceWithRange()}
      basisRecordRef={basisRecordRef}
      sourceFileNames={new Map([[assignmentSourceId, "host-a.log"]])}
    />,
  );
  // 値の無い欄は印「—」を出し、理由は tooltip と読み上げの文に持つ。
  const missing = "—指定なし";

  expect(rowCells("起動時の指定")).toEqual({
    "接続元 IP": missing,
    "端末 ID": missing,
    端末の表示名: "host-a.example.test",
    記録した方法: "起動時の指定",
    割り当てる収集元: "host-a.log",
    期間の収集元: "host-a.log",
    理由: missing,
    分析者: missing,
    根拠のレコード: missing,
  });
  expect(rowCells("分析者")).toEqual({
    "接続元 IP": "192.0.2.10",
    "端末 ID": missing,
    端末の表示名: missing,
    記録した方法: "分析者",
    割り当てる収集元: "host-a.log",
    期間の収集元: "host-a.log",
    理由: "別の端末の ssh の接続先から導いた",
    分析者: "analyst-a",
    根拠のレコード: "1 件",
  });
  // 収集元の全体に付けない割当は、接続元 IP と端末の対応だけを表す。
  expect(rowCells("収集元のレコード")).toEqual({
    "接続元 IP": "192.0.2.20",
    "端末 ID": "host-b",
    端末の表示名: "host-b.example.test",
    記録した方法: "収集元のレコード",
    割り当てる収集元: "—接続元 IP で判定",
    期間の収集元: "host-a.log",
    理由: missing,
    分析者: missing,
    根拠のレコード: missing,
  });
});

test("適用する収集元を file 名で出し、表に無い収集元は sourceId で出す", () => {
  const assignment = decodeTerminalAssignment(
    importSpecifiedAssignmentJson(),
    "$",
  );
  const renderWith = (names: ReadonlyMap<string, string>) =>
    render(
      <TerminalAssignments
        view={viewOf({ state: { status: "loaded", value: [assignment] } })}
        selectedSource={sourceWithRange()}
        basisRecordRef={basisRecordRef}
        sourceFileNames={names}
      />,
    );
  // 適用する収集元と期間を取った収集元の 2 列に出る。
  renderWith(new Map([[assignmentSourceId, "Host-A.log"]]));
  expect(screen.getAllByText("Host-A.log")).toHaveLength(2);
  expect(screen.queryByText(assignmentSourceId)).toBeNull();
  cleanup();

  renderWith(new Map());
  expect(screen.getAllByText(assignmentSourceId)).toHaveLength(2);
});

test("同じ内容の割当が並んでも、別の行として出す", () => {
  const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
  onTestFinished(() => consoleError.mockRestore());
  const recorded = decodeTerminalAssignment(analystAssignmentJson(), "$");
  const assignments: TerminalAssignment[] = [recorded, { ...recorded }];
  render(
    <TerminalAssignments
      view={viewOf({ state: { status: "loaded", value: assignments } })}
      selectedSource={sourceWithRange()}
      basisRecordRef={basisRecordRef}
    />,
  );
  expect(
    screen
      .getAllByRole("cell")
      .filter((cell) => cell.textContent === "analyst-a"),
  ).toHaveLength(assignments.length);
  // React は同じ key を持つ兄弟を console.error で知らせる。
  const keyWarnings = consoleError.mock.calls.filter((call) =>
    String(call[0]).includes("same key"),
  );
  expect(keyWarnings).toEqual([]);
});

test("記録している間は、同じ割当をもう一度送らない", () => {
  const record = recordMock();
  render(
    <TerminalAssignments
      view={viewOf({ record, recording: true })}
      selectedSource={sourceWithRange()}
      basisRecordRef={basisRecordRef}
    />,
  );
  const submit = submitButton();
  expect(submit.getAttribute("aria-disabled")).toBe("true");
  // submit の事象は form まで伝わる。
  fireEvent.submit(submit);
  expect(record).not.toHaveBeenCalled();
});

test("区切りだけのホスト名は空の並びにし、記録できたら全体に適用する選択を外す", () => {
  const record = recordMock();
  const source = sourceWithRange();
  const { rerender } = render(
    <TerminalAssignments
      view={viewOf({ record })}
      selectedSource={source}
      basisRecordRef={basisRecordRef}
    />,
  );
  fireEvent.change(screen.getByLabelText(terminalIdLabel), {
    target: { value: "host-a" },
  });
  fireEvent.change(screen.getByLabelText(hostnamesLabel), {
    target: { value: " , ," },
  });
  const applies = screen.getByLabelText<HTMLInputElement>(appliesLabel);
  fireEvent.click(applies);
  fireEvent.click(submitButton());
  expect(record.mock.calls[0][0].terminalHostnames).toEqual([]);
  expect(applies.checked).toBe(true);

  rerender(
    <TerminalAssignments
      view={viewOf({ record, recordedCount: 1 })}
      selectedSource={source}
      basisRecordRef={basisRecordRef}
    />,
  );
  fireEvent.change(screen.getByLabelText(terminalIdLabel), {
    target: { value: "host-b" },
  });
  expect(screen.getByLabelText<HTMLInputElement>(appliesLabel).checked).toBe(
    false,
  );
});
