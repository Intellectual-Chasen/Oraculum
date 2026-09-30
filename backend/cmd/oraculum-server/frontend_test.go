// in-package test: 非公開の parseArgs と run に引数と出力先を注入する。
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArgsAcceptsTheFrontendDirectory(t *testing.T) {
	opts, err := parseArgs([]string{frontendDirFlag, "web/dist", sourceRootFlag, "data"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.frontendDir != "web/dist" {
		t.Errorf("frontendDir = %q, want web/dist", opts.frontendDir)
	}
	for _, args := range [][]string{
		{sourceRootFlag, "data", frontendDirFlag},
		{sourceRootFlag, "data", frontendDirFlag, "a", frontendDirFlag, "b"},
	} {
		if _, err := parseArgs(args); err == nil {
			t.Errorf("args=%q accepted", args)
		}
	}
}

// 画面の build を持たない directory を渡した起動は、取り込みの前に止まる。
func TestRunRefusesAFrontendDirectoryWithoutTheIndex(t *testing.T) {
	restoreDefaultLogger(t)
	var stdout, stderr bytes.Buffer
	args := append([]string{frontendDirFlag, t.TempDir(),
		"squid_combined:" + filepath.Join(runFixtureDir, "absent.log")}, testAttackRuleArgs()...)
	code := run(args, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "frontend build") || strings.Contains(stderr.String(), "running import") {
		t.Fatalf("stderr=%q, want the frontend failure before the import", stderr.String())
	}
}

// 存在しない directory を渡した起動は、取り込みの前に止まる。
func TestRunRefusesAMissingFrontendDirectory(t *testing.T) {
	restoreDefaultLogger(t)
	var stdout, stderr bytes.Buffer
	args := append([]string{frontendDirFlag, filepath.Join(t.TempDir(), "absent"),
		"squid_combined:" + filepath.Join(runFixtureDir, "absent.log")}, testAttackRuleArgs()...)
	code := run(args, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "opening the frontend build") ||
		strings.Contains(stderr.String(), "running import") {
		t.Fatalf("stderr=%q, want the frontend failure before the import", stderr.String())
	}
}

// 画面の build を開いた後に取り込みが失敗した起動は、取り込みの失敗だけを返して止まる。
func TestRunReportsOnlyTheImportFailureAfterOpeningTheFrontend(t *testing.T) {
	restoreDefaultLogger(t)
	build := t.TempDir()
	if err := os.WriteFile(filepath.Join(build, "index.html"), []byte("<!doctype html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	args := append([]string{frontendDirFlag, build,
		"squid_combined:" + filepath.Join(runFixtureDir, "absent.log")}, testAttackRuleArgs()...)
	code := run(args, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "running import") || strings.Contains(stderr.String(), "close") {
		t.Fatalf("stderr=%q, want only the import failure", stderr.String())
	}
}
