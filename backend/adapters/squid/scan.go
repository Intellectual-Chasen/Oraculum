package squid

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 既知の制限: 原文を 1 MiB まで保持して上限超過を read の失敗にする,
// Squid は 1 行を squidLogLineLimit の byte 数で切るため、Squid が書いた行は上限に達しない,
// 上限超過の入力が確認されたときに保持上限を見直す
const maxRecordBytes = 1 << 20

// Reader は収集元を行単位で読む。zero value に Reset して使う。
type Reader struct {
	source     *bufio.Reader
	layout     Layout
	lineNumber int64
	byteOffset int64
	done       bool
}

// Reset は入力と欄の並びを交換し、位置と読み込み終了の状態を初期化する。
//
// **並びを入力の内容から選ばない。** 呼び出し側が収集元の入力形式から渡す。
func (r *Reader) Reset(input io.Reader, layout Layout) {
	*r = Reader{layout: layout}
	if input != nil {
		r.source = bufio.NewReader(input)
	}
}

// Next は原文と位置と項目を返す。文字列の分割の失敗は failure で返し、次の行へ進める。
// 読み込みの失敗は読めた断片と failure と error を返して走査を終える。
// byte を読めなかった場合は Record の zero value を返し、位置は failure だけに載せる。
// 正常な終端は io.EOF。failure はこの層が知る項目だけを持つ。
// Squid の 1 行の上限で切れた行は、RecordTruncated の failure と、切れる前に読めた欄を持つ
// Record を返す。行の終わりまで読んだ最後の欄は Item.Truncated が真である。
func (r *Reader) Next() (Record, *core.ImportFailure, error) {
	if r.source == nil {
		return Record{}, nil, fmt.Errorf("reading Squid source: %w", errNotReset)
	}
	if r.done {
		return Record{}, nil, io.EOF
	}
	line, err := r.readLine()
	if len(line) == 0 {
		r.done = true
		if errors.Is(err, io.EOF) {
			return Record{}, nil, io.EOF
		}
		failure := failureAt(core.FailureStageRead, r.lineNumber+1, r.byteOffset,
			"a complete record within the byte limit", err.Error())
		return Record{}, failure, fmt.Errorf("reading Squid source at byte offset %d: %w", r.byteOffset, err)
	}
	r.lineNumber++
	raw, ending := splitLineEnding(line)
	record := Record{rawText: raw, lineEnding: ending, lineNumber: r.lineNumber, byteOffset: r.byteOffset}
	r.byteOffset += int64(len(line))
	if err != nil {
		r.done = true
		if !errors.Is(err, io.EOF) {
			failure := failureAt(core.FailureStageRead, r.lineNumber, r.byteOffset,
				"a complete record within the byte limit", err.Error())
			return record, failure, fmt.Errorf("reading Squid source at byte offset %d: %w", r.byteOffset, err)
		}
	}
	items, problem := tokenizeRecord(raw, r.layout)
	record.items = items
	if problem != nil {
		failure := problem.failure(record)
		if problem.offset == len(raw) && len(raw) == squidLogLineLimit && ending == "\n" {
			markTruncated(record.items, len(raw))
			failure.RecordTruncated = true
		}
		return record, failure, nil
	}
	return record, nil, nil
}

// squidLogLineLimit は Squid が access log の 1 行に書く byte 数の上限である。Squid は 1 行を
// 8192 byte の buffer に組み、収まらない行を 8191 byte で切って改行を足す (src/log/File.cc の
// logfilePrintf)。
const squidLogLineLimit = 8191

// markTruncated は、行の終わりまで読んだ最後の欄を、途中で切れた欄にする。
//
// 既知の制限: 引用符や角括弧の途中で切れた欄は欄として読めないため、その手前の欄までを残す,
// 囲んだ欄の途中で切れた行を repo の中で作る Squid の設定が無く測れない, 囲んだ欄の途中で切れた行を
// 読む必要が出たときに、囲みの途中までを値にする
func markTruncated(items []Item, end int) {
	if last := len(items) - 1; last >= 0 && int(items[last].byteOffset)+len(items[last].rawText) == end {
		items[last].truncated = true
	}
}

func (r *Reader) readLine() (string, error) {
	var line strings.Builder
	for {
		b, err := r.source.ReadByte()
		if err != nil {
			// EOF で確定した裸の CR は原文の byte として数える。
			// I/O error は上限の診断で置換せず、Next が位置を添えて返す。
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
