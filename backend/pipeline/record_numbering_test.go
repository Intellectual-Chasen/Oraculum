// in-package test: 取り込んだ Windows イベントログのレコードの番号の抜けと、2 つの収集元の
// 突き合わせを確かめる。
package pipeline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// numberedEventXML はイベント 1 件を返す。
func numberedEventXML(channel, computer, recordID, at, eventID string) string {
	return `<Event><System><Provider Name="Example-Provider"/><EventID>` + eventID + `</EventID>` +
		`<TimeCreated SystemTime="` + at + `"/><EventRecordID>` + recordID + `</EventRecordID>` +
		`<Channel>` + channel + `</Channel><Computer>` + computer + `</Computer></System></Event>` + "\n"
}

// brokenEventXML は `</Event>` を持たず、取り込みの失敗になる 1 件である。
const brokenEventXML = "<Event><System><EventRecordID>13</EventRecordID>\n"

func eventsXML(events ...string) string {
	return "<Events>\n" + strings.Join(events, "") + "</Events>\n"
}

func xmlInput(id, content string) fieldsSourceInput {
	return fieldsSourceInput{
		id: id, fileName: id + ".xml", format: WindowsEventXMLFormatKey,
		parser: NewTestParser(WindowsEventXMLFormatKey, nil), content: content,
	}
}

func csvInput(id, content string) fieldsSourceInput {
	return fieldsSourceInput{
		id: id, fileName: id + ".csv", format: WindowsEventCSVFormatKey,
		parser: NewTestParser(WindowsEventCSVFormatKey, nil), content: content,
	}
}

// viewerRow はイベントビューアーの CSV の論理レコード 1 件を返す。
func viewerRow(at, eventID string) string {
	return "情報," + at + ",Example-Provider," + eventID + ",,\"合成の説明\"\n"
}

// recordNumberViewerHeader は BOM を持たない見出しである。読み取りは BOM の有無を問わない。
const recordNumberViewerHeader = "レベル,日付と時刻,ソース,イベント ID,タスクのカテゴリ\n"

// offsetOf は document の中で text が始まる byte 位置を返す。
func offsetOf(t *testing.T, document, text string) int64 {
	t.Helper()
	at := strings.Index(document, text)
	if at < 0 {
		t.Fatalf("the document holds no %q", text)
	}
	return int64(at)
}

func offsetsOf(refs []core.RecordLocator) []int64 {
	offsets := make([]int64, 0, len(refs))
	for _, ref := range refs {
		offsets = append(offsets, *ref.ByteOffset)
	}
	return offsets
}

func streamNamed(t *testing.T, numbers core.RecordNumbers, channel string) core.RecordNumberStream {
	t.Helper()
	for _, stream := range numbers.Streams {
		if stream.Channel != nil && *stream.Channel == channel {
			return stream
		}
	}
	t.Fatalf("no stream of %s in %+v", channel, numbers.Streams)
	return core.RecordNumberStream{}
}

// computerCounts は単位の Computer ごとの件数を「名前=件数」の並びにする。
func computerCounts(stream core.RecordNumberStream) []string {
	counts := make([]string, 0, len(stream.Computers))
	for _, computer := range stream.Computers {
		counts = append(counts, *computer.Computer+"="+strconv.FormatInt(computer.RecordCount, 10))
	}
	return counts
}

// 番号の抜けはチャネルごとに数える。抜けの位置に取り込みの失敗があるときだけ、その失敗を
// 抜けに添える。
func TestRecordNumbersReportGapsPerChannel(t *testing.T) {
	const at = "2001-02-03T04:05:06Z"
	document := eventsXML(
		numberedEventXML("Security", "host-a.example.test", "11", at, "7"),
		numberedEventXML("Security", "host-a.example.test", "12", at, "7"),
		brokenEventXML,
		numberedEventXML("Security", "host-a.example.test", "15", at, "7"),
		numberedEventXML("Security", "host-a.example.test", "15", at, "8"),
		numberedEventXML("Security", "host-a.example.test", "18", at, "7"),
		numberedEventXML("Application", "host-a.example.test", "21", at, "7"),
		numberedEventXML("Application", "host-a.example.test", "22", at, "7"),
		numberedEventXML("Security", "host-a.example.test", "not-a-number", at, "7"),
	)
	result := fieldsResult(t, xmlInput("events", document))
	numbers, found := result.RecordNumbers("events")
	if !found {
		t.Fatal("the imported source has no record numbers")
	}
	if numbers.Examination != core.RecordNumberExaminationGapsFound || numbers.UnreadableRecordCount == nil ||
		*numbers.UnreadableRecordCount != 1 || numbers.FileHeader != nil {
		t.Fatalf("numbers = %+v, want gaps with one unreadable number and no file header", numbers)
	}
	security := streamNamed(t, numbers, "Security")
	if security.LowestNumber != 11 || security.HighestNumber != 18 || security.MissingNumberCount != 4 ||
		security.DuplicatedRecordCount != 1 || security.RecordCount != 5 || len(security.Gaps) != 2 {
		t.Fatalf("Security on host-a = %+v", security)
	}
	first, second := security.Gaps[0], security.Gaps[1]
	if first.FirstMissingNumber != 13 || first.LastMissingNumber != 14 ||
		!slices.Equal(offsetsOf(first.FailureRecordRefs), []int64{offsetOf(t, document, brokenEventXML)}) {
		t.Errorf("the first gap = %+v, want 13-14 tied to the broken record", first)
	}
	if *first.PrecedingRecordRef.ByteOffset != offsetOf(t, document, numberedEventXML("Security", "host-a.example.test", "12", at, "7")) {
		t.Errorf("the first gap is preceded by %+v", first.PrecedingRecordRef)
	}
	if second.FirstMissingNumber != 16 || second.LastMissingNumber != 17 || len(second.FailureRecordRefs) != 0 {
		t.Errorf("the second gap = %+v, want 16-17 without a failure", second)
	}
	if application := streamNamed(t, numbers, "Application"); len(application.Gaps) != 0 ||
		application.MissingNumberCount != 0 {
		t.Errorf("Application has gaps %+v", application.Gaps)
	}
	if *numbers.Streams[0].Channel != "Application" {
		t.Errorf("streams are not ordered by channel: %+v", numbers.Streams)
	}
}

// 同じチャネルの番号は Computer の名前が変わっても続く。変わり目の抜けを見つけ、2 つの端末の
// 番号が混ざる file は重複した番号と Computer の種類で表れる。
func TestRecordNumbersFollowTheChannelAcrossComputers(t *testing.T) {
	const at = "2001-02-03T04:05:06Z"
	// renamed は、31 と 32 の後ろで Computer の名前が変わり、third と fourth が続く file を返す。
	renamed := func(third, fourth string) string {
		return eventsXML(
			numberedEventXML("System", "host-a", "31", at, "7"),
			numberedEventXML("System", "host-a", "32", at, "7"),
			numberedEventXML("System", "host-a.example.test", third, at, "7"),
			numberedEventXML("System", "host-a.example.test", fourth, at, "7"))
	}
	mixed := eventsXML(
		numberedEventXML("System", "host-a.example.test", "41", at, "7"),
		numberedEventXML("System", "host-b.example.test", "41", at, "7"),
		numberedEventXML("System", "host-a.example.test", "42", at, "7"),
		numberedEventXML("System", "host-b.example.test", "42", at, "7"),
		numberedEventXML("System", "host-a.example.test", "43", at, "7"))
	result := fieldsResult(t,
		xmlInput("continued", renamed("33", "34")), xmlInput("broken", renamed("34", "35")), xmlInput("mixed", mixed))

	continued, _ := result.RecordNumbers("continued")
	stream := streamNamed(t, continued, "System")
	if continued.Examination != core.RecordNumberExaminationNoGaps || len(continued.Streams) != 1 ||
		!slices.Equal(computerCounts(stream), []string{"host-a=2", "host-a.example.test=2"}) {
		t.Errorf("the renamed computer splits the numbers: %+v", continued)
	}
	broken, _ := result.RecordNumbers("broken")
	if gaps := streamNamed(t, broken, "System").Gaps; broken.Examination != core.RecordNumberExaminationGapsFound ||
		len(gaps) != 1 || gaps[0].FirstMissingNumber != 33 || gaps[0].LastMissingNumber != 33 {
		t.Errorf("the gap at the renamed computer is %+v", broken)
	}
	mixedNumbers, _ := result.RecordNumbers("mixed")
	stream = streamNamed(t, mixedNumbers, "System")
	if stream.DuplicatedRecordCount != 2 || stream.MissingNumberCount != 0 ||
		!slices.Equal(computerCounts(stream), []string{"host-a.example.test=3", "host-b.example.test=2"}) {
		t.Errorf("the mixed file = %+v", stream)
	}
}

// 番号の並びと失敗の位置の組ごとに、抜けと、抜けに結ぶ失敗を確かめる。失敗を結ぶのは、抜けの
// 直前と直後のレコードの間に公開されたレコードが 1 件も無いときだけである。
func TestRecordNumbersTieAFailureOnlyToTheGapItSitsIn(t *testing.T) {
	const at = "2001-02-03T04:05:06Z"
	event := func(channel, recordID string) string {
		return numberedEventXML(channel, "host-a.example.test", recordID, at, "7")
	}
	type wantGap struct {
		first, last  string
		tiedFailures int
	}
	// 境界の値は型の上限から組み、文字列を手で書かない。
	beyondInt64 := func(offset uint64) string { return strconv.FormatUint(math.MaxInt64+offset, 10) }
	// belowSafeLimit は 2^53 より offset だけ小さい番号である。2^53 は読めない番号になる。
	belowSafeLimit := func(offset uint64) string { return strconv.FormatUint(1<<53-offset, 10) }
	for _, test := range []struct {
		name       string
		events     []string
		gaps       map[string][]wantGap
		unreadable int64
	}{{
		name:   "a failure between two channels ties only to the gap around it",
		events: []string{event("Security", "100"), event("System", "5"), brokenEventXML, event("System", "7"), event("Security", "102")},
		gaps: map[string][]wantGap{
			"Security": {{"101", "101", 0}},
			"System":   {{"6", "6", 1}},
		},
	}, {
		name: "numbers written in descending order tie the failure between the adjacent records",
		events: []string{
			event("Security", "102"), event("System", "7"), brokenEventXML, event("System", "5"), event("Security", "100"),
		},
		gaps: map[string][]wantGap{
			"Security": {{"101", "101", 0}},
			"System":   {{"6", "6", 1}},
		},
	}, {
		name:   "a channel of one record has no gap",
		events: []string{event("System", "5")},
		gaps:   map[string][]wantGap{"System": {}},
	}, {
		name: "numbers that are not unsigned decimals or exceed the safe integers are unreadable",
		events: []string{
			event("System", "-1"), event("System", "+5"), event("System", " 5"),
			event("System", strconv.FormatUint(math.MaxUint64, 10)+"0"), event("System", beyondInt64(1)),
			event("System", belowSafeLimit(0)), event("System", belowSafeLimit(1)), event("System", belowSafeLimit(3)),
		},
		gaps:       map[string][]wantGap{"System": {{belowSafeLimit(2), belowSafeLimit(2), 0}}},
		unreadable: 6,
	}} {
		t.Run(test.name, func(t *testing.T) {
			result := fieldsResult(t, xmlInput("events", eventsXML(test.events...)))
			numbers, _ := result.RecordNumbers("events")
			if numbers.UnreadableRecordCount == nil || *numbers.UnreadableRecordCount != test.unreadable {
				t.Errorf("unreadable = %v, want %d", numbers.UnreadableRecordCount, test.unreadable)
			}
			if len(numbers.Streams) != len(test.gaps) {
				t.Fatalf("streams = %+v, want the channels of %v", numbers.Streams, test.gaps)
			}
			for channel, want := range test.gaps {
				gaps := streamNamed(t, numbers, channel).Gaps
				if len(gaps) != len(want) {
					t.Fatalf("%s gaps = %+v, want %v", channel, gaps, want)
				}
				for index, gap := range gaps {
					got := wantGap{fmt.Sprint(gap.FirstMissingNumber), fmt.Sprint(gap.LastMissingNumber), len(gap.FailureRecordRefs)}
					if got != want[index] {
						t.Errorf("%s gap %d = %+v, want %+v", channel, index, got, want[index])
					}
				}
			}
		})
	}
}

// 抜けが無い収集元、番号を持たない形式、番号を読めない収集元を分ける。
func TestRecordNumbersSeparateNoGapsFromNotExamined(t *testing.T) {
	const at = "2001-02-03T04:05:06Z"
	result := fieldsResult(t,
		xmlInput("complete", eventsXML(
			numberedEventXML("System", "host-a.example.test", "31", at, "7"),
			numberedEventXML("System", "host-a.example.test", "32", at, "7"))),
		xmlInput("unnumbered", eventsXML(
			numberedEventXML("System", "host-a.example.test", "", at, "7"))),
		csvInput("viewer", recordNumberViewerHeader+viewerRow("2001/02/03 04:05:06", "7")),
	)
	for _, want := range []struct {
		sourceId    string
		examination core.RecordNumberExamination
		reason      core.RecordNumberNotExaminedReason
	}{
		{"complete", core.RecordNumberExaminationNoGaps, ""},
		{"unnumbered", core.RecordNumberExaminationNotExamined, core.RecordNumberNotExaminedNumbersUnreadable},
		{"viewer", core.RecordNumberExaminationNotExamined, core.RecordNumberNotExaminedNotDeclared},
	} {
		numbers, found := result.RecordNumbers(want.sourceId)
		if !found || numbers.Examination != want.examination || numbers.NotExaminedReason != want.reason {
			t.Errorf("%s: numbers = %+v, want %s %s", want.sourceId, numbers, want.examination, want.reason)
		}
	}
	if viewer, _ := result.RecordNumbers("viewer"); viewer.UnreadableRecordCount != nil {
		t.Errorf("the CSV source counts unreadable numbers: %d", *viewer.UnreadableRecordCount)
	}
	if _, found := result.RecordNumbers("missing"); found {
		t.Error("a source outside the result has record numbers")
	}
}

// headerNames は見出しの欄の名前である。
var headerNames = RecordNumberingNames{
	RecordHeaderID: "record.header.id", HeaderNextRecordID: "file.next", HeaderDirty: "file.dirty",
}

func textRecordField(t testing.TB, name, text string) core.RecordField {
	t.Helper()
	value, err := core.NewRawValue(core.ValueStatePresent, text)
	if err != nil {
		t.Fatal(err)
	}
	field, err := core.NewTextField(name, "", value)
	if err != nil {
		t.Fatal(err)
	}
	return field
}

// headerRecord はレコードの見出しの番号だけを持つレコードである。
func headerRecord(t *testing.T, number string) RecordEntry {
	t.Helper()
	field := textRecordField(t, headerNames.RecordHeaderID, number)
	return RecordEntry{Semantics: &RecordSemantics{Fields: []core.RecordField{field}}}
}

// file の見出しの次のレコード番号は、レコードの見出しの番号の最大の 1 つ後ろと比べる。番号は
// uint64 のまま比べ、見出しを読めないときは読めなかったと返す。
func TestFileHeaderComparisonUsesTheHighestRecordHeaderNumber(t *testing.T) {
	header := func(next string) []core.RecordField {
		return []core.RecordField{
			textRecordField(t, headerNames.HeaderNextRecordID, next), textRecordField(t, headerNames.HeaderDirty, "true"),
		}
	}
	// 境界の値は読める番号の上限 (2^53 - 1) から組み、文字列を手で書かない。
	safeMax, belowMax, beyondSafe := strconv.FormatUint(1<<53-1, 10), strconv.FormatUint(1<<53-2, 10),
		strconv.FormatUint(1<<53, 10)
	for _, want := range []struct {
		name       string
		header     []core.RecordField
		records    []RecordEntry
		comparison core.RecordNumberHeaderComparison
		next       string
		highest    string
		unreadable int64
	}{
		{"the next number follows the highest", header("10"), []RecordEntry{headerRecord(t, "9"), headerRecord(t, "8")},
			core.RecordNumberHeaderAgrees, "10", "9", 0},
		{"a record beyond the header", header("10"), []RecordEntry{headerRecord(t, "8"), headerRecord(t, "12")},
			core.RecordNumberHeaderDiffers, "10", "12", 0},
		{"no readable record header", header("10"), []RecordEntry{{}, headerRecord(t, "+9")},
			core.RecordNumberHeaderNoRecordNumbers, "10", "", 2},
		{"numbers at the safe limit agree", header(safeMax), []RecordEntry{headerRecord(t, belowMax)},
			core.RecordNumberHeaderAgrees, safeMax, belowMax, 0},
		{"a record header beyond the safe limit is unreadable", header(safeMax),
			[]RecordEntry{headerRecord(t, belowMax), headerRecord(t, beyondSafe)},
			core.RecordNumberHeaderAgrees, safeMax, belowMax, 1},
		{"a header number beyond the safe limit is unreadable", header(beyondSafe), []RecordEntry{headerRecord(t, safeMax)},
			core.RecordNumberHeaderUnreadable, "", safeMax, 0},
		{"an unreadable header number", header("-10"), []RecordEntry{headerRecord(t, "9")},
			core.RecordNumberHeaderUnreadable, "", "9", 0},
		{"a header without the next number", nil, []RecordEntry{headerRecord(t, "9")},
			core.RecordNumberHeaderUnreadable, "", "9", 0},
	} {
		got := fileHeaderComparisonOf(want.header, want.records, headerNames)
		if got == nil || got.Comparison != want.comparison || optionalNumber(got.NextRecordNumber) != want.next ||
			optionalNumber(got.HighestRecordNumber) != want.highest || got.UnreadableRecordHeaderCount != want.unreadable {
			t.Errorf("%s: comparison = %+v", want.name, got)
		}
	}
	if got := fileHeaderComparisonOf(header("10"), nil, headerNames); got == nil || got.Dirty == nil || *got.Dirty != "true" {
		t.Errorf("the dirty mark is %+v", got)
	}
	if got := fileHeaderComparisonOf(header("10"), nil, RecordNumberingNames{}); got != nil {
		t.Errorf("a format without the header items has the comparison %+v", got)
	}
}

func optionalNumber(number *uint64) string {
	if number == nil {
		return ""
	}
	return strconv.FormatUint(*number, 10)
}

// 失敗は直前のレコードの終わりから直後のレコードの始まりの前までの位置で結ぶ。
func TestFailuresBetweenKeepsTheByteBoundaries(t *testing.T) {
	locator := func(offset, length int64) core.RecordLocator {
		return core.RecordLocator{ByteOffset: &offset, ByteLength: &length}
	}
	failureAt := func(offset int64) core.ImportFailure {
		ref := locator(offset, 1)
		return core.ImportFailure{RecordRef: &ref}
	}
	before, after := locator(100, 10), locator(150, 10)
	failures := []core.ImportFailure{failureAt(109), failureAt(110), failureAt(149), failureAt(150), {}}
	if got := offsetsOf(failuresBetween(failures, before, after)); !slices.Equal(got, []int64{110, 149}) {
		t.Errorf("failures tied at %v, want 110 and 149", got)
	}
}

// 抜けと、片方にしか無いレコードの位置は先頭の上限の件数に切り、件数は全件を数える。
func TestRecordNumberListsStopAtTheLimit(t *testing.T) {
	const at = "2001-02-03T04:05:06Z"
	events := make([]string, 0, core.RecordNumberListLimit+2)
	for index := range core.RecordNumberListLimit + 2 {
		events = append(events, numberedEventXML("System", "host-a.example.test", strconv.Itoa(index*2+1), at, "7"))
	}
	result := fieldsResult(t, xmlInput("sparse", eventsXML(events...)), xmlInput("other", eventsXML(events[0])))
	numbers, _ := result.RecordNumbers("sparse")
	stream := streamNamed(t, numbers, "System")
	if stream.GapCount != core.RecordNumberListLimit+1 || len(stream.Gaps) != core.RecordNumberListLimit ||
		numbers.ListLimit != core.RecordNumberListLimit || stream.MissingNumberCount != core.RecordNumberListLimit+1 {
		t.Errorf("gaps = %d of %d, limit %d", len(stream.Gaps), stream.GapCount, numbers.ListLimit)
	}
	comparison, _ := result.CompareRecords("sparse", "other")
	if comparison.OnlyInSourceRecordCount != core.RecordNumberListLimit+1 ||
		len(comparison.OnlyInSourceRecordRefs) != core.RecordNumberListLimit || comparison.OnlyInComparedRecordCount != 0 {
		t.Errorf("only in the source = %d of %d", len(comparison.OnlyInSourceRecordRefs), comparison.OnlyInSourceRecordCount)
	}
}

// 番号を持つ 2 つの形式は、チャネルと端末と番号で突き合わせる。
func TestCompareRecordsByRecordNumber(t *testing.T) {
	const at = "2001-02-03T04:05:06Z"
	onlyLeft := numberedEventXML("Security", "host-a.example.test", "61", at, "7")
	onlyRight := numberedEventXML("Security", "host-a.example.test", "64", at, "7")
	otherComputer := numberedEventXML("Security", "host-b.example.test", "62", at, "7")
	left := eventsXML(onlyLeft,
		numberedEventXML("Security", "host-a.example.test", "62", at, "7"),
		numberedEventXML("Security", "host-a.example.test", "63", at, "7"))
	right := eventsXML(
		numberedEventXML("Security", "host-a.example.test", "62", at, "8"),
		numberedEventXML("Security", "host-a.example.test", "63", at, "8"),
		onlyRight, otherComputer)
	result := fieldsResult(t, xmlInput("left", left), xmlInput("right", right))
	comparison, found := result.CompareRecords("left", "right")
	if !found || comparison.State != core.RecordComparisonCompared ||
		comparison.Key != core.RecordComparisonKeyRecordNumber || comparison.OneToOneKeyCount != 2 ||
		comparison.UndeterminedKeyCount != 0 {
		t.Fatalf("comparison = %+v", comparison)
	}
	if got := offsetsOf(comparison.OnlyInSourceRecordRefs); !slices.Equal(got, []int64{offsetOf(t, left, onlyLeft)}) {
		t.Errorf("only in the source at %v", got)
	}
	want := []int64{offsetOf(t, right, onlyRight), offsetOf(t, right, otherComputer)}
	if got := offsetsOf(comparison.OnlyInComparedRecordRefs); !slices.Equal(got, want) {
		t.Errorf("only in the compared source at %v, want %v", got, want)
	}
}

// 片方にしか無いレコードを、相手のチャネルの番号の最小から最大の範囲の外と内に分ける。範囲は
// 両端を含み、Computer の名前で分けない。相手に無いチャネルのレコードは範囲の外である。
func TestCompareRecordsSplitsTheRecordsByTheComparedRange(t *testing.T) {
	const at = "2001-02-03T04:05:06Z"
	below := numberedEventXML("Security", "host-a.example.test", "5", at, "7")
	inside := numberedEventXML("Security", "host-a.example.test", "15", at, "7")
	otherComputer := numberedEventXML("Security", "host-b.example.test", "20", at, "7")
	otherChannel := numberedEventXML("System", "host-a.example.test", "15", at, "7")
	left := eventsXML(below, inside, otherComputer, otherChannel)
	right := eventsXML(
		numberedEventXML("Security", "host-a.example.test", "10", at, "7"),
		numberedEventXML("Security", "host-a.example.test", "12", at, "7"),
		numberedEventXML("Security", "host-a.example.test", "20", at, "7"))
	comparison, _ := fieldsResult(t, xmlInput("left", left), xmlInput("right", right)).CompareRecords("left", "right")
	if comparison.OnlyInSourceRecordCount != 4 || comparison.OnlyInSourceOutsideComparedRangeRecordCount != 2 ||
		comparison.OnlyInSourceInsideComparedRangeRecordCount != 2 {
		t.Fatalf("comparison = %+v", comparison)
	}
	want := []int64{offsetOf(t, left, inside), offsetOf(t, left, otherComputer)}
	if got := offsetsOf(comparison.OnlyInSourceInsideComparedRangeRecordRefs); !slices.Equal(got, want) {
		t.Errorf("inside the compared range at %v, want %v", got, want)
	}
	// 相手の 3 件はどれも、こちらの Security の番号 5 から 20 の範囲の内にある。
	if comparison.OnlyInComparedRecordCount != 3 || comparison.OnlyInComparedInsideSourceRangeRecordCount != 3 ||
		comparison.OnlyInComparedOutsideSourceRangeRecordCount != 0 {
		t.Errorf("the compared side = %+v", comparison)
	}
}

// 番号を持たない CSV は、時点の秒とプロバイダとイベント ID で突き合わせる。時点は分析者の
// 時刻の解釈で決まり、解釈が無ければ突き合わせない。
func TestCompareRecordsOfTheViewerCSVByInterpretedSecond(t *testing.T) {
	onlyCSV := viewerRow("2001/02/03 13:05:09", "8")
	csv := recordNumberViewerHeader +
		viewerRow("2001/02/03 13:05:06", "7") + viewerRow("2001/02/03 13:05:06", "7") +
		viewerRow("2001/02/03 13:05:08", "7") + onlyCSV
	onlyXML := numberedEventXML("Application", "host-a.example.test", "53", "2001-02-03T04:05:10Z", "7")
	xml := eventsXML(
		numberedEventXML("Application", "host-a.example.test", "51", "2001-02-03T04:05:06.9Z", "7"),
		numberedEventXML("Application", "host-a.example.test", "52", "2001-02-03T04:05:08Z", "7"),
		onlyXML)
	result := fieldsResult(t, csvInput("viewer", csv), xmlInput("events", xml))
	uninterpreted, _ := result.CompareRecords("viewer", "events")
	if uninterpreted.State != core.RecordComparisonNotCompared ||
		uninterpreted.NotComparedReason != core.RecordNotComparedTimeOffsetUndetermined {
		t.Fatalf("without an interpretation the comparison is %+v", uninterpreted)
	}
	interpreted := result.withTimeInterpretations(map[string]core.TimestampInterpretation{
		result.identities["viewer"].ContentSha256: {Offset: "+09:00", AssertionId: "assertion-1"},
	})
	comparison, _ := interpreted.CompareRecords("viewer", "events")
	if comparison.State != core.RecordComparisonCompared ||
		comparison.Key != core.RecordComparisonKeySecondProviderEvent || comparison.OneToOneKeyCount != 1 ||
		comparison.UndeterminedKeyCount != 1 || comparison.UndeterminedSourceRecordCount != 2 ||
		comparison.UndeterminedComparedRecordCount != 1 || comparison.UnequalKeyCount != 1 ||
		comparison.SourceSurplusRecordCount != 1 || comparison.ComparedSurplusRecordCount != 0 {
		t.Fatalf("comparison = %+v", comparison)
	}
	if got := offsetsOf(comparison.OnlyInSourceRecordRefs); !slices.Equal(got, []int64{offsetOf(t, csv, onlyCSV)}) {
		t.Errorf("only in the CSV at %v", got)
	}
	if got := offsetsOf(comparison.OnlyInComparedRecordRefs); !slices.Equal(got, []int64{offsetOf(t, xml, onlyXML)}) {
		t.Errorf("only in the XML at %v", got)
	}
	// CSV にしか無い 1 件は XML の秒の範囲の内、XML にしか無い 1 件は CSV の秒の範囲の外にある。
	if comparison.OnlyInSourceInsideComparedRangeRecordCount != 1 ||
		comparison.OnlyInSourceOutsideComparedRangeRecordCount != 0 ||
		comparison.OnlyInComparedOutsideSourceRangeRecordCount != 1 ||
		comparison.OnlyInComparedInsideSourceRangeRecordCount != 0 {
		t.Errorf("range split = %+v", comparison)
	}
	if uninterpreted.OnlyInSourceInsideComparedRangeRecordRefs == nil ||
		uninterpreted.OnlyInComparedInsideSourceRangeRecordRefs == nil {
		t.Error("the not compared result leaves the inside-range lists nil, want empty")
	}
	// 相手を入れ替えると、余りは相手の側の件数になる。
	swapped, _ := interpreted.CompareRecords("events", "viewer")
	if swapped.ComparedSurplusRecordCount != 1 || swapped.SourceSurplusRecordCount != 0 ||
		len(swapped.OnlyInSourceRecordRefs) != len(comparison.OnlyInComparedRecordRefs) {
		t.Errorf("the swapped comparison = %+v", swapped)
	}
}

// Windows イベントログでない形式は鍵の欄を持たない。
func TestCompareRecordsRejectsAFormatWithoutTheKey(t *testing.T) {
	result := fieldsResult(t,
		xmlInput("events", eventsXML(numberedEventXML("System", "host-a.example.test", "1", "2001-02-03T04:05:06Z", "7"))),
		squidInput("proxy", "981173106.000     10 192.0.2.20 TCP_MISS/200 100 GET http://example.test/ - HIER_DIRECT/198.51.100.9 text/html\n"))
	comparison, found := result.CompareRecords("events", "proxy")
	if !found || comparison.State != core.RecordComparisonNotCompared ||
		comparison.NotComparedReason != core.RecordNotComparedNotDeclared || comparison.Key != "" {
		t.Errorf("comparison = %+v", comparison)
	}
}

// largeNumberParser は、EVTX の形式の宣言を名乗り、番号と見出しの番号を差し替えたレコードを
// 返す。番号とホスト名である。
type largeNumberParser struct {
	numbers    []string
	nextNumber string
	at         int
}

func (p *largeNumberParser) Identity() ParserIdentity {
	return NewTestParser(WindowsEVTXFormatKey, nil).Identity()
}

func (p *largeNumberParser) Reset(input io.Reader) {
	p.at = 0
	_, _ = io.Copy(io.Discard, input)
}

func (p *largeNumberParser) Next() (ParsedRecord, *core.ImportFailure, error) {
	if p.at >= len(p.numbers) {
		return ParsedRecord{}, nil, io.EOF
	}
	names, number := p.Identity().RecordNumbering, p.numbers[p.at]
	var fields []core.RecordField
	for name, text := range map[string]string{
		names.Channel: "System", names.Computer: "host-a.example.test",
		names.EventRecordID: number, names.RecordHeaderID: number,
	} {
		value, _ := core.NewRawValue(core.ValueStatePresent, text)
		field, _ := core.NewTextField(name, "", value)
		fields = append(fields, field)
	}
	length := int64(10)
	record := ParsedRecord{
		// 行番号は test の原文の参照 (indexRawTextRef) が探す鍵である。
		RawText: "<Event/>", ByteOffset: int64(p.at * 10), ByteLength: &length, LineNumber: int64(p.at + 1),
		Semantics: &RecordSemantics{Fields: fields},
	}
	p.at++
	return record, nil, nil
}

func (p *largeNumberParser) SourceHeader() []core.RecordField {
	names := p.Identity().RecordNumbering
	next, _ := core.NewRawValue(core.ValueStatePresent, p.nextNumber)
	dirty, _ := core.NewRawValue(core.ValueStatePresent, "false")
	nextField, _ := core.NewTextField(names.HeaderNextRecordID, "", next)
	dirtyField, _ := core.NewTextField(names.HeaderDirty, "", dirty)
	return []core.RecordField{nextField, dirtyField}
}

// jsonNumbersOf は JSON の値の中の数を、文字列のまま集める。
func jsonNumbersOf(value any, into []json.Number) []json.Number {
	switch typed := value.(type) {
	case json.Number:
		return append(into, typed)
	case map[string]any:
		for _, item := range typed {
			into = jsonNumbersOf(item, into)
		}
	case []any:
		for _, item := range typed {
			into = jsonNumbersOf(item, into)
		}
	}
	return into
}

// 2^53 以上の番号は読めない番号として数え、応答の数はどれも画面が丸めずに読める整数に収まる。
func TestRecordNumbersKeepTheResponseWithinTheSafeIntegers(t *testing.T) {
	beyondInt64 := strconv.FormatUint(math.MaxInt64+1, 10)
	parser := &largeNumberParser{
		numbers:    []string{"3", "5", beyondInt64, strconv.FormatUint(1<<53, 10)},
		nextNumber: strconv.FormatUint(math.MaxInt64+2, 10),
	}
	result := fieldsResult(t, fieldsSourceInput{
		id: "large", fileName: "large.evtx", format: WindowsEVTXFormatKey, parser: parser,
		content: strings.Repeat("\x00", 40),
	})
	numbers, found := result.RecordNumbers("large")
	if !found || numbers.UnreadableRecordCount == nil || *numbers.UnreadableRecordCount != 2 ||
		numbers.FileHeader == nil || numbers.FileHeader.UnreadableRecordHeaderCount != 2 ||
		numbers.FileHeader.Comparison != core.RecordNumberHeaderUnreadable {
		t.Fatalf("numbers = %+v header %+v", numbers, numbers.FileHeader)
	}
	encoded, err := json.Marshal(numbers)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	found = false
	for _, number := range jsonNumbersOf(decoded, nil) {
		value, err := strconv.ParseUint(number.String(), 10, 64)
		if err != nil || value > 1<<53-1 {
			t.Errorf("the response carries %s beyond the safe integers", number)
		}
		found = found || number.String() == "4"
	}
	if !found {
		t.Errorf("the response %s carries no missing number 4", encoded)
	}
}

// 件数の違う鍵のレコードを、鍵に入れていない語彙の項目の値で対にし、片方にだけあるレコードを
// 特定する。形式ごとに文字列が違う項目は比べる項目から外れる。
func TestResolveUnequalKeyIdentifiesTheRecordByValues(t *testing.T) {
	record := func(line int64, port, protocol string) RecordEntry {
		fields := []core.RecordField{
			textField("SourcePort", core.SemanticKeyConnectionSourcePort, presentText(port)),
			textField("Protocol", core.SemanticKeyConnectionProtocol, presentText(protocol)),
		}
		return RecordEntry{Locator: core.RecordLocator{LineNumber: &line}, Semantics: &RecordSemantics{Fields: fields}}
	}
	// プロトコルは片方が番号、もう片方が名前で書き、値が一致しない。
	longer := []RecordEntry{record(1, "50001", "6"), record(2, "50002", "6"), record(3, "50003", "6")}
	shorter := []RecordEntry{record(7, "50001", "TCP"), record(8, "50002", "TCP")}
	resolved := resolveUnequalKey(longer, shorter, []int{0, 1, 2}, []int{0, 1})
	if resolved.Outcome != core.UnequalKeyIdentified || len(resolved.OnlyInSourceRecordRefs) != 1 ||
		*resolved.OnlyInSourceRecordRefs[0].LineNumber != 3 || len(resolved.OnlyInComparedRecordRefs) != 0 ||
		!slices.Equal(resolved.ComparedSemantics, []core.SemanticKey{core.SemanticKeyConnectionSourcePort}) ||
		len(resolved.SourceRecordRefs) != 3 || len(resolved.ComparedRecordRefs) != 2 {
		t.Errorf("resolved = %+v", resolved)
	}
	// 相手の側から見ても同じレコードを特定する。
	if reversed := resolveUnequalKey(shorter, longer, []int{0, 1}, []int{0, 1, 2}); reversed.Outcome != core.UnequalKeyIdentified ||
		len(reversed.OnlyInComparedRecordRefs) != 1 || *reversed.OnlyInComparedRecordRefs[0].LineNumber != 3 {
		t.Errorf("reversed = %+v", reversed)
	}
	// 同じ値の組のレコードが同じ側に 2 件あるときは 1 つに決めない。
	twins := []RecordEntry{record(1, "50001", "6"), record(2, "50001", "6"), record(3, "50002", "6")}
	if got := resolveUnequalKey(twins, shorter, []int{0, 1, 2}, []int{0, 1}); got.Outcome != core.UnequalKeyUndecided {
		t.Errorf("twins = %+v, want undecided", got)
	}
	// どの項目の値も両側で揃わないときは、値で比べない。
	other := []RecordEntry{record(7, "60001", "TCP"), record(8, "60002", "TCP")}
	if got := resolveUnequalKey(longer, other, []int{0, 1, 2}, []int{0, 1}); got.Outcome != core.UnequalKeyNoComparedValues ||
		len(got.OnlyInSourceRecordRefs) != 0 || len(got.ComparedSemantics) != 0 {
		t.Errorf("no compared values = %+v", got)
	}
}
