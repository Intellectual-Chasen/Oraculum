package core

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AssistProposalState は AI 提案の採否の状態である。
type AssistProposalState string

// AssistProposalState の値。
const (
	// AssistProposalStateProposed は、分析者がまだ採否を決めていない提案である。
	AssistProposalStateProposed AssistProposalState = "proposed"
	// AssistProposalStateAdopted は、分析者が採用し、分析者を著者とする所見を作った提案である。
	AssistProposalStateAdopted AssistProposalState = "adopted"
	// AssistProposalStateRejected は、分析者が却下した提案である。
	AssistProposalStateRejected AssistProposalState = "rejected"
)

// IsKnown は値が定義の中にあることを返す。
func (s AssistProposalState) IsKnown() bool {
	switch s {
	case AssistProposalStateProposed, AssistProposalStateAdopted, AssistProposalStateRejected:
		return true
	default:
		return false
	}
}

// AssistProposalDecision は、分析者が AI 提案の採否を決めた記録である。
type AssistProposalDecision struct {
	// Analyst は採否を決めた分析者である。
	Analyst string `json:"analyst"`
	// DecidedAt は採否を決めた時刻である。
	DecidedAt AssertionTime `json:"decidedAt"`
	// Reason は却下の理由である。分析者が書いた字句を入力された言語のまま保つ。却下で任意、
	// 採用では出ない。
	Reason string `json:"reason,omitempty"`
	// AssertionId は採用で作った所見の識別子である。採用で必須、却下では出ない。
	AssertionId string `json:"assertionId,omitempty"`
}

// validate は state の決定として項目の整合を確かめる。
func (d AssistProposalDecision) validate(state AssistProposalState) error {
	problem := firstProblem(
		requirePresent("AssistProposalDecision.analyst", d.Analyst),
		requireSanitized("AssistProposalDecision.analyst", d.Analyst),
	)
	if problem != nil {
		return problem
	}
	if err := d.DecidedAt.Validate(); err != nil {
		return itemError("AssistProposalDecision.decidedAt", err)
	}
	if state == AssistProposalStateAdopted {
		return firstProblem(
			requirePresent("AssistProposalDecision.assertionId", d.AssertionId),
			requireAbsent("AssistProposalDecision.reason", d.Reason),
		)
	}
	return requireAbsent("AssistProposalDecision.assertionId", d.AssertionId)
}

// AssistProposal は、LLM が会話の中で挙げた所見の候補 1 件である。
//
// **分析者の判断 (Assertion) と別の型である。** 所見の一覧、所見の検索、所見の出力は本型を
// 持たない。分析者が採用した時点で、分析者を著者とする所見が新しく作られ、その所見が本型の
// 識別子を持つ。
//
// 対象と根拠は、会話の中で server が受け渡した記録・ノード・関係を、取り込みをやり直しても
// 同じものを指す参照へ引き当てた値で持つ。
type AssistProposal struct {
	// Id は提案を指す不透明な識別子である。
	Id string `json:"id"`
	// Target は提案が指す対象である。ノード、関係、レコードのいずれかである。
	Target AssertionTarget `json:"target"`
	// Note は LLM が書いた記述である。
	Note string `json:"note"`
	// RecordRefs は根拠に挙げたレコードである。1 件以上を持つ。
	RecordRefs []AssertionRecordRef `json:"recordRefs"`
	// ConversationId は提案を作った会話の識別子である。受け渡しの監査記録を引く材料になる。
	ConversationId string `json:"conversationId"`
	// TurnId は提案を作った発言の識別子である。
	TurnId string `json:"turnId"`
	// Provider は会話の提供者である。
	Provider AssistProvider `json:"provider"`
	// Model は会話の提供者の model の名前である。中継が申告しなかった会話では出ない。
	Model string `json:"model,omitempty"`
	// AddsRelation は、提案がグラフに無い関係を足す提案であることである。Target の Kind が edge の
	// 提案だけが真を取る。採用するまでグラフに関係を足さない。
	AddsRelation bool `json:"addsRelation"`
	// MatchConditions は、提案を作った発言の関連付けの条件の選択である。要素数 0 の場合も集合である。
	MatchConditions []AssistMatchCondition `json:"matchConditions"`
	// State は採否の状態である。
	State AssistProposalState `json:"state"`
	// CreatedAt は server が提案を記録した時刻である。
	CreatedAt AssertionTime `json:"createdAt"`
	// Decision は採否の記録である。State が proposed の提案では出ず、それ以外で必須である。
	Decision *AssistProposalDecision `json:"decision,omitempty"`
}

// Validate は項目の整合と、状態と採否の記録の対応を確かめる。
func (p AssistProposal) Validate() error {
	problem := firstProblem(
		requirePresent("AssistProposal.id", p.Id),
		requireSanitized("AssistProposal.id", p.Id),
		ValidateAssistConversationId("AssistProposal.conversationId", p.ConversationId),
		ValidateAssistTurnId("AssistProposal.turnId", p.TurnId),
		requireKnownEnum("AssistProposal.provider", p.Provider),
		requireSanitized("AssistProposal.model", p.Model),
		requireKnownEnum("AssistProposal.state", p.State),
	)
	if problem != nil {
		return problem
	}
	if err := p.validateTarget(); err != nil {
		return err
	}
	if strings.TrimSpace(p.Note) == "" {
		return itemError("AssistProposal.note", ErrMissingRequiredItem)
	}
	if len(p.RecordRefs) == 0 {
		return itemError("AssistProposal.recordRefs", ErrMissingRequiredItem)
	}
	if err := validateElements("AssistProposal.recordRefs", p.RecordRefs); err != nil {
		return err
	}
	if err := validateAssistMatchConditions("AssistProposal.matchConditions", p.MatchConditions); err != nil {
		return err
	}
	if err := p.CreatedAt.Validate(); err != nil {
		return itemError("AssistProposal.createdAt", err)
	}
	return p.validateDecision()
}

// validateTarget は、対象がノード・関係・レコードのいずれかであり、関係を足す提案が関係を指すことを
// 確かめる。
func (p AssistProposal) validateTarget() error {
	if err := p.Target.Validate(); err != nil {
		return itemError("AssistProposal.target", err)
	}
	if p.Target.Kind == AssertionTargetKindSource {
		return itemError("AssistProposal.target.kind "+string(p.Target.Kind), ErrInvalid)
	}
	if p.AddsRelation && p.Target.Kind != AssertionTargetKindEdge {
		return itemError("AssistProposal.addsRelation on a target other than an edge", ErrUnexpectedItem)
	}
	return nil
}

// validateDecision は、提案中の提案が採否の記録を持たず、採否を決めた提案が記録を持つことを確かめる。
func (p AssistProposal) validateDecision() error {
	if p.State == AssistProposalStateProposed {
		if p.Decision != nil {
			return itemError("AssistProposal.decision on a proposed proposal", ErrUnexpectedItem)
		}
		return nil
	}
	if p.Decision == nil {
		return itemError("AssistProposal.decision", ErrMissingRequiredItem)
	}
	if err := p.Decision.validate(p.State); err != nil {
		return itemError("AssistProposal.decision", err)
	}
	return nil
}

// MarshalJSON は必須の集合を要素数 0 の場合も集合として出す。
func (p AssistProposal) MarshalJSON() ([]byte, error) {
	// items は AssistProposal の method を持たないため、この Marshal は再帰しない。
	type items AssistProposal
	copied := items(p)
	copied.RecordRefs = emptyIfNil(copied.RecordRefs)
	copied.MatchConditions = emptyIfNil(copied.MatchConditions)
	encoded, err := json.Marshal(copied)
	if err != nil {
		return nil, fmt.Errorf("marshaling AssistProposal: %w", err)
	}
	return encoded, nil
}
