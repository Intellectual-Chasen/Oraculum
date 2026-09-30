package api_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/api"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// matchConditionParam は、候補を絞るのに用いる条件を持つ要求の項目である。
const matchConditionParam = "matchCondition"

// retainedMatchSelectionsAtLeast は、handler が同時に保つ選択の数の下限である。
//
// **定義元は api の retainedMatchSelections である。** 本 package は外の test package で
// あり、非公開の定数を読めない。検査が求めるのは退避を起こすことだけであるため、定義元の値を
// 写さず、定義元がこの値を下回らないことだけを前提にする。定義元を下げるときは本定数も下げる。
const retainedMatchSelectionsAtLeast = 4

// allMatchConditionsQuery は、契約が定めるすべての条件を幅 0 で選ぶ要求の文字列を返す。
//
// **条件の種別を検査の文字列に複製しない。** 定義元は core.KnownConditionKeys() であり、
// 条件を足した変更で検査の文字列が通知なしに古くならない形にする。
func allMatchConditionsQuery() string {
	keys := core.KnownConditionKeys()
	items := make([]string, 0, len(keys))
	for _, key := range keys {
		items = append(items, matchConditionParam+"="+string(key))
	}
	return strings.Join(items, "&")
}

// withAllMatchConditions は、グラフを読む要求の URL へすべての条件の選択を足す。
func withAllMatchConditions(rawURL string) string {
	separator := "?"
	if strings.Contains(rawURL, "?") {
		separator = "&"
	}
	return rawURL + separator + allMatchConditionsQuery()
}

// testAssertionEpoch は所見を記録した時刻の起点である。
// 検査が時刻を期待値に置けるよう、ホストの時計から切り離した値にする。
var testAssertionEpoch = time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

// testAssertionStep は 1 回の記録ごとに進む幅である。
const testAssertionStep = time.Second

// testAssertionClock は所見を記録した時刻を、固定の起点から固定の幅で進める。
// 改訂ごとに異なる時刻になり、記録の順が時刻に出る。
type testAssertionClock struct{ ticks int64 }

func (c *testAssertionClock) Now() core.AssertionTime {
	c.ticks++
	return core.NewAssertionTime(testAssertionEpoch.Add(time.Duration(c.ticks) * testAssertionStep))
}

// testHandler は取り込み結果とメモリ上の所見の保存先を組にした handler を返す。
func testHandler(result pipeline.ImportResult) http.Handler {
	return handlerOf(pipeline.NewMemoryStore(result, &testAssertionClock{}))
}

// handlerOf は store と、store のグラフを本番と同じ関数で組む catalog から handler を返す。
func handlerOf(store pipeline.InvestigationStore) http.Handler {
	return handlerWithLayers(store, pipeline.DefaultGraphLayers())
}

func handlerWithLayers(store pipeline.InvestigationStore, layers pipeline.GraphLayers) http.Handler {
	rules, err := pipeline.LoadAttackRuleSet("../rules/attack")
	if err != nil {
		panic(err)
	}
	handler, err := api.NewHandler(store, pipeline.NewGraphCatalog(store, layers), pipeline.SigmaEvaluation{},
		rules)
	if err != nil {
		panic(err)
	}
	return handler
}

// twoFormatImportResult は Squid と markii 形式のレコードの fixture を、その順で取り込む。
//
// 収集元の順を保つ。応答の並びが収集元の取り込みの入力順に依存するためである。
func twoFormatImportResult(t *testing.T) pipeline.ImportResult {
	t.Helper()
	manifest := recordsFixtures(t)
	plans := make([]pipeline.SourcePlan, 0, 2)
	for _, record := range []manifestRecord{manifest.SquidRecord, manifest.MarkiiRecord} {
		plans = append(plans, pipeline.SourcePlan{
			FormatKey: record.Format, FileName: record.File, OriginPath: "testdata/" + record.File,
		})
	}
	return importResultOfFiles(t, plans...)
}

// withheldImportResult は公開を停止する収集元だけを取り込んだ結果と、その識別を返す。
func withheldImportResult(t *testing.T) (http.Handler, core.SourceIdentity) {
	t.Helper()
	result := importResultOfFiles(t, pipeline.SourcePlan{
		FormatKey: markIIFormatKey,
		FileName:  "collision.log", OriginPath: "testdata/collision.log",
	})
	entries, err := result.SourceEntries()
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Status.PublicationState != core.PublicationStateWithheld {
		t.Fatalf("publicationState=%q want withheld", entries[0].Status.PublicationState)
	}
	return testHandler(result), entries[0].Identity
}

func importResultOfFiles(t *testing.T, plans ...pipeline.SourcePlan) pipeline.ImportResult {
	t.Helper()
	result, err := newTestRunner(t).Run(plans)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
