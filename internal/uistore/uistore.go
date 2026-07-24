// Package uistore reads/writes the per-repo config for `sr ui`:
// <repo>/.skillrunner/ui.json holding credential tags, per-feature sessions, and
// run history. Writes are atomic + 0600, and adding config auto-ensures the
// repo's .gitignore excludes .skillrunner/ so plaintext tokens never get
// committed. See docs/sr-ui-design.md.
package uistore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UIConfig is the whole ui.json document.
type UIConfig struct {
	Tags     map[string]*Tag     `json:"tags,omitempty"`
	Sessions map[string]*Session `json:"sessions,omitempty"`
	History  []HistoryEntry      `json:"history,omitempty"`
}

// Tag is one reusable Confluence credential region {site, gmail, token}.
type Tag struct {
	Site  string `json:"site"`
	Email string `json:"email"`
	Token string `json:"token,omitempty"`
}

// Session is one feature's link batch, bound to a tag.
type Session struct {
	Tag        string   `json:"tag"`
	Confluence []string `json:"confluence,omitempty"`
	Sheets     []string `json:"sheets,omitempty"`
}

// HistoryEntry logs one run/refresh.
type HistoryEntry struct {
	Session string   `json:"session"`
	At      string   `json:"at"`
	Files   []string `json:"files,omitempty"`
	OK      bool     `json:"ok"`
}

// Dir returns <repo>/.skillrunner.
func Dir(repo string) string { return filepath.Join(repo, ".skillrunner") }

// Path returns <repo>/.skillrunner/ui.json.
func Path(repo string) string { return filepath.Join(Dir(repo), "ui.json") }

// Load reads ui.json for a repo. A missing file yields an empty (non-nil) config.
func Load(repo string) (*UIConfig, error) {
	data, err := os.ReadFile(Path(repo))
	if err != nil {
		if os.IsNotExist(err) {
			return &UIConfig{Tags: map[string]*Tag{}, Sessions: map[string]*Session{}}, nil
		}
		return nil, err
	}
	var cfg UIConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse ui.json: %w", err)
	}
	if cfg.Tags == nil {
		cfg.Tags = map[string]*Tag{}
	}
	if cfg.Sessions == nil {
		cfg.Sessions = map[string]*Session{}
	}
	return &cfg, nil
}

// Save writes ui.json atomically at 0600 and ensures the repo gitignores
// .skillrunner/ (so plaintext tokens are never committed).
func Save(repo string, cfg *UIConfig) error {
	dir := Dir(repo)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create .skillrunner dir: %w", err)
	}
	if err := ensureGitignore(repo); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	// Atomic: write a temp file in the same dir, then rename over the target.
	tmp, err := os.CreateTemp(dir, "ui-*.json.tmp")
	if err != nil {
		return fmt.Errorf("temp ui.json: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, Path(repo)); err != nil {
		return fmt.Errorf("write ui.json: %w", err)
	}
	return nil
}

// ensureGitignore makes sure <repo>/.gitignore ignores .skillrunner/. Idempotent.
func ensureGitignore(repo string) error {
	giPath := filepath.Join(repo, ".gitignore")
	data, err := os.ReadFile(giPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		switch strings.TrimSpace(line) {
		case ".skillrunner/", ".skillrunner", "/.skillrunner/", "/.skillrunner":
			return nil // already ignored
		}
	}
	block := "\n# sr ui — local credentials/config (never commit)\n.skillrunner/\n"
	if len(data) == 0 {
		block = strings.TrimPrefix(block, "\n")
	}
	f, err := os.OpenFile(giPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("update .gitignore: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(block); err != nil {
		return err
	}
	return nil
}

// --- mutation helpers used by the server ---

// PutTag adds/updates a tag and saves. An empty token keeps any existing token.
func PutTag(repo, name string, t Tag) error {
	cfg, err := Load(repo)
	if err != nil {
		return err
	}
	if t.Token == "" {
		if old, ok := cfg.Tags[name]; ok {
			t.Token = old.Token
		}
	}
	cfg.Tags[name] = &t
	return Save(repo, cfg)
}

// DeleteTag removes a tag and saves.
func DeleteTag(repo, name string) error {
	cfg, err := Load(repo)
	if err != nil {
		return err
	}
	delete(cfg.Tags, name)
	return Save(repo, cfg)
}

// PutSession adds/updates a session and saves.
func PutSession(repo, name string, s Session) error {
	cfg, err := Load(repo)
	if err != nil {
		return err
	}
	cfg.Sessions[name] = &s
	return Save(repo, cfg)
}

// DeleteSession removes a session and saves.
func DeleteSession(repo, name string) error {
	cfg, err := Load(repo)
	if err != nil {
		return err
	}
	delete(cfg.Sessions, name)
	return Save(repo, cfg)
}

// AppendHistory prepends an entry (newest first), capping the log.
func AppendHistory(repo string, e HistoryEntry) error {
	cfg, err := Load(repo)
	if err != nil {
		return err
	}
	cfg.History = append([]HistoryEntry{e}, cfg.History...)
	if len(cfg.History) > 100 {
		cfg.History = cfg.History[:100]
	}
	return Save(repo, cfg)
}
