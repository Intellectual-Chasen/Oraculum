package core

// ApiErrorCode は失敗の種別を持つ。
type ApiErrorCode string

// ApiErrorCode の値。HTTP status との対応は確定しており、status を扱うのは backend/api/
// である。
//
// 宣言の並びは判定の順序である。**判定そのものは本 package が持たない。** どの値の条件に
// 該当したかを決めるには要求の項目の一覧が要り、要求の型は backend/api/ が持つ。
const (
	// ApiErrorCodeNotImplemented は要求が未実装の操作を指す失敗である。
	ApiErrorCodeNotImplemented ApiErrorCode = "not_implemented"
	// ApiErrorCodeCandidateWindowMissing は候補の要求が時刻の範囲の項目を欠く失敗である。
	ApiErrorCodeCandidateWindowMissing ApiErrorCode = "candidate_window_missing"
	// ApiErrorCodeInvalidRequest は要求の項目が欠けている失敗と、要求の値が Validate の
	// 許す範囲の外にある失敗である。
	ApiErrorCodeInvalidRequest ApiErrorCode = "invalid_request"
	// ApiErrorCodeSourceNotFound は要求の sourceId に一致する収集元が無い失敗である。
	ApiErrorCodeSourceNotFound ApiErrorCode = "source_not_found"
	// ApiErrorCodeSourceHashMismatch は収集元の内容の識別が一致しない失敗である。
	ApiErrorCodeSourceHashMismatch ApiErrorCode = "source_hash_mismatch"
	// ApiErrorCodeImportWithheld は要求の範囲の結果公開を停止している失敗である。
	ApiErrorCodeImportWithheld ApiErrorCode = "import_withheld"
	// ApiErrorCodePositionOutsideSource は要求の位置が収集元の範囲の外にある失敗である。
	ApiErrorCodePositionOutsideSource ApiErrorCode = "position_outside_source"
	// ApiErrorCodeRecordNotFound は要求の位置に一致するレコードが無い失敗である。
	ApiErrorCodeRecordNotFound ApiErrorCode = "record_not_found"
	// ApiErrorCodeRecordUnreadable は、要求の位置に byte 列があり、取り込みが読めな
	// かった失敗である。
	//
	// **位置にレコードが無い状態と分けて返す。** 読めなかったレコードは、
	// `/api/v0/sources` の取り込みの状態が診断と位置を持つ。
	ApiErrorCodeRecordUnreadable ApiErrorCode = "record_unreadable"
	// ApiErrorCodeStageNotReady は、要求が必要とする調査の段階 (収集元の読み込み、処理) を
	// まだ終えていない失敗である。分析者がその段階を始めて終えるまで、同じ要求は通らない。
	ApiErrorCodeStageNotReady ApiErrorCode = "stage_not_ready"
	// ApiErrorCodeStageAlreadyStarted は、始めようとした段階が実行中か完了済みである失敗である。
	ApiErrorCodeStageAlreadyStarted ApiErrorCode = "stage_already_started"
	// ApiErrorCodeSourceUploadAlreadyExists は同じ保存先へ資料を再送した失敗である。
	ApiErrorCodeSourceUploadAlreadyExists ApiErrorCode = "source_upload_already_exists"
	// ApiErrorCodeTerminalAssignmentAlreadyRecorded は、記録しようとした端末の割当と同じ内容の
	// 割当を既に記録している失敗である。
	ApiErrorCodeTerminalAssignmentAlreadyRecorded ApiErrorCode = "terminal_assignment_already_recorded"
	// ApiErrorCodeRequestOriginRejected は、要求の Host が server の許可した待ち受け先に一致しない
	// 失敗と、状態を変える要求を別の origin から送った失敗である。
	ApiErrorCodeRequestOriginRejected ApiErrorCode = "request_origin_rejected"
	// ApiErrorCodeAuthenticationRequired は、アカウントを持つ server への要求が、使えるセッションを
	// 持たない失敗である。セッションが無い、切れた、失効した、利用者を無効にした、を区別しない。
	ApiErrorCodeAuthenticationRequired ApiErrorCode = "authentication_required"
	// ApiErrorCodeLoginRejected は、ログイン名とパスワードの組を受け付けない失敗である。
	ApiErrorCodeLoginRejected ApiErrorCode = "login_rejected"
	// ApiErrorCodeLoginRateLimited は、同じログイン名のログインが直近に続けて失敗し、しばらく
	// 受け付けない失敗である。
	ApiErrorCodeLoginRateLimited ApiErrorCode = "login_rate_limited"
	// ApiErrorCodeInvestigationNotFound は、ログインした利用者が調査の役割を持たない失敗である。
	// 調査の存在を返さない。
	ApiErrorCodeInvestigationNotFound ApiErrorCode = "investigation_not_found"
	// ApiErrorCodePermissionDenied は、ログインした利用者の役割が操作を許さない失敗である。
	ApiErrorCodePermissionDenied ApiErrorCode = "permission_denied"
	// ApiErrorCodeWorkspaceChanged は、ワークスペースを置き換える要求が元にした revision より後に、
	// 別の置き換えを記録した失敗である。画面は最新の値を読み直してから置き換える。
	ApiErrorCodeWorkspaceChanged ApiErrorCode = "workspace_changed"
	// ApiErrorCodeAssertionChanged は、所見の改訂が元にした revision より後に別の改訂を記録した失敗と、
	// 収集元の時刻の解釈を別の分析者が先に記録した失敗である。応答は現在の所見を conflict に持つ。
	ApiErrorCodeAssertionChanged ApiErrorCode = "assertion_changed"
	// ApiErrorCodeAssistUnavailable は、調査の directory を渡さない起動が AI 支援の操作を受けた
	// 失敗である。AI 支援の記録は調査の file に置くため、この起動の間は同じ要求が通らない。
	ApiErrorCodeAssistUnavailable ApiErrorCode = "assist_unavailable"
	// ApiErrorCodeAssistNotPermitted は、調査が提供者への証拠の送信を現在の改訂で許可していない失敗で
	// ある。分析者が許可を記録するまで、同じ要求は通らない。
	ApiErrorCodeAssistNotPermitted ApiErrorCode = "assist_not_permitted"
	// ApiErrorCodeConversationNotFound は、要求の会話の識別子に一致する会話が無い失敗である。
	ApiErrorCodeConversationNotFound ApiErrorCode = "conversation_not_found"
	// ApiErrorCodeAssistProposalDecided は、採否を決めた後の AI 提案の採用または却下を退けた失敗である。
	// 応答は現在の提案を conflict に持つ。
	ApiErrorCodeAssistProposalDecided ApiErrorCode = "assist_proposal_decided"
	// ApiErrorCodeInternalError は上のいずれにも該当しない失敗である。
	ApiErrorCodeInternalError ApiErrorCode = "internal_error"
)

// ApiErrorCodes は ApiErrorCode の値を判定の順序で返す。
//
// HTTP status を割り当てる backend/api/ が、割り当て漏れの無いことを値の並びから
// 確かめられるように公開する。
func ApiErrorCodes() []ApiErrorCode {
	return []ApiErrorCode{
		ApiErrorCodeNotImplemented,
		ApiErrorCodeCandidateWindowMissing,
		ApiErrorCodeInvalidRequest,
		ApiErrorCodeSourceNotFound,
		ApiErrorCodeSourceHashMismatch,
		ApiErrorCodeImportWithheld,
		ApiErrorCodePositionOutsideSource,
		ApiErrorCodeRecordNotFound,
		ApiErrorCodeRecordUnreadable,
		ApiErrorCodeStageNotReady,
		ApiErrorCodeStageAlreadyStarted,
		ApiErrorCodeSourceUploadAlreadyExists,
		ApiErrorCodeTerminalAssignmentAlreadyRecorded,
		ApiErrorCodeRequestOriginRejected,
		ApiErrorCodeAuthenticationRequired,
		ApiErrorCodeLoginRejected,
		ApiErrorCodeLoginRateLimited,
		ApiErrorCodeInvestigationNotFound,
		ApiErrorCodePermissionDenied,
		ApiErrorCodeWorkspaceChanged,
		ApiErrorCodeAssertionChanged,
		ApiErrorCodeAssistUnavailable,
		ApiErrorCodeAssistNotPermitted,
		ApiErrorCodeConversationNotFound,
		ApiErrorCodeAssistProposalDecided,
		ApiErrorCodeInternalError,
	}
}

// IsKnown は ApiErrorCode が定義の中の値であるかを返す。
func (c ApiErrorCode) IsKnown() bool {
	for _, known := range ApiErrorCodes() {
		if c == known {
			return true
		}
	}
	return false
}

// ApiError は API の失敗を持つ。
//
// Message は英語で書き、token と credential を載せない。原資料の byte 列を
// そのまま載せず、制御文字と改行を持たない表現を載せる。原文は `/api/v0/records` が返す。
type ApiError struct {
	// Code は失敗の種別である。
	Code ApiErrorCode `json:"code"`
	// Message は失敗の内容である。
	Message string `json:"message"`
	// MissingParameters は欠けている要求の項目の集合である。欠けている要求の項目が
	// あるとき必須で、要素数は 1 以上である。出ない場合は要求の項目が欠けていない。
	//
	// Code が invalid_request の失敗のうち、limit に 0 を与える要求と、RecordField の
	// kind を欠いた object を受け取る要求と、RecordField が text と timestamp を同時に
	// 持つ要求は、要求の項目が欠けていない失敗である。同じ要求がその操作の必須の項目を
	// すべて揃えているとき、応答は本項目を出さない。
	MissingParameters []string `json:"missingParameters,omitempty"`
	// SourceId は失敗に関わる収集元である。出ない場合は失敗が収集元に関わらない。
	SourceId string `json:"sourceId,omitempty"`
	// SourceContentSha256 は失敗に関わる収集元の内容の識別である。SourceId があるとき
	// 必須で、SourceId が出ないとき出ない。出ない場合は失敗が収集元に関わらない。
	SourceContentSha256 string `json:"sourceContentSha256,omitempty"`
	// OriginPath は Code が source_hash_mismatch のとき必須である。
	// 値は要求の SourceId に対応する SourceIdentity の OriginPath である。読み込みの要求を
	// 退けた invalid_request では、退けた収集元の、要求が指した path である。
	OriginPath string `json:"originPath,omitempty"`
	// RecordRef は失敗に関わるレコード位置である。
	// 出ない場合は失敗がレコードに関わらない。
	RecordRef *RecordLocator `json:"recordRef,omitempty"`
	// ImportStatusRef は Code が import_withheld のとき必須である。
	ImportStatusRef string `json:"importStatusRef,omitempty"`
	// LoadingRejection は読み込みの要求を退けた理由である。Code が invalid_request の
	// ときだけ出る。出ない場合は失敗が読み込みの要求の検査に関わらない。
	LoadingRejection LoadingRejection `json:"loadingRejection,omitempty"`
	// SearchExpressionError は検索式を退けた理由と誤りの範囲である。Code が invalid_request の
	// ときだけ出る。出ない場合は失敗が検索式の構文に関わらない。
	SearchExpressionError *SearchExpressionError `json:"searchExpressionError,omitempty"`
}

// Validate は項目の整合と、Code ごとの項目の必須・省略条件を確かめる。
func (e ApiError) Validate() error {
	problem := firstProblem(
		requireKnownEnum("ApiError.code", e.Code),
		requirePresent("ApiError.message", e.Message),
		requireSanitized("ApiError.message", e.Message),
		requireBothOrNeither("ApiError.sourceId", e.SourceId,
			"ApiError.sourceContentSha256", e.SourceContentSha256),
	)
	if problem != nil {
		return problem
	}
	if e.SourceContentSha256 != "" {
		sha256Problem := requireLowerHex64("ApiError.sourceContentSha256", e.SourceContentSha256)
		if sha256Problem != nil {
			return sha256Problem
		}
	}
	if e.RecordRef != nil {
		if recordProblem := e.RecordRef.Validate(); recordProblem != nil {
			return itemError("ApiError.recordRef", recordProblem)
		}
	}
	if problem := requireNoEmptyElement("ApiError.missingParameters",
		e.MissingParameters); problem != nil {
		return problem
	}
	return e.validateCodeSpecificItems()
}

// validateCodeSpecificItems は Code ごとに必須になる項目と、出てはいけない項目を確かめる。
//
// candidate_window_missing を返す条件はどれも要求の項目が欠けている失敗であり、応答は
// missingParameters を出す。invalid_request は要求の項目が欠けていない失敗 (limit に 0 を
// 与える要求など) を含むため、本項目を必須にしない。
func (e ApiError) validateCodeSpecificItems() error {
	if e.LoadingRejection != "" {
		if e.Code != ApiErrorCodeInvalidRequest {
			return itemError("ApiError.loadingRejection", ErrUnexpectedItem)
		}
		if problem := requireKnownEnum("ApiError.loadingRejection", e.LoadingRejection); problem != nil {
			return problem
		}
	}
	if e.SearchExpressionError != nil {
		if e.Code != ApiErrorCodeInvalidRequest {
			return itemError("ApiError.searchExpressionError", ErrUnexpectedItem)
		}
		if problem := e.SearchExpressionError.Validate(); problem != nil {
			return itemError("ApiError.searchExpressionError", problem)
		}
	}
	switch e.Code {
	case ApiErrorCodeCandidateWindowMissing:
		if len(e.MissingParameters) == 0 {
			return itemError("ApiError.missingParameters", ErrMissingRequiredItem)
		}
	case ApiErrorCodeSourceHashMismatch:
		// 要求の sourceContentSha256 と収集元の内容の識別を比べた結果であり、収集元に
		// 関わらない失敗にならない。
		return firstProblem(
			requirePresent("ApiError.sourceId", e.SourceId),
			requirePresent("ApiError.sourceContentSha256", e.SourceContentSha256),
			requirePresent("ApiError.originPath", e.OriginPath),
		)
	case ApiErrorCodeImportWithheld:
		return requirePresent("ApiError.importStatusRef", e.ImportStatusRef)
	}
	return nil
}
