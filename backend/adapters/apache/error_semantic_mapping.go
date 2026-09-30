package apache

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// errorSemanticByItem はエラーログの欄と語彙の項目の対応である。
//
// module と severity と tid は Apache 自身の module 名と thread の識別子であり、この
// 入力形式に固有の意味を持つ。pid は語彙の項目 (process.pid) を持つが、値を導く元の
// process は Apache 自身の worker であり、EDR が記録する子 process の識別子とは別の
// process を指すため、突き合わせの条件には使わない。
var errorSemanticByItem = map[ErrorItemName]core.SemanticKey{
	ErrorItemTime:       core.SemanticKeyEventTime,
	ErrorItemPid:        core.SemanticKeyProcessPid,
	ErrorItemClientIP:   core.SemanticKeyConnectionSourceAddress,
	ErrorItemClientPort: core.SemanticKeyConnectionSourcePort,
	ErrorItemMessage:    core.SemanticKeyEventMessage,
}

// ErrorSemanticOfItem は 1 つの欄の語彙の項目を返す。対応表に無い欄には空の値を返す。
func ErrorSemanticOfItem(name ErrorItemName) core.SemanticKey {
	return errorSemanticByItem[name]
}
