// in-package test: 公開 API では構築できない引用符無しの要求行を持つ Record を検証する。
package squid

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func TestParseRequestLineRejectsUnquotedItem(t *testing.T) {
	for _, raw := range []string{"GET / HTTP/1.1", "", "G"} {
		t.Run(raw, func(t *testing.T) {
			record := Record{
				lineNumber: 1,
				items: []Item{{
					name:    ItemRequestLine,
					rawText: raw,
					value:   raw,
				}},
			}
			got, failure := ParseRequestLine(record)
			if failure == nil {
				t.Fatal("unquoted request line must fail")
			}
			if got != (RequestLine{}) {
				t.Fatalf("failed request returned a value: %+v", got)
			}
			if failure.Stage != core.FailureStageNormalize || failure.ExpectedMeaning != "a quoted request line" {
				t.Fatalf("unexpected diagnosis: %+v", failure)
			}
			if failure.LineNumber == nil || *failure.LineNumber != 1 || failure.ByteOffset == nil || *failure.ByteOffset != 0 {
				t.Fatalf("diagnosis lost the request location: %+v", failure)
			}
		})
	}
}
