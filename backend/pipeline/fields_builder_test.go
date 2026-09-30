// in-package test: 非公開の構築子で作った取り込み結果から fields を組んで検査する。
package pipeline

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 期待値は下記 5 つの収集元の原文から読んで決める。
//
// fieldsUnreadableClientIpSource は 1 行で、接続元 IP の欄が値の不在を表す文字列である。
// 突き合わせる値を読めない状態を作る。
//
// fieldsSquidSource は 3 行である。1 行目の接続元は 192.0.2.10 で、割当の期間の中に
// ある。2 行目の接続元 203.0.113.9 は割当を持たない。3 行目の接続元は 192.0.2.10 で、
// srv01 の割当の期間の後にある。
//
// fieldsSrv01Source は 4 行で、観測期間は 03:04:05.678 から 15:00:00.500 である。
// 1 行目と 4 行目は evt が file のレコードで、接続の 4 項目と recv と send を持たない。
// 2 行目は recv と send を持たない接続、3 行目は recv と send を持つ切断である。
//
// fieldsPc01Source は 192.0.2.10 を CLIENT01 に割り当て、期間が srv01 と重なる。
// fieldsPc02Source は 192.0.2.10 を CLIENT02 に割り当て、期間が srv01 と重ならない。
const (
	fieldsSquidSource = `192.0.2.10 - - [01/Feb/2000:13:55:00 +0900] "GET http://198.51.100.42/first HTTP/1.1" 200 4096 "-" "-" TCP_MEM_HIT:HIER_NONE` + "\n" +
		`203.0.113.9 - - [01/Feb/2000:13:55:01 +0900] "GET http://198.51.100.42/second HTTP/1.1" 200 12 "-" "-" TCP_MISS:DIRECT` + "\n" +
		`192.0.2.10 - - [01/Feb/2000:20:00:00 +0900] "GET http://198.51.100.42/third HTTP/1.1" 200 12 "-" "-" TCP_MISS:DIRECT` + "\n"

	fieldsSrv01Source = `02/01/2000 03:04:05.678 +0900 sn=1000 evt=file subEvt=close com="TESTHOST" ` +
		`tmid=00000000-1111-2222-3333-444444444444 ip=192.0.2.10,2001:db8::1` + "\n" +
		`02/01/2000 13:55:00.100 +0900 sn=1001 evt=net subEvt=con com="TESTHOST" ` +
		`tmid=00000000-1111-2222-3333-444444444444 csid=S-1-5-21-1111111111-2222222222-3333333333 ` +
		`psGUID={11111111-2222-3333-4444-555555555555} psPath="C:\Tools\agent.exe" ` +
		`ip=192.0.2.10,2001:db8::1 srcIP=192.0.2.10 srcPort=50002 dstIP=198.51.100.42 dstPort=80` + "\n" +
		`02/01/2000 14:00:00.000 +0900 sn=1002 evt=net subEvt=dcon com="TESTHOST" ` +
		`tmid=00000000-1111-2222-3333-444444444444 csid=S-1-5-21-1111111111-2222222222-3333333333 ` +
		`psGUID={11111111-2222-3333-4444-555555555555} psPath="C:\Tools\agent.exe" ` +
		`ip=192.0.2.10,2001:db8::1 srcIP=192.0.2.10 srcPort=50002 dstIP=198.51.100.42 dstPort=80 ` +
		`recv=0 send=1024` + "\n" +
		`02/01/2000 15:00:00.500 +0900 sn=1003 evt=file subEvt=close com="TESTHOST" ` +
		`tmid=00000000-1111-2222-3333-444444444444 ip=192.0.2.10,2001:db8::1` + "\n"

	fieldsPc01Source = `02/01/2000 13:50:00.000 +0900 sn=101 evt=file subEvt=close com="CLIENT01" tmid=pc01 ip=192.0.2.10` + "\n" +
		`02/01/2000 14:10:00.000 +0900 sn=102 evt=file subEvt=close com="CLIENT01" tmid=pc01 ip=192.0.2.10` + "\n"

	fieldsPc02Source = `02/01/2000 19:00:00.000 +0900 sn=201 evt=file subEvt=close com="CLIENT02" tmid=pc02 ip=192.0.2.10` + "\n" +
		`02/01/2000 21:00:00.000 +0900 sn=202 evt=file subEvt=close com="CLIENT02" tmid=pc02 ip=192.0.2.10` + "\n"

	fieldsUnreadableClientIpSource = `- - - [01/Feb/2000:13:55:00 +0900] "GET http://198.51.100.42/first HTTP/1.1" 200 12 "-" "-" TCP_MISS:DIRECT` + "\n"
)

// Squid のレコードを指した `/api/v0/records` の応答が返す name を、応答の順で写す。
// combined の欄と、同じレコードの %ru から導く項目と、段階 2 が補う項目である。
var squidResponseFieldNames = []string{
	"clientIp", "ident", "user", "requestTime", "requestLine", "statusCode", "replyBytes",
	"referer", "userAgent", "squidStatus", "requestMethod", "requestTargetHost",
	"requestTargetPort", "requestTarget", "requestVersion", "clientTerminal", "clientTerminalName",
	"clientPort", "process",
}

// markii 形式のレコードを指した `/api/v0/records` の応答が返す name を、応答の順で写す。
// ヘッダーの時刻と、fieldsSrv01Source の sn=1001 が書いた key と、当該レコードに出ない recv と send である。
//
// **段階 2 の補いが 1 件も無い。** 接続元と実行主体の語彙の項目をレコード自身が持つ。
var markiiResponseFieldNames = []string{
	"headerTime", "sn", "evt", "subEvt", "com", "tmid", "csid", "psGUID", "psPath", "ip",
	"srcIP", "srcPort", "dstIP", "dstPort", "recv", "send",
}

type fieldsSourceInput struct {
	id       string
	fileName string
	format   core.FormatKey
	parser   SourceParser
	content  string
}

func squidInput(id, content string) fieldsSourceInput {
	return fieldsSourceInput{
		id: id, fileName: id + ".log", format: SquidFormatKey,
		parser: NewTestSquidParser(), content: content,
	}
}

func markiiInput(id, content string) fieldsSourceInput {
	return fieldsSourceInput{
		id: id, fileName: id + ".log", format: MarkIIFormatKey,
		parser: NewTestMarkIIParser(), content: content,
	}
}

// fieldsResult は収集元を取り込み、識別も含めた取り込み結果を返す。
func fieldsResult(t *testing.T, inputs ...fieldsSourceInput) ImportResult {
	t.Helper()
	sources := make([]scannedSource, len(inputs))
	statuses := make([]core.ImportStatus, len(inputs))
	for i, input := range inputs {
		sources[i] = scanIndexSource(t, input.parser, input.fileName, input.format, input.content)
		statuses[i] = settleStatus(t, sources[i], input.id)
	}
	result, err := newImportResult(sources, statuses, "run", indexRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	result.identities = make(map[string]core.SourceIdentity, len(inputs))
	for i, input := range inputs {
		result.identities[input.id] = sourceIdentity(sources[i], input.id)
	}
	return result
}

// builtFields は収集元と位置で指したレコードから応答の fields を組む。
func builtFields(
	t *testing.T, result ImportResult, sourceId string, kind core.PositionKind, position int64,
) []core.RecordField {
	t.Helper()
	index := NewCandidateIndex(result)
	entry, found := index.RecordAt(sourceId, kind, position)
	if !found {
		t.Fatalf("the record at %s %d of %q is missing", kind, position, sourceId)
	}
	fields, err := NewFieldsBuilder(result).Build(entry)
	if err != nil {
		t.Fatalf("building the response fields: %v", err)
	}
	return fields
}

func fieldNames(fields []core.RecordField) []string {
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, field.Name)
	}
	return names
}

func namedField(t *testing.T, fields []core.RecordField, name string) core.RecordField {
	t.Helper()
	for _, field := range fields {
		if field.Name == name {
			return field
		}
	}
	t.Fatalf("the fields %v carry no item named %q", fieldNames(fields), name)
	return core.RecordField{}
}

func requireFieldNames(t *testing.T, fields []core.RecordField, want []string) {
	t.Helper()
	got := fieldNames(fields)
	if len(got) != len(want) {
		t.Fatalf("the fields carry %d items %v, want %d items %v",
			len(got), got, len(want), want)
	}
	for index, name := range want {
		if got[index] != name {
			t.Errorf("the item at %d is named %q, want %q", index, got[index], name)
		}
	}
}

// Squid のレコードの fields の name の並びは squidResponseFieldNames と一致する。
func TestBuildReturnsTheSquidFieldsInOrder(t *testing.T) {
	result := fieldsResult(t, squidInput("squid", fieldsSquidSource),
		markiiInput("srv01", fieldsSrv01Source))

	fields := builtFields(t, result, "squid", core.PositionKindLineNumber, 1)

	requireFieldNames(t, fields, squidResponseFieldNames)
}

// markii 形式のレコードの fields の name の並びは markiiResponseFieldNames と一致する。
func TestBuildReturnsTheMarkIIFieldsInOrder(t *testing.T) {
	result := fieldsResult(t, markiiInput("srv01", fieldsSrv01Source))

	fields := builtFields(t, result, "srv01", core.PositionKindSequenceNumber, 1001)

	requireFieldNames(t, fields, markiiResponseFieldNames)
}

// 段階 1 が意味を付けた項目が応答に出る。
//
// name の集合を入力形式ごとに固定していた間、この 11 件は応答から消えていた。
// **evt が file のレコードの file.sha256 と file.path は、段階 1 が写した意味の届き先が
// 応答であることを示す。**
func TestBuildCarriesTheItemsStageOneGaveASemantic(t *testing.T) {
	// evt が file のレコード。対象 file の path と sha256 を持つ。
	const fileEventSource = `02/01/2000 13:20:00.000 +0900 sn=4000 evt=file subEvt=close ` +
		`com="TESTHOST" tmid=` + srv01TerminalId + ` ip=192.0.2.10 ` +
		`psGUID={22222222-3333-4444-5555-666666666666} path="C:\data\sample.txt" ` +
		`sha256=2e5ac0d1c4f0dbb7b3c1a2e5d6f708192a3b4c5d6e7f80912a3b4c5d6e7f8091` + "\n"
	result := fieldsResult(t, squidInput("squid", fieldsSquidSource),
		markiiInput("srv01", fieldsSrv01Source), markiiInput("file", fileEventSource))
	cases := map[string]struct {
		fields   []core.RecordField
		semantic map[string]core.SemanticKey
	}{
		"Squid": {
			fields: builtFields(t, result, "squid", core.PositionKindLineNumber, 1),
			semantic: map[string]core.SemanticKey{
				"requestTargetPort": core.SemanticKeyConnectionDestinationPort,
				// combined に固有の意味を持つ 3 欄は語彙の項目を持たないまま応答に出る。
				"ident":       "",
				"requestLine": "",
				"squidStatus": "",
			},
		},
		"markii 形式の evt が net": {
			fields: builtFields(t, result, "srv01", core.PositionKindSequenceNumber, 1001),
			semantic: map[string]core.SemanticKey{
				"tmid":   core.SemanticKeyTerminalId,
				"com":    core.SemanticKeyTerminalHostname,
				"csid":   core.SemanticKeyTerminalSecurityId,
				"psPath": core.SemanticKeyProcessBinaryPath,
			},
		},
		"markii 形式の evt が file": {
			fields: builtFields(t, result, "file", core.PositionKindSequenceNumber, 4000),
			semantic: map[string]core.SemanticKey{
				"path":   core.SemanticKeyFilePath,
				"sha256": core.SemanticKeyFileSha256,
			},
		},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			for item, semantic := range want.semantic {
				field := namedField(t, want.fields, item)
				if field.Semantic != semantic {
					t.Errorf("%s semantic = %q, want %q", item, field.Semantic, semantic)
				}
			}
		})
	}
	// 対象 file の sha256 の文字列が応答に出る。
	fileFields := builtFields(t, result, "file", core.PositionKindSequenceNumber, 4000)
	sha256 := namedField(t, fileFields, "sha256")
	const wantSha256 = "2e5ac0d1c4f0dbb7b3c1a2e5d6f708192a3b4c5d6e7f80912a3b4c5d6e7f8091"
	if raw, ok := sha256.Text.RawTextValue(); !ok || raw != wantSha256 {
		t.Errorf("sha256 rawText = %q (present %t), want %q", raw, ok, wantSha256)
	}
	// 反対側。接続を記録しないレコードには段階 2 の補いが 1 件も出ない。
	for _, item := range []string{clientTerminalFieldName, clientTerminalNameFieldName,
		clientIpFieldName, clientPortFieldName, processFieldName} {
		for _, got := range fieldNames(fileFields) {
			if got == item {
				t.Errorf("the fields of the file record carry the supplement %q", item)
			}
		}
	}
}

// 段階 2 は、接続元と実行主体の項目をレコードが持つときに補わない。
//
// Squid のレコードは端末とプロセスの欄を持たないため 4 件を補い、markii 形式の接続の
// レコードは 4 つとも自分で持つため 1 件も補わない。
func TestBuildSupplementsOnlyTheItemsTheRecordDoesNotCarry(t *testing.T) {
	result := fieldsResult(t, squidInput("squid", fieldsSquidSource),
		markiiInput("srv01", fieldsSrv01Source))
	supplements := []string{clientTerminalFieldName, clientTerminalNameFieldName,
		clientPortFieldName, processFieldName}

	squidFields := builtFields(t, result, "squid", core.PositionKindLineNumber, 1)
	for _, item := range supplements {
		namedField(t, squidFields, item)
	}
	// 接続元 IP は combined の欄が持つため、補った項目にならない。
	clientIp := namedField(t, squidFields, clientIpFieldName)
	if clientIp.Text.ValueState != core.ValueStatePresent {
		t.Errorf("clientIp valueState = %q, want %q",
			clientIp.Text.ValueState, core.ValueStatePresent)
	}

	// 反対側。tmid と com と srcPort と psGUID を持つレコードは 4 件とも補われない。
	markiiFields := builtFields(t, result, "srv01", core.PositionKindSequenceNumber, 1001)
	names := fieldNames(markiiFields)
	for _, item := range append(supplements, clientIpFieldName) {
		for _, got := range names {
			if got == item {
				t.Errorf("the fields %v carry the supplement %q", names, item)
			}
		}
	}
}

// 入力形式に欄そのものが無い 2 件は item_absent で、文字列を 1 つも持たない。
func TestBuildAddsTheTwoItemsSquidCombinedHasNoColumnFor(t *testing.T) {
	result := fieldsResult(t, squidInput("squid", fieldsSquidSource),
		markiiInput("srv01", fieldsSrv01Source))

	fields := builtFields(t, result, "squid", core.PositionKindLineNumber, 1)

	for _, name := range []string{"clientPort", "process"} {
		field := namedField(t, fields, name)
		if field.Text == nil {
			t.Fatalf("%s must carry a text value", name)
		}
		if field.Text.ValueState != core.ValueStateItemAbsent {
			t.Errorf("%s valueState = %q, want %q", name, field.Text.ValueState,
				core.ValueStateItemAbsent)
		}
		if raw, ok := field.Text.RawTextValue(); ok {
			t.Errorf("%s carries the raw text %q", name, raw)
		}
		if normalized, ok := field.Text.NormalizedValue(); ok {
			t.Errorf("%s carries the normalized value %q", name, normalized)
		}
		if derivation, ok := field.Text.DerivationValue(); ok {
			t.Errorf("%s carries the derivation %q", name, derivation)
		}
	}
}

// 値の不在を表す文字列を持つ 3 欄は、外側の引用符を含む原資料の byte 列を持つ。
func TestBuildKeepsTheQuotesOfTheAbsentMarkers(t *testing.T) {
	result := fieldsResult(t, squidInput("squid", fieldsSquidSource),
		markiiInput("srv01", fieldsSrv01Source))

	fields := builtFields(t, result, "squid", core.PositionKindLineNumber, 1)

	cases := map[string]string{"referer": `"-"`, "userAgent": `"-"`, "user": "-"}
	for name, want := range cases {
		field := namedField(t, fields, name)
		raw, ok := field.Text.RawTextValue()
		if !ok || raw != want {
			t.Errorf("%s rawText = %q (present %t), want %q", name, raw, ok, want)
		}
		if len(raw) != len(want) {
			t.Errorf("%s rawText is %d byte, want %d byte", name, len(raw), len(want))
		}
		if field.Text.ValueState != core.ValueStateAbsent {
			t.Errorf("%s valueState = %q, want %q", name, field.Text.ValueState,
				core.ValueStateAbsent)
		}
	}
}

// recv と send は、値を持つレコードで present、key が出ないレコードで item_absent である。
func TestBuildCarriesTheByteCountsOnBothSides(t *testing.T) {
	result := fieldsResult(t, markiiInput("srv01", fieldsSrv01Source))
	cases := map[string]struct {
		sequenceNumber int64
		present        map[string]string
		itemAbsent     []string
	}{
		"接続": {
			sequenceNumber: 1001,
			itemAbsent:     []string{"recv", "send"},
		},
		"切断": {
			sequenceNumber: 1002,
			present:        map[string]string{"recv": "0", "send": "1024"},
		},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			fields := builtFields(t, result, "srv01", core.PositionKindSequenceNumber,
				want.sequenceNumber)
			requireFieldNames(t, fields, markiiResponseFieldNames)
			for key, rawText := range want.present {
				field := namedField(t, fields, key)
				if field.Text.ValueState != core.ValueStatePresent {
					t.Errorf("%s valueState = %q, want %q", key, field.Text.ValueState,
						core.ValueStatePresent)
				}
				if raw, ok := field.Text.RawTextValue(); !ok || raw != rawText {
					t.Errorf("%s rawText = %q (present %t), want %q", key, raw, ok, rawText)
				}
			}
			for _, key := range want.itemAbsent {
				field := namedField(t, fields, key)
				if field.Text.ValueState != core.ValueStateItemAbsent {
					t.Errorf("%s valueState = %q, want %q", key, field.Text.ValueState,
						core.ValueStateItemAbsent)
				}
				if raw, ok := field.Text.RawTextValue(); ok {
					t.Errorf("%s carries the raw text %q", key, raw)
				}
			}
		})
	}
}

// requireDerivationItems は derivation が 4 つの値を持つことを 1 つずつ確かめる。
func requireDerivationItems(t *testing.T, derivation string, want ...string) {
	t.Helper()
	for _, item := range want {
		if !strings.Contains(derivation, item) {
			t.Errorf("the derivation %q carries no %q", derivation, item)
		}
	}
}

// srv01TerminalId は fieldsSrv01Source の tmid である。**com の TESTHOST と別の値である。**
const srv01TerminalId = "00000000-1111-2222-3333-444444444444"

// 割当を 1 件に絞れた接続元は derived で、derivation が導出をたどる 4 つを持つ。
//
// clientTerminal が端末の外部識別子、clientTerminalName が表示名を持つ。**2 項目は
// 別の値になる。** 同じ値であれば、両者を取り違えても検査が通る。
func TestBuildDerivesTheClientTerminalFromTheAssignment(t *testing.T) {
	result := fieldsResult(t, squidInput("squid", fieldsSquidSource),
		markiiInput("srv01", fieldsSrv01Source))

	fields := builtFields(t, result, "squid", core.PositionKindLineNumber, 1)

	if srv01TerminalId == "TESTHOST" {
		t.Fatal("the identifier and the hostname must differ for this check to read anything")
	}
	for _, want := range []struct {
		name       string
		semantic   core.SemanticKey
		normalized string
	}{
		{"clientTerminal", core.SemanticKeyTerminalId, srv01TerminalId},
		{"clientTerminalName", core.SemanticKeyTerminalHostname, "TESTHOST"},
	} {
		terminal := namedField(t, fields, want.name)
		if terminal.Semantic != want.semantic {
			t.Errorf("%s semantic = %q, want %q", want.name, terminal.Semantic, want.semantic)
		}
		if terminal.Text.ValueState != core.ValueStateDerived {
			t.Fatalf("%s valueState = %q, want %q", want.name, terminal.Text.ValueState,
				core.ValueStateDerived)
		}
		normalized, ok := terminal.Text.NormalizedValue()
		if !ok || normalized != want.normalized {
			t.Errorf("%s normalized = %q (present %t), want %q",
				want.name, normalized, ok, want.normalized)
		}
		if raw, ok := terminal.Text.RawTextValue(); ok {
			t.Errorf("%s carries the raw text %q on a derived value", want.name, raw)
		}
		derivation, ok := terminal.Text.DerivationValue()
		if !ok {
			t.Fatal("a derived value must carry how it was derived")
		}
		requireDerivationItems(t, derivation,
			"clientIp=192.0.2.10",
			"sourceId=srv01",
			"rule=terminal_ip_assignment",
			"assignmentValidRange=2000-02-01T03:04:05.678+09:00/2000-02-01T15:00:00.500+09:00",
		)
	}
}

// 利用者が入力した割当で決まった端末は、導出の説明に割当の由来を書く。省いた項目は導出
// できなかった値として返す。
func TestBuildNamesTheOriginOfAUserSuppliedAssignment(t *testing.T) {
	result := fieldsResult(t, squidInput("squid", fieldsSquidSource))
	identity, _ := result.Identity("squid")
	specified, err := importSpecifiedAssignment(
		SourceTerminal{TerminalId: "T-user", Ip: "192.0.2.10"}, identity, nil)
	if err != nil {
		t.Fatal(err)
	}
	analyst := specified
	analyst.Origin = core.TerminalAssignmentOriginAnalystSupplied
	analyst.Derivation, analyst.Author = "別の端末の記録から導いた", "analyst-a"
	analyst.BasisRecordRefs = []core.AssertionRecordRef{
		core.NewAssertionRecordRef(result.publications[0].records[0].Locator),
	}

	for name, want := range map[string]struct {
		imported, recorded []core.TerminalAssignment
		items              []string
	}{
		"取り込みの指定": {imported: []core.TerminalAssignment{specified},
			items: []string{"origin=import_specified"}},
		"画面からの記録": {recorded: []core.TerminalAssignment{analyst},
			items: []string{"origin=analyst_supplied", "author=analyst-a"}},
	} {
		t.Run(name, func(t *testing.T) {
			scoped := result.WithAnalystTerminalAssignments(want.recorded)
			scoped.importAssignments = want.imported
			fields := builtFields(t, scoped, "squid", core.PositionKindLineNumber, 1)

			terminal := namedField(t, fields, "clientTerminal")
			normalized, _ := terminal.Text.NormalizedValue()
			if terminal.Text.ValueState != core.ValueStateDerived || normalized != "T-user" {
				t.Fatalf("clientTerminal = %q (%q), want the derived T-user",
					normalized, terminal.Text.ValueState)
			}
			derivation, _ := terminal.Text.DerivationValue()
			requireDerivationItems(t, derivation,
				append([]string{"sourceId=squid", "clientIp=192.0.2.10"}, want.items...)...)

			hostname := namedField(t, fields, "clientTerminalName")
			if hostname.Text.ValueState != core.ValueStateDerivationUndetermined {
				t.Fatalf("clientTerminalName valueState = %q, want %q",
					hostname.Text.ValueState, core.ValueStateDerivationUndetermined)
			}
			reason, _ := hostname.Text.DerivationValue()
			requireDerivationItems(t, reason, "reason=該当の割当はこの項目を持たない")
		})
	}
}

// 同じ表示名を持つ 2 台の端末を 1 件の割当へまとめない。
//
// 2 台は tmid が異なり com と ip が同じである。表示名をまとめる鍵に入れると 1 件になり、
// 別の端末の通信が 1 台のものとして確定する。
func TestTerminalAssignmentsKeepTwoTerminalsSharingAHostname(t *testing.T) {
	const sharedHostnameSource = `02/01/2000 13:50:00.000 +0900 sn=301 evt=file subEvt=close ` +
		`com="TESTHOST" tmid=` + srv01TerminalId + ` ip=192.0.2.10` + "\n" +
		`02/01/2000 14:10:00.000 +0900 sn=302 evt=file subEvt=close ` +
		`com="TESTHOST" tmid=99999999-8888-7777-6666-555555555555 ip=192.0.2.10` + "\n"
	result := fieldsResult(t, squidInput("squid", fieldsSquidSource),
		markiiInput("twins", sharedHostnameSource))

	assignments := terminalAssignmentsOf(result).entries

	if len(assignments) != 2 {
		t.Fatalf("the result carries %d assignments, want 2: %+v", len(assignments), assignments)
	}
	for index, want := range []string{srv01TerminalId, "99999999-8888-7777-6666-555555555555"} {
		if assignments[index].TerminalId != want {
			t.Errorf("the assignment at %d carries the identifier %q, want %q",
				index, assignments[index].TerminalId, want)
		}
		if assignments[index].TerminalHostname != "TESTHOST" {
			t.Errorf("the assignment at %d carries the hostname %q, want TESTHOST",
				index, assignments[index].TerminalHostname)
		}
	}
	// 2 件残るため、同じ接続元 IP の起点は端末を 1 つに確定できない。
	fields := builtFields(t, result, "squid", core.PositionKindLineNumber, 1)
	for _, name := range []string{"clientTerminal", "clientTerminalName"} {
		terminal := namedField(t, fields, name)
		if terminal.Text.ValueState != core.ValueStateDerivationUndetermined {
			t.Fatalf("%s valueState = %q, want %q", name, terminal.Text.ValueState,
				core.ValueStateDerivationUndetermined)
		}
		derivation, _ := terminal.Text.DerivationValue()
		requireDerivationItems(t, derivation, "reason=該当が複数件", "memberCount=2")
		// 段階 3 の残りが 2 件である。表示名の数を持つ項目は出ない。
		if strings.Contains(derivation, "hostnameCount=") {
			t.Errorf("the derivation %q carries hostnameCount=", derivation)
		}
	}
}

// 同じ端末に別の表示名が付いた原資料では、表示名を 1 つに確定させない。
//
// まとめる鍵 (sourceId, clientIp, terminal.id) に表示名を入れないため、まとめた割当が持つ
// 表示名は先に走査した 1 つになる。**それを derived で返すと、原資料に別の表示名が
// あることが応答から消える。** 識別子は一意に定まっているため clientTerminal は
// derived のままにし、clientTerminalName だけを derivation_undetermined にする。
func TestBuildLeavesTheTerminalNameUndeterminedWhenTheHostnamesDiffer(t *testing.T) {
	// 2 行は tmid と ip が同じで com だけが違う。srv01 の観測期間に収まる時刻を持つ。
	const conflictingSource = `02/01/2000 13:50:00.000 +0900 sn=401 evt=file subEvt=close ` +
		`com="TESTHOST" tmid=` + srv01TerminalId + ` ip=192.0.2.10` + "\n" +
		`02/01/2000 14:10:00.000 +0900 sn=402 evt=file subEvt=close ` +
		`com="TESTHOST-RENAMED" tmid=` + srv01TerminalId + ` ip=192.0.2.10` + "\n"
	// 反対側。同じ tmid に同じ com が付く 2 行である。
	const agreeingSource = `02/01/2000 13:50:00.000 +0900 sn=401 evt=file subEvt=close ` +
		`com="TESTHOST" tmid=` + srv01TerminalId + ` ip=192.0.2.10` + "\n" +
		`02/01/2000 14:10:00.000 +0900 sn=402 evt=file subEvt=close ` +
		`com="TESTHOST" tmid=` + srv01TerminalId + ` ip=192.0.2.10` + "\n"
	cases := map[string]struct {
		source string
		// nameState は clientTerminalName の値の状態である。
		nameState core.ValueState
		// normalized は nameState が derived のときの正規化値である。
		normalized string
		// hostnameCount はまとめる鍵に対して観測した表示名の個数である。
		hostnameCount int
	}{
		"同じ tmid に同じ com": {
			source: agreeingSource, nameState: core.ValueStateDerived,
			normalized: "TESTHOST", hostnameCount: 1,
		},
		"同じ tmid に別の com": {
			source: conflictingSource, nameState: core.ValueStateDerivationUndetermined,
			hostnameCount: 2,
		},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			result := fieldsResult(t, squidInput("squid", fieldsSquidSource),
				markiiInput("srv01", want.source))

			assignments := terminalAssignmentsOf(result)
			// 割当は IP 1 つにつき 1 件へまとめられる。
			if len(assignments.entries) != 1 {
				t.Fatalf("the result carries %d assignments, want 1: %+v",
					len(assignments.entries), assignments.entries)
			}
			observed := assignments.hostnamesOf(assignments.entries[0])
			if len(observed) != want.hostnameCount {
				t.Fatalf("the key carries the hostnames %v, want %d of them",
					observed, want.hostnameCount)
			}

			fields := builtFields(t, result, "squid", core.PositionKindLineNumber, 1)
			// 識別子は一意に定まるため、どちらの原資料でも derived である。
			terminal := namedField(t, fields, "clientTerminal")
			if terminal.Text.ValueState != core.ValueStateDerived {
				t.Fatalf("clientTerminal valueState = %q, want %q",
					terminal.Text.ValueState, core.ValueStateDerived)
			}
			if normalized, _ := terminal.Text.NormalizedValue(); normalized != srv01TerminalId {
				t.Errorf("clientTerminal normalized = %q, want %q", normalized, srv01TerminalId)
			}

			terminalName := namedField(t, fields, "clientTerminalName")
			if terminalName.Text.ValueState != want.nameState {
				t.Fatalf("clientTerminalName valueState = %q, want %q",
					terminalName.Text.ValueState, want.nameState)
			}
			normalized, hasNormalized := terminalName.Text.NormalizedValue()
			if want.nameState == core.ValueStateDerived {
				if !hasNormalized || normalized != want.normalized {
					t.Errorf("clientTerminalName normalized = %q (present %t), want %q",
						normalized, hasNormalized, want.normalized)
				}
				return
			}
			// 導けなかった表示名は正規化値を持たず、理由と表示名の数を derivation が持つ。
			if hasNormalized {
				t.Errorf("clientTerminalName carries the normalized value %q", normalized)
			}
			derivation, ok := terminalName.Text.DerivationValue()
			if !ok {
				t.Fatal("an undetermined value must carry why it was not derived")
			}
			requireDerivationItems(t, derivation, "reason=表示名が複数件", "hostnameCount=2",
				"clientIp=192.0.2.10", "rule=terminal_ip_assignment", "sourceId=srv01")
			// **段階 3 の残りは 1 件である。** 割当の数を持つ項目と理由を書くと、端末が
			// 1 件に確定している応答を曖昧だと読ませる。
			for _, forbidden := range []string{"memberCount=", "reason=該当が複数件"} {
				if strings.Contains(derivation, forbidden) {
					t.Errorf("the derivation %q carries %q", derivation, forbidden)
				}
			}
			// 割当 1 件分の収集元と適用期間だけを並べる。
			if count := strings.Count(derivation, "sourceId="); count != 1 {
				t.Errorf("the derivation %q carries %d sourceId items, want 1", derivation, count)
			}
		})
	}
}

// forgedHostname は derivation の 3 つの区切りをすべて含む表示名である。
// 素朴に組へ分けた読み手に対して、実在しない rule の組に見える形を選ぶ。
const forgedHostname = `TESTHOST; rule=process_guid_match,NODE02`

// backslashHostname は末尾が `\` の表示名である。escape しないまま並べると、直後の
// 区切りが escape 扱いになり、2 つの要素が 1 つに融合する。
const backslashHostname = `NODE01\`

// derivation の値の読み方を test の中に書く。実装の関数を参照しない。
//
// 読み手は段階ごとに **escape の付かない区切り**で分け、葉に着いてから `\` を外す。
// 組の段階で `\` を外すと、値の中の並びと label の区切りが escape を失う。
func splitUnescapedDerivation(text, separator string) []string {
	parts := []string{}
	start := 0
	for index := 0; index < len(text); index++ {
		if text[index] == '\\' {
			index++
			continue
		}
		if strings.HasPrefix(text[index:], separator) {
			parts = append(parts, text[start:index])
			index += len(separator) - 1
			start = index + 1
		}
	}
	return append(parts, text[start:])
}

// unescapeDerivationValue は葉の値から escape を外す。
func unescapeDerivationValue(value string) string {
	var plain strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] == '\\' && index+1 < len(value) {
			index++
		}
		plain.WriteByte(value[index])
	}
	return plain.String()
}

// derivationPairValue は組から label と、escape を保ったままの値を返す。
func derivationPairValue(pair string) (string, string, bool) {
	parts := splitUnescapedDerivation(pair, derivationLabelSeparator)
	if len(parts) < 2 {
		return "", "", false
	}
	return parts[0], strings.Join(parts[1:], derivationLabelSeparator), true
}

// TestBuildEscapesTheDerivationValuesThatCarrySeparators は、原資料の文字列が derivation の
// 区切りを含んでも、実在しない組が現れないことを確かめる。
//
// 表示名は原資料が引用符で囲む欄であり、3 つの区切りをすべて書ける。
func TestBuildEscapesTheDerivationValuesThatCarrySeparators(t *testing.T) {
	// 3 行は tmid と ip が同じで com だけが違う。srv01 の観測期間に収まる時刻を持つ。
	// **末尾が `\` の表示名を、区切りを含む表示名より前に置く。** 最後に置くと、
	// 融合する相手が無いため `\` の escape の有無が derivation に出ない。
	markiiRow := func(sequence, clock, com string) string {
		return `02/01/2000 ` + clock + ` +0900 sn=` + sequence + ` evt=file subEvt=close ` +
			`com="` + com + `" tmid=` + srv01TerminalId + ` ip=192.0.2.10` + "\n"
	}
	source := markiiRow("401", "13:50:00.000", "TESTHOST") +
		markiiRow("402", "14:00:00.000", backslashHostname) +
		markiiRow("403", "14:10:00.000", forgedHostname)
	result := fieldsResult(t, squidInput("squid", fieldsSquidSource),
		markiiInput("srv01", source))

	// 原資料の側に区切りが届いていることを先に確かめる。届いていなければ、この test は
	// escape ではなく解析の漏れを見ていることになる。
	assignments := terminalAssignmentsOf(result)
	if len(assignments.entries) != 1 {
		t.Fatalf("the result carries %d assignments, want 1", len(assignments.entries))
	}
	wantHostnames := []string{"TESTHOST", backslashHostname, forgedHostname}
	observed := assignments.hostnamesOf(assignments.entries[0])
	if !slices.Equal(observed, wantHostnames) {
		t.Fatalf("the observed hostnames are %q, want %q", observed, wantHostnames)
	}

	fields := builtFields(t, result, "squid", core.PositionKindLineNumber, 1)
	derivation, ok := namedField(t, fields, "clientTerminalName").Text.DerivationValue()
	if !ok {
		t.Fatal("an undetermined value must carry why it was not derived")
	}

	// 段階 1。escape を知る読み手が分けた組の label は、下の wantLabels と一致する。
	labels := []string{}
	values := map[string]string{}
	for _, pair := range splitUnescapedDerivation(derivation, derivationItemSeparator) {
		label, value, found := derivationPairValue(pair)
		if !found {
			t.Fatalf("the pair %q carries no label separator (derivation %q)", pair, derivation)
		}
		labels = append(labels, label)
		values[label] = value
	}
	wantLabels := []string{"reason", "clientIp", "rule", "sourceId", "assignmentValidRange",
		"eventTime", "hostnameCount", "hostnames"}
	if !slices.Equal(labels, wantLabels) {
		t.Errorf("the derivation %q carries the labels %q, want %q",
			derivation, labels, wantLabels)
	}

	// **escape を知らない読み手にも、label の字面が増えない。** 本経路の derivation では
	// 組の外に `label=` の字面が無く、区切りを含む値が字面を残すと 2 つ目が現れる。
	for _, label := range wantLabels {
		literal := label + derivationLabelSeparator
		if count := strings.Count(derivation, literal); count != 1 {
			t.Errorf("the derivation %q carries %d %q literals, want 1",
				derivation, count, literal)
		}
	}

	// 段階 2 と段階 3。表示名そのものを応答が持つ。分析者が原資料へ戻って数え直さずに済む。
	// **要素を指定して確かめる。** 1 本の文字列として比べると、並びの区切りと `\` の
	// escape が外れても同じ値になる。
	carried := []string{}
	for _, element := range splitUnescapedDerivation(values["hostnames"], derivationListJoiner) {
		carried = append(carried, unescapeDerivationValue(element))
	}
	if !slices.Equal(carried, wantHostnames) {
		t.Errorf("the derivation %q carries the hostnames %q, want %q",
			derivation, carried, wantHostnames)
	}

	// derivation は表示名の数と並びの両方を持つ。2 つが食い違わない。
	if count := values["hostnameCount"]; count != strconv.Itoa(len(carried)) {
		t.Errorf("the hostnameCount is %q for the %d hostnames %q", count, len(carried), carried)
	}

	// 突き合わせに使った値も、同じ読み方で原資料の文字列へ戻る。
	if got := unescapeDerivationValue(values["clientIp"]); got != "192.0.2.10" {
		t.Errorf("the clientIp is %q, want %q", got, "192.0.2.10")
	}
}

// 段階 2 が補う item_absent の項目も語彙の項目を持つ。
//
// **意味の有無が値の有無で変わらない。** 同じ name が値を持つレコードで語彙の項目を
// 持ち、値を持たないレコードで持たない状態を作らない。
func TestBuildGivesTheAbsentItemsTheirSemantic(t *testing.T) {
	result := fieldsResult(t, squidInput("squid", fieldsSquidSource),
		markiiInput("srv01", fieldsSrv01Source))

	squidFields := builtFields(t, result, "squid", core.PositionKindLineNumber, 1)
	// Squid combined に欄そのものが無い 2 件。
	for name, want := range map[string]core.SemanticKey{
		"clientPort": core.SemanticKeyConnectionSourcePort,
		"process":    core.SemanticKeyProcessId,
	} {
		field := namedField(t, squidFields, name)
		if field.Text.ValueState != core.ValueStateItemAbsent {
			t.Fatalf("%s valueState = %q, want %q",
				name, field.Text.ValueState, core.ValueStateItemAbsent)
		}
		if field.Semantic != want {
			t.Errorf("%s semantic = %q, want %q", name, field.Semantic, want)
		}
	}

	// 反対側。段階 1 が補う item_absent の項目も、値を持つレコードと同じ語彙の項目を持つ。
	// 接続の subEvt が con のレコードは通信量の key を持たず、dcon のレコードは持つ。
	absent := builtFields(t, result, "srv01", core.PositionKindSequenceNumber, 1001)
	present := builtFields(t, result, "srv01", core.PositionKindSequenceNumber, 1002)
	for _, name := range []string{"recv", "send"} {
		absentField := namedField(t, absent, name)
		presentField := namedField(t, present, name)
		if absentField.Text.ValueState != core.ValueStateItemAbsent {
			t.Fatalf("%s valueState = %q, want %q",
				name, absentField.Text.ValueState, core.ValueStateItemAbsent)
		}
		if presentField.Text.ValueState != core.ValueStatePresent {
			t.Fatalf("%s valueState = %q, want %q",
				name, presentField.Text.ValueState, core.ValueStatePresent)
		}
		if absentField.Semantic != presentField.Semantic {
			t.Errorf("%s semantic = %q on the absent record and %q on the present record",
				name, absentField.Semantic, presentField.Semantic)
		}
		if !absentField.Semantic.IsKnown() {
			t.Errorf("%s semantic = %q, which is outside the vocabulary",
				name, absentField.Semantic)
		}
	}
}

// 期間の違う割当が 2 件あり、レコードの時刻で 1 件に絞れる起点は derived になる。
func TestBuildDerivesWhenTheRecordTimeNarrowsTwoAssignmentsToOne(t *testing.T) {
	result := fieldsResult(t, squidInput("squid", fieldsSquidSource),
		markiiInput("srv01", fieldsSrv01Source), markiiInput("pc02", fieldsPc02Source))
	cases := map[string]struct {
		lineNumber   int64
		terminalId   string
		terminalName string
		sourceId     string
	}{
		"期間の中が srv01": {
			lineNumber: 1, terminalId: srv01TerminalId, terminalName: "TESTHOST",
			sourceId: "sourceId=srv01",
		},
		"期間の中が pc02": {
			lineNumber: 3, terminalId: "pc02", terminalName: "CLIENT02", sourceId: "sourceId=pc02",
		},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			fields := builtFields(t, result, "squid", core.PositionKindLineNumber, want.lineNumber)

			for item, wanted := range map[string]string{
				"clientTerminal": want.terminalId, "clientTerminalName": want.terminalName,
			} {
				terminal := namedField(t, fields, item)
				if terminal.Text.ValueState != core.ValueStateDerived {
					t.Fatalf("%s valueState = %q, want %q", item, terminal.Text.ValueState,
						core.ValueStateDerived)
				}
				if normalized, _ := terminal.Text.NormalizedValue(); normalized != wanted {
					t.Errorf("%s normalized = %q, want %q", item, normalized, wanted)
				}
				derivation, _ := terminal.Text.DerivationValue()
				requireDerivationItems(t, derivation, want.sourceId)
			}
		})
	}
}

// 割当を探せない接続元と、期間の外の起点と、割当が複数残る起点の 3 つは
// derivation_undetermined になり、理由と併せて書くものを derivation が持つ。
func TestBuildLeavesTheClientTerminalUndeterminedWithTheReason(t *testing.T) {
	cases := map[string]struct {
		inputs     []fieldsSourceInput
		lineNumber int64
		reason     string
		items      []string
	}{
		"該当が 0 件": {
			inputs: []fieldsSourceInput{squidInput("squid", fieldsSquidSource),
				markiiInput("srv01", fieldsSrv01Source)},
			lineNumber: 2,
			reason:     "reason=該当が 0 件",
			items: []string{"clientIp=203.0.113.9", "rule=terminal_ip_assignment",
				"referencedSourceIds=srv01"},
		},
		"割当の期間の外": {
			inputs: []fieldsSourceInput{squidInput("squid", fieldsSquidSource),
				markiiInput("srv01", fieldsSrv01Source)},
			lineNumber: 3,
			reason:     "reason=割当の期間の外",
			items: []string{"clientIp=192.0.2.10", "sourceId=srv01",
				"assignmentValidRange=2000-02-01T03:04:05.678+09:00/2000-02-01T15:00:00.500+09:00",
				"eventTime=[01/Feb/2000:20:00:00 +0900]"},
		},
		"該当が複数件": {
			inputs: []fieldsSourceInput{squidInput("squid", fieldsSquidSource),
				markiiInput("srv01", fieldsSrv01Source), markiiInput("pc01", fieldsPc01Source)},
			lineNumber: 1,
			reason:     "reason=該当が複数件",
			items: []string{"clientIp=192.0.2.10", "sourceId=srv01", "sourceId=pc01",
				"memberCount=2", "eventTime=[01/Feb/2000:13:55:00 +0900]"},
		},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			result := fieldsResult(t, want.inputs...)

			fields := builtFields(t, result, "squid", core.PositionKindLineNumber, want.lineNumber)

			// name の集合は導出できなかった理由のいずれでも変わらない。
			requireFieldNames(t, fields, squidResponseFieldNames)
			// 識別子を導けない起点では、表示名も同じ理由で導けない。
			for _, item := range []string{"clientTerminal", "clientTerminalName"} {
				terminal := namedField(t, fields, item)
				if terminal.Text.ValueState != core.ValueStateDerivationUndetermined {
					t.Fatalf("%s valueState = %q, want %q", item, terminal.Text.ValueState,
						core.ValueStateDerivationUndetermined)
				}
				if raw, ok := terminal.Text.RawTextValue(); ok {
					t.Errorf("%s carries the raw text %q", item, raw)
				}
				if normalized, ok := terminal.Text.NormalizedValue(); ok {
					t.Errorf("%s carries the normalized value %q", item, normalized)
				}
				derivation, ok := terminal.Text.DerivationValue()
				if !ok {
					t.Fatal("an undetermined value must carry why it was not derived")
				}
				requireDerivationItems(t, derivation, append(want.items, want.reason)...)
			}
		})
	}
}

// 時刻を比べられない起点は、4 つ目の理由と比べられない側を derivation が持つ。
//
// fixture の 2 形式は UTC からのずれを値に持つため、offsetState が undetermined の
// 時刻をして確かめる。
func TestBuildReportsTheTimeItCannotCompare(t *testing.T) {
	result := fieldsResult(t, squidInput("squid", fieldsSquidSource),
		markiiInput("srv01", fieldsSrv01Source))
	index := NewCandidateIndex(result)
	entry, found := index.RecordAt("squid", core.PositionKindLineNumber, 1)
	if !found {
		t.Fatal("the record at line 1 of the squid source is missing")
	}
	rawText, normalized := "[01/Feb/2000:13:55:00]", "2000-02-01T13:55:00"
	observedAt, err := core.NewTimestamp(core.Timestamp{
		RawText: &rawText, Normalized: &normalized,
		NormalizedForm: core.NormalizedFormLocalWithoutOffset,
		Precision:      core.PrecisionSecond, OffsetState: core.OffsetStateUndetermined,
		Clock: core.ClockObserverLocal, Meaning: core.MeaningEvent,
		ValueState: core.ValueStatePresent,
	})
	if err != nil {
		t.Fatal(err)
	}
	entry.ObservedAt = &observedAt

	fields, err := NewFieldsBuilder(result).Build(entry)
	if err != nil {
		t.Fatalf("building the response fields: %v", err)
	}

	// name の集合は導出できなかった理由のいずれでも変わらない。
	requireFieldNames(t, fields, squidResponseFieldNames)
	terminal := namedField(t, fields, "clientTerminal")
	if terminal.Text.ValueState != core.ValueStateDerivationUndetermined {
		t.Fatalf("clientTerminal valueState = %q, want %q", terminal.Text.ValueState,
			core.ValueStateDerivationUndetermined)
	}
	derivation, _ := terminal.Text.DerivationValue()
	requireDerivationItems(t, derivation,
		"reason=時刻を比べられない",
		"clientIp=192.0.2.10",
		"sourceId=srv01",
		"assignmentValidRange=2000-02-01T03:04:05.678+09:00/2000-02-01T15:00:00.500+09:00",
		"eventTime=[01/Feb/2000:13:55:00]",
		"notComparable=eventTime=[01/Feb/2000:13:55:00]",
		"normalizedForm=local_without_offset",
		"offsetState=undetermined",
	)
}

// 接続元 IP を読めないレコードは、割当を探す段階の前で分かれる。
//
// **参照した収集元を書かない。** 段階 1 を実行していない状態で「0 件」と書くと、割当が
// 1 件も無かった事実を応答が持つ。srv01 は 192.0.2.10 の割当を持つ収集元であり、
// 「探したが 0 件だった」と読める応答を落第させる。
func TestBuildReportsTheMatchKeyItCannotRead(t *testing.T) {
	result := fieldsResult(t, squidInput("squid", fieldsUnreadableClientIpSource),
		markiiInput("srv01", fieldsSrv01Source))

	fields := builtFields(t, result, "squid", core.PositionKindLineNumber, 1)

	// name の集合は導出できなかった理由のいずれでも変わらない。
	requireFieldNames(t, fields, squidResponseFieldNames)
	terminal := namedField(t, fields, "clientTerminal")
	if terminal.Text.ValueState != core.ValueStateDerivationUndetermined {
		t.Fatalf("clientTerminal valueState = %q, want %q", terminal.Text.ValueState,
			core.ValueStateDerivationUndetermined)
	}
	if raw, ok := terminal.Text.RawTextValue(); ok {
		t.Errorf("clientTerminal carries the raw text %q", raw)
	}
	if normalized, ok := terminal.Text.NormalizedValue(); ok {
		t.Errorf("clientTerminal carries the normalized value %q", normalized)
	}
	derivation, ok := terminal.Text.DerivationValue()
	if !ok {
		t.Fatal("an undetermined value must carry why it was not derived")
	}
	const want = "reason=突き合わせる値を読めない; matchKeyItem=clientIp; " +
		"matchKeyValueState=absent; rule=terminal_ip_assignment"
	if derivation != want {
		t.Fatalf("the derivation is %q, want %q", derivation, want)
	}
	// 探していない段階の結果を持つ 2 つの文字列を落第させる。
	for _, forbidden := range []string{"referencedSourceIds=", "該当が 0 件"} {
		if strings.Contains(derivation, forbidden) {
			t.Errorf("the derivation %q carries %q", derivation, forbidden)
		}
	}
	// 反対側。同じ収集元の組で、接続元 IP を読めるレコードは端末を導ける。
	readable := fieldsResult(t, squidInput("squid", fieldsSquidSource),
		markiiInput("srv01", fieldsSrv01Source))
	derived := namedField(t, builtFields(t, readable, "squid", core.PositionKindLineNumber, 1),
		"clientTerminal")
	if derived.Text.ValueState != core.ValueStateDerived {
		t.Fatalf("clientTerminal valueState = %q, want %q", derived.Text.ValueState,
			core.ValueStateDerived)
	}
}

// syntheticField は指定した name と語彙の項目を持つ、原資料の文字列だけの項目を組む。
//
// 現行の 2 つの binding が収集元の byte 列から作らない項目の組み合わせを、分岐へ届ける。
func syntheticField(
	t *testing.T, name string, semantic core.SemanticKey, rawText string,
) core.RecordField {
	t.Helper()
	value, err := core.NewRawValue(core.ValueStatePresent, rawText)
	if err != nil {
		t.Fatal(err)
	}
	field, err := core.NewTextField(name, semantic, value)
	if err != nil {
		t.Fatal(err)
	}
	return field
}

// syntheticEntry は接続を記録したレコードを、渡した項目だけで組む。
// 接続の両端は先頭の項目が埋める。
func syntheticEntry(fields ...core.RecordField) RecordEntry {
	return RecordEntry{Semantics: &RecordSemantics{
		ObservationKind: core.ObservationKind{Raw: []core.RecordField{}},
		Fields:          fields,
		Endpoint: &RecordEndpoint{
			ClientEndpoint: fields[:1],
			Destination:    fields[:1],
		},
	}}
}

// 時刻を持つ項目を持たないレコードは、割当を探した後に「時刻を比べられない」で分かれる。
//
// 現行の 2 つの binding は意味付けを与えたレコードに必ず時刻を入れるため、本分岐へ届く
// レコードを収集元の byte 列から作れない。組を直接組み立てて分岐を固定する。
func TestBuildReportsTheRecordThatCarriesNoTime(t *testing.T) {
	result := fieldsResult(t, squidInput("squid", fieldsSquidSource),
		markiiInput("srv01", fieldsSrv01Source))
	entry := syntheticEntry(syntheticField(t, clientIpFieldName,
		core.SemanticKeyConnectionSourceAddress, "192.0.2.10"))

	fields, err := NewFieldsBuilder(result).Build(entry)
	if err != nil {
		t.Fatalf("building the response fields: %v", err)
	}

	terminal := namedField(t, fields, "clientTerminal")
	if terminal.Text.ValueState != core.ValueStateDerivationUndetermined {
		t.Fatalf("clientTerminal valueState = %q, want %q", terminal.Text.ValueState,
			core.ValueStateDerivationUndetermined)
	}
	derivation, ok := terminal.Text.DerivationValue()
	if !ok {
		t.Fatal("an undetermined value must carry why it was not derived")
	}
	const want = "reason=時刻を比べられない; clientIp=192.0.2.10; " +
		"rule=terminal_ip_assignment; sourceId=srv01; " +
		"assignmentValidRange=2000-02-01T03:04:05.678+09:00/2000-02-01T15:00:00.500+09:00; " +
		"notComparable=レコードが時刻を持つ項目を持っていない"
	if derivation != want {
		t.Fatalf("the derivation is %q, want %q", derivation, want)
	}
	// 割当は探せているため、参照した収集元の一覧を書かない。
	if strings.Contains(derivation, "referencedSourceIds=") {
		t.Errorf("the derivation %q carries referencedSourceIds=", derivation)
	}
}

// derivation は、突き合わせる値を読んだ項目の実際の name を書く。
//
// Squid combined は接続元 IP を clientIp の欄に書き、markii 形式の接続のレコードは srcIP に
// 書く。**段階 2 の補いが付ける clientIp を、どちらのレコードにも書かない。**
// 導出の元を name で指せなければ、応答から原資料の項目へ戻れない。
func TestBuildWritesTheNameOfTheItemItReadTheMatchKeyFrom(t *testing.T) {
	result := fieldsResult(t, squidInput("squid", fieldsSquidSource),
		markiiInput("srv01", fieldsSrv01Source))
	// srv01 の割当の期間の中にある時刻である。
	rawText, normalized, offsetText := "02/01/2000 13:55:00 +0900", "2000-02-01T13:55:00+09:00", "+0900"
	observedAt, err := core.NewTimestamp(core.Timestamp{
		RawText: &rawText, Normalized: &normalized, OffsetText: &offsetText,
		NormalizedForm: core.NormalizedFormRFC3339Absolute,
		Precision:      core.PrecisionSecond, OffsetState: core.OffsetStateInValue,
		Clock: core.ClockTerminalLocal, Meaning: core.MeaningEvent,
		ValueState: core.ValueStatePresent,
	})
	if err != nil {
		t.Fatal(err)
	}
	const sourceIp = "srcIP"
	entry := syntheticEntry(syntheticField(t, sourceIp,
		core.SemanticKeyConnectionSourceAddress, "192.0.2.10"))
	entry.ObservedAt = &observedAt

	fields, err := NewFieldsBuilder(result).Build(entry)
	if err != nil {
		t.Fatalf("building the response fields: %v", err)
	}

	terminal := namedField(t, fields, clientTerminalFieldName)
	if terminal.Text.ValueState != core.ValueStateDerived {
		t.Fatalf("clientTerminal valueState = %q, want %q", terminal.Text.ValueState,
			core.ValueStateDerived)
	}
	derivation, ok := terminal.Text.DerivationValue()
	if !ok {
		t.Fatal("a derived value must carry how it was derived")
	}
	const wantDerived = "srcIP=192.0.2.10; rule=terminal_ip_assignment; sourceId=srv01; " +
		"assignmentValidRange=2000-02-01T03:04:05.678+09:00/2000-02-01T15:00:00.500+09:00"
	if derivation != wantDerived {
		t.Errorf("the derivation is %q, want %q", derivation, wantDerived)
	}

	// 導出できなかった理由も同じ name を書く。時刻を外したレコードで確かめる。
	withoutTime := syntheticEntry(syntheticField(t, sourceIp,
		core.SemanticKeyConnectionSourceAddress, "192.0.2.10"))
	undeterminedFields, err := NewFieldsBuilder(result).Build(withoutTime)
	if err != nil {
		t.Fatalf("building the response fields: %v", err)
	}
	undetermined, ok := namedField(t, undeterminedFields,
		clientTerminalFieldName).Text.DerivationValue()
	if !ok {
		t.Fatal("an undetermined value must carry why it was not derived")
	}
	requireDerivationItems(t, undetermined, "srcIP=192.0.2.10")

	// 反対側。接続元 IP を clientIp の欄に書く Squid のレコードは clientIp を書く。
	squidDerivation, ok := namedField(t,
		builtFields(t, result, "squid", core.PositionKindLineNumber, 1),
		clientTerminalFieldName).Text.DerivationValue()
	if !ok {
		t.Fatal("a derived value must carry how it was derived")
	}
	requireDerivationItems(t, squidDerivation, "clientIp=192.0.2.10")

	for _, got := range []string{derivation, undetermined} {
		if strings.Contains(got, clientIpFieldName+"=") {
			t.Errorf("the derivation %q names the match key %q, which the record does not carry",
				got, clientIpFieldName)
		}
	}
}

// 端末の項目を片方だけ持つレコードには、もう片方も補わない。
//
// 識別子と表示名のどちらを持っていても、別の収集元の割当から残りを導かない。レコードが
// 書いた端末と割当が指す端末が別でも、応答が食い違いを持たなくなる (carriesSupplement)。
func TestBuildSupplementsNeitherTerminalItemWhenTheRecordCarriesOne(t *testing.T) {
	result := fieldsResult(t, squidInput("squid", fieldsSquidSource),
		markiiInput("srv01", fieldsSrv01Source))
	cases := map[string]struct {
		terminal core.RecordField
		want     []string
	}{
		"端末の識別子だけを持つ": {
			terminal: syntheticField(t, "tmid", core.SemanticKeyTerminalId, srv01TerminalId),
			want:     []string{"srcIP", "tmid", clientPortFieldName, processFieldName},
		},
		"端末の表示名だけを持つ": {
			terminal: syntheticField(t, "com", core.SemanticKeyTerminalHostname, `"TESTHOST"`),
			want:     []string{"srcIP", "com", clientPortFieldName, processFieldName},
		},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			entry := syntheticEntry(
				syntheticField(t, "srcIP", core.SemanticKeyConnectionSourceAddress,
					"192.0.2.10"),
				want.terminal)

			fields, err := NewFieldsBuilder(result).Build(entry)
			if err != nil {
				t.Fatalf("building the response fields: %v", err)
			}

			requireFieldNames(t, fields, want.want)
		})
	}
}

// 意味付けを持たないレコードは error になる。
//
// **入力形式は fields を組む条件に入らない。** 反対側として、取り込みの実行が対応表を
// 持たない入力形式のレコードからも fields が出ることを確かめる。
func TestBuildRejectsTheRecordsItCannotBuildFieldsFor(t *testing.T) {
	result := fieldsResult(t, markiiInput("srv01", fieldsSrv01Source))
	index := NewCandidateIndex(result)
	builder := NewFieldsBuilder(result)

	// 段階 1 が全レコードに意味付けを与えるため、取り込みの結果から Semantics が nil の
	// レコードは出てこない。内部不変条件が破れた場合の分岐を直接与えて確かめる。
	t.Run("意味付けを持たないレコード", func(t *testing.T) {
		entry, found := index.RecordAt("srv01", core.PositionKindSequenceNumber, 1000)
		if !found {
			t.Fatal("the record at sn=1000 is missing")
		}
		if entry.Semantics == nil {
			t.Fatal("the record at sn=1000 must carry semantics")
		}
		entry.Semantics = nil
		_, err := builder.Build(entry)
		if !errors.Is(err, ErrRecordWithoutSemantics) {
			t.Errorf("error = %v, want %v", err, ErrRecordWithoutSemantics)
		}
	})

	t.Run("意味付けを持つレコード", func(t *testing.T) {
		entry, found := index.RecordAt("srv01", core.PositionKindSequenceNumber, 1001)
		if !found {
			t.Fatal("the record at sn=1001 is missing")
		}
		fields, err := builder.Build(entry)
		if err != nil {
			t.Fatalf("building the response fields: %v", err)
		}
		requireFieldNames(t, fields, markiiResponseFieldNames)
	})
}

// 同じ内容の収集元を 2 回取り込むと割当が 2 件になる。2 件は同じ端末を指すため、端末を
// 確定させ、2 つの sourceId を derivation に残す。
func TestBuildDeterminesTheTerminalForTheSourceImportedTwice(t *testing.T) {
	result := fieldsResult(t, squidInput("squid", fieldsSquidSource),
		markiiInput("srv01", fieldsSrv01Source), markiiInput("srv01-again", fieldsSrv01Source))

	fields := builtFields(t, result, "squid", core.PositionKindLineNumber, 1)

	terminal := namedField(t, fields, "clientTerminal")
	if terminal.Text.ValueState != core.ValueStateDerived {
		t.Fatalf("clientTerminal valueState = %q, want %q", terminal.Text.ValueState,
			core.ValueStateDerived)
	}
	derivation, _ := terminal.Text.DerivationValue()
	requireDerivationItems(t, derivation, "sourceId=srv01", "sourceId=srv01-again")
	// 端末の値は、収集元を 1 回だけ取り込んだときと同じである。
	once := fieldsResult(t, squidInput("squid", fieldsSquidSource), markiiInput("srv01", fieldsSrv01Source))
	want := namedField(t, builtFields(t, once, "squid", core.PositionKindLineNumber, 1), "clientTerminal")
	got, _ := terminal.Text.ComparableValue()
	wanted, _ := want.Text.ComparableValue()
	if got == "" || got != wanted {
		t.Errorf("clientTerminal = %q, want %q", got, wanted)
	}
}

// 割当は収集元ごとに IPv4 と IPv6 の 2 件へまとめられる。
func TestTerminalAssignmentsFoldTheRepeatedPairs(t *testing.T) {
	result := fieldsResult(t, markiiInput("srv01", fieldsSrv01Source))

	assignments := terminalAssignmentsOf(result).entries

	if len(assignments) != 2 {
		t.Fatalf("the result carries %d assignments, want 2", len(assignments))
	}
	wantIps := []string{"192.0.2.10", "2001:db8::1"}
	for index, assignment := range assignments {
		if assignment.ClientIp != wantIps[index] {
			t.Errorf("the assignment at %d carries the IP %q, want %q",
				index, assignment.ClientIp, wantIps[index])
		}
		if assignment.TerminalId != srv01TerminalId {
			t.Errorf("the assignment at %d carries the identifier %q, want %q",
				index, assignment.TerminalId, srv01TerminalId)
		}
		if assignment.TerminalHostname != "TESTHOST" {
			t.Errorf("the assignment at %d carries the hostname %q, want TESTHOST",
				index, assignment.TerminalHostname)
		}
		if assignment.SourceId != "srv01" {
			t.Errorf("the assignment at %d carries the source %q, want srv01",
				index, assignment.SourceId)
		}
	}
}
