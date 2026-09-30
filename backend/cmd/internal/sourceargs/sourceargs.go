// Package sourceargs は CLI の起動引数のうち、取り込む収集元の指定を読む。
//
// oraculum-import と oraculum-server が同じ文字列を受け取る。
package sourceargs

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// Usage は収集元の指定の文字列である。各 CLI の usage が組み込む。
// originPath は起動した directory からの相対 path である。
const Usage = "([--case <caseId>] [--logformat <spec>] [--logformat-file <path>] " +
	"[--terminal-id <id>] [--terminal-name <name>] [--terminal-ip <ip>] [--time-offset <+hh:mm>] " +
	"<formatKey>:<originPath> ... | --import-spec <path>)"

// 収集元の指定に付ける flag。
const (
	logFormatFlag     = "--logformat"
	logFormatFileFlag = "--logformat-file"
	caseFlag          = "--case"
	importSpecFlag    = "--import-spec"
	// 端末の flag と時刻のずれの flag は、直後の収集元 1 件だけに付ける。
	terminalIdFlag   = "--terminal-id"
	terminalNameFlag = "--terminal-name"
	terminalIpFlag   = "--terminal-ip"
	// timeOffsetFlag は、UTC からのずれを持たない収集元の時刻を読むずれである。端末の flag と
	// 一緒にも、単独でも渡せる。
	timeOffsetFlag = "--time-offset"
)

// isTerminalFlag は、収集元を記録した端末を指定する flag であるかを返す。
func isTerminalFlag(arg string) bool {
	return arg == terminalIdFlag || arg == terminalNameFlag || arg == terminalIpFlag || arg == timeOffsetFlag
}

// ReadFile は欄の並びと取り込みの指定の記録を書いた file を読む。
type ReadFile func(path string) ([]byte, error)

// Parse は収集元の指定と欄の並びの指定を読み、残りの引数を順序を保って返す。
//
// `--logformat`、`--logformat-file`、`--case` は、**それより後ろに並ぶ収集元**に適用する。
// 収集元ごとに別の値を渡す起動は、flag を収集元の直前に置いて繰り返す。
//
// `--terminal-id`、`--terminal-name`、`--terminal-ip`、`--time-offset` は、**直後の収集元
// 1 件だけ**に適用する。1 つのログのファイルは 1 台の端末が書いたものであり、後ろに並ぶ別の
// 収集元へ同じ端末を付けない。4 つのうち分かるものだけを渡せる。後ろに収集元の無い flag を退ける。
//
// `--import-spec` は記録した取り込みの指定 (ImportSpec) を読む。記録だけで取り込みを
// 決めるため、収集元の引数と、収集元に値を付ける flag と一緒に渡す起動を退ける。
//
// 欄の並びを収集元の内容から推測しない。flag を置かない収集元は、入力形式そのものが
// 並びを定めることを求める。
// valueFlags は呼び出し側が読む flag のうち、値を 1 つ取るものである。flag と値の 2 つを
// そのまま rest へ送る。収集元の指定と同じ文字列の値 (`--addr 127.0.0.1:8080`) を収集元と
// 取り違えない。
func Parse(
	args []string, readFile ReadFile, valueFlags ...string,
) ([]pipeline.SourcePlan, []string, error) {
	var plans, recorded []pipeline.SourcePlan
	var rest []string
	var spec, caseId *string
	var terminal *pipeline.SourceTerminal
	sourceFlagSeen, importSpecSeen := false, false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case isTerminalFlag(arg):
			index++
			if index == len(args) {
				return nil, nil, fmt.Errorf("parsing arguments: %s requires a value", arg)
			}
			next, err := withTerminalItem(terminal, arg, args[index])
			if err != nil {
				return nil, nil, fmt.Errorf("parsing arguments: %w", err)
			}
			terminal, sourceFlagSeen = next, true
			continue
		case arg == logFormatFlag || arg == logFormatFileFlag || arg == caseFlag || arg == importSpecFlag:
			index++
			if index == len(args) {
				return nil, nil, fmt.Errorf("parsing arguments: %s requires a value", arg)
			}
			switch arg {
			case caseFlag:
				if err := core.ValidateCaseId(caseFlag, args[index]); err != nil {
					return nil, nil, fmt.Errorf("parsing arguments: %w", err)
				}
				value := args[index]
				caseId, sourceFlagSeen = &value, true
			case importSpecFlag:
				if importSpecSeen {
					return nil, nil, fmt.Errorf("parsing arguments: %s is given twice", arg)
				}
				read, err := importSpecPlans(args[index], readFile)
				if err != nil {
					return nil, nil, err
				}
				recorded, importSpecSeen = read, true
			default:
				value, err := formatSpec(arg, args[index], readFile)
				if err != nil {
					return nil, nil, err
				}
				spec, sourceFlagSeen = &value, true
			}
			continue
		case slices.Contains(valueFlags, arg):
			rest = append(rest, arg)
			index++
			if index == len(args) {
				return nil, nil, fmt.Errorf("parsing arguments: %s requires a value", arg)
			}
			rest = append(rest, args[index])
			continue
		}
		if !strings.Contains(arg, ":") || strings.HasPrefix(arg, "-") {
			rest = append(rest, arg)
			continue
		}
		plan, err := sourcePlan(arg, spec, caseId)
		if err != nil {
			return nil, nil, err
		}
		plan.Terminal, terminal = terminal, nil
		plans = append(plans, plan)
	}
	if importSpecSeen {
		if len(plans) > 0 || sourceFlagSeen {
			return nil, nil, fmt.Errorf("parsing arguments: %s takes no source argument, %s, %s, %s or %s",
				importSpecFlag, caseFlag, logFormatFlag, logFormatFileFlag, "--terminal-*")
		}
		return recorded, rest, nil
	}
	if err := requireNoPendingTerminal(terminal); err != nil {
		return nil, nil, err
	}
	return plans, rest, nil
}

// requireNoPendingTerminal は、後ろに収集元の無い端末の指定を退ける。
func requireNoPendingTerminal(terminal *pipeline.SourceTerminal) error {
	if terminal == nil {
		return nil
	}
	return fmt.Errorf("parsing arguments: %s, %s, %s and %s need a source argument after them",
		terminalIdFlag, terminalNameFlag, terminalIpFlag, timeOffsetFlag)
}

// withTerminalItem は、直後の収集元に付ける端末へ flag の値 1 つを足した値を返す。
//
// **同じ収集元に同じ flag を 2 回渡す起動を退ける。** 後の値で通知せずに置き換えない。
// 値の形は取り込みの記録を読むときと同じ検査で確かめる (validateTerminal)。
func withTerminalItem(
	current *pipeline.SourceTerminal, flag, value string,
) (*pipeline.SourceTerminal, error) {
	next := pipeline.SourceTerminal{}
	if current != nil {
		next = *current
	}
	if flag == timeOffsetFlag {
		if next.TimeOffset != nil {
			return nil, fmt.Errorf("%s is given twice for one source", flag)
		}
		offset := core.UtcOffset(value)
		if err := offset.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", flag, err)
		}
		next.TimeOffset = &offset
		return &next, nil
	}
	var slot *string
	switch flag {
	case terminalIdFlag:
		slot = &next.TerminalId
	case terminalNameFlag:
		slot = &next.TerminalHostname
	default:
		slot = &next.Ip
	}
	if *slot != "" {
		return nil, fmt.Errorf("%s is given twice for one source", flag)
	}
	if strings.TrimSpace(value) == "" {
		return nil, fmt.Errorf("%s requires a non-empty value", flag)
	}
	*slot = value
	if err := validateTerminal(next); err != nil {
		return nil, fmt.Errorf("%s: %w", flag, err)
	}
	return &next, nil
}

// importSpecPlans は取り込みの指定の記録を読み、収集元ごとの計画にする。
func importSpecPlans(path string, readFile ReadFile) ([]pipeline.SourcePlan, error) {
	if readFile == nil {
		return nil, errors.New("parsing arguments: " + importSpecFlag + " is unavailable without a file reader")
	}
	content, err := readFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading the import specification file %q: %w", path, err)
	}
	plans, err := decodeImportSpec(content)
	if err != nil {
		return nil, fmt.Errorf("reading the import specification file %q: %w", path, err)
	}
	return plans, nil
}

// formatSpec は flag が渡した欄の並びを読む。
//
// 並びの文字列は `%` と引用符を含み、shell の引用を通す間に壊れやすい。file から渡す経路を
// 置き、squid.conf から切り出した 1 行をそのまま読めるようにする。
func formatSpec(flag, value string, readFile ReadFile) (string, error) {
	if flag == logFormatFlag {
		if value == "" {
			return "", fmt.Errorf("parsing arguments: %s requires a non-empty value", flag)
		}
		return value, nil
	}
	if readFile == nil {
		return "", fmt.Errorf("parsing arguments: %s is unavailable without a file reader", flag)
	}
	content, err := readFile(value)
	if err != nil {
		return "", fmt.Errorf("reading the format specification file %q: %w", value, err)
	}
	// squid.conf から切り出した 1 行は行末の改行を持つ。
	spec := strings.TrimRight(string(content), "\r\n")
	if spec == "" {
		return "", fmt.Errorf("reading the format specification file %q: the file declares no item", value)
	}
	return spec, nil
}

// sourcePlan は `<formatKey>:<originPath>` を 1 件の取り込みの計画にする。
//
// 絶対 path を退ける。原資料の指定は起動引数だけが持ち、API の要求は path を持たない。
func sourcePlan(arg string, spec, caseId *string) (pipeline.SourcePlan, error) {
	format, path, ok := strings.Cut(arg, ":")
	if !ok || format == "" || path == "" || filepath.IsAbs(path) {
		return pipeline.SourcePlan{}, fmt.Errorf(
			"parsing source argument %q: expected <formatKey>:<relative path>", arg)
	}
	return pipeline.SourcePlan{
		FormatKey: core.FormatKey(format), OriginPath: path, FileName: filepath.Base(path),
		FormatSpec: clone(spec), CaseId: clone(caseId),
	}, nil
}
