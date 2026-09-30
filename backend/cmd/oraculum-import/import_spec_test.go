// in-package test: 非公開の run に引数と出力先を注入し、取り込みの指定の記録から
// 取り込みを再現する経路を確かめる。
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/cmd/internal/sourceargs"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

type importResponse struct {
	Statuses   []core.ImportStatus   `json:"statuses"`
	ImportSpec sourceargs.ImportSpec `json:"importSpec"`
}

// importRecording は起動引数から取り込みを実行し、応答を返す。
func importRecording(t *testing.T, args []string) importResponse {
	t.Helper()
	restoreDefaultLogger(t)
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("args=%q exit=%d stderr=%q", args, code, stderr.String())
	}
	var response importResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}
	return response
}

// writeImportSpec は取り込みの指定の記録を file に書き、path を返す。
func writeImportSpec(t *testing.T, spec sourceargs.ImportSpec) string {
	t.Helper()
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "import-spec.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// assertReproduced は、記録から読み戻した取り込みが、記録した取り込みと同じ指定と
// 同じ件数を持つことを確かめる。sourceId は取り込み通番を材料にするので比べない。
func assertReproduced(t *testing.T, recorded, replayed importResponse) {
	t.Helper()
	if !reflect.DeepEqual(replayed.ImportSpec, recorded.ImportSpec) {
		t.Errorf("the replayed specification = %+v, want %+v", replayed.ImportSpec, recorded.ImportSpec)
	}
	if len(replayed.Statuses) != len(recorded.Statuses) {
		t.Fatalf("the replay published %d sources, the recording %d",
			len(replayed.Statuses), len(recorded.Statuses))
	}
	for i := range recorded.Statuses {
		want, got := recorded.Statuses[i], replayed.Statuses[i]
		if got.PublicationState != want.PublicationState || !reflect.DeepEqual(got.Counts, want.Counts) ||
			!reflect.DeepEqual(got.DiagnosisCounts, want.DiagnosisCounts) {
			t.Errorf("source %d: replayed %s %+v %+v, recorded %s %+v %+v", i,
				got.PublicationState, got.Counts, got.DiagnosisCounts,
				want.PublicationState, want.Counts, want.DiagnosisCounts)
		}
	}
}

// 取り込みが出した指定の記録を読み戻すと、同じ入力形式と欄の並びと案件で同じ収集元を読む。
func TestRunReproducesTheRecordedImport(t *testing.T) {
	recorded := importRecording(t, []string{
		"--case", "challenge", "infotrace_mark_ii:testdata/withheld.log",
		"--logformat", combinedSpec, "squid_logformat:testdata/full.log",
	})
	challenge, spec := "challenge", combinedSpec
	sources := recorded.ImportSpec.Sources
	if len(sources) != 2 || sources[0].FormatSpec != nil || !reflect.DeepEqual(sources[1].FormatSpec, &spec) ||
		!reflect.DeepEqual(sources[0].CaseId, &challenge) || !reflect.DeepEqual(sources[1].CaseId, &challenge) ||
		sources[0].FormatKey != "infotrace_mark_ii" || sources[1].FormatKey != "squid_logformat" {
		t.Fatalf("the recorded specification = %+v", sources)
	}
	replayed := importRecording(t, []string{"--import-spec", writeImportSpec(t, recorded.ImportSpec)})
	assertReproduced(t, recorded, replayed)
}

// 収集元に指定した端末は取り込みの記録に残り、読み戻した取り込みも同じ端末を持つ。
func TestRunReproducesTheRecordedTerminal(t *testing.T) {
	recorded := importRecording(t, []string{
		"--terminal-name", "proxy.example.test", "--terminal-ip", "192.0.2.8",
		"squid_combined:testdata/full.log",
	})
	want := &sourceargs.ImportSpecTerminal{Hostname: "proxy.example.test", Ip: "192.0.2.8"}
	if sources := recorded.ImportSpec.Sources; len(sources) != 1 || !reflect.DeepEqual(sources[0].Terminal, want) {
		t.Fatalf("the recorded specification = %+v, want the terminal %+v", sources, want)
	}
	replayed := importRecording(t, []string{"--import-spec", writeImportSpec(t, recorded.ImportSpec)})
	assertReproduced(t, recorded, replayed)
}

// 記録と別の byte 列を持つ収集元の取り込みは、何も公開せずに終わる。
func TestRunRejectsASourceThatDiffersFromTheRecord(t *testing.T) {
	recorded := importRecording(t, []string{"squid_combined:testdata/full.log"})
	tampered := recorded.ImportSpec
	tampered.Sources[0].ContentSha256 = strings.Repeat("0", 64)
	restoreDefaultLogger(t)
	var stdout, stderr bytes.Buffer
	code := run([]string{"--import-spec", writeImportSpec(t, tampered)}, &stdout, &stderr)
	if code == 0 || stdout.Len() != 0 {
		t.Fatalf("exit=%d stdout=%q", code, stdout.String())
	}
	if !strings.Contains(stderr.String(), "content sha256 mismatch") {
		t.Errorf("stderr=%q, want the mismatch", stderr.String())
	}
}

// 宣言と食い違う欄の並びを持つ記録は、収集元を開く前に失敗する。開けない path の収集元でも、
// 失敗の理由は欄の並びである。
func TestRunRejectsARecordedSpecificationBeforeOpeningTheSource(t *testing.T) {
	spec := combinedSpec
	for _, testCase := range []struct {
		name   string
		source sourceargs.ImportSpecSource
	}{
		{"a specification on a fixed layout", sourceargs.ImportSpecSource{
			FormatKey: "squid_combined", OriginPath: "testdata/missing.log", FormatSpec: &spec,
		}},
		{"no specification on a requested layout", sourceargs.ImportSpecSource{
			FormatKey: "squid_logformat", OriginPath: "testdata/missing.log",
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			source := testCase.source
			source.ContentSha256 = strings.Repeat("a", 64)
			path := writeImportSpec(t, sourceargs.ImportSpec{Sources: []sourceargs.ImportSpecSource{source}})
			restoreDefaultLogger(t)
			var stdout, stderr bytes.Buffer
			if code := run([]string{"--import-spec", path}, &stdout, &stderr); code == 0 || stdout.Len() != 0 {
				t.Fatalf("exit=%d stdout=%q", code, stdout.String())
			}
			if !strings.Contains(stderr.String(), "formatSpec") || strings.Contains(stderr.String(), "missing.log") {
				t.Errorf("stderr=%q, want the specification failure before opening the source", stderr.String())
			}
		})
	}
}

// 一部の収集元にだけ案件を付けた起動は、収集元を読まずに失敗する。
func TestRunRejectsAPartlyCasedImport(t *testing.T) {
	restoreDefaultLogger(t)
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"squid_combined:testdata/missing.log", "--case", "baseline", "squid_combined:testdata/full.log",
	}, &stdout, &stderr)
	if code == 0 || stdout.Len() != 0 {
		t.Fatalf("exit=%d stdout=%q", code, stdout.String())
	}
	if !strings.Contains(stderr.String(), "some sources carry a case") || strings.Contains(stderr.String(), "missing.log") {
		t.Errorf("stderr=%q, want the case failure before opening a source", stderr.String())
	}
}
