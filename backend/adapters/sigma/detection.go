package sigma

import (
	"net/netip"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// 評価しなかった理由の分類。機械が読む値であり、利用者に見せる文は Unevaluated.Detail が持つ。
const (
	ReasonFileUnreadable       = "yaml_unreadable"
	ReasonNotDetectionRule     = "not_a_detection_rule"
	ReasonRuleCollection       = "rule_collection"
	ReasonLogsourceUnsupported = "logsource_unsupported"
	ReasonKeywordSearch        = "keyword_search"
	ReasonModifierUnsupported  = "modifier_unsupported"
	ReasonValueUnsupported     = "value_unsupported"
	ReasonRegexUnsupported     = "regex_unsupported"
	ReasonConditionUnsupported = "condition_unsupported"
	// ReasonFieldUnavailable は、logsource が指すどのイベントも記録しない項目をルールが参照する
	// ことを表す。
	ReasonFieldUnavailable = "field_unavailable"
)

// unsupported はルールを評価できない理由である。
type unsupported struct {
	reason string
	detail string
}

func (u unsupported) Error() string { return u.reason + ": " + u.detail }

// record は評価の間の 1 件のレコードである。小文字にした値を項目ごとに 1 回だけ作る。
type record struct {
	value   func(name string) (string, bool)
	lowered map[string]string
}

func (r *record) lower(name, raw string) string {
	if lowered, done := r.lowered[name]; done {
		return lowered
	}
	lowered := strings.ToLower(raw)
	r.lowered[name] = lowered
	return lowered
}

// selection は detection の検索 1 つである。alternatives のどれか 1 つの項目がすべて
// 一致したとき一致する。
type selection struct {
	name         string
	alternatives [][]fieldItem
}

func (s selection) match(r *record, fieldNames map[string]string) bool {
	for _, items := range s.alternatives {
		matched := true
		for _, item := range items {
			if !item.match(r, fieldNames) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// fieldItem は項目名 1 つと値の条件である。all が偽なら値のどれか 1 つ、真ならすべてが
// 一致したとき一致する。
type fieldItem struct {
	field    string
	all      bool
	matchers []matcher
}

func (item fieldItem) match(r *record, fieldNames map[string]string) bool {
	name := item.field
	if mapped, found := fieldNames[name]; found {
		name = mapped
	}
	raw, present := r.value(name)
	for _, m := range item.matchers {
		if m.match(r, name, raw, present) != item.all {
			return !item.all
		}
	}
	return item.all
}

type matchKind int

const (
	matchExact matchKind = iota
	matchPrefix
	matchSuffix
	matchContains
	matchPattern
	matchRegex
	matchCIDR
	matchNull
	matchExists
)

// matcher は値 1 つの条件である。文字列の比較は大文字と小文字を区別しない。matchRegex
// だけは式が決める。
type matcher struct {
	kind   matchKind
	text   string
	re     *regexp.Regexp
	prefix netip.Prefix
	exists bool
}

func (m matcher) match(r *record, name, raw string, present bool) bool {
	switch m.kind {
	case matchNull:
		return !present
	case matchExists:
		return present == m.exists
	}
	if !present {
		return false
	}
	switch m.kind {
	case matchExact:
		return r.lower(name, raw) == m.text
	case matchPrefix:
		return strings.HasPrefix(r.lower(name, raw), m.text)
	case matchSuffix:
		return strings.HasSuffix(r.lower(name, raw), m.text)
	case matchContains:
		return strings.Contains(r.lower(name, raw), m.text)
	case matchCIDR:
		addr, err := netip.ParseAddr(raw)
		return err == nil && m.prefix.Contains(addr.Unmap())
	default:
		return m.re.MatchString(raw)
	}
}

// parseSelection は detection の値 1 つを読む。
func parseSelection(name string, node *yaml.Node) (selection, error) {
	parsed := selection{name: name}
	switch node.Kind {
	case yaml.MappingNode:
		items, err := parseFieldItems(node)
		if err != nil {
			return selection{}, err
		}
		parsed.alternatives = [][]fieldItem{items}
	case yaml.SequenceNode:
		for _, element := range node.Content {
			if element.Kind != yaml.MappingNode {
				return selection{}, unsupported{ReasonKeywordSearch, "selection " + name + " searches values without a field name"}
			}
			items, err := parseFieldItems(element)
			if err != nil {
				return selection{}, err
			}
			parsed.alternatives = append(parsed.alternatives, items)
		}
	default:
		return selection{}, unsupported{ReasonKeywordSearch, "selection " + name + " searches values without a field name"}
	}
	if len(parsed.alternatives) == 0 {
		return selection{}, unsupported{ReasonValueUnsupported, "selection " + name + " is empty"}
	}
	return parsed, nil
}

func parseFieldItems(node *yaml.Node) ([]fieldItem, error) {
	if len(node.Content) == 0 {
		return nil, unsupported{ReasonValueUnsupported, "a selection map is empty"}
	}
	items := make([]fieldItem, 0, len(node.Content)/2)
	for at := 0; at+1 < len(node.Content); at += 2 {
		item, err := parseFieldItem(node.Content[at].Value, node.Content[at+1])
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// parseFieldItem は `項目名|修飾子|...` の key と値を読む。
func parseFieldItem(key string, valueNode *yaml.Node) (fieldItem, error) {
	parts := strings.Split(key, "|")
	item := fieldItem{field: parts[0]}
	if item.field == "" {
		return fieldItem{}, unsupported{ReasonKeywordSearch, "item " + key + " has no field name"}
	}
	var values []*yaml.Node
	switch valueNode.Kind {
	case yaml.ScalarNode:
		values = []*yaml.Node{valueNode}
	case yaml.SequenceNode:
		values = valueNode.Content
	default:
		return fieldItem{}, unsupported{ReasonValueUnsupported, "field " + item.field + " has a value that is neither a scalar nor a list"}
	}
	if len(values) == 0 {
		return fieldItem{}, unsupported{ReasonValueUnsupported, "field " + item.field + " has an empty list"}
	}
	mods, err := readModifiers(parts[1:])
	if err != nil {
		return fieldItem{}, err
	}
	item.all = mods.all
	for _, value := range values {
		if value.Kind != yaml.ScalarNode {
			return fieldItem{}, unsupported{ReasonValueUnsupported, "field " + item.field + " has a nested value"}
		}
		built, err := mods.matcherOf(item.field, value)
		if err != nil {
			return fieldItem{}, err
		}
		item.matchers = append(item.matchers, built)
	}
	return item, nil
}

// modifiers は項目名に付いた修飾子である。
type modifiers struct {
	position string // "", "contains", "startswith", "endswith"
	all      bool
	windash  bool
	re       bool
	reFlags  string
	cidr     bool
	exists   bool
}

func readModifiers(names []string) (modifiers, error) {
	var mods modifiers
	for _, name := range names {
		switch name {
		case "contains", "startswith", "endswith":
			if mods.position != "" {
				return modifiers{}, unsupported{ReasonModifierUnsupported, "modifiers " + mods.position + " and " + name + " are combined"}
			}
			mods.position = name
		case "all":
			mods.all = true
		case "windash":
			mods.windash = true
		case "re":
			mods.re = true
		case "i", "m", "s":
			mods.reFlags += name
		case "cidr":
			mods.cidr = true
		case "exists":
			mods.exists = true
		default:
			return modifiers{}, unsupported{ReasonModifierUnsupported, "modifier " + name + " is not supported"}
		}
	}
	special := 0
	for _, set := range []bool{mods.re, mods.cidr, mods.exists, mods.position != "" || mods.windash} {
		if set {
			special++
		}
	}
	if special > 1 || (mods.reFlags != "" && !mods.re) {
		return modifiers{}, unsupported{ReasonModifierUnsupported, "modifiers " + strings.Join(names, "|") + " are combined"}
	}
	return mods, nil
}

// matcherOf は値 1 つの条件を作る。
func (mods modifiers) matcherOf(field string, value *yaml.Node) (matcher, error) {
	if value.Tag == "!!null" {
		return matcher{kind: matchNull}, nil
	}
	switch {
	case mods.exists:
		switch strings.ToLower(value.Value) {
		case "true":
			return matcher{kind: matchExists, exists: true}, nil
		case "false":
			return matcher{kind: matchExists}, nil
		}
		return matcher{}, unsupported{ReasonValueUnsupported, "field " + field + " has a non-boolean exists value"}
	case mods.cidr:
		prefix, err := netip.ParsePrefix(value.Value)
		if err != nil {
			return matcher{}, unsupported{ReasonValueUnsupported, "field " + field + " has an invalid CIDR " + value.Value}
		}
		return matcher{kind: matchCIDR, prefix: prefix.Masked()}, nil
	case mods.re:
		expression := value.Value
		if mods.reFlags != "" {
			expression = "(?" + mods.reFlags + ")" + expression
		}
		compiled, err := regexp.Compile(expression)
		if err != nil {
			return matcher{}, unsupported{ReasonRegexUnsupported, "field " + field + ": " + err.Error()}
		}
		return matcher{kind: matchRegex, re: compiled}, nil
	}
	tokens := tokenize(value.Value, mods.windash)
	switch mods.position {
	case "contains":
		tokens = append(append([]token{{kind: tokenStar}}, tokens...), token{kind: tokenStar})
	case "startswith":
		tokens = append(tokens, token{kind: tokenStar})
	case "endswith":
		tokens = append([]token{{kind: tokenStar}}, tokens...)
	}
	return patternMatcher(tokens)
}

type tokenKind int

const (
	tokenLiteral tokenKind = iota
	tokenStar
	tokenOne
	tokenDash
)

// token は Sigma の文字列の 1 文字である。
type token struct {
	kind tokenKind
	char rune
}

// dashes は windash が `-` と `/` の位置に当てる文字である。
const dashes = "-/–—―"

// tokenize は Sigma の文字列を読む。`*` は 0 文字以上、`?` は 1 文字に一致する。`\` の後ろの
// `*`、`?`、`\` はその文字そのものであり、ほかの文字の前の `\` は `\` そのものである。
//
// windash が真なら、前が単語の文字でなく後ろが単語の文字である `-` と `/` を、
// `-`、`/`、en dash、em dash、horizontal bar のどれにも一致する位置にする。
func tokenize(text string, windash bool) []token {
	var tokens []token
	runes := []rune(text)
	for at := 0; at < len(runes); at++ {
		char := runes[at]
		switch {
		case char == '\\' && at+1 < len(runes) && strings.ContainsRune(`*?\`, runes[at+1]):
			at++
			tokens = append(tokens, token{kind: tokenLiteral, char: runes[at]})
		case char == '*':
			tokens = append(tokens, token{kind: tokenStar})
		case char == '?':
			tokens = append(tokens, token{kind: tokenOne})
		default:
			tokens = append(tokens, token{kind: tokenLiteral, char: char})
		}
	}
	if !windash {
		return tokens
	}
	for at, current := range tokens {
		if current.kind != tokenLiteral || (current.char != '-' && current.char != '/') {
			continue
		}
		before := at > 0 && isWordToken(tokens[at-1])
		after := at+1 < len(tokens) && isWordToken(tokens[at+1])
		if !before && after {
			tokens[at] = token{kind: tokenDash}
		}
	}
	return tokens
}

func isWordToken(t token) bool {
	c := t.char
	return t.kind == tokenLiteral && (c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z')
}

// patternMatcher は文字列の並びから条件を作る。`*` が両端にしか無い並びは文字列の比較にし、
// ほかは正規表現にする。
func patternMatcher(tokens []token) (matcher, error) {
	leading, trailing := 0, len(tokens)
	for leading < trailing && tokens[leading].kind == tokenStar {
		leading++
	}
	for trailing > leading && tokens[trailing-1].kind == tokenStar {
		trailing--
	}
	var literal strings.Builder
	plain := true
	for _, t := range tokens[leading:trailing] {
		if t.kind != tokenLiteral {
			plain = false
			break
		}
		literal.WriteRune(t.char)
	}
	if plain {
		text := strings.ToLower(literal.String())
		switch {
		case leading > 0 && trailing < len(tokens):
			return matcher{kind: matchContains, text: text}, nil
		case leading > 0:
			return matcher{kind: matchSuffix, text: text}, nil
		case trailing < len(tokens):
			return matcher{kind: matchPrefix, text: text}, nil
		default:
			return matcher{kind: matchExact, text: text}, nil
		}
	}
	var expression strings.Builder
	expression.WriteString("(?is)^")
	for _, t := range tokens {
		switch t.kind {
		case tokenStar:
			expression.WriteString(".*")
		case tokenOne:
			expression.WriteString(".")
		case tokenDash:
			expression.WriteString("[" + regexp.QuoteMeta(dashes) + "]")
		default:
			expression.WriteString(regexp.QuoteMeta(string(t.char)))
		}
	}
	expression.WriteString("$")
	compiled, err := regexp.Compile(expression.String())
	if err != nil {
		return matcher{}, unsupported{ReasonValueUnsupported, "a wildcard value cannot be compiled"}
	}
	return matcher{kind: matchPattern, re: compiled}, nil
}
