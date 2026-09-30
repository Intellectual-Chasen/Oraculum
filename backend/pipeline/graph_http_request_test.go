// in-package test: 取り込みで端末を付けた Proxy の記録から、HTTP の要求の関係を組む条件を確かめる。
package pipeline

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// Squid の収集元。接続元 IP は RFC 5737、host は example.test 系である。
const proxiedRequestsDocument = `192.0.2.10 - - [03/Feb/2001:04:10:00 +0000] "GET http://a.example.test/ HTTP/1.1" ` +
	`200 3 "-" "agent" TCP_MISS:HIER_DIRECT` + "\n" +
	`192.0.2.10 - - [03/Feb/2001:04:11:00 +0000] "GET http://a.example.test/b HTTP/1.1" ` +
	`200 3 "-" "agent" TCP_MISS:HIER_DIRECT` + "\n"

// 取り込みで Proxy の端末を付けた収集元でも、要求を出したのは各行の接続元 IP である。
// 関係は接続元 IP から要求先へ向き、付けた端末は収集元を記録した Proxy として自身の IP へ
// つながる。端末の外部識別子を省いた指定も同じである。
func TestProxyTerminalKeepsTheHttpRequestFromTheClientAddress(t *testing.T) {
	for name, terminal := range map[string]SourceTerminal{
		"identifier": {TerminalId: "proxy-1", TerminalHostname: "proxy.example.test", Ip: "192.0.2.80"},
		"ip alone":   {Ip: "192.0.2.80"},
	} {
		t.Run(name, func(t *testing.T) {
			result := sessionSourcesResult(t, sessionSource{
				name: "access.log", format: SquidFormatKey, document: proxiedRequestsDocument,
			})
			sourceId := result.publications[0].status.SourceId
			assignment, err := importSpecifiedAssignment(terminal, result.identities[sourceId], nil)
			if err != nil {
				t.Fatalf("building the import specification: %v", err)
			}
			result.importAssignments = []core.TerminalAssignment{assignment}
			graph := NewGraph(result, AllMatchConditions())

			ipKey := func(value string) core.NodeKey {
				return core.NodeKey{Kind: core.NodeKindIp, Form: core.NodeKeyFormAddress,
					Values: []core.NodeIdentityValue{{Value: value}}}
			}
			clientAt := requireNodeAt(t, graph, ipKey("192.0.2.10"))
			destinationAt := requireNodeAt(t, graph, core.NodeKey{
				Kind: core.NodeKindDomain, Form: core.NodeKeyFormHostname,
				Values: []core.NodeIdentityValue{{Value: "a.example.test"}},
			})
			if !hasEdge(graph, core.EdgeKindHttpRequest, clientAt, destinationAt) {
				t.Error("the client address has no http_request edge to the requested host")
			}
			if got := len(edgesOfKind(graph, core.EdgeKindHttpRequest)); got != 1 {
				t.Errorf("http_request edges = %d, want only the one from the client address", got)
			}
			proxyKey, built := assignment.TerminalNodeKey()
			if !built {
				t.Fatal("the import specification builds no terminal key")
			}
			proxyAt := requireNodeAt(t, graph, proxyKey)
			if !hasEdge(graph, core.EdgeKindTerminalAddress, proxyAt, requireNodeAt(t, graph, ipKey("192.0.2.80"))) {
				t.Error("the proxy terminal has no terminal_address edge to its own address")
			}
			if hasEdge(graph, core.EdgeKindTerminalAddress, proxyAt, clientAt) {
				t.Error("the proxy terminal claims the client address as its own")
			}
		})
	}
}
