package markii

// unresolvedReasonRecordObservationFieldMap は、文字列を項目へ対応付けられなかった原因を
// 4 分類のいずれにも確定できない理由である。
//
// 対応付けが止まる形は 4 つある。key が無い、key が 2 回以上出る、値が空である、
// 値を期待した形として読めない、である。**4 つのどれであっても**、入力の不整合と、
// 対応していないバージョンの出力と、実装の不具合を分けられない。どの形であったかは observedResult が持つ。
const unresolvedReasonRecordObservationFieldMap = "the keys of the record do not map onto the " +
	"items any markii record needs. a malformed input, an unsupported output of another " +
	"version, and a defect of this parser are not told apart by the mapping alone"

// interpretationRecordObservation は、種別に依らない観測が何をどう読んだかである。
//
// evt と subEvt を除く値を数値や address や path へ変換しない。文字列を持つだけである。
const interpretationRecordObservation = "header read as a local date and time with the offset in the value, " +
	"values read as text without converting them to numbers, addresses, or paths"

// recordObservationSemantics は種別に依らない観測の失敗に載せる文面である。
//
// 2 つとも共有へ上げない。unresolvedReasonRecordObservationFieldMap は
// 「any markii record」の語を持ち、レコードの種別ごとに変わる。
var recordObservationSemantics = recordSemantics{
	interpretation: interpretationRecordObservation,
	fieldMapReason: unresolvedReasonRecordObservationFieldMap,
}
