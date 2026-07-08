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

Docs: `docs/skill-runner-design.md` (design), `docs/skill-taxonomy.md` (the unified skill pool).
