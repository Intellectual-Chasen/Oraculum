package attackrules

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Decode は filename と一致する rule ID を持つ YAML 1 文書を strict に読む。
func Decode(name string, data []byte) (Rule, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var rule Rule
	if err := decoder.Decode(&rule); err != nil {
		if errors.Is(err, io.EOF) {
			return Rule{}, fmt.Errorf("ATT&CK rule %q is empty", name)
		}
		return Rule{}, fmt.Errorf("decode ATT&CK rule %q: %w", name, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return Rule{}, fmt.Errorf("decode trailing ATT&CK YAML in %q: %w", name, err)
		}
		return Rule{}, fmt.Errorf("ATT&CK rule %q contains multiple YAML documents", name)
	}
	if err := rule.Validate(); err != nil {
		return Rule{}, fmt.Errorf("validate ATT&CK rule %q: %w", name, err)
	}
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	if stem != rule.ID {
		return Rule{}, fmt.Errorf("ATT&CK rule %q declares id %q; file name must equal the rule id", name, rule.ID)
	}
	return rule, nil
}
