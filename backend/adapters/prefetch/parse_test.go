// in-package test: Prefetch の file を読む。
package prefetch

import (
	"encoding/binary"
	"errors"
	"slices"
	"testing"
)

// synthRuns は新しい順の 8 つの実行時刻である。最後の 2 枠は 0 である。
var synthRuns = []uint64{
	128000000000000000, 127900000000000000, 127800000000000000, 127700000000000000,
	127600000000000000, 127500000000000000, 0, 0,
}

func sampleFile(version, metricsOffset uint32) synthFile {
	return synthFile{
		version: version, metricsOffset: metricsOffset, name: "TOOL51.EXE", hash: 0x0A0B0C0D,
		lastRuns: synthRuns[:versionLayouts[version].lastRunSlot], runCount: 7,
		files: []string{
			`\VOLUME{00000000000000a1-0000b2c3}\WINDOWS\SYSTEM32\NTDLL.DLL`,
			`\VOLUME{00000000000000a1-0000b2c3}\DATA\TOOL51.EXE`,
		},
		volumes: []Volume{{DevicePath: `\VOLUME{00000000000000a1-0000b2c3}`, Serial: 0xB2C3D4E5, Created: 127000000000000000}},
	}
}

// 形式ごとの位置から、名前・hash・実行時刻・実行回数・参照した file・volume を読む。0 の枠は読まない。
func TestParseFileReadsEveryVersion(t *testing.T) {
	for _, testCase := range []struct {
		version, metricsOffset uint32
		runs                   int
	}{{17, 0x98, 1}, {23, 0xF0, 1}, {26, 0x130, 6}, {30, 0x128, 6}, {30, 0x130, 6}, {31, 0x128, 6}} {
		synth := sampleFile(testCase.version, testCase.metricsOffset)
		file, err := parseFile(synth.build())
		if err != nil {
			t.Fatalf("version %d: %v", testCase.version, err)
		}
		if file.Version != testCase.version || file.ExecutableName != synth.name || file.PathHash != synth.hash ||
			!file.RunCountKnown || file.RunCount != synth.runCount || file.Compressed {
			t.Errorf("version %d: %+v", testCase.version, file)
		}
		if !slices.Equal(file.LastRuns, synthRuns[:testCase.runs]) || !slices.Equal(file.ReferencedFiles, synth.files) ||
			!slices.Equal(file.Volumes, synth.volumes) {
			t.Errorf("version %d: runs %v files %q volumes %+v", testCase.version, file.LastRuns,
				file.ReferencedFiles, file.Volumes)
		}
	}
}

// 形式 30 の metrics の位置が既知の 2 つの配置のどちらでもない file は、実行回数だけを読まず、
// ほかの値を読む。
func TestParseFileLeavesTheRunCountOfAnUnknownLayout(t *testing.T) {
	file, err := parseFile(sampleFile(30, 0x140).build())
	if err != nil || file.RunCountKnown || file.ExecutableName != "TOOL51.EXE" || len(file.LastRuns) != 6 {
		t.Errorf("file %+v (%v), want the values except the run count", file, err)
	}
}

// MAM の見出しを持つ file を展開して読む。CRC32 を持つ形は CRC32 を照合する。
func TestParseFileExpandsCompressedFiles(t *testing.T) {
	content := sampleFile(30, 0x130).build()
	for _, checksum := range []bool{false, true} {
		file, err := parseFile(compressedFile(content, checksum))
		if err != nil || !file.Compressed || file.ExecutableName != "TOOL51.EXE" {
			t.Errorf("checksum %t: %+v (%v)", checksum, file, err)
		}
	}
	damaged := compressedFile(content, true)
	damaged[len(damaged)-1] ^= 0xFF
	if _, err := parseFile(damaged); !errors.Is(err, errChecksum) {
		t.Errorf("a damaged checksummed file: %v, want %v", err, errChecksum)
	}
}

// 署名・形式の番号・圧縮の方式・大きさ・表の範囲が合わない file は失敗する。
func TestParseFileRejectsInconsistentFiles(t *testing.T) {
	valid := sampleFile(30, 0x130).build()
	modified := func(change func([]byte) []byte) []byte { return change(slices.Clone(valid)) }
	for name, testCase := range map[string]struct {
		content []byte
		want    error
	}{
		"signature": {modified(func(c []byte) []byte { copy(c[offsetSignature:], "XXXX"); return c }), errSignature},
		"short":     {valid[:0x20], errSignature},
		"version": {modified(func(c []byte) []byte {
			binary.LittleEndian.PutUint32(c[offsetVersion:], 99)
			return c
		}), errUnsupportedVersion},
		"size": {modified(func(c []byte) []byte { return append(c, 0) }), errSizeMismatch},
		"filenames outside": {modified(func(c []byte) []byte {
			binary.LittleEndian.PutUint32(c[offsetFilenamesSize:], 0xFFFF)
			return c
		}), errOutOfRange},
		"volumes outside": {modified(func(c []byte) []byte {
			binary.LittleEndian.PutUint32(c[offsetVolumeCount:], 50)
			return c
		}), errOutOfRange},
		"compression method": {modified(func(c []byte) []byte {
			return append([]byte{'M', 'A', 'M', 0x02, 0, 0, 0, 0}, c...)
		}), errUnsupportedCompression},
		"expanded size": {[]byte{'M', 'A', 'M', compressionHuffman, 0, 0, 0, 0x80}, errExpandedTooLarge},
	} {
		if _, err := parseFile(testCase.content); !errors.Is(err, testCase.want) {
			t.Errorf("%s: err = %v, want %v", name, err, testCase.want)
		}
	}
}
