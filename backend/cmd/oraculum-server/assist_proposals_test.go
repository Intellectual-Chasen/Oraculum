// in-package test: 非公開の openStages で調査を作り、AI 提案の採否を開き直した後の応答と比べる。
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// proposeOn は開いた調査の保存先へ、ノード nodeId を対象にした提案を 1 件記録する。LLM の tool の
// 作成を経由しない。
func (o openedInvestigation) proposeOn(t *testing.T, nodeId, turnId string) core.AssistProposal {
	t.Helper()
	store, loaded := o.stages.Loaded()
	if !loaded {
		t.Fatal("the investigation is not loaded")
	}
	line := int64(1)
	proposal, err := store.AssistProposals().Create(pipeline.AssistProposalDraft{
		Target: core.AssertionTarget{Kind: core.AssertionTargetKindNode, NodeId: nodeId},
		Note:   "LLM が挙げた候補",
		RecordRefs: []core.AssertionRecordRef{{
			SourceContentSha256: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
			PositionKind:        core.PositionKindLineNumber, LineNumber: &line,
		}},
		ConversationId: "0123456789abcdef0123456789abcdef", TurnId: turnId, Provider: core.AssistProviderClaude,
		MatchConditions: []core.AssistMatchCondition{{ConditionKey: core.ConditionKeyDestinationIp}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return proposal
}

// 新しい表を持たない調査を開き直せ、AI 提案と採否の記録と採用で作った所見が開き直した後も残る。
func TestReopenedInvestigationKeepsTheProposalsAndTheirDecisions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "case")
	created := openInvestigation(t, append([]string{investigationFlag, dir}, investigationSources()...)...)
	created.close(t)

	opened := openInvestigation(t, investigationFlag, dir)
	proposalsPath := "/api/v0/assist-proposals?" + allConditionsQuery()
	var empty struct {
		ProposalCount int64 `json:"proposalCount"`
	}
	if err := json.Unmarshal(opened.request(t, http.MethodGet, proposalsPath, nil, http.StatusOK), &empty); err != nil {
		t.Fatal(err)
	}
	if empty.ProposalCount != 0 {
		t.Fatalf("an investigation without the proposal tables lists %d proposals, want none", empty.ProposalCount)
	}
	adopted := opened.proposeOn(t, "n:process:proposed", "turn-1")
	rejected := opened.proposeOn(t, "n:process:proposed", "turn-2")
	opened.request(t, http.MethodPost, "/api/v0/assist-proposals/"+adopted.Id+"/adoption?"+allConditionsQuery(),
		map[string]any{"analyst": "analyst-a", "note": "分析者が直した記述"}, http.StatusCreated)
	opened.request(t, http.MethodPost, "/api/v0/assist-proposals/"+rejected.Id+"/rejection?"+allConditionsQuery(),
		map[string]any{"analyst": "analyst-b", "reason": "別の端末"}, http.StatusOK)
	assertionsPath := "/api/v0/assertions?" + allConditionsQuery()
	beforeProposals := opened.request(t, http.MethodGet, proposalsPath, nil, http.StatusOK)
	beforeAssertions := opened.request(t, http.MethodGet, assertionsPath, nil, http.StatusOK)
	opened.close(t)

	reopened := openInvestigation(t, investigationFlag, dir)
	defer reopened.close(t)
	afterProposals := reopened.request(t, http.MethodGet, proposalsPath, nil, http.StatusOK)
	afterAssertions := reopened.request(t, http.MethodGet, assertionsPath, nil, http.StatusOK)
	if !bytes.Equal(beforeProposals, afterProposals) {
		t.Errorf("the proposals differ after reopening:\nbefore %s\nafter  %s", beforeProposals, afterProposals)
	}
	if !bytes.Equal(beforeAssertions, afterAssertions) {
		t.Errorf("the assertions differ after reopening:\nbefore %s\nafter  %s", beforeAssertions, afterAssertions)
	}
	var listed struct {
		Proposals []struct {
			Proposal core.AssistProposal `json:"proposal"`
		} `json:"proposals"`
	}
	if err := json.Unmarshal(afterProposals, &listed); err != nil {
		t.Fatal(err)
	}
	states := map[string]core.AssistProposalState{}
	for _, item := range listed.Proposals {
		states[item.Proposal.Id] = item.Proposal.State
	}
	want := map[string]core.AssistProposalState{
		adopted.Id: core.AssistProposalStateAdopted, rejected.Id: core.AssistProposalStateRejected,
	}
	for id, state := range want {
		if states[id] != state {
			t.Errorf("the proposal %q is %q after reopening, want %q", id, states[id], state)
		}
	}
	var assertions struct {
		Assertions []struct {
			Assertion core.Assertion `json:"assertion"`
		} `json:"assertions"`
	}
	if err := json.Unmarshal(afterAssertions, &assertions); err != nil {
		t.Fatal(err)
	}
	if len(assertions.Assertions) != 1 || assertions.Assertions[0].Assertion.ProposalId != adopted.Id {
		t.Errorf("assertions = %s, want the adopted assertion naming its proposal", afterAssertions)
	}
}
