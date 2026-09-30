package pipeline

import (
	"slices"
	"sort"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// sourceTimeInterpretations は所見の一覧から読んだ、収集元の時刻の解釈である。
type sourceTimeInterpretations struct {
	// applied は、収集元の内容の識別ごとに適用する解釈である。
	applied map[string]core.TimestampInterpretation
	// conflicted は、主張中の解釈が 2 件以上あるために解釈を適用しない収集元の内容の識別で
	// ある。文字列の昇順に並ぶ。
	conflicted []string
	// revision は収集元を対象にした所見の改訂の番号の和である。所見を 1 件記録しても、改訂を 1 つ
	// 足しても増えるため、解釈が変わるたびに大きくなる。
	revision int64
}

// timeInterpretationsOf は所見の一覧から、収集元ごとに適用する時刻の解釈を読む。
//
// **主張されている解釈だけを適用する。** 取り消した所見の収集元は解釈を持たない状態に戻る。
// 同じ収集元に主張されている所見が 2 件以上あるときは、どちらのずれで読むかを決めず、
// その収集元に解釈を適用しない。保存先は同じ収集元の 2 件目を退けるため
// (ErrSourceAlreadyInterpreted)、この分岐は保存先の外から入った所見への防御である。
func timeInterpretationsOf(assertions []core.Assertion) sourceTimeInterpretations {
	read := sourceTimeInterpretations{applied: map[string]core.TimestampInterpretation{}}
	conflicted := map[string]bool{}
	for _, assertion := range assertions {
		if assertion.Target.Kind != core.AssertionTargetKindSource {
			continue
		}
		read.revision += assertion.RevisionNumber
		if assertion.State != core.AssertionStateActive || assertion.TimeOffset == nil {
			continue
		}
		content := assertion.Target.SourceContentSha256
		if _, taken := read.applied[content]; taken {
			conflicted[content] = true
			continue
		}
		read.applied[content] = core.TimestampInterpretation{
			Offset: *assertion.TimeOffset, AssertionId: assertion.Id,
		}
	}
	for content := range conflicted {
		delete(read.applied, content)
		read.conflicted = append(read.conflicted, content)
	}
	sort.Strings(read.conflicted)
	return read
}

// AnalystInputRevisions は、store の端末の割当の更新回数と、収集元の時刻の解釈の更新回数を返す。どちらも
// 分析者が記録するたびに大きくなり、グラフの本文を組み直すのに要る。
func AnalystInputRevisions(store InvestigationStore) (assignment, interpretation int64) {
	_, assignment = store.TerminalAssignments().List()
	return assignment, timeInterpretationsOf(store.Assertions().List()).revision
}

// withTimeInterpretations は、解釈を持つ収集元のレコードの時刻へ分析者のずれを与え、解釈で
// 読んだ収録範囲を持つ取り込み結果を返す。
//
// **原資料の文字列と正規化値を書き換えない。** レコードの時刻は Timestamp.Interpretation を
// 足した複製に替わり、原文・正規化値・精度はそのまま残る。収集元の識別の収録範囲も変えない。
// 解釈を持たない収集元のレコードは元の結果と領域を共有する。
//
// **分析者の所見を持たない収集元は、取り込みの起動で指定したずれで読む** (importOffsets)。
// そのずれは所見を持たず、AssertionId を持たない解釈になる。
func (r ImportResult) withTimeInterpretations(
	interpretations map[string]core.TimestampInterpretation,
) ImportResult {
	if len(interpretations) == 0 && len(r.importOffsets) == 0 {
		return r
	}
	publications := slices.Clone(r.publications)
	ranges := map[string]core.TimeRange{}
	applied := map[string]core.TimestampInterpretation{}
	for index, publication := range publications {
		interpretation, found := interpretations[r.identities[publication.status.SourceId].ContentSha256]
		if !found {
			offset, specified := r.importOffsets[publication.status.SourceId]
			if !specified {
				continue
			}
			interpretation = core.TimestampInterpretation{Offset: offset}
		}
		applied[publication.status.SourceId] = interpretation
		if interpreted, ok := interpretedRangeOf(publication.localRange, interpretation); ok {
			ranges[publication.status.SourceId] = interpreted
		}
		records := slices.Clone(publication.records)
		for at, record := range records {
			if record.ObservedAt == nil || !record.ObservedAt.AcceptsInterpretation() {
				continue
			}
			interpreted, err := record.ObservedAt.WithInterpretation(interpretation)
			// ずれは所見の Validate を通っており、ここで退けられるのは文字列を読めない時刻だけ
			// である。その時刻は解釈を持たないまま残す。
			if err != nil {
				continue
			}
			records[at].ObservedAt = &interpreted
		}
		publications[index].records = records
	}
	r.publications = publications
	r.interpretedRanges = ranges
	r.sourceInterpretations = applied
	return r
}

// withRecordInterpretation は、レコードの時刻 (observedAt) が分析者のずれを持つとき、
// 同じレコードの事象の時刻の欄へ同じずれを与えた欄を返す。
//
// **解釈が変えるのは、時系列の並びと関連付けの時刻の範囲に使う時点だけである。** 元レコードの事象の時刻の
// 欄と時系列の行が同じ時点を指すようにし、ほかの時刻の欄と、ずれを与えられない欄はそのまま返す。
func withRecordInterpretation(field core.RecordField, observedAt *core.Timestamp) core.RecordField {
	if observedAt == nil || observedAt.Interpretation == nil || field.Timestamp == nil ||
		field.Semantic != core.SemanticKeyEventTime || !field.Timestamp.AcceptsInterpretation() {
		return field
	}
	interpreted, err := field.Timestamp.WithInterpretation(*observedAt.Interpretation)
	if err != nil {
		return field
	}
	field.Timestamp = &interpreted
	return field
}

// interpretedRangeOf は地方時の収録範囲の両端に分析者のずれを与える。
func interpretedRangeOf(
	local *core.TimeRange, interpretation core.TimestampInterpretation,
) (core.TimeRange, bool) {
	if local == nil {
		return core.TimeRange{}, false
	}
	from, fromErr := local.From.WithInterpretation(interpretation)
	to, toErr := local.To.WithInterpretation(interpretation)
	if fromErr != nil || toErr != nil {
		return core.TimeRange{}, false
	}
	return core.TimeRange{From: from, To: to}, true
}

// withObservedRange は、収録範囲を持たない収集元の識別に、解釈で読んだ収録範囲を入れた
// 複製を返す。関連付けの割当の期間と、時系列の収録範囲の判定が使う。
func (r ImportResult) withObservedRange(identity core.SourceIdentity) core.SourceIdentity {
	if identity.ObservedRangeFirst != nil && identity.ObservedRangeLast != nil {
		return identity
	}
	interpreted, found := r.interpretedRanges[identity.SourceId]
	if !found {
		return identity
	}
	first, last := cloneTimestamp(interpreted.From), cloneTimestamp(interpreted.To)
	identity.ObservedRangeFirst, identity.ObservedRangeLast = &first, &last
	return identity
}

// InterpretedObservedRange は、観測期間を持たない収集元の、分析者の時刻の解釈で読んだ観測
// 期間を返す。両端は地方時の文字列と分析者のずれ (Timestamp.Interpretation) の組である。
// ok が偽になるのは、収集元の識別が観測期間を持つとき、または収集元が解釈を持たないとき
// である。
//
// **利用者が入力する割当の期間の材料である。** 割当はずれを外した地方時の文字列で期間を保存し
// (core.TerminalAssignment.Validate)、グラフを組むときのその収集元の解釈で読む
// (withInterpretedRange)。
func (r ImportResult) InterpretedObservedRange(sourceId string) (core.TimeRange, bool) {
	identity, found := r.identities[sourceId]
	if !found || (identity.ObservedRangeFirst != nil && identity.ObservedRangeLast != nil) {
		return core.TimeRange{}, false
	}
	interpreted, found := r.interpretedRanges[sourceId]
	if !found {
		return core.TimeRange{}, false
	}
	return core.TimeRange{From: cloneTimestamp(interpreted.From), To: cloneTimestamp(interpreted.To)}, true
}

// withInterpretedRange は、利用者の割当の期間が地方時の文字列であるとき、期間を読み取った
// 収集元の今の解釈で読んだ時点を UTC で書いた期間に替えた複製を返す。
//
// **割当は収集元のレコードと同じ解釈で期間を読む。** 分析者が解釈を変えると期間も読み直し、
// 解釈を取り消すと期間は地方時の文字列に戻って比べられない (core.ResolveTerminal の
// time_not_comparable)。UTC からのずれを持つ期間と、解釈を持たない収集元の期間は変えない。
func (r ImportResult) withInterpretedRange(assignment core.TerminalAssignment) core.TerminalAssignment {
	interpretation, found := r.sourceInterpretations[assignment.SourceId]
	if !found {
		return assignment
	}
	from, fromRead := absoluteByInterpretation(assignment.AssignmentValidRange.From, interpretation)
	to, toRead := absoluteByInterpretation(assignment.AssignmentValidRange.To, interpretation)
	if !fromRead || !toRead {
		return assignment
	}
	assignment.AssignmentValidRange = core.TimeRange{From: from, To: to}
	return assignment
}

// absoluteByInterpretation は地方時の時刻を分析者のずれで読み、UTC で書いた時刻を返す。
// ok が偽になるのは、ずれを与えられない時刻である。
func absoluteByInterpretation(
	timestamp core.Timestamp, interpretation core.TimestampInterpretation,
) (core.Timestamp, bool) {
	if !timestamp.AcceptsInterpretation() {
		return core.Timestamp{}, false
	}
	interpreted, err := timestamp.WithInterpretation(interpretation)
	if err != nil {
		return core.Timestamp{}, false
	}
	return interpreted.AbsoluteByInterpretation()
}

// localRangeOf は、UTC からのずれの決まらない地方時の文字列を持つレコードの最も早い値と最も
// 遅い値を返す。母集団は解析に失敗したレコードを含む。
func localRangeOf(source scannedSource) *core.TimeRange {
	var first, last *core.Timestamp
	var firstAt, lastAt time.Time
	observe := func(timestamp *core.Timestamp) {
		if timestamp == nil || !timestamp.AcceptsInterpretation() {
			return
		}
		// 字面を UTC に置いて比べる。同じ収集元の地方時は同じずれで読まれるため、先後は変わらない。
		at, err := time.Parse("2006-01-02T15:04:05", *timestamp.Normalized)
		if err != nil {
			return
		}
		if first == nil || at.Before(firstAt) {
			first, firstAt = cloneTimestampPointer(timestamp), at
		}
		if last == nil || at.After(lastAt) {
			last, lastAt = cloneTimestampPointer(timestamp), at
		}
	}
	for _, record := range source.Records {
		observe(record.ObservedAt)
	}
	for _, failed := range source.Failures {
		observe(failed.ObservedAt)
	}
	if first == nil {
		return nil
	}
	return &core.TimeRange{From: *first, To: *last}
}
