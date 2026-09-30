package winregistry

import (
	"encoding/binary"
	"fmt"
)

// unitSize は、hive bins data の byte がどの file のどの位置から来たかを記録する単位である。
// log entry の dirty page の位置と大きさはこの倍数である。
const unitSize = 512

// noCell は cell を指さない位置の値である。
const noCell = 0xFFFFFFFF

// hive は log を適用した後の hive bins data と、その各 byte が収集元のどこから来たかである。
type hive struct {
	bins []byte
	// unitOrigin は bins の unitSize ごとの、主 file と log を連結した byte 列の中の位置である。
	// nil のときは、bins の全体が主 file の base block の直後にある。負の値は、log entry が
	// hive bins data を広げたが、どの page も書かなかった範囲である。
	unitOrigin []int64
	// unitEntry は bins の unitSize ごとの、その byte を書いた log entry の entries の位置に 1 を
	// 足した値である。0 は主 file の byte である。nil のときは全体が主 file の byte である。
	unitEntry []int32
	// entries は適用した log entry である。
	entries        []appliedEntry
	minorVersion   uint32
	rootCellOffset uint32
	// read は cellAt が返した cell の位置である。
	read map[uint32]struct{}
}

// appliedEntry は適用した log entry 1 件である。
type appliedEntry struct {
	member   string
	sequence uint32
}

// span は、主 file と log を連結した byte 列の中の連続した範囲である。
type span struct {
	offset, length int64
}

// cell は hive bins data の中の割り当て済みの cell 1 つである。
type cell struct {
	// rel は cell の先頭 (大きさの欄) の、hive bins data の先頭からの位置である。
	rel int64
	// size は大きさの欄を含む cell の byte 数である。
	size int64
	// data は大きさの欄の後の byte 列である。
	data []byte
}

// cellAt は offset が指す割り当て済みの cell を返す。
//
// **cell 1 つを 1 回だけ返す。** hive の key、list、値、data は、どれも 1 つの持ち主だけが
// 指す (sk は読まない)。2 回目の参照を失敗にし、list を共有させた細工で出力と確保が参照の
// 数の 2 乗へ増えることを防ぐ。
func (h *hive) cellAt(offset uint32) (cell, error) {
	rel := int64(offset)
	if offset == noCell || rel+4 > int64(len(h.bins)) {
		return cell{}, fmt.Errorf("the cell offset %d lies outside the hive bins data of %d bytes", offset, len(h.bins))
	}
	if _, seen := h.read[offset]; seen {
		return cell{}, fmt.Errorf("the cell at offset %d is referenced twice", offset)
	}
	if h.read == nil {
		h.read = map[uint32]struct{}{}
	}
	h.read[offset] = struct{}{}
	size := int64(int32(binary.LittleEndian.Uint32(h.bins[rel:]))) // #nosec G115 -- 大きさの欄は符号付きで、負の値が割り当て済みを表す。
	if size >= 0 {
		return cell{}, fmt.Errorf("the cell at offset %d is unallocated", offset)
	}
	size = -size
	if size < 8 || rel+size > int64(len(h.bins)) {
		return cell{}, fmt.Errorf("the cell at offset %d declares %d bytes, past the hive bins data of %d bytes",
			offset, size, len(h.bins))
	}
	for _, s := range h.spans(rel, size) {
		if s.offset < 0 {
			return cell{}, fmt.Errorf("the cell at offset %d lies in hive bins data that no file holds", offset)
		}
	}
	return cell{rel: rel, size: size, data: h.bins[rel+4 : rel+size]}, nil
}

// sourceOffset は hive bins data の rel の byte の、連結した byte 列の中の位置である。
func (h *hive) sourceOffset(rel int64) int64 {
	if h.unitOrigin == nil {
		return baseBlockSize + rel
	}
	origin := h.unitOrigin[rel/unitSize]
	if origin < 0 {
		return -1
	}
	return origin + rel%unitSize
}

// spans は hive bins data の [rel, rel+length) の byte が占める、連結した byte 列の中の範囲である。
// 範囲は bins の並びの順で、隣り合う範囲が連続するときは 1 つにまとめる。
func (h *hive) spans(rel, length int64) []span {
	if h.unitOrigin == nil {
		return []span{{offset: baseBlockSize + rel, length: length}}
	}
	var out []span
	for length > 0 {
		n := min(length, unitSize-rel%unitSize)
		offset := h.sourceOffset(rel)
		last := len(out) - 1
		if last >= 0 && offset >= 0 && out[last].offset >= 0 && out[last].offset+out[last].length == offset {
			out[last].length += n
		} else {
			out = append(out, span{offset: offset, length: n})
		}
		rel, length = rel+n, length-n
	}
	return out
}

// addEntries は [rel, rel+length) の byte を書いた log entry を into に足す。
func (h *hive) addEntries(rel, length int64, into map[int32]struct{}) {
	if h.unitEntry == nil {
		return
	}
	for unit := rel / unitSize; unit <= (rel+length-1)/unitSize; unit++ {
		if entry := h.unitEntry[unit]; entry > 0 {
			into[entry-1] = struct{}{}
		}
	}
}
