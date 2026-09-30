// in-package test: 値の検索と、値ごとの件数の検査を置く。グラフを組む検査は
// graph_test.go にある。
package pipeline

import (
	"net/netip"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 検索は原資料の文字列と正規化値の両方と比べ、一致した欄と値の形を持つ。
func TestQueryReportsTheFieldAndTheFormOfEveryValueMatch(t *testing.T) {
	line := int64(1)
	graph := graphOfRecords(t, []RecordEntry{{
		Locator: core.RecordLocator{
			SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber,
			LineNumber: &line,
		},
		Semantics: &RecordSemantics{
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
			Fields: []core.RecordField{
				syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
				syntheticField(t, "cmd", core.SemanticKeyProcessCommandLine,
					"powershell -enc Get-ChildItem"),
				// 語彙に写していない欄。原資料の key の文字列で名前を持つ。
				syntheticField(t, "profile", "", "Get-ChildItem_profile"),
			},
		},
	}})
	for _, testCase := range []struct {
		name     string
		contains string
		want     []core.NodeValueMatch
	}{
		{
			name: "a token two fields carry", contains: "Get-ChildItem",
			want: []core.NodeValueMatch{
				{
					Semantic: core.SemanticKeyProcessCommandLine,
					Form:     core.ValueMatchFormRawText,
					Value:    "powershell -enc Get-ChildItem",
				},
				{Name: "profile", Form: core.ValueMatchFormRawText, Value: "Get-ChildItem_profile"},
			},
		},
		{
			name:     "a token only the field without a vocabulary item carries",
			contains: "_profile",
			want: []core.NodeValueMatch{
				{Name: "profile", Form: core.ValueMatchFormRawText, Value: "Get-ChildItem_profile"},
			},
		},
		{
			name: "a token the record does not carry", contains: "Get-Process",
		},
		{
			name: "a token that differs in case", contains: "get-CHILDitem",
			want: []core.NodeValueMatch{
				{
					Semantic: core.SemanticKeyProcessCommandLine,
					Form:     core.ValueMatchFormRawText,
					Value:    "powershell -enc Get-ChildItem",
				},
				{Name: "profile", Form: core.ValueMatchFormRawText, Value: "Get-ChildItem_profile"},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			subgraph := graph.Query(GraphQuery{NodeKinds: []core.NodeKind{core.NodeKindRecord}, ValueContains: []string{testCase.contains}, Depth: 0})
			if len(testCase.want) == 0 {
				if subgraph.MatchedNodeCount != 0 {
					t.Fatalf("the query matched %d nodes, want none", subgraph.MatchedNodeCount)
				}
				return
			}
			if len(subgraph.Nodes) != 1 {
				t.Fatalf("the query matched %d nodes, want 1", len(subgraph.Nodes))
			}
			got := subgraph.Nodes[0].ValueMatches
			if !slices.Equal(got, testCase.want) {
				t.Errorf("the node carries the matches %+v, want %+v", got, testCase.want)
			}
		})
	}
	// 含まない文字列も大文字と小文字を区別せずに比べる。
	excluded := graph.Query(GraphQuery{
		NodeKinds: []core.NodeKind{core.NodeKindRecord}, ValueExcludes: []string{"GET-childitem"}, Depth: 0,
	})
	if excluded.MatchedNodeCount != 0 {
		t.Errorf("the query excluding a token in another case kept %d nodes, want none",
			excluded.MatchedNodeCount)
	}
}

// ASCII だけの文字列と、ASCII の外の文字を含む文字列のどちらも、大文字と小文字を区別せずに比べる。
func TestContainsFoldIgnoresTheCaseOfEveryLetter(t *testing.T) {
	for _, testCase := range []struct {
		text, token string
		want        bool
	}{
		{`C:\Windows\SYSTEM32\cmd.exe`, `system32\CMD`, true},
		{`C:\Windows\system32\cmd.exe`, `System32\cmd.exe`, true},
		{"cmd.exe", "cmd.exe ", false},
		{"cmd", "", true},
		{"", "cmd", false},
		{"ÄRGER.txt", "ärger", true},
		{"Straße", "STRASSE", false},
		{"C:\\Users\\ユーザー\\Desktop", "desktop", true},
		{"x\xfey", "\xffy", false},
		{"x\xfey", "\xfey", true},
		{"x\xffÄRGER", "ärger", true},
		{"x\xfey", "�y", false},
	} {
		if got := containsFold(testCase.text, testCase.token); got != testCase.want {
			t.Errorf("containsFold(%q, %q) = %v, want %v", testCase.text, testCase.token, got, testCase.want)
		}
	}
}

// 正規化値を持つ欄は、原資料の文字列と正規化値のそれぞれで一致したことを分けて持つ。
func TestQuerySeparatesTheRawTextMatchFromTheNormalizedMatch(t *testing.T) {
	subgraph := graphOf(t).Query(GraphQuery{NodeKinds: []core.NodeKind{core.NodeKindTerminal}, ValueContains: []string{"PC01"}, Depth: 0})
	if len(subgraph.Nodes) == 0 {
		t.Fatal("the query matched no terminal, want the terminals named PC01")
	}
	// fixture の端末は com="PC01" を原文字列に持ち、正規化値は引用符を外した PC01 である。
	want := []core.NodeValueMatch{
		{Semantic: core.SemanticKeyTerminalHostname, Form: core.ValueMatchFormRawText, Value: `"PC01"`},
		{Semantic: core.SemanticKeyTerminalHostname, Form: core.ValueMatchFormNormalized, Value: "PC01"},
	}
	if !slices.Equal(subgraph.Nodes[0].ValueMatches, want) {
		t.Errorf("the terminal carries the matches %+v, want %+v",
			subgraph.Nodes[0].ValueMatches, want)
	}
}

// valueCountTexts は値ごとの件数を、値と件数の組の文字列にする。
func valueCountTexts(counts []core.ValueCount) []string {
	texts := make([]string, 0, len(counts))
	for _, count := range counts {
		texts = append(texts, count.Value+" "+strconv.FormatInt(count.RecordCount, 10))
	}
	slices.Sort(texts)
	return texts
}

// 値ごとの件数は、絞り込みに合うノードの全件から数える。
func TestQueryCountsValuesAcrossEveryMatchedNode(t *testing.T) {
	graph := graphOf(t)
	// 期待値は fixture の原文の psPath から読んで決める。C:\app.exe は 1 から 5 行目と
	// 7 行目と 9 から 11 行目の 9 レコード、C:\net8.exe は 15 行目と 16 行目の 2 レコード、
	// 他の 4 つは 1 レコードずつである。
	want := []string{
		`C:\app.exe 9`, `C:\app2.exe 1`, `C:\child6.exe 1`, `C:\late.exe 1`,
		`C:\child7.exe 1`, `C:\net8.exe 2`,
	}
	slices.Sort(want)
	whole := graph.Query(GraphQuery{
		CountBySemantic: core.SemanticKeyProcessBinaryPath, Depth: 0,
	})
	if got := valueCountTexts(whole.ValueCounts); !slices.Equal(got, want) {
		t.Fatalf("the query counts %v, want %v", got, want)
	}
	if whole.DistinctValueCount != len(whole.ValueCounts) {
		t.Errorf("the query counted %d distinct values and returned %d",
			whole.DistinctValueCount, len(whole.ValueCounts))
	}
	// **ノードの数の上限を超えても同じ値の集合を返す。** 数える範囲は絞り込みに合うノードであり、
	// 上限が変えるのは図の本体を返すかだけである。
	limited := graph.Query(GraphQuery{
		NodeLimit:       1,
		CountBySemantic: core.SemanticKeyProcessBinaryPath, Depth: 0,
	})
	if got := valueCountTexts(limited.ValueCounts); !limited.NodeLimitExceeded || !slices.Equal(got, want) {
		t.Errorf("the limited query (exceeded=%v) counts %v, want %v", limited.NodeLimitExceeded, got, want)
	}
}

// 値ごとの件数は、語彙の項目と原資料の key のどちらでも数える欄を指せる。
func TestQueryCountsValuesByTheVocabularyAndTheRawKey(t *testing.T) {
	first, second := int64(1), int64(2)
	build := func(position *int64, profile string) RecordEntry {
		return RecordEntry{
			Locator: core.RecordLocator{
				SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber,
				LineNumber: position,
			},
			Semantics: &RecordSemantics{
				ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
				Fields: []core.RecordField{
					syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
					// 同じ意味を 2 つの key が持つ。語彙の項目で数えると 1 つにまとまる。
					syntheticField(t, "ip", core.SemanticKeyTerminalIpAddress, "192.0.2.1"),
					syntheticField(t, "wsIp", core.SemanticKeyTerminalIpAddress, "192.0.2.1"),
					syntheticField(t, "profile", "", profile),
				},
			},
		}
	}
	graph := graphOfRecords(t, []RecordEntry{
		build(&first, "lab_server"), build(&second, "lab_client"),
	})
	for _, testCase := range []struct {
		name  string
		query GraphQuery
		want  []string
	}{
		{
			name:  "a field the vocabulary carries",
			query: GraphQuery{CountBySemantic: core.SemanticKeyTerminalIpAddress},
			want:  []string{"192.0.2.1 2"},
		},
		{
			name:  "a field named by the key of the raw material",
			query: GraphQuery{CountByName: "profile"},
			want:  []string{"lab_client 1", "lab_server 1"},
		},
		{
			name:  "a key no field carries",
			query: GraphQuery{CountByName: "psProfile"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			subgraph := graph.Query(testCase.query)
			got := valueCountTexts(subgraph.ValueCounts)
			want := slices.Clone(testCase.want)
			slices.Sort(want)
			if len(got) != len(want) || (len(want) > 0 && !slices.Equal(got, want)) {
				t.Errorf("the query counts %v, want %v", got, want)
			}
		})
	}
}

// 時刻を読める根拠を持たない値は、時刻の両端を持たない。
func TestQueryLeavesTheTimeBoundsAbsentWithoutAReadableInstant(t *testing.T) {
	line := int64(1)
	graph := graphOfRecords(t, []RecordEntry{{
		// ObservedAt を持たないレコードは、絶対時刻として読める根拠にならない。
		Locator: core.RecordLocator{
			SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber,
			LineNumber: &line,
		},
		Semantics: &RecordSemantics{
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
			Fields: []core.RecordField{
				syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
				syntheticField(t, "com", core.SemanticKeyTerminalHostname, "PC01"),
			},
		},
	}})
	subgraph := graph.Query(GraphQuery{CountBySemantic: core.SemanticKeyTerminalHostname, Depth: 0})
	if len(subgraph.ValueCounts) != 1 {
		t.Fatalf("the query counts %d values, want 1", len(subgraph.ValueCounts))
	}
	counted := subgraph.ValueCounts[0]
	if counted.FirstEventTime != nil || counted.LastEventTime != nil {
		t.Errorf("the value carries the bounds %+v and %+v, want neither",
			counted.FirstEventTime, counted.LastEventTime)
	}
	if counted.RecordCount != 1 {
		t.Errorf("the value was observed by %d records, want 1", counted.RecordCount)
	}
}

// 数える欄を与えない要求は、値ごとの件数を返さない。
func TestQueryCountsNoValueWithoutACountedField(t *testing.T) {
	subgraph := wholeGraphOf(t, graphOf(t))
	if len(subgraph.ValueCounts) != 0 || subgraph.DistinctValueCount != 0 {
		t.Errorf("the subgraph counts %d values and %d distinct values, want none",
			len(subgraph.ValueCounts), subgraph.DistinctValueCount)
	}
}

// 対象の粒度で合う対象が 0 件で、合うレコードが対象を指さないときだけ真を返す。
func TestHasRecordMatchWithoutObjectSeparatesARecordThatPointsToNoObject(t *testing.T) {
	line := int64(1)
	record := func(fields ...core.RecordField) RecordEntry {
		return RecordEntry{
			Locator: core.RecordLocator{
				SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber,
				LineNumber: &line,
			},
			Semantics: &RecordSemantics{
				ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
				Fields:          fields,
			},
		}
	}
	objects := GraphQuery{Granularity: core.GraphGranularityObject}

	withoutObject := graphOfRecords(t, []RecordEntry{
		record(syntheticField(t, "msg", "", "scan finished")),
	})
	if withoutObject.Query(objects).MatchedNodeCount != 0 {
		t.Fatal("the record without an object matched an object node")
	}
	if !withoutObject.HasRecordMatchWithoutObject(objects) {
		t.Error("the record that points to no object is not reported")
	}
	records := objects
	records.Granularity = core.GraphGranularityRecord
	if withoutObject.HasRecordMatchWithoutObject(records) {
		t.Error("the record granularity reports a record without an object")
	}

	withObject := graphOfRecords(t, []RecordEntry{
		record(syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
			syntheticField(t, "ip", core.SemanticKeyTerminalIpAddress, "192.0.2.1")),
	})
	// 種別の絞り込みで対象が残らなかった要求は、対象を指すレコードを持つ。
	accounts := objects
	accounts.NodeKinds = []core.NodeKind{core.NodeKindAccount}
	if withObject.HasRecordMatchWithoutObject(accounts) {
		t.Error("the record that points to an object is reported as pointing to none")
	}

	// 事象の種別で絞る要求は端末を合うノードに数えない。端末だけを指すレコードも真に数える。
	terminalOnly := record(syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
		syntheticField(t, "evt", core.SemanticKeyEventCategory, "svc"))
	onlyTerminal := graphOfRecords(t, []RecordEntry{terminalOnly})
	byKind := objects
	byKind.RecordFilter = RecordFilter{EventCategory: "svc"}
	if onlyTerminal.Query(byKind).MatchedNodeCount != 0 {
		t.Fatal("the query by the event kind matched the terminal")
	}
	if !onlyTerminal.HasRecordMatchWithoutObject(byKind) {
		t.Error("the record that points only to its terminal is not reported")
	}

	// アドレスの範囲で対象が残らなかった要求も、対象を指すレコードを持つ。
	outOfRange := graphOfRecords(t, []RecordEntry{
		record(syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
			syntheticField(t, "evt", core.SemanticKeyEventCategory, "net"),
			syntheticField(t, "dst", core.SemanticKeyConnectionDestinationAddress, "198.51.100.7")),
	})
	prefix := netip.MustParsePrefix("203.0.113.0/24")
	inRange := objects
	inRange.NodeKinds = []core.NodeKind{core.NodeKindIp}
	inRange.AddressInPrefix = &prefix
	inRange.RecordFilter = RecordFilter{EventCategory: "net"}
	if outOfRange.Query(inRange).MatchedNodeCount != 0 {
		t.Fatal("the address outside the range matched")
	}
	if outOfRange.HasRecordMatchWithoutObject(inRange) {
		t.Error("the record that points to an address outside the range is reported as pointing to none")
	}
}

// 検索の文字列を与えない要求は、一致した欄を持たない。
func TestQueryCarriesNoValueMatchWithoutASearchedToken(t *testing.T) {
	subgraph := wholeGraphOf(t, graphOf(t))
	for _, node := range subgraph.Nodes {
		if len(node.ValueMatches) != 0 {
			t.Fatalf("the node %q carries the matches %+v, want none", node.Id, node.ValueMatches)
		}
	}
}

// 値ごとの件数は、絞り込みを通った根拠のレコードだけを数える。
// 期待値は fixture の原文から読んで決める。copy のレコードは 3 行目の 1 件だけである。
func TestQueryCountsOnlyTheEvidenceThatPassesTheFilter(t *testing.T) {
	graph := graphOf(t)
	subgraph := graph.Query(GraphQuery{CountBySemantic: core.SemanticKeyProcessBinaryPath, Depth: 0, RecordFilter: RecordFilter{EventCategory: "file", EventAction: "copy"}})
	// 3 行目のプロセスは {P1} で psPath は C:\app.exe である。
	want := []string{`C:\app.exe 1`}
	if got := valueCountTexts(subgraph.ValueCounts); !slices.Equal(got, want) {
		t.Fatalf("the query counts %v, want %v", got, want)
	}
	if subgraph.DistinctValueCount != len(want) {
		t.Errorf("the query counted %d distinct values, want %d",
			subgraph.DistinctValueCount, len(want))
	}
	// 数えた根拠は copy のレコード 1 件だけである。値ごとの件数は根拠の中身を含まない
	// ため、時刻の両端が同じ 1 件を指すことで確かめる。
	counted := subgraph.ValueCounts[0]
	if counted.RecordCount != 1 {
		t.Fatalf("the value counts %d records, want 1", counted.RecordCount)
	}
	if counted.FirstEventTime == nil || counted.LastEventTime == nil {
		t.Fatalf("the value carries the bounds %v and %v, want both to be present",
			counted.FirstEventTime, counted.LastEventTime)
	}
	first, firstKnown := counted.FirstEventTime.Instant()
	last, lastKnown := counted.LastEventTime.Instant()
	if !firstKnown || !lastKnown || !first.Equal(last) {
		t.Errorf("the value spans %v to %v, want a single record at one time", first, last)
	}
}

// 期間を与えた要求は、期間の外のレコードを時刻の両端に出さない。
func TestQueryKeepsTheTimeBoundsInsideTheRequestedPeriod(t *testing.T) {
	from := time.Date(2000, 2, 1, 13, 0, 2, 0, time.FixedZone("JST", 9*60*60))
	to := time.Date(2000, 2, 1, 13, 0, 4, 0, time.FixedZone("JST", 9*60*60))
	subgraph := graphOf(t).Query(GraphQuery{CountBySemantic: core.SemanticKeyProcessBinaryPath, Depth: 0, RecordFilter: RecordFilter{TimeFrom: &from, TimeTo: &to, TimeUnit: time.Second}})
	if len(subgraph.ValueCounts) == 0 {
		t.Fatal("the query counts no value, want the process of the period")
	}
	for _, counted := range subgraph.ValueCounts {
		for _, bound := range []*core.Timestamp{
			counted.FirstEventTime, counted.LastEventTime,
		} {
			if bound == nil {
				t.Fatalf("the value %q carries no time bound", counted.Value)
			}
			instant, readable := bound.Instant()
			if !readable {
				t.Fatalf("the value %q carries an unreadable bound", counted.Value)
			}
			if instant.Before(from) || instant.After(to) {
				t.Errorf("the value %q carries the bound %s, want it inside the period",
					counted.Value, instant)
			}
		}
	}
}

// 値の不在で数から外したレコードがあるとき、値ごとの件数の和と、値の不在のレコードの件数の
// 和が、絞り込みに合うレコードの件数と揃う。
func TestQueryCountsEveryRecordAsAValueOrAnAbsentValue(t *testing.T) {
	present, absent := int64(1), int64(2)
	build := func(position *int64, state core.ValueState, text string) RecordEntry {
		value, err := core.NewRawValue(state, text)
		if err != nil {
			t.Fatal(err)
		}
		field, err := core.NewTextField("agent", core.SemanticKeyHttpUserAgent, value)
		if err != nil {
			t.Fatal(err)
		}
		return RecordEntry{
			Locator: core.RecordLocator{
				SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber,
				LineNumber: position,
			},
			Semantics: &RecordSemantics{
				ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
				Fields: []core.RecordField{
					syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
					field,
				},
			},
		}
	}
	graph := graphOfRecords(t, []RecordEntry{
		build(&present, core.ValueStatePresent, "ExampleClient/1.0"),
		build(&absent, core.ValueStateAbsent, "-"),
	})
	subgraph := graph.Query(GraphQuery{CountBySemantic: core.SemanticKeyHttpUserAgent, NodeKinds: []core.NodeKind{core.NodeKindRecord}, Depth: 0})
	counted := int64(0)
	for _, value := range subgraph.ValueCounts {
		counted += value.RecordCount
	}
	// レコードは 2 件で、値を読めるのは 1 件である。値ごとの件数の和に、値の不在の
	// 1 件を足すと、絞り込みに合うレコードのノードの件数と揃う。
	const absentValueRecords = 1
	if counted+absentValueRecords != int64(subgraph.MatchedNodeCount) {
		t.Errorf("the counted %d records and the %d records without a value do not add up to "+
			"the %d matched records", counted, absentValueRecords, subgraph.MatchedNodeCount)
	}
	if subgraph.DistinctValueCount != 1 {
		t.Errorf("the query counted %d distinct values, want 1", subgraph.DistinctValueCount)
	}
}

// 1 レコードが同じ欄へ 2 つの異なる値を持つと、そのレコードを 2 つの値が数える。
// **値ごとの件数の和は、合致したレコードの件数より大きくなる。** 差から値の不在の件数を
// 求められないことの反例である (valueCounts の既知の制限)。
func TestQueryCountsARecordOnceForEachValueOfTheSameField(t *testing.T) {
	line := int64(1)
	graph := graphOfRecords(t, []RecordEntry{{
		Locator: core.RecordLocator{
			SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber,
			LineNumber: &line,
		},
		Semantics: &RecordSemantics{
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
			Fields: []core.RecordField{
				syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
				// markii 形式の 1 レコードは端末の IPv4 と IPv6 を 1 つずつ持つ。
				syntheticField(t, "ip", core.SemanticKeyTerminalIpAddress, "192.0.2.1"),
				syntheticField(t, "ip6", core.SemanticKeyTerminalIpAddress, "2001:db8::1"),
			},
		},
	}})
	subgraph := graph.Query(GraphQuery{CountBySemantic: core.SemanticKeyTerminalIpAddress, NodeKinds: []core.NodeKind{core.NodeKindRecord},
		Depth: 0,
	})
	want := []string{"192.0.2.1 1", "2001:db8::1 1"}
	if got := valueCountTexts(subgraph.ValueCounts); !slices.Equal(got, want) {
		t.Fatalf("the query counts %v, want %v", got, want)
	}
	counted := int64(0)
	for _, value := range subgraph.ValueCounts {
		counted += value.RecordCount
	}
	// 合致したレコードのノードは 1 件で、件数の和は 2 である。
	if counted <= int64(subgraph.MatchedNodeCount) {
		t.Errorf("the counted %d records do not exceed the %d matched records",
			counted, subgraph.MatchedNodeCount)
	}
}

// 他の絞り込みを外した走査は、文字列がどこにも無いことと、絞り込みで残らなかったことを分ける。
func TestHasValueMatchAnywhereSeparatesTheAbsentTokenFromTheFilteredOne(t *testing.T) {
	graph := graphOf(t)
	for _, testCase := range []struct {
		name     string
		contains []string
		want     bool
	}{
		{"a token the fixture carries", []string{"secret.txt"}, true},
		{"a token no field carries", []string{"no-such-token-in-the-fixture"}, false},
		{"tokens no single node carries together",
			[]string{"secret.txt", "no-such-token-in-the-fixture"}, false},
		{"no token", nil, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := graph.HasValueMatchAnywhere(GraphQuery{ValueContains: testCase.contains}); got != testCase.want {
				t.Errorf("the graph carries the token: %v, want %v", got, testCase.want)
			}
		})
	}
}

// 他の絞り込みを外した走査は、数える欄の名前が無いことと、絞り込みで残らなかったことを
// 分ける。
func TestCountedFieldObservationSeparatesTheUnknownFieldFromTheFilteredOne(t *testing.T) {
	graph := graphOf(t)
	for _, testCase := range []struct {
		name  string
		query GraphQuery
		want  bool
	}{
		{
			"a field the fixture carries",
			GraphQuery{CountBySemantic: core.SemanticKeyProcessBinaryPath}, true,
		},
		{
			"a vocabulary item no field carries",
			GraphQuery{CountBySemantic: core.SemanticKeyProcessWindowTitle}, false,
		},
		{"a key no field carries", GraphQuery{CountByName: "psProfile"}, false},
		{"no counted field", GraphQuery{}, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := len(graph.CountedFieldObservationOf(testCase.query).NodeKinds) > 0; got != testCase.want {
				t.Errorf("the graph carries the field: %v, want %v", got, testCase.want)
			}
		})
	}
}

// 値が時刻である欄は、正規化値を持つ側と持たない側の両方が属性になり、検索に一致する。
func TestQueryCarriesTimestampFieldsAsAttributes(t *testing.T) {
	line := int64(1)
	// 時刻は固定値を使う。現在時刻を材料にしない。
	rawWithOffset, normalizedText := "02/01/2000 13:00:00.000 +0900", "2000-02-01T13:00:00.000+09:00"
	offsetText := "+0900"
	normalized, err := core.NewTimestamp(core.Timestamp{
		RawText: &rawWithOffset, Normalized: &normalizedText,
		NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Precision:      core.PrecisionMillisecond, OffsetState: core.OffsetStateInValue,
		OffsetText: &offsetText,
		Clock:      core.ClockTerminalLocal, Meaning: core.MeaningEvent,
		ValueState: core.ValueStatePresent,
	})
	if err != nil {
		t.Fatal(err)
	}
	// 値の不在の時刻は、原資料の文字列も正規化値も持たない (Timestamp.validateAbsence)。
	absent, err := core.NewTimestamp(core.Timestamp{
		Precision: core.PrecisionMillisecond, OffsetState: core.OffsetStateItemAbsent,
		Clock: core.ClockTerminalLocal, Meaning: core.MeaningOperationStart,
		ValueState: core.ValueStateItemAbsent,
	})
	if err != nil {
		t.Fatal(err)
	}
	withNormalized, err := core.NewTimestampField(
		"evtTime", core.SemanticKeyEventTime, normalized)
	if err != nil {
		t.Fatal(err)
	}
	withoutValue, err := core.NewTimestampField(
		"opTime", core.SemanticKeyEventOperationStartTime, absent)
	if err != nil {
		t.Fatal(err)
	}
	graph := graphOfRecords(t, []RecordEntry{{
		Locator: core.RecordLocator{
			SourceFileName: "synthetic.log", PositionKind: core.PositionKindLineNumber,
			LineNumber: &line,
		},
		Semantics: &RecordSemantics{
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
			Fields: []core.RecordField{
				syntheticField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
				withNormalized, withoutValue,
			},
		},
	}})
	for _, testCase := range []struct {
		name     string
		contains string
		want     []core.NodeValueMatch
	}{
		{
			// 正規化値を持つ欄は、原資料の文字列と正規化値の両方に一致する。
			name: "a timestamp carrying a normalized value", contains: "13:00:00",
			want: []core.NodeValueMatch{
				{Semantic: core.SemanticKeyEventTime, Form: core.ValueMatchFormRawText, Value: "02/01/2000 13:00:00.000 +0900"},
				{Semantic: core.SemanticKeyEventTime, Form: core.ValueMatchFormNormalized, Value: "2000-02-01T13:00:00.000+09:00"},
			},
		},
		{
			// 値の不在の時刻は属性にならず、どの文字列にも一致しない。
			name: "a timestamp without a value", contains: "13:00:01",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			subgraph := graph.Query(GraphQuery{NodeKinds: []core.NodeKind{core.NodeKindRecord}, ValueContains: []string{testCase.contains}, Depth: 0})
			if len(testCase.want) == 0 {
				if subgraph.MatchedNodeCount != 0 {
					t.Fatalf("the query matched %d nodes, want none", subgraph.MatchedNodeCount)
				}
				return
			}
			if len(subgraph.Nodes) != 1 {
				t.Fatalf("the query matched %d nodes, want 1", len(subgraph.Nodes))
			}
			if got := subgraph.Nodes[0].ValueMatches; !slices.Equal(got, testCase.want) {
				t.Errorf("the node carries the matches %+v, want %+v", got, testCase.want)
			}
		})
	}
}
