import { useMemo, useState } from "react";
import type { NodeRef } from "@/shared/api/graph";
import type { RecordLocator } from "@/shared/contracts/common";
import type {
  SigmaMatchedRule,
  SigmaRevisionSource,
  SigmaRuleCandidatesResponse,
  SigmaRuleMatch,
  SigmaRuleSet,
  SigmaUnevaluatedReason,
  SigmaUnevaluatedRecordGroup,
} from "@/shared/contracts/sigmaRuleCandidates";
import type { FetchState } from "@/shared/lib/fetchState";
import { formatCount } from "@/shared/lib/format";
import { toVisibleRawText } from "@/shared/lib/rawText";
import {
  describeRecordLocation,
  recordRefKey,
} from "@/shared/lib/recordPosition";
import { DataTable, type DataTableColumn } from "@/shared/ui/DataTable";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { Highlighted, PeriodMark } from "@/shared/ui/Highlighted";
import { Hint } from "@/shared/ui/Hint";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { MissingValue } from "@/shared/ui/MissingValue";
import { RawText } from "@/shared/ui/RawText";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import { TimestampOffsetNote } from "@/shared/ui/TimestampOffsetNote";
import { nodeRefOf } from "./InvestigationOrderPane";
import { LazyDetails } from "./LazyDetails";
import { NodeLabelValue, nodeLabelName } from "./NodeLabelView";
import { ShowMoreRows } from "./ShownRows";
import { readNodeLabel } from "./subgraph";
import { useSigmaRuleCandidates } from "./useSigmaRuleCandidates";
import type { SubgraphCriteria } from "./useSubgraph";

/** 一致したルールとレコードから Graph と Record へ移る操作。 */
export type SigmaRuleActions = {
  onSelectRecord: (locator: RecordLocator) => void;
  /** 一致したレコードのノードを選ぶ。グラフに無いノードは、そのノードを起点に関係先を出す。 */
  onSelectNode: (node: NodeRef) => void;
  /** Graph で強調しているルールの path。強調していないときは undefined。 */
  highlightedRulePath: string | undefined;
  /** ルールの一致のノードとエッジを Graph で強調する。undefined は強調を解除する。 */
  onHighlightRule: (rule: SigmaMatchedRule | undefined) => void;
};

type SigmaRuleCandidateListProps = {
  state: FetchState<SigmaRuleCandidatesResponse>;
  actions: SigmaRuleActions;
};

/** 一致したレコードを 1 回に出す件数。 */
const matchPageSize = 50;

const revisionSourceLabels: Record<SigmaRevisionSource, string> = {
  git_head: "ルールの directory の git の HEAD",
  argument: "起動の引数",
  unverified: "未確認",
};

/** commit の出どころの補足。出どころの tooltip に出す。 */
const revisionSourceDetails: Partial<Record<SigmaRevisionSource, string>> = {
  argument: "git の HEAD との照合: なし",
};

const reasonLabels: Record<SigmaUnevaluatedReason, string> = {
  yaml_unreadable: "YAML として解析できない",
  not_a_detection_rule: "検出のルールでない",
  rule_collection: "1 つの file に複数のルール",
  logsource_unsupported: "未対応の logsource",
  keyword_search: "フィールド名の無い値の検索",
  modifier_unsupported: "未対応の修飾子",
  value_unsupported: "未対応の値の形",
  regex_unsupported: "Go の正規表現として解釈できない式",
  condition_unsupported: "未対応の condition",
  field_unavailable: "logsource のイベントに無いフィールドの参照",
};

/** file の値。file に値が無い項目は、無い印を出す。 */
function FileValue({ value, item }: { value: string; item: string }) {
  return value === "" ? (
    <MissingValue description={`file に ${item} なし`} />
  ) : (
    <RawText text={value} />
  );
}

/** 検索の条件で Sigma のルールの候補を取得して一覧にする。 */
export function SigmaRuleCandidatePane({
  criteria,
  version,
  ...actions
}: {
  criteria: SubgraphCriteria;
  version: number;
} & SigmaRuleActions) {
  const state = useSigmaRuleCandidates(criteria, version);
  return <SigmaRuleCandidateList state={state} actions={actions} />;
}

/** Sigma のルールに一致したレコードを候補として示し、評価しなかったルールを理由とともに示す。 */
export function SigmaRuleCandidateList({
  state,
  actions,
}: SigmaRuleCandidateListProps) {
  return (
    <section aria-labelledby="sigma-candidates-heading">
      <div className="flex items-center gap-2">
        <h3 id="sigma-candidates-heading">Sigma ルールの候補</h3>
        <StatusLabel
          status="idle"
          label="候補"
          details="未確定: イベントの発生とエッジ"
        />
        <Hint text="検索の条件のうち、レコードのフィルタと文字列の条件を通るレコードの一致を表示する">
          <span className="text-xs text-muted">検索の条件を適用</span>
        </Hint>
      </div>
      <FetchStateView
        state={state}
        loadingDescription="Sigma ルールの候補の読み込み中"
      >
        {(response) =>
          response.ruleSet === undefined ? (
            <p role="status">
              <StatusLabel
                status="idle"
                label="Sigma ルールなし"
                details="次の操作: 起動時に --sigma-rules を指定"
              />
            </p>
          ) : (
            <SigmaRuleCandidates
              response={response}
              ruleSet={response.ruleSet}
              actions={actions}
            />
          )
        }
      </FetchStateView>
    </section>
  );
}

function SigmaRuleCandidates({
  response,
  ruleSet,
  actions,
}: {
  response: SigmaRuleCandidatesResponse;
  ruleSet: SigmaRuleSet;
  actions: SigmaRuleActions;
}) {
  const matchesByRule = useMemo(() => {
    const grouped = new Map<string, SigmaRuleMatch[]>();
    for (const match of response.matches) {
      const list = grouped.get(match.rulePath) ?? [];
      list.push(match);
      grouped.set(match.rulePath, list);
    }
    return grouped;
  }, [response.matches]);
  const outsideCount = response.unevaluatedRecordGroups.reduce(
    (sum, group) => sum + group.recordCount,
    0,
  );
  return (
    <>
      <section aria-label="ルールの集合">
        <KeyValueList
          stacked
          pairs={[
            {
              name: "ルールの directory",
              value: <RawText text={ruleSet.directory} />,
            },
            {
              name: "commit",
              value:
                ruleSet.revision === undefined ? undefined : (
                  <code>
                    <RawText text={ruleSet.revision} />
                  </code>
                ),
            },
            {
              name: "commit の出どころ",
              value:
                revisionSourceDetails[ruleSet.revisionSource] === undefined ? (
                  revisionSourceLabels[ruleSet.revisionSource]
                ) : (
                  <Hint
                    text={revisionSourceDetails[ruleSet.revisionSource] ?? ""}
                  >
                    {revisionSourceLabels[ruleSet.revisionSource]}
                  </Hint>
                ),
            },
            {
              name: "未確認の理由",
              value:
                ruleSet.revisionDetail === undefined ? undefined : (
                  <RawText text={ruleSet.revisionDetail} />
                ),
            },
            {
              name: "git の作業ツリー",
              value:
                ruleSet.gitWorkTree === undefined ? undefined : (
                  <RawText text={ruleSet.gitWorkTree} />
                ),
            },
            { name: "SHA-256", value: <code>{ruleSet.contentSha256}</code> },
            {
              name: "ルールの file",
              value: formatCount(ruleSet.ruleFileCount),
            },
          ]}
        />
      </section>
      <section aria-label="評価の件数">
        <KeyValueList
          stacked
          pairs={[
            {
              name: "評価したルール",
              value:
                response.evaluatedRecordCount === 0
                  ? undefined
                  : formatCount(response.evaluatedRuleCount),
            },
            {
              name: "対象の Windows イベントログのレコード",
              value: formatCount(response.evaluatedRecordCount),
            },
            {
              name: "一致したルール",
              value:
                response.evaluatedRecordCount === 0
                  ? undefined
                  : formatCount(response.rules.length),
            },
            {
              name: "一致したレコードの延べ数",
              value:
                response.evaluatedRecordCount === 0
                  ? undefined
                  : formatCount(response.matches.length),
            },
            {
              name: "グラフに無いレコードの一致",
              value:
                response.outsideGraphMatchCount === 0 ? undefined : (
                  <Hint text="検索の条件を判定するレコードのノードが無いため、表示から除いた一致">
                    {formatCount(response.outsideGraphMatchCount)}
                  </Hint>
                ),
            },
            {
              name: "評価しなかったルール",
              value: formatCount(response.unevaluatedRules.length),
            },
            {
              name: "フィールドが無く適用しなかったレコード",
              value:
                response.skippedPairCount === 0 ? undefined : (
                  <Hint
                    text={`ルールとレコードの組: ${formatCount(response.skippedPairCount)}`}
                  >
                    {formatCount(response.skippedPairRecordCount)}
                  </Hint>
                ),
            },
            {
              name: "意味を解釈できなかったレコード",
              value:
                response.recordsWithoutSemantics === 0
                  ? undefined
                  : formatCount(response.recordsWithoutSemantics),
            },
            {
              name: "logsource に該当しないレコード",
              value: outsideCount === 0 ? undefined : formatCount(outsideCount),
            },
          ]}
        />
      </section>
      {outsideCount === 0 ? null : (
        <DataTable
          label="ルールを適用しなかったレコードのチャネルとプロバイダ"
          rows={response.unevaluatedRecordGroups}
          rowKey={recordGroupKey}
          columns={[
            {
              key: "group",
              header: "チャネルかプロバイダ",
              cell: (group) => <RecordGroupLabel group={group} />,
            },
            {
              key: "count",
              header: "レコード数",
              numeric: true,
              cell: (group) => formatCount(group.recordCount),
            },
          ]}
        />
      )}
      {response.rules.length === 0 ? (
        <p>一致したルールなし</p>
      ) : (
        <ul aria-label="一致したルール">
          {response.rules.map((rule) => (
            <li key={rule.path}>
              <SigmaRuleDetail
                rule={rule}
                matches={matchesByRule.get(rule.path) ?? []}
                actions={actions}
              />
            </li>
          ))}
        </ul>
      )}
      <UnevaluatedRules rules={response.unevaluatedRules} />
    </>
  );
}

/** 組を一覧の中で 1 つに決める鍵を返す。 */
function recordGroupKey(group: SigmaUnevaluatedRecordGroup): string {
  if ("channel" in group) return `channel:${group.channel}`;
  if ("provider" in group) return `provider:${group.provider}`;
  return "absent";
}

/** 原文を示す。空の文字列は空の値の印にする。 */
function ValueText({ text }: { text: string }) {
  return text === "" ? (
    <MissingValue description="空の値" />
  ) : (
    <RawText text={text} />
  );
}

function RecordGroupLabel({ group }: { group: SigmaUnevaluatedRecordGroup }) {
  if ("channel" in group) {
    return (
      <>
        チャネル: <ValueText text={group.channel} />
      </>
    );
  }
  if ("provider" in group) {
    return (
      <Hint text="Channel フィールドの無いレコード">
        プロバイダ: <ValueText text={group.provider} />
      </Hint>
    );
  }
  return <>Channel と Provider のフィールドなし</>;
}

function SigmaRuleDetail({
  rule,
  matches,
  actions,
}: {
  rule: SigmaMatchedRule;
  matches: SigmaRuleMatch[];
  actions: SigmaRuleActions;
}) {
  const title = rule.title === "" ? rule.path : rule.title;
  return (
    <LazyDetails
      summary={
        <>
          <RawText text={title} />
          {rule.level === "" ? null : (
            <span className="ml-1.5 text-xs text-muted">
              <RawText text={rule.level} />
            </span>
          )}
          <span className="ml-1.5 text-xs text-muted">
            {`一致: ${formatCount(rule.matchCount)}`}
          </span>
        </>
      }
    >
      {() => <SigmaRuleBody rule={rule} matches={matches} actions={actions} />}
    </LazyDetails>
  );
}

function SigmaRuleBody({
  rule,
  matches,
  actions,
}: {
  rule: SigmaMatchedRule;
  matches: SigmaRuleMatch[];
  actions: SigmaRuleActions;
}) {
  const [shown, setShown] = useState(matchPageSize);
  const matchedNames = new Set(
    matches.flatMap((match) => match.matchedSelections),
  );
  const highlighted = actions.highlightedRulePath === rule.path;
  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          aria-pressed={highlighted}
          disabled={rule.nodeIds.length === 0}
          onClick={() =>
            actions.onHighlightRule(highlighted ? undefined : rule)
          }
        >
          {highlighted ? "Graph の強調を解除" : "Graph で強調"}
        </button>
        <span className="text-xs text-muted">
          {`ノード: ${formatCount(rule.nodeIds.length)} · エッジ: ${formatCount(rule.edgeIds.length)}`}
        </span>
      </div>
      <section aria-label="ルール">
        <KeyValueList
          stacked
          pairs={[
            { name: "ID", value: <FileValue value={rule.id} item="id" /> },
            {
              name: "著者",
              value: <FileValue value={rule.author} item="author" />,
            },
            { name: "ルールの file", value: <RawText text={rule.path} /> },
            {
              name: "condition",
              value: (
                <code>
                  <RawText text={rule.condition} />
                </code>
              ),
            },
          ]}
        />
      </section>
      <h4>
        <Hint text="ルールの file の定義を YAML に書き直した値 · 字下げ・引用符・コメントの違いあり">
          検索の定義
        </Hint>
      </h4>
      {rule.selections.map((selection) => (
        <section key={selection.name} aria-label={`検索 ${selection.name}`}>
          <h5 className="flex items-center gap-2">
            <RawText text={selection.name} />
            {matchedNames.has(selection.name) ? (
              <StatusLabel status="done" label="一致" />
            ) : (
              <StatusLabel status="idle" label="不一致" />
            )}
          </h5>
          {/* 定義は YAML の複数行である。行の区切りだけを改行で描き、行の中の制御文字は符号にする。 */}
          <pre>
            {selection.definition
              .replace(/\n$/, "")
              .split("\n")
              .map(toVisibleRawText)
              .join("\n")}
          </pre>
        </section>
      ))}
      <DataTable
        label="一致したレコード"
        rows={matches.slice(0, shown)}
        rowKey={(match) => recordRefKey(match.record)}
        columns={matchColumns(actions.onSelectRecord, actions.onSelectNode)}
      />
      <ShowMoreRows
        shown={Math.min(shown, matches.length)}
        total={matches.length}
        showMore={() => setShown((count) => count + matchPageSize)}
      />
    </>
  );
}

/**
 * 一致したレコードの表の列。候補どうしの端末と時刻を列で比べる。レコードを押すとレコードを開き、
 * ノードを押すとノードを選ぶ。
 */
function matchColumns(
  onSelectRecord: (record: RecordLocator) => void,
  onSelectNode: (node: NodeRef) => void,
): DataTableColumn<SigmaRuleMatch>[] {
  return [
    {
      key: "record",
      header: "レコード",
      cell: (match) => (
        <button
          type="button"
          className="value-link"
          onClick={() => onSelectRecord(match.record)}
        >
          <RawText text={describeRecordLocation(match.record)} />
        </button>
      ),
    },
    {
      key: "node",
      header: "ノード",
      cell: (match) => {
        const recordNode = match.recordNode;
        const label =
          recordNode === undefined
            ? undefined
            : readNodeLabel(recordNode.label);
        return recordNode === undefined || label === undefined ? (
          <MissingValue description="グラフにノードなし" />
        ) : (
          <button
            type="button"
            className="value-link"
            aria-label={`ノード ${nodeLabelName(label)} を選ぶ`}
            onClick={() => onSelectNode(nodeRefOf(recordNode))}
          >
            <NodeLabelValue label={label} />
          </button>
        );
      },
    },
    {
      key: "selections",
      header: "一致した検索",
      cell: (match) => <RawText text={match.matchedSelections.join(", ")} />,
    },
    {
      key: "terminal",
      header: "端末",
      cell: (match) =>
        match.terminal === undefined ? (
          <MissingValue description="端末の記録なし" />
        ) : match.terminalAssigned === true ? (
          <Hint text="評価の時に収集元に割り当てた端末">
            <Highlighted text={match.terminal} />
          </Hint>
        ) : (
          <Highlighted text={match.terminal} />
        ),
    },
    {
      key: "time",
      header: "時刻",
      mono: true,
      cell: (match) =>
        match.eventTime === undefined ? (
          <MissingValue description="時刻なし" />
        ) : (
          <>
            {match.eventTime.rawText === undefined ? (
              <MissingValue description="原文なし" />
            ) : (
              <PeriodMark timestamp={match.eventTime}>
                <RawText text={match.eventTime.rawText} />
              </PeriodMark>
            )}
            {/* タイムゾーンの出どころと、表示のタイムゾーンの時刻を次の行に添える。 */}
            <TimestampOffsetNote timestamp={match.eventTime} />
          </>
        ),
    },
  ];
}

function UnevaluatedRules({
  rules,
}: {
  rules: SigmaRuleCandidatesResponse["unevaluatedRules"];
}) {
  return (
    <LazyDetails
      summary={
        <>
          評価しなかったルール{" "}
          <span className="tab-count">{formatCount(rules.length)}</span>
        </>
      }
    >
      {() => <UnevaluatedRulesBody rules={rules} />}
    </LazyDetails>
  );
}

function UnevaluatedRulesBody({
  rules,
}: {
  rules: SigmaRuleCandidatesResponse["unevaluatedRules"];
}) {
  if (rules.length === 0) {
    return <p>評価しなかったルールなし</p>;
  }
  const counts = new Map<SigmaUnevaluatedReason, number>();
  for (const rule of rules) {
    counts.set(rule.reason, (counts.get(rule.reason) ?? 0) + 1);
  }
  const ordered = [...counts.entries()].sort((a, b) => b[1] - a[1]);
  return (
    <>
      <DataTable
        label="評価しなかった理由ごとの件数"
        rows={ordered}
        rowKey={([reason]) => reason}
        columns={[
          {
            key: "reason",
            header: "理由",
            cell: ([reason]) => reasonLabels[reason],
          },
          {
            key: "count",
            header: "ルール数",
            numeric: true,
            cell: ([, count]) => formatCount(count),
          },
        ]}
      />
      <DataTable
        label="評価しなかったルールの一覧"
        rows={rules}
        rowKey={(rule) => rule.path}
        columns={[
          {
            key: "path",
            header: "ルールの file",
            cell: (rule) => <RawText text={rule.path} />,
          },
          {
            key: "id",
            header: "ID",
            cell: (rule) => <FileValue value={rule.id} item="id" />,
          },
          {
            key: "title",
            header: "名前",
            cell: (rule) => <FileValue value={rule.title} item="title" />,
          },
          {
            key: "reason",
            header: "理由",
            cell: (rule) => reasonLabels[rule.reason],
          },
          {
            key: "detail",
            header: "詳細",
            cell: (rule) => <RawText text={rule.detail} />,
          },
        ]}
      />
    </>
  );
}
