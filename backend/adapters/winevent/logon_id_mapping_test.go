package winevent_test

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/winevent"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// ログオンの成功は、作ったセッションの TargetLogonId と、ログオンを要求したセッションの
// SubjectLogonId を別の語彙の項目で持つ。比べる値は 0 を詰めない小文字の 16 進である。
func TestObserveMapsTheTwoLogonIdsOfALogon(t *testing.T) {
	observation := observe(t, logonEvent("4624",
		"SubjectLogonId", "0x00000000000003A1", "TargetLogonId", "0x00000000000B2C3D",
		"TargetLinkedLogonId", "0x00000000000B2C4E", "LogonGuid", "{0A1B2C3D-0000-1111-2222-333344445555}"))
	declared := winevent.ItemSemantics()
	for name, want := range map[string]struct {
		semantic   core.SemanticKey
		comparable string
		raw        string
	}{
		"EventData.SubjectLogonId": {core.SemanticKeyEventSubjectLogonId, "0x3a1", "0x00000000000003A1"},
		"EventData.TargetLogonId":  {core.SemanticKeyEventTargetLogonId, "0xb2c3d", "0x00000000000B2C3D"},
		"EventData.TargetLinkedLogonId": {
			core.SemanticKeyEventTargetLinkedLogonId, "0xb2c4e", "0x00000000000B2C4E",
		},
		// LogonGuid は原資料の文字列のまま比べる。比べる形への直しは関係を組む側が行う。
		"EventData.LogonGuid": {
			core.SemanticKeyEventTargetLogonGuid,
			"{0A1B2C3D-0000-1111-2222-333344445555}", "{0A1B2C3D-0000-1111-2222-333344445555}",
		},
	} {
		field := fieldNamed(t, observation.Fields, name)
		if field.Semantic != want.semantic {
			t.Errorf("%s semantic = %q, want %q", name, field.Semantic, want.semantic)
		}
		if got, ok := field.Text.ComparableValue(); !ok || got != want.comparable {
			t.Errorf("%s comparable = %q (%t), want %q", name, got, ok, want.comparable)
		}
		if raw, _ := field.Text.RawTextValue(); raw != want.raw {
			t.Errorf("%s raw = %q, want %q", name, raw, want.raw)
		}
		if !slices.Contains(declared, want.semantic) {
			t.Errorf("ItemSemantics does not declare %q", want.semantic)
		}
	}
}

// チケットの要求は LogonGuid を、明示的な資格情報を使ったログオンは作ったログオンの
// TargetLogonGuid を持つ。4648 の LogonGuid は要求した側のセッションの値であり、写さない。
func TestObserveMapsTheLogonGuidsOfTicketsAndExplicitLogons(t *testing.T) {
	const guid = "{0A1B2C3D-0000-1111-2222-333344445555}"
	ticket := observe(t, logonEvent("4769", "LogonGuid", guid))
	if got := fieldNamed(t, ticket.Fields, "EventData.LogonGuid").Semantic; got != core.SemanticKeyEventTicketLogonGuid {
		t.Errorf("4769 LogonGuid semantic = %q", got)
	}
	explicit := observe(t, logonEvent("4648", "LogonGuid", guid, "TargetLogonGuid", guid))
	if got := fieldNamed(t, explicit.Fields, "EventData.TargetLogonGuid").Semantic; got != core.SemanticKeyEventTargetLogonGuid {
		t.Errorf("4648 TargetLogonGuid semantic = %q", got)
	}
	if got := fieldNamed(t, explicit.Fields, "EventData.LogonGuid").Semantic; got != "" {
		t.Errorf("4648 LogonGuid semantic = %q, want none", got)
	}
}

// 後続の操作のイベントは、操作を行ったセッションとして SubjectLogonId を持つ。プロセスの作成の
// 既存の対応も変わらない。
func TestObserveMapsTheSubjectLogonIdOfOperations(t *testing.T) {
	for _, eventID := range []string{"4672", "4648", "4720", "4688", "5140", "5379", "4703"} {
		t.Run(eventID, func(t *testing.T) {
			observation := observe(t, logonEvent(eventID,
				"SubjectLogonId", "0xB2C3D", "NewProcessId", "0x2b"))
			field := fieldNamed(t, observation.Fields, "EventData.SubjectLogonId")
			if field.Semantic != core.SemanticKeyEventSubjectLogonId {
				t.Errorf("semantic = %q, want %q", field.Semantic, core.SemanticKeyEventSubjectLogonId)
			}
			if got, ok := field.Text.ComparableValue(); !ok || got != "0xb2c3d" {
				t.Errorf("comparable = %q (%t), want 0xb2c3d", got, ok)
			}
			wantPid := core.SemanticKey("")
			if eventID == "4688" {
				wantPid = core.SemanticKeyProcessPid
			}
			if pid := fieldNamed(t, observation.Fields, "EventData.NewProcessId"); pid.Semantic != wantPid {
				t.Errorf("NewProcessId semantic = %q, want %q", pid.Semantic, wantPid)
			}
		})
	}
}

// ログオフ (4634 と 4647) の TargetLogonId は終えたセッションを指し、ログオンが作った
// セッションの項目と別の項目で持つ。
func TestObserveMapsTheLogonIdOfALogoff(t *testing.T) {
	for _, eventID := range []string{"4634", "4647"} {
		observation := observe(t, logonEvent(eventID, "TargetLogonId", "0x00000000000B2C3D"))
		field := fieldNamed(t, observation.Fields, "EventData.TargetLogonId")
		if field.Semantic != core.SemanticKeyEventLogoffLogonId {
			t.Errorf("%s TargetLogonId semantic = %q, want %q", eventID, field.Semantic, core.SemanticKeyEventLogoffLogonId)
		}
		if got, ok := field.Text.ComparableValue(); !ok || got != "0xb2c3d" {
			t.Errorf("%s comparable = %q (%t), want 0xb2c3d", eventID, got, ok)
		}
	}
	if !slices.Contains(winevent.ItemSemantics(), core.SemanticKeyEventLogoffLogonId) {
		t.Errorf("ItemSemantics does not declare %q", core.SemanticKeyEventLogoffLogonId)
	}
}

// 端末の起動を記録するイベントだけが SystemStart を持つ。同じ ID の別のプロバイダのイベントは持たない。
func TestObserveMarksTheSystemStartEvents(t *testing.T) {
	for _, tc := range []struct {
		provider, eventID string
		want              bool
	}{
		{"Microsoft-Windows-Security-Auditing", "4608", true},
		{"EventLog", "6005", true},
		{"Microsoft-Windows-Kernel-General", "12", true},
		{"Microsoft-Windows-Kernel-General", "13", false},
		{"EventLog", "4608", false},
	} {
		observation := observe(t, eventWith(tc.provider, tc.eventID, ""))
		if observation.SystemStart != tc.want {
			t.Errorf("%s %s SystemStart = %t, want %t", tc.provider, tc.eventID, observation.SystemStart, tc.want)
		}
	}
}

// "-" は値の不在であり、形に合わない文字列は範囲の外の値である。どちらも比べる値にならない。
func TestObserveKeepsUnreadableLogonIdsUncompared(t *testing.T) {
	for text, want := range map[string]core.ValueState{
		"-":        core.ValueStateAbsent,
		"not-a-id": core.ValueStateOutOfDefinition,
	} {
		observation := observe(t, logonEvent("4672", "SubjectLogonId", text))
		field := fieldNamed(t, observation.Fields, "EventData.SubjectLogonId")
		if field.Text.ValueState != want {
			t.Errorf("%q valueState = %q, want %q", text, field.Text.ValueState, want)
		}
		if _, ok := field.Text.ComparableValue(); ok {
			t.Errorf("%q has a comparable value", text)
		}
	}
}
