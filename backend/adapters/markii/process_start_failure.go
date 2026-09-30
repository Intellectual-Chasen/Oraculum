package markii

// unresolvedReasonFieldMap は、文字列を項目へ対応付けられなかった原因を 4 分類のいずれにも
// 確定できない理由である。
//
// 対応付けが止まる形は 4 つある。key が無い、key が 2 回以上出る、値が空である、
// 値を期待した形として読めない、である。**4 つのどれであっても**、入力の不整合と、
// 対応していないバージョンの出力と、実装の不具合を分けられない。どの形であったかは observedResult が持つ。
const unresolvedReasonFieldMap = "the keys of the record do not map onto the items a process " +
	"start record needs. a malformed input, an unsupported output of another version, and a " +
	"defect of this parser are not told apart by the mapping alone"

// interpretationProcessStart は、プロセス開始の観測が何をどう読んだかである。
const interpretationProcessStart = "header read as a local date and time with the offset in the value, " +
	"values read as text without converting them to numbers or paths"

// processStartSemantics はプロセス開始の観測の失敗に載せる文面である。
//
// 2 つとも共有へ上げない。unresolvedReasonFieldMap は「a process start record」の語を
// 持ち、interpretationProcessStart は値を数値や path へ変換しないことを述べており、
// どちらもレコードの種別ごとに変わる。
var processStartSemantics = recordSemantics{
	interpretation: interpretationProcessStart,
	fieldMapReason: unresolvedReasonFieldMap,
}
