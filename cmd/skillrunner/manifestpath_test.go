package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeManifest drops a minimal skill.json into dir and returns its path.
func writeManifest(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, manifestName)
	if err := os.WriteFile(p, []byte(`{"version":"1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// fakeHome points os.UserHomeDir at a temp dir on both POSIX (HOME) and
// Windows (USERPROFILE), then optionally writes the pointer file into it.
func fakeHome(t *testing.T, poolDir string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if poolDir != "" {
		dir := filepath.Join(home, pointerDirName)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, pointerFileName), []byte(poolDir+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

// TestResolveFromForeignDirSucceeds is the regression test for the bug this
// ladder exists to fix: running from a project that has no skill.json of its own
// used to fail with `read manifest "skill.json"`, which made every `sr` verb in
// a consumer repo — and the whole managed CLAUDE.md block — a dead letter.
func TestResolveFromForeignDirSucceeds(t *testing.T) {
	pool := t.TempDir()
	want := writeManifest(t, pool)
	fakeHome(t, pool)

	foreign := t.TempDir() // a consumer repo: no skill.json anywhere in it
	got, via, err := resolveManifestPath("", foreign)
	if err != nil {
		t.Fatalf("resolve from a foreign dir failed: %v", err)
	}
	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	if via != viaPointer {
		t.Errorf("via = %q, want %q", via, viaPointer)
	}
}

func TestResolveExplicitFlagWins(t *testing.T) {
	// Every lower rung is also satisfied, so this only passes if rung 1 wins.
	explicitDir := t.TempDir()
	want := writeManifest(t, explicitDir)

	cwd := t.TempDir()
	writeManifest(t, cwd)
	envPool := t.TempDir()
	writeManifest(t, envPool)
	t.Setenv(homeEnv, envPool)
	fakeHome(t, envPool)

	got, via, err := resolveManifestPath(want, cwd)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	if via != viaFlag {
		t.Errorf("via = %q, want %q", via, viaFlag)
	}
}

// TestResolveExplicitFlagIsNotSecondGuessed keeps rung 1 honest: an explicit -f
// that does not exist must surface that path in the error rather than silently
// falling through to a different manifest. Silently using another pool would be
// worse than failing — it would emit the wrong rules.
func TestResolveExplicitFlagIsNotSecondGuessed(t *testing.T) {
	cwd := t.TempDir()
	writeManifest(t, cwd)
	missing := filepath.Join(t.TempDir(), "nope.json")

	_, _, err := resolveManifestPath(missing, cwd)
	if err == nil {
		t.Fatal("want an error for an explicit -f that does not exist")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("error should name the missing path %q, got: %v", missing, err)
	}
}

func TestResolveCwdBeatsEnvAndPointer(t *testing.T) {
	// The documented "drop the binary + skill.json into any project" behaviour:
	// a local pool must keep winning over the central one.
	cwd := t.TempDir()
	want := writeManifest(t, cwd)

	envPool := t.TempDir()
	writeManifest(t, envPool)
	t.Setenv(homeEnv, envPool)
	fakeHome(t, envPool)

	got, via, err := resolveManifestPath("", cwd)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	if via != viaCwd {
		t.Errorf("via = %q, want %q", via, viaCwd)
	}
}

func TestResolveEnvBeatsPointer(t *testing.T) {
	envPool := t.TempDir()
	want := writeManifest(t, envPool)
	t.Setenv(homeEnv, envPool)

	pointerPool := t.TempDir()
	writeManifest(t, pointerPool)
	fakeHome(t, pointerPool)

	got, via, err := resolveManifestPath("", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	if via != viaEnv {
		t.Errorf("via = %q, want %q", via, viaEnv)
	}
}

// TestResolveEnvWithoutManifestFallsThrough: a stale SKILLRUNNER_HOME (pointing
// at a moved or deleted pool) must not dead-end the ladder.
func TestResolveEnvWithoutManifestFallsThrough(t *testing.T) {
	t.Setenv(homeEnv, filepath.Join(t.TempDir(), "moved-away"))

	pointerPool := t.TempDir()
	want := writeManifest(t, pointerPool)
	fakeHome(t, pointerPool)

	got, via, err := resolveManifestPath("", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	if via != viaPointer {
		t.Errorf("via = %q, want %q", via, viaPointer)
	}
}

// TestResolveErrorNamesEveryRung: when nothing resolves, the error is the only
// thing the user has to work with, so it must name each way out — not just
// report a missing file the way the old relative default did.
func TestResolveErrorNamesEveryRung(t *testing.T) {
	t.Setenv(homeEnv, "")
	fakeHome(t, "")

	_, _, err := resolveManifestPath("", t.TempDir())
	if err == nil {
		t.Fatal("want an error when no rung resolves")
	}
	msg := err.Error()
	for _, want := range []string{"-f", manifestName, homeEnv, pointerFileName, "sr home --set"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should mention %q; got:\n%s", want, msg)
		}
	}
	// The subcommand is os.Args[1] and flags are parsed from os.Args[2:], so
	// `sr -f <path> list` does not work — it reads -f as the command. A hint
	// printed in the wrong order sends the user down a dead end, so pin it.
	if strings.Contains(msg, "sr -f ") {
		t.Errorf("hint has the flag before the subcommand, which does not parse; got:\n%s", msg)
	}
	if !strings.Contains(msg, "sr <command> -f") {
		t.Errorf("hint should show `sr <command> -f <path>`; got:\n%s", msg)
	}
}

// TestPointerFileToleratesWhitespace: the pointer is written by `make install`
// and edited by hand often enough that a trailing newline or stray spaces must
// not break resolution.
func TestPointerFileToleratesWhitespace(t *testing.T) {
	pool := t.TempDir()
	want := writeManifest(t, pool)

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(home, pointerDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "  " + pool + "  \r\n"
	if err := os.WriteFile(filepath.Join(dir, pointerFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got, _, err := resolveManifestPath("", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}
