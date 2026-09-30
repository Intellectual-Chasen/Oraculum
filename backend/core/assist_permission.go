package core

import "strconv"

// AssistPermissionAction は、送信の許可の改訂が許可と取り消しのどちらを記録したかである。
type AssistPermissionAction string

// AssistPermissionAction の値。
const (
	// AssistPermissionActionGrant は、提供者へ証拠を送ることを許可した改訂である。
	AssistPermissionActionGrant AssistPermissionAction = "grant"
	// AssistPermissionActionRevoke は、許可を取り消した改訂である。
	AssistPermissionActionRevoke AssistPermissionAction = "revoke"
)

// IsKnown は値が定義の中にあることを返す。
func (a AssistPermissionAction) IsKnown() bool {
	return a == AssistPermissionActionGrant || a == AssistPermissionActionRevoke
}

// FirstAssistPermissionRevisionNumber は、提供者ごとの送信の許可の最初の改訂の番号である。
const FirstAssistPermissionRevisionNumber int64 = 1

// AssistPermissionRevision は、調査 1 件の中で、提供者 1 つへの送信の許可を変えた改訂 1 つである。
//
// **改訂を消さず、足すだけにする。** 許可と取り消しの履歴は、どの受け渡しが許可の下で行われたかを
// 後から確かめる材料である。現在の状態は、提供者ごとの最後の改訂が決める。
type AssistPermissionRevision struct {
	// Provider は証拠を送る提供者である。
	Provider AssistProvider `json:"provider"`
	// RevisionNumber は提供者ごとの改訂の番号である。1 から始まり、改訂ごとに 1 つ増える。
	RevisionNumber int64 `json:"revisionNumber"`
	// Action は許可と取り消しのどちらを記録したかである。
	Action AssistPermissionAction `json:"action"`
	// Analyst は改訂を記録した分析者の名前である。分析者が入力した文字列であり、認証を経ない。
	Analyst string `json:"analyst"`
	// RecordedAt は改訂を記録した時刻である。
	RecordedAt AssertionTime `json:"recordedAt"`
}

// Validate は項目の整合を確かめる。
func (r AssistPermissionRevision) Validate() error {
	problem := firstProblem(
		requireKnownEnum("AssistPermissionRevision.provider", r.Provider),
		requireKnownEnum("AssistPermissionRevision.action", r.Action),
		requirePresent("AssistPermissionRevision.analyst", r.Analyst),
		requireSanitized("AssistPermissionRevision.analyst", r.Analyst),
	)
	if problem != nil {
		return problem
	}
	if r.RevisionNumber < FirstAssistPermissionRevisionNumber {
		return itemError("AssistPermissionRevision.revisionNumber "+strconv.FormatInt(r.RevisionNumber, 10),
			ErrInvalid)
	}
	return r.RecordedAt.Validate()
}

// AssistPermitted は、改訂の並びの中で provider の最後の改訂が許可であるかを返す。改訂を持たない
// 提供者は許可していない。revisions は記録した順に並ぶ。
func AssistPermitted(revisions []AssistPermissionRevision, provider AssistProvider) bool {
	permitted := false
	for _, revision := range revisions {
		if revision.Provider == provider {
			permitted = revision.Action == AssistPermissionActionGrant
		}
	}
	return permitted
}
