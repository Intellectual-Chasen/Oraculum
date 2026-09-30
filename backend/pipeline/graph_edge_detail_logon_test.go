// in-package test: 非公開の構築子で作った取り込み結果からグラフを組んで検査する。
package pipeline

import (
	"strconv"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// graph-markii-logon-types.log は、端末 T1 (192.0.2.1) から端末 T2 (192.0.2.9) への遠隔の
// セッションを、ログオンの種別の異なる根拠で記録する。
//
// 303・304 はネットワーク (3)、305 はリモート対話 (10) のログオンである。306 は logonType が
// 「名前(数字)」の形でないログオンであり、種別のコードを導けない。307 は logonType の key を
// 持たない遠隔ログインである。
func logonTypesDetail(t *testing.T, filter EdgeEvidenceFilter) EdgeDetail {
	t.Helper()
	result, err := graphRunner(t).Run([]SourcePlan{{
		OriginPath: "testdata/graph-markii-logon-types.log",
		FileName:   "graph-markii-logon-types.log",
		FormatKey:  MarkIIFormatKey,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return sessionMeansDetail(t, NewGraph(result, AllMatchConditions()), filter)
}

// logonGroupLabel は区分 1 つを、観測の種別とログオンの種別の状態の文字列で表す。
func logonGroupLabel(t *testing.T, group core.EdgeEvidenceGroup) string {
	t.Helper()
	kind := groupLabel(t, group)
	kind = kind[:len(kind)-len(groupPortLabel(t, group))]
	if group.LogonType == nil {
		switch group.LogonTypeAbsence {
		case logonTypeAbsentReason:
			return kind + "logon_absent"
		case logonTypeUnreadableReason:
			return kind + "logon_unreadable"
		}
		t.Fatalf("a group without a logon type carries the reason %q, want one of the two "+
			"reasons", group.LogonTypeAbsence)
	}
	value, readable := comparableTextOf(*group.LogonType)
	if !readable {
		t.Fatal("a group carries a logon type it cannot compare")
	}
	return kind + "logon=" + value
}

// sequenceNumbersOf は根拠のレコード番号を並べる。
func sequenceNumbersOf(t *testing.T, evidence []core.GraphEvidence) []string {
	t.Helper()
	numbers := make([]string, 0, len(evidence))
	for _, item := range evidence {
		if item.RecordRef.SequenceNumber == nil {
			t.Fatal("an evidence record carries no sequence number")
		}
		numbers = append(numbers, strconv.FormatInt(*item.RecordRef.SequenceNumber, 10))
	}
	return numbers
}

// 1 本の遠隔のセッションの根拠が、ログオンの種別ごとの区分に分かれる。種別を持たない区分と、
// 種別を導けなかった区分は、別の区分として別の理由を持つ。
func TestEdgeEvidenceGroupsSplitByTheLogonType(t *testing.T) {
	detail := logonTypesDetail(t, EdgeEvidenceFilter{})
	want := map[string]int64{
		"os/evtLog logon=3":           2,
		"os/evtLog logon=10":          1,
		"os/evtLog logon_unreadable":  1,
		"session/loginR logon_absent": 1,
	}
	got := make(map[string]int64, len(detail.EvidenceGroups))
	for _, group := range detail.EvidenceGroups {
		if err := group.Validate(); err != nil {
			t.Errorf("the group %q = %v, want a valid group", logonGroupLabel(t, group), err)
		}
		got[logonGroupLabel(t, group)] = group.EvidenceCount
	}
	for label, count := range want {
		if got[label] != count {
			t.Errorf("the group %q carries %d evidence records, want %d", label, got[label], count)
		}
	}
	for label := range got {
		if _, expected := want[label]; !expected {
			t.Errorf("the groups carry %q, want only the groups of the fixture", label)
		}
	}
}

// 種別を持たない区分と値を持つ区分は要求で指せ、指した区分の根拠だけが返る。種別を導けな
// かった区分は指せない理由を持つ。
func TestEdgeEvidenceGroupOfALogonTypeNarrowsTheEvidence(t *testing.T) {
	detail := logonTypesDetail(t, EdgeEvidenceFilter{})
	selectable := 0
	for _, group := range detail.EvidenceGroups {
		label := logonGroupLabel(t, group)
		if label == "os/evtLog logon_unreadable" {
			if group.Selector != nil || group.SelectorAbsence != logonTypeUnreadableSelectorReason {
				t.Errorf("the group %q carries %+v %q, want the reason it cannot be named",
					label, group.Selector, group.SelectorAbsence)
			}
			continue
		}
		if group.Selector == nil {
			t.Fatalf("the group %q carries no selector (%q)", label, group.SelectorAbsence)
		}
		selectable++
		narrowed := logonTypesDetail(t, EdgeEvidenceFilterOf(*group.Selector))
		if narrowed.Edge.EvidenceCount != group.EvidenceCount {
			t.Errorf("the selector of %q returned %d evidence records, want %d",
				label, narrowed.Edge.EvidenceCount, group.EvidenceCount)
		}
	}
	if selectable == 0 {
		t.Error("the groups carry no selectable group")
	}
}

// 種別の値の条件は種別を持たないレコードを通さず、種別の不在を指す条件は種別を導けなかった
// レコードを通さない。
func TestEdgeDetailNarrowsByTheLogonType(t *testing.T) {
	for name, tc := range map[string]struct {
		filter EdgeEvidenceFilter
		want   []string
	}{
		"ネットワーク":    {EdgeEvidenceFilter{LogonType: "3"}, []string{"303", "304"}},
		"リモート対話":    {EdgeEvidenceFilter{LogonType: "10"}, []string{"305"}},
		"種別の値が無い":   {EdgeEvidenceFilter{LogonTypeAbsent: true}, []string{"307"}},
		"条件を与えない要求": {EdgeEvidenceFilter{}, []string{"303", "304", "305", "306", "307"}},
	} {
		t.Run(name, func(t *testing.T) {
			detail := logonTypesDetail(t, tc.filter)
			requireSameStrings(t, "the narrowed evidence",
				sequenceNumbersOf(t, detail.Evidence), tc.want...)
		})
	}
}

// 値の不在を表す文字列を持つログオンの種別は「種別の値が無い」の区分に入る。種別の key を
// 持たないレコードと同じ区分である。接続先 port の同じ文字列は、欄はあるが比べられない区分に
// 入る。
func TestTheAbsentValueOfTheLogonTypeJoinsTheGroupWithoutTheValue(t *testing.T) {
	value, err := core.NewRawValue(core.ValueStateAbsent, "-")
	if err != nil {
		t.Fatal(err)
	}
	dash, err := core.NewTextField("EventData.LogonType", core.SemanticKeyEventLogonType, value)
	if err != nil {
		t.Fatal(err)
	}
	port := dash
	port.Semantic = core.SemanticKeyConnectionDestinationPort
	graph := Graph{records: []graphRecord{
		{eventCategory: "os", eventAction: "evtLog", logonType: &dash, destinationPort: &port},
		{eventCategory: "os", eventAction: "evtLog", destinationPort: &port},
	}}
	for at := range graph.records {
		key := graph.evidenceGroupKeyOf(at)
		if key.logonTypeState != carriedFieldAbsent {
			t.Errorf("the record %d falls in the logon type state %d, want the absent one",
				at, key.logonTypeState)
		}
		if key.portState != carriedUnreadable {
			t.Errorf("the record %d falls in the port state %d, want the unreadable one",
				at, key.portState)
		}
		if !graph.recordInGroup(at, EdgeEvidenceFilter{LogonTypeAbsent: true}) {
			t.Errorf("the record %d does not pass the filter of the absent logon type", at)
		}
	}
	if graph.evidenceGroupKeyOf(0) != graph.evidenceGroupKeyOf(1) {
		t.Error("the two records fall in two groups, want one group")
	}
}
