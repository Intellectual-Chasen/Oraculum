package auditd

import (
	"slices"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 語彙の項目へ写す key の文字列。
const (
	keyProcessId       = "pid"
	keyParentProcessId = "ppid"
	keyExecutablePath  = "exe"
	keyPathName        = "name"
	keyLoginUserId     = "auid"
	keyProctitle       = "proctitle"
	keySession         = "ses"
)

// keyLoginUserName は auditd が auid から求めた表示用の値の key である。
// 0x1D の後ろに出る。
const keyLoginUserName = "AUID"

// semanticByLineAndKey は、行の種別と key の組を語彙の項目へ写す。
//
// **写すのは、2 つ以上の入力形式が同じ意味の値を持つ項目だけである。**
// `uid` / `cwd` / `inode` / `dev` / `cap_*` は auditd だけが持つため写さず、
// 原資料の key の文字列として name に残す。作業 directory を持つ入力形式は auditd の
// ほかに無く、比べる相手が無い。
//
// `ses` はログインのセッションの番号であり、Windows の Logon ID と同じくセッションを指す。
// USER_START は開いたセッション、USER_END は閉じたセッション、SYSCALL は操作を行った
// セッションの番号を書く。USER_START と USER_END の `ses` は、内側の `res` が success のときだけ
// 写す (sessionSucceeded)。
//
// **`auid` と `uid` を 1 つの実行主体へまとめない。** `auid` はログインした利用者、
// `uid` は実行時の利用者である。権限を昇格した後の操作では 2 つが異なる値を持ち、
// 1 つへまとめると昇格そのものが読めなくなる。
// 語彙の account.name へ写すのは、他の収集元の利用者名と文字列で比べられる
// `AUID` の表示用の値である。
var semanticByLineAndKey = map[lineItemKey]core.SemanticKey{
	{LineTypeSyscall, keyProcessId, false}:       core.SemanticKeyProcessPid,
	{LineTypeSyscall, keyParentProcessId, false}: core.SemanticKeyParentProcessPid,
	{LineTypeSyscall, keyExecutablePath, false}:  core.SemanticKeyProcessBinaryPath,
	{LineTypeSyscall, keyLoginUserName, true}:    core.SemanticKeyAccountName,
	{LineTypeSyscall, keySession, false}:         core.SemanticKeyEventSubjectLogonId,
	{lineTypeUserStart, keySession, false}:       core.SemanticKeyEventTargetLogonId,
	{lineTypeUserEnd, keySession, false}:         core.SemanticKeyEventLogoffLogonId,
}

// PAM がセッションを開いた記録と閉じた記録の行の種別。
const (
	lineTypeUserStart = "USER_START"
	lineTypeUserEnd   = "USER_END"
)

// sessionSucceeded は、行の内側の `res` が success であるかを返す。内側に `res` を持たない行は
// 偽である。
func sessionSucceeded(line Line) bool {
	for _, item := range line.Items() {
		for _, inner := range item.Inner() {
			if inner.Key() == "res" {
				return inner.Value() == "success"
			}
		}
	}
	return false
}

// lineItemKey は行の種別と key と、0x1D の前後の立場の組である。
type lineItemKey struct {
	lineType    string
	key         string
	interpreted bool
}

// semanticOfItem は行の種別と項目から語彙の項目を返す。
// 返り値が空文字列になるのは、語彙へ写さない項目である。
func semanticOfItem(lineType string, item Item) core.SemanticKey {
	return semanticByLineAndKey[lineItemKey{lineType, item.Key(), item.Interpreted()}]
}

// derivedSemantics は、原資料の 1 つの key から直接取り出せず、行の組から導く語彙の項目である。
var derivedSemantics = []core.SemanticKey{
	// EXECVE の引数、または PROCTITLE の復号値から組む。
	core.SemanticKeyProcessCommandLine,
	// PATH の name から取り出す。
	core.SemanticKeyFilePath,
}

// ItemSemantics は本 package のレコードが持ちうる語彙の項目を返す。
//
// **写像表と導出の一覧から組む。** key を 1 つ足す作業で本関数を手で直さずに済む。
//
// **返した slice の変更は表に及ばない。**
func ItemSemantics() []core.SemanticKey {
	semantics := make([]core.SemanticKey, 0, len(semanticByLineAndKey)+len(derivedSemantics))
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
	for _, semantic := range semanticByLineAndKey {
		add(semantic)
	}
	for _, semantic := range derivedSemantics {
		add(semantic)
	}
	// map の走査の順は決まらないため、返す並びを文字列で 1 つに定める。
	slices.Sort(semantics)
	return semantics
}
