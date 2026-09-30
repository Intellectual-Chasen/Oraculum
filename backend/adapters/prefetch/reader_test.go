// in-package test: 1 つの file を 1 件のレコードとして返す。
package prefetch

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 読めた file はレコードを 1 件返し、原文は file の全体の 16 進である。次は io.EOF を返す。
func TestReaderReturnsOneRecordPerFile(t *testing.T) {
	content := compressedFile(sampleFile(30, 0x130).build(), false)
	var reader Reader
	reader.Reset(bytes.NewReader(content))
	record, failure, err := reader.Next()
	if failure != nil || err != nil || record.File.ExecutableName != "TOOL51.EXE" ||
		record.ByteLength != int64(len(content)) || !strings.HasPrefix(record.RawText, "4D414D04") ||
		len(record.RawText) != 2*len(content) {
		t.Fatalf("record %+v, failure %+v, err %v", record, failure, err)
	}
	if _, _, err := reader.Next(); !errors.Is(err, io.EOF) {
		t.Errorf("the second Next = %v, want io.EOF", err)
	}
}

// 解釈できない file は、原文を持つ Tokenize の失敗を返し、走査を止めない。
func TestReaderReportsAnUninterpretableFile(t *testing.T) {
	const input = "not a prefetch file"
	var reader Reader
	reader.Reset(strings.NewReader(input))
	record, failure, err := reader.Next()
	if err != nil || failure == nil || failure.Stage != core.FailureStageTokenize ||
		record.RawText != strings.ToUpper(hex.EncodeToString([]byte(input))) || record.ByteLength != int64(len(input)) {
		t.Fatalf("record %+v, failure %+v, err %v", record, failure, err)
	}
	if *failure.ByteOffset != 0 || failure.ObservedResult == "" {
		t.Errorf("failure %+v, want the offset 0 and the reason", failure)
	}
}

// 上限を超える file と、Reset を呼ばない走査は、読み取りの失敗である。
func TestReaderRejectsOversizedFilesAndMissingReset(t *testing.T) {
	var reader Reader
	reader.Reset(bytes.NewReader(make([]byte, maxFileBytes+1)))
	if _, failure, err := reader.Next(); !errors.Is(err, errFileTooLarge) || failure == nil ||
		failure.Stage != core.FailureStageRead {
		t.Errorf("an oversized file: failure %+v, err %v", failure, err)
	}
	var unset Reader
	if _, _, err := unset.Next(); !errors.Is(err, errNotReset) {
		t.Errorf("Next without Reset = %v", err)
	}
}

// 形式の識別子は、利用者が起動引数に書く文字列である。pipeline の test も同じ文字列を持つ。
func TestFormatKeyIsTheArgumentText(t *testing.T) {
	if FormatKeyPrefetch != "windows_prefetch" || Formats()[0].PositionKind != core.PositionKindByteRange {
		t.Errorf("Formats() = %+v", Formats())
	}
}
