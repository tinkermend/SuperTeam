# Login Captcha Implementation Plan
> 复核状态：已实现（2026-06-30完成）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a mandatory 4-character image captcha to the SuperTeam Web login flow, with server-side validation and PostgreSQL-backed one-time challenges.

**Architecture:** Control Plane owns captcha generation, persistence, validation, consumption, and login-log auditing. Web loads a captcha challenge before login, displays the PNG data URL, submits `captcha_id` and `captcha_code` with credentials, and refreshes the challenge after any login failure.

**Tech Stack:** Go 1.25, chi/oapi-codegen, pgx/sqlc, PostgreSQL Atlas migrations, React 19, React Hook Form, Zod, TanStack Router, Vitest browser tests, `golang.org/x/image/font/basicfont` for simple PNG text rendering.

---

## Scope And Worktree

Implement this in an isolated worktree because it touches database migrations, generated sqlc/OpenAPI files, Go service/handler code, Web API types, and visible login UI.

Before executing the plan:

```bash
git status --short
```

Expected: current dirty root may contain unrelated user work. Do not stage or modify unrelated files. If executing in this root is unsafe, create a dedicated worktree with the `superpowers:using-git-worktrees` skill.

## File Structure

- Create `apps/control-plane/internal/auth/captcha.go`: captcha code generation, answer normalization/hash, PNG data URL rendering, challenge create/validate service methods.
- Create `apps/control-plane/internal/storage/migrations/039_auth_captcha_challenges.sql`: PostgreSQL challenge table.
- Create `apps/control-plane/internal/storage/queries/captcha.sql`: sqlc queries for create, lock-read, consume, and cleanup.
- Modify `apps/control-plane/internal/auth/types.go`: captcha domain types and login failure constants.
- Modify `apps/control-plane/internal/auth/errors.go`: captcha-specific errors.
- Modify `apps/control-plane/internal/auth/service.go`: repository interface additions and service options for captcha secret/TTL.
- Modify `apps/control-plane/internal/auth/pg_repository.go`: sqlc-backed captcha persistence methods.
- Modify `apps/control-plane/internal/auth/handler.go`: `GET /api/auth/captcha`, captcha-aware `POST /api/auth/login`, captcha error mapping.
- Modify `contracts/control-plane/auth.yaml`: captcha endpoint and login request fields.
- Regenerate `apps/control-plane/internal/auth/generated.go`.
- Modify `apps/control-plane/internal/storage/migrations/atlas.sum`: Atlas migration checksum.
- Regenerate `apps/control-plane/internal/storage/queries/*.go`.
- Modify `apps/web/src/lib/api/auth.ts`: captcha response type, `getLoginCaptcha`, login request fields.
- Modify `apps/web/src/features/auth/auth-context.tsx`: expose `apiBaseUrl` and update login credentials type.
- Modify `apps/web/src/features/auth/auth-provider.tsx`: include `apiBaseUrl` in context value and pass captcha fields through.
- Modify `apps/web/src/features/auth/sign-in/components/user-auth-form.tsx`: captcha input, image, refresh, failure refresh behavior.
- Tests: `apps/control-plane/internal/auth/service_test.go`, `apps/control-plane/internal/auth/handler_test.go`, `apps/control-plane/internal/storage/migrations_test.go`, `apps/web/src/lib/api/auth.test.ts`, `apps/web/src/features/auth/auth-provider.test.tsx`, `apps/web/src/features/auth/sign-in/components/user-auth-form.test.tsx`.
- Optional config doc update: `apps/control-plane/config/config.example.yaml` documents `AUTH_CAPTCHA_SECRET` as production hardening, but the first implementation must still run when it is absent by generating a process-local secret.

## Task 1: Captcha Domain And Service Tests

**Files:**
- Create: `apps/control-plane/internal/auth/captcha.go`
- Modify: `apps/control-plane/internal/auth/types.go`
- Modify: `apps/control-plane/internal/auth/errors.go`
- Modify: `apps/control-plane/internal/auth/service.go`
- Test: `apps/control-plane/internal/auth/service_test.go`

- [ ] **Step 1: Add captcha domain types and errors**

In `apps/control-plane/internal/auth/types.go`, add:

```go
const (
	LoginFailureCaptchaInvalid = "captcha_invalid"
	LoginFailureCaptchaExpired = "captcha_expired"
)

type CaptchaChallenge struct {
	ID           uuid.UUID
	ImageDataURL string
	ExpiresAt    time.Time
}

type CaptchaChallengeRecord struct {
	ID         uuid.UUID
	TenantID   uuid.UUID
	AnswerHash string
	ExpiresAt  time.Time
	UsedAt     *time.Time
	ClientIP   string
	UserAgent  string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type CreateCaptchaChallengeParams struct {
	ID         uuid.UUID
	TenantID   uuid.UUID
	AnswerHash string
	ExpiresAt  time.Time
	ClientIP   string
	UserAgent  string
}
```

In `apps/control-plane/internal/auth/errors.go`, add:

```go
var (
	ErrCaptchaInvalid = errors.New("captcha invalid")
	ErrCaptchaExpired = errors.New("captcha expired")
	ErrCaptchaUsed    = errors.New("captcha already used")
)
```

- [ ] **Step 2: Extend the repository interface and service options**

In `apps/control-plane/internal/auth/service.go`, extend `Repository`:

```go
	CreateCaptchaChallenge(ctx context.Context, params CreateCaptchaChallengeParams) (*CaptchaChallengeRecord, error)
	GetCaptchaChallengeForUpdate(ctx context.Context, id uuid.UUID) (*CaptchaChallengeRecord, error)
	ConsumeCaptchaChallenge(ctx context.Context, id uuid.UUID, usedAt time.Time) error
	DeleteExpiredCaptchaChallenges(ctx context.Context, before time.Time) error
```

Add service options:

```go
type ServiceOption func(*Service) error

type CaptchaOptions struct {
	Secret string
	TTL    time.Duration
	Now    func() time.Time
}

func WithCaptchaOptions(options CaptchaOptions) ServiceOption {
	return func(s *Service) error {
		if options.TTL <= 0 {
			options.TTL = 5 * time.Minute
		}
		if options.Now == nil {
			options.Now = func() time.Time { return time.Now().UTC() }
		}
		secret := strings.TrimSpace(options.Secret)
		if secret == "" {
			token, err := GenerateToken()
			if err != nil {
				return err
			}
			secret = token
			log.Println("auth captcha secret is not configured; using process-local secret")
		}
		s.captchaSecret = []byte(secret)
		s.captchaTTL = options.TTL
		s.now = options.Now
		return nil
	}
}
```

Change `NewService` signature to accept options:

```go
func NewService(repo Repository, options ...ServiceOption) (*Service, error)
```

Initialize default captcha settings when no option is supplied:

```go
svc := &Service{
	repo:       repo,
	captchaTTL: 5 * time.Minute,
	now:       func() time.Time { return time.Now().UTC() },
}
if err := WithCaptchaOptions(CaptchaOptions{})(svc); err != nil {
	return nil, err
}
for _, option := range options {
	if err := option(svc); err != nil {
		return nil, err
	}
}
return svc, nil
```

- [ ] **Step 3: Add failing service tests**

In `apps/control-plane/internal/auth/service_test.go`, extend `mockRepo`:

```go
	captchaChallenges map[uuid.UUID]*CaptchaChallengeRecord
```

Initialize it in `newMockRepo()`:

```go
captchaChallenges: make(map[uuid.UUID]*CaptchaChallengeRecord),
```

Add mock methods:

```go
func (m *mockRepo) CreateCaptchaChallenge(ctx context.Context, params CreateCaptchaChallengeParams) (*CaptchaChallengeRecord, error) {
	record := &CaptchaChallengeRecord{
		ID:         params.ID,
		TenantID:   params.TenantID,
		AnswerHash: params.AnswerHash,
		ExpiresAt:  params.ExpiresAt,
		ClientIP:   params.ClientIP,
		UserAgent:  params.UserAgent,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	if record.ID == uuid.Nil {
		record.ID = uuid.New()
	}
	m.captchaChallenges[record.ID] = record
	return record, nil
}

func (m *mockRepo) GetCaptchaChallengeForUpdate(ctx context.Context, id uuid.UUID) (*CaptchaChallengeRecord, error) {
	record, ok := m.captchaChallenges[id]
	if !ok {
		return nil, ErrCaptchaInvalid
	}
	copied := *record
	return &copied, nil
}

func (m *mockRepo) ConsumeCaptchaChallenge(ctx context.Context, id uuid.UUID, usedAt time.Time) error {
	record, ok := m.captchaChallenges[id]
	if !ok {
		return ErrCaptchaInvalid
	}
	if record.UsedAt != nil {
		return ErrCaptchaUsed
	}
	record.UsedAt = &usedAt
	record.UpdatedAt = usedAt
	return nil
}

func (m *mockRepo) DeleteExpiredCaptchaChallenges(ctx context.Context, before time.Time) error {
	for id, record := range m.captchaChallenges {
		if record.ExpiresAt.Before(before) {
			delete(m.captchaChallenges, id)
		}
	}
	return nil
}
```

Add tests:

```go
func TestCreateCaptchaChallengeReturnsImageAndPersistsHash(t *testing.T) {
	repo := newMockRepo()
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	svc, err := NewService(repo, WithCaptchaOptions(CaptchaOptions{
		Secret: "test-captcha-secret",
		TTL:    5 * time.Minute,
		Now:    func() time.Time { return now },
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	challenge, err := svc.CreateCaptcha(context.Background(), "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("create captcha: %v", err)
	}
	if challenge.ID == uuid.Nil {
		t.Fatal("expected captcha id")
	}
	if !strings.HasPrefix(challenge.ImageDataURL, "data:image/png;base64,") {
		t.Fatalf("expected png data url, got %q", challenge.ImageDataURL)
	}
	if !challenge.ExpiresAt.Equal(now.Add(5 * time.Minute)) {
		t.Fatalf("expected ttl expiry, got %s", challenge.ExpiresAt)
	}
	record := repo.captchaChallenges[challenge.ID]
	if record == nil {
		t.Fatal("expected persisted challenge")
	}
	if record.AnswerHash == "" || len(record.AnswerHash) < 32 {
		t.Fatalf("expected answer hash, got %q", record.AnswerHash)
	}
}

func TestCaptchaCodeGenerationIncludesDigitAndLetter(t *testing.T) {
	for i := 0; i < 100; i++ {
		code, err := generateCaptchaCode()
		if err != nil {
			t.Fatalf("generate code: %v", err)
		}
		if len(code) != 4 {
			t.Fatalf("expected four characters, got %q", code)
		}
		if !captchaHasDigit(code) || !captchaHasLetter(code) {
			t.Fatalf("expected digit and letter in %q", code)
		}
	}
}

func TestValidateAndConsumeCaptchaIsCaseInsensitiveAndOneTime(t *testing.T) {
	repo := newMockRepo()
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	svc, err := NewService(repo, WithCaptchaOptions(CaptchaOptions{
		Secret: "test-captcha-secret",
		Now:    func() time.Time { return now },
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	captchaID := uuid.New()
	record := &CaptchaChallengeRecord{
		ID:         captchaID,
		TenantID:   uuid.MustParse(DefaultTenantID),
		AnswerHash: svc.hashCaptchaAnswer(captchaID.String(), "A7K2"),
		ExpiresAt:  now.Add(time.Minute),
	}
	repo.captchaChallenges[record.ID] = record

	if err := svc.ValidateAndConsumeCaptcha(context.Background(), record.ID, "a7k2", "admin", "127.0.0.1", "agent"); err != nil {
		t.Fatalf("validate captcha: %v", err)
	}
	if err := svc.ValidateAndConsumeCaptcha(context.Background(), record.ID, "A7K2", "admin", "127.0.0.1", "agent"); !errors.Is(err, ErrCaptchaUsed) {
		t.Fatalf("expected used captcha, got %v", err)
	}
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/auth -run 'Test(CreateCaptcha|CaptchaCode|ValidateAndConsumeCaptcha)' -count=1
```

Expected: FAIL because `Service` does not yet have captcha fields and methods such as `CreateCaptcha`, `ValidateAndConsumeCaptcha`, and helper functions.

- [ ] **Step 5: Implement captcha domain and service methods**

Create `apps/control-plane/internal/auth/captcha.go` with:

```go
package auth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const (
	captchaDigits  = "23456789"
	captchaLetters = "ABCDEFGHJKLMNPQRSTUVWXYZ"
)

func (s *Service) CreateCaptcha(ctx context.Context, clientIP, userAgent string) (*CaptchaChallenge, error) {
	now := s.now()
	_ = s.repo.DeleteExpiredCaptchaChallenges(ctx, now)
	code, err := generateCaptchaCode()
	if err != nil {
		return nil, err
	}
	id := uuid.New()
	expiresAt := now.Add(s.captchaTTL)
	imageDataURL, err := renderCaptchaPNGDataURL(code)
	if err != nil {
		return nil, err
	}
	record, err := s.repo.CreateCaptchaChallenge(ctx, CreateCaptchaChallengeParams{
		ID:         id,
		TenantID:   uuid.MustParse(DefaultTenantID),
		AnswerHash: s.hashCaptchaAnswer(id.String(), code),
		ExpiresAt:  expiresAt,
		ClientIP:   clientIP,
		UserAgent:  userAgent,
	})
	if err != nil {
		return nil, err
	}
	return &CaptchaChallenge{ID: record.ID, ImageDataURL: imageDataURL, ExpiresAt: expiresAt}, nil
}

func (s *Service) ValidateAndConsumeCaptcha(ctx context.Context, captchaID uuid.UUID, code, username, clientIP, userAgent string) error {
	normalized := normalizeCaptchaCode(code)
	if captchaID == uuid.Nil || len(normalized) != 4 {
		s.recordCaptchaFailure(ctx, username, clientIP, userAgent, LoginFailureCaptchaInvalid)
		return ErrCaptchaInvalid
	}
	var result error
	err := s.repo.WithTransaction(ctx, func(repo Repository) error {
		record, err := repo.GetCaptchaChallengeForUpdate(ctx, captchaID)
		if err != nil {
			// Only a genuinely missing challenge is a user-facing "invalid
			// captcha". Any other error (timeout, connection loss) is an
			// infrastructure failure and must roll the transaction back and
			// propagate, not be masked as a wrong code.
			if errors.Is(err, ErrCaptchaInvalid) {
				result = ErrCaptchaInvalid
				return nil
			}
			return err
		}
		now := s.now()
		if record.UsedAt != nil {
			result = ErrCaptchaUsed
			return nil
		}
		if !record.ExpiresAt.After(now) {
			_ = repo.ConsumeCaptchaChallenge(ctx, captchaID, now)
			result = ErrCaptchaExpired
			return nil
		}
		expected := s.hashCaptchaAnswer(captchaID.String(), normalized)
		if !hmac.Equal([]byte(record.AnswerHash), []byte(expected)) {
			_ = repo.ConsumeCaptchaChallenge(ctx, captchaID, now)
			result = ErrCaptchaInvalid
			return nil
		}
		result = repo.ConsumeCaptchaChallenge(ctx, captchaID, now)
		return result
	})
	if err != nil {
		return err
	}
	if result != nil {
		reason := LoginFailureCaptchaInvalid
		if errors.Is(result, ErrCaptchaExpired) {
			reason = LoginFailureCaptchaExpired
		}
		s.recordCaptchaFailure(ctx, username, clientIP, userAgent, reason)
		return result
	}
	return nil
}
```

Add helper functions in the same file:

```go
func generateCaptchaCode() (string, error) {
	digit, err := secureChar(captchaDigits)
	if err != nil {
		return "", err
	}
	letter, err := secureChar(captchaLetters)
	if err != nil {
		return "", err
	}
	alphabet := captchaDigits + captchaLetters
	chars := []byte{digit, letter}
	for len(chars) < 4 {
		ch, err := secureChar(alphabet)
		if err != nil {
			return "", err
		}
		chars = append(chars, ch)
	}
	for i := len(chars) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return "", err
		}
		j := int(n.Int64())
		chars[i], chars[j] = chars[j], chars[i]
	}
	return string(chars), nil
}

func secureChar(alphabet string) (byte, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
	if err != nil {
		return 0, err
	}
	return alphabet[n.Int64()], nil
}

func normalizeCaptchaCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func captchaHasDigit(code string) bool {
	return strings.ContainsAny(code, captchaDigits)
}

func captchaHasLetter(code string) bool {
	return strings.ContainsAny(code, captchaLetters)
}

func (s *Service) hashCaptchaAnswer(id, code string) string {
	mac := hmac.New(sha256.New, s.captchaSecret)
	mac.Write([]byte(id + ":" + normalizeCaptchaCode(code)))
	return id + ":" + hex.EncodeToString(mac.Sum(nil))
}

func renderCaptchaPNGDataURL(code string) (string, error) {
	img := image.NewRGBA(image.Rect(0, 0, 132, 44))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.RGBA{R: 245, G: 247, B: 250, A: 255}}, image.Point{}, draw.Src)
	drawer := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.RGBA{R: 32, G: 38, B: 52, A: 255}),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(28, 27),
	}
	drawer.DrawString(code)
	for x := 8; x < 128; x += 17 {
		for y := 8; y < 36; y += 11 {
			img.Set(x, y, color.RGBA{R: 115, G: 129, B: 148, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func (s *Service) recordCaptchaFailure(ctx context.Context, username, clientIP, userAgent, reason string) {
	_ = s.repo.CreateLoginLog(ctx, CreateLoginLogParams{
		EventType:     LoginEventFailed,
		Username:      username,
		ClientIP:      clientIP,
		UserAgent:     userAgent,
		Result:        LoginResultFailed,
		FailureReason: reason,
	})
}
```

Also add `captchaSecret []byte`, `captchaTTL time.Duration`, and `now func() time.Time` fields to `Service`.

- [ ] **Step 6: Run tests and tidy dependencies**

Run:

```bash
go test ./apps/control-plane/internal/auth -run 'Test(CreateCaptcha|CaptchaCode|ValidateAndConsumeCaptcha)' -count=1
(cd apps/control-plane && go mod tidy)
```

Expected: targeted auth tests PASS. `go.mod` / `go.sum` may add `golang.org/x/image`.

- [ ] **Step 7: Commit Task 1**

```bash
git add apps/control-plane/go.mod apps/control-plane/go.sum apps/control-plane/internal/auth/captcha.go apps/control-plane/internal/auth/errors.go apps/control-plane/internal/auth/service.go apps/control-plane/internal/auth/service_test.go apps/control-plane/internal/auth/types.go
git commit -m "feat(auth): add captcha challenge service"
```

Expected: commit includes only auth domain/service changes and dependency updates.

## Task 2: PostgreSQL Schema, sqlc, And Repository

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/039_auth_captcha_challenges.sql`
- Create: `apps/control-plane/internal/storage/queries/captcha.sql`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`
- Modify generated: `apps/control-plane/internal/storage/queries/*.go`
- Modify: `apps/control-plane/internal/auth/pg_repository.go`
- Test: `apps/control-plane/internal/storage/migrations_test.go`

- [ ] **Step 1: Add migration test**

In `apps/control-plane/internal/storage/migrations_test.go`, add:

```go
func TestAuthCaptchaChallengeMigrationAddsOneTimeChallengeStorage(t *testing.T) {
	body, err := os.ReadFile("migrations/039_auth_captcha_challenges.sql")
	if err != nil {
		t.Fatalf("read captcha migration: %v", err)
	}
	sql := string(body)
	expected := []string{
		"CREATE TABLE IF NOT EXISTS auth_captcha_challenges",
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid",
		"answer_hash VARCHAR(255) NOT NULL",
		"expires_at TIMESTAMPTZ NOT NULL",
		"used_at TIMESTAMPTZ",
		"CREATE INDEX IF NOT EXISTS idx_auth_captcha_challenges_expires_at",
		"CREATE INDEX IF NOT EXISTS idx_auth_captcha_challenges_used_at",
		"COMMENT ON TABLE auth_captcha_challenges IS",
	}
	for _, item := range expected {
		if !strings.Contains(sql, item) {
			t.Fatalf("expected migration to contain %q", item)
		}
	}
}
```

- [ ] **Step 2: Run migration test to verify it fails**

```bash
go test ./apps/control-plane/internal/storage -run TestAuthCaptchaChallengeMigrationAddsOneTimeChallengeStorage -count=1
```

Expected: FAIL because migration `039_auth_captcha_challenges.sql` does not exist.

- [ ] **Step 3: Create migration**

Create `apps/control-plane/internal/storage/migrations/039_auth_captcha_challenges.sql`:

```sql
CREATE TABLE IF NOT EXISTS auth_captcha_challenges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid,
    answer_hash VARCHAR(255) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    client_ip VARCHAR(255),
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_auth_captcha_challenges_expires_at
    ON auth_captcha_challenges(expires_at);

CREATE INDEX IF NOT EXISTS idx_auth_captcha_challenges_used_at
    ON auth_captcha_challenges(used_at);

CREATE INDEX IF NOT EXISTS idx_auth_captcha_challenges_created_at
    ON auth_captcha_challenges(created_at DESC);

CREATE TRIGGER update_auth_captcha_challenges_updated_at
    BEFORE UPDATE ON auth_captcha_challenges
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE auth_captcha_challenges IS 'Web 登录图形验证码挑战表';
COMMENT ON COLUMN auth_captcha_challenges.id IS '验证码挑战主键 UUID';
COMMENT ON COLUMN auth_captcha_challenges.tenant_id IS '所属租户 ID';
COMMENT ON COLUMN auth_captcha_challenges.answer_hash IS '验证码答案哈希，不保存明文';
COMMENT ON COLUMN auth_captcha_challenges.expires_at IS '验证码过期时间';
COMMENT ON COLUMN auth_captcha_challenges.used_at IS '验证码消费时间；非空表示已使用';
COMMENT ON COLUMN auth_captcha_challenges.client_ip IS '创建验证码的客户端 IP';
COMMENT ON COLUMN auth_captcha_challenges.user_agent IS '创建验证码的 User-Agent';
COMMENT ON COLUMN auth_captcha_challenges.created_at IS '创建时间';
COMMENT ON COLUMN auth_captcha_challenges.updated_at IS '更新时间';
```

- [ ] **Step 4: Add sqlc queries**

Create `apps/control-plane/internal/storage/queries/captcha.sql`:

```sql
-- name: CreateCaptchaChallenge :one
INSERT INTO auth_captcha_challenges (
    id,
    tenant_id,
    answer_hash,
    expires_at,
    client_ip,
    user_agent
) VALUES (
    sqlc.arg('id')::uuid,
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('answer_hash')::varchar,
    sqlc.arg('expires_at')::timestamptz,
    sqlc.narg('client_ip')::varchar,
    sqlc.narg('user_agent')::text
) RETURNING *;

-- name: GetCaptchaChallengeForUpdate :one
SELECT * FROM auth_captcha_challenges
WHERE id = sqlc.arg('id')::uuid
FOR UPDATE;

-- name: ConsumeCaptchaChallenge :execrows
UPDATE auth_captcha_challenges
SET used_at = sqlc.arg('used_at')::timestamptz,
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND used_at IS NULL;

-- name: DeleteExpiredCaptchaChallenges :exec
DELETE FROM auth_captcha_challenges
WHERE expires_at < sqlc.arg('before')::timestamptz;
```

- [ ] **Step 5: Generate sqlc and Atlas checksum**

```bash
make -C apps/control-plane generate-sqlc
atlas migrate hash --dir file://apps/control-plane/internal/storage/migrations
```

Expected: generated query files update and `atlas.sum` includes migration `039_auth_captcha_challenges.sql`.

- [ ] **Step 6: Implement PgRepository captcha methods**

In `apps/control-plane/internal/auth/pg_repository.go`, add methods:

```go
func (r *PgRepository) CreateCaptchaChallenge(ctx context.Context, params CreateCaptchaChallengeParams) (*CaptchaChallengeRecord, error) {
	row, err := r.q.CreateCaptchaChallenge(ctx, queries.CreateCaptchaChallengeParams{
		ID:         params.ID,
		TenantID:   params.TenantID,
		AnswerHash: params.AnswerHash,
		ExpiresAt:  pgtype.Timestamptz{Time: params.ExpiresAt, Valid: true},
		ClientIp:   pgtype.Text{String: params.ClientIP, Valid: params.ClientIP != ""},
		UserAgent:  pgtype.Text{String: params.UserAgent, Valid: params.UserAgent != ""},
	})
	if err != nil {
		return nil, err
	}
	return toDomainCaptchaChallenge(row), nil
}

func (r *PgRepository) GetCaptchaChallengeForUpdate(ctx context.Context, id uuid.UUID) (*CaptchaChallengeRecord, error) {
	row, err := r.q.GetCaptchaChallengeForUpdate(ctx, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrCaptchaInvalid
		}
		return nil, err
	}
	return toDomainCaptchaChallenge(row), nil
}

func (r *PgRepository) ConsumeCaptchaChallenge(ctx context.Context, id uuid.UUID, usedAt time.Time) error {
	rows, err := r.q.ConsumeCaptchaChallenge(ctx, queries.ConsumeCaptchaChallengeParams{
		ID:     id,
		UsedAt: pgtype.Timestamptz{Time: usedAt, Valid: true},
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrCaptchaUsed
	}
	return nil
}

func (r *PgRepository) DeleteExpiredCaptchaChallenges(ctx context.Context, before time.Time) error {
	return r.q.DeleteExpiredCaptchaChallenges(ctx, pgtype.Timestamptz{Time: before, Valid: true})
}
```

Add mapper:

```go
func toDomainCaptchaChallenge(row queries.AuthCaptchaChallenge) *CaptchaChallengeRecord {
	return &CaptchaChallengeRecord{
		ID:         row.ID,
		TenantID:   row.TenantID,
		AnswerHash: row.AnswerHash,
		ExpiresAt:  row.ExpiresAt.Time,
		UsedAt:     timePtr(row.UsedAt),
		ClientIP:   row.ClientIp.String,
		UserAgent:  row.UserAgent.String,
		CreatedAt:  row.CreatedAt.Time,
		UpdatedAt:  row.UpdatedAt.Time,
	}
}
```

- [ ] **Step 7: Run migration/storage tests**

```bash
go test ./apps/control-plane/internal/storage -run 'TestAuthCaptchaChallengeMigration|TestMigrations' -count=1
go test ./apps/control-plane/internal/auth -run 'Test(CreateCaptcha|CaptchaCode|ValidateAndConsumeCaptcha)' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit Task 2**

```bash
git add apps/control-plane/internal/storage/migrations/039_auth_captcha_challenges.sql apps/control-plane/internal/storage/migrations/atlas.sum apps/control-plane/internal/storage/migrations_test.go apps/control-plane/internal/storage/queries/captcha.sql apps/control-plane/internal/storage/queries apps/control-plane/internal/auth/pg_repository.go
git commit -m "feat(auth): persist captcha challenges"
```

Expected: commit contains migration, sqlc query source/generated files, repository mapping, and migration test.

## Task 3: Auth API Contract And Handler

**Files:**
- Modify: `contracts/control-plane/auth.yaml`
- Regenerate: `apps/control-plane/internal/auth/generated.go`
- Modify: `apps/control-plane/internal/auth/handler.go`
- Test: `apps/control-plane/internal/auth/handler_test.go`
- Test: `apps/control-plane/internal/auth/service_test.go`

- [ ] **Step 1: Add failing handler tests**

In `apps/control-plane/internal/auth/handler_test.go`, add:

```go
func TestHTTPHandlerCreatesCaptchaChallenge(t *testing.T) {
	_, _, handler, _ := newAuthenticatedHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/api/auth/captcha", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	request.Header.Set("User-Agent", "test-agent")
	recorder := httptest.NewRecorder()

	handler.CreateCaptcha(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response CaptchaChallengeResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if uuid.UUID(response.CaptchaId) == uuid.Nil {
		t.Fatal("expected captcha id")
	}
	if !strings.HasPrefix(response.ImageDataUrl, "data:image/png;base64,") {
		t.Fatalf("expected image data url, got %q", response.ImageDataUrl)
	}
}

func TestHTTPHandlerLoginRequiresCaptcha(t *testing.T) {
	_, _, handler, _ := newAuthenticatedHandler(t)
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"operator","password":"secret"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.Login(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
```

- [ ] **Step 2: Run handler tests to verify they fail**

```bash
go test ./apps/control-plane/internal/auth -run 'TestHTTPHandler(CreateCaptcha|LoginRequiresCaptcha)' -count=1
```

Expected: FAIL because generated types and handler method `CreateCaptcha` do not exist.

- [ ] **Step 3: Update OpenAPI auth contract**

In `contracts/control-plane/auth.yaml`, add path:

```yaml
  /api/auth/captcha:
    get:
      summary: 获取登录图形验证码
      operationId: createCaptcha
      tags:
        - Auth
      responses:
        '200':
          description: 成功
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/CaptchaChallengeResponse'
```

Update login request schema required fields:

```yaml
              required:
                - username
                - password
                - captcha_id
                - captcha_code
              properties:
                captcha_id:
                  type: string
                  format: uuid
                captcha_code:
                  type: string
                  minLength: 4
                  maxLength: 4
```

Add schema:

```yaml
    CaptchaChallengeResponse:
      type: object
      required:
        - captcha_id
        - image_data_url
        - expires_at
      properties:
        captcha_id:
          type: string
          format: uuid
        image_data_url:
          type: string
        expires_at:
          type: string
          format: date-time
```

- [ ] **Step 4: Regenerate auth OpenAPI server**

```bash
make -C apps/control-plane generate-openapi
```

Expected: `apps/control-plane/internal/auth/generated.go` contains `CreateCaptcha` and login request fields `CaptchaId` / `CaptchaCode`.

- [ ] **Step 5: Implement handler method and login captcha validation**

In `apps/control-plane/internal/auth/handler.go`, add:

```go
func (h *HTTPHandler) CreateCaptcha(w http.ResponseWriter, r *http.Request) {
	challenge, err := h.service.CreateCaptcha(r.Context(), clientIP(r), r.UserAgent())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, CaptchaChallengeResponse{
		CaptchaId:    openapi_types.UUID(challenge.ID),
		ImageDataUrl: challenge.ImageDataURL,
		ExpiresAt:    challenge.ExpiresAt,
	})
}
```

Update `Login` before calling `h.service.Login`:

```go
captchaID := uuid.UUID(body.CaptchaId)
if captchaID == uuid.Nil || strings.TrimSpace(body.CaptchaCode) == "" {
	writeError(w, http.StatusBadRequest, "captcha is required")
	return
}
if err := h.service.ValidateAndConsumeCaptcha(r.Context(), captchaID, body.CaptchaCode, body.Username, clientIP(r), r.UserAgent()); err != nil {
	h.writeAuthError(w, err)
	return
}
```

Update `writeAuthError`:

```go
case errors.Is(err, ErrCaptchaInvalid), errors.Is(err, ErrCaptchaExpired), errors.Is(err, ErrCaptchaUsed):
	writeError(w, http.StatusUnauthorized, "验证码不正确或已过期")
```

Add `strings` to `apps/control-plane/internal/auth/handler.go` imports.

- [ ] **Step 6: Add login success/failed captcha handler tests**

In `apps/control-plane/internal/auth/handler_test.go`, add a helper:

```go
func createCaptchaForHandlerLogin(t *testing.T, handler *HTTPHandler) CaptchaChallengeResponse {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/auth/captcha", nil)
	recorder := httptest.NewRecorder()
	handler.CreateCaptcha(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("create captcha got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response CaptchaChallengeResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode captcha: %v", err)
	}
	return response
}
```

Add test with a deterministic repository challenge:

```go
func TestHTTPHandlerRejectsInvalidCaptchaBeforePassword(t *testing.T) {
	repo, _, handler, _ := newAuthenticatedHandler(t)
	challenge := createCaptchaForHandlerLogin(t, handler)
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(fmt.Sprintf(`{
		"username":"operator",
		"password":"secret",
		"captcha_id":"%s",
		"captcha_code":"ZZZZ"
	}`, uuid.UUID(challenge.CaptchaId))))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.Login(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if len(repo.loginLogs) == 0 || repo.loginLogs[len(repo.loginLogs)-1].FailureReason != LoginFailureCaptchaInvalid {
		t.Fatalf("expected captcha failure log, got %#v", repo.loginLogs)
	}
}
```

Add this test-only helper in `apps/control-plane/internal/auth/handler_test.go`:

```go
func (s *Service) createCaptchaForTest(ctx context.Context, code string) (*CaptchaChallengeRecord, error) {
	id := uuid.New()
	now := s.now()
	return s.repo.CreateCaptchaChallenge(ctx, CreateCaptchaChallengeParams{
		ID:         id,
		TenantID:   uuid.MustParse(DefaultTenantID),
		AnswerHash: s.hashCaptchaAnswer(id.String(), code),
		ExpiresAt:  now.Add(s.captchaTTL),
	})
}
```

Use it to verify `Login` returns `200` and sets `session_token`.

- [ ] **Step 7: Run contract and handler tests**

```bash
go test ./apps/control-plane/internal/auth -run 'TestHTTPHandler.*Captcha|TestLogin' -count=1
corepack pnpm verify:contracts
```

Expected: targeted auth tests PASS and contract verification PASS.

- [ ] **Step 8: Commit Task 3**

```bash
git add contracts/control-plane/auth.yaml apps/control-plane/internal/auth/generated.go apps/control-plane/internal/auth/handler.go apps/control-plane/internal/auth/handler_test.go apps/control-plane/internal/auth/service_test.go
git commit -m "feat(auth): require captcha on login"
```

Expected: commit contains contract, generated server types, handler changes, and handler tests.

## Task 4: Web API Client And Auth Provider

**Files:**
- Modify: `apps/web/src/lib/api/auth.ts`
- Modify: `apps/web/src/lib/api/auth.test.ts`
- Modify: `apps/web/src/features/auth/auth-context.tsx`
- Modify: `apps/web/src/features/auth/auth-provider.tsx`
- Modify: `apps/web/src/features/auth/auth-provider.test.tsx`

- [ ] **Step 1: Add failing Web API tests**

In `apps/web/src/lib/api/auth.test.ts`, update the login test input to include captcha fields and expected body:

```ts
await expect(
  login(
    {
      baseUrl: "http://control-plane.local/",
      fetcher,
    },
    {
      username: "admin",
      password: "admin",
      captcha_id: "11111111-1111-4111-8111-111111111111",
      captcha_code: "A7K2",
    },
  ),
).resolves.toMatchObject({
  user: {
    username: "admin",
  },
});

expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/auth/login", {
  body: JSON.stringify({
    username: "admin",
    password: "admin",
    captcha_id: "11111111-1111-4111-8111-111111111111",
    captcha_code: "A7K2",
  }),
  credentials: "include",
  headers: {
    accept: "application/json",
    "content-type": "application/json",
  },
  method: "POST",
});
```

Add captcha load test:

```ts
it("loads a login captcha challenge", async () => {
  const fetcher = vi.fn(async () =>
    jsonResponse({
      captcha_id: "11111111-1111-4111-8111-111111111111",
      image_data_url: "data:image/png;base64,abc",
      expires_at: "2026-06-30T10:00:00Z",
    }),
  );

  await expect(
    getLoginCaptcha({
      baseUrl: "http://control-plane.local/",
      fetcher,
    }),
  ).resolves.toMatchObject({
    captcha_id: "11111111-1111-4111-8111-111111111111",
    image_data_url: "data:image/png;base64,abc",
  });

  expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/auth/captcha", {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });
});
```

- [ ] **Step 2: Run Web API tests to verify they fail**

```bash
corepack pnpm --filter ./apps/web run test -- src/lib/api/auth.test.ts
```

Expected: FAIL because `getLoginCaptcha` and captcha request fields do not exist.

- [ ] **Step 3: Update Web API client types and function**

In `apps/web/src/lib/api/auth.ts`, update:

```ts
export type LoginRequest = {
  captcha_code: string;
  captcha_id: string;
  password: string;
  username: string;
};

export type CaptchaChallengeResponse = {
  captcha_id: string;
  expires_at: string;
  image_data_url: string;
};
```

Add:

```ts
export async function getLoginCaptcha(options: ApiClientOptions): Promise<CaptchaChallengeResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/auth/captcha"), {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });

  return parseJson<CaptchaChallengeResponse>(response, "auth captcha");
}
```

- [ ] **Step 4: Update AuthContext/AuthProvider types**

In `apps/web/src/features/auth/auth-context.tsx`, change:

```ts
apiBaseUrl: string
login: (credentials: {
  captcha_code: string
  captcha_id: string
  password: string
  username: string
}) => Promise<void>
```

In `apps/web/src/features/auth/auth-provider.tsx`, change the `login` callback credential type to match the new context type. Do not transform the fields; pass them directly to `loginRequest`.

Update the context value:

```ts
const value = useMemo(
  () => ({
    apiBaseUrl,
    isAuthenticated: Boolean(user),
    isLoading,
    login,
    logout,
    refreshCurrentUser,
    user,
  }),
  [apiBaseUrl, isLoading, login, logout, refreshCurrentUser, user]
)
```

- [ ] **Step 5: Update AuthProvider tests**

In `apps/web/src/features/auth/auth-provider.test.tsx`, update `LoginProbe` and `FailedLoginProbe` calls:

```tsx
void login({
  username: 'new',
  password: 'secret',
  captcha_id: '11111111-1111-4111-8111-111111111111',
  captcha_code: 'A7K2',
})
```

Update any expected body for `/api/auth/login` to include the captcha fields.

- [ ] **Step 6: Run Web auth tests**

```bash
corepack pnpm --filter ./apps/web run test -- src/lib/api/auth.test.ts src/features/auth/auth-provider.test.tsx
```

Expected: PASS.

- [ ] **Step 7: Commit Task 4**

```bash
git add apps/web/src/lib/api/auth.ts apps/web/src/lib/api/auth.test.ts apps/web/src/features/auth/auth-context.tsx apps/web/src/features/auth/auth-provider.tsx apps/web/src/features/auth/auth-provider.test.tsx
git commit -m "feat(web): pass captcha through auth client"
```

Expected: commit contains only Web auth client/provider changes.

## Task 5: Login Form UI And Behavior

**Files:**
- Modify: `apps/web/src/features/auth/sign-in/components/user-auth-form.tsx`
- Test: `apps/web/src/features/auth/sign-in/components/user-auth-form.test.tsx`

- [ ] **Step 1: Add failing form tests**

In `apps/web/src/features/auth/sign-in/components/user-auth-form.test.tsx`, mock the API client:

```ts
const getLoginCaptcha = vi.fn()

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    getLoginCaptcha: (...args: unknown[]) => getLoginCaptcha(...args),
  }
})
```

Update the existing `useAuth` mock:

```ts
vi.mock('@/features/auth/use-auth', () => ({
  useAuth: () => ({
    apiBaseUrl: 'http://control-plane.local',
    login,
  }),
}))
```

In `beforeEach`, add:

```ts
getLoginCaptcha.mockResolvedValue({
  captcha_id: '11111111-1111-4111-8111-111111111111',
  image_data_url: 'data:image/png;base64,abc',
  expires_at: '2026-06-30T10:00:00Z',
})
```

Add tests:

```tsx
it('loads and renders the login captcha', async () => {
  const screen = await render(<UserAuthForm />)

  await expect.element(screen.getByLabelText(/^图形验证码$/i)).toBeVisible()
  await expect.element(screen.getByRole('img', { name: '图形验证码' })).toHaveAttribute(
    'src',
    'data:image/png;base64,abc',
  )
  await expect.element(screen.getByRole('button', { name: '刷新验证码' })).toBeVisible()
})

it('submits login with captcha fields', async () => {
  const screen = await render(<UserAuthForm />)

  await userEvent.fill(screen.getByRole('textbox', { name: /^账号$/i }), 'admin')
  await userEvent.fill(screen.getByLabelText(/^密码$/i), 'admin')
  await userEvent.fill(screen.getByLabelText(/^图形验证码$/i), 'a7k2')
  await userEvent.click(screen.getByRole('button', { name: /^登录$/i }))

  await vi.waitFor(() =>
    expect(login).toHaveBeenCalledWith({
      username: 'admin',
      password: 'admin',
      captcha_id: '11111111-1111-4111-8111-111111111111',
      captcha_code: 'A7K2',
    }),
  )
})

it('refreshes captcha after login failure without clearing username or password', async () => {
  login.mockRejectedValueOnce(new Error('captcha invalid'))
  getLoginCaptcha
    .mockResolvedValueOnce({
      captcha_id: '11111111-1111-4111-8111-111111111111',
      image_data_url: 'data:image/png;base64,abc',
      expires_at: '2026-06-30T10:00:00Z',
    })
    .mockResolvedValueOnce({
      captcha_id: '22222222-2222-4222-8222-222222222222',
      image_data_url: 'data:image/png;base64,def',
      expires_at: '2026-06-30T10:05:00Z',
    })
  const screen = await render(<UserAuthForm />)

  await userEvent.fill(screen.getByRole('textbox', { name: /^账号$/i }), 'admin')
  await userEvent.fill(screen.getByLabelText(/^密码$/i), 'wrong')
  await userEvent.fill(screen.getByLabelText(/^图形验证码$/i), 'A7K2')
  await userEvent.click(screen.getByRole('button', { name: /^登录$/i }))

  await expect.element(screen.getByText('用户名或密码不正确')).toBeVisible()
  await expect.element(screen.getByRole('textbox', { name: /^账号$/i })).toHaveValue('admin')
  await expect.element(screen.getByLabelText(/^密码$/i)).toHaveValue('wrong')
  await expect.element(screen.getByLabelText(/^图形验证码$/i)).toHaveValue('')
  await expect.element(screen.getByRole('img', { name: '图形验证码' })).toHaveAttribute(
    'src',
    'data:image/png;base64,def',
  )
})
```

- [ ] **Step 2: Run form tests to verify they fail**

```bash
corepack pnpm --filter ./apps/web run test -- src/features/auth/sign-in/components/user-auth-form.test.tsx
```

Expected: FAIL because `UserAuthForm` does not load or render captcha.

- [ ] **Step 3: Update UserAuthForm schema and state**

In `apps/web/src/features/auth/sign-in/components/user-auth-form.tsx`, import:

```ts
import { useCallback, useEffect, useRef, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import { getLoginCaptcha, type CaptchaChallengeResponse } from '@/lib/api'
```

Replace the existing React import with the `useCallback/useEffect/useRef/useState` import above.

Update schema:

```ts
const formSchema = z.object({
  username: z.string().min(1, '请输入用户名。'),
  password: z.string().min(1, '请输入密码。'),
  captcha_code: z
    .string()
    .min(1, '请输入图形验证码。')
    .length(4, '请输入 4 位图形验证码。'),
})
```

Add state:

```ts
const { apiBaseUrl, login } = useAuth()
const [captcha, setCaptcha] = useState<CaptchaChallengeResponse | null>(null)
const [isCaptchaLoading, setIsCaptchaLoading] = useState(false)
const [captchaError, setCaptchaError] = useState<string | null>(null)
```

Add `captcha_code: ''` to form defaults.

- [ ] **Step 4: Add captcha loading and refresh function**

Add inside component after `form` is created:

```ts
const refreshCaptcha = useCallback(async (options?: { clearInput?: boolean }) => {
  setIsCaptchaLoading(true)
  setCaptchaError(null)
  try {
    const response = await getLoginCaptcha({ baseUrl: apiBaseUrl })
    setCaptcha(response)
    if (options?.clearInput ?? true) {
      form.setValue('captcha_code', '')
    }
  } catch {
    setCaptcha(null)
    setCaptchaError('验证码加载失败，请刷新重试')
  } finally {
    setIsCaptchaLoading(false)
  }
}, [apiBaseUrl, form])
```

Add initial load:

```ts
useEffect(() => {
  void refreshCaptcha({ clearInput: false })
}, [refreshCaptcha])
```

- [ ] **Step 5: Submit captcha fields and refresh after failure**

Update login call:

```ts
if (!captcha) {
  setFormError('验证码加载失败，请刷新重试')
  return
}
await login({
  username: data.username,
  password: data.password,
  captcha_id: captcha.captcha_id,
  captcha_code: data.captcha_code.toUpperCase(),
})
```

In the `catch` block:

```ts
setFormError('用户名或密码不正确')
void refreshCaptcha({ clearInput: true })
```

Disable submit when `isLoading || isCaptchaLoading || !captcha`.

- [ ] **Step 6: Render captcha controls**

Add after password field:

```tsx
<FormField
  control={form.control}
  name='captcha_code'
  render={({ field }) => (
    <FormItem>
      <FormLabel className='text-v3-ink-2'>图形验证码</FormLabel>
      <div className='flex gap-2'>
        <FormControl>
          <Input
            className='h-12 flex-1 rounded-xl border-v3-line-strong bg-v3-card-soft px-4 text-v3-ink shadow-none placeholder:text-v3-ink-3 focus-visible:border-v3-brand focus-visible:ring-v3-brand/20'
            maxLength={4}
            placeholder='请输入验证码'
            value={field.value}
            onChange={(event) => field.onChange(event.target.value.toUpperCase())}
          />
        </FormControl>
        <div className='flex h-12 items-center gap-2'>
          {captcha ? (
            <img
              src={captcha.image_data_url}
              alt='图形验证码'
              className='h-12 w-32 rounded-xl border border-v3-line-strong bg-v3-card-soft object-contain'
            />
          ) : (
            <div className='flex h-12 w-32 items-center justify-center rounded-xl border border-v3-line-strong bg-v3-card-soft text-xs text-v3-ink-3'>
              加载失败
            </div>
          )}
          <button
            type='button'
            aria-label='刷新验证码'
            className='inline-flex h-12 w-12 items-center justify-center rounded-xl border border-v3-line-strong bg-v3-card-soft text-v3-ink-2 hover:border-v3-brand hover:text-v3-brand'
            onClick={() => void refreshCaptcha({ clearInput: true })}
            disabled={isCaptchaLoading}
          >
            <RefreshCw className={cn('size-4', isCaptchaLoading ? 'animate-spin' : '')} />
          </button>
        </div>
      </div>
      <FormMessage />
    </FormItem>
  )}
/>
{captchaError ? (
  <p className='text-sm font-bold text-v3-danger' role='alert'>
    {captchaError}
  </p>
) : null}
```

- [ ] **Step 7: Run form tests**

```bash
corepack pnpm --filter ./apps/web run test -- src/features/auth/sign-in/components/user-auth-form.test.tsx
```

Expected: PASS.

- [ ] **Step 8: Commit Task 5**

```bash
git add apps/web/src/features/auth/sign-in/components/user-auth-form.tsx apps/web/src/features/auth/sign-in/components/user-auth-form.test.tsx
git commit -m "feat(web): add login captcha form"
```

Expected: commit contains only login form changes and tests.

## Task 6: Verification, Real Smoke, And Cleanup

**Files:**
- Modify: `CHANGELOG.md`
- No code creation unless verification exposes a defect.

- [ ] **Step 1: Run targeted test suite**

```bash
go test ./apps/control-plane/internal/auth ./apps/control-plane/internal/storage -run 'Test.*Captcha|TestLogin|TestAuthCaptchaChallenge' -count=1
corepack pnpm --filter ./apps/web run test -- src/lib/api/auth.test.ts src/features/auth/auth-provider.test.tsx src/features/auth/sign-in/components/user-auth-form.test.tsx
corepack pnpm verify:contracts
```

Expected: all commands PASS.

- [ ] **Step 2: Run broader compile checks**

```bash
go test ./apps/control-plane/...
corepack pnpm --filter ./apps/web run test
corepack pnpm --filter ./apps/web run typecheck
```

Expected: PASS. For a failure that is not caused by this feature, capture the exact failing test and reproduce it on the pre-feature baseline before excluding it from this feature's completion gate.

- [ ] **Step 3: Apply migration to intended local database**

Check services:

```bash
scripts/dev-services.sh status
```

Apply migration only against the intended SuperTeam development database:

```bash
DATABASE_URL="$(yq -r '.postgres.url' apps/control-plane/config/config.yaml)" make -C apps/control-plane migrate-status
DATABASE_URL="$(yq -r '.postgres.url' apps/control-plane/config/config.yaml)" make -C apps/control-plane migrate-up
```

Expected: migration status shows pending `039_auth_captcha_challenges.sql` before apply and clean/applied after apply. When `yq` is unavailable, read the URL from `apps/control-plane/config/config.yaml` without printing secrets into the final report.

- [ ] **Step 4: Restart current services**

```bash
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh restart web
scripts/dev-services.sh status
```

Expected: Control Plane and Web are running current branch code.

- [ ] **Step 5: Curl smoke captcha and login failures**

Fetch captcha:

```bash
CAPTCHA_JSON="$(curl -sS http://127.0.0.1:8081/api/auth/captcha)"
printf '%s\n' "$CAPTCHA_JSON"
CAPTCHA_ID="$(printf '%s' "$CAPTCHA_JSON" | node -e "let data='';process.stdin.on('data',c=>data+=c);process.stdin.on('end',()=>process.stdout.write(JSON.parse(data).captcha_id))")"
```

Expected: JSON includes `captcha_id`, `image_data_url` starting with `data:image/png;base64,`, and `expires_at`; `CAPTCHA_ID` contains the returned UUID.

Submit missing captcha:

```bash
curl -i -sS -X POST http://127.0.0.1:8081/api/auth/login \
  -H 'content-type: application/json' \
  --data '{"username":"admin","password":"admin"}'
```

Expected: HTTP `400`.

Submit invalid captcha with a real `captcha_id` from the previous response:

```bash
curl -i -sS -X POST http://127.0.0.1:8081/api/auth/login \
  -H 'content-type: application/json' \
  --data "{\"username\":\"admin\",\"password\":\"admin\",\"captcha_id\":\"${CAPTCHA_ID}\",\"captcha_code\":\"ZZZZ\"}"
```

Expected: HTTP `401` with “验证码不正确或已过期”.

- [ ] **Step 6: Browser smoke real login page**

Use Chrome plug or browser automation against the real Web app:

1. Open `http://127.0.0.1:3000/login`.
2. Confirm captcha image is visible.
3. Submit empty captcha and confirm client-side validation.
4. Submit wrong captcha and confirm captcha refresh.
5. Submit wrong password with a fresh captcha and confirm password error plus captcha refresh.
6. Submit valid `admin/admin` with a fresh captcha and confirm navigation to an authenticated route.

Expected: real Web talks to real Control Plane and no stale mock result is used.

- [ ] **Step 7: Verify login logs**

Use existing login log API or a read-only DB query to confirm captcha failure audit:

```sql
SELECT event_type, username, result, failure_reason, created_at
FROM web_login_logs
WHERE failure_reason IN ('captcha_invalid', 'captcha_expired')
ORDER BY created_at DESC
LIMIT 5;
```

Expected: at least one recent `login_failed` row with `failure_reason = 'captcha_invalid'` from the smoke test.

- [ ] **Step 8: Add changelog entry**

Get the current Shanghai timestamp and generate the changelog bullet:

```bash
CHANGELOG_TS="$(TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M')"
printf -- "- %s：登录页新增强制图形验证码：Control Plane 生成 4 位数字+字母图片验证码并以 PostgreSQL 一次性 challenge 校验，登录请求必须提交 \`captcha_id\`/\`captcha_code\`，失败写入登录日志；Web 登录表单加载、刷新并在失败后清空验证码。验证：更新为本任务实际通过的测试命令与真实登录链路结果。\n" "$CHANGELOG_TS"
```

Add the printed bullet under `## [Unreleased]` in `CHANGELOG.md`. Replace the verification sentence with the exact commands and real smoke results completed in this task.

```markdown
- 2026-06-30 18:30：登录页新增强制图形验证码：Control Plane 生成 4 位数字+字母图片验证码并以 PostgreSQL 一次性 challenge 校验，登录请求必须提交 `captcha_id`/`captcha_code`，失败写入登录日志；Web 登录表单加载、刷新并在失败后清空验证码。验证：`go test ./apps/control-plane/internal/auth ./apps/control-plane/internal/storage -run 'Test.*Captcha|TestLogin|TestAuthCaptchaChallenge' -count=1`、`corepack pnpm --filter ./apps/web run test -- src/lib/api/auth.test.ts src/features/auth/auth-provider.test.tsx src/features/auth/sign-in/components/user-auth-form.test.tsx`、`corepack pnpm verify:contracts` 通过；真实 Web 登录页加载验证码、错误验证码返回 401 并刷新、`admin/admin` 携带新验证码登录成功，登录日志写入 `captcha_invalid`。
```

Expected: changelog entry uses the actual timestamp from the command and names the real verification evidence from this task.

- [ ] **Step 9: Commit changelog**

```bash
git add CHANGELOG.md
git commit -m "docs: record login captcha release note"
```

Expected: commit contains only `CHANGELOG.md`.

- [ ] **Step 10: Run completion gate and hygiene**

Use `superteam-completion-check`.

Run:

```bash
git diff --check
git status --short
```

Expected: no whitespace errors. Status only includes intentional files for this feature and unrelated pre-existing dirty files remain unstaged.

- [ ] **Step 11: Handle verification-driven fixes**

When Task 6 exposes a defect, return to the task that owns the defective file, patch that task's implementation/test file list, rerun that task's targeted test command, then commit using that task's commit command. When Task 6 produces no file changes beyond `CHANGELOG.md`, no extra commit is needed.

Expected: the branch ends with no unstaged captcha feature files. Verification-driven code fixes are committed with their owning task area; pure verification with no code fix has no extra commit after the changelog commit.

## Self-Review Checklist

- Spec coverage: all requirements from `docs/superpowers/specs/2026-06-30-login-captcha-design.md` map to tasks:
  - mandatory captcha: Tasks 3, 5, 6
  - 4-character digit+letter code: Task 1
  - PostgreSQL challenge storage: Task 2
  - answer not stored as plaintext: Task 1 and Task 2
  - one-time and 5-minute expiry: Tasks 1, 2, 3
  - Web load/refresh/failure behavior: Tasks 4, 5
  - login-log audit: Tasks 1, 3, 6
  - real-chain verification: Task 6
- Placeholder scan: no `TBD`, `TODO`, or unresolved implementation markers.
- Type consistency: captcha request fields are consistently `captcha_id` and `captcha_code`; response fields are consistently `captcha_id`, `image_data_url`, and `expires_at`.
