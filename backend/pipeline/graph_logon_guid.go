package pipeline

import (
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// nullLogonGuid は、Kerberos のチケットを使わないログオン (NTLM など) と失敗した要求が書く、
// 全桁 0 の LogonGuid の比べる形である。
const nullLogonGuid = "00000000-0000-0000-0000-000000000000"

// addTicketRequestLogonEdges は、Kerberos のチケットの要求のレコードから、同じ LogonGuid の
// ログオンのレコードへの候補のエッジを足す。
//
// **LogonGuid の一致だけで結ぶ。** 要求の event.ticket_logon_guid とログオンの
// event.target_logon_guid が、前後の中括弧を外し大文字と小文字を区別せずに一致する組が候補
// である。チケットを発行した端末とチケットを使った端末は違うため、端末を比べない。全桁 0 の
// LogonGuid は一致の根拠にしない。
//
// 既知の制限: 時刻を比べない, 端末をまたぐ時計のずれを補正する値を持たず、チケットは有効な間に
// 何度も使われる。LogonGuid は 128 bit の値であり偶然には一致しない。要求 R 件とログオン N 件の
// GUID は R×N 本のエッジを作る。1 つの GUID の要求 100 件×ログオン 2,000 件で、20 万本を
// 0.64 秒・366 MB で組む,
// LogonGuid が一致しながら無関係な組を記録した入力を確認したとき
func (g *Graph) addTicketRequestLogonEdges(result ImportResult) {
	requests := make(map[string][]int)
	var logons []struct {
		guid string
		at   int
	}
	for _, publication := range result.publications {
		if publication.status.PublicationState == core.PublicationStateWithheld {
			continue
		}
		for _, record := range publication.records {
			fields := graphFieldsOf(record)
			requested, isRequest := logonGuidOf(fields, core.SemanticKeyEventTicketLogonGuid)
			logged, isLogon := logonGuidOf(fields, core.SemanticKeyEventTargetLogonGuid)
			if !isRequest && !isLogon {
				continue
			}
			at, indexed := g.recordAtLocator(record.Locator)
			if !indexed || !g.records[at].hasRecordNode {
				continue
			}
			if isRequest {
				requests[requested] = append(requests[requested], at)
			}
			if isLogon {
				logons = append(logons, struct {
					guid string
					at   int
				}{logged, at})
			}
		}
	}
	for _, logon := range logons {
		for _, request := range requests[logon.guid] {
			if request == logon.at {
				continue
			}
			edge := g.ensureEdge(core.EdgeKindTicketRequestLogon, core.RelationStateCandidate,
				g.records[request].recordNode, g.records[logon.at].recordNode)
			g.addEdgeEvidence(edge, request)
			g.addEdgeEvidence(edge, logon.at)
			g.addRecordPair(edge, pairRuleTicketLogon, request, logon.at)
		}
	}
}

// logonGuidOf は、語彙の項目が持つ LogonGuid を、前後の中括弧を外した小文字の文字列で返す。
// ok が偽になるのは、値を比べられないときと、全桁 0 の値のときである。
func logonGuidOf(fields []core.RecordField, semantic core.SemanticKey) (string, bool) {
	raw, readable := comparableOfSemantic(fields, semantic)
	if !readable {
		return "", false
	}
	guid := strings.ToLower(raw)
	if strings.HasPrefix(guid, "{") && strings.HasSuffix(guid, "}") {
		guid = guid[1 : len(guid)-1]
	}
	if guid == "" || guid == nullLogonGuid {
		return "", false
	}
	return guid, true
}
