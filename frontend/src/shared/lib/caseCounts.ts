import type { CaseEvidenceCount } from "../contracts/cases";
import { formatCount } from "./format";

/**
 * 根拠の件数を案件ごとの「案件: 件数」の組で並べる。例: `baseline: 10、challenge: 2`。
 *
 * 案件の識別子は英数字と `.`、`_`、`-` だけの文字列であり (`decodeCaseId` が確かめる)、
 * 制御文字を持たないためそのまま出す。
 */
export function describeEvidenceByCase(
  counts: readonly CaseEvidenceCount[],
): string {
  return counts
    .map((count) => `${count.caseId}: ${formatCount(count.evidenceCount)}`)
    .join("、");
}

/**
 * 「合計: 件数」の組の後ろに、案件ごとの件数の組を「、」で並べた文字列を作る。
 * 案件ごとの件数が無い応答では、全体の件数だけを返す。
 */
export function formatEvidenceCountWithCases(
  total: number,
  byCase: readonly CaseEvidenceCount[] | undefined,
): string {
  if (byCase === undefined) {
    return formatCount(total);
  }
  return `合計: ${formatCount(total)}、${describeEvidenceByCase(byCase)}`;
}
