package markii

import (
	"net"
	"net/url"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// urlHostFieldName は url から導いた接続先の host の項目の名前である。
const urlHostFieldName = "urlHost"

// derivationUrlHost は導き方を表す文字列である。分析者が画面で読む値であるため日本語で書く。
const derivationUrlHost = "url の文字列の authority から port と IPv6 の角括弧を外した host"

// webUrlSchemes は接続先を導く scheme である。
//
// 既知の制限: 接続先を導く scheme を http と https の 2 つに限る,
// url は res や c のようなローカルの資源を指す scheme と、scheme を書かない文字列も持つ。
// scheme を書かない文字列は authority と path を区別できず、ローカルの資源はネットワークの
// 接続先を持たない,
// scheme を書かない文字列の authority を確定できる規則を原資料で確かめたとき、
// または別の scheme の接続先を収集元で確認したときに、導く範囲を見直す
var webUrlSchemes = map[string]struct{}{"http": {}, "https": {}}

// urlHostField は url の文字列から接続先の host の項目を導く。
// ok が偽になるのは、原資料が接続先の host の欄を持つとき、url の key が無いとき、
// scheme が対象の 2 つでないとき、host の文字列が無いときである。
//
// host の文字列が IP アドレスのときは接続先 IP アドレス、ホスト名のときは接続先
// ホスト名になる。
func urlHostField(record Record) (core.RecordField, bool) {
	// **原資料が接続先の host の欄を持つレコードから導かない。** 導いた値と原資料の文字列が
	// 同じ意味の項目を 2 つ作ると、1 件のアクセスの接続先のノードが 2 つに分かれる。
	if _, recorded := record.Field(keyUrlHostname); recorded {
		return core.RecordField{}, false
	}
	field, found := record.Field(keyUrl)
	if !found {
		return core.RecordField{}, false
	}
	parsed, err := url.Parse(field.Value())
	if err != nil {
		return core.RecordField{}, false
	}
	if _, supported := webUrlSchemes[strings.ToLower(parsed.Scheme)]; !supported {
		return core.RecordField{}, false
	}
	host := parsed.Hostname()
	if host == "" {
		return core.RecordField{}, false
	}
	semantic := core.SemanticKeyConnectionDestinationHostname
	if net.ParseIP(host) != nil {
		semantic = core.SemanticKeyConnectionDestinationAddress
	}
	value, err := core.NewDerivedValue(host, derivationUrlHost)
	if err != nil {
		return core.RecordField{}, false
	}
	derived, err := core.NewTextField(urlHostFieldName, semantic, value)
	if err != nil {
		return core.RecordField{}, false
	}
	return derived, true
}
