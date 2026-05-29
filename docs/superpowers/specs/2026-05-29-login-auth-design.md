# SuperTeam 登录认证系统设计

## 文档信息

- **创建日期**：2026-05-29
- **设计范围**：Web 登录页面 + Control Plane 认证 API（第一阶段）
- **参考设计**：PulseAI 登录页面视觉风格
- **实施阶段**：MVP 核心认证流程

## 一、项目背景

SuperTeam 是企业级数字员工控制平面，需要实现用户认证体系以保护控制台访问。第一阶段聚焦核心登录认证流程，为后续功能开发提供基础。

### 实施范围

**第一阶段（本设计）**：
- ✅ 用户登录认证（POST /api/auth/login）
- ✅ 获取当前用户信息（GET /api/auth/me）
- ✅ 用户登出（POST /api/auth/logout）
- ✅ 会话管理（Redis 存储，12 小时 TTL）
- ✅ 路由保护（ProtectedRoute 组件）
- ✅ 默认管理员账号（数据库迁移创建）
- ✅ 登录页面（参考 PulseAI 视觉风格）

**第二阶段（后续扩展）**：
- ❌ 用户管理 CRUD（创建/编辑/禁用用户）
- ❌ 密码修改功能
- ❌ 会话列表和强制登出
- ❌ 操作审计日志
- ❌ 权限系统和角色管理
- ❌ 租户隔离

## 二、整体架构

### 2.1 系统分层

```
┌─────────────────────────────────────────────────────────┐
│                     Web Console                          │
│  ┌──────────────┐  ┌─────────────┐  ┌────────────────┐ │
│  │  Login Page  │  │ AuthProvider│  │ Protected Routes│ │
│  │  (PulseAI    │  │  (Context + │  │                 │ │
│  │   Style)     │  │   Query)    │  │                 │ │
│  └──────────────┘  └─────────────┘  └────────────────┘ │
└─────────────────────────────────────────────────────────┘
                          │ HTTP + Cookie
                          ▼
┌─────────────────────────────────────────────────────────┐
│              Control Plane (Go)                          │
│  ┌──────────────────────────────────────────────────┐  │
│  │  API Layer (chi router + oapi-codegen)           │  │
│  │  POST /api/auth/login                            │  │
│  │  GET  /api/auth/me                               │  │
│  │  POST /api/auth/logout                           │  │
│  └──────────────────────────────────────────────────┘  │
│  ┌──────────────────────────────────────────────────┐  │
│  │  Auth Service                                     │  │
│  │  - bcrypt 密码验证                                │  │
│  │  - Session token 生成                             │  │
│  │  - Cookie 设置                                    │  │
│  └──────────────────────────────────────────────────┘  │
│  ┌──────────────────────────────────────────────────┐  │
│  │  Repository Layer                                 │  │
│  │  - User CRUD (pgx + sqlc)                        │  │
│  │  - Session CRUD (Redis)                          │  │
│  └──────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
                    │                    │
                    ▼                    ▼
         ┌──────────────────┐  ┌──────────────────┐
         │   PostgreSQL     │  │      Redis       │
         │  - auth_users    │  │  - sessions      │
         │  - migrations    │  │    (TTL 12h)     │
         └──────────────────┘  └──────────────────┘
```

### 2.2 认证流程

**登录流程**：
1. 用户在登录页输入用户名和密码
2. 前端调用 `POST /api/auth/login`
3. Go 后端验证用户名和密码（bcrypt）
4. 生成随机 session token（32 字节）
5. 将 token 的 SHA-256 hash 存入 Redis（TTL 12 小时）
6. 设置 HTTP-only Cookie（session_token）
7. 返回用户信息给前端
8. 前端更新 AuthProvider 状态，跳转到控制台首页

**会话验证流程**：
1. 前端请求受保护的 API 端点
2. 浏览器自动携带 Cookie（session_token）
3. Go 中间件从 Cookie 读取 token
4. 计算 token 的 SHA-256 hash
5. 从 Redis 查询会话信息
6. 验证会话是否过期
7. 更新 lastSeenAt 时间戳
8. 将用户信息注入 request context
9. 继续处理业务逻辑

**登出流程**：
1. 前端调用 `POST /api/auth/logout`
2. Go 后端从 Cookie 读取 token
3. 从 Redis 删除会话记录
4. 清除 Cookie（Max-Age=0）
5. 前端清除 AuthProvider 状态，跳转到登录页

### 2.3 技术选型

**后端**：
- 语言：Go 1.21+
- Web 框架：chi router
- 数据库：PostgreSQL 14+（用户数据）
- 缓存：Redis 7+（会话存储）
- 数据库访问：pgx + sqlc
- 数据库迁移：goose 或 Atlas
- API 契约：OpenAPI 3.0 + oapi-codegen
- 密码加密：bcrypt（cost factor 10）

**前端**：
- 框架：Next.js 16 (App Router)
- UI 库：React 19
- 状态管理：TanStack Query v5
- UI 组件：shadcn/ui + Radix UI
- 样式：Tailwind CSS 4
- 动画：motion/react (Framer Motion)
- 表单：React Hook Form + Zod
- 图标：lucide-react

**开发工具**：
- 测试：Go testing + testify, Vitest, Playwright
- 代码生成：oapi-codegen, sqlc
- 类型检查：TypeScript 5+

## 三、数据模型

### 3.1 PostgreSQL Schema

**auth_users 表**（用户基础信息）：

```sql
CREATE TABLE auth_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_login_at TIMESTAMPTZ
);

CREATE INDEX idx_auth_users_username ON auth_users(username);
CREATE INDEX idx_auth_users_status ON auth_users(status);

COMMENT ON TABLE auth_users IS '认证用户表';
COMMENT ON COLUMN auth_users.status IS '用户状态: active, disabled';
COMMENT ON COLUMN auth_users.password_hash IS 'bcrypt 密码哈希';
```

**初始数据**（通过 migration 插入）：

```sql
-- 默认管理员账号
-- 用户名: admin
-- 密码: admin
-- bcrypt hash (cost 10)
INSERT INTO auth_users (username, password_hash, status) 
VALUES (
    'admin', 
    '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy',
    'active'
);
```

**字段说明**：
- `id`：用户唯一标识（UUID）
- `username`：登录用户名（唯一索引）
- `password_hash`：bcrypt 加密后的密码哈希
- `status`：用户状态（active=正常，disabled=禁用）
- `created_at`：创建时间
- `updated_at`：更新时间
- `last_login_at`：最后登录时间（登录成功时更新）

### 3.2 Redis Session 结构

**Key 格式**：`session:{sha256(token)}`

**Value**（JSON 字符串）：
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "user_id": "123e4567-e89b-12d3-a456-426614174000",
  "expires_at": "2026-05-29T22:00:00Z",
  "last_seen_at": "2026-05-29T10:00:00Z",
  "client_ip": "127.0.0.1",
  "user_agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)..."
}
```

**TTL**：43200 秒（12 小时）

**字段说明**：
- `id`：会话唯一标识（UUID）
- `user_id`：关联的用户 ID
- `expires_at`：会话过期时间（ISO 8601 格式）
- `last_seen_at`：最后活跃时间（每次请求更新）
- `client_ip`：客户端 IP 地址（用于审计）
- `user_agent`：客户端 User-Agent（用于审计）

**为什么使用 SHA-256 hash 作为 Key**：
- 防止 token 泄露：即使 Redis 数据被导出，攻击者也无法反推原始 token
- 固定长度：SHA-256 输出固定 64 字符（hex），便于索引
- 快速查找：hash 计算开销小，查询性能高

### 3.3 Go 数据结构

```go
// internal/auth/types.go

// User 用户模型
type User struct {
    ID           string     `db:"id"`
    Username     string     `db:"username"`
    PasswordHash string     `db:"password_hash"`
    Status       string     `db:"status"`
    CreatedAt    time.Time  `db:"created_at"`
    UpdatedAt    time.Time  `db:"updated_at"`
    LastLoginAt  *time.Time `db:"last_login_at"`
}

// Session 会话模型
type Session struct {
    ID         string    `json:"id"`
    UserID     string    `json:"user_id"`
    ExpiresAt  time.Time `json:"expires_at"`
    LastSeenAt time.Time `json:"last_seen_at"`
    ClientIP   string    `json:"client_ip"`
    UserAgent  string    `json:"user_agent"`
}

// UserSummary 用户摘要（API 响应）
type UserSummary struct {
    ID       string `json:"id"`
    Username string `json:"username"`
    Status   string `json:"status"`
}
```


## 四、API 契约设计

### 4.1 OpenAPI Spec 结构

**文件位置**：`contracts/control-plane/auth.yaml`

**核心端点定义**：

```yaml
openapi: 3.0.3
info:
  title: SuperTeam Control Plane - Auth API
  version: 1.0.0
  description: 用户认证相关 API

servers:
  - url: http://localhost:8080
    description: 本地开发环境

paths:
  /api/auth/login:
    post:
      summary: 用户登录
      operationId: login
      tags:
        - Auth
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required:
                - username
                - password
              properties:
                username:
                  type: string
                  minLength: 1
                  maxLength: 255
                  example: admin
                password:
                  type: string
                  format: password
                  minLength: 1
                  example: admin
      responses:
        '200':
          description: 登录成功
          headers:
            Set-Cookie:
              schema:
                type: string
                example: session_token=abc123; HttpOnly; Secure; SameSite=Lax
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/LoginResponse'
        '401':
          $ref: '#/components/responses/Unauthorized'
        '403':
          $ref: '#/components/responses/Forbidden'

  /api/auth/me:
    get:
      summary: 获取当前登录用户信息
      operationId: getCurrentUser
      tags:
        - Auth
      security:
        - cookieAuth: []
      responses:
        '200':
          description: 成功
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/CurrentUserResponse'
        '401':
          $ref: '#/components/responses/Unauthorized'

  /api/auth/logout:
    post:
      summary: 用户登出
      operationId: logout
      tags:
        - Auth
      security:
        - cookieAuth: []
      responses:
        '200':
          description: 登出成功
          headers:
            Set-Cookie:
              schema:
                type: string
                example: session_token=; Max-Age=0
          content:
            application/json:
              schema:
                type: object
                properties:
                  message:
                    type: string
                    example: logout success

components:
  securitySchemes:
    cookieAuth:
      type: apiKey
      in: cookie
      name: session_token

  schemas:
    LoginResponse:
      type: object
      required: [user]
      properties:
        user:
          $ref: '#/components/schemas/UserSummary'

    CurrentUserResponse:
      type: object
      required: [user]
      properties:
        user:
          $ref: '#/components/schemas/UserSummary'

    UserSummary:
      type: object
      required: [id, username, status]
      properties:
        id:
          type: string
          format: uuid
        username:
          type: string
        status:
          type: string
          enum: [active, disabled]

    ErrorResponse:
      type: object
      required: [error]
      properties:
        error:
          type: string
        code:
          type: string

  responses:
    Unauthorized:
      description: 未登录或凭据无效
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/ErrorResponse'
    Forbidden:
      description: 账号已被禁用
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/ErrorResponse'
```

### 4.2 Cookie 配置

**Cookie 名称**：`session_token`

**Cookie 属性**：
- **HttpOnly**: `true` - 防止 JavaScript 访问，降低 XSS 风险
- **Secure**: `true` - 仅通过 HTTPS 传输（生产环境）
- **SameSite**: `Lax` - 防止 CSRF 攻击，允许顶级导航携带 Cookie
- **Path**: `/` - 全站可用
- **Max-Age**: `43200` 秒（12 小时）

**为什么选择 SameSite=Lax**：
- `Strict`：最安全，但会导致从外部链接跳转时丢失登录状态
- `Lax`：平衡安全性和用户体验，允许 GET 请求携带 Cookie
- `None`：不推荐，需要配合 Secure 使用，且容易受 CSRF 攻击

### 4.3 API 错误码设计

| HTTP 状态码 | 错误信息 | 场景 |
|------------|---------|------|
| 401 | 用户名或密码错误 | 登录凭据无效 |
| 401 | 未登录或会话已过期 | 访问受保护资源但未登录 |
| 403 | 账号已被禁用 | 用户状态为 disabled |
| 500 | 登录失败，请稍后重试 | 服务器内部错误 |


## 五、Go 后端实现

### 5.1 目录结构

```
apps/control-plane/
├── cmd/
│   └── control-plane/
│       └── main.go                    # 入口，启动 HTTP 服务
├── internal/
│   ├── auth/
│   │   ├── service.go                 # 认证服务
│   │   ├── handler.go                 # HTTP handlers
│   │   ├── middleware.go              # 认证中间件
│   │   ├── repository.go              # 数据访问接口
│   │   ├── repository_pg.go           # PostgreSQL 实现
│   │   ├── repository_redis.go        # Redis 实现
│   │   ├── types.go                   # 类型定义
│   │   └── errors.go                  # 错误定义
│   ├── storage/
│   │   ├── postgres.go                # PostgreSQL 连接池
│   │   └── redis.go                   # Redis 客户端
│   ├── config/
│   │   └── config.go                  # 配置加载
│   └── api/
│       ├── router.go                  # chi router 配置
│       └── server.go                  # HTTP server 启动
├── migrations/
│   └── 001_create_auth_users.sql      # 数据库迁移
├── go.mod
└── go.sum
```

### 5.2 Service 层实现要点

**核心方法**：

```go
// HashPassword 使用 bcrypt 加密密码（cost 10）
func (s *Service) HashPassword(password string) (string, error)

// VerifyPassword 验证密码
func (s *Service) VerifyPassword(password, hash string) error

// GenerateToken 生成 32 字节随机 token
func (s *Service) GenerateToken() (string, error)

// HashToken 计算 token 的 SHA-256 hash
func (s *Service) HashToken(token string) string

// Login 登录逻辑
// 返回：session, user, token, error
func (s *Service) Login(ctx context.Context, username, password, clientIP, userAgent string) (*Session, *User, string, error)

// GetUserBySessionToken 通过 token 获取用户
func (s *Service) GetUserBySessionToken(ctx context.Context, token string) (*User, error)

// Logout 登出
func (s *Service) Logout(ctx context.Context, token string) error
```

**Login 流程**：
1. 根据 username 查询用户
2. 验证用户状态（active/disabled）
3. 使用 bcrypt 验证密码
4. 生成随机 session token（32 字节）
5. 创建会话记录（存 Redis）
6. 更新用户最后登录时间（PostgreSQL）
7. 返回 session、user、token

**GetUserBySessionToken 流程**：
1. 计算 token 的 SHA-256 hash
2. 从 Redis 查询会话
3. 验证会话是否过期
4. 根据 user_id 查询用户
5. 验证用户状态
6. 更新会话 last_seen_at
7. 返回用户信息

### 5.3 Handler 层实现要点

**Login Handler**：

```go
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
    var req LoginRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        respondError(w, http.StatusBadRequest, "invalid request body")
        return
    }
    
    clientIP := getClientIP(r)
    userAgent := r.UserAgent()
    
    session, user, token, err := h.service.Login(r.Context(), req.Username, req.Password, clientIP, userAgent)
    if err != nil {
        switch {
        case errors.Is(err, ErrInvalidCredentials):
            respondError(w, http.StatusUnauthorized, "用户名或密码错误")
        case errors.Is(err, ErrUserDisabled):
            respondError(w, http.StatusForbidden, "账号已被禁用")
        default:
            respondError(w, http.StatusInternalServerError, "登录失败，请稍后重试")
        }
        return
    }
    
    // 设置 Cookie
    http.SetCookie(w, &http.Cookie{
        Name:     "session_token",
        Value:    token,
        Path:     "/",
        MaxAge:   int(SessionTTL.Seconds()),
        HttpOnly: true,
        Secure:   h.config.IsProduction,
        SameSite: http.SameSiteLaxMode,
    })
    
    respondJSON(w, http.StatusOK, LoginResponse{
        User: toUserSummary(user),
    })
}
```

**GetCurrentUser Handler**：

```go
func (h *Handler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
    // 从 context 获取用户（由中间件注入）
    user, ok := r.Context().Value(UserContextKey).(*User)
    if !ok {
        respondError(w, http.StatusUnauthorized, "未登录")
        return
    }
    
    respondJSON(w, http.StatusOK, CurrentUserResponse{
        User: toUserSummary(user),
    })
}
```

**Logout Handler**：

```go
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
    cookie, err := r.Cookie("session_token")
    if err != nil {
        respondError(w, http.StatusUnauthorized, "未登录")
        return
    }
    
    if err := h.service.Logout(r.Context(), cookie.Value); err != nil {
        // 即使删除失败也清除 Cookie
    }
    
    // 清除 Cookie
    http.SetCookie(w, &http.Cookie{
        Name:     "session_token",
        Value:    "",
        Path:     "/",
        MaxAge:   -1,
        HttpOnly: true,
        Secure:   h.config.IsProduction,
        SameSite: http.SameSiteLaxMode,
    })
    
    respondJSON(w, http.StatusOK, map[string]string{
        "message": "logout success",
    })
}
```

### 5.4 Middleware 实现要点

**RequireAuth 中间件**：

```go
func RequireAuth(service *Service) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // 1. 从 Cookie 读取 token
            cookie, err := r.Cookie("session_token")
            if err != nil {
                respondError(w, http.StatusUnauthorized, "未登录或会话已过期")
                return
            }
            
            // 2. 验证 token 并获取用户
            user, err := service.GetUserBySessionToken(r.Context(), cookie.Value)
            if err != nil {
                respondError(w, http.StatusUnauthorized, "未登录或会话已过期")
                return
            }
            
            // 3. 将用户信息注入 context
            ctx := context.WithValue(r.Context(), UserContextKey, user)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

### 5.5 Repository 实现要点

**PostgreSQL User Repository**（使用 sqlc）：

```sql
-- queries/auth.sql

-- name: GetUserByUsername :one
SELECT * FROM auth_users WHERE username = $1 LIMIT 1;

-- name: GetUserByID :one
SELECT * FROM auth_users WHERE id = $1 LIMIT 1;

-- name: UpdateUserLastLogin :exec
UPDATE auth_users SET last_login_at = $2 WHERE id = $1;
```

**Redis Session Repository**：

```go
func (r *RedisSessionRepository) Create(ctx context.Context, session *Session, token string) error {
    key := fmt.Sprintf("session:%s", r.hashToken(token))
    data, err := json.Marshal(session)
    if err != nil {
        return err
    }
    
    ttl := time.Until(session.ExpiresAt)
    return r.client.Set(ctx, key, data, ttl).Err()
}

func (r *RedisSessionRepository) GetByToken(ctx context.Context, token string) (*Session, error) {
    key := fmt.Sprintf("session:%s", r.hashToken(token))
    data, err := r.client.Get(ctx, key).Bytes()
    if err != nil {
        return nil, err
    }
    
    var session Session
    if err := json.Unmarshal(data, &session); err != nil {
        return nil, err
    }
    
    return &session, nil
}

func (r *RedisSessionRepository) Delete(ctx context.Context, token string) error {
    key := fmt.Sprintf("session:%s", r.hashToken(token))
    return r.client.Del(ctx, key).Err()
}

func (r *RedisSessionRepository) UpdateLastSeen(ctx context.Context, token string, lastSeenAt time.Time) error {
    key := fmt.Sprintf("session:%s", r.hashToken(token))
    
    // 获取现有会话
    session, err := r.GetByToken(ctx, token)
    if err != nil {
        return err
    }
    
    // 更新 last_seen_at
    session.LastSeenAt = lastSeenAt
    data, err := json.Marshal(session)
    if err != nil {
        return err
    }
    
    // 重新设置（保持原有 TTL）
    ttl := time.Until(session.ExpiresAt)
    return r.client.Set(ctx, key, data, ttl).Err()
}
```

### 5.6 数据库迁移

**文件**：`migrations/001_create_auth_users.sql`

```sql
-- +goose Up
CREATE TABLE auth_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_login_at TIMESTAMPTZ
);

CREATE INDEX idx_auth_users_username ON auth_users(username);
CREATE INDEX idx_auth_users_status ON auth_users(status);

COMMENT ON TABLE auth_users IS '认证用户表';
COMMENT ON COLUMN auth_users.status IS '用户状态: active, disabled';
COMMENT ON COLUMN auth_users.password_hash IS 'bcrypt 密码哈希';

-- 插入默认管理员账号
-- 用户名: admin
-- 密码: admin
INSERT INTO auth_users (username, password_hash, status) 
VALUES (
    'admin', 
    '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy',
    'active'
);

-- +goose Down
DROP TABLE auth_users;
```


## 六、前端实现

### 6.1 目录结构

```
packages/
├── api-client/
│   ├── src/
│   │   ├── generated/          # oapi-codegen 生成的类型
│   │   │   └── auth.ts
│   │   ├── client.ts           # HTTP 客户端封装
│   │   ├── auth-api.ts         # 认证 API 封装
│   │   └── index.ts
│   └── package.json
├── core/
│   ├── src/
│   │   ├── auth/
│   │   │   ├── auth-provider.tsx    # AuthProvider 组件
│   │   │   ├── auth-context.ts      # Context 定义
│   │   │   ├── use-auth.ts          # useAuth hook
│   │   │   ├── use-login.ts         # useLogin mutation
│   │   │   ├── use-logout.ts        # useLogout mutation
│   │   │   └── queries.ts           # Query 配置
│   │   └── index.ts
│   └── package.json
├── ui/
│   ├── src/
│   │   ├── components/
│   │   │   ├── button.tsx           # 已有
│   │   │   ├── input.tsx            # 已有
│   │   │   ├── label.tsx            # 需新增
│   │   │   └── alert.tsx            # 需新增
│   │   └── index.ts
│   └── package.json
└── views/
    ├── src/
    │   ├── auth/
    │   │   ├── login-page.tsx       # 登录页面
    │   │   └── protected-route.tsx  # 路由保护
    │   └── index.ts
    └── package.json

apps/web/
├── app/
│   ├── login/
│   │   └── page.tsx                 # Next.js 登录路由
│   ├── layout.tsx                   # 根布局（AuthProvider）
│   └── page.tsx                     # 首页（重定向）
└── public/
    └── assets/
        └── branding/
            └── superteam-logo.png   # Logo
```

### 6.2 API Client 实现

**HTTP 客户端**（`packages/api-client/src/client.ts`）：

```typescript
export class ApiError extends Error {
  constructor(
    public status: number,
    public code?: string,
    message?: string
  ) {
    super(message || 'Request failed')
    this.name = 'ApiError'
  }
}

export async function request<T>(
  url: string,
  options: RequestInit = {}
): Promise<T> {
  const response = await fetch(url, {
    ...options,
    credentials: 'include', // 携带 Cookie
    headers: {
      'Content-Type': 'application/json',
      ...options.headers,
    },
  })

  if (!response.ok) {
    const error = await response.json().catch(() => ({}))
    throw new ApiError(
      response.status,
      error.code,
      error.error || response.statusText
    )
  }

  return response.json()
}
```

**认证 API**（`packages/api-client/src/auth-api.ts`）：

```typescript
import { request } from './client'
import type { LoginResponse, CurrentUserResponse, UserSummary } from './generated/auth'

export const authApi = {
  login: (username: string, password: string) =>
    request<LoginResponse>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),

  getCurrentUser: () =>
    request<CurrentUserResponse>('/api/auth/me'),

  logout: () =>
    request<{ message: string }>('/api/auth/logout', {
      method: 'POST',
    }),
}
```

### 6.3 AuthProvider 实现

**Context 定义**（`packages/core/src/auth/auth-context.ts`）：

```typescript
import { createContext } from 'react'
import type { UserSummary } from '@superteam/api-client'

export interface AuthContextValue {
  user?: UserSummary
  isLoading: boolean
  isAuthenticated: boolean
  refetch: () => Promise<void>
}

export const AuthContext = createContext<AuthContextValue | undefined>(undefined)
```

**AuthProvider**（`packages/core/src/auth/auth-provider.tsx`）：

```tsx
import { type ReactNode, useMemo } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { authApi } from '@superteam/api-client'
import { AuthContext } from './auth-context'

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  
  const { data, isLoading } = useQuery({
    queryKey: ['auth', 'me'],
    queryFn: async () => {
      const response = await authApi.getCurrentUser()
      return response.user
    },
    retry: false,
    staleTime: 5 * 60 * 1000, // 5分钟
  })

  const value = useMemo(() => ({
    user: data,
    isLoading,
    isAuthenticated: !!data,
    refetch: async () => {
      await queryClient.invalidateQueries({ queryKey: ['auth', 'me'] })
    },
  }), [data, isLoading, queryClient])

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
```

**useAuth hook**（`packages/core/src/auth/use-auth.ts`）：

```typescript
import { useContext } from 'react'
import { AuthContext } from './auth-context'

export function useAuth() {
  const context = useContext(AuthContext)
  if (!context) {
    throw new Error('useAuth must be used within AuthProvider')
  }
  return context
}
```

**useLogin hook**（`packages/core/src/auth/use-login.ts`）：

```typescript
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { authApi } from '@superteam/api-client'

export function useLogin() {
  const queryClient = useQueryClient()
  
  return useMutation({
    mutationFn: async (credentials: { username: string; password: string }) => {
      const response = await authApi.login(credentials.username, credentials.password)
      return response.user
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['auth', 'me'] })
    },
  })
}
```

**useLogout hook**（`packages/core/src/auth/use-logout.ts`）：

```typescript
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { authApi } from '@superteam/api-client'

export function useLogout() {
  const queryClient = useQueryClient()
  
  return useMutation({
    mutationFn: async () => {
      await authApi.logout()
    },
    onSuccess: () => {
      queryClient.setQueryData(['auth', 'me'], null)
    },
  })
}
```

### 6.4 登录页面实现

**LoginPage 组件**（`packages/views/src/auth/login-page.tsx`）：

```tsx
import { type FormEvent, useState } from 'react'
import { motion } from 'motion/react'
import { useRouter } from 'next/navigation'
import { Loader2, LogIn } from 'lucide-react'
import { useLogin } from '@superteam/core'
import { Button, Input, Label, Alert, AlertDescription } from '@superteam/ui'

export function LoginPage() {
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string>()
  const loginMutation = useLogin()
  const router = useRouter()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(undefined)
    
    try {
      await loginMutation.mutateAsync({ username, password })
      router.push('/overview')
    } catch (err) {
      setError((err as Error).message)
    }
  }

  return (
    <LoginFrame>
      <motion.div
        className="relative z-10 flex w-full max-w-[420px] flex-col items-center gap-6"
        initial={{ opacity: 0, y: 18 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.55, ease: 'easeOut' }}
      >
        {/* Logo 区域 */}
        <motion.div
          className="relative flex w-full justify-center"
          initial={{ opacity: 0, scale: 0.94 }}
          animate={{ opacity: 1, scale: 1 }}
          transition={{ delay: 0.08, duration: 0.5, ease: 'easeOut' }}
        >
          <motion.div
            aria-hidden="true"
            className="absolute top-1/2 h-32 w-80 -translate-y-1/2 rounded-full bg-cyan-100/80 blur-3xl"
            animate={{ scale: [1, 1.08, 1], opacity: [0.55, 0.82, 0.55] }}
            transition={{ duration: 6, repeat: Infinity, ease: 'easeInOut' }}
          />
          <img
            src="/assets/branding/superteam-logo.png"
            alt="SuperTeam"
            className="relative h-auto w-full max-w-[250px] object-contain drop-shadow-[0_22px_44px_rgba(56,108,166,0.22)]"
          />
        </motion.div>

        {/* 登录表单卡片 */}
        <motion.section
          className="w-full rounded-2xl border border-white/70 bg-white/62 p-5 shadow-[0_28px_90px_rgba(45,70,120,0.16)] backdrop-blur-xl sm:p-6"
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.18, duration: 0.5, ease: 'easeOut' }}
        >
          <h1 className="mb-5 text-center text-2xl font-semibold tracking-normal text-slate-950">
            账号登录
          </h1>

          <form className="space-y-4" onSubmit={handleSubmit}>
            <div className="space-y-2">
              <Label htmlFor="login-username">账号</Label>
              <Input
                id="login-username"
                name="username"
                autoComplete="username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className="h-11 border-white/75 bg-white/82 shadow-[0_12px_28px_rgba(45,70,120,0.08)] backdrop-blur-sm"
                required
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="login-password">密码</Label>
              <Input
                id="login-password"
                name="password"
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className="h-11 border-white/75 bg-white/82 shadow-[0_12px_28px_rgba(45,70,120,0.08)] backdrop-blur-sm"
                required
              />
            </div>

            {error && (
              <Alert variant="destructive" className="bg-white/76">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}

            <Button
              className="h-12 w-full bg-gradient-to-r from-slate-950 via-slate-900 to-blue-950 text-white shadow-[0_18px_42px_rgba(15,23,42,0.22)] hover:from-slate-900 hover:to-blue-900"
              type="submit"
              disabled={loginMutation.isPending}
            >
              {loginMutation.isPending ? (
                <>
                  <Loader2 className="animate-spin" />
                  登录中...
                </>
              ) : (
                <>
                  <LogIn />
                  登录
                </>
              )}
            </Button>
          </form>
        </motion.section>
      </motion.div>
    </LoginFrame>
  )
}

// 登录页面背景框架
function LoginFrame({ children }: { children: React.ReactNode }) {
  return (
    <main className="relative flex min-h-svh overflow-hidden bg-[#f5faff] px-6 py-10 text-slate-950">
      {/* 背景装饰层 */}
      <motion.div
        aria-hidden="true"
        className="pointer-events-none absolute inset-0 overflow-hidden"
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ duration: 0.7, ease: 'easeOut' }}
      >
        {/* 径向渐变背景 */}
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_50%_24%,rgba(255,255,255,0.98)_0%,rgba(250,253,255,0.78)_25%,rgba(224,241,255,0.78)_52%,rgba(211,229,245,0.88)_100%)]" />
        
        {/* 线性渐变叠加 */}
        <div className="absolute inset-0 bg-[linear-gradient(118deg,rgba(20,184,166,0.13)_0%,transparent_29%,transparent_62%,rgba(59,130,246,0.12)_100%)]" />
        
        {/* 网格纹理 */}
        <div className="absolute inset-0 opacity-[0.16] [background-image:linear-gradient(rgba(14,116,144,0.16)_1px,transparent_1px),linear-gradient(90deg,rgba(14,116,144,0.16)_1px,transparent_1px)] [background-size:68px_68px]" />
        
        {/* 动画光晕 1 */}
        <motion.div
          className="absolute left-[-18%] top-[45%] h-[24rem] w-[82rem] -rotate-10 rounded-full border border-cyan-200/55 bg-gradient-to-r from-transparent via-white/62 to-transparent blur-[1px]"
          animate={{ x: [0, 28, 0], opacity: [0.36, 0.64, 0.36] }}
          transition={{ duration: 11, repeat: Infinity, ease: 'easeInOut' }}
        />
        
        {/* 动画光晕 2 */}
        <motion.div
          className="absolute right-[-20%] top-[39%] h-[22rem] w-[78rem] -rotate-10 rounded-full border border-blue-200/45 bg-gradient-to-r from-transparent via-cyan-100/38 to-transparent blur-[1px]"
          animate={{ x: [0, -26, 0], opacity: [0.3, 0.55, 0.3] }}
          transition={{ duration: 13, repeat: Infinity, ease: 'easeInOut' }}
        />
        
        {/* 中心圆环 */}
        <motion.div
          className="absolute left-1/2 top-[22%] h-[30rem] w-[30rem] -translate-x-1/2 rounded-full border border-white/75 shadow-[0_0_86px_rgba(14,165,233,0.18)]"
          animate={{ scale: [1, 1.04, 1], opacity: [0.3, 0.56, 0.3] }}
          transition={{ duration: 8, repeat: Infinity, ease: 'easeInOut' }}
        />
      </motion.div>

      {/* 内容区域 */}
      <div className="relative z-10 mx-auto flex w-full items-center justify-center">
        {children}
      </div>
    </main>
  )
}
```

### 6.5 路由保护实现

**ProtectedRoute 组件**（`packages/views/src/auth/protected-route.tsx`）：

```tsx
import { type ReactNode, useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { useAuth } from '@superteam/core'
import { Loader2 } from 'lucide-react'

export function ProtectedRoute({ children }: { children: ReactNode }) {
  const { isAuthenticated, isLoading } = useAuth()
  const router = useRouter()

  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      router.push('/login')
    }
  }, [isAuthenticated, isLoading, router])

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    )
  }

  if (!isAuthenticated) {
    return null
  }

  return <>{children}</>
}
```

### 6.6 Next.js 集成

**根布局**（`apps/web/app/layout.tsx`）：

```tsx
import { type ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AuthProvider } from '@superteam/core'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
})

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="zh-CN">
      <body>
        <QueryClientProvider client={queryClient}>
          <AuthProvider>
            {children}
          </AuthProvider>
        </QueryClientProvider>
      </body>
    </html>
  )
}
```

**登录页路由**（`apps/web/app/login/page.tsx`）：

```tsx
'use client'

import { LoginPage } from '@superteam/views'

export default function LoginRoute() {
  return <LoginPage />
}
```

**首页重定向**（`apps/web/app/page.tsx`）：

```tsx
'use client'

import { useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { useAuth } from '@superteam/core'

export default function HomePage() {
  const { isAuthenticated, isLoading } = useAuth()
  const router = useRouter()

  useEffect(() => {
    if (!isLoading) {
      router.push(isAuthenticated ? '/overview' : '/login')
    }
  }, [isAuthenticated, isLoading, router])

  return null
}
```


## 七、错误处理和安全性

### 7.1 错误处理策略

**Go 后端错误分类**：

```go
// internal/auth/errors.go
var (
    ErrInvalidCredentials = errors.New("invalid username or password")
    ErrUserDisabled       = errors.New("user account is disabled")
    ErrSessionExpired     = errors.New("session expired")
    ErrSessionNotFound    = errors.New("session not found")
    ErrUnauthorized       = errors.New("unauthorized")
)
```

**Handler 错误处理原则**：
- 登录失败统一返回"用户名或密码错误"，不泄露用户是否存在
- 区分 401（未认证）和 403（已认证但无权限）
- 服务器内部错误返回通用错误信息，详细错误记录日志
- 错误响应包含 `error` 字段（必需）和 `code` 字段（可选）

**前端错误处理**：

```typescript
// ApiError 类
export class ApiError extends Error {
  constructor(
    public status: number,
    public code?: string,
    message?: string
  ) {
    super(message || 'Request failed')
    this.name = 'ApiError'
  }
}

// 错误处理示例
try {
  await loginMutation.mutateAsync({ username, password })
} catch (err) {
  if (err instanceof ApiError) {
    if (err.status === 401) {
      setError('用户名或密码错误')
    } else if (err.status === 403) {
      setError('账号已被禁用')
    } else {
      setError('登录失败，请稍后重试')
    }
  } else {
    setError('网络错误，请检查连接')
  }
}
```

### 7.2 安全性措施

**密码安全**：
- ✅ 使用 bcrypt 加密，cost factor 10
- ✅ 密码明文不记录日志
- ✅ 登录失败不泄露用户是否存在
- ✅ 密码字段使用 `type="password"` 和 `autocomplete="current-password"`

**Session 安全**：
- ✅ Token 使用 `crypto/rand` 生成 32 字节随机值
- ✅ Redis 存储 token 的 SHA-256 hash，不存储明文
- ✅ 会话 12 小时自动过期（Redis TTL）
- ✅ 记录 client IP 和 user agent（用于审计）
- ✅ 每次请求更新 `last_seen_at`（活跃检测）

**Cookie 安全**：
- ✅ `HttpOnly=true`：防止 JavaScript 访问，降低 XSS 风险
- ✅ `Secure=true`：生产环境强制 HTTPS
- ✅ `SameSite=Lax`：防止 CSRF，允许顶级导航
- ✅ `Path=/`：全站可用
- ✅ 登出时立即清除（`Max-Age=-1`）

**API 安全**：
- ✅ 认证中间件验证所有受保护端点
- ✅ 登录失败不返回详细错误
- ⏳ 速率限制（后续通过中间件添加）
- ⏳ IP 白名单（可选，企业环境）

**前端安全**：
- ✅ 不在 localStorage/sessionStorage 存储敏感信息
- ✅ 401 响应自动跳转登录页
- ✅ API 请求自动携带 Cookie（`credentials: 'include'`）
- ✅ 密码输入框使用正确的 autocomplete 属性

**日志和审计**：

```go
// 登录成功
log.Info("user login success",
    "user_id", user.ID,
    "username", user.Username,
    "client_ip", clientIP,
    "user_agent", userAgent,
)

// 登录失败
log.Warn("user login failed",
    "username", username,
    "client_ip", clientIP,
    "reason", "invalid_credentials",
)

// 会话验证失败
log.Warn("session validation failed",
    "reason", "expired",
    "client_ip", clientIP,
)
```

### 7.3 安全检查清单

**部署前检查**：
- [ ] 生产环境 Cookie `Secure=true`
- [ ] HTTPS 证书配置正确
- [ ] Redis 密码保护已启用
- [ ] PostgreSQL 连接使用 SSL
- [ ] 默认 admin 密码已修改
- [ ] 日志不包含敏感信息（密码、token）
- [ ] CORS 配置正确（仅允许控制台域名）
- [ ] 速率限制已启用

**运行时监控**：
- [ ] 登录失败率监控（检测暴力破解）
- [ ] 会话创建速率监控（检测异常登录）
- [ ] Redis 连接池监控
- [ ] PostgreSQL 慢查询监控

## 八、测试策略

### 8.1 Go 后端测试

**Service 层测试**（`internal/auth/service_test.go`）：

```go
func TestService_HashPassword(t *testing.T) {
    s := NewService(nil, nil)
    hash, err := s.HashPassword("test123")
    assert.NoError(t, err)
    assert.NotEmpty(t, hash)
    assert.NotEqual(t, "test123", hash)
}

func TestService_VerifyPassword(t *testing.T) {
    s := NewService(nil, nil)
    hash, _ := s.HashPassword("test123")
    
    // 正确密码
    err := s.VerifyPassword("test123", hash)
    assert.NoError(t, err)
    
    // 错误密码
    err = s.VerifyPassword("wrong", hash)
    assert.Error(t, err)
}

func TestService_Login_Success(t *testing.T) {
    mockUserRepo := &MockUserRepository{}
    mockSessionRepo := &MockSessionRepository{}
    s := NewService(mockUserRepo, mockSessionRepo)
    
    // Mock 用户数据
    hash, _ := s.HashPassword("admin")
    mockUserRepo.On("GetByUsername", mock.Anything, "admin").Return(&User{
        ID:           "user-123",
        Username:     "admin",
        PasswordHash: hash,
        Status:       "active",
    }, nil)
    
    mockSessionRepo.On("Create", mock.Anything, mock.Anything, mock.Anything).Return(nil)
    mockUserRepo.On("UpdateLastLogin", mock.Anything, "user-123", mock.Anything).Return(nil)
    
    // 执行登录
    session, user, token, err := s.Login(context.Background(), "admin", "admin", "127.0.0.1", "test-agent")
    
    assert.NoError(t, err)
    assert.NotNil(t, session)
    assert.NotNil(t, user)
    assert.NotEmpty(t, token)
    assert.Equal(t, "admin", user.Username)
}

func TestService_Login_InvalidPassword(t *testing.T) {
    mockUserRepo := &MockUserRepository{}
    s := NewService(mockUserRepo, nil)
    
    hash, _ := s.HashPassword("admin")
    mockUserRepo.On("GetByUsername", mock.Anything, "admin").Return(&User{
        ID:           "user-123",
        Username:     "admin",
        PasswordHash: hash,
        Status:       "active",
    }, nil)
    
    _, _, _, err := s.Login(context.Background(), "admin", "wrong", "127.0.0.1", "test-agent")
    
    assert.Error(t, err)
    assert.Equal(t, ErrInvalidCredentials, err)
}

func TestService_Login_UserDisabled(t *testing.T) {
    mockUserRepo := &MockUserRepository{}
    s := NewService(mockUserRepo, nil)
    
    hash, _ := s.HashPassword("admin")
    mockUserRepo.On("GetByUsername", mock.Anything, "admin").Return(&User{
        ID:           "user-123",
        Username:     "admin",
        PasswordHash: hash,
        Status:       "disabled",
    }, nil)
    
    _, _, _, err := s.Login(context.Background(), "admin", "admin", "127.0.0.1", "test-agent")
    
    assert.Error(t, err)
    assert.Equal(t, ErrUserDisabled, err)
}
```

**Handler 层测试**（`internal/auth/handler_test.go`）：

```go
func TestHandler_Login_Success(t *testing.T) {
    mockService := &MockService{}
    handler := NewHandler(mockService, &Config{IsProduction: false})
    
    mockService.On("Login", mock.Anything, "admin", "admin", mock.Anything, mock.Anything).
        Return(&Session{ID: "sess-123"}, &User{ID: "user-123", Username: "admin", Status: "active"}, "token-abc", nil)
    
    req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"username":"admin","password":"admin"}`))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    
    handler.Login(w, req)
    
    assert.Equal(t, http.StatusOK, w.Code)
    
    // 验证 Cookie
    cookies := w.Result().Cookies()
    assert.Len(t, cookies, 1)
    assert.Equal(t, "session_token", cookies[0].Name)
    assert.Equal(t, "token-abc", cookies[0].Value)
    assert.True(t, cookies[0].HttpOnly)
}

func TestHandler_Login_InvalidCredentials(t *testing.T) {
    mockService := &MockService{}
    handler := NewHandler(mockService, &Config{})
    
    mockService.On("Login", mock.Anything, "admin", "wrong", mock.Anything, mock.Anything).
        Return(nil, nil, "", ErrInvalidCredentials)
    
    req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"username":"admin","password":"wrong"}`))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    
    handler.Login(w, req)
    
    assert.Equal(t, http.StatusUnauthorized, w.Code)
    
    var resp map[string]string
    json.NewDecoder(w.Body).Decode(&resp)
    assert.Contains(t, resp["error"], "用户名或密码错误")
}
```

**Middleware 测试**（`internal/auth/middleware_test.go`）：

```go
func TestRequireAuth_ValidSession(t *testing.T) {
    mockService := &MockService{}
    middleware := RequireAuth(mockService)
    
    mockService.On("GetUserBySessionToken", mock.Anything, "valid-token").
        Return(&User{ID: "user-123", Username: "admin", Status: "active"}, nil)
    
    handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        user := r.Context().Value(UserContextKey).(*User)
        assert.Equal(t, "admin", user.Username)
        w.WriteHeader(http.StatusOK)
    }))
    
    req := httptest.NewRequest("GET", "/api/protected", nil)
    req.AddCookie(&http.Cookie{Name: "session_token", Value: "valid-token"})
    w := httptest.NewRecorder()
    
    handler.ServeHTTP(w, req)
    
    assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireAuth_MissingCookie(t *testing.T) {
    mockService := &MockService{}
    middleware := RequireAuth(mockService)
    
    handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        t.Fatal("should not reach here")
    }))
    
    req := httptest.NewRequest("GET", "/api/protected", nil)
    w := httptest.NewRecorder()
    
    handler.ServeHTTP(w, req)
    
    assert.Equal(t, http.StatusUnauthorized, w.Code)
}
```

### 8.2 前端测试

**AuthProvider 测试**（`packages/core/src/auth/__tests__/auth-provider.test.tsx`）：

```tsx
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AuthProvider, useAuth } from '../'
import { authApi } from '@superteam/api-client'

jest.mock('@superteam/api-client')

describe('AuthProvider', () => {
  test('初始状态为 loading', () => {
    const queryClient = new QueryClient()
    const wrapper = ({ children }) => (
      <QueryClientProvider client={queryClient}>
        <AuthProvider>{children}</AuthProvider>
      </QueryClientProvider>
    )
    
    const { result } = renderHook(() => useAuth(), { wrapper })
    
    expect(result.current.isLoading).toBe(true)
    expect(result.current.isAuthenticated).toBe(false)
  })
  
  test('登录成功后 isAuthenticated 为 true', async () => {
    const mockUser = { id: 'user-123', username: 'admin', status: 'active' }
    ;(authApi.getCurrentUser as jest.Mock).mockResolvedValue({ user: mockUser })
    
    const queryClient = new QueryClient()
    const wrapper = ({ children }) => (
      <QueryClientProvider client={queryClient}>
        <AuthProvider>{children}</AuthProvider>
      </QueryClientProvider>
    )
    
    const { result } = renderHook(() => useAuth(), { wrapper })
    
    await waitFor(() => {
      expect(result.current.isLoading).toBe(false)
    })
    
    expect(result.current.isAuthenticated).toBe(true)
    expect(result.current.user).toEqual(mockUser)
  })
})
```

**LoginPage 测试**（`packages/views/src/auth/__tests__/login-page.test.tsx`）：

```tsx
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { LoginPage } from '../login-page'
import { useLogin } from '@superteam/core'

jest.mock('@superteam/core')
jest.mock('next/navigation', () => ({
  useRouter: () => ({ push: jest.fn() }),
}))

describe('LoginPage', () => {
  test('渲染登录表单', () => {
    ;(useLogin as jest.Mock).mockReturnValue({
      mutateAsync: jest.fn(),
      isPending: false,
    })
    
    render(<LoginPage />)
    
    expect(screen.getByLabelText('账号')).toBeInTheDocument()
    expect(screen.getByLabelText('密码')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /登录/ })).toBeInTheDocument()
  })
  
  test('提交表单调用 login mutation', async () => {
    const mockMutateAsync = jest.fn().mockResolvedValue({ id: 'user-123' })
    ;(useLogin as jest.Mock).mockReturnValue({
      mutateAsync: mockMutateAsync,
      isPending: false,
    })
    
    render(<LoginPage />)
    
    fireEvent.change(screen.getByLabelText('账号'), { target: { value: 'admin' } })
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'admin' } })
    fireEvent.click(screen.getByRole('button', { name: /登录/ }))
    
    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledWith({
        username: 'admin',
        password: 'admin',
      })
    })
  })
  
  test('登录失败显示错误信息', async () => {
    const mockMutateAsync = jest.fn().mockRejectedValue(new Error('用户名或密码错误'))
    ;(useLogin as jest.Mock).mockReturnValue({
      mutateAsync: mockMutateAsync,
      isPending: false,
    })
    
    render(<LoginPage />)
    
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'wrong' } })
    fireEvent.click(screen.getByRole('button', { name: /登录/ }))
    
    await waitFor(() => {
      expect(screen.getByText('用户名或密码错误')).toBeInTheDocument()
    })
  })
})
```

### 8.3 集成测试（Playwright）

**完整登录流程**（`apps/web/__tests__/e2e/login.spec.ts`）：

```typescript
import { test, expect } from '@playwright/test'

test('完整登录流程', async ({ page }) => {
  await page.goto('/login')
  
  // 验证登录页面元素
  await expect(page.locator('h1')).toContainText('账号登录')
  
  // 填写表单
  await page.fill('[name="username"]', 'admin')
  await page.fill('[name="password"]', 'admin')
  
  // 提交登录
  await page.click('button[type="submit"]')
  
  // 验证跳转到控制台
  await expect(page).toHaveURL('/overview')
})

test('登录失败显示错误', async ({ page }) => {
  await page.goto('/login')
  
  await page.fill('[name="username"]', 'admin')
  await page.fill('[name="password"]', 'wrong')
  await page.click('button[type="submit"]')
  
  // 验证错误提示
  await expect(page.locator('[role="alert"]')).toContainText('用户名或密码错误')
  
  // 验证仍在登录页
  await expect(page).toHaveURL('/login')
})

test('未登录访问受保护页面自动跳转', async ({ page }) => {
  await page.goto('/overview')
  
  // 验证跳转到登录页
  await expect(page).toHaveURL('/login')
})

test('登出后无法访问受保护页面', async ({ page }) => {
  // 先登录
  await page.goto('/login')
  await page.fill('[name="username"]', 'admin')
  await page.fill('[name="password"]', 'admin')
  await page.click('button[type="submit"]')
  await expect(page).toHaveURL('/overview')
  
  // 登出
  await page.click('[data-testid="user-menu"]')
  await page.click('[data-testid="logout-button"]')
  
  // 验证跳转到登录页
  await expect(page).toHaveURL('/login')
  
  // 尝试访问受保护页面
  await page.goto('/overview')
  await expect(page).toHaveURL('/login')
})
```

### 8.4 手动测试清单

**功能测试**：
- [ ] 使用正确的用户名密码登录成功
- [ ] 使用错误的密码登录失败，显示错误提示
- [ ] 使用不存在的用户名登录失败
- [ ] 登录后访问 `/api/auth/me` 返回用户信息
- [ ] 未登录访问受保护页面自动跳转登录页
- [ ] 登出后 Cookie 被清除，无法访问受保护页面
- [ ] 会话过期后自动跳转登录页（等待 12 小时或手动删除 Redis key）
- [ ] 刷新页面后登录状态保持

**UI 测试**：
- [ ] 登录页在桌面端显示正常（1920x1080）
- [ ] 登录页在移动端显示正常（375x667）
- [ ] 动画效果流畅（logo 呼吸、背景光晕）
- [ ] 输入框 focus 状态正常
- [ ] 登录按钮 loading 状态正常
- [ ] 错误提示样式正常
- [ ] 密码输入框不显示明文

**安全测试**：
- [ ] Cookie 包含 HttpOnly、Secure、SameSite 属性
- [ ] 浏览器开发者工具无法读取 session_token
- [ ] 登录失败不泄露用户是否存在
- [ ] Redis 存储的是 token hash，不是明文
- [ ] 密码字段不出现在日志中

**性能测试**：
- [ ] 登录响应时间 < 500ms
- [ ] 会话验证响应时间 < 100ms
- [ ] Redis 连接池正常工作
- [ ] PostgreSQL 查询使用索引


## 九、实施计划

### 9.1 开发阶段划分

**阶段 1：基础设施准备**（1-2 天）
- [ ] 创建 OpenAPI spec（`contracts/control-plane/auth.yaml`）
- [ ] 配置 oapi-codegen 生成 Go 和 TypeScript 类型
- [ ] 配置 PostgreSQL 连接池和 Redis 客户端
- [ ] 配置数据库迁移工具（goose 或 Atlas）
- [ ] 创建基础目录结构

**阶段 2：Go 后端实现**（3-4 天）
- [ ] 实现 Service 层（密码加密、会话管理）
- [ ] 实现 Repository 层（PostgreSQL + Redis）
- [ ] 实现 Handler 层（login/me/logout）
- [ ] 实现认证中间件
- [ ] 编写数据库迁移脚本
- [ ] 编写单元测试（Service + Handler + Middleware）
- [ ] 本地测试验证

**阶段 3：前端实现**（3-4 天）
- [ ] 实现 API Client（HTTP 封装 + 错误处理）
- [ ] 实现 AuthProvider（TanStack Query + Context）
- [ ] 实现 useAuth、useLogin、useLogout hooks
- [ ] 实现登录页面（参考 PulseAI 视觉风格）
- [ ] 实现 ProtectedRoute 组件
- [ ] 集成 Next.js App Router
- [ ] 编写单元测试（AuthProvider + LoginPage）
- [ ] 本地测试验证

**阶段 4：集成测试和优化**（2-3 天）
- [ ] 编写 E2E 测试（Playwright）
- [ ] 手动测试完整流程
- [ ] 性能测试和优化
- [ ] 安全检查（Cookie 配置、日志脱敏）
- [ ] 文档补充（API 文档、部署文档）
- [ ] Code Review

**总计**：9-13 天

### 9.2 技术依赖

**Go 依赖**：
```go
// go.mod
require (
    github.com/go-chi/chi/v5 v5.0.0
    github.com/jackc/pgx/v5 v5.5.0
    github.com/redis/go-redis/v9 v9.3.0
    golang.org/x/crypto v0.17.0
    github.com/google/uuid v1.5.0
    github.com/oapi-codegen/runtime v1.1.0
    github.com/stretchr/testify v1.8.4
)
```

**前端依赖**：
```json
{
  "dependencies": {
    "@tanstack/react-query": "^5.0.0",
    "motion": "^11.0.0",
    "next": "16.2.6",
    "react": "19.2.6",
    "lucide-react": "^0.468.0"
  },
  "devDependencies": {
    "@playwright/test": "^1.40.0",
    "vitest": "^1.0.0",
    "@testing-library/react": "^14.0.0"
  }
}
```

### 9.3 部署清单

**环境变量配置**：
```bash
# PostgreSQL
DATABASE_URL=postgres://user:pass@localhost:5432/superteam?sslmode=disable

# Redis
REDIS_URL=redis://localhost:6379/0
REDIS_PASSWORD=

# Server
PORT=8080
ENV=production

# Cookie
COOKIE_SECURE=true
COOKIE_DOMAIN=superteam.example.com

# Logging
LOG_LEVEL=info
```

**数据库初始化**：
```bash
# 运行迁移
goose -dir migrations postgres "$DATABASE_URL" up

# 验证默认用户
psql $DATABASE_URL -c "SELECT username, status FROM auth_users;"
```

**服务启动**：
```bash
# 启动 Control Plane
cd apps/control-plane
go run cmd/control-plane/main.go

# 启动 Web Console
cd apps/web
npm run dev
```

**健康检查**：
```bash
# 检查 API 可用性
curl http://localhost:8080/api/health

# 检查 Redis 连接
redis-cli ping

# 检查 PostgreSQL 连接
psql $DATABASE_URL -c "SELECT 1;"
```

### 9.4 风险和缓解措施

| 风险 | 影响 | 概率 | 缓解措施 |
|------|------|------|---------|
| bcrypt 性能问题 | 登录响应慢 | 低 | Cost factor 10 已平衡安全性和性能 |
| Redis 单点故障 | 所有用户登出 | 中 | 后续引入 Redis Sentinel 或 Cluster |
| Session 过期时间过短 | 用户频繁登录 | 低 | 12 小时已足够，可根据反馈调整 |
| Cookie 跨域问题 | 前后端分离部署失败 | 中 | 确保前后端同域或配置 CORS |
| 默认密码未修改 | 安全风险 | 高 | 部署文档强调修改，后续增加强制修改提示 |
| 数据库迁移失败 | 无法启动 | 低 | 提供回滚脚本，测试环境验证 |

### 9.5 后续扩展计划

**第二阶段功能**（优先级排序）：
1. **用户管理 CRUD**：创建/编辑/禁用用户
2. **密码修改**：用户自助修改密码
3. **会话管理**：查看活跃会话、强制登出
4. **操作审计日志**：记录登录、登出、敏感操作
5. **速率限制**：防止暴力破解
6. **双因素认证**：TOTP 或短信验证码
7. **SSO 集成**：OIDC/SAML 企业登录
8. **权限系统**：RBAC 或 ABAC
9. **租户隔离**：多租户数据隔离

## 十、总结

### 10.1 核心特性

本设计实现了 SuperTeam 控制平面的核心认证体系：

1. **Cookie-based 认证**：安全、简单、符合 Web 最佳实践
2. **双存储架构**：PostgreSQL 存用户，Redis 存会话，职责清晰
3. **企业级安全**：bcrypt 加密、HttpOnly Cookie、token hash 存储
4. **优雅的用户体验**：参考 PulseAI 的玻璃态设计，流畅动画
5. **完整的测试覆盖**：单元测试、集成测试、E2E 测试

### 10.2 技术亮点

- **OpenAPI 契约优先**：前后端类型安全，减少沟通成本
- **TanStack Query + Context**：状态管理清晰，性能优化自动
- **SHA-256 token hash**：即使 Redis 泄露也无法反推原始 token
- **12 小时会话 TTL**：平衡安全性和用户体验
- **SameSite=Lax Cookie**：防止 CSRF，允许顶级导航

### 10.3 设计原则

1. **安全优先**：所有安全措施都经过深思熟虑
2. **渐进增强**：第一阶段聚焦核心，后续按需扩展
3. **职责分离**：Service、Repository、Handler 层次清晰
4. **可测试性**：接口设计便于 mock，测试覆盖完整
5. **可维护性**：代码结构清晰，文档完善

### 10.4 关键决策回顾

| 决策点 | 选择 | 理由 |
|--------|------|------|
| 认证方式 | Cookie-based | 安全、简单、符合 Web 标准 |
| 密码加密 | bcrypt | Go 生态成熟，独立用户体系 |
| 会话存储 | Redis | 高性能、TTL 自动过期 |
| 前端状态 | TanStack Query + Context | 数据管理规范，便捷访问 |
| 视觉风格 | 完全复用 PulseAI | 快速实现，视觉冲击力强 |
| API 契约 | OpenAPI + oapi-codegen | 类型安全，自动生成代码 |
| 实施策略 | 分阶段实现 | 快速验证，降低风险 |

### 10.5 成功标准

**功能完整性**：
- ✅ 用户可以使用用户名密码登录
- ✅ 登录后可以访问受保护的控制台页面
- ✅ 未登录自动跳转登录页
- ✅ 用户可以主动登出
- ✅ 会话 12 小时自动过期

**安全性**：
- ✅ 密码使用 bcrypt 加密
- ✅ Session token 使用 SHA-256 hash 存储
- ✅ Cookie 配置 HttpOnly、Secure、SameSite
- ✅ 登录失败不泄露用户信息
- ✅ 日志不包含敏感信息

**用户体验**：
- ✅ 登录页面视觉效果流畅
- ✅ 登录响应时间 < 500ms
- ✅ 错误提示清晰友好
- ✅ 移动端适配良好

**代码质量**：
- ✅ 单元测试覆盖率 > 80%
- ✅ E2E 测试覆盖核心流程
- ✅ 代码通过 lint 检查
- ✅ API 文档完整

---

**设计完成日期**：2026-05-29  
**设计版本**：v1.0  
**下一步**：等待用户审核，审核通过后进入实施阶段

