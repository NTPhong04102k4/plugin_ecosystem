package skill

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// gitRepo builds a throwaway repo with an identity of its own, so the test does
// not depend on (or touch) the developer's git config.
func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
		{"config", "commit.gpgsign", "false"},
	} {
		run(t, dir, args...)
	}
	return dir
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// write creates rel (with any parent dirs) inside dir.
func write(t *testing.T, dir, rel, body string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// commit stages everything and records one commit.
func commit(t *testing.T, dir, msg string) {
	t.Helper()
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-q", "-m", msg)
}

const profile = "docs/project-profile.md"

func TestCheckFreshness(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
		want  Freshness
		stale bool
	}{
		{
			name: "fresh at HEAD",
			setup: func(t *testing.T, dir string) {
				write(t, dir, "main.go", "package main")
				write(t, dir, profile, "# profile")
				commit(t, dir, "init")
			},
			want:  Freshness{Known: true},
			stale: false,
		},
		{
			name: "source changed after the profile was written",
			setup: func(t *testing.T, dir string) {
				write(t, dir, profile, "# profile")
				commit(t, dir, "profile")
				write(t, dir, "main.go", "package main")
				commit(t, dir, "add main")
				write(t, dir, "internal/a/a.go", "package a")
				write(t, dir, "internal/b/b.go", "package b")
				commit(t, dir, "add packages")
			},
			want:  Freshness{Known: true, Commits: 2, Files: 3},
			stale: true,
		},
		{
			name: "docs churn and ledger writes do not age the profile",
			setup: func(t *testing.T, dir string) {
				write(t, dir, profile, "# profile")
				commit(t, dir, "profile")
				write(t, dir, "docs/sr-fetch-design.md", "notes")
				write(t, dir, "README.md", "readme")
				write(t, dir, ".skillrunner/ledger.json", "{}")
				commit(t, dir, "docs + ledger")
			},
			want:  Freshness{Known: true, Commits: 1},
			stale: false,
		},
		{
			name: "uncommitted and untracked source counts as drift",
			setup: func(t *testing.T, dir string) {
				write(t, dir, "main.go", "package main")
				write(t, dir, profile, "# profile")
				commit(t, dir, "init")
				write(t, dir, "main.go", "package main // edited")
				write(t, dir, "internal/new.go", "package internal")
				write(t, dir, "docs/notes.md", "ignored by the filter")
			},
			want:  Freshness{Known: true, Dirty: 2},
			stale: true,
		},
		{
			name: "profile written but never committed",
			setup: func(t *testing.T, dir string) {
				write(t, dir, "main.go", "package main")
				commit(t, dir, "init")
				write(t, dir, profile, "# profile")
			},
			want:  Freshness{},
			stale: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := gitRepo(t)
			tt.setup(t, dir)

			got := CheckFreshness(dir, profile)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CheckFreshness() = %+v, want %+v", got, tt.want)
			}
			if got.Stale() != tt.stale {
				t.Errorf("Stale() = %v, want %v", got.Stale(), tt.stale)
			}
		})
	}
}

// A directory outside any repo must degrade to "unknown", not to an error or a
// hang: staleness is advisory and `status` has to keep working without git.
func TestCheckFreshnessOutsideGitRepo(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, profile, "# profile")

	got := CheckFreshness(dir, profile)
	if got.Known {
		t.Errorf("CheckFreshness() outside a repo = %+v, want Known false", got)
	}
	if got.Stale() {
		t.Error("Stale() = true outside a repo, want false")
	}
}

func TestIsSourcePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"main.go", true},
		{"internal/skill/emit.go", true},
		{"packs/go.json", true},
		{"skill.json", true},
		{"README.md", false},
		{"docs/module-registry.md", false},
		{"docs/diagram.png", false},
		{"internal/docs/thing.go", false}, // any "docs" segment, at any depth
		{".skillrunner/ledger.json", false},
		{"", false},
		{"   ", false},
	}

	for _, tt := range tests {
		if got := isSourcePath(tt.path); got != tt.want {
			t.Errorf("isSourcePath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestPorcelainPaths(t *testing.T) {
	out := " M main.go\n?? internal/new.go\nR  old.go -> new.go\nA  \"has space.go\""
	want := []string{"main.go", "internal/new.go", "new.go", "has space.go"}

	if got := porcelainPaths(out); !reflect.DeepEqual(got, want) {
		t.Errorf("porcelainPaths() = %q, want %q", got, want)
	}
}
