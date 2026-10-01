package pipeline

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// TimelineQuery は時系列の要求である。
//
// **上限も続きを取る位置も持たない。** 絞り込んだ結果を一度にすべて返す。
type TimelineQuery struct {
	// RecordFilter は根拠のレコードを絞る条件である。
	RecordFilter
	// TextSearch はレコードの値と欄に対する文字列条件である。NodeIds の近傍を辿る条件には使わない。
	TextSearch GraphQuery
	// NodeIds は起点のノードの識別子である。空のときはノードで絞らない。
	//
	// **与えたときは、起点から Depth の段数で辿ったエッジと起点のノードの根拠のレコードだけを
	// 並べる。** 辿り方はグラフの要求と同じであり、RecordFilter を通らないレコードだけを
	// 根拠に持つエッジは辿らない。識別子はすべてグラフにあることを呼び出し元が確かめる。
	NodeIds []string
	// Depth は起点から辿る段数である。0 は起点のノードの根拠だけを並べる。
	Depth int
	// AccountNodeId は役割付きで名指されたアカウントのノードで絞る。空なら絞らない。
	AccountNodeId string
}

// Timeline は要求が選んだ根拠のレコードを、時刻順に並べた列である。
type Timeline struct {
	// Entries は時刻順の行である。件数は len(Entries) が持つ。
	//
	// **UTC からのずれが決まらない地方時のレコードは、時点を持つ行の後ろに別に並べる。**
	// 並びは収集元の識別子の順で、収集元の中では地方時の文字列の順である。時点を持つ行の間に
	// 混ぜると、分析者はずれを仮定した
	// 位置を原資料の事実として読む。分析者が収集元の時刻の解釈を記録すると、そのレコードは
	// 時点を持つ行に入る。
	Entries []core.TimelineEntry
	// UndatedRecordCount は、絞り込みを通りながら比較に用いる時刻も地方時の文字列も持たない
	// レコードの件数である。
	//
	// **既定の日付を入れて列へ混ぜない。** 時刻を読めなかったレコードに時刻を与えると、
	// 分析者はその行の位置を原資料の事実として読む。
	UndatedRecordCount int64
	// PeriodUnjudgedLocalRecordCount と PeriodUnjudgedUndatedRecordCount は、要求が期間で
	// 絞るとき、期間のほかの絞り込みを通りながら、時点を持たないため期間の内か外かを判定
	// できず Entries から外れたレコードの件数である。前者は UTC からのずれが決まらない地方時の
	// 文字列を持つレコード、後者は時刻を持たないレコードを数える。期間で絞らない要求では 0 である。
	PeriodUnjudgedLocalRecordCount   int64
	PeriodUnjudgedUndatedRecordCount int64
	// SourceCoverages は収集元ごとの収録範囲と、要求の期間との重なり方である。
	// 並びは取り込みの入力順である。
	SourceCoverages []core.SourceCoverage
}

// sourceRecording は収集元 1 件の収録範囲である。
type sourceRecording struct {
	sourceId string
	fileName string
	// observed は収録範囲の両端である。known が真のときだけ意味を持つ。
	observed core.TimeRange
	known    bool
}

// sourceRecordingsOf は取り込み結果から収集元ごとの収録範囲を、入力順で採る。
//
// **公開を差し止めた収集元も並べる。** 差し止めはレコードを読めないことを表し、
// その収集元が記録を持たないことを表さない。収録範囲を除くと、分析者は差し止めた
// 区間を「記録が無い区間」と読む。
func sourceRecordingsOf(result ImportResult) []sourceRecording {
	statuses := result.Statuses()
	recordings := make([]sourceRecording, 0, len(statuses))
	for _, status := range statuses {
		recording := sourceRecording{sourceId: status.SourceId}
		identity, found := result.Identity(status.SourceId)
		// 地方時の収集元は、分析者の時刻の解釈で読んだ収録範囲で重なりを判定する。
		identity = result.withObservedRange(identity)
		if found {
			recording.fileName = identity.FileName
		}
		if found && identity.ObservedRangeFirst != nil && identity.ObservedRangeLast != nil {
			recording.observed = core.TimeRange{
				From: *identity.ObservedRangeFirst,
				To:   *identity.ObservedRangeLast,
			}
			recording.known = true
		}
		recordings = append(recordings, recording)
	}
	return recordings
}

// widenRecordingsByEventTimes は、収集元の収録範囲を、その収集元のレコードの時点と、ほかの事象の
// 時刻 (g.additionalEventTimes) を含む範囲へ広げる。時系列はほかの事象の時刻ごとに行を持つため、
// 収録範囲が ObservedAt だけの範囲だと、行の時刻が収録範囲の外に出る。
func (g *Graph) widenRecordingsByEventTimes() {
	indexOf := make(map[string]int, len(g.recordings))
	for index, recording := range g.recordings {
		indexOf[recording.sourceId] = index
	}
	widen := func(sourceId string, at core.Timestamp, instant time.Time) {
		index, found := indexOf[sourceId]
		if !found {
			return
		}
		recording := &g.recordings[index]
		if !recording.known {
			recording.observed, recording.known = core.TimeRange{From: at, To: at}, true
			return
		}
		if from, ok := recording.observed.From.Instant(); ok && instant.Before(from) {
			recording.observed.From = at
		}
		if to, ok := recording.observed.To.Instant(); ok && instant.After(to) {
			recording.observed.To = at
		}
	}
	for at, record := range g.records {
		if record.hasInstant && record.eventTime != nil {
			widen(record.locator.SourceId, *record.eventTime, record.instant)
		}
		for _, additional := range g.additionalEventTimes[at] {
			widen(record.locator.SourceId, *additional.field.Timestamp, additional.instant)
		}
	}
}

// recordsNearNodes は、起点のノードと、起点から query.Depth の段数で辿ったエッジの根拠の
// レコードを g.records の位置で印を付けて返す。起点の無い要求では nil を返す。
func (g Graph) recordsNearNodes(query TimelineQuery) []bool {
	if len(query.NodeIds) == 0 {
		return nil
	}
	near := make([]bool, len(g.records))
	origins := make([]int, 0, len(query.NodeIds))
	for _, id := range query.NodeIds {
		index, found := g.nodeAt[id]
		if !found {
			continue
		}
		origins = append(origins, index)
		for _, at := range g.nodes[index].evidence {
			near[at] = true
		}
	}
	step := GraphQuery{Depth: query.Depth, RecordFilter: query.RecordFilter}
	for _, edge := range g.reachableEdges(origins, step, g.recordPasses(step)) {
		for _, at := range g.edges[edge].evidence {
			near[at] = true
		}
	}
	return near
}

// localWithoutInstant は、時点を持たないレコードが UTC からのずれの決まらない地方時の文字列を
// 持つかを返す。
//
// **時点を持たないレコードを 2 つに分ける。** 地方時のレコードは、分析者が収集元の時刻の解釈で
// ずれを与えると時点を持つ。時刻を持たないレコードは時点を持てない。時系列・ノードの一覧・件数の
// 分布が同じ分け方で数え、分析者が取れる対処を読み分けられるようにする。
func (r graphRecord) localWithoutInstant() bool {
	return !r.hasInstant && r.eventTime != nil && r.eventTime.AcceptsInterpretation()
}

// Timeline は絞り込みを通った根拠のレコードを時刻順に返す。
//
// **並びは事象の時刻の昇順である。** 同じ時刻に並ぶレコードは、収集元の識別子、
// 原資料の位置、取り込みの走査順の順で先後を決める。3 つ目まで見るのは、同じ時刻の
// 同じ位置を 2 件が名乗る取り込みが公開されないことを前提にせず、比較関数が全順序を
// 返すためである。
func (g Graph) Timeline(query TimelineQuery) Timeline {
	query.Validate()
	timeline := Timeline{
		Entries:         make([]core.TimelineEntry, 0, len(g.records)),
		SourceCoverages: g.sourceCoverages(query),
	}
	textSearch := g.withResolvedFieldNames(query.TextSearch)
	var rows, localRows []timelineRow
	matched := make(map[string]int64, len(g.recordings))
	near := g.recordsNearNodes(query)
	accountRecords := g.accountRecords(query.AccountNodeId)
	judgesPeriod := query.TimeFrom != nil || query.TimeTo != nil
	periodless := query.withoutPeriod()
	for at, record := range g.records {
		if !g.recordMatches(at, periodless) {
			continue
		}
		if near != nil && !near[at] {
			continue
		}
		if accountRecords != nil && len(accountRecords[at]) == 0 {
			continue
		}
		if textSearch.SearchesText() &&
			(!record.hasRecordNode || !textSearch.textMatches(g.nodes[record.recordNode])) {
			continue
		}
		passed := false
		switch {
		case judgesPeriod && !record.hasInstant:
			if record.eventTime != nil && record.eventTime.AcceptsInterpretation() {
				timeline.PeriodUnjudgedLocalRecordCount++
			} else {
				timeline.PeriodUnjudgedUndatedRecordCount++
			}
		case record.hasInstant:
			if passed = query.instantInPeriod(record.instant); passed {
				rows = append(rows, timelineRow{at: at, additional: -1, instant: record.instant})
			}
		case record.localWithoutInstant():
			passed = true
			localRows = append(localRows, timelineRow{at: at, additional: -1})
		default:
			passed = true
			timeline.UndatedRecordCount++
		}
		// ほかの事象の時刻は 1 つずつ期間を判定し、通った時刻ごとに行を足す。
		for index, additional := range g.additionalEventTimes[at] {
			if query.instantInPeriod(additional.instant) {
				passed = true
				rows = append(rows, timelineRow{at: at, additional: index, instant: additional.instant})
			}
		}
		if passed {
			matched[record.locator.SourceId]++
		}
	}
	g.sortTimelineRows(rows, func(left, right timelineRow) int {
		return left.instant.Compare(right.instant)
	})
	// ずれの決まらない地方時は収集元を第 1 の鍵にして並べる。収集元ごとにずれが違いうるため、
	// 2 つの収集元の文字列の先後は時刻の先後を表さない。収集元の中では、桁の位置が揃った
	// local_without_offset の文字列の順が地方時の順になる。
	g.sortTimelineRows(localRows, func(left, right timelineRow) int {
		leftRecord, rightRecord := &g.records[left.at], &g.records[right.at]
		if order := strings.Compare(leftRecord.locator.SourceId, rightRecord.locator.SourceId); order != 0 {
			return order
		}
		return strings.Compare(*leftRecord.eventTime.Normalized, *rightRecord.eventTime.Normalized)
	})
	matchedRows := make(map[string]int64, len(g.recordings))
	for _, row := range append(rows, localRows...) {
		matchedRows[g.records[row.at].locator.SourceId]++
		entry := g.timelineEntry(row)
		if accountRecords != nil {
			entry.AccountRoles = accountRecords[row.at]
			entry.OtherAccounts = g.otherAccountsOfRecord(row.at, query.AccountNodeId)
			entry.SourceAddress = g.sourceAddressOfRecord(row.at)
		}
		timeline.Entries = append(timeline.Entries, entry)
	}
	for index := range timeline.SourceCoverages {
		coverage := &timeline.SourceCoverages[index]
		coverage.MatchedRecordCount = matched[coverage.SourceId]
		coverage.MatchedRowCount = matchedRows[coverage.SourceId]
	}
	return timeline
}

// accountRecords はアカウントへの役割付きエッジの根拠をレコードごとに集める。
func (g Graph) accountRecords(id string) map[int][]core.EdgeKind {
	if id == "" {
		return nil
	}
	records := make(map[int][]core.EdgeKind)
	node, found := g.nodeAt[id]
	if !found {
		return records
	}
	for _, at := range g.adjacency[node].incoming {
		edge := g.edges[at]
		if edge.kind != core.EdgeKindRecordSubjectAccount && edge.kind != core.EdgeKindRecordTargetAccount && edge.kind != core.EdgeKindRecordNamesObject {
			continue
		}
		for _, record := range edge.evidence {
			if !slices.Contains(records[record], edge.kind) {
				records[record] = append(records[record], edge.kind)
			}
		}
	}
	return records
}

func (g Graph) otherAccountsOfRecord(record int, selected string) []core.TimelineAccount {
	if !g.records[record].hasRecordNode {
		return nil
	}
	var accounts []core.TimelineAccount
	for _, at := range g.adjacency[g.records[record].recordNode].outgoing {
		edge := g.edges[at]
		if edge.kind != core.EdgeKindRecordSubjectAccount && edge.kind != core.EdgeKindRecordTargetAccount && edge.kind != core.EdgeKindRecordNamesObject {
			continue
		}
		if g.nodes[edge.target].key.Kind != core.NodeKindAccount {
			continue
		}
		if g.nodes[edge.target].id == selected || !slices.Contains(edge.evidence, record) {
			continue
		}
		accounts = append(accounts, core.TimelineAccount{
			Role: edge.kind,
			Node: *g.graphNodeOf(g.nodes[edge.target].id),
		})
	}
	return accounts
}

func (g Graph) sourceAddressOfRecord(record int) string {
	if !g.records[record].hasRecordNode {
		return ""
	}
	for _, attribute := range g.nodes[g.records[record].recordNode].attributes {
		if attribute.field.Semantic == core.SemanticKeyConnectionSourceAddress {
			if value, ok := attribute.field.Text.RawTextValue(); ok {
				return value
			}
		}
	}
	return ""
}

// timelineRow は時系列の 1 行が指すレコードと時刻である。
type timelineRow struct {
	// at はレコードの g.records での位置である。
	at int
	// additional は、行の時刻が g.additionalEventTimes[at] の何番目の時刻かである。-1 の行は
	// レコードの ObservedAt の行である。
	additional int
	// instant は行の時点である。地方時の行では使わない。
	instant time.Time
}

// additionalEventTime は、レコードが ObservedAt のほかに記録した事象の時刻 1 つである。
type additionalEventTime struct {
	// field は時刻の項目であり、取り込み結果の項目を指す。
	field   *core.RecordField
	instant time.Time
}

// addAdditionalEventTimes は、レコードが ObservedAt のほかに記録した事象の時刻のうち、時点を
// 持つ時刻を位置 at の表へ入れる。
func (g *Graph) addAdditionalEventTimes(record RecordEntry, at int) {
	if record.Semantics == nil {
		return
	}
	for index := range record.Semantics.AdditionalEventTimes {
		field := &record.Semantics.AdditionalEventTimes[index]
		if field.Timestamp == nil {
			continue
		}
		instant, ok := field.Timestamp.Instant()
		if !ok {
			continue
		}
		if g.additionalEventTimes == nil {
			g.additionalEventTimes = make(map[int][]additionalEventTime)
		}
		g.additionalEventTimes[at] = append(g.additionalEventTimes[at], additionalEventTime{field: field, instant: instant})
	}
	if len(g.additionalEventTimes[at]) == 0 {
		return
	}
	for _, field := range record.Semantics.Fields {
		if field.Semantic == core.SemanticKeyEventTime && field.Timestamp != nil {
			if g.eventTimeFieldNames == nil {
				g.eventTimeFieldNames = make(map[int]string)
			}
			g.eventTimeFieldNames[at] = field.Name
			return
		}
	}
}

// sortTimelineRows は行を byTime の順に並べ、同じ順位の行を収集元の識別子、原資料の位置、
// 取り込みの走査順、レコードの中の時刻の順で並べる。
func (g Graph) sortTimelineRows(rows []timelineRow, byTime func(left, right timelineRow) int) {
	slices.SortStableFunc(rows, func(left, right timelineRow) int {
		leftRecord, rightRecord := &g.records[left.at], &g.records[right.at]
		if order := byTime(left, right); order != 0 {
			return order
		}
		if order := strings.Compare(
			leftRecord.locator.SourceId, rightRecord.locator.SourceId); order != 0 {
			return order
		}
		if order := strings.Compare(
			locatorPositionKey(leftRecord.locator),
			locatorPositionKey(rightRecord.locator)); order != 0 {
			return order
		}
		return cmp.Or(left.at-right.at, left.additional-right.additional)
	})
}

// timelineEntry は行 1 つを時系列の行へ直す。ほかの事象の時刻の行は、その時刻と項目の名前を持つ。
func (g Graph) timelineEntry(row timelineRow) core.TimelineEntry {
	record := g.records[row.at]
	entry := core.TimelineEntry{
		GraphEvidence: core.GraphEvidence{
			RecordRef:       cloneLocator(record.locator),
			EventTime:       cloneTimestampPointer(record.eventTime),
			ObservationKind: cloneObservationKind(record.observationKind),
			EventKind:       record.eventKindPair(),
		},
	}
	if row.additional >= 0 {
		additional := g.additionalEventTimes[row.at][row.additional].field
		entry.EventTime = cloneTimestampPointer(additional.Timestamp)
		entry.TimeFieldName = additional.Name
	} else {
		entry.TimeFieldName = g.eventTimeFieldNames[row.at]
	}
	entry.Terminal = g.graphNodeOf(record.terminalNodeId)
	entry.Account = g.graphNodeOf(record.accountNodeId)
	return entry
}

// graphNodeOf は識別子が指すノードを含む組を返す。
// 識別子が空の文字列であるとき、およびその識別子のノードが無いときは nil を返す。
func (g Graph) graphNodeOf(id string) *core.GraphNode {
	if id == "" {
		return nil
	}
	index, found := g.nodeAt[id]
	if !found {
		return nil
	}
	node := g.graphNode(index)
	return &node
}

// sourceCoverages は収集元ごとの収録範囲と、要求の期間との重なり方を返す。
// 合致した件数は呼び出し元が後から入れる。
//
// 案件で絞った要求では、その案件の収集元だけを並べる。他の案件の収集元を並べると、
// 要求の期間を覆いながら合致が 0 件の収集元に見える。
func (g Graph) sourceCoverages(query TimelineQuery) []core.SourceCoverage {
	coverages := make([]core.SourceCoverage, 0, len(g.recordings))
	for _, recording := range g.recordings {
		if query.Case != "" && g.caseOfSource[recording.sourceId] != query.Case {
			continue
		}
		if len(query.Sources) > 0 && !slices.Contains(query.Sources, recording.sourceId) {
			continue
		}
		coverage := core.SourceCoverage{
			SourceId:       recording.sourceId,
			SourceFileName: recording.fileName,
			State: core.EvaluateCoverage(recording.observed, recording.known,
				query.TimeFrom, query.TimeTo, query.TimeUnit),
		}
		if recording.known {
			first, last := recording.observed.From, recording.observed.To
			coverage.ObservedRangeFirst, coverage.ObservedRangeLast = &first, &last
		}
		coverages = append(coverages, coverage)
	}
	return coverages
}
