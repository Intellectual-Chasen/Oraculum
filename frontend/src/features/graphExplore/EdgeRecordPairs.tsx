import { Fragment, type ReactNode, useState } from "react";
import type { RecordField, RecordLocator } from "@/shared/contracts/common";
import {
  type EdgeCandidateTally,
  type EdgePairCondition,
  type EdgePairConditionKey,
  type EdgeRecordPair,
  timeEdgePairConditions,
} from "@/shared/contracts/edgeRecordPairs";
import type { GraphEvidence } from "@/shared/contracts/graph";
import { formatCount } from "@/shared/lib/format";
import {
  type FieldValue,
  readRecordFieldNormalized,
  readRecordFieldRawText,
} from "@/shared/lib/recordField";
import { describeRecordLocation } from "@/shared/lib/recordPosition";
import { Fold } from "@/shared/ui/Fold";
import { Hint } from "@/shared/ui/Hint";
import { KeyValueList, type KeyValuePair } from "@/shared/ui/KeyValueList";
import { MissingValue } from "@/shared/ui/MissingValue";
import { RawText } from "@/shared/ui/RawText";
import { RecordFieldList } from "@/shared/ui/RecordFieldList";
import { TimestampOffsetNote } from "@/shared/ui/TimestampOffsetNote";
import { ShowMoreRows } from "./ShownRows";

/** 条件の種類の短い名前。定義元は `backend/core/edge_record_pair.go` の各値の説明である。 */
export const edgePairConditionLabels: Record<EdgePairConditionKey, string> = {
  process_pid: "プロセス番号の一致",
  terminal: "同じ端末",
  time_order: "時刻の順",
  time_proximity: "時刻の近さ",
  time_overlap: "時刻の範囲の重なり",
  nearest_identity_record: "直前か直後の同じ識別子",
  logon_id: "Logon ID の一致",
  linked_logon_id: "対のログオンの Logon ID",
  logon_guid: "LogonGuid の一致",
  task_name: "タスクの名前の一致",
  account: "アカウントの一致",
  source_endpoint: "接続元のアドレスと port の一致",
  destination_endpoint: "宛先の port とアドレスの一致",
  destination_terminal: "接続先の端末の割り当て",
  source_unassigned: "接続元の割り当てなし",
  source_outside_assignment_range: "割り当ての期間の外",
  requesting_session: "要求したセッション",
  source_terminal: "接続元の端末の割り当て",
  session_start_logon: "始まり: ログオンの記録",
  session_start_first_operation: "始まり: 最初の操作の記録",
  session_start_logoff_record: "始まり: ログオフの記録の時刻",
  session_end_logoff: "終わり: ログオフの記録",
  session_end_system_start: "終わり: 次の起動の記録",
  session_end_time_limit: "終わり: 最後の操作と分析者の指定の時間",
  session_end_last_operation: "終わり: 最後の操作の記録",
  session_account_match: "アカウントが一致",
  session_account_different: "アカウントが不一致",
  session_logon_interactive: "対話か画面の遠隔操作のログオン",
  session_logon_network: "ネットワークのログオン",
  session_logon_other: "その他のログオン",
};

/** 条件の種類の補足。名前の tooltip に出す。始点と終点はエッジの両端のレコードを指す。 */
export const edgePairConditionDescriptions: Partial<
  Record<EdgePairConditionKey, string>
> = {
  time_order: "始点のレコードの時刻 ≤ 終点のレコードの時刻",
  time_proximity: "エッジの種類の定める幅の中の時刻の差",
  time_overlap: "2 つのプロセスのレコードの時刻の範囲",
  nearest_identity_record:
    "始点の時刻の直前か直後に同じ名前と識別子を記録した終点のレコード",
  logon_id: "ログオンが作ったセッションと操作を行ったセッション",
  linked_logon_id: "分けて作った 2 件のログオンの互いを指す Logon ID",
  task_name: "登録したタスクと起動したタスク",
  destination_terminal:
    "始点の接続先のアドレスから分析者の割り当てで求めた終点の端末",
  source_unassigned: "端末の割り当ての無い接続元のアドレス",
  source_outside_assignment_range:
    "どの割り当ての期間にも入らないレコードの時刻",
  requesting_session:
    "始点のセッションの Logon ID で終点のログオンを要求した記録",
  source_terminal:
    "終点の接続元のアドレスから分析者の割り当てで求めた始点の端末",
  session_start_logon: "終点の時刻: セッションの始まりの後",
  session_start_first_operation: "終点の時刻: セッションの始まりの後",
  session_start_logoff_record: "終点の時刻: セッションの始まりの後",
  session_end_logoff: "終点の時刻: セッションの終わりの前",
  session_end_system_start: "終点の時刻: セッションの終わりの前",
  session_end_time_limit: "終点の時刻: セッションの終わりの前",
  session_end_last_operation:
    "ログオフの記録の無いネットワークのログオン · 操作の無いときは始まりの記録",
  session_account_match: "始点のセッションと終点のログオンの SID か名前",
  session_account_different: "比べる値が片側に無い組を含む",
  session_logon_interactive: "ログオンタイプ: 2 · 7 · 10 · 11 · 12 · 13",
  session_logon_network: "ログオンタイプ: 3",
  session_logon_other:
    "対話・画面の遠隔操作・ネットワーク以外かログオンタイプなし",
};

/** 候補の順位の並べ方。候補の組の tooltip に出す。 */
const candidateOrderDescription =
  "候補の順: アカウントの一致 · 対話と画面の遠隔操作 · その他 · ネットワーク";

/** レコードを持たない端のラベル。始点が欠けるのは、始点が割り当ての無い IP アドレスの組である。 */
const noRecordSide = {
  left: "レコードなし: 割り当ての無い IP アドレスのノード",
  right: "レコードなし",
} as const;
type Side = keyof typeof noRecordSide;
/** 一度に描く組の数。組 1 つが十数行の表であり、1,000 組を一度に描くと画面が止まる。 */
const pairPageSize = 50;
const noFieldValue = "フィールドなし";
const noEventTime = "時刻なし";
/** 開いた状態で出す組の数の上限。組 1 つが表 1 つであり、行数が多い。 */
const openPairLimit = 3;

type SelectRecord = (recordRef: RecordLocator) => void;

/**
 * フィールドの原文を読み、原文が無いフィールドは比べた値を読む。レコードを置いた端末は
 * 収集元や割り当てから導いた値であり、原文を持たない。
 */
function readRawTextOrNormalized(field: RecordField): FieldValue {
  const raw = readRecordFieldRawText(field);
  if ("text" in raw) {
    return raw;
  }
  const normalized = readRecordFieldNormalized(field);
  return "text" in normalized ? normalized : raw;
}

/** 組の片端のレコード。位置を Record に表示する値にする。 */
function PairSideCell({
  sideName,
  side,
  onSelectRecord,
}: {
  sideName: Side;
  side: GraphEvidence | undefined;
  onSelectRecord: SelectRecord;
}) {
  if (side === undefined) {
    return (
      <td>
        <MissingValue description={noRecordSide[sideName]} />
      </td>
    );
  }
  const ref = side.recordRef;
  return (
    <td>
      <button
        type="button"
        className="value-link"
        onClick={() => onSelectRecord(ref)}
      >
        <RawText text={describeRecordLocation(ref)} />
      </button>
    </td>
  );
}

/** 条件の片端の値。時刻の条件はレコードの時刻を、他の条件はフィールドを出す。 */
function ConditionSideCell({
  conditionKey,
  sideName,
  side,
  fields,
}: {
  conditionKey: EdgePairConditionKey;
  sideName: Side;
  side: GraphEvidence | undefined;
  fields: RecordField[];
}) {
  if (side === undefined) {
    return (
      <td>
        <MissingValue description={noRecordSide[sideName]} />
      </td>
    );
  }
  if (timeEdgePairConditions.has(conditionKey)) {
    // 比べたのは UTC 時刻である。タイムゾーンは収集元ごとに違うため、その出どころを添える。
    const time = side.eventTime;
    return (
      <td>
        {time?.rawText === undefined ? (
          <MissingValue description={noEventTime} />
        ) : (
          <>
            <RawText text={time.rawText} />
            <TimestampOffsetNote timestamp={time} />
          </>
        )}
      </td>
    );
  }
  return (
    <td>
      {fields.length === 0 ? (
        <MissingValue description={noFieldValue} />
      ) : (
        <RecordFieldList fields={fields} readValue={readRawTextOrNormalized} />
      )}
    </td>
  );
}

/**
 * 項目 1 つを、項目の名前の行と両端の 2 行で出す。名前を値と別の行に置き、始点と終点を縦に
 * 並べる。詳細の狭い列でも値の列の幅を保つ。
 */
function SideRows({
  label,
  description,
  left,
  right,
}: {
  label: string;
  description?: string;
  left: ReactNode;
  right: ReactNode;
}) {
  return (
    <tbody>
      <tr>
        <th scope="rowgroup" colSpan={2}>
          {description === undefined ? (
            label
          ) : (
            <Hint text={description}>{label}</Hint>
          )}
        </th>
      </tr>
      <tr>
        <th scope="row">始点</th>
        {left}
      </tr>
      <tr>
        <th scope="row">終点</th>
        {right}
      </tr>
    </tbody>
  );
}

function PairTable({
  pair,
  index,
  onSelectRecord,
}: {
  pair: EdgeRecordPair;
  index: number;
  onSelectRecord: SelectRecord;
}) {
  return (
    <table>
      <caption>{`レコードの組 ${formatCount(index + 1)}`}</caption>
      <thead>
        <tr>
          <th scope="col">端</th>
          <th scope="col">値</th>
        </tr>
      </thead>
      <SideRows
        label="レコード"
        left={
          <PairSideCell
            sideName="left"
            side={pair.left}
            onSelectRecord={onSelectRecord}
          />
        }
        right={
          <PairSideCell
            sideName="right"
            side={pair.right}
            onSelectRecord={onSelectRecord}
          />
        }
      />
      {pair.candidateTally === undefined ? null : (
        <tbody>
          <tr>
            <th scope="row">
              <Hint text={candidateOrderDescription}>候補の順位</Hint>
            </th>
            <td>
              <KeyValueList pairs={candidateTallyPairs(pair.candidateTally)} />
            </td>
          </tr>
        </tbody>
      )}
      {pair.conditions.map((condition: EdgePairCondition) => (
        <Fragment key={condition.conditionKey}>
          <SideRows
            label={edgePairConditionLabels[condition.conditionKey]}
            description={edgePairConditionDescriptions[condition.conditionKey]}
            left={
              <ConditionSideCell
                conditionKey={condition.conditionKey}
                sideName="left"
                side={pair.left}
                fields={condition.leftValue}
              />
            }
            right={
              <ConditionSideCell
                conditionKey={condition.conditionKey}
                sideName="right"
                side={pair.right}
                fields={condition.rightValue}
              />
            }
          />
          {condition.windowSeconds === undefined ? null : (
            <tbody>
              <tr>
                <th scope="row">
                  <Hint text="終点の時刻 − 始点の時刻">時刻の差</Hint>
                </th>
                <td>
                  <KeyValueList
                    pairs={timeWindowPairs({
                      ...condition,
                      windowSeconds: condition.windowSeconds,
                    })}
                  />
                </td>
              </tr>
            </tbody>
          )}
        </Fragment>
      ))}
    </table>
  );
}

/** 候補の数と、並びでこの候補より上の順位に入る候補の数の組。 */
export function candidateTallyPairs(tally: EdgeCandidateTally): KeyValuePair[] {
  return [
    { name: "候補", value: formatCount(tally.candidateCount) },
    { name: "上位の候補", value: formatCount(tally.precedingCandidateCount) },
  ];
}

/** 許容幅を持つ時刻の条件。 */
export type TimeWindowCondition = EdgePairCondition & { windowSeconds: number };

/** 差を出す小数の桁数。 */
const differenceFractionDigits = 6;

/**
 * 時刻の差の許容幅と、照合が比べた両端の時刻の差の組。差は終点の時刻から始点の時刻を引いた
 * 秒数であり、符号で前後を表す。差を求められないときは、UTC 時刻にできない時刻の印を出す。
 * 秒未満を切り捨てて比べた差の 0 と、小数の桁数で丸めて 0 になる差は、差の上限で書く。
 */
export function timeWindowPairs(
  condition: TimeWindowCondition,
): KeyValuePair[] {
  const window = {
    name: "許容幅",
    value: `±${formatCount(condition.windowSeconds)} 秒`,
  };
  const seconds = condition.differenceSeconds;
  if (seconds === undefined) {
    return [
      window,
      {
        name: "時刻の差",
        value: <MissingValue description="UTC 時刻にできない時刻" />,
      },
    ];
  }
  const wholeSeconds = condition.differenceInWholeSeconds === true;
  const magnitude = Number(Math.abs(seconds).toFixed(differenceFractionDigits));
  const sign = magnitude === 0 ? "" : seconds < 0 ? "−" : "+";
  const text = wholeSeconds
    ? formatCount(magnitude)
    : magnitude.toLocaleString("ja-JP", {
        maximumFractionDigits: differenceFractionDigits,
      });
  const difference =
    wholeSeconds && magnitude === 0
      ? "1 秒未満"
      : magnitude === 0 && seconds !== 0
        ? `${(10 ** -differenceFractionDigits).toFixed(differenceFractionDigits)} 秒未満`
        : `${sign}${text} 秒`;
  return [
    window,
    { name: "時刻の差", value: difference },
    {
      name: "秒未満",
      value:
        condition.differenceInWholeSeconds === true ? "切り捨て" : undefined,
    },
  ];
}

/**
 * 観測の層が推定のエッジを作ったレコードの組を、条件ごとの両端の値と、両端のレコードを
 * Record に表示する値と一緒に出す。組を持たないエッジでは何も出さない。
 */
export function EdgeRecordPairs({
  pairs,
  pairCount,
  onSelectRecord,
}: {
  pairs: EdgeRecordPair[] | undefined;
  pairCount: number | undefined;
  onSelectRecord: SelectRecord;
}) {
  if (pairs === undefined || pairCount === undefined || pairCount === 0) {
    return null;
  }
  return (
    <>
      <h3>エッジを作ったレコードの組</h3>
      <KeyValueList
        pairs={[
          { name: "組数", value: formatCount(pairCount) },
          {
            name: "受け取った組",
            value:
              pairs.length < pairCount ? (
                <Hint text="組の両端のレコード: 根拠のレコードの表にすべてあり">
                  {formatCount(pairs.length)}
                </Hint>
              ) : undefined,
          },
        ]}
      />
      <Fold
        summary="レコードの組"
        summaryNote={`組数: ${formatCount(pairCount)}`}
        rowCount={pairs.length}
        openLimit={openPairLimit}
      >
        <PairPages pairs={pairs} onSelectRecord={onSelectRecord} />
      </Fold>
    </>
  );
}

/**
 * 組を先頭から pairPageSize 組ずつ描き、続きを表示する操作を置く。描いた数はセクションを
 * 閉じると初めに戻る。
 */
function PairPages({
  pairs,
  onSelectRecord,
}: {
  pairs: EdgeRecordPair[];
  onSelectRecord: SelectRecord;
}) {
  const [shown, setShown] = useState(pairPageSize);
  return (
    <>
      {pairs.slice(0, shown).map((pair, index) => (
        <PairTable
          // 組の並びは応答が決め、同じ応答の中で変わらない。
          // biome-ignore lint/suspicious/noArrayIndexKey: 組は識別子を持たない。
          key={index}
          pair={pair}
          index={index}
          onSelectRecord={onSelectRecord}
        />
      ))}
      <ShowMoreRows
        shown={Math.min(shown, pairs.length)}
        total={pairs.length}
        showMore={() => setShown((current) => current + pairPageSize)}
      />
    </>
  );
}
