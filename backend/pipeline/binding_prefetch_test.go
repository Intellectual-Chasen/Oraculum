// in-package test: Prefetch の file を取り込み、端末とファイルのノードを確かめる。
package pipeline

import (
	"encoding/binary"
	"io"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// syntheticPrefetch は、形式 17 の展開した形の Prefetch の file を組む。実行時刻は 1 つ、
// volume は持たない。本 package の test は adapter を import しないため、ここで byte を組む。
func syntheticPrefetch(name string, lastRun uint64, files ...string) string {
	const filenamesAt = 0x98
	utf16le := func(text string) []byte {
		var encoded []byte
		for _, unit := range utf16.Encode([]rune(text)) {
			encoded = binary.LittleEndian.AppendUint16(encoded, unit)
		}
		return encoded
	}
	content := make([]byte, filenamesAt)
	binary.LittleEndian.PutUint32(content[0x00:], 17)
	copy(content[0x04:], "SCCA")
	copy(content[0x10:], utf16le(name))
	var names []byte
	for _, file := range files {
		names = append(append(names, utf16le(file)...), 0, 0)
	}
	binary.LittleEndian.PutUint32(content[0x64:], filenamesAt)
	binary.LittleEndian.PutUint32(content[0x68:], uint32(len(names)))
	binary.LittleEndian.PutUint32(content[0x6C:], uint32(filenamesAt+len(names)))
	binary.LittleEndian.PutUint64(content[0x78:], lastRun)
	binary.LittleEndian.PutUint32(content[0x90:], 1)
	content = append(content, names...)
	binary.LittleEndian.PutUint32(content[0x0C:], uint32(len(content)))
	return string(content)
}

// 壊れた file はレコードを持たず失敗だけを持つ。端末を指定した取り込みは、その file に
// 端末を付けずに進み、ほかの file のレコードを指定した端末に置く。
func TestTerminalSpecifiedImportKeepsAFileWithOnlyFailures(t *testing.T) {
	documents := map[string]string{
		"TOOL63.EXE-00000003.pf":  syntheticPrefetch("TOOL63.EXE", 128000000000000000, `\VOLUME{01}\A\TOOL63.EXE`),
		"DAMAGED.EXE-00000004.pf": strings.Repeat("\x00", 64),
	}
	runner, err := NewRunner(Config{
		Open: func(originPath string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(documents[originPath])), nil
		},
		Parsers: NewTestFormatRegistry(),
		Minter:  DigestMinter{}, Ordinals: NewInMemoryOrdinals(),
		Sanitize: func(value string) string { return value },
		Revision: "prefetch-revision", SettingsDigest: "prefetch-settings",
	})
	if err != nil {
		t.Fatal(err)
	}
	var plans []SourcePlan
	for _, name := range []string{"TOOL63.EXE-00000003.pf", "DAMAGED.EXE-00000004.pf"} {
		plans = append(plans, SourcePlan{OriginPath: name, FileName: name, FormatKey: WindowsPrefetchFormatKey,
			Terminal: &SourceTerminal{TerminalId: "pf-host"}})
	}
	result, err := runner.Run(plans)
	if err != nil {
		t.Fatalf("the import stopped: %v", err)
	}
	if len(result.importAssignments) != 1 || result.publications[1].status.FailureCount != 1 {
		t.Errorf("assignments %d, failures of the damaged file %d; want the valid file alone assigned and the failure kept",
			len(result.importAssignments), result.publications[1].status.FailureCount)
	}
	terminalKey, _ := core.TerminalNodeKey("pf-host")
	requireNodeAt(t, NewGraph(result, AllMatchConditions()), terminalKey)
}

// 同じ端末の外部識別子を指定した 2 つの Prefetch の file は、1 つの端末のノードに置かれる。
// 各レコードは最新の実行時刻を持ち、実行ファイルの path のファイルのノードを指す。
func TestPrefetchFilesOfOneTerminalNameTheirExecutables(t *testing.T) {
	parser := NewTestPrefetchParser()
	sources := []scannedSource{
		scanIndexSource(t, parser, "TOOL61.EXE-00000001.pf", WindowsPrefetchFormatKey,
			syntheticPrefetch("TOOL61.EXE", 128000000000000000, `\VOLUME{01}\A\NTDLL.DLL`, `\VOLUME{01}\A\TOOL61.EXE`)),
		scanIndexSource(t, parser, "TOOL62.EXE-00000002.pf", WindowsPrefetchFormatKey,
			syntheticPrefetch("TOOL62.EXE", 128000000010000000, `\VOLUME{01}\B\TOOL62.EXE`)),
	}
	// レコードは行を持たないため、原文の参照を file の名前と byte の位置から組む。
	rawTextRef := func(locator core.RecordLocator, _ string) string {
		return "raw:" + locator.SourceFileName + ":" + strconv.FormatInt(*locator.ByteOffset, 10)
	}
	statuses := make([]core.ImportStatus, 0, len(sources))
	for index, source := range sources {
		status, err := buildImportStatus(source, "run", "parser-v1", "pf-"+strconv.Itoa(index), settleSanitize, rawTextRef)
		if err != nil {
			t.Fatal(err)
		}
		statuses = append(statuses, status)
	}
	result, err := newImportResult(sources, statuses, "run", rawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	for _, publication := range result.publications {
		record := publication.records[0]
		if record.ObservedAt == nil || record.Locator.ByteOffset == nil || *record.Locator.ByteOffset != 0 {
			t.Fatalf("the record of %s is %+v, want the run time and the whole file", publication.status.SourceId, record)
		}
		assignment := core.TerminalAssignment{
			TerminalId: "pf-host", TerminalHostname: "pf-host.example.test",
			SourceId:             publication.status.SourceId,
			SourceContentSha256:  publication.status.Scope.SourceContentSha256,
			AssignmentValidRange: core.TimeRange{From: *record.ObservedAt, To: *record.ObservedAt},
			Origin:               core.TerminalAssignmentOriginImportSpecified,
			AppliesToSourceId:    publication.status.SourceId,
		}
		if err := assignment.Validate(); err != nil {
			t.Fatalf("building the import specification: %v", err)
		}
		result.importAssignments = append(result.importAssignments, assignment)
	}
	graph := NewGraph(result, AllMatchConditions())

	terminals := 0
	for _, node := range graph.nodes {
		if node.key.Kind == core.NodeKindTerminal {
			terminals++
		}
	}
	terminalKey, _ := core.TerminalNodeKey("pf-host")
	if terminals != 1 {
		t.Errorf("the graph holds %d terminal nodes, want the specified terminal alone", terminals)
	}
	requireNodeAt(t, graph, terminalKey)
	for _, item := range []struct {
		record int
		path   string
		label  string
	}{
		{0, `\VOLUME{01}\A\TOOL61.EXE`, "TOOL61.EXE"},
		{1, `\VOLUME{01}\B\TOOL62.EXE`, "TOOL62.EXE"},
		{0, `\VOLUME{01}\A\NTDLL.DLL`, "NTDLL.DLL"},
	} {
		files := fileNodesOf(graph, item.path)
		if len(files) != 1 {
			t.Fatalf("the graph holds the file nodes %v for %s, want 1", files, item.path)
		}
		record := graph.records[item.record].recordNode
		if !hasEdge(graph, core.EdgeKindRecordNamesObject, record, files[0]) {
			t.Errorf("the record %d does not name the file node of %s", item.record, item.path)
		}
		// 鍵の path は小文字にそろえ、表示名は記録した文字列から取る。
		if label, _ := graph.nodes[files[0]].label.ComparableValue(); label != item.label {
			t.Errorf("the file node of %s is labelled %q, want %q", item.path, label, item.label)
		}
	}
}
