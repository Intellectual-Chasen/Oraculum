package winregistry

import (
	"encoding/binary"
	"math"
	"unicode/utf16"
)

const (
	// baseBlockSize は主 file の base block の byte 数である。hive bins data はその後に続く。
	baseBlockSize = 4096
	// logBaseBlockSize は log の file の先頭の、base block の写しの byte 数である。
	logBaseBlockSize = 512
	// checksumOffset は base block の XOR-32 checksum の位置である。先行する 508 byte の値である。
	checksumOffset = 508
	// hiveBinAlignment は hive bin と hive bins data の大きさの単位である。
	hiveBinAlignment = 4096
)

// base block の File type の値。
const (
	fileTypePrimary   = 0
	fileTypeLogLegacy = 1
	fileTypeLogOld    = 2
	fileTypeLogNew    = 6
)

// baseBlock は主 file または log の file の先頭の base block から読んだ値である。
type baseBlock struct {
	primarySequence   uint32
	secondarySequence uint32
	lastWritten       uint64
	majorVersion      uint32
	minorVersion      uint32
	fileType          uint32
	rootCellOffset    uint32
	hiveBinsDataSize  uint32
	fileName          string
	checksumValid     bool
}

// parseBaseBlock は block の先頭 512 byte を base block として読む。ok が偽になるのは、
// 512 byte に満たないときと、署名が `regf` でないときである。
func parseBaseBlock(block []byte) (baseBlock, bool) {
	if len(block) < logBaseBlockSize || string(block[:4]) != "regf" {
		return baseBlock{}, false
	}
	le := binary.LittleEndian
	return baseBlock{
		primarySequence:   le.Uint32(block[4:]),
		secondarySequence: le.Uint32(block[8:]),
		lastWritten:       le.Uint64(block[12:]),
		majorVersion:      le.Uint32(block[20:]),
		minorVersion:      le.Uint32(block[24:]),
		fileType:          le.Uint32(block[28:]),
		rootCellOffset:    le.Uint32(block[36:]),
		hiveBinsDataSize:  le.Uint32(block[40:]),
		fileName:          utf16String(block[48:112]),
		checksumValid:     xorChecksum(block[:checksumOffset]) == le.Uint32(block[checksumOffset:]),
	}, true
}

// xorChecksum は 4 byte ごとの XOR である。0xFFFFFFFF は 0xFFFFFFFE に、0 は 1 に置き換える。
func xorChecksum(data []byte) uint32 {
	var sum uint32
	for i := 0; i+4 <= len(data); i += 4 {
		sum ^= binary.LittleEndian.Uint32(data[i:])
	}
	switch sum {
	case math.MaxUint32:
		return math.MaxUint32 - 1
	case 0:
		return 1
	}
	return sum
}

// dirty は主 file の書き出しが途中で終わったことを base block が示すかである。
func (b baseBlock) dirty() bool {
	return !b.checksumValid || b.primarySequence != b.secondarySequence
}

// utf16String は UTF-16LE の byte 列を、最初の NUL の前までの文字列にする。
func utf16String(data []byte) string {
	units := make([]uint16, 0, len(data)/2)
	for i := 0; i+2 <= len(data); i += 2 {
		unit := binary.LittleEndian.Uint16(data[i:])
		if unit == 0 {
			break
		}
		units = append(units, unit)
	}
	return string(utf16.Decode(units))
}
