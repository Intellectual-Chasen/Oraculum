package core_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func TestDerivedValueValidation(t *testing.T) {
	type validationCase struct {
		value core.RawAndNormalized
		want  error
	}
	text, reason, empty := "synthetic-value", "synthetic derivation", ""
	for _, state := range []core.ValueState{core.ValueStateDerived, core.ValueStateDerivationUndetermined} {
		t.Run(string(state), func(t *testing.T) {
			var normalized *string
			if state == core.ValueStateDerived {
				normalized = &text
			}
			valid := core.RawAndNormalized{ValueState: state, Normalized: normalized, Derivation: &reason}
			cases := map[string]validationCase{
				"valid":              {valid, nil},
				"raw text":           {core.RawAndNormalized{ValueState: state, RawText: &text, Normalized: normalized, Derivation: &reason}, core.ErrUnexpectedItem},
				"missing derivation": {core.RawAndNormalized{ValueState: state, Normalized: normalized}, core.ErrMissingRequiredItem},
				"empty derivation":   {core.RawAndNormalized{ValueState: state, Normalized: normalized, Derivation: &empty}, core.ErrMissingRequiredItem},
			}
			if state == core.ValueStateDerived {
				cases["missing normalized"] = validationCase{core.RawAndNormalized{ValueState: state, Derivation: &reason}, core.ErrMissingRequiredItem}
				cases["empty normalized"] = validationCase{core.RawAndNormalized{ValueState: state, Normalized: &empty, Derivation: &reason}, nil}
			} else {
				cases["unexpected normalized"] = validationCase{core.RawAndNormalized{ValueState: state, Normalized: &text, Derivation: &reason}, core.ErrUnexpectedItem}
				cases["empty normalized"] = validationCase{core.RawAndNormalized{ValueState: state, Normalized: &empty, Derivation: &reason}, core.ErrUnexpectedItem}
			}
			for name, tc := range cases {
				t.Run(name, func(t *testing.T) {
					if err := tc.value.Validate(); !errors.Is(err, tc.want) {
						t.Errorf("Validate() = %v, want %v", err, tc.want)
					}
				})
			}
		})
	}
}

func TestOriginalValueStatesRemainValid(t *testing.T) {
	text, empty := "synthetic", ""
	for _, state := range []core.ValueState{core.ValueStatePresent, core.ValueStateAbsent, core.ValueStateNoBody, core.ValueStateOutOfDefinition} {
		t.Run(string(state), func(t *testing.T) {
			valid := core.RawAndNormalized{ValueState: state, RawText: &text}
			if err := valid.Validate(); err != nil {
				t.Fatal(err)
			}
			valid.Normalized, valid.Derivation = &empty, &text
			if err := valid.Validate(); err != nil {
				t.Fatal(err)
			}
			valid.Derivation = &empty
			if err := valid.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
				t.Errorf("empty derivation: Validate() = %v", err)
			}
			if _, err := core.NewNormalizedValue(state, text, empty, empty); !errors.Is(err, core.ErrMissingRequiredItem) {
				t.Errorf("empty derivation: NewNormalizedValue() = %v", err)
			}
			valid.Derivation = &text
			valid.RawText = nil
			if err := valid.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
				t.Errorf("Validate() = %v", err)
			}
		})
	}
	item := core.NewAbsentItemValue()
	if err := item.Validate(); err != nil {
		t.Fatal(err)
	}
	item.Derivation = &text
	if err := item.Validate(); !errors.Is(err, core.ErrUnexpectedItem) {
		t.Errorf("Validate() = %v", err)
	}
}

func TestDerivedConstructorsAndJSON(t *testing.T) {
	for _, normalized := range []string{"synthetic", ""} {
		value, err := core.NewDerivedValue(normalized, "synthetic derivation")
		if err != nil {
			t.Fatal(err)
		}
		if value.ValueState != core.ValueStateDerived || value.RawText != nil {
			t.Fatalf("unexpected value: %+v", value)
		}
		if got, ok := value.NormalizedValue(); !ok || got != normalized {
			t.Errorf("normalized = %q, %v", got, ok)
		}
		assertDerivedFieldRoundTrip(t, value)
	}
	value, err := core.NewDerivationUndeterminedValue("synthetic reason")
	if err != nil {
		t.Fatal(err)
	}
	if value.ValueState != core.ValueStateDerivationUndetermined || value.RawText != nil || value.Normalized != nil {
		t.Fatalf("unexpected value: %+v", value)
	}
	if got, ok := value.DerivationValue(); !ok || got != "synthetic reason" {
		t.Errorf("derivation = %q, %v", got, ok)
	}
	assertDerivedFieldRoundTrip(t, value)
	if _, err := core.NewDerivedValue("synthetic", ""); !errors.Is(err, core.ErrMissingRequiredItem) {
		t.Errorf("empty derivation: %v", err)
	}
	if _, err := core.NewDerivationUndeterminedValue(""); !errors.Is(err, core.ErrMissingRequiredItem) {
		t.Errorf("empty reason: %v", err)
	}
	for _, state := range []core.ValueState{core.ValueStateDerived, core.ValueStateDerivationUndetermined} {
		if _, err := core.NewRawValue(state, "synthetic"); !errors.Is(err, core.ErrUnexpectedItem) {
			t.Errorf("NewRawValue(%s) = %v", state, err)
		}
		if _, err := core.NewNormalizedValue(state, "synthetic", "synthetic", "reason"); !errors.Is(err, core.ErrUnexpectedItem) {
			t.Errorf("NewNormalizedValue(%s) = %v", state, err)
		}
	}
}

func assertDerivedFieldRoundTrip(t *testing.T, value core.RawAndNormalized) {
	t.Helper()
	field, err := core.NewTextField("synthetic", "", value)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(field)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"rawText"`) {
		t.Errorf("unexpected rawText: %s", encoded)
	}
	if (value.Normalized != nil) != strings.Contains(string(encoded), `"normalized"`) {
		t.Errorf("normalized presence: %s", encoded)
	}
	var decoded core.RecordField
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, field) {
		t.Errorf("round trip = %+v, want %+v", decoded, field)
	}
}

func TestRecordFieldRejectsUnknownValueState(t *testing.T) {
	var field core.RecordField
	err := json.Unmarshal([]byte(`{"name":"synthetic","kind":"text","text":{"rawText":"synthetic","valueState":"unknown"}}`), &field)
	if !errors.Is(err, core.ErrUnknownEnumValue) {
		t.Errorf("UnmarshalJSON = %v", err)
	}
}

func TestTimestampRejectsDerivedStates(t *testing.T) {
	for _, state := range []core.ValueState{core.ValueStateDerived, core.ValueStateDerivationUndetermined} {
		t.Run(string(state), func(t *testing.T) {
			value := markIIEventTime(t)
			value.ValueState = state
			err := value.Validate()
			if !errors.Is(err, core.ErrInconsistentValue) || !strings.Contains(err.Error(), "has no derivation field") {
				t.Errorf("Validate() = %v", err)
			}
			if _, err := core.NewTimestamp(value); !errors.Is(err, core.ErrInconsistentValue) {
				t.Errorf("NewTimestamp() = %v", err)
			}
		})
	}
}

func TestObservationKindOmitsStatusForDerivedStates(t *testing.T) {
	derived, err := core.NewDerivedValue("synthetic", "synthetic derivation")
	if err != nil {
		t.Fatal(err)
	}
	undetermined, err := core.NewDerivationUndeterminedValue("synthetic reason")
	if err != nil {
		t.Fatal(err)
	}
	present, err := core.NewRawValue(core.ValueStatePresent, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []core.RawAndNormalized{derived, undetermined, present} {
		t.Run(string(value.ValueState), func(t *testing.T) {
			kind := core.ObservationKind{Raw: []core.RecordField{
				{Name: "evt", Kind: core.RecordFieldKindText, Text: &value},
				{Name: "subEvt", Kind: core.RecordFieldKindText, Text: &present},
			}}
			for _, status := range []core.ObservationKindStatus{"", core.ObservationKindStatusDetermined, core.ObservationKindStatusUndetermined} {
				kind.Status = status
				wantValid := (value.ValueState == core.ValueStatePresent) == (status != "")
				if err := kind.Validate(); (err == nil) != wantValid {
					t.Errorf("status %q: %v, want valid %v", status, err, wantValid)
				}
			}
		})
	}
}
