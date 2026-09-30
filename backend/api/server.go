package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/output"
)

// frontendIndex は画面の build の入口の file である。
const frontendIndex = "index.html"

// allowReading は GET と HEAD だけを受け付ける操作が 405 で返す Allow header の値である。
const allowReading = "GET, HEAD"

// NewServerHandler は、API と画面の build を 1 つの origin にまとめる。
//
// `/api/` で始まる path は api が答え、残りの path は frontend が答える。frontend が nil の起動は
// 画面を配信せず、すべての path を api が答える。
func NewServerHandler(apiHandler http.Handler, frontend http.Handler) http.Handler {
	if frontend == nil {
		return apiHandler
	}
	mux := http.NewServeMux()
	mux.Handle("/api/", apiHandler)
	mux.Handle("/", frontend)
	return mux
}

// NewFrontendHandler は、root にある画面の build (`vite build` の出力) を配信する handler を返す。
//
// **root の外の file を開かない。** file は os.Root を通して開く。拡張子を持たない path で
// file を指さない GET と HEAD には index.html を返し、画面の中の path を直接開いた要求にも画面を
// 出す。**拡張子を持つ path で file が無い要求には 404 を返す。** server を別の build に替えた後に
// 古い画面が読む asset を、index.html で置き換えない。root が通常の file の index.html を
// 持たないときは error を返す。
func NewFrontendHandler(root *os.Root) (http.Handler, error) {
	info, err := root.Stat(frontendIndex)
	if err != nil {
		return nil, fmt.Errorf("reading the frontend build: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("reading the frontend build: %s is not a regular file", frontendIndex)
	}
	files := root.FS()
	return frontendHandler{files: files, server: http.FileServerFS(files)}, nil
}

// frontendHandler は画面の build の file と、file を指さない画面の中の path への index.html を返す。
type frontendHandler struct {
	files  fs.FS
	server http.Handler
}

func (h frontendHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", allowReading)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	// mux は符号化した path で `/api/` を振り分ける。`/api%2F...` も API の path であり、画面を返さない。
	if name == "api" || strings.HasPrefix(name, "api/") {
		http.NotFound(w, r)
		return
	}
	if name == "" || name == frontendIndex {
		h.serveIndex(w, r)
		return
	}
	regular, err := h.isRegularFile(name)
	switch {
	case err != nil:
		// #nosec G706 -- output.Sanitize が制御文字と改行を除いてから log へ渡す。
		slog.Error("reading the frontend build failed", "path", output.Sanitize(name), "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	case regular:
		if javaScriptExtension(name) {
			// module script と module worker は JavaScript の MIME type だけを受け付ける。
			// 拡張子から引く MIME type は host の MIME 表 (Windows の registry など) で変わる。
			w.Header().Set("Content-Type", javaScriptContentType)
		}
		h.server.ServeHTTP(w, r)
	case path.Ext(name) == "":
		h.serveIndex(w, r)
	default:
		http.NotFound(w, r)
	}
}

// javaScriptContentType は画面の build の JavaScript の file に付ける Content-Type である。
const javaScriptContentType = "text/javascript; charset=utf-8"

// javaScriptExtension は name が JavaScript の file の拡張子を持つことを返す。
func javaScriptExtension(name string) bool {
	switch path.Ext(name) {
	case ".js", ".mjs":
		return true
	default:
		return false
	}
}

// isRegularFile は name が root の中の通常の file であることを返す。
//
// **無い file と読めない file を分ける。** 無い file、OS が file の名前として受け付けない path
// (Windows の予約名と colon)、途中が directory でない path、root の外へ出る symlink を通る path は
// 偽を返す。権限で読めない file と、その他の読み取りの失敗は error を返す。
func (h frontendHandler) isRegularFile(name string) (bool, error) {
	if _, err := filepath.Localize(name); err != nil {
		return false, nil
	}
	info, err := fs.Stat(h.files, name)
	switch {
	case err == nil:
		return info.Mode().IsRegular(), nil
	case errors.Is(err, fs.ErrPermission):
		return false, err
	case errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrInvalid) || errors.Is(err, syscall.ENOTDIR):
		return false, nil
	case h.passesSymlink(name):
		// os.Root は root の外へ出る symlink を、公開された型を持たない error で退ける。
		return false, nil
	default:
		return false, err
	}
}

// passesSymlink は、name の途中または末尾の要素に symlink があることを返す。
func (h frontendHandler) passesSymlink(name string) bool {
	prefix := ""
	for element := range strings.SplitSeq(name, "/") {
		prefix = path.Join(prefix, element)
		info, err := fs.Lstat(h.files, prefix)
		if err != nil {
			return false
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return true
		}
	}
	return false
}

// serveIndex は index.html を返す。index.html は build ごとに変わる asset の名前を持つため、
// browser に保存したものを確かめずに使わせない。
//
// **再検証は内容の hash の ETag で決める。** 更新時刻は build を古いものに戻した起動や、更新時刻を
// 固定する配置で内容と対応しないため、Last-Modified を出さない。
func (h frontendHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	content, err := fs.ReadFile(h.files, frontendIndex)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, fs.ErrNotExist) {
			status = http.StatusNotFound
		}
		slog.Error("reading the frontend index failed", "error", err)
		http.Error(w, http.StatusText(status), status)
		return
	}
	digest := sha256.Sum256(content)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", `"`+hex.EncodeToString(digest[:])+`"`)
	http.ServeContent(w, r, frontendIndex, time.Time{}, bytes.NewReader(content))
}
