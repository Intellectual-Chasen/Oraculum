package pipeline

import (
	"cmp"
	"net/netip"
	"slices"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// ProxyBypassDestination は、接続元のアドレス 1 つから Proxy を経由せずに出た接続の接続先 1 つである。
type ProxyBypassDestination struct {
	// Address と Port は接続先の比べられる形の文字列である。port を持たない記録では Port が空である。
	Address string
	Port    string
	// RecordCount はこの接続先への接続を記録したレコードの件数である。
	RecordCount int64
}

// ProxyBypassClient は、接続元のアドレス 1 つについて、Proxy のログが記録した要求と、
// 他の収集元のレコードが記録した接続を並べた件数である。
//
// **件数の単位はレコードである。** 1 つの接続を開始と終了の 2 レコードで記録した収集元や、
// 同じ接続を端末とネットワーク機器の両方が記録した場合は、接続 1 つを複数件と数える。
// **割当の適用期間とレコードの時刻を比べない。** 件数はアドレス単位であり、Terminals は
// 期間を問わずこのアドレスに結んだ端末をすべて並べる。
type ProxyBypassClient struct {
	// ClientIp は接続元のアドレスの比べられる形の文字列である。
	ClientIp string
	// Terminals は、利用者が与えた端末の割当がこのアドレスに結んだ端末の識別子である。
	// 識別子を持たない割当は表示名で出す。文字列の順に並ぶ。
	Terminals []string
	// ProxyRequestCount は Proxy のログがこのアドレスから受けた要求を記録したレコードの件数である。
	ProxyRequestCount int64
	// ProxyConnectionCount は、他の収集元のレコードのうち、このアドレスから Proxy の
	// アドレスへの接続を記録したレコードの件数である。
	ProxyConnectionCount int64
	// DirectConnectionCount は、他の収集元のレコードのうち、このアドレスから Proxy の
	// アドレス以外への接続を記録したレコードの件数である。DirectDestinations の件数の和である。
	DirectConnectionCount int64
	// DirectConnectionCountable は、このアドレスの接続を記録する収集元があるかである。
	// Proxy のアドレスが無いときは常に偽である。偽のとき DirectConnectionCount と
	// ProxyConnectionCount の 0 は、数えた結果ではない。真になるのは、Proxy 以外の収集元の
	// レコードがこのアドレスからの接続を 1 件以上記録したときと、このアドレスか割当の端末に、
	// 接続元と接続先のアドレスを持つレコードを 1 件以上持つ Proxy 以外の収集元を割り当てたとき
	// である。
	DirectConnectionCountable bool
	// DirectDestinations は Proxy のアドレス以外の接続先ごとの件数である。件数の多い順に並び、
	// 同じ件数はアドレス、port の文字列の順に並ぶ。
	DirectDestinations []ProxyBypassDestination
}

// ProxyBypass は Proxy のログの収集元 1 つについて、接続元のアドレスごとの要求と接続を並べる。
type ProxyBypass struct {
	// ProxyAddresses は Proxy のログの収集元に付けた端末の IP アドレスである。要素数 0 のときは
	// Proxy への接続と直接の接続を分けられず、Clients は Proxy のログの要求だけを数える。
	ProxyAddresses []string
	// UnreadableProxyRecordCount は、接続元を IP アドレスとして読めず、Clients のどこにも
	// 数えなかった Proxy のログのレコードの件数である。
	UnreadableProxyRecordCount int64
	// Clients は接続元のアドレスの文字列の順に並ぶ。
	Clients []ProxyBypassClient
}

// proxyBypassCounter は ProxyBypassOf が数える途中の件数を持つ。
type proxyBypassCounter struct {
	clients map[string]*ProxyBypassClient
	direct  map[string]map[[2]string]int64
	// connectionSources は、接続元と接続先の両方のアドレスを持つレコードを 1 件以上持つ
	// 収集元の sourceId である。割当で数えられるとするのは、この収集元を割り当てたときだけである。
	connectionSources map[string]bool
}

func (c *proxyBypassCounter) clientOf(address string) *ProxyBypassClient {
	client, seen := c.clients[address]
	if !seen {
		client = &ProxyBypassClient{ClientIp: address, Terminals: []string{}}
		c.clients[address] = client
	}
	return client
}

// ProxyBypassOf は、Proxy のログの収集元 sourceId について、接続元のアドレスごとに Proxy が
// 記録した要求の件数と、他の収集元のレコードが記録した直接の接続の件数を返す。ok が偽になるのは、
// sourceId の公開された収集元が無いときと、その収集元が Proxy のログでないときである。
//
// **接続元にするアドレスは、Proxy のログの要求の接続元と、端末の割当のアドレスである。**
// 他の収集元の接続を、その接続元がこのどちらかであるときだけ数える。外から受けた接続の
// 接続元を端末として並べない。Proxy のログの収集元と、他の Proxy のログの収集元の
// レコードは、接続を数える側に入れない。
func (r ImportResult) ProxyBypassOf(sourceId string) (ProxyBypass, bool) {
	proxy, found := r.Publication(sourceId)
	if !found || !proxy.parser.ProxyRequestLog {
		return ProxyBypass{}, false
	}
	proxyAddresses := sourceTerminalAddressesOf(r)[sourceId]
	counter := proxyBypassCounter{
		clients:           make(map[string]*ProxyBypassClient),
		direct:            make(map[string]map[[2]string]int64),
		connectionSources: make(map[string]bool),
	}
	bypass := ProxyBypass{ProxyAddresses: slices.Clone(proxyAddresses)}
	if bypass.ProxyAddresses == nil {
		bypass.ProxyAddresses = []string{}
	}
	bypass.UnreadableProxyRecordCount = counter.countProxyRequests(proxy)
	counter.collectAssignedTerminals(r.userAssignments(), proxyAddresses)
	if len(proxyAddresses) > 0 {
		for _, publication := range r.publications {
			if publication.status.PublicationState != core.PublicationStateWithheld &&
				!publication.parser.ProxyRequestLog {
				counter.countConnections(publication, proxyAddresses)
			}
		}
		counter.markCountable(r)
	}
	bypass.Clients = counter.sortedClients()
	return bypass, true
}

// markCountable は、接続を記録する収集元がある接続元に DirectConnectionCountable を立てる
// (ProxyBypassClient.DirectConnectionCountable)。割当で被覆するのは、接続のレコードを持つ
// 収集元 (connectionSources) の割当だけである。
func (c *proxyBypassCounter) markCountable(r ImportResult) {
	coveredAddresses := make(map[string]bool)
	coveredTerminals := make(map[string]bool)
	for _, assignment := range r.userAssignments() {
		if !c.connectionSources[assignment.AppliesToSourceId] {
			continue
		}
		if address, err := netip.ParseAddr(assignment.ClientIp); err == nil {
			coveredAddresses[address.String()] = true
		}
		if terminal := cmp.Or(assignment.TerminalId, assignment.TerminalHostname); terminal != "" {
			coveredTerminals[terminal] = true
		}
	}
	for address, client := range c.clients {
		client.DirectConnectionCountable = client.ProxyConnectionCount > 0 ||
			client.DirectConnectionCount > 0 || coveredAddresses[address] ||
			slices.ContainsFunc(client.Terminals, func(terminal string) bool { return coveredTerminals[terminal] })
	}
}

// countProxyRequests は Proxy のログの要求を接続元ごとに数え、接続元を読めないレコードの件数を返す。
func (c *proxyBypassCounter) countProxyRequests(proxy SourcePublication) int64 {
	var unreadable int64
	for _, record := range proxy.records {
		if address, ok := recordAddress(record, core.SemanticKeyConnectionSourceAddress); ok {
			c.clientOf(address).ProxyRequestCount++
		} else {
			unreadable++
		}
	}
	return unreadable
}

// collectAssignedTerminals は割当のアドレスを接続元に足し、割当の端末を並べる。Proxy 自身の
// アドレスの割当は接続元にしない。
func (c *proxyBypassCounter) collectAssignedTerminals(assignments []core.TerminalAssignment, proxyAddresses []string) {
	for _, assignment := range assignments {
		address, err := netip.ParseAddr(assignment.ClientIp)
		if err != nil || slices.Contains(proxyAddresses, address.String()) {
			continue
		}
		terminal := cmp.Or(assignment.TerminalId, assignment.TerminalHostname)
		client := c.clientOf(address.String())
		if terminal != "" && !slices.Contains(client.Terminals, terminal) {
			client.Terminals = append(client.Terminals, terminal)
		}
	}
}

// countConnections は収集元 publication のレコードのうち、既知の接続元からの接続を数える。
func (c *proxyBypassCounter) countConnections(publication SourcePublication, proxyAddresses []string) {
	for _, record := range publication.records {
		from, hasFrom := recordAddress(record, core.SemanticKeyConnectionSourceAddress)
		to, hasTo := recordAddress(record, core.SemanticKeyConnectionDestinationAddress)
		if hasFrom && hasTo {
			c.connectionSources[publication.status.SourceId] = true
		}
		client, known := c.clients[from]
		if !hasFrom || !hasTo || !known {
			continue
		}
		if slices.Contains(proxyAddresses, to) {
			client.ProxyConnectionCount++
			continue
		}
		port, _ := comparableOfSemantic(recordFields(record), core.SemanticKeyConnectionDestinationPort)
		if c.direct[from] == nil {
			c.direct[from] = make(map[[2]string]int64)
		}
		c.direct[from][[2]string{to, port}]++
		client.DirectConnectionCount++
	}
}

// sortedClients は接続元を文字列の順に、接続先を件数の多い順に並べて返す。
func (c *proxyBypassCounter) sortedClients() []ProxyBypassClient {
	clients := make([]ProxyBypassClient, 0, len(c.clients))
	for address, client := range c.clients {
		slices.Sort(client.Terminals)
		client.DirectDestinations = make([]ProxyBypassDestination, 0, len(c.direct[address]))
		for destination, count := range c.direct[address] {
			client.DirectDestinations = append(client.DirectDestinations, ProxyBypassDestination{
				Address: destination[0], Port: destination[1], RecordCount: count,
			})
		}
		slices.SortFunc(client.DirectDestinations, func(left, right ProxyBypassDestination) int {
			return cmp.Or(cmp.Compare(right.RecordCount, left.RecordCount),
				strings.Compare(left.Address, right.Address), strings.Compare(left.Port, right.Port))
		})
		clients = append(clients, *client)
	}
	slices.SortFunc(clients, func(left, right ProxyBypassClient) int {
		return strings.Compare(left.ClientIp, right.ClientIp)
	})
	return clients
}

// recordFields はレコードの意味付けの項目を返す。意味付けを持たないレコードでは nil を返す。
func recordFields(record RecordEntry) []core.RecordField {
	if record.Semantics == nil {
		return nil
	}
	return record.Semantics.Fields
}

// recordAddress は、語彙の項目 semantic のアドレスを比べられる形の文字列で返す。
// IP アドレスとして読めない文字列では ok が偽である。
func recordAddress(record RecordEntry, semantic core.SemanticKey) (string, bool) {
	text, ok := comparableOfSemantic(recordFields(record), semantic)
	if !ok {
		return "", false
	}
	address, err := netip.ParseAddr(text)
	if err != nil {
		return "", false
	}
	return address.String(), true
}
