package winregistry

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"unicode/utf16"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// hiveBuilder は hive bins data を組む。hbin は 1 つで、cell を先頭から順に置く。
type hiveBuilder struct {
	bins []byte
}

func newHiveBuilder() *hiveBuilder {
	b := &hiveBuilder{bins: make([]byte, 32)}
	copy(b.bins, "hbin")
	return b
}

// cell は data を持つ割り当て済みの cell を置き、位置を返す。
func (b *hiveBuilder) cell(data []byte) uint32 {
	offset := uint32(len(b.bins))
	size := (len(data) + 4 + 7) &^ 7
	cell := make([]byte, size)
	binary.LittleEndian.PutUint32(cell, uint32(-int32(size)))
	copy(cell[4:], data)
	b.bins = append(b.bins, cell...)
	return offset
}

// key は nk cell を置く。subkeys は子の nk の位置、values は vk の位置である。
func (b *hiveBuilder) key(name string, lastWritten uint64, subkeys, values []uint32) uint32 {
	subkeyList, valueList := uint32(noCell), uint32(noCell)
	if len(subkeys) > 0 {
		subkeyList = b.list("lf", subkeys)
	}
	if len(values) > 0 {
		data := make([]byte, 4*len(values))
		for i, value := range values {
			binary.LittleEndian.PutUint32(data[4*i:], value)
		}
		valueList = b.cell(data)
	}
	return b.rawKey(name, lastWritten, uint32(len(subkeys)), subkeyList, uint32(len(values)), valueList)
}

func (b *hiveBuilder) rawKey(name string, lastWritten uint64, subkeyCount, subkeyList, valueCount, valueList uint32) uint32 {
	data := make([]byte, keyNodeHeaderSize+len(name))
	copy(data, "nk")
	binary.LittleEndian.PutUint16(data[2:], keyCompressedName)
	binary.LittleEndian.PutUint64(data[4:], lastWritten)
	binary.LittleEndian.PutUint32(data[20:], subkeyCount)
	binary.LittleEndian.PutUint32(data[28:], subkeyList)
	binary.LittleEndian.PutUint32(data[32:], noCell)
	binary.LittleEndian.PutUint32(data[36:], valueCount)
	binary.LittleEndian.PutUint32(data[40:], valueList)
	binary.LittleEndian.PutUint32(data[44:], noCell)
	binary.LittleEndian.PutUint32(data[48:], noCell)
	binary.LittleEndian.PutUint16(data[72:], uint16(len(name)))
	copy(data[keyNodeHeaderSize:], name)
	return b.cell(data)
}

// list は subkey の list を置く。lf と lh は 8 byte、li と ri は 4 byte の要素を持つ。
func (b *hiveBuilder) list(signature string, offsets []uint32) uint32 {
	width := 4
	if signature == "lf" || signature == "lh" {
		width = 8
	}
	data := make([]byte, 4+width*len(offsets))
	copy(data, signature)
	binary.LittleEndian.PutUint16(data[2:], uint16(len(offsets)))
	for i, offset := range offsets {
		binary.LittleEndian.PutUint32(data[4+i*width:], offset)
	}
	return b.cell(data)
}

// value は vk cell を置く。4 byte 以下の data は vk の中に置き、長い data は別の cell に置く。
func (b *hiveBuilder) value(name string, valueType uint32, data []byte) uint32 {
	size, dataOffset := uint32(len(data)), uint32(0)
	switch {
	case len(data) <= 4:
		inline := make([]byte, 4)
		copy(inline, data)
		size |= inlineDataFlag
		dataOffset = binary.LittleEndian.Uint32(inline)
	case len(data) > bigDataSegmentSize:
		var segments []uint32
		for rest := data; len(rest) > 0; rest = rest[min(len(rest), bigDataSegmentSize):] {
			segments = append(segments, b.cell(rest[:min(len(rest), bigDataSegmentSize)]))
		}
		listData := make([]byte, 4*len(segments))
		for i, segment := range segments {
			binary.LittleEndian.PutUint32(listData[4*i:], segment)
		}
		header := make([]byte, 8)
		copy(header, "db")
		binary.LittleEndian.PutUint16(header[2:], uint16(len(segments)))
		binary.LittleEndian.PutUint32(header[4:], b.cell(listData))
		dataOffset = b.cell(header)
	default:
		dataOffset = b.cell(data)
	}
	vk := make([]byte, keyValueHeaderSize+len(name))
	copy(vk, "vk")
	binary.LittleEndian.PutUint16(vk[2:], uint16(len(name)))
	binary.LittleEndian.PutUint32(vk[4:], size)
	binary.LittleEndian.PutUint32(vk[8:], dataOffset)
	binary.LittleEndian.PutUint32(vk[12:], valueType)
	binary.LittleEndian.PutUint16(vk[16:], valueCompressedName)
	copy(vk[keyValueHeaderSize:], name)
	return b.cell(vk)
}

// padTo は bins を size byte まで未割り当ての cell で埋める。
func (b *hiveBuilder) padTo(size int) {
	if len(b.bins) >= size {
		return
	}
	free := make([]byte, size-len(b.bins))
	binary.LittleEndian.PutUint32(free, uint32(len(free)))
	b.bins = append(b.bins, free...)
}

// binsData は 4096 の倍数に埋めた hive bins data を返す。
func (b *hiveBuilder) binsData() []byte {
	b.padTo((len(b.bins) + hiveBinAlignment - 1) / hiveBinAlignment * hiveBinAlignment)
	bins := bytes.Clone(b.bins)
	binary.LittleEndian.PutUint32(bins[8:], uint32(len(bins)))
	return bins
}

// baseBlockBytes は base block を組む。size は 4096 (主 file) か 512 (log の写し) である。
func baseBlockBytes(size int, primary, secondary, fileType, root, binsSize uint32) []byte {
	block := make([]byte, size)
	copy(block, "regf")
	binary.LittleEndian.PutUint32(block[4:], primary)
	binary.LittleEndian.PutUint32(block[8:], secondary)
	binary.LittleEndian.PutUint64(block[12:], synthLastWritten)
	binary.LittleEndian.PutUint32(block[20:], 1)
	binary.LittleEndian.PutUint32(block[24:], 5)
	binary.LittleEndian.PutUint32(block[28:], fileType)
	binary.LittleEndian.PutUint32(block[32:], 1)
	binary.LittleEndian.PutUint32(block[36:], root)
	binary.LittleEndian.PutUint32(block[40:], binsSize)
	binary.LittleEndian.PutUint32(block[44:], 1)
	for i, unit := range utf16.Encode([]rune("synth")) {
		binary.LittleEndian.PutUint16(block[48+2*i:], unit)
	}
	binary.LittleEndian.PutUint32(block[checksumOffset:], xorChecksum(block[:checksumOffset]))
	return block
}

// primaryFile は base block と hive bins data を並べた主 file である。
func primaryFile(bins []byte, primary, secondary, root uint32) []byte {
	return append(baseBlockBytes(baseBlockSize, primary, secondary, fileTypePrimary, root, uint32(len(bins))), bins...)
}

// synthPage は log entry が書く page 1 つである。
type synthPage struct {
	rel  uint32
	data []byte
}

// synthEntry は log entry 1 件である。
type synthEntry struct {
	sequence uint32
	binsSize uint32
	pages    []synthPage
}

// logBytes は新形式の log の file を組む。base block の番号は最初の entry の番号である。
func logBytes(root uint32, entries ...synthEntry) []byte {
	out := baseBlockBytes(logBaseBlockSize, entries[0].sequence, entries[0].sequence, fileTypeLogNew, root, entries[0].binsSize)
	for _, entry := range entries {
		out = append(out, entryBytes(entry)...)
	}
	return out
}

func entryBytes(entry synthEntry) []byte {
	size := logEntryHeaderSize + 8*len(entry.pages)
	for _, page := range entry.pages {
		size += len(page.data)
	}
	size = (size + logBaseBlockSize - 1) / logBaseBlockSize * logBaseBlockSize
	data := make([]byte, size)
	copy(data, "HvLE")
	binary.LittleEndian.PutUint32(data[4:], uint32(size))
	binary.LittleEndian.PutUint32(data[12:], entry.sequence)
	binary.LittleEndian.PutUint32(data[16:], entry.binsSize)
	binary.LittleEndian.PutUint32(data[20:], uint32(len(entry.pages)))
	position := logEntryHeaderSize + 8*len(entry.pages)
	for i, page := range entry.pages {
		binary.LittleEndian.PutUint32(data[logEntryHeaderSize+8*i:], page.rel)
		binary.LittleEndian.PutUint32(data[logEntryHeaderSize+8*i+4:], uint32(len(page.data)))
		copy(data[position:], page.data)
		position += len(page.data)
	}
	binary.LittleEndian.PutUint64(data[24:], marvin32(data[logEntryHeaderSize:], logEntrySeed))
	binary.LittleEndian.PutUint64(data[32:], marvin32(data[:32], logEntrySeed))
	return data
}

// changedPages は old と new の hive bins data の、512 byte 単位で異なる page を返す。
func changedPages(old, new []byte) []synthPage {
	var pages []synthPage
	for rel := 0; rel < len(new); rel += unitSize {
		end := rel + unitSize
		if rel >= len(old) || !bytes.Equal(old[rel:end], new[rel:end]) {
			pages = append(pages, synthPage{rel: uint32(rel), data: bytes.Clone(new[rel:end])})
		}
	}
	return pages
}

// synthLastWritten は key と base block の最終更新の FILETIME (2001-02-03T04:05:06Z) である。
const synthLastWritten uint64 = 126256467060000000

// synthFile は収集元を構成する file 1 つである。
type synthFile struct {
	name string
	data []byte
}

// readAll は file を連結して Reader に渡し、全件を読む。
func readAll(t *testing.T, files ...synthFile) ([]Record, []*core.ImportFailure, *Reader) {
	t.Helper()
	var content []byte
	var members []Member
	for _, file := range files {
		members = append(members, Member{Name: file.name, Offset: int64(len(content)), Size: int64(len(file.data))})
		content = append(content, file.data...)
	}
	reader := &Reader{}
	reader.SetMembers(members)
	reader.Reset(bytes.NewReader(content))
	var records []Record
	var failures []*core.ImportFailure
	for {
		record, failure, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return records, failures, reader
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if failure != nil {
			failures = append(failures, failure)
			continue
		}
		records = append(records, record)
	}
}

// fieldText は名前の項目の文字列を返す。時刻の項目は正規化値である。
func fieldText(fields []core.RecordField, name string) (string, bool) {
	for _, field := range fields {
		if field.Name != name {
			continue
		}
		if field.Timestamp != nil {
			return *field.Timestamp.Normalized, true
		}
		return *field.Text.RawText, true
	}
	return "", false
}

// recordAt は KeyPath が path のレコードを返す。
func recordAt(t *testing.T, records []Record, path string) Record {
	t.Helper()
	for _, record := range records {
		if value, _ := fieldText(record.Fields, fieldKeyPath); value == path {
			return record
		}
	}
	t.Fatalf("no record for %s", path)
	return Record{}
}

func utf16Bytes(text string) []byte {
	units := utf16.Encode([]rune(text))
	data := make([]byte, 2*len(units)+2)
	for i, unit := range units {
		binary.LittleEndian.PutUint16(data[2*i:], unit)
	}
	return data
}
