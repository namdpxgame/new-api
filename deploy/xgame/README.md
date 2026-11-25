# XGame AI Proxy — Hướng dẫn triển khai

Bộ triển khai New API cho XGame trên một VPS Ubuntu (Vultr) bằng Docker Compose:

| Service | Vai trò |
|---|---|
| `new-api` | Gateway AI, build từ chính repo này (gồm code riêng `// XGAME:`) |
| `postgres` | Database chính (PostgreSQL 16) |
| `redis` | Cache, rate limit, channel affinity |
| `caddy` | Reverse proxy HTTPS, tự xin chứng chỉ Let's Encrypt |

Chỉ Caddy mở cổng ra ngoài (80/443). New API, PostgreSQL và Redis chỉ nằm trong mạng Docker nội bộ.

## 1. Chuẩn bị

- VPS Ubuntu có Docker Engine và Docker Compose plugin (`docker compose version`).
- Một domain (ví dụ `ai.xgame.vn`) có bản ghi A/AAAA trỏ về IP của VPS.
- Firewall mở cổng 80 và 443 (TCP), 443 (UDP cho HTTP/3).

## 2. Cấu hình

```bash
git clone <repo fork của XGame> new-api
cd new-api/deploy/xgame
cp .env.example .env
chmod 600 .env
```

Điền `.env`:

- `DOMAIN`, `ACME_EMAIL`: domain và email nhận thông báo chứng chỉ.
- `POSTGRES_PASSWORD`, `REDIS_PASSWORD`, `SESSION_SECRET`, `CRYPTO_SECRET`: mỗi biến sinh một chuỗi riêng bằng `openssl rand -hex 32`.
- **Không đổi `CRYPTO_SECRET` sau khi đã chạy thật** — dữ liệu đã mã hoá sẽ không đọc lại được.
- `XGAME_GCHAT_WEBHOOK_URL`: xem mục 5.

File `.env` chứa toàn bộ bí mật, không bao giờ commit (đã có trong `.gitignore`).

## 3. Khởi động

```bash
docker compose up -d --build
docker compose ps
docker compose logs -f new-api
```

Lần build đầu mất vài phút (build cả frontend và backend).

## 4. Tạo tài khoản admin ngay sau khi khởi động

Lần đầu chạy, New API hiện trang **Setup** để tạo tài khoản root. **Ai mở trang web trước sẽ tạo được root**, nên:

1. Mở `https://<DOMAIN>` ngay sau khi `docker compose ps` báo `new-api` healthy.
2. Tạo tài khoản root với mật khẩu mạnh.
3. Nếu muốn an toàn tuyệt đối, tạm giới hạn cổng 443 chỉ cho IP văn phòng (firewall Vultr) cho tới khi setup xong.

Sau đó cấu hình trong trang admin (chi tiết ở Phase 2): channel, group, giá model, OIDC Google Workspace, SMTP.

## 5. Cảnh báo Google Chat khi key nhà cung cấp gặp vấn đề

Khi New API **tự động disable** một kênh hoặc một key của nhà cung cấp, proxy gửi một tin nhắn vào Google Chat space của admin. Các trường hợp gây disable:

- Nhà cung cấp trả lỗi **401** (key sai hoặc bị thu hồi).
- Key **hết số dư** (ví dụ "Insufficient balance" khi chạy request hoặc khi admin bấm cập nhật số dư).
- Lỗi khớp danh sách mã lỗi/từ khoá tự disable trong Settings.
- Plugin kiểm tra số dư định kỳ báo key hết tiền.

Với kênh nhiều key, chỉ key lỗi bị disable, các key khác vẫn chạy. Vào trang kênh để xem key nào bị disable và lý do.

Cấu hình:

1. Trong Google Chat: mở space → **Apps & integrations** → **Webhooks** → tạo webhook, copy URL.
2. Dán URL vào `XGAME_GCHAT_WEBHOOK_URL` trong `.env`. URL chứa `key` và `token` của space — ai có URL đều gửi được tin vào space, nên chỉ để trong `.env`.
3. Áp dụng: `docker compose up -d` (Compose tự tạo lại container `new-api` khi biến môi trường đổi).
4. Trong trang admin, vào **System Settings → Request Policies → Channel health** (đường dẫn `/system-settings/request-policies`) và bật **Disable on failure** — mặc định đang **tắt**, nếu không bật thì không có kênh nào bị tự disable và cũng không có cảnh báo. Ô **Auto-disable status codes** mặc định chỉ có `401`; nên thêm `402`. Ô **Failure keywords** chứa các từ khoá lỗi (ví dụ báo hết số dư) cũng dẫn tới disable.
5. Mỗi kênh còn có công tắc riêng **Auto Ban** trong form sửa kênh. Kênh tạo trên giao diện mặc định bật; kênh tạo qua API phải gửi `auto_ban: 1`, nếu không kênh đó không bao giờ bị tự disable và không có cảnh báo.

Kiểm tra webhook hoạt động (chạy trên VPS):

```bash
set -a; source .env; set +a
curl -sS -X POST -H 'Content-Type: application/json; charset=UTF-8' \
  -d '{"text":"XGame AI Proxy: test cảnh báo"}' "$XGAME_GCHAT_WEBHOOK_URL"
```

Để trống `XGAME_GCHAT_WEBHOOK_URL` thì không gửi gì. Các thông báo email sẵn có của New API vẫn hoạt động song song.

## 6. Backup PostgreSQL

`backup.sh` chạy `pg_dump` (định dạng custom) vào `BACKUP_DIR` (mặc định `./backups`) và xoá bản cũ hơn `BACKUP_KEEP_DAYS` ngày (mặc định 7).

Chạy tay:

```bash
./backup.sh
```

Cron hằng ngày lúc 2h sáng (`crontab -e`):

```cron
0 2 * * * /đường/dẫn/new-api/deploy/xgame/backup.sh >> /var/log/xgame-backup.log 2>&1
```

Nên copy thư mục `backups/` ra ngoài VPS (ví dụ Vultr Object Storage) để phòng mất máy.

Khôi phục từ một bản backup:

```bash
docker compose stop new-api
set -a; source .env; set +a
docker compose exec -T postgres pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  --clean --if-exists --no-owner < backups/new-api-YYYYmmdd-HHMMSS.dump
docker compose start new-api
```

## 7. Cập nhật

Lấy bản vá từ project gốc rồi build lại:

```bash
git fetch upstream
git merge upstream/main
cd deploy/xgame
./backup.sh
docker compose up -d --build
```

Code riêng của XGame được đánh dấu `// XGAME:` để dễ xử lý khi merge bị conflict.

## 8. Vận hành

- Log ứng dụng: `docker compose logs -f new-api` và thư mục `./logs`.
- Khởi động lại: `docker compose restart new-api`.
- Dừng toàn bộ: `docker compose down` (dữ liệu trong volume vẫn giữ). **Không** dùng `docker compose down -v` — lệnh này xoá luôn database.

## 9. Giấy phép

New API dùng giấy phép AGPL-3.0. Giữ nguyên file `LICENSE` và đường link tới project gốc hiển thị trên giao diện.
