package core_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// pairEvidence は検査を通る根拠のレコード 1 件を組む。
func pairEvidence() *core.GraphEvidence {
	return &core.GraphEvidence{
		RecordRef:       squidLocator(3),
		ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
	}
}

func TestEdgeRecordPairValidate(t *testing.T) {
	condition := core.EdgePairCondition{ConditionKey: core.EdgePairConditionLogonGuid}
	for _, item := range []struct {
		name  string
		pair  core.EdgeRecordPair
		valid bool
	}{
		{"one side", core.EdgeRecordPair{Right: pairEvidence(), Conditions: []core.EdgePairCondition{condition}}, true},
		{"no side", core.EdgeRecordPair{Conditions: []core.EdgePairCondition{condition}}, false},
		{"no condition", core.EdgeRecordPair{Left: pairEvidence(), Right: pairEvidence()}, false},
		{"unknown condition", core.EdgeRecordPair{Left: pairEvidence(),
			Conditions: []core.EdgePairCondition{{ConditionKey: "unknown"}}}, false},
		{"window and difference", core.EdgeRecordPair{Left: pairEvidence(), Conditions: []core.EdgePairCondition{{
			ConditionKey: core.EdgePairConditionTimeProximity, WindowSeconds: new(int64(2)), DifferenceSeconds: new(-0.5),
		}}}, true},
		{"negative window", core.EdgeRecordPair{Left: pairEvidence(), Conditions: []core.EdgePairCondition{{
			ConditionKey: core.EdgePairConditionTimeProximity, WindowSeconds: new(int64(-1)),
		}}}, false},
		{"difference without a window", core.EdgeRecordPair{Left: pairEvidence(), Conditions: []core.EdgePairCondition{{
			ConditionKey: core.EdgePairConditionTimeProximity, DifferenceSeconds: new(0.5),
		}}}, false},
		{"tally", core.EdgeRecordPair{Left: pairEvidence(), Conditions: []core.EdgePairCondition{condition},
			CandidateTally: &core.EdgeCandidateTally{CandidateCount: 3, PrecedingCandidateCount: 2}}, true},
		{"tally with every candidate preceding", core.EdgeRecordPair{Left: pairEvidence(),
			Conditions:     []core.EdgePairCondition{condition},
			CandidateTally: &core.EdgeCandidateTally{CandidateCount: 2, PrecedingCandidateCount: 2}}, false},
		{"tally with negative preceding", core.EdgeRecordPair{Left: pairEvidence(),
			Conditions:     []core.EdgePairCondition{condition},
			CandidateTally: &core.EdgeCandidateTally{CandidateCount: 2, PrecedingCandidateCount: -1}}, false},
	} {
		if err := item.pair.Validate(); (err == nil) != item.valid {
			t.Errorf("%s: Validate() = %v, want valid %v", item.name, err, item.valid)
		}
	}
}

func TestEdgeEvidenceAuthenticationRequiresACount(t *testing.T) {
	value, err := core.NewTextField("AuthenticationPackageName", core.SemanticKeyEventAuthenticationPackage,
		core.RawAndNormalized{RawText: new("NTLM"), ValueState: core.ValueStatePresent})
	if err != nil {
		t.Fatal(err)
	}
	if err := (core.EdgeEvidenceAuthentication{Value: value, EvidenceCount: 1}).Validate(); err != nil {
		t.Errorf("a counted authentication = %v, want valid", err)
	}
	if (core.EdgeEvidenceAuthentication{Value: value}).Validate() == nil {
		t.Error("an authentication without a count is valid, want invalid")
	}
}
