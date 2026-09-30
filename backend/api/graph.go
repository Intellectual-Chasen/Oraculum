package api

import (
	"net/http"
	"slices"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const graphPattern = "GET /api/v0/graph"

// graphHandler は部分グラフを返す。
//
// グラフは handler 1 つにつき 1 回だけ組む。組み直すたびに取り込み結果の全レコードを
// 走査するためである (pipeline.NewGraph)。
type graphHandler struct {
	result pipeline.ImportResult
	graph  pipeline.Graph
}

// graphResponse は `/api/v0/graph` の応答である。
// **本型が項目の定義元である。**
//
// **絞り込んだ結果を全件返す。** 続きを取る位置を持たない。
//
// **部分グラフのノードの数 (subgraphNodeCount) が要求の nodeLimit を超えるときは、図の本体を
// 返さない。** nodes は合ったノードだけを持ち、edges は空であり、nodeLimitExceeded が真になり、
// edgeKindCounts が辿ったエッジの本数を関係の種別ごとに持つ。
//
// **edges の両端は必ず nodes にある。** 絞り込みに合わない端点は selection が
// edge_endpoint の要素として入る。
//
// **根拠の中身を持たない。** 図は根拠を描かない。根拠はノードまたはエッジを選んだときに
// `/api/v0/nodes/{id}` と `/api/v0/edges/{id}` が返す。
type graphResponse struct {
	Nodes     []core.SubgraphNode `json:"nodes"`
	NodeCount int64               `json:"nodeCount"`
	Edges     []core.GraphEdge    `json:"edges"`
	EdgeCount int64               `json:"edgeCount"`
	// SubgraphNodeCount は部分グラフのノードの数である。合ったノードと、辿ったエッジの端点の和である。
	// nodeLimitExceeded が出ないときは nodes の要素の数と等しい。
	SubgraphNodeCount int64 `json:"subgraphNodeCount"`
	// NodeLimit は要求が与えたノードの数の上限である。
	NodeLimit int `json:"nodeLimit,omitempty"`
	// NodeLimitExceeded は、subgraphNodeCount が nodeLimit を超えたときだけ真で出る。
	NodeLimitExceeded bool `json:"nodeLimitExceeded,omitempty"`
	// EdgeKindCounts は、nodeLimitExceeded が出るときだけ、辿ったエッジの本数を関係の種別ごとに持つ。
	// 種別の昇順に並ぶ。分析者が関係の種別で絞るための件数である。
	//
	// **辿ったエッジが 0 本でも、nodeLimitExceeded が出るときは空の配列で出す。** omitzero は nil だけを
	// 省く。pipeline は上限を超えたときに nil でない slice を返す。
	EdgeKindCounts []core.EdgeKindCount `json:"edgeKindCounts,omitzero"`
	// MatchedKinds は、絞り込みに合うノードの種別ごとの件数である。種別の昇順に並ぶ。
	// 件数の和は NodeCount と等しい。
	MatchedKinds []core.MatchedKind `json:"matchedKinds"`
	// MatchedAccountIdentityPairCount は、絞り込みに合うアカウントのうち、同じアカウントの
	// 候補で結ばれた SID と名前のノードの組の数である。MatchedKinds はその組の 2 つの
	// ノードを別に数える。組が無いときは出ない
	// (pipeline.Subgraph.MatchedAccountIdentityPairCount)。
	MatchedAccountIdentityPairCount int64 `json:"matchedAccountIdentityPairCount,omitempty"`
	// NodeKinds から FilterUnit までは、要求が与えた絞り込みの条件をそのまま返す。
	// 要求が省略した項目は応答にも出ない。
	NodeKinds     []core.NodeKind       `json:"nodeKinds,omitempty"`
	Granularity   core.GraphGranularity `json:"granularity,omitempty"`
	NodeIds       []string              `json:"nodeIds,omitempty"`
	Depth         int                   `json:"depth"`
	EdgeKinds     []core.EdgeKind       `json:"edgeKinds,omitempty"`
	EventCategory string                `json:"eventCategory,omitempty"`
	EventAction   string                `json:"eventAction,omitempty"`
	// EventActionFrom と EventActionTo は要求が与えた動作の範囲の両端である。
	EventActionFrom *uint64 `json:"eventActionFrom,omitempty"`
	EventActionTo   *uint64 `json:"eventActionTo,omitempty"`
	// ValueField は要求が与えた、文字列を当てる欄の指定である。
	ValueField string `json:"valueField,omitempty"`
	// FieldContains は要求が与えた欄と文字列の組の文字列 (`欄=文字列`) である。要求に書いた順に並ぶ。
	FieldContains []string `json:"fieldContains,omitempty"`
	// FieldEquals は要求が与えた完全一致の欄と文字列の組の文字列 (`欄=文字列`) である。要求に
	// 書いた順に並ぶ。
	FieldEquals []string `json:"fieldEquals,omitempty"`
	// Case は要求が与えた案件である。
	Case string `json:"case,omitempty"`
	// Terminal は要求が与えた端末のノードの識別子である。
	Terminal string `json:"terminal,omitempty"`
	// AddressInCidr と AddressNotInCidr は要求が与えたアドレスの範囲の文字列である。
	AddressInCidr    string `json:"addressInCidr,omitempty"`
	AddressNotInCidr string `json:"addressNotInCidr,omitempty"`
	// ValueContains と ValueExcludes は要求が与えた検索の文字列である。要求に書いた順に並ぶ。
	//
	// **利用者が入れた文字列をそのまま返す。** 応答を読む画面が、表示の境界で無害化する。
	ValueContains []string `json:"valueContains,omitempty"`
	ValueExcludes []string `json:"valueExcludes,omitempty"`
	// SearchExpression は要求が与えた検索式の文字列である。利用者が入れた文字列をそのまま返す。
	SearchExpression string `json:"searchExpression,omitempty"`
	// ConditionsOnOriginsOnly は、文字列と事象の種別の条件を起点の判定だけに当てたかである。
	// 真のとき、エッジの根拠の件数はそれらの条件の外のレコードも数える。
	ConditionsOnOriginsOnly bool `json:"conditionsOnOriginsOnly,omitempty"`
	// EndpointRecordsInPeriod は、端点のレコードが期間の外にあるエッジを辿らなかったかである。
	EndpointRecordsInPeriod bool `json:"endpointRecordsInPeriod,omitempty"`
	// CountBy は要求が与えた、値ごとに数える欄の指定である。
	CountBy string `json:"countBy,omitempty"`
	// ValueCounts は数える欄を与えた要求が返す、値ごとの件数である。
	//
	// **数える範囲は絞り込みに合うノードの全件である。**
	ValueCounts []core.ValueCount `json:"valueCounts,omitempty"`
	// DistinctValueCount は値の異なりの個数である。len(ValueCounts) と等しい。
	//
	// **数えていない状態と 0 件を分ける。** 数える欄を与えない要求では出ない。
	// 与えた要求では 0 件でも出る。
	DistinctValueCount *int64 `json:"distinctValueCount,omitempty"`
	// ValueCountsEmptyReason は値ごとの件数が 0 件になった理由である。
	// 出る条件は DistinctValueCount が 0 のときである。
	ValueCountsEmptyReason core.EmptyReason `json:"valueCountsEmptyReason,omitempty"`
	// ValueCountsNodeKinds は、数える欄の値を持つノードの種別である。種別の昇順に並ぶ。
	// 出る条件は ValueCountsEmptyReason が field_on_other_node_kind のときである。
	ValueCountsNodeKinds []core.NodeKind `json:"valueCountsNodeKinds,omitempty"`
	// Sources は要求が与えた、根拠のレコードを絞る収集元の sourceId である。要求に書いた順に並ぶ。
	Sources     []string            `json:"source,omitempty"`
	TimeFrom    *core.RequestedTime `json:"timeFrom,omitempty"`
	TimeTo      *core.RequestedTime `json:"timeTo,omitempty"`
	FilterUnit  core.FilterUnit     `json:"filterUnit,omitempty"`
	EmptyReason core.EmptyReason    `json:"emptyReason,omitempty"`
	// UnknownFields は、検索式の比較が指した欄のうち、どのノードの属性にも無い欄の名前である。
	// 式に書いた順に並び、重複を持たない。該当する欄が無いときは出ない。
	UnknownFields []string `json:"unknownFields,omitempty"`
	// UnknownFieldsHint は、UnknownFields が出るときに、書ける欄の名前の一覧を返す操作を示す
	// 英語の文である。それ以外のときは出ない。
	UnknownFieldsHint string `json:"unknownFieldsHint,omitempty"`
}

// observedFieldsHint は、欄の名前の一覧を返す操作を示す文である。
const observedFieldsHint = "observed field names for searchExpression, fieldEquals, fieldContains and countBy: " +
	"semanticFields and fieldNames of GET /api/v0/event-kinds"

func (h graphHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	request, apiError := parseGraphRequest(r)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	if apiError := checkKnownSearch(h.graph, request.search); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	writeJSON(w, http.StatusOK, h.buildResponse(request, h.graph.Query(request.query())))
}

// buildResponse は部分グラフを応答の組へ直す。
func (h graphHandler) buildResponse(
	request graphRequest, subgraph pipeline.Subgraph,
) graphResponse {
	search := request.search
	response := graphResponse{
		Nodes: subgraph.Nodes, NodeCount: int64(subgraph.MatchedNodeCount),
		Edges: subgraph.Edges, EdgeCount: int64(subgraph.EdgeCount),
		SubgraphNodeCount: int64(subgraph.SubgraphNodeCount), NodeLimit: request.nodeLimit,
		NodeLimitExceeded: subgraph.NodeLimitExceeded, EdgeKindCounts: subgraph.EdgeKindCounts,
		MatchedKinds:                    subgraph.MatchedKinds,
		MatchedAccountIdentityPairCount: subgraph.MatchedAccountIdentityPairCount,
		NodeKinds:                       search.NodeKinds, Granularity: search.Granularity,
		NodeIds: search.NodeIds, Depth: search.Depth, EdgeKinds: search.EdgeKinds,
		EventCategory: search.EventCategory, EventAction: search.EventAction,
		EventActionFrom: search.EventActionFrom, EventActionTo: search.EventActionTo,
		ValueField:    search.ValueField,
		FieldContains: search.FieldContains, FieldEquals: search.FieldEquals,
		Case: search.Case, Terminal: search.Terminal,
		AddressInCidr: search.AddressInCidr, AddressNotInCidr: search.AddressNotInCidr,
		ValueContains: search.ValueContains, ValueExcludes: search.ValueExcludes,
		SearchExpression: search.SearchExpression,
		CountBy:          search.CountBy,
		FilterUnit:       search.FilterUnit,
		Sources:          search.Sources,

		ConditionsOnOriginsOnly: search.ConditionsOnOriginsOnly,
		EndpointRecordsInPeriod: search.EndpointRecordsInPeriod,
	}
	h.addValueCounts(&response, request, subgraph)
	response.TimeFrom, response.TimeTo = request.records.requestedTimes()
	if response.NodeCount == 0 {
		response.EmptyReason = h.graphEmptyReasonOf(request)
	}
	response.UnknownFields = h.unknownFieldsOf(request)
	if response.UnknownFields != nil {
		response.UnknownFieldsHint = observedFieldsHint
	}
	return response
}

// unknownFieldsOf は、検索式の比較が指した欄のうち、どのノードの属性にも無い欄の名前を返す。
func (h graphHandler) unknownFieldsOf(request graphRequest) []string {
	if request.searchExpression == nil {
		return nil
	}
	var unknown []string
	for _, field := range request.searchExpression.ComparedFields() {
		name := string(field.Semantic) + field.Name
		if !slices.Contains(unknown, name) && !h.graph.ObservesField(field.Semantic, field.Name) {
			unknown = append(unknown, name)
		}
	}
	return unknown
}

// addValueCounts は、数える欄を与えた要求の応答へ値ごとの件数を足す。
func (h graphHandler) addValueCounts(
	response *graphResponse, request graphRequest, subgraph pipeline.Subgraph,
) {
	if request.search.CountBy == "" {
		return
	}
	distinct := int64(subgraph.DistinctValueCount)
	response.ValueCounts = subgraph.ValueCounts
	response.DistinctValueCount = &distinct
	// **欄の名前の打ち間違いと、値の不在と、対象の種別の選び方と、絞り込みで残らなかった
	// 状態を、走査して分ける。** 打ち間違いを「その欄には値が無い」と同じ応答にすると、
	// 分析者は綴りを疑う手掛かりを失う。走査するのは値の異なりが 0 件の応答のときだけである。
	if distinct == 0 {
		query := request.query()
		response.ValueCountsEmptyReason, response.ValueCountsNodeKinds =
			h.graph.CountedFieldObservationOf(query).EmptyReason(query)
	}
}

// graphEmptyReasonOf は、合致したノードが 0 件になった理由を返す。
//
// **検索の文字列が 1 つも無い状態と、他の絞り込みで残らなかった状態を、走査して分ける。**
// 分析者が次に採る手が異なる。前者は文字列を見直す手掛かりであり、後者は絞り込みを緩める
// 手掛かりである。**確かめずに「どこにも無い」を名乗らない。** それは調査の結論であり、
// 応答が主張してよい内容ではない。
//
// 走査するのは 0 件の応答のときだけである (呼び出し元の判定)。
func (h graphHandler) graphEmptyReasonOf(request graphRequest) core.EmptyReason {
	query := request.query()
	// 合うレコードが対象を指さない状態を、合うレコードが無い状態と分ける。前者はレコードの
	// 粒度か時系列で見れば、合ったレコードを読める。
	if h.graph.HasRecordMatchWithoutObject(query) {
		return core.EmptyReasonRecordMatchWithoutObject
	}
	if !query.SearchesText() {
		return core.EmptyReasonNoRecordInFilter
	}
	// 欄の名前の打ち間違いを、文字列が無い状態と分ける。
	if (query.ValueFieldSemantic != "" || query.ValueFieldName != "") &&
		!h.graph.ObservesField(query.ValueFieldSemantic, query.ValueFieldName) {
		return core.EmptyReasonNoFieldObserved
	}
	for _, term := range query.FieldContains {
		if !h.graph.ObservesField(term.Semantic, term.Name) {
			return core.EmptyReasonNoFieldObserved
		}
	}
	if h.graph.HasValueMatchAnywhere(query) {
		return core.EmptyReasonValueMatchOutsideFilter
	}
	return core.EmptyReasonNoValueMatch
}
