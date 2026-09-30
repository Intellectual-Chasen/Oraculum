import { afterEach, expect, test, vi } from "vitest";
import { candidateResponse } from "@/testdata/attackCandidates/candidateResponse";
import { jsonResponse, textResponse } from "@/testdata/http";
import { decodeCaseId } from "../contracts/cases";
import { fetchAttackCandidates } from "./attackCandidates";

afterEach(() => vi.unstubAllGlobals());

const baseRequest = {
  depth: 1,
  matchConditions: {
    conditions: [{ conditionKey: "destination_ip" as const }],
  },
};
const emptyResponse = candidateResponse([], []);

test("全graph条件を同名のquery parameterで送る", async () => {
  const fetchMock = vi.fn(async (_input: string, _options?: RequestInit) =>
    jsonResponse(200, emptyResponse),
  );
  vi.stubGlobal("fetch", fetchMock);
  const result = await fetchAttackCandidates({
    ...baseRequest,
    nodeLimit: 500,
    nodeKinds: ["process"],
    nodeIds: ["n:process:a"],
    edgeKinds: ["process_injection"],
    eventCategory: "proc",
    eventAction: "inject",
    caseId: decodeCaseId("case-a", "caseId"),
    addressInCidr: "192.0.2.0/24",
    addressNotInCidr: "198.51.100.0/24",
    valueContains: ["example"],
    countBy: "process.name",
    timeFilter: {
      from: { text: "2026-09-24T10:00:00+09:00", precision: "second" },
      to: { text: "2026-09-24T10:01:00+09:00", precision: "second" },
      unit: "second",
    },
  });
  expect(result).toEqual({ ok: true, value: emptyResponse });
  const url = new URL(
    fetchMock.mock.calls[0]?.[0] ?? "",
    "http://example.test",
  );
  expect(url.pathname).toBe("/api/v0/attack-candidates");
  expect(Object.fromEntries(url.searchParams)).toEqual({
    nodeLimit: "500",
    nodeKind: "process",
    nodeId: "n:process:a",
    depth: "1",
    edgeKind: "process_injection",
    eventCategory: "proc",
    eventAction: "inject",
    case: "case-a",
    addressInCidr: "192.0.2.0/24",
    addressNotInCidr: "198.51.100.0/24",
    valueContains: "example",
    countBy: "process.name",
    timeFrom: "2026-09-24T10:00:00+09:00",
    timeFromPrecision: "second",
    timeTo: "2026-09-24T10:01:00+09:00",
    timeToPrecision: "second",
    filterUnit: "second",
    matchCondition: "destination_ip",
  });
});

test("HTTP失敗を既存のApiResultへ変換する", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => textResponse(503, "unavailable")),
  );
  const result = await fetchAttackCandidates(baseRequest);
  expect(result.ok).toBe(false);
  if (!result.ok) expect(result.failure.kind).toBe("server");
});

test("不正な応答をresponse_unreadableへ変換する", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse(200, { rules: [], matches: null })),
  );
  const result = await fetchAttackCandidates(baseRequest);
  expect(result.ok).toBe(false);
  if (!result.ok) expect(result.failure.kind).toBe("response_unreadable");
});

test("abort signalを送信へ渡し、中断をnetwork failureとして返す", async () => {
  const controller = new AbortController();
  controller.abort();
  const fetchMock = vi.fn(async (_url: string, options: RequestInit) => {
    expect(options.signal).toBe(controller.signal);
    throw new DOMException("Aborted", "AbortError");
  });
  vi.stubGlobal("fetch", fetchMock);
  const result = await fetchAttackCandidates(baseRequest, controller.signal);
  expect(fetchMock).toHaveBeenCalledTimes(1);
  expect(result.ok).toBe(false);
  if (!result.ok) expect(result.failure.kind).toBe("network");
});
