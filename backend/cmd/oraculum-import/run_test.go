// in-package test: 非公開の CLI run に引数と出力先を注入する。
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"testing"
	"unicode"

	"github.com/Intellectual-Chasen/Oraculum/backend/cmd/internal/sourceargs"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// restoreDefaultLogger は run が書き換える process 全体の既定 logger を test の終了後に戻す。
// 戻さないと、終了した test の bytes.Buffer に束縛された logger が既定のまま残る。
func restoreDefaultLogger(t *testing.T) {
	t.Helper()
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
}

func TestRunPublicationStates(t *testing.T) {
	restoreDefaultLogger(t)
	data, err := os.ReadFile("testdata/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		File, Format, Summary   string
		State                   core.PublicationState
		Read, Succeeded, Failed int64
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 3 {
		t.Fatal("CLI cases missing")
	}
	for _, tc := range cases {
		t.Run(tc.File, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run([]string{tc.Format + ":testdata/" + tc.File}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("exit=%d stderr=%q", code, stderr.String())
			}
			var response struct {
				Statuses   []core.ImportStatus   `json:"statuses"`
				ImportSpec sourceargs.ImportSpec `json:"importSpec"`
			}
			decoder := json.NewDecoder(&stdout)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&response); err != nil {
				t.Fatal(err)
			}
			if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
				t.Fatalf("trailing JSON: %v", err)
			}
			content, err := os.ReadFile("testdata/" + tc.File)
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(content)
			wantSpec := []sourceargs.ImportSpecSource{{
				FormatKey: core.FormatKey(tc.Format), OriginPath: "testdata/" + tc.File,
				ContentSha256: hex.EncodeToString(digest[:]),
			}}
			if !reflect.DeepEqual(response.ImportSpec.Sources, wantSpec) {
				t.Fatalf("importSpec=%+v want=%+v", response.ImportSpec.Sources, wantSpec)
			}
			if len(response.Statuses) != 1 {
				t.Fatalf("statuses=%+v", response.Statuses)
			}
			status := response.Statuses[0]
			if err := status.Validate(); err != nil {
				t.Fatal(err)
			}
			if status.PublicationState != tc.State || status.FailureCount != tc.Failed {
				t.Fatalf("status=%+v", status)
			}
			for category, want := range map[core.ImportCategory]int64{core.ImportCategoryRead: tc.Read, core.ImportCategorySucceeded: tc.Succeeded, core.ImportCategoryFailed: tc.Failed} {
				if got, ok := status.Counts.Count(category); !ok || got != want {
					t.Fatalf("%s=%d present=%t", category, got, ok)
				}
			}
			if stderr.String() != tc.Summary {
				t.Fatalf("stderr=%q want=%q", stderr.String(), tc.Summary)
			}
		})
	}
}

func TestRunSanitizesSourceNames(t *testing.T) {
	restoreDefaultLogger(t)
	data, err := os.ReadFile("testdata/full.log")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	name := "source\n\x1b\t\u0085.log"
	if err := os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"squid_combined:" + name}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	want := "source\\x0a\\x1b\\x09\\xc2\\x85.log published_full read=1 succeeded=1 failed=0\n"
	if stderr.String() != want {
		t.Fatalf("stderr=%q want=%q", stderr.String(), want)
	}
	line := strings.TrimSuffix(stderr.String(), "\n")
	if strings.ContainsFunc(line, unicode.IsControl) {
		t.Fatalf("control in summary=%q", line)
	}
	var response struct {
		Statuses []core.ImportStatus `json:"statuses"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Statuses) != 1 || response.Statuses[0].Validate() != nil {
		t.Fatalf("response=%+v", response)
	}
}

func TestRunInvalidArguments(t *testing.T) {
	restoreDefaultLogger(t)
	for _, args := range [][]string{nil, {"missing-colon"}, {":source"}, {"squid_combined:"}, {"squid_combined:/absolute"}, {"bad\nargument"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code == 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), usage) {
			t.Fatalf("args=%q stdout=%q stderr=%q", args, stdout.String(), stderr.String())
		}
		wantLines := 1
		if len(args) != 0 {
			wantLines = 2
		}
		if strings.Count(stderr.String(), "\n") != wantLines || strings.Contains(stderr.String(), "running import") {
			t.Fatalf("diagnostic split: %q", stderr.String())
		}
	}
}

// TestRunInstallsTheSanitizingLogger は、run の後の既定の logger が無害化を通し、
// 注入した stderr へ書くことを確かめる。
func TestRunInstallsTheSanitizingLogger(t *testing.T) {
	restoreDefaultLogger(t)
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code == 0 {
		t.Fatalf("exit=%d", code)
	}
	stderr.Reset()
	slog.Error("reading the source failed", "error", errors.New("markii.log\nERROR forged"))
	line := stderr.String()
	if strings.Count(line, "\n") != 1 || !strings.Contains(line, `error="markii.log\\x0aERROR forged"`) {
		t.Fatalf("log line = %q", line)
	}
}

func TestRunOpenFailureAndUnsupportedFormat(t *testing.T) {
	restoreDefaultLogger(t)
	for _, arg := range []string{"squid_combined:testdata/nonexistent.log", "apache_access:testdata/full.log"} {
		var stdout, stderr bytes.Buffer
		if code := run([]string{arg}, &stdout, &stderr); code == 0 || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("arg=%s stdout=%q stderr=%q", arg, stdout.String(), stderr.String())
		}
	}
}

func TestRunErrorContext(t *testing.T) {
	restoreDefaultLogger(t)
	t.Chdir(t.TempDir())
	for _, test := range []struct {
		argument, want string
		runningCount   int
	}{
		{"", usage + "\n", 0},
		{"badformspec", "invalid source argument: badformspec\n" + usage + "\n", 0},
		{"nosuchformat:x.log", "running import: no parser for \"nosuchformat\"\n", 1},
		{"squid_combined:missing.log", "running import: importing source \"missing.log\": open missing.log: no such file or directory\n", 1},
	} {
		t.Run(test.argument, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			var args []string
			if test.argument != "" {
				args = []string{test.argument}
			}
			if code := run(args, &stdout, &stderr); code != 1 || stdout.Len() != 0 {
				t.Fatalf("exit=%d stdout=%q", code, stdout.String())
			}
			message := stderr.String()
			if message != test.want || strings.Count(message, "running import") != test.runningCount || strings.Contains(message, "opening") {
				t.Errorf("message=%q want=%q", message, test.want)
			}
		})
	}
}

type rejectedOutput struct {
	calls    int
	received []byte
}

func (w *rejectedOutput) Write(data []byte) (int, error) {
	w.calls++
	w.received = append(w.received, data...)
	return 0, errors.New("output unavailable")
}

func TestRunOutputFailure(t *testing.T) {
	restoreDefaultLogger(t)
	for _, destination := range []string{"stdout", "stderr", "usage"} {
		t.Run(destination, func(t *testing.T) {
			failed := &rejectedOutput{}
			var out, diagnostic bytes.Buffer
			var stdout, stderr io.Writer = &out, &diagnostic
			args := []string{"squid_combined:testdata/full.log"}
			if destination == "stdout" {
				stdout = failed
			} else {
				stderr = failed
			}
			if destination == "usage" {
				args = nil
			}
			if code := run(args, stdout, stderr); code == 0 || failed.calls != 1 {
				t.Fatalf("exit=%d write calls=%d", code, failed.calls)
			}
			if len(failed.received) == 0 {
				t.Fatal("writer received no output")
			}
			if destination == "usage" && string(failed.received) != usage+"\n" {
				t.Fatalf("usage=%q", failed.received)
			}
			if destination == "stderr" && string(failed.received) != "full.log published_full read=1 succeeded=1 failed=0\n" {
				t.Fatalf("summary=%q", failed.received)
			}
		})
	}
}

func TestRunMultipleSources(t *testing.T) {
	restoreDefaultLogger(t)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"infotrace_mark_ii:testdata/withheld.log", "squid_combined:testdata/full.log"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	var response struct {
		Statuses []core.ImportStatus `json:"statuses"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Statuses) != 2 || response.Statuses[0].PublicationState != core.PublicationStateWithheld || response.Statuses[1].PublicationState != core.PublicationStatePublishedFull || strings.Count(stderr.String(), "\n") != 2 {
		t.Fatalf("response=%+v stderr=%q", response, stderr.String())
	}
}

func TestRunJSONEscapesC1AndPreservesDiagnosis(t *testing.T) {
	restoreDefaultLogger(t)
	data, err := os.ReadFile("testdata/full.log")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	rawTime := "99/Apr/2024:06:07:08 +0000"
	fileName := "c1\u0085\u009b.log"
	input := strings.Replace(string(data), "06/Apr/2024:07:08:09 +0000", rawTime, 1)
	if err := os.WriteFile(fileName, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"squid_combined:" + fileName}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	if strings.ContainsFunc(strings.TrimSuffix(stdout.String(), "\n"), unicode.IsControl) {
		t.Errorf("JSON contains raw controls: %q", stdout.String())
	}
	var response struct {
		Statuses []core.ImportStatus `json:"statuses"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Statuses) != 1 || len(response.Statuses[0].Failures) != 1 {
		t.Fatalf("diagnosis=%+v", response)
	}
	if err := response.Statuses[0].Validate(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(response.Statuses[0].Failures[0].ObservedResult, rawTime) {
		t.Fatalf("original time lost: %q", response.Statuses[0].Failures[0].ObservedResult)
	}
	if ref := response.Statuses[0].Failures[0].RecordRef; ref == nil || ref.SourceFileName != fileName {
		t.Fatalf("original filename lost: %+v", ref)
	}
}
