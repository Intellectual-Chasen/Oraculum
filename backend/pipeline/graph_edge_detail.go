package pipeline

import (
	"log/slog"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// EdgeDetail はエッジ 1 本の詳細である。絞り込みを通した根拠と関連付けを全件含む。
//
// **両端のノードを一緒に返す。** 画面が別の要求の結果と突き合わせないためである。
// 部分グラフの応答が edges の両端を必ず nodes に持つのと同じ理由による。
type EdgeDetail struct {
	// Edge はエッジの識別と状態と根拠の件数である。
	Edge core.GraphEdge
	// Evidence は絞り込みを通った根拠のレコードである。
	//
	// **core.GraphEdge は持たない。** 根拠の中身を含むのは `/api/v0/edges/{id}` だけである。
	Evidence []core.GraphEvidence
	// SourceNode はエッジの起点のノードである。
	SourceNode core.GraphNode
	// TargetNode はエッジの終点のノードである。
	TargetNode core.GraphNode
	// MatchTable はエッジを作った関連付けである。観測から直に作ったエッジでは関連付けを持たない。
	MatchTable core.EdgeMatchTable
	// MatchCount は関連付けの総数である。
	MatchCount int
	// AssignmentBases は、接続元のアドレスから端末を導いて作った候補のエッジの成立の
	// 根拠である。用いた割当ごとに 1 件を持つ。他のエッジでは要素数 0 である。
	AssignmentBases []core.EdgeAssignmentBasis
	// TerminalAssignments は、エッジを作った端末の割当である。利用者が収集元に付けた割当の
	// IP から作った terminal_address のエッジだけが持つ。他のエッジでは要素数 0 である。
	TerminalAssignments []core.TerminalAssignment
	// ObservedInRecords は、TerminalAssignments を持つエッジを、レコードも直に観測したかである。
	// レコード自身の IP と割当が同じ端末と IP の関係を与えると、エッジは 1 本にまとめられる。
	// TerminalAssignments を持たないエッジでは偽である。
	ObservedInRecords bool
	// RecordPairs は、観測の層が候補のエッジを作ったレコードの組と、成立に用いた条件ごとの
	// 両側の値である。先頭から maxResponsePairs 組までを持つ。他のエッジでは要素数 0 である。
	RecordPairs []core.EdgeRecordPair
	// RecordPairCount はレコードの組の総数である。
	RecordPairCount int
	// EvidenceGroups は根拠を観測の種別と接続先 port とログオンの種別で分けた区分である。
	//
	// **絞り込みを通す前の全数から組む。** 1 つの区分だけを読む要求でも、他の区分が
	// 何件あるかを分析者が読める。
	EvidenceGroups []core.EdgeEvidenceGroup
}

// EdgeEvidenceFilter は、返す根拠を 1 つの区分へ絞る条件である。
// 空の文字列である項目では絞らない。
type EdgeEvidenceFilter struct {
	// EventCategory は観測の種別の category の比べられる値である。
	EventCategory string
	// EventAction は観測の種別の action の比べられる値である。
	EventAction string
	// DestinationPort は接続先 port の比べられる値である。
	DestinationPort string
	// DestinationPortAbsent は、接続先 port の欄を持たないレコードだけを通す条件である。
	// DestinationPort と同時に立てない。
	DestinationPortAbsent bool
	// LogonType はログオンの種別のコードの比べられる値である。
	LogonType string
	// LogonTypeAbsent は、ログオンの種別を持たないレコードだけを通す条件である。
	// LogonType と同時に立てない。
	LogonTypeAbsent bool
	// HttpStatus は HTTP の状態の比べられる値である。HttpStatusAbsent は、HTTP の状態の値を
	// 持たない HTTP の要求のレコードだけを通す条件である。2 つを同時に立てない。
	HttpStatus       string
	HttpStatusAbsent bool
	// Case は根拠のレコードの収集元に付けた案件である。
	Case string
}

// EdgeEvidenceFilterOf は、区分を指す値から絞り込みの条件を組む。
//
// **Validate を通らない組も条件になる。** 要求が項目を 1 つも与えない場合と、一部だけを
// 与える場合があり、空の文字列である項目では絞らない。api は要求と cursor が持つ項目を
// EdgeEvidenceSelector へ入れて本関数へ渡す。
func EdgeEvidenceFilterOf(selector core.EdgeEvidenceSelector) EdgeEvidenceFilter {
	return EdgeEvidenceFilter{
		EventCategory:         selector.EventCategory,
		EventAction:           selector.EventAction,
		DestinationPort:       selector.DestinationPort,
		DestinationPortAbsent: selector.DestinationPortAbsent,
		LogonType:             selector.LogonType,
		LogonTypeAbsent:       selector.LogonTypeAbsent,
		HttpStatus:            selector.HttpStatus,
		HttpStatusAbsent:      selector.HttpStatusAbsent,
	}
}

// filters は条件を 1 つでも持つかを返す。
func (f EdgeEvidenceFilter) filters() bool {
	return f.EventCategory != "" || f.EventAction != "" || f.DestinationPort != "" ||
		f.DestinationPortAbsent || f.LogonType != "" || f.LogonTypeAbsent ||
		f.HttpStatus != "" || f.HttpStatusAbsent || f.Case != ""
}

// EdgeDetail は識別子で指したエッジの詳細を返す。
// ok が偽になるのは、その識別子のエッジがグラフに無いときである。
//
// filter を与えた要求では、Evidence と Edge.EvidenceCount と
// Edge.ApplicableRange が条件を通った根拠だけから決まる。EvidenceGroups は条件に
// かかわらず全数から組む。
func (g Graph) EdgeDetail(id string, filter EdgeEvidenceFilter) (EdgeDetail, bool) {
	index, found := g.edgeIndexOf(id)
	if !found {
		return EdgeDetail{}, false
	}
	edge := g.edges[index]
	selected := g.evidenceInGroup(edge.evidence, filter)
	detail := EdgeDetail{
		Edge:                g.responseEdge(edge, selected),
		Evidence:            g.evidenceItems(selected),
		SourceNode:          g.subgraphNode(edge.source, core.NodeSelectionMatched, GraphQuery{}).GraphNode,
		TargetNode:          g.subgraphNode(edge.target, core.NodeSelectionMatched, GraphQuery{}).GraphNode,
		MatchTable:          g.edgeMatchTableOf(edge),
		MatchCount:          len(edge.matchList()),
		AssignmentBases:     cloneAssignmentBases(edge.assignmentBasisList()),
		TerminalAssignments: cloneAssignments(edge.terminalAssignmentList()),
		ObservedInRecords:   edge.basis != nil && edge.basis.observedInRecords,
		RecordPairs:         g.recordPairsOf(edge),
		RecordPairCount:     len(edge.pairList()),
		EvidenceGroups:      g.evidenceGroups(edge.evidence),
	}
	return detail, true
}

// evidenceInGroup は条件を通った根拠を、走査の順で返す。
func (g Graph) evidenceInGroup(evidence []int, filter EdgeEvidenceFilter) []int {
	if !filter.filters() {
		return evidence
	}
	passed := make([]int, 0, len(evidence))
	for _, at := range evidence {
		if g.recordInGroup(at, filter) {
			passed = append(passed, at)
		}
	}
	return passed
}

// recordInGroup はレコード 1 件が条件のすべてに一致するかを返す。
//
// **接続先 port とログオンの種別の条件は、欄を持たないレコードを通さない** (fieldInGroup)。
func (g Graph) recordInGroup(at int, filter EdgeEvidenceFilter) bool {
	record := g.records[at]
	if filter.EventCategory != "" && record.eventCategory != filter.EventCategory {
		return false
	}
	if filter.Case != "" && g.caseOfRecord(at) != filter.Case {
		return false
	}
	if filter.EventAction != "" && record.eventAction != filter.EventAction {
		return false
	}
	portState, port := destinationPortOf(record)
	if !fieldInGroup(portState, port, filter.DestinationPort, filter.DestinationPortAbsent) {
		return false
	}
	logonTypeState, logonType := logonTypeOf(record)
	if !fieldInGroup(logonTypeState, logonType, filter.LogonType, filter.LogonTypeAbsent) {
		return false
	}
	httpState, httpStatus, isHttp := carriedHttpStatusOf(record)
	filtersHttp := filter.HttpStatus != "" || filter.HttpStatusAbsent
	if !isHttp {
		return !filtersHttp
	}
	// 観測の種別を持たない HTTP の区分を指す値は、種別の条件を持たない。その値で観測の種別を
	// 持つレコードを通すと、別の区分のレコードが混ざる。
	if filtersHttp && filter.EventCategory == "" && filter.EventAction == "" &&
		(record.eventCategory != "" || record.eventAction != "") {
		return false
	}
	return fieldInGroup(httpState, httpStatus, filter.HttpStatus, filter.HttpStatusAbsent)
}

// carriedHttpStatusOf は、HTTP の要求のレコードの HTTP の状態の持ち方と比べられる値を返す。
// 要求先の URL を持たないレコードは HTTP の要求でなく、isHttp が偽である。値の不在を表す
// 文字列は、状態の値が無い状態にする。
func carriedHttpStatusOf(record graphRecord) (state carriedState, value string, isHttp bool) {
	if record.requestUrl == nil {
		return carriedFieldAbsent, "", false
	}
	state, value = carriedValueOf(record.httpStatus, carriedFieldAbsent)
	return state, value, true
}

// fieldInGroup は、レコードの項目 1 つの状態と比べられる値が、値の条件と欄の不在の条件に
// 一致するかを返す。2 つの条件のどちらも与えない要求では、どの項目も一致する。
//
// **値の条件は、欄を持たないレコードを通さない。** 欄の不在を指す条件は absent が運び、
// 欄はあるが値を比べられないレコードはどちらの条件にも一致しない。
func fieldInGroup(state carriedState, carried, value string, absent bool) bool {
	if absent {
		return state == carriedFieldAbsent
	}
	if value == "" {
		return true
	}
	return state == carriedReadable && carried == value
}

// carriedState は、レコードが区分を分ける項目 (接続先 port、ログオンの種別) をどう持っている
// かである。
type carriedState int

const (
	// carriedFieldAbsent は欄そのものが無い状態である。
	carriedFieldAbsent carriedState = iota
	// carriedUnreadable は欄はあるが値を比べられない状態である。
	carriedUnreadable
	// carriedReadable は比べられる値を読めた状態である。
	carriedReadable
)

// carriedValueOf はレコードの項目 1 つの状態と、比べられる値を返す。項目を持たないレコードでは
// field が nil である。absentValue は、値の不在を表す文字列 (値の状態が absent) を持つ項目を
// 置く状態である。
//
// **3 つの状態を分ける。** 欄の不在と、欄はあるが値を比べられない状態を 1 つにまとめると、
// 片方の集団に他方の理由が付く。
//
// 欄の不在は 2 つの形で届く。収集元がその `evt` で欄を宣言しないレコードは項目そのものを
// 持たず、宣言する欄に key が出ていないレコードは値の状態が item_absent の項目を持つ。
// **2 つは同じ「欄が無い」である** (recordField.ts の describeRawTextAbsence と同じ区別)。
// 原資料の文字列から値を導けなかったレコードは、欄はあるが値を比べられない状態である。
func carriedValueOf(field *core.RecordField, absentValue carriedState) (carriedState, string) {
	if field == nil || field.Text == nil || field.Text.ValueState == core.ValueStateItemAbsent {
		return carriedFieldAbsent, ""
	}
	if field.Text.ValueState == core.ValueStateAbsent {
		return absentValue, ""
	}
	value, readable := field.Text.ComparableValue()
	if !readable {
		return carriedUnreadable, ""
	}
	return carriedReadable, value
}

// destinationPortOf はレコードの接続先 port の状態と、比べられる値を返す。
//
// 値の不在を表す文字列を持つレコードは、欄はあるが値を比べられない状態である。接続先 port の
// 欄を持つ通信のレコードが、その値だけを書かなかった状態であり、欄を持たない種別の
// レコードと区分を分ける。
func destinationPortOf(record graphRecord) (carriedState, string) {
	return carriedValueOf(record.destinationPort, carriedUnreadable)
}

// logonTypeOf はレコードのログオンの種別の状態と、比べられる値を返す。
//
// **値の不在を表す文字列を持つレコードは、種別の値が無い状態である。** Windows イベントログは
// 種別を書かないログオンの LogonType に "-" を置き、markii 形式は logonType の値の不在から種別の
// 項目を作らない。2 つを同じ区分にする。
func logonTypeOf(record graphRecord) (carriedState, string) {
	return carriedValueOf(record.logonType, carriedFieldAbsent)
}

// destinationPortAbsentReason は、接続先 port を持たないレコードの区分が出す理由である。
const destinationPortAbsentReason = "区分のレコードが接続先 port の欄を持っていません"

// destinationPortUnreadableReason は、接続先 port の欄はあるが値を比べられない区分が出す
// 理由である。
const destinationPortUnreadableReason = "区分のレコードの接続先 port の値を比べられません"

// observationKindAbsentReason は、観測の種別の値を読めない区分が要求で指せない理由である。
//
// **欄の不在を断定しない。** 区分の鍵は観測の種別を 1 つの比べられる値にまとめるため、欄が
// 無いレコードと、欄はあるが値を比べられないレコードと、値が空の文字列であるレコードが
// 同じ鍵になる。3 つを分けずに「欄を持たない」と述べると、応答が誤った事実を出す。
const observationKindAbsentReason = "区分のレコードの観測の種別の値を読めないため、要求で指せません"

// destinationPortUnreadableSelectorReason は、接続先 port の値を比べられない区分が要求で
// 指せない理由である。
const destinationPortUnreadableSelectorReason = "区分のレコードの接続先 port の値を比べられないため、要求で指せません"

// destinationPortEmptySelectorReason は、接続先 port の値が空の文字列である区分が要求で
// 指せない理由である。
//
// **空の文字列と欄の不在を同じ条件で指せない。** 要求の項目は、空の文字列を「条件を与えない」
// と読む (EdgeEvidenceFilter の doc comment)。
const destinationPortEmptySelectorReason = "区分のレコードの接続先 port が空の文字列であるため、要求で指せません"

// logonTypeAbsentReason は、ログオンの種別の値を持たないレコードの区分が出す理由である。
// 種別の欄を持たないレコードと、欄に値の不在を表す文字列を持つレコードが入る (logonTypeOf)。
//
// **コード 0 と書かない。** 0 は Windows の定義が持つ種別のコードの 1 つである。
const logonTypeAbsentReason = "種別の値が無い。区分のレコードはログオンの種別の値を持ちません"

// logonTypeUnreadableReason は、ログオンの種別の欄はあるが値を比べられない区分が出す理由で
// ある。原資料の文字列から種別のコードを導けなかったレコードがこの区分に入る。
const logonTypeUnreadableReason = "区分のレコードのログオンの種別の値を比べられません"

// logonTypeUnreadableSelectorReason は、ログオンの種別の値を比べられない区分が要求で指せない
// 理由である。
const logonTypeUnreadableSelectorReason = "区分のレコードのログオンの種別の値を比べられないため、要求で指せません"

// logonTypeEmptySelectorReason は、ログオンの種別が空の文字列である区分が要求で指せない理由で
// ある。理由は destinationPortEmptySelectorReason と同じである。
const logonTypeEmptySelectorReason = "区分のレコードのログオンの種別が空の文字列であるため、要求で指せません"

// httpStatusAbsentReason と httpStatusUnreadableReason は、HTTP の要求のレコードの区分で
// HTTP の状態が出ない理由である。
const (
	httpStatusAbsentReason     = "区分の HTTP の要求のレコードは HTTP の状態の値を持ちません"
	httpStatusUnreadableReason = "区分の HTTP の要求のレコードの HTTP の状態の値を比べられません"
)

// httpStatusUnreadableSelectorReason と httpStatusEmptySelectorReason は、HTTP の状態の値を
// 比べられない区分と空の文字列である区分が要求で指せない理由である。
const (
	httpStatusUnreadableSelectorReason = "区分のレコードの HTTP の状態の値を比べられないため、要求で指せません"
	httpStatusEmptySelectorReason      = "区分のレコードの HTTP の状態が空の文字列であるため、要求で指せません"
)

// evidenceGroupSelectorUnbuildableReason は、区分を指す値を組めなかった区分が要求で
// 指せない理由である。**欄を挙げない。** 個別の分岐が挙げる理由に該当しない組み合わせで
// あり、どの欄が原因かをこの理由は述べない。
const evidenceGroupSelectorUnbuildableReason = "区分を指す値を組めないため、要求で指せません"

// evidenceGroupKey は根拠の区分 1 つを指す鍵である。
type evidenceGroupKey struct {
	eventCategory   string
	eventAction     string
	destinationPort string
	// portState は接続先 port の運び方である。
	// **欄の不在と、欄はあるが値を比べられない状態と、値 0 を別の鍵にする。**
	portState carriedState
	logonType string
	// logonTypeState はログオンの種別の運び方である。portState と同じく 3 つを別の鍵にする。
	logonTypeState carriedState
	// isHttp は HTTP の要求のレコードの区分であるかである。httpStatus と httpStatusState は
	// その HTTP の状態と運び方であり、isHttp が偽の区分では使わない。
	isHttp          bool
	httpStatus      string
	httpStatusState carriedState
}

// evidenceGroups は根拠を観測の種別と接続先 port とログオンの種別で分ける。
//
// **走査の順に区分を並べる。** 同じ入力から同じ並びを返し、画面が区分の位置で要素を
// 指せる形にしない。
//
// 既知の制限: 区分に上限と打ち切りを置かず、エッジ 1 本の区分を全件返す,
// 区分の数は観測の種別と接続先 port とログオンの種別の組の数で抑えられる, 600 区分を超える
// エッジを収集元で確認したとき、または区分の一覧の大きさが `/api/v0/edges/{id}` の応答時間に
// 出たときに、打ち切りの形を決める。上限で区分を切ると、用いた手段の一覧そのものが欠ける。
// 打ち切りを置くなら、件数の合計を残す形にする
func (g Graph) evidenceGroups(evidence []int) []core.EdgeEvidenceGroup {
	order := make([]evidenceGroupKey, 0, len(evidence))
	members := make(map[evidenceGroupKey][]int, len(evidence))
	for _, at := range evidence {
		key := g.evidenceGroupKeyOf(at)
		if _, taken := members[key]; !taken {
			order = append(order, key)
		}
		members[key] = append(members[key], at)
	}
	groups := make([]core.EdgeEvidenceGroup, 0, len(order))
	for _, key := range order {
		groups = append(groups, g.evidenceGroupOf(key, members[key]))
	}
	return groups
}

// evidenceGroupKeyOf はレコード 1 件が属する区分の鍵を返す。
func (g Graph) evidenceGroupKeyOf(at int) evidenceGroupKey {
	record := g.records[at]
	portState, port := destinationPortOf(record)
	logonTypeState, logonType := logonTypeOf(record)
	httpState, httpStatus, isHttp := carriedHttpStatusOf(record)
	return evidenceGroupKey{
		eventCategory: record.eventCategory, eventAction: record.eventAction,
		destinationPort: port, portState: portState,
		logonType: logonType, logonTypeState: logonTypeState,
		isHttp: isHttp, httpStatus: httpStatus, httpStatusState: httpState,
	}
}

// evidenceGroupOf は区分 1 つを応答の項目へ直す。
//
// 観測の種別と接続先 port とログオンの種別の欄は先頭のレコードから写す。鍵がそれぞれの
// 比べられる値と持ち方を含むため、区分の中の欄は同じ意味の同じ値を持つ。**原資料の文字列が
// 区分の中で揃うことは鍵が保証しない。** 同じ比べられる値を別の原資料の文字列が持つ入力形式では、
// 先頭のレコードの原資料の文字列が区分を代表する。
func (g Graph) evidenceGroupOf(key evidenceGroupKey, members []int) core.EdgeEvidenceGroup {
	first := g.records[members[0]]
	group := core.EdgeEvidenceGroup{
		ObservationKind: core.ObservationKind{
			Raw:     cloneRecordFields(first.observationKind.Raw),
			Status:  first.observationKind.Status,
			Meaning: first.observationKind.Meaning,
		},
		Accounts:        g.evidenceGroupAccounts(members),
		Authentications: g.evidenceGroupAuthentications(members),
		EvidenceCount:   int64(len(members)),
	}
	group.DestinationPort, group.DestinationPortAbsence = groupFieldOf(key.portState,
		first.destinationPort, destinationPortUnreadableReason, destinationPortAbsentReason)
	group.LogonType, group.LogonTypeAbsence = groupFieldOf(key.logonTypeState,
		first.logonType, logonTypeUnreadableReason, logonTypeAbsentReason)
	if key.isHttp {
		group.HttpStatus, group.HttpStatusAbsence = groupFieldOf(key.httpStatusState,
			first.httpStatus, httpStatusUnreadableReason, httpStatusAbsentReason)
	}
	group.Selector, group.SelectorAbsence = evidenceGroupSelectorOf(key)
	return group
}

// groupFieldOf は、区分の先頭のレコードの項目 1 つを応答の欄にする。比べられる値を持たない
// 区分では、欄を出さずに理由を返す。
func groupFieldOf(
	state carriedState, field *core.RecordField, unreadableReason, absentReason string,
) (*core.RecordField, string) {
	switch state {
	case carriedReadable:
		copied := cloneRecordField(*field)
		return &copied, ""
	case carriedUnreadable:
		return nil, unreadableReason
	default:
		return nil, absentReason
	}
}

// evidenceGroupSelectorOf は区分を要求で指す値を組む。
// 指せない区分では、指せない理由を返す。
//
// **HTTP の要求の区分は、観測の種別を持たなくても HTTP の状態で指せる。** Proxy のログは観測の
// 種別を記録しない。その値は、観測の種別を持つレコードを通さない (recordInGroup)。
func evidenceGroupSelectorOf(key evidenceGroupKey) (*core.EdgeEvidenceSelector, string) {
	withoutKind := key.eventCategory == "" && key.eventAction == ""
	if (key.eventCategory == "" || key.eventAction == "") && (!key.isHttp || !withoutKind) {
		return nil, observationKindAbsentReason
	}
	carriedItems := []struct {
		state                    carriedState
		value                    string
		unreadable, emptyReading string
	}{
		{key.portState, key.destinationPort,
			destinationPortUnreadableSelectorReason, destinationPortEmptySelectorReason},
		{key.logonTypeState, key.logonType,
			logonTypeUnreadableSelectorReason, logonTypeEmptySelectorReason},
	}
	if key.isHttp {
		carriedItems = append(carriedItems, struct {
			state                    carriedState
			value                    string
			unreadable, emptyReading string
		}{key.httpStatusState, key.httpStatus,
			httpStatusUnreadableSelectorReason, httpStatusEmptySelectorReason})
	}
	for _, carried := range carriedItems {
		if carried.state == carriedUnreadable {
			return nil, carried.unreadable
		}
		if carried.state == carriedReadable && carried.value == "" {
			return nil, carried.emptyReading
		}
	}
	selector := core.EdgeEvidenceSelector{
		EventCategory: key.eventCategory, EventAction: key.eventAction,
		DestinationPort:       key.destinationPort,
		DestinationPortAbsent: key.portState == carriedFieldAbsent,
		LogonType:             key.logonType,
		LogonTypeAbsent:       key.logonTypeState == carriedFieldAbsent,
	}
	if key.isHttp {
		selector.HttpStatus = key.httpStatus
		selector.HttpStatusAbsent = key.httpStatusState == carriedFieldAbsent
	}
	// 上の分岐を通った組は現在の Validate をすべて通るため、ここは到達しない。
	// Validate が条件を足したときに、組めない値を応答が指せる区分として出さないために残す。
	if selector.Validate() != nil {
		return nil, evidenceGroupSelectorUnbuildableReason
	}
	return &selector, ""
}

// evidenceGroupAccounts は区分のレコードに現れたアカウントを、現れた件数とともに返す。
//
// **アカウントを指さないレコードは要素を作らない。** 集合が空である区分は、その観測の
// 種別がアカウントを持たないことを表す。
func (g Graph) evidenceGroupAccounts(members []int) []core.EdgeEvidenceAccount {
	order := make([]string, 0, len(members))
	counts := make(map[string]int64, len(members))
	for _, at := range members {
		nodeId := g.records[at].accountNodeId
		if nodeId == "" {
			continue
		}
		if _, taken := counts[nodeId]; !taken {
			order = append(order, nodeId)
		}
		counts[nodeId]++
	}
	accounts := make([]core.EdgeEvidenceAccount, 0, len(order))
	for _, nodeId := range order {
		index, present := g.nodeAt[nodeId]
		// **前提が破れたことを通知せずに除かない。** 根拠のレコードが指すアカウントの
		// 識別鍵は、同じレコードからノードを作る経路と同じ材料で組むため、一致するノードが
		// 無い状態はグラフの組み立ての不整合である。識別子はグラフが作った不透明な値で
		// あり、原資料の文字列を含まない。
		if !present {
			slog.Error("an account named by an evidence record is absent from the graph",
				"nodeId", nodeId)
			continue
		}
		accounts = append(accounts, core.EdgeEvidenceAccount{
			// 区分は値の検索の条件を持たないため、空の GraphQuery を渡す。渡した条件で
			// 決まるのは SubgraphNode.ValueMatches であり、本項目は GraphNode だけを読む。
			Node:          g.subgraphNode(index, core.NodeSelectionMatched, GraphQuery{}).GraphNode,
			EvidenceCount: counts[nodeId],
		})
	}
	return accounts
}

// evidenceGroupAuthentications は、区分のレコードが記録した認証の方式を、比べられる値ごとに
// 記録した件数とともに返す。並びは最初に記録したレコードの走査の順である。
//
// **方式を記録しないレコードは要素を作らない。** 集合が空である区分は、その観測の種別が
// 認証の方式を持たないことを表す。
func (g Graph) evidenceGroupAuthentications(members []int) []core.EdgeEvidenceAuthentication {
	authentications := []core.EdgeEvidenceAuthentication{}
	positions := make(map[string]int)
	for _, at := range members {
		record := g.records[at]
		if !record.hasRecordNode {
			continue
		}
		// レコードのノードの属性は読めた欄だけを持つ。欄は初めて見た値のときだけ複製する。
		for _, attribute := range g.nodes[record.recordNode].attributes {
			if attribute.field.Semantic != core.SemanticKeyEventAuthenticationPackage {
				continue
			}
			value, readable := comparableFieldValue(*attribute.field)
			if !readable {
				continue
			}
			if position, seen := positions[value]; seen {
				authentications[position].EvidenceCount++
				continue
			}
			positions[value] = len(authentications)
			authentications = append(authentications, core.EdgeEvidenceAuthentication{
				Value: cloneRecordField(*attribute.field), EvidenceCount: 1,
			})
		}
	}
	return authentications
}

// cloneAssignmentBases は成立の根拠を複製する。呼び出し側の変更がグラフに及ばない。
func cloneAssignmentBases(bases []core.EdgeAssignmentBasis) []core.EdgeAssignmentBasis {
	cloned := make([]core.EdgeAssignmentBasis, 0, len(bases))
	for _, basis := range bases {
		cloned = append(cloned, cloneAssignmentBasis(basis))
	}
	return cloned
}

// cloneAssignmentBasis は成立の根拠 1 件を複製する。
func cloneAssignmentBasis(basis core.EdgeAssignmentBasis) core.EdgeAssignmentBasis {
	copied := basis
	conditions := make([]core.MatchCondition, 0, len(basis.Conditions))
	for _, condition := range basis.Conditions {
		condition.LeftValue = cloneRecordFields(condition.LeftValue)
		condition.RightValue = cloneRecordFields(condition.RightValue)
		condition.AssignmentValidRange = cloneTimeRangePointer(condition.AssignmentValidRange)
		condition.OutsideAssignmentRange = clonePointer(condition.OutsideAssignmentRange)
		conditions = append(conditions, condition)
	}
	copied.Conditions = conditions
	copied.Assumptions = append(
		make([]core.MatchAssumption, 0, len(basis.Assumptions)), basis.Assumptions...)
	return copied
}

// cloneTimeRangePointer は時刻の両端を複製する。
func cloneTimeRangePointer(source *core.TimeRange) *core.TimeRange {
	if source == nil {
		return nil
	}
	copied := core.TimeRange{
		From: cloneTimestamp(source.From), To: cloneTimestamp(source.To),
	}
	return &copied
}
