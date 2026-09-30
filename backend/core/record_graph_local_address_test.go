package core_test

import (
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// addressNodeOf は識別鍵の並びから IP アドレスのノードを返す。
func addressNodeOf(t *testing.T, keys []core.NodeKey) core.NodeKey {
	t.Helper()
	for _, key := range keys {
		if key.Kind == core.NodeKindIp {
			return key
		}
	}
	t.Fatalf("the keys %+v carry no ip node", keys)
	return core.NodeKey{}
}

func digestOf(key core.NodeKey) string {
	return strings.Join(key.DigestParts(), "\x00")
}

// ループバックとリンクローカルのアドレスは端末ごとに別のノードになる。ほかのアドレスは
// 端末をまたいで 1 つのノードになる。
func TestNewRecordGraphScopesTheTerminalLocalAddressToTheTerminal(t *testing.T) {
	for _, testCase := range []struct {
		address string
		local   bool
	}{
		{"127.0.0.1", true},
		{"127.10.0.1", true},
		{"::1", true},
		{"::ffff:127.0.0.1", true},
		{"169.254.1.1", true},
		{"fe80::1", true},
		{"192.0.2.1", false},
		{"2001:db8::1", false},
		{"not-an-address", false},
	} {
		t.Run(testCase.address, func(t *testing.T) {
			onTerminal := func(terminalId string) core.NodeKey {
				graph := core.NewRecordGraph([]core.RecordField{
					textField(t, "tmid", core.SemanticKeyTerminalId, terminalId),
					textField(t, "ip", core.SemanticKeyTerminalIpAddress, testCase.address),
				})
				requireStrings(t, "links", linkNames(graph.Links), "terminal_address terminal ip")
				return addressNodeOf(t, graph.Nodes)
			}
			first, second := onTerminal("T1"), onTerminal("T2")
			if !testCase.local {
				if first.Form != core.NodeKeyFormAddress || digestOf(first) != digestOf(second) {
					t.Errorf("the keys are %+v and %+v, want one address key", first, second)
				}
				return
			}
			if first.Form != core.NodeKeyFormTerminalAddress {
				t.Errorf("the key form is %q, want %q", first.Form, core.NodeKeyFormTerminalAddress)
			}
			if digestOf(first) == digestOf(second) {
				t.Errorf("both terminals key the address as %q, want one node per terminal", digestOf(first))
			}
			requireStrings(t, "identity values", identityValues(first), "T1", testCase.address)
			if err := first.Validate(); err != nil {
				t.Errorf("the key %+v = %v, want a valid key", first, err)
			}
			if label, _ := first.LabelValue(); label != testCase.address {
				t.Errorf("the label value is %q, want %q", label, testCase.address)
			}
		})
	}
}

// 端末の範囲が決まらないレコードは、ループバックのアドレスのノードを作らない。
func TestNewRecordGraphBuildsNoTerminalLocalAddressWithoutTheTerminal(t *testing.T) {
	graph := core.NewRecordGraph([]core.RecordField{
		textField(t, "clientIp", core.SemanticKeyConnectionSourceAddress, "127.0.0.1"),
		textField(t, "host", core.SemanticKeyConnectionDestinationHostname, "example.test"),
		textField(t, "requestMethod", core.SemanticKeyHttpRequestMethod, "GET"),
	})
	requireStrings(t, "node kinds", nodeKinds(graph.Nodes), "domain")
	requireStrings(t, "links", linkNames(graph.Links))
	if keys := core.DestinationNodeKeys([]core.RecordField{
		textField(t, "dstIP", core.SemanticKeyConnectionDestinationAddress, "::1"),
	}, core.RecordScope{}); len(keys) != 0 {
		t.Errorf("the destination keys are %+v, want none", keys)
	}
}

// ループバックのアドレスを接続先や接続元にした関係は、端末の範囲の鍵を両端に置く。
func TestNewRecordGraphLinksTheScopedLocalAddress(t *testing.T) {
	recording, _ := core.RecordingTerminalNodeKey(
		"0000000000000000000000000000000000000000000000000000000000000002")

	t.Run("プロセスの接続先", func(t *testing.T) {
		fields := []core.RecordField{
			textField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
			textField(t, "psGUID", core.SemanticKeyProcessId, "P1"),
			textField(t, "dstIP", core.SemanticKeyConnectionDestinationAddress, "127.0.0.1"),
		}
		graph := core.NewRecordGraph(fields)
		requireStrings(t, "links", linkNames(graph.Links),
			"ran_on process terminal", "process_communication process ip")
		target := graph.Links[1].Target
		requireStrings(t, "destination identity", identityValues(target), "T1", "127.0.0.1")
		destinations := core.DestinationNodeKeys(fields, core.RecordScope{})
		if len(destinations) != 1 || digestOf(destinations[0]) != digestOf(target) {
			t.Errorf("the destination keys are %+v, want %+v", destinations, target)
		}
	})
	t.Run("端末を指さない Proxy 型のレコードの接続元", func(t *testing.T) {
		fields := []core.RecordField{
			textField(t, "clientIp", core.SemanticKeyConnectionSourceAddress, "127.0.0.1"),
			textField(t, "host", core.SemanticKeyConnectionDestinationHostname, "example.test"),
			textField(t, "requestMethod", core.SemanticKeyHttpRequestMethod, "GET"),
		}
		graph := core.NewRecordGraphInScope(fields, core.RecordScope{Terminal: &recording})
		requireStrings(t, "node kinds", nodeKinds(graph.Nodes), "ip", "domain")
		requireStrings(t, "links", linkNames(graph.Links), "http_request ip domain")
		source := graph.Links[0].Source
		if source.Form != core.NodeKeyFormTerminalAddress ||
			source.Values[0] != recording.Values[0] {
			t.Errorf("the request source is %+v, want the address in the scope of %+v",
				source, recording)
		}
	})
}
