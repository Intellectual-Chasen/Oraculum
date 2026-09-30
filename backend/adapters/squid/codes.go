package squid

import "strings"

// valueKind は format code が書く値の形である。幅の指定の読み方と、`.max` の意味が値の形で
// 変わる。
type valueKind int

const (
	// valueText は文字列の値である。`.max` は値を切り詰める。
	valueText valueKind = iota
	// valueNumber は整数の値である。`0` は 0 で埋め、`.max` は作用しない。
	valueNumber
	// valueDuration はミリ秒の時間である。`.max` は小数の桁数になる。
	valueDuration
	// valueTime は時刻の値である (ts、tl、tg)。
	valueTime
	// valueSubsecond は秒未満の時刻 (tu) である。`.max` が単位の桁数になる。
	valueSubsecond
)

// argumentUse は format code の `{arg}` の使い方である。zero value は `{arg}` を読まない code である。
type argumentUse int

const (
	// argumentNamesItem は `{arg}` を欄の名前に足す code である (ヘッダー、note など)。
	argumentNamesItem argumentUse = iota + 1
	// argumentStrftime は `{arg}` が strftime の書式である code である (tl、tg)。
	argumentStrftime
)

// formatCode は Squid の format code 1 つと欄の対応である。
type formatCode struct {
	// name は欄の名前である。argumentNamesItem の code では、`{arg}` を持たないときの名前である。
	name ItemName
	kind valueKind
	// argument は `{arg}` の使い方である。
	argument argumentUse
	// argumentPrefix は `{arg}` を持つときの欄の名前の接頭辞である。
	argumentPrefix string
}

// 本 package が他の欄と区別して扱う format code の欄の名前。
const (
	// ItemRequestMethod は %rm の要求 method の欄である。
	ItemRequestMethod ItemName = "requestMethod"
	// ItemRequestURL は %ru の要求先の欄である。
	ItemRequestURL ItemName = "requestUrl"
	// ItemClientRequestURL は %>ru の、client から受け取った要求先の欄である。
	ItemClientRequestURL ItemName = "clientRequestUrl"
	// ItemSquidRequestStatus は %Ss の Squid の要求処理の結果の欄である。
	ItemSquidRequestStatus ItemName = "squidRequestStatus"
	// ItemHierarchyStatus は %Sh の上位への転送の経路の欄である。
	ItemHierarchyStatus ItemName = "hierarchyStatus"
	// ItemSubsecondTime は %tu の秒未満の時刻の欄である。requestTime にまとめない %tu が
	// この欄になる。
	ItemSubsecondTime ItemName = "subsecondTime"
)

// 時刻の code が requestTime にならないときの欄の名前。
const (
	itemEpochTime ItemName = "epochTime"
	itemLocalTime ItemName = "localTime"
	itemGMTTime   ItemName = "gmtTime"
)

// defaultCodes は名前空間の接頭辞を持たない format code の表である。`http::` の接頭辞は
// 省いて探す。
//
// **code と意味の出典は Squid の src/format/Token.cc の TokenTable と、logformat の
// 文書である。** 表に無い code を読み飛ばさない。`ui` は現行の Squid に無いが、既存の
// 固定の並びが ident の位置に置くため残す。
var defaultCodes = map[string]formatCode{
	">a":               {name: ItemClientIP},
	">p":               {name: "clientPort", kind: valueNumber},
	">A":               {name: "clientFqdn"},
	"<a":               {name: ItemUpstreamIP},
	"<p":               {name: "upstreamPort", kind: valueNumber},
	"<A":               {name: "upstreamName"},
	">h":               {name: "requestHeaders", argument: argumentNamesItem, argumentPrefix: RequestHeaderItemPrefix},
	"<h":               {name: "responseHeaders", argument: argumentNamesItem, argumentPrefix: ResponseHeaderItemPrefix},
	">v":               {name: "protocolVersion"},
	">la":              {name: "clientLocalIp"},
	"la":               {name: "listeningIp"},
	">lp":              {name: "clientLocalPort", kind: valueNumber},
	"lp":               {name: "listeningPort", kind: valueNumber},
	"<la":              {name: "upstreamLocalIp"},
	"oa":               {name: "upstreamLocalIp"},
	"<lp":              {name: "upstreamLocalPort", kind: valueNumber},
	"ts":               {name: itemEpochTime, kind: valueTime},
	"tu":               {name: ItemSubsecondTime, kind: valueSubsecond},
	"tl":               {name: itemLocalTime, kind: valueTime, argument: argumentStrftime},
	"tg":               {name: itemGMTTime, kind: valueTime, argument: argumentStrftime},
	"tS":               {name: "transactionStartTime", kind: valueDuration},
	"tr":               {name: "responseTime", kind: valueDuration},
	"<pt":              {name: "peerResponseTime", kind: valueDuration},
	"<tt":              {name: "forwardingTime", kind: valueDuration},
	"dt":               {name: "dnsTime", kind: valueDuration},
	"busy_time":        {name: "busyTime", kind: valueNumber},
	">ha":              {name: "adaptedRequestHeaders", argument: argumentNamesItem, argumentPrefix: "adaptedRequestHeader."},
	"un":               {name: ItemUser},
	"ul":               {name: "userLogin"},
	"ue":               {name: "userExternal"},
	"ui":               {name: ItemIdent},
	"Hs":               {name: ItemStatusCode, kind: valueNumber},
	">Hs":              {name: ItemStatusCode, kind: valueNumber},
	"<Hs":              {name: ItemUpstreamStatusCode, kind: valueNumber},
	"<bs":              {name: "upstreamBodyBytes", kind: valueNumber},
	"Ss":               {name: ItemSquidRequestStatus},
	"Sh":               {name: ItemHierarchyStatus},
	"mt":               {name: ItemMimeType},
	">rm":              {name: "clientRequestMethod"},
	">ru":              {name: ItemClientRequestURL},
	">rs":              {name: "clientRequestScheme"},
	">rd":              {name: "clientRequestDomain"},
	">rP":              {name: "clientRequestPort", kind: valueNumber},
	">rp":              {name: "clientRequestPath"},
	">rv":              {name: "clientProtocolVersion"},
	"rm":               {name: ItemRequestMethod},
	"ru":               {name: ItemRequestURL},
	"rp":               {name: "requestPath"},
	"rv":               {name: "protocolVersion"},
	"rG":               {name: "requestUrlGroup"},
	"<rm":              {name: "upstreamRequestMethod"},
	"<ru":              {name: "upstreamRequestUrl"},
	"<rs":              {name: "upstreamRequestScheme"},
	"<rd":              {name: "upstreamRequestDomain"},
	"<rP":              {name: "upstreamRequestPort", kind: valueNumber},
	"<rp":              {name: "upstreamRequestPath"},
	"<rv":              {name: "upstreamProtocolVersion"},
	">st":              {name: ItemRequestBytes, kind: valueNumber},
	">sh":              {name: "requestHeaderBytes", kind: valueNumber},
	"<st":              {name: ItemReplyBytes, kind: valueNumber},
	"<sH":              {name: "replyHighOffset", kind: valueNumber},
	"<sS":              {name: "upstreamObjectSize", kind: valueNumber},
	"<sh":              {name: "replyHeaderBytes", kind: valueNumber},
	"st":               {name: "totalBytes", kind: valueNumber},
	"et":               {name: "externalAclTag"},
	"ea":               {name: "externalAclLog"},
	"sn":               {name: "sequenceNumber", kind: valueNumber},
	">eui":             {name: "clientEui"},
	">qos":             {name: "clientTos", kind: valueNumber},
	"<qos":             {name: "upstreamTos", kind: valueNumber},
	">nfmark":          {name: "clientNfmark", kind: valueNumber},
	"<nfmark":          {name: "upstreamNfmark", kind: valueNumber},
	">handshake":       {name: "clientHandshake"},
	"err_code":         {name: "errorCode"},
	"err_detail":       {name: "errorDetail"},
	"request_attempts": {name: "requestAttempts", kind: valueNumber},
	"note":             {name: "notes", argument: argumentNamesItem, argumentPrefix: "note."},
	"credentials":      {name: "credentials"},
	"master_xaction":   {name: "masterTransactionId", kind: valueNumber},
}

// namespacedCodes は `<名前空間>::` の接頭辞を持つ format code の表である。`tls::` は
// `ssl::` と同じ表を探す。
var namespacedCodes = map[string]map[string]formatCode{
	"adapt": {
		"all_trs": {name: "adaptationAllTimes", argument: argumentNamesItem, argumentPrefix: "adaptationAllTimes."},
		"sum_trs": {name: "adaptationSumTimes", argument: argumentNamesItem, argumentPrefix: "adaptationSumTimes."},
		"<last_h": {name: "adaptationLastHeaders", argument: argumentNamesItem, argumentPrefix: "adaptationLastHeader."},
	},
	"icap": {
		"tt":            {name: "icapTotalTime", kind: valueDuration},
		"<last_h":       {name: "adaptationLastHeaders", argument: argumentNamesItem, argumentPrefix: "adaptationLastHeader."},
		"<A":            {name: "icapServerIp"},
		"<service_name": {name: "icapServiceName"},
		"ru":            {name: "icapRequestUri"},
		"rm":            {name: "icapRequestMethod"},
		">st":           {name: "icapRequestBytes", kind: valueNumber},
		"<st":           {name: "icapResponseBytes", kind: valueNumber},
		"<bs":           {name: "icapResponseBodyBytes", kind: valueNumber},
		">h":            {name: "icapRequestHeaders", argument: argumentNamesItem, argumentPrefix: "icapRequestHeader."},
		"<h":            {name: "icapResponseHeaders", argument: argumentNamesItem, argumentPrefix: "icapResponseHeader."},
		"tr":            {name: "icapResponseTime", kind: valueDuration},
		"tio":           {name: "icapIoTime", kind: valueDuration},
		"to":            {name: "icapOutcome"},
		"Hs":            {name: "icapStatusCode", kind: valueNumber},
	},
	"ssl": {
		"bump_mode":                   {name: "sslBumpMode"},
		">cert_subject":               {name: "sslClientCertSubject"},
		">cert_issuer":                {name: "sslClientCertIssuer"},
		">sni":                        {name: "sslClientSni"},
		"<cert_subject":               {name: "sslServerCertSubject"},
		"<cert_issuer":                {name: "sslServerCertIssuer"},
		"<cert_errors":                {name: "sslServerCertErrors"},
		"<cert":                       {name: "sslServerCert"},
		">negotiated_version":         {name: "tlsClientNegotiatedVersion"},
		"<negotiated_version":         {name: "tlsServerNegotiatedVersion"},
		">negotiated_cipher":          {name: "tlsClientNegotiatedCipher"},
		"<negotiated_cipher":          {name: "tlsServerNegotiatedCipher"},
		">received_hello_version":     {name: "tlsClientHelloVersion"},
		"<received_hello_version":     {name: "tlsServerHelloVersion"},
		">received_supported_version": {name: "tlsClientSupportedVersion"},
		"<received_supported_version": {name: "tlsServerSupportedVersion"},
	},
	"proxy_protocol": {
		">h": {name: "proxyProtocolHeaders", argument: argumentNamesItem, argumentPrefix: "proxyProtocolHeader."},
	},
	"transport": {
		">connection_id": {name: "clientConnectionId", kind: valueNumber},
	},
}

// lookupCode は `%` と修飾の後ろの文字列から format code を探し、code の文字列の長さを返す。
//
// Squid と同じく、名前空間の接頭辞を先に見て、無ければ `http::` を外して既定の表を探す。
// 表の中は最長一致で探す。Squid は長い code の表から順に先頭一致で探すため、結果が同じになる。
func lookupCode(text string) (formatCode, int, bool) {
	for namespace, table := range namespacedCodes {
		for _, prefix := range namespacePrefixes(namespace) {
			if rest, found := strings.CutPrefix(text, prefix); found {
				code, length, ok := longestCode(table, rest)
				return code, len(prefix) + length, ok
			}
		}
	}
	if rest, found := strings.CutPrefix(text, "http::"); found {
		code, length, ok := longestCode(defaultCodes, rest)
		return code, len("http::") + length, ok
	}
	return longestCode(defaultCodes, text)
}

// namespacePrefixes は名前空間の表を探す接頭辞を返す。`tls::` は `ssl::` と同じ表を探す。
func namespacePrefixes(namespace string) []string {
	if namespace == "ssl" {
		return []string{"ssl::", "tls::"}
	}
	return []string{namespace + "::"}
}

func longestCode(table map[string]formatCode, text string) (formatCode, int, bool) {
	best, bestLength := formatCode{}, 0
	for key, code := range table {
		if len(key) > bestLength && strings.HasPrefix(text, key) {
			best, bestLength = code, len(key)
		}
	}
	return best, bestLength, bestLength > 0
}
