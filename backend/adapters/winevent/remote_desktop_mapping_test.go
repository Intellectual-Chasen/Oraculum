package winevent_test

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// eventWith は、プロバイダとイベント ID の 1 件に、body を System の後ろへ置いた文書である。
func eventWith(provider, eventID, body string) string {
	return `<Event><System><Provider Name="` + provider + `"/><EventID>` + eventID + `</EventID>` +
		`<TimeCreated SystemTime="2001-02-03T04:05:06Z"/><EventRecordID>501</EventRecordID>` +
		`<Computer>host05.example.test</Computer></System>` + body + `</Event>`
}

// userDataOf は、`<UserData><EventXML>` の子を名前と値の組で並べる。
func userDataOf(pairs ...string) string {
	body := `<UserData><EventXML xmlns="Event_NS">`
	for index := 0; index+1 < len(pairs); index += 2 {
		body += `<` + pairs[index] + `>` + pairs[index+1] + `</` + pairs[index] + `>`
	}
	return body + `</EventXML></UserData>`
}

const (
	remoteConnectionManager = "Microsoft-Windows-TerminalServices-RemoteConnectionManager"
	localSessionManager     = "Microsoft-Windows-TerminalServices-LocalSessionManager"
	rdpCoreTS               = "Microsoft-Windows-RemoteDesktopServices-RdpCoreTS"
)

// 1149 の UserData は、アカウントの名前と領域と接続元のアドレスを持つ。1149 は遠隔の
// セッションの記録である。
func TestObserveMapsTheUserDataOfAnAuthenticatedRemoteDesktopConnection(t *testing.T) {
	observation := observe(t, eventWith(remoteConnectionManager, "1149",
		userDataOf("Param1", "user51", "Param2", "EXAMPLE", "Param3", "192.0.2.10")))
	requireSemantics(t, observation,
		semanticItem{"UserData.EventXML.Param1", core.SemanticKeyTargetAccountName, "user51"},
		semanticItem{"UserData.EventXML.Param2", core.SemanticKeyTargetAccountDomain, "EXAMPLE"},
		semanticItem{"UserData.EventXML.Param3", core.SemanticKeyConnectionSourceAddress, "192.0.2.10"})
	if !observation.RemoteSession {
		t.Error("1149 is not a remote session record")
	}
}

// LocalSessionManager の Address は、IP アドレスのときだけ接続元の語彙を持つ。端末の前の
// セッションが書く文字列は、原資料の文字列のまま語彙を持たない項目になる。
func TestObserveMapsTheAddressOfASessionOnlyWhenItIsAnIpAddress(t *testing.T) {
	remote := observe(t, eventWith(localSessionManager, "25",
		userDataOf("User", `EXAMPLE\user52`, "SessionID", "3", "Address", "192.0.2.11")))
	requireSemantics(t, remote,
		semanticItem{"UserData.EventXML.User", core.SemanticKeyTargetAccountName, `EXAMPLE\user52`},
		semanticItem{"UserData.EventXML.SessionID", "", "3"},
		semanticItem{"UserData.EventXML.Address", core.SemanticKeyConnectionSourceAddress, "192.0.2.11"})
	local := observe(t, eventWith(localSessionManager, "21",
		userDataOf("User", `EXAMPLE\user52`, "SessionID", "1", "Address", "LOCAL")))
	address := fieldNamed(t, local.Fields, "UserData.EventXML.Address")
	if value, ok := address.Text.ComparableValue(); address.Semantic != "" || !ok || value != "LOCAL" {
		t.Errorf("the local address = %q %q (%t), want a present value without a semantic", address.Semantic, value, ok)
	}
	for eventID, want := range map[string]bool{"21": true, "23": false, "24": true, "25": true} {
		observation := observe(t, eventWith(localSessionManager, eventID, userDataOf("User", `EXAMPLE\user52`)))
		if observation.RemoteSession != want {
			t.Errorf("LocalSessionManager %s RemoteSession = %t, want %t", eventID, observation.RemoteSession, want)
		}
	}
}

// 131 の ClientIP は port を付けたアドレスであり、アドレスだけを比べる。131 は認証の前の
// 接続の受け付けであり、遠隔のセッションの記録としない。
func TestObserveComparesTheAddressOfAnAcceptedConnectionWithoutThePort(t *testing.T) {
	for _, clientIP := range []string{"192.0.2.12:50001", "[192.0.2.12]:50001", "[::ffff:192.0.2.12]:50001"} {
		observation := observe(t, eventWith(rdpCoreTS, "131",
			`<EventData><Data Name="ConnType">TCP</Data><Data Name="ClientIP">`+clientIP+`</Data></EventData>`))
		requireSemantics(t, observation,
			semanticItem{"EventData.ClientIP", core.SemanticKeyConnectionSourceAddress, "192.0.2.12"})
		if raw, _ := fieldNamed(t, observation.Fields, "EventData.ClientIP").Text.RawTextValue(); raw != clientIP {
			t.Errorf("ClientIP raw = %q, want %q", raw, clientIP)
		}
		if observation.RemoteSession {
			t.Errorf("131 with %s is a remote session record", clientIP)
		}
	}
}

// 4104 のスクリプトブロックの本文は、シェルが実行したコマンドである。Path は本文を読んだ
// スクリプトのファイルである。ほかの欄は語彙を持たない。
func TestObserveMapsTheScriptBlockTextToTheShellCommand(t *testing.T) {
	observation := observe(t, eventWith("Microsoft-Windows-PowerShell", "4104",
		`<EventData><Data Name="MessageNumber">1</Data><Data Name="MessageTotal">1</Data>`+
			`<Data Name="ScriptBlockText">.\tool51.exe a 'X:\d\o.zip' X:\d\i.txt</Data>`+
			`<Data Name="ScriptBlockId">00000000-0000-0000-0000-000000000051</Data>`+
			`<Data Name="Path">C:\Example\s51.ps1</Data></EventData>`))
	requireSemantics(t, observation,
		semanticItem{"EventData.ScriptBlockText", core.SemanticKeyProcessShellCommand, `.\tool51.exe a 'X:\d\o.zip' X:\d\i.txt`},
		semanticItem{"EventData.Path", core.SemanticKeyFilePath, `C:\Example\s51.ps1`},
		semanticItem{"EventData.MessageTotal", "", ""})
	// 端末に置いたレコードは、Path のファイルのノードを指す。
	terminal, _ := core.TerminalNodeKey("terminal-51")
	files := core.OperatedFileNodeKeys(observation.Fields, core.RecordScope{Terminal: &terminal, NamesTerminal: true})
	if len(files) != 1 || files[0].Values[len(files[0].Values)-1].Value != core.FilePathKeyValue(`C:\Example\s51.ps1`) {
		t.Errorf("the file nodes = %+v, want the script file", files)
	}
	// 対話の入力の本文は Path が空であり、ファイルのノードを作らない。
	interactive := observe(t, eventWith("Microsoft-Windows-PowerShell", "4104",
		`<EventData><Data Name="MessageNumber">1</Data><Data Name="MessageTotal">1</Data>`+
			`<Data Name="ScriptBlockText">Get-Item X:\d</Data>`+
			`<Data Name="ScriptBlockId">00000000-0000-0000-0000-000000000052</Data><Data Name="Path"></Data></EventData>`))
	if files := core.OperatedFileNodeKeys(interactive.Fields,
		core.RecordScope{Terminal: &terminal, NamesTerminal: true}); len(files) != 0 {
		t.Errorf("the file nodes of an empty Path = %+v, want none", files)
	}
}
