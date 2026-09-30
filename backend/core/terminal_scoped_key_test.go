package core_test

import (
	"reflect"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 内容の識別。原資料の file の値ではない。
const scopedKeySha = "0000000000000000000000000000000000000000000000000000000000000001"

func scopedKeyField(t *testing.T, name string, semantic core.SemanticKey, text string) core.RecordField {
	t.Helper()
	value, err := core.NewRawValue(core.ValueStatePresent, text)
	if err != nil {
		t.Fatal(err)
	}
	field, err := core.NewTextField(name, semantic, value)
	if err != nil {
		t.Fatal(err)
	}
	return field
}

// 値が 1 つの端末の鍵から組む、端末の範囲の対象の鍵は、端末の鍵の値 1 つと対象の値の組である。
func TestTerminalScopedKeysOfASingleValueTerminal(t *testing.T) {
	recording, _ := core.RecordingTerminalNodeKey(scopedKeySha)
	process, built := core.NewProcessIntervalNodeKey(recording, "42", "2001-02-03T04:05:06Z")
	want := []core.NodeIdentityValue{
		{Value: scopedKeySha},
		{Semantic: core.SemanticKeyProcessPid, Value: "42"},
		{Value: "2001-02-03T04:05:06Z"},
	}
	if !built || !reflect.DeepEqual(process.Values, want) {
		t.Errorf("process interval key = %+v, want %+v", process.Values, want)
	}

	fields := []core.RecordField{
		scopedKeyField(t, "tmid", core.SemanticKeyTerminalId, "T1"),
		scopedKeyField(t, "guid", core.SemanticKeyProcessId, "P1"),
		scopedKeyField(t, "user", core.SemanticKeyAccountName, "operator"),
	}
	processKey, _ := core.ProcessNodeKey(fields)
	wantProcess := []core.NodeIdentityValue{
		{Semantic: core.SemanticKeyTerminalId, Value: "T1"},
		{Semantic: core.SemanticKeyProcessId, Value: "P1"},
	}
	if !reflect.DeepEqual(processKey.Values, wantProcess) {
		t.Errorf("process key = %+v, want %+v", processKey.Values, wantProcess)
	}
	account, _ := core.AccountNodeKey(fields[2:], core.RecordScope{Terminal: &recording, AccountNamesLocal: true})
	wantAccount := []core.NodeIdentityValue{
		{Value: scopedKeySha}, {Semantic: core.SemanticKeyAccountName, Value: "operator"},
	}
	if !reflect.DeepEqual(account.Values, wantAccount) {
		t.Errorf("account key = %+v, want %+v", account.Values, wantAccount)
	}
}

// 内容の識別とホスト名の組で指す端末では、端末の範囲の対象の鍵が端末の 2 つの値を両方持つ。
// 同じ収集元の別のホスト名の対象は別の鍵になる。
func TestTerminalScopedKeysOfAHostnameTerminal(t *testing.T) {
	hostA, _ := core.RecordingHostTerminalNodeKey(scopedKeySha, "host-a.example.test")
	hostB, _ := core.RecordingHostTerminalNodeKey(scopedKeySha, "host-b.example.test")
	if hostA.Form != core.NodeKeyFormRecordingSourceHostname || hostA.Validate() != nil {
		t.Fatalf("terminal key = %+v, want a valid %q key", hostA, core.NodeKeyFormRecordingSourceHostname)
	}
	processA, _ := core.NewProcessIntervalNodeKey(hostA, "42", "2001-02-03T04:05:06Z")
	processB, _ := core.NewProcessIntervalNodeKey(hostB, "42", "2001-02-03T04:05:06Z")
	if !reflect.DeepEqual(processA.Values[:len(hostA.Values)], hostA.Values) {
		t.Errorf("process key %+v does not start with the terminal values %+v", processA.Values, hostA.Values)
	}
	if reflect.DeepEqual(processA.DigestParts(), processB.DigestParts()) {
		t.Error("the processes of two hostnames in one source share a key")
	}
	fields := []core.RecordField{scopedKeyField(t, "path", core.SemanticKeyFilePath, `C:\Example\a.txt`)}
	filesA := core.OperatedFileNodeKeys(fields, core.RecordScope{Terminal: &hostA})
	filesB := core.OperatedFileNodeKeys(fields, core.RecordScope{Terminal: &hostB})
	if len(filesA) != 1 || len(filesB) != 1 || reflect.DeepEqual(filesA[0].DigestParts(), filesB[0].DigestParts()) {
		t.Errorf("file keys = %+v and %+v, want one distinct key per hostname", filesA, filesB)
	}
}
