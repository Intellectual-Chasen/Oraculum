package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// 分析者が与える端末の割当の操作の pattern。一覧と記録は同じ path を method で分ける。
const (
	terminalAssignmentsPattern      = "GET /api/v0/terminal-assignments"
	terminalAssignmentCreatePattern = "POST /api/v0/terminal-assignments"
)

// terminalAssignmentsHandler は利用者が与えた端末の割当を一覧し、画面から入力された割当を
// 記録する。
//
// **取り込み結果を書き換えない。** 割当は保存先が別に保ち、端末の判定に足す材料として
// 読まれる。収集元のレコードが記録した割当は取り込み結果から出るものであり、本 handler を
// 通らない。
type terminalAssignmentsHandler struct {
	// result は取り込みの起動で指定した割当と、割当が指してよい収集元を持つ。
	result pipeline.ImportResult
	store  pipeline.TerminalAssignmentStore
	// recorded は割当を 1 件記録した直後に呼ぶ。グラフの組み直しを、次の要求を待たずに始める。
	recorded func()
}

// terminalAssignmentsResponse は割当の一覧の応答である。**本型が項目の定義元である。**
type terminalAssignmentsResponse struct {
	Assignments     []core.TerminalAssignment `json:"assignments"`
	AssignmentCount int64                     `json:"assignmentCount"`
}

// terminalAssignmentRequestBody は割当を記録する要求の本文である。
//
// 由来を要求が与えない。本 path から入る割当は分析者が与えたものであり、保存先が
// analyst_supplied に決める。
type terminalAssignmentRequestBody struct {
	ClientIp         string `json:"clientIp"`
	TerminalId       string `json:"terminalId"`
	TerminalHostname string `json:"terminalHostname"`
	// TerminalHostnames は端末が名乗るホスト名 (短い名前と FQDN) の並びである。省ける。
	TerminalHostnames    []string                  `json:"terminalHostnames,omitempty"`
	SourceId             string                    `json:"sourceId"`
	SourceContentSha256  string                    `json:"sourceContentSha256"`
	AssignmentValidRange core.TimeRange            `json:"assignmentValidRange"`
	Derivation           string                    `json:"derivation"`
	BasisRecordRefs      []core.AssertionRecordRef `json:"basisRecordRefs"`
	Author               string                    `json:"author"`
	// AppliesToSourceId は、レコードの全体がこの端末のものである収集元である。
	// 自機の識別子を持たない収集元へ端末を与えるときに入れる。
	AppliesToSourceId string `json:"appliesToSourceId,omitempty"`
}

func (b terminalAssignmentRequestBody) draft() pipeline.TerminalAssignmentDraft {
	return pipeline.TerminalAssignmentDraft{
		ClientIp:             b.ClientIp,
		TerminalId:           b.TerminalId,
		TerminalHostname:     b.TerminalHostname,
		TerminalHostnames:    b.TerminalHostnames,
		SourceId:             b.SourceId,
		SourceContentSha256:  b.SourceContentSha256,
		AssignmentValidRange: b.AssignmentValidRange,
		Derivation:           b.Derivation,
		BasisRecordRefs:      b.BasisRecordRefs,
		Author:               b.Author,
		AppliesToSourceId:    b.AppliesToSourceId,
	}
}

func (h terminalAssignmentsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.list(w)
	case http.MethodPost:
		h.create(w, r)
	default:
		writeError(w, http.StatusInternalServerError, core.ApiError{
			Code: core.ApiErrorCodeInternalError, Message: "the method reached the wrong handler",
		})
	}
}

// list は取り込みの起動で指定した割当を収集元の入力順に並べ、その後ろに画面から記録した
// 割当を記録した順に並べて返す。
//
// ページ分けを持たない。1 つの調査で利用者が与える割当は端末の台数に収まり、
// 上限を適用する対象が無い。
func (h terminalAssignmentsHandler) list(w http.ResponseWriter) {
	recorded, _ := h.store.List()
	assignments := append(h.result.ImportSpecifiedTerminalAssignments(), recorded...)
	writeJSON(w, http.StatusOK, terminalAssignmentsResponse{
		Assignments: emptyAssignments(assignments), AssignmentCount: int64(len(assignments)),
	})
}

// create は割当を 1 件記録する。
func (h terminalAssignmentsHandler) create(w http.ResponseWriter, r *http.Request) {
	body, apiError := decodeTerminalAssignmentBody(r)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	if apiError := h.sourceProblem(body); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	author, apiError := requestAuthor(r, body.Author)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	body.Author = author
	assignment, err := h.store.Create(body.draft())
	if err != nil {
		writeAssignmentStoreError(w, err)
		return
	}
	h.recorded()
	writeJSON(w, http.StatusCreated, assignment)
}

// sourceProblem は、割当が取り込み結果に無い収集元を指す要求を退ける。
//
// **期間を読み取った収集元は、要求が持つ内容の識別と一致する。** 同じ sourceId の収集元が
// 別の内容を持つ要求を受け入れると、割当が指す端末のノード (収集元の内容の識別で識別する)
// と、割当を付けた収集元が食い違う。
func (h terminalAssignmentsHandler) sourceProblem(
	body terminalAssignmentRequestBody,
) *core.ApiError {
	unknown := &core.ApiError{
		Code:    core.ApiErrorCodeInvalidRequest,
		Message: "the terminal assignment names a source the investigation does not hold",
	}
	identity, found := h.result.Identity(body.SourceId)
	if !found || identity.ContentSha256 != body.SourceContentSha256 {
		return unknown
	}
	if body.AppliesToSourceId != "" {
		if _, found := h.result.Identity(body.AppliesToSourceId); !found {
			return unknown
		}
	}
	return nil
}

// writeAssignmentStoreError は保存の失敗を、要求起因と内部の破れに分けて返す。
//
// **要求起因の失敗だけを個別に 400 と 409 にする。** 保存先の内部の失敗を 400 で返すと、
// 分析者が入力を直しても記録できない状態が、入力の誤りとして出る。
func writeAssignmentStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, pipeline.ErrTerminalAssignmentAlreadyRecorded) {
		apiError := core.ApiError{
			Code:    core.ApiErrorCodeTerminalAssignmentAlreadyRecorded,
			Message: "the same terminal assignment is already recorded",
		}
		writeError(w, httpStatusFor(apiError.Code), apiError)
		return
	}
	if errors.Is(err, pipeline.ErrTerminalAssignmentNotStorable) {
		writeError(w, http.StatusBadRequest, core.ApiError{
			Code: core.ApiErrorCodeInvalidRequest, Message: "the terminal assignment is not storable",
		})
		return
	}
	// 内部不変条件の破れであり、応答は理由を持たない。err は保存先の状態だけを持ち、
	// 分析者が書いた文字列を持たない。
	slog.Error("storing the terminal assignment failed", "error", err)
	writeError(w, http.StatusInternalServerError, core.ApiError{
		Code: core.ApiErrorCodeInternalError, Message: "storing the terminal assignment failed",
	})
}

// decodeTerminalAssignmentBody は要求の本文を読む。未知の項目を持つ本文を退ける。
func decodeTerminalAssignmentBody(
	r *http.Request,
) (terminalAssignmentRequestBody, *core.ApiError) {
	invalid := func(message string) *core.ApiError {
		return &core.ApiError{Code: core.ApiErrorCodeInvalidRequest, Message: message}
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, requestBodyLimit))
	decoder.DisallowUnknownFields()
	var body terminalAssignmentRequestBody
	if err := decoder.Decode(&body); err != nil {
		return terminalAssignmentRequestBody{},
			invalid("the request body is not a readable terminal assignment")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return terminalAssignmentRequestBody{},
			invalid("the request body carries a value after its JSON object")
	}
	return body, nil
}

// emptyAssignments は要素数 0 の集合を null にせず集合として出す。
func emptyAssignments(assignments []core.TerminalAssignment) []core.TerminalAssignment {
	if assignments == nil {
		return []core.TerminalAssignment{}
	}
	return assignments
}
