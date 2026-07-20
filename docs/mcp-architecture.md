# Kiến trúc MCP — 3 con, mỗi con một việc

> ⚠️ **TRẠNG THÁI: HOÃN (superseded).** Quyết định cuối (2026-07-20) là **CLI-first** —
> xem [`sr-pull-design.md`](./sr-pull-design.md). Lý do: MCP không lợi hơn về token, và vì luôn
> dùng qua Claude Code (curl/Bash được) nên MCP chỉ thêm code JSON-RPC + process. Logic viết 1
> lần trong `internal/` → **có thể bọc MCP sau** theo đúng doc này nếu cần agent khác / discoverable.
> Giữ doc này làm phương án tương lai.
>
> Nguyên tắc: **một MCP một việc** (single responsibility).

## 1. Topology

```
                         ┌──────────────── CLAUDE (orchestrator) ────────────────┐
                         │  gọi 3 MCP như 3 tool tách bạch                        │
                         └───┬───────────────────┬──────────────────────┬────────┘
                             │                   │                      │
                   ┌─────────▼────────┐ ┌────────▼─────────┐ ┌──────────▼──────────┐
                   │  MCP #1           │ │  MCP #2           │ │  MCP #3             │
                   │  skillrunner      │ │  authswagger      │ │  bridge (pull)      │
                   │  VIỆC: convention │ │  VIỆC: spec       │ │  VIỆC: ghép→digest  │
                   │  (đã có)          │ │  (MỚI)            │ │  (MỚI)              │
                   └───────────────────┘ └─────────┬─────────┘ └──────────┬──────────┘
                                                    │ dùng chung            │ gọi /endpoints (HTTP)
                                                    ▼ package specdigest     ▼ + import skillrunner pkg
                                          ┌───────────────────────────────────────────┐
                                          │  authswagger HTTP :8080 (proxy runtime)     │
                                          │  ForgeRock login · /specs · /endpoints ·    │
                                          │  proxy /env/<env>/api/*                      │
                                          └───────────────────────────────────────────┘
```

## 2. Ba MCP — việc & tool

### MCP #1 — skillrunner (đã có, KHÔNG đổi)
- **Việc:** cấp convention/skill.
- **Tool:** `detect_stack`, `list_skills`, `emit_skill`, `apply_base`, `status`.
- stdlib-only, stdio JSON-RPC. Giữ nguyên.

### MCP #2 — authswagger MCP (MỚI, trong WorkFlowAutomation)
- **Việc:** expose spec/endpoints (read-only, deterministic).
- **Tool:**
  | Tool | Trả về |
  |---|---|
  | `list_envs` | danh sách environment |
  | `list_specs(env)` | nhóm spec (dropdown) |
  | `get_endpoints(env, spec, tag?)` | endpoint digest gọn: `[{op,method,path,reqRef,resRef}]` + proxyBase |
  | `get_schema(env, spec, refs[])` | subset schema tối thiểu cho các ref cần |
- **Hiện thực:** thêm mode `authswagger mcp` vào binary Go sẵn có; tách logic lọc tag ra
  package **`specdigest`** dùng chung cho cả MCP lẫn HTTP.
- **Login lazy:** MCP process start cùng session nhưng **chỉ login ForgeRock khi tool đầu tiên
  cần spec được gọi** (cache token) → session start không tốn login nếu không dùng.

### MCP #3 — bridge / pull (MỚI, trong my-plugin-ecosystem)
- **Việc:** ghép authswagger + skillrunner → data-layer + digest (hành vi = `sr pull` đã chốt).
- **Tool:** `pull(from|env|spec|tag, out, dir)` → trả **digest có cấu trúc** + ghi
  `types.ts` + `<tag>.hooks.ts` + `index.ts` + cache `docs/context/pull-<tag>.json`.
- **Hiện thực:** mode `skillrunner serve-pull` (cùng binary skillrunner, entry .mcp.json RIÊNG →
  vẫn là "một MCP một việc"). Logic viết 1 lần trong `internal/`, expose **cả** `sr pull` (CLI)
  **lẫn** tool MCP `pull`.
- **Cách ghép (KHÔNG MCP→MCP):**
  1. Lấy endpoints từ **authswagger HTTP `/endpoints?env=&spec=&tag=`** (endpoint mới, dùng
     chung package `specdigest`).
  2. Lấy pack/template + convention từ **skillrunner internal package** (import trực tiếp, cùng repo).
  3. Codegen types: **openapi-typescript** (npx).
  4. Instantiate template **react-query hook**.
  5. Ghi file + cache + trả digest.

## 3. Điểm tinh tế cần chốt

1. **HTTP :8080 vẫn cần chạy lúc RUNTIME.** MCP là mặt *design-time* (kéo spec để codegen).
   Nhưng hook sinh ra gọi API thật qua **proxy HTTP `/env/<env>/api`** lúc chạy FE → bạn **vẫn
   phải bật `go run .`** như quyết định trước. MCP không thay thế được mặt proxy này.
2. **Login lazy** (mục MCP #2) — đồng ý để session start không tốn login.
3. **Bridge gọi authswagger qua HTTP, không qua MCP #2** — tránh MCP-gọi-MCP (phức tạp). MCP #2
   là mặt cho **Claude khám phá spec trực tiếp**; bridge tự đi HTTP. Cả hai xài chung package
   `specdigest` nên **không trùng logic**.

## 4. .mcp.json sau khi xong

```json
{
  "mcpServers": {
    "skillrunner": { "command": "skillrunner", "args": ["serve", "-f", ".../skill.json"] },
    "authswagger": { "command": "authswagger", "args": ["mcp", "-c", ".../config.yaml"] },
    "pull":        { "command": "skillrunner", "args": ["serve-pull", "-f", ".../skill.json"] }
  }
}
```

## 5. Thứ tự hiện thực (khi được duyệt)

- **A · authswagger** (repo WorkFlowAutomation): package `specdigest` (lọc tag, minimal schema)
  → HTTP `/endpoints` → mode `authswagger mcp` (4 tool) + login lazy.
- **B · skillrunner** (repo my-plugin-ecosystem): xác minh/bổ sung `packs/react.json` template
  react-query → logic `pull` trong `internal/` → CLI `sr pull` + mode `serve-pull` (tool `pull`).
- **C · wiring**: cập nhật `.mcp.json` (thêm authswagger + pull).
- **D · test**: tag `Khoa` thật (sau khi bạn bật authswagger HTTP + login).

## 6. Ngoài phạm vi

- Không MCP-hóa barcode_createQr (chưa có code).
- Bridge không build UI / không commit (gate của Claude giữ nguyên).
- Không đụng n8n (đã bỏ khỏi luồng).
</content>
