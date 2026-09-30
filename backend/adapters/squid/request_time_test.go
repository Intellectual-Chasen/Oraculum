package squid_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/squid"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func TestRequestTime(t *testing.T) {
	got, failure := squid.ParseRequestTime(readOK(t, exampleLine))
	if failure != nil {
		t.Fatal(failure)
	}
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
	if raw, ok := got.RawTextValue(); !ok || raw != "[14/Feb/2024:08:09:10 +0530]" {
		t.Fatal("raw time lost brackets")
	}
	if normalized, ok := got.NormalizedValue(); !ok || normalized != "2024-02-14T08:09:10+05:30" {
		t.Fatal("normalized time differs")
	}
	if offset, ok := got.OffsetTextValue(); !ok || offset != "+0530" {
		t.Fatal("offset differs")
	}
	if got.Precision != core.PrecisionSecond || got.Clock != core.ClockObserverLocal || got.OffsetState != core.OffsetStateInValue || got.NormalizedForm != core.NormalizedFormRFC3339Absolute || got.Meaning != core.MeaningEvent || got.ValueState != core.ValueStatePresent {
		t.Fatal("timestamp metadata differs")
	}
	if instant, ok := got.Instant(); !ok || !instant.Equal(time.Date(2024, time.February, 14, 2, 39, 10, 0, time.UTC)) {
		t.Fatal("absolute instant differs")
	}
	for _, pair := range [][2]string{{"+0000", "2024-02-14T08:09:10Z"}, {"-0330", "2024-02-14T08:09:10-03:30"}} {
		r := readOK(t, strings.Replace(exampleLine, "+0530", pair[0], 1))
		stamp, f := squid.ParseRequestTime(r)
		if value, ok := stamp.NormalizedValue(); f != nil || !ok || value != pair[1] {
			t.Fatal("offset normalization differs")
		}
	}
}

func TestRequestTimeRejectsFractions(t *testing.T) {
	for _, second := range []string{"10.1", "10,001", "10.000"} {
		r := readOK(t, strings.Replace(exampleLine, "08:09:10", "08:09:"+second, 1))
		_, failure := squid.ParseRequestTime(r)
		checkFailure(t, failure, core.FailureStageNormalize, 1, failureDetails{"a bracketed local timestamp with second precision and a numeric offset", "time item does not match the bracketed second-precision layout", 14})
	}
}

func TestRequestTimeRejectsInvalidValues(t *testing.T) {
	for _, tt := range []struct{ raw, observed string }{
		{"[31/Feb/2024:08:09:10 +0530]", `parsing time "31/Feb/2024:08:09:10 +0530": day out of range`},
		{"[14/Bad/2024:08:09:10 +0530]", `parsing time "14/Bad/2024:08:09:10 +0530" as "02/Jan/2006:15:04:05 -0700": cannot parse "Bad/2024:08:09:10 +0530" as "Jan"`},
		{"[14/Feb/2024:24:09:10 +0530]", `parsing time "14/Feb/2024:24:09:10 +0530": hour out of range`},
		{"[14/Feb/2024:08:09:10]", "time item does not match the bracketed second-precision layout"},
		{"[]", "time item does not match the bracketed second-precision layout"},
	} {
		r := readOK(t, strings.Replace(exampleLine, "[14/Feb/2024:08:09:10 +0530]", tt.raw, 1))
		_, failure := squid.ParseRequestTime(r)
		checkFailure(t, failure, core.FailureStageNormalize, 1, failureDetails{"a bracketed local timestamp with second precision and a numeric offset", tt.observed, 14})
		if *failure.ByteOffset != 14 {
			t.Fatal("time failure not located at item")
		}
	}
	_, failure := squid.ParseRequestTime(squid.Record{})
	checkUnknownPosition(t, failure)
}

func TestRequestTimeRejectsOffsetOverflow(t *testing.T) {
	for _, zone := range []string{"+0060", "-0060", "+2400"} {
		_, failure := squid.ParseRequestTime(readOK(t, strings.Replace(exampleLine, "+0530", zone, 1)))
		checkFailure(t, failure, core.FailureStageNormalize, 1, failureDetails{"a bracketed local timestamp with second precision and a numeric offset", "numeric time offset is outside the hour or minute range", 14})
	}
}

func TestRequestTimeMissingItem(t *testing.T) {
	record, _ := readOne(t, "192.0.2.8 - - \n")
	_, failure := squid.ParseRequestTime(record)
	checkFailure(t, failure, core.FailureStageNormalize, 1, failureDetails{
		"a bracketed local timestamp with second precision and a numeric offset",
		"the requestTime item is absent", 0,
	})
}
