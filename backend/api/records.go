package api

import (
	"log/slog"
	"net/http"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const recordsPattern = "GET /api/v0/records"

// recordsHandler は`/api/v0/records` の元レコードを返す。
//
// 索引と fields を組む器は handler 1 つにつき 1 回だけ組む。組み直すたびに取り込み結果の
// 全レコードを走査するためである (pipeline.NewCandidateIndex と pipeline.NewFieldsBuilder)。
type recordsHandler struct {
	result pipeline.ImportResult
	index  pipeline.CandidateIndex
	fields *pipeline.FieldsBuilder
	// graphs は起点を与えた要求だけが、選んだ条件のグラフを取るのに使う。
	graphs graphSource
}

// recordResponse は`/api/v0/records` の応答である。
// **本型が項目の定義元である。**
//
// RawText は原資料の byte 列をそのまま持つ。制御文字を JSON escape で表すのは
// output.WriteJSON であり、値を書き換えない。
type recordResponse struct {
	RecordRef       core.RecordLocator   `json:"recordRef"`
	RawText         string               `json:"rawText"`
	SourceIdentity  core.SourceIdentity  `json:"sourceIdentity"`
	Fields          []core.RecordField   `json:"fields"`
	ObservationKind core.ObservationKind `json:"observationKind"`
	// EventKind はレコードの事象の分類と動作の組である。根拠のレコードを絞る条件の
	// eventCategory と eventAction にそのまま渡せる。分類を持たないレコードでは出ない。
	EventKind *core.EventKindPair `json:"eventKind,omitempty"`
	// DerivationTrail は起点のレコードからこのレコードへ至った経路である。起点を与えた要求だけが
	// 持つ。起点とこのレコードの間に関連付けが無いときは、止まった段階を持つ経路になる。
	DerivationTrail *core.DerivationTrail `json:"derivationTrail,omitempty"`
}

// requestedPositionAt は要求が与えた位置 1 つを、位置の指し方と対で持つ。
type requestedPositionAt struct {
	kind  core.PositionKind
	value int64
}

func (h recordsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	request, apiError := parseRecordsRequest(r)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	identity, entry, apiError := h.findCheckedRecord(request.record)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	var trail *core.DerivationTrail
	if request.origin != nil {
		_, origin, apiError := h.findCheckedRecord(*request.origin)
		if apiError != nil {
			writeError(w, httpStatusFor(apiError.Code), *apiError)
			return
		}
		graph, _, ok := graphOfSelection(w, r, h.graphs, request.selection)
		if !ok {
			return
		}
		built, err := derivationTrailBetween(graph, origin.Locator, entry.Locator, request.selection)
		if err != nil {
			// 内部不変条件の破れである。err は項目の名前だけを持ち、原資料の byte 列を持たない。
			slog.Error("building the derivation trail failed", "error", err)
			writeError(w, http.StatusInternalServerError, core.ApiError{
				Code: core.ApiErrorCodeInternalError, Message: "building the derivation trail failed",
			})
			return
		}
		trail = &built
	}
	h.writeRecord(w, entry, identity, trail)
}

// findCheckedRecord は要求が指した収集元を確かめ、位置が指す 1 レコードを返す。
func (h recordsHandler) findCheckedRecord(
	record requestedRecord,
) (core.SourceIdentity, pipeline.RecordEntry, *core.ApiError) {
	identity, apiError := h.checkRequestedSource(record.source)
	if apiError != nil {
		return core.SourceIdentity{}, pipeline.RecordEntry{}, apiError
	}
	entry, apiError := h.findRecord(record)
	return identity, entry, apiError
}

// derivationTrailBetween は起点のレコードから開いたレコードへ至った経路を組む。
//
// **関連付けが無い組を失敗にしない。** レコードの表示は経路と別に読めるため、止まった段階を
// 持つ経路で関連付けが無いことを示す。
func derivationTrailBetween(
	graph pipeline.Graph, origin, opened core.RecordLocator, selection pipeline.MatchConditionSelection,
) (core.DerivationTrail, error) {
	match, found, err := graph.EdgeMatchBetween(origin, opened)
	if err != nil {
		return core.DerivationTrail{}, err
	}
	if found {
		return core.DerivationTrailOfMatch(match)
	}
	return selection.UnmatchedDerivationTrail(origin)
}

// checkRequestedSource は要求が指した収集元を、code の判定の順序で確かめる。
func (h recordsHandler) checkRequestedSource(
	source requestedSource,
) (core.SourceIdentity, *core.ApiError) {
	return checkRequestedSource(h.result, source)
}

// checkRequestedSource は収集元の識別と内容の識別と公開の状態を、この順で確かめる。
//
// **判定の順序を操作ごとに変えない。** 収集元を探せない要求、内容の識別が食い違う要求、
// 公開を止めた収集元への要求で、同じ code を返す。
func checkRequestedSource(
	result pipeline.ImportResult, source requestedSource,
) (core.SourceIdentity, *core.ApiError) {
	identity, found := result.Identity(source.sourceId)
	if !found {
		return core.SourceIdentity{}, &core.ApiError{
			Code:                core.ApiErrorCodeSourceNotFound,
			Message:             "no source matches the requested sourceId",
			SourceId:            source.sourceId,
			SourceContentSha256: source.contentSha256,
		}
	}
	// 要求の sourceContentSha256 を、取り込みの時点の contentSha256 と比べる。本 handler が
	// 返すのは取り込みで読んだ byte 列であり、その sha256 が contentSha256 である。
	// originPath の内容の差し替えは、調査を開くときに検出する。調査を開くたびに
	// Investigation.RecordedPlans の計画が原資料を読み直し、記録した sha256 と一致しなければ
	// 取り込み全体を止める。
	if identity.ContentSha256 != source.contentSha256 {
		return core.SourceIdentity{}, &core.ApiError{
			Code:                core.ApiErrorCodeSourceHashMismatch,
			Message:             "the requested sourceContentSha256 differs from the imported content",
			SourceId:            source.sourceId,
			SourceContentSha256: source.contentSha256,
			OriginPath:          identity.OriginPath,
		}
	}
	return identity, checkWithheldSource(result, source.sourceId)
}

// checkWithheldSource は公開を停止している収集元への要求を退ける。
func checkWithheldSource(result pipeline.ImportResult, sourceId string) *core.ApiError {
	for _, status := range result.Statuses() {
		if status.SourceId != sourceId ||
			status.PublicationState != core.PublicationStateWithheld {
			continue
		}
		statusRef, ok := result.ImportStatusRef(sourceId)
		if !ok {
			// 内部不変条件の破れである。公開の状態を持つ収集元は取り込み結果に属する。
			slog.Error("the withheld source has no import status reference")
			return &core.ApiError{
				Code:    core.ApiErrorCodeInternalError,
				Message: "reading the record failed",
			}
		}
		return &core.ApiError{
			Code:            core.ApiErrorCodeImportWithheld,
			Message:         "the requested source is withheld",
			ImportStatusRef: statusRef,
		}
	}
	return nil
}

// findRecord は要求が与えた位置が指す 1 レコードを返す。
func (h recordsHandler) findRecord(
	request requestedRecord,
) (pipeline.RecordEntry, *core.ApiError) {
	scope, found := h.index.SourceScope(request.source.sourceId)
	if !found {
		// 内部不変条件の破れである。checkRequestedSource が公開を停止した収集元を先に
		// 退けるため、ここへ届く収集元は索引に範囲を持つ。
		slog.Error("the published source has no scanned range")
		return pipeline.RecordEntry{}, &core.ApiError{
			Code:    core.ApiErrorCodeInternalError,
			Message: "reading the record failed",
		}
	}
	positions := requestedPositionsOf(request.position)
	if apiError := checkPositionsInScope(request.source, scope, positions); apiError != nil {
		return pipeline.RecordEntry{}, apiError
	}
	return h.recordAt(request.source, positions)
}

// requestedPositionsOf は要求が与えた位置を、通番、行番号、byte 位置の順で返す。
//
// **位置の指し方を 1 つの表から回す。** `parseRecordsRequest` が受け付ける項目と
// この表が食い違うと、必須条件を満たした要求が位置を 1 つも持たないまま
// record_not_found になる。
func requestedPositionsOf(position requestedPosition) []requestedPositionAt {
	requested := []struct {
		kind  core.PositionKind
		value *int64
	}{
		{core.PositionKindSequenceNumber, position.sequenceNumber},
		{core.PositionKindLineNumber, position.lineNumber},
		{core.PositionKindByteRange, position.byteOffset},
	}
	positions := make([]requestedPositionAt, 0, len(requested))
	for _, item := range requested {
		if item.value == nil {
			continue
		}
		positions = append(positions, requestedPositionAt{
			kind: item.kind, value: *item.value,
		})
	}
	return positions
}

// checkPositionsInScope は、収集元の範囲の外にある位置を退ける。
//
// **判定を行うのは、要求が与えた位置と収集元の範囲の positionKind が同じ単位である場合に
// 限る。** 単位が異なる位置は record_not_found の判定に回す。範囲が whole_source の
// 収集元は両端が揃わないため、判定の対象にしない。
func checkPositionsInScope(source requestedSource, scope core.RecordRange,
	positions []requestedPositionAt,
) *core.ApiError {
	if scope.RangeKind != core.RangeKindPositioned ||
		scope.FromPosition == nil || scope.ToPosition == nil {
		return nil
	}
	for _, position := range positions {
		if position.kind != scope.PositionKind {
			continue
		}
		if position.value < *scope.FromPosition || position.value > *scope.ToPosition {
			return &core.ApiError{
				Code:                core.ApiErrorCodePositionOutsideSource,
				Message:             "the requested position is outside the scanned range of the source",
				SourceId:            source.sourceId,
				SourceContentSha256: source.contentSha256,
			}
		}
	}
	return nil
}

// recordAt は収集元と位置で指した 1 レコードを返す。
//
// **要求が与えた位置をすべて同じレコードへ解決する。** 応答が持つ recordRef は 1 つで
// あり、一方の位置だけに一致するレコードを返すと、応答の位置が要求の位置と食い違う。
//
// 既知の制限: 通番と行番号と byte 位置のうち 2 つ以上が別のレコードを指す要求を
// record_not_found で返す,
// 測る対象が無い。2 つの位置が別のレコードを指す要求に返す code が未決であり、
// 比べる相手の値が無い, 同じ要求の code が決まったときに見直す。
func (h recordsHandler) recordAt(source requestedSource,
	positions []requestedPositionAt,
) (pipeline.RecordEntry, *core.ApiError) {
	entry, apiError := recordAt(h.index, source, positions)
	if apiError == nil || apiError.Code != core.ApiErrorCodeRecordNotFound {
		return entry, apiError
	}
	// **読めなかったレコードの位置を、レコードが無い位置と分けて返す。**
	// 分析者が次に採る手が異なる (`/api/v0/sources` の取り込みの状態が診断を持つ)。
	if h.unreadableAt(source, positions) {
		return pipeline.RecordEntry{}, &core.ApiError{
			Code: core.ApiErrorCodeRecordUnreadable,
			Message: "the import could not read the record at the requested position; " +
				"the import status carries the diagnosis",
			SourceId:            source.sourceId,
			SourceContentSha256: source.contentSha256,
		}
	}
	return pipeline.RecordEntry{}, apiError
}

// unreadableAt は、要求の位置に取り込みが失敗したレコードがあるかを返す。
//
// **行番号と通番と byte 位置を突き合わせる。** 失敗した段階によって、失敗が確定できた
// 位置が異なる。文字列へ分ける段階で失敗したレコードは行番号だけを持ち、項目を写す段階と
// 正規化の段階で失敗したレコードは原資料から読んだ通番と byte 位置も持つ。
func (h recordsHandler) unreadableAt(
	source requestedSource, positions []requestedPositionAt,
) bool {
	publication, found := h.result.Publication(source.sourceId)
	if !found {
		return false
	}
	for _, failure := range publication.Status().Failures {
		if failureAtAllPositions(failure, positions) {
			return true
		}
	}
	return false
}

// failureAtAllPositions は、要求が与えた位置がすべて同じ 1 件の失敗を指すかを返す。
//
// **位置の 1 つが一致しただけで読めなかったと判定しない。** 要求が与えた位置は同じ
// 1 レコードを指す (recordAt)。一方の位置が読めたレコードを指し、もう一方が別の失敗の
// 位置と一致する要求へ、読めなかったという診断を返さない。
func failureAtAllPositions(
	failure core.ImportFailure, positions []requestedPositionAt,
) bool {
	for _, position := range positions {
		if position.value != failurePositionOf(failure, position.kind) {
			return false
		}
	}
	// parseRecordsRequest は位置を 1 つ以上持つ要求だけを通す。
	return len(positions) > 0
}

// failurePositionOf は失敗が確定できた位置を、位置の指し方ごとに返す。
// 確定できていない指し方には、要求の値と一致しない値を返す。
func failurePositionOf(failure core.ImportFailure, kind core.PositionKind) int64 {
	// 要求の位置は行番号が 1 以上、通番が 0 以上である
	// (records_request.go の firstLineNumber と lowestSequenceNumber)。
	const noPosition = -1
	switch kind {
	case core.PositionKindLineNumber:
		if failure.LineNumber == nil {
			return noPosition
		}
		return *failure.LineNumber
	case core.PositionKindSequenceNumber:
		if failure.RecordRef == nil || failure.RecordRef.SequenceNumber == nil {
			return noPosition
		}
		return *failure.RecordRef.SequenceNumber
	case core.PositionKindByteRange:
		if failure.RecordRef == nil || failure.RecordRef.ByteOffset == nil {
			return noPosition
		}
		return *failure.RecordRef.ByteOffset
	default:
		return noPosition
	}
}

func recordAt(index pipeline.CandidateIndex, source requestedSource,
	positions []requestedPositionAt,
) (pipeline.RecordEntry, *core.ApiError) {
	var selected pipeline.RecordEntry
	resolved := false
	for _, position := range positions {
		entry, found := index.RecordAt(source.sourceId, position.kind, position.value)
		if !found {
			return pipeline.RecordEntry{}, recordNotFound(source)
		}
		if resolved && selected.Locator.RecordRawTextRef != entry.Locator.RecordRawTextRef {
			return pipeline.RecordEntry{}, recordNotFound(source)
		}
		selected, resolved = entry, true
	}
	if !resolved {
		// parseRecordsRequest は位置を 1 つ以上持つ要求だけを通す。
		return pipeline.RecordEntry{}, recordNotFound(source)
	}
	return selected, nil
}

func recordNotFound(source requestedSource) *core.ApiError {
	return &core.ApiError{
		Code:                core.ApiErrorCodeRecordNotFound,
		Message:             "no record of the source matches the requested position",
		SourceId:            source.sourceId,
		SourceContentSha256: source.contentSha256,
	}
}

// writeRecord は特定した 1 レコードを応答へ書く。
func (h recordsHandler) writeRecord(w http.ResponseWriter,
	entry pipeline.RecordEntry, identity core.SourceIdentity, trail *core.DerivationTrail,
) {
	fields, err := h.fields.Build(entry)
	if err != nil {
		// 内部不変条件の破れである。err は項目の name と、端末を導く元にした接続元 IP の
		// 文字列を持つ。どちらも外部由来であり、無害化は cmd が設定した logger が行う。
		slog.Error("building the response fields of a record failed", "error", err)
		writeError(w, http.StatusInternalServerError, core.ApiError{
			Code:    core.ApiErrorCodeInternalError,
			Message: "reading the record failed",
		})
		return
	}
	writeJSON(w, http.StatusOK, recordResponse{
		RecordRef:       entry.Locator,
		RawText:         entry.RawText,
		SourceIdentity:  identity,
		Fields:          fields,
		ObservationKind: entry.Semantics.ObservationKind,
		EventKind:       pipeline.EventKindPairOf(fields),
		DerivationTrail: trail,
	})
}
