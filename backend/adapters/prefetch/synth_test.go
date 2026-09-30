// in-package test: Prefetch の file と、LZXpress Huffman の符号化を組む。
package prefetch

import (
	"encoding/binary"
	"hash/crc32"
	"math/bits"
	"slices"
	"unicode/utf16"
)

// synthFile は生成する Prefetch の file の値である。
type synthFile struct {
	version       uint32
	metricsOffset uint32
	name          string
	hash          uint32
	lastRuns      []uint64
	runCount      uint32
	files         []string
	volumes       []Volume
}

// synthFilenamesAt は生成した file の filename strings の位置である。見出しと file の情報の後ろに置く。
const synthFilenamesAt = 0x100

// build は展開した形の file の byte を組む。
func (s synthFile) build() []byte {
	layout := versionLayouts[s.version]
	content := make([]byte, synthFilenamesAt)
	binary.LittleEndian.PutUint32(content[offsetVersion:], s.version)
	copy(content[offsetSignature:], uncompressedMagic)
	copy(content[offsetExecutable:], utf16Bytes(s.name))
	binary.LittleEndian.PutUint32(content[offsetPathHash:], s.hash)
	binary.LittleEndian.PutUint32(content[offsetMetrics:], s.metricsOffset)
	for slot, at := range s.lastRuns {
		binary.LittleEndian.PutUint64(content[layout.lastRunAt+8*slot:], at)
	}
	runCountAt := layout.runCountAt
	if runCountAt == 0 {
		runCountAt = runCountByMetricsOffset[s.metricsOffset]
	}
	if runCountAt != 0 {
		binary.LittleEndian.PutUint32(content[runCountAt:], s.runCount)
	}
	var names []byte
	for _, name := range s.files {
		names = append(names, utf16Bytes(name)...)
		names = append(names, 0, 0)
	}
	binary.LittleEndian.PutUint32(content[offsetFilenames:], uint32(len(content)))
	binary.LittleEndian.PutUint32(content[offsetFilenamesSize:], uint32(len(names)))
	content = append(content, names...)
	volumesAt := len(content)
	entries := make([]byte, layout.volumeEntry*len(s.volumes))
	var paths []byte
	for index, volume := range s.volumes {
		entry := entries[index*layout.volumeEntry:]
		binary.LittleEndian.PutUint32(entry, uint32(len(entries)+len(paths)))
		binary.LittleEndian.PutUint32(entry[4:], uint32(len(utf16.Encode([]rune(volume.DevicePath)))))
		binary.LittleEndian.PutUint64(entry[8:], volume.Created)
		binary.LittleEndian.PutUint32(entry[16:], volume.Serial)
		paths = append(paths, utf16Bytes(volume.DevicePath)...)
		paths = append(paths, 0, 0)
	}
	binary.LittleEndian.PutUint32(content[offsetVolumes:], uint32(volumesAt))
	binary.LittleEndian.PutUint32(content[offsetVolumeCount:], uint32(len(s.volumes)))
	binary.LittleEndian.PutUint32(content[offsetVolumesSize:], uint32(len(entries)+len(paths)))
	content = append(content, entries...)
	content = append(content, paths...)
	binary.LittleEndian.PutUint32(content[offsetFileSize:], uint32(len(content)))
	return content
}

func utf16Bytes(text string) []byte {
	units := utf16.Encode([]rune(text))
	encoded := make([]byte, 2*len(units))
	for index, unit := range units {
		binary.LittleEndian.PutUint16(encoded[2*index:], unit)
	}
	return encoded
}

// token は符号化する 1 つの literal か match である。length が 0 の token は literal である。
type token struct {
	literal  byte
	length   int
	distance int
}

func literals(text []byte) []token {
	tokens := make([]token, len(text))
	for index, value := range text {
		tokens[index] = token{literal: value}
	}
	return tokens
}

// compressHuffman は tokens を LZXpress Huffman で符号化する。全 512 記号に長さ 9 の符号を
// 与える (符号は記号の番号そのものになる)。長さ 18 以上の match は長さの続きの byte を置く。
// 出力の 64 KiB ごとに表を置き直す。
func compressHuffman(tokens []token) []byte {
	var compressed []byte
	for len(tokens) > 0 {
		compressed = append(compressed, slices.Repeat([]byte{0x99}, huffmanTableBytes)...)
		var writer streamWriter
		for produced := 0; len(tokens) > 0 && produced < huffmanBlockBytes; tokens = tokens[1:] {
			current := tokens[0]
			if current.length == 0 {
				writer.write(uint32(current.literal), 9)
				produced++
				continue
			}
			distanceBits := bits.Len(uint(current.distance)) - 1
			length := current.length - minimumMatchLength
			writer.write(uint32(literalSymbolCount+distanceBits<<matchDistanceShift+min(length, matchLengthEscape)), 9)
			writer.lengthEscape(length)
			writer.write(uint32(current.distance-1<<distanceBits), distanceBits)
			produced += current.length
		}
		compressed = append(compressed, writer.finish()...)
	}
	return compressed
}

// streamWriter は bit を上位から 16 bit の語へ詰め、語を little endian で並べる。
//
// 復号は先に 2 語を読み、残りの bit が 16 を下回るたびに 1 語を補う。長さの続きの byte は、
// 復号がその時点までに読んだ語の直後に置く。語の場所を先に取り、bit は後から埋める。
type streamWriter struct {
	out   []byte
	slots []int
	bits  int
}

func (w *streamWriter) ensureSlots(count int) {
	for len(w.slots) < count {
		w.slots = append(w.slots, len(w.out))
		w.out = append(w.out, 0, 0)
	}
}

func (w *streamWriter) write(value uint32, count int) {
	for bit := count - 1; bit >= 0; bit-- {
		w.ensureSlots(w.bits/16 + 1)
		if value>>bit&1 == 1 {
			at, position := w.slots[w.bits/16], 15-w.bits%16
			w.out[at+position/8] |= 1 << (position % 8)
		}
		w.bits++
	}
}

// wordsRead は、復号がここまでの bit を読んだときに読み終えた語の数である。
func (w *streamWriter) wordsRead() int {
	return 2 + max(0, (w.bits-16+15)/16)
}

// lengthEscape は、最小の長さを引いた長さ length が 15 以上のとき、長さの続きの byte を置く。
func (w *streamWriter) lengthEscape(length int) {
	if length < matchLengthEscape {
		return
	}
	w.ensureSlots(w.wordsRead())
	switch {
	case length-matchLengthEscape < matchLengthByteEscape:
		w.out = append(w.out, byte(length-matchLengthEscape))
	case length <= 0xFFFF:
		w.out = binary.LittleEndian.AppendUint16(append(w.out, matchLengthByteEscape), uint16(length))
	default:
		w.out = binary.LittleEndian.AppendUint32(append(w.out, matchLengthByteEscape, 0, 0), uint32(length))
	}
}

func (w *streamWriter) finish() []byte {
	w.ensureSlots(w.wordsRead())
	return w.out
}

// compressedFile は展開した形の file を MAM の見出しで包む。checksum が真のときは CRC32 を置く。
func compressedFile(content []byte, checksum bool) []byte {
	body := compressHuffman(literals(content))
	header := []byte{'M', 'A', 'M', compressionHuffman, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(header[4:], uint32(len(content)))
	if !checksum {
		return append(header, body...)
	}
	header[3] |= compressionChecksumFlag
	file := slices.Concat(header, make([]byte, checksumBytes), body)
	binary.LittleEndian.PutUint32(file[compressedHeaderBytes:], crc32.ChecksumIEEE(file))
	return file
}
