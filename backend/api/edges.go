package api

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/output"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const edgesPattern = "GET /api/v0/edges/{id}"

// `/api/v0/edges/{id}` の要求の項目の名前。id は path が持つ。
// eventCategory と eventAction は`/api/v0/nodes/{id}` と同じ名前で、同じ観測の種別の欄を指す。
const (
	edgePathValue              = "id"
	destinationPortParam       = "destinationPort"
	destinationPortAbsentParam = "destinationPortAbsent"
	logonTypeParam             = "logonType"
	logonTypeAbsentParam       = "logonTypeAbsent"
	httpStatusParam            = "httpStatus"
	httpStatusAbsentParam      = "httpStatusAbsent"
)

// absentFlagValue は、欄を持たないレコードを指す項目 (destinationPortAbsent と
// logonTypeAbsent と httpStatusAbsent) が取る文字列である。
const absentFlagValue = "true"

// edgesHandler はエッジ 1 本の根拠と条件と前提を返す。
type edgesHandler struct {
	result pipeline.ImportResult
	graph  pipeline.Graph
}

// writeEdgeResponse は`/api/v0/edges/{id}` の応答を、項目ごとに書く。
//
// 応答は次の項目を持つ object である。
//   - edge: エッジと、絞り込みを通した根拠 (edgeDetailEdge)
//   - sourceNode, targetNode: 両端のノード。画面がノードの表示名を探すために別の要求を出さない
//   - matchRecords, matchStages, matches: エッジを作った関連付け (core.EdgeMatchTable)
//   - matchCount: 関連付けの総数
//   - assignmentBases: 候補のエッジの成立の根拠。IP から端末への割当で作ったエッジだけが
//     持ち、用いた割当ごとに 1 件を持つ。他のエッジでは出ない
//   - terminalAssignments: エッジを作った端末の割当。利用者が収集元に付けた割当の IP から
//     作った terminal_address のエッジだけが持つ。他のエッジでは出ない
//   - observedInRecords: terminalAssignments を持つエッジを、レコードも直に観測したか。
//     terminalAssignments と同時に出る
//   - recordPairs, recordPairCount: 観測の層が候補のエッジを作ったレコードの組と、成立に
//     用いた条件ごとの両側の値 (core.EdgeRecordPair)。組を持つエッジだけが持つ。recordPairs は
//     先頭から上限までの組、recordPairCount は組の総数である
//   - evidenceGroups: 根拠を観測の種別と接続先 port とログオンの種別で分けた区分。**絞り込みの有無に
//     かかわらずエッジの根拠の全数から組む。**
//
// 絞り込んだ根拠と関連付けを全件返す。上限も続きを取る位置も持たない。**関連付けの表を要素ごとに
// 書き、応答の全体を memory に組まない。** 関連付けを 50 万件持つエッジがある。
func writeEdgeResponse(w http.ResponseWriter, detail pipeline.EdgeDetail) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	stream := output.NewJSONStream(w)
	stream.Token(`{"edge":`)
	stream.Value(edgeDetailEdge{GraphEdge: detail.Edge, Evidence: emptyIfNil(detail.Evidence)})
	stream.Token(`,"sourceNode":`)
	stream.Value(detail.SourceNode)
	stream.Token(`,"targetNode":`)
	stream.Value(detail.TargetNode)
	stream.Token(`,"matchRecords":`)
	output.WriteJSONArray(stream, detail.MatchTable.MatchRecords)
	stream.Token(`,"matchStages":`)
	output.WriteJSONArray(stream, detail.MatchTable.MatchStages)
	stream.Token(`,"matches":`)
	output.WriteJSONArray(stream, detail.MatchTable.Matches)
	stream.Token(`,"matchCount":`)
	stream.Value(int64(detail.MatchCount))
	if len(detail.AssignmentBases) > 0 {
		stream.Token(`,"assignmentBases":`)
		stream.Value(detail.AssignmentBases)
	}
	if len(detail.TerminalAssignments) > 0 {
		stream.Token(`,"terminalAssignments":`)
		stream.Value(detail.TerminalAssignments)
		stream.Token(`,"observedInRecords":`)
		stream.Value(detail.ObservedInRecords)
	}
	if detail.RecordPairCount > 0 {
		stream.Token(`,"recordPairs":`)
		output.WriteJSONArray(stream, detail.RecordPairs)
		stream.Token(`,"recordPairCount":`)
		stream.Value(int64(detail.RecordPairCount))
	}
	stream.Token(`,"evidenceGroups":`)
	stream.Value(emptyIfNil(detail.EvidenceGroups))
	stream.Token("}")
	if err := stream.End(); err != nil {
		// status line と header は送信済みで、応答の形を変える手段が残っていない。
		// 本体が途中で切れた事象を記録し、接続を切って client に転送の失敗として伝える。
		// 切らないと、閉じていない JSON が完結した 200 の応答として届く。
		// err は外部由来の文字列を持たない。
		slog.Error("writing API response body failed", "status", http.StatusOK, "error", err)
		panic(http.ErrAbortHandler)
	}
}

// edgeDetailEdge はエッジ 1 本と、`/api/v0/edges/{id}` だけが持つ根拠である。
//
// `/api/v0/graph` は図を組む材料だけを返し、根拠の中身を持たない。根拠を持つのはこの操作である。
// 置き場を分けるのは、複数の操作が受け渡す項目を core に、1 つの操作だけが返す項目を api に
// 置く規約による。
//
// **core.GraphEdge に MarshalJSON を足すと、埋め込みの promote が止まりこの包みが壊れる。**
type edgeDetailEdge struct {
	core.GraphEdge
	// Evidence は根拠のレコードである。絞り込みを通した全件を持つ。
	Evidence []core.GraphEvidence `json:"evidence"`
}

// edgeRequest は`/api/v0/edges/{id}` の要求の項目である。
type edgeRequest struct {
	id string
	// filter は根拠を 1 つの区分へ絞る条件である。指定の無い要求では条件を持たない。
	filter pipeline.EdgeEvidenceFilter
}

func (h edgesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	request, apiError := parseEdgeRequest(r)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	if apiError := checkKnownCase(h.graph, request.filter.Case); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	detail, found := h.graph.EdgeDetail(request.id, request.filter)
	if !found {
		writeError(w, http.StatusNotFound, core.ApiError{
			Code:    core.ApiErrorCodeRecordNotFound,
			Message: "no edge matches the requested id",
		})
		return
	}
	writeEdgeResponse(w, detail)
}

// parseEdgeRequest は`/api/v0/edges/{id}` の要求を読む。
func parseEdgeRequest(r *http.Request) (edgeRequest, *core.ApiError) {
	query := r.URL.Query()
	if err := checkEdgeParameterNames(query); err != nil {
		return edgeRequest{}, invalidRequestError(err, nil)
	}
	filter, problem := parseEdgeEvidenceFilter(query)
	if problem != nil {
		return edgeRequest{}, problem
	}
	request := edgeRequest{id: r.PathValue(edgePathValue), filter: filter}
	if request.id == "" {
		return edgeRequest{}, invalidRequestError(errors.New("id must not be empty"), nil)
	}
	return request, nil
}

// parseEdgeEvidenceFilter は根拠を 1 つの区分へ絞る条件を読む。
//
// **接続先 port とログオンの種別のそれぞれで、値と、欄の不在を指す条件は排他である。**
// 2 つを同時に与えた要求は、通るレコードが 1 件も無い条件であり、書き方の不備として退ける。
func parseEdgeEvidenceFilter(query url.Values) (pipeline.EdgeEvidenceFilter, *core.ApiError) {
	for _, name := range []string{
		eventCategoryParam, eventActionParam, destinationPortParam, destinationPortAbsentParam,
		logonTypeParam, logonTypeAbsentParam, httpStatusParam, httpStatusAbsentParam,
	} {
		if _, given := query[name]; given && query.Get(name) == "" {
			return pipeline.EdgeEvidenceFilter{},
				invalidRequestError(errors.New(name+" must not be empty"), nil)
		}
	}
	for _, pair := range [][2]string{
		{destinationPortParam, destinationPortAbsentParam}, {logonTypeParam, logonTypeAbsentParam},
		{httpStatusParam, httpStatusAbsentParam},
	} {
		value, absentName := pair[0], pair[1]
		absentText := query.Get(absentName)
		if absentText != "" && absentText != absentFlagValue {
			return pipeline.EdgeEvidenceFilter{}, invalidRequestError(
				errors.New(absentName+" takes "+absentFlagValue), nil)
		}
		if absentText != "" && query.Get(value) != "" {
			return pipeline.EdgeEvidenceFilter{}, invalidRequestError(
				errors.New("a request takes either "+value+" or "+absentName), nil)
		}
	}
	filter := pipeline.EdgeEvidenceFilterOf(core.EdgeEvidenceSelector{
		EventCategory:         query.Get(eventCategoryParam),
		EventAction:           query.Get(eventActionParam),
		DestinationPort:       query.Get(destinationPortParam),
		DestinationPortAbsent: query.Get(destinationPortAbsentParam) == absentFlagValue,
		LogonType:             query.Get(logonTypeParam),
		LogonTypeAbsent:       query.Get(logonTypeAbsentParam) == absentFlagValue,
		HttpStatus:            query.Get(httpStatusParam),
		HttpStatusAbsent:      query.Get(httpStatusAbsentParam) == absentFlagValue,
	})
	caseId, err := readRequestedCase(query)
	if err != nil {
		return pipeline.EdgeEvidenceFilter{}, invalidRequestError(err, nil)
	}
	filter.Case = caseId
	return filter, nil
}

// checkEdgeParameterNames は要求の項目の名前と多重度を確かめる。
func checkEdgeParameterNames(query url.Values) error {
	known := map[string]struct{}{
		eventCategoryParam: {}, eventActionParam: {}, destinationPortParam: {},
		destinationPortAbsentParam: {}, logonTypeParam: {}, logonTypeAbsentParam: {},
		httpStatusParam: {}, httpStatusAbsentParam: {},
		matchConditionParam: {}, caseParam: {},
	}
	if err := unsupportedParameterError(query, known); err != nil {
		return err
	}
	for name, values := range query {
		if len(values) != 1 && !repeatedRequestItem(name) {
			return errors.New("query parameter must occur once")
		}
	}
	return nil
}
