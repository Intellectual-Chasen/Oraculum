import type {
  GraphRequest,
  NodeRef,
  RecordFilterCriteria,
} from "@/shared/api/graph";
import type { SearchTerms } from "@/shared/lib/searchTerms";
import type { ResolvedView, ViewInput } from "@/shared/lib/searchView";

/** 図に出す対象の自動の選択が読む、グラフの探索の条件。 */
export type ViewCriteria = Pick<
  GraphRequest,
  "addressInCidr" | "addressNotInCidr" | "edgeKinds" | "countBy"
> & { origins?: readonly NodeRef[] };

/**
 * 図に出す対象に、一致ノードを限るノードの種別を足す。起点にレコードがあれば、レコードの粒度で
 * 出す。
 *
 * **起点の種別を足さないと、起点が種別の絞り込みで応答から消える。** 応答は起点も種別と粒度で
 * 絞る。関係先の探索の要求と同じ規則である。
 */
export function withOriginKinds(
  resolved: ResolvedView,
  origins: readonly NodeRef[] | undefined,
): ResolvedView {
  if (origins === undefined || origins.length === 0) {
    return resolved;
  }
  const originKinds = origins.flatMap((origin) =>
    origin.kind === undefined ? [] : [origin.kind],
  );
  return {
    granularity: originKinds.includes("record")
      ? "record"
      : resolved.granularity,
    nodeKinds:
      resolved.nodeKinds === undefined
        ? undefined
        : [...new Set([...resolved.nodeKinds, ...originKinds])],
  };
}

/**
 * 検索の条件から、図に出す対象の自動の選択が読む要約を作る。グラフの探索が図の要求を組むときと、
 * AI 支援の発言に図が出している対象を添えるときに同じ要約を使う。
 *
 * **グラフを絞る条件があれば、端末の粒度をやめる。** 端末の粒度では、文字列・検索式・期間・
 * アドレスの範囲・関係の種別・数える欄のどれもグラフに現れない。
 */
export function viewInputOf(
  searchTerms: SearchTerms,
  recordFilter: RecordFilterCriteria,
  criteria: ViewCriteria,
): ViewInput {
  const hasCondition =
    searchTerms.contains.length > 0 ||
    searchTerms.excludes.length > 0 ||
    (searchTerms.fieldContains?.length ?? 0) > 0 ||
    (searchTerms.fieldEquals?.length ?? 0) > 0 ||
    searchTerms.expression !== undefined ||
    recordFilter.terminal !== undefined ||
    (recordFilter.sources?.length ?? 0) > 0 ||
    recordFilter.eventCategory !== undefined ||
    recordFilter.eventAction !== undefined ||
    recordFilter.eventActionFrom !== undefined ||
    recordFilter.eventActionTo !== undefined ||
    recordFilter.timeFilter !== undefined ||
    criteria.addressInCidr !== undefined ||
    criteria.addressNotInCidr !== undefined ||
    (criteria.edgeKinds?.length ?? 0) > 0 ||
    criteria.countBy !== undefined ||
    (criteria.origins?.length ?? 0) > 0;
  return { hasCondition, eventCategory: recordFilter.eventCategory };
}
