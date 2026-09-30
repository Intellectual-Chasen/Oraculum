package api

import (
	"encoding/json"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// 集計の値を取り違えずに応答へ写す。port の記録が無い接続先は port を出さない。
func TestProxyBypassResponseCopiesEveryCount(t *testing.T) {
	response := proxyBypassResponseOf("s-1", pipeline.ProxyBypass{
		ProxyAddresses: []string{"192.0.2.80"}, UnreadableProxyRecordCount: 7,
		Clients: []pipeline.ProxyBypassClient{{
			ClientIp: "192.0.2.10", Terminals: []string{"ws-1"},
			ProxyRequestCount: 11, ProxyConnectionCount: 5, DirectConnectionCount: 3,
			DirectConnectionCountable: true,
			DirectDestinations: []pipeline.ProxyBypassDestination{
				{Address: "203.0.113.50", Port: "443", RecordCount: 2},
				{Address: "203.0.113.51", RecordCount: 1},
			},
		}},
	})
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"sourceId":"s-1","proxyAddresses":["192.0.2.80"],"unreadableProxyRecordCount":7,"clients":[` +
		`{"clientIp":"192.0.2.10","terminals":["ws-1"],"proxyRequestCount":11,"proxyConnectionCount":5,` +
		`"directConnectionCount":3,"directConnectionCountable":true,"directDestinations":[{"address":"203.0.113.50","port":"443","recordCount":2},` +
		`{"address":"203.0.113.51","recordCount":1}]}]}`
	if string(body) != want {
		t.Errorf("the response is\n%s\nwant\n%s", body, want)
	}
}
