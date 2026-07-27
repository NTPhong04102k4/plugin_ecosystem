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

			got := CheckFreshness(dir, profile, "")
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CheckFreshness() = %+v, want %+v", got, tt.want)
			}
			if got.Stale() != tt.stale {
				t.Errorf("Stale() = %v, want %v", got.Stale(), tt.stale)
			}
		})
	}
}

// head returns the current commit of the repo at dir.
func head(t *testing.T, dir string) string {
	t.Helper()
	out, ok := gitOut(dir, "rev-parse", "HEAD")
	if !ok {
		t.Fatal("cannot read HEAD")
	}
	return out
}

// The whole point of the build-commit baseline: touching the doc must not be
// able to launder away drift that really happened.
func TestCheckFreshnessBuildCommitBeatsATypoFix(t *testing.T) {
	dir := gitRepo(t)
	write(t, dir, profile, "# profile")
	commit(t, dir, "profile built here")
	built := head(t, dir)

	write(t, dir, "main.go", "package main")
	commit(t, dir, "real source change")

	// Someone fixes a typo in the profile and commits it. The doc's own last
	// commit is now newer than the source change.
	write(t, dir, profile, "# profile (typo fixed)")
	commit(t, dir, "typo")

	if got := CheckFreshness(dir, profile, ""); got.Stale() {
		t.Fatalf("precondition: without a build commit this should read fresh, got %+v", got)
	}

	got := CheckFreshness(dir, profile, built)
	want := Freshness{Known: true, Commits: 2, Files: 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CheckFreshness(build) = %+v, want %+v", got, want)
	}
	if !got.Stale() {
		t.Error("Stale() = false, want true — the source change must still count")
	}
}

func TestCheckFreshnessBuildCommitNeverLoosensTheSignal(t *testing.T) {
	tests := []struct {
		name  string
		build func(t *testing.T, dir string) string
		want  Freshness
	}{
		{
			// The doc was committed long after it was built, so its own history
			// looks fresher than reality. The older baseline must win.
			name:  "doc committed later than it was built",
			build: func(t *testing.T, dir string) string { return head(t, dir) },
			want:  Freshness{Known: true, Commits: 2, Files: 1},
		},
		{
			// A ledger carried in from another repo names an unknown commit.
			name:  "unresolvable build commit is ignored",
			build: func(t *testing.T, dir string) string { return "0000000000000000000000000000000000000000" },
			want:  Freshness{Known: true},
		},
		{
			name:  "empty build commit falls back to the doc's history",
			build: func(t *testing.T, dir string) string { return "" },
			want:  Freshness{Known: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := gitRepo(t)
			write(t, dir, "seed.go", "package seed")
			commit(t, dir, "seed")
			build := tt.build(t, dir)

			write(t, dir, "main.go", "package main")
			commit(t, dir, "source change")
			write(t, dir, profile, "# profile")
			commit(t, dir, "commit the profile afterwards")

			if got := CheckFreshness(dir, profile, build); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CheckFreshness() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestWorseThan(t *testing.T) {
	tests := []struct {
		name string
		a, b Freshness
		want bool
	}{
		{"more source files wins", Freshness{Files: 3, Commits: 1}, Freshness{Files: 1, Commits: 9}, true},
		{"fewer source files loses", Freshness{Files: 1, Commits: 9}, Freshness{Files: 3, Commits: 1}, false},
		{"commits break a file tie", Freshness{Files: 2, Commits: 5}, Freshness{Files: 2, Commits: 4}, true},
		{"identical is not worse", Freshness{Files: 2, Commits: 4}, Freshness{Files: 2, Commits: 4}, false},
	}

	for _, tt := range tests {
		if got := tt.a.worseThan(tt.b); got != tt.want {
			t.Errorf("%s: worseThan() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// A directory outside any repo must degrade to "unknown", not to an error or a
// hang: staleness is advisory and `status` has to keep working without git.
func TestCheckFreshnessOutsideGitRepo(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, profile, "# profile")

	got := CheckFreshness(dir, profile, "")
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
