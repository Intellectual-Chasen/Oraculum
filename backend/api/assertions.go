package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// 分析者の所見の操作の pattern。一覧と記録は同じ path を method で分け、改訂の追加は
// 識別子を path に持つ。
const (
	assertionsPattern        = "GET /api/v0/assertions"
	assertionCreatePattern   = "POST /api/v0/assertions"
	assertionRevisionPattern = "PUT /api/v0/assertions/{assertionId}"
	assertionIdPathValue     = "assertionId"
)

// requestBodyLimit は要求の本文を読む上限である。分析者の根拠の記述を収める大きさにする。
const requestBodyLimit = 1 << 20

// assertionsHandler は分析者の所見を一覧し、記録し、改訂を足す。
//
// **取り込み結果を書き換えない。** 所見は保存先が別に保ち、対象は識別子で指す。
// 応答が持つ targetOrigin は、その対象が現在のグラフの何から出たかを読んだ結果である。
type assertionsHandler struct {
	store    pipeline.AssertionStore
	resolver pipeline.AssertionResolver
	// timeInterpreted は収集元の時刻の解釈の改訂を 1 件記録した直後に呼ぶ。解釈は並びと関連付けの時刻の範囲に
	// 使う時点を変えるため、グラフの組み直しを次の要求を待たずに始める。
	timeInterpreted func()
}

// assertionsResponse は所見の一覧の応答である。**本型が項目の定義元である。**
//
// 記録した所見を全件返す。上限も続きを取る位置も持たない。
type assertionsResponse struct {
	Assertions     []assertionItem  `json:"assertions"`
	AssertionCount int64            `json:"assertionCount"`
	EmptyReason    core.EmptyReason `json:"emptyReason,omitempty"`
}

// assertionItem は所見 1 件と、その対象が現在のグラフの何から出たかである。
type assertionItem struct {
	Assertion core.Assertion `json:"assertion"`
	// TargetOrigin は対象の出所である。観測から直に出した関係、関連付けが挙げた候補、
	// 分析者が付けた所見、現在の取り込み結果に無い対象を分ける。
	TargetOrigin core.AssertionTargetOrigin `json:"targetOrigin"`
}

// assertionRequestBody は所見を記録する要求の本文である。
type assertionRequestBody struct {
	Target core.AssertionTarget  `json:"target"`
	Author string                `json:"author"`
	Basis  assertionBasisRequest `json:"basis"`
	State  *core.AssertionState  `json:"state,omitempty"`
	// TimeOffset は収集元の時刻を読む UTC からのずれである。対象が収集元のとき必須。
	TimeOffset *core.UtcOffset `json:"timeOffset,omitempty"`
	// BaseRevision は改訂が元にした所見の revision の番号である。改訂の要求で必須。記録の要求では読まない。
	BaseRevision *int64 `json:"baseRevision,omitempty"`
}

// assertionBasisRequest は要求が与える根拠である。
type assertionBasisRequest struct {
	Note       string                    `json:"note"`
	RecordRefs []core.AssertionRecordRef `json:"recordRefs,omitempty"`
}

func (b assertionBasisRequest) basis() core.AssertionBasis {
	return core.AssertionBasis{Note: b.Note, RecordRefs: b.RecordRefs}
}

func (h assertionsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.list(w, r)
	case http.MethodPost:
		h.create(w, r)
	case http.MethodPut:
		h.revise(w, r)
	default:
		writeError(w, http.StatusInternalServerError, core.ApiError{
			Code: core.ApiErrorCodeInternalError, Message: "the method reached the wrong handler",
		})
	}
}

// list は記録した順の所見を全件返す。
func (h assertionsHandler) list(w http.ResponseWriter, r *http.Request) {
	if err := checkAssertionsParameterNames(r.URL.Query()); err != nil {
		writeError(w, http.StatusBadRequest, core.ApiError{
			Code:    core.ApiErrorCodeInvalidRequest,
			Message: err.Error(),
		})
		return
	}
	assertions := h.store.List()
	added := pipeline.NewAddedRelations(assertions)
	response := assertionsResponse{
		Assertions:     make([]assertionItem, 0, len(assertions)),
		AssertionCount: int64(len(assertions)),
	}
	for _, assertion := range assertions {
		response.Assertions = append(response.Assertions, h.item(assertion, added))
	}
	if response.AssertionCount == 0 {
		response.EmptyReason = core.EmptyReasonNoRecordInFilter
	}
	writeJSON(w, http.StatusOK, response)
}

// create は新しい所見を 1 件記録する。
//
// **対象がグラフに無い要求も受け取る。** 対象を持たない取り込みへ所見を持ち込んだ状態は
// targetOrigin の absent が表す。
func (h assertionsHandler) create(w http.ResponseWriter, r *http.Request) {
	body, apiError := decodeAssertionBody(r)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	if body.State != nil {
		writeError(w, http.StatusBadRequest, core.ApiError{
			Code:    core.ApiErrorCodeInvalidRequest,
			Message: "state is decided by the store when an assertion is created",
		})
		return
	}
	// **関係を足したかは記録する時点のグラフで決める。** 後のグラフが関係を持たないとき、
	// 足した関係と再解析で消えた関係を所見の値で分ける。
	assertion, err := h.store.Create(pipeline.AssertionDraft{
		Target: body.Target, Author: body.Author, Basis: body.Basis.basis(), TimeOffset: body.TimeOffset,
		AddsRelation: body.Target.Kind == core.AssertionTargetKindEdge && !h.resolver.InGraph(body.Target),
	})
	// **収集元 1 件の解釈を 1 件の所見とその改訂で保つ。** 2 件目を受け入れると、どちらのずれで読むかが
	// 決まらず、解釈の変更の履歴も 2 つに割れる。確かめるのは保存先であり、同時に届いた要求も
	// 1 件だけが通る。
	// 所見は削除されないので、先に記録された所見は探すまでの間に消えない。
	if errors.Is(err, pipeline.ErrSourceAlreadyInterpreted) {
		for _, existing := range h.store.List() {
			if sameAssertionTarget(existing.Target, body.Target) {
				h.writeAssertionChanged(w, existing,
					"the source already carries a time interpretation; add a revision to it")
				return
			}
		}
		writeStoreError(w, fmt.Errorf("%w: %w", pipeline.ErrAssertionStoreFailure, err),
			"storing the assertion failed", "")
		return
	}
	if err != nil {
		writeStoreError(w, err, "storing the assertion failed", "the assertion is not storable")
		return
	}
	h.recordedRevision(assertion)
	writeJSON(w, http.StatusCreated, h.item(assertion, h.addedRelations()))
}

// recordedRevision は、収集元の時刻の解釈の改訂を記録したときにグラフの組み直しを始める。
func (h assertionsHandler) recordedRevision(assertion core.Assertion) {
	if assertion.Target.Kind == core.AssertionTargetKindSource && h.timeInterpreted != nil {
		h.timeInterpreted()
	}
}

// writeStoreError は保存先の失敗を、内部の失敗と要求の不備に分けて返す。
//
// **通番の枯渇と識別子の衝突は要求の不備ではない。** 要求を書き直しても結果が変わらない
// 失敗を invalid_request で返すと、分析者が要求の項目を疑う形になる。
func writeStoreError(w http.ResponseWriter, err error, internalMessage, requestMessage string) {
	if errors.Is(err, pipeline.ErrAssertionStoreFailure) {
		// 内部不変条件の破れであり、応答は理由を持たない。err は保存先の状態だけを持ち、
		// 分析者が書いた文字列を持たない。
		slog.Error(internalMessage, "error", err)
		writeError(w, http.StatusInternalServerError, core.ApiError{
			Code: core.ApiErrorCodeInternalError, Message: internalMessage,
		})
		return
	}
	writeError(w, http.StatusBadRequest, core.ApiError{
		Code: core.ApiErrorCodeInvalidRequest, Message: requestMessage,
	})
}

// revise は既存の所見へ新しい改訂を足す。置き換えられた改訂は履歴に残る。
func (h assertionsHandler) revise(w http.ResponseWriter, r *http.Request) {
	body, apiError := decodeAssertionBody(r)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	if body.State == nil {
		writeError(w, http.StatusBadRequest, core.ApiError{
			Code:              core.ApiErrorCodeInvalidRequest,
			Message:           "a new revision must carry its state",
			MissingParameters: []string{"state"},
		})
		return
	}
	if body.BaseRevision == nil {
		writeError(w, http.StatusBadRequest, core.ApiError{
			Code:              core.ApiErrorCodeInvalidRequest,
			Message:           "a new revision must carry the revision it is based on",
			MissingParameters: []string{"baseRevision"},
		})
		return
	}
	existing, found := h.store.Find(r.PathValue(assertionIdPathValue))
	if !found {
		writeError(w, http.StatusNotFound, core.ApiError{
			Code: core.ApiErrorCodeRecordNotFound, Message: "no assertion matches the identifier",
		})
		return
	}
	// **対象は改訂で変わらない。** 別の対象を指す判断は別の所見であり、履歴を引き継ぐと、
	// 置き換えられた改訂の根拠が別の対象の根拠として読まれる。
	if !sameAssertionTarget(body.Target, existing.Target) {
		writeError(w, http.StatusBadRequest, core.ApiError{
			Code:    core.ApiErrorCodeInvalidRequest,
			Message: "a new revision must keep the target of the assertion",
		})
		return
	}
	assertion, err := h.store.Revise(existing.Id, pipeline.AssertionRevisionDraft{
		State: *body.State, Author: body.Author, Basis: body.Basis.basis(), TimeOffset: body.TimeOffset,
		BaseRevision: *body.BaseRevision,
	})
	if errors.Is(err, pipeline.ErrAssertionRevisionConflict) {
		h.writeAssertionChanged(w, assertion, "the assertion changed after the base revision")
		return
	}
	if err != nil {
		if errors.Is(err, pipeline.ErrAssertionNotFound) {
			writeError(w, http.StatusNotFound, core.ApiError{
				Code:    core.ApiErrorCodeRecordNotFound,
				Message: "no assertion matches the identifier",
			})
			return
		}
		writeStoreError(w, err, "storing the revision failed", "the revision is not storable")
		return
	}
	h.recordedRevision(assertion)
	writeJSON(w, http.StatusOK, h.item(assertion, h.addedRelations()))
}

// assertionChangedResponse は assertion_changed の応答である。Conflict は現在の所見を、
// 所見の応答と同じ形で持つ。
type assertionChangedResponse struct {
	core.ApiError
	Conflict assertionItem `json:"conflict"`
}

// writeAssertionChanged は、別の分析者が先に記録した所見を 409 で返す。
//
// **グラフの組み直しを始めない。** 保存先は何も記録していない。
func (h assertionsHandler) writeAssertionChanged(
	w http.ResponseWriter, current core.Assertion, message string,
) {
	writeJSON(w, http.StatusConflict, assertionChangedResponse{
		ApiError: core.ApiError{Code: core.ApiErrorCodeAssertionChanged, Message: message},
		Conflict: h.item(current, h.addedRelations()),
	})
}

func (h assertionsHandler) item(
	assertion core.Assertion, added pipeline.AddedRelations,
) assertionItem {
	return assertionItem{
		Assertion: assertion, TargetOrigin: h.resolver.Origin(assertion, added),
	}
}

// addedRelations は保存先の全件から、分析者が足した関係の集合を組む。
func (h assertionsHandler) addedRelations() pipeline.AddedRelations {
	return pipeline.NewAddedRelations(h.store.List())
}

// sameAssertionTarget は 2 つの対象が同じ対象を指すかを返す。
func sameAssertionTarget(left, right core.AssertionTarget) bool {
	leftParts, rightParts := left.DigestParts(), right.DigestParts()
	if len(leftParts) != len(rightParts) {
		return false
	}
	for index, part := range leftParts {
		if part != rightParts[index] {
			return false
		}
	}
	return true
}

// decodeAssertionBody は要求の本文を、未知の項目と後続の値を退けて読む。
//
// 未知の項目を無視すると、綴り誤りを含む要求が既定値のまま受理され、分析者が書いた
// 根拠が保存先に入らないまま応答が 201 を返す。
//
// 著者はログインした利用者の要求ではセッションのログイン名に置き換える (requestAuthor)。
func decodeAssertionBody(r *http.Request) (assertionRequestBody, *core.ApiError) {
	body, apiError := decodeAssertionBodyText(r)
	if apiError != nil {
		return assertionRequestBody{}, apiError
	}
	author, apiError := requestAuthor(r, body.Author)
	if apiError != nil {
		return assertionRequestBody{}, apiError
	}
	body.Author = author
	return body, nil
}

func decodeAssertionBodyText(r *http.Request) (assertionRequestBody, *core.ApiError) {
	invalid := func(message string) *core.ApiError {
		return &core.ApiError{Code: core.ApiErrorCodeInvalidRequest, Message: message}
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, requestBodyLimit))
	decoder.DisallowUnknownFields()
	var body assertionRequestBody
	if err := decoder.Decode(&body); err != nil {
		return assertionRequestBody{}, invalid("the request body is not a readable assertion")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return assertionRequestBody{}, invalid("the request body carries a value after its JSON object")
	}
	if body.State != nil && !body.State.IsKnown() {
		return assertionRequestBody{}, invalid("state is not a known value")
	}
	return body, nil
}

// checkAssertionsParameterNames は一覧の要求の項目の名前と多重度を確かめる。
//
// 未知の項目は綴り誤りを通知せずに既定値で処理する経路になるため退ける。
func checkAssertionsParameterNames(query url.Values) error {
	for name, values := range query {
		if name != matchConditionParam {
			return errors.New("unsupported query parameter")
		}
		if len(values) != 1 && !repeatedRequestItem(name) {
			return errors.New("query parameter must occur once")
		}
	}
	return nil
}
