package apache

import (
	"errors"
	"fmt"
	"io"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// ErrorReader はエラーログを行単位で読む。zero value に Reset して使う。
type ErrorReader struct {
	scanner lineScanner
}

// Reset は入力を差し替え、走査を先頭に戻す。
func (r *ErrorReader) Reset(input io.Reader) {
	r.scanner.reset(input)
}

// Next は原文と位置と項目を返す。文字列の分割の失敗は failure で返し、次の行へ進める。
// 読み込みの失敗は読めた断片と failure と error を返して走査を終える。
func (r *ErrorReader) Next() (ErrorRecord, *core.ImportFailure, error) {
	raw, ending, lineNumber, byteOffset, hasLine, err := r.scanner.rawLine()
	if errors.Is(err, errNotReset) {
		return ErrorRecord{}, nil, fmt.Errorf("reading Apache error source: %w", err)
	}
	if !hasLine {
		if errors.Is(err, io.EOF) {
			return ErrorRecord{}, nil, io.EOF
		}
		failure := failureAt(core.FailureStageRead, FormatKeyError, lineNumber, byteOffset,
			"a complete record within the byte limit", err.Error())
		return ErrorRecord{}, failure, fmt.Errorf("reading Apache error source at byte offset %d: %w", byteOffset, err)
	}
	record := ErrorRecord{rawText: raw, lineEnding: ending, lineNumber: lineNumber, byteOffset: byteOffset}
	if err != nil && !errors.Is(err, io.EOF) {
		failure := failureAt(core.FailureStageRead, FormatKeyError, lineNumber, byteOffset,
			"a complete record within the byte limit", err.Error())
		return record, failure, fmt.Errorf("reading Apache error source at byte offset %d: %w", byteOffset, err)
	}
	items, problem := tokenizeErrorLine(raw)
	record.items = items
	if problem != nil {
		return record, problem.failure(record), nil
	}
	return record, nil, nil
}
