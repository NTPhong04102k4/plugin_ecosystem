# Skill Runner (Go) — Tài liệu thiết kế & quyết định

> Mục tiêu: Một **binary Go** duy nhất (build ra `bin`/`.exe`) đóng vai trò "bộ chạy skill".
> Bạn thả binary + một **file manifest JSON khai báo** (không phải script chi tiết) vào bất kỳ project nào,
> và Claude dùng nó để chạy skill — **không cần tái cấu trúc theo từng dự án**.

Định dạng manifest đã chốt: **JSON** (`skill.json`).

---

## 1. Khái niệm cốt lõi: "file yêu cầu" vs "file thực thi chi tiết"

Đây là ý tưởng trung tâm bạn đưa ra. Phân biệt:

| Loại | Là gì | Ví dụ |
|------|-------|-------|
| **File yêu cầu (declarative)** ✅ | Chỉ **khai báo** "muốn chạy gì", không chứa logic | `skill.json`: `{"skill": "build", "args": {...}}` |
| **File thực thi chi tiết (imperative)** ❌ | Chứa toàn bộ logic thực thi, phải viết lại mỗi project | `build.sh`, `deploy.js` dài 200 dòng |

**Nguyên tắc:** logic thực thi nằm **trong binary Go** (viết 1 lần, dùng mọi nơi).
Mỗi project chỉ cần **1 file JSON nhỏ khai báo ý định**. Đổi project → chỉ đổi JSON, không đổi code.

```
┌─────────────────┐      đọc      ┌──────────────────┐
│  skill.json     │ ────────────► │  skillrunner     │  (logic nằm ở đây, dùng chung)
│  (khai báo)     │               │  (Go binary)     │
└─────────────────┘               └──────────────────┘
      ▲ mỗi project 1 file                 ▲ 1 binary cho tất cả project
```

---

## 2. Ba phương án tích hợp — giao tiếp diễn ra như thế nào?

### Phương án A — CLI standalone ⭐ (đơn giản nhất)

Claude gọi binary qua công cụ **Bash**, giao tiếp bằng **stdin/stdout/exit code**.

```
Claude ──(Bash tool)──► ./skillrunner run skill.json ──► stdout: kết quả JSON
                                                     └──► exit 0 = OK, ≠0 = lỗi
```

**Luồng giao tiếp:**
1. Claude chạy `./skillrunner run skill.json` (hoặc `skillrunner run --skill build`).
2. Binary đọc `skill.json`, thực thi skill tương ứng.
3. Binary in kết quả ra **stdout** (dạng JSON hoặc text) → Claude đọc lại.
4. **Exit code** cho biết thành công/thất bại.

**Ưu điểm:**
- Không cần cấu hình gì thêm. Chỉ cần binary nằm trong project (hoặc trong PATH).
- Portable tuyệt đối: copy binary + `skill.json` là chạy được ở mọi máy/mọi project.
- Dễ debug: bạn tự chạy tay `./skillrunner run skill.json` được ngay.
- Hợp nhất với ý "kéo file tool vào để chạy".

**Nhược điểm:**
- Claude phải "biết" gọi lệnh này (cần ghi trong `CLAUDE.md` hoặc bạn bảo nó).
- Không tự động — Claude chỉ gọi khi được yêu cầu / khi đọc hướng dẫn.

**Khi nào dùng:** Muốn nhanh, gọn, portable, ít ma thuật. **Khuyến nghị cho lần đầu.**

---

### Phương án B — MCP server (tích hợp sâu)

Binary chạy như một **MCP server** (Model Context Protocol). Claude tự động **khám phá** các skill
như những "tool" và gọi trực tiếp — không cần qua Bash.

```
Claude ◄──(giao thức MCP qua stdio/JSON-RPC)──► skillrunner (đang chạy nền)
   │
   ├─ khám phá: "server này có tool: build, deploy, test..."
   └─ gọi: tool "build" với tham số {...} ──► server trả kết quả
```

**Luồng giao tiếp:**
1. Bạn đăng ký server 1 lần trong cấu hình MCP của Claude Code (`.mcp.json` / settings).
2. Khi Claude khởi động, nó bắt tay (handshake) với server, hỏi "có tool gì?".
3. Binary đọc `skill.json` → khai báo mỗi skill thành **một MCP tool** (có tên, mô tả, schema tham số).
4. Claude thấy các tool này trong danh sách và **tự gọi khi phù hợp**, truyền tham số dạng JSON.
5. Giao tiếp qua **JSON-RPC trên stdio** (hoặc HTTP/SSE) — chuẩn MCP, không phải stdout thô.

**Ưu điểm:**
- Claude **tự động** biết skill nào tồn tại và tự gọi — không cần bạn nhắc.
- Có schema tham số rõ ràng → Claude gọi đúng đối số.
- Tích hợp "native", giống các MCP server khác (Figma, Atlassian...).

**Nhược điểm:**
- Phức tạp hơn: phải implement giao thức MCP (handshake, list tools, call tool).
- Cần đăng ký cấu hình 1 lần cho mỗi máy/mỗi nơi dùng (kém "kéo-thả" hơn).
- Server chạy nền, khó debug hơn CLI.

**Khi nào dùng:** Muốn Claude tự động dùng skill mà không cần nhắc, chấp nhận setup 1 lần.

---

### Phương án C — Claude Code hook (tự động theo sự kiện)

Binary chạy như một **hook**: Claude Code tự gọi nó **khi có sự kiện** (trước/sau khi dùng tool,
khi bắt đầu/kết thúc phiên, khi submit prompt...). Cấu hình trong `settings.json`.

```
Sự kiện (vd: sau khi Edit file) ──► Claude Code chạy hook ──► skillrunner
                                                              │ đọc stdin (JSON sự kiện)
                                                              └► stdout/exit code: phản hồi
```

**Luồng giao tiếp:**
1. Bạn khai báo hook trong `settings.json`, trỏ tới binary + sự kiện muốn bắt (vd `PostToolUse`).
2. Khi sự kiện xảy ra, Claude Code chạy binary và **đẩy dữ liệu sự kiện vào stdin** (JSON).
3. Binary xử lý (vd: chạy skill "format" sau mỗi lần sửa file), trả kết quả qua stdout/exit code.
4. Exit code / output có thể **chặn hoặc cho phép** hành động, hoặc chèn phản hồi cho Claude.

**Ưu điểm:**
- **Tự động hoàn toàn** — không ai phải gọi, cứ đúng sự kiện là chạy (vd auto-format, auto-test).
- Tốt cho các quy tắc "mỗi khi X thì làm Y".

**Nhược điểm:**
- Không phải để "chạy skill theo yêu cầu" — nó thiên về phản ứng sự kiện.
- Cấu hình gắn với Claude Code cụ thể, kém portable.
- Dễ gây phiền nếu bắt sai sự kiện (chạy quá nhiều lần).

**Khi nào dùng:** Muốn tự động hoá theo sự kiện (format, lint, test tự chạy), không phải gọi thủ công.

---

## 3. So sánh nhanh

| Tiêu chí | A. CLI | B. MCP server | C. Hook |
|---------|--------|---------------|---------|
| Độ phức tạp code | Thấp | Cao | Trung bình |
| Cấu hình mỗi project | Không (chỉ cần file) | 1 lần / máy | 1 lần / máy |
| Claude tự động dùng | Không (cần nhắc) | **Có** | **Có** (theo sự kiện) |
| Portable ("kéo-thả") | **Cao nhất** | Trung bình | Thấp |
| Dễ debug tay | **Cao nhất** | Thấp | Trung bình |
| Giao thức | stdout + exit code | JSON-RPC (MCP) | stdin JSON + exit code |
| Hợp mục tiêu của bạn | ✅✅✅ | ✅✅ | ✅ |

**Gợi ý:** Bắt đầu với **A (CLI)** vì đúng nhất với ý "kéo binary + file JSON vào là chạy, không tái cấu trúc".
Sau này có thể **nâng cấp cùng 1 binary lên B (MCP)** — chỉ thêm một lệnh con `skillrunner mcp`,
dùng chung logic đọc `skill.json`. Kiến trúc bên dưới thiết kế để đi được cả hai.

---

## 4. "Skill" là gì? — Giải thích 3 lựa chọn loại skill

Đây là câu hỏi bạn muốn tôi giải thích để chọn. "Skill" = đơn vị công việc mà binary chạy.
Có 3 cách hiểu:

### Lựa chọn 1 — Lệnh/script khai báo trong manifest ⭐

Manifest JSON khai báo các **bước** (chuỗi lệnh shell / script) để binary thực thi tuần tự.

```jsonc
// skill.json
{
  "skills": {
    "build": {
      "description": "Build project",
      "steps": ["npm install", "npm run build"]
    },
    "deploy": {
      "description": "Deploy lên staging",
      "steps": ["npm run build", "./scripts/upload.sh staging"]
    }
  }
}
```

- **Bản chất:** binary là một "task runner" khai báo (giống Make/npm scripts nhưng portable, 1 binary).
- **Ưu:** cực kỳ linh hoạt, mỗi project chỉ khác nhau ở JSON. Không cần viết code Go mới cho skill mới.
- **Nhược:** vẫn phụ thuộc shell command của môi trường (npm, bash...). Logic vẫn là "lệnh", chỉ là được khai báo gọn.
- **Đúng nhất với ý bạn:** "file yêu cầu thực thi, không phải file thực thi chi tiết".

### Lựa chọn 2 — Claude Code SKILL.md

Binary đọc và chạy các skill theo **chuẩn `SKILL.md`** của Claude Code (file markdown + frontmatter mô tả skill).

- **Bản chất:** binary trở thành "trình chạy" cho hệ skill sẵn có của Claude Code.
- **Ưu:** tái dùng được các skill đã viết theo chuẩn Claude Code; thống nhất hệ sinh thái.
- **Nhược:** `SKILL.md` vốn là **hướng dẫn cho model đọc**, không phải chương trình để binary "chạy".
  Binary chỉ có thể *nạp/định tuyến* chúng, chứ không tự "hiểu" như Claude. Ít giá trị nếu chạy độc lập.
- **Khi nào chọn:** nếu bạn đã có nhiều `SKILL.md` và muốn 1 công cụ để quản lý/định tuyến chúng.

### Lựa chọn 3 — Plugin binary/module riêng

Mỗi skill là một **plugin độc lập** (binary con, hoặc Go plugin `.so`) mà binary chính điều phối.

```jsonc
// skill.json
{
  "skills": {
    "build":  { "plugin": "./plugins/build",  "args": {...} },
    "deploy": { "plugin": "./plugins/deploy", "args": {...} }
  }
}
```

- **Bản chất:** binary chính là "orchestrator", mỗi skill là chương trình riêng.
- **Ưu:** mạnh nhất, mỗi skill có thể viết bằng ngôn ngữ bất kỳ, cô lập tốt.
- **Nhược:** nặng nhất — phải phân phối nhiều binary con, mất tính "1 file kéo-thả".
- **Khi nào chọn:** hệ skill lớn, phức tạp, cần cô lập/độc lập từng skill.

### Bảng chọn

| | 1. Lệnh khai báo | 2. SKILL.md | 3. Plugin riêng |
|--|------------------|-------------|-----------------|
| Portable "1 file" | ✅✅✅ | ✅✅ | ❌ |
| Thêm skill mới không sửa code Go | ✅ | ✅ | ✅ |
| Chạy độc lập (không cần Claude hiểu) | ✅ | ❌ | ✅ |
| Phức tạp phân phối | Thấp | Thấp | Cao |
| Hợp mục tiêu của bạn | ✅✅✅ | ✅ | ✅ |

**Gợi ý:** **Lựa chọn 1** (lệnh khai báo trong manifest) đúng nhất với mục tiêu:
1 binary + 1 file JSON khai báo, đổi project chỉ đổi JSON, không tái cấu trúc.

---

## 5. Đề xuất tổng thể (để bạn duyệt)

> **CLI standalone (A) + Skill kiểu lệnh khai báo (1) + manifest JSON.**

Kiến trúc dự kiến của binary:

```
skillrunner
├── run <skill> [--file skill.json]   # chạy 1 skill theo manifest  (phương án A)
├── list [--file skill.json]          # liệt kê skill có trong manifest
├── validate [--file skill.json]      # kiểm tra manifest hợp lệ
└── mcp                               # (nâng cấp sau) chạy như MCP server (phương án B)
```

- Build đa nền tảng: `bin/skillrunner` (macOS/Linux) và `skillrunner.exe` (Windows) từ cùng mã nguồn Go.
- Dùng **thư viện chuẩn Go** (`encoding/json`, `os/exec`) — không phụ thuộc ngoài, binary tĩnh, dễ mang đi.
- Thiết kế tách lớp để sau này thêm `mcp` mà không phải viết lại phần đọc `skill.json`.

---

## 6. Bạn cần quyết định

- [ ] **Tích hợp:** A (CLI) / B (MCP) / C (Hook) — *đề xuất: A trước, chừa đường lên B*
- [ ] **Loại skill:** 1 (lệnh khai báo) / 2 (SKILL.md) / 3 (plugin) — *đề xuất: 1*
- [ ] Manifest: **JSON** ✅ (đã chốt)

Chọn xong, tôi sẽ khởi tạo `go.mod`, viết mã nguồn và build ra binary + `skill.json` mẫu.
