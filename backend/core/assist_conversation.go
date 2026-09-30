package core

import "strconv"

// AssistTurn は、分析者の発言 1 つに添えた画面の文脈のうち、同じ発言の受け渡しが従う値である。
//
// **関連付けの条件の選択は発言ごとに変わりうる。** 発言の中の受け渡しは、発言に登録した選択の
// グラフから本文を組む。
type AssistTurn struct {
	ConversationId string `json:"conversationId"`
	TurnId         string `json:"turnId"`
	// MatchConditions は発言の時点で画面が選んでいた関連付けの条件である。
	MatchConditions []AssistMatchCondition `json:"matchConditions"`
	// Case は発言の時点で画面が選んでいた案件である。選んでいない発言では空である。
	Case string `json:"case,omitempty"`
}

// Validate は項目の整合を確かめる。
func (t AssistTurn) Validate() error {
	problem := firstProblem(
		ValidateAssistConversationId("AssistTurn.conversationId", t.ConversationId),
		ValidateAssistTurnId("AssistTurn.turnId", t.TurnId),
		validateAssistMatchConditions("AssistTurn.matchConditions", t.MatchConditions),
	)
	if problem != nil || t.Case == "" {
		return problem
	}
	return ValidateCaseId("AssistTurn.case", t.Case)
}

// AssistConversation は、分析者が提供者 1 つと行う会話 1 件である。
//
// **会話の本文 (分析者の発言と LLM の応答) を持たない。** 本文は分析者の端末の中継だけが持つ。
// server が持つのは、会話の識別子から受け渡しの監査記録を探す材料である。
type AssistConversation struct {
	// Id は server が乱数 128 bit から発行した会話の識別子である。
	Id string `json:"id"`
	// Provider は会話の提供者である。
	Provider AssistProvider `json:"provider"`
	// Model は中継が申告した提供者の model の名前である。申告の無い会話では空である。
	Model string `json:"model,omitempty"`
	// PermissionRevision は、会話を発行した時点の送信の許可の改訂の番号である。
	PermissionRevision int64 `json:"permissionRevision"`
	// CreatedAt は会話を発行した時刻である。
	CreatedAt AssertionTime `json:"createdAt"`
}

// Validate は項目の整合を確かめる。
func (c AssistConversation) Validate() error {
	problem := firstProblem(
		ValidateAssistConversationId("AssistConversation.id", c.Id),
		requireKnownEnum("AssistConversation.provider", c.Provider),
		requireSanitized("AssistConversation.model", c.Model),
	)
	if problem != nil {
		return problem
	}
	if c.PermissionRevision < FirstAssistPermissionRevisionNumber {
		return itemError("AssistConversation.permissionRevision", ErrInvalid)
	}
	return c.CreatedAt.Validate()
}

// AssistRefKind は、会話の短い参照が指すものの種類である。
type AssistRefKind string

// AssistRefKind の値。
const (
	AssistRefKindRecord AssistRefKind = "record"
	AssistRefKindNode   AssistRefKind = "node"
	AssistRefKindEdge   AssistRefKind = "edge"
)

// IsKnown は値が定義の中にあることを返す。
func (k AssistRefKind) IsKnown() bool {
	return k == AssistRefKindRecord || k == AssistRefKindNode || k == AssistRefKindEdge
}

// prefix は短い参照の文字列の先頭の 1 文字である。
func (k AssistRefKind) prefix() string {
	switch k {
	case AssistRefKindRecord:
		return "r"
	case AssistRefKindNode:
		return "n"
	default:
		return "e"
	}
}

// AssistShortRefOf は、種類と会話の中の通番から短い参照の文字列を作る。
func AssistShortRefOf(kind AssistRefKind, ordinal int64) string {
	return kind.prefix() + strconv.FormatInt(ordinal, 10)
}

// AssistRecordTarget は、短い参照が指す記録 1 件である。取り込みをやり直しても同じ記録に一致する
// 材料と、この取り込みで探すための収集元の識別子を持つ。
type AssistRecordTarget struct {
	SourceId string             `json:"sourceId"`
	Record   AssertionRecordRef `json:"record"`
}

// AssistShortRef は、会話の中で server が発行した短い参照 1 つと、それが指すものである。
//
// **LLM に記録の識別子 (sha256 と位置) をそのまま写させない。** 64 桁の値の写し間違いを避け、
// 参照が同じ会話で受け渡したものであることを server が確かめられる。
type AssistShortRef struct {
	// Ref は会話の中で一意の短い文字列である (`r12`、`n3`、`e7`)。
	Ref  string        `json:"ref"`
	Kind AssistRefKind `json:"kind"`
	// Record は記録を指す参照が持つ。
	Record *AssistRecordTarget `json:"record,omitempty"`
	// NodeId はノードを指す参照が持つ。
	NodeId string `json:"nodeId,omitempty"`
	// EdgeId は関係を指す参照が持つ。
	EdgeId string `json:"edgeId,omitempty"`
}

// Validate は項目の整合を確かめる。
func (r AssistShortRef) Validate() error {
	problem := firstProblem(
		requirePresent("AssistShortRef.ref", r.Ref),
		requireKnownEnum("AssistShortRef.kind", r.Kind),
	)
	if problem != nil {
		return problem
	}
	hasRecord, hasNode, hasEdge := r.Record != nil, r.NodeId != "", r.EdgeId != ""
	switch r.Kind {
	case AssistRefKindRecord:
		if !hasRecord || hasNode || hasEdge {
			return itemError("AssistShortRef.record", ErrInconsistentValue)
		}
		return firstProblem(requirePresent("AssistShortRef.record.sourceId", r.Record.SourceId),
			r.Record.Record.Validate())
	case AssistRefKindNode:
		if hasRecord || !hasNode || hasEdge {
			return itemError("AssistShortRef.nodeId", ErrInconsistentValue)
		}
	default:
		if hasRecord || hasNode || !hasEdge {
			return itemError("AssistShortRef.edgeId", ErrInconsistentValue)
		}
	}
	return nil
}

// AssistTool は、中継の tool が呼ぶ server の受け渡しの種類である。
type AssistTool string

// AssistTool の値。
const (
	// AssistToolTurn は、分析者の発言に添えた画面の文脈の登録である。
	AssistToolTurn AssistTool = "turn"
	// AssistToolOverview は調査の概要である。
	AssistToolOverview AssistTool = "overview"
	// AssistToolGraphSearch はグラフの検索である。
	AssistToolGraphSearch AssistTool = "graph_search"
	// AssistToolTarget は、ノードまたは関係の詳細である。
	AssistToolTarget AssistTool = "target"
	// AssistToolRecords は記録の原文である。
	AssistToolRecords AssistTool = "records"
	// AssistToolTimeline は時系列である。
	AssistToolTimeline AssistTool = "timeline"
)

// IsKnown は値が定義の中にあることを返す。
func (t AssistTool) IsKnown() bool {
	switch t {
	case AssistToolTurn, AssistToolOverview, AssistToolGraphSearch, AssistToolTarget, AssistToolRecords,
		AssistToolTimeline:
		return true
	default:
		return false
	}
}

// AssistVersions は、受け渡しの本文を組み直すのに要る commit、改訂の番号、更新回数である。
//
// **応答の本文は保存しない。** 本文は、これらの値と要求から組み直し、sha256 で照合する。
type AssistVersions struct {
	// ServerRevision は server の build の VCS の revision である。VCS の情報を持たない build では
	// 空であり、その受け渡しは組み直しで照合できない。
	ServerRevision string `json:"serverRevision,omitempty"`
	// ServerModified は、server の build が commit に無い変更を含むことである。
	ServerModified bool `json:"serverModified"`
	// AssignmentRevision は、端末の割当の更新回数である。
	AssignmentRevision int64 `json:"assignmentRevision"`
	// InterpretationRevision は、収集元の時刻の解釈の更新回数である。
	InterpretationRevision int64 `json:"interpretationRevision"`
	// SourceSetSha256 は、取り込んだ収集元の集合の識別である。
	SourceSetSha256 string `json:"sourceSetSha256"`
	// MatchConditions は、本文を組んだ関連付けの条件の選択である。
	MatchConditions []AssistMatchCondition `json:"matchConditions"`
	// PermissionRevision は、受け渡した時点の送信の許可の改訂の番号である。
	PermissionRevision int64 `json:"permissionRevision"`
}

// Validate は項目の整合を確かめる。
func (v AssistVersions) Validate() error {
	problem := firstProblem(
		requireSanitized("AssistVersions.serverRevision", v.ServerRevision),
		requireLowerHex64("AssistVersions.sourceSetSha256", v.SourceSetSha256),
	)
	if problem != nil {
		return problem
	}
	if v.AssignmentRevision < 0 || v.InterpretationRevision < 0 {
		return itemError("AssistVersions.revision", ErrNegativeCount)
	}
	if v.PermissionRevision < FirstAssistPermissionRevisionNumber {
		return itemError("AssistVersions.permissionRevision", ErrInvalid)
	}
	return validateAssistMatchConditions("AssistVersions.matchConditions", v.MatchConditions)
}

// AssistDisclosure は、server が LLM へ本文を渡した受け渡し 1 回の監査記録である。
//
// **server は本文を返す前に本記録を書く。** 書けなかった本文は返さない。
type AssistDisclosure struct {
	// Ordinal は調査の中の受け渡しの通番である。1 から始まる。
	Ordinal        int64  `json:"ordinal"`
	ConversationId string `json:"conversationId"`
	// TurnId は受け渡しを行った発言の識別子である。中継が発言ごとに付ける。
	TurnId string     `json:"turnId"`
	Tool   AssistTool `json:"tool"`
	// Request は要求の本文の JSON の文字列である。検索の条件などの証拠の値を含む。
	Request string `json:"request"`
	// RecordRefs は本文に載せた記録の短い参照である。本文に載せた順に並ぶ。
	RecordRefs []string `json:"recordRefs"`
	// Truncated は、件数の上限で本文を切り詰めたことである。
	Truncated bool `json:"truncated"`
	// BodySha256 と BodyBytes は、LLM へ渡した本文の byte 列の sha256 と長さである。
	BodySha256 string         `json:"bodySha256"`
	BodyBytes  int64          `json:"bodyBytes"`
	Versions   AssistVersions `json:"versions"`
	RecordedAt AssertionTime  `json:"recordedAt"`
}

// Validate は項目の整合を確かめる。
func (d AssistDisclosure) Validate() error {
	problem := firstProblem(
		ValidateAssistConversationId("AssistDisclosure.conversationId", d.ConversationId),
		ValidateAssistTurnId("AssistDisclosure.turnId", d.TurnId),
		requireKnownEnum("AssistDisclosure.tool", d.Tool),
		requirePresent("AssistDisclosure.request", d.Request),
		requireLowerHex64("AssistDisclosure.bodySha256", d.BodySha256),
		requireNoEmptyElement("AssistDisclosure.recordRefs", d.RecordRefs),
	)
	if problem != nil {
		return problem
	}
	if d.Ordinal < 1 {
		return itemError("AssistDisclosure.ordinal", ErrInvalid)
	}
	if d.BodyBytes < 0 {
		return itemError("AssistDisclosure.bodyBytes", ErrNegativeCount)
	}
	if err := d.Versions.Validate(); err != nil {
		return err
	}
	return d.RecordedAt.Validate()
}
