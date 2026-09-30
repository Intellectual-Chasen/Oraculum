import { Focus } from "lucide-react";
import { type ReactNode, useState } from "react";
import type { NodeRef } from "@/shared/api/graph";
import type { GraphNode } from "@/shared/contracts/graph";
import {
  type InvestigationOrderInputs,
  type InvestigationOrderKind,
  type InvestigationOrderResponse,
  type SigmaMinLevel,
  sigmaLevelRankMultiplier,
  sigmaMinLevels,
} from "@/shared/contracts/investigationOrder";
import type { FetchState } from "@/shared/lib/fetchState";
import { formatCount } from "@/shared/lib/format";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import { TimestampText } from "@/shared/ui/TimestampText";
import {
  investigationOrderMethodDescriptions,
  investigationOrderMethodLabels,
  nodeKindLabels,
  type ShownInvestigationOrderMethod,
  shownInvestigationOrderMethods,
} from "./labels";
import { NodeLabelView, nodeLabelName } from "./NodeLabelView";
import { readNodeLabel } from "./subgraph";
import { useInvestigationOrder } from "./useInvestigationOrder";
import type { SubgraphCriteria } from "./useSubgraph";

/**
 * 種別ごとに描く行の数。
 *
 * 既知の制限: 種別ごとに上位 20 行だけを描き、残りは件数だけを出す,
 * 応答は測定のために全件を含み、全件を描くと対象の多い種別 (レコードごとのプロセス) で
 * 数万行の表になる。目安は上から見る並びであり、下位の行を開く操作の要求が出たときに見直す
 */
const shownEntryLimit = 20;

type InvestigationOrderPaneProps = {
  criteria: SubgraphCriteria;
  version: number;
  /** 対象を起点にして関係先だけを出す。グラフに無い対象も起点として出る。 */
  onShowNeighbours: (node: NodeRef) => void;
};

/** 手法を選ばせ、今の検索の条件で調べる順序の目安を取得して出す。 */
export function InvestigationOrderPane({
  criteria,
  version,
  onShowNeighbours,
}: InvestigationOrderPaneProps) {
  const [method, setMethod] = useState<ShownInvestigationOrderMethod>(
    shownInvestigationOrderMethods[0],
  );
  const [sigmaMinLevel, setSigmaMinLevel] = useState<SigmaMinLevel>();
  const state = useInvestigationOrder(criteria, method, sigmaMinLevel, version);
  return (
    <InvestigationOrderList
      state={state}
      method={method}
      onChangeMethod={setMethod}
      sigmaMinLevel={sigmaMinLevel}
      onChangeSigmaMinLevel={setSigmaMinLevel}
      onShowNeighbours={onShowNeighbours}
    />
  );
}

type InvestigationOrderListProps = {
  state: FetchState<InvestigationOrderResponse>;
  method: ShownInvestigationOrderMethod;
  onChangeMethod: (method: ShownInvestigationOrderMethod) => void;
  /** Sigma の手法が一致を数えるルールのレベルの下限。`undefined` は全レベルを数える。 */
  sigmaMinLevel: SigmaMinLevel | undefined;
  onChangeSigmaMinLevel: (level: SigmaMinLevel | undefined) => void;
  onShowNeighbours: (node: NodeRef) => void;
};

/** レベルの下限の選択肢で「すべてのレベル」を表す値。 */
const allSigmaLevels = "all";

/** 応答の係数から、名前の値を取り出す。 */
function parameterOf(
  response: InvestigationOrderResponse,
  name: string,
): string | undefined {
  return response.parameters.find((parameter) => parameter.name === name)
    ?.value;
}

/** Sigma の手法で並べられない理由の状態のラベル。並べられるときは `undefined` である。 */
function sigmaUnavailableLabel(
  response: InvestigationOrderResponse,
): ReactNode | undefined {
  if (parameterOf(response, "rule_set_content_sha256") === undefined) {
    return (
      <StatusLabel
        status="idle"
        label="Sigma ルールなし"
        details="次の操作: 起動時に --sigma-rules を指定"
      />
    );
  }
  if (parameterOf(response, "evaluated_record_count") === "0") {
    return (
      <StatusLabel
        status="idle"
        label="Sigma ルールを適用したレコードなし"
        details="適用先: Windows イベントログのレコード"
      />
    );
  }
  return undefined;
}

/** 調べる順序の範囲。見出しの説明に出す。 */
const orderScopePairs = [
  { name: "順位の単位", value: "同じ種類のノード" },
  { name: "同じ値", value: "同じ順位" },
  {
    name: "計算の範囲",
    value: "レコードのフィルタ・文字列の条件・一致ノード・ホップ数",
  },
  { name: "不使用", value: "グラフの粒度・ノードの種類・エッジの作り方" },
];

/** 調べる順序の目安を、手法の説明と計算に使った入力とともに種類ごとの表で出す。 */
export function InvestigationOrderList({
  state,
  method,
  onChangeMethod,
  sigmaMinLevel,
  onChangeSigmaMinLevel,
  onShowNeighbours,
}: InvestigationOrderListProps) {
  return (
    <section aria-labelledby="investigation-order-heading">
      <h3 id="investigation-order-heading">
        調べる順序
        <HelpPopover label="調べる順序">
          <KeyValueList stacked pairs={orderScopePairs} />
        </HelpPopover>
      </h3>
      <label>
        並べ方{" "}
        <select
          value={method}
          onChange={(event) =>
            onChangeMethod(event.target.value as ShownInvestigationOrderMethod)
          }
        >
          {shownInvestigationOrderMethods.map((candidate) => (
            <option key={candidate} value={candidate}>
              {investigationOrderMethodLabels[candidate]}
            </option>
          ))}
        </select>
      </label>
      {method === "sigma" ? (
        <label>
          {" "}
          数える Sigma ルールのレベル{" "}
          <select
            value={sigmaMinLevel ?? allSigmaLevels}
            onChange={(event) =>
              onChangeSigmaMinLevel(
                event.target.value === allSigmaLevels
                  ? undefined
                  : (event.target.value as SigmaMinLevel),
              )
            }
          >
            <option value={allSigmaLevels}>すべてのレベル</option>
            {sigmaMinLevels.map((level) => (
              <option key={level} value={level}>
                {level} 以上
              </option>
            ))}
          </select>
        </label>
      ) : null}
      <KeyValueList
        pairs={investigationOrderMethodDescriptions[method].map(
          ([name, value]) => ({ name, value }),
        )}
      />
      <FetchStateView state={state} loadingDescription="調べる順序の計算中">
        {(response) => {
          // 値の読み方は応答の手法で決める。手法を切り替えた直後の描画は、前の手法の応答を持つ。
          const sigma = response.method === "sigma";
          const unavailable = sigma
            ? sigmaUnavailableLabel(response)
            : undefined;
          return (
            <>
              <InputsSummary inputs={response.inputs} />
              {response.parameters.length === 0 ? null : (
                <KeyValueList
                  pairs={response.parameters.map((parameter) => ({
                    name: parameter.name,
                    value: parameter.value,
                  }))}
                />
              )}
              {unavailable !== undefined ? (
                <p>{unavailable}</p>
              ) : response.kinds.length === 0 ? (
                <p>該当するノードなし</p>
              ) : (
                response.kinds.map((kind) => (
                  <KindOrderTable
                    key={kind.kind}
                    kind={kind}
                    formatValue={sigma ? formatSigmaValue : formatOrderValue}
                    onShowNeighbours={onShowNeighbours}
                  />
                ))
              )}
            </>
          );
        }}
      </FetchStateView>
    </section>
  );
}

/** 計算に使ったノード・組・レコードの件数と期間を出す。 */
function InputsSummary({ inputs }: { inputs: InvestigationOrderInputs }) {
  return (
    <KeyValueList
      pairs={[
        { name: "ノード", value: formatCount(inputs.objectCount) },
        { name: "ノードの組", value: formatCount(inputs.pairCount) },
        { name: "レコード", value: formatCount(inputs.recordCount) },
        {
          name: "UTC 時刻を持つレコード",
          value: formatCount(inputs.timedRecordCount),
        },
        {
          name: "期間",
          value:
            inputs.timeRange === undefined ? (
              "なし"
            ) : (
              <>
                <TimestampText timestamp={inputs.timeRange.from} />
                {" – "}
                <TimestampText timestamp={inputs.timeRange.to} />
              </>
            ),
        },
      ]}
    />
  );
}

/** 手法の値を出す文字列。整数は桁を区切り、小数は有効数字 6 桁にする。 */
function formatOrderValue(value: number | null): string {
  if (value === null) return "値なし";
  return Number.isInteger(value) ? formatCount(value) : value.toPrecision(6);
}

/** Sigma の手法の値の順位から求めるレベルの表示。定義元は `sigmaLevelRanks` である。 */
const sigmaLevelOfRank = [
  "一致なし",
  "その他のレベル",
  "informational",
  "low",
  "medium",
  "high",
  "critical",
];

/** Sigma の手法の値を、最高レベルと一致したレコードの件数に分けて出す。 */
function formatSigmaValue(value: number | null): string {
  if (value === null) return "値なし";
  const rank = Math.floor(value / sigmaLevelRankMultiplier);
  const count = value % sigmaLevelRankMultiplier;
  if (rank === 0) return sigmaLevelOfRank[0];
  return `${sigmaLevelOfRank[rank] ?? formatCount(rank)} · ${formatCount(count)}`;
}

/** 起点に渡す表示名。原資料の文字列、正規化値、識別子の順に使う。 */
export function nodeRefOf(node: GraphNode): NodeRef {
  return {
    id: node.id,
    label: node.label.rawText ?? node.label.normalized ?? node.id,
    kind: node.kind,
  };
}

function KindOrderTable({
  kind,
  formatValue,
  onShowNeighbours,
}: {
  kind: InvestigationOrderKind;
  formatValue: (value: number | null) => string;
  onShowNeighbours: (node: NodeRef) => void;
}) {
  const kindLabel = nodeKindLabels[kind.kind];
  const hiddenCount = kind.entries.length - shownEntryLimit;
  return (
    <table>
      <caption>
        {kindLabel}{" "}
        <span className="tab-count">{formatCount(kind.entries.length)}</span>
      </caption>
      <thead>
        <tr>
          <th scope="col">順位</th>
          <th scope="col">表示名</th>
          <th scope="col">値</th>
          <th scope="col">同じ順位の数</th>
          <th scope="col">操作</th>
        </tr>
      </thead>
      <tbody>
        {kind.entries.slice(0, shownEntryLimit).map((entry) => {
          const label = readNodeLabel(entry.node.label);
          return (
            <tr key={entry.node.id}>
              <td className="short-cell">{formatCount(entry.rank)}</td>
              <td className="wrapping-cell">
                <NodeLabelView label={label} />
              </td>
              <td className="short-cell">{formatValue(entry.value)}</td>
              <td className="short-cell">{formatCount(entry.tieCount)}</td>
              <td>
                <IconButton
                  label={`${kindLabel} ${nodeLabelName(label)} の隣接ノードだけを表示`}
                  onPress={() => onShowNeighbours(nodeRefOf(entry.node))}
                >
                  <Focus size={14} aria-hidden="true" />
                </IconButton>
              </td>
            </tr>
          );
        })}
      </tbody>
      {hiddenCount > 0 ? (
        <tfoot>
          <tr>
            <td colSpan={5}>
              <KeyValueList
                pairs={[
                  { name: "表示していない行", value: formatCount(hiddenCount) },
                ]}
              />
            </td>
          </tr>
        </tfoot>
      ) : null}
    </table>
  );
}
