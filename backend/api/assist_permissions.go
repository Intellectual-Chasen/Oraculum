package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// 送信の許可の操作の pattern。一覧と記録は同じ path を method で分ける。
//
// **画面が使う path であり、中継が退ける会話の path の外に置く。** 中継を起動していない分析者も
// 許可を取り消せる。
const (
	assistPermissionsPattern      = "GET /api/v0/assist-permissions"
	assistPermissionRecordPattern = "POST /api/v0/assist-permissions"
)

// assistPermissionsHandler は、証拠を外部の LLM の提供者へ送る許可を、提供者ごとに一覧し記録する。
type assistPermissionsHandler struct {
	store pipeline.AssistStore
}

// assistPermissionsResponse は送信の許可の一覧の応答である。**本型が項目の定義元である。**
//
// 提供者の定義の順に、すべての提供者を 1 件ずつ返す。改訂を持たない提供者も出る。
type assistPermissionsResponse struct {
	Providers []assistProviderPermission `json:"providers"`
}

// assistProviderPermission は提供者 1 つへの送信の許可の現在の状態と、記録した順の改訂である。
type assistProviderPermission struct {
	Provider core.AssistProvider `json:"provider"`
	// Permitted は最後の改訂が許可であることである。改訂を持たない提供者は偽である。
	Permitted bool                            `json:"permitted"`
	Revisions []core.AssistPermissionRevision `json:"revisions"`
}

// assistPermissionRequestBody は送信の許可の改訂を記録する要求の本文である。
type assistPermissionRequestBody struct {
	Provider core.AssistProvider         `json:"provider"`
	Action   core.AssistPermissionAction `json:"action"`
	// Analyst はログインした要求では省き、server がセッションの利用者を記録する。
	Analyst string `json:"analyst"`
}

func (h assistPermissionsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.store.Available() {
		writeError(w, http.StatusConflict, core.ApiError{
			Code:    core.ApiErrorCodeAssistUnavailable,
			Message: "the launch keeps no investigation, so it records no assist permission",
		})
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.list(w, r)
	case http.MethodPost:
		h.record(w, r)
	default:
		writeError(w, http.StatusInternalServerError, core.ApiError{
			Code: core.ApiErrorCodeInternalError, Message: "the method reached the wrong handler",
		})
	}
}

// list は提供者ごとの送信の許可を返す。
func (h assistPermissionsHandler) list(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.Query()) != 0 {
		writeError(w, http.StatusBadRequest, core.ApiError{
			Code: core.ApiErrorCodeInvalidRequest, Message: "the operation takes no query parameter",
		})
		return
	}
	revisions, err := h.store.PermissionRevisions()
	if err != nil {
		writeStoreError(w, err, "reading the assist permissions failed", "the assist permissions are not readable")
		return
	}
	response := assistPermissionsResponse{Providers: make([]assistProviderPermission, 0, len(core.AssistProviders()))}
	for _, provider := range core.AssistProviders() {
		response.Providers = append(response.Providers, providerPermissionOf(revisions, provider))
	}
	writeJSON(w, http.StatusOK, response)
}

// record は送信の許可の新しい改訂を 1 つ記録し、その提供者の状態を返す。
func (h assistPermissionsHandler) record(w http.ResponseWriter, r *http.Request) {
	body, apiError := decodeAssistPermissionBody(r)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	if !body.Provider.IsKnown() || !body.Action.IsKnown() {
		writeError(w, http.StatusBadRequest, core.ApiError{
			Code:    core.ApiErrorCodeInvalidRequest,
			Message: "provider and action must be values the contract defines",
		})
		return
	}
	analyst, apiError := requestAuthor(r, body.Analyst)
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	if _, err := h.store.RecordPermission(pipeline.AssistPermissionDraft{
		Provider: body.Provider, Action: body.Action, Analyst: analyst,
	}); err != nil {
		writeStoreError(w, err, "storing the assist permission failed", "the assist permission is not storable")
		return
	}
	revisions, err := h.store.PermissionRevisions()
	if err != nil {
		writeStoreError(w, err, "reading the assist permissions failed", "the assist permissions are not readable")
		return
	}
	writeJSON(w, http.StatusCreated, providerPermissionOf(revisions, body.Provider))
}

// providerPermissionOf は改訂の並びから提供者 1 つの状態を組む。
func providerPermissionOf(
	revisions []core.AssistPermissionRevision, provider core.AssistProvider,
) assistProviderPermission {
	permission := assistProviderPermission{
		Provider: provider, Permitted: core.AssistPermitted(revisions, provider),
		Revisions: []core.AssistPermissionRevision{},
	}
	for _, revision := range revisions {
		if revision.Provider == provider {
			permission.Revisions = append(permission.Revisions, revision)
		}
	}
	return permission
}

// decodeAssistPermissionBody は要求の本文を、未知の項目と後続の値を退けて読む。
func decodeAssistPermissionBody(r *http.Request) (assistPermissionRequestBody, *core.ApiError) {
	invalid := func(message string) *core.ApiError {
		return &core.ApiError{Code: core.ApiErrorCodeInvalidRequest, Message: message}
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, requestBodyLimit))
	decoder.DisallowUnknownFields()
	var body assistPermissionRequestBody
	if err := decoder.Decode(&body); err != nil {
		return assistPermissionRequestBody{}, invalid("the request body is not a readable assist permission")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return assistPermissionRequestBody{}, invalid("the request body carries a value after its JSON object")
	}
	return body, nil
}
