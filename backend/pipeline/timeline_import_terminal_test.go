// in-package test: 起動で指定した端末を時系列の行が持つことを確かめる。
package pipeline

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 起動で端末を指定した収集元のレコードは、その端末を時系列の行の端末に持つ。画面から収集元の
// 全体に記録した割当は、時系列の行の端末にしない。
func TestTimelineCarriesTheTerminalSpecifiedAtImport(t *testing.T) {
	document := "<Events>\n" +
		processCreationXML("", "2001-02-03T04:05:06Z", "701", "0x10", "0x1", `C:\Example\a.exe`, "") +
		"</Events>\n"
	result, err := memoryRunner(t, map[string]string{"ws.xml": document}).Run([]SourcePlan{{
		OriginPath: "ws.xml", FileName: "ws.xml", FormatKey: WindowsEventXMLFormatKey,
		Terminal: &SourceTerminal{TerminalId: "ws-01", TerminalHostname: "ws-01"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := core.TerminalNodeKey("ws-01")
	entries := NewGraph(result, AllMatchConditions()).Timeline(TimelineQuery{}).Entries
	if len(entries) != 1 || entries[0].Terminal == nil || entries[0].Terminal.Id != nodeIdOf(want) {
		t.Fatalf("entries = %+v, want 1 entry on the terminal %q", entries, nodeIdOf(want))
	}

	plain, err := memoryRunner(t, map[string]string{"ws.xml": document}).Run([]SourcePlan{{
		OriginPath: "ws.xml", FileName: "ws.xml", FormatKey: WindowsEventXMLFormatKey,
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, specified := range result.ImportSpecifiedTerminalAssignments() {
		specified.Origin = core.TerminalAssignmentOriginAnalystSupplied
		specified.Derivation, specified.Author = "合成の筋道", "analyst"
		specified.SourceId = plain.publications[0].status.SourceId
		specified.AppliesToSourceId = specified.SourceId
		specified.BasisRecordRefs = []core.AssertionRecordRef{
			core.NewAssertionRecordRef(plain.publications[0].records[0].Locator)}
		supplied := plain.WithAnalystTerminalAssignments([]core.TerminalAssignment{specified})
		analystEntries := NewGraph(supplied, AllMatchConditions()).Timeline(TimelineQuery{}).Entries
		if len(analystEntries) != 1 || analystEntries[0].Terminal != nil {
			t.Errorf("entries = %+v, want 1 entry without the terminal of the analyst assignment", analystEntries)
		}
	}
}
