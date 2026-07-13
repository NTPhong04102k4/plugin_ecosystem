package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ghmsoft/skillrunner/internal/skill"
)

// The bootstrap block teaches ANY Claude session how to discover and use `sr`.
// Without it, Claude in another project never learns the binary exists — it only
// sees skill names like "learn-project" with no idea they are sr skills. The
// block is delimited by markers so `bootstrap` can find it and refresh it in
// place, never duplicating.
const (
	bootstrapBegin = "<!-- skillrunner:begin (managed by `sr bootstrap` — do not edit inside) -->"
	bootstrapEnd   = "<!-- skillrunner:end -->"
	bt             = "`"
)

// bootstrapBlock builds the managed CLAUDE.md section. It is stack-aware (names
// the detected stack for context) but drives everything through the plain `sr`
// verbs — the shell wrapper already pins the central skill.json, so no manifest
// path is hardcoded. Same stack => same output. No trailing newline.
func bootstrapBlock(stack string) string {
	if stack == "" {
		stack = "unknown"
	}
	lines := []string{
		bootstrapBegin,
		"## skillrunner (" + bt + "sr" + bt + ") — use it every session",
		"",
		"This project (stack: **" + stack + "**) is served by " + bt + "sr" + bt + " (aka " + bt + "skillrunner" + bt + "), a central",
		"skill dispatcher on your PATH. It detects the stack and prints \"marching orders\"",
		"(rules + steps) for YOU (Claude) to execute — it never reasons or edits files itself.",
		"",
		"When a request matches a skill:",
		"1. " + bt + "sr status" + bt + " — stack + whether docs/project-profile.md and docs/module-registry.md are cached.",
		"2. " + bt + "sr list" + bt + " — skills with one-line descriptions; map the task to the right one.",
		"3. " + bt + "sr emit <skill>" + bt + " — print the marching orders, then READ and FOLLOW the \"Rules you MUST follow\" section.",
		"4. A skill tagged " + bt + "[needs approval]" + bt + " → only propose a plan/goal and STOP for the user; do not edit files first.",
		"5. First task in a project with no docs/project-profile.md → run " + bt + "learn-project" + bt + " before implementing.",
		"",
		"If a task clearly matches a skill, prefer " + bt + "sr emit <skill>" + bt + " over improvising.",
		bootstrapEnd,
	}
	return strings.Join(lines, "\n")
}

// ensureBootstrap runs the per-project cascade:
//   - project CLAUDE.md already carries the block → refresh it in place (stack or
//     wording may have changed) and stop;
//   - else the global ~/.claude/CLAUDE.md carries it (Claude is covered in every
//     project) → do nothing, unless force;
//   - else write the block into the project CLAUDE.md.
func ensureBootstrap(detectDir string, force bool) error {
	projectPath := filepath.Join(detectDir, "CLAUDE.md")
	stack := skill.Detect(detectDir).Stack
	block := bootstrapBlock(stack)

	projectHas, err := fileHasMarker(projectPath, bootstrapBegin)
	if err != nil {
		return err
	}
	if projectHas {
		return writeBlock(projectPath, block, stack)
	}

	if !force {
		if globalPath, err := globalClaudePath(); err == nil {
			has, err := fileHasMarker(globalPath, bootstrapBegin)
			if err != nil {
				return err
			}
			if has {
				fmt.Printf("Covered globally by %s — nothing to do.\n", globalPath)
				fmt.Printf("(run with --force to add a copy to %s anyway)\n", projectPath)
				return nil
			}
		}
	}

	return writeBlock(projectPath, block, stack)
}

// writeBlock creates CLAUDE.md, refreshes an existing managed block in place, or
// appends the block — whichever applies — then prints what it did. Idempotent:
// an unchanged block writes nothing.
func writeBlock(path, block, stack string) error {
	report := func(status string) {
		fmt.Printf("CLAUDE.md %s (%s) — skillrunner block, stack: %s\n", status, path, displayStack(stack))
	}

	existing, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if werr := os.WriteFile(path, []byte(block+"\n"), 0o644); werr != nil {
			return werr
		}
		report("created")
		return nil
	}

	content := string(existing)
	si := strings.Index(content, bootstrapBegin)
	ei := strings.Index(content, bootstrapEnd)
	if si != -1 && ei != -1 && ei > si {
		ei += len(bootstrapEnd)
		updated := content[:si] + block + content[ei:]
		if updated == content {
			report("unchanged")
			return nil
		}
		if werr := os.WriteFile(path, []byte(updated), 0o644); werr != nil {
			return werr
		}
		report("updated")
		return nil
	}

	// Append, keeping a blank line between prior content and the block.
	sep := "\n\n"
	switch {
	case content == "":
		sep = ""
	case strings.HasSuffix(content, "\n\n"):
		sep = ""
	case strings.HasSuffix(content, "\n"):
		sep = "\n"
	}
	if werr := os.WriteFile(path, []byte(content+sep+block+"\n"), 0o644); werr != nil {
		return werr
	}
	report("appended")
	return nil
}

// fileHasMarker reports whether the file at path contains marker. A missing file
// is not an error — it simply has no marker.
func fileHasMarker(path, marker string) (bool, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.Contains(string(b), marker), nil
}

// globalClaudePath returns ~/.claude/CLAUDE.md, the file Claude loads in every
// project regardless of cwd.
func globalClaudePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "CLAUDE.md"), nil
}

func displayStack(stack string) string {
	if stack == "" {
		return "unknown"
	}
	return stack
}
