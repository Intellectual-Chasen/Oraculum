package markii_test

import (
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/markii"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// tmid と com と ip の組を語彙の意味を付けた項目にし、ip をカンマで分割する。
// 3 つの key を揃えたレコードだけが値を返す。
func TestTerminalFields(t *testing.T) {
	const terminalId = "00000000-1111-2222-3333-444444444444"
	for _, tc := range []struct {
		name         string
		fields       string
		wantId       string
		wantHostname string
		wantIps      []string
		wantFound    bool
	}{
		{
			name: "IPv4 と IPv6 を持つ",
			fields: `tmid=` + terminalId + ` com="TESTHOST" ` +
				`ip=192.0.2.10,2001:db8::1`,
			wantId: terminalId, wantHostname: "TESTHOST",
			wantIps: []string{"192.0.2.10", "2001:db8::1"}, wantFound: true,
		},
		{
			name:   "IPv4 だけを持つ",
			fields: `tmid=` + terminalId + ` com="CLIENT01" ip=192.0.2.10`,
			wantId: terminalId, wantHostname: "CLIENT01",
			wantIps: []string{"192.0.2.10"}, wantFound: true,
		},
		{name: "tmid を持たない", fields: `com="CLIENT01" ip=192.0.2.10`},
		{name: "com を持たない", fields: `tmid=` + terminalId + ` ip=192.0.2.10`},
		{name: "ip を持たない", fields: `tmid=` + terminalId + ` com="CLIENT01"`},
		{name: "tmid の値が空", fields: `tmid="" com="CLIENT01" ip=192.0.2.10`},
		{name: "com の値が空", fields: `tmid=` + terminalId + ` com="" ip=192.0.2.10`},
		{name: "ip の値が空", fields: `tmid=` + terminalId + ` com="CLIENT01" ip=""`},
		{name: "ip が区切りだけ", fields: `tmid=` + terminalId + ` com="CLIENT01" ip=","`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := readOneOK(t, "02/01/2000 03:04:05.678 +0900 sn=1 "+tc.fields)
			got, found := markii.TerminalFields(record)
			if found != tc.wantFound {
				t.Fatalf("TerminalFields found = %t, want %t", found, tc.wantFound)
			}
			if !found {
				return
			}
			want := append([]semanticValue{
				{core.SemanticKeyTerminalId, tc.wantId},
				{core.SemanticKeyTerminalHostname, tc.wantHostname},
			}, terminalAddressValues(tc.wantIps)...)
			assertSemanticValues(t, got, want)
		})
	}
}

// semanticValue は 1 項目の語彙の項目と、比較に用いる値である。
type semanticValue struct {
	semantic core.SemanticKey
	value    string
}

func terminalAddressValues(addresses []string) []semanticValue {
	values := make([]semanticValue, 0, len(addresses))
	for _, address := range addresses {
		values = append(values, semanticValue{core.SemanticKeyTerminalIpAddress, address})
	}
	return values
}

func assertSemanticValues(t *testing.T, got []core.RecordField, want []semanticValue) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("fields = %d items, want %d", len(got), len(want))
	}
	for index, wanted := range want {
		field := got[index]
		if field.Semantic != wanted.semantic {
			t.Errorf("semantic at %d = %q, want %q", index, field.Semantic, wanted.semantic)
		}
		if field.Text == nil {
			t.Fatalf("field at %d carries no text", index)
		}
		value, readable := field.Text.ComparableValue()
		if !readable || value != wanted.value {
			t.Errorf("value at %d = %q (readable=%t), want %q",
				index, value, readable, wanted.value)
		}
	}
}

func TestSequenceNumberNonNegative(t *testing.T) {
	for _, event := range []string{"evt=ps subEvt=start", "evt=net subEvt=con"} {
		for _, tc := range []struct {
			text  string
			want  int64
			valid bool
		}{
			{"-1", 0, false}, {"0", 0, true}, {"9", 9, true},
		} {
			t.Run(event+"/"+tc.text, func(t *testing.T) {
				var reader markii.Reader
				reader.Reset(strings.NewReader("04/05/2024 06:07:08.009 +0000 sn=" + tc.text + " " + event + " psGUID=p tmid=t com=c csid=s psPath=tool"))
				record, failure, err := reader.Next()
				if err != nil || failure != nil {
					t.Fatalf("read: %v %v", failure, err)
				}
				var number *int64
				if markii.IsProcessStart(record) {
					observation, problem := markii.ParseProcessStart(record)
					number, failure = observation.SequenceNumber, problem
				} else {
					observation, problem := markii.ParseCommunication(record)
					number, failure = observation.SequenceNumber, problem
				}
				if tc.valid {
					if failure != nil || number == nil || *number != tc.want {
						t.Fatalf("number=%v failure=%+v", number, failure)
					}
				} else if failure == nil || failure.Stage != core.FailureStageFieldMap || failure.ExpectedMeaning != "a non-negative decimal integer as the value of sn" {
					t.Fatalf("negative sequence failure=%+v", failure)
				}
			})
		}
	}
}

func TestSequenceNumberPublic(t *testing.T) {
	for _, tc := range []struct {
		fields string
		want   int64
		ok     bool
	}{
		{"sn=0", 0, true}, {"sn=9", 9, true}, {"sn=-1", 0, false}, {"sn=bad", 0, false},
		{"evt=file", 0, false}, {"sn=1 sn=2", 0, false}, {"sn=9223372036854775808", 0, false},
	} {
		t.Run(tc.fields, func(t *testing.T) {
			var reader markii.Reader
			reader.Reset(strings.NewReader("04/05/2024 06:07:08.009 +0000 " + tc.fields))
			record, failure, err := reader.Next()
			if err != nil || failure != nil {
				t.Fatalf("read=%+v %v", failure, err)
			}
			got, ok := markii.SequenceNumber(record)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("sequence=(%d,%t) want=(%d,%t)", got, ok, tc.want, tc.ok)
			}
		})
	}
}
