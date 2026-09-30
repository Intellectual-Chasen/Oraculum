package sourceargs

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/output"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// Sigma のルールの集合を渡す flag。
const (
	// SigmaRulesFlag はルールの file を置いた directory を渡す。
	SigmaRulesFlag = "--sigma-rules"
	// SigmaRulesRevisionFlag はルールの集合の commit の文字列を渡す。渡さない起動は、directory を
	// 含む git の作業ツリーの HEAD を commit にし、HEAD も読めなければ commit を確かめられなかったと記録する。
	SigmaRulesRevisionFlag = "--sigma-rules-revision"
)

// SigmaUsage は Sigma のルールの集合の指定の文字列である。各 CLI の usage が組み込む。
const SigmaUsage = "[" + SigmaRulesFlag + " <dir> [" + SigmaRulesRevisionFlag + " <commit>]]"

// ParseSigmaRules は args から Sigma のルールの集合の指定を取り出し、残りの引数を順序を保って
// 返す。残りの引数は Parse へ渡す。ルールの集合を渡していない起動は nil を返す。
//
// `--import-spec` が記録した集合を持つときは、記録の directory と commit と内容の識別を指定にする。
// そのとき SigmaRulesFlag と一緒に渡す起動を退ける。記録を通知せずに別の集合に置き換えない。
func ParseSigmaRules(args []string, readFile ReadFile) (*pipeline.SigmaRuleSpec, []string, error) {
	var directory, revision *string
	var importSpecPath string
	rest := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg != SigmaRulesFlag && arg != SigmaRulesRevisionFlag {
			rest = append(rest, arg)
			if arg == importSpecFlag && index+1 < len(args) {
				importSpecPath = args[index+1]
			}
			continue
		}
		index++
		if index == len(args) || args[index] == "" {
			return nil, nil, fmt.Errorf("parsing arguments: %s requires a non-empty value", arg)
		}
		target := &directory
		if arg == SigmaRulesRevisionFlag {
			target = &revision
		}
		if *target != nil {
			return nil, nil, fmt.Errorf("parsing arguments: %s is given twice", arg)
		}
		value := args[index]
		*target = &value
	}
	if revision != nil && directory == nil {
		return nil, nil, fmt.Errorf("parsing arguments: %s requires %s", SigmaRulesRevisionFlag, SigmaRulesFlag)
	}
	recorded, err := recordedSigmaRules(importSpecPath, readFile)
	if err != nil {
		return nil, nil, err
	}
	if recorded != nil {
		if directory != nil {
			return nil, nil, fmt.Errorf("parsing arguments: %s records Sigma rules and takes no %s", importSpecFlag, SigmaRulesFlag)
		}
		spec := recorded.ruleSpec()
		return &spec, rest, nil
	}
	if directory == nil {
		return nil, rest, nil
	}
	spec := pipeline.SigmaRuleSpec{Directory: *directory}
	if revision != nil {
		spec.Revision = *revision
	}
	return &spec, rest, nil
}

// LoadSigmaRules は spec のルールの集合を読む。spec が nil の起動は nil を返す。
func LoadSigmaRules(spec *pipeline.SigmaRuleSpec) (*pipeline.SigmaRules, error) {
	if spec == nil {
		return nil, nil
	}
	rules, err := pipeline.LoadSigmaRules(*spec)
	if err != nil {
		return nil, err
	}
	return &rules, nil
}

// EvaluateSigmaRules は取り込み結果にルールを当て、件数と所要を運用者へ伝える。rules が nil の
// 起動は何も伝えず、ルールの集合を持たない結果を返す。
func EvaluateSigmaRules(
	stderr io.Writer, result pipeline.ImportResult, rules *pipeline.SigmaRules,
) (pipeline.SigmaEvaluation, error) {
	started := time.Now()
	evaluation := pipeline.EvaluateSigmaRules(result, rules)
	if rules == nil {
		return evaluation, nil
	}
	revision := "unverified"
	if info := rules.Info(); info.Revision != nil {
		revision = *info.Revision
	}
	var outside int64
	for _, group := range evaluation.UnevaluatedRecordGroups {
		outside += group.RecordCount
	}
	if _, err := output.Fprintf(stderr,
		"sigma rules at %s: %d evaluated, %d not evaluated, %d candidates in %d records "+
			"(%d records outside every logsource, %d records without semantics, "+
			"%d rule and record pairs skipped for unnamed fields) in %s\n",
		revision, evaluation.EvaluatedRuleCount, len(evaluation.UnevaluatedRules), len(evaluation.Matches),
		evaluation.EvaluatedRecordCount, outside, evaluation.RecordsWithoutSemantics, evaluation.SkippedPairCount,
		time.Since(started).Round(time.Millisecond)); err != nil {
		return pipeline.SigmaEvaluation{}, fmt.Errorf("writing the Sigma rule summary: %w", err)
	}
	return evaluation, nil
}

// recordedSigmaRules は取り込みの指定の記録が持つルールの集合を返す。記録を渡していない
// 起動と、集合を持たない記録には nil を返す。
func recordedSigmaRules(path string, readFile ReadFile) (*ImportSpecSigmaRules, error) {
	if path == "" {
		return nil, nil
	}
	if readFile == nil {
		return nil, errors.New("parsing arguments: " + importSpecFlag + " is unavailable without a file reader")
	}
	content, err := readFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading the import specification file %q: %w", path, err)
	}
	spec, err := readImportSpec(content)
	if err != nil {
		return nil, fmt.Errorf("reading the import specification file %q: %w", path, err)
	}
	return spec.SigmaRules, nil
}
