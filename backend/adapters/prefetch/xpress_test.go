// in-package test: LZXpress Huffman の展開を確かめる。
package prefetch

import (
	"bytes"
	"errors"
	"slices"
	"testing"
)

// applyTokens は tokens が表す byte 列を組む。
func applyTokens(tokens []token) []byte {
	var output []byte
	for _, current := range tokens {
		if current.length == 0 {
			output = append(output, current.literal)
			continue
		}
		start := len(output) - current.distance
		for index := range current.length {
			output = append(output, output[start+index])
		}
	}
	return output
}

// literal と、重なる match と、重ならない match を元の byte に戻す。
func TestDecompressHuffmanRestoresLiteralsAndMatches(t *testing.T) {
	for name, tokens := range map[string][]token{
		"literals":          literals([]byte("synthetic prefetch body")),
		"overlapping match": append(literals([]byte("ab")), token{length: 9, distance: 2}),
		"distance of one":   append(literals([]byte("z")), token{length: 17, distance: 1}),
		"separate match":    append(literals([]byte("abcdefgh-")), token{length: 8, distance: 9}),
	} {
		want := applyTokens(tokens)
		got, err := decompressHuffman(compressHuffman(tokens), len(want))
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s: got %q (%v), want %q", name, got, err, want)
		}
	}
}

// 長さ 18 以上の match は、1 byte、16 bit、32 bit の続きの欄から長さを読む。
func TestDecompressHuffmanReadsTheLengthEscapes(t *testing.T) {
	for _, length := range []int{18, 272, 273, 300, 70000} {
		tokens := []token{{literal: 'q'}, {length: length, distance: 1}, {literal: 'r'}}
		want := applyTokens(tokens)
		got, err := decompressHuffman(compressHuffman(tokens), len(want))
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("length %d: got %d bytes (%v), want %d bytes equal to the tokens", length, len(got), err, len(want))
		}
	}
}

// 32 bit の長さの欄が書く長さは、指定の大きさの領域の外を確保させない。
func TestDecompressHuffmanBoundsAMatchByTheStatedSize(t *testing.T) {
	compressed := compressHuffman([]token{{literal: 'q'}, {length: 200_000_000, distance: 1}})
	got, err := decompressHuffman(compressed, 10)
	if err != nil || string(got) != "qqqqqqqqqq" || cap(got) != 10 {
		t.Errorf("got %q with capacity %d (%v), want 10 bytes in a 10 byte buffer", got, cap(got), err)
	}
}

// 16 bit の長さの欄が 15 未満の match と、符号がすべての並びを覆わない表は失敗する。
func TestDecompressHuffmanRejectsInvalidFields(t *testing.T) {
	var writer streamWriter
	writer.write('q', 9)
	writer.write(literalSymbolCount+matchLengthEscape, 9)
	writer.ensureSlots(writer.wordsRead())
	writer.out = append(writer.out, matchLengthByteEscape, 14, 0)
	shortLength := append(slices.Repeat([]byte{0x99}, huffmanTableBytes), writer.finish()...)
	if _, err := decompressHuffman(shortLength, 40); !errors.Is(err, errMatchLength) {
		t.Errorf("a 16 bit length of 14: err = %v, want %v", err, errMatchLength)
	}
	incomplete := append([]byte{0x01}, make([]byte, huffmanTableBytes+4)...)
	if _, err := decompressHuffman(incomplete, 1); !errors.Is(err, errInvalidCodeTable) {
		t.Errorf("a table with one code of length 1: err = %v, want %v", err, errInvalidCodeTable)
	}
}

// 出力の 64 KiB ごとに表を読み直す。区切りをまたぐ match の後も、次の表から読む。
func TestDecompressHuffmanReadsATableForEveryBlock(t *testing.T) {
	tokens := literals([]byte("x"))
	for len(applyTokens(tokens)) < huffmanBlockBytes+100 {
		tokens = append(tokens, token{length: 16, distance: 1})
	}
	tokens = append(tokens, literals([]byte("second block"))...)
	want := applyTokens(tokens)
	if (huffmanBlockBytes-1)%16 == 0 {
		t.Fatal("the matches end on the block boundary; the test needs a match that crosses it")
	}
	got, err := decompressHuffman(compressHuffman(tokens), len(want))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("got %d bytes (%v), want %d bytes equal to the tokens", len(got), err, len(want))
	}
}

// 最後の match が指定の大きさの外まで書いた分は捨てる。
func TestDecompressHuffmanStopsAtTheStatedSize(t *testing.T) {
	tokens := append(literals([]byte("ab")), token{length: 6, distance: 2})
	got, err := decompressHuffman(compressHuffman(tokens), 5)
	if err != nil || string(got) != "ababa" {
		t.Errorf("got %q (%v), want the first 5 bytes", got, err)
	}
}

// 途中で切れた入力、表の後ろの流れが短すぎる入力、出力の前を指す match は失敗する。
func TestDecompressHuffmanRejectsBrokenStreams(t *testing.T) {
	text := bytes.Repeat([]byte("synthetic"), 40)
	compressed := compressHuffman(literals(text))
	for name, testCase := range map[string]struct {
		input []byte
		size  int
		want  error
	}{
		"table cut":         {compressed[:100], len(text), errTruncatedInput},
		"stream cut":        {compressed[:huffmanTableBytes+40], len(text), errTruncatedInput},
		"match before data": {compressHuffman([]token{{length: 4, distance: 2}}), 4, errMatchBeforeOutput},
		"empty table":       {make([]byte, huffmanTableBytes+4), 1, errInvalidCodeTable},
	} {
		if _, err := decompressHuffman(testCase.input, testCase.size); !errors.Is(err, testCase.want) {
			t.Errorf("%s: err = %v, want %v", name, err, testCase.want)
		}
	}
}
