// @vitest-environment jsdom
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { TerminalAssignmentDraft } from "@/shared/api/terminalAssignments";
import { decodeTerminalAssignment } from "@/shared/contracts/terminalAssignments";
import { jsonResponse } from "@/testdata/http";
import {
  analystAssignmentJson,
  terminalAssignmentsResponseJson,
} from "@/testdata/terminalAssignments/terminalAssignmentsResponse";
import { useTerminalAssignments } from "./useTerminalAssignments";

const listPath = "/api/v0/terminal-assignments";

/** 一覧の取得には一覧を返し、記録には reply が返す応答を返す fetch の mock を置く。 */
function stubFetch(reply: () => Promise<Response>) {
  const mock = vi.fn(async (_input: string, init?: RequestInit) =>
    init?.method === "POST"
      ? reply()
      : jsonResponse(200, terminalAssignmentsResponseJson()),
  );
  vi.stubGlobal("fetch", mock);
  return mock;
}

function draftOf(): TerminalAssignmentDraft {
  const assignment = decodeTerminalAssignment(analystAssignmentJson(), "$");
  return {
    clientIp: assignment.clientIp,
    terminalId: "host-a",
    sourceId: assignment.sourceId,
    sourceContentSha256: assignment.sourceContentSha256,
    assignmentValidRange: assignment.assignmentValidRange,
    derivation: assignment.derivation ?? "",
    basisRecordRefs: assignment.basisRecordRefs ?? [],
    author: assignment.author ?? "",
  };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

test("記録している間だけ recording が真になり、409 で拒まれたら理由を残して偽へ戻す", async () => {
  let respond: (response: Response) => void = () => {};
  stubFetch(
    () =>
      new Promise<Response>((resolve) => {
        respond = resolve;
      }),
  );
  const { result } = renderHook(() => useTerminalAssignments());
  await waitFor(() => expect(result.current.state.status).toBe("loaded"));

  let recorded: Promise<void> = Promise.resolve();
  act(() => {
    recorded = result.current.record(draftOf());
  });
  expect(result.current.recording).toBe(true);

  await act(async () => {
    respond(
      jsonResponse(409, {
        code: "terminal_assignment_already_recorded",
        message: "the same terminal assignment is already recorded",
      }),
    );
    await recorded;
  });
  expect(result.current.recording).toBe(false);
  expect(result.current.recordFailure?.failureCode).toBe(
    "terminal_assignment_already_recorded",
  );
  expect(result.current.recordedCount).toBe(0);
});

test("記録できたら一覧を読み直してから recording を偽へ戻す", async () => {
  const mock = stubFetch(async () =>
    jsonResponse(201, analystAssignmentJson()),
  );
  const { result } = renderHook(() => useTerminalAssignments());
  await waitFor(() => expect(result.current.state.status).toBe("loaded"));

  await act(async () => {
    await result.current.record(draftOf());
  });
  const listCalls = mock.mock.calls.filter(
    ([input, init]) => input.endsWith(listPath) && init?.method !== "POST",
  );
  expect(listCalls).toHaveLength(2);
  expect(result.current.recording).toBe(false);
  expect(result.current.recordedCount).toBe(1);
  expect(result.current.recordFailure).toBeUndefined();
});
