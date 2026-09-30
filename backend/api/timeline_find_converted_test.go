package api_test

import (
	"bytes"
	"encoding/hex"
	"io"
	"net/url"
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/output"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const convertedFormatKey core.FormatKey = "converted_find_test"

const (
	convertedValueName  = "Greeting"
	convertedValueText  = "hello-synthetic"
	convertedSourceBody = "synthetic-cell-bytes"
)

// convertedParser は、原文を byte 列の 16 進で書いた 1 レコードと、復号した値の欄を返す。
type convertedParser struct {
	t        *testing.T
	returned bool
}

func (p *convertedParser) Identity() pipeline.ParserIdentity {
	return pipeline.ParserIdentity{
		ParserID: "converted-find", SupportedFormatVersion: "1", FormatKey: convertedFormatKey,
		PositionKind: core.PositionKindByteRange, ItemSemantics: []core.SemanticKey{},
		ConnectionRequestKinds:    []core.ObservationKindSelector{},
		ConnectionMatchConditions: []pipeline.ConnectionMatchCondition{},
		TranscriptIdentityItems:   []string{},
		RecordedByOneTerminal:     true,
		RawTextConverted:          true,
	}
}

func (p *convertedParser) Reset(input io.Reader) {
	p.returned = false
	_, _ = io.Copy(io.Discard, input)
}

func (p *convertedParser) Next() (pipeline.ParsedRecord, *core.ImportFailure, error) {
	if p.returned {
		return pipeline.ParsedRecord{}, nil, io.EOF
	}
	p.returned = true
	length := int64(len(convertedSourceBody))
	timeText := "2031-04-05T06:07:08Z"
	observedAt, err := core.NewTimestamp(core.Timestamp{
		RawText: &timeText, Normalized: &timeText, NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Precision: core.PrecisionSecond, OffsetState: core.OffsetStateInValue, OffsetText: new("Z"),
		Clock: core.ClockTerminalLocal, Meaning: core.MeaningEvent, ValueState: core.ValueStatePresent,
	})
	if err != nil {
		p.t.Fatal(err)
	}
	value, err := core.NewRawValue(core.ValueStatePresent, convertedValueText)
	if err != nil {
		p.t.Fatal(err)
	}
	field, err := core.NewTextField("Value."+convertedValueName, "", value)
	if err != nil {
		p.t.Fatal(err)
	}
	return pipeline.ParsedRecord{
		RawText: hex.EncodeToString([]byte(convertedSourceBody)), ByteLength: &length,
		ObservedAt: &observedAt,
		Semantics: &pipeline.RecordSemantics{
			ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
			Fields:          []core.RecordField{field},
		},
	}, nil, nil
}

// 原文を 16 進で書いた収集元の行は、原文に無い値の文字列と欄の名前でも見つかる。大文字と
// 小文字の区別の指定に従う。
func TestTimelineFindsTheDecodedValuesOfAConvertedRawText(t *testing.T) {
	runner, err := pipeline.NewRunner(pipeline.Config{
		Open: func(string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader([]byte(convertedSourceBody))), nil
		},
		Parsers: map[core.FormatKey]pipeline.ParserFactory{
			convertedFormatKey: func(*string) (pipeline.SourceParser, error) {
				return &convertedParser{t: t}, nil
			},
		},
		Minter:   pipeline.DigestMinter{},
		Ordinals: pipeline.NewInMemoryOrdinals(),
		Sanitize: output.Sanitize,
		Revision: "api-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run([]pipeline.SourcePlan{{
		FormatKey: convertedFormatKey, FileName: "synthetic.bin", OriginPath: "synthetic.bin",
	}})
	if err != nil {
		t.Fatal(err)
	}
	handler := testHandler(result)
	if whole := decodeTimeline(t, handler, ""); len(whole.Entries) != 1 {
		t.Fatalf("entries=%d, want the one converted record", len(whole.Entries))
	}
	for _, check := range []struct {
		query string
		want  []int
	}{
		{"find=" + url.QueryEscape(convertedValueText), []int{0}},
		{"find=" + url.QueryEscape(convertedValueName), []int{0}},
		{"find=HELLO-SYNTHETIC", []int{0}},
		{"find=HELLO-SYNTHETIC&findCaseSensitive=true", []int{}},
		{"find=absent-synthetic", []int{}},
	} {
		got := decodeTimeline(t, handler, check.query).FindMatches
		if got == nil || !slices.Equal(*got, check.want) {
			t.Errorf("%s: findMatches=%v, want %v", check.query, got, check.want)
		}
	}
}
