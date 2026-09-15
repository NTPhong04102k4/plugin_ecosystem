package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Locating the central manifest.
//
// The manifest path used to default to the relative string "skill.json", which
// silently assumed the process was started inside the pool repo. Run from any
// consumer project — which is the tool's entire purpose — it failed with
// `read manifest "skill.json"`, so every `sr` verb named in a bootstrapped
// CLAUDE.md was a dead letter there.
//
// Resolution is a fixed ladder rather than a search: same environment and
// filesystem always produce the same answer, which keeps the "same input =>
// same output" guarantee the whole tool rests on. Walking parent directories
// looking for a skill.json was rejected for exactly that reason — the result
// would depend on where you happened to stand.
const (
	manifestName    = "skill.json"
	homeEnv         = "SKILLRUNNER_HOME"
	pointerDirName  = ".skillrunner"
	pointerFileName = "home"
)

// Labels for the rung that produced a path, reported by `sr home` so a wrong
// manifest can be diagnosed without guessing.
const (
	viaFlag    = "flag"
	viaCwd     = "cwd"
	viaEnv     = "env"
	viaPointer = "pointer"
)

// resolveManifestPath finds the manifest to load, trying in order:
//
//  1. an explicit -f/--file (never second-guessed: if it is missing, that is an
//     error, not a reason to quietly load a different pool's rules)
//  2. <cwd>/skill.json — a project carrying its own pool, the documented
//     "drop the binary + skill.json + packs/ into any project" layout
//  3. $SKILLRUNNER_HOME/skill.json
//  4. the pointer file written by `make install`
//
// cwd is passed in rather than read from the process so callers and tests can
// resolve against any directory.
func resolveManifestPath(explicit, cwd string) (path, via string, err error) {
	if explicit != "" {
		if !fileExists(explicit) {
			// Plain %s, not %q: on Windows %q escapes every separator
			// (C:\\Users\\...), which makes the path harder to read and to copy.
			return "", "", fmt.Errorf("manifest %s not found (from -f/--file)", explicit)
		}
		return explicit, viaFlag, nil
	}

	if p := filepath.Join(cwd, manifestName); fileExists(p) {
		return p, viaCwd, nil
	}

	// A stale env var or pointer — pointing at a pool that was moved or deleted
	// — falls through to the next rung instead of dead-ending the ladder.
	if home := strings.TrimSpace(os.Getenv(homeEnv)); home != "" {
		if p := filepath.Join(home, manifestName); fileExists(p) {
			return p, viaEnv, nil
		}
	}

	if home, perr := pointerHome(); perr == nil && home != "" {
		if p := filepath.Join(home, manifestName); fileExists(p) {
			return p, viaPointer, nil
		}
	}

	return "", "", noManifestError(cwd)
}

// pointerHome reads the pool directory recorded at ~/.skillrunner/home. The file
// is a single line so it stays editable by hand; surrounding whitespace and the
// trailing newline (CRLF included, since `make install` writes it on Windows
// too) are stripped.
func pointerHome() (string, error) {
	dir, err := pointerPath()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(dir)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// pointerPath is the absolute location of the pointer file. os.UserHomeDir reads
// USERPROFILE on Windows and HOME elsewhere, so this is the one place the two
// platforms differ and it is already handled for us.
func pointerPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, pointerDirName, pointerFileName), nil
}

// noManifestError names every way out. This message is what a user sees when
// the tool is freshly installed or the pool has moved, so it lists the rungs in
// priority order rather than just reporting a missing file.
func noManifestError(cwd string) error {
	loc := "~/" + pointerDirName + "/" + pointerFileName
	if p, err := pointerPath(); err == nil {
		loc = p
	}
	return fmt.Errorf(`no %s found. Tried, in order:
  1. -f/--file            (not given)
  2. %s
  3. $%s/%s   (%s)
  4. %s

Fix with one of:
  sr home --set <path-to-skill-pool>   write the pointer file
  export %s=<path-to-skill-pool>       (setx on Windows)
  sr <command> -f <path-to-skill.json>  one-off

(the subcommand comes first: flags are parsed from os.Args[2:])`,
		manifestName,
		filepath.Join(cwd, manifestName),
		homeEnv, manifestName, envState(),
		loc,
		homeEnv)
}

func envState() string {
	if v := strings.TrimSpace(os.Getenv(homeEnv)); v != "" {
		return "set to " + v
	}
	return "unset"
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
