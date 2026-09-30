package api_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// decodeListing は基準の directory の一覧を読み、不変条件を確かめる。
func decodeListing(t *testing.T, body []byte) core.SourceFileListing {
	t.Helper()
	var listing core.SourceFileListing
	if err := json.Unmarshal(body, &listing); err != nil {
		t.Fatalf("decoding the listing %s: %v", body, err)
	}
	if err := listing.Validate(); err != nil {
		t.Fatalf("the listing does not validate: %v (%s)", err, body)
	}
	return listing
}

// entryNamed は一覧の中の名前 name の項目を返す。
func entryNamed(t *testing.T, listing core.SourceFileListing, name string) core.SourceFileEntry {
	t.Helper()
	at := slices.IndexFunc(listing.Entries, func(entry core.SourceFileEntry) bool { return entry.Name == name })
	if at < 0 {
		t.Fatalf("the listing has no entry %q", name)
	}
	return listing.Entries[at]
}

// 基準の directory の一覧は、file ごとに読める入力形式の候補か、候補が無い理由を持つ。
func TestStagedHandlerListsTheBaseDirectoryWithFormatCandidates(t *testing.T) {
	_, handler := newStagedFixture(t, true)

	listing := decodeListing(t, serveStaged(t, handler, http.MethodGet, "/api/v0/stages/source-files", "", http.StatusOK))
	if listing.Path != "." || listing.Recursive || listing.Truncated {
		t.Errorf("the listing is %q recursive=%v truncated=%v, want the base directory in full", listing.Path,
			listing.Recursive, listing.Truncated)
	}
	for name, want := range map[string]core.FormatKey{"markii.log": "infotrace_mark_ii", "squid.log": "squid_combined"} {
		entry := entryNamed(t, listing, name)
		if entry.Kind != core.SourceFileKindFile || !slices.Equal(entry.FormatCandidates, []core.FormatKey{want}) {
			t.Errorf("the entry %q is %+v, want a file read as %s", name, entry, want)
		}
	}
	manifest := entryNamed(t, listing, "manifest.json")
	if manifest.Undetected == nil || manifest.Undetected.Reason != core.SourceFileUndetectedReasonUnsupportedFormat {
		t.Errorf("the manifest is %+v, want a file of no supported format", manifest)
	}

	recursive := decodeListing(t, serveStaged(t, handler, http.MethodGet,
		"/api/v0/stages/source-files?path=.&recursive=true", "", http.StatusOK))
	if !recursive.Recursive || len(recursive.Entries) != len(listing.Entries) {
		t.Errorf("the recursive listing has %d entries, want the %d files of the directory without subdirectories",
			len(recursive.Entries), len(listing.Entries))
	}
}

// 基準の外、directory でない path、読めない query は 400 で退ける。
func TestStagedHandlerRefusesAnInvalidListing(t *testing.T) {
	_, handler := newStagedFixture(t, true)
	for target, rejection := range map[string]core.LoadingRejection{
		"/api/v0/stages/source-files?path=..":            core.LoadingRejectionPathOutsideBase,
		"/api/v0/stages/source-files?path=markii.log":    core.LoadingRejectionNotDirectory,
		"/api/v0/stages/source-files?path=absent":        core.LoadingRejectionFileAbsent,
		"/api/v0/stages/source-files?recursive=yes":      "",
		"/api/v0/stages/source-files?depth=1":            "",
		"/api/v0/stages/source-files?path=.&path=absent": "",
	} {
		t.Run(target, func(t *testing.T) {
			body := serveStaged(t, handler, http.MethodGet, target, "", http.StatusBadRequest)
			apiError := requireStageErrorCode(t, body, core.ApiErrorCodeInvalidRequest)
			if apiError.LoadingRejection != rejection {
				t.Errorf("the failure is %q, want %q", apiError.LoadingRejection, rejection)
			}
		})
	}

	_, withoutBase := newStagedFixture(t, false)
	body := serveStaged(t, withoutBase, http.MethodGet, "/api/v0/stages/source-files", "", http.StatusBadRequest)
	if apiError := requireStageErrorCode(t, body, core.ApiErrorCodeInvalidRequest); apiError.LoadingRejection !=
		core.LoadingRejectionNoBaseDirectory {
		t.Errorf("the failure is %q, want %q", apiError.LoadingRejection, core.LoadingRejectionNoBaseDirectory)
	}
}

// thenProcess を持つ読み込みの要求は、読み込みの完了に続けて処理を終える。
func TestStagedHandlerProcessesAfterTheLoadingWhenRequested(t *testing.T) {
	stages, handler := newStagedFixture(t, true)
	request := strings.TrimSuffix(loadingRequestOfFixtures(t), "}") + `,"thenProcess":true}`

	serveStaged(t, handler, http.MethodPost, "/api/v0/stages/loading", request, http.StatusAccepted)
	stages.Wait()

	snapshot := decodeStages(t, serveStaged(t, handler, http.MethodGet, "/api/v0/stages", "", http.StatusOK))
	if snapshot.Processing.State != core.StageStateCompleted {
		t.Fatalf("the processing is %q (failure %+v), want completed", snapshot.Processing.State,
			snapshot.Processing.Failure)
	}
	serveStaged(t, handler, http.MethodGet, processedTarget, "", http.StatusOK)
}
