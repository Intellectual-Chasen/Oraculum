// in-package test: 非公開の CLI run が Sigma のルールの集合を取り込みの指定へ記録し、読み戻して突き合わせることを確かめる。
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/cmd/internal/sourceargs"
)

// ルール。4688 の子のプロセスの path の末尾に一致する。
const childRule = `title: Synthetic child start
logsource:
  product: windows
  category: process_creation
detection:
  selection:
    Image|endswith: '\child.exe'
  condition: selection
`

// processCreationXML は 4688 を 1 件持つ Windows イベントログの XML である。
const processCreationXML = `<Events><Event><System><Provider Name="Microsoft-Windows-Security-Auditing"/>` +
	`<EventID>4688</EventID><TimeCreated SystemTime="2001-02-03T04:05:06Z"/><EventRecordID>9</EventRecordID>` +
	`<Channel>Security</Channel><Computer>host-a.example.test</Computer></System>` +
	`<EventData><Data Name="NewProcessName">C:\Example\child.exe</Data></EventData></Event></Events>` + "\n"

func TestRunRecordsTheSigmaRulesAndChecksThemOnReplay(t *testing.T) {
	restoreDefaultLogger(t)
	work := t.TempDir()
	t.Chdir(work)
	rules := filepath.Join(work, "rules")
	if err := os.MkdirAll(rules, 0o750); err != nil {
		t.Fatal(err)
	}
	rulePath := filepath.Join(rules, "child.yml")
	for path, content := range map[string]string{rulePath: childRule, "events.xml": processCreationXML} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{sourceargs.SigmaRulesFlag, rules, "windows_event_xml:events.xml"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "1 evaluated, 0 not evaluated, 1 candidates") {
		t.Errorf("stderr = %q", stderr.String())
	}
	var response struct {
		ImportSpec json.RawMessage `json:"importSpec"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	var spec sourceargs.ImportSpec
	if err := json.Unmarshal(response.ImportSpec, &spec); err != nil {
		t.Fatal(err)
	}
	if spec.SigmaRules == nil || spec.SigmaRules.Directory != rules || spec.SigmaRules.RevisionSource != "unverified" ||
		len(spec.SigmaRules.ContentSha256) != 64 {
		t.Fatalf("sigmaRules = %+v", spec.SigmaRules)
	}
	if err := os.WriteFile("spec.json", response.ImportSpec, 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"--import-spec", "spec.json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("replay: exit=%d stderr=%q", code, stderr.String())
	}
	// 記録した後にルールを書き換えた集合は、記録と内容の識別が食い違う。
	if err := os.WriteFile(rulePath, []byte(childRule+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := run([]string{"--import-spec", "spec.json"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "recorded") {
		t.Fatalf("changed rules: exit=%d stderr=%q", code, stderr.String())
	}
}
