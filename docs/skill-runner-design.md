# Skill Runner (Go) — Design & Decisions

> Goal: A single **Go binary** (builds to `bin`/`.exe`) that acts as a "skill runner".
> You drop the binary + a **declarative JSON manifest** (not a detailed script) into any
> project, and Claude uses it to run skills — **without restructuring per project**.

Manifest format: **JSON** (`skill.json`).

---

## 1. Core concept: "request file" vs "detailed execution file"

This is the central idea. The distinction:

| Type | What it is | Example |
|------|------------|---------|
| **Request file (declarative)** ✅ | Only **declares** "what to run", no logic | `skill.json`: `{"skill": "build", "args": {...}}` |
| **Detailed execution file (imperative)** ❌ | Contains all execution logic, rewritten per project | `build.sh`, a 200-line `deploy.js` |

**Principle:** the execution logic lives **inside the Go binary** (written once, used everywhere).
Each project only needs **one small JSON file declaring intent**. Switch projects → change the
JSON only, not the code.

```
┌─────────────────┐    reads    ┌──────────────────┐
│  skill.json     │ ──────────► │  skillrunner     │  (logic lives here, shared)
│  (declarative)  │             │  (Go binary)     │
└─────────────────┘             └──────────────────┘
      ▲ one file per project              ▲ one binary for all projects
```

---

## 2. Three integration options — how does communication happen?

### Option A — Standalone CLI ⭐ (simplest)

Claude calls the binary via the **Bash** tool, communicating through **stdin/stdout/exit code**.

```
Claude ──(Bash tool)──► ./skillrunner run skill.json ──► stdout: JSON result
                                                     └──► exit 0 = OK, ≠0 = error
```

**Flow:**
1. Claude runs `./skillrunner run skill.json` (or `skillrunner run --skill build`).
2. The binary reads `skill.json` and executes the matching skill.
3. The binary prints the result to **stdout** (JSON or text) → Claude reads it back.
4. The **exit code** signals success/failure.

**Pros:**
- No extra configuration. The binary just sits in the project (or on PATH).
- Fully portable: copy the binary + `skill.json` and it runs on any machine / any project.
- Easy to debug: you can run `./skillrunner run skill.json` by hand.
- Matches the "drag the tool file in to run" idea.

**Cons:**
- Claude must "know" to call this (needs it noted in `CLAUDE.md` or you tell it).
- Not automatic — Claude only calls it on request / when it reads the instructions.

**Use when:** you want something fast, compact, portable, low-magic. **Recommended first.**

---

### Option B — MCP server (deep integration)

The binary runs as an **MCP server** (Model Context Protocol). Claude automatically **discovers**
skills as "tools" and calls them directly — no Bash needed.

```
Claude ◄──(MCP protocol over stdio/JSON-RPC)──► skillrunner (running in background)
   │
   ├─ discover: "this server has tools: build, deploy, test..."
   └─ call: tool "build" with params {...} ──► server returns the result
```

**Flow:**
1. You register the server once in Claude Code's MCP config (`.mcp.json` / settings).
2. On startup Claude handshakes with the server and asks "what tools do you have?".
3. The binary reads `skill.json` → declares each skill as an **MCP tool** (name, description, param schema).
4. Claude sees these tools and **calls them automatically when appropriate**, passing JSON params.
5. Communication is **JSON-RPC over stdio** (or HTTP/SSE) — standard MCP, not raw stdout.

**Pros:**
- Claude **automatically** knows which skills exist and calls them — no need to remind it.
- Clear parameter schemas → Claude passes the right arguments.
- "Native" integration, like other MCP servers (Figma, Atlassian...).

**Cons:**
- More complex: you must implement the MCP protocol (handshake, list tools, call tool).
- Requires one-time config per machine / location (less "drag-and-drop").
- The server runs in the background, harder to debug than a CLI.

**Use when:** you want Claude to use skills automatically without reminders, and accept a one-time setup.

---

### Option C — Claude Code hook (event-driven, automatic)

The binary runs as a **hook**: Claude Code calls it **on events** (before/after using a tool,
session start/end, on prompt submit...). Configured in `settings.json`.

```
Event (e.g. after editing a file) ──► Claude Code runs the hook ──► skillrunner
                                                                    │ reads stdin (event JSON)
                                                                    └► stdout/exit code: response
```

**Flow:**
1. You declare the hook in `settings.json`, pointing at the binary + the event to catch (e.g. `PostToolUse`).
2. When the event fires, Claude Code runs the binary and **feeds the event data via stdin** (JSON).
3. The binary handles it (e.g. run the "format" skill after each file edit), returning via stdout/exit code.
4. The exit code / output can **block or allow** the action, or inject feedback for Claude.

**Pros:**
- **Fully automatic** — no one has to call it; it fires on the right event (e.g. auto-format, auto-test).
- Great for "whenever X, do Y" rules.

**Cons:**
- Not meant for "run a skill on request" — it is reactive to events.
- Config is tied to a specific Claude Code install, less portable.
- Can be annoying if it catches the wrong event (runs too often).

**Use when:** you want event-driven automation (auto format/lint/test), not manual invocation.

---

## 3. Quick comparison

| Criterion | A. CLI | B. MCP server | C. Hook |
|-----------|--------|---------------|---------|
| Code complexity | Low | High | Medium |
| Config per project | None (just the file) | Once / machine | Once / machine |
| Claude uses it automatically | No (needs a nudge) | **Yes** | **Yes** (on events) |
| Portability ("drag-and-drop") | **Highest** | Medium | Low |
| Easy to debug by hand | **Highest** | Low | Medium |
| Protocol | stdout + exit code | JSON-RPC (MCP) | stdin JSON + exit code |
| Fit for your goal | ✅✅✅ | ✅✅ | ✅ |

**Recommendation:** start with **A (CLI)** — it best matches "drop the binary + JSON file in and it
runs, no restructuring". Later you can **upgrade the same binary to B (MCP)** — just add a `skillrunner
mcp` subcommand that reuses the `skill.json` loading logic. The architecture below is built to support both.

---

## 4. What is a "skill"? — the three options explained

"Skill" = the unit of work the binary runs. Three interpretations:

### Option 1 — Commands/scripts declared in the manifest ⭐

The JSON manifest declares **steps** (a sequence of shell commands / scripts) for the binary to run.

```jsonc
// skill.json
{
  "skills": {
    "build": {
      "description": "Build the project",
      "steps": ["npm install", "npm run build"]
    },
    "deploy": {
      "description": "Deploy to staging",
      "steps": ["npm run build", "./scripts/upload.sh staging"]
    }
  }
}
```

- **Essence:** the binary is a declarative task runner (like Make/npm scripts but portable, one binary).
- **Pros:** extremely flexible; each project differs only in JSON. No new Go code for a new skill.
- **Cons:** still depends on the environment's shell commands (npm, bash...). Logic is still "commands", just declared compactly.
- **Best fit for your idea:** "request files, not detailed execution files".

### Option 2 — Claude Code SKILL.md

The binary reads and runs skills in Claude Code's **`SKILL.md`** format (a markdown file + frontmatter).

- **Essence:** the binary becomes a "runner" for Claude Code's existing skill system.
- **Pros:** reuse skills already written in the Claude Code format; unified ecosystem.
- **Cons:** `SKILL.md` is **instructions for the model to read**, not a program to "run". The binary can
  only *load/route* them, not "understand" them like Claude. Little value when run standalone.
- **Choose when:** you already have many `SKILL.md` files and want one tool to manage/route them.

### Option 3 — Separate plugin binaries/modules

Each skill is a **standalone plugin** (a child binary, or a Go plugin `.so`) orchestrated by the main binary.

```jsonc
// skill.json
{
  "skills": {
    "build":  { "plugin": "./plugins/build",  "args": {...} },
    "deploy": { "plugin": "./plugins/deploy", "args": {...} }
  }
}
```

- **Essence:** the main binary is an "orchestrator"; each skill is its own program.
- **Pros:** most powerful; each skill can be written in any language, well isolated.
- **Cons:** heaviest — you must distribute many child binaries, losing the "single-file drag-and-drop".
- **Choose when:** a large, complex skill set that needs isolated/independent skills.

### Decision table

| | 1. Declared commands | 2. SKILL.md | 3. Separate plugins |
|--|----------------------|-------------|---------------------|
| Portable "single file" | ✅✅✅ | ✅✅ | ❌ |
| New skill without Go code | ✅ | ✅ | ✅ |
| Runs standalone (no Claude needed to understand) | ✅ | ❌ | ✅ |
| Distribution complexity | Low | Low | High |
| Fit for your goal | ✅✅✅ | ✅ | ✅ |

**Recommendation:** **Option 1** (declared commands in the manifest) best fits the goal:
one binary + one JSON file, switching projects only changes JSON, no restructuring.

---

## 5. Overall proposal (for your approval)

> **Standalone CLI (A) + declared-command skills (1) + JSON manifest.**

Intended binary architecture:

```
skillrunner
├── run <skill> [--file skill.json]   # run one skill per the manifest  (Option A)
├── list [--file skill.json]          # list skills in the manifest
├── validate [--file skill.json]      # check the manifest is valid
└── mcp                               # (later) run as an MCP server (Option B)
```

- Cross-platform builds: `bin/skillrunner` (macOS/Linux) and `skillrunner.exe` (Windows) from the same Go source.
- Uses only the **Go standard library** (`encoding/json`, `os/exec`) — no external deps, static binary, easy to ship.
- Layered so `mcp` can be added later without rewriting the `skill.json` loading.

---

> Note: the final implementation evolved beyond this initial proposal — see `docs/skill-taxonomy.md`
> for the shipped model (shared skills + per-stack rule packs + `detect`).
