# `sr ui` & `sr refresh` — web cục bộ quản lý credential/phiên và chạy fetch

> Trạng thái: **ĐÃ CÓ CODE** (`internal/uiserver/`, `internal/uistore/`).
> Doc này là **hướng dẫn dùng + mô tả tác dụng**. Bản thiết kế & các quyết định đã khoá nằm ở
> [`sr-ui-design.md`](./sr-ui-design.md); phần fetch bên dưới nằm ở [`sr-fetch-design.md`](./sr-fetch-design.md).

## 1. Nó giải quyết chuyện gì

`sr fetch` chạy được bằng CLI, nhưng để dùng lâu dài thì vướng: token phải đặt env, link phải dán
tay mỗi lần, nhiều vùng Confluence khác nhau, nhiều tính năng khác nhau. `sr ui` là **lớp vỏ web
cục bộ bọc quanh `sr fetch`** để:

- khai báo **credential một lần** (gọi là **tag**), dùng lại cho nhiều tính năng;
- gom link theo **phiên** (session) = một tính năng đang làm;
- bấm **Run/Refresh** → sinh markdown vào repo, có **history** để tra lại.

Nó **không** gọi Claude, **không** tốn token. Nó chỉ là bộ mặt của làn 0-token.

```
   Trình duyệt (127.0.0.1:7777)
        │  chọn repo → khai tag → gom link vào phiên → Run
        ▼
   sr ui (Go, SPA nhúng go:embed)
        │  đọc/ghi <repo>/.skillrunner/ui.json   (0600, auto-gitignore)
        ▼
   skill.Fetch(...)  ← ĐÚNG hàm mà `sr fetch` CLI gọi
        │
        ├─►  <repo>/docs/specs/<slug>.md          ← AI agent đọc file này
        ├─►  <repo>/docs/context/fetch-<slug>.json ← cache digest
        └─►  history trong ui.json
```

---

## 2. Chạy

```bash
sr ui                 # mặc định http://127.0.0.1:7777
sr ui --port 8123     # đổi cổng
```

In ra URL rồi mở bằng trình duyệt. Dừng bằng `Ctrl-C`.

- **Bind cứng `127.0.0.1`** — không listen ra ngoài mạng, không cấu hình nào đổi được.
- **Không đăng nhập, không master password.** Bảo vệ bằng: chỉ localhost + nonce (§6).
- Server **stateless**: mọi request đều mang `?dir=<đường dẫn repo tuyệt đối>`; đổi repo trên UI
  không cần restart.

---

## 3. Ba khái niệm

| Khái niệm | Là gì | Lưu ở đâu |
|---|---|---|
| **Tag** | Một **vùng credential Confluence**: `{site, email, token}`. Ví dụ tag `X` = workspace khách hàng A, tag `Y` = workspace nội bộ. **Tái dùng** cho nhiều phiên. | `ui.json → tags` |
| **Session (phiên)** | Một **tính năng đang làm**: chọn 1 tag + N link Confluence + N link Google Sheet. | `ui.json → sessions` |
| **History** | Log các lần Run/Refresh: phiên nào, lúc nào, sinh ra file gì, thành công không. Giữ **100 bản ghi gần nhất**. | `ui.json → history` |

Tách tag ⇄ session vì cùng một credential nhưng hôm nay làm tính năng X, mai làm tính năng Y —
không muốn khai lại token, cũng không muốn trộn lẫn link của hai tính năng.

---

## 4. Luồng dùng thực tế

1. **Chọn repo** — dùng file browser trong UI (`/api/browse`, duyệt phía server, có đánh dấu thư
   mục nào là git repo). Phải chọn phía server vì JavaScript trong trình duyệt **không** lấy được
   đường dẫn tuyệt đối.
2. **Tạo tag** — nhập `site` (vd `ghmsoftjsc.atlassian.net`), `email`, `token` (API token Atlassian).
3. **Kiểm tra tag** — nút Check gọi Confluence thật và trả về email của tài khoản → biết ngay token
   đúng/sai, không phải đợi tới lúc Run mới lòi lỗi.
4. **Tạo phiên** — đặt tên (vd `khoa-kham-benh`), chọn tag, dán các link Confluence + link Google Sheet.
5. **Run** (dùng cache nếu nội dung không đổi) hoặc **Refresh** (ép fetch lại).
6. Đọc kết quả: `docs/specs/*.md` trong repo → đây là thứ Claude/agent sẽ đọc.

### Chạy lại phiên bằng CLI (không cần mở web)

```bash
cd <repo>
sr refresh --session khoa-kham-benh
```

`sr refresh` = **đúng logic Run của UI với `no-cache` bật sẵn**. Nó đọc `ui.json` của repo hiện
tại (`--dir`), fetch lại toàn bộ link của phiên, in `✓ <url> → <file>` từng dòng, và **cũng ghi
history** như bấm trên web. Hợp cho cron/script/CI cục bộ.

---

## 5. `ui.json` — file cấu hình

Đường dẫn: **`<repo>/.skillrunner/ui.json`**. **Auto-ghi bằng code, không sửa tay.**

```jsonc
{
  "tags": {
    "X": { "site": "ghmsoftjsc.atlassian.net", "email": "ai@do.vn", "token": "ATATT..." }
  },
  "sessions": {
    "khoa-kham-benh": {
      "tag": "X",
      "confluence": ["https://ghmsoftjsc.atlassian.net/wiki/spaces/KT/pages/2693562387/..."],
      "sheets":     ["https://docs.google.com/spreadsheets/d/1ZDA.../edit#gid=933348279"]
    }
  },
  "history": [
    { "session": "khoa-kham-benh", "at": "2026-07-27T03:12:00Z",
      "files": ["docs/specs/quy-trinh-kham.md"], "ok": true }
  ]
}
```

**Tác dụng phụ khi ghi file này** (biết trước để khỏi ngạc nhiên):

- Tạo `<repo>/.skillrunner/` với quyền `0700`.
- Ghi **atomic** (temp file + rename) ở quyền `0600` — không bao giờ để lại file nửa vời.
- **Tự thêm `.skillrunner/` vào `<repo>/.gitignore`** nếu chưa có, kèm dòng chú thích. Đây là
  hàng rào duy nhất giữ token khỏi bị commit → **đừng gỡ dòng đó ra**.
- Sửa/xoá tag hay session đều đọc-sửa-ghi lại nguyên file.

**Về token:** lưu **plaintext**. Đây là quyết định có chủ đích (design §3 khoản 4): đổi lại là
không thêm dependency mã hoá và không phải nhớ master password. Mức tin cậy ngang `config.yaml`
của authswagger. Bù lại bằng `0600` + auto-gitignore.

Một chi tiết dễ chịu: **sửa tag mà để trống ô token → token cũ được giữ nguyên**, không bị xoá.
Nên có thể đổi `site`/`email` mà không phải dán lại token.

---

## 6. Bảo mật — nó chặn cái gì

| Mối lo | Cách chặn |
|---|---|
| Máy khác trong LAN gọi vào | Bind cứng `127.0.0.1` |
| Một trang web bất kỳ bạn đang mở gọi lén API cục bộ (CSRF) | Mỗi lần khởi động sinh **nonce ngẫu nhiên 16 byte**, nhúng vào SPA; mọi request `/api/*` phải gửi header `X-SR-UI: <nonce>`, sai → **403** |
| Trang khác gắn Origin lạ | `Origin` (nếu có) phải là `localhost` / `127.0.0.1` / `::1`, khác → **403** |
| Token lộ qua API đọc cấu hình | `GET /api/config` **redact** token, chỉ trả `hasToken: true/false` |
| Token bị commit | auto-gitignore `.skillrunner/` (§5) |

Nonce **đổi mỗi lần khởi động** → tab web cũ sau khi restart `sr ui` sẽ nhận 403; **reload trang**
là xong.

Không chặn: người dùng khác trên **cùng máy** cùng user (quyền `0600` chỉ chặn user khác), và
đường dẫn `?dir=` — server thao tác trên **bất kỳ** thư mục nào bạn trỏ tới.

---

## 7. API (dành cho ai muốn script hoặc sửa SPA)

Mọi endpoint dưới `/api` cần header `X-SR-UI: <nonce>` và query `?dir=<repo tuyệt đối>`
(trừ `/api/browse` chỉ cần `?path=`).

| Method | Path | Việc |
|---|---|---|
| `GET` | `/` | SPA (một file HTML nhúng sẵn, không cần Node) |
| `GET` | `/api/browse?path=` | Liệt kê **thư mục con** (ẩn dotfolder, trừ `.skillrunner`), đánh dấu `isGitRepo` |
| `GET` | `/api/config` | tags (đã redact token) + sessions + history |
| `POST` | `/api/tags` | Thêm/sửa tag `{tag, site, email, token}` |
| `DELETE` | `/api/tags/{tag}` | Xoá tag |
| `POST` | `/api/tags/{tag}/check` | Gọi Confluence thật để xác thực → `{valid, accountEmail}` |
| `POST` | `/api/sessions` | Thêm/sửa phiên `{session, tag, confluence[], sheets[]}` |
| `DELETE` | `/api/sessions/{name}` | Xoá phiên |
| `POST` | `/api/run` | Chạy phiên `{session}` (**dùng cache**) |
| `POST` | `/api/refresh` | Chạy phiên `{session}` (**bỏ cache**) |

`/api/run` và `/api/refresh` trả `results[]`, mỗi phần tử là `{url, ok, error?, item?}` với `item`
là digest fetch (`source`, `title`, `file`, `tables[]`, `sections[]`, `contentHash`, `cachedAt`).

---

## 8. Giới hạn hiện tại (MVP)

- **Phạm vi Run = fetch thôi.** Không tự sinh testcase, không gọi Claude. Sinh xong markdown là hết
  việc của `sr ui`; bước dùng não là của agent đọc `docs/specs/`.
- **Google Sheet phải là link-shared** — nhánh này fetch **không kèm credential**. Sheet riêng tư
  cần Sheets API v4 thì phải chạy `sr fetch` CLI với cấu hình `google` trong
  `.skillrunner/fetch.json` (xem [`sr-fetch-design.md`](./sr-fetch-design.md)).
- **Chỉ tag Confluence.** Không có chỗ khai credential cho nguồn khác.
- **Không sửa được nội dung** đã fetch trong UI — muốn khác thì sửa file markdown trong repo.
- `.skillrunner/ui.json` (dùng cho UI) và `.skillrunner/fetch.json` (dùng cho CLI) là **hai file
  riêng, không đồng bộ với nhau**. Khai token trên UI **không** làm `sr fetch` CLI chạy được và
  ngược lại.

---

## 9. Troubleshooting

| Triệu chứng | Nguyên nhân & cách xử lý |
|---|---|
| Bấm gì cũng **403** | Nonce cũ (đã restart `sr ui`). **Reload trang**. |
| `missing ?dir= (pick a repo first)` | Chưa chọn repo trong UI. |
| `repo dir not found` | Đường dẫn không tồn tại hoặc không phải thư mục. |
| Check tag báo lỗi | Token Atlassian sai/hết hạn, hoặc `site` sai (phải là host `*.atlassian.net`, không phải link trang). |
| `session ... references unknown tag` | Tag đã bị xoá nhưng phiên vẫn trỏ vào. Gán lại tag cho phiên. |
| Sheet fetch lỗi 401/403 | Sheet chưa bật link-shared. Xem §8. |
| Run xong không thấy file | File nằm ở **`<repo>/docs/specs/`**, không phải thư mục đang chạy `sr ui`. |
| Đổi tag/phiên xong bị mất | Kiểm tra quyền ghi `<repo>/.skillrunner/` (cần `0700` của chính bạn). |
| Cổng bận | `sr ui --port <khác>`. |

---

## 10. Liên quan

- [`sr-ui-design.md`](./sr-ui-design.md) — thiết kế & các quyết định đã khoá
- [`sr-fetch-design.md`](./sr-fetch-design.md) — `sr fetch`: nguồn, auth, digest
- [`sr-serve-mcp.md`](./sr-serve-mcp.md) — chế độ MCP server (khác hẳn, không liên quan web UI)
- Code: `internal/uiserver/server.go` (HTTP), `runner.go` (chạy phiên), `internal/uistore/uistore.go` (config)
