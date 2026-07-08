# Unified Skill Taxonomy — the central skill pool

> Decision: **divide skills by PURPOSE (verb), not by stack.**
> Each verb is a **single skill** (the intent/steps). Differences between React, Flutter,
> Kotlin, Swift, Go, React Native, ASP.NET live in a **per-stack rule pack**.
> `skillrunner` detects the stack → merges `skill (shared) + rule pack (stack)` → `emit`s the marching orders.

Evidence for this decision: two projects on different stacks (`app-hrm` Flutter/GetX, `ghm-hrm` React)
**independently converged on the same set of verbs**. That is the real structure of the work, not a coincidence.

---

## 1. Principle: separate INTENT from CONTENT

```
SKILL (intent, verb)      →  same across every stack   →  written & maintained once
  scaffold-data, build-ui, refactor, ...

RULE PACK (content)       →  differs per stack          →  where the real effort goes
  convention, template, lint, design-tokens, library-docs
```

Mistake to avoid: `scaffold-data-react`, `scaffold-data-flutter`, ... → 7 stacks × N skills =
maintenance explosion, and fixing one shared principle means editing 7 places. Correct:
`scaffold-data` (one copy) + 7 rule packs.

---

## 2. CORE skills — 100% generic, written once, used on every stack

Not tied to a stack. Need no rule pack (or only a very thin one).

| Skill | Purpose | Origin |
|-------|---------|--------|
| `plan-feature` | Request → plan + goals for the user to decide (no code) | working-policy + skillrunner |
| `spec-from-source` | Jira/Confluence/Swagger → structured spec before coding | app-hrm `atlassian-spec`, `swagger-import` |
| `commit` | Split changes into per-module commits, present, confirm before committing | app-hrm `commit` |
| `check-diff` | Review `git diff` against conventions → report `file:line` + run lint | ghm `check-convention`, app `clean-code` |
| `refactor` | Improve within scope, do NOT change behavior | both `refactor`/`clean-component` |
| `explain-lib` | Explain how the project ACTUALLY uses a library (from curated docs) | ghm `explain-lib` + `libraries/` |
| `working-policy` | Discipline: ask when ambiguous · plan file before code · minimal change · verify with git | app `working-policy` |

> `working-policy` is **cross-cutting**: every other skill inherits it (like the shared `rules` in skill.json).

---

## 3. STACK-PARAMETRIZED skills — one intent, content from the pack

Verb × stack matrix. A cell = the existing name in the sample project (if any); blank = a pack still to write.

| Skill (intent) | Purpose | React | Flutter | Kotlin | Swift | Go | RN | ASP.NET |
|----------------|---------|-------|---------|--------|-------|----|----|---------|
| `scaffold-data` | Build the data layer from an API/Swagger | `add-api` | `add-data`,`swagger-import` | — | — | — | — | — |
| `scaffold-screen` | Build a screen/feature per the architecture | `new-list`,`new-dialog` | `add-feature` | — | — | — | — | — |
| `build-ui` | Build/edit UI per the design system | `gen-components/*` | `fix-ui` | — | — | — | — | — |
| `gen-routes` | Generate routes from the file structure | (react-router) | (GetX routes) | — | — | — | — | — |

A blank does not mean a missing skill — the skill is still shared; you just need to **write the rule pack** for that stack.

---

## 4. What a RULE PACK contains (common shape for every stack)

Each pack (e.g. `packs/react.json`, `packs/flutter.json`) declares:

- **architecture** — the layer flow (data → logic → view) and each layer's responsibility
- **conventions** — naming, folder structure, i18n, formatting rules
- **lint** — the code-style standard + how to run the stack's linter
- **templates** — file templates for `scaffold-*` (data layer, screen, component)
- **design-system / tokens** — shared components, color/spacing tokens
- **library-docs** — an index of the libraries the project actually uses (to prevent invented APIs) — following ghm-hrm's `libraries/` pattern
- **detect** — the signal that identifies the stack (see §6)

---

## 5. The first two packs (extracted from sample projects)

### 5a. `react` pack (from ghm-hrm)
- **architecture:** `src/api/{resource}.ts` (data) → `handler.tsx` (logic hook) → `index.tsx` (view)
- **stack:** React 18 + TS + Vite, TanStack Query/Table, React Hook Form, CASL (permissions), the `@ghm` design system
- **key conventions:**
  - always `return res` so backend messages reach the toast
  - `useConfirm` for delete / icon-only / auto-save / exiting a dirty form
  - global search debounced 500ms, reset to page 1, server-side
  - avoid races: put every param into the react-query `queryKey`
  - 3 table shapes: server paginated (skip/take) · small (`pageCount: -1`) · infinite (`useInfiniteQuery`)
  - RHF validation: title ≤100, description ≤250, trim
- **templates:** `add-api` (types+service+query hooks) · `new-list` · `new-dialog` · 9 gen-components patterns
- **library-docs:** the existing `docs/reference/libraries/` folder — reuse as-is

### 5b. `flutter` pack (from app-hrm)
- **architecture:** View → Controller → Repository → Provider → native ForgeRock (`AuthMethodChannel`)
- **stack:** Flutter + GetX (binding/controller/page/repository), ScreenUtil, GetStorage, multi-brand + multi-flavor
- **key conventions:**
  - API ONLY via `AuthMethodChannel` — no dio/http
  - real entrypoints are `main_{dev,staging,prod}.dart`
  - no hardcoded colors — use semantic tokens (5 layers: primitives→semantic→ThemeController→ThemeData)
  - size via ScreenUtil; Vietnamese UI strings; `log` over `print`; prefer `const`
  - build/run via `scripts/flutter_flavor.sh` per flavor
- **templates:** `add-data` (Provider/Model/Repository) · `add-feature` (binding/controller/page/route) · `fix-ui`
- **library-docs / references:** architecture, conventions, getx-conventions, theme-tokens, toolkit, rules

---

## 6. "Auto-division" = detect the stack (deterministic, no reasoning)

`skillrunner detect` scans signature files and picks the pack:

| Signature file | Stack | Pack |
|----------------|-------|------|
| `pubspec.yaml` | Flutter | `flutter` |
| `package.json` contains `react-native` | React Native | `rn` |
| `package.json` contains `react` (not RN) | React | `react` |
| `build.gradle(.kts)` + Kotlin/Java | Android native | `kotlin` |
| `*.xcodeproj` / `Package.swift` | iOS Swift | `swift` |
| `go.mod` | Go | `go` |
| `*.csproj` / `*.sln` | ASP.NET | `dotnet` |

Flow: `detect` → pick pack → `emit <skill>` merges **skill (shared) + rules (pack)** → Claude executes.

---

## 7. Mapping to skillrunner (already built)

- `skills` in `skill.json` = §2 + §3 (the shared verbs) — **written once**.
- `rules` groups = per-stack packs (§4–§5) — split into `packs/<stack>.json`.
- The `skillrunner detect` command (§6) auto-picks the pack; `emit` loads the matching pack.
- `working-policy` → lives in the shared `rules`; every skill with `appliesRules` inherits it.

---

## 8. Roadmap

1. ✅ skillrunner core (shared skills + rules + emit) — done.
2. ✅ `react` + `flutter` packs (extracted from the two sample projects).
3. ✅ `skillrunner detect` + split `packs/<stack>.json`.
4. ✅ Core generic skills (`plan-feature`, `spec-from-source`, `commit`, `refactor`, `explain-lib`, `check-diff`).
5. ✅ `go`, `dotnet`, `kotlin`, `swift`, `rn` packs (composed from per-stack best practices — no sample project, tune when applied for real).
6. ▶ Try it on `Flutter_weather` (currently empty) to prove "drop it in and it runs, no restructuring".
7. ▶ Add `go test` for `Detect`/`Emit` (guarantee deterministic output).

> Note: the `go`/`dotnet` packs are **backend** — their `design-system` group is reinterpreted as
> "API-contract conventions" (response envelope, status codes, OpenAPI), not UI. The `build-ui` skill
> is rarely used for those two stacks.

---

## 9. Recorded risks

1. **Do not duplicate skills per stack** — duplicate only the *rule pack*.
2. **The real effort is in the rule pack**, not in splitting skills; intent is small and stable.
3. **Code-generating skills always depend on the pack** for their output → the pack is mandatory for `scaffold-*` and `build-ui`.
4. **Core generic skills** are written once, not tied to a stack.
5. Packs must keep **accurate library-docs** (to prevent invented APIs) — following ghm-hrm's `libraries/` pattern.
