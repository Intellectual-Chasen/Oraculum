package core_test

import (
	"errors"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 種別ごとの件数は、定義の中の種別と 1 以上の件数だけを受け付ける。
func TestKindCountsRejectAnUnknownKindAndAZeroCount(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		check func() error
		want  error
	}{
		{"a matched kind", core.MatchedKind{Kind: core.NodeKindIp, Count: 1}.Validate, nil},
		{"a matched kind of zero", core.MatchedKind{Kind: core.NodeKindIp, Count: 0}.Validate, core.ErrInvalid},
		{"an unknown matched kind", core.MatchedKind{Kind: "session", Count: 1}.Validate, core.ErrUnknownEnumValue},
		{"an edge kind count", core.EdgeKindCount{Kind: core.EdgeKindRanOn, Count: 1}.Validate, nil},
		{"an edge kind count of zero", core.EdgeKindCount{Kind: core.EdgeKindRanOn, Count: 0}.Validate, core.ErrInvalid},
		{"an unknown edge kind", core.EdgeKindCount{Kind: "copied_to", Count: 1}.Validate, core.ErrUnknownEnumValue},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.check()
			if testCase.want == nil && err != nil || testCase.want != nil && !errors.Is(err, testCase.want) {
				t.Errorf("err=%v, want %v", err, testCase.want)
			}
		})
	}
}
