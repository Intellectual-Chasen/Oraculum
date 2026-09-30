package sourceargs_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/cmd/internal/sourceargs"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const exampleSpec = `%>a [%tl] "%rm %ru HTTP/%rv" %>Hs %<st`

func failingRead(string) ([]byte, error) { return nil, errors.New("no such file") }

func readsSpec(content string) sourceargs.ReadFile {
	return func(string) ([]byte, error) { return []byte(content), nil }
}

// 欄の並びの指定は、それより後ろに並ぶ収集元に適用する。前の収集元には適用しない。
func TestParseAppliesTheSpecToTheFollowingSources(t *testing.T) {
	plans, rest, err := sourceargs.Parse([]string{
		"squid_combined:logs/a.log",
		"--logformat", exampleSpec,
		"squid_logformat:logs/b.log",
		"squid_logformat:logs/c.log",
	}, failingRead)
	if err != nil {
		t.Fatal(err)
	}
	if len(rest) != 0 {
		t.Errorf("the parser left %v unread", rest)
	}
	want := []struct {
		path string
		spec string
	}{
		{"logs/a.log", ""},
		{"logs/b.log", exampleSpec},
		{"logs/c.log", exampleSpec},
	}
	if len(plans) != len(want) {
		t.Fatalf("the parser read %d sources, want %d", len(plans), len(want))
	}
	for index, expected := range want {
		plan := plans[index]
		if plan.OriginPath != expected.path {
			t.Errorf("source %d points %q, want %q", index, plan.OriginPath, expected.path)
		}
		switch {
		case expected.spec == "" && plan.FormatSpec != nil:
			t.Errorf("%q carries the spec %q, want none", plan.OriginPath, *plan.FormatSpec)
		case expected.spec != "" && plan.FormatSpec == nil:
			t.Errorf("%q carries no spec, want the requested one", plan.OriginPath)
		case expected.spec != "" && *plan.FormatSpec != expected.spec:
			t.Errorf("%q carries the spec %q, want %q", plan.OriginPath, *plan.FormatSpec, expected.spec)
		}
	}
}

// 収集元ごとに別の並びを渡せる。後ろの指定が前の指定を上書きしない。
func TestParseGivesEachSourceItsOwnSpec(t *testing.T) {
	const other = `%>a [%tl] "%rm %ru HTTP/%rv" %>Hs %>st %<st`
	plans, _, err := sourceargs.Parse([]string{
		"--logformat", exampleSpec, "squid_logformat:logs/a.log",
		"--logformat", other, "squid_logformat:logs/b.log",
	}, failingRead)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 {
		t.Fatalf("the parser read %d sources, want 2", len(plans))
	}
	if *plans[0].FormatSpec != exampleSpec || *plans[1].FormatSpec != other {
		t.Errorf("the sources carry %q and %q", *plans[0].FormatSpec, *plans[1].FormatSpec)
	}
}

// file から渡した並びが、引数で渡した並びと同じ計画になる。行末の改行を値に含めない。
func TestParseReadsTheSpecFromAFile(t *testing.T) {
	plans, _, err := sourceargs.Parse(
		[]string{"--logformat-file", "squid.conf.logformat", "squid_logformat:logs/a.log"},
		readsSpec(exampleSpec+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || plans[0].FormatSpec == nil {
		t.Fatalf("the parser read %v", plans)
	}
	if *plans[0].FormatSpec != exampleSpec {
		t.Errorf("the spec = %q, want %q", *plans[0].FormatSpec, exampleSpec)
	}
}

// 値を 1 つ取ると呼び出し側が告げた flag は、値ごとそのまま残りへ送る。
// 収集元の指定と同じ文字列の値を収集元と取り違えない。
func TestParseKeepsTheValueOfADeclaredFlag(t *testing.T) {
	plans, rest, err := sourceargs.Parse(
		[]string{"--addr", "127.0.0.1:8080", "squid_combined:logs/a.log"},
		failingRead, "--addr")
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || plans[0].OriginPath != "logs/a.log" {
		t.Fatalf("the parser read %v", plans)
	}
	if len(rest) != 2 || rest[0] != "--addr" || rest[1] != "127.0.0.1:8080" {
		t.Errorf("the parser left %v, want the flag and its value", rest)
	}
}

// 読めない指定を計画にしない。
func TestParseRejectsUnreadableArguments(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		args     []string
		readFile sourceargs.ReadFile
		reason   string
	}{
		{"an absolute path", []string{"squid_combined:/etc/passwd"}, failingRead, "relative path"},
		{"an empty format key", []string{":logs/a.log"}, failingRead, "relative path"},
		{"an empty path", []string{"squid_combined:"}, failingRead, "relative path"},
		{"a spec flag without a value", []string{"--logformat"}, failingRead, "requires a value"},
		{"an empty spec", []string{"--logformat", ""}, failingRead, "non-empty value"},
		{"an unreadable spec file",
			[]string{"--logformat-file", "missing"}, failingRead, "no such file"},
		{"an empty spec file",
			[]string{"--logformat-file", "empty"}, readsSpec("\n"), "declares no item"},
		{"a declared flag without a value",
			[]string{"squid_combined:logs/a.log", "--addr"}, failingRead, "requires a value"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			plans, _, err := sourceargs.Parse(testCase.args, testCase.readFile, "--addr")
			if err == nil {
				t.Fatalf("the parser accepted %v as %v", testCase.args, plans)
			}
			if !strings.Contains(err.Error(), testCase.reason) {
				t.Errorf("the error %q does not carry %q", err, testCase.reason)
			}
			if plans != nil {
				t.Errorf("the parser returned the plans %v with an error", plans)
			}
		})
	}
}

// 収集元の指定でも flag でもない引数は、呼び出し側が読む分として順序を保って残る。
func TestParseLeavesTheArgumentsOfTheCaller(t *testing.T) {
	var plans []pipeline.SourcePlan
	plans, rest, err := sourceargs.Parse(
		[]string{"--verbose", "squid_combined:logs/a.log", "leftover"}, failingRead)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 {
		t.Fatalf("the parser read %d sources, want 1", len(plans))
	}
	if len(rest) != 2 || rest[0] != "--verbose" || rest[1] != "leftover" {
		t.Errorf("the parser left %v, want the two arguments in order", rest)
	}
}
