package api_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// matchConditionQuery は、指定した条件の選択を含む要求の文字列を組む。
func matchConditionQuery(items ...string) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, matchConditionParam+"="+item)
	}
	return strings.Join(parts, "&")
}

// matchConditionsExcept は、指定した条件を外したすべての条件の選択を含む文字列を返す。
func matchConditionsExcept(excluded ...core.ConditionKey) string {
	items := make([]string, 0, len(core.KnownConditionKeys()))
	for _, key := range core.KnownConditionKeys() {
		skipped := false
		for _, other := range excluded {
			if key == other {
				skipped = true
			}
		}
		if !skipped {
			items = append(items, string(key))
		}
	}
	return matchConditionQuery(items...)
}

// requestPath は 1 つの要求を送り、応答を返す。
func requestPath(
	t *testing.T, handler http.Handler, method, path string,
) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(method, path, nil))
	return response
}

// graphReadingPaths は、候補の絞り込みの条件を読む経路の要求を、条件を書かない形で返す。
func graphReadingPaths(t *testing.T, handler http.Handler) map[string]string {
	t.Helper()
	nodeId := terminalNodeId(t, handler)
	edgeId := anyGraphEdgeId(t, handler)
	return map[string]string{
		"探索":      graphPath + "?" + wholeGraphQuery,
		"時系列":     timelinePath,
		"ノードの詳細":  nodesPath + nodeId,
		"エッジの詳細":  edgesPath + edgeId,
		"URL の断片": edgesPath + edgeId + "/url-fragments",
		"所見の一覧":   assertionsPath,
	}
}

// グラフを読む経路は、条件の選択を書かない要求を退ける。
//
// **既定の選択を当てない。** 当てると、条件を渡し忘れた画面が「この条件で絞った」と
// 読める応答を受け取る。
func TestGraphReadingRoutesRequireTheMatchCondition(t *testing.T) {
	handler := graphHandler(t)
	for name, path := range graphReadingPaths(t, handler) {
		t.Run(name, func(t *testing.T) {
			problem := requestRecordsError(t, handler, path, http.StatusBadRequest)
			if problem.Code != core.ApiErrorCodeInvalidRequest || problem.Message == "" {
				t.Fatalf("error=%+v", problem)
			}
			assertMissingParameters(t, problem.MissingParameters,
				[]string{matchConditionParam})
		})
	}
}

// 関連付けの条件で変わらない経路は、条件の選択を書かない要求を通す。
func TestTheRoutesOutsideTheGraphTakeNoMatchCondition(t *testing.T) {
	handler := graphHandler(t)
	response := requestPath(t, handler, http.MethodGet, "/api/v0/sources")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%s",
			response.Code, http.StatusOK, response.Body.String())
	}
}

// 契約の外にある選択を退ける。
func TestMatchConditionRejectsTheSelectionOutsideTheContract(t *testing.T) {
	handler := graphHandler(t)
	cases := map[string]string{
		"契約の外の条件の種別":  matchConditionQuery("destination_host"),
		"幅が負である":      matchConditionQuery("second_of_time~-1"),
		"幅を取らない条件への幅": matchConditionQuery("destination_ip~2"),
		"幅が数でない":      matchConditionQuery("second_of_time~x"),
		"同じ条件の 2 回指定": matchConditionQuery("second_of_time", "second_of_time"),
		// 契約の外の文字列は、契約の中の条件と一緒に書いても退ける。
		"契約の外の文字列と条件の同時指定": matchConditionQuery(
			"none", string(core.ConditionKeyDestinationIp)),
	}
	for name, conditions := range cases {
		t.Run(name, func(t *testing.T) {
			problem := requestRecordsError(t, handler,
				graphPath+"?"+wholeGraphQuery+"&"+conditions, http.StatusBadRequest)
			if problem.Code != core.ApiErrorCodeInvalidRequest || problem.Message == "" {
				t.Fatalf("error=%+v", problem)
			}
			assertMissingParameters(t, problem.MissingParameters,
				[]string{matchConditionParam})
		})
	}
}

// candidateMatchesOf は、条件の選択 1 つで候補のエッジを読み、その関連付けを返す。
func candidateMatchesOf(t *testing.T, handler http.Handler, conditions string) []edgeMatch {
	t.Helper()
	edge := candidateEdgeWithConditions(t, handler, candidateEdgeFixture(t), conditions)
	response := requestPath(t, handler, http.MethodGet,
		edgesPath+edge.Id+"?"+conditions)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var decoded edgeResponse
	decodeJSON(t, response.Body, &decoded)
	return decoded.expandedMatches(t)
}

// candidateEdgeWithConditions は、条件の選択 1 つで組んだグラフから manifest の候補の
// エッジを、種別と起点のノードの種別で指定して読む。
func candidateEdgeWithConditions(
	t *testing.T, handler http.Handler, want candidateEdgeManifest, conditions string,
) graphEdge {
	t.Helper()
	response := requestPath(t, handler, http.MethodGet,
		graphPath+"?"+wholeGraphQuery+"&"+conditions)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var decoded graphResponse
	decodeJSON(t, response.Body, &decoded)
	return fixtureCandidateEdgeOf(t, decoded, want)
}

// matchPairOf は関連付けの結果 1 件を、起点と候補のレコードの位置で指す文字列にする。
func matchPairOf(t *testing.T, match edgeMatch) string {
	t.Helper()
	if match.OriginRef.LineNumber == nil || match.CandidateRef.SequenceNumber == nil {
		t.Fatalf("the match %+v carries no position to name it by", match)
	}
	return match.OriginRef.SourceFileName +
		":" + strconv.FormatInt(*match.OriginRef.LineNumber, 10) +
		"->" + match.CandidateRef.SourceFileName +
		":" + strconv.FormatInt(*match.CandidateRef.SequenceNumber, 10)
}

// 幅を広げた要求は、幅 0 の要求が挙げた候補を除かない。
func TestWiderToleranceKeepsEveryCandidateOfTheNarrowRequest(t *testing.T) {
	handler := graphHandler(t)
	narrow := candidateMatchesOf(t, handler, matchConditionsExcept())
	// 幅 0 の要求は manifest の候補のエッジの関連付けを読む。0 件どうしの比較で終わらせない。
	wantNarrow := make([]string, 0, len(candidateEdgeFixture(t).Matches))
	for _, match := range candidateEdgeFixture(t).Matches {
		wantNarrow = append(wantNarrow, match.OriginFileName+":"+
			strconv.FormatInt(match.OriginLineNumber, 10)+"->"+match.CandidateFileName+":"+
			strconv.FormatInt(match.CandidateSequenceNumber, 10))
	}
	gotNarrow := make([]string, 0, len(narrow))
	for _, match := range narrow {
		gotNarrow = append(gotNarrow, matchPairOf(t, match))
	}
	if !slices.Equal(gotNarrow, wantNarrow) {
		t.Fatalf("the narrow request carries the matches %v, want %v", gotNarrow, wantNarrow)
	}
	wide := candidateMatchesOf(t, handler,
		matchConditionsExcept(core.ConditionKeySecondOfTime)+
			"&"+matchConditionQuery("second_of_time~3"))
	within := make(map[string]struct{}, len(wide))
	for _, match := range wide {
		within[matchPairOf(t, match)] = struct{}{}
	}
	for _, match := range narrow {
		if _, kept := within[matchPairOf(t, match)]; !kept {
			t.Errorf("the wider request dropped the match %s", matchPairOf(t, match))
		}
	}
	if len(wide) < len(narrow) {
		t.Errorf("the wider request carries %d matches and the narrow one %d, "+
			"want the wider request to carry no fewer", len(wide), len(narrow))
	}
}

// 時刻の条件を選ばない関連付けは、段階 1 で終わり、時計のずれの前提を持たない。
func TestTheSelectionWithoutTheTimeConditionEndsAtTheClockIndependentStage(t *testing.T) {
	handler := graphHandler(t)
	matches := candidateMatchesOf(t, handler,
		matchConditionsExcept(core.ConditionKeySecondOfTime))
	if len(matches) == 0 {
		t.Fatal("the candidate edge carries no match without the time condition")
	}
	for index, match := range matches {
		if match.StageKey != string(core.StageKeyClockIndependent) {
			t.Errorf("match %d comes from the stage %q, want %q",
				index, match.StageKey, core.StageKeyClockIndependent)
		}
		last := match.StageTallies[len(match.StageTallies)-1]
		if last.StageKey != string(core.StageKeyClockIndependent) {
			t.Errorf("match %d ends at the stage %q, want %q",
				index, last.StageKey, core.StageKeyClockIndependent)
		}
		for _, assumption := range match.Assumptions {
			if assumption.AssumptionKey == core.AssumptionKeyClockOffsetBelowOneSecond {
				t.Errorf("match %d carries the clock assumption %q while it compares no time",
					index, assumption.AssumptionKey)
			}
		}
	}
}

// 選択の違う 2 つの要求は、それぞれ自分の選択に対応する応答を返す。
func TestTwoSelectionsReturnTheResponseOfTheirOwnSelection(t *testing.T) {
	handler := graphHandler(t)
	everyCondition := matchConditionsExcept()
	withoutTime := matchConditionsExcept(core.ConditionKeySecondOfTime)
	// 同じ handler へ交互に要求を出し、片方の答えがもう片方へ移らないことを確かめる。
	for round := 0; round < 2; round++ {
		timed := candidateMatchesOf(t, handler, everyCondition)
		untimed := candidateMatchesOf(t, handler, withoutTime)
		for index, match := range timed {
			if match.StageKey != string(core.StageKeySecondTimeMatched) {
				t.Fatalf("round %d: the match %d of the timed selection comes from %q, want %q",
					round, index, match.StageKey, core.StageKeySecondTimeMatched)
			}
		}
		for index, match := range untimed {
			if match.StageKey != string(core.StageKeyClockIndependent) {
				t.Fatalf("round %d: the match %d of the untimed selection comes from %q, want %q",
					round, index, match.StageKey, core.StageKeyClockIndependent)
			}
		}
	}
}

// 端末の割当を登録した後も、同じ選択で読んだグラフは割当を含めた取り込み結果から組む。
//
// 分析者が与えた端末はレコードが指す端末と別であり、この収集元ではノードを足さない。
// 割当を登録したことで先に読んだノードが消えないこと、同じ選択が引き続き応答することを
// 確かめる。
func TestTheGraphIsRebuiltAfterTheTerminalAssignment(t *testing.T) {
	handler := graphHandler(t)
	before := decodeGraph(t, handler, wholeGraphQuery)
	requestJSON(t, handler, http.MethodPost, terminalAssignmentsPath,
		analystAssignmentBody(t, handler), http.StatusCreated)
	after := decodeGraph(t, handler, wholeGraphQuery)

	carried := make(map[string]struct{}, len(after.Nodes))
	for _, node := range after.Nodes {
		carried[node.Id] = struct{}{}
	}
	for _, node := range before.Nodes {
		if _, kept := carried[node.Id]; !kept {
			t.Errorf("the node %q of the kind %q left the graph after the assignment",
				node.Id, node.Kind)
		}
	}
	if after.NodeCount != int64(len(after.Nodes)) {
		t.Errorf("the graph counts %d nodes and carries %d after the assignment",
			after.NodeCount, len(after.Nodes))
	}
	// 登録した割当は、同じ handler の一覧に残る。
	listed := requestJSON(t, handler, http.MethodGet, terminalAssignmentsPath, nil,
		http.StatusOK)
	var assignments struct {
		Assignments     []core.TerminalAssignment `json:"assignments"`
		AssignmentCount int64                     `json:"assignmentCount"`
	}
	decodeInto(t, listed, &assignments)
	analyst := 0
	for _, assignment := range assignments.Assignments {
		if assignment.Origin == core.TerminalAssignmentOriginAnalystSupplied {
			analyst++
		}
	}
	if analyst == 0 {
		t.Error("the listing carries no analyst assignment after the request recorded one")
	}
}

// graphBodyOf は、条件の選択 1 つで組んだグラフの応答の本文を返す。
func graphBodyOf(t *testing.T, handler http.Handler, conditions string) string {
	t.Helper()
	response := requestPath(t, handler, http.MethodGet,
		graphPath+"?"+wholeGraphQuery+"&"+conditions)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	return response.Body.String()
}

// 保つグラフの数には上限がある。上限を超えて別の選択を要求した後でも、先に要求した選択は
// 同じ答えを返す。
//
// **退避したグラフを取り込み結果から組み直す。** 退避で答えが変わると、分析者は同じ選択で
// 別の結果を受け取る。
func TestAnEvictedSelectionAnswersTheSameAfterItIsRebuilt(t *testing.T) {
	handler := graphHandler(t)
	first := matchConditionQuery(
		string(core.ConditionKeyDestinationIp),
		string(core.ConditionKeySecondOfTime)+"~0")
	before := graphBodyOf(t, handler, first)

	// 上限を超える数の別の選択を要求し、最初の選択を退避させる。幅の違う選択は別の鍵になる。
	for radius := 1; radius <= retainedMatchSelectionsAtLeast; radius++ {
		other := matchConditionQuery(
			string(core.ConditionKeyDestinationIp),
			string(core.ConditionKeySecondOfTime)+"~"+strconv.Itoa(radius))
		if body := graphBodyOf(t, handler, other); body == "" {
			t.Fatalf("the selection with the tolerance %d returned an empty body", radius)
		}
	}

	if after := graphBodyOf(t, handler, first); after != before {
		t.Error("the evicted selection answers differently after it is rebuilt")
	}
}

// 同じ条件を別の順で書いた 2 つの要求は、同じ関連付けを求める。
func TestTheOrderOfTheSelectedConditionsDoesNotChangeTheAnswer(t *testing.T) {
	handler := graphHandler(t)
	ascending := matchConditionQuery(
		string(core.ConditionKeyDestinationIp), string(core.ConditionKeyDestinationPort))
	descending := matchConditionQuery(
		string(core.ConditionKeyDestinationPort), string(core.ConditionKeyDestinationIp))
	if graphBodyOf(t, handler, ascending) != graphBodyOf(t, handler, descending) {
		t.Error("the same conditions written in another order answer differently")
	}
}
