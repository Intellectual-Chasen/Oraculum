import { type CaseId, decodeCaseId } from "./cases";
import {
  type Decoder,
  decodeString,
  optionalArray,
  optionalBoolean,
  optionalCount,
  optionalEnum,
  optionalMember,
  optionalString,
  readObject,
  requireCount,
} from "./decoding";
import {
  decodeEdgeKind,
  decodeNodeKind,
  type EdgeKind,
  type FilterUnit,
  filterUnits,
  type GraphBoundPrecision,
  type GraphGranularity,
  graphBoundPrecisions,
  graphGranularities,
  type NodeKind,
} from "./graph";

/**
 * 画面の検索欄が表せる検索の条件。定義元は `backend/core/search_query.go` の `SearchQuery` である。
 * AI 支援の発言に添える画面の文脈と、LLM が画面に出す検索の条件の card が持つ。
 * 出ない項目は条件を与えていない。
 */
export type SearchQuery = {
  nodeKinds?: NodeKind[];
  granularity?: GraphGranularity;
  /**
   * 一致ノードを限るノード。画面と card はノードの識別子、LLM へ渡す文脈は server が発行した
   * 短い参照を持つ。
   */
  nodeIds?: string[];
  depth: number;
  edgeKinds?: EdgeKind[];
  valueContains?: string[];
  valueExcludes?: string[];
  valueField?: string;
  /** 欄と文字列の組。`欄=文字列` の文字列である。 */
  fieldContains?: string[];
  /** 値の全体が文字列と等しい欄だけを一致とする欄と文字列の組。書き方は fieldContains と同じである。 */
  fieldEquals?: string[];
  /** 欄・演算子・論理・括弧で書いた検索式の文字列。 */
  searchExpression?: string;
  /** 真のとき、文字列と事象の種別の条件を起点の判定だけに当てる。 */
  conditionsOnOriginsOnly?: boolean;
  /** 真のとき、端点のレコードが期間の外にあるエッジを辿らない。 */
  endpointRecordsInPeriod?: boolean;
  countBy?: string;
  addressInCidr?: string;
  addressNotInCidr?: string;
  eventCategory?: string;
  eventAction?: string;
  eventActionFrom?: number;
  eventActionTo?: number;
  /** 日付と UTC からのずれを含む文字列。 */
  timeFrom?: string;
  timeFromPrecision?: GraphBoundPrecision;
  timeTo?: string;
  timeToPrecision?: GraphBoundPrecision;
  filterUnit?: FilterUnit;
  case?: CaseId;
  /** 根拠のレコードを絞る端末のノードの識別子。 */
  terminal?: string;
  /** 根拠のレコードを絞る収集元の sourceId。 */
  sources?: string[];
};

/** `SearchQuery` を検証する。 */
export const decodeSearchQuery: Decoder<SearchQuery> = (input, path) => {
  const source = readObject(input, path);
  return {
    nodeKinds: optionalArray(source, "nodeKinds", path, decodeNodeKind),
    granularity: optionalEnum(source, "granularity", path, graphGranularities),
    nodeIds: optionalArray(source, "nodeIds", path, decodeString),
    depth: requireCount(source, "depth", path),
    edgeKinds: optionalArray(source, "edgeKinds", path, decodeEdgeKind),
    valueContains: optionalArray(source, "valueContains", path, decodeString),
    valueExcludes: optionalArray(source, "valueExcludes", path, decodeString),
    valueField: optionalString(source, "valueField", path),
    fieldContains: optionalArray(source, "fieldContains", path, decodeString),
    fieldEquals: optionalArray(source, "fieldEquals", path, decodeString),
    searchExpression: optionalString(source, "searchExpression", path),
    conditionsOnOriginsOnly: optionalBoolean(
      source,
      "conditionsOnOriginsOnly",
      path,
    ),
    endpointRecordsInPeriod: optionalBoolean(
      source,
      "endpointRecordsInPeriod",
      path,
    ),
    countBy: optionalString(source, "countBy", path),
    addressInCidr: optionalString(source, "addressInCidr", path),
    addressNotInCidr: optionalString(source, "addressNotInCidr", path),
    eventCategory: optionalString(source, "eventCategory", path),
    eventAction: optionalString(source, "eventAction", path),
    eventActionFrom: optionalCount(source, "eventActionFrom", path),
    eventActionTo: optionalCount(source, "eventActionTo", path),
    timeFrom: optionalString(source, "timeFrom", path),
    timeFromPrecision: optionalEnum(
      source,
      "timeFromPrecision",
      path,
      graphBoundPrecisions,
    ),
    timeTo: optionalString(source, "timeTo", path),
    timeToPrecision: optionalEnum(
      source,
      "timeToPrecision",
      path,
      graphBoundPrecisions,
    ),
    filterUnit: optionalEnum(source, "filterUnit", path, filterUnits),
    case: optionalMember(source, "case", path, decodeCaseId),
    terminal: optionalString(source, "terminal", path),
    sources: optionalArray(source, "sources", path, decodeString),
  };
};
