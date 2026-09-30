import type { LoadingSourceDraft } from "@/shared/api/stages";
import type { SourceFileEntry } from "@/shared/contracts/sourceFiles";
import type { FormatKey } from "@/shared/contracts/sources";

/** 読み込む収集元 1 件。基準の directory の一覧で選んだ file と、分析者が決めた指定を持つ。 */
export type LoadingFormRow = {
  /** 基準の directory からの相対 path。行を区別する。 */
  originPath: string;
  /** file の byte 数。 */
  sizeBytes?: number;
  /** server が判定した入力形式の候補。 */
  formatCandidates: FormatKey[];
  /** 読み込む入力形式。空文字列は未選択である。 */
  formatKey: string;
  formatSpec: string;
  terminalId: string;
  terminalHostname: string;
  terminalIp: string;
};

/** 入力欄を持つ項目の名前。 */
export type LoadingFormField =
  | "formatKey"
  | "formatSpec"
  | "terminalId"
  | "terminalHostname"
  | "terminalIp";

/**
 * 一覧の file から読み込む収集元を作る。**候補が 1 つだけの file は、その形式を選んだ状態にする。**
 * 候補が無い file と候補が複数の file は、分析者が形式を選ぶまで未選択である。
 */
export function loadingRowOf(entry: SourceFileEntry): LoadingFormRow {
  const [only, ...rest] = entry.formatCandidates;
  return {
    originPath: entry.originPath,
    sizeBytes: entry.sizeBytes,
    formatCandidates: entry.formatCandidates,
    formatKey: only !== undefined && rest.length === 0 ? only : "",
    formatSpec: "",
    terminalId: "",
    terminalHostname: "",
    terminalIp: "",
  };
}

/** 一覧の file を並びの後ろに足す。既に選んだ path は足さず、先に選んだ指定を保つ。 */
export function withAddedRows(
  rows: LoadingFormRow[],
  entries: SourceFileEntry[],
): LoadingFormRow[] {
  const selected = new Set(rows.map((row) => row.originPath));
  const added = entries
    .filter((entry) => entry.kind === "file" && !selected.has(entry.originPath))
    .map(loadingRowOf);
  return added.length === 0 ? rows : [...rows, ...added];
}

/** 入力形式を選んだ行か。未選択の行は読み込みの要求に載せない。 */
export function hasFormat(row: LoadingFormRow): boolean {
  return (
    row.formatKey !== "" &&
    (!requiresLogFormatSpec(row.formatKey) || row.formatSpec.trim() !== "")
  );
}

/** 独自のログ書式を指定する入力形式か。 */
export function requiresLogFormatSpec(formatKey: string): boolean {
  return formatKey === "squid_logformat";
}

/** 編集した資料へ書式を適用し、一括指定では現在Squid形式を選択した資料も更新する。 */
export function withLogFormatSpec(
  rows: LoadingFormRow[],
  originPath: string,
  spec: string,
  applyToSquid: boolean,
): LoadingFormRow[] {
  return rows.map((row) => {
    const isSquid =
      row.formatKey === "squid_combined" ||
      row.formatKey === "squid_combined_request_bytes" ||
      requiresLogFormatSpec(row.formatKey);
    return row.originPath === originPath || (applyToSquid && isSquid)
      ? { ...row, formatKey: "squid_logformat", formatSpec: spec }
      : row;
  });
}

/**
 * 行の欄を入力の文字列から組んだ、読み込みを始める要求の入力。`caseId` はすべての収集元に付ける。
 * 空白の扱いは通信の境界が決める。
 */
export function loadingDraftOf(
  row: LoadingFormRow,
  caseId: string,
): LoadingSourceDraft {
  return {
    originPath: row.originPath,
    formatKey: row.formatKey,
    formatSpec: requiresLogFormatSpec(row.formatKey)
      ? row.formatSpec
      : undefined,
    caseId,
    terminal: {
      id: row.terminalId,
      hostname: row.terminalHostname,
      ip: row.terminalIp,
    },
  };
}
