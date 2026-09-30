package markii

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// プロセス開始の観測が必要とする key。**欠けたレコードから観測を作らない。**
//
// 8 つの用途は testdata/process-start-manifest.json の requiredKeys が持つ。
// sn は位置、psGUID と tmid はプロセスの識別、com と csid は端末の識別、psPath は
// 実行 file の path、evt と subEvt は観測の種別である。
//
// 一覧そのものはプロセス開始に固有である。8 つの key の文字列は共有の fields.go が持つ。
var processStartRequiredKeys = []string{
	keySequenceNumber, keyEvent, keySubEvent, keyProcessGuid,
	keyTerminalId, keyComputerName, keySecurityId, keyProcessPath,
}

// keyParentGuid は親のプロセスの GUID の key である。通信のレコードはこの key を持たない。
const keyParentGuid = "parentGUID"

// observationKindOf は evt と subEvt の 2 件から core.ObservationKind を組む。
//
// evt が ps で subEvt が start の組は形式が意味を定める。ParseProcessStart は
// この組のレコードだけを受け付けるため、意味は確定している。
func observationKindOf(record Record) core.ObservationKind {
	return core.ObservationKind{
		Raw:    observationKindRaw(record),
		Status: core.ObservationKindStatusDetermined,
	}
}
