package pipeline

import (
	"reflect"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// **Proxy のログの要求と、端末の記録が示す接続を、接続元のアドレスごとに並べる。** 接続先が
// Proxy のアドレスである接続と、それ以外の直接の接続を分けて数える。
func TestProxyBypassCountsTheDirectConnectionsBesideTheProxyRequests(t *testing.T) {
	result := proxyCandidateResult(t)
	proxySource := result.publications[3].status.SourceId

	bypass, found := result.ProxyBypassOf(proxySource)
	if !found {
		t.Fatal("the proxy source has no comparison")
	}
	want := ProxyBypass{
		ProxyAddresses: []string{"192.0.2.80"},
		Clients: []ProxyBypassClient{{
			ClientIp:                  "192.0.2.10",
			Terminals:                 []string{"ws-1"},
			ProxyRequestCount:         3,
			ProxyConnectionCount:      2,
			DirectConnectionCount:     1,
			DirectConnectionCountable: true,
			DirectDestinations: []ProxyBypassDestination{
				{Address: "203.0.113.50", Port: "443", RecordCount: 1},
			},
		}},
	}
	if !reflect.DeepEqual(bypass, want) {
		t.Errorf("the comparison is %+v, want %+v", bypass, want)
	}
}

// **接続元のアドレスの接続を記録する収集元が無い行は、経由しない接続を数えられない。**
// 接続を 1 件も記録していない接続元でも、その IP か割当の端末に、接続を記録した Proxy 以外の
// 収集元を割り当てていれば、0 件は数えた結果である。接続のレコードを 1 件も持たない収集元の
// 割当は、数えられる根拠にならない。
func TestProxyBypassMarksTheClientsWithoutASourceAsUncountable(t *testing.T) {
	base := proxyCandidateResult(t)
	result := base.WithAnalystTerminalAssignments(append(base.AnalystTerminalAssignments(),
		sessionAssignment(t, base, 1, "192.0.2.20", "ws-2", "", false),
		sessionAssignment(t, base, 2, "192.0.2.30", "ws-3", "", true),
		sessionAssignment(t, base, 1, "192.0.2.31", "ws-3", "", false),
		sessionAssignment(t, base, 0, "192.0.2.40", "ws-4", "", true),
		sessionAssignment(t, base, 1, "192.0.2.41", "ws-4", "", false),
	))
	bypass, _ := result.ProxyBypassOf(result.publications[3].status.SourceId)
	countable := make(map[string]bool, len(bypass.Clients))
	for _, client := range bypass.Clients {
		countable[client.ClientIp] = client.DirectConnectionCountable
	}
	want := map[string]bool{
		"192.0.2.10": true, "192.0.2.20": false, "192.0.2.30": false, "192.0.2.31": false,
		"192.0.2.40": true, "192.0.2.41": true,
	}
	if !reflect.DeepEqual(countable, want) {
		t.Errorf("the countable clients are %v, want %v", countable, want)
	}
}

// Proxy のログでない収集元と、公開されていない収集元には比較を返さない。
func TestProxyBypassNeedsAProxySource(t *testing.T) {
	result := proxyCandidateResult(t)
	if _, found := result.ProxyBypassOf(result.publications[0].status.SourceId); found {
		t.Error("a Windows source returned a proxy comparison")
	}
	if _, found := result.ProxyBypassOf("unknown"); found {
		t.Error("an unknown source returned a proxy comparison")
	}
}

// Proxy の収集元に IP の割当が無いと、接続を数えずに Proxy のログの要求だけを数える。
func TestProxyBypassCountsOnlyTheRequestsWithoutTheProxyAddress(t *testing.T) {
	result := proxyCandidateResultWithProxyIp(t, "")
	bypass, _ := result.ProxyBypassOf(result.publications[3].status.SourceId)
	want := ProxyBypass{
		ProxyAddresses: []string{},
		Clients: []ProxyBypassClient{{
			ClientIp: "192.0.2.10", Terminals: []string{"ws-1"}, ProxyRequestCount: 3,
			DirectDestinations: []ProxyBypassDestination{},
		}},
	}
	if !reflect.DeepEqual(bypass, want) {
		t.Errorf("the comparison is %+v, want %+v", bypass, want)
	}
}

// 公開していない収集元のレコードは接続を数える側に入れない。
func TestProxyBypassSkipsTheWithheldSource(t *testing.T) {
	result := proxyCandidateResult(t)
	result.publications[0].status.PublicationState = core.PublicationStateWithheld
	bypass, _ := result.ProxyBypassOf(result.publications[3].status.SourceId)
	client := bypass.Clients[0]
	if client.ProxyConnectionCount != 0 || client.DirectConnectionCount != 0 {
		t.Errorf("the withheld source was counted: %+v", client)
	}
}

// 接続元を IP として読めない Proxy のログのレコードの件数を返す。
func TestProxyBypassReportsTheProxyRecordsWithoutAClientAddress(t *testing.T) {
	proxy := `client.example.test - - [03/Feb/2001:04:10:02 +0000] "GET http://example.test/a HTTP/1.1" 200 12 "-" "test" TCP_MISS:DIRECT` + "\n" +
		`192.0.2.10 - - [03/Feb/2001:04:20:00 +0000] "GET http://example.test/c HTTP/1.1" 200 12 "-" "test" TCP_HIT:NONE` + "\n"
	result := sessionSourcesResult(t, sessionSource{name: "access.log", format: SquidFormatKey, document: proxy})
	bypass, _ := result.ProxyBypassOf(result.publications[0].status.SourceId)
	if bypass.UnreadableProxyRecordCount != 1 || len(bypass.Clients) != 1 || bypass.Clients[0].ProxyRequestCount != 1 {
		t.Errorf("the comparison is %+v, want 1 unreadable record and 1 counted request", bypass)
	}
}
