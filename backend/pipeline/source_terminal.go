package pipeline

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 利用者が与えた端末を持つ項目の名前。原資料の key ではないため、導出であることが
// 分かる名前にする。
const (
	analystTerminalIdFieldName       = "analystTerminal"
	analystTerminalHostnameFieldName = "analystTerminalName"
	analystTerminalIpFieldName       = "analystTerminalIp"
	// importTerminalIdFieldName は、取り込みの起動で指定した端末の外部識別子を持つ。
	// 時系列の行の端末はこの項目を読み、分析者の割当の項目を読まない。
	importTerminalIdFieldName = "importTerminal"
)

// unknownTerminalDerivation は名前不明の端末の表示名の導出の説明である。
const unknownTerminalDerivation = "収集元の file 名。利用者が端末を指定していない収集元の端末"

// sourceTerminal は、端末を名乗らないレコードを置く、収集元 1 件の端末である。
//
// **1 つのログのファイルは 1 台の端末が書いたものとして扱う。** 端末が分からない
// 収集元は、名前不明の端末でつなぐ。
type sourceTerminal struct {
	// fields は、端末の外部識別子を持たないレコードへ足す端末の項目である。値の状態は
	// derived である。端末を指定していない収集元では出ない。
	fields []core.RecordField
	// scope はレコードを置く端末の範囲である。
	scope core.RecordScope
	// unknownLabel は名前不明の端末の表示名である。利用者が表示名を与えた端末では出ない。
	unknownLabel *core.RawAndNormalized
	// assignedLabels は、利用者が割当に書いた端末の表示名 (derived) である。表示名を持つ
	// 割当 1 件につき 1 つある。割当が表示名を持たない収集元では空である。
	assignedLabels []core.RawAndNormalized
	// assignedTerminal は、割当が指す端末のノードの識別鍵である。assignments と同時に出る。
	assignedTerminal core.NodeKey
	// assignments は、収集元を 1 台の端末に置いた割当である。割当で端末を決めていない
	// 収集元では空である。
	assignments []core.TerminalAssignment
	// hostScopedContentSha256 は、レコードが名乗ったホスト名ごとに名前不明の端末を分ける
	// 収集元の内容の識別である。分けない収集元では空である
	// (ParserIdentity の RecordingTerminalPerHostname)。
	hostScopedContentSha256 string
	// namedAssignments は、ホスト名ごとに分ける収集元のレコードが名乗ったホスト名と比べる、
	// 同じ案件の利用者の割当である (namedAssignmentsOf)。分けない収集元では空である。
	namedAssignments []namedAssignment
	// collection は、収集の directory から取り出した収集元の、収集の端末である。利用者の割当で
	// 端末を決めた収集元と、収集の directory から取り出していない収集元では nil である。
	collection *collectionTerminal
}

// sourceTerminals は収集元の sourceId から、その収集元の端末を探す表である。
type sourceTerminals map[string]sourceTerminal

// sourceTerminalsOf は、公開中の収集元ごとに、端末を名乗らないレコードを置く端末を組む。
//
// 利用者が収集元に付けた割当 (取り込みの指定と、画面から記録した割当) が 1 台の端末を
// 指す収集元は、その端末に置く (claimedTerminalOf)。**同じ収集元の割当が 2 台以上の端末を
// 指すときは、どれにも決めない。** 先に見つけた 1 件を選ばず、割当の無い収集元と同じく
// 名前不明の端末に置く。
func sourceTerminalsOf(result ImportResult) sourceTerminals {
	claimed := make(map[string][]core.TerminalAssignment)
	for _, assignment := range result.userAssignments() {
		if assignment.AppliesToSourceId == "" || assignment.Validate() != nil {
			continue
		}
		claimed[assignment.AppliesToSourceId] = append(
			claimed[assignment.AppliesToSourceId], assignment)
	}
	terminals := make(sourceTerminals, len(result.publications))
	hostNamed := namedAssignmentsOf(result)
	collections := collectionTerminalsOf(result)
	for _, publication := range result.publications {
		if publication.status.PublicationState == core.PublicationStateWithheld ||
			len(publication.records) == 0 {
			continue
		}
		sourceId := publication.status.SourceId
		accountsLocal := publication.parser.AccountNamesLocalToTerminal
		// 収集元の内容の識別と file 名は、レコードの位置が持つ値と同じである
		// (RecordLocator の SourceContentSha256 と SourceFileName)。
		locator := publication.records[0].Locator
		unknown, unknownBuilt := unknownTerminalOf(
			locator.SourceContentSha256, locator.SourceFileName, accountsLocal)
		if terminal, named, built := claimedTerminalOf(claimed[sourceId], accountsLocal); built {
			// 表示名を与えない指定の端末は、名前不明の端末と同じ表示名を持つ。
			if !named {
				terminal.unknownLabel = unknown.unknownLabel
			}
			terminals[sourceId] = terminal
			continue
		}
		if collection := collections[result.identities[sourceId].CollectionPath]; collection != nil {
			if terminal, built := collectionSourceTerminalOf(collection, publication.parser, hostNamed); built {
				terminals[sourceId] = terminal
				continue
			}
		}
		// **利用者が端末を指定していない収集元は、1 台の端末が書く形式だけを名前不明の
		// 端末に置く** (ParserIdentity の RecordedByOneTerminal)。
		if unknownBuilt && publication.parser.RecordedByOneTerminal {
			terminals[sourceId] = unknown
			continue
		}
		// **ホスト名ごとに分ける形式は、ホスト名 1 つにつき名前不明の端末を 1 台置く。**
		// 1 つの file が複数台の記録を持ちうるため、1 台に置くと別の端末のプロセスが同じ
		// 端末で動いた関係と親子の候補を持つ。
		if publication.parser.RecordingTerminalPerHostname {
			terminals[sourceId] = sourceTerminal{
				scope:                   core.RecordScope{AccountNamesLocal: accountsLocal},
				hostScopedContentSha256: locator.SourceContentSha256,
				namedAssignments:        hostNamed,
			}
		}
	}
	return terminals
}

// namedAssignment は、ホスト名を記録した利用者の割当 1 件と、比べるために前もって読んだ
// 端末の鍵と期間の両端の時点である。
//
// **レコードごとに期間の文字列を読み直さない。** 端末を決める判定は、ホスト名を名乗るレコード
// 1 件につき呼び出し元ごとに走る。
type namedAssignment struct {
	assignment core.TerminalAssignment
	terminal   core.NodeKey
	from, to   time.Time
}

// covers は、割当が hostname を名前に記録し、at が期間の中 (両端を含む) にあるかを返す。
//
// **比較の単位は秒である** (core.TimeRange.Contains と同じ)。from と to は秒に切り捨てて
// 持ち、at も秒に切り捨てて比べる。境界の秒に並ぶ秒未満の時刻を期間の中に入れる。
func (n namedAssignment) covers(hostname string, at time.Time) bool {
	second := secondOf(at)
	return !second.Before(n.from) && !second.After(n.to) && n.assignment.NamesHostname(hostname)
}

// secondOf は時点を秒に切り捨てる。
func secondOf(at time.Time) time.Time {
	return time.Unix(at.Unix(), 0).UTC()
}

// namedAssignmentsOf は、利用者が入力した割当のうち、ホスト名 (表示名か TerminalHostnames) を
// 記録し、端末のノードを指し、期間の両端を時点として読める割当を、userAssignments の並びで
// 返す。期間は、期間を読み取った収集元の今の時刻の解釈で読む (withInterpretedRange)。
// 両端を読めない割当は、どのレコードの時刻とも比べられない。
func namedAssignmentsOf(result ImportResult) []namedAssignment {
	var named []namedAssignment
	for _, assignment := range result.userAssignments() {
		if assignment.TerminalHostname == "" && len(assignment.TerminalHostnames) == 0 {
			continue
		}
		key, built := assignment.TerminalNodeKey()
		if !built || assignment.Validate() != nil {
			continue
		}
		assignment = result.withInterpretedRange(assignment)
		from, fromReadable := assignment.AssignmentValidRange.From.Instant()
		to, toReadable := assignment.AssignmentValidRange.To.Instant()
		if !fromReadable || !toReadable {
			continue
		}
		named = append(named, namedAssignment{
			assignment: assignment, terminal: key, from: secondOf(from), to: secondOf(to),
		})
	}
	return named
}

// namedAssignmentsByCase は、案件ごとに絞った割当 (caseScopes) から、案件ごとの
// namedAssignmentsOf の割当を組む。
func namedAssignmentsByCase(result ImportResult, scopes []ImportResult) map[string][]namedAssignment {
	byCase := make(map[string][]namedAssignment)
	for _, scope := range scopes {
		for _, named := range namedAssignmentsOf(scope) {
			caseId := result.caseOfSource(named.assignment.SourceId)
			byCase[caseId] = append(byCase[caseId], named)
		}
	}
	return byCase
}

// assignedTerminalsNaming は、hostname を名前に記録し、at を期間に含む割当が指す端末のノードの
// 識別鍵を、重複を除いて返す。at が nil のとき、または時点として読めないときは空である。
//
// **案件は呼び出し側が絞る。** assignments は at のレコードと同じ案件の割当である。
func assignedTerminalsNaming(assignments []namedAssignment, hostname string, at *core.Timestamp) []core.NodeKey {
	if at == nil || len(assignments) == 0 {
		return nil
	}
	instant, readable := at.Instant()
	if !readable {
		return nil
	}
	return assignedTerminalsAt(assignments, hostname, instant)
}

// assignedTerminalsAt は、時点 instant で assignedTerminalsNaming と同じ端末を返す。
func assignedTerminalsAt(assignments []namedAssignment, hostname string, instant time.Time) []core.NodeKey {
	var keys []core.NodeKey
	for _, named := range assignments {
		if !named.covers(hostname, instant) {
			continue
		}
		if !slices.ContainsFunc(keys, func(held core.NodeKey) bool { return nodeIdOf(held) == nodeIdOf(named.terminal) }) {
			keys = append(keys, named.terminal)
		}
	}
	return keys
}

// claimedTerminalOf は、利用者が収集元に付けた割当から、その収集元の端末を組む。named は、
// 割当のいずれかが端末の外部識別子か表示名を与えたかである。ok が偽になるのは、割当が
// 無いとき、割当が 2 台以上の端末を指すとき (TerminalAssignment.TerminalNodeKey が異なる)、
// 端末を組めないときである。
//
// **同じ端末を指す割当は 1 台の端末である。** 1 台の端末に IP を 2 つ割り当てた収集元は、
// 割当ごとの項目をすべて足した端末に置く。
func claimedTerminalOf(
	assignments []core.TerminalAssignment, accountsLocal bool,
) (sourceTerminal, bool, bool) {
	if len(assignments) == 0 {
		return sourceTerminal{}, false, false
	}
	first, built := assignments[0].TerminalNodeKey()
	if !built {
		return sourceTerminal{}, false, false
	}
	var terminal sourceTerminal
	var labels []core.RawAndNormalized
	named := false
	for index, assignment := range assignments {
		key, keyBuilt := assignment.TerminalNodeKey()
		if !keyBuilt || nodeIdOf(key) != nodeIdOf(first) {
			return sourceTerminal{}, false, false
		}
		specified, specifiedBuilt := specifiedTerminalOf(assignment, accountsLocal)
		if !specifiedBuilt {
			return sourceTerminal{}, false, false
		}
		named = named || assignment.TerminalHostname != "" || assignment.TerminalId != ""
		if assignment.TerminalHostname != "" {
			label, err := core.NewDerivedValue(assignment.TerminalHostname,
				assignmentFieldDerivation(assignment)+assignedLabelDerivationSuffix)
			if err != nil {
				return sourceTerminal{}, false, false
			}
			labels = append(labels, label)
		}
		if index == 0 {
			terminal = specified
			continue
		}
		terminal.fields = appendMissingFields(terminal.fields, specified.fields)
	}
	terminal.assignedLabels, terminal.assignedTerminal, terminal.assignments = labels, first, assignments
	return terminal, named, true
}

// appendMissingFields は、added のうち held に同じ名前と値の項目が無いものを held へ足す。
func appendMissingFields(held, added []core.RecordField) []core.RecordField {
	for _, field := range added {
		if !slices.ContainsFunc(held, func(present core.RecordField) bool {
			return sameTextField(present, field)
		}) {
			held = append(held, field)
		}
	}
	return held
}

// sameTextField は、2 つの項目が同じ名前と同じ比べられる値を持つかを返す。
func sameTextField(left, right core.RecordField) bool {
	if left.Name != right.Name || left.Text == nil || right.Text == nil {
		return false
	}
	leftValue, leftReadable := left.Text.ComparableValue()
	rightValue, rightReadable := right.Text.ComparableValue()
	return leftReadable && rightReadable && leftValue == rightValue
}

// specifiedTerminalOf は、利用者が収集元に付けた割当 1 件から、その収集元の端末を組む。
//
// **端末の外部識別子を持つ割当は、その識別子を derived の項目としてレコードへ足す。**
// 持たない割当は、収集元を記録した端末の鍵に置き、全レコードがその端末を指す。
// 表示名は、どちらの場合も derived の項目として足す。
//
// **割当の IP をレコードの項目にしない。** 収集元のレコードはその IP を記録していない。
// 項目にすると、全レコードが IP を指す関係と、端末の IP の関係の根拠を持つ。割当の IP は
// addAssignedAddressEdges が割当から関係にする。
func specifiedTerminalOf(
	assignment core.TerminalAssignment, accountsLocal bool,
) (sourceTerminal, bool) {
	derivation := assignmentFieldDerivation(assignment)
	terminalIdFieldName := analystTerminalIdFieldName
	if assignment.Origin == core.TerminalAssignmentOriginImportSpecified {
		terminalIdFieldName = importTerminalIdFieldName
	}
	terminal := sourceTerminal{scope: core.RecordScope{AccountNamesLocal: accountsLocal}}
	for _, item := range []struct {
		name     string
		semantic core.SemanticKey
		value    string
	}{
		{terminalIdFieldName, core.SemanticKeyTerminalId, assignment.TerminalId},
		{analystTerminalHostnameFieldName, core.SemanticKeyTerminalHostname,
			assignment.TerminalHostname},
	} {
		if item.value == "" {
			continue
		}
		value, err := core.NewDerivedValue(item.value, derivation)
		if err != nil {
			return sourceTerminal{}, false
		}
		field, err := core.NewTextField(item.name, item.semantic, value)
		if err != nil {
			return sourceTerminal{}, false
		}
		terminal.fields = append(terminal.fields, field)
	}
	if assignment.TerminalId == "" {
		key, built := assignment.TerminalNodeKey()
		if !built {
			return sourceTerminal{}, false
		}
		terminal.scope.Terminal, terminal.scope.NamesTerminal = &key, true
	}
	return terminal, true
}

// importSpecifiedDerivation は、取り込みの起動で指定した端末の項目の導出の説明である。
const importSpecifiedDerivation = "取り込みの起動で指定した端末"

// assignmentFieldDerivation は、利用者が与えた端末の項目の導出の説明を返す。
func assignmentFieldDerivation(assignment core.TerminalAssignment) string {
	if assignment.Origin == core.TerminalAssignmentOriginImportSpecified {
		return importSpecifiedDerivation
	}
	return "分析者が与えた端末の割当: " + assignment.Derivation
}

// unknownTerminalOf は、端末を指定していない収集元の、名前不明の端末を組む。
//
// **端末の範囲で識別する対象を指すレコードだけが、この端末を指す。**
// Proxy の記録のように、端末の範囲の対象を持たないレコードを 1 つのノードへ集めない。
func unknownTerminalOf(contentSha256, fileName string, accountsLocal bool) (sourceTerminal, bool) {
	key, built := core.RecordingTerminalNodeKey(contentSha256)
	if !built {
		return sourceTerminal{}, false
	}
	label, err := core.NewDerivedValue(fileName+" を記録した端末 (名前不明)", unknownTerminalDerivation)
	if err != nil {
		return sourceTerminal{}, false
	}
	return sourceTerminal{
		scope:        core.RecordScope{Terminal: &key, AccountNamesLocal: accountsLocal},
		unknownLabel: &label,
	}, true
}

// assignedLabelDerivationSuffix は、割当の表示名を端末の表示名にした理由である。
const assignedLabelDerivationSuffix = "。この端末のレコードが 2 つ以上のホスト名を名乗るため、割当の表示名を端末の表示名にする"

// applySourceTerminalLabels は、名前不明の端末と、割当が表示名を与えた端末のノードに
// 表示名を付ける。
//
// **表示名を読めたノードの表示名を置き換えない** (graphNode.applyLabel)。同じ内容の
// 収集元を 2 つの file 名で取り込んだときは、先に取り込んだ file 名が残る。
// 表示名を与えずに指定した端末 (IP だけの指定) も、名前不明の端末の表示名を持つ。
//
// **この規則の例外は、割当が表示名を与え、端末のノードのレコードが 2 つ以上の異なる
// ホスト名を名乗るときだけである。** そのノードの表示名は割当の表示名 (derived) にする。
// 割当は収集元の全レコードを 1 台の端末に置くため、改名の前後や別の端末の名前が混ざると、
// 最初のレコードのホスト名は端末を表さず、別の端末のノードと同じ表示名になりうる。
// 原資料のホスト名は端末の属性に残る。1 つのホスト名だけを名乗るノードは原資料の値を保つ。
//
// **同じノードを指す割当の表示名が 2 つ以上に分かれるときは、表示名を 1 つに選ばない。**
// 同じ収集元の割当でも、収集元をまたぐ割当でも、原資料の値を保つ。どの表示名が正しいかを
// 取り込みの順や割当の並びで決めない。表示名が 1 つにそろい筋道が分かれるときは、導き方の
// 文字列の順で先の割当を使い、結果を並びに依らないものにする。
func (g *Graph) applySourceTerminalLabels(result ImportResult, terminals sourceTerminals) {
	assigned := make(map[int][]core.RawAndNormalized)
	for _, publication := range result.publications {
		terminal, found := terminals[publication.status.SourceId]
		if !found {
			continue
		}
		if g.applyCollectionLabel(terminal.collection) {
			continue
		}
		if len(terminal.assignedLabels) > 0 {
			if at, present := g.nodeAt[nodeIdOf(terminal.assignedTerminal)]; present {
				assigned[at] = append(assigned[at], terminal.assignedLabels...)
			}
			continue
		}
		if terminal.unknownLabel == nil || terminal.scope.Terminal == nil {
			continue
		}
		if at, present := g.nodeAt[nodeIdOf(*terminal.scope.Terminal)]; present {
			g.nodes[at].applyLabel(*terminal.unknownLabel)
		}
	}
	for at, labels := range assigned {
		if !namesOneTerminal(labels) || !g.nodes[at].readsSeveralHostnames() {
			continue
		}
		g.nodes[at].label = cloneRawAndNormalized(slices.MinFunc(labels,
			func(left, right core.RawAndNormalized) int {
				return strings.Compare(*left.Derivation, *right.Derivation)
			}))
	}
}

// assignedAddress は、端末から IP への terminal_address のエッジにする割当 1 件である。
type assignedAddress struct {
	terminalAt int
	terminal   core.NodeKey
	assignment core.TerminalAssignment
}

// addAssignedAddressEdges は、収集元を 1 台の端末に置いた割当の IP を、その端末から IP への
// terminal_address のエッジにする。
//
// **エッジは割当を運び、根拠は割当が挙げたレコードだけである。** 収集元のレコードは割当の
// IP を記録していない。取り込みの指定は根拠のレコードを持たないため、そのエッジは根拠を
// 持たない。端末のノードがグラフに無い収集元 (全レコードが自身の端末を名乗る) では作らない。
func (g *Graph) addAssignedAddressEdges(result ImportResult, terminals sourceTerminals) {
	var addresses []assignedAddress
	for _, publication := range result.publications {
		terminal := terminals[publication.status.SourceId]
		terminalAt, present := g.nodeAt[nodeIdOf(terminal.assignedTerminal)]
		if len(terminal.assignments) == 0 || !present {
			continue
		}
		for _, assignment := range terminal.assignments {
			if assignment.ClientIp != "" {
				addresses = append(addresses, assignedAddress{terminalAt, terminal.assignedTerminal, assignment})
			}
		}
	}
	g.addAddressEdges(addresses)
}

// addClientAddressEdges は、分析者が収集元の全体に付けずに与えた割当 (接続元 IP と端末の対応) の
// IP を、割当の端末から IP への terminal_address のエッジにする。割当は案件 1 つの割当である。
//
// **端末のノードが無いときは、割当の端末のノードを足す。** 観測の層の他の処理が端末のノードを
// 作った後に呼ぶ。その処理が数える端末の根拠を、割当のノードで変えない。
func (g *Graph) addClientAddressEdges(scope ImportResult) {
	var addresses []assignedAddress
	for _, assignment := range scope.analystAssignments {
		key, keyed := assignment.TerminalNodeKey()
		if assignment.AppliesToSourceId != "" || assignment.ClientIp == "" || !keyed {
			continue
		}
		terminalAt, present := g.nodeAt[nodeIdOf(key)]
		if !present {
			terminalAt = g.ensureNode(key, core.NodeObservationReferenced)
			label, err := core.NewDerivedValue(assignment.TerminalHostname, assignmentFieldDerivation(assignment))
			if err == nil && assignment.TerminalHostname != "" {
				g.nodes[terminalAt].applyLabel(label)
			}
		}
		addresses = append(addresses, assignedAddress{terminalAt, key, assignment})
	}
	g.addAddressEdges(addresses)
}

// addAddressEdges は割当ごとに、端末から IP への terminal_address のエッジを足す。
func (g *Graph) addAddressEdges(addresses []assignedAddress) {
	// basisRecords は割当が挙げた根拠のレコードの鍵から、そのレコードの位置を探す。
	// 同じ内容を 2 つの収集元として取り込んだときは、両方のレコードが同じ鍵を持つ。
	basisRecords := make(map[assertionRecordKey][]int)
	for _, address := range addresses {
		for _, ref := range address.assignment.BasisRecordRefs {
			basisRecords[assertionRecordKeyOf(ref)] = nil
		}
	}
	if len(basisRecords) > 0 {
		for at, record := range g.records {
			key := assertionRecordKeyOf(core.NewAssertionRecordRef(record.locator))
			if held, wanted := basisRecords[key]; wanted {
				basisRecords[key] = append(held, at)
			}
		}
	}
	for _, address := range addresses {
		g.addAssignedAddressEdge(address.terminalAt, address.terminal, address.assignment, basisRecords)
	}
}

// addAssignedAddressEdge は割当 1 件の IP のエッジを足す。basisRecords は根拠のレコードの鍵
// (assertionRecordKeyOf) から、そのレコードの位置を探す表である。
//
// **レコードが既に同じエッジを作っていたら、そのことを残す** (edgeBasis.observedInRecords)。
// 割当は、レコードのエッジをすべて足した後に足す。割当より前にある terminal_address の
// エッジは、レコードが作ったものである。
func (g *Graph) addAssignedAddressEdge(
	terminalAt int, terminal core.NodeKey, assignment core.TerminalAssignment,
	basisRecords map[assertionRecordKey][]int,
) {
	value, err := core.NewDerivedValue(assignment.ClientIp, assignmentFieldDerivation(assignment))
	if err != nil {
		return
	}
	field, err := core.NewTextField(analystTerminalIpFieldName, core.SemanticKeyTerminalIpAddress, value)
	if err != nil {
		return
	}
	fields := []core.RecordField{field}
	// IP のノードの鍵は、レコードの項目から組むときと同じ経路で組む。
	linked := core.NewRecordGraphInScope(fields,
		core.RecordScope{Terminal: &terminal, NamesTerminal: true})
	for _, link := range linked.Links {
		if link.Kind != core.EdgeKindTerminalAddress {
			continue
		}
		ipAt := g.ensureNode(link.Target, core.NodeObservationReferenced)
		if label, found := nodeLabelOf(fields, link.Target); found {
			g.nodes[ipAt].applyLabel(label)
		}
		edgesBefore := len(g.edges)
		edge := g.ensureEdge(link.Kind, core.RelationStateObserved, terminalAt, ipAt)
		held := g.edges[edge].ensureBasis()
		if edge < edgesBefore && len(held.terminalAssignments) == 0 {
			held.observedInRecords = true
		}
		held.terminalAssignments = append(held.terminalAssignments, assignment)
		for _, ref := range assignment.BasisRecordRefs {
			for _, at := range basisRecords[assertionRecordKeyOf(ref)] {
				g.addEdgeEvidence(edge, at)
			}
		}
	}
}

// assignedAddressMatches は、IP のノードに入る割当のエッジの 1 本以上が絞り込みを通るかを
// 返す (assignmentsMatch)。割当のエッジが入らないノードでは偽である。
func (g Graph) assignedAddressMatches(index int, filter RecordFilter) bool {
	if g.nodes[index].key.Kind != core.NodeKindIp {
		return false
	}
	return slices.ContainsFunc(g.adjacency[index].incoming, func(at int) bool {
		return g.assignmentsMatch(g.edges[at], filter)
	})
}

// assignmentsMatch は、エッジを作った割当の 1 件以上が絞り込みを通るかを返す。割当を
// 持たないエッジでは偽である。
//
// **割当のエッジは、割当の側で絞り込みを判定する。** 根拠は割当が挙げたレコードだけであり
// (取り込みの指定では 0 件)、根拠のレコードで判定すると、割当の端末や期間で絞った要求から
// 割当の端末と IP の関係が消える。
//
//   - 端末: エッジの起点 (割当の端末) のノードの識別子で比べる
//   - 案件: 割当を付けた収集元 (付けない割当は期間を読み取った収集元) の案件で比べる
//   - 収集元: 割当を付けた収集元 (付けない割当は期間を読み取った収集元) で比べる
//   - 期間: 割当の適用期間が期間と重なるかで比べる。両端を時点として読めない割当は通らない
//
// **事象の種別の条件を持つ要求では、割当で通さない。** 割当は事象ではなく、適用期間のあいだ
// 成り立つ端末と IP の対応である。文字列の条件と同じく、割当はその条件に合う欄を持たない。
// その要求では、根拠のレコードが条件を通るときだけエッジとノードが残る。
func (g Graph) assignmentsMatch(edge graphEdge, filter RecordFilter) bool {
	return slices.ContainsFunc(edge.terminalAssignmentList(), func(assignment core.TerminalAssignment) bool {
		return g.assignmentMatches(assignment, g.nodes[edge.source].id, filter)
	})
}

// assignmentMatches は割当 1 件が絞り込みを通るかを返す。terminalNodeId は割当の端末の
// ノードの識別子である。
func (g Graph) assignmentMatches(
	assignment core.TerminalAssignment, terminalNodeId string, filter RecordFilter,
) bool {
	if filter.EventCategory != "" || filter.EventAction != "" || filter.filtersEventActionRange() {
		return false
	}
	// 収集元の全体に付けない割当は、期間を読み取った収集元で比べる (assignmentsInCase と同じ)。
	source := cmp.Or(assignment.AppliesToSourceId, assignment.SourceId)
	if filter.Case != "" && g.caseOfSource[source] != filter.Case {
		return false
	}
	if filter.Terminal != "" && terminalNodeId != filter.Terminal {
		return false
	}
	if len(filter.Sources) > 0 && !slices.Contains(filter.Sources, source) {
		return false
	}
	if filter.TimeFrom == nil && filter.TimeTo == nil {
		return true
	}
	// 地方時の期間は、期間を読み取った収集元の時刻の解釈で読んだ時点で比べる。
	valid := g.interpretedValidRange(assignment)
	from, fromReadable := valid.From.Instant()
	to, toReadable := valid.To.Instant()
	if !fromReadable || !toReadable {
		return false
	}
	unit := filter.TimeUnit
	if filter.TimeTo != nil && from.Truncate(unit).After(filter.TimeTo.Truncate(unit)) {
		return false
	}
	return filter.TimeFrom == nil || !to.Truncate(unit).Before(filter.TimeFrom.Truncate(unit))
}

// namesOneTerminal は、割当の表示名がすべて同じ比べられる値を持つかを返す。
func namesOneTerminal(labels []core.RawAndNormalized) bool {
	name, readable := labels[0].ComparableValue()
	if !readable {
		return false
	}
	return !slices.ContainsFunc(labels[1:], func(label core.RawAndNormalized) bool {
		other, otherReadable := label.ComparableValue()
		return !otherReadable || other != name
	})
}

// readsSeveralHostnames は、ノードの属性が原資料から読んだ異なるホスト名を 2 つ以上持つかを
// 返す。利用者が与えた表示名 (derived) は数えない。
func (n graphNode) readsSeveralHostnames() bool {
	var first string
	seen := false
	for _, attribute := range n.attributes {
		field := attribute.field
		if field.Semantic != core.SemanticKeyTerminalHostname || field.Text == nil ||
			field.Text.ValueState != core.ValueStatePresent {
			continue
		}
		value, readable := field.Text.ComparableValue()
		if !readable {
			continue
		}
		if seen && value != first {
			return true
		}
		first, seen = value, true
	}
	return false
}

// forRecord は、そのレコードに足す端末の項目と、レコードを置く端末の範囲を返す。
//
// **レコードが自身の端末の欄を持つときは項目を足さない。** 収集元が記録した値を利用者の
// 指定で置き換えない。範囲はレコード自身の端末が優先する (core.NewRecordGraphInScope)。
func (t sourceTerminals) forRecord(record RecordEntry) ([]core.RecordField, core.RecordScope) {
	terminal, found := t[record.Locator.SourceId]
	if !found {
		return nil, core.RecordScope{}
	}
	if len(fieldsWithSemantic(record.Terminal, core.SemanticKeyTerminalId)) != 0 {
		return nil, terminal.scope
	}
	if terminal.hostScopedContentSha256 != "" {
		// **分析者の割当がそのホスト名を記録し、レコードの時刻が割当の期間の中にあるときは、
		// 割当の端末に置く。** 同じ端末の別の収集元のレコードと、割当で端末を与えた収集元の
		// レコードが 1 つのノードにまとまる。割当が 2 台以上の端末を指すときはまとめない。
		// 名前の一致だけ (割当無し) ではまとめない。
		scope := t.recordingScope(record)
		if hostname := t.assignableHostname(record); hostname != "" {
			if keys := assignedTerminalsNaming(terminal.namedAssignments, hostname, record.ObservedAt); len(keys) == 1 {
				scope.Terminal, scope.NamesTerminal = &keys[0], true
			}
		}
		return nil, scope
	}
	return terminal.fields, terminal.scope
}

// recordingScope は、forRecord の範囲のうち、分析者の割当で置き場所を差し替える前の、レコードを
// 記録した端末の範囲を返す。ログオンのセッションは、この範囲の端末で Logon ID を比べる。
// 割当の期間の端をまたぐセッションも、同じ端末の 1 つのセッションである。
//
// ホスト名で分ける収集元では、ホスト名を名乗らないレコードは、どの端末にも置かない。**ホスト名を
// 名乗ったレコードは、端末の範囲の対象を名指さなくても、そのホスト名の端末を名指す。**
// レコード自身が端末を名乗っており、全レコードを 1 つのノードへ集めることにならない。
func (t sourceTerminals) recordingScope(record RecordEntry) core.RecordScope {
	terminal, found := t[record.Locator.SourceId]
	if !found {
		return core.RecordScope{}
	}
	scope := terminal.scope
	hostname := t.assignableHostname(record)
	if hostname == "" {
		return scope
	}
	// 収集の端末の名前を名乗ったレコードは、収集の端末を名乗る。
	if terminal.collection != nil && terminal.collection.names[shortHostname(hostname)] {
		key := terminal.collection.key
		scope.Terminal, scope.NamesTerminal = &key, true
		return scope
	}
	if key, built := core.RecordingHostTerminalNodeKey(terminal.hostScopedContentSha256, hostname); built {
		scope.Terminal, scope.NamesTerminal = &key, true
	}
	return scope
}

// assignableHostname は、分析者の割当が置き場所を差し替え得るレコードの、ホスト名の比べる値を返す。
// ホスト名で分ける収集元の、端末の外部識別子を持たずホスト名を名乗るレコードだけが持ち、それ以外は
// 空の字句である。
func (t sourceTerminals) assignableHostname(record RecordEntry) string {
	terminal, found := t[record.Locator.SourceId]
	if !found || terminal.hostScopedContentSha256 == "" ||
		len(fieldsWithSemantic(record.Terminal, core.SemanticKeyTerminalId)) != 0 {
		return ""
	}
	hostname, _ := comparableOfSemantic(record.Terminal, core.SemanticKeyTerminalHostname)
	return hostname
}

// placementAt は、収集元 sourceId が記録した、assignableHostname が hostname のレコードを、時刻 at に
// 置く端末のノードの識別子を返す。規則は forRecord と同じであり、割当が 1 台の端末を指さないときは
// 記録した端末 recording である。
func (t sourceTerminals) placementAt(sourceId, hostname, recording string, at time.Time) string {
	if hostname == "" {
		return recording
	}
	if keys := assignedTerminalsAt(t[sourceId].namedAssignments, hostname, at); len(keys) == 1 {
		return nodeIdOf(keys[0])
	}
	return recording
}

// assignedTerminalIds は、収集元 sourceId の割当のうち、期間を問わず hostname を記録した割当の端末の
// ノードの識別子を返す。placementAt が返し得る、記録した端末以外の端末である。
func (t sourceTerminals) assignedTerminalIds(sourceId, hostname string) []string {
	var ids []string
	if hostname == "" {
		return ids
	}
	for _, named := range t[sourceId].namedAssignments {
		if id := nodeIdOf(named.terminal); named.assignment.NamesHostname(hostname) && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids
}
