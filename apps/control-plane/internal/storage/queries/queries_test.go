package queries_test

import (
	"context"
	"encoding/json"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/superteam/control-plane/internal/storage/queries"
)

var (
	testDB      *pgxpool.Pool
	testQueries *queries.Queries
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	// 启动 PostgreSQL 容器
	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("superteam_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		panic(err)
	}

	// 获取连接字符串
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic(err)
	}

	// 连接数据库
	testDB, err = pgxpool.New(ctx, connStr)
	if err != nil {
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
	if err := testcontainers.TerminateContainer(pgContainer); err != nil {
		panic(err)
	}

	os.Exit(code)
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	// 读取迁移文件
	migrationSQL, err := os.ReadFile("../migrations/001_initial.sql")
	if err != nil {
		return err
	}

	// 执行迁移
	_, err = pool.Exec(ctx, string(migrationSQL))
	return err
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
		PasswordHash: pgtype.Text{String: "$2a$10$hashedpassword", Valid: true},
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
		CreatorID:     pgtype.Int8{Int64: user.ID, Valid: true},
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
	defer testQueries.DeleteTask(ctx, task.ID)
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
	defer testQueries.DeleteTask(ctx, created.ID)

	// 查询任务
	task, err := testQueries.GetTask(ctx, created.ID)
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
	defer testQueries.DeleteTask(ctx, task1.ID)

	task2, err := testQueries.CreateTask(ctx, queries.CreateTaskParams{
		Title:        "Task 2",
		Status:       "running",
		Priority:     3,
		ProviderType: "opencode",
		Params:       paramsJSON,
	})
	require.NoError(t, err)
	defer testQueries.DeleteTask(ctx, task2.ID)

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
	defer testQueries.DeleteTask(ctx, task.ID)

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
	defer testQueries.DeleteTask(ctx, task.ID)

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
	defer testQueries.DeleteTask(ctx, task.ID)

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
	events, err := testQueries.ListTaskEvents(ctx, task.ID)
	require.NoError(t, err)
	assert.Len(t, events, 2)
	assert.Equal(t, int32(1), events[0].SequenceNumber)
	assert.Equal(t, int32(2), events[1].SequenceNumber)

	// 获取最新序列号
	maxSeq, err := testQueries.GetLatestTaskEventSequence(ctx, task.ID)
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
		PasswordHash: pgtype.Text{String: "$2a$10$hashedpassword", Valid: true},
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
		PasswordHash: pgtype.Text{String: "$2a$10$hashedpassword", Valid: true},
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

// ============================================================================
// Audit Tests
// ============================================================================

func TestCreateAuditEvent(t *testing.T) {
	ctx := context.Background()

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
	assert.GreaterOrEqual(t, count, int64(5))
}

func TestAuditEventsTimeFilter(t *testing.T) {
	ctx := context.Background()

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
