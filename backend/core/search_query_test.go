package core_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func uint64Of(value uint64) *uint64 { return &value }

// 検索の条件の規則ごとに、通る値と退ける値の両側を確かめる。
func TestSearchQueryValidate(t *testing.T) {
	longToken := strings.Repeat("a", core.MaxSearchTokenLength)
	tokens := make([]string, core.MaxSearchTokens)
	for index := range tokens {
		tokens[index] = "t"
	}
	nodeIds := make([]string, core.MaxSearchOriginNodes)
	for index := range nodeIds {
		nodeIds[index] = "node"
	}
	period := core.RecordConditions{
		TimeFrom: "2026-01-02T03:04:05Z", TimeFromPrecision: core.PrecisionSecond,
		TimeTo: "2026-01-02T03:04:05.678+09:00", TimeToPrecision: core.PrecisionMillisecond,
		FilterUnit: core.FilterUnitSecond,
	}
	cases := []struct {
		name    string
		query   core.SearchQuery
		wantErr string
	}{
		{"an empty query", core.SearchQuery{}, ""},
		{"the deepest depth", core.SearchQuery{Depth: core.MaxSearchDepth}, ""},
		{"a depth beyond the limit", core.SearchQuery{Depth: core.MaxSearchDepth + 1}, "depth must be between"},
		{"a negative depth", core.SearchQuery{Depth: core.MinSearchDepth - 1}, "depth must be between"},
		{"a field with a term", core.SearchQuery{ValueField: "f", ValueContains: []string{"x"}}, ""},
		{"a field without a term", core.SearchQuery{ValueField: "f"}, "valueField must name a field"},
		{"tokens at the limit", core.SearchQuery{ValueContains: tokens}, ""},
		{"tokens beyond the limit", core.SearchQuery{ValueContains: tokens, ValueExcludes: []string{"u"}},
			"must carry at most"},
		{"a token at the length limit", core.SearchQuery{ValueExcludes: []string{longToken}}, ""},
		{"a token beyond the length limit", core.SearchQuery{ValueExcludes: []string{longToken + "a"}},
			"a search token must be at most"},
		{"an empty token", core.SearchQuery{ValueContains: []string{""}}, "valueContains must not be empty"},
		{"field pairs", core.SearchQuery{FieldContains: []string{"f=x"}, FieldEquals: []string{"g=y=z"}}, ""},
		{"a field pair without =", core.SearchQuery{FieldContains: []string{"fx"}},
			"fieldContains must carry a field and a search term"},
		{"a whole-value pair without a term", core.SearchQuery{FieldEquals: []string{"f="}},
			"fieldEquals must carry a field and a search term"},
		{"pairs beyond the token limit", core.SearchQuery{ValueContains: tokens, FieldEquals: []string{"f=x"}},
			"must carry at most"},
		{"sources", core.SearchQuery{RecordConditions: core.RecordConditions{Sources: []string{"src:1"}}}, ""},
		{"an empty source", core.SearchQuery{RecordConditions: core.RecordConditions{Sources: []string{""}}},
			"source must not be empty"},
		{"origins at the limit", core.SearchQuery{NodeIds: nodeIds}, ""},
		{"origins beyond the limit", core.SearchQuery{NodeIds: append(nodeIds, "node")}, "must occur at most"},
		{"a known node kind", core.SearchQuery{NodeKinds: []core.NodeKind{core.NodeKindProcess}}, ""},
		{"an unknown node kind", core.SearchQuery{NodeKinds: []core.NodeKind{"planet"}}, "nodeKind is outside"},
		{"an unknown edge kind", core.SearchQuery{EdgeKinds: []core.EdgeKind{"orbit"}}, "edgeKind is outside"},
		{"an unknown granularity", core.SearchQuery{Granularity: "atom"}, "granularity is outside"},
		{"the record kind at the object granularity", core.SearchQuery{
			Granularity: core.GraphGranularityObject, NodeKinds: []core.NodeKind{core.NodeKindRecord},
		}, "carries no record node"},
		{"an address range", core.SearchQuery{AddressInCidr: "198.51.100.7/24", AddressNotInCidr: "2001:db8::/32"}, ""},
		{"an address without a length", core.SearchQuery{AddressInCidr: "198.51.100.7"},
			"addressInCidr must be an address range"},
		{"an event action range", core.SearchQuery{RecordConditions: core.RecordConditions{
			EventActionFrom: uint64Of(4624), EventActionTo: uint64Of(4624)}}, ""},
		{"a reversed event action range", core.SearchQuery{RecordConditions: core.RecordConditions{
			EventActionFrom: uint64Of(4625), EventActionTo: uint64Of(4624)}}, "must not exceed"},
		{"an event action beyond the safe integer", core.SearchQuery{RecordConditions: core.RecordConditions{
			EventActionTo: uint64Of(uint64(core.MaxSafeInteger) + 1)}}, "up to 2^53-1"},
		{"a case", core.SearchQuery{RecordConditions: core.RecordConditions{Case: "case-1"}}, ""},
		{"a case with a space", core.SearchQuery{RecordConditions: core.RecordConditions{Case: "case 1"}},
			"reading the requested case"},
		{"a period", core.SearchQuery{RecordConditions: period}, ""},
		{"a period without the unit", core.SearchQuery{RecordConditions: core.RecordConditions{
			TimeFrom: period.TimeFrom, TimeFromPrecision: period.TimeFromPrecision}}, "filterUnit is required"},
		{"a unit without a period", core.SearchQuery{RecordConditions: core.RecordConditions{
			FilterUnit: core.FilterUnitSecond}}, "filterUnit needs timeFrom or timeTo"},
		{"an unknown unit", core.SearchQuery{RecordConditions: core.RecordConditions{
			TimeTo: period.TimeTo, TimeToPrecision: period.TimeToPrecision, FilterUnit: "hour"}},
			"filterUnit must be second, millisecond, or microsecond"},
		{"a precision without its bound", core.SearchQuery{RecordConditions: core.RecordConditions{
			TimeFromPrecision: core.PrecisionSecond}}, "timeFromPrecision needs timeFrom"},
		{"a bound without the offset", core.SearchQuery{RecordConditions: core.RecordConditions{
			TimeFrom: "2026-01-02T03:04:05", TimeFromPrecision: core.PrecisionSecond,
			FilterUnit: core.FilterUnitSecond}}, "must be a date-time with an offset"},
		{"a bound whose fraction differs from its precision", core.SearchQuery{
			RecordConditions: core.RecordConditions{
				TimeFrom: "2026-01-02T03:04:05.1Z", TimeFromPrecision: core.PrecisionMillisecond,
				FilterUnit: core.FilterUnitSecond}}, "3 for millisecond"},
		{"a bound without a precision", core.SearchQuery{RecordConditions: core.RecordConditions{
			TimeFrom: period.TimeFrom, FilterUnit: core.FilterUnitSecond}},
			"timeFrom needs timeFromPrecision, which must be second, millisecond, or microsecond"},
	}
	for _, c := range cases {
		err := c.query.Validate()
		switch {
		case c.wantErr == "" && err != nil:
			t.Errorf("%s: Validate = %v, want nil", c.name, err)
		case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
			t.Errorf("%s: Validate = %v, want an error with %q", c.name, err, c.wantErr)
		}
	}
}

// 画面の検索欄が表せる条件だけを通す。
func TestSearchQueryValidateShowable(t *testing.T) {
	process := []core.NodeKind{core.NodeKindProcess}
	withRecord := []core.NodeKind{core.NodeKindProcess, core.NodeKindRecord}
	cases := []struct {
		name    string
		query   core.SearchQuery
		wantErr string
	}{
		{"no view item", core.SearchQuery{Depth: 1}, ""},
		{"the object granularity without kinds", core.SearchQuery{Granularity: core.GraphGranularityObject}, ""},
		{"kinds without a granularity", core.SearchQuery{NodeKinds: process}, ""},
		{"kinds at the object granularity", core.SearchQuery{Granularity: core.GraphGranularityObject,
			NodeKinds: process}, ""},
		{"the record kind at the record granularity", core.SearchQuery{Granularity: core.GraphGranularityRecord,
			NodeKinds: withRecord}, ""},
		{"origin nodes", core.SearchQuery{NodeIds: []string{"n:1"}, Depth: 2}, ""},
		{"the record granularity without kinds", core.SearchQuery{Granularity: core.GraphGranularityRecord},
			"record granularity only with the record nodeKind"},
		{"the record granularity without the record kind", core.SearchQuery{
			Granularity: core.GraphGranularityRecord, NodeKinds: process},
			"record granularity only with the record nodeKind"},
	}
	for _, c := range cases {
		err := c.query.ValidateShowable()
		switch {
		case c.wantErr == "" && err != nil:
			t.Errorf("%s: ValidateShowable = %v, want nil", c.name, err)
		case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
			t.Errorf("%s: ValidateShowable = %v, want an error with %q", c.name, err, c.wantErr)
		}
	}
}

// 利用者が入れた文字列を error message に載せない。
func TestSearchQueryValidateDoesNotEchoTheTerms(t *testing.T) {
	secret := "synthetic-secret-term"
	for _, query := range []core.SearchQuery{
		{ValueContains: []string{strings.Repeat(secret, core.MaxSearchTokenLength)}},
		{AddressInCidr: secret},
		{RecordConditions: core.RecordConditions{
			TimeFrom: secret, TimeFromPrecision: core.PrecisionSecond, FilterUnit: core.FilterUnitSecond}},
		{RecordConditions: core.RecordConditions{Case: secret + " "}},
	} {
		err := query.Validate()
		if err == nil {
			t.Fatalf("Validate(%+v) = nil, want an error", query)
		}
		if strings.Contains(err.Error(), secret) {
			t.Errorf("Validate echoed the term: %v", err)
		}
	}
}

// アドレスの範囲は、範囲の中の値を持つ文字列をその長さの範囲へ直す。
func TestSearchQueryAddressPrefixes(t *testing.T) {
	in, notIn, err := core.SearchQuery{AddressInCidr: "198.51.100.7/24"}.AddressPrefixes()
	if err != nil {
		t.Fatal(err)
	}
	if in == nil || in.String() != "198.51.100.0/24" || notIn != nil {
		t.Errorf("AddressPrefixes = %v, %v, want 198.51.100.0/24 and nil", in, notIn)
	}
}

// 期間の端は、応答へ返す文字列の組と比較に用いる時刻を持つ。
func TestRecordConditionsPeriod(t *testing.T) {
	from, to, err := core.RecordConditions{
		TimeFrom: "2026-01-02T03:04:05.678+09:00", TimeFromPrecision: core.PrecisionMillisecond,
		FilterUnit: core.FilterUnitMillisecond,
	}.Period()
	if err != nil {
		t.Fatal(err)
	}
	if to != nil {
		t.Errorf("to = %+v, want nil", to)
	}
	want := time.Date(2026, 1, 1, 18, 4, 5, 678_000_000, time.UTC)
	if from == nil || !from.Instant.Equal(want) {
		t.Fatalf("from = %+v, want the instant %v", from, want)
	}
	if from.Requested.RequestText != "2026-01-02T03:04:05.678+09:00" ||
		from.Requested.Normalized != "2026-01-02T03:04:05.678+09:00" ||
		from.Requested.Precision != core.PrecisionMillisecond {
		t.Errorf("requested = %+v", from.Requested)
	}
	if err := from.Requested.Validate(); err != nil {
		t.Error(err)
	}
	if got := core.FilterUnitMillisecond.Duration(); got != time.Millisecond {
		t.Errorf("millisecond unit = %v", got)
	}
	if got := core.FilterUnitMicrosecond.Duration(); got != time.Microsecond {
		t.Errorf("microsecond unit = %v", got)
	}
	if got := core.FilterUnit("").Duration(); got != time.Second {
		t.Errorf("empty unit = %v, want a second", got)
	}
}

func TestRecordConditionsPeriodAcceptsMicrosecondBounds(t *testing.T) {
	from, _, err := core.RecordConditions{
		TimeFrom: "2031-10-08T01:02:03.123456Z", TimeFromPrecision: core.PrecisionMicrosecond,
		TimeTo: "2031-10-08T01:02:03.123456Z", TimeToPrecision: core.PrecisionMicrosecond,
		FilterUnit: core.FilterUnitMicrosecond,
	}.Period()
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2031, 10, 8, 1, 2, 3, 123_456_000, time.UTC)
	if from == nil || !from.Instant.Equal(want) || from.Requested.Precision != core.PrecisionMicrosecond {
		t.Fatalf("from = %+v, want the microsecond instant %v", from, want)
	}
}

// JSON の項目の名前は、部分グラフの要求の query の項目と同じ名前であり、往復で値が変わらない。
func TestSearchQueryJSONRoundTrip(t *testing.T) {
	query := core.SearchQuery{
		NodeKinds: []core.NodeKind{core.NodeKindProcess}, Granularity: core.GraphGranularityObject,
		NodeIds: []string{"node"}, Depth: 2, EdgeKinds: []core.EdgeKind{core.EdgeKindProcessParentChild},
		ValueContains: []string{"x"}, ValueExcludes: []string{"y"}, ValueField: "f", CountBy: "g",
		AddressInCidr: "198.51.100.0/24", AddressNotInCidr: "203.0.113.0/24",
		RecordConditions: core.RecordConditions{
			EventCategory: "c", EventAction: "a", EventActionFrom: uint64Of(1), EventActionTo: uint64Of(2),
			TimeFrom: "2026-01-02T03:04:05Z", TimeFromPrecision: core.PrecisionSecond,
			TimeTo: "2026-01-02T03:04:06Z", TimeToPrecision: core.PrecisionSecond,
			FilterUnit: core.FilterUnitSecond, Case: "case-1", Terminal: "terminal",
		},
	}
	encoded, err := json.Marshal(query)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"nodeKinds", "granularity", "nodeIds", "depth", "edgeKinds", "valueContains", "valueExcludes",
		"valueField", "countBy", "addressInCidr", "addressNotInCidr", "eventCategory", "eventAction",
		"eventActionFrom", "eventActionTo", "timeFrom", "timeFromPrecision", "timeTo", "timeToPrecision",
		"filterUnit", "case", "terminal",
	} {
		if _, ok := fields[name]; !ok {
			t.Errorf("JSON lacks %s: %s", name, encoded)
		}
	}
	var decoded core.SearchQuery
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, query) {
		t.Errorf("round trip = %+v, want %+v", decoded, query)
	}
}
