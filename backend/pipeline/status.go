package pipeline

import (
	"fmt"
	"slices"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// buildImportStatus は診断と識別と件数を完成させ、整合を検査する。
// 位置付き失敗の原文参照は rawTextRef が供給する。取り込みの失敗を全件含む。
func buildImportStatus(s scannedSource, runRef, parserVersion, sourceId string,
	sanitize func(string) string, rawTextRef func(core.RecordLocator, string) string,
) (core.ImportStatus, error) {
	if sanitize == nil {
		return core.ImportStatus{}, fmt.Errorf("building import status: sanitize is required")
	}
	status := core.ImportStatus{
		SourceId: sourceId, Scope: s.Scope, Counts: s.Counts,
		DiagnosisCounts: slices.Clone(s.DiagnosisCounts),
		Failures:        make([]core.ImportFailure, len(s.Failures)), FailureCount: s.FailureCount,
		AnalysisRunRef: runRef,
	}
	status.Scope.SourceId = sourceId
	status.Scope.SourceContentSha256 = s.Measurement.ContentSha256
	status.PublicationState, status.WithheldReason = decidePublicationState(s, nil)
	for i, failure := range s.Failures {
		completed, err := completeImportFailure(failure, sourceId, s.Measurement.ContentSha256, parserVersion, sanitize, rawTextRef)
		if err != nil {
			return core.ImportStatus{}, fmt.Errorf("completing import failure %d: %w", i, err)
		}
		status.Failures[i] = completed
	}
	if err := status.Validate(); err != nil {
		return core.ImportStatus{}, fmt.Errorf("validating completed import status: %w", err)
	}
	return cloneStatus(status), nil
}

func completeImportFailure(failed failedRecord, sourceId, hash, parserVersion string,
	sanitize func(string) string, rawTextRef func(core.RecordLocator, string) string,
) (core.ImportFailure, error) {
	failure := failed.Failure
	if (failure.RecordRef != nil) != (failed.RawText != nil) {
		return core.ImportFailure{}, fmt.Errorf("failure raw text and record reference must be present together")
	}
	if failure.Stage == core.FailureStageRead && (failure.RecordRef != nil || failure.RawTextRef != "") {
		return core.ImportFailure{}, fmt.Errorf("read failure must omit record and raw text references")
	}
	failure = cloneFailure(failure)
	failure.SourceId, failure.SourceContentSha256, failure.ParserVersion = sourceId, hash, parserVersion
	failure.SanitizedMessage = sanitize(fmt.Sprintf("Import failure at stage %s; diagnosis %s.", failure.Stage, failure.DiagnosisClass))
	if failure.RecordRef != nil {
		locator, err := completeRecordLocator(*failure.RecordRef, sourceId, hash, *failed.RawText, rawTextRef)
		if err != nil {
			return core.ImportFailure{}, fmt.Errorf("completing failure locator: %w", err)
		}
		failure.RecordRef = &locator
		failure.RawTextRef = locator.RecordRawTextRef
	}
	return failure, nil
}

func completeRecordLocator(locator core.RecordLocator, sourceId, hash, rawText string,
	rawTextRef func(core.RecordLocator, string) string,
) (core.RecordLocator, error) {
	locator = cloneLocator(locator)
	if locator.SourceId == "" {
		locator.SourceId = sourceId
	}
	if locator.SourceContentSha256 == "" {
		locator.SourceContentSha256 = hash
	}
	if locator.SourceId != sourceId || locator.SourceContentSha256 != hash {
		return core.RecordLocator{}, fmt.Errorf("locator source identity differs from assigned identity")
	}
	if rawTextRef == nil {
		return core.RecordLocator{}, fmt.Errorf("rawTextRef supplier is required for a positioned record")
	}
	locator.RecordRawTextRef = rawTextRef(cloneLocator(locator), rawText)
	if locator.RecordRawTextRef == "" {
		return core.RecordLocator{}, fmt.Errorf("rawTextRef supplier returned an empty reference")
	}
	if err := locator.Validate(); err != nil {
		return core.RecordLocator{}, fmt.Errorf("validating completed locator: %w", err)
	}
	return locator, nil
}
