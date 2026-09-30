package winevent

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// viewerColumnsAfterFirst は見出しの 2 欄目から 5 欄目である。イベントビューアーは 1 欄目に
// レベルかキーワードを置き、6 欄目の説明には見出しを書かない。
const viewerColumnsAfterFirst = ",日付と時刻,ソース,イベント ID,タスクのカテゴリ"

// viewerFieldCount はデータ行の欄の数である。
const viewerFieldCount = 6

// viewerDateTimeColumn は 2 欄目の「日付と時刻」の文字列の形である。
const viewerDateTimeColumn = `,\d{4}/\d{1,2}/\d{1,2} \d{1,2}:\d{2}:\d{2},`

// viewerHeaders は見出しの 1 欄目ごとに、1 欄目の値を置く項目の名前と、論理レコードの
// 先頭の行の形を持つ。
//
// 説明の欄は引用符で囲まれ、改行を含む。**論理レコードの先頭を行の形で区切ってから、
// 1 件ずつ CSV として読む。** 閉じない引用符を持つ 1 件が、後ろのレコードを説明として
// 飲み込まない。先頭の行は、1 欄目の値と「日付と時刻」の文字列で始まる。
//
// レベルの値は Windows のレベルの表示名である。キーワードの値はプロバイダが定義する名前で
// あるため、引用符と区切りを含まない文字列を受け付ける。
//
// 既知の制限: 説明の中の行が論理レコードの先頭と同じ形であれば、そこで区切る,
// 説明の文字列はプロバイダごとに異なり、すべてのプロバイダの説明を repo の中に揃えられず測れない,
// 区切りの誤りで壊れたレコードが見つかったとき、区切りに引用符の対応を加える
var viewerHeaders = map[string]struct {
	firstColumnName string
	recordStart     *regexp.Regexp
}{
	"レベル": {"RenderingInfo.Level",
		regexp.MustCompile(`^(?:情報|警告|エラー|重大|詳細)` + viewerDateTimeColumn)},
	"キーワード": {"RenderingInfo.Keywords.Keyword",
		regexp.MustCompile(`^[^",\r\n]*` + viewerDateTimeColumn)},
}

// CSVReader は Windows のイベントビューアーが書き出した CSV から 1 件ずつ読む。zero value に
// Reset して使う。
//
// UTF-8 の file を読み、先頭の BOM を受け付ける。原文の位置は file の先頭からの byte 位置と
// 行番号である。CSV はレコード番号と端末名の欄を持たない。
type CSVReader struct {
	content []byte
	// cursor は次の論理レコードを探し始める行の先頭の位置である。
	cursor int
	// lineOffset と lineNumber は、行番号を数え終えた位置とその位置の行番号である。
	lineOffset int
	lineNumber int64
	// firstColumnName と recordStart は見出しの 1 欄目で決まる (viewerHeaders)。
	firstColumnName string
	recordStart     *regexp.Regexp
	headerRead      bool
	done            bool
	wasReset        bool
	readErr         error
}

// Reset は入力を差し替え、走査を先頭に戻す。入力全体を読み切ってから走査する。
//
// 既知の制限: 収集元の全体をもう 1 つ複製して保持する,
// 複製 1 つ分の大きさは収集元の byte 数と同じである,
// 取り込みの実行が収集元を複製せずに渡せるようになったとき、複製をやめる
func (r *CSVReader) Reset(input io.Reader) {
	*r = CSVReader{}
	if input == nil {
		return
	}
	r.wasReset = true
	r.lineNumber = 1
	r.content, r.readErr = io.ReadAll(input)
}

// Next は次の 1 件を返す。
//
// 返り値の組み合わせは XMLReader.Next と同じ 4 通りである。CSV として読めない 1 件と欄の
// 数が 6 でない 1 件は、原文と位置を持つ Event と失敗を返し、次の論理レコードへ進む。
// 見出しが 2 つの形のどちらでもない file は、見出しの行の失敗を 1 つ返して終わる。
func (r *CSVReader) Next() (Event, *core.ImportFailure, error) {
	if !r.wasReset {
		return Event{}, nil, fmt.Errorf("reading Windows Event Viewer CSV source: %w", errNotReset)
	}
	if r.readErr != nil {
		return r.readFailure()
	}
	if r.done {
		return Event{}, nil, io.EOF
	}
	if !r.headerRead {
		r.headerRead = true
		if failure := r.readHeader(); failure != nil {
			r.done = true
			return Event{Source: failure.source}, failure.failure, nil
		}
	}
	start := r.nextRecordStart(r.cursor)
	gapEnd := start
	if start < 0 {
		gapEnd = len(r.content)
	}
	if len(bytes.TrimSpace(r.content[r.cursor:gapEnd])) > 0 {
		source := r.sourceOf(r.cursor, gapEnd)
		return Event{Source: source}, viewerFailureAt(core.FailureStageTokenize, source,
			"a record starting with the first column value and the date and time",
			"lines that follow no record start"), nil
	}
	if start < 0 {
		r.done = true
		return Event{}, nil, io.EOF
	}
	stop := r.nextRecordStart(lineEnd(r.content, start))
	if stop < 0 {
		// 最後のレコードは、空白でない最後の byte を含む行の改行までである。後ろの空行を
		// レコードの範囲に入れない。次の Next は残りの空白を件の間の空白として読む。
		stop = lineEnd(r.content, start+len(bytes.TrimRight(r.content[start:], " \t\r\n"))-1)
	}
	source := r.sourceOf(start, stop)
	fields, problem := parseViewerRecord(r.content[start:stop], source.LineNumber)
	if problem != "" {
		return Event{Source: source}, viewerFailureAt(core.FailureStageTokenize, source,
			"a CSV record of 6 fields closed before the next record start", problem), nil
	}
	event := r.eventOf(fields)
	event.Source = source
	return event, nil, nil
}

// readFailure は読み込みの失敗を、読めた byte の終わりの位置で 1 度だけ返し、走査を終える。
func (r *CSVReader) readFailure() (Event, *core.ImportFailure, error) {
	err := r.readErr
	r.readErr, r.done = nil, true
	read := Source{
		ByteOffset: int64(len(r.content)),
		LineNumber: int64(bytes.Count(r.content, newline)) + 1,
	}
	failure := viewerFailureAt(core.FailureStageRead, read, "a readable Windows Event Viewer CSV file", err.Error())
	return Event{}, failure, fmt.Errorf("reading Windows Event Viewer CSV source at byte offset %d: %w",
		read.ByteOffset, err)
}

// headerFailure は見出しの失敗と、その原文と位置である。
type headerFailure struct {
	source  Source
	failure *core.ImportFailure
}

// readHeader は 1 行目の見出しを読み、1 欄目で論理レコードの先頭の形を決める。空の file と
// 空白だけの file は、見出しの失敗にせずに終わりへ進める。
func (r *CSVReader) readHeader() *headerFailure {
	begin := 0
	if bytes.HasPrefix(r.content, utf8BOM) {
		begin = len(utf8BOM)
	}
	if len(bytes.TrimSpace(r.content[begin:])) == 0 {
		r.cursor = len(r.content)
		return nil
	}
	end := lineEnd(r.content, begin)
	header := string(bytes.TrimRight(r.content[begin:end], "\r\n"))
	for first, form := range viewerHeaders {
		if header == first+viewerColumnsAfterFirst {
			r.firstColumnName, r.recordStart = form.firstColumnName, form.recordStart
			r.cursor = end
			return nil
		}
	}
	source := r.sourceOf(begin, end)
	return &headerFailure{source: source, failure: viewerFailureAt(core.FailureStageTokenize, source,
		"a header of レベル or キーワード followed by "+viewerColumnsAfterFirst[1:],
		"the first line is "+strconv.Quote(header))}
}

// nextRecordStart は from 以降の行のうち、論理レコードの先頭の形を持つ最初の行の先頭を
// 返す。無ければ -1 を返す。from は行の先頭である。
func (r *CSVReader) nextRecordStart(from int) int {
	for at := from; at < len(r.content); {
		end := lineEnd(r.content, at)
		if r.recordStart.Match(r.content[at:end]) {
			return at
		}
		at = end
	}
	return -1
}

// lineEnd は at を含む行の終わり (改行の次) を返す。改行が無ければ content の長さを返す。
func lineEnd(content []byte, at int) int {
	if found := bytes.IndexByte(content[at:], '\n'); found >= 0 {
		return at + found + 1
	}
	return len(content)
}

// sourceOf は [start, stop) の原文と位置を組み、次の走査を stop から始める。
func (r *CSVReader) sourceOf(start, stop int) Source {
	r.lineNumber += int64(bytes.Count(r.content[r.lineOffset:start], newline))
	r.lineOffset = start
	r.cursor = stop
	segment := r.content[start:stop]
	return Source{
		RawText: string(segment), ByteOffset: int64(start), ByteLength: int64(len(segment)),
		LineNumber: r.lineNumber, LineCount: int64(bytes.Count(bytes.TrimSuffix(segment, newline), newline)) + 1,
	}
}

// parseViewerRecord は論理レコード 1 件を CSV として読み、6 欄を返す。読めないときは、
// 収集元の中の行番号を付けた理由を返す。
func parseViewerRecord(segment []byte, firstLine int64) ([]string, string) {
	reader := csv.NewReader(bytes.NewReader(segment))
	reader.FieldsPerRecord = -1
	fields, err := reader.Read()
	if err != nil {
		var syntax *csv.ParseError
		if errors.As(err, &syntax) {
			return nil, fmt.Sprintf("CSV syntax error on line %d: %v", firstLine+int64(syntax.Line)-1, syntax.Err)
		}
		return nil, err.Error()
	}
	if len(fields) != viewerFieldCount {
		return nil, fmt.Sprintf("the record on line %d has %d fields, not %d", firstLine, len(fields), viewerFieldCount)
	}
	if rest := segment[reader.InputOffset():]; len(bytes.TrimSpace(rest)) > 0 {
		line := firstLine + int64(bytes.Count(segment[:reader.InputOffset()], newline))
		return nil, fmt.Sprintf("bytes on line %d follow the record before the next record start", line)
	}
	return fields, ""
}

// eventOf は 6 欄を Event に組む。ソースはプロバイダの名前、日付と時刻は書き出した端末の
// 地方時の文字列である。
func (r *CSVReader) eventOf(fields []string) Event {
	var event Event
	event.System.ProviderName = &fields[2]
	event.System.EventID = &fields[3]
	event.System.SystemTime = &fields[1]
	event.Sections = []Value{
		{Name: r.firstColumnName, Text: fields[0]},
		{Name: "RenderingInfo.Task", Text: fields[4]},
		{Name: nameMessage, Text: fields[5]},
	}
	event.EventData, event.MessageUnrendered = descriptionValues(fields[2], fields[3], fields[5])
	return event
}

// viewerFailureAt は failureAt の失敗に、CSV の読み取りの解釈を置く。
func viewerFailureAt(stage core.FailureStage, source Source, expected, observed string) *core.ImportFailure {
	failure := failureAt(stage, source, expected, observed)
	switch stage {
	case core.FailureStageRead:
		failure.Interpretation = "the whole Windows Event Viewer CSV file read into memory before any record was interpreted"
	case core.FailureStageTokenize:
		failure.Interpretation = "UTF-8 lines grouped into records at lines starting with the first column value " +
			"and the date and time, each record parsed as CSV on its own"
	}
	return failure
}
