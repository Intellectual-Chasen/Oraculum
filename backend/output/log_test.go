package output_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/output"
)

// TestNewSanitizingLoggerKeepsTheRecordTime は、記録が起きた時刻が記録に残ることを
// 確かめる。無害化の対象は文字列と error の値だけであり、時刻を除かない。
func TestNewSanitizingLoggerKeepsTheRecordTime(t *testing.T) {
	var buf bytes.Buffer
	record := slog.NewRecord(time.Unix(0, 0).UTC(), slog.LevelInfo, "probe", 0)
	if err := output.NewSanitizingLogger(&buf).Handler().Handle(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), "time=1970-01-01T00:00:00.000Z ") {
		t.Fatalf("line=%q", buf.String())
	}
}

// TestNewSanitizingLoggerAttributeKinds は、属性の種別ごとに記録へ出る表記を確かめる。
// 文字列と error だけが無害化の対象で、数と真偽値と nil は値のまま出る。
func TestNewSanitizingLoggerAttributeKinds(t *testing.T) {
	tests := []struct {
		name string
		attr slog.Attr
		want string
	}{
		{"string", slog.String("value", "a\n\x1b"), `value=a\x0a\x1b`},
		{"error", slog.Any("error", errors.New("read\r\nfailed")), `error=read\x0d\x0afailed`},
		{"invalid utf8", slog.String("value", "日本語\xc2\x85\xff"), `value=日本語\xc2\x85\xff`},
		{"plain", slog.String("value", "日本語"), "value=日本語"},
		{"integer", slog.Int("count", 4), "count=4"},
		{"boolean", slog.Bool("valid", true), "valid=true"},
		{"nil", slog.Any("value", nil), "value=<nil>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			output.NewSanitizingLogger(&buf).Info("probe", tt.attr)
			if !strings.Contains(buf.String(), tt.want) {
				t.Fatalf("line=%q does not carry %q", buf.String(), tt.want)
			}
		})
	}
}

// TestNewSanitizingLoggerKeepsAMessageOnOneLine は、外部由来の文字列が message に入った
// 記録が 1 行に収まることを確かめる。slog の既定の logger は属性の値と key を quote する
// 一方、message を与えられたまま書く。
func TestNewSanitizingLoggerKeepsAMessageOnOneLine(t *testing.T) {
	var buf bytes.Buffer
	output.NewSanitizingLogger(&buf).Error("reading \"markii.log\"\nERROR the import succeeded\x1b[0m",
		"error", errors.New("line 3\r\nlevel=INFO msg=forged"))
	line := buf.String()
	if strings.Count(line, "\n") != 1 || !strings.HasSuffix(line, "\n") {
		t.Fatalf("record must occupy one line: %q", line)
	}
	for _, want := range []string{
		"level=ERROR",
		`msg="reading \"markii.log\"\\x0aERROR the import succeeded\\x1b[0m"`,
		`error="line 3\\x0d\\x0alevel=INFO msg=forged"`,
	} {
		if !strings.Contains(line, want) {
			t.Errorf("line=%q does not carry %q", line, want)
		}
	}
}

// TestNewSanitizingLoggerSanitizesGroupsAndWith は、group の中の属性と With で先に付けた
// 属性も無害化を通ることを確かめる。
func TestNewSanitizingLoggerSanitizesGroupsAndWith(t *testing.T) {
	var buf bytes.Buffer
	output.NewSanitizingLogger(&buf).With(slog.String("source", "file\nname")).
		Info("read result", slog.Group("diagnostic",
			slog.String("value", "日本語\xc2\x85\xff"),
			slog.Any("error", errors.New("bad\r\ninput")),
			slog.Int("count", 3), slog.Bool("valid", false)))
	line := buf.String()
	if strings.Count(line, "\n") != 1 || !strings.HasSuffix(line, "\n") {
		t.Fatalf("record must occupy one line: %q", line)
	}
	for _, want := range []string{
		`source=file\x0aname`,
		`diagnostic.value=日本語\xc2\x85\xff`,
		`diagnostic.error=bad\x0d\x0ainput`,
		"diagnostic.count=3",
		"diagnostic.valid=false",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("line=%q does not carry %q", line, want)
		}
	}
}
