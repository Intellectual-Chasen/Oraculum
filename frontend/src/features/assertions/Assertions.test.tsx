// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import type { AssertionDraft } from "@/shared/api/assertions";
import {
  type AssertionItem,
  type AssertionTarget,
  decodeAssertionItem,
} from "@/shared/contracts/assertions";
import { assertionTargetKey } from "@/shared/lib/assertionTargetKey";
import { SignedInContext } from "@/shared/ui/SignedInAccount";
import {
  assertionItemJson,
  assertionNote,
  assertionRecordRef,
} from "@/testdata/assertions/assertionsResponse";
import {
  accountLabel,
  signedInAlice,
  signedInViewer,
} from "@/testdata/session";
import { Assertions } from "./Assertions";
import type { AssertionsView, LoadedAssertions } from "./useAssertions";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const recordTarget: AssertionTarget = {
  kind: "record",
  record: assertionRecordRef,
};

const otherRecordTarget: AssertionTarget = {
  kind: "record",
  record: { ...assertionRecordRef, sequenceNumber: 999999 },
};

const authorLabel = "分析者の名前";
const noteLabel = "メモ";
const recordButtonName = "メモを記録";

const recordFailureSummary = "メモの記録";

/** メモの件数が 0 であることを「メモ: 0」の組で出しているか。 */
function showsNoAssertion(scope: HTMLElement = document.body): boolean {
  return Array.from(scope.querySelectorAll("li")).some(
    (item) => item.textContent === "メモ: 0",
  );
}

/** メモの表の、見出しを除く行。 */
function assertionRows(): HTMLElement[] {
  return within(screen.getByRole("table", { name: "メモ" }))
    .getAllByRole("row")
    .slice(1);
}

function recordedItem(): AssertionItem {
  return decodeAssertionItem(assertionItemJson(), "response");
}

test("AI 提案を採用して作ったメモにだけ、作成の列に AI 提案の採用を出す", () => {
  const adopted = recordedItem();
  adopted.assertion.proposalId = "ap:0001";
  const { rerender } = render(
    <SignedInContext.Provider value={signedInAlice}>
      <Assertions
        target={recordTarget}
        recordRefs={[assertionRecordRef]}
        view={{
          state: { status: "loaded", value: loadedAssertions([adopted]) },
          recording: false,
          record: () => {},
        }}
      />
    </SignedInContext.Provider>,
  );
  expect(screen.getByRole("cell", { name: "AI 提案の採用" })).toBeTruthy();

  rerender(
    <SignedInContext.Provider value={signedInAlice}>
      <Assertions
        target={recordTarget}
        recordRefs={[assertionRecordRef]}
        view={{
          state: {
            status: "loaded",
            value: loadedAssertions([recordedItem()]),
          },
          recording: false,
          record: () => {},
        }}
      />
    </SignedInContext.Provider>,
  );
  expect(screen.queryByRole("cell", { name: "AI 提案の採用" })).toBeNull();
  expect(screen.getByRole("cell", { name: "分析者" })).toBeTruthy();
});

/** 読み込んだ一覧と、その一覧から組んだ対象の索引を返す。 */
function loadedAssertions(items: AssertionItem[]): LoadedAssertions {
  const byTarget = new Map<string, AssertionItem[]>();
  for (const item of items) {
    const key = assertionTargetKey(item.assertion.target);
    if (key === undefined) {
      continue;
    }
    byTarget.set(key, [...(byTarget.get(key) ?? []), item]);
  }
  return { items, byTarget };
}

function renderAssertions(
  overrides: Partial<AssertionsView> = {},
  target: AssertionTarget = recordTarget,
) {
  const view: AssertionsView = {
    state: { status: "loaded", value: loadedAssertions([]) },
    recording: false,
    record: () => {},
    ...overrides,
  };
  render(
    <Assertions
      target={target}
      recordRefs={[assertionRecordRef]}
      view={view}
    />,
  );
}

test("メモが 0 件のときは、件数 0 を出し、表を出さない", () => {
  renderAssertions();

  expect(showsNoAssertion()).toBe(true);
  expect(screen.queryByRole("table", { name: "メモ" })).toBeNull();
});

test("対象に付いたメモを、記録した分析者と根拠の件数とともに 1 行に出す", () => {
  renderAssertions({
    state: {
      status: "loaded",
      value: loadedAssertions([recordedItem()]),
    },
  });

  const [row] = assertionRows();
  expect(row?.textContent).toContain(assertionNote);
  expect(row?.textContent).toContain("analyst-a");
});

test("入力欄は分析者の名前とメモだけを持ち、決まった書式の欄を出さない", () => {
  renderAssertions();

  expect(screen.getByLabelText(authorLabel)).toBeTruthy();
  expect(screen.getByLabelText(noteLabel)).toBeTruthy();
  // 反対側。除去した書式の欄を出さない。
  for (const label of [
    /技法の識別子/,
    /目録の名前/,
    /読んだ目録のバージョン/,
    /識別子の出典/,
  ]) {
    expect(screen.queryByLabelText(label)).toBeNull();
  }
});

test("別の対象に付いた所見を、この対象の欄に出さない", () => {
  renderAssertions(
    {
      state: {
        status: "loaded",
        value: loadedAssertions([recordedItem()]),
      },
    },
    otherRecordTarget,
  );

  expect(showsNoAssertion()).toBe(true);
});

test("入力した名前とメモを、対象と根拠のレコードとともに送る", () => {
  const recorded = vi.fn();
  renderAssertions({ record: recorded });

  fireEvent.change(screen.getByLabelText(authorLabel), {
    target: { value: "analyst-a" },
  });
  fireEvent.change(screen.getByLabelText(noteLabel), {
    target: { value: "port 5985 の acpt と wsmprovhost.exe の起動を読んだ" },
  });
  fireEvent.click(screen.getByRole("button", { name: recordButtonName }));

  expect(recorded).toHaveBeenCalledTimes(1);
  const draft = recorded.mock.calls[0]?.[0] as AssertionDraft;
  expect(draft.target).toEqual(recordTarget);
  expect(draft.author).toBe("analyst-a");
  expect(draft.note).toBe(
    "port 5985 の acpt と wsmprovhost.exe の起動を読んだ",
  );
  expect(draft.recordRefs).toEqual([assertionRecordRef]);
});

test("ログインしているときは名前の入力欄を出さず、ログインした利用者を出して著者を送らない", () => {
  const recorded = vi.fn();
  render(
    <SignedInContext.Provider value={signedInAlice}>
      <Assertions
        target={recordTarget}
        recordRefs={[assertionRecordRef]}
        view={{
          state: { status: "loaded", value: loadedAssertions([]) },
          recording: false,
          record: recorded,
        }}
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
  fireEvent.change(screen.getByLabelText(noteLabel), {
    target: { value: "メモ" },
  });
  fireEvent.click(screen.getByRole("button", { name: recordButtonName }));

  const draft = recorded.mock.calls[0]?.[0] as AssertionDraft;
  expect(draft.author).toBeUndefined();
});

test("閲覧者の役割では、メモの欄と記録のボタンを無効にし、理由を出す", () => {
  render(
    <SignedInContext.Provider value={signedInViewer}>
      <Assertions
        target={recordTarget}
        recordRefs={[assertionRecordRef]}
        view={{
          state: { status: "loaded", value: loadedAssertions([]) },
          recording: false,
          record: vi.fn(),
        }}
      />
    </SignedInContext.Provider>,
  );

  expect(screen.getByText("閲覧者は記録不可")).toBeTruthy();
  expect(screen.getByLabelText(noteLabel)).toBeDisabled();
  expect(screen.getByRole("button", { name: recordButtonName })).toBeDisabled();
});

test("記録が失敗しても、読めた所見を消さない", () => {
  renderAssertions({
    state: {
      status: "loaded",
      value: loadedAssertions([recordedItem()]),
    },
    recordFailure: {
      failure: buildFetchFailure("request_rejected", recordFailureSummary),
      target: recordTarget,
    },
  });

  expect(screen.getByText(recordFailureSummary, { exact: false })).toBeTruthy();
  expect(
    assertionRows().some((row) => row.textContent?.includes(assertionNote)),
  ).toBe(true);
});

test("記録の失敗を、記録しようとした対象の欄だけに出す", () => {
  renderAssertions(
    {
      recordFailure: {
        failure: buildFetchFailure("request_rejected", recordFailureSummary),
        target: otherRecordTarget,
      },
    },
    recordTarget,
  );

  // 反対側。別の対象の記録の失敗を、この対象の欄に出さない。
  expect(screen.queryByText(recordFailureSummary, { exact: false })).toBeNull();
});

test("記録の途中は、記録する操作を押せなくする", () => {
  renderAssertions({ recording: true });

  expect(
    screen.getByRole("button", { name: recordButtonName }),
  ).toHaveAttribute("aria-disabled", "true");
});

test("記録の途中は、button を押しても入力欄で form を送っても、もう一度記録しない", () => {
  const recorded = vi.fn();
  renderAssertions({ recording: true, record: recorded });
  fireEvent.change(screen.getByLabelText(noteLabel), {
    target: { value: "メモ" },
  });

  const button = screen.getByRole("button", { name: recordButtonName });
  fireEvent.click(button);
  const form = button.closest("form");
  if (form === null) throw new Error("記録の form が無い");
  fireEvent.submit(form);

  expect(recorded).not.toHaveBeenCalled();
});

/** 2 つの欄が 1 つの view を読む形を描く。App.tsx が対象ごとの欄に渡す形である。 */
function twoFields(view: AssertionsView) {
  return (
    <>
      <Assertions
        target={recordTarget}
        recordRefs={[assertionRecordRef]}
        view={view}
      />
      <Assertions
        target={otherRecordTarget}
        recordRefs={[assertionRecordRef]}
        view={view}
      />
    </>
  );
}

/** 位置で指した欄の要素を返す。1 つ目が recordTarget、2 つ目が otherRecordTarget である。 */
function fieldAt(container: HTMLElement, index: number): HTMLElement {
  const section = container.querySelectorAll("section")[index];
  if (section === undefined) {
    throw new Error(`the render carries no field at ${index}`);
  }
  return section;
}

/** 欄の入力の値を読む。 */
function inputValueOf(field: HTMLElement, label: string | RegExp): string {
  const input = within(field).getByLabelText(label);
  return (input as HTMLInputElement | HTMLTextAreaElement).value;
}

function baseView(): AssertionsView {
  return {
    state: { status: "loaded", value: loadedAssertions([]) },
    recording: false,
    record: () => {},
  };
}

/**
 * 片方の欄の記録の成功で、もう片方の欄の未保存の入力を消さない。
 *
 * 一覧の件数を入力欄を戻す引き金にすると、1 つの一覧を読む 2 つの欄のうち、記録して
 * いない側の入力が消える。
 */
test("片方の欄の記録が成功しても、もう片方の欄の未保存の入力を残す", () => {
  const view = baseView();
  const { container, rerender } = render(twoFields(view));

  // 2 つ目の欄に、まだ送っていない入力を書く。
  const writing = fieldAt(container, 1);
  fireEvent.change(within(writing).getByLabelText(noteLabel), {
    target: { value: "書きかけのメモ" },
  });

  // 1 つ目の欄の対象への記録が成功し、一覧が 1 件増える。
  rerender(
    twoFields({
      ...view,
      state: {
        status: "loaded",
        value: loadedAssertions([recordedItem()]),
      },
      recordSuccess: { target: recordTarget, count: 1 },
    }),
  );

  expect(inputValueOf(fieldAt(container, 1), noteLabel)).toBe("書きかけのメモ");
});

/** 記録に成功した欄は、メモの入力を空へ戻す。 */
test("記録に成功した欄は、メモの入力を空へ戻し、分析者の名前を残す", () => {
  const view = baseView();
  const { container, rerender } = render(twoFields(view));

  const recording = fieldAt(container, 0);
  fireEvent.change(within(recording).getByLabelText(authorLabel), {
    target: { value: "analyst-a" },
  });
  fireEvent.change(within(recording).getByLabelText(noteLabel), {
    target: { value: "port 5985 の acpt を読んだ" },
  });

  rerender(
    twoFields({
      ...view,
      state: {
        status: "loaded",
        value: loadedAssertions([recordedItem()]),
      },
      recordSuccess: { target: recordTarget, count: 1 },
    }),
  );

  const returned = fieldAt(container, 0);
  expect(inputValueOf(returned, noteLabel)).toBe("");
  // 反対側。同じ分析者が続けて記録するため、名前は残す。
  expect(inputValueOf(returned, authorLabel)).toBe("analyst-a");
});
