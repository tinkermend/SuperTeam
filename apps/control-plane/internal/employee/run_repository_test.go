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

func TestRunPreflightFromQueryRejectsMissingTeam(t *testing.T) {
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

	require.ErrorIs(t, err, ErrInvalidInput)
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

func TestPgRunRepositoryCreateRunRejectsMissingTeam(t *testing.T) {
	repo := NewPgRunRepository(nil)

	_, err := repo.CreateRun(context.Background(), CreateRunRecordRequest{})

	require.ErrorIs(t, err, ErrInvalidInput)
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
	teamConfigRevisionID := uuid.New()
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

		INSERT INTO tenant_team_config_revisions (
			id,
			tenant_id,
			team_id,
			revision_number,
			constitution,
			capability_policy,
			context_policy,
			approval_policy,
			artifact_contract,
			internal_collaboration_policy,
			runtime_scope_policy,
			status,
			approved_at
		) VALUES (
			$7,
			$1,
			$2,
			1,
			'{}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			'active',
			NOW()
		);

		INSERT INTO digital_employee_config_revisions (
			id,
			tenant_id,
			digital_employee_id,
			revision_number,
			role_profile,
			constitution_addendum,
			capability_selection,
			context_policy_override,
			approval_policy_override,
			output_contract_addendum,
			status
		) VALUES (
			$8,
			$1,
			$5,
			1,
			'{}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			'draft'
		);

		INSERT INTO digital_employee_effective_configs (
			tenant_id,
			digital_employee_id,
			tenant_team_config_revision_id,
			employee_config_revision_id,
			effective_config_snapshot,
			validation_result,
			status,
			approved_at
		) VALUES (
			$1,
			$5,
			$7,
			$8,
			'{}'::jsonb,
			'{}'::jsonb,
			'approved',
			NOW()
		);
	`, tenantID, teamID, runtimeNodeID, authoritativeNodeID, employeeID, executionInstanceID, teamConfigRevisionID, employeeConfigRevisionID)
	require.NoError(t, err)

	repo := NewPgRunRepository(queries.New(conn))
	preflight, err := repo.GetRunPreflight(ctx, tenantID, employeeID)

	require.NoError(t, err)
	require.Equal(t, authoritativeNodeID, preflight.NodeID)
	require.Equal(t, runtimeNodeID, preflight.RuntimeNodeID)
	require.Equal(t, executionInstanceID, preflight.ExecutionInstanceID)
	require.Equal(t, "codex", preflight.ProviderType)
	require.Equal(t, "isolated", preflight.WorkspacePolicy["workspace"])
	require.True(t, preflight.HasApprovedEffectiveConfig)
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
	teamConfigRevisionID := uuid.New()
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

		INSERT INTO tenant_team_config_revisions (
			id,
			tenant_id,
			team_id,
			revision_number,
			constitution,
			capability_policy,
			context_policy,
			approval_policy,
			artifact_contract,
			internal_collaboration_policy,
			runtime_scope_policy,
			status,
			approved_at
		) VALUES (
			$7,
			$1,
			$2,
			1,
			'{}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			'active',
			NOW()
		);

		INSERT INTO digital_employee_config_revisions (
			id,
			tenant_id,
			digital_employee_id,
			revision_number,
			role_profile,
			constitution_addendum,
			capability_selection,
			context_policy_override,
			approval_policy_override,
			output_contract_addendum,
			status
		) VALUES (
			$8,
			$1,
			$5,
			1,
			'{}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			'approved'
		);

		INSERT INTO digital_employee_effective_configs (
			tenant_id,
			digital_employee_id,
			tenant_team_config_revision_id,
			employee_config_revision_id,
			effective_config_snapshot,
			validation_result,
			status,
			approved_at
		) VALUES (
			$1,
			$5,
			$7,
			$8,
			'{"budget_policy":{"daily_token_limit":1000}}'::jsonb,
			'{}'::jsonb,
			'approved',
			NOW()
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
			($9, $1, $2, '午夜前运行', 'completed', 'codex', $4, '{}'::jsonb, $13, $13),
			($10, $1, $2, '午夜后运行', 'completed', 'codex', $4, '{}'::jsonb, $14, $14);

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
			($11, $1, $9, $4, $3, 'completed', $13, $13, $13, '{"usage":{"total_tokens":700}}'::jsonb, $13, $13, 'cmd-before-midnight', $5, $6, 'codex'),
			($12, $1, $10, $4, $3, 'completed', $14, $14, $14, '{"usage":{"total_tokens":300}}'::jsonb, $14, $14, 'cmd-after-midnight', $5, $6, 'codex');
	`, tenantID, teamID, runtimeNodeID, nodeID, employeeID, executionInstanceID, teamConfigRevisionID, employeeConfigRevisionID, taskBeforeID, taskInsideID, runBeforeID, runInsideID, beforeToday, insideToday)
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
