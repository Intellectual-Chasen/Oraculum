// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import type { AssistMatchCondition } from "@/shared/contracts/assistRelay";
import { SearchQueryCard } from "./SearchQueryCard";

afterEach(cleanup);

function renderCard(
  matchConditions: readonly AssistMatchCondition[],
  currentMatchConditions: readonly AssistMatchCondition[],
) {
  render(
    <SearchQueryCard
      query={{ depth: 1 }}
      matchConditions={matchConditions}
      currentMatchConditions={currentMatchConditions}
      terminals={{ status: "loaded", value: [] }}
      restorable={false}
      onApply={() => {}}
      onRestore={() => {}}
    />,
  );
}

/** 推定条件の不一致の詳細の「名前: 値」の組の文字列。 */
function mismatchPairs(): string[] {
  const label = screen.getByRole("img", { name: "推定条件の不一致" });
  const details = document.getElementById(
    label.getAttribute("aria-describedby") ?? "",
  );
  return [...(details?.querySelectorAll("li") ?? [])].map(
    (item) => item.textContent ?? "",
  );
}

test("許容幅だけが違う推定条件の不一致は、条件の名前と許容幅で出す", () => {
  renderCard(
    [{ conditionKey: "destination_ip", tolerance: 0 }],
    [{ conditionKey: "destination_ip", tolerance: 3 }],
  );

  expect(mismatchPairs()).toEqual([
    "card: 接続先 IP 許容幅: 0 秒",
    "今: 接続先 IP 許容幅: 3 秒",
  ]);
});

test("推定条件を選んでいない側は、なしと出す", () => {
  renderCard([{ conditionKey: "destination_ip", tolerance: 0 }], []);

  expect(mismatchPairs()).toEqual(["card: 接続先 IP 許容幅: 0 秒", "今: なし"]);
});
