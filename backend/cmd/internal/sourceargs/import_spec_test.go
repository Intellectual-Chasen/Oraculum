package sourceargs_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/cmd/internal/sourceargs"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

var exampleSha256 = strings.Repeat("ab", 32)

// 案件の指定は、それより後ろに並ぶ収集元に適用する。
func TestParseAppliesTheCaseToTheFollowingSources(t *testing.T) {
	plans, _, err := sourceargs.Parse([]string{
		"--case", "baseline", "infotrace_mark_ii:logs/a.log",
		"--case", "challenge", "infotrace_mark_ii:logs/b.log", "infotrace_mark_ii:logs/c.log",
	}, failingRead)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, plan := range plans {
		if plan.CaseId == nil {
			t.Fatalf("%q carries no case", plan.OriginPath)
		}
		got[plan.OriginPath] = *plan.CaseId
	}
	want := map[string]string{"logs/a.log": "baseline", "logs/b.log": "challenge", "logs/c.log": "challenge"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the cases = %v, want %v", got, want)
	}
}

// 案件を指定しない起動の計画は案件を持たない。
func TestParseLeavesTheCaseAbsentWithoutTheFlag(t *testing.T) {
	plans, _, err := sourceargs.Parse([]string{"infotrace_mark_ii:logs/a.log"}, failingRead)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || plans[0].CaseId != nil {
		t.Fatalf("the parser read %+v, want one source without a case", plans)
	}
}

// 記録した取り込みの指定を、収集元ごとの計画にする。記録した内容の hash を、取り込む file との突き合わせに渡す。
func TestParseReadsTheImportSpecification(t *testing.T) {
	record := `{"sources":[` +
		`{"caseId":"baseline","formatKey":"infotrace_mark_ii","originPath":"logs/a.log","contentSha256":"` + exampleSha256 + `"},` +
		`{"caseId":"baseline","formatKey":"squid_logformat","originPath":"logs/b.log",` +
		`"formatSpec":"` + strings.ReplaceAll(exampleSpec, `"`, `\"`) + `","contentSha256":"` + exampleSha256 + `"}]}`
	plans, rest, err := sourceargs.Parse(
		[]string{"--import-spec", "spec.json", "--addr", "127.0.0.1:8080"}, readsSpec(record), "--addr")
	if err != nil {
		t.Fatal(err)
	}
	if len(rest) != 2 {
		t.Errorf("the parser left %v, want the declared flag", rest)
	}
	baseline, spec, sha := "baseline", exampleSpec, exampleSha256
	want := []pipeline.SourcePlan{
		{OriginPath: "logs/a.log", FileName: "a.log", FormatKey: "infotrace_mark_ii",
			CaseId: &baseline, ExpectedContentSha256: &sha},
		{OriginPath: "logs/b.log", FileName: "b.log", FormatKey: "squid_logformat",
			FormatSpec: &spec, CaseId: &baseline, ExpectedContentSha256: &sha},
	}
	if !reflect.DeepEqual(plans, want) {
		t.Errorf("the plans = %+v, want %+v", plans, want)
	}
}

// 記録と収集元の引数を一緒に渡す起動と、読めない記録を退ける。
func TestParseRejectsAnUnusableImportSpecification(t *testing.T) {
	valid := `{"sources":[{"formatKey":"infotrace_mark_ii","originPath":"logs/a.log","contentSha256":"` +
		exampleSha256 + `"}]}`
	sourceWith := func(item string) string {
		return `{"sources":[{"formatKey":"infotrace_mark_ii","originPath":"logs/a.log",` + item + `}]}`
	}
	for _, testCase := range []struct {
		name   string
		args   []string
		record string
		reason string
	}{
		{"a source argument", []string{"--import-spec", "s", "infotrace_mark_ii:logs/b.log"}, valid, "takes no source"},
		{"a case flag", []string{"--case", "baseline", "--import-spec", "s"}, valid, "takes no source"},
		{"a format flag", []string{"--import-spec", "s", "--logformat", exampleSpec}, valid, "takes no source"},
		{"two records", []string{"--import-spec", "s", "--import-spec", "s"}, valid, "given twice"},
		{"an unknown item", []string{"--import-spec", "s"},
			sourceWith(`"contentSha256":"` + exampleSha256 + `","sourceId":"x"`), "unknown field"},
		{"no source", []string{"--import-spec", "s"}, `{"sources":[]}`, "sources is empty"},
		{"a missing digest", []string{"--import-spec", "s"}, sourceWith(`"caseId":"baseline"`), "contentSha256"},
		{"an uppercase digest", []string{"--import-spec", "s"},
			sourceWith(`"contentSha256":"` + strings.ToUpper(exampleSha256) + `"`), "contentSha256"},
		{"an empty case", []string{"--import-spec", "s"},
			sourceWith(`"caseId":"","contentSha256":"` + exampleSha256 + `"`), "caseId"},
		{"an absolute path", []string{"--import-spec", "s"},
			`{"sources":[{"formatKey":"f","originPath":"/etc/passwd","contentSha256":"` + exampleSha256 + `"}]}`,
			"relative path"},
		{"trailing data", []string{"--import-spec", "s"}, valid + "{}", "trailing data"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			plans, _, err := sourceargs.Parse(testCase.args, readsSpec(testCase.record))
			if err == nil {
				t.Fatalf("the parser accepted %v as %+v", testCase.args, plans)
			}
			if !strings.Contains(err.Error(), testCase.reason) {
				t.Errorf("the error %q does not carry %q", err, testCase.reason)
			}
		})
	}
}

// 端末の指定は、直後の収集元 1 件だけに付く。3 項目のうち分かるものだけを渡せる。
func TestParseAppliesTheTerminalToTheNextSourceAlone(t *testing.T) {
	plans, _, err := sourceargs.Parse([]string{
		"--terminal-name", "linux-host.example.test", "--terminal-ip", "192.0.2.14", "linux_auditd:logs/a.log",
		"infotrace_mark_ii:logs/b.log",
		"--terminal-id", "T-web", "apache_combined:logs/c.log",
		"--time-offset", "+09:00", "--terminal-id", "T-ad", "windows_event_viewer_csv:logs/d.csv",
		"--time-offset", "+00:00", "windows_event_xml:logs/e.xml",
	}, failingRead)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]*pipeline.SourceTerminal{}
	for _, plan := range plans {
		got[plan.OriginPath] = plan.Terminal
	}
	offset, utc := core.UtcOffset("+09:00"), core.UtcOffset("+00:00")
	want := map[string]*pipeline.SourceTerminal{
		"logs/a.log": {TerminalHostname: "linux-host.example.test", Ip: "192.0.2.14"},
		"logs/b.log": nil,
		"logs/c.log": {TerminalId: "T-web"},
		"logs/d.csv": {TerminalId: "T-ad", TimeOffset: &offset},
		"logs/e.xml": {TimeOffset: &utc},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the terminals = %+v, want %+v", got, want)
	}
}

// 後ろに収集元の無い指定、同じ収集元への 2 回の指定、空の値、読めない IP を退ける。
func TestParseRejectsAnUnusableTerminal(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		args   []string
		reason string
	}{
		{"no following source", []string{"infotrace_mark_ii:logs/a.log", "--terminal-name", "x"},
			"need a source argument"},
		{"the same flag twice", []string{"--terminal-id", "a", "--terminal-id", "b", "infotrace_mark_ii:logs/a.log"},
			"given twice"},
		{"an empty value", []string{"--terminal-name", " ", "infotrace_mark_ii:logs/a.log"}, "non-empty"},
		{"an unreadable IP", []string{"--terminal-ip", "192.0.2.300", "infotrace_mark_ii:logs/a.log"},
			"not an IP address"},
		{"no value", []string{"infotrace_mark_ii:logs/a.log", "--terminal-ip"}, "requires a value"},
		{"with a record", []string{"--terminal-id", "a", "--import-spec", "s"}, "takes no source"},
		{"an unreadable offset", []string{"--time-offset", "+9", "--terminal-id", "a", "infotrace_mark_ii:logs/a.log"},
			"--time-offset"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			plans, _, err := sourceargs.Parse(testCase.args, readsSpec(
				`{"sources":[{"formatKey":"infotrace_mark_ii","originPath":"logs/a.log","contentSha256":"`+
					exampleSha256+`"}]}`))
			if err == nil {
				t.Fatalf("the parser accepted %v as %+v", testCase.args, plans)
			}
			if !strings.Contains(err.Error(), testCase.reason) {
				t.Errorf("the error %q does not carry %q", err, testCase.reason)
			}
		})
	}
}

// 記録した端末を読み戻す。3 項目をすべて欠く端末と、読めない IP を持つ端末を退ける。
func TestParseReadsTheRecordedTerminal(t *testing.T) {
	sourceWith := func(terminal string) string {
		return `{"sources":[{"formatKey":"linux_auditd","originPath":"logs/a.log","contentSha256":"` +
			exampleSha256 + `","terminal":` + terminal + `}]}`
	}
	plans, _, err := sourceargs.Parse([]string{"--import-spec", "s"},
		readsSpec(sourceWith(`{"hostname":"linux-host.example.test","timeOffset":"-05:00"}`)))
	if err != nil {
		t.Fatal(err)
	}
	offset := core.UtcOffset("-05:00")
	want := &pipeline.SourceTerminal{TerminalHostname: "linux-host.example.test", TimeOffset: &offset}
	if len(plans) != 1 || !reflect.DeepEqual(plans[0].Terminal, want) {
		t.Fatalf("the plans = %+v, want one source with the terminal %+v", plans, want)
	}
	for name, terminal := range map[string]string{
		"no item": `{}`, "an unreadable IP": `{"ip":"not-an-ip"}`,
		"a blank name": `{"hostname":"  "}`, "a control character": `{"id":"a\u0007b"}`,
		"an unreadable offset": `{"id":"a","timeOffset":"9"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if plans, _, err := sourceargs.Parse([]string{"--import-spec", "s"},
				readsSpec(sourceWith(terminal))); err == nil {
				t.Errorf("the parser accepted the terminal %s as %+v", terminal, plans)
			}
		})
	}
}

// 案件の文字列を外れる値を退ける。
func TestParseRejectsAnUnreadableCase(t *testing.T) {
	for _, value := range []string{"", "has space", "slash/case", strings.Repeat("a", 65)} {
		plans, _, err := sourceargs.Parse([]string{"--case", value, "infotrace_mark_ii:logs/a.log"}, failingRead)
		if err == nil {
			t.Errorf("the parser accepted the case %q as %+v", value, plans)
		}
	}
}

// 取り込みの記録は、起動が渡した欄の並びと案件と、読んだ内容の識別を持つ。
func TestImportSpecOfRecordsTheRequestedSpecification(t *testing.T) {
	spec, caseId := exampleSpec, "challenge"
	plans := []pipeline.SourcePlan{{OriginPath: "logs/b.log", FileName: "b.log",
		FormatKey: "squid_logformat", FormatSpec: &spec, CaseId: &caseId,
		Terminal: &pipeline.SourceTerminal{TerminalId: "T-proxy", Ip: "192.0.2.8"}}}
	entries := []pipeline.SourceEntry{{Identity: core.SourceIdentity{OriginPath: "logs/b.log", ContentSha256: exampleSha256}}}
	got, err := sourceargs.ImportSpecOf(plans, entries)
	if err != nil {
		t.Fatal(err)
	}
	want := sourceargs.ImportSpec{Sources: []sourceargs.ImportSpecSource{{CaseId: &caseId,
		FormatKey: "squid_logformat", OriginPath: "logs/b.log", FormatSpec: &spec, ContentSha256: exampleSha256,
		Terminal: &sourceargs.ImportSpecTerminal{Id: "T-proxy", Ip: "192.0.2.8"}}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the record = %+v, want %+v", got, want)
	}
	entries[0].Identity.OriginPath = "logs/other.log"
	if _, err := sourceargs.ImportSpecOf(plans, entries); err == nil {
		t.Error("the record paired a source with the plan of another path")
	}
}
