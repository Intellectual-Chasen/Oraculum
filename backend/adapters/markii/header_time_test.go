package markii_test

import (
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/markii"
)

func TestHeaderTimestamp(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		valid       bool
	}{
		{"other_event", "04/05/2024 06:07:08.009 +0000 sn=12 evt=file", true},
		{"invalid_time", "99/05/2024 06:07:08.009 +0000 sn=12 evt=file", false},
		{"missing_header", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var record markii.Record
			if tc.input != "" {
				var reader markii.Reader
				reader.Reset(strings.NewReader(tc.input))
				value, failure, err := reader.Next()
				if err != nil || failure != nil {
					t.Fatalf("read: %+v %v", failure, err)
				}
				record = value
			}
			timestamp, ok := markii.HeaderTimestamp(record)
			if ok != tc.valid {
				t.Fatalf("time present=%t, want %t", ok, tc.valid)
			}
			if !ok {
				return
			}
			if err := timestamp.Validate(); err != nil {
				t.Fatal(err)
			}
			if raw, ok := timestamp.RawTextValue(); !ok || raw != "04/05/2024 06:07:08.009 +0000" {
				t.Fatalf("raw=%q, present=%t", raw, ok)
			}
			if normalized, ok := timestamp.NormalizedValue(); !ok || normalized != "2024-04-05T06:07:08.009Z" {
				t.Fatalf("normalized=%q present=%t", normalized, ok)
			}
		})
	}
}
