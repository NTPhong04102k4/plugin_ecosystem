# CLAUDE.md

## skillrunner — how to use it every session

This repo is a **central skill pool**: a Go binary `skillrunner` + `skill.json` (shared
skills) + `packs/<stack>.json` (stack-specific rules). It does NOT reason — it detects the
stack and prints "marching orders" (rules + steps) for **you (Claude) to execute**.

Architecture: **one shared skill + a per-stack rule pack**. Switching projects only changes
the pack, never the skill.

### When the user asks for something that matches a skill

1. Detect the stack: `./bin/skillrunner detect --dir <project>` (or run inside the project).
2. See the skills: `./bin/skillrunner list`
3. Get the marching orders: `./bin/skillrunner emit <skill> --dir <project>`
   - Auto-detects the stack and merges `packs/<stack>.json`.
   - Force a pack manually: `--pack react` (or flutter/go/...).
4. **Read and follow** the output — obey the "Rules you MUST follow" section.
5. If a skill is marked `[needs approval]` / has an "Approval gate": only **propose a
   plan/goal** and STOP for the user's decision; do not change files before approval.
6. If you see the warning `⚠ Stack rule groups not loaded`: no pack exists for that stack —
   tell the user or create `packs/<stack>.json` before continuing.

### Layout
- `skill.json` — shared skills (core generic + parametrized). Edit once.
- `packs/react.json`, `packs/flutter.json`, ... — per-stack rules (architecture / conventions /
  lint / templates / design-system / library-docs). Adding a new stack = adding one pack file.
- JSON allows `//` line comments.

### Other commands
- `skillrunner validate` — check the manifest
- `skillrunner init` — write a starter skill.json into a new project

## Feature delivery workflow (`deliver-feature`)

When the user gives a **Jira task code + problem description**, run the `deliver-feature` skill:

1. Consult this file and **`docs/module-registry.md`** to locate the relevant module, its files, and routes.
2. Produce a **plan + measurable goals** → present and STOP for the user's approval before coding.
3. Implement code/components per the stack conventions and design system (reuse existing first).
4. `check-diff` against conventions + run the linter; clean/refactor touched components (no behavior change).
5. Update **`docs/module-registry.md`** for the module (Purpose / Files / Routes).
6. **Verify** the change end to end.
7. Propose the commit message **as text only** (no file dumps): `ref-<jira>: <type> - <desc>`, subject ≤150 chars.
8. Commit **only after the user confirms**. **Do NOT push** — the user pushes themselves. No author trailer.

### Module registry — `docs/module-registry.md`
The map of "which module develops what, which files, and what routes". Read it to find where things
live; update it after delivering a feature so future tasks can look up files/routes per module.

Docs: `docs/skill-runner-design.md` (design), `docs/skill-taxonomy.md` (the unified skill pool).
