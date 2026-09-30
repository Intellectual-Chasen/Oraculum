package pipeline

import (
	"cmp"
	"slices"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// DefaultLogonSessionLimit は、ログオンのセッションが続く最長の時間の既定である。終わりを
// 決める記録を持たないセッションは、最後の操作の記録からこの時間の後に終わる。
const DefaultLogonSessionLimit = 24 * time.Hour

// sessionStartKind は、ログオンのセッションの始まりを決めた記録の種類である。
type sessionStartKind uint8

const (
	// sessionStartLogon は、ログオンの記録 (4624、USER_START) が始まりを決めたことである。
	sessionStartLogon sessionStartKind = iota
	// sessionStartFirstOperation は、同じ Logon ID を持つ最初の操作の記録が始まりを決めたことである。
	sessionStartFirstOperation
	// sessionStartLogoffRecord は、ログオフの記録が書いた始まりの時刻が始まりを決めたことである。
	sessionStartLogoffRecord
	sessionStartKindCount
)

// sessionEndKind は、ログオンのセッションの終わりを決めた記録の種類である。
type sessionEndKind uint8

const (
	// sessionEndLogoff は、ログオフの記録 (4634、4647、USER_END) が終わりを決めたことである。
	sessionEndLogoff sessionEndKind = iota
	// sessionEndSystemStart は、次の起動の記録が終わりを決めたことである。
	sessionEndSystemStart
	// sessionEndTimeLimit は、最後の操作の記録から時間 T の後を終わりにしたことである。
	sessionEndTimeLimit
	// sessionEndLastOperation は、ログオフの記録が無いネットワークのログオンのセッションを、最後の
	// 操作の記録 (無いときは始まりの記録) で終えたことである。
	sessionEndLastOperation
	sessionEndKindCount
)

// logonSessionPeriod は、端末 1 台のログオンのセッション 1 つの期間である。
type logonSessionPeriod struct {
	// terminal はセッションを記録した端末のノードの識別子である (graphRecord.recordingTerminalNodeId)。
	// 分析者の割当はセッションの期間を延ばさない。ある時刻に置く端末は placementAt で求める。
	terminal string
	// logonId はセッションの Logon ID の比べる値である。Logon ID を持たないログオフの記録が
	// 期間を決めたセッションでは空の文字列である。
	logonId string
	// startAt は始まりを決めた記録の g.records での位置であり、セッションを指すレコードである。
	startAt int
	// endAt は終わりを決めた記録の g.records での位置である。ログオフの記録、次の起動の記録、
	// T を数え始めた最後の操作の記録 (操作が無いときは始まりの記録) のどれかである。
	endAt      int
	start, end time.Time
	startKind  sessionStartKind
	endKind    sessionEndKind
}

// addSessionBasis は、エッジ edge の根拠に、セッションの始まりと終わりを決めた記録を足し、始まりの
// 記録と終点のレコード target の組を startRule で、終わりの記録と target の組を endRule で足す。
// 終わりの記録が始まりの記録と同じときは始まりの組だけを足す。
//
// セッションの期間と終点のレコードと、候補の件数 tally を、エッジの根拠に残す (sessionSpan)。
func (g *Graph) addSessionBasis(
	edge int, session logonSessionPeriod, startRule, endRule pairRule, tally candidateTally, target int,
) {
	g.addEdgeEvidence(edge, session.startAt)
	g.addRecordPair(edge, startRule, session.startAt, target)
	if session.endAt != session.startAt {
		g.addEdgeEvidence(edge, session.endAt)
		g.addRecordPair(edge, endRule, session.endAt, target)
	}
	basis := g.edges[edge].ensureBasis()
	basis.sessions = append(basis.sessions, sessionSpan{
		start: session.start, end: session.end, startAt: session.startAt, endAt: session.endAt, target: target,
		candidateTally: tally,
	})
}

// sessionSpan は、ログオンのセッション 1 つの期間と、そのセッションから結んだ終点のレコードである。
// startAt と endAt はセッションの始まりと終わりを決めた記録、target は終点のレコードの g.records での
// 位置である。
type sessionSpan struct {
	start, end             time.Time
	startAt, endAt, target int
	candidateTally
}

// candidateTally は、終点に挙がった候補の数 (candidates) と、そのうち並びでこのセッションより上の
// 区分に入る候補の数 (preceding) と、このセッションの区分の順 (tier、chainCandidate.tier) である。
// 候補を区分で並べない関係では零値である (core.EdgeCandidateTally)。件数は uint16 の上限で止める。
type candidateTally struct {
	candidates, preceding uint16
	tier                  uint8
}

// sessionList はエッジのセッションの期間を返す。成立の根拠を持たないエッジでは nil を返す。
func (e graphEdge) sessionList() []sessionSpan {
	if e.basis == nil {
		return nil
	}
	return e.basis.sessions
}

// contains は、時刻 at がセッションの期間の中 (両端を含む) にあるかを返す。
func (p logonSessionPeriod) contains(at time.Time) bool {
	return !at.Before(p.start) && !at.After(p.end)
}

// sessionKey は、ログオンのセッションを識別する端末と、起動の区切りと、Logon ID の組である。
// boot はその端末の起動の記録のうち、時刻がレコードの時刻より後でないものの数である。
type sessionKey struct {
	terminal string
	boot     int
	logonId  string
}

// sessionRecord は、Logon ID を持つレコード 1 件の位置と時刻である。network は、ログオンの
// 記録がネットワークのログオン (種別 3) を記録したかである。
type sessionRecord struct {
	at      int
	instant time.Time
	network bool
}

// networkLogonType は、ネットワークのログオンの種別のコードである。
const networkLogonType = "3"

// sessionRecords は、1 つのセッションの鍵を持つレコードを、役割ごとに並べる。
type sessionRecords struct {
	logons, logoffs, operations []sessionRecord
}

// logonSessionPeriodsOf は、公開中の収集元のレコードから、端末ごとのログオンのセッションの
// 期間を求める。並びは端末、起動の区切り、Logon ID、始まりの時刻の順である。
//
// **セッションは、レコードを記録した端末と起動の区切りと Logon ID の組で識別する。** 始まりはログオンの記録であり、
// 同じ組のログオンが 2 件以上あるときは、ログオンごとに 1 つのセッションである。ログオンの記録が
// 無い組は、最初の操作の記録を始まりにする。ログオフの記録だけを持つ組は始まりを持たず、
// セッションにならない。終わりは、始まりより前でない最初のログオフの記録、無いときは次の起動の
// 記録、無いときは始まりより前でない最後の操作の記録 (無いときは始まり) から limit の後である。
// **ログオフの記録が無いネットワークのログオン (種別 3) のセッションは、次の起動の記録を待たず、
// 最後の操作の記録 (無いときは始まり) で終える。** 種別 3 のセッションはログオフを記録しないことが
// 多く、次の起動や limit まで延ばすと、後の無関係なログオンの期間まで覆う。
//
// ログオフの記録が始まりの時刻を書いたとき (event.session_start_time) は、Logon ID を持たない
// そのレコード 1 件が、書いた始まりから記録の時刻までのセッションである。
//
// 既知の制限: 時点を持たないレコード (UTC からのずれの無い時刻) をセッションに入れない,
// 端末ごとの時計の間で前後を比べる値を持たない。時点を持たない操作だけの組が
// セッションにならないことを test で確かめた, 地方時の記録だけの端末のログオンの連鎖を求める
// 要求が出たとき、同じ時計の記録どうしで期間を組む
func (g Graph) logonSessionPeriodsOf(result ImportResult, limit time.Duration) []logonSessionPeriod {
	boots := make(map[string][]sessionRecord)
	type keyedRecord struct {
		terminal, logonId string
		record            sessionRecord
		role              core.SemanticKey
	}
	var keyed []keyedRecord
	var periods []logonSessionPeriod
	for _, publication := range result.publications {
		if publication.status.PublicationState == core.PublicationStateWithheld {
			continue
		}
		for _, record := range publication.records {
			at, indexed := g.recordAtLocator(record.Locator)
			if !indexed || !g.records[at].hasRecordNode || !g.records[at].hasInstant ||
				g.records[at].recordingTerminalNodeId == "" {
				continue
			}
			terminal, instant := g.records[at].recordingTerminalNodeId, g.records[at].instant
			if record.Semantics != nil && record.Semantics.SystemStart {
				boots[terminal] = append(boots[terminal], sessionRecord{at: at, instant: instant})
			}
			fields := graphFieldsOf(record)
			if start, recorded := logoffRecordedStart(fields, instant); recorded {
				periods = append(periods, logonSessionPeriod{terminal: terminal, startAt: at, endAt: at, start: start,
					end: instant, startKind: sessionStartLogoffRecord, endKind: sessionEndLogoff})
				continue
			}
			for _, role := range []core.SemanticKey{core.SemanticKeyEventTargetLogonId,
				core.SemanticKeyEventLogoffLogonId, core.SemanticKeyEventSubjectLogonId} {
				if logonId, carried := sessionLogonIdOf(fields, role); carried {
					logonType, _ := comparableOfSemantic(fields, core.SemanticKeyEventLogonType)
					network := role == core.SemanticKeyEventTargetLogonId && logonType == networkLogonType
					keyed = append(keyed, keyedRecord{terminal, logonId, sessionRecord{at, instant, network}, role})
				}
			}
		}
	}
	for _, records := range boots {
		slices.SortFunc(records, func(a, b sessionRecord) int {
			return cmp.Or(a.instant.Compare(b.instant), cmp.Compare(a.at, b.at))
		})
	}
	groups := make(map[sessionKey]*sessionRecords)
	for _, item := range keyed {
		key := sessionKey{item.terminal, bootsNotAfter(boots[item.terminal], item.record.instant), item.logonId}
		group, found := groups[key]
		if !found {
			group = &sessionRecords{}
			groups[key] = group
		}
		switch item.role {
		case core.SemanticKeyEventTargetLogonId:
			group.logons = append(group.logons, item.record)
		case core.SemanticKeyEventLogoffLogonId:
			group.logoffs = append(group.logoffs, item.record)
		default:
			group.operations = append(group.operations, item.record)
		}
	}
	for key, group := range groups {
		periods = append(periods, group.periods(key, boots[key.terminal], limit)...)
	}
	slices.SortFunc(periods, func(a, b logonSessionPeriod) int {
		return cmp.Or(cmp.Compare(a.terminal, b.terminal), a.start.Compare(b.start), cmp.Compare(a.startAt, b.startAt))
	})
	return periods
}

// bootsNotAfter は、時刻順の起動の記録のうち、時刻が at より後でないものの数を返す。
func bootsNotAfter(boots []sessionRecord, at time.Time) int {
	count, _ := slices.BinarySearchFunc(boots, at, func(boot sessionRecord, target time.Time) int {
		if boot.instant.After(target) {
			return 1
		}
		return -1
	})
	return count
}

// periods は、1 つの鍵のレコードからセッションの期間を組む (logonSessionPeriodsOf)。
func (r *sessionRecords) periods(key sessionKey, boots []sessionRecord, limit time.Duration) []logonSessionPeriod {
	byTime := func(a, b sessionRecord) int { return cmp.Or(a.instant.Compare(b.instant), cmp.Compare(a.at, b.at)) }
	slices.SortFunc(r.logons, byTime)
	slices.SortFunc(r.logoffs, byTime)
	slices.SortFunc(r.operations, byTime)
	starts, kind := r.logons, sessionStartLogon
	if len(starts) == 0 {
		starts, kind = r.operations[:min(1, len(r.operations))], sessionStartFirstOperation
	}
	periods := make([]logonSessionPeriod, 0, len(starts))
	for _, start := range starts {
		period := logonSessionPeriod{terminal: key.terminal, logonId: key.logonId, startAt: start.at,
			start: start.instant, startKind: kind}
		var ender sessionRecord
		ender, period.endKind = r.endOf(start, key.boot, boots)
		period.endAt, period.end = ender.at, ender.instant
		if period.endKind == sessionEndTimeLimit {
			period.end = ender.instant.Add(limit)
		}
		periods = append(periods, period)
	}
	return periods
}

// endOf は、start に始まるセッションの終わりを決めた記録と、その種類を返す (logonSessionPeriodsOf)。
// 種類が sessionEndTimeLimit と sessionEndLastOperation のときの記録は最後の操作の記録であり、
// 始まりより後の操作が無いときは start である。
func (r *sessionRecords) endOf(
	start sessionRecord, boot int, boots []sessionRecord,
) (sessionRecord, sessionEndKind) {
	for _, logoff := range r.logoffs {
		if !logoff.instant.Before(start.instant) {
			return logoff, sessionEndLogoff
		}
	}
	last := start
	for _, operation := range r.operations {
		if operation.instant.After(last.instant) {
			last = operation
		}
	}
	if start.network {
		return last, sessionEndLastOperation
	}
	if boot < len(boots) {
		return boots[boot], sessionEndSystemStart
	}
	return last, sessionEndTimeLimit
}

// logoffRecordedStart は、ログオフの記録が書いたセッションの始まりの時刻を、記録の時刻 end と
// 同じ UTC からのずれで読んだ時点として返す。ok が偽になるのは、始まりの時刻を持たないときと、
// 始まりの時刻を壁時計の日時として読めないときと、始まりが end より後のときである。
//
// 既知の制限: 始まりの時刻を記録の時刻と同じずれで読む, 始まりの時刻は UTC からのずれを持たず、
// 記録を書いた端末の壁時計の日時である。記録は夏時間の切り替えの時点を持たず、切り替えをまたぐ
// セッションのずれを検出できない, 夏時間を持つ地域の入力を取り込むとき、端末のずれの記録で読む
func logoffRecordedStart(fields []core.RecordField, end time.Time) (time.Time, bool) {
	for _, field := range fields {
		if field.Semantic != core.SemanticKeyEventSessionStartTime || field.Timestamp == nil {
			continue
		}
		wall, readable := field.Timestamp.LocalClockTime()
		start := time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(),
			wall.Nanosecond(), end.Location())
		return start, readable && !start.After(end)
	}
	return time.Time{}, false
}
