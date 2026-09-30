package royalts_test

import (
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/royalts"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func connectionOf(t *testing.T, document string) royalts.Connection {
	t.Helper()
	var reader royalts.Reader
	reader.Reset(strings.NewReader(document))
	connection, failure, err := reader.Next()
	if err != nil {
		t.Fatalf("Next() returned error %v", err)
	}
	if failure != nil {
		t.Fatalf("Next() returned failure %+v", failure)
	}
	return connection
}

func TestConnectionFieldsHostnameURI(t *testing.T) {
	connection := connectionOf(t, singleConnectionDocument)
	fields := royalts.ConnectionFields(connection)
	byName := make(map[string]core.RecordField, len(fields))
	for _, field := range fields {
		byName[field.Name] = field
		if err := field.Validate(); err != nil {
			t.Errorf("field %s failed Validate(): %v", field.Name, err)
		}
	}
	uri, ok := byName["uri"]
	if !ok {
		t.Fatal("uri field is absent")
	}
	if uri.Semantic != core.SemanticKeyConnectionDestinationHostname {
		t.Errorf("uri.Semantic = %q, want %q", uri.Semantic, core.SemanticKeyConnectionDestinationHostname)
	}
	user, ok := byName["credentialUsername"]
	if !ok {
		t.Fatal("credentialUsername field is absent")
	}
	if user.Semantic != core.SemanticKeyAccountName {
		t.Errorf("credentialUsername.Semantic = %q, want %q", user.Semantic, core.SemanticKeyAccountName)
	}
	if raw, _ := user.Text.RawTextValue(); raw != "exampleuser" {
		t.Errorf("credentialUsername raw = %q, want exampleuser", raw)
	}
	for _, field := range fields {
		if field.Name == "credentialPassword" {
			t.Fatal("ConnectionFields must never carry a credentialPassword field")
		}
	}
}

func TestConnectionFieldsIPAddressURI(t *testing.T) {
	document := `<RTSZDocument><RoyalFolder>` +
		`<RoyalRDSConnection><Name>Direct</Name><URI>203.0.113.20</URI></RoyalRDSConnection>` +
		`</RoyalFolder></RTSZDocument>`
	connection := connectionOf(t, document)
	fields := royalts.ConnectionFields(connection)
	for _, field := range fields {
		if field.Name != "uri" {
			continue
		}
		if field.Semantic != core.SemanticKeyConnectionDestinationAddress {
			t.Errorf("uri.Semantic = %q, want %q", field.Semantic, core.SemanticKeyConnectionDestinationAddress)
		}
		return
	}
	t.Fatal("uri field is absent")
}

func TestConnectionFieldsMissingItemsAreItemAbsent(t *testing.T) {
	document := `<RTSZDocument><RoyalFolder>` +
		`<RoyalRDSConnection><Name>NoCredential</Name></RoyalRDSConnection>` +
		`</RoyalFolder></RTSZDocument>`
	connection := connectionOf(t, document)
	fields := royalts.ConnectionFields(connection)
	byName := make(map[string]core.RecordField, len(fields))
	for _, field := range fields {
		byName[field.Name] = field
	}
	for _, name := range []string{"uri", "credentialUsername"} {
		field, ok := byName[name]
		if !ok {
			t.Errorf("%s field is absent from ConnectionFields()", name)
			continue
		}
		if field.Text.ValueState != core.ValueStateItemAbsent {
			t.Errorf("%s.Text.ValueState = %q, want %q", field.Name, field.Text.ValueState, core.ValueStateItemAbsent)
		}
	}
}
