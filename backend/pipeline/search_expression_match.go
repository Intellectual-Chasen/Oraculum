package pipeline

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// matches は、ノードの属性が式を満たすかを返す。
func (e *SearchExpression) matches(node graphNode) bool {
	return e.root.matches(node, e.namesAccount)
}

// withNamedAccounts は、欄を持たない文字列を、レコードが指すアカウントの表示名にも当てる式の
// 複製を返す。g は式を当てるグラフである。
//
// **欄が識別子だけを持つレコードも、アカウントの名前で見つかる。** グループへのメンバーの追加は
// メンバーを識別子だけで記録し、名前は同じ識別子を名前と共に記録した別のレコードから決まる。
func (e *SearchExpression) withNamedAccounts(g Graph) *SearchExpression {
	return &SearchExpression{root: e.root, namesAccount: g.recordNamesAccountContaining}
}

// recordNamesAccountContaining は、レコードのノード node から関係で指したアカウントのノードの
// 表示名に value を含むものがあるかを返す。レコード以外のノードは偽である。
func (g Graph) recordNamesAccountContaining(node graphNode, value string) bool {
	if node.key.Kind != core.NodeKindRecord {
		return false
	}
	index, found := g.nodeAt[node.id]
	if !found {
		return false
	}
	for _, edge := range g.adjacency[index].outgoing {
		account := g.nodes[g.edges[edge].target]
		if account.key.Kind != core.NodeKindAccount {
			continue
		}
		if raw, present := account.label.RawTextValue(); present && containsFold(raw, value) {
			return true
		}
		if normalized, present := account.label.NormalizedValue(); present && containsFold(normalized, value) {
			return true
		}
	}
	return false
}

// withExactNames は、原資料の key で指した欄を、名前の一致だけで当てるかを決めた式の複製を返す。
// exact は、その名前の欄をグラフが持つかである (Graph.hasFieldNamed)。
//
// **呼び出し元の式を書き換えない。** 同じ式を別のグラフの要求にも渡せる。
func (e *SearchExpression) withExactNames(exact func(name string) bool) *SearchExpression {
	return &SearchExpression{root: e.root.withExactNames(exact), namesAccount: e.namesAccount}
}

func (n expressionNode) withExactNames(exact func(name string) bool) expressionNode {
	if n.kind == expressionCompare {
		n.field.exact = exact(n.field.Name)
		return n
	}
	if len(n.children) == 0 {
		return n
	}
	children := make([]expressionNode, len(n.children))
	for at, child := range n.children {
		children[at] = child.withExactNames(exact)
	}
	n.children = children
	return n
}

func (n expressionNode) matches(node graphNode, namesAccount func(graphNode, string) bool) bool {
	switch n.kind {
	case expressionAnd:
		for _, child := range n.children {
			if !child.matches(node, namesAccount) {
				return false
			}
		}
		return true
	case expressionOr:
		for _, child := range n.children {
			if child.matches(node, namesAccount) {
				return true
			}
		}
		return false
	case expressionNot:
		return !n.children[0].matches(node, namesAccount)
	case expressionAnyField:
		return hasValueMatch(node, n.value, everyField) ||
			namesAccount != nil && namesAccount(node, n.value)
	default:
		return n.compares(node)
	}
}

// everyField は、欄を持たない文字列がどの欄とも比べることを表す。
func everyField(core.RecordField) bool { return true }

// compares は比較 1 つを判定する。
//
// **!= は、欄を持ち、欄のどの値も等しくないノードに合う。** 欄を持たないノードは合わない。
// 欄を持たないノードも含める問いは `not 欄 == 値` で書ける。属性になるのは比べられる値を持つ
// 観測だけであるため (addAttribute)、「欄を持つ」は比べられる値を 1 つ以上持つことである。
func (n expressionNode) compares(node graphNode) bool {
	if n.operator == comparisonNotEqual {
		held := false
		for _, attribute := range node.attributes {
			if !n.field.searches(*attribute.field) {
				continue
			}
			held = true
			if n.equalsAnyForm(*attribute.field) {
				return false
			}
		}
		return held
	}
	for _, attribute := range node.attributes {
		if n.field.searches(*attribute.field) && len(n.satisfiedForms(*attribute.field)) > 0 {
			return true
		}
	}
	return false
}

// equalsAnyForm は、欄の原資料の文字列と正規化値のどちらかが値と等しいかを返す。
func (n expressionNode) equalsAnyForm(field core.RecordField) bool {
	if raw, present := fieldRawText(field); present && equalFold(raw, n.value) {
		return true
	}
	normalized, present := fieldNormalized(field)
	return present && equalFold(normalized, n.value)
}

// satisfiedForms は、欄 1 つの原資料の文字列と正規化値のうち、比較 (== / contains / 大小) を満たす
// ものの形を返す。
//
// **時刻の欄を時点と比べるときは、欄の時刻を時点として読む (core.Timestamp.Instant)。** 時点は
// 正規化値から定まるため、形は正規化値である。UTC からのずれの決まらない地方時は時点を持たず、
// 合わない。
func (n expressionNode) satisfiedForms(field core.RecordField) []core.ValueMatchForm {
	if n.operator.ordered() && n.isInstant && field.Timestamp != nil {
		instant, readable := field.Timestamp.Instant()
		if readable && n.operator.holds(instant.Compare(n.instant)) {
			return []core.ValueMatchForm{core.ValueMatchFormNormalized}
		}
		return nil
	}
	var forms []core.ValueMatchForm
	if raw, present := fieldRawText(field); present && n.valueSatisfies(raw) {
		forms = append(forms, core.ValueMatchFormRawText)
	}
	if normalized, present := fieldNormalized(field); present && n.valueSatisfies(normalized) {
		forms = append(forms, core.ValueMatchFormNormalized)
	}
	return forms
}

// valueSatisfies は、欄の値の文字列 text が比較 (== / contains / 大小) を満たすかを返す。
//
// **大小の比較で、数としても時点としても読めない値は合わない。** 値を 0 や既定の時刻として
// 比べない。
func (n expressionNode) valueSatisfies(text string) bool {
	switch n.operator {
	case comparisonEqual:
		return equalFold(text, n.value)
	case comparisonContains:
		return containsFold(text, n.value)
	}
	if n.isInstant {
		instant, err := time.Parse(time.RFC3339, text)
		return err == nil && n.operator.holds(instant.Compare(n.instant))
	}
	number, readable := parseDecimal(text)
	return readable && n.operator.holds(number.compare(n.number))
}

// equalFold は、大文字と小文字を区別せずに 2 つの文字列が等しいかを返す。
//
// **UTF-8 として読めない byte を持つ文字列は、大小をそろえずに byte で比べる。** strings.EqualFold は
// 読めない byte を同じ置き換えの文字として比べ、別の byte どうしを等しいとする。
func equalFold(text, value string) bool {
	if !utf8.ValidString(text) || !utf8.ValidString(value) {
		return text == value
	}
	return strings.EqualFold(text, value)
}

// collectValueMatches は、式の条件のうち否定の下に無いものに一致した欄を、matches へ足して返す。
//
// **否定の下の条件と != は、一致した欄を持たない。** 一致しないことを求める条件であり、画面が
// 一致した欄として示す値が無い。
func (e *SearchExpression) collectValueMatches(
	node graphNode, matches []core.NodeValueMatch,
) []core.NodeValueMatch {
	return e.root.collectValueMatches(node, false, matches)
}

func (n expressionNode) collectValueMatches(
	node graphNode, negated bool, matches []core.NodeValueMatch,
) []core.NodeValueMatch {
	switch n.kind {
	case expressionAnd, expressionOr:
		for _, child := range n.children {
			matches = child.collectValueMatches(node, negated, matches)
		}
		return matches
	case expressionNot:
		return n.children[0].collectValueMatches(node, !negated, matches)
	}
	if negated || (n.kind == expressionCompare && n.operator == comparisonNotEqual) {
		return matches
	}
	for _, attribute := range node.attributes {
		field := *attribute.field
		var forms []core.ValueMatchForm
		if n.kind == expressionAnyField {
			forms = matchedFormsOf(field, n.value)
		} else if n.field.searches(field) {
			forms = n.satisfiedForms(field)
		}
		for _, form := range forms {
			matches = appendValueMatch(matches, field, form)
		}
	}
	return matches
}

// decimal は 10 進の数の文字列を、符号と整数部と小数部に分けたものである。
//
// **浮動小数点へ直さずに桁で比べる。** 64 bit の整数の上限に近い識別子や、桁の多い小数を
// 丸めずに比べる。
type decimal struct {
	negative bool
	// integer は先頭の 0 を外した整数部、fraction は末尾の 0 を外した小数部である。0 はどちらも
	// 空の文字列で、negative は偽である。
	integer  string
	fraction string
}

// parseDecimal は `[+-]数字[.数字]` の形の文字列を読む。指数と 16 進と桁の区切りを読まない。
func parseDecimal(text string) (decimal, bool) {
	negative := false
	if text != "" && (text[0] == '+' || text[0] == '-') {
		negative = text[0] == '-'
		text = text[1:]
	}
	integer, fraction, hasPoint := strings.Cut(text, ".")
	if !allDigits(integer) || (hasPoint && !allDigits(fraction)) {
		return decimal{}, false
	}
	integer = strings.TrimLeft(integer, "0")
	fraction = strings.TrimRight(fraction, "0")
	if integer == "" && fraction == "" {
		negative = false
	}
	return decimal{negative: negative, integer: integer, fraction: fraction}, true
}

// allDigits は文字列が 1 文字以上の ASCII の数字だけからなるかを返す。
func allDigits(text string) bool {
	if text == "" {
		return false
	}
	for index := 0; index < len(text); index++ {
		if text[index] < '0' || text[index] > '9' {
			return false
		}
	}
	return true
}

// compare は d と other の先後を返す。負は d が小さい。
func (d decimal) compare(other decimal) int {
	if d.negative != other.negative {
		if d.negative {
			return -1
		}
		return 1
	}
	order := d.compareMagnitude(other)
	if d.negative {
		return -order
	}
	return order
}

// compareMagnitude は符号を外した 2 つの数の先後を返す。
func (d decimal) compareMagnitude(other decimal) int {
	if len(d.integer) != len(other.integer) {
		if len(d.integer) < len(other.integer) {
			return -1
		}
		return 1
	}
	if order := strings.Compare(d.integer, other.integer); order != 0 {
		return order
	}
	return strings.Compare(d.fraction, other.fraction)
}
