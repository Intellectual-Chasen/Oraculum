package core_test

import (
	"errors"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// kindField は観測の種別を持つ項目 1 件を組む。
func kindField(t *testing.T, name, value string) core.RecordField {
	t.Helper()
	text, err := core.NewRawValue(core.ValueStatePresent, value)
	if err != nil {
		t.Fatalf("building the value of %q: %v", name, err)
	}
	field, err := core.NewTextField(name, "", text)
	if err != nil {
		t.Fatalf("building the field %q: %v", name, err)
	}
	return field
}

// selectorOf は名前と値の組から選ぶ条件を組む。引数は name, value の順の並びである。
func selectorOf(pairs ...string) core.ObservationKindSelector {
	items := make([]core.ObservationKindSelectorItem, 0, len(pairs)/2)
	for index := 0; index+1 < len(pairs); index += 2 {
		items = append(items, core.ObservationKindSelectorItem{
			Name: pairs[index], Value: pairs[index+1],
		})
	}
	return core.ObservationKindSelector{Items: items}
}

// 観測の種別の欄の名前と個数は入力形式が決める。
func TestObservationKindAcceptsTheColumnsOfAnyInputFormat(t *testing.T) {
	cases := map[string][]core.RecordField{
		"種別を 1 欄で書く形式": {kindField(t, "EventID", "5156")},
		"種別を 2 欄で書く形式": {kindField(t, "evt", "net"), kindField(t, "subEvt", "con")},
		"種別を 3 欄で書く形式": {
			kindField(t, "Channel", "Security"),
			kindField(t, "Provider", "Microsoft-Windows-Security-Auditing"),
			kindField(t, "EventID", "5156"),
		},
		"種別の欄を持たない形式": {},
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			kind := core.ObservationKind{Raw: raw}
			if len(raw) > 0 {
				kind.Status = core.ObservationKindStatusDetermined
			}
			if err := kind.Validate(); err != nil {
				t.Errorf("validating the observation kind: %v", err)
			}
		})
	}
}

// 意味の状態と推定した意味の組み合わせは、inferred のときだけ意味を持つ。
func TestObservationKindRequiresTheMeaningOfAnInferredKind(t *testing.T) {
	raw := []core.RecordField{kindField(t, "evt", "net"), kindField(t, "subEvt", "est")}
	const meaning = "接続が続いている間の通信を示す"
	for _, testCase := range []struct {
		name    string
		status  core.ObservationKindStatus
		meaning string
		want    error
	}{
		{
			name: "推定した種別が意味を持つ", status: core.ObservationKindStatusInferred,
			meaning: meaning, want: nil,
		},
		{
			name: "推定した種別が意味を持たない", status: core.ObservationKindStatusInferred,
			meaning: "", want: core.ErrMissingRequiredItem,
		},
		{
			name: "形式が意味を定める種別が意味を持つ", status: core.ObservationKindStatusDetermined,
			meaning: meaning, want: core.ErrInconsistentValue,
		},
		{
			name: "意味を確定できない種別が意味を持つ", status: core.ObservationKindStatusUndetermined,
			meaning: meaning, want: core.ErrInconsistentValue,
		},
		{
			name: "形式が意味を定める種別が意味を持たない", status: core.ObservationKindStatusDetermined,
			meaning: "", want: nil,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			kind := core.ObservationKind{
				Raw: raw, Status: testCase.status, Meaning: testCase.meaning,
			}

			err := kind.Validate()

			if testCase.want == nil {
				if err != nil {
					t.Errorf("validating the observation kind: %v", err)
				}
				return
			}
			if !errors.Is(err, testCase.want) {
				t.Errorf("error = %v, want one wrapping %v", err, testCase.want)
			}
		})
	}
}

// 同じ名前の欄を 2 つ持つ観測の種別は、どちらの値を比べたかが読めない。
func TestObservationKindRejectsARepeatedColumnName(t *testing.T) {
	kind := core.ObservationKind{
		Raw: []core.RecordField{
			kindField(t, "EventID", "5156"),
			kindField(t, "EventID", "5158"),
		},
		Status: core.ObservationKindStatusDetermined,
	}
	if err := kind.Validate(); !errors.Is(err, core.ErrDuplicateElement) {
		t.Errorf("error = %v, want one wrapping %v", err, core.ErrDuplicateElement)
	}
}

// Matches は、選ぶ条件が挙げた欄すべての値が一致し、欄の個数も一致するときだけ真を返す。
func TestObservationKindMatchesTheWholeSetOfColumns(t *testing.T) {
	twoColumns := core.ObservationKind{
		Raw: []core.RecordField{
			kindField(t, "evt", "net"), kindField(t, "subEvt", "con"),
		},
		Status: core.ObservationKindStatusDetermined,
	}
	oneColumn := core.ObservationKind{
		Raw:    []core.RecordField{kindField(t, "EventID", "5156")},
		Status: core.ObservationKindStatusDetermined,
	}
	cases := map[string]struct {
		kind     core.ObservationKind
		selector core.ObservationKindSelector
		want     bool
	}{
		"2 欄が両方一致する":        {twoColumns, selectorOf("evt", "net", "subEvt", "con"), true},
		"2 欄の並びが逆でも一致する":    {twoColumns, selectorOf("subEvt", "con", "evt", "net"), true},
		"2 欄のうち 1 件の値が違う":   {twoColumns, selectorOf("evt", "net", "subEvt", "est"), false},
		"選ぶ条件が欄を 1 件しか挙げない": {twoColumns, selectorOf("evt", "net"), false},
		"選ぶ条件が知らない欄を挙げる":    {twoColumns, selectorOf("evt", "net", "EventID", "5156"), false},
		"1 欄が一致する":          {oneColumn, selectorOf("EventID", "5156"), true},
		"1 欄の値が違う":          {oneColumn, selectorOf("EventID", "5158"), false},
		"欄を持たない観測":          {core.ObservationKind{Raw: []core.RecordField{}}, selectorOf("evt", "net"), false},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			if got := want.kind.Matches(want.selector); got != want.want {
				t.Errorf("Matches = %t, want %t", got, want.want)
			}
		})
	}
}

// 選ぶ条件は欄を 1 件以上挙げ、各要素に名前と値を持ち、名前が重ならない。
func TestObservationKindSelectorRequiresNamedValues(t *testing.T) {
	cases := map[string]struct {
		selector core.ObservationKindSelector
		want     error
	}{
		"欄を 1 件も挙げない": {core.ObservationKindSelector{}, core.ErrMissingRequiredItem},
		"名前が無い":       {selectorOf("", "net"), core.ErrMissingRequiredItem},
		"値が無い":        {selectorOf("evt", ""), core.ErrMissingRequiredItem},
		"名前が重なる":      {selectorOf("evt", "net", "evt", "os"), core.ErrDuplicateElement},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			if err := want.selector.Validate(); !errors.Is(err, want.want) {
				t.Errorf("error = %v, want one wrapping %v", err, want.want)
			}
		})
	}
	t.Run("欄を 1 件挙げる", func(t *testing.T) {
		if err := selectorOf("EventID", "5156").Validate(); err != nil {
			t.Errorf("validating a one-column selector: %v", err)
		}
	})
}
