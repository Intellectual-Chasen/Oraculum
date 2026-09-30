package winevent_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/winevent"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// sysmonEvent は Sysmon のイベント 1 件を返す。data は Name と値の組を交互に並べる。
func sysmonEvent(eventID string, data ...string) string {
	var elements strings.Builder
	for at := 0; at+1 < len(data); at += 2 {
		elements.WriteString(`<Data Name="` + data[at] + `">` + data[at+1] + `</Data>`)
	}
	return `<Event><System><Provider Name="Microsoft-Windows-Sysmon"/>` +
		`<EventID>` + eventID + `</EventID>` +
		`<TimeCreated SystemTime="2001-02-03T04:05:09.1234567Z"/>` +
		`<EventRecordID>71</EventRecordID><Channel>Microsoft-Windows-Sysmon/Operational</Channel>` +
		`<Computer>host07.example.test</Computer></System>` +
		`<EventData>` + elements.String() + `</EventData></Event>`
}

const (
	syntheticChildGuid  = "{0A1B2C3D-0001-4000-8000-00000000C001}"
	syntheticParentGuid = "{0A1B2C3D-0001-4000-8000-00000000A001}"
)

var sysmonProcessCreateEvent = sysmonEvent("1",
	"UtcTime", "2001-02-03 04:05:06.789",
	"ProcessGuid", syntheticChildGuid,
	"ProcessId", "41",
	"Image", `C:\Example\child.exe`,
	"CommandLine", "child.exe /q",
	"Company", "Example Company",
	"TerminalSessionId", "2",
	"User", `EXAMPLE\analyst07`,
	"ParentProcessGuid", syntheticParentGuid,
	"ParentProcessId", "31",
	"ParentImage", `C:\Example\parent.exe`,
)

func TestObserveMapsSysmonProcessCreate(t *testing.T) {
	observation := observe(t, sysmonProcessCreateEvent)
	for _, item := range []struct {
		name     string
		semantic core.SemanticKey
		raw      string
	}{
		{"EventData.ProcessGuid", core.SemanticKeyProcessId, syntheticChildGuid},
		{"EventData.ProcessId", core.SemanticKeyProcessPid, "41"},
		{"EventData.Image", core.SemanticKeyProcessBinaryPath, `C:\Example\child.exe`},
		{"EventData.CommandLine", core.SemanticKeyProcessCommandLine, "child.exe /q"},
		{"EventData.Company", core.SemanticKeyProcessBinaryCompany, "Example Company"},
		{"EventData.TerminalSessionId", core.SemanticKeyProcessSessionId, "2"},
		{"EventData.ParentProcessGuid", core.SemanticKeyParentProcessId, syntheticParentGuid},
		{"EventData.ParentProcessId", core.SemanticKeyParentProcessPid, "31"},
		{"EventData.ParentImage", core.SemanticKeyParentProcessBinaryPath, `C:\Example\parent.exe`},
		// User は `ドメイン\名前` を 1 つの文字列に書くため、語彙の項目を持たない。
		{"EventData.User", "", `EXAMPLE\analyst07`},
	} {
		field := fieldNamed(t, observation.Fields, item.name)
		raw, _ := field.Text.RawTextValue()
		comparable, readable := field.Text.ComparableValue()
		// GUID の中括弧と大文字小文字をそろえない。比べる値は原資料の文字列そのものである。
		if field.Semantic != item.semantic || raw != item.raw || !readable || comparable != item.raw {
			t.Errorf("%s = %q raw=%q comparable=%q, want %q %q", item.name, field.Semantic, raw, comparable,
				item.semantic, item.raw)
		}
	}
	if !observation.ProcessStart {
		t.Error("ProcessStart is false for a Sysmon process creation")
	}
}

// UtcTime と TimeCreated@SystemTime を別の時刻として保ち、2 つの差を補わない。UtcTime は
// ずれを書かずに UTC を表す。
func TestObserveKeepsSysmonUtcTimeApartFromTheRecordTime(t *testing.T) {
	observation := observe(t, sysmonProcessCreateEvent)
	utc := fieldNamed(t, observation.Fields, "EventData.UtcTime")
	if utc.Kind != core.RecordFieldKindTimestamp || utc.Semantic != core.SemanticKeyProcessStartTime || utc.Timestamp == nil {
		t.Fatalf("UtcTime = %+v, want a process.start_time timestamp", utc)
	}
	timestamp := *utc.Timestamp
	raw, _ := timestamp.RawTextValue()
	normalized, _ := timestamp.NormalizedValue()
	instant, hasInstant := timestamp.Instant()
	if raw != "2001-02-03 04:05:06.789" || normalized != "2001-02-03T04:05:06.789Z" ||
		timestamp.Precision != core.PrecisionMillisecond || timestamp.OffsetState != core.OffsetStateFormatDefined ||
		timestamp.Clock != core.ClockTerminalLocal || timestamp.Meaning != core.MeaningOperationStart || !hasInstant {
		t.Errorf("UtcTime = raw %q normalized %q %s %s %s %s instant=%v", raw, normalized, timestamp.Precision,
			timestamp.OffsetState, timestamp.Clock, timestamp.Meaning, hasInstant)
	}
	if offsetText, written := timestamp.OffsetTextValue(); written {
		t.Errorf("UtcTime carries the offset text %q the source does not write", offsetText)
	}
	recorded, _ := observation.EventTime.Instant()
	if recordedText, _ := observation.EventTime.NormalizedValue(); recordedText != "2001-02-03T04:05:09.123456Z" ||
		instant.Equal(recorded) {
		t.Errorf("EventTime = %q, want the TimeCreated value kept apart from UtcTime", recordedText)
	}
	system := fieldNamed(t, observation.Fields, "TimeCreated@SystemTime")
	if system.Semantic != core.SemanticKeyEventTime {
		t.Errorf("TimeCreated@SystemTime semantic = %q, want event.time", system.Semantic)
	}
}

// 書式に合わない UtcTime とずれを書く UtcTime は、語彙の項目を持たずに原資料の文字列を保つ。
func TestObserveLeavesAnUnreadableSysmonUtcTimeWithoutMeaning(t *testing.T) {
	for _, raw := range []string{"not a time", "2001-02-03 04:05:06.789+09:00"} {
		observation := observe(t, sysmonEvent("11", "UtcTime", raw, "TargetFilename", `C:\Example\out.txt`))
		field := fieldNamed(t, observation.Fields, "EventData.UtcTime")
		if text, _ := field.Text.RawTextValue(); field.Semantic != "" || field.Kind != core.RecordFieldKindText || text != raw {
			t.Errorf("%q: UtcTime = %+v, want a text without meaning", raw, field)
		}
	}
}

func TestObserveMapsSysmonOperations(t *testing.T) {
	for _, item := range []struct {
		eventID  string
		name     string
		value    string
		semantic core.SemanticKey
	}{
		{"2", "TargetFilename", `C:\Example\touched.txt`, core.SemanticKeyFilePath},
		{"2", "CreationUtcTime", "2000-01-02 03:04:05.678", core.SemanticKeyFileCreatedTime},
		{"2", "PreviousCreationUtcTime", "2000-01-03 03:04:05.678", ""},
		{"3", "Protocol", "tcp", core.SemanticKeyConnectionProtocol},
		{"3", "SourceIp", "192.0.2.7", core.SemanticKeyConnectionSourceAddress},
		{"3", "SourcePort", "50007", core.SemanticKeyConnectionSourcePort},
		{"3", "DestinationIp", "198.51.100.7", core.SemanticKeyConnectionDestinationAddress},
		{"3", "DestinationPort", "8007", core.SemanticKeyConnectionDestinationPort},
		{"3", "DestinationHostname", "dest07.example.test", core.SemanticKeyConnectionDestinationReverseLookupName},
		{"11", "TargetFilename", `C:\Example\created.txt`, core.SemanticKeyFilePath},
		{"11", "CreationUtcTime", "2001-02-03 04:05:06.700", core.SemanticKeyFileCreatedTime},
		{"12", "EventType", "CreateKey", core.SemanticKeyEventAction},
		{"12", "TargetObject", `HKU\Example\Key07`, core.SemanticKeyRegistryValueKeyPath},
		{"13", "TargetObject", `HKU\Example\Key07\Value07`, core.SemanticKeyRegistryValueKeyPath},
		{"13", "Details", "example data", core.SemanticKeyRegistryValueData},
		{"14", "TargetObject", `HKU\Example\Key07`, core.SemanticKeyRegistryValueKeyPath},
		{"14", "NewName", `HKU\Example\Key08`, ""},
	} {
		observation := observe(t, sysmonEvent(item.eventID,
			"UtcTime", "2001-02-03 04:05:06.700", "ProcessGuid", syntheticChildGuid, "ProcessId", "41",
			"Image", `C:\Example\child.exe`, "Initiated", "true", item.name, item.value))
		for name, semantic := range map[string]core.SemanticKey{
			"EventData." + item.name: item.semantic,
			"EventData.UtcTime":      core.SemanticKeyEventOperationStartTime,
			"EventData.ProcessGuid":  core.SemanticKeyProcessId,
			"EventData.ProcessId":    core.SemanticKeyProcessPid,
			"EventData.Image":        core.SemanticKeyProcessBinaryPath,
		} {
			if field := fieldNamed(t, observation.Fields, name); field.Semantic != semantic {
				t.Errorf("event %s %s semantic = %q, want %q", item.eventID, name, field.Semantic, semantic)
			}
		}
		if observation.ProcessStart {
			t.Errorf("event %s is a process start", item.eventID)
		}
	}
	created := fieldNamed(t, observe(t, sysmonEvent("11", "CreationUtcTime", "2001-02-03 04:05:06.700")).Fields,
		"EventData.CreationUtcTime")
	if created.Timestamp == nil || created.Timestamp.Clock != core.ClockFileProperty ||
		created.Timestamp.Meaning != core.MeaningProperty {
		t.Errorf("CreationUtcTime = %+v, want a file property time", created)
	}
}

// 自分の端末が始めたと書かない通信 (Initiated が true でない) は、接続元と接続先の項目に
// 語彙の項目を与えず、原資料の文字列を保つ。着信では Destination が自分の端末である。
func TestObserveLeavesSysmonEndpointsOfAnInboundConnectionWithoutMeaning(t *testing.T) {
	endpoints := []string{"SourceIp", "SourcePort", "DestinationIp", "DestinationPort", "DestinationHostname"}
	for _, initiated := range []string{"false", "", "maybe", "True", "item absent"} {
		data := []string{"ProcessGuid", syntheticChildGuid, "Protocol", "tcp",
			"SourceIp", "192.0.2.8", "SourcePort", "50008", "DestinationIp", "198.51.100.8",
			"DestinationPort", "8008", "DestinationHostname", "host07.example.test"}
		if initiated != "item absent" {
			data = append(data, "Initiated", initiated)
		}
		observation := observe(t, sysmonEvent("3", data...))
		for _, name := range endpoints {
			field := fieldNamed(t, observation.Fields, "EventData."+name)
			if raw, _ := field.Text.RawTextValue(); field.Semantic != "" || raw == "" {
				t.Errorf("Initiated %q: %s = %q %q, want the source text without meaning", initiated, name,
					field.Semantic, raw)
			}
		}
		// 通信の端点のほかの項目は、着信でも意味を持つ。
		for name, semantic := range map[string]core.SemanticKey{
			"EventData.ProcessGuid": core.SemanticKeyProcessId, "EventData.Protocol": core.SemanticKeyConnectionProtocol,
		} {
			if field := fieldNamed(t, observation.Fields, name); field.Semantic != semantic {
				t.Errorf("Initiated %q: %s semantic = %q, want %q", initiated, name, field.Semantic, semantic)
			}
		}
	}
	// Initiated を他のイベントで読まない。
	file := observe(t, sysmonEvent("11", "Initiated", "false", "TargetFilename", `C:\Example\out.txt`))
	if field := fieldNamed(t, file.Fields, "EventData.TargetFilename"); field.Semantic != core.SemanticKeyFilePath {
		t.Errorf("TargetFilename semantic = %q, want file.path", field.Semantic)
	}
}

// 値を持たない文字列は、対象を識別する項目と親の識別子で値の不在になる。値を持つ項目の空の
// 文字列は値のまま保つ。
func TestObserveKeepsSysmonPlaceholdersAsAbsent(t *testing.T) {
	observation := observe(t, sysmonEvent("1",
		"ProcessGuid", syntheticChildGuid,
		"ParentProcessGuid", "{00000000-0000-0000-0000-000000000000}",
		"ParentImage", "-"))
	parent := fieldNamed(t, observation.Fields, "EventData.ParentProcessGuid")
	if _, readable := parent.Text.ComparableValue(); readable || parent.Text.ValueState != core.ValueStateAbsent ||
		parent.Semantic != core.SemanticKeyParentProcessId {
		t.Errorf("the all-zero ParentProcessGuid = %+v, want an absent parent_process.id", parent)
	}
	// 表示名の項目はノードを識別しないため、文字列のまま保つ。
	if image := fieldNamed(t, observation.Fields, "EventData.ParentImage"); image.Text.ValueState != core.ValueStatePresent {
		t.Errorf("ParentImage = %+v, want the present text", image)
	}
	for _, item := range []struct {
		eventID string
		name    string
		value   string
		state   core.ValueState
	}{
		{"1", "ParentProcessGuid", "", core.ValueStateAbsent},
		{"2", "TargetFilename", "-", core.ValueStateAbsent},
		{"11", "TargetFilename", "", core.ValueStateAbsent},
		{"13", "Details", "", core.ValueStatePresent},
	} {
		field := fieldNamed(t, observe(t, sysmonEvent(item.eventID, item.name, item.value)).Fields, "EventData."+item.name)
		if raw, _ := field.Text.RawTextValue(); field.Text.ValueState != item.state || raw != item.value {
			t.Errorf("event %s %s %q = %+v, want %s", item.eventID, item.name, item.value, field.Text, item.state)
		}
	}
	outbound := observe(t, sysmonEvent("3", "Initiated", "true", "DestinationHostname", "-"))
	if field := fieldNamed(t, outbound.Fields, "EventData.DestinationHostname"); field.Text.ValueState != core.ValueStateAbsent ||
		field.Semantic != core.SemanticKeyConnectionDestinationReverseLookupName {
		t.Errorf("DestinationHostname - = %+v, want an absent %s", field,
			core.SemanticKeyConnectionDestinationReverseLookupName)
	}
}

// Sysmon の対応は Security のイベントに及ばず、Security の対応は Sysmon のイベントに及ばない。
func TestSysmonAndSecurityTablesStayApart(t *testing.T) {
	security := observe(t, strings.Replace(processCreationEvent, "<Data Name=\"SubjectUserName\">",
		"<Data Name=\"ProcessGuid\">"+syntheticChildGuid+"</Data><Data Name=\"SubjectUserName\">", 1))
	if field := fieldNamed(t, security.Fields, "EventData.ProcessGuid"); field.Semantic != "" {
		t.Errorf("a Security event maps ProcessGuid to %q", field.Semantic)
	}
	sysmon := observe(t, sysmonEvent("1", "NewProcessId", "0x2b"))
	if field := fieldNamed(t, sysmon.Fields, "EventData.NewProcessId"); field.Semantic != "" {
		t.Errorf("a Sysmon event maps NewProcessId to %q", field.Semantic)
	}
}

func TestItemSemanticsCoverTheSysmonFields(t *testing.T) {
	declared := winevent.ItemSemantics()
	for _, document := range []string{
		sysmonProcessCreateEvent,
		sysmonEvent("3", "UtcTime", "2001-02-03 04:05:06.700", "Initiated", "true", "DestinationIp", "198.51.100.7"),
		sysmonEvent("13", "TargetObject", `HKU\Example\Key07`, "Details", "example data"),
	} {
		for _, field := range observe(t, document).Fields {
			if field.Semantic != "" && !slices.Contains(declared, field.Semantic) {
				t.Errorf("%s carries %q, which ItemSemantics does not declare", field.Name, field.Semantic)
			}
		}
	}
}
