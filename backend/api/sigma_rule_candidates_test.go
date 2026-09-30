package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/api"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// sigmaCandidatesPath は、条件を与えない Sigma のルールの候補の要求である。
const sigmaCandidatesPath = "/api/v0/sigma-rule-candidates?depth=1&matchCondition=destination_port"

func sigmaCandidatesHandler(t *testing.T, evaluation pipeline.SigmaEvaluation) http.Handler {
	t.Helper()
	store := pipeline.NewMemoryStore(pipeline.ImportResult{}, &testAssertionClock{})
	handler, err := api.NewHandler(store, pipeline.NewGraphCatalog(store, pipeline.DefaultGraphLayers()), evaluation,
		pipeline.AttackRuleSet{})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func getSigmaCandidates(t *testing.T, handler http.Handler, target string) (int, map[string]json.RawMessage) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
	var body map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding %s: %v", response.Body.String(), err)
	}
	return response.Code, body
}

func TestSigmaRuleCandidatesReturnsTheEvaluation(t *testing.T) {
	commit := strings.Repeat("ab12", 10)
	channel, empty := "Example/Operational", ""
	line := int64(7)
	record := core.RecordLocator{
		SourceId: "src-synthetic", SourceContentSha256: strings.Repeat("c", 64), SourceFileName: "events.xml",
		PositionKind: core.PositionKindLineNumber, LineNumber: &line, RecordRawTextRef: "raw:synthetic",
	}
	timeText := "2031-04-05T06:07:08Z"
	eventTime, err := core.NewTimestamp(core.Timestamp{
		RawText: &timeText, Normalized: &timeText, NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Precision: core.PrecisionSecond, OffsetState: core.OffsetStateInValue, OffsetText: new("Z"),
		Clock: core.ClockTerminalLocal, Meaning: core.MeaningEvent, ValueState: core.ValueStatePresent,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := sigmaCandidatesHandler(t, pipeline.SigmaEvaluation{
		RuleSet: &pipeline.SigmaRuleSetInfo{
			Directory: "rules", Revision: &commit, RevisionSource: "git_head", GitWorkTree: "/synthetic",
			ContentSha256: strings.Repeat("d", 64), RuleFileCount: 2,
		},
		EvaluatedRuleCount: 1, EvaluatedRecordCount: 3, SkippedPairCount: 4, SkippedPairRecordCount: 2,
		RecordsWithoutSemantics: 7,
		UnevaluatedRecordGroups: []pipeline.SigmaUnevaluatedRecordGroup{
			{Channel: &channel, RecordCount: 5}, {Provider: &empty, RecordCount: 1},
			{ChannelAndProviderAbsent: true, RecordCount: 1},
		},
		Rules: []pipeline.SigmaMatchedRule{{
			Path: "a.yml", ID: "synthetic-id", Title: "Synthetic", Author: "Synthetic Author", Condition: "sel",
			Selections: []pipeline.SigmaSelection{{Name: "sel", Definition: "Image: x\n"}}, MatchCount: 1,
		}},
		Matches: []pipeline.SigmaRuleMatch{{
			RulePath: "a.yml", Record: record, Terminal: "ws-9", TerminalAssigned: true, EventTime: &eventTime,
		}},
		UnevaluatedRules: []pipeline.SigmaUnevaluatedRule{{Path: "b.yml", Reason: "keyword_search", Detail: "synthetic"}},
	})
	status, body := getSigmaCandidates(t, handler, sigmaCandidatesPath)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	var ruleSet struct {
		Revision       string `json:"revision"`
		RevisionSource string `json:"revisionSource"`
		GitWorkTree    string `json:"gitWorkTree"`
	}
	for key, want := range map[string]string{
		"skippedPairCount": "4", "skippedPairRecordCount": "2", "recordsWithoutSemantics": "7",
		"unevaluatedRecordGroups": `[{"channel":"Example/Operational","recordCount":5},` +
			`{"provider":"","recordCount":1},{"channelAndProviderAbsent":true,"recordCount":1}]`,
	} {
		if got := string(body[key]); got != want {
			t.Errorf("%s = %s, want %s", key, got, want)
		}
	}
	var rules []struct {
		Path       string `json:"path"`
		Author     string `json:"author"`
		MatchCount int64  `json:"matchCount"`
		Selections []struct {
			Name string `json:"name"`
		} `json:"selections"`
	}
	var matches []struct {
		RulePath          string             `json:"rulePath"`
		Record            core.RecordLocator `json:"record"`
		MatchedSelections []string           `json:"matchedSelections"`
		Terminal          string             `json:"terminal"`
		TerminalAssigned  bool               `json:"terminalAssigned"`
		EventTime         *core.Timestamp    `json:"eventTime"`
	}
	var unevaluated []struct {
		Path   string `json:"path"`
		Reason string `json:"reason"`
	}
	for key, target := range map[string]any{"ruleSet": &ruleSet, "rules": &rules, "matches": &matches, "unevaluatedRules": &unevaluated} {
		if err := json.Unmarshal(body[key], target); err != nil {
			t.Fatalf("%s: %v", key, err)
		}
	}
	if ruleSet.Revision != commit || ruleSet.RevisionSource != "git_head" || ruleSet.GitWorkTree != "/synthetic" {
		t.Errorf("ruleSet = %+v", ruleSet)
	}
	if len(rules) != 1 || rules[0].Path != "a.yml" || rules[0].Author != "Synthetic Author" ||
		rules[0].MatchCount != int64(len(matches)) || len(rules[0].Selections) != 1 {
		t.Errorf("rules = %+v", rules)
	}
	if len(matches) != 1 || matches[0].RulePath != "a.yml" || matches[0].Record.RecordRawTextRef != record.RecordRawTextRef ||
		matches[0].MatchedSelections == nil || matches[0].Terminal != "ws-9" || !matches[0].TerminalAssigned ||
		matches[0].EventTime == nil || *matches[0].EventTime.RawText != *eventTime.RawText {
		t.Errorf("matches = %+v", matches)
	}
	if len(unevaluated) != 1 || unevaluated[0].Reason != "keyword_search" {
		t.Errorf("unevaluatedRules = %+v", unevaluated)
	}
}

func TestSigmaRuleCandidatesWithoutRuleSet(t *testing.T) {
	status, body := getSigmaCandidates(t, sigmaCandidatesHandler(t, pipeline.SigmaEvaluation{}), sigmaCandidatesPath)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if _, present := body["ruleSet"]; present {
		t.Errorf("ruleSet is present: %s", body["ruleSet"])
	}
	for _, key := range []string{"rules", "matches", "unevaluatedRules", "unevaluatedRecordGroups"} {
		if string(body[key]) != "[]" {
			t.Errorf("%s = %s, want []", key, body[key])
		}
	}
}

func TestSigmaRuleCandidatesRejectsQueryParameters(t *testing.T) {
	status, body := getSigmaCandidates(t, sigmaCandidatesHandler(t, pipeline.SigmaEvaluation{}),
		sigmaCandidatesPath+"&unknown=x")
	if status != http.StatusBadRequest || !strings.Contains(string(body["code"]), "invalid_request") {
		t.Fatalf("status = %d body = %v", status, body)
	}
}
