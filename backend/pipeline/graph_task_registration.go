package pipeline

import (
	"slices"
	"strconv"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// taskKey は、端末と、大文字と小文字をそろえたタスクの名前の組である。
type taskKey struct {
	terminal string
	name     string
}

// addTaskRegistrationRunEdges は、タスクの登録のレコードから、同じ名前のタスクの起動の
// レコードへの候補のエッジを足す。
//
// **同じ端末のレコードのタスクの名前の一致と、時刻の前後だけで結ぶ。** 登録の
// scheduled_task.name と起動の started_task.name が大文字と小文字を区別せずに一致し、起動の
// 時刻が登録の時刻より前でない組が候補である。端末の比べ方と時刻の前後は、ログオンの
// セッションの候補と同じである (sameTerminalOrderRejectionOf)。ログオンのセッションと違い、
// 収集元をまたいで結ぶ。登録と起動は別のログ (Security と TaskScheduler) に記録される。
// 結ばなかった組の理由は残さない。
//
// **起動は、起動の時刻で有効な登録の内容のバージョンだけと結ぶ。** 登録のレコード 1 件が
// 登録の内容のバージョンを始め、同じ端末の同じ名前の次の登録の時刻でそのバージョンが終わる。
// 登録を記録した観測の種別 (Security の 4698、TaskScheduler の 106 など) ごとに、上の条件を
// 満たす登録のうち時刻が最も遅いものを、起動の時刻で有効なバージョンとする。同じ 1 件の登録を
// 別のログの 2 つのイベントが記録し、2 つの時刻はずれることがある。登録のレコードのノードを、
// そのレコードが始めたバージョンの端とする。
//
// **同じ種別で最も遅い時刻の登録が 2 件以上あるときは、そのすべてと結ぶ。**
//
// 既知の制限: 一方のログだけが記録した登録し直しの前の登録を、もう一方のログの種別では有効な
// バージョンとして結ぶ, 片方のログだけが登録し直しを記録する条件を形式が定めず、テストの入力でしか
// 確かめられない, 片方のログだけが登録し直しを記録した入力が見つかったとき、種別をまたいで
// バージョンを終える規則を決める
func (g *Graph) addTaskRegistrationRunEdges(result ImportResult, terminals sourceTerminals) {
	// registrations は、端末とタスクの名前ごとに、観測の種別の文字列で分けた登録を持つ。
	registrations := make(map[taskKey]map[string][]logonSessionSide)
	var runs []struct {
		key  taskKey
		side logonSessionSide
	}
	for _, publication := range result.publications {
		if publication.status.PublicationState == core.PublicationStateWithheld {
			continue
		}
		for _, record := range publication.records {
			supplied, scope := terminals.forRecord(record)
			if len(supplied) > 0 {
				record.Terminal = append(slices.Clone(record.Terminal), supplied...)
			}
			fields := graphFieldsOf(record)
			registered, isRegistration := comparableOfSemantic(fields, core.SemanticKeyScheduledTaskName)
			started, isRun := comparableOfSemantic(fields, core.SemanticKeyStartedTaskName)
			if !isRegistration && !isRun {
				continue
			}
			side, built := g.logonSessionSideOf(record, fields, scope)
			if !built {
				continue
			}
			if isRegistration {
				key := taskKey{side.terminal, strings.ToLower(registered)}
				if registrations[key] == nil {
					registrations[key] = make(map[string][]logonSessionSide)
				}
				// scheduled_task.name は意味付けの結果の項目にだけ現れ、Semantics は値を持つ。
				kind := observationKindKeyOf(record.Semantics.ObservationKind)
				registrations[key][kind] = append(registrations[key][kind], side)
			}
			if isRun {
				runs = append(runs, struct {
					key  taskKey
					side logonSessionSide
				}{taskKey{side.terminal, strings.ToLower(started)}, side})
			}
		}
	}
	for _, run := range runs {
		// **観測の種別の map の反復順に依らず、登録のレコードの位置の順にエッジを足す。**
		// エッジと隣接の並びは、ノードの詳細の値の出現順に使われる。
		var matched []logonSessionSide
		for _, sameKind := range registrations[run.key] {
			matched = append(matched, effectiveRegistrationsOf(sameKind, run.side)...)
		}
		slices.SortFunc(matched, func(a, b logonSessionSide) int { return a.recordAt - b.recordAt })
		for _, registration := range matched {
			edge := g.ensureEdge(core.EdgeKindTaskRegistrationRun, core.RelationStateCandidate,
				g.records[registration.recordAt].recordNode, g.records[run.side.recordAt].recordNode)
			g.addEdgeEvidence(edge, registration.recordAt)
			g.addEdgeEvidence(edge, run.side.recordAt)
			g.addRecordPair(edge, pairRuleTaskRun, registration.recordAt, run.side.recordAt)
		}
	}
}

// effectiveRegistrationsOf は、同じ観測の種別の登録 sameKind のうち、起動 run と端末と時刻の
// 条件を満たし、時刻が最も遅い登録のすべてを返す。条件を満たす登録が無いときは空である。
// 起動と同じレコードの登録は返さない。
func effectiveRegistrationsOf(sameKind []logonSessionSide, run logonSessionSide) []logonSessionSide {
	var effective []logonSessionSide
	for _, registration := range sameKind {
		// 1 件のレコードが登録と起動の両方の意味を持つとき、自分自身へのエッジを作らない。
		if registration.recordAt == run.recordAt {
			continue
		}
		if _, rejected := sameTerminalOrderRejectionOf(registration, run); rejected {
			continue
		}
		// 条件を満たす登録はどれも起動と同じ時計の時刻を持ち、互いに前後を比べられる。
		if len(effective) > 0 && registration.at.Before(effective[0].at) {
			continue
		}
		if len(effective) > 0 && registration.at.After(effective[0].at) {
			effective = effective[:0]
		}
		effective = append(effective, registration)
	}
	return effective
}

// observationKindKeyOf は、レコードの観測の種別の欄の名前と原資料の文字列を並べた文字列を返す。
// 同じ入力形式の同じ種別のレコードは同じ文字列を持つ。各値の前に byte 数を置き (specsKeyOf と
// 同じ)、値の前の 1 byte で、欄の値が無い (n)、文字列を持たない (u)、文字列を持つ (t) を分ける。
func observationKindKeyOf(kind core.ObservationKind) string {
	var key strings.Builder
	write := func(value string) {
		key.WriteString(strconv.Itoa(len(value)))
		key.WriteByte(':')
		key.WriteString(value)
	}
	for _, field := range kind.Raw {
		write(field.Name)
		if field.Text == nil {
			key.WriteByte('n')
			continue
		}
		raw, present := field.Text.RawTextValue()
		if !present {
			key.WriteByte('u')
			continue
		}
		key.WriteByte('t')
		write(raw)
	}
	return key.String()
}
