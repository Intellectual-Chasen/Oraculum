package markii_test

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/markii"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// sourceDirVar は opt-in の検査が読む入力の directory を指す環境変数の名前である。
//
// 値が無いときは repo root の .env から同じ名前を読む。.env は git の管理対象外である
// (.gitignore)。どちらにも無ければ検査を skip する。
const sourceDirVar = "ORACULUM_MARKII_SOURCE_DIR"

// ORACULUM_MARKII_SOURCE_DIR が指す入力の全レコードで、文字列の分割が失敗せず、組み直すと
// 原文に戻る。
//
// **件数や key の種類数を期待値に持たない。** 本 test が確かめるのは入力に依らない 4 つである。
//
//  1. 走査が読み込みの失敗で終わらない
//  2. 末尾の単独 DOS EOF marker は原文と位置を持つ入力不整合の診断であり、それ以外の
//     レコードは文字列の分割の失敗を返さない
//  3. どのレコードも field 列から組み直すと原文に戻る
//  4. field の位置が原文の範囲に収まり、その範囲の原文が RawValue と一致する
//
// 数えた値は t.Log が出し、assert しない。
func TestTokenizeReadsTheConfiguredSource(t *testing.T) {
	logDir := sourceDir(t)
	entries, err := filepath.Glob(filepath.Join(logDir, "*.log"))
	if err != nil {
		t.Fatalf("listing the logs in %s: %v", logDir, err)
	}
	if len(entries) == 0 {
		t.Fatalf("no log file found in %s", logDir)
	}

	records, longest, markers := 0, 0, 0
	keys := map[string]struct{}{}
	for _, path := range entries {
		readOneSourceFile(t, path, &records, &longest, &markers, keys)
	}
	// 1 件も読めていない状態を成功にしない。
	if records == 0 {
		t.Fatalf("no record read from %d file(s) in %s", len(entries), logDir)
	}
	t.Logf("files=%d records=%d diagnosedEOFMarkers=%d keyKinds=%d longestRecordBytes=%d",
		len(entries), records, markers, len(keys), longest)
}

// readOneSourceFile は 1 file を走査し、4 つの性質を確かめる。
func readOneSourceFile(t *testing.T, path string, records, longest, markers *int, keys map[string]struct{}) {
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
		record, failure, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			// 1. 走査が読み込みの失敗で終わらない。
			t.Fatalf("%s line %d: reading stopped: %v", path, record.LineNumber(), err)
		}
		if failure != nil {
			if record.RawText() == "\x1a" && failure.DiagnosisClass == core.DiagnosisClassInconsistentInputConfirmed {
				*markers++
				if failure.Stage != core.FailureStageTokenize || failure.UnresolvedReason != "" {
					t.Errorf("%s: the trailing marker must carry a confirmed tokenization diagnosis", path)
				}
				if failure.LineNumber == nil || *failure.LineNumber != record.LineNumber() ||
					failure.ByteOffset == nil || *failure.ByteOffset != record.ByteOffset() {
					t.Errorf("%s: the trailing marker diagnosis must retain its record position", path)
				}
				if _, nextFailure, nextErr := reader.Next(); !errors.Is(nextErr, io.EOF) || nextFailure != nil {
					t.Errorf("%s: the diagnosed DOS EOF marker must be at the physical end of the input", path)
				}
				return
			}
			// 既知の末尾 marker 以外の失敗は許容しない。
			t.Errorf("%s line %d: the tokenizer reported a failure: expected %s, observed %s",
				path, record.LineNumber(), failure.ExpectedMeaning, failure.ObservedResult)
			continue
		}
		*records++

		rawText := record.RawText()
		if len(rawText) > *longest {
			*longest = len(rawText)
		}
		// 3. 組み直すと原文に戻る。
		restored, hasHeader := restoreOf(record)
		if !hasHeader {
			t.Errorf("%s line %d: the record must carry its header", path, record.LineNumber())
			continue
		}
		if restored != rawText {
			t.Errorf("%s line %d: the restored text differs from the original text",
				path, record.LineNumber())
			continue
		}
		// 4. field の位置が原文の範囲に収まる。
		if problem := offsetProblemOf(record); problem != "" {
			t.Errorf("%s line %d: %s", path, record.LineNumber(), problem)
		}
		for _, field := range record.Fields() {
			keys[field.Key()] = struct{}{}
		}
	}
}

// sourceDir は ORACULUM_MARKII_SOURCE_DIR の値を返す。環境変数に無ければ repo root の .env を
// 読み、どちらにも無ければ skip する。
func sourceDir(t *testing.T) string {
	t.Helper()
	if value := os.Getenv(sourceDirVar); value != "" {
		return value
	}
	if value := valueFromDotEnv(t, sourceDirVar); value != "" {
		return value
	}
	t.Skip(sourceDirVar + " が設定されていないため飛ばす")
	return ""
}

// valueFromDotEnv は repo root の .env から 1 つの名前の値を読む。
//
// 行の形は NAME=value である。# で始まる行と空行を飛ばす。値の前後の引用符を外す。
//
// **file が無いことだけを許す。** .env は利用者が置くもので、無いことが既定である。
// 権限が無いなどの他の失敗は、設定が無いのと同じ skip にすると原因が見えなくなる
// ので、test を止める。
func valueFromDotEnv(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(repoRoot(t), ".env")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("closing %s: %v", path, err)
		}
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found || strings.TrimSpace(key) != name {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		return value
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return ""
}

// repoRoot は test の作業ディレクトリから repo root を探す。
//
// test は package の directory で走る。go.mod を持つ directory の 1 つ上が repo root
// である (backend/go.mod)。
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("reading the working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Dir(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod found above the working directory")
		}
		dir = parent
	}
}
