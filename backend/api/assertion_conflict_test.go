// in-package test: グラフの組み直しを始める関数を差し替えるため、非公開の handler を組む。
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// fixedAssertionClock は記録のたびに 1 秒進む時計である。
type fixedAssertionClock struct{ ticks int64 }

func (c *fixedAssertionClock) Now() core.AssertionTime {
	c.ticks++
	return core.NewAssertionTime(time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC).
		Add(time.Duration(c.ticks) * time.Second))
}

// 改訂の要求は baseRevision を必須とし、古い baseRevision の改訂を 409 assertion_changed で退け、
// 現在の所見を conflict に入れる。退けた改訂ではグラフの組み直しを始めない。
func TestAssertionRevisionRejectsAStaleBaseRevision(t *testing.T) {
	store := pipeline.NewMemoryAssertionStore(&fixedAssertionClock{})
	offset := core.UtcOffset("+09:00")
	target := core.AssertionTarget{Kind: core.AssertionTargetKindSource, SourceContentSha256: strings.Repeat("a", 64)}
	created, err := store.Create(pipeline.AssertionDraft{
		Target: target, Author: "analyst-a", Basis: core.AssertionBasis{Note: "合成の根拠"}, TimeOffset: &offset,
	})
	if err != nil {
		t.Fatal(err)
	}
	invalidated := 0
	handler := assertionsHandler{
		store: store, resolver: pipeline.NewAssertionResolver(pipeline.Graph{}),
		timeInterpreted: func() { invalidated++ },
	}
	revise := func(body map[string]any) *httptest.ResponseRecorder {
		t.Helper()
		body["target"], body["author"], body["state"], body["timeOffset"] =
			target, "analyst-b", string(core.AssertionStateActive), "+00:00"
		body["basis"] = map[string]any{"note": "別の分析者の根拠"}
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPut, "/api/v0/assertions/"+created.Id, strings.NewReader(string(encoded)))
		request.SetPathValue(assertionIdPathValue, created.Id)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}

	missing := revise(map[string]any{})
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 without baseRevision body=%s", missing.Code, missing.Body.String())
	}
	var missingError core.ApiError
	if err := json.Unmarshal(missing.Body.Bytes(), &missingError); err != nil {
		t.Fatal(err)
	}
	if len(missingError.MissingParameters) != 1 || missingError.MissingParameters[0] != "baseRevision" {
		t.Errorf("missingParameters = %v, want [baseRevision]", missingError.MissingParameters)
	}
	if first := revise(map[string]any{"baseRevision": created.RevisionNumber}); first.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 on the current base body=%s", first.Code, first.Body.String())
	}
	if invalidated != 1 {
		t.Fatalf("invalidated=%d want 1 after the stored revision", invalidated)
	}

	stale := revise(map[string]any{"baseRevision": created.RevisionNumber})
	if stale.Code != http.StatusConflict {
		t.Fatalf("status=%d want 409 on a stale base body=%s", stale.Code, stale.Body.String())
	}
	var response struct {
		Code     core.ApiErrorCode `json:"code"`
		Conflict assertionItem     `json:"conflict"`
	}
	if err := json.Unmarshal(stale.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != core.ApiErrorCodeAssertionChanged ||
		response.Conflict.Assertion.RevisionNumber != created.RevisionNumber+1 ||
		response.Conflict.Assertion.Author != "analyst-b" {
		t.Errorf("the conflict answered %+v, want assertion_changed with the current revision", response)
	}
	if invalidated != 1 {
		t.Errorf("invalidated=%d want the rejected revision to leave the graph alone", invalidated)
	}
	if current, _ := store.Find(created.Id); current.RevisionNumber != created.RevisionNumber+1 {
		t.Errorf("revisionNumber=%d want the rejected revision to stay out", current.RevisionNumber)
	}
}
