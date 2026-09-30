package markii_test

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/markii"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// eventLogRecordWith は evt が os、subEvt が evtLog のレコードを、イベント ID と evtLogonID の
// 値を差し替えて組む。値は原資料の文字列の形 (引用符付きの 0x に続く 16 進) に合わせている。
func eventLogRecordWith(eventID, logonId string) string {
	return "02/01/2000 03:04:08.901 +0900 loc=ja-JP type=ITM2 " +
		"sn=1009 lv=5 evt=os subEvt=evtLog os=Win com=\"HOST01\" domain=\"AD\" " +
		"tmid=" + recordObservationTerminal + " csid=S-1-5-21-1-2-3 " +
		"channel=\"Security\" evtID=" + eventID + " evtRecID=9001 " +
		"evtSrc=\"Microsoft-Windows-Security-Auditing\" evtUsr=\"user01\" " +
		"evtLogonID=" + logonId
}

// ログオンの成功の evtLogonID はログオンが作ったセッション、ログオフの evtLogonID は終えた
// セッション、ほかのイベントの evtLogonID は操作を行ったセッションを指す。比べる値は小文字の
// 16 進であり、原資料の文字列は引用符ごと保つ。
func TestRecordFieldsSplitTheLogonIdSemanticByTheEventId(t *testing.T) {
	declared := markii.ItemSemantics()
	for eventID, want := range map[string]core.SemanticKey{
		"4624": core.SemanticKeyEventTargetLogonId,
		"4634": core.SemanticKeyEventLogoffLogonId,
		"4647": core.SemanticKeyEventLogoffLogonId,
		"4672": core.SemanticKeyEventSubjectLogonId,
		"4662": core.SemanticKeyEventSubjectLogonId,
	} {
		t.Run(eventID, func(t *testing.T) {
			fields := parseRecordObservationOK(t, eventLogRecordWith(eventID, `"0x1A2B3"`)).Fields
			assertSemanticOfField(t, fields, "evtLogonID", want)
			if got := textValueOfField(t, fields, "evtLogonID"); got != "0x1a2b3" {
				t.Errorf("evtLogonID = %q, want 0x1a2b3", got)
			}
			field, _ := recordFieldNamed(fields, "evtLogonID")
			if raw, _ := field.Text.RawTextValue(); raw != `"0x1A2B3"` {
				t.Errorf("the raw text = %q, want the quoted source bytes", raw)
			}
			if !slices.Contains(declared, want) {
				t.Errorf("ItemSemantics does not declare %q", want)
			}
		})
	}
}

// sessionRecordWith は evt が session のレコードを、subEvt を差し替えて組む。
func sessionRecordWith(subEvent string) string {
	return "02/01/2000 03:04:08.901 +0900 loc=ja-JP type=ITM2 " +
		"sn=1010 lv=5 evt=session subEvt=" + subEvent + " os=Win com=\"HOST01\" domain=\"AD\" " +
		"tmid=" + recordObservationTerminal + " csid=S-1-5-21-1-2-3 " +
		"usr=\"user01\" sessionID=2 sTime=\"01/30/2000 10:00:00.000\""
}

// ログアウトの sTime は終えたセッションの始まりの時刻であり、ロックの解除の sTime は語彙の
// 項目を持たない。
func TestRecordFieldsGiveTheSessionStartTimeOnlyToLogouts(t *testing.T) {
	fields := parseRecordObservationOK(t, sessionRecordWith("logout")).Fields
	assertSemanticOfField(t, fields, "sTime", core.SemanticKeyEventSessionStartTime)
	field, _ := recordFieldNamed(fields, "sTime")
	if field.Timestamp == nil || field.Timestamp.Normalized == nil ||
		*field.Timestamp.Normalized != "2000-01-30T10:00:00.000" {
		t.Errorf("the sTime of the logout = %+v, want the local time 2000-01-30T10:00:00.000", field.Timestamp)
	}
	if !slices.Contains(markii.ItemSemantics(), core.SemanticKeyEventSessionStartTime) {
		t.Errorf("ItemSemantics does not declare %q", core.SemanticKeyEventSessionStartTime)
	}
	unlock := parseRecordObservationOK(t, sessionRecordWith("unlock")).Fields
	assertSemanticOfField(t, unlock, "sTime", "")
}

// 引用符付きの - は値の不在であり、形に合わない文字列は範囲の外の値である。どちらも比べる値に
// ならない。
func TestRecordFieldsKeepUnreadableLogonIdsUncompared(t *testing.T) {
	for text, want := range map[string]core.ValueState{
		`"-"`:     core.ValueStateAbsent,
		`"not-a"`: core.ValueStateOutOfDefinition,
	} {
		fields := parseRecordObservationOK(t, eventLogRecordWith("4672", text)).Fields
		field, found := recordFieldNamed(fields, "evtLogonID")
		if !found {
			t.Fatalf("%s: no evtLogonID field", text)
		}
		if field.Text.ValueState != want {
			t.Errorf("%s: valueState = %q, want %q", text, field.Text.ValueState, want)
		}
		if _, ok := field.Text.ComparableValue(); ok {
			t.Errorf("%s: an unreadable Logon ID has a comparable value", text)
		}
	}
}
