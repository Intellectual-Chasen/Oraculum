package core

import "testing"

func TestEveryKnownRelationDerivationOutcomeIsAccepted(t *testing.T) {
	for _, outcome := range KnownRelationDerivationOutcomes() {
		if !outcome.IsKnown() {
			t.Fatalf("%q is listed and rejected by IsKnown", outcome)
		}
	}
	for _, outside := range []RelationDerivationOutcome{"", "MATCHED", "no_such_outcome"} {
		if outside.IsKnown() {
			t.Fatalf("%q is outside the contract and accepted", outside)
		}
	}
}

// 分類ごとの区分を漏れなく置く。区分を持たない分類は、分析者が次に採る手を読めない。
func TestEveryOutcomeCarriesABasis(t *testing.T) {
	for _, outcome := range KnownRelationDerivationOutcomes() {
		basis := outcome.BasisOf()
		if !basis.IsKnown() {
			t.Fatalf("%q carries the basis %q, which is outside the contract",
				outcome, basis)
		}
	}
	// 契約の外の分類は区分を持たない。呼び出し元が分類を先に検査する形を保つ。
	if basis := RelationDerivationOutcome("no_such_outcome").BasisOf(); basis != "" {
		t.Fatalf("an outcome outside the contract carries the basis %q", basis)
	}
}

// 原資料について言える事実と、本ツールが処理できなかったことを別の区分で示す。
func TestRelationDerivationSeparatesTheSourceFactFromTheInternalGap(t *testing.T) {
	for _, item := range []struct {
		outcome RelationDerivationOutcome
		want    RelationDerivationBasis
	}{
		{RelationDerivationNoCandidateRecord, RelationDerivationBasisSourceFact},
		{RelationDerivationNoCandidateInWindow, RelationDerivationBasisSourceFact},
		{RelationDerivationNoCandidateMatchingConditions, RelationDerivationBasisSourceFact},
		{RelationDerivationDestinationIpAbsent, RelationDerivationBasisItemAbsent},
		{RelationDerivationOriginItemUnreadable, RelationDerivationBasisItemAbsent},
		{
			RelationDerivationSourceDeclarationConflict,
			RelationDerivationBasisSourceDeclarationDefect,
		},
		{RelationDerivationNodeUnresolved, RelationDerivationBasisInternalGap},
		{RelationDerivationFailed, RelationDerivationBasisInternalGap},
		{RelationDerivationMatched, RelationDerivationBasisNotApplicable},
		{RelationDerivationNotUsedAsOrigin, RelationDerivationBasisNotApplicable},
	} {
		if got := item.outcome.BasisOf(); got != item.want {
			t.Fatalf("%q carries the basis %q, want %q", item.outcome, got, item.want)
		}
	}
}

func TestNewRelationDerivationAddsTheBasis(t *testing.T) {
	built, err := NewRelationDerivation(RelationDerivationNoCandidateInWindow)
	if err != nil {
		t.Fatalf("building the derivation: %v", err)
	}
	if built.Outcome != RelationDerivationNoCandidateInWindow {
		t.Fatalf("outcome=%q", built.Outcome)
	}
	if built.Basis != RelationDerivationBasisSourceFact {
		t.Fatalf("basis=%q want %q", built.Basis, RelationDerivationBasisSourceFact)
	}
	if err := built.Validate(); err != nil {
		t.Fatalf("the built derivation did not validate: %v", err)
	}
}

func TestNewRelationDerivationRejectsAnOutcomeOutsideTheContract(t *testing.T) {
	if _, err := NewRelationDerivation("no_such_outcome"); err == nil {
		t.Fatal("an outcome outside the contract built a derivation")
	}
}

// 分類と区分の組が食い違う値を退ける。
func TestRelationDerivationRejectsABasisThatDoesNotFollowTheOutcome(t *testing.T) {
	derivation := RelationDerivation{
		Outcome: RelationDerivationNoCandidateRecord,
		Basis:   RelationDerivationBasisInternalGap,
	}
	if err := derivation.Validate(); err == nil {
		t.Fatal("a derivation whose basis contradicts its outcome validated")
	}
}

func TestRelationDerivationRejectsAnEmptyOutcome(t *testing.T) {
	if err := (RelationDerivation{}).Validate(); err == nil {
		t.Fatal("an empty derivation validated")
	}
}
