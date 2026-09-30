import type { CaseId } from "@/shared/contracts/cases";
import type {
  DiagnosisClass,
  DiagnosisCount,
  ImportCategory,
  ImportCount,
  ImportFailure,
  SkippedFile,
  SourcesResponse,
  SourceWithImportStatus,
} from "@/shared/contracts/sources";
import {
  describeRecordPosition,
  describeSpan,
  recordPositionAbsent,
} from "@/shared/lib/recordPosition";

/** 一覧の 1 行。収集元と、その収集元の取り込みの状態を組にする。 */
export type SourceInventoryRow = SourceWithImportStatus;

/** 一覧 1 つ分。取り込んだ収集元の全件と総数を併せて持つ。 */
export type SourceInventory = {
  rows: SourceInventoryRow[];
  /** 収集元の総数。 */
  sourceCount: number;
  /** 収集の directory にあり取り込まなかった file。 */
  skippedFiles: SkippedFile[];
};

/** 操作 1 の応答を一覧の行へ組み替える。 */
export function buildSourceInventory(
  response: SourcesResponse,
): SourceInventory {
  return {
    rows: response.sources,
    sourceCount: response.sourceCount,
    skippedFiles: response.skippedFiles,
  };
}

/**
 * 収集元に付いた案件の識別子を、重複を除いて昇順に並べる。
 * 案件を持つ収集元が無い一覧では要素数 0 である。
 */
export function inventoryCaseIds(inventory: SourceInventory): CaseId[] {
  const caseIds = new Set<CaseId>();
  for (const row of inventory.rows) {
    if (row.source.caseId !== undefined) {
      caseIds.add(row.source.caseId);
    }
  }
  // 並びは応答の案件ごとの件数と同じ、文字列の code unit の昇順にする。
  return [...caseIds].sort((left, right) =>
    left < right ? -1 : left > right ? 1 : 0,
  );
}

/** 取り込み結果の区分 1 つの件数を取る。区分を数えていないときは `undefined` を返す。 */
export function findImportCount(
  counts: ImportCount[],
  category: ImportCategory,
): number | undefined {
  return counts.find((count) => count.category === category)?.count;
}

/**
 * 取り込めなかったレコード 1 件の範囲を画面の 1 欄に書く。レコードの位置か行番号を書き、
 * 行番号で指すレコードが行の始まりの byte 位置と長さを持つときは「位置: 始まり-終わり」を並べる。
 */
export function describeFailureRecord(failure: ImportFailure): string {
  const { recordRef, lineNumber } = failure;
  if (recordRef === undefined) {
    return lineNumber === undefined
      ? recordPositionAbsent
      : describeSpan("行", lineNumber);
  }
  const position = describeRecordPosition(recordRef);
  const { positionKind, byteOffset, byteLength } = recordRef;
  if (positionKind !== "line_number" || byteOffset === undefined) {
    return position;
  }
  return `${position}、${describeSpan("位置", byteOffset, byteLength)}`;
}

/**
 * 取り込めなかったレコード 1 件の、失敗した byte の位置を書く。応答が含まないときと、
 * レコードの byte 範囲の先頭と同じ値のときは `undefined` を返す。失敗した byte の位置は
 * レコードの中の失敗した byte を指し、レコードの先頭と違う値になることがある。
 */
export function describeFailedByte(failure: ImportFailure): string | undefined {
  const { recordRef, byteOffset } = failure;
  if (
    byteOffset === undefined ||
    (recordRef?.positionKind === "byte_range" &&
      recordRef.byteOffset === byteOffset)
  ) {
    return undefined;
  }
  return describeSpan("位置", byteOffset);
}

/** 失敗原因の分類 1 つの件数を取る。分類を数えていないときは `undefined` を返す。 */
export function findDiagnosisCount(
  diagnosisCounts: DiagnosisCount[],
  diagnosisClass: DiagnosisClass,
): number | undefined {
  return diagnosisCounts.find(
    (count) => count.diagnosisClass === diagnosisClass,
  )?.count;
}
