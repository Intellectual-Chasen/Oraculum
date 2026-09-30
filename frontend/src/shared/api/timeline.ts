import type { CaseId } from "../contracts/cases";
import {
  decodeTimelineResponse,
  type TimelineResponse,
} from "../contracts/timeline";
import { buildFetchFailure } from "./apiFailure";
import {
  expressionParam,
  type GraphTimeFilter,
  nonEmpty,
  numberParam,
} from "./graph";
import { type ApiResult, requestJson } from "./httpClient";
import {
  type MatchConditionSelection,
  matchConditionParams,
} from "./matchConditions";

/**
 * 時系列の操作の要求の項目。
 *
 * **上限も続きを取る位置も持たない。** 応答は絞り込んだ結果を全件返す。
 */
export type TimelineRequest = {
  /** 候補を絞るのに用いる条件の選択。要求が必ず含む。 */
  matchConditions: MatchConditionSelection;
  /** 根拠のレコードを絞る期間。両端が無い場合は要求へ載せない。 */
  timeFilter?: GraphTimeFilter;
  /** 事象の分類の文字列。文字列の一致を完全一致で調べる。 */
  eventCategory?: string;
  eventAction?: string;
  /** 事象の動作を 10 進の数として比べる範囲の両端。 */
  eventActionFrom?: number;
  eventActionTo?: number;
  /** 根拠のレコードを絞る案件。出ない場合はすべての案件を読む。 */
  caseId?: CaseId;
  /** 根拠のレコードを絞る端末のノードの識別子。出ない場合は端末で絞らない。 */
  terminal?: string;
  /** 根拠のレコードを絞る収集元の sourceId。空か出ない場合は収集元で絞らない。 */
  sources?: readonly string[];
  /**
   * 欄・演算子・論理・括弧で書いた検索式。式に合うレコードだけを並べる。グラフの探索と
   * 同じ文字列を与える。空白だけの式は要求に載せない。
   */
  searchExpression?: string;
  /**
   * 起点のノードと段数。与えると、起点から `depth` の段数で辿ったエッジと起点のノードの根拠の
   * レコードだけを並べる。出ない場合はノードで絞らない。
   */
  near?: { nodeId: string; depth: number };
  /** 役割付きで名指されたアカウントのノード。 */
  accountNodeId?: string;
  /**
   * 原文から探す文字列と、大文字と小文字を区別するか。与えると応答が `findMatches` を含む。
   * 空の文字列は与えない。
   */
  find?: { text: string; caseSensitive: boolean };
};

const timelineFailureSummary = "Timeline の取得";

/**
 * 時系列の操作 (`GET /api/v0/timeline`) を実行する。
 * handler が失敗を返す要求 (時刻の文字列が 1 つも無い期間の絞り込み) を送らずに
 * 失敗として返す。
 */
export async function fetchTimeline(
  request: TimelineRequest,
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<TimelineResponse>> {
  const timeFilter = request.timeFilter;
  if (
    timeFilter !== undefined &&
    timeFilter.from === undefined &&
    timeFilter.to === undefined
  ) {
    return {
      ok: false,
      failure: buildFetchFailure("request_rejected", timelineFailureSummary),
    };
  }
  return requestJson({
    path: "/timeline",
    searchParams: {
      eventCategory: request.eventCategory,
      eventAction: request.eventAction,
      eventActionFrom: numberParam(request.eventActionFrom),
      eventActionTo: numberParam(request.eventActionTo),
      case: request.caseId,
      terminal: request.terminal,
      source: nonEmpty(request.sources),
      searchExpression: expressionParam(request.searchExpression),
      nodeId: request.near?.nodeId,
      accountNodeId: request.accountNodeId,
      depth: numberParam(request.near?.depth),
      find: request.find?.text,
      findCaseSensitive:
        request.find === undefined
          ? undefined
          : String(request.find.caseSensitive),
      timeFrom: timeFilter?.from?.text,
      timeFromPrecision: timeFilter?.from?.precision,
      timeTo: timeFilter?.to?.text,
      timeToPrecision: timeFilter?.to?.precision,
      filterUnit: timeFilter === undefined ? undefined : timeFilter.unit,
      matchCondition: matchConditionParams(request.matchConditions),
    },
    decode: decodeTimelineResponse,
    failureSummary: timelineFailureSummary,
    signal: options.signal,
  });
}
