// in-package test: 検索式の解析と、グラフと時系列への適用を確かめる。
package pipeline

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 構文の誤りは、理由と、式の先頭から数えた符号位置の範囲で返る。
func TestParseSearchExpressionLocatesTheSyntaxError(t *testing.T) {
	for _, testCase := range []struct {
		text   string
		reason core.SearchExpressionErrorReason
		offset int
		length int
	}{
		{"", core.SearchExpressionErrorReasonEmptyExpression, 0, 0},
		{"   ", core.SearchExpressionErrorReasonEmptyExpression, 0, 3},
		{"a = b", core.SearchExpressionErrorReasonUnexpectedCharacter, 2, 1},
		{"a & b", core.SearchExpressionErrorReasonUnexpectedCharacter, 2, 1},
		{"a | b", core.SearchExpressionErrorReasonUnexpectedCharacter, 2, 1},
		{`"abc`, core.SearchExpressionErrorReasonUnterminatedString, 0, 4},
		{`"ab\`, core.SearchExpressionErrorReasonUnterminatedString, 0, 4},
		{`x == "a\nb"`, core.SearchExpressionErrorReasonInvalidEscape, 7, 2},
		{"== x", core.SearchExpressionErrorReasonMissingField, 0, 2},
		{"a == b == c", core.SearchExpressionErrorReasonMissingField, 7, 2},
		{"a and contains b", core.SearchExpressionErrorReasonMissingOperand, 2, 3},
		{"a ==", core.SearchExpressionErrorReasonMissingValue, 2, 2},
		{"a == )", core.SearchExpressionErrorReasonMissingValue, 2, 2},
		{"a == and", core.SearchExpressionErrorReasonMissingValue, 2, 2},
		{"a and", core.SearchExpressionErrorReasonMissingOperand, 2, 3},
		{"or a", core.SearchExpressionErrorReasonMissingOperand, 0, 2},
		{"not", core.SearchExpressionErrorReasonMissingOperand, 0, 3},
		{"a || )", core.SearchExpressionErrorReasonMissingOperand, 2, 2},
		{"()", core.SearchExpressionErrorReasonMissingOperand, 1, 1},
		{"(a", core.SearchExpressionErrorReasonUnclosedParenthesis, 0, 1},
		{"(", core.SearchExpressionErrorReasonUnclosedParenthesis, 0, 1},
		{"a)", core.SearchExpressionErrorReasonUnmatchedParenthesis, 1, 1},
		{"hits > many", core.SearchExpressionErrorReasonValueNotOrdered, 7, 4},
		// 日付だけの文字列は、UTC からのずれを持つ時刻ではない。
		{"at < 2031-04-05", core.SearchExpressionErrorReasonValueNotOrdered, 5, 10},
		// 位置はbyte ではなく符号位置で数える。
		{"ユーザー = x", core.SearchExpressionErrorReasonUnexpectedCharacter, 5, 1},
		{strings.Repeat("a", maxSearchExpressionLength+1),
			core.SearchExpressionErrorReasonTooLong, maxSearchExpressionLength, 1},
		// 上限の byte の位置が 3 byte の文字の途中に該当するときは、上限に収まる文字の後から数える。
		{strings.Repeat("あ", maxSearchExpressionLength/3+1),
			core.SearchExpressionErrorReasonTooLong, maxSearchExpressionLength / 3, 1},
		{strings.Repeat("a ", maxSearchExpressionTerms) + "b",
			core.SearchExpressionErrorReasonTooManyTerms, 2 * maxSearchExpressionTerms, 1},
		{strings.Repeat("(", maxSearchExpressionDepth+1) + "a" + strings.Repeat(")", maxSearchExpressionDepth+1),
			core.SearchExpressionErrorReasonNestingTooDeep, maxSearchExpressionDepth, 1},
		{strings.Repeat("not ", maxSearchExpressionDepth+1) + "a",
			core.SearchExpressionErrorReasonNestingTooDeep, 4 * maxSearchExpressionDepth, 3},
	} {
		t.Run(testCase.text, func(t *testing.T) {
			_, err := ParseSearchExpression(testCase.text)
			var syntax *SearchExpressionSyntaxError
			if !errors.As(err, &syntax) {
				t.Fatalf("ParseSearchExpression returned %v, want a syntax error", err)
			}
			want := core.SearchExpressionError{
				Reason: testCase.reason, Offset: testCase.offset, Length: testCase.length,
			}
			if syntax.Detail != want {
				t.Errorf("the syntax error is %+v, want %+v", syntax.Detail, want)
			}
			if err := syntax.Detail.Validate(); err != nil {
				t.Errorf("the syntax error does not validate: %v", err)
			}
			if strings.Contains(syntax.Error(), testCase.text) && testCase.text != "" {
				t.Errorf("the error message %q echoes the expression", syntax.Error())
			}
		})
	}
}

// 上限ちょうどの式は読める。
func TestParseSearchExpressionAcceptsTheExpressionAtTheLimits(t *testing.T) {
	for _, text := range []string{
		strings.Repeat("a", maxSearchExpressionLength),
		strings.TrimSpace(strings.Repeat("a ", maxSearchExpressionTerms)),
		strings.Repeat("(", maxSearchExpressionDepth) + "a" + strings.Repeat(")", maxSearchExpressionDepth),
	} {
		if _, err := ParseSearchExpression(text); err != nil {
			t.Errorf("ParseSearchExpression returned %v for an expression at the limit", err)
		}
	}
}

// expressionGraph は検索式の検査に使うレコードを組む。
//
// 行 1 と行 2 は acct と hits と evtTime を持ち、行 3 は acct を持たず、hits が数でなく、evtTime が
// UTC からのずれの決まらない地方時である。
func expressionGraph(t *testing.T) Graph {
	t.Helper()
	record := func(line int64, fields ...core.RecordField) RecordEntry {
		return RecordEntry{
			Locator: core.RecordLocator{
				SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber,
				LineNumber: &line,
			},
			ObservedAt: timelineTimestamp(t, "2031-04-05T06:07:0"+string(rune('0'+line))+"Z",
				core.NormalizedFormRFC3339Absolute, core.OffsetStateInValue),
			Semantics: &RecordSemantics{
				ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
				Fields:          fields,
			},
		}
	}
	timeField := func(normalized string, form core.NormalizedForm, offset core.OffsetState) core.RecordField {
		field, err := core.NewTimestampField("evtTime", core.SemanticKeyEventTime,
			*timelineTimestamp(t, normalized, form, offset))
		if err != nil {
			t.Fatal(err)
		}
		return field
	}
	return graphOfRecords(t, []RecordEntry{
		record(1,
			syntheticField(t, "cmd", core.SemanticKeyProcessCommandLine, "powershell -enc AAA"),
			syntheticField(t, "acct", "", "alice"),
			syntheticField(t, "hits", "", "15"),
			syntheticField(t, "EventData.TargetUser", "", "alice"),
			timeField("2031-04-05T06:07:01Z", core.NormalizedFormRFC3339Absolute, core.OffsetStateInValue)),
		record(2,
			syntheticField(t, "cmd", core.SemanticKeyProcessCommandLine, "cmd.exe /c whoami"),
			syntheticField(t, "acct", "", "Bob"),
			syntheticField(t, "hits", "", "7"),
			timeField("2031-04-05T06:08:00Z", core.NormalizedFormRFC3339Absolute, core.OffsetStateInValue)),
		record(3,
			syntheticField(t, "cmd", core.SemanticKeyProcessCommandLine, "notepad.exe"),
			syntheticField(t, "hits", "", "many"),
			timeField("2031-04-05T06:09:00", core.NormalizedFormLocalWithoutOffset, core.OffsetStateUndetermined)),
	})
}

// matchedLines は、式に合うレコードのノードの行番号を並べる。
func matchedLines(t *testing.T, graph Graph, text string) []int64 {
	t.Helper()
	expression, err := ParseSearchExpression(text)
	if err != nil {
		t.Fatalf("ParseSearchExpression(%q) returned %v", text, err)
	}
	subgraph := graph.Query(GraphQuery{
		NodeKinds: []core.NodeKind{core.NodeKindRecord}, Expression: expression, RecordSummary: true,
	})
	var lines []int64
	for _, node := range subgraph.Nodes {
		lines = append(lines, *node.Record.RecordRef.LineNumber)
	}
	slices.Sort(lines)
	return lines
}

// 比較・論理・括弧・欄を持たない文字列を、レコード 1 件の欄で判定する。
func TestQueryKeepsTheRecordsThatSatisfyTheExpression(t *testing.T) {
	graph := expressionGraph(t)
	for _, testCase := range []struct {
		text string
		want []int64
	}{
		{"whoami", []int64{2}},
		{"process.command_line contains POWERSHELL", []int64{1}},
		{"cmd contains POWERSHELL", []int64{1}},
		{"acct == ALICE", []int64{1}},
		{"acct == ali", nil},
		// 欄を持たない行 3 は != に合わず、not に合う。
		{"acct != alice", []int64{2}},
		{"not acct == alice", []int64{2, 3}},
		{"! acct == alice", []int64{2, 3}},
		// 原資料の key は、`.` で区切った末尾の部分でも指せる。
		{"TargetUser == alice", []int64{1}},
		{"hits > 10", []int64{1}},
		{"hits > 7.0", []int64{1}},
		{"hits <= 7", []int64{2}},
		{"hits >= -1", []int64{1, 2}},
		{"hits < 99999999999999999999999", []int64{1, 2}},
		// 地方時の行 3 は時点を持たないため、時点との比較に合わない。
		{"evtTime > 2031-04-05T06:07:30Z", []int64{2}},
		{"evtTime <= 2031-04-05T15:07:01+09:00", []int64{1}},
		{"(acct == alice or acct == bob) and hits > 10", []int64{1}},
		{"acct == alice || whoami", []int64{1, 2}},
		{"notepad OR whoami", []int64{2, 3}},
		{"powershell -enc", []int64{1}},
		{"powershell && -enc", []int64{1}},
		{`"-enc AAA"`, []int64{1}},
		{`"-enc   AAA"`, nil},
		{`"acct" == "alice"`, []int64{1}},
		{"not (acct == alice or acct == bob)", []int64{3}},
		{"not not acct == alice", []int64{1}},
	} {
		t.Run(testCase.text, func(t *testing.T) {
			if got := matchedLines(t, graph, testCase.text); !slices.Equal(got, testCase.want) {
				t.Errorf("the expression matched the lines %v, want %v", got, testCase.want)
			}
		})
	}
}

// 一致した欄は、否定の下に無い条件と、!= 以外の比較だけが持つ。
func TestQueryReportsTheFieldsThePositiveConditionsMatched(t *testing.T) {
	graph := expressionGraph(t)
	expression, err := ParseSearchExpression("acct == alice and not hits > 100 and acct != bob and enc")
	if err != nil {
		t.Fatal(err)
	}
	subgraph := graph.Query(GraphQuery{NodeKinds: []core.NodeKind{core.NodeKindRecord}, Expression: expression})
	if len(subgraph.Nodes) != 1 {
		t.Fatalf("the expression matched %d records, want 1", len(subgraph.Nodes))
	}
	want := []core.NodeValueMatch{
		{Name: "acct", Form: core.ValueMatchFormRawText, Value: "alice"},
		{Semantic: core.SemanticKeyProcessCommandLine, Form: core.ValueMatchFormRawText, Value: "powershell -enc AAA"},
	}
	if got := subgraph.Nodes[0].ValueMatches; !slices.Equal(got, want) {
		t.Errorf("the record carries the matches %+v, want %+v", got, want)
	}
	// 時点との比較は、時刻の正規化値と比べる。
	timed, err := ParseSearchExpression("evtTime < 2031-04-05T06:07:30Z")
	if err != nil {
		t.Fatal(err)
	}
	timedGraph := graph.Query(GraphQuery{NodeKinds: []core.NodeKind{core.NodeKindRecord}, Expression: timed})
	wantTimed := []core.NodeValueMatch{{
		Semantic: core.SemanticKeyEventTime, Form: core.ValueMatchFormNormalized, Value: "2031-04-05T06:07:01Z",
	}}
	if len(timedGraph.Nodes) != 1 || !slices.Equal(timedGraph.Nodes[0].ValueMatches, wantTimed) {
		t.Errorf("the time comparison matched %+v, want one record carrying %+v", timedGraph.Nodes, wantTimed)
	}
}

// 0 件の理由を分ける走査も式で判定する。
func TestHasValueMatchAnywhereEvaluatesTheExpression(t *testing.T) {
	graph := expressionGraph(t)
	for _, testCase := range []struct {
		text string
		want bool
	}{
		{"acct == alice", true},
		{"acct == carol", false},
	} {
		expression, err := ParseSearchExpression(testCase.text)
		if err != nil {
			t.Fatal(err)
		}
		if got := graph.HasValueMatchAnywhere(GraphQuery{Expression: expression}); got != testCase.want {
			t.Errorf("HasValueMatchAnywhere(%q) = %v, want %v", testCase.text, got, testCase.want)
		}
	}
}

// 起点だけに条件を当てる要求は、広げる段階の判定から式を外す。
func TestStepQueryDropsTheExpression(t *testing.T) {
	expression, err := ParseSearchExpression("acct == alice")
	if err != nil {
		t.Fatal(err)
	}
	origins := GraphQuery{Expression: expression, ConditionsOnOriginsOnly: true}
	if origins.stepQuery().Expression != nil {
		t.Error("the step of a query on the origins only kept the expression")
	}
	every := GraphQuery{Expression: expression}
	if every.stepQuery().Expression != expression {
		t.Error("the step of a query on every step dropped the expression")
	}
}

// 時系列は式を満たすレコードだけを並べる。
func TestTimelineKeepsTheRecordsThatSatisfyTheExpression(t *testing.T) {
	graph := expressionGraph(t)
	for _, testCase := range []struct {
		text string
		want []int64
	}{
		{"acct == alice", []int64{1}},
		{"not acct == alice", []int64{2, 3}},
		{"acct == carol", nil},
	} {
		t.Run(testCase.text, func(t *testing.T) {
			expression, err := ParseSearchExpression(testCase.text)
			if err != nil {
				t.Fatal(err)
			}
			timeline := graph.Timeline(TimelineQuery{TextSearch: GraphQuery{Expression: expression}})
			var lines []int64
			for _, entry := range timeline.Entries {
				lines = append(lines, *entry.RecordRef.LineNumber)
			}
			slices.Sort(lines)
			if !slices.Equal(lines, testCase.want) {
				t.Errorf("the timeline listed the lines %v, want %v", lines, testCase.want)
			}
			if timeline.UndatedRecordCount != 0 {
				t.Errorf("the timeline counted %d undated records, want none", timeline.UndatedRecordCount)
			}
			var counted int64
			for _, coverage := range timeline.SourceCoverages {
				counted += coverage.MatchedRecordCount
			}
			if counted != int64(len(timeline.Entries)) {
				t.Errorf("the coverages count %d records, want the %d listed", counted, len(timeline.Entries))
			}
		})
	}
	// 式を与えない要求は、どのレコードも並べる。
	if got, want := len(graph.Timeline(TimelineQuery{}).Entries), len(graph.records); got != want {
		t.Errorf("the timeline without an expression listed %d records, want the %d records", got, want)
	}
}

// 10 進の数を、丸めずに桁で比べる。
func TestDecimalComparesTheDigits(t *testing.T) {
	for _, testCase := range []struct {
		left, right string
		want        int
	}{
		{"15", "7", 1},
		{"7", "7.0", 0},
		{"-0", "0", 0},
		{"+3", "3", 0},
		{"0.25", "0.5", -1},
		{"-2", "-10", 1},
		{"-2", "1", -1},
		{"18446744073709551615", "18446744073709551614", 1},
		{"0012", "12.000", 0},
	} {
		left, leftRead := parseDecimal(testCase.left)
		right, rightRead := parseDecimal(testCase.right)
		if !leftRead || !rightRead {
			t.Fatalf("parseDecimal could not read %q or %q", testCase.left, testCase.right)
		}
		if got := left.compare(right); got != testCase.want {
			t.Errorf("compare(%q, %q) = %d, want %d", testCase.left, testCase.right, got, testCase.want)
		}
	}
	for _, text := range []string{"", "-", "1.", ".5", "1e3", "0x10", "1_000", "NaN", "Inf", " 1"} {
		if _, readable := parseDecimal(text); readable {
			t.Errorf("parseDecimal read %q as a decimal", text)
		}
	}
}
