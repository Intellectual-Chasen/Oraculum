package pipeline

import (
	"reflect"
	"strings"
	"testing"
)

// eventKindsGraph は、事象の分類と動作の組を件数の異なる形で持つレコードと、分類の欄を
// 持たない入力形式のレコードから組んだグラフである。値はであり、組ごとの件数を挙げて
// 確かめるために置く。
func eventKindsGraph(t *testing.T) Graph {
	t.Helper()
	const common = " tmid=t com=TESTHOST csid=s psPath=app"
	markii := []string{
		"02/01/2000 03:04:01.000 +0900 sn=1 evt=ps subEvt=start psGUID=p1" + common,
		"02/01/2000 03:04:02.000 +0900 sn=2 evt=ps subEvt=start psGUID=p2" + common,
		"02/01/2000 03:04:03.000 +0900 sn=3 evt=ps subEvt=stop psGUID=p3" + common,
		"02/01/2000 03:04:04.000 +0900 sn=4 evt=file subEvt=close psGUID=p1" + common,
		"02/01/2000 03:04:05.000 +0900 sn=5 evt=net subEvt=con psGUID=p1" + common +
			" srcIP=192.0.2.10 srcPort=51724 dstIP=198.51.100.42 dstPort=8080",
	}
	squid := []string{
		`192.0.2.1 - - [10/Oct/2000:13:55:36 +0000] "GET http://example.test/ HTTP/1.1" 200 12 "-" "test" TCP_MISS:DIRECT`,
		`192.0.2.1 - - [10/Oct/2000:13:55:37 +0000] "GET http://example.test/a HTTP/1.1" 200 12 "-" "test" TCP_MISS:DIRECT`,
	}
	return NewGraph(caseImport(t,
		markIISource("kinds.log", strings.Join(markii, "\n")+"\n", nil),
		caseSource{name: "access.log", format: string(SquidFormatKey),
			content: strings.Join(squid, "\n") + "\n"},
	), AllMatchConditions())
}

func TestEventKindsCountEachCategoryAndActionPair(t *testing.T) {
	kinds, uncategorized := eventKindsGraph(t).EventKinds(RecordFilter{})

	// 件数の多い順に並び、同じ件数の組は分類、動作の文字列の順に並ぶ。
	want := []EventKindCount{
		{Category: "ps", Action: "start", RecordCount: 2},
		{Category: "file", Action: "close", RecordCount: 1},
		{Category: "net", Action: "con", RecordCount: 1},
		{Category: "ps", Action: "stop", RecordCount: 1},
	}
	if !reflect.DeepEqual(kinds, want) {
		t.Errorf("kinds = %+v, want %+v", kinds, want)
	}
	// squid の 2 行は事象の分類の欄を持たない。
	if uncategorized != 2 {
		t.Errorf("uncategorized = %d, want 2", uncategorized)
	}
}

func TestEventKindsIgnoreTheEventKindConditionsOfTheFilter(t *testing.T) {
	graph := eventKindsGraph(t)
	whole, wholeUncategorized := graph.EventKinds(RecordFilter{})

	narrowed, narrowedUncategorized := graph.EventKinds(RecordFilter{EventCategory: "file", EventAction: "close"})

	if !reflect.DeepEqual(narrowed, whole) || narrowedUncategorized != wholeUncategorized {
		t.Errorf("narrowed = %+v (%d), want %+v (%d)",
			narrowed, narrowedUncategorized, whole, wholeUncategorized)
	}
}

// windowsEventKindsGraph は、Windows イベントログの XML の 4 件と、Windows イベントログを
// 写した markii 形式の 1 行から組んだグラフである。
func windowsEventKindsGraph(t *testing.T) Graph {
	t.Helper()
	event := func(provider, eventID, recordID string) string {
		return `<Event><System>` + provider + `<EventID>` + eventID + `</EventID>` +
			`<TimeCreated SystemTime="2001-02-03T04:05:0` + recordID + `Z"/>` +
			`<EventRecordID>` + recordID + `</EventRecordID><Channel>Security</Channel>` +
			`<Computer>host01.example.test</Computer></System></Event>`
	}
	const security = `<Provider Name="Microsoft-Windows-Security-Auditing"/>`
	// 1 行に 1 件を置く。同じ行の 2 件は同じ位置を名乗る。
	document := strings.Join([]string{`<Events>`,
		event(security, "4624", "1"), event(security, "4624", "2"), event(security, "4688", "3"),
		// プロバイダを持たないイベントは、事象の種別を持たない。
		event("", "4624", "4"),
		`</Events>`}, "\n")
	markii := "02/01/2000 03:04:01.000 +0900 sn=1 evt=os subEvt=evtLog tmid=t com=TESTHOST csid=s " +
		`channel="Security" evtID=4624 evtRecID=5 evtSrc="Microsoft-Windows-Security-Auditing"` + "\n"
	return NewGraph(caseImport(t,
		caseSource{name: "events.xml", format: string(WindowsEventXMLFormatKey), content: document},
		markIISource("evtlog.log", markii, nil),
	), AllMatchConditions())
}

// 事象の分類を持たない Windows イベントログのレコードは、プロバイダとイベント ID の組を
// 事象の種別にする。事象の分類を持つレコードは、Windows イベントログの項目を持っても分類と
// 動作の組を保つ。
func TestEventKindsPairTheProviderAndTheEventIdOfAWindowsEvent(t *testing.T) {
	kinds, uncategorized := windowsEventKindsGraph(t).EventKinds(RecordFilter{})

	want := []EventKindCount{
		{Category: "Microsoft-Windows-Security-Auditing", Action: "4624", RecordCount: 2, WindowsEvent: true},
		{Category: "Microsoft-Windows-Security-Auditing", Action: "4688", RecordCount: 1, WindowsEvent: true},
		{Category: "os", Action: "evtLog", RecordCount: 1},
	}
	if !reflect.DeepEqual(kinds, want) {
		t.Errorf("kinds = %+v, want %+v", kinds, want)
	}
	if uncategorized != 1 {
		t.Errorf("uncategorized = %d, want the event without a provider", uncategorized)
	}
}

// eventKindsOfSources は、Windows イベントログの XML の要素の並びと markii 形式の行から組んだ
// グラフの事象の種別を返す。1 行に 1 件を置く。同じ行の 2 件は同じ位置を名乗る。
func eventKindsOfSources(t *testing.T, events, markiiLines []string) ([]EventKindCount, int64) {
	t.Helper()
	document := "<Events>\n" + strings.Join(events, "\n") + "\n</Events>"
	markii := strings.Join(markiiLines, "\n") + "\n"
	return NewGraph(caseImport(t,
		caseSource{name: "events.xml", format: string(WindowsEventXMLFormatKey), content: document},
		markIISource("evtlog.log", markii, nil),
	), AllMatchConditions()).EventKinds(RecordFilter{})
}

// systemOnlyEvent は System の中身を与えた Windows イベントログの 1 件である。
func systemOnlyEvent(system string) string {
	return `<Event><System>` + system + `<Computer>host01.example.test</Computer></System></Event>`
}

// イベント ID を持たないイベントと空の文字列のイベントは、組を作らず分類を持たない件数に入る。
// 組を作ると、動作の空な組が、動作で絞らない条件としてプロバイダの全件を指す。
func TestEventKindsLeaveAWindowsEventWithoutAnEventIdUncategorized(t *testing.T) {
	const provider = `<Provider Name="Example-Provider"/>`
	kinds, uncategorized := eventKindsOfSources(t, []string{
		systemOnlyEvent(provider + `<EventID>42</EventID><EventRecordID>1</EventRecordID>`),
		systemOnlyEvent(provider + `<EventRecordID>2</EventRecordID>`),
		systemOnlyEvent(provider + `<EventID></EventID><EventRecordID>3</EventRecordID>`),
	}, []string{"02/01/2000 03:04:01.000 +0900 sn=1 evt=ps subEvt=start psGUID=p1 tmid=t com=TESTHOST csid=s psPath=app"})

	want := []EventKindCount{
		{Category: "Example-Provider", Action: "42", RecordCount: 1, WindowsEvent: true},
		{Category: "ps", Action: "start", RecordCount: 1},
	}
	if !reflect.DeepEqual(kinds, want) || uncategorized != 2 {
		t.Errorf("kinds = %+v (%d), want %+v and the two events without an event id", kinds, uncategorized, want)
	}
}

// 事象の分類の欄を持ち、値が不在のレコードは、プロバイダの組に切り替わらず分類を持たない。
func TestEventKindsKeepARecordWithAnAbsentCategoryUncategorized(t *testing.T) {
	kinds, uncategorized := eventKindsOfSources(t, nil, []string{
		"02/01/2000 03:04:01.000 +0900 sn=1 evt=\"-\" subEvt=evtLog tmid=t com=TESTHOST csid=s " +
			`evtID=42 evtSrc="Example-Provider"`,
	})
	if len(kinds) != 0 || uncategorized != 1 {
		t.Errorf("kinds = %+v (%d), want the record without a category", kinds, uncategorized)
	}
}

// 事象の分類がプロバイダの名前と同じ文字列の組は、Windows イベントログの組と 1 つにまとまり、
// Windows イベントログの組と示さない。
func TestEventKindsMarkAPairMixingTheTwoOriginsAsNotAWindowsEvent(t *testing.T) {
	kinds, _ := eventKindsOfSources(t, []string{
		systemOnlyEvent(`<Provider Name="Example-Provider"/><EventID>42</EventID><EventRecordID>1</EventRecordID>`),
	}, []string{
		"02/01/2000 03:04:01.000 +0900 sn=1 evt=Example-Provider subEvt=42 tmid=t com=TESTHOST csid=s",
	})
	want := []EventKindCount{{Category: "Example-Provider", Action: "42", RecordCount: 2}}
	if !reflect.DeepEqual(kinds, want) {
		t.Errorf("kinds = %+v, want %+v", kinds, want)
	}
}

// プロバイダとイベント ID の組を事象の種別の条件に与えると、その組のレコードだけが残る。
func TestRecordFilterNarrowsWindowsEventsByTheEventId(t *testing.T) {
	timeline := windowsEventKindsGraph(t).Timeline(TimelineQuery{RecordFilter: RecordFilter{
		EventCategory: "Microsoft-Windows-Security-Auditing", EventAction: "4624",
	}})
	var eventIDs []string
	for _, entry := range timeline.Entries {
		for _, field := range entry.ObservationKind.Raw {
			if field.Name == "EventID" {
				raw, _ := field.Text.RawTextValue()
				eventIDs = append(eventIDs, raw)
			}
		}
	}
	// markii 形式の行は事象の分類 os を持ち、プロバイダの組で絞る条件に入らない。
	if !reflect.DeepEqual(eventIDs, []string{"4624", "4624"}) || len(timeline.Entries) != len(eventIDs) {
		t.Errorf("entries = %+v, want the two 4624 events of the provider", timeline.Entries)
	}
}
