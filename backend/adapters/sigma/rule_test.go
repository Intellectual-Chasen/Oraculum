package sigma_test

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/sigma"
)

// event はレコードである。項目名は Sigma のルールの項目名である。
type event map[string]string

func (e event) value(name string) (string, bool) {
	text, found := e[name]
	return text, found
}

func sysmonEvent(eventID string, fields event) event {
	record := event{sigma.FieldChannel: "Microsoft-Windows-Sysmon/Operational",
		sigma.FieldProvider: "Microsoft-Windows-Sysmon", sigma.FieldEventID: eventID}
	for name, text := range fields {
		record[name] = text
	}
	return record
}

func loadRules(t *testing.T, files map[string]string) sigma.RuleSet {
	t.Helper()
	fsys := fstest.MapFS{}
	for name, content := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(content)}
	}
	set, err := sigma.Load(fsys)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return set
}

// matchedPaths はレコードに一致したルールの path と、一致した検索の名前を返す。
func matchedPaths(set sigma.RuleSet, record event) map[string][]string {
	got := map[string][]string{}
	for _, match := range set.Match(sigma.Record{Value: record.value}).Matches {
		got[set.Rules[match.Rule].Path] = match.Selections
	}
	return got
}

const processRule = `title: Synthetic process rule
id: 00000000-0000-4000-8000-000000000001
author: Synthetic Author
level: high
logsource:
  product: windows
  category: process_creation
detection:
  selection_image:
    Image|endswith: '\synthetic-tool.exe'
  selection_cli:
    CommandLine|contains|all:
      - ' -flag '
      - 'payload'
  filter_parent:
    ParentImage|startswith: 'C:\Trusted\'
  condition: 1 of selection_* and not filter_parent
`

func TestMatchReportsMatchedRuleAndSelections(t *testing.T) {
	set := loadRules(t, map[string]string{"rules/process.yml": processRule})
	if len(set.Unevaluated) != 0 {
		t.Fatalf("Unevaluated = %+v, want none", set.Unevaluated)
	}
	rule := set.Rules[0]
	if rule.ID != "00000000-0000-4000-8000-000000000001" || rule.Title != "Synthetic process rule" ||
		rule.Author != "Synthetic Author" || rule.Level != "high" || rule.Path != "rules/process.yml" {
		t.Fatalf("rule = %+v", rule)
	}
	names := make([]string, 0, len(rule.Selections))
	for _, sel := range rule.Selections {
		names = append(names, sel.Name)
	}
	if !slices.Equal(names, []string{"selection_image", "selection_cli", "filter_parent"}) {
		t.Fatalf("selections = %v", names)
	}
	if !strings.Contains(rule.Selections[0].Definition, `synthetic-tool.exe`) {
		t.Fatalf("definition = %q", rule.Selections[0].Definition)
	}

	// 大文字と小文字を区別しない。
	matching := sysmonEvent("1", event{"Image": `C:\Work\SYNTHETIC-TOOL.EXE`, "CommandLine": "run", "ParentImage": `C:\Shell\parent.exe`})
	if got := matchedPaths(set, matching); !slices.Equal(got["rules/process.yml"], []string{"selection_image"}) || len(got) != 1 {
		t.Fatalf("matching record: %v", got)
	}
	both := sysmonEvent("1", event{"Image": `C:\x\synthetic-tool.exe`, "CommandLine": "a -flag payload", "ParentImage": `C:\Shell\parent.exe`})
	if got := matchedPaths(set, both); !slices.Equal(got["rules/process.yml"], []string{"selection_image", "selection_cli"}) {
		t.Fatalf("both selections: %v", got)
	}
}

func TestMatchRejectsRecordsOutsideTheRule(t *testing.T) {
	set := loadRules(t, map[string]string{"process.yml": processRule})
	for name, record := range map[string]event{
		"filtered parent":  sysmonEvent("1", event{"Image": `C:\x\synthetic-tool.exe`, "ParentImage": `c:\trusted\p.exe`}),
		"other image":      sysmonEvent("1", event{"Image": `C:\x\other.exe`, "CommandLine": "a -flag only"}),
		"all needs both":   sysmonEvent("1", event{"Image": `C:\x\other.exe`, "CommandLine": "payload"}),
		"other event id":   sysmonEvent("3", event{"Image": `C:\x\synthetic-tool.exe`}),
		"unmapped channel": {sigma.FieldChannel: "Synthetic/Operational", sigma.FieldEventID: "1", "Image": `C:\x\synthetic-tool.exe`},
		"field absent":     sysmonEvent("1", event{"CommandLine": "run"}),
	} {
		if got := matchedPaths(set, record); len(got) != 0 {
			t.Errorf("%s: matched %v, want no match", name, got)
		}
	}
}

func TestMatchMapsProcessCreationToSecurityAuditing(t *testing.T) {
	set := loadRules(t, map[string]string{"process.yml": processRule})
	security := event{sigma.FieldChannel: "Security", sigma.FieldProvider: "Microsoft-Windows-Security-Auditing",
		sigma.FieldEventID: "4688", "NewProcessName": `C:\x\synthetic-tool.exe`, "ParentProcessName": `C:\y\p.exe`}
	if got := matchedPaths(set, security); len(got) != 1 {
		t.Fatalf("security 4688: %v, want the rule", got)
	}
	// Channel の欄を持たないレコードはプロバイダで対応付ける。
	delete(security, sigma.FieldChannel)
	if got := matchedPaths(set, security); len(got) != 1 {
		t.Fatalf("security 4688 without channel: %v, want the rule", got)
	}
	security["ParentProcessName"] = `C:\Trusted\p.exe`
	if got := matchedPaths(set, security); len(got) != 0 {
		t.Fatalf("filtered 4688: %v, want no match", got)
	}
}

func TestMatchValueForms(t *testing.T) {
	rule := func(detection string) string {
		return "title: t\nlogsource:\n  product: windows\n  service: security\ndetection:\n" + detection
	}
	set := loadRules(t, map[string]string{
		"wildcard.yml": rule("  sel:\n    TargetName: 'pre*mid?end'\n  condition: sel\n"),
		"escaped.yml":  rule("  sel:\n    TargetName: 'lit\\*'\n  condition: sel\n"),
		"windash.yml":  rule("  sel:\n    CommandLine|windash|contains: ' -opt'\n  condition: sel\n"),
		"regex.yml":    rule("  sel:\n    CommandLine|re: '^Run[0-9]+$'\n  condition: sel\n"),
		"cidr.yml":     rule("  sel:\n    IpAddress|cidr: '192.0.2.0/24'\n  condition: sel\n"),
		"null.yml":     rule("  sel:\n    EventID: 4625\n    SubStatus: null\n  condition: sel\n"),
		"list.yml":     rule("  sel:\n    - TargetName: one\n    - TargetName: two\n  condition: all of them\n"),
		"exists.yml":   rule("  sel:\n    Marker|exists: true\n  condition: sel\n"),
	})
	if len(set.Unevaluated) != 0 {
		t.Fatalf("Unevaluated = %+v", set.Unevaluated)
	}
	base := func(fields event) event {
		record := event{sigma.FieldChannel: "Security", sigma.FieldEventID: "4625"}
		for name, text := range fields {
			record[name] = text
		}
		return record
	}
	cases := []struct {
		name   string
		record event
		want   []string
	}{
		{"wildcards", base(event{"TargetName": "PRE-x-MIDzEND"}), []string{"null.yml", "wildcard.yml"}},
		{"question needs one char", base(event{"TargetName": "premidend"}), []string{"null.yml"}},
		{"escaped star is literal", base(event{"TargetName": "lit*"}), []string{"escaped.yml", "null.yml"}},
		{"escaped star is not a wildcard", base(event{"TargetName": "litx"}), []string{"null.yml"}},
		{"windash slash", base(event{"CommandLine": "tool /OPT"}), []string{"null.yml", "windash.yml"}},
		{"windash em dash", base(event{"CommandLine": "tool \u2014opt"}), []string{"null.yml", "windash.yml"}},
		{"regex is case sensitive", base(event{"CommandLine": "run12"}), []string{"null.yml"}},
		{"regex", base(event{"CommandLine": "Run12"}), []string{"null.yml", "regex.yml"}},
		{"cidr", base(event{"IpAddress": "192.0.2.9"}), []string{"cidr.yml", "null.yml"}},
		{"cidr outside", base(event{"IpAddress": "198.51.100.9"}), []string{"null.yml"}},
		{"null needs absence", base(event{"SubStatus": "0x0"}), nil},
		{"list alternative", base(event{"TargetName": "two"}), []string{"list.yml", "null.yml"}},
		{"exists", base(event{"Marker": ""}), []string{"exists.yml", "null.yml"}},
	}
	for _, c := range cases {
		got := matchedPaths(set, c.record)
		paths := make([]string, 0, len(got))
		for path := range got {
			paths = append(paths, path)
		}
		slices.Sort(paths)
		if !slices.Equal(paths, c.want) {
			t.Errorf("%s: matched %v, want %v", c.name, paths, c.want)
		}
	}
}

func TestLoadListsRulesItCannotEvaluate(t *testing.T) {
	windows := "logsource:\n  product: windows\n  service: security\n"
	set := loadRules(t, map[string]string{
		"ok.yml":          processRule,
		"broken.yml":      "title: [unclosed\n",
		"keyword.yml":     "title: kw\nid: kw-id\n" + windows + "detection:\n  keywords:\n    - 'text'\n  condition: keywords\n",
		"modifier.yml":    "title: mod\n" + windows + "detection:\n  sel:\n    Data|base64offset|contains: 'x'\n  condition: sel\n",
		"aggregate.yml":   "title: agg\n" + windows + "detection:\n  sel:\n    EventID: 1\n  condition: sel | count() > 5\n",
		"undefined.yml":   "title: undef\n" + windows + "detection:\n  sel:\n    EventID: 1\n  condition: other\n",
		"lookahead.yml":   "title: re\n" + windows + "detection:\n  sel:\n    Data|re: '(?=x)'\n  condition: sel\n",
		"linux.yml":       "title: lx\nlogsource:\n  product: linux\ndetection:\n  sel:\n    a: b\n  condition: sel\n",
		"unmapped.yml":    "title: um\nlogsource:\n  product: windows\n  service: synthetic-service\ndetection:\n  sel:\n    a: b\n  condition: sel\n",
		"collection.yml":  processRule + "---\ntitle: second\n",
		"correlation.yml": "title: corr\ncorrelation:\n  type: event_count\n",
		"notes.txt":       "not a rule",
		".git/HEAD":       "ref: refs/heads/main\n",
	})
	if len(set.Rules) != 1 || set.Rules[0].Path != "ok.yml" {
		t.Fatalf("Rules = %+v, want ok.yml alone", set.Rules)
	}
	want := map[string]string{
		"broken.yml":      sigma.ReasonFileUnreadable,
		"keyword.yml":     sigma.ReasonKeywordSearch,
		"modifier.yml":    sigma.ReasonModifierUnsupported,
		"aggregate.yml":   sigma.ReasonConditionUnsupported,
		"undefined.yml":   sigma.ReasonConditionUnsupported,
		"lookahead.yml":   sigma.ReasonRegexUnsupported,
		"linux.yml":       sigma.ReasonLogsourceUnsupported,
		"unmapped.yml":    sigma.ReasonLogsourceUnsupported,
		"collection.yml":  sigma.ReasonRuleCollection,
		"correlation.yml": sigma.ReasonNotDetectionRule,
	}
	got := map[string]string{}
	for _, skipped := range set.Unevaluated {
		got[skipped.Path] = skipped.Reason
		if skipped.Detail == "" {
			t.Errorf("%s: Detail is empty", skipped.Path)
		}
		if skipped.Path == "keyword.yml" && (skipped.ID != "kw-id" || skipped.Title != "kw") {
			t.Errorf("keyword.yml: identity = %+v", skipped)
		}
	}
	for path, reason := range want {
		if got[path] != reason {
			t.Errorf("%s: reason %q, want %q", path, got[path], reason)
		}
	}
	if len(got) != len(want) {
		t.Errorf("Unevaluated = %v, want %v", got, want)
	}
	if set.FileCount != len(set.Rules)+len(set.Unevaluated) {
		t.Errorf("FileCount = %d, rules %d + unevaluated %d", set.FileCount, len(set.Rules), len(set.Unevaluated))
	}
}

func TestLoadDigestFollowsContent(t *testing.T) {
	first := loadRules(t, map[string]string{"a.yml": processRule})
	same := loadRules(t, map[string]string{"a.yml": processRule})
	changed := loadRules(t, map[string]string{"a.yml": processRule + "\n"})
	if len(first.ContentSha256) != 64 || first.ContentSha256 != same.ContentSha256 ||
		first.ContentSha256 == changed.ContentSha256 {
		t.Fatalf("digests = %s %s %s", first.ContentSha256, same.ContentSha256, changed.ContentSha256)
	}
}

func TestLoadRejectsDirectoryWithoutRules(t *testing.T) {
	if _, err := sigma.Load(fstest.MapFS{"readme.txt": {Data: []byte("x")}}); err == nil {
		t.Fatal("Load succeeded on a directory without rule files")
	}
}

// fs.WalkDir は directory の中身を directory の名前の位置で読むため、`a/b.yml` を `a-c.yml` より
// 先に読む。Rules と Unevaluated は path の文字列の順に並ぶ。
func TestLoadOrdersRulesByPath(t *testing.T) {
	set := loadRules(t, map[string]string{
		"a/b.yml": processRule, "a-c.yml": processRule, "a/x.yml": "title: [", "a-y.yml": "title: [",
	})
	paths := []string{}
	for _, rule := range set.Rules {
		paths = append(paths, rule.Path)
	}
	for _, skipped := range set.Unevaluated {
		paths = append(paths, skipped.Path)
	}
	if !slices.Equal(paths, []string{"a-c.yml", "a/b.yml", "a-y.yml", "a/x.yml"}) {
		t.Fatalf("paths = %v", paths)
	}
}

// 4688 が記録しない項目を参照するルールは 4688 に当てない。logsource が 4688 だけを指すなら、
// 評価しなかったルールに入る。
func TestLoadDropsEventsThatDoNotRecordTheReferencedFields(t *testing.T) {
	sysmonAndSecurity := "title: t\nlogsource:\n  product: windows\n  category: process_creation\ndetection:\n" +
		"  sel:\n    Image|endswith: '\\x.exe'\n  filter:\n    Hashes|contains: 'SYNTHETIC'\n" +
		"  condition: sel and not filter\n"
	securityOnly := strings.Replace(sysmonAndSecurity, "category: process_creation", "category: process_creation\n  service: security", 1)
	set := loadRules(t, map[string]string{"both.yml": sysmonAndSecurity, "security.yml": securityOnly})
	if len(set.Rules) != 1 || set.Rules[0].Path != "both.yml" {
		t.Fatalf("Rules = %+v", set.Rules)
	}
	if len(set.Unevaluated) != 1 || set.Unevaluated[0].Reason != sigma.ReasonFieldUnavailable ||
		!strings.Contains(set.Unevaluated[0].Detail, "Hashes") {
		t.Fatalf("Unevaluated = %+v", set.Unevaluated)
	}
	security := event{sigma.FieldChannel: "Security", sigma.FieldEventID: "4688", "NewProcessName": `C:\a\x.exe`}
	if got := matchedPaths(set, security); len(got) != 0 {
		t.Errorf("4688: matched %v", got)
	}
	if got := matchedPaths(set, sysmonEvent("1", event{"Image": `C:\a\x.exe`, "Hashes": "MD5=0"})); len(got) != 1 {
		t.Errorf("Sysmon 1: matched %v, want both.yml", got)
	}
}

// 名前で持たない項目を参照するルールは当てず、数える。System の項目だけを参照するルールは当てる。
func TestMatchSkipsRulesThatReferenceFieldsTheRecordDoesNotName(t *testing.T) {
	windows := "logsource:\n  product: windows\n  service: security\n"
	set := loadRules(t, map[string]string{
		"data.yml":   "title: d\n" + windows + "detection:\n  sel:\n    EventID: 4624\n  filter:\n    TargetUserName: x\n  condition: sel and not filter\n",
		"system.yml": "title: s\n" + windows + "detection:\n  sel:\n    EventID: 4624\n  condition: sel\n",
	})
	record := event{sigma.FieldProvider: "Microsoft-Windows-Security-Auditing", sigma.FieldEventID: "4624"}
	result := set.Match(sigma.Record{Value: record.value, Named: func(name string) bool {
		return name == sigma.FieldEventID || name == sigma.FieldProvider || name == sigma.FieldChannel
	}})
	if !result.ServiceKnown || result.SkippedRules != 1 || len(result.Matches) != 1 ||
		set.Rules[result.Matches[0].Rule].Path != "system.yml" {
		t.Fatalf("result = %+v", result)
	}
	unmapped := set.Match(sigma.Record{Value: event{sigma.FieldChannel: "Synthetic/Operational"}.value})
	if unmapped.ServiceKnown || len(unmapped.Matches) != 0 {
		t.Fatalf("unmapped channel: %+v", unmapped)
	}
}

// Microsoft-Windows-Eventlog は Security と System の両方に書く。Channel を持たないレコードは、
// Security に書くイベント ID のときだけ security に対応付ける。
func TestMatchMapsTheEventlogProviderToSecurityByEventID(t *testing.T) {
	rule := "title: t\nlogsource:\n  product: windows\n  service: security\ndetection:\n" +
		"  sel:\n    Provider_Name: Microsoft-Windows-Eventlog\n  condition: sel\n"
	set := loadRules(t, map[string]string{"cleared.yml": rule})
	for eventID, want := range map[string]int{"1102": 1, "104": 0} {
		record := event{sigma.FieldProvider: "Microsoft-Windows-Eventlog", sigma.FieldEventID: eventID}
		if got := matchedPaths(set, record); len(got) != want {
			t.Errorf("EventID %s: matched %v, want %d", eventID, got, want)
		}
	}
}
