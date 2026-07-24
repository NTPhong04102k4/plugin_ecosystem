package uistore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	repo := t.TempDir()
	cfg := &UIConfig{
		Tags: map[string]*Tag{
			"X": {Site: "acme.atlassian.net", Email: "me@x.vn", Token: "tok-x"},
		},
		Sessions: map[string]*Session{
			"feat-x": {Tag: "X", Confluence: []string{"https://c/1"}, Sheets: []string{"https://s/1"}},
		},
	}
	if err := Save(repo, cfg); err != nil {
		t.Fatal(err)
	}

	// File perms must be 0600.
	info, err := os.Stat(Path(repo))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("ui.json perm = %o, want 600", perm)
	}

	got, err := Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if got.Tags["X"].Token != "tok-x" || got.Sessions["feat-x"].Tag != "X" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestSaveEnsuresGitignore(t *testing.T) {
	repo := t.TempDir()
	if err := Save(repo, &UIConfig{}); err != nil {
		t.Fatal(err)
	}
	gi, err := os.ReadFile(filepath.Join(repo, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gi), ".skillrunner/") {
		t.Errorf(".gitignore missing .skillrunner/: %q", gi)
	}

	// Idempotent: saving again must not duplicate the entry.
	if err := Save(repo, &UIConfig{}); err != nil {
		t.Fatal(err)
	}
	gi2, _ := os.ReadFile(filepath.Join(repo, ".gitignore"))
	if strings.Count(string(gi2), ".skillrunner/") != 1 {
		t.Errorf("gitignore entry duplicated: %q", gi2)
	}
}

func TestGitignoreRespectsExisting(t *testing.T) {
	repo := t.TempDir()
	os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("node_modules/\n.skillrunner\n"), 0o644)
	if err := Save(repo, &UIConfig{}); err != nil {
		t.Fatal(err)
	}
	gi, _ := os.ReadFile(filepath.Join(repo, ".gitignore"))
	// ".skillrunner" (no slash) already covers it → must not append again.
	if strings.Count(string(gi), ".skillrunner") != 1 {
		t.Errorf("should not re-add when already ignored: %q", gi)
	}
}

func TestPutTagKeepsExistingTokenWhenEmpty(t *testing.T) {
	repo := t.TempDir()
	if err := PutTag(repo, "X", Tag{Site: "s", Email: "e", Token: "secret"}); err != nil {
		t.Fatal(err)
	}
	// Update email without resupplying the token.
	if err := PutTag(repo, "X", Tag{Site: "s", Email: "e2", Token: ""}); err != nil {
		t.Fatal(err)
	}
	cfg, _ := Load(repo)
	if cfg.Tags["X"].Token != "secret" {
		t.Errorf("token lost on tokenless update: %+v", cfg.Tags["X"])
	}
	if cfg.Tags["X"].Email != "e2" {
		t.Errorf("email not updated: %+v", cfg.Tags["X"])
	}
}

func TestAppendHistoryNewestFirstCapped(t *testing.T) {
	repo := t.TempDir()
	for i := 0; i < 3; i++ {
		if err := AppendHistory(repo, HistoryEntry{Session: "s", At: string(rune('a' + i)), OK: true}); err != nil {
			t.Fatal(err)
		}
	}
	cfg, _ := Load(repo)
	if len(cfg.History) != 3 || cfg.History[0].At != "c" {
		t.Errorf("history order wrong: %+v", cfg.History)
	}
}
