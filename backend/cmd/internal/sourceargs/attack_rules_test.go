package sourceargs_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/cmd/internal/sourceargs"
)

func TestLoadAttackRulesUsesExplicitDirectoryAndReportsProvenance(t *testing.T) {
	directory := t.TempDir()
	rule := `id: synthetic.rule
title: Synthetic rule
description: Test runtime source.
references: [https://example.test/rules/synthetic]
attack: [{id: T1055, basis: inferred}]
variants:
  - id: default
    rationale: Synthetic test.
    pattern:
      nodes: {source: {kind: process}, target: {kind: process}}
      edges: {injection: {kind: process_injection, from: source, to: target}}
      evidence: {required: [injection]}
`
	if err := os.WriteFile(filepath.Join(directory, "synthetic.rule.yaml"), []byte(rule), 0o600); err != nil {
		t.Fatal(err)
	}
	set, err := sourceargs.LoadAttackRules(directory)
	if err != nil {
		t.Fatal(err)
	}
	if set.Info.FileCount != 1 || set.Info.Directory == "" || set.Info.ContentSha256 == "" ||
		set.Info.Revision == "" || set.Info.RevisionSource == "" {
		t.Fatalf("provenance = %+v", set.Info)
	}
	var output bytes.Buffer
	if err := sourceargs.ReportAttackRules(&output, set); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"attack rules: 1 files", set.Info.ContentSha256, set.Info.RevisionSource, set.Info.Directory} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("report %q does not contain %q", output.String(), want)
		}
	}
}

func TestDefaultAttackRuleDirectoryIsRelativeToExecutable(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	want, err := filepath.Abs(filepath.Join(filepath.Dir(executable), "../share/oraculum/attack-rules"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := sourceargs.DefaultAttackRuleDirectory()
	if err != nil || got != want {
		t.Fatalf("DefaultAttackRuleDirectory() = %q, %v; want %q", got, err, want)
	}
}

func TestInstalledServerLoadsDefaultRulesWithoutFlag(t *testing.T) {
	if os.Getenv("ORACULUM_ATTACK_RULES_INSTALL_INTEGRATION") != "1" {
		t.Skip("set ORACULUM_ATTACK_RULES_INSTALL_INTEGRATION=1 to run the staged install smoke test")
	}
	for _, tool := range []string{"make", "go"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is required for the staged install smoke test: %v", tool, err)
		}
	}
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() did not return the test source path")
	}
	backendRoot := findBackendModuleRoot(t, filepath.Dir(sourceFile))
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	install := exec.CommandContext(ctx, "make", "-C", backendRoot, "install-server", "PREFIX=/usr/local", "DESTDIR="+root)
	if output, err := install.CombinedOutput(); err != nil {
		t.Fatalf("make install-server failed: %v\n%s", err, output)
	}

	installedRules := filepath.Join(root, "usr", "local", "share", "oraculum", "attack-rules")
	server := filepath.Join(root, "usr", "local", "bin", "oraculum-server")
	command := exec.CommandContext(ctx, server, "squid_combined:missing.csv")
	command.Dir = t.TempDir()
	stdout, err := command.Output()
	if err == nil {
		t.Fatal("server unexpectedly succeeded while its source file was absent")
	}
	if !strings.Contains(string(stdout), "from "+installedRules) {
		t.Fatalf("server did not load its installed default rules; stdout=%q, err=%v", stdout, err)
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		t.Fatalf("server returned a non-process error: %v", err)
	}
	if strings.Contains(string(exitError.Stderr), "read ATT&CK rule directory") {
		t.Fatalf("server failed to read the installed rule directory: stderr=%q", exitError.Stderr)
	}
}

func findBackendModuleRoot(t *testing.T, directory string) string {
	t.Helper()
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("could not find backend go.mod for staged install smoke test")
		}
		directory = parent
	}
}
