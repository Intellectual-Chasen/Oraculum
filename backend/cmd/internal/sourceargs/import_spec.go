package sourceargs

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// ImportSpec は取り込みの起動が渡した指定の記録である。
//
// oraculum-import が取り込みの後に出力し、`--import-spec` が読み戻す。読み戻した取り込みは、
// 記録と同じ収集元を同じ入力形式と欄の並びで読み、同じ案件に入れる。
type ImportSpec struct {
	// Sources は取り込んだ収集元を起動の順に並べる。
	Sources []ImportSpecSource `json:"sources"`
	// SigmaRules は取り込みのレコードに当てた Sigma のルールの集合である。出ない場合は、
	// ルールの集合を渡していない。
	SigmaRules *ImportSpecSigmaRules `json:"sigmaRules,omitempty"`
}

// ImportSpecSigmaRules は Sigma のルールの集合の記録である。読み戻した取り込みは、同じ
// directory から同じ内容の集合を読み、同じ commit であることを確かめる。
type ImportSpecSigmaRules struct {
	// Directory はルールの file を置いた directory である。
	Directory string `json:"directory"`
	// Revision は commit の文字列である。出ない場合は、commit を確かめられなかった。
	Revision *string `json:"revision,omitempty"`
	// RevisionSource は commit をどこから決めたかである (pipeline.SigmaRuleSetInfo)。
	RevisionSource string `json:"revisionSource"`
	// ContentSha256 はルールの file の path と内容から求めた識別である。小文字 16 進 64 文字。
	ContentSha256 string `json:"contentSha256"`
}

// ImportSpecSigmaRulesOf は評価に使ったルールの集合を、記録の形へ直す。
func ImportSpecSigmaRulesOf(info pipeline.SigmaRuleSetInfo) *ImportSpecSigmaRules {
	return &ImportSpecSigmaRules{
		Directory: info.Directory, Revision: clone(info.Revision), RevisionSource: info.RevisionSource,
		ContentSha256: info.ContentSha256,
	}
}

// ruleSpec は記録を、ルールの集合の指定へ直す。記録した commit と内容の識別を、読み戻した集合で
// 確かめる。
func (s ImportSpecSigmaRules) ruleSpec() pipeline.SigmaRuleSpec {
	spec := pipeline.SigmaRuleSpec{Directory: s.Directory, ExpectedContentSha256: clone(&s.ContentSha256)}
	if s.Revision != nil {
		spec.Revision = *s.Revision
	}
	return spec
}

func (s ImportSpecSigmaRules) validate() error {
	if s.Directory == "" {
		return errors.New("sigmaRules.directory is required")
	}
	if s.Revision != nil && *s.Revision == "" {
		return errors.New("sigmaRules.revision must not be empty when present")
	}
	if s.RevisionSource == "" {
		return errors.New("sigmaRules.revisionSource is required")
	}
	if !isLowerHex64(s.ContentSha256) {
		return errors.New("sigmaRules.contentSha256 must be 64 lowercase hexadecimal digits")
	}
	return nil
}

// ImportSpecSource は収集元 1 件の取り込みの指定である。
type ImportSpecSource struct {
	// CaseId は収集元に付けた案件である。出ない場合は、取り込みが案件を区別しない。
	CaseId *string `json:"caseId,omitempty"`
	// FormatKey は入力形式である。
	FormatKey core.FormatKey `json:"formatKey"`
	// OriginPath は収集元の取得元である。取り込みを起動した directory からの相対 path。
	OriginPath string `json:"originPath"`
	// FormatSpec は起動が渡した欄の並びである。出ない場合は、入力形式そのものが並びを定める。
	FormatSpec *string `json:"formatSpec,omitempty"`
	// ContentSha256 は記録した時点の収集元の内容の識別である。小文字 16 進 64 文字。
	ContentSha256 string `json:"contentSha256"`
	// Terminal は利用者が指定した、この収集元を記録した端末である。出ない場合は、利用者が
	// 端末を指定していない。
	Terminal *ImportSpecTerminal `json:"terminal,omitempty"`
	// CollectionPath は、収集元を取り出した収集の directory である。出ない場合は、収集元の
	// file を 1 件ずつ指定した。**読み戻した取り込みは directory を辿り直さない。** 記録した
	// file を読み、同じ収集の端末に置く。
	CollectionPath string `json:"collectionPath,omitempty"`
}

// ImportSpecTerminal は収集元 1 件に指定した端末である。3 項目のうち 1 つ以上を持つ。
type ImportSpecTerminal struct {
	// Id は端末の外部識別子である。
	Id string `json:"id,omitempty"`
	// Hostname は端末の表示名である。
	Hostname string `json:"hostname,omitempty"`
	// Ip は端末が持つ IP アドレスである。
	Ip string `json:"ip,omitempty"`
	// TimeOffset は、UTC からのずれを持たない収集元の時刻を読む UTC からのずれである。
	// 出ない場合は、ずれを指定していない。
	TimeOffset *core.UtcOffset `json:"timeOffset,omitempty"`
}

// importSpecTerminalOf は取り込みの計画が持つ端末を、記録の形へ直す。
func importSpecTerminalOf(terminal *pipeline.SourceTerminal) *ImportSpecTerminal {
	if terminal == nil {
		return nil
	}
	return &ImportSpecTerminal{
		Id: terminal.TerminalId, Hostname: terminal.TerminalHostname, Ip: terminal.Ip,
		TimeOffset: terminal.TimeOffset,
	}
}

// sourceTerminal は記録した端末を、取り込みの計画の形へ直す。
func (t *ImportSpecTerminal) sourceTerminal() *pipeline.SourceTerminal {
	if t == nil {
		return nil
	}
	return &pipeline.SourceTerminal{
		TerminalId: t.Id, TerminalHostname: t.Hostname, Ip: t.Ip, TimeOffset: t.TimeOffset,
	}
}

// validateTerminal は収集元に指定した端末の形を確かめる (pipeline.SourceTerminal の Validate)。
// 起動の文字列を読む段階で退け、収集元の byte を 1 つも読まずに止める。
func validateTerminal(terminal pipeline.SourceTerminal) error {
	return terminal.Validate()
}

// ImportSpecOf は取り込みの計画と取り込んだ収集元の識別から、取り込みの指定の記録を作る。
//
// 欄の並びは起動が渡した値を記録する。入力形式そのものが並びを定める形式は、並びを
// 渡した起動を退ける (core.InputFormat.FormatSpecFor) ので、解析後の並びを記録すると
// 読み戻せない。
func ImportSpecOf(plans []pipeline.SourcePlan, entries []pipeline.SourceEntry) (ImportSpec, error) {
	if len(plans) != len(entries) {
		return ImportSpec{}, fmt.Errorf("recording the import specification: %d plans and %d sources",
			len(plans), len(entries))
	}
	spec := ImportSpec{Sources: make([]ImportSpecSource, len(plans))}
	for i, plan := range plans {
		identity := entries[i].Identity
		if identity.OriginPath != plan.OriginPath {
			return ImportSpec{}, fmt.Errorf("recording the import specification: source %d is %q, planned %q",
				i, identity.OriginPath, plan.OriginPath)
		}
		spec.Sources[i] = ImportSpecSource{
			CaseId: clone(plan.CaseId), FormatKey: plan.FormatKey, OriginPath: plan.OriginPath,
			FormatSpec: clone(plan.FormatSpec), ContentSha256: identity.ContentSha256,
			Terminal: importSpecTerminalOf(plan.Terminal), CollectionPath: plan.CollectionPath,
		}
	}
	return spec, nil
}

// readImportSpec は取り込みの指定の記録を読む。知らない項目と、収集元を持たない記録と、
// 形の崩れた Sigma のルールの集合の記録を退ける。
func readImportSpec(data []byte) (ImportSpec, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var spec ImportSpec
	if err := decoder.Decode(&spec); err != nil {
		return ImportSpec{}, fmt.Errorf("decoding the import specification: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return ImportSpec{}, errors.New("decoding the import specification: trailing data after the object")
	}
	if len(spec.Sources) == 0 {
		return ImportSpec{}, errors.New("decoding the import specification: sources is empty")
	}
	if spec.SigmaRules != nil {
		if err := spec.SigmaRules.validate(); err != nil {
			return ImportSpec{}, fmt.Errorf("decoding the import specification: %w", err)
		}
	}
	return spec, nil
}

// decodeImportSpec は取り込みの指定の記録を読み、収集元ごとの計画にする。
// 知らない項目と、必須の項目を欠く記録を退ける。
func decodeImportSpec(data []byte) ([]pipeline.SourcePlan, error) {
	spec, err := readImportSpec(data)
	if err != nil {
		return nil, err
	}
	plans := make([]pipeline.SourcePlan, len(spec.Sources))
	for i, source := range spec.Sources {
		if err := source.validate(); err != nil {
			return nil, fmt.Errorf("decoding the import specification: sources[%d]: %w", i, err)
		}
		sha := source.ContentSha256
		plans[i] = pipeline.SourcePlan{
			OriginPath: source.OriginPath, FileName: filepath.Base(source.OriginPath),
			FormatKey: source.FormatKey, FormatSpec: clone(source.FormatSpec),
			CaseId: clone(source.CaseId), ExpectedContentSha256: &sha,
			Terminal: source.Terminal.sourceTerminal(), CollectionPath: source.CollectionPath,
		}
	}
	return plans, nil
}

func (s ImportSpecSource) validate() error {
	if s.FormatKey == "" {
		return errors.New("formatKey is required")
	}
	if s.OriginPath == "" || filepath.IsAbs(s.OriginPath) {
		return errors.New("originPath must be a non-empty relative path")
	}
	if s.FormatSpec != nil && *s.FormatSpec == "" {
		return errors.New("formatSpec must not be empty when present")
	}
	if s.CaseId != nil {
		if err := core.ValidateCaseId("caseId", *s.CaseId); err != nil {
			return err
		}
	}
	if s.Terminal != nil {
		if err := validateTerminal(*s.Terminal.sourceTerminal()); err != nil {
			return fmt.Errorf("terminal: %w", err)
		}
	}
	if !isLowerHex64(s.ContentSha256) {
		return errors.New("contentSha256 must be 64 lowercase hexadecimal digits")
	}
	return nil
}

func isLowerHex64(text string) bool {
	const sha256HexLength = 64
	_, err := hex.DecodeString(text)
	return err == nil && len(text) == sha256HexLength && strings.ToLower(text) == text
}

func clone(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
