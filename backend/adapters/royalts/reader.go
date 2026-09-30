package royalts

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

var errNotReset = errors.New("royalts: call Reset with an input first")

// utf8BOM は文書が持ちうる UTF-8 の byte order mark である。
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// maxDocumentBytes は Reset が本 package の中で読み切る文書の上限である。
//
// 収集元 1 件を全部メモリに保持する読み込みは pipeline.Runner.scan が先に行う。本上限は、
// 本 package が自身の走査と redactCredentialPassword の中で保持する複製の大きさを抑える。
//
// 既知の制限: 文書を 8 MiB まで保持して上限超過を read の失敗にする,
// 上限を超える文書が repo の中に無く測れない,
// pipeline 側の収集元の大きさの上限が決まったとき、本上限をそれに合わせて見直す
const maxDocumentBytes = 8 << 20

var errDocumentTooLarge = errors.New("royalts: document exceeds the byte limit")

const credentialPasswordLocalName = "CredentialPassword"                                 // #nosec G101 -- XML 要素名の定数であり秘密情報の値ではない
const credentialPasswordRedacted = "<CredentialPassword>[redacted]</CredentialPassword>" // #nosec G101 -- 置き換え後の固定文言であり秘密情報の値ではない

// redactCredentialPassword は RawText に含まれる `CredentialPassword` 要素の内容を除く。
//
// **RawText は原資料の原文を保持する対象だが、CredentialPassword の暗号文だけは例外と
// する。** RawText は要素全体の原文を byte 範囲でそのまま切り出すため、除かないと暗号文が残る。
//
// 要素の判定は royalRDSConnectionXML の decode と同じ Name.Local で行い、名前空間の接頭辞を
// 持つ文字列 (`<ns:CredentialPassword>`) も除く。
//
// 走査に失敗した場合は redactionFailedPlaceholder を返す。原文を返すと、失敗した箇所より
// 後ろの CredentialPassword が残る。
func redactCredentialPassword(raw string) string {
	decoder := xml.NewDecoder(strings.NewReader(raw))
	var redacted strings.Builder
	var cursor int64
	for {
		start := decoder.InputOffset()
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return redactionFailedPlaceholder
		}
		element, ok := token.(xml.StartElement)
		if !ok || element.Name.Local != credentialPasswordLocalName {
			continue
		}
		redacted.WriteString(raw[cursor:start])
		if err := decoder.Skip(); err != nil {
			return redactionFailedPlaceholder
		}
		cursor = decoder.InputOffset()
		redacted.WriteString(credentialPasswordRedacted)
	}
	redacted.WriteString(raw[cursor:])
	return redacted.String()
}

// redactionFailedPlaceholder は CredentialPassword の走査に失敗したときに RawText として
// 返す固定文言である。
const redactionFailedPlaceholder = "[content withheld: CredentialPassword redaction failed]" // #nosec G101 -- 固定文言であり秘密情報の値ではない

// royalRDSConnectionXML は `<RoyalRDSConnection>` 要素のうち本 adapter が読む項目である。
//
// **CredentialPassword の field を持たせない。** encoding/xml は宣言していない要素を
// 読まないため、暗号文であっても値を本型に取り込まない。
type royalRDSConnectionXML struct {
	ID                 *string `xml:"ID"`
	Name               *string `xml:"Name"`
	URI                *string `xml:"URI"`
	CredentialUsername *string `xml:"CredentialUsername"`
}

// Connection は 1 つの `<RoyalRDSConnection>` 要素の原文と位置と、読めた項目を持つ。
type Connection struct {
	rawText            string
	lineNumber         int64
	byteOffset         int64
	sequence           int64
	id                 *string
	name               *string
	uri                *string
	credentialUsername *string
}

// RawText はこの要素の原文である。CredentialPassword の暗号文は
// redactCredentialPassword で除いてある。
func (c Connection) RawText() string { return c.rawText }

// LineNumber は開始タグがある行番号を 1 起点で返す。
func (c Connection) LineNumber() int64 { return c.lineNumber }

// ByteOffset は文書内の開始タグの byte offset を 0 起点で返す。BOM を持つ文書では、
// BOM を含めた文書全体の中の位置である。
func (c Connection) ByteOffset() int64 { return c.byteOffset }

// Sequence は文書内で何番目の `<RoyalRDSConnection>` 要素かを 1 起点で返す。
func (c Connection) Sequence() int64 { return c.sequence }

// ID は `<ID>` の値である。要素が無いとき ok は偽になる。
func (c Connection) ID() (string, bool) { return derefString(c.id) }

// Name は `<Name>` の値である。要素が無いとき ok は偽になる。
func (c Connection) Name() (string, bool) { return derefString(c.name) }

// URI は `<URI>` の値である。要素が無いとき ok は偽になる。
func (c Connection) URI() (string, bool) { return derefString(c.uri) }

// CredentialUsername は `<CredentialUsername>` の値である。要素が無いとき ok は偽になる。
func (c Connection) CredentialUsername() (string, bool) { return derefString(c.credentialUsername) }

func derefString(v *string) (string, bool) {
	if v == nil {
		return "", false
	}
	return *v, true
}

// Reader は Royal TS 文書から `<RoyalRDSConnection>` 要素を 1 件ずつ読む。
// zero value に Reset して使う。
type Reader struct {
	content  []byte
	bomLen   int64
	decoder  *xml.Decoder
	sequence int64
	done     bool
	wasReset bool
	readErr  error
}

// Reset は入力を差し替え、走査を先頭に戻す。入力全体を読み切ってから走査する。
// XML は要素の対応を先読みしないと確定できないため、行単位の走査を持たない。
func (r *Reader) Reset(input io.Reader) {
	*r = Reader{}
	if input == nil {
		return
	}
	r.wasReset = true
	limited := io.LimitReader(input, maxDocumentBytes+1)
	content, err := io.ReadAll(limited)
	switch {
	case err != nil:
		r.readErr = err
	case int64(len(content)) > maxDocumentBytes:
		r.readErr = errDocumentTooLarge
	default:
		trimmed := bytes.TrimPrefix(content, utf8BOM)
		r.bomLen = int64(len(content) - len(trimmed))
		r.content = trimmed
		r.decoder = xml.NewDecoder(bytes.NewReader(trimmed))
	}
	if r.readErr != nil {
		r.done = true
	}
}

// Next は次の `<RoyalRDSConnection>` 要素を返す。
//
// 返り値の組み合わせは他の adapter の Reader.Next と同じ 4 通りである。文書そのものが
// 整形式でない場合は、最初の呼び出しで読み込みの失敗として返り、以降は呼ばれない前提で
// 走査を終える。
func (r *Reader) Next() (Connection, *core.ImportFailure, error) {
	if !r.wasReset {
		return Connection{}, nil, fmt.Errorf("reading Royal TS source: %w", errNotReset)
	}
	if r.readErr != nil {
		err := r.readErr
		r.readErr = nil
		failure := failureAt(core.FailureStageRead, 1, 0,
			"a readable Royal TS document within the byte limit", err.Error())
		return Connection{}, failure, fmt.Errorf("reading Royal TS source: %w", err)
	}
	if r.done {
		return Connection{}, nil, io.EOF
	}
	for {
		startOffset := r.decoder.InputOffset()
		token, err := r.decoder.Token()
		if errors.Is(err, io.EOF) {
			r.done = true
			return Connection{}, nil, io.EOF
		}
		if err != nil {
			r.done = true
			line := lineNumberAt(r.content, startOffset)
			failure := failureAt(core.FailureStageTokenize, line, startOffset+r.bomLen,
				"well-formed XML", err.Error())
			return Connection{}, failure, fmt.Errorf("reading Royal TS source at byte offset %d: %w", startOffset+r.bomLen, err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "RoyalRDSConnection" {
			continue
		}
		var decoded royalRDSConnectionXML
		if err := r.decoder.DecodeElement(&decoded, &start); err != nil {
			r.done = true
			line := lineNumberAt(r.content, startOffset)
			failure := failureAt(core.FailureStageTokenize, line, startOffset+r.bomLen,
				"a well-formed RoyalRDSConnection element", err.Error())
			return Connection{}, failure, fmt.Errorf("reading Royal TS source at byte offset %d: %w", startOffset+r.bomLen, err)
		}
		endOffset := r.decoder.InputOffset()
		r.sequence++
		return Connection{
			rawText:    redactCredentialPassword(string(r.content[startOffset:endOffset])),
			lineNumber: lineNumberAt(r.content, startOffset),
			byteOffset: startOffset + r.bomLen, sequence: r.sequence,
			id: decoded.ID, name: decoded.Name, uri: decoded.URI,
			credentialUsername: decoded.CredentialUsername,
		}, nil, nil
	}
}

// lineNumberAt は byte offset までに現れる改行の数から、1 起点の行番号を返す。
// BOM は改行を持たないため、BOM を除いた content で数えても行番号は変わらない。
func lineNumberAt(content []byte, offset int64) int64 {
	if offset < 0 {
		offset = 0
	}
	if offset > int64(len(content)) {
		offset = int64(len(content))
	}
	return int64(bytes.Count(content[:offset], []byte{'\n'})) + 1
}
