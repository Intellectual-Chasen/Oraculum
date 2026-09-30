package winevent_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/winevent"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// taskSchedulerEvent はタスク スケジューラのイベント 1 件である。
func taskSchedulerEvent(eventID string, data ...string) string {
	document := `<Event><System><Provider Name="Microsoft-Windows-TaskScheduler"/>` +
		`<EventID>` + eventID + `</EventID><TimeCreated SystemTime="2001-02-03T04:05:06Z"/>` +
		`<EventRecordID>401</EventRecordID><Channel>Microsoft-Windows-TaskScheduler/Operational</Channel>` +
		`<Computer>host04.example.test</Computer></System><EventData>`
	for index := 0; index+1 < len(data); index += 2 {
		document += `<Data Name="` + data[index] + `">` + data[index+1] + `</Data>`
	}
	return document + `</EventData></Event>`
}

// 106 の TaskName は登録されたタスク、100 の TaskName は起動されたタスクの名前である。
// ほかのイベントの TaskName には語彙を与えない。
func TestObserveMapsTheTaskNamesOfRegistrationAndStart(t *testing.T) {
	for eventID, want := range map[string]core.SemanticKey{
		"106": core.SemanticKeyScheduledTaskName,
		"100": core.SemanticKeyStartedTaskName,
		"102": "",
	} {
		observation := observe(t, taskSchedulerEvent(eventID,
			"TaskName", `\Example Task`, "UserContext", `EXAMPLE\user41`))
		requireSemantics(t, observation,
			semanticItem{"EventData.TaskName", want, ""},
			semanticItem{"EventData.UserContext", "", ""})
	}
}

// 129 と 200 の TaskName は起動したタスクの名前である。129 の ProcessID と Path は作成した
// プロセスの番号と実行ファイルであり、129 は起動の記録にしない。200 の ActionName は語彙を
// 持たない。
func TestObserveMapsTheTaskProcessAndActionRecords(t *testing.T) {
	created := observe(t, taskSchedulerEvent("129",
		"TaskName", `\Grp\Job1`, "Path", `X:\d\job1.exe`, "ProcessID", "4101", "Priority", "16384"))
	requireSemantics(t, created,
		semanticItem{"EventData.TaskName", core.SemanticKeyStartedTaskName, `\Grp\Job1`},
		semanticItem{"EventData.Path", core.SemanticKeyProcessBinaryPath, `X:\d\job1.exe`},
		semanticItem{"EventData.ProcessID", core.SemanticKeyProcessPid, "4101"})
	if created.ProcessStart {
		t.Error("129 is a process start record")
	}
	action := observe(t, taskSchedulerEvent("200",
		"TaskName", `\Grp\Job1`, "ActionName", `X:\d\job1.exe`, "EnginePID", "4101"))
	requireSemantics(t, action,
		semanticItem{"EventData.TaskName", core.SemanticKeyStartedTaskName, `\Grp\Job1`},
		semanticItem{"EventData.ActionName", "", ""})
}

// 129 が前に付ける `NT TASK` を外した名前を比べ、原資料の文字列は保つ。付けない名前はそのまま比べる。
func TestObserveComparesTheTaskNameWithoutTheNtTaskPrefix(t *testing.T) {
	for raw, want := range map[string]string{
		`NT TASK\Grp\Job1`: `\Grp\Job1`,
		`\Job1`:            `\Job1`,
		`NT TASKJob1`:      `NT TASKJob1`,
	} {
		field := fieldNamed(t, observe(t, taskSchedulerEvent("129", "TaskName", raw)).Fields, "EventData.TaskName")
		if value, _ := field.Text.ComparableValue(); value != want {
			t.Errorf("%s comparable = %q, want %q", raw, value, want)
		}
		if text, _ := field.Text.RawTextValue(); text != raw {
			t.Errorf("%s raw = %q", raw, text)
		}
	}
}

// observeTaskSchedulerViewer は、タスク スケジューラの説明を持つイベントビューアーの CSV の
// 1 件を読む。
func observeTaskSchedulerViewer(t *testing.T, eventID, description string) winevent.Observation {
	t.Helper()
	events, failures := readAllCSV(t, levelHeader+viewerRecord("情報", "2001/02/03 04:05:06",
		"Microsoft-Windows-TaskScheduler", eventID, "", description))
	if len(failures) != 0 || len(events) != 1 {
		t.Fatalf("events = %d, failures = %+v; want 1 event", len(events), failures)
	}
	observation, failure := winevent.Observe(events[0])
	if failure != nil {
		t.Fatalf("Observe() failure = %+v", failure)
	}
	return observation
}

// CSV の 100 と 106 の説明の文から、XML と同じ名前の値を取り出し、同じ語彙を与える。
func TestObserveReadsTheTaskSchedulerSentencesOfTheViewer(t *testing.T) {
	started := observeTaskSchedulerViewer(t, "100",
		`タスク スケジューラは、ユーザー "EXAMPLE\user41" の "\Example Task" タスクの "{00000000-0000-0000-0000-000000000041}" インスタンスを開始しました。`)
	requireSemantics(t, started,
		semanticItem{"EventData.TaskName", core.SemanticKeyStartedTaskName, `\Example Task`},
		semanticItem{"EventData.UserContext", "", ""},
		semanticItem{"EventData.InstanceId", "", ""})
	registered := observeTaskSchedulerViewer(t, "106",
		`ユーザー "EXAMPLE\user41" はタスク スケジューラのタスク "\Example Task" を登録しました。`)
	requireSemantics(t, registered,
		semanticItem{"EventData.TaskName", core.SemanticKeyScheduledTaskName, `\Example Task`})
	created := observeTaskSchedulerViewer(t, "129",
		`タスク スケジューラは、プロセス ID 4101 でタスク "\Example Task"、インスタンス "X:\d\job1.exe" を起動しました。`)
	requireSemantics(t, created,
		semanticItem{"EventData.TaskName", core.SemanticKeyStartedTaskName, `\Example Task`},
		semanticItem{"EventData.Path", core.SemanticKeyProcessBinaryPath, `X:\d\job1.exe`},
		semanticItem{"EventData.ProcessID", core.SemanticKeyProcessPid, "4101"})
}

// 文が違う説明は、欄を持たないレコードとして取り込まれ、失敗にならない。
func TestObserveKeepsAnUnknownTaskSchedulerSentenceWithoutFields(t *testing.T) {
	observation := observeTaskSchedulerViewer(t, "100",
		`Task Scheduler started "{00000000-0000-0000-0000-000000000042}" instance of the "\Example Task" task.`)
	for _, field := range observation.Fields {
		if field.Name == "EventData.TaskName" {
			t.Errorf("the unknown sentence yields %+v, want no task name", field)
		}
	}
}
