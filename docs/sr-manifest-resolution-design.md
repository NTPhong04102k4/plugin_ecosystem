# Thiết kế: định vị `skill.json` trung tâm — thang resolve đa dự án, đa OS

> Trạng thái: **ĐÃ TRIỂN KHAI** — code + test đã vào, `make test` xanh trên Windows.
> Ngày chốt & triển khai: 2026-08-11.

## 1. Mục tiêu & vấn đề

`skillrunner` bán ra một lời hứa duy nhất: chạy từ **bất kỳ project nào**, in marching orders cho
Claude. Lời hứa đó **đang hỏng ở 3/4 lối vào**, trong khi mọi `CLAUDE.md` lại khẳng định nó chạy.

Đo trên máy Windows này (2026-08-11, trước khi sửa):

| Lối vào | Kết quả |
|---|---|
| `sr <cmd>` — bất kỳ đâu | ❌ `sr` không tồn tại (`which sr` fail; chỉ có `skillrunner.exe`) |
| `skillrunner <cmd>` — từ `my-plugin-ecosystem` | ✅ chạy |
| `skillrunner <cmd>` — từ repo khác | ❌ `error: read manifest "skill.json"` |
| MCP `mcp__skillrunner__*` | ❌ `open /Users/ghmsoft/...` (đường dẫn macOS của máy cũ) |

**Nguyên nhân gốc là MỘT, không phải bốn.** `main.go` đặt default của `-f` là chuỗi tương đối
`"skill.json"`, tức ngầm giả định tiến trình được khởi động **bên trong** repo pool. Không có cơ
chế nào ghim manifest trung tâm. `bootstrap.go` ghi rõ thiết kế đã giả định
*"the shell wrapper already pins the central skill.json"* — nhưng wrapper đó chưa từng được tạo
trên Windows, và `.mcp.json` vẫn giữ nguyên đường dẫn macOS của máy cũ.

**Hệ quả thật, không phải lý thuyết.** Managed block trong `my-plugin-ecosystem/CLAUDE.md` và
`WorkFlowAutomation/CLAUDE.md` bảo Claude chạy `sr status` / `sr list` / `sr emit` — những lệnh
**không tồn tại**. Mọi phiên Claude ở các repo đó nhận chỉ thị không thực thi được rồi lặng lẽ tự
improvise, đúng cái mà `sr` sinh ra để ngăn.

**Vì sao sống sót lâu.** `make test` = `go vet ./... && go test ./...`, luôn chạy với cwd = repo
gốc — nơi **luôn có `skill.json`**. Toàn bộ test suite đi qua đường hạnh phúc. Contract "chạy từ
project khác" chưa từng được test một lần. Đây là bài học đắt hơn cả bug: **test không bao giờ
đứng ở vị trí của consumer thì không bảo vệ được contract bán cho consumer.**

## 2. Vị trí trong kiến trúc

```
                        ┌──────────────────────────────┐
   sr <cmd>  ──────────►│  main: resolveManifestPath   │  1 lần, TRƯỚC dispatch
   skillrunner <cmd> ──►│  (cmd/skillrunner)           │
                        └──────────────┬───────────────┘
                                       │ file (tuyệt đối)
                        ┌──────────────┴───────────────┐
                        ▼                              ▼
                 loadWithPack                   runMCPServer
                 (CLI: emit/list/…)             (MCP: serve)
                        │                              │
                        └──────────► internal/skill ◄──┘
                                   (KHÔNG đổi 1 dòng)
```

Điểm mấu chốt: resolve **một lần trong `main`, trước `switch cmd`**, nên CLI và MCP server nhận
cùng một đường dẫn **theo cấu trúc**, không phải nhờ trùng hợp. `runMCPServer(file, …)` vốn đã nhận
`file` từ đúng flag đó, nên `mcp.go` không cần sửa.

## 3. Quyết định đã khoá

| # | Quyết định | Chốt |
|---|---|---|
| 1 | Cơ chế | **Thang resolve cố định** (ladder), không phải search ngược cây thư mục |
| 2 | Tầng sửa | **`cmd/skillrunner`**, tuyệt đối không đụng `internal/skill` |
| 3 | Ưu tiên `./skill.json` | **Trên** env và pointer — giữ tính năng pool-per-project đã tài liệu hoá |
| 4 | Lưu vị trí pool | **File con trỏ `~/.skillrunner/home`** (1 dòng text) + env `SKILLRUNNER_HOME` |
| 5 | Tên ngắn `sr` | **Wrapper script**, sinh bởi `sr home --shims`, viết bằng Go |
| 6 | Windows | **Hai** shim: `sr.cmd` + `sr` không đuôi (Git Bash bỏ qua `PATHEXT`) |
| 7 | `.mcp.json` | **Bỏ hẳn `-f`** — cùng một file chạy được trên Windows lẫn macOS |
| 8 | ldflags nhúng path | **LOẠI** — `make all` cross-compile 4 target, không thể nhúng 1 path hợp lệ cho mọi OS |

## 4. Thang resolve

`resolveManifestPath(explicit, cwd) (path, via string, err error)` —
`cmd/skillrunner/manifestpath.go`.

| Nấc | Nguồn | Ghi chú |
|---|---|---|
| 1 | `-f` / `--file` | Tường minh. **Không bao giờ đoán lại**: path sai ⇒ lỗi, không lặng lẽ dùng pool khác |
| 2 | `<cwd>/skill.json` | Project mang pool riêng — layout đã tài liệu hoá ở đầu `main.go` |
| 3 | `$SKILLRUNNER_HOME/skill.json` | Quy ước đường dẫn tuyệt đối |
| 4 | `~/.skillrunner/home` | File con trỏ, ghi bởi `make install` |
| — | lỗi | Nêu tên **cả 4 nấc** + 3 cách sửa |

Ba tính chất được cố ý thiết kế:

**Thang, không phải search.** Duyệt ngược cây thư mục tìm `skill.json` đã bị loại: kết quả sẽ phụ
thuộc vào chỗ bạn đang đứng, phá vỡ đảm bảo *"same input ⇒ same output"* mà cả công cụ dựa vào.
Thang thì cùng env + filesystem luôn ra cùng đáp án, và mỗi nấc kiểm chứng được riêng.

**Nấc 1 không được đoán lại.** Nếu `-f` trỏ vào file không tồn tại mà lại rơi xuống nấc dưới,
Claude sẽ nhận **rules của pool khác** mà không ai biết. Sai lặng lẽ tệ hơn fail.

**Nấc 3 và 4 rơi xuyên (fall through).** Env hoặc con trỏ trỏ vào pool đã bị di chuyển/xoá thì đi
tiếp nấc dưới, không dead-end. Một cấu hình cũ không được phép làm chết cả thang.

**Vì sao nấc 2 trên nấc 3/4:** giữ nguyên tính năng đã ghi ở đầu `main.go` —
*"Drop the binary + skill.json + packs/ into any project and the same verbs work"* — và
backward-compatible tuyệt đối: chạy trong repo pool, hành vi y hệt trước khi sửa.

**`init` đứng ngoài thang:** nó *ghi* manifest chứ không *đọc*, nên giữ default `skill.json` tương đối.

## 5. Shim `sr` — vì sao Windows cần hai file

Đo trên máy này: `npm`, `npx`, `bun`, `bunx` **đều** ship ba file (script không đuôi + `.cmd` +
`.ps1`). Và `which bun` trả về `/d/AppData/nodejs/bun` — **script sh, không phải `bun.cmd`**.

Lý do: cmd.exe và PowerShell tra `PATHEXT` nên thấy `.cmd`; **Git Bash không tra `PATHEXT`**, nó
tìm file không đuôi. Chỉ ship `.cmd` thì `sr` chết trong Git Bash — đúng shell mà phần lớn tooling
của workspace này chạy.

| OS | File | Nội dung |
|---|---|---|
| Windows | `sr.cmd` | `@"<abs>\skillrunner.exe" %*` |
| Windows | `sr` | `#!/bin/sh` + `exec "<abs>/skillrunner.exe" "$@"` — **forward slash** |
| POSIX | `sr` (0755) | `#!/bin/sh` + `exec "<abs>/skillrunner" "$@"` |

Hai chi tiết dễ sai:
- Bản sh trên Windows phải đổi `\` → `/` (`filepath.ToSlash`): shell POSIX coi `\` là ký tự escape.
- Gọi binary bằng **đường dẫn tuyệt đối**, không bằng tên. Shim được sinh tại chỗ, trên chính máy sẽ
  chạy nó, nên không có rủi ro cross-compile — đổi lại nó không thể bị một `skillrunner` khác đứng
  trước trên PATH cướp mất.

Shim **không** ghim `-f`: thang resolve trong binary đã lo. Nhờ vậy shim chỉ là bí danh tên, và MCP
(gọi thẳng binary, không qua shim) vẫn được vá bởi cùng một cơ chế.

## 6. `sr home`

Một lệnh, ba chế độ — gộp có chủ đích để không nở scope:

| Gọi | Làm gì |
|---|---|
| `sr home` | In manifest đã resolve + nấc nào thắng + trạng thái từng nấc |
| `sr home --set <dir>` | Ghi `~/.skillrunner/home` |
| `sr home --shims` | Ghi shim `sr` cạnh executable đang chạy (`os.Executable()`) |

Hai ràng buộc:

- **`home` resolve lười và không `fatal`.** Đây là lệnh dùng để chẩn đoán khi mọi thứ khác đã chết;
  nếu nó cũng chết vì cùng lý do thì vô dụng.
- **`--set` từ chối thư mục không có `skill.json`.** Ghi một con trỏ "trông như đã cấu hình" nhưng
  resolve ra rỗng chính là kiểu hỏng mà cả thiết kế này sinh ra để chấm dứt.

`--shims` viết bằng **Go, không phải Makefile**: escaping `#!/bin/sh` và `>` trong cmd.exe đúng
loại rắc rối mà `CLAUDE.md` cảnh báo. Một cài đặt chạy cho mọi OS, unit-test được.

## 7. `make install` — ba bước, không phải một

```
1. cài binary            (go install / cp)
2. sr home --set <repo>  ghi con trỏ
3. sr home --shims       ghi shim
```

Binary một mình **không** phải một setup chạy được: thiếu con trỏ thì không tìm được `skill.json`
từ project khác; thiếu shim thì `sr` không tồn tại. Cả ba chạy lại mỗi lần install, diệt luôn cái
bẫy mà `CLAUDE.md` từng tự thừa nhận (*"re-copy `sr.exe` if you rely on the short name"*).

**`home --set` ghi vào `$HOME` nên KHÔNG được chạy dưới `sudo`** — sẽ ghi vào home của root, nơi
không lệnh thường nào tìm tới. Nhánh POSIX của Makefile để bước này ngoài khối sudo.

## 8. Xử lý lỗi

Khi không nấc nào resolve, thông báo lỗi là thứ **duy nhất** người dùng có, nên nó nêu cả 4 nấc
kèm trạng thái hiện tại, rồi 3 cách sửa. Ba điểm đã học được khi làm:

- Dùng `%s` chứ **không** `%q` cho đường dẫn: trên Windows `%q` escape mọi separator
  (`C:\\Users\\...`), khó đọc và khó copy.
- Gợi ý phải đúng thứ tự `sr <command> -f <path>`. Bản đầu viết `sr -f <path> <command>` — **không
  parse được**, vì subcommand là `os.Args[1]` còn flag parse từ `os.Args[2:]`. Đã có test chặn.

## 9. Test

`manifestpath_test.go` (8) + `home_test.go` (4) — 12 case. Quan trọng nhất:

| Test | Chốt điều gì |
|---|---|
| `TestResolveFromForeignDirSucceeds` | **Regression cho đúng bug**: chạy từ thư mục không có `skill.json` |
| `TestResolveExplicitFlagIsNotSecondGuessed` | Nấc 1 sai path ⇒ lỗi, không rơi xuống |
| `TestResolveCwdBeatsEnvAndPointer` | Thứ tự 2 > 3 > 4 |
| `TestResolveEnvWithoutManifestFallsThrough` | Nấc cũ không dead-end |
| `TestResolveErrorNamesEveryRung` | Lỗi nêu đủ 4 nấc + đúng thứ tự flag |
| `TestShimFilesPerPlatform` | Windows ra 2 file; bản sh không chứa `\` |
| `TestWritePointerRejectsDirWithoutManifest` | Không ghi con trỏ rỗng nghĩa |

`resolveManifestPath` nhận `cwd` làm **tham số** thay vì đọc cwd tiến trình — test được mà không
cần `t.Chdir`, và không có test nào phụ thuộc thứ tự chạy.

Quy ước cross-platform (theo `CLAUDE.md`): không assert Unix permission bits; `t.Setenv` cả `HOME`
lẫn `USERPROFILE` vì `os.UserHomeDir()` đọc biến khác nhau theo OS.

**Không thêm `make smoke`:** kịch bản hỏng test được ngay trong `go test`, giữ `make test` là cổng
kiểm duy nhất — đúng triết lý repo.

## 10. Phương án đã loại

| Phương án | Vì sao loại |
|---|---|
| Nhúng path bằng `-ldflags -X` | `make all` cross-compile darwin/linux/windows — không thể nhúng 1 path hợp lệ cho mọi target. Ràng buộc đa-OS giết phương án này |
| Search ngược cây thư mục | Kết quả phụ thuộc cwd ⇒ phá determinism, hard constraint của repo |
| Sửa trong `internal/skill.Load` | gitnexus impact: `Load` HIGH risk, và `main` còn gọi thẳng nó (`validate`). Nhét logic "đi tìm manifest" vào tầng thư viện thuần *"reads JSON and formats text"* là sai tầng |
| `sr` là binary thứ hai | Tốn thêm ~13MB; wrapper đủ dùng vì thang resolve đã nằm trong binary |
| Symlink POSIX + copy Windows | Symlink trên Windows cần quyền admin/Developer Mode ⇒ có thể fail âm thầm; hai nhánh hành vi khác nhau khó test đồng nhất |

## 11. Ảnh hưởng & giới hạn

**Không đổi:** `internal/skill` (0 dòng), `mcp.go`, `bootstrap.go`. Chữ ký `Load` và `loadWithPack`
nguyên vẹn — đây là mục tiêu rút ra từ gitnexus impact (cả hai đều **HIGH**, fan-out tới cả `main`
lẫn `runTool`).

**Managed block không đổi** — nó vốn đã dùng `sr` cho mọi verb và giới thiệu `sr` (aka
`skillrunner`). Lần đầu tiên nó **nói đúng sự thật**.

**Giới hạn của gitnexus ở việc này:** index chỉ có `plugin_ecosystem`. `.mcp.json`, `Makefile`,
`CLAUDE.md` không có node nào trong graph — phần đó phải verify bằng chạy thật từ nhiều repo và
nhiều shell, không trông vào `detect_changes`.

**Cần restart Claude Code** để nạp `.mcp.json` mới trước khi MCP path được xác nhận.
