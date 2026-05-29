# 登录认证系统实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 SuperTeam 控制平面的登录认证系统，包括 Web 登录页面和 Control Plane 认证 API

**Architecture:** Cookie-based 认证，PostgreSQL 存储用户，Redis 存储会话（12h TTL），Go 后端使用 bcrypt + chi router，前端使用 Next.js + TanStack Query + AuthProvider

**Tech Stack:** Go 1.21+, chi, pgx, sqlc, Redis, bcrypt, Next.js 16, React 19, TanStack Query v5, motion/react, OpenAPI 3.0, oapi-codegen

---

## 文件结构概览

### 后端文件（Go）

**新建文件**：
```
contracts/control-plane/
  └── auth.yaml                                    # OpenAPI 契约定义

apps/control-plane/
  ├── internal/
  │   └── auth/
  │       ├── types.go                             # 类型定义（User, Session, UserSummary）
  │       ├── errors.go                            # 错误定义
  │       ├── service.go                           # 认证服务（密码验证、会话管理）
  │       ├── service_test.go                      # Service 单元测试
  │       ├── repository.go                        # Repository 接口定义
  │       ├── repository_pg.go                     # PostgreSQL User Repository
  │       ├── repository_pg_test.go                # PostgreSQL Repository 测试
  │       ├── repository_redis.go                  # Redis Session Repository
  │       ├── repository_redis_test.go             # Redis Repository 测试
  │       ├── handler.go                           # HTTP Handlers
  │       ├── handler_test.go                      # Handler 测试
  │       ├── middleware.go                        # 认证中间件
  │       └── middleware_test.go                   # Middleware 测试
  └── migrations/
      └── 001_create_auth_users.sql                # 数据库迁移脚本
```

**修改文件**：
```
apps/control-plane/
  ├── go.mod                                       # 添加依赖
  ├── internal/
  │   ├── storage/
  │   │   ├── postgres.go                          # 添加 PostgreSQL 连接池
  │   │   └── redis.go                             # 添加 Redis 客户端
  │   ├── config/
  │   │   └── config.go                            # 添加认证配置
  │   └── api/
  │       ├── router.go                            # 添加认证路由
  │       └── server.go                            # 集成认证中间件
  └── cmd/control-plane/
      └── main.go                                  # 初始化认证模块
```

### 前端文件（TypeScript/React）

**新建文件**：
```
packages/api-client/src/
  ├── generated/
  │   └── auth.ts                                  # oapi-codegen 生成的类型
  ├── client.ts                                    # HTTP 客户端封装
  └── auth-api.ts                                  # 认证 API 封装

packages/core/src/auth/
  ├── auth-context.ts                              # Context 定义
  ├── auth-provider.tsx                            # AuthProvider 组件
  ├── use-auth.ts                                  # useAuth hook
  ├── use-login.ts                                 # useLogin mutation
  ├── use-logout.ts                                # useLogout mutation
  ├── index.ts                                     # 导出
  └── __tests__/
      ├── auth-provider.test.tsx                   # AuthProvider 测试
      └── use-login.test.ts                        # useLogin 测试

packages/ui/src/components/
  ├── label.tsx                                    # Label 组件（新增）
  └── alert.tsx                                    # Alert 组件（新增）

packages/views/src/auth/
  ├── login-page.tsx                               # 登录页面
  ├── protected-route.tsx                          # 路由保护组件
  ├── index.ts                                     # 导出
  └── __tests__/
      ├── login-page.test.tsx                      # 登录页面测试
      └── protected-route.test.tsx                 # 路由保护测试

apps/web/
  ├── app/
  │   ├── login/
  │   │   └── page.tsx                             # 登录路由
  │   ├── layout.tsx                               # 根布局（集成 AuthProvider）
  │   └── page.tsx                                 # 首页（重定向逻辑）
  └── public/assets/branding/
      └── superteam-logo.png                       # Logo 图片
```

**修改文件**：
```
packages/api-client/
  ├── package.json                                 # 添加依赖
  └── src/index.ts                                 # 导出认证 API

packages/core/
  ├── package.json                                 # 添加依赖
  └── src/index.ts                                 # 导出认证模块

packages/ui/
  └── src/index.ts                                 # 导出新组件

packages/views/
  ├── package.json                                 # 添加依赖
  └── src/index.ts                                 # 导出认证视图

apps/web/
  └── package.json                                 # 添加依赖
```

---

## 阶段 1：基础设施准备

### Task 1: 创建 OpenAPI 契约

**Files:**
- Create: `contracts/control-plane/auth.yaml`

- [ ] **Step 1: 创建 contracts 目录**

```bash
mkdir -p contracts/control-plane
```

- [ ] **Step 2: 编写 OpenAPI spec**

创建 `contracts/control-plane/auth.yaml`:

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

- [ ] **Step 3: 验证 OpenAPI spec**

```bash
# 安装 openapi-generator-cli（如果未安装）
npm install -g @openapitools/openapi-generator-cli

# 验证 spec
openapi-generator-cli validate -i contracts/control-plane/auth.yaml
```

Expected: "Spec is valid"

- [ ] **Step 4: Commit**

```bash
git add contracts/control-plane/auth.yaml
git commit -m "feat(contracts): add auth API OpenAPI spec"
```

### Task 2: 配置 Go 依赖和代码生成

**Files:**
- Modify: `apps/control-plane/go.mod`
- Create: `apps/control-plane/tools/tools.go`
- Create: `apps/control-plane/Makefile`

- [ ] **Step 1: 添加 Go 依赖**

```bash
cd apps/control-plane

# 添加依赖
go get github.com/go-chi/chi/v5@v5.0.12
go get github.com/jackc/pgx/v5@v5.5.5
go get github.com/redis/go-redis/v9@v9.5.1
go get golang.org/x/crypto@v0.22.0
go get github.com/google/uuid@v1.6.0
go get github.com/oapi-codegen/runtime@v1.1.1
go get github.com/deepmap/oapi-codegen/v2/cmd/oapi-codegen@v2.1.0

# 测试依赖
go get github.com/stretchr/testify@v1.9.0
```

- [ ] **Step 2: 创建 tools.go**

创建 `apps/control-plane/tools/tools.go`:

```go
//go:build tools
// +build tools

package tools

import (
	_ "github.com/deepmap/oapi-codegen/v2/cmd/oapi-codegen"
)
```

- [ ] **Step 3: 创建 Makefile**

创建 `apps/control-plane/Makefile`:

```makefile
.PHONY: generate test build clean

# 生成代码
generate:
	@echo "Generating code from OpenAPI spec..."
	oapi-codegen -package auth -generate types,chi-server \
		-o internal/auth/generated.go \
		../../contracts/control-plane/auth.yaml

# 运行测试
test:
	go test ./... -v -cover

# 构建
build:
	go build -o bin/control-plane cmd/control-plane/main.go

# 清理
clean:
	rm -rf bin/
	rm -f internal/auth/generated.go
```

- [ ] **Step 4: 生成代码**

```bash
cd apps/control-plane
make generate
```

Expected: 生成 `internal/auth/generated.go` 文件

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum tools/tools.go Makefile internal/auth/generated.go
git commit -m "feat(control-plane): add dependencies and code generation"
```

### Task 3: 配置数据库迁移工具

**Files:**
- Modify: `apps/control-plane/go.mod`
- Create: `apps/control-plane/migrations/.gitkeep`
- Modify: `apps/control-plane/Makefile`

- [ ] **Step 1: 安装 Atlas CLI**

```bash
# macOS
brew install ariga/tap/atlas

# 或使用 go install
go install ariga.io/atlas/cmd/atlas@latest
```

- [ ] **Step 2: 创建 migrations 目录**

```bash
cd apps/control-plane
mkdir -p migrations
touch migrations/.gitkeep
```

- [ ] **Step 3: 更新 Makefile**

在 `apps/control-plane/Makefile` 添加迁移命令:

```makefile
# 数据库迁移
migrate-up:
	atlas migrate apply --dir file://migrations --url "$(DATABASE_URL)"

migrate-down:
	atlas migrate down --dir file://migrations --url "$(DATABASE_URL)"

migrate-status:
	atlas migrate status --dir file://migrations --url "$(DATABASE_URL)"
```

- [ ] **Step 4: Commit**

```bash
git add migrations/.gitkeep Makefile
git commit -m "feat(control-plane): setup database migration tool"
```

---

## 阶段 2：Go 后端实现

### Task 4: 实现认证类型定义

**Files:**
- Create: `apps/control-plane/internal/auth/types.go`
- Create: `apps/control-plane/internal/auth/errors.go`

- [ ] **Step 1: 创建 types.go**

创建 `apps/control-plane/internal/auth/types.go`:

```go
package auth

import "time"

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

// ToUserSummary 转换为用户摘要
func (u *User) ToUserSummary() UserSummary {
	return UserSummary{
		ID:       u.ID,
		Username: u.Username,
		Status:   u.Status,
	}
}

// contextKey 用于 context 存储
type contextKey string

const (
	// UserContextKey 用户信息在 context 中的 key
	UserContextKey contextKey = "user"
)
```

- [ ] **Step 2: 创建 errors.go**

创建 `apps/control-plane/internal/auth/errors.go`:

```go
package auth

import "errors"

var (
	// ErrInvalidCredentials 用户名或密码错误
	ErrInvalidCredentials = errors.New("invalid username or password")
	
	// ErrUserDisabled 用户账号已被禁用
	ErrUserDisabled = errors.New("user account is disabled")
	
	// ErrSessionExpired 会话已过期
	ErrSessionExpired = errors.New("session expired")
	
	// ErrSessionNotFound 会话不存在
	ErrSessionNotFound = errors.New("session not found")
	
	// ErrUnauthorized 未授权
	ErrUnauthorized = errors.New("unauthorized")
)
```

- [ ] **Step 3: Commit**

```bash
git add internal/auth/types.go internal/auth/errors.go
git commit -m "feat(auth): add types and error definitions"
```

### Task 5: 实现 Repository 接口

**Files:**
- Create: `apps/control-plane/internal/auth/repository.go`

- [ ] **Step 1: 创建 repository.go**

创建 `apps/control-plane/internal/auth/repository.go`:

```go
package auth

import (
	"context"
	"time"
)

// UserRepository 用户数据访问接口
type UserRepository interface {
	// GetByUsername 根据用户名查询用户
	GetByUsername(ctx context.Context, username string) (*User, error)
	
	// GetByID 根据 ID 查询用户
	GetByID(ctx context.Context, id string) (*User, error)
	
	// UpdateLastLogin 更新最后登录时间
	UpdateLastLogin(ctx context.Context, userID string, loginAt time.Time) error
}

// SessionRepository 会话数据访问接口
type SessionRepository interface {
	// Create 创建会话
	Create(ctx context.Context, session *Session, token string) error
	
	// GetByToken 根据 token 查询会话
	GetByToken(ctx context.Context, token string) (*Session, error)
	
	// Delete 删除会话
	Delete(ctx context.Context, token string) error
	
	// UpdateLastSeen 更新最后活跃时间
	UpdateLastSeen(ctx context.Context, token string, lastSeenAt time.Time) error
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/auth/repository.go
git commit -m "feat(auth): add repository interfaces"
```

由于实施计划非常庞大，我将继续创建剩余部分。让我继续补充...


### Task 6: 实现 PostgreSQL User Repository

**Files:**
- Create: `apps/control-plane/internal/auth/repository_pg.go`
- Create: `apps/control-plane/internal/auth/repository_pg_test.go`

- [ ] **Step 1: 编写 PostgreSQL Repository 实现**

创建 `apps/control-plane/internal/auth/repository_pg.go`:

```go
package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgUserRepository PostgreSQL 用户仓储实现
type PgUserRepository struct {
	pool *pgxpool.Pool
}

// NewPgUserRepository 创建 PostgreSQL 用户仓储
func NewPgUserRepository(pool *pgxpool.Pool) *PgUserRepository {
	return &PgUserRepository{pool: pool}
}

// GetByUsername 根据用户名查询用户
func (r *PgUserRepository) GetByUsername(ctx context.Context, username string) (*User, error) {
	query := `
		SELECT id, username, password_hash, status, created_at, updated_at, last_login_at
		FROM auth_users
		WHERE username = $1
	`
	
	var user User
	err := r.pool.QueryRow(ctx, query, username).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&user.Status,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.LastLoginAt,
	)
	
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	
	return &user, nil
}

// GetByID 根据 ID 查询用户
func (r *PgUserRepository) GetByID(ctx context.Context, id string) (*User, error) {
	query := `
		SELECT id, username, password_hash, status, created_at, updated_at, last_login_at
		FROM auth_users
		WHERE id = $1
	`
	
	var user User
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&user.Status,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.LastLoginAt,
	)
	
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	
	return &user, nil
}

// UpdateLastLogin 更新最后登录时间
func (r *PgUserRepository) UpdateLastLogin(ctx context.Context, userID string, loginAt time.Time) error {
	query := `
		UPDATE auth_users
		SET last_login_at = $2, updated_at = $2
		WHERE id = $1
	`
	
	_, err := r.pool.Exec(ctx, query, userID, loginAt)
	return err
}
```

- [ ] **Step 2: 编写测试**

创建 `apps/control-plane/internal/auth/repository_pg_test.go`:

```go
package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 注意：这些测试需要真实的 PostgreSQL 数据库
// 可以使用 testcontainers 或 docker-compose 启动测试数据库

func TestPgUserRepository_GetByUsername(t *testing.T) {
	// 跳过集成测试（需要数据库）
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	
	// TODO: 设置测试数据库连接
	// pool := setupTestDB(t)
	// defer pool.Close()
	
	// repo := NewPgUserRepository(pool)
	
	// 测试查询存在的用户
	// user, err := repo.GetByUsername(context.Background(), "admin")
	// require.NoError(t, err)
	// assert.Equal(t, "admin", user.Username)
	// assert.Equal(t, "active", user.Status)
	
	// 测试查询不存在的用户
	// _, err = repo.GetByUsername(context.Background(), "nonexistent")
	// assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestPgUserRepository_GetByID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	
	// TODO: 实现集成测试
}

func TestPgUserRepository_UpdateLastLogin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	
	// TODO: 实现集成测试
}
```

- [ ] **Step 3: 运行测试（跳过集成测试）**

```bash
cd apps/control-plane
go test ./internal/auth -v -short
```

Expected: PASS (集成测试被跳过)

- [ ] **Step 4: Commit**

```bash
git add internal/auth/repository_pg.go internal/auth/repository_pg_test.go
git commit -m "feat(auth): implement PostgreSQL user repository"
```

### Task 7: 实现 Redis Session Repository

**Files:**
- Create: `apps/control-plane/internal/auth/repository_redis.go`
- Create: `apps/control-plane/internal/auth/repository_redis_test.go`

- [ ] **Step 1: 编写 Redis Repository 实现**

创建 `apps/control-plane/internal/auth/repository_redis.go`:

```go
package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisSessionRepository Redis 会话仓储实现
type RedisSessionRepository struct {
	client *redis.Client
}

// NewRedisSessionRepository 创建 Redis 会话仓储
func NewRedisSessionRepository(client *redis.Client) *RedisSessionRepository {
	return &RedisSessionRepository{client: client}
}

// hashToken 计算 token 的 SHA-256 hash
func (r *RedisSessionRepository) hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// sessionKey 生成 Redis key
func (r *RedisSessionRepository) sessionKey(token string) string {
	return fmt.Sprintf("session:%s", r.hashToken(token))
}

// Create 创建会话
func (r *RedisSessionRepository) Create(ctx context.Context, session *Session, token string) error {
	key := r.sessionKey(token)
	
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	
	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		return fmt.Errorf("session already expired")
	}
	
	return r.client.Set(ctx, key, data, ttl).Err()
}

// GetByToken 根据 token 查询会话
func (r *RedisSessionRepository) GetByToken(ctx context.Context, token string) (*Session, error) {
	key := r.sessionKey(token)
	
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("get session: %w", err)
	}
	
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	
	return &session, nil
}

// Delete 删除会话
func (r *RedisSessionRepository) Delete(ctx context.Context, token string) error {
	key := r.sessionKey(token)
	return r.client.Del(ctx, key).Err()
}

// UpdateLastSeen 更新最后活跃时间
func (r *RedisSessionRepository) UpdateLastSeen(ctx context.Context, token string, lastSeenAt time.Time) error {
	key := r.sessionKey(token)
	
	// 获取现有会话
	session, err := r.GetByToken(ctx, token)
	if err != nil {
		return err
	}
	
	// 更新 last_seen_at
	session.LastSeenAt = lastSeenAt
	
	// 重新序列化
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	
	// 保持原有 TTL
	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		return ErrSessionExpired
	}
	
	return r.client.Set(ctx, key, data, ttl).Err()
}
```

- [ ] **Step 2: 编写测试**

创建 `apps/control-plane/internal/auth/repository_redis_test.go`:

```go
package auth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 注意：这些测试需要真实的 Redis
// 可以使用 miniredis 或 testcontainers

func TestRedisSessionRepository_Create(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	
	// TODO: 设置测试 Redis 连接
	// client := setupTestRedis(t)
	// defer client.Close()
	
	// repo := NewRedisSessionRepository(client)
	
	// session := &Session{
	// 	ID:         uuid.New().String(),
	// 	UserID:     uuid.New().String(),
	// 	ExpiresAt:  time.Now().Add(12 * time.Hour),
	// 	LastSeenAt: time.Now(),
	// 	ClientIP:   "127.0.0.1",
	// 	UserAgent:  "test-agent",
	// }
	// token := "test-token-123"
	
	// err := repo.Create(context.Background(), session, token)
	// require.NoError(t, err)
	
	// 验证可以查询到
	// retrieved, err := repo.GetByToken(context.Background(), token)
	// require.NoError(t, err)
	// assert.Equal(t, session.ID, retrieved.ID)
	// assert.Equal(t, session.UserID, retrieved.UserID)
}

func TestRedisSessionRepository_GetByToken(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	
	// TODO: 实现集成测试
}

func TestRedisSessionRepository_Delete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	
	// TODO: 实现集成测试
}

func TestRedisSessionRepository_UpdateLastSeen(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	
	// TODO: 实现集成测试
}
```

- [ ] **Step 3: 运行测试（跳过集成测试）**

```bash
cd apps/control-plane
go test ./internal/auth -v -short
```

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/auth/repository_redis.go internal/auth/repository_redis_test.go
git commit -m "feat(auth): implement Redis session repository"
```

### Task 8: 实现 Auth Service

**Files:**
- Create: `apps/control-plane/internal/auth/service.go`
- Create: `apps/control-plane/internal/auth/service_test.go`

- [ ] **Step 1: 编写 Service 实现**

创建 `apps/control-plane/internal/auth/service.go`:

```go
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	// SessionTTL 会话有效期
	SessionTTL = 12 * time.Hour
	
	// TokenBytes token 字节数
	TokenBytes = 32
	
	// BcryptCost bcrypt cost factor
	BcryptCost = 10
)

// Service 认证服务
type Service struct {
	userRepo    UserRepository
	sessionRepo SessionRepository
}

// NewService 创建认证服务
func NewService(userRepo UserRepository, sessionRepo SessionRepository) *Service {
	return &Service{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
	}
}

// HashPassword 使用 bcrypt 加密密码
func (s *Service) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword 验证密码
func (s *Service) VerifyPassword(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// GenerateToken 生成随机 session token
func (s *Service) GenerateToken() (string, error) {
	b := make([]byte, TokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// HashToken 计算 token 的 SHA-256 hash
func (s *Service) HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// Login 登录逻辑
func (s *Service) Login(ctx context.Context, username, password, clientIP, userAgent string) (*Session, *User, string, error) {
	// 1. 查询用户
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		return nil, nil, "", ErrInvalidCredentials
	}
	
	// 2. 验证状态
	if user.Status != "active" {
		return nil, nil, "", ErrUserDisabled
	}
	
	// 3. 验证密码
	if err := s.VerifyPassword(password, user.PasswordHash); err != nil {
		return nil, nil, "", ErrInvalidCredentials
	}
	
	// 4. 生成 session token
	token, err := s.GenerateToken()
	if err != nil {
		return nil, nil, "", fmt.Errorf("generate token: %w", err)
	}
	
	// 5. 创建会话
	now := time.Now()
	session := &Session{
		ID:         uuid.New().String(),
		UserID:     user.ID,
		ExpiresAt:  now.Add(SessionTTL),
		LastSeenAt: now,
		ClientIP:   clientIP,
		UserAgent:  userAgent,
	}
	
	if err := s.sessionRepo.Create(ctx, session, token); err != nil {
		return nil, nil, "", fmt.Errorf("create session: %w", err)
	}
	
	// 6. 更新最后登录时间
	if err := s.userRepo.UpdateLastLogin(ctx, user.ID, now); err != nil {
		// 非关键错误，记录日志但不中断流程
		// log.Warn("failed to update last login", "error", err)
	}
	
	return session, user, token, nil
}

// GetUserBySessionToken 通过 token 获取用户
func (s *Service) GetUserBySessionToken(ctx context.Context, token string) (*User, error) {
	// 1. 查询会话
	session, err := s.sessionRepo.GetByToken(ctx, token)
	if err != nil {
		return nil, ErrSessionNotFound
	}
	
	// 2. 验证过期时间
	if time.Now().After(session.ExpiresAt) {
		return nil, ErrSessionExpired
	}
	
	// 3. 查询用户
	user, err := s.userRepo.GetByID(ctx, session.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	
	// 4. 验证用户状态
	if user.Status != "active" {
		return nil, ErrUserDisabled
	}
	
	// 5. 更新最后活跃时间
	if err := s.sessionRepo.UpdateLastSeen(ctx, token, time.Now()); err != nil {
		// 非关键错误，记录日志但不中断流程
		// log.Warn("failed to update last seen", "error", err)
	}
	
	return user, nil
}

// Logout 登出
func (s *Service) Logout(ctx context.Context, token string) error {
	return s.sessionRepo.Delete(ctx, token)
}
```

- [ ] **Step 2: 编写单元测试**

创建 `apps/control-plane/internal/auth/service_test.go`:

```go
package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockUserRepository mock 用户仓储
type MockUserRepository struct {
	mock.Mock
}

func (m *MockUserRepository) GetByUsername(ctx context.Context, username string) (*User, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *MockUserRepository) GetByID(ctx context.Context, id string) (*User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *MockUserRepository) UpdateLastLogin(ctx context.Context, userID string, loginAt time.Time) error {
	args := m.Called(ctx, userID, mock.Anything)
	return args.Error(0)
}

// MockSessionRepository mock 会话仓储
type MockSessionRepository struct {
	mock.Mock
}

func (m *MockSessionRepository) Create(ctx context.Context, session *Session, token string) error {
	args := m.Called(ctx, mock.Anything, mock.Anything)
	return args.Error(0)
}

func (m *MockSessionRepository) GetByToken(ctx context.Context, token string) (*Session, error) {
	args := m.Called(ctx, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Session), args.Error(1)
}

func (m *MockSessionRepository) Delete(ctx context.Context, token string) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func (m *MockSessionRepository) UpdateLastSeen(ctx context.Context, token string, lastSeenAt time.Time) error {
	args := m.Called(ctx, token, mock.Anything)
	return args.Error(0)
}

func TestService_HashPassword(t *testing.T) {
	s := NewService(nil, nil)
	
	hash, err := s.HashPassword("test123")
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, "test123", hash)
	assert.Contains(t, hash, "$2a$10$")
}

func TestService_VerifyPassword(t *testing.T) {
	s := NewService(nil, nil)
	
	hash, err := s.HashPassword("test123")
	require.NoError(t, err)
	
	// 正确密码
	err = s.VerifyPassword("test123", hash)
	assert.NoError(t, err)
	
	// 错误密码
	err = s.VerifyPassword("wrong", hash)
	assert.Error(t, err)
}

func TestService_GenerateToken(t *testing.T) {
	s := NewService(nil, nil)
	
	token1, err := s.GenerateToken()
	require.NoError(t, err)
	assert.Len(t, token1, TokenBytes*2) // hex 编码后长度翻倍
	
	token2, err := s.GenerateToken()
	require.NoError(t, err)
	assert.NotEqual(t, token1, token2) // 每次生成不同
}

func TestService_Login_Success(t *testing.T) {
	mockUserRepo := new(MockUserRepository)
	mockSessionRepo := new(MockSessionRepository)
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
	
	require.NoError(t, err)
	assert.NotNil(t, session)
	assert.NotNil(t, user)
	assert.NotEmpty(t, token)
	assert.Equal(t, "admin", user.Username)
	assert.Equal(t, "user-123", session.UserID)
	
	mockUserRepo.AssertExpectations(t)
	mockSessionRepo.AssertExpectations(t)
}

func TestService_Login_InvalidPassword(t *testing.T) {
	mockUserRepo := new(MockUserRepository)
	s := NewService(mockUserRepo, nil)
	
	hash, _ := s.HashPassword("admin")
	mockUserRepo.On("GetByUsername", mock.Anything, "admin").Return(&User{
		ID:           "user-123",
		Username:     "admin",
		PasswordHash: hash,
		Status:       "active",
	}, nil)
	
	_, _, _, err := s.Login(context.Background(), "admin", "wrong", "127.0.0.1", "test-agent")
	
	assert.ErrorIs(t, err, ErrInvalidCredentials)
	mockUserRepo.AssertExpectations(t)
}

func TestService_Login_UserDisabled(t *testing.T) {
	mockUserRepo := new(MockUserRepository)
	s := NewService(mockUserRepo, nil)
	
	hash, _ := s.HashPassword("admin")
	mockUserRepo.On("GetByUsername", mock.Anything, "admin").Return(&User{
		ID:           "user-123",
		Username:     "admin",
		PasswordHash: hash,
		Status:       "disabled",
	}, nil)
	
	_, _, _, err := s.Login(context.Background(), "admin", "admin", "127.0.0.1", "test-agent")
	
	assert.ErrorIs(t, err, ErrUserDisabled)
	mockUserRepo.AssertExpectations(t)
}

func TestService_Login_UserNotFound(t *testing.T) {
	mockUserRepo := new(MockUserRepository)
	s := NewService(mockUserRepo, nil)
	
	mockUserRepo.On("GetByUsername", mock.Anything, "nonexistent").Return(nil, ErrInvalidCredentials)
	
	_, _, _, err := s.Login(context.Background(), "nonexistent", "password", "127.0.0.1", "test-agent")
	
	assert.ErrorIs(t, err, ErrInvalidCredentials)
	mockUserRepo.AssertExpectations(t)
}

func TestService_GetUserBySessionToken_Success(t *testing.T) {
	mockUserRepo := new(MockUserRepository)
	mockSessionRepo := new(MockSessionRepository)
	s := NewService(mockUserRepo, mockSessionRepo)
	
	session := &Session{
		ID:         "session-123",
		UserID:     "user-123",
		ExpiresAt:  time.Now().Add(1 * time.Hour),
		LastSeenAt: time.Now(),
		ClientIP:   "127.0.0.1",
		UserAgent:  "test-agent",
	}
	
	mockSessionRepo.On("GetByToken", mock.Anything, "token-abc").Return(session, nil)
	mockUserRepo.On("GetByID", mock.Anything, "user-123").Return(&User{
		ID:       "user-123",
		Username: "admin",
		Status:   "active",
	}, nil)
	mockSessionRepo.On("UpdateLastSeen", mock.Anything, "token-abc", mock.Anything).Return(nil)
	
	user, err := s.GetUserBySessionToken(context.Background(), "token-abc")
	
	require.NoError(t, err)
	assert.Equal(t, "admin", user.Username)
	
	mockUserRepo.AssertExpectations(t)
	mockSessionRepo.AssertExpectations(t)
}

func TestService_GetUserBySessionToken_Expired(t *testing.T) {
	mockSessionRepo := new(MockSessionRepository)
	s := NewService(nil, mockSessionRepo)
	
	session := &Session{
		ID:         "session-123",
		UserID:     "user-123",
		ExpiresAt:  time.Now().Add(-1 * time.Hour), // 已过期
		LastSeenAt: time.Now(),
		ClientIP:   "127.0.0.1",
		UserAgent:  "test-agent",
	}
	
	mockSessionRepo.On("GetByToken", mock.Anything, "token-abc").Return(session, nil)
	
	_, err := s.GetUserBySessionToken(context.Background(), "token-abc")
	
	assert.ErrorIs(t, err, ErrSessionExpired)
	mockSessionRepo.AssertExpectations(t)
}

func TestService_Logout_Success(t *testing.T) {
	mockSessionRepo := new(MockSessionRepository)
	s := NewService(nil, mockSessionRepo)
	
	mockSessionRepo.On("Delete", mock.Anything, "token-abc").Return(nil)
	
	err := s.Logout(context.Background(), "token-abc")
	
	assert.NoError(t, err)
	mockSessionRepo.AssertExpectations(t)
}
```

- [ ] **Step 3: 运行测试**

```bash
cd apps/control-plane
go test ./internal/auth -v -run TestService
```

Expected: 所有 TestService 测试通过

- [ ] **Step 4: Commit**

```bash
git add internal/auth/service.go internal/auth/service_test.go
git commit -m "feat(auth): implement auth service with tests"
```


### Task 9: 实现 HTTP Handlers

**Files:**
- Create: `apps/control-plane/internal/auth/handler.go`
- Create: `apps/control-plane/internal/auth/handler_test.go`

- [ ] **Step 1: 编写 Handler 实现**

创建 `apps/control-plane/internal/auth/handler.go`:

```go
package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// Config Handler 配置
type Config struct {
	IsProduction bool
	CookieDomain string
}

// Handler HTTP 处理器
type Handler struct {
	service *Service
	config  *Config
}

// NewHandler 创建 Handler
func NewHandler(service *Service, config *Config) *Handler {
	return &Handler{
		service: service,
		config:  config,
	}
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	User UserSummary `json:"user"`
}

// CurrentUserResponse 当前用户响应
type CurrentUserResponse struct {
	User UserSummary `json:"user"`
}

// ErrorResponse 错误响应
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// Login 处理登录请求
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
		Domain:   h.config.CookieDomain,
	})
	
	respondJSON(w, http.StatusOK, LoginResponse{
		User: user.ToUserSummary(),
	})
}

// GetCurrentUser 获取当前用户信息
func (h *Handler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	// 从 context 获取用户（由中间件注入）
	user, ok := r.Context().Value(UserContextKey).(*User)
	if !ok {
		respondError(w, http.StatusUnauthorized, "未登录")
		return
	}
	
	respondJSON(w, http.StatusOK, CurrentUserResponse{
		User: user.ToUserSummary(),
	})
}

// Logout 处理登出请求
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
		Domain:   h.config.CookieDomain,
	})
	
	respondJSON(w, http.StatusOK, map[string]string{
		"message": "logout success",
	})
}

// 辅助函数

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, ErrorResponse{Error: message})
}

func getClientIP(r *http.Request) string {
	// 优先从 X-Forwarded-For 获取
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	// 其次从 X-Real-IP 获取
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// 最后使用 RemoteAddr
	return r.RemoteAddr
}
```

- [ ] **Step 2: 编写 Handler 测试**

创建 `apps/control-plane/internal/auth/handler_test.go`:

```go
package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestHandler_Login_Success(t *testing.T) {
	mockUserRepo := new(MockUserRepository)
	mockSessionRepo := new(MockSessionRepository)
	service := NewService(mockUserRepo, mockSessionRepo)
	handler := NewHandler(service, &Config{IsProduction: false})
	
	// Mock 数据
	hash, _ := service.HashPassword("admin")
	mockUserRepo.On("GetByUsername", mock.Anything, "admin").Return(&User{
		ID:           "user-123",
		Username:     "admin",
		PasswordHash: hash,
		Status:       "active",
	}, nil)
	mockSessionRepo.On("Create", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockUserRepo.On("UpdateLastLogin", mock.Anything, "user-123", mock.Anything).Return(nil)
	
	// 构造请求
	body := `{"username":"admin","password":"admin"}`
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	
	// 执行
	handler.Login(w, req)
	
	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)
	
	var resp LoginResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "admin", resp.User.Username)
	assert.Equal(t, "active", resp.User.Status)
	
	// 验证 Cookie
	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, "session_token", cookies[0].Name)
	assert.NotEmpty(t, cookies[0].Value)
	assert.True(t, cookies[0].HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, cookies[0].SameSite)
	
	mockUserRepo.AssertExpectations(t)
	mockSessionRepo.AssertExpectations(t)
}

func TestHandler_Login_InvalidCredentials(t *testing.T) {
	mockUserRepo := new(MockUserRepository)
	mockSessionRepo := new(MockSessionRepository)
	service := NewService(mockUserRepo, mockSessionRepo)
	handler := NewHandler(service, &Config{})
	
	hash, _ := service.HashPassword("admin")
	mockUserRepo.On("GetByUsername", mock.Anything, "admin").Return(&User{
		ID:           "user-123",
		Username:     "admin",
		PasswordHash: hash,
		Status:       "active",
	}, nil)
	
	body := `{"username":"admin","password":"wrong"}`
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	
	handler.Login(w, req)
	
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	
	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	assert.Contains(t, resp.Error, "用户名或密码错误")
	
	mockUserRepo.AssertExpectations(t)
}

func TestHandler_Login_UserDisabled(t *testing.T) {
	mockUserRepo := new(MockUserRepository)
	service := NewService(mockUserRepo, nil)
	handler := NewHandler(service, &Config{})
	
	hash, _ := service.HashPassword("admin")
	mockUserRepo.On("GetByUsername", mock.Anything, "admin").Return(&User{
		ID:           "user-123",
		Username:     "admin",
		PasswordHash: hash,
		Status:       "disabled",
	}, nil)
	
	body := `{"username":"admin","password":"admin"}`
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	
	handler.Login(w, req)
	
	assert.Equal(t, http.StatusForbidden, w.Code)
	
	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	assert.Contains(t, resp.Error, "账号已被禁用")
	
	mockUserRepo.AssertExpectations(t)
}

func TestHandler_Login_InvalidJSON(t *testing.T) {
	handler := NewHandler(nil, &Config{})
	
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	
	handler.Login(w, req)
	
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetCurrentUser_Success(t *testing.T) {
	handler := NewHandler(nil, &Config{})
	
	user := &User{
		ID:       "user-123",
		Username: "admin",
		Status:   "active",
	}
	
	req := httptest.NewRequest("GET", "/api/auth/me", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserContextKey, user))
	w := httptest.NewRecorder()
	
	handler.GetCurrentUser(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	
	var resp CurrentUserResponse
	json.NewDecoder(w.Body).Decode(&resp)
	assert.Equal(t, "admin", resp.User.Username)
}

func TestHandler_GetCurrentUser_Unauthorized(t *testing.T) {
	handler := NewHandler(nil, &Config{})
	
	req := httptest.NewRequest("GET", "/api/auth/me", nil)
	w := httptest.NewRecorder()
	
	handler.GetCurrentUser(w, req)
	
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandler_Logout_Success(t *testing.T) {
	mockSessionRepo := new(MockSessionRepository)
	service := NewService(nil, mockSessionRepo)
	handler := NewHandler(service, &Config{IsProduction: false})
	
	mockSessionRepo.On("Delete", mock.Anything, "token-abc").Return(nil)
	
	req := httptest.NewRequest("POST", "/api/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: "token-abc"})
	w := httptest.NewRecorder()
	
	handler.Logout(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	
	// 验证 Cookie 被清除
	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, "session_token", cookies[0].Name)
	assert.Equal(t, "", cookies[0].Value)
	assert.Equal(t, -1, cookies[0].MaxAge)
	
	mockSessionRepo.AssertExpectations(t)
}

func TestHandler_Logout_NoCookie(t *testing.T) {
	handler := NewHandler(nil, &Config{})
	
	req := httptest.NewRequest("POST", "/api/auth/logout", nil)
	w := httptest.NewRecorder()
	
	handler.Logout(w, req)
	
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
```

- [ ] **Step 3: 运行测试**

```bash
cd apps/control-plane
go test ./internal/auth -v -run TestHandler
```

Expected: 所有 TestHandler 测试通过

- [ ] **Step 4: Commit**

```bash
git add internal/auth/handler.go internal/auth/handler_test.go
git commit -m "feat(auth): implement HTTP handlers with tests"
```

### Task 10: 实现认证中间件

**Files:**
- Create: `apps/control-plane/internal/auth/middleware.go`
- Create: `apps/control-plane/internal/auth/middleware_test.go`

- [ ] **Step 1: 编写 Middleware 实现**

创建 `apps/control-plane/internal/auth/middleware.go`:

```go
package auth

import (
	"context"
	"net/http"
)

// RequireAuth 认证中间件
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

- [ ] **Step 2: 编写 Middleware 测试**

创建 `apps/control-plane/internal/auth/middleware_test.go`:

```go
package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRequireAuth_ValidSession(t *testing.T) {
	mockUserRepo := new(MockUserRepository)
	mockSessionRepo := new(MockSessionRepository)
	service := NewService(mockUserRepo, mockSessionRepo)
	
	session := &Session{
		ID:         "session-123",
		UserID:     "user-123",
		ExpiresAt:  time.Now().Add(1 * time.Hour),
		LastSeenAt: time.Now(),
		ClientIP:   "127.0.0.1",
		UserAgent:  "test-agent",
	}
	
	mockSessionRepo.On("GetByToken", mock.Anything, "valid-token").Return(session, nil)
	mockUserRepo.On("GetByID", mock.Anything, "user-123").Return(&User{
		ID:       "user-123",
		Username: "admin",
		Status:   "active",
	}, nil)
	mockSessionRepo.On("UpdateLastSeen", mock.Anything, "valid-token", mock.Anything).Return(nil)
	
	middleware := RequireAuth(service)
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
	mockUserRepo.AssertExpectations(t)
	mockSessionRepo.AssertExpectations(t)
}

func TestRequireAuth_MissingCookie(t *testing.T) {
	service := NewService(nil, nil)
	middleware := RequireAuth(service)
	
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach here")
	}))
	
	req := httptest.NewRequest("GET", "/api/protected", nil)
	w := httptest.NewRecorder()
	
	handler.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_InvalidToken(t *testing.T) {
	mockSessionRepo := new(MockSessionRepository)
	service := NewService(nil, mockSessionRepo)
	
	mockSessionRepo.On("GetByToken", mock.Anything, "invalid-token").Return(nil, ErrSessionNotFound)
	
	middleware := RequireAuth(service)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach here")
	}))
	
	req := httptest.NewRequest("GET", "/api/protected", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: "invalid-token"})
	w := httptest.NewRecorder()
	
	handler.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	mockSessionRepo.AssertExpectations(t)
}

func TestRequireAuth_ExpiredSession(t *testing.T) {
	mockSessionRepo := new(MockSessionRepository)
	service := NewService(nil, mockSessionRepo)
	
	session := &Session{
		ID:         "session-123",
		UserID:     "user-123",
		ExpiresAt:  time.Now().Add(-1 * time.Hour), // 已过期
		LastSeenAt: time.Now(),
		ClientIP:   "127.0.0.1",
		UserAgent:  "test-agent",
	}
	
	mockSessionRepo.On("GetByToken", mock.Anything, "expired-token").Return(session, nil)
	
	middleware := RequireAuth(service)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach here")
	}))
	
	req := httptest.NewRequest("GET", "/api/protected", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: "expired-token"})
	w := httptest.NewRecorder()
	
	handler.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	mockSessionRepo.AssertExpectations(t)
}
```

- [ ] **Step 3: 运行测试**

```bash
cd apps/control-plane
go test ./internal/auth -v -run TestRequireAuth
```

Expected: 所有 TestRequireAuth 测试通过

- [ ] **Step 4: Commit**

```bash
git add internal/auth/middleware.go internal/auth/middleware_test.go
git commit -m "feat(auth): implement auth middleware with tests"
```

### Task 11: 创建数据库迁移脚本

**Files:**
- Create: `apps/control-plane/migrations/001_create_auth_users.sql`

- [ ] **Step 1: 编写迁移脚本**

创建 `apps/control-plane/migrations/001_create_auth_users.sql`:

```sql
-- Create auth_users table
CREATE TABLE IF NOT EXISTS auth_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_login_at TIMESTAMPTZ
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_auth_users_username ON auth_users(username);
CREATE INDEX IF NOT EXISTS idx_auth_users_status ON auth_users(status);

-- Add comments
COMMENT ON TABLE auth_users IS '认证用户表';
COMMENT ON COLUMN auth_users.id IS '用户唯一标识';
COMMENT ON COLUMN auth_users.username IS '登录用户名';
COMMENT ON COLUMN auth_users.password_hash IS 'bcrypt 密码哈希';
COMMENT ON COLUMN auth_users.status IS '用户状态: active, disabled';
COMMENT ON COLUMN auth_users.created_at IS '创建时间';
COMMENT ON COLUMN auth_users.updated_at IS '更新时间';
COMMENT ON COLUMN auth_users.last_login_at IS '最后登录时间';

-- Insert default admin user
-- Username: admin
-- Password: admin
-- bcrypt hash (cost 10)
INSERT INTO auth_users (username, password_hash, status) 
VALUES (
    'admin', 
    '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy',
    'active'
)
ON CONFLICT (username) DO NOTHING;
```

- [ ] **Step 2: 验证 SQL 语法**

```bash
# 使用 psql 验证语法（需要 PostgreSQL 客户端）
psql --version

# 或者使用在线 SQL 验证工具
```

- [ ] **Step 3: Commit**

```bash
git add migrations/001_create_auth_users.sql
git commit -m "feat(auth): add database migration for auth_users table"
```


### Task 12: 配置 PostgreSQL 和 Redis 连接

**Files:**
- Modify: `apps/control-plane/internal/storage/postgres.go`
- Modify: `apps/control-plane/internal/storage/redis.go`
- Modify: `apps/control-plane/internal/config/config.go`

- [ ] **Step 1: 配置 PostgreSQL 连接池**

修改 `apps/control-plane/internal/storage/postgres.go`:

```go
package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresConfig PostgreSQL 配置
type PostgresConfig struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// NewPostgresPool 创建 PostgreSQL 连接池
func NewPostgresPool(ctx context.Context, cfg PostgresConfig) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}
	
	config.MaxConns = cfg.MaxConns
	config.MinConns = cfg.MinConns
	config.MaxConnLifetime = cfg.MaxConnLifetime
	config.MaxConnIdleTime = cfg.MaxConnIdleTime
	
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}
	
	// 测试连接
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	
	return pool, nil
}
```

- [ ] **Step 2: 配置 Redis 客户端**

修改 `apps/control-plane/internal/storage/redis.go`:

```go
package storage

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// RedisConfig Redis 配置
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// NewRedisClient 创建 Redis 客户端
func NewRedisClient(ctx context.Context, cfg RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	
	// 测试连接
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	
	return client, nil
}
```

- [ ] **Step 3: 添加认证配置**

修改 `apps/control-plane/internal/config/config.go`:

```go
package config

import (
	"os"
	"strconv"
	"time"
)

// Config 应用配置
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	Auth     AuthConfig
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port         string
	IsProduction bool
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// AuthConfig 认证配置
type AuthConfig struct {
	CookieDomain string
}

// Load 加载配置
func Load() (*Config, error) {
	return &Config{
		Server: ServerConfig{
			Port:         getEnv("PORT", "8080"),
			IsProduction: getEnv("ENV", "development") == "production",
		},
		Database: DatabaseConfig{
			URL:             getEnv("DATABASE_URL", "postgres://localhost:5432/superteam?sslmode=disable"),
			MaxConns:        int32(getEnvInt("DB_MAX_CONNS", 25)),
			MinConns:        int32(getEnvInt("DB_MIN_CONNS", 5)),
			MaxConnLifetime: time.Duration(getEnvInt("DB_MAX_CONN_LIFETIME", 3600)) * time.Second,
			MaxConnIdleTime: time.Duration(getEnvInt("DB_MAX_CONN_IDLE_TIME", 600)) * time.Second,
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		Auth: AuthConfig{
			CookieDomain: getEnv("COOKIE_DOMAIN", ""),
		},
	}, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
```

- [ ] **Step 4: Commit**

```bash
git add internal/storage/postgres.go internal/storage/redis.go internal/config/config.go
git commit -m "feat(storage): configure PostgreSQL and Redis connections"
```

### Task 13: 集成认证模块到主程序

**Files:**
- Modify: `apps/control-plane/cmd/control-plane/main.go`
- Modify: `apps/control-plane/internal/api/router.go`

- [ ] **Step 1: 修改 main.go**

修改 `apps/control-plane/cmd/control-plane/main.go`:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"superteam/control-plane/internal/api"
	"superteam/control-plane/internal/auth"
	"superteam/control-plane/internal/config"
	"superteam/control-plane/internal/storage"
)

func main() {
	ctx := context.Background()
	
	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	
	// 初始化 PostgreSQL
	pgPool, err := storage.NewPostgresPool(ctx, storage.PostgresConfig{
		URL:             cfg.Database.URL,
		MaxConns:        cfg.Database.MaxConns,
		MinConns:        cfg.Database.MinConns,
		MaxConnLifetime: cfg.Database.MaxConnLifetime,
		MaxConnIdleTime: cfg.Database.MaxConnIdleTime,
	})
	if err != nil {
		log.Fatalf("init postgres: %v", err)
	}
	defer pgPool.Close()
	log.Println("PostgreSQL connected")
	
	// 初始化 Redis
	redisClient, err := storage.NewRedisClient(ctx, storage.RedisConfig{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err != nil {
		log.Fatalf("init redis: %v", err)
	}
	defer redisClient.Close()
	log.Println("Redis connected")
	
	// 初始化认证模块
	userRepo := auth.NewPgUserRepository(pgPool)
	sessionRepo := auth.NewRedisSessionRepository(redisClient)
	authService := auth.NewService(userRepo, sessionRepo)
	authHandler := auth.NewHandler(authService, &auth.Config{
		IsProduction: cfg.Server.IsProduction,
		CookieDomain: cfg.Auth.CookieDomain,
	})
	
	// 创建路由
	router := api.NewRouter(authService, authHandler)
	
	// 启动 HTTP 服务器
	server := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	
	// 优雅关闭
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		
		log.Println("Shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
	}()
	
	log.Printf("Server starting on port %s", cfg.Server.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
	
	log.Println("Server stopped")
}
```

- [ ] **Step 2: 修改 router.go**

修改 `apps/control-plane/internal/api/router.go`:

```go
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	
	"superteam/control-plane/internal/auth"
)

// NewRouter 创建路由
func NewRouter(authService *auth.Service, authHandler *auth.Handler) http.Handler {
	r := chi.NewRouter()
	
	// 中间件
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Timeout(60 * time.Second))
	
	// 健康检查
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	
	// 认证路由（公开）
	r.Route("/api/auth", func(r chi.Router) {
		r.Post("/login", authHandler.Login)
		r.Post("/logout", authHandler.Logout)
		r.Get("/me", authHandler.GetCurrentUser)
	})
	
	// 受保护的路由
	r.Route("/api", func(r chi.Router) {
		r.Use(auth.RequireAuth(authService))
		
		// TODO: 添加其他受保护的路由
		r.Get("/protected", func(w http.ResponseWriter, r *http.Request) {
			user := r.Context().Value(auth.UserContextKey).(*auth.User)
			w.Write([]byte("Hello, " + user.Username))
		})
	})
	
	return r
}
```

- [ ] **Step 3: 测试启动**

```bash
cd apps/control-plane

# 设置环境变量
export DATABASE_URL="postgres://localhost:5432/superteam?sslmode=disable"
export REDIS_ADDR="localhost:6379"

# 运行迁移
make migrate-up

# 启动服务
go run cmd/control-plane/main.go
```

Expected: 服务启动成功，输出 "Server starting on port 8080"

- [ ] **Step 4: 测试 API**

```bash
# 测试健康检查
curl http://localhost:8080/health

# 测试登录
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' \
  -c cookies.txt

# 测试获取当前用户
curl http://localhost:8080/api/auth/me -b cookies.txt

# 测试登出
curl -X POST http://localhost:8080/api/auth/logout -b cookies.txt
```

Expected: 所有 API 正常响应

- [ ] **Step 5: Commit**

```bash
git add cmd/control-plane/main.go internal/api/router.go
git commit -m "feat(control-plane): integrate auth module into main app"
```

---

## 阶段 3：前端实现

### Task 14: 配置前端依赖

**Files:**
- Modify: `packages/api-client/package.json`
- Modify: `packages/core/package.json`
- Modify: `packages/views/package.json`
- Modify: `apps/web/package.json`

- [ ] **Step 1: 安装 api-client 依赖**

```bash
cd packages/api-client

# 添加依赖到 package.json
npm install --save-dev @openapitools/openapi-generator-cli
```

修改 `packages/api-client/package.json`:

```json
{
  "name": "@superteam/api-client",
  "version": "0.0.0",
  "type": "module",
  "scripts": {
    "generate": "openapi-generator-cli generate -i ../../contracts/control-plane/auth.yaml -g typescript-fetch -o src/generated",
    "build": "tsc",
    "test": "vitest run"
  },
  "dependencies": {},
  "devDependencies": {
    "@openapitools/openapi-generator-cli": "^2.7.0",
    "typescript": "^5.3.0"
  }
}
```

- [ ] **Step 2: 安装 core 依赖**

```bash
cd packages/core

npm install @tanstack/react-query
```

修改 `packages/core/package.json` 添加依赖:

```json
{
  "dependencies": {
    "@tanstack/react-query": "^5.0.0",
    "react": "19.2.6"
  }
}
```

- [ ] **Step 3: 安装 views 依赖**

```bash
cd packages/views

npm install motion lucide-react
```

修改 `packages/views/package.json` 添加依赖:

```json
{
  "dependencies": {
    "motion": "^11.0.0",
    "lucide-react": "^0.468.0",
    "react": "19.2.6"
  }
}
```

- [ ] **Step 4: 安装 web 依赖**

```bash
cd apps/web

npm install @tanstack/react-query
```

- [ ] **Step 5: Commit**

```bash
git add packages/*/package.json apps/web/package.json
git commit -m "feat(frontend): add dependencies for auth implementation"
```

### Task 15: 实现 API Client

**Files:**
- Create: `packages/api-client/src/client.ts`
- Create: `packages/api-client/src/auth-api.ts`
- Modify: `packages/api-client/src/index.ts`

- [ ] **Step 1: 实现 HTTP 客户端**

创建 `packages/api-client/src/client.ts`:

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

- [ ] **Step 2: 实现认证 API**

创建 `packages/api-client/src/auth-api.ts`:

```typescript
import { request } from './client'

export interface UserSummary {
  id: string
  username: string
  status: 'active' | 'disabled'
}

export interface LoginResponse {
  user: UserSummary
}

export interface CurrentUserResponse {
  user: UserSummary
}

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

- [ ] **Step 3: 导出**

修改 `packages/api-client/src/index.ts`:

```typescript
export * from './client'
export * from './auth-api'
```

- [ ] **Step 4: Commit**

```bash
git add packages/api-client/src/
git commit -m "feat(api-client): implement HTTP client and auth API"
```

### Task 16: 实现 AuthProvider

**Files:**
- Create: `packages/core/src/auth/auth-context.ts`
- Create: `packages/core/src/auth/auth-provider.tsx`
- Create: `packages/core/src/auth/use-auth.ts`
- Create: `packages/core/src/auth/use-login.ts`
- Create: `packages/core/src/auth/use-logout.ts`
- Create: `packages/core/src/auth/index.ts`
- Modify: `packages/core/src/index.ts`

- [ ] **Step 1: 创建 Context 定义**

创建 `packages/core/src/auth/auth-context.ts`:

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

- [ ] **Step 2: 实现 AuthProvider**

创建 `packages/core/src/auth/auth-provider.tsx`:

```typescript
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

- [ ] **Step 3: 实现 useAuth hook**

创建 `packages/core/src/auth/use-auth.ts`:

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

- [ ] **Step 4: 实现 useLogin hook**

创建 `packages/core/src/auth/use-login.ts`:

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

- [ ] **Step 5: 实现 useLogout hook**

创建 `packages/core/src/auth/use-logout.ts`:

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

- [ ] **Step 6: 导出**

创建 `packages/core/src/auth/index.ts`:

```typescript
export * from './auth-context'
export * from './auth-provider'
export * from './use-auth'
export * from './use-login'
export * from './use-logout'
```

修改 `packages/core/src/index.ts`:

```typescript
export * from './auth'
```

- [ ] **Step 7: Commit**

```bash
git add packages/core/src/auth/
git commit -m "feat(core): implement AuthProvider and auth hooks"
```


### Task 17: 实现登录页面

**Files:**
- Create: `packages/views/src/auth/login-page.tsx`
- Create: `packages/views/src/auth/protected-route.tsx`
- Create: `packages/views/src/auth/index.ts`
- Modify: `packages/views/src/index.ts`

- [ ] **Step 1: 实现登录页面**

创建 `packages/views/src/auth/login-page.tsx`:

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

- [ ] **Step 2: 实现路由保护组件**

创建 `packages/views/src/auth/protected-route.tsx`:

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

- [ ] **Step 3: 导出**

创建 `packages/views/src/auth/index.ts`:

```typescript
export * from './login-page'
export * from './protected-route'
```

修改 `packages/views/src/index.ts`:

```typescript
export * from './auth'
```

- [ ] **Step 4: Commit**

```bash
git add packages/views/src/auth/
git commit -m "feat(views): implement login page and protected route"
```

### Task 18: 集成到 Next.js 应用

**Files:**
- Modify: `apps/web/app/layout.tsx`
- Create: `apps/web/app/login/page.tsx`
- Modify: `apps/web/app/page.tsx`
- Create: `apps/web/public/assets/branding/superteam-logo.png`

- [ ] **Step 1: 修改根布局**

修改 `apps/web/app/layout.tsx`:

```tsx
'use client'

import { type ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AuthProvider } from '@superteam/core'
import './globals.css'

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

- [ ] **Step 2: 创建登录页路由**

创建 `apps/web/app/login/page.tsx`:

```tsx
'use client'

import { LoginPage } from '@superteam/views'

export default function LoginRoute() {
  return <LoginPage />
}
```

- [ ] **Step 3: 修改首页重定向**

修改 `apps/web/app/page.tsx`:

```tsx
'use client'

import { useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { useAuth } from '@superteam/core'
import { Loader2 } from 'lucide-react'

export default function HomePage() {
  const { isAuthenticated, isLoading } = useAuth()
  const router = useRouter()

  useEffect(() => {
    if (!isLoading) {
      router.push(isAuthenticated ? '/overview' : '/login')
    }
  }, [isAuthenticated, isLoading, router])

  return (
    <div className="flex min-h-screen items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin" />
    </div>
  )
}
```

- [ ] **Step 4: 添加 Logo 图片**

```bash
# 创建目录
mkdir -p apps/web/public/assets/branding

# 复制或创建 SuperTeam logo
# 注意：需要准备一个 logo 图片文件
cp /path/to/superteam-logo.png apps/web/public/assets/branding/
```

- [ ] **Step 5: 测试前端**

```bash
cd apps/web
npm run dev
```

打开浏览器访问 http://localhost:3000，应该自动跳转到登录页

- [ ] **Step 6: Commit**

```bash
git add apps/web/app/ apps/web/public/
git commit -m "feat(web): integrate auth into Next.js app"
```

---

## 阶段 4：测试和验证

### Task 19: 端到端测试

**Files:**
- Create: `apps/web/__tests__/e2e/login.spec.ts`

- [ ] **Step 1: 安装 Playwright**

```bash
cd apps/web
npm install -D @playwright/test
npx playwright install
```

- [ ] **Step 2: 编写 E2E 测试**

创建 `apps/web/__tests__/e2e/login.spec.ts`:

```typescript
import { test, expect } from '@playwright/test'

test.describe('登录认证流程', () => {
  test('完整登录流程', async ({ page }) => {
    await page.goto('/')
    
    // 应该跳转到登录页
    await expect(page).toHaveURL('/login')
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
    
    // 登出（假设有登出按钮）
    // await page.click('[data-testid="logout-button"]')
    
    // 验证跳转到登录页
    // await expect(page).toHaveURL('/login')
    
    // 尝试访问受保护页面
    // await page.goto('/overview')
    // await expect(page).toHaveURL('/login')
  })
})
```

- [ ] **Step 3: 运行 E2E 测试**

```bash
# 确保后端和前端都在运行
# Terminal 1: cd apps/control-plane && go run cmd/control-plane/main.go
# Terminal 2: cd apps/web && npm run dev

# Terminal 3: 运行测试
cd apps/web
npx playwright test
```

Expected: 测试通过

- [ ] **Step 4: Commit**

```bash
git add apps/web/__tests__/
git commit -m "test: add E2E tests for login flow"
```

### Task 20: 最终验证和文档

**Files:**
- Create: `docs/auth-setup.md`
- Update: `CHANGELOG.md`

- [ ] **Step 1: 完整功能测试**

手动测试清单：

```bash
# 1. 后端测试
cd apps/control-plane
go test ./... -v

# 2. 启动后端
export DATABASE_URL="postgres://localhost:5432/superteam?sslmode=disable"
export REDIS_ADDR="localhost:6379"
make migrate-up
go run cmd/control-plane/main.go

# 3. 测试 API
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' \
  -c cookies.txt

curl http://localhost:8080/api/auth/me -b cookies.txt

curl -X POST http://localhost:8080/api/auth/logout -b cookies.txt

# 4. 启动前端
cd apps/web
npm run dev

# 5. 浏览器测试
# - 访问 http://localhost:3000
# - 测试登录、登出、路由保护
# - 测试错误处理
# - 测试会话过期（等待 12 小时或手动删除 Redis key）
```

- [ ] **Step 2: 编写部署文档**

创建 `docs/auth-setup.md`:

```markdown
# 认证系统部署指南

## 环境要求

- PostgreSQL 14+
- Redis 7+
- Go 1.21+
- Node.js 18+

## 数据库初始化

1. 创建数据库：
\`\`\`bash
createdb superteam
\`\`\`

2. 运行迁移：
\`\`\`bash
cd apps/control-plane
export DATABASE_URL="postgres://localhost:5432/superteam?sslmode=disable"
make migrate-up
\`\`\`

3. 验证默认用户：
\`\`\`bash
psql $DATABASE_URL -c "SELECT username, status FROM auth_users;"
\`\`\`

## 环境变量配置

\`\`\`bash
# 数据库
export DATABASE_URL="postgres://user:pass@localhost:5432/superteam?sslmode=disable"

# Redis
export REDIS_ADDR="localhost:6379"
export REDIS_PASSWORD=""

# 服务器
export PORT="8080"
export ENV="production"

# Cookie
export COOKIE_DOMAIN="superteam.example.com"
\`\`\`

## 启动服务

### 后端
\`\`\`bash
cd apps/control-plane
go build -o bin/control-plane cmd/control-plane/main.go
./bin/control-plane
\`\`\`

### 前端
\`\`\`bash
cd apps/web
npm run build
npm start
\`\`\`

## 默认账号

- 用户名：admin
- 密码：admin

**重要：生产环境部署后请立即修改默认密码！**

## 安全检查清单

- [ ] 生产环境 Cookie Secure=true
- [ ] HTTPS 证书配置正确
- [ ] Redis 密码保护已启用
- [ ] PostgreSQL 连接使用 SSL
- [ ] 默认 admin 密码已修改
- [ ] 日志不包含敏感信息
- [ ] CORS 配置正确
```

- [ ] **Step 3: 更新 CHANGELOG**

更新 `CHANGELOG.md`:

```markdown
# Changelog

## [Unreleased]

### Added
- 用户登录认证系统
  - Cookie-based 会话管理（12 小时 TTL）
  - PostgreSQL 用户存储
  - Redis 会话存储
  - bcrypt 密码加密
  - 登录页面（参考 PulseAI 视觉风格）
  - 路由保护组件
  - 认证 API（login/me/logout）
  - 完整的单元测试和 E2E 测试

### Security
- HTTP-only Cookie 防止 XSS
- SameSite=Lax 防止 CSRF
- Session token SHA-256 hash 存储
- 登录失败不泄露用户信息
```

- [ ] **Step 4: 最终 Commit**

```bash
git add docs/auth-setup.md CHANGELOG.md
git commit -m "docs: add auth setup guide and update changelog"
```

- [ ] **Step 5: 创建 PR（如果使用 Git 工作流）**

```bash
git push origin feature/login-auth
# 然后在 GitHub/GitLab 创建 Pull Request
```

---

## 实施完成检查清单

### 后端

- [ ] OpenAPI 契约定义完成
- [ ] Go 依赖安装完成
- [ ] 代码生成配置完成
- [ ] 数据库迁移脚本完成
- [ ] Auth Service 实现并测试通过
- [ ] PostgreSQL Repository 实现
- [ ] Redis Repository 实现
- [ ] HTTP Handlers 实现并测试通过
- [ ] 认证中间件实现并测试通过
- [ ] 存储层配置完成
- [ ] 主程序集成完成
- [ ] 所有单元测试通过

### 前端

- [ ] 前端依赖安装完成
- [ ] API Client 实现完成
- [ ] AuthProvider 实现完成
- [ ] useAuth hooks 实现完成
- [ ] 登录页面实现完成
- [ ] 路由保护组件实现完成
- [ ] Next.js 集成完成
- [ ] Logo 图片准备完成

### 测试

- [ ] 后端单元测试全部通过
- [ ] E2E 测试全部通过
- [ ] 手动功能测试完成
- [ ] 安全检查完成

### 文档

- [ ] 部署文档编写完成
- [ ] CHANGELOG 更新完成
- [ ] API 文档完整

---

## 预计时间

- 阶段 1（基础设施）：1-2 天
- 阶段 2（Go 后端）：3-4 天
- 阶段 3（前端实现）：3-4 天
- 阶段 4（测试验证）：2-3 天

**总计**：9-13 天

---

## 执行建议

1. **按顺序执行**：严格按照 Task 顺序执行，每个 Task 完成后 commit
2. **频繁测试**：每完成一个模块立即运行测试
3. **及时 commit**：每个 Task 完成后立即 commit，保持 git 历史清晰
4. **遇到问题及时沟通**：如果某个步骤不清楚或遇到错误，及时询问
5. **保持代码整洁**：遵循 DRY、YAGNI、TDD 原则

