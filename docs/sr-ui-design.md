# Thiết kế: `sr ui` — web cục bộ quản lý tag/phiên + chạy `sr fetch`

> Trạng thái: **ĐÃ CHỐT (frozen)** — chờ code (đã duyệt để bắt đầu). Ngày chốt: 2026-07-21.

## 1. Mục tiêu & vấn đề

Hiện `sr fetch` chạy bằng CLI + `.skillrunner/fetch.json` sửa tay + env token. Muốn dùng lâu dài
cho nhiều vùng Confluence (X, Y…) và nhiều tính năng thì cần một **giao diện dễ dùng**: chọn repo,
khai báo credential một lần, gom link theo từng tính năng, bấm chạy, xem lịch sử.

`sr ui` là **web app cục bộ** (localhost) bọc quanh `sr fetch`. Không đăng nhập/mật khẩu. Mọi cấu
hình được **auto-ghi vào config trong chính repo** (`<repo>/.skillrunner/ui.json`), không sửa tay.

## 2. Mô hình

```
Repo (chọn bằng file browser → path tuyệt đối)
└── <repo>/.skillrunner/ui.json                (auto-ghi, gitignored, 0600)
    ├── tags:      X = {site, gmail, token}     ← vùng credential, DÙNG LẠI nhiều lần
    │              Y = {site, gmail, token}
    ├── sessions:  "feature-X" = {tag:X, confluence:[...], sheets:[...]}   ← 1 phiên = 1 tính năng
    │              "feature-Y" = {tag:Y, confluence:[...], sheets:[...]}
    └── history:   [ {session, at, files, ok} ]
          │
          ▼ Run/Refresh phiên
   sr fetch (0 token) → docs/specs/*.md  ← AI agent đọc
```

- **Tag** = credential một vùng Confluence `{site, gmail, token}`, tái dùng cho nhiều phiên.
- **Session (phiên)** = một tính năng: chọn 1 tag + N link Confluence + N link Excel. Chia phiên
  rõ ràng vì cùng một tag lúc làm tính năng X, lúc làm Y.
- **History** = log các lần chạy (thời điểm, file sinh ra) để tra cứu.

## 3. Quyết định đã khóa

| # | Quyết định | Chốt |
|---|---|---|
| 1 | Dạng UI | **Web cục bộ**, `sr ui`, Go serve HTTP **bind 127.0.0.1**, SPA nhúng `go:embed` (HTML/CSS/JS thuần, không Node) |
| 2 | Đăng nhập | **KHÔNG** master password/session. Server stateless; frontend gửi kèm `dir` (repo path) mỗi request |
| 3 | Lưu cấu hình | **`<repo>/.skillrunner/ui.json`** — auto-ghi bằng code, không sửa tay |
| 4 | Token | **Plaintext** trong ui.json. An toàn bằng: auto thêm `.skillrunner/` vào `<repo>/.gitignore` + đặt quyền `0600` |
| 5 | Chọn repo | **File browser server-side** (`/api/browse`) → lấy path tuyệt đối chuẩn (web JS không lấy được path) |
| 6 | Google Sheet | **Link-shared** (không auth) |
| 7 | Phạm vi Run | **MVP = fetch** các link của phiên → markdown + digest; không tự sinh testcase |
| 8 | Dep | Không thêm `x/crypto` (bỏ mã hoá). Chỉ stdlib + `x/oauth2` (đã có, cho v4 khi cần) |

## 4. Config schema — `<repo>/.skillrunner/ui.json`

```jsonc
{
  "tags": {
    "X": { "site": "ghmsoftjsc.atlassian.net", "email": "gmailX@...", "token": "ATATT..." }
  },
  "sessions": {
    "khoa-kham-benh": {
      "tag": "X",
      "confluence": ["https://ghmsoftjsc.atlassian.net/wiki/spaces/KT/pages/2693562387/..."],
      "sheets":     ["https://docs.google.com/spreadsheets/d/1ZDA.../edit#gid=933348279"]
    }
  },
  "history": [
    { "session": "khoa-kham-benh", "at": "<stamp>", "files": ["docs/specs/....md"], "ok": true }
  ]
}
```

## 5. Lệnh CLI

```
sr ui [--port 7777]                 Mở web quản lý (localhost); in URL, có thể auto-open
sr refresh --dir <repo> --session s Fetch lại (no-cache) mọi link của phiên (bắt tài liệu update)
```

## 6. HTTP API (đều yêu cầu header nonce, xem §12; server bind 127.0.0.1)

| Method | Path | Vào | Ra |
|---|---|---|---|
| GET | `/` | — | SPA |
| GET | `/api/browse?path=` | path | `{path, parent, entries:[{name,isDir,isGitRepo}]}` |
| GET | `/api/config?dir=` | repo | `{tags:{name:{site,email,hasToken}}, sessions, history}` — **không trả token** |
| POST | `/api/tags?dir=` | `{tag,site,email,token}` | ghi ui.json (+gitignore+0600); `{ok}` |
| DELETE | `/api/tags/{tag}?dir=` | — | `{ok}` |
| POST | `/api/tags/{tag}/check?dir=` | — | validate qua Confluence `/user/current` → `{valid, accountEmail}` |
| POST | `/api/sessions?dir=` | `{session,tag,confluence[],sheets[]}` | `{ok}` |
| DELETE | `/api/sessions/{s}?dir=` | — | `{ok}` |
| POST | `/api/run?dir=` | `{session}` | fetch hết link → `{results:[digest], files:[...]}` + ghi history |
| POST | `/api/refresh?dir=` | `{session}` | như run nhưng no-cache |
| GET | `/api/history?dir=` | repo | `[{session,at,files,ok}]` |

## 7. File browser (`/api/browse`)

Duyệt thư mục phía server (vì web JS không lấy được path tuyệt đối). Trả `parent` + danh sách
thư mục con; đánh dấu `isGitRepo` (có `.git`) để dễ nhận repo. Frontend điều hướng, bấm "Chọn" →
giữ path tuyệt đối, gửi kèm `dir` cho mọi API sau đó. Chỉ liệt kê thư mục (bỏ file thường).

## 8. Auto-writer + an toàn

Khi ghi ui.json (`internal/uistore`):
1. `MkdirAll <repo>/.skillrunner` (0700).
2. Ghi `ui.json` với quyền **0600**.
3. **Đảm bảo `.gitignore` của repo có dòng `.skillrunner/`** — nếu chưa, append (idempotent). Chống
   lỡ commit token.
4. Ghi atomically (ghi file tạm rồi rename) để không hỏng config khi lỗi giữa chừng.

## 9. Run / Refresh

Cho mỗi phiên: lấy tag → `{site,email,token}`; với mỗi link Confluence/Sheet gọi `skill.Fetch`
(truyền **token trực tiếp** — thêm `FetchOptions.Token`, tái dùng `Email`; Sheet link-shared khỏi
token). Kết quả: markdown ghi `<repo>/docs/specs/`, digest trả về UI, và **append history**.
`refresh` = chạy lại với `NoCache:true` để bắt tài liệu vừa cập nhật (khỏi fix tay).

## 10. Tích hợp đọc tài liệu (skillrunner)

Khi Claude cần đọc spec/testcase trong repo có `ui.json`, nó nhìn `sessions[].files`
(qua history/`docs/specs/`) để **tự trỏ vào markdown đã fetch** thay vì đọc raw. Nghi ngờ tài liệu
cũ → gợi ý `sr refresh --session <s>`. (Cập nhật hướng dẫn ở `skill.json`/CLAUDE.md ở P5.)

## 11. UI — 3 màn (SPA nhúng, theme-aware)

1. **Project** — file browser chọn repo (hiện ✓ nếu là git repo); path đang chọn hiển thị trên đầu.
2. **Tags** — bảng vùng credential; form thêm/sửa `{tên, site, gmail, token}`; nút "Check" (validate);
   token nhập vào không hiển thị lại (chỉ hiện ●●●/`hasToken`).
3. **Run** — chọn/tạo session `{tên, tag, textarea N link Confluence, textarea N link Excel}`; nút
   **Run** + **Refresh**; khu kết quả (title, file, #bảng/#section) + **bảng lịch sử** các lần chạy.

## 12. Bảo mật

- **Bind 127.0.0.1** (không phơi mạng).
- **Chống CSRF từ web khác**: kiểm `Origin`/`Referer` phải là chính server; thêm **nonce** ngẫu
  nhiên sinh lúc khởi động, nhúng vào SPA, mọi `/api` phải gửi kèm header `X-SR-UI` = nonce.
- **Token không bao giờ trả ra browser** (API chỉ trả `hasToken`/trạng thái).
- Token plaintext trên đĩa nhưng `0600` + `.gitignore` tự thêm. (Có thể thêm mã hoá tuỳ chọn sau.)
- `/api/browse` chỉ đọc, chỉ liệt kê tên thư mục.

## 13. Determinism & phạm vi

- Fetch vẫn deterministic + cache theo hash như `sr fetch`.
- **Ngoài phạm vi:** không login/đa người dùng; không sinh testcase (chỉ fetch); không auth Google
  (link-shared); không sửa tài liệu nguồn; không expose ra ngoài localhost.

## 14. Phụ thuộc & phasing

- **Không dep mới** (bỏ x/crypto). Dùng `net/http`, `html/template` không cần; SPA tĩnh nhúng
  `embed`. `skill.Fetch` tái dùng, thêm `FetchOptions.Token`.
- **P1** `internal/uistore/` — đọc/ghi ui.json (tags/sessions/history) + auto-writer (0600 + gitignore) + test.
- **P2** runner: nối `skill.Fetch` (thêm `FetchOptions.Token`) + append history + lệnh `sr refresh` + test.
- **P3** server `internal/uiserver/` — browse/config/tags/sessions/run/refresh + nonce + Origin check; lệnh `sr ui`.
- **P4** SPA nhúng (3 màn) + `go:embed`.
- **P5** docs (mục "đã code" + verify) + cập nhật `skill.json`/CLAUDE.md cho integration đọc tài liệu.
