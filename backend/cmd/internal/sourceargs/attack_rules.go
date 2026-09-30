package sourceargs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Intellectual-Chasen/Oraculum/backend/attackrules"
	"github.com/Intellectual-Chasen/Oraculum/backend/output"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// AttackRulesFlag は外部 ATT&CK rule directory を指定する flag である。
const AttackRulesFlag = "--attack-rules"

// AttackRulesUsage は ATT&CK rule directory の指定形式である。
const AttackRulesUsage = "[" + AttackRulesFlag + " <dir>]"

const attackRulesInstallRelativePath = "../share/oraculum/attack-rules"

// DefaultAttackRuleDirectory は executable の配置から install path を返す。
func DefaultAttackRuleDirectory() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable for the default ATT&CK rule directory: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	directory, err := filepath.Abs(filepath.Join(filepath.Dir(executable), attackRulesInstallRelativePath))
	if err != nil {
		return "", fmt.Errorf("resolve installed ATT&CK rule directory: %w", err)
	}
	return directory, nil
}

// LoadAttackRules は指定または install default の rule directory を全件検証して読む。
func LoadAttackRules(directory string) (attackrules.Set, error) {
	if directory == "" {
		var err error
		directory, err = DefaultAttackRuleDirectory()
		if err != nil {
			return attackrules.Set{}, err
		}
	}
	set, err := pipeline.LoadAttackRuleSet(directory)
	if err != nil {
		return attackrules.Set{}, fmt.Errorf("load ATT&CK rules: %w", err)
	}
	return set, nil
}

// ReportAttackRules は起動時に利用する rule set provenance を出力する。
func ReportAttackRules(stdout io.Writer, set attackrules.Set) error {
	if _, err := output.Fprintf(stdout,
		"attack rules: %d files, content %s, revision %s (%s), from %s\n",
		set.Info.FileCount, set.Info.ContentSha256, set.Info.Revision, set.Info.RevisionSource, set.Info.Directory); err != nil {
		return fmt.Errorf("write ATT&CK rule provenance: %w", err)
	}
	return nil
}
