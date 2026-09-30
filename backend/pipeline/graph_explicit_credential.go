package pipeline

import (
	"net/netip"
	"strings"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// explicitCredentialTolerance は、資格情報を使ったログオンの要求と、接続先の端末のログオンの
// 時刻の差の許容幅である。
//
// 既知の制限: 許容幅を 2 秒に固定する。幅を広げると、同じアカウントと接続先の別の要求を
// 結ぶ, 端末の時計のずれを補正する値を持たない, 時計のずれた端末の組を結ぶ要求が出たとき、関連付けの条件と同じ選択に移す
const explicitCredentialTolerance = 2 * time.Second

// logonAtTerminal は、端末 1 台がログオンした 1 つのアカウントの名前を指す鍵である。
type logonAtTerminal struct {
	terminalNodeId string
	account        string
}

// addExplicitCredentialLogonEdges は、資格情報を使ったログオンの要求のレコードから、接続先の
// 端末が記録したログオンのレコードへの候補のエッジを足す。
//
// **要求は、接続先のアドレスか接続先の端末の名前 (connection.destination_server_name) と、
// 資格情報のアカウント (target_account.name) を記録し、ログオンを記録しないレコードである。**
// 接続先の端末は、要求の時刻にそのアドレスを持つ端末の割当が導く。アドレスが端末を導かない
// ときは、要求の時刻にその名前を記録した割当が 1 台だけの端末を導くとき、その端末である
// (NamesHostname)。ログオンは、その端末に置いたレコードのうち、接続元のアドレスの欄を持つ
// ログオンの成功のレコード (recordsLogon、失敗の状態のコードを持たない) であり、ログオンした
// アカウントの名前が大文字と小文字を区別せずに一致し、時刻の差が許容幅の中にあるものである。
//
// ループバック・リンクローカルの接続先と、要求を記録した端末自身へ導いた割当は結ばない。
//
// 既知の制限: 短い名前だけを記録した割当は、別のドメインの同じ短い名前の端末の要求も 1 台の
// 端末へ導く, 割当はドメインを記録しない, 別のドメインの同名の端末を結んだ入力を確認した
// とき、割当に FQDN を記録させる
//
// 既知の制限: アカウントのドメインを比べない。要求は資格情報のドメインを `.` や短い名前で書き、
// ログオンは接続先の端末の名前で書くため、同じアカウントでも文字列が揃わない。別の端末の同じ
// 名前のローカルアカウントも、割当と許容幅の条件を満たせば結ぶ, ドメインの文字列を寄せる規則を
// 持たない, 同じ名前の別のアカウントを結んだ入力を確認したとき
func (g *Graph) addExplicitCredentialLogonEdges(result ImportResult) {
	assignments := terminalAssignmentsOf(result).comparableEntries()
	logons := make(map[logonAtTerminal][]int)
	var requests []RecordEntry
	for _, publication := range result.publications {
		if publication.status.PublicationState == core.PublicationStateWithheld {
			continue
		}
		for _, record := range publication.records {
			fields := graphFieldsOf(record)
			account, named := comparableOfSemantic(fields, core.SemanticKeyTargetAccountName)
			if !named || account == "" {
				continue
			}
			if recordsLogon(record, fields) {
				g.indexSuccessfulLogon(logons, record, fields, account)
				continue
			}
			if len(fieldsWithSemantic(fields, core.SemanticKeyConnectionDestinationAddress)) > 0 ||
				carriesSemanticValue(fields, core.SemanticKeyConnectionDestinationServerName) {
				requests = append(requests, record)
			}
		}
	}
	if len(requests) == 0 {
		return
	}
	named := namedAssignmentsOf(result)
	for _, request := range requests {
		g.addExplicitCredentialLogonEdgesOf(request, assignments, named, logons)
	}
}

// requestDestinations は、要求の接続先の端末のノードの識別鍵を返す (addExplicitCredentialLogonEdges)。
func requestDestinations(
	request RecordEntry, fields []core.RecordField, assignments []core.TerminalAssignment, named []namedAssignment,
) []core.NodeKey {
	var keys []core.NodeKey
	destination, _ := comparableOfSemantic(fields, core.SemanticKeyConnectionDestinationAddress)
	if address, err := netip.ParseAddr(destination); err == nil {
		if address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsUnspecified() {
			return nil
		}
		resolution, err := core.ResolveTerminal(core.TerminalResolutionInput{
			ClientIp: destination, EventTime: *request.ObservedAt, Assignments: assignments,
		})
		if err != nil {
			return nil
		}
		for _, member := range resolution.Members {
			if member.AppliesToSourceId != "" && member.AppliesToSourceId == request.Locator.SourceId {
				continue
			}
			if key, built := member.TerminalNodeKey(); built {
				keys = append(keys, key)
			}
		}
		// IP の割当が一致した要求は、記録した端末自身を指す場合も名前で探し直さない。
		if len(resolution.Members) > 0 {
			return keys
		}
	}
	name, hasName := comparableOfSemantic(fields, core.SemanticKeyConnectionDestinationServerName)
	if !hasName {
		return nil
	}
	if terminals := assignedTerminalsNaming(named, name, request.ObservedAt); len(terminals) == 1 {
		return terminals
	}
	return nil
}

// indexSuccessfulLogon は、接続元のアドレスの欄を持ち、失敗の状態のコードを持たないログオンの
// レコードを、置いた端末とアカウントの名前で探せる表に入れる。
func (g *Graph) indexSuccessfulLogon(
	logons map[logonAtTerminal][]int, record RecordEntry, fields []core.RecordField, account string,
) {
	if len(fieldsWithSemantic(fields, core.SemanticKeyConnectionSourceAddress)) == 0 ||
		len(fieldsWithSemantic(fields, core.SemanticKeyEventLogonFailureStatus)) > 0 {
		return
	}
	if at, indexed := g.recordAtLocator(record.Locator); indexed && g.records[at].hasRecordNode {
		key := logonAtTerminal{g.records[at].placedTerminalNodeId, strings.ToLower(account)}
		logons[key] = append(logons[key], at)
	}
}

// addExplicitCredentialLogonEdgesOf は要求のレコード 1 件が作る候補のエッジを足す。
func (g *Graph) addExplicitCredentialLogonEdgesOf(
	request RecordEntry, assignments []core.TerminalAssignment, named []namedAssignment,
	logons map[logonAtTerminal][]int,
) {
	at, indexed := g.recordAtLocator(request.Locator)
	if !indexed || !g.records[at].hasRecordNode || !g.records[at].hasInstant || request.ObservedAt == nil {
		return
	}
	fields := graphFieldsOf(request)
	account, _ := comparableOfSemantic(fields, core.SemanticKeyTargetAccountName)
	for _, key := range requestDestinations(request, fields, assignments, named) {
		if nodeIdOf(key) == g.records[at].placedTerminalNodeId {
			continue
		}
		for _, logon := range logons[logonAtTerminal{nodeIdOf(key), strings.ToLower(account)}] {
			if !g.records[logon].hasInstant ||
				g.records[logon].instant.Sub(g.records[at].instant).Abs() > explicitCredentialTolerance {
				continue
			}
			edge := g.ensureEdge(core.EdgeKindExplicitCredentialLogon, core.RelationStateCandidate,
				g.records[at].recordNode, g.records[logon].recordNode)
			g.addEdgeEvidence(edge, at)
			g.addEdgeEvidence(edge, logon)
			g.addRecordPair(edge, pairRuleExplicitCredential, at, logon)
		}
	}
}
