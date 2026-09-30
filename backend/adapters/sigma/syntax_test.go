package sigma_test

import (
	"testing"
	"testing/fstest"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/sigma"
)

// securityRule は service: security のルールの file を返す。
func securityRule(detection string) string {
	return "title: t\nlogsource:\n  product: windows\n  service: security\ndetection:\n" + detection
}

// flags は a、b、c の 3 つの検索を立てるレコードを返す。立てた検索は項目の値が "1" である。
func flags(a, b, c bool) event {
	record := event{sigma.FieldChannel: "Security", sigma.FieldEventID: "4624"}
	for name, set := range map[string]bool{"A": a, "B": b, "C": c} {
		if set {
			record[name] = "1"
		} else {
			record[name] = "0"
		}
	}
	return record
}

func TestConditionPrecedence(t *testing.T) {
	selections := "  a:\n    A: '1'\n  b:\n    B: '1'\n  c:\n    C: '1'\n  _x:\n    A: '0'\n"
	cases := []struct {
		condition string
		record    event
		want      bool
	}{
		// and は or より強く結ぶ。
		{"a or b and c", flags(true, false, false), true},
		{"a or b and c", flags(false, true, false), false},
		{"(a or b) and c", flags(true, false, false), false},
		{"(a or b) and c", flags(true, false, true), true},
		// not は直後の operand だけに掛かる。
		{"not a and b", flags(false, true, false), true},
		{"not a and b", flags(true, true, false), false},
		{"not (a and b)", flags(true, false, false), true},
		// them は `_` で始まる検索を除く。_x は A が "0" のとき立つ。
		{"all of them", flags(true, true, true), true},
		{"1 of them", flags(false, false, false), false},
		{"all of ?", flags(true, true, false), false},
		{"all of ?", flags(true, true, true), true},
		{"1 of _*", flags(false, false, false), true},
	}
	for _, c := range cases {
		set := loadRules(t, map[string]string{"rule.yml": securityRule(selections + "  condition: " + c.condition + "\n")})
		if len(set.Unevaluated) != 0 {
			t.Fatalf("%s: Unevaluated = %+v", c.condition, set.Unevaluated)
		}
		if got := len(matchedPaths(set, c.record)) == 1; got != c.want {
			t.Errorf("%q on %v: matched %v, want %v", c.condition, c.record, got, c.want)
		}
	}
}

func TestValueSyntax(t *testing.T) {
	cases := []struct {
		name, item, value string
		want              bool
	}{
		{"escaped backslash before a wildcard", `V: 'C:\\*'`, `C:\dir`, true},
		{"escaped backslash is one backslash", `V: 'C:\\*'`, `C:dir`, false},
		{"windash keeps a dash inside a word", `V|windash|contains: 'a-b'`, `xa/by`, false},
		{"windash keeps a dash inside a word literally", `V|windash|contains: 'a-b'`, `xa-by`, true},
		{"windash replaces a dash at a word start", `V|windash: '-x'`, `/x`, true},
		{"re with i", `V|re|i: '^abc$'`, `ABC`, true},
		{"re without i is case sensitive", `V|re: '^abc$'`, `ABC`, false},
		{"all of the values", `V|contains|all: ['a', 'b']`, `ba`, true},
		{"all of the values needs each", `V|contains|all: ['a', 'b']`, `aa`, false},
	}
	for _, c := range cases {
		set := loadRules(t, map[string]string{"rule.yml": securityRule("  sel:\n    " + c.item + "\n  condition: sel\n")})
		if len(set.Unevaluated) != 0 {
			t.Fatalf("%s: Unevaluated = %+v", c.name, set.Unevaluated)
		}
		record := event{sigma.FieldChannel: "Security", sigma.FieldEventID: "1", "V": c.value}
		if got := len(matchedPaths(set, record)) == 1; got != c.want {
			t.Errorf("%s: %q on %q matched %v, want %v", c.name, c.item, c.value, got, c.want)
		}
	}
}

func TestExistsFalseNeedsAbsence(t *testing.T) {
	set := loadRules(t, map[string]string{"rule.yml": securityRule("  sel:\n    V|exists: false\n  condition: sel\n")})
	present := event{sigma.FieldChannel: "Security", sigma.FieldEventID: "1", "V": ""}
	absent := event{sigma.FieldChannel: "Security", sigma.FieldEventID: "1"}
	if len(matchedPaths(set, present)) != 0 || len(matchedPaths(set, absent)) != 1 {
		t.Fatalf("present %v, absent %v", matchedPaths(set, present), matchedPaths(set, absent))
	}
}

func TestLoadRejectsModifierCombinations(t *testing.T) {
	for _, item := range []string{
		"V|re|contains: 'x'", "V|cidr|startswith: '192.0.2.0/24'", "V|exists|re: 'x'",
		"V|contains|endswith: 'x'", "V|i: 'x'",
	} {
		set, err := sigma.Load(fstest.MapFS{"rule.yml": {Data: []byte(securityRule("  sel:\n    " + item + "\n  condition: sel\n"))}})
		if err != nil {
			t.Fatal(err)
		}
		if len(set.Unevaluated) != 1 || set.Unevaluated[0].Reason != sigma.ReasonModifierUnsupported {
			t.Errorf("%s: Unevaluated = %+v, want modifier_unsupported", item, set.Unevaluated)
		}
	}
}
