package markii

import (
	"slices"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// ParserID は収集元 1 件を読む走査器を指す識別子である。イベント種別ごとの意味付けと
// ヘッダー時刻の取得を結合した走査器を指す。parserVersion の材料になる。
const ParserID = "markii-client-log"

// FormatKeyClientLog は markii 形式のクライアントログである。
const FormatKeyClientLog core.FormatKey = "infotrace_mark_ii"

// SupportedFormatVersion は本 package が読み取りを確認した出力のバージョンである。
const SupportedFormatVersion = "V3.0/V3.2"

// Formats は本 package が読める入力形式の宣言である。
//
// **どの形式を読めるかを知っているのは本 package だけである。** 取り込みの実行は
// 宣言を読んで収集元を振り分ける。レコードが欄の key を持つため、欄の並びの指定を取らない。
// 返した slice の変更は宣言に及ばない。
func Formats() []core.InputFormat {
	return []core.InputFormat{{
		Key: FormatKeyClientLog, ParserID: ParserID,
		PositionKind: core.PositionKindSequenceNumber,
		SpecInput:    core.FormatSpecInputRejected,
	}}
}

// 観測の種別の value の文字列。通信のイベントと、その中の接続の送信・切断・確立を表す。
const (
	// EventNetwork は通信のイベントを表す evt の value である。
	EventNetwork = communicationEvent
	// SubEventConnect は接続の送信の要求を表す subEvt の value である。
	SubEventConnect = communicationSubEventConnect
	// SubEventClose は接続の切断を表す subEvt の value である。
	SubEventClose = communicationSubEventClose
	// SubEventEstablish は接続の確立を表す subEvt の value である。
	SubEventEstablish = communicationSubEventEstablish
)

// ItemSemantics は本 package のレコードが持ちうる語彙の項目を返す。
//
// **写像表から導く。** レコードごとに出る key は異なるため、返すのは入力形式が持ちうる欄の
// 全体である。返した slice の変更は表に及ばない。
func ItemSemantics() []core.SemanticKey {
	semantics := make([]core.SemanticKey, 0, len(semanticByKey)+len(semanticByEventAndKey))
	seen := make(map[core.SemanticKey]struct{}, cap(semantics))
	add := func(semantic core.SemanticKey) {
		if semantic == "" {
			return
		}
		if _, duplicate := seen[semantic]; duplicate {
			return
		}
		seen[semantic] = struct{}{}
		semantics = append(semantics, semantic)
	}
	for _, semantic := range semanticByKey {
		add(semantic)
	}
	for _, semantic := range semanticByEventAndKey {
		add(semantic)
	}
	for _, semantic := range semanticByWindowsEventAndKey {
		add(semantic)
	}
	for _, semantic := range semanticBySubEventAndKey {
		add(semantic)
	}
	// map の走査の順は決まらないため、返す並びを文字列で 1 つに定める。
	slices.Sort(semantics)
	return semantics
}

// TranscriptIdentityItems は、同じ 1 つの事象を 2 回転記したレコードを見分ける欄の名前を
// 返す。
//
// クライアントログは Windows イベントログの 1 レコードを複数の行へ写す。`channel` と
// `evtRecID` が同じ 2 行は、Windows のイベント 1 件を 2 回書いたものである。
//
// **欄の名前を本 package が持つ。** 名前はクライアントログの key の文字列であり、core と
// pipeline はこの文字列を知らない。
func TranscriptIdentityItems() []string {
	return []string{keyEventChannel, keyEventRecordId}
}

// ObservationKindSelectorOf は evt と subEvt の value から、候補として数えるレコードを
// 選ぶ観測の種別 1 件を組む。欄の名前は TranscriptIdentityItems と同じく本 package が持つ。
func ObservationKindSelectorOf(event, subEvent string) core.ObservationKindSelector {
	return core.ObservationKindSelector{Items: []core.ObservationKindSelectorItem{
		{Name: keyEvent, Value: event},
		{Name: keySubEvent, Value: subEvent},
	}}
}

// ContentReplacementKinds は、ファイルまたはレジストリの値の内容を置き換える記録の観測の種別を
// 返す。
//
//   - file の del はファイルを消す。消した後の内容は、消す前の書き込みを含まない。
//   - reg の setVal は値を作るか変え、delVal は値を消す。どちらも値の全体を記録の後の値にする。
//   - file の close の new は、close したファイルをプロセスが作ったときに 1 になる。作った
//     ファイルは前の内容を持たない。
//
// new が 0 の close と new を持たない close は、書き込みが内容を置き換えたかを持たないため
// 選ばない。
func ContentReplacementKinds() []core.ContentReplacementSelector {
	return []core.ContentReplacementSelector{
		{Kind: ObservationKindSelectorOf(fileEvent, "del")},
		{Kind: ObservationKindSelectorOf(registryEvent, "setVal")},
		{Kind: ObservationKindSelectorOf(registryEvent, "delVal")},
		{
			Kind:   ObservationKindSelectorOf(fileEvent, "close"),
			Fields: []core.ObservationKindSelectorItem{{Name: "new", Value: "1"}},
		},
	}
}

// FlowOperationKinds は、影響のエッジの向きを決める操作の分類を、観測の種別ごとに返す。
//
//   - file の del、reg の setVal、delVal、delKey は書き込みである。
//   - file の rename は名前の変更である。
//   - net の acpt は接続の受け入れである。影響は接続から受け入れたプロセスへ渡る。
//   - net の con は接続を張った記録であり、送信と受信を分けない。
//   - net の dcon は接続を切った記録である。recv と send は接続を張ってから切るまでに受け取った
//     byte 数と送った byte 数であり、2 つを持つ dcon は、その量が向きを決める。量の欄を持たない
//     dcon だけが、送信と受信を分けない記録になる。
//
// file の close は開いてからの読み書きの量を記録し、向きは量の欄が決めるため含めない。
func FlowOperationKinds() []core.FlowOperationSelector {
	return []core.FlowOperationSelector{
		{Kind: ObservationKindSelectorOf(fileEvent, "del"), Operation: core.FlowOperationWrite},
		{Kind: ObservationKindSelectorOf(fileEvent, "rename"), Operation: core.FlowOperationRename},
		{Kind: ObservationKindSelectorOf(registryEvent, "setVal"), Operation: core.FlowOperationWrite},
		{Kind: ObservationKindSelectorOf(registryEvent, "delVal"), Operation: core.FlowOperationWrite},
		{Kind: ObservationKindSelectorOf(registryEvent, "delKey"), Operation: core.FlowOperationWrite},
		{Kind: ObservationKindSelectorOf(communicationEvent, "acpt"), Operation: core.FlowOperationRead},
		{Kind: ObservationKindSelectorOf(communicationEvent, "con"), Operation: core.FlowOperationCommunication},
		{Kind: ObservationKindSelectorOf(communicationEvent, "dcon"), Operation: core.FlowOperationCommunication},
	}
}
