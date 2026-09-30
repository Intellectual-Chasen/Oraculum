package apache

import "strings"

// errorTokenizeProblem は文字列の分割で確定した失敗の位置と、その場で期待していた形を持つ。
type errorTokenizeProblem struct {
	offset   int
	expected string
}

// tokenizeErrorLine はエラーログの 1 行を 8 項目へ分ける。
//
// 並びは [time] [module:severity] [pid P:tid T] [client host:port] message である。
// **未対応の並びを読み飛ばさない。** 4 つの角括弧のいずれかが無い行と、本文が空の行は、
// 文字列の分割の失敗として返す。
func tokenizeErrorLine(raw string) ([]ErrorItem, *errorTokenizeProblem) {
	timeText, timeStart, pos, problem := scanErrorBracket(raw, 0, "the time item")
	if problem != nil {
		return nil, problem
	}
	moduleSeverityText, moduleSeverityStart, pos, problem := scanErrorBracket(raw, pos, "the module and severity item")
	if problem != nil {
		return nil, problem
	}
	module, severity, problem := splitModuleSeverity(moduleSeverityText, moduleSeverityStart)
	if problem != nil {
		return nil, problem
	}
	pidTidText, pidTidStart, pos, problem := scanErrorBracket(raw, pos, "the pid and tid item")
	if problem != nil {
		return nil, problem
	}
	pid, tid, problem := splitPidTid(pidTidText, pidTidStart)
	if problem != nil {
		return nil, problem
	}
	clientText, clientStart, pos, problem := scanErrorBracket(raw, pos, "the client item")
	if problem != nil {
		return nil, problem
	}
	clientIP, clientPort, problem := splitClient(clientText, clientStart)
	if problem != nil {
		return nil, problem
	}
	pos = skipSpaces(raw, pos)
	if pos >= len(raw) {
		return nil, &errorTokenizeProblem{offset: pos, expected: "the message item"}
	}
	items := []ErrorItem{
		{name: ErrorItemTime, rawText: timeText, value: timeText, byteOffset: int64(timeStart) + 1},
		{name: ErrorItemModule, rawText: module.text, value: module.text, byteOffset: module.offset},
		{name: ErrorItemSeverity, rawText: severity.text, value: severity.text, byteOffset: severity.offset},
		{name: ErrorItemPid, rawText: pid.text, value: pid.text, byteOffset: pid.offset},
		{name: ErrorItemTid, rawText: tid.text, value: tid.text, byteOffset: tid.offset},
		{name: ErrorItemClientIP, rawText: clientIP.text, value: clientIP.text, byteOffset: clientIP.offset},
		{name: ErrorItemClientPort, rawText: clientPort.text, value: clientPort.text, byteOffset: clientPort.offset},
		{name: ErrorItemMessage, rawText: raw[pos:], value: raw[pos:], byteOffset: int64(pos)},
	}
	return items, nil
}

// scanErrorBracket は次の空白を読み飛ばし、raw[pos] が '[' であることを確かめて、対応する
// ']' の中の文字列と、その角括弧の開始位置を返す。角括弧の中に角括弧の入れ子は無い。
func scanErrorBracket(raw string, pos int, expected string) (value string, bracketStart, newPos int, problem *errorTokenizeProblem) {
	pos = skipSpaces(raw, pos)
	if pos >= len(raw) || raw[pos] != '[' {
		return "", pos, pos, &errorTokenizeProblem{offset: pos, expected: expected}
	}
	_, value, newPos, ok := scanBracketed(raw, pos)
	if !ok {
		return "", pos, pos, &errorTokenizeProblem{offset: pos, expected: "a closing ] for " + expected}
	}
	return value, pos, newPos, nil
}

type errorSubItem struct {
	text   string
	offset int64
}

// splitModuleSeverity は "module:severity" を 2 項目へ分ける。
func splitModuleSeverity(text string, bracketStart int) (module, severity errorSubItem, problem *errorTokenizeProblem) {
	before, after, found := strings.Cut(text, ":")
	if !found || before == "" || after == "" {
		return errorSubItem{}, errorSubItem{},
			&errorTokenizeProblem{offset: bracketStart, expected: "module:severity inside the brackets"}
	}
	return errorSubItem{text: before, offset: int64(bracketStart) + 1},
		errorSubItem{text: after, offset: int64(bracketStart) + 1 + int64(len(before)) + 1}, nil
}

// splitPidTid は "pid P:tid T" を 2 項目へ分ける。
func splitPidTid(text string, bracketStart int) (pid, tid errorSubItem, problem *errorTokenizeProblem) {
	const pidPrefix = "pid "
	const tidInfix = ":tid "
	fail := &errorTokenizeProblem{offset: bracketStart, expected: "pid <number>:tid <number> inside the brackets"}
	if !strings.HasPrefix(text, pidPrefix) {
		return errorSubItem{}, errorSubItem{}, fail
	}
	rest := text[len(pidPrefix):]
	pidText, tidText, found := strings.Cut(rest, tidInfix)
	if !found || pidText == "" || tidText == "" || !isDigits(pidText) || !isDigits(tidText) {
		return errorSubItem{}, errorSubItem{}, fail
	}
	pidOffset := int64(bracketStart) + 1 + int64(len(pidPrefix))
	tidOffset := pidOffset + int64(len(pidText)) + int64(len(tidInfix))
	return errorSubItem{text: pidText, offset: pidOffset}, errorSubItem{text: tidText, offset: tidOffset}, nil
}

// splitClient は "client host:port" を 2 項目へ分ける。port は末尾の ':' の後の十進数字と
// する (IPv6 の host に現れる ':' と区別するため)。
func splitClient(text string, bracketStart int) (host, port errorSubItem, problem *errorTokenizeProblem) {
	const clientPrefix = "client "
	fail := &errorTokenizeProblem{offset: bracketStart, expected: "client <host>:<port> inside the brackets"}
	if !strings.HasPrefix(text, clientPrefix) {
		return errorSubItem{}, errorSubItem{}, fail
	}
	rest := text[len(clientPrefix):]
	at := strings.LastIndexByte(rest, ':')
	if at < 0 || at == len(rest)-1 {
		return errorSubItem{}, errorSubItem{}, fail
	}
	hostText, portText := rest[:at], rest[at+1:]
	if hostText == "" || !isDigits(portText) {
		return errorSubItem{}, errorSubItem{}, fail
	}
	hostOffset := int64(bracketStart) + 1 + int64(len(clientPrefix))
	portOffset := hostOffset + int64(len(hostText)) + 1
	return errorSubItem{text: hostText, offset: hostOffset}, errorSubItem{text: portText, offset: portOffset}, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, b := range []byte(s) {
		if b < '0' || b > '9' {
			return false
		}
	}
	return true
}
