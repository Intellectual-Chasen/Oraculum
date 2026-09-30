package core

import "fmt"

// TerminalCategory は、端末のレコードを調べる観点で分けた分類である。1 件のレコードは 0 個以上の
// 分類に入る。
type TerminalCategory string

// TerminalCategory の値。
const (
	// TerminalCategoryRemoteLogon は、別の端末からのログオンの試行と成功と、リモート デスクトップの
	// 接続である。
	TerminalCategoryRemoteLogon TerminalCategory = "remote_logon"
	// TerminalCategoryAccountManagement は、アカウントの作成・有効化・パスワードの設定・変更・
	// 削除と、グループへのメンバーの追加である。
	TerminalCategoryAccountManagement TerminalCategory = "account_management"
	// TerminalCategoryProgramExecution は、Prefetch と Amcache が記録したプログラムの実行と配置である。
	TerminalCategoryProgramExecution TerminalCategory = "program_execution"
	// TerminalCategoryTaskServiceRegistration は、タスクとサービスの登録である。
	TerminalCategoryTaskServiceRegistration TerminalCategory = "task_service_registration"
	// TerminalCategoryPowerShell は、PowerShell が実行したスクリプトブロックである。
	TerminalCategoryPowerShell TerminalCategory = "powershell"
	// TerminalCategoryInstallation は、Windows Installer によるインストールである。
	TerminalCategoryInstallation TerminalCategory = "installation"
	// TerminalCategoryDefenseEvasion は、Defender のリアルタイム保護の無効化とイベントログの消去である。
	TerminalCategoryDefenseEvasion TerminalCategory = "defense_evasion"
)

// TerminalCategories は分類の全ての値を、画面に並べる順で返す。
func TerminalCategories() []TerminalCategory {
	return []TerminalCategory{
		TerminalCategoryRemoteLogon, TerminalCategoryAccountManagement, TerminalCategoryProgramExecution,
		TerminalCategoryTaskServiceRegistration, TerminalCategoryPowerShell, TerminalCategoryInstallation,
		TerminalCategoryDefenseEvasion,
	}
}

// IsKnown は契約が定める値かを返す。
func (c TerminalCategory) IsKnown() bool {
	for _, known := range TerminalCategories() {
		if c == known {
			return true
		}
	}
	return false
}

// RemoteLogonOutcome は、遠隔のログオンのレコードが記録した結果である。
type RemoteLogonOutcome string

// RemoteLogonOutcome の値。
const (
	// RemoteLogonOutcomeFailure はログオンの失敗 (4625) である。
	RemoteLogonOutcomeFailure RemoteLogonOutcome = "failure"
	// RemoteLogonOutcomeSuccess はログオンの成功と、認証に成功したリモート デスクトップの接続である。
	RemoteLogonOutcomeSuccess RemoteLogonOutcome = "success"
	// RemoteLogonOutcomeConnection は、認証の前の接続の受け付けである。
	RemoteLogonOutcomeConnection RemoteLogonOutcome = "connection"
)

// TerminalSummary は端末の一覧の 1 行である。
type TerminalSummary struct {
	// Node は端末のノードである。
	Node GraphNode `json:"node"`
	// Names は端末が名乗った名前である。並びは最初に名乗った順である。
	Names []string `json:"names"`
	// OperatingSystem は OS の名前、バージョン、ビルド番号を空白で繋いだ文字列である。
	// registry を持たない端末では出ない。
	OperatingSystem string `json:"operatingSystem,omitempty"`
	// RecordCount は端末に置いたレコードの件数である。
	RecordCount int64 `json:"recordCount"`
	// CategorizedRecordCount は、いずれかの分類に入るレコードの件数である。
	CategorizedRecordCount int64 `json:"categorizedRecordCount"`
}

// Validate は項目の整合を確かめる。
func (s TerminalSummary) Validate() error {
	if err := s.Node.Validate(); err != nil {
		return itemError("TerminalSummary.node", err)
	}
	if s.Names == nil {
		return itemError("TerminalSummary.names", ErrMissingRequiredItem)
	}
	if s.RecordCount < 0 || s.CategorizedRecordCount < 0 {
		return itemError("TerminalSummary.recordCount", ErrNegativeCount)
	}
	if s.CategorizedRecordCount > s.RecordCount {
		return itemError("TerminalSummary.categorizedRecordCount", ErrInconsistentValue)
	}
	return nil
}

// TerminalProfileEntry は、registry の値 1 つと、その値を記録した key のレコードである。
type TerminalProfileEntry struct {
	Name      string        `json:"name"`
	Value     string        `json:"value"`
	RecordRef RecordLocator `json:"recordRef"`
}

// TerminalNameEntry は端末が名乗った名前 1 つと、その名前を記録したレコードである。
type TerminalNameEntry struct {
	Name string `json:"name"`
	// First と Last は名前を記録したレコードの時刻のうち最も早い時刻と最も遅い時刻である。
	First      *Timestamp      `json:"first,omitempty"`
	Last       *Timestamp      `json:"last,omitempty"`
	RecordRefs []RecordLocator `json:"recordRefs"`
}

// TerminalAddressPeriod はネットワークのインターフェース 1 つが記録した IP アドレスと、保持した期間である。
type TerminalAddressPeriod struct {
	Interface string                 `json:"interface"`
	Values    []TerminalProfileEntry `json:"values"`
	// From は DHCP のリースを得た時刻、To は同じ収集の最も遅いレコードの時刻である。リースの時刻を
	// 持たないインターフェースでは出ない。
	From *Timestamp `json:"from,omitempty"`
	To   *Timestamp `json:"to,omitempty"`
}

// TerminalRemoteLogonSummary は、1 つの接続元 IP からの遠隔のログオンのレコードを数えた結果である。
type TerminalRemoteLogonSummary struct {
	SourceIp        string `json:"sourceIp"`
	FailureCount    int64  `json:"failureCount"`
	SuccessCount    int64  `json:"successCount"`
	ConnectionCount int64  `json:"connectionCount"`
	// AttemptedAccounts は失敗したログオンの対象のアカウント、LoggedOnAccounts は成功した
	// ログオンの対象のアカウントである。`ドメイン\名前` か名前の形で、最初に記録した順に並ぶ。
	AttemptedAccounts []string `json:"attemptedAccounts"`
	LoggedOnAccounts  []string `json:"loggedOnAccounts"`
	// First と Last は、時点を持つレコードの時刻のうち最も早い時刻と最も遅い時刻である。
	First *Timestamp `json:"first,omitempty"`
	Last  *Timestamp `json:"last,omitempty"`
	// FirstRecordRef と LastRecordRef は、First と Last の時刻を持つレコードである。
	FirstRecordRef *RecordLocator `json:"firstRecordRef,omitempty"`
	LastRecordRef  *RecordLocator `json:"lastRecordRef,omitempty"`
}

// Validate は項目の整合を確かめる。
func (s TerminalRemoteLogonSummary) Validate() error {
	if s.SourceIp == "" {
		return itemError("TerminalRemoteLogonSummary.sourceIp", ErrMissingRequiredItem)
	}
	if s.FailureCount < 0 || s.SuccessCount < 0 || s.ConnectionCount < 0 {
		return itemError("TerminalRemoteLogonSummary.failureCount", ErrNegativeCount)
	}
	if s.AttemptedAccounts == nil || s.LoggedOnAccounts == nil {
		return itemError("TerminalRemoteLogonSummary.attemptedAccounts", ErrMissingRequiredItem)
	}
	if (s.First == nil) != (s.FirstRecordRef == nil) || (s.Last == nil) != (s.LastRecordRef == nil) {
		return itemError("TerminalRemoteLogonSummary.firstRecordRef", ErrInconsistentValue)
	}
	return nil
}

// TerminalCategoryCount は、端末の 1 つの分類のレコードの件数と、分類に要る収集元の種類の有無である。
type TerminalCategoryCount struct {
	Category    TerminalCategory `json:"category"`
	RecordCount int64            `json:"recordCount"`
	// RequiredSources は分類のレコードを記録する収集元の種類である。Windows イベントログは
	// チャネルの名前、ほかは prefetch・amcache・task_cache である。
	RequiredSources []string `json:"requiredSources"`
	// PresentSources は RequiredSources のうち、端末に置いたレコードを持つ種類である。
	// 要素数 0 のとき、件数 0 は「記録が無い」と読めず「収集元が無い」と読む。
	PresentSources []string `json:"presentSources"`
}

// Validate は項目の整合を確かめる。
func (c TerminalCategoryCount) Validate() error {
	if !c.Category.IsKnown() {
		return itemError("TerminalCategoryCount.category", ErrUnknownEnumValue)
	}
	if c.RecordCount < 0 {
		return itemError("TerminalCategoryCount.recordCount", ErrNegativeCount)
	}
	if c.RequiredSources == nil || c.PresentSources == nil {
		return itemError("TerminalCategoryCount.presentSources", ErrMissingRequiredItem)
	}
	return nil
}

// TerminalDetail は端末 1 台の情報である。
type TerminalDetail struct {
	Node            GraphNode                    `json:"node"`
	OperatingSystem []TerminalProfileEntry       `json:"operatingSystem"`
	Names           []TerminalNameEntry          `json:"names"`
	Addresses       []TerminalAddressPeriod      `json:"addresses"`
	TimeZone        []TerminalProfileEntry       `json:"timeZone"`
	RemoteLogons    []TerminalRemoteLogonSummary `json:"remoteLogons"`
	// Categories は全ての分類を TerminalCategories の順に持つ。
	Categories []TerminalCategoryCount `json:"categories"`
}

// Validate は項目の整合を確かめる。
func (d TerminalDetail) Validate() error {
	if err := d.Node.Validate(); err != nil {
		return itemError("TerminalDetail.node", err)
	}
	for index, summary := range d.RemoteLogons {
		if err := summary.Validate(); err != nil {
			return itemError(fmt.Sprintf("TerminalDetail.remoteLogons[%d]", index), err)
		}
	}
	if len(d.Categories) != len(TerminalCategories()) {
		return itemError("TerminalDetail.categories", ErrInconsistentValue)
	}
	for index, count := range d.Categories {
		if err := count.Validate(); err != nil {
			return itemError(fmt.Sprintf("TerminalDetail.categories[%d]", index), err)
		}
	}
	return nil
}

// TerminalEvent は端末の分類に入るレコード 1 件である。
type TerminalEvent struct {
	GraphEvidence
	// Categories はレコードが入る分類である。要素数は 1 以上である。
	Categories []TerminalCategory `json:"categories"`
	// OtherEventTimes は、レコードが事象の時刻のほかに記録した事象の時刻である (Prefetch の
	// 過去の実行時刻など)。
	OtherEventTimes []Timestamp `json:"otherEventTimes"`
	// Fields はレコードの欄のうち語彙の項目を持つ欄である。
	Fields []RecordField `json:"fields"`
	// RecordNode はレコードのノードである。ノードを持たないレコードでは出ない。
	RecordNode *GraphNode `json:"recordNode,omitempty"`
	// NamedNodes は、レコードを根拠に持つ、端末とレコードを除くノード (アカウント、ファイル、
	// プロセス、IP など) である。
	NamedNodes []GraphNode `json:"namedNodes"`
	// OriginalFileNameDiffers は、レコードの file.original_file_name と file.name が、大文字と
	// 小文字を区別せずに比べて異なるかである。
	OriginalFileNameDiffers bool `json:"originalFileNameDiffers"`
	// LogonOutcome は、遠隔のログオンの分類に入るレコードが記録した結果である。ほかのレコードでは出ない。
	LogonOutcome RemoteLogonOutcome `json:"logonOutcome,omitempty"`
}

// Validate は項目の整合を確かめる。
func (e TerminalEvent) Validate() error {
	if err := e.GraphEvidence.Validate(); err != nil {
		return err
	}
	if len(e.Categories) == 0 {
		return itemError("TerminalEvent.categories", ErrMissingRequiredItem)
	}
	for _, category := range e.Categories {
		if !category.IsKnown() {
			return itemError("TerminalEvent.categories", ErrUnknownEnumValue)
		}
	}
	for _, field := range e.Fields {
		if err := field.Validate(); err != nil {
			return itemError("TerminalEvent.fields", err)
		}
	}
	if e.OtherEventTimes == nil || e.Fields == nil || e.NamedNodes == nil {
		return itemError("TerminalEvent.namedNodes", ErrMissingRequiredItem)
	}
	return nil
}
