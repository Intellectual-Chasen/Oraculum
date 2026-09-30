package royalts

import (
	"net"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// フィールド名。原資料の要素名をそのまま使う。
const (
	fieldID   = "id"
	fieldName = "name"
	fieldURI  = "uri"
	// fieldCredentialUsername は field の名前であり、値そのものではない。
	fieldCredentialUsername = "credentialUsername" // #nosec G101 -- field 名の定数であり秘密情報の値ではない
)

// ConnectionFields は 1 つの接続項目を core.RecordField の集合へ直す。
//
// id と name はこの入力形式に固有の意味を持つ (Royal TS 内部の識別子と表示名であり、
// 語彙に対応先が無い)。uri は文字列が IP アドレスかホスト名かで接続先の語彙の項目を選ぶ
// (RequestTargetHostSemantic と同じ判定)。
// credentialUsername は利用者の語彙の項目を持つ。
//
// **CredentialPassword の field を持たない。** Connection 型そのものが値を保持しない。
//
// error を返さない。名前は非空の定数、値の状態は present か item_absent のいずれかで
// あるため、core の constructor の検査が失敗する組み合わせが無い。
func ConnectionFields(connection Connection) []core.RecordField {
	fields := make([]core.RecordField, 0, 4)
	fields = append(fields, textFieldOf(fieldID, "", connection.id))
	fields = append(fields, textFieldOf(fieldName, "", connection.name))
	fields = append(fields, textFieldOf(fieldURI, uriSemantic(connection.uri), connection.uri))
	fields = append(fields,
		textFieldOf(fieldCredentialUsername, core.SemanticKeyAccountName, connection.credentialUsername))
	return fields
}

// uriSemantic は uri の文字列が IP アドレスかホスト名かで接続先の語彙の項目を選ぶ。
// 値が無い要素には空の値を返す。
func uriSemantic(uri *string) core.SemanticKey {
	if uri == nil {
		return ""
	}
	if net.ParseIP(*uri) != nil {
		return core.SemanticKeyConnectionDestinationAddress
	}
	return core.SemanticKeyConnectionDestinationHostname
}

func textFieldOf(name string, semantic core.SemanticKey, value *string) core.RecordField {
	var text core.RawAndNormalized
	if value == nil {
		text = core.NewAbsentItemValue()
	} else {
		text, _ = core.NewRawValue(core.ValueStatePresent, *value)
	}
	field, _ := core.NewTextField(name, semantic, text)
	return field
}
