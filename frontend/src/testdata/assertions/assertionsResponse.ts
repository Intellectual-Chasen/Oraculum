/**
 * 分析者の所見の操作 (`/api/v0/assertions`) の応答の JSON。
 * 識別子は backend が発行する不透明な値であり、本 fixture が決める。
 */

import { hostALogSha256 } from "../sources/sourcesResponse";

/** 所見が指す原資料のレコード。取り込みをまたいで同じ値になる材料だけを持つ。 */
export const assertionRecordRef = {
  sourceContentSha256: hostALogSha256,
  positionKind: "sequence_number" as const,
  sequenceNumber: 112,
  lineNumber: 1022,
};

/** fixture の所見が含むメモ。 */
export const assertionNote = "接続先 port 5985 を WinRM と読んだ";

/** 記録した所見 1 件。 */
export function assertionItemJson() {
  return {
    assertion: {
      id: "as:0001",
      target: { kind: "record", record: assertionRecordRef },
      state: "active",
      author: "analyst-a",
      recordedAt: "2026-01-02T03:04:05.000Z",
      basis: {
        note: assertionNote,
        recordRefs: [assertionRecordRef],
      },
      revisionNumber: 1,
      history: [],
    },
    targetOrigin: "observation",
  };
}

/** 所見の一覧。所見 1 件を含む。 */
export function assertionsResponseJson() {
  return {
    assertions: [assertionItemJson()],
    assertionCount: 1,
  };
}

/**
 * 所見 1 件を含みながら、`assertionCount` が 2 件を名乗る一覧。
 * 一覧に出ていない所見があることを、総数と要素数の食い違いが表す。
 */
export function countMismatchAssertionsResponseJson() {
  return {
    assertions: [assertionItemJson()],
    assertionCount: 2,
  };
}

/** 所見を 1 件も持たない一覧。 */
export function emptyAssertionsResponseJson() {
  return {
    assertions: [],
    assertionCount: 0,
    emptyReason: "no_record_in_filter",
  };
}
