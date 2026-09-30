package winevent_test

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/winevent"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// providerEvent は、プロバイダ provider のイベント eventID に data の `<Data>` を並べた文書を返す。
func providerEvent(provider, eventID string, data ...string) string {
	document := `<Event><System><Provider Name="` + provider + `"/>` +
		`<EventID>` + eventID + `</EventID>` +
		`<TimeCreated SystemTime="2001-02-03T04:05:06.1234567Z"/>` +
		`<EventRecordID>401</EventRecordID><Channel>System</Channel>` +
		`<Computer>host04.example.test</Computer></System><EventData>`
	for index := 0; index+1 < len(data); index += 2 {
		document += `<Data Name="` + data[index] + `">` + data[index+1] + `</Data>`
	}
	return document + `</EventData></Event>`
}

const dnsClient = "Microsoft-Windows-DNS-Client"

// 8020 の Ipaddress は記録した端末の IP アドレスになる。DNS サーバの値、別のイベントの Ipaddress、
// IP アドレスとして読めない値は語彙を持たない。IPv6 の形の IPv4 はドット 10 進で比べる。
func TestObserveReadsTheAddressOfADnsRegistration(t *testing.T) {
	declared := winevent.ItemSemantics()
	if !slices.Contains(declared, core.SemanticKeyTerminalIpAddress) {
		t.Error("ItemSemantics does not declare terminal.ip_address")
	}
	for _, tc := range []struct {
		name, provider, eventID, address string
		want                             core.SemanticKey
		comparable                       string
	}{
		{"registration", dnsClient, "8020", "192.0.2.10", core.SemanticKeyTerminalIpAddress, "192.0.2.10"},
		{"mapped", dnsClient, "8020", "::ffff:192.0.2.10", core.SemanticKeyTerminalIpAddress, "192.0.2.10"},
		{"link local", dnsClient, "8020", "fe80::1", core.SemanticKeyTerminalIpAddress, "fe80::1"},
		{"dash", dnsClient, "8020", "-", "", ""},
		{"unreadable", dnsClient, "8020", "unknown", "", ""},
		{"two addresses", dnsClient, "8020", "192.0.2.10, 192.0.2.11", "", ""},
		{"another event", dnsClient, "1014", "192.0.2.10", "", ""},
		{"another provider", "Microsoft-Windows-Example", "8020", "192.0.2.10", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fields := observe(t, providerEvent(tc.provider, tc.eventID,
				"HostName", "host04", "DnsServerList", "192.0.2.53", "Ipaddress", tc.address)).Fields
			address := fieldNamed(t, fields, "EventData.Ipaddress")
			if address.Semantic != tc.want {
				t.Fatalf("Ipaddress semantic = %q, want %q", address.Semantic, tc.want)
			}
			if tc.want != "" {
				if got, _ := address.Text.ComparableValue(); got != tc.comparable {
					t.Errorf("Ipaddress compares as %q, want %q", got, tc.comparable)
				}
			}
			if semantic := fieldNamed(t, fields, "EventData.DnsServerList").Semantic; semantic != "" {
				t.Errorf("DnsServerList semantic = %q, want none", semantic)
			}
		})
	}
}

// 端末が 1 つの相手へ始めた接続を記録したイベントだけが外向きの接続になる。
func TestObserveMarksTheOutboundConnections(t *testing.T) {
	const security = "Microsoft-Windows-Security-Auditing"
	const sysmon = "Microsoft-Windows-Sysmon"
	for _, tc := range []struct {
		name     string
		document string
		want     bool
	}{
		{"explicit credential request", providerEvent(security, "4648", "IpAddress", "192.0.2.20"), true},
		{"outbound permit", providerEvent(security, "5156",
			"Direction", "%%14593", "DestAddress", "192.0.2.20", "Protocol", "6"), true},
		{"inbound permit", providerEvent(security, "5156",
			"Direction", "%%14592", "DestAddress", "192.0.2.20", "Protocol", "6"), false},
		{"outbound multicast", providerEvent(security, "5156",
			"Direction", "%%14593", "DestAddress", "233.252.0.2", "Protocol", "17"), false},
		{"outbound broadcast", providerEvent(security, "5156",
			"Direction", "%%14593", "DestAddress", "192.0.2.255", "Protocol", "17"), false},
		{"outbound tcp to the last address", providerEvent(security, "5156",
			"Direction", "%%14593", "DestAddress", "192.0.2.255", "Protocol", "6"), true},
		{"sysmon initiated", providerEvent(sysmon, "3",
			"Initiated", "true", "DestinationIp", "192.0.2.20", "Protocol", "tcp"), true},
		{"sysmon broadcast", providerEvent(sysmon, "3",
			"Initiated", "true", "DestinationIp", "192.0.2.255", "Protocol", "udp"), false},
		{"sysmon accepted", providerEvent(sysmon, "3",
			"Initiated", "false", "DestinationIp", "192.0.2.20", "Protocol", "tcp"), false},
		{"logon", providerEvent(security, "4624", "IpAddress", "192.0.2.20", "LogonType", "3"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := observe(t, tc.document).OutboundConnection; got != tc.want {
				t.Errorf("OutboundConnection = %v, want %v", got, tc.want)
			}
		})
	}
}
