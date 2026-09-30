import type { CaseId } from "../contracts/cases";
import {
  decodeGraphResponse,
  type EdgeDirection,
  type EdgeKind,
  type GraphGranularity,
  type GraphResponse,
  type GraphTimeFilter,
  type NodeKind,
} from "../contracts/graph";
import {
  decodeEdgeDetailResponse,
  decodeNodeDetailResponse,
  type EdgeDetailResponse,
  type EdgeEvidenceSelector,
  type NodeDetailResponse,
} from "../contracts/graphDetail";
import {
  decodeNodeValueCountsResponse,
  type NodeValueCountsResponse,
} from "../contracts/nodeValueCounts";
import {
  decodeUrlFragmentJoin,
  type UrlFragmentJoin,
} from "../contracts/urlFragments";
import { buildFetchFailure } from "./apiFailure";
import { type ApiResult, requestJson } from "./httpClient";
import {
  type MatchConditionSelection,
  matchConditionParams,
} from "./matchConditions";

export type { GraphTimeBound, GraphTimeFilter } from "../contracts/graph";

/**
 * 根拠のレコードを絞る条件。
 *
 * **グラフの探索と時系列が同じ値を読む。** どちらも同じレコードの集合を絞るため、
 * 条件を 2 つ持つと 2 つの画面が別の集合を出す。
 */
export type RecordFilterCriteria = {
  /** 両端が無い場合は要求へ載せない。 */
  timeFilter?: GraphTimeFilter;
  /**
   * 事象の分類の文字列。
   *
   * **文字列の一致を完全一致で調べる** (`backend/pipeline/record_filter.go` の `RecordFilter`)。
   * 部分一致の `valueContains` と意味が違う。
   */
  eventCategory?: string;
  eventAction?: string;
  /**
   * 事象の動作 (Windows イベントログではイベント ID) を 10 進の数として比べる範囲の両端。
   * 両端を含み、片側だけでもよい。数として読めない動作のレコードは、範囲を与えると外れる。
   */
  eventActionFrom?: number;
  eventActionTo?: number;
  /**
   * 根拠のレコードを、この案件を付けた収集元のものに絞る。出ない場合はすべての案件を読む。
   * 取り込み結果に無い案件を与えた要求は handler が退ける
   * (`backend/api/request_items.go` の `checkKnownCase`)。
   */
  caseId?: CaseId;
  /**
   * 根拠のレコードを、レコードが名乗った端末に絞る。出ない場合は端末で絞らない。
   * 端末を名乗らないレコードは、端末を与えた要求の結果に入らない。
   */
  terminal?: NodeRef;
  /**
   * 根拠のレコードを、この収集元のどれかのものに絞る。出ない場合と空の並びは収集元で絞らない。
   * 取り込み結果に無い収集元を与えた要求は handler が退ける
   * (`backend/api/request_items.go` の `checkKnownSources`)。
   */
  sources?: readonly SourceRef[];
};

/** 収集元 1 件の参照。要求には sourceId を載せ、画面は file 名を出す。 */
export type SourceRef = {
  id: string;
  label: string;
};

/** ノード 1 つの参照。要求には識別子を載せ、画面は表示名を出す。 */
export type NodeRef = {
  /** ノードの識別子。操作 9 の応答が返した `id` をそのまま持つ。 */
  id: string;
  /** ノードの表示名。原資料の文字列である。 */
  label: string;
  /** ノードの種別。出ない場合は種別を知らない。 */
  kind?: NodeKind;
};

/**
 * 操作 9 の要求の項目。`depth` は既定値を持たない。
 *
 * **続きを取る位置を持たない。** 応答は絞り込んだ結果を全件返す。図に描くノードの数が
 * `nodeLimit` を超えるときは、合ったノードと件数だけを返す。
 */
export type GraphRequest = {
  /**
   * 候補を絞るのに用いる条件の選択。要求が必ず含む
   * (`backend/api/match_condition_request.go` の `readMatchConditions`)。
   */
  matchConditions: MatchConditionSelection;
  /** 図に描くノードの数の上限。1 以上の整数。出ない場合は上限を置かない。 */
  nodeLimit?: number;
  /**
   * 起点から広げるホップ数。`minGraphDepth` から `maxGraphDepth` までを受け取る
   * (`backend/api/graph_request.go` の `minDepth` と `maxDepth`)。
   */
  depth: number;
  /** ノードの種別。どれかに該当するノードが残る。出ない場合と空の集合は種別で絞らない。 */
  nodeKinds?: readonly NodeKind[];
  /**
   * レコードのノードを出すか、対象のノードだけを出すか。出ない場合は backend がレコードの
   * 粒度として扱う。
   */
  granularity?: GraphGranularity;
  /**
   * 近傍を展開する起点のノード。起点のどれかから広げた近傍の和を返す。値は応答が返した
   * `id` をそのまま返す。
   */
  nodeIds?: readonly string[];
  /** 関係の種別。どれかの種別の関係だけを辿る。出ない場合は種別で絞らない。 */
  edgeKinds?: readonly EdgeKind[];
  eventCategory?: string;
  eventAction?: string;
  /** 事象の動作を 10 進の数として比べる範囲の両端 (`RecordFilterCriteria`)。 */
  eventActionFrom?: number;
  eventActionTo?: number;
  /** 根拠のレコードを絞る案件。出ない場合はすべての案件を読む。 */
  caseId?: CaseId;
  /** 根拠のレコードを絞る端末のノードの識別子。 */
  terminal?: string;
  /** 根拠のレコードを絞る収集元の sourceId。どれかに該当するレコードが残る。 */
  sources?: readonly string[];
  /**
   * 真のとき、合致したレコードのノードに位置と時刻と事象の種別を添える
   * (`SubgraphNode.record`)。
   */
  recordSummary?: boolean;
  /**
   * IP アドレスのノードを、アドレスがこの範囲に入るものだけに絞る。
   * 文字列は先頭のアドレスと prefix の長さを `/` で繋いだ形である。
   */
  addressInCidr?: string;
  /** IP アドレスのノードを、アドレスがこの範囲に入らないものだけに絞る。 */
  addressNotInCidr?: string;
  /**
   * どの文字列も含む値を持つノードだけに絞る。対象の粒度では、判定はレコード 1 件ごとに行う。
   * 一致は大文字と小文字を区別しない (`backend/pipeline/graph_query.go` の matchedFormsOf)。
   */
  valueContains?: readonly string[];
  /** どの文字列も含まないノードだけに絞る。 */
  valueExcludes?: readonly string[];
  /**
   * `valueContains` と `valueExcludes` を照合する欄。出ない場合は全欄を照合する。文字列が 1 つも
   * 無い要求には載せない (handler は文字列を伴わない欄の指定を退ける)。
   */
  valueField?: string;
  /**
   * 欄と文字列の組 (`欄=文字列`)。どの組も、その欄に文字列を含むノードだけに絞る
   * (`backend/api/graph_request.go` の fieldContains)。
   */
  fieldContains?: readonly string[];
  /**
   * 完全一致の欄と文字列の組 (`欄=文字列`)。どの組も、その欄の値の全体が文字列と等しいノードだけに
   * 絞る (`backend/api/graph_request.go` の fieldEquals)。
   */
  fieldEquals?: readonly string[];
  /**
   * 欄・演算子・論理・括弧で書いた検索式。判定はレコード 1 件の欄に対して行う。
   * 空白だけの式は要求に載せない。
   */
  searchExpression?: string;
  /**
   * 値ごとに数える欄。語彙の項目の一覧に無い文字列は原資料の key を指す
   * (`backend/api/graph_request.go` の readGraphValueCount)。
   */
  countBy?: string;
  timeFilter?: GraphTimeFilter;
  /**
   * 真のとき、文字列と事象の種別の条件を起点のノードの判定だけに適用する。起点から辿るエッジは
   * 関係の種別で選ぶ (`backend/pipeline/graph_query.go` の `ConditionsOnOriginsOnly`)。
   */
  conditionsOnOriginsOnly?: boolean;
  /**
   * 真のとき、期間を与えた要求で、端点のレコードのノードが期間の外にあるエッジを辿らない。
   * 偽の値は、根拠のレコードの 1 件が期間の中にあるエッジを辿る。
   */
  endpointRecordsInPeriod?: boolean;
};

/** 操作 10 の要求の項目。属性と根拠を全件受け取る。 */
export type NodeDetailRequest = {
  /** ノードを指す不透明な識別子。値は操作 9 の応答が返した `id` である。 */
  id: string;
  /** 候補を絞るのに用いる条件の選択。要求が必ず含む。 */
  matchConditions: MatchConditionSelection;
};

/**
 * 操作 11 の要求の項目。絞り込みを通った根拠と関連付けを全件受け取る。
 *
 * `selector` は根拠の区分 1 つを指す。**応答が返した `selector` をそのまま渡す。**
 * 省いた要求はエッジの根拠の全数を返す。
 */
export type EdgeDetailRequest = {
  /** エッジを指す不透明な識別子。値は操作 9 の応答が返した `id` である。 */
  id: string;
  /** 候補を絞るのに用いる条件の選択。要求が必ず含む。 */
  matchConditions: MatchConditionSelection;
  selector?: EdgeEvidenceSelector;
  /** 根拠のレコードを絞る案件。出ない場合はすべての案件の根拠を返す。 */
  caseId?: CaseId;
};

/**
 * handler が受け取る `depth` の下限と上限
 * (`backend/api/graph_request.go` の `minDepth` と `maxDepth`)。
 */
export const minGraphDepth = 0;
export const maxGraphDepth = 8;

/** handler が受け取る起点のノードの数の上限 (`backend/api/graph_request.go` の `maxOriginNodes`)。 */
export const maxOriginNodes = 32;

const graphFailureSummary = "グラフの取得";
const nodeDetailFailureSummary = "ノードの詳細の取得";
const edgeDetailFailureSummary = "エッジの詳細の取得";
function rejectedResult<T>(summary: string): ApiResult<T> {
  return {
    ok: false,
    failure: buildFetchFailure("request_rejected", summary),
  };
}

/** GraphRequestがgraph APIで受理される局所的な値域かを確認する。 */
export function isValidGraphRequest(request: GraphRequest): boolean {
  return (
    Number.isSafeInteger(request.depth) &&
    request.depth >= minGraphDepth &&
    request.depth <= maxGraphDepth &&
    (request.nodeIds?.length ?? 0) <= maxOriginNodes &&
    (request.nodeLimit === undefined ||
      (Number.isSafeInteger(request.nodeLimit) && request.nodeLimit >= 1)) &&
    (request.timeFilter === undefined ||
      request.timeFilter.from !== undefined ||
      request.timeFilter.to !== undefined)
  );
}

/** graph queryをgraph APIと候補APIで同じfield名に写す。 */
export function graphRequestParams(
  request: GraphRequest,
): Record<string, string | string[] | undefined> {
  const timeFilter = request.timeFilter;
  return {
    nodeLimit: numberParam(request.nodeLimit),
    nodeKind: nonEmpty(request.nodeKinds),
    granularity: request.granularity,
    nodeId: nonEmpty(request.nodeIds),
    depth: String(request.depth),
    edgeKind: nonEmpty(request.edgeKinds),
    eventCategory: request.eventCategory,
    eventAction: request.eventAction,
    eventActionFrom: numberParam(request.eventActionFrom),
    eventActionTo: numberParam(request.eventActionTo),
    case: request.caseId,
    terminal: request.terminal,
    source: nonEmpty(request.sources),
    recordSummary: request.recordSummary ? "true" : undefined,
    addressInCidr: request.addressInCidr,
    addressNotInCidr: request.addressNotInCidr,
    valueContains: nonEmpty(request.valueContains),
    valueExcludes: nonEmpty(request.valueExcludes),
    valueField:
      (request.valueContains?.length ?? 0) +
        (request.valueExcludes?.length ?? 0) >
      0
        ? request.valueField
        : undefined,
    fieldContains: nonEmpty(request.fieldContains),
    fieldEquals: nonEmpty(request.fieldEquals),
    searchExpression: expressionParam(request.searchExpression),
    countBy: request.countBy,
    conditionsOnOriginsOnly: request.conditionsOnOriginsOnly
      ? "true"
      : undefined,
    endpointRecordsInPeriod: request.endpointRecordsInPeriod
      ? "true"
      : undefined,
    timeFrom: timeFilter?.from?.text,
    timeFromPrecision: timeFilter?.from?.precision,
    timeTo: timeFilter?.to?.text,
    timeToPrecision: timeFilter?.to?.precision,
    filterUnit: timeFilter === undefined ? undefined : timeFilter.unit,
    matchCondition: matchConditionParams(request.matchConditions),
  };
}

/** 数の項目を 10 進の文字列にする。出ない値は項目を載せない。 */
export function numberParam(value: number | undefined): string | undefined {
  return value === undefined ? undefined : String(value);
}

/**
 * 検索式の項目の文字列を返す。空白だけの式は項目を載せない。handler は空の文字列を退ける。
 * 文字列は前後の空白を含めて与えられたまま送る。応答の誤りの位置は、送った文字列の上で数える。
 */
export function expressionParam(text: string | undefined): string | undefined {
  return text === undefined || text.trim() === "" ? undefined : text;
}

/**
 * 繰り返す項目の値を複製して返す。空の集合は項目を載せない。handler は空の文字列を退けるため、
 * 空の集合を空の文字列 1 つとして送らない。
 */
export function nonEmpty<T extends string>(
  values: readonly T[] | undefined,
): T[] | undefined {
  return values === undefined || values.length === 0 ? undefined : [...values];
}

/**
 * 操作 9 (`GET /api/v0/graph`) を実行する。
 * handler が失敗を返す要求 (`depth` が受け取る範囲の外、負または整数でない展開の度合い、
 * 時刻の文字列が無い期間の絞り込み) を送らずに失敗として返す。
 */
export async function fetchGraph(
  request: GraphRequest,
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<GraphResponse>> {
  if (!isValidGraphRequest(request)) {
    return rejectedResult(graphFailureSummary);
  }
  return requestJson({
    path: "/graph",
    searchParams: graphRequestParams(request),
    decode: decodeGraphResponse,
    failureSummary: graphFailureSummary,
    signal: options.signal,
  });
}

/**
 * 操作 10 (`GET /api/v0/nodes/{id}`) を実行する。
 * ノードの識別子は path の一部であり、path の区切りと衝突しない形へ符号化して送る。
 */
export async function fetchNodeDetail(
  request: NodeDetailRequest,
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<NodeDetailResponse>> {
  if (request.id === "") {
    return rejectedResult(nodeDetailFailureSummary);
  }
  return requestJson({
    path: `/nodes/${encodeURIComponent(request.id)}`,
    searchParams: {
      matchCondition: matchConditionParams(request.matchConditions),
    },
    decode: decodeNodeDetailResponse,
    failureSummary: nodeDetailFailureSummary,
    signal: options.signal,
  });
}

/** 関係の相手側の欄の値ごとの件数の要求の項目。 */
export type NodeValueCountsRequest = {
  id: string;
  edgeKind: EdgeKind;
  direction: EdgeDirection;
  /** 数える欄。語彙の項目か原資料の key である。 */
  countBy: string;
  matchConditions: MatchConditionSelection;
};

const nodeValueCountsFailureSummary = "隣接ノードのフィールドの値の件数の取得";

/** 隣接ノードのフィールドの値ごとの件数 (`GET /api/v0/nodes/{id}/value-counts`) を取得する。 */
export async function fetchNodeValueCounts(
  request: NodeValueCountsRequest,
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<NodeValueCountsResponse>> {
  if (request.id === "" || request.countBy.trim() === "") {
    return rejectedResult(nodeValueCountsFailureSummary);
  }
  return requestJson({
    path: `/nodes/${encodeURIComponent(request.id)}/value-counts`,
    searchParams: {
      edgeKind: request.edgeKind,
      direction: request.direction,
      countBy: request.countBy.trim(),
      matchCondition: matchConditionParams(request.matchConditions),
    },
    decode: decodeNodeValueCountsResponse,
    failureSummary: nodeValueCountsFailureSummary,
    signal: options.signal,
  });
}

/** URL の断片をつなぐ要求の項目。 */
export type EdgeUrlFragmentsRequest = {
  id: string;
  caseId?: CaseId;
  matchConditions: MatchConditionSelection;
};

export const edgeUrlFragmentsFailureSummary = "URL の断片の連結";

/** 関係の根拠の URL の断片をつないだ結果 (`GET /api/v0/edges/{id}/url-fragments`) を取得する。 */
export async function fetchEdgeUrlFragments(
  request: EdgeUrlFragmentsRequest,
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<UrlFragmentJoin>> {
  if (request.id === "") {
    return rejectedResult(edgeUrlFragmentsFailureSummary);
  }
  return requestJson({
    path: `/edges/${encodeURIComponent(request.id)}/url-fragments`,
    searchParams: {
      case: request.caseId,
      matchCondition: matchConditionParams(request.matchConditions),
    },
    decode: decodeUrlFragmentJoin,
    failureSummary: edgeUrlFragmentsFailureSummary,
    signal: options.signal,
  });
}

/**
 * 操作 11 (`GET /api/v0/edges/{id}`) を実行する。
 *
 * 区分を指す値は応答が返した組をそのまま項目へ写す。接続先 port とログオンの種別の
 * それぞれで、値と欄の不在を指す項目は排他であり、handler が両方を持つ要求を退ける。
 */
export async function fetchEdgeDetail(
  request: EdgeDetailRequest,
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<EdgeDetailResponse>> {
  if (request.id === "") {
    return rejectedResult(edgeDetailFailureSummary);
  }
  const selector = request.selector;
  return requestJson({
    path: `/edges/${encodeURIComponent(request.id)}`,
    searchParams: {
      // 観測の種別を持たない HTTP の要求の区分は、空の文字列を載せない。handler は空の文字列を退ける。
      eventCategory: selector?.eventCategory || undefined,
      eventAction: selector?.eventAction || undefined,
      destinationPort: selector?.destinationPort,
      destinationPortAbsent:
        selector?.destinationPortAbsent === true ? "true" : undefined,
      logonType: selector?.logonType,
      logonTypeAbsent: selector?.logonTypeAbsent === true ? "true" : undefined,
      httpStatus: selector?.httpStatus,
      httpStatusAbsent:
        selector?.httpStatusAbsent === true ? "true" : undefined,
      case: request.caseId,
      matchCondition: matchConditionParams(request.matchConditions),
    },
    decode: decodeEdgeDetailResponse,
    failureSummary: edgeDetailFailureSummary,
    signal: options.signal,
  });
}
