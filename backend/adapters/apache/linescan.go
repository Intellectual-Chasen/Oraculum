package apache

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

// 既知の制限: 原文を 1 MiB まで保持して上限超過を read の失敗にする,
// 行の長さは収集元の設定ごとに異なり、repo の中に上限を超える入力が無く測れない,
// 上限超過の入力が確認されたときに保持上限を見直す
const maxRecordBytes = 1 << 20

var errRecordTooLong = errors.New("apache: record exceeds the byte limit")
var errNotReset = errors.New("apache: call Reset with an input first")

// lineScanner は収集元を行単位で読む。zero value に reset して使う。
//
// AccessReader と ErrorReader の両方が本型を持つ。行の切り出しは 2 つの形式で同じであり、
// 文字列の分割だけが異なる (access_record.go と error_record.go)。
type lineScanner struct {
	source     *bufio.Reader
	lineNumber int64
	byteOffset int64
	done       bool
}

// reset は入力を差し替え、位置と読み込み終了の状態を初期化する。
func (s *lineScanner) reset(input io.Reader) {
	*s = lineScanner{}
	if input != nil {
		s.source = bufio.NewReader(input)
	}
}

// rawLine は 1 行分の原文と行末と位置を返す。文字列の分割は呼び出し側が行う。
//
// hasLine が偽になるのは、byte を 1 つも読めずに走査が終わったときである。この場合、
// err が io.EOF なら正常な終端、そうでなければ読み込みそのものの失敗であり、lineNumber と
// byteOffset は失敗が起きた位置を指す。
// hasLine が真のときは、読めた断片を record として返す。err が io.EOF 以外の値を持つのは
// 読み込みが断片の後で失敗したときで、その場合も raw と位置は有効である。
func (s *lineScanner) rawLine() (raw, ending string, lineNumber, byteOffset int64, hasLine bool, err error) {
	if s.source == nil {
		return "", "", 0, 0, false, errNotReset
	}
	if s.done {
		return "", "", 0, 0, false, io.EOF
	}
	line, readErr := s.readLine()
	if len(line) == 0 {
		s.done = true
		return "", "", s.lineNumber + 1, s.byteOffset, false, readErr
	}
	s.lineNumber++
	byteOffset = s.byteOffset
	raw, ending = splitLineEnding(line)
	s.byteOffset += int64(len(line))
	if readErr != nil {
		s.done = true
		if !errors.Is(readErr, io.EOF) {
			return raw, ending, s.lineNumber, byteOffset, true, readErr
		}
	}
	return raw, ending, s.lineNumber, byteOffset, true, nil
}

func (s *lineScanner) readLine() (string, error) {
	var line strings.Builder
	for {
		b, err := s.source.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) && line.Len() > maxRecordBytes {
				return line.String(), errRecordTooLong
			}
			return line.String(), err
		}
		line.WriteByte(b)
		if b == '\n' {
			return line.String(), nil
		}
		content := line.Len()
		if b == '\r' {
			content--
		}
		if content > maxRecordBytes {
			return line.String(), errRecordTooLong
		}
	}
}

func splitLineEnding(line string) (string, string) {
	if raw, found := strings.CutSuffix(line, "\r\n"); found {
		return raw, "\r\n"
	}
	if raw, found := strings.CutSuffix(line, "\n"); found {
		return raw, "\n"
	}
	return line, ""
}
