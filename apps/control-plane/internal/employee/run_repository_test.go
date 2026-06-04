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
	`, tenantID, teamID, runtimeNodeID, authoritativeNodeID, employeeID, executionInstanceID)
	require.NoError(t, err)

	repo := NewPgRunRepository(queries.New(conn))
	preflight, err := repo.GetRunPreflight(ctx, tenantID, employeeID)

	require.NoError(t, err)
	require.Equal(t, authoritativeNodeID, preflight.NodeID)
	require.Equal(t, runtimeNodeID, preflight.RuntimeNodeID)
	require.Equal(t, executionInstanceID, preflight.ExecutionInstanceID)
	require.Equal(t, "codex", preflight.ProviderType)
	require.Equal(t, "isolated", preflight.WorkspacePolicy["workspace"])
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
