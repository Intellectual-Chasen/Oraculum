import type { GraphConditions } from "./graphConditions";
import { edgeKindLabels, filterFieldLabels } from "./graphLabels";

/** 検索の条件の chip の種類。外す操作を種類ごとに選ぶ。 */
export type SearchChipKind =
  | "contains"
  | "excludes"
  | "terminal"
  | "source"
  | "eventCategory"
  | "eventAction"
  | "eventActionRange"
  | "fieldContains"
  | "fieldEquals"
  | "expression"
  | "field";

/** 検索の条件 1 つの表示。 */
export type SearchChip<K extends string = SearchChipKind> = {
  /** 一覧の中で一意な key。 */
  key: string;
  kind: K;
  label: string;
  /** 原資料の文字列を含みうる値。描く側が文字列として出す。 */
  value: string;
  /** chip が指す条件の識別子。収集元の chip は sourceId を持つ。 */
  id?: string;
};

/** chip に組む検索の文字列と、根拠のレコードを絞る条件。 */
export type SearchChipSource = {
  contains: readonly string[];
  excludes: readonly string[];
  field?: string;
  /** 欄と文字列の組。`欄=文字列` の文字列である。 */
  fieldContains?: readonly string[];
  /** 値の全体が文字列と等しい欄だけを一致とする欄と文字列の組。書き方は fieldContains と同じである。 */
  fieldEquals?: readonly string[];
  /** 検索式の文字列。 */
  expression?: string;
  /** 端末の表示名。 */
  terminal?: string;
  /** 根拠のレコードを絞る収集元の sourceId と表示名。 */
  sources?: readonly { id: string; label: string }[];
  eventCategory?: string;
  eventAction?: string;
  eventActionFrom?: number;
  eventActionTo?: number;
};

/** 事象の種別の 2 つの欄の名前。事象の種別の一覧から選ぶ。 */
export type EventKindChipLabels = {
  eventCategory: string;
  eventAction: string;
};

/** 事象の種別の一覧を読まないときの欄の名前。 */
export const genericEventKindLabels: EventKindChipLabels = {
  eventCategory: filterFieldLabels.eventCategory,
  eventAction: filterFieldLabels.eventAction,
};

/**
 * 検索欄が chip に出す条件を、画面に出す順に組む。検索欄の chip と、AI 支援の検索の条件の
 * card が同じ手順で組む。
 */
export function searchChipsOf(
  source: SearchChipSource,
  eventKindLabels: EventKindChipLabels,
): SearchChip[] {
  const chips: SearchChip[] = [
    ...source.contains.map((token) => ({
      key: `contains:${token}`,
      kind: "contains" as const,
      label: filterFieldLabels.valueContains,
      value: token,
    })),
    ...source.excludes.map((token) => ({
      key: `excludes:${token}`,
      kind: "excludes" as const,
      label: filterFieldLabels.valueExcludes,
      value: token,
    })),
  ];
  if (source.terminal !== undefined) {
    chips.push({
      key: "terminal",
      kind: "terminal",
      label: filterFieldLabels.terminal,
      value: source.terminal,
    });
  }
  for (const chosen of source.sources ?? []) {
    chips.push({
      key: `source:${chosen.id}`,
      kind: "source",
      label: filterFieldLabels.sources,
      value: chosen.label,
      id: chosen.id,
    });
  }
  if (source.eventCategory !== undefined) {
    chips.push({
      key: "eventCategory",
      kind: "eventCategory",
      label: eventKindLabels.eventCategory,
      value: source.eventCategory,
    });
  }
  if (source.eventAction !== undefined) {
    chips.push({
      key: "eventAction",
      kind: "eventAction",
      label: eventKindLabels.eventAction,
      value: source.eventAction,
    });
  }
  if (
    source.eventActionFrom !== undefined ||
    source.eventActionTo !== undefined
  ) {
    chips.push({
      key: "eventActionRange",
      kind: "eventActionRange",
      label: "イベント ID",
      value: `${source.eventActionFrom ?? ""}-${source.eventActionTo ?? ""}`,
    });
  }
  for (const pair of source.fieldContains ?? []) {
    chips.push({
      key: `fieldContains:${pair}`,
      kind: "fieldContains",
      label: filterFieldLabels.fieldContains,
      value: pair,
    });
  }
  for (const pair of source.fieldEquals ?? []) {
    chips.push({
      key: `fieldEquals:${pair}`,
      kind: "fieldEquals",
      label: filterFieldLabels.fieldEquals,
      value: pair,
    });
  }
  if (source.expression !== undefined) {
    chips.push({
      key: "expression",
      kind: "expression",
      label: filterFieldLabels.searchExpression,
      value: source.expression,
    });
  }
  if (source.field !== undefined) {
    chips.push({
      key: "field",
      kind: "field",
      label: "検索するフィールド",
      value: source.field,
    });
  }
  return chips;
}

/** 図だけが読む条件の chip の種類。 */
export type GraphChipKind =
  | "origin"
  | "depth"
  | "edgeKinds"
  | "addressInCidr"
  | "addressNotInCidr"
  | "countBy"
  | "conditionsOnOriginsOnly"
  | "endpointRecordsInPeriod";

/** 図だけが読む条件を、詳しい条件の欄の並びで chip に組む。 */
export function graphChipsOf(
  conditions: GraphConditions,
): SearchChip<GraphChipKind>[] {
  const chips: SearchChip<GraphChipKind>[] = [
    // 起点の chip はノードの表示名を値に、ノードの識別子を id に持つ。表示名の無いノードは識別子を値にする。
    ...(conditions.origins ?? []).map((origin) => ({
      key: `origin:${origin.id}`,
      kind: "origin" as const,
      label: "ノードID",
      value: origin.label === "" ? origin.id : origin.label,
      id: origin.id,
    })),
    {
      key: "depth",
      kind: "depth",
      label: "一致ノードからのホップ数",
      value: String(conditions.depth),
    },
  ];
  if (conditions.conditionsOnOriginsOnly === true) {
    chips.push({
      key: "conditionsOnOriginsOnly",
      kind: "conditionsOnOriginsOnly",
      label: "文字列とイベントの条件の対象",
      value: "一致ノードだけ",
    });
  }
  if (conditions.endpointRecordsInPeriod === true) {
    chips.push({
      key: "endpointRecordsInPeriod",
      kind: "endpointRecordsInPeriod",
      label: "期間の条件",
      value: "両端が期間内のエッジだけ",
    });
  }
  if ((conditions.edgeKinds?.length ?? 0) > 0) {
    chips.push({
      key: "edgeKinds",
      kind: "edgeKinds",
      label: "エッジの種類",
      value: (conditions.edgeKinds ?? [])
        .map((kind) => edgeKindLabels[kind])
        .join("、"),
    });
  }
  if (conditions.addressInCidr !== undefined) {
    chips.push({
      key: "addressInCidr",
      kind: "addressInCidr",
      label: filterFieldLabels.addressInCidr,
      value: conditions.addressInCidr,
    });
  }
  if (conditions.addressNotInCidr !== undefined) {
    chips.push({
      key: "addressNotInCidr",
      kind: "addressNotInCidr",
      label: filterFieldLabels.addressNotInCidr,
      value: conditions.addressNotInCidr,
    });
  }
  if (conditions.countBy !== undefined) {
    chips.push({
      key: "countBy",
      kind: "countBy",
      label: filterFieldLabels.countBy,
      value: conditions.countBy,
    });
  }
  return chips;
}
