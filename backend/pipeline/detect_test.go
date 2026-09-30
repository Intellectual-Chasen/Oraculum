// in-package test: 入力形式の候補を、形式ごとの標本で確かめる。
package pipeline

import (
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// detectionParsers は server が登録する入力形式の表である。
func detectionParsers(t *testing.T) map[core.FormatKey]ParserFactory {
	t.Helper()
	parsers, err := NewFormatRegistry(MarkIIFormats(), SquidFormats(), ApacheFormats(), RoyalTSFormats(),
		AuditdFormats(), WindowsEventFormats(), PrefetchFormats(), RegistryFormats())
	if err != nil {
		t.Fatal(err)
	}
	return parsers
}

// 形式ごとの標本。host は example.test 系、address は RFC 5737 の文書用範囲である。
const (
	detectMarkII = "04/05/2024 06:07:08.009 +0000 sn=9 evt=file subEvt=close tmid=t\r\n" +
		"04/05/2024 06:07:09.009 +0000 sn=10 evt=file subEvt=close tmid=t\r\n"
	detectSquid        = `192.0.2.17 - - [05/Apr/2024:06:07:08 +0000] "GET http://sample.test/ HTTP/1.1" 200 3 "-" "run-test" TCP_MISS:HIER_DIRECT` + "\n"
	detectApacheAccess = `203.0.113.10 - - [01/Feb/2000:11:20:10 +0900] "GET /files/archive.zip HTTP/1.1" 200 2048 "-" "example-agent/1.0"` + "\n"
	detectApacheError  = `[Tue Feb 01 11:22:33.123456 2000] [cgi:error] [pid 4242:tid 24] [client 203.0.113.10:50100] AH01215: example message` + "\n"
	detectAuditd       = "type=SYSCALL msg=audit(1000000000.100:1): ppid=900 pid=1000 comm=\"bash\" exe=\"/usr/bin/bash\"\n" +
		"type=EXECVE msg=audit(1000000000.100:1): argc=1 a0=\"bash\"\n"
	// detectAuditdEnriched は auditd の拡張形式である。解釈した値の前に 0x1D を置き、先頭が事象の途中
	// (SYSCALL の行を持たない事象) から始まる。
	detectAuditdEnriched = "type=CWD msg=audit(1000000000.050:1): cwd=\"/home/analyst\"\n" +
		"type=SYSCALL msg=audit(1000000000.100:2): arch=c000003e syscall=59 success=yes exit=0 ppid=900 pid=1000 " +
		"uid=0 comm=\"bash\" exe=\"/usr/bin/bash\"\x1dARCH=x86_64 SYSCALL=execve UID=\"root\"\n" +
		"type=EXECVE msg=audit(1000000000.100:2): argc=1 a0=\"bash\"\n" +
		"type=PATH msg=audit(1000000000.100:2): item=0 name=\"/usr/bin/bash\" nametype=NORMAL\x1dOUID=\"root\"\n"
	detectEventXML = `<?xml version="1.0" encoding="utf-8"?>` + "\n<Events>\n" +
		`<Event><System><Provider Name="Microsoft-Windows-Security-Auditing"/><EventID>4688</EventID>` +
		`<TimeCreated SystemTime="2001-02-03T04:05:06.1234567Z"/><EventRecordID>301</EventRecordID>` +
		`<Computer>host01.example.test</Computer></System><EventData>` +
		`<Data Name="NewProcessId">0x10</Data></EventData></Event>` + "\n</Events>\n"
	detectRoyalTS = `<RTSZDocument><RoyalFolder><RoyalRDSConnection>` +
		`<ID>00000000-0000-0000-0000-000000000001</ID><Name>Example RDP</Name>` +
		`<URI>host01.example.test</URI></RoyalRDSConnection></RoyalFolder></RTSZDocument>`
)

// 形式ごとの標本は、その形式だけを候補にする。
func TestFormatDetectorNamesTheFormatOfEachSample(t *testing.T) {
	detector := newFormatDetector(detectionParsers(t))
	evtx, err := os.ReadFile("../internal/testdata/winevent/process-creation.evtx")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		sample    string
		truncated bool
		want      core.FormatKey
	}{
		{"Mark II", detectMarkII, false, MarkIIFormatKey},
		{"Squid", detectSquid, false, "squid_combined"},
		{"Apache access", detectApacheAccess, false, "apache_access_combined"},
		{"Apache error", detectApacheError, false, "apache_error"},
		{"auditd", detectAuditd, false, "linux_auditd"},
		{"auditd enriched", detectAuditdEnriched, false, "linux_auditd"},
		{"event XML", detectEventXML, false, "windows_event_xml"},
		{"Royal TS", detectRoyalTS, false, "royalts_rds_connection"},
		// 閉じない引用符のレコードを 1 件持つ。
		{"event viewer CSV", viewerCSVDocument, false, WindowsEventCSVFormatKey},
		{"EVTX", string(evtx[:min(len(evtx), detectionSampleBytes)]), len(evtx) > detectionSampleBytes, "windows_evtx"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detector.detect([]byte(tt.sample), tt.truncated)
			if !slices.Equal(got.candidates, []core.FormatKey{tt.want}) {
				t.Errorf("candidates = %v, want [%s]", got.candidates, tt.want)
			}
		})
	}
}

// panicParser は、読むと panic するパーサーである。
type panicParser struct{ ParserIdentity }

func (p panicParser) Identity() ParserIdentity { return p.ParserIdentity }
func (panicParser) Reset(io.Reader)            {}
func (panicParser) Next() (ParsedRecord, *core.ImportFailure, error) {
	panic("synthetic parser panic")
}

// 試し読みで panic した形式は候補にせず、ほかの形式の判定を続ける。
func TestFormatDetectorSkipsAPanickingParser(t *testing.T) {
	parsers := detectionParsers(t)
	parsers["panicking"] = func(*string) (SourceParser, error) { return panicParser{}, nil }
	got := newFormatDetector(parsers).detect([]byte(detectMarkII), false)
	if !slices.Equal(got.candidates, []core.FormatKey{MarkIIFormatKey}) {
		t.Errorf("candidates = %v, want [%s]", got.candidates, MarkIIFormatKey)
	}
}

// 読めるレコードが半分に満たない標本と、テキストでない標本は候補を持たない。
func TestFormatDetectorLeavesUnreadableSamplesWithoutCandidates(t *testing.T) {
	detector := newFormatDetector(detectionParsers(t))
	tests := []struct {
		name      string
		sample    string
		truncated bool
		wantKind  string
	}{
		{"prose", "hello world\nsample text\n", false, "text"},
		{"mixed formats", detectMarkII + detectSquid + detectApacheAccess + detectApacheError + detectAuditd, false, "text"},
		{"binary", "\x00\x01\x02\x03", false, ""},
		{"zip", "PK\x03\x04\x14\x00", false, "zip"},
		// 改行を持たずに切れた標本は、行を単位に読む形式のどれにも当たらない。
		{"truncated without a line end", strings.Repeat("a", 32), true, "text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detector.detect([]byte(tt.sample), tt.truncated)
			if len(got.candidates) != 0 || got.detectedKind != tt.wantKind {
				t.Errorf("detect = %+v, want no candidate and the kind %q", got, tt.wantKind)
			}
		})
	}
}

// 途中で切れた標本は、切れた行を除いて試し読みする。
func TestFormatDetectorDropsTheCutLineOfATruncatedSample(t *testing.T) {
	detector := newFormatDetector(detectionParsers(t))
	sample := detectMarkII + detectMarkII[:20]
	got := detector.detect([]byte(sample), true)
	if !slices.Equal(got.candidates, []core.FormatKey{MarkIIFormatKey}) {
		t.Errorf("candidates = %v, want [%s]", got.candidates, MarkIIFormatKey)
	}
}
