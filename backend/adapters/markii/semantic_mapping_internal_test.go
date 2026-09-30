// in-package test: 対応表の全項目を読むため、非公開の 2 つの表を直接読む。公開 API は
// 1 件のレコードに出た key の意味しか返さず、表に増えた項目を外から数えられない。
package markii

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// wantEventScopedSemantics は evt と組で意味が決まる key の期待値である。
//
// **core の定数を参照せず、語彙の項目を文字列で書く。** 定数を参照すると、実装が別の
// 定数を選んだときに期待値が追随して検査が通る。
var wantEventScopedSemantics = map[eventScopedKey]core.SemanticKey{
	{key: "path", event: "file"}:        "file.path",
	{key: "dstPath", event: "file"}:     "file.destination_path",
	{key: "path", event: "reg"}:         "registry_value.key_path",
	{key: "size", event: "file"}:        "file.size_bytes",
	{key: "sha256", event: "file"}:      "file.sha256",
	{key: "sha1", event: "file"}:        "file.sha1",
	{key: "md5", event: "file"}:         "file.md5",
	{key: "srcIP", event: "net"}:        "connection.source_address",
	{key: "srcPort", event: "net"}:      "connection.source_port",
	{key: "srcIP", event: "session"}:    "connection.source_address",
	{key: "srcPort", event: "session"}:  "connection.source_port",
	{key: "srcIP", event: "os"}:         "connection.source_address",
	{key: "srcPort", event: "os"}:       "connection.source_port",
	{key: "wsIp", event: "os"}:          "connection.source_address",
	{key: "wsPort", event: "os"}:        "connection.source_port",
	{key: "read", event: "file"}:        "event.read_bytes",
	{key: "write", event: "file"}:       "event.written_bytes",
	{key: "crTime", event: "file"}:      "file.created_time",
	{key: "acTime", event: "file"}:      "file.accessed_time",
	{key: "moTime", event: "file"}:      "file.modified_time",
	{key: "sha256", event: "ps"}:        "process_binary.sha256",
	{key: "sha1", event: "ps"}:          "process_binary.sha1",
	{key: "md5", event: "ps"}:           "process_binary.md5",
	{key: "company", event: "ps"}:       "process_binary.company",
	{key: "copyright", event: "ps"}:     "process_binary.copyright",
	{key: "fileDesc", event: "ps"}:      "process_binary.description",
	{key: "fileVer", event: "ps"}:       "process_binary.file_version",
	{key: "product", event: "ps"}:       "process_binary.product",
	{key: "productVer", event: "ps"}:    "process_binary.product_version",
	{key: "signer", event: "ps"}:        "process_binary.signer",
	{key: "issuer", event: "ps"}:        "process_binary.certificate_issuer",
	{key: "sig", event: "ps"}:           "process_binary.signature_validity",
	{key: "crTime", event: "ps"}:        "process_binary.created_time",
	{key: "acTime", event: "ps"}:        "process_binary.accessed_time",
	{key: "moTime", event: "ps"}:        "process_binary.modified_time",
	{key: "url", event: "net"}:          "http.request_url",
	{key: "decode", event: "net"}:       "http.request_decoded_url",
	{key: "url_hostname", event: "net"}: "connection.destination_hostname",
	{key: "sTime", event: "ps"}:         "process.start_time",
	{key: "tpsGUID", event: "ps"}:       "injection_target_process.id",
	{key: "tpsPath", event: "ps"}:       "injection_target_process.binary_path",
	{key: "sTime", event: "file"}:       "event.operation_start_time",
}

// wantPlainSemantics は evt に依らず意味が決まる key の期待値である。
// 書き方は wantEventScopedSemantics と同じである。
var wantPlainSemantics = map[string]core.SemanticKey{
	"sn":         "record.sequence_number",
	"evt":        "event.category",
	"subEvt":     "event.action",
	"tmid":       "terminal.id",
	"com":        "terminal.hostname",
	"csid":       "terminal.security_id",
	"psGUID":     "process.id",
	"psID":       "process.pid",
	"psPath":     "process.binary_path",
	"cmd":        "process.command_line",
	"clipData":   "process.clipboard_data",
	"winTitle":   "process.window_title",
	"evtMsg":     "event.message",
	"evtUsr":     "event.account_name",
	"evtDomain":  "event.account_domain",
	"psUser":     "process.user_name",
	"psDomain":   "process.user_domain",
	"parentGUID": "parent_process.id",
	"parentPath": "parent_process.binary_path",
	"usr":        "account.name",
	"usrDomain":  "account.domain",
	"dstIP":      "connection.destination_address",
	"dstPort":    "connection.destination_port",
	"dstHost":    "connection.destination_hostname",
	"recv":       "connection.received_bytes",
	"send":       "connection.sent_bytes",
	"entry":      "registry_value.name",
	"valType":    "registry_value.data_type",
	"valStr":     "registry_value.data",
	"valNum":     "registry_value.data",
	"sessionID":  "process.session_id",
	"shCmd":      "process.shell_command",
	"evtLogonID": "event.subject_logon_id",
	"rcCom":      "remote_session.client_hostname",
	"srcCom":     "remote_session.client_hostname",
	"wsName":     "remote_session.client_hostname",
	"cliCom":     "remote_session.client_hostname",
	"rcIP":       "remote_session.client_address",
	"cliUsr":     "remote_session.client_account_name",
	"channel":    "windows_event.channel",
	"evtID":      "windows_event.id",
	"evtRecID":   "windows_event.record_id",
	"evtSrc":     "windows_event.provider",
}

// 2 つの対応表の全 entry が、文字列で書いた期待値と一致する。
//
// 表に足りない entry、余分な entry、別の語彙の項目を選んだ entry のすべてで失敗させる。
func TestSemanticMappingFixesEveryEntry(t *testing.T) {
	for scoped, want := range wantEventScopedSemantics {
		got, found := semanticByEventAndKey[scoped]
		if !found {
			t.Errorf("the table carries no entry for (%s, %s)", scoped.key, scoped.event)
			continue
		}
		if got != want {
			t.Errorf("the semantic of (%s, %s) = %q, want %q",
				scoped.key, scoped.event, got, want)
		}
	}
	for scoped := range semanticByEventAndKey {
		if _, wanted := wantEventScopedSemantics[scoped]; !wanted {
			t.Errorf("the table carries an unexpected entry for (%s, %s)",
				scoped.key, scoped.event)
		}
	}
	for key, want := range wantPlainSemantics {
		got, found := semanticByKey[key]
		if !found {
			t.Errorf("the table carries no entry for %s", key)
			continue
		}
		if got != want {
			t.Errorf("the semantic of %s = %q, want %q", key, got, want)
		}
	}
	for key := range semanticByKey {
		if _, wanted := wantPlainSemantics[key]; !wanted {
			t.Errorf("the table carries an unexpected entry for %s", key)
		}
	}
}

// 2 つの対応表が語彙の中の値だけを返す。
func TestSemanticMappingHoldsTheItemsOfTheVocabulary(t *testing.T) {
	for scoped, semantic := range semanticByEventAndKey {
		if !semantic.IsKnown() {
			t.Errorf("the semantic of (%s, %s) = %q, which is outside the vocabulary",
				scoped.key, scoped.event, semantic)
		}
	}
	for key, semantic := range semanticByKey {
		if !semantic.IsKnown() {
			t.Errorf("the semantic of %s = %q, which is outside the vocabulary", key, semantic)
		}
	}
}

// evt と組にした対応を持つ key は、evt に依らない対応を持たない。
//
// 両方の表に同じ key が載ると、表に無い evt のレコードが evt に依らない意味を受け取る。
func TestSemanticMappingKeepsEveryKeyInOneTable(t *testing.T) {
	for scoped := range semanticByEventAndKey {
		if semantic, found := semanticByKey[scoped.key]; found {
			t.Errorf("the key %s carries the event scoped semantic and the semantic %q",
				scoped.key, semantic)
		}
	}
}

// 表に無い key と、表に無い evt の組は空の値になる。
func TestSemanticOfReturnsAnEmptyValueOutsideTheTables(t *testing.T) {
	cases := map[string]struct {
		key   string
		event string
	}{
		"a key outside both tables":         {key: "loc", event: fileEvent},
		"an event outside the scoped table": {key: keyPath, event: "win"},
		"a record without an event":         {key: keyPath, event: ""},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			if semantic := semanticOf(testCase.key, testCase.event); semantic != "" {
				t.Errorf("semantic = %q, want an empty value", semantic)
			}
		})
	}
}
