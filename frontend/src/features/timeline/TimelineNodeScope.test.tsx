// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import {
  defaultTimelineNodeScope,
  TimelineNodeScope,
} from "./TimelineNodeScope";

afterEach(cleanup);

test("ノードを選んでいないときは未選択を出し、フィルタの対象は「?」の説明で読める", () => {
  render(
    <TimelineNodeScope
      node={undefined}
      value={defaultTimelineNodeScope}
      onChange={vi.fn()}
    />,
  );
  expect(screen.getByRole("listitem").textContent).toBe("ノード: 未選択");
  fireEvent.click(
    screen.getByRole("button", { name: "Graph の選択ノードでフィルタ の説明" }),
  );
  expect(screen.getByRole("tooltip").textContent).toContain(
    "選択ノードからホップ数までのノードとエッジのレコード",
  );
  // 時系列に適用する検索の条件も、同じ「?」の説明に出す。
  expect(screen.getByRole("tooltip").textContent).toContain(
    "期間・イベントの種類・案件・端末・収集元・検索式",
  );
});

test("ノードを選ぶと、そのノードの表示名を組で出す", () => {
  render(
    <TimelineNodeScope
      node={{ id: "n:1", label: "HOST-A" }}
      value={defaultTimelineNodeScope}
      onChange={vi.fn()}
    />,
  );
  expect(screen.getByRole("listitem").textContent).toBe("ノード: HOST-A");
});

test("フィルタを切ると、ノードの組を出さない", () => {
  render(
    <TimelineNodeScope
      node={undefined}
      value={{ ...defaultTimelineNodeScope, enabled: false }}
      onChange={vi.fn()}
    />,
  );
  expect(screen.queryByRole("listitem")).toBeNull();
  expect(
    screen.getByRole<HTMLInputElement>("checkbox", {
      name: "Graph の選択ノードでフィルタ",
    }).checked,
  ).toBe(false);
});
