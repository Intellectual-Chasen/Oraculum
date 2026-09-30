package pipeline_test

import (
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

func namesOf(plans []pipeline.SourcePlan) []string {
	names := make([]string, len(plans))
	for i, plan := range plans {
		names[i] = plan.FileName
	}
	return names
}

func planAt(originPath, fileName string) pipeline.SourcePlan {
	return pipeline.SourcePlan{OriginPath: originPath, FileName: fileName, FormatKey: pipeline.SquidFormatKey}
}

// 同じ file 名の plan だけに、親 directory のうち違う階層を足す。共通の前と後ろの階層は外す。
// 親が同じ plan は、入力形式、案件、位置の順に違いの出る値で区別する。
func TestDistinguishFileNames(t *testing.T) {
	caseA, caseB := "case-a", "case-b"
	withCase := func(plan pipeline.SourcePlan, caseId *string) pipeline.SourcePlan {
		plan.CaseId = caseId
		return plan
	}
	withFormat := func(plan pipeline.SourcePlan, format core.FormatKey) pipeline.SourcePlan {
		plan.FormatKey = format
		return plan
	}
	for name, test := range map[string]struct {
		plans []pipeline.SourcePlan
		want  []string
	}{
		"1 階層で区別する": {
			plans: []pipeline.SourcePlan{planAt("a/x.log", "x.log"), planAt("b/x.log", "x.log"), planAt("c/y.log", "y.log")},
			want:  []string{"x.log (a)", "x.log (b)", "y.log"},
		},
		"共通の後ろの階層を外す": {
			plans: []pipeline.SourcePlan{
				planAt("collect/host-a/logs/deep/x.log", "x.log"), planAt("collect/host-b/logs/deep/x.log", "x.log"),
			},
			want: []string{"x.log (host-a)", "x.log (host-b)"},
		},
		"違う階層が 2 つ": {
			plans: []pipeline.SourcePlan{planAt("a/p/s/x.log", "x.log"), planAt("b/q/s/x.log", "x.log")},
			want:  []string{"x.log (a, p)", "x.log (b, q)"},
		},
		"親を持たない plan": {
			plans: []pipeline.SourcePlan{planAt("x.log", "x.log"), planAt("sub/x.log", "x.log")},
			want:  []string{"x.log (.)", "x.log (sub)"},
		},
		"同じ取得元を別の案件で": {
			plans: []pipeline.SourcePlan{
				withCase(planAt("a/x.log", "x.log"), &caseA), withCase(planAt("a/x.log", "x.log"), &caseB),
			},
			want: []string{"x.log (case-a)", "x.log (case-b)"},
		},
		"同じ取得元を別の入力形式で": {
			plans: []pipeline.SourcePlan{
				planAt("a/x.log", "x.log"), withFormat(planAt("a/x.log", "x.log"), pipeline.SquidLogFormatKey),
			},
			want: []string{"x.log (" + string(pipeline.SquidFormatKey) + ")", "x.log (" + string(pipeline.SquidLogFormatKey) + ")"},
		},
		"同じ取得元を 2 回": {
			plans: []pipeline.SourcePlan{planAt("a/x.log", "x.log"), planAt("a/x.log", "x.log")},
			want:  []string{"x.log (1)", "x.log (2)"},
		},
		"名前が 1 件ずつ": {
			plans: []pipeline.SourcePlan{planAt("a/x.log", "x.log"), planAt("b/y.log", "y.log")},
			want:  []string{"x.log", "y.log"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			original := slices.Clone(test.plans)
			got := pipeline.DistinguishFileNames(test.plans)
			if !slices.Equal(namesOf(got), test.want) {
				t.Errorf("names = %q, want %q", namesOf(got), test.want)
			}
			if !slices.Equal(namesOf(test.plans), namesOf(original)) {
				t.Error("the caller's plans were rewritten")
			}
			if again := pipeline.DistinguishFileNames(got); !slices.Equal(namesOf(again), test.want) {
				t.Errorf("distinguishing twice gives %q", namesOf(again))
			}
		})
	}
}

// 取り込みの結果の収集元の一覧と、レコードの位置が、区別した名前を持つ。
func TestRunnerCarriesTheDistinguishedFileNames(t *testing.T) {
	const line = `192.0.2.10 - - [03/Feb/2001:04:10:00 +0000] "GET http://a.example.test/ HTTP/1.1" ` +
		`200 3 "-" "agent" TCP_MISS:HIER_DIRECT` + "\n"
	runner, err := pipeline.NewRunner(runnerConfig(line))
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run([]pipeline.SourcePlan{planAt("host-a/x.log", "x.log"), planAt("host-b/x.log", "x.log")})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := result.SourceEntries()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"x.log (host-a)", "x.log (host-b)"}
	for i, entry := range entries {
		if entry.Identity.FileName != want[i] {
			t.Errorf("identity %d = %q, want %q", i, entry.Identity.FileName, want[i])
		}
		publication, found := result.Publication(entry.Status.SourceId)
		if !found || len(publication.Records()) == 0 {
			t.Fatalf("source %d has no records", i)
		}
		if got := publication.Records()[0].Locator.SourceFileName; got != want[i] {
			t.Errorf("record locator %d = %q, want %q", i, got, want[i])
		}
	}
}
