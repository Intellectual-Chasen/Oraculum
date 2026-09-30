package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 同名・入れ子の資料を保存しても既存資料を変えず、本文の失敗で部分的な資料を残さない。
func TestRequestedUploadKeepsEvidenceAndCleansPartialFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "original.log"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	files, closer, err := openRequestedFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	first, err := files.Upload("", "folder/original.log", strings.NewReader("one"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := files.Upload("", "folder/original.log", strings.NewReader("two"))
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("uploads reused an evidence path")
	}
	for name, want := range map[string]string{"original.log": "original", first: "one", second: "two"} {
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(got) != want {
			t.Fatalf("%s: %q, %v", name, got, err)
		}
	}
	before, err := os.ReadDir(filepath.Join(root, "oraculum-uploads"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := files.Upload("", "partial.log", brokenUploadReader{}); err == nil {
		t.Fatal("a broken upload succeeded")
	}
	after, err := os.ReadDir(filepath.Join(root, "oraculum-uploads"))
	if err != nil || len(before) != len(after) {
		t.Fatal("partial evidence was retained")
	}
	if _, err := files.Upload("", "../escape.log", strings.NewReader("outside")); err == nil {
		t.Fatal("an escaping upload succeeded")
	}
	if _, err := files.Upload("same-batch", "folder/one.log", strings.NewReader("one")); err != nil {
		t.Fatal(err)
	}
	if _, err := files.Upload("same-batch", "folder/two.log", strings.NewReader("two")); err != nil {
		t.Fatal(err)
	}
	if _, err := files.Upload("same-batch", "folder/one.log", strings.NewReader("overwrite")); err == nil {
		t.Fatal("a duplicate upload overwrote evidence")
	}
	if _, err := files.Upload("same-batch", "folder/partial.log", brokenUploadReader{}); err == nil {
		t.Fatal("a broken batch upload succeeded")
	}
	got, err := os.ReadFile(filepath.Join(root, "oraculum-uploads/same-batch/folder/one.log"))
	if err != nil || string(got) != "one" {
		t.Fatal("a failed upload removed earlier evidence")
	}
}

type brokenUploadReader struct{}

func (brokenUploadReader) Read([]byte) (int, error) { return 0, errors.New("upload interrupted") }

var _ io.Reader = brokenUploadReader{}

// 同じ保存先への再送はサーバー障害とせず、元資料を保った競合として返す。
func TestDuplicateUploadConflict(t *testing.T) {
	root := t.TempDir()
	handler := stagedHandlerUnder(t, root)
	const target = "/api/v0/stages/source-files?path=folder/sample.log&upload=batch"
	for index, body := range []string{"original", "replacement"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, target, strings.NewReader(body)))
		want := http.StatusCreated
		if index == 1 {
			want = http.StatusConflict
		}
		if response.Code != want {
			t.Fatalf("upload %d: %d %s", index, response.Code, response.Body.String())
		}
		if index == 1 {
			var failure core.ApiError
			if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil || failure.Code != core.ApiErrorCodeSourceUploadAlreadyExists {
				t.Fatalf("the duplicate has no specific reason: %+v, %v", failure, err)
			}
		}
	}
	got, err := os.ReadFile(filepath.Join(root, "oraculum-uploads/batch/folder/sample.log"))
	if err != nil || string(got) != "original" {
		t.Fatal("the duplicate upload replaced evidence")
	}
}

// HTTP から保存した資料は同じ一覧・形式判定を通り、基準の外への path を退ける。
func TestSourceUploadHTTP(t *testing.T) {
	root := t.TempDir()
	handler := stagedHandlerUnder(t, root)
	request := httptest.NewRequest(http.MethodPost, "/api/v0/stages/source-files?path=folder/sample.log", strings.NewReader("unrecognized log"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", response.Code, response.Body.String())
	}
	var listing core.SourceUploadResult
	if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if len(listing.Entries) != 1 || listing.Entries[0].SizeBytes == nil {
		t.Fatalf("invalid upload listing: %+v", listing)
	}
	status, _ := listBase(t, handler, "/api/v0/stages/source-files?path="+path.Dir(listing.Entries[0].OriginPath))
	if status != http.StatusOK {
		t.Fatal("uploaded evidence cannot be listed")
	}
	for _, target := range []string{"/api/v0/stages/source-files?path=../outside.log", "/api/v0/stages/source-files?path=/absolute.log", "/api/v0/stages/source-files?path=x&path=y"} {
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, target, strings.NewReader("x")))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s answered %d", target, response.Code)
		}
	}
}

// 既存の一覧の表示上限で打ち切られても、今アップロードした資料を応答から落とさない。
func TestUploadDoesNotListUnrelatedFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "oraculum-uploads/batch/folder")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for index := range 5000 {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%05d.log", index)), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	response := httptest.NewRecorder()
	stagedHandlerUnder(t, root).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v0/stages/source-files?path=folder/zz-new.log&upload=batch", strings.NewReader("new evidence")))
	if response.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", response.Code, response.Body.String())
	}
	var listing core.SourceUploadResult
	if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if len(listing.Entries) != 1 || !slices.ContainsFunc(listing.Entries, func(entry core.SourceFileEntry) bool {
		return entry.OriginPath == "oraculum-uploads/batch/folder/zz-new.log"
	}) {
		t.Fatal("uploaded evidence was omitted after listing truncation")
	}
}
