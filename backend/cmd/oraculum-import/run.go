package main

import (
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/cmd/internal/sourceargs"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/output"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const usage = "usage: oraculum-import " + sourceargs.SigmaUsage + " " + sourceargs.Usage

func run(args []string, stdout, stderr io.Writer) int {
	// 診断ログの出力境界はここだけである。外部由来の文字列は、この境界で無害化してから
	// log と画面に出す。
	slog.SetDefault(output.NewSanitizingLogger(stderr))
	if len(args) == 0 {
		return reportError(stderr, usage)
	}
	sigmaSpec, args, err := sourceargs.ParseSigmaRules(args, os.ReadFile)
	if err != nil {
		return reportUsage(stderr, err.Error())
	}
	plans, rest, err := sourceargs.Parse(args, os.ReadFile)
	if err != nil {
		return reportUsage(stderr, err.Error())
	}
	if len(rest) > 0 {
		return reportUsage(stderr, "invalid source argument: "+rest[0])
	}
	if len(plans) == 0 {
		return reportError(stderr, usage)
	}
	config, err := importConfig()
	if err != nil {
		return reportError(stderr, "collecting the declared input formats:", err)
	}
	// 収集の directory の path は、起動した directory からの相対 path である。
	plans, skipped, err := pipeline.ExpandPlans(os.DirFS("."), plans, config.Parsers)
	if err != nil {
		return reportError(stderr, "expanding the collection:", err)
	}
	for _, file := range skipped {
		line := "skipped " + file.OriginPath + ": " + string(file.Reason)
		if file.DetectedKind != "" {
			line += " (" + file.DetectedKind + ")"
		}
		if _, err := output.Fprintln(stderr, line); err != nil {
			return 1
		}
	}
	sigmaRules, err := sourceargs.LoadSigmaRules(sigmaSpec)
	if err != nil {
		return reportError(stderr, err)
	}
	runner, err := pipeline.NewRunner(config)
	if err != nil {
		return reportError(stderr, "creating import runner:", err)
	}
	result, err := runner.Run(plans)
	if err != nil {
		return reportError(stderr, "running import:", err)
	}
	statuses := result.Statuses()
	entries, err := result.SourceEntries()
	if err != nil {
		return reportError(stderr, "listing imported sources:", err)
	}
	importSpec, err := sourceargs.ImportSpecOf(plans, entries)
	if err != nil {
		return reportError(stderr, "recording the import specification:", err)
	}
	if sigmaRules != nil {
		if _, err := sourceargs.EvaluateSigmaRules(stderr, result, sigmaRules); err != nil {
			return reportError(stderr, err)
		}
		importSpec.SigmaRules = sourceargs.ImportSpecSigmaRulesOf(sigmaRules.Info())
	}
	// importSpec をそのまま file に保存すると、`--import-spec` が同じ取り込みを再現する。
	response := struct {
		Statuses   []core.ImportStatus   `json:"statuses"`
		ImportSpec sourceargs.ImportSpec `json:"importSpec"`
	}{Statuses: statuses, ImportSpec: importSpec}
	if err := output.WriteJSON(stdout, response); err != nil {
		return reportError(stderr, "writing import statuses:", err)
	}
	// 画面と同じ表示名で書く。同じ file 名の収集元を行で区別する。
	plans = pipeline.DistinguishFileNames(plans)
	for i, status := range statuses {
		var summary strings.Builder
		for _, category := range []core.ImportCategory{core.ImportCategoryRead, core.ImportCategorySucceeded, core.ImportCategoryFailed} {
			if count, ok := status.Counts.Count(category); ok {
				summary.WriteString(" " + string(category) + "=" + strconv.FormatInt(count, 10))
			}
		}
		if _, err := output.Fprintf(stderr, "%s %s%s\n", plans[i].FileName, status.PublicationState, summary.String()); err != nil {
			return 1
		}
	}
	return 0
}

// reportUsage は理由と usage を別の行に書く。1 つの文字列に改行を入れると、出力境界の
// 無害化が改行を escape して 1 行になる。
func reportUsage(stderr io.Writer, reason string) int {
	if _, err := output.Fprintf(stderr, "%s\n%s\n", reason, usage); err != nil {
		return 1
	}
	return 1
}

func reportError(stderr io.Writer, args ...any) int {
	if _, err := output.Fprintln(stderr, args...); err != nil {
		return 1
	}
	return 1
}
