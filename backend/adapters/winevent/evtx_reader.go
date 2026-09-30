package winevent

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"www.velocidex.com/golang/evtx"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// EVTX の file の構造の大きさ。file の見出し、chunk、chunk の見出しの大きさは EVTX の
// 形式が定める。
const (
	evtxFileHeaderSize    = 128
	evtxChunkSize         = evtx.EVTX_CHUNK_SIZE
	evtxChunkHeaderSize   = evtx.EVTX_CHUNK_HEADER_SIZE
	evtxRecordHeaderSize  = evtx.EVTX_EVENT_RECORD_SIZE
	evtxRecordTrailerSize = 4
	// evtxChunkFreeSpaceAt は、chunk の見出しのうち空き領域の始まりの位置を持つ欄の位置である。
	// ライブラリの ChunkHeader はこの欄を名前の無い欄にしている。
	evtxChunkFreeSpaceAt = 48
	// evtxMaxRecordsPerChunk は chunk 1 つに収まるレコードの件数の上限である。レコードは
	// 見出しと末尾の大きさの複製の分より小さくならない。
	evtxMaxRecordsPerChunk = (evtxChunkSize - evtxChunkHeaderSize) / (evtxRecordHeaderSize + evtxRecordTrailerSize)
	// evtxSupportedMajorVersion と evtxSupportedMinorVersion は、ライブラリが読める EVTX の file 形式のバージョンである。
	evtxSupportedMajorVersion = 3
	evtxSupportedMinorVersion = 2
	// evtxFileFlagDirty は、file の見出しの flags のうち、書き込みの途中で閉じられた file を
	// 表す bit である。
	evtxFileFlagDirty = 0x1
)

// file の見出しが記録した値の名前。SourceIdentity.fileHeader の項目の名前になる。
const (
	nameHeaderNextRecordID = "NextRecordID"
	nameHeaderDirty        = "Dirty"
	// nameHeaderChunkCount は file の見出しが記録した chunk の数である。
	nameHeaderChunkCount = "ChunkCount"
	// nameSignedChunkCount は、file の中で chunk の署名 (ElfChnk) で始まる chunk を、走査で
	// 数えた数である。
	nameSignedChunkCount = "SignedChunkCount"
)

// nameRecordHeaderID は、レコードの見出しの番号を Event.Sections に置く名前である。
//
// **System の EventRecordID と別の項目である。** 2 つの番号は一致しないことがある。
const nameRecordHeaderID = "RecordHeader.RecordID"

var (
	evtxFileMagic   = []byte(evtx.EVTX_HEADER_MAGIC)
	evtxChunkMagic  = []byte(evtx.EVTX_CHUNK_HEADER_MAGIC)
	evtxRecordMagic = []byte(evtx.EVTX_EVENT_RECORD_MAGIC)
)

// EVTXReader は EVTX の file からレコードを 1 件ずつ読む。zero value に Reset して使う。
//
// **chunk を file の末尾まで順に読む。** file の見出しの次のレコード番号と chunk の数で
// 止めない。書き込みの途中で閉じられた file は、見出しの値を超えたレコードを持つ。
//
// レコードの byte 位置は、chunk の見出しの後ろからレコードの見出し (magic と大きさ) を
// たどって数える。ライブラリが返すレコードは file の中の位置を持たないため、たどった
// レコードとレコードの見出しの番号で突き合わせる。
//
// 1 件の原文は、ライブラリがレコードから組んだ構造を `<Event>` 要素の XML に書いた文字列で
// ある (renderEventXML)。byte 列が壊れた chunk とレコードは、その byte 列を 16 進で書いた
// 文字列を原文に持つ失敗になる。
type EVTXReader struct {
	content []byte
	header  evtx.EVTXHeader
	// headerRead は file の見出しを読み終えたか、headerValid はその見出しが EVTX の見出しで
	// あったかである。
	headerRead  bool
	headerValid bool
	// nextChunk は次に読む chunk の開始位置である。
	nextChunk int
	// signedChunks は、読んだ chunk のうち chunk の署名で始まる chunk の数である。
	signedChunks int
	// chunksWalked は、見出しを読んだ後に chunk の走査へ進んだかである。進まなかった file は
	// 署名を持つ chunk の数を数えていない。
	chunksWalked bool
	pending      []evtxItem
	done         bool
	wasReset     bool
	readErr      error
}

// evtxItem は Next が返す 1 件である。
type evtxItem struct {
	event   Event
	failure *core.ImportFailure
}

// recordSpan は chunk の中でたどったレコード 1 件である。offset は chunk の先頭からの位置である。
type recordSpan struct {
	offset int
	size   int
	id     uint64
}

// Reset は入力を差し替え、走査を先頭に戻す。入力全体を読み切ってから走査する。
//
// 既知の制限: 収集元の全体をもう 1 つ複製して保持する,
// 数十 MB の file の取り込みで、複製 1 つ分は取り込みの実行全体の最大 RSS の一部である,
// 取り込みの実行が収集元を複製せずに渡せるようになったとき、複製をやめる
func (r *EVTXReader) Reset(input io.Reader) {
	*r = EVTXReader{}
	if input == nil {
		return
	}
	r.wasReset = true
	r.content, r.readErr = io.ReadAll(input)
}

// Next は次の 1 件を返す。返り値の組み合わせは XMLReader.Next と同じ 4 通りである。
func (r *EVTXReader) Next() (Event, *core.ImportFailure, error) {
	if !r.wasReset {
		return Event{}, nil, fmt.Errorf("reading EVTX source: %w", errNotReset)
	}
	if r.readErr != nil {
		err := r.readErr
		r.readErr, r.done = nil, true
		failure := evtxFailure(core.FailureStageRead, Source{}, "a readable EVTX file", err.Error())
		offset := int64(len(r.content))
		failure.ByteOffset = &offset
		return Event{}, failure, fmt.Errorf("reading EVTX source at byte offset %d: %w", offset, err)
	}
	for len(r.pending) == 0 {
		if r.done {
			return Event{}, nil, io.EOF
		}
		r.advance()
	}
	item := r.pending[0]
	r.pending = r.pending[1:]
	return item.event, item.failure, nil
}

// SourceHeader は file の見出しが記録した次のレコード番号と、書き込みの途中で閉じられた
// file であるかと、見出しが記録した chunk の数と、走査で数えた署名を持つ chunk の数を返す。
// 見出しを読めなかった file では要素数 0 である。署名を持つ chunk の数は、Next が io.EOF を
// 返した後に呼ぶと file 全体の数になる。見出しの版か大きさで chunk の走査へ進まなかった file は、
// 署名を持つ chunk の数を持たない。
func (r *EVTXReader) SourceHeader() []core.RecordField {
	if !r.headerValid {
		return nil
	}
	dirty := r.header.FileFlags&evtxFileFlagDirty != 0
	fields := []core.RecordField{
		textField(nameHeaderNextRecordID, "", strconv.FormatUint(r.header.NextRecordID, 10)),
		textField(nameHeaderDirty, "", strconv.FormatBool(dirty)),
		textField(nameHeaderChunkCount, "", strconv.FormatUint(uint64(r.header.ChunkCount), 10)),
	}
	if r.chunksWalked {
		fields = append(fields, textField(nameSignedChunkCount, "", strconv.Itoa(r.signedChunks)))
	}
	return fields
}

// advance は file の見出しか chunk 1 つを読み、返す件を pending へ足す。
func (r *EVTXReader) advance() {
	if !r.headerRead {
		r.readFileHeader()
		return
	}
	start := r.nextChunk
	if start >= len(r.content) {
		r.done = true
		return
	}
	end := min(start+evtxChunkSize, len(r.content))
	r.nextChunk = start + evtxChunkSize
	chunk := r.content[start:end]
	if bytes.HasPrefix(chunk, evtxChunkMagic) {
		r.signedChunks++
	}
	switch {
	// 使っていない chunk は 0 で埋まっている。
	case isZero(chunk):
	case len(chunk) < evtxChunkSize:
		r.fail(start, len(chunk), "a whole chunk of "+strconv.Itoa(evtxChunkSize)+" bytes",
			"the file ends "+strconv.Itoa(len(chunk))+" bytes into the chunk")
	case !bytes.HasPrefix(chunk, evtxChunkMagic):
		r.fail(start, len(chunk), "a chunk starting with the chunk magic", "the chunk starts with other bytes")
	default:
		r.readChunk(start, chunk)
	}
}

// readFileHeader は file の見出しを読む。EVTX の見出しで無い file は、失敗 1 件で走査を終える。
func (r *EVTXReader) readFileHeader() {
	r.headerRead = true
	headerEnd := min(len(r.content), evtxFileHeaderSize)
	if len(r.content) < evtxFileHeaderSize || !bytes.HasPrefix(r.content, evtxFileMagic) {
		r.done = true
		r.fail(0, headerEnd, "an EVTX file header of "+strconv.Itoa(evtxFileHeaderSize)+" bytes starting with the file magic",
			"the file does not start with an EVTX file header")
		return
	}
	// 見出しの大きさの byte 列を読むため、binary.Read は失敗しない。
	_ = binary.Read(bytes.NewReader(r.content[:evtxFileHeaderSize]), binary.LittleEndian, &r.header)
	r.headerValid = true
	version := fmt.Sprintf("%d.%d", r.header.MajorVersion, r.header.MinorVersion)
	switch {
	case r.header.MajorVersion != evtxSupportedMajorVersion || r.header.MinorVersion > evtxSupportedMinorVersion:
		r.done = true
		r.fail(0, headerEnd, "an EVTX file of version 3.0 to 3.2", "the file header names the version "+version)
	case int(r.header.HeaderBlockSize) < evtxFileHeaderSize:
		r.done = true
		r.fail(0, headerEnd, "a file header block of at least "+strconv.Itoa(evtxFileHeaderSize)+" bytes",
			"the file header names a block of "+strconv.Itoa(int(r.header.HeaderBlockSize))+" bytes")
	default:
		r.nextChunk = int(r.header.HeaderBlockSize)
		r.chunksWalked = true
	}
}

// readChunk は chunk 1 つのレコードを読み、件と失敗を pending へ足す。start は chunk の
// file の中の開始位置である。
func (r *EVTXReader) readChunk(start int, chunk []byte) {
	var header evtx.ChunkHeader
	// chunk の見出しより長い byte 列を読むため、binary.Read は失敗しない。
	_ = binary.Read(bytes.NewReader(chunk), binary.LittleEndian, &header)
	spans, broken := walkRecords(chunk, header)
	var records []*evtx.EventRecord
	// **ライブラリに読ませる件数を、大きさを確かめたレコードの数に縛る。** ライブラリは
	// chunk の見出しの最初と最後の番号の間を回り、大きさが 0 のレコードで位置が進まないため、
	// 見出しの番号を渡すと入力が名乗る回数だけ回る。**番号を 0 から数え直して渡す。** 見出しの
	// 最初の番号に件数を足すと uint64 の最後で 0 に戻り、ループが終わらない。ライブラリは
	// 2 つの番号をループの回数にだけ使う。
	if len(spans) > 0 {
		counted := header
		counted.FirstEventRecNumber, counted.LastEventRecNumber = 0, uint64(len(spans))-1
		var err error
		if records, err = parseChunk(chunk, counted); err != nil {
			r.fail(start, len(chunk), "a chunk whose records the EVTX library reads", err.Error())
			return
		}
	}
	next := 0
	for _, span := range spans {
		if next < len(records) && records[next].Header.RecordID == span.id {
			r.pending = append(r.pending, r.eventOf(start, span, records[next]))
			next++
			continue
		}
		r.fail(start+span.offset, span.size, "an event the EVTX library read from the record",
			"the library returned no event for the record "+strconv.FormatUint(span.id, 10))
	}
	if broken != nil {
		r.fail(start+broken.offset, broken.end-broken.offset, broken.expected, broken.observed)
	}
}

// brokenRecord は、chunk の中でレコードとして読めない [offset, end) の byte 列と理由である。
type brokenRecord struct {
	offset, end        int
	expected, observed string
}

// walkRecords は chunk の見出しの後ろからレコードの見出しをたどる。
//
// ライブラリと同じく、chunk の見出しの最初と最後のレコード番号が示す件数だけたどる。
// たどれなくなった位置から chunk の末尾までと、数えたレコードと空き領域の始まりの間を
// broken で返す。broken の byte 列はレコードの境界を決められない。
//
// 既知の制限: 空き領域の始まりより後ろに残る、chunk を前に使ったときのレコードを読まない,
// 前に使ったときのレコードは番号が chunk の最初のレコードより小さく、そのレコードが残る EVTX の
// file が repo の中に無く測れない,
// 消えたレコードを回復する要件が出たとき、残ったレコードを別の区分で読む
func walkRecords(chunk []byte, header evtx.ChunkHeader) ([]recordSpan, *brokenRecord) {
	free := int(binary.LittleEndian.Uint32(chunk[evtxChunkFreeSpaceAt:]))
	// 書き込まれたレコードが無い chunk は、空き領域が chunk の見出しの直後から始まり、最後の
	// レコード番号がすべての bit が 1 の値のままである。空き領域の後ろに前に使ったときの
	// レコードが残っても読まない。
	if free == evtxChunkHeaderSize && header.LastEventRecNumber == math.MaxUint64 {
		return nil, nil
	}
	// 差が chunk に収まる件数を超える見出しも矛盾である。差が uint64 の最大の値のとき、
	// 件数を差に 1 を足して求めると 0 に戻る。
	if header.LastEventRecNumber < header.FirstEventRecNumber ||
		header.LastEventRecNumber-header.FirstEventRecNumber >= evtxMaxRecordsPerChunk {
		return nil, &brokenRecord{offset: 0, end: len(chunk),
			expected: fmt.Sprintf("a chunk header whose record numbers count 1 to %d records", evtxMaxRecordsPerChunk),
			observed: fmt.Sprintf("the chunk header names the records %d to %d",
				header.FirstEventRecNumber, header.LastEventRecNumber)}
	}
	count := header.LastEventRecNumber - header.FirstEventRecNumber + 1
	var spans []recordSpan
	at := evtxChunkHeaderSize
	for n := uint64(0); n < count; n++ {
		size, problem := recordSizeAt(chunk, at)
		if problem != "" {
			return spans, &brokenRecord{offset: at, end: len(chunk),
				expected: fmt.Sprintf("the record %d of %d the chunk header counts", n+1, count),
				observed: problem}
		}
		spans = append(spans, recordSpan{offset: at, size: size, id: binary.LittleEndian.Uint64(chunk[at+8:])})
		at += size
	}
	if at < free {
		return spans, &brokenRecord{offset: at, end: min(free, len(chunk)),
			expected: fmt.Sprintf("the %d records the chunk header counts to end at the free space offset %d", count, free),
			observed: fmt.Sprintf("the counted records end at %d", at)}
	}
	return spans, nil
}

// recordSizeAt は at から始まるレコードの大きさを返す。レコードの見出しとして読めないときは
// 理由を返す。
func recordSizeAt(chunk []byte, at int) (int, string) {
	if at+evtxRecordHeaderSize > len(chunk) {
		return 0, "the record header runs past the end of the chunk"
	}
	if !bytes.HasPrefix(chunk[at:], evtxRecordMagic) {
		return 0, "the record does not start with the record magic"
	}
	size := int(binary.LittleEndian.Uint32(chunk[at+len(evtxRecordMagic):]))
	if size < evtxRecordHeaderSize+evtxRecordTrailerSize || size > len(chunk)-at {
		return 0, fmt.Sprintf("the record header names a size of %d bytes, which does not fit in the chunk", size)
	}
	// レコードは末尾に大きさの複製を持つ。
	if trailer := int(binary.LittleEndian.Uint32(chunk[at+size-evtxRecordTrailerSize:])); trailer != size {
		return 0, fmt.Sprintf("the record header names a size of %d bytes and the record ends with %d", size, trailer)
	}
	return size, ""
}

// parseChunk はライブラリに chunk 1 つのレコードを読ませる。
//
// **ライブラリの panic を chunk 1 つの失敗にする。** 壊れた byte 列で panic が起きると、
// 後ろの chunk を読めなくなる。
func parseChunk(chunk []byte, header evtx.ChunkHeader) (records []*evtx.EventRecord, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			records, err = nil, fmt.Errorf("the EVTX library panicked: %v", recovered)
		}
	}()
	parsed := &evtx.Chunk{Header: header, Fd: bytes.NewReader(chunk)}
	return parsed.Parse(0)
}

// eventOf はたどったレコード 1 件とライブラリが読んだ構造から、Event または失敗を組む。
func (r *EVTXReader) eventOf(chunkStart int, span recordSpan, record *evtx.EventRecord) evtxItem {
	offset := chunkStart + span.offset
	text, err := renderEventXML(record.Event, record.Header.FileTime)
	if err != nil {
		return r.failureItem(offset, span.size, "a record the EVTX library reads as an <Event> element", err.Error())
	}
	source := Source{RawText: text, ByteOffset: int64(offset), ByteLength: int64(span.size)}
	event, err := decodeEvent([]byte(text))
	if err != nil {
		return evtxItem{event: Event{Source: source}, failure: evtxFailure(core.FailureStageTokenize, source,
			"a record that renders as a well-formed <Event> element", err.Error())}
	}
	event.Source = source
	event.Sections = append(event.Sections, Value{Name: nameRecordHeaderID, Text: strconv.FormatUint(span.id, 10)})
	return evtxItem{event: event}
}

// fail は [offset, offset+length) の byte 列の失敗を pending へ足す。
func (r *EVTXReader) fail(offset, length int, expected, observed string) {
	r.pending = append(r.pending, r.failureItem(offset, length, expected, observed))
}

// failureItem は [offset, offset+length) の byte 列を 16 進で書いた文字列を原文に持つ失敗を返す。
func (r *EVTXReader) failureItem(offset, length int, expected, observed string) evtxItem {
	source := Source{
		RawText:    strings.ToUpper(hex.EncodeToString(r.content[offset : offset+length])),
		ByteOffset: int64(offset), ByteLength: int64(length),
	}
	return evtxItem{event: Event{Source: source}, failure: evtxFailure(core.FailureStageTokenize, source, expected, observed)}
}

// evtxFailure は EVTX の読み取りの失敗を組む。返す値の扱いは failureAt と同じである。
func evtxFailure(stage core.FailureStage, source Source, expected, observed string) *core.ImportFailure {
	failure := failureAt(stage, source, expected, observed)
	switch stage {
	case core.FailureStageRead:
		failure.Interpretation = "the whole EVTX file read into memory before any chunk was interpreted"
		failure.UnresolvedReason = "reading stopped; the I/O failure alone does not establish input inconsistency or a parser defect"
	default:
		failure.Interpretation = "an EVTX file header followed by chunks of " + strconv.Itoa(evtxChunkSize) +
			" bytes; records followed from each chunk header by their size and rendered by the EVTX library"
		failure.UnresolvedReason = "the bytes alone do not distinguish a damaged copy, a file closed while it was written, and parser defects"
	}
	return failure
}

func isZero(content []byte) bool {
	for _, b := range content {
		if b != 0 {
			return false
		}
	}
	return true
}
