// skillrunner is a portable skill dispatcher for Claude. It holds SHARED skills
// (the "verbs": scaffold-data, build-ui, refactor, ...) plus base rules, and
// merges in a STACK PACK (packs/<stack>.json) chosen by inspecting the project.
// It composes them into full instructions ("marching orders") that Claude
// executes. The binary never reasons — it detects files and formats text. Drop
// the binary + skill.json + packs/ into any project and the same verbs work
// across React, Flutter, Go, etc. without restructuring per project.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ghmsoft/skillrunner/internal/skill"
)

const usage = `skillrunner — portable skill dispatcher for Claude

Usage:
  skillrunner detect                   Print the detected stack (pack) for this project
  skillrunner status                   Show stack + whether the project profile/registry are cached
  skillrunner list                     List skills (auto-detects & merges the stack pack)
  skillrunner emit <skill>             Print marching orders for <skill> (records it in the project ledger)
  skillrunner ledger                   Show which skills have already been emitted in this project
  skillrunner validate                 Check that the manifest is well-formed
  skillrunner init                     Write a starter skill.json in the current dir
  skillrunner bootstrap                Ensure the project's CLAUDE.md tells Claude to use sr

Flags:
  -f, --file <path>   Manifest path (default: skill.json)
  -p, --pack <stack>  Force a stack pack (e.g. react, flutter). Default: auto-detect.
      --dir <path>    Project dir to detect against (default: manifest's dir)
      --force         (bootstrap) Write the project CLAUDE.md even if already covered

Flow: detect stack -> emit merges shared skill + stack rule pack -> Claude executes.
Skills marked [needs approval] stop for your decision before changing files.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	var file, pack, dir string
	var force bool
	fs.StringVar(&file, "file", "skill.json", "manifest path")
	fs.StringVar(&file, "f", "skill.json", "manifest path (shorthand)")
	fs.StringVar(&pack, "pack", "", "force stack pack")
	fs.StringVar(&pack, "p", "", "force stack pack (shorthand)")
	fs.StringVar(&dir, "dir", "", "project dir to detect against")
	fs.BoolVar(&force, "force", false, "bootstrap: write project CLAUDE.md even if already covered")
	// Go's flag package stops at the first positional, so flags placed AFTER the
	// skill name would be ignored. Interleave parsing to accept flags anywhere.
	rest := parseInterleaved(fs, os.Args[2:])

	// detectDir is the target project to inspect (defaults to the current dir).
	// packDir is where packs/ lives — always next to the manifest, since packs
	// ship centrally with skill.json. In the per-project layout the two coincide.
	detectDir := dir
	if detectDir == "" {
		detectDir = "."
	}
	packDir := filepath.Dir(file)

	switch cmd {
	case "init":
		if err := writeStarter(file); err != nil {
			fatal(err)
		}
		fmt.Printf("Wrote starter manifest to %s\n", file)

	case "bootstrap":
		if err := ensureBootstrap(detectDir, force); err != nil {
			fatal(err)
		}

	case "validate":
		m, err := skill.Load(file)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("OK: %s is valid (%d skills)\n", file, len(m.Skills))

	case "detect":
		d := skill.Detect(detectDir)
		if d.Stack == "" {
			fmt.Printf("No known stack detected (%s)\n", d.Reason)
			if packs := skill.AvailablePacks(packDir); len(packs) > 0 {
				fmt.Printf("Available packs: %v\n", packs)
			}
			os.Exit(0)
		}
		fmt.Printf("Detected stack: %s (%s)\n", d.Stack, d.Reason)

	case "status":
		d := skill.Detect(detectDir)
		if d.Stack == "" {
			fmt.Printf("Stack:   unknown (%s)\n", d.Reason)
		} else {
			fmt.Printf("Stack:   %s (%s)\n", d.Stack, d.Reason)
		}
		reportCache(detectDir, "Profile", "docs/project-profile.md", "run `learn-project` to build it")
		reportCache(detectDir, "Registry", "docs/module-registry.md", "will be created as features land")
		if l, err := skill.LoadLedger(detectDir); err == nil {
			fmt.Println(l.StatusLine())
		}

	case "ledger":
		l, err := skill.LoadLedger(detectDir)
		if err != nil {
			fatal(err)
		}
		fmt.Print(l.Summary())

	case "list":
		m, err := loadWithPack(file, detectDir, packDir, pack)
		if err != nil {
			fatal(err)
		}
		fmt.Print(m.List())

	case "emit":
		if len(rest) < 1 {
			fatal(fmt.Errorf("emit requires a skill name; run `skillrunner list`"))
		}
		m, err := loadWithPack(file, detectDir, packDir, pack)
		if err != nil {
			fatal(err)
		}
		out, err := m.Emit(rest[0])
		if err != nil {
			fatal(err)
		}
		fmt.Print(out)
		// Record the emit in the project ledger so a later session sees this
		// skill was already run here. A ledger failure must not swallow the
		// marching orders we just printed — warn and carry on.
		stack := pack
		if stack == "" {
			stack = skill.Detect(detectDir).Stack
		}
		if err := skill.RecordEmit(detectDir, projectLabel(detectDir), rest[0], stack, time.Now()); err != nil {
			fmt.Fprintf(os.Stderr, "note: could not record emit in ledger (%v)\n", err)
		}

	case "-h", "--help", "help":
		fmt.Print(usage)

	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
}

// loadWithPack loads the manifest and merges the stack pack: the one named by
// forcePack, or the auto-detected one. If no pack is found the base manifest is
// returned unchanged (emit will flag any missing stack rules).
func loadWithPack(file, detectDir, packDir, forcePack string) (*skill.Manifest, error) {
	m, err := skill.Load(file)
	if err != nil {
		return nil, err
	}
	stack := forcePack
	if stack == "" {
		stack = skill.Detect(detectDir).Stack
	}
	if stack == "" {
		return m, nil // no stack; base manifest only
	}
	p, err := skill.LoadPack(packDir, stack)
	if err != nil {
		// A missing pack is not fatal — emit will note the gap.
		fmt.Fprintf(os.Stderr, "note: no pack for stack %q (%v)\n", stack, err)
		return m, nil
	}
	return m.Merge(p), nil
}

// parseInterleaved parses flags that may appear before or after positional
// arguments, returning the collected positionals in order.
func parseInterleaved(fs *flag.FlagSet, args []string) []string {
	var positionals []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			os.Exit(2)
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		positionals = append(positionals, args[0])
		args = args[1:]
	}
	return positionals
}

// projectLabel names the target project by its directory, for the ledger stored
// inside it — the central manifest's own name would be wrong there.
func projectLabel(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		return filepath.Base(abs)
	}
	return filepath.Base(dir)
}

// reportCache prints whether a cached knowledge file exists, with a hint if not.
func reportCache(dir, label, rel, hint string) {
	if _, err := os.Stat(filepath.Join(dir, rel)); err == nil {
		fmt.Printf("%-8s cached (%s) — reuse it, do not re-scan source\n", label+":", rel)
	} else {
		fmt.Printf("%-8s missing (%s) — %s\n", label+":", rel, hint)
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

func writeStarter(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists; refusing to overwrite", path)
	}
	return os.WriteFile(path, []byte(starterManifest), 0o644)
}
