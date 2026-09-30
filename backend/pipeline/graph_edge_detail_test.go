// in-package test: 非公開の構築子で作った取り込み結果からグラフを組んで検査する。
package pipeline

import (
	"strconv"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// graph-markii-session-means.log は、端末 T1 (192.0.2.1) から端末 T2 (192.0.2.9) への
// 遠隔のセッションを、用いた手段の異なる根拠で記録する。
//
// 3・4 行目は接続先 port 5985 への acpt、5 行目は 445 への acpt である。
// 6・7 行目は alice の遠隔ログイン、8 行目は bob の遠隔ログイン、9 行目は Windows イベントで
// あり、3 行はいずれも接続先 port の欄を持たない。
// 10 行目は dstPort の欄に値の不在の文字列を持つ acpt、11 行目は dstPort の key そのものを
// 持たない acpt であり、2 行が「欄はあるが値を比べられない」と「欄が無い」を分ける。
// 12 行目は dstPort が空の文字列である acpt であり、比べられる値が空である区分を作る。
func sessionMeansGraph(t *testing.T) Graph {
	t.Helper()
	runner := graphRunner(t)
	result, err := runner.Run([]SourcePlan{{
		OriginPath: "testdata/graph-markii-session-means.log",
		FileName:   "graph-markii-session-means.log",
		FormatKey:  MarkIIFormatKey,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return NewGraph(result, AllMatchConditions())
}

// sessionMeansEdgeId は遠隔のセッションのエッジ 1 本の識別子を返す。
func sessionMeansEdgeId(t *testing.T, graph Graph) string {
	t.Helper()
	edges := terminalSessionEdges(t, graph)
	if len(edges) != 1 {
		t.Fatalf("the graph carries %d remote session relations, want 1", len(edges))
	}
	return edges[0].Id
}

// sessionMeansDetail は遠隔のセッションのエッジ 1 本の詳細を返す。
func sessionMeansDetail(t *testing.T, graph Graph, filter EdgeEvidenceFilter) EdgeDetail {
	t.Helper()
	id := sessionMeansEdgeId(t, graph)
	detail, found := graph.EdgeDetail(id, filter)
	if !found {
		t.Fatalf("the relation %q has no detail", id)
	}
	return detail
}

// groupLabel は区分 1 つを、観測の種別と接続先 port の状態の文字列で表す。
func groupLabel(t *testing.T, group core.EdgeEvidenceGroup) string {
	t.Helper()
	category, action := "", ""
	for _, field := range group.ObservationKind.Raw {
		value, readable := comparableTextOf(field)
		if !readable {
			continue
		}
		switch field.Semantic {
		case core.SemanticKeyEventCategory:
			category = value
		case core.SemanticKeyEventAction:
			action = value
		}
	}
	return category + "/" + action + " " + groupPortLabel(t, group)
}

// groupPortLabel は区分の接続先 port の状態を文字列で表す。
// 値を持つ区分はその値、空の文字列である区分は port_empty、値を持たない区分は持たない理由の
// 区別を返す。
func groupPortLabel(t *testing.T, group core.EdgeEvidenceGroup) string {
	t.Helper()
	if group.DestinationPort == nil {
		switch group.DestinationPortAbsence {
		case destinationPortAbsentReason:
			return "port_field_absent"
		case destinationPortUnreadableReason:
			return "port_unreadable"
		}
		t.Fatalf("a group without a destination port carries the reason %q, want one of the "+
			"two reasons", group.DestinationPortAbsence)
	}
	value, readable := comparableTextOf(*group.DestinationPort)
	if !readable {
		t.Fatal("a group carries a destination port it cannot compare")
	}
	if value == "" {
		return "port_empty"
	}
	return value
}

// comparableTextOf は項目の比べられる値を返す。
func comparableTextOf(field core.RecordField) (string, bool) {
	if field.Text == nil {
		return "", false
	}
	return field.Text.ComparableValue()
}

// accountLabelsOf は区分に現れたアカウントを、表示名と件数の文字列で返す。
func accountLabelsOf(t *testing.T, group core.EdgeEvidenceGroup) []string {
	t.Helper()
	labels := make([]string, 0, len(group.Accounts))
	for _, account := range group.Accounts {
		value, readable := account.Node.Label.ComparableValue()
		if !readable {
			t.Fatal("an account node carries a label it cannot compare")
		}
		labels = append(labels, value+"="+strconv.FormatInt(account.EvidenceCount, 10))
	}
	return labels
}

// groupCountsOf は区分を「観測の種別 接続先 port」の文字列で探せる表にする。
func groupCountsOf(t *testing.T, groups []core.EdgeEvidenceGroup) map[string]int64 {
	t.Helper()
	counts := make(map[string]int64, len(groups))
	for _, group := range groups {
		label := groupLabel(t, group)
		if _, taken := counts[label]; taken {
			t.Errorf("the groups carry %q twice, want one group per observation kind and port",
				label)
		}
		counts[label] = group.EvidenceCount
	}
	return counts
}

// sessionMeansGroupCounts は fixture が作る区分と、その根拠の件数である。
func sessionMeansGroupCounts() map[string]int64 {
	return map[string]int64{
		"net/acpt 5985":                    2,
		"net/acpt 445":                     1,
		"net/acpt port_unreadable":         1,
		"net/acpt port_field_absent":       1,
		"net/acpt port_empty":              1,
		"session/loginR port_field_absent": 3,
		"os/evtLog port_field_absent":      1,
	}
}

// requireGroupCounts は区分の一覧が期待の表と両方向で一致することを確かめる。
func requireGroupCounts(t *testing.T, groups []core.EdgeEvidenceGroup, want map[string]int64) {
	t.Helper()
	counts := groupCountsOf(t, groups)
	for label, expected := range want {
		if counts[label] != expected {
			t.Errorf("the group %q carries %d evidence records, want %d",
				label, counts[label], expected)
		}
	}
	for label := range counts {
		if _, expected := want[label]; !expected {
			t.Errorf("the groups carry %q, want only the groups of the fixture", label)
		}
	}
}

// 1 本の遠隔のセッションの根拠が、観測の種別と接続先 port ごとの区分に分かれる。
func TestEdgeEvidenceGroupsSplitByTheObservationKindAndTheDestinationPort(t *testing.T) {
	detail := sessionMeansDetail(t, sessionMeansGraph(t), EdgeEvidenceFilter{})
	requireGroupCounts(t, detail.EvidenceGroups, sessionMeansGroupCounts())

	// **区分の件数の和は、エッジの根拠の総数に等しい。** 1 件も除かれない。
	var summed int64
	for _, group := range detail.EvidenceGroups {
		summed += group.EvidenceCount
	}
	if summed != detail.Edge.EvidenceCount {
		t.Errorf("the groups carry %d evidence records in total, want the edge count %d",
			summed, detail.Edge.EvidenceCount)
	}
}

// **エッジの詳細は根拠を全件返す。** fixture が遠隔のセッションの根拠にするレコードを、
// レコード番号で指して確かめる。
func TestEdgeDetailReturnsEveryEvidenceRecord(t *testing.T) {
	detail := sessionMeansDetail(t, sessionMeansGraph(t), EdgeEvidenceFilter{})

	sequenceNumbers := make([]string, 0, len(detail.Evidence))
	for _, evidence := range detail.Evidence {
		if evidence.RecordRef.SequenceNumber == nil {
			t.Fatal("an evidence record carries no sequence number")
		}
		sequenceNumbers = append(sequenceNumbers,
			strconv.FormatInt(*evidence.RecordRef.SequenceNumber, 10))
	}
	requireSameStrings(t, "the evidence of the remote session", sequenceNumbers,
		"203", "204", "205", "206", "207", "208", "209", "210", "211", "212")

	// 返した件数と、エッジが述べる根拠の総数は一致する。
	if int64(len(detail.Evidence)) != detail.Edge.EvidenceCount {
		t.Errorf("the detail returned %d evidence records, want the edge count %d",
			len(detail.Evidence), detail.Edge.EvidenceCount)
	}
}

// 接続先 port の欄を持たない区分と、欄はあるが値を比べられない区分は、別の区分になり、
// それぞれの理由を持つ。
func TestEdgeEvidenceGroupSeparatesTheAbsentFieldFromTheUnreadableValue(t *testing.T) {
	detail := sessionMeansDetail(t, sessionMeansGraph(t), EdgeEvidenceFilter{})

	reasons := make(map[string]string, len(detail.EvidenceGroups))
	for _, group := range detail.EvidenceGroups {
		if err := group.Validate(); err != nil {
			t.Errorf("the group %q = %v, want a valid group", groupLabel(t, group), err)
		}
		reasons[groupLabel(t, group)] = group.DestinationPortAbsence
	}
	if reasons["net/acpt port_field_absent"] != destinationPortAbsentReason {
		t.Errorf("the group of the absent field carries the reason %q, want %q",
			reasons["net/acpt port_field_absent"], destinationPortAbsentReason)
	}
	// 反対側。欄はあるが値を比べられない区分は、別の理由を持つ。
	if reasons["net/acpt port_unreadable"] != destinationPortUnreadableReason {
		t.Errorf("the group of the unreadable value carries the reason %q, want %q",
			reasons["net/acpt port_unreadable"], destinationPortUnreadableReason)
	}
	if reasons["net/acpt 5985"] != "" {
		t.Errorf("the group carrying a port also carries the reason %q, want no reason",
			reasons["net/acpt 5985"])
	}
}

// 区分を要求で指す値を応答が運び、指せない区分は指せない理由を持つ。
func TestEdgeEvidenceGroupCarriesTheSelectorOrTheReasonItIsAbsent(t *testing.T) {
	graph := sessionMeansGraph(t)
	detail := sessionMeansDetail(t, graph, EdgeEvidenceFilter{})

	// 指せない区分と、その区分が出す理由。区分ごとに固有の理由を出す。
	unselectableReasons := map[string]string{
		"net/acpt port_unreadable": destinationPortUnreadableSelectorReason,
		"net/acpt port_empty":      destinationPortEmptySelectorReason,
	}

	selectable, unselectable := 0, 0
	for _, group := range detail.EvidenceGroups {
		label := groupLabel(t, group)
		if want, named := unselectableReasons[label]; named {
			unselectable++
			if group.Selector != nil {
				t.Errorf("the group %q carries a selector, want the reason it cannot be named",
					label)
			}
			if group.SelectorAbsence != want {
				t.Errorf("the group %q carries the reason %q, want %q",
					label, group.SelectorAbsence, want)
			}
			continue
		}
		// 反対側。値を持つ区分と、欄を持たない区分は指せる。
		selectable++
		if group.Selector == nil {
			t.Fatalf("the group %q carries no selector, want the group to be selectable", label)
		}
		narrowed := sessionMeansDetail(t, graph, EdgeEvidenceFilterOf(*group.Selector))
		if narrowed.Edge.EvidenceCount != group.EvidenceCount {
			t.Errorf("the selector of %q returned %d evidence records, want the group count %d",
				label, narrowed.Edge.EvidenceCount, group.EvidenceCount)
		}
	}
	if selectable == 0 {
		t.Error("the groups carry no selectable group, want both sides")
	}
	if unselectable != len(unselectableReasons) {
		t.Errorf("the groups carry %d unselectable, want the %d of the fixture",
			unselectable, len(unselectableReasons))
	}
}

// 接続先 port の値が空の文字列である区分は、要求で指せない固有の理由を持つ。
//
// **欄の不在を述べる理由を出さない。** 空の文字列を持つ区分は接続先 port の欄を持ち、
// 応答も欄を含む。
func TestEdgeEvidenceGroupOfAnEmptyDestinationPortCarriesItsOwnReason(t *testing.T) {
	detail := sessionMeansDetail(t, sessionMeansGraph(t), EdgeEvidenceFilter{})

	found := false
	for _, group := range detail.EvidenceGroups {
		if groupLabel(t, group) != "net/acpt port_empty" {
			continue
		}
		found = true
		if err := group.Validate(); err != nil {
			t.Errorf("the group of the empty destination port = %v, want a valid group", err)
		}
		if group.Selector != nil {
			t.Error("the group of the empty destination port carries a selector, want the " +
				"reason it cannot be named")
		}
		if group.SelectorAbsence != destinationPortEmptySelectorReason {
			t.Errorf("the group of the empty destination port carries the reason %q, want %q",
				group.SelectorAbsence, destinationPortEmptySelectorReason)
		}
		// 欄そのものは応答が含む。欄の不在の理由を出さない。
		if group.DestinationPort == nil {
			t.Error("the group of the empty destination port carries no destination port, " +
				"want the field the record carries")
		}
		if group.DestinationPortAbsence != "" {
			t.Errorf("the group of the empty destination port carries the absence reason %q, "+
				"want no reason", group.DestinationPortAbsence)
		}
	}
	if !found {
		t.Fatal("the fixture carries no group of an empty destination port")
	}
}

// squidGraph は観測の種別を持たない収集元だけを取り込んだグラフである。
//
// Squid の combined は event.category と event.action の語彙の項目を 1 つも割り当てない。
func squidGraph(t *testing.T) Graph {
	t.Helper()
	runner := graphRunner(t)
	result, err := runner.Run([]SourcePlan{{
		OriginPath: "testdata/graph-squid.log",
		FileName:   "graph-squid.log",
		FormatKey:  SquidFormatKey,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return NewGraph(result, AllMatchConditions())
}

// 観測の種別を持たない収集元のエッジは、HTTP の要求の区分を除いて要求で指せず、読めない
// 理由を持つ。HTTP の要求の区分は HTTP の状態で指せる。
func TestEdgeEvidenceGroupsOfASourceWithoutAnObservationKindCannotBeNamed(t *testing.T) {
	graph := squidGraph(t)

	unselectable, httpSelectable := 0, 0
	for _, edge := range graph.edges {
		detail, found := graph.EdgeDetail(edge.id, EdgeEvidenceFilter{})
		if !found {
			t.Fatalf("the relation %q has no detail", edge.id)
		}
		for _, group := range detail.EvidenceGroups {
			if err := group.Validate(); err != nil {
				t.Errorf("a group of the relation %q = %v, want a valid group", edge.id, err)
			}
			isHttp := group.HttpStatus != nil || group.HttpStatusAbsence != ""
			if group.Selector != nil {
				if !isHttp {
					t.Errorf("a group of the relation %q outside HTTP carries a selector", edge.id)
				}
				httpSelectable++
				continue
			}
			unselectable++
			if !isHttp && group.SelectorAbsence != observationKindAbsentReason {
				t.Errorf("a group of the relation %q carries the reason %q, want %q",
					edge.id, group.SelectorAbsence, observationKindAbsentReason)
			}
		}
	}
	if unselectable == 0 || httpSelectable == 0 {
		t.Fatalf("the squid graph carries %d unselectable groups and %d HTTP groups to name, want both",
			unselectable, httpSelectable)
	}
	means := sessionMeansDetail(t, sessionMeansGraph(t), EdgeEvidenceFilter{})
	named := 0
	for _, group := range means.EvidenceGroups {
		if group.Selector != nil {
			named++
		}
	}
	if named == 0 {
		t.Error("the mark II fixture carries no selectable group, want both sides")
	}
}

// 遠隔ログインの区分に現れたアカウントを、関係の詳細から辿れる。
func TestEdgeEvidenceGroupNamesTheAccountsOfItsRecords(t *testing.T) {
	graph := sessionMeansGraph(t)
	detail := sessionMeansDetail(t, graph, EdgeEvidenceFilter{})
	named := make(map[string][]string, len(detail.EvidenceGroups))
	for _, group := range detail.EvidenceGroups {
		named[groupLabel(t, group)] = accountLabelsOf(t, group)
	}
	requireSameStrings(t, "the accounts of session/loginR",
		named["session/loginR port_field_absent"], "alice=2", "bob=1")
	// 条件の両側。アカウントを指さないレコードの区分は要素を 1 つも持たない。
	if len(named["net/acpt 5985"]) != 0 {
		t.Errorf("the group of net/acpt 5985 names %v, want no account",
			named["net/acpt 5985"])
	}
	// **アカウントのノードへ識別子で辿れる。** グラフが同じ識別子のノードを持つ。
	for _, group := range detail.EvidenceGroups {
		for _, account := range group.Accounts {
			if _, present := graph.nodeAt[account.Node.Id]; !present {
				t.Errorf("the account node %q is absent from the graph", account.Node.Id)
			}
			if account.Node.Kind != core.NodeKindAccount {
				t.Errorf("the named node carries the kind %q, want account", account.Node.Kind)
			}
		}
	}
}

// 区分を指した要求は、その区分の根拠だけを返し、区分の一覧は全数から組む。
func TestEdgeDetailNarrowsTheEvidenceToTheRequestedGroup(t *testing.T) {
	graph := sessionMeansGraph(t)
	whole := sessionMeansDetail(t, graph, EdgeEvidenceFilter{})
	narrowed := sessionMeansDetail(t, graph, EdgeEvidenceFilter{
		EventCategory: "net", EventAction: "acpt", DestinationPort: "5985",
	})
	if narrowed.Edge.EvidenceCount != 2 {
		t.Errorf("the narrowed detail carries %d evidence records, want 2",
			narrowed.Edge.EvidenceCount)
	}
	if int64(len(narrowed.Evidence)) != narrowed.Edge.EvidenceCount {
		t.Errorf("the narrowed detail returned %d evidence records, want the count %d",
			len(narrowed.Evidence), narrowed.Edge.EvidenceCount)
	}
	if narrowed.Edge.EvidenceCount >= whole.Edge.EvidenceCount {
		t.Errorf("the narrowed count %d is not below the whole count %d",
			narrowed.Edge.EvidenceCount, whole.Edge.EvidenceCount)
	}
	// **区分の一覧は絞り込みで変わらない。** 他の区分が何件あるかを同じ応答で読める。
	requireGroupCounts(t, narrowed.EvidenceGroups, sessionMeansGroupCounts())
	sequenceNumbers := make([]string, 0, len(narrowed.Evidence))
	for _, evidence := range narrowed.Evidence {
		if evidence.RecordRef.SequenceNumber == nil {
			t.Fatal("an evidence record carries no sequence number")
		}
		sequenceNumbers = append(sequenceNumbers,
			strconv.FormatInt(*evidence.RecordRef.SequenceNumber, 10))
	}
	requireSameStrings(t, "the narrowed evidence", sequenceNumbers, "203", "204")
}

// 接続先 port の条件は、接続先 port の欄を持たないレコードを通さない。
func TestEdgeDetailKeepsTheRecordsWithoutADestinationPortOutOfThePortGroup(t *testing.T) {
	graph := sessionMeansGraph(t)
	byPort := sessionMeansDetail(t, graph, EdgeEvidenceFilter{DestinationPort: "445"})
	if byPort.Edge.EvidenceCount != 1 {
		t.Errorf("the detail narrowed to the port 445 carries %d evidence records, want 1",
			byPort.Edge.EvidenceCount)
	}
	// 反対側。観測の種別だけで絞ると、接続先 port を持たないレコードが通る。
	byKind := sessionMeansDetail(t, graph, EdgeEvidenceFilter{
		EventCategory: "session", EventAction: "loginR",
	})
	if byKind.Edge.EvidenceCount != 3 {
		t.Errorf("the detail narrowed to session/loginR carries %d evidence records, want 3",
			byKind.Edge.EvidenceCount)
	}
	for _, evidence := range byKind.Evidence {
		if evidence.EventTime == nil {
			t.Error("an evidence record of session/loginR carries no event time")
		}
	}
}

// 接続先 port の欄の不在を指す条件は、欄を持たないレコードだけを通す。
//
// **欄はあるが値を比べられないレコードを通さない。** 2 つを 1 つの条件で通すと、
// 区分が分けた集団を絞り込みが混ぜ直す。
func TestEdgeDetailNarrowsToTheRecordsWithoutTheDestinationPortField(t *testing.T) {
	graph := sessionMeansGraph(t)
	narrowed := sessionMeansDetail(t, graph, EdgeEvidenceFilter{
		EventCategory: "net", EventAction: "acpt", DestinationPortAbsent: true,
	})
	if narrowed.Edge.EvidenceCount != 1 {
		t.Fatalf("the detail narrowed to the absent field carries %d evidence records, want 1",
			narrowed.Edge.EvidenceCount)
	}
	if narrowed.Evidence[0].RecordRef.SequenceNumber == nil ||
		*narrowed.Evidence[0].RecordRef.SequenceNumber != 211 {
		t.Errorf("the detail narrowed to the absent field returned %+v, want the record 211",
			narrowed.Evidence[0].RecordRef.SequenceNumber)
	}
	// 反対側。値を持つ条件は、欄を持たないレコードを通さない。
	byValue := sessionMeansDetail(t, graph, EdgeEvidenceFilter{
		EventCategory: "net", EventAction: "acpt", DestinationPort: "445",
	})
	if byValue.Edge.EvidenceCount != 1 {
		t.Errorf("the detail narrowed to the port 445 carries %d evidence records, want 1",
			byValue.Edge.EvidenceCount)
	}
}
