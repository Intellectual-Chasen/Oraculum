import { useEffect, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import {
  fetchGraph,
  type GraphRequest,
  type GraphTimeFilter,
  type NodeRef,
} from "@/shared/api/graph";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import type { CaseId } from "@/shared/contracts/cases";
import type {
  EdgeKind,
  GraphGranularity,
  GraphResponse,
  NodeKind,
} from "@/shared/contracts/graph";
import type { FetchState } from "@/shared/lib/fetchState";

/** 利用者が適用した部分グラフの条件。 */
export type SubgraphCriteria = {
  /**
   * 候補を絞るのに用いる条件の選択。上位の画面が持ち、時系列と詳細も同じ選択を読む。
   */
  matchConditions: MatchConditionSelection;
  /** 図に描くノードの数の上限。出ない場合は上限を置かない。 */
  nodeLimit?: number;
  /**
   * 起点から広げるホップ数。要求が受け取るのは `minGraphDepth` から `maxGraphDepth` まで
   * である。
   */
  depth: number;
  /** 出ない場合はノードの種別で絞らない。 */
  nodeKinds?: readonly NodeKind[];
  /** 出ない場合はレコードの粒度で読む。 */
  granularity?: GraphGranularity;
  /** 出ない場合は関係の種別で絞らない。どれかの種別の関係だけを辿る。 */
  edgeKinds?: readonly EdgeKind[];
  /** 近傍を展開する起点のノード。出ない場合は起点を指定しない。 */
  nodeIds?: readonly string[];
  /**
   * 検索の条件が一致ノードを限るノードと表示名。検索の要求は、この識別子を `nodeIds` に載せる。
   * 探索の要求は持たない。
   */
  origins?: readonly NodeRef[];
  /** 根拠のレコードを絞る期間。両端が無い場合は要求へ載せない。 */
  timeFilter?: GraphTimeFilter;
  /**
   * 事象の分類の文字列。出ない場合はその文字列で絞らない。
   * **文字列の一致を完全一致で調べる。**
   */
  eventCategory?: string;
  eventAction?: string;
  /** 事象の動作を 10 進の数として比べる範囲の両端。出ない場合は範囲で絞らない。 */
  eventActionFrom?: number;
  eventActionTo?: number;
  /** 根拠のレコードを絞る案件。出ない場合はすべての案件を読む。 */
  caseId?: CaseId;
  /** 根拠のレコードを絞る端末のノードの識別子。出ない場合は端末で絞らない。 */
  terminal?: string;
  /** 根拠のレコードを絞る収集元の sourceId。出ない場合は収集元で絞らない。 */
  sources?: readonly string[];
  /** IP アドレスのノードを残すアドレスの範囲。出ない場合は範囲で絞らない。 */
  addressInCidr?: string;
  /** IP アドレスのノードを外すアドレスの範囲。出ない場合は範囲で絞らない。 */
  addressNotInCidr?: string;
  /** 属性の値にどれも含まれることを求める文字列。空の集合は文字列で絞らない。 */
  valueContains?: readonly string[];
  /** 属性の値にどれも含まれないことを求める文字列。 */
  valueExcludes?: readonly string[];
  /** 文字列を照合する欄。出ない場合は全欄を照合する。 */
  valueField?: string;
  /** 欄と文字列の組 (`欄=文字列`)。どの組もその欄に文字列を含むことを求める。 */
  fieldContains?: readonly string[];
  /** 完全一致の欄と文字列の組 (`欄=文字列`)。どの組もその欄の値の全体が文字列と等しいことを求める。 */
  fieldEquals?: readonly string[];
  /** 欄・演算子・論理・括弧で書いた検索式。出ない場合は式で絞らない。 */
  searchExpression?: string;
  /** 値ごとに数える欄。語彙の項目または原資料の key。出ない場合は数えない。 */
  countBy?: string;
  /** 真のとき、文字列と事象の種別の条件を起点の判定だけに適用し、起点から辿るエッジに適用しない。 */
  conditionsOnOriginsOnly?: boolean;
  /** 真のとき、端点のレコードが期間の外にあるエッジを辿らない。 */
  endpointRecordsInPeriod?: boolean;
};

const failureSummary = "グラフの取得";

/** 画面の条件を、操作 9 の要求の項目へ写す。 */
export function graphRequestOf(criteria: SubgraphCriteria): GraphRequest {
  return {
    matchConditions: criteria.matchConditions,
    nodeLimit: criteria.nodeLimit,
    depth: criteria.depth,
    nodeKinds: criteria.nodeKinds,
    granularity: criteria.granularity,
    edgeKinds: criteria.edgeKinds,
    nodeIds: criteria.nodeIds,
    timeFilter: criteria.timeFilter,
    eventCategory: criteria.eventCategory,
    eventAction: criteria.eventAction,
    eventActionFrom: criteria.eventActionFrom,
    eventActionTo: criteria.eventActionTo,
    caseId: criteria.caseId,
    terminal: criteria.terminal,
    sources: criteria.sources,
    addressInCidr: criteria.addressInCidr,
    addressNotInCidr: criteria.addressNotInCidr,
    valueContains: criteria.valueContains,
    valueExcludes: criteria.valueExcludes,
    valueField: criteria.valueField,
    fieldContains: criteria.fieldContains,
    fieldEquals: criteria.fieldEquals,
    searchExpression: criteria.searchExpression,
    countBy: criteria.countBy,
    conditionsOnOriginsOnly: criteria.conditionsOnOriginsOnly,
    endpointRecordsInPeriod: criteria.endpointRecordsInPeriod,
  };
}

/** 読み込み中の状態。描画ごとに別の値を作らない。 */
const loadingState: FetchState<GraphResponse> = { status: "loading" };

/**
 * 部分グラフを取得する。
 * 条件が変わるたびと、`version` が変わるたびに取り直し、前の取得を `AbortController` で
 * 打ち切る。`version` は、端末の割当の記録で backend のグラフが変わったことを表す番号である。
 *
 * **状態をどの条件で取ったかを覚え、今の条件と違う間は読み込み中を返す。** 取り直しの effect は
 * 条件が変わった描画の後に走るため、覚えずに返すと、条件が変わった描画で前の条件の応答を
 * 読み込み済みとして返す。その応答で選択を判定すると、新しい条件の応答にだけあるノードの選択を外す。
 */
export function useSubgraph(
  criteria: SubgraphCriteria | undefined,
  version: number,
): FetchState<GraphResponse> {
  const [held, setHeld] = useState<{
    criteria: SubgraphCriteria | undefined;
    version: number;
    state: FetchState<GraphResponse>;
  }>({ criteria: undefined, version, state: loadingState });

  // version が変わったときも、同じ条件で取り直す。
  useEffect(() => {
    const hold = (state: FetchState<GraphResponse>) =>
      setHeld({ criteria, version, state });
    // 条件の無い間は取得しない。部分グラフの背景は、部分グラフを出している間だけ取る。
    if (criteria === undefined) {
      hold(loadingState);
      return;
    }
    const controller = new AbortController();
    hold(loadingState);

    const load = async () => {
      const result = await fetchGraph(graphRequestOf(criteria), {
        signal: controller.signal,
      });
      if (controller.signal.aborted) {
        return;
      }
      if (!result.ok) {
        hold({ status: "failed", failure: result.failure });
        return;
      }
      // **0 件の応答も loaded で渡す。** 応答は絞り込みに用いた期間を含むため、
      // 取得の状態へまとめると画面がその条件を出せない。0 件の表示は描く側が持つ。
      hold({ status: "loaded", value: result.value });
    };

    void load().catch(() => {
      if (controller.signal.aborted) {
        return;
      }
      hold({
        status: "failed",
        failure: buildFetchFailure("unexpected", failureSummary),
      });
    });
    return () => controller.abort();
  }, [criteria, version]);

  return held.criteria === criteria && held.version === version
    ? held.state
    : loadingState;
}
