package sigma

import (
	"slices"
	"strings"
)

// Sigma のルールがイベントの System の項目に付ける名前。
const (
	FieldEventID  = "EventID"
	FieldChannel  = "Channel"
	FieldProvider = "Provider_Name"
)

// ruleTarget はルール 1 つと、そのルールが指すレコードの集合 1 つの組である。
type ruleTarget struct {
	rule   int
	target *target
}

// ruleIndex は service とイベント ID から、評価するルールを探す表である。レコード 1 件に
// 当てるルールを、logsource が指すレコードの集合を持つものに限る。
type ruleIndex struct {
	byEvent  map[string]map[string][]ruleTarget
	anyEvent map[string][]ruleTarget
}

func indexRules(rules []Rule) ruleIndex {
	index := ruleIndex{byEvent: map[string]map[string][]ruleTarget{}, anyEvent: map[string][]ruleTarget{}}
	for at := range rules {
		for t := range rules[at].targets {
			entry := ruleTarget{rule: at, target: &rules[at].targets[t]}
			service := entry.target.service
			if len(entry.target.eventIDs) == 0 {
				index.anyEvent[service] = append(index.anyEvent[service], entry)
				continue
			}
			if index.byEvent[service] == nil {
				index.byEvent[service] = map[string][]ruleTarget{}
			}
			for _, eventID := range entry.target.eventIDs {
				index.byEvent[service][eventID] = append(index.byEvent[service][eventID], entry)
			}
		}
	}
	return index
}

// Match はレコード 1 件に一致したルールの番号 (Rules の位置) と、一致した検索の名前である。
type Match struct {
	Rule       int
	Selections []string
}

// Record はルールを当てるレコード 1 件である。
type Record struct {
	// Value は Sigma のルールの項目名 (FieldEventID、FieldChannel、FieldProvider と、
	// `<EventData>` の `<Data>` の Name) からレコードの値を取り出す。値の無い項目には偽を返す。
	Value func(name string) (string, bool)
	// Named は、レコードが項目名 name の項目を同じ名前で持ちうるかを返す。偽の項目を参照する
	// ルールは、そのレコードに当てない。nil はすべての項目名を持ちうることを表す。
	Named func(name string) bool
}

// MatchResult はレコード 1 件にルールを当てた結果である。
type MatchResult struct {
	// ServiceKnown は、レコードの Channel (または Provider) がどれかの service に一致したかである。
	// 偽のレコードには、どのルールも該当しえない。
	ServiceKnown bool
	// Matches は一致したルールを Rules の順で持つ。
	Matches []Match
	// SkippedRules は、Record.Named が偽の項目を参照するため当てなかったルールの数である。
	SkippedRules int
}

// Match はレコード 1 件にルールを当てる。
//
// 既知の制限: logsource が service だけのルールは、そのチャネルのすべてのレコードに当てる,
// 起動時に 1 回だけ当て、所要はチャネルのレコードの数とそのルールの数の積に比例する,
// 取り込みの待ちが問題になったとき、detection の EventID の条件も表の鍵に使う
func (s RuleSet) Match(r Record) MatchResult {
	value := r.Value
	channel, hasChannel := value(FieldChannel)
	provider, _ := value(FieldProvider)
	eventID, _ := value(FieldEventID)
	service, known := serviceOf(channel, hasChannel, provider, eventID)
	result := MatchResult{ServiceKnown: known}
	if !known {
		return result
	}
	candidates := append(append([]ruleTarget(nil), s.index.byEvent[service][eventID]...), s.index.anyEvent[service]...)
	current := &record{value: value, lowered: map[string]string{}}
	var matches []Match
	for _, candidate := range candidates {
		if !candidate.target.accepts(value) {
			continue
		}
		if r.Named != nil && !allNamed(candidate.target.used, r.Named) {
			result.SkippedRules++
			continue
		}
		rule := &s.Rules[candidate.rule]
		fieldNames := candidate.target.fieldNames
		if !rule.condition.evaluate(rule.memoized(current, fieldNames)) {
			continue
		}
		var selected []string
		for _, sel := range rule.selections {
			if sel.match(current, fieldNames) {
				selected = append(selected, sel.name)
			}
		}
		matches = append(matches, Match{Rule: candidate.rule, Selections: selected})
	}
	// byEvent と anyEvent の 2 つの一覧をつないだため、Rules の順へ並べ直す。
	slices.SortFunc(matches, func(a, b Match) int { return a.Rule - b.Rule })
	result.Matches = matches
	return result
}

func allNamed(names []string, named func(string) bool) bool {
	for _, name := range names {
		if !named(name) {
			return false
		}
	}
	return true
}

// memoized は検索の結果を 1 回だけ求めて覚える関数を返す。
func (r *Rule) memoized(current *record, fieldNames map[string]string) func(int) bool {
	known := make([]int8, len(r.selections))
	return func(index int) bool {
		if known[index] == 0 {
			known[index] = -1
			if r.selections[index].match(current, fieldNames) {
				known[index] = 1
			}
		}
		return known[index] == 1
	}
}

// accepts はレコードが target の equals の欄をすべて満たすかを返す。
func (t *target) accepts(value func(string) (string, bool)) bool {
	for field, allowed := range t.equals {
		text, present := value(field)
		if !present || !containsFold(allowed, text) {
			return false
		}
	}
	return true
}

func containsFold(values []string, text string) bool {
	for _, value := range values {
		if strings.EqualFold(value, text) {
			return true
		}
	}
	return false
}
