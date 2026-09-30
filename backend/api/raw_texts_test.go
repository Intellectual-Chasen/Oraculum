package api_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// 取り込めなかったレコードの原文を、失敗が持つ参照で開ける。
func TestRawTextsReturnsTheRawTextOfAFailedRecord(t *testing.T) {
	const broken = "broken line without the items"
	handler := testHandler(recordsImportResultOfText(t, pipeline.SourcePlan{
		FormatKey: squidFormatKey, FileName: "access.log", OriginPath: "testdata/access.log",
	}, broken+"\n"))
	var sources struct {
		Sources []struct {
			ImportStatus core.ImportStatus `json:"importStatus"`
		} `json:"sources"`
	}
	decodeInto(t, requestPath(t, handler, http.MethodGet, "/api/v0/sources").Body.Bytes(), &sources)
	failure := sources.Sources[0].ImportStatus.Failures[0]
	if failure.RecordRef == nil || failure.RecordRef.ByteOffset == nil || *failure.RecordRef.ByteOffset != 0 ||
		failure.RecordRef.ByteLength == nil || *failure.RecordRef.ByteLength != int64(len(broken)) {
		t.Fatalf("failure ref = %+v, want the byte range of the line", failure.RecordRef)
	}

	response := requestPath(t, handler, http.MethodGet, "/api/v0/raw-texts?ref="+url.QueryEscape(failure.RawTextRef))
	var decoded struct {
		RawTextRef string `json:"rawTextRef"`
		RawText    string `json:"rawText"`
	}
	decodeInto(t, response.Body.Bytes(), &decoded)
	if response.Code != http.StatusOK || decoded.RawText != broken || decoded.RawTextRef != failure.RawTextRef {
		t.Errorf("status=%d response=%+v", response.Code, decoded)
	}

	for query, status := range map[string]int{
		"ref=raw:absent":  http.StatusNotFound,
		"":                http.StatusBadRequest,
		"ref=a&ref=b":     http.StatusBadRequest,
		"ref=a&unknown=1": http.StatusBadRequest,
	} {
		if got := requestPath(t, handler, http.MethodGet, "/api/v0/raw-texts?"+query).Code; got != status {
			t.Errorf("%q: status=%d want %d", query, got, status)
		}
	}
}
