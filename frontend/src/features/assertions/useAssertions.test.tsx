// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, test, vi } from "vitest";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import {
  type AssertionTarget,
  decodeAssertionItem,
} from "@/shared/contracts/assertions";
import { assertionTargetKey } from "@/shared/lib/assertionTargetKey";
import {
  assertionItemJson,
  assertionNote,
  assertionRecordRef,
  assertionsResponseJson,
  emptyAssertionsResponseJson,
} from "@/testdata/assertions/assertionsResponse";
import { jsonResponse } from "@/testdata/http";
import { useAssertions } from "./useAssertions";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// 本 test が渡す関連付けの条件の選択。
const matchConditions: MatchConditionSelection = {
  conditions: [{ conditionKey: "destination_ip" }],
};
const matchConditionQuery = "matchCondition=destination_ip";

/** 一覧の所見が指す対象。fixture の所見と同じレコードを指す。 */
const fixtureTarget: AssertionTarget = {
  kind: "record",
  record: assertionRecordRef,
};

/** 一覧のどの所見も指していない対象。 */
const otherTarget: AssertionTarget = {
  kind: "record",
  record: {
    sourceContentSha256: "b".repeat(64),
    positionKind: "sequence_number",
    sequenceNumber: 404,
  },
};

/** hook の結果を読める形で描く。対象で探した件数も出す。 */
function Probe({ target }: { target: AssertionTarget }) {
  const view = useAssertions(matchConditions);
  if (view.state.status !== "loaded") {
    return <p>status={view.state.status}</p>;
  }
  const key = assertionTargetKey(target);
  const matched =
    key === undefined ? [] : (view.state.value.byTarget.get(key) ?? []);
  return (
    <p>
      items={view.state.value.items.length} matched={matched.length} note=
      {matched[0]?.assertion.basis.note ?? "none"}
    </p>
  );
}

function stubJson(body: unknown) {
  const mock = vi.fn(async (_input: string, _init?: RequestInit) =>
    jsonResponse(200, body),
  );
  vi.stubGlobal("fetch", mock);
  return mock;
}

test("1 回の要求で所見の全件を受け取る", async () => {
  const mock = stubJson(assertionsResponseJson());

  render(<Probe target={fixtureTarget} />);

  await waitFor(() => expect(screen.getByText(/items=1/)).toBeTruthy());
  expect(mock).toHaveBeenCalledTimes(1);
  expect(mock.mock.calls[0]?.[0]).toBe(
    `/api/v0/assertions?${matchConditionQuery}`,
  );
});

test("対象を指す文字列から、その対象に付いた所見を探す", async () => {
  stubJson(assertionsResponseJson());

  render(<Probe target={fixtureTarget} />);

  await waitFor(() =>
    expect(screen.getByText(new RegExp(`note=${assertionNote}`))).toBeTruthy(),
  );
  expect(screen.getByText(/matched=1/)).toBeTruthy();
});

test("一覧のどの所見も指していない対象は 0 件を返す", async () => {
  stubJson(assertionsResponseJson());

  render(<Probe target={otherTarget} />);

  await waitFor(() => expect(screen.getByText(/items=1/)).toBeTruthy());
  expect(screen.getByText(/matched=0 note=none/)).toBeTruthy();
});

test("所見を 1 件も持たない一覧を、失敗にせず 0 件で受け取る", async () => {
  const mock = stubJson(emptyAssertionsResponseJson());

  render(<Probe target={fixtureTarget} />);

  await waitFor(() =>
    expect(screen.getByText(/items=0 matched=0/)).toBeTruthy(),
  );
  expect(mock).toHaveBeenCalledTimes(1);
});

/** 別の操作が作った所見を足す操作と、一覧の件数を描く。 */
function AddProbe() {
  const view = useAssertions(matchConditions);
  // 採用を送った時点の描画の関数を保つ。応答はこの関数で所見を足す。
  const [savedAdd, setSavedAdd] = useState<typeof view.add | undefined>(
    undefined,
  );
  return (
    <>
      <button
        type="button"
        onClick={() => view.add(decodeAssertionItem(assertionItemJson(), "$"))}
      >
        足す
      </button>
      <button type="button" onClick={() => setSavedAdd(() => view.add)}>
        送る
      </button>
      <button type="button" onClick={view.reload}>
        取り直す
      </button>
      <button
        type="button"
        onClick={() =>
          savedAdd?.(decodeAssertionItem(assertionItemJson(), "$"))
        }
      >
        送った時点の関数で足す
      </button>
      <p>
        {view.state.status === "loaded"
          ? `items=${view.state.value.items.length}`
          : `status=${view.state.status}`}
      </p>
    </>
  );
}

test("一覧に同じ識別子の所見があるときは、足さずに置き換える", async () => {
  stubJson(assertionsResponseJson());
  render(<AddProbe />);
  await waitFor(() => expect(screen.getByText("items=1")).toBeTruthy());

  fireEvent.click(screen.getByRole("button", { name: "足す" }));

  expect(screen.getByText("items=1")).toBeTruthy();
});

/** 所見を足す操作と、一覧の最初の所見の対象の出所を描く。 */
function ItemProbe() {
  const view = useAssertions(matchConditions);
  return (
    <>
      <button
        type="button"
        onClick={() => view.add(decodeAssertionItem(assertionItemJson(), "$"))}
      >
        足す
      </button>
      <p>
        origin=
        {view.state.status === "loaded"
          ? (view.state.value.items[0]?.targetOrigin ?? "none")
          : view.state.status}
      </p>
    </>
  );
}

/** 呼ばれた順に、応答を後から返せる fetch の mock を置く。 */
function stubPending() {
  const pending: ((response: Response) => void)[] = [];
  const mock = vi.fn(
    (_input: string, _init?: RequestInit) =>
      new Promise<Response>((resolve) => pending.push(resolve)),
  );
  vi.stubGlobal("fetch", mock);
  return { mock, pending };
}

test("最初の取得の途中に足した所見は、その取得が所見を作る前の一覧を返しても出す", async () => {
  const { mock, pending } = stubPending();
  render(<AddProbe />);
  expect(screen.getByText("status=loading")).toBeTruthy();

  fireEvent.click(screen.getByRole("button", { name: "足す" }));
  // 足す前に送った取得は、所見を作る前の一覧を返す。
  pending[0]?.(jsonResponse(200, emptyAssertionsResponseJson()));

  await waitFor(() => expect(screen.getByText("items=1")).toBeTruthy());
  expect(mock).toHaveBeenCalledTimes(1);
});

test("取り直しの途中に、送った時点の関数で足した所見も、取り直した一覧に出す", async () => {
  const { mock, pending } = stubPending();
  render(<AddProbe />);
  pending[0]?.(jsonResponse(200, emptyAssertionsResponseJson()));
  await waitFor(() => expect(screen.getByText("items=0")).toBeTruthy());
  fireEvent.click(screen.getByRole("button", { name: "送る" }));

  fireEvent.click(screen.getByRole("button", { name: "取り直す" }));
  await waitFor(() => expect(mock).toHaveBeenCalledTimes(2));
  fireEvent.click(
    screen.getByRole("button", { name: "送った時点の関数で足す" }),
  );
  // 取り直しの要求は、所見を作る前に届いた。
  pending[1]?.(jsonResponse(200, emptyAssertionsResponseJson()));

  await waitFor(() => expect(mock).toHaveBeenCalledTimes(2));
  await waitFor(() => expect(screen.getByText("items=1")).toBeTruthy());
});

test("一覧の取得が失敗している間に足した所見は、一覧を取り直して出し、読めた一覧では取り直さない", async () => {
  const { mock, pending } = stubPending();
  render(<AddProbe />);
  pending[0]?.(
    jsonResponse(500, { code: "internal_error", message: "reading failed" }),
  );
  await waitFor(() => expect(screen.getByText("status=failed")).toBeTruthy());

  fireEvent.click(screen.getByRole("button", { name: "足す" }));

  await waitFor(() => expect(mock).toHaveBeenCalledTimes(2));
  pending[1]?.(jsonResponse(200, assertionsResponseJson()));
  await waitFor(() => expect(screen.getByText("items=1")).toBeTruthy());
  fireEvent.click(screen.getByRole("button", { name: "足す" }));
  expect(mock).toHaveBeenCalledTimes(2);
});

test("取得の途中に足した所見を応答が持つときは、応答の値を出す", async () => {
  const { pending } = stubPending();
  render(<ItemProbe />);
  fireEvent.click(screen.getByRole("button", { name: "足す" }));
  const fromServer = assertionsResponseJson();
  fromServer.assertions[0] = {
    ...assertionItemJson(),
    targetOrigin: "absent",
  };

  pending[0]?.(jsonResponse(200, fromServer));

  await waitFor(() => expect(screen.getByText("origin=absent")).toBeTruthy());
});

test("足した後に始めた取得の一覧は、足した所見で上書きしない", async () => {
  const { pending } = stubPending();
  render(<AddProbe />);
  pending[0]?.(jsonResponse(200, emptyAssertionsResponseJson()));
  await waitFor(() => expect(screen.getByText("items=0")).toBeTruthy());
  fireEvent.click(screen.getByRole("button", { name: "足す" }));
  expect(screen.getByText("items=1")).toBeTruthy();

  // 足した後に始めた取得は、server が確定した一覧を返す。足した所見を持たない一覧はそのまま出す。
  fireEvent.click(screen.getByRole("button", { name: "取り直す" }));
  pending[1]?.(jsonResponse(200, emptyAssertionsResponseJson()));

  await waitFor(() => expect(screen.getByText("items=0")).toBeTruthy());
});

/** 記録した対象と、記録の結果を読める形で描く。 */
function RecordProbe({ target }: { target: AssertionTarget }) {
  const view = useAssertions(matchConditions);
  const success = view.recordSuccess;
  return (
    <>
      <button
        type="button"
        onClick={() =>
          view.record({
            target,
            author: "analyst-a",
            note: "メモ",
            recordRefs: [],
          })
        }
      >
        記録する
      </button>
      <p>
        success=
        {success === undefined
          ? "none"
          : `${String(success.target.record?.sequenceNumber)}:${success.count}`}
      </p>
    </>
  );
}

/**
 * 記録の成功が、記録した対象と通算の回数を返す。
 *
 * **成功したことだけを返さない。** 1 つの一覧を 2 つの欄が読むため、対象を持たない
 * 成功は、記録していない欄の未保存の入力を消す引き金になる。
 */
test("記録の成功が、記録した対象と通算の回数を返す", async () => {
  const target: AssertionTarget = {
    kind: "record",
    record: {
      sourceContentSha256: "a".repeat(64),
      positionKind: "sequence_number",
      sequenceNumber: 203,
    },
  };
  const mock = vi.fn(async (_input: string, init?: RequestInit) => {
    if (init?.method === "POST") {
      return jsonResponse(201, assertionItemJson());
    }
    return jsonResponse(200, emptyAssertionsResponseJson());
  });
  vi.stubGlobal("fetch", mock);

  render(<RecordProbe target={target} />);
  await waitFor(() => expect(screen.getByText(/success=none/)).toBeTruthy());

  fireEvent.click(screen.getByRole("button", { name: "記録する" }));
  await waitFor(() => expect(screen.getByText(/success=203:1/)).toBeTruthy());

  // 2 回目の成功で回数が増える。同じ対象へ続けて記録した 2 回を回数が分ける。
  fireEvent.click(screen.getByRole("button", { name: "記録する" }));
  await waitFor(() => expect(screen.getByText(/success=203:2/)).toBeTruthy());
});
