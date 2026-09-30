package pipeline_test

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// アップロード順が逆でも付属資料を分類し、無関係な資料を再読込しない。
func TestUploadClassifiesOnlyRelatedEvidence(t *testing.T) {
	for _, order := range [][]string{{"SYSTEM", "SYSTEM.LOG1"}, {"SYSTEM.LOG1", "SYSTEM"}} {
		t.Run(order[0], func(t *testing.T) {
			files := fstest.MapFS{"unrelated.evtx": {Data: []byte("ElfFile\x00synthetic")}}
			var opened []string
			open := func(name string) (io.ReadCloser, error) {
				opened = append(opened, name)
				return files.Open(name)
			}
			stages, err := pipeline.NewStages(context.Background(), pipeline.StagesConfig{
				Import: collectionConfig(),
				Requested: &pipeline.RequestedFiles{
					FS: files, Open: open,
					Upload: func(_, name string, body io.Reader) (string, error) {
						data, err := io.ReadAll(body)
						files[name] = &fstest.MapFile{Data: data}
						return name, err
					},
				},
				Recorder: pipeline.NewMemoryRecorder(), Layers: pipeline.DefaultGraphLayers(), Clock: fixedClock{},
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(stages.Wait)
			var result core.SourceUploadResult
			for _, name := range order {
				result, err = stages.UploadRequestedFile(context.Background(), "batch", name, strings.NewReader("regf-synthetic"))
				if err != nil {
					t.Fatal(err)
				}
			}
			if slices.Contains(opened, "unrelated.evtx") {
				t.Fatal("an unrelated file was read")
			}
			if len(result.Entries) != 2 {
				t.Fatalf("related entries: %+v", result.Entries)
			}
			for _, entry := range result.Entries {
				if entry.OriginPath == "SYSTEM" && !slices.Equal(entry.FormatCandidates, []core.FormatKey{"windows_registry_hive"}) {
					t.Fatal("the primary format was lost")
				}
				if entry.OriginPath == "SYSTEM.LOG1" && (entry.Undetected == nil || entry.Undetected.Reason != core.SourceFileUndetectedReasonCompanionFile) {
					t.Fatal("the companion was selected as independent evidence")
				}
			}
		})
	}
}
