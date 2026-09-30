package api

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const proxyBypassPattern = "GET /api/v0/proxy-bypass" //nolint:gosec // 操作の経路の文字列であり、資格情報の値ではない。

// proxyBypassHandler は、Proxy のログの収集元 1 つについて、接続元のアドレスごとに Proxy が
// 記録した要求の件数と、他の収集元のレコードが記録した Proxy を経由しない接続の件数を返す
// (pipeline.ImportResult.ProxyBypassOf)。
type proxyBypassHandler struct {
	result pipeline.ImportResult
}

// proxyBypassDestination は Proxy を経由しない接続の接続先 1 つである。
type proxyBypassDestination struct {
	Address string `json:"address"`
	// Port は接続先 port の文字列である。port を記録しない接続では出ない。
	Port        string `json:"port,omitempty"`
	RecordCount int64  `json:"recordCount"`
}

// proxyBypassClient は接続元のアドレス 1 つの件数である。件数の単位はレコードであり、割当の
// 適用期間とレコードの時刻を比べない (pipeline.ProxyBypassClient)。
type proxyBypassClient struct {
	ClientIp string `json:"clientIp"`
	// Terminals は端末の割当がこのアドレスに結んだ端末である。要素数 0 の場合も集合である。
	Terminals             []string `json:"terminals"`
	ProxyRequestCount     int64    `json:"proxyRequestCount"`
	ProxyConnectionCount  int64    `json:"proxyConnectionCount"`
	DirectConnectionCount int64    `json:"directConnectionCount"`
	// DirectConnectionCountable が偽のとき、このアドレスの接続を記録する収集元が無く、
	// proxyConnectionCount と directConnectionCount の 0 は数えた結果ではない
	// (pipeline.ProxyBypassClient.DirectConnectionCountable)。
	DirectConnectionCountable bool                     `json:"directConnectionCountable"`
	DirectDestinations        []proxyBypassDestination `json:"directDestinations"`
}

// proxyBypassResponse は Proxy を経由した要求と経由しない接続の比較の応答である。
// **本型が項目の定義元である。**
type proxyBypassResponse struct {
	SourceId string `json:"sourceId"`
	// ProxyAddresses は収集元に付けた端末の IP アドレスである。要素数 0 のときは、接続の
	// 件数を数えず、Proxy のログの要求だけを数える。
	ProxyAddresses []string `json:"proxyAddresses"`
	// UnreadableProxyRecordCount は接続元を IP アドレスとして読めず、clients に数えなかった
	// Proxy のログのレコードの件数である。
	UnreadableProxyRecordCount int64               `json:"unreadableProxyRecordCount"`
	Clients                    []proxyBypassClient `json:"clients"`
}

func (h proxyBypassHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if err := checkProxyBypassParameterNames(query); err != nil {
		writeError(w, http.StatusBadRequest, *invalidRequestError(err, nil))
		return
	}
	if !query.Has(sourceIdParam) || !query.Has(sourceContentSha256Param) {
		writeError(w, http.StatusBadRequest, *invalidRequestError(
			errors.New("required query parameters are missing"),
			[]string{sourceIdParam, sourceContentSha256Param}))
		return
	}
	source := requestedSource{sourceId: query.Get(sourceIdParam), contentSha256: query.Get(sourceContentSha256Param)}
	if _, apiError := checkRequestedSource(h.result, source); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	bypass, found := h.result.ProxyBypassOf(source.sourceId)
	if !found {
		writeError(w, http.StatusBadRequest, *invalidRequestError(
			errors.New("the requested source is not a proxy log"), []string{sourceIdParam}))
		return
	}
	writeJSON(w, http.StatusOK, proxyBypassResponseOf(source.sourceId, bypass))
}

// proxyBypassResponseOf は集計 bypass を応答の形に写す。
func proxyBypassResponseOf(sourceId string, bypass pipeline.ProxyBypass) proxyBypassResponse {
	response := proxyBypassResponse{
		SourceId: sourceId, ProxyAddresses: bypass.ProxyAddresses,
		UnreadableProxyRecordCount: bypass.UnreadableProxyRecordCount,
		Clients:                    make([]proxyBypassClient, 0, len(bypass.Clients)),
	}
	for _, client := range bypass.Clients {
		destinations := make([]proxyBypassDestination, 0, len(client.DirectDestinations))
		for _, destination := range client.DirectDestinations {
			destinations = append(destinations, proxyBypassDestination{
				Address: destination.Address, Port: destination.Port, RecordCount: destination.RecordCount,
			})
		}
		response.Clients = append(response.Clients, proxyBypassClient{
			ClientIp: client.ClientIp, Terminals: client.Terminals,
			ProxyRequestCount: client.ProxyRequestCount, ProxyConnectionCount: client.ProxyConnectionCount,
			DirectConnectionCount:     client.DirectConnectionCount,
			DirectConnectionCountable: client.DirectConnectionCountable, DirectDestinations: destinations,
		})
	}
	return response
}

// checkProxyBypassParameterNames は要求の項目の名前と多重度を確かめる。
func checkProxyBypassParameterNames(query url.Values) error {
	known := map[string]struct{}{
		matchConditionParam: {}, sourceIdParam: {}, sourceContentSha256Param: {},
	}
	for name, values := range query {
		if _, ok := known[name]; !ok {
			return errors.New("unsupported query parameter")
		}
		if len(values) != 1 && !repeatedRequestItem(name) {
			return errors.New("query parameter must occur once")
		}
	}
	return nil
}
