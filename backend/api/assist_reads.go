package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// LLM 用の読み取り 1 回の本文に載せる件数の上限。
//
// 既知の制限: グラフの検索は 100 ノード、詳細の根拠と時系列は 50 行、記録の原文は 1 回 10 件、
// 詳細の属性は 50 個と属性ごとの値 10 個に固定し、超えた分は切り詰めたことを本文に書く, 上限は
// LLM の文脈の token を抑えるために置いた。会話の token の量は中継した LLM の応答だけが持ち、
// repo の test では測れない, 実機の会話で文脈の上限に達するか、上限で切り詰めた本文が調査を
// 止めたときに見直す
const (
	maxAssistGraphNodes      = 100
	maxAssistEvidence        = 50
	maxAssistTimelineRows    = 50
	maxAssistRecordsPerRead  = 10
	maxAssistEventKinds      = 100
	maxAssistValueCounts     = 50
	maxAssistAttributes      = 50
	maxAssistAttributeValues = 10
)

// errAssistUnknownRef は、要求の短い参照がこの会話で発行したものでないことを表す。
var errAssistUnknownRef = errors.New(
	"the reference was not issued in this conversation; read it through a tool of this conversation first")

// assistRecordSummary は本文に載せる記録 1 件の参照と位置である。
type assistRecordSummary struct {
	Ref            string `json:"ref"`
	SourceFileName string `json:"sourceFileName"`
	// Position は収集元の中の位置の文字列である (`line 12`、`sequence 3`、`byte 40`)。
	Position string `json:"position"`
}

// assistNodeSummary は本文に載せるノード 1 件である。
type assistNodeSummary struct {
	Ref   string        `json:"ref"`
	Kind  core.NodeKind `json:"kind"`
	Label string        `json:"label"`
	// Record はレコードのノードが指す記録の短い参照である。
	Record string `json:"record,omitempty"`
}

// assistEdgeSummary は本文に載せる関係 1 件である。
type assistEdgeSummary struct {
	Ref           string             `json:"ref"`
	Kind          core.EdgeKind      `json:"kind"`
	State         core.RelationState `json:"state"`
	Source        string             `json:"source"`
	Target        string             `json:"target"`
	EvidenceCount int64              `json:"evidenceCount"`
}

// positionTextOf は位置の文字列を返す。
func positionTextOf(kind core.PositionKind, sequence, line, byteOffset *int64) string {
	switch {
	case kind == core.PositionKindSequenceNumber && sequence != nil:
		return "sequence " + strconv.FormatInt(*sequence, 10)
	case kind == core.PositionKindLineNumber && line != nil:
		return "line " + strconv.FormatInt(*line, 10)
	case kind == core.PositionKindByteRange && byteOffset != nil:
		return "byte " + strconv.FormatInt(*byteOffset, 10)
	default:
		return string(kind)
	}
}

// labelText は表示名の文字列を返す。原資料の文字列が無い表示名は正規化値を使う。
func labelText(label core.RawAndNormalized) string {
	switch {
	case label.RawText != nil:
		return *label.RawText
	case label.Normalized != nil:
		return *label.Normalized
	default:
		return ""
	}
}

// recordSummary は位置の記録を取り込み結果から探し、短い参照を発行する。ok が偽になるのは、
// 位置の記録が今の取り込み結果に無いか、公開を停止した収集元にあるときである。
func (h assistConversationsHandler) recordSummary(
	issuer pipeline.AssistRefIssuer, locator core.RecordLocator,
) (assistRecordSummary, bool) {
	entry, ok := h.recordEntry(locator)
	if !ok {
		return assistRecordSummary{}, false
	}
	ref := issuer.Record(core.AssistRecordTarget{
		SourceId: locator.SourceId, Record: core.NewAssertionRecordRef(entry.Locator),
	})
	return assistRecordSummary{
		Ref: ref, SourceFileName: entry.Locator.SourceFileName,
		Position: positionTextOf(entry.Locator.PositionKind, entry.Locator.SequenceNumber,
			entry.Locator.LineNumber, entry.Locator.ByteOffset),
	}, true
}

// recordEntry は位置の記録を取り込み結果から探す。
func (h assistConversationsHandler) recordEntry(locator core.RecordLocator) (pipeline.RecordEntry, bool) {
	source := requestedSource{sourceId: locator.SourceId, contentSha256: locator.SourceContentSha256}
	if _, apiError := checkRequestedSource(h.result, source); apiError != nil {
		return pipeline.RecordEntry{}, false
	}
	entry, apiError := recordAt(h.index, source, requestedPositionsOf(requestedPosition{
		sequenceNumber: locator.SequenceNumber, lineNumber: locator.LineNumber, byteOffset: locator.ByteOffset,
	}))
	return entry, apiError == nil
}

// recordTargetOfNode はレコードのノードの識別鍵から、指す記録を組む。ok が偽になるのは、
// レコードのノードでないか、識別鍵が位置を持たないときである。
func (h assistConversationsHandler) recordTargetOfNode(node core.GraphNode) (core.AssistRecordTarget, bool) {
	if node.Kind != core.NodeKindRecord || len(node.Identity) != 3 {
		return core.AssistRecordTarget{}, false
	}
	sha, kind := node.Identity[0].Value, core.PositionKind(node.Identity[1].Value)
	position, err := strconv.ParseInt(node.Identity[2].Value, 10, 64)
	if err != nil {
		return core.AssistRecordTarget{}, false
	}
	record := core.AssertionRecordRef{SourceContentSha256: sha, PositionKind: kind}
	switch kind {
	case core.PositionKindSequenceNumber:
		record.SequenceNumber = &position
	case core.PositionKindLineNumber:
		record.LineNumber = &position
	case core.PositionKindByteRange:
		record.ByteOffset = &position
	default:
		return core.AssistRecordTarget{}, false
	}
	entries, err := h.result.SourceEntries()
	if err != nil {
		return core.AssistRecordTarget{}, false
	}
	// 同じ内容を 2 回取り込んだ収集元のレコードは 1 つのノードにまとまる。最初の収集元で探す。
	for _, entry := range entries {
		if entry.Identity.ContentSha256 == sha {
			return core.AssistRecordTarget{SourceId: entry.Identity.SourceId, Record: record}, true
		}
	}
	return core.AssistRecordTarget{}, false
}

// nodeSummaryOf はノードへ短い参照を発行し、本文に載せる組にする。
func nodeSummaryOf(issuer pipeline.AssistRefIssuer, node core.GraphNode) assistNodeSummary {
	return assistNodeSummary{Ref: issuer.Node(node.Id), Kind: node.Kind, Label: labelText(node.Label)}
}

// edgeSummaryOf は関係と両端のノードへ短い参照を発行し、本文に載せる組にする。
func edgeSummaryOf(issuer pipeline.AssistRefIssuer, edge core.GraphEdge) assistEdgeSummary {
	return assistEdgeSummary{
		Ref: issuer.Edge(edge.Id), Kind: edge.Kind, State: edge.State,
		Source: issuer.Node(edge.SourceNodeId), Target: issuer.Node(edge.TargetNodeId),
		EvidenceCount: edge.EvidenceCount,
	}
}

// overviewResponse は調査の概要の本文である。
type overviewResponse struct {
	Sources        []overviewSource            `json:"sources"`
	NodeKinds      []core.MatchedKind          `json:"nodeKinds"`
	EventKinds     []overviewEventKind         `json:"eventKinds"`
	EventKindTotal int                         `json:"eventKindTotal"`
	Conditions     []core.AssistMatchCondition `json:"matchConditions"`
	Truncated      bool                        `json:"truncated"`
}

type overviewSource struct {
	SourceId    string `json:"sourceId"`
	FileName    string `json:"fileName"`
	RecordCount *int64 `json:"recordCount,omitempty"`
}

type overviewEventKind struct {
	EventCategory string `json:"eventCategory"`
	EventAction   string `json:"eventAction,omitempty"`
	RecordCount   int64  `json:"recordCount"`
}

// overview は調査の概要を返す。収集元、ノードの種類ごとの件数、事象の種別、関連付けの条件である。
func (h assistConversationsHandler) overview(w http.ResponseWriter, r *http.Request) {
	raw, apiError := readStrictBody(r)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	var body turnRequest
	if apiError := decodeStrictBytes(raw, &body); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	read, ok := h.assistTurnOf(w, r, body.TurnId, h.graphs)
	if !ok {
		return
	}
	turn, graph := read.turn, read.graph
	entries, err := h.result.SourceEntries()
	if err != nil {
		writeGraphFailure(w, r, err)
		return
	}
	h.disclose(w, pipeline.AssistDisclosureRequest{
		ConversationId: turn.ConversationId, TurnId: turn.TurnId, Tool: core.AssistToolOverview, Request: string(raw),
		Versions: read.versions,
	}, func(pipeline.AssistRefIssuer) (pipeline.AssistComposition, error) {
		response := overviewResponse{Sources: []overviewSource{}, EventKinds: []overviewEventKind{},
			Conditions: turn.MatchConditions}
		for _, entry := range entries {
			response.Sources = append(response.Sources, overviewSource{
				SourceId: entry.Identity.SourceId, FileName: entry.Identity.FileName,
				RecordCount: entry.Identity.RecordCount,
			})
		}
		response.NodeKinds = graph.Query(pipeline.GraphQuery{}).MatchedKinds
		kinds, _ := graph.EventKinds(pipeline.RecordFilter{})
		response.EventKindTotal = len(kinds)
		for index, kind := range kinds {
			if index == maxAssistEventKinds {
				response.Truncated = true
				break
			}
			response.EventKinds = append(response.EventKinds, overviewEventKind{
				EventCategory: kind.Category, EventAction: kind.Action, RecordCount: kind.RecordCount,
			})
		}
		return composeAssistBody(response, nil, response.Truncated)
	})
}

// graphSearchBody はグラフの検索の要求の本文である。
type graphSearchBody struct {
	TurnId      string           `json:"turnId"`
	SearchQuery core.SearchQuery `json:"searchQuery"`
}

// graphSearchResponse はグラフの検索の本文である。
type graphSearchResponse struct {
	MatchedNodeCount int64                 `json:"matchedNodeCount"`
	Nodes            []assistNodeSummary   `json:"nodes"`
	Edges            []assistEdgeSummary   `json:"edges"`
	ValueCounts      []assistValueCount    `json:"valueCounts,omitempty"`
	EmptyReason      core.EmptyReason      `json:"emptyReason,omitempty"`
	Truncated        bool                  `json:"truncated"`
	Records          []assistRecordSummary `json:"records"`
}

type assistValueCount struct {
	Value       string `json:"value"`
	RecordCount int64  `json:"recordCount"`
}

// graphSearch は検索の条件でグラフを検索する。
//
// **まとめずに返し、上限までのノードを載せる。** 本文をグループノードのまとめ方に依らせない。
func (h assistConversationsHandler) graphSearch(w http.ResponseWriter, r *http.Request) {
	raw, apiError := readStrictBody(r)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	var body graphSearchBody
	if apiError := decodeStrictBytes(raw, &body); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	read, ok := h.assistTurnOf(w, r, body.TurnId, h.graphs)
	if !ok {
		return
	}
	turn, graph := read.turn, read.graph
	query, err := h.resolveOrigins(turn.ConversationId, body.SearchQuery)
	if err != nil {
		writeAssistStoreError(w, err)
		return
	}
	request, err := newGraphRequest(query)
	if err != nil {
		writeError(w, http.StatusBadRequest, *searchQueryError(err))
		return
	}
	if apiError := checkKnownSearch(graph, query); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	subgraph := graph.Query(request.query())
	emptyReason := core.EmptyReason("")
	if subgraph.MatchedNodeCount == 0 {
		emptyReason = graphHandler{graph: graph}.graphEmptyReasonOf(request)
	}
	h.disclose(w, pipeline.AssistDisclosureRequest{
		ConversationId: turn.ConversationId, TurnId: turn.TurnId, Tool: core.AssistToolGraphSearch,
		Request: string(raw), Versions: read.versions,
	}, func(issuer pipeline.AssistRefIssuer) (pipeline.AssistComposition, error) {
		response := graphSearchResponse{
			MatchedNodeCount: int64(subgraph.MatchedNodeCount), Nodes: []assistNodeSummary{},
			Edges: []assistEdgeSummary{}, Records: []assistRecordSummary{}, EmptyReason: emptyReason,
		}
		shown := map[string]bool{}
		var records []string
		for index, node := range subgraph.Nodes {
			if index == maxAssistGraphNodes {
				response.Truncated = true
				break
			}
			summary := nodeSummaryOf(issuer, node.GraphNode)
			if target, isRecord := h.recordTargetOfNode(node.GraphNode); isRecord {
				summary.Record = issuer.Record(target)
				records = append(records, summary.Record)
			}
			response.Nodes = append(response.Nodes, summary)
			shown[node.Id] = true
		}
		for _, edge := range subgraph.Edges {
			if !shown[edge.SourceNodeId] || !shown[edge.TargetNodeId] {
				response.Truncated = true
				continue
			}
			response.Edges = append(response.Edges, edgeSummaryOf(issuer, edge))
		}
		for index, count := range subgraph.ValueCounts {
			if index == maxAssistValueCounts {
				response.Truncated = true
				break
			}
			response.ValueCounts = append(response.ValueCounts, assistValueCount{
				Value: count.Value, RecordCount: count.RecordCount,
			})
		}
		return composeAssistBody(response, records, response.Truncated)
	})
}

// targetBody は対象の詳細の要求の本文である。
type targetBody struct {
	TurnId string `json:"turnId"`
	Ref    string `json:"ref"`
}

// targetResponse は対象の詳細の本文である。
type targetResponse struct {
	Node          *assistNodeSummary    `json:"node,omitempty"`
	Edge          *assistEdgeSummary    `json:"edge,omitempty"`
	Attributes    []assistAttribute     `json:"attributes,omitempty"`
	EvidenceCount int64                 `json:"evidenceCount"`
	Evidence      []assistRecordSummary `json:"evidence"`
	// UnreadableRecordCount は、上限までの根拠のうち、収集元を探せずに本文へ載せなかった記録の数である。
	UnreadableRecordCount int64 `json:"unreadableRecordCount,omitempty"`
	Truncated             bool  `json:"truncated"`
}

type assistAttribute struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

// resolveOrigins は、起点のノードの短い参照を、この会話で発行したノードの識別子へ置き換える。
// LLM はノードの識別子を受け取らず短い参照だけを受け取るため、起点を短い参照で指す。
func (h assistConversationsHandler) resolveOrigins(
	conversationId string, query core.SearchQuery,
) (core.SearchQuery, error) {
	if len(query.NodeIds) == 0 {
		return query, nil
	}
	resolved := make([]string, 0, len(query.NodeIds))
	for _, text := range query.NodeIds {
		ref, found, err := h.store.ResolveRef(conversationId, text)
		if err != nil {
			return core.SearchQuery{}, err
		}
		if !found || ref.Kind != core.AssistRefKindNode {
			return core.SearchQuery{}, errAssistUnknownRef
		}
		resolved = append(resolved, ref.NodeId)
	}
	query.NodeIds = resolved
	return query, nil
}

// target は、この会話で発行したノードまたは関係の参照の詳細を返す。
func (h assistConversationsHandler) target(w http.ResponseWriter, r *http.Request) {
	raw, apiError := readStrictBody(r)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	var body targetBody
	if apiError := decodeStrictBytes(raw, &body); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	read, ok := h.assistTurnOf(w, r, body.TurnId, h.graphs)
	if !ok {
		return
	}
	turn, graph := read.turn, read.graph
	ref, found, err := h.store.ResolveRef(turn.ConversationId, body.Ref)
	if err != nil {
		writeAssistStoreError(w, err)
		return
	}
	if !found || ref.Kind == core.AssistRefKindRecord {
		writeAssistStoreError(w, errAssistUnknownRef)
		return
	}
	h.disclose(w, pipeline.AssistDisclosureRequest{
		ConversationId: turn.ConversationId, TurnId: turn.TurnId, Tool: core.AssistToolTarget, Request: string(raw),
		Versions: read.versions,
	}, func(issuer pipeline.AssistRefIssuer) (pipeline.AssistComposition, error) {
		response := targetResponse{Evidence: []assistRecordSummary{}}
		var evidence []core.GraphEvidence
		if ref.Kind == core.AssistRefKindNode {
			detail, found := graph.NodeDetail(ref.NodeId)
			if !found {
				return pipeline.AssistComposition{}, errAssistUnknownSelection
			}
			summary := nodeSummaryOf(issuer, detail.Node)
			response.Node = &summary
			for index, attribute := range detail.Attributes {
				if index == maxAssistAttributes {
					response.Truncated = true
					break
				}
				name := attribute.Name
				if name == "" {
					name = string(attribute.Semantic)
				}
				item := assistAttribute{Name: name, Values: []string{}}
				for _, value := range attribute.Values {
					item.Values = append(item.Values, fieldValues(value.Field)...)
				}
				if len(item.Values) > maxAssistAttributeValues {
					item.Values = item.Values[:maxAssistAttributeValues]
					response.Truncated = true
				}
				response.Attributes = append(response.Attributes, item)
			}
			evidence, response.EvidenceCount = detail.Evidence, int64(detail.EvidenceCount)
		} else {
			detail, found := graph.EdgeDetail(ref.EdgeId, pipeline.EdgeEvidenceFilter{})
			if !found {
				return pipeline.AssistComposition{}, errAssistUnknownSelection
			}
			summary := edgeSummaryOf(issuer, detail.Edge)
			response.Edge = &summary
			evidence, response.EvidenceCount = detail.Evidence, detail.Edge.EvidenceCount
		}
		var records []string
		for index, item := range evidence {
			if index == maxAssistEvidence {
				response.Truncated = true
				break
			}
			summary, ok := h.recordSummary(issuer, item.RecordRef)
			if !ok {
				response.UnreadableRecordCount++
				continue
			}
			response.Evidence = append(response.Evidence, summary)
			records = append(records, summary.Ref)
		}
		return composeAssistBody(response, records, response.Truncated)
	})
}

// recordsBody は記録の原文の要求の本文である。
type recordsBody struct {
	TurnId string   `json:"turnId"`
	Refs   []string `json:"refs"`
}

// recordsReadResponse は記録の原文の本文である。
type recordsReadResponse struct {
	Records []assistRecordText `json:"records"`
}

type assistRecordText struct {
	assistRecordSummary
	RawText string            `json:"rawText"`
	Fields  []assistAttribute `json:"fields"`
}

// records は、この会話で発行した記録の参照の原文と読めた欄を返す。
func (h assistConversationsHandler) records(w http.ResponseWriter, r *http.Request) {
	raw, apiError := readStrictBody(r)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	var body recordsBody
	if apiError := decodeStrictBytes(raw, &body); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	if len(body.Refs) == 0 || len(body.Refs) > maxAssistRecordsPerRead {
		writeError(w, http.StatusBadRequest, *invalidRequestError(
			errors.New("refs must carry 1 to "+strconv.Itoa(maxAssistRecordsPerRead)+" record references"), nil))
		return
	}
	turn, err := h.store.Turn(r.PathValue(conversationIdPathValue), body.TurnId)
	if err != nil {
		writeAssistStoreError(w, err)
		return
	}
	targets := make([]core.AssistRecordTarget, 0, len(body.Refs))
	for _, text := range body.Refs {
		ref, found, err := h.store.ResolveRef(turn.ConversationId, text)
		if err != nil {
			writeAssistStoreError(w, err)
			return
		}
		if !found || ref.Kind != core.AssistRefKindRecord {
			writeAssistStoreError(w, errAssistUnknownRef)
			return
		}
		targets = append(targets, *ref.Record)
	}
	versions, err := h.versions()
	if err != nil {
		writeGraphFailure(w, r, err)
		return
	}
	h.disclose(w, pipeline.AssistDisclosureRequest{
		ConversationId: turn.ConversationId, TurnId: turn.TurnId, Tool: core.AssistToolRecords, Request: string(raw),
		Versions: versions,
	}, func(issuer pipeline.AssistRefIssuer) (pipeline.AssistComposition, error) {
		response := recordsReadResponse{Records: []assistRecordText{}}
		var records []string
		for _, target := range targets {
			locator := core.RecordLocator{
				SourceId: target.SourceId, SourceContentSha256: target.Record.SourceContentSha256,
				PositionKind: target.Record.PositionKind, SequenceNumber: target.Record.SequenceNumber,
				LineNumber: target.Record.LineNumber, ByteOffset: target.Record.ByteOffset,
			}
			entry, found := h.recordEntry(locator)
			if !found {
				return pipeline.AssistComposition{}, errAssistUnknownSelection
			}
			summary, _ := h.recordSummary(issuer, locator)
			fields, err := h.fields.Build(entry)
			if err != nil {
				return pipeline.AssistComposition{}, err
			}
			text := assistRecordText{assistRecordSummary: summary, RawText: entry.RawText, Fields: []assistAttribute{}}
			for _, field := range fields {
				text.Fields = append(text.Fields, assistAttribute{Name: field.Name, Values: fieldValues(field)})
			}
			response.Records = append(response.Records, text)
			records = append(records, summary.Ref)
		}
		return composeAssistBody(response, records, false)
	})
}

// fieldValues は欄 1 つの値の文字列を返す。
func fieldValues(field core.RecordField) []string {
	switch {
	case field.Text != nil:
		return []string{labelText(*field.Text)}
	case field.Timestamp != nil && field.Timestamp.Normalized != nil:
		return []string{*field.Timestamp.Normalized}
	case field.Timestamp != nil && field.Timestamp.RawText != nil:
		return []string{*field.Timestamp.RawText}
	default:
		return []string{}
	}
}

// timelineBody は時系列の要求の本文である。
type timelineBody struct {
	TurnId     string                `json:"turnId"`
	Conditions core.RecordConditions `json:"conditions"`
}

// timelineReadResponse は時系列の本文である。
type timelineReadResponse struct {
	RowCount int                 `json:"rowCount"`
	Rows     []assistTimelineRow `json:"rows"`
	// UnreadableRecordCount は、上限までの行のうち、収集元を探せずに本文へ載せなかった記録の数である。
	UnreadableRecordCount int64 `json:"unreadableRecordCount,omitempty"`
	Truncated             bool  `json:"truncated"`
}

type assistTimelineRow struct {
	assistRecordSummary
	Time            string               `json:"time,omitempty"`
	ObservationKind core.ObservationKind `json:"observationKind"`
}

// timeline は根拠のレコードを絞る条件で時系列を返す。
func (h assistConversationsHandler) timeline(w http.ResponseWriter, r *http.Request) {
	raw, apiError := readStrictBody(r)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	var body timelineBody
	if apiError := decodeStrictBytes(raw, &body); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	read, ok := h.assistTurnOf(w, r, body.TurnId, h.observed)
	if !ok {
		return
	}
	turn, graph := read.turn, read.graph
	request, err := newTimelineRequest(body.Conditions)
	if err != nil {
		writeError(w, http.StatusBadRequest, *invalidRequestError(err, nil))
		return
	}
	if apiError := checkKnownRecordConditions(graph, body.Conditions); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	timeline := graph.Timeline(request.query())
	h.disclose(w, pipeline.AssistDisclosureRequest{
		ConversationId: turn.ConversationId, TurnId: turn.TurnId, Tool: core.AssistToolTimeline, Request: string(raw),
		Versions: read.versions,
	}, func(issuer pipeline.AssistRefIssuer) (pipeline.AssistComposition, error) {
		response := timelineReadResponse{RowCount: len(timeline.Entries), Rows: []assistTimelineRow{}}
		var records []string
		for index, entry := range timeline.Entries {
			if index == maxAssistTimelineRows {
				response.Truncated = true
				break
			}
			summary, found := h.recordSummary(issuer, entry.RecordRef)
			if !found {
				response.UnreadableRecordCount++
				continue
			}
			row := assistTimelineRow{assistRecordSummary: summary, ObservationKind: entry.ObservationKind}
			if entry.EventTime != nil && entry.EventTime.Normalized != nil {
				row.Time = *entry.EventTime.Normalized
			}
			response.Rows = append(response.Rows, row)
			records = append(records, summary.Ref)
		}
		return composeAssistBody(response, records, response.Truncated)
	})
}

// searchQueryCheckResponse は検索の条件の検証の結果である。
type searchQueryCheckResponse struct {
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason,omitempty"`
	// Origins は、起点の短い参照を解決したノードの識別子と種別と表示名である。起点の順に並ぶ。
	Origins []core.AssistOrigin `json:"origins,omitempty"`
}

// checkSearchQuery は検索の条件を実行せずに検証する。
//
// **受け渡しを記録しない。** 応答は中継が画面へ渡すだけで、LLM へ渡さない。起点の表示名は、
// 参照を発行した受け渡しの本文にすでに載っている。拒否の理由は案件と端末が実在するかを伝える
// ため、送信の許可を取り消した会話の検証を退ける。
func (h assistConversationsHandler) checkSearchQuery(w http.ResponseWriter, r *http.Request) {
	var body graphSearchBody
	if apiError := decodeStrictBody(r, &body); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	read, ok := h.assistTurnOf(w, r, body.TurnId, h.graphs)
	if !ok {
		return
	}
	turn, graph := read.turn, read.graph
	if err := h.store.Permitted(turn.ConversationId); err != nil {
		writeAssistStoreError(w, err)
		return
	}
	query, err := h.resolveOrigins(turn.ConversationId, body.SearchQuery)
	if errors.Is(err, errAssistUnknownRef) {
		writeJSON(w, http.StatusOK, searchQueryCheckResponse{Reason: err.Error()})
		return
	}
	if err != nil {
		writeAssistStoreError(w, err)
		return
	}
	// 検索式の構文も確かめる。構文の誤った式の card は、適用した画面の要求が退けられる。
	_, err = newGraphRequest(query)
	if err == nil {
		err = query.ValidateShowable()
	}
	if err != nil {
		writeJSON(w, http.StatusOK, searchQueryCheckResponse{Reason: err.Error()})
		return
	}
	if apiError := checkKnownSearch(graph, query); apiError != nil {
		writeJSON(w, http.StatusOK, searchQueryCheckResponse{Reason: apiError.Message})
		return
	}
	response := searchQueryCheckResponse{Accepted: true}
	for _, id := range query.NodeIds {
		// checkKnownSearch がどの起点もグラフにあることを確かめた。
		detail, _ := graph.NodeDetail(id)
		response.Origins = append(response.Origins, core.AssistOrigin{
			Id: id, Kind: detail.Node.Kind, Label: labelText(detail.Node.Label),
		})
	}
	writeJSON(w, http.StatusOK, response)
}
