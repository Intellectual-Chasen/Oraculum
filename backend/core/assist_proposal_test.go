package core_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// proposalConversationId は会話の識別子である。
const proposalConversationId = "0123456789abcdef0123456789abcdef"

func validProposal() core.AssistProposal {
	return core.AssistProposal{
		Id:              "ap:0123",
		Target:          assertionNodeTarget(),
		Note:            "起動の直後に外部へ接続している",
		RecordRefs:      []core.AssertionRecordRef{assertionLineRef(7)},
		ConversationId:  proposalConversationId,
		TurnId:          "turn-1",
		Provider:        core.AssistProviderClaude,
		Model:           "model-a",
		MatchConditions: []core.AssistMatchCondition{{ConditionKey: core.ConditionKeyDestinationIp}},
		State:           core.AssistProposalStateProposed,
		CreatedAt:       assertionRecordedAt,
	}
}

// 提案中の提案は採否の記録を持たずに通り、根拠と関連付けの条件を集合として出す。
func TestAssistProposalPassesWithoutADecisionWhileProposed(t *testing.T) {
	proposal := validProposal()
	proposal.MatchConditions = nil
	if err := proposal.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want the proposed proposal to pass", err)
	}
	encoded, err := json.Marshal(proposal)
	if err != nil {
		t.Fatalf("Marshal() = %v", err)
	}
	text := string(encoded)
	for _, want := range []string{`"matchConditions":[]`, `"addsRelation":false`, `"state":"proposed"`} {
		if !strings.Contains(text, want) {
			t.Errorf("json = %s, want it to carry %s", text, want)
		}
	}
	if strings.Contains(text, `"decision"`) {
		t.Errorf("json = %s, want no decision on a proposed proposal", text)
	}
}

// 対象はノード・関係・レコードに限り、収集元を退ける。
func TestAssistProposalAcceptsNodeEdgeAndRecordTargetsAlone(t *testing.T) {
	record := assertionLineRef(9)
	targets := map[string]core.AssertionTarget{
		"node":   assertionNodeTarget(),
		"edge":   assertionEdgeTarget(),
		"record": {Kind: core.AssertionTargetKindRecord, Record: &record},
	}
	for name, target := range targets {
		proposal := validProposal()
		proposal.Target = target
		if err := proposal.Validate(); err != nil {
			t.Errorf("Validate() with the %s target = %v, want nil", name, err)
		}
	}
	proposal := validProposal()
	proposal.Target = core.AssertionTarget{
		Kind: core.AssertionTargetKindSource, SourceContentSha256: assertionContentSha256,
	}
	if err := proposal.Validate(); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("Validate() with a source target = %v, want ErrInvalid", err)
	}
}

// 関係を足す提案は関係を指す。
func TestAssistProposalAddsARelationOnAnEdgeAlone(t *testing.T) {
	proposal := validProposal()
	proposal.Target, proposal.AddsRelation = assertionEdgeTarget(), true
	if err := proposal.Validate(); err != nil {
		t.Errorf("Validate() of a relation to add = %v, want nil", err)
	}
	proposal.Target = assertionNodeTarget()
	if err := proposal.Validate(); !errors.Is(err, core.ErrUnexpectedItem) {
		t.Errorf("Validate() of a node proposal adding a relation = %v, want ErrUnexpectedItem", err)
	}
}

// 記述と 1 件以上の根拠と、会話・発言・提供者の識別を必須とする。
func TestAssistProposalRequiresTheNoteTheBasisAndTheConversation(t *testing.T) {
	cases := map[string]func(*core.AssistProposal){
		"blank note":         func(p *core.AssistProposal) { p.Note = " \n" },
		"no basis":           func(p *core.AssistProposal) { p.RecordRefs = nil },
		"invalid basis":      func(p *core.AssistProposal) { p.RecordRefs[0].LineNumber = nil },
		"invalid convo":      func(p *core.AssistProposal) { p.ConversationId = "conversation" },
		"no turn":            func(p *core.AssistProposal) { p.TurnId = "" },
		"unknown provider":   func(p *core.AssistProposal) { p.Provider = "other" },
		"control in model":   func(p *core.AssistProposal) { p.Model = "model\n" },
		"unknown condition":  func(p *core.AssistProposal) { p.MatchConditions[0].ConditionKey = "other" },
		"negative tolerance": func(p *core.AssistProposal) { p.MatchConditions[0].Tolerance = -1 },
		"no id":              func(p *core.AssistProposal) { p.Id = "" },
		"invalid time":       func(p *core.AssistProposal) { p.CreatedAt = "yesterday" },
	}
	for name, mutate := range cases {
		proposal := validProposal()
		mutate(&proposal)
		if err := proposal.Validate(); !errors.Is(err, core.ErrInvalid) {
			t.Errorf("%s: Validate() = %v, want ErrInvalid", name, err)
		}
	}
}

// 採用は所見の識別子を持ち、却下は理由を任意で持つ。提案中の提案は採否の記録を持たない。
func TestAssistProposalDecisionFollowsTheState(t *testing.T) {
	decided := func(state core.AssistProposalState, decision *core.AssistProposalDecision) error {
		proposal := validProposal()
		proposal.State, proposal.Decision = state, decision
		return proposal.Validate()
	}
	adoption := core.AssistProposalDecision{Analyst: "analyst-a", DecidedAt: assertionRecordedAt, AssertionId: "as:1"}
	rejection := core.AssistProposalDecision{Analyst: "analyst-a", DecidedAt: assertionRecordedAt}
	withReason := rejection
	withReason.Reason = "根拠のレコードは別の端末のもの"
	passing := map[string]error{
		"adopted":             decided(core.AssistProposalStateAdopted, &adoption),
		"rejected":            decided(core.AssistProposalStateRejected, &rejection),
		"rejected, reasoned":  decided(core.AssistProposalStateRejected, &withReason),
		"proposed, undecided": decided(core.AssistProposalStateProposed, nil),
	}
	for name, err := range passing {
		if err != nil {
			t.Errorf("%s: Validate() = %v, want nil", name, err)
		}
	}
	adoptedWithReason := adoption
	adoptedWithReason.Reason = "理由"
	rejectedWithAssertion := rejection
	rejectedWithAssertion.AssertionId = "as:1"
	adoptedWithoutAssertion := adoption
	adoptedWithoutAssertion.AssertionId = ""
	anonymous := adoption
	anonymous.Analyst = ""
	failing := map[string]error{
		"proposed, decided":          decided(core.AssistProposalStateProposed, &rejection),
		"adopted, undecided":         decided(core.AssistProposalStateAdopted, nil),
		"rejected, undecided":        decided(core.AssistProposalStateRejected, nil),
		"adopted, reasoned":          decided(core.AssistProposalStateAdopted, &adoptedWithReason),
		"adopted, without assertion": decided(core.AssistProposalStateAdopted, &adoptedWithoutAssertion),
		"rejected, with assertion":   decided(core.AssistProposalStateRejected, &rejectedWithAssertion),
		"adopted, no analyst":        decided(core.AssistProposalStateAdopted, &anonymous),
		"unknown state":              decided("pending", nil),
	}
	for name, err := range failing {
		if !errors.Is(err, core.ErrInvalid) {
			t.Errorf("%s: Validate() = %v, want ErrInvalid", name, err)
		}
	}
}

// 採用で作った所見は元の提案の識別子を持ち、直接記録した所見の JSON はその項目を出さない。
func TestAssertionCarriesTheProposalItWasAdoptedFrom(t *testing.T) {
	assertion := validAssertion()
	direct, err := json.Marshal(assertion)
	if err != nil {
		t.Fatalf("Marshal() = %v", err)
	}
	if strings.Contains(string(direct), "proposalId") {
		t.Errorf("json = %s, want no proposalId on an assertion recorded directly", direct)
	}
	assertion.ProposalId = "ap:0123"
	if err := assertion.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want an adopted assertion to pass", err)
	}
	adopted, err := json.Marshal(assertion)
	if err != nil {
		t.Fatalf("Marshal() = %v", err)
	}
	if !strings.Contains(string(adopted), `"proposalId":"ap:0123"`) {
		t.Errorf("json = %s, want the proposal identifier", adopted)
	}
	assertion.ProposalId = "ap:\n"
	if err := assertion.Validate(); !errors.Is(err, core.ErrControlCharacter) {
		t.Errorf("Validate() with a control character in proposalId = %v, want ErrControlCharacter", err)
	}
}
