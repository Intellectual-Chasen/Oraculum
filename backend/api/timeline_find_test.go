package api_test

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// rawTextOf は行の原文を操作の応答から読む。
func rawTextOf(t *testing.T, handler http.Handler, entry timelineEntry) string {
	t.Helper()
	var decoded struct {
		RawText string `json:"rawText"`
	}
	response := requestPath(t, handler, http.MethodGet,
		"/api/v0/raw-texts?ref="+url.QueryEscape(entry.RecordRef.RecordRawTextRef))
	decodeInto(t, response.Body.Bytes(), &decoded)
	return decoded.RawText
}

// 原文に文字列を含む行の位置を昇順に返す。大文字と小文字を区別しない要求は、区別する要求が
// 見つけない大文字の文字列でも同じ行を見つける。文字列を与えない要求は位置を返さない。
func TestTimelineFindsTheRowsWhoseRawTextContainsTheText(t *testing.T) {
	handler := graphHandler(t)
	whole := decodeTimeline(t, handler, "")
	if whole.FindMatches != nil {
		t.Fatalf("a request without find returned findMatches=%v", *whole.FindMatches)
	}
	var word string
	for _, field := range strings.FieldsFunc(rawTextOf(t, handler, whole.Entries[0]), func(r rune) bool {
		return !unicode.IsLetter(r)
	}) {
		if len(field) >= 4 && strings.ToLower(field) == field {
			word = field
			break
		}
	}
	if word == "" {
		t.Fatal("the first raw text has no lowercase word of four letters")
	}

	found := decodeTimeline(t, handler, "find="+url.QueryEscape(word))
	if found.FindMatches == nil || len(*found.FindMatches) == 0 || (*found.FindMatches)[0] != 0 {
		t.Fatalf("findMatches=%v, want the first row among them", found.FindMatches)
	}
	if !slices.IsSorted(*found.FindMatches) {
		t.Errorf("findMatches=%v are not ascending", *found.FindMatches)
	}
	for _, index := range *found.FindMatches {
		if !strings.Contains(strings.ToLower(rawTextOf(t, handler, found.Entries[index])), word) {
			t.Errorf("the row %d does not contain %q", index, word)
		}
	}

	upper := strings.ToUpper(word)
	sensitive := decodeTimeline(t, handler,
		"find="+url.QueryEscape(upper)+"&findCaseSensitive=true")
	insensitive := decodeTimeline(t, handler,
		"find="+url.QueryEscape(upper)+"&findCaseSensitive=false")
	if slices.Contains(*sensitive.FindMatches, 0) {
		t.Errorf("the case-sensitive request found %q in the first row, which holds %q", upper, word)
	}
	if !slices.Equal(*insensitive.FindMatches, *found.FindMatches) {
		t.Errorf("the case-insensitive request found %v, want %v",
			*insensitive.FindMatches, *found.FindMatches)
	}

	absent := decodeTimeline(t, handler, "find="+url.QueryEscape("\x01absent\x01"))
	if absent.FindMatches == nil || len(*absent.FindMatches) != 0 {
		t.Errorf("a text in no row returned findMatches=%v, want an empty list", absent.FindMatches)
	}
}

// 空の文字列、文字列の無い区別の指定、true と false 以外の区別の値を invalid_request で退ける。
func TestTimelineRejectsAnInvalidFind(t *testing.T) {
	handler := graphHandler(t)
	for _, query := range []string{
		"find=", "findCaseSensitive=true", "find=abc&findCaseSensitive=yes",
	} {
		apiError := requestTimelineError(t, handler, query, http.StatusBadRequest)
		if apiError.Code != core.ApiErrorCodeInvalidRequest {
			t.Errorf("%s: code=%q want invalid_request", query, apiError.Code)
		}
	}
}
