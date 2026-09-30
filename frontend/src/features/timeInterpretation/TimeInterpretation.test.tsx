// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { Assertion } from "@/shared/contracts/assertions";
import type { RecordLocator } from "@/shared/contracts/common";
import {
  decodeSourcesResponse,
  type SourceIdentity,
} from "@/shared/contracts/sources";
import { SignedInContext } from "@/shared/ui/SignedInAccount";
import {
  accountLabel,
  signedInAlice,
  signedInViewer,
} from "@/testdata/session";
import { sourcesResponseJson } from "@/testdata/sources/sourcesResponse";
import { TimeInterpretation } from "./TimeInterpretation";
import type { TimeInterpretationsView } from "./useTimeInterpretations";

afterEach(cleanup);

const offsetLabel = "タイムゾーン";
const authorLabel = "分析者";
const noteLabel = "メモ";
const recordLabel = "タイムゾーンを記録";
const reviseLabel = "タイムゾーンを変更";
const addBasisLabel = "開いているレコードを根拠に追加";
const reviseWithMineLabel = "入力した値で上書き";
const takeTheirsLabel = "先に保存された値を採用";

/** 「名前: 値」の組のうち、名前が `name` のものの文字列。無ければ undefined。 */
function pairText(name: string): string | undefined {
  return screen
    .queryAllByRole("listitem")
    .map((item) => item.textContent ?? "")
    .find((text) => text.startsWith(`${name}: `));
}

function firstSource(): SourceIdentity {
  const source = decodeSourcesResponse(sourcesResponseJson(), "$").sources[0]
    ?.source;
  if (source === undefined) {
    throw new Error("the fixture holds no source");
  }
  return source;
}

const utcRecord: RecordLocator = {
  sourceId: "ingest-utc-1",
  sourceContentSha256: "d".repeat(64),
  sourceFileName: "utc.log",
  positionKind: "line_number",
  lineNumber: 41,
  recordRawTextRef: "/api/v0/records",
};

function viewOf(
  overrides: Partial<TimeInterpretationsView> = {},
): TimeInterpretationsView {
  return {
    state: { status: "loaded", value: new Map() },
    recordFailure: undefined,
    conflict: undefined,
    dismissConflict: vi.fn(),
    recordedCount: 0,
    lastRecordedSource: undefined,
    recording: false,
    create: vi.fn(),
    revise: vi.fn(),
    ...overrides,
  };
}

function interpretationOf(source: SourceIdentity): Assertion {
  const basis = {
    note: "同じ要求の UTC の行と比べた",
    recordRefs: [
      {
        sourceContentSha256: utcRecord.sourceContentSha256,
        positionKind: "line_number" as const,
        lineNumber: 41,
      },
    ],
  };
  return {
    id: "as:synthetic",
    target: { kind: "source", sourceContentSha256: source.contentSha256 },
    state: "active",
    author: "analyst-b",
    recordedAt: "2031-01-02T03:05:00.000Z",
    basis,
    timeOffset: "+09:00",
    revisionNumber: 2,
    history: [
      {
        revisionNumber: 1,
        state: "active",
        author: "analyst-a",
        recordedAt: "2031-01-02T03:04:00.000Z",
        basis,
        timeOffset: "+00:00",
      },
    ],
  };
}

/** 収集元 1 件の解釈を持つ一覧の状態。 */
function loadedWith(
  source: SourceIdentity,
  assertion: Assertion,
): TimeInterpretationsView["state"] {
  return {
    status: "loaded",
    value: new Map([
      [source.contentSha256, { kind: "single" as const, assertion }],
    ]),
  };
}

const withdrawLabel = "記録を取り消す";

test("解釈の無い収集元は、タイムゾーン不明を出し、タイムゾーンの入力欄を空欄から始めて最初の改訂を記録する", () => {
  const source = firstSource();
  const create = vi.fn();
  render(
    <TimeInterpretation
      view={viewOf({ create })}
      selectedSource={source}
      openedRecordRef={utcRecord}
      sourceFileNamesByContent={new Map()}
    />,
  );
  expect(pairText("記録")).toBe("記録: なし");
  expect(pairText("タイムゾーン")).toBe("タイムゾーン: タイムゾーン不明");
  expect((screen.getByLabelText(offsetLabel) as HTMLInputElement).value).toBe(
    "",
  );
  fireEvent.change(screen.getByLabelText(offsetLabel), {
    target: { value: "+09:00" },
  });
  fireEvent.change(screen.getByLabelText(authorLabel), {
    target: { value: "analyst-a" },
  });
  fireEvent.change(screen.getByLabelText(noteLabel), {
    target: { value: "同じ要求の UTC の行と 9 時間ずれていた" },
  });
  fireEvent.click(screen.getByRole("button", { name: addBasisLabel }));
  expect(
    screen
      .getByRole("button", { name: addBasisLabel })
      .getAttribute("aria-disabled"),
  ).toBe("true");
  expect(pairText("根拠のレコード")).toBe("根拠のレコード: 1");
  const basisTable = screen.getByRole("table", { name: "根拠のレコード" });
  expect(within(basisTable).getAllByRole("cell")[0]?.textContent).toBe(
    "収集元: utc.log行: 41",
  );
  fireEvent.click(screen.getByRole("button", { name: recordLabel }));

  expect(create).toHaveBeenCalledTimes(1);
  expect(create.mock.calls[0]?.[0]).toEqual({
    target: { kind: "source", sourceContentSha256: source.contentSha256 },
    author: "analyst-a",
    note: "同じ要求の UTC の行と 9 時間ずれていた",
    recordRefs: [
      {
        sourceContentSha256: utcRecord.sourceContentSha256,
        positionKind: "line_number",
        lineNumber: 41,
        sequenceNumber: undefined,
        byteOffset: undefined,
      },
    ],
    timeOffset: "+09:00",
    state: "active",
  });
});

test("記録の途中は、記録の button を押しても入力欄で form を送っても、もう一度記録しない", () => {
  const create = vi.fn();
  render(
    <TimeInterpretation
      view={viewOf({ create, recording: true })}
      selectedSource={firstSource()}
      openedRecordRef={utcRecord}
      sourceFileNamesByContent={new Map()}
    />,
  );
  fireEvent.change(screen.getByLabelText(offsetLabel), {
    target: { value: "+09:00" },
  });
  const button = screen.getByRole("button", { name: recordLabel });
  fireEvent.click(button);
  const form = button.closest("form");
  if (form === null) throw new Error("記録の form が無い");
  fireEvent.submit(form);
  expect(create).not.toHaveBeenCalled();
});

test("ログインしているときは名前の入力欄を出さず、ログインした利用者を出して著者を送らない", () => {
  const create = vi.fn();
  render(
    <SignedInContext.Provider value={signedInAlice}>
      <TimeInterpretation
        view={viewOf({ create })}
        selectedSource={firstSource()}
        openedRecordRef={utcRecord}
        sourceFileNamesByContent={new Map()}
      />
    </SignedInContext.Provider>,
  );

  expect(screen.queryByLabelText(authorLabel)).toBeNull();
  expect(
    screen.getByText(
      (_content, element) =>
        element?.tagName === "P" &&
        element.textContent === `分析者: ${accountLabel}`,
    ),
  ).toBeTruthy();
  fireEvent.change(screen.getByLabelText(offsetLabel), {
    target: { value: "+09:00" },
  });
  fireEvent.click(screen.getByRole("button", { name: recordLabel }));

  expect(create).toHaveBeenCalledTimes(1);
  expect(create.mock.calls[0]?.[0].author).toBeUndefined();
});

test("閲覧者の役割では、ずれの欄と記録のボタンを無効にし、理由を出す", () => {
  render(
    <SignedInContext.Provider value={signedInViewer}>
      <TimeInterpretation
        view={viewOf({})}
        selectedSource={firstSource()}
        openedRecordRef={utcRecord}
        sourceFileNamesByContent={new Map()}
      />
    </SignedInContext.Provider>,
  );

  expect(screen.getByText("閲覧者は記録不可")).toBeTruthy();
  expect(screen.getByLabelText(offsetLabel)).toBeDisabled();
  expect(screen.getByRole("button", { name: recordLabel })).toBeDisabled();
});

test("解釈のある収集元は、現在のずれと改訂ごとの状態・ずれ・分析者の履歴を出す", () => {
  const source = firstSource();
  const assertion = interpretationOf(source);
  render(
    <TimeInterpretation
      view={viewOf({ state: loadedWith(source, assertion) })}
      selectedSource={source}
      openedRecordRef={undefined}
      sourceFileNamesByContent={
        new Map([[utcRecord.sourceContentSha256, "utc.log"]])
      }
    />,
  );
  expect(pairText("タイムゾーン")).toBe("タイムゾーン: UTC+09:00");
  expect(pairText("出どころ")).toBe("出どころ: 分析者の記録");
  expect(pairText("分析者")).toBe("分析者: analyst-b");
  const table = screen.getByRole("table", { name: "変更履歴" });
  const rows = within(table)
    .getAllByRole("row")
    .slice(1)
    .map((row) =>
      within(row)
        .getAllByRole("cell")
        .slice(0, 4)
        .map((cell) => cell.textContent),
    );
  expect(rows).toEqual([
    ["1", "適用中", "UTC+00:00", "analyst-a"],
    ["2", "適用中", "UTC+09:00", "analyst-b"],
  ]);
  expect(
    within(table)
      .getAllByRole("listitem")
      .filter((item) => item.textContent === "収集元: utc.log").length,
  ).toBe(assertion.revisionNumber);
});

test("起動でタイムゾーンを指定した収集元は、起動時の指定を出し、解釈を記録すると分析者の記録を出す", () => {
  const source = firstSource();
  const { rerender } = render(
    <TimeInterpretation
      view={viewOf()}
      selectedSource={source}
      openedRecordRef={undefined}
      sourceFileNamesByContent={new Map()}
      importTimeOffset="+00:00"
    />,
  );
  expect(pairText("記録")).toBe("記録: なし");
  expect(pairText("タイムゾーン")).toBe("タイムゾーン: UTC+00:00");
  expect(pairText("出どころ")).toBe("出どころ: 起動時の指定");

  rerender(
    <TimeInterpretation
      view={viewOf({ state: loadedWith(source, interpretationOf(source)) })}
      selectedSource={source}
      openedRecordRef={undefined}
      sourceFileNamesByContent={new Map()}
      importTimeOffset="+00:00"
    />,
  );
  expect(pairText("タイムゾーン")).toBe("タイムゾーン: UTC+09:00");
  expect(pairText("出どころ")).toBe("出どころ: 分析者の記録");
  expect(pairText("起動時の指定")).toBe("起動時の指定: UTC+00:00");
});

test("ずれの欄を書き換えてから取り消しても、取り消しの改訂は適用中のずれを持つ", () => {
  const source = firstSource();
  const assertion = interpretationOf(source);
  const revise = vi.fn();
  const create = vi.fn();
  render(
    <TimeInterpretation
      view={viewOf({ state: loadedWith(source, assertion), revise, create })}
      selectedSource={source}
      openedRecordRef={undefined}
      sourceFileNamesByContent={new Map()}
    />,
  );
  fireEvent.change(screen.getByLabelText(offsetLabel), {
    target: { value: "+05:30" },
  });
  fireEvent.change(screen.getByLabelText(authorLabel), {
    target: { value: "analyst-c" },
  });
  // 取り消す理由をメモに書くまでは押せない。
  expect(
    screen
      .getByRole("button", { name: withdrawLabel })
      .getAttribute("aria-disabled"),
  ).toBe("true");
  fireEvent.change(screen.getByLabelText(noteLabel), {
    target: { value: "比べた行が別の事象だった" },
  });
  fireEvent.click(screen.getByRole("button", { name: withdrawLabel }));

  expect(create).not.toHaveBeenCalled();
  expect(revise).toHaveBeenCalledTimes(1);
  expect(revise.mock.calls[0]?.[0]).toBe(assertion.id);
  expect(revise.mock.calls[0]?.[1]).toEqual({
    target: { kind: "source", sourceContentSha256: source.contentSha256 },
    author: "analyst-c",
    note: "比べた行が別の事象だった",
    recordRefs: [],
    timeOffset: "+09:00",
    state: "withdrawn",
    baseRevision: assertion.revisionNumber,
  });
});

test("別の分析者の改訂と競合したら、相手の値と自分の値を並べ、自分の値で改訂すると相手の改訂を元に送り直す", () => {
  const source = firstSource();
  const theirs = {
    ...interpretationOf(source),
    revisionNumber: 3,
    author: "analyst-d",
    timeOffset: "+08:00",
    basis: { note: "相手の根拠", recordRefs: [] },
  };
  const mine = {
    target: {
      kind: "source" as const,
      sourceContentSha256: source.contentSha256,
    },
    author: "analyst-c",
    note: "自分の根拠",
    recordRefs: [],
    timeOffset: "+05:30",
    state: "active" as const,
    baseRevision: 2,
  };
  const revise = vi.fn();
  const dismissConflict = vi.fn();
  render(
    <TimeInterpretation
      view={viewOf({
        state: loadedWith(source, theirs),
        conflict: { sourceContentSha256: source.contentSha256, theirs, mine },
        revise,
        dismissConflict,
      })}
      selectedSource={source}
      openedRecordRef={undefined}
      sourceFileNamesByContent={new Map()}
    />,
  );

  expect(screen.getByRole("alert").textContent).toContain(
    "別の分析者の改訂と競合",
  );
  expect(pairText("改訂した分析者")).toBe("改訂した分析者: analyst-d");
  expect(pairText("改訂した時刻")).toBe(`改訂した時刻: ${theirs.recordedAt}`);
  const table = screen.getByRole("table", {
    name: "先に保存された値と入力した値",
  });
  const rows = within(table)
    .getAllByRole("row")
    .slice(1)
    .map((row) => [
      within(row).getByRole("rowheader").textContent,
      ...within(row)
        .getAllByRole("cell")
        .map((cell) => cell.textContent),
    ]);
  expect(rows).toEqual([
    ["状態", "適用中", "適用中"],
    ["タイムゾーン", "UTC+08:00", "UTC+05:30"],
    ["メモ", "相手の根拠", "自分の根拠"],
    ["根拠のレコード", "0", "0"],
  ]);

  fireEvent.click(screen.getByRole("button", { name: reviseWithMineLabel }));
  expect(revise).toHaveBeenCalledTimes(1);
  expect(revise.mock.calls[0]?.[0]).toBe(theirs.id);
  expect(revise.mock.calls[0]?.[1]).toEqual({ ...mine, baseRevision: 3 });
  expect(dismissConflict).not.toHaveBeenCalled();
});

test("先に保存された値を採用すると、入力欄をその値にして何も送らず、その後の記録はその根拠のレコードをそのまま送る", () => {
  const source = firstSource();
  const base = interpretationOf(source);
  const theirs = {
    ...base,
    timeOffset: "+08:00",
    basis: { note: "相手の根拠", recordRefs: base.basis.recordRefs },
  };
  const revise = vi.fn();
  const create = vi.fn();
  const dismissConflict = vi.fn();
  render(
    <TimeInterpretation
      view={viewOf({
        state: loadedWith(source, theirs),
        conflict: {
          sourceContentSha256: source.contentSha256,
          theirs,
          mine: {
            target: {
              kind: "source",
              sourceContentSha256: source.contentSha256,
            },
            note: "自分の根拠",
            recordRefs: [],
            timeOffset: "+05:30",
            state: "active",
            baseRevision: 1,
          },
        },
        revise,
        create,
        dismissConflict,
      })}
      selectedSource={source}
      openedRecordRef={undefined}
      sourceFileNamesByContent={
        new Map([[utcRecord.sourceContentSha256, "utc.log"]])
      }
    />,
  );
  const table = screen.getByRole("table", {
    name: "先に保存された値と入力した値",
  });
  expect(
    within(table).getByRole("rowheader", { name: "根拠のレコード" })
      .parentElement?.textContent,
  ).toBe("根拠のレコード1収集元: utc.log行: 410");
  fireEvent.change(screen.getByLabelText(offsetLabel), {
    target: { value: "+05:30" },
  });

  fireEvent.click(screen.getByRole("button", { name: takeTheirsLabel }));

  expect((screen.getByLabelText(offsetLabel) as HTMLInputElement).value).toBe(
    "+08:00",
  );
  expect((screen.getByLabelText(noteLabel) as HTMLTextAreaElement).value).toBe(
    "相手の根拠",
  );
  expect(dismissConflict).toHaveBeenCalledTimes(1);
  expect(revise).not.toHaveBeenCalled();
  expect(create).not.toHaveBeenCalled();
  const basisRows = () =>
    screen
      .queryByRole("table", { name: "根拠のレコード" })
      ?.querySelectorAll("tbody tr");
  expect(basisRows()?.[0]?.textContent).toBe(
    "収集元: utc.log行: 41先に保存された値",
  );

  fireEvent.change(screen.getByLabelText(authorLabel), {
    target: { value: "analyst-c" },
  });
  fireEvent.click(screen.getByRole("button", { name: reviseLabel }));
  expect(revise).toHaveBeenCalledTimes(1);
  expect(revise.mock.calls[0]?.[1]).toEqual({
    target: { kind: "source", sourceContentSha256: source.contentSha256 },
    author: "analyst-c",
    note: "相手の根拠",
    recordRefs: theirs.basis.recordRefs,
    timeOffset: "+08:00",
    state: "active",
    baseRevision: theirs.revisionNumber,
  });

  // 引き継いだレコードを外してから記録すると、そのレコードを送らない。
  fireEvent.click(screen.getByRole("button", { name: / を根拠から除外$/ }));
  expect(basisRows()).toBeUndefined();
  fireEvent.click(screen.getByRole("button", { name: reviseLabel }));
  expect(revise).toHaveBeenCalledTimes(2);
  expect(revise.mock.calls[1]?.[1].recordRefs).toEqual([]);
});

/** 取り消しの競合の表示を組む。 */
function renderWithdrawalConflict(theirState: "active" | "withdrawn") {
  const source = firstSource();
  const theirs = {
    ...interpretationOf(source),
    state: theirState,
    revisionNumber: 5,
    timeOffset: "+08:00",
  };
  const revise = vi.fn();
  render(
    <TimeInterpretation
      view={viewOf({
        state: loadedWith(source, theirs),
        conflict: {
          sourceContentSha256: source.contentSha256,
          theirs,
          mine: {
            target: {
              kind: "source",
              sourceContentSha256: source.contentSha256,
            },
            note: "取り消す理由",
            recordRefs: [],
            timeOffset: "+09:00",
            state: "withdrawn",
            baseRevision: 2,
          },
        },
        revise,
      })}
      selectedSource={source}
      openedRecordRef={undefined}
      sourceFileNamesByContent={new Map()}
    />,
  );
  return { theirs, revise };
}

test("取り消しの競合で送り直すと、相手の現在のずれを取り消す", () => {
  const { theirs, revise } = renderWithdrawalConflict("active");
  fireEvent.click(screen.getByRole("button", { name: reviseWithMineLabel }));
  expect(revise.mock.calls[0]?.[1]).toMatchObject({
    state: "withdrawn",
    timeOffset: "+08:00",
    baseRevision: theirs.revisionNumber,
  });
});

test("相手がすでに取り消しているときは、送り直しのボタンを出さずに取り消し済みを出す", () => {
  renderWithdrawalConflict("withdrawn");
  expect(
    screen.queryByRole("button", { name: reviseWithMineLabel }),
  ).toBeNull();
  expect(screen.getByText("取り消し済み")).toBeTruthy();
});

test("適用中の解釈が競合している収集元は、競合と適用中の記録の数を出す", () => {
  const source = firstSource();
  const assertion = interpretationOf(source);
  render(
    <TimeInterpretation
      view={viewOf({
        state: {
          status: "loaded",
          value: new Map([
            [
              source.contentSha256,
              {
                kind: "conflicted" as const,
                assertions: [assertion, { ...assertion, id: "as:other" }],
              },
            ],
          ]),
        },
      })}
      selectedSource={source}
      openedRecordRef={undefined}
      sourceFileNamesByContent={new Map()}
    />,
  );
  expect(pairText("記録")).toContain("記録: 競合");
  expect(pairText("適用中の記録")).toBe("適用中の記録: 2");
  expect(pairText("タイムゾーン")).toBe("タイムゾーン: タイムゾーン不明");
  expect(screen.queryByLabelText(offsetLabel)).toBeNull();
});

test("解釈が競合していて起動でタイムゾーンを指定した収集元は、起動時の指定を出す", () => {
  const source = firstSource();
  const assertion = interpretationOf(source);
  render(
    <TimeInterpretation
      view={viewOf({
        state: {
          status: "loaded",
          value: new Map([
            [
              source.contentSha256,
              {
                kind: "conflicted" as const,
                assertions: [assertion, { ...assertion, id: "as:other" }],
              },
            ],
          ]),
        },
      })}
      selectedSource={source}
      openedRecordRef={undefined}
      sourceFileNamesByContent={new Map()}
      importTimeOffset="+09:00"
    />,
  );
  expect(pairText("記録")).toContain("記録: 競合");
  expect(pairText("タイムゾーン")).toBe("タイムゾーン: UTC+09:00");
  expect(pairText("出どころ")).toBe("出どころ: 起動時の指定");
});

test("記録の成功でメモを空へ戻すのは、成功した収集元を表示しているときだけである", () => {
  const source = firstSource();
  const render_ = (lastRecordedSource: string, recordedCount: number) => (
    <TimeInterpretation
      view={viewOf({ lastRecordedSource, recordedCount })}
      selectedSource={source}
      openedRecordRef={undefined}
      sourceFileNamesByContent={new Map()}
    />
  );
  const { rerender } = render(render_("0".repeat(64), 0));
  fireEvent.change(screen.getByLabelText(noteLabel), {
    target: { value: "書きかけの根拠" },
  });
  const note = () =>
    (screen.getByLabelText(noteLabel) as HTMLTextAreaElement).value;

  rerender(render_("0".repeat(64), 1));
  expect(note()).toBe("書きかけの根拠");
  rerender(render_(source.contentSha256, 2));
  expect(note()).toBe("");
});

test("記録の失敗は、失敗した収集元を表示しているときだけ出す", () => {
  const source = firstSource();
  const failure = {
    kind: "request_rejected" as const,
    summary: "タイムゾーンの記録",
    nextAction: "指定を直して再実行",
  };
  const { rerender } = render(
    <TimeInterpretation
      view={viewOf({
        recordFailure: { sourceContentSha256: "0".repeat(64), failure },
      })}
      selectedSource={source}
      openedRecordRef={undefined}
      sourceFileNamesByContent={new Map()}
    />,
  );
  expect(screen.queryByRole("alert")).toBeNull();
  rerender(
    <TimeInterpretation
      view={viewOf({
        recordFailure: { sourceContentSha256: source.contentSha256, failure },
      })}
      selectedSource={source}
      openedRecordRef={undefined}
      sourceFileNamesByContent={new Map()}
    />,
  );
  expect(screen.getByRole("alert").textContent).toContain(failure.summary);
});

test("収集元を選んでいないときは、選択なしの状態を出す", () => {
  render(
    <TimeInterpretation
      view={viewOf()}
      selectedSource={undefined}
      openedRecordRef={utcRecord}
      sourceFileNamesByContent={new Map()}
    />,
  );
  expect(screen.getByText("収集元の選択なし")).toBeDefined();
  expect(screen.queryByLabelText(offsetLabel)).toBeNull();
});
