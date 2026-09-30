// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import type { AssistProposalAdoption } from "@/shared/contracts/assistProposals";
import {
  adoptedItemJson,
  adoptionJson,
  assistProposalsResponseJson,
  rejectedItemJson,
} from "@/testdata/assistProposals/assistProposalsResponse";
import { jsonResponse } from "@/testdata/http";
import {
  type AssistProposalAdoptionHandlers,
  useAssistProposals,
} from "./useAssistProposals";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const matchConditions: MatchConditionSelection = {
  conditions: [{ conditionKey: "destination_ip" }],
};
const listPath = "/api/v0/assist-proposals?matchCondition=destination_ip";
const adoptionPath =
  "/api/v0/assist-proposals/ap%3A0001/adoption?matchCondition=destination_ip";

/** hook の結果を読める形で描き、操作のボタンを置く。 */
function Probe({ handlers }: { handlers: AssistProposalAdoptionHandlers }) {
  const view = useAssistProposals(matchConditions, handlers);
  const state =
    view.state.status === "loaded"
      ? `states=${view.state.value.items.map((item) => item.proposal.state).join(",")}`
      : view.state.status === "failed"
        ? `failed=${view.state.failure.failureCode ?? "none"}`
        : `status=${view.state.status}`;
  return (
    <>
      <p>{state}</p>
      <p>draft={view.drafts.get("ap:0001")?.note ?? "none"}</p>
      <p>failure={view.failures.get("ap:0001")?.failureCode ?? "none"}</p>
      <button
        type="button"
        onClick={() =>
          view.changeDraft("ap:0001", {
            note: "直した記述",
            reason: "",
            analyst: "",
          })
        }
      >
        下書き
      </button>
      <button type="button" onClick={view.reload}>
        取り直す
      </button>
      <button
        type="button"
        onClick={() => view.adopt("ap:0001", { note: "直した記述" })}
      >
        採用
      </button>
      <button
        type="button"
        onClick={() => {
          view.adopt("ap:0001", {});
          view.adopt("ap:0001", {});
        }}
      >
        2 回採用
      </button>
      <button
        type="button"
        onClick={() => view.reject("ap:0001", { reason: "理由" })}
      >
        却下
      </button>
    </>
  );
}

function handlersOf() {
  return { onAdopted: vi.fn(), onAdoptedElsewhere: vi.fn() };
}

/** 呼ばれた順に responses の応答を返す fetch の mock を置く。 */
function stubResponses(...responses: Response[]) {
  const mock = vi.fn(async (_input: string, _init?: RequestInit) => {
    const next = responses.shift();
    if (next === undefined) {
      throw new Error("no more responses");
    }
    return next;
  });
  vi.stubGlobal("fetch", mock);
  return mock;
}

function decidedConflict(conflict: unknown): Response {
  return jsonResponse(409, {
    code: "assist_proposal_decided",
    message: "the proposal is already decided",
    conflict,
  });
}

async function renderLoaded(handlers = handlersOf()) {
  render(<Probe handlers={handlers} />);
  await waitFor(() => expect(screen.getByText("states=proposed")).toBeTruthy());
  return handlers;
}

test("採用は一覧の提案を置き換え、下書きを消し、応答を受け取る側へ渡す", async () => {
  const mock = stubResponses(
    jsonResponse(200, assistProposalsResponseJson()),
    jsonResponse(201, adoptionJson("直した記述")),
  );
  const handlers = await renderLoaded();
  fireEvent.click(screen.getByRole("button", { name: "下書き" }));
  expect(screen.getByText("draft=直した記述")).toBeTruthy();

  fireEvent.click(screen.getByRole("button", { name: "採用" }));

  await waitFor(() => expect(screen.getByText("states=adopted")).toBeTruthy());
  expect(screen.getByText("draft=none")).toBeTruthy();
  expect(mock.mock.calls[0]?.[0]).toBe(listPath);
  expect(mock.mock.calls[1]?.[0]).toBe(adoptionPath);
  expect(JSON.parse(String(mock.mock.calls[1]?.[1]?.body))).toEqual({
    note: "直した記述",
  });
  expect(handlers.onAdopted).toHaveBeenCalledTimes(1);
  const adoption = handlers.onAdopted.mock
    .calls[0]?.[0] as AssistProposalAdoption;
  expect(adoption.assertion.assertion.basis.note).toBe("直した記述");
  expect(handlers.onAdoptedElsewhere).not.toHaveBeenCalled();
});

test("別の操作が先に採用していた提案は、競合の応答の提案に置き換え、別の操作の採用として知らせる", async () => {
  stubResponses(
    jsonResponse(200, assistProposalsResponseJson()),
    decidedConflict(adoptedItemJson()),
  );
  const handlers = await renderLoaded();

  fireEvent.click(screen.getByRole("button", { name: "採用" }));

  await waitFor(() => expect(screen.getByText("states=adopted")).toBeTruthy());
  expect(screen.getByText("failure=assist_proposal_decided")).toBeTruthy();
  expect(handlers.onAdopted).not.toHaveBeenCalled();
  expect(handlers.onAdoptedElsewhere).toHaveBeenCalledTimes(1);
  expect(handlers.onAdoptedElsewhere.mock.calls[0]?.[0]).toMatchObject({
    proposal: { id: "ap:0001", state: "adopted" },
  });
});

test("別の操作が先に却下していた提案は、置き換えるだけで所見ができたと知らせない", async () => {
  stubResponses(
    jsonResponse(200, assistProposalsResponseJson()),
    decidedConflict(rejectedItemJson()),
  );
  const handlers = await renderLoaded();

  fireEvent.click(screen.getByRole("button", { name: "採用" }));

  await waitFor(() => expect(screen.getByText("states=rejected")).toBeTruthy());
  expect(handlers.onAdoptedElsewhere).not.toHaveBeenCalled();
});

test("読めない競合の記録と、競合以外の失敗は、一覧と下書きを残して失敗だけを出す", async () => {
  stubResponses(
    jsonResponse(200, assistProposalsResponseJson()),
    decidedConflict({ proposal: "broken" }),
    jsonResponse(500, { code: "internal_error", message: "storing failed" }),
  );
  const handlers = await renderLoaded();
  fireEvent.click(screen.getByRole("button", { name: "下書き" }));

  fireEvent.click(screen.getByRole("button", { name: "採用" }));
  await waitFor(() =>
    expect(screen.getByText("failure=assist_proposal_decided")).toBeTruthy(),
  );
  expect(screen.getByText("states=proposed")).toBeTruthy();

  fireEvent.click(screen.getByRole("button", { name: "採用" }));
  await waitFor(() =>
    expect(screen.getByText("failure=internal_error")).toBeTruthy(),
  );
  expect(screen.getByText("states=proposed")).toBeTruthy();
  expect(screen.getByText("draft=直した記述")).toBeTruthy();
  expect(handlers.onAdopted).not.toHaveBeenCalled();
  expect(handlers.onAdoptedElsewhere).not.toHaveBeenCalled();
});

test("却下は一覧の提案を置き換え、所見ができたと知らせない", async () => {
  const mock = stubResponses(
    jsonResponse(200, assistProposalsResponseJson()),
    jsonResponse(200, rejectedItemJson()),
  );
  const handlers = await renderLoaded();

  fireEvent.click(screen.getByRole("button", { name: "却下" }));

  await waitFor(() => expect(screen.getByText("states=rejected")).toBeTruthy());
  expect(JSON.parse(String(mock.mock.calls[1]?.[1]?.body))).toEqual({
    reason: "理由",
  });
  expect(handlers.onAdopted).not.toHaveBeenCalled();
  expect(handlers.onAdoptedElsewhere).not.toHaveBeenCalled();
});

test("送信中の提案への 2 回目の操作は送らない", async () => {
  const mock = stubResponses(
    jsonResponse(200, assistProposalsResponseJson()),
    jsonResponse(201, adoptionJson()),
  );
  await renderLoaded();

  fireEvent.click(screen.getByRole("button", { name: "2 回採用" }));

  await waitFor(() => expect(screen.getByText("states=adopted")).toBeTruthy());
  expect(mock).toHaveBeenCalledTimes(2);
});

test("一覧を取り直しても下書きを消さず、採否が決まった提案の前の失敗を消す", async () => {
  const mock = stubResponses(
    jsonResponse(200, assistProposalsResponseJson()),
    jsonResponse(500, { code: "internal_error", message: "storing failed" }),
    jsonResponse(200, assistProposalsResponseJson([adoptedItemJson()])),
  );
  await renderLoaded();
  fireEvent.click(screen.getByRole("button", { name: "下書き" }));
  fireEvent.click(screen.getByRole("button", { name: "採用" }));
  await waitFor(() =>
    expect(screen.getByText("failure=internal_error")).toBeTruthy(),
  );

  fireEvent.click(screen.getByRole("button", { name: "取り直す" }));

  await waitFor(() => expect(screen.getByText("states=adopted")).toBeTruthy());
  expect(mock.mock.calls[2]?.[0]).toBe(listPath);
  expect(screen.getByText("failure=none")).toBeTruthy();
  expect(screen.getByText("draft=直した記述")).toBeTruthy();
});

test("この起動で使えない一覧は assist_unavailable の失敗として持つ", async () => {
  stubResponses(
    jsonResponse(409, {
      code: "assist_unavailable",
      message: "the launch keeps no investigation",
    }),
  );
  render(<Probe handlers={handlersOf()} />);

  await waitFor(() =>
    expect(screen.getByText("failed=assist_unavailable")).toBeTruthy(),
  );
});
