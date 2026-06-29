# 登录图形验证码设计

日期：2026-06-30
状态：已确认，待实施计划

## 1. 背景

当前 Web 登录页只要求账号和密码。`UserAuthForm` 调用 `AuthProvider.login`，最终由前端 API client 请求 `POST /api/auth/login`。Control Plane 的 auth handler 再调用 `Service.Login` 创建 session cookie，并把成功或失败写入 `web_login_logs`。

本次需求是在登录页增加图形验证码，并且所有登录都必须输入验证码。验证码不能只停留在前端校验，必须由 Control Plane 登录接口强制校验，避免绕过登录页直接调用接口。

## 2. 目标

- 所有 Web 控制台登录都必须提交图形验证码。
- 验证码为 4 位字符图片验证码。
- 每个验证码必须同时包含数字和字母。
- 验证码 challenge 由 Control Plane 生成并保存到 PostgreSQL。
- 验证码答案不得明文落库。
- 验证码一次性使用，过期或已使用后不可再次登录。
- 登录页支持加载、刷新、失败后自动刷新验证码。
- 验证码校验失败进入现有登录审计链路。

## 3. 非目标

- 本阶段不做滑块、拼图、行为轨迹或第三方验证码。
- 本阶段不引入 Redis 作为验证码存储。
- 本阶段不做按失败次数触发验证码；所有登录都必须输入。
- 本阶段不改造账号密码认证、session cookie 或用户权限模型。
- 本阶段不实现后台定时清理任务；提供 repository 清理方法，并在生成新验证码时清理过期 challenge。

## 4. 当前代码事实

- 前端登录入口是 `apps/web/src/features/auth/sign-in/index.tsx` 和 `components/user-auth-form.tsx`。
- `UserAuthForm` 目前只有账号、密码、表单错误和提交防重。
- `AuthProvider.login` 当前只接收 `{ username, password }`。
- 前端 API client 在 `apps/web/src/lib/api/auth.ts` 中定义 `LoginRequest` 和 `login()`。
- Auth OpenAPI 契约在 `contracts/control-plane/auth.yaml`。
- Control Plane 登录 handler 在 `apps/control-plane/internal/auth/handler.go`。
- 登录核心逻辑在 `apps/control-plane/internal/auth/service.go` 的 `Service.Login`。
- 登录成功和失败已经通过 `CreateLoginLog` 写入 `web_login_logs`。
- 数据库迁移遵循 `DATABASE_DESIGN.md`，当前应新增 forward migration，不修改已存在迁移。

## 5. 架构

验证码属于登录认证链路，放在 `apps/control-plane/internal/auth`，不放到 Web 前端或 Runtime。Web 只负责展示图片、收集输入、刷新验证码并提交字段；Control Plane 负责生成 challenge、生成图片、校验答案、消费 challenge、记录登录失败。

新增模块边界：

- `CaptchaService`：生成 4 位混合验证码、生成图片、校验并消费验证码。
- `CaptchaRepository`：基于 PostgreSQL 保存 challenge hash、过期时间、使用时间和客户端信息。
- `HTTPHandler`：新增 `GET /api/auth/captcha`，并让 `POST /api/auth/login` 在密码认证前先校验验证码。
- `UserAuthForm`：加载验证码、展示图片、收集验证码输入、刷新验证码、登录失败后刷新。

`Service.Login` 保持账号密码认证和 session 创建职责。验证码校验在 auth service 的登录编排层先完成，再进入现有 `Login` 流程。这样验证码逻辑不会污染密码认证函数，也不会改变 session 创建语义。

## 6. API 契约

新增接口：

```http
GET /api/auth/captcha
```

响应：

```json
{
  "captcha_id": "00000000-0000-0000-0000-000000000000",
  "image_data_url": "data:image/png;base64,...",
  "expires_at": "2026-06-30T10:00:00Z"
}
```

修改登录请求：

```json
{
  "username": "admin",
  "password": "admin",
  "captcha_id": "00000000-0000-0000-0000-000000000000",
  "captcha_code": "A7K2"
}
```

契约变更点：

- `contracts/control-plane/auth.yaml` 新增 `/api/auth/captcha`。
- `LoginJSONBody` 增加必填 `captcha_id` 和 `captcha_code`。
- 增加 `CaptchaChallengeResponse` schema。
- 运行 `corepack pnpm generate:control-plane` 更新 oapi-codegen 生成文件。
- 前端手写 API client 也要同步类型和请求函数。

## 7. 数据模型

新增 migration：`apps/control-plane/internal/storage/migrations/039_auth_captcha_challenges.sql`。

表名：`auth_captcha_challenges`

字段：

- `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`
- `tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid`
- `answer_hash VARCHAR(255) NOT NULL`
- `expires_at TIMESTAMPTZ NOT NULL`
- `used_at TIMESTAMPTZ NULL`
- `client_ip VARCHAR(255) NULL`
- `user_agent TEXT NULL`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`
- `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`

索引：

- `idx_auth_captcha_challenges_expires_at`
- `idx_auth_captcha_challenges_used_at`
- `idx_auth_captcha_challenges_created_at`

说明：

- 不保存验证码明文。
- `tenant_id` 采用当前 auth 默认租户，保持 UUID-first 和租户字段一致性。
- `used_at` 用于一次性消费和并发复用防护。
- migration 需要更新 `atlas.sum`，并补充迁移测试断言。

## 8. Repository 与 SQL

新增 sqlc 查询放在 `apps/control-plane/internal/storage/queries/captcha.sql`：

- `CreateCaptchaChallenge`
- `GetCaptchaChallengeForUpdate`
- `ConsumeCaptchaChallenge`
- `DeleteExpiredCaptchaChallenges`

`GetCaptchaChallengeForUpdate` 必须在事务内使用 `FOR UPDATE` 读取，登录校验时锁住 challenge，避免两个并发请求复用同一个验证码。

`ConsumeCaptchaChallenge` 只更新 `used_at IS NULL` 的记录，调用侧需要检查更新结果，防止重复消费。

Repository 接口新增验证码相关方法。`PgRepository` 使用生成的 sqlc 方法实现。测试 mock repo 需要同步实现这些方法。

## 9. 验证码生成规则

验证码长度固定 4 位。

字符集必须包含数字和字母：

- 数字：`2-9`
- 字母：`A-H`、`J-N`、`P-Z`，排除易混淆字符 `I` 和 `O`

生成规则：

1. 至少取 1 个数字。
2. 至少取 1 个字母。
3. 剩余位从完整字符集中随机取。
4. 使用安全随机源打乱顺序。
5. 最终 code 长度必须为 4，且至少包含 1 个数字和 1 个字母。

输入规范化：

- 去掉前后空格。
- 转成大写。
- 大小写不敏感，`a7k2` 与 `A7K2` 等价。

图片格式：

- 服务端生成 PNG，并以 `data:image/png;base64,...` 返回。
- 图片要包含轻量干扰线或噪点，但不要牺牲可读性。
- 图片尺寸保持紧凑，适配登录页现有 v3 表单密度。

## 10. 答案 Hash

答案不明文落库。使用 HMAC-SHA256，hash 输入为：

```text
captcha_id + ":" + normalized_code + ":" + server_secret
```

`server_secret` 由配置提供，新增 auth captcha 配置：

```yaml
auth:
  captcha:
    secret: "replace-with-random-secret"
    ttlSeconds: 300
```

配置缺失时服务启动失败，避免生产环境使用不稳定的进程级临时 secret。开发环境也必须在 `config.yaml` 或环境变量中显式配置 secret。

这里不是密码存储场景，验证码 TTL 短且答案必须结合 challenge id 和 server secret，HMAC-SHA256 足够清晰、便于测试。

## 11. 登录流程

页面加载流程：

1. `UserAuthForm` mount。
2. 调用 `GET /api/auth/captcha`。
3. 保存 `captcha_id`、`image_data_url`、`expires_at`。
4. 渲染图片、输入框和刷新按钮。

提交流程：

1. 前端校验账号、密码、验证码必填。
2. 验证码输入必须为 4 位。
3. 提交 `username`、`password`、`captcha_id`、`captcha_code`。
4. 后端 decode body。
5. 校验验证码字段必填。
6. `ValidateAndConsumeCaptcha`。
7. 验证码通过后执行现有账号密码认证和 session 创建。
8. 设置 `session_token` cookie。
9. 前端调用 `getCurrentUser` 并跳转。

失败流程：

- 验证码不存在、过期、已使用或答案错误：拒绝登录，消费 challenge，前端刷新验证码并清空验证码输入。
- 账号密码错误：沿用现有失败处理，当前 challenge 已消费，前端刷新验证码并清空验证码输入。
- 验证码加载失败：禁用登录按钮，展示“验证码加载失败，请刷新重试”。

## 12. 错误处理与审计

后端错误：

- 验证码无效、过期或已使用：`401 Unauthorized`
- 响应 body 继续使用现有错误 envelope，message 为 `验证码不正确或已过期`
- 登录请求缺少验证码字段：`400 Bad Request`
- 图片生成或存储失败：`500 Internal Server Error`

前端展示：

- 验证码错误：`验证码不正确或已过期`
- 账号密码错误：`用户名或密码不正确`
- 验证码加载失败：`验证码加载失败，请刷新重试`

登录审计：

- 验证码失败应写入 `web_login_logs`。
- `event_type = login_failed`
- `result = failed`
- `username` 使用请求中的账号快照。
- `failure_reason` 新增 `captcha_invalid` 或 `captcha_expired`。
- `client_ip` 和 `user_agent` 与现有登录日志一致。

当前登录日志的 `failure_reason` 按字符串处理，本次扩展后端常量、OpenAPI 示例和前端展示测试，不把失败原因收窄成封闭枚举。

## 13. 前端设计

`UserAuthForm` 在密码字段下方、登录按钮上方增加验证码区域：

- 左侧或上方为验证码输入框，label 为“图形验证码”。
- 右侧显示验证码图片。
- 图片旁提供刷新图标按钮，使用 lucide 图标，按钮有 tooltip 或 `aria-label="刷新验证码"`。
- 刷新验证码不清空账号和密码。
- 登录失败后自动刷新验证码，并清空验证码输入。
- 保持 v3 表单样式：`rounded-xl`、`bg-v3-card-soft`、`border-v3-line-strong`、focus 使用 `v3-brand`。

前端文件变更：

- `apps/web/src/lib/api/auth.ts`
  - 新增 `CaptchaChallengeResponse`
  - 新增 `getLoginCaptcha(options)`
  - 扩展 `LoginRequest`
- `apps/web/src/features/auth/auth-context.tsx`
  - 扩展 `login` credentials 类型
- `apps/web/src/features/auth/auth-provider.tsx`
  - 透传验证码字段
- `apps/web/src/features/auth/sign-in/components/user-auth-form.tsx`
  - zod schema 增加 `captcha_code`
  - 加载和刷新验证码状态
  - 提交时附带 `captcha_id`

## 14. 测试

后端单元测试：

- 生成验证码长度为 4。
- 生成验证码至少包含 1 个数字和 1 个字母。
- 输入大小写不敏感。
- 正确验证码允许进入账号密码校验。
- 错误验证码拒绝登录。
- 过期验证码拒绝登录。
- 已使用验证码拒绝登录。
- 同一个验证码不能并发复用。
- 验证码失败写入登录失败日志。
- 账号密码失败后验证码也被消费。

后端 repository / migration 测试：

- migration 包含 `auth_captcha_challenges` 表、UUID 主键、租户字段、过期和使用索引、注释。
- sqlc 生成方法可创建、锁定读取、消费和清理 challenge。

前端测试：

- 登录页初始加载验证码并渲染图片。
- 验证码加载中或失败时按钮状态正确。
- 空验证码提交显示必填提示。
- 非 4 位验证码显示格式提示。
- 点击刷新重新请求验证码，不清空账号和密码。
- 登录提交包含 `captcha_id` 和 `captcha_code`。
- 登录失败后刷新验证码并清空验证码输入。
- 登录成功后保持现有 redirect 行为。

实施时运行：

```bash
corepack pnpm --filter ./apps/web run test -- src/features/auth/sign-in/components/user-auth-form.test.tsx src/features/auth/auth-provider.test.tsx src/lib/api/auth.test.ts
go test ./apps/control-plane/internal/auth ./apps/control-plane/internal/storage -run 'Test.*Captcha|TestLogin' -count=1
corepack pnpm verify:contracts
```

实现完成并准备声明可用前，仍需要按项目规则做真实链路验证。

## 15. 真实验证

本功能涉及前端、Control Plane、数据库和登录 session，完成时不能只以单元测试或组件测试作为“可用”证明。

真实验证步骤：

1. 确认 dev services 状态：`scripts/dev-services.sh status`。
2. 如有变更，定向重启 Control Plane 和 Web。
3. 打开真实登录页。
4. 确认页面展示验证码图片。
5. 输入错误验证码，确认登录失败、验证码刷新、无 session。
6. 输入正确验证码和错误密码，确认账号密码错误、验证码刷新、失败日志写入。
7. 输入正确验证码和正确账号密码，确认登录成功并进入受保护页面。
8. 复用同一个验证码再次请求登录，确认失败。
9. 通过数据库或 API 验证 `web_login_logs` 有验证码失败记录。

收尾前必须使用项目内 `superteam-completion-check` skill。不得把 mock、组件测试、单元测试或构建通过表述为真实登录链路已验证。

## 16. 实施顺序

1. 后端 captcha domain/service 测试先行。
2. 新增 migration、sqlc 查询、repository 方法和生成产物。
3. 更新 OpenAPI 契约和 generated auth server types。
4. 新增 `GET /api/auth/captcha` handler。
5. 修改 `POST /api/auth/login` 强制校验验证码。
6. 更新前端 API client、AuthContext/AuthProvider 类型。
7. 更新 `UserAuthForm` 交互和测试。
8. 运行定向测试、契约验证和真实登录链路验证。
