# Sơ đồ tương tác — skill nối với nhau như thế nào

> Trạng thái: **THAM CHIẾU (living doc)** — cập nhật khi thêm/sửa skill hoặc rule group.
> Nguồn sự thật vẫn là `skill.json` + `packs/<stack>.json`; doc này chỉ **đọc chúng ra thành hình**.

## 0. Một câu

**Các skill không gọi nhau.** Chúng nối với nhau qua đúng **hai cơ chế**:

| # | Cơ chế | Khi nào xảy ra | Nhìn thấy ở đâu |
|---|---|---|---|
| **1** | **Rule groups dùng chung** — `appliesRules` được merge lúc emit | *trong* một lần `sr emit` | §2 ma trận |
| **2** | **File cache trong repo** — skill A ghi, skill B đọc ở phiên sau | *giữa* các lần chạy | §3 bản đồ file |

Ngoại lệ duy nhất: **`deliver-feature`** — skill duy nhất gọi tên skill khác trong `instructions`.
13 skill còn lại là **lá**, không biết nhau tồn tại; **người dùng (hoặc Claude) mới là bên nối chuỗi**,
dựa vào `description` + `Keywords`.

---

## 1. Ba tầng

```
┌─ TẦNG 3 — ORCHESTRATION ─────────────────────────────────────────────┐
│  deliver-feature   (skill DUY NHẤT gọi skill khác)                    │
│     └─ learn-project · check-diff · commit                            │
└───────────────────────────┬──────────────────────────────────────────┘
                            │
┌─ TẦNG 2 — 13 SKILL LÁ ────▼──────────────────────────────────────────┐
│  QUYẾT ĐỊNH ⏸        HIỂU                DỰNG              KIỂM       │
│  plan-feature        learn-project       scaffold-data     check-diff │
│  spec-from-source    explain-lib         scaffold-screen   refactor   │
│  check-conflicts     update-module-reg   build-ui          commit     │
│                                          gen-routes                   │
└───────────────────────────┬──────────────────────────────────────────┘
                            │ appliesRules  (merge lúc emit)
┌─ TẦNG 1 — RULES ──────────▼──────────────────────────────────────────┐
│  CHUNG (skill.json)          THEO STACK (packs/<stack>.json)          │
│  working-policy              architecture   templates                 │
│  technical                   conventions    design-system             │
│  commit-format               lint           library-docs              │
│  module-registry                                                      │
│  project-profile                                                      │
└──────────────────────────────────────────────────────────────────────┘
        ▲ đổi project = đổi TẦNG 1 bên phải, TẦNG 2 và 3 giữ nguyên
```

---

## 2. Ma trận `appliesRules` — bản đồ tương tác

`wp`=working-policy · `tech`=technical · `cf`=commit-format · `mr`=module-registry · `pp`=project-profile
| `arch`=architecture · `conv`=conventions · `lint` · `tpl`=templates · `ds`=design-system · `lib`=library-docs
(6 cột phải **đến từ pack** → nội dung đổi theo stack)

| Skill | wp | tech | cf | mr | pp | arch | conv | lint | tpl | ds | lib |
|---|:-:|:-:|:-:|:-:|:-:|:-:|:-:|:-:|:-:|:-:|:-:|
| `plan-feature` ⏸ | ● | | | | | | | | | | |
| `spec-from-source` ⏸ | ● | | | | | | | | | | |
| `check-conflicts` ⏸ | ● | | | | ● | | ● | | | ● | |
| `commit` | ● | | ● | | | | | | | | |
| `check-diff` | ● | | | | | | ● | ● | | | |
| `refactor` | ● | | | | | | ● | ● | | | |
| `explain-lib` | ● | | | | | | | | | | ● |
| `learn-project` | ● | | | | ● | ● | ● | | | ● | ● |
| `update-module-registry` | ● | | | ● | | | | | | | |
| **`deliver-feature`** | ● | | ● | ● | ● | ● | ● | ● | | ● | |
| `scaffold-data` | ● | ● | | | ● | ● | ● | | ● | | ● |
| `scaffold-screen` | ● | | | | ● | ● | ● | | ● | | |
| `build-ui` | ● | | | | ● | | ● | | | ● | |
| `gen-routes` | ● | | | | | ● | ● | | | | |

⏸ = `requiresApproval: true`

**Đọc theo cột thì thấy thiết kế:**

- `working-policy` có ở **cả 14** → hiến pháp, không skill nào thoát.
- `project-profile` có ở **6** → đúng 6 skill đó **rẻ đi** sau khi chạy `learn-project` một lần.
- `lint` chỉ ở **3** (`refactor`, `check-diff`, `deliver-feature`) → lint là **cổng kiểm**, không rải khắp nơi.
- `templates` chỉ ở **2** skill scaffold → template chỉ có nghĩa khi dựng khung.
- `technical` ("output phải deterministic") chỉ ở **`scaffold-data`** — nơi duy nhất sinh code từ contract.
- `deliver-feature` gom **8/11** nhóm → đúng bản chất orchestrator.

---

## 3. Bản đồ file — ai ghi, ai đọc

Đây là chỗ luồng thật sự chảy giữa các phiên làm việc.

```
   learn-project ─────ghi────► docs/project-profile.md
                                      │
                    ┌─────────────────┼──────────────────┬──────────────┐
                 đọc▼              đọc▼               đọc▼           đọc▼
          deliver-feature    scaffold-data     scaffold-screen    build-ui
                                                              check-conflicts

   update-module-registry ──ghi──►  docs/module-registry.md  ──đọc──► deliver-feature
   deliver-feature ─────────ghi──►        (cùng file)

   sr fetch ────ghi────► docs/specs/*.md          ──đọc──► spec-from-source
                         docs/context/fetch-*.json          check-conflicts

   sr pull  ────ghi────► src/api/<tag>/*.ts       ──đọc──► scaffold-data  (bước 1)
                         docs/context/pull-*.json

   sr emit  ────ghi────► .skillrunner/ledger.json ──đọc──► sr status
   sr ui    ────ghi────► .skillrunner/ui.json     ──đọc──► sr refresh
```

> **Vì sao thứ tự quan trọng:** chạy `deliver-feature` *trước* `learn-project` → nó phải tự quét lại
> toàn bộ source. Chạy *sau* → nó đọc profile đã cache. Cùng một skill, chi phí khác hẳn.

---

## 4. Ba chuỗi thực tế

### Chuỗi A — nhập môn một repo (chạy MỘT lần)

```
sr status ──► thiếu docs/project-profile.md? ──► learn-project ──► profile
                                                                    └► mọi task sau rẻ hơn
```

### Chuỗi B — yêu cầu nghiệp vụ → code (chuỗi chính)

```
   Confluence / Google Sheet
        │  sr fetch            ← LÀN 0-TOKEN. Không bao giờ cho Claude đọc HTML/ADF thô
        ▼
   docs/specs/*.md
        │  spec-from-source    ⏸ DỪNG chờ duyệt
        ▼
   spec có cấu trúc
        │  check-conflicts     ⏸ DỪNG — đối chiếu spec ⟷ testcase ⟷ ảnh ⟷ rule UI/UX
        ▼                        (taxonomy: docs/ui-ux-conflicts.md §3.0 P1–P8, §3.1–3.6 A1–F4)
   xung đột đã được NGƯỜI quyết
        │  plan-feature        ⏸ DỪNG  (tuỳ chọn — deliver-feature có bước plan riêng)
        ▼
   deliver-feature ──────────┐
        │                     ├─ learn-project        (chỉ khi thiếu profile)
        │                     ├─ scaffold-data → scaffold-screen → build-ui
        │                     ├─ check-diff + linter → refactor
        │                     ├─ update-module-registry
        │                     └─ commit  ⏸ DỪNG chờ xác nhận → KHÔNG BAO GIỜ push
        ▼
   feature đã verify + registry cập nhật + commit
```

### Chuỗi C — API thật → tầng data

```
   authswagger :8080
        │  GET /specs?env=<env>                    → danh sách nhóm
        │  GET /openapi.json?env=<env>&spec=<n>    → spec (servers[0] đã là /env/<env>/api)
        ▼
   sr pull --tag <Tag>          ← LÀN 0-TOKEN (chỉ stack có `codegen` trong pack — nay là react)
        └► src/api/<tag>/{types.ts, <tag>.hooks.ts, index.ts} + digest gọn
              │
              ▼   scaffold-data BƯỚC 1 gọi chính lệnh trên → chuỗi đã liền, không còn nối tay
        scaffold-data → build-ui
```

---

## 5. Skill chạm ra ngoài repo ở đâu

| Skill | Chạm cái gì bên ngoài | Qua đường nào |
|---|---|---|
| `spec-from-source` | Confluence / Google Sheet | **`sr fetch`** trước, Atlassian MCP chỉ là fallback |
| `spec-from-source` | OpenAPI có auth | **authswagger** `/openapi.json?env=&spec=` (đã authenticated sẵn) |
| `check-conflicts` | Confluence / Sheet | **`sr fetch`** (READ-ONLY tuyệt đối — không ghi ngược Jira/Confluence) |
| `check-conflicts` | Taxonomy xung đột | `$SKILLRUNNER_HOME/docs/ui-ux-conflicts.md` — **emit tự đổi thành đường dẫn tuyệt đối** (§6) |
| `check-conflicts` | Ảnh mockup | Claude vision |
| `explain-lib` | Thư viện | pack rule `library-docs` + call site thật trong repo |
| `scaffold-data` | Contract API | **`sr pull`** trước (bước 1), rồi pack rule `templates` |
| `commit` / `check-diff` | git | `git status` / `git diff`; **commit sau xác nhận, không push** |

---

## 6. Hai lỗ hổng — **ĐÃ VÁ** (2026-07-27)

Giữ lại mô tả vì đây là hai *kiểu* lỗi rất dễ tái phát khi thêm skill hoặc lệnh mới.

### Lỗ hổng 1 — `sr pull` mồ côi khỏi tầng skill ✅

**Triệu chứng cũ.** `sr fetch` được **2 skill** gọi đích danh (`spec-from-source`, `check-conflicts`,
cả hai ghi "chạy `sr fetch` TRƯỚC"), nhưng `sr pull` thì **không skill nào nhắc tới** — kể cả
`scaffold-data`, nơi nó thuộc về; skill này chỉ nói *"copy template của pack"*. Hệ quả: chuỗi C phải
nối tay, và Claude tự viết type bằng tay thay vì dùng `types.ts` đã sinh sẵn — đúng cái mà `sr pull`
sinh ra để tránh.

**Ràng buộc đã tôn trọng.** `sr pull` chỉ chạy được trên stack có khối `codegen` trong pack — hiện
**chỉ `packs/react.json`**. `scaffold-data` lại dùng chung cho mọi stack, nên bản vá **không** được là
một bước bắt buộc.

**Đã làm.** Thêm một bước **có điều kiện** làm `instructions[0]` của `scaffold-data` (trước bước "copy
template"), cộng một dòng vào `inputs`. Bước đó nói rõ: có nguồn OpenAPI thì chạy `sr pull` trước, đọc
**digest** chứ không đọc spec thô, xây trên file đã sinh; **pack thiếu `codegen` thì lệnh tự báo lỗi
rõ ràng và rơi về template cũ**. Chỉ sửa `skill.json` — **không đụng code Go**, không stack nào gãy.

**Mở rộng sau này:** thêm khối `codegen` cho `packs/flutter.json` / `packs/kotlin.json` — cùng dòng
instruction đó tự nhiên áp dụng cho stack mới, không phải sửa `skill.json` lần nữa.

---

### Lỗ hổng 2 — `check-conflicts` phụ thuộc biến môi trường `$SKILLRUNNER_HOME` ✅

**Triệu chứng cũ.** `$SKILLRUNNER_HOME` xuất hiện **đúng 2 dòng**, cả hai trong `check-conflicts`, trỏ
tới `docs/ui-ux-conflicts.md`. Biến này do **hàm wrapper `sr` trong `~/.zshrc`** định nghĩa → nó
**không tồn tại** khi emit qua **MCP** (`sr serve` là tiến trình con, không nạp `.zshrc`), khi gọi
thẳng binary `skillrunner`, hay khi chạy cron/CI. Claude nhận một đường dẫn không giải được → bỏ qua
taxonomy hoặc đoán bừa. Đây là skill **duy nhất** có ràng buộc này nên rất dễ bị bỏ sót.

**Đã làm — để `emit` tự thay thế placeholder, không trông chờ shell.** Binary luôn biết manifest nằm
ở đâu (`-f skill.json`), nên nó tự điền:

| Chỗ sửa | Việc |
|---|---|
| `manifest.go` — `Manifest.home` | field không export, giữ thư mục tuyệt đối của manifest |
| `manifest.go` — `Load()` | `m.home = abs(filepath.Dir(path))`; nếu lỗi thì **để nguyên placeholder**, không resolve thành `""` |
| `emit.go` — `Emit()` | trả về qua `m.resolveHome(...)`; `EmitAll()` gọi `Emit()` nên tự hưởng |
| `emit_test.go` | 3 test hồi quy: `Emit`, `EmitAll`, và **`Merge` phải giữ `home`** (`Merge` copy struct — chỗ dễ mất nhất) |

`skill.json` **không sửa một dòng nào** — vẫn viết `$SKILLRUNNER_HOME` như cũ, và mọi skill sau này
dùng lại được ngay. Vẫn deterministic: cùng manifest → cùng đường dẫn.

**Các phương án đã cân nhắc rồi loại:**

| Phương án | Vì sao loại |
|---|---|
| Thêm lệnh `sr docs <name>` | Cũng chạy được mọi nơi, nhưng thêm command + phải sửa `skill.json`, và Claude tốn thêm một lệnh. Giữ làm bước sau nếu muốn phát doc qua MCP. |
| Hardcode đường dẫn tuyệt đối | Hỏng ngay khi người khác clone repo. |
| Ra luật "bắt buộc phải có wrapper `sr`" | Chính là bug hiện tại, chỉ là hợp thức hoá nó. |

**Kiểm chứng lại bất cứ lúc nào:**

```bash
sr emit check-conflicts | grep -c SKILLRUNNER_HOME      # phải = 0

# và qua đúng đường đã từng hỏng — MCP, với biến môi trường bị gỡ:
printf '%s\n' \
 '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}' \
 '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"emit_skill","arguments":{"skill":"check-conflicts","dir":"."}}}' \
 | env -u SKILLRUNNER_HOME skillrunner serve -f "$PWD/skill.json" | grep -c SKILLRUNNER_HOME   # phải = 0
```

---

## 7. Cách tự dựng lại hiểu biết này khi doc lạc hậu

Doc này là ảnh chụp. Bốn lệnh dưới đây **luôn** cho bản mới nhất:

```bash
sr emit all > /tmp/catalog.md        # toàn bộ 14 skill + rule đã merge (KHÔNG ghi ledger)

sr emit build-ui --pack react   > /tmp/a.md    # bài tập quan trọng nhất:
sr emit build-ui --pack flutter > /tmp/b.md    # phần GIỐNG = skill, phần KHÁC = pack
diff /tmp/a.md /tmp/b.md

sr status                            # repo này đang ở đâu: stack, profile, registry, ledger
sr ledger                            # skill nào đã emit ở đây, lúc nào
```

Muốn biết vì sao một skill hành xử thế nào → tìm `appliesRules` của nó trong `skill.json`. **Toàn bộ
hành vi nằm ở dòng đó**, không giấu trong code Go.

---

## 8. Liên quan

- [`skill-taxonomy.md`](./skill-taxonomy.md) — kho skill hợp nhất (danh mục)
- [`skill-runner-design.md`](./skill-runner-design.md) — thiết kế hệ thống
- [`ui-ux-conflicts.md`](./ui-ux-conflicts.md) — taxonomy xung đột cho `check-conflicts`
- [`sr-pull-design.md`](./sr-pull-design.md) · [`sr-fetch-design.md`](./sr-fetch-design.md) — hai làn 0-token
- [`sr-serve-mcp.md`](./sr-serve-mcp.md) — chế độ MCP server (nơi lỗ hổng 2 phát tác)
- `skill.json` — **nguồn sự thật**; doc này chỉ vẽ lại nó
