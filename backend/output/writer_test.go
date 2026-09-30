package output_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/output"
)

func TestWriters(t *testing.T) {
	tests := []struct {
		name  string
		write func(io.Writer, ...any) (int, error)
		args  []any
		want  string
	}{
		{"formatted", func(w io.Writer, args ...any) (int, error) {
			return output.Fprintf(w, "value=%s count=%d valid=%t\n", args...)
		}, []any{"日\n\r\x00\x1b %s%%", 7, true}, "value=日\\x0a\\x0d\\x00\\x1b %s%% count=7 valid=true\n"},
		{"line", output.Fprintln, []any{"日\n\r\x00\x1b %s%%", 7, true}, "日\\x0a\\x0d\\x00\\x1b %s%% 7 true\n"},
		{"empty_line", output.Fprintln, nil, "\n"},
		{"literal_format", func(w io.Writer, args ...any) (int, error) {
			return output.Fprintf(w, "literal\t100%%\n", args...)
		}, nil, "literal\t100%\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := append([]any(nil), tt.args...)
			var buf bytes.Buffer
			n, err := tt.write(&buf, tt.args...)
			if err != nil || buf.String() != tt.want || n != len(tt.want) {
				t.Fatalf("write = (%d, %v, %q), want (%d, nil, %q)", n, err, buf.String(), len(tt.want), tt.want)
			}
			for i, arg := range tt.args {
				if arg != original[i] {
					t.Fatalf("argument %d changed: %v", i, arg)
				}
			}
		})
	}
}

type failingWriter struct {
	received []byte
	written  []byte
	err      error
	limit    int
}

func (w *failingWriter) Write(p []byte) (int, error) {
	w.received = append(w.received, p...)
	w.written = append(w.written, p[:w.limit]...)
	return w.limit, w.err
}

func TestWriterErrors(t *testing.T) {
	cause := errors.New("destination unavailable")
	tests := []struct {
		name          string
		write         func(io.Writer) (int, error)
		want, context string
	}{
		{"formatted", func(w io.Writer) (int, error) { return output.Fprintf(w, "%s", "a\nb") }, `a\x0ab`, "output: writing formatted text: "},
		{"line", func(w io.Writer) (int, error) { return output.Fprintln(w, "a\nb") }, "a\\x0ab\n", "output: writing line: "},
	}
	for _, tt := range tests {
		for _, limit := range []int{0, 2} {
			t.Run(tt.name+"/written_"+strconv.Itoa(limit), func(t *testing.T) {
				w := &failingWriter{err: cause, limit: limit}
				n, err := tt.write(w)
				if !errors.Is(err, cause) || err.Error() != tt.context+cause.Error() {
					t.Fatalf("error = %v, want wrapped cause with context %q", err, tt.context)
				}
				if n != len(w.written) || n != limit || string(w.written) != tt.want[:limit] {
					t.Fatalf("written = (%d, %q), want (%d, %q)", n, w.written, limit, tt.want[:limit])
				}
				if string(w.received) != tt.want {
					t.Fatalf("writer received %q, want %q", w.received, tt.want)
				}
			})
		}
	}
}

type namedText string

type controlStringer struct{}

func (controlStringer) String() string { return "text\n\x1b" }

func TestFprintfSanitizesFormattedValues(t *testing.T) {
	tests := []struct {
		name, format string
		value        any
		want         string
	}{
		{"error", "%v", errors.New("read\nfailed"), `read\x0afailed`},
		{"stringer", "%v", controlStringer{}, `text\x0a\x1b`},
		{"named_string", "%s", namedText("text\n"), `text\x0a`},
		{"bytes", "%s", []byte("text\n"), `text\x0a`},
		{"decimal", "%d", 42, "42"},
		{"boolean", "%t", true, "true"},
		{"quoted", "%q", "text\n", `"text\n"`},
		{"width_flag", "%+06d", 42, "+00042"},
		{"precision", "%8.2f", 1.25, "    1.25"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			n, err := output.Fprintf(&buf, tt.format, tt.value)
			if err != nil || buf.String() != tt.want || n != len(tt.want) {
				t.Fatalf("Fprintf = (%d, %v, %q), want (%d, nil, %q)", n, err, buf.String(), len(tt.want), tt.want)
			}
		})
	}
}

func TestFprintlnSanitizesFormattedValues(t *testing.T) {
	var buf bytes.Buffer
	n, err := output.Fprintln(&buf, errors.New("read\nfailed"), controlStringer{}, namedText("name\r"), []byte("a\n"), 42, true)
	want := "read\\x0afailed text\\x0a\\x1b name\\x0d [97 10] 42 true\n"
	if err != nil || buf.String() != want || n != len(want) {
		t.Fatalf("Fprintln = (%d, %v, %q), want (%d, nil, %q)", n, err, buf.String(), len(want), want)
	}
}

func TestFprintfPreservesArgumentDirectives(t *testing.T) {
	tests := []struct {
		name, format string
		args         []any
		want         string
	}{
		{"dynamic_width", "%*s", []any{5, "abc"}, "  abc"},
		{"dynamic_precision", "%.*f", []any{2, 1.25}, "1.25"},
		{"wrapped_type", "%T", []any{"abc"}, "output.sanitizingArg"},
		{"numeric_type", "%T", []any{42}, "int"},
		{"boolean_type", "%T", []any{true}, "bool"},
		{"nil", "%v", []any{nil}, "<nil>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			n, err := output.Fprintf(&buf, tt.format, tt.args...)
			if err != nil || buf.String() != tt.want || n != len(tt.want) {
				t.Fatalf("Fprintf = (%d, %v, %q), want (%d, nil, %q)", n, err, buf.String(), len(tt.want), tt.want)
			}
		})
	}
}

func TestFprintfPointerDiagnosticIsSanitized(t *testing.T) {
	var buf bytes.Buffer
	n, err := output.Fprintf(&buf, "%p", "text\n\x1b")
	if err != nil || n != buf.Len() {
		t.Fatalf("Fprintf = (%d, %v), wrote %d bytes", n, err, buf.Len())
	}
	if strings.ContainsFunc(buf.String(), unicode.IsControl) {
		t.Fatalf("control character in pointer diagnostic: %q", buf.String())
	}
	if !strings.HasPrefix(buf.String(), "0x") {
		t.Fatalf("wrapper function address = %q, want hexadecimal address", buf.String())
	}
	if _, err := strconv.ParseUint(strings.TrimPrefix(buf.String(), "0x"), 16, 64); err != nil {
		t.Fatalf("invalid wrapper function address %q: %v", buf.String(), err)
	}
}

func TestWriteJSONPreservesControlCharacters(t *testing.T) {
	for _, raw := range []string{"plain 日本語", "a\u0085b\u009bc\x01d\x7f", `literal \u0085 and \x85`, "\t\r\n"} {
		t.Run(raw, func(t *testing.T) {
			var buffer bytes.Buffer
			failure := core.ImportFailure{ObservedResult: raw}
			if err := output.WriteJSON(&buffer, failure); err != nil {
				t.Fatal(err)
			}
			if strings.ContainsFunc(strings.TrimSuffix(buffer.String(), "\n"), unicode.IsControl) {
				t.Errorf("JSON has controls: %q", buffer.String())
			}
			var decoded struct {
				ObservedResult string `json:"observedResult"`
			}
			if err := json.Unmarshal(buffer.Bytes(), &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.ObservedResult != raw {
				t.Errorf("decoded=%q want=%q", decoded.ObservedResult, raw)
			}
		})
	}
}

// 部分ごとに書いた値は、同じ値を WriteJSON で書いた文字列と同じである。
func TestJSONStreamWritesWhatWriteJSONWrites(t *testing.T) {
	failures := []core.ImportFailure{{ObservedResult: "a\u0085b\x01"}, {ObservedResult: "plain 日本語"}}
	var whole bytes.Buffer
	if err := output.WriteJSON(&whole, map[string]any{"count": 2, "failures": failures}); err != nil {
		t.Fatal(err)
	}
	var streamed bytes.Buffer
	stream := output.NewJSONStream(&streamed)
	stream.Token(`{"count":`)
	stream.Value(2)
	stream.Token(`,"failures":`)
	output.WriteJSONArray(stream, failures)
	stream.Token("}")
	if err := stream.End(); err != nil {
		t.Fatal(err)
	}
	if streamed.String() != whole.String() {
		t.Errorf("the stream wrote %q, WriteJSON writes %q", streamed.String(), whole.String())
	}
}

// 最初の失敗の後は何も書かず、End がその失敗を返す。
func TestJSONStreamReportsTheFirstFailure(t *testing.T) {
	var buffer bytes.Buffer
	stream := output.NewJSONStream(&buffer)
	stream.Token("[")
	stream.Value(make(chan int))
	stream.Token("]")
	if err := stream.End(); err == nil || buffer.Len() != 0 {
		t.Fatalf("an unencodable value gave error=%v bytes=%q", err, buffer.Bytes())
	}
	controlled := output.NewJSONStream(&buffer)
	controlled.Token("[\x01")
	if err := controlled.End(); err == nil {
		t.Error("a token carrying a control character was written")
	}
	cause := errors.New("JSON output unavailable")
	failed := output.NewJSONStream(&failingWriter{err: cause, limit: 0})
	failed.Value("a")
	if err := failed.End(); !errors.Is(err, cause) {
		t.Errorf("the write error=%v", err)
	}
}

type shortJSONWriter struct{ received []byte }

func (w *shortJSONWriter) Write(data []byte) (int, error) {
	w.received = append(w.received, data...)
	return len(data) - 1, nil
}

func TestWriteJSONFailures(t *testing.T) {
	var buffer bytes.Buffer
	if err := output.WriteJSON(&buffer, make(chan int)); err == nil || buffer.Len() != 0 {
		t.Fatalf("encoding error=%v bytes=%q", err, buffer.Bytes())
	}
	cause := errors.New("JSON output unavailable")
	failed := &failingWriter{err: cause, limit: 0}
	if err := output.WriteJSON(failed, "a\u0085"); !errors.Is(err, cause) {
		t.Fatalf("write error=%v", err)
	}
	want := "\"a\\u0085\"\n"
	if string(failed.received) != want {
		t.Fatalf("writer received=%q want=%q", failed.received, want)
	}
	short := &shortJSONWriter{}
	if err := output.WriteJSON(short, "a\u0085"); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write error=%v", err)
	}
	if string(short.received) != want {
		t.Fatalf("short writer received=%q want=%q", short.received, want)
	}
}
