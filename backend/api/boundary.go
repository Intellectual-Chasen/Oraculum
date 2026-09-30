package api

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/output"
)

// NewBoundaryHandler は、server の待ち受け先として許可した host に届いた要求だけを next へ渡す。
//
// 次の要求を request_origin_rejected で退ける。
//   - Host が allowedHosts に無い要求。DNS rebinding で別の名前から届く要求を退ける。
//   - GET・HEAD 以外の要求で、Origin が allowedHosts の http の origin に無い要求。別の origin の
//     page が、利用者のブラウザーから状態を変える要求を送れない。Origin を持たない要求は
//     ブラウザー以外の client の要求であり、受け付ける。
//   - `/api/` の要求で、Sec-Fetch-Site が cross-site の要求。
//
// allowedHosts の値は `host:port` か、port 80 を表す `host` である。大文字と小文字を区別しない。
func NewBoundaryHandler(next http.Handler, allowedHosts []string) (http.Handler, error) {
	allowed := make(map[string]bool, len(allowedHosts))
	for _, host := range allowedHosts {
		normalized, err := normalizeHost(host)
		if err != nil {
			return nil, fmt.Errorf("reading the allowed host %q: %w", host, err)
		}
		allowed[normalized] = true
	}
	return &boundaryHandler{next: next, allowed: allowed}, nil
}

type boundaryHandler struct {
	next    http.Handler
	allowed map[string]bool
}

func (h *boundaryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if reason := h.rejection(r); reason != "" {
		// #nosec G706 -- output.Sanitize が制御文字と改行を除いてから log へ渡す。
		slog.Warn("refused a request at the server boundary", "reason", reason,
			"method", output.Sanitize(r.Method), "path", output.Sanitize(r.URL.Path),
			"host", output.Sanitize(r.Host), "origin", output.Sanitize(r.Header.Get("Origin")))
		writeError(w, http.StatusForbidden, core.ApiError{
			Code: core.ApiErrorCodeRequestOriginRejected, Message: reason,
		})
		return
	}
	h.next.ServeHTTP(w, r)
}

// rejection は要求を退ける理由を返す。受け付ける要求には空文字列を返す。
func (h *boundaryHandler) rejection(r *http.Request) string {
	if !h.allows(r.Host) {
		return "the request host is not an address of this server"
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		if origin, sent := r.Header["Origin"]; sent && !h.allowsOrigin(strings.Join(origin, ",")) {
			return "the request changing the state comes from another origin"
		}
	}
	if strings.HasPrefix(r.URL.Path, "/api/") && r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return "the api request comes from another site"
	}
	return ""
}

func (h *boundaryHandler) allows(host string) bool {
	normalized, err := normalizeHost(host)
	return err == nil && h.allowed[normalized]
}

func (h *boundaryHandler) allowsOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "http" || parsed.Path != "" || parsed.RawQuery != "" ||
		parsed.User != nil {
		return false
	}
	return h.allows(parsed.Host)
}

// CheckAllowedHost は、値が NewBoundaryHandler の allowedHosts に渡せる文字列であるかを確かめる。
func CheckAllowedHost(host string) error {
	_, err := normalizeHost(host)
	return err
}

// normalizeHost は `host:port` の文字列を小文字にし、port を持たない文字列に port 80 を補う。
//
// URL の authority として読めない文字列 (path、userinfo、閉じていない角括弧を持つ文字列) を退ける。
func normalizeHost(host string) (string, error) {
	host = strings.ToLower(host)
	parsed, err := url.Parse("http://" + host)
	if err != nil || parsed.Host != host || parsed.User != nil || parsed.Path != "" ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("the host %q is not a host and an optional port", host)
	}
	name, port := parsed.Hostname(), parsed.Port()
	if port == "" {
		if strings.HasSuffix(host, ":") {
			return "", fmt.Errorf("the host %q has an empty port", host)
		}
		port = "80"
	}
	if name == "" || strings.ContainsAny(name, "[]") || strings.Contains(name, ":") && !strings.HasPrefix(host, "[") {
		return "", fmt.Errorf("the host %q has no name or an IPv6 address without brackets", host)
	}
	return net.JoinHostPort(name, port), nil
}
