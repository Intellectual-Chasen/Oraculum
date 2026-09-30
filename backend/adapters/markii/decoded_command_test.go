package markii_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/markii"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 符号化されたコマンド行を持つ 2 行。
// Base64 の文字列は、UTF-16LE の文字列 decodedCommandText を符号化した値である。
const (
	encodedCommandText = "VwByAGkAdABlAC0ATwB1AHQAcAB1AHQAIABlAHgAYQBtAHAAbABlAA=="
	decodedCommandText = "Write-Output example"

	processStartWithEncodedCommandLine = "02/01/2000 03:04:05.678 +0900 loc=ja-JP " +
		"type=ITM2 sn=801100 lv=5 evt=ps subEvt=start os=Win com=\"HOST01\" " +
		"domain=\"EXAMPLE\" tmid=" + recordObservationTerminal + " " +
		"psGUID={00000000-1111-2222-3333-555555555555} " +
		"psPath=\"C:\\Windows\\System32\\example.exe\" " +
		"cmd=\"-Nop -EncodedCommand " + encodedCommandText + "\" psID=1234"

	processStartWithUndecodableCommandLine = "02/01/2000 03:04:06.678 +0900 loc=ja-JP " +
		"type=ITM2 sn=801101 lv=5 evt=ps subEvt=start os=Win com=\"HOST01\" " +
		"domain=\"EXAMPLE\" tmid=" + recordObservationTerminal + " " +
		"psGUID={00000000-1111-2222-3333-555555555555} " +
		"psPath=\"C:\\Windows\\System32\\example.exe\" " +
		"cmd=\"-Nop -EncodedCommand \" psID=1234"

	processStartWithPlainCommandLine = "02/01/2000 03:04:07.678 +0900 loc=ja-JP " +
		"type=ITM2 sn=801102 lv=5 evt=ps subEvt=start os=Win com=\"HOST01\" " +
		"domain=\"EXAMPLE\" tmid=" + recordObservationTerminal + " " +
		"psGUID={00000000-1111-2222-3333-555555555555} " +
		"psPath=\"C:\\Windows\\System32\\example.exe\" " +
		"cmd=\"-Nop -File C:\\Tools\\example.ps1\" psID=1234"
)

// fieldByName は名前で指した 1 件の項目を返す。
func fieldByName(t *testing.T, fields []core.RecordField, name string) core.RecordField {
	t.Helper()
	for _, field := range fields {
		if field.Name == name {
			return field
		}
	}
	t.Fatalf("the field %q is absent from the set", name)
	return core.RecordField{}
}

// hasFieldNamed は名前で指した項目が集合にあるかを返す。
func hasFieldNamed(fields []core.RecordField, name string) bool {
	for _, field := range fields {
		if field.Name == name {
			return true
		}
	}
	return false
}

// 符号化されたコマンド行を持つレコードは、復号した行を原資料の文字列と別の項目で持つ。
func TestParseRecordObservationCarriesTheDecodedCommandLine(t *testing.T) {
	fields := parseRecordObservationOK(t, processStartWithEncodedCommandLine).Fields

	decoded := fieldByName(t, fields, "decodedCmd")
	if decoded.Semantic != core.SemanticKeyProcessDecodedCommandLine {
		t.Errorf("semantic = %q, want %q",
			decoded.Semantic, core.SemanticKeyProcessDecodedCommandLine)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatal(err)
	}
	if decoded.Text == nil {
		t.Fatal("the decoded field carries no text value")
	}
	if decoded.Text.ValueState != core.ValueStateDerived {
		t.Errorf("valueState = %q, want %q", decoded.Text.ValueState, core.ValueStateDerived)
	}
	want := "-Nop -EncodedCommand " + decodedCommandText
	if got, ok := decoded.Text.NormalizedValue(); !ok || got != want {
		t.Errorf("normalized = %q (%t), want %q", got, ok, want)
	}
	if derivation, ok := decoded.Text.DerivationValue(); !ok || derivation == "" {
		t.Errorf("derivation = %q (%t), want the way the value was derived", derivation, ok)
	}
	// **原資料の文字列は cmd の項目に残る。** 復号した行が原資料の文字列を置き換えない。
	command := fieldByName(t, fields, "cmd")
	wantRaw := `"-Nop -EncodedCommand ` + encodedCommandText + `"`
	if got, ok := command.Text.RawTextValue(); !ok || got != wantRaw {
		t.Errorf("cmd rawText = %q (%t), want %q", got, ok, wantRaw)
	}
	if got, ok := command.Text.NormalizedValue(); !ok ||
		got != "-Nop -EncodedCommand "+encodedCommandText {
		t.Errorf("cmd normalized = %q (%t), want the source lexeme without the quotes", got, ok)
	}
}

// 復号できない文字列を宣言するレコードは、導出未確定の項目を持ち、原資料の文字列を残す。
func TestParseRecordObservationLeavesTheUndecodableCommandLineUndetermined(t *testing.T) {
	fields := parseRecordObservationOK(t, processStartWithUndecodableCommandLine).Fields

	decoded := fieldByName(t, fields, "decodedCmd")
	if err := decoded.Validate(); err != nil {
		t.Fatal(err)
	}
	if decoded.Text == nil {
		t.Fatal("the decoded field carries no text value")
	}
	if decoded.Text.ValueState != core.ValueStateDerivationUndetermined {
		t.Errorf("valueState = %q, want %q",
			decoded.Text.ValueState, core.ValueStateDerivationUndetermined)
	}
	if _, present := decoded.Text.NormalizedValue(); present {
		t.Error("the undetermined field carries a normalized value")
	}
	if derivation, ok := decoded.Text.DerivationValue(); !ok || derivation == "" {
		t.Errorf("derivation = %q (%t), want the reason the value is undetermined",
			derivation, ok)
	}
	command := fieldByName(t, fields, "cmd")
	if got, ok := command.Text.RawTextValue(); !ok || got != `"-Nop -EncodedCommand "` {
		t.Errorf("cmd rawText = %q (%t), want the source lexeme", got, ok)
	}
}

// 符号化された文字列を持たないコマンド行のレコードは、復号の項目を持たない。
func TestParseRecordObservationAddsNoDecodedFieldForAPlainCommandLine(t *testing.T) {
	fields := parseRecordObservationOK(t, processStartWithPlainCommandLine).Fields

	if hasFieldNamed(fields, "decodedCmd") {
		t.Error("the record carries a decoded field for a command line without an encoded lexeme")
	}
	assertSemanticOfField(t, fields, "cmd", core.SemanticKeyProcessCommandLine)
}

// cmd の key を持たないレコードは、復号の項目を持たない。
func TestParseRecordObservationAddsNoDecodedFieldWithoutTheCommandLine(t *testing.T) {
	fields := parseRecordObservationOK(t, fileCloseLine).Fields

	if hasFieldNamed(fields, "decodedCmd") {
		t.Error("the record carries a decoded field without the cmd key")
	}
}

// ORACULUM_MARKII_SOURCE_DIR が指す入力の全レコードで、復号の項目が 4 つの性質を満たす。
// opt-in の検査である。
//
//  1. 項目が単独で Validate を通る
//  2. 導出済みの項目が正規化値を持ち、その値が cmd の原資料の文字列と別である
//  3. 導出未確定の項目が正規化値を持たない
//  4. 導出済みの正規化値が ASCII の印字可能文字と改行と水平タブだけから成る
//
// 4 が、符号化されていない文字列を復号して捏造した値を持つ経路を防ぐ。
// Base64 の文字だけから成る英単語と 16 進は、UTF-16LE として復号すると CJK になる。
//
// **件数を期待値に持たない。** 数えた値は t.Log が出す。
func TestDecodedCommandFieldOverTheConfiguredSource(t *testing.T) {
	logDir := sourceDir(t)
	entries, err := filepath.Glob(filepath.Join(logDir, "*.log"))
	if err != nil {
		t.Fatalf("listing the logs in %s: %v", logDir, err)
	}
	if len(entries) == 0 {
		t.Fatalf("no log file found in %s", logDir)
	}
	derived, undetermined := 0, 0
	for _, path := range entries {
		readDecodedCommandsOfOneFile(t, path, &derived, &undetermined)
	}
	t.Logf("files=%d derived=%d undetermined=%d", len(entries), derived, undetermined)
}

// readDecodedCommandsOfOneFile は 1 file の全レコードの復号の項目を確かめる。
func readDecodedCommandsOfOneFile(t *testing.T, path string, derived, undetermined *int) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("closing %s: %v", path, err)
		}
	}()

	var reader markii.Reader
	reader.Reset(file)
	for {
		record, tokenizeFailure, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil || tokenizeFailure != nil {
			continue
		}
		observation, failure := markii.ParseRecordObservation(record)
		if failure != nil {
			continue
		}
		if !hasFieldNamed(observation.Fields, "decodedCmd") {
			continue
		}
		checkDecodedCommandField(t, path, record.LineNumber(), observation.Fields,
			derived, undetermined)
	}
}

// checkDecodedCommandField は 1 件の復号の項目の 3 つの性質を確かめる。
func checkDecodedCommandField(
	t *testing.T,
	path string,
	lineNumber int64,
	fields []core.RecordField,
	derived, undetermined *int,
) {
	t.Helper()
	decoded := fieldByName(t, fields, "decodedCmd")
	if err := decoded.Validate(); err != nil {
		t.Errorf("%s line %d: %v", path, lineNumber, err)
		return
	}
	if decoded.Text == nil {
		t.Errorf("%s line %d: the decoded field carries no text value", path, lineNumber)
		return
	}
	normalized, present := decoded.Text.NormalizedValue()
	switch decoded.Text.ValueState {
	case core.ValueStateDerived:
		*derived++
		if !present || normalized == "" {
			t.Errorf("%s line %d: the derived field carries no normalized value",
				path, lineNumber)
			return
		}
		command := fieldByName(t, fields, "cmd")
		if raw, ok := command.Text.RawTextValue(); ok && raw == normalized {
			t.Errorf("%s line %d: the decoded line equals the source lexeme of cmd",
				path, lineNumber)
		}
		// 符号化されていない文字列を復号した値を通さない。
		for _, r := range normalized {
			if r == '\n' || r == '\r' || r == '\t' {
				continue
			}
			if r < ' ' || r > '~' {
				t.Errorf("%s line %d: the decoded line carries %q outside printable ASCII",
					path, lineNumber, r)
				break
			}
		}
	case core.ValueStateDerivationUndetermined:
		*undetermined++
		if present {
			t.Errorf("%s line %d: the undetermined field carries a normalized value",
				path, lineNumber)
		}
	default:
		t.Errorf("%s line %d: valueState = %q, want derived or derivation_undetermined",
			path, lineNumber, decoded.Text.ValueState)
	}
}
