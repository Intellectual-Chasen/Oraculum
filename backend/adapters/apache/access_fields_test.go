package apache_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/apache"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func TestAccessRecordFieldsValueStates(t *testing.T) {
	record := accessRecordOf(t, fullShapeLine)
	timestamp, failure := apache.ParseAccessTime(record)
	if failure != nil {
		t.Fatalf("ParseAccessTime() returned failure %+v", failure)
	}
	fields := apache.AccessRecordFields(record, timestamp)
	byName := make(map[string]core.RecordField, len(fields))
	for _, field := range fields {
		byName[field.Name] = field
	}

	replyBytes, ok := byName[string(apache.AccessItemReplyBytes)]
	if !ok {
		t.Fatal("replyBytes field is absent")
	}
	if replyBytes.Text.ValueState != core.ValueStateNoBody {
		t.Errorf("replyBytes.Text.ValueState = %q, want %q", replyBytes.Text.ValueState, core.ValueStateNoBody)
	}
	if replyBytes.Semantic != core.SemanticKeyHttpResponseBytes {
		t.Errorf("replyBytes.Semantic = %q, want %q", replyBytes.Semantic, core.SemanticKeyHttpResponseBytes)
	}

	referer, ok := byName[string(apache.AccessItemReferer)]
	if !ok {
		t.Fatal("referer field is absent")
	}
	if referer.Text.ValueState != core.ValueStateAbsent {
		t.Errorf("referer.Text.ValueState = %q, want %q", referer.Text.ValueState, core.ValueStateAbsent)
	}

	clientIP, ok := byName[string(apache.AccessItemClientIP)]
	if !ok {
		t.Fatal("clientIp field is absent")
	}
	if clientIP.Semantic != core.SemanticKeyConnectionSourceAddress {
		t.Errorf("clientIp.Semantic = %q, want %q", clientIP.Semantic, core.SemanticKeyConnectionSourceAddress)
	}
	rawText, ok := clientIP.Text.RawTextValue()
	if !ok || rawText != "203.0.113.10" {
		t.Errorf("clientIp.Text.RawTextValue() = %q, %v, want %q, true", rawText, ok, "203.0.113.10")
	}

	if err := (core.RecordField{}).Validate(); err == nil {
		t.Fatal("sanity: zero-value RecordField must fail Validate")
	}
	for _, field := range fields {
		if err := field.Validate(); err != nil {
			t.Errorf("field %s failed Validate(): %v", field.Name, err)
		}
	}
}

func TestAccessRecordFieldsReplyBytesPresent(t *testing.T) {
	line := `203.0.113.10 - - [01/Feb/2000:11:20:15 +0900] "GET /files/archive.zip HTTP/1.1" 200 2048 "-" "-"`
	record := accessRecordOf(t, line)
	timestamp, failure := apache.ParseAccessTime(record)
	if failure != nil {
		t.Fatalf("ParseAccessTime() returned failure %+v", failure)
	}
	fields := apache.AccessRecordFields(record, timestamp)
	for _, field := range fields {
		if field.Name != string(apache.AccessItemReplyBytes) {
			continue
		}
		if field.Text.ValueState != core.ValueStatePresent {
			t.Errorf("ValueState = %q, want %q", field.Text.ValueState, core.ValueStatePresent)
		}
		raw, _ := field.Text.RawTextValue()
		if raw != "2048" {
			t.Errorf("RawTextValue() = %q, want 2048", raw)
		}
		return
	}
	t.Fatal("replyBytes field is absent")
}
