package core_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func TestValueStateKnownValues(t *testing.T) {
	known := []core.ValueState{"present", "absent", "item_absent", "no_body", "out_of_definition",
		"derived", "derivation_undetermined"}
	for _, state := range known {
		if !state.IsKnown() {
			t.Errorf("valueState %q must be one of the 7 states", state)
		}
	}
	unknown := []core.ValueState{"missing", "null", "", "Present"}
	for _, state := range unknown {
		if state.IsKnown() {
			t.Errorf("valueState %q must not be accepted", state)
		}
	}
}

// 項目そのものが無い欄は文字列を持たない。値が 0 の欄は present である。
func TestRawAndNormalizedSeparatesAnAbsentItemFromAZeroValue(t *testing.T) {
	itemAbsent := core.NewAbsentItemValue()
	if err := itemAbsent.Validate(); err != nil {
		t.Fatalf("validating an absent item: %v", err)
	}

	zeroValue, err := core.NewRawValue(core.ValueStatePresent, "0")
	if err != nil {
		t.Fatalf("building a value of 0: %v", err)
	}
	if itemAbsent.ValueState == zeroValue.ValueState {
		t.Error("an absent item and a value of 0 must not share one state")
	}

	present := core.RawAndNormalized{ValueState: core.ValueStatePresent}
	if err := present.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
	}
}

// 原資料に実在する空文字列を、欄の不在の代用にしない。
//
// 空文字列の文字列を持つ present の項目と、欄が無い item_absent の項目を別の形にする。
func TestRawAndNormalizedSeparatesAnEmptyLexemeFromAnAbsentItem(t *testing.T) {
	empty, err := core.NewRawValue(core.ValueStatePresent, "")
	if err != nil {
		t.Fatalf("building a value whose lexeme is the empty string: %v", err)
	}
	rawText, ok := empty.RawTextValue()
	if !ok {
		t.Fatal("a present value must report its lexeme")
	}
	if rawText != "" {
		t.Errorf("rawText = %q, want the empty string", rawText)
	}

	itemAbsent := core.NewAbsentItemValue()
	if _, absentOk := itemAbsent.RawTextValue(); absentOk {
		t.Error("an item_absent value must not report a lexeme")
	}

	encoded, err := json.Marshal(empty)
	if err != nil {
		t.Fatalf("marshaling the value: %v", err)
	}
	want := `{"rawText":"","valueState":"present"}`
	if string(encoded) != want {
		t.Errorf("json = %s, want %s", encoded, want)
	}

	absentEncoded, err := json.Marshal(itemAbsent)
	if err != nil {
		t.Fatalf("marshaling the absent item: %v", err)
	}
	if string(absentEncoded) != `{"valueState":"item_absent"}` {
		t.Errorf("json = %s, want %s", absentEncoded, `{"valueState":"item_absent"}`)
	}
}

// 欄が無い項目は文字列を 1 つも持たない。
func TestRawAndNormalizedRejectsALexemeOnAnAbsentItem(t *testing.T) {
	rawText := "-"
	broken := core.RawAndNormalized{RawText: &rawText, ValueState: core.ValueStateItemAbsent}
	if err := broken.Validate(); !errors.Is(err, core.ErrUnexpectedItem) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrUnexpectedItem)
	}
}

// 正規化値を持つ項目は導き方を持つ。
func TestRawAndNormalizedRequiresTheDerivationOfTheNormalizedValue(t *testing.T) {
	rawText := `"C:\Tools\agent.exe"`
	normalized := `C:\Tools\agent.exe`
	value := core.RawAndNormalized{
		RawText:    &rawText,
		Normalized: &normalized,
		ValueState: core.ValueStatePresent,
	}
	if err := value.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
	}

	withDerivation, err := core.NewNormalizedValue(
		core.ValueStatePresent, rawText, normalized, "引用符を外した文字列")
	if err != nil {
		t.Fatalf("building the value: %v", err)
	}
	if got, ok := withDerivation.DerivationValue(); !ok || got != "引用符を外した文字列" {
		t.Errorf("derivation = (%q, %v), want (%q, true)", got, ok, "引用符を外した文字列")
	}

	derivation := "引用符を外した文字列"
	withoutNormalized := core.RawAndNormalized{
		RawText:    &rawText,
		Derivation: &derivation,
		ValueState: core.ValueStatePresent,
	}
	if err := withoutNormalized.Validate(); !errors.Is(err, core.ErrUnexpectedItem) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrUnexpectedItem)
	}
}

// RecordField は kind に対応する 1 つの値だけを持つ。
func TestRecordFieldCarriesOneValuePerKind(t *testing.T) {
	value, err := core.NewNormalizedValue(
		core.ValueStatePresent, "51234", "51234", "文字列をそのまま用いた")
	if err != nil {
		t.Fatalf("building a text value: %v", err)
	}
	textField, err := core.NewTextField("srcPort", core.SemanticKeyConnectionSourcePort, value)
	if err != nil {
		t.Fatalf("building a text field: %v", err)
	}
	if textField.Timestamp != nil {
		t.Error("a text field must not carry a timestamp")
	}
	if textField.Semantic != core.SemanticKeyConnectionSourcePort {
		t.Errorf("semantic = %q, want %q",
			textField.Semantic, core.SemanticKeyConnectionSourcePort)
	}

	timestampField, err := core.NewTimestampField(
		"headerTime", core.SemanticKeyEventTime, markIIEventTime(t))
	if err != nil {
		t.Fatalf("building a timestamp field: %v", err)
	}
	if timestampField.Text != nil {
		t.Error("a timestamp field must not carry a text value")
	}
	if timestampField.Timestamp.Precision != core.PrecisionMillisecond {
		t.Errorf("precision = %q, want %q",
			timestampField.Timestamp.Precision, core.PrecisionMillisecond)
	}
}

func TestRecordFieldRejectsABrokenDiscriminator(t *testing.T) {
	text, err := core.NewRawValue(core.ValueStatePresent, "51234")
	if err != nil {
		t.Fatalf("building a text value: %v", err)
	}
	timestamp := markIIEventTime(t)

	cases := map[string]struct {
		field core.RecordField
		want  error
	}{
		"a field without a kind": {
			field: core.RecordField{Name: "srcPort", Text: &text},
			want:  core.ErrUnknownEnumValue,
		},
		"a field with both values": {
			field: core.RecordField{
				Name:      "headerTime",
				Kind:      core.RecordFieldKindTimestamp,
				Text:      &text,
				Timestamp: &timestamp,
			},
			want: core.ErrInconsistentValue,
		},
		"a text kind without a text value": {
			field: core.RecordField{Name: "srcPort", Kind: core.RecordFieldKindText},
			want:  core.ErrMissingRequiredItem,
		},
		"a timestamp kind without a timestamp": {
			field: core.RecordField{Name: "headerTime", Kind: core.RecordFieldKindTimestamp},
			want:  core.ErrMissingRequiredItem,
		},
		"a field without a name": {
			field: core.RecordField{Kind: core.RecordFieldKindText, Text: &text},
			want:  core.ErrMissingRequiredItem,
		},
		"a field with a semantic outside the vocabulary": {
			field: core.RecordField{
				Name:     "srcPort",
				Semantic: "connection.source_portnumber",
				Kind:     core.RecordFieldKindText,
				Text:     &text,
			},
			want: core.ErrUnknownEnumValue,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			if err := testCase.field.Validate(); !errors.Is(err, testCase.want) {
				t.Errorf("error = %v, want one wrapping %v", err, testCase.want)
			}
		})
	}
}

func TestRecordFieldUnmarshalRejectsABrokenObject(t *testing.T) {
	cases := map[string]string{
		"an object without a kind": `{"name":"srcPort","text":{"rawText":"51234",` +
			`"valueState":"present"}}`,
		"an object with both values": `{"name":"headerTime","kind":"timestamp",` +
			`"text":{"rawText":"51234","valueState":"present"},` +
			`"timestamp":{"rawText":"03/14/2024 10:20:30.482 +0900",` +
			`"normalized":"2024-03-14T10:20:30.482+09:00","precision":"millisecond",` +
			`"offsetState":"in_value","offsetText":"+0900","clock":"terminal_local",` +
			`"meaning":"event","valueState":"present"}}`,
		"an item outside the contract": `{"name":"srcPort","kind":"text","unit":"port"}`,
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			var field core.RecordField
			if err := json.Unmarshal([]byte(encoded), &field); err == nil {
				t.Error("decoding must fail")
			}
		})
	}
}

func TestRecordFieldJsonRoundTrip(t *testing.T) {
	field, err := core.NewTimestampField(
		"headerTime", core.SemanticKeyEventTime, markIIEventTime(t))
	if err != nil {
		t.Fatalf("building a timestamp field: %v", err)
	}
	encoded, err := json.Marshal(field)
	if err != nil {
		t.Fatalf("marshaling the field: %v", err)
	}
	var decoded core.RecordField
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decoding the field: %v", err)
	}
	if decoded.Name != "headerTime" {
		t.Errorf("name = %q, want %q", decoded.Name, "headerTime")
	}
	if decoded.Kind != core.RecordFieldKindTimestamp {
		t.Errorf("kind = %q, want %q", decoded.Kind, core.RecordFieldKindTimestamp)
	}
	if decoded.Semantic != core.SemanticKeyEventTime {
		t.Errorf("semantic = %q, want %q", decoded.Semantic, core.SemanticKeyEventTime)
	}
	if decoded.Timestamp == nil {
		t.Fatal("the decoded field must carry a timestamp")
	}
	assertSameTimestampItems(t, *field.Timestamp, *decoded.Timestamp)
}

// 時刻の条件の値を kind が text の項目で持たない。
func TestTimeConditionRejectsATextValue(t *testing.T) {
	requestTimeText, err := core.NewRawValue(core.ValueStatePresent, "[14/Mar/2024:10:20:30 +0900]")
	if err != nil {
		t.Fatalf("building a text value: %v", err)
	}
	textField, err := core.NewTextField("requestTime", core.SemanticKeyEventTime, requestTimeText)
	if err != nil {
		t.Fatalf("building a text field: %v", err)
	}
	timestampField, err := core.NewTimestampField(
		"headerTime", core.SemanticKeyEventTime, markIIEventTime(t))
	if err != nil {
		t.Fatalf("building a timestamp field: %v", err)
	}
	condition := core.MatchCondition{
		ConditionKey: core.ConditionKeySecondOfTime,
		Use:          core.ConditionUseUsed,
		LeftValue:    []core.RecordField{textField},
	}
	if err := condition.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
	}

	squidTimeField, err := core.NewTimestampField(
		"requestTime", core.SemanticKeyEventTime, squidRequestTime(t))
	if err != nil {
		t.Fatalf("building a timestamp field: %v", err)
	}
	condition.LeftValue = []core.RecordField{squidTimeField}
	if err := condition.Validate(); err != nil {
		t.Fatalf("validating the condition: %v", err)
	}
	if condition.LeftValue[0].Timestamp.Precision != core.PrecisionSecond {
		t.Errorf("left precision = %q, want %q",
			condition.LeftValue[0].Timestamp.Precision, core.PrecisionSecond)
	}

	// 時刻の 2 条件は候補の側の値を出さない。候補の側の時刻は候補 1 件ごとに変わり、
	// 段階に 1 組しか置けない rightValue では持てない。
	condition.RightValue = []core.RecordField{timestampField}
	if err := condition.Validate(); !errors.Is(err, core.ErrInconsistentValue) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
	}
}

// 端末の割当を用いた条件は、適用期間と期間の外かの判定を揃えて持つ。
func TestTerminalAssignmentConditionRequiresTheRangeAndTheOutsideFlag(t *testing.T) {
	condition := core.MatchCondition{
		ConditionKey: core.ConditionKeyTerminalIpAssignment,
		Use:          core.ConditionUseNotUsed,
	}
	if err := condition.Validate(); !errors.Is(err, core.ErrMissingRequiredItem) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
	}

	validRange := markIIAssignment(t).AssignmentValidRange
	outside := false
	condition.AssignmentValidRange = &validRange
	condition.OutsideAssignmentRange = &outside
	if err := condition.Validate(); err != nil {
		t.Fatalf("validating the condition: %v", err)
	}
}

// 用いた条件は起点の側の値を必ず持つ。候補の側の値は候補の総数に依る。
//
// memberCount が 1 以上のとき rightValue を出し、0 のとき出さない。
func TestUsedConditionRequiresTheRightValueOnlyWhenCandidatesExist(t *testing.T) {
	withRight := destinationIpCondition(t, true)
	if err := core.ValidateMatchConditions([]core.MatchCondition{withRight}, 4); err != nil {
		t.Fatalf("validating a condition with candidates: %v", err)
	}

	t.Run("omitting the right value while candidates exist", func(t *testing.T) {
		withoutRight := destinationIpCondition(t, false)
		err := core.ValidateMatchConditions([]core.MatchCondition{withoutRight}, 4)
		if !errors.Is(err, core.ErrMissingRequiredItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
		}
	})

	t.Run("carrying the right value while no candidate exists", func(t *testing.T) {
		err := core.ValidateMatchConditions([]core.MatchCondition{withRight}, 0)
		if !errors.Is(err, core.ErrInconsistentValue) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrInconsistentValue)
		}
	})

	t.Run("omitting the right value while no candidate exists", func(t *testing.T) {
		withoutRight := destinationIpCondition(t, false)
		if err := core.ValidateMatchConditions([]core.MatchCondition{withoutRight}, 0); err != nil {
			t.Fatalf("validating a condition without candidates: %v", err)
		}
	})

	// 起点の側の値は候補の有無に依らず定まる。
	t.Run("omitting the left value", func(t *testing.T) {
		withoutLeft := destinationIpCondition(t, false)
		withoutLeft.LeftValue = nil
		err := core.ValidateMatchConditions([]core.MatchCondition{withoutLeft}, 0)
		if !errors.Is(err, core.ErrMissingRequiredItem) {
			t.Errorf("error = %v, want one wrapping %v", err, core.ErrMissingRequiredItem)
		}
	})
}
