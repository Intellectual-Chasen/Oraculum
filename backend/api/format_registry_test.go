package api_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// 入力形式の識別子の文字列。宣言の定義元は adapter が持ち、本 package の test は adapter を
// import しないため (backend/.golangci.yml の api-no-adapter-import)、利用者が起動引数に
// 書く文字列をそのまま置く。宣言と文字列が一致することは adapter の test が確かめる。
const (
	markIIFormatKey core.FormatKey = "infotrace_mark_ii"
	squidFormatKey  core.FormatKey = "squid_combined"
)

// testFormatRegistry は adapter の宣言から取り込みの表を作る。
func testFormatRegistry(t *testing.T) map[core.FormatKey]pipeline.ParserFactory {
	t.Helper()
	registry, err := pipeline.NewFormatRegistry(pipeline.MarkIIFormats(), pipeline.SquidFormats())
	if err != nil {
		t.Fatal(err)
	}
	return registry
}
