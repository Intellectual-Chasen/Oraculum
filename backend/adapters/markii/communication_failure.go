package markii

// unresolvedReasonCommunicationFieldMap は、文字列を項目へ対応付けられなかった原因を
// 4 分類のいずれにも確定できない理由である。
//
// 対応付けが止まる形は 4 つある。key が無い、key が 2 回以上出る、値が空である、
// 値を期待した形として読めない、である。**4 つのどれであっても**、入力の不整合と、
// 対応していないバージョンの出力と、実装の不具合を分けられない。どの形であったかは observedResult が持つ。
const unresolvedReasonCommunicationFieldMap = "the keys of the record do not map onto the " +
	"items a communication record needs. a malformed input, an unsupported output of another " +
	"version, and a defect of this parser are not told apart by the mapping alone"

// interpretationCommunication は、通信の観測が何をどう読んだかである。
//
// 接続の相手の 4 つを数値や address へ変換しない。port を整数へ直すと、原資料の文字列と
// 応答の値が 1 対 1 で対応しなくなる。
const interpretationCommunication = "header read as a local date and time with the offset in the value, " +
	"values read as text without converting them to numbers, addresses, or paths"

// communicationSemantics は通信の観測の失敗に載せる文面である。
//
// 2 つとも共有へ上げない。unresolvedReasonCommunicationFieldMap は
// 「a communication record」の語を持ち、interpretationCommunication は値を数値や
// address や path へ変換しないことを述べており、どちらもレコードの種別ごとに変わる。
var communicationSemantics = recordSemantics{
	interpretation: interpretationCommunication,
	fieldMapReason: unresolvedReasonCommunicationFieldMap,
}
