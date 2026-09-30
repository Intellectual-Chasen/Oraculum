package pipeline_test

import (
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// Royal TS 文書。RFC 5737 の文書用範囲と example.test 系の文字列を使う。
const royalTSScanDocument = `<RTSZDocument><RoyalFolder>` +
	`<RoyalRDSConnection>` +
	`<ID>00000000-0000-0000-0000-000000000001</ID>` +
	`<Name>Example RDP</Name>` +
	`<URI>host01.example.test</URI>` +
	`<CredentialUsername>exampleuser</CredentialUsername>` +
	`<CredentialPassword>AQAAANCMnd8BFdERjHoAwE_example_ciphertext</CredentialPassword>` +
	`</RoyalRDSConnection>` +
	`</RoyalFolder></RTSZDocument>`

func TestRoyalTSParserIdentity(t *testing.T) {
	got := pipeline.NewTestRoyalTSParser().Identity()
	if got.FormatKey != pipeline.RoyalTSRDSConnectionFormatKey {
		t.Errorf("FormatKey = %q, want %q", got.FormatKey, pipeline.RoyalTSRDSConnectionFormatKey)
	}
	if got.PositionKind != core.PositionKindSequenceNumber {
		t.Errorf("PositionKind = %q, want %q", got.PositionKind, core.PositionKindSequenceNumber)
	}
	if len(got.ConnectionMatchConditions) != 0 {
		t.Errorf("ConnectionMatchConditions = %v, want an empty set", got.ConnectionMatchConditions)
	}
}

// CredentialUsername と接続先ホストがグラフに載る fields として含まれ、
// CredentialPassword は含まれない (ユーザ確認済み)。
func TestRoyalTSParserCarriesUsernameAndHost(t *testing.T) {
	parser := pipeline.NewTestRoyalTSParser()
	parser.Reset(strings.NewReader(royalTSScanDocument))
	record, failure, err := parser.Next()
	if err != nil || failure != nil {
		t.Fatalf("Next() = %+v, %v", failure, err)
	}
	if record.Semantics == nil {
		t.Fatal("the record carries no semantics")
	}
	if record.SequenceNumber == nil || *record.SequenceNumber != 1 {
		t.Errorf("SequenceNumber = %v, want a pointer to 1", record.SequenceNumber)
	}
	byName := make(map[string]core.RecordField, len(record.Semantics.Fields))
	for _, field := range record.Semantics.Fields {
		byName[field.Name] = field
		if field.Name == "credentialPassword" {
			t.Fatal("the record must never carry a credentialPassword field")
		}
	}
	user, ok := byName["credentialUsername"]
	if !ok {
		t.Fatal("credentialUsername field is absent")
	}
	if user.Semantic != core.SemanticKeyAccountName {
		t.Errorf("credentialUsername.Semantic = %q, want %q", user.Semantic, core.SemanticKeyAccountName)
	}
	uri, ok := byName["uri"]
	if !ok {
		t.Fatal("uri field is absent")
	}
	if uri.Semantic != core.SemanticKeyConnectionDestinationHostname {
		t.Errorf("uri.Semantic = %q, want %q", uri.Semantic, core.SemanticKeyConnectionDestinationHostname)
	}
}
