package markii

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// maxRecordBytes は 1 レコードの byte 数の上限である。
//
// 既知の制限: 1 レコードを 1 MiB までとする,
// opt-in の検査 (source_test.go) が ORACULUM_MARKII_SOURCE_DIR が指す入力の全レコードを
// 走査し、この上限による読み込みの失敗と最大長を t.Log に出す,
// 上限に達した失敗が入力で 1 件でも出たときに、その入力の最大長の上を取る値へ直す
const maxRecordBytes = 1 << 20

// Reader は 1 つの収集元の byte 列をレコードごとに読む。
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
}

// Reset は読む対象を入れ替える。行番号と byte offset を 0 に戻す。
//
// bufio.Scanner を使わない。既定の token の上限 64 KiB を超えた行で走査が静かに終わる。
// 失敗を無音にしない。
func (r *Reader) Reset(input io.Reader) {
	r.source = bufio.NewReader(input)
	r.lineNumber = 0
	r.byteOffset = 0
	r.done = false
}

// Next は次のレコードへ進む。
//
// **1 レコードの文字列の分割の失敗で走査を止めない。** 返り値の 3 つは次の組み合わせを取る。
//
//	正常                     : record が完成した値、failure が nil、err が nil
//	文字列の分割の失敗           : record が原文と位置と読めた field、failure が診断、err が nil
//	収集元の末尾             : record が zero value、failure が nil、err が io.EOF
//	読み込みが続けられない   : record が読めた断片、failure が stage が read の診断、err が非 nil
//
// 文字列の分割の失敗では err が nil のままなので、呼ぶ側は次の Next を呼んで走査を続ける。
// err が非 nil になったら走査を終える。
//
// 返した failure は文字列の分割が埋められる項目だけを埋めた値である。
// **そのままでは Validate を通らない。** 残る項目を埋めるのはパーサーと取り込みの実行で
// ある (failure.go の importFailure を見ること)。
func (r *Reader) Next() (Record, *core.ImportFailure, error) {
	if r.source == nil {
		return Record{}, nil, fmt.Errorf("reading a markii source: %w", errNotReset)
	}
	if r.done {
		return Record{}, nil, io.EOF
	}

	line, readErr := r.readLine()
	if len(line) == 0 {
		r.done = true
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			failure := readFailure(r.lineNumber+1, r.byteOffset, "reading the source stopped before a record boundary")
			return Record{}, &failure, fmt.Errorf("reading a markii source at byte offset %d: %w", r.byteOffset, readErr)
		}
		return Record{}, nil, io.EOF
	}

	r.lineNumber++
	recordOffset := r.byteOffset
	r.byteOffset += int64(len(line))
	rawText, lineEnding := splitLineEnding(line)

	if readErr != nil && !errors.Is(readErr, io.EOF) {
		// 改行に達する前に読み込みが止まった。読めた断片と位置を返し、走査を終える。
		// 診断の位置は止まった位置である。読めた断片の byte 数を行頭に足す。
		r.done = true
		stopOffset := recordOffset + int64(len(line))
		failure := readFailure(r.lineNumber, stopOffset,
			"reading the source stopped inside the record at line "+strconv.FormatInt(r.lineNumber, 10)+
				" after "+strconv.Itoa(len(line))+" bytes")
		record := Record{rawText: rawText, lineNumber: r.lineNumber, byteOffset: recordOffset, lineEnding: lineEnding}
		return record, &failure, fmt.Errorf("reading a markii source at byte offset %d: %w", stopOffset, readErr)
	}
	if errors.Is(readErr, io.EOF) {
		// 末尾に改行が無い最後のレコードである。次の Next が io.EOF を返す。
		r.done = true
	}

	var fields []Field
	var problem *tokenizeProblem
	if r.isTrailingDOSEOFMarker(rawText, lineEnding, readErr) {
		problem = dosEOFMarkerProblem()
	} else {
		fields, problem = tokenizeRecord(rawText)
	}
	record := Record{
		rawText:    rawText,
		lineNumber: r.lineNumber,
		byteOffset: recordOffset,
		lineEnding: lineEnding,
		fields:     fields,
	}
	if problem != nil {
		failure := problem.importFailure(r.lineNumber, recordOffset)
		return record, &failure, nil
	}
	return record, nil, nil
}

// isTrailingDOSEOFMarker は、物理的な入力末尾にある単独の DOS EOF marker を判定する。
// marker 自体が改行で終わる形式では readLine の readErr が nil になるため、次の byte を
// 覗いて物理的な末尾であることを確かめる。marker 行の後に入力が続く場合は false である。
func (r *Reader) isTrailingDOSEOFMarker(rawText, lineEnding string, readErr error) bool {
	if rawText != "\x1a" {
		return false
	}
	if errors.Is(readErr, io.EOF) {
		return true
	}
	if lineEnding == "" {
		return false
	}
	_, peekErr := r.source.Peek(1)
	return errors.Is(peekErr, io.EOF)
}

// readLine は改行までを読む。上限を超えた行を打ち切らず、読み込みの失敗として返す。
//
// bufio.Reader.ReadString は改行か末尾まで無制限に memory へ積む。改行を 1 個も持たない
// 巨大な入力を渡すと、診断を返す前に memory を使い切る。**上限を超えた行を「読めた」に
// しない。** 失敗した処理を別の処理で代替して成功に見せない。
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
		// 上限はレコードの原文に掛かる。改行の文字列を数えない。
		// 読んだ byte が CR のときは、次の byte が LF なら CR LF の改行であり原文に
		// 入らないので、1 byte を引いてから比べる。次の byte が LF でなければ CR は
		// 原文の一部であり、その byte を読んだ時点で上限を超える。
		content := line.Len()
		if b == '\r' {
			content--
		}
		if content > maxRecordBytes {
			return line.String(), errRecordTooLong
		}
	}
}

// splitLineEnding は 1 行から改行の文字列を切り離す。
//
// CR LF の CR を原文に入れない。改行の byte 数は呼ぶ側が byte offset に算入している。
func splitLineEnding(line string) (rawText, lineEnding string) {
	if strings.HasSuffix(line, "\r\n") {
		return line[:len(line)-2], "\r\n"
	}
	if strings.HasSuffix(line, "\n") {
		return line[:len(line)-1], "\n"
	}
	return line, ""
}
