# 登录图形验证码技术方案

日期：2026-06-30
状态：已确认，待实施计划

## 1. 目标

在 Web 控制台登录页增加图形验证码。所有登录请求都必须提交验证码，校验必须在 Control Plane 服务端完成，前端只负责展示、输入和刷新，不能成为安全边界。

已确认规则：

- 验证码为 4 位字符图片验证码。
- 每个验证码必须同时包含数字和字母。
- 所有登录都必须输入验证码，不做“失败几次后触发”。
- 验证码 challenge 存 PostgreSQL，不用 Redis，不用内存态。
- 验证码一次性使用，默认 5 分钟过期。
- 验证码答案不明文落库。

## 2. 范围

本次只覆盖登录验证码，不改造账号密码认证、session cookie、用户权限模型或登录后授权逻辑。

不做滑块、拼图、行为轨迹、第三方验证码，也不引入后台定时清理任务。过期 challenge 可以在生成新验证码或登录校验时顺手清理。

## 3. 当前代码入口

- 登录页：`apps/web/src/features/auth/sign-in/components/user-auth-form.tsx`
- 前端 auth client：`apps/web/src/lib/api/auth.ts`
- 前端 auth 状态：`apps/web/src/features/auth/auth-provider.tsx`
- Auth 契约：`contracts/control-plane/auth.yaml`
- 后端登录 handler：`apps/control-plane/internal/auth/handler.go`
- 后端登录 service：`apps/control-plane/internal/auth/service.go`
- 登录日志：`web_login_logs`

## 4. 接口设计

新增验证码接口：

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

登录接口请求体扩展：

```json
{
  "username": "admin",
  "password": "admin",
  "captcha_id": "00000000-0000-0000-0000-000000000000",
  "captcha_code": "A7K2"
}
```

`captcha_id` 和 `captcha_code` 为必填。缺失时返回 `400`。验证码错误、过期或已使用时返回 `401`，错误文案为“验证码不正确或已过期”。

## 5. 后端设计

Control Plane 负责生成、保存、校验、消费验证码。

新增验证码 challenge 表，建议字段：

- `id`
- `tenant_id`
- `answer_hash`
- `expires_at`
- `used_at`
- `client_ip`
- `user_agent`
- `created_at`
- `updated_at`

存储规则：

- `answer_hash` 存服务端密钥参与计算后的 hash，不保存明文答案。
- 登录校验时通过事务或行锁保证同一个验证码不能并发复用。
- 无论后续账号密码是否正确，验证码校验通过后都要消费。
- 验证码错误、过期、已使用也要让前端刷新验证码。

验证码生成规则：

- 长度固定 4 位。
- 至少 1 个数字，至少 1 个字母。
- 输入校验大小写不敏感，提交前后端都按大写处理。
- 图片返回 PNG data URL，加入轻量噪点或干扰线，但优先保证可读性。

## 6. 登录流程

页面加载：

1. 前端请求 `GET /api/auth/captcha`。
2. 前端展示图片，保存 `captcha_id`。
3. 用户输入账号、密码、验证码。

提交登录：

1. 前端提交 `username/password/captcha_id/captcha_code`。
2. 后端先校验验证码。
3. 验证码通过后再执行现有账号密码认证。
4. 账号密码通过后创建 session cookie。
5. 前端进入现有登录成功跳转流程。

失败处理：

- 验证码错误或过期：提示“验证码不正确或已过期”，刷新验证码并清空验证码输入。
- 账号密码错误：提示“用户名或密码不正确”，刷新验证码并清空验证码输入。
- 验证码加载失败：禁用登录按钮，提示“验证码加载失败，请刷新重试”。

刷新验证码不能清空账号和密码。

## 7. 前端设计

在密码输入框下方、登录按钮上方增加“图形验证码”区域：

- 输入框 label：`图形验证码`
- 图片展示验证码。
- 图片旁提供刷新按钮，`aria-label="刷新验证码"`。
- 提交前校验验证码必填且 4 位。
- 登录失败后自动刷新验证码并清空验证码字段。

视觉沿用现有登录页 v3 表单控件风格，不引入新的页面布局。

## 8. 审计

验证码失败要进入登录失败日志：

- `event_type = login_failed`
- `result = failed`
- `username` 使用请求中的账号快照。
- `failure_reason` 使用 `captcha_invalid` 或 `captcha_expired`。
- `client_ip` 和 `user_agent` 沿用现有登录日志采集逻辑。

账号密码错误继续沿用现有失败原因。

## 9. 验收标准

实现完成后至少验证：

1. 登录页能加载并显示验证码图片。
2. 不填验证码不能提交。
3. 错误验证码登录失败，并刷新验证码。
4. 正确验证码加错误密码登录失败，并刷新验证码。
5. 正确验证码加正确账号密码登录成功。
6. 同一个验证码不能重复使用。
7. 验证码失败写入登录失败日志。
8. 刷新验证码不清空账号和密码。

本功能涉及 Web、Control Plane、数据库和登录 session。最终声明可用前必须做真实登录链路验证，不能只用单元测试、组件测试或 mock API 作为完成证明。
