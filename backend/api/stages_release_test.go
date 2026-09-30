// in-package test: 非公開の stagedHandler が処理の前の収集元の一覧を手放したかを確かめる。
package api

import (
	"context"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// switchedStages は読み込みを終えた段階であり、処理を終えたかを test が切り替える。
type switchedStages struct {
	store     pipeline.InvestigationStore
	processed bool
}

func (s *switchedStages) Snapshot() core.InvestigationStages { return core.InvestigationStages{} }

func (s *switchedStages) StartRequestedLoading([]pipeline.SourcePlan) error {
	return pipeline.ErrStageAlreadyStarted
}

func (s *switchedStages) StartRequestedLoadingThenProcessing([]pipeline.SourcePlan) error {
	return pipeline.ErrStageAlreadyStarted
}

func (s *switchedStages) StartProcessing() error { return pipeline.ErrStageAlreadyStarted }

func (s *switchedStages) ListRequestedFiles(context.Context, string, bool) (core.SourceFileListing, error) {
	return core.SourceFileListing{}, pipeline.ErrLoadingRequestInvalid
}

func (s *switchedStages) Loaded() (pipeline.InvestigationStore, bool) { return s.store, true }

func (s *switchedStages) Processed() (pipeline.Session, bool) {
	if !s.processed {
		return pipeline.Session{}, false
	}
	return pipeline.Session{
		Store: s.store, Catalog: pipeline.NewGraphCatalog(s.store, pipeline.DefaultGraphLayers()),
	}, true
}

// 処理を終えた後の要求は処理を終えた調査の API が答えるため、処理の前に組んだ収集元の一覧を
// 手放す。時刻の解釈を当てた取り込み結果はレコードの複製を持ち、server が止まるまで残さない。
func TestStagedHandlerReleasesTheLoadedSourcesAfterTheProcessing(t *testing.T) {
	stages := &switchedStages{store: pipeline.NewMemoryStore(pipeline.ImportResult{}, pipeline.SystemClock{})}
	handler := &stagedHandler{stages: stages}
	if _, loaded := handler.loadedSources(); !loaded {
		t.Fatal("the sources before the processing were not answered")
	}
	if handler.loaded == nil {
		t.Fatal("the handler did not keep the sources before the processing")
	}

	stages.processed = true
	if _, ready := handler.processedHandler(); !ready {
		t.Fatal("the investigation after the processing was not answered")
	}
	if handler.loaded != nil {
		t.Error("the handler keeps the sources built before the processing after the processing")
	}
}

// 処理を終える前に処理の API を確かめた要求が、処理を終えた後に収集元の一覧を求めても、処理の前の
// 一覧を組み直さず、処理を終えた調査の API が答える。
func TestStagedHandlerKeepsNoLoadedSourcesForARequestOverlappingTheProcessing(t *testing.T) {
	stages := &switchedStages{store: pipeline.NewMemoryStore(pipeline.ImportResult{}, pipeline.SystemClock{})}
	handler := &stagedHandler{stages: stages}
	// 1 つ目の要求は、処理を終える前に処理の API を確かめる。
	if _, ready := handler.processedHandler(); ready {
		t.Fatal("the investigation was answered before the processing")
	}
	// 2 つ目の要求が、処理を終えた後に処理の API を組む。
	stages.processed = true
	processed, ready := handler.processedHandler()
	if !ready {
		t.Fatal("the investigation after the processing was not answered")
	}
	// 1 つ目の要求が収集元の一覧を求める。
	sources, loaded := handler.loadedSources()
	if !loaded {
		t.Fatal("the sources were not answered")
	}
	if sources != processed {
		t.Error("the sources after the processing were not answered by the investigation after the processing")
	}
	if handler.loaded != nil {
		t.Error("the handler built the sources before the processing again after the processing")
	}
}
