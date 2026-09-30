package api_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/api"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const assistPermissionsPath = "/api/v0/assist-permissions"

// ログインした利用者が出す許可には server が著者を付け、別の分析者を名乗る本文は退ける。
func TestAssistPermissionsUseTheSignedInAnalyst(t *testing.T) {
	handler := api.NewSessionHandler(assistHandlerOf(t), newFakeAccounts())
	cookie := login(t, handler, "alice", "alice-password")
	for _, body := range []string{
		`{"provider":"claude","action":"grant","analyst":"bob"}`,
		`{"provider":"claude","action":"grant","analyst":"alice"}`,
	} {
		response := serveSession(handler, http.MethodPost, assistPermissionsPath, body, cookie)
		if response.Code != http.StatusBadRequest {
			t.Errorf("supplied analyst = %d %s, want rejection", response.Code, response.Body)
		}
	}
	response := serveSession(handler, http.MethodPost, assistPermissionsPath,
		`{"provider":"claude","action":"grant"}`, cookie)
	if response.Code != http.StatusCreated {
		t.Fatalf("grant without analyst = %d %s", response.Code, response.Body)
	}
	var granted assistProviderPermissionBody
	decodeJSON(t, response.Body, &granted)
	if len(granted.Revisions) != 1 || granted.Revisions[0].Analyst != "alice" {
		t.Fatalf("revisions = %+v, want only the signed-in analyst's grant", granted.Revisions)
	}
}

// assistPermissionsBody は送信の許可の一覧の応答の項目名を test 側で固定する。
type assistPermissionsBody struct {
	Providers []assistProviderPermissionBody `json:"providers"`
}

type assistProviderPermissionBody struct {
	Provider  core.AssistProvider             `json:"provider"`
	Permitted bool                            `json:"permitted"`
	Revisions []core.AssistPermissionRevision `json:"revisions"`
}

// assistHandlerOf は AI 支援の記録を保てる保存先を持つ handler を返す。
func assistHandlerOf(t *testing.T) http.Handler {
	t.Helper()
	store := investigationStore{
		result: graphImportResult(t), assertions: pipeline.NewMemoryAssertionStore(&testAssertionClock{}),
		assignments: pipeline.NewMemoryTerminalAssignmentStore(),
		assist:      pipeline.NewMemoryAssistStore(&testAssertionClock{}),
	}
	return handlerOf(store)
}

func recordAssistPermission(
	t *testing.T, handler http.Handler, action core.AssistPermissionAction, wantStatus int,
) []byte {
	t.Helper()
	return requestJSON(t, handler, http.MethodPost, assistPermissionsPath, encodeBody(t, map[string]any{
		"provider": core.AssistProviderClaude, "action": action, "analyst": "analyst-a",
	}), wantStatus)
}

// 許可と取り消しを改訂の列として残し、現在の状態は最後の改訂が決める。
func TestAssistPermissionsKeepEveryRevision(t *testing.T) {
	handler := assistHandlerOf(t)
	var initial assistPermissionsBody
	decodeJSON(t, bytes.NewReader(requestJSON(t, handler, http.MethodGet, assistPermissionsPath, nil,
		http.StatusOK)), &initial)
	if len(initial.Providers) != len(core.AssistProviders()) {
		t.Fatalf("providers=%+v, want every provider", initial.Providers)
	}
	for index, provider := range core.AssistProviders() {
		got := initial.Providers[index]
		if got.Provider != provider || got.Permitted || len(got.Revisions) != 0 {
			t.Errorf("initial %s = %+v, want no permission and no revision", provider, got)
		}
	}

	var granted assistProviderPermissionBody
	decodeJSON(t, bytes.NewReader(recordAssistPermission(t, handler, core.AssistPermissionActionGrant,
		http.StatusCreated)), &granted)
	if !granted.Permitted || len(granted.Revisions) != 1 ||
		granted.Revisions[0].RevisionNumber != core.FirstAssistPermissionRevisionNumber ||
		granted.Revisions[0].Analyst != "analyst-a" {
		t.Fatalf("granted = %+v", granted)
	}

	var revoked assistProviderPermissionBody
	decodeJSON(t, bytes.NewReader(recordAssistPermission(t, handler, core.AssistPermissionActionRevoke,
		http.StatusCreated)), &revoked)
	if revoked.Permitted || len(revoked.Revisions) != 2 ||
		revoked.Revisions[0].Action != core.AssistPermissionActionGrant ||
		revoked.Revisions[1].Action != core.AssistPermissionActionRevoke ||
		revoked.Revisions[1].RevisionNumber != revoked.Revisions[0].RevisionNumber+1 {
		t.Fatalf("revoked = %+v, want the grant kept before the revocation", revoked)
	}

	var listed assistPermissionsBody
	decodeJSON(t, bytes.NewReader(requestJSON(t, handler, http.MethodGet, assistPermissionsPath, nil,
		http.StatusOK)), &listed)
	if listed.Providers[0].Permitted || len(listed.Providers[0].Revisions) != 2 {
		t.Errorf("listed = %+v, want the revoked state with both revisions", listed.Providers[0])
	}
}

// 定義の外の値と、分析者の名前を欠く要求を退ける。
func TestAssistPermissionsRejectAnInvalidRequest(t *testing.T) {
	handler := assistHandlerOf(t)
	for name, body := range map[string]map[string]any{
		"unknown provider": {"provider": "other", "action": "grant", "analyst": "analyst-a"},
		"unknown action":   {"provider": "claude", "action": "allow", "analyst": "analyst-a"},
		"no analyst":       {"provider": "claude", "action": "grant", "analyst": ""},
		"unknown item":     {"provider": "claude", "action": "grant", "analyst": "analyst-a", "scope": "all"},
	} {
		response := requestJSON(t, handler, http.MethodPost, assistPermissionsPath, encodeBody(t, body),
			http.StatusBadRequest)
		var apiError core.ApiError
		decodeInto(t, response, &apiError)
		if apiError.Code != core.ApiErrorCodeInvalidRequest {
			t.Errorf("%s: code=%q, want invalid_request", name, apiError.Code)
		}
	}
	var listed assistPermissionsBody
	decodeJSON(t, bytes.NewReader(requestJSON(t, handler, http.MethodGet, assistPermissionsPath, nil,
		http.StatusOK)), &listed)
	if len(listed.Providers[0].Revisions) != 0 {
		t.Errorf("the rejected requests recorded %+v", listed.Providers[0].Revisions)
	}
}

// 調査の directory を渡さない起動は、一覧も記録も assist_unavailable で退ける。
func TestAssistPermissionsAreUnavailableWithoutAnInvestigation(t *testing.T) {
	handler := testHandler(graphImportResult(t))
	for _, request := range []struct {
		method string
		body   []byte
	}{
		{http.MethodGet, nil},
		{http.MethodPost, encodeBody(t, map[string]any{
			"provider": "claude", "action": "grant", "analyst": "analyst-a",
		})},
	} {
		response := requestJSON(t, handler, request.method, assistPermissionsPath, request.body,
			http.StatusConflict)
		var apiError core.ApiError
		decodeInto(t, response, &apiError)
		if apiError.Code != core.ApiErrorCodeAssistUnavailable {
			t.Errorf("%s: code=%q, want assist_unavailable", request.method, apiError.Code)
		}
	}
}
