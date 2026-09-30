package core_test

import (
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 案件の文字列は英数字と `.`、`_`、`-` の 1 byte 以上 64 byte 以下である。
func TestValidateCaseId(t *testing.T) {
	for _, value := range []string{"baseline", "challenge-2025", "case_1.a", strings.Repeat("a", 64)} {
		if err := core.ValidateCaseId("case", value); err != nil {
			t.Errorf("the case %q is rejected: %v", value, err)
		}
	}
	for _, value := range []string{"", "has space", "slash/case", "改行\n", "案件", strings.Repeat("a", 65)} {
		if err := core.ValidateCaseId("case", value); err == nil {
			t.Errorf("the case %q is accepted", value)
		}
	}
}

// 案件ごとの件数は、案件の昇順に並び、和が全体の件数に等しい。
func TestGraphEdgeValidatesTheBreakdownByCase(t *testing.T) {
	edge := core.GraphEdge{
		Id: "e:1", Kind: core.EdgeKindRanOn, State: core.RelationStateObserved,
		SourceNodeId: "n:1", TargetNodeId: "n:2", EvidenceCount: 3,
	}
	for _, testCase := range []struct {
		name   string
		counts []core.CaseEvidenceCount
		valid  bool
	}{
		{"absent", nil, true},
		{"sorted and summed", []core.CaseEvidenceCount{{CaseId: "baseline", EvidenceCount: 1},
			{CaseId: "challenge", EvidenceCount: 2}}, true},
		{"a short sum", []core.CaseEvidenceCount{{CaseId: "baseline", EvidenceCount: 1}}, false},
		{"unsorted", []core.CaseEvidenceCount{{CaseId: "challenge", EvidenceCount: 2},
			{CaseId: "baseline", EvidenceCount: 1}}, false},
		{"a duplicate", []core.CaseEvidenceCount{{CaseId: "baseline", EvidenceCount: 1},
			{CaseId: "baseline", EvidenceCount: 2}}, false},
		{"a zero count", []core.CaseEvidenceCount{{CaseId: "baseline", EvidenceCount: 0},
			{CaseId: "challenge", EvidenceCount: 3}}, false},
		{"an unreadable case", []core.CaseEvidenceCount{{CaseId: "", EvidenceCount: 3}}, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			candidate := edge
			candidate.EvidenceByCase = testCase.counts
			if err := candidate.Validate(); (err == nil) != testCase.valid {
				t.Errorf("Validate() = %v, want valid=%t", err, testCase.valid)
			}
		})
	}
}

// 収集元に付けた案件の文字列を検査する。
func TestSourceIdentityValidatesTheCase(t *testing.T) {
	identity := core.SourceIdentity{
		SourceId: "source", ContentSha256: strings.Repeat("a", 64), OriginPath: "logs/a.log",
		FileName: "a.log", LineEnding: core.LineEndingLf, FormatKey: "format",
	}
	if err := identity.Validate(); err != nil {
		t.Fatalf("the identity without a case is rejected: %v", err)
	}
	valid, empty := "baseline", ""
	identity.CaseId = &valid
	if err := identity.Validate(); err != nil {
		t.Errorf("the identity with a case is rejected: %v", err)
	}
	identity.CaseId = &empty
	if err := identity.Validate(); err == nil {
		t.Error("the identity with an empty case is accepted")
	}
}
