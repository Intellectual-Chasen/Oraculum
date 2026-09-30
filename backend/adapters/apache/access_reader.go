package apache

import (
	"errors"
	"fmt"
	"io"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// AccessReader は combined のアクセスログを行単位で読む。zero value に Reset して使う。
type AccessReader struct {
	scanner lineScanner
}

// Reset は入力を差し替え、走査を先頭に戻す。
func (r *AccessReader) Reset(input io.Reader) {
	r.scanner.reset(input)
}

// Next は原文と位置と項目を返す。文字列の分割の失敗は failure で返し、次の行へ進める。
// 読み込みの失敗は読めた断片と failure と error を返して走査を終える。
func (r *AccessReader) Next() (AccessRecord, *core.ImportFailure, error) {
	raw, ending, lineNumber, byteOffset, hasLine, err := r.scanner.rawLine()
	if errors.Is(err, errNotReset) {
		return AccessRecord{}, nil, fmt.Errorf("reading Apache access source: %w", err)
	}
	if !hasLine {
		if errors.Is(err, io.EOF) {
			return AccessRecord{}, nil, io.EOF
		}
		failure := failureAt(core.FailureStageRead, FormatKeyAccessCombined, lineNumber, byteOffset,
			"a complete record within the byte limit", err.Error())
		return AccessRecord{}, failure, fmt.Errorf("reading Apache access source at byte offset %d: %w", byteOffset, err)
	}
	record := AccessRecord{rawText: raw, lineEnding: ending, lineNumber: lineNumber, byteOffset: byteOffset}
	if err != nil && !errors.Is(err, io.EOF) {
		failure := failureAt(core.FailureStageRead, FormatKeyAccessCombined, lineNumber, byteOffset,
			"a complete record within the byte limit", err.Error())
		return record, failure, fmt.Errorf("reading Apache access source at byte offset %d: %w", byteOffset, err)
	}
	items, problem := tokenizeAccessLine(raw)
	record.items = items
	if problem != nil {
		return record, problem.failure(record), nil
	}
	return record, nil, nil
}
