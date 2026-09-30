package pipeline

import (
	"fmt"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// FormatRegistration は adapter が宣言した入力形式 1 つと、その形式を読む走査器の
// 作り方の組である。
//
// **どの入力形式を読めるかを決めるのは adapter である。** 本 package は宣言を受け取り、
// 収集元の形式の識別子で探して byte 列を渡す。
type FormatRegistration struct {
	// Format は adapter の宣言である。
	Format core.InputFormat
	// New は宣言した形式の走査器を作る。
	New ParserFactory
}

// buildParser は宣言 1 つ分の走査器を、確定した欄の並びから作る。
type buildParser func(format core.InputFormat, formatSpec string) (SourceParser, error)

// registrationsOf は adapter の宣言を、指定の検査を通した走査器の作り方と組にする。
//
// 欄の並びの指定が宣言と食い違う取り込みは、収集元を開く前に error になる
// (core.InputFormat.FormatSpecFor)。
func registrationsOf(formats []core.InputFormat, build buildParser) []FormatRegistration {
	registrations := make([]FormatRegistration, 0, len(formats))
	for _, format := range formats {
		registrations = append(registrations, FormatRegistration{
			Format: format,
			New: func(requested *string) (SourceParser, error) {
				if err := format.Validate(); err != nil {
					return nil, fmt.Errorf("registering the input format %q: %w", format.Key, err)
				}
				spec, err := format.FormatSpecFor(requested)
				if err != nil {
					return nil, fmt.Errorf("reading the source as %q: %w", format.Key, err)
				}
				parser, err := build(format, spec)
				if err != nil {
					return nil, fmt.Errorf("reading the source as %q: %w", format.Key, err)
				}
				return parser, nil
			},
		})
	}
	return registrations
}

// NewFormatRegistry は adapter の宣言を集めて Config.Parsers の表を作る。
//
// 同じ識別子を 2 つの宣言が名乗る場合は error を返す。どちらの adapter が収集元を読むか
// が決まらない状態を、後から来た宣言で上書きしない。
func NewFormatRegistry(groups ...[]FormatRegistration) (map[core.FormatKey]ParserFactory, error) {
	registry := make(map[core.FormatKey]ParserFactory)
	for _, group := range groups {
		for _, registration := range group {
			format := registration.Format
			if err := format.Validate(); err != nil {
				return nil, fmt.Errorf("registering the input format %q: %w", format.Key, err)
			}
			if registration.New == nil {
				return nil, fmt.Errorf("registering the input format %q: the parser factory is absent", format.Key)
			}
			if _, declared := registry[format.Key]; declared {
				return nil, fmt.Errorf("registering the input format %q: the key is already declared", format.Key)
			}
			registry[format.Key] = registration.New
		}
	}
	if len(registry) == 0 {
		return nil, fmt.Errorf("registering input formats: no format is declared")
	}
	return registry, nil
}
