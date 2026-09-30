// in-package test: 非公開の構築子を通して意味付けの結果の複製と完成を検査する。
package pipeline

import (
	"strings"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

// markiiSettleLine は接続の 4 項目とプロセスの識別を持つ通信のレコードである。
const markiiSettleLine = "01/02/2024 03:04:05.006 +0000 sn=7 evt=net subEvt=con " +
	"psGUID=p tmid=t com=c csid=s psPath=app srcIP=192.0.2.10 srcPort=50002 " +
	"dstIP=198.51.100.42 dstPort=80\n"

func settleScannedMarkII(t *testing.T, input string) scannedSource {
	t.Helper()
	measurement, err := measureWholeSource(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	source, err := scanSource(NewTestMarkIIParser(), strings.NewReader(input), measurement,
		SourcePlan{FileName: "synthetic.log", FormatKey: MarkIIFormatKey})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func TestScanFillsProcessContentHashAndLeavesSourceId(t *testing.T) {
	source := settleScannedMarkII(t, markiiSettleLine)
	if len(source.Records) != 1 {
		t.Fatalf("record count = %d, want 1", len(source.Records))
	}
	reference := source.Records[0].Semantics.ProcessRef
	if reference == nil {
		t.Fatal("the scan dropped the process reference")
	}
	if reference.SourceContentSha256 != source.Measurement.ContentSha256 {
		t.Errorf("process sourceContentSha256 = %q, want %q",
			reference.SourceContentSha256, source.Measurement.ContentSha256)
	}
	if reference.SourceId != "" {
		t.Errorf("the scan prefilled the process sourceId: %q", reference.SourceId)
	}
}

func TestPublicationCompletesProcessReference(t *testing.T) {
	source := settleScannedMarkII(t, markiiSettleLine)
	status := settleStatus(t, source, "one")
	result, err := newImportResult([]scannedSource{source}, []core.ImportStatus{status},
		"run", settleRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	publication, ok := result.Publication("one")
	if !ok {
		t.Fatal("publication missing")
	}
	records := publication.Records()
	if len(records) != 1 || records[0].Semantics == nil {
		t.Fatalf("published records = %+v", records)
	}
	reference := records[0].Semantics.ProcessRef
	if reference == nil {
		t.Fatal("the publication dropped the process reference")
	}
	if reference.SourceId != "one" {
		t.Errorf("process sourceId = %q, want %q", reference.SourceId, "one")
	}
	if err := reference.Validate(); err != nil {
		t.Errorf("the completed process reference did not validate: %v", err)
	}
}

func TestPublishedRecordsCopySemantics(t *testing.T) {
	source := settleScannedMarkII(t, markiiSettleLine)
	status := settleStatus(t, source, "one")
	result, err := newImportResult([]scannedSource{source}, []core.ImportStatus{status},
		"run", settleRawTextRef)
	if err != nil {
		t.Fatal(err)
	}
	publication, ok := result.Publication("one")
	if !ok {
		t.Fatal("publication missing")
	}
	semantics := publication.Records()[0].Semantics
	*semantics.Endpoint.Destination[0].Text.RawText = "changed"
	*semantics.Fields[0].Timestamp.RawText = "changed"
	*semantics.ObservationKind.Raw[0].Text.RawText = "changed"
	semantics.ProcessRef.ProcessId = "changed"
	source.Records[0].Semantics.Endpoint.Destination[1].Text.RawText = nil

	got := publication.Records()[0].Semantics
	if value, ok := got.Endpoint.Destination[0].Text.RawTextValue(); !ok ||
		value != "198.51.100.42" {
		t.Errorf("the destination address mutated: %q %v", value, ok)
	}
	if value, ok := got.Endpoint.Destination[1].Text.RawTextValue(); !ok || value != "80" {
		t.Errorf("the destination port mutated: %q %v", value, ok)
	}
	if got.Fields[0].Timestamp == nil || *got.Fields[0].Timestamp.RawText == "changed" {
		t.Errorf("field timestamp mutated: %+v", got.Fields[0].Timestamp)
	}
	if value, ok := got.ObservationKind.Raw[0].Text.RawTextValue(); !ok || value != "net" {
		t.Errorf("observation kind mutated: %q %v", value, ok)
	}
	if got.ProcessRef.ProcessId != "p" {
		t.Errorf("process id mutated: %q", got.ProcessRef.ProcessId)
	}
}
