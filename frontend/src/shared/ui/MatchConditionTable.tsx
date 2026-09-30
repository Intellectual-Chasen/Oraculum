import type { MatchAssumption, MatchCondition } from "../contracts/candidates";
import type { RecordField } from "../contracts/common";
import { conditionKeyLabels, conditionUseLabels } from "../lib/conditionLabels";
import { toVisibleRawText } from "../lib/rawText";
import {
  readRecordFieldNormalized,
  readRecordFieldRawText,
} from "../lib/recordField";
import { Hint } from "./Hint";
import { MissingValue } from "./MissingValue";
import { RecordFieldList } from "./RecordFieldList";

const leftValueAbsent = "値なし";
// 時刻の条件と、候補が 0 件の段階は候補の値を持たない。
const rightValueAbsent = "値なし";

/** 原文の列と、比較に使う正規化値の列を別に出す。 */
function SideCells({
  fields,
  absence,
}: {
  fields: RecordField[] | undefined;
  absence: string;
}) {
  if (fields === undefined) {
    return (
      <>
        <td>
          <MissingValue description={absence} />
        </td>
        <td>
          <MissingValue description={absence} />
        </td>
      </>
    );
  }
  return (
    <>
      <td>
        <RecordFieldList fields={fields} readValue={readRecordFieldRawText} />
      </td>
      <td>
        <RecordFieldList
          fields={fields}
          readValue={readRecordFieldNormalized}
        />
      </td>
    </>
  );
}

/**
 * エッジの推定に使った条件と、使わなかった条件の全数を出す。基準のレコードと候補のレコードの
 * それぞれの原文と比較の値を列にする。使った条件の件数は `use` が `used` の行の個数であり、
 * 別の列に出さない。
 *
 * 推定の段階と、子のプロセスの成立条件の両方が同じ表を使う。
 *
 * 段階が候補の接続先を Proxy のアドレスに限ったときは、接続先 IP の行にその前提を出す。
 * 基準の接続先と比べていなくても、候補は接続先で絞っている。
 */
export function MatchConditionTable({
  conditions,
  tableLabel,
  assumptions,
}: {
  conditions: MatchCondition[];
  /** 表の見出しに入れる名前。1 画面に 2 つ以上の表を置くため、対象ごとに変える。 */
  tableLabel: string;
  /** 段階が依拠する前提。 */
  assumptions?: MatchAssumption[];
}) {
  const proxy = assumptions?.find(
    (assumption) =>
      assumption.assumptionKey === "counterpart_connected_to_proxy",
  );
  return (
    <table>
      <caption>{tableLabel}</caption>
      <thead>
        <tr>
          <th scope="col">条件</th>
          <th scope="col">使用</th>
          <th scope="col">基準の原文</th>
          <th scope="col">基準の比較値</th>
          <th scope="col">候補の原文</th>
          <th scope="col">候補の比較値</th>
        </tr>
      </thead>
      <tbody>
        {conditions.map((condition) => (
          <tr key={condition.conditionKey}>
            <th scope="row">{conditionKeyLabels[condition.conditionKey]}</th>
            <td>
              {conditionUseLabels[condition.use]}
              {proxy !== undefined &&
              condition.conditionKey === "destination_ip" &&
              condition.use !== "used" ? (
                <p>
                  <Hint text={toVisibleRawText(proxy.statement)}>
                    前提: 候補の接続先は Proxy
                  </Hint>
                </p>
              ) : null}
            </td>
            <SideCells fields={condition.leftValue} absence={leftValueAbsent} />
            <SideCells
              fields={condition.rightValue}
              absence={rightValueAbsent}
            />
          </tr>
        ))}
      </tbody>
    </table>
  );
}
