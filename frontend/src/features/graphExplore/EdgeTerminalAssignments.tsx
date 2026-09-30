import type { TimestampInterpretation } from "@/shared/contracts/common";
import type { EdgeKind } from "@/shared/contracts/graph";
import type { TerminalAssignment } from "@/shared/contracts/terminalAssignments";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { TerminalAssignmentTable } from "@/shared/ui/TerminalAssignmentTable";

/**
 * エッジを作った端末の割り当てを出す。
 *
 * **割り当てから作ったエッジであることを明示する。** 割り当ての IP とホスト名は分析者か利用者の
 * 入力であり、収集元のレコードが記録した値と分けて読む。
 *
 * **エッジの種類で、割り当てと照合した値を書き分ける。** 引数が指す対象のエッジは、引数の
 * ホスト名と割り当てのホスト名の文字列の一致で割り当ての端末へ結ぶ。ほかの種類は割り当ての
 * IP アドレスから作る。
 */
export function EdgeTerminalAssignments({
  edgeKind,
  assignments,
  sourceFileNames,
  sourceInterpretations,
}: {
  edgeKind: EdgeKind;
  assignments: TerminalAssignment[] | undefined;
  sourceFileNames: ReadonlyMap<string, string> | undefined;
  /** 収集元の sourceId から、その収集元の今の時刻の解釈を探す表。地方時の適用期間をこの解釈で読む。 */
  sourceInterpretations?: ReadonlyMap<string, TimestampInterpretation>;
}) {
  if (assignments === undefined || assignments.length === 0) {
    return null;
  }
  const matchedBy =
    edgeKind === "argument_names_object"
      ? "コマンドの引数のホスト名"
      : "IP アドレス";
  return (
    <>
      <h3>エッジを作った端末の割り当て</h3>
      <KeyValueList pairs={[{ name: "割り当ての照合", value: matchedBy }]} />
      <TerminalAssignmentTable
        assignments={assignments}
        caption="端末の割り当て"
        sourceFileNames={sourceFileNames}
        sourceInterpretations={sourceInterpretations}
      />
    </>
  );
}
