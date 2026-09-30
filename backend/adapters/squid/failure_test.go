package squid_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func checkUnknownPosition(t *testing.T, failure *core.ImportFailure) {
	t.Helper()
	if failure == nil {
		t.Fatal("missing failure")
	}
	if failure.LineNumber != nil || failure.ByteOffset != nil {
		t.Fatal("unknown position was filled with zero")
	}
	if !errors.Is(failure.Validate(), core.ErrMissingRequiredItem) {
		t.Fatal("incomplete diagnosis must require caller context")
	}
	completed := *failure
	completed.SourceId = "synthetic-source"
	completed.SourceContentSha256 = strings.Repeat("a", 64)
	completed.ParserVersion = "test-parser"
	completed.SanitizedMessage = "test diagnosis"
	if err := completed.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
		t.Fatalf("unknown position is invalid: %v", err)
	}
	// 呼び出し側が原資料の位置を確認して補完する。
	line := int64(1)
	completed.LineNumber = &line
	completed.RawTextRef = "synthetic-record"
	completed.RecordRef = &core.RecordLocator{
		SourceId: completed.SourceId, SourceContentSha256: completed.SourceContentSha256,
		SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber,
		LineNumber: &line, RecordRawTextRef: "synthetic-record",
	}
	if err := completed.Validate(); err != nil {
		t.Fatalf("completed diagnosis is invalid: %v", err)
	}
}
