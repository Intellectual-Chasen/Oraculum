// in-package test: 読んだ値を項目と語彙へ直す。
package prefetch

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

func fieldNamed(t *testing.T, fields []core.RecordField, name string) core.RecordField {
	t.Helper()
	for _, field := range fields {
		if field.Name == name {
			return field
		}
	}
	t.Fatalf("no field named %q", name)
	return core.RecordField{}
}

func hasField(fields []core.RecordField, name string) bool {
	for _, field := range fields {
		if field.Name == name {
			return true
		}
	}
	return false
}

// 最新の実行時刻は event.time、実行ファイルと参照した file の path は file.path の語彙を持つ。
// 2 番目以降の実行時刻と実行回数は語彙を持たず、2 番目以降の実行時刻は AdditionalRunTimes が返す。
func TestFieldsMapTheLatestRunAndTheExecutable(t *testing.T) {
	file, err := parseFile(sampleFile(30, 0x130).build())
	if err != nil {
		t.Fatal(err)
	}
	fields := Fields(file)
	latest := fieldNamed(t, fields, "LastRunTime")
	if latest.Semantic != core.SemanticKeyEventTime || latest.Timestamp == nil ||
		*latest.Timestamp.Normalized != "2006-08-14T03:33:20.000000Z" ||
		latest.Timestamp.Clock != core.ClockTerminalLocal || latest.Timestamp.OffsetState != core.OffsetStateEpoch {
		t.Errorf("LastRunTime = %+v", latest)
	}
	if second := fieldNamed(t, fields, "LastRunTime#2"); second.Semantic != "" || second.Timestamp == nil {
		t.Errorf("LastRunTime#2 = %+v", second)
	}
	if hasField(fields, "LastRunTime#7") {
		t.Error("an empty slot became a field")
	}
	for name, want := range map[string]string{
		"ExecutablePath": `\VOLUME{00000000000000a1-0000b2c3}\DATA\TOOL51.EXE`,
		"RunCount":       "7", "PathHash": "0A0B0C0D", "FormatVersion": "30", "Compressed": "false",
		"ReferencedFile#2": `\VOLUME{00000000000000a1-0000b2c3}\DATA\TOOL51.EXE`,
		"Volume.Serial":    "B2C3D4E5",
	} {
		if value, _ := fieldNamed(t, fields, name).Text.ComparableValue(); value != want {
			t.Errorf("%s = %q, want %q", name, value, want)
		}
	}
	if fieldNamed(t, fields, "ExecutablePath").Semantic != core.SemanticKeyFilePath ||
		fieldNamed(t, fields, "ReferencedFile").Semantic != core.SemanticKeyFilePath {
		t.Error("the executable path or a referenced file is not file.path")
	}
	additional := AdditionalRunTimes(fields)
	if len(additional) == 0 || additional[0].Name != "LastRunTime#2" ||
		*additional[0].Timestamp.Normalized != *fieldNamed(t, fields, "LastRunTime#2").Timestamp.Normalized {
		t.Errorf("AdditionalRunTimes() = %+v, want the runs after the latest from LastRunTime#2", additional)
	}
	for _, field := range additional {
		if field.Name == "LastRunTime" {
			t.Error("AdditionalRunTimes() returns the latest run")
		}
	}
	if at := LastRunTime(file); at == nil || *at.Normalized != "2006-08-14T03:33:20.000000Z" {
		t.Errorf("LastRunTime() = %+v", at)
	}
}

// 実行ファイルの名前は 29 文字で切れるため前方一致で比べる。候補が 2 つある file は、最初の
// 候補を原資料の文字列で残し、語彙を付けない。
func TestFieldsMatchTheExecutableByItsPrefix(t *testing.T) {
	truncated := File{ExecutableName: "LONGNAMEDTOOL-WITH-A-VERSION-", ReferencedFiles: []string{
		`\VOLUME{01}\A\NTDLL.DLL`, `\VOLUME{01}\A\LONGNAMEDTOOL-WITH-A-VERSION-2.EXE`,
	}}
	path := fieldNamed(t, Fields(truncated), "ExecutablePath")
	if value, _ := path.Text.ComparableValue(); path.Semantic != core.SemanticKeyFilePath ||
		value != `\VOLUME{01}\A\LONGNAMEDTOOL-WITH-A-VERSION-2.EXE` {
		t.Errorf("the truncated name matched %+v", path)
	}
	withResources := File{ExecutableName: "TOOL55.EXE", ReferencedFiles: []string{
		`\VOLUME{01}\A\EN-US\TOOL55.EXE.MUI`, `\VOLUME{01}\A\TOOL55.EXE`,
	}}
	path = fieldNamed(t, Fields(withResources), "ExecutablePath")
	if value, _ := path.Text.ComparableValue(); path.Semantic != core.SemanticKeyFilePath || value != `\VOLUME{01}\A\TOOL55.EXE` {
		t.Errorf("a name that fits the field matched %+v, want the exact name alone", path)
	}
	twice := File{ExecutableName: "TOOL52.EXE", ReferencedFiles: []string{`\VOLUME{01}\A\TOOL52.EXE`, `\VOLUME{01}\B\TOOL52.EXE`}}
	path = fieldNamed(t, Fields(twice), "ExecutablePath")
	if value, _ := path.Text.ComparableValue(); path.Semantic != "" || value != `\VOLUME{01}\A\TOOL52.EXE` {
		t.Errorf("two candidates gave %+v, want the first without a semantic", path)
	}
	if hasField(Fields(File{ExecutableName: "TOOL53.EXE"}), "ExecutablePath") {
		t.Error("a file without the executable in its references has an executable path")
	}
}

// System32 の下の file を参照したボリュームだけを返し、そのボリュームの path だけにドライブ文字の
// 正規化値を持たせる。原資料の文字列は変えない。
func TestVolumesUnderAndMapVolumeRewriteTheSystemVolume(t *testing.T) {
	fields := Fields(File{
		ExecutableName: "TOOL55.EXE",
		ReferencedFiles: []string{
			`\VOLUME{01}\WINDOWS\SYSTEM32\NTDLL.DLL`, `\VOLUME{01}\Tools\TOOL55.EXE`, `\VOLUME{02}\DATA\X.DAT`,
		},
		Volumes: []Volume{{DevicePath: `\VOLUME{01}`}, {DevicePath: `\VOLUME{02}`}},
	})
	devices := VolumesUnder(fields, `\windows\system32\`)
	if len(devices) != 1 || devices[0] != `\VOLUME{01}` {
		t.Fatalf("VolumesUnder() = %v, want the system volume alone", devices)
	}
	MapVolume(fields, `\volume{01}`, "Y:")
	for name, want := range map[string]string{
		"ExecutablePath":   `Y:\Tools\TOOL55.EXE`,
		"ReferencedFile":   `Y:\WINDOWS\SYSTEM32\NTDLL.DLL`,
		"ReferencedFile#3": `\VOLUME{02}\DATA\X.DAT`,
	} {
		field := fieldNamed(t, fields, name)
		if value, _ := field.Text.ComparableValue(); value != want {
			t.Errorf("%s = %q, want %q", name, value, want)
		}
	}
	if raw, _ := fieldNamed(t, fields, "ReferencedFile").Text.RawTextValue(); raw != `\VOLUME{01}\WINDOWS\SYSTEM32\NTDLL.DLL` {
		t.Errorf("the raw text became %q", raw)
	}
}

// FILETIME の 100 ナノ秒の端数は、マイクロ秒へ切り捨てる。
func TestFieldsTruncateTheFileTimeToMicroseconds(t *testing.T) {
	for value, want := range map[uint64]string{
		128000000000000009: "2006-08-14T03:33:20.000000Z",
		128000000000000019: "2006-08-14T03:33:20.000001Z",
	} {
		if at := LastRunTime(File{LastRuns: []uint64{value}}); at == nil || *at.Normalized != want {
			t.Errorf("%d: %+v, want %s", value, at, want)
		}
	}
}

// 位置を決められなかった実行回数は導けなかった値になる。時刻として書けない FILETIME は原資料の文字列の項目になる。
func TestFieldsKeepUnreadableValues(t *testing.T) {
	fields := Fields(File{ExecutableName: "TOOL54.EXE", LastRuns: []uint64{^uint64(0)}})
	if count := fieldNamed(t, fields, "RunCount"); count.Text.ValueState != core.ValueStateDerivationUndetermined {
		t.Errorf("RunCount = %+v", count)
	}
	if latest := fieldNamed(t, fields, "LastRunTime"); latest.Timestamp != nil || latest.Semantic != "" {
		t.Errorf("LastRunTime = %+v, want the raw number without a semantic", latest)
	}
	if LastRunTime(File{LastRuns: []uint64{^uint64(0)}}) != nil || LastRunTime(File{}) != nil {
		t.Error("an unreadable or absent run time gave a record time")
	}
}
