package core_test

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// semanticEntry は語彙の 1 項目の期待値である。
type semanticEntry struct {
	key    core.SemanticKey
	object core.SemanticObject
	role   core.SemanticRole
}

// semanticVocabulary は語彙の全項目の期待値である。
//
// **実装の対応表を参照せずに literal で持つ。** 参照すると、実装が語彙を変えたときに
// 期待値が追随して検査が通る。
func semanticVocabulary() []semanticEntry {
	return []semanticEntry{
		{"terminal.id", "terminal", "identity"},
		{"terminal.hostname", "terminal", "label"},
		{"terminal.ip_address", "ip", "identity"},
		{"terminal.security_id", "terminal", "attribute"},
		{"terminal.os_product_name", "terminal", "attribute"},
		{"terminal.os_display_version", "terminal", "attribute"},
		{"terminal.os_build", "terminal", "attribute"},
		{"terminal.os_ubr", "terminal", "attribute"},
		{"terminal.os_edition", "terminal", "attribute"},

		{"process.id", "process", "identity"},
		{"process.pid", "process", "attribute"},
		{"process.binary_path", "process", "label"},
		{"process.command_line", "process", "attribute"},
		{"process.start_time", "process", "attribute"},
		{"process.user_name", "process", "attribute"},
		{"process.user_domain", "process", "attribute"},
		{"process.window_title", "process", "attribute"},
		{"process.clipboard_data", "process", "attribute"},
		{"process.decoded_command_line", "process", "attribute"},
		{"process.shell_command", "process", "attribute"},
		{"process.session_id", "process", "attribute"},
		{"parent_process.id", "process", "attribute"},
		{"parent_process.pid", "process", "attribute"},
		{"parent_process.binary_path", "process", "attribute"},
		{"injection_target_process.id", "process", "attribute"},
		{"injection_target_process.binary_path", "process", "attribute"},
		{"process_binary.md5", "process", "attribute"},
		{"process_binary.sha1", "process", "attribute"},
		{"process_binary.sha256", "process", "attribute"},
		{"process_binary.company", "process", "attribute"},
		{"process_binary.copyright", "process", "attribute"},
		{"process_binary.description", "process", "attribute"},
		{"process_binary.file_version", "process", "attribute"},
		{"process_binary.product", "process", "attribute"},
		{"process_binary.product_version", "process", "attribute"},
		{"process_binary.signer", "process", "attribute"},
		{"process_binary.certificate_issuer", "process", "attribute"},
		{"process_binary.signature_validity", "process", "attribute"},
		{"process_binary.created_time", "process", "attribute"},
		{"process_binary.accessed_time", "process", "attribute"},
		{"process_binary.modified_time", "process", "attribute"},

		{"remote_session.client_hostname", "terminal", "attribute"},
		{"remote_session.client_address", "terminal", "attribute"},
		{"remote_session.client_account_name", "terminal", "attribute"},

		{"file.path", "file", "identity"},
		{"file.destination_path", "file", "identity"},
		{"file.name", "file", "label"},
		{"file.size_bytes", "file", "attribute"},
		{"file.created_time", "file", "attribute"},
		{"file.modified_time", "file", "attribute"},
		{"file.accessed_time", "file", "attribute"},
		{"file.md5", "file", "attribute"},
		{"file.sha1", "file", "attribute"},
		{"file.sha256", "file", "attribute"},
		{"file.product_name", "file", "attribute"},
		{"file.product_version", "file", "attribute"},
		{"file.original_file_name", "file", "attribute"},
		{"file.publisher", "file", "attribute"},
		{"file.link_time", "file", "attribute"},

		{"registry_value.key_path", "registry_value", "identity"},
		{"registry_value.name", "registry_value", "label"},
		{"registry_value.data", "registry_value", "attribute"},
		{"registry_value.data_type", "registry_value", "attribute"},

		{"account.sid", "account", "identity"},
		{"account.name", "account", "identity"},
		{"account.domain", "account", "identity"},
		{"subject_account.sid", "account", "attribute"},
		{"subject_account.name", "account", "attribute"},
		{"subject_account.domain", "account", "attribute"},
		{"target_account.sid", "account", "attribute"},
		{"target_account.name", "account", "attribute"},
		{"target_account.domain", "account", "attribute"},

		{"share.name", "event", "attribute"},
		{"scheduled_task.name", "event", "attribute"},
		{"started_task.name", "event", "attribute"},
		{"scheduled_task.command", "event", "attribute"},
		{"service.name", "event", "attribute"},
		{"target_group.sid", "event", "attribute"},
		{"target_group.name", "event", "attribute"},
		{"target_group.domain", "event", "attribute"},

		{"connection.source_address", "ip", "identity"},
		{"connection.source_port", "connection", "attribute"},
		{"connection.destination_address", "ip", "identity"},
		{"connection.destination_port", "connection", "attribute"},
		{"connection.destination_server_name", "connection", "attribute"},
		{"connection.destination_hostname", "domain", "identity"},
		{"connection.destination_reverse_lookup_name", "connection", "attribute"},
		{"connection.protocol", "connection", "attribute"},
		{"connection.sent_bytes", "connection", "attribute"},
		{"connection.received_bytes", "connection", "attribute"},

		{"event.time", "event", "attribute"},
		{"event.category", "event", "attribute"},
		{"event.action", "event", "attribute"},
		{"event.message", "event", "attribute"},
		{"event.account_name", "event", "attribute"},
		{"event.account_domain", "event", "attribute"},
		{"event.operation_start_time", "event", "attribute"},
		{"event.read_bytes", "event", "attribute"},
		{"event.written_bytes", "event", "attribute"},
		{"event.logon_type", "event", "attribute"},
		{"event.logon_failure_status", "event", "attribute"},
		{"event.logon_failure_sub_status", "event", "attribute"},
		{"event.outbound_account_name", "event", "attribute"},
		{"event.outbound_account_domain", "event", "attribute"},
		{"event.authentication_package", "event", "attribute"},
		{"event.target_logon_id", "event", "attribute"},
		{"event.subject_logon_id", "event", "attribute"},
		{"event.target_linked_logon_id", "event", "attribute"},
		{"event.logoff_logon_id", "event", "attribute"},
		{"event.session_start_time", "event", "attribute"},
		{"event.ticket_logon_guid", "event", "attribute"},
		{"event.target_logon_guid", "event", "attribute"},

		{"windows_event.id", "event", "attribute"},
		{"windows_event.channel", "event", "attribute"},
		{"windows_event.record_id", "event", "attribute"},
		{"windows_event.provider", "event", "attribute"},

		{"record.sequence_number", "record", "attribute"},

		{"http.request_method", "http", "attribute"},
		{"http.request_url", "http", "attribute"},
		{"http.request_decoded_url", "http", "attribute"},
		{"http.request_version", "http", "attribute"},
		{"http.request_bytes", "http", "attribute"},
		{"http.response_bytes", "http", "attribute"},
		{"http.status_code", "http", "attribute"},
		{"http.user_agent", "http", "attribute"},
		{"http.referrer", "http", "attribute"},
	}
}

// 語彙の項目のすべてが既知であり、対象と役割が表と一致することを確かめる。
//
// **期待の表と実装の対応表を両方向で突き合わせる。** 片側だけに項目が増えた状態を、
// 増えた側によらず見つける。
func TestSemanticKeyCarriesTheObjectAndTheRoleOfEveryItem(t *testing.T) {
	vocabulary := semanticVocabulary()
	if len(vocabulary) == 0 {
		t.Fatal("the expected table holds no item")
	}
	seen := make(map[core.SemanticKey]bool, len(vocabulary))
	for _, entry := range vocabulary {
		if seen[entry.key] {
			t.Fatalf("the expected table holds %q twice", entry.key)
		}
		seen[entry.key] = true
		if !entry.key.IsKnown() {
			t.Errorf("%q is not known", entry.key)
			continue
		}
		if got := entry.key.Object(); got != entry.object {
			t.Errorf("%q has the object %q, want %q", entry.key, got, entry.object)
		}
		if got := entry.key.Role(); got != entry.role {
			t.Errorf("%q has the role %q, want %q", entry.key, got, entry.role)
		}
	}
	// 実装の対応表にあって期待の表に無い項目を見つける。
	for _, key := range core.SemanticVocabularyKeys() {
		if !seen[key] {
			t.Errorf("the vocabulary holds %q, which the expected table omits", key)
		}
	}
}

// 語彙の外の値が既知にならず、対象と役割を持たないことを確かめる。
func TestSemanticKeyRejectsTheValuesOutsideTheVocabulary(t *testing.T) {
	outside := []core.SemanticKey{
		"",
		"terminal",
		"terminal.",
		"terminal.id ",
		"Terminal.Id",
		"terminal.mac_address",
		"tmid",
		"registry_key.path",
		// ログオンの種別は event.logon_type が持つ。
		"windows_event.logon_type",
		"session.client_hostname",
	}
	for _, key := range outside {
		if key.IsKnown() {
			t.Errorf("%q is known", key)
		}
		if got := key.Object(); got != "" {
			t.Errorf("%q has the object %q, want none", key, got)
		}
		if got := key.Role(); got != "" {
			t.Errorf("%q has the role %q, want none", key, got)
		}
	}
}

// ノードの識別鍵に入る項目の一覧を固定する。
//
// 期待値は literal で持つ。実装の役割を語彙の順で集め、一覧と突き合わせる。
func TestSemanticKeyFixesTheIdentityItems(t *testing.T) {
	want := []core.SemanticKey{
		"terminal.id",
		"terminal.ip_address",
		"process.id",
		"file.path",
		"file.destination_path",
		"registry_value.key_path",
		"account.sid",
		"account.name",
		"account.domain",
		"connection.source_address",
		"connection.destination_address",
		"connection.destination_hostname",
	}
	got := make([]core.SemanticKey, 0, len(want))
	for _, entry := range semanticVocabulary() {
		if entry.key.Role() == core.SemanticRoleIdentity {
			got = append(got, entry.key)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("the identity items are %v, want %v", got, want)
	}
}

// 表示名になる項目の一覧を固定する。期待値は同じ表の表示名の列である。
func TestSemanticKeyFixesTheLabelItems(t *testing.T) {
	want := []core.SemanticKey{
		"terminal.hostname",
		"process.binary_path",
		"file.name",
		"registry_value.name",
	}
	got := make([]core.SemanticKey, 0, len(want))
	for _, entry := range semanticVocabulary() {
		if entry.key.Role() == core.SemanticRoleLabel {
			got = append(got, entry.key)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("the label items are %v, want %v", got, want)
	}
}

// RecordField が語彙の外の semantic を拒否し、出ていない semantic を通すことを確かめる。
func TestRecordFieldAcceptsTheKnownSemanticAndRejectsTheUnknownOne(t *testing.T) {
	field := presentText(t, "dstIP", "198.51.100.42")

	if err := field.Validate(); err != nil {
		t.Fatalf("the field without a semantic is invalid: %v", err)
	}

	known := field
	known.Semantic = core.SemanticKeyConnectionDestinationAddress
	if err := known.Validate(); err != nil {
		t.Fatalf("the field with a known semantic is invalid: %v", err)
	}

	unknown := field
	unknown.Semantic = "connection.destination_ip"
	err := unknown.Validate()
	if !errors.Is(err, core.ErrUnknownEnumValue) {
		t.Fatalf("the field with an unknown semantic returned %v, want ErrUnknownEnumValue", err)
	}
}

// semantic を持たない RecordField の直列化が、項目を 1 つも足さないことを確かめる。
// 段階 1 が semantic を付けるまで、応答の byte 列は変わらない。
func TestRecordFieldWithoutSemanticSerializesWithoutTheItem(t *testing.T) {
	field := presentText(t, "dstPort", "80")
	encoded, err := json.Marshal(field)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"name":"dstPort","kind":"text",` +
		`"text":{"rawText":"80","valueState":"present"}}`
	if string(encoded) != want {
		t.Fatalf("the field serialized to %s, want %s", encoded, want)
	}
}

// semantic を持つ RecordField を復元でき、語彙の外の値を持つ object を復元しないことを
// 確かめる。
func TestRecordFieldDecodesTheSemanticItem(t *testing.T) {
	const known = `{"name":"dstIP","semantic":"connection.destination_address",` +
		`"kind":"text","text":{"rawText":"198.51.100.42","valueState":"present"}}`
	var decoded core.RecordField
	if err := json.Unmarshal([]byte(known), &decoded); err != nil {
		t.Fatalf("the object with a known semantic did not decode: %v", err)
	}
	if decoded.Semantic != core.SemanticKeyConnectionDestinationAddress {
		t.Fatalf("semantic=%q want %q", decoded.Semantic,
			core.SemanticKeyConnectionDestinationAddress)
	}

	const unknown = `{"name":"dstIP","semantic":"connection.destination_ip",` +
		`"kind":"text","text":{"rawText":"198.51.100.42","valueState":"present"}}`
	var rejected core.RecordField
	if err := json.Unmarshal([]byte(unknown), &rejected); !errors.Is(err, core.ErrUnknownEnumValue) {
		t.Fatalf("the object with an unknown semantic returned %v, want ErrUnknownEnumValue", err)
	}
}
