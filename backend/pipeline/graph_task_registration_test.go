// in-package test: タスクの登録と起動を結ぶ候補を確かめる。
package pipeline

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 起動と同じレコードの登録は、時刻と端末の条件を満たしても有効なバージョンにしない。
// 今の入力形式に登録と起動の両方の意味を持つレコードが無いため、選ぶ関数を直接呼ぶ。
func TestEffectiveRegistrationsSkipTheRunRecordItself(t *testing.T) {
	at := time.Date(2001, 2, 3, 4, 5, 0, 0, time.UTC)
	run := logonSessionSide{recordAt: 7, terminal: "terminal-a", at: at, timed: true}
	other := logonSessionSide{recordAt: 6, terminal: "terminal-a", at: at, timed: true}
	got := effectiveRegistrationsOf([]logonSessionSide{run, other}, run)
	if len(got) != 1 || got[0].recordAt != other.recordAt {
		t.Errorf("effective registrations = %v, want only the record %d", got, other.recordAt)
	}
}

// 同じ端末の登録と起動は、収集元が違っても結ぶ。登録と起動は別のログに記録される。
func TestEffectiveRegistrationsCrossSources(t *testing.T) {
	at := time.Date(2001, 2, 3, 4, 5, 0, 0, time.UTC)
	run := logonSessionSide{recordAt: 7, terminal: "terminal-a", source: "source-b", at: at.Add(time.Minute), timed: true}
	registration := logonSessionSide{recordAt: 6, terminal: "terminal-a", source: "source-a", at: at, timed: true}
	got := effectiveRegistrationsOf([]logonSessionSide{registration}, run)
	if len(got) != 1 || got[0].recordAt != registration.recordAt {
		t.Errorf("effective registrations = %v, want the record %d", got, registration.recordAt)
	}
}

// 観測の種別の欄の値が無い場合と、欄があって文字列を持たない場合は、別の文字列になる。
func TestObservationKindKeySeparatesMissingTextFromMissingRaw(t *testing.T) {
	missingText := core.ObservationKind{Raw: []core.RecordField{{Name: "EventID"}}}
	missingRaw := core.ObservationKind{Raw: []core.RecordField{
		{Name: "EventID", Text: &core.RawAndNormalized{ValueState: core.ValueStateAbsent}}}}
	emptyRaw := ""
	emptyText := core.ObservationKind{Raw: []core.RecordField{
		{Name: "EventID", Text: &core.RawAndNormalized{RawText: &emptyRaw, ValueState: core.ValueStatePresent}}}}
	keys := []string{observationKindKeyOf(missingText), observationKindKeyOf(missingRaw), observationKindKeyOf(emptyText)}
	if keys[0] == keys[1] || keys[1] == keys[2] || keys[0] == keys[2] {
		t.Errorf("observation kind keys = %q, want three different keys", keys)
	}
}

// 同じ種別で最も遅い時刻の登録が 2 件以上あるときは、そのすべてを返し、前の時刻の登録を外す。
func TestEffectiveRegistrationsKeepEveryLatestTie(t *testing.T) {
	at := time.Date(2001, 2, 3, 4, 5, 0, 0, time.UTC)
	run := logonSessionSide{recordAt: 9, terminal: "terminal-a", at: at.Add(time.Minute), timed: true}
	earlier := logonSessionSide{recordAt: 5, terminal: "terminal-a", at: at.Add(-time.Minute), timed: true}
	first := logonSessionSide{recordAt: 6, terminal: "terminal-a", at: at, timed: true}
	second := logonSessionSide{recordAt: 7, terminal: "terminal-a", at: at, timed: true}
	got := effectiveRegistrationsOf([]logonSessionSide{earlier, first, second}, run)
	if len(got) != 2 || got[0].recordAt != first.recordAt || got[1].recordAt != second.recordAt {
		t.Errorf("effective registrations = %v, want the records %d and %d", got, first.recordAt, second.recordAt)
	}
}

// taskEventXML は、プロバイダ provider のイベント 1 件を返す。
func taskEventXML(provider, recordID, eventID, computer, systemTime string, data ...string) string {
	var elements strings.Builder
	for at := 0; at+1 < len(data); at += 2 {
		elements.WriteString(`<Data Name="` + data[at] + `">` + data[at+1] + `</Data>`)
	}
	return `<Event><System><Provider Name="` + provider + `"/><EventID>` + eventID +
		`</EventID><TimeCreated SystemTime="` + systemTime + `"/><EventRecordID>` + recordID +
		`</EventRecordID><Channel>Example</Channel><Computer>` + computer + `</Computer></System>` +
		`<EventData>` + elements.String() + `</EventData></Event>` + "\n"
}

const taskSchedulerProvider = "Microsoft-Windows-TaskScheduler"

// 同じ端末の 4698 と 106 の登録は、大文字と小文字の違う名前の 100 の起動へ、それぞれ候補を
// 張る。根拠は登録と起動の 2 件である。起動より後の登録と、別の端末の起動は結ばない。
func TestTaskRegistrationLinksTheRunsOfTheSameName(t *testing.T) {
	document := "<Events>\n" +
		securityEventXML("111", "4698", logonHostA, "2001-02-03T04:05:00.000Z", "TaskName", `\Example Task`) +
		taskEventXML(taskSchedulerProvider, "112", "106", logonHostA, "2001-02-03T04:05:00.000Z",
			"TaskName", `\Example Task`) +
		taskEventXML(taskSchedulerProvider, "113", "100", logonHostA, "2001-02-03T04:06:00.000Z",
			"TaskName", `\example task`) +
		taskEventXML(taskSchedulerProvider, "114", "100", logonHostB, "2001-02-03T04:06:00.000Z",
			"TaskName", `\Example Task`) +
		securityEventXML("115", "4698", logonHostA, "2001-02-03T04:07:00.000Z", "TaskName", `\Example Task`) +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	node := func(recordID string) int { return graph.records[recordOfEvent(t, graph, recordID)].recordNode }
	var pairs [][2]int
	for _, edge := range graph.edges {
		if edge.kind != core.EdgeKindTaskRegistrationRun {
			continue
		}
		pairs = append(pairs, [2]int{edge.source, edge.target})
		if edge.state != core.RelationStateCandidate || len(edge.evidence) != 2 {
			t.Errorf("edge %s is %s with %d evidence, want a candidate with two records",
				edge.id, edge.state, len(edge.evidence))
		}
	}
	want := [][2]int{{node("111"), node("113")}, {node("112"), node("113")}}
	slices.SortFunc(pairs, func(left, right [2]int) int { return left[0] - right[0] })
	slices.SortFunc(want, func(left, right [2]int) int { return left[0] - right[0] })
	if !slices.Equal(pairs, want) {
		t.Errorf("task_registration_run = %v, want %v", pairs, want)
	}
	requireRecordPairs(t, graph, core.EdgeKindTaskRegistrationRun,
		core.EdgePairConditionTaskName, core.EdgePairConditionTerminal, core.EdgePairConditionTimeOrder)
}

// 4698 と 106 が同じ登録を違う時刻で記録したときは、観測の種別ごとに直前の登録を選び、
// 起動を両方と結ぶ。同じ種別の登録し直しは、その種別の前の登録を外す。
func TestTaskRegistrationRunChoosesTheVersionPerObservationKind(t *testing.T) {
	document := "<Events>\n" +
		securityEventXML("131", "4698", logonHostA, "2001-02-03T04:04:00.000Z", "TaskName", `\Example Task`) +
		securityEventXML("132", "4698", logonHostA, "2001-02-03T04:05:00.100Z", "TaskName", `\Example Task`) +
		taskEventXML(taskSchedulerProvider, "133", "106", logonHostA, "2001-02-03T04:05:00.300Z",
			"TaskName", `\Example Task`) +
		taskEventXML(taskSchedulerProvider, "134", "100", logonHostA, "2001-02-03T04:06:00.000Z",
			"TaskName", `\Example Task`) +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	node := func(recordID string) int { return graph.records[recordOfEvent(t, graph, recordID)].recordNode }
	var pairs [][2]int
	for _, edge := range graph.edges {
		if edge.kind == core.EdgeKindTaskRegistrationRun {
			pairs = append(pairs, [2]int{edge.source, edge.target})
		}
	}
	// **並べ替えずに比べる。** エッジは観測の種別に依らず登録のレコードの位置の順に足す。
	want := [][2]int{{node("132"), node("134")}, {node("133"), node("134")}}
	if !slices.Equal(pairs, want) {
		t.Errorf("task_registration_run = %v, want %v", pairs, want)
	}
	requireRecordPairs(t, graph, core.EdgeKindTaskRegistrationRun,
		core.EdgePairConditionTaskName, core.EdgePairConditionTerminal, core.EdgePairConditionTimeOrder)
}

// 登録し直したタスクの起動は、起動の時刻で有効な直前の登録だけと結ぶ。再登録の前の起動は
// 前の登録と、再登録の後の起動は再登録とだけ結ぶ。登録の無い名前の起動は結ばない。
func TestTaskRegistrationRunLinksOnlyTheEffectiveVersion(t *testing.T) {
	document := "<Events>\n" +
		securityEventXML("121", "4698", logonHostA, "2001-02-03T04:05:00.000Z", "TaskName", `\Example Task`) +
		taskEventXML(taskSchedulerProvider, "122", "100", logonHostA, "2001-02-03T04:06:00.000Z",
			"TaskName", `\Example Task`) +
		securityEventXML("123", "4698", logonHostA, "2001-02-03T04:07:00.000Z", "TaskName", `\Example Task`) +
		taskEventXML(taskSchedulerProvider, "124", "100", logonHostA, "2001-02-03T04:08:00.000Z",
			"TaskName", `\Example Task`) +
		taskEventXML(taskSchedulerProvider, "125", "100", logonHostA, "2001-02-03T04:09:00.000Z",
			"TaskName", `\Unregistered Task`) +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	node := func(recordID string) int { return graph.records[recordOfEvent(t, graph, recordID)].recordNode }
	var pairs [][2]int
	for _, edge := range graph.edges {
		if edge.kind == core.EdgeKindTaskRegistrationRun {
			pairs = append(pairs, [2]int{edge.source, edge.target})
		}
	}
	want := [][2]int{{node("121"), node("122")}, {node("123"), node("124")}}
	slices.SortFunc(pairs, func(left, right [2]int) int { return left[0] - right[0] })
	slices.SortFunc(want, func(left, right [2]int) int { return left[0] - right[0] })
	if !slices.Equal(pairs, want) {
		t.Errorf("task_registration_run = %v, want %v", pairs, want)
	}
	requireRecordPairs(t, graph, core.EdgeKindTaskRegistrationRun,
		core.EdgePairConditionTaskName, core.EdgePairConditionTerminal, core.EdgePairConditionTimeOrder)
}

// イベントビューアーの CSV の 106 と 100 の説明の文から、同じ file の中で候補が張られる。
func TestTaskRegistrationLinksTheViewerSentences(t *testing.T) {
	csv := "レベル,日付と時刻,ソース,イベント ID,タスクのカテゴリ\n" +
		"情報,2001/02/03 13:06:00," + taskSchedulerProvider + ",100,,\"タスク スケジューラは、ユーザー \"\"EXAMPLE\\user41\"\" の \"\"\\Example Task\"\" タスクの \"\"{00000000-0000-0000-0000-000000000041}\"\" インスタンスを開始しました。\"\n" +
		"情報,2001/02/03 13:05:00," + taskSchedulerProvider + ",106,,\"ユーザー \"\"EXAMPLE\\user41\"\" はタスク スケジューラのタスク \"\"\\Example Task\"\" を登録しました。\"\n"
	graph := NewGraph(sigmaImportResult(t, WindowsEventCSVFormatKey, "tasks.csv", csv), AllMatchConditions())
	var edges int
	for _, edge := range graph.edges {
		if edge.kind == core.EdgeKindTaskRegistrationRun {
			edges++
			if len(edge.evidence) != 2 {
				t.Errorf("the edge carries %d records, want the registration and the run", len(edge.evidence))
			}
		}
	}
	if edges != 1 {
		t.Errorf("task_registration_run = %d edges, want 1 from the registration to the run", edges)
	}
}
