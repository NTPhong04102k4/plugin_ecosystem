package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	profileRel  = "docs/project-profile.md"
	missingHint = "run `learn-project` to build it"
	refreshWith = "learn-project"
)

// writeFile creates rel (with parents) under dir.
func writeFile(t *testing.T, dir, rel, body string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// initRepo turns dir into a git repo with a self-contained identity.
func initRepo(t *testing.T, dir string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
		{"config", "commit.gpgsign", "false"},
	} {
		gitRun(t, dir, args...)
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func gitCommit(t *testing.T, dir, msg string) {
	t.Helper()
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", msg)
}

func TestCacheLine(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
		want  []string // substrings the line must contain
		deny  []string // substrings it must not contain
	}{
		{
			name:  "missing doc points at the skill that builds it",
			setup: func(t *testing.T, dir string) {},
			want:  []string{"Profile:", "missing (" + profileRel + ")", missingHint},
			deny:  []string{"STALE", "fresh at HEAD"},
		},
		{
			name: "outside a git repo the line is unchanged",
			setup: func(t *testing.T, dir string) {
				writeFile(t, dir, profileRel, "# profile")
			},
			want: []string{"cached (" + profileRel + ")", "reuse it, do not re-scan source"},
			deny: []string{"STALE", "fresh at HEAD"},
		},
		{
			name: "committed alongside the source is fresh",
			setup: func(t *testing.T, dir string) {
				initRepo(t, dir)
				writeFile(t, dir, "main.go", "package main")
				writeFile(t, dir, profileRel, "# profile")
				gitCommit(t, dir, "init")
			},
			want: []string{"cached (" + profileRel + ")", "fresh at HEAD"},
			deny: []string{"STALE"},
		},
		{
			name: "source moved on since the doc was written",
			setup: func(t *testing.T, dir string) {
				initRepo(t, dir)
				writeFile(t, dir, profileRel, "# profile")
				gitCommit(t, dir, "profile")
				writeFile(t, dir, "main.go", "package main")
				gitCommit(t, dir, "add main")
			},
			want: []string{
				"STALE (" + profileRel + ")",
				"1 commit, 1 source file changed since it was written",
				"re-run `" + refreshWith + "`",
			},
			deny: []string{"cached"},
		},
		{
			name: "only uncommitted drift reads as a working-tree warning",
			setup: func(t *testing.T, dir string) {
				initRepo(t, dir)
				writeFile(t, dir, "main.go", "package main")
				writeFile(t, dir, profileRel, "# profile")
				gitCommit(t, dir, "init")
				writeFile(t, dir, "internal/new.go", "package internal")
			},
			want: []string{"STALE", "1 source file uncommitted in the working tree"},
			deny: []string{"changed since it was written"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)

			got := cacheLine(dir, "Profile", profileRel, missingHint, refreshWith)
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("cacheLine() = %q\n  missing %q", got, want)
				}
			}
			for _, deny := range tt.deny {
				if strings.Contains(got, deny) {
					t.Errorf("cacheLine() = %q\n  should not contain %q", got, deny)
				}
			}
			if !strings.HasSuffix(got, "\n") {
				t.Errorf("cacheLine() = %q, want a trailing newline", got)
			}
		})
	}
}

// The MCP `status` tool and the CLI must not drift apart: both render through
// cacheLine, and this pins that they still agree on a stale profile.
func TestStatusTextMatchesCacheLine(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir)
	writeFile(t, dir, profileRel, "# profile")
	gitCommit(t, dir, "profile")
	writeFile(t, dir, "main.go", "package main")
	gitCommit(t, dir, "add main")

	want := cacheLine(dir, "Profile", profileRel, missingHint, refreshWith)
	got := (&mcpServer{}).statusText(dir)

	if !strings.Contains(got, want) {
		t.Errorf("statusText() = %q\n  does not contain the CLI line %q", got, want)
	}
}

func TestPlural(t *testing.T) {
	tests := []struct {
		n    int
		noun string
		want string
	}{
		{0, "commit", "0 commits"},
		{1, "commit", "1 commit"},
		{2, "commit", "2 commits"},
		{1, "source file", "1 source file"},
		{7, "source file", "7 source files"},
	}

	for _, tt := range tests {
		if got := plural(tt.n, tt.noun); got != tt.want {
			t.Errorf("plural(%d, %q) = %q, want %q", tt.n, tt.noun, got, tt.want)
		}
	}
}
