package employee

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/superteam/control-plane/internal/storage/queries"
)

func TestDigitalEmployeeRunStatusTerminal(t *testing.T) {
	require.True(t, DigitalEmployeeRunStatusCompleted.IsTerminal())
	require.True(t, DigitalEmployeeRunStatusFailed.IsTerminal())
	require.True(t, DigitalEmployeeRunStatusCancelled.IsTerminal())
	require.True(t, DigitalEmployeeRunStatusTimedOut.IsTerminal())
	require.False(t, DigitalEmployeeRunStatusRunning.IsTerminal())
	require.False(t, DigitalEmployeeRunStatusCancelling.IsTerminal())
}

func TestRuntimeWritebackEventRedactsSensitivePayload(t *testing.T) {
	event := RuntimeCommandEventWriteback{
		EventType:      "text_delta",
		SequenceNumber: 1,
		Payload: map[string]any{
			"text":          "ok",
			"authorization": "Bearer secret",
			"nested": map[string]any{
				"token": "secret",
			},
			"events": []any{
				map[string]any{"token": "array item is redacted"},
				"scalar stays intact",
			},
		},
	}

	redacted := redactRuntimeEventPayload(event.Payload)

	require.Equal(t, "[redacted]", redacted["authorization"])
	require.Equal(t, "[redacted]", redacted["nested"].(map[string]any)["token"])
	events := redacted["events"].([]any)
	require.Equal(t, "[redacted]", events[0].(map[string]any)["token"])
	require.Equal(t, "scalar stays intact", events[1])
}

func TestDigitalEmployeeRunFromQueryMapsProviderTypeAndJSONFields(t *testing.T) {
	run := queries.TaskRun{
		ID:                  uuid.New(),
		TenantID:            uuid.New(),
		TaskID:              uuid.New(),
		NodeID:              "runtime-a",
		RuntimeNodeID:       uuid.NullUUID{UUID: uuid.New(), Valid: true},
		Status:              string(DigitalEmployeeRunStatusCompleted),
		ProviderType:        pgtype.Text{String: "codex", Valid: true},
		Result:              []byte(`{"summary":"done"}`),
		Diagnostic:          []byte(`{"duration_ms":1200}`),
		WorkProducts:        []byte(`[{"type":"report","title":"Run report","summary":"ok","ref":"s3://bucket/report.json","metadata":{"format":"json"},"created_at":"2026-06-04T12:00:00Z"}]`),
		SessionState:        []byte(`{"provider_cursor":"abc"}`),
		DigitalEmployeeID:   uuid.NullUUID{UUID: uuid.New(), Valid: true},
		ExecutionInstanceID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
		CommandID:           pgtype.Text{String: "cmd-1", Valid: true},
		CreatedAt:           pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		UpdatedAt:           pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	mapped := digitalEmployeeRunFromQuery(run)

	require.Equal(t, "codex", mapped.ProviderType)
	require.Equal(t, "done", mapped.Result["summary"])
	require.Equal(t, float64(1200), mapped.Diagnostic["duration_ms"])
	require.Equal(t, "abc", mapped.SessionState["provider_cursor"])
	require.Len(t, mapped.WorkProducts, 1)
	require.Equal(t, "report", mapped.WorkProducts[0].Type)
	require.Equal(t, "Run report", mapped.WorkProducts[0].Title)
	require.Equal(t, "json", mapped.WorkProducts[0].Metadata["format"])
}

func TestRuntimeCommandEventFromTaskEventMapsPersistedEventFields(t *testing.T) {
	logRef := "s3://logs/run.log"
	rawRef := "s3://events/1.json"
	event := queries.TaskEvent{
		EventType:      "text_delta",
		SequenceNumber: 7,
		Payload:        []byte(`{"text":"ok","token":"[redacted]"}`),
		LogRef:         pgtype.Text{String: logRef, Valid: true},
		RawEventRef:    pgtype.Text{String: rawRef, Valid: true},
		Metadata:       []byte(`{"provider":"codex"}`),
	}

	mapped := runtimeCommandEventFromTaskEvent(event)

	require.Equal(t, "text_delta", mapped.EventType)
	require.Equal(t, int32(7), mapped.SequenceNumber)
	require.Equal(t, "ok", mapped.Payload["text"])
	require.Equal(t, "[redacted]", mapped.Payload["token"])
	require.Equal(t, &logRef, mapped.LogRef)
	require.Equal(t, &rawRef, mapped.RawEventRef)
	require.Equal(t, "codex", mapped.Metadata["provider"])
}

func TestRunPreflightFromQueryAllowsMissingTeam(t *testing.T) {
	_, err := runPreflightFromQuery(queries.GetDigitalEmployeeRunPreflightRow{
		TenantID:              uuid.New(),
		TeamID:                uuid.NullUUID{},
		DigitalEmployeeID:     uuid.New(),
		DigitalEmployeeStatus: string(DigitalEmployeeStatusReady),
		ExecutionInstanceID:   uuid.New(),
		ExecutionStatus:       string(ExecutionInstanceStatusReady),
		RuntimeNodeID:         uuid.New(),
		NodeID:                "runtime-authoritative",
		ProviderType:          "codex",
		AgentHomeDir:          "/var/lib/superteam/agents/employee",
		RuntimeSelector:       []byte(`{}`),
		SessionPolicy:         []byte(`{}`),
		WorkspacePolicy:       []byte(`{}`),
	})

	require.NoError(t, err)
}

func TestRunPreflightDailyTokenUsageDefaultsEmptySumToZero(t *testing.T) {
	normalized := strings.Join(strings.Fields(queries.GetDigitalEmployeeRunPreflight), " ")

	require.Contains(t, normalized, "LEAST( COALESCE( SUM(")
	require.Contains(t, normalized, "), 0 ), 2147483647 )::integer AS usage_tokens_today")
}

func TestMapCreateRunErrorMapsIdempotencyFingerprintMismatch(t *testing.T) {
	idempotencyKey := "idem-1"

	err := mapCreateRunError(pgx.ErrNoRows, CreateRunRecordRequest{
		IdempotencyKey: &idempotencyKey,
	})

	require.ErrorIs(t, err, ErrConflict)
	require.Contains(t, err.Error(), "idempotency fingerprint mismatch")

	err = mapCreateRunError(pgx.ErrNoRows, CreateRunRecordRequest{})
	require.ErrorIs(t, err, pgx.ErrNoRows)
}

func TestPgRunRepositoryCreateRunAllowsMissingTeam(t *testing.T) {
	idempotencyKey := "idem-team-less"
	repo := NewPgRunRepository(queries.New(fakeRunRepositoryDBTX{rowErr: pgx.ErrNoRows}))
	req := validCreateRunRecordRequest(idempotencyKey)
	req.TeamID = uuid.Nil

	_, err := repo.CreateRun(context.Background(), req)

	require.ErrorIs(t, err, ErrConflict)
	require.Contains(t, err.Error(), "idempotency fingerprint mismatch")
}

func TestPgRunRepositoryCreateRunMapsIdempotencyFingerprintMismatch(t *testing.T) {
	idempotencyKey := "idem-1"
	repo := NewPgRunRepository(queries.New(fakeRunRepositoryDBTX{rowErr: pgx.ErrNoRows}))

	_, err := repo.CreateRun(context.Background(), validCreateRunRecordRequest(idempotencyKey))

	require.ErrorIs(t, err, ErrConflict)
	require.Contains(t, err.Error(), "idempotency fingerprint mismatch")
}

func TestPgRunRepositoryGetRunPreflightUsesRuntimeNodeIDFromRuntimeNodes(t *testing.T) {
	ctx := context.Background()
	cfg, ok := employeeRunRepositoryTestConfig()
	if !ok {
		t.Skip("set TEST_DATABASE_URL and TEST_REDIS_URL, or set ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 with DATABASE_URL and REDIS_URL")
	}
	require.NoError(t, pingEmployeeRunRepositoryTestRedis(ctx, cfg.redisURL))

	conn, err := pgx.Connect(ctx, cfg.databaseURL)
	require.NoError(t, err)
	defer conn.Close(ctx)

	schemaName := "employee_run_repo_" + strings.ReplaceAll(strings.ToLower(uuid.NewString()), "-", "_")
	_, err = conn.Exec(ctx, `CREATE SCHEMA `+schemaName)
	require.NoError(t, err)
	defer conn.Exec(ctx, `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)

	_, err = conn.Exec(ctx, `SET search_path TO `+schemaName)
	require.NoError(t, err)
	require.NoError(t, runEmployeeRepositoryTestMigrations(ctx, conn))

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	runtimeNodeID := uuid.New()
	employeeID := uuid.New()
	executionInstanceID := uuid.New()
	employeeConfigRevisionID := uuid.New()
	authoritativeNodeID := "runtime-authoritative"

	_, err = conn.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, status)
		VALUES ($1, 'default', '默认租户', 'active')
		ON CONFLICT (id) DO UPDATE SET status = EXCLUDED.status;

		INSERT INTO tenant_teams (id, tenant_id, slug, name, status)
		VALUES ($2, $1, 'default', '默认团队', 'active')
		ON CONFLICT (id) DO UPDATE SET status = EXCLUDED.status;

		INSERT INTO runtime_nodes (
			id,
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
			$3,
			$1,
			$4,
			'Runtime Authoritative',
			'["codex"]'::jsonb,
			2,
			0,
			'online',
			'{}'::jsonb,
			NOW()
		);

		INSERT INTO runtime_capabilities (
			tenant_id,
			runtime_node_id,
			capability_type,
			capability_key,
			provider_type,
			provider_version,
			binary_path,
			available,
			workspace_base_dir,
			capacity,
			labels,
			status,
			details,
			health_status,
			metadata,
			last_seen_at
		) VALUES (
			$1,
			$3,
			'provider',
			'provider:codex',
			'codex',
			'1.0.0',
			'/usr/local/bin/codex',
			true,
			'/tmp/superteam',
			'{}'::jsonb,
			'{}'::jsonb,
			'healthy',
			'{}'::jsonb,
			'healthy',
			'{}'::jsonb,
			NOW()
		);

		INSERT INTO digital_employees (
			id,
			tenant_id,
			team_id,
			name,
			role,
			status,
			permission_policy,
			context_policy,
			approval_policy,
			risk_level,
			metadata
		) VALUES (
			$5,
			$1,
			$2,
			'执行员工',
			'operator',
			'ready',
			'{}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			'normal',
			'{}'::jsonb
		);

		INSERT INTO digital_employee_execution_instances (
			id,
			tenant_id,
			digital_employee_id,
			runtime_node_id,
			provider_type,
			agent_home_dir,
			workspace_policy,
			session_policy,
			runtime_selector,
			capacity_requirements,
			fallback_policy,
			status,
			ready_at,
			metadata
		) VALUES (
			$6,
			$1,
			$5,
			$3,
			'codex',
			'/var/lib/superteam/agents/employee',
			'{"workspace":"isolated"}'::jsonb,
			'{"resume":true}'::jsonb,
			'{"node_id":"wrong-selector-value"}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			'ready',
			NOW(),
			'{}'::jsonb
		);

		INSERT INTO digital_employee_config_revisions (
			id,
			tenant_id,
			digital_employee_id,
			revision_number,
			persona_memory_markdown,
			capability_bindings,
			budget_policy,
			status
		) VALUES (
			$7,
			$1,
			$5,
			1,
			'# 人格画像
证据优先',
			'{"skills":["incident-diagnosis"],"mcp_servers":["postgres-readonly"],"external_capabilities":[],"environment_variable_refs":["PG_DSN"]}'::jsonb,
			'{}'::jsonb,
			'draft'
		);
	`, tenantID, teamID, runtimeNodeID, authoritativeNodeID, employeeID, executionInstanceID, employeeConfigRevisionID)
	require.NoError(t, err)

	repo := NewPgRunRepository(queries.New(conn))
	preflight, err := repo.GetRunPreflight(ctx, tenantID, employeeID)

	require.NoError(t, err)
	require.Equal(t, authoritativeNodeID, preflight.NodeID)
	require.Equal(t, runtimeNodeID, preflight.RuntimeNodeID)
	require.Equal(t, executionInstanceID, preflight.ExecutionInstanceID)
	require.Equal(t, "codex", preflight.ProviderType)
	require.Equal(t, "isolated", preflight.WorkspacePolicy["workspace"])
	require.True(t, preflight.ProviderHealthy)
}

func TestRunPreflightUsesAsiaShanghaiDailyTokenUsage(t *testing.T) {
	ctx := context.Background()
	cfg, ok := employeeRunRepositoryTestConfig()
	if !ok {
		t.Skip("set TEST_DATABASE_URL and TEST_REDIS_URL, or set ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 with DATABASE_URL and REDIS_URL")
	}
	require.NoError(t, pingEmployeeRunRepositoryTestRedis(ctx, cfg.redisURL))

	conn, err := pgx.Connect(ctx, cfg.databaseURL)
	require.NoError(t, err)
	defer conn.Close(ctx)

	schemaName := "employee_run_repo_" + strings.ReplaceAll(strings.ToLower(uuid.NewString()), "-", "_")
	_, err = conn.Exec(ctx, `CREATE SCHEMA `+schemaName)
	require.NoError(t, err)
	defer conn.Exec(ctx, `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)

	_, err = conn.Exec(ctx, `SET search_path TO `+schemaName)
	require.NoError(t, err)
	require.NoError(t, runEmployeeRepositoryTestMigrations(ctx, conn))

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	runtimeNodeID := uuid.New()
	employeeID := uuid.New()
	executionInstanceID := uuid.New()
	employeeConfigRevisionID := uuid.New()
	taskBeforeID := uuid.New()
	taskInsideID := uuid.New()
	runBeforeID := uuid.New()
	runInsideID := uuid.New()
	nodeID := "runtime-budget-boundary"

	beforeBusinessDay := time.Date(2026, 6, 6, 15, 59, 0, 0, time.UTC)
	insideBusinessDay := time.Date(2026, 6, 6, 16, 1, 0, 0, time.UTC)
	referenceBusinessMidnight := time.Date(2026, 6, 6, 16, 0, 0, 0, time.UTC)
	var currentBusinessMidnight time.Time
	err = conn.QueryRow(ctx, `SELECT date_trunc('day', timezone('Asia/Shanghai', now())) AT TIME ZONE 'Asia/Shanghai'`).Scan(&currentBusinessMidnight)
	require.NoError(t, err)
	dayShift := int(currentBusinessMidnight.Sub(referenceBusinessMidnight) / (24 * time.Hour))
	beforeToday := beforeBusinessDay.AddDate(0, 0, dayShift)
	insideToday := insideBusinessDay.AddDate(0, 0, dayShift)

	_, err = conn.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, status)
		VALUES ($1, 'default', '默认租户', 'active');

		INSERT INTO tenant_teams (id, tenant_id, slug, name, status)
		VALUES ($2, $1, 'default', '默认团队', 'active');

		INSERT INTO runtime_nodes (
			id,
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
			$3,
			$1,
			$4,
			'Runtime Budget Boundary',
			'["codex"]'::jsonb,
			2,
			0,
			'online',
			'{}'::jsonb,
			NOW()
		);

		INSERT INTO runtime_capabilities (
			tenant_id,
			runtime_node_id,
			capability_type,
			capability_key,
			provider_type,
			provider_version,
			binary_path,
			available,
			workspace_base_dir,
			capacity,
			labels,
			status,
			details,
			health_status,
			metadata,
			last_seen_at
		) VALUES (
			$1,
			$3,
			'provider',
			'provider:codex',
			'codex',
			'1.0.0',
			'/usr/local/bin/codex',
			true,
			'/tmp/superteam',
			'{}'::jsonb,
			'{}'::jsonb,
			'healthy',
			'{}'::jsonb,
			'healthy',
			'{}'::jsonb,
			NOW()
		);

		INSERT INTO digital_employees (
			id,
			tenant_id,
			team_id,
			name,
			role,
			status,
			permission_policy,
			context_policy,
			approval_policy,
			risk_level,
			metadata
		) VALUES (
			$5,
			$1,
			$2,
			'预算验证员工',
			'operator',
			'ready',
			'{}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			'normal',
			'{}'::jsonb
		);

		INSERT INTO digital_employee_execution_instances (
			id,
			tenant_id,
			digital_employee_id,
			runtime_node_id,
			provider_type,
			agent_home_dir,
			workspace_policy,
			session_policy,
			runtime_selector,
			capacity_requirements,
			fallback_policy,
			status,
			ready_at,
			metadata
		) VALUES (
			$6,
			$1,
			$5,
			$3,
			'codex',
			'/var/lib/superteam/agents/employee',
			'{"workspace":"isolated"}'::jsonb,
			'{"resume":true}'::jsonb,
			'{"node_id":"runtime-budget-boundary"}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			'ready',
			NOW(),
			'{}'::jsonb
		);

		INSERT INTO digital_employee_config_revisions (
			id,
			tenant_id,
			digital_employee_id,
			revision_number,
			persona_memory_markdown,
			capability_bindings,
			budget_policy,
			status
		) VALUES (
			$7,
			$1,
			$5,
			1,
			'# 人格画像
证据优先',
			'{"skills":["incident-diagnosis"],"mcp_servers":["postgres-readonly"],"external_capabilities":[],"environment_variable_refs":["PG_DSN"]}'::jsonb,
			'{}'::jsonb,
			'{"daily_token_limit":1000}'::jsonb,
			'active'
		);

		INSERT INTO tasks (
			id,
			tenant_id,
			team_id,
			title,
			status,
			provider_type,
			target_node_id,
			params,
			created_at,
			updated_at
		) VALUES
			($8, $1, $2, '午夜前运行', 'completed', 'codex', $4, '{}'::jsonb, $12, $12),
			($9, $1, $2, '午夜后运行', 'completed', 'codex', $4, '{}'::jsonb, $13, $13);

		INSERT INTO task_runs (
			id,
			tenant_id,
			task_id,
			node_id,
			runtime_node_id,
			status,
			started_at,
			completed_at,
			finished_at,
			result,
			created_at,
			updated_at,
			command_id,
			digital_employee_id,
			execution_instance_id,
			provider_type
		) VALUES
			($10, $1, $8, $4, $3, 'completed', $12, $12, $12, '{"usage":{"total_tokens":700}}'::jsonb, $12, $12, 'cmd-before-midnight', $5, $6, 'codex'),
			($11, $1, $9, $4, $3, 'completed', $13, $13, $13, '{"usage":{"total_tokens":300}}'::jsonb, $13, $13, 'cmd-after-midnight', $5, $6, 'codex');
	`, tenantID, teamID, runtimeNodeID, nodeID, employeeID, executionInstanceID, employeeConfigRevisionID, taskBeforeID, taskInsideID, runBeforeID, runInsideID, beforeToday, insideToday)
	require.NoError(t, err)

	repo := NewPgRunRepository(queries.New(conn))
	preflight, err := repo.GetRunPreflight(ctx, tenantID, employeeID)

	require.NoError(t, err)
	require.Equal(t, int32(300), preflight.TodayTokenUsage)
	require.Equal(t, "Asia/Shanghai", preflight.BusinessTimezone)
	require.Equal(t, float64(1000), preflight.BudgetPolicy["daily_token_limit"])
}

func validCreateRunRecordRequest(idempotencyKey string) CreateRunRecordRequest {
	return CreateRunRecordRequest{
		IdempotencyKey:      &idempotencyKey,
		TenantID:            uuid.New(),
		DigitalEmployeeID:   uuid.New(),
		TeamID:              uuid.New(),
		Title:               "修复一个测试失败",
		Priority:            1,
		ProviderType:        "codex",
		TargetNodeID:        "runtime-a",
		Params:              map[string]any{"objective": "修复一个测试失败"},
		NodeID:              "runtime-a",
		RuntimeNodeID:       uuid.New(),
		RunStatus:           DigitalEmployeeRunStatusDispatching,
		CommandID:           "cmd-1",
		ExecutionInstanceID: uuid.New(),
	}
}

type fakeRunRepositoryDBTX struct {
	rowErr error
}

func (f fakeRunRepositoryDBTX) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, fmt.Errorf("unexpected Exec")
}

func (f fakeRunRepositoryDBTX) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, fmt.Errorf("unexpected Query")
}

func (f fakeRunRepositoryDBTX) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	return fakeRunRepositoryRow{err: f.rowErr}
}

type fakeRunRepositoryRow struct {
	err error
}

func (r fakeRunRepositoryRow) Scan(...interface{}) error {
	return r.err
}

type employeeRunRepositoryIntegrationConfig struct {
	databaseURL string
	redisURL    string
}

func employeeRunRepositoryTestConfig() (employeeRunRepositoryIntegrationConfig, bool) {
	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	redisURL := strings.TrimSpace(os.Getenv("TEST_REDIS_URL"))
	if employeeRunRepositoryEnvBool("ALLOW_DATABASE_URL_FOR_QUERY_TESTS") {
		if databaseURL == "" {
			databaseURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
		}
		if redisURL == "" {
			redisURL = strings.TrimSpace(os.Getenv("REDIS_URL"))
		}
	}
	if databaseURL == "" || redisURL == "" {
		return employeeRunRepositoryIntegrationConfig{}, false
	}
	return employeeRunRepositoryIntegrationConfig{
		databaseURL: databaseURL,
		redisURL:    redisURL,
	}, true
}

func employeeRunRepositoryEnvBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func pingEmployeeRunRepositoryTestRedis(ctx context.Context, redisURL string) error {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return err
	}
	client := redis.NewClient(options)
	defer client.Close()
	return client.Ping(ctx).Err()
}

func runEmployeeRepositoryTestMigrations(ctx context.Context, conn *pgx.Conn) error {
	files, err := filepath.Glob(filepath.Join("..", "storage", "migrations", "*.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read %s: %w", file, err)
		}
		if _, err := conn.Exec(ctx, string(content)); err != nil {
			return fmt.Errorf("apply %s: %w", filepath.Base(file), err)
		}
	}
	return nil
}

func TestPgRepositoryGetDigitalEmployeeRunStatsAggregatesByStatus(t *testing.T) {
	ctx := context.Background()
	cfg, ok := employeeRunRepositoryTestConfig()
	if !ok {
		t.Skip("set TEST_DATABASE_URL and TEST_REDIS_URL, or set ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 with DATABASE_URL and REDIS_URL")
	}

	conn, err := pgx.Connect(ctx, cfg.databaseURL)
	require.NoError(t, err)
	defer conn.Close(ctx)

	schemaName := "employee_run_stats_" + strings.ReplaceAll(strings.ToLower(uuid.NewString()), "-", "_")
	_, err = conn.Exec(ctx, `CREATE SCHEMA `+schemaName)
	require.NoError(t, err)
	defer conn.Exec(ctx, `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)
	_, err = conn.Exec(ctx, `SET search_path TO `+schemaName)
	require.NoError(t, err)
	require.NoError(t, runEmployeeRepositoryTestMigrations(ctx, conn))

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	employeeID := uuid.New()
	otherEmployeeID := uuid.New()
	taskID := uuid.New()

	_, err = conn.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, status)
		VALUES ($1, 'default', '默认租户', 'active')
		ON CONFLICT (id) DO NOTHING;
	`, tenantID)
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `
		INSERT INTO tasks (id, tenant_id, title, provider_type, status)
		VALUES ($1, $2, 'stats-fixture-task', 'codex', 'completed');
	`, taskID, tenantID)
	require.NoError(t, err)

	insertRun := func(employee uuid.UUID, status string, startedAt, finishedAt *time.Time, createdAt time.Time) {
		_, err := conn.Exec(ctx, `
			INSERT INTO task_runs (id, tenant_id, task_id, digital_employee_id, node_id, status, started_at, finished_at, created_at)
			VALUES ($1, $2, $3, $4, 'node-a', $5, $6, $7, $8)
		`, uuid.New(), tenantID, taskID, employee, status, startedAt, finishedAt, createdAt)
		require.NoError(t, err)
	}

	now := time.Now().UTC()
	start1, finish1 := now.Add(-30*time.Minute), now.Add(-10*time.Minute)
	start2, finish2 := now.Add(-2*time.Hour), now.Add(-1*time.Hour)
	insertRun(employeeID, "completed", &start1, &finish1, now.Add(-1*24*time.Hour))
	insertRun(employeeID, "completed", &start2, &finish2, now.Add(-10*24*time.Hour))
	insertRun(employeeID, "failed", &start1, &finish1, now.Add(-2*24*time.Hour))
	insertRun(employeeID, "cancelled", &start1, &finish1, now.Add(-12*24*time.Hour))
	insertRun(otherEmployeeID, "completed", &start1, &finish1, now)

	repo := NewPgRepository(queries.New(conn))
	stats, err := repo.GetDigitalEmployeeRunStats(ctx, tenantID, employeeID)
	require.NoError(t, err)

	require.Equal(t, int64(4), stats.TotalCount)
	require.Equal(t, int64(2), stats.SucceededCount)
	require.Equal(t, int64(1), stats.FailedCount)
	require.Equal(t, int64(1), stats.CancelledCount)
	require.Equal(t, int64(2), stats.Last7dCount)
	// prev_7d window is [now-14d, now-7d): both the -10d and -12d fixture rows fall inside it.
	require.Equal(t, int64(2), stats.Prev7dCount)
	require.NotNil(t, stats.AvgDurationSec)
	// durations across all 4 finished runs for employeeID: 1200s, 3600s, 1200s, 1200s -> avg 1800s.
	require.InDelta(t, 1800, *stats.AvgDurationSec, 1)
	require.NotNil(t, stats.P90DurationSec)
	// PERCENTILE_CONT(0.9) over sorted [1200, 1200, 1200, 3600] interpolates to 2880s.
	require.InDelta(t, 2880, *stats.P90DurationSec, 1)
}

func TestPgRepositoryGetDigitalEmployeeRunStatsNullDurationsForNoFinishedRuns(t *testing.T) {
	ctx := context.Background()
	cfg, ok := employeeRunRepositoryTestConfig()
	if !ok {
		t.Skip("set TEST_DATABASE_URL and TEST_REDIS_URL, or set ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 with DATABASE_URL and REDIS_URL")
	}

	conn, err := pgx.Connect(ctx, cfg.databaseURL)
	require.NoError(t, err)
	defer conn.Close(ctx)

	schemaName := "employee_run_stats_null_" + strings.ReplaceAll(strings.ToLower(uuid.NewString()), "-", "_")
	_, err = conn.Exec(ctx, `CREATE SCHEMA `+schemaName)
	require.NoError(t, err)
	defer conn.Exec(ctx, `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)
	_, err = conn.Exec(ctx, `SET search_path TO `+schemaName)
	require.NoError(t, err)
	require.NoError(t, runEmployeeRepositoryTestMigrations(ctx, conn))

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	employeeWithNoRuns := uuid.New()
	employeeWithUnfinishedRuns := uuid.New()
	taskID := uuid.New()

	_, err = conn.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, status)
		VALUES ($1, 'default', '默认租户', 'active')
		ON CONFLICT (id) DO NOTHING;
	`, tenantID)
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `
		INSERT INTO tasks (id, tenant_id, title, provider_type, status)
		VALUES ($1, $2, 'stats-fixture-task-null', 'codex', 'running');
	`, taskID, tenantID)
	require.NoError(t, err)

	insertRun := func(employee uuid.UUID, status string, startedAt time.Time, finishedAt *time.Time, createdAt time.Time) {
		_, err := conn.Exec(ctx, `
			INSERT INTO task_runs (id, tenant_id, task_id, digital_employee_id, node_id, status, started_at, finished_at, created_at)
			VALUES ($1, $2, $3, $4, 'node-a', $5, $6, $7, $8)
		`, uuid.New(), tenantID, taskID, employee, status, startedAt, finishedAt, createdAt)
		require.NoError(t, err)
	}

	now := time.Now().UTC()
	// employeeWithUnfinishedRuns has runs, but none of them have finished_at set (still in flight).
	insertRun(employeeWithUnfinishedRuns, "running", now.Add(-10*time.Minute), nil, now.Add(-10*time.Minute))
	insertRun(employeeWithUnfinishedRuns, "dispatching", now.Add(-5*time.Minute), nil, now.Add(-5*time.Minute))

	repo := NewPgRepository(queries.New(conn))

	t.Run("employee with zero runs at all", func(t *testing.T) {
		stats, err := repo.GetDigitalEmployeeRunStats(ctx, tenantID, employeeWithNoRuns)
		require.NoError(t, err)
		require.Equal(t, int64(0), stats.TotalCount)
		require.Equal(t, int64(0), stats.SucceededCount)
		require.Equal(t, int64(0), stats.FailedCount)
		require.Equal(t, int64(0), stats.CancelledCount)
		require.Equal(t, int64(0), stats.Last7dCount)
		require.Equal(t, int64(0), stats.Prev7dCount)
		require.Nil(t, stats.AvgDurationSec)
		require.Nil(t, stats.P90DurationSec)
	})

	t.Run("employee with runs but none finished", func(t *testing.T) {
		stats, err := repo.GetDigitalEmployeeRunStats(ctx, tenantID, employeeWithUnfinishedRuns)
		require.NoError(t, err)
		require.Equal(t, int64(2), stats.TotalCount)
		require.Equal(t, int64(0), stats.SucceededCount)
		require.Equal(t, int64(0), stats.FailedCount)
		require.Equal(t, int64(0), stats.CancelledCount)
		require.Nil(t, stats.AvgDurationSec)
		require.Nil(t, stats.P90DurationSec)
	})
}

// TestPgRepositoryListRunsDetailedFiltersByStatusAndProject exercises the joined,
// filtered, paginated run list: it verifies total_count matches the filtered (not
// unfiltered) set, status/project/time-window filters apply symmetrically to list and
// count, the task/project LEFT JOIN surfaces task_title and project_name, work-product
// count is read, duration_sec is computed for finished runs AND stays nil for
// in-progress runs (which have finished_at IS NULL — this guards against the pgx
// NULL-into-float64 scan crash that a naive SQL EXTRACT column would trigger).
func TestPgRepositoryListRunsDetailedFiltersByStatusAndProject(t *testing.T) {
	ctx := context.Background()
	cfg, ok := employeeRunRepositoryTestConfig()
	if !ok {
		t.Skip("set TEST_DATABASE_URL and TEST_REDIS_URL, or set ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 with DATABASE_URL and REDIS_URL")
	}
	conn, err := pgx.Connect(ctx, cfg.databaseURL)
	require.NoError(t, err)
	defer conn.Close(ctx)

	schemaName := "employee_run_list_" + strings.ReplaceAll(strings.ToLower(uuid.NewString()), "-", "_")
	_, err = conn.Exec(ctx, `CREATE SCHEMA `+schemaName)
	require.NoError(t, err)
	defer conn.Exec(ctx, `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)
	_, err = conn.Exec(ctx, `SET search_path TO `+schemaName)
	require.NoError(t, err)
	require.NoError(t, runEmployeeRepositoryTestMigrations(ctx, conn))

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	employeeID := uuid.New()
	otherEmployeeID := uuid.New()
	taskID := uuid.New()
	projectID := uuid.New()
	completedRunID := uuid.New()
	failedRunID := uuid.New()
	inProgressRunID := uuid.New()
	humanOwnerID := uuid.New()
	nodeID := "node-a"

	// Fixture setup uses one Exec per statement: pgx's extended protocol rejects
	// multi-statement strings that also bind parameters.
	_, err = conn.Exec(ctx, `INSERT INTO tenants (id, slug, name, status) VALUES ($1, 'default', '默认租户', 'active') ON CONFLICT (id) DO NOTHING`, tenantID)
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `INSERT INTO tasks (id, tenant_id, title, provider_type, status) VALUES ($1, $2, '需求梳理任务', 'codex', 'completed')`, taskID, tenantID)
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `
		INSERT INTO task_runs (id, tenant_id, task_id, digital_employee_id, node_id, status, started_at, finished_at, work_products, created_at)
		VALUES ($1, $2, $3, $4, $5, 'completed', NOW() - interval '30 minutes', NOW() - interval '10 minutes', '[{"type":"report","title":"r"}]'::jsonb, NOW() - interval '3 hours')
	`, completedRunID, tenantID, taskID, employeeID, nodeID)
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `
		INSERT INTO task_runs (id, tenant_id, task_id, digital_employee_id, node_id, status, started_at, finished_at, created_at)
		VALUES ($1, $2, $3, $4, $5, 'failed', NOW() - interval '1 hour', NOW() - interval '50 minutes', NOW() - interval '2 hours')
	`, failedRunID, tenantID, taskID, employeeID, nodeID)
	require.NoError(t, err)
	// In-progress run with NO finished_at: proves duration computation is NULL-safe.
	_, err = conn.Exec(ctx, `
		INSERT INTO task_runs (id, tenant_id, task_id, digital_employee_id, node_id, status, started_at, created_at)
		VALUES ($1, $2, $3, $4, $5, 'running', NOW() - interval '5 minutes', NOW() - interval '30 minutes')
	`, inProgressRunID, tenantID, taskID, employeeID, nodeID)
	require.NoError(t, err)
	// Run for a different employee: must be excluded from every query below.
	_, err = conn.Exec(ctx, `
		INSERT INTO task_runs (id, tenant_id, task_id, digital_employee_id, node_id, status, started_at, finished_at, created_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, 'completed', NOW() - interval '1 hour', NOW() - interval '40 minutes', NOW())
	`, tenantID, taskID, otherEmployeeID, nodeID)
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `INSERT INTO projects (id, tenant_id, name, status, human_owner_user_id) VALUES ($1, $2, '试点项目 A', 'active', $3)`, projectID, tenantID, humanOwnerID)
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `INSERT INTO project_tasks (id, tenant_id, project_id, digital_employee_run_id, title, status) VALUES (gen_random_uuid(), $1, $2, $3, '需求梳理任务', 'completed')`, tenantID, projectID, completedRunID)
	require.NoError(t, err)

	repo := NewPgRepository(queries.New(conn))

	all, err := repo.ListRunsDetailed(ctx, tenantID, employeeID, DigitalEmployeeRunListFilter{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, int64(3), all.TotalCount)
	require.Len(t, all.Projects, 1)
	require.Equal(t, "试点项目 A", all.Projects[0].Name)
	require.Equal(t, projectID, all.Projects[0].ID)
	// Ordering is created_at DESC: completed(-3h) was inserted with created_at NOW()-3h...
	// actually each row sets its own created_at. Verify the in-progress run (most recent,
	// created NOW()-30min) is first and has nil DurationSec.
	require.Len(t, all.Items, 3)
	require.Nil(t, all.Items[0].DurationSec, "in-progress run must have nil duration_sec, not crash")

	onlyCompleted, err := repo.ListRunsDetailed(ctx, tenantID, employeeID, DigitalEmployeeRunListFilter{
		Statuses: []string{"completed"},
		Limit:    10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), onlyCompleted.TotalCount)
	require.Len(t, onlyCompleted.Items, 1)
	require.Equal(t, "需求梳理任务", onlyCompleted.Items[0].TaskTitle)
	require.NotNil(t, onlyCompleted.Items[0].ProjectName)
	require.Equal(t, "试点项目 A", *onlyCompleted.Items[0].ProjectName)
	require.NotNil(t, onlyCompleted.Items[0].ProjectID)
	require.Equal(t, projectID, *onlyCompleted.Items[0].ProjectID)
	require.Equal(t, int32(1), onlyCompleted.Items[0].WorkProductCount)
	require.NotNil(t, onlyCompleted.Items[0].DurationSec)
	require.InDelta(t, 1200, *onlyCompleted.Items[0].DurationSec, 1)

	scopedToProject, err := repo.ListRunsDetailed(ctx, tenantID, employeeID, DigitalEmployeeRunListFilter{
		ProjectID: &projectID,
		Limit:     10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), scopedToProject.TotalCount)
	require.Equal(t, completedRunID, scopedToProject.Items[0].Run.ID)

	// Time window: only the in-progress run (created NOW()-30min) falls inside the last hour.
	from := time.Now().UTC().Add(-1 * time.Hour)
	to := time.Now().UTC().Add(1 * time.Minute)
	recent, err := repo.ListRunsDetailed(ctx, tenantID, employeeID, DigitalEmployeeRunListFilter{
		From:  &from,
		To:    &to,
		Limit: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), recent.TotalCount)
	require.Equal(t, inProgressRunID, recent.Items[0].Run.ID)
}

// TestPgRunRepositoryFindProviderSessionForTaskRoot exercises the task-lineage
// scoped session lookup added for session resumption across task revisions:
// a session hits when (employee, task lineage root) matches a recoverable
// active/idle/completed row, but a different root — even for the same
// employee — must miss. Completed sessions must be resumable because the
// real upstream-supplement flow dispatches the revision/supplement task
// after the upstream task (and its session) has already completed.
func TestPgRunRepositoryFindProviderSessionForTaskRoot(t *testing.T) {
	ctx := context.Background()
	cfg, ok := employeeRunRepositoryTestConfig()
	if !ok {
		t.Skip("set TEST_DATABASE_URL and TEST_REDIS_URL, or set ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 with DATABASE_URL and REDIS_URL")
	}

	conn, err := pgx.Connect(ctx, cfg.databaseURL)
	require.NoError(t, err)
	defer conn.Close(ctx)

	schemaName := "provider_session_task_root_" + strings.ReplaceAll(strings.ToLower(uuid.NewString()), "-", "_")
	_, err = conn.Exec(ctx, `CREATE SCHEMA `+schemaName)
	require.NoError(t, err)
	defer conn.Exec(ctx, `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)
	_, err = conn.Exec(ctx, `SET search_path TO `+schemaName)
	require.NoError(t, err)
	require.NoError(t, runEmployeeRepositoryTestMigrations(ctx, conn))

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	employeeID := uuid.New()
	root := uuid.New()
	otherRoot := uuid.New()

	_, err = conn.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, status)
		VALUES ($1, 'default', '默认租户', 'active')
		ON CONFLICT (id) DO NOTHING;
	`, tenantID)
	require.NoError(t, err)

	insertSession := func(providerSessionID string, forEmployee, taskRoot uuid.UUID, status string, recoverable bool, lastActiveAt time.Time) {
		_, err := conn.Exec(ctx, `
			INSERT INTO provider_sessions (
				tenant_id, provider_session_id, digital_employee_id,
				execution_instance_id, runtime_node_id, provider_type,
				status, recoverable, last_active_at, project_task_root_id
			) VALUES ($1, $2, $3, $4, $5, 'codex', $6, $7, $8, $9)
		`, tenantID, providerSessionID, forEmployee, uuid.New(), uuid.New(), status, recoverable, lastActiveAt, taskRoot)
		require.NoError(t, err)
	}

	now := time.Now().UTC()
	insertSession("sess-root-active", employeeID, root, "active", true, now.Add(-5*time.Minute))
	// The latest recoverable row for the root — completed but more recent
	// than the active row — must win the "latest by last_active_at" ordering.
	insertSession("sess-root-completed", employeeID, root, "completed", true, now)
	insertSession("sess-other-root-active", employeeID, otherRoot, "active", true, now.Add(-1*time.Minute))

	repo := NewPgRunRepository(queries.New(conn))

	t.Run("same employee and root prefers the latest recoverable session, including completed", func(t *testing.T) {
		found, err := repo.FindProviderSessionForTaskRoot(ctx, tenantID, employeeID, root)
		require.NoError(t, err)
		require.Equal(t, "sess-root-completed", found)
	})

	t.Run("a completed session alone for a root must be returned", func(t *testing.T) {
		completedOnlyRoot := uuid.New()
		insertSession("sess-completed-only", employeeID, completedOnlyRoot, "completed", true, now)

		found, err := repo.FindProviderSessionForTaskRoot(ctx, tenantID, employeeID, completedOnlyRoot)
		require.NoError(t, err)
		require.Equal(t, "sess-completed-only", found)
	})

	t.Run("failed session for a root misses", func(t *testing.T) {
		failedRoot := uuid.New()
		insertSession("sess-failed-only", employeeID, failedRoot, "failed", true, now)

		found, err := repo.FindProviderSessionForTaskRoot(ctx, tenantID, employeeID, failedRoot)
		require.NoError(t, err)
		require.Empty(t, found)
	})

	t.Run("stopped session for a root misses", func(t *testing.T) {
		stoppedRoot := uuid.New()
		insertSession("sess-stopped-only", employeeID, stoppedRoot, "stopped", true, now)

		found, err := repo.FindProviderSessionForTaskRoot(ctx, tenantID, employeeID, stoppedRoot)
		require.NoError(t, err)
		require.Empty(t, found)
	})

	t.Run("non-recoverable completed session for a root misses", func(t *testing.T) {
		nonRecoverableRoot := uuid.New()
		insertSession("sess-non-recoverable", employeeID, nonRecoverableRoot, "completed", false, now)

		found, err := repo.FindProviderSessionForTaskRoot(ctx, tenantID, employeeID, nonRecoverableRoot)
		require.NoError(t, err)
		require.Empty(t, found)
	})

	t.Run("different root for the same employee misses", func(t *testing.T) {
		found, err := repo.FindProviderSessionForTaskRoot(ctx, tenantID, employeeID, uuid.New())
		require.NoError(t, err)
		require.Empty(t, found)
	})

	t.Run("unknown employee misses even for a known root", func(t *testing.T) {
		found, err := repo.FindProviderSessionForTaskRoot(ctx, tenantID, uuid.New(), root)
		require.NoError(t, err)
		require.Empty(t, found)
	})
}

// TestPgRunRepositoryResolveProjectTaskLineageRoot exercises the
// planner_metadata > revision_of_task_id > self precedence that mirrors
// projectcoordination's revisionRootTaskID (project_store.go) without this
// package importing that one.
func TestPgRunRepositoryResolveProjectTaskLineageRoot(t *testing.T) {
	ctx := context.Background()
	cfg, ok := employeeRunRepositoryTestConfig()
	if !ok {
		t.Skip("set TEST_DATABASE_URL and TEST_REDIS_URL, or set ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 with DATABASE_URL and REDIS_URL")
	}

	conn, err := pgx.Connect(ctx, cfg.databaseURL)
	require.NoError(t, err)
	defer conn.Close(ctx)

	schemaName := "project_task_lineage_root_" + strings.ReplaceAll(strings.ToLower(uuid.NewString()), "-", "_")
	_, err = conn.Exec(ctx, `CREATE SCHEMA `+schemaName)
	require.NoError(t, err)
	defer conn.Exec(ctx, `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)
	_, err = conn.Exec(ctx, `SET search_path TO `+schemaName)
	require.NoError(t, err)
	require.NoError(t, runEmployeeRepositoryTestMigrations(ctx, conn))

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	_, err = conn.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, status)
		VALUES ($1, 'default', '默认租户', 'active')
		ON CONFLICT (id) DO NOTHING;
	`, tenantID)
	require.NoError(t, err)

	projectID := uuid.New()

	insertTask := func(id uuid.UUID, revisionOfTaskID *uuid.UUID, plannerMetadata string) {
		_, err := conn.Exec(ctx, `
			INSERT INTO project_tasks (
				id, tenant_id, project_id, title, status, revision_of_task_id, planner_metadata
			) VALUES ($1, $2, $3, 'task', 'pending', $4, $5::jsonb)
		`, id, tenantID, projectID, revisionOfTaskID, plannerMetadata)
		require.NoError(t, err)
	}

	rootTaskID := uuid.New()
	insertTask(rootTaskID, nil, `{}`)

	// One-hop revision: no planner_metadata root override, falls back to
	// revision_of_task_id.
	revisionTaskID := uuid.New()
	insertTask(revisionTaskID, &rootTaskID, `{}`)

	// planner_metadata root override wins even when revision_of_task_id is
	// also set (keeps multi-hop revisions pointing at the original root).
	overriddenTaskID := uuid.New()
	overrideRoot := uuid.New()
	insertTask(overriddenTaskID, &revisionTaskID, fmt.Sprintf(`{"revision_root_task_id": %q}`, overrideRoot.String()))

	// No revision lineage at all: root is the task's own id.
	freshTaskID := uuid.New()
	insertTask(freshTaskID, nil, `{}`)

	repo, ok := NewPgRunRepository(queries.New(conn)).(ProjectTaskRunPreflightRepository)
	require.True(t, ok, "PgRunRepository must implement ProjectTaskRunPreflightRepository")

	t.Run("task with no lineage resolves to itself", func(t *testing.T) {
		got, err := repo.ResolveProjectTaskLineageRoot(ctx, tenantID, freshTaskID)
		require.NoError(t, err)
		require.Equal(t, freshTaskID, got)
	})

	t.Run("one-hop revision falls back to revision_of_task_id", func(t *testing.T) {
		got, err := repo.ResolveProjectTaskLineageRoot(ctx, tenantID, revisionTaskID)
		require.NoError(t, err)
		require.Equal(t, rootTaskID, got)
	})

	t.Run("planner_metadata root override takes precedence", func(t *testing.T) {
		got, err := repo.ResolveProjectTaskLineageRoot(ctx, tenantID, overriddenTaskID)
		require.NoError(t, err)
		require.Equal(t, overrideRoot, got)
	})

	t.Run("unknown task id is reported as not found", func(t *testing.T) {
		_, err := repo.ResolveProjectTaskLineageRoot(ctx, tenantID, uuid.New())
		require.ErrorIs(t, err, ErrNotFound)
	})
}

// TestPgRunRepositoryUpsertProviderSessionByExternalIDPersistsProjectTaskRootID
// exercises the writeback-side counterpart to task-lineage session scoping:
// a fresh session upsert must persist project_task_root_id when supplied, a
// session upsert without a root must leave the column null (pre-refactor
// compatibility), and FindProviderSessionForTaskRoot must hit the
// just-written-back root.
func TestPgRunRepositoryUpsertProviderSessionByExternalIDPersistsProjectTaskRootID(t *testing.T) {
	ctx := context.Background()
	cfg, ok := employeeRunRepositoryTestConfig()
	if !ok {
		t.Skip("set TEST_DATABASE_URL and TEST_REDIS_URL, or set ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 with DATABASE_URL and REDIS_URL")
	}

	conn, err := pgx.Connect(ctx, cfg.databaseURL)
	require.NoError(t, err)
	defer conn.Close(ctx)

	schemaName := "provider_session_upsert_root_" + strings.ReplaceAll(strings.ToLower(uuid.NewString()), "-", "_")
	_, err = conn.Exec(ctx, `CREATE SCHEMA `+schemaName)
	require.NoError(t, err)
	defer conn.Exec(ctx, `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)
	_, err = conn.Exec(ctx, `SET search_path TO `+schemaName)
	require.NoError(t, err)
	require.NoError(t, runEmployeeRepositoryTestMigrations(ctx, conn))

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	_, err = conn.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, status)
		VALUES ($1, 'default', '默认租户', 'active')
		ON CONFLICT (id) DO NOTHING;
	`, tenantID)
	require.NoError(t, err)

	repo := NewPgRunRepository(queries.New(conn))
	employeeID := uuid.New()
	executionInstanceID := uuid.New()
	runtimeNodeID := uuid.New()
	rootTaskID := uuid.New()

	baseReq := func(providerSessionID string, sequenceNumber int32, rootID *uuid.UUID) UpsertProviderSessionRequest {
		return UpsertProviderSessionRequest{
			TenantID:            tenantID,
			ProviderSessionID:   providerSessionID,
			DigitalEmployeeID:   employeeID,
			ExecutionInstanceID: executionInstanceID,
			RuntimeNodeID:       runtimeNodeID,
			ProviderType:        "codex",
			Status:              "active",
			Recoverable:         true,
			LastSequenceNumber:  sequenceNumber,
			ProjectTaskRootID:   rootID,
		}
	}

	t.Run("upsert with a root persists it and FindProviderSessionForTaskRoot hits it", func(t *testing.T) {
		_, err := repo.UpsertProviderSession(ctx, baseReq("sess-with-root", 1, &rootTaskID))
		require.NoError(t, err)

		found, err := repo.FindProviderSessionForTaskRoot(ctx, tenantID, employeeID, rootTaskID)
		require.NoError(t, err)
		require.Equal(t, "sess-with-root", found)
	})

	t.Run("upsert without a root leaves project_task_root_id null", func(t *testing.T) {
		_, err := repo.UpsertProviderSession(ctx, baseReq("sess-without-root", 1, nil))
		require.NoError(t, err)

		var rootID *uuid.UUID
		err = conn.QueryRow(ctx, `
			SELECT project_task_root_id FROM provider_sessions
			WHERE tenant_id = $1 AND provider_type = 'codex' AND provider_session_id = 'sess-without-root'
		`, tenantID).Scan(&rootID)
		require.NoError(t, err)
		require.Nil(t, rootID)

		// A later event for the same session still without a root must not
		// error and must keep the column null.
		_, err = repo.UpsertProviderSession(ctx, baseReq("sess-without-root", 2, nil))
		require.NoError(t, err)
		err = conn.QueryRow(ctx, `
			SELECT project_task_root_id FROM provider_sessions
			WHERE tenant_id = $1 AND provider_type = 'codex' AND provider_session_id = 'sess-without-root'
		`, tenantID).Scan(&rootID)
		require.NoError(t, err)
		require.Nil(t, rootID)
	})

	t.Run("a later event with a root backfills a previously-null root", func(t *testing.T) {
		_, err := repo.UpsertProviderSession(ctx, baseReq("sess-backfill-root", 1, nil))
		require.NoError(t, err)

		_, err = repo.UpsertProviderSession(ctx, baseReq("sess-backfill-root", 2, &rootTaskID))
		require.NoError(t, err)

		var rootID uuid.UUID
		err = conn.QueryRow(ctx, `
			SELECT project_task_root_id FROM provider_sessions
			WHERE tenant_id = $1 AND provider_type = 'codex' AND provider_session_id = 'sess-backfill-root'
		`, tenantID).Scan(&rootID)
		require.NoError(t, err)
		require.Equal(t, rootTaskID, rootID)
	})
}

// TestPgRunRepositoryGetRunTaskMetadata exercises the fallback metadata
// lookup writeback uses when the runtime event itself doesn't carry
// revision_root_task_id: the metadata the control plane stamped on the task
// at dispatch time (tasks.params["metadata"]) must be readable back by task
// id, and a task with no metadata must report an empty map, not an error.
func TestPgRunRepositoryGetRunTaskMetadata(t *testing.T) {
	ctx := context.Background()
	cfg, ok := employeeRunRepositoryTestConfig()
	if !ok {
		t.Skip("set TEST_DATABASE_URL and TEST_REDIS_URL, or set ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 with DATABASE_URL and REDIS_URL")
	}

	conn, err := pgx.Connect(ctx, cfg.databaseURL)
	require.NoError(t, err)
	defer conn.Close(ctx)

	schemaName := "run_task_metadata_" + strings.ReplaceAll(strings.ToLower(uuid.NewString()), "-", "_")
	_, err = conn.Exec(ctx, `CREATE SCHEMA `+schemaName)
	require.NoError(t, err)
	defer conn.Exec(ctx, `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)
	_, err = conn.Exec(ctx, `SET search_path TO `+schemaName)
	require.NoError(t, err)
	require.NoError(t, runEmployeeRepositoryTestMigrations(ctx, conn))

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	_, err = conn.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, status)
		VALUES ($1, 'default', '默认租户', 'active')
		ON CONFLICT (id) DO NOTHING;
	`, tenantID)
	require.NoError(t, err)

	rootTaskID := uuid.New()
	taskWithMetadataID := uuid.New()
	_, err = conn.Exec(ctx, `
		INSERT INTO tasks (id, tenant_id, title, status, provider_type, params)
		VALUES ($1, $2, 'task', 'queued', 'codex', $3::jsonb)
	`, taskWithMetadataID, tenantID, fmt.Sprintf(`{"metadata": {"revision_root_task_id": %q}}`, rootTaskID.String()))
	require.NoError(t, err)

	taskWithoutMetadataID := uuid.New()
	_, err = conn.Exec(ctx, `
		INSERT INTO tasks (id, tenant_id, title, status, provider_type, params)
		VALUES ($1, $2, 'task', 'queued', 'codex', '{}'::jsonb)
	`, taskWithoutMetadataID, tenantID)
	require.NoError(t, err)

	repo := NewPgRunRepository(queries.New(conn))

	t.Run("returns the persisted metadata map", func(t *testing.T) {
		metadata, err := repo.GetRunTaskMetadata(ctx, tenantID, taskWithMetadataID)
		require.NoError(t, err)
		require.Equal(t, rootTaskID.String(), metadata["revision_root_task_id"])
	})

	t.Run("a task with no metadata reports an empty map, not an error", func(t *testing.T) {
		metadata, err := repo.GetRunTaskMetadata(ctx, tenantID, taskWithoutMetadataID)
		require.NoError(t, err)
		require.Empty(t, metadata)
	})

	t.Run("unknown task id is reported as not found", func(t *testing.T) {
		_, err := repo.GetRunTaskMetadata(ctx, tenantID, uuid.New())
		require.ErrorIs(t, err, ErrNotFound)
	})
}
