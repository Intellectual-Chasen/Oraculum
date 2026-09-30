package api_test

import (
	"net/http"
	"net/url"
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 収集元で絞った時系列は、その収集元の行と収録範囲だけを持ち、用いた収集元を応答に返す。
// 取り込んでいない収集元は invalid_request で退ける。
func TestTimelineNarrowsToTheRequestedSources(t *testing.T) {
	handler := graphHandler(t)
	whole := decodeTimeline(t, handler, "")
	chosen := whole.Entries[0].RecordRef.SourceId

	narrowed := decodeTimeline(t, handler, "source="+url.QueryEscape(chosen))
	if narrowed.EntryCount == 0 || narrowed.EntryCount >= whole.EntryCount {
		t.Fatalf("the narrowed timeline carries %d of %d rows", narrowed.EntryCount, whole.EntryCount)
	}
	for _, entry := range narrowed.Entries {
		if entry.RecordRef.SourceId != chosen {
			t.Fatalf("the narrowed timeline carries a row of %q", entry.RecordRef.SourceId)
		}
	}
	for _, coverage := range narrowed.SourceCoverages {
		if coverage.SourceId != chosen {
			t.Errorf("the narrowed timeline carries the coverage of %q", coverage.SourceId)
		}
	}
	if !slices.Equal(narrowed.Sources, []string{chosen}) {
		t.Errorf("the response returned source=%v, want [%s]", narrowed.Sources, chosen)
	}

	response := requestTimeline(t, handler, "source="+url.QueryEscape("no-such-source"))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("an unknown source returned status=%d", response.Code)
	}
	var failure core.ApiError
	decodeJSON(t, response.Body, &failure)
	if failure.Code != core.ApiErrorCodeInvalidRequest {
		t.Errorf("code=%s, want invalid_request", failure.Code)
	}
}
