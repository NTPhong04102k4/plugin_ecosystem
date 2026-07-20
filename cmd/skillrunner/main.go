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
  skillrunner emit all                 Print marching orders for EVERY skill (catalog dump; not recorded)
  skillrunner apply-base               Copy the stack's base config files (eslint/linter/etc.) into the project
  skillrunner pull --tag <tag>         Bridge authswagger spec -> types.ts + react-query hook skeleton + digest (0 token)
  skillrunner fetch --from <url>       Bridge Confluence/Google Sheet (link+token) -> markdown + digest (0 token)
  skillrunner ledger                   Show which skills have already been emitted in this project
  skillrunner validate                 Check that the manifest is well-formed
  skillrunner init                     Write a starter skill.json in the current dir
  skillrunner bootstrap                Ensure the project's CLAUDE.md tells Claude to use sr
  skillrunner serve                    Run as an MCP server over stdio (exposes detect/list/emit/apply-base as tools)

Flags:
  -f, --file <path>   Manifest path (default: skill.json)
  -p, --pack <stack>  Force a stack pack (e.g. react, flutter). Default: auto-detect.
      --dir <path>    Project dir to detect against (default: manifest's dir)
      --force         (bootstrap) Write the project CLAUDE.md even if already covered
                      (apply-base) Overwrite existing config files instead of skipping them

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
	// pull-only flags (ignored by other commands)
	var pullFrom, pullEnv, pullSpec, pullTag, pullOut, pullBase, pullCodegen string
	var pullNoCache bool
	fs.StringVar(&pullFrom, "from", "http://localhost:8080/openapi.json", "pull: OpenAPI spec URL (or a local file path)")
	fs.StringVar(&pullEnv, "env", "dev", "pull: authswagger environment (?env=)")
	fs.StringVar(&pullSpec, "spec", "", "pull: authswagger spec group (?spec=)")
	fs.StringVar(&pullTag, "tag", "", "pull: OpenAPI tag to filter operations by (required)")
	fs.StringVar(&pullOut, "out", "", "pull: output dir (default src/api/<tag>)")
	fs.StringVar(&pullBase, "base", "", "pull: override proxyBase (default: spec servers[0].url)")
	fs.StringVar(&pullCodegen, "codegen", "ts", "pull: type generator (only \"ts\")")
	fs.BoolVar(&pullNoCache, "no-cache", false, "pull/fetch: ignore cached digest and regenerate")
	// fetch-only flags (reuses --from/--out/--no-cache above; ignored by other commands)
	var fetchEmail, fetchTokenEnv, fetchRange, fetchGid string
	fs.StringVar(&fetchEmail, "email", "", "fetch: Confluence Basic-auth email (default: config)")
	fs.StringVar(&fetchTokenEnv, "token-env", "", "fetch: env var holding the token/secret (default: per-source)")
	fs.StringVar(&fetchRange, "range", "", "fetch: Google Sheet A1 range (e.g. Sheet1!A1:H)")
	fs.StringVar(&fetchGid, "gid", "", "fetch: Google Sheet tab id (default: #gid= in the URL)")
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
			fatal(fmt.Errorf("emit requires a skill name (or `all`); run `skillrunner list`"))
		}
		m, err := loadWithPack(file, detectDir, packDir, pack)
		if err != nil {
			fatal(err)
		}
		// `emit all` is a catalog dump of every skill's orders. It is a read-only
		// reference view, so it prints and returns without touching the ledger.
		if rest[0] == "all" {
			out, err := m.EmitAll()
			if err != nil {
				fatal(err)
			}
			fmt.Print(out)
			// `emit all` is a catalog view, not an execution. It deliberately does
			// NOT touch the ledger — recording all skills on a browse would make a
			// later session think each was actually run here. Say so, so the empty
			// ledger is expected behavior, not a surprise.
			fmt.Fprintln(os.Stderr, "\n(catalog view — nothing recorded in the ledger; `emit <skill>` records a real run)")
			break
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

	case "apply-base":
		// Copy the stack's base config files into the project. This needs only the
		// pack (not the shared manifest): the assets live on the pack.
		stack := pack
		if stack == "" {
			stack = skill.Detect(detectDir).Stack
		}
		if stack == "" {
			fatal(fmt.Errorf("could not detect a stack; pass --pack <stack> (available: %v)", skill.AvailablePacks(packDir)))
		}
		p, err := skill.LoadPack(packDir, stack)
		if err != nil {
			fatal(err)
		}
		results, err := p.ApplyBase(packDir, detectDir, force)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("apply-base [%s] -> %s\n", stack, detectDir)
		for _, r := range results {
			fmt.Printf("  %s %-28s %s\n", applyMark(r.Status), r.To, applyNote(r))
		}
		if !force && anySkipped(results) {
			fmt.Println("\nSome files were skipped because they already exist. Re-run with --force to overwrite them.")
		}

	case "pull":
		// Deterministic bridge: authswagger spec -> data layer + digest (0 token).
		// Resolves the target project's stack pack for its codegen templates.
		stack := pack
		if stack == "" {
			stack = skill.Detect(detectDir).Stack
		}
		if stack == "" {
			fatal(fmt.Errorf("could not detect a stack for pull; pass --pack <stack> (available: %v)", skill.AvailablePacks(packDir)))
		}
		p, err := skill.LoadPack(packDir, stack)
		if err != nil {
			fatal(err)
		}
		_, digestJSON, err := skill.Pull(skill.PullOptions{
			From:       pullFrom,
			Env:        pullEnv,
			Spec:       pullSpec,
			Tag:        pullTag,
			Out:        pullOut,
			Base:       pullBase,
			Codegen:    pullCodegen,
			ProjectDir: detectDir,
			PackDir:    packDir,
			NoCache:    pullNoCache,
		}, p)
		if err != nil {
			fatal(err)
		}
		fmt.Println(digestJSON)

	case "fetch":
		// Deterministic bridge: Confluence/Google Sheet -> markdown + digest (0 token).
		_, digestJSON, err := skill.Fetch(skill.FetchOptions{
			From:       pullFrom,
			Out:        pullOut,
			Email:      fetchEmail,
			TokenEnv:   fetchTokenEnv,
			Range:      fetchRange,
			Gid:        fetchGid,
			ProjectDir: detectDir,
			NoCache:    pullNoCache,
		})
		if err != nil {
			fatal(err)
		}
		fmt.Println(digestJSON)

	case "serve":
		// Run as an MCP server over stdio. packDir is where packs/ live (next to
		// the manifest); detectDir is the default project when a tool omits "dir".
		if err := runMCPServer(file, detectDir, packDir); err != nil {
			fatal(err)
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
	fmt.Print(cacheLine(dir, label, rel, hint))
}

// cacheLine is reportCache as a string, so both the CLI and the MCP server can
// render identical cache status without one printing and the other capturing.
func cacheLine(dir, label, rel, hint string) string {
	if _, err := os.Stat(filepath.Join(dir, rel)); err == nil {
		return fmt.Sprintf("%-8s cached (%s) — reuse it, do not re-scan source\n", label+":", rel)
	}
	return fmt.Sprintf("%-8s missing (%s) — %s\n", label+":", rel, hint)
}

// applyMark returns a status glyph for one apply-base result line.
func applyMark(status string) string {
	switch status {
	case skill.ApplyWrote, skill.ApplyOverwrote:
		return "✓"
	case skill.ApplySkipped:
		return "•"
	default: // ApplyMissing or anything unexpected
		return "✗"
	}
}

// applyNote picks the human-readable trailing note for a result line.
func applyNote(r skill.ApplyResult) string {
	switch r.Status {
	case skill.ApplyOverwrote:
		if r.Detail != "" {
			return "overwrote — " + r.Detail
		}
		return "overwrote"
	case skill.ApplyWrote:
		return r.Detail
	default:
		return r.Detail
	}
}

// anySkipped reports whether any asset was left untouched because it existed.
func anySkipped(results []skill.ApplyResult) bool {
	for _, r := range results {
		if r.Status == skill.ApplySkipped {
			return true
		}
	}
	return false
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
