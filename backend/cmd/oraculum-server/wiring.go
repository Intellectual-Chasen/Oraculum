package main

import (
	"io"
	"os"

	"github.com/Intellectual-Chasen/Oraculum/backend/output"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// importConfig は adapter が読めると宣言した入力形式を集めて取り込みの依存を組む。
//
// **読める形式の一覧を本 file が持たない。** 形式を 1 つ足す作業は、adapter の宣言と
// 下の呼び出しへの 1 つの追加で完結する。
func importConfig() (pipeline.Config, error) {
	parsers, err := pipeline.NewFormatRegistry(pipeline.MarkIIFormats(), pipeline.SquidFormats(),
		pipeline.ApacheFormats(), pipeline.RoyalTSFormats(), pipeline.AuditdFormats(),
		pipeline.WindowsEventFormats(), pipeline.PrefetchFormats(), pipeline.RegistryFormats())
	if err != nil {
		return pipeline.Config{}, err
	}
	return pipeline.Config{
		Open: func(originPath string) (io.ReadCloser, error) {
			return os.Open(originPath) // #nosec G304 -- 運用者が起動引数で指定した原資料を開く。
		},
		Parsers: parsers,
		Minter:  pipeline.DigestMinter{}, Ordinals: pipeline.NewInMemoryOrdinals(),
		Sanitize: output.Sanitize, Revision: revision(),
	}, nil
}
