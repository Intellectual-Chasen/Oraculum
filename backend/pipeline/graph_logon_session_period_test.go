// in-package test: Windows イベントログと markii 形式のレコードから、端末ごとのログオンの
// セッションの期間と、始まりと終わりを決めた記録の種類を確かめる。
package pipeline

import (
	"slices"
	"testing"
	"time"
)

// providerEventXML は、プロバイダを差し替えた、EventData を持たないイベント 1 件を返す。
func providerEventXML(provider, recordID, eventID, computer, systemTime string) string {
	return `<Event><System><Provider Name="` + provider + `"/><EventID>` + eventID +
		`</EventID><TimeCreated SystemTime="` + systemTime + `"/><EventRecordID>` + recordID +
		`</EventRecordID><Channel>System</Channel><Computer>` + computer + `</Computer></System></Event>` + "\n"
}

// periodSummary はセッションの期間を、始まりの記録の番号と時刻と種類で比べる形である。
type periodSummary struct {
	startRecord string
	start, end  string
	startKind   sessionStartKind
	endKind     sessionEndKind
}

// recordNameOf は、g.records の at のレコードの EventRecordID を返す。EventRecordID を持たない
// レコードではレコードのノードの通番の値を返す。
func recordNameOf(t *testing.T, graph Graph, at int) string {
	t.Helper()
	node := graph.records[at].recordNode
	for _, attribute := range graph.nodes[node].attributes {
		if attribute.field.Name == "EventRecordID" {
			value, _ := attribute.field.Text.RawTextValue()
			return value
		}
	}
	return recordLabel(t, graph, node)
}

// periodsByRecord は、グラフのセッションの期間を、始まりの記録の番号で探せる形にして並びのまま返す。
func periodsByRecord(t *testing.T, graph Graph, result ImportResult, limit time.Duration) []periodSummary {
	t.Helper()
	var summaries []periodSummary
	for _, period := range graph.logonSessionPeriodsOf(result, limit) {
		summaries = append(summaries, periodSummary{
			startRecord: recordNameOf(t, graph, period.startAt),
			start:       period.start.UTC().Format(time.RFC3339),
			end:         period.end.UTC().Format(time.RFC3339),
			startKind:   period.startKind, endKind: period.endKind,
		})
	}
	return summaries
}

// 始まりはログオン、無いときは最初の操作である。終わりはログオフ、無いときは次の起動、無いときは
// 最後の操作から T の後である。起動をまたいだ同じ Logon ID は別のセッションである。ログオフだけの
// Logon ID、時点を持たない操作だけの Logon ID、固定の値の Logon ID、端末の起動の記録そのものは
// セッションにならない。
func TestLogonSessionPeriodsFollowTheRecordsThatEndThem(t *testing.T) {
	const limit = 2 * time.Hour
	document := "<Events>\n" +
		// 0xa1: ログオン、操作、ログオフ。ログオフの後の操作は期間に入らない。
		securityEventXML("51", "4624", logonHostA, "2001-02-03T04:00:00.000Z", "TargetLogonId", "0xa1", "LogonType", "3") +
		securityEventXML("52", "4672", logonHostA, "2001-02-03T04:01:00.000Z", "SubjectLogonId", "0xa1") +
		securityEventXML("53", "4634", logonHostA, "2001-02-03T04:02:00.000Z", "TargetLogonId", "0xa1") +
		// 0xa2: ログオンの後に起動し、起動の後に同じ Logon ID のログオンがある。
		securityEventXML("54", "4624", logonHostA, "2001-02-03T04:10:00.000Z", "TargetLogonId", "0xa2", "LogonType", "2") +
		securityEventXML("55", "4608", logonHostA, "2001-02-03T05:00:00.000Z") +
		securityEventXML("56", "4624", logonHostA, "2001-02-03T05:10:00.000Z", "TargetLogonId", "0xa2", "LogonType", "2") +
		// 0xa3: 操作だけ。最後の操作から T の後に終わる。
		securityEventXML("57", "4672", logonHostA, "2001-02-03T05:20:00.000Z", "SubjectLogonId", "0xa3") +
		securityEventXML("58", "4688", logonHostA, "2001-02-03T05:30:00.000Z", "SubjectLogonId", "0xa3") +
		// 0xa4: ログオフだけ。0xa5: 時点を持たない操作だけ。0x3e7: 固定の値。
		securityEventXML("59", "4634", logonHostA, "2001-02-03T05:40:00.000Z", "TargetLogonId", "0xa4") +
		securityEventXML("60", "4672", logonHostA, "2001-02-03 05:41:00.000", "SubjectLogonId", "0xa5") +
		securityEventXML("61", "4672", logonHostA, "2001-02-03T05:42:00.000Z", "SubjectLogonId", "0x3e7") +
		"</Events>\n"
	result := windowsEventSessionResult(t, document)
	graph := NewGraph(result, AllMatchConditions())
	want := []periodSummary{
		{"51", "2001-02-03T04:00:00Z", "2001-02-03T04:02:00Z", sessionStartLogon, sessionEndLogoff},
		{"54", "2001-02-03T04:10:00Z", "2001-02-03T05:00:00Z", sessionStartLogon, sessionEndSystemStart},
		{"56", "2001-02-03T05:10:00Z", "2001-02-03T07:10:00Z", sessionStartLogon, sessionEndTimeLimit},
		{"57", "2001-02-03T05:20:00Z", "2001-02-03T07:30:00Z", sessionStartFirstOperation, sessionEndTimeLimit},
	}
	if got := periodsByRecord(t, graph, result, limit); !slices.Equal(got, want) {
		t.Errorf("the periods are\n%v\nwant\n%v", got, want)
	}
}

// 起動の記録は、プロバイダとイベント ID の組で決まる。6005 と Kernel-General の 12 も区切りになる。
func TestLogonSessionPeriodsEndAtEveryKindOfSystemStart(t *testing.T) {
	for _, boot := range []string{
		providerEventXML("EventLog", "72", "6005", logonHostA, "2001-02-03T05:00:00.000Z"),
		providerEventXML("Microsoft-Windows-Kernel-General", "72", "12", logonHostA, "2001-02-03T05:00:00.000Z"),
	} {
		document := "<Events>\n" +
			securityEventXML("71", "4624", logonHostA, "2001-02-03T04:10:00.000Z", "TargetLogonId", "0xa2", "LogonType", "2") +
			boot + "</Events>\n"
		result := windowsEventSessionResult(t, document)
		want := []periodSummary{
			{"71", "2001-02-03T04:10:00Z", "2001-02-03T05:00:00Z", sessionStartLogon, sessionEndSystemStart},
		}
		if got := periodsByRecord(t, NewGraph(result, AllMatchConditions()), result, time.Hour); !slices.Equal(got, want) {
			t.Errorf("the periods are %v, want %v", got, want)
		}
	}
}

// markii 形式のログアウトは、書いた始まりの時刻から記録の時刻までのセッションである。始まりの
// 時刻はヘッダーの時刻と同じ UTC からのずれで読む。始まりが記録の時刻より後のログアウトは
// セッションにならない。
func TestLogonSessionPeriodsReadTheStartWrittenByAMarkIILogout(t *testing.T) {
	logout := func(sequence, header, start string) string {
		return header + " +0900 loc=ja-JP type=ITM2 sn=" + sequence + " lv=5 evt=session subEvt=logout os=Win " +
			`com="HOST01" domain="AD" tmid=00000000-0000-4000-8000-000000000001 csid=S-1-5-21-1-2-3 ` +
			`usr="user01" sessionID=2 sTime="` + start + `"` + "\n"
	}
	result := sessionSourcesResult(t, sessionSource{
		name: "client.log", format: MarkIIFormatKey,
		document: logout("501", "02/01/2000 12:00:00.000", "02/01/2000 09:00:00.000") +
			logout("502", "02/01/2000 13:00:00.000", "02/01/2000 14:00:00.000"),
	})
	graph := NewGraph(result, AllMatchConditions())
	want := []periodSummary{
		{"501", "2000-02-01T00:00:00Z", "2000-02-01T03:00:00Z", sessionStartLogoffRecord, sessionEndLogoff},
	}
	if got := periodsByRecord(t, graph, result, time.Hour); !slices.Equal(got, want) {
		t.Errorf("the periods are %v, want %v", got, want)
	}
}
