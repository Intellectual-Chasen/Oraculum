package squid

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// CombinedSpec は Squid に組み込みの combined の logformat である。
//
// **欄の位置と意味の出典は Squid の logformat のページである。**
const CombinedSpec = `%>a %[ui %[un [%tl] "%rm %ru HTTP/%rv" %>Hs %<st ` +
	`"%{Referer}>h" "%{User-Agent}>h" %Ss:%Sh`

// CombinedRequestBytesSpec は、状態符号と応答 byte 数の間に要求の byte 数を 1 欄持つ
// logformat である。
//
// combined の `%>Hs` と `%<st` の間に `%>st` を足した、よく使われる設定の形式の文字列である。
// Squid の logformat のページは、`%>st` を `Total size of request received from client`、
// `%<st` を `Total size of reply sent to client (after adaptation)` と定める。
const CombinedRequestBytesSpec = `%>a %[ui %[un [%tl] "%rm %ru HTTP/%rv" %>Hs %>st %<st ` +
	`"%{Referer}>h" "%{User-Agent}>h" %Ss:%Sh`

// Layout は 1 レコードが持つ欄の並びである。zero value は欄を持たない。
//
// **並びを入力の内容から選ばない。** どの並びで読むかは Reader を作る側が渡す。
// 値を作るのは ParseLogFormat である。
type Layout struct {
	spec  string
	items []layoutItem
	// trailer は末尾の欄の後ろに置かれた literal である。
	trailer string
}

// layoutItem は欄 1 つと、直前の欄との間に置かれた literal である。
type layoutItem struct {
	// separator は直前の欄との間の literal である。先頭の欄では、欄の前に置かれた literal である。
	separator string
	name      ItemName
	enclosure enclosure
	// escaped は、値が Squid の引用符の escape (\" と \\ など) を持ちうることを表す。
	// 区切りの literal を探すとき、escape された byte を区切りとして読まない。
	escaped bool
	// shellQuoted は `%/` の値であることを表す。空白を含む値を Squid が引用符で囲む。
	shellQuoted bool
	// widthMin は幅の最小値である。0 は幅の指定が無いことを表す。
	widthMin    int
	leftAligned bool
	// time は時刻の code の読み方である。角括弧で囲んだ既定の %tl と、時刻以外の欄では nil である。
	time *timeSpec
}

// enclosure は欄を囲む literal の種類である。zero value は囲みを持たない欄である。
type enclosure int

const (
	// enclosureQuotes は引用符で囲む欄である。引用符内の escape を復号する。
	enclosureQuotes enclosure = iota + 1
	// enclosureBrackets は角括弧で囲む欄である。角括弧を原資料の文字列に含める。
	enclosureBrackets
)

// timeSpec は時刻の code の値の読み方である。
type timeSpec struct {
	// code は ts、tl、tg のいずれかである。
	code string
	// format は tl と tg の strftime の書式である。ts では zero value である。
	format strftimeFormat
	// subsecondDigits は、直後の `.%tu` をまとめたときの秒未満の桁数である。まとめない
	// ときは 0 である。
	subsecondDigits int
}

// LayoutCombined は組み込みの combined の並びである。
var LayoutCombined = mustParseLogFormat(CombinedSpec)

// LayoutCombinedRequestBytes は要求の byte 数を 1 欄持つ並びである。
var LayoutCombinedRequestBytes = mustParseLogFormat(CombinedRequestBytesSpec)

// Spec は並びの元になった logformat を返す。zero value では空である。
func (l Layout) Spec() string { return l.spec }

// ItemOrderOf は並びが持つ欄を原文の順で返す。欄を持たない並びには要素数 0 を返す。
//
// **返した slice の変更は並びに及ばない。**
func ItemOrderOf(layout Layout) []ItemName {
	order := make([]ItemName, 0, len(layout.items))
	for _, item := range layout.items {
		order = append(order, item.name)
	}
	return order
}

// separatorAfter は index の欄の直後に来る literal を返す。末尾の欄では末尾の literal を返す。
//
// 直後の欄が囲みを持ち、間の literal が空であるときは、囲みの開きの byte を返す。
func (l Layout) separatorAfter(index int) string {
	if index+1 >= len(l.items) {
		return l.trailer
	}
	next := l.items[index+1]
	if next.separator != "" {
		return next.separator
	}
	switch next.enclosure {
	case enclosureQuotes:
		return `"`
	case enclosureBrackets:
		return "["
	}
	return ""
}

// LogFormatError は logformat を欄の並びへ直せなかった理由である。
type LogFormatError struct {
	// Offset は logformat の中の byte offset である。0 起点。
	Offset int
	// Token は理由の対象になった文字列である。
	Token string
	// Reason は満たされなかった条件である。
	Reason string
}

func (e *LogFormatError) Error() string {
	return fmt.Sprintf("parsing logformat at byte offset %d: %s: %q", e.Offset, e.Reason, e.Token)
}

func mustParseLogFormat(spec string) Layout {
	layout, err := ParseLogFormat(spec)
	if err != nil {
		panic(err)
	}
	return layout
}

// ParseLogFormat は Squid の logformat を欄の並びへ直す。
//
// 文字列は Squid の src/format/Token.cc の Token::parse と同じ順で読む。`%` の後ろに
// encoding (`"` `'` `[` `#` `/`)、左寄せの `-`、0 埋めの `0`、幅、`.` と最大幅、`{arg}`、
// 名前空間 (`ns::`)、code、後置の `{arg}` が続く。literal の `"` は引用符の区間を開閉し、
// 区間の中の code は引用符の escape を持つ。表 (codes.go) に無い code は error を返す。
//
// **未知の code を読み飛ばさない。** 読み飛ばすと後続の欄の位置がすべてずれ、診断を
// 1 件も出さずに別の意味の値を取り込む。
//
// 既知の制限: literal を挟まずに隣り合う 2 つの code を退ける,
// 測る対象が無い。値の境目を文字列から決められない書き方であり、性能でも正答率でもない。
// Squid はこの書き方を受け付けるが、2 つの値の境目は出力に残らない,
// 境目を値の形から決められる code の組を収集元で確認したとき、その組だけを受け付ける
func ParseLogFormat(spec string) (Layout, error) {
	parts, err := lexLogFormat(spec)
	if err != nil {
		return Layout{}, err
	}
	return buildLayout(spec, parts)
}

// specifier は logformat の `%` で始まる文字列 1 つである。
type specifier struct {
	offset int
	text   string
	code   formatCode
	// codeText は名前空間を含む code の文字列である (`tl`、`ssl::<cert`)。
	codeText    string
	encoding    byte
	leftAligned bool
	widthMin    int
	// widthMax は `.` の後ろの最大幅である。指定が無いときは -1 である。
	widthMax    int
	argument    string
	hasArgument bool
	// argumentOffset は `{arg}` の中身の先頭の byte offset である。
	argumentOffset int
}

// part は literal か specifier のどちらか 1 つである。
type part struct {
	literal   string
	specifier *specifier
}

// lexLogFormat は logformat を literal と specifier の列にする。`%%` と `%byte{N}` は
// literal に含める。隣り合う literal は 1 つにまとめる。
func lexLogFormat(spec string) ([]part, error) {
	var parts []part
	appendLiteral := func(text string) {
		if last := len(parts) - 1; last >= 0 && parts[last].specifier == nil {
			parts[last].literal += text
			return
		}
		parts = append(parts, part{literal: text})
	}
	region, regionOffset := byte(0), 0
	for position := 0; position < len(spec); {
		if spec[position] != '%' {
			end := strings.IndexByte(spec[position:], '%')
			if end < 0 {
				end = len(spec) - position
			}
			// Squid は literal の " で引用符の区間を、[ と ] で [ の encoding の区間を開閉する。
			for index := position; index < position+end; index++ {
				switch {
				case spec[index] == '"' && region == 0:
					region, regionOffset = '"', index
				case spec[index] == '"' && region == '"':
					region = 0
				case spec[index] == '[' && region == 0:
					region, regionOffset = '[', index
				case spec[index] == ']' && region == '[':
					region = 0
				}
			}
			appendLiteral(spec[position : position+end])
			position += end
			continue
		}
		parsed, next, literal, err := lexSpecifier(spec, position, region)
		if err != nil {
			return nil, err
		}
		if parsed == nil {
			appendLiteral(literal)
		} else {
			parts = append(parts, part{specifier: parsed})
		}
		position = next
	}
	if region == '"' {
		return nil, &LogFormatError{Offset: regionOffset, Token: spec[regionOffset:],
			Reason: "the quoted item has no closing quote"}
	}
	return parts, nil
}

// lexSpecifier は start の `%` から specifier を 1 つ読む。literal を書く `%%` と
// `%byte{N}` では specifier を返さず、書く literal を返す。
func lexSpecifier(spec string, start int, region byte) (*specifier, int, string, error) {
	parsed := &specifier{offset: start, widthMax: -1, encoding: region}
	position := start + 1
	incomplete := &LogFormatError{Offset: start, Token: spec[start:],
		Reason: "the item ends with an incomplete specifier"}
	if position >= len(spec) {
		return nil, 0, "", incomplete
	}
	if spec[position] == '%' {
		return nil, position + 1, "%", nil
	}
	switch spec[position] {
	case '"', '\'', '[', '#', '/':
		parsed.encoding = spec[position]
		position++
	}
	if position < len(spec) && spec[position] == '-' {
		parsed.leftAligned = true
		position++
	}
	// 0 埋めは数値の値にだけ作用し、値の文字列に 0 が残る。文字列の値は空白で埋まる。
	if position < len(spec) && spec[position] == '0' {
		position++
	}
	parsed.widthMin, position = decimalAt(spec, position, 0)
	if position+1 < len(spec) && spec[position] == '.' && isDigit(spec[position+1]) {
		parsed.widthMax, position = decimalAt(spec, position+1, -1)
	}
	var err error
	if position < len(spec) && spec[position] == '{' {
		if position, err = parsed.readArgument(spec, position); err != nil {
			return nil, 0, "", err
		}
	}
	if position >= len(spec) {
		return nil, 0, "", incomplete
	}
	if strings.HasPrefix(spec[position:], "byte") {
		return lexByte(spec, start, position+len("byte"), parsed)
	}
	code, length, found := lookupCode(spec[position:])
	if !found {
		return nil, 0, "", &LogFormatError{Offset: start, Token: spec[start:wordEnd(spec, position)],
			Reason: "the specifier is outside the supported set"}
	}
	parsed.code, parsed.codeText = code, strings.TrimPrefix(spec[position:position+length], "http::")
	position += length
	if position < len(spec) && spec[position] == '{' && !parsed.hasArgument {
		if position, err = parsed.readArgument(spec, position); err != nil {
			return nil, 0, "", err
		}
	}
	parsed.text = spec[start:position]
	return parsed, position, "", nil
}

// readArgument は `{` から `}` までを `{arg}` として読み、`}` の次の位置を返す。
func (s *specifier) readArgument(spec string, open int) (int, error) {
	closeAt := strings.IndexByte(spec[open:], '}')
	if closeAt < 0 {
		return 0, &LogFormatError{Offset: s.offset, Token: spec[s.offset:],
			Reason: "the specifier has no closing brace"}
	}
	s.argument, s.hasArgument, s.argumentOffset = spec[open+1:open+closeAt], true, open+1
	return open + closeAt + 1, nil
}

// lexByte は `%byte{N}` を 1 byte の literal にする。
//
// LF と CR は 1 行を 1 レコードとして読む前提を崩すため退ける。
func lexByte(spec string, start, position int, parsed *specifier) (*specifier, int, string, error) {
	var err error
	if position < len(spec) && spec[position] == '{' && !parsed.hasArgument {
		if position, err = parsed.readArgument(spec, position); err != nil {
			return nil, 0, "", err
		}
	}
	value, parseErr := strconv.ParseUint(parsed.argument, 10, 8)
	if !parsed.hasArgument || parseErr != nil || value == 0 ||
		(len(parsed.argument) > 1 && parsed.argument[0] == '0') {
		return nil, 0, "", &LogFormatError{Offset: start, Token: spec[start:position],
			Reason: "the byte specifier requires a decimal value from 1 to 255"}
	}
	if value == '\n' || value == '\r' {
		return nil, 0, "", &LogFormatError{Offset: start, Token: spec[start:position],
			Reason: "the byte specifier writes a line break inside a record"}
	}
	return nil, position, string([]byte{byte(value)}), nil
}

// decimalAt は position から 10 進の数字を読み、値と数字の次の位置を返す。数字が無いときは
// absent を返す。
func decimalAt(spec string, position, absent int) (int, int) {
	end := position
	for end < len(spec) && isDigit(spec[end]) {
		end++
	}
	value, err := strconv.Atoi(spec[position:end])
	if end == position || err != nil {
		return absent, position
	}
	return value, end
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// wordEnd は position から空白と `%` の手前までの終わりを返す。未知の code の診断に使う。
func wordEnd(spec string, position int) int {
	for position < len(spec) && spec[position] != ' ' && spec[position] != '\t' && spec[position] != '%' {
		position++
	}
	return position
}

// layoutBuilder は specifier の列から欄の並びを組む途中の状態である。
type layoutBuilder struct {
	layout Layout
	seen   map[ItemName]int
	// hasTime は requestTime の欄をすでに足したかを表す。
	hasTime bool
	// literal は次に足す欄の前に置く literal である。
	literal string
}

// buildLayout は literal と specifier の列を欄の並びにする。
//
// 既存の出力を保つため、`"%rm %ru HTTP/%rv"` を requestLine の 1 欄、`%Ss:%Sh` を
// squidStatus の 1 欄、角括弧で囲んだ既定の `%tl` を角括弧を含む 1 欄にする。
func buildLayout(spec string, parts []part) (Layout, error) {
	b := &layoutBuilder{layout: Layout{spec: spec}, seen: map[ItemName]int{}}
	for index := 0; index < len(parts); index++ {
		current := parts[index].specifier
		if current == nil {
			b.literal = parts[index].literal
			continue
		}
		if previous := index - 1; previous >= 0 && parts[previous].specifier != nil {
			first := parts[previous].specifier
			return Layout{}, &LogFormatError{Offset: first.offset,
				Token:  spec[first.offset : current.offset+len(current.text)],
				Reason: "the specifiers carry no literal between them"}
		}
		consumed, err := b.addItem(parts, index)
		if err != nil {
			return Layout{}, err
		}
		index += consumed
	}
	if len(b.layout.items) == 0 {
		return Layout{}, &LogFormatError{Offset: 0, Token: spec,
			Reason: "the logformat declares no item"}
	}
	b.layout.trailer = b.literal
	return b.layout, nil
}

// addItem は parts[index] の specifier から欄を 1 つ足し、まとめて読んだ後続の part の数を返す。
//
// parts の literal から、欄の囲みにした引用符と角括弧を外す。
func (b *layoutBuilder) addItem(parts []part, index int) (int, error) {
	current := parts[index].specifier
	following := func(offset int) part {
		if index+offset < len(parts) {
			return parts[index+offset]
		}
		return part{}
	}
	if b.startsRequestLine(parts, index) {
		closing := index + len(requestLineParts)
		b.literal = strings.TrimSuffix(b.literal, `"`)
		parts[closing].literal = strings.TrimPrefix(parts[closing].literal, `"`)
		return len(requestLineParts) - 1,
			b.append(layoutItem{name: ItemRequestLine, enclosure: enclosureQuotes, escaped: true}, current)
	}
	after := following(1).literal
	if next := following(pairParts).specifier; current.text == "%Ss" && after == ":" && next != nil && next.text == "%Sh" {
		return pairParts, b.append(layoutItem{name: ItemSquidStatus}, current)
	}
	item, err := b.itemOf(current)
	if err != nil {
		return 0, err
	}
	switch {
	case current.encoding == '"' && strings.HasSuffix(b.literal, `"`) && strings.HasPrefix(after, `"`):
		b.literal = strings.TrimSuffix(b.literal, `"`)
		parts[index+1].literal = strings.TrimPrefix(after, `"`)
		item.enclosure = enclosureQuotes
	case current.text == "%tl" && strings.HasSuffix(b.literal, "[") && strings.HasPrefix(after, "]"):
		b.literal = strings.TrimSuffix(b.literal, "[")
		parts[index+1].literal = strings.TrimPrefix(after, "]")
		item.enclosure, item.time = enclosureBrackets, nil
	}
	if next := following(pairParts).specifier; item.name == ItemRequestTime && item.time != nil &&
		after == "." && next != nil && next.code.kind == valueSubsecond {
		item.time.subsecondDigits = subsecondDigitsOf(next)
		return pairParts, b.append(item, current)
	}
	return 0, b.append(item, current)
}

// pairParts は 2 つの specifier を 1 欄にまとめるときに読む、後続の literal と specifier の
// part の数である。
const pairParts = 2

// requestLineParts は引用符で囲んだ要求行の中の part の並びである。
var requestLineParts = []string{"%rm", " ", "%ru", " HTTP/", "%rv"}

// %tu の桁数。Squid は最大幅を桁数として読み、指定が無いときはミリ秒の桁数で書く
// (src/format/Token.cc)。
const (
	defaultSubsecondDigits = 3
	maxSubsecondDigits     = 6
)

// subsecondDigitsOf は %tu が書く桁数を返す。
func subsecondDigitsOf(subsecond *specifier) int {
	if subsecond.widthMax >= 1 && subsecond.widthMax <= maxSubsecondDigits {
		return subsecond.widthMax
	}
	return defaultSubsecondDigits
}

// startsRequestLine は parts[index] から、引用符で囲んだ `%rm %ru HTTP/%rv` が始まるかを返す。
func (b *layoutBuilder) startsRequestLine(parts []part, index int) bool {
	closing := index + len(requestLineParts)
	if closing >= len(parts) || !strings.HasSuffix(b.literal, `"`) ||
		!strings.HasPrefix(parts[closing].literal, `"`) {
		return false
	}
	for offset, text := range requestLineParts {
		current := parts[index+offset]
		if current.specifier == nil && current.literal != text ||
			current.specifier != nil && current.specifier.text != text {
			return false
		}
	}
	return true
}

// append は欄を並びへ足す。欄の前の literal を separator にし、同じ名前の 2 つ目以降の欄に
// 番号を付ける。
func (b *layoutBuilder) append(item layoutItem, from *specifier) error {
	item.separator = b.literal
	b.literal = ""
	base := item.name
	if count := b.seen[base]; count > 0 {
		item.name = ItemName(string(base) + "." + strconv.Itoa(count+1))
	}
	b.seen[base]++
	if from.widthMax >= 0 && from.code.kind == valueText && feedsSemantic(item.name) {
		return &LogFormatError{Offset: from.offset, Token: from.text,
			Reason: "the " + string(item.name) + " item carries a meaning and cannot be truncated by a maximum width"}
	}
	b.layout.items = append(b.layout.items, item)
	return nil
}

// feedsSemantic は欄が語彙の項目を持つか、要求先の導出に使われるかを返す。切り詰めた値は
// 通知なしに別の IP や別の host になる。
func feedsSemantic(name ItemName) bool {
	switch name {
	case ItemRequestLine, ItemRequestURL, ItemClientRequestURL:
		return true
	}
	return SemanticOfItem(name) != ""
}

// itemOf は specifier 1 つを欄にする。
//
// 時刻の code のうち、時点を秒まで定める最初の 1 つを requestTime にする。
func (b *layoutBuilder) itemOf(from *specifier) (layoutItem, error) {
	item := layoutItem{
		name:        from.code.name,
		escaped:     from.encoding == '"' || from.encoding == '/',
		shellQuoted: from.encoding == '/',
		widthMin:    from.widthMin,
		leftAligned: from.leftAligned,
	}
	if from.code.argument == argumentNamesItem && from.hasArgument {
		name, err := argumentItemName(from)
		if err != nil {
			return layoutItem{}, err
		}
		item.name = name
	}
	if from.code.kind != valueTime {
		return item, nil
	}
	spec := &timeSpec{code: from.codeText}
	full := true
	if from.code.argument == argumentStrftime {
		format, err := compileStrftime(strftimeOf(from))
		if err != nil {
			return layoutItem{}, offsetStrftimeError(err, from)
		}
		spec.format, full = format, format.fullDateTime()
	}
	item.time = spec
	if !b.hasTime && full {
		b.hasTime = true
		item.name = ItemRequestTime
	}
	return item, nil
}

// strftimeOf は tl と tg の書式を返す。`{arg}` が無いときは Squid の既定の書式である
// (src/format/Format.cc)。
func strftimeOf(from *specifier) string {
	if from.hasArgument {
		return from.argument
	}
	if from.codeText == "tg" {
		return "%d/%b/%Y:%H:%M:%S"
	}
	return "%d/%b/%Y:%H:%M:%S %z"
}

// offsetStrftimeError は strftime の書式の中の位置を logformat の中の位置へ直す。
func offsetStrftimeError(err error, from *specifier) error {
	var problem *LogFormatError
	if !errors.As(err, &problem) || !from.hasArgument {
		return &LogFormatError{Offset: from.offset, Token: from.text, Reason: err.Error()}
	}
	problem.Offset += from.argumentOffset
	return problem
}

// argumentItemName は `{arg}` を持つヘッダーなどの欄の名前を返す。
//
// Referer と User-Agent の要求ヘッダーは既存の欄の名前を保つ。HTTP のヘッダーの名前は
// 文字列を検査し、欄の名前に使えない byte を退ける。
func argumentItemName(from *specifier) (ItemName, error) {
	prefix := from.code.argumentPrefix
	if prefix == RequestHeaderItemPrefix && from.argument == "Referer" {
		return ItemReferer, nil
	}
	if prefix == RequestHeaderItemPrefix && from.argument == "User-Agent" {
		return ItemUserAgent, nil
	}
	if !httpHeaderPrefixes[prefix] {
		if !isPrintableArgument(from.argument) {
			return "", &LogFormatError{Offset: from.offset, Token: from.text,
				Reason: "the argument carries a space or a control byte"}
		}
		return ItemName(prefix + from.argument), nil
	}
	// `{名前:区切り要素}` はヘッダーの 1 要素を指す。区切りと要素は名前の後ろに続く。
	header, element, hasElement := strings.Cut(from.argument, ":")
	if !isHeaderName(header) || hasElement && !isPrintableArgument(element) {
		return "", &LogFormatError{Offset: from.offset, Token: from.text,
			Reason: "the header name carries a byte outside letters, digits, hyphen and underscore"}
	}
	return ItemName(prefix + from.argument), nil
}

// httpHeaderPrefixes は `{arg}` が HTTP または ICAP のヘッダーの名前である欄の接頭辞である。
var httpHeaderPrefixes = map[string]bool{
	RequestHeaderItemPrefix: true, ResponseHeaderItemPrefix: true, "adaptedRequestHeader.": true,
	"icapRequestHeader.": true, "icapResponseHeader.": true, "adaptationLastHeader.": true,
}

// isHeaderName はヘッダー名が欄の名前に使える文字列であるかを返す。
func isHeaderName(header string) bool {
	if header == "" {
		return false
	}
	for index := 0; index < len(header); index++ {
		b := header[index]
		letter := (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
		if !letter && !isDigit(b) && b != '-' && b != '_' {
			return false
		}
	}
	return true
}

// isPrintableArgument は `{arg}` が空白と制御文字を持たない印字可能な文字列であるかを返す。
func isPrintableArgument(argument string) bool {
	if argument == "" {
		return false
	}
	for index := 0; index < len(argument); index++ {
		if argument[index] <= ' ' || argument[index] == 0x7f {
			return false
		}
	}
	return true
}
