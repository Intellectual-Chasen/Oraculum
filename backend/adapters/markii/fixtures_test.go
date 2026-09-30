package markii_test

import (
	"fmt"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/markii"
)

// offsetProblemOf は field の位置がレコードの範囲に収まり、その範囲の原文が RawValue と
// 一致することを確かめる。問題が無ければ空文字列を返す。
//
// 位置の不変条件は fuzz と ORACULUM_MARKII_SOURCE_DIR の入力を読む検査が同じ形で確かめる。
func offsetProblemOf(record markii.Record) string {
	rawText := record.RawText()
	for _, field := range record.Fields() {
		if field.KeyByteOffset() < 0 || field.KeyByteOffset() > int64(len(rawText)) {
			return fmt.Sprintf("the key offset of %s is %d, outside the record of %d bytes",
				field.Key(), field.KeyByteOffset(), len(rawText))
		}
		end := field.ValueByteOffset() + field.ValueByteLength()
		if field.ValueByteOffset() < 0 || end > int64(len(rawText)) {
			return fmt.Sprintf("the value range of %s is [%d, %d), outside the record of %d bytes",
				field.Key(), field.ValueByteOffset(), end, len(rawText))
		}
		if got := rawText[field.ValueByteOffset():end]; got != field.RawValue() {
			return fmt.Sprintf("the text at the value offset of %s = %q, want %q",
				field.Key(), got, field.RawValue())
		}
	}
	return ""
}

// restoreOf は field 列からレコードの原文を組み直す。ヘッダーの 2 つの文字列のどちらかを
// レコードが持たないときは false を返す。
//
// 「組み直したら原文に戻る」は文字列の分割の不変条件であり、tokenize_test.go と
// fuzz_test.go と integration_test.go の 3 本が同じ組み直し方を使う。
func restoreOf(record markii.Record) (string, bool) {
	dateTime, hasDateTime := record.DateTimeText()
	zone, hasZone := record.ZoneText()
	if !hasDateTime || !hasZone {
		return "", false
	}
	var restored strings.Builder
	restored.WriteString(dateTime)
	restored.WriteByte(' ')
	restored.WriteString(zone)
	for _, field := range record.Fields() {
		restored.WriteByte(' ')
		restored.WriteString(field.Key())
		restored.WriteByte('=')
		restored.WriteString(field.RawValue())
	}
	return restored.String(), true
}

// 通信のレコードの文字列構造を持つ 1 行。
//
// key の並びと引用符の有無と、value に現れる文字の種類 (逆斜線を含む path、波括弧で
// 囲んだ GUID、読点で連ねた 2 つの IP、コロンで区切った MAC) を、通信のレコードの
// 文字列構造に合わせた。値は本 test が決めたものである。
const fullShapeLine = `02/01/2000 10:11:12.181 +0900 loc=ja-JP type=ITM2 sn=900100 lv=5 evt=net subEvt=est os=Win com="HOST01" domain="EXAMPLE" profile="example_profile" tmid=00000000-1111-2222-3333-444444444444 csid=S-1-5-21-1111111111-2222222222-3333333333 ip=192.0.2.10,fe80::1111:2222:3333:4444 mac=00:00:5e:00:53:01 sessionID=0 psGUID={00000000-1111-2222-3333-444444444444} psPath="C:\Tools\agent.exe" srcIP=192.0.2.10 srcPort=50417 dstIP=198.51.100.20 dstPort=443`

// fullShapeLineFieldCount は上の行の field の個数である。文字列を数えて固定した。
const fullShapeLineFieldCount = 21

// テスト用の行。文字列の分割が扱い分ける形を 1 行ずつ持たせた。
const (
	// 引用符 2 個のエスケープ。引用符で囲んだ value の中で引用符を 2 個並べる形を、
	// 1 つの value の中に 2 度と、value の末尾に 1 度持つ。
	escapedQuoteLine = `02/01/2000 10:12:13.450 +0900 evt=ps subEvt=start sn=900001 cmd="/c ""echo hello"" done" winTitle="say ""hi"""`

	// 素朴な正規表現が key として誤って拾う文字列。引用符付き value の内側に
	// group=all と enable=yes と SubSystemType=Windows の 3 つの字面を持つ。
	naiveGrepLine = `02/01/2000 13:11:00.000 +0900 evt=ps subEvt=start sn=900002 cmd="netsh advfirewall set group=all enable=yes SubSystemType=Windows" lv=5`

	// 欠測の 3 通り。値が空の欄と、値が "-" の欄と、key が無い欄を 1 行に持つ。
	// usrDomain は key が無い形で表す。
	absenceLine = `02/01/2000 13:12:00.000 +0900 evt=session subEvt=loginR sn=900003 evtDomain="" srcCom="-" srcIP="-"`

	// 値が 0 の欄。欄が無い状態と別の形で返すことを固定する。
	zeroValueLine = `02/01/2000 13:13:00.000 +0900 evt=net subEvt=dcon sn=900004 recv=0 send=1024`

	// key の並びが固定でない。下の 2 行は同じ 4 key を別の並びで持つ。
	orderLineA = `02/01/2000 13:14:00.000 +0900 evt=file subEvt=close sn=900005 lv=5`
	orderLineB = `02/01/2000 13:14:01.000 +0900 sn=900006 lv=5 evt=file subEvt=close`

	// 同じ key が 1 レコードに 2 回出る形。map の上書きで重複を消さないことを固定する。
	duplicateKeyLine = `02/01/2000 13:15:00.000 +0900 evt=ps subEvt=start sn=900007 lv=5 lv=7`

	// 同じ key が引用符付きの形と引用符無しの形の両方で現れる。
	quotedSrcIPLine   = `02/01/2000 13:16:00.000 +0900 evt=session subEvt=loginR sn=900008 srcIP="192.0.2.10"`
	unquotedSrcIPLine = `02/01/2000 13:16:01.000 +0900 evt=net subEvt=con sn=900009 srcIP=192.0.2.10`
)
