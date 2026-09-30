import { Filter, Undo2 } from "lucide-react";
import type { NodeRef } from "@/shared/api/graph";
import type {
  AssistMatchCondition,
  AssistOrigin,
} from "@/shared/contracts/assistRelay";
import type { SearchQuery } from "@/shared/contracts/searchQuery";
import { conditionKeyLabels } from "@/shared/lib/conditionLabels";
import type { FetchState } from "@/shared/lib/fetchState";
import { formatCount } from "@/shared/lib/format";
import { nodeKindLabels } from "@/shared/lib/graphLabels";
import {
  genericEventKindLabels,
  graphChipsOf,
  type SearchChip,
  searchChipsOf,
} from "@/shared/lib/searchChips";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { RawText } from "@/shared/ui/RawText";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import { AssistText } from "./AssistText";

/** 関連付けの条件の 2 つの選択が、同じ条件と幅の組であるか。 */
export function sameMatchConditions(
  left: readonly AssistMatchCondition[],
  right: readonly AssistMatchCondition[],
): boolean {
  const keyOf = (condition: AssistMatchCondition) =>
    `${condition.conditionKey}~${condition.tolerance}`;
  const leftKeys = new Set(left.map(keyOf));
  const rightKeys = new Set(right.map(keyOf));
  return (
    leftKeys.size === rightKeys.size &&
    [...leftKeys].every((key) => rightKeys.has(key))
  );
}

/** 図に出す対象の選び方を文字列にする。 */
function describeViewItems(query: SearchQuery): string {
  const kinds = query.nodeKinds ?? [];
  if (kinds.length > 0) {
    return kinds.map((kind) => nodeKindLabels[kind]).join("、");
  }
  return query.granularity === undefined
    ? "検索の条件から決定"
    : "レコード以外のすべて";
}

/** card の端末が、読み込んだ端末の一覧に無いか。一覧を読み込む前は無いと判定しない。 */
function isUnlistedTerminal(
  query: SearchQuery,
  terminals: FetchState<NodeRef[]>,
): boolean {
  return (
    query.terminal !== undefined &&
    terminals.status === "loaded" &&
    !terminals.value.some((candidate) => candidate.id === query.terminal)
  );
}

/** card の条件を、検索欄の chip と同じ手順で組む。 */
function cardChipsOf(
  query: SearchQuery,
  origins: readonly AssistOrigin[] | undefined,
  terminals: FetchState<NodeRef[]>,
): SearchChip<string>[] {
  // 端末の一覧に無い端末は識別子を出し、card に印を付ける。適用するかは分析者が決め、server が判定する。
  const listed =
    terminals.status === "loaded"
      ? terminals.value.find((candidate) => candidate.id === query.terminal)
      : undefined;
  const terminal =
    query.terminal === undefined
      ? undefined
      : (listed?.label ?? query.terminal);
  const chips: SearchChip<string>[] = [
    ...searchChipsOf(
      {
        contains: query.valueContains ?? [],
        excludes: query.valueExcludes ?? [],
        field: query.valueField,
        fieldContains: query.fieldContains,
        fieldEquals: query.fieldEquals,
        expression: query.searchExpression,
        terminal,
        // card は収集元の表示名を持たないため、sourceId を出す。
        sources: query.sources?.map((id) => ({ id, label: id })),
        eventCategory: query.eventCategory,
        eventAction: query.eventAction,
        eventActionFrom: query.eventActionFrom,
        eventActionTo: query.eventActionTo,
      },
      genericEventKindLabels,
    ),
  ];
  if (query.timeFrom !== undefined || query.timeTo !== undefined) {
    chips.push({
      key: "period",
      kind: "period",
      label: "期間",
      value: `${query.timeFrom ?? ""} – ${query.timeTo ?? ""}`,
    });
  }
  if (query.case !== undefined) {
    chips.push({
      key: "case",
      kind: "case",
      label: "案件",
      value: query.case,
    });
  }
  chips.push(
    ...graphChipsOf({
      depth: query.depth,
      origins,
      edgeKinds: query.edgeKinds,
      addressInCidr: query.addressInCidr,
      addressNotInCidr: query.addressNotInCidr,
      countBy: query.countBy,
      conditionsOnOriginsOnly: query.conditionsOnOriginsOnly,
      endpointRecordsInPeriod: query.endpointRecordsInPeriod,
    }),
    {
      key: "view",
      kind: "view",
      label: "グラフに表示する対象",
      value: describeViewItems(query),
    },
  );
  return chips;
}

type SearchQueryCardProps = {
  query: SearchQuery;
  /** 条件の `nodeIds` の種別と表示名。 */
  origins?: readonly AssistOrigin[];
  /** LLM が添えた説明。 */
  explanation?: string;
  /** card を作った発言の関連付けの条件の選択。 */
  matchConditions: readonly AssistMatchCondition[];
  /** 今の画面の関連付けの条件の選択。 */
  currentMatchConditions: readonly AssistMatchCondition[];
  terminals: FetchState<NodeRef[]>;
  /** 適用した後に、元の条件に戻す操作が使える。 */
  restorable: boolean;
  onApply: () => void;
  onRestore: () => void;
};

/**
 * LLM が画面に出した検索の条件を、検索欄の chip と同じ形で出す。適用するかは分析者が決める。
 * **LLM の説明は「AI の説明」と印を付けた別の欄に出す。**
 */
export function SearchQueryCard({
  query,
  origins,
  explanation,
  matchConditions,
  currentMatchConditions,
  terminals,
  restorable,
  onApply,
  onRestore,
}: SearchQueryCardProps) {
  const chips = cardChipsOf(query, origins, terminals);
  const conditionKeys = (conditions: readonly AssistMatchCondition[]) =>
    conditions.length === 0
      ? "なし"
      : conditions
          .map(
            (condition) =>
              `${conditionKeyLabels[condition.conditionKey]} 許容幅: ${formatCount(condition.tolerance)} 秒`,
          )
          .join("、");
  const marks = [
    ...(sameMatchConditions(matchConditions, currentMatchConditions)
      ? []
      : [
          <StatusLabel
            key="conditions"
            status="pending"
            label="推定条件の不一致"
            details={
              <KeyValueList
                stacked
                pairs={[
                  { name: "card", value: conditionKeys(matchConditions) },
                  { name: "今", value: conditionKeys(currentMatchConditions) },
                ]}
              />
            }
          />,
        ]),
    ...(isUnlistedTerminal(query, terminals)
      ? [<StatusLabel key="terminal" status="pending" label="一覧に無い端末" />]
      : []),
  ];
  return (
    <section className="search-query-card" aria-label="AI が勧めた検索の条件">
      <p className="card-title">AI が勧めた検索の条件</p>
      <ul aria-label="勧めた検索の条件" className="condition-chips card-chips">
        {chips.map((chip) => (
          <li
            key={chip.key}
            className={chip.kind === "excludes" ? "is-excluded" : ""}
          >
            <span className="chip-label">
              {chip.label}
              <span className="visually-hidden">: </span>
            </span>
            <span className="chip-value">
              <RawText text={chip.value} />
            </span>
          </li>
        ))}
      </ul>
      {explanation === undefined || explanation === "" ? null : (
        <div className="ai-explanation">
          <p className="ai-mark">AI の説明</p>
          <AssistText text={explanation} />
        </div>
      )}
      {marks.length === 0 ? null : (
        <div role="note" className="flex flex-wrap gap-2">
          {marks}
        </div>
      )}
      <ApplyActions
        restorable={restorable}
        onApply={onApply}
        onRestore={onRestore}
      />
    </section>
  );
}

/** 検索の条件に適用する操作と、適用する前の条件へ戻す操作。 */
export function ApplyActions({
  restorable,
  onApply,
  onRestore,
}: Pick<SearchQueryCardProps, "restorable" | "onApply" | "onRestore">) {
  return (
    <div className="card-actions">
      <IconButton label="検索の条件に適用" onPress={onApply}>
        <Filter size={14} aria-hidden="true" />
      </IconButton>
      <IconButton
        label="適用前の条件に復元"
        onPress={onRestore}
        isDisabled={!restorable}
        disabledReason={{ title: "適用前", text: "条件の適用が必要" }}
      >
        <Undo2 size={14} aria-hidden="true" />
      </IconButton>
    </div>
  );
}
