package core_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// byte 位置で指すレコードもノードになり、位置が違えば別のノードになる。
func TestRecordNodeKeyOfAByteRangeLocator(t *testing.T) {
	first, found := core.NewRecordNodeKey(byteRangeLocator(byteRangeOffset, byteRangeLength))
	if !found {
		t.Fatal("NewRecordNodeKey found no key, want a key for a byte range locator")
	}
	second, found := core.NewRecordNodeKey(byteRangeLocator(byteRangeOffset+byteRangeLength, 120))
	if !found {
		t.Fatal("NewRecordNodeKey found no key for the following record")
	}
	firstParts := strings.Join(first.DigestParts(), "\x00")
	secondParts := strings.Join(second.DigestParts(), "\x00")
	if firstParts == secondParts {
		t.Errorf("both records key as %q, want the byte offset to separate them", firstParts)
	}

	withoutOffset := byteRangeLocator(byteRangeOffset, byteRangeLength)
	withoutOffset.ByteOffset = nil
	if _, found := core.NewRecordNodeKey(withoutOffset); found {
		t.Error("NewRecordNodeKey found a key without a byte offset, want none")
	}
}

// textField は検査用に 1 項目を組む。値は原資料の文字列だけを持つ。
func textField(t *testing.T, name string, semantic core.SemanticKey, rawText string) core.RecordField {
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

// absentField は値の不在を表す文字列が入った 1 項目を組む。
func absentField(t *testing.T, name string, semantic core.SemanticKey) core.RecordField {
	t.Helper()
	value, err := core.NewRawValue(core.ValueStateAbsent, "-")
	if err != nil {
		t.Fatal(err)
	}
	field, err := core.NewTextField(name, semantic, value)
	if err != nil {
		t.Fatal(err)
	}
	return field
}

// linkNames は関係を「種別 起点の種別 終点の種別」の文字列へ直す。
func linkNames(links []core.RecordLink) []string {
	names := make([]string, 0, len(links))
	for _, link := range links {
		names = append(names, string(link.Kind)+" "+string(link.Source.Kind)+" "+
			string(link.Target.Kind))
	}
	return names
}

func requireStrings(t *testing.T, item string, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s is %v, want %v", item, got, want)
	}
	for index, value := range want {
		if got[index] != value {
			t.Errorf("%s at %d is %q, want %q", item, index, got[index], value)
		}
	}
}

func TestNewRecordGraphBuildsProcessObjectLinks(t *testing.T) {
	graph := core.NewRecordGraph([]core.RecordField{
		textField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
		textField(t, "com", core.SemanticKeyTerminalHostname, "CLIENT01"),
		textField(t, "psGUID", core.SemanticKeyProcessId, "P1"),
		textField(t, "path", core.SemanticKeyFilePath, `C:\a\b.txt`),
		textField(t, "usr", core.SemanticKeyAccountName, "alice"),
		textField(t, "usrDomain", core.SemanticKeyAccountDomain, "AD"),
		textField(t, "ip", core.SemanticKeyTerminalIpAddress, "192.0.2.1"),
	})
	requireStrings(t, "node kinds", nodeKinds(graph.Nodes),
		"terminal", "process", "account", "file", "ip")
	requireStrings(t, "links", linkNames(graph.Links),
		"terminal_address terminal ip",
		"terminal_account terminal account",
		"ran_on process terminal",
		"file_operation process file")
}

func TestNewRecordGraphBuildsDirectedFileCopyLinks(t *testing.T) {
	graph := core.NewRecordGraph([]core.RecordField{
		textField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
		textField(t, "psGUID", core.SemanticKeyProcessId, "P1"),
		textField(t, "path", core.SemanticKeyFilePath, `\\server\source.zip`),
		textField(t, "dstPath", core.SemanticKeyFileDestinationPath, `E:\source.zip`),
	})
	if len(graph.Nodes) != 4 {
		t.Fatalf("the graph carries %d nodes, want terminal, process, source, and destination", len(graph.Nodes))
	}
	if graph.Nodes[2].Values[1].Semantic != "" || graph.Nodes[3].Values[1].Semantic != "" {
		t.Fatalf("file identities are %+v, want no naming semantic", graph.Nodes[2:])
	}
	if len(graph.Links) != 3 {
		t.Fatalf("the graph carries %d links, want ran_on, file_operation, and file_copy", len(graph.Links))
	}
	var copyLink core.RecordLink
	for _, link := range graph.Links {
		if link.Kind == core.EdgeKindFileCopy {
			copyLink = link
		}
	}
	if copyLink.Kind != core.EdgeKindFileCopy ||
		copyLink.Source.Values[1].Value != `\\server\source.zip` ||
		copyLink.Target.Values[1].Value != `e:\source.zip` {
		t.Errorf("copy link = %+v, want source-to-destination direction", copyLink)
	}
}

func TestNewRecordGraphSkipsFileCopySelfLinks(t *testing.T) {
	graph := core.NewRecordGraph([]core.RecordField{
		textField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
		textField(t, "psGUID", core.SemanticKeyProcessId, "P1"),
		textField(t, "path", core.SemanticKeyFilePath, `C:\same.zip`),
		textField(t, "dstPath", core.SemanticKeyFileDestinationPath, `C:\same.zip`),
	})
	for _, link := range graph.Links {
		if link.Kind == core.EdgeKindFileCopy {
			t.Fatalf("the graph carries a file_copy self-link %+v, want no self-link", link)
		}
	}
}

func TestNewRecordGraphReferencesTheParentProcess(t *testing.T) {
	graph := core.NewRecordGraph([]core.RecordField{
		textField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
		textField(t, "psGUID", core.SemanticKeyProcessId, "{P1}"),
		textField(t, "parentGUID", core.SemanticKeyParentProcessId, "{P0}"),
		textField(t, "parentPath", core.SemanticKeyParentProcessBinaryPath, `C:\parent.exe`),
	})
	requireStrings(t, "node kinds", nodeKinds(graph.Nodes), "terminal", "process")
	if len(graph.ReferencedNodes) != 1 {
		t.Fatalf("the record graph carries %d referenced nodes, want 1",
			len(graph.ReferencedNodes))
	}
	parent := graph.ReferencedNodes[0]
	if parent.Key.Kind != core.NodeKindProcess ||
		parent.Key.Form != core.NodeKeyFormTerminalProcess {
		t.Errorf("the referenced node is %s/%s, want process/terminal_id_process_id",
			parent.Key.Kind, parent.Key.Form)
	}
	if parent.LabelSemantic != core.SemanticKeyParentProcessBinaryPath {
		t.Errorf("the referenced node reads its label from %q, want %q",
			parent.LabelSemantic, core.SemanticKeyParentProcessBinaryPath)
	}
	// **鍵の項目は process.id である。** 同じプロセスを起動のレコードが記録したときと
	// 同じ識別鍵になる。
	requireStrings(t, "identity values", identityValues(parent.Key), "T1", "{P0}")
	if len(parent.Key.Values) != 2 ||
		parent.Key.Values[1].Semantic != core.SemanticKeyProcessId {
		t.Errorf("the referenced node carries the identity %+v, want the semantic process.id",
			parent.Key.Values)
	}
	requireStrings(t, "links", linkNames(graph.Links),
		"ran_on process terminal", "process_parent_child process process")
	if graph.Links[1].Source.Values[1].Value != "{P0}" ||
		graph.Links[1].Target.Values[1].Value != "{P1}" {
		t.Errorf("the parent and child link runs from %q to %q, want {P0} to {P1}",
			graph.Links[1].Source.Values[1].Value, graph.Links[1].Target.Values[1].Value)
	}
}

// 親の識別子がレコードの記録したプロセスと同じ値である組は関係にしない。
func TestNewRecordGraphSkipsTheParentThatNamesTheSameProcess(t *testing.T) {
	graph := core.NewRecordGraph([]core.RecordField{
		textField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
		textField(t, "psGUID", core.SemanticKeyProcessId, "{P1}"),
		textField(t, "parentGUID", core.SemanticKeyParentProcessId, "{P1}"),
	})
	if len(graph.ReferencedNodes) != 0 {
		t.Errorf("the record graph carries %d referenced nodes, want 0",
			len(graph.ReferencedNodes))
	}
	requireStrings(t, "links", linkNames(graph.Links), "ran_on process terminal")
}

// 注入のレコードは、注入元のプロセスから注入先のプロセスへ向かう関係を作る。
func TestNewRecordGraphRunsTheInjectionFromTheRecordedProcess(t *testing.T) {
	graph := core.NewRecordGraph([]core.RecordField{
		textField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
		textField(t, "psGUID", core.SemanticKeyProcessId, "{P1}"),
		textField(t, "tpsGUID", core.SemanticKeyInjectionTargetProcessId, "{P9}"),
		textField(t, "tpsPath", core.SemanticKeyInjectionTargetProcessBinaryPath,
			`C:\target.exe`),
	})
	requireStrings(t, "node kinds", nodeKinds(graph.Nodes), "terminal", "process")
	if len(graph.ReferencedNodes) != 1 {
		t.Fatalf("the record graph carries %d referenced nodes, want 1",
			len(graph.ReferencedNodes))
	}
	target := graph.ReferencedNodes[0]
	if target.LabelSemantic != core.SemanticKeyInjectionTargetProcessBinaryPath {
		t.Errorf("the injection target reads its label from %q, want %q",
			target.LabelSemantic, core.SemanticKeyInjectionTargetProcessBinaryPath)
	}
	// **鍵の項目は process.id である。** 注入先のプロセスの起動のレコードが指すときと
	// 同じ識別鍵になる。
	requireStrings(t, "identity values", identityValues(target.Key), "T1", "{P9}")
	if len(target.Key.Values) != 2 ||
		target.Key.Values[1].Semantic != core.SemanticKeyProcessId {
		t.Errorf("the injection target carries the identity %+v, want the semantic process.id",
			target.Key.Values)
	}
	requireStrings(t, "links", linkNames(graph.Links),
		"ran_on process terminal", "process_injection process process")
	if graph.Links[1].Source.Values[1].Value != "{P1}" ||
		graph.Links[1].Target.Values[1].Value != "{P9}" {
		t.Errorf("the injection link runs from %q to %q, want {P1} to {P9}",
			graph.Links[1].Source.Values[1].Value, graph.Links[1].Target.Values[1].Value)
	}
}

// 注入先がレコードの記録したプロセスと同じ値である組は関係にしない。
func TestNewRecordGraphSkipsTheInjectionThatNamesTheSameProcess(t *testing.T) {
	graph := core.NewRecordGraph([]core.RecordField{
		textField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
		textField(t, "psGUID", core.SemanticKeyProcessId, "{P1}"),
		textField(t, "tpsGUID", core.SemanticKeyInjectionTargetProcessId, "{P1}"),
	})
	if len(graph.ReferencedNodes) != 0 {
		t.Errorf("the record graph carries %d referenced nodes, want 0",
			len(graph.ReferencedNodes))
	}
	requireStrings(t, "links", linkNames(graph.Links), "ran_on process terminal")
}

// 大文字と小文字だけが違う Windows の path は、同じファイルの鍵になる。POSIX の path は区別する。
func TestFileNodeKeysFoldTheCaseOfWindowsPaths(t *testing.T) {
	terminal, _ := core.TerminalNodeKey("T1")
	scope := core.RecordScope{Terminal: &terminal, NamesTerminal: true}
	keyOf := func(path string) core.NodeKey {
		t.Helper()
		keys := core.OperatedFileNodeKeys([]core.RecordField{textField(t, "p", core.SemanticKeyFilePath, path)}, scope)
		if len(keys) != 1 {
			t.Fatalf("the path %s gives %d file keys, want 1", path, len(keys))
		}
		return keys[0]
	}
	for _, pair := range [][2]string{
		{`c:\synth\tool.exe`, `C:\SYNTH\TOOL.EXE`},
		{`\\host.example.test\s\a.txt`, `\\HOST.EXAMPLE.TEST\S\A.TXT`},
	} {
		if left, right := identityValues(keyOf(pair[0])), identityValues(keyOf(pair[1])); !slices.Equal(left, right) {
			t.Errorf("the keys of %s and %s are %v and %v, want one key", pair[0], pair[1], left, right)
		}
	}
	if left, right := identityValues(keyOf("/synth/a.txt")), identityValues(keyOf("/SYNTH/A.TXT")); slices.Equal(left, right) {
		t.Errorf("the POSIX paths share the key %v", left)
	}
}

// ExecutableLink は、端末とプロセスと空でない実行ファイルの path を持つレコードだけから、
// 同じ端末のファイルからプロセスへの関係を組む。
func TestExecutableLinkNeedsTheTerminalTheProcessAndThePath(t *testing.T) {
	terminal := textField(t, "tmid", core.SemanticKeyTerminalId, "T1")
	process := textField(t, "psGUID", core.SemanticKeyProcessId, "{P1}")
	path := textField(t, "psPath", core.SemanticKeyProcessBinaryPath, `C:\a.exe`)
	link, ok := core.ExecutableLink([]core.RecordField{terminal, process, path}, core.RecordScope{})
	if !ok {
		t.Fatal("the record with the terminal, the process and the path builds no link")
	}
	if link.Kind != core.EdgeKindProcessExecutable || link.Source.Kind != core.NodeKindFile ||
		link.Target.Kind != core.NodeKindProcess {
		t.Errorf("the link is %s from %s to %s, want process_executable from file to process",
			link.Kind, link.Source.Kind, link.Target.Kind)
	}
	requireStrings(t, "file identity values", identityValues(link.Source), "T1", `c:\a.exe`)
	for name, fields := range map[string][]core.RecordField{
		"without the terminal": {process, path},
		"without the process":  {terminal, path},
		"without the path":     {terminal, process},
		"with an empty path": {terminal, process,
			textField(t, "psPath", core.SemanticKeyProcessBinaryPath, "")},
	} {
		if _, ok := core.ExecutableLink(fields, core.RecordScope{}); ok {
			t.Errorf("the record %s builds a link", name)
		}
	}
}

// 端末の外部識別子を持たないレコードは、注入の関係と注入先のノードを持たない。
func TestNewRecordGraphBuildsNoInjectionWithoutTheTerminal(t *testing.T) {
	graph := core.NewRecordGraph([]core.RecordField{
		textField(t, "psGUID", core.SemanticKeyProcessId, "{P1}"),
		textField(t, "tpsGUID", core.SemanticKeyInjectionTargetProcessId, "{P9}"),
	})
	if len(graph.ReferencedNodes) != 0 || len(graph.Links) != 0 {
		t.Errorf("the record graph carries %d referenced nodes and %d links, want 0 and 0",
			len(graph.ReferencedNodes), len(graph.Links))
	}
}

func TestDestinationNodeKeysAndProcessNodeKeyReadTheSameRecord(t *testing.T) {
	fields := []core.RecordField{
		textField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
		textField(t, "psGUID", core.SemanticKeyProcessId, "{P1}"),
		textField(t, "dstIP", core.SemanticKeyConnectionDestinationAddress, "198.51.100.7"),
		textField(t, "host", core.SemanticKeyConnectionDestinationHostname, "example.test"),
	}
	destinations := core.DestinationNodeKeys(fields, core.RecordScope{})
	requireStrings(t, "destination kinds", nodeKinds(destinations), "ip", "domain")
	requireStrings(t, "destination values",
		[]string{destinations[0].Values[0].Value, destinations[1].Values[0].Value},
		"198.51.100.7", "example.test")

	process, built := core.ProcessNodeKey(fields)
	if !built {
		t.Fatal("a record naming a terminal and a process returned no process key")
	}
	requireStrings(t, "process identity values", identityValues(process), "T1", "{P1}")

	// 端末の外部識別子を持たないレコードはプロセスのノードを持たない。
	if _, built := core.ProcessNodeKey([]core.RecordField{
		textField(t, "psGUID", core.SemanticKeyProcessId, "{P1}"),
	}); built {
		t.Error("a record without a terminal identifier returned a process key")
	}
}

// identityValues は識別鍵の値を並び順で返す。
func identityValues(key core.NodeKey) []string {
	values := make([]string, 0, len(key.Values))
	for _, value := range key.Values {
		values = append(values, value.Value)
	}
	return values
}

func TestNewRecordGraphWithoutTerminalIdBuildsNoScopedNode(t *testing.T) {
	graph := core.NewRecordGraph([]core.RecordField{
		textField(t, "psGUID", core.SemanticKeyProcessId, "P1"),
		textField(t, "path", core.SemanticKeyFilePath, `C:\a\b.txt`),
		textField(t, "entry", core.SemanticKeyRegistryValueKeyPath, `HKLM\Run`),
	})
	requireStrings(t, "node kinds", nodeKinds(graph.Nodes))
	requireStrings(t, "links", linkNames(graph.Links))
}

// 端末を名乗らないレコードは、範囲の端末の範囲で対象を識別する。端末の範囲の対象を指さない
// レコードは、範囲が端末を指させるときだけ端末を指す。
func TestNewRecordGraphInScopePlacesTheRecordOnTheRecordingTerminal(t *testing.T) {
	recording, _ := core.RecordingTerminalNodeKey(
		"0000000000000000000000000000000000000000000000000000000000000001")
	scoped := []core.RecordField{
		textField(t, "path", core.SemanticKeyFilePath, "/usr/bin/tool"),
		textField(t, "AUID", core.SemanticKeyAccountName, "operator"),
	}
	unscoped := []core.RecordField{
		textField(t, "user", core.SemanticKeyAccountName, "operator"),
		textField(t, "dst", core.SemanticKeyConnectionDestinationAddress, "192.0.2.1"),
	}

	t.Run("端末の範囲の対象を指すレコード", func(t *testing.T) {
		graph := core.NewRecordGraphInScope(scoped, core.RecordScope{
			Terminal: &recording, AccountNamesLocal: true,
		})
		requireStrings(t, "node kinds", nodeKinds(graph.Nodes), "terminal", "account", "file")
		requireStrings(t, "links", linkNames(graph.Links), "terminal_account terminal account")
		account, named := core.AccountNodeKey(scoped, core.RecordScope{
			Terminal: &recording, AccountNamesLocal: true,
		})
		if !named || account.Form != core.NodeKeyFormTerminalAccountName ||
			account.Values[0] != recording.Values[0] ||
			!slices.ContainsFunc(graph.Nodes, func(key core.NodeKey) bool {
				return reflect.DeepEqual(key, account)
			}) {
			t.Errorf("the account key is %+v, want the login name in the scope of %+v among %+v",
				account, recording, graph.Nodes)
		}
	})
	t.Run("ログイン名を端末の範囲で識別しない形式", func(t *testing.T) {
		graph := core.NewRecordGraphInScope(unscoped, core.RecordScope{Terminal: &recording})
		requireStrings(t, "node kinds", nodeKinds(graph.Nodes), "ip")
	})
	t.Run("範囲が端末を指させる", func(t *testing.T) {
		graph := core.NewRecordGraphInScope(unscoped, core.RecordScope{
			Terminal: &recording, NamesTerminal: true,
		})
		requireStrings(t, "node kinds", nodeKinds(graph.Nodes), "terminal", "ip")
	})
	t.Run("レコード自身の端末を範囲の端末で置き換えない", func(t *testing.T) {
		own := append([]core.RecordField{textField(t, "tmid", core.SemanticKeyTerminalId, "T1")}, scoped...)
		graph := core.NewRecordGraphInScope(own, core.RecordScope{
			Terminal: &recording, NamesTerminal: true,
		})
		requireStrings(t, "terminal", identityValues(graph.Nodes[0]), "T1")
	})
}

func TestNewRecordGraphKeepsOneIpNodeForTwoSemantics(t *testing.T) {
	graph := core.NewRecordGraph([]core.RecordField{
		textField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
		textField(t, "ip", core.SemanticKeyTerminalIpAddress, "192.0.2.1"),
		textField(t, "dstIP", core.SemanticKeyConnectionDestinationAddress, "192.0.2.1"),
	})
	requireStrings(t, "node kinds", nodeKinds(graph.Nodes), "terminal", "ip")
	if len(graph.Nodes) != 2 {
		t.Fatalf("the record graph carries %d nodes, want 2", len(graph.Nodes))
	}
	if got := graph.Nodes[1].Form; got != core.NodeKeyFormAddress {
		t.Errorf("the address node carries the key form %q, want %q", got, core.NodeKeyFormAddress)
	}
}

func TestNewRecordGraphSplitsTheTwoAccountKeyForms(t *testing.T) {
	withSid := core.NewRecordGraph([]core.RecordField{
		textField(t, "sid", core.SemanticKeyAccountSid, "S-1-5-21-1"),
		textField(t, "usr", core.SemanticKeyAccountName, "alice"),
		textField(t, "usrDomain", core.SemanticKeyAccountDomain, "AD"),
	})
	withoutSid := core.NewRecordGraph([]core.RecordField{
		textField(t, "usr", core.SemanticKeyAccountName, "alice"),
		textField(t, "usrDomain", core.SemanticKeyAccountDomain, "AD"),
	})
	if got := withSid.Nodes[0].Form; got != core.NodeKeyFormAccountSid {
		t.Errorf("the account with a sid carries the key form %q, want %q",
			got, core.NodeKeyFormAccountSid)
	}
	if got := withoutSid.Nodes[0].Form; got != core.NodeKeyFormAccountDomainName {
		t.Errorf("the account without a sid carries the key form %q, want %q",
			got, core.NodeKeyFormAccountDomainName)
	}
}

// イベントが記録したアカウントは、レコードのアカウントの識別鍵を上書きしない。
func TestNewRecordGraphKeepsEventAccountOutsideTheAccountIdentity(t *testing.T) {
	graph := core.NewRecordGraph([]core.RecordField{
		textField(t, "usr", core.SemanticKeyAccountName, "alice"),
		textField(t, "usrDomain", core.SemanticKeyAccountDomain, "AD"),
		textField(t, "evtUsr", core.SemanticKeyEventAccountName, "bob"),
		textField(t, "evtDomain", core.SemanticKeyEventAccountDomain, "OTHER"),
	})
	if len(graph.Nodes) != 1 || graph.Nodes[0].Kind != core.NodeKindAccount {
		t.Fatalf("the graph carries %d nodes, want one account node", len(graph.Nodes))
	}
	requireStrings(t, "account identity values", identityValues(graph.Nodes[0]), "AD", "alice")
}

// 比べる値が空の文字列である識別の項目からノードを作らない。空の値の識別鍵は
// NodeKey.Validate を通らない。
//
// アカウントでは、ドメインが空の文字列であるレコードはドメインとログイン名の組の鍵を組めず、
// アカウントのノードを指さない。
func TestNewRecordGraphSkipsTheEmptyIdentityValues(t *testing.T) {
	graph := core.NewRecordGraph([]core.RecordField{
		textField(t, "tid", core.SemanticKeyTerminalId, "T1"),
		textField(t, "IpAddress", core.SemanticKeyConnectionSourceAddress, ""),
		textField(t, "dstHost", core.SemanticKeyConnectionDestinationHostname, ""),
		textField(t, "usrDomain", core.SemanticKeyAccountDomain, ""),
		textField(t, "usr", core.SemanticKeyAccountName, "alice"),
	})
	requireStrings(t, "node kinds", nodeKinds(graph.Nodes), "terminal")
	for _, key := range graph.Nodes {
		if err := key.Validate(); err != nil {
			t.Errorf("the node key %+v = %v, want a valid key", key, err)
		}
	}
	// 反対側。空でない値は同じ項目からノードを作る。
	named := core.NewRecordGraph([]core.RecordField{
		textField(t, "tid", core.SemanticKeyTerminalId, "T1"),
		textField(t, "IpAddress", core.SemanticKeyConnectionSourceAddress, "192.0.2.1"),
		textField(t, "usrDomain", core.SemanticKeyAccountDomain, "AD"),
		textField(t, "usr", core.SemanticKeyAccountName, "alice"),
	})
	requireStrings(t, "node kinds", nodeKinds(named.Nodes), "terminal", "account", "ip")
}

func TestNewRecordGraphSkipsTheValueAbsentAccountName(t *testing.T) {
	graph := core.NewRecordGraph([]core.RecordField{
		textField(t, "clientIp", core.SemanticKeyConnectionSourceAddress, "192.0.2.1"),
		absentField(t, "user", core.SemanticKeyAccountName),
		textField(t, "requestTargetHost", core.SemanticKeyConnectionDestinationHostname, "example.test"),
		textField(t, "requestMethod", core.SemanticKeyHttpRequestMethod, "GET"),
	})
	requireStrings(t, "node kinds", nodeKinds(graph.Nodes), "ip", "domain")
	requireStrings(t, "links", linkNames(graph.Links), "http_request ip domain")
}

func TestNewRecordGraphWithoutHttpItemsBuildsNoRequestLink(t *testing.T) {
	graph := core.NewRecordGraph([]core.RecordField{
		textField(t, "clientIp", core.SemanticKeyConnectionSourceAddress, "192.0.2.1"),
		textField(t, "dstIP", core.SemanticKeyConnectionDestinationAddress, "198.51.100.7"),
	})
	requireStrings(t, "node kinds", nodeKinds(graph.Nodes), "ip", "ip")
	requireStrings(t, "links", linkNames(graph.Links))
}

// 1 つのアドレスを 2 つの語彙の項目が指しても、同じ 1 つのノードになる。
func TestNewRecordGraphDigestsAnAddressWithoutTheNamingSemantic(t *testing.T) {
	byTerminal := core.NewRecordGraph([]core.RecordField{
		textField(t, "ip", core.SemanticKeyTerminalIpAddress, "192.0.2.1"),
	})
	byDestination := core.NewRecordGraph([]core.RecordField{
		textField(t, "dstIP", core.SemanticKeyConnectionDestinationAddress, "192.0.2.1"),
	})
	requireStrings(t, "digest parts by the terminal address",
		byTerminal.Nodes[0].DigestParts(), "ip", "address", "192.0.2.1")
	requireStrings(t, "digest parts by the destination address",
		byDestination.Nodes[0].DigestParts(), "ip", "address", "192.0.2.1")
	identity := byTerminal.Nodes[0].Identity()
	if len(identity) != 1 || identity[0].Value != "192.0.2.1" || identity[0].Semantic != "" {
		t.Fatalf("the address identity is %+v, want one value without a semantic", identity)
	}
}

// ホスト名は識別鍵に入る語彙の項目が 1 つであるため、鍵が項目を持つ。
func TestNewRecordGraphKeepsTheHostnameSemanticInTheKey(t *testing.T) {
	graph := core.NewRecordGraph([]core.RecordField{
		textField(t, "dstHost", core.SemanticKeyConnectionDestinationHostname, "example.test"),
	})
	identity := graph.Nodes[0].Identity()
	if len(identity) != 1 || identity[0].Semantic != core.SemanticKeyConnectionDestinationHostname {
		t.Fatalf("the hostname identity is %+v, want the destination hostname semantic", identity)
	}
}

func TestNodeKeyValidateSeparatesTheCompleteKeyFromTheBrokenOne(t *testing.T) {
	complete := core.NodeKey{
		Kind: core.NodeKindProcess, Form: core.NodeKeyFormTerminalProcess,
		Values: []core.NodeIdentityValue{
			{Semantic: core.SemanticKeyTerminalId, Value: "T1"},
			{Semantic: core.SemanticKeyProcessId, Value: "{P1}"},
		},
	}
	if err := complete.Validate(); err != nil {
		t.Errorf("a complete process key did not validate: %v", err)
	}
	for _, testCase := range []struct {
		name string
		key  core.NodeKey
	}{
		{"without a value", core.NodeKey{
			Kind: core.NodeKindProcess, Form: core.NodeKeyFormTerminalProcess,
		}},
		{"of an unknown kind", core.NodeKey{
			Kind: "session", Form: core.NodeKeyFormTerminalProcess, Values: complete.Values,
		}},
		{"of an unknown key form", core.NodeKey{
			Kind: core.NodeKindProcess, Form: "process_id", Values: complete.Values,
		}},
		{"carrying an empty value", core.NodeKey{
			Kind: core.NodeKindTerminal, Form: core.NodeKeyFormTerminalId,
			Values: []core.NodeIdentityValue{{Semantic: core.SemanticKeyTerminalId}},
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if err := testCase.key.Validate(); err == nil {
				t.Fatalf("a key %s validated, want an error", testCase.name)
			}
		})
	}
}

// LabelSemanticOf は語彙を走査して 1 件を返す。同じ対象に label の役割の項目が 2 つ以上
// あると、返す値が走査の順に依存する。
func TestLabelSemanticOfReturnsOneItemPerNodeKind(t *testing.T) {
	for _, testCase := range []struct {
		kind core.NodeKind
		want core.SemanticKey
	}{
		{core.NodeKindTerminal, core.SemanticKeyTerminalHostname},
		{core.NodeKindProcess, core.SemanticKeyProcessBinaryPath},
		{core.NodeKindFile, core.SemanticKeyFileName},
		{core.NodeKindRegistryValue, core.SemanticKeyRegistryValueName},
		{core.NodeKindAccount, ""},
		{core.NodeKindIp, ""},
		{core.NodeKindDomain, ""},
	} {
		t.Run(string(testCase.kind), func(t *testing.T) {
			for repeat := range 20 {
				semantic, found := core.LabelSemanticOf(testCase.kind)
				if testCase.want == "" {
					if found {
						t.Fatalf("run %d returned the label item %q, want none", repeat, semantic)
					}
					continue
				}
				if !found || semantic != testCase.want {
					t.Fatalf("run %d returned %q (found %v), want %q",
						repeat, semantic, found, testCase.want)
				}
			}
		})
	}
}

// ノードを作らない対象の項目は、どのノードの種別にも対応しない。
func TestNodeKindOfSeparatesTheObjectsThatBecomeNodes(t *testing.T) {
	for _, object := range []core.SemanticObject{
		core.SemanticObjectTerminal, core.SemanticObjectProcess, core.SemanticObjectFile,
		core.SemanticObjectRegistryValue, core.SemanticObjectAccount, core.SemanticObjectIp,
		core.SemanticObjectDomain, core.SemanticObjectRecord,
	} {
		kind, isNode := core.NodeKindOf(object)
		if !isNode || string(kind) != string(object) {
			t.Errorf("the object %q maps to %q (isNode %v), want the node kind of the same text",
				object, kind, isNode)
		}
	}
	for _, object := range []core.SemanticObject{
		core.SemanticObjectConnection, core.SemanticObjectEvent, core.SemanticObjectHttp,
	} {
		if kind, isNode := core.NodeKindOf(object); isNode {
			t.Errorf("the object %q maps to the node kind %q, want no node kind", object, kind)
		}
	}
}

func nodeKinds(keys []core.NodeKey) []string {
	kinds := make([]string, 0, len(keys))
	for _, key := range keys {
		kinds = append(kinds, string(key.Kind))
	}
	return kinds
}

// addressNodeKey は IP アドレスのノードの識別鍵を組む。
func addressNodeKey(value string) core.NodeKey {
	return core.NodeKey{
		Kind: core.NodeKindIp, Form: core.NodeKeyFormAddress,
		Values: []core.NodeIdentityValue{{Value: value}},
	}
}

// アドレスを読めるかは、鍵の形と原資料の文字列の両方で決まる。
func TestNodeKeyAddressValueReadsTheAddressKeyForm(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		key          core.NodeKey
		want         string
		wantReadable bool
	}{
		{"an IPv4 address", addressNodeKey("192.0.2.10"), "192.0.2.10", true},
		{"an IPv6 address", addressNodeKey("2001:db8::1"), "2001:db8::1", true},
		{
			"an IPv4 written in the IPv6 form", addressNodeKey("::ffff:192.0.2.10"),
			"192.0.2.10", true,
		},
		{"a text outside the address syntax", addressNodeKey("192.0.2.300"), "", false},
		{
			"an address key form without a value",
			core.NodeKey{Kind: core.NodeKindIp, Form: core.NodeKeyFormAddress}, "", false,
		},
		{"a hostname key form", core.NodeKey{
			Kind: core.NodeKindDomain, Form: core.NodeKeyFormHostname,
			Values: []core.NodeIdentityValue{{Value: "host.example.test"}},
		}, "", false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			address, readable := testCase.key.AddressValue()
			if readable != testCase.wantReadable {
				t.Fatalf("the key is readable as an address: %v, want %v",
					readable, testCase.wantReadable)
			}
			if !readable {
				return
			}
			if address.String() != testCase.want {
				t.Errorf("the key carries the address %q, want %q", address, testCase.want)
			}
		})
	}
}
