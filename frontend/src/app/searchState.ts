import type {
  GraphTimeFilter,
  NodeRef,
  RecordFilterCriteria,
} from "@/shared/api/graph";
import type { AssistOrigin } from "@/shared/contracts/assistRelay";
import type { SearchQuery } from "@/shared/contracts/searchQuery";
import type { GraphConditions } from "@/shared/lib/graphConditions";
import type { SearchTerms } from "@/shared/lib/searchTerms";
import {
  autoView,
  type ResolvedView,
  type ViewChoice,
} from "@/shared/lib/searchView";

/**
 * 画面の検索の条件のすべて。AI 支援の card は、この組を 1 回で置き換え、元へ戻す。
 * 検索の文字列と根拠のレコードを絞る条件は本画面が持ち、図の条件と図に出す対象の選び方は
 * グラフの探索が持つ。
 */
export type SearchState = {
  searchTerms: SearchTerms;
  recordFilter: RecordFilterCriteria;
  graphConditions: GraphConditions;
  view: ViewChoice;
};

/**
 * 検索の条件の項目から、図に出す対象の選び方を読む。種別と粒度を持たない条件は、図に出す対象を
 * 検索の条件から決める。
 */
function viewOf(query: SearchQuery): ViewChoice {
  const kinds = query.nodeKinds ?? [];
  if (kinds.length > 0) {
    return { kind: "manual", nodeKinds: kinds };
  }
  return query.granularity === undefined
    ? autoView
    : { kind: "manual", nodeKinds: [] };
}

/** 空の並びを「条件を与えない」として項目から外す。 */
function nonEmpty<T>(values: readonly T[] | undefined): T[] | undefined {
  return values === undefined || values.length === 0 ? undefined : [...values];
}

/**
 * 画面の検索の条件と、図が出している対象を、AI 支援の発言に添える `SearchQuery` にする。
 *
 * **図に出す対象は、画面が決めた種別と粒度を載せる。** 検索の条件から決める選び方でも、AI が読む
 * グラフが画面のグラフと同じ種別で絞られる。
 */
export function searchQueryOf(
  state: Omit<SearchState, "view">,
  shown: ResolvedView,
): SearchQuery {
  const { searchTerms, recordFilter, graphConditions } = state;
  const timeFilter = recordFilter.timeFilter;
  return {
    granularity: shown.granularity,
    nodeKinds: nonEmpty(shown.nodeKinds),
    depth: graphConditions.depth,
    nodeIds: nonEmpty(graphConditions.origins?.map((origin) => origin.id)),
    edgeKinds: nonEmpty(graphConditions.edgeKinds),
    valueContains: nonEmpty(searchTerms.contains),
    valueExcludes: nonEmpty(searchTerms.excludes),
    valueField:
      searchTerms.contains.length > 0 || searchTerms.excludes.length > 0
        ? searchTerms.field
        : undefined,
    fieldContains: nonEmpty(searchTerms.fieldContains),
    fieldEquals: nonEmpty(searchTerms.fieldEquals),
    searchExpression: searchTerms.expression,
    conditionsOnOriginsOnly: graphConditions.conditionsOnOriginsOnly,
    endpointRecordsInPeriod: graphConditions.endpointRecordsInPeriod,
    countBy: graphConditions.countBy,
    addressInCidr: graphConditions.addressInCidr,
    addressNotInCidr: graphConditions.addressNotInCidr,
    eventCategory: recordFilter.eventCategory,
    eventAction: recordFilter.eventAction,
    eventActionFrom: recordFilter.eventActionFrom,
    eventActionTo: recordFilter.eventActionTo,
    timeFrom: timeFilter?.from?.text,
    timeFromPrecision: timeFilter?.from?.precision,
    timeTo: timeFilter?.to?.text,
    timeToPrecision: timeFilter?.to?.precision,
    filterUnit: timeFilter?.unit,
    case: recordFilter.caseId,
    terminal: recordFilter.terminal?.id,
    sources: nonEmpty(recordFilter.sources?.map((source) => source.id)),
  };
}

/** 検索の条件の期間の項目を、根拠のレコードを絞る期間へ読む。 */
function timeFilterOf(query: SearchQuery): GraphTimeFilter | undefined {
  if (query.filterUnit === undefined) {
    return undefined;
  }
  return {
    from:
      query.timeFrom === undefined || query.timeFromPrecision === undefined
        ? undefined
        : { text: query.timeFrom, precision: query.timeFromPrecision },
    to:
      query.timeTo === undefined || query.timeToPrecision === undefined
        ? undefined
        : { text: query.timeTo, precision: query.timeToPrecision },
    unit: query.filterUnit,
  };
}

/** 検索の文字列の条件を組む。与えていない項目は条件に載せない。 */
function searchTermsOf(query: SearchQuery): SearchTerms {
  return {
    contains: query.valueContains ?? [],
    excludes: query.valueExcludes ?? [],
    ...(query.valueField === undefined ? {} : { field: query.valueField }),
    ...(query.fieldContains === undefined || query.fieldContains.length === 0
      ? {}
      : { fieldContains: query.fieldContains }),
    ...(query.fieldEquals === undefined || query.fieldEquals.length === 0
      ? {}
      : { fieldEquals: query.fieldEquals }),
    ...(query.searchExpression === undefined
      ? {}
      : { expression: query.searchExpression }),
  };
}

/**
 * AI 支援の card の `SearchQuery` を、画面の検索の条件にする。
 * 端末の表示名は端末の選択肢から、収集元の表示名は sourceLabels から探す。どちらにも無いものは
 * 識別子を表示名にする。一致ノードを限るノードは、card が `nodeIds` と同じ順に持つ origins の
 * 種別と表示名で組む。起点を持たない card は、前に適用した起点を外す。
 */
export function searchStateOf(
  query: SearchQuery,
  origins: readonly AssistOrigin[] | undefined,
  terminals: readonly NodeRef[],
  sourceLabels: ReadonlyMap<string, string>,
): SearchState {
  const terminalId = query.terminal;
  const terminal =
    terminalId === undefined
      ? undefined
      : (terminals.find((candidate) => candidate.id === terminalId) ?? {
          id: terminalId,
          label: terminalId,
        });
  const sources = query.sources?.map((id) => ({
    id,
    label: sourceLabels.get(id) ?? id,
  }));
  return {
    searchTerms: searchTermsOf(query),
    recordFilter: {
      timeFilter: timeFilterOf(query),
      eventCategory: query.eventCategory,
      eventAction: query.eventAction,
      eventActionFrom: query.eventActionFrom,
      eventActionTo: query.eventActionTo,
      caseId: query.case,
      terminal,
      sources:
        sources === undefined || sources.length === 0 ? undefined : sources,
    },
    graphConditions: {
      depth: query.depth,
      origins:
        origins === undefined || origins.length === 0
          ? undefined
          : origins.map(({ id, label, kind }) => ({ id, label, kind })),
      edgeKinds:
        (query.edgeKinds?.length ?? 0) > 0 ? query.edgeKinds : undefined,
      addressInCidr: query.addressInCidr,
      addressNotInCidr: query.addressNotInCidr,
      countBy: query.countBy,
      conditionsOnOriginsOnly: query.conditionsOnOriginsOnly || undefined,
      endpointRecordsInPeriod: query.endpointRecordsInPeriod || undefined,
    },
    view: viewOf(query),
  };
}
