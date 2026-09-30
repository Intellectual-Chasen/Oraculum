package core

// FlowOperation は、レコードが記録した操作の分類のうち、影響のエッジの向きを決める分類である。
// 分類は入力形式に依らない。入力形式の記録から分類への対応は、入力形式を読む adapter が
// FlowOperationSelector で宣言する。
type FlowOperation string

// FlowOperation の値。
const (
	// FlowOperationWrite は、書き込み、作成、削除、レジストリの値の設定と削除である。
	// 影響はプロセスから対象へ渡る。
	FlowOperationWrite FlowOperation = "write"
	// FlowOperationRead は、読み込み、ライブラリの読み込み、レジストリの値の読み込みである。
	// 影響は対象からプロセスへ渡る。
	FlowOperationRead FlowOperation = "read"
	// FlowOperationRename は名前の変更である。影響はプロセスから新しい名前のファイルへ、
	// 元の名前のファイルから新しい名前のファイルへ渡る。
	FlowOperationRename FlowOperation = "rename"
	// FlowOperationCommunication は、送信と受信を分けない通信の記録である。影響はプロセスと
	// 接続の間を両方の向きに渡る。
	FlowOperationCommunication FlowOperation = "communication"
	// FlowOperationAccountManagement は、アカウントの作成、削除、グループへの追加である。
	// 影響は操作の主体から、記録が対象とするアカウントへ渡る。
	FlowOperationAccountManagement FlowOperation = "account_management"
	// FlowOperationLogon は、アカウントの資格情報で成功したログオンである。影響は、記録が対象と
	// するアカウント (ログオンしたアカウント) から、ログオンのレコードへ渡る。
	FlowOperationLogon FlowOperation = "logon"
)

// IsKnown は FlowOperation が定義の中の値であるかを返す。
func (o FlowOperation) IsKnown() bool {
	switch o {
	case FlowOperationWrite, FlowOperationRead, FlowOperationRename, FlowOperationCommunication,
		FlowOperationAccountManagement, FlowOperationLogon:
		return true
	default:
		return false
	}
}

// FlowOperationSelector は、観測の種別が Kind に該当するレコードの操作の分類を Operation とする
// 宣言である。**観測の種別の欄の名前と value の文字列の定義元は入力形式を読む adapter である。**
type FlowOperationSelector struct {
	Kind      ObservationKindSelector
	Operation FlowOperation
}

// InfluenceBasis は影響のエッジの根拠の種類の値である。値は 3 つの軸 (向きの根拠、関係の状態、
// 同一性) のどれか 1 つに属し、影響のエッジは軸ごとに 1 つの値を持つ。
//
// **値の間に順序を付けない。** 値の並びは意味を持たない。
type InfluenceBasis string

// InfluenceBasis の値。
const (
	// InfluenceBasisSpecifiedOperation は、向きを入力形式の仕様が定める操作から決めたことである。
	InfluenceBasisSpecifiedOperation InfluenceBasis = "specified_operation"
	// InfluenceBasisInferredRecord は、向きを、仕様に定めの無い観測の種別から推定した操作で
	// 決めたことである (ObservationKindStatusInferred)。
	InfluenceBasisInferredRecord InfluenceBasis = "inferred_record"
	// InfluenceBasisUndeterminedDirection は、記録から向きを決められず、両方の向きの影響の
	// エッジにしたことである。
	InfluenceBasisUndeterminedDirection InfluenceBasis = "undetermined_direction"
	// InfluenceBasisAccountManagement は、向きを、アカウントの管理操作 (FlowOperationAccountManagement)
	// の記録が対象とするアカウントから決めたことである。向きの根拠の軸の値である。
	InfluenceBasisAccountManagement InfluenceBasis = "account_management"
	// InfluenceBasisArgumentName は、向きを、プロセスの引数に現れた名前から推定したことである
	// (EdgeKindArgumentNamesObject)。引数に現れたことは操作の記録を持たない。向きの根拠の軸の値である。
	InfluenceBasisArgumentName InfluenceBasis = "argument_name"
	// InfluenceBasisRequestedDestination は、向きを、別の収集元のプロセスと関連付いた接続の記録が
	// 指す接続先から決めたことである。接続から接続先への影響である。向きの根拠の軸の値である。
	InfluenceBasisRequestedDestination InfluenceBasis = "requested_destination"
	// InfluenceBasisCredentialUse は、向きを、ログオン (FlowOperationLogon) の記録が対象とする
	// アカウントから決めたことである。アカウントからログオンへの、資格情報の使用の影響である。
	// 向きの根拠の軸の値である。
	InfluenceBasisCredentialUse InfluenceBasis = "credential_use" //nolint:gosec // 根拠の種類の文字列であり、資格情報の値ではない。
	// InfluenceBasisObserved は、根拠の関係が観測であることである。
	InfluenceBasisObserved InfluenceBasis = "observed"
	// InfluenceBasisCandidate は、根拠の関係が関連付けの候補であることである。
	InfluenceBasisCandidate InfluenceBasis = "candidate"
	// InfluenceBasisUncertainChain は、根拠の関係が不確定の連鎖であることである。
	InfluenceBasisUncertainChain InfluenceBasis = "uncertain_chain"
	// InfluenceBasisSingleNode は、影響のエッジが 2 つの実体の間の影響を表すことである。
	InfluenceBasisSingleNode InfluenceBasis = "single_node"
	// InfluenceBasisEquivalence は、影響のエッジが、同じ実体が分かれた 2 つのノードを結ぶ
	// 等価の影響のエッジであることである。
	InfluenceBasisEquivalence InfluenceBasis = "equivalence"
)

// IsKnown は InfluenceBasis が定義の中の値であるかを返す。
func (b InfluenceBasis) IsKnown() bool {
	switch b {
	case InfluenceBasisSpecifiedOperation, InfluenceBasisInferredRecord, InfluenceBasisUndeterminedDirection,
		InfluenceBasisAccountManagement, InfluenceBasisArgumentName, InfluenceBasisRequestedDestination,
		InfluenceBasisCredentialUse, InfluenceBasisObserved, InfluenceBasisCandidate, InfluenceBasisUncertainChain,
		InfluenceBasisSingleNode, InfluenceBasisEquivalence:
		return true
	default:
		return false
	}
}

// InfluenceBasisOfState は関係の状態に対応する、関係の状態の軸の値を返す。
func InfluenceBasisOfState(state RelationState) InfluenceBasis {
	switch state {
	case RelationStateObserved:
		return InfluenceBasisObserved
	case RelationStateUncertainChain:
		return InfluenceBasisUncertainChain
	default:
		return InfluenceBasisCandidate
	}
}

// InfluenceTimeBasis は、経路の条件の時刻を満たした方法の種類である。**値の間に順序を付けない。**
type InfluenceTimeBasis string

// InfluenceTimeBasis の値。
const (
	// InfluenceTimeBasisSameTerminal は、経路の影響のエッジと起点と終点のタイムスタンプを、
	// 1 つの端末が記録したことである。
	InfluenceTimeBasisSameTerminal InfluenceTimeBasis = "same_terminal"
	// InfluenceTimeBasisUnboundedOffset は、経路が含む端末に、タイムスタンプのずれの差の上限の
	// 無い組があることである。
	InfluenceTimeBasisUnboundedOffset InfluenceTimeBasis = "unbounded_offset"
	// InfluenceTimeBasisPrecisionWidth は、区間を精度の幅だけ広げたときにだけ条件を満たすことである。
	InfluenceTimeBasisPrecisionWidth InfluenceTimeBasis = "precision_width"
)

// IsKnown は InfluenceTimeBasis が定義の中の値であるかを返す。
func (b InfluenceTimeBasis) IsKnown() bool {
	switch b {
	case InfluenceTimeBasisSameTerminal, InfluenceTimeBasisUnboundedOffset, InfluenceTimeBasisPrecisionWidth:
		return true
	default:
		return false
	}
}

// InfluencePathStop は、経路の集合が空であるか、求めきれなかった理由である。
type InfluencePathStop string

// InfluencePathStop の値。
const (
	// InfluencePathStopOriginWithoutTimestamp は、起点のノードがタイムスタンプを持つレコードを
	// 持たないことである。
	InfluencePathStopOriginWithoutTimestamp InfluencePathStop = "origin_without_timestamp"
	// InfluencePathStopDestinationWithoutTimestamp は、終点のノードがタイムスタンプを持つ
	// レコードを持たないことである。
	InfluencePathStopDestinationWithoutTimestamp InfluencePathStop = "destination_without_timestamp"
	// InfluencePathStopNoInfluenceRoute は、時刻を問わなくても、起点から終点へ影響のエッジを
	// 辿れないことである。
	InfluencePathStopNoInfluenceRoute InfluencePathStop = "no_influence_route"
	// InfluencePathStopTimeOrderUnsatisfied は、影響のエッジを辿れるが、どの列も経路の時刻の
	// 条件を満たさないことである。
	InfluencePathStopTimeOrderUnsatisfied InfluencePathStop = "time_order_unsatisfied"
	// InfluencePathStopComputationLimit は、探索の状態の数が上限に達し、集合を求めきれなかった
	// ことである。区間を精度の幅で広げた計算が上限に達したときは、集合は空で返る。広げない
	// 計算だけが上限に達したときは、広げた計算の集合を返し、その影響のエッジに精度の幅の
	// 時刻の根拠を付けない。
	InfluencePathStopComputationLimit InfluencePathStop = "computation_limit"
	// InfluencePathStopOriginNotInfluenceEnd は、起点のノードを端に持つ影響のエッジが、根拠の種類で
	// 除く前から 1 本も無いことである。IP、端末などの種別のノードと、影響のエッジになる関係を
	// 持たないレコードが該当する。レコードのノードは、記録したプロセスへ読み替えた後も無いときである。
	InfluencePathStopOriginNotInfluenceEnd InfluencePathStop = "origin_not_influence_end"
	// InfluencePathStopDestinationNotInfluenceEnd は、終点のノードについての同じ理由である。
	InfluencePathStopDestinationNotInfluenceEnd InfluencePathStop = "destination_not_influence_end"
	// InfluencePathStopTruncatedRouteUnverified は、経路に乗る影響のエッジを上限で切り、返した
	// エッジだけで起点から終点へ時刻の条件を満たす経路が成り立つと確かめられないことである。
	// タイムスタンプを記録した端末が 2 つ以上の入力、起点と終点が同じノードの入力、時刻の条件を
	// 満たす最も早い経路が上限より長い入力が該当する。
	InfluencePathStopTruncatedRouteUnverified InfluencePathStop = "truncated_route_unverified"
	// InfluencePathStopRouteThroughExcludedBasis は、除いた根拠の種類を持つ影響のエッジも使うと、起点から
	// 終点へ時刻の条件を満たす経路があることである。その経路に乗るエッジが持つ、除いた根拠の種類を
	// 応答に添える。
	InfluencePathStopRouteThroughExcludedBasis InfluencePathStop = "route_through_excluded_basis"
	// InfluencePathStopExcludedBasisRouteLimit は、除いた根拠の種類を持つ影響のエッジも使って経路を
	// 求め直した計算が探索の状態の数の上限に達し、戻したときに経路があるかが分からないことである。
	InfluencePathStopExcludedBasisRouteLimit InfluencePathStop = "excluded_basis_route_limit"
)

// IsKnown は InfluencePathStop が定義の中の値であるかを返す。
func (s InfluencePathStop) IsKnown() bool {
	switch s {
	case InfluencePathStopOriginNotInfluenceEnd, InfluencePathStopDestinationNotInfluenceEnd,
		InfluencePathStopTruncatedRouteUnverified, InfluencePathStopRouteThroughExcludedBasis,
		InfluencePathStopExcludedBasisRouteLimit,
		InfluencePathStopOriginWithoutTimestamp, InfluencePathStopDestinationWithoutTimestamp,
		InfluencePathStopNoInfluenceRoute, InfluencePathStopTimeOrderUnsatisfied, InfluencePathStopComputationLimit:
		return true
	default:
		return false
	}
}

// InfluenceFrontierReason は、起点から時刻の条件を満たして届いたノードから先へ影響が進まない
// 理由である。
type InfluenceFrontierReason string

// InfluenceFrontierReason の値。
const (
	// InfluenceFrontierReasonNoOutgoingInfluence は、そのノードから出る影響のエッジが無いこと
	// である。そのノードから先へ影響を示す記録が無い。
	InfluenceFrontierReasonNoOutgoingInfluence InfluenceFrontierReason = "no_outgoing_influence"
	// InfluenceFrontierReasonOutgoingTimeUnsatisfied は、そのノードから出る影響のエッジが
	// どれも、届いた時刻より後の時刻の条件を満たさないことである。
	InfluenceFrontierReasonOutgoingTimeUnsatisfied InfluenceFrontierReason = "outgoing_time_unsatisfied"
	// InfluenceFrontierReasonOutgoingExcluded は、そのノードから出る影響のエッジがどれも、
	// 除いた根拠の種類を持つことである。
	InfluenceFrontierReasonOutgoingExcluded InfluenceFrontierReason = "outgoing_excluded"
)

// IsKnown は InfluenceFrontierReason が定義の中の値であるかを返す。
func (r InfluenceFrontierReason) IsKnown() bool {
	switch r {
	case InfluenceFrontierReasonNoOutgoingInfluence, InfluenceFrontierReasonOutgoingTimeUnsatisfied,
		InfluenceFrontierReasonOutgoingExcluded:
		return true
	default:
		return false
	}
}

// InfluenceVertex は影響の経路のノード 1 つである。ファイル、レジストリの値は内容のバージョンに
// 分かれ、同じグラフのノードが 2 つ以上の要素になる。
type InfluenceVertex struct {
	// Key は影響の経路の中でこの要素を指す文字列である。InfluenceEdge の両端が指す。
	Key string `json:"key"`
	// Node はグラフのノードである。
	Node GraphNode `json:"node"`
	// VersionStart は内容のバージョンが始まった時刻 (RFC 3339) である。内容を置き換える記録の
	// 後のバージョンだけが持つ。
	VersionStart string `json:"versionStart,omitempty"`
	// InfluenceEdgeCount は、根拠の種類で除いた後の影響のエッジのうち、この要素を端に持つものの数である。
	InfluenceEdgeCount int64 `json:"influenceEdgeCount"`
	// EnteredFrom は、2 件以上の候補のプロセスを持つレコードの要素のうち、1 つの候補から入った
	// 要素の、その候補のプロセスである。**その要素から、ほかの候補のプロセスへは進まない。**
	EnteredFrom *GraphNode `json:"enteredFrom,omitempty"`
}

// Validate は項目の整合を確かめる。
func (v InfluenceVertex) Validate() error {
	problem := firstProblem(
		requirePresent("InfluenceVertex.key", v.Key),
		v.Node.Validate(),
		requireNonNegative("InfluenceVertex.influenceEdgeCount", v.InfluenceEdgeCount),
	)
	if problem == nil && v.EnteredFrom != nil {
		problem = v.EnteredFrom.Validate()
	}
	return problem
}

// InfluenceEdge は、経路に乗る影響のエッジ 1 本である。
type InfluenceEdge struct {
	// Id は影響のエッジを指す文字列である。
	Id string `json:"id"`
	// SourceKey と TargetKey は、影響の渡る元と先の InfluenceVertex.Key である。
	SourceKey string `json:"sourceKey"`
	TargetKey string `json:"targetKey"`
	// GraphEdgeId と GraphEdgeKind は、影響のエッジを作った根拠の関係である。
	GraphEdgeId   string   `json:"graphEdgeId"`
	GraphEdgeKind EdgeKind `json:"graphEdgeKind"`
	// Bases は根拠の種類であり、軸ごとに 1 つの値を持つ。**並びは意味を持たない。**
	Bases []InfluenceBasis `json:"bases"`
	// TimeBases は、この影響のエッジを通る経路が時刻の条件を満たした方法の種類の集合である。
	TimeBases []InfluenceTimeBasis `json:"timeBases"`
	// Evidence は根拠のレコードの組である。
	Evidence []GraphEvidence `json:"evidence"`
	// CandidateCount は、関係の状態が不確定の連鎖である影響のエッジだけが持ち、同じ先の要素に入る
	// 不確定の連鎖の関係の数である。このエッジはその候補の 1 つである。
	CandidateCount int64 `json:"candidateCount,omitempty"`
	// CandidateTally と CandidateOrder は、候補を区分で並べる不確定の連鎖 (ログオンの連鎖) の影響の
	// エッジだけが持つ。CandidateTally は同じ終点のログオンに挙がった候補の数と、並びでこの候補より
	// 上の区分に入る候補の数である。CandidateOrder は、この候補の区分を決めた条件 (アカウントの一致と
	// ログオンの種別の区分) である。
	CandidateTally *EdgeCandidateTally    `json:"candidateTally,omitempty"`
	CandidateOrder []EdgePairConditionKey `json:"candidateOrder,omitempty"`
}

// Validate は項目の整合を確かめる。
func (e InfluenceEdge) Validate() error {
	problem := firstProblem(
		requirePresent("InfluenceEdge.id", e.Id),
		requirePresent("InfluenceEdge.sourceKey", e.SourceKey),
		requirePresent("InfluenceEdge.targetKey", e.TargetKey),
		requirePresent("InfluenceEdge.graphEdgeId", e.GraphEdgeId),
		requireKnownEnum("InfluenceEdge.graphEdgeKind", e.GraphEdgeKind),
		requireNonNegative("InfluenceEdge.candidateCount", e.CandidateCount),
	)
	if problem != nil {
		return problem
	}
	for _, basis := range e.Bases {
		if err := requireKnownEnum("InfluenceEdge.bases", basis); err != nil {
			return err
		}
	}
	for _, basis := range e.TimeBases {
		if err := requireKnownEnum("InfluenceEdge.timeBases", basis); err != nil {
			return err
		}
	}
	for _, key := range e.CandidateOrder {
		if err := requireKnownEnum("InfluenceEdge.candidateOrder", key); err != nil {
			return err
		}
	}
	if tally := e.CandidateTally; tally != nil &&
		(tally.PrecedingCandidateCount < 0 || tally.PrecedingCandidateCount >= tally.CandidateCount) {
		return itemError("InfluenceEdge.candidateTally", ErrInconsistentValue)
	}
	if len(e.Evidence) == 0 {
		return itemError("InfluenceEdge.evidence", ErrMissingRequiredItem)
	}
	return validateElements("InfluenceEdge.evidence", e.Evidence)
}

// InfluenceEndpoint は経路の起点または終点である。
type InfluenceEndpoint struct {
	// Key は起点または終点の InfluenceVertex.Key である。
	Key string `json:"key"`
	// Record は、タイムスタンプを決めたノードのレコードである。起点はノードの最初のレコード、
	// 終点はノードの最後のレコードである。
	Record GraphEvidence `json:"record"`
}

// Validate は、起点または終点がノードの文字列と、正しい根拠のレコードを持つことを確かめる。
func (e InfluenceEndpoint) Validate() error {
	return firstProblem(
		requirePresent("InfluenceEndpoint.key", e.Key),
		e.Record.Validate(),
	)
}

// InfluenceFrontier は、起点から届いたが先へ影響が進まないノード 1 つと理由である。**このノードは
// 経路のエッジの端でなく、応答の経路の要素 (vertices) に入らない。** ノードの項目をここに持つ。
type InfluenceFrontier struct {
	InfluenceVertex
	Reason InfluenceFrontierReason `json:"reason"`
}

// Validate は項目の整合を確かめる。
func (f InfluenceFrontier) Validate() error {
	return firstProblem(
		f.InfluenceVertex.Validate(),
		requireKnownEnum("InfluenceFrontier.reason", f.Reason),
	)
}
