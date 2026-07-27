package skill

import (
	"testing"
	"time"
)

var emitTime = time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)

// RecordEmit stamps HEAD so a later status check knows when a doc-building
// skill actually ran — the baseline the doc's own commit history cannot give.
func TestRecordEmitStampsHead(t *testing.T) {
	dir := gitRepo(t)
	write(t, dir, "main.go", "package main")
	commit(t, dir, "init")
	want := head(t, dir)

	if err := RecordEmit(dir, "proj", "learn-project", "go", emitTime); err != nil {
		t.Fatal(err)
	}

	l, err := LoadLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := l.BuildCommit("learn-project"); got != want {
		t.Errorf("BuildCommit() = %q, want HEAD %q", got, want)
	}
	if got := l.Skills["learn-project"].Count; got != 1 {
		t.Errorf("Count = %d, want 1", got)
	}
}

// A second emit re-stamps: the newest build is the one that matters.
func TestRecordEmitRestampsOnReEmit(t *testing.T) {
	dir := gitRepo(t)
	write(t, dir, "main.go", "package main")
	commit(t, dir, "init")
	if err := RecordEmit(dir, "proj", "learn-project", "go", emitTime); err != nil {
		t.Fatal(err)
	}
	first := head(t, dir)

	write(t, dir, "other.go", "package main")
	commit(t, dir, "move on")
	if err := RecordEmit(dir, "proj", "learn-project", "go", emitTime.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	l, err := LoadLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := l.BuildCommit("learn-project")
	if got == first {
		t.Error("BuildCommit() still points at the first emit; want the latest HEAD")
	}
	if want := head(t, dir); got != want {
		t.Errorf("BuildCommit() = %q, want %q", got, want)
	}
}

// Outside a repo there is no commit to record, and BuildCommit must degrade to
// "" rather than inventing a baseline.
func TestBuildCommitWithoutGit(t *testing.T) {
	dir := t.TempDir()
	if err := RecordEmit(dir, "proj", "learn-project", "go", emitTime); err != nil {
		t.Fatal(err)
	}

	l, err := LoadLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := l.BuildCommit("learn-project"); got != "" {
		t.Errorf("BuildCommit() = %q, want empty outside a repo", got)
	}
	if got := l.BuildCommit("never-emitted"); got != "" {
		t.Errorf("BuildCommit(unknown skill) = %q, want empty", got)
	}
}
