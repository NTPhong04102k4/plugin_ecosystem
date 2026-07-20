# Xung đột: Confluence / testcase ⟷ quy ước rules UI/UX

> Trạng thái: **THAM CHIẾU (living doc)** — cập nhật khi phát hiện loại xung đột mới.
> Ngày tạo: 2026-07-20.

## 1. Bối cảnh & vấn đề

Khi làm việc từ **spec/testcase trên Confluence** (qua `spec-from-source` → `deliver-feature`),
nội dung nghiệp vụ đôi khi **mâu thuẫn** với **quy ước UI/UX của repo** — tức các nhóm rule
`design-system` và `conventions` trong `packs/<stack>.json` (react/flutter/rn…).

Ví dụ Confluence viết "bấm **Xoá** là xoá ngay", nhưng rule react bắt buộc `useConfirm` cho hành
động delete. Cả hai đều "đúng" theo nguồn của nó → cần một chỗ ghi rõ **các loại xung đột** và
**cách xử lý** để không âm thầm chọn bừa một bên.

**Nguyên tắc xử lý (đã chốt): KHÔNG tự quyết.** Khi phát hiện xung đột, Claude phải:

1. **Dừng (STOP)** — không code phần đang mâu thuẫn.
2. **Liệt kê** từng xung đột theo mẫu ở §4 (rule nói gì / Confluence-testcase nói gì).
3. **Đề xuất** phương án mặc định (cột "Đề xuất" ở §3) kèm lý do.
4. Chờ **người quyết** từng cái, rồi mới tiếp tục.

Không có bên nào "mặc định thắng". Đề xuất chỉ là gợi ý để người quyết nhanh.

## 2. Xung đột đến từ đâu (checklist khi đọc spec)

- **Testcase mô tả tương tác** (bấm nút → kết quả) trái với recipe tương tác của design-system.
- **Số liệu cứng trong spec** (giới hạn ký tự, số dòng, thời gian) khác với `conventions`.
- **Mockup/ảnh đính kèm** chỉ định control, màu, khoảng cách cụ thể trái với `design-system`.
- **Spec bỏ sót** một trục mà rule bắt buộc (quyền/CASL, dark mode, đa brand, accessibility) →
  không phải "mâu thuẫn trực diện" nhưng là **coverage gap**, cũng phải nêu.

## 3. Danh mục loại xung đột

Bám sát rule hiện có trong `packs/react.json`, `packs/flutter.json`, `packs/rn.json`.

### 3.1. Tương tác & phản hồi

| # | Rule UI/UX | Điều Confluence/testcase hay yêu cầu | Đề xuất mặc định |
|---|------------|--------------------------------------|------------------|
| A1 | `useConfirm` cho **delete, icon-only, auto-save, thoát form dirty** | "Bấm Xoá là xoá ngay", "thoát form không hỏi lại" | Giữ `useConfirm` (tránh mất dữ liệu ngoài ý muốn); nếu spec cố tình bỏ confirm phải nêu rõ. |
| A2 | **KHÔNG** dùng `useConfirm` cho nút **create/update có nhãn** | "Bấm Lưu → hiện popup xác nhận rồi mới lưu" | Bỏ confirm ở nút có nhãn theo rule; xác nhận với người quyết. |
| A3 | Luôn `return res` để **message của backend** hiện lên toast | Testcase khẳng định một chuỗi thông báo cứng ("Lưu thành công") | Ưu tiên message backend; nếu testcase kỳ vọng chuỗi cố định → cần backend đảm bảo hoặc chấp nhận sai lệch. |

### 3.2. Danh sách, tìm kiếm, phân trang

| # | Rule UI/UX | Điều Confluence/testcase hay yêu cầu | Đề xuất mặc định |
|---|------------|--------------------------------------|------------------|
| B1 | Search **debounce 500ms, server-side**, 1 ô `keyword` | "Gõ tới đâu lọc ngay tới đó", search nhiều trường ở client | Giữ debounce server-side; testcase "ra ngay" cần chỉnh kỳ vọng (có độ trễ 500ms). |
| B2 | Bảng có **3 dạng**: server-paginated (skip/take), nhỏ không phân trang (`pageCount:-1`), infinite | "Hiển thị toàn bộ bản ghi trên một trang" | Chọn dạng theo khối lượng dữ liệu; nếu spec đòi "tất cả" mà data lớn → xung đột hiệu năng, nêu rõ. |
| B3 | Mọi query param phải vào `queryKey` (tránh stale) | (spec thường không nói) — mâu thuẫn khi spec mô tả cache/giữ kết quả cũ | Theo rule (không giữ kết quả stale). |

### 3.3. Form & validation

| # | Rule UI/UX | Điều Confluence/testcase hay yêu cầu | Đề xuất mặc định |
|---|------------|--------------------------------------|------------------|
| C1 | `title ≤ 100`, `description ≤ 250`, **trim input**, hiện `Field.ErrorText` | Spec ghi giới hạn khác (vd title 200), hoặc "giữ nguyên khoảng trắng đầu/cuối" | Nêu chênh lệch con số; nếu spec là yêu cầu nghiệp vụ thật → cập nhật rule/độ dài, đừng im lặng đổi. |
| C2 | Validate bằng **React Hook Form** | Testcase mô tả kiểu báo lỗi/timing khác (blur vs submit) | Theo cơ chế RHF của repo; điều chỉnh mô tả testcase. |

### 3.4. Design system & styling

| # | Rule UI/UX | Điều Confluence/testcase hay yêu cầu | Đề xuất mặc định |
|---|------------|--------------------------------------|------------------|
| D1 | **Không hardcode màu** — dùng design token/class (react/flutter/rn đều vậy) | Mockup ghi mã hex/màu cụ thể không có token tương ứng | Map sang token gần nhất; nếu không có token → nêu để bổ sung token, không hardcode. |
| D2 | Dùng **component nội bộ** (@ghm: Dialog/Portal, BaseSelect/MultiCombobox, FileUpload, Field) | Mockup dùng control khác (native select, date picker lạ) | Thay bằng component design-system tương đương; nêu nếu thiếu component. |
| D3 | (flutter) Size bằng **ScreenUtil** `.w/.h/.sp`; (rn) responsive theo Dimensions — **không pixel cứng** | Spec/mockup cho kích thước pixel cố định | Quy đổi sang đơn vị responsive; testcase kiểm "đúng X px" cần đổi cách kiểm. |

### 3.5. Quyền, trạng thái, đa ngữ cảnh (coverage gap)

| # | Rule UI/UX | Điều Confluence/testcase hay bỏ sót | Đề xuất mặc định |
|---|------------|-------------------------------------|------------------|
| E1 | Gate hành động bằng **CASL** `ability.can(action, subject)` | Testcase giả định user luôn thấy/bấm được nút | Bổ sung ca test theo quyền; nêu nếu spec không định nghĩa ma trận quyền. |
| E2 | (flutter) **đa brand** (ThaiThinh/NeoMedic); (rn) **light/dark** theo `useColorScheme` | Spec chỉ mô tả 1 theme/brand | Nêu thiếu ca cho brand/theme còn lại; không tự bịa hành vi. |
| E3 | (rn) touch target **≥ 44pt** + `accessibilityLabel` | Mockup icon nhỏ hơn, không nói accessibility | Giữ 44pt + label; nêu chênh so với mockup. |

## 4. Mẫu ghi nhận một xung đột (dùng khi STOP báo người quyết)

```
### Xung đột #<n> — <tiêu đề ngắn>
- Loại: <mã ở §3, vd A1 / D1>
- Nguồn spec: <link Confluence / mã testcase>
- Rule UI/UX: <trích rule từ pack + tên pack>
- Spec/testcase yêu cầu: <trích nguyên văn>
- Ảnh hưởng: <màn hình/flow nào>
- Đề xuất: <phương án + lý do>
- Cần người quyết: [ ] theo rule  [ ] theo spec  [ ] phương án khác: ____
```

## 5. Liên quan

- Rule nguồn: `packs/react.json`, `packs/flutter.json`, `packs/rn.json` (`design-system`, `conventions`).
- Skill đọc spec: `spec-from-source` (Confluence/OpenAPI/authswagger) → xem `skill.json`.
- Quy trình dùng: khi `deliver-feature`/sinh testcase gặp mâu thuẫn, áp §1 (STOP + liệt kê §4).
