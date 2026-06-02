package queries_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
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

	migrated, err := schemaAlreadyMigrated(ctx, conn)
	if err != nil {
		return err
	}
	if migrated {
		return nil
	}

	// 发现所有迁移文件
	migrationsDir := filepath.Join("..", "migrations")
	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil {
		return err
	}

	// 按文件名排序
	sort.Strings(files)

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

func schemaAlreadyMigrated(ctx context.Context, conn *pgx.Conn) (bool, error) {
	const query = `
SELECT EXISTS (
	SELECT 1
	FROM information_schema.tables
	WHERE table_schema = current_schema()
		AND table_name = 'runtime_nodes'
)
`

	var exists bool
	if err := conn.QueryRow(ctx, query).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// cleanupTestData 清理测试数据，避免测试之间相互影响
func cleanupTestData(t *testing.T, db *pgxpool.Pool) {
	ctx := context.Background()
	_, err := db.Exec(ctx, `
		TRUNCATE
			provider_session_events,
			provider_sessions,
			digital_employee_execution_instances,
			digital_employees,
			runtime_capabilities,
			runtime_sessions,
			runtime_enrollments,
			runtime_bootstrap_keys,
			web_operation_logs,
			web_login_logs,
			audit_events,
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
		VALUES ($1, $2, $3, 'team', $3::text, 'active')
	`, tenantID, node.ID, teamID)
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
		VALUES ($1, $2, $3, 'tenant', $1::text, 'active')
	`, tenantID, nodeWithTeamID.ID, teamID)
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
		VALUES ($1, 'user', $2, 'owner', 'active');

		INSERT INTO tenant_members (tenant_id, team_id, principal_type, principal_id, role, status)
		VALUES ($1, $3, 'user', $2, 'developer', 'active');

		INSERT INTO tenant_members (tenant_id, principal_type, principal_id, role, status)
		VALUES ($1, 'user', $4, 'viewer', 'active');

		INSERT INTO tenant_members (tenant_id, principal_type, principal_id, role, status)
		VALUES ($5, 'user', $6, 'owner', 'active');
	`, tenantID, multiMemberUser.ID, teamID, otherUser.ID, otherTenantID, otherTenantUser.ID)
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
		VALUES ($1, 'authz-other', 'Authz Other Tenant', 'active');

		INSERT INTO tenant_teams (id, tenant_id, slug, name, status)
		VALUES ($2, $1, 'authz-other', 'Authz Other Team', 'active');
	`, otherTenantID, otherTeamID)
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

	enrollment, err := testQueries.UpsertRuntimeEnrollment(ctx, queries.UpsertRuntimeEnrollmentParams{
		TenantID:       tenantID,
		RuntimeNodeID:  node.ID,
		NodeID:         "runtime-enroll-node",
		BootstrapKeyID: uuid.NullUUID{UUID: bootstrapKey.ID, Valid: true},
		Status:         "pending",
		RequestPayload: []byte(`{"node_id":"runtime-enroll-node","version":"0.1.0"}`),
		LastHelloAt:    now,
	})
	require.NoError(t, err)
	assert.Equal(t, "pending", enrollment.Status)

	otherEnrollment, err := testQueries.UpsertRuntimeEnrollment(ctx, queries.UpsertRuntimeEnrollmentParams{
		TenantID:       otherTenantID,
		RuntimeNodeID:  uuid.New(),
		NodeID:         "runtime-enroll-node",
		BootstrapKeyID: uuid.NullUUID{UUID: bootstrapKey.ID, Valid: true},
		Status:         "pending",
		RequestPayload: []byte(`{"node_id":"runtime-enroll-node","tenant":"other"}`),
		LastHelloAt:    now,
	})
	require.NoError(t, err)
	assert.Equal(t, otherTenantID, otherEnrollment.TenantID)
	assert.NotEqual(t, enrollment.ID, otherEnrollment.ID)

	defaultEnrollment, err := testQueries.GetRuntimeEnrollmentByNodeID(ctx, queries.GetRuntimeEnrollmentByNodeIDParams{
		TenantID: tenantID,
		NodeID:   "runtime-enroll-node",
	})
	require.NoError(t, err)
	assert.Equal(t, enrollment.ID, defaultEnrollment.ID)
	assert.Equal(t, tenantID, defaultEnrollment.TenantID)

	approved, err := testQueries.ApproveRuntimeEnrollment(ctx, queries.ApproveRuntimeEnrollmentParams{
		ID:         enrollment.ID,
		TenantID:   tenantID,
		ApprovedBy: uuid.NullUUID{UUID: uuid.New(), Valid: true},
	})
	require.NoError(t, err)
	assert.Equal(t, "approved", approved.Status)
	require.True(t, approved.ApprovedAt.Valid)

	sessionExpiresAt := pgtype.Timestamptz{}
	require.NoError(t, sessionExpiresAt.Scan(time.Now().Add(12*time.Hour)))
	session, err := testQueries.CreateRuntimeSession(ctx, queries.CreateRuntimeSessionParams{
		TenantID:        tenantID,
		RuntimeNodeID:   node.ID,
		EnrollmentID:    uuid.NullUUID{UUID: approved.ID, Valid: true},
		TokenLookupHash: "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		TokenSecretHash: "$2a$10$runtimeSessionSecretHashForLaterValidation",
		ExpiresAt:       sessionExpiresAt,
	})
	require.NoError(t, err)

	validatedSession, err := testQueries.GetActiveRuntimeSessionByLookupHash(ctx, queries.GetActiveRuntimeSessionByLookupHashParams{
		TenantID:        tenantID,
		TokenLookupHash: "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
	})
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

	otherCapability, err := testQueries.UpsertRuntimeCapability(ctx, queries.UpsertRuntimeCapabilityParams{
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
	require.NoError(t, err)
	assert.NotEqual(t, capability.ID, otherCapability.ID)
	assert.Equal(t, otherTenantID, otherCapability.TenantID)

	capabilities, err := testQueries.ListRuntimeCapabilities(ctx, queries.ListRuntimeCapabilitiesParams{
		TenantID:      tenantID,
		RuntimeNodeID: node.ID,
	})
	require.NoError(t, err)
	require.Len(t, capabilities, 1)
	assert.Equal(t, capability.ID, capabilities[0].ID)

	revokedSession, err := testQueries.RevokeRuntimeSession(ctx, queries.RevokeRuntimeSessionParams{
		ID:            session.ID,
		TenantID:      tenantID,
		RevokedReason: pgtype.Text{String: "administrator revoked enrollment", Valid: true},
	})
	require.NoError(t, err)
	require.True(t, revokedSession.RevokedAt.Valid)

	_, err = testQueries.GetActiveRuntimeSessionByLookupHash(ctx, queries.GetActiveRuntimeSessionByLookupHashParams{
		TenantID:        tenantID,
		TokenLookupHash: "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
	})
	assert.Error(t, err)
}

func TestDigitalEmployeeExecutionQueries(t *testing.T) {
	ctx := context.Background()
	cleanupTestData(t, testDB)

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")

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

	eventPayload1 := []byte(`{"message":"session started"}`)
	event1, err := testQueries.CreateProviderSessionEvent(ctx, queries.CreateProviderSessionEventParams{
		TenantID:          tenantID,
		ProviderSessionID: session.ID,
		EventType:         "message_delta",
		SequenceNumber:    1,
		Payload:           eventPayload1,
		RawEventRef:       pgtype.Text{String: "s3://superteam/raw/session-001/1.json", Valid: true},
		Metadata:          []byte(`{"channel":"stdout"}`),
	})
	require.NoError(t, err)
	assert.Equal(t, employee.ID, event1.DigitalEmployeeID)
	assert.Equal(t, instance.ID, event1.ExecutionInstanceID)
	assert.Equal(t, node.ID, event1.RuntimeNodeID)
	assert.Equal(t, "claude-code", event1.ProviderType)

	eventPayload2 := []byte(`{"tool":"write_file"}`)
	_, err = testQueries.CreateProviderSessionEvent(ctx, queries.CreateProviderSessionEventParams{
		TenantID:          tenantID,
		ProviderSessionID: session.ID,
		EventType:         "tool_call",
		SequenceNumber:    2,
		Payload:           eventPayload2,
		Metadata:          []byte(`{"channel":"json_stream"}`),
	})
	require.NoError(t, err)

	_, err = testQueries.CreateProviderSessionEvent(ctx, queries.CreateProviderSessionEventParams{
		TenantID:          uuid.New(),
		ProviderSessionID: session.ID,
		EventType:         "message_delta",
		SequenceNumber:    3,
		Payload:           []byte(`{"message":"wrong tenant"}`),
		Metadata:          []byte(`{}`),
	})
	assert.Error(t, err)

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

	// 尝试创建重复 node_id 的节点
	_, err = testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "duplicate-node",
		Name:               "Node 2",
		SupportedProviders: providersJSON,
		MaxSlots:           4,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           metadataJSON,
		LastHeartbeatAt:    now,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate key value")
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

	// 尝试为同一个 node_id 创建第二个 token
	_, err = testQueries.CreateRuntimeToken(ctx, queries.CreateRuntimeTokenParams{
		NodeID:    "duplicate-token-node",
		TokenHash: "hash2",
		ExpiresAt: expiresAt,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate key value")
}
