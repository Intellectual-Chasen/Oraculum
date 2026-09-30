package winevent

import (
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Velocidex/ordereddict"
)

// ライブラリがレコードから組む構造の名前。
const (
	// textKey は、要素の文字列を置く名前である。ライブラリは属性と子要素と文字列を 1 つの
	// 順序付きの表に置き、文字列にこの名前を与える。
	textKey             = "Value"
	eventElement        = "Event"
	systemElement       = "System"
	eventDataElement    = "EventData"
	dataElement         = "Data"
	dataNameAttribute   = "Name"
	createdElement      = "TimeCreated"
	systemTimeAttribute = "SystemTime"
	systemTimeTicks     = "2006-01-02T15:04:05.0000000Z"
	systemTimeMicros    = "2006-01-02T15:04:05.000000Z"
)

// FILETIME は 1601-01-01 からの 100 ナノ秒の数である。
const (
	fileTimeTicksPerSecond   = 10_000_000
	fileTimeNanosPerTick     = 100
	fileTimeUnixEpochSeconds = 11_644_473_600
	// sameInstantTolerance は、SystemTime の秒とレコードの見出しの書き込み時刻を同じ時点と
	// みなす差の上限である。ライブラリは FILETIME を float64 に直してから秒へ割るため、
	// 2000 年代の時刻で 1 マイクロ秒程度の誤差を持つ。
	sameInstantTolerance = 2e-6
	microsPerSecond      = 1e6
	nanosPerMicro        = 1000
	// latestSystemTimeSeconds は 9999-12-31T23:59:59Z の Unix 秒である。4 桁の年で書ける
	// 最後の時点である。
	latestSystemTimeSeconds = 253_402_300_799
)

var errNoEventElement = errors.New("the record carries no <Event> element")

// renderEventXML は、ライブラリがレコード 1 件から組んだ構造を `<Event>` 要素の XML にする。
// writtenAt はレコードの見出しの書き込み時刻 (FILETIME) である。
//
// **ライブラリの構造は属性と子要素を区別しない。** 次の規則で書き分ける。
//   - System の子要素が持つ文字列と数の値は属性にする (`<Provider Name="..."/>`)。
//     文字列の名前 (textKey) の値は要素の文字列にする (`<EventID Qualifiers="0">4688</EventID>`)
//   - EventData の値は `<Data Name="...">` にする。ライブラリが `<Data>` を名前ごとの値へ
//     直せなかった EventData は、`<Data>` の Name だけを属性にする
//   - UserData など、そのほかの値はすべて子要素にする
//
// 名前空間の宣言は書かない (isNamespaceName)。`<Binary>` は、`<Data>` を名前ごとの値へ直した
// EventData からライブラリが除く。
func renderEventXML(parsed any, writtenAt uint64) (string, error) {
	root, ok := parsed.(*ordereddict.Dict)
	if !ok {
		return "", errNoEventElement
	}
	body, ok := root.Get(eventElement)
	event, isDict := body.(*ordereddict.Dict)
	if !ok || !isDict {
		return "", errNoEventElement
	}
	w := &xmlWriter{writtenAt: writtenAt}
	w.raw("<" + eventElement + ">")
	for _, key := range event.Keys() {
		value, _ := event.Get(key)
		switch key {
		case systemElement:
			w.system(value)
		case eventDataElement:
			w.eventData(value)
		default:
			w.element(key, value, false)
		}
	}
	w.raw("</" + eventElement + ">")
	if w.err != nil {
		return "", w.err
	}
	return w.String(), nil
}

// xmlWriter は XML を書く。writtenAt はレコードの見出しの書き込み時刻である。err は、
// 要素と属性の名前のうち XML の名前として読めない最初の名前の error である。
type xmlWriter struct {
	strings.Builder
	writtenAt uint64
	err       error
}

// name は要素または属性の名前を書く。XML の名前として読めない名前は書かず、error を残す。
//
// **名前を escape しない。** 名前に `"` や空白が入ると、属性の並びが変わった XML になり、
// 構文の誤りにならずに別の値として読める。
func (w *xmlWriter) name(text string) {
	if !isXMLName(text) {
		if w.err == nil {
			w.err = fmt.Errorf("the record names an element or attribute %s, which is not an XML name",
				strconv.Quote(text))
		}
		return
	}
	w.raw(text)
}

// isXMLName は、文字列が XML の名前 (NameStartChar に NameChar が続く文字列) であるかを返す。
// 名前に使える文字は文字、数字、`_`、`:`、`.`、`-` とし、先頭には文字、`_`、`:` だけを置ける。
func isXMLName(text string) bool {
	if text == "" {
		return false
	}
	for index, r := range text {
		switch {
		case unicode.IsLetter(r) || r == '_' || r == ':':
		case index > 0 && (unicode.IsDigit(r) || r == '.' || r == '-' || unicode.Is(unicode.Mn, r)):
		default:
			return false
		}
	}
	return true
}

func (w *xmlWriter) system(value any) {
	system, ok := value.(*ordereddict.Dict)
	if !ok {
		w.element(systemElement, value, false)
		return
	}
	w.raw("<" + systemElement + ">")
	for _, key := range system.Keys() {
		item, _ := system.Get(key)
		w.element(key, item, true)
	}
	w.raw("</" + systemElement + ">")
}

// eventData は EventData を書く。ライブラリが `<Data>` を名前ごとの値へ直した EventData は、
// 名前を持たない。直せなかった EventData は `Data` の名前を持つ。
func (w *xmlWriter) eventData(value any) {
	w.raw("<" + eventDataElement + ">")
	data, ok := value.(*ordereddict.Dict)
	_, unnormalized := dictGet(data, dataElement)
	switch {
	case !ok:
		w.content(value)
	case unnormalized:
		for _, key := range data.Keys() {
			item, _ := data.Get(key)
			w.element(key, item, key == dataElement)
		}
	default:
		for _, name := range data.Keys() {
			item, _ := data.Get(name)
			items, isList := listOf(item)
			if !isList {
				items = []any{item}
			}
			// 要素の無い列も、項目があったことを空の Data 1 つで残す。
			if len(items) == 0 {
				items = []any{""}
			}
			for _, one := range items {
				w.raw("<" + dataElement + " " + dataNameAttribute + `="`)
				w.escape(name)
				w.raw(`">`)
				w.content(one)
				w.raw("</" + dataElement + ">")
			}
		}
	}
	w.raw("</" + eventDataElement + ">")
}

// element は name の要素を書く。attributes が真なら、value の表の文字列と数の値を属性にする。
// value が列であれば、同じ名前の要素を値ごとに書く。
func (w *xmlWriter) element(name string, value any, attributes bool) {
	// 名前空間の宣言は要素の名前の文字列を決めるだけで、事象の値ではない。ライブラリは
	// `xmlns:` の付いた宣言を値として返すため、子要素にも属性にも書かない。
	if isNamespaceName(name) {
		return
	}
	if list, ok := listOf(value); ok {
		for _, item := range list {
			w.element(name, item, attributes)
		}
		return
	}
	w.raw("<")
	w.name(name)
	dict, isDict := value.(*ordereddict.Dict)
	if isDict && attributes {
		for _, key := range dict.Keys() {
			item, _ := dict.Get(key)
			if key != textKey && isScalar(item) && !isNamespaceName(key) {
				w.raw(" ")
				w.name(key)
				w.raw(`="`)
				w.escape(w.attributeText(name, key, item))
				w.raw(`"`)
			}
		}
	}
	w.raw(">")
	if isDict {
		for _, key := range dict.Keys() {
			item, _ := dict.Get(key)
			switch {
			case key == textKey && isScalar(item):
				w.escape(valueText(item))
			case attributes && isScalar(item):
			default:
				w.element(key, item, false)
			}
		}
	} else {
		w.escape(valueText(value))
	}
	w.raw("</" + name + ">")
}

// isNamespaceName は、名前が名前空間の宣言 (`xmlns` または `xmlns:` で始まる名前) であるかを返す。
func isNamespaceName(name string) bool {
	return name == "xmlns" || strings.HasPrefix(name, "xmlns:")
}

// content は要素の中身を書く。表は子要素に、列は値ごとの中身に、ほかの値は文字列にする。
func (w *xmlWriter) content(value any) {
	switch v := value.(type) {
	case *ordereddict.Dict:
		for _, key := range v.Keys() {
			item, _ := v.Get(key)
			if key == textKey && isScalar(item) {
				w.escape(valueText(item))
				continue
			}
			w.element(key, item, false)
		}
	case []any:
		for _, item := range v {
			w.content(item)
		}
	case []string:
		for _, item := range v {
			w.escape(item)
		}
	default:
		w.escape(valueText(value))
	}
}

// attributeText は属性の値の文字列を返す。TimeCreated の SystemTime は ISO 8601 の日時にする。
func (w *xmlWriter) attributeText(element, name string, value any) string {
	if seconds, ok := value.(float64); ok && element == createdElement && name == systemTimeAttribute {
		return systemTimeText(seconds, w.writtenAt)
	}
	return valueText(value)
}

// raw は文字列をそのまま書く。strings.Builder への書き込みは失敗しない。
func (w *xmlWriter) raw(text string) {
	_, _ = w.WriteString(text)
}

// escape は XML の文字列として書く。XML が持てない文字は U+FFFD に置き換わる。
func (w *xmlWriter) escape(text string) {
	// strings.Builder への書き込みは失敗しない。
	_ = xml.EscapeText(w, []byte(text))
}

// systemTimeText は SystemTime の秒を ISO 8601 の日時にする。
//
// ライブラリは FILETIME を float64 の秒に直して返す。レコードの見出しの書き込み時刻と同じ
// 時点であれば、見出しの FILETIME から 100 ナノ秒の単位まで書く。
//
// 既知の制限: 見出しの書き込み時刻と別の時点の SystemTime は、float64 の秒をマイクロ秒へ
// 丸めて書き、2 マイクロ秒までずれうる,
// ずれの大きさは float64 の秒の精度とマイクロ秒への丸めで決まる,
// ライブラリが FILETIME を整数のまま返すようになったとき、その値から書く
func systemTimeText(seconds float64, writtenAt uint64) string {
	// #nosec G115 -- uint64 の FILETIME を 1 秒の tick 数で割った値は 2^41 未満であり、int64 に収まる。
	unixSeconds := int64(writtenAt/fileTimeTicksPerSecond) - fileTimeUnixEpochSeconds
	ticks := int64(writtenAt % fileTimeTicksPerSecond) // #nosec G115 -- 1 秒の tick 数未満である。
	if math.Abs(float64(unixSeconds)+float64(ticks)/fileTimeTicksPerSecond-seconds) <= sameInstantTolerance {
		return time.Unix(unixSeconds, ticks*fileTimeNanosPerTick).UTC().Format(systemTimeTicks)
	}
	// FILETIME の起点より前と 4 桁の年の後ろ、NaN は数のまま書き、時刻の読み取りの失敗にする。
	if !(seconds >= -fileTimeUnixEpochSeconds && seconds <= latestSystemTimeSeconds) {
		return valueText(seconds)
	}
	whole := math.Floor(seconds)
	micros := math.Round((seconds - whole) * microsPerSecond)
	return time.Unix(int64(whole), int64(micros)*nanosPerMicro).UTC().Format(systemTimeMicros)
}

// libraryGUIDPattern は、ライブラリが GUID の型の値を書く文字列である。中括弧を持たない
// 大文字の 16 進である。
var libraryGUIDPattern = regexp.MustCompile(`^[0-9A-F]{8}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{12}$`)

// valueText はライブラリの値 1 つを文字列にする。byte 列は大文字の 16 進にする。
//
// GUID は Windows が XML に書き出す文字列と同じく、中括弧で囲んだ大文字の 16 進にする。
//
// 既知の制限: 中括弧を持たない大文字の GUID の文字列の文字列の値にも中括弧を足す,
// ライブラリは GUID の型の値を文字列に直して返し、文字列の型の値と区別できないため測れない,
// ライブラリが GUID の型を値に残すようになったとき、型で判定する
func valueText(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		if libraryGUIDPattern.MatchString(v) {
			return "{" + v + "}"
		}
		return v
	case []byte:
		return strings.ToUpper(hex.EncodeToString(v))
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	default:
		return fmt.Sprint(v)
	}
}

// listOf は列の値を要素ごとに返す。ライブラリは文字列の配列の型 (0x81) の値を []string で
// 返す。Windows は配列の値を要素ごとに 1 つの `<Data>` として書くため、同じく要素ごとに書く。
func listOf(value any) ([]any, bool) {
	switch v := value.(type) {
	case []any:
		return v, true
	case []string:
		list := make([]any, len(v))
		for i, item := range v {
			list[i] = item
		}
		return list, true
	default:
		return nil, false
	}
}

// isScalar は、値が表と列のどちらでもないかを返す。
func isScalar(value any) bool {
	switch value.(type) {
	case *ordereddict.Dict, []any, []string:
		return false
	default:
		return true
	}
}

// dictGet は表から名前の値を探す。表が nil のときは見つからない。
func dictGet(dict *ordereddict.Dict, key string) (any, bool) {
	if dict == nil {
		return nil, false
	}
	return dict.Get(key)
}
