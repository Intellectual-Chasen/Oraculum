package prefetch

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"slices"
	"strings"
	"unicode/utf16"
)

// 圧縮した file の見出し。`MAM` に続く 1 byte の下位 4 bit が圧縮の方式、上位の bit が
// CRC32 の有無である。続く 4 byte が展開後の大きさ、CRC32 を持つ形ではその後ろの 4 byte が CRC32 である。
const (
	compressedSignature     = "MAM"
	compressionHuffman      = 0x04
	compressionChecksumFlag = 0x80
	compressedHeaderBytes   = 8
	checksumBytes           = 4
)

// 展開した file の見出しの位置。
const (
	offsetVersion       = 0x00
	offsetSignature     = 0x04
	offsetFileSize      = 0x0C
	offsetExecutable    = 0x10
	executableNameBytes = 60
	// executableNameUnits は、名前の欄が 0 の終端の前に置ける UTF-16 の単位の数である。
	executableNameUnits = executableNameBytes/2 - 1
	offsetPathHash      = 0x4C
	offsetMetrics       = 0x54
	offsetFilenames     = 0x64
	offsetFilenamesSize = 0x68
	offsetVolumes       = 0x6C
	offsetVolumeCount   = 0x70
	offsetVolumesSize   = 0x74
	uncompressedMagic   = "SCCA"
	// fileTimeBytes は 1 つの FILETIME の byte 数、runCountBytes は実行回数の byte 数である。
	fileTimeBytes = 8
	runCountBytes = 4
)

// 形式の番号ごとの、最終実行時刻と実行回数の位置と、volume の情報の 1 件の大きさ。
type versionLayout struct {
	lastRunAt   int
	lastRunSlot int
	runCountAt  int
	volumeEntry int
}

var versionLayouts = map[uint32]versionLayout{
	17: {lastRunAt: 0x78, lastRunSlot: 1, runCountAt: 0x90, volumeEntry: 40},
	23: {lastRunAt: 0x80, lastRunSlot: 1, runCountAt: 0x98, volumeEntry: 104},
	26: {lastRunAt: 0x80, lastRunSlot: 8, runCountAt: 0xD0, volumeEntry: 104},
	30: {lastRunAt: 0x80, lastRunSlot: 8, volumeEntry: 96},
	31: {lastRunAt: 0x80, lastRunSlot: 8, volumeEntry: 96},
}

// runCountByMetricsOffset は、形式 30 と 31 の実行回数の位置を metrics の位置から求める。
// 2 つの配置は、実行回数の前に置く欄の大きさが 8 byte 違う。
var runCountByMetricsOffset = map[uint32]int{0x128: 0xC8, 0x130: 0xD0}

var (
	errUnsupportedCompression = errors.New("the compression method is not LZXpress Huffman")
	errChecksum               = errors.New("the CRC32 of the compressed file does not match")
	errSizeMismatch           = errors.New("the expanded size differs from the size in the header")
	errSignature              = errors.New("the file does not start with a Prefetch signature")
	errUnsupportedVersion     = errors.New("the Prefetch format version is not supported")
	errOutOfRange             = errors.New("a table of the file points outside the file")
	// errExpandedTooLarge は、見出しが書く展開後の大きさが maxFileBytes を超える失敗である。
	// 展開の前に出力の領域を確保するため、見出しの値だけで大きな領域を確保しない。
	errExpandedTooLarge = errors.New("the expanded size exceeds the byte limit")
)

// File は 1 つの Prefetch の file から読んだ値である。
type File struct {
	// Compressed は file が MAM の見出しで圧縮されていたかである。
	Compressed bool
	// Version は形式の番号である。
	Version uint32
	// ExecutableName は実行ファイルの名前である。欄は 29 文字までを持つ。
	ExecutableName string
	// PathHash は、実行ファイルの path から Windows が求めた値である。
	PathHash uint32
	// LastRuns は新しい順の最終実行時刻の FILETIME である。0 の枠は含まない。
	LastRuns []uint64
	// RunCount は実行回数である。RunCountKnown が偽のときは位置を決められなかった。
	RunCount      uint32
	RunCountKnown bool
	// ReferencedFiles は、起動のときに読んだ file の path である。
	ReferencedFiles []string
	// Volumes は、参照した file を置いた volume である。
	Volumes []Volume
}

// Volume は 1 つの volume の情報である。
type Volume struct {
	DevicePath string
	Serial     uint32
	// Created は volume の作成時刻の FILETIME である。
	Created uint64
}

// parseFile は file の全体の byte を読む。
func parseFile(content []byte) (File, error) {
	var file File
	if len(content) >= compressedHeaderBytes && string(content[:len(compressedSignature)]) == compressedSignature {
		expanded, err := expand(content)
		if err != nil {
			return File{}, err
		}
		content, file.Compressed = expanded, true
	}
	if len(content) < offsetVolumesSize+4 || string(content[offsetSignature:offsetSignature+4]) != uncompressedMagic {
		return File{}, errSignature
	}
	file.Version = binary.LittleEndian.Uint32(content[offsetVersion:])
	layout, known := versionLayouts[file.Version]
	if !known {
		return File{}, fmt.Errorf("version %d: %w", file.Version, errUnsupportedVersion)
	}
	if size := binary.LittleEndian.Uint32(content[offsetFileSize:]); int(size) != len(content) {
		return File{}, fmt.Errorf("the header states %d bytes and the file has %d: %w", size, len(content), errSizeMismatch)
	}
	file.ExecutableName = utf16String(content[offsetExecutable : offsetExecutable+executableNameBytes])
	file.PathHash = binary.LittleEndian.Uint32(content[offsetPathHash:])
	runCountAt := layout.runCountAt
	if runCountAt == 0 {
		runCountAt = runCountByMetricsOffset[binary.LittleEndian.Uint32(content[offsetMetrics:])]
	}
	if end := max(layout.lastRunAt+fileTimeBytes*layout.lastRunSlot, runCountAt+runCountBytes); end > len(content) {
		return File{}, fmt.Errorf("the file information ends at byte %d: %w", end, errOutOfRange)
	}
	for slot := range layout.lastRunSlot {
		if at := binary.LittleEndian.Uint64(content[layout.lastRunAt+fileTimeBytes*slot:]); at != 0 {
			file.LastRuns = append(file.LastRuns, at)
		}
	}
	if runCountAt != 0 {
		file.RunCount, file.RunCountKnown = binary.LittleEndian.Uint32(content[runCountAt:]), true
	}
	var err error
	if file.ReferencedFiles, err = referencedFiles(content); err != nil {
		return File{}, err
	}
	if file.Volumes, err = volumes(content, layout.volumeEntry); err != nil {
		return File{}, err
	}
	return file, nil
}

// expand は MAM の見出しを持つ file を展開する。
func expand(content []byte) ([]byte, error) {
	method := content[len(compressedSignature)]
	if method&^compressionChecksumFlag != compressionHuffman {
		return nil, fmt.Errorf("method 0x%02X: %w", method, errUnsupportedCompression)
	}
	// int へ直す前に上限と比べる。32 bit の int では大きな値が負になる。
	stated := binary.LittleEndian.Uint32(content[4:])
	if stated > maxFileBytes {
		return nil, fmt.Errorf("the header states %d expanded bytes: %w", stated, errExpandedTooLarge)
	}
	size := int(stated)
	body := content[compressedHeaderBytes:]
	if method&compressionChecksumFlag != 0 {
		if len(body) < checksumBytes {
			return nil, errTruncatedInput
		}
		checksum := binary.LittleEndian.Uint32(body)
		// CRC32 は、CRC32 の欄を 0 にした見出しと圧縮した本体から求める。
		checked := slices.Concat(content[:compressedHeaderBytes], make([]byte, checksumBytes), body[checksumBytes:])
		if crc32.ChecksumIEEE(checked) != checksum {
			return nil, errChecksum
		}
		body = body[checksumBytes:]
	}
	expanded, err := decompressHuffman(body, size)
	if err != nil {
		return nil, err
	}
	if len(expanded) != size {
		return nil, fmt.Errorf("the header states %d bytes and the stream expands to %d: %w", size, len(expanded), errSizeMismatch)
	}
	return expanded, nil
}

// referencedFiles は filename strings の節を、0 で終わる UTF-16 の path の並びとして読む。
func referencedFiles(content []byte) ([]string, error) {
	section, err := sectionOf(content, offsetFilenames, offsetFilenamesSize)
	if err != nil {
		return nil, fmt.Errorf("the filename strings: %w", err)
	}
	var paths []string
	for start := 0; start+1 < len(section); {
		end := start
		for end+1 < len(section) && (section[end] != 0 || section[end+1] != 0) {
			end += 2
		}
		if end > start {
			paths = append(paths, utf16String(section[start:end]))
		}
		start = end + 2
	}
	return paths, nil
}

// volumes は volume の情報の節を読む。device path の位置は節の先頭から数える。
func volumes(content []byte, entrySize int) ([]Volume, error) {
	section, err := sectionOf(content, offsetVolumes, offsetVolumesSize)
	if err != nil {
		return nil, fmt.Errorf("the volume information: %w", err)
	}
	// 位置と数は uint64 で比べてから int へ直す。32 bit の int では掛け算と足し算が溢れる。
	stated := uint64(binary.LittleEndian.Uint32(content[offsetVolumeCount:]))
	if stated*uint64(entrySize) > uint64(len(section)) { // #nosec G115 -- entrySize は versionLayouts の正の定数である。
		return nil, fmt.Errorf("%d volumes of %d bytes in %d bytes: %w", stated, entrySize, len(section), errOutOfRange)
	}
	count := int(stated) // #nosec G115 -- 数は節の byte 数を 1 件の大きさで割った値以下であり、int に収まる。
	found := make([]Volume, 0, count)
	for index := range count {
		entry := section[index*entrySize:]
		pathAt := uint64(binary.LittleEndian.Uint32(entry))
		pathEnd := pathAt + 2*uint64(binary.LittleEndian.Uint32(entry[4:]))
		if pathEnd > uint64(len(section)) {
			return nil, fmt.Errorf("the device path of volume %d: %w", index, errOutOfRange)
		}
		found = append(found, Volume{
			DevicePath: utf16String(section[pathAt:pathEnd]),
			Created:    binary.LittleEndian.Uint64(entry[8:]),
			Serial:     binary.LittleEndian.Uint32(entry[16:]),
		})
	}
	return found, nil
}

// sectionOf は、位置の欄と大きさの欄が指す節を返す。
func sectionOf(content []byte, offsetAt, sizeAt int) ([]byte, error) {
	start := uint64(binary.LittleEndian.Uint32(content[offsetAt:]))
	end := start + uint64(binary.LittleEndian.Uint32(content[sizeAt:]))
	if end > uint64(len(content)) {
		return nil, fmt.Errorf("bytes %d to %d of %d: %w", start, end, len(content), errOutOfRange)
	}
	return content[start:end], nil
}

// utf16String は UTF-16 LE の byte 列を、最初の 0 の文字の前までの文字列にする。
func utf16String(content []byte) string {
	units := make([]uint16, 0, len(content)/2)
	for index := 0; index+1 < len(content); index += 2 {
		unit := binary.LittleEndian.Uint16(content[index:])
		if unit == 0 {
			break
		}
		units = append(units, unit)
	}
	return strings.ToValidUTF8(string(utf16.Decode(units)), "�")
}
