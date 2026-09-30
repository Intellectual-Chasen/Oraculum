package api

import (
	"net/http"
	"slices"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const timelinePattern = "GET /api/v0/timeline"

// timelineHandler は根拠のレコードを時刻順に返す。
type timelineHandler struct {
	graph pipeline.Graph
	// result は、原文から文字列を探す要求が行の原文を読むのに使う。
	result pipeline.ImportResult
}

// timelineResponse は時系列の操作の応答である。
// **本型が項目の定義元である。**
//
// **絞り込んだ結果を全件返す。** 上限も続きを取る位置も持たない。
//
// **原資料の値を持たない。** 行が指すのは根拠のレコードの位置であり、値を読むのは
// `/api/v0/records` である。
type timelineResponse struct {
	// Entries は事象の時刻の昇順に並んだ行である。
	//
	// **UTC からのずれの決まらない地方時の行は、時点を持つ行の後ろに並ぶ。** その行は収集元ごとに
	// まとまり、収集元の中では地方時の文字列の順に並ぶ。分析者が収集元の時刻の解釈を記録すると、
	// その収集元の行は解釈のずれで読んだ時点で時点を持つ行に入る。期間を指定した要求では、
	// 時点を持たない地方時の行は期間の判定を通らず、並ばない。
	Entries []core.TimelineEntry `json:"entries"`
	// EntryCount は行の件数である。len(Entries) と等しい。
	EntryCount int64 `json:"entryCount"`
	// UndatedRecordCount は、絞り込みを通りながら比較に用いる時刻も地方時の文字列も持たない
	// レコードの件数である。**この件数のレコードは Entries に入らない。** ずれの決まらない
	// 地方時のレコードはこの件数に入らず、Entries の末尾に並ぶ。
	UndatedRecordCount int64 `json:"undatedRecordCount"`
	// PeriodUnjudged は、期間を指定した要求で、期間のほかの絞り込みを通りながら時点を
	// 持たないため期間の判定から外れ、Entries に入らなかったレコードの件数である。期間を
	// 指定しない要求では出ない。
	PeriodUnjudged *periodUnjudged `json:"periodUnjudged,omitempty"`
	// SourceCoverages は収集元ごとの収録範囲と、要求の期間との重なり方である。
	// 並びは取り込みの入力順である。
	SourceCoverages []core.SourceCoverage `json:"sourceCoverages"`
	// EventCategory から FilterUnit までは、要求が与えた絞り込みの条件をそのまま返す。
	// 要求が省略した項目は応答にも出ない。
	EventCategory   string              `json:"eventCategory,omitempty"`
	EventAction     string              `json:"eventAction,omitempty"`
	EventActionFrom *uint64             `json:"eventActionFrom,omitempty"`
	EventActionTo   *uint64             `json:"eventActionTo,omitempty"`
	Case            string              `json:"case,omitempty"`
	Terminal        string              `json:"terminal,omitempty"`
	TimeFrom        *core.RequestedTime `json:"timeFrom,omitempty"`
	TimeTo          *core.RequestedTime `json:"timeTo,omitempty"`
	FilterUnit      core.FilterUnit     `json:"filterUnit,omitempty"`
	// SearchExpression は要求が与えた検索式の文字列である。利用者が入れた文字列をそのまま返す。
	SearchExpression string `json:"searchExpression,omitempty"`
	// Sources は要求が与えた、根拠のレコードを絞る収集元の sourceId である。要求に書いた順に並ぶ。
	Sources []string `json:"source,omitempty"`
	// NodeIds と Depth は、要求が与えた起点のノードと段数をそのまま返す。起点の無い要求では
	// 応答にも出ない。
	NodeIds []string `json:"nodeIds,omitempty"`
	Depth   *int     `json:"depth,omitempty"`
	// AccountNodeId は役割付きで指されたアカウントのノードである。
	AccountNodeId string `json:"accountNodeId,omitempty"`
	// FindMatches は、要求の find を原文に含む行の Entries での位置である。昇順に並ぶ。
	// 原文を収集元の byte 列から組み立てた収集元の行は、欄の名前と値に含むときも入る
	// (findInRawTexts)。
	// find を与えない要求では応答に出ず、与えて 1 行も含まないときは空の配列である。
	FindMatches *[]int `json:"findMatches,omitempty"`
	// EmptyReason は行が 0 件になった理由である。出る条件は EntryCount が 0 のときで
	// ある。
	//
	// **時刻も地方時の文字列も持たないレコードだけが絞り込みを通った応答にも出る。** その応答の
	// UndatedRecordCount は 0 より大きく、分析者は 2 つを並べて読む。
	EmptyReason core.EmptyReason `json:"emptyReason,omitempty"`
}

func (h timelineHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	request, apiError := parseTimelineRequest(r)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	if apiError := checkKnownRecordConditions(h.graph, request.records.conditions); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	if apiError := checkKnownNodes(h.graph, request.nodeIds); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	if request.accountNodeId != "" {
		if apiError := checkKnownNodesForParameter(
			h.graph, []string{request.accountNodeId}, accountNodeIdParam,
		); apiError != nil {
			writeError(w, httpStatusFor(apiError.Code), *apiError)
			return
		}
		detail, _ := h.graph.NodeDetail(request.accountNodeId)
		if detail.Node.Kind != core.NodeKindAccount {
			writeError(w, http.StatusBadRequest, core.ApiError{Code: core.ApiErrorCodeInvalidRequest, Message: "accountNodeId must name an account"})
			return
		}
	}
	response := buildTimelineResponse(request, h.graph.Timeline(request.query()))
	if request.find != "" {
		matches := findInRawTexts(h.result, response.Entries, request.find, request.findCaseSensitive)
		response.FindMatches = &matches
	}
	writeJSON(w, http.StatusOK, response)
}

// findInRawTexts は、原文に find を含む行の位置を昇順に返す。原文を読めない行は含まない。
//
// **原文を収集元の byte 列から組み立てた収集元 (registry の hive、Prefetch など) の行は、欄の
// 名前と、欄の原資料の文字列と正規化値も探す。** その原文は 16 進の表記であり、値の文字列を
// 含まない。
//
// ponytail: 大文字と小文字を区別しない比較は strings.ToLower で両方を小文字にして比べる。
// 小文字にすると長さの変わる文字は位置がずれるが、返すのは含むかどうかだけである。
func findInRawTexts(
	result pipeline.ImportResult, entries []core.TimelineEntry, find string, caseSensitive bool,
) []int {
	if !caseSensitive {
		find = strings.ToLower(find)
	}
	contains := func(text string) bool {
		if !caseSensitive {
			text = strings.ToLower(text)
		}
		return strings.Contains(text, find)
	}
	refs := make(map[string]bool, len(entries))
	for _, entry := range entries {
		refs[entry.RecordRef.RecordRawTextRef] = true
	}
	fieldTexts := result.ConvertedFieldTexts(refs)
	matches := []int{}
	for index, entry := range entries {
		ref := entry.RecordRef.RecordRawTextRef
		text, found := result.RawText(ref)
		if !found {
			continue
		}
		if contains(text) || slices.ContainsFunc(fieldTexts[ref], contains) {
			matches = append(matches, index)
		}
	}
	return matches
}

// periodUnjudged は期間の判定から外れたレコードの件数を理由ごとに持つ
// (pipeline.Timeline.PeriodUnjudgedLocalRecordCount)。
type periodUnjudged struct {
	// LocalRecordCount は UTC からのずれが決まらない地方時の文字列を持つレコードの件数である。
	LocalRecordCount int64 `json:"localRecordCount"`
	// UndatedRecordCount は時刻を持たないレコードの件数である。
	UndatedRecordCount int64 `json:"undatedRecordCount"`
}

// buildTimelineResponse は時系列を応答の組へ直す。
func buildTimelineResponse(
	request timelineRequest, timeline pipeline.Timeline,
) timelineResponse {
	conditions := request.records.conditions
	response := timelineResponse{
		Entries:            timeline.Entries,
		EntryCount:         int64(len(timeline.Entries)),
		UndatedRecordCount: timeline.UndatedRecordCount,
		SourceCoverages:    timeline.SourceCoverages,
		EventCategory:      conditions.EventCategory,
		EventAction:        conditions.EventAction,
		EventActionFrom:    conditions.EventActionFrom,
		EventActionTo:      conditions.EventActionTo,
		Case:               conditions.Case,
		Terminal:           conditions.Terminal,
		FilterUnit:         conditions.FilterUnit,
		SearchExpression:   request.searchExpressionText,
		Sources:            conditions.Sources,
	}
	response.TimeFrom, response.TimeTo = request.records.requestedTimes()
	if response.TimeFrom != nil || response.TimeTo != nil {
		response.PeriodUnjudged = &periodUnjudged{
			LocalRecordCount:   timeline.PeriodUnjudgedLocalRecordCount,
			UndatedRecordCount: timeline.PeriodUnjudgedUndatedRecordCount,
		}
	}
	if len(request.nodeIds) > 0 {
		response.NodeIds = request.nodeIds
		response.Depth = &request.depth
	}
	response.AccountNodeId = request.accountNodeId
	if response.EntryCount == 0 {
		response.EmptyReason = core.EmptyReasonNoRecordInFilter
	}
	return response
}
