# Taxonomy skill hợp nhất — bể skill trung tâm

> Quyết sách: **chia skill theo MỤC ĐÍCH (verb), không theo stack.**
> Mỗi verb là **1 skill duy nhất** (phần intent/các bước). Khác biệt giữa React, Flutter,
> Kotlin, Swift, Go, React Native, ASP.NET nằm trong **rule pack riêng của từng stack**.
> `skillrunner` phát hiện stack → gộp `skill (chung) + rule pack (đúng stack)` → `emit` mệnh lệnh.

Bằng chứng cho quyết sách: hai dự án khác stack (`app-hrm` Flutter/GetX, `ghm-hrm` React)
**độc lập hội tụ về cùng một bộ verb**. Đó là cấu trúc thật của công việc, không phải trùng hợp.

---

## 1. Nguyên tắc: tách INTENT khỏi CONTENT

```
SKILL (intent, verb)      →  giống nhau mọi stack   →  viết & bảo trì 1 lần
  scaffold-data, build-ui, refactor, ...

RULE PACK (content)       →  khác theo stack        →  nơi bỏ công sức chính
  convention, template, lint, design-tokens, library-docs
```

Sai lầm cần tránh: `scaffold-data-react`, `scaffold-data-flutter`, ... → 7 stack × N skill =
bùng nổ bảo trì. Đúng: `scaffold-data` (1 bản) + 7 rule pack.

---

## 2. Skill CORE — generic 100%, viết một lần, dùng mọi stack

Không gắn stack. Không cần rule pack (hoặc chỉ cần pack rất mỏng).

| Skill | Mục đích | Nguồn gốc |
|-------|----------|-----------|
| `plan-feature` | Yêu cầu → plan + goals cho user duyệt (không code) | working-policy + skillrunner |
| `spec-from-source` | Jira/Confluence/Swagger → spec có cấu trúc trước khi code | app-hrm `atlassian-spec`, `swagger-import` |
| `commit` | Chia commit theo module, trình bày, xác nhận trước khi commit | app-hrm `commit` |
| `check-diff` | Review `git diff` theo convention → báo `file:line` + chạy lint | ghm `check-convention`, app `clean-code` |
| `refactor` | Cải thiện trong phạm vi, KHÔNG đổi hành vi | cả hai `refactor`/`clean-component` |
| `explain-lib` | Giải thích cách project THỰC SỰ dùng một lib (từ docs cong sẵn) | ghm `explain-lib` + `libraries/` |
| `working-policy` | Kỷ luật: hỏi khi mơ hồ · file plan trước khi code · minimal change · verify bằng git | app `working-policy` |

> `working-policy` là **cross-cutting**: mọi skill khác kế thừa nó (giống `rules` chung trong skill.json).

---

## 3. Skill STACK-PARAMETRIZED — 1 intent, content theo pack

Ma trận verb × stack. Ô = tên hiện có ở project mẫu (nếu có), trống = cần viết pack.

| Skill (intent) | Mục đích | React | Flutter | Kotlin | Swift | Go | RN | ASP.NET |
|----------------|----------|-------|---------|--------|-------|----|----|---------|
| `scaffold-data` | Dựng data layer từ API/Swagger | `add-api` | `add-data`,`swagger-import` | — | — | — | — | — |
| `scaffold-screen` | Dựng màn hình/feature theo kiến trúc | `new-list`,`new-dialog` | `add-feature` | — | — | — | — | — |
| `build-ui` | Dựng/sửa UI theo design-system | `gen-components/*` | `fix-ui` | — | — | — | — | — |
| `gen-routes` | Sinh route theo cấu trúc file | (react-router) | (GetX routes) | — | — | — | — | — |

Trống không có nghĩa là thiếu skill — skill vẫn dùng chung; chỉ cần **viết rule pack** cho stack đó.

---

## 4. Một RULE PACK gồm những gì (khuôn chung cho mọi stack)

Mỗi pack (vd `packs/react.json`, `packs/flutter.json`) khai báo:

- **architecture** — luồng tầng (data → logic → view) và trách nhiệm mỗi tầng
- **conventions** — quy tắc đặt tên, cấu trúc thư mục, i18n, format
- **lint** — chuẩn code-style + cách chạy linter của stack
- **templates** — mẫu file cho `scaffold-*` (data layer, screen, component)
- **design-system / tokens** — component dùng chung, màu/spacing token
- **library-docs** — index các lib project thực dùng (chống bịa API) — theo mẫu `libraries/` của ghm-hrm
- **detect** — dấu hiệu nhận diện stack (xem §6)

---

## 5. Hai pack đầu tiên (trích từ project mẫu)

### 5a. `react` pack (từ ghm-hrm)
- **architecture:** `src/api/{resource}.ts` (data) → `handler.tsx` (logic hook) → `index.tsx` (view)
- **stack:** React 18 + TS + Vite, TanStack Query/Table, React Hook Form, CASL (permission), design system `@ghm`
- **conventions bắt buộc:**
  - luôn `return res` để message BE tới được toast
  - `useConfirm` cho delete / icon-only / auto-save / thoát form dirty
  - global search debounce 500ms, reset page 1, server-side
  - tránh race: nhét mọi param vào react-query `queryKey`
  - table 3 dạng: server paginated (skip/take) · nhỏ (`pageCount:-1`) · infinite (`useInfiniteQuery`)
  - validate RHF: title ≤100, description ≤250, trim
- **templates:** `add-api` (types+service+query hooks) · `new-list` · `new-dialog` · 9 gen-components pattern
- **library-docs:** đã có sẵn thư mục `docs/reference/libraries/` — bê nguyên

### 5b. `flutter` pack (từ app-hrm)
- **architecture:** View → Controller → Repository → Provider → native ForgeRock (`AuthMethodChannel`)
- **stack:** Flutter + GetX (binding/controller/page/repository), ScreenUtil, GetStorage, multi-brand + multi-flavor
- **conventions bắt buộc:**
  - API CHỈ qua `AuthMethodChannel` — cấm dio/http
  - entrypoint thật là `main_{dev,staging,prod}.dart`
  - không hardcode màu — dùng semantic token (5 tầng: primitives→semantic→ThemeController→ThemeData)
  - kích thước qua ScreenUtil; string UI tiếng Việt; `log` thay `print`; ưu tiên `const`
  - build/run qua `scripts/flutter_flavor.sh` theo flavor
- **templates:** `add-data` (Provider/Model/Repository) · `add-feature` (binding/controller/page/route) · `fix-ui`
- **library-docs / references:** architecture, conventions, getx-conventions, theme-tokens, toolkit, rules

---

## 6. "Tự động phân chia" = detect stack (việc xác định, không cần suy luận)

`skillrunner detect` quét file dấu hiệu và chọn pack:

| File dấu hiệu | Stack | Pack |
|---------------|-------|------|
| `pubspec.yaml` | Flutter | `flutter` |
| `package.json` chứa `react-native` | React Native | `rn` |
| `package.json` chứa `react` (không RN) | React | `react` |
| `build.gradle(.kts)` + Kotlin/Java | Android native | `kotlin` |
| `*.xcodeproj` / `Package.swift` | iOS Swift | `swift` |
| `go.mod` | Go | `go` |
| `*.csproj` / `*.sln` | ASP.NET | `dotnet` |

Luồng: `detect` → chọn pack → `emit <skill>` gộp **skill (chung) + rules (pack)** → Claude thực thi.

---

## 7. Ánh xạ vào skillrunner (đã dựng)

- `skills` trong `skill.json` = mục §2 + §3 (verb dùng chung) — **viết một lần**.
- `rules` groups = rule pack theo stack (§4–§5) — có thể tách thành `packs/<stack>.json`.
- Thêm lệnh `skillrunner detect` (§6) để tự chọn pack; `emit` tự nạp pack tương ứng.
- `working-policy` → đưa vào `rules` chung, mọi skill `appliesRules` kế thừa.

---

## 8. Lộ trình

1. ✅ Khung skillrunner (skill chung + rules + emit) — đã có.
2. ✅ Pack `react` + `flutter` (trích từ 2 project mẫu).
3. ✅ `skillrunner detect` + tách `packs/<stack>.json`.
4. ✅ Skill core generic (`plan-feature`, `spec-from-source`, `commit`, `refactor`, `explain-lib`, `check-diff`).
5. ✅ Pack `go`, `dotnet`, `kotlin`, `swift`, `rn` (soạn theo best-practice từng stack — chưa có project mẫu, cần tinh chỉnh khi áp thật).
6. ▶ Áp thử vào `Flutter_weather` (đang trống) để kiểm chứng "kéo vào là chạy, không tái cấu trúc".
7. ▶ Thêm `go test` cho `Detect`/`Emit` (đảm bảo output xác định).

> Lưu ý: pack `go`/`dotnet` là **backend** — nhóm `design-system` được diễn giải thành "quy ước API contract"
> (response envelope, status code, OpenAPI), không phải UI. Skill `build-ui` ít dùng cho hai stack này.

---

## 9. Rủi ro đã ghi nhận

1. **Không nhân bản skill theo stack** — chỉ nhân bản *rule pack*.
2. **Công sức thật nằm ở rule pack**, không ở việc chẻ skill; intent ít và ổn định.
3. **Skill sinh code luôn phụ thuộc pack** ở đầu ra → pack là bắt buộc với `scaffold-*`, `build-ui`.
4. **Skill core generic** viết một lần, không gắn stack.
5. Pack phải giữ **library-docs chính xác** (chống bịa API) — theo mẫu `libraries/` của ghm-hrm.
