// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import type { FetchState } from "../lib/fetchState";
import { FetchStateView } from "./FetchStateView";

afterEach(cleanup);

function renderState(state: FetchState<string>) {
  render(
    <FetchStateView state={state} loadingDescription="レコードの読み込み中">
      {(value) => <p>{value}</p>}
    </FetchStateView>,
  );
}

test("読み込み中に、処理中の状態の icon とラベルを出す", () => {
  renderState({ status: "loading" });

  const status = screen.getByRole("status");
  expect(status.textContent).toBe("レコードの読み込み中");
  expect(status.querySelector("svg.animate-spin")).not.toBeNull();
  expect(screen.queryByRole("alert")).toBeNull();
});

test("取得失敗に、失敗した操作と失敗の種類のラベルを出し、次の操作とコードを詳細の組にする", () => {
  renderState({
    status: "failed",
    failure: {
      kind: "network",
      summary: "収集元の一覧の取得",
      nextAction: "通信を確認して再実行",
      failureCode: "internal_error",
    },
  });

  const alert = screen.getByRole("alert");
  expect(screen.getByText("収集元の一覧の取得")).toBeTruthy();
  const label = screen.getByText("通信の失敗");
  // 詳細は tooltip に出し、tooltip を開く前から読み上げに結び付ける。
  const details = document.getElementById(
    label.getAttribute("aria-describedby") ?? "",
  );
  expect(details?.textContent).toBe(
    "次の操作: 通信を確認して再実行コード: internal_error",
  );
  expect(alert.contains(details)).toBe(true);
});

test("読み込みを退けた理由を持つ失敗は、理由をラベルにし、種類を詳細に入れる", () => {
  renderState({
    status: "failed",
    failure: {
      kind: "request_rejected",
      summary: "収集元の読み込みの開始",
      nextAction: "指定を直して再実行",
      failureCode: "invalid_request",
      failureDescription: "指定の誤り",
      rejectionDescription: "file なし",
    },
  });

  const label = screen.getByText("file なし");
  expect(
    document.getElementById(label.getAttribute("aria-describedby") ?? "")
      ?.textContent,
  ).toContain("種類: 指定の誤り");
});

test("失敗に関わる収集元の path を、持つ失敗だけに出す", () => {
  const failure = {
    kind: "request_rejected",
    summary: "収集元の読み込みの開始",
    nextAction: "指定を直して再実行",
    failureCode: "invalid_request",
  } as const;
  renderState({
    status: "failed",
    failure: { ...failure, originPath: "../outside/access.log" },
  });
  expect(screen.getByRole("alert").textContent).toContain(
    "path: ../outside/access.log",
  );
  cleanup();

  renderState({ status: "failed", failure });
  expect(screen.getByRole("alert").textContent).not.toContain("path:");
});

// 失敗の表示はレコード位置を出さない。backend が recordRef を載せた失敗を返さないため
// である (FetchFailureNotice)。
test("失敗の表示にレコードの欄を出さない", () => {
  renderState({
    status: "failed",
    failure: {
      kind: "request_rejected",
      summary: "レコードの取得",
      nextAction: "指定を直して再実行",
      failureCode: "record_not_found",
    },
  });

  const alert = screen.getByRole("alert");
  expect(alert.textContent).toContain("コード: record_not_found");
  expect(alert.textContent).not.toContain("位置:");
});

test("結果なしに理由を出し、失敗として出さない", () => {
  renderState({
    status: "empty",
    description: "収集元を 1 件も取り込んでいません。",
  });

  expect(screen.getByRole("status").textContent).toBe(
    "収集元を 1 件も取り込んでいません。",
  );
  expect(screen.queryByRole("alert")).toBeNull();
});

test("成功に値の表示を出す", () => {
  renderState({ status: "loaded", value: "access.log" });

  expect(screen.getByText("access.log")).toBeTruthy();
  expect(screen.queryByRole("status")).toBeNull();
  expect(screen.queryByRole("alert")).toBeNull();
});
