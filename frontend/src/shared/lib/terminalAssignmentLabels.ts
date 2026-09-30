import type { TerminalAssignmentOrigin } from "../contracts/terminalAssignments";

/** 端末の割り当てを記録した方法の表示。「記録した方法」の列の値である。 */
export const terminalAssignmentOriginLabels: Record<
  TerminalAssignmentOrigin,
  string
> = {
  observed_in_source: "収集元のレコード",
  import_specified: "起動時の指定",
  analyst_supplied: "分析者",
};
