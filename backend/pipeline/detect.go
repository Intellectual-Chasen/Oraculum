package pipeline

import (
	"bytes"
	"errors"
	"io"
	"maps"
	"slices"
	"unicode"
	"unicode/utf8"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// detectionSampleBytes は、入力形式の候補を決めるために読む file の先頭の byte 数である。
const detectionSampleBytes = 64 << 10

// detectionRecordLimit は、テキストの標本から試しに読むレコードの上限である。
const detectionRecordLimit = 64

// formatDetector は、file の先頭の標本から、その file を読める入力形式の候補を決める。
//
// **候補を決めるのは入力形式を読むパーサーである。** 署名を持つ形式 (SignedParser) は先頭の
// byte 列の署名で決め、テキストの形式は標本をパーサーで試しに読み、読めた形式を候補にする。
// 候補は読み込みの要求の既定値であり、読み込みは要求が明示した形式で行う (SourcePlan.FormatKey)。
//
// **SignedParser を持つ形式も、署名が当たらなければ試し読みする。** 同じパーサーが署名を持つ
// 形式 (EVTX) と持たない形式 (XML・CSV) を読む (binding_winevent.go)。
type formatDetector struct {
	formats []detectableFormat
}

// detectableFormat は、候補の判定に使う入力形式 1 つである。
type detectableFormat struct {
	key     core.FormatKey
	factory ParserFactory
	signed  SignedParser
	// suffixes は、主 file と一緒に読む付属の file の接尾辞である (CompanionFileParser)。
	suffixes []string
}

// newFormatDetector は、パーサーの表から判定器を作る。欄の並びの指定を求める形式は、指定なしでは
// 作れないため候補にしない。
func newFormatDetector(parsers map[core.FormatKey]ParserFactory) formatDetector {
	var formats []detectableFormat
	for _, key := range slices.Sorted(maps.Keys(parsers)) {
		parser, err := parsers[key](nil)
		if err != nil {
			continue
		}
		format := detectableFormat{key: key, factory: parsers[key]}
		if signed, ok := parser.(SignedParser); ok {
			format.signed = signed
		}
		if companion, ok := parser.(CompanionFileParser); ok {
			format.suffixes = companion.CompanionSuffixes()
		}
		formats = append(formats, format)
	}
	return formatDetector{formats: formats}
}

// detection は file 1 つの判定の結果である。
type detection struct {
	// candidates は読める入力形式の候補を、識別子の順に並べたものである。
	candidates []core.FormatKey
	// detectedKind は、候補が無い file の先頭の byte 列から分かった種類である
	// (core.SkippedFile.DetectedKind)。
	detectedKind string
}

// detect は、file の先頭の標本 sample から入力形式の候補を返す。truncated は、標本が file の
// 途中で切れているかである。
//
// **署名を持つ形式が 1 つでも当たれば、テキストの試し読みをしない。** 署名は byte 列の完全な
// 一致であり、試し読みより強い根拠である。
func (d formatDetector) detect(sample []byte, truncated bool) detection {
	head := sample[:min(len(sample), collectionHeadBytes)]
	var signed []core.FormatKey
	for _, format := range d.formats {
		if format.signed != nil && format.signed.HasSignature(head) {
			signed = append(signed, format.key)
		}
	}
	if len(signed) > 0 {
		return detection{candidates: signed}
	}
	text, ok := textSample(sample, truncated)
	if !ok {
		return detection{detectedKind: detectedKindOf(head)}
	}
	var candidates []core.FormatKey
	for _, format := range d.formats {
		// 付属の file を持つ形式は、主 file と付属の file の組で読む。標本だけでは試せない。
		if len(format.suffixes) > 0 {
			continue
		}
		if readsSample(format.factory, text) {
			candidates = append(candidates, format.key)
		}
	}
	if len(candidates) == 0 {
		return detection{detectedKind: detectedKindOf(head)}
	}
	return detection{candidates: candidates}
}

// companionSuffixes は、入力形式 key が主 file と一緒に読む付属の file の接尾辞を返す。
func (d formatDetector) companionSuffixes(key core.FormatKey) []string {
	for _, format := range d.formats {
		if format.key == key {
			return format.suffixes
		}
	}
	return nil
}

// textSample は、標本がテキストであれば、試し読みに渡す byte 列を返す。テキストは、UTF-8 として
// 読めて、tab・改行・情報区切りの制御文字 (0x1C から 0x1F) のほかに制御文字を持たない byte 列である。
//
// **情報区切りの制御文字をテキストに含める。** auditd の拡張形式は、解釈した値の前に 0x1D を置く。
//
// 途中で切れた標本は、最後の改行までに縮める。切れた行と、切れた複数 byte の文字を試し読みに
// 渡さない。**改行を持たない切れた標本はテキストとして扱わない。** 1 行が標本の大きさを超える
// file は、行を単位に読む形式のどれにも当たらない。
func textSample(sample []byte, truncated bool) ([]byte, bool) {
	text := sample
	if truncated {
		end := bytes.LastIndexByte(text, '\n')
		if end < 0 {
			return nil, false
		}
		text = text[:end+1]
	}
	text = bytes.TrimPrefix(text, []byte("\xef\xbb\xbf"))
	if len(bytes.TrimSpace(text)) == 0 || !utf8.Valid(text) || bytes.ContainsFunc(text, isBinaryControl) {
		return nil, false
	}
	return text, true
}

// isBinaryControl は、テキストの標本が持たない制御文字であるかを返す。
func isBinaryControl(r rune) bool {
	switch {
	case r == '\t' || r == '\n' || r == '\r':
		return false
	case r >= 0x1c && r <= 0x1f:
		return false
	default:
		return unicode.IsControl(r)
	}
}

// readsSample は、標本の先頭から detectionRecordLimit 件までのレコードのうち、形式のパーサーが
// 読めた件数が、読めなかった件数を上回るかを返す。
//
// **一部のレコードの失敗を許す。** 収集元は、途中で切れた記録や壊れた行を持ちうる。形式の
// 合わないパーサーは、単一の形式の標本のレコードを 1 件も読めない (detect_test.go)。
//
// **パーサーの panic と読み込みの失敗を、その形式が読めない結果にする。** 判定は分析者が選んだ
// file すべてに当たり、形式と合わない byte 列を読むことが前提である。
func readsSample(factory ParserFactory, text []byte) (reads bool) {
	defer func() {
		if recover() != nil {
			reads = false
		}
	}()
	parser, err := factory(nil)
	if err != nil {
		return false
	}
	parser.Reset(bytes.NewReader(text))
	parsed, failed := 0, 0
	for parsed+failed < detectionRecordLimit {
		_, failure, err := parser.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return false
		}
		if failure != nil {
			failed++
			continue
		}
		parsed++
	}
	return parsed > failed
}
