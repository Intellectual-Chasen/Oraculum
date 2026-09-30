package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/api"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

const (
	frontendIndexBody = "<!doctype html><title>index</title>"
	frontendAssetBody = "export const worker = 1;"
)

// newFrontendRoot は index.html と asset を 1 つ持つ画面の build を作り、その os.Root を返す。
func newFrontendRoot(t *testing.T) *os.Root {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(frontendIndexBody), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"worker.js", "module.mjs"} {
		if err := os.WriteFile(filepath.Join(dir, "assets", name), []byte(frontendAssetBody), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	})
	return root
}

// newServerFixture は段階を始めていない API と、画面の build をまとめた handler を返す。
func newServerFixture(t *testing.T, withFrontend bool) http.Handler {
	t.Helper()
	_, staged := newStagedFixture(t, false)
	var frontend http.Handler
	if withFrontend {
		var err error
		frontend, err = api.NewFrontendHandler(newFrontendRoot(t))
		if err != nil {
			t.Fatal(err)
		}
	}
	return api.NewServerHandler(staged, frontend)
}

func serve(handler http.Handler, method, target string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, target, nil))
	return recorder
}

// 画面を配信する起動でも、調査の API は段階を終えるまで段階の失敗で答える。
func TestServerHandlerPassesTheAPIToTheStages(t *testing.T) {
	handler := newServerFixture(t, true)
	response := serve(handler, http.MethodGet, processedTarget)
	if response.Code != http.StatusConflict {
		t.Fatalf("status of %s = %d, want 409: %s", processedTarget, response.Code, response.Body)
	}
	var got core.ApiError
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Code != core.ApiErrorCodeStageNotReady {
		t.Errorf("code = %q, want %q", got.Code, core.ApiErrorCodeStageNotReady)
	}
}

func TestFrontendHandlerServesTheBuild(t *testing.T) {
	handler := newServerFixture(t, true)
	cases := []struct {
		target, wantBody, wantType string
	}{
		{"/", frontendIndexBody, "text/html"},
		// JavaScript の file は host の MIME 表に依らず同じ Content-Type で届く。
		{"/assets/worker.js", frontendAssetBody, "text/javascript; charset=utf-8"},
		{"/assets/module.mjs", frontendAssetBody, "text/javascript; charset=utf-8"},
		// 画面の中の path を直接開いた要求にも画面を出す。
		{"/investigation/node", frontendIndexBody, "text/html"},
		{"/assets/", frontendIndexBody, "text/html"},
	}
	for _, c := range cases {
		response := serve(handler, http.MethodGet, c.target)
		if response.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", c.target, response.Code)
			continue
		}
		if got := response.Body.String(); got != c.wantBody {
			t.Errorf("%s: body = %q, want %q", c.target, got, c.wantBody)
		}
		if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, c.wantType) {
			t.Errorf("%s: Content-Type = %q, want %s", c.target, got, c.wantType)
		}
	}
	if got := serve(handler, http.MethodGet, "/").Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("index Cache-Control = %q, want no-cache", got)
	}
	if response := serve(handler, http.MethodPost, "/"); response.Code != http.StatusMethodNotAllowed ||
		response.Header().Get("Allow") != "GET, HEAD" {
		t.Errorf("POST / status = %d Allow = %q, want 405 with GET, HEAD",
			response.Code, response.Header().Get("Allow"))
	}
	// HEAD は GET と同じ status と Content-Type を返し、本文を返さない。
	for _, c := range []struct{ target, wantType string }{
		{"/", "text/html"}, {"/assets/worker.js", "text/javascript; charset=utf-8"},
	} {
		response := serve(handler, http.MethodHead, c.target)
		if response.Code != http.StatusOK || response.Body.Len() != 0 ||
			!strings.HasPrefix(response.Header().Get("Content-Type"), c.wantType) {
			t.Errorf("HEAD %s: status = %d body = %q Content-Type = %q", c.target, response.Code, response.Body,
				response.Header().Get("Content-Type"))
		}
	}
	// 拡張子を持つ path で file が無い要求は、index.html で置き換えず 404 で答える。
	for _, target := range []string{"/assets/missing.js", "/favicon.ico"} {
		if response := serve(handler, http.MethodGet, target); response.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", target, response.Code)
		}
	}
	// `/api/` で始まる path は画面を返さず、API が答える。
	response := serve(handler, http.MethodGet, "/api/v1/unknown")
	if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), frontendIndexBody) {
		t.Errorf("/api/v1/unknown: status = %d body = %q, want the API's 404", response.Code, response.Body)
	}
}

// root の外を指す path は root の中の path に丸め、root の外の file を返さない。
func TestFrontendHandlerStaysInsideTheRoot(t *testing.T) {
	parent := t.TempDir()
	if err := os.WriteFile(filepath.Join(parent, "secret.txt"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	build := filepath.Join(parent, "dist")
	if err := os.Mkdir(build, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(build, "index.html"), []byte(frontendIndexBody), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(parent, "secret.txt"), filepath.Join(build, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(parent, filepath.Join(build, "linkdir")); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(build)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	handler, err := api.NewFrontendHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		target   string
		wantCode int
		wantBody string
	}{
		// root の外への移動は root の中の path に丸め、root の中に無い file として 404 を返す。
		{"/../secret.txt", http.StatusNotFound, ""},
		// root の外を指す symlink は root の中の file として開けず、404 を返す。
		{"/link.txt", http.StatusNotFound, ""},
		// root の外を指す symlink の directory の下の path も、root の中に無い path として扱う。
		{"/linkdir/secret.txt", http.StatusNotFound, ""},
		{"/linkdir/page", http.StatusOK, frontendIndexBody},
		// 拡張子を持たない path は root の外へ出ず、画面を返す。
		{"/../../etc", http.StatusOK, frontendIndexBody},
	}
	if runtime.GOOS == "windows" {
		// OS が file の名前として受け付けない path (予約名、colon) は、無い file として扱う。
		cases = append(cases, []struct {
			target   string
			wantCode int
			wantBody string
		}{
			{"/CON", http.StatusOK, frontendIndexBody},
			{"/a:b.js", http.StatusNotFound, ""},
		}...)
	}
	for _, c := range cases {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.URL.Path = c.target
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if strings.Contains(recorder.Body.String(), "outside") {
			t.Errorf("%s answered a file outside the root", c.target)
		}
		if recorder.Code != c.wantCode {
			t.Errorf("%s: status = %d, want %d", c.target, recorder.Code, c.wantCode)
		}
		if c.wantBody != "" && recorder.Body.String() != c.wantBody {
			t.Errorf("%s: body = %q, want %q", c.target, recorder.Body, c.wantBody)
		}
	}
}

// index.html の再検証は内容で決まる。更新時刻が同じで内容の違う build に替えた server は、前の
// build の index.html を browser に使わせない。
func TestFrontendHandlerRevalidatesTheIndexByItsContent(t *testing.T) {
	modified := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	build := func(body string) http.Handler {
		dir := t.TempDir()
		index := filepath.Join(dir, "index.html")
		if err := os.WriteFile(index, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(index, modified, modified); err != nil {
			t.Fatal(err)
		}
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = root.Close() })
		handler, err := api.NewFrontendHandler(root)
		if err != nil {
			t.Fatal(err)
		}
		return handler
	}
	previous, current := build("<title>previous</title>"), build("<title>current</title>")
	first := serve(previous, http.MethodGet, "/")
	tag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || tag == "" {
		t.Fatalf("status = %d ETag = %q, want 200 with an ETag", first.Code, tag)
	}
	revalidate := func(handler http.Handler, header, value string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set(header, value)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}
	for _, c := range []struct{ header, value string }{
		{"If-None-Match", tag},
		{"If-Modified-Since", modified.Format(http.TimeFormat)},
	} {
		response := revalidate(current, c.header, c.value)
		if response.Code != http.StatusOK || response.Body.String() != "<title>current</title>" {
			t.Errorf("%s from the previous build: status = %d body = %q, want 200 with the current index",
				c.header, response.Code, response.Body)
		}
	}
	if response := revalidate(previous, "If-None-Match", tag); response.Code != http.StatusNotModified {
		t.Errorf("If-None-Match of the same build: status = %d, want 304", response.Code)
	}
}

// 権限で読めない file は、無い file と分けて 500 で答え、index.html で置き換えない。
func TestFrontendHandlerReportsAnUnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("the directory permission does not stop this process from reading the file")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(frontendIndexBody), 0o600); err != nil {
		t.Fatal(err)
	}
	assets := filepath.Join(dir, "assets")
	if err := os.Mkdir(assets, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "worker.js"), []byte(frontendAssetBody), 0o600); err != nil {
		t.Fatal(err)
	}
	// root の中の読めない file を指す symlink も、無い file と分けて答える。
	if err := os.Symlink(filepath.Join("assets", "worker.js"), filepath.Join(dir, "linked.js")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(assets, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(assets, 0o700) })
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	handler, err := api.NewFrontendHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"/assets/worker.js", "/assets/page", "/linked.js"} {
		response := serve(handler, http.MethodGet, target)
		if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), frontendIndexBody) {
			t.Errorf("%s: status = %d body = %q, want 500", target, response.Code, response.Body)
		}
	}
}

// 符号化した `/` で `/api/` を書いた path にも画面を返さない。
func TestFrontendHandlerDoesNotAnswerAnEncodedAPIPath(t *testing.T) {
	handler := newServerFixture(t, true)
	for _, target := range []string{"/api%2Fv0%2Fstages", "/api%2F"} {
		response := serve(handler, http.MethodGet, target)
		if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), frontendIndexBody) {
			t.Errorf("%s: status = %d body = %q, want 404", target, response.Code, response.Body)
		}
	}
}

// 画面を配信しない起動では、`/api/` の外の path も API が答え、画面を返さない。
func TestServerHandlerWithoutFrontend(t *testing.T) {
	handler := newServerFixture(t, false)
	response := serve(handler, http.MethodGet, "/")
	if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), frontendIndexBody) {
		t.Errorf("GET / status = %d body = %q, want the API's 404", response.Code, response.Body)
	}
}

func TestFrontendHandlerRequiresTheIndex(t *testing.T) {
	withoutIndex := t.TempDir()
	// index.html の名前を持つ directory は、画面の入口の file として受け付けない。
	indexDirectory := t.TempDir()
	if err := os.Mkdir(filepath.Join(indexDirectory, "index.html"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{withoutIndex, indexDirectory} {
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := api.NewFrontendHandler(root); err == nil {
			t.Errorf("NewFrontendHandler accepted %s", dir)
		}
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	}
}
