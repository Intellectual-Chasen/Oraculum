package pipeline

import (
	"cmp"
	"path"
	"slices"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// terminalRecordClassification は、レコード 1 件の分類と、レコードの収集元の種類である。
type terminalRecordClassification struct {
	categories   []core.TerminalCategory
	logonOutcome core.RemoteLogonOutcome
	// sourceKind は、Windows イベントログではチャネルの名前、ほかは成果物の種類である。
	// 分類の収集元の種類に当たらないレコードでは空の文字列である。
	sourceKind string
}

// classifyTerminalRecord と terminalCategorySources は、入力形式に依る分類の処理である。binding の
// file が init で置き換える。
var (
	classifyTerminalRecord = func(core.FormatKey, RecordEntry) terminalRecordClassification {
		return terminalRecordClassification{}
	}
	terminalCategorySources = func(core.TerminalCategory) []string { return []string{} }
)

// terminalFacts は端末 1 台に置いたレコードの分類の集計である。
type terminalFacts struct {
	recordCount int
	// events は分類に入るレコードである。並びはグラフへ入った順である。
	events      []terminalFactEvent
	sourceKinds map[string]struct{}
	// remoteLogons は接続元 IP ごとの遠隔のログオンの集計である。並びは IP を最初に記録した順である。
	remoteLogons []*remoteLogonTally
}

// terminalFactEvent は分類に入るレコード 1 件である。
type terminalFactEvent struct {
	at         int
	categories []core.TerminalCategory
	// fields は取り込み結果の項目を指す。複製しない。
	fields   []core.RecordField
	sourceIp string
	// logonOutcome は遠隔のログオンの分類に入るレコードの結果である。ほかのレコードでは空である。
	logonOutcome core.RemoteLogonOutcome
	// namedNodes は、レコードを根拠に持つ、端末とレコードを除くノードの g.nodes での位置である。
	namedNodes []int
}

// remoteLogonTally は接続元 IP 1 つからの遠隔のログオンの集計である。
type remoteLogonTally struct {
	sourceIp                     string
	failure, success, connection int64
	attempted, loggedOn          []string
	// first と last は時点を持つレコードのうち最も早いものと遅いものの g.records での位置である。
	// 時点を持つレコードが無いときは -1 である。
	first, last int
}

// collectTerminalFact は、位置 at に入れたレコードを、置いた端末の集計へ入れる。
func (g *Graph) collectTerminalFact(format core.FormatKey, record RecordEntry, at int) {
	terminal := g.records[at].placedTerminalNodeId
	if terminal == "" {
		return
	}
	if g.terminalFacts == nil {
		g.terminalFacts = make(map[string]*terminalFacts)
	}
	facts := g.terminalFacts[terminal]
	if facts == nil {
		facts = &terminalFacts{sourceKinds: make(map[string]struct{})}
		g.terminalFacts[terminal] = facts
	}
	facts.recordCount++
	classified := classifyTerminalRecord(format, record)
	if classified.sourceKind != "" {
		facts.sourceKinds[classified.sourceKind] = struct{}{}
	}
	if len(classified.categories) == 0 || record.Semantics == nil {
		return
	}
	fields := record.Semantics.Fields
	sourceIp, _ := comparableOfSemantic(fields, core.SemanticKeyConnectionSourceAddress)
	facts.events = append(facts.events, terminalFactEvent{
		at: at, categories: classified.categories, fields: fields, sourceIp: sourceIp,
		logonOutcome: classified.logonOutcome,
	})
	if classified.logonOutcome != "" && sourceIp != "" && sourceIp != "-" {
		g.tallyRemoteLogon(facts, sourceIp, classified.logonOutcome, accountTextOf(fields), at)
	}
}

// accountTextOf は、レコードの target の役割のアカウントを `ドメイン\名前` か名前の文字列にする。
// 名前を持たないレコードでは空の文字列である。
func accountTextOf(fields []core.RecordField) string {
	name, _ := comparableOfSemantic(fields, core.SemanticKeyTargetAccountName)
	if name == "" || name == "-" {
		return ""
	}
	if domain, _ := comparableOfSemantic(fields, core.SemanticKeyTargetAccountDomain); domain != "" && domain != "-" {
		return domain + `\` + name
	}
	return name
}

// tallyRemoteLogon は遠隔のログオンのレコード 1 件を接続元 IP の集計へ足す。
func (g *Graph) tallyRemoteLogon(
	facts *terminalFacts, sourceIp string, outcome core.RemoteLogonOutcome, account string, at int,
) {
	index := slices.IndexFunc(facts.remoteLogons, func(t *remoteLogonTally) bool { return t.sourceIp == sourceIp })
	if index < 0 {
		facts.remoteLogons = append(facts.remoteLogons, &remoteLogonTally{sourceIp: sourceIp, first: -1, last: -1})
		index = len(facts.remoteLogons) - 1
	}
	tally := facts.remoteLogons[index]
	addAccount := func(accounts []string) []string {
		if account == "" || slices.Contains(accounts, account) {
			return accounts
		}
		return append(accounts, account)
	}
	switch outcome {
	case core.RemoteLogonOutcomeFailure:
		tally.failure++
		tally.attempted = addAccount(tally.attempted)
	case core.RemoteLogonOutcomeSuccess:
		tally.success++
		tally.loggedOn = addAccount(tally.loggedOn)
	case core.RemoteLogonOutcomeConnection:
		tally.connection++
	}
	record := g.records[at]
	if !record.hasInstant {
		return
	}
	if tally.first < 0 || record.instant.Before(g.records[tally.first].instant) {
		tally.first = at
	}
	if tally.last < 0 || record.instant.After(g.records[tally.last].instant) {
		tally.last = at
	}
}

// indexTerminalFactNodes は、分類に入るレコードごとに、そのレコードを根拠に持つノードを探して残す。
// 観測の層のノードを組み終えた後に 1 回呼ぶ。
func (g *Graph) indexTerminalFactNodes() {
	events := make(map[int][]*terminalFactEvent)
	for _, facts := range g.terminalFacts {
		for index := range facts.events {
			event := &facts.events[index]
			events[event.at] = append(events[event.at], event)
		}
	}
	if len(events) == 0 {
		return
	}
	for node, entry := range g.nodes {
		if entry.key.Kind == core.NodeKindTerminal || entry.key.Kind == core.NodeKindRecord {
			continue
		}
		for _, at := range entry.evidence {
			for _, event := range events[at] {
				if !slices.Contains(event.namedNodes, node) {
					event.namedNodes = append(event.namedNodes, node)
				}
			}
		}
	}
}

// TerminalSummaries は、分類に入るレコードまたは収集の registry の端末の情報を持つ端末の一覧を
// 返す。並びはノードの並びの順である。
func (g Graph) TerminalSummaries() []core.TerminalSummary {
	ids := make([]string, 0, len(g.terminalFacts)+len(g.terminalProfiles))
	for id, facts := range g.terminalFacts {
		if len(facts.events) > 0 {
			ids = append(ids, id)
		}
	}
	for id := range g.terminalProfiles {
		if facts := g.terminalFacts[id]; facts == nil || len(facts.events) == 0 {
			ids = append(ids, id)
		}
	}
	summaries := make([]core.TerminalSummary, 0, len(ids))
	for _, id := range ids {
		node := g.graphNodeOf(id)
		if node == nil {
			continue
		}
		summary := core.TerminalSummary{
			Node: *node, Names: []string{}, OperatingSystem: operatingSystemText(g.terminalProfiles[id].OperatingSystem),
		}
		for _, name := range g.TerminalNames(id) {
			summary.Names = append(summary.Names, name.Name)
		}
		if facts := g.terminalFacts[id]; facts != nil {
			summary.RecordCount, summary.CategorizedRecordCount = int64(facts.recordCount), int64(len(facts.events))
		}
		summaries = append(summaries, summary)
	}
	slices.SortFunc(summaries, func(left, right core.TerminalSummary) int {
		return cmp.Compare(g.nodeAt[left.Node.Id], g.nodeAt[right.Node.Id])
	})
	return summaries
}

// operatingSystemText は ProductName、DisplayVersion、`<CurrentBuild>.<UBR>` を空白で繋ぐ。
// UBR は DWORD の `<10 進> (0x<16 進>)` の文字列の 10 進を使う。
func operatingSystemText(values []TerminalProfileValue) string {
	named := make(map[string]string, len(values))
	for _, value := range values {
		if _, taken := named[value.Name]; !taken {
			named[value.Name] = value.Value
		}
	}
	build := named["CurrentBuild"]
	if ubr, _, _ := strings.Cut(named["UBR"], " "); build != "" && ubr != "" {
		build += "." + ubr
	}
	var parts []string
	for _, part := range []string{named["ProductName"], named["DisplayVersion"], build} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, " ")
}

// TerminalDetail は端末のノード id の情報を返す。found が偽になるのは、id が端末のノードでないときである。
func (g Graph) TerminalDetail(id string) (core.TerminalDetail, bool) {
	if !g.IsTerminalNode(id) {
		return core.TerminalDetail{}, false
	}
	profile := g.terminalProfiles[id]
	detail := core.TerminalDetail{
		Node:            *g.graphNodeOf(id),
		OperatingSystem: profileEntriesOf(profile.OperatingSystem),
		Names:           []core.TerminalNameEntry{},
		Addresses:       []core.TerminalAddressPeriod{},
		TimeZone:        profileEntriesOf(profile.TimeZone),
		RemoteLogons:    []core.TerminalRemoteLogonSummary{},
		Categories:      make([]core.TerminalCategoryCount, 0, len(core.TerminalCategories())),
	}
	for _, name := range g.TerminalNames(id) {
		detail.Names = append(detail.Names, core.TerminalNameEntry{
			Name: name.Name, First: name.First, Last: name.Last, RecordRefs: name.RecordRefs,
		})
	}
	for _, address := range profile.Addresses {
		detail.Addresses = append(detail.Addresses, core.TerminalAddressPeriod{
			Interface: address.Interface, Values: profileEntriesOf(address.Values),
			From: cloneTimestampPointer(address.From), To: cloneTimestampPointer(address.To),
		})
	}
	facts := g.terminalFacts[id]
	if facts == nil {
		facts = &terminalFacts{}
	}
	for _, tally := range facts.remoteLogons {
		summary := core.TerminalRemoteLogonSummary{
			SourceIp: tally.sourceIp, FailureCount: tally.failure, SuccessCount: tally.success,
			ConnectionCount:   tally.connection,
			AttemptedAccounts: append([]string{}, tally.attempted...),
			LoggedOnAccounts:  append([]string{}, tally.loggedOn...),
		}
		if tally.first >= 0 {
			first, last := g.records[tally.first], g.records[tally.last]
			summary.First, summary.Last = cloneTimestampPointer(first.eventTime), cloneTimestampPointer(last.eventTime)
			firstRef, lastRef := cloneLocator(first.locator), cloneLocator(last.locator)
			summary.FirstRecordRef, summary.LastRecordRef = &firstRef, &lastRef
		}
		detail.RemoteLogons = append(detail.RemoteLogons, summary)
	}
	for _, category := range core.TerminalCategories() {
		count := core.TerminalCategoryCount{
			Category: category, RequiredSources: terminalCategorySources(category), PresentSources: []string{},
		}
		for _, event := range facts.events {
			if slices.Contains(event.categories, category) {
				count.RecordCount++
			}
		}
		for _, kind := range count.RequiredSources {
			if _, present := facts.sourceKinds[kind]; present {
				count.PresentSources = append(count.PresentSources, kind)
			}
		}
		detail.Categories = append(detail.Categories, count)
	}
	return detail, true
}

// profileEntriesOf は端末の情報の値を応答の組へ直す。
func profileEntriesOf(values []TerminalProfileValue) []core.TerminalProfileEntry {
	entries := make([]core.TerminalProfileEntry, 0, len(values))
	for _, value := range values {
		entries = append(entries, core.TerminalProfileEntry{
			Name: value.Name, Value: value.Value, RecordRef: cloneLocator(value.RecordRef),
		})
	}
	return entries
}

// TerminalEventQuery は端末の分類に入るレコードを絞る条件である。空の項目では絞らない。
type TerminalEventQuery struct {
	Category core.TerminalCategory
	SourceIp string
}

// TerminalEvents は、端末のノード id に置いた、分類に入るレコードのうち query を満たすものを
// 時刻の昇順に並べてすべて返す。時点を持たない
// レコードは末尾にグラフへ入った順で並ぶ。found が偽になるのは、id が端末のノードでないときである。
func (g Graph) TerminalEvents(id string, query TerminalEventQuery) (events []core.TerminalEvent, found bool) {
	if !g.IsTerminalNode(id) {
		return nil, false
	}
	var matched []*terminalFactEvent
	if facts := g.terminalFacts[id]; facts != nil {
		for index := range facts.events {
			event := &facts.events[index]
			if (query.Category == "" || slices.Contains(event.categories, query.Category)) &&
				(query.SourceIp == "" || event.sourceIp == query.SourceIp) {
				matched = append(matched, event)
			}
		}
	}
	slices.SortStableFunc(matched, func(left, right *terminalFactEvent) int {
		leftRecord, rightRecord := &g.records[left.at], &g.records[right.at]
		if leftRecord.hasInstant != rightRecord.hasInstant {
			if leftRecord.hasInstant {
				return -1
			}
			return 1
		}
		return cmp.Or(leftRecord.instant.Compare(rightRecord.instant), left.at-right.at)
	})
	events = make([]core.TerminalEvent, 0, len(matched))
	for _, event := range matched {
		events = append(events, g.terminalEvent(event))
	}
	return events, true
}

// terminalEvent は分類に入るレコード 1 件を応答の組へ直す。
func (g Graph) terminalEvent(event *terminalFactEvent) core.TerminalEvent {
	record := g.records[event.at]
	result := core.TerminalEvent{
		GraphEvidence: core.GraphEvidence{
			RecordRef:       cloneLocator(record.locator),
			EventTime:       cloneTimestampPointer(record.eventTime),
			ObservationKind: cloneObservationKind(record.observationKind),
			EventKind:       record.eventKindPair(),
		},
		Categories:      slices.Clone(event.categories),
		LogonOutcome:    event.logonOutcome,
		OtherEventTimes: []core.Timestamp{},
		Fields:          []core.RecordField{},
		NamedNodes:      make([]core.GraphNode, 0, len(event.namedNodes)),
	}
	for _, additional := range g.additionalEventTimes[event.at] {
		result.OtherEventTimes = append(result.OtherEventTimes, *cloneTimestampPointer(additional.field.Timestamp))
	}
	for _, field := range event.fields {
		if field.Semantic != "" {
			result.Fields = append(result.Fields, field)
		}
	}
	if record.hasRecordNode {
		node := g.graphNode(record.recordNode)
		result.RecordNode = &node
	}
	for _, node := range event.namedNodes {
		result.NamedNodes = append(result.NamedNodes, g.graphNode(node))
	}
	result.OriginalFileNameDiffers = originalFileNameDiffers(event.fields)
	return result
}

// originalFileNameDiffers は、file.original_file_name と file.name の拡張子を除いた名前が、
// 大文字と小文字を区別せずに比べて異なるかを返す。どちらかが空・空白だけのときは偽である。
// 元の名前の形で比べる対象から外すことはしない。偽装したツールの名前を見逃さないためである。
func originalFileNameDiffers(fields []core.RecordField) bool {
	name, _ := comparableOfSemantic(fields, core.SemanticKeyFileName)
	original, _ := comparableOfSemantic(fields, core.SemanticKeyFileOriginalFileName)
	name, original = strings.TrimSpace(name), strings.TrimSpace(original)
	if name == "" || original == "" {
		return false
	}
	return !strings.EqualFold(baseNameWithoutExtension(name), baseNameWithoutExtension(original))
}

// baseNameWithoutExtension は、名前の最後の点から後ろを除いた部分を返す。
func baseNameWithoutExtension(name string) string {
	return strings.TrimSuffix(name, path.Ext(name))
}
