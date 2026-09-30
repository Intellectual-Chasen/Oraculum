import type { Timestamp, TimestampInterpretation } from "../contracts/common";
import type { TerminalAssignment } from "../contracts/terminalAssignments";
import { formatCount } from "../lib/format";
import { listKey } from "../lib/listKey";
import { terminalAssignmentOriginLabels } from "../lib/terminalAssignmentLabels";
import { utcTextOf } from "../lib/timestampInstant";
import { Hint } from "./Hint";
import { MissingValue } from "./MissingValue";
import { RawText } from "./RawText";
import {
  InterpretationNote,
  offsetUnknownDescription,
  offsetUnknownLabel,
} from "./TimestampOffsetNote";
import { TimestampText } from "./TimestampText";

/** 割り当てがフィールドを持たないときの表示。 */
const unspecifiedDescription = "指定なし";

/** 割り当てが持つ文字列を出す。フィールドを持たないときは欠測として出す。 */
function OptionalText({ text }: { text: string | undefined }) {
  if (text === undefined) {
    return <MissingValue description={unspecifiedDescription} />;
  }
  return <RawText text={text} />;
}

type TerminalAssignmentTableProps = {
  /** 出す割当。要素数 1 以上で渡す。 */
  assignments: TerminalAssignment[];
  /** 表の見出し。件数の前に置く。 */
  caption: string;
  /**
   * 収集元の sourceId から file 名を探す表。「適用する収集元」と「期間を取った収集元」を
   * file 名で出す。表に無い収集元は sourceId をそのまま出す。
   */
  sourceFileNames: ReadonlyMap<string, string> | undefined;
  /**
   * 収集元の sourceId から、その収集元の今の時刻の解釈を探す表。解釈のある収集元から期間を
   * 取った割当は、地方時の期間をその解釈で読んで UTC で出す。出ない場合は期間をそのまま出す。
   */
  sourceInterpretations?: ReadonlyMap<string, TimestampInterpretation>;
};

/** 地方時の時刻に、期間を取った収集元の解釈を与える。解釈を持つ時刻はそのまま返す。 */
function interpretedOf(
  timestamp: Timestamp,
  interpretation: TimestampInterpretation | undefined,
): Timestamp {
  return interpretation === undefined ||
    timestamp.interpretation !== undefined ||
    timestamp.normalizedForm !== "local_without_offset"
    ? timestamp
    : { ...timestamp, interpretation };
}

/**
 * 割り当ての適用期間を「始まり – 終わり」で出す。期間の収集元のタイムゾーンで UTC に直せない
 * 期間には「タイムゾーン不明」の印を添える。
 */
function AssignmentPeriod({
  assignment,
  interpretation,
}: {
  assignment: TerminalAssignment;
  interpretation: TimestampInterpretation | undefined;
}) {
  const from = interpretedOf(
    assignment.assignmentValidRange.from,
    interpretation,
  );
  const to = interpretedOf(assignment.assignmentValidRange.to, interpretation);
  return (
    <>
      <TimestampText timestamp={from} /> – <TimestampText timestamp={to} />
      {/* 与えたタイムゾーンで読んだ期間は原文の事実ではない。タイムゾーンだけを添える。
          表示のタイムゾーンの時刻は TimestampText が両端の title に入れるため、
          TimestampOffsetNote を使わない。 */}
      {from.interpretation === undefined ? null : (
        <InterpretationNote interpretation={from.interpretation} />
      )}
      {utcTextOf(from) === undefined || utcTextOf(to) === undefined ? (
        <strong style={{ display: "block" }}>
          <Hint text={offsetUnknownDescription}>{offsetUnknownLabel}</Hint>
        </strong>
      ) : null}
    </>
  );
}

/** 収集元を file 名で出す。表に無い収集元は sourceId で出す。 */
function sourceNameOf(
  sourceId: string,
  sourceFileNames: ReadonlyMap<string, string> | undefined,
): string {
  return sourceFileNames?.get(sourceId) ?? sourceId;
}

/** 端末の割当を 1 件 1 行で出す表。割当の一覧と関係の詳細が使う。 */
export function TerminalAssignmentTable({
  assignments,
  caption,
  sourceFileNames,
  sourceInterpretations,
}: TerminalAssignmentTableProps) {
  return (
    <table>
      <caption>{`${caption}: ${formatCount(assignments.length)} 件`}</caption>
      <thead>
        <tr>
          <th scope="col">接続元 IP</th>
          <th scope="col">端末 ID</th>
          <th scope="col">端末の表示名</th>
          <th scope="col">ホスト名</th>
          <th scope="col">記録した方法</th>
          <th scope="col">割り当てる収集元</th>
          <th scope="col">期間の収集元</th>
          <th scope="col">適用期間</th>
          <th scope="col">理由</th>
          <th scope="col">分析者</th>
          <th scope="col">根拠のレコード</th>
        </tr>
      </thead>
      <tbody>
        {assignments.map((assignment, position) => (
          <tr
            // **一覧の位置を鍵に入れる。** 同じ内容の割当を拒む前に保存した割当は、同じ内容で
            // 並びうるため、項目だけでは行を分けられない。割当の並びは応答が決め、同じ応答の
            // 中で変わらない。
            key={listKey([
              position,
              assignment.origin,
              assignment.clientIp,
              assignment.terminalId,
              assignment.terminalHostname,
              assignment.sourceId,
              assignment.appliesToSourceId,
              assignment.derivation,
            ])}
          >
            <td>
              <OptionalText text={assignment.clientIp} />
            </td>
            <td>
              <OptionalText text={assignment.terminalId} />
            </td>
            <td>
              <OptionalText text={assignment.terminalHostname} />
            </td>
            <td>
              <OptionalText text={assignment.terminalHostnames?.join(", ")} />
            </td>
            <td>{terminalAssignmentOriginLabels[assignment.origin]}</td>
            <td>
              {assignment.appliesToSourceId === undefined ? (
                // 収集元の全体に付けない割当は、接続元 IP と端末の対応だけを表す。
                <MissingValue description="接続元 IP で判定" />
              ) : (
                <RawText
                  text={sourceNameOf(
                    assignment.appliesToSourceId,
                    sourceFileNames,
                  )}
                />
              )}
            </td>
            <td>
              <RawText
                text={sourceNameOf(assignment.sourceId, sourceFileNames)}
              />
            </td>
            <td>
              <AssignmentPeriod
                assignment={assignment}
                interpretation={sourceInterpretations?.get(assignment.sourceId)}
              />
            </td>
            <td>
              <OptionalText text={assignment.derivation} />
            </td>
            <td>
              <OptionalText text={assignment.author} />
            </td>
            <td>
              {assignment.basisRecordRefs === undefined ? (
                <MissingValue description={unspecifiedDescription} />
              ) : (
                `${formatCount(assignment.basisRecordRefs.length)} 件`
              )}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
