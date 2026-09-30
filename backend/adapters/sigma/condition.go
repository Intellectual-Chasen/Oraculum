package sigma

import (
	"path"
	"strings"
)

// condition は detection の condition の式である。evaluate は選択の番号ごとの結果を取り出す
// 関数を受け取る。
type condition interface {
	evaluate(selected func(index int) bool) bool
}

type selectionRef int
type notExpr struct{ operand condition }
type andExpr []condition
type orExpr []condition

func (s selectionRef) evaluate(selected func(int) bool) bool { return selected(int(s)) }
func (n notExpr) evaluate(selected func(int) bool) bool      { return !n.operand.evaluate(selected) }

func (a andExpr) evaluate(selected func(int) bool) bool {
	for _, operand := range a {
		if !operand.evaluate(selected) {
			return false
		}
	}
	return true
}

func (o orExpr) evaluate(selected func(int) bool) bool {
	for _, operand := range o {
		if operand.evaluate(selected) {
			return true
		}
	}
	return false
}

// conditionParser は condition の文字列を再帰下降で読む。
//
//	expr    = and { "or" and }
//	and     = not { "and" not }
//	not     = "not" not | primary
//	primary = "(" expr ")" | ("1" | "all") "of" (name | pattern | "them") | name
type conditionParser struct {
	tokens []string
	at     int
	names  []string
}

// parseCondition は condition を、names の番号を指す式にする。
func parseCondition(text string, names []string) (condition, error) {
	if strings.Contains(text, "|") {
		return nil, unsupported{ReasonConditionUnsupported, "condition aggregates records with |"}
	}
	spaced := strings.NewReplacer("(", " ( ", ")", " ) ").Replace(text)
	parser := &conditionParser{tokens: strings.Fields(spaced), names: names}
	if len(parser.tokens) == 0 {
		return nil, unsupported{ReasonConditionUnsupported, "condition is empty"}
	}
	parsed, err := parser.expression()
	if err != nil {
		return nil, err
	}
	if parser.at != len(parser.tokens) {
		return nil, parser.fail("unexpected " + parser.tokens[parser.at])
	}
	return parsed, nil
}

func (p *conditionParser) fail(detail string) error {
	return unsupported{ReasonConditionUnsupported, "condition: " + detail}
}

func (p *conditionParser) peek() string {
	if p.at < len(p.tokens) {
		return p.tokens[p.at]
	}
	return ""
}

func (p *conditionParser) expression() (condition, error) {
	return p.chain("or", p.conjunction, func(operands []condition) condition { return orExpr(operands) })
}

func (p *conditionParser) conjunction() (condition, error) {
	return p.chain("and", p.negation, func(operands []condition) condition { return andExpr(operands) })
}

// chain は operator で並ぶ operand を読み、2 つ以上なら join でまとめる。
func (p *conditionParser) chain(
	operator string, operand func() (condition, error), join func([]condition) condition,
) (condition, error) {
	first, err := operand()
	if err != nil {
		return nil, err
	}
	operands := []condition{first}
	for p.peek() == operator {
		p.at++
		next, err := operand()
		if err != nil {
			return nil, err
		}
		operands = append(operands, next)
	}
	if len(operands) == 1 {
		return first, nil
	}
	return join(operands), nil
}

func (p *conditionParser) negation() (condition, error) {
	if p.peek() == "not" {
		p.at++
		operand, err := p.negation()
		if err != nil {
			return nil, err
		}
		return notExpr{operand}, nil
	}
	return p.primary()
}

func (p *conditionParser) primary() (condition, error) {
	word := p.peek()
	p.at++
	switch word {
	case "":
		return nil, p.fail("ends before an operand")
	case "(":
		inner, err := p.expression()
		if err != nil {
			return nil, err
		}
		if p.peek() != ")" {
			return nil, p.fail("a parenthesis is not closed")
		}
		p.at++
		return inner, nil
	case "1", "all":
		if p.peek() != "of" {
			return nil, p.fail(word + " is not followed by of")
		}
		p.at++
		return p.quantified(word, p.peek())
	case ")", "and", "or", "not", "of", "them":
		return nil, p.fail("unexpected " + word)
	}
	if p.peek() == "of" {
		return nil, p.fail(word + " of is not supported")
	}
	for index, name := range p.names {
		if name == word {
			return selectionRef(index), nil
		}
	}
	return nil, p.fail("selection " + word + " is not defined")
}

// quantified は `1 of` と `all of` の対象を読む。them は `_` で始まらない選択のすべてを指す。
func (p *conditionParser) quantified(quantifier, target string) (condition, error) {
	p.at++
	if target == "" {
		return nil, p.fail(quantifier + " of has no target")
	}
	var operands []condition
	for index, name := range p.names {
		included := !strings.HasPrefix(name, "_")
		if target != "them" {
			matched, err := path.Match(target, name)
			if err != nil {
				return nil, p.fail("pattern " + target + " is malformed")
			}
			included = matched
		}
		if included {
			operands = append(operands, selectionRef(index))
		}
	}
	if len(operands) == 0 {
		return nil, p.fail(quantifier + " of " + target + " matches no selection")
	}
	if quantifier == "all" {
		return andExpr(operands), nil
	}
	return orExpr(operands), nil
}
