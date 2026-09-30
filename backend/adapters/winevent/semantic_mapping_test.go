package winevent_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/winevent"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// プロセスの作成のイベント。プロセス番号、パス、アカウントである。
const processCreationEvent = `<Event><System>` +
	`<Provider Name="Microsoft-Windows-Security-Auditing"/>` +
	`<EventID>4688</EventID>` +
	`<TimeCreated SystemTime="2001-02-03T04:05:06.1234567Z"/>` +
	`<EventRecordID>201</EventRecordID><Channel>Security</Channel>` +
	`<Computer>host02.example.test</Computer></System>` +
	`<EventData><Data Name="SubjectUserSid">S-1-5-21-1000-2000-3000-1002</Data>` +
	`<Data Name="SubjectUserName">analyst02</Data>` +
	`<Data Name="SubjectDomainName">EXAMPLE</Data>` +
	`<Data Name="TargetUserSid">S-1-0-0</Data>` +
	`<Data Name="TargetUserName">-</Data>` +
	`<Data Name="NewProcessId">0x2b</Data>` +
	`<Data Name="NewProcessName">C:\Example\child.exe</Data>` +
	`<Data Name="ProcessId">0x1f</Data>` +
	`<Data Name="CommandLine">child.exe /q</Data>` +
	`<Data Name="ParentProcessName">C:\Example\parent.exe</Data>` +
	`<Data Name="ProcessId">0x1e</Data>` +
	`</EventData></Event>`

func observe(t *testing.T, document string) winevent.Observation {
	t.Helper()
	events, failures := readAll(t, document)
	if len(failures) != 0 || len(events) != 1 {
		t.Fatalf("events = %d, failures = %+v; want 1 event", len(events), failures)
	}
	observation, failure := winevent.Observe(events[0])
	if failure != nil {
		t.Fatalf("Observe() failure = %+v", failure)
	}
	return observation
}

func fieldNamed(t *testing.T, fields []core.RecordField, name string) core.RecordField {
	t.Helper()
	for _, field := range fields {
		if field.Name == name {
			return field
		}
	}
	t.Fatalf("no field named %q in %+v", name, fields)
	return core.RecordField{}
}

func TestObserveMapsProcessCreation(t *testing.T) {
	observation := observe(t, processCreationEvent)
	for _, item := range []struct {
		name       string
		semantic   core.SemanticKey
		raw        string
		normalized string
	}{
		{"EventData.NewProcessId", core.SemanticKeyProcessPid, "0x2b", "43"},
		{"EventData.NewProcessName", core.SemanticKeyProcessBinaryPath, `C:\Example\child.exe`, ""},
		{"EventData.CommandLine", core.SemanticKeyProcessCommandLine, "child.exe /q", ""},
		{"EventData.ProcessId", core.SemanticKeyParentProcessPid, "0x1f", "31"},
		{"EventData.ParentProcessName", core.SemanticKeyParentProcessBinaryPath, `C:\Example\parent.exe`, ""},
		{"EventData.SubjectUserSid", core.SemanticKeySubjectAccountSid, "S-1-5-21-1000-2000-3000-1002", ""},
		{"EventData.SubjectUserName", core.SemanticKeySubjectAccountName, "analyst02", ""},
		{"EventData.SubjectDomainName", core.SemanticKeySubjectAccountDomain, "EXAMPLE", ""},
		{"EventData.TargetUserSid", core.SemanticKeyTargetAccountSid, "S-1-0-0", ""},
		// 同じ名前の 2 回目は出現の番号を持つ。
		{"EventData.ProcessId#2", core.SemanticKeyParentProcessPid, "0x1e", "30"},
		{"Computer", core.SemanticKeyTerminalHostname, "host02.example.test", ""},
		{"Channel", core.SemanticKeyWindowsEventChannel, "Security", ""},
		{"EventID", core.SemanticKeyWindowsEventId, "4688", ""},
		{"EventRecordID", core.SemanticKeyWindowsEventRecordId, "201", ""},
		{"Provider@Name", core.SemanticKeyWindowsEventProvider, "Microsoft-Windows-Security-Auditing", ""},
	} {
		field := fieldNamed(t, observation.Fields, item.name)
		if field.Semantic != item.semantic {
			t.Errorf("%s semantic = %q, want %q", item.name, field.Semantic, item.semantic)
		}
		if raw, _ := field.Text.RawTextValue(); raw != item.raw {
			t.Errorf("%s raw = %q, want %q", item.name, raw, item.raw)
		}
		if normalized, _ := field.Text.NormalizedValue(); normalized != item.normalized {
			t.Errorf("%s normalized = %q, want %q", item.name, normalized, item.normalized)
		}
	}
	if len(observation.Terminal) != 1 || observation.Terminal[0].Semantic != core.SemanticKeyTerminalHostname {
		t.Errorf("Terminal = %+v, want the Computer as terminal.hostname", observation.Terminal)
	}
	wantKind := []string{"Microsoft-Windows-Security-Auditing", "4688"}
	var gotKind []string
	for _, field := range observation.ObservationKind.Raw {
		raw, _ := field.Text.RawTextValue()
		gotKind = append(gotKind, raw)
	}
	if !slices.Equal(gotKind, wantKind) || observation.ObservationKind.Status != core.ObservationKindStatusDetermined {
		t.Errorf("ObservationKind = %v %q, want %v determined", gotKind, observation.ObservationKind.Status, wantKind)
	}
	if err := observation.ObservationKind.Validate(); err != nil {
		t.Errorf("ObservationKind.Validate() = %v", err)
	}
	if !observation.ProcessStart {
		t.Error("ProcessStart is false for a process creation")
	}
}

// プロセスの作成と終了のほかのイベントは、同じ名前の Data に語彙の項目を与えず、起動でも
// 終了でもない。
func TestObserveLeavesOtherEventsFormatSpecific(t *testing.T) {
	document := strings.Replace(processCreationEvent, "<EventID>4688</EventID>", "<EventID>4690</EventID>", 1)
	observation := observe(t, document)
	if observation.ProcessStart || observation.ProcessEnd {
		t.Error("ProcessStart or ProcessEnd is true for another event")
	}
	for _, field := range observation.Fields {
		if strings.HasPrefix(field.Name, "EventData.") && field.Semantic != "" {
			t.Errorf("%s semantic = %q, want none", field.Name, field.Semantic)
		}
	}
}

// プロセスの終了のイベントは、ProcessId を終了したプロセスの番号にし、終了を記録する。
func TestObserveReadsTheProcessTermination(t *testing.T) {
	observation := observe(t, `<Event><System>`+
		`<Provider Name="Microsoft-Windows-Security-Auditing"/><EventID>4689</EventID>`+
		`<TimeCreated SystemTime="2001-02-03T04:05:06.1234567Z"/>`+
		`<EventRecordID>202</EventRecordID><Channel>Security</Channel>`+
		`<Computer>host02.example.test</Computer></System>`+
		`<EventData><Data Name="ProcessId">0x2b</Data>`+
		`<Data Name="ProcessName">C:\Example\child.exe</Data></EventData></Event>`)
	if !observation.ProcessEnd || observation.ProcessStart {
		t.Errorf("ProcessEnd = %v, ProcessStart = %v; want the termination only",
			observation.ProcessEnd, observation.ProcessStart)
	}
	semantics := make(map[string]core.SemanticKey)
	for _, field := range observation.Fields {
		semantics[field.Name] = field.Semantic
	}
	if semantics["EventData.ProcessId"] != core.SemanticKeyProcessPid ||
		semantics["EventData.ProcessName"] != core.SemanticKeyProcessBinaryPath {
		t.Errorf("semantics = %v, want the pid and the binary path of the terminated process", semantics)
	}
}

// イベント ID とレコード番号は、先頭の 0 を外した 10 進の文字列で比べる。原資料の文字列は保つ。
func TestObserveComparesTheEventIdAndTheRecordIdWithoutLeadingZeros(t *testing.T) {
	observation := observe(t, `<Event><System>`+
		`<Provider Name="Example-Provider"/><EventID>0042</EventID>`+
		`<EventRecordID>0</EventRecordID><Channel>Example/Operational</Channel></System></Event>`)
	for _, item := range []struct {
		name       string
		raw        string
		comparable string
		normalized bool
	}{
		{"EventID", "0042", "42", true},
		// 0 だけの文字列は、そのまま比べる形である。
		{"EventRecordID", "0", "0", false},
		{"Channel", "Example/Operational", "Example/Operational", false},
		{"Provider@Name", "Example-Provider", "Example-Provider", false},
	} {
		field := fieldNamed(t, observation.Fields, item.name)
		raw, _ := field.Text.RawTextValue()
		comparable, _ := field.Text.ComparableValue()
		_, normalized := field.Text.NormalizedValue()
		if raw != item.raw || comparable != item.comparable || normalized != item.normalized {
			t.Errorf("%s raw=%q comparable=%q normalized=%t, want %q %q %t",
				item.name, raw, comparable, normalized, item.raw, item.comparable, item.normalized)
		}
	}
	// 0 だけを 2 つ以上並べた文字列は 0 と比べ、数字でない文字列は原資料の文字列のまま比べる。
	zeros := observe(t, `<Event><System><EventID>00</EventID><EventRecordID>0x1</EventRecordID></System></Event>`)
	for name, want := range map[string]string{"EventID": "0", "EventRecordID": "0x1"} {
		if got, _ := fieldNamed(t, zeros.Fields, name).Text.ComparableValue(); got != want {
			t.Errorf("%s comparable = %q, want %q", name, got, want)
		}
	}
}

func TestObserveCarriesUserDataAndMessage(t *testing.T) {
	observation := observe(t, `<Event><System><EventID>9</EventID></System>`+
		`<UserData><LogCleared><SubjectUserName>analyst01</SubjectUserName></LogCleared></UserData>`+
		`<RenderingInfo Culture="ja-JP"><Message>ログを消去した</Message></RenderingInfo></Event>`)
	userName := fieldNamed(t, observation.Fields, "UserData.LogCleared.SubjectUserName")
	if raw, _ := userName.Text.RawTextValue(); raw != "analyst01" {
		t.Errorf("UserData value = %q, want analyst01", raw)
	}
	message := fieldNamed(t, observation.Fields, "RenderingInfo.Message")
	if raw, _ := message.Text.RawTextValue(); message.Semantic != core.SemanticKeyEventMessage || raw != "ログを消去した" {
		t.Errorf("Message = %q %q, want event.message with the source bytes", message.Semantic, raw)
	}
	fieldNamed(t, observation.Fields, "RenderingInfo@Culture")
	if observation.EventTime != nil || len(observation.Terminal) != 0 {
		t.Errorf("an event without TimeCreated and Computer carries %+v %+v", observation.EventTime, observation.Terminal)
	}
}

func TestObserveKeepsTheOffsetOfSystemTimeAsWritten(t *testing.T) {
	for _, item := range []struct {
		raw         string
		normalized  string
		precision   core.Precision
		offsetState core.OffsetState
		offsetText  string
		hasInstant  bool
	}{
		{"2001-02-03T04:05:06.1234567Z", "2001-02-03T04:05:06.123456Z", core.PrecisionMicrosecond,
			core.OffsetStateInValue, "Z", true},
		{"2001-02-03T04:05:06.123+09:00", "2001-02-03T04:05:06.123+09:00", core.PrecisionMillisecond,
			core.OffsetStateInValue, "+09:00", true},
		{"2001-02-03T04:05:06Z", "2001-02-03T04:05:06Z", core.PrecisionSecond, core.OffsetStateInValue, "Z", true},
		// ずれを持たない文字列に Z を補わない。関連付けに使う時点を持たない。
		{"2001-02-03 04:05:07.654321", "2001-02-03T04:05:07.654321", core.PrecisionMicrosecond,
			core.OffsetStateUndetermined, "", false},
	} {
		observation := observe(t, `<Event><System><TimeCreated SystemTime="`+item.raw+`"/></System></Event>`)
		timestamp := observation.EventTime
		if timestamp == nil {
			t.Fatalf("%s: EventTime is absent", item.raw)
		}
		raw, _ := timestamp.RawTextValue()
		normalized, _ := timestamp.NormalizedValue()
		offsetText, _ := timestamp.OffsetTextValue()
		_, hasInstant := timestamp.Instant()
		if raw != item.raw || normalized != item.normalized || timestamp.Precision != item.precision ||
			timestamp.OffsetState != item.offsetState || offsetText != item.offsetText || hasInstant != item.hasInstant {
			t.Errorf("%s: got raw=%q normalized=%q %s %s offset=%q instant=%v", item.raw, raw, normalized,
				timestamp.Precision, timestamp.OffsetState, offsetText, hasInstant)
		}
		field := fieldNamed(t, observation.Fields, "TimeCreated@SystemTime")
		if field.Kind != core.RecordFieldKindTimestamp || field.Semantic != core.SemanticKeyEventTime {
			t.Errorf("%s: field = %+v, want an event.time timestamp", item.raw, field)
		}
	}
}

func TestObserveReportsAnUnreadableSystemTime(t *testing.T) {
	events, _ := readAll(t, `<Event><System><TimeCreated SystemTime="not a time"/></System></Event>`)
	if _, failure := winevent.Observe(events[0]); failure == nil || failure.Stage != core.FailureStageNormalize {
		t.Errorf("Observe() failure = %+v, want a normalize failure", failure)
	}
}

// 項目が持つ語彙の項目は、宣言した語彙の項目に含まれる。転記の同一性を決める欄の名前は
// 項目の名前にある。
// MsiInstaller のイベントは Name の無い `<Data>` に製品を書き、出現の順で語彙の項目を付ける。
func TestObserveMapsTheProductOfAnInstallerEvent(t *testing.T) {
	installer := func(eventID string, data ...string) string {
		var body strings.Builder
		for _, value := range data {
			body.WriteString("<Data>" + value + "</Data>")
		}
		return `<Event><System><Provider Name="MsiInstaller"/><EventID>` + eventID + `</EventID>` +
			`<TimeCreated SystemTime="2001-02-03T04:05:06Z"/><EventRecordID>1</EventRecordID>` +
			`<Channel>Application</Channel><Computer>host-a</Computer></System><EventData>` +
			body.String() + `</EventData></Event>`
	}
	declared := winevent.ItemSemantics()
	for _, tc := range []struct {
		eventID string
		data    []string
		want    map[string]core.SemanticKey
	}{
		{"1033", []string{"Example Tool", "1.2.3", "1033", "0", "Example Vendor"}, map[string]core.SemanticKey{
			"EventData.Data": core.SemanticKeyFileProductName, "EventData.Data#2": core.SemanticKeyFileProductVersion,
			"EventData.Data#3": "",
		}},
		{"1040", []string{`C:\Example\tool.msi`, "100"}, map[string]core.SemanticKey{
			"EventData.Data": core.SemanticKeyFilePath, "EventData.Data#2": "",
		}},
		{"11707", []string{"Product: Example Tool -- Installation completed successfully.", "(NULL)"},
			map[string]core.SemanticKey{"EventData.Data": core.SemanticKeyEventMessage, "EventData.Data#2": ""}},
	} {
		fields := observe(t, installer(tc.eventID, tc.data...)).Fields
		for name, semantic := range tc.want {
			if got := fieldNamed(t, fields, name).Semantic; got != semantic {
				t.Errorf("%s %s semantic = %q, want %q", tc.eventID, name, got, semantic)
			}
			if semantic != "" && !slices.Contains(declared, semantic) {
				t.Errorf("ItemSemantics does not declare %q", semantic)
			}
		}
	}
}

func TestItemSemanticsCoverTheObservedFields(t *testing.T) {
	declared := winevent.ItemSemantics()
	observation := observe(t, processCreationEvent)
	for _, field := range observation.Fields {
		if field.Semantic != "" && !slices.Contains(declared, field.Semantic) {
			t.Errorf("%s carries %q, which ItemSemantics does not declare", field.Name, field.Semantic)
		}
	}
	for _, semantic := range declared {
		if !semantic.IsKnown() {
			t.Errorf("ItemSemantics declares the unknown %q", semantic)
		}
	}
	for _, name := range winevent.TranscriptIdentityItems() {
		fieldNamed(t, observation.Fields, name)
	}
}

// logonEvent は Security の監査のイベントを、イベント ID と `<Data>` の名前と値の組を
// 差し替えて組む。アカウント、アドレス、ホスト名である。
func logonEvent(eventID string, data ...string) string {
	document := `<Event><System>` +
		`<Provider Name="Microsoft-Windows-Security-Auditing"/>` +
		`<EventID>` + eventID + `</EventID>` +
		`<TimeCreated SystemTime="2001-02-03T04:05:06.1234567Z"/>` +
		`<EventRecordID>301</EventRecordID><Channel>Security</Channel>` +
		`<Computer>host03.example.test</Computer></System><EventData>`
	for index := 0; index+1 < len(data); index += 2 {
		document += `<Data Name="` + data[index] + `">` + data[index+1] + `</Data>`
	}
	return document + `</EventData></Event>`
}

// ログオンの成功と失敗とログオフは、種別のコードと接続元とアカウントを語彙の項目で持つ。
// 失敗は失敗の状態の 2 つのコードも持つ。
func TestObserveMapsLogonEvents(t *testing.T) {
	type mapped struct {
		name     string
		semantic core.SemanticKey
		raw      string
	}
	logon := []mapped{
		{"EventData.TargetUserSid", core.SemanticKeyTargetAccountSid, "S-1-5-21-1000-2000-3000-1003"},
		{"EventData.TargetUserName", core.SemanticKeyTargetAccountName, "user03"},
		{"EventData.TargetDomainName", core.SemanticKeyTargetAccountDomain, "EXAMPLE"},
		{"EventData.LogonType", core.SemanticKeyEventLogonType, "10"},
		{"EventData.IpAddress", core.SemanticKeyConnectionSourceAddress, "192.0.2.30"},
		{"EventData.IpPort", core.SemanticKeyConnectionSourcePort, "50001"},
		{"EventData.WorkstationName", core.SemanticKeyRemoteSessionClientHostname, "CLIENT03"},
	}
	data := []string{
		"TargetUserSid", "S-1-5-21-1000-2000-3000-1003",
		"TargetUserName", "user03", "TargetDomainName", "EXAMPLE", "LogonType", "10",
		"IpAddress", "192.0.2.30", "IpPort", "50001", "WorkstationName", "CLIENT03",
		"Status", "0xc0000234", "SubStatus", "0xc0000064",
	}
	cases := map[string]struct {
		eventID string
		want    []mapped
		// unmapped は、そのイベントでは語彙の項目を持たない `<Data>` である。
		unmapped []string
	}{
		"ログオンの成功": {eventID: "4624", want: logon,
			unmapped: []string{"EventData.Status", "EventData.SubStatus"}},
		"ログオンの失敗": {eventID: "4625", want: append(slices.Clone(logon),
			mapped{"EventData.Status", core.SemanticKeyEventLogonFailureStatus, "0xc0000234"},
			mapped{"EventData.SubStatus", core.SemanticKeyEventLogonFailureSubStatus, "0xc0000064"})},
		"ログオフ": {eventID: "4634", want: logon[:4], unmapped: []string{
			"EventData.IpAddress", "EventData.IpPort", "EventData.WorkstationName",
			"EventData.Status", "EventData.SubStatus",
		}},
	}
	declared := winevent.ItemSemantics()
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			observation := observe(t, logonEvent(tc.eventID, data...))
			for _, item := range tc.want {
				field := fieldNamed(t, observation.Fields, item.name)
				if field.Semantic != item.semantic {
					t.Errorf("%s semantic = %q, want %q", item.name, field.Semantic, item.semantic)
				}
				if got, ok := field.Text.ComparableValue(); !ok || got != item.raw {
					t.Errorf("%s comparable = %q (%t), want %q", item.name, got, ok, item.raw)
				}
				if !slices.Contains(declared, item.semantic) {
					t.Errorf("ItemSemantics does not declare %q", item.semantic)
				}
			}
			for _, name := range tc.unmapped {
				if field := fieldNamed(t, observation.Fields, name); field.Semantic != "" {
					t.Errorf("%s semantic = %q, want none", name, field.Semantic)
				}
			}
		})
	}
}

// LogonType を持たない認証のイベントに種別のコードと失敗のコードを補わない。4776 の
// `<Data>` は語彙の項目を持たない。
func TestObserveGivesNoLogonTypeToAuthenticationEvents(t *testing.T) {
	for _, eventID := range []string{"4768", "4769", "4776"} {
		t.Run(eventID, func(t *testing.T) {
			observation := observe(t, logonEvent(eventID,
				"TargetUserName", "user03", "Status", "0x0", "IpAddress", "192.0.2.30"))
			for _, field := range observation.Fields {
				if !strings.HasPrefix(field.Name, "EventData.") {
					continue
				}
				switch {
				case field.Semantic == core.SemanticKeyEventLogonType ||
					field.Semantic == core.SemanticKeyEventLogonFailureStatus:
					t.Errorf("%s semantic = %q, want no logon code", field.Name, field.Semantic)
				case eventID == "4776" && field.Semantic != "":
					t.Errorf("%s semantic = %q, want none", field.Name, field.Semantic)
				}
			}
		})
	}
}

// 接続元とアカウントと種別のコードの "-" は値の不在であり、比べる値にならない。原資料の文字列は保つ。
func TestObserveReadsTheDashOfALogonAsAbsent(t *testing.T) {
	observation := observe(t, logonEvent("4624",
		"TargetUserName", "-", "IpAddress", "-", "IpPort", "-", "WorkstationName", "-",
		"LogonType", "-", "TargetUserSid", "-", "SubjectDomainName", "-"))
	for _, name := range []string{
		"EventData.TargetUserName", "EventData.IpAddress", "EventData.IpPort",
		"EventData.WorkstationName", "EventData.LogonType", "EventData.TargetUserSid",
		"EventData.SubjectDomainName",
	} {
		field := fieldNamed(t, observation.Fields, name)
		if field.Text.ValueState != core.ValueStateAbsent {
			t.Errorf("%s valueState = %q, want %q", name, field.Text.ValueState, core.ValueStateAbsent)
		}
		if raw, _ := field.Text.RawTextValue(); raw != "-" {
			t.Errorf("%s raw = %q, want the dash", name, raw)
		}
	}
	// 反対側。値のある種別のコードはそのまま比べる。
	coded := observe(t, logonEvent("4624", "LogonType", "3"))
	logonType := fieldNamed(t, coded.Fields, "EventData.LogonType")
	if got, ok := logonType.Text.ComparableValue(); !ok || got != "3" {
		t.Errorf("LogonType comparable = %q (%t), want 3", got, ok)
	}
}

// 特権の割り当ては、特権を割り当てられたアカウントを Subject に書く。
func TestObserveMapsTheSubjectAccountOfASpecialPrivilegeAssignment(t *testing.T) {
	observation := observe(t, logonEvent("4672",
		"SubjectUserSid", "S-1-5-21-1000-2000-3000-1004", "SubjectUserName", "admin04",
		"SubjectDomainName", "EXAMPLE", "PrivilegeList", "SeExamplePrivilege"))
	for name, want := range map[string]core.SemanticKey{
		"EventData.SubjectUserSid":    core.SemanticKeySubjectAccountSid,
		"EventData.SubjectUserName":   core.SemanticKeySubjectAccountName,
		"EventData.SubjectDomainName": core.SemanticKeySubjectAccountDomain,
		"EventData.PrivilegeList":     "",
	} {
		if field := fieldNamed(t, observation.Fields, name); field.Semantic != want {
			t.Errorf("%s semantic = %q, want %q", name, field.Semantic, want)
		}
	}
}

func TestFormatsDeclareTheXMLCSVAndEVTXFormats(t *testing.T) {
	formats := winevent.Formats()
	for _, key := range []core.FormatKey{"windows_event_xml", "windows_event_viewer_csv", "windows_evtx"} {
		declared := slices.ContainsFunc(formats, func(format core.InputFormat) bool {
			return format.Key == key && format.PositionKind == core.PositionKindByteRange && format.Validate() == nil
		})
		if !declared {
			t.Errorf("Formats() = %+v, want a valid %s read by byte range", formats, key)
		}
	}
}

// FieldNameOf が返す名前は、Observe が項目に付けた名前と同じである。
func TestFieldNameOfNamesTheObservedFields(t *testing.T) {
	observation := observe(t, "<Events>"+processCreationEvent+"</Events>")
	names := make([]string, 0, len(observation.Fields))
	for _, field := range observation.Fields {
		names = append(names, field.Name)
	}
	for _, xmlName := range []string{"EventID", "Channel", "Computer", "Provider_Name", "SubjectUserName"} {
		if !slices.Contains(names, winevent.FieldNameOf(xmlName)) {
			t.Errorf("FieldNameOf(%q) = %q, not among %v", xmlName, winevent.FieldNameOf(xmlName), names)
		}
	}
	if got := winevent.FieldNameOf("Unobserved"); got != "EventData.Unobserved" {
		t.Errorf("FieldNameOf(Unobserved) = %q", got)
	}
}
