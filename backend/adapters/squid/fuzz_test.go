package squid_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/squid"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// FuzzTokenizeRecord は原文復元、項目の byte 範囲、panic が無いことを確かめる。
func FuzzTokenizeRecord(f *testing.F) {
	for _, seed := range []string{exampleLine, "", "\n", "\r\n", `192.0.2.8 - - [bad] "unterminated`, strings.Replace(exampleLine, `"agent"`, `"a\"b\\c\t"`, 1)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		var reader squid.Reader
		reader.Reset(strings.NewReader(input), squid.LayoutCombined)
		r, failure, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return
		}
		checkItems(t, r, err == nil && failure == nil)
		if err != nil && failure == nil {
			t.Fatal("read error without diagnosis")
		}
		if failure == nil {
			stamp, problem := squid.ParseRequestTime(r)
			if problem == nil && stamp.Validate() != nil {
				t.Fatal("invalid successful timestamp")
			}
			request, problem := squid.ParseRequestLine(r)
			item, _ := r.Item(squid.ItemRequestLine)
			if problem == nil && request.Authority != nil && !strings.Contains(item.Value(), *request.Authority) {
				t.Fatal("authority not sourced from target")
			}
		}
	})
}

// FuzzReader は行番号、byte offset、改行を含む原文の復元を確かめる。
// 上限で停止した場合は読めた断片までが入力の接頭辞であることを確かめる。
func FuzzReader(f *testing.F) {
	for _, seed := range []string{exampleLine, "", "\n", "\r\n", exampleLine + "\n" + exampleLine + "\r\n" + exampleLine, strings.Repeat("a", (1<<20)+1)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		var reader squid.Reader
		reader.Reset(strings.NewReader(input), squid.LayoutCombined)
		var restored strings.Builder
		for line := int64(1); ; line++ {
			r, failure, err := reader.Next()
			if errors.Is(err, io.EOF) {
				if restored.String() != input {
					t.Fatal("records do not reconstruct input")
				}
				return
			}
			if r.LineNumber() != line || r.ByteOffset() != int64(restored.Len()) {
				t.Fatal("record position differs")
			}
			restored.WriteString(r.RawText())
			restored.WriteString(r.LineEnding())
			if !strings.HasPrefix(input, restored.String()) {
				t.Fatal("record text differs from input")
			}
			checkItems(t, r, err == nil && failure == nil)
			if err != nil {
				checkFailure(t, failure, core.FailureStageRead, line, failureDetails{"a complete record within the byte limit", "squid: record exceeds the byte limit", int64(restored.Len())})
				if len(r.RawText()) <= 1<<20 {
					t.Fatal("bounded string source failed below limit")
				}
				if _, _, nextErr := reader.Next(); !errors.Is(nextErr, io.EOF) {
					t.Fatal("read failure did not stop reader")
				}
				return
			}
		}
	})
}
