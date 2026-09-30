package prefetch

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

var errNotReset = errors.New("prefetch: call Reset with an input first")

// maxFileBytes は Reset が読み切る file の上限であり、展開した後の大きさの上限でもある。
//
// 既知の制限: 8 MiB を超える file を read の失敗、展開した後に超える file を tokenize の失敗に
// する, 上限を超える Prefetch の file が repo の中に無く測れない,
// pipeline 側の収集元の大きさの上限が決まったとき、本上限をそれに合わせて見直す
const maxFileBytes = 8 << 20

var errFileTooLarge = errors.New("prefetch: file exceeds the byte limit")

// Record は 1 つの file を読んだ結果である。
type Record struct {
	// File は file から読んだ値である。失敗した file では空である。
	File File
	// RawText は file の全体の byte を大文字の 16 進で書いた文字列である。
	RawText string
	// ByteLength は file の byte 数である。レコードの位置は file の先頭から全体である。
	ByteLength int64
}

// Reader は 1 つの file を 1 件のレコードとして返す。
type Reader struct {
	content  []byte
	readErr  error
	wasReset bool
	done     bool
}

// Reset は入力を差し替え、走査を先頭に戻す。入力全体を読み切ってから走査する。
func (r *Reader) Reset(input io.Reader) {
	*r = Reader{}
	if input == nil {
		return
	}
	r.wasReset = true
	r.content, r.readErr = io.ReadAll(io.LimitReader(input, maxFileBytes+1))
	if r.readErr == nil && len(r.content) > maxFileBytes {
		r.readErr = errFileTooLarge
	}
}

// Next は file の 1 件を返し、2 回目からは io.EOF を返す。返り値の組み合わせは次の 3 通りである。
//
//   - 読めた file: レコードを返し、失敗と error は nil である。
//   - 解釈できない file: レコードの原文と失敗を返し、error は nil である。
//   - 読み取りの失敗: 失敗と error を返し、走査は止まる。
func (r *Reader) Next() (Record, *core.ImportFailure, error) {
	if !r.wasReset {
		return Record{}, nil, fmt.Errorf("reading Prefetch source: %w", errNotReset)
	}
	if r.done {
		return Record{}, nil, io.EOF
	}
	r.done = true
	if r.readErr != nil {
		failure := failureAt(core.FailureStageRead, "a readable Prefetch file of at most 8 MiB", r.readErr.Error())
		return Record{}, failure, fmt.Errorf("reading Prefetch source: %w", r.readErr)
	}
	record := Record{
		RawText:    strings.ToUpper(hex.EncodeToString(r.content)),
		ByteLength: int64(len(r.content)),
	}
	file, err := parseFile(r.content)
	if err != nil {
		return record, failureAt(core.FailureStageTokenize, "a Prefetch file", err.Error()), nil
	}
	record.File = file
	return record, nil, nil
}
