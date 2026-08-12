# skillrunner

**Kho skill trung tâm cho Claude Code** — một Go binary (`skillrunner`) đọc `skill.json` (skill dùng chung) + `packs/<stack>.json` (rule theo stack), phát hiện stack của project rồi **in ra "marching orders" (bộ rule + các bước) để Claude thực thi**.

Ngoài việc phát rule, nó còn là **cầu nối 0-token** giữa các nguồn dữ liệu thô và Claude: `sr pull` (OpenAPI → data layer), `sr fetch` (Confluence/Google Sheet → markdown), `sr ui` (web cục bộ quản lý credential/phiên).

> `skillrunner` **không suy luận, không gọi LLM**. Mọi thứ nó làm đều tất định. Nó *có* ghi file — nhưng chỉ là file **sinh ra bằng code** (types, markdown, config, cache), không bao giờ tự sửa code nghiệp vụ của bạn. Xem [Nó ghi những file nào](#nó-ghi-những-file-nào).

---

## Mục lục
- [Ý tưởng cốt lõi](#ý-tưởng-cốt-lõi)
- [CLI tất định — không phải Claude "thực thi"](#cli-tất-định--không-phải-claude-thực-thi)
- [Ba làn 0-token: emit / pull / fetch](#ba-làn-0-token-emit--pull--fetch)
- [Nó ghi những file nào](#nó-ghi-những-file-nào)
- [Cài đặt](#cài-đặt)
- [Dùng cho mọi project (wrapper `sr`)](#dùng-cho-mọi-project-wrapper-sr)
- [Các lệnh](#các-lệnh)
- [Ví dụ `sr emit`](#ví-dụ-sr-emit)
- [Cách đọc output của emit](#cách-đọc-output-của-emit)
- [Danh sách skill](#danh-sách-skill)
- [`sr pull` — OpenAPI → data layer](#sr-pull--openapi--data-layer)
- [`sr fetch` — Confluence / Google Sheet → markdown](#sr-fetch--confluence--google-sheet--markdown)
- [`sr ui` / `sr refresh` — web cục bộ](#sr-ui--sr-refresh--web-cục-bộ)
- [`sr serve` — chạy như MCP server](#sr-serve--chạy-như-mcp-server)
- [`sr apply-base` / `sr ledger` / `sr bootstrap`](#sr-apply-base--sr-ledger--sr-bootstrap)
- [Stack packs & cách detect](#stack-packs--cách-detect)
- [Quy trình khuyến nghị trong 1 project](#quy-trình-khuyến-nghị-trong-1-project)
- [Cấu trúc repo](#cấu-trúc-repo)
- [Upgrade](#upgrade)
- [Thêm stack / skill mới](#thêm-stack--skill-mới)
- [Troubleshooting](#troubleshooting)
- [Docs chi tiết](#docs-chi-tiết)

---

## Ý tưởng cốt lõi

Kiến trúc: **một skill dùng chung + một rule pack cho mỗi stack.**

```
                    ┌─────────────┐
   skill.json  ─────┤             │
 (skill chung)      │ skillrunner │──► marching orders (Rules + Steps)
                    │   emit      │        │
 packs/react.json ──┤             │        └──► Claude đọc & thực thi
 packs/flutter.json─┤             │
        ...         └─────────────┘
                          ▲
                          │ detect: dò file trong project
                    (package.json, pubspec.yaml, go.mod, ...)
```

- Đổi project → **chỉ đổi pack**, skill giữ nguyên.
- Skill viết **một lần**, dùng cho **mọi stack**. Thêm stack mới = thêm **một file** `packs/<stack>.json`.

---

## CLI tất định — không phải Claude "thực thi"

**Điều quan trọng nhất cần nắm:** `sr` là một **CLI Go chạy cục bộ**. Nó chỉ **đọc JSON và IN ra text** — bên trong **không có model nào "hiểu" hay "thực thi" gì cả**. Cùng một input luôn cho **cùng một output**, không gọi LLM, không tốn token. Việc *thực thi* các marching orders đó là **bước sau, do Claude (hoặc bạn) làm** khi đọc output.

```
sr emit build-ui  ──►  (Go thuần, tất định)  ──►  in ra Markdown marching orders
                                                   ▲ KHÔNG do model sinh ra
Claude đọc output đó  ──►  đây mới là bước "thực thi"  (do model làm)
```

Nói cách khác: skillrunner trả lời **"nên làm gì, theo rule nào"**; nó **không** tự suy luận và không tự sửa code nghiệp vụ.

### Trade-off có chủ đích: bỏ portability ⇄ được deterministic + 0-token

skillrunner **cố tình KHÔNG** theo đặc tả Agent Skills (`SKILL.md` + YAML frontmatter mà claude.ai/API tự đọc). Đây là đánh đổi có ý thức:

| Bỏ đi | Nhận lại |
|-------|----------|
| **Portability across surfaces.** Một Agent Skill (`SKILL.md`) chạy y hệt trên claude.ai / API / Claude Code vì nó là *dữ liệu để model tự đọc*. `sr` là *chương trình phải được thực thi* → cần **Bash hoặc MCP** để gọi binary, nên **chỉ chạy trong Claude Code / terminal (hoặc nơi có MCP)** — **KHÔNG chạy trên claude.ai / API**. | **Deterministic** — Go thuần, cùng input → cùng output, không "trôi" theo model. **0-token dispatch** — `list`/`emit` không tốn token LLM; token chỉ phát sinh khi Claude đọc output ở bước sau. **Debug bằng tay** — gõ `sr emit <skill>` ngay trong terminal để thấy *chính xác* Claude sẽ nhận gì. |

> ⚠️ **Đừng kỳ vọng chạy trên claude.ai / API.** skillrunner là công cụ **dòng lệnh** cho môi trường có shell (Claude Code CLI / terminal), hoặc chạy dưới dạng MCP server (xem [`sr serve`](#sr-serve--chạy-như-mcp-server)). Nếu bạn cần một skill chạy đa nền tảng kể cả claude.ai/API, đó là mô hình Agent Skills (`SKILL.md`) — một hướng khác, không phải cái này.

---

## Ba làn 0-token: emit / pull / fetch

Nguyên tắc xuyên suốt: **việc nào máy móc thì đừng cho Claude làm** — đẩy xuống CLI, Claude chỉ vào ở khúc thật sự cần não.

```
   NGUỒN THÔ (đắt token nếu cho Claude đọc trực tiếp)
   ┌──────────────┬───────────────────────┬──────────────────────────┐
   │ convention   │ OpenAPI spec          │ Confluence ADF / Sheet    │
   │ của stack    │ (50–200k token)       │ (HTML/bảng dài)           │
   └──────┬───────┴───────────┬───────────┴────────────┬─────────────┘
          │                   │                        │
     sr emit             sr pull                  sr fetch
     (rule text)    (types.ts + hooks + digest)  (markdown + digest)
          │                   │                        │
          └───────────────────┴────────────────────────┘
                     LÀN 0-TOKEN — không có Claude
                              │
   ═══════════════════════════▼═══════════════════════════
                     CLAUDE — chỉ ăn digest/markdown (~0.5–2k token)
                     build UI · dựng spec · sinh testcase · dò xung đột
```

| Lệnh | Nguồn | Ra cái gì | Claude nhận |
|---|---|---|---|
| `sr emit <skill>` | `skill.json` + `packs/<stack>.json` | marching orders (Markdown) | rule + các bước |
| `sr pull --tag <tag>` | OpenAPI (thường qua **authswagger**) | `types.ts` + `<tag>.hooks.ts` + `index.ts` | digest gọn: endpoint + hook name |
| `sr fetch --from <url>` | Confluence / Google Sheet | `docs/specs/<slug>.md` | digest: title, sections, shape của bảng |

`sr ui` là bộ mặt web của `sr fetch`; `sr serve` là cách gói `emit` lại thành tool MCP. Cả hai **không** thêm làn mới.

---

## Nó ghi những file nào

Điều này quan trọng vì `sr` **có** đụng vào repo đích. Bảng đầy đủ:

| Lệnh | Ghi gì | Ở đâu | Ghi đè? |
|---|---|---|---|
| `emit <skill>` | ledger (nhớ skill đã chạy) | `<repo>/.skillrunner/ledger.json` | append |
| `emit all` | *(không ghi gì)* | — | — |
| `pull` | `types.ts`, `<tag>.hooks.ts`, `index.ts` | `<repo>/src/api/<tag>/` (đổi bằng `--out`) | **có** |
| `pull` | cache digest | `<repo>/docs/context/pull-<tag>.json` | có |
| `fetch` | markdown đã chuẩn hoá | `<repo>/docs/specs/<slug>.md` (đổi bằng `--out`) | **có** |
| `fetch` | cache digest | `<repo>/docs/context/fetch-<slug>.json` | có |
| `ui` / `refresh` | tag + phiên + history | `<repo>/.skillrunner/ui.json` (0600) | có |
| `ui` / `refresh` | **thêm `.skillrunner/` vào `.gitignore`** | `<repo>/.gitignore` | append (idempotent) |
| `apply-base` | file config của stack (eslint/linter/tsconfig…) | gốc repo | **bỏ qua** file đã có, trừ khi `--force` |
| `bootstrap` | managed block hướng dẫn dùng `sr` | `<repo>/CLAUDE.md` | refresh block, không đụng phần còn lại |
| `init` | `skill.json` mẫu | thư mục hiện tại | không ghi đè |
| `serve` (MCP) | như `emit` + `apply_base` ở trên | — | — |

**Không lệnh nào** sửa code nghiệp vụ, chạy `git`, hay gọi mạng ngoài các nguồn bạn chỉ định.

---

## Cài đặt

Yêu cầu: **Go ≥ 1.26** (chỉ cần khi build). `sr pull` cần thêm **Node/npx** (dùng `openapi-typescript`).

```bash
git clone <repo-url> my-plugin-ecosystem
cd my-plugin-ecosystem

make build          # -> bin/skillrunner (native cho máy hiện tại)
```

Cài global để gọi được ở mọi nơi (macOS/Linux, `/usr/local/bin` thường có sẵn trong PATH):

```bash
make install
which skillrunner   # kiểm tra: /usr/local/bin/skillrunner
which sr            # shim ngắn, cài cùng lúc
sr home             # skill.json nào đang được dùng, và nhờ nấc nào
```

`make install` làm **ba** việc chứ không phải một, và thiếu việc nào cũng ra một setup không chạy:

1. cài binary,
2. ghi thư mục repo này vào `~/.skillrunner/home` — nhờ đó `sr` tìm được `skill.json` từ **mọi**
   project khác, không chỉ từ trong repo này,
3. viết shim `sr` cạnh binary.

Cả ba đều chạy lại mỗi lần `make install`, nên không còn bước copy tay nào để quên.

Trên **Windows** thì chạy `make` ngay trong `cmd.exe` được — Makefile tự nhận
`OS=Windows_NT` và dùng recipe cmd-native (không cần sh/grep/awk/sudo), đồng thời
`chcp 65001` để log UTF-8 không bị vỡ ký tự (`—` hiện thành `ΓÇö` nếu còn code page cũ):

```bat
make build          :: -> bin\skillrunner.exe
make install        :: go install -> %USERPROFILE%\go\bin (đã có sẵn trong PATH) + con trỏ + shim
where skillrunner   :: kiểm tra
sr home             :: skill.json nào đang được dùng
```

Không cần copy tay `sr.exe` nữa — `make install` tự viết shim. Trên Windows nó viết **hai** file,
cố ý: `sr.cmd` cho cmd.exe/PowerShell, và `sr` (không đuôi) cho **Git Bash** — Git Bash không đọc
`PATHEXT` nên sẽ không thấy lệnh chỉ có `.cmd`. Đây cũng là lý do `npm`/`bun` ship cả hai.

Build cho nền tảng khác (Intel mac / Linux / Windows):

```bash
make all            # -> bin/skillrunner-darwin-amd64, -linux-amd64, .exe, ...
```

| Nền tảng | File binary |
|----------|-------------|
| macOS Apple Silicon (M1/M2/M3…) | `skillrunner-darwin-arm64` |
| macOS Intel | `skillrunner-darwin-amd64` |
| Linux | `skillrunner-linux-amd64` |
| Windows | `skillrunner.exe` |

---

## Dùng cho mọi project (wrapper `sr`)

⚠️ **Quan trọng:** binary mặc định tìm `skill.json` **trong thư mục hiện tại** (`-f skill.json`). Kho skill lại nằm tập trung ở repo này. Vì vậy để dùng ở project bất kỳ, hãy **trỏ cố định về skill.json trung tâm** bằng một hàm wrapper.

Thêm vào `~/.zshrc` (hoặc `~/.bashrc`):

```zsh
# skillrunner — central skill pool
export SKILLRUNNER_HOME="$HOME/Documents/Personal/my-plugin-ecosystem"
sr() { skillrunner "$@" -f "$SKILLRUNNER_HOME/skill.json" --dir "$PWD"; }
```

```bash
source ~/.zshrc
```

Từ giờ, đứng ở **bất kỳ project nào**:

```bash
cd ~/work/my-react-app
sr detect          # dò stack của project hiện tại
sr list            # xem các skill
sr emit build-ui   # lấy marching orders (tự merge pack theo stack)
```

Wrapper tự động gắn `-f <skill.json trung tâm>` và `--dir <project hiện tại>`.

> **Không dùng wrapper thì sao?** Chạy thẳng `skillrunner emit ...` ngoài repo sẽ báo lỗi "không tìm thấy skill.json". Chỉ lệnh `detect` (chỉ soi file project) là chạy được mà không cần manifest.
>
> ⚠️ Wrapper `sr` là **hàm shell** — tiến trình con **không** thấy nó. Khi cấu hình MCP trong `.mcp.json` phải dùng binary `skillrunner` + đường dẫn tuyệt đối, xem [`sr serve`](#sr-serve--chạy-như-mcp-server).

---

## Các lệnh

```
skillrunner detect                   In stack đã phát hiện cho project
skillrunner status                   Stack + đã cache profile/registry/ledger chưa (+ cảnh báo cache đã cũ)
skillrunner list                     Liệt kê skill (tự merge pack theo stack)
skillrunner emit <skill>             In marching orders cho <skill> (ghi vào ledger)
skillrunner emit all                 In marching orders cho MỌI skill (dump catalog, không ghi ledger)
skillrunner apply-base               Copy file config nền của stack (eslint/linter/…) vào project
skillrunner pull --tag <tag>         Cầu nối authswagger spec -> types.ts + react-query hook + digest (0 token)
skillrunner fetch --from <url>       Cầu nối Confluence/Google Sheet -> markdown + digest (0 token)
skillrunner ui [--port 7777]         Web cục bộ quản lý credential tag/phiên và chạy fetch
skillrunner refresh --session <s>    Fetch lại (bỏ cache) mọi link của một phiên đã lưu
skillrunner ledger                   Xem skill nào đã được emit ở project này
skillrunner validate                 Kiểm tra manifest có hợp lệ không
skillrunner init                     Tạo skill.json mẫu trong thư mục hiện tại
skillrunner bootstrap                Đảm bảo CLAUDE.md của project có hướng dẫn dùng sr
skillrunner serve                    Chạy như MCP server qua stdio (5 tool: detect/list/emit/apply-base/status)
```

Flags dùng chung:

| Flag | Mô tả | Mặc định |
|------|-------|----------|
| `-f, --file <path>` | Đường dẫn manifest | `skill.json` |
| `-p, --pack <stack>` | Ép dùng pack cụ thể (react, flutter…) | auto-detect |
| `--dir <path>` | Thư mục project để dò stack / thao tác | thư mục của manifest |
| `--force` | `bootstrap`: ghi CLAUDE.md kể cả khi global đã cover<br>`apply-base`: ghi đè file đã tồn tại | tắt |

Flags của `pull`:

| Flag | Mô tả | Mặc định |
|------|-------|----------|
| `--from <url\|path>` | URL spec OpenAPI, hoặc đường dẫn file cục bộ | `http://localhost:8080/openapi.json` |
| `--env <name>` | environment của authswagger (`?env=`) | `dev` |
| `--spec <n>` | nhóm spec của authswagger (`?spec=`) | (trống) |
| `--tag <tag>` | **bắt buộc** — lọc operation theo tag OpenAPI | — |
| `--out <dir>` | thư mục output | `src/api/<tag>` |
| `--base <url>` | ghi đè proxyBase | `servers[0].url` của spec |
| `--codegen ts` | bộ sinh type (hiện chỉ `ts`) | `ts` |
| `--no-cache` | bỏ qua digest đã cache, sinh lại | tắt |

Flags của `fetch` (dùng lại `--from` / `--out` / `--no-cache`):

| Flag | Mô tả |
|------|-------|
| `--email <email>` | email Basic-auth của Confluence (mặc định lấy từ config) |
| `--token-env <VAR>` | tên biến môi trường chứa token/secret |
| `--range <A1>` | đọc Google Sheet qua Sheets API v4, vd `Sheet1!A1:H` |
| `--gid <id>` | id tab của Google Sheet cho đường CSV (mặc định lấy `#gid=` trong URL) |
| `--all-tabs` | kéo mọi tab qua Sheets API v4 (batchGet) |

Flags của `ui` / `refresh`:

| Flag | Mô tả | Mặc định |
|------|-------|----------|
| `--port <n>` | cổng localhost cho `sr ui` | `7777` |
| `--session <name>` | tên phiên trong `ui.json` của repo (bắt buộc với `refresh`) | — |

> Skill gắn nhãn **`[needs approval]`** sẽ dừng lại chờ bạn quyết định trước khi đụng file.

---

## Ví dụ `sr emit`

```bash
cd ~/work/my-react-app
sr emit build-ui
```

Output thực tế (rút gọn):

```markdown
# SKILL: build-ui

Build or edit UI using the stack's design system and tokens.

## Goal
UI that reuses shared components and obeys design tokens.

## Rules you MUST follow

**Working-policy**
- When the request is ambiguous, ask via AskUserQuestion before coding — do not guess.
- Minimal change only: stay within the requested scope, no drive-by refactors.
- ...

**Conventions**
- Always `return res` from service calls so backend messages reach the toast.
- Use useConfirm for: delete, icon-only actions, auto-save, exiting a dirty form.
- Avoid stale results: put every query param into the react-query queryKey.
- ...

**Design-system**
- Use the internal design system (@ghm) components: Dialog/Portal, BaseSelect, Field.
- Styling with Tailwind; do not hardcode colors — use design tokens/classes.
- ...

## Steps
1. Reuse existing shared components before creating new ones.
2. Use semantic design tokens — never hardcode colors/spacing.
...

## Expected outputs
- UI files using shared components and tokens
```

Cùng skill đó nhưng ép pack Flutter → phần **Conventions/Design-system đổi theo Flutter**, còn phần skill giữ nguyên:

```bash
sr emit build-ui --pack flutter
```

---

## Cách đọc output của emit

| Phần | Ý nghĩa |
|------|---------|
| `## Goal` | Kết quả cần đạt |
| `## Rules you MUST follow` | **Quan trọng nhất.** Các nhóm rule được merge: rule chung (Working-policy, Project-profile) + rule từ pack (Conventions, Design-system, Lint…). **Bắt buộc tuân theo.** |
| `## Inputs / context to gather first` | Cần thu thập gì trước khi làm |
| `## Steps` | Các bước thực thi tuần tự |
| `## Expected outputs` | Sản phẩm mong đợi |

---

## Danh sách skill

Chạy `sr list` để xem bản mới nhất (kèm mô tả đầy đủ + keyword). Hiện có:

| Skill | Mô tả |
|-------|-------|
| `build-ui` | Dựng/sửa UI theo design system & tokens của stack |
| `check-conflicts` | Đối chiếu testcase ⟷ spec ⟷ ảnh mockup ⟷ rule UI/UX, nêu **xung đột** để người quyết. Read-only `[needs approval]` |
| `check-diff` | Soi git diff hiện tại theo conventions của stack |
| `commit` | Chia thay đổi thành commit theo module, message Conventional Commit + Jira ref |
| `deliver-feature` | Giao 1 feature Jira đầu-cuối: locate → plan → goal → code → check → clean → verify → commit |
| `explain-lib` | Giải thích project THỰC SỰ dùng một thư viện thế nào (không bịa API) |
| `gen-routes` | Sinh/refresh route từ cấu trúc file |
| `learn-project` | Deep-dive project MỘT LẦN, cache kiến trúc vào `docs/project-profile.md` |
| `plan-feature` | Biến yêu cầu thành plan + goals để bạn quyết định `[needs approval]` |
| `refactor` | Cải thiện component/module cho rõ ràng & tái dùng, KHÔNG đổi behavior |
| `scaffold-data` | Dựng tầng data cho một API call theo kiến trúc của stack |
| `scaffold-screen` | Scaffold màn hình/feature mới theo kiến trúc của stack |
| `spec-from-source` | Biến issue Jira/Confluence, Google Sheet, hoặc file/endpoint Swagger-OpenAPI thành spec có cấu trúc `[needs approval]` |
| `update-module-registry` | Tạo/cập nhật `docs/module-registry.md` |

Taxonomy xung đột dùng cho `check-conflicts` nằm ở [`docs/ui-ux-conflicts.md`](docs/ui-ux-conflicts.md) — đây là **nguồn sự thật duy nhất**, cả MCP Studio cũng ground vào file này.

---

## `sr pull` — OpenAPI → data layer

Biến mối nối **spec → code tầng data** (gần như 100% máy móc) thành **một lệnh 0 token**.

```bash
cd ~/work/my-react-app
sr pull --from "http://localhost:8080/openapi.json?env=dev&spec=1" --tag Appointment
```

Sinh ra:

```
src/api/appointment/
├── types.ts              # sinh bằng openapi-typescript (cần npx)
├── appointment.hooks.ts  # khung react-query hook
└── index.ts
docs/context/pull-appointment.json   # cache digest
```

…rồi in ra **digest** (`tag`, `proxyBase`, `endpoints[]`, `files[]`, `rules[]`, `specHash`) — đây mới là thứ Claude đọc, thay vì nuốt cả spec 200k token.

- **Không tự chạy authswagger.** Chỉ health-check; nếu chết thì in lệnh để bạn tự chạy rồi thoát khác 0.
- Cần **Node/npx** trên máy.
- Chi tiết: [`docs/sr-pull-design.md`](docs/sr-pull-design.md).

---

## `sr fetch` — Confluence / Google Sheet → markdown

Cùng ý tưởng, nhưng cho **nội dung prose/bảng** thay vì OpenAPI.

```bash
sr fetch --from "https://acme.atlassian.net/wiki/spaces/KT/pages/2693562387/Quy-trinh"
sr fetch --from "https://docs.google.com/spreadsheets/d/1ZDA.../edit#gid=933348279" --all-tabs
```

Sinh ra `docs/specs/<slug>.md` (markdown sạch) + `docs/context/fetch-<slug>.json` (cache), và in digest gọn (`title`, `sections[]`, `tables[]` chỉ mô tả *shape*, `contentHash`).

Cấu hình secret ở `<repo>/.skillrunner/fetch.json` (đã gitignore) — file này chỉ chứa **tham chiếu** tới secret, không chứa token thô:

```jsonc
{
  "confluence": { "site": "acme.atlassian.net", "email": "ai@do.vn", "tokenEnv": "ATLASSIAN_TOKEN" },
  "google":     { "saKeyFile": "~/.config/gcp/sa.json" },   // hoặc refresh / apiKeyEnv
  "sources":    { "tc-login": "https://docs.google.com/spreadsheets/d/1ZDA.../edit" }
}
```

Thứ tự ưu tiên: **flag > env > config**. Có `sources` thì gọi ngắn: `sr fetch --from tc-login`.

Chi tiết: [`docs/sr-fetch-design.md`](docs/sr-fetch-design.md).

---

## `sr ui` / `sr refresh` — web cục bộ

Bộ mặt web bọc quanh `sr fetch`, để khỏi phải nhớ token và dán link mỗi lần.

```bash
sr ui                 # http://127.0.0.1:7777
```

- **Tag** = một vùng credential Confluence `{site, email, token}`, tái dùng nhiều lần.
- **Session (phiên)** = một tính năng: 1 tag + N link Confluence + N link Sheet.
- **History** = log các lần chạy (100 bản ghi gần nhất).
- Cấu hình auto-ghi vào `<repo>/.skillrunner/ui.json` (**0600**, và **tự thêm `.skillrunner/` vào `.gitignore`**).
- Bind cứng `127.0.0.1`, không đăng nhập; chống CSRF bằng nonce header `X-SR-UI` + kiểm tra Origin localhost.

Chạy lại một phiên không cần mở web:

```bash
cd <repo> && sr refresh --session khoa-kham-benh
```

Hướng dẫn đầy đủ (API, bảo mật, giới hạn, troubleshooting): **[`docs/sr-ui-guide.md`](docs/sr-ui-guide.md)**.
Thiết kế & quyết định đã khoá: [`docs/sr-ui-design.md`](docs/sr-ui-design.md).

---

## `sr serve` — chạy như MCP server

`sr serve` biến skill pool thành **MCP server** để Claude Code gọi như tool native, không cần Bash/PATH.

> **Đính chính:** `sr serve` là **MCP _server_**, không phải host. **Claude Code mới là host** — nó spawn tiến trình và cầm dây; `sr serve` chỉ ngồi chờ trên stdio.

```jsonc
// <repo>/.mcp.json
{
  "mcpServers": {
    "skillrunner": {
      "command": "skillrunner",           // binary trong PATH — KHÔNG dùng shim `sr`
      "args": ["serve"]                   // không cần -f: binary tự resolve (xem `sr home`)
    }
  }
}
```

Năm tool được khai báo: `detect_stack`, `list_skills`, `emit_skill`, `apply_base`, `status`
(tham số chung: `dir`, `pack`; `emit_skill` cần `skill`; `apply_base` nhận `force`).

Ba điều đáng nhớ:

1. **Kết quả giống hệt CLI** — cùng gọi một hàm trong `internal/`. Khác nhau ở trải nghiệm, không ở nội dung.
2. **MCP KHÔNG rẻ hơn về token.** Lợi ích thật là **tool tự hiện ra** (Claude không cần được nhắc) và **chạy được nơi không có shell**.
3. `emit_skill` **ghi ledger**, `apply_base` **ghi file vào repo** — hai tool có side effect.

`pull` / `fetch` / `ui` **không** có đường MCP; muốn dùng thì gọi CLI.

Chi tiết (giao thức, tool schema, so sánh CLI ⇄ MCP, troubleshooting): **[`docs/sr-serve-mcp.md`](docs/sr-serve-mcp.md)**.

---

## `sr status` — stack, cache, và **độ cũ** của cache

```
Stack:   go (go.mod present)
Profile: cached (docs/project-profile.md) — fresh at HEAD, reuse it, do not re-scan source
Registry: STALE (docs/module-registry.md) — 25 commits, 67 source files changed since it was written, plus 12 uncommitted
          reuse for orientation only; confirm files/symbols still exist, or ask the user to re-run `update-module-registry`
Emitted: check-conflicts×5, commit×1 (see `sr ledger`)
```

`docs/project-profile.md` được cache **một lần** rồi tái dùng, và chỉ rebuild khi bạn yêu cầu — nên
nó có thể tụt lại sau source hàng tháng mà không ai biết. `status` đo khoảng cách đó:

- **Không cần marker trong file, không cần state file.** Profile là file **được commit**, nên git đã
  biết nó đổi lần cuối ở commit nào: `git log -1 -- docs/project-profile.md`. Từ mốc đó đếm số
  commit tới `HEAD`, số file nguồn đã đổi (`git diff --name-only`), và số file đang dở
  (`git status --porcelain`). Hoạt động cả với profile do người khác trong team commit.
- **Chỉ file nguồn mới tính.** `docs/`, mọi `*.md` và `.skillrunner/` bị loại — nếu không thì mỗi
  lần `emit` ghi ledger là profile bị coi là cũ, và cảnh báo sẽ thành nhiễu.
- **STALE khi có >0 file nguồn đổi.** Một refactor cũng đủ làm profile nhắc tới symbol không còn tồn tại.
- **Không đo được thì im lặng.** Không phải git repo, git thiếu, hoặc doc chưa từng commit → in đúng
  dòng `cached (…) — reuse it` như trước. Mọi lệnh git có timeout 2s: `status` không bao giờ được treo.

Dòng này giống hệt nhau ở CLI và ở MCP tool `status` (chung một hàm `cacheLine`).

---

## `sr apply-base` / `sr ledger` / `sr bootstrap`

**`sr apply-base`** — copy file config nền của stack (eslint / linter / tsconfig…) từ `packs/assets/` vào project. Mặc định **bỏ qua** file đã tồn tại; `--force` mới ghi đè. In rõ từng file đã copy/bỏ qua.

**`sr ledger`** — xem skill nào đã được emit ở project này (`<repo>/.skillrunner/ledger.json`). Giúp phiên Claude sau biết `learn-project` đã chạy chưa, `deliver-feature` đang tới đâu. `sr status` cũng in một dòng tóm tắt của ledger.

**`sr bootstrap`** — đảm bảo `CLAUDE.md` của project có khối hướng dẫn dùng `sr`. Chạy một lần cho mỗi project: có block rồi thì refresh tại chỗ, `~/.claude/CLAUDE.md` global đã cover thì bỏ qua, còn lại thì append (biết stack). `--force` ghi bản project kể cả khi global đã cover.

---

## Stack packs & cách detect

`skillrunner detect` dò các "chữ ký" trong project theo thứ tự (chữ ký cụ thể trước chữ ký chung):

| Stack (pack) | Tín hiệu nhận diện |
|--------------|--------------------|
| `flutter` | `pubspec.yaml` |
| `rn` | `"react-native"` trong `package.json` |
| `react` | `"react"` trong `package.json` |
| `kotlin` | `build.gradle(.kts)` có tín hiệu Kotlin |
| `java` | build Gradle nhưng **không** thấy tín hiệu Kotlin |
| `swift` | `Package.swift` / `*.xcodeproj` / `*.xcworkspace` |
| `go` | `go.mod` |
| `dotnet` | `*.sln` / `*.csproj` |

Packs có sẵn: `dotnet`, `flutter`, `go`, `java`, `kotlin`, `react`, `rn`, `swift` (+ `packs/assets/` chứa file config nền cho `apply-base`).

Nếu detect ra `""` (không nhận diện được) hoặc thấy cảnh báo `⚠ Stack rule groups not loaded` → chưa có pack cho stack đó. Hãy dùng `--pack` để ép, hoặc tạo `packs/<stack>.json`.

---

## Quy trình khuyến nghị trong 1 project

Với mỗi project mới, làm theo trình tự (khớp với `CLAUDE.md`):

1. **`sr status`** — xem stack + đã cache profile/registry/ledger chưa.
2. Nếu `docs/project-profile.md` **thiếu** → chạy skill **`learn-project`** (deep-dive 1 lần: framework, ngôn ngữ, kiến trúc/layer, flow, config, catalog component tái dùng kèm path).
   - Lần sau profile **đã có** → **tái dùng**, không quét lại toàn bộ source. Chỉ rebuild khi bạn **yêu cầu rõ ràng**.
   - Profile bị đánh dấu **`STALE`** → vẫn tái dùng để định hướng, nhưng **xác minh file/symbol còn tồn tại** trước khi tin; đề nghị người dùng chạy lại `learn-project`, **không tự refresh**.
3. Khi có **mã task Jira + mô tả** → chạy skill **`deliver-feature`**:
   1. Đọc `CLAUDE.md` + `docs/module-registry.md` để định vị module/file/route.
   2. Ra **plan + goal đo lường được** → trình bày và **DỪNG** chờ duyệt trước khi code.
   3. Code theo conventions & design system (ưu tiên tái dùng).
   4. `check-diff` + chạy linter; refactor phần đã đụng (không đổi behavior).
   5. Cập nhật `docs/module-registry.md`.
   6. **Verify** đầu-cuối.
   7. Đề xuất commit message dạng text: `ref-<jira>: <type> - <desc>` (≤150 ký tự).
   8. Commit **chỉ sau khi bạn xác nhận**. **Không push** — bạn tự push.

Có spec/testcase trên Confluence hay Google Sheet? Kéo về bằng `sr fetch` / `sr ui` **trước**, rồi mới cho Claude đọc `docs/specs/` — đừng để Claude đọc thẳng HTML/ADF thô.

---

## Cấu trúc repo

```
my-plugin-ecosystem/
├── bin/skillrunner              # binary sau khi build
├── cmd/skillrunner/
│   ├── main.go                  # CLI: parse flag + dispatch lệnh
│   ├── mcp.go                   # chế độ MCP server (`sr serve`), stdlib JSON-RPC
│   ├── bootstrap.go             # `sr bootstrap`
│   └── starter.go               # `sr init`
├── internal/skill/              # detect, manifest, pack, emit, ledger, pull, fetch,
│                                #   confluence, gsheet (+v4), google auth, codegen, jsonc
├── internal/uiserver/           # `sr ui`: HTTP server + SPA nhúng + session runner
│   └── assets/index.html
├── internal/uistore/            # đọc/ghi <repo>/.skillrunner/ui.json (0600, auto-gitignore)
├── skill.json                   # skill dùng chung (sửa 1 lần)  ← EDIT
├── packs/                       # rule theo stack (mỗi stack 1 file)  ← EDIT
│   ├── react.json  flutter.json  go.json  java.json  kotlin.json  rn.json  swift.json  dotnet.json
│   └── assets/                  # file config nền cho `apply-base`
├── docs/
│   ├── skill-runner-design.md   # thiết kế hệ thống
│   ├── skill-taxonomy.md        # kho skill hợp nhất
│   ├── sr-manifest-resolution-design.md  # thang resolve skill.json + `sr home` + shim
│   ├── sr-pull-design.md        # `sr pull`  (OpenAPI → data layer)
│   ├── sr-fetch-design.md       # `sr fetch` (Confluence/Sheet → markdown)
│   ├── sr-ui-design.md          # `sr ui` — thiết kế & quyết định đã khoá
│   ├── sr-ui-guide.md           # `sr ui` / `sr refresh` — hướng dẫn dùng
│   ├── sr-serve-mcp.md          # `sr serve` — chế độ MCP server
│   ├── mcp-architecture.md      # thiết kế 3 MCP (HOÃN — giữ làm phương án)
│   ├── ui-ux-conflicts.md       # taxonomy xung đột (nguồn sự thật cho check-conflicts)
│   ├── project-profile.md       # (cache/ project) hồ sơ project — learn-project
│   └── module-registry.md       # (cache/ project) bản đồ module ↔ file ↔ route
├── .mcp.json                    # đăng ký `sr serve` với Claude Code
├── Makefile
└── CLAUDE.md                    # chỉ dẫn cho Claude khi làm trong repo này
```

> `skill.json` và `packs/*.json` cho phép comment `//`.

---

## Upgrade

```bash
cd "$SKILLRUNNER_HOME"
git pull
make build
sudo cp bin/skillrunner /usr/local/bin/skillrunner
```

> **Chỉ sửa `skill.json` hoặc `packs/*.json` (không đụng code Go) → KHÔNG cần build lại.** Binary đọc các file JSON này lúc runtime, nên `sr` dùng bản mới ngay lập tức. Điều này đúng cả với MCP server: nó đọc lại manifest mỗi lần gọi tool, **không cần restart**. Ngược lại, sửa **code Go** thì phải build lại *và* khởi động lại Claude Code (để respawn tiến trình MCP).

Sau khi sửa manifest, nên kiểm tra:

```bash
skillrunner validate -f "$SKILLRUNNER_HOME/skill.json"
```

---

## Thêm stack / skill mới

**Thêm stack mới** (ví dụ `vue`):
1. Tạo `packs/vue.json` (theo mẫu các pack có sẵn — architecture / conventions / lint / design-system…).
2. (Tùy chọn) thêm chữ ký detect cho stack đó trong `internal/skill/detect.go` rồi build lại.
3. Dùng ngay: `sr emit build-ui --pack vue`.

**Thêm skill mới**: thêm một entry vào `skill.json` — tự động dùng được cho **mọi** stack.

---

## Troubleshooting

| Triệu chứng | Nguyên nhân & cách xử lý |
|-------------|--------------------------|
| `cp: bin/skillrunner: No such file or directory` | Bạn đang ở thư mục khác. Dùng đường dẫn tuyệt đối hoặc `cd "$SKILLRUNNER_HOME"` rồi build trước. |
| `emit`/`list` báo không thấy `skill.json` | Chạy thẳng `skillrunner` ngoài repo. Dùng wrapper `sr` (đã gắn `-f`). |
| `No known stack detected` | Đang đứng ở thư mục không phải project (vd `~`), hoặc stack chưa được hỗ trợ. `cd` vào project hoặc dùng `--pack`. |
| `⚠ Stack rule groups not loaded` | Chưa có `packs/<stack>.json`. Tạo pack hoặc ép `--pack`. |
| `.exe` không chạy trên macOS | `.exe` là binary Windows. macOS dùng `skillrunner` / `skillrunner-darwin-arm64`. |
| `sr serve` gõ tay thì **đứng im** | Đúng hành vi — nó chờ JSON-RPC trên stdin. Nó được Claude Code spawn, không gõ tay. |
| Claude Code không thấy tool MCP | `.mcp.json` phải trỏ **binary** `skillrunner` (không phải hàm `sr`) + `-f` đường dẫn **tuyệt đối**. Restart Claude Code, kiểm tra `/mcp`. |
| `sr pull` báo lỗi `openapi-typescript failed` | Thiếu Node/npx trên PATH, hoặc spec không hợp lệ. |
| `sr pull` không kết nối được spec | authswagger chưa chạy: `cd ../WorkFlowAutomation && go run .` |
| `sr ui` bấm gì cũng **403** | Nonce cũ sau khi restart `sr ui`. **Reload trang.** |
| `sr ui` báo `missing ?dir=` | Chưa chọn repo trong UI. |
| `sr fetch` Confluence 401 | Token Atlassian sai/hết hạn, hoặc thiếu `--email` / `confluence.email` trong `.skillrunner/fetch.json`. |
| `sr fetch` Google Sheet 403 | Sheet chưa link-shared, hoặc cần cấu hình `google` (SA key / refresh token) cho Sheets API v4. |

---

## Docs chi tiết

| File | Nội dung |
|---|---|
| [`docs/skill-runner-design.md`](docs/skill-runner-design.md) | Thiết kế hệ thống |
| [`docs/skill-taxonomy.md`](docs/skill-taxonomy.md) | Kho skill hợp nhất |
| [`docs/skill-flow.md`](docs/skill-flow.md) | **Sơ đồ tương tác: skill nối nhau qua rule + file, ba chuỗi thực tế** |
| [`docs/sr-manifest-resolution-design.md`](docs/sr-manifest-resolution-design.md) | **Thang resolve `skill.json` — chạy được từ mọi project, mọi OS; `sr home`, shim** |
| [`docs/sr-pull-design.md`](docs/sr-pull-design.md) | `sr pull` — OpenAPI → data layer |
| [`docs/sr-fetch-design.md`](docs/sr-fetch-design.md) | `sr fetch` — Confluence/Sheet → markdown |
| [`docs/sr-ui-design.md`](docs/sr-ui-design.md) | `sr ui` — thiết kế, quyết định đã khoá |
| [`docs/sr-ui-guide.md`](docs/sr-ui-guide.md) | **`sr ui` / `sr refresh` — hướng dẫn dùng, API, bảo mật** |
| [`docs/sr-serve-mcp.md`](docs/sr-serve-mcp.md) | **`sr serve` — MCP server: chạy sao, tool gì, tác dụng gì** |
| [`docs/mcp-architecture.md`](docs/mcp-architecture.md) | Thiết kế 3 MCP — **đang hoãn**, giữ làm phương án |
| [`docs/ui-ux-conflicts.md`](docs/ui-ux-conflicts.md) | Taxonomy xung đột (nguồn sự thật cho `check-conflicts`) |
| [`CLAUDE.md`](CLAUDE.md) | Chỉ dẫn cho Claude khi làm việc trong repo |
