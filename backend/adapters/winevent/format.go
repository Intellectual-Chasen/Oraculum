package winevent

import (
	"bytes"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// ParserIDXML は XML の走査器を指す識別子である。parserVersion の材料になる。
const ParserIDXML = "windows-event-xml"

// FormatKeyXML は Windows イベントログを XML に書き出した file である。`<Events>` の中に
// `<Event>` 要素が並ぶ。
const FormatKeyXML core.FormatKey = "windows_event_xml"

// ParserIDViewerCSV はイベントビューアーの CSV の走査器を指す識別子である。
const ParserIDViewerCSV = "windows-event-viewer-csv"

// FormatKeyViewerCSV は Windows のイベントビューアーが日本語の見出しで CSV に書き出した
// file である。見出しは 5 欄であり、データ行は見出しの無い 6 欄目に説明を持つ。
const FormatKeyViewerCSV core.FormatKey = "windows_event_viewer_csv"

// ParserIDEVTX は EVTX の走査器を指す識別子である。parserVersion の材料になる。
const ParserIDEVTX = "windows-event-evtx"

// FormatKeyEVTX は Windows イベントログの EVTX の file である。file の見出しの後ろに、
// レコードを並べた chunk が続く。
const FormatKeyEVTX core.FormatKey = "windows_evtx"

// HasEVTXSignature は、file の先頭の byte 列 head が EVTX の file の見出しの署名 `ElfFile\0` を
// 持つかを返す。
func HasEVTXSignature(head []byte) bool {
	return bytes.HasPrefix(head, []byte("ElfFile\x00"))
}

// Formats は本 package が読める入力形式の宣言である。
//
// **どの形式を読めるかを知っているのは本 package だけである。** 取り込みの実行は
// 宣言を読んで収集元を振り分ける。
//
// 位置の指し方は byte 範囲である。XML は 1 件が複数行にわたるため、行番号だけでは
// レコードの終わりを指せない。EVTX は行を持たない。
//
// **返した slice の変更は宣言に及ばない。**
func Formats() []core.InputFormat {
	return []core.InputFormat{{
		Key: FormatKeyXML, ParserID: ParserIDXML,
		PositionKind: core.PositionKindByteRange,
		SpecInput:    core.FormatSpecInputRejected,
	}, {
		Key: FormatKeyViewerCSV, ParserID: ParserIDViewerCSV,
		PositionKind: core.PositionKindByteRange,
		SpecInput:    core.FormatSpecInputRejected,
	}, {
		Key: FormatKeyEVTX, ParserID: ParserIDEVTX,
		PositionKind: core.PositionKindByteRange,
		SpecInput:    core.FormatSpecInputRejected,
	}}
}
