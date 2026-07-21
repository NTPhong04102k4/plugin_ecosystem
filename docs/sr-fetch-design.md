# Thiết kế: `sr fetch` — cầu nối deterministic Confluence / Google Sheet → markdown + digest

> Trạng thái: **ĐÃ CHỐT (frozen)** — chờ duyệt để code. Không code trước khi duyệt.
> Ngày chốt: 2026-07-20.

## 1. Mục tiêu & vấn đề

Nguồn spec/testcase của team nằm trên **Confluence** (mô tả nghiệp vụ) và **Google Sheet**
(bảng testcase). Việc lấy nội dung về là **deterministic** — không cần suy luận. Nếu để Claude
đọc raw thì đốt token ở hai chỗ:

1. Đọc **HTML/ADF thô của Confluence** (trang dài, nhiều bảng).
2. Đọc **Sheet thô** qua MCP/interactive-auth (không chạy được ở headless/cron).

`sr fetch` biến seam này thành **một lệnh 0 token**: fetch bằng **link + token** → chuẩn hoá về
**markdown sạch** + in **digest gọn**. Claude chỉ đọc digest/markdown gọn rồi tiêu token ở phần
cần não: dựng spec, sinh/đối chiếu testcase, phát hiện xung đột UI/UX (xem
[`ui-ux-conflicts.md`](./ui-ux-conflicts.md)).

Đây là **anh em với `sr pull`** ([`sr-pull-design.md`](./sr-pull-design.md)) nhưng cho **nội dung
prose/tabular**, không phải OpenAPI→code.

## 2. Vị trí trong kiến trúc

```
Confluence / Google Sheet ──fetch(link+token)──► chuẩn hoá ──► markdown sạch + DIGEST gọn
     (HTML/ADF, CSV thô)                          (parse)      (docs/specs/ + stdout)
        └────────────────── LÀN 0-TOKEN (sr fetch, không có Claude) ──────────────┘
                                                                          ▼ (~0.5–2k token)
                                       CLAUDE: đọc digest/markdown → dựng spec / testcase
                                       (KHÔNG BAO GIỜ đọc raw HTML/ADF/Sheet)
```

`sr fetch` là làn 0-token; Claude vào sau, chỉ ăn markdown/digest. Kết quả này là **input cho
skill `spec-from-source`**.

## 3. Quyết định đã khóa

| # | Quyết định | Chốt |
|---|---|---|
| 1 | Command | **Lệnh mới `sr fetch`** (tách khỏi `sr pull`: pull = OpenAPI→code, fetch = nội dung→markdown) |
| 2 | Confluence auth | **API token Atlassian**, Basic auth `base64(email:token)`, gọi **v2 API** `body-format=atlas_doc_format` (ADF JSON) |
| 3 | Google auth | **Service-account / refresh-token** (robust, vào core) — tự mint access token ngắn hạn, cache theo TTL, tự làm mới |
| 4 | Dep oauth2 | **Đồng ý thêm `golang.org/x/oauth2/google`** — dep ngoài đầu tiên đáng kể của repo (khác `sr pull` giữ zero-dep). Chốt có chủ đích |
| 5 | Secret | **env/stdin + config file `.skillrunner/fetch.json`** (đã gitignore). Ưu tiên: flag > env > config. Không log token; SA key chỉ tham chiếu qua path/env |
| 6 | Cache | Digest cache ở **`docs/context/fetch-<slug>.json`**; markdown ở **`docs/specs/<slug>.md`** (theo git, review được) |

## 4. Command & flags

```
sr fetch --from <url> [flags]
```

| Flag | Ý nghĩa | Default |
|---|---|---|
| `--from <url>` | link Confluence hoặc Google Sheet (hoặc **alias** trong config `sources`) | **bắt buộc** |
| `--out <path>` | file markdown ghi ra | `docs/specs/<slug>.md` |
| `--email <e>` | (Confluence) email cho Basic auth | từ config `confluence.email` |
| `--token-env <VAR>` | tên biến môi trường chứa token/secret | theo nguồn (xem §7) |
| `--range <A1>` | (Google) vùng/tab cần lấy (vd `Sheet1!A1:H`) | cả tab đầu / gid trong link |
| `--gid <n>` | (Google) tab id (nếu không có trong link) | từ `#gid=` |
| `--dir <project>` | repo đích (để tìm `.skillrunner/fetch.json`) | cwd |
| `--no-cache` | ép fetch lại, bỏ qua cache | off |

Nguồn được **auto-detect theo host** (§5); không cần cờ chọn loại.

## 5. Nhận diện nguồn (source detection)

| Host / pattern | Nguồn | Trích ra |
|---|---|---|
| `*.atlassian.net/wiki/spaces/.../pages/<id>/...` | Confluence | `site`, page `id` |
| `docs.google.com/spreadsheets/d/<id>/...#gid=<gid>` | Google Sheet | spreadsheet `id`, `gid` |
| alias khớp `sources[<name>]` trong config | tra ra URL thật rồi lặp lại nhận diện | — |

URL không khớp → liệt kê 2 dạng hỗ trợ rồi thoát non-zero.

## 6. Pipeline (toàn bộ 0 token)

```
1. Resolve   : nếu --from là alias → tra sources[] trong .skillrunner/fetch.json
2. Detect    : nhận diện nguồn theo host, tách id/gid
3. Auth      : lấy credential theo thứ tự flag > env > config (§7)
                 - Confluence: Basic base64(email:token)
                 - Google    : gauth.go → access token (SA JWT hoặc refresh-token), cache TTL
4. Fetch     : HTTP GET
                 - Confluence: /wiki/api/v2/pages/<id>?body-format=atlas_doc_format
                 - Google    : /spreadsheets/d/<id>/export?format=csv&gid=<gid>  (Bearer)
                               hoặc Sheets API v4 values nếu cần nhiều range
5. Normalize : ADF JSON → markdown (đoạn văn + BẢNG); CSV → markdown table (giữ header)
6. Digest    : in JSON gọn ra stdout + ghi markdown vào --out + cache vào docs/context/
```

Auth chết / token sai / trang-sheet không có quyền → báo lỗi rõ + exit != 0, **không tự đoán**.

## 7. Auth & secret model

**Config `.skillrunner/fetch.json`** (đã gitignore; giữ **tham chiếu** bí mật, không giữ raw key):

```jsonc
{
  "confluence": { "site": "acme.atlassian.net", "email": "me@acme.vn", "tokenEnv": "CONFLUENCE_TOKEN" },
  "google": {
    // chọn 1 trong 2:
    "saKeyFile": "~/.secrets/ghm-sa.json",          // service account key (share sheet cho email SA)
    "refresh": { "clientEnv": "GS_CLIENT", "secretEnv": "GS_SECRET", "refreshEnv": "GS_REFRESH" }
  },
  "sources": {                                        // alias → link (đỡ dán URL dài mỗi lần)
    "tc-login": "https://docs.google.com/spreadsheets/d/<id>/edit#gid=0",
    "spec-hr":  "https://acme.atlassian.net/wiki/spaces/HR/pages/12345/Spec"
  }
}
```

- **Thứ tự đọc:** `--flag` > **biến môi trường** > **config file**. env luôn thắng file.
- **Confluence:** token qua `CONFLUENCE_TOKEN` (hoặc `--token-env`), email qua `--email`/config.
- **Google (`internal/skill/gauth.go`):** dùng `golang.org/x/oauth2/google`
  - *service account:* đọc SA key JSON → `google.JWTConfigFromJSON` (scope
    `spreadsheets.readonly`) → `TokenSource`.
  - *refresh token:* `oauth2.Config` + `TokenSource` từ refresh token.
  - access token **cache trong RAM theo TTL (~1h)**, tự refresh khi gần hết hạn → "cấu hình một
    lần, chạy mãi".
- **An toàn secret:** không bao giờ log/ in token; SA key chỉ nạp qua đường dẫn/env; không nhét
  raw key vào config hay digest.

## 8. Output

**Markdown ghi vào `--out`** (header `<!-- generated by sr fetch — do not edit -->`):
- Tiêu đề trang / tên sheet.
- Nội dung chuẩn hoá: đoạn văn + **bảng markdown** (testcase giữ nguyên header cột).

**Digest** (stdout — thứ DUY NHẤT Claude bắt buộc đọc, mục tiêu < 2k token):

```json
{
  "source": "confluence",              // hoặc "gsheet"
  "url": "https://acme.atlassian.net/wiki/spaces/HR/pages/12345/Spec",
  "title": "Spec đăng nhập",
  "file": "docs/specs/spec-dang-nhap.md",
  "tables": [ { "name": "Testcase", "headers": ["ID","Bước","Kỳ vọng"], "rows": 24 } ],
  "sections": ["Mục tiêu", "Luồng", "Ràng buộc"],
  "contentHash": "sha256:...",
  "cachedAt": "<caller stamp>"
}
```

## 9. Cache

- **Key** = `hash(url + range/gid + contentHash)`.
- **Vị trí**: `<project>/docs/context/fetch-<slug>.json` + markdown ở `docs/specs/<slug>.md`.
- **Hit** (contentHash trùng) → in digest cache, bỏ fetch.
- **Đổi nội dung** (hash khác) → tự regen. `--no-cache` → ép chạy lại.
- Không có `Date.now()` trong logic sinh khoá; `cachedAt` do caller đóng dấu (đúng nguyên tắc
  determinism như `sr pull`).

## 10. Determinism & an toàn

- Cùng nội dung nguồn = cùng markdown/digest (sort ổn định thứ tự bảng/section).
- Chỉ ghi trong `--out` + `docs/context/`; không đụng file khác.
- Không overwrite file thiếu header `generated by sr fetch`.
- `sr fetch` **không cần approval** (deterministic, chỉ kéo nội dung). Approval vẫn ở 2 gate của
  Claude: **duyệt plan** + **duyệt commit**.

## 11. Quan hệ với skill

- `sr fetch` = phần lấy + chuẩn hoá nguồn (0 token), thay việc Claude đọc raw Confluence/Sheet.
- `spec-from-source` thêm nhánh input: "link Confluence/Sheet + token → chạy `sr fetch` trước,
  rồi đọc markdown/digest gọn". (Cập nhật `skill.json` ở P3.)
- Khi dựng/đối chiếu testcase gặp mâu thuẫn quy ước UI/UX → áp
  [`ui-ux-conflicts.md`](./ui-ux-conflicts.md) (STOP + liệt kê + người quyết).

## 12. Xử lý lỗi

| Tình huống | Hành vi |
|---|---|
| URL không nhận diện được | Liệt kê 2 dạng hỗ trợ, exit != 0 |
| Confluence 401/403 | "token/email sai hoặc không có quyền trang" + exit != 0 |
| Google 403 (chưa share cho SA) | "hãy share sheet cho email service-account: <email>" |
| Token Google hết hạn | tự refresh; nếu refresh fail → báo cấu hình SA/refresh sai |
| Thiếu config/secret | in đúng biến/khoá còn thiếu (không lộ giá trị) |

## 13. Ngoài phạm vi (out of scope)

- Không **ghi ngược** lên Confluence/Sheet (chỉ đọc; scope `readonly`).
- Không dựng spec/testcase (việc của Claude sau khi đọc digest).
- Không commit (gate của Claude).
- Không MCP (transport **CLI-first**, xem [`mcp-architecture.md`](./mcp-architecture.md) đã hoãn).

## 14. Phụ thuộc & ghi chú hiện thực

- **Dep mới:** `golang.org/x/oauth2` (+ `.../google`). Đây là ngoại lệ có chủ đích so với chủ
  trương zero-dep của `sr pull` — đổi lấy auth Google an toàn (JWT/refresh do lib lo). `go.mod`
  sẽ có nhóm dep này (và các dep bắc cầu của oauth2).
- **Confluence:** parse ADF JSON bằng stdlib `encoding/json`; chỉ cần các node `paragraph`,
  `table`/`tableRow`/`tableCell`, `heading`, `text` → markdown. Không cần lib HTML.
- **Google:** mặc định CSV export (đơn giản, đọc được sheet link-shared, đủ cho 1 tab);
  `--range`/`--all-tabs` chuyển sang **Sheets API v4** (`gsheet_v4.go`): đọc metadata (title +
  tên tab) rồi `values:batchGet` cho nhiều tab/range. V4 cần Bearer (service-account/refresh)
  hoặc `google.apiKeyEnv` (sheet public) — v4 từ chối request ẩn danh.
- **Slug hoá** tiêu đề → tên file (bỏ dấu, kebab-case) cho `--out` mặc định.
- **Phasing:** P1 Confluence + config/secret loader → P2 Google (`gauth.go` SA+refresh, CSV) →
  P3 nối `main.go` + `skill.json` → P4 doc này chuyển trạng thái "đã code" + ghi verify thật.
- **Test:** dùng fixture (ADF JSON mẫu, CSV mẫu, SA key giả) — không gọi mạng/Google thật trong
  unit test; token flow test bằng TokenSource giả.

## 15. Ghi chú hiện thực (đã code — cập nhật 2026-07-20)

- **Files:** `internal/skill/fetch.go` (orchestrate + detect + cache + ghi markdown),
  `confluence.go` (v2 API + ADF→markdown), `gsheet.go` (CSV export + parse), `gauth.go`
  (token source SA/refresh), `fetchconfig.go` (`.skillrunner/fetch.json` + secret resolve).
  `cmd/skillrunner/main.go` thêm case `fetch` + flags; `skill.json` cập nhật `spec-from-source`.
- **Dep:** `golang.org/x/oauth2` v0.36.0 (direct) + `cloud.google.com/go/compute/metadata`
  (indirect) — ngoại lệ có chủ đích so với zero-dep của `sr pull`.
- **Cache key = hash RAW payload** (JSON page / CSV) như `sr pull` hash spec; nội dung không đổi
  → tái dùng digest, bỏ normalize/ghi lại.
- **An toàn Google:** phát hiện sheet private trả về HTML 200 (trang login) → báo lỗi thay vì ghi
  rác (`looksLikeHTML`).
- **Verify:** unit test phủ ADF→markdown (heading/bold/list/table + escape `|`), CSV→markdown
  (ragged row padding, sheet trống), detect nguồn (confluence/gsheet + alias + lỗi), cache
  round-trip, guard không overwrite file lạ, token source (fake). Đường lỗi CLI đã chạy thật
  (unsupported URL, thiếu page id, thiếu token).
- **End-to-end (mock server):** `fetch_integration_test.go` chạy trọn `Fetch()` qua `httptest`:
  Confluence (Basic auth đúng → ADF parse → ghi `docs/specs/*.md` có header → digest sections →
  cache hit lần 2, server bị gọi cả 2 lần), Google Sheet link-shared (CSV → bảng markdown +
  digest table), và alias trong config. Base-URL được inject qua `confluenceBaseURL` /
  `gsheetExportBase` / `gsheetV4Base` (mặc định là host thật).
- **Verify host thật (2026-07-21):** cả hai nguồn đã kéo thật OK.
  - Google Sheet CSV (link-shared): sheet testcase "Danh mục Khoa khám bệnh" 77 dòng, bảng đúng.
  - Confluence v2 (Basic auth email+token): trang "DANH MỤC KHOA KHÁM BỆNH" → ADF→markdown 66
    dòng, 11 sections, 4 bảng trích đúng, giữ bold/list/heading.
  - Config + alias: `--from <alias>` tự đọc email/tokenEnv từ `.skillrunner/fetch.json` chạy đúng.
  - Sheets API v4 (`--range`/`--all-tabs`) verify qua mock (API-key path); host thật chờ khi cần
    nhiều tab.
  - Bài học: Confluence Basic auth phải dùng đúng email tài khoản Atlassian **sở hữu token**
    (email khác → 403 "cannot access Confluence", không phải 401).
