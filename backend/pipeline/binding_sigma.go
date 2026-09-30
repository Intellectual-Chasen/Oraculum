package pipeline

import (
	"cmp"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/sigma"
	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/winevent"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// SigmaRules は読み込んだ Sigma のルールの集合と、その集合を特定する情報である。
type SigmaRules struct {
	set  sigma.RuleSet
	info SigmaRuleSetInfo
}

// Info はルールの集合を特定する情報を返す。
func (r SigmaRules) Info() SigmaRuleSetInfo {
	info := r.info
	info.Revision = clonePointer(info.Revision)
	return info
}

// LoadSigmaRules は spec の directory からルールの集合を読み、ルールの集合の commit を決める。
//
// directory を読めないとき、ルールの file が 1 つも無いとき、渡した commit が git の HEAD と
// 食い違うとき、記録した内容の識別と読んだ集合の識別が食い違うときに error を返す。
func LoadSigmaRules(spec SigmaRuleSpec) (SigmaRules, error) {
	set, err := sigma.Load(os.DirFS(spec.Directory))
	if err != nil {
		return SigmaRules{}, fmt.Errorf("loading the Sigma rules in %q: %w", spec.Directory, err)
	}
	if expected := spec.ExpectedContentSha256; expected != nil && *expected != set.ContentSha256 {
		return SigmaRules{}, fmt.Errorf("the Sigma rules in %q have content %s, recorded %s",
			spec.Directory, set.ContentSha256, *expected)
	}
	revision, err := sigma.ResolveRevision(spec.Directory, spec.Revision)
	if err != nil {
		return SigmaRules{}, fmt.Errorf("deciding the revision of the Sigma rules: %w", err)
	}
	info := SigmaRuleSetInfo{
		Directory: spec.Directory, RevisionSource: string(revision.Source), RevisionDetail: revision.Detail,
		GitWorkTree: revision.WorkTree, ContentSha256: set.ContentSha256, RuleFileCount: int64(set.FileCount),
	}
	if revision.Commit != "" {
		info.Revision = &revision.Commit
	}
	return SigmaRules{set: set, info: info}, nil
}

// EvaluateSigmaRules は、取り込み結果のうち公開された Windows イベントログのレコードに
// ルールを当てる。
//
// rules が nil の起動は RuleSet が nil の結果を返す。
func EvaluateSigmaRules(result ImportResult, rules *SigmaRules) SigmaEvaluation {
	evaluation := SigmaEvaluation{Rules: []SigmaMatchedRule{}, Matches: []SigmaRuleMatch{},
		UnevaluatedRules: []SigmaUnevaluatedRule{}, UnevaluatedRecordGroups: []SigmaUnevaluatedRecordGroup{}}
	if rules == nil {
		return evaluation
	}
	info := rules.Info()
	evaluation.RuleSet = &info
	evaluation.EvaluatedRuleCount = int64(len(rules.set.Rules))
	for _, skipped := range rules.set.Unevaluated {
		evaluation.UnevaluatedRules = append(evaluation.UnevaluatedRules, SigmaUnevaluatedRule{
			Path: skipped.Path, ID: skipped.ID, Title: skipped.Title, Reason: skipped.Reason, Detail: skipped.Detail,
		})
	}
	windowsEventFormats := map[core.FormatKey]struct{}{}
	for _, format := range winevent.Formats() {
		windowsEventFormats[format.Key] = struct{}{}
	}
	matchesByRule := make([][]SigmaRuleMatch, len(rules.set.Rules))
	terminals := sourceTerminalsOf(result)
	unevaluated := map[recordGroupKey]int64{}
	for _, publication := range result.publications {
		format := publication.parser.FormatKey
		if _, windowsEvent := windowsEventFormats[format]; !windowsEvent ||
			publication.status.PublicationState == core.PublicationStateWithheld {
			continue
		}
		for at := range publication.records {
			entry := &publication.records[at]
			if entry.Semantics == nil {
				evaluation.RecordsWithoutSemantics++
				continue
			}
			values := eventValues(entry.Semantics.Fields)
			outcome := rules.set.Match(sigma.Record{Value: values, Named: winevent.NamedFields(format, entry.Semantics.Fields)})
			if !outcome.ServiceKnown {
				unevaluated[unevaluatedGroupOf(values)]++
				continue
			}
			evaluation.EvaluatedRecordCount++
			if outcome.SkippedRules > 0 {
				evaluation.SkippedPairCount += int64(outcome.SkippedRules)
				evaluation.SkippedPairRecordCount++
			}
			for _, match := range outcome.Matches {
				terminal, assigned := sigmaMatchTerminalOf(*entry, terminals[publication.status.SourceId])
				matchesByRule[match.Rule] = append(matchesByRule[match.Rule], SigmaRuleMatch{
					RulePath: rules.set.Rules[match.Rule].Path, Record: entry.Locator,
					MatchedSelections: match.Selections,
					Terminal:          terminal, TerminalAssigned: assigned, EventTime: entry.ObservedAt,
				})
			}
		}
	}
	keys := slices.Collect(maps.Keys(unevaluated))
	slices.SortFunc(keys, func(a, b recordGroupKey) int {
		return cmp.Or(cmp.Compare(unevaluated[b], unevaluated[a]), cmp.Compare(a.by, b.by), strings.Compare(a.text, b.text))
	})
	for _, key := range keys {
		evaluation.UnevaluatedRecordGroups = append(evaluation.UnevaluatedRecordGroups, key.group(unevaluated[key]))
	}
	for at, matches := range matchesByRule {
		if len(matches) == 0 {
			continue
		}
		rule := rules.set.Rules[at]
		selections := make([]SigmaSelection, 0, len(rule.Selections))
		for _, sel := range rule.Selections {
			selections = append(selections, SigmaSelection{Name: sel.Name, Definition: sel.Definition})
		}
		evaluation.Rules = append(evaluation.Rules, SigmaMatchedRule{
			Path: rule.Path, ID: rule.ID, Title: rule.Title, Author: rule.Author, Level: rule.Level,
			Status: rule.Status, Condition: rule.Condition, Selections: selections, MatchCount: int64(len(matches)),
		})
		evaluation.Matches = append(evaluation.Matches, matches...)
	}
	return evaluation
}

// sigmaMatchTerminalOf は、一致したレコードの端末の名前と、それが評価の時点で収集元に割り当てた
// 端末から来たかを返す (SigmaRuleMatch.Terminal)。収集元の割当はすべて同じ端末を指す
// (sourceTerminalsOf) ため、先頭の割当の表示名か識別子を使う。
func sigmaMatchTerminalOf(entry RecordEntry, source sourceTerminal) (string, bool) {
	for _, field := range entry.Terminal {
		if field.Text != nil && field.Text.RawText != nil && *field.Text.RawText != "" {
			return *field.Text.RawText, false
		}
	}
	if len(source.assignments) > 0 {
		assignment := source.assignments[0]
		if name := cmp.Or(assignment.TerminalHostname, assignment.TerminalId); name != "" {
			return name, true
		}
	}
	return "", false
}

// recordGroupKey は、どのルールも該当しえないレコードを分ける鍵である。by は分けた欄の種類で
// あり、Channel、Provider、どちらの欄も無い、の順に並べる。
type recordGroupKey struct {
	by   int
	text string
}

const (
	groupByChannel = iota
	groupByProvider
	groupWithoutChannelAndProvider
)

// group は鍵と件数を応答の組にする。
func (k recordGroupKey) group(count int64) SigmaUnevaluatedRecordGroup {
	group := SigmaUnevaluatedRecordGroup{RecordCount: count}
	switch k.by {
	case groupByChannel:
		group.Channel = &k.text
	case groupByProvider:
		group.Provider = &k.text
	default:
		group.ChannelAndProviderAbsent = true
	}
	return group
}

// unevaluatedGroupOf は、どのルールも該当しえないレコードを分ける鍵を返す。Channel の欄を持つ
// レコードはチャネル、持たないレコードはプロバイダの文字列で分ける。空の文字列もそのまま鍵にする。
func unevaluatedGroupOf(values func(string) (string, bool)) recordGroupKey {
	if channel, found := values(sigma.FieldChannel); found {
		return recordGroupKey{by: groupByChannel, text: channel}
	}
	if provider, found := values(sigma.FieldProvider); found {
		return recordGroupKey{by: groupByProvider, text: provider}
	}
	return recordGroupKey{by: groupWithoutChannelAndProvider}
}

// eventValues は、Sigma のルールの項目名からレコードの原資料の文字列を取り出す関数を返す。項目の表は
// 最初に取り出したときに組む。
func eventValues(fields []core.RecordField) func(string) (string, bool) {
	var byName map[string]string
	return func(name string) (string, bool) {
		if byName == nil {
			byName = make(map[string]string, len(fields))
			for _, field := range fields {
				if field.Text != nil && field.Text.RawText != nil {
					byName[field.Name] = *field.Text.RawText
				}
			}
		}
		text, found := byName[winevent.FieldNameOf(name)]
		return text, found
	}
}
