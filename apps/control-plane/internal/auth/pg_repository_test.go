package auth

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/superteam/control-plane/internal/storage/queries"
)

func TestToDomainUserDefaultsMissingAvatarSeed(t *testing.T) {
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}

	user := toDomainUser(queries.AuthUser{
		ID:             uuid.New(),
		Username:       "legacy-admin",
		PasswordHash:   "hash",
		Status:         UserStatusActive,
		AvatarProvider: "dicebear",
		AvatarStyle:    "adventurer",
		AvatarOptions:  []byte(`{}`),
		CreatedAt:      now,
		UpdatedAt:      now,
	})

	if user.Avatar.Seed != "user:legacy-admin" {
		t.Fatalf("expected legacy user avatar seed fallback, got %#v", user.Avatar)
	}
}

func TestPgRepositoryValidatesActiveUsersAndTenantTeams(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping real PostgreSQL auth repository validation test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin test transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	tenantID := uuid.New()
	userID := uuid.New()
	disabledUserID := uuid.New()
	activeTeamID := uuid.New()
	disabledTeamID := uuid.New()

	if _, err := tx.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, status)
		VALUES ($1, $2, 'Auth Repo Validation Tenant', 'active')
	`, tenantID, "auth-repo-validation-"+tenantID.String()[:8]); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO auth_users (id, username, password_hash, status)
		VALUES ($1, $2, 'hash', 'active'), ($3, $4, 'hash', 'disabled')
	`, userID, "auth-repo-user-"+userID.String()[:8], disabledUserID, "auth-repo-disabled-"+disabledUserID.String()[:8]); err != nil {
		t.Fatalf("seed users: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO tenant_teams (id, tenant_id, slug, name, status)
		VALUES ($1, $2, $3, 'Active Team', 'active'), ($4, $2, $5, 'Disabled Team', 'disabled')
	`, activeTeamID, tenantID, "active-"+activeTeamID.String()[:8], disabledTeamID, "disabled-"+disabledTeamID.String()[:8]); err != nil {
		t.Fatalf("seed teams: %v", err)
	}

	repo := NewPgRepository(queries.New(tx))
	if err := repo.EnsureActiveUser(ctx, userID); err != nil {
		t.Fatalf("expected active user to validate: %v", err)
	}
	if err := repo.EnsureActiveUser(ctx, disabledUserID); !errors.Is(err, ErrManagedUserNotFound) {
		t.Fatalf("expected disabled user to be rejected, got %v", err)
	}
	if err := repo.ValidateActiveTenantTeamIDs(ctx, tenantID, []uuid.UUID{activeTeamID}); err != nil {
		t.Fatalf("expected active tenant team to validate: %v", err)
	}
	if err := repo.ValidateActiveTenantTeamIDs(ctx, tenantID, []uuid.UUID{activeTeamID, disabledTeamID}); !errors.Is(err, ErrInvalidManagedUserInput) {
		t.Fatalf("expected disabled tenant team to be rejected, got %v", err)
	}
}

func TestPgRepositoryWithTransactionRollsBackProjectTeamScopes(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping real PostgreSQL auth transaction rollback test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()

	cleanupTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin fixture transaction: %v", err)
	}
	defer cleanupTx.Rollback(ctx)

	tenantID := uuid.New()
	userID := uuid.New()
	grantedByUserID := uuid.New()
	teamID := uuid.New()

	if _, err := cleanupTx.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, status)
		VALUES ($1, $2, 'Auth Repo Rollback Tenant', 'active')
	`, tenantID, "auth-repo-rollback-"+tenantID.String()[:8]); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if _, err := cleanupTx.Exec(ctx, `
		INSERT INTO auth_users (id, username, password_hash, status)
		VALUES ($1, $2, 'hash', 'active'), ($3, $4, 'hash', 'active')
	`, userID, "auth-repo-rollback-user-"+userID.String()[:8], grantedByUserID, "auth-repo-rollback-grantor-"+grantedByUserID.String()[:8]); err != nil {
		t.Fatalf("seed users: %v", err)
	}
	if _, err := cleanupTx.Exec(ctx, `
		INSERT INTO tenant_teams (id, tenant_id, slug, name, status)
		VALUES ($1, $2, $3, 'Rollback Team', 'active')
	`, teamID, tenantID, "rollback-"+teamID.String()[:8]); err != nil {
		t.Fatalf("seed team: %v", err)
	}
	if err := cleanupTx.Commit(ctx); err != nil {
		t.Fatalf("commit fixtures: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM user_project_team_scopes WHERE tenant_id = $1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM tenant_teams WHERE tenant_id = $1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM auth_users WHERE id = ANY($1::uuid[])`, []uuid.UUID{userID, grantedByUserID})
		_, _ = pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
	}()

	repo := NewPgRepository(queries.New(pool), pool)
	forcedErr := errors.New("force rollback")
	err = repo.WithTransaction(ctx, func(txRepo Repository) error {
		if _, err := txRepo.ReplaceUserProjectTeamScopes(ctx, tenantID, userID, grantedByUserID, []uuid.UUID{teamID}); err != nil {
			return err
		}
		return forcedErr
	})
	if !errors.Is(err, forcedErr) {
		t.Fatalf("expected forced rollback error, got %v", err)
	}

	scopes, err := repo.ListUserProjectTeamScopes(ctx, tenantID, userID)
	if err != nil {
		t.Fatalf("list scopes after rollback: %v", err)
	}
	if len(scopes) != 0 {
		t.Fatalf("expected transaction rollback to leave no scopes, got %#v", scopes)
	}
}
