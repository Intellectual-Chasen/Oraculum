package pipeline

import (
	"regexp"

	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/prefetch"
	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/winevent"
	"github.com/Intellectual-Chasen/Oraculum/backend/adapters/winregistry"
	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// 分類の対象にする registry の key。path は hive の root の key の名前を含まない。
var (
	amcacheFileKeyPath   = regexp.MustCompile(`(?i)^\\Root\\InventoryApplicationFile\\`)
	taskCacheTaskKeyPath = regexp.MustCompile(`(?i)^\\Microsoft\\Windows NT\\CurrentVersion\\Schedule\\TaskCache\\Tasks\\`)
)

func init() {
	classifyTerminalRecord = terminalRecordClassificationOf
	terminalCategorySources = winevent.TerminalCategorySources
}

// terminalRecordClassificationOf は、入力形式 format のレコードの分類と収集元の種類を返す。
func terminalRecordClassificationOf(format core.FormatKey, record RecordEntry) terminalRecordClassification {
	if record.Semantics == nil {
		return terminalRecordClassification{}
	}
	fields := record.Semantics.Fields
	var input winevent.TerminalRecord
	var kind string
	switch format {
	case prefetch.FormatKeyPrefetch:
		input.Artifact, kind = winevent.ArtifactPrefetch, winevent.ArtifactPrefetch
	case winregistry.FormatKeyHive:
		path, _ := rawValueOf(fields, "KeyPath")
		switch {
		case amcacheFileKeyPath.MatchString(path):
			input.Artifact = winevent.ArtifactAmcache
		case taskCacheTaskKeyPath.MatchString(path):
			input.Artifact = winevent.ArtifactTaskCache
		default:
			return terminalRecordClassification{}
		}
		kind = input.Artifact
	case winevent.FormatKeyXML, winevent.FormatKeyViewerCSV, winevent.FormatKeyEVTX:
		input.Provider, _ = comparableOfSemantic(fields, core.SemanticKeyWindowsEventProvider)
		input.EventID, _ = comparableOfSemantic(fields, core.SemanticKeyWindowsEventId)
		input.LogonType, _ = comparableOfSemantic(fields, core.SemanticKeyEventLogonType)
		kind, _ = comparableOfSemantic(fields, core.SemanticKeyWindowsEventChannel)
	default:
		return terminalRecordClassification{}
	}
	classified := winevent.ClassifyTerminalRecord(input)
	return terminalRecordClassification{
		categories: classified.Categories, logonOutcome: classified.LogonOutcome, sourceKind: kind,
	}
}
