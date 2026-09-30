// in-package test: ファイルとレジストリの値の内容のバージョンを確かめる。
package pipeline

import (
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// markiiObjectLine は、 markii 形式の操作のレコード 1 行を返す。
func markiiObjectLine(clock, sequence, event, subEvent, path string) string {
	return "01/02/2024 " + clock + ".000 +0000 sn=" + sequence + " evt=" + event + " subEvt=" + subEvent +
		` psGUID={P1} tmid=T1 com="PC01" path="` + path + `"` + "\n"
}

// new=1 の close は新規ファイルの作成を記録し、前の書き込みはその後の読み込みにつながらない。
// new=0 の close と new を持たない close はバージョンを区切らない。
func TestContentVersionSplitsTheFileAtTheNewFileClose(t *testing.T) {
	line := func(clock, sequence, extra string) string {
		return "01/02/2024 " + clock + ".000 +0000 sn=" + sequence + " evt=file subEvt=close" +
			` psGUID={P1} tmid=T1 com="PC01" path="C:\data\c.txt"` + extra + "\n"
	}
	document := line("03:00:00", "1", "") +
		line("03:01:00", "2", " new=0") +
		line("03:02:00", "3", "") +
		line("03:03:00", "4", " new=1") +
		line("03:04:00", "5", "")
	result := sessionSourcesResult(t, sessionSource{name: "t.log", format: MarkIIFormatKey, document: document})
	graph := NewGraph(result, AllMatchConditions())
	file := operatedNodeOf(t, graph, core.EdgeKindFileOperation)
	requireIncludesWrite(t, graph, file, map[[2]int]bool{
		{0, 1}: true, {0, 2}: true, {1, 2}: true,
		{0, 4}: false, {2, 4}: false, {3, 4}: true,
	})
}

// 欄の条件は、その名前の項目を持たないレコード、原資料の文字列を持たない項目、文字列の違う項目に
// 一致しない。
func TestReplacesContentRequiresTheFieldText(t *testing.T) {
	kind := core.ObservationKind{Raw: []core.RecordField{textRecordField(t, "kind", "k")}}
	publication := SourcePublication{parser: ParserIdentity{ContentReplacementKinds: []core.ContentReplacementSelector{{
		Kind:   core.ObservationKindSelector{Items: []core.ObservationKindSelectorItem{{Name: "kind", Value: "k"}}},
		Fields: []core.ObservationKindSelectorItem{{Name: "flag", Value: "1"}},
	}}}}
	absent := core.RecordField{Name: "flag", Text: &core.RawAndNormalized{ValueState: core.ValueStateAbsent}}
	cases := map[string]struct {
		fields []core.RecordField
		want   bool
	}{
		"文字列が一致する":      {[]core.RecordField{textRecordField(t, "flag", "1")}, true},
		"文字列が違う":        {[]core.RecordField{textRecordField(t, "flag", "0")}, false},
		"項目が無い":         {nil, false},
		"項目が文字列の欄を持たない": {[]core.RecordField{{Name: "flag"}}, false},
		"文字列が無い":        {[]core.RecordField{absent}, false},
	}
	for name, test := range cases {
		if got := publication.replacesContent(kind, test.fields); got != test.want {
			t.Errorf("%s: replacesContent = %v, want %v", name, got, test.want)
		}
	}
}

// operatedNodeOf は、kind のエッジが向かう唯一のノードの位置を返す。
func operatedNodeOf(t *testing.T, graph Graph, kind core.EdgeKind) int {
	t.Helper()
	targets := map[int]struct{}{}
	target := -1
	for _, edge := range graph.edges {
		if edge.kind == kind {
			targets[edge.target], target = struct{}{}, edge.target
		}
	}
	if len(targets) != 1 {
		t.Fatalf("%s reaches %d nodes, want one", kind, len(targets))
	}
	return target
}

// requireIncludesWrite は、レコードの位置の組 (書き込み, 読み込み) ごとに contentIncludesWrite の値を確かめる。
func requireIncludesWrite(t *testing.T, graph Graph, node int, want map[[2]int]bool) {
	t.Helper()
	for pair, included := range want {
		if got := graph.contentIncludesWrite(node, pair[0], pair[1]); got != included {
			t.Errorf("contentIncludesWrite(write %d, read %d) = %v, want %v", pair[0], pair[1], got, included)
		}
	}
}

// 追記の close はバージョンを区切らず、前の書き込みが後の読み込みに含まれる。削除の後に作成した
// ファイルの読み込みは、削除の前の書き込みを含まず、削除の後の書き込みを含む。書き込みより前の
// 読み込みは、その書き込みを含まない。
func TestContentVersionSplitsTheFileAtTheDeletion(t *testing.T) {
	document := markiiObjectLine("03:00:00", "1", "file", "close", `C:\data\a.txt`) +
		markiiObjectLine("03:01:00", "2", "file", "close", `C:\data\a.txt`) +
		markiiObjectLine("03:02:00", "3", "file", "close", `C:\data\a.txt`) +
		markiiObjectLine("03:03:00", "4", "file", "del", `C:\data\a.txt`) +
		markiiObjectLine("03:04:00", "5", "file", "close", `C:\data\a.txt`) +
		markiiObjectLine("03:05:00", "6", "file", "close", `C:\data\a.txt`)
	result := sessionSourcesResult(t, sessionSource{name: "t.log", format: MarkIIFormatKey, document: document})
	graph := NewGraph(result, AllMatchConditions())
	file := operatedNodeOf(t, graph, core.EdgeKindFileOperation)
	requireIncludesWrite(t, graph, file, map[[2]int]bool{
		{0, 1}: true, {0, 2}: true, {1, 2}: true,
		{0, 5}: false, {2, 5}: false,
		{4, 5}: true, {3, 5}: true,
		{2, 1}: false,
	})
	if start, replaced := graph.contentVersionStartAt(file, graph.records[2].instant); replaced {
		t.Errorf("the version at the third record starts at %v, want no replacement", start)
	}
	if start, _ := graph.contentVersionStartAt(file, graph.records[5].instant); !start.Equal(graph.records[3].instant) {
		t.Errorf("the version at the last record starts at %v, want the deletion %v", start, graph.records[3].instant)
	}
}

// レジストリの値の設定は値の全体を置き換え、前の設定は次の設定の後の読み込みに含まれない。
func TestContentVersionSplitsTheRegistryValueAtEachSet(t *testing.T) {
	path := `HKLM\SOFTWARE\Example\Value`
	document := markiiObjectLine("03:00:00", "1", "reg", "setVal", path) +
		markiiObjectLine("03:01:00", "2", "reg", "setVal", path) +
		markiiObjectLine("03:02:00", "3", "reg", "delVal", path)
	result := sessionSourcesResult(t, sessionSource{name: "t.log", format: MarkIIFormatKey, document: document})
	graph := NewGraph(result, AllMatchConditions())
	value := operatedNodeOf(t, graph, core.EdgeKindRegistryOperation)
	requireIncludesWrite(t, graph, value, map[[2]int]bool{
		{0, 0}: true, {0, 1}: false, {1, 1}: true, {1, 2}: false,
	})
}

// Sysmon の 11 はファイルの作成と上書きを記録し、上書きの前の書き込みは上書きの後の記録に
// 含まれない。2 (作成時刻の変更) はバージョンを区切らない。
func TestContentVersionSplitsTheFileAtTheSysmonOverwrite(t *testing.T) {
	event := func(recordID, eventID, systemTime string) string {
		return taskEventXML("Microsoft-Windows-Sysmon", recordID, eventID, logonHostA, systemTime,
			"UtcTime", "2001-02-03 04:05:00.000", "ProcessGuid", "{00000000-0000-0000-0000-000000000001}",
			"ProcessId", "4", "Image", `C:\tool.exe`, "TargetFilename", `C:\data\b.txt`)
	}
	document := "<Events>\n" +
		event("1", "11", "2001-02-03T04:05:00.000Z") +
		event("2", "2", "2001-02-03T04:06:00.000Z") +
		event("3", "11", "2001-02-03T04:07:00.000Z") +
		event("4", "2", "2001-02-03T04:08:00.000Z") +
		"</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	file := operatedNodeOf(t, graph, core.EdgeKindFileOperation)
	at := func(recordID string) int { return recordOfEvent(t, graph, recordID) }
	requireIncludesWrite(t, graph, file, map[[2]int]bool{
		{at("1"), at("2")}: true, {at("1"), at("4")}: false, {at("2"), at("4")}: false, {at("3"), at("4")}: true,
	})
}

// Sysmon の 13 は値の設定を記録し、前の設定は次の設定の後の記録に含まれない。
func TestContentVersionSplitsTheRegistryValueAtTheSysmonSet(t *testing.T) {
	event := func(recordID, systemTime string) string {
		return taskEventXML("Microsoft-Windows-Sysmon", recordID, "13", logonHostA, systemTime,
			"EventType", "SetValue", "UtcTime", "2001-02-03 04:05:00.000",
			"ProcessGuid", "{00000000-0000-0000-0000-000000000001}", "ProcessId", "4", "Image", `C:\tool.exe`,
			"TargetObject", `HKLM\SOFTWARE\Example\Value`)
	}
	document := "<Events>\n" +
		event("1", "2001-02-03T04:05:00.000Z") + event("2", "2001-02-03T04:06:00.000Z") + "</Events>\n"
	graph := NewGraph(windowsEventImportResult(t, document), AllMatchConditions())
	value := operatedNodeOf(t, graph, core.EdgeKindRegistryOperation)
	at := func(recordID string) int { return recordOfEvent(t, graph, recordID) }
	requireIncludesWrite(t, graph, value, map[[2]int]bool{{at("1"), at("1")}: true, {at("1"), at("2")}: false})
}

// file_operation と registry_operation のほかの関係の根拠は、置き換えの記録でもバージョンを
// 区切らない。タイムスタンプを持たない記録は、書き込みにも読み込みにもならない。
func TestContentVersionReadsOnlyTheOperationsWithTimestamps(t *testing.T) {
	at := time.Date(2001, 2, 3, 4, 5, 0, 0, time.UTC)
	graph := Graph{
		records: []graphRecord{
			{instant: at, hasInstant: true},
			{instant: at.Add(time.Minute), hasInstant: true, replacesContent: true},
			{instant: at.Add(2 * time.Minute), hasInstant: true},
			{replacesContent: true},
		},
		adjacency: []nodeAdjacency{{}, {incoming: []int{0, 1, 2}}},
		edges: []graphEdge{
			{kind: core.EdgeKindFileOperation, target: 1, evidence: []int{0, 2, 3}},
			{kind: core.EdgeKindFileCopy, target: 1, evidence: []int{1}},
			{kind: core.EdgeKindFileOperation, target: 1, evidence: []int{3}},
		},
	}
	requireIncludesWrite(t, graph, 1, map[[2]int]bool{{0, 2}: true, {3, 2}: false, {0, 3}: false})
}
