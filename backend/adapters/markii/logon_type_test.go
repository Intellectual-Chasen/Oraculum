package markii_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// logonRecordWith は evt が os、subEvt が evtLog のログオンのレコードを、logonType の欄を
// 差し替えて組む。空の文字列を渡すと logonType の key を持たない。値は原資料の文字列の形に
// 合わせている。
func logonRecordWith(logonTypeItem string) string {
	return strings.Join(slices.DeleteFunc([]string{
		"02/01/2000 03:04:05.678 +0900 loc=ja-JP type=ITM2 sn=802100 lv=5",
		"evt=os subEvt=evtLog os=Win com=\"HOST01\" domain=\"EXAMPLE\"",
		"tmid=" + recordObservationTerminal + " channel=\"Security\" evtRecID=7001",
		"evtID=4624 evtUsr=\"user01\" evtDomain=\"EXAMPLE\"", logonTypeItem,
		"wsName=\"CLIENT01\" wsIp=\"192.0.2.20\" wsPort=50000",
	}, func(part string) bool { return part == "" }), " ")
}

// logonType の値の括弧の中の数字を、原資料の文字列と別の項目の種別のコードとして持つ。
func TestParseRecordObservationCarriesTheLogonTypeCode(t *testing.T) {
	cases := map[string]struct {
		item string
		want string
	}{
		"ネットワーク":      {item: `logonType="Network(3)"`, want: "3"},
		"リモート対話":      {item: `logonType="RemoteInteractive(10)"`, want: "10"},
		"先頭に 0 を置く数字": {item: `logonType="Network(03)"`, want: "3"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fields := parseRecordObservationOK(t, logonRecordWith(tc.item)).Fields
			code := fieldByName(t, fields, "logonTypeCode")
			if err := code.Validate(); err != nil {
				t.Fatal(err)
			}
			if code.Semantic != core.SemanticKeyEventLogonType {
				t.Errorf("semantic = %q, want %q", code.Semantic, core.SemanticKeyEventLogonType)
			}
			if code.Text.ValueState != core.ValueStateDerived {
				t.Errorf("valueState = %q, want %q", code.Text.ValueState, core.ValueStateDerived)
			}
			if got, ok := code.Text.ComparableValue(); !ok || got != tc.want {
				t.Errorf("comparable = %q (%t), want %q", got, ok, tc.want)
			}
			assertRawLogonTypeKept(t, fields, tc.item)
		})
	}
}

// 形に合わない値は、原資料の文字列を保ち、種別のコードを導出未確定にする。数字を推し量らない。
func TestParseRecordObservationLeavesTheLogonTypeCodeUndetermined(t *testing.T) {
	for name, item := range map[string]string{
		"括弧を持たない":      `logonType="Network"`,
		"名前を持たない":      `logonType="(3)"`,
		"括弧を閉じない":      `logonType="Network(3"`,
		"括弧の中が数字でない":   `logonType="Network(x)"`,
		"括弧の中が空":       `logonType="Network()"`,
		"括弧の中が符号付き":    `logonType="Network(+3)"`,
		"括弧の後ろに文字列がある": `logonType="Network(3) extra"`,
	} {
		t.Run(name, func(t *testing.T) {
			fields := parseRecordObservationOK(t, logonRecordWith(item)).Fields
			code := fieldByName(t, fields, "logonTypeCode")
			if err := code.Validate(); err != nil {
				t.Fatal(err)
			}
			if code.Text.ValueState != core.ValueStateDerivationUndetermined {
				t.Errorf("valueState = %q, want %q",
					code.Text.ValueState, core.ValueStateDerivationUndetermined)
			}
			if got, ok := code.Text.ComparableValue(); ok {
				t.Errorf("comparable = %q, want no comparable value", got)
			}
			assertRawLogonTypeKept(t, fields, item)
		})
	}
}

// logonType の key を持たないレコードと、値の不在を表す文字列を持つレコードに、種別の
// コードを補わない。
func TestParseRecordObservationAddsNoLogonTypeCodeWithoutTheValue(t *testing.T) {
	for name, item := range map[string]string{
		"key が無い":    "",
		"値の不在を表す文字列": `logonType="-"`,
	} {
		t.Run(name, func(t *testing.T) {
			fields := parseRecordObservationOK(t, logonRecordWith(item)).Fields
			if hasFieldNamed(fields, "logonTypeCode") {
				t.Error("the record carries a logon type code without a logon type value")
			}
			for _, field := range fields {
				if field.Semantic == core.SemanticKeyEventLogonType {
					t.Errorf("the field %q carries the logon type semantic", field.Name)
				}
			}
		})
	}
}

// assertRawLogonTypeKept は logonType の項目が原資料の文字列をそのまま持つことを確かめる。
func assertRawLogonTypeKept(t *testing.T, fields []core.RecordField, item string) {
	t.Helper()
	raw := fieldByName(t, fields, "logonType")
	if raw.Semantic != "" {
		t.Errorf("the raw logonType field carries the semantic %q", raw.Semantic)
	}
	want := item[len("logonType="):]
	if got, ok := raw.Text.RawTextValue(); !ok || got != want {
		t.Errorf("rawText = %q (%t), want %q", got, ok, want)
	}
}
