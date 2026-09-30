package apache_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/apache"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func TestErrorRecordFields(t *testing.T) {
	record := errorRecordOf(t, errorFullShapeLine)
	timestamp, failure := apache.ParseErrorTime(record)
	if failure != nil {
		t.Fatalf("ParseErrorTime() returned failure %+v", failure)
	}
	fields := apache.ErrorRecordFields(record, timestamp)
	byName := make(map[string]core.RecordField, len(fields))
	for _, field := range fields {
		byName[field.Name] = field
		if err := field.Validate(); err != nil {
			t.Errorf("field %s failed Validate(): %v", field.Name, err)
		}
	}

	clientIP, ok := byName[string(apache.ErrorItemClientIP)]
	if !ok {
		t.Fatal("clientIp field is absent")
	}
	if clientIP.Semantic != core.SemanticKeyConnectionSourceAddress {
		t.Errorf("clientIp.Semantic = %q, want %q", clientIP.Semantic, core.SemanticKeyConnectionSourceAddress)
	}

	pid, ok := byName[string(apache.ErrorItemPid)]
	if !ok {
		t.Fatal("pid field is absent")
	}
	if pid.Semantic != core.SemanticKeyProcessPid {
		t.Errorf("pid.Semantic = %q, want %q", pid.Semantic, core.SemanticKeyProcessPid)
	}

	message, ok := byName[string(apache.ErrorItemMessage)]
	if !ok {
		t.Fatal("message field is absent")
	}
	if message.Semantic != core.SemanticKeyEventMessage {
		t.Errorf("message.Semantic = %q, want %q", message.Semantic, core.SemanticKeyEventMessage)
	}

	timeField, ok := byName[string(apache.ErrorItemTime)]
	if !ok {
		t.Fatal("time field is absent")
	}
	if timeField.Kind != core.RecordFieldKindTimestamp {
		t.Errorf("time.Kind = %q, want %q", timeField.Kind, core.RecordFieldKindTimestamp)
	}
}
