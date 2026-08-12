# `sr serve` — chạy skillrunner như một **MCP server**

> Trạng thái: **ĐÃ CÓ CODE** (`cmd/skillrunner/mcp.go`). Doc này mô tả *cách chạy*, *tác dụng*,
> và *khác gì so với gọi CLI*.

## 0. Đính chính một nhầm lẫn hay gặp: server hay host?

| Vai | Ai đóng | Làm gì |
|---|---|---|
| **MCP host / client** | **Claude Code** (hoặc Claude Desktop, agent khác) | Khởi động tiến trình server, hỏi nó có tool gì, quyết định khi nào gọi |
| **MCP server** | **`sr serve`** ← chính là cái này | Ngồi im chờ, khai báo 5 tool, thực thi khi được gọi |

`sr serve` **không** phải MCP host và **không** tự gọi ai. Nó là **đầu bên kia của sợi dây** —
Claude Code mới là bên cầm dây. (Cái *host* thật trong workspace này là `mcp-client` / MCP Studio,
xem `../../mcp-client/docs/architecture.md` — nó nằm ở phía đối diện.)

```
   Claude Code  ────spawn tiến trình con────►  sr serve
   (MCP HOST)   ◄───stdin/stdout JSON-RPC───►  (MCP SERVER)
        │                                          │
        │ "có tool nào?" (tools/list)              │ → 5 tool
        │ "chạy emit_skill{skill:build-ui}"        │ → text marching orders
        ▼                                          ▼
   đưa text vào context                     đọc skill.json + packs/, in text
```

---

## 1. Chạy thế nào

`sr serve` **không dùng để gõ tay**. Nếu gõ trực tiếp, nó sẽ đứng im chờ JSON-RPC trên stdin —
đó là hành vi đúng, không phải treo máy. Nó được **Claude Code tự khởi động** qua `.mcp.json`:

```jsonc
// <repo>/.mcp.json  — file này commit được, không chứa secret
{
  "mcpServers": {
    "skillrunner": {
      "command": "skillrunner",
      "args": [
        "serve",
        "-f",
        "/Users/<you>/.../my-plugin-ecosystem/skill.json"
      ]
    }
  }
}
```

- `command` phải là binary **có trong PATH** (`skillrunner`), **không phải hàm shell `sr`** —
  hàm wrapper trong `~/.zshrc` không tồn tại với tiến trình con do Claude Code sinh ra.
- Vì mất wrapper nên **bắt buộc truyền `-f <đường dẫn tuyệt đối tới skill.json>`**; nếu thiếu,
  server sẽ tìm `skill.json` trong thư mục làm việc và hầu như luôn báo lỗi.
- `packs/` được tìm **cạnh** file manifest (`filepath.Dir(file)`), nên chỉ cần trỏ đúng
  `skill.json` là pack tự có.
- Mở Claude Code trong repo → tool xuất hiện dạng `mcp__skillrunner__emit_skill`, …

Kiểm tra thủ công (mô phỏng một host tối giản):

```bash
printf '%s\n' \
 '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}' \
 '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
 | skillrunner serve -f "$SKILLRUNNER_HOME/skill.json"
```

---

## 2. Năm tool được khai báo

| Tool | Tham số | Trả về | **Có ghi file không?** |
|---|---|---|---|
| `detect_stack` | `dir?` | `Detected stack: go (go.mod present)` | không |
| `list_skills` | `dir?`, `pack?` | danh sách skill + mô tả | không |
| `emit_skill` | **`skill`** (bắt buộc), `dir?`, `pack?` | marching orders (Markdown) | **CÓ** — ghi `.skillrunner/ledger.json` |
| `apply_base` | `dir?`, `pack?`, `force?` | báo cáo từng file copied/skipped | **CÓ** — copy config file vào repo |
| `status` | `dir?` | stack + profile/registry/ledger đã cache chưa | không |

Ghi chú quan trọng:

- **`dir` mặc định** = `--dir` lúc khởi động server (nếu không truyền thì là thư mục làm việc của
  tiến trình). Muốn thao tác trên repo khác → truyền `dir` tuyệt đối trong mỗi lần gọi tool.
- **`emit_skill` có side effect**: mỗi lần gọi được ghi vào ledger để phiên sau biết skill này
  đã chạy ở repo này rồi. `emit_skill{skill:"all"}` là *dump catalog* → **không** ghi ledger.
  Nếu ghi ledger lỗi, marching orders **vẫn được trả về** (chỉ cảnh báo ra stderr) — không bao
  giờ nuốt mất kết quả đã có.
- **`apply_base` là tool duy nhất sửa repo thật**: nó copy file config của pack (eslint / linter /
  tsconfig…) vào project. Mặc định **bỏ qua file đã tồn tại**; `force: true` mới ghi đè.
- Lỗi ở mức tool trả về `isError: true` + text (đúng chuẩn MCP), **không** ném JSON-RPC error —
  nhờ vậy Claude đọc được thông báo lỗi và tự xoay xở thay vì đứt kết nối.

---

## 3. Giao thức & ràng buộc kỹ thuật

| Khoản | Giá trị |
|---|---|
| Transport | **stdio**, JSON-RPC 2.0, mỗi message một dòng (newline-delimited) |
| Protocol version | `2024-11-05`; nếu client gửi version khác thì **echo lại version của client** |
| Method hỗ trợ | `initialize`, `notifications/initialized`, `ping`, `tools/list`, `tools/call` |
| Dependency | **stdlib Go thuần** — không thư viện MCP nào, giữ tính chất "một binary tự chứa" |
| Buffer | tối đa **4 MB/message** (marching orders dài vẫn lọt) |
| Capability | chỉ `tools`. **Không** resources, **không** prompts, **không** sampling |

**Luật vàng của stdio:** `stdout` **chỉ** chứa message giao thức; mọi log/cảnh báo đi ra
`stderr`. Bất kỳ dòng `fmt.Println` lạc vào stdout sẽ **làm hỏng phiên MCP**. Nếu bạn sửa code
trong `mcp.go`, nhớ điều này.

---

## 4. Tác dụng thật sự: MCP so với CLI

Cả hai đường **chạy chung một hàm** trong `internal/skill/` (`runTool` gọi đúng thứ CLI gọi), nên
**kết quả giống hệt nhau**. Khác biệt nằm ở *trải nghiệm*, không phải *nội dung*:

| | Gọi CLI (`Bash: sr emit build-ui`) | Gọi MCP (`emit_skill`) |
|---|---|---|
| Claude có tự biết tool tồn tại? | **Không** — phải được CLAUDE.md nhắc mới nghĩ tới | **Có** — tool nằm sẵn trong danh sách |
| Cần wrapper `sr` / PATH? | Có | Không (đường dẫn cố định trong `.mcp.json`) |
| Xin quyền | mỗi lệnh Bash một lần | theo cấu hình permission của MCP |
| Chạy ở môi trường không có shell | không | có |
| Debug bằng mắt | **dễ** — gõ lệnh, thấy ngay output | khó hơn, phải xem log MCP |
| Token tiêu tốn | **như nhau** — cả hai đều 0 token lúc *sinh* text; token chỉ phát sinh khi Claude *đọc* output | như nhau |

> **MCP không giúp tiết kiệm token.** Đây là kết luận đã chốt trong
> [`mcp-architecture.md`](./mcp-architecture.md) (§ trạng thái: hoãn mở rộng MCP). Lợi ích thật
> của `sr serve` là **tính khám phá được** (discoverability) và **không phụ thuộc shell** —
> chọn nó vì hai lý do đó, đừng chọn vì tưởng rẻ hơn.

### Nên dùng đường nào?

- **Đang ngồi trong Claude Code, có Bash** → CLI cũng tốt; dễ debug, thấy tận mắt.
- **Muốn Claude tự nhớ ra skill mà không cần nhắc trong CLAUDE.md** → MCP.
- **Agent khác / môi trường không shell** → bắt buộc MCP.
- **Cần `pull` / `fetch` / `ui`** → **CLI, không có đường MCP.** Xem mục 5.

---

## 5. Cái `sr serve` **không** làm

- **Không expose `pull`, `fetch`, `ui`, `refresh`, `bootstrap`, `init`, `validate`, `ledger`.**
  Chỉ 5 tool ở §2. Muốn dùng `pull`/`fetch` từ Claude → gọi qua Bash.
  (Bản thiết kế cho một MCP `pull` riêng nằm ở [`mcp-architecture.md`](./mcp-architecture.md) §2
  MCP #3 — **đang hoãn**, chưa code.)
- **Không giữ state giữa các lần gọi.** Mỗi tool call đọc lại manifest + pack từ đĩa → sửa
  `skill.json` hay `packs/*.json` **có hiệu lực ngay**, không cần restart server.
  (Ngược lại, sửa **code Go** thì phải build lại + khởi động lại Claude Code.)
- **Không xác thực, không phân quyền.** Ai spawn được tiến trình thì gọi được mọi tool. Đây là
  tiến trình con cục bộ, không phải service mạng — nhưng nhớ rằng `apply_base` **ghi vào repo**.

---

## 6. Troubleshooting

| Triệu chứng | Nguyên nhân & cách xử lý |
|---|---|
| Claude Code không thấy tool `skillrunner` | `.mcp.json` chưa được tin cậy/nạp → khởi động lại Claude Code trong repo; kiểm tra `/mcp`. |
| `command not found: skillrunner` | `.mcp.json` phải trỏ **binary trong PATH**, không phải hàm `sr`. `sudo cp bin/skillrunner /usr/local/bin/`. |
| Tool báo *không tìm thấy skill.json* | Thiếu `-f <đường dẫn tuyệt đối>` trong `args`. |
| Tool chạy nhưng **sai repo** | `dir` mặc định là thư mục của tiến trình server. Truyền `dir` tuyệt đối khi gọi tool. |
| Cảnh báo `no pack for stack "x"` (stderr) | Chưa có `packs/x.json`. Tạo pack, hoặc truyền `pack` trong tham số tool. |
| Phiên MCP đứt giữa chừng | Có gì đó ghi rác vào **stdout**. Mọi log phải đi `stderr` (§3). |
| Sửa `skill.json` mà không thấy đổi | Không phải lỗi — nó đọc lại mỗi lần gọi. Nếu vẫn cũ, có thể bạn sửa nhầm manifest khác với cái ghi trong `.mcp.json`. |

---

## 7. Liên quan

- [`sr-ui-guide.md`](./sr-ui-guide.md) — `sr ui` / `sr refresh` (web cục bộ, không phải MCP)
- [`mcp-architecture.md`](./mcp-architecture.md) — thiết kế 3 MCP (đang **hoãn**, giữ làm phương án)
- [`skill-runner-design.md`](./skill-runner-design.md) — thiết kế tổng thể
- `cmd/skillrunner/mcp.go` — toàn bộ hiện thực (~350 dòng, stdlib)
