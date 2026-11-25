# XGame AI Proxy — Spec cho agent

> Đặt file này tại `docs/xgame/PROXY_SPEC.md` trong repo `new-api` (fork từ QuantumNous/new-api).
> Agent: đọc toàn bộ file trước khi làm. **Luôn kiểm tra New API đã có tính năng chưa trước khi viết mới.**

## 1. Mục tiêu

Proxy nội bộ của XGame (game studio, Việt Nam), đứng giữa nhân viên và các nhà cung cấp AI:

- Nhân viên dùng **key nội bộ** do proxy cấp (không bao giờ thấy key thật).
- Proxy giữ **key thật** của công ty cho từng nhà cung cấp, có thể nhiều key mỗi nhà cung cấp.
- **Quota quản lý nội bộ** theo đơn vị riêng của proxy, quy đổi từ token / ảnh / giây video theo bảng tỉ giá. Không phụ thuộc cách tính của bên thứ ba.
- **Ghi log** mọi request: ai, lúc nào, model gì, lượng dùng, chi phí quy đổi.
- Proxy **không** xử lý prompt (không chèn hay sửa prompt), chỉ chuyển tiếp, kiểm soát và ghi log.

## 2. Use case

| Nhóm | Việc | Model |
|---|---|---|
| Nhân viên chung | Hỏi đáp, chat | DeepSeek V4.1 Flash (mặc định), Qwen3.8-Flash, Qwen3.7-Plus |
| Dev | Coding agent (Claude Code, Cline, OpenCode...) | DeepSeek V4.1 Flash (mặc định), GLM-5.3, Kimi K3.1 Pro, MiniMax M3.1 Flash (chỉ batch/background) |
| Creative / UA | Ảnh demo level game theo style ref (image+text → image), ảnh ad creative cho network | Nano Banana 2.1 (Gemini API), Seedream 5.0 flash / 5.5 (BytePlus ModelArk), GPT Image 2.6 (ảnh final) |
| Creative / UA | Video ngắn intro/outro/hook cho game puzzle, hyper-casual (image-to-video) | Kling 4.0 / 4.0 Flash (Kling Open Platform), Wan 3.0 (Alibaba Model Studio, Singapore), Seedance 2.0 Fast/Mini (BytePlus) |

## 3. Nhóm user và hạn mức (giá trị khởi đầu, admin chỉnh trên UI)

| Group | Model được phép | Quota gợi ý / tháng |
|---|---|---|
| `staff` | DeepSeek V4.1 Flash, Qwen3.8-Flash | ~ $5–10 / người |
| `dev` | staff + GLM-5.3, Kimi K3.1 Pro, Qwen3.7-Plus, MiniMax M3.1 Flash | ~ $30–50 / người |
| `creative` | staff + toàn bộ model ảnh/video ở trên | ~ $200–500 / team |
| `admin` | tất cả | không giới hạn, chỉ log |

Có rate limit theo key nội bộ, và cảnh báo khi một user hoặc group dùng hết 80% quota.

## 4. Upstream channels

- **Mỗi nhà cung cấp dùng một tài khoản công ty.** Có thể có nhiều key trong cùng tài khoản để tách nhóm / xoay vòng, nhưng **không** lập nhiều tài khoản để lách rate limit (vi phạm ToS).
- Fallback **giữa các nhà cung cấp**, ví dụ: `deepseek-flash` → Qwen → OpenRouter.
- Xử lý lỗi upstream:
  - `429`: cooldown key/kênh rồi thử kênh khác.
  - `402` / hết số dư: disable key và gửi cảnh báo nạp tiền.
  - `401`: disable key.
- **Sticky key theo user/session** cho chat và coding, để giữ prompt cache hit (DeepSeek/Qwen cache theo tài khoản).
- **Task video bất đồng bộ** (Kling, Seedance, Wan): query status và tải kết quả phải dùng **đúng key/kênh đã tạo task**.
- Lưu ý giờ cao điểm của DeepSeek: 01–04h và 06–10h UTC (8–11h, 13–17h giờ VN), giá gấp đôi. Tỉ giá quota nên tính theo giá cao điểm.

## 5. Logging và dữ liệu

- Mặc định chỉ log metadata: user, group, model, channel, usage, chi phí quy đổi, latency, status.
- Log nội dung prompt/response là tùy chọn bật theo group, có retention (mặc định 30 ngày) và giới hạn quyền xem.
- Ảnh/video output: lưu bản sao sang storage của công ty (S3-compatible, ví dụ Vultr Object Storage), vì URL của nhà cung cấp là tạm thời.

## 6. Triển khai

- Server: VPS Vultr (Ubuntu), Docker Compose gồm `new-api`, PostgreSQL, Redis, reverse proxy HTTPS (Caddy hoặc Nginx).
- Secrets nằm trong `.env`, **không commit**. Có script backup DB hằng ngày.
- Đăng nhập admin/user: ưu tiên OIDC / Google Workspace công ty (New API có OIDC, cần kiểm tra).

## 7. Ràng buộc code

- License AGPL-3.0: giữ LICENSE và **giữ link hiển thị tới project gốc** trên UI.
- Giữ remote `upstream` = QuantumNous/new-api để merge bản vá. **Hạn chế sửa code lõi.** Code riêng của XGame đặt trong module/file tách biệt, đánh dấu comment `// XGAME:`.
- Mỗi thay đổi có test (Go: `go test ./...`), không phá build frontend `web/`.
- Không hard-code key, giá hay URL. Mọi thứ lấy từ config/DB.

## 8. Lộ trình

**Phase 0: Khảo sát (chỉ đọc, không sửa code).** Viết báo cáo `docs/xgame/GAP_ANALYSIS.md`, mỗi mục trong spec ghi rõ: *có sẵn / có một phần / chưa có*, kèm file/đường dẫn code liên quan. Cần kiểm tra cụ thể:
- multi-key channel
- auto-disable
- model ratio / quota
- group
- rate limit
- log nội dung
- task video Kling / Volcengine (Seedance) / Ali (Wan)
- Gemini image (Nano Banana)
- OIDC
- sticky channel

**Phase 1: Deploy.** `deploy/xgame/docker-compose.yml` + `.env.example` + Caddyfile + script backup + README tiếng Việt.

**Phase 2: Cấu hình seed.** Script hoặc hướng dẫn tạo channels, model ratio, groups theo mục 3–4, dùng admin API của New API nếu có.

**Phase 3: Tính năng còn thiếu.** Chỉ làm những mục Phase 0 ghi là "chưa có", theo thứ tự ưu tiên:
1. cảnh báo quota/số dư (webhook Telegram hoặc Slack, hỏi lại admin dùng kênh nào)
2. sticky key/session
3. lưu media sang S3
4. log nội dung có retention
