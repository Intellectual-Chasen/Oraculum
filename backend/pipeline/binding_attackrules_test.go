package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const syntheticRuleYAML = `id: synthetic.one
title: One
description: One synthetic rule.
references: [https://example.test/rules/one]
attack: [{id: T1055, basis: official}]
variants:
  - id: default
    pattern:
      nodes: {source: {kind: process}, target: {kind: process}}
      edges: {edge: {kind: process_injection, from: source, to: target}}
      evidence: {required: [edge]}
`

func TestLoadAttackRuleSetIsAllOrNothingAndReportsProvenance(t *testing.T) {
	directory := t.TempDir()
	writeAttackRule(t, directory, "synthetic.one.yaml", syntheticRuleYAML)
	set, err := LoadAttackRuleSet(directory)
	if err != nil {
		t.Fatalf("LoadAttackRuleSet() error = %v", err)
	}
	if set.Info.Directory != directory || set.Info.FileCount != len(set.Rules) ||
		set.Info.ContentSha256 == "" || set.Info.Revision == "" || set.Info.RevisionSource == "" {
		t.Fatalf("LoadAttackRuleSet() provenance = %+v", set.Info)
	}
	writeAttackRule(t, directory, "synthetic.bad.yaml", "id: synthetic.bad\nunsupported: true\n")
	partial, err := LoadAttackRuleSet(directory)
	if err == nil || len(partial.Rules) != 0 {
		t.Fatalf("LoadAttackRuleSet() = %+v, %v; want empty set and error", partial, err)
	}
}

func TestLoadAttackRuleSetRejectsBrokenRuleReferences(t *testing.T) {
	directory := t.TempDir()
	writeAttackRule(t, directory, "synthetic.one.yaml", strings.Replace(
		syntheticRuleYAML, "variants:", "distinguishes_from: [synthetic.missing]\nvariants:", 1))
	set, err := LoadAttackRuleSet(directory)
	if err == nil || len(set.Rules) != 0 || !strings.Contains(err.Error(), "unknown rule") {
		t.Fatalf("LoadAttackRuleSet() = %+v, %v; want broken reference error and empty set", set, err)
	}
}

func TestLoadAttackRuleSetRejectsDuplicateIDsAcrossExtensions(t *testing.T) {
	directory := t.TempDir()
	writeAttackRule(t, directory, "synthetic.one.yaml", syntheticRuleYAML)
	writeAttackRule(t, directory, "synthetic.one.yml", syntheticRuleYAML)
	set, err := LoadAttackRuleSet(directory)
	if err == nil || len(set.Rules) != 0 || !strings.Contains(err.Error(), "duplicate ATT&CK rule id") {
		t.Fatalf("LoadAttackRuleSet() = %+v, %v; want duplicate rule ID and no partial set", set, err)
	}
}

func TestRevisionOfReadsGitDirectoryAndWorktreeFile(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		worktree bool
	}{
		{name: "repository directory"},
		{name: "linked worktree file", worktree: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			gitDir := filepath.Join(root, ".git")
			if testCase.worktree {
				gitDir = filepath.Join(root, "gitdir")
				if err := os.MkdirAll(gitDir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: gitdir\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(gitDir, "refs", "heads", "main"), []byte("0123456789abcdef\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			directory := filepath.Join(root, "rules")
			if err := os.Mkdir(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			if revision, source := revisionOf(directory); revision != "0123456789abcdef" || source != "git_head" {
				t.Fatalf("revisionOf() = %q, %q", revision, source)
			}
		})
	}
}

func writeAttackRule(t *testing.T, directory, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
		t.Fatalf("write rule %q: %v", name, err)
	}
}
