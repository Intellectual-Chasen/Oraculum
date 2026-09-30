// in-package test: 取り込み結果の Windows イベントログのレコードへルールを当て、候補の位置を確かめる。
package pipeline

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// ルール。matchingRule は子のプロセスの path の末尾、unmatchedRule は現れない
// コマンド行に一致する。brokenRule は評価できない修飾子を持つ。
const (
	matchingRule = `title: Synthetic child start
id: 00000000-0000-4000-8000-00000000000a
author: Synthetic Author
level: medium
logsource:
  product: windows
  category: process_creation
detection:
  selection:
    Image|endswith: '\b-child.exe'
  condition: selection
`
	unmatchedRule = `title: Synthetic absent command
logsource:
  product: windows
  service: security
detection:
  selection:
    CommandLine|contains: 'absent-token'
  condition: selection
`
	brokenRule = `title: Synthetic unsupported
id: 00000000-0000-4000-8000-00000000000b
logsource:
  product: windows
  service: security
detection:
  selection:
    CommandLine|base64: 'x'
  condition: selection
`
)

func writeSigmaRules(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestEvaluateSigmaRulesPointsCandidatesAtMatchedRecords(t *testing.T) {
	result := windowsEventImportResult(t, twoComputersDocument)
	dir := writeSigmaRules(t, map[string]string{
		"a_match.yml": matchingRule, "b_unmatched.yml": unmatchedRule, "c_broken.yml": brokenRule,
	})
	rules, err := LoadSigmaRules(SigmaRuleSpec{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	evaluation := EvaluateSigmaRules(result, &rules)

	info := evaluation.RuleSet
	if info == nil || info.Directory != dir || info.Revision != nil || info.RevisionSource != "unverified" ||
		info.RevisionDetail == "" || info.RuleFileCount != 3 || len(info.ContentSha256) != 64 {
		t.Fatalf("RuleSet = %+v", info)
	}
	records := result.publications[0].records
	if evaluation.EvaluatedRecordCount != int64(len(records)) || evaluation.EvaluatedRuleCount != 2 {
		t.Fatalf("evaluated %d records and %d rules, want %d records and 2 rules",
			evaluation.EvaluatedRecordCount, evaluation.EvaluatedRuleCount, len(records))
	}
	if len(evaluation.Rules) != 1 || evaluation.Rules[0].Path != "a_match.yml" ||
		evaluation.Rules[0].Author != "Synthetic Author" || evaluation.Rules[0].Condition != "selection" ||
		evaluation.Rules[0].MatchCount != int64(len(evaluation.Matches)) {
		t.Fatalf("Rules = %+v", evaluation.Rules)
	}
	if len(evaluation.Matches) != 1 {
		t.Fatalf("Matches = %+v, want the child record alone", evaluation.Matches)
	}
	match := evaluation.Matches[0]
	var matched *RecordEntry
	for at := range records {
		if records[at].Locator == match.Record {
			matched = &records[at]
		}
	}
	if matched == nil || !strings.Contains(matched.RawText, `b-child.exe`) ||
		match.RulePath != "a_match.yml" || !slices.Equal(match.MatchedSelections, []string{"selection"}) {
		t.Fatalf("match = %+v, record = %+v", match, matched)
	}
	// 端末と時刻を一致したレコードから載せ、一致した候補を一覧で比べられるようにする。
	if match.Terminal != *matched.Terminal[0].Text.RawText || match.TerminalAssigned ||
		match.EventTime == nil || *match.EventTime.RawText != *matched.ObservedAt.RawText {
		t.Fatalf("the match carries the terminal %q and the time %+v, want the record's", match.Terminal, match.EventTime)
	}
	if len(evaluation.UnevaluatedRules) != 1 || evaluation.UnevaluatedRules[0].Path != "c_broken.yml" ||
		evaluation.UnevaluatedRules[0].Reason != "modifier_unsupported" {
		t.Fatalf("UnevaluatedRules = %+v", evaluation.UnevaluatedRules)
	}
}

// 端末を記録しないレコードは、収集元に指定した端末の名前を、指定から来たことと一緒に返す。
func TestSigmaMatchTerminalFallsBackToTheSpecifiedTerminal(t *testing.T) {
	source := sourceTerminal{assignments: []core.TerminalAssignment{{TerminalId: "ws-9"}}}
	if name, assigned := sigmaMatchTerminalOf(RecordEntry{}, source); name != "ws-9" || !assigned {
		t.Errorf("the terminal is %q (assigned %t), want the specified ws-9", name, assigned)
	}
	if name, assigned := sigmaMatchTerminalOf(RecordEntry{}, sourceTerminal{}); name != "" || assigned {
		t.Errorf("a source without a terminal gave %q (assigned %t)", name, assigned)
	}
}

func TestEvaluateSigmaRulesWithoutRuleSet(t *testing.T) {
	evaluation := EvaluateSigmaRules(windowsEventImportResult(t, twoComputersDocument), nil)
	if evaluation.RuleSet != nil || evaluation.Rules == nil || evaluation.Matches == nil ||
		evaluation.UnevaluatedRules == nil || evaluation.EvaluatedRecordCount != 0 {
		t.Fatalf("evaluation = %+v", evaluation)
	}
}

func TestLoadSigmaRulesRejectsContentThatDiffersFromTheRecord(t *testing.T) {
	dir := writeSigmaRules(t, map[string]string{"a.yml": matchingRule})
	loaded, err := LoadSigmaRules(SigmaRuleSpec{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	recorded := loaded.Info().ContentSha256
	if _, err := LoadSigmaRules(SigmaRuleSpec{Directory: dir, ExpectedContentSha256: &recorded}); err != nil {
		t.Fatalf("the same content was rejected: %v", err)
	}
	other := strings.Repeat("0", 64)
	if _, err := LoadSigmaRules(SigmaRuleSpec{Directory: dir, ExpectedContentSha256: &other}); err == nil {
		t.Fatal("content that differs from the record was accepted")
	}
}
