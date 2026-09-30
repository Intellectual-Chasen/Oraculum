package api_test

import (
	"net/http"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// Squid の収集元。接続元 IP は RFC 5737、host は example.test、文字列は "ABCDEF" の
// base64url を 2 つに分けた値である。
const urlFragmentsDocument = `192.0.2.10 - - [03/Feb/2001:04:10:00 +0000] "GET http://files.example.test/0/QUJD HTTP/1.1" ` +
	`200 3 "-" "agent" TCP_MISS:HIER_DIRECT` + "\n" +
	`192.0.2.10 - - [03/Feb/2001:04:10:01 +0000] "GET http://files.example.test/1/REVG HTTP/1.1" ` +
	`200 3 "-" "agent" TCP_MISS:HIER_DIRECT` + "\n"

// urlFragmentsResponse は応答の項目の名前を test の側で固定する。
type urlFragmentsResponse struct {
	EdgeId                string `json:"edgeId"`
	UnnumberedRecordCount int64  `json:"unnumberedRecordCount"`
	Segments              []struct {
		FragmentCount             int64  `json:"fragmentCount"`
		LastNumber                int64  `json:"lastNumber"`
		DuplicateCount            int64  `json:"duplicateCount"`
		ConflictingDuplicateCount int64  `json:"conflictingDuplicateCount"`
		MissingNumberCount        int64  `json:"missingNumberCount"`
		Encoding                  string `json:"encoding"`
		DecodeFailure             string `json:"decodeFailure"`
		Fragments                 []struct {
			Number         int64  `json:"number"`
			Adopted        bool   `json:"adopted"`
			HttpStatusCode string `json:"httpStatusCode"`
			Truncated      bool   `json:"truncated"`
			// 位置の項目は `/api/v0/records` の test が固定する。
			RecordRef map[string]any `json:"recordRef"`
		} `json:"fragments"`
		Decoded *struct {
			ByteCount       int64  `json:"byteCount"`
			Sha256          string `json:"sha256"`
			LeadingBytesHex string `json:"leadingBytesHex"`
			ContentType     string `json:"contentType"`
			Zip             any    `json:"zip"`
		} `json:"decoded"`
	} `json:"segments"`
}

func TestEdgeUrlFragmentsJoinsTheNumberedPaths(t *testing.T) {
	handler := testHandler(recordsImportResultOfText(t, pipeline.SourcePlan{
		FormatKey: squidFormatKey, FileName: "access.log", OriginPath: "testdata/access.log",
	}, urlFragmentsDocument))
	edge := graphEdgeOfKind(t, handler, "http_request")
	path := edgesPath + edge.Id + "/url-fragments"

	response := requestPath(t, handler, http.MethodGet, withAllMatchConditions(path))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
	var decoded urlFragmentsResponse
	decodeJSON(t, response.Body, &decoded)
	if decoded.EdgeId != edge.Id || len(decoded.Segments) != 1 {
		t.Fatalf("response = %+v", decoded)
	}
	segment := decoded.Segments[0]
	if segment.FragmentCount != 2 || segment.Encoding != "base64url" || segment.Decoded == nil ||
		segment.Decoded.ByteCount != 6 || segment.Decoded.LeadingBytesHex != "414243444546" ||
		segment.Decoded.ContentType != "text/plain; charset=utf-8" {
		t.Errorf("segment = %+v decoded %+v", segment, segment.Decoded)
	}
	if line := segment.Fragments[1].RecordRef["lineNumber"]; line != float64(2) || !segment.Fragments[1].Adopted ||
		segment.Fragments[1].HttpStatusCode != "200" || segment.Fragments[1].Truncated {
		t.Errorf("second fragment = %+v", segment.Fragments[1])
	}

	for query, status := range map[string]int{
		"unknown=1":        http.StatusBadRequest,
		"case=a&case=b":    http.StatusBadRequest,
		"case=absent-case": http.StatusBadRequest,
		"":                 http.StatusOK,
	} {
		target := path
		if query != "" {
			target += "?" + query
		}
		if got := requestPath(t, handler, http.MethodGet, withAllMatchConditions(target)).Code; got != status {
			t.Errorf("query=%q status=%d, want %d", query, got, status)
		}
	}
	missing := requestPath(t, handler, http.MethodGet, withAllMatchConditions(edgesPath+"absent/url-fragments"))
	if missing.Code != http.StatusNotFound {
		t.Errorf("absent edge status=%d, want 404", missing.Code)
	}
}
