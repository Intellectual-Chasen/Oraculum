package core_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func TestComparableValuePrefersNormalized(t *testing.T) {
	value, err := core.NewNormalizedValue(core.ValueStatePresent, `"agent"`, "agent", "引用符を外した文字列")
	if err != nil {
		t.Fatal(err)
	}
	got, ok := value.ComparableValue()
	if !ok || got != "agent" {
		t.Fatalf("ComparableValue = %q %v, want %q true", got, ok, "agent")
	}
}

func TestComparableValueFallsBackToRawText(t *testing.T) {
	value, err := core.NewRawValue(core.ValueStatePresent, "198.51.100.42")
	if err != nil {
		t.Fatal(err)
	}
	got, ok := value.ComparableValue()
	if !ok || got != "198.51.100.42" {
		t.Fatalf("ComparableValue = %q %v, want %q true", got, ok, "198.51.100.42")
	}
}

func TestComparableValueUsesDerivedNormalized(t *testing.T) {
	value, err := core.NewDerivedValue("TESTHOST", "IP から端末への割当を適用した")
	if err != nil {
		t.Fatal(err)
	}
	got, ok := value.ComparableValue()
	if !ok || got != "TESTHOST" {
		t.Fatalf("ComparableValue = %q %v, want %q true", got, ok, "TESTHOST")
	}
}

func TestComparableValueRejectsStatesWithoutAValue(t *testing.T) {
	absent, err := core.NewRawValue(core.ValueStateAbsent, "-")
	if err != nil {
		t.Fatal(err)
	}
	undetermined, err := core.NewDerivationUndeterminedValue("割当を適用できない")
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]core.RawAndNormalized{
		"absent":                  absent,
		"item_absent":             core.NewAbsentItemValue(),
		"derivation_undetermined": undetermined,
	} {
		got, ok := value.ComparableValue()
		if ok || got != "" {
			t.Errorf("%s ComparableValue = %q %v, want %q false", name, got, ok, "")
		}
	}
}
