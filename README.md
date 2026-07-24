# skillrunner

**Kho skill trung tâm cho Claude Code** — một Go binary (`skillrunner`) đọc `skill.json` (skill dùng chung) + `packs/<stack>.json` (rule theo stack), phát hiện stack của project rồi **in ra "marching orders" (bộ rule + các bước) để Claude thực thi**.

> `skillrunner` **không tự suy luận, không sửa code**. Nó chỉ dò stack và in hướng dẫn. Việc đọc và làm theo là do **Claude** (hoặc bạn) thực hiện.

---

## Mục lục
- [Ý tưởng cốt lõi](#ý-tưởng-cốt-lõi)
- [CLI tất định — không phải Claude "thực thi"](#cli-tất-định--không-phải-claude-thực-thi)
- [Cài đặt](#cài-đặt)
- [Dùng cho mọi project (wrapper `sr`)](#dùng-cho-mọi-project-wrapper-sr)
- [Các lệnh](#các-lệnh)
- [Ví dụ `sr emit`](#ví-dụ-sr-emit)
- [Cách đọc output của emit](#cách-đọc-output-của-emit)
- [Danh sách skill](#danh-sách-skill)
- [Stack packs & cách detect](#stack-packs--cách-detect)
- [Quy trình khuyến nghị trong 1 project](#quy-trình-khuyến-nghị-trong-1-project)
- [Cấu trúc repo](#cấu-trúc-repo)
- [Upgrade](#upgrade)
- [Thêm stack / skill mới](#thêm-stack--skill-mới)
- [Troubleshooting](#troubleshooting)

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

Nói cách khác: skillrunner trả lời **"nên làm gì, theo rule nào"**; nó **không** tự làm. Không sửa file, không suy luận, không chạy lệnh của project.

### Trade-off có chủ đích: bỏ portability ⇄ được deterministic + 0-token

skillrunner **cố tình KHÔNG** theo đặc tả Agent Skills (`SKILL.md` + YAML frontmatter mà claude.ai/API tự đọc). Đây là đánh đổi có ý thức:

| Bỏ đi | Nhận lại |
|-------|----------|
| **Portability across surfaces.** Một Agent Skill (`SKILL.md`) chạy y hệt trên claude.ai / API / Claude Code vì nó là *dữ liệu để model tự đọc*. `sr` là *chương trình phải được thực thi* → cần **Bash hoặc MCP** để gọi binary, nên **chỉ chạy trong Claude Code / terminal (hoặc nơi có MCP)** — **KHÔNG chạy trên claude.ai / API**. | **Deterministic** — Go thuần, cùng input → cùng output, không "trôi" theo model. **0-token dispatch** — `list`/`emit` không tốn token LLM; token chỉ phát sinh khi Claude đọc output ở bước sau. **Debug bằng tay** — gõ `sr emit <skill>` ngay trong terminal để thấy *chính xác* Claude sẽ nhận gì. |

> ⚠️ **Đừng kỳ vọng chạy trên claude.ai / API.** skillrunner là công cụ **dòng lệnh** cho môi trường có shell (Claude Code CLI / terminal), hoặc chạy dưới dạng MCP server. Nếu bạn cần một skill chạy đa nền tảng kể cả claude.ai/API, đó là mô hình Agent Skills (`SKILL.md`) — một hướng khác, không phải cái này.

---

## Cài đặt

Yêu cầu: **Go ≥ 1.26** (chỉ cần khi build).

```bash
git clone <repo-url> my-plugin-ecosystem
cd my-plugin-ecosystem

make build          # -> bin/skillrunner (native cho máy hiện tại)
```

Cài global để gọi được ở mọi nơi (macOS/Linux, `/usr/local/bin` thường có sẵn trong PATH):

```bash
sudo cp bin/skillrunner /usr/local/bin/skillrunner
which skillrunner   # kiểm tra: /usr/local/bin/skillrunner
```

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

---

## Các lệnh

```
skillrunner detect            In stack đã phát hiện cho project
skillrunner status            Stack + đã cache project-profile / module-registry chưa
skillrunner list              Liệt kê skill (tự merge pack theo stack)
skillrunner emit <skill>      In marching orders cho <skill> (để Claude thực thi)
skillrunner validate          Kiểm tra manifest có hợp lệ không
skillrunner init              Tạo skill.json mẫu trong thư mục hiện tại
```

Flags:

| Flag | Mô tả | Mặc định |
|------|-------|----------|
| `-f, --file <path>` | Đường dẫn manifest | `skill.json` |
| `-p, --pack <stack>` | Ép dùng pack cụ thể (react, flutter…) | auto-detect |
| `--dir <path>` | Thư mục project để dò stack | thư mục của manifest |

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

Chạy `sr list` để xem bản mới nhất. Hiện có:

| Skill | Mô tả |
|-------|-------|
| `build-ui` | Dựng/sửa UI theo design system & tokens của stack |
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
| `spec-from-source` | Biến issue Jira/Confluence hoặc file Swagger/OpenAPI thành spec có cấu trúc `[needs approval]` |
| `update-module-registry` | Tạo/cập nhật `docs/module-registry.md` |

---

## Stack packs & cách detect

`skillrunner detect` dò các "chữ ký" trong project theo thứ tự (chữ ký cụ thể trước chữ ký chung):

| Stack (pack) | Tín hiệu nhận diện |
|--------------|--------------------|
| `flutter` | `pubspec.yaml` |
| `rn` | `"react-native"` trong `package.json` |
| `react` | `"react"` trong `package.json` |
| `kotlin` | `build.gradle` / `settings.gradle(.kts)` |
| `swift` | `Package.swift` / `*.xcodeproj` / `*.xcworkspace` |
| `go` | `go.mod` |
| `dotnet` | `*.sln` / `*.csproj` |

Packs có sẵn: `dotnet`, `flutter`, `go`, `kotlin`, `react`, `rn`, `swift`.

Nếu detect ra `""` (không nhận diện được) hoặc thấy cảnh báo `⚠ Stack rule groups not loaded` → chưa có pack cho stack đó. Hãy dùng `--pack` để ép, hoặc tạo `packs/<stack>.json`.

---

## Quy trình khuyến nghị trong 1 project

Với mỗi project mới, làm theo trình tự (khớp với `CLAUDE.md`):

1. **`sr status`** — xem stack + đã cache profile/registry chưa.
2. Nếu `docs/project-profile.md` **thiếu** → chạy skill **`learn-project`** (deep-dive 1 lần: framework, ngôn ngữ, kiến trúc/layer, flow, config, catalog component tái dùng kèm path).
   - Lần sau profile **đã có** → **tái dùng**, không quét lại toàn bộ source. Chỉ rebuild khi bạn **yêu cầu rõ ràng**.
3. Khi có **mã task Jira + mô tả** → chạy skill **`deliver-feature`**:
   1. Đọc `CLAUDE.md` + `docs/module-registry.md` để định vị module/file/route.
   2. Ra **plan + goal đo lường được** → trình bày và **DỪNG** chờ duyệt trước khi code.
   3. Code theo conventions & design system (ưu tiên tái dùng).
   4. `check-diff` + chạy linter; refactor phần đã đụng (không đổi behavior).
   5. Cập nhật `docs/module-registry.md`.
   6. **Verify** đầu-cuối.
   7. Đề xuất commit message dạng text: `ref-<jira>: <type> - <desc>` (≤150 ký tự).
   8. Commit **chỉ sau khi bạn xác nhận**. **Không push** — bạn tự push.

---

## Cấu trúc repo

```
my-plugin-ecosystem/
├── bin/skillrunner              # binary sau khi build
├── cmd/skillrunner/             # entrypoint (main.go, starter.go)
├── internal/skill/              # detect, manifest, pack, jsonc loader
├── skill.json                   # skill dùng chung (sửa 1 lần)  ← EDIT
├── packs/                       # rule theo stack (mỗi stack 1 file)  ← EDIT
│   ├── react.json  flutter.json  go.json ...
├── docs/
│   ├── skill-runner-design.md   # thiết kế
│   ├── skill-taxonomy.md        # kho skill hợp nhất
│   ├── project-profile.md       # (cache/ project) hồ sơ project — learn-project
│   └── module-registry.md       # (cache/ project) bản đồ module ↔ file ↔ route
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

> **Chỉ sửa `skill.json` hoặc `packs/*.json` (không đụng code Go) → KHÔNG cần build lại.** Binary đọc các file JSON này lúc runtime, nên `sr` dùng bản mới ngay lập tức.

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

---

## Docs chi tiết
- `docs/skill-runner-design.md` — thiết kế hệ thống
- `docs/skill-taxonomy.md` — kho skill hợp nhất
- `CLAUDE.md` — chỉ dẫn cho Claude khi làm việc trong repo
