package sigma

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Rule は評価できる Sigma のルール 1 つである。
type Rule struct {
	// Path はルールの集合の directory からの file の path である。区切りは `/`。
	// ルールの集合の中でルールを 1 つに決める。
	Path string
	// ID、Title、Author、Level、Status はルールの file の同じ名前の項目である。file に無い
	// 項目は空である。
	ID     string
	Title  string
	Author string
	Level  string
	Status string
	// Condition は detection の condition の文字列である。
	Condition string
	// Selections は detection の検索を file の順で持つ。
	Selections []Selection

	selections []selection
	condition  condition
	targets    []target
}

// Selection は detection の検索 1 つの名前と定義である。
type Selection struct {
	Name string
	// Definition は file に書かれた定義を YAML に書き直したものである。値と構造は file と同じで
	// あり、字下げ、引用符、コメントの位置は file と異なりうる。
	Definition string
}

// Unevaluated は評価しなかったルールの file 1 つである。
type Unevaluated struct {
	// Path、ID、Title は Rule と同じである。YAML を読めなかった file では ID と Title が空である。
	Path  string
	ID    string
	Title string
	// Reason は評価しなかった理由の分類 (Reason で始まる定数) である。
	Reason string
	// Detail は理由の説明の文である。
	Detail string
}

// RuleSet は directory 1 つから読んだルールの集合である。
type RuleSet struct {
	// Rules と Unevaluated は path の文字列の順に並ぶ。
	Rules       []Rule
	Unevaluated []Unevaluated
	// ContentSha256 は、読んだ file の path と内容を fs.WalkDir が読んだ順 (directory ごとに
	// 名前の順) に連ねた byte 列の sha256 である。小文字 16 進 64 文字。同じ file の集合から
	// 同じ値になる。
	ContentSha256 string
	// FileCount は読んだルールの file の数である。
	FileCount int

	index ruleIndex
}

// ruleFile は Sigma のルールの file の項目のうち、評価と表示に使うものである。
type ruleFile struct {
	ID        string     `yaml:"id"`
	Title     string     `yaml:"title"`
	Author    yaml.Node  `yaml:"author"`
	Level     string     `yaml:"level"`
	Status    string     `yaml:"status"`
	Logsource *logsource `yaml:"logsource"`
	Detection yaml.Node  `yaml:"detection"`
	Action    string     `yaml:"action"`
}

// authorOf は author の値を 1 つの文字列にする。list で書いた著者は `, ` でつなぐ。
func authorOf(node yaml.Node) string {
	if node.Kind == yaml.SequenceNode {
		names := make([]string, 0, len(node.Content))
		for _, element := range node.Content {
			names = append(names, element.Value)
		}
		return strings.Join(names, ", ")
	}
	return node.Value
}

// Load は fsys の中の `.yml` と `.yaml` の file をすべて読む。
//
// 評価できない file は Unevaluated に理由を付けて入れ、読み続ける。file を読めない
// I/O の失敗と、ルールの file が 1 つも無い directory は error を返す。`.` で始まる
// directory (`.git` など) は読まない。
func Load(fsys fs.FS) (RuleSet, error) {
	var set RuleSet
	digest := sha256.New()
	err := fs.WalkDir(fsys, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walking the rule directory at %q: %w", name, walkErr)
		}
		if entry.IsDir() {
			if name != "." && strings.HasPrefix(entry.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			return nil
		}
		content, err := fs.ReadFile(fsys, name)
		if err != nil {
			return fmt.Errorf("reading the rule file %q: %w", name, err)
		}
		// hash.Hash の Write は error を返さない。
		_, _ = fmt.Fprintf(digest, "%d:%s%d:", len(name), name, len(content))
		_, _ = digest.Write(content)
		set.FileCount++
		rule, problem := parseRule(name, content)
		if problem != nil {
			set.Unevaluated = append(set.Unevaluated, *problem)
			return nil
		}
		set.Rules = append(set.Rules, rule)
		return nil
	})
	if err != nil {
		return RuleSet{}, err
	}
	if set.FileCount == 0 {
		return RuleSet{}, errors.New("the rule directory has no .yml or .yaml file")
	}
	set.ContentSha256 = hex.EncodeToString(digest.Sum(nil))
	slices.SortFunc(set.Rules, func(a, b Rule) int { return strings.Compare(a.Path, b.Path) })
	slices.SortFunc(set.Unevaluated, func(a, b Unevaluated) int { return strings.Compare(a.Path, b.Path) })
	set.index = indexRules(set.Rules)
	return set, nil
}

// parseRule は file 1 つを読む。評価できない file には理由を返す。
func parseRule(name string, content []byte) (Rule, *Unevaluated) {
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	var file ruleFile
	if err := decoder.Decode(&file); err != nil {
		return Rule{}, &Unevaluated{Path: name, Reason: ReasonFileUnreadable, Detail: err.Error()}
	}
	skipped := func(reason, detail string) (Rule, *Unevaluated) {
		return Rule{}, &Unevaluated{Path: name, ID: file.ID, Title: file.Title, Reason: reason, Detail: detail}
	}
	var another yaml.Node
	switch err := decoder.Decode(&another); {
	case err == nil:
		return skipped(ReasonRuleCollection, "the file holds more than one YAML document")
	case !errors.Is(err, io.EOF):
		return Rule{}, &Unevaluated{Path: name, ID: file.ID, Title: file.Title, Reason: ReasonFileUnreadable, Detail: err.Error()}
	}
	if file.Action != "" {
		return skipped(ReasonRuleCollection, "the file declares action "+file.Action)
	}
	if file.Detection.Kind == 0 || file.Logsource == nil {
		return skipped(ReasonNotDetectionRule, "the file has no logsource or detection")
	}
	targets, reason := file.Logsource.targets()
	if reason != "" {
		return skipped(ReasonLogsourceUnsupported, reason)
	}
	rule := Rule{
		Path: name, ID: file.ID, Title: file.Title, Author: authorOf(file.Author), Level: file.Level,
		Status: file.Status, targets: targets,
	}
	if problem := rule.compileDetection(&file.Detection); problem != nil {
		return skipped(problem.reason, problem.detail)
	}
	if problem := rule.compileTargets(); problem != nil {
		return skipped(problem.reason, problem.detail)
	}
	return rule, nil
}

// compileTargets は、ルールが参照する項目をイベントが記録しない target を外し、残る target に
// 参照する項目名を埋める。
//
// **イベントが記録しない項目を「値が無い」と扱わない。** 扱うと `not filter` の filter が
// 成り立たず、Sysmon の 1 を前提にしたルールが Security の 4688 に一致する。
func (r *Rule) compileTargets() *unsupported {
	referenced := map[string]struct{}{}
	for _, sel := range r.selections {
		for _, items := range sel.alternatives {
			for _, item := range items {
				referenced[item.field] = struct{}{}
			}
		}
	}
	names := make([]string, 0, len(referenced))
	for name := range referenced {
		names = append(names, name)
	}
	slices.Sort(names)
	var kept []target
	var reasons []string
	for _, candidate := range r.targets {
		var missing []string
		candidate.used = make([]string, 0, len(names))
		for _, name := range names {
			mapped := name
			if renamed, found := candidate.fieldNames[name]; found {
				mapped = renamed
			}
			candidate.used = append(candidate.used, mapped)
			_, system := systemFields[name]
			_, recorded := candidate.fields[mapped]
			if candidate.fields != nil && !system && !recorded {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			reasons = append(reasons, "fields "+strings.Join(missing, ", ")+" are not recorded by "+
				candidate.service+" event "+strings.Join(candidate.eventIDs, ", "))
			continue
		}
		kept = append(kept, candidate)
	}
	if len(kept) == 0 {
		return &unsupported{ReasonFieldUnavailable, strings.Join(reasons, "; ")}
	}
	r.targets = kept
	return nil
}

// compileDetection は detection の検索と condition を読む。
func (r *Rule) compileDetection(detection *yaml.Node) *unsupported {
	if detection.Kind != yaml.MappingNode {
		return &unsupported{ReasonNotDetectionRule, "detection is not a map"}
	}
	var names []string
	hasCondition := false
	for at := 0; at+1 < len(detection.Content); at += 2 {
		key, value := detection.Content[at].Value, detection.Content[at+1]
		switch key {
		case "condition":
			if value.Kind != yaml.ScalarNode {
				return &unsupported{ReasonConditionUnsupported, "condition is not a single expression"}
			}
			r.Condition, hasCondition = value.Value, true
			continue
		case "timeframe":
			return &unsupported{ReasonConditionUnsupported, "detection has a timeframe"}
		}
		parsed, err := parseSelection(key, value)
		if err != nil {
			problem := asUnsupported(err)
			return &problem
		}
		definition, err := yaml.Marshal(value)
		if err != nil {
			return &unsupported{ReasonValueUnsupported, "selection " + key + " cannot be written back: " + err.Error()}
		}
		names = append(names, key)
		r.selections = append(r.selections, parsed)
		r.Selections = append(r.Selections, Selection{Name: key, Definition: string(definition)})
	}
	if !hasCondition {
		return &unsupported{ReasonNotDetectionRule, "detection has no condition"}
	}
	parsed, err := parseCondition(r.Condition, names)
	if err != nil {
		problem := asUnsupported(err)
		return &problem
	}
	r.condition = parsed
	return nil
}

func asUnsupported(err error) unsupported {
	var problem unsupported
	if errors.As(err, &problem) {
		return problem
	}
	return unsupported{ReasonValueUnsupported, err.Error()}
}
