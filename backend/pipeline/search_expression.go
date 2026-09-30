package pipeline

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 検索式の大きさの上限。
//
// 既知の制限: 式を 4,096 byte、条件を 32 個、括弧と否定の入れ子を 16 階層までに固定する,
// 式の判定は条件ごとに全ノードの全属性を走査するため、要求の手間は条件の数に比例する。
// 分析者が書く式の大きさを記録した実例がまだ無いため、上限の所要は測れない,
// 分析者が上限を超える式を書く操作を実測したときに見直す
const (
	maxSearchExpressionLength = 4096
	maxSearchExpressionTerms  = 32
	maxSearchExpressionDepth  = 16
)

// SearchExpression は解析した検索式である。
//
// **判定の単位はノードの属性である。** 文字列の条件 (GraphQuery.ValueContains) と同じく、
// レコードのノードでは 1 件のレコードの欄に対する判定になる。
type SearchExpression struct {
	root expressionNode
	// namesAccount は、レコードのノードが指すアカウントの表示名が文字列を含むかである
	// (Graph.recordNamesAccountContaining)。欄を持たない文字列だけが使う。nil のときは使わない。
	namesAccount func(node graphNode, value string) bool
}

// SearchExpressionSyntaxError は、検索式を読めなかった理由と式の中の範囲である。
//
// **利用者が入れた文字列を Error の文に載せない。** 載せるのは理由の種別と位置だけである。
type SearchExpressionSyntaxError struct {
	Detail core.SearchExpressionError
}

func (e *SearchExpressionSyntaxError) Error() string {
	return fmt.Sprintf("searchExpression has a syntax error %s at %d", e.Detail.Reason, e.Detail.Offset)
}

// expressionKind は式の要素の種別である。
type expressionKind int

const (
	expressionAnd expressionKind = iota
	expressionOr
	expressionNot
	// expressionAnyField は欄を持たない文字列で、どの欄かに文字列を含むノードに合う。
	expressionAnyField
	expressionCompare
)

// comparison は比較の演算子である。
type comparison int

const (
	comparisonEqual comparison = iota
	comparisonNotEqual
	comparisonContains
	comparisonGreater
	comparisonGreaterOrEqual
	comparisonLess
	comparisonLessOrEqual
)

// ordered は比較が大小を比べる演算子であるかを返す。
func (c comparison) ordered() bool {
	return c == comparisonGreater || c == comparisonGreaterOrEqual ||
		c == comparisonLess || c == comparisonLessOrEqual
}

// holds は、比べた結果 order (欄の値と式の値の先後。負は欄の値が小さい) が演算子を満たすかを返す。
func (c comparison) holds(order int) bool {
	switch c {
	case comparisonGreater:
		return order > 0
	case comparisonGreaterOrEqual:
		return order >= 0
	case comparisonLess:
		return order < 0
	case comparisonLessOrEqual:
		return order <= 0
	default:
		return false
	}
}

// expressionNode は式の要素 1 つである。kind が決める項目だけが値を持つ。
type expressionNode struct {
	kind expressionKind
	// children は and と or の 2 つ以上の要素と、not の 1 つの要素である。
	children []expressionNode
	// field は比較の欄である。
	field FieldTerm
	// operator は比較の演算子である。
	operator comparison
	// value は比較の値と、欄を持たない文字列である。
	value string
	// number と instant は、大小の比較の値を 10 進の数か時点として読んだものである。
	// isInstant が真のとき instant を、偽のとき number を比べる。
	number    decimal
	instant   time.Time
	isInstant bool
}

// ParseSearchExpression は検索式の文字列を読む。
//
// 構文の誤りは *SearchExpressionSyntaxError で返す。
func ParseSearchExpression(text string) (*SearchExpression, error) {
	if len(text) > maxSearchExpressionLength {
		fitting := runesWithin(text, maxSearchExpressionLength)
		return nil, syntaxError(core.SearchExpressionErrorReasonTooLong,
			fitting, utf8.RuneCountInString(text)-fitting)
	}
	tokens, err := lexSearchExpression(text)
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, syntaxError(core.SearchExpressionErrorReasonEmptyExpression,
			0, utf8.RuneCountInString(text))
	}
	parser := expressionParser{tokens: tokens, end: utf8.RuneCountInString(text)}
	root, err := parser.parseOr(0)
	if err != nil {
		return nil, err
	}
	if next, found := parser.peek(); found {
		if next.kind == tokenCloseParen {
			return nil, next.error(core.SearchExpressionErrorReasonUnmatchedParenthesis)
		}
		// 論理の演算子と比較の演算子の前は、条件を読み終えた位置でもある。
		return nil, parser.unexpected(next)
	}
	return &SearchExpression{root: root}, nil
}

// ComparedFields は、式の比較が指す欄を、式に書いた順で返す。同じ欄を 2 回返すことがある。
func (e *SearchExpression) ComparedFields() []FieldTerm {
	var fields []FieldTerm
	var walk func(node expressionNode)
	walk = func(node expressionNode) {
		if node.kind == expressionCompare {
			fields = append(fields, node.field)
		}
		for _, child := range node.children {
			walk(child)
		}
	}
	walk(e.root)
	return fields
}

// runesWithin は、先頭から limit byte に収まる符号位置の個数を返す。
//
// **limit の位置が複数 byte の文字の途中に該当するときは、その文字を数えない。** 文字列を limit で
// 切ってから数えると、切った文字の残りの byte を 1 つの符号位置として数える。
func runesWithin(text string, limit int) int {
	count := 0
	for at := 0; at < len(text); {
		_, size := utf8.DecodeRuneInString(text[at:])
		if at+size > limit {
			break
		}
		at += size
		count++
	}
	return count
}

// syntaxError は理由と範囲から構文の誤りを作る。
func syntaxError(reason core.SearchExpressionErrorReason, offset, length int) error {
	return &SearchExpressionSyntaxError{Detail: core.SearchExpressionError{
		Reason: reason, Offset: offset, Length: length,
	}}
}

// tokenKind は検索式の token の種別である。
type tokenKind int

const (
	tokenWord tokenKind = iota
	tokenQuoted
	tokenOpenParen
	tokenCloseParen
	tokenAnd
	tokenOr
	tokenNot
	tokenComparison
)

// expressionToken は検索式の token 1 つである。offset と length は符号位置の個数である。
type expressionToken struct {
	kind       tokenKind
	text       string
	comparison comparison
	offset     int
	length     int
}

// error は token の範囲を指す構文の誤りを作る。
func (t expressionToken) error(reason core.SearchExpressionErrorReason) error {
	return syntaxError(reason, t.offset, t.length)
}

// startsOperand は token が条件を始められるかを返す。
func (t expressionToken) startsOperand() bool {
	switch t.kind {
	case tokenWord, tokenQuoted, tokenOpenParen, tokenNot:
		return true
	default:
		return false
	}
}

// operatorSymbols は、引用符の外の token を区切る記号である。
const operatorSymbols = "=!<>&|"

// lexSearchExpression は検索式を token へ分ける。
//
// **語の and・or・not・contains は、引用符の外にあるときだけ演算子として読む。** 大文字と
// 小文字を区別しない。
func lexSearchExpression(text string) ([]expressionToken, error) {
	var tokens []expressionToken
	position := 0
	for at := 0; at < len(text); {
		character, size := utf8.DecodeRuneInString(text[at:])
		switch {
		case isExpressionSpace(character):
			at += size
			position++
		case character == '(' || character == ')':
			kind := tokenOpenParen
			if character == ')' {
				kind = tokenCloseParen
			}
			tokens = append(tokens, expressionToken{kind: kind, offset: position, length: 1})
			at += size
			position++
		case character == '"':
			token, consumed, runes, err := lexQuoted(text[at:], position)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token)
			at += consumed
			position += runes
		case strings.ContainsRune(operatorSymbols, character):
			token, consumed, err := lexSymbol(text[at:], position)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token)
			at += consumed
			position += token.length
		default:
			token, consumed := lexWord(text[at:], position)
			tokens = append(tokens, token)
			at += consumed
			position += token.length
		}
	}
	return tokens, nil
}

// isExpressionSpace は token を区切る空白であるかを返す。
func isExpressionSpace(character rune) bool {
	return character == ' ' || character == '\t' || character == '\n' || character == '\r'
}

// escapeLength は引用符の中の escape (`\"` と `\\`) の符号位置の個数である。
const escapeLength = 2

// lexQuoted は `"` で始まる token を読む。consumed は byte 数、runes は符号位置の個数である。
func lexQuoted(text string, position int) (expressionToken, int, int, error) {
	var value strings.Builder
	runes := 1
	for at := 1; at < len(text); {
		character, size := utf8.DecodeRuneInString(text[at:])
		switch character {
		case '"':
			return expressionToken{
				kind: tokenQuoted, text: value.String(), offset: position, length: runes + 1,
			}, at + size, runes + 1, nil
		case '\\':
			if at+size >= len(text) {
				return expressionToken{}, 0, 0, syntaxError(
					core.SearchExpressionErrorReasonUnterminatedString, position, runes+1)
			}
			escaped, escapedSize := utf8.DecodeRuneInString(text[at+size:])
			if escaped != '"' && escaped != '\\' {
				return expressionToken{}, 0, 0, syntaxError(
					core.SearchExpressionErrorReasonInvalidEscape, position+runes, escapeLength)
			}
			value.WriteRune(escaped)
			at += size + escapedSize
			runes += escapeLength
		default:
			// UTF-8 として読めない byte は 1 byte ずつそのまま写す。読めない byte を持つ値に
			// 当てる検索が、置き換えの文字で別の byte に一致しないようにする。
			value.WriteString(text[at : at+size])
			at += size
			runes++
		}
	}
	return expressionToken{}, 0, 0, syntaxError(
		core.SearchExpressionErrorReasonUnterminatedString, position, runes)
}

// symbolTokens は記号の演算子と、その token の種別である。2 文字の記号を、その先頭の 1 文字の
// 記号より前に並べる。記号はどれも ASCII であり、byte 数と符号位置の個数が等しい。
var symbolTokens = []struct {
	symbol     string
	kind       tokenKind
	comparison comparison
}{
	{"==", tokenComparison, comparisonEqual},
	{"!=", tokenComparison, comparisonNotEqual},
	{">=", tokenComparison, comparisonGreaterOrEqual},
	{"<=", tokenComparison, comparisonLessOrEqual},
	{"&&", tokenAnd, 0},
	{"||", tokenOr, 0},
	{">", tokenComparison, comparisonGreater},
	{"<", tokenComparison, comparisonLess},
	{"!", tokenNot, 0},
}

// lexSymbol は記号の演算子を読む。単独の `=`、`&`、`|` はどの記号も始めない。
func lexSymbol(text string, position int) (expressionToken, int, error) {
	for _, candidate := range symbolTokens {
		if strings.HasPrefix(text, candidate.symbol) {
			return expressionToken{
				kind: candidate.kind, comparison: candidate.comparison,
				offset: position, length: len(candidate.symbol),
			}, len(candidate.symbol), nil
		}
	}
	return expressionToken{}, 0, syntaxError(core.SearchExpressionErrorReasonUnexpectedCharacter, position, 1)
}

// lexWord は、空白・括弧・引用符・記号の演算子の前までを 1 つの語として読む。
func lexWord(text string, position int) (expressionToken, int) {
	at, runes := 0, 0
	for at < len(text) {
		character, size := utf8.DecodeRuneInString(text[at:])
		if isExpressionSpace(character) || character == '(' || character == ')' || character == '"' ||
			strings.ContainsRune(operatorSymbols, character) {
			break
		}
		at += size
		runes++
	}
	word := text[:at]
	token := expressionToken{kind: tokenWord, text: word, offset: position, length: runes}
	switch strings.ToLower(word) {
	case "and":
		token.kind = tokenAnd
	case "or":
		token.kind = tokenOr
	case "not":
		token.kind = tokenNot
	case "contains":
		token.kind, token.comparison = tokenComparison, comparisonContains
	}
	return token, at
}

// expressionParser は token の列を、not・and・or の優先の順で読む。
type expressionParser struct {
	tokens []expressionToken
	next   int
	// terms は読んだ条件 (比較と欄を持たない文字列) の個数である。
	terms int
	// end は式の終わりの位置である。式の終わりで足りない誤りが指す。
	end int
}

// peek は次の token を返す。
func (p *expressionParser) peek() (expressionToken, bool) {
	if p.next >= len(p.tokens) {
		return expressionToken{}, false
	}
	return p.tokens[p.next], true
}

// unexpected は、条件を読み終えた位置に来た token の誤りを作る。
func (p *expressionParser) unexpected(token expressionToken) error {
	if token.kind == tokenComparison {
		return token.error(core.SearchExpressionErrorReasonMissingField)
	}
	return token.error(core.SearchExpressionErrorReasonMissingOperand)
}

// parseOr は or で結んだ条件を読む。depth は括弧と否定の入れ子の深さである。
func (p *expressionParser) parseOr(depth int) (expressionNode, error) {
	first, err := p.parseAnd(depth)
	if err != nil {
		return expressionNode{}, err
	}
	operands := []expressionNode{first}
	for {
		operator, found := p.peek()
		if !found || operator.kind != tokenOr {
			break
		}
		p.next++
		if err := p.requireOperand(operator); err != nil {
			return expressionNode{}, err
		}
		operand, err := p.parseAnd(depth)
		if err != nil {
			return expressionNode{}, err
		}
		operands = append(operands, operand)
	}
	if len(operands) == 1 {
		return first, nil
	}
	return expressionNode{kind: expressionOr, children: operands}, nil
}

// parseAnd は and で結んだ条件と、並べた条件を読む。
func (p *expressionParser) parseAnd(depth int) (expressionNode, error) {
	first, err := p.parseNot(depth)
	if err != nil {
		return expressionNode{}, err
	}
	operands := []expressionNode{first}
	for {
		operator, found := p.peek()
		if !found {
			break
		}
		if operator.kind == tokenAnd {
			p.next++
			if err := p.requireOperand(operator); err != nil {
				return expressionNode{}, err
			}
		} else if !operator.startsOperand() {
			break
		}
		operand, err := p.parseNot(depth)
		if err != nil {
			return expressionNode{}, err
		}
		operands = append(operands, operand)
	}
	if len(operands) == 1 {
		return first, nil
	}
	return expressionNode{kind: expressionAnd, children: operands}, nil
}

// requireOperand は、論理の演算子 operator の後に条件を始める token があることを確かめる。
func (p *expressionParser) requireOperand(operator expressionToken) error {
	next, found := p.peek()
	if !found || !next.startsOperand() {
		return operator.error(core.SearchExpressionErrorReasonMissingOperand)
	}
	return nil
}

// parseNot は not を前に置いた条件を読む。
func (p *expressionParser) parseNot(depth int) (expressionNode, error) {
	token, found := p.peek()
	if !found || token.kind != tokenNot {
		return p.parsePrimary(depth)
	}
	if depth+1 > maxSearchExpressionDepth {
		return expressionNode{}, token.error(core.SearchExpressionErrorReasonNestingTooDeep)
	}
	p.next++
	if err := p.requireOperand(token); err != nil {
		return expressionNode{}, err
	}
	operand, err := p.parseNot(depth + 1)
	if err != nil {
		return expressionNode{}, err
	}
	return expressionNode{kind: expressionNot, children: []expressionNode{operand}}, nil
}

// parsePrimary は括弧で囲んだ式、比較、欄を持たない文字列のどれかを読む。
func (p *expressionParser) parsePrimary(depth int) (expressionNode, error) {
	token, found := p.peek()
	if !found {
		return expressionNode{}, syntaxError(core.SearchExpressionErrorReasonMissingOperand, p.end, 0)
	}
	switch token.kind {
	case tokenOpenParen:
		return p.parseParenthesized(token, depth)
	case tokenWord, tokenQuoted:
		return p.parseTerm(token)
	default:
		return expressionNode{}, p.unexpected(token)
	}
}

// parseParenthesized は括弧で囲んだ式を読む。open は開いた括弧の token である。
func (p *expressionParser) parseParenthesized(open expressionToken, depth int) (expressionNode, error) {
	if depth+1 > maxSearchExpressionDepth {
		return expressionNode{}, open.error(core.SearchExpressionErrorReasonNestingTooDeep)
	}
	p.next++
	if next, found := p.peek(); !found || !next.startsOperand() {
		if !found {
			return expressionNode{}, open.error(core.SearchExpressionErrorReasonUnclosedParenthesis)
		}
		return expressionNode{}, p.unexpected(next)
	}
	inner, err := p.parseOr(depth + 1)
	if err != nil {
		return expressionNode{}, err
	}
	closing, found := p.peek()
	if !found {
		return expressionNode{}, open.error(core.SearchExpressionErrorReasonUnclosedParenthesis)
	}
	if closing.kind != tokenCloseParen {
		return expressionNode{}, p.unexpected(closing)
	}
	p.next++
	return inner, nil
}

// parseTerm は語か引用符の token first で始まる比較か、欄を持たない文字列を読む。
func (p *expressionParser) parseTerm(first expressionToken) (expressionNode, error) {
	p.terms++
	if p.terms > maxSearchExpressionTerms {
		return expressionNode{}, first.error(core.SearchExpressionErrorReasonTooManyTerms)
	}
	p.next++
	operator, found := p.peek()
	if !found || operator.kind != tokenComparison {
		return expressionNode{kind: expressionAnyField, value: first.text}, nil
	}
	p.next++
	value, found := p.peek()
	if !found || (value.kind != tokenWord && value.kind != tokenQuoted) {
		return expressionNode{}, operator.error(core.SearchExpressionErrorReasonMissingValue)
	}
	p.next++
	semantic, name := DesignatedField(first.text)
	node := expressionNode{
		kind: expressionCompare, operator: operator.comparison, value: value.text,
		field: FieldTerm{Semantic: semantic, Name: name},
	}
	if !operator.comparison.ordered() {
		return node, nil
	}
	if number, readable := parseDecimal(value.text); readable {
		node.number = number
		return node, nil
	}
	if instant, err := time.Parse(time.RFC3339, value.text); err == nil {
		node.instant, node.isInstant = instant, true
		return node, nil
	}
	return expressionNode{}, value.error(core.SearchExpressionErrorReasonValueNotOrdered)
}

// DesignatedField は欄の文字列を、語彙の項目か原資料の key のどちらかに振り分ける。
//
// **語彙の項目の一覧に無い文字列は、原資料の key を指したものとして読む。** 収集元に固有の
// 欄を key の文字列で指すためである。検索の文字列を当てる欄、数える欄、検索式の欄が同じ振り分けを使う。
func DesignatedField(text string) (core.SemanticKey, string) {
	if semantic := core.SemanticKey(text); semantic.IsKnown() {
		return semantic, ""
	}
	return "", text
}
