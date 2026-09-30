package winevent

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// NumberingNames は、レコードの番号の抜けと、形式の違う取り込みの突き合わせに使う欄の名前で
// ある。空の名前は、その形式のレコードが欄を持たないことを表す。
//
// **欄の名前で指し、語彙の項目に依らない。** 番号と番号を振った単位を読む欄を、本型が 1 か所で
// 指す。
type NumberingNames struct {
	// Channel と Computer は、EventRecordID を振った単位 (チャネルと端末) の欄である。
	Channel  string
	Computer string
	// EventRecordID は System の番号の欄である。
	EventRecordID string
	// RecordHeaderID は EVTX のレコードの見出しの番号の欄である。
	RecordHeaderID string
	// ProviderName と EventID はイベントを定義したプロバイダとイベント ID の欄である。
	ProviderName string
	EventID      string
	// HeaderNextRecordID と HeaderDirty は、収集元の識別の fileHeader の項目の名前である。
	HeaderNextRecordID string
	HeaderDirty        string
}

// NumberingNamesOf は形式ごとの NumberingNames を返す。イベントビューアーの CSV は番号と
// チャネルと端末の欄を持たない。XML は file の見出しとレコードの見出しを持たない。
func NumberingNamesOf(format core.FormatKey) NumberingNames {
	names := NumberingNames{ProviderName: nameProviderName, EventID: nameEventID}
	if format == FormatKeyViewerCSV {
		return names
	}
	names.Channel, names.Computer, names.EventRecordID = nameChannel, nameComputer, nameEventRecordID
	if format == FormatKeyEVTX {
		names.RecordHeaderID = nameRecordHeaderID
		names.HeaderNextRecordID, names.HeaderDirty = nameHeaderNextRecordID, nameHeaderDirty
	}
	return names
}
