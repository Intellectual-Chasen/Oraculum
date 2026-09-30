package assist

import (
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// contentSecurityPolicy は中継が画面の応答に付ける Content-Security-Policy である。画面は同じ
// origin の file と API だけを読み、LLM の出力を経由して browser が外部へ要求を送る経路を持たない。
const contentSecurityPolicy = "default-src 'self'; img-src 'self' data: blob:; connect-src 'self'"

// conversationPathPrefix は、中継の tool だけが呼ぶ server の会話の endpoint の path である。
const conversationPathPrefix = "/api/v0/conversations"

// loopbackHosts は port と組んで Host に許す名前である。
var loopbackHosts = []string{"127.0.0.1", "localhost"}

// allowedHost は、Host が loopback の名前と中継の port の組であることを返す。DNS rebinding で
// 別の名前から届いた要求を退ける。
func allowedHost(host string, port int) bool {
	for _, name := range loopbackHosts {
		if host == name+":"+strconv.Itoa(port) {
			return true
		}
	}
	return false
}

// ownOrigin は、Origin が中継自身の origin であることを返す。
func ownOrigin(origin string, port int) bool {
	for _, name := range loopbackHosts {
		if origin == "http://"+name+":"+strconv.Itoa(port) {
			return true
		}
	}
	return false
}

// guardBrowser は browser からの要求を検査し、通った要求だけを next へ渡す。
//
//   - Host は loopback の名前と中継の port の組だけを受け付ける。
//   - `CONNECT` と absolute-form の要求を退ける。
//   - path は符号化した区切りと、整えていない文字列を退け、会話の endpoint への要求を退ける。
//   - GET と HEAD は、`Sec-Fetch-Site` が `same-origin` か `none` の要求と、この header を持たない
//     要求を受け付ける。
//   - それ以外の method は `Origin` が中継自身の origin の要求だけを受け付け、本文を持つ要求は
//     `Content-Type: application/json` だけを受け付ける。
func guardBrowser(port int, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		if status, reason := browserRefusal(r, port); status != 0 {
			http.Error(w, reason, status)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// browserRefusal は browser からの要求を退ける status と理由を返す。受け付ける要求では 0 を返す。
func browserRefusal(r *http.Request, port int) (int, string) {
	if r.Method == http.MethodConnect || !strings.HasPrefix(r.RequestURI, "/") {
		return http.StatusBadRequest, "the relay accepts only origin-form requests"
	}
	if !allowedHost(r.Host, port) {
		return http.StatusForbidden, "the Host is not the relay"
	}
	escapedPath := strings.ToLower(r.URL.EscapedPath())
	if strings.Contains(escapedPath, "%2f") || strings.Contains(escapedPath, "%5c") || !isCleanPath(r.URL.Path) {
		return http.StatusBadRequest, "the path must be written without encoded separators or dot segments"
	}
	if r.URL.Path == conversationPathPrefix || strings.HasPrefix(r.URL.Path, conversationPathPrefix+"/") {
		return http.StatusForbidden, "the conversation endpoints are for the relay tools only"
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		switch r.Header.Get("Sec-Fetch-Site") {
		case "", "same-origin", "none":
			return 0, ""
		default:
			return http.StatusForbidden, "the request comes from another site"
		}
	}
	if !ownOrigin(r.Header.Get("Origin"), port) {
		return http.StatusForbidden, "the Origin is not the relay"
	}
	if r.ContentLength != 0 || len(r.TransferEncoding) > 0 {
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			return http.StatusUnsupportedMediaType, "a request with a body must be application/json"
		}
	}
	return 0, ""
}

// isCleanPath は、path が `.` と `..` の区間と重なった区切りを持たないことを返す。末尾の区切りは
// 許す。整えた path で判定し、転送先が別の path として解釈する文字列を通さない。
func isCleanPath(value string) bool {
	cleaned := path.Clean(value)
	return cleaned == value || cleaned+"/" == value
}

// guardMCP は MCP の待ち受けへの要求を検査する。Host は loopback の名前と MCP の port の組だけを
// 受け付け、`Origin` を持つ要求 (browser からの要求) を退ける。bearer secret の検査は後に重ねる。
func guardMCP(port int, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedHost(r.Host, port) {
			http.Error(w, "the Host is not the relay", http.StatusForbidden)
			return
		}
		if r.Header.Get("Origin") != "" {
			http.Error(w, "the MCP endpoint does not accept browser requests", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
