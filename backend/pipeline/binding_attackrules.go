package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Intellectual-Chasen/Oraculum/backend/attackrules"
)

// LoadAttackRuleSet は external directory の YAML rule を全件検証して読む。
func LoadAttackRuleSet(directory string) (attackrules.Set, error) {
	resolved, err := filepath.Abs(directory)
	if err != nil {
		return attackrules.Set{}, fmt.Errorf("resolve ATT&CK rule directory %q: %w", directory, err)
	}
	if real, err := filepath.EvalSymlinks(resolved); err == nil {
		resolved = real
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return attackrules.Set{}, fmt.Errorf("read ATT&CK rule directory %q: %w", resolved, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".yaml" || ext == ".yml" {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	if len(names) == 0 {
		return attackrules.Set{}, fmt.Errorf("ATT&CK rule directory %q contains no YAML rules", resolved)
	}

	set := attackrules.Set{Rules: make([]attackrules.Rule, 0, len(names))}
	digest := sha256.New()
	for _, name := range names {
		path := filepath.Join(resolved, name)
		info, err := os.Lstat(path)
		if err != nil {
			return attackrules.Set{}, fmt.Errorf("inspect ATT&CK rule %q: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return attackrules.Set{}, fmt.Errorf("ATT&CK rule %q is not a regular file", path)
		}
		data, err := os.ReadFile(path) //nolint:gosec // path is a YAML file enumerated directly under the configured rule directory.
		if err != nil {
			return attackrules.Set{}, fmt.Errorf("read ATT&CK rule %q: %w", path, err)
		}
		rule, err := attackrules.Decode(name, data)
		if err != nil {
			return attackrules.Set{}, err
		}
		rule.Path = path
		set.Rules = append(set.Rules, rule)
		_, _ = fmt.Fprintf(digest, "%d:", len(name))
		_, _ = digest.Write([]byte(name))
		_, _ = fmt.Fprintf(digest, "%d:", len(data))
		_, _ = digest.Write(data)
	}
	if err := set.Validate(); err != nil {
		return attackrules.Set{}, err
	}
	revision, revisionSource := revisionOf(resolved)
	set.Info = attackrules.SetInfo{
		Directory: resolved, FileCount: len(set.Rules), ContentSha256: hex.EncodeToString(digest.Sum(nil)),
		Revision: revision, RevisionSource: revisionSource,
	}
	return set, nil
}

func revisionOf(directory string) (string, string) {
	root := directory
	for {
		gitPath := filepath.Join(root, ".git")
		data, err := os.ReadFile(gitPath) //nolint:gosec // Read the .git marker encountered while walking the configured rule directory's ancestors.
		if err == nil {
			gitDir := gitPath
			if strings.HasPrefix(string(data), "gitdir: ") {
				gitDir = strings.TrimSpace(strings.TrimPrefix(string(data), "gitdir: "))
				if !filepath.IsAbs(gitDir) {
					gitDir = filepath.Join(root, gitDir)
				}
			}
			if value, source, ok := readGitRevision(gitDir); ok {
				return value, source
			}
			return "unverified", "unavailable"
		}
		if info, statErr := os.Stat(gitPath); statErr == nil && info.IsDir() {
			if value, source, ok := readGitRevision(gitPath); ok {
				return value, source
			}
			return "unverified", "unavailable"
		}
		parent := filepath.Dir(root)
		if parent == root {
			return "unverified", "unavailable"
		}
		root = parent
	}
}

func readGitRevision(gitDir string) (string, string, bool) {
	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD")) //nolint:gosec // Read the repository metadata file selected from its .git marker.
	if err != nil {
		return "", "", false
	}
	value := strings.TrimSpace(string(head))
	if !strings.HasPrefix(value, "ref: ") {
		return value, "git_head", value != ""
	}
	ref := strings.TrimPrefix(value, "ref: ")
	if !strings.HasPrefix(ref, "refs/") {
		return "", "", false
	}
	refPath := filepath.Join(gitDir, filepath.FromSlash(ref))
	relativeRef, err := filepath.Rel(gitDir, refPath)
	if err != nil || relativeRef == ".." || strings.HasPrefix(relativeRef, ".."+string(filepath.Separator)) {
		return "", "", false
	}
	if contents, err := os.ReadFile(refPath); err == nil { //nolint:gosec // The refs/ prefix and path containment above keep this inside the selected Git metadata directory.
		return strings.TrimSpace(string(contents)), "git_head", true
	}
	packed, err := os.ReadFile(filepath.Join(gitDir, "packed-refs")) //nolint:gosec // Read the fixed packed-refs metadata file from the selected Git directory.
	if err != nil {
		return "", "", false
	}
	for _, line := range strings.Split(string(packed), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == ref {
			return fields[0], "git_packed_ref", true
		}
	}
	return "", "", false
}
