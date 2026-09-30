// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { SubgraphCanvasBoundary } from "./SubgraphCanvasBoundary";

beforeEach(() => {
  // React は受け止めた例外を console へ書く。test の出力を読めるように console への出力を止める。
  vi.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function Throwing({ reason }: { reason: string }): never {
  throw new Error(reason);
}

test("図の組み立てが投げた例外を受け止め、一覧へ案内する", () => {
  render(
    <SubgraphCanvasBoundary>
      <Throwing reason="a node with the same key already exists" />
    </SubgraphCanvasBoundary>,
  );

  const alert = screen.getByRole("alert");
  expect(alert.textContent).toContain("描画に失敗");
  expect(alert.textContent).toContain("ノードの選択: Nodes のビュー");
  // 例外の文字列を画面に出さない。
  expect(alert.textContent).not.toContain("already exists");
});

test("例外の文字列の制御文字を可視の符号にして記録する", () => {
  render(
    <SubgraphCanvasBoundary>
      <Throwing reason={"node ‮ id"} />
    </SubgraphCanvasBoundary>,
  );

  expect(console.error).toHaveBeenCalledWith(
    "rendering the subgraph failed",
    "node U+202E id",
  );
});

test("例外を投げない子をそのまま描く", () => {
  render(
    <SubgraphCanvasBoundary>
      <p>図の中身</p>
    </SubgraphCanvasBoundary>,
  );

  expect(screen.getByText("図の中身")).toBeTruthy();
  expect(screen.queryByRole("alert")).toBe(null);
});
