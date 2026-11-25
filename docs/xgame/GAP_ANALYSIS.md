# GAP_ANALYSIS.md — Phase 0 khảo sát New API cho XGame AI Proxy

> Chỉ đọc code, không sửa. Đối chiếu với `docs/xgame/PROXY_SPEC.md` mục 8 (Phase 0). Repo: `QuantumNous/new-api` (nhánh `xgame/main`).

## Tóm tắt nhanh

| # | Mục | Trạng thái |
|---|---|---|
| 1 | Multi-key channel | **Có sẵn** |
| 2 | Auto-disable | **Có một phần** (401/402 có, 429 cooldown chưa có) |
| 3 | Model ratio / quota | **Có sẵn** |
| 4 | Group (phân quyền model) | **Có sẵn** |
| 4b | Group (quota pool chung cho team) | **Có một phần** |
| 5 | Rate limit | **Có một phần** (thiếu cảnh báo 80%) |
| 6 | Log nội dung (metadata) | **Có sẵn** |
| 6b | Log nội dung (prompt/response, toggle theo group, retention tự động) | **Chưa có** |
| 6c | Lưu media sang S3 | **Có một phần** (chỉ có config stub, chưa có code upload) |
| 7 | Task video Kling | **Có sẵn** |
| 8 | Task video Volcengine (Seedance) | **Có sẵn** |
| 9 | Task video Ali (Wan) | **Có sẵn** |
| 10 | Gemini image (Nano Banana) | **Có sẵn** |
| 11 | OIDC | **Có sẵn** |
| 12 | Sticky channel/key | **Có sẵn** (gọi là "Channel Affinity", chưa có rule sẵn cho DeepSeek/Qwen) |

---

## 1. Multi-key channel — Có sẵn

Một channel có thể chứa nhiều key, mỗi key có trạng thái bật/tắt riêng.

- `model/channel.go:73-79` — `ChannelInfo.IsMultiKey`, `MultiKeySize`, `MultiKeyStatusList` (trạng thái từng key), `MultiKeyDisabledReason` / `MultiKeyDisabledTime` (per-key), `MultiKeyPollingIndex`, `MultiKeyMode`.
- Chọn key: `model/channel.go:221-293` — bỏ qua key đã disable, chọn theo `MultiKeyMode` (random hoặc polling/xoay vòng).
- Disable per-key khi hết tiền: `model/channel.go:796-848` (`handlerMultiKeyUpdate`), `controller/channel_balance.go:91-223` (hết số dư → disable đúng key đó; chỉ disable cả channel khi **tất cả** key đã disable → `ChannelStatusReasonAllKeysDisabled`).

**Cách cấu hình:** trong form tạo/sửa channel trên admin UI, chọn `is_multi_key = true`, dán nhiều key (mỗi dòng một key — giống field `Key` nhiều dòng trước đây), chọn chế độ random/polling. Không cần sửa code.

## 2. Auto-disable — Có một phần

- Toggle tổng: `common.AutomaticDisableChannelEnabled` / `AutomaticEnableChannelEnabled` (`common/constants.go:134-135`), cấu hình qua admin → Options.
- Logic quyết định disable: `service/channel.go:68-94` (`ShouldDisableChannel`) — disable khi lỗi tới từ phía channel, mã lỗi khớp `AutomaticDisableStatusCodeRanges` (**mặc định chỉ có 401**, `setting/operation_setting/status_code_ranges.go:17`), hoặc message khớp `AutomaticDisableKeywords`.
- **401** → khớp mặc định trên → **có sẵn**.
- **402 / hết số dư** → xử lý riêng, đúng ý spec: `controller/channel_balance.go:209-223` disable channel/key khi gặp "Insufficient balance".
- **429** → **chưa có**. Không có khái niệm cooldown tạm thời cho key/channel (grep "cooldown" ra 0 kết quả). 429 chỉ kích hoạt chuyển sang channel khác cho request hiện tại (`service/relay_error.go:20-55`, `DecideRelayRetry`), key/channel bị 429 **không** bị hạ ưu tiên hay tạm khóa — request tiếp theo vẫn có thể chọn lại ngay channel đó.
- Tự enable lại: `service/channel.go:95-107` (`ShouldEnableChannel`) — channel tự bật lại khi có request sau đó thành công.
- Cảnh báo khi kênh/key bị disable: `DisableChannel` (`service/channel.go:30-52`) gọi `NotifyChannelWatchers` (`service/user_notify.go:21-66`), gửi tới các admin bật "channel notify" (mặc định root) qua kênh thông báo admin đó chọn: email (mặc định), webhook, Bark, Gotify. **Chưa có** Telegram/Slack native (payload webhook hiện tại không khớp format Slack/Telegram).

**Gap cần làm (Phase 3):** cooldown theo key/channel khi gặp 429 (thời gian ngắn, không phải disable hẳn); thêm notify-channel kiểu Telegram/Slack.

## 3. Model ratio / quota — Có sẵn

Hai hệ thống song song, đều cấu hình qua admin, không phụ thuộc cách tính bên thứ ba:

- **Bảng tỉ giá kiểu cũ**: `types/price_data.go:24-59` — `PriceData{ModelRatio, CompletionRatio, CacheRatio, ImageRatio, AudioRatio, ...}` + `OtherRatios` (qua `AddOtherRatio`, chặn giá trị ≤0/NaN/+Inf).
- **Hệ thống billing-expression (mới, cho model mới)**: `pkg/billingexpr/` (thiết kế đầy đủ ở `pkg/billingexpr/expr.md`), giá built-in khai báo ở `setting/billing_setting/builtin_billing.go` (USD/triệu token), tiered pricing ở `setting/billing_setting/tiered_billing.go`.
- Quy đổi quota tập trung ở `common/quota_math.go` (`QuotaFromFloat`, `QuotaRound`, `QuotaFromDecimal` + bản `*Checked` để audit khi bị clamp).
- Cấu hình: admin UI pricing (`controller/pricing.go`) cho sửa giá/tỉ giá theo từng model/channel, lưu DB — đúng yêu cầu "quota nội bộ quy đổi theo bảng tỉ giá riêng, không phụ thuộc bên thứ ba".

Không có gap. Việc còn lại là **nhập giá theo tỉ giá cao điểm DeepSeek** (mục 4 spec) — chỉ là công việc seed dữ liệu (Phase 2), không phải code mới.

## 4. Group — Có sẵn (phân quyền model), Có một phần (quota pool chung)

- **Group ratio**: `setting/ratio_setting/group_ratio.go:12-80` — `GroupRatio` (hệ số giá theo group, sửa qua admin dạng JSON), `GroupGroupRatio`, `GroupSpecialUsableGroup`. Tên group tự do (`default`, `vip`, `svip`, hoặc `staff`/`dev`/`creative`/`admin` theo spec).
- **User ↔ group**: field string trên `model/user.go` (`GetUserGroup`).
- **Group ↔ model/channel**: `model/channel.go:45` (field `Group`, nhiều group phân tách bằng dấu phẩy trên mỗi channel), `model/channel.go:147-171` (`NormalizeChannelGroupFilter`/`ApplyChannelGroupFilter`), `model/channel_cache.go:20` (`group2model2channels`), `model/channel_satisfy.go:10-47` (`IsChannelEnabledForGroupModel`) — đây chính là cơ chế để nhóm `staff` chỉ thấy DeepSeek/Qwen còn `creative` thấy thêm model ảnh/video.
- **API liệt kê group**: `controller/group.go` (`GetGroups`, `GetUserGroups`).

**Gap (có một phần):** quota hiện tại là theo **user** (cột quota/wallet trên `model/user.go`), **không có** khái niệm "quota pool chung cho cả team/group" (spec mục 3 muốn `creative` ~$200-500/**team**, không phải mỗi người). Nếu cần pool chung thật, phải tự cộng dồn hoặc viết thêm logic — đây là việc code mới thực sự (ưu tiên thấp, có thể dùng workaround: set quota cao cho từng user trong group creative).

## 5. Rate limit — Có một phần

- **Theo model**: `middleware/model-rate-limit.go` — giới hạn dựa trên Redis (sliding window, `checkRedisRateLimit`/`recordRedisRequest` dòng 29-79), có cả giới hạn tổng request và giới hạn request thành công, thời gian cấu hình qua `setting.ModelRequestRateLimitDurationMinutes`. Khoá theo "subject" (user/token/model) — đáp ứng rate limit theo key nội bộ.
- **Tổng quát**: `middleware/rate-limit.go:16-120` — fixed window bằng Lua script atomic trên Redis, khoá theo IP hoặc user; có fallback in-memory (`common.InMemoryRateLimiter`).
- **Cảnh báo 80% quota** — **chưa có đúng như spec**. Cái đang có là `service/quota.go:539-551` (`quotaReminderDue`) + `QuotaRemindThreshold`/`QuotaWarningThreshold`: cảnh báo khi **số dư còn lại** xuống dưới một ngưỡng tuyệt đối (không phải "đã dùng 80%"), gửi qua email/Bark/Gotify/webhook (`quotaNotifyContent`, dòng 556-567) — **không có** Telegram/Slack sẵn, và cơ chế này theo **user**, không theo **group** pool.

**Gap (Phase 3):** thêm notify-type Telegram/Slack; đổi/ thêm điều kiện cảnh báo theo % đã dùng (không chỉ số dư tuyệt đối); cần cộng dồn theo group nếu làm pool quota chung (phụ thuộc gap mục 4b).

## 6. Log nội dung — Có sẵn (metadata), Chưa có (nội dung + toggle + retention tự động)

- **Metadata** (user, group, model, channel, usage, cost, latency, status): log sẵn đầy đủ trong `model/log.go`.
- **Nội dung prompt/response**: field `Log.Content` (`model/log.go:60-75`) **không** chứa prompt/response thật, chỉ chứa mô tả ngắn do hệ thống dựng (ví dụ "Model ratio {{model_ratio}}...", xây ở `service/quota.go:213-245,352-384`, `service/task_billing.go:90`). Không tìm thấy field hay toggle `LogContentEnabled`/`log_content` nào trong code. → **Chưa có** việc lưu nội dung thật, và dĩ nhiên **chưa có** toggle theo group.
- **Dọn log theo retention**: có cơ chế xoá log cũ `model/log.go:776-820` (`CountOldLog`, `DeleteOldLogBatch`) + `service/system_task.go:76-170,339-430` (`logCleanupHandler`, `StartLogCleanupTask`), nhưng **chỉ chạy khi admin gọi tay** qua `controller/system_task.go:24` (truyền `targetTimestamp`), **không có job lịch tự động với retention mặc định (ví dụ 30 ngày)**. Có thể so sánh với `service/auth_cleanup.go:14,51` — cleanup session có hằng số retention theo ngày tự động, log thì không có hằng số tương đương.
- **Phân quyền xem log**: **có sẵn** nhưng chỉ cho phần *metadata nội bộ*, không phải nội dung — `model/log.go:118,124-127,169,228,628,661` ẩn `other.admin_info` khỏi user thường (non-admin), chỉ admin thấy (`service/log_info_generate.go:22-34`). Cơ chế này có thể tái dùng để gate nội dung prompt/response sau khi field đó được thêm.

**Gap (Phase 3):** (a) thêm field lưu prompt/response (phải cân nhắc kích thước DB — nên để log DB riêng/ClickHouse), (b) toggle bật/tắt theo group, (c) job lịch tự động xoá log cũ theo retention mặc định 30 ngày (có thể dựa trên `StartLogCleanupTask` đã có, chỉ cần thêm cron + hằng số retention), (d) gate quyền xem nội dung (tái dùng cơ chế `admin_info`).

## 6c. Lưu media sang S3 — Có một phần (chỉ có khung, chưa chạy)

- `setting/system_setting/task_artifact_store.go` đã định nghĩa đầy đủ: `TaskArtifactStoreModeUpstream` / `TaskArtifactStoreModeS3`, cấu hình S3 endpoint/bucket/region/access key/prefix/presign TTL qua env `TASK_ARTIFACT_STORE_S3_*`, có validate.
- Nhưng `LoadTaskArtifactStoreConfig` **ép cứng `Mode = TaskArtifactStoreModeUpstream`** và log cảnh báo `"task artifact S3 storage is not implemented; using upstream mode"` mỗi khi ai đó cấu hình S3 mode — **chưa có code upload/copy thật nào**. Không tìm thấy tích hợp s3/minio/oss nào khác trong `service/`, `relay/`, hay task-plugin host.

**Gap (Phase 3):** viết phần upload thật (copy ảnh/video từ URL tạm của provider sang S3-compatible storage) theo đúng contract đã có sẵn trong `task_artifact_store.go` — khung cấu hình đã xong, chỉ thiếu logic.

## 7–9. Task video (Kling / Volcengine-Seedance / Ali-Wan) — Có sẵn

Cả ba đều đã có plugin JavaScript (task plugin host, `pkg/jsplugin/`), và cơ chế **sticky-by-task** (query status/download đúng key/channel đã tạo task) đã có sẵn cho mọi task plugin, không chỉ riêng 3 nhà cung cấp này:

- **Kling**: `plugins/tasks/kling/plugin.js` (`channelTypes:[50]`, v1.2.0) — route `/v1/videos/text2video`, `/v1/videos/image2video`, query theo `task_id`, có cả route Kling 4.0 (`kling4ImageToVideo`).
- **Volcengine (Seedance)**: `plugins/tasks/doubao/plugin.js` ("Doubao", v1.5.0) — `VIDEO_MODELS` (Seedance) + `IMAGE_MODELS` (Seedream) trong cùng plugin, `fetchMode: "per_task"`. (Có thêm `plugins/tasks/jimeng/plugin.js` riêng cho video Jimeng của ByteDance, không liên quan Seedance.)
- **Ali (Wan)**: `plugins/tasks/alibaba/plugin.js` ("Alibaba Bailian", v1.5.0) — cùng một plugin xử lý cả ảnh (`IMAGE_MODELS`, Qwen-Image, protocol `openai_image`) **và** video (`WAN_MODELS`, protocol `openai_video`, `videoAction()` dòng ~838 dispatch `image_to_video`/`text_to_video`). *(Đính chính: `.agents/rules/billing.md` chỉ nhắc phần ảnh, nhưng video Wan cũng nằm trong plugin này, không thiếu.)*

**Cơ chế sticky-by-task (đáp ứng đúng yêu cầu spec mục 4):**
- `model/task.go:269-284` (`InitTask`) — với `ChannelTypeTaskPlugin`, lưu `privateData.CredentialAccount` của key đã tạo task, và mỗi `Task` row lưu `ChannelId` (`model/task.go:308`).
- `service/task_polling.go` nhóm task theo `ChannelId` (`taskChannelM`, dòng 209), resolve lại đúng channel (`cacheGetChannel`, dòng 290/527) trước khi polling — đảm bảo query status/download luôn dùng đúng key/channel đã tạo task.

Không có gap cho 3 mục này.

## 10. Gemini image (Nano Banana) — Có sẵn

Là channel adaptor built-in (không qua task plugin, vì sinh ảnh Gemini đồng bộ):

- `relay/channel/gemini/adaptor.go:58-70` — route ảnh kiểu `/v1/images/generations` chỉ dành cho Imagen (`imagen*`).
- Model Nano Banana (`gemini-3-pro-image-preview`, `nano-banana-pro-preview`, `gemini-3.1-flash-image-preview` — `relay/channel/gemini/constant.go:17-18`) đi qua luồng chat `generateContent`/stream thông thường, decode ảnh qua `InlineData` ở `relay-gemini.go:167,245`. Đây chính là luồng chat đa lượt (multi-turn) chuẩn, nên ảnh+text nhiều lượt được hỗ trợ tự nhiên, không cần code thêm.

Không có gap.

## 11. OIDC — Có sẵn

Client OIDC generic, chuẩn, không khoá cứng vào provider nào — dùng được với Google Workspace làm issuer mà không cần sửa code:

- `oauth/oidc.go:1-181` — `OIDCProvider` theo interface `oauth.Provider` chung; `ExchangeToken` dùng `authorization_code` grant chuẩn; `GetUserInfo` map `sub`→ProviderUserID, `email`, `name`.
- `setting/system_setting/oidc.go:9-41` — `OIDCSettings{Enabled, DisplayName, ClientId, ClientSecret, WellKnown, AuthorizationEndpoint, TokenEndpoint, UserInfoEndpoint}`, lưu DB qua `config.GlobalConfig`.
- `controller/option.go:238` — API admin lưu các key `oidc.*`.
- `controller/misc.go:111-114` — endpoint public trả `oidc_enabled`/`oidc_client_id`/... cho trang login.
- `controller/oauth.go` — flow CSRF-safe chung cho mọi OAuth/OIDC (state một lần, hết hạn qua `model.CreateAuthFlow`/`GetAuthFlow`/`ConsumeAuthFlow`); redirect URI do server tự dựng (`{ServerAddress}/oauth/oidc`) — không có nguy cơ open-redirect.
- `model/user.go`, `model/account_security.go` — field `OidcId`, `IsOidcIdAlreadyTaken`, `FillUserByOidcId` để liên kết tài khoản.
- **Frontend**: `web/src/features/system-settings/auth/oauth-section.tsx:74-112,187-226,260-303` — form admin có field "well-known URL" tự động discover endpoint, hoặc nhập tay từng endpoint, Client ID/Secret, Display Name, toggle Enabled.

**Cách cấu hình với Google Workspace (không cần sửa code):**
1. Tạo OAuth 2.0 Client trong Google Cloud Console, redirect URI = `{server_address}/oauth/oidc`.
2. Admin New API → mục Auth → OIDC: dán well-known URL `https://accounts.google.com/.well-known/openid-configuration` để auto-fill endpoint, hoặc điền tay.
3. Nhập Client ID/Secret, Display Name, bật Enabled.

**Lưu ý nhỏ (không phải gap chặn Phase 0, nhưng nên ghi nhận cho OWASP):** `oauth/oidc.go` chưa thấy verify chữ ký/nonce của `id_token` — tin tưởng vào kết quả gọi UserInfo endpoint làm trust anchor thay vì validate `id_token` đầy đủ. Đây là pattern OIDC được chấp nhận nhưng yếu hơn full ID-token validation; nên liệt kê thành ghi chú rủi ro nếu có task nào đụng tới auth flow sau này (tham chiếu `AGENTS.md` mục OWASP).

## 12. Sticky channel/key — Có sẵn (tên trong code: "Channel Affinity")

Hệ thống session-affinity generic, mạnh hơn yêu cầu spec, chỉ cần thêm rule cấu hình:

- `setting/operation_setting/channel_affinity_setting.go` — `ChannelAffinitySetting{Enabled, SessionMode, SwitchOnSuccess, KeepOnChannelDisabled, MaxEntries, DefaultTTLSeconds, Rules[]}`. Mỗi `ChannelAffinityRule` match theo `ModelRegex`/`PathRegex`/`UserAgentInclude`, lấy cache key từ `KeySources` (context value, header, hoặc field JSON trong body qua gjson), có `TTLSeconds`, `SkipRetryOnFailure`, `SessionMode` (off/prefer/strict).
- Đã có sẵn rule mẫu cho Codex CLI và Claude CLI (cùng file, dòng 117-154) — có thể copy pattern này để tạo rule riêng cho chat DeepSeek/Qwen, khoá theo user/token.
- Runtime: `service/channel_affinity.go` (get/set cache, TTL, stats), nối vào chọn channel ở `service/channel_select.go:125-361` và `middleware/distributor.go:137`.
- API admin: `GET/DELETE /option/channel_affinity_cache`, `GET /log/channel_affinity_usage_cache` (`router/api-router.go:226-227,336`).

**Gap thực tế:** chỉ là **thiếu 1 rule cấu hình** cho DeepSeek/Qwen (khoá theo user/token để giữ cache hit) — đây là việc cấu hình (Phase 2), **không phải** code mới như liệt kê ở Phase 3 mục 2 trong spec.

---

## Đề xuất thứ tự ưu tiên Phase 1–3 (cập nhật theo kết quả khảo sát)

### Phase 1 — Deploy (không đổi so với spec)
Không có gap kỹ thuật nào ảnh hưởng Phase 1. `deploy/xgame/docker-compose.yml` + `.env.example` + Caddyfile + script backup + README có thể làm độc lập với các gap ở trên.

### Phase 2 — Cấu hình seed (ưu tiên cao, vì đều là "có sẵn" chỉ cần cấu hình)
1. Tạo channel cho từng nhà cung cấp, bật `is_multi_key`, nhập các key công ty, chọn `MultiKeyMode` (polling để xoay vòng đều, hoặc random).
2. Tạo group `staff`/`dev`/`creative`/`admin`, gán `GroupRatio`, gán field `Group` trên từng channel theo model được phép (mục 3 spec).
3. Nhập giá/tỉ giá model qua `pkg/billingexpr` hoặc bảng ratio cũ — **nhớ tính theo giá cao điểm DeepSeek** (01–04h, 06–10h UTC) vì tỉ giá quota không tách theo giờ.
4. **Thêm Channel Affinity rule mới** cho chat DeepSeek/Qwen (khoá theo user/token) — tái dùng pattern rule Claude CLI/Codex CLI đã có sẵn trong `channel_affinity_setting.go`. *(Việc này đã bị liệt nhầm vào Phase 3 mục 2 trong spec gốc — thực chất chỉ là seed config, nên chuyển lên Phase 2.)*
5. Cấu hình OIDC với Google Workspace (well-known URL + Client ID/Secret) cho admin/user login.
6. Set `AutomaticDisableStatusCodeRanges` và `AutomaticDisableKeywords` phù hợp (401 đã mặc định có, cần xác nhận keyword cho các case đặc thù nếu có).

### Phase 3 — Tính năng còn thiếu (theo mức độ ưu tiên, dựa trên gap thật tìm được ở Phase 0)
1. **Cooldown theo key/channel khi gặp 429** (chưa có): hiện 429 chỉ fail-over tức thời, không hạ ưu tiên hay tạm khóa key/channel đó — cần thêm để tránh dính rate-limit lặp lại ngay. *(Gap mới phát hiện, không có trong danh sách gốc của spec — nên ưu tiên cao vì ảnh hưởng trực tiếp chi phí/độ ổn định.)*
2. **Cảnh báo cho admin khi key nhà cung cấp bị tự disable** (spec mục 1) — **đã làm**: admin chọn Google Chat webhook, chỉ cảnh báo phía key nhà cung cấp. Code ở `service/xgame_gchat_notify.go`, gắn vào `DisableChannel` (`service/channel.go`) và luồng plugin kiểm tra số dư định kỳ (`service/provider_plugin_scheduled.go`); cấu hình bằng biến `XGAME_GCHAT_WEBHOOK_URL`. Cảnh báo quota phía nhân viên: dùng ngưỡng cố định sẵn có (quota không reset theo tháng, admin cấp thêm thủ công), không làm code mới.
3. **Lưu media sang S3** (spec mục 3): code upload thật theo contract đã có sẵn ở `setting/system_setting/task_artifact_store.go` (chỉ cần implement, không cần thiết kế lại cấu hình).
4. **Log nội dung có retention** (spec mục 4): thêm field lưu prompt/response thật (cân nhắc DB log riêng/ClickHouse vì dung lượng), toggle theo group, job lịch tự động xoá log cũ theo retention mặc định 30 ngày (tái dùng `StartLogCleanupTask` đã có, thêm cron + hằng số retention), và gate quyền xem nội dung (tái dùng cơ chế ẩn `admin_info` đã có cho metadata).
5. **Quota pool chung theo group/team** (gap mới phát hiện, không có trong spec gốc nhưng liên quan mục 3): hiện quota chỉ theo từng user, nhóm `creative` muốn pool $200-500 chung cho cả team thì chưa có cơ chế cộng dồn — có thể làm workaround bằng set quota cao từng user trước, để code mới sau nếu cần kiểm soát chặt.

---

*Báo cáo này chỉ dựa trên đọc code tại thời điểm khảo sát (nhánh `xgame/main`), chưa sửa bất kỳ file code nào ngoài tạo file báo cáo này.*
