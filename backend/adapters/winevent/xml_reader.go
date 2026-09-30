package winevent

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

var errNotReset = errors.New("winevent: call Reset with an input first")

var (
	eventOpenTag  = []byte("<Event")
	eventCloseTag = []byte("</Event")
	newline       = []byte{'\n'}
)

// XMLReader は Windows イベントログを XML に書き出した file から `<Event>` 要素を 1 件ずつ
// 読む。zero value に Reset して使う。
//
// **境界を byte 列の走査で決め、1 件ごとに新しい Decoder で読む。** encoding/xml の Decoder は
// 一度 error を返すと以後も error を返すため、1 本の Decoder で file を通すと、壊れた 1 件の
// 後ろの件を読めない。XML 宣言と `<Events>` は Decoder に渡さない。Go の Decoder は
// `version="1.1"` の宣言を受け付けない。
type XMLReader struct {
	content []byte
	// cursor は次の `<Event` を探し始める位置である。
	cursor int
	// lineOffset と lineNumber は、行番号を数え終えた位置とその位置の行番号である。
	// 位置は前にしか進まないため、改行を先頭から数え直さない。
	lineOffset int
	lineNumber int64
	// sawContent は、件または件の外の文字列の失敗を 1 つ以上返したかである。
	sawContent bool
	done       bool
	wasReset   bool
	readErr    error
}

// Reset は入力を差し替え、走査を先頭に戻す。入力全体を読み切ってから走査する。
//
// 既知の制限: 収集元の全体をもう 1 つ複製して保持する,
// 数 MB の file の取り込みで、取り込みの実行全体の最大 RSS が file の大きさの十数倍であり、
// 複製 1 つ分は全体の 1 割に満たない,
// 取り込みの実行が収集元を複製せずに渡せるようになったとき、複製をやめる
func (r *XMLReader) Reset(input io.Reader) {
	*r = XMLReader{}
	if input == nil {
		return
	}
	r.wasReset = true
	r.lineNumber = 1
	// 読み込みに失敗したときも、読めた byte を失敗の位置に使う。
	r.content, r.readErr = io.ReadAll(input)
}

// Next は次の `<Event>` 要素を返す。
//
// 返り値の組み合わせは他の adapter の Reader.Next と同じ 4 通りである。`</Event>` を持たない
// 1 件と XML として読めない 1 件は、原文と位置を持つ Event と失敗を返し、次の件へ進む。
// `</Event>` を持たない 1 件は、次の `<Event` の手前で区切る。
//
// 件と件の間と最後の件の後ろに置けるのは、空白、`<?…?>`、`<!--…-->`、`<!DOCTYPE…>`、
// `<Events…>`、`</Events>` である。ほかの byte は、そこから次の `<Event` の手前までを原文に
// 持つ失敗にする。`<Evxnt>`、接頭辞付きの `<e:Event>`、`<Event/>`、前半が欠けた 1 件を
// 通知せずに飛ばさない。
func (r *XMLReader) Next() (Event, *core.ImportFailure, error) {
	if !r.wasReset {
		return Event{}, nil, fmt.Errorf("reading Windows event XML source: %w", errNotReset)
	}
	if r.readErr != nil {
		err := r.readErr
		r.readErr, r.done = nil, true
		read := Source{
			ByteOffset: int64(len(r.content)),
			LineNumber: int64(bytes.Count(r.content, newline)) + 1,
		}
		failure := failureAt(core.FailureStageRead, read, "a readable Windows event XML file", err.Error())
		return Event{}, failure, fmt.Errorf("reading Windows event XML source at byte offset %d: %w",
			read.ByteOffset, err)
	}
	if r.done {
		return Event{}, nil, io.EOF
	}
	start := indexEventStart(r.content, r.cursor)
	gapEnd := start
	if start < 0 {
		gapEnd = len(r.content)
	}
	if unexpected := unexpectedOutsideEvents(r.content[:gapEnd], r.cursor); unexpected >= 0 {
		r.sawContent = true
		source := r.sourceOf(unexpected, gapEnd)
		return Event{Source: source}, failureAt(core.FailureStageTokenize, source,
			"only white space, declarations, comments, and the <Events> tags outside <Event> elements",
			"bytes outside any <Event> element that are none of them"), nil
	}
	if start < 0 {
		r.done = true
		if !r.sawContent && len(bytes.TrimSpace(r.content)) > 0 {
			return Event{}, failureAt(core.FailureStageTokenize, Source{LineNumber: 1},
				"at least one <Event> element", "the file carries no <Event> start tag"), nil
		}
		return Event{}, nil, io.EOF
	}
	r.sawContent = true
	afterOpen := start + len(eventOpenTag)
	// 終了タグは次の開始タグの手前だけを探す。壊れた件が続く file でも走査を 1 回にする。
	stop := indexEventStart(r.content, afterOpen)
	if stop < 0 {
		stop = len(r.content)
	}
	end := indexEventEnd(r.content[:stop], afterOpen)
	if end < 0 {
		source := r.sourceOf(start, stop)
		return Event{Source: source}, failureAt(core.FailureStageTokenize, source,
			"an <Event> element closed by </Event> before the next <Event>",
			"no </Event> end tag before the next <Event> start tag or the end of the file"), nil
	}
	source := r.sourceOf(start, end)
	event, err := decodeEvent(r.content[start:end])
	if err != nil {
		return Event{Source: source}, failureAt(core.FailureStageTokenize, source,
			"a well-formed <Event> element", syntaxMessage(err, source.LineNumber)), nil
	}
	event.Source = source
	return event, nil, nil
}

// sourceOf は [start, stop) の原文と位置を組み、次の走査を stop から始める。
func (r *XMLReader) sourceOf(start, stop int) Source {
	r.lineNumber += int64(bytes.Count(r.content[r.lineOffset:start], newline))
	r.lineOffset = start
	r.cursor = stop
	segment := r.content[start:stop]
	return Source{
		RawText: string(segment), ByteOffset: int64(start), ByteLength: int64(len(segment)),
		LineNumber: r.lineNumber, LineCount: int64(bytes.Count(segment, newline)) + 1,
	}
}

// indexEventStart は from 以降で最初の `<Event` の開始タグの位置を返す。無ければ -1 を返す。
// 直後が空白か `>` のときだけ開始タグとし、`<Events>` や `<EventData>`、`<EventID>`、
// `<EventRecordID>` に当てない。
func indexEventStart(content []byte, from int) int {
	return scanMarkup(content, from, func(at int) int {
		after := at + len(eventOpenTag)
		if bytes.HasPrefix(content[at:], eventOpenTag) && after < len(content) &&
			(content[after] == '>' || isXMLSpace(content[after])) {
			return at
		}
		return -1
	})
}

// indexEventEnd は from 以降で最初の `</Event>` の終わりの位置 (`>` の次) を返す。無ければ
// -1 を返す。`</Event` と `>` の間の空白を受け付け、`</Events>` に当てない。
func indexEventEnd(content []byte, from int) int {
	return scanMarkup(content, from, func(at int) int {
		if !bytes.HasPrefix(content[at:], eventCloseTag) {
			return -1
		}
		cursor := at + len(eventCloseTag)
		for cursor < len(content) && isXMLSpace(content[cursor]) {
			cursor++
		}
		if cursor < len(content) && content[cursor] == '>' {
			return cursor + 1
		}
		return -1
	})
}

// opaqueSections は、中の `<` をタグとして読まない区間の始まりと終わりである。
var opaqueSections = []struct{ open, close []byte }{
	{commentOpen, commentClose},
	{[]byte("<![CDATA["), []byte("]]>")},
}

// scanMarkup は from 以降の `<` を順に見て、match が 0 以上を返した値を返す。無ければ -1 を
// 返す。コメントと CDATA の区間の中は見ない。
//
// **終わりの無い区間の始まりは、普通の `<` として読み進める。** 区間の後ろを見ないと、
// 終わりの無い 1 つの区間の後ろの件がすべて 1 件の失敗に入る。
func scanMarkup(content []byte, from int, match func(at int) int) int {
	for from < len(content) {
		found := bytes.IndexByte(content[from:], '<')
		if found < 0 {
			return -1
		}
		at := from + found
		if next, opaque := skipOpaque(content, at); opaque && next >= 0 {
			from = next
			continue
		}
		if result := match(at); result >= 0 {
			return result
		}
		from = at + 1
	}
	return -1
}

// skipOpaque は at がコメントか CDATA の区間の始まりであるとき、区間の終わりの次の位置と
// 真を返す。終わりが無いときの位置は -1 である。
func skipOpaque(content []byte, at int) (int, bool) {
	for _, section := range opaqueSections {
		if bytes.HasPrefix(content[at:], section.open) {
			return endAfter(content, at+len(section.open), section.close), true
		}
	}
	return 0, false
}

// endAfter は from 以降で最初の marker の次の位置を返す。無ければ -1 を返す。
func endAfter(content []byte, from int, marker []byte) int {
	found := bytes.Index(content[from:], marker)
	if found < 0 {
		return -1
	}
	return from + found + len(marker)
}

var (
	processingInstructionOpen  = []byte("<?")
	processingInstructionClose = []byte("?>")
	commentOpen                = []byte("<!--")
	commentClose               = []byte("-->")
	doctypeOpen                = []byte("<!DOCTYPE")
	internalSubsetClose        = []byte("]>")
	eventsOpenTag              = []byte("<Events")
	eventsCloseTag             = []byte("</Events")
	tagClose                   = []byte(">")
	utf8BOM                    = []byte{0xEF, 0xBB, 0xBF}
)

// unexpectedOutsideEvents は from 以降のうち、件の外に置けない最初の byte の位置を返す。
// すべて置ける文字列であれば -1 を返す。終わりの無い宣言とコメントは、その始まりの位置を返す。
// file の先頭から見るときだけ、UTF-8 の BOM を置ける文字列として飛ばす。
func unexpectedOutsideEvents(content []byte, from int) int {
	if from == 0 && bytes.HasPrefix(content, utf8BOM) {
		from = len(utf8BOM)
	}
	for from < len(content) {
		if isXMLSpace(content[from]) {
			from++
			continue
		}
		next := skipOutsideEvents(content, from)
		if next < 0 {
			return from
		}
		from = next
	}
	return -1
}

// skipOutsideEvents は at から始まる件の外に置ける文字列 1 つの次の位置を返す。置けない
// 文字列であれば -1 を返す。
func skipOutsideEvents(content []byte, at int) int {
	rest := content[at:]
	switch {
	case bytes.HasPrefix(rest, commentOpen):
		return endAfter(content, at+len(commentOpen), commentClose)
	case bytes.HasPrefix(rest, processingInstructionOpen):
		return endAfter(content, at+len(processingInstructionOpen), processingInstructionClose)
	case bytes.HasPrefix(rest, doctypeOpen):
		// 内部 subset を持つ宣言は `]>` で終わる。
		after := at + len(doctypeOpen)
		if bracket := bytes.IndexAny(content[after:], "[>"); bracket >= 0 && content[after+bracket] == '[' {
			return endAfter(content, after+bracket, internalSubsetClose)
		}
		return endAfter(content, after, tagClose)
	case isTagNamed(rest, eventsOpenTag, true), isTagNamed(rest, eventsCloseTag, false):
		return endAfter(content, at, tagClose)
	}
	return -1
}

// isTagNamed は rest が name のタグで始まるかを返す。名前の直後は空白か `>` であり、
// selfClosing が真なら `/` も受け付ける。
func isTagNamed(rest, name []byte, selfClosing bool) bool {
	if !bytes.HasPrefix(rest, name) || len(rest) == len(name) {
		return false
	}
	after := rest[len(name)]
	return after == '>' || isXMLSpace(after) || (selfClosing && after == '/')
}

func isXMLSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}

// syntaxMessage は Decoder の error を、収集元の中の行番号で書き直す。Decoder が数える行は
// 1 件の原文の中の行である。
func syntaxMessage(err error, eventLine int64) string {
	var syntax *xml.SyntaxError
	if errors.As(err, &syntax) {
		return fmt.Sprintf("XML syntax error on line %d: %s", eventLine+int64(syntax.Line)-1, syntax.Msg)
	}
	return err.Error()
}

// xmlNode は 1 つの要素を、名前・属性・文字列・子要素のまま持つ。
type xmlNode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Text     string     `xml:",chardata"`
	Children []xmlNode  `xml:",any"`
}

// decodeEvent は `<Event>` 要素 1 つの byte 列を Event へ直す。
func decodeEvent(segment []byte) (Event, error) {
	var root xmlNode
	if err := xml.NewDecoder(bytes.NewReader(segment)).Decode(&root); err != nil {
		return Event{}, fmt.Errorf("decoding an Event element: %w", err)
	}
	var event Event
	for _, child := range root.Children {
		switch child.XMLName.Local {
		case "System":
			event.Sections = event.System.fill(child, event.Sections)
		case "EventData":
			for _, data := range child.Children {
				if data.XMLName.Local == "Data" {
					event.EventData = append(event.EventData,
						Value{Name: attributeOf(data, "Name"), Text: data.Text})
					continue
				}
				event.Sections = flatten(data, "EventData."+data.XMLName.Local, event.Sections)
			}
		default:
			event.Sections = flatten(child, child.XMLName.Local, event.Sections)
		}
	}
	return event, nil
}

// fill は `<System>` の子要素を System の項目へ入れ、項目に無い値を sections へ足して返す。
// 同じ項目が 2 回出たときは、1 回目を項目に入れ、2 回目を sections へ足す。
func (s *System) fill(node xmlNode, sections []Value) []Value {
	var values []Value
	for _, child := range node.Children {
		values = flatten(child, child.XMLName.Local, values)
	}
	slots := s.slots()
next:
	for _, value := range values {
		for _, slot := range slots {
			if slot.name == value.Name && *slot.value == nil {
				text := value.Text
				*slot.value = &text
				continue next
			}
		}
		sections = append(sections, Value{Name: "System." + value.Name, Text: value.Text})
	}
	return sections
}

// flatten は要素の属性と、子を持たない要素の文字列を、path を名前にして out へ足す。
//
// 属性を持ち文字列が空の要素 (`<Provider Name="..."/>`) は、文字列の値を足さない。属性を
// 持たない要素は、文字列が空でも値として足す。空の要素は原資料が書いた空の値である。
func flatten(node xmlNode, path string, out []Value) []Value {
	attributes := 0
	for _, attribute := range node.Attrs {
		if isNamespaceDeclaration(attribute) {
			continue
		}
		attributes++
		out = append(out, Value{Name: path + "@" + attribute.Name.Local, Text: attribute.Value})
	}
	if len(node.Children) == 0 {
		if attributes == 0 || node.Text != "" {
			out = append(out, Value{Name: path, Text: node.Text})
		}
		return out
	}
	for _, child := range node.Children {
		out = flatten(child, path+"."+child.XMLName.Local, out)
	}
	return out
}

func isNamespaceDeclaration(attribute xml.Attr) bool {
	return attribute.Name.Space == "xmlns" || (attribute.Name.Space == "" && attribute.Name.Local == "xmlns")
}

// attributeOf は名前の属性の値を返す。属性が無いときは空文字列を返す。
func attributeOf(node xmlNode, name string) string {
	for _, attribute := range node.Attrs {
		if attribute.Name.Local == name {
			return attribute.Value
		}
	}
	return ""
}
