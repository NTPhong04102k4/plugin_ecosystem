# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is

`skillrunner` (binary name `sr`/`skillrunner`) is a **central skill pool** for Claude Code: a Go
CLI that reads `skill.json` (shared skills) + `packs/<stack>.json` (per-stack rules), detects a
target project's stack, and prints deterministic "marching orders" (rules + steps) for Claude to
execute in *that* project. The binary itself **never calls an LLM and never reasons** — same
input always produces the same output; only the generated-file writers (`pull`, `fetch`,
`apply-base`, `ui`) touch a target repo's filesystem, and only with generated artifacts, never
hand-written source. See `docs/skill-runner-design.md` for the full rationale (deterministic
CLI vs. an LLM-executed script) and the declared portability trade-off (this only runs where a
shell or MCP host is available — not on claude.ai/API directly).

Architecture is deliberately **one shared skill + one rule pack per stack**: switching target
projects only changes which pack gets merged in, never the skill definitions.

## Build, test, run

```bash
make build          # -> bin/skillrunner (native; bin/skillrunner.exe on Windows)
make all            # cross-compile darwin-arm64/amd64, linux-amd64, windows (bin/skillrunner*)
make test           # go vet ./... && go test ./...
make install        # POSIX: build + copy into PREFIX (default /usr/local/bin)
                    # Windows: go install -> %USERPROFILE%\go\bin (already on PATH)
                    # BOTH: also records this dir in ~/.skillrunner/home and writes the `sr` shim
make clean          # rm -rf bin
```

`install` is three steps because the binary alone is not a working setup: without the pointer file
it cannot find this `skill.json` from any other project, and without the shim `sr` does not exist.
All three re-run on every install, so there is no manual copy step to forget. On Windows it writes
**two** shims — `sr.cmd` for cmd.exe/PowerShell and an extensionless `sr` sh script for Git Bash,
which ignores `PATHEXT`; shipping only the `.cmd` would leave `sr` broken in Git Bash.

The Makefile is dual-platform: it branches on `OS=Windows_NT` and every Windows recipe is
cmd-native (no sh/grep/awk/rm/sudo, which cmd.exe does not have) and starts with `chcp 65001`,
since cmd defaults to a legacy code page that renders this file's UTF-8 output as mojibake.
Two consequences when editing it: the shell is pinned (`SHELL := cmd.exe`) so a stray `sh.exe`
on PATH cannot make half a build take POSIX recipes, and the Windows **help text is duplicated
by hand** — cmd cannot run the `grep`/`awk` introspection over the `## ` comments, so a new
target must be added to both branches.

`make test` passes on Windows as well as POSIX, and two conventions keep it that way when adding
tests:

- **Pool paths are strings, not `filepath.Join`.** Skills spell `$SKILLRUNNER_HOME/docs/...` with
  `/` on every platform and `resolveHome` substitutes only the home prefix, so emitted text is
  `home + "/docs/..."` — mixed separators on Windows, by design. Assert against that literal
  (see `taxonomyRel` in `emit_test.go`), never against a `filepath.Join`ed expectation.
- **Don't assert Unix permission bits.** Windows has none; `os.Chmod` there only flips the
  read-only attribute, so `uistore`'s 0600 write stats back as 0666 and that one assertion is
  guarded by `runtime.GOOS != "windows"`. Worth knowing beyond the test: on Windows `ui.json`'s
  Atlassian tokens are **not** perm-protected — only NTFS ACLs could do that.

Run a single test package or case directly with `go test` (the Makefile only wires the
all-packages form):

```bash
go test ./internal/skill/... -run TestEmit -v
go test ./cmd/skillrunner/... -run TestCacheLine -v
```

Requires Go ≥ 1.26 (see `go.mod`). `skillrunner pull` additionally shells out to `npx
openapi-typescript`, so exercising that path needs Node/npx on PATH. There is no repo-root
`.golangci.yml` — `packs/assets/go/.golangci.yml` is a generated *asset* this tool ships to
**consumer** Go projects via `apply-base`, not lint config for this repo itself; `make test`
(`go vet` + `go test`) is the check that matters here.

Manually exercise the CLI against the local build:

```bash
./bin/skillrunner detect                          # stack detection (no manifest needed)
./bin/skillrunner list                             # skills, after pack merge
./bin/skillrunner emit <skill> --dir <project>      # marching orders
./bin/skillrunner status --dir <project>            # cache/staleness of profile+registry+ledger
./bin/skillrunner home                              # which skill.json resolves, and via which rung
./bin/skillrunner serve                             # MCP server over stdio (JSON-RPC on stdin — appears to hang if run by hand; that's correct, it's waiting for a client)
```

`skill.json` and `packs/*.json` are JSONC (allow `//` comments) — read them with that in mind,
and don't strip comments when editing.

### Finding the manifest

`-f` used to default to the relative string `"skill.json"`, which silently assumed the process was
started inside this repo. Run from a consumer project — the tool's entire purpose — it failed with
`read manifest "skill.json"`, so every `sr` verb named in a bootstrapped `CLAUDE.md` was a dead
letter there. `resolveManifestPath` (`cmd/skillrunner/manifestpath.go`) now walks a fixed ladder:

1. `-f/--file` — explicit, and never second-guessed: a missing path is an error, not a reason to
   quietly load a different pool's rules
2. `./skill.json` — a project carrying its own pool (the documented "drop the binary + skill.json +
   packs/ into any project" layout, which is why this rung outranks the central ones)
3. `$SKILLRUNNER_HOME/skill.json`
4. `~/.skillrunner/home` — pointer file written by `make install`

A ladder, not a search: walking parent directories was rejected because the answer would depend on
where you stood, breaking the "same input => same output" guarantee. Rungs 3 and 4 fall through when
they point at a pool that moved, rather than dead-ending. `sr home` reports which rung won and
what each one points at — it resolves lazily so it still works when *nothing* resolves, which is
exactly when you need it. `init` opts out of the ladder entirely: it writes a manifest rather than
reading one.

## Architecture

```
cmd/skillrunner/         CLI entrypoint: flag parsing, subcommand dispatch, pack merge
  main.go                 dispatch table, reportCache/cacheLine, plural
  manifestpath.go          resolveManifestPath — the -f > cwd > env > pointer ladder
  home.go                  `sr home` — diagnose resolution; --set pointer; --shims write `sr`
  bootstrap.go             `sr bootstrap` — managed CLAUDE.md block (refresh/skip/append cascade)
  starter.go               `sr init` — embedded starter skill.json
  mcp.go                   `sr serve` — MCP/JSON-RPC server; same internal/ calls as the CLI

internal/skill/           core library — nothing here calls an LLM; it reads JSON and formats text
  manifest.go, pack.go     Manifest/Skill/Rules/Pack types, Load, Validate, Merge
  detect.go                stack detection via file signatures (bounded recursive walk)
  emit.go                  Manifest.Emit/EmitAll/List — compose the marching-orders text
  assets.go                Pack.ApplyBase — copy a stack's base config into a target repo
  ledger.go                .skillrunner/ledger.json — RecordEmit, BuildCommit, Summary
  freshness.go             CheckFreshness — git-derived STALE detection for cached docs
  pull.go, codegen.go      OpenAPI -> types.ts/hooks/index.ts + digest (shells to openapi-typescript)
  fetch.go, fetchconfig.go,
  confluence.go, gsheet.go,
  gsheet_v4.go, gauth.go   Confluence/Sheets -> docs/specs/<slug>.md + digest; secrets referenced
                           via .skillrunner/fetch.json, never stored
  jsonc.go                 JSON-with-// -comments reader

internal/uiserver/        `sr ui` — local HTTP server (127.0.0.1 only) + embedded SPA + session runner
internal/uistore/         <repo>/.skillrunner/ui.json (0600, auto-added to that repo's .gitignore)

skill.json                 shared skills + base rule groups (working-policy, technical,
                            commit-format, module-registry, project-profile) — edit once
packs/<stack>.json          per-stack rules (architecture/conventions/lint/templates/
                            design-system/library-docs) + codegen templates + base assets —
                            one new file = one new supported stack, no Go changes required
                            (unless you also want auto-detection, which lives in detect.go)
```

`docs/module-registry.md` is the authoritative, kept-up-to-date map of module -> files ->
routes/subcommands for this repo; consult it instead of re-deriving file responsibilities from
scratch. `docs/skill-flow.md` explains how the skills in `skill.json` chain together
(appliesRules matrix + which skill reads/writes which cache file).

## Working in this repo

This repo governs itself the same way it governs any Go target: run `skillrunner status` /
`list` / `emit <skill>` here before improvising, per the managed block at the bottom of this
file. Two things are specific to developing the tool itself rather than consuming it:

- **`skill.json` and `packs/*.json` are data, not code** — most feature work here means adding
  or editing a skill/rule entry, not writing Go. Only touch `internal/skill` when the *mechanism*
  (detection, emission, a new 0-token bridge) needs to change.
- **Determinism is a hard constraint**: any change to `emit`/`pull`/`fetch`/`apply-base` must
  keep "same input -> same output" true, since that guarantee is the tool's whole value
  proposition over an LLM-executed script.

Commit message format for this repo (from `skill.json`'s `commit-format` rule group):
`ref-<jira-number>: <type> - <short description>` using Conventional Commits types
(feat/fix/docs/style/refactor/perf/test/build/ci/chore/revert), subject ≤150 chars, no
Co-Authored-By/author trailer. Ask for the Jira number (or derive from the branch) if unknown.

<!-- skillrunner:begin (managed by `sr bootstrap` — do not edit inside) -->
## skillrunner (`sr`) — use it every session

This project (stack: **go**) is served by `sr` (aka `skillrunner`), a central
skill dispatcher on your PATH. It detects the stack and prints "marching orders"
(rules + steps) for YOU (Claude) to execute — it never reasons and never rewrites your
source. `emit` only appends to `.skillrunner/ledger.json`; `pull`/`fetch`/`apply-base` write
generated files (types / markdown / base config). Nothing else in the repo is touched.

When a request matches a skill:
1. `sr status` — stack + whether docs/project-profile.md and docs/module-registry.md are cached.
   A cached doc marked `STALE` has fallen behind the source: still use it to orient, but confirm any
   file/symbol still exists before relying on it, and ask the user before rebuilding it.
2. `sr list` — skills with one-line descriptions; map the task to the right one.
3. `sr emit <skill>` — print the marching orders, then READ and FOLLOW the "Rules you MUST follow" section.
4. A skill tagged `[needs approval]` → only propose a plan/goal and STOP for the user; do not edit files first.
5. First task in a project with no docs/project-profile.md → run `learn-project` before implementing.

If a task clearly matches a skill, prefer `sr emit <skill>` over improvising.

Beyond skills, two deterministic 0-token bridges — use them instead of reading raw
sources yourself: `sr pull` (OpenAPI → types + hooks + digest) and `sr fetch`
(Confluence / Google Sheet → clean markdown + digest).
<!-- skillrunner:end -->

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **plugin_ecosystem** (919 symbols, 2325 relationships, 77 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> Index stale? Run `node .gitnexus/run.cjs analyze` from the project root — it auto-selects an available runner. No `.gitnexus/run.cjs` yet? `npx gitnexus analyze` (npm 11 crash → `npm i -g gitnexus`; #1939).

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows. For regression review, compare against the default branch: `detect_changes({scope: "compare", base_ref: "main"})`.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `query({search_query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `context({name: "symbolName"})`.
- For security review, `explain({target: "fileOrSymbol"})` lists taint findings (source→sink flows; needs `analyze --pdg`).

## Never Do

- NEVER edit a function, class, or method without first running `impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `rename` which understands the call graph.
- NEVER commit changes without running `detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/plugin_ecosystem/context` | Codebase overview, check index freshness |
| `gitnexus://repo/plugin_ecosystem/clusters` | All functional areas |
| `gitnexus://repo/plugin_ecosystem/processes` | All execution flows |
| `gitnexus://repo/plugin_ecosystem/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->
