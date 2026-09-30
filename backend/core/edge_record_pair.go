package core

// EdgePairConditionKey は、候補の関係を 2 件のレコードの値から作った条件の種別である。
type EdgePairConditionKey string

// EdgePairConditionKey の値。
const (
	// EdgePairConditionProcessPid は、2 つのプロセス番号の一致である。親子の候補では、親の
	// レコードのプロセス番号と、子のレコードが記録した親のプロセス番号を比べる。
	EdgePairConditionProcessPid EdgePairConditionKey = "process_pid"
	// EdgePairConditionTerminal は、2 件のレコードを置いた端末が同じであることである。値は
	// レコードを置いた端末の表示名である。端末の欄を持たないレコードは収集元の端末に置く。
	EdgePairConditionTerminal EdgePairConditionKey = "terminal"
	// EdgePairConditionTimeOrder は、起点の側のレコードの時刻が終点の側のレコードの時刻より
	// 後でないことである。時刻は両側のレコードの eventTime が持つ。
	EdgePairConditionTimeOrder EdgePairConditionKey = "time_order"
	// EdgePairConditionTimeProximity は、2 件のレコードの時刻の差が、関係の種別が定める幅の
	// 中にあることである。
	EdgePairConditionTimeProximity EdgePairConditionKey = "time_proximity"
	// EdgePairConditionTimeOverlap は、2 つのプロセスを記録したレコードの時刻の範囲が重なる
	// ことである。両側のレコードは、それぞれのプロセスのプロセス番号を記録したレコードのうち
	// 時刻の最も早いレコードである。
	EdgePairConditionTimeOverlap EdgePairConditionKey = "time_overlap"
	// EdgePairConditionNearestIdentityRecord は、終点の側のレコードが、起点の側のレコードの
	// 時刻の直前か直後に同じ名前と識別子を記録したレコードであり、直前と直後の識別子が同じ
	// であることである。
	EdgePairConditionNearestIdentityRecord EdgePairConditionKey = "nearest_identity_record"
	// EdgePairConditionLogonId は、ログオンが作ったセッションの Logon ID と、操作を行った
	// セッションの Logon ID の一致である。
	EdgePairConditionLogonId EdgePairConditionKey = "logon_id"
	// EdgePairConditionLinkedLogonId は、分けて作った 2 件のログオンの Logon ID と、分けた
	// ログオンの Logon ID が互いを指すことである。
	EdgePairConditionLinkedLogonId EdgePairConditionKey = "linked_logon_id"
	// EdgePairConditionLogonGuid は、チケットの要求とログオンの LogonGuid の一致である。
	EdgePairConditionLogonGuid EdgePairConditionKey = "logon_guid"
	// EdgePairConditionTaskName は、登録したタスクと起動したタスクの名前の一致である。
	EdgePairConditionTaskName EdgePairConditionKey = "task_name"
	// EdgePairConditionAccount は、2 件のレコードが記録したアカウントの一致である。
	EdgePairConditionAccount EdgePairConditionKey = "account"
	// EdgePairConditionSourceEndpoint は、接続元のアドレスと port の一致である。
	EdgePairConditionSourceEndpoint EdgePairConditionKey = "source_endpoint"
	// EdgePairConditionDestinationEndpoint は、宛先の port の一致と、両側が記録した宛先の
	// アドレスの一致である。宛先のアドレスを記録しない側のレコードは port だけを持つ。
	EdgePairConditionDestinationEndpoint EdgePairConditionKey = "destination_endpoint"
	// EdgePairConditionDestinationTerminal は、起点の側のレコードの接続先のアドレスを、
	// 分析者の割当が終点の側のレコードを置いた端末へ導くことである。終点の側の値は端末の
	// 表示名である。
	EdgePairConditionDestinationTerminal EdgePairConditionKey = "destination_terminal"
	// EdgePairConditionSourceUnassigned は、終点の側のレコードの接続元のアドレスに、端末の
	// 割当が 1 件も無いことである。起点の側はレコードを持たない。
	EdgePairConditionSourceUnassigned EdgePairConditionKey = "source_unassigned"
	// EdgePairConditionSourceOutsideAssignmentRange は、終点の側のレコードの接続元のアドレスに
	// 端末の割当があり、レコードの時刻がどの割当の期間にも入らないことである。起点の側は
	// レコードを持たない。
	EdgePairConditionSourceOutsideAssignmentRange EdgePairConditionKey = "source_outside_assignment_range"
	// EdgePairConditionRequestingSession は、起点の側のレコードが始めたログオンのセッションの
	// 期間の中に、そのセッションの Logon ID で終点の側のログオンを要求した記録があることである。
	// 起点の側の値はセッションの Logon ID である。要求の記録は関係の根拠のレコードにある。
	EdgePairConditionRequestingSession EdgePairConditionKey = "requesting_session"
	// EdgePairConditionSourceTerminal は、終点の側のレコードの接続元のアドレスを、分析者の割当が
	// 起点の側のレコードを置いた端末へ導くことである。起点の側の値は端末の表示名である。
	EdgePairConditionSourceTerminal EdgePairConditionKey = "source_terminal"
	// 以下の 3 つは、起点の側のレコードが始めたセッションの始まりを決めた記録の種類である。
	// どれも、終点の側のレコードの時刻がその始まりより前でないことを含む。
	//
	// EdgePairConditionSessionStartLogon はログオンの記録、EdgePairConditionSessionStartFirstOperation は
	// 同じ Logon ID を持つ最初の操作の記録、EdgePairConditionSessionStartLogoffRecord はログオフの
	// 記録が書いた始まりの時刻である。
	EdgePairConditionSessionStartLogon          EdgePairConditionKey = "session_start_logon"
	EdgePairConditionSessionStartFirstOperation EdgePairConditionKey = "session_start_first_operation"
	EdgePairConditionSessionStartLogoffRecord   EdgePairConditionKey = "session_start_logoff_record"
	// 以下の 4 つは、同じセッションの終わりを決めた記録の種類である。どれも、終点の側のレコードの
	// 時刻がその終わりより後でないことを含む。
	//
	// EdgePairConditionSessionEndLogoff はログオフの記録、EdgePairConditionSessionEndSystemStart は
	// 次の起動の記録、EdgePairConditionSessionEndTimeLimit は最後の操作の記録から分析者が与えた
	// 時間の後である。EdgePairConditionSessionEndLastOperation は、ログオフの記録が無いネットワークの
	// ログオン (種別 3) のセッションの、最後に記録された操作 (無いときは始まりの記録) である。次の
	// 起動の記録があっても、最後の操作で終える。
	EdgePairConditionSessionEndLogoff        EdgePairConditionKey = "session_end_logoff"
	EdgePairConditionSessionEndSystemStart   EdgePairConditionKey = "session_end_system_start"
	EdgePairConditionSessionEndTimeLimit     EdgePairConditionKey = "session_end_time_limit"
	EdgePairConditionSessionEndLastOperation EdgePairConditionKey = "session_end_last_operation"
	// 以下の 2 つは、起点の側のセッションのアカウントと、終点の側のログオンのアカウントの比較である。
	// 起点の側の値は、セッションを始めたログオンの対象のアカウントと、他の端末への接続に使う
	// アカウント (種別 9 のログオン) である。始まりが最初の操作の記録のセッションでは、操作を
	// 行ったアカウントである。終点の側の値は、終点のログオンの対象のアカウントの SID、ドメイン、
	// 名前である。
	//
	// EdgePairConditionSessionAccountMatch は、SID か名前 (大文字と小文字を区別しない) が一致する
	// ことである。EdgePairConditionSessionAccountDifferent は、どちらも一致しないか、比べる値が
	// 片側に無いことである。
	EdgePairConditionSessionAccountMatch     EdgePairConditionKey = "session_account_match"
	EdgePairConditionSessionAccountDifferent EdgePairConditionKey = "session_account_different"
	// 以下の 3 つは、起点の側のセッションを始めたログオンの種別の区分である。起点の側の値は
	// セッションを始めたログオンの種別のコード、終点の側の値は終点のログオンの種別のコードである。
	// 区分を決めるのは起点の側の値だけである。
	//
	// EdgePairConditionSessionLogonInteractive は対話と画面の遠隔操作の種別 (2、7、10、11、12、13)、
	// EdgePairConditionSessionLogonNetwork はネットワークの種別 (3)、EdgePairConditionSessionLogonOther は
	// それ以外の種別と、種別を記録していない始まりである。
	EdgePairConditionSessionLogonInteractive EdgePairConditionKey = "session_logon_interactive"
	EdgePairConditionSessionLogonNetwork     EdgePairConditionKey = "session_logon_network"
	EdgePairConditionSessionLogonOther       EdgePairConditionKey = "session_logon_other"
)

// IsKnown は契約が定める値かを返す。
func (k EdgePairConditionKey) IsKnown() bool {
	switch k {
	case EdgePairConditionProcessPid, EdgePairConditionTerminal, EdgePairConditionTimeOrder,
		EdgePairConditionTimeProximity, EdgePairConditionTimeOverlap,
		EdgePairConditionNearestIdentityRecord, EdgePairConditionLogonId,
		EdgePairConditionLinkedLogonId, EdgePairConditionLogonGuid, EdgePairConditionTaskName,
		EdgePairConditionAccount, EdgePairConditionSourceEndpoint, EdgePairConditionDestinationEndpoint,
		EdgePairConditionDestinationTerminal, EdgePairConditionSourceUnassigned,
		EdgePairConditionRequestingSession, EdgePairConditionSourceTerminal,
		EdgePairConditionSessionStartLogon, EdgePairConditionSessionStartFirstOperation,
		EdgePairConditionSessionStartLogoffRecord, EdgePairConditionSessionEndLogoff,
		EdgePairConditionSessionEndSystemStart, EdgePairConditionSessionEndTimeLimit,
		EdgePairConditionSessionEndLastOperation, EdgePairConditionSourceOutsideAssignmentRange,
		EdgePairConditionSessionAccountMatch, EdgePairConditionSessionAccountDifferent,
		EdgePairConditionSessionLogonInteractive, EdgePairConditionSessionLogonNetwork,
		EdgePairConditionSessionLogonOther:
		return true
	default:
		return false
	}
}

// EdgePairCondition は、レコードの組が満たした条件 1 つと、条件に用いた両側の値である。
//
// **時刻の条件は値を持たない。** 比べた時刻は組の両側の GraphEvidence の eventTime である。
type EdgePairCondition struct {
	ConditionKey EdgePairConditionKey `json:"conditionKey"`
	// LeftValue は起点の側のレコードの欄である。欄を読めないレコードと、時刻の条件では要素数 0 である。
	LeftValue []RecordField `json:"leftValue"`
	// RightValue は終点の側のレコードの欄である。LeftValue と同じ規則で要素数 0 になる。
	RightValue []RecordField `json:"rightValue"`
	// WindowSeconds は、時刻の差の許容幅の秒数である。関係の種別が幅を定める時刻の条件
	// (EdgePairConditionTimeProximity) でだけ出る。
	WindowSeconds *int64 `json:"windowSeconds,omitempty"`
	// DifferenceSeconds は、終点の側のレコードの時刻から起点の側のレコードの時刻を引いた秒数
	// である。WindowSeconds を持つ条件で、両側の時刻を時点として読めるときだけ出る。
	DifferenceSeconds *float64 `json:"differenceSeconds,omitempty"`
	// DifferenceInWholeSeconds は、照合が両側の時刻の秒未満を切り捨てて比べたことである。真のとき
	// DifferenceSeconds は切り捨てた 2 つの時刻の差であり、整数の秒数である。
	DifferenceInWholeSeconds bool `json:"differenceInWholeSeconds,omitempty"`
}

// Validate は項目の整合を確かめる。
func (c EdgePairCondition) Validate() error {
	if err := requireKnownEnum("EdgePairCondition.conditionKey", c.ConditionKey); err != nil {
		return err
	}
	if c.WindowSeconds != nil && *c.WindowSeconds < 0 {
		return itemError("EdgePairCondition.windowSeconds", ErrInvalid)
	}
	if c.DifferenceSeconds != nil && c.WindowSeconds == nil {
		return itemError("EdgePairCondition.differenceSeconds", ErrInconsistentValue)
	}
	if problem := validateElements("EdgePairCondition.leftValue", c.LeftValue); problem != nil {
		return problem
	}
	return validateElements("EdgePairCondition.rightValue", c.RightValue)
}

// EdgeRecordPair は、観測の層が候補の関係を作ったレコードの組 1 つと、成立に用いた条件である。
//
// Left は関係の起点の側、Right は終点の側のレコードである。**片側がレコードを持たない組が
// ある。** 接続元が未同定の遠隔のセッションの起点は、割当の無い IP アドレスである。
type EdgeRecordPair struct {
	Left       *GraphEvidence      `json:"left,omitempty"`
	Right      *GraphEvidence      `json:"right,omitempty"`
	Conditions []EdgePairCondition `json:"conditions"`
	// CandidateTally は、同じ終点のレコードに挙がった候補の数と、そのうち並びでこの組の起点より
	// 上の区分に入る候補の数である。候補を区分で並べる関係 (EdgeKindLogonChain) の組でだけ出る。
	CandidateTally *EdgeCandidateTally `json:"candidateTally,omitempty"`
}

// EdgeCandidateTally は、終点のレコード 1 件に挙がった候補を、組の条件の区分で並べた件数である。
//
// **並びは条件の値の区分の順だけを表す。** ログオンの連鎖では、アカウントが一致する候補を上に、
// 次にログオンの種別の区分 (対話と画面の遠隔操作、その他、ネットワークの順) で並べる。区分を
// 決めた条件は、組の Conditions にある。
type EdgeCandidateTally struct {
	// CandidateCount は同じ終点のレコードに挙がった候補の数である。1 以上である。
	CandidateCount int `json:"candidateCount"`
	// PrecedingCandidateCount は、そのうち並びでこの組の起点より上の区分に入る候補の数である。
	PrecedingCandidateCount int `json:"precedingCandidateCount"`
}

// Validate は項目の整合を確かめる。
func (p EdgeRecordPair) Validate() error {
	if p.Left == nil && p.Right == nil {
		return itemError("EdgeRecordPair.left", ErrMissingRequiredItem)
	}
	for _, side := range []struct {
		item     string
		evidence *GraphEvidence
	}{{"EdgeRecordPair.left", p.Left}, {"EdgeRecordPair.right", p.Right}} {
		if side.evidence == nil {
			continue
		}
		if err := side.evidence.Validate(); err != nil {
			return itemError(side.item, err)
		}
	}
	if len(p.Conditions) == 0 {
		return itemError("EdgeRecordPair.conditions", ErrMissingRequiredItem)
	}
	if tally := p.CandidateTally; tally != nil &&
		(tally.PrecedingCandidateCount < 0 || tally.PrecedingCandidateCount >= tally.CandidateCount) {
		return itemError("EdgeRecordPair.candidateTally", ErrInconsistentValue)
	}
	return validateElements("EdgeRecordPair.conditions", p.Conditions)
}
