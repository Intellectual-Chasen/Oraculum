package pipeline

import (
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 導入先の値が 1 つで、その System32 を参照したボリュームが 1 つの収集では、Prefetch の path を
// ドライブ文字へ直す。導入先が 2 つの収集では直さない。
func TestMapCollectionVolumesRewritesTheSystemVolume(t *testing.T) {
	prefetchSource := func() scannedSource {
		return scannedSource{
			Plan: SourcePlan{FormatKey: WindowsPrefetchFormatKey},
			Records: []RecordEntry{{Semantics: &RecordSemantics{Fields: []core.RecordField{
				syntheticField(t, "ExecutablePath", core.SemanticKeyFilePath, `\VOLUME{71}\T\TOOL71.EXE`),
				syntheticField(t, "ReferencedFile", core.SemanticKeyFilePath, `\VOLUME{71}\SYNTHWIN\SYSTEM32\A.DLL`),
				syntheticField(t, "Volume.DevicePath", "", `\VOLUME{71}`),
			}}}},
		}
	}
	registrySource := func(root string) scannedSource {
		return scannedSource{
			Plan: SourcePlan{FormatKey: WindowsRegistryHiveFormatKey},
			Records: []RecordEntry{{Semantics: &RecordSemantics{Fields: []core.RecordField{
				syntheticField(t, "KeyPath", "", `\Microsoft\Windows NT\CurrentVersion`),
				syntheticField(t, "Value.SystemRoot", "", root),
			}}}},
		}
	}
	executableOf := func(source scannedSource) string {
		value, _ := comparableOfName(source.Records[0].Semantics.Fields, "ExecutablePath")
		return value
	}

	mapped := []scannedSource{prefetchSource(), registrySource(`Y:\SynthWin`)}
	mapCollectionVolumes(mapped)
	if got := executableOf(mapped[0]); got != `Y:\T\TOOL71.EXE` {
		t.Errorf("the executable path is %q, want the drive letter of the system root", got)
	}
	ambiguous := []scannedSource{prefetchSource(), registrySource(`Y:\SynthWin`), registrySource(`Z:\SynthWin`)}
	mapCollectionVolumes(ambiguous)
	if got := executableOf(ambiguous[0]); got != `\VOLUME{71}\T\TOOL71.EXE` {
		t.Errorf("two system roots rewrote the path to %q", got)
	}

	// 導入先の異なる 2 つの収集は、それぞれの導入先のドライブ文字へ直す。
	inCollection := func(source scannedSource, path string) scannedSource {
		source.Plan.CollectionPath = path
		return source
	}
	two := []scannedSource{
		inCollection(prefetchSource(), "c1"), inCollection(registrySource(`Y:\SynthWin`), "c1"),
		inCollection(prefetchSource(), "c2"), inCollection(registrySource(`Z:\SynthWin`), "c2"),
	}
	mapCollectionVolumes(two)
	if got := executableOf(two[0]); got != `Y:\T\TOOL71.EXE` {
		t.Errorf("the first collection path is %q, want its own system root drive", got)
	}
	if got := executableOf(two[2]); got != `Z:\T\TOOL71.EXE` {
		t.Errorf("the second collection path is %q, want its own system root drive", got)
	}
}
