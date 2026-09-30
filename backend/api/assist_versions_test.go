package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// changingAssignments は、List のたびに返す更新回数を revisions の順に進める割当の保存先である。
type changingAssignments struct {
	pipeline.TerminalAssignmentStore
	revisions []int64
	calls     int
}

func (s *changingAssignments) List() ([]core.TerminalAssignment, int64) {
	revision := s.revisions[min(s.calls, len(s.revisions)-1)]
	s.calls++
	return nil, revision
}

type fixedClock struct{}

func (fixedClock) Now() core.AssertionTime {
	return core.NewAssertionTime(time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC))
}

// versionsStore は、割当の更新回数だけを変える調査の保存先である。
type versionsStore struct {
	pipeline.InvestigationStore
	assignments *changingAssignments
	assertions  pipeline.AssertionStore
}

func (s versionsStore) TerminalAssignments() pipeline.TerminalAssignmentStore { return s.assignments }
func (s versionsStore) Assertions() pipeline.AssertionStore                   { return s.assertions }

func versionsHandlerOf(revisions ...int64) (assistConversationsHandler, *changingAssignments) {
	assignments := &changingAssignments{revisions: revisions}
	return assistConversationsHandler{investigation: versionsStore{
		assignments: assignments, assertions: pipeline.NewMemoryAssertionStore(fixedClock{}),
	}}, assignments
}

func noGraph(context.Context, pipeline.MatchConditionSelection) (pipeline.Graph, pipeline.ImportResult, error) {
	return pipeline.Graph{}, pipeline.ImportResult{}, nil
}

// グラフを読む間に割当が記録されたら読み直し、読み直したグラフと同じ時点の更新回数を返す。
func TestGraphWithVersionsRereadsWhenTheAnalystInputChanges(t *testing.T) {
	// 1 回目の読む前と後で 1 → 2 に変わり、2 回目は前後とも 2 である。
	handler, assignments := versionsHandlerOf(1, 2, 2, 2)
	_, versions, err := handler.graphWithVersions(context.Background(), pipeline.MatchConditionSelection{}, noGraph)
	if err != nil {
		t.Fatal(err)
	}
	if versions.AssignmentRevision != 2 || assignments.calls != 4 {
		t.Errorf("versions = %+v after %d reads, want the revision 2 read twice around the second graph",
			versions, assignments.calls)
	}
}

// 読み直しても入力が変わり続けるときは、組み直せない受け渡しを記録せずに失敗する。
func TestGraphWithVersionsGivesUpWhenTheAnalystInputKeepsChanging(t *testing.T) {
	handler, _ := versionsHandlerOf(1, 2, 3, 4, 5, 6, 7)
	_, _, err := handler.graphWithVersions(context.Background(), pipeline.MatchConditionSelection{}, noGraph)
	if !errors.Is(err, errAnalystInputKeptChanging) {
		t.Errorf("err = %v, want errAnalystInputKeptChanging", err)
	}
}
