// in-package test: 根拠のレコードを持たない割当のエッジの期間を確かめる。
package pipeline

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// localTime は UTC からのずれを持たない地方時の時刻の JSON である。時点として読めない。
func localTime(text string) string {
	return `{"rawText":"` + text + `","normalized":"` + text + `","normalizedForm":"local_without_offset",` +
		`"precision":"second","offsetState":"item_absent","clock":"observer_local","meaning":"event","valueState":"present"}`
}

// absoluteTime は UTC からのずれを持つ時刻の JSON である。時点として読める。
func absoluteTime(text string) string {
	return `{"rawText":"` + text + `","normalized":"` + text + `","normalizedForm":"rfc3339_absolute",` +
		`"precision":"second","offsetState":"in_value","offsetText":"Z","clock":"terminal_local","meaning":"event","valueState":"present"}`
}

func timeRangeOf(t *testing.T, from, to string) core.TimeRange {
	t.Helper()
	var valid core.TimeRange
	if err := json.Unmarshal([]byte(`{"from":`+from+`,"to":`+to+`}`), &valid); err != nil {
		t.Fatal(err)
	}
	return valid
}

// 割当の適用期間からエッジの期間を決める。時点として読めない期間は、1 件だけか、全割当の
// 期間の文字列が同じときだけ出す。読める期間と読めない期間が混ざるときと、読めない期間どうしが
// 違うときは出さない。
func TestAssignmentsRangeOfSeveralAssignments(t *testing.T) {
	localA := timeRangeOf(t, localTime("2001-02-03T04:05:06"), localTime("2001-02-03T05:06:07"))
	localB := timeRangeOf(t, localTime("2001-02-04T04:05:06"), localTime("2001-02-04T05:06:07"))
	early := timeRangeOf(t, absoluteTime("2001-02-03T04:05:06Z"), absoluteTime("2001-02-03T05:00:00Z"))
	late := timeRangeOf(t, absoluteTime("2001-02-03T04:30:00Z"), absoluteTime("2001-02-03T06:00:00Z"))
	joined := core.TimeRange{From: early.From, To: late.To}
	for name, testCase := range map[string]struct {
		ranges []core.TimeRange
		want   *core.TimeRange
	}{
		"割当なし":         {nil, nil},
		"読めない 1 件":     {[]core.TimeRange{localA}, &localA},
		"読める 2 件":      {[]core.TimeRange{early, late}, &joined},
		"読める + 読めない":   {[]core.TimeRange{early, localA}, nil},
		"読めない同じ期間 2 件": {[]core.TimeRange{localA, localA}, &localA},
		"読めない違う期間":     {[]core.TimeRange{localA, localB}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			var assignments []core.TerminalAssignment
			for _, valid := range testCase.ranges {
				assignments = append(assignments, core.TerminalAssignment{AssignmentValidRange: valid})
			}
			got := assignmentsRange(assignments)
			if (got == nil) != (testCase.want == nil) || got != nil && !reflect.DeepEqual(*got, *testCase.want) {
				t.Errorf("the range is %+v, want %+v", got, testCase.want)
			}
		})
	}
}
