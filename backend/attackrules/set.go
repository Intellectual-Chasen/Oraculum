package attackrules

import (
	"fmt"
)

// SetInfo は rule set の読み込み元と内容を識別する。
type SetInfo struct {
	Directory      string `json:"directory"`
	FileCount      int    `json:"fileCount"`
	ContentSha256  string `json:"contentSha256"`
	Revision       string `json:"revision"`
	RevisionSource string `json:"revisionSource"`
}

// Set は検証済み rule と、その読み込み元の情報を持つ。
type Set struct {
	Rules []Rule
	Info  SetInfo
}

// Validate は rule set 内の ID と相互参照を検証する。
func (s Set) Validate() error {
	ids := make(map[string]struct{}, len(s.Rules))
	for _, rule := range s.Rules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("invalid ATT&CK rule %q: %w", rule.ID, err)
		}
		if _, duplicate := ids[rule.ID]; duplicate {
			return fmt.Errorf("duplicate ATT&CK rule id %q", rule.ID)
		}
		ids[rule.ID] = struct{}{}
	}
	for _, rule := range s.Rules {
		seen := make(map[string]struct{}, len(rule.DistinguishesFrom))
		for _, other := range rule.DistinguishesFrom {
			if _, duplicate := seen[other]; duplicate {
				return fmt.Errorf("rule %q repeats distinguishes_from rule %q", rule.ID, other)
			}
			seen[other] = struct{}{}
			if _, found := ids[other]; !found {
				return fmt.Errorf("rule %q distinguishes_from unknown rule %q", rule.ID, other)
			}
		}
	}
	return nil
}
