package queries_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/superteam/control-plane/internal/storage/queries"
)

var (
	testDB      *pgxpool.Pool
	testQueries *queries.Queries
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	cfg, ok := testConfig()
	if !ok {
		fmt.Fprintln(os.Stderr, "skipping storage query integration tests: set TEST_DATABASE_URL and TEST_REDIS_URL, or set ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 with DATABASE_URL and REDIS_URL")
		os.Exit(0)
	}

	if err := pingRedis(ctx, cfg.redisURL); err != nil {
		panic(err)
	}

	// 连接数据库
	var err error
	testDB, err = pgxpool.New(ctx, cfg.databaseURL)
	if err != nil {
		panic(err)
	}
	if err := testDB.Ping(ctx); err != nil {
		testDB.Close()
		panic(err)
	}

	// 运行迁移
	if err := runMigrations(ctx, testDB); err != nil {
		panic(err)
	}

	// 创建 queries 实例
	testQueries = queries.New(testDB)

	// 运行测试
	code := m.Run()

	// 清理
	testDB.Close()

	os.Exit(code)
}

type integrationTestConfig struct {
	databaseURL string
	redisURL    string
}

func testConfig() (integrationTestConfig, bool) {
	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	redisURL := strings.TrimSpace(os.Getenv("TEST_REDIS_URL"))
	if envBool("ALLOW_DATABASE_URL_FOR_QUERY_TESTS") {
		if databaseURL == "" {
			databaseURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
		}
		if redisURL == "" {
			redisURL = strings.TrimSpace(os.Getenv("REDIS_URL"))
		}
	}
	if databaseURL == "" || redisURL == "" {
		return integrationTestConfig{}, false
	}

	return integrationTestConfig{
		databaseURL: databaseURL,
		redisURL:    redisURL,
	}, true
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func pingRedis(ctx context.Context, redisURL string) error {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return err
	}

	client := redis.NewClient(options)
	defer client.Close()

	return client.Ping(ctx).Err()
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	// 获取连接字符串
	connString := pool.Config().ConnString()

	// 使用单独的连接执行迁移
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	runtimeEventsMigrated, err := schemaHasTable(ctx, conn, "runtime_events")
	if err != nil {
		return err
	}
	if runtimeEventsMigrated {
		return nil
	}

	runLoopMigrated, err := schemaHasTable(ctx, conn, "runtime_command_receipts")
	if err != nil {
		return err
	}

	baseMigrated, err := schemaHasTable(ctx, conn, "runtime_nodes")
	if err != nil {
		return err
	}

	// 发现所有迁移文件
	migrationsDir := filepath.Join("..", "migrations")
	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil {
		return err
	}

	// 按文件名排序
	sort.Strings(files)
	files = migrationFilesForSchemaState(files, migrationSchemaState{
		baseMigrated:          baseMigrated,
		runLoopMigrated:       runLoopMigrated,
		runtimeEventsMigrated: runtimeEventsMigrated,
	})

	// 执行每个迁移文件
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return err
		}

		if _, err := conn.Exec(ctx, string(content)); err != nil {
			return err
		}
	}

	return nil
}

type migrationSchemaState struct {
	baseMigrated          bool
	runLoopMigrated       bool
	runtimeEventsMigrated bool
}

func migrationFilesForSchemaState(files []string, state migrationSchemaState) []string {
	if state.runtimeEventsMigrated {
		return nil
	}
	if state.runLoopMigrated {
		return migrationFilesFrom(files, "007_runtime_events_overview.sql")
	}
	if !state.baseMigrated {
		return files
	}

	return migrationFilesFrom(files, "003_add_team_governance_config.sql")
}

func migrationFilesFrom(files []string, firstMigration string) []string {
	filtered := make([]string, 0, len(files))
	for _, file := range files {
		base := filepath.Base(file)
		if base >= firstMigration {
			filtered = append(filtered, file)
		}
	}
	return filtered
}

func TestMigrationFilesForSchemaStateAppliesRuntimeEventsAfterRunLoop(t *testing.T) {
	files := []string{
		filepath.Join("..", "migrations", "001_initial.sql"),
		filepath.Join("..", "migrations", "003_add_team_governance_config.sql"),
		filepath.Join("..", "migrations", "006_digital_employee_run_loop.sql"),
		filepath.Join("..", "migrations", "007_runtime_events_overview.sql"),
	}

	selected := migrationFilesForSchemaState(files, migrationSchemaState{
		baseMigrated:          true,
		runLoopMigrated:       true,
		runtimeEventsMigrated: false,
	})

	require.Equal(t, []string{filepath.Join("..", "migrations", "007_runtime_events_overview.sql")}, selected)
}

func schemaHasTable(ctx context.Context, conn *pgx.Conn, tableName string) (bool, error) {
	const query = `
SELECT EXISTS (
	SELECT 1
	FROM information_schema.tables
	WHERE table_schema = current_schema()
		AND table_name = $1
)
`

	var exists bool
	if err := conn.QueryRow(ctx, query, tableName).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// cleanupTestData 清理测试数据，避免测试之间相互影响
func cleanupTestData(t *testing.T, db *pgxpool.Pool) {
	ctx := context.Background()
	_, err := db.Exec(ctx, `
		TRUNCATE
			runtime_events,
			runtime_command_receipts,
			provider_session_events,
			provider_sessions,
			digital_employee_execution_instances,
			digital_employee_effective_configs,
			digital_employee_config_revisions,
			digital_employees,
			runtime_capabilities,
			runtime_sessions,
			runtime_enrollments,
			runtime_bootstrap_keys,
			web_operation_logs,
			web_login_logs,
			audit_events,
			tenant_team_member_role_requests,
			task_artifacts,
			task_events,
			task_state_history,
			runtime_leases,
			task_runs,
			tasks,
			auth_sessions,
			auth_runtime_tokens,
			runtime_node_scopes,
			runtime_nodes,
			tenant_members,
			auth_users,
			tenant_team_config_revisions,
			tenant_teams,
			tenant_profiles,
			tenants
		RESTART IDENTITY CASCADE
	`)
	require.NoError(t, err)
	seedDefaultTenant(t, db)
}

func seedDefaultTenant(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO tenants (id, slug, name, status)
		VALUES ('00000000-0000-0000-0000-000000000001'::uuid, 'default', '默认租户', 'active')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO tenant_teams (id, tenant_id, slug, name, status)
		VALUES (
			'00000000-0000-0000-0000-000000000101'::uuid,
			'00000000-0000-0000-0000-000000000001'::uuid,
			'default',
			'默认团队',
			'active'
		)
		ON CONFLICT (id) DO NOTHING;
	`)
	require.NoError(t, err)
}

func seedTestTenant(t *testing.T, db *pgxpool.Pool) uuid.UUID {
	t.Helper()
	tenantID := uuid.New()
	_, err := db.Exec(context.Background(), `
		INSERT INTO tenants (id, slug, name, status)
		VALUES ($1, $2, '测试租户', 'active')
	`, tenantID, fmt.Sprintf("tenant-%s", tenantID.String()))
	require.NoError(t, err)
	return tenantID
}

func seedTestTeam(t *testing.T, db *pgxpool.Pool, tenantID uuid.UUID, slug, name string) uuid.UUID {
	t.Helper()
	teamID := uuid.New()
	_, err := db.Exec(context.Background(), `
		INSERT INTO tenant_teams (id, tenant_id, slug, name, status, metadata)
		VALUES ($1, $2, $3, $4, 'active', '{}'::jsonb)
	`, teamID, tenantID, slug, name)
	require.NoError(t, err)
	return teamID
}

func seedTestAuthUser(t *testing.T, db *pgxpool.Pool, username string) uuid.UUID {
	t.Helper()
	user, err := queries.New(db).CreateUser(context.Background(), queries.CreateUserParams{
		Username:     username,
		DisplayName:  pgtype.Text{String: username, Valid: true},
		Email:        pgtype.Text{String: fmt.Sprintf("%s@example.com", username), Valid: true},
		PasswordHash: "$2a$10$hashedpassword",
		Status:       "active",
	})
	require.NoError(t, err)
	return user.ID
}

func seedTestTeamConfigRevision(t *testing.T, db *pgxpool.Pool, tenantID, teamID uuid.UUID, status string, revisionNumber int32, ownerID uuid.NullUUID) queries.TenantTeamConfigRevision {
	t.Helper()
	revision, err := queries.New(db).CreateTenantTeamConfigRevision(context.Background(), queries.CreateTenantTeamConfigRevisionParams{
		TenantID:                    tenantID,
		TeamID:                      teamID,
		RevisionNumber:              revisionNumber,
		Constitution:                []byte(`{}`),
		CapabilityPolicy:            []byte(`{}`),
		ContextPolicy:               []byte(`{}`),
		ApprovalPolicy:              []byte(`{}`),
		ArtifactContract:            []byte(`{}`),
		InternalCollaborationPolicy: []byte(`{}`),
		RuntimeScopePolicy:          []byte(`{}`),
		HumanOwnerUserID:            ownerID,
		Status:                      status,
	})
	require.NoError(t, err)
	return revision
}

func TestTeamConfigAndDigitalEmployeeEffectiveConfigQueries(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	owner, err := testQueries.CreateUser(ctx, queries.CreateUserParams{
		Username:     "ops-owner",
		DisplayName:  pgtype.Text{String: "Ops Owner", Valid: true},
		Email:        pgtype.Text{String: "ops-owner@example.com", Valid: true},
		PasswordHash: "$2a$10$hashedpassword",
		Status:       "active",
	})
	require.NoError(t, err)

	team, err := testQueries.CreateTenantTeam(ctx, queries.CreateTenantTeamParams{
		TenantID:         tenantID,
		Slug:             "ops",
		Name:             "运维团队",
		HumanOwnerUserID: uuid.NullUUID{UUID: owner.ID, Valid: true},
		Metadata:         []byte(`{"domain":"operations"}`),
	})
	require.NoError(t, err)

	teamConfig, err := testQueries.CreateTenantTeamConfigRevision(ctx, queries.CreateTenantTeamConfigRevisionParams{
		TenantID:                    tenantID,
		TeamID:                      team.ID,
		RevisionNumber:              1,
		Constitution:                []byte(`{"hard_rules":["禁止执行未审批的生产写操作"]}`),
		CapabilityPolicy:            []byte(`{"allowed_mcp_servers":["prometheus"],"allowed_skills":["incident-diagnosis"],"allowed_plugins":["log-viewer"],"allowed_provider_types":["codex"]}`),
		ContextPolicy:               []byte(`{"sources":["monitoring","logs"]}`),
		ApprovalPolicy:              []byte(`{"min_risk_for_human":"high","write_actions_require_human":true}`),
		ArtifactContract:            []byte(`{"required":["Finding","Risk","DecisionRequest"]}`),
		InternalCollaborationPolicy: []byte(`{"allowed_request_types":["info_request","review_request","artifact_request"],"max_auto_rounds":2,"max_auto_participants":3}`),
		RuntimeScopePolicy:          []byte(`{"allowed_provider_types":["codex"]}`),
		HumanOwnerUserID:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		Status:                      "active",
		ApprovedBy:                  uuid.NullUUID{UUID: owner.ID, Valid: true},
		ApprovedAt:                  pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	require.NoError(t, err)

	employee, err := testQueries.CreateDigitalEmployee(ctx, queries.CreateDigitalEmployeeParams{
		TenantID:  tenantID,
		TeamID:    uuid.NullUUID{UUID: team.ID, Valid: true},
		Name:      "数据库运维员工",
		Role:      "database_operator",
		Status:    "draft",
		RiskLevel: "medium",
	})
	require.NoError(t, err)

	employeeConfig, err := testQueries.CreateDigitalEmployeeConfigRevision(ctx, queries.CreateDigitalEmployeeConfigRevisionParams{
		TenantID:               tenantID,
		DigitalEmployeeID:      employee.ID,
		RevisionNumber:         1,
		RoleProfile:            []byte(`{"specialty":"postgres"}`),
		ConstitutionAddendum:   []byte(`{"required_output_rules":["输出慢查询证据"]}`),
		CapabilitySelection:    []byte(`{"enabled_mcp_servers":["prometheus"],"enabled_skills":["incident-diagnosis"],"enabled_plugins":["log-viewer"]}`),
		ContextPolicyOverride:  []byte(`{"sources":["monitoring"]}`),
		ApprovalPolicyOverride: []byte(`{"min_risk_for_human":"high"}`),
		OutputContractAddendum: []byte(`{"required":["SlowQueryFinding"]}`),
		Status:                 "active",
		ApprovedBy:             uuid.NullUUID{UUID: owner.ID, Valid: true},
		ApprovedAt:             pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	require.NoError(t, err)

	draftEmployeeConfig, err := testQueries.CreateDigitalEmployeeConfigRevision(ctx, queries.CreateDigitalEmployeeConfigRevisionParams{
		TenantID:               tenantID,
		DigitalEmployeeID:      employee.ID,
		RevisionNumber:         2,
		RoleProfile:            []byte(`{"specialty":"postgres","mode":"draft"}`),
		ConstitutionAddendum:   []byte(`{"required_output_rules":["输出执行计划证据"]}`),
		CapabilitySelection:    []byte(`{"enabled_mcp_servers":["prometheus"],"enabled_skills":["incident-diagnosis"]}`),
		ContextPolicyOverride:  []byte(`{"sources":["monitoring","traces"]}`),
		ApprovalPolicyOverride: []byte(`{"min_risk_for_human":"medium"}`),
		OutputContractAddendum: []byte(`{"required":["ExecutionPlanFinding"]}`),
		Status:                 "draft",
	})
	require.NoError(t, err)

	effective, err := testQueries.CreateDigitalEmployeeEffectiveConfig(ctx, queries.CreateDigitalEmployeeEffectiveConfigParams{
		TenantID:                   tenantID,
		DigitalEmployeeID:          employee.ID,
		TenantTeamConfigRevisionID: teamConfig.ID,
		EmployeeConfigRevisionID:   employeeConfig.ID,
		EffectiveConfigSnapshot:    []byte(`{"team":{"revision":1},"employee":{"revision":1}}`),
		ValidationResult:           []byte(`{"blocking_errors":[],"warnings":[]}`),
		Status:                     "approved",
		ApprovedBy:                 uuid.NullUUID{UUID: owner.ID, Valid: true},
		ApprovedAt:                 pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	require.NoError(t, err)
	require.Equal(t, employee.ID, effective.DigitalEmployeeID)

	pendingEffective, err := testQueries.CreateDigitalEmployeeEffectiveConfig(ctx, queries.CreateDigitalEmployeeEffectiveConfigParams{
		TenantID:                   tenantID,
		DigitalEmployeeID:          employee.ID,
		TenantTeamConfigRevisionID: teamConfig.ID,
		EmployeeConfigRevisionID:   draftEmployeeConfig.ID,
		EffectiveConfigSnapshot:    []byte(`{"team":{"revision":1},"employee":{"revision":2}}`),
		ValidationResult:           []byte(`{"blocking_errors":[],"warnings":["等待审批"]}`),
		Status:                     "pending_approval",
	})
	require.NoError(t, err)
	require.Equal(t, employee.ID, pendingEffective.DigitalEmployeeID)

	current, err := testQueries.GetCurrentTenantTeamConfigRevision(ctx, queries.GetCurrentTenantTeamConfigRevisionParams{
		TenantID: tenantID,
		TeamID:   team.ID,
	})
	require.NoError(t, err)
	require.Equal(t, teamConfig.ID, current.ID)

	currentEmployeeConfig, err := testQueries.GetCurrentDigitalEmployeeConfigRevision(ctx, queries.GetCurrentDigitalEmployeeConfigRevisionParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employee.ID,
	})
	require.NoError(t, err)
	require.Equal(t, employeeConfig.ID, currentEmployeeConfig.ID)

	currentEffective, err := testQueries.GetCurrentDigitalEmployeeEffectiveConfig(ctx, queries.GetCurrentDigitalEmployeeEffectiveConfigParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employee.ID,
	})
	require.NoError(t, err)
	require.Equal(t, effective.ID, currentEffective.ID)
}

func TestTeamGovernanceDraftLifecycleQueries(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)
	db := testDB
	q := queries.New(db)
	tenantID := seedTestTenant(t, db)
	teamID := seedTestTeam(t, db, tenantID, "ops", "运维团队")
	active := seedTestTeamConfigRevision(t, db, tenantID, teamID, "active", 1, uuid.NullUUID{})
	draftOwnerID := seedTestAuthUser(t, db, "draft-owner")
	draft := seedTestTeamConfigRevision(t, db, tenantID, teamID, "draft", 2, uuid.NullUUID{UUID: draftOwnerID, Valid: true})

	updated, err := q.UpdateTenantTeamConfigRevisionDraft(ctx, queries.UpdateTenantTeamConfigRevisionDraftParams{
		ID:               draft.ID,
		TenantID:         tenantID,
		TeamID:           teamID,
		Constitution:     []byte(`{"hard_rules":["禁止未审批生产写操作"]}`),
		CapabilityPolicy: []byte(`{"skill_bindings":["incident-diagnosis"]}`),
	})
	require.NoError(t, err)
	require.Equal(t, int32(2), updated.RevisionNumber)
	require.Equal(t, uuid.NullUUID{UUID: draftOwnerID, Valid: true}, updated.HumanOwnerUserID)
	require.JSONEq(t, `{"hard_rules":["禁止未审批生产写操作"]}`, string(updated.Constitution))
	require.JSONEq(t, `{"skill_bindings":["incident-diagnosis"]}`, string(updated.CapabilityPolicy))
	require.JSONEq(t, string(draft.ContextPolicy), string(updated.ContextPolicy))
	require.JSONEq(t, string(draft.ApprovalPolicy), string(updated.ApprovalPolicy))

	archived, err := q.ArchiveActiveTenantTeamConfigRevision(ctx, queries.ArchiveActiveTenantTeamConfigRevisionParams{
		TenantID: tenantID,
		TeamID:   teamID,
	})
	require.NoError(t, err)
	require.Len(t, archived, 1)
	require.Equal(t, active.ID, archived[0].ID)
	require.Equal(t, "archived", archived[0].Status)
	require.True(t, archived[0].ArchivedAt.Valid)

	approverID := seedTestAuthUser(t, db, "approver")
	approved, err := q.ActivateTenantTeamConfigRevision(ctx, queries.ActivateTenantTeamConfigRevisionParams{
		ID:         draft.ID,
		TenantID:   tenantID,
		TeamID:     teamID,
		ApprovedBy: approverID,
	})
	require.NoError(t, err)
	require.Equal(t, "active", approved.Status)
	require.Equal(t, approverID, approved.ApprovedBy.UUID)
	require.True(t, approved.ApprovedAt.Valid)

	rejectedDraft := seedTestTeamConfigRevision(t, db, tenantID, teamID, "draft", 3, uuid.NullUUID{UUID: draftOwnerID, Valid: true})
	rejected, err := q.RejectTenantTeamConfigRevision(ctx, queries.RejectTenantTeamConfigRevisionParams{
		ID:       rejectedDraft.ID,
		TenantID: tenantID,
		TeamID:   teamID,
	})
	require.NoError(t, err)
	require.Equal(t, "rejected", rejected.Status)
	require.True(t, rejected.ArchivedAt.Valid)

	fetchedRejected, err := q.GetTenantTeamConfigRevision(ctx, queries.GetTenantTeamConfigRevisionParams{
		ID:       rejectedDraft.ID,
		TenantID: tenantID,
	})
	require.NoError(t, err)
	require.Equal(t, rejectedDraft.ID, fetchedRejected.ID)
	require.Equal(t, "rejected", fetchedRejected.Status)
	require.True(t, fetchedRejected.ArchivedAt.Valid)
}

func TestTeamGovernanceDraftApprovalTransactionRollbackQueries(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)
	db := testDB
	q := queries.New(db)
	tenantID := seedTestTenant(t, db)
	teamID := seedTestTeam(t, db, tenantID, "rollback-ops", "回滚运维团队")
	active := seedTestTeamConfigRevision(t, db, tenantID, teamID, "active", 1, uuid.NullUUID{})
	draftOwnerID := seedTestAuthUser(t, db, "rollback-draft-owner")
	_ = seedTestTeamConfigRevision(t, db, tenantID, teamID, "draft", 2, uuid.NullUUID{UUID: draftOwnerID, Valid: true})

	tx, err := db.Begin(ctx)
	require.NoError(t, err)
	qtx := q.WithTx(tx)

	archived, err := qtx.ArchiveActiveTenantTeamConfigRevision(ctx, queries.ArchiveActiveTenantTeamConfigRevisionParams{
		TenantID: tenantID,
		TeamID:   teamID,
	})
	require.NoError(t, err)
	require.Len(t, archived, 1)
	require.Equal(t, "archived", archived[0].Status)

	approverID := seedTestAuthUser(t, db, "rollback-approver")
	_, err = qtx.ActivateTenantTeamConfigRevision(ctx, queries.ActivateTenantTeamConfigRevisionParams{
		ID:         uuid.New(),
		TenantID:   tenantID,
		TeamID:     teamID,
		ApprovedBy: approverID,
	})
	require.ErrorIs(t, err, pgx.ErrNoRows)
	require.NoError(t, tx.Rollback(ctx))

	current, err := q.GetCurrentTenantTeamConfigRevision(ctx, queries.GetCurrentTenantTeamConfigRevisionParams{
		TenantID: tenantID,
		TeamID:   teamID,
	})
	require.NoError(t, err)
	require.Equal(t, active.ID, current.ID)
	require.Equal(t, "active", current.Status)
	require.False(t, current.ArchivedAt.Valid)
}

// ============================================================================
// Authz Tests
// ============================================================================

func TestAuthzTenantMembershipQueries(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	user, err := testQueries.CreateUser(ctx, queries.CreateUserParams{
		Username:     "authz-owner",
		DisplayName:  pgtype.Text{String: "Authz Owner", Valid: true},
		Email:        pgtype.Text{String: "authz-owner@example.com", Valid: true},
		PasswordHash: "$2a$10$hashedpassword",
		Status:       "active",
	})
	require.NoError(t, err)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	_, err = testDB.Exec(ctx, `
		INSERT INTO tenant_members (tenant_id, principal_type, principal_id, role, status)
		VALUES ($1, 'user', $2, 'owner', 'active')
	`, tenantID, user.ID)
	require.NoError(t, err)

	member, err := testQueries.GetActiveTenantMembership(ctx, queries.GetActiveTenantMembershipParams{
		TenantID:      tenantID,
		PrincipalType: "user",
		PrincipalID:   user.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, "owner", member.Role)
	assert.False(t, member.TeamID.Valid)

	inactiveUser, err := testQueries.CreateUser(ctx, queries.CreateUserParams{
		Username:     "authz-inactive-member",
		DisplayName:  pgtype.Text{String: "Authz Inactive Member", Valid: true},
		Email:        pgtype.Text{String: "authz-inactive-member@example.com", Valid: true},
		PasswordHash: "$2a$10$hashedpassword",
		Status:       "active",
	})
	require.NoError(t, err)
	_, err = testDB.Exec(ctx, `
		INSERT INTO tenant_members (tenant_id, principal_type, principal_id, role, status)
		VALUES ($1, 'user', $2, 'member', 'invited')
	`, tenantID, inactiveUser.ID)
	require.NoError(t, err)
	_, err = testQueries.GetActiveTenantMembership(ctx, queries.GetActiveTenantMembershipParams{
		TenantID:      tenantID,
		PrincipalType: "user",
		PrincipalID:   inactiveUser.ID,
	})
	require.ErrorIs(t, err, pgx.ErrNoRows)

	disabledUser, err := testQueries.CreateUser(ctx, queries.CreateUserParams{
		Username:     "authz-disabled-member",
		DisplayName:  pgtype.Text{String: "Authz Disabled Member", Valid: true},
		Email:        pgtype.Text{String: "authz-disabled-member@example.com", Valid: true},
		PasswordHash: "$2a$10$hashedpassword",
		Status:       "active",
	})
	require.NoError(t, err)
	_, err = testDB.Exec(ctx, `
		INSERT INTO tenant_members (tenant_id, principal_type, principal_id, role, status, disabled_at)
		VALUES ($1, 'user', $2, 'member', 'active', NOW())
	`, tenantID, disabledUser.ID)
	require.NoError(t, err)
	_, err = testQueries.GetActiveTenantMembership(ctx, queries.GetActiveTenantMembershipParams{
		TenantID:      tenantID,
		PrincipalType: "user",
		PrincipalID:   disabledUser.ID,
	})
	require.ErrorIs(t, err, pgx.ErrNoRows)
}

func TestAuthzRuntimeNodeCoversTaskScope(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")

	providersJSON, err := json.Marshal(map[string]interface{}{"providers": []string{"claude-code"}})
	require.NoError(t, err)
	metadataJSON, err := json.Marshal(map[string]interface{}{"test": true})
	require.NoError(t, err)
	now := pgtype.Timestamptz{}
	require.NoError(t, now.Scan(time.Now()))
	heartbeatAfter := pgtype.Timestamptz{}
	require.NoError(t, heartbeatAfter.Scan(time.Now().Add(-time.Minute)))

	node, err := testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "authz-node-001",
		Name:               "Authz Node 001",
		SupportedProviders: providersJSON,
		MaxSlots:           2,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           metadataJSON,
		LastHeartbeatAt:    now,
	})
	require.NoError(t, err)

	paramsJSON, err := json.Marshal(map[string]interface{}{"command": "authz"})
	require.NoError(t, err)
	task, err := testQueries.CreateTask(ctx, queries.CreateTaskParams{
		TenantID:     uuid.NullUUID{UUID: tenantID, Valid: true},
		TeamID:       uuid.NullUUID{UUID: teamID, Valid: true},
		Title:        "Authz Task",
		Description:  pgtype.Text{String: "Task for runtime scope authz", Valid: true},
		Status:       "pending",
		Priority:     1,
		ProviderType: "claude-code",
		Params:       paramsJSON,
	})
	require.NoError(t, err)

	_, err = testDB.Exec(ctx, `
		INSERT INTO runtime_node_scopes (tenant_id, runtime_node_id, team_id, scope_type, scope_value, status)
		VALUES ($1, $2, $3, 'team', $4, 'active')
	`, tenantID, node.ID, teamID, teamID.String())
	require.NoError(t, err)

	covered, err := testQueries.RuntimeNodeCoversTaskScope(ctx, queries.RuntimeNodeCoversTaskScopeParams{
		TenantID:           tenantID,
		TeamID:             uuid.NullUUID{UUID: teamID, Valid: true},
		TaskID:             task.ID,
		NodeID:             "authz-node-001",
		LastHeartbeatAfter: heartbeatAfter,
	})
	require.NoError(t, err)
	assert.True(t, covered)

	missingNodeCovered, err := testQueries.RuntimeNodeCoversTaskScope(ctx, queries.RuntimeNodeCoversTaskScopeParams{
		TenantID:           tenantID,
		TeamID:             uuid.NullUUID{UUID: teamID, Valid: true},
		TaskID:             task.ID,
		NodeID:             "authz-node-missing",
		LastHeartbeatAfter: heartbeatAfter,
	})
	require.NoError(t, err)
	assert.False(t, missingNodeCovered)

	nilTeamCovered, err := testQueries.RuntimeNodeCoversTaskScope(ctx, queries.RuntimeNodeCoversTaskScopeParams{
		TenantID:           tenantID,
		TeamID:             uuid.NullUUID{},
		TaskID:             task.ID,
		NodeID:             "authz-node-001",
		LastHeartbeatAfter: heartbeatAfter,
	})
	require.NoError(t, err)
	assert.False(t, nilTeamCovered)

	_, err = testQueries.UpdateRuntimeNodeStatus(ctx, queries.UpdateRuntimeNodeStatusParams{
		NodeID: "authz-node-001",
		Status: "offline",
	})
	require.NoError(t, err)

	offlineNodeCovered, err := testQueries.RuntimeNodeCoversTaskScope(ctx, queries.RuntimeNodeCoversTaskScopeParams{
		TenantID:           tenantID,
		TeamID:             uuid.NullUUID{UUID: teamID, Valid: true},
		TaskID:             task.ID,
		NodeID:             "authz-node-001",
		LastHeartbeatAfter: heartbeatAfter,
	})
	require.NoError(t, err)
	assert.False(t, offlineNodeCovered)

	_, err = testQueries.UpdateRuntimeNodeStatus(ctx, queries.UpdateRuntimeNodeStatusParams{
		NodeID: "authz-node-001",
		Status: "online",
	})
	require.NoError(t, err)
	staleHeartbeat := pgtype.Timestamptz{}
	require.NoError(t, staleHeartbeat.Scan(time.Now().Add(-2*time.Hour)))
	_, err = testQueries.UpdateRuntimeNodeHeartbeat(ctx, queries.UpdateRuntimeNodeHeartbeatParams{
		NodeID:          "authz-node-001",
		LastHeartbeatAt: staleHeartbeat,
	})
	require.NoError(t, err)

	staleNodeCovered, err := testQueries.RuntimeNodeCoversTaskScope(ctx, queries.RuntimeNodeCoversTaskScopeParams{
		TenantID:           tenantID,
		TeamID:             uuid.NullUUID{UUID: teamID, Valid: true},
		TaskID:             task.ID,
		NodeID:             "authz-node-001",
		LastHeartbeatAfter: heartbeatAfter,
	})
	require.NoError(t, err)
	assert.False(t, staleNodeCovered)
}

func TestRuntimeEventsOverviewQueries(t *testing.T) {
	if testQueries == nil {
		t.Skip("query integration tests require TEST_DATABASE_URL")
	}
	ctx := context.Background()
	cleanupTestData(t, testDB)
	tenantID := seedTestTenant(t, testDB)
	now := time.Now().UTC()

	node, err := testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "overview-node-001",
		Name:               "overview node",
		SupportedProviders: []byte(`["codex","claude-code"]`),
		MaxSlots:           8,
		CurrentLoad:        2,
		Status:             "online",
		Metadata:           []byte(`{"scope":"local"}`),
		LastHeartbeatAt:    pgtype.Timestamptz{Time: now, Valid: true},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = testQueries.DeleteRuntimeNode(ctx, "overview-node-001") })

	_, err = testDB.Exec(ctx, `UPDATE runtime_nodes SET tenant_id = $1 WHERE id = $2`, tenantID, node.ID)
	require.NoError(t, err)

	event, err := testQueries.CreateRuntimeEvent(ctx, queries.CreateRuntimeEventParams{
		TenantID:        tenantID,
		RuntimeNodeID:   uuid.NullUUID{UUID: node.ID, Valid: true},
		NodeID:          pgtype.Text{String: "overview-node-001", Valid: true},
		EventType:       "enrollment_approved",
		Severity:        "success",
		Source:          "runtime_enrollment",
		Title:           "Runtime 节点接入通过",
		Description:     pgtype.Text{String: "overview-node-001 已批准接入", Valid: true},
		ProviderType:    pgtype.Text{},
		CorrelationType: pgtype.Text{String: "runtime_enrollment", Valid: true},
		CorrelationID:   pgtype.Text{String: "enrollment-001", Valid: true},
		Payload:         []byte(`{"status":"approved"}`),
	})
	require.NoError(t, err)
	assert.Equal(t, "enrollment_approved", event.EventType)

	items, err := testQueries.ListRuntimeEvents(ctx, queries.ListRuntimeEventsParams{
		TenantID:     tenantID,
		EventType:    pgtype.Text{String: "enrollment_approved", Valid: true},
		Severity:     pgtype.Text{},
		NodeID:       pgtype.Text{},
		ProviderType: pgtype.Text{},
		Limit:        10,
		Offset:       0,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, event.ID, items[0].ID)

	totalNodes, err := testQueries.CountRuntimeNodesForTenant(ctx, tenantID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), totalNodes)

	onlineNodes, err := testQueries.CountOnlineRuntimeNodesForTenant(ctx, queries.CountOnlineRuntimeNodesForTenantParams{
		TenantID:        tenantID,
		LastHeartbeatAt: pgtype.Timestamptz{Time: now.Add(-time.Minute), Valid: true},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), onlineNodes)
}

func TestListTenantTeamSummariesReturnsGovernanceCounts(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	owner, err := testQueries.CreateUser(ctx, queries.CreateUserParams{
		Username:     "summary-owner",
		DisplayName:  pgtype.Text{String: "Summary Owner", Valid: true},
		Email:        pgtype.Text{String: "summary-owner@example.com", Valid: true},
		PasswordHash: "$2a$10$hashedpassword",
		Status:       "active",
	})
	require.NoError(t, err)

	member, err := testQueries.CreateUser(ctx, queries.CreateUserParams{
		Username:     "summary-member",
		DisplayName:  pgtype.Text{String: "Summary Member", Valid: true},
		Email:        pgtype.Text{String: "summary-member@example.com", Valid: true},
		PasswordHash: "$2a$10$hashedpassword",
		Status:       "active",
	})
	require.NoError(t, err)

	team, err := testQueries.CreateTenantTeam(ctx, queries.CreateTenantTeamParams{
		TenantID:         tenantID,
		Slug:             "ops-summary",
		Name:             "运维团队",
		HumanOwnerUserID: uuid.NullUUID{UUID: owner.ID, Valid: true},
		Metadata:         []byte(`{"domain":"operations"}`),
	})
	require.NoError(t, err)

	_, err = testDB.Exec(ctx, `
		INSERT INTO tenant_members (tenant_id, team_id, principal_type, principal_id, role, status)
		VALUES ($1, $2, 'user', $3, 'member', 'active')
	`, tenantID, team.ID, member.ID)
	require.NoError(t, err)

	_, err = testQueries.CreateDigitalEmployee(ctx, queries.CreateDigitalEmployeeParams{
		TenantID:  tenantID,
		TeamID:    uuid.NullUUID{UUID: team.ID, Valid: true},
		Name:      "数据库运维员工",
		Role:      "database_operator",
		Status:    "active",
		RiskLevel: "medium",
	})
	require.NoError(t, err)

	_, err = testQueries.CreateTenantTeamConfigRevision(ctx, queries.CreateTenantTeamConfigRevisionParams{
		TenantID:                    tenantID,
		TeamID:                      team.ID,
		RevisionNumber:              1,
		Constitution:                []byte(`{"hard_rules":["禁止执行未审批的生产写操作"]}`),
		CapabilityPolicy:            []byte(`{"skill_bindings":["incident-diagnosis"],"mcp_bindings":["prometheus"],"knowledge_base_bindings":[],"external_capability_bindings":["deploy-api"],"allowed_provider_types":["codex"]}`),
		ContextPolicy:               []byte(`{"sources":["monitoring","logs"]}`),
		ApprovalPolicy:              []byte(`{"risk_summary":"生产写操作需审批"}`),
		ArtifactContract:            []byte(`{"required":["Finding","Risk","DecisionRequest"]}`),
		InternalCollaborationPolicy: []byte(`{"allowed_request_types":["info_request","review_request"]}`),
		RuntimeScopePolicy:          []byte(`{"allowed_provider_types":["codex"]}`),
		HumanOwnerUserID:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		Status:                      "active",
		ApprovedBy:                  uuid.NullUUID{UUID: owner.ID, Valid: true},
		ApprovedAt:                  pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	require.NoError(t, err)

	rows, err := testQueries.ListTenantTeamSummaries(ctx, queries.ListTenantTeamSummariesParams{
		TenantID: tenantID,
		Q:        pgtype.Text{String: "ops", Valid: true},
		Limit:    20,
		Offset:   0,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int32(1), rows[0].MemberCount)
	assert.Equal(t, int32(1), rows[0].DigitalEmployeeCount)
	assert.Equal(t, int32(4), rows[0].CapabilityCount)
	assert.Equal(t, int32(1), rows[0].CurrentRevision.Int32)
	assert.Equal(t, "active", rows[0].GovernanceStatus)
	assert.Equal(t, "生产写操作需审批", rows[0].RiskSummary)

	for _, query := range []string{"summary-owner", "Summary Owner", "summary-owner@example.com"} {
		ownerRows, err := testQueries.ListTenantTeamSummaries(ctx, queries.ListTenantTeamSummariesParams{
			TenantID: tenantID,
			Q:        pgtype.Text{String: query, Valid: true},
			Limit:    20,
			Offset:   0,
		})
		require.NoError(t, err)
		require.Len(t, ownerRows, 1, "expected owner search %q to find the team", query)
		assert.Equal(t, team.ID, ownerRows[0].ID)
	}

	archivedTeam, err := testQueries.CreateTenantTeam(ctx, queries.CreateTenantTeamParams{
		TenantID:         tenantID,
		Slug:             "archive-summary",
		Name:             "归档团队",
		Status:           "archived",
		HumanOwnerUserID: uuid.NullUUID{UUID: owner.ID, Valid: true},
		Metadata:         []byte(`{}`),
	})
	require.NoError(t, err)
	_, err = testQueries.SetTenantTeamStatus(ctx, queries.SetTenantTeamStatusParams{
		TenantID: tenantID,
		ID:       archivedTeam.ID,
		Status:   "archived",
	})
	require.NoError(t, err)

	defaultRows, err := testQueries.ListTenantTeamSummaries(ctx, queries.ListTenantTeamSummariesParams{
		TenantID: tenantID,
		Q:        pgtype.Text{String: "archive-summary", Valid: true},
		Limit:    20,
		Offset:   0,
	})
	require.NoError(t, err)
	assert.Empty(t, defaultRows, "archived teams should be hidden from the default list")

	archivedRows, err := testQueries.ListTenantTeamSummaries(ctx, queries.ListTenantTeamSummariesParams{
		TenantID: tenantID,
		Status:   pgtype.Text{String: "archived", Valid: true},
		Q:        pgtype.Text{String: "archive-summary", Valid: true},
		Limit:    20,
		Offset:   0,
	})
	require.NoError(t, err)
	require.Len(t, archivedRows, 1)
	assert.Equal(t, "archived", archivedRows[0].Status)
}

func TestTeamMemberRoleRequestQueries(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	requester, err := testQueries.CreateUser(ctx, queries.CreateUserParams{
		Username:     "role-requester",
		DisplayName:  pgtype.Text{String: "Role Requester", Valid: true},
		Email:        pgtype.Text{String: "role-requester@example.com", Valid: true},
		PasswordHash: "$2a$10$hashedpassword",
		Status:       "active",
	})
	require.NoError(t, err)

	target, err := testQueries.CreateUser(ctx, queries.CreateUserParams{
		Username:     "role-target",
		DisplayName:  pgtype.Text{String: "Role Target", Valid: true},
		Email:        pgtype.Text{String: "role-target@example.com", Valid: true},
		PasswordHash: "$2a$10$hashedpassword",
		Status:       "active",
	})
	require.NoError(t, err)

	team, err := testQueries.CreateTenantTeam(ctx, queries.CreateTenantTeamParams{
		TenantID:         tenantID,
		Slug:             "role-requests",
		Name:             "角色申请团队",
		Status:           "active",
		HumanOwnerUserID: uuid.NullUUID{UUID: requester.ID, Valid: true},
		Metadata:         []byte(`{"domain":"team-management"}`),
	})
	require.NoError(t, err)

	request, err := testQueries.CreateTeamMemberRoleRequest(ctx, queries.CreateTeamMemberRoleRequestParams{
		TenantID:      tenantID,
		TeamID:        team.ID,
		TargetUserID:  target.ID,
		RequestedRole: "admin",
		RequestedBy:   requester.ID,
		Reason:        "需要维护团队治理草稿",
	})
	require.NoError(t, err)
	assert.Equal(t, tenantID, request.TenantID)
	assert.Equal(t, team.ID, request.TeamID)
	assert.Equal(t, target.ID, request.TargetUserID)
	assert.Equal(t, "admin", request.RequestedRole)
	assert.Equal(t, requester.ID, request.RequestedBy)
	assert.Equal(t, "pending", request.Status)

	requests, err := testQueries.ListTeamMemberRoleRequests(ctx, queries.ListTeamMemberRoleRequestsParams{
		TenantID: tenantID,
		TeamID:   team.ID,
		Status:   pgtype.Text{String: "pending", Valid: true},
		Limit:    20,
		Offset:   0,
	})
	require.NoError(t, err)
	require.Len(t, requests, 1)
	assert.Equal(t, request.ID, requests[0].ID)
	assert.Equal(t, "admin", requests[0].RequestedRole)
	assert.Equal(t, "需要维护团队治理草稿", requests[0].Reason)
}

func TestAuthzRuntimeNodeRejectsMalformedTenantScopePayloads(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")

	providersJSON, err := json.Marshal(map[string]interface{}{"providers": []string{"claude-code"}})
	require.NoError(t, err)
	metadataJSON, err := json.Marshal(map[string]interface{}{"test": true})
	require.NoError(t, err)
	now := pgtype.Timestamptz{}
	require.NoError(t, now.Scan(time.Now()))
	heartbeatAfter := pgtype.Timestamptz{}
	require.NoError(t, heartbeatAfter.Scan(time.Now().Add(-time.Minute)))

	nodeWithTeamID, err := testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "authz-malformed-tenant-team",
		Name:               "Authz Malformed Tenant Team",
		SupportedProviders: providersJSON,
		MaxSlots:           2,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           metadataJSON,
		LastHeartbeatAt:    now,
	})
	require.NoError(t, err)

	nodeWithWrongValue, err := testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "authz-malformed-tenant-value",
		Name:               "Authz Malformed Tenant Value",
		SupportedProviders: providersJSON,
		MaxSlots:           2,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           metadataJSON,
		LastHeartbeatAt:    now,
	})
	require.NoError(t, err)

	paramsJSON, err := json.Marshal(map[string]interface{}{"command": "authz"})
	require.NoError(t, err)
	task, err := testQueries.CreateTask(ctx, queries.CreateTaskParams{
		TenantID:     uuid.NullUUID{UUID: tenantID, Valid: true},
		TeamID:       uuid.NullUUID{},
		Title:        "Authz Tenant Scope Task",
		Description:  pgtype.Text{String: "Task for tenant scope authz", Valid: true},
		Status:       "pending",
		Priority:     1,
		ProviderType: "claude-code",
		Params:       paramsJSON,
	})
	require.NoError(t, err)

	_, err = testDB.Exec(ctx, `
		INSERT INTO runtime_node_scopes (tenant_id, runtime_node_id, team_id, scope_type, scope_value, status)
		VALUES ($1, $2, $3, 'tenant', $4, 'active')
	`, tenantID, nodeWithTeamID.ID, teamID, tenantID.String())
	require.NoError(t, err)

	tenantScopeWithTeamIDCovered, err := testQueries.RuntimeNodeCoversTaskScope(ctx, queries.RuntimeNodeCoversTaskScopeParams{
		TenantID:           tenantID,
		TeamID:             uuid.NullUUID{},
		TaskID:             task.ID,
		NodeID:             "authz-malformed-tenant-team",
		LastHeartbeatAfter: heartbeatAfter,
	})
	require.NoError(t, err)
	assert.False(t, tenantScopeWithTeamIDCovered)

	_, err = testDB.Exec(ctx, `
		INSERT INTO runtime_node_scopes (tenant_id, runtime_node_id, scope_type, scope_value, status)
		VALUES ($1, $2, 'tenant', 'not-the-tenant', 'active')
	`, tenantID, nodeWithWrongValue.ID)
	require.NoError(t, err)

	tenantScopeWithWrongValueCovered, err := testQueries.RuntimeNodeCoversTaskScope(ctx, queries.RuntimeNodeCoversTaskScopeParams{
		TenantID:           tenantID,
		TeamID:             uuid.NullUUID{},
		TaskID:             task.ID,
		NodeID:             "authz-malformed-tenant-value",
		LastHeartbeatAfter: heartbeatAfter,
	})
	require.NoError(t, err)
	assert.False(t, tenantScopeWithWrongValueCovered)
}

func TestAuthzCenterDecisionQueries(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	otherTenantID := uuid.MustParse("00000000-0000-0000-0000-000000000201")
	_, err := testDB.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, status)
		VALUES ($1, 'authz-decision-other', 'Authz Decision Other', 'active')
	`, otherTenantID)
	require.NoError(t, err)
	userID := uuid.New()
	_, err = testQueries.CreateWebOperationLog(ctx, queries.CreateWebOperationLogParams{
		TenantID: uuid.NullUUID{UUID: tenantID, Valid: true},
		UserID:   uuid.NullUUID{UUID: userID, Valid: true},
		Username: pgtype.Text{String: "authz-auditor", Valid: true},
		Module:   "authz",
		Action:   "task.claim",
		Result:   "failed",
		Details:  []byte(`{"engine":"db","reason":"runtime scope does not cover task","actor_type":"runtime_node","actor_id":"node-1"}`),
	})
	require.NoError(t, err)
	_, err = testQueries.CreateWebOperationLog(ctx, queries.CreateWebOperationLogParams{
		TenantID: uuid.NullUUID{UUID: otherTenantID, Valid: true},
		UserID:   uuid.NullUUID{UUID: userID, Valid: true},
		Username: pgtype.Text{String: "authz-auditor", Valid: true},
		Module:   "authz",
		Action:   "task.claim",
		Result:   "failed",
		Details:  []byte(`{"engine":"db","reason":"other tenant"}`),
	})
	require.NoError(t, err)

	rows, err := testQueries.ListAuthzDecisions(ctx, queries.ListAuthzDecisionsParams{
		TenantID: tenantID,
		Result:   pgtype.Text{String: "failed", Valid: true},
		Action:   pgtype.Text{String: "task.claim", Valid: true},
		Limit:    20,
		Offset:   0,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "task.claim", rows[0].Action)
	assert.Equal(t, "failed", rows[0].Result)
}

func TestAuthzCenterMemberPaginationPaginatesUsersBeforeMemberships(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	otherTenantID := uuid.MustParse("00000000-0000-0000-0000-000000000201")
	_, err := testDB.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, status)
		VALUES ($1, 'authz-member-other-tenant', 'Authz Member Other Tenant', 'active')
	`, otherTenantID)
	require.NoError(t, err)

	otherUser, err := testQueries.CreateUser(ctx, queries.CreateUserParams{
		Username:     "authz-member-other",
		DisplayName:  pgtype.Text{String: "Authz Member Other", Valid: true},
		Email:        pgtype.Text{String: "authz-member-other@example.com", Valid: true},
		PasswordHash: "$2a$10$hashedpassword",
		Status:       "active",
	})
	require.NoError(t, err)
	multiMemberUser, err := testQueries.CreateUser(ctx, queries.CreateUserParams{
		Username:     "authz-member-multi",
		DisplayName:  pgtype.Text{String: "Authz Member Multi", Valid: true},
		Email:        pgtype.Text{String: "authz-member-multi@example.com", Valid: true},
		PasswordHash: "$2a$10$hashedpassword",
		Status:       "active",
	})
	require.NoError(t, err)
	otherTenantUser, err := testQueries.CreateUser(ctx, queries.CreateUserParams{
		Username:     "authz-member-external",
		DisplayName:  pgtype.Text{String: "Authz Member External", Valid: true},
		Email:        pgtype.Text{String: "authz-member-external@example.com", Valid: true},
		PasswordHash: "$2a$10$hashedpassword",
		Status:       "active",
	})
	require.NoError(t, err)
	_, err = testDB.Exec(ctx, `
		UPDATE auth_users
		SET created_at = CASE
			WHEN id = $1 THEN NOW() - INTERVAL '1 hour'
			WHEN id = $2 THEN NOW()
			WHEN id = $3 THEN NOW() + INTERVAL '1 hour'
			ELSE created_at
		END
		WHERE id IN ($1, $2, $3)
	`, otherUser.ID, multiMemberUser.ID, otherTenantUser.ID)
	require.NoError(t, err)

	_, err = testDB.Exec(ctx, `
		INSERT INTO tenant_members (tenant_id, principal_type, principal_id, role, status)
		VALUES ($1, 'user', $2, 'owner', 'active')
	`, tenantID, multiMemberUser.ID)
	require.NoError(t, err)
	_, err = testDB.Exec(ctx, `
		INSERT INTO tenant_members (tenant_id, team_id, principal_type, principal_id, role, status)
		VALUES ($1, $2, 'user', $3, 'developer', 'active')
	`, tenantID, teamID, multiMemberUser.ID)
	require.NoError(t, err)
	_, err = testDB.Exec(ctx, `
		INSERT INTO tenant_members (tenant_id, principal_type, principal_id, role, status)
		VALUES ($1, 'user', $2, 'viewer', 'active')
	`, tenantID, otherUser.ID)
	require.NoError(t, err)
	_, err = testDB.Exec(ctx, `
		INSERT INTO tenant_members (tenant_id, principal_type, principal_id, role, status)
		VALUES ($1, 'user', $2, 'owner', 'active')
	`, otherTenantID, otherTenantUser.ID)
	require.NoError(t, err)

	rows, err := testQueries.ListAuthzMembers(ctx, queries.ListAuthzMembersParams{
		TenantID: tenantID,
		Limit:    1,
		Offset:   0,
	})
	require.NoError(t, err)
	require.Len(t, rows, 2)

	roles := map[string]bool{}
	for _, row := range rows {
		assert.Equal(t, multiMemberUser.ID, row.UserID)
		assert.Equal(t, "authz-member-multi", row.UserUsername)
		require.NotEmpty(t, row.Role)
		roles[row.Role] = true
	}
	assert.Equal(t, map[string]bool{"owner": true, "developer": true}, roles)
}

func TestAuthzCenterRuntimeScopeQueries(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	otherTenantID := uuid.MustParse("00000000-0000-0000-0000-000000000201")
	_, err := testDB.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, status)
		VALUES ($1, 'authz-scope-other-tenant', 'Authz Scope Other Tenant', 'active')
	`, otherTenantID)
	require.NoError(t, err)
	node, err := testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "authz-center-node",
		Name:               "Authz Center Node",
		SupportedProviders: []byte(`["codex"]`),
		MaxSlots:           2,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           []byte(`{}`),
		LastHeartbeatAt:    pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	require.NoError(t, err)
	_, err = testDB.Exec(ctx, `
		INSERT INTO runtime_nodes (
			tenant_id, node_id, name, supported_providers, max_slots, current_load, status, metadata, last_heartbeat_at
		) VALUES (
			$1, 'authz-center-other-node', 'Authz Center Other Node', '["codex"]'::jsonb, 2, 0, 'online', '{}'::jsonb, NOW()
		)
	`, otherTenantID)
	require.NoError(t, err)

	scope, err := testQueries.CreateRuntimeNodeScope(ctx, queries.CreateRuntimeNodeScopeParams{
		TenantID:      tenantID,
		RuntimeNodeID: node.ID,
		ScopeType:     "tenant",
		ScopeValue:    tenantID.String(),
	})
	require.NoError(t, err)
	require.Equal(t, "active", scope.Status)

	updated, err := testQueries.UpdateRuntimeNodeScopeStatus(ctx, queries.UpdateRuntimeNodeScopeStatusParams{
		TenantID: tenantID,
		ID:       scope.ID,
		Status:   "disabled",
	})
	require.NoError(t, err)
	assert.Equal(t, "disabled", updated.Status)
	assert.True(t, updated.DisabledAt.Valid)

	_, err = testQueries.UpdateRuntimeNodeScopeStatus(ctx, queries.UpdateRuntimeNodeScopeStatusParams{
		TenantID: otherTenantID,
		ID:       scope.ID,
		Status:   "active",
	})
	require.ErrorIs(t, err, pgx.ErrNoRows)

	nodes, err := testQueries.ListRuntimeNodesWithScopes(ctx, tenantID)
	require.NoError(t, err)
	var listedScope *queries.ListRuntimeNodesWithScopesRow
	for i := range nodes {
		assert.Equal(t, tenantID, nodes[i].RuntimeTenantID)
		assert.NotEqual(t, "authz-center-other-node", nodes[i].NodeID)
		if nodes[i].RuntimeNodeID == node.ID && nodes[i].ScopeID.Valid && nodes[i].ScopeID.UUID == scope.ID {
			listedScope = &nodes[i]
		}
	}
	require.NotNil(t, listedScope)
	assert.Equal(t, node.ID, listedScope.RuntimeNodeID)
	assert.Equal(t, tenantID, listedScope.RuntimeTenantID)
	assert.True(t, listedScope.ScopeID.Valid)
	assert.Equal(t, scope.ID, listedScope.ScopeID.UUID)
	assert.True(t, listedScope.ScopeStatus.Valid)
	assert.Equal(t, "disabled", listedScope.ScopeStatus.String)
	assert.True(t, listedScope.ScopeDisabledAt.Valid)

	reactivated, err := testQueries.CreateRuntimeNodeScope(ctx, queries.CreateRuntimeNodeScopeParams{
		TenantID:      tenantID,
		RuntimeNodeID: node.ID,
		ScopeType:     "tenant",
		ScopeValue:    tenantID.String(),
	})
	require.NoError(t, err)
	assert.Equal(t, scope.ID, reactivated.ID)
	assert.Equal(t, "active", reactivated.Status)
	assert.False(t, reactivated.DisabledAt.Valid)
}

func TestAuthzCenterRuntimeScopeRejectsInconsistentInput(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	otherTenantID := uuid.MustParse("00000000-0000-0000-0000-000000000201")
	otherTeamID := uuid.MustParse("00000000-0000-0000-0000-000000000301")
	_, err := testDB.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, status)
		VALUES ($1, 'authz-other', 'Authz Other Tenant', 'active')
	`, otherTenantID)
	require.NoError(t, err)
	_, err = testDB.Exec(ctx, `
		INSERT INTO tenant_teams (id, tenant_id, slug, name, status)
		VALUES ($1, $2, 'authz-other', 'Authz Other Team', 'active')
	`, otherTeamID, otherTenantID)
	require.NoError(t, err)

	node, err := testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "authz-center-consistency-node",
		Name:               "Authz Center Consistency Node",
		SupportedProviders: []byte(`["codex"]`),
		MaxSlots:           2,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           []byte(`{}`),
		LastHeartbeatAt:    pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	require.NoError(t, err)

	_, err = testQueries.CreateRuntimeNodeScope(ctx, queries.CreateRuntimeNodeScopeParams{
		TenantID:      otherTenantID,
		RuntimeNodeID: node.ID,
		ScopeType:     "tenant",
		ScopeValue:    otherTenantID.String(),
	})
	require.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.CreateRuntimeNodeScope(ctx, queries.CreateRuntimeNodeScopeParams{
		TenantID:      tenantID,
		RuntimeNodeID: node.ID,
		TeamID:        uuid.NullUUID{UUID: otherTeamID, Valid: true},
		ScopeType:     "team",
		ScopeValue:    otherTeamID.String(),
	})
	require.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.CreateRuntimeNodeScope(ctx, queries.CreateRuntimeNodeScopeParams{
		TenantID:      tenantID,
		RuntimeNodeID: node.ID,
		TeamID:        uuid.NullUUID{UUID: teamID, Valid: true},
		ScopeType:     "team",
		ScopeValue:    uuid.NewString(),
	})
	require.ErrorIs(t, err, pgx.ErrNoRows)
}

// ============================================================================
// Runtime Node Tests
// ============================================================================

func TestCreateRuntimeNode(t *testing.T) {
	ctx := context.Background()

	supportedProviders := map[string]interface{}{
		"providers": []string{"claude-code", "opencode"},
	}
	providersJSON, err := json.Marshal(supportedProviders)
	require.NoError(t, err)

	metadata := map[string]interface{}{
		"version": "1.0.0",
		"region":  "us-west-2",
	}
	metadataJSON, err := json.Marshal(metadata)
	require.NoError(t, err)

	now := pgtype.Timestamptz{}
	err = now.Scan(time.Now())
	require.NoError(t, err)

	node, err := testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "test-node-001",
		Name:               "Test Node 001",
		SupportedProviders: providersJSON,
		MaxSlots:           4,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           metadataJSON,
		LastHeartbeatAt:    now,
	})

	require.NoError(t, err)
	assert.Equal(t, "test-node-001", node.NodeID)
	assert.Equal(t, "Test Node 001", node.Name)
	assert.Equal(t, int32(4), node.MaxSlots)
	assert.Equal(t, int32(0), node.CurrentLoad)
	assert.Equal(t, "online", node.Status)

	// 清理
	defer testQueries.DeleteRuntimeNode(ctx, "test-node-001")
}

func TestGetRuntimeNode(t *testing.T) {
	ctx := context.Background()

	// 创建测试节点
	providersJSON, _ := json.Marshal(map[string]interface{}{"providers": []string{"claude-code"}})
	metadataJSON, _ := json.Marshal(map[string]interface{}{"test": true})
	now := pgtype.Timestamptz{}
	now.Scan(time.Now())

	created, err := testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "test-node-002",
		Name:               "Test Node 002",
		SupportedProviders: providersJSON,
		MaxSlots:           2,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           metadataJSON,
		LastHeartbeatAt:    now,
	})
	require.NoError(t, err)
	defer testQueries.DeleteRuntimeNode(ctx, "test-node-002")

	// 查询节点
	node, err := testQueries.GetRuntimeNode(ctx, "test-node-002")
	require.NoError(t, err)
	assert.Equal(t, created.NodeID, node.NodeID)
	assert.Equal(t, created.Name, node.Name)
}

func TestUpdateRuntimeNodeHeartbeat(t *testing.T) {
	ctx := context.Background()

	// 创建测试节点
	providersJSON, _ := json.Marshal(map[string]interface{}{"providers": []string{"claude-code"}})
	metadataJSON, _ := json.Marshal(map[string]interface{}{"test": true})
	now := pgtype.Timestamptz{}
	now.Scan(time.Now())

	_, err := testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "test-node-003",
		Name:               "Test Node 003",
		SupportedProviders: providersJSON,
		MaxSlots:           2,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           metadataJSON,
		LastHeartbeatAt:    now,
	})
	require.NoError(t, err)
	defer testQueries.DeleteRuntimeNode(ctx, "test-node-003")

	// 更新心跳
	time.Sleep(100 * time.Millisecond)
	newHeartbeat := pgtype.Timestamptz{}
	newHeartbeat.Scan(time.Now())

	updated, err := testQueries.UpdateRuntimeNodeHeartbeat(ctx, queries.UpdateRuntimeNodeHeartbeatParams{
		NodeID:          "test-node-003",
		LastHeartbeatAt: newHeartbeat,
	})
	require.NoError(t, err)
	assert.True(t, updated.LastHeartbeatAt.Time.After(now.Time))
}

func TestListOnlineNodes(t *testing.T) {
	ctx := context.Background()

	// 创建多个节点
	providersJSON, _ := json.Marshal(map[string]interface{}{"providers": []string{"claude-code"}})
	metadataJSON, _ := json.Marshal(map[string]interface{}{"test": true})
	now := pgtype.Timestamptz{}
	now.Scan(time.Now())

	// 在线节点
	_, err := testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "test-node-online-1",
		Name:               "Online Node 1",
		SupportedProviders: providersJSON,
		MaxSlots:           2,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           metadataJSON,
		LastHeartbeatAt:    now,
	})
	require.NoError(t, err)
	defer testQueries.DeleteRuntimeNode(ctx, "test-node-online-1")

	// 离线节点
	oldHeartbeat := pgtype.Timestamptz{}
	oldHeartbeat.Scan(time.Now().Add(-10 * time.Minute))
	_, err = testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "test-node-offline-1",
		Name:               "Offline Node 1",
		SupportedProviders: providersJSON,
		MaxSlots:           2,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           metadataJSON,
		LastHeartbeatAt:    oldHeartbeat,
	})
	require.NoError(t, err)
	defer testQueries.DeleteRuntimeNode(ctx, "test-node-offline-1")

	// 查询在线节点（心跳在 5 分钟内）
	threshold := pgtype.Timestamptz{}
	threshold.Scan(time.Now().Add(-5 * time.Minute))

	nodes, err := testQueries.ListOnlineNodes(ctx, threshold)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(nodes), 1)

	// 验证返回的节点都是最近心跳的
	for _, node := range nodes {
		assert.True(t, node.LastHeartbeatAt.Time.After(threshold.Time))
	}
}

func TestRuntimeEnrollmentAndSessionQueries(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	otherTenantID := uuid.MustParse("00000000-0000-0000-0000-000000000201")
	_, err := testDB.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, status)
		VALUES ($1, 'runtime-enroll-other', 'Runtime Enrollment Other Tenant', 'active')
		ON CONFLICT (id) DO NOTHING
	`, otherTenantID)
	require.NoError(t, err)

	keyExpiresAt := pgtype.Timestamptz{}
	require.NoError(t, keyExpiresAt.Scan(time.Now().Add(24*time.Hour)))
	bootstrapKey, err := testQueries.CreateRuntimeBootstrapKey(ctx, queries.CreateRuntimeBootstrapKeyParams{
		TenantID:    uuid.NullUUID{UUID: tenantID, Valid: true},
		Name:        "customer-vm-bootstrap",
		KeyHash:     "bootstrap-key-hash",
		Status:      "active",
		ExpiresAt:   keyExpiresAt,
		CreatedBy:   uuid.NullUUID{UUID: uuid.New(), Valid: true},
		Description: pgtype.Text{String: "Customer VM enrollment key", Valid: true},
		Metadata:    []byte(`{"environment":"customer-vm"}`),
	})
	require.NoError(t, err)

	otherTenantBootstrapKey, err := testQueries.CreateRuntimeBootstrapKey(ctx, queries.CreateRuntimeBootstrapKeyParams{
		TenantID:    uuid.NullUUID{UUID: otherTenantID, Valid: true},
		Name:        "other-tenant-bootstrap",
		KeyHash:     "other-tenant-bootstrap-key-hash",
		Status:      "active",
		ExpiresAt:   keyExpiresAt,
		CreatedBy:   uuid.NullUUID{UUID: uuid.New(), Valid: true},
		Description: pgtype.Text{String: "Other tenant enrollment key", Valid: true},
		Metadata:    []byte(`{"environment":"other"}`),
	})
	require.NoError(t, err)

	revokedBootstrapKey, err := testQueries.CreateRuntimeBootstrapKey(ctx, queries.CreateRuntimeBootstrapKeyParams{
		TenantID:    uuid.NullUUID{UUID: tenantID, Valid: true},
		Name:        "revoked-bootstrap",
		KeyHash:     "revoked-bootstrap-key-hash",
		Status:      "active",
		ExpiresAt:   keyExpiresAt,
		CreatedBy:   uuid.NullUUID{UUID: uuid.New(), Valid: true},
		Description: pgtype.Text{String: "Revoked enrollment key", Valid: true},
		Metadata:    []byte(`{"environment":"customer-vm"}`),
	})
	require.NoError(t, err)
	_, err = testQueries.RevokeRuntimeBootstrapKey(ctx, queries.RevokeRuntimeBootstrapKeyParams{
		ID:            revokedBootstrapKey.ID,
		TenantID:      tenantID,
		RevokedBy:     uuid.NullUUID{UUID: uuid.New(), Valid: true},
		RevokedReason: pgtype.Text{String: "operator revoked bootstrap key", Valid: true},
	})
	require.NoError(t, err)

	replacementBootstrapKey, err := testQueries.CreateRuntimeBootstrapKey(ctx, queries.CreateRuntimeBootstrapKeyParams{
		TenantID:    uuid.NullUUID{UUID: tenantID, Valid: true},
		Name:        "replacement-bootstrap",
		KeyHash:     "replacement-bootstrap-key-hash",
		Status:      "active",
		ExpiresAt:   keyExpiresAt,
		CreatedBy:   uuid.NullUUID{UUID: uuid.New(), Valid: true},
		Description: pgtype.Text{String: "Replacement enrollment key", Valid: true},
		Metadata:    []byte(`{"environment":"customer-vm"}`),
	})
	require.NoError(t, err)

	activeKey, err := testQueries.GetActiveRuntimeBootstrapKeyByHash(ctx, queries.GetActiveRuntimeBootstrapKeyByHashParams{
		TenantID: tenantID,
		KeyHash:  "bootstrap-key-hash",
	})
	require.NoError(t, err)
	assert.Equal(t, bootstrapKey.ID, activeKey.ID)

	now := pgtype.Timestamptz{}
	require.NoError(t, now.Scan(time.Now()))
	node, err := testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "runtime-enroll-node",
		Name:               "Runtime Enrollment Node",
		SupportedProviders: []byte(`{"providers":["claude-code"]}`),
		MaxSlots:           4,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           []byte(`{"region":"local"}`),
		LastHeartbeatAt:    now,
	})
	require.NoError(t, err)

	reusedTenantNode, err := testQueries.UpsertRuntimeNodeForTenant(ctx, queries.UpsertRuntimeNodeForTenantParams{
		TenantID:           tenantID,
		NodeID:             "runtime-enroll-node",
		Name:               "Runtime Enrollment Node Refreshed",
		SupportedProviders: []byte(`{"providers":["claude-code","codex"]}`),
		MaxSlots:           6,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           []byte(`{"region":"local","refreshed":true}`),
		LastHeartbeatAt:    now,
	})
	require.NoError(t, err)
	assert.Equal(t, node.ID, reusedTenantNode.ID)
	assert.Equal(t, tenantID, reusedTenantNode.TenantID)

	_, err = testQueries.UpsertRuntimeNodeForTenant(ctx, queries.UpsertRuntimeNodeForTenantParams{
		TenantID:           otherTenantID,
		NodeID:             "runtime-enroll-node",
		Name:               "Other Tenant Runtime Should Not Take Over",
		SupportedProviders: []byte(`{"providers":["codex"]}`),
		MaxSlots:           1,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           []byte(`{"region":"other"}`),
		LastHeartbeatAt:    now,
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	alternateNode, err := testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "runtime-enroll-node-rebound",
		Name:               "Runtime Enrollment Rebound Node",
		SupportedProviders: []byte(`{"providers":["claude-code"]}`),
		MaxSlots:           4,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           []byte(`{"region":"local"}`),
		LastHeartbeatAt:    now,
	})
	require.NoError(t, err)

	callerApprovedNode, err := testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "runtime-enroll-node-caller-approved",
		Name:               "Runtime Enrollment Caller Approved Node",
		SupportedProviders: []byte(`{"providers":["claude-code"]}`),
		MaxSlots:           4,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           []byte(`{"region":"local"}`),
		LastHeartbeatAt:    now,
	})
	require.NoError(t, err)

	rejectedNode, err := testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "runtime-enroll-node-rejected",
		Name:               "Runtime Enrollment Rejected Node",
		SupportedProviders: []byte(`{"providers":["claude-code"]}`),
		MaxSlots:           4,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           []byte(`{"region":"local"}`),
		LastHeartbeatAt:    now,
	})
	require.NoError(t, err)

	revokedNode, err := testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "runtime-enroll-node-revoked",
		Name:               "Runtime Enrollment Revoked Node",
		SupportedProviders: []byte(`{"providers":["claude-code"]}`),
		MaxSlots:           4,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           []byte(`{"region":"local"}`),
		LastHeartbeatAt:    now,
	})
	require.NoError(t, err)

	disabledNode, err := testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "runtime-enroll-node-disabled",
		Name:               "Runtime Enrollment Disabled Node",
		SupportedProviders: []byte(`{"providers":["claude-code"]}`),
		MaxSlots:           4,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           []byte(`{"region":"local"}`),
		LastHeartbeatAt:    now,
	})
	require.NoError(t, err)
	_, err = testQueries.UpdateRuntimeNodeStatus(ctx, queries.UpdateRuntimeNodeStatusParams{
		NodeID: disabledNode.NodeID,
		Status: "offline",
	})
	require.NoError(t, err)

	var otherTenantRuntimeNodeID uuid.UUID
	err = testDB.QueryRow(ctx, `
		INSERT INTO runtime_nodes (
			tenant_id,
			node_id,
			name,
			supported_providers,
			max_slots,
			current_load,
			status,
			metadata,
			last_heartbeat_at
		) VALUES (
			$1,
			'runtime-enroll-other-tenant-node',
			'Runtime Enrollment Other Tenant Node',
			'{"providers":["claude-code"]}'::jsonb,
			4,
			0,
			'online',
			'{"region":"other"}'::jsonb,
			$2
		)
		RETURNING id
	`, otherTenantID, now).Scan(&otherTenantRuntimeNodeID)
	require.NoError(t, err)

	_, hasStatus := reflect.TypeOf(queries.UpsertRuntimeEnrollmentParams{}).FieldByName("Status")
	assert.False(t, hasStatus)

	enrollment, err := testQueries.UpsertRuntimeEnrollment(ctx, queries.UpsertRuntimeEnrollmentParams{
		TenantID:       tenantID,
		NodeID:         "runtime-enroll-node",
		BootstrapKeyID: bootstrapKey.ID,
		RequestPayload: []byte(`{"node_id":"runtime-enroll-node","version":"0.1.0"}`),
		LastHelloAt:    now,
	})
	require.NoError(t, err)
	assert.Equal(t, "pending", enrollment.Status)

	otherTenantConflictingEnrollment, err := testQueries.UpsertRuntimeEnrollment(ctx, queries.UpsertRuntimeEnrollmentParams{
		TenantID:       otherTenantID,
		NodeID:         "runtime-enroll-node",
		BootstrapKeyID: otherTenantBootstrapKey.ID,
		RequestPayload: []byte(`{"node_id":"runtime-enroll-node","version":"other-tenant"}`),
		LastHelloAt:    now,
	})
	require.NoError(t, err)
	_, err = testQueries.ApproveRuntimeEnrollmentWithNode(ctx, queries.ApproveRuntimeEnrollmentWithNodeParams{
		ID:                 otherTenantConflictingEnrollment.ID,
		TenantID:           otherTenantID,
		ApprovedBy:         uuid.NullUUID{UUID: uuid.New(), Valid: true},
		Name:               "Other Tenant Runtime Should Not Take Over",
		SupportedProviders: []byte(`{"providers":["codex"]}`),
		MaxSlots:           1,
		CurrentLoad:        0,
		NodeStatus:         "online",
		Metadata:           []byte(`{"region":"other"}`),
		LastHeartbeatAt:    now,
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)
	otherTenantConflictingEnrollment, err = testQueries.GetRuntimeEnrollment(ctx, queries.GetRuntimeEnrollmentParams{
		TenantID: otherTenantID,
		ID:       otherTenantConflictingEnrollment.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, "pending", otherTenantConflictingEnrollment.Status)
	assert.False(t, otherTenantConflictingEnrollment.RuntimeNodeID.Valid)

	_, err = testQueries.UpsertRuntimeEnrollment(ctx, queries.UpsertRuntimeEnrollmentParams{
		TenantID:       tenantID,
		NodeID:         "runtime-enroll-node-no-bootstrap",
		BootstrapKeyID: uuid.Nil,
		RequestPayload: []byte(`{"node_id":"runtime-enroll-node-no-bootstrap"}`),
		LastHelloAt:    now,
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	callerApprovedEnrollment, err := testQueries.UpsertRuntimeEnrollment(ctx, queries.UpsertRuntimeEnrollmentParams{
		TenantID:       tenantID,
		NodeID:         "runtime-enroll-node-caller-approved",
		BootstrapKeyID: bootstrapKey.ID,
		RequestPayload: []byte(`{"node_id":"runtime-enroll-node-caller-approved"}`),
		LastHelloAt:    now,
	})
	require.NoError(t, err)
	assert.Equal(t, "pending", callerApprovedEnrollment.Status)

	_, err = testQueries.CreateRuntimeSession(ctx, queries.CreateRuntimeSessionParams{
		TenantID:        tenantID,
		RuntimeNodeID:   callerApprovedNode.ID,
		EnrollmentID:    uuid.NullUUID{UUID: callerApprovedEnrollment.ID, Valid: true},
		TokenLookupHash: "0b0b4a2f2c33c8f20a7a50e617b9eac6f6e26f629dddf1cdaff1301ff68c60d5",
		TokenSecretHash: "$2a$10$callerApprovedEnrollmentSessionHash",
		ExpiresAt:       keyExpiresAt,
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.UpsertRuntimeEnrollment(ctx, queries.UpsertRuntimeEnrollmentParams{
		TenantID:       otherTenantID,
		NodeID:         "runtime-enroll-node",
		BootstrapKeyID: bootstrapKey.ID,
		RequestPayload: []byte(`{"node_id":"runtime-enroll-node","tenant":"other"}`),
		LastHelloAt:    now,
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.UpsertRuntimeEnrollment(ctx, queries.UpsertRuntimeEnrollmentParams{
		TenantID:       tenantID,
		NodeID:         "runtime-enroll-node-mismatched",
		BootstrapKeyID: bootstrapKey.ID,
		RequestPayload: []byte(`{"node_id":"runtime-enroll-node-mismatched"}`),
		LastHelloAt:    now,
	})
	require.NoError(t, err)

	_, err = testQueries.UpsertRuntimeEnrollment(ctx, queries.UpsertRuntimeEnrollmentParams{
		TenantID:       tenantID,
		NodeID:         "runtime-enroll-node-wrong-runtime-tenant",
		BootstrapKeyID: bootstrapKey.ID,
		RequestPayload: []byte(`{"node_id":"runtime-enroll-node-wrong-runtime-tenant"}`),
		LastHelloAt:    now,
	})
	require.NoError(t, err)

	_, err = testQueries.UpsertRuntimeEnrollment(ctx, queries.UpsertRuntimeEnrollmentParams{
		TenantID:       tenantID,
		NodeID:         "runtime-enroll-node-disabled",
		BootstrapKeyID: bootstrapKey.ID,
		RequestPayload: []byte(`{"node_id":"runtime-enroll-node-disabled"}`),
		LastHelloAt:    now,
	})
	require.NoError(t, err)

	_, err = testQueries.UpsertRuntimeEnrollment(ctx, queries.UpsertRuntimeEnrollmentParams{
		TenantID:       tenantID,
		NodeID:         "runtime-enroll-node-wrong-bootstrap-tenant",
		BootstrapKeyID: otherTenantBootstrapKey.ID,
		RequestPayload: []byte(`{"node_id":"runtime-enroll-node-wrong-bootstrap-tenant"}`),
		LastHelloAt:    now,
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.UpsertRuntimeEnrollment(ctx, queries.UpsertRuntimeEnrollmentParams{
		TenantID:       tenantID,
		NodeID:         "runtime-enroll-node-revoked-bootstrap",
		BootstrapKeyID: revokedBootstrapKey.ID,
		RequestPayload: []byte(`{"node_id":"runtime-enroll-node-revoked-bootstrap"}`),
		LastHelloAt:    now,
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	var invalidEnrollmentCount int
	err = testDB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM runtime_enrollments
		WHERE node_id IN (
			'runtime-enroll-node-no-bootstrap',
			'runtime-enroll-node-wrong-bootstrap-tenant',
			'runtime-enroll-node-revoked-bootstrap'
		)
	`).Scan(&invalidEnrollmentCount)
	require.NoError(t, err)
	assert.Zero(t, invalidEnrollmentCount)

	defaultEnrollment, err := testQueries.GetRuntimeEnrollmentByNodeID(ctx, queries.GetRuntimeEnrollmentByNodeIDParams{
		TenantID: tenantID,
		NodeID:   "runtime-enroll-node",
	})
	require.NoError(t, err)
	assert.Equal(t, enrollment.ID, defaultEnrollment.ID)
	assert.Equal(t, tenantID, defaultEnrollment.TenantID)

	sessionExpiresAt := pgtype.Timestamptz{}
	require.NoError(t, sessionExpiresAt.Scan(time.Now().Add(12*time.Hour)))
	_, err = testQueries.CreateRuntimeSession(ctx, queries.CreateRuntimeSessionParams{
		TenantID:        tenantID,
		RuntimeNodeID:   node.ID,
		EnrollmentID:    uuid.NullUUID{UUID: enrollment.ID, Valid: true},
		TokenLookupHash: "7019a7eed9f6c05309b6b9402fe9bc59d41f9a6dfc0c1b8c40b6a4d829148a11",
		TokenSecretHash: "$2a$10$pendingRuntimeSessionSecretHash",
		ExpiresAt:       sessionExpiresAt,
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	approved, err := testQueries.ApproveRuntimeEnrollment(ctx, queries.ApproveRuntimeEnrollmentParams{
		RuntimeNodeID: node.ID,
		ID:            enrollment.ID,
		TenantID:      tenantID,
		ApprovedBy:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
	})
	require.NoError(t, err)
	assert.Equal(t, "approved", approved.Status)
	require.True(t, approved.ApprovedAt.Valid)

	terminalHelloAt := pgtype.Timestamptz{}
	require.NoError(t, terminalHelloAt.Scan(time.Now().Add(2*time.Minute)))
	terminalHello, err := testQueries.UpsertRuntimeEnrollment(ctx, queries.UpsertRuntimeEnrollmentParams{
		TenantID:       tenantID,
		NodeID:         "runtime-enroll-node",
		BootstrapKeyID: replacementBootstrapKey.ID,
		RequestPayload: []byte(`{"node_id":"runtime-enroll-node","version":"rebound"}`),
		LastHelloAt:    terminalHelloAt,
	})
	require.NoError(t, err)
	assert.Equal(t, approved.ID, terminalHello.ID)
	assert.Equal(t, "approved", terminalHello.Status)
	require.True(t, terminalHello.RuntimeNodeID.Valid)
	assert.Equal(t, node.ID, terminalHello.RuntimeNodeID.UUID)
	assert.Equal(t, bootstrapKey.ID, terminalHello.BootstrapKeyID)
	assert.JSONEq(t, `{"node_id":"runtime-enroll-node","version":"0.1.0"}`, string(terminalHello.RequestPayload))
	assert.WithinDuration(t, terminalHelloAt.Time, terminalHello.LastHelloAt.Time, time.Second)

	_, err = testQueries.RejectRuntimeEnrollment(ctx, queries.RejectRuntimeEnrollmentParams{
		ID:           approved.ID,
		TenantID:     tenantID,
		RejectedBy:   uuid.NullUUID{UUID: uuid.New(), Valid: true},
		RejectReason: pgtype.Text{String: "approved enrollments cannot be rejected", Valid: true},
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	rejectedEnrollment, err := testQueries.UpsertRuntimeEnrollment(ctx, queries.UpsertRuntimeEnrollmentParams{
		TenantID:       tenantID,
		NodeID:         "runtime-enroll-node-rejected",
		BootstrapKeyID: bootstrapKey.ID,
		RequestPayload: []byte(`{"node_id":"runtime-enroll-node-rejected"}`),
		LastHelloAt:    now,
	})
	require.NoError(t, err)
	rejected, err := testQueries.RejectRuntimeEnrollment(ctx, queries.RejectRuntimeEnrollmentParams{
		ID:           rejectedEnrollment.ID,
		TenantID:     tenantID,
		RejectedBy:   uuid.NullUUID{UUID: uuid.New(), Valid: true},
		RejectReason: pgtype.Text{String: "operator rejected", Valid: true},
	})
	require.NoError(t, err)
	assert.Equal(t, "rejected", rejected.Status)

	_, err = testQueries.ApproveRuntimeEnrollment(ctx, queries.ApproveRuntimeEnrollmentParams{
		RuntimeNodeID: rejectedNode.ID,
		ID:            rejectedEnrollment.ID,
		TenantID:      tenantID,
		ApprovedBy:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.RevokeRuntimeEnrollment(ctx, queries.RevokeRuntimeEnrollmentParams{
		ID:           rejectedEnrollment.ID,
		TenantID:     tenantID,
		RevokedBy:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
		RevokeReason: pgtype.Text{String: "rejected enrollments cannot be revoked", Valid: true},
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	revokedEnrollment, err := testQueries.UpsertRuntimeEnrollment(ctx, queries.UpsertRuntimeEnrollmentParams{
		TenantID:       tenantID,
		NodeID:         "runtime-enroll-node-revoked",
		BootstrapKeyID: bootstrapKey.ID,
		RequestPayload: []byte(`{"node_id":"runtime-enroll-node-revoked"}`),
		LastHelloAt:    now,
	})
	require.NoError(t, err)
	revokedEnrollmentRow, err := testQueries.RevokeRuntimeEnrollment(ctx, queries.RevokeRuntimeEnrollmentParams{
		ID:           revokedEnrollment.ID,
		TenantID:     tenantID,
		RevokedBy:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
		RevokeReason: pgtype.Text{String: "operator revoked", Valid: true},
	})
	require.NoError(t, err)
	assert.Equal(t, "revoked", revokedEnrollmentRow.Status)

	_, err = testQueries.ApproveRuntimeEnrollment(ctx, queries.ApproveRuntimeEnrollmentParams{
		RuntimeNodeID: revokedNode.ID,
		ID:            revokedEnrollmentRow.ID,
		TenantID:      tenantID,
		ApprovedBy:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.CreateRuntimeSession(ctx, queries.CreateRuntimeSessionParams{
		TenantID:        tenantID,
		RuntimeNodeID:   alternateNode.ID,
		EnrollmentID:    uuid.NullUUID{UUID: approved.ID, Valid: true},
		TokenLookupHash: "0d89688e9f4af8f3cc501cc9406a9daa9ae98b512704fcb69c4fdf818175c998",
		TokenSecretHash: "$2a$10$mismatchedRuntimeSessionSecretHash",
		ExpiresAt:       sessionExpiresAt,
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	session, err := testQueries.CreateRuntimeSession(ctx, queries.CreateRuntimeSessionParams{
		TenantID:        tenantID,
		RuntimeNodeID:   node.ID,
		EnrollmentID:    uuid.NullUUID{UUID: approved.ID, Valid: true},
		TokenLookupHash: "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		TokenSecretHash: "$2a$10$runtimeSessionSecretHashForLaterValidation",
		ExpiresAt:       sessionExpiresAt,
	})
	require.NoError(t, err)

	validatedSession, err := testQueries.GetActiveRuntimeSessionByLookupHash(ctx, "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08")
	require.NoError(t, err)
	assert.Equal(t, session.ID, validatedSession.ID)
	assert.Equal(t, "$2a$10$runtimeSessionSecretHashForLaterValidation", validatedSession.TokenSecretHash)

	renewedExpiresAt := pgtype.Timestamptz{}
	require.NoError(t, renewedExpiresAt.Scan(time.Now().Add(24*time.Hour)))
	renewedSession, err := testQueries.RenewRuntimeSession(ctx, queries.RenewRuntimeSessionParams{
		ID:        session.ID,
		TenantID:  tenantID,
		ExpiresAt: renewedExpiresAt,
	})
	require.NoError(t, err)
	assert.True(t, renewedSession.ExpiresAt.Time.After(session.ExpiresAt.Time))

	expiredSessionExpiresAt := pgtype.Timestamptz{}
	require.NoError(t, expiredSessionExpiresAt.Scan(time.Now().Add(-1*time.Hour)))
	expiredSession, err := testQueries.CreateRuntimeSession(ctx, queries.CreateRuntimeSessionParams{
		TenantID:        tenantID,
		RuntimeNodeID:   node.ID,
		EnrollmentID:    uuid.NullUUID{UUID: approved.ID, Valid: true},
		TokenLookupHash: "de9f2c7fd25e1b3afad3e85a0bd17d9b954789f0ecfbb0a1c6dfc1d54c2bb74b",
		TokenSecretHash: "$2a$10$expiredRuntimeSessionSecretHash",
		ExpiresAt:       expiredSessionExpiresAt,
	})
	require.NoError(t, err)

	_, err = testQueries.RenewRuntimeSession(ctx, queries.RenewRuntimeSessionParams{
		ID:        expiredSession.ID,
		TenantID:  tenantID,
		ExpiresAt: renewedExpiresAt,
	})
	assert.Error(t, err)

	_, err = testQueries.TouchRuntimeSessionLastSeen(ctx, queries.TouchRuntimeSessionLastSeenParams{
		ID:       expiredSession.ID,
		TenantID: tenantID,
	})
	assert.Error(t, err)

	capability, err := testQueries.UpsertRuntimeCapability(ctx, queries.UpsertRuntimeCapabilityParams{
		TenantID:         tenantID,
		RuntimeNodeID:    node.ID,
		CapabilityType:   "provider",
		CapabilityKey:    "claude-code",
		ProviderType:     "claude-code",
		ProviderVersion:  pgtype.Text{String: "1.0.0", Valid: true},
		BinaryPath:       pgtype.Text{String: "/usr/local/bin/claude", Valid: true},
		Available:        true,
		WorkspaceBaseDir: pgtype.Text{String: "/data/superteam/workspaces", Valid: true},
		Capacity:         []byte(`{"max_slots":4}`),
		Labels:           []byte(`{"os":"darwin"}`),
		Status:           "healthy",
		Details:          []byte(`{"source":"initial"}`),
		HealthStatus:     "healthy",
		Metadata:         []byte(`{"source":"hello"}`),
		LastSeenAt:       now,
	})
	require.NoError(t, err)
	assert.Equal(t, "claude-code", capability.ProviderType)

	later := pgtype.Timestamptz{}
	require.NoError(t, later.Scan(time.Now().Add(time.Minute)))
	refreshedCapability, err := testQueries.UpsertRuntimeCapability(ctx, queries.UpsertRuntimeCapabilityParams{
		TenantID:         tenantID,
		RuntimeNodeID:    node.ID,
		CapabilityType:   "provider",
		CapabilityKey:    "claude-code",
		ProviderType:     "claude-code",
		ProviderVersion:  pgtype.Text{String: "9.9.9", Valid: true},
		BinaryPath:       pgtype.Text{String: "/tmp/rewritten", Valid: true},
		Available:        false,
		WorkspaceBaseDir: pgtype.Text{String: "/tmp/rewritten-workspace", Valid: true},
		Capacity:         []byte(`{"max_slots":1}`),
		Labels:           []byte(`{"os":"linux"}`),
		Status:           "degraded",
		Details:          []byte(`{"reason":"binary_missing"}`),
		HealthStatus:     "degraded",
		Metadata:         []byte(`{"source":"refresh"}`),
		LastSeenAt:       later,
	})
	require.NoError(t, err)
	assert.Equal(t, capability.ID, refreshedCapability.ID)
	assert.Equal(t, tenantID, refreshedCapability.TenantID)
	assert.Equal(t, "degraded", refreshedCapability.Status)
	assert.JSONEq(t, `{"reason":"binary_missing"}`, string(refreshedCapability.Details))
	assert.Equal(t, "9.9.9", refreshedCapability.ProviderVersion.String)
	assert.Equal(t, "/tmp/rewritten", refreshedCapability.BinaryPath.String)
	assert.False(t, refreshedCapability.Available)
	assert.Equal(t, "/tmp/rewritten-workspace", refreshedCapability.WorkspaceBaseDir.String)
	assert.JSONEq(t, `{"max_slots":1}`, string(refreshedCapability.Capacity))
	assert.JSONEq(t, `{"os":"linux"}`, string(refreshedCapability.Labels))
	assert.Equal(t, "degraded", refreshedCapability.HealthStatus)
	assert.JSONEq(t, `{"source":"refresh"}`, string(refreshedCapability.Metadata))

	_, err = testQueries.UpsertRuntimeCapability(ctx, queries.UpsertRuntimeCapabilityParams{
		TenantID:         otherTenantID,
		RuntimeNodeID:    node.ID,
		CapabilityType:   "provider",
		CapabilityKey:    "claude-code",
		ProviderType:     "claude-code",
		ProviderVersion:  pgtype.Text{String: "2.0.0", Valid: true},
		BinaryPath:       pgtype.Text{String: "/other/bin/claude", Valid: true},
		Available:        true,
		WorkspaceBaseDir: pgtype.Text{String: "/other/workspace", Valid: true},
		Capacity:         []byte(`{"max_slots":2}`),
		Labels:           []byte(`{"os":"linux"}`),
		Status:           "healthy",
		Details:          []byte(`{"source":"other-tenant"}`),
		HealthStatus:     "healthy",
		Metadata:         []byte(`{"source":"other"}`),
		LastSeenAt:       later,
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	persistedCapability, err := testQueries.GetRuntimeCapability(ctx, queries.GetRuntimeCapabilityParams{
		TenantID:       tenantID,
		RuntimeNodeID:  node.ID,
		CapabilityType: "provider",
		CapabilityKey:  "claude-code",
	})
	require.NoError(t, err)
	assert.Equal(t, refreshedCapability.ID, persistedCapability.ID)
	assert.Equal(t, "degraded", persistedCapability.Status)
	assert.JSONEq(t, `{"source":"refresh"}`, string(persistedCapability.Metadata))

	var otherTenantCapabilityCount int
	err = testDB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM runtime_capabilities
		WHERE tenant_id = $1
		  AND runtime_node_id = $2
		  AND capability_type = 'provider'
		  AND capability_key = 'claude-code'
	`, otherTenantID, node.ID).Scan(&otherTenantCapabilityCount)
	require.NoError(t, err)
	assert.Zero(t, otherTenantCapabilityCount)

	capabilities, err := testQueries.ListRuntimeCapabilities(ctx, queries.ListRuntimeCapabilitiesParams{
		TenantID:      tenantID,
		RuntimeNodeID: node.ID,
	})
	require.NoError(t, err)
	require.Len(t, capabilities, 1)
	assert.Equal(t, capability.ID, capabilities[0].ID)

	revokedApprovedEnrollment, err := testQueries.RevokeRuntimeEnrollment(ctx, queries.RevokeRuntimeEnrollmentParams{
		ID:           approved.ID,
		TenantID:     tenantID,
		RevokedBy:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
		RevokeReason: pgtype.Text{String: "administrator revoked enrollment", Valid: true},
	})
	require.NoError(t, err)
	assert.Equal(t, "revoked", revokedApprovedEnrollment.Status)
	require.True(t, revokedApprovedEnrollment.RevokedAt.Valid)

	_, err = testQueries.GetActiveRuntimeSessionByLookupHash(ctx, "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08")
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.RenewRuntimeSession(ctx, queries.RenewRuntimeSessionParams{
		ID:        session.ID,
		TenantID:  tenantID,
		ExpiresAt: renewedExpiresAt,
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.TouchRuntimeSessionLastSeen(ctx, queries.TouchRuntimeSessionLastSeenParams{
		ID:       session.ID,
		TenantID: tenantID,
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	var sessionRevokedAt pgtype.Timestamptz
	err = testDB.QueryRow(ctx, `
		SELECT revoked_at
		FROM runtime_sessions
		WHERE id = $1
		  AND tenant_id = $2
	`, session.ID, tenantID).Scan(&sessionRevokedAt)
	require.NoError(t, err)
	assert.True(t, sessionRevokedAt.Valid)
}

func TestDigitalEmployeeExecutionQueries(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	otherTenantID := uuid.MustParse("00000000-0000-0000-0000-000000000201")
	_, err := testDB.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, status)
		VALUES ($1, 'digital-employee-other', 'Digital Employee Other Tenant', 'active')
		ON CONFLICT (id) DO NOTHING
	`, otherTenantID)
	require.NoError(t, err)

	now := pgtype.Timestamptz{}
	require.NoError(t, now.Scan(time.Now()))
	node, err := testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "digital-employee-runtime-node",
		Name:               "Digital Employee Runtime Node",
		SupportedProviders: []byte(`{"providers":["claude-code"]}`),
		MaxSlots:           2,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           []byte(`{"region":"local"}`),
		LastHeartbeatAt:    now,
	})
	require.NoError(t, err)

	digitalBootstrapKeyExpiresAt := pgtype.Timestamptz{}
	require.NoError(t, digitalBootstrapKeyExpiresAt.Scan(time.Now().Add(24*time.Hour)))
	digitalBootstrapKey, err := testQueries.CreateRuntimeBootstrapKey(ctx, queries.CreateRuntimeBootstrapKeyParams{
		TenantID:    uuid.NullUUID{UUID: tenantID, Valid: true},
		Name:        "digital-employee-bootstrap",
		KeyHash:     "digital-employee-bootstrap-key-hash",
		Status:      "active",
		ExpiresAt:   digitalBootstrapKeyExpiresAt,
		CreatedBy:   uuid.NullUUID{UUID: uuid.New(), Valid: true},
		Description: pgtype.Text{String: "Digital employee runtime enrollment key", Valid: true},
		Metadata:    []byte(`{"environment":"digital-employee-test"}`),
	})
	require.NoError(t, err)

	employee, err := testQueries.CreateDigitalEmployee(ctx, queries.CreateDigitalEmployeeParams{
		TenantID:         tenantID,
		TeamID:           uuid.NullUUID{UUID: teamID, Valid: true},
		Name:             "需求分析数字员工",
		Role:             "requirements_analyst",
		Description:      pgtype.Text{String: "分析需求并产出结构化交接包", Valid: true},
		Status:           "draft",
		PermissionPolicy: []byte(`{"scope":"team"}`),
		ContextPolicy:    []byte(`{"memory":"task_slice"}`),
		ApprovalPolicy:   []byte(`{"high_risk":"human_required"}`),
		RiskLevel:        "normal",
		Metadata:         []byte(`{"owner":"qa"}`),
	})
	require.NoError(t, err)

	_, err = testQueries.UpsertDigitalEmployeeExecutionInstance(ctx, queries.UpsertDigitalEmployeeExecutionInstanceParams{
		TenantID:             tenantID,
		DigitalEmployeeID:    employee.ID,
		RuntimeNodeID:        node.ID,
		ProviderType:         "claude-code",
		AgentHomeDir:         "/data/superteam/workspaces/agents/no-approved-runtime",
		WorkspacePolicy:      []byte(`{"base_dir":"/data/superteam/workspaces"}`),
		SessionPolicy:        []byte(`{"mode":"reuse_latest"}`),
		RuntimeSelector:      []byte(`{"labels":{"os":"darwin"}}`),
		CapacityRequirements: []byte(`{"slots":1}`),
		FallbackPolicy:       []byte(`{"enabled":false}`),
		Status:               "provisioning",
		Metadata:             []byte(`{"source":"no-approved-runtime"}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	enrollment, err := testQueries.UpsertRuntimeEnrollment(ctx, queries.UpsertRuntimeEnrollmentParams{
		TenantID:       tenantID,
		NodeID:         "digital-employee-runtime-node",
		BootstrapKeyID: digitalBootstrapKey.ID,
		RequestPayload: []byte(`{"node_id":"digital-employee-runtime-node","version":"0.1.0"}`),
		LastHelloAt:    now,
	})
	require.NoError(t, err)

	_, err = testQueries.UpsertRuntimeCapability(ctx, queries.UpsertRuntimeCapabilityParams{
		TenantID:         tenantID,
		RuntimeNodeID:    node.ID,
		CapabilityType:   "provider",
		CapabilityKey:    "claude-code",
		ProviderType:     "claude-code",
		ProviderVersion:  pgtype.Text{String: "1.0.0", Valid: true},
		BinaryPath:       pgtype.Text{String: "/usr/local/bin/claude", Valid: true},
		Available:        true,
		WorkspaceBaseDir: pgtype.Text{String: "/data/superteam/workspaces", Valid: true},
		Capacity:         []byte(`{"max_slots":2}`),
		Labels:           []byte(`{"os":"darwin"}`),
		Status:           "healthy",
		Details:          []byte(`{"source":"digital-employee-test"}`),
		HealthStatus:     "healthy",
		Metadata:         []byte(`{"source":"capability-report"}`),
		LastSeenAt:       now,
	})
	require.NoError(t, err)

	_, err = testQueries.UpsertDigitalEmployeeExecutionInstance(ctx, queries.UpsertDigitalEmployeeExecutionInstanceParams{
		TenantID:             tenantID,
		DigitalEmployeeID:    employee.ID,
		RuntimeNodeID:        node.ID,
		ProviderType:         "claude-code",
		AgentHomeDir:         "/data/superteam/workspaces/agents/pending-runtime",
		WorkspacePolicy:      []byte(`{"base_dir":"/data/superteam/workspaces"}`),
		SessionPolicy:        []byte(`{"mode":"reuse_latest"}`),
		RuntimeSelector:      []byte(`{"labels":{"os":"darwin"}}`),
		CapacityRequirements: []byte(`{"slots":1}`),
		FallbackPolicy:       []byte(`{"enabled":false}`),
		Status:               "provisioning",
		Metadata:             []byte(`{"source":"pending-runtime"}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	approvedEnrollment, err := testQueries.ApproveRuntimeEnrollment(ctx, queries.ApproveRuntimeEnrollmentParams{
		RuntimeNodeID: node.ID,
		ID:            enrollment.ID,
		TenantID:      tenantID,
		ApprovedBy:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
	})
	require.NoError(t, err)

	_, err = testQueries.UpsertRuntimeCapability(ctx, queries.UpsertRuntimeCapabilityParams{
		TenantID:         tenantID,
		RuntimeNodeID:    node.ID,
		CapabilityType:   "provider",
		CapabilityKey:    "claude-code",
		ProviderType:     "claude-code",
		ProviderVersion:  pgtype.Text{String: "1.0.0", Valid: true},
		BinaryPath:       pgtype.Text{String: "/usr/local/bin/claude", Valid: true},
		Available:        false,
		WorkspaceBaseDir: pgtype.Text{String: "/data/superteam/workspaces", Valid: true},
		Capacity:         []byte(`{"max_slots":0}`),
		Labels:           []byte(`{"os":"darwin"}`),
		Status:           "degraded",
		Details:          []byte(`{"reason":"provider_unavailable"}`),
		HealthStatus:     "degraded",
		Metadata:         []byte(`{"source":"capability-report"}`),
		LastSeenAt:       now,
	})
	require.NoError(t, err)

	_, err = testQueries.UpsertDigitalEmployeeExecutionInstance(ctx, queries.UpsertDigitalEmployeeExecutionInstanceParams{
		TenantID:             tenantID,
		DigitalEmployeeID:    employee.ID,
		RuntimeNodeID:        node.ID,
		ProviderType:         "claude-code",
		AgentHomeDir:         "/data/superteam/workspaces/agents/unavailable-provider",
		WorkspacePolicy:      []byte(`{"base_dir":"/data/superteam/workspaces"}`),
		SessionPolicy:        []byte(`{"mode":"reuse_latest"}`),
		RuntimeSelector:      []byte(`{"labels":{"os":"darwin"}}`),
		CapacityRequirements: []byte(`{"slots":1}`),
		FallbackPolicy:       []byte(`{"enabled":false}`),
		Status:               "provisioning",
		Metadata:             []byte(`{"source":"unavailable-provider"}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.UpsertRuntimeCapability(ctx, queries.UpsertRuntimeCapabilityParams{
		TenantID:         tenantID,
		RuntimeNodeID:    node.ID,
		CapabilityType:   "provider",
		CapabilityKey:    "claude-code",
		ProviderType:     "claude-code",
		ProviderVersion:  pgtype.Text{String: "1.0.0", Valid: true},
		BinaryPath:       pgtype.Text{String: "/usr/local/bin/claude", Valid: true},
		Available:        true,
		WorkspaceBaseDir: pgtype.Text{String: "/data/superteam/workspaces", Valid: true},
		Capacity:         []byte(`{"max_slots":2}`),
		Labels:           []byte(`{"os":"darwin"}`),
		Status:           "healthy",
		Details:          []byte(`{"source":"digital-employee-test"}`),
		HealthStatus:     "healthy",
		Metadata:         []byte(`{"source":"capability-report"}`),
		LastSeenAt:       now,
	})
	require.NoError(t, err)

	_, err = testQueries.UpdateDigitalEmployeeStatus(ctx, queries.UpdateDigitalEmployeeStatusParams{
		ID:       employee.ID,
		TenantID: tenantID,
		Status:   "disabled",
	})
	require.NoError(t, err)
	_, err = testQueries.UpsertDigitalEmployeeExecutionInstance(ctx, queries.UpsertDigitalEmployeeExecutionInstanceParams{
		TenantID:             tenantID,
		DigitalEmployeeID:    employee.ID,
		RuntimeNodeID:        node.ID,
		ProviderType:         "claude-code",
		AgentHomeDir:         "/data/superteam/workspaces/agents/disabled-employee-upsert",
		WorkspacePolicy:      []byte(`{"base_dir":"/data/superteam/workspaces"}`),
		SessionPolicy:        []byte(`{"mode":"reuse_latest"}`),
		RuntimeSelector:      []byte(`{"labels":{"os":"darwin"}}`),
		CapacityRequirements: []byte(`{"slots":1}`),
		FallbackPolicy:       []byte(`{"enabled":false}`),
		Status:               "provisioning",
		Metadata:             []byte(`{"source":"disabled-employee-upsert"}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.UpdateDigitalEmployeeStatus(ctx, queries.UpdateDigitalEmployeeStatusParams{
		ID:       employee.ID,
		TenantID: tenantID,
		Status:   "error",
	})
	require.NoError(t, err)
	_, err = testQueries.UpsertDigitalEmployeeExecutionInstance(ctx, queries.UpsertDigitalEmployeeExecutionInstanceParams{
		TenantID:             tenantID,
		DigitalEmployeeID:    employee.ID,
		RuntimeNodeID:        node.ID,
		ProviderType:         "claude-code",
		AgentHomeDir:         "/data/superteam/workspaces/agents/error-employee-upsert",
		WorkspacePolicy:      []byte(`{"base_dir":"/data/superteam/workspaces"}`),
		SessionPolicy:        []byte(`{"mode":"reuse_latest"}`),
		RuntimeSelector:      []byte(`{"labels":{"os":"darwin"}}`),
		CapacityRequirements: []byte(`{"slots":1}`),
		FallbackPolicy:       []byte(`{"enabled":false}`),
		Status:               "provisioning",
		Metadata:             []byte(`{"source":"error-employee-upsert"}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.UpdateDigitalEmployeeStatus(ctx, queries.UpdateDigitalEmployeeStatusParams{
		ID:       employee.ID,
		TenantID: tenantID,
		Status:   "draft",
	})
	require.NoError(t, err)

	runtimeSessionExpiresAt := pgtype.Timestamptz{}
	require.NoError(t, runtimeSessionExpiresAt.Scan(time.Now().Add(12*time.Hour)))
	runtimeSession, err := testQueries.CreateRuntimeSession(ctx, queries.CreateRuntimeSessionParams{
		TenantID:        tenantID,
		RuntimeNodeID:   node.ID,
		EnrollmentID:    uuid.NullUUID{UUID: approvedEnrollment.ID, Valid: true},
		TokenLookupHash: "b2c6249cd92c1aee9a06844a5c3f451a8dd76e6ac8f7fd599a7445a027fbc617",
		TokenSecretHash: "$2a$10$digitalEmployeeRuntimeSessionHash",
		ExpiresAt:       runtimeSessionExpiresAt,
	})
	require.NoError(t, err)

	instance, err := testQueries.UpsertDigitalEmployeeExecutionInstance(ctx, queries.UpsertDigitalEmployeeExecutionInstanceParams{
		TenantID:             tenantID,
		DigitalEmployeeID:    employee.ID,
		RuntimeNodeID:        node.ID,
		ProviderType:         "claude-code",
		AgentHomeDir:         "/data/superteam/workspaces/agents/requirements-analyst",
		WorkspacePolicy:      []byte(`{"base_dir":"/data/superteam/workspaces"}`),
		SessionPolicy:        []byte(`{"mode":"reuse_latest"}`),
		RuntimeSelector:      []byte(`{"labels":{"os":"darwin"}}`),
		CapacityRequirements: []byte(`{"slots":1}`),
		FallbackPolicy:       []byte(`{"enabled":false}`),
		Status:               "provisioning",
		Metadata:             []byte(`{"source":"web"}`),
	})
	require.NoError(t, err)
	assert.Equal(t, employee.ID, instance.DigitalEmployeeID)

	updatedInstance, err := testQueries.UpsertDigitalEmployeeExecutionInstance(ctx, queries.UpsertDigitalEmployeeExecutionInstanceParams{
		TenantID:             tenantID,
		DigitalEmployeeID:    employee.ID,
		RuntimeNodeID:        node.ID,
		ProviderType:         "claude-code",
		AgentHomeDir:         "/data/superteam/workspaces/agents/requirements-analyst-updated",
		WorkspacePolicy:      []byte(`{"base_dir":"/data/superteam/workspaces","mode":"updated"}`),
		SessionPolicy:        []byte(`{"mode":"new"}`),
		RuntimeSelector:      []byte(`{"labels":{"tier":"gpu"}}`),
		CapacityRequirements: []byte(`{"slots":2}`),
		FallbackPolicy:       []byte(`{"enabled":true}`),
		Status:               "provisioning",
		Metadata:             []byte(`{"source":"web-update"}`),
	})
	require.NoError(t, err)
	assert.Equal(t, instance.ID, updatedInstance.ID)
	assert.Equal(t, "/data/superteam/workspaces/agents/requirements-analyst-updated", updatedInstance.AgentHomeDir)
	assert.JSONEq(t, `{"mode":"new"}`, string(updatedInstance.SessionPolicy))

	_, err = testQueries.UpsertDigitalEmployeeExecutionInstance(ctx, queries.UpsertDigitalEmployeeExecutionInstanceParams{
		TenantID:             otherTenantID,
		DigitalEmployeeID:    employee.ID,
		RuntimeNodeID:        node.ID,
		ProviderType:         "claude-code",
		AgentHomeDir:         "/data/superteam/workspaces/agents/wrong-tenant",
		WorkspacePolicy:      []byte(`{"base_dir":"/wrong-tenant"}`),
		SessionPolicy:        []byte(`{"mode":"ephemeral"}`),
		RuntimeSelector:      []byte(`{"labels":{"tenant":"other"}}`),
		CapacityRequirements: []byte(`{"slots":3}`),
		FallbackPolicy:       []byte(`{"enabled":false}`),
		Status:               "provisioning",
		Metadata:             []byte(`{"source":"wrong-tenant"}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	tenantInstance, err := testQueries.GetDigitalEmployeeExecutionInstanceByEmployeeID(ctx, queries.GetDigitalEmployeeExecutionInstanceByEmployeeIDParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employee.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, updatedInstance.ID, tenantInstance.ID)
	assert.Equal(t, "/data/superteam/workspaces/agents/requirements-analyst-updated", tenantInstance.AgentHomeDir)

	var otherTenantInstanceCount int
	err = testDB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM digital_employee_execution_instances
		WHERE tenant_id = $1
		  AND digital_employee_id = $2
		  AND deleted_at IS NULL
	`, otherTenantID, employee.ID).Scan(&otherTenantInstanceCount)
	require.NoError(t, err)
	assert.Zero(t, otherTenantInstanceCount)

	readyInstance, err := testQueries.UpdateDigitalEmployeeExecutionInstanceStatus(ctx, queries.UpdateDigitalEmployeeExecutionInstanceStatusParams{
		ID:       instance.ID,
		TenantID: tenantID,
		Status:   "ready",
	})
	require.NoError(t, err)
	assert.Equal(t, "ready", readyInstance.Status)
	require.True(t, readyInstance.ReadyAt.Valid)

	activeEmployee, err := testQueries.UpdateDigitalEmployeeStatus(ctx, queries.UpdateDigitalEmployeeStatusParams{
		ID:       employee.ID,
		TenantID: tenantID,
		Status:   "active",
	})
	require.NoError(t, err)
	assert.Equal(t, "active", activeEmployee.Status)

	_, err = testQueries.UpdateDigitalEmployeeExecutionInstanceStatus(ctx, queries.UpdateDigitalEmployeeExecutionInstanceStatusParams{
		ID:       instance.ID,
		TenantID: tenantID,
		Status:   "provisioning",
	})
	require.NoError(t, err)
	_, err = testQueries.CreateProviderSession(ctx, queries.CreateProviderSessionParams{
		TenantID:            tenantID,
		ProviderSessionID:   "claude-session-provisioning-instance",
		DigitalEmployeeID:   employee.ID,
		ExecutionInstanceID: instance.ID,
		RuntimeNodeID:       node.ID,
		ProviderType:        "claude-code",
		Status:              "running",
		Recoverable:         true,
		LastActiveAt:        now,
		Metadata:            []byte(`{"mode":"provisioning-instance"}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.UpdateDigitalEmployeeExecutionInstanceStatus(ctx, queries.UpdateDigitalEmployeeExecutionInstanceStatusParams{
		ID:       instance.ID,
		TenantID: tenantID,
		Status:   "disabled",
	})
	require.NoError(t, err)
	_, err = testQueries.CreateProviderSession(ctx, queries.CreateProviderSessionParams{
		TenantID:            tenantID,
		ProviderSessionID:   "claude-session-disabled-instance",
		DigitalEmployeeID:   employee.ID,
		ExecutionInstanceID: instance.ID,
		RuntimeNodeID:       node.ID,
		ProviderType:        "claude-code",
		Status:              "running",
		Recoverable:         true,
		LastActiveAt:        now,
		Metadata:            []byte(`{"mode":"disabled-instance"}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.UpdateDigitalEmployeeExecutionInstanceStatus(ctx, queries.UpdateDigitalEmployeeExecutionInstanceStatusParams{
		ID:           instance.ID,
		TenantID:     tenantID,
		Status:       "error",
		ErrorMessage: pgtype.Text{String: "provider provisioning failed", Valid: true},
	})
	require.NoError(t, err)
	_, err = testQueries.CreateProviderSession(ctx, queries.CreateProviderSessionParams{
		TenantID:            tenantID,
		ProviderSessionID:   "claude-session-error-instance",
		DigitalEmployeeID:   employee.ID,
		ExecutionInstanceID: instance.ID,
		RuntimeNodeID:       node.ID,
		ProviderType:        "claude-code",
		Status:              "running",
		Recoverable:         true,
		LastActiveAt:        now,
		Metadata:            []byte(`{"mode":"error-instance"}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	readyInstance, err = testQueries.UpdateDigitalEmployeeExecutionInstanceStatus(ctx, queries.UpdateDigitalEmployeeExecutionInstanceStatusParams{
		ID:       instance.ID,
		TenantID: tenantID,
		Status:   "ready",
	})
	require.NoError(t, err)
	assert.Equal(t, "ready", readyInstance.Status)

	_, err = testQueries.UpdateDigitalEmployeeStatus(ctx, queries.UpdateDigitalEmployeeStatusParams{
		ID:       employee.ID,
		TenantID: tenantID,
		Status:   "disabled",
	})
	require.NoError(t, err)
	_, err = testQueries.CreateProviderSession(ctx, queries.CreateProviderSessionParams{
		TenantID:            tenantID,
		ProviderSessionID:   "claude-session-disabled-employee",
		DigitalEmployeeID:   employee.ID,
		ExecutionInstanceID: instance.ID,
		RuntimeNodeID:       node.ID,
		ProviderType:        "claude-code",
		Status:              "running",
		Recoverable:         true,
		LastActiveAt:        now,
		Metadata:            []byte(`{"mode":"disabled-employee"}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.UpdateDigitalEmployeeStatus(ctx, queries.UpdateDigitalEmployeeStatusParams{
		ID:       employee.ID,
		TenantID: tenantID,
		Status:   "error",
	})
	require.NoError(t, err)
	_, err = testQueries.CreateProviderSession(ctx, queries.CreateProviderSessionParams{
		TenantID:            tenantID,
		ProviderSessionID:   "claude-session-error-employee",
		DigitalEmployeeID:   employee.ID,
		ExecutionInstanceID: instance.ID,
		RuntimeNodeID:       node.ID,
		ProviderType:        "claude-code",
		Status:              "running",
		Recoverable:         true,
		LastActiveAt:        now,
		Metadata:            []byte(`{"mode":"error-employee"}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	activeEmployee, err = testQueries.UpdateDigitalEmployeeStatus(ctx, queries.UpdateDigitalEmployeeStatusParams{
		ID:       employee.ID,
		TenantID: tenantID,
		Status:   "active",
	})
	require.NoError(t, err)
	assert.Equal(t, "active", activeEmployee.Status)

	_, err = testDB.Exec(ctx, `
		UPDATE runtime_enrollments
		SET status = 'pending',
		    revoked_at = NULL,
		    rejected_at = NULL,
		    updated_at = NOW()
		WHERE id = $1
		  AND tenant_id = $2
	`, approvedEnrollment.ID, tenantID)
	require.NoError(t, err)
	_, err = testQueries.CreateProviderSession(ctx, queries.CreateProviderSessionParams{
		TenantID:            tenantID,
		ProviderSessionID:   "claude-session-pending-enrollment",
		DigitalEmployeeID:   employee.ID,
		ExecutionInstanceID: instance.ID,
		RuntimeNodeID:       node.ID,
		ProviderType:        "claude-code",
		Status:              "running",
		Recoverable:         true,
		LastActiveAt:        now,
		Metadata:            []byte(`{"mode":"pending-enrollment"}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testDB.Exec(ctx, `
		UPDATE runtime_enrollments
		SET status = 'revoked',
		    revoked_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1
		  AND tenant_id = $2
	`, approvedEnrollment.ID, tenantID)
	require.NoError(t, err)
	_, err = testQueries.CreateProviderSession(ctx, queries.CreateProviderSessionParams{
		TenantID:            tenantID,
		ProviderSessionID:   "claude-session-revoked-enrollment",
		DigitalEmployeeID:   employee.ID,
		ExecutionInstanceID: instance.ID,
		RuntimeNodeID:       node.ID,
		ProviderType:        "claude-code",
		Status:              "running",
		Recoverable:         true,
		LastActiveAt:        now,
		Metadata:            []byte(`{"mode":"revoked-enrollment"}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testDB.Exec(ctx, `
		UPDATE runtime_enrollments
		SET status = 'approved',
		    revoked_at = NULL,
		    rejected_at = NULL,
		    updated_at = NOW()
		WHERE id = $1
		  AND tenant_id = $2
	`, approvedEnrollment.ID, tenantID)
	require.NoError(t, err)

	_, err = testQueries.UpdateRuntimeNodeStatus(ctx, queries.UpdateRuntimeNodeStatusParams{
		NodeID: node.NodeID,
		Status: "offline",
	})
	require.NoError(t, err)
	_, err = testQueries.CreateProviderSession(ctx, queries.CreateProviderSessionParams{
		TenantID:            tenantID,
		ProviderSessionID:   "claude-session-offline-runtime",
		DigitalEmployeeID:   employee.ID,
		ExecutionInstanceID: instance.ID,
		RuntimeNodeID:       node.ID,
		ProviderType:        "claude-code",
		Status:              "running",
		Recoverable:         true,
		LastActiveAt:        now,
		Metadata:            []byte(`{"mode":"offline-runtime"}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.UpdateRuntimeNodeStatus(ctx, queries.UpdateRuntimeNodeStatusParams{
		NodeID: node.NodeID,
		Status: "online",
	})
	require.NoError(t, err)
	_, err = testDB.Exec(ctx, `
		UPDATE runtime_nodes
		SET disabled_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1
	`, node.ID)
	require.NoError(t, err)
	_, err = testQueries.CreateProviderSession(ctx, queries.CreateProviderSessionParams{
		TenantID:            tenantID,
		ProviderSessionID:   "claude-session-disabled-runtime",
		DigitalEmployeeID:   employee.ID,
		ExecutionInstanceID: instance.ID,
		RuntimeNodeID:       node.ID,
		ProviderType:        "claude-code",
		Status:              "running",
		Recoverable:         true,
		LastActiveAt:        now,
		Metadata:            []byte(`{"mode":"disabled-runtime"}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testDB.Exec(ctx, `
		UPDATE runtime_nodes
		SET disabled_at = NULL,
		    updated_at = NOW()
		WHERE id = $1
	`, node.ID)
	require.NoError(t, err)

	_, err = testDB.Exec(ctx, `
		UPDATE runtime_sessions
		SET expires_at = NOW() - INTERVAL '1 hour',
		    updated_at = NOW()
		WHERE id = $1
		  AND tenant_id = $2
	`, runtimeSession.ID, tenantID)
	require.NoError(t, err)
	_, err = testQueries.CreateProviderSession(ctx, queries.CreateProviderSessionParams{
		TenantID:            tenantID,
		ProviderSessionID:   "claude-session-expired-runtime-session",
		DigitalEmployeeID:   employee.ID,
		ExecutionInstanceID: instance.ID,
		RuntimeNodeID:       node.ID,
		ProviderType:        "claude-code",
		Status:              "running",
		Recoverable:         true,
		LastActiveAt:        now,
		Metadata:            []byte(`{"mode":"expired-runtime-session"}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testDB.Exec(ctx, `
		UPDATE runtime_sessions
		SET expires_at = $3,
		    updated_at = NOW()
		WHERE id = $1
		  AND tenant_id = $2
	`, runtimeSession.ID, tenantID, runtimeSessionExpiresAt)
	require.NoError(t, err)

	_, err = testQueries.RevokeRuntimeSession(ctx, queries.RevokeRuntimeSessionParams{
		ID:            runtimeSession.ID,
		TenantID:      tenantID,
		RevokedReason: pgtype.Text{String: "runtime stopped before provider session create", Valid: true},
	})
	require.NoError(t, err)
	_, err = testQueries.CreateProviderSession(ctx, queries.CreateProviderSessionParams{
		TenantID:            tenantID,
		ProviderSessionID:   "claude-session-revoked-runtime-session",
		DigitalEmployeeID:   employee.ID,
		ExecutionInstanceID: instance.ID,
		RuntimeNodeID:       node.ID,
		ProviderType:        "claude-code",
		Status:              "running",
		Recoverable:         true,
		LastActiveAt:        now,
		Metadata:            []byte(`{"mode":"revoked-runtime-session"}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	runtimeSession, err = testQueries.CreateRuntimeSession(ctx, queries.CreateRuntimeSessionParams{
		TenantID:        tenantID,
		RuntimeNodeID:   node.ID,
		EnrollmentID:    uuid.NullUUID{UUID: approvedEnrollment.ID, Valid: true},
		TokenLookupHash: "641e92a38e2ec02749c9e47c409af6befb86bc0f89ddd65317cdd3a425926d01",
		TokenSecretHash: "$2a$10$digitalEmployeeRuntimeSessionHashForProviderSession",
		ExpiresAt:       runtimeSessionExpiresAt,
	})
	require.NoError(t, err)

	session, err := testQueries.CreateProviderSession(ctx, queries.CreateProviderSessionParams{
		TenantID:            tenantID,
		ProviderSessionID:   "claude-session-001",
		DigitalEmployeeID:   employee.ID,
		ExecutionInstanceID: instance.ID,
		RuntimeNodeID:       node.ID,
		ProviderType:        "claude-code",
		Status:              "running",
		Recoverable:         true,
		LastActiveAt:        now,
		Metadata:            []byte(`{"mode":"resume"}`),
	})
	require.NoError(t, err)

	_, err = testQueries.CreateProviderSession(ctx, queries.CreateProviderSessionParams{
		TenantID:            otherTenantID,
		ProviderSessionID:   "claude-session-wrong-tenant",
		DigitalEmployeeID:   employee.ID,
		ExecutionInstanceID: instance.ID,
		RuntimeNodeID:       node.ID,
		ProviderType:        "claude-code",
		Status:              "running",
		Recoverable:         true,
		LastActiveAt:        now,
		Metadata:            []byte(`{"mode":"wrong-tenant"}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.CreateProviderSession(ctx, queries.CreateProviderSessionParams{
		TenantID:            tenantID,
		ProviderSessionID:   "claude-session-mismatched-runtime",
		DigitalEmployeeID:   employee.ID,
		ExecutionInstanceID: instance.ID,
		RuntimeNodeID:       uuid.New(),
		ProviderType:        "claude-code",
		Status:              "running",
		Recoverable:         true,
		LastActiveAt:        now,
		Metadata:            []byte(`{"mode":"mismatched-runtime"}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.CreateProviderSession(ctx, queries.CreateProviderSessionParams{
		TenantID:            tenantID,
		ProviderSessionID:   "claude-session-mismatched-provider",
		DigitalEmployeeID:   employee.ID,
		ExecutionInstanceID: instance.ID,
		RuntimeNodeID:       node.ID,
		ProviderType:        "codex",
		Status:              "running",
		Recoverable:         true,
		LastActiveAt:        now,
		Metadata:            []byte(`{"mode":"mismatched-provider"}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	var pollutedProviderSessionCount int
	err = testDB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM provider_sessions
		WHERE provider_session_id IN (
			'claude-session-wrong-tenant',
			'claude-session-mismatched-runtime',
			'claude-session-mismatched-provider',
			'claude-session-provisioning-instance',
			'claude-session-pending-enrollment',
			'claude-session-revoked-enrollment',
			'claude-session-offline-runtime',
			'claude-session-disabled-runtime',
			'claude-session-expired-runtime-session',
			'claude-session-revoked-runtime-session'
		)
	`).Scan(&pollutedProviderSessionCount)
	require.NoError(t, err)
	assert.Zero(t, pollutedProviderSessionCount)

	eventPayload1 := []byte(`{"message":"session started"}`)
	event1, err := testQueries.CreateProviderSessionEvent(ctx, queries.CreateProviderSessionEventParams{
		TenantID:          tenantID,
		ProviderSessionID: session.ID,
		NodeID:            node.NodeID,
		EventType:         "message_delta",
		SequenceNumber:    1,
		Payload:           eventPayload1,
		RequestID:         pgtype.Text{String: "request-001", Valid: true},
		CommandID:         pgtype.Text{String: "command-001", Valid: true},
		RawEventRef:       pgtype.Text{String: "s3://superteam/raw/session-001/1.json", Valid: true},
		Metadata:          []byte(`{"channel":"stdout"}`),
	})
	require.NoError(t, err)
	assert.Equal(t, employee.ID, event1.DigitalEmployeeID)
	assert.Equal(t, instance.ID, event1.ExecutionInstanceID)
	assert.Equal(t, node.ID, event1.RuntimeNodeID)
	assert.Equal(t, "claude-code", event1.ProviderType)
	assert.Equal(t, "request-001", event1.RequestID.String)
	assert.Equal(t, "command-001", event1.CommandID.String)

	eventPayload2 := []byte(`{"tool":"write_file"}`)
	_, err = testQueries.CreateProviderSessionEvent(ctx, queries.CreateProviderSessionEventParams{
		TenantID:          tenantID,
		ProviderSessionID: session.ID,
		NodeID:            node.NodeID,
		EventType:         "tool_call",
		SequenceNumber:    2,
		Payload:           eventPayload2,
		Metadata:          []byte(`{"channel":"json_stream"}`),
	})
	assert.Error(t, err)

	_, err = testQueries.CreateProviderSessionEvent(ctx, queries.CreateProviderSessionEventParams{
		TenantID:          tenantID,
		ProviderSessionID: session.ID,
		NodeID:            node.NodeID,
		EventType:         "tool_call",
		SequenceNumber:    2,
		Payload:           eventPayload2,
		CommandID:         pgtype.Text{String: "command-002", Valid: true},
		Metadata:          []byte(`{"channel":"json_stream"}`),
	})
	require.NoError(t, err)

	_, err = testQueries.UpdateDigitalEmployeeStatus(ctx, queries.UpdateDigitalEmployeeStatusParams{
		ID:       employee.ID,
		TenantID: tenantID,
		Status:   "disabled",
	})
	require.NoError(t, err)
	_, err = testQueries.CreateProviderSessionEvent(ctx, queries.CreateProviderSessionEventParams{
		TenantID:          tenantID,
		ProviderSessionID: session.ID,
		NodeID:            node.NodeID,
		EventType:         "message_delta",
		SequenceNumber:    3,
		Payload:           []byte(`{"message":"disabled employee"}`),
		RequestID:         pgtype.Text{String: "request-disabled-employee-event", Valid: true},
		Metadata:          []byte(`{}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.UpdateDigitalEmployeeStatus(ctx, queries.UpdateDigitalEmployeeStatusParams{
		ID:       employee.ID,
		TenantID: tenantID,
		Status:   "error",
	})
	require.NoError(t, err)
	_, err = testQueries.CreateProviderSessionEvent(ctx, queries.CreateProviderSessionEventParams{
		TenantID:          tenantID,
		ProviderSessionID: session.ID,
		NodeID:            node.NodeID,
		EventType:         "message_delta",
		SequenceNumber:    4,
		Payload:           []byte(`{"message":"error employee"}`),
		RequestID:         pgtype.Text{String: "request-error-employee-event", Valid: true},
		Metadata:          []byte(`{}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.UpdateDigitalEmployeeStatus(ctx, queries.UpdateDigitalEmployeeStatusParams{
		ID:       employee.ID,
		TenantID: tenantID,
		Status:   "active",
	})
	require.NoError(t, err)

	_, err = testQueries.UpdateDigitalEmployeeExecutionInstanceStatus(ctx, queries.UpdateDigitalEmployeeExecutionInstanceStatusParams{
		ID:       instance.ID,
		TenantID: tenantID,
		Status:   "provisioning",
	})
	require.NoError(t, err)
	_, err = testQueries.CreateProviderSessionEvent(ctx, queries.CreateProviderSessionEventParams{
		TenantID:          tenantID,
		ProviderSessionID: session.ID,
		NodeID:            node.NodeID,
		EventType:         "message_delta",
		SequenceNumber:    5,
		Payload:           []byte(`{"message":"provisioning execution instance"}`),
		RequestID:         pgtype.Text{String: "request-provisioning-instance-event", Valid: true},
		Metadata:          []byte(`{}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.UpdateDigitalEmployeeExecutionInstanceStatus(ctx, queries.UpdateDigitalEmployeeExecutionInstanceStatusParams{
		ID:       instance.ID,
		TenantID: tenantID,
		Status:   "disabled",
	})
	require.NoError(t, err)
	_, err = testQueries.CreateProviderSessionEvent(ctx, queries.CreateProviderSessionEventParams{
		TenantID:          tenantID,
		ProviderSessionID: session.ID,
		NodeID:            node.NodeID,
		EventType:         "message_delta",
		SequenceNumber:    6,
		Payload:           []byte(`{"message":"disabled execution instance"}`),
		RequestID:         pgtype.Text{String: "request-disabled-instance-event", Valid: true},
		Metadata:          []byte(`{}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.UpdateDigitalEmployeeExecutionInstanceStatus(ctx, queries.UpdateDigitalEmployeeExecutionInstanceStatusParams{
		ID:           instance.ID,
		TenantID:     tenantID,
		Status:       "error",
		ErrorMessage: pgtype.Text{String: "provider became unavailable", Valid: true},
	})
	require.NoError(t, err)
	_, err = testQueries.CreateProviderSessionEvent(ctx, queries.CreateProviderSessionEventParams{
		TenantID:          tenantID,
		ProviderSessionID: session.ID,
		NodeID:            node.NodeID,
		EventType:         "message_delta",
		SequenceNumber:    7,
		Payload:           []byte(`{"message":"error execution instance"}`),
		RequestID:         pgtype.Text{String: "request-error-instance-event", Valid: true},
		Metadata:          []byte(`{}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.UpdateDigitalEmployeeExecutionInstanceStatus(ctx, queries.UpdateDigitalEmployeeExecutionInstanceStatusParams{
		ID:       instance.ID,
		TenantID: tenantID,
		Status:   "ready",
	})
	require.NoError(t, err)

	stoppedSession, err := testQueries.CreateProviderSession(ctx, queries.CreateProviderSessionParams{
		TenantID:            tenantID,
		ProviderSessionID:   "claude-session-stopped-event-context",
		DigitalEmployeeID:   employee.ID,
		ExecutionInstanceID: instance.ID,
		RuntimeNodeID:       node.ID,
		ProviderType:        "claude-code",
		Status:              "running",
		Recoverable:         true,
		LastActiveAt:        now,
		Metadata:            []byte(`{"mode":"stopped-event-context"}`),
	})
	require.NoError(t, err)
	_, err = testQueries.UpdateProviderSessionStatus(ctx, queries.UpdateProviderSessionStatusParams{
		ID:       stoppedSession.ID,
		TenantID: tenantID,
		Status:   "stopped",
	})
	require.NoError(t, err)
	_, err = testQueries.CreateProviderSessionEvent(ctx, queries.CreateProviderSessionEventParams{
		TenantID:          tenantID,
		ProviderSessionID: stoppedSession.ID,
		NodeID:            node.NodeID,
		EventType:         "message_delta",
		SequenceNumber:    1,
		Payload:           []byte(`{"message":"stopped provider session"}`),
		RequestID:         pgtype.Text{String: "request-stopped-provider-session-event", Valid: true},
		Metadata:          []byte(`{}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	completedSession, err := testQueries.CreateProviderSession(ctx, queries.CreateProviderSessionParams{
		TenantID:            tenantID,
		ProviderSessionID:   "claude-session-completed-event-context",
		DigitalEmployeeID:   employee.ID,
		ExecutionInstanceID: instance.ID,
		RuntimeNodeID:       node.ID,
		ProviderType:        "claude-code",
		Status:              "running",
		Recoverable:         true,
		LastActiveAt:        now,
		Metadata:            []byte(`{"mode":"completed-event-context"}`),
	})
	require.NoError(t, err)
	_, err = testQueries.UpdateProviderSessionStatus(ctx, queries.UpdateProviderSessionStatusParams{
		ID:       completedSession.ID,
		TenantID: tenantID,
		Status:   "completed",
	})
	require.NoError(t, err)
	_, err = testQueries.CreateProviderSessionEvent(ctx, queries.CreateProviderSessionEventParams{
		TenantID:          tenantID,
		ProviderSessionID: completedSession.ID,
		NodeID:            node.NodeID,
		EventType:         "message_delta",
		SequenceNumber:    1,
		Payload:           []byte(`{"message":"completed provider session"}`),
		RequestID:         pgtype.Text{String: "request-completed-provider-session-event", Valid: true},
		Metadata:          []byte(`{}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	failedSession, err := testQueries.CreateProviderSession(ctx, queries.CreateProviderSessionParams{
		TenantID:            tenantID,
		ProviderSessionID:   "claude-session-failed-event-context",
		DigitalEmployeeID:   employee.ID,
		ExecutionInstanceID: instance.ID,
		RuntimeNodeID:       node.ID,
		ProviderType:        "claude-code",
		Status:              "running",
		Recoverable:         true,
		LastActiveAt:        now,
		Metadata:            []byte(`{"mode":"failed-event-context"}`),
	})
	require.NoError(t, err)
	_, err = testQueries.UpdateProviderSessionStatus(ctx, queries.UpdateProviderSessionStatusParams{
		ID:           failedSession.ID,
		TenantID:     tenantID,
		Status:       "failed",
		ErrorMessage: pgtype.Text{String: "provider session failed", Valid: true},
	})
	require.NoError(t, err)
	_, err = testQueries.CreateProviderSessionEvent(ctx, queries.CreateProviderSessionEventParams{
		TenantID:          tenantID,
		ProviderSessionID: failedSession.ID,
		NodeID:            node.NodeID,
		EventType:         "message_delta",
		SequenceNumber:    1,
		Payload:           []byte(`{"message":"failed provider session"}`),
		RequestID:         pgtype.Text{String: "request-failed-provider-session-event", Valid: true},
		Metadata:          []byte(`{}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.CreateProviderSessionEvent(ctx, queries.CreateProviderSessionEventParams{
		TenantID:          tenantID,
		ProviderSessionID: session.ID,
		NodeID:            node.NodeID,
		EventType:         "message_delta",
		SequenceNumber:    3,
		Payload:           []byte(`{"message":"empty correlation"}`),
		RequestID:         pgtype.Text{String: "", Valid: true},
		CommandID:         pgtype.Text{String: "", Valid: true},
		Metadata:          []byte(`{}`),
	})
	assert.Error(t, err)

	_, err = testQueries.CreateProviderSessionEvent(ctx, queries.CreateProviderSessionEventParams{
		TenantID:          uuid.New(),
		ProviderSessionID: session.ID,
		NodeID:            node.NodeID,
		EventType:         "message_delta",
		SequenceNumber:    3,
		Payload:           []byte(`{"message":"wrong tenant"}`),
		RequestID:         pgtype.Text{String: "request-wrong-tenant", Valid: true},
		Metadata:          []byte(`{}`),
	})
	assert.Error(t, err)

	_, err = testQueries.CreateProviderSessionEvent(ctx, queries.CreateProviderSessionEventParams{
		TenantID:          tenantID,
		ProviderSessionID: session.ID,
		NodeID:            "wrong-runtime-node",
		EventType:         "message_delta",
		SequenceNumber:    3,
		Payload:           []byte(`{"message":"wrong runtime node"}`),
		RequestID:         pgtype.Text{String: "request-wrong-runtime-node", Valid: true},
		Metadata:          []byte(`{}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.RevokeRuntimeSession(ctx, queries.RevokeRuntimeSessionParams{
		ID:            runtimeSession.ID,
		TenantID:      tenantID,
		RevokedReason: pgtype.Text{String: "runtime stopped before event upload", Valid: true},
	})
	require.NoError(t, err)
	_, err = testQueries.CreateProviderSessionEvent(ctx, queries.CreateProviderSessionEventParams{
		TenantID:          tenantID,
		ProviderSessionID: session.ID,
		NodeID:            node.NodeID,
		EventType:         "message_delta",
		SequenceNumber:    3,
		Payload:           []byte(`{"message":"revoked runtime session"}`),
		RequestID:         pgtype.Text{String: "request-revoked-runtime-session", Valid: true},
		Metadata:          []byte(`{}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = testQueries.CreateRuntimeSession(ctx, queries.CreateRuntimeSessionParams{
		TenantID:        tenantID,
		RuntimeNodeID:   node.ID,
		EnrollmentID:    uuid.NullUUID{UUID: approvedEnrollment.ID, Valid: true},
		TokenLookupHash: "94f98d42ed5bca91ec4099422b84f0d0de2a1bd8f80d5d7c0dd9e5f1d5418571",
		TokenSecretHash: "$2a$10$digitalEmployeeRuntimeSessionHashAfterRevoke",
		ExpiresAt:       runtimeSessionExpiresAt,
	})
	require.NoError(t, err)
	_, err = testQueries.RevokeRuntimeEnrollment(ctx, queries.RevokeRuntimeEnrollmentParams{
		ID:           approvedEnrollment.ID,
		TenantID:     tenantID,
		RevokedBy:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
		RevokeReason: pgtype.Text{String: "operator revoked runtime before event upload", Valid: true},
	})
	require.NoError(t, err)
	_, err = testQueries.CreateProviderSessionEvent(ctx, queries.CreateProviderSessionEventParams{
		TenantID:          tenantID,
		ProviderSessionID: session.ID,
		NodeID:            node.NodeID,
		EventType:         "message_delta",
		SequenceNumber:    3,
		Payload:           []byte(`{"message":"revoked runtime enrollment"}`),
		RequestID:         pgtype.Text{String: "request-revoked-runtime-enrollment", Valid: true},
		Metadata:          []byte(`{}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	events, err := testQueries.ListProviderSessionEvents(ctx, queries.ListProviderSessionEventsParams{
		TenantID:          tenantID,
		ProviderSessionID: session.ID,
	})
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, event1.ID, events[0].ID)
	assert.Equal(t, int32(2), events[1].SequenceNumber)

	maxSequence, err := testQueries.GetLatestProviderSessionEventSequence(ctx, queries.GetLatestProviderSessionEventSequenceParams{
		TenantID:          tenantID,
		ProviderSessionID: session.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, int32(2), maxSequence)

	updatedSession, err := testQueries.UpdateProviderSessionStatus(ctx, queries.UpdateProviderSessionStatusParams{
		ID:       session.ID,
		TenantID: tenantID,
		Status:   "idle",
	})
	require.NoError(t, err)
	assert.Equal(t, "idle", updatedSession.Status)
}

func TestRuntimeProvisioningPreflightEnforcesTeamPolicies(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	now := pgtype.Timestamptz{}
	require.NoError(t, now.Scan(time.Now()))

	node, err := testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "preflight-policy-runtime-node",
		Name:               "Preflight Policy Runtime Node",
		SupportedProviders: []byte(`{"providers":["codex","opencode"]}`),
		MaxSlots:           2,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           []byte(`{"agent_home_dir":"/nodes/preflight-policy"}`),
		LastHeartbeatAt:    now,
	})
	require.NoError(t, err)

	bootstrapKeyExpiresAt := pgtype.Timestamptz{}
	require.NoError(t, bootstrapKeyExpiresAt.Scan(time.Now().Add(24*time.Hour)))
	bootstrapKey, err := testQueries.CreateRuntimeBootstrapKey(ctx, queries.CreateRuntimeBootstrapKeyParams{
		TenantID:  uuid.NullUUID{UUID: tenantID, Valid: true},
		Name:      "preflight-policy-bootstrap",
		KeyHash:   "preflight-policy-bootstrap-key-hash",
		Status:    "active",
		ExpiresAt: bootstrapKeyExpiresAt,
		Metadata:  []byte(`{}`),
	})
	require.NoError(t, err)
	enrollment, err := testQueries.UpsertRuntimeEnrollment(ctx, queries.UpsertRuntimeEnrollmentParams{
		TenantID:       tenantID,
		NodeID:         node.NodeID,
		BootstrapKeyID: bootstrapKey.ID,
		RequestPayload: []byte(`{"node_id":"preflight-policy-runtime-node"}`),
		LastHelloAt:    now,
	})
	require.NoError(t, err)
	approvedEnrollment, err := testQueries.ApproveRuntimeEnrollment(ctx, queries.ApproveRuntimeEnrollmentParams{
		RuntimeNodeID: node.ID,
		ID:            enrollment.ID,
		TenantID:      tenantID,
		ApprovedBy:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
	})
	require.NoError(t, err)
	sessionExpiresAt := pgtype.Timestamptz{}
	require.NoError(t, sessionExpiresAt.Scan(time.Now().Add(12*time.Hour)))
	_, err = testQueries.CreateRuntimeSession(ctx, queries.CreateRuntimeSessionParams{
		TenantID:        tenantID,
		RuntimeNodeID:   node.ID,
		EnrollmentID:    uuid.NullUUID{UUID: approvedEnrollment.ID, Valid: true},
		TokenLookupHash: "preflight-policy-session-lookup-hash",
		TokenSecretHash: "$2a$10$preflightPolicySessionSecretHash",
		ExpiresAt:       sessionExpiresAt,
	})
	require.NoError(t, err)
	for _, providerType := range []string{"codex", "opencode"} {
		_, err = testQueries.UpsertRuntimeCapability(ctx, queries.UpsertRuntimeCapabilityParams{
			TenantID:         tenantID,
			RuntimeNodeID:    node.ID,
			CapabilityType:   "provider",
			CapabilityKey:    providerType,
			ProviderType:     providerType,
			ProviderVersion:  pgtype.Text{String: "1.0.0", Valid: true},
			BinaryPath:       pgtype.Text{String: "/usr/local/bin/" + providerType, Valid: true},
			Available:        true,
			WorkspaceBaseDir: pgtype.Text{String: "/data/superteam/workspaces", Valid: true},
			Capacity:         []byte(`{"max_slots":2}`),
			Labels:           []byte(`{"os":"darwin"}`),
			Status:           "healthy",
			Details:          []byte(`{"agent_home_dir":"/provider/preflight-policy"}`),
			HealthStatus:     "healthy",
			Metadata:         []byte(`{}`),
			LastSeenAt:       now,
		})
		require.NoError(t, err)
	}
	_, err = testQueries.UpsertRuntimeCapability(ctx, queries.UpsertRuntimeCapabilityParams{
		TenantID:         tenantID,
		RuntimeNodeID:    node.ID,
		CapabilityType:   "workspace",
		CapabilityKey:    "base-dir",
		ProviderType:     "workspace",
		ProviderVersion:  pgtype.Text{},
		BinaryPath:       pgtype.Text{},
		Available:        true,
		WorkspaceBaseDir: pgtype.Text{String: "/data/superteam/workspaces", Valid: true},
		Capacity:         []byte(`{}`),
		Labels:           []byte(`{}`),
		Status:           "available",
		Details:          []byte(`{}`),
		HealthStatus:     "configured",
		Metadata:         []byte(`{}`),
		LastSeenAt:       now,
	})
	require.NoError(t, err)

	runtimeScopePolicy := []byte(fmt.Sprintf(`{"allowed_runtime_node_ids":["%s"],"allowed_node_ids":["%s"]}`, node.ID.String(), node.NodeID))
	teamConfig, err := testQueries.CreateTenantTeamConfigRevision(ctx, queries.CreateTenantTeamConfigRevisionParams{
		TenantID:                    tenantID,
		TeamID:                      teamID,
		RevisionNumber:              1,
		Constitution:                []byte(`{}`),
		CapabilityPolicy:            []byte(`{"allowed_provider_types":["codex"]}`),
		ContextPolicy:               []byte(`{}`),
		ApprovalPolicy:              []byte(`{}`),
		ArtifactContract:            []byte(`{}`),
		InternalCollaborationPolicy: []byte(`{}`),
		RuntimeScopePolicy:          runtimeScopePolicy,
		Status:                      "active",
		ApprovedBy:                  uuid.NullUUID{UUID: uuid.New(), Valid: true},
		ApprovedAt:                  now,
	})
	require.NoError(t, err)

	preflight, err := testQueries.GetRuntimeProvisioningPreflight(ctx, queries.GetRuntimeProvisioningPreflightParams{
		TenantID:      tenantID,
		TeamID:        teamID,
		RuntimeNodeID: node.ID,
		ProviderType:  "codex",
	})
	require.NoError(t, err)
	assert.True(t, preflight.HasActiveTeamConfig)
	assert.True(t, preflight.RuntimeOnline)
	assert.True(t, preflight.EnrollmentApproved)
	assert.True(t, preflight.RuntimeSessionActive)
	assert.True(t, preflight.ProviderAvailable)
	assert.True(t, preflight.ProviderPolicyAllowed)
	assert.True(t, preflight.RuntimePolicyAllowed)
	assert.Equal(t, "/provider/preflight-policy", preflight.AgentHomeDir)

	_, err = testDB.Exec(ctx, `
		UPDATE runtime_capabilities
		SET details = '{}'::jsonb,
		    workspace_base_dir = NULL,
		    updated_at = NOW()
		WHERE tenant_id = $1
		  AND runtime_node_id = $2
		  AND capability_type = 'provider'
		  AND provider_type = 'codex'
	`, tenantID, node.ID)
	require.NoError(t, err)
	preflight, err = testQueries.GetRuntimeProvisioningPreflight(ctx, queries.GetRuntimeProvisioningPreflightParams{
		TenantID:      tenantID,
		TeamID:        teamID,
		RuntimeNodeID: node.ID,
		ProviderType:  "codex",
	})
	require.NoError(t, err)
	assert.Equal(t, "/data/superteam/workspaces", preflight.AgentHomeDir)

	preflight, err = testQueries.GetRuntimeProvisioningPreflight(ctx, queries.GetRuntimeProvisioningPreflightParams{
		TenantID:      tenantID,
		TeamID:        teamID,
		RuntimeNodeID: node.ID,
		ProviderType:  "opencode",
	})
	require.NoError(t, err)
	assert.True(t, preflight.ProviderAvailable)
	assert.False(t, preflight.ProviderPolicyAllowed)
	assert.True(t, preflight.RuntimePolicyAllowed)

	_, err = testDB.Exec(ctx, `
		UPDATE tenant_team_config_revisions
		SET capability_policy = '{}'::jsonb,
		    runtime_scope_policy = $2::jsonb,
		    updated_at = NOW()
		WHERE id = $1
	`, teamConfig.ID, runtimeScopePolicy)
	require.NoError(t, err)
	preflight, err = testQueries.GetRuntimeProvisioningPreflight(ctx, queries.GetRuntimeProvisioningPreflightParams{
		TenantID:      tenantID,
		TeamID:        teamID,
		RuntimeNodeID: node.ID,
		ProviderType:  "codex",
	})
	require.NoError(t, err)
	assert.True(t, preflight.ProviderAvailable)
	assert.False(t, preflight.ProviderPolicyAllowed)
	assert.True(t, preflight.RuntimePolicyAllowed)

	runtimeScopeProviderTypes := []byte(`{"provider_types":["codex"]}`)
	_, err = testDB.Exec(ctx, `
		UPDATE tenant_team_config_revisions
		SET capability_policy = '{}'::jsonb,
		    runtime_scope_policy = $2::jsonb,
		    updated_at = NOW()
		WHERE id = $1
	`, teamConfig.ID, runtimeScopeProviderTypes)
	require.NoError(t, err)
	preflight, err = testQueries.GetRuntimeProvisioningPreflight(ctx, queries.GetRuntimeProvisioningPreflightParams{
		TenantID:      tenantID,
		TeamID:        teamID,
		RuntimeNodeID: node.ID,
		ProviderType:  "codex",
	})
	require.NoError(t, err)
	assert.True(t, preflight.ProviderAvailable)
	assert.True(t, preflight.ProviderPolicyAllowed)
	assert.True(t, preflight.RuntimePolicyAllowed)

	_, err = testDB.Exec(ctx, `
		UPDATE tenant_team_config_revisions
		SET capability_policy = '{"allowed_provider_types":[]}'::jsonb,
		    runtime_scope_policy = $2::jsonb,
		    updated_at = NOW()
		WHERE id = $1
	`, teamConfig.ID, runtimeScopePolicy)
	require.NoError(t, err)
	preflight, err = testQueries.GetRuntimeProvisioningPreflight(ctx, queries.GetRuntimeProvisioningPreflightParams{
		TenantID:      tenantID,
		TeamID:        teamID,
		RuntimeNodeID: node.ID,
		ProviderType:  "codex",
	})
	require.NoError(t, err)
	assert.True(t, preflight.ProviderAvailable)
	assert.False(t, preflight.ProviderPolicyAllowed)
	assert.True(t, preflight.RuntimePolicyAllowed)

	_, err = testDB.Exec(ctx, `
		UPDATE tenant_team_config_revisions
		SET capability_policy = '{"allowed_provider_types":["codex"]}'::jsonb,
		    runtime_scope_policy = '{}'::jsonb,
		    updated_at = NOW()
		WHERE id = $1
	`, teamConfig.ID)
	require.NoError(t, err)
	preflight, err = testQueries.GetRuntimeProvisioningPreflight(ctx, queries.GetRuntimeProvisioningPreflightParams{
		TenantID:      tenantID,
		TeamID:        teamID,
		RuntimeNodeID: node.ID,
		ProviderType:  "codex",
	})
	require.NoError(t, err)
	assert.True(t, preflight.ProviderPolicyAllowed)
	assert.False(t, preflight.RuntimePolicyAllowed)

	_, err = testDB.Exec(ctx, `
		UPDATE tenant_team_config_revisions
		SET runtime_scope_policy = $2::jsonb,
		    updated_at = NOW()
		WHERE id = $1
	`, teamConfig.ID, runtimeScopePolicy)
	require.NoError(t, err)
	_, err = testDB.Exec(ctx, `
		UPDATE runtime_enrollments
		SET status = 'revoked',
		    revoked_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1
		  AND tenant_id = $2
	`, approvedEnrollment.ID, tenantID)
	require.NoError(t, err)
	preflight, err = testQueries.GetRuntimeProvisioningPreflight(ctx, queries.GetRuntimeProvisioningPreflightParams{
		TenantID:      tenantID,
		TeamID:        teamID,
		RuntimeNodeID: node.ID,
		ProviderType:  "codex",
	})
	require.NoError(t, err)
	assert.False(t, preflight.EnrollmentApproved)
	assert.False(t, preflight.RuntimeSessionActive)
	assert.True(t, preflight.ProviderPolicyAllowed)
	assert.True(t, preflight.RuntimePolicyAllowed)
}

// ============================================================================
// Task Tests
// ============================================================================

func TestCreateTask(t *testing.T) {
	ctx := context.Background()

	// 创建测试用户
	user, err := testQueries.CreateUser(ctx, queries.CreateUserParams{
		Username:     "task-creator",
		DisplayName:  pgtype.Text{String: "Task Creator", Valid: true},
		Email:        pgtype.Text{String: "creator@example.com", Valid: true},
		PasswordHash: "$2a$10$hashedpassword",
		Status:       "active",
	})
	require.NoError(t, err)
	defer testQueries.DeleteUser(ctx, user.ID)

	params := map[string]interface{}{
		"command": "test command",
		"args":    []string{"arg1", "arg2"},
	}
	paramsJSON, err := json.Marshal(params)
	require.NoError(t, err)

	task, err := testQueries.CreateTask(ctx, queries.CreateTaskParams{
		Title:         "Test Task",
		Description:   pgtype.Text{String: "Test task description", Valid: true},
		Status:        "pending",
		Priority:      5,
		ProviderType:  "claude-code",
		CreatorID:     uuid.NullUUID{UUID: user.ID, Valid: true},
		TargetNodeID:  pgtype.Text{String: "test-node-001", Valid: true},
		WorkspacePath: pgtype.Text{String: "/tmp/workspace", Valid: true},
		Params:        paramsJSON,
	})

	require.NoError(t, err)
	assert.Equal(t, "Test Task", task.Title)
	assert.Equal(t, "pending", task.Status)
	assert.Equal(t, int32(5), task.Priority)
	assert.Equal(t, "claude-code", task.ProviderType)

	// 清理
	defer testQueries.DeleteTask(ctx, queries.DeleteTaskParams{ID: task.ID})
}

func TestGetTask(t *testing.T) {
	ctx := context.Background()

	// 创建任务
	paramsJSON, _ := json.Marshal(map[string]interface{}{"test": true})
	created, err := testQueries.CreateTask(ctx, queries.CreateTaskParams{
		Title:        "Get Task Test",
		Description:  pgtype.Text{String: "Description", Valid: true},
		Status:       "pending",
		Priority:     3,
		ProviderType: "claude-code",
		Params:       paramsJSON,
	})
	require.NoError(t, err)
	defer testQueries.DeleteTask(ctx, queries.DeleteTaskParams{ID: created.ID})

	// 查询任务
	task, err := testQueries.GetTask(ctx, queries.GetTaskParams{ID: created.ID})
	require.NoError(t, err)
	assert.Equal(t, created.ID, task.ID)
	assert.Equal(t, created.Title, task.Title)
}

func TestListTasks(t *testing.T) {
	ctx := context.Background()

	// 创建多个任务
	paramsJSON, _ := json.Marshal(map[string]interface{}{"test": true})

	task1, err := testQueries.CreateTask(ctx, queries.CreateTaskParams{
		Title:        "Task 1",
		Status:       "pending",
		Priority:     5,
		ProviderType: "claude-code",
		Params:       paramsJSON,
	})
	require.NoError(t, err)
	defer testQueries.DeleteTask(ctx, queries.DeleteTaskParams{ID: task1.ID})

	task2, err := testQueries.CreateTask(ctx, queries.CreateTaskParams{
		Title:        "Task 2",
		Status:       "running",
		Priority:     3,
		ProviderType: "opencode",
		Params:       paramsJSON,
	})
	require.NoError(t, err)
	defer testQueries.DeleteTask(ctx, queries.DeleteTaskParams{ID: task2.ID})

	// 测试无过滤
	tasks, err := testQueries.ListTasks(ctx, queries.ListTasksParams{
		Offset: 0,
		Limit:  10,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(tasks), 2)

	// 测试状态过滤
	pendingTasks, err := testQueries.ListTasks(ctx, queries.ListTasksParams{
		Status: pgtype.Text{String: "pending", Valid: true},
		Offset: 0,
		Limit:  10,
	})
	require.NoError(t, err)
	for _, task := range pendingTasks {
		assert.Equal(t, "pending", task.Status)
	}

	// 测试 provider 过滤
	claudeTasks, err := testQueries.ListTasks(ctx, queries.ListTasksParams{
		ProviderType: pgtype.Text{String: "claude-code", Valid: true},
		Offset:       0,
		Limit:        10,
	})
	require.NoError(t, err)
	for _, task := range claudeTasks {
		assert.Equal(t, "claude-code", task.ProviderType)
	}
}

func TestUpdateTaskStatus(t *testing.T) {
	ctx := context.Background()

	// 创建任务
	paramsJSON, _ := json.Marshal(map[string]interface{}{"test": true})
	task, err := testQueries.CreateTask(ctx, queries.CreateTaskParams{
		Title:        "Status Update Test",
		Status:       "pending",
		Priority:     3,
		ProviderType: "claude-code",
		Params:       paramsJSON,
	})
	require.NoError(t, err)
	defer testQueries.DeleteTask(ctx, queries.DeleteTaskParams{ID: task.ID})

	// 更新状态
	updated, err := testQueries.UpdateTaskStatus(ctx, queries.UpdateTaskStatusParams{
		ID:     task.ID,
		Status: "running",
	})
	require.NoError(t, err)
	assert.Equal(t, "running", updated.Status)
	assert.True(t, updated.UpdatedAt.Time.After(task.UpdatedAt.Time))
}

func TestTaskStateTransition(t *testing.T) {
	ctx := context.Background()

	// 创建任务
	paramsJSON, _ := json.Marshal(map[string]interface{}{"test": true})
	task, err := testQueries.CreateTask(ctx, queries.CreateTaskParams{
		Title:        "State Transition Test",
		Status:       "pending",
		Priority:     3,
		ProviderType: "claude-code",
		Params:       paramsJSON,
	})
	require.NoError(t, err)
	defer testQueries.DeleteTask(ctx, queries.DeleteTaskParams{ID: task.ID})

	// 状态转换: pending -> running
	task, err = testQueries.UpdateTaskStatus(ctx, queries.UpdateTaskStatusParams{
		ID:     task.ID,
		Status: "running",
	})
	require.NoError(t, err)
	assert.Equal(t, "running", task.Status)

	// 状态转换: running -> completed
	task, err = testQueries.UpdateTaskStatus(ctx, queries.UpdateTaskStatusParams{
		ID:     task.ID,
		Status: "completed",
	})
	require.NoError(t, err)
	assert.Equal(t, "completed", task.Status)
}

func TestTaskEvents(t *testing.T) {
	ctx := context.Background()

	// 创建任务
	paramsJSON, _ := json.Marshal(map[string]interface{}{"test": true})
	task, err := testQueries.CreateTask(ctx, queries.CreateTaskParams{
		Title:        "Event Test",
		Status:       "pending",
		Priority:     3,
		ProviderType: "claude-code",
		Params:       paramsJSON,
	})
	require.NoError(t, err)
	defer testQueries.DeleteTask(ctx, queries.DeleteTaskParams{ID: task.ID})

	// 创建事件
	eventPayload, _ := json.Marshal(map[string]interface{}{"message": "Task started"})
	event1, err := testQueries.CreateTaskEvent(ctx, queries.CreateTaskEventParams{
		TaskID:         task.ID,
		EventType:      "task.started",
		SequenceNumber: 1,
		Payload:        eventPayload,
	})
	require.NoError(t, err)
	assert.Equal(t, task.ID, event1.TaskID)
	assert.Equal(t, int32(1), event1.SequenceNumber)

	// 创建第二个事件
	eventPayload2, _ := json.Marshal(map[string]interface{}{"message": "Task progress"})
	_, err = testQueries.CreateTaskEvent(ctx, queries.CreateTaskEventParams{
		TaskID:         task.ID,
		EventType:      "task.progress",
		SequenceNumber: 2,
		Payload:        eventPayload2,
	})
	require.NoError(t, err)

	// 列出事件
	events, err := testQueries.ListTaskEvents(ctx, queries.ListTaskEventsParams{TaskID: task.ID})
	require.NoError(t, err)
	assert.Len(t, events, 2)
	assert.Equal(t, int32(1), events[0].SequenceNumber)
	assert.Equal(t, int32(2), events[1].SequenceNumber)

	// 获取最新序列号
	maxSeq, err := testQueries.GetLatestTaskEventSequence(ctx, queries.GetLatestTaskEventSequenceParams{TaskID: task.ID})
	require.NoError(t, err)
	assert.Equal(t, int32(2), maxSeq)
}

func TestTaskEventIdempotencyIsScopedByRun(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	paramsJSON, _ := json.Marshal(map[string]interface{}{"test": true})
	task, err := testQueries.CreateTask(ctx, queries.CreateTaskParams{
		Title:        "Run Scoped Event Test",
		Status:       "pending",
		Priority:     3,
		ProviderType: "claude-code",
		Params:       paramsJSON,
	})
	require.NoError(t, err)

	run1, err := testQueries.CreateTaskRun(ctx, queries.CreateTaskRunParams{
		TaskID: task.ID,
		NodeID: "runtime-node-001",
		Status: "running",
	})
	require.NoError(t, err)
	run2, err := testQueries.CreateTaskRun(ctx, queries.CreateTaskRunParams{
		TaskID: task.ID,
		NodeID: "runtime-node-001",
		Status: "running",
	})
	require.NoError(t, err)

	event1, err := testQueries.CreateTaskEventIfAbsent(ctx, queries.CreateTaskEventIfAbsentParams{
		TenantID:       task.TenantID,
		TaskID:         task.ID,
		RunID:          run1.ID,
		EventType:      "provider.text_delta",
		SequenceNumber: 1,
		Payload:        []byte(`{"run":1}`),
		CommandID:      pgtype.Text{String: "cmd-run-1", Valid: true},
		Metadata:       []byte(`{}`),
	})
	require.NoError(t, err)

	event2, err := testQueries.CreateTaskEventIfAbsent(ctx, queries.CreateTaskEventIfAbsentParams{
		TenantID:       task.TenantID,
		TaskID:         task.ID,
		RunID:          run2.ID,
		EventType:      "provider.text_delta",
		SequenceNumber: 1,
		Payload:        []byte(`{"run":2}`),
		CommandID:      pgtype.Text{String: "cmd-run-2", Valid: true},
		Metadata:       []byte(`{}`),
	})
	require.NoError(t, err)
	assert.NotEqual(t, event1.ID, event2.ID)
	assert.Equal(t, run1.ID, event1.RunID.UUID)
	assert.Equal(t, run2.ID, event2.RunID.UUID)
}

func TestDigitalEmployeeRunLoopPersistenceQueries(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	runtimeNodeID := uuid.New()
	digitalEmployeeID := uuid.New()
	executionInstanceID := uuid.New()
	commandID := "cmd-run-loop-001"
	dispatchedAt := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	receipt, err := testQueries.CreateRuntimeCommandReceipt(ctx, queries.CreateRuntimeCommandReceiptParams{
		TenantID:      tenantID,
		CommandID:     commandID,
		CommandType:   "provider_run",
		RuntimeNodeID: runtimeNodeID,
		NodeID:        "runtime-node-001",
		ResourceType:  "task_run",
		ResourceID:    uuid.New(),
		Status:        "dispatching",
		Payload:       []byte(`{"objective":"diagnose failing checkout"}`),
		DispatchedAt:  dispatchedAt,
	})
	require.NoError(t, err)
	assert.Equal(t, commandID, receipt.CommandID)
	assert.Equal(t, "dispatching", receipt.Status)

	duplicateReceipt, err := testQueries.CreateRuntimeCommandReceipt(ctx, queries.CreateRuntimeCommandReceiptParams{
		TenantID:      tenantID,
		CommandID:     commandID,
		CommandType:   "provider_run",
		RuntimeNodeID: runtimeNodeID,
		NodeID:        "runtime-node-001",
		ResourceType:  "task_run",
		ResourceID:    uuid.New(),
		Status:        "dispatching",
		Payload:       []byte(`{"objective":"duplicate should not overwrite"}`),
		DispatchedAt:  dispatchedAt,
	})
	require.NoError(t, err)
	assert.Equal(t, receipt.ID, duplicateReceipt.ID)
	assert.JSONEq(t, `{"objective":"diagnose failing checkout"}`, string(duplicateReceipt.Payload))

	fetchedReceipt, err := testQueries.GetRuntimeCommandReceiptByCommandID(ctx, queries.GetRuntimeCommandReceiptByCommandIDParams{
		TenantID:  tenantID,
		CommandID: commandID,
	})
	require.NoError(t, err)
	assert.Equal(t, receipt.ID, fetchedReceipt.ID)

	completedReceipt, err := testQueries.UpdateRuntimeCommandReceiptStatus(ctx, queries.UpdateRuntimeCommandReceiptStatusParams{
		TenantID:     tenantID,
		CommandID:    commandID,
		Status:       "completed",
		Result:       []byte(`{"ok":true}`),
		ErrorMessage: pgtype.Text{},
	})
	require.NoError(t, err)
	assert.Equal(t, "completed", completedReceipt.Status)
	assert.True(t, completedReceipt.CompletedAt.Valid)
	assert.JSONEq(t, `{"ok":true}`, string(completedReceipt.Result))

	createdRun, err := testQueries.CreateDigitalEmployeeTaskRun(ctx, queries.CreateDigitalEmployeeTaskRunParams{
		TenantID:            tenantID,
		TeamID:              teamID,
		Title:               "执行结账诊断",
		Description:         pgtype.Text{String: "复现并诊断结账失败", Valid: true},
		Priority:            5,
		ProviderType:        "claude-code",
		CreatorID:           uuid.NullUUID{},
		TargetNodeID:        "runtime-node-001",
		WorkspacePath:       pgtype.Text{String: "/workspace/superteam", Valid: true},
		Params:              []byte(`{"objective":"diagnose failing checkout"}`),
		IdempotencyKey:      pgtype.Text{String: "idem-run-loop-001", Valid: true},
		RiskLevel:           pgtype.Text{String: "normal", Valid: true},
		NodeID:              "runtime-node-001",
		RuntimeNodeID:       runtimeNodeID,
		ProviderSessionID:   pgtype.Text{},
		RunStatus:           "queued",
		CommandID:           commandID,
		DigitalEmployeeID:   digitalEmployeeID,
		ExecutionInstanceID: executionInstanceID,
		TimeoutSec:          pgtype.Int4{Int32: 600, Valid: true},
		GraceSec:            pgtype.Int4{Int32: 30, Valid: true},
		IdempotencyFingerprint: pgtype.Text{
			String: "fingerprint-run-loop-001",
			Valid:  true,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "pending", createdRun.TaskStatus)
	assert.Equal(t, "queued", createdRun.RunStatus)

	retriedRun, err := testQueries.CreateDigitalEmployeeTaskRun(ctx, queries.CreateDigitalEmployeeTaskRunParams{
		TenantID:            tenantID,
		TeamID:              teamID,
		Title:               "重复执行结账诊断",
		Description:         pgtype.Text{String: "同一幂等键重试不应新建任务", Valid: true},
		Priority:            1,
		ProviderType:        "claude-code",
		CreatorID:           uuid.NullUUID{},
		TargetNodeID:        "runtime-node-001",
		WorkspacePath:       pgtype.Text{String: "/workspace/superteam", Valid: true},
		Params:              []byte(`{"objective":"duplicate retry"}`),
		IdempotencyKey:      pgtype.Text{String: "idem-run-loop-001", Valid: true},
		RiskLevel:           pgtype.Text{String: "normal", Valid: true},
		NodeID:              "runtime-node-001",
		RuntimeNodeID:       runtimeNodeID,
		ProviderSessionID:   pgtype.Text{},
		RunStatus:           "queued",
		CommandID:           "cmd-run-loop-retry",
		DigitalEmployeeID:   digitalEmployeeID,
		ExecutionInstanceID: executionInstanceID,
		TimeoutSec:          pgtype.Int4{Int32: 600, Valid: true},
		GraceSec:            pgtype.Int4{Int32: 30, Valid: true},
		IdempotencyFingerprint: pgtype.Text{
			String: "fingerprint-run-loop-001",
			Valid:  true,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, createdRun.TaskID, retriedRun.TaskID)
	assert.Equal(t, createdRun.RunID, retriedRun.RunID)
	assert.Equal(t, createdRun.CommandID, retriedRun.CommandID)

	_, err = testQueries.CreateDigitalEmployeeTaskRun(ctx, queries.CreateDigitalEmployeeTaskRunParams{
		TenantID:            tenantID,
		TeamID:              teamID,
		Title:               "幂等冲突执行结账诊断",
		Description:         pgtype.Text{String: "同一幂等键不同指纹应返回冲突信号", Valid: true},
		Priority:            1,
		ProviderType:        "claude-code",
		CreatorID:           uuid.NullUUID{},
		TargetNodeID:        "runtime-node-001",
		WorkspacePath:       pgtype.Text{String: "/workspace/superteam", Valid: true},
		Params:              []byte(`{"objective":"conflicting retry"}`),
		IdempotencyKey:      pgtype.Text{String: "idem-run-loop-001", Valid: true},
		RiskLevel:           pgtype.Text{String: "normal", Valid: true},
		NodeID:              "runtime-node-001",
		RuntimeNodeID:       runtimeNodeID,
		ProviderSessionID:   pgtype.Text{},
		RunStatus:           "queued",
		CommandID:           "cmd-run-loop-conflict",
		DigitalEmployeeID:   digitalEmployeeID,
		ExecutionInstanceID: executionInstanceID,
		TimeoutSec:          pgtype.Int4{Int32: 600, Valid: true},
		GraceSec:            pgtype.Int4{Int32: 30, Valid: true},
		IdempotencyFingerprint: pgtype.Text{
			String: "fingerprint-run-loop-conflict",
			Valid:  true,
		},
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	var taskCountAfterConflict int
	err = testDB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM tasks
		WHERE tenant_id = $1
		  AND idempotency_key = 'idem-run-loop-001'
	`, tenantID).Scan(&taskCountAfterConflict)
	require.NoError(t, err)
	assert.Equal(t, 1, taskCountAfterConflict)

	var runCountAfterConflict int
	err = testDB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM task_runs
		WHERE tenant_id = $1
		  AND digital_employee_id = $2
		  AND idempotency_key = 'idem-run-loop-001'
	`, tenantID, digitalEmployeeID).Scan(&runCountAfterConflict)
	require.NoError(t, err)
	assert.Equal(t, 1, runCountAfterConflict)

	activeRun, err := testQueries.GetActiveDigitalEmployeeRun(ctx, queries.GetActiveDigitalEmployeeRunParams{
		TenantID:          tenantID,
		DigitalEmployeeID: digitalEmployeeID,
	})
	require.NoError(t, err)
	assert.Equal(t, createdRun.RunID, activeRun.ID)

	runByCommand, err := testQueries.GetDigitalEmployeeRunByCommandID(ctx, queries.GetDigitalEmployeeRunByCommandIDParams{
		TenantID:  tenantID,
		CommandID: commandID,
	})
	require.NoError(t, err)
	assert.Equal(t, createdRun.RunID, runByCommand.ID)

	listedRuns, err := testQueries.ListDigitalEmployeeRuns(ctx, queries.ListDigitalEmployeeRunsParams{
		TenantID:          tenantID,
		DigitalEmployeeID: digitalEmployeeID,
		Limit:             10,
		Offset:            0,
	})
	require.NoError(t, err)
	require.Len(t, listedRuns, 1)
	assert.Equal(t, createdRun.RunID, listedRuns[0].ID)

	updatedRun, err := testQueries.UpdateDigitalEmployeeRunStatus(ctx, queries.UpdateDigitalEmployeeRunStatusParams{
		TenantID:                  tenantID,
		RunID:                     createdRun.RunID,
		Status:                    "timed_out",
		Result:                    []byte(`{"summary":"timeout"}`),
		ErrorMessage:              pgtype.Text{String: "provider did not stop before deadline", Valid: true},
		Diagnostic:                []byte(`{"deadline":"exceeded"}`),
		LogRef:                    pgtype.Text{String: "s3://superteam/logs/cmd-run-loop-001.log", Valid: true},
		RawResultRef:              pgtype.Text{String: "s3://superteam/results/cmd-run-loop-001.json", Valid: true},
		WorkProducts:              []byte(`[{"type":"ExecutionResult","ref":"artifact://result"}]`),
		SessionState:              []byte(`{"phase":"terminated"}`),
		ErrorCode:                 pgtype.Text{String: "PROVIDER_TIMEOUT", Valid: true},
		ErrorFamily:               pgtype.Text{String: "timeout", Valid: true},
		ExitCode:                  pgtype.Int4{Int32: 124, Valid: true},
		Signal:                    pgtype.Text{String: "SIGTERM", Valid: true},
		ProviderSessionExternalID: pgtype.Text{String: "provider-session-001", Valid: true},
	})
	require.NoError(t, err)
	assert.True(t, updatedRun.TimedOut)
	assert.True(t, updatedRun.FinishedAt.Valid)
	assert.JSONEq(t, `{"deadline":"exceeded"}`, string(updatedRun.Diagnostic))
	assert.JSONEq(t, `[{"type":"ExecutionResult","ref":"artifact://result"}]`, string(updatedRun.WorkProducts))

	dispatchedTaskEvent, err := testQueries.CreateTaskEventIfAbsent(ctx, queries.CreateTaskEventIfAbsentParams{
		TenantID:       tenantID,
		TaskID:         createdRun.TaskID,
		RunID:          createdRun.RunID,
		EventType:      "run_dispatched",
		SequenceNumber: -1,
		Payload:        []byte(`{"source":"control-plane"}`),
		CommandID:      pgtype.Text{String: commandID, Valid: true},
		Metadata:       []byte(`{"source":"control-plane"}`),
	})
	require.NoError(t, err)

	taskEvent, err := testQueries.CreateTaskEventIfAbsent(ctx, queries.CreateTaskEventIfAbsentParams{
		TenantID:       tenantID,
		TaskID:         createdRun.TaskID,
		RunID:          createdRun.RunID,
		EventType:      "provider.text_delta",
		SequenceNumber: 1,
		Payload:        []byte(`{"text":"working"}`),
		CommandID:      pgtype.Text{String: commandID, Valid: true},
		RawEventRef:    pgtype.Text{String: "s3://superteam/raw/cmd-run-loop-001/1.json", Valid: true},
		LogRef:         pgtype.Text{String: "s3://superteam/logs/cmd-run-loop-001.log", Valid: true},
		Metadata:       []byte(`{"source":"runtime-writeback"}`),
	})
	require.NoError(t, err)

	duplicateTaskEvent, err := testQueries.CreateTaskEventIfAbsent(ctx, queries.CreateTaskEventIfAbsentParams{
		TenantID:       tenantID,
		TaskID:         createdRun.TaskID,
		RunID:          createdRun.RunID,
		EventType:      "provider.text_delta",
		SequenceNumber: 1,
		Payload:        []byte(`{"text":"duplicate"}`),
		CommandID:      pgtype.Text{String: commandID, Valid: true},
		RawEventRef:    pgtype.Text{String: "s3://superteam/raw/cmd-run-loop-001/duplicate.json", Valid: true},
		LogRef:         pgtype.Text{String: "s3://superteam/logs/cmd-run-loop-001.log", Valid: true},
		Metadata:       []byte(`{"source":"runtime-writeback"}`),
	})
	require.NoError(t, err)
	assert.Equal(t, taskEvent.ID, duplicateTaskEvent.ID)
	assert.JSONEq(t, `{"text":"working"}`, string(duplicateTaskEvent.Payload))

	stopRequestedTaskEvent, err := testQueries.CreateTaskEventIfAbsent(ctx, queries.CreateTaskEventIfAbsentParams{
		TenantID:       tenantID,
		TaskID:         createdRun.TaskID,
		RunID:          createdRun.RunID,
		EventType:      "stop_requested",
		SequenceNumber: -2,
		Payload:        []byte(`{"source":"control-plane"}`),
		CommandID:      pgtype.Text{String: "cmd-stop-run-loop-001", Valid: true},
		Metadata:       []byte(`{"source":"control-plane"}`),
	})
	require.NoError(t, err)

	baseEventTime := time.Now().UTC().Add(-3 * time.Minute)
	_, err = testDB.Exec(ctx, `
		UPDATE task_events
		SET created_at = CASE id
			WHEN $1 THEN $4
			WHEN $2 THEN $5
			WHEN $3 THEN $6
			ELSE created_at
		END
		WHERE tenant_id = $7
		  AND id IN ($1, $2, $3)
	`, dispatchedTaskEvent.ID, taskEvent.ID, stopRequestedTaskEvent.ID, baseEventTime, baseEventTime.Add(time.Minute), baseEventTime.Add(2*time.Minute), tenantID)
	require.NoError(t, err)

	runEvents, err := testQueries.ListTaskEventsForRun(ctx, queries.ListTaskEventsForRunParams{
		TenantID: tenantID,
		TaskID:   createdRun.TaskID,
		RunID:    createdRun.RunID,
		Limit:    10,
		Offset:   0,
	})
	require.NoError(t, err)
	require.Len(t, runEvents, 3)
	assert.Equal(t, []string{"run_dispatched", "provider.text_delta", "stop_requested"}, []string{runEvents[0].EventType, runEvents[1].EventType, runEvents[2].EventType})
	assert.Equal(t, []int32{-1, 1, -2}, []int32{runEvents[0].SequenceNumber, runEvents[1].SequenceNumber, runEvents[2].SequenceNumber})

	session, err := testQueries.UpsertProviderSessionByExternalID(ctx, queries.UpsertProviderSessionByExternalIDParams{
		TenantID:            tenantID,
		ProviderSessionID:   "provider-session-001",
		DigitalEmployeeID:   digitalEmployeeID,
		ExecutionInstanceID: executionInstanceID,
		RuntimeNodeID:       runtimeNodeID,
		ProviderType:        "claude-code",
		Status:              "running",
		Recoverable:         true,
		SessionDisplayID:    pgtype.Text{String: "claude-code #001", Valid: true},
		SessionParams:       []byte(`{"model":"claude-sonnet"}`),
		SessionState:        []byte(`{"phase":"running"}`),
		LastSequenceNumber:  7,
		LastCommandID:       pgtype.Text{String: commandID, Valid: true},
		LastRunID:           uuid.NullUUID{UUID: createdRun.RunID, Valid: true},
		LastErrorFamily:     pgtype.Text{},
		Metadata:            []byte(`{"source":"latest-writeback"}`),
	})
	require.NoError(t, err)
	assert.Equal(t, "provider-session-001", session.ProviderSessionID)
	assert.Equal(t, int32(7), session.LastSequenceNumber)

	session, err = testQueries.UpsertProviderSessionByExternalID(ctx, queries.UpsertProviderSessionByExternalIDParams{
		TenantID:            tenantID,
		ProviderSessionID:   "provider-session-001",
		DigitalEmployeeID:   digitalEmployeeID,
		ExecutionInstanceID: executionInstanceID,
		RuntimeNodeID:       runtimeNodeID,
		ProviderType:        "claude-code",
		Status:              "idle",
		Recoverable:         true,
		SessionDisplayID:    pgtype.Text{},
		SessionParams:       []byte(`{"model":"claude-sonnet"}`),
		SessionState:        []byte(`{"phase":"idle"}`),
		LastSequenceNumber:  5,
		LastCommandID:       pgtype.Text{String: commandID, Valid: true},
		LastRunID:           uuid.NullUUID{UUID: createdRun.RunID, Valid: true},
		LastErrorFamily:     pgtype.Text{},
		Metadata:            []byte(`{"source":"stale-writeback"}`),
	})
	require.NoError(t, err)
	assert.Equal(t, "running", session.Status)
	assert.Equal(t, int32(7), session.LastSequenceNumber)
	assert.Equal(t, "claude-code #001", session.SessionDisplayID.String)
	assert.JSONEq(t, `{"phase":"running"}`, string(session.SessionState))
	assert.JSONEq(t, `{"source":"latest-writeback"}`, string(session.Metadata))

	providerEvent, err := testQueries.CreateProviderSessionEventIfAbsent(ctx, queries.CreateProviderSessionEventIfAbsentParams{
		TenantID:            tenantID,
		ProviderSessionUuid: session.ID,
		EventType:           "provider.text_delta",
		SequenceNumber:      7,
		Payload:             []byte(`{"text":"done"}`),
		RequestID:           pgtype.Text{String: "request-run-loop-001", Valid: true},
		CommandID:           pgtype.Text{String: commandID, Valid: true},
		RawEventRef:         pgtype.Text{String: "s3://superteam/raw/cmd-run-loop-001/7.json", Valid: true},
		LogRef:              pgtype.Text{String: "s3://superteam/logs/cmd-run-loop-001.log", Valid: true},
		SessionStatePatch:   []byte(`{"last_event":"done"}`),
		Metadata:            []byte(`{"source":"provider"}`),
	})
	require.NoError(t, err)

	duplicateProviderEvent, err := testQueries.CreateProviderSessionEventIfAbsent(ctx, queries.CreateProviderSessionEventIfAbsentParams{
		TenantID:            tenantID,
		ProviderSessionUuid: session.ID,
		EventType:           "provider.text_delta",
		SequenceNumber:      7,
		Payload:             []byte(`{"text":"duplicate"}`),
		RequestID:           pgtype.Text{String: "request-run-loop-001", Valid: true},
		CommandID:           pgtype.Text{String: commandID, Valid: true},
		RawEventRef:         pgtype.Text{String: "s3://superteam/raw/cmd-run-loop-001/duplicate.json", Valid: true},
		LogRef:              pgtype.Text{String: "s3://superteam/logs/cmd-run-loop-001.log", Valid: true},
		SessionStatePatch:   []byte(`{"last_event":"duplicate"}`),
		Metadata:            []byte(`{"source":"provider"}`),
	})
	require.NoError(t, err)
	assert.Equal(t, providerEvent.ID, duplicateProviderEvent.ID)
	assert.JSONEq(t, `{"text":"done"}`, string(duplicateProviderEvent.Payload))
	assert.JSONEq(t, `{"last_event":"done"}`, string(duplicateProviderEvent.SessionStatePatch))

	requestOnlyEvent, err := testQueries.CreateProviderSessionEventIfAbsent(ctx, queries.CreateProviderSessionEventIfAbsentParams{
		TenantID:            tenantID,
		ProviderSessionUuid: session.ID,
		EventType:           "provider.tool_call",
		SequenceNumber:      8,
		Payload:             []byte(`{"tool":"read_file"}`),
		RequestID:           pgtype.Text{String: "request-only-run-loop-001", Valid: true},
		RawEventRef:         pgtype.Text{String: "s3://superteam/raw/request-only/8.json", Valid: true},
		LogRef:              pgtype.Text{String: "s3://superteam/logs/request-only.log", Valid: true},
		SessionStatePatch:   []byte(`{"last_event":"tool_call"}`),
		Metadata:            []byte(`{"source":"provider"}`),
	})
	require.NoError(t, err)

	duplicateRequestOnlyEvent, err := testQueries.CreateProviderSessionEventIfAbsent(ctx, queries.CreateProviderSessionEventIfAbsentParams{
		TenantID:            tenantID,
		ProviderSessionUuid: session.ID,
		EventType:           "provider.tool_call",
		SequenceNumber:      8,
		Payload:             []byte(`{"tool":"duplicate"}`),
		RequestID:           pgtype.Text{String: "request-only-run-loop-001", Valid: true},
		RawEventRef:         pgtype.Text{String: "s3://superteam/raw/request-only/duplicate.json", Valid: true},
		LogRef:              pgtype.Text{String: "s3://superteam/logs/request-only.log", Valid: true},
		SessionStatePatch:   []byte(`{"last_event":"duplicate"}`),
		Metadata:            []byte(`{"source":"provider"}`),
	})
	require.NoError(t, err)
	assert.Equal(t, requestOnlyEvent.ID, duplicateRequestOnlyEvent.ID)
	assert.JSONEq(t, `{"tool":"read_file"}`, string(duplicateRequestOnlyEvent.Payload))

	otherSession, err := testQueries.UpsertProviderSessionByExternalID(ctx, queries.UpsertProviderSessionByExternalIDParams{
		TenantID:            tenantID,
		ProviderSessionID:   "provider-session-002",
		DigitalEmployeeID:   digitalEmployeeID,
		ExecutionInstanceID: executionInstanceID,
		RuntimeNodeID:       runtimeNodeID,
		ProviderType:        "claude-code",
		Status:              "running",
		Recoverable:         true,
		SessionDisplayID:    pgtype.Text{String: "claude-code #002", Valid: true},
		SessionParams:       []byte(`{"model":"claude-sonnet"}`),
		SessionState:        []byte(`{"phase":"running"}`),
		LastSequenceNumber:  7,
		LastCommandID:       pgtype.Text{String: commandID, Valid: true},
		LastRunID:           uuid.NullUUID{UUID: createdRun.RunID, Valid: true},
		LastErrorFamily:     pgtype.Text{},
		Metadata:            []byte(`{"source":"other-session"}`),
	})
	require.NoError(t, err)

	_, err = testQueries.CreateProviderSessionEventIfAbsent(ctx, queries.CreateProviderSessionEventIfAbsentParams{
		TenantID:            tenantID,
		ProviderSessionUuid: otherSession.ID,
		EventType:           "provider.text_delta",
		SequenceNumber:      7,
		Payload:             []byte(`{"text":"other session duplicate command"}`),
		RequestID:           pgtype.Text{String: "request-other-session", Valid: true},
		CommandID:           pgtype.Text{String: commandID, Valid: true},
		RawEventRef:         pgtype.Text{String: "s3://superteam/raw/other-session/7.json", Valid: true},
		LogRef:              pgtype.Text{String: "s3://superteam/logs/other-session.log", Valid: true},
		SessionStatePatch:   []byte(`{"last_event":"other-session"}`),
		Metadata:            []byte(`{"source":"provider"}`),
	})
	assert.ErrorIs(t, err, pgx.ErrNoRows)
}

// ============================================================================
// Auth Tests
// ============================================================================

func TestCreateUser(t *testing.T) {
	ctx := context.Background()

	user, err := testQueries.CreateUser(ctx, queries.CreateUserParams{
		Username:     "testuser",
		DisplayName:  pgtype.Text{String: "Test User", Valid: true},
		Email:        pgtype.Text{String: "test@example.com", Valid: true},
		PasswordHash: "$2a$10$hashedpassword",
		Status:       "active",
	})

	require.NoError(t, err)
	assert.Equal(t, "testuser", user.Username)
	assert.Equal(t, "Test User", user.DisplayName.String)
	assert.Equal(t, "test@example.com", user.Email.String)
	assert.Equal(t, "active", user.Status)

	// 清理
	defer testQueries.DeleteUser(ctx, user.ID)
}

func TestGetUserByUsername(t *testing.T) {
	ctx := context.Background()

	// 创建用户
	created, err := testQueries.CreateUser(ctx, queries.CreateUserParams{
		Username:     "findme",
		DisplayName:  pgtype.Text{String: "Find Me", Valid: true},
		Email:        pgtype.Text{String: "findme@example.com", Valid: true},
		PasswordHash: "$2a$10$hashedpassword",
		Status:       "active",
	})
	require.NoError(t, err)
	defer testQueries.DeleteUser(ctx, created.ID)

	// 按用户名查询
	user, err := testQueries.GetUserByUsername(ctx, "findme")
	require.NoError(t, err)
	assert.Equal(t, created.ID, user.ID)
	assert.Equal(t, "findme", user.Username)
}

func TestCreateRuntimeToken(t *testing.T) {
	ctx := context.Background()

	expiresAt := pgtype.Timestamptz{}
	expiresAt.Scan(time.Now().Add(24 * time.Hour))

	token, err := testQueries.CreateRuntimeToken(ctx, queries.CreateRuntimeTokenParams{
		NodeID:    "test-node-token-1",
		TokenHash: "hashed_token_value",
		ExpiresAt: expiresAt,
	})

	require.NoError(t, err)
	assert.Equal(t, "test-node-token-1", token.NodeID)
	assert.Equal(t, "hashed_token_value", token.TokenHash)

	// 清理
	defer testQueries.DeleteRuntimeToken(ctx, token.NodeID)
}

func TestValidateRuntimeToken(t *testing.T) {
	ctx := context.Background()

	// 创建有效 token
	expiresAt := pgtype.Timestamptz{}
	expiresAt.Scan(time.Now().Add(24 * time.Hour))

	_, err := testQueries.CreateRuntimeToken(ctx, queries.CreateRuntimeTokenParams{
		NodeID:    "test-node-validate",
		TokenHash: "valid_token_hash",
		ExpiresAt: expiresAt,
	})
	require.NoError(t, err)
	defer testQueries.DeleteRuntimeToken(ctx, "test-node-validate")

	// 验证正确的 token
	token, err := testQueries.ValidateRuntimeToken(ctx, queries.ValidateRuntimeTokenParams{
		NodeID:    "test-node-validate",
		TokenHash: "valid_token_hash",
	})
	require.NoError(t, err)
	assert.Equal(t, "test-node-validate", token.NodeID)

	// 验证错误的 token hash
	_, err = testQueries.ValidateRuntimeToken(ctx, queries.ValidateRuntimeTokenParams{
		NodeID:    "test-node-validate",
		TokenHash: "wrong_token_hash",
	})
	assert.Error(t, err) // 应该找不到
}

func TestExpiredRuntimeToken(t *testing.T) {
	ctx := context.Background()

	// 创建已过期的 token
	expiresAt := pgtype.Timestamptz{}
	expiresAt.Scan(time.Now().Add(-1 * time.Hour))

	_, err := testQueries.CreateRuntimeToken(ctx, queries.CreateRuntimeTokenParams{
		NodeID:    "test-node-expired",
		TokenHash: "expired_token_hash",
		ExpiresAt: expiresAt,
	})
	require.NoError(t, err)
	defer testQueries.DeleteRuntimeToken(ctx, "test-node-expired")

	// 验证过期的 token
	_, err = testQueries.ValidateRuntimeToken(ctx, queries.ValidateRuntimeTokenParams{
		NodeID:    "test-node-expired",
		TokenHash: "expired_token_hash",
	})
	assert.Error(t, err) // 应该找不到，因为已过期
}

func TestWebLoginLogs(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	user, err := testQueries.CreateUser(ctx, queries.CreateUserParams{
		Username:     "login-log-user",
		DisplayName:  pgtype.Text{String: "Login Log User", Valid: true},
		Email:        pgtype.Text{String: "login-log-user@example.com", Valid: true},
		PasswordHash: "$2a$10$hashedpassword",
		Status:       "active",
	})
	require.NoError(t, err)

	success, err := testQueries.CreateWebLoginLog(ctx, queries.CreateWebLoginLogParams{
		EventType: "login_succeeded",
		UserID:    uuid.NullUUID{UUID: user.ID, Valid: true},
		Username:  "login-log-user",
		SessionID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
		ClientIp:  pgtype.Text{String: "127.0.0.1", Valid: true},
		UserAgent: pgtype.Text{String: "test-agent", Valid: true},
		Result:    "succeeded",
	})
	require.NoError(t, err)
	require.Equal(t, "login_succeeded", success.EventType)
	require.True(t, success.UserID.Valid)
	assert.Equal(t, user.ID, success.UserID.UUID)

	failed, err := testQueries.CreateWebLoginLog(ctx, queries.CreateWebLoginLogParams{
		EventType:     "login_failed",
		Username:      "login-log-user",
		ClientIp:      pgtype.Text{String: "127.0.0.1", Valid: true},
		UserAgent:     pgtype.Text{String: "test-agent", Valid: true},
		Result:        "failed",
		FailureReason: pgtype.Text{String: "invalid_credentials", Valid: true},
	})
	require.NoError(t, err)
	require.Equal(t, "login_failed", failed.EventType)

	logs, err := testQueries.ListWebLoginLogs(ctx, queries.ListWebLoginLogsParams{
		Offset: 0,
		Limit:  10,
	})
	require.NoError(t, err)
	require.Len(t, logs, 2)
	assert.Equal(t, failed.ID, logs[0].ID)
	assert.Equal(t, success.ID, logs[1].ID)
	assert.Equal(t, "invalid_credentials", logs[0].FailureReason.String)
}

// ============================================================================
// Audit Tests
// ============================================================================

func TestCreateAuditEvent(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	details := map[string]interface{}{
		"action":  "create",
		"changes": []string{"field1", "field2"},
	}
	detailsJSON, err := json.Marshal(details)
	require.NoError(t, err)

	ip, _ := netip.ParseAddr("192.168.1.100")

	event, err := testQueries.CreateAuditEvent(ctx, queries.CreateAuditEventParams{
		EventType:    "task.created",
		ActorType:    "user",
		ActorID:      "user-123",
		ResourceType: pgtype.Text{String: "task", Valid: true},
		ResourceID:   pgtype.Text{String: "task-456", Valid: true},
		Action:       "create",
		Details:      detailsJSON,
		IpAddress:    &ip,
	})

	require.NoError(t, err)
	assert.Equal(t, "task.created", event.EventType)
	assert.Equal(t, "user", event.ActorType)
	assert.Equal(t, "user-123", event.ActorID)
	assert.Equal(t, "task", event.ResourceType.String)
	assert.Equal(t, "task-456", event.ResourceID.String)
	assert.Equal(t, "create", event.Action)
	assert.NotNil(t, event.IpAddress)
}

func TestListAuditEvents(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	// 创建多个审计事件
	detailsJSON, _ := json.Marshal(map[string]interface{}{"test": true})

	_, err := testQueries.CreateAuditEvent(ctx, queries.CreateAuditEventParams{
		EventType: "task.created",
		ActorType: "user",
		ActorID:   "user-1",
		Action:    "create",
		Details:   detailsJSON,
	})
	require.NoError(t, err)

	_, err = testQueries.CreateAuditEvent(ctx, queries.CreateAuditEventParams{
		EventType: "task.updated",
		ActorType: "user",
		ActorID:   "user-1",
		Action:    "update",
		Details:   detailsJSON,
	})
	require.NoError(t, err)

	_, err = testQueries.CreateAuditEvent(ctx, queries.CreateAuditEventParams{
		EventType: "node.registered",
		ActorType: "system",
		ActorID:   "system",
		Action:    "register",
		Details:   detailsJSON,
	})
	require.NoError(t, err)

	// 测试无过滤
	events, err := testQueries.ListAuditEvents(ctx, queries.ListAuditEventsParams{
		Offset: 0,
		Limit:  10,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(events), 3)

	// 测试按 actor_type 过滤
	userEvents, err := testQueries.ListAuditEvents(ctx, queries.ListAuditEventsParams{
		ActorType: pgtype.Text{String: "user", Valid: true},
		Offset:    0,
		Limit:     10,
	})
	require.NoError(t, err)
	for _, event := range userEvents {
		assert.Equal(t, "user", event.ActorType)
	}

	// 测试按 event_type 过滤
	createdEvents, err := testQueries.ListAuditEvents(ctx, queries.ListAuditEventsParams{
		EventType: pgtype.Text{String: "task.created", Valid: true},
		Offset:    0,
		Limit:     10,
	})
	require.NoError(t, err)
	for _, event := range createdEvents {
		assert.Equal(t, "task.created", event.EventType)
	}
}

func TestListTeamAuditEvents(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	otherTenantID := uuid.New()
	teamID := uuid.New()
	otherTeamID := uuid.New()
	detailsJSON, err := json.Marshal(map[string]interface{}{"test": true})
	require.NoError(t, err)

	_, err = testDB.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, status)
		VALUES ($1, 'other-audit-tenant', 'Other Audit Tenant', 'active')
	`, otherTenantID)
	require.NoError(t, err)

	included, err := testQueries.CreateAuditEvent(ctx, queries.CreateAuditEventParams{
		TenantID:     uuid.NullUUID{UUID: tenantID, Valid: true},
		EventType:    "team.updated",
		ActorType:    "user",
		ActorID:      "auditor",
		ResourceType: pgtype.Text{String: "team", Valid: true},
		ResourceID:   pgtype.Text{String: teamID.String(), Valid: true},
		Action:       "team.update",
		Details:      detailsJSON,
	})
	require.NoError(t, err)

	_, err = testQueries.CreateAuditEvent(ctx, queries.CreateAuditEventParams{
		TenantID:     uuid.NullUUID{UUID: otherTenantID, Valid: true},
		EventType:    "team.updated",
		ActorType:    "user",
		ActorID:      "auditor",
		ResourceType: pgtype.Text{String: "team", Valid: true},
		ResourceID:   pgtype.Text{String: teamID.String(), Valid: true},
		Action:       "team.update",
		Details:      detailsJSON,
	})
	require.NoError(t, err)

	_, err = testQueries.CreateAuditEvent(ctx, queries.CreateAuditEventParams{
		TenantID:     uuid.NullUUID{UUID: tenantID, Valid: true},
		EventType:    "team.updated",
		ActorType:    "user",
		ActorID:      "auditor",
		ResourceType: pgtype.Text{String: "team", Valid: true},
		ResourceID:   pgtype.Text{String: otherTeamID.String(), Valid: true},
		Action:       "team.update",
		Details:      detailsJSON,
	})
	require.NoError(t, err)

	_, err = testQueries.CreateAuditEvent(ctx, queries.CreateAuditEventParams{
		TenantID:     uuid.NullUUID{UUID: tenantID, Valid: true},
		EventType:    "task.updated",
		ActorType:    "user",
		ActorID:      "auditor",
		ResourceType: pgtype.Text{String: "task", Valid: true},
		ResourceID:   pgtype.Text{String: teamID.String(), Valid: true},
		Action:       "team.update",
		Details:      detailsJSON,
	})
	require.NoError(t, err)

	_, err = testQueries.CreateAuditEvent(ctx, queries.CreateAuditEventParams{
		TenantID:     uuid.NullUUID{UUID: tenantID, Valid: true},
		EventType:    "team.read",
		ActorType:    "user",
		ActorID:      "auditor",
		ResourceType: pgtype.Text{String: "team", Valid: true},
		ResourceID:   pgtype.Text{String: teamID.String(), Valid: true},
		Action:       "authz.check",
		Details:      detailsJSON,
	})
	require.NoError(t, err)

	events, err := testQueries.ListTeamAuditEvents(ctx, queries.ListTeamAuditEventsParams{
		TenantID: tenantID,
		TeamID:   teamID,
		Offset:   0,
		Limit:    10,
	})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, included.ID, events[0].ID)
	assert.Equal(t, tenantID, events[0].TenantID)
	assert.Equal(t, "team", events[0].ResourceType.String)
	assert.Equal(t, teamID.String(), events[0].ResourceID.String)
	assert.Equal(t, "team.update", events[0].Action)
}

func TestCountAuditEvents(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	// 创建一些审计事件
	detailsJSON, _ := json.Marshal(map[string]interface{}{"test": true})

	for i := 0; i < 5; i++ {
		_, err := testQueries.CreateAuditEvent(ctx, queries.CreateAuditEventParams{
			EventType: "test.event",
			ActorType: "test",
			ActorID:   "test-actor",
			Action:    "test",
			Details:   detailsJSON,
		})
		require.NoError(t, err)
	}

	// 统计事件数量
	count, err := testQueries.CountAuditEvents(ctx, queries.CountAuditEventsParams{
		EventType: pgtype.Text{String: "test.event", Valid: true},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(5), count)
}

func TestAuditEventsTimeFilter(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	detailsJSON, _ := json.Marshal(map[string]interface{}{"test": true})

	// 创建事件
	_, err := testQueries.CreateAuditEvent(ctx, queries.CreateAuditEventParams{
		EventType: "time.test",
		ActorType: "test",
		ActorID:   "test-actor",
		Action:    "test",
		Details:   detailsJSON,
	})
	require.NoError(t, err)

	// 使用时间过滤查询
	startTime := pgtype.Timestamptz{}
	startTime.Scan(time.Now().Add(-1 * time.Hour))

	endTime := pgtype.Timestamptz{}
	endTime.Scan(time.Now().Add(1 * time.Hour))

	events, err := testQueries.ListAuditEvents(ctx, queries.ListAuditEventsParams{
		EventType: pgtype.Text{String: "time.test", Valid: true},
		StartTime: startTime,
		EndTime:   endTime,
		Offset:    0,
		Limit:     10,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(events), 1)
}

// ============================================================================
// Error Case Tests
// ============================================================================

func TestCreateUser_DuplicateUsername(t *testing.T) {
	ctx := context.Background()

	// 创建第一个用户
	user1, err := testQueries.CreateUser(ctx, queries.CreateUserParams{
		Username:     "duplicate-user",
		DisplayName:  pgtype.Text{String: "User 1", Valid: true},
		Email:        pgtype.Text{String: "user1@example.com", Valid: true},
		PasswordHash: "$2a$10$hashedpassword",
		Status:       "active",
	})
	require.NoError(t, err)
	defer testQueries.DeleteUser(ctx, user1.ID)

	// 尝试创建重复用户名的用户
	_, err = testQueries.CreateUser(ctx, queries.CreateUserParams{
		Username:     "duplicate-user",
		DisplayName:  pgtype.Text{String: "User 2", Valid: true},
		Email:        pgtype.Text{String: "user2@example.com", Valid: true},
		PasswordHash: "$2a$10$hashedpassword",
		Status:       "active",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate key value")
}

func TestCreateRuntimeNode_DuplicateNodeID(t *testing.T) {
	ctx := context.Background()

	providersJSON, _ := json.Marshal(map[string]interface{}{"providers": []string{"claude-code"}})
	metadataJSON, _ := json.Marshal(map[string]interface{}{"test": true})
	now := pgtype.Timestamptz{}
	now.Scan(time.Now())

	// 创建第一个节点
	node1, err := testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "duplicate-node",
		Name:               "Node 1",
		SupportedProviders: providersJSON,
		MaxSlots:           2,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           metadataJSON,
		LastHeartbeatAt:    now,
	})
	require.NoError(t, err)
	defer testQueries.DeleteRuntimeNode(ctx, node1.NodeID)

	// 重复 node_id 会按注册 upsert 语义刷新同一节点
	node2, err := testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "duplicate-node",
		Name:               "Node 2",
		SupportedProviders: providersJSON,
		MaxSlots:           4,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           metadataJSON,
		LastHeartbeatAt:    now,
	})
	require.NoError(t, err)
	assert.Equal(t, node1.ID, node2.ID)
	assert.Equal(t, "Node 2", node2.Name)
	assert.Equal(t, int32(4), node2.MaxSlots)
}

func TestCreateTask_PreservesCreatorUUIDWithoutForeignKey(t *testing.T) {
	ctx := context.Background()

	paramsJSON, _ := json.Marshal(map[string]interface{}{"test": true})
	creatorID := uuid.New()

	task, err := testQueries.CreateTask(ctx, queries.CreateTaskParams{
		Title:        "Invalid Creator Task",
		Description:  pgtype.Text{String: "Task with application-validated creator reference", Valid: true},
		Status:       "pending",
		Priority:     3,
		ProviderType: "claude-code",
		CreatorID:    uuid.NullUUID{UUID: creatorID, Valid: true},
		Params:       paramsJSON,
	})
	require.NoError(t, err)
	defer testQueries.DeleteTask(ctx, queries.DeleteTaskParams{ID: task.ID})
	require.True(t, task.CreatorID.Valid)
	assert.Equal(t, creatorID, task.CreatorID.UUID)
}

func TestCreateUser_DuplicateEmail(t *testing.T) {
	ctx := context.Background()

	// 创建第一个用户
	user1, err := testQueries.CreateUser(ctx, queries.CreateUserParams{
		Username:     "user-email-1",
		DisplayName:  pgtype.Text{String: "User 1", Valid: true},
		Email:        pgtype.Text{String: "duplicate@example.com", Valid: true},
		PasswordHash: "$2a$10$hashedpassword",
		Status:       "active",
	})
	require.NoError(t, err)
	defer testQueries.DeleteUser(ctx, user1.ID)

	// 尝试创建重复邮箱的用户
	_, err = testQueries.CreateUser(ctx, queries.CreateUserParams{
		Username:     "user-email-2",
		DisplayName:  pgtype.Text{String: "User 2", Valid: true},
		Email:        pgtype.Text{String: "duplicate@example.com", Valid: true},
		PasswordHash: "$2a$10$hashedpassword",
		Status:       "active",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate key value")
}

func TestCreateRuntimeToken_DuplicateNodeID(t *testing.T) {
	ctx := context.Background()

	expiresAt := pgtype.Timestamptz{}
	expiresAt.Scan(time.Now().Add(24 * time.Hour))

	// 创建第一个 token
	token1, err := testQueries.CreateRuntimeToken(ctx, queries.CreateRuntimeTokenParams{
		NodeID:    "duplicate-token-node",
		TokenHash: "hash1",
		ExpiresAt: expiresAt,
	})
	require.NoError(t, err)
	defer testQueries.DeleteRuntimeToken(ctx, token1.NodeID)

	// 同一个 node_id 的有效 token 会按 upsert 语义刷新哈希和过期时间
	token2, err := testQueries.CreateRuntimeToken(ctx, queries.CreateRuntimeTokenParams{
		NodeID:    "duplicate-token-node",
		TokenHash: "hash2",
		ExpiresAt: expiresAt,
	})
	require.NoError(t, err)
	assert.Equal(t, token1.ID, token2.ID)
	assert.Equal(t, "hash2", token2.TokenHash)
}
