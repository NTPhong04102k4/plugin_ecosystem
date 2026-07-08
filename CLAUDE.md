# CLAUDE.md

## skillrunner — cách dùng trong mọi phiên

Repo này là **bể skill trung tâm**: một binary Go `skillrunner` + `skill.json` (skill dùng
chung) + `packs/<stack>.json` (rule riêng theo stack). Nó KHÔNG tự suy luận — nó phát hiện
stack và in ra "mệnh lệnh" (rules + các bước) để **bạn (Claude) thực thi**.

Kiến trúc: **1 skill dùng chung + rule pack theo stack**. Đổi dự án chỉ đổi pack, không đổi skill.

### Khi người dùng yêu cầu một việc khớp với một skill

1. Phát hiện stack: `./bin/skillrunner detect --dir <project>` (hoặc chạy trong project).
2. Xem skill: `./bin/skillrunner list`
3. Lấy mệnh lệnh: `./bin/skillrunner emit <skill> --dir <project>`
   - Tự động detect stack và gộp `packs/<stack>.json`.
   - Ép pack thủ công: `--pack react` (hoặc flutter/go/...).
4. **Đọc và làm theo** output — tuân thủ mục "Rules you MUST follow".
5. Nếu có `[needs approval]` / "Approval gate": chỉ **đề xuất plan/goal** rồi DỪNG chờ duyệt,
   không sửa file trước khi được đồng ý.
6. Nếu thấy cảnh báo `⚠ Stack rule groups not loaded`: pack cho stack đó chưa có — báo người
   dùng hoặc tạo `packs/<stack>.json` trước khi tiếp tục.

### Cấu trúc
- `skill.json` — skill dùng chung (core generic + parametrized). Sửa một lần.
- `packs/react.json`, `packs/flutter.json` — rule theo stack (architecture/conventions/lint/
  templates/design-system/library-docs). Thêm stack mới = thêm 1 file pack.
- JSON cho phép comment `//`.

### Lệnh khác
- `skillrunner validate` — kiểm manifest
- `skillrunner init` — tạo skill.json mẫu trong dự án mới

Tài liệu: `docs/skill-runner-design.md` (thiết kế), `docs/skill-taxonomy.md` (bể skill hợp nhất).
