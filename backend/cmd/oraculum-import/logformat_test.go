// in-package test: 非公開の run に引数と出力先を注入し、起動引数から取り込みまでの
// 結線を確かめる。
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// combinedSpec は testdata/full.log の欄の並びである。
const combinedSpec = `%>a %[ui %[un [%tl] "%rm %ru HTTP/%rv" %>Hs %<st ` +
	`"%{Referer}>h" "%{User-Agent}>h" %Ss:%Sh`

// importWithSpec は起動引数から取り込みを実行し、収集元 1 件の状態を返す。
func importWithSpec(t *testing.T, args []string) (core.ImportStatus, int, string) {
	t.Helper()
	restoreDefaultLogger(t)
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	if code != 0 {
		return core.ImportStatus{}, code, stderr.String()
	}
	var response struct {
		Statuses []core.ImportStatus `json:"statuses"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("decoding the statuses: %v", err)
	}
	if len(response.Statuses) != 1 {
		t.Fatalf("the run published %d sources, want the requested one", len(response.Statuses))
	}
	return response.Statuses[0], code, stderr.String()
}

// 欄の並びを渡した起動が、並びが決まっている入力形式と同じ件数を取り込む。
func TestRunReadsTheRequestedLogFormat(t *testing.T) {
	status, code, stderr := importWithSpec(t, []string{
		"--logformat", combinedSpec, "squid_logformat:testdata/full.log",
	})
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if status.PublicationState != core.PublicationStatePublishedFull {
		t.Errorf("the publication state = %q, want published_full", status.PublicationState)
	}
	read, readOk := status.Counts.Count(core.ImportCategoryRead)
	succeeded, succeededOk := status.Counts.Count(core.ImportCategorySucceeded)
	failed, failedOk := status.Counts.Count(core.ImportCategoryFailed)
	if !readOk || !succeededOk || !failedOk {
		t.Fatalf("the status carries the counts %+v", status.Counts)
	}
	if succeeded != read || failed != 0 {
		t.Errorf("read=%d succeeded=%d failed=%d, want every record read", read, succeeded, failed)
	}
}

// file から渡した並びが、引数で渡した並びと同じ結果を出す。
func TestRunReadsTheLogFormatFromAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "squid.conf.logformat")
	if err := os.WriteFile(path, []byte(combinedSpec+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fromFlag, _, _ := importWithSpec(t, []string{
		"--logformat", combinedSpec, "squid_logformat:testdata/full.log",
	})
	fromFile, code, stderr := importWithSpec(t, []string{
		"--logformat-file", path, "squid_logformat:testdata/full.log",
	})
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if fromFile.AnalysisRunRef != fromFlag.AnalysisRunRef {
		t.Errorf("the file gave the analysis run %q, the flag gave %q",
			fromFile.AnalysisRunRef, fromFlag.AnalysisRunRef)
	}
}

// 読めない指定の起動は、収集元を 1 byte も読まずに終わる。
func TestRunRejectsAnUnreadableLogFormat(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		args   []string
		reason string
	}{
		{"an unsupported specifier",
			[]string{"--logformat", `%h [%tl]`, "squid_logformat:testdata/full.log"},
			"outside the supported set"},
		{"no specification",
			[]string{"squid_logformat:testdata/full.log"}, "formatSpec"},
		{"a specification on a fixed layout",
			[]string{"--logformat", combinedSpec, "squid_combined:testdata/full.log"},
			"formatSpec"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			restoreDefaultLogger(t)
			var stdout, stderr bytes.Buffer
			if code := run(testCase.args, &stdout, &stderr); code == 0 {
				t.Fatalf("the run accepted an unreadable specification: %q", stdout.String())
			}
			if !strings.Contains(stderr.String(), testCase.reason) {
				t.Errorf("stderr=%q, want it to carry %q", stderr.String(), testCase.reason)
			}
			if stdout.Len() != 0 {
				t.Errorf("the run published %q", stdout.String())
			}
		})
	}
}
