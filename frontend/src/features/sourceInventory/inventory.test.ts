import { expect, test } from "vitest";
import type { RecordLocator } from "@/shared/contracts/common";
import type {
  ImportFailure,
  ImportStatus,
  SourceIdentity,
  SourcesResponse,
} from "@/shared/contracts/sources";
import { baselineCaseId, challengeCaseId } from "@/testdata/cases/caseCounts";
import {
  buildSourceInventory,
  describeFailedByte,
  describeFailureRecord,
  findDiagnosisCount,
  findImportCount,
  inventoryCaseIds,
} from "./inventory";

function source(sourceId: string, fileName: string): SourceIdentity {
  return {
    sourceId,
    contentSha256: "0".repeat(64),
    originPath: `/data/example/${fileName}`,
    fileName,
    sizeBytes: 1,
    newlineCount: 0,
    endsWithNewline: true,
    lineEnding: "lf",
    formatKey: "squid_combined",
  };
}

function status(sourceId: string): ImportStatus {
  return {
    sourceId,
    scope: {
      sourceId,
      sourceContentSha256: "0".repeat(64),
      rangeKind: "whole_source",
    },
    counts: [
      { category: "read", count: 4 },
      { category: "failed", count: 0 },
    ],
    diagnosisCounts: [{ diagnosisClass: "undetermined", count: 1 }],
    publicationState: "published_partial",
    failures: [],
    analysisRunRef: "analysis-run-1",
  };
}

function response(): SourcesResponse {
  return {
    sources: [
      { source: source("a", "access.log"), importStatus: status("a") },
      { source: source("b", "host-a.log"), importStatus: status("b") },
    ],
    sourceCount: 2,
    skippedFiles: [],
  };
}

test("応答の収集元と取り込みの状態の組を一覧へ写す", () => {
  const inventory = buildSourceInventory(response());

  expect(inventory.rows.map((row) => row.source.fileName)).toEqual([
    "access.log",
    "host-a.log",
  ]);
  expect(inventory.rows[0]?.importStatus?.sourceId).toBe("a");
  expect(inventory.rows[1]?.importStatus?.sourceId).toBe("b");
  expect(inventory.sourceCount).toBe(inventory.rows.length);
});

test("収集元の案件を重複を除いて昇順に並べ、案件を持たない収集元を数えない", () => {
  const whole = response();
  const withCases: SourcesResponse = {
    sources: [
      {
        source: { ...source("c", "b.log"), caseId: challengeCaseId },
        importStatus: status("c"),
      },
      ...whole.sources,
      {
        source: { ...source("d", "a.log"), caseId: baselineCaseId },
        importStatus: status("d"),
      },
      {
        source: { ...source("e", "c.log"), caseId: challengeCaseId },
        importStatus: status("e"),
      },
    ],
    sourceCount: whole.sourceCount + 3,
    skippedFiles: [],
  };

  expect(inventoryCaseIds(buildSourceInventory(withCases))).toEqual([
    baselineCaseId,
    challengeCaseId,
  ]);
  expect(inventoryCaseIds(buildSourceInventory(whole))).toEqual([]);
});

test("区分の件数が 0 である状態と、区分を数えていない状態を分ける", () => {
  const counts = status("a").counts;

  expect(findImportCount(counts, "read")).toBe(4);
  expect(findImportCount(counts, "failed")).toBe(0);
  expect(findImportCount(counts, "succeeded")).toBeUndefined();
});

test("取り込めなかったレコードの行の範囲と、失敗した byte の位置を分けて書く", () => {
  const failure = (position: Partial<ImportFailure>): ImportFailure => ({
    diagnosisClass: "undetermined",
    stage: "tokenize",
    expectedMeaning: "a separator",
    observedResult: "the end of the record",
    ...position,
  });
  const recordRef: RecordLocator = {
    sourceId: "a",
    sourceContentSha256: "0".repeat(64),
    sourceFileName: "proxy.log",
    positionKind: "line_number",
    lineNumber: 1234,
    byteOffset: 50000,
    byteLength: 8191,
    recordRawTextRef: "raw:a-1234",
  };
  const cut = failure({ recordRef, lineNumber: 1234, byteOffset: 58191 });

  expect(describeFailureRecord(cut)).toBe("行: 1234、位置: 50000-58191");
  expect(describeFailedByte(cut)).toBe("位置: 58191");
  expect(describeFailureRecord(failure({ byteOffset: 0 }))).toBe("位置なし");
  expect(describeFailedByte(failure({ byteOffset: 0 }))).toBe("位置: 0");
  expect(describeFailureRecord(failure({ lineNumber: 7 }))).toBe("行: 7");
  expect(describeFailedByte(failure({ lineNumber: 7 }))).toBeUndefined();
});

test("byte 範囲で指すレコードの失敗は、失敗した byte の位置がレコードの先頭と違うときだけ書く", () => {
  const recordRef: RecordLocator = {
    sourceId: "a",
    sourceContentSha256: "0".repeat(64),
    sourceFileName: "audit.log",
    positionKind: "byte_range",
    lineNumber: 12,
    lineCount: 3,
    byteOffset: 1000,
    byteLength: 200,
    recordRawTextRef: "raw:a-1000",
  };
  const failure = (byteOffset: number): ImportFailure => ({
    recordRef,
    lineNumber: 12,
    byteOffset,
    diagnosisClass: "undetermined",
    stage: "tokenize",
    expectedMeaning: "a separator",
    observedResult: "the end of the record",
    unresolvedReason: "the record alone does not decide the class",
  });

  expect(describeFailureRecord(failure(1000))).toBe(
    "行: 12-14、位置: 1000-1200",
  );
  expect(describeFailedByte(failure(1000))).toBeUndefined();
  expect(describeFailedByte(failure(1037))).toBe("位置: 1037");
});

test("通番で指すレコードの失敗は、レコードの位置と失敗した byte の位置を分けて書く", () => {
  const failure: ImportFailure = {
    recordRef: {
      sourceId: "a",
      sourceContentSha256: "0".repeat(64),
      sourceFileName: "events.log",
      positionKind: "sequence_number",
      sequenceNumber: 112,
      lineNumber: 3,
      recordRawTextRef: "raw:a-112",
    },
    lineNumber: 3,
    byteOffset: 5000,
    diagnosisClass: "inconsistent_input_confirmed",
    stage: "normalize",
    expectedMeaning: "a timestamp",
    observedResult: "an empty value",
  };

  expect(describeFailureRecord(failure)).toBe("ID: 112、行: 3");
  expect(describeFailedByte(failure)).toBe("位置: 5000");
});

test("分類の件数が有る状態と、分類を数えていない状態を分ける", () => {
  const diagnosisCounts = status("a").diagnosisCounts;

  expect(findDiagnosisCount(diagnosisCounts, "undetermined")).toBe(1);
  expect(
    findDiagnosisCount(diagnosisCounts, "unsupported_format"),
  ).toBeUndefined();
});
