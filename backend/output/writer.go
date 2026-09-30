package output

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

// WriteJSON は値を JSON として書き、制御文字を JSON escape で表す。
// 復号した値の文字列を保持し、末尾にレコード区切りの改行を付ける。
func WriteJSON(w io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("output: encoding JSON: %w", err)
	}
	escaped := append(escapeControls(data), '\n')
	n, err := w.Write(escaped)
	if err != nil {
		return fmt.Errorf("output: writing JSON: %w", err)
	}
	if n != len(escaped) {
		return fmt.Errorf("output: writing JSON: %w", io.ErrShortWrite)
	}
	return nil
}

// escapeControls は JSON の文字列 data の制御文字を JSON escape で表す。data は json.Marshal が
// 書いた値 1 つであり、制御文字は文字列の中にだけ現れる。制御文字が無ければ data をそのまま返す。
func escapeControls(data []byte) []byte {
	if !bytes.ContainsFunc(data, unicode.IsControl) {
		return data
	}
	escaped := make([]byte, 0, len(data)+1)
	for _, r := range string(data) {
		if unicode.IsControl(r) {
			escaped = fmt.Appendf(escaped, "\\u%04x", r)
		} else {
			escaped = utf8.AppendRune(escaped, r)
		}
	}
	return escaped
}

// JSONStream は JSON の値 1 つを部分に分けて書く。部分ごとに WriteJSON と同じく制御文字を
// JSON escape で表し、End が末尾にレコード区切りの改行を付ける。
//
// **値の全体を memory に組まない。** 大きな配列を要素ごとに書く。最初の失敗の後は何も書かず、
// End がその失敗を返す。
type JSONStream struct {
	w   *bufio.Writer
	err error
}

// NewJSONStream は w へ書く JSONStream を返す。
func NewJSONStream(w io.Writer) *JSONStream {
	return &JSONStream{w: bufio.NewWriter(w)}
}

// Token は呼び出し元が書いた JSON の区切りの文字列 (`{`、`,"name":`、`]` など) を書く。
// 文字列は制御文字を含まない。
func (s *JSONStream) Token(text string) {
	if s.err != nil {
		return
	}
	if strings.ContainsFunc(text, unicode.IsControl) {
		s.err = errors.New("output: a JSON token carries a control character")
		return
	}
	s.write([]byte(text))
}

// Value は value を JSON の値 1 つとして書く。
func (s *JSONStream) Value(value any) {
	if s.err != nil {
		return
	}
	data, err := json.Marshal(value)
	if err != nil {
		s.err = fmt.Errorf("output: encoding JSON: %w", err)
		return
	}
	s.write(escapeControls(data))
}

func (s *JSONStream) write(data []byte) {
	if s.err != nil {
		return
	}
	if _, err := s.w.Write(data); err != nil {
		s.err = fmt.Errorf("output: writing JSON: %w", err)
	}
}

// End は末尾のレコード区切りの改行を書き、書き残しを送る。書く途中の最初の失敗を返す。
func (s *JSONStream) End() error {
	s.write([]byte{'\n'})
	if s.err != nil {
		return s.err
	}
	if err := s.w.Flush(); err != nil {
		return fmt.Errorf("output: writing JSON: %w", err)
	}
	return nil
}

// WriteJSONArray は items を JSON の配列として、要素ごとに s へ書く。
func WriteJSONArray[T any](s *JSONStream, items []T) {
	s.Token("[")
	for index, item := range items {
		if index > 0 {
			s.Token(",")
		}
		s.Value(item)
	}
	s.Token("]")
}

// Fprintf は args に書式を適用した文字列を無害化し、w に書き込む。
func Fprintf(w io.Writer, format string, args ...any) (int, error) {
	n, err := fmt.Fprintf(w, format, sanitizeArgs(args)...)
	if err != nil {
		return n, fmt.Errorf("output: writing formatted text: %w", err)
	}
	return n, nil
}

// Fprintln は args の表示文字列を無害化し、空白で区切って末尾に改行を付けて w に書き込む。
func Fprintln(w io.Writer, args ...any) (int, error) {
	n, err := fmt.Fprintln(w, sanitizeArgs(args)...)
	if err != nil {
		return n, fmt.Errorf("output: writing line: %w", err)
	}
	return n, nil
}

func sanitizeArgs(args []any) []any {
	clean := make([]any, len(args))
	for i, arg := range args {
		if needsSanitizing(arg) {
			clean[i] = sanitizingArg(func() any { return arg })
		} else {
			clean[i] = arg
		}
	}
	return clean
}

func needsSanitizing(arg any) bool {
	switch arg.(type) {
	case string, []byte, error, fmt.Stringer:
		return true
	default:
		kind := reflect.TypeOf(arg)
		return kind != nil && kind.Kind() == reflect.String
	}
}

// fmt の書式診断が内部を表示した際にも、生文字列を露出させないよう関数に保持する。
// 既知の制限: %T と %p は wrapper の型名と関数アドレスを表示する,
// TestFprintfPreservesArgumentDirectives と TestFprintfPointerDiagnosticIsSanitized で確認,
// CLI 出力で元の型やアドレスを保持する要件が生じたら見直す
type sanitizingArg func() any

func (a sanitizingArg) Format(state fmt.State, verb rune) {
	text := fmt.Sprintf(fmt.FormatString(state, verb), a())
	// fmt が渡す State は内部バッファに書き込み、書き込み失敗を返さない。
	_, _ = io.WriteString(state, Sanitize(text))
}
