package pipeline

import (
	"fmt"
	"strconv"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// invariantBreak は公開を止める理由と収集元を持つ。runWide は実行全体を対象にする。
type invariantBreak struct {
	sourceIndex int
	runWide     bool
	reason      core.WithheldReason
}

// checkIdentifierCollision は assigned[i] を sources[i] の識別子として衝突を検査する。
func checkIdentifierCollision(sources []scannedSource, assigned []string) ([]invariantBreak, error) {
	if len(sources) != len(assigned) {
		return nil, fmt.Errorf("checking identifier collisions: sources and assigned IDs differ in length")
	}
	var broken []invariantBreak
	seen := make(map[string]bool, len(assigned))
	for i, id := range assigned {
		if seen[id] {
			broken = append(broken, invariantBreak{runWide: true, reason: core.WithheldReasonIdentifierCollision})
		}
		seen[id] = true
		source := sources[i]
		positions := make(map[string]bool, len(source.Records)+len(source.Failures))
		takePosition := func(locator core.RecordLocator) {
			for _, key := range locatorPositionKeys(locator) {
				if positions[key] {
					broken = append(broken, invariantBreak{
						sourceIndex: i, reason: core.WithheldReasonIdentifierCollision,
					})
				}
				positions[key] = true
			}
		}
		for _, record := range source.Records {
			takePosition(record.Locator)
		}
		for _, failed := range source.Failures {
			// 取り込んだレコードと同じ位置を名乗る失敗は、同じ 1 行の記録である。
			if failed.Failure.RecordRef == nil || failed.Kept {
				continue
			}
			takePosition(*failed.Failure.RecordRef)
		}
	}
	return broken, nil
}

// locatorPositionKeys は 1 レコードが名乗る位置を、指し方ごとの鍵へ直す。
//
// **索引で探せる位置をすべて確かめる** (candidate_index.go の positionKeysOf と同じ表)。
// 主の位置だけを見ると、byte 位置が異なり先頭の行番号が同じ 2 レコードが衝突の検査を
// すり抜け、行番号で探した要求が先に入れた 1 件だけを返す。
func locatorPositionKeys(locator core.RecordLocator) []string {
	keys := []string{locatorPositionKey(locator)}
	if locator.PositionKind != core.PositionKindLineNumber && locator.LineNumber != nil {
		keys = append(keys, positionKeyText(core.PositionKindLineNumber, locator.LineNumber))
	}
	return keys
}

// locatorPositionKey はレコードの主の位置を鍵へ直す。
// レコードの原文と根拠の位置を探す鍵はこちらを使う。
func locatorPositionKey(locator core.RecordLocator) string {
	return positionKeyText(locator.PositionKind, locatorPosition(&locator))
}

func positionKeyText(kind core.PositionKind, position *int64) string {
	if position == nil {
		return string(kind) + ":absent"
	}
	return string(kind) + ":" + strconv.FormatInt(*position, 10)
}

func checkEvidenceReferences(sources []scannedSource, statuses []core.ImportStatus) []invariantBreak {
	var broken []invariantBreak
	for i, status := range statuses {
		hash := sources[i].Measurement.ContentSha256
		for _, failure := range status.Failures {
			if failure.RecordRef == nil && failure.RawTextRef == "" {
				continue
			}
			valid := false
			if failure.RecordRef != nil {
				ref := *failure.RecordRef
				valid = ref.SourceId == status.SourceId &&
					ref.SourceContentSha256 == hash &&
					ref.RecordRawTextRef != "" && failure.RawTextRef == ref.RecordRawTextRef
			}
			if !valid {
				broken = append(broken, invariantBreak{sourceIndex: i, reason: core.WithheldReasonDanglingEvidenceReference})
			}
		}
	}
	return broken
}

func checkAnalysisRun(statuses []core.ImportStatus, runRef string) []invariantBreak {
	for _, status := range statuses {
		if status.AnalysisRunRef != runRef {
			return []invariantBreak{{runWide: true, reason: core.WithheldReasonMixedAnalysisRun}}
		}
	}
	return nil
}

// decidePublicationState は収集元に適用される破れと走査結果から公開状態を決める。
// 位置を確定できない失敗だけでも、失敗が1件以上あれば published_partial にする。
func decidePublicationState(s scannedSource, broken []invariantBreak) (core.PublicationState, core.WithheldReason) {
	if len(broken) > 0 {
		return core.PublicationStateWithheld, broken[0].reason
	}
	if s.FailureCount > 0 || len(s.Failures) > 0 || s.ReadStopped {
		return core.PublicationStatePublishedPartial, ""
	}
	return core.PublicationStatePublishedFull, ""
}
