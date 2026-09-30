// in-package test: 非公開の 3 つの意味の状態の表が同じ判断を返すことを検査する。
package markii

import (
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 通信の表と、種別に依らない観測の 2 つの表が、net の subEvt で同じ状態を返す。
func TestCommunicationSubEventStatusAgreesWithTheDocumentedTable(t *testing.T) {
	documented := documentedSubEvents[communicationEvent]
	if len(documented) == 0 {
		t.Fatalf("the documented table carries no subEvt for %q", communicationEvent)
	}
	inferred := inferredSubEvents[communicationEvent]
	if len(inferred) == 0 {
		t.Fatalf("the inferred table carries no subEvt for %q", communicationEvent)
	}
	for subEvent, want := range communicationSubEventStatus {
		got := core.ObservationKindStatusUndetermined
		if _, inInferredTable := inferred[subEvent]; inInferredTable {
			got = core.ObservationKindStatusInferred
		}
		if _, inDocumentedTable := documented[subEvent]; inDocumentedTable {
			got = core.ObservationKindStatusDetermined
		}
		if got != want {
			t.Errorf("%s/%s: the shared tables say %q, the communication table says %q",
				communicationEvent, subEvent, got, want)
		}
	}
}

// 推定した組が documentedSubEvents と重ならず、意味と根拠をどちらも持つ。
func TestInferredSubEventsDoNotOverlapTheDocumentedTable(t *testing.T) {
	for event, subEvents := range inferredSubEvents {
		for subEvent, inferred := range subEvents {
			if _, documented := documentedSubEvents[event][subEvent]; documented {
				t.Errorf("%s/%s is in both the documented table and the inferred table",
					event, subEvent)
			}
			if inferred.meaning == "" || inferred.evidence == "" {
				t.Errorf("%s/%s carries meaning %q and evidence %q, want both",
					event, subEvent, inferred.meaning, inferred.evidence)
			}
		}
	}
}

// プロセス開始の組はバージョンごとの表の和集合にある。
//
// observationKindOf は determined を定数で返す。表から ps/start が消えると、
// プロセス開始だけが確定のまま残る。
func TestProcessStartKindIsInTheDocumentedTable(t *testing.T) {
	subEvents, documented := documentedSubEvents[processStartEvent]
	if !documented {
		t.Fatalf("the documented table carries no subEvt for %q", processStartEvent)
	}
	if _, documented := subEvents[processStartSubEvent]; !documented {
		t.Fatalf("the documented table has no %s/%s", processStartEvent, processStartSubEvent)
	}
}

// wantDocumentedSubEventsV30 は V3.0 の Recorder が出力する evt と subEvt の組である。
//
// **実装の表と同じ文字列を独立に置く。** 個数だけを数えると、chgConf を chgConfig と
// 取り違えた表が通る。一覧は testdata/record-observation-manifest.json の
// documentedEventKinds が持つ。
var wantDocumentedSubEventsV30 = map[string][]string{
	"sys":     {"start", "stop", "run", "chgConf", "outDsk", "startRed", "stopRed"},
	"os":      {"startSafemode", "stopSafemode", "suspend", "resume", "chgDate", "evtLog"},
	"dev":     {"mnt", "unMnt", "mntd"},
	"session": {"login", "loginFail", "loginR", "loginRFail", "logout", "lock", "unlock", "conR", "dconR", "startSCR", "stopSCR"},
	"ps":      {"start", "stop", "run", "inject", "guardInject"},
	"file": {"close", "del", "rename", "copy", "chgAttr", "delDir", "renameDir",
		"impWPD", "expWPD", "renameWPD", "delWPD", "download", "unblock", "enableMacro"},
	"reg":             {"setVal", "delKey", "delVal", "renameKey"},
	"prt":             {"create"},
	"win":             {"active"},
	"clip":            {"paste"},
	"net":             {"con", "acpt", "dcon", "lsn", "webURL", "chgSSID", "dnsQuery", "mailSend", "mailRecv"},
	"windowsDefender": {"dtctMalState"},
}

// wantDocumentedSubEventsV32 は V3.2 の Recorder が出力する evt と subEvt の組である。
var wantDocumentedSubEventsV32 = map[string][]string{
	"sys": {"start", "stop", "run", "chgConf", "outDsk", "startRed", "stopRed",
		"suspendLog", "resumeLog", "restart", "suspendPSMon", "resumePSMon"},
	"os": {"startSafemode", "stopSafemode", "suspend", "resume", "chgDate", "evtLog",
		"start", "stop"},
	"dev":     {"mnt", "unMnt", "mntd", "con", "dcon"},
	"session": {"login", "loginFail", "loginR", "loginRFail", "logout", "lock", "unlock", "conR", "dconR", "startSCR", "stopSCR"},
	"ps":      {"start", "stop", "run", "inject", "guardInject"},
	"file": {"close", "del", "rename", "copy", "chgAttr", "delDir", "renameDir",
		"impWPD", "expWPD", "renameWPD", "delWPD", "download", "unblock", "enableMacro",
		"upload"},
	"reg":        {"setVal", "delKey", "delVal", "renameKey"},
	"prt":        {"create"},
	"win":        {"active", "inactive", "psActive", "psInactive"},
	"clip":       {"copy", "paste"},
	"net":        {"con", "acpt", "dcon", "lsn", "webURL", "chgSSID", "dnsQuery", "mailSend", "mailRecv", "login"},
	"wmi":        {"psCreate"},
	"powerShell": {"exec"},
	"windowsDefender": {"scnComplete", "scnCancel", "scnFail", "qrtnRestore",
		"qrtnRestoreFail", "qrtnDel", "qrtnDelFail", "dtctMalState", "malStateActTaken",
		"malStateActFail", "malStateActCritFail", "ASRBlock", "ASRAudit", "sigUpdate",
		"sigUpdateFail", "engUpdate", "engUpdateFail", "chgConf", "smartScrnApp",
		"smartScrnUri", "smartScrnUsr"},
}

// バージョンごとの表が、期待値の文字列そのもので一致する。
func TestDocumentedSubEventsOfEveryVersionCarriesTheWholeTable(t *testing.T) {
	for _, table := range []struct {
		version string
		got     map[string]map[string]struct{}
		want    map[string][]string
	}{
		{"V3.0", documentedSubEventsV30, wantDocumentedSubEventsV30},
		{"V3.2", documentedSubEventsV32, wantDocumentedSubEventsV32},
	} {
		t.Run(table.version, func(t *testing.T) {
			if !reflect.DeepEqual(sortedSubEvents(table.got), sortedNames(table.want)) {
				t.Errorf("the table = %v, want %v",
					sortedSubEvents(table.got), sortedNames(table.want))
			}
		})
	}
}

// 和集合の表が、バージョンごとの期待値の和集合の文字列そのもので一致する。
func TestDocumentedSubEventsIsTheUnionOfTheVersionTables(t *testing.T) {
	want := map[string][]string{}
	for _, table := range []map[string][]string{
		wantDocumentedSubEventsV30, wantDocumentedSubEventsV32,
	} {
		for event, subEvents := range table {
			want[event] = append(want[event], subEvents...)
		}
	}
	for event, subEvents := range want {
		sort.Strings(subEvents)
		want[event] = slices.Compact(subEvents)
	}
	if !reflect.DeepEqual(sortedSubEvents(documentedSubEvents), want) {
		t.Errorf("the table = %v, want %v", sortedSubEvents(documentedSubEvents), want)
	}
}

// バージョンごとの表は和集合を組んだ後も変わらない。
func TestUnionOfSubEventTablesLeavesTheVersionTablesUntouched(t *testing.T) {
	if _, merged := documentedSubEventsV30["win"]["psInactive"]; merged {
		t.Error("the V3.0 table gained win/psInactive from the union")
	}
	if _, merged := documentedSubEventsV32["clip"]["paste"]; !merged {
		t.Error("the V3.2 table lost clip/paste")
	}
	if _, present := documentedSubEventsV30["powerShell"]; present {
		t.Error("the V3.0 table gained the powerShell event from the union")
	}
}

// sortedSubEvents は表の subEvt を evt ごとに並べた一覧へ直す。
func sortedSubEvents(table map[string]map[string]struct{}) map[string][]string {
	sorted := map[string][]string{}
	for event, subEvents := range table {
		names := make([]string, 0, len(subEvents))
		for subEvent := range subEvents {
			names = append(names, subEvent)
		}
		sort.Strings(names)
		sorted[event] = names
	}
	return sorted
}

// sortedNames は期待値の一覧を evt ごとに並べ替える。
func sortedNames(table map[string][]string) map[string][]string {
	sorted := map[string][]string{}
	for event, subEvents := range table {
		names := slices.Clone(subEvents)
		sort.Strings(names)
		sorted[event] = names
	}
	return sorted
}
