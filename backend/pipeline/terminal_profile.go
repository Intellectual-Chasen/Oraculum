package pipeline

import "github.com/Intellectual-Chasen/Oraculum/backend/core"

// TerminalProfileValue は、収集の registry の値 1 つと、その値を記録した key のレコードである。
type TerminalProfileValue struct {
	// Name は registry の値の名前である。
	Name string
	// Value は値の原資料の文字列である (registry の reader が書いた文字列)。
	Value string
	// RecordRef は値を持つ key のレコードの位置である。
	RecordRef core.RecordLocator
}

// TerminalAddress は、ネットワークのインターフェース 1 つの key が記録した IP アドレスである。
type TerminalAddress struct {
	// Interface はインターフェースの key の名前である。
	Interface string
	// Values は DhcpIPAddress、IPAddress、LeaseObtainedTime のうち key が持つ値である。
	Values []TerminalProfileValue
	// From は LeaseObtainedTime が示す、DHCP のリースを得た時刻である。値を持たない key では nil である。
	From *core.Timestamp
	// To は同じ収集の全レコードの時刻のうち最も遅い時刻である。From が nil のときは nil である。
	To *core.Timestamp
}

// TerminalProfile は、収集の端末 1 台について収集の registry が記録した OS、名前、IP アドレス、
// タイムゾーンである。値の並びは、レコードを取り込んだ順である。
type TerminalProfile struct {
	// OperatingSystem は SOFTWARE の CurrentVersion の key の ProductName、DisplayVersion、
	// CurrentBuild、UBR、EditionID、SystemRoot である。
	OperatingSystem []TerminalProfileValue
	// Names は端末が名乗った名前の記録である (Graph.TerminalNames)。
	Names []TerminalName
	// Addresses は、Select の Current が選ぶ ControlSet の Tcpip の Interfaces の key ごとの記録である。
	Addresses []TerminalAddress
	// TimeZone は、Select の Current が選ぶ ControlSet の TimeZoneInformation の key の
	// TimeZoneKeyName、Bias、ActiveTimeBias である。
	TimeZone []TerminalProfileValue
}

// scannedSourcesAdjusters は、走査を終えた収集元の組を、レコードを組む前に直す処理である。
// 入力形式に依る処理を binding の file が init で登録する。
var scannedSourcesAdjusters []func([]scannedSource)
