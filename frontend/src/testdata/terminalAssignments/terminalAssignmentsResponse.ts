/**
 * 端末の割当の操作 (`/api/v0/terminal-assignments`) の応答の JSON。
 * IP は文書用の範囲、表示名は `example.test` である。
 */

/** 割当の期間を読み取った収集元の取り込み 1 件。 */
export const assignmentSourceId = "ingest-host-a-1";

/** 割当の期間を読み取った収集元の内容の識別。 */
export const assignmentSourceSha256 =
  "0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c4b5a69788796a5b4c3d2e1f0";

/** 割当を適用してよい期間。 */
export const assignmentValidRangeJson = {
  from: {
    normalized: "2026-01-02T03:00:00.000Z",
    normalizedForm: "rfc3339_absolute",
    precision: "millisecond",
    offsetState: "epoch",
    clock: "terminal_local",
    meaning: "event",
    valueState: "present",
  },
  to: {
    normalized: "2026-01-02T04:00:00.000Z",
    normalizedForm: "rfc3339_absolute",
    precision: "millisecond",
    offsetState: "epoch",
    clock: "terminal_local",
    meaning: "event",
    valueState: "present",
  },
};

/** 取り込みの起動で指定した割当 1 件。端末の表示名だけを持つ。 */
export function importSpecifiedAssignmentJson(): Record<string, unknown> {
  return {
    terminalHostname: "host-a.example.test",
    sourceId: assignmentSourceId,
    sourceContentSha256: assignmentSourceSha256,
    assignmentValidRange: assignmentValidRangeJson,
    origin: "import_specified",
    appliesToSourceId: assignmentSourceId,
  };
}

/** 収集元のレコードが記録した割当 1 件。3 項目をすべて持つ。 */
export function observedAssignmentJson(): Record<string, unknown> {
  return {
    clientIp: "192.0.2.20",
    terminalId: "host-b",
    terminalHostname: "host-b.example.test",
    sourceId: assignmentSourceId,
    sourceContentSha256: assignmentSourceSha256,
    assignmentValidRange: assignmentValidRangeJson,
    origin: "observed_in_source",
  };
}

/** 画面から記録した割当 1 件。接続元 IP だけを持つ。 */
export function analystAssignmentJson(): Record<string, unknown> {
  return {
    clientIp: "192.0.2.10",
    sourceId: assignmentSourceId,
    sourceContentSha256: assignmentSourceSha256,
    assignmentValidRange: assignmentValidRangeJson,
    origin: "analyst_supplied",
    derivation: "別の端末の ssh の接続先から導いた",
    basisRecordRefs: [
      {
        sourceContentSha256: assignmentSourceSha256,
        positionKind: "byte_range",
        byteOffset: 1024,
      },
    ],
    author: "analyst-a",
    appliesToSourceId: assignmentSourceId,
  };
}

/**
 * 割当の一覧。取り込みの起動で指定した割当を先に並べ、画面から記録した割当を後ろに並べる。
 * 一覧の操作は収集元のレコードが記録した割当を返さない (`backend/api/terminal_assignments.go`)。
 */
export function terminalAssignmentsResponseJson() {
  const assignments = [
    importSpecifiedAssignmentJson(),
    analystAssignmentJson(),
  ];
  return { assignments, assignmentCount: assignments.length };
}
