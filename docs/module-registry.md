# Module Registry

The map of what each module develops, its key files, and its routes/commands. Read this to find
where things live; update it after delivering a feature (`update-module-registry` /
step 5 of `deliver-feature`). Keep entries sorted and stable.

Template for a new entry:

```
## <module-name>
- **Purpose:** what this module develops.
- **Files:** `path` — role; `path` — role.
- **Routes:** `<route/command>` -> `<handler/screen>`.
```

---

## cli (`cmd/skillrunner`)
- **Purpose:** command-line entrypoint; parses args and dispatches subcommands.
- **Files:**
  - `cmd/skillrunner/main.go` — subcommand dispatch, flags, pack loading/merging.
  - `cmd/skillrunner/starter.go` — embedded starter manifest for `init`.
- **Routes (subcommands):**
  - `detect` -> `skill.Detect`
  - `list` -> `Manifest.List` (after pack merge)
  - `emit <skill>` -> `Manifest.Emit` (after pack merge)
  - `validate` -> `skill.Load` + `Manifest.Validate`
  - `init` -> `writeStarter`

## skill (`internal/skill`)
- **Purpose:** core library — manifest model, pack merging, stack detection, and instruction emission.
- **Files:**
  - `manifest.go` — `Manifest`/`Skill`/`Rules` types, `Load`, `Validate`, `Merge`, `isPackRule`.
  - `pack.go` — `Pack` type, `LoadPack`, `AvailablePacks`.
  - `detect.go` — `Detect` (file-signature stack detection).
  - `emit.go` — `Emit` (compose marching orders), `List`.
  - `jsonc.go` — JSON-with-`//`-comments reader (`decodeJSONC`).
- **Routes:** n/a (library).

## manifest + packs (data)
- **Purpose:** the declarative skill pool — shared skills, base rules, and per-stack rule packs.
- **Files:**
  - `skill.json` — shared skills + base rules (`working-policy`, `technical`, `commit-format`, `module-registry`).
  - `packs/<stack>.json` — per-stack rules (react, flutter, go, dotnet, kotlin, swift, rn).
- **Routes:** n/a (consumed by the `skill` module).
