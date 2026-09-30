// in-package test: 非公開の openStages と sigmaResult が、読み込みの段階の終わりに Sigma のルールを当て、
// 処理を終えた後の API がその結果を返すことを確かめる。
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/api"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// ルール。EVTX の fixture の子のプロセスの path の末尾に一致する。
const childRule = `title: Synthetic child start
logsource:
  product: windows
  category: process_creation
detection:
  selection:
    Image|endswith: '\b-child.exe'
  condition: selection
`

// childRules はルールを 1 件置いた集合を読む。
func childRules(t *testing.T) (*pipeline.SigmaRules, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "child.yml"), []byte(childRule), 0o600); err != nil {
		t.Fatal(err)
	}
	rules, err := pipeline.LoadSigmaRules(pipeline.SigmaRuleSpec{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	return &rules, dir
}

// 起動引数の収集元を同期で読み込むと、その取り込み結果にルールを当てる。
func TestStagedServerServesTheSigmaEvaluation(t *testing.T) {
	rules, dir := childRules(t)
	opts, err := parseArgs([]string{"windows_evtx:../../internal/testdata/winevent/process-creation.evtx"})
	if err != nil {
		t.Fatal(err)
	}
	sigma := &sigmaResult{rules: rules}
	var stderr bytes.Buffer
	stages, _, _, err := openStages(t.Context(), opts, pipeline.DefaultGraphLayers(), sigma, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "1 candidates") {
		t.Errorf("stderr = %q, want the summary with 1 candidate", stderr.String())
	}
	requireSigmaCandidates(t, stages, sigma, dir)
}

// 要求による背景の読み込みでも、読み込んだ取り込み結果にルールを当てる。
func TestRequestedLoadingServesTheSigmaEvaluation(t *testing.T) {
	rules, dir := childRules(t)
	opts, err := parseArgs([]string{sourceRootFlag, "../../internal/testdata/winevent"})
	if err != nil {
		t.Fatal(err)
	}
	sigma := &sigmaResult{rules: rules}
	stages, _, root, err := openStages(t.Context(), opts, pipeline.DefaultGraphLayers(), sigma, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stages.Wait()
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := stages.StartRequestedLoading([]pipeline.SourcePlan{
		{OriginPath: "process-creation.evtx", FormatKey: "windows_evtx"},
	}); err != nil {
		t.Fatal(err)
	}
	stages.Wait()
	if _, loaded := stages.Loaded(); !loaded {
		t.Fatalf("the requested loading did not complete: %+v", stages.Snapshot().Loading)
	}
	requireSigmaCandidates(t, stages, sigma, dir)
}

// requireSigmaCandidates は、処理の前はルールを当てた結果も段階を待ち、処理の後はルールの
// 一致 1 件を返すことを確かめる。
func requireSigmaCandidates(t *testing.T, stages *pipeline.Stages, sigma *sigmaResult, dir string) {
	t.Helper()
	handler, err := api.NewStagedHandler(stages, sigma.current, pipeline.AttackRuleSet{})
	if err != nil {
		t.Fatal(err)
	}
	request := func() *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v0/sigma-rule-candidates?depth=1&matchCondition=destination_port", nil))
		return response
	}
	// 処理を終えるまでは、ルールを当てた結果も段階を待つ。
	if response := request(); response.Code != http.StatusConflict {
		t.Fatalf("before the processing: status %d body %s, want 409", response.Code, response.Body.String())
	}
	if err := stages.Process(); err != nil {
		t.Fatal(err)
	}
	response := request()
	var body struct {
		RuleSet struct {
			Directory string `json:"directory"`
		} `json:"ruleSet"`
		Matches []struct {
			RulePath string `json:"rulePath"`
		} `json:"matches"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("status %d body %s: %v", response.Code, response.Body.String(), err)
	}
	if body.RuleSet.Directory != dir || len(body.Matches) != 1 || body.Matches[0].RulePath != "child.yml" {
		t.Fatalf("body = %+v", body)
	}
}
