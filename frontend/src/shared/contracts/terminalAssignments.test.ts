import { expect, test } from "vitest";
import {
  analystAssignmentJson,
  importSpecifiedAssignmentJson,
  observedAssignmentJson,
  terminalAssignmentsResponseJson,
} from "@/testdata/terminalAssignments/terminalAssignmentsResponse";
import { DecodeFailure } from "./decoding";
import {
  decodeTerminalAssignment,
  decodeTerminalAssignmentsResponse,
} from "./terminalAssignments";

test("省いた項目を undefined で受け、取り込みの起動で指定した由来を読む", () => {
  const decoded = decodeTerminalAssignment(
    importSpecifiedAssignmentJson(),
    "$",
  );
  expect(decoded.origin).toBe("import_specified");
  expect(decoded.terminalHostname).toBe("host-a.example.test");
  expect(decoded.clientIp).toBeUndefined();
  expect(decoded.terminalId).toBeUndefined();
  expect(decoded.appliesToSourceId).toBe(decoded.sourceId);
});

test("接続元 IP だけを持つ割当を読む", () => {
  const decoded = decodeTerminalAssignment(analystAssignmentJson(), "$");
  expect(decoded.clientIp).toBe("192.0.2.10");
  expect(decoded.terminalId).toBeUndefined();
  expect(decoded.terminalHostname).toBeUndefined();
  expect(decoded.origin).toBe("analyst_supplied");
});

test("ホスト名の並びを読み、文字列でない要素を退ける", () => {
  const decoded = decodeTerminalAssignment(
    {
      ...analystAssignmentJson(),
      terminalHostnames: ["pc01", "pc01.example.test"],
    },
    "$",
  );
  expect(decoded.terminalHostnames).toEqual(["pc01", "pc01.example.test"]);
  expect(
    decodeTerminalAssignment(analystAssignmentJson(), "$").terminalHostnames,
  ).toBeUndefined();
  expect(() =>
    decodeTerminalAssignment(
      { ...analystAssignmentJson(), terminalHostnames: [1] },
      "$",
    ),
  ).toThrow(DecodeFailure);
});

test("一覧の由来ごとの割当を読み、件数を要素の数と揃える", () => {
  const decoded = decodeTerminalAssignmentsResponse(
    terminalAssignmentsResponseJson(),
    "$",
  );
  expect(decoded.assignments.map((item) => item.origin)).toEqual([
    "import_specified",
    "analyst_supplied",
  ]);
  expect(decoded.assignmentCount).toBe(decoded.assignments.length);
});

test("収集元のレコードが記録した割当の 3 項目を読む", () => {
  const decoded = decodeTerminalAssignment(observedAssignmentJson(), "$");
  expect(decoded.origin).toBe("observed_in_source");
  expect([
    decoded.clientIp,
    decoded.terminalId,
    decoded.terminalHostname,
  ]).toEqual(["192.0.2.20", "host-b", "host-b.example.test"]);
});

test("定義に無い由来を退ける", () => {
  expect(() =>
    decodeTerminalAssignment(
      { ...observedAssignmentJson(), origin: "guessed_by_frontend" },
      "$",
    ),
  ).toThrow(DecodeFailure);
});

test("接続元 IP、端末の識別子、端末の表示名のいずれも持たない割当を退ける", () => {
  const {
    clientIp: _clientIp,
    terminalId: _terminalId,
    terminalHostname: _terminalHostname,
    ...rest
  } = observedAssignmentJson();
  expect(() => decodeTerminalAssignment(rest, "$")).toThrow(
    /at least one of clientIp, terminalId and terminalHostname/,
  );
});

test("3 項目のいずれかが文字列でない割当を退ける", () => {
  expect(() =>
    decodeTerminalAssignment(
      { ...importSpecifiedAssignmentJson(), terminalId: 7 },
      "$",
    ),
  ).toThrow(DecodeFailure);
});
