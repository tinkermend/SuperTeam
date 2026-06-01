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
		TenantID: tenantID,
		TeamID:   uuid.NullUUID{UUID: teamID, Valid: true},
		TaskID:   task.ID,
		NodeID:   "authz-node-001",
	})
	require.NoError(t, err)
	assert.True(t, covered)

	missingNodeCovered, err := testQueries.RuntimeNodeCoversTaskScope(ctx, queries.RuntimeNodeCoversTaskScopeParams{
		TenantID: tenantID,
		TeamID:   uuid.NullUUID{UUID: teamID, Valid: true},
		TaskID:   task.ID,
		NodeID:   "authz-node-missing",
	})
	require.NoError(t, err)
	assert.False(t, missingNodeCovered)
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
