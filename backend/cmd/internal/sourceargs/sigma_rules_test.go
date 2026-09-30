package sourceargs_test

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/cmd/internal/sourceargs"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

func TestParseSigmaRulesTakesTheFlagsOutOfTheArguments(t *testing.T) {
	spec, rest, err := sourceargs.ParseSigmaRules([]string{
		"--addr", "127.0.0.1:0", sourceargs.SigmaRulesFlag, "rules", sourceargs.SigmaRulesRevisionFlag, "abc1234",
		"windows_event_xml:logs/a.xml",
	}, failingRead)
	if err != nil {
		t.Fatal(err)
	}
	if spec == nil || spec.Directory != "rules" || spec.Revision != "abc1234" || spec.ExpectedContentSha256 != nil {
		t.Fatalf("spec = %+v", spec)
	}
	if !slices.Equal(rest, []string{"--addr", "127.0.0.1:0", "windows_event_xml:logs/a.xml"}) {
		t.Fatalf("rest = %v", rest)
	}
}

func TestParseSigmaRulesWithoutTheFlags(t *testing.T) {
	args := []string{"windows_event_xml:logs/a.xml"}
	spec, rest, err := sourceargs.ParseSigmaRules(args, failingRead)
	if err != nil || spec != nil || !slices.Equal(rest, args) {
		t.Fatalf("spec = %+v, rest = %v, err = %v", spec, rest, err)
	}
}

func TestParseSigmaRulesRejectsMalformedFlags(t *testing.T) {
	for name, args := range map[string][]string{
		"revision without directory": {sourceargs.SigmaRulesRevisionFlag, "abc1234"},
		"directory twice":            {sourceargs.SigmaRulesFlag, "a", sourceargs.SigmaRulesFlag, "b"},
		"missing value":              {sourceargs.SigmaRulesFlag},
		"empty value":                {sourceargs.SigmaRulesFlag, ""},
	} {
		if _, _, err := sourceargs.ParseSigmaRules(args, failingRead); err == nil {
			t.Errorf("%s: accepted %v", name, args)
		}
	}
}

// importSpecWithRules は Sigma のルールの集合を記録した取り込みの指定の記録を返す。
func importSpecWithRules(t *testing.T, rules *sourceargs.ImportSpecSigmaRules) []byte {
	t.Helper()
	content, err := json.Marshal(sourceargs.ImportSpec{
		Sources: []sourceargs.ImportSpecSource{{
			FormatKey: "windows_event_xml", OriginPath: "logs/a.xml", ContentSha256: exampleSha256,
		}},
		SigmaRules: rules,
	})
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func TestParseSigmaRulesReadsTheRecordedRuleSet(t *testing.T) {
	commit := "abc1234"
	recorded := sourceargs.ImportSpecSigmaRulesOf(pipeline.SigmaRuleSetInfo{
		Directory: "rules", Revision: &commit, RevisionSource: "git_head", ContentSha256: exampleSha256,
	})
	content := importSpecWithRules(t, recorded)
	read := func(string) ([]byte, error) { return content, nil }
	spec, rest, err := sourceargs.ParseSigmaRules([]string{"--import-spec", "spec.json"}, read)
	if err != nil {
		t.Fatal(err)
	}
	if spec == nil || spec.Directory != "rules" || spec.Revision != commit ||
		spec.ExpectedContentSha256 == nil || *spec.ExpectedContentSha256 != exampleSha256 {
		t.Fatalf("spec = %+v", spec)
	}
	if !slices.Equal(rest, []string{"--import-spec", "spec.json"}) {
		t.Fatalf("rest = %v", rest)
	}
	// 記録を持つ指定と一緒に別の集合を渡す起動を退ける。
	if _, _, err := sourceargs.ParseSigmaRules(
		[]string{"--import-spec", "spec.json", sourceargs.SigmaRulesFlag, "other"}, read); err == nil {
		t.Fatal("accepted another rule set with a recorded one")
	}
	// Parse も同じ記録を読める。
	if plans, _, err := sourceargs.Parse(rest, read); err != nil || len(plans) != 1 {
		t.Fatalf("Parse: plans = %v, err = %v", plans, err)
	}
}

func TestParseSigmaRulesRejectsAMalformedRecord(t *testing.T) {
	content := importSpecWithRules(t, &sourceargs.ImportSpecSigmaRules{Directory: "rules", RevisionSource: "git_head"})
	read := func(string) ([]byte, error) { return content, nil }
	if _, _, err := sourceargs.ParseSigmaRules([]string{"--import-spec", "spec.json"}, read); err == nil {
		t.Fatal("accepted a record without the content digest")
	}
	unreadable := func(string) ([]byte, error) { return nil, errors.New("synthetic read failure") }
	if _, _, err := sourceargs.ParseSigmaRules([]string{"--import-spec", "spec.json"}, unreadable); err == nil {
		t.Fatal("accepted an unreadable import specification")
	}
}
