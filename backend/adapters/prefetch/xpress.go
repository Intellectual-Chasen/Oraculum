package prefetch

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// LZXpress Huffman (MS-XCA 2.2.4) の定数。
const (
	// huffmanSymbolCount は記号の数である。0 から 255 が literal、256 から 511 が match である。
	huffmanSymbolCount = 512
	// huffmanTableBytes は、記号ごとの符号の長さを 4 bit ずつ並べた表の byte 数である。
	huffmanTableBytes = huffmanSymbolCount / 2
	// huffmanMaxCodeLength は符号の長さの上限である。
	huffmanMaxCodeLength = 15
	// huffmanBlockBytes は、1 つの表で展開する出力の byte 数である。
	huffmanBlockBytes = 1 << 16
	// minimumMatchLength は match の長さの最小である。
	minimumMatchLength = 3
	// maxLookaheadBytes は、復号が記号を読む前に補っておく bit の byte 数である。圧縮した
	// 流れの終わりでは、この分だけ input の後ろを 0 として読む。
	maxLookaheadBytes = 4
	// literalSymbolCount は literal の記号の数である。これ以上の記号は match である。
	literalSymbolCount = 256
	// matchLengthEscape は、match の記号の長さの欄が、長さの続きの byte を置くことを示す値である。
	matchLengthEscape = 0x0F
	// matchLengthByteEscape は、長さの続きの byte が、16 bit の長さを置くことを示す値である。
	matchLengthByteEscape = 0xFF
	// matchDistanceShift は、match の記号のうち距離の bit 数を置く位置である。
	matchDistanceShift = 4
	// codeLengthBits は、表の 1 つの記号の符号の長さの bit 数である。
	codeLengthBits = 4
	codeLengthMask = 1<<codeLengthBits - 1
)

var (
	errTruncatedInput    = errors.New("the compressed stream ends before the output is complete")
	errInvalidCodeTable  = errors.New("the code lengths do not form a complete prefix code")
	errMatchBeforeOutput = errors.New("a match refers to bytes before the start of the output")
	errMatchLength       = errors.New("a match length field is shorter than its escape value")
)

// decompressHuffman は LZXpress Huffman で圧縮した input を outputSize byte へ展開する。
//
// 出力の 64 KiB ごとに、input の現在の位置から 256 byte の符号の長さの表を読み直す。
// match は区切りをまたいで次の区切りの分まで書くことがある。区切りの後の表は、その
// 区切りまでに読んだ 16 bit の語の直後から始まる。
func decompressHuffman(input []byte, outputSize int) ([]byte, error) {
	output := make([]byte, 0, outputSize)
	position := 0
	for len(output) < outputSize {
		if position+huffmanTableBytes > len(input) {
			return nil, fmt.Errorf("reading the code table at byte %d: %w", position, errTruncatedInput)
		}
		table, err := newDecodingTable(input[position : position+huffmanTableBytes])
		if err != nil {
			return nil, fmt.Errorf("reading the code table at byte %d: %w", position, err)
		}
		stream := bitStream{input: input, position: position + huffmanTableBytes}
		stream.next = uint32(stream.word())<<16 | uint32(stream.word())
		stream.extra = 16
		blockEnd := min(len(output)+huffmanBlockBytes, outputSize)
		for len(output) < blockEnd {
			if output, err = decodeSymbol(&stream, table, output); err != nil {
				return nil, err
			}
		}
		if stream.overrun > maxLookaheadBytes {
			return nil, fmt.Errorf("decoding the block that ends at output byte %d: %w", len(output), errTruncatedInput)
		}
		position = stream.position
	}
	// decodeSymbol は領域 (cap = outputSize) の外へ書かない。
	return output, nil
}

// decodeSymbol は 1 つの記号を読み、literal か match の byte を output の後ろへ足す。
func decodeSymbol(stream *bitStream, table *decodingTable, output []byte) ([]byte, error) {
	// 表は符号ですべての並びを覆うため (newDecodingTable)、どの 15 bit も記号を求められる。
	entry := table.entries[stream.next>>(32-huffmanMaxCodeLength)]
	stream.consume(int(entry.length))
	if entry.symbol < literalSymbolCount {
		return append(output, byte(entry.symbol)), nil
	}
	symbol := int(entry.symbol) - literalSymbolCount
	length := symbol & matchLengthEscape
	distanceBits := symbol >> matchDistanceShift
	// 長さの続きの byte は、距離の bit を読む前の位置にある。距離の bit を読むと語を補い、位置が進む。
	if length == matchLengthEscape {
		extended, err := stream.matchLength()
		if err != nil {
			return nil, err
		}
		length = extended
	}
	length += minimumMatchLength
	// 距離の bit 数は 15 以下であり、32 から引いた shift の後の値は int に収まる。bit 数 0 の
	// shift は 32 になり、uint32 の値は 0 になる。
	distance := 1<<distanceBits | int(stream.next>>(32-distanceBits))
	stream.consume(distanceBits)
	if distance > len(output) {
		return nil, fmt.Errorf("decoding a match of distance %d at output byte %d: %w",
			distance, len(output), errMatchBeforeOutput)
	}
	// 最後の match は指定の大きさの外まで書くことがある。Windows の展開は出力の領域の終わりで
	// 止まるため、領域 (cap) の残りまでを写す。32 bit の長さの欄が大きな領域を確保させない。
	length = min(length, cap(output)-len(output))
	// 重なる match は、書いたばかりの byte を読み返すため 1 byte ずつ写す。
	start := len(output) - distance
	for index := range length {
		output = append(output, output[start+index])
	}
	return output, nil
}

// bitStream は 16 bit の語を little endian で読み、上位の bit から記号を取り出す。
type bitStream struct {
	input    []byte
	position int
	// next は、上位の bit から読む 32 bit である。
	next uint32
	// extra は、next の下位にまだ補っていない bit の数である。負になったとき 16 bit を補う。
	extra int
	// overrun は、input の終わりの後ろを読んだ byte の数である。
	overrun int
}

// word は次の 16 bit の語を読む。input の終わりの後ろは 0 として読み、overrun に数える。
func (s *bitStream) word() uint16 {
	if s.position+2 > len(s.input) {
		s.overrun += s.position + 2 - max(s.position, len(s.input))
		s.position += 2
		return 0
	}
	value := binary.LittleEndian.Uint16(s.input[s.position:])
	s.position += 2
	return value
}

// consume は上位の count bit を捨て、足りなくなった bit を補う。
func (s *bitStream) consume(count int) {
	if count == 0 {
		return
	}
	s.next <<= count
	s.extra -= count
	if s.extra < 0 {
		s.next |= uint32(s.word()) << -s.extra
		s.extra += 16
	}
}

// matchLength は、長さの欄が 15 の match が続けて置く byte から長さを読む。返す値は
// 最小の長さを足す前の値である。
func (s *bitStream) matchLength() (int, error) {
	if s.position+1 > len(s.input) {
		return 0, fmt.Errorf("reading a match length at compressed byte %d: %w", s.position, errTruncatedInput)
	}
	length := int(s.input[s.position])
	s.position++
	if length < matchLengthByteEscape {
		return length + matchLengthEscape, nil
	}
	if s.position+2 > len(s.input) {
		return 0, fmt.Errorf("reading a match length at compressed byte %d: %w", s.position, errTruncatedInput)
	}
	length = int(binary.LittleEndian.Uint16(s.input[s.position:]))
	s.position += 2
	if length == 0 {
		if s.position+4 > len(s.input) {
			return 0, fmt.Errorf("reading a match length at compressed byte %d: %w", s.position, errTruncatedInput)
		}
		length = int(binary.LittleEndian.Uint32(s.input[s.position:]))
		s.position += 4
	}
	if length < matchLengthEscape {
		return 0, fmt.Errorf("reading a match length of %d at compressed byte %d: %w", length, s.position, errMatchLength)
	}
	// 16 bit と 32 bit の欄は、最小の長さを足す前の長さそのものを書く。
	return length, nil
}

// decodingEntry は、上位 15 bit が指す記号と、その符号の長さである。
type decodingEntry struct {
	symbol uint16
	length uint8
}

// decodingTable は上位 15 bit から記号を求める表である。
type decodingTable struct {
	entries [1 << huffmanMaxCodeLength]decodingEntry
}

// newDecodingTable は、記号ごとの符号の長さから canonical Huffman の表を組む。符号は長さの
// 短い順、同じ長さでは記号の小さい順に割り当てる。
func newDecodingTable(lengths []byte) (*decodingTable, error) {
	table := &decodingTable{}
	next := 0
	for length := 1; length <= huffmanMaxCodeLength; length++ {
		span := 1 << (huffmanMaxCodeLength - length)
		for symbol := range huffmanSymbolCount {
			if codeLength(lengths, symbol) != length {
				continue
			}
			if next+span > len(table.entries) {
				return nil, errInvalidCodeTable
			}
			for index := next; index < next+span; index++ {
				table.entries[index] = decodingEntry{symbol: uint16(symbol), length: uint8(length)}
			}
			next += span
		}
	}
	// MS-XCA は、符号がすべての bit の並びを覆う表だけを受ける。
	if next != len(table.entries) {
		return nil, errInvalidCodeTable
	}
	return table, nil
}

// codeLength は表の中の記号 symbol の符号の長さを返す。偶数の記号は下位の 4 bit、奇数の
// 記号は上位の 4 bit に置かれる。
func codeLength(lengths []byte, symbol int) int {
	packed := lengths[symbol/2]
	if symbol%2 == 0 {
		return int(packed & codeLengthMask)
	}
	return int(packed >> codeLengthBits)
}
