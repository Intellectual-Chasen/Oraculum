package pipeline

import (
	"net/netip"
	"slices"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// addTerminalOutboundEdges は、端末が始めた接続を記録したレコード (RecordSemantics.OutboundConnection)
// から、レコードを記録した端末から接続先の IP アドレスへのエッジを足す。
//
// **端末とアドレスの図に接続を表示するための関係である。** プロセスを記録したレコードは
// process_communication も持つが、その起点はプロセスであり、端末とアドレスだけを辿る要求に
// 表示されない。
//
// **記録した端末が持つアドレスへは作らない。** 呼ぶのは端末のアドレスの関係を全て足した後である。
func (g *Graph) addTerminalOutboundEdges(result ImportResult, terminals sourceTerminals) {
	seen := make(map[int]map[transcriptKey]struct{})
	for _, publication := range result.publications {
		if publication.status.PublicationState == core.PublicationStateWithheld {
			continue
		}
		transcriptItems := publication.TranscriptIdentityItems()
		for _, record := range publication.records {
			if record.Semantics == nil || !record.Semantics.OutboundConnection {
				continue
			}
			g.addTerminalOutboundEdgesOfRecord(seen, terminals, record, transcriptItems)
		}
	}
}

// addTerminalOutboundEdgesOfRecord はレコード 1 件が作る外向きの接続のエッジを足す。
func (g *Graph) addTerminalOutboundEdgesOfRecord(
	seen map[int]map[transcriptKey]struct{}, terminals sourceTerminals,
	record RecordEntry, transcriptItems []string,
) {
	supplied, scope := terminals.forRecord(record)
	if len(supplied) > 0 {
		record.Terminal = append(slices.Clone(record.Terminal), supplied...)
	}
	fields := graphFieldsOf(record)
	terminal, built := core.RecordTerminalNodeKey(fields, scope)
	if !built {
		return
	}
	terminalIndex, present := g.nodeAt[nodeIdOf(terminal)]
	if !present {
		return
	}
	at, indexed := g.recordAtLocator(record.Locator)
	if !indexed {
		return
	}
	for _, field := range fieldsWithSemantic(fields, core.SemanticKeyConnectionDestinationAddress) {
		if field.Text == nil {
			continue
		}
		destination, comparable := field.Text.ComparableValue()
		if !comparable || !reachesAnotherTerminal(destination) {
			continue
		}
		ipIndex, present := g.nodeAt[nodeIdOf(core.NodeKey{
			Kind: core.NodeKindIp, Form: core.NodeKeyFormAddress,
			Values: []core.NodeIdentityValue{{Value: destination}},
		})]
		if !present {
			continue
		}
		if _, owned := g.edgeIndexOf(edgeIdOf(core.EdgeKindTerminalAddress,
			g.nodes[terminalIndex].id, g.nodes[ipIndex].id)); owned {
			continue
		}
		edge := g.ensureEdge(core.EdgeKindTerminalOutboundConnection, core.RelationStateObserved,
			terminalIndex, ipIndex)
		if countsAsNewEvidence(seen, edge, g.nodes[terminalIndex].id, fields, transcriptItems) {
			g.addEdgeEvidence(edge, at)
		}
	}
}

// reachesAnotherTerminal は、接続先のアドレスが 1 つの別の端末を指しうるかを返す。
// ループバック・リンクローカル・未指定のアドレスは端末の中の接続であり、マルチキャストと
// 限定ブロードキャストのアドレスは宛先を 1 台に決めていない。読めない文字列は偽である。
func reachesAnotherTerminal(text string) bool {
	address, err := netip.ParseAddr(text)
	if err != nil {
		return false
	}
	address = address.Unmap()
	return !address.IsLoopback() && !address.IsLinkLocalUnicast() && !address.IsLinkLocalMulticast() &&
		!address.IsUnspecified() && !address.IsMulticast() &&
		address != netip.AddrFrom4([4]byte{255, 255, 255, 255})
}
