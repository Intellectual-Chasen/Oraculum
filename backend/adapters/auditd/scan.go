package auditd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// maxLineBytes は 1 行の byte 数の上限である。
//
// 既知の制限: 1 行を 1 MiB までとする,
// ORACULUM_AUDITD_SOURCE を設定した opt-in の検査 (source_test.go) が収集元を端まで走査し、
// 最長の行の長さを t.Log に出す,
// 上限に達した失敗が 1 件でも出たときに、その入力の最大長の上を取る値へ直す
const maxLineBytes = 1 << 20

// eventLineWindow は、1 つの事象の行が現れる範囲として見込む行数である。
//
// **同じ事象の行が連続しない。** 実行が並ぶ区間では、別の事象の行が間に入る。
// 先頭の行からこの行数だけ後ろまでを見て、それより後に現れた同じ鍵の行は別の
// レコードになる。
//
// 既知の制限: 1 つの事象の行を先頭から 256 行の範囲で集める,
// 間隔がこの範囲を超えると同じ鍵の事象が 2 レコードに分かれ、ORACULUM_AUDITD_SOURCE を
// 設定した opt-in の検査 (source_test.go) がレコードの数と鍵の異なりの数の不一致として報告する,
// 間隔がこの範囲を超える収集元が 1 件でも出たときに、その入力の最大の上を取る値へ直す
const eventLineWindow = 256

// Reader は 1 つの収集元の byte 列をレコードごとに読む。
//
// **同じ事象の行を 1 レコードにまとめる。** 1 つの事象が複数行に分かれ、同じ事象の行は
// `msg=audit(...)` の値が一致する。
// レコードを返す順は、事象の先頭の行が現れた順である。
//
// file を開かない。path を持たない。収集元を開き、閉じ、SourceIdentity を組むのは
// 取り込みの実行である。
//
// zero value に Reset して使う。
type Reader struct {
	source     *bufio.Reader
	lineNumber int64
	byteOffset int64
	done       bool
	// open は行を集めている途中の事象である。先頭の行が現れた順に並ぶ。
	open []*recordBuilder
	// ready は集め終えたレコードである。返す順に並ぶ。
	ready []Record
	// emitted は 1 件以上のレコードを返したかである。収集元の先頭の判定に使う。
	emitted bool
}

// Reset は読む対象を入れ替える。行番号と byte offset を 0 に戻す。
//
// bufio.Scanner を使わない。既定の token の上限 64 KiB を超えた行で走査が静かに終わる。
func (r *Reader) Reset(input io.Reader) {
	r.source = bufio.NewReader(input)
	r.lineNumber = 0
	r.byteOffset = 0
	r.done = false
	r.open = nil
	r.ready = nil
	r.emitted = false
}

// Next は次のレコードへ進む。
//
// 返り値の 3 つは次の組み合わせを取る。
//
//	正常                     : record が完成した値、failure が nil、err が nil
//	文字列の分割の失敗           : record が失敗した 1 行だけの値、failure が診断、err が nil
//	収集元の先頭が切れている : record が zero value、failure が読めなかった範囲の診断、
//	                           err が nil。切れている事象そのものは次の Next が返す
//	収集元の末尾             : record が zero value、failure が nil、err が io.EOF
//	読み込みが続けられない   : record が読めた断片、failure が stage が read の診断、err が非 nil
//
// 文字列の分割の失敗では err が nil のままなので、呼ぶ側は次の Next を呼んで走査を続ける。
func (r *Reader) Next() (Record, *core.ImportFailure, error) {
	if r.source == nil {
		return Record{}, nil, fmt.Errorf("reading an auditd source: %w", errNotReset)
	}
	for {
		if len(r.ready) > 0 {
			return r.emit()
		}
		if r.done {
			if len(r.open) == 0 {
				return Record{}, nil, io.EOF
			}
			r.closeOpenUntil(nil)
			continue
		}
		scanned, failure, err := r.readScannedLine()
		if err != nil {
			r.closeOpenUntil(nil)
			return r.emitWithReadFailure(scanned, failure, err)
		}
		if failure != nil {
			// 事象の鍵を読めなかった行は、どの事象にも属さない 1 行のレコードにする。
			return singleLineRecord(scanned), failure, nil
		}
		if scanned == nil {
			continue
		}
		r.collect(*scanned)
		r.closeOpenUntil(scanned)
	}
}

// emit は集め終えたレコードを 1 件返す。
//
// **読めなかった範囲の診断を、レコードと別の返り値で返す。** 診断を付けたレコードは
// 取り込みの実行が失敗として扱い、公開するレコードから外す。先頭が切れている事象も
// 読めた行を持つため、診断だけを先に返してから事象を返す。
func (r *Reader) emit() (Record, *core.ImportFailure, error) {
	// 収集元の先頭が事象の途中で切れている状態は、最初のレコードでだけ判定できる。
	if !r.emitted {
		r.emitted = true
		if r.ready[0].HasSyscallCompanionOnly() {
			failure := truncatedHeadFailure(r.ready[0])
			return Record{}, &failure, nil
		}
	}
	record := r.ready[0]
	r.ready = r.ready[1:]
	return record, nil, nil
}

// emitWithReadFailure は、読み込みが続けられなくなったときのレコードと診断を返す。
func (r *Reader) emitWithReadFailure(
	scanned *scannedLine, failure *core.ImportFailure, err error,
) (Record, *core.ImportFailure, error) {
	if len(r.ready) > 0 {
		record := r.ready[0]
		r.ready = r.ready[1:]
		r.emitted = true
		return record, failure, err
	}
	return singleLineRecord(scanned), failure, err
}

// collect は 1 行を、同じ事象の行を集めている途中のレコードへ足す。
func (r *Reader) collect(scanned scannedLine) {
	for _, builder := range r.open {
		if builder.event.RawText == scanned.event.RawText {
			builder.add(scanned)
			return
		}
	}
	builder := &recordBuilder{}
	builder.add(scanned)
	r.open = append(r.open, builder)
}

// closeOpenUntil は、行を集める範囲を過ぎた事象を返す順の待ち行列へ移す。
// scanned が nil のときは、集めている途中の事象をすべて移す。
func (r *Reader) closeOpenUntil(scanned *scannedLine) {
	kept := r.open[:0]
	for _, builder := range r.open {
		if scanned != nil &&
			scanned.line.lineNumber-builder.firstLineNumber <= eventLineWindow {
			kept = append(kept, builder)
			continue
		}
		r.ready = append(r.ready, builder.build())
	}
	r.open = kept
}

// singleLineRecord は 1 行だけを持つレコードを返す。
func singleLineRecord(scanned *scannedLine) Record {
	if scanned == nil {
		return Record{}
	}
	builder := &recordBuilder{}
	builder.add(*scanned)
	return builder.build()
}

// scannedLine は読み終えた 1 行と、その行の位置を持つ。
type scannedLine struct {
	line       Line
	event      EventKey
	lineEnding string
	byteLength int64
}

// readScannedLine は 1 行を読んで文字列へ分ける。
//
// 収集元の末尾では scanned が nil、failure が nil、err が nil になる。
func (r *Reader) readScannedLine() (*scannedLine, *core.ImportFailure, error) {
	line, readErr := r.readLine()
	if len(line) == 0 {
		r.done = true
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			failure := readFailure(r.lineNumber+1, r.byteOffset,
				"reading the source stopped before a line boundary")
			return nil, &failure, fmt.Errorf(
				"reading an auditd source at byte offset %d: %w", r.byteOffset, readErr)
		}
		return nil, nil, nil
	}

	r.lineNumber++
	lineOffset := r.byteOffset
	r.byteOffset += int64(len(line))
	rawText, lineEnding := splitLineEnding(line)
	scanned := &scannedLine{
		line:       Line{rawText: rawText, lineNumber: r.lineNumber, byteOffset: lineOffset},
		lineEnding: lineEnding,
		byteLength: int64(len(rawText)),
	}

	if readErr != nil && !errors.Is(readErr, io.EOF) {
		r.done = true
		stopOffset := lineOffset + int64(len(line))
		failure := readFailure(r.lineNumber, stopOffset,
			"reading the source stopped inside the line at line "+
				strconv.FormatInt(r.lineNumber, 10)+" after "+strconv.Itoa(len(line))+" bytes")
		return scanned, &failure, fmt.Errorf(
			"reading an auditd source at byte offset %d: %w", stopOffset, readErr)
	}
	if errors.Is(readErr, io.EOF) {
		r.done = true
	}

	items, problem := tokenizeLine(rawText)
	scanned.line.recordType, _ = lineTypeOf(items)
	scanned.line.items = items
	if problem == nil {
		event, eventProblem := eventKeyOf(items)
		problem = eventProblem
		scanned.event = event
	}
	if problem != nil {
		failure := problem.importFailure(r.lineNumber, lineOffset)
		return scanned, &failure, nil
	}
	return scanned, nil, nil
}

// readLine は改行までを読む。上限を超えた行を打ち切らず、読み込みの失敗として返す。
func (r *Reader) readLine() (string, error) {
	var line strings.Builder
	for {
		b, err := r.source.ReadByte()
		if err != nil {
			return line.String(), err
		}
		line.WriteByte(b)
		if b == '\n' {
			return line.String(), nil
		}
		// 上限は行の原文に掛かる。改行の文字列を数えない。
		content := line.Len()
		if b == '\r' {
			content--
		}
		if content > maxLineBytes {
			return line.String(), errRecordTooLong
		}
	}
}

// splitLineEnding は 1 行から改行の文字列を切り離す。
func splitLineEnding(line string) (rawText, lineEnding string) {
	if strings.HasSuffix(line, "\r\n") {
		return line[:len(line)-2], "\r\n"
	}
	if strings.HasSuffix(line, "\n") {
		return line[:len(line)-1], "\n"
	}
	return line, ""
}

// recordBuilder は 1 事象に属する行を集める。
type recordBuilder struct {
	event           EventKey
	lines           []Line
	rawText         strings.Builder
	firstLineNumber int64
	byteOffset      int64
	byteEnd         int64
	lineEnding      string
}

// add は 1 行を集める。2 行目からは、直前の行の改行を原文へ入れる。
func (b *recordBuilder) add(scanned scannedLine) {
	if len(b.lines) == 0 {
		b.event = scanned.event
		b.firstLineNumber = scanned.line.lineNumber
		b.byteOffset = scanned.line.byteOffset
	} else {
		b.rawText.WriteString(b.lineEnding)
	}
	b.rawText.WriteString(scanned.line.rawText)
	b.byteEnd = scanned.line.byteOffset + scanned.byteLength
	b.lineEnding = scanned.lineEnding
	b.lines = append(b.lines, scanned.line)
}

func (b *recordBuilder) build() Record {
	return Record{
		event:      b.event,
		rawText:    b.rawText.String(),
		lineNumber: b.firstLineNumber,
		lineCount:  int64(len(b.lines)),
		byteOffset: b.byteOffset,
		byteLength: b.byteEnd - b.byteOffset,
		lineEnding: b.lineEnding,
		lines:      b.lines,
	}
}
