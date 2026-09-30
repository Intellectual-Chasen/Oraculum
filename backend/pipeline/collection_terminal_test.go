// in-package test: 収集の directory の収集元が 1 台の端末にまとまり、名前の履歴を持つことを確かめる。
package pipeline

import (
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// terminalNamingParser は、1 行を 1 件のレコードとして返し、行の `<名前>|<時刻>` を、レコードが
// 記録した端末自身の名前と時刻にする。registry の代わりである。
type terminalNamingParser struct {
	t      *testing.T
	lines  []string
	next   int
	offset int64
}

func (p *terminalNamingParser) Identity() ParserIdentity {
	return ParserIdentity{
		ParserID: "terminal-naming-test", FormatKey: "terminal_naming_test", PositionKind: core.PositionKindByteRange,
		TimePrecision: core.PrecisionSecond, RecordedByOneTerminal: true, ItemSemantics: []core.SemanticKey{},
		ConnectionRequestKinds: []core.ObservationKindSelector{}, ConnectionMatchConditions: []ConnectionMatchCondition{},
		TranscriptIdentityItems: []string{},
	}
}

func (p *terminalNamingParser) Reset(input io.Reader) {
	content, _ := io.ReadAll(input)
	p.lines, p.next, p.offset = strings.SplitAfter(strings.TrimSuffix(string(content), "\n"), "\n"), 0, 0
}

func (p *terminalNamingParser) Next() (ParsedRecord, *core.ImportFailure, error) {
	if p.next == len(p.lines) {
		return ParsedRecord{}, nil, io.EOF
	}
	line := p.lines[p.next]
	p.next++
	name, at, _ := strings.Cut(strings.TrimSuffix(line, "\n"), "|")
	length := int64(len(line))
	record := ParsedRecord{
		RawText: line, LineNumber: int64(p.next), ByteOffset: p.offset, ByteLength: &length,
		ObservedAt:      utcAt(p.t, at),
		TerminalNamings: []TerminalNaming{{Name: name}},
	}
	p.offset += length
	return record, nil, nil
}

// renameXML は端末の名前の変更のイベント 1 件を返す。
func renameXML(computer, at, recordID, previous, current string) string {
	return `<Event><System><Provider Name="EventLog"/><EventID>6011</EventID>` +
		`<TimeCreated SystemTime="` + at + `"/><EventRecordID>` + recordID + `</EventRecordID>` +
		`<Channel>System</Channel><Computer>` + computer + `</Computer></System>` +
		`<EventData><Data>` + previous + `</Data><Data>` + current + `</Data></EventData></Event>` + "\n"
}

// collectionSource は、走査した収集元 1 件と、その収集元を取り出した収集の directory である。
type collectionSource struct {
	scanned    scannedSource
	collection string
}

// collectionImportResult は収集元を並びの順に取り込み、収集元の識別に収集の directory を持たせる。
func collectionImportResult(t *testing.T, sources ...collectionSource) ImportResult {
	t.Helper()
	var scanned []scannedSource
	var statuses []core.ImportStatus
	for index, source := range sources {
		scanned = append(scanned, source.scanned)
		statuses = append(statuses, settleStatus(t, source.scanned, "collection-"+string(rune('a'+index))))
	}
	result, err := newImportResult(scanned, statuses, "run", indexRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	result.identities = make(map[string]core.SourceIdentity)
	for index, status := range statuses {
		result.identities[status.SourceId] = core.SourceIdentity{
			SourceId: status.SourceId, ContentSha256: status.Scope.SourceContentSha256,
			CollectionPath: sources[index].collection,
		}
	}
	return result
}

func eventSource(t *testing.T, name, collection string, events ...string) collectionSource {
	t.Helper()
	document := "<Events>\n" + strings.Join(events, "") + "</Events>\n"
	return collectionSource{
		scanned:    scanIndexSource(t, NewTestParser(WindowsEventXMLFormatKey, nil), name, WindowsEventXMLFormatKey, document),
		collection: collection,
	}
}

func TestCollectionJoinsTheRenamedTerminalAndKeepsOtherHostsApart(t *testing.T) {
	events := eventSource(t, "System.xml", "triage-a",
		processCreationXML("host-old.example.test", "2001-02-03T04:00:00Z", "801", "0x10", "0x1",
			`C:\Example\before-rename.exe`, ""),
		renameXML("host-old.example.test", "2001-02-03T05:00:00Z", "802", "HOST-OLD", "HOST-NEW"),
		processCreationXML("host-new.example.test", "2001-02-03T06:00:00Z", "803", "0x20", "0x1",
			`C:\Example\after-rename.exe`, ""),
		processCreationXML("forwarded.example.test", "2001-02-03T06:30:00Z", "804", "0x30", "0x1",
			`C:\Example\forwarded.exe`, ""))
	registry := collectionSource{
		scanned: scanIndexSource(t, &terminalNamingParser{t: t}, "SYSTEM", "terminal_naming_test",
			"HOST-NEW|2001-02-03T07:00:00Z\n"),
		collection: "triage-a",
	}
	otherCollection := eventSource(t, "Other.xml", "triage-b",
		processCreationXML("host-new.example.test", "2001-02-03T06:00:00Z", "901", "0x40", "0x1",
			`C:\Example\other-collection.exe`, ""))
	single := eventSource(t, "Single.xml", "",
		processCreationXML("host-new.example.test", "2001-02-03T06:00:00Z", "951", "0x50", "0x1",
			`C:\Example\single-file.exe`, ""))
	result := collectionImportResult(t, events, registry, otherCollection, single)
	graph := NewGraph(result, AllMatchConditions())

	shaOf := func(source collectionSource) string { return source.scanned.Measurement.ContentSha256 }
	digestA := collectionDigest([]string{shaOf(events), shaOf(registry)})
	if digestA != collectionDigest([]string{shaOf(registry), shaOf(events)}) {
		t.Error("the collection digest depends on the order of the sources")
	}
	collectionKey, _ := core.CollectionTerminalNodeKey(digestA)
	collectionAt := requireNodeAt(t, graph, collectionKey)
	label := graph.nodes[collectionAt].label
	if label.ValueState != core.ValueStateDerived || label.Normalized == nil || *label.Normalized != "HOST-NEW" {
		t.Errorf("the collection terminal is labelled %+v, want the registry name", label)
	}
	for _, path := range []string{`C:\Example\before-rename.exe`, `C:\Example\after-rename.exe`} {
		if !hasEdge(graph, core.EdgeKindRanOn, processNodeLabelled(t, graph, path), collectionAt) {
			t.Errorf("%s did not run on the collection terminal", path)
		}
	}
	separate := []struct {
		path, digest, hostname string
	}{
		{`C:\Example\forwarded.exe`, digestA, "forwarded.example.test"},
		{`C:\Example\other-collection.exe`, collectionDigest([]string{shaOf(otherCollection)}), "host-new.example.test"},
		{`C:\Example\single-file.exe`, shaOf(single), "host-new.example.test"},
	}
	for _, want := range separate {
		processAt := processNodeLabelled(t, graph, want.path)
		host, _ := core.RecordingHostTerminalNodeKey(want.digest, want.hostname)
		if !hasEdge(graph, core.EdgeKindRanOn, processAt, requireNodeAt(t, graph, host)) ||
			hasEdge(graph, core.EdgeKindRanOn, processAt, collectionAt) {
			t.Errorf("%s did not run on its own terminal of %s", want.path, want.hostname)
		}
	}

	// 収集の端末に置いた、ホスト名を持たない収集元のレコードは、時系列の行に収集の端末を出す。
	query := TimelineQuery{}
	query.Validate()
	registryRows := 0
	for _, entry := range graph.Timeline(query).Entries {
		if entry.RecordRef.SourceFileName != "SYSTEM" {
			continue
		}
		registryRows++
		if entry.Terminal == nil || entry.Terminal.Id != nodeIdOf(collectionKey) {
			t.Errorf("the registry row shows the terminal %+v, want the collection terminal", entry.Terminal)
		}
	}
	if registryRows != 1 {
		t.Errorf("the timeline has %d registry rows, want 1", registryRows)
	}

	names := graph.TerminalNames(nodeIdOf(collectionKey))
	type nameRange struct{ name, first, last string }
	var got []nameRange
	for _, name := range names {
		if name.First == nil || name.Last == nil {
			t.Fatalf("the name %q has no time", name.Name)
		}
		got = append(got, nameRange{name.Name, *name.First.Normalized, *name.Last.Normalized})
	}
	want := []nameRange{
		{"HOST-OLD", "2001-02-03T05:00:00Z", "2001-02-03T05:00:00Z"},
		{"HOST-NEW", "2001-02-03T05:00:00Z", "2001-02-03T07:00:00Z"},
	}
	if len(got) != len(want) {
		t.Fatalf("the names are %+v, want %+v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("the name %d is %+v, want %+v", index, got[index], want[index])
		}
	}
	if len(names[1].RecordRefs) != 2 || names[1].RecordRefs[0].SourceId == names[1].RecordRefs[1].SourceId {
		t.Errorf("the current name is recorded by %+v, want the rename and the registry", names[1].RecordRefs)
	}
	if other := graph.TerminalNames(nodeIdOf(collectionKey) + "-absent"); len(other) != 0 {
		t.Errorf("a node without a collection has the names %+v", other)
	}
}

// 2 つの収集の registry が同じ名前を記録しても、収集の端末は収集ごとに 2 つあり、各収集の
// イベントログの同じ名前の Computer は、自分の収集の端末だけに置く。
func TestCollectionsWithTheSameRegistryNameStayApart(t *testing.T) {
	collection := func(directory, eventName, recordID, path string) (collectionSource, collectionSource) {
		events := eventSource(t, eventName, directory,
			processCreationXML("host-same.example.test", "2001-02-03T06:00:00Z", recordID, "0x10", "0x1", path, ""))
		registry := collectionSource{
			scanned: scanIndexSource(t, &terminalNamingParser{t: t}, "SYSTEM-"+directory, "terminal_naming_test",
				"HOST-SAME|2001-02-03T07:00:00Z\n"),
			collection: directory,
		}
		return events, registry
	}
	eventsA, registryA := collection("triage-e", "A.xml", "811", `C:\Example\collection-e.exe`)
	eventsB, registryB := collection("triage-f", "B.xml", "812", `C:\Example\collection-f.exe`)
	graph := NewGraph(collectionImportResult(t, eventsA, registryA, eventsB, registryB), AllMatchConditions())

	shaOf := func(source collectionSource) string { return source.scanned.Measurement.ContentSha256 }
	terminalOf := func(events, registry collectionSource) int {
		key, _ := core.CollectionTerminalNodeKey(collectionDigest([]string{shaOf(events), shaOf(registry)}))
		return requireNodeAt(t, graph, key)
	}
	terminalA, terminalB := terminalOf(eventsA, registryA), terminalOf(eventsB, registryB)
	if terminalA == terminalB {
		t.Fatal("the two collections share one terminal")
	}
	for _, want := range []struct {
		path       string
		own, other int
	}{{`C:\Example\collection-e.exe`, terminalA, terminalB}, {`C:\Example\collection-f.exe`, terminalB, terminalA}} {
		processAt := processNodeLabelled(t, graph, want.path)
		if !hasEdge(graph, core.EdgeKindRanOn, processAt, want.own) ||
			hasEdge(graph, core.EdgeKindRanOn, processAt, want.other) {
			t.Errorf("%s did not run only on its own collection terminal", want.path)
		}
	}
}

// registry が名前を記録していない収集は、名前の集合を持たず、イベントログのホスト名を収集の端末に
// まとめない。1 台の端末が書く収集元は、収集の directory の path を表示名にした収集の端末に置く。
func TestCollectionWithoutARegistryNameKeepsTheHostTerminals(t *testing.T) {
	events := eventSource(t, "System.xml", "triage-c",
		renameXML("host-old.example.test", "2001-02-03T05:00:00Z", "802", "HOST-OLD", "HOST-NEW"),
		processCreationXML("host-new.example.test", "2001-02-03T06:00:00Z", "803", "0x20", "0x1",
			`C:\Example\after-rename.exe`, ""))
	result := collectionImportResult(t, events)
	graph := NewGraph(result, AllMatchConditions())
	digest := collectionDigest([]string{events.scanned.Measurement.ContentSha256})
	host, _ := core.RecordingHostTerminalNodeKey(digest, "host-new.example.test")
	if !hasEdge(graph, core.EdgeKindRanOn, processNodeLabelled(t, graph, `C:\Example\after-rename.exe`),
		requireNodeAt(t, graph, host)) {
		t.Error("the process did not run on the terminal of its hostname")
	}
	collectionKey, _ := core.CollectionTerminalNodeKey(digest)
	if _, present := graph.nodeAt[nodeIdOf(collectionKey)]; present {
		t.Error("the collection terminal exists without a registry name")
	}
}

// 収集の端末の改名の前の名前をドメインに書いたアカウントの名前のノードは、今の名前と共に記録した
// SID のノードと同じアカウントの候補のエッジで結ばれる。端末の名前でないドメインの同じログイン名は
// 結ばれない。
func TestCollectionOldHostnameAccountPairsWithTheSid(t *testing.T) {
	events := eventSource(t, "Security.xml", "triage-d",
		renameXML("host-new.example.test", "2001-02-03T05:00:00Z", "700", "HOST-OLD", "HOST-NEW"),
		securityEventXML("701", "4720", "host-new.example.test", "2001-02-03T06:00:00Z",
			"TargetSid", aliasSid, "TargetUserName", aliasUser, "TargetDomainName", "HOST-NEW"),
		remoteDesktopXML(rdpLsm, "21", "702", "2001-02-03T04:00:00Z",
			`<UserData><EventXML><User>host-old\`+aliasUser+`</User><Address>192.0.2.30</Address></EventXML></UserData>`),
		remoteDesktopXML(rdpLsm, "21", "703", "2001-02-03T04:00:00Z",
			`<UserData><EventXML><User>unrelated\`+aliasUser+`</User><Address>192.0.2.30</Address></EventXML></UserData>`))
	registry := collectionSource{
		scanned: scanIndexSource(t, &terminalNamingParser{t: t}, "SYSTEM", "terminal_naming_test",
			"HOST-NEW|2001-02-03T07:00:00Z\n"),
		collection: "triage-d",
	}
	graph := NewGraph(collectionImportResult(t, events, registry), AllMatchConditions())
	sidNode := accountNodeOfSid(graph, aliasSid)
	if sidNode < 0 {
		t.Fatal("no account node carries the SID")
	}
	identity := edgePairsOfKind(graph, core.EdgeKindAccountIdentityMatch)
	if named := nameNodeNamedBy(t, graph, recordOfEvent(t, graph, "702")); !slices.Contains(identity, [2]int{named, sidNode}) {
		t.Errorf("the old hostname account %d has no identity candidate to the SID node %d, edges %v", named, sidNode, identity)
	}
	if named := nameNodeNamedBy(t, graph, recordOfEvent(t, graph, "703")); slices.Contains(identity, [2]int{named, sidNode}) {
		t.Errorf("the unrelated domain account %d pairs with the SID node %d", named, sidNode)
	}
}
