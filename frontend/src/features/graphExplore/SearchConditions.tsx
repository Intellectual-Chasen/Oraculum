import {
  maxGraphDepth,
  minGraphDepth,
  type NodeRef,
  type RecordFilterCriteria,
} from "@/shared/api/graph";
import {
  everyMatchCondition,
  type MatchConditionSelection,
} from "@/shared/api/matchConditions";
import type { CaseId } from "@/shared/contracts/cases";
import type { EventKindsResponse } from "@/shared/contracts/eventKinds";
import {
  type EdgeKind,
  edgeKinds,
  type NodeKind,
} from "@/shared/contracts/graph";
import { conditionKeyLabels } from "@/shared/lib/conditionLabels";
import type {
  FetchState,
  SearchExpressionFailure,
} from "@/shared/lib/fetchState";
import { formatCount } from "@/shared/lib/format";
import { toVisibleRawText } from "@/shared/lib/rawText";
import {
  type SearchTerms,
  withExpression,
  withField,
  withFieldTerm,
  withoutExpression,
  withoutField,
  withoutFieldTerm,
  withoutTerm,
  withTerm,
} from "@/shared/lib/searchTerms";
import {
  autoView,
  type ResolvedView,
  type ViewChoice,
} from "@/shared/lib/searchView";
import {
  type ContextMenuContent,
  useContextMenu,
} from "@/shared/ui/ContextMenu";
import { Hint } from "@/shared/ui/Hint";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { type MenuEntry, menuSeparator } from "@/shared/ui/Menu";
import { useCopyText } from "@/shared/ui/useCopyText";
import {
  type Choice,
  type ChoiceKind,
  ConditionInput,
  type ConditionValue,
} from "./ConditionInput";
import {
  edgeKindLabels,
  eventKindFieldLabels,
  filterFieldLabels,
  nodeKindLabels,
  nodeSelectionLabels,
  timeUnitLabels,
} from "./labels";
import {
  SearchExpressionErrorView,
  SearchExpressionSyntax,
} from "./SearchExpressionForm";
import type { SubgraphCriteria } from "./useSubgraph";

/** 図に出す対象として選べる種別と、その並び。 */
const viewKindOrder: readonly NodeKind[] = [
  "terminal",
  "process",
  "ip",
  "domain",
  "file",
  "registry_value",
  "account",
  "record",
];

/** 文字列の条件の一致の規則。「文字列の一致」の popover に出す。 */
const textMatchRules = [
  { name: "判定の単位", value: "1 件のレコードのフィールド" },
  { name: "2 つ以上の文字列", value: "すべてを含むレコード" },
  { name: "大文字と小文字の区別", value: "なし" },
  {
    name: "フィールドの値が等しい",
    value: "値の全体が等しいレコード · 例: EventRecordID",
  },
  { name: "フィールドを指定", value: "含む文字列と含まない文字列の照合先" },
  {
    name: "フィールド名",
    value: "process.command_line · CommandLine · EventData.CommandLine",
  },
  { name: "「.」で繋いだフィールド名", value: "末尾の部分で指定可" },
  {
    name: "Record と Node Detail からの文字列の追加",
    value: "フィールドの指定の解除",
  },
  {
    name: "フィールドの値が文字列を含む",
    value: "すべての組を満たすレコード · フィールドの指定に依らない照合",
  },
];

/** 初めのホップ数。この値のときはホップ数の chip を出さない。 */
const defaultDepth = 1;

/**
 * ホップ数の chip の説明。ノードを指す語は `nodeSelectionLabels` を通す。
 */
function depthDescriptionOf(depth: number): string {
  return `${nodeSelectionLabels.matched}から辿るエッジの本数: ${depth}`;
}

/** ホップ数の候補。値だけを並べる。 */
const depthChoices: readonly Choice[] = Array.from(
  { length: maxGraphDepth - minGraphDepth + 1 },
  (_, offset) => minGraphDepth + offset,
).map((depth) => ({ id: String(depth), label: String(depth) }));

/** 事象の種別の候補 1 つを、候補の識別子へ直す。 */
function eventKindValue(category: string, action?: string): string {
  return JSON.stringify(action === undefined ? [category] : [category, action]);
}

/** 候補の識別子を、事象の分類と動作へ戻す。読めない文字列は undefined を返す。 */
function readEventKindValue(
  value: string,
): { category: string; action?: string } | undefined {
  try {
    const parsed: unknown = JSON.parse(value);
    if (!Array.isArray(parsed) || typeof parsed[0] !== "string") {
      return undefined;
    }
    if (parsed.length === 1) {
      return { category: parsed[0] };
    }
    if (parsed.length === 2 && typeof parsed[1] === "string") {
      return { category: parsed[0], action: parsed[1] };
    }
  } catch {
    // 候補の外の文字列は条件を変えない。
  }
  return undefined;
}

/**
 * 事象の種別の候補を、分類ごとの区分に並べる。区分の先頭に分類のすべてを置く。
 *
 * Windows イベントログの組は、分類がプロバイダ、動作がイベント ID であると書く。
 * プロバイダの名前は長く、狭い列で後ろが切れるため、候補はイベント ID から書く。
 */
function eventKindChoices(
  eventKinds: FetchState<EventKindsResponse>,
): FetchState<readonly Choice[]> {
  if (eventKinds.status !== "loaded") return eventKinds;
  const categories = new Map<
    string,
    { total: number; actions: EventKindsResponse["kinds"][number][] }
  >();
  for (const kind of eventKinds.value.kinds) {
    const entry = categories.get(kind.category) ?? { total: 0, actions: [] };
    entry.total += kind.recordCount;
    entry.actions.push(kind);
    categories.set(kind.category, entry);
  }
  const choices: Choice[] = [];
  for (const [category, entry] of categories) {
    const windowsEvent = entry.actions.every((kind) => kind.windowsEvent);
    const section = windowsEvent ? `プロバイダ: ${category}` : category;
    choices.push({
      id: eventKindValue(category),
      label: `${windowsEvent ? category : section} のすべて`,
      count: entry.total,
      section,
    });
    for (const kind of entry.actions) {
      if (kind.action === undefined) continue;
      choices.push({
        id: eventKindValue(category, kind.action),
        label: windowsEvent
          ? `イベント ID ${kind.action}`
          : `${category} / ${kind.action}`,
        count: kind.recordCount,
        section,
      });
    }
  }
  return { status: "loaded", value: choices };
}

/**
 * 関係の種別の集合に kind を足すか外した集合を返す。並びは `edgeKinds` の順に揃え、
 * 同じ集合で要求の文字列が変わらないようにする。空の集合は `undefined` (すべての関係) にする。
 */
function toggledEdgeKinds(
  current: readonly EdgeKind[] | undefined,
  kind: EdgeKind,
): readonly EdgeKind[] | undefined {
  const chosen = new Set(current);
  if (chosen.has(kind)) {
    chosen.delete(kind);
  } else {
    chosen.add(kind);
  }
  const next = edgeKinds.filter((candidate) => chosen.has(candidate));
  return next.length === 0 ? undefined : next;
}

/** 図に出すノードの種類の選択を、読める文字列にする。 */
export function describeView(resolved: ResolvedView): string {
  return resolved.nodeKinds === undefined
    ? "レコード以外のすべて"
    : resolved.nodeKinds.map((kind) => nodeKindLabels[kind]).join("、");
}

/** 推定条件の選択を、読める文字列にする。 */
function describeMatchConditions(selection: MatchConditionSelection): string {
  return selection.conditions
    .map((condition) =>
      condition.toleranceSeconds === undefined
        ? conditionKeyLabels[condition.conditionKey]
        : `${conditionKeyLabels[condition.conditionKey]} · 許容幅 ${condition.toleranceSeconds} 秒`,
    )
    .join("、");
}

/** 適用している条件 1 つ。表示の文字列と、外す操作を持つ。 */
type AppliedCondition = {
  key: string;
  label: string;
  value: string;
  nodeKind?: NodeKind;
  /** 値を写す操作が clipboard へ渡す文字列。条件が持つ値そのものである。 */
  copyValue: string;
  remove: () => void;
  /** 文字列の条件の向きを替える操作。文字列の条件だけが持つ。 */
  invert?: { label: string; apply: () => void };
  /** 値の意味の説明。chip には出さず、pointer を重ねたときに出す。 */
  description?: string;
};

/**
 * 条件の chip のコンテキストメニューの項目を組む。外す、文字列の向きを替える、値を写す。
 * 条件が外れた後の描画では、項目を持たないメニューを返す。
 */
function conditionMenuContent(
  condition: AppliedCondition | undefined,
  copy: (text: string, what: string) => void,
): ContextMenuContent {
  if (condition === undefined) {
    return { label: "削除した条件", entries: [] };
  }
  const entries: MenuEntry[] = [
    {
      kind: "item",
      key: "remove",
      label: "条件を削除",
      onSelect: condition.remove,
    },
  ];
  if (condition.invert !== undefined) {
    entries.push({
      kind: "item",
      key: "invert",
      label: condition.invert.label,
      onSelect: condition.invert.apply,
    });
  }
  entries.push(menuSeparator("copy"), {
    kind: "item",
    key: "copy-value",
    label: "値をコピー",
    onSelect: () => copy(condition.copyValue, `${condition.label}の値`),
  });
  return {
    label: `${condition.label} ${toVisibleRawText(condition.value)} の操作`,
    entries,
  };
}

type SearchConditionsProps = {
  terms: SearchTerms;
  onChangeTerms: (terms: SearchTerms) => void;
  /**
   * 適用している検索式を、グラフの取得が読めなかった誤り。出ない場合は誤りを出さない。
   * 上位の画面がグラフの取得の失敗から渡す。
   */
  expressionError?: SearchExpressionFailure | undefined;
  recordFilter: RecordFilterCriteria;
  onApplyRecordFilter: (filter: RecordFilterCriteria) => void;
  terminals: FetchState<NodeRef[]>;
  /** 収集元の sourceId から file 名を探す表。収集元の候補である。 */
  sourceFileNames: ReadonlyMap<string, string>;
  /** 事象の種別の候補。端末と案件で絞った範囲のレコードが持つ組である。 */
  eventKinds: FetchState<EventKindsResponse>;
  view: ViewChoice;
  resolvedView: ResolvedView;
  onChangeView: (view: ViewChoice) => void;
  /** 部分グラフのホップ数・関係の種別・件数を集計する欄・アドレスの範囲ほか。 */
  criteria: SubgraphCriteria;
  onApplyCriteria: (criteria: SubgraphCriteria) => void;
  /** 関係の種別を選び直す。関係先の探索は関係の種別で絞るため、他の条件と分けて渡す。 */
  onApplyEdgeKinds: (edgeKinds: readonly EdgeKind[] | undefined) => void;
  /** 候補を絞るのに用いる条件の選択。時系列と詳細も同じ値を読むため、上位の画面が持つ。 */
  matchConditions: MatchConditionSelection;
  onApplyMatchConditions: (selection: MatchConditionSelection) => void;
  /** 取り込んだ収集元に付いた案件の識別子。要素数 0 のときは案件を種類の一覧に出さない。 */
  caseIds: readonly CaseId[];
  /** 同じアカウントのノードを図と表で 1 つにまとめるか。要求の条件に入れない。 */
  mergeSameAccount: boolean;
  onChangeMergeSameAccount: (merge: boolean) => void;
};

/** 同じアカウントとしてまとめる切り替えの補足。行ごとに「名前: 値」の組を置く。 */
const mergeSameAccountHint = [
  "対象: 同じ案件の、同じドメインとログイン名のアカウントのノード",
  "まとめない SID: 記録した名前が 2 つ以上",
  "適用先: Graph・Nodes・Edges",
  "対象外: Path",
].join("\n");

/**
 * 検索の条件を chip として足し、外す。条件の値は上位の画面が持ち、本 component は
 * 入力欄の操作を、条件の値を置き換える関数の呼び出しへ振り分ける。
 *
 * **条件は足した時点で適用する。** 分析者は図を見ながら条件を 1 つずつ足して次の検索を決める。
 * 書きかけの文字列は、足す操作まで要求へ載せない。
 */
export function SearchConditions({
  terms,
  onChangeTerms,
  expressionError,
  recordFilter,
  onApplyRecordFilter,
  terminals,
  sourceFileNames,
  eventKinds,
  view,
  resolvedView,
  onChangeView,
  criteria,
  onApplyCriteria,
  onApplyEdgeKinds,
  matchConditions,
  onApplyMatchConditions,
  caseIds,
  mergeSameAccount,
  onChangeMergeSameAccount,
}: SearchConditionsProps) {
  const chosenSources = recordFilter.sources ?? [];
  const manualKinds = view.kind === "manual" ? view.nodeKinds : [];

  // **文字列の向きは withTerm で反対の向きに足して替える。** 反対の向きに足すと、元の向きの
  // 同じ文字列は外れる。
  const applied: AppliedCondition[] = [
    ...terms.contains.map((token) => ({
      key: `contains:${token}`,
      label: filterFieldLabels.valueContains,
      value: token,
      copyValue: token,
      remove: () => onChangeTerms(withoutTerm(terms, "contains", token)),
      invert: {
        label: "含まない文字列に変更",
        apply: () => onChangeTerms(withTerm(terms, "excludes", token)),
      },
    })),
    ...terms.excludes.map((token) => ({
      key: `excludes:${token}`,
      label: filterFieldLabels.valueExcludes,
      value: token,
      copyValue: token,
      remove: () => onChangeTerms(withoutTerm(terms, "excludes", token)),
      invert: {
        label: "含む文字列に変更",
        apply: () => onChangeTerms(withTerm(terms, "contains", token)),
      },
    })),
  ];
  const push = (
    key: string,
    label: string,
    value: string,
    remove: () => void,
    nodeKind?: NodeKind,
  ) => applied.push({ key, label, value, copyValue: value, remove, nodeKind });

  if (terms.field !== undefined) {
    const field = terms.field;
    applied.push({
      key: "field",
      label: "文字列を探すフィールド",
      value: field,
      copyValue: field,
      remove: () => onChangeTerms(withoutField(terms)),
      description:
        terms.contains.length + terms.excludes.length === 0
          ? "照合する文字列なし"
          : undefined,
    });
  }
  for (const pair of terms.fieldContains ?? []) {
    push(`fieldContains:${pair}`, filterFieldLabels.fieldContains, pair, () =>
      onChangeTerms(withoutFieldTerm(terms, pair)),
    );
  }
  for (const pair of terms.fieldEquals ?? []) {
    push(`fieldEquals:${pair}`, filterFieldLabels.fieldEquals, pair, () =>
      onChangeTerms(withoutFieldTerm(terms, pair, true)),
    );
  }
  if (terms.expression !== undefined) {
    push(
      "expression",
      filterFieldLabels.searchExpression,
      terms.expression,
      () => onChangeTerms(withoutExpression(terms)),
    );
  }
  if (recordFilter.terminal !== undefined) {
    push(
      "terminal",
      filterFieldLabels.terminal,
      recordFilter.terminal.label,
      () => onApplyRecordFilter({ ...recordFilter, terminal: undefined }),
      "terminal",
    );
  }
  for (const source of chosenSources) {
    push(`source:${source.id}`, filterFieldLabels.sources, source.label, () =>
      onApplyRecordFilter({
        ...recordFilter,
        sources: chosenSources.filter((kept) => kept.id !== source.id),
      }),
    );
  }
  if (recordFilter.eventCategory !== undefined) {
    // 分類と動作は 1 つの chip にする。動作だけを外す操作は、分類のすべてを選び直して行う。
    const labels = eventKindFieldLabels(
      eventKinds.status === "loaded" ? eventKinds.value.kinds : [],
      recordFilter.eventCategory,
      recordFilter.eventAction,
    );
    push(
      "eventKind",
      recordFilter.eventAction === undefined
        ? labels.eventCategory
        : `${labels.eventCategory} / ${labels.eventAction}`,
      recordFilter.eventAction === undefined
        ? recordFilter.eventCategory
        : `${recordFilter.eventCategory} / ${recordFilter.eventAction}`,
      () =>
        onApplyRecordFilter({
          ...recordFilter,
          eventCategory: undefined,
          eventAction: undefined,
        }),
    );
  }
  if (
    recordFilter.eventActionFrom !== undefined ||
    recordFilter.eventActionTo !== undefined
  ) {
    push(
      "eventActionRange",
      "イベント ID の範囲",
      `${recordFilter.eventActionFrom ?? ""}–${recordFilter.eventActionTo ?? ""}`,
      () =>
        onApplyRecordFilter({
          ...recordFilter,
          eventActionFrom: undefined,
          eventActionTo: undefined,
        }),
    );
  }
  if (recordFilter.timeFilter !== undefined) {
    const { from, to, unit } = recordFilter.timeFilter;
    const range = `${from?.text ?? ""} – ${to?.text ?? ""} · ${timeUnitLabels[unit]}`;
    push("timeFilter", "期間", range, () =>
      onApplyRecordFilter({ ...recordFilter, timeFilter: undefined }),
    );
  }
  if (recordFilter.caseId !== undefined) {
    push("caseId", filterFieldLabels.caseId, recordFilter.caseId, () =>
      onApplyRecordFilter({ ...recordFilter, caseId: undefined }),
    );
  }
  for (const kind of manualKinds) {
    push(
      `nodeKind:${kind}`,
      "ノードの種類",
      nodeKindLabels[kind],
      () => {
        const kept = manualKinds.filter((shown) => shown !== kind);
        // 最後の種類を外すと、検索の条件から決める選択へ戻す。
        onChangeView(
          kept.length === 0 ? autoView : { kind: "manual", nodeKinds: kept },
        );
      },
      kind,
    );
  }
  for (const origin of criteria.origins ?? []) {
    // chip には表示名を出し、ノードの識別子は写す値と説明が持つ。表示名の無いノードは識別子を出す。
    applied.push({
      key: `origin:${origin.id}`,
      label: "ノードID",
      value: origin.label === "" ? origin.id : origin.label,
      copyValue: origin.id,
      nodeKind: origin.kind,
      remove: () => {
        const kept = (criteria.origins ?? []).filter(
          (candidate) => candidate.id !== origin.id,
        );
        onApplyCriteria({
          ...criteria,
          origins: kept.length === 0 ? undefined : kept,
        });
      },
      description: `ノードID: ${origin.id}`,
    });
  }
  if (criteria.depth !== defaultDepth) {
    // chip には数だけを出す。数の意味は chip の説明が持つ。
    applied.push({
      key: "depth",
      label: "ホップ数",
      value: String(criteria.depth),
      copyValue: String(criteria.depth),
      remove: () => onApplyCriteria({ ...criteria, depth: defaultDepth }),
      description: depthDescriptionOf(criteria.depth),
    });
  }
  for (const kind of criteria.edgeKinds ?? []) {
    push(`edgeKind:${kind}`, "エッジの種類", edgeKindLabels[kind], () =>
      onApplyEdgeKinds(toggledEdgeKinds(criteria.edgeKinds, kind)),
    );
  }
  if (
    JSON.stringify(matchConditions) !== JSON.stringify(everyMatchCondition())
  ) {
    push(
      "matchConditions",
      "推定条件",
      describeMatchConditions(matchConditions),
      () => onApplyMatchConditions(everyMatchCondition()),
    );
  }
  if (criteria.countBy !== undefined) {
    push("countBy", filterFieldLabels.countBy, criteria.countBy, () =>
      onApplyCriteria({ ...criteria, countBy: undefined }),
    );
  }
  if (criteria.addressInCidr !== undefined) {
    push(
      "addressInCidr",
      filterFieldLabels.addressInCidr,
      criteria.addressInCidr,
      () => onApplyCriteria({ ...criteria, addressInCidr: undefined }),
      "ip",
    );
  }
  if (criteria.addressNotInCidr !== undefined) {
    push(
      "addressNotInCidr",
      filterFieldLabels.addressNotInCidr,
      criteria.addressNotInCidr,
      () => onApplyCriteria({ ...criteria, addressNotInCidr: undefined }),
      "ip",
    );
  }
  if (criteria.conditionsOnOriginsOnly === true) {
    push(
      "conditionsOnOriginsOnly",
      "条件の適用先",
      `${nodeSelectionLabels.matched}だけ`,
      () =>
        onApplyCriteria({ ...criteria, conditionsOnOriginsOnly: undefined }),
    );
  }
  if (criteria.endpointRecordsInPeriod === true) {
    push(
      "endpointRecordsInPeriod",
      "表示するエッジ",
      "両端のレコードが期間内",
      () =>
        onApplyCriteria({ ...criteria, endpointRecordsInPeriod: undefined }),
    );
  }

  const { copy, notice } = useCopyText();
  // 対象は条件の key で持ち、項目は描画ごとに今の条件から組む。開いている間に条件が変わっても、
  // 前の条件の値で外したり替えたりしない。
  const { triggers, menu } = useContextMenu((key: string) =>
    conditionMenuContent(
      applied.find((condition) => condition.key === key),
      copy,
    ),
  );

  const choices: Partial<Record<ChoiceKind, FetchState<readonly Choice[]>>> = {
    terminal:
      terminals.status === "loaded"
        ? {
            status: "loaded",
            value: terminals.value.map((terminal) => ({
              id: terminal.id,
              label: terminal.label,
            })),
          }
        : terminals,
    source: {
      status: "loaded",
      value: [...sourceFileNames]
        .filter(([id]) => !chosenSources.some((source) => source.id === id))
        .map(([id, label]) => ({ id, label })),
    },
    eventKind: eventKindChoices(eventKinds),
    nodeKind: {
      status: "loaded",
      value: viewKindOrder
        .filter((kind) => !manualKinds.includes(kind))
        .map((kind) => ({ id: kind, label: nodeKindLabels[kind] })),
    },
    depth: { status: "loaded", value: depthChoices },
    edgeKind: {
      status: "loaded",
      value: edgeKinds
        .filter((kind) => !(criteria.edgeKinds ?? []).includes(kind))
        .map((kind) => ({ id: kind, label: edgeKindLabels[kind] })),
    },
  };
  if (caseIds.length > 0) {
    choices.caseId = {
      status: "loaded",
      value: caseIds.map((caseId) => ({ id: caseId, label: caseId })),
    };
  }

  const uncategorized =
    eventKinds.status === "loaded"
      ? eventKinds.value.uncategorizedRecordCount
      : 0;

  const add = (condition: ConditionValue) => {
    switch (condition.kind) {
      case "contains":
      case "excludes":
        onChangeTerms(withTerm(terms, condition.kind, condition.text));
        return;
      case "field":
        onChangeTerms(withField(terms, condition.text));
        return;
      case "fieldContains":
      case "fieldEquals":
        onChangeTerms(
          withFieldTerm(
            terms,
            condition.field,
            condition.text,
            condition.kind === "fieldEquals",
          ),
        );
        return;
      case "expression":
        onChangeTerms(withExpression(terms, condition.text));
        return;
      case "terminal": {
        // 端末は 1 つだけを持つ。選び直すと置き換える。
        const terminal =
          terminals.status === "loaded"
            ? terminals.value.find((candidate) => candidate.id === condition.id)
            : undefined;
        if (terminal !== undefined) {
          onApplyRecordFilter({ ...recordFilter, terminal });
        }
        return;
      }
      case "source": {
        const label = sourceFileNames.get(condition.id);
        if (label !== undefined) {
          onApplyRecordFilter({
            ...recordFilter,
            sources: [...chosenSources, { id: condition.id, label }],
          });
        }
        return;
      }
      case "eventKind": {
        const chosen = readEventKindValue(condition.id);
        if (chosen !== undefined) {
          onApplyRecordFilter({
            ...recordFilter,
            eventCategory: chosen.category,
            eventAction: chosen.action,
          });
        }
        return;
      }
      case "eventActionRange":
        onApplyRecordFilter({
          ...recordFilter,
          eventActionFrom: condition.from,
          eventActionTo: condition.to,
        });
        return;
      case "timeFilter":
        onApplyRecordFilter({ ...recordFilter, timeFilter: condition.filter });
        return;
      case "caseId": {
        const caseId = caseIds.find((candidate) => candidate === condition.id);
        if (caseId !== undefined) {
          onApplyRecordFilter({ ...recordFilter, caseId });
        }
        return;
      }
      case "nodeKind": {
        const kind = viewKindOrder.find(
          (candidate) => candidate === condition.id,
        );
        if (kind === undefined) return;
        // 自動のときは、今出している種類に足す。
        const base =
          view.kind === "manual"
            ? view.nodeKinds
            : (resolvedView.nodeKinds ?? []);
        onChangeView({
          kind: "manual",
          nodeKinds: viewKindOrder.filter(
            (candidate) => candidate === kind || base.includes(candidate),
          ),
        });
        return;
      }
      case "depth": {
        const depth = Number(condition.id);
        if (Number.isInteger(depth)) onApplyCriteria({ ...criteria, depth });
        return;
      }
      case "edgeKind": {
        const kind = edgeKinds.find((candidate) => candidate === condition.id);
        if (kind !== undefined && !(criteria.edgeKinds ?? []).includes(kind)) {
          onApplyEdgeKinds(toggledEdgeKinds(criteria.edgeKinds, kind));
        }
        return;
      }
      case "matchConditions":
        onApplyMatchConditions(condition.selection);
        return;
      case "countBy":
        onApplyCriteria({
          ...criteria,
          countBy: condition.text === "" ? undefined : condition.text,
        });
        return;
      case "addressInCidr":
      case "addressNotInCidr":
        onApplyCriteria({ ...criteria, [condition.kind]: condition.text });
        return;
      case "conditionsOnOriginsOnly":
      case "endpointRecordsInPeriod":
        onApplyCriteria({ ...criteria, [condition.kind]: true });
        return;
      default: {
        const exhaustive: never = condition;
        throw new Error(`unknown condition: ${JSON.stringify(exhaustive)}`);
      }
    }
  };

  // 誤りは適用している式について出す。式を外した後に、前の式の誤りを残さない。
  const shownExpressionError =
    terms.expression === undefined ? undefined : expressionError;

  return (
    <section
      aria-labelledby="search-conditions-heading"
      className="flex flex-col gap-2"
    >
      <h3 id="search-conditions-heading" className="sr-only">
        条件
      </h3>
      <ConditionInput
        chips={applied.map(({ key, label, value, nodeKind, description }) => ({
          key,
          label,
          value,
          nodeKind,
          description,
          excluded: key.startsWith("excludes:"),
        }))}
        choices={choices}
        hints={{ depth: String(criteria.depth) }}
        descriptions={{
          eventKind:
            uncategorized > 0
              ? `フィルタで除外するイベントの種類の無いレコード: ${formatCount(uncategorized)}`
              : undefined,
          edgeKind: "未選択: すべてのエッジの種類",
          source: "一致: 選択したどれかの収集元のレコード",
          caseId: "適用先: 根拠のレコード・Timeline・Edge Detail",
        }}
        initial={{
          expression: terms.expression,
          eventActionRange: {
            from: recordFilter.eventActionFrom,
            to: recordFilter.eventActionTo,
          },
          timeFilter: recordFilter.timeFilter,
          matchConditions,
        }}
        onAdd={add}
        onRemove={(chip) =>
          applied.find((condition) => condition.key === chip.key)?.remove()
        }
        chipMenu={triggers}
        help={
          <>
            <span className="help-section">文字列の一致</span>
            <KeyValueList stacked pairs={textMatchRules} />
            <span className="help-section">検索式の書き方</span>
            <SearchExpressionSyntax />
          </>
        }
      >
        {terms.expression === undefined ||
        shownExpressionError === undefined ? null : (
          <SearchExpressionErrorView
            expression={terms.expression}
            error={shownExpressionError}
          />
        )}
      </ConditionInput>
      {notice}
      {menu}
      <div className="text-xs text-muted">
        <KeyValueList
          pairs={[
            {
              name: "ノードの種類",
              value:
                view.kind === "auto"
                  ? `自動: ${describeView(resolvedView)}`
                  : describeView(resolvedView),
            },
          ]}
        />
        <label>
          <input
            type="checkbox"
            checked={mergeSameAccount}
            onChange={(event) => onChangeMergeSameAccount(event.target.checked)}
          />
          <Hint text={mergeSameAccountHint}>同じアカウントとしてまとめる</Hint>
        </label>
      </div>
    </section>
  );
}
