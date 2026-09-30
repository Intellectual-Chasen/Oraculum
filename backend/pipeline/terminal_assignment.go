package pipeline

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// terminalAssignmentKey は同じ割当を 1 件にまとめる鍵である。
// 収集元ごとに適用期間が決まるため、鍵は sourceId を含む。
type terminalAssignmentKey struct {
	sourceId   string
	clientIp   string
	terminalId string
}

// terminalAssignments はまとめた割当と、鍵ごとに観測した端末の表示名である。
type terminalAssignments struct {
	// entries はまとめた割当である。
	entries []core.TerminalAssignment
	// hostnames はまとめる鍵ごとに観測した表示名を、走査順で重複無く持つ。
	//
	// **要素数が 2 以上の鍵は、同じ端末に別の表示名が付いた組である。** まとめる鍵に表示名を
	// 入れないため、まとめた割当が持つのは先に走査した 1 つだけになる。除いた表示名が
	// あることを応答に出せるよう、観測した全数を本項目が保つ。
	hostnames map[terminalAssignmentKey][]string
}

// terminalAssignmentsOf は取り込み結果から IP から端末への割当を組む。
//
// 材料は公開中の収集元のレコードが持つ端末の項目である。**語彙の項目で探す。**
// terminal.id が端末の外部識別子、terminal.hostname が分析者の読む表示名、
// terminal.ip_address が端末の持つ IP である。IP の要素 1 つにつき割当 1 件を作り、
// 鍵 (収集元の sourceId、IP、terminal.id) が同じ組を 1 件にまとめる。並びは収集元の
// 取り込みの入力順、同じ収集元の中はレコードの走査順である。
//
// **表示名をまとめる鍵に入れない。** 同じ表示名を持つ別の端末を 1 件にまとめないためである。
// **同じ鍵に別の表示名が付いた組は、表示名を集めて残す。** 集めた表示名は
// FieldsBuilder が読み、clientTerminalName の表示名を 1 つに確定させない。
//
// **割当を適用してよい期間は、その収集元の観測期間である。** 期間の両端は
// observedRangeFirst と observedRangeLast が持つ。
//
// **次の 3 つに該当する収集元と組から割当を作らない。** どれも割当を適用してよい期間または
// 割当の識別を確定できず、core.TerminalAssignment の検査を通らない値になる。
// 作らなかった収集元は、導出できなかった理由が持つ「参照した収集元」にも入らない。
//
//   - 観測期間の両端の 1 つ以上を持たない収集元 (時刻を持つレコードが 0 件である)
//   - 観測期間の両端の前後が逆である収集元 (最初のレコードの時刻が最後のレコードの
//     時刻より後にある)
//   - 収集元の識別、端末の外部識別子、端末の表示名の 1 つ以上を欠く組
func terminalAssignmentsOf(result ImportResult) terminalAssignments {
	collected := terminalAssignments{
		entries:   make([]core.TerminalAssignment, 0),
		hostnames: make(map[terminalAssignmentKey][]string),
	}
	for _, publication := range result.publications {
		if publication.status.PublicationState == core.PublicationStateWithheld {
			continue
		}
		identity, found := result.Identity(publication.status.SourceId)
		if !found {
			continue
		}
		// 地方時の収集元は、分析者の時刻の解釈で読んだ収録範囲を割当の期間にする。
		identity = result.withObservedRange(identity)
		validRange, usable := applicableRangeOf(identity)
		if !usable {
			continue
		}
		for _, record := range publication.records {
			recordTerminalAssignments(record, identity, validRange, &collected)
		}
	}
	// 地方時の期間を持つ利用者の割当は、期間を読み取った収集元の今の解釈で読む。
	supplied := result.userAssignments()
	for index, assignment := range supplied {
		supplied[index] = result.withInterpretedRange(assignment)
	}
	collected.addUserAssignments(supplied)
	return collected
}

// addUserAssignments は利用者が入力した割当を、収集元から読んだ割当の後ろに足す。
//
// **収集元から読んだ割当を利用者の割当で置き換えない。** 同じ鍵の組が両方あるときは、
// 2 件の候補として並べる。どちらが正しいかを取り込みの実行が決めない。
//
// **接続元 IP を持たない割当を足さない。** 本型の割当は接続元 IP から端末を探す材料である。
//
// **表示名の集合には入れない。** 集合が表すのは収集元が観測した表示名であり、
// 利用者の与えた表示名を観測に混ぜない。
func (a *terminalAssignments) addUserAssignments(supplied []core.TerminalAssignment) {
	for _, assignment := range supplied {
		if assignment.ClientIp == "" || assignment.Validate() != nil {
			continue
		}
		a.entries = append(a.entries, assignment)
	}
}

// hostnamesOf は割当 1 件のまとめる鍵に対して観測した表示名を返す。
func (a terminalAssignments) hostnamesOf(assignment core.TerminalAssignment) []string {
	return a.hostnames[terminalAssignmentKey{
		sourceId:   assignment.SourceId,
		clientIp:   assignment.ClientIp,
		terminalId: assignment.TerminalId,
	}]
}

// comparableEntries は、期間の両端を時点として読める割当だけを、entries の並びで返す。
//
// **遠隔のセッションの候補を足す処理だけが使う。** 解釈を取り消した収集元の地方時の期間の
// ように、両端を時点として読めない割当を core.ResolveTerminal に渡すと、判定は 1 件で
// time_not_comparable を返し、同じ IP のほかの割当の候補も作らない。遠隔のセッションは割当
// ごとに候補を作るため、その割当だけが候補を作らないよう、判定の前に外す。
//
// 端末を 1 つに決める判定 (FieldsBuilder の clientTerminal と、関連付けの起点の端末) は、比べ
// られない割当があれば端末を確定させないため、本 method を使わない。
func (a terminalAssignments) comparableEntries() []core.TerminalAssignment {
	comparable := make([]core.TerminalAssignment, 0, len(a.entries))
	for _, assignment := range a.entries {
		_, fromReadable := assignment.AssignmentValidRange.From.Instant()
		_, toReadable := assignment.AssignmentValidRange.To.Instant()
		if fromReadable && toReadable {
			comparable = append(comparable, assignment)
		}
	}
	return comparable
}

// applicableRangeOf は収集元の観測期間を、割当を適用してよい期間として返す。
// ok が偽になるのは、両端の 1 つ以上を持たない収集元と、両端の前後が逆である収集元である。
func applicableRangeOf(identity core.SourceIdentity) (core.TimeRange, bool) {
	if identity.ObservedRangeFirst == nil || identity.ObservedRangeLast == nil {
		return core.TimeRange{}, false
	}
	validRange := core.TimeRange{
		From: cloneTimestamp(*identity.ObservedRangeFirst),
		To:   cloneTimestamp(*identity.ObservedRangeLast),
	}
	if err := validRange.Validate(); err != nil {
		return core.TimeRange{}, false
	}
	return validRange, true
}

// recordTerminalAssignments はレコード 1 件の端末の項目から割当を作り、collected へ足す。
// 既に鍵を持つ組の割当を作り直さず、検査を通らない組を除く。
func recordTerminalAssignments(
	record RecordEntry, identity core.SourceIdentity, validRange core.TimeRange,
	collected *terminalAssignments,
) {
	terminalId, hasId := comparableOfSemantic(record.Terminal, core.SemanticKeyTerminalId)
	hostname, hasHostname := comparableOfSemantic(record.Terminal, core.SemanticKeyTerminalHostname)
	if !hasId || !hasHostname {
		return
	}
	for _, field := range fieldsWithSemantic(record.Terminal, core.SemanticKeyTerminalIpAddress) {
		if field.Text == nil {
			continue
		}
		clientIp, readable := field.Text.ComparableValue()
		if !readable {
			continue
		}
		key := terminalAssignmentKey{
			sourceId: identity.SourceId, clientIp: clientIp, terminalId: terminalId,
		}
		assignment := core.TerminalAssignment{
			ClientIp: clientIp, TerminalId: terminalId, TerminalHostname: hostname,
			SourceId:            identity.SourceId,
			SourceContentSha256: identity.ContentSha256,
			AssignmentValidRange: core.TimeRange{
				From: cloneTimestamp(validRange.From), To: cloneTimestamp(validRange.To),
			},
			// 接続元 IP と端末を同じレコードが記録している。
			Origin: core.TerminalAssignmentOriginObservedInSource,
		}
		// 収集元の識別と端末の 2 項目を欠く組は、割当の識別を確定できない。
		if err := assignment.Validate(); err != nil {
			continue
		}
		observed, taken := collected.hostnames[key]
		if !taken {
			collected.entries = append(collected.entries, assignment)
		}
		if !contains(observed, hostname) {
			collected.hostnames[key] = append(observed, hostname)
		}
	}
}
