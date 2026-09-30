import { afterEach, expect, test, vi } from "vitest";
import { jsonResponse } from "@/testdata/http";
import {
  analystAssignmentJson,
  assignmentSourceId,
  assignmentSourceSha256,
  assignmentValidRangeJson,
  terminalAssignmentsResponseJson,
} from "@/testdata/terminalAssignments/terminalAssignmentsResponse";
import { decodeTimeRange } from "../contracts/common";
import {
  createTerminalAssignment,
  fetchTerminalAssignments,
  type TerminalAssignmentDraft,
} from "./terminalAssignments";

afterEach(() => {
  vi.unstubAllGlobals();
});

function stubFetch(result: Response) {
  const mock = vi.fn(async (_input: string, _init?: RequestInit) => result);
  vi.stubGlobal("fetch", mock);
  return mock;
}

// 本 test が送る根拠のレコード。
const basisRecordRef = {
  sourceContentSha256: assignmentSourceSha256,
  positionKind: "byte_range" as const,
  byteOffset: 1024,
};

// 3 項目を持たない入力。各 test が必要な項目を足す。
const baseDraft: TerminalAssignmentDraft = {
  sourceId: assignmentSourceId,
  sourceContentSha256: assignmentSourceSha256,
  assignmentValidRange: decodeTimeRange(assignmentValidRangeJson, "$"),
  derivation: "別の端末の ssh の接続先から導いた",
  basisRecordRefs: [basisRecordRef],
  author: "analyst-a",
  appliesToSourceId: assignmentSourceId,
};

test("一覧を取得し、取り込みの起動で指定した割当を読む", async () => {
  const mock = stubFetch(jsonResponse(200, terminalAssignmentsResponseJson()));

  const result = await fetchTerminalAssignments();

  expect(mock.mock.calls[0]?.[0]).toBe("/api/v0/terminal-assignments");
  expect(result.ok).toBe(true);
  if (!result.ok) {
    return;
  }
  expect(result.value.assignments.map((item) => item.origin)).toContain(
    "import_specified",
  );
});

test("端末の表示名だけの割当を、空白の項目を省いて送る", async () => {
  // backend は送った項目だけを持つ割当を返す。
  const { clientIp: _clientIp, ...recorded } = analystAssignmentJson();
  const mock = stubFetch(
    jsonResponse(201, { ...recorded, terminalHostname: "host-a.example.test" }),
  );

  const result = await createTerminalAssignment({
    ...baseDraft,
    clientIp: "  ",
    terminalId: "",
    terminalHostname: " host-a.example.test ",
  });

  const init = mock.mock.calls[0]?.[1];
  expect(init?.method).toBe("POST");
  const body = JSON.parse(String(init?.body));
  expect(body.terminalHostname).toBe("host-a.example.test");
  expect("clientIp" in body).toBe(false);
  expect("terminalId" in body).toBe(false);
  expect(body.appliesToSourceId).toBe(assignmentSourceId);
  expect(result.ok).toBe(true);
  if (!result.ok) {
    return;
  }
  expect(result.value.terminalHostname).toBe("host-a.example.test");
  expect(result.value.terminalId).toBeUndefined();
});

test("接続元 IP だけの割当を送る", async () => {
  const mock = stubFetch(jsonResponse(201, analystAssignmentJson()));

  await createTerminalAssignment({ ...baseDraft, clientIp: "192.0.2.10" });

  const body = JSON.parse(String(mock.mock.calls[0]?.[1]?.body));
  expect(body.clientIp).toBe("192.0.2.10");
  expect("terminalId" in body).toBe(false);
  expect("terminalHostname" in body).toBe(false);
});

test("著者を省いた入力は、本文に著者を載せずに送る", async () => {
  const mock = stubFetch(jsonResponse(201, analystAssignmentJson()));

  const result = await createTerminalAssignment({
    ...baseDraft,
    clientIp: "192.0.2.10",
    author: undefined,
  });

  expect(result.ok).toBe(true);
  const body = JSON.parse(String(mock.mock.calls[0]?.[1]?.body));
  expect("author" in body).toBe(false);
});

test("3 項目がすべて空白の入力を、送る前に退ける", async () => {
  const mock = stubFetch(jsonResponse(201, analystAssignmentJson()));

  const result = await createTerminalAssignment({
    ...baseDraft,
    clientIp: " ",
    terminalId: "",
    terminalHostname: "\t",
  });

  expect(mock).not.toHaveBeenCalled();
  expect(result.ok).toBe(false);
  if (result.ok) {
    return;
  }
  expect(result.failure.kind).toBe("request_rejected");
  expect(result.failure.summary).toBe("接続元 IP・端末 ID・端末の表示名なし");
});

test("端末の識別子を省き、選んだ収集元に適用しない入力を、送る前に退ける", async () => {
  const mock = stubFetch(jsonResponse(201, analystAssignmentJson()));

  for (const appliesToSourceId of [undefined, "ingest-host-z-1"]) {
    const result = await createTerminalAssignment({
      ...baseDraft,
      terminalHostname: "host-a.example.test",
      appliesToSourceId,
    });
    expect(result.ok).toBe(false);
    if (result.ok) {
      continue;
    }
    expect(result.failure.kind).toBe("request_rejected");
    expect(result.failure.summary).toBe("端末 ID か収集元の全体の指定が必要");
  }
  expect(mock).not.toHaveBeenCalled();
});

test("端末の識別子と接続元 IP を持つ入力は、収集元に適用しなくても送る", async () => {
  const mock = stubFetch(jsonResponse(201, analystAssignmentJson()));

  const result = await createTerminalAssignment({
    ...baseDraft,
    terminalId: "host-a",
    clientIp: "192.0.2.10",
    appliesToSourceId: undefined,
  });

  expect(mock).toHaveBeenCalledTimes(1);
  const body = JSON.parse(String(mock.mock.calls[0]?.[1]?.body));
  expect(body.terminalId).toBe("host-a");
  expect("appliesToSourceId" in body).toBe(false);
  expect(result.ok).toBe(true);
});

test("接続元 IP もホスト名も持たず、収集元にも適用しない入力を、送る前に退ける", async () => {
  const mock = stubFetch(jsonResponse(201, analystAssignmentJson()));

  const result = await createTerminalAssignment({
    ...baseDraft,
    terminalId: "host-a",
    clientIp: " ",
    terminalHostnames: [],
    appliesToSourceId: undefined,
  });

  expect(mock).not.toHaveBeenCalled();
  expect(result.ok).toBe(false);
  if (!result.ok) {
    expect(result.failure.summary).toBe(
      "接続元 IP・ホスト名か収集元の全体の指定が必要",
    );
  }
});

test("端末の識別子とホスト名だけの入力を、収集元に適用せずに送る", async () => {
  const mock = stubFetch(jsonResponse(201, analystAssignmentJson()));

  const result = await createTerminalAssignment({
    ...baseDraft,
    terminalId: "host-a",
    terminalHostnames: ["host-a.example.test"],
    appliesToSourceId: undefined,
  });

  expect(mock).toHaveBeenCalledTimes(1);
  const body = JSON.parse(String(mock.mock.calls[0]?.[1]?.body));
  expect(body.clientIp).toBeUndefined();
  expect(body.terminalHostnames).toEqual(["host-a.example.test"]);
  expect(result.ok).toBe(true);
});
