// in-package test: 非公開の openRequestedFiles が基準の外を指す path を退けることを確かめる。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/api"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// sourceRootFixture は、基準の directory と基準の外の directory に file を置き、基準を開く。
func sourceRootFixture(t *testing.T) (string, string, func(string) (io.ReadCloser, error), func(string) (int64, error)) {
	t.Helper()
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	outside := filepath.Join(parent, "outside")
	for path, content := range map[string]string{
		filepath.Join(root, "collected", "inside.log"): "inside\n",
		filepath.Join(outside, "secret.log"):           "outside\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files, closer, err := openRequestedFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := closer.Close(); err != nil {
			t.Error(err)
		}
	})
	return root, outside, files.Open, files.Size
}

// stagedHandlerUnder は、root を基準の directory とし、要求による読み込みを受け付ける段階の API を返す。
func stagedHandlerUnder(t *testing.T, root string) http.Handler {
	t.Helper()
	config, err := importConfig()
	if err != nil {
		t.Fatal(err)
	}
	stages, closer, err := newStages(context.Background(), config, pipeline.NewMemoryRecorder(), root,
		pipeline.DefaultGraphLayers(), nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stages.Wait()
		if err := closer.Close(); err != nil {
			t.Error(err)
		}
	})
	handler, err := api.NewStagedHandler(stages, nil, pipeline.AttackRuleSet{})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

// requireRefusedLoading は、name を読み込む要求が 400 で退けられ、失敗の応答が want の理由と要求の
// path を含むことを確かめる。応答の文字列は、基準の path と OS の失敗の文字列を含まない。
func requireRefusedLoading(t *testing.T, handler http.Handler, root, name string, want core.LoadingRejection) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"sources": []map[string]any{
		{"originPath": name, "formatKey": "squid_combined"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v0/stages/loading", bytes.NewReader(body)))
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("the loading of %q answered %d, want %d: %s", name, recorder.Code, http.StatusBadRequest, recorder.Body)
		return
	}
	var apiError core.ApiError
	if err := json.Unmarshal(recorder.Body.Bytes(), &apiError); err != nil {
		t.Fatal(err)
	}
	if apiError.LoadingRejection != want || apiError.OriginPath != name {
		t.Errorf("the loading of %q was refused as %q for %q, want %q for %q",
			name, apiError.LoadingRejection, apiError.OriginPath, want, name)
	}
	for _, leaked := range []string{root, "path escapes", "not a directory", "permission denied"} {
		if strings.Contains(apiError.Message, leaked) {
			t.Errorf("the refusal of %q carries %q: %q", name, leaked, apiError.Message)
		}
	}
}

func readRequested(t *testing.T, open func(string) (io.ReadCloser, error), name string) (string, error) {
	t.Helper()
	file, err := open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), nil
}

// 基準の下の file と、基準の下を指す symbolic link は開ける。
func TestRequestedFilesOpenPathsUnderTheRoot(t *testing.T) {
	root, _, open, size := sourceRootFixture(t)
	if err := os.Symlink(filepath.Join("collected", "inside.log"), filepath.Join(root, "link.log")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"collected/inside.log", "link.log", "collected/../collected/inside.log"} {
		content, err := readRequested(t, open, name)
		if err != nil || content != "inside\n" {
			t.Errorf("opening %q returned %q, %v", name, content, err)
		}
		if got, err := size(name); err != nil || got != int64(len("inside\n")) {
			t.Errorf("the size of %q is %d, %v", name, got, err)
		}
	}
}

// os.Root の脱出の失敗と、openat2 の RESOLVE_BENEATH が返す EXDEV を脱出に数え、OS が返した
// その他の失敗と閉じた基準の失敗を脱出に数えない。
func TestEscapesRootTellsTheEscapeFromOtherFailures(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, escape := root.Stat("../outside.log")
	_ = root.Close() // 閉じた基準の失敗を作るために閉じる。
	_, closed := root.Stat("inside.log")
	for name, want := range map[string]struct {
		err     error
		escapes bool
	}{
		"escape":  {escape, true},
		"exdev":   {&os.PathError{Op: "openat2", Path: "x", Err: syscall.EXDEV}, true},
		"enotdir": {&os.PathError{Op: "openat", Path: "x", Err: syscall.ENOTDIR}, false},
		"eloop":   {&os.PathError{Op: "openat", Path: "x", Err: syscall.ELOOP}, false},
		"closed":  {closed, false},
	} {
		if want.err == nil {
			t.Fatalf("%s: the failure was not produced", name)
		}
		if got := escapesRoot(want.err); got != want.escapes {
			t.Errorf("%s: escapesRoot(%v) = %v, want %v", name, want.err, got, want.escapes)
		}
	}
}

// 基準の外を指す path と symbolic link は開かず、応答に基準の path を載せない。読み込みの要求は、
// 基準の外を指す理由で退ける。基準の外へ出てから戻る link と絶対 path の link は、指す先が基準の
// 中でも同じ理由で退ける。
func TestRequestedFilesRefusePathsLeavingTheRoot(t *testing.T) {
	root, outside, open, size := sourceRootFixture(t)
	if err := os.Symlink(filepath.Join(outside, "secret.log"), filepath.Join(root, "escape.log")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "outside"), filepath.Join(root, "escape-dir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "collected", "inside.log"), filepath.Join(root, "absolute-inside.log")); err != nil {
		t.Fatal(err)
	}
	// 基準の外へ出てから基準の中へ戻る相対 path の link である。
	if err := os.Symlink(filepath.Join("..", filepath.Base(root), "collected", "inside.log"),
		filepath.Join(root, "reentering.log")); err != nil {
		t.Fatal(err)
	}
	handler := stagedHandlerUnder(t, root)
	for _, name := range []string{
		"../outside/secret.log", filepath.Join(outside, "secret.log"), "escape.log", "escape-dir/secret.log",
		"collected/../../outside/secret.log", "absolute-inside.log", "reentering.log",
	} {
		if content, err := readRequested(t, open, name); err == nil {
			t.Errorf("opening %q read %q", name, content)
		} else if strings.Contains(err.Error(), root) {
			t.Errorf("the failure of %q names the root: %v", name, err)
		}
		if _, err := size(name); err == nil {
			t.Errorf("the size of %q was answered", name)
		}
		requireRefusedLoading(t, handler, root, name, core.LoadingRejectionPathOutsideBase)
	}
}

// 無い file と読めない file と file でない path を、別の文字列で退ける。読み込みの要求は、要求の
// 時点で理由の種別を分けて退ける。
func TestRequestedFilesTellTheReasonOfTheFailure(t *testing.T) {
	root, _, open, size := sourceRootFixture(t)
	if err := syscall.Mkfifo(filepath.Join(root, "collected", "pipe.log"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := stagedHandlerUnder(t, root)
	for name, want := range map[string]struct {
		err       error
		rejection core.LoadingRejection
	}{
		"absent.log":         {os.ErrNotExist, core.LoadingRejectionFileAbsent},
		"collected":          {pipeline.ErrRequestedFileNotRegular, core.LoadingRejectionFileNotRegular},
		"collected/pipe.log": {pipeline.ErrRequestedFileNotRegular, core.LoadingRejectionFileNotRegular},
		// 分類に該当しない失敗も、元の失敗を辿れる形に残す。
		"collected/inside.log/x": {syscall.ENOTDIR, core.LoadingRejectionFileUnreadable},
	} {
		if _, err := size(name); !errors.Is(err, want.err) {
			t.Errorf("the size of %q returned %v, want %v", name, err, want.err)
		}
		// FIFO を開いても書き手を待たずに退ける。
		if _, err := readRequested(t, open, name); !errors.Is(err, want.err) {
			t.Errorf("opening %q returned %v, want %v", name, err, want.err)
		}
		requireRefusedLoading(t, handler, root, name, want.rejection)
	}

	unreadable := filepath.Join(root, "collected", "unreadable.log")
	if err := os.WriteFile(unreadable, []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}
	if file, err := os.Open(unreadable); err == nil {
		_ = file.Close() // 読めるかを試しただけであり、読まない。
		t.Skip("権限を無視する利用者 (root) で走ると読めない file を作れない。一般利用者で走る CI が確かめる")
	} else if !errors.Is(err, os.ErrPermission) {
		t.Fatal(err)
	}
	if _, err := readRequested(t, open, "collected/unreadable.log"); !errors.Is(err, os.ErrPermission) ||
		!strings.Contains(err.Error(), "is not readable") {
		t.Errorf("opening an unreadable file returned %v", err)
	}
	if _, err := size("collected/unreadable.log"); !errors.Is(err, os.ErrPermission) {
		t.Errorf("the size of an unreadable file returned %v, want %v", err, os.ErrPermission)
	}
	requireRefusedLoading(t, handler, root, "collected/unreadable.log", core.LoadingRejectionFileUnreadable)
}

// listBase は基準の directory の一覧を要求し、status と本文を返す。
func listBase(t *testing.T, handler http.Handler, target string) (int, []byte) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder.Code, recorder.Body.Bytes()
}

// 基準の directory の一覧は、基準の下を指す symbolic link を file に数え、基準の外を指す link と
// FIFO を選べない項目にする。FIFO を開かずに一覧を返す。基準の外の directory は一覧にせず、
// 応答に基準の path を載せない。
func TestRequestedFilesListOnlyUnderTheRoot(t *testing.T) {
	root, outside, _, _ := sourceRootFixture(t)
	for link, target := range map[string]string{
		"link.log":   filepath.Join("collected", "inside.log"),
		"escape.log": filepath.Join(outside, "secret.log"),
		"escape-dir": filepath.Join("..", "outside"),
		"linked-dir": "collected",
	} {
		if err := os.Symlink(target, filepath.Join(root, link)); err != nil {
			t.Fatal(err)
		}
	}
	if err := syscall.Mkfifo(filepath.Join(root, "pipe.log"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := stagedHandlerUnder(t, root)

	status, body := listBase(t, handler, "/api/v0/stages/source-files")
	if status != http.StatusOK {
		t.Fatalf("the listing answered %d: %s", status, body)
	}
	var listing core.SourceFileListing
	if err := json.Unmarshal(body, &listing); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]core.SourceFileKind{}
	for _, entry := range listing.Entries {
		kinds[entry.Name] = entry.Kind
	}
	want := map[string]core.SourceFileKind{
		"collected": core.SourceFileKindDirectory, "linked-dir": core.SourceFileKindDirectory,
		"link.log": core.SourceFileKindFile, "escape.log": core.SourceFileKindOther, "escape-dir": core.SourceFileKindOther,
		"pipe.log": core.SourceFileKindOther,
	}
	for name, kind := range want {
		if kinds[name] != kind {
			t.Errorf("the entry %q is %q, want %q", name, kinds[name], kind)
		}
	}
	if strings.Contains(string(body), root) || strings.Contains(string(body), outside) {
		t.Errorf("the listing names a server path: %s", body)
	}

	// 再帰の一覧は symbolic link の directory を辿らず、選べない項目として残す。
	status, body = listBase(t, handler, "/api/v0/stages/source-files?recursive=true")
	if status != http.StatusOK || strings.Contains(string(body), "secret.log") ||
		strings.Count(string(body), `"name":"inside.log"`) != 1 ||
		!strings.Contains(string(body), `{"originPath":"linked-dir","name":"linked-dir","kind":"other"}`) {
		t.Errorf("the recursive listing answered %d: %s", status, body)
	}

	for _, target := range []string{
		"/api/v0/stages/source-files?path=escape-dir", "/api/v0/stages/source-files?path=../outside",
		"/api/v0/stages/source-files?path=" + filepath.ToSlash(outside),
	} {
		status, body := listBase(t, handler, target)
		if status != http.StatusBadRequest || strings.Contains(string(body), root) ||
			strings.Contains(string(body), "secret.log") {
			t.Errorf("%s answered %d: %s", target, status, body)
		}
	}
}
