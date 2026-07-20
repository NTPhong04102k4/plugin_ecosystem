package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFetchConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".skillrunner"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{
  // confluence auth
  "confluence": { "site": "acme.atlassian.net", "email": "me@acme.vn", "tokenEnv": "CONFLUENCE_TOKEN" },
  "google": { "saKeyFile": "~/.secrets/sa.json" },
  "sources": { "tc-login": "https://docs.google.com/spreadsheets/d/ABC/edit#gid=0" }
}`
	if err := os.WriteFile(filepath.Join(dir, ".skillrunner", "fetch.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFetchConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Confluence == nil || cfg.Confluence.Email != "me@acme.vn" {
		t.Errorf("confluence = %+v", cfg.Confluence)
	}
	if cfg.Google == nil || cfg.Google.SAKeyFile != "~/.secrets/sa.json" {
		t.Errorf("google = %+v", cfg.Google)
	}
	if cfg.Sources["tc-login"] == "" {
		t.Errorf("sources = %v", cfg.Sources)
	}
}

func TestLoadFetchConfigMissing(t *testing.T) {
	cfg, err := LoadFetchConfig(t.TempDir())
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if cfg == nil || cfg.Confluence != nil {
		t.Errorf("expected empty config, got %+v", cfg)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", "c", "d"); got != "c" {
		t.Errorf("got %q", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	if got := expandHome("~/x/y"); got != filepath.Join(home, "x/y") {
		t.Errorf("got %q", got)
	}
	if got := expandHome("/abs/path"); got != "/abs/path" {
		t.Errorf("absolute path changed: %q", got)
	}
}
