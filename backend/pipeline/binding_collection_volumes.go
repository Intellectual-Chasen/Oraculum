package pipeline

import (
	"regexp"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/prefetch"
	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/winregistry"
)

func init() {
	scannedSourcesAdjusters = append(scannedSourcesAdjusters, mapCollectionVolumes)
}

// systemRootPattern は、ドライブ文字で始まる Windows の導入先の path である。
var systemRootPattern = regexp.MustCompile(`^([A-Za-z]:)(\\[^\\]+(?:\\[^\\]+)*)\\?$`)

// mapCollectionVolumes は、Prefetch の file が記録したボリュームのデバイスの path を、同じ収集の
// registry の SystemRoot のドライブ文字へ直す。直した path は file.path の項目の正規化値になり、
// ファイルのノードの鍵に入る (prefetch.MapVolume)。
//
// **直すのは、導入先の値が 1 つに決まり、導入先の System32 の下の file を参照したボリュームが
// 1 つだけのときである。** ほかの収集では path をデバイスの path のまま残す。
//
// 収集の範囲は、案件と収集の directory (SourcePlan.CollectionPath) の組である。収集の directory を
// 持たない収集元は、案件ごとに 1 つの収集として扱う。
func mapCollectionVolumes(sources []scannedSource) {
	byCollection := make(map[[2]string][]int)
	for index, source := range sources {
		caseId := ""
		if source.Plan.CaseId != nil {
			caseId = *source.Plan.CaseId
		}
		key := [2]string{caseId, source.Plan.CollectionPath}
		byCollection[key] = append(byCollection[key], index)
	}
	for _, members := range byCollection {
		mapVolumesOf(sources, members)
	}
}

// mapVolumesOf は、members の収集元を 1 つの収集として、ボリュームのデバイスの path を直す。
func mapVolumesOf(sources []scannedSource, members []int) {
	roots := make(map[string]struct{})
	for _, index := range members {
		if sources[index].Plan.FormatKey != winregistry.FormatKeyHive {
			continue
		}
		for _, record := range sources[index].Records {
			if record.Semantics == nil {
				continue
			}
			if root, found := winregistry.SystemRootOf(record.Semantics.Fields); found {
				roots[strings.ToUpper(root)] = struct{}{}
			}
		}
	}
	if len(roots) != 1 {
		return
	}
	var drive, directory string
	for root := range roots {
		parts := systemRootPattern.FindStringSubmatch(root)
		if parts == nil {
			return
		}
		drive, directory = parts[1], parts[2]+`\SYSTEM32\`
	}
	devices := make(map[string]string)
	for _, index := range members {
		if sources[index].Plan.FormatKey != prefetch.FormatKeyPrefetch {
			continue
		}
		for _, record := range sources[index].Records {
			if record.Semantics == nil {
				continue
			}
			for _, device := range prefetch.VolumesUnder(record.Semantics.Fields, directory) {
				devices[strings.ToUpper(device)] = device
			}
		}
	}
	if len(devices) != 1 {
		return
	}
	for _, device := range devices {
		for _, index := range members {
			if sources[index].Plan.FormatKey != prefetch.FormatKeyPrefetch {
				continue
			}
			for _, record := range sources[index].Records {
				if record.Semantics != nil {
					prefetch.MapVolume(record.Semantics.Fields, device, drive)
				}
			}
		}
	}
}
