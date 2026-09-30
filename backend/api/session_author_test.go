package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/api"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// signedInHandler は、調査の API をセッションの wrapper で包み、alice のセッションの cookie を
// すべての要求に付ける。
func signedInHandler(t *testing.T) http.Handler {
	t.Helper()
	handler := api.NewSessionHandler(graphHandler(t), newFakeAccounts())
	cookie := login(t, handler, "alice", "alice-password")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(cookie)
		handler.ServeHTTP(w, r)
	})
}

// withoutAuthor は要求の本文から著者の項目を除く。
func withoutAuthor(t *testing.T, body string) string {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatal(err)
	}
	delete(decoded, "author")
	encoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

// ログインした利用者の所見と改訂は、セッションのログイン名を著者に持つ。
func TestSignedInAssertionsCarryTheLoginAsTheAuthor(t *testing.T) {
	handler := signedInHandler(t)
	observed := edgeOfState(t, graphTargets(t, handler), string(core.RelationStateObserved))
	body := withoutAuthor(t, edgeTargetBody(observed, "同じ秒の別の候補を採る"))

	created := decodeAssertionItem(t, requestAssertion(t, handler, http.MethodPost, assertionsPath, body),
		http.StatusCreated)
	if created.Assertion.Author != "alice" {
		t.Fatalf("author=%q want the login", created.Assertion.Author)
	}
	var revision map[string]any
	if err := json.Unmarshal([]byte(body), &revision); err != nil {
		t.Fatal(err)
	}
	revision["state"] = string(core.AssertionStateWithdrawn)
	revision["baseRevision"] = created.Assertion.RevisionNumber
	encoded, err := json.Marshal(revision)
	if err != nil {
		t.Fatal(err)
	}
	revised := decodeAssertionItem(t, requestAssertion(t, handler, http.MethodPut,
		assertionsPath+"/"+created.Assertion.Id, string(encoded)), http.StatusOK)
	if revised.Assertion.Author != "alice" {
		t.Fatalf("revision author=%q want the login", revised.Assertion.Author)
	}
}

// ログインした利用者の要求は、本文で別の著者を名乗れない。
func TestSignedInRequestsCannotNameAnotherAuthor(t *testing.T) {
	handler := signedInHandler(t)
	observed := edgeOfState(t, graphTargets(t, handler), string(core.RelationStateObserved))
	response := requestAssertion(t, handler, http.MethodPost, assertionsPath, edgeTargetBody(observed, "note"))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("assertion status=%d body=%s", response.Code, response.Body)
	}
	requestJSON(t, handler, http.MethodPost, terminalAssignmentsPath, analystAssignmentBody(t, handler),
		http.StatusBadRequest)
}

// ログインした利用者の端末割当は、セッションのログイン名を著者に持つ。
func TestSignedInTerminalAssignmentsCarryTheLoginAsTheAuthor(t *testing.T) {
	handler := signedInHandler(t)
	created := requestJSON(t, handler, http.MethodPost, terminalAssignmentsPath,
		[]byte(withoutAuthor(t, string(analystAssignmentBody(t, handler)))), http.StatusCreated)
	var assignment core.TerminalAssignment
	decodeInto(t, created, &assignment)
	if assignment.Author != "alice" {
		t.Fatalf("author=%q want the login", assignment.Author)
	}
}
