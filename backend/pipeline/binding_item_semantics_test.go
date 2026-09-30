package pipeline_test

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// 走査器が名乗る欄の集合は、その収集元の欄の並びから決まる。
//
// 集合は MatchRequest.CounterpartItemSemantics になり、client_port と process と user の
// use が not_used になるか item_absent_on_counterpart になるかを決める。
func TestSquidParserNamesTheItemSemanticsOfTheLayout(t *testing.T) {
	cases := map[string]struct {
		parser  pipeline.SourceParser
		carries []core.SemanticKey
		absent  []core.SemanticKey
	}{
		"combined の preset": {
			parser: pipeline.NewTestSquidParser(),
			carries: []core.SemanticKey{
				core.SemanticKeyConnectionSourceAddress,
				core.SemanticKeyAccountName,
				core.SemanticKeyEventTime,
				core.SemanticKeyHttpRequestMethod,
				core.SemanticKeyConnectionDestinationAddress,
				core.SemanticKeyConnectionDestinationHostname,
				core.SemanticKeyConnectionDestinationPort,
			},
			// combined は接続元 port とプロセスの欄を持たない。
			absent: []core.SemanticKey{
				core.SemanticKeyConnectionSourcePort, core.SemanticKeyProcessId,
			},
		},
		"利用者の欄を持たない並び": {
			parser: pipeline.NewTestSquidLogFormatParser(
				`%>a [%tl] "%rm %ru HTTP/%rv" %>Hs %<st`),
			carries: []core.SemanticKey{
				core.SemanticKeyConnectionSourceAddress,
				core.SemanticKeyConnectionDestinationPort,
			},
			absent: []core.SemanticKey{core.SemanticKeyAccountName},
		},
		"要求行を持たない並び": {
			parser:  pipeline.NewTestSquidLogFormatParser(`%>a %[un [%tl] %>Hs`),
			carries: []core.SemanticKey{core.SemanticKeyAccountName},
			// 接続先の 3 件と method は要求行の欄から導く。
			absent: []core.SemanticKey{
				core.SemanticKeyHttpRequestMethod,
				core.SemanticKeyConnectionDestinationAddress,
				core.SemanticKeyConnectionDestinationHostname,
				core.SemanticKeyConnectionDestinationPort,
			},
		},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			got := want.parser.Identity().ItemSemantics
			for _, semantic := range want.carries {
				if !slices.Contains(got, semantic) {
					t.Errorf("the item semantics %v omit %q", got, semantic)
				}
			}
			for _, semantic := range want.absent {
				if slices.Contains(got, semantic) {
					t.Errorf("the item semantics %v carry %q", got, semantic)
				}
			}
			if len(got) != len(slices.Compact(slices.Clone(got))) {
				t.Errorf("the item semantics %v repeat an element", got)
			}
		})
	}
}

// Squid のレコードの時刻は秒までを書く。秒未満を比べる相手にならない。
func TestSquidParserNamesTheSecondTimePrecision(t *testing.T) {
	got := pipeline.NewTestSquidParser().Identity().TimePrecision
	if got != core.PrecisionSecond {
		t.Errorf("the time precision is %q, want %q", got, core.PrecisionSecond)
	}
}

// markii 形式の欄の集合は、条件が取り出す接続元 port とプロセスと利用者を持つ。
func TestMarkIIParserNamesTheConditionColumns(t *testing.T) {
	identity := pipeline.NewTestMarkIIParser().Identity()
	for _, semantic := range []core.SemanticKey{
		core.SemanticKeyConnectionSourcePort,
		core.SemanticKeyProcessId,
		core.SemanticKeyEventAccountName,
		core.SemanticKeyAccountName,
		core.SemanticKeyConnectionDestinationAddress,
		core.SemanticKeyConnectionDestinationPort,
		core.SemanticKeyTerminalId,
	} {
		if !slices.Contains(identity.ItemSemantics, semantic) {
			t.Errorf("the item semantics %v omit %q", identity.ItemSemantics, semantic)
		}
	}
	if got := identity.ItemSemantics; len(got) != len(slices.Compact(slices.Clone(got))) {
		t.Errorf("the item semantics %v repeat an element", got)
	}
	// クライアントログのヘッダーの時刻はミリ秒までを書く。
	if identity.TimePrecision != core.PrecisionMillisecond {
		t.Errorf("the time precision is %q, want %q",
			identity.TimePrecision, core.PrecisionMillisecond)
	}
}
