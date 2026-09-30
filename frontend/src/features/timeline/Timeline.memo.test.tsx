// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import { jsonResponse } from "@/testdata/http";
import { timelineResponseJson } from "@/testdata/timeline/timelineResponse";
import { Timeline } from "./Timeline";
import { useTimeline } from "./useTimeline";

// Timeline が描かれた回数を、取得の hook が呼ばれた回数で数える。hook の動作は変えない。
vi.mock("./useTimeline", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./useTimeline")>();
  return { useTimeline: vi.fn(actual.useTimeline) };
});

const matchConditions: MatchConditionSelection = {
  conditions: [{ conditionKey: "destination_ip" }],
};
const onSelectRecord = () => {};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.mocked(useTimeline).mockClear();
});

function TimelineHost({
  eventCategory,
}: {
  /** 上位の画面が持つ状態の 1 つ。時系列の props に渡さない値を変えるのに使う。 */
  unrelated: number;
  eventCategory: string | undefined;
}) {
  return (
    <Timeline
      matchConditions={matchConditions}
      timeFilter={undefined}
      eventCategory={eventCategory}
      eventAction={undefined}
      caseId={undefined}
      terminal={undefined}
      onSelectRecord={onSelectRecord}
      dataVersion={0}
    />
  );
}

test("上位の画面が時系列の props を変えずに描き直すとき、時系列を描き直さない", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse(200, timelineResponseJson())),
  );
  const hook = vi.mocked(useTimeline);
  const view = render(<TimelineHost unrelated={0} eventCategory={undefined} />);
  await screen.findByRole("table", { name: "時刻順のレコード" });
  const settled = hook.mock.calls.length;
  expect(hook.mock.results.at(-1)?.value).toMatchObject({ status: "loaded" });

  view.rerender(<TimelineHost unrelated={1} eventCategory={undefined} />);
  expect(hook.mock.calls.length).toBe(settled);

  view.rerender(<TimelineHost unrelated={1} eventCategory="net" />);
  await waitFor(() => expect(hook.mock.calls.length).toBeGreaterThan(settled));
  expect(hook.mock.calls.at(-1)?.[0]).toMatchObject({ eventCategory: "net" });
});
