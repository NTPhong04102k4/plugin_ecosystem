# Module Registry

The map of what each module develops, its key files, and its routes/commands. Read this to find
where things live; update it after delivering a feature (`update-module-registry` /
step 5 of `deliver-feature`). Keep entries sorted and stable.

`sr status` reports whether this file has gone **`STALE`** (source files changed since it was last
committed). Stale means the Files/Routes below may name things that have moved — confirm a path
before trusting it.

Template for a new entry:

```
## <module-name>
- **Purpose:** what this module develops.
- **Files:** `path` — role; `path` — role.
- **Routes:** `<route/command>` -> `<handler/screen>`.
```

---

## cli (`cmd/skillrunner`)
- **Purpose:** command-line entrypoint; parses flags and dispatches every subcommand.
- **Files:**
  - `cmd/skillrunner/main.go` — subcommand dispatch, flags, pack loading/merging, `reportCache`/`cacheLine`, `plural`.
  - `cmd/skillrunner/bootstrap.go` — `bootstrapBlock` (the managed CLAUDE.md text) + `ensureBootstrap` cascade (refresh in place / covered globally / append).
  - `cmd/skillrunner/starter.go` — embedded starter manifest for `init`.
  - `cmd/skillrunner/status_test.go` — `cacheLine` rendering (missing/cached/fresh/STALE) + CLI-vs-MCP parity.
- **Routes (subcommands):**
  - `detect` -> `skill.Detect`
  - `status` -> `skill.Detect` + `reportCache` (profile/registry cache state + staleness via `skill.CheckFreshness`) + `skill.Ledger.StatusLine`
  - `list` -> `Manifest.List` (after pack merge)
  - `emit <skill>` -> `Manifest.Emit` + `skill.RecordEmit` (ledger)
  - `emit all` -> `Manifest.EmitAll` (catalog view; deliberately does NOT record)
  - `apply-base` -> `Pack.ApplyBase`
  - `pull --tag <tag>` -> `skill.Pull`
  - `fetch --from <url>` -> `skill.Fetch`
  - `ui [--port]` -> `uiserver.New` (handler), served by a `net/http` server on 127.0.0.1
  - `refresh --session <s>` -> `uiserver.RunSession`
  - `ledger` -> `skill.Ledger.Summary`
  - `validate` -> `skill.Load` + `Manifest.Validate`
  - `init` -> `writeStarter`
  - `bootstrap` -> `ensureBootstrap`
  - `serve` -> `runMCPServer`

## mcp server (`cmd/skillrunner/mcp.go`)
- **Purpose:** run the pool as an MCP server over stdio (JSON-RPC 2.0). `sr` is the *server*;
  Claude Code is the host. Same behavior as the CLI — both call the same `internal/` functions.
- **Files:**
  - `cmd/skillrunner/mcp.go` — `runMCPServer`, `dispatch`, `toolDefs` (tool schemas), `runTool`, `statusText`, JSON-RPC reply helpers.
- **Routes (MCP tools):**
  - `detect_stack` -> `skill.Detect`
  - `list_skills` -> `Manifest.List`
  - `emit_skill` -> `Manifest.Emit` (+ ledger write)
  - `apply_base` -> `Pack.ApplyBase` (+ `formatApplyResults`)
  - `status` -> `statusText` (shares `cacheLine` with the CLI)
- **Not exposed:** `pull` / `fetch` / `ui` — call those via Bash. See `docs/sr-serve-mcp.md`.

## skill — core (`internal/skill`)
- **Purpose:** the manifest model, stack detection, rule merging, instruction emission, and the
  per-project caches. Nothing here reasons; it reads JSON and formats text.
- **Files:**
  - `manifest.go` — `Manifest`/`Skill`/`Rules` types, `Load`, `Validate`, `Merge`, `isPackRule`.
  - `pack.go` — `Pack`/`Asset` types, `LoadPack`, `AvailablePacks`.
  - `detect.go` — `Detect` (file-signature stack detection, bounded recursive walk).
  - `emit.go` — `Manifest.Emit` / `EmitAll` / `List` (compose marching orders).
  - `assets.go` — `Pack.ApplyBase` + `ApplyResult` (copy the stack's base config into a project).
  - `ledger.go` — `Ledger`/`LedgerRecord`, `LoadLedger`, `RecordEmit`, `Summary`, `StatusLine` (`.skillrunner/ledger.json`).
  - `freshness.go` — `CheckFreshness`/`Freshness` (git-derived staleness of a cached doc; no marker, no state file).
  - `jsonc.go` — JSON-with-`//`-comments reader (`decodeJSONC`).
- **Routes:** n/a (library).

## skill — `pull` bridge (`internal/skill`)
- **Purpose:** OpenAPI -> generated data layer + a compact digest, deterministically and at 0 token
  cost. Claude reads the digest, never the raw spec.
- **Files:**
  - `pull.go` — `PullOptions`, `Pull`, `Digest`/`DigestEndpoint`; shells out to `npx openapi-typescript`.
  - `codegen.go` — `Codegen`/`CodegenData`/`CodegenEndpoint` (the pack-supplied templates `pull` renders).
- **Writes:** `src/api/<tag>/types.ts`, `<tag>.hooks.ts`, `index.ts`.
- **Design:** `docs/sr-pull-design.md`.

## skill — `fetch` bridge (`internal/skill`)
- **Purpose:** Confluence page / Google Sheet -> clean markdown + a digest, at 0 token cost.
  Secrets are *referenced* (env var names), never stored.
- **Files:**
  - `fetch.go` — `FetchOptions`/`FetchDigest`/`DigestTable`, `Fetch`, source detection, content-hash cache, generated-file write.
  - `fetchconfig.go` — `FetchConfig`/`ConfluenceConfig`/`GoogleConfig`/`GoogleRefresh`, `LoadFetchConfig` (`.skillrunner/fetch.json`).
  - `confluence.go` — `ConfluenceCheck` + ADF renderer (`adfRenderer`: blocks/lists/tables/inline marks -> markdown).
  - `gsheet.go` — CSV export path (`gsheetFetch`/`gsheetParse`) for public sheets.
  - `gsheet_v4.go` — Sheets API v4 path (`gsheetV4Fetch`/`gsheetV4Parse`), multi-tab support.
  - `gauth.go` — Google auth (`googleTokenSource`, `googleAccessToken`): service account or refresh token.
- **Writes:** `docs/specs/<slug>.md`.
- **Design:** `docs/sr-fetch-design.md`.

## uiserver (`internal/uiserver`)
- **Purpose:** the local web UI behind `sr ui` (127.0.0.1 only) plus the runner that drives
  `sr fetch` from stored tags/sessions.
- **Files:**
  - `server.go` — `Server`, `New`, route table, `guard` middleware, handlers.
  - `runner.go` — `RunSession`/`RunResult` (re-fetch every link of a saved session).
- **Routes (HTTP):**
  - `GET /` -> `index`
  - `GET /api/browse` -> `browse`
  - `GET /api/config` -> `getConfig`
  - `POST /api/tags` -> `putTag`; `DELETE /api/tags/{tag}` -> `delTag`; `POST /api/tags/{tag}/check` -> `checkTag`
  - `POST /api/sessions` -> `putSession`; `DELETE /api/sessions/{name}` -> `delSession`
  - `POST /api/run` -> `runHandler(false)`; `POST /api/refresh` -> `runHandler(true)`
- **Guide:** `docs/sr-ui-guide.md` · **Design:** `docs/sr-ui-design.md`.

## uistore (`internal/uistore`)
- **Purpose:** per-repo config for `sr ui` — credential tags, per-feature sessions, run history.
- **Files:**
  - `uistore.go` — `UIConfig`/`Tag`/`Session`/`HistoryEntry`, `Dir`, `Path`, `Load`, `Save`.
- **Writes:** `<repo>/.skillrunner/ui.json` (atomic, `0600`); auto-adds `.skillrunner/` to the
  repo's `.gitignore` so plaintext tokens are never committed.
- **Routes:** n/a (library).

## manifest + packs (data)
- **Purpose:** the declarative skill pool — shared skills, base rules, and per-stack rule packs.
- **Files:**
  - `skill.json` — shared skills + base rules (`working-policy`, `technical`, `commit-format`, `module-registry`, `project-profile`).
  - `packs/<stack>.json` — per-stack rules (dotnet, flutter, go, java, kotlin, react, rn, swift) + `codegen` templates + base `assets`.
- **Routes:** n/a (consumed by the `skill` module).
