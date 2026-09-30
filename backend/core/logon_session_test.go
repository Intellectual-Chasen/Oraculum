package core_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 0 を詰めた文字列と詰めない文字列、大文字と小文字の文字列が、同じ比べる値になることを確かめる。
func TestLogonIdValueComparesZeroPaddedAndUppercaseForms(t *testing.T) {
	for name, text := range map[string]string{
		"0 を詰めた 16 桁": "0x00000000000a1b2c",
		"0 を詰めない":     "0xa1b2c",
		"大文字の数字":      "0xA1B2C",
		"大文字の接頭辞":     "0XA1B2C",
	} {
		t.Run(name, func(t *testing.T) {
			value := core.LogonIdValue(text, text)
			comparable, readable := value.ComparableValue()
			if !readable || comparable != "0xa1b2c" {
				t.Errorf("the comparable value is %q (readable %v), want 0xa1b2c", comparable, readable)
			}
			if raw, _ := value.RawTextValue(); raw != text {
				t.Errorf("the raw text is %q, want %q", raw, text)
			}
		})
	}
}

// 値 0 は 0x0 になる。0 の桁を全部除いて空の文字列にしない。
func TestLogonIdValueKeepsTheZeroValue(t *testing.T) {
	comparable, readable := core.LogonIdValue("0x0000000000000000", "0x0000000000000000").ComparableValue()
	if !readable || comparable != "0x0" {
		t.Errorf("the comparable value is %q (readable %v), want 0x0", comparable, readable)
	}
}

// 引用符を外した文字列から比べる値を作り、原資料の文字列は引用符ごと保つ。
func TestLogonIdValueKeepsTheQuotedRawText(t *testing.T) {
	value := core.LogonIdValue(`"0x1f"`, "0x1f")
	if raw, _ := value.RawTextValue(); raw != `"0x1f"` {
		t.Errorf("the raw text is %q", raw)
	}
	if comparable, _ := value.ComparableValue(); comparable != "0x1f" {
		t.Errorf("the comparable value is %q, want 0x1f", comparable)
	}
}

// 形に合わない文字列は範囲の外の値になり、比べる値を持たない。
func TestLogonIdValueRejectsTextOutsideTheForm(t *testing.T) {
	for name, text := range map[string]string{
		"接頭辞が無い":      "a1b2c",
		"数字が無い":       "0x",
		"16 進でない":     "0xg1",
		"符号がある":       "0x-1",
		"64 bit を超える": "0x1ffffffffffffffff",
		"値の不在の文字列":    "-",
	} {
		t.Run(name, func(t *testing.T) {
			value := core.LogonIdValue(text, text)
			if value.ValueState != core.ValueStateOutOfDefinition {
				t.Errorf("the value state is %q, want out_of_definition", value.ValueState)
			}
			if _, readable := value.ComparableValue(); readable {
				t.Error("a text outside the form has a comparable value")
			}
			if raw, _ := value.RawTextValue(); raw != text {
				t.Errorf("the raw text is %q, want %q", raw, text)
			}
		})
	}
}

func TestLogonSessionRejectionValidatesTheReasonAndTheLogon(t *testing.T) {
	locator := markIILocator(7)
	valid := core.LogonSessionRejection{
		Reason: core.LogonSessionRejectionOtherTerminal,
		Logon:  core.GraphEvidence{RecordRef: locator, ObservationKind: core.ObservationKind{Raw: []core.RecordField{}}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("a valid rejection is refused: %v", err)
	}
	unknown := valid
	unknown.Reason = "logged_on"
	if unknown.Validate() == nil {
		t.Error("a reason outside the contract is accepted")
	}
	broken := valid
	broken.Logon.RecordRef = core.RecordLocator{}
	if broken.Validate() == nil {
		t.Error("a logon without a record position is accepted")
	}
}
