// in-package test: 取り込みの表を作る非公開の組み立てと、package の外へ出さない
// 走査器の作り方を、同じ package の test が共有する。
package pipeline

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 入力形式の識別子の文字列。宣言の定義元は adapter が持ち、本 package の test は adapter を
// import しないため (backend/.golangci.yml の
// pipeline-test-adapter-import-is-forbidden)、利用者が起動引数に書く文字列をそのまま置く。
// 宣言と文字列が一致することは adapter の test が確かめる。
const (
	MarkIIFormatKey               core.FormatKey = "infotrace_mark_ii"
	SquidFormatKey                core.FormatKey = "squid_combined"
	SquidRequestBytesFormatKey    core.FormatKey = "squid_combined_request_bytes"
	SquidLogFormatKey             core.FormatKey = "squid_logformat"
	ApacheAccessCombinedFormatKey core.FormatKey = "apache_access_combined"
	ApacheErrorFormatKey          core.FormatKey = "apache_error"
	RoyalTSRDSConnectionFormatKey core.FormatKey = "royalts_rds_connection"
	AuditdFormatKeyText           core.FormatKey = "linux_auditd"
	WindowsEventXMLFormatKey      core.FormatKey = "windows_event_xml"
	WindowsEventCSVFormatKey      core.FormatKey = "windows_event_viewer_csv"
	WindowsEVTXFormatKey          core.FormatKey = "windows_evtx"
	WindowsPrefetchFormatKey      core.FormatKey = "windows_prefetch"
	WindowsRegistryHiveFormatKey  core.FormatKey = "windows_registry_hive"
)

// TestingFormatRegistry は adapter の宣言から取り込みの表を作る。
var TestingFormatRegistry = NewTestFormatRegistry()

func NewTestFormatRegistry() map[core.FormatKey]ParserFactory {
	registry, err := NewFormatRegistry(MarkIIFormats(), SquidFormats(), ApacheFormats(),
		RoyalTSFormats(), AuditdFormats(), WindowsEventFormats(), PrefetchFormats(), RegistryFormats())
	if err != nil {
		panic(err)
	}
	return registry
}

// 宣言を集めた表は、adapter が名乗った識別子をすべて持つ。
func TestFormatRegistryCarriesEveryDeclaredKey(t *testing.T) {
	declared := []core.FormatKey{
		MarkIIFormatKey, SquidFormatKey, SquidRequestBytesFormatKey, SquidLogFormatKey,
		ApacheAccessCombinedFormatKey, ApacheErrorFormatKey, RoyalTSRDSConnectionFormatKey,
		AuditdFormatKeyText, WindowsEventXMLFormatKey, WindowsEventCSVFormatKey, WindowsEVTXFormatKey,
		WindowsPrefetchFormatKey, WindowsRegistryHiveFormatKey,
	}
	registry := NewTestFormatRegistry()
	for _, key := range declared {
		if registry[key] == nil {
			t.Errorf("the registry carries no factory for %q", key)
		}
	}
	if len(registry) != len(declared) {
		t.Errorf("the registry carries %d keys; the declared keys are %v", len(registry), declared)
	}
}

// 同じ識別子を 2 つの宣言が名乗る表は作れない。どちらの adapter が読むかが決まらない。
func TestFormatRegistryRejectsADuplicateKey(t *testing.T) {
	if _, err := NewFormatRegistry(SquidFormats(), SquidFormats()); err == nil {
		t.Fatal("registering the same declaration twice returned no error")
	}
}

// mustParser は宣言した入力形式の走査器を作る。表に無い識別子と、宣言と食い違う並びの
// 指定は test の組み立ての誤りであるため panic にする。
func NewTestParser(key core.FormatKey, spec *string) SourceParser {
	factory := TestingFormatRegistry[key]
	if factory == nil {
		panic("no parser factory for " + string(key))
	}
	parser, err := factory(spec)
	if err != nil {
		panic(err)
	}
	return parser
}

func NewTestMarkIIParser() SourceParser { return NewTestParser(MarkIIFormatKey, nil) }

func NewTestSquidParser() SourceParser { return NewTestParser(SquidFormatKey, nil) }

// NewTestSquidRequestBytesParser は要求の byte 数を 1 欄持つ並びの走査器を作る。
func NewTestSquidRequestBytesParser() SourceParser {
	return NewTestParser(SquidRequestBytesFormatKey, nil)
}

// NewTestSquidLogFormatParser は取り込みの指定が渡した並びの走査器を作る。
func NewTestSquidLogFormatParser(spec string) SourceParser {
	return NewTestParser(SquidLogFormatKey, &spec)
}

func NewTestApacheAccessParser() SourceParser {
	return NewTestParser(ApacheAccessCombinedFormatKey, nil)
}

func NewTestApacheErrorParser() SourceParser { return NewTestParser(ApacheErrorFormatKey, nil) }

func NewTestRoyalTSParser() SourceParser { return NewTestParser(RoyalTSRDSConnectionFormatKey, nil) }

func NewTestPrefetchParser() SourceParser { return NewTestParser(WindowsPrefetchFormatKey, nil) }
