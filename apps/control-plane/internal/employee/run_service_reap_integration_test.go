package employee

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/superteam/control-plane/internal/storage/queries"
)

// TestRunServiceCreateRunReapsStaleDispatchingRunAgainstPostgres drives the real
// DigitalEmployeeRunService against a real Postgres schema and proves that an
// abandoned pre-confirmation run (dispatching, untouched past staleDispatchTTL)
// is reaped when a new dispatch for the same digital employee arrives: the stale
// row is marked failed/dispatch_stale, a run_reaped_stale lifecycle event is
// recorded, and a fresh run is created and dispatched. The repository, the
// service, and the lifecycle/audit writes are all real; only the runtime
// dispatcher is faked (it reports the node connected and records commands).
func TestRunServiceCreateRunReapsStaleDispatchingRunAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	cfg, ok := employeeRunRepositoryTestConfig()
	if !ok {
		t.Skip("set TEST_DATABASE_URL and TEST_REDIS_URL, or set ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 with DATABASE_URL and REDIS_URL")
	}
	require.NoError(t, pingEmployeeRunRepositoryTestRedis(ctx, cfg.redisURL))

	conn, err := pgx.Connect(ctx, cfg.databaseURL)
	require.NoError(t, err)
	defer conn.Close(ctx)

	schemaName := "employee_run_reap_" + strings.ReplaceAll(strings.ToLower(uuid.NewString()), "-", "_")
	_, err = conn.Exec(ctx, `CREATE SCHEMA `+schemaName)
	require.NoError(t, err)
	defer conn.Exec(ctx, `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)

	_, err = conn.Exec(ctx, `SET search_path TO `+schemaName)
	require.NoError(t, err)
	require.NoError(t, runEmployeeRepositoryTestMigrations(ctx, conn))

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	runtimeNodeID := uuid.New()
	authoritativeNodeID := "runtime-authoritative"
	employeeID := uuid.New()
	executionInstanceID := uuid.New()
	teamConfigRevisionID := uuid.New()
	employeeConfigRevisionID := uuid.New()

	require.NoError(t, seedEmployeeRunGraph(ctx, conn, tenantID, teamID, runtimeNodeID, authoritativeNodeID, employeeID, executionInstanceID, teamConfigRevisionID, employeeConfigRevisionID))

	repo := NewPgRunRepository(queries.New(conn))
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[authoritativeNodeID] = true
	audit := &fakeRunServiceAuditLogger{}
	service := mustNewRunService(t, repo, dispatcher, audit)

	// First dispatch: the runtime confirms nothing yet, so the run lands in
	// dispatching and holds the single active-run slot.
	req := reapIntegrationCreateRunRequest(tenantID, employeeID, "first objective: stand up the run")
	runA, err := service.CreateRun(ctx, req)
	require.NoError(t, err, "first dispatch should succeed against real DB")
	require.Equal(t, DigitalEmployeeRunStatusDispatching, runA.Status, "first run should reach dispatching")

	// Simulate the run going stale: the runtime never confirmed back, and the
	// row has sat untouched past staleDispatchTTL. The before-update trigger
	// normally pins updated_at to now(), so backdate it with the trigger
	// disabled in this isolated schema.
	_, err = conn.Exec(ctx, `ALTER TABLE task_runs DISABLE TRIGGER update_task_runs_updated_at`)
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `UPDATE task_runs SET status = 'dispatching', updated_at = NOW() - INTERVAL '10 minutes' WHERE id = $1`, runA.ID)
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `ALTER TABLE task_runs ENABLE TRIGGER update_task_runs_updated_at`)
	require.NoError(t, err)

	// Second dispatch for the same digital employee: the stale run must be
	// reaped so the new dispatch can proceed.
	reqB := reapIntegrationCreateRunRequest(tenantID, employeeID, "second objective: should reap the abandoned run")
	runB, err := service.CreateRun(ctx, reqB)
	require.NoError(t, err, "second dispatch should reap the stale run and create a new one")
	require.NotEqual(t, runA.ID, runB.ID, "a fresh run should be created after reap")
	require.Equal(t, DigitalEmployeeRunStatusDispatching, runB.Status, "new run should reach dispatching")

	// The abandoned run must now be marked failed/dispatch_stale in the real DB.
	var staleStatus, staleCode, staleFamily, staleMessage string
	require.NoError(t, conn.QueryRow(ctx,
		`SELECT status, COALESCE(error_code,''), COALESCE(error_family,''), COALESCE(error_message,'') FROM task_runs WHERE id = $1`,
		runA.ID,
	).Scan(&staleStatus, &staleCode, &staleFamily, &staleMessage))
	require.Equal(t, string(DigitalEmployeeRunStatusFailed), staleStatus, "stale run must be failed after reap")
	require.Equal(t, "dispatch_stale", staleCode, "stale run must carry dispatch_stale error code")
	require.Equal(t, "dispatch_timeout", staleFamily, "stale run must carry dispatch_timeout error family")
	require.Contains(t, staleMessage, "reaped as stale", "stale run error message must explain the reap")

	// A run_reaped_stale lifecycle event must have been recorded for the run.
	var eventType string
	var seqNo int
	require.NoError(t, conn.QueryRow(ctx,
		`SELECT event_type, sequence_number FROM task_events WHERE run_id = $1 AND event_type = 'run_reaped_stale' LIMIT 1`,
		runA.ID,
	).Scan(&eventType, &seqNo))
	require.Equal(t, "run_reaped_stale", eventType)
	require.Equal(t, runReapedStaleLifecycleSequence, seqNo, "run_reaped_stale must use its dedicated lifecycle sequence number")

	// The new run was dispatched to the runtime node.
	var dispatched int
	for _, c := range dispatcher.commands {
		if c.nodeID == authoritativeNodeID {
			dispatched++
		}
	}
	require.GreaterOrEqual(t, dispatched, 1, "the new run must be dispatched to the runtime node")

	// Audit must attribute the reap to the triggering actor.
	foundReapAudit := false
	for _, e := range audit.events {
		if e.eventType == "digital_employee_run_reaped_stale" && e.resourceID == runA.ID.String() {
			foundReapAudit = true
			require.Equal(t, "employee.run.reap_stale", e.action)
		}
	}
	require.True(t, foundReapAudit, "reap must be recorded in the audit log")
}

// TestRunServiceCreateRunDoesNotReapRecentDispatchingRunAgainstPostgres is the
// real-DB negative: a dispatching run still inside the staleness window is a
// genuine in-flight dispatch, so it must NOT be reaped — CreateRun returns
// ErrConflict and leaves the row untouched.
func TestRunServiceCreateRunDoesNotReapRecentDispatchingRunAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	cfg, ok := employeeRunRepositoryTestConfig()
	if !ok {
		t.Skip("set TEST_DATABASE_URL and TEST_REDIS_URL, or set ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 with DATABASE_URL and REDIS_URL")
	}
	require.NoError(t, pingEmployeeRunRepositoryTestRedis(ctx, cfg.redisURL))

	conn, err := pgx.Connect(ctx, cfg.databaseURL)
	require.NoError(t, err)
	defer conn.Close(ctx)

	schemaName := "employee_run_reap_neg_" + strings.ReplaceAll(strings.ToLower(uuid.NewString()), "-", "_")
	_, err = conn.Exec(ctx, `CREATE SCHEMA `+schemaName)
	require.NoError(t, err)
	defer conn.Exec(ctx, `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)

	_, err = conn.Exec(ctx, `SET search_path TO `+schemaName)
	require.NoError(t, err)
	require.NoError(t, runEmployeeRepositoryTestMigrations(ctx, conn))

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	runtimeNodeID := uuid.New()
	authoritativeNodeID := "runtime-authoritative"
	employeeID := uuid.New()
	executionInstanceID := uuid.New()
	teamConfigRevisionID := uuid.New()
	employeeConfigRevisionID := uuid.New()

	require.NoError(t, seedEmployeeRunGraph(ctx, conn, tenantID, teamID, runtimeNodeID, authoritativeNodeID, employeeID, executionInstanceID, teamConfigRevisionID, employeeConfigRevisionID))

	repo := NewPgRunRepository(queries.New(conn))
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[authoritativeNodeID] = true
	service := mustNewRunService(t, repo, dispatcher, &fakeRunServiceAuditLogger{})

	// Recent dispatching run — still within staleDispatchTTL.
	recent, err := service.CreateRun(ctx, reapIntegrationCreateRunRequest(tenantID, employeeID, "in-flight dispatch"))
	require.NoError(t, err)
	require.Equal(t, DigitalEmployeeRunStatusDispatching, recent.Status)

	_, err = service.CreateRun(ctx, reapIntegrationCreateRunRequest(tenantID, employeeID, "second dispatch should conflict, not reap"))
	require.ErrorIs(t, err, ErrConflict, "recent dispatching run must conflict, not be reaped")

	// The recent run must remain dispatching (untouched), not failed.
	var status, code string
	require.NoError(t, conn.QueryRow(ctx,
		`SELECT status, COALESCE(error_code,'') FROM task_runs WHERE id = $1`, recent.ID,
	).Scan(&status, &code))
	require.Equal(t, string(DigitalEmployeeRunStatusDispatching), status, "recent run must not be reaped")
	require.Empty(t, code, "recent run must not carry a reap error code")
}

func reapIntegrationCreateRunRequest(tenantID, employeeID uuid.UUID, objective string) CreateDigitalEmployeeRunRequest {
	timeoutSec := int32(120)
	graceSec := int32(15)
	return CreateDigitalEmployeeRunRequest{
		TenantID:          tenantID,
		UserID:            uuid.New(),
		DigitalEmployeeID: employeeID,
		Objective:         objective,
		Prompt:            "proceed with the assigned objective",
		TimeoutSec:        &timeoutSec,
		GraceSec:          &graceSec,
		Metadata:          map[string]any{"source": "reap-integration-test"},
	}
}

// seedEmployeeRunGraph inserts the minimum real entity graph (tenant, team,
// runtime node + capability, digital employee, execution instance, config
// revisions, approved effective config) that GetRunPreflight resolves for a
// dispatchable digital employee. It mirrors the production seeding shape used
// by the run-loop repository tests. The graph is seeded as a single arg-free
// (simple-protocol) multi-statement query, because pgx's extended protocol
// rejects multi-statement queries that carry bind parameters.
func seedEmployeeRunGraph(
	ctx context.Context,
	conn *pgx.Conn,
	tenantID, teamID, runtimeNodeID uuid.UUID,
	authoritativeNodeID string,
	employeeID, executionInstanceID, teamConfigRevisionID, employeeConfigRevisionID uuid.UUID,
) error {
	ownerUserID := uuid.New()
	sql := fmt.Sprintf(`
		INSERT INTO tenants (id, slug, name, status)
		VALUES ('%s', 'default', '默认租户', 'active')
		ON CONFLICT (id) DO UPDATE SET status = EXCLUDED.status;

		INSERT INTO tenant_teams (id, tenant_id, slug, name, status)
		VALUES ('%s', '%s', 'default', '默认团队', 'active')
		ON CONFLICT (id) DO UPDATE SET status = EXCLUDED.status;

		INSERT INTO runtime_nodes (
			id, tenant_id, node_id, name, supported_providers,
			max_slots, current_load, status, metadata, last_heartbeat_at
		) VALUES (
			'%s', '%s', '%s', 'Runtime Authoritative', '["codex"]'::jsonb,
			2, 0, 'online', '{}'::jsonb, NOW()
		);

		INSERT INTO runtime_capabilities (
			tenant_id, runtime_node_id, capability_type, capability_key,
			provider_type, provider_version, binary_path, available,
			workspace_base_dir, capacity, labels, status, details,
			health_status, metadata, last_seen_at
		) VALUES (
			'%s', '%s', 'provider', 'provider:codex', 'codex', '1.0.0',
			'/usr/local/bin/codex', true, '/tmp/superteam', '{}'::jsonb,
			'{}'::jsonb, 'healthy', '{}'::jsonb, 'healthy', '{}'::jsonb, NOW()
		);

		INSERT INTO digital_employees (
			id, tenant_id, team_id, name, role, employee_type, owner_user_id, status,
			permission_policy, context_policy, approval_policy,
			risk_level, metadata
		) VALUES (
			'%s', '%s', '%s', '执行员工', 'operator', 'operator', '%s', 'ready',
			'{}'::jsonb, '{}'::jsonb, '{}'::jsonb,
			'normal', '{}'::jsonb
		);

		INSERT INTO digital_employee_execution_instances (
			id, tenant_id, digital_employee_id, runtime_node_id, provider_type,
			agent_home_dir, workspace_policy, session_policy, runtime_selector,
			capacity_requirements, fallback_policy, status, ready_at, metadata
		) VALUES (
			'%s', '%s', '%s', '%s', 'codex',
			'/var/lib/superteam/agents/employee',
			'{"workspace":"isolated"}'::jsonb, '{"resume":true}'::jsonb,
			'{}'::jsonb, '{}'::jsonb, '{}'::jsonb,
			'ready', NOW(), '{}'::jsonb
		);

		INSERT INTO tenant_team_config_revisions (
			id, tenant_id, team_id, revision_number, constitution,
			capability_policy, context_policy, approval_policy,
			artifact_contract, internal_collaboration_policy,
			runtime_scope_policy, status, approved_at
		) VALUES (
			'%s', '%s', '%s', 1, '{}'::jsonb, '{}'::jsonb, '{}'::jsonb,
			'{}'::jsonb, '{}'::jsonb, '{}'::jsonb, '{}'::jsonb,
			'active', NOW()
		);

		INSERT INTO digital_employee_config_revisions (
			id, tenant_id, digital_employee_id, revision_number, role_profile,
			constitution_addendum, capability_selection,
			context_policy_override, approval_policy_override,
			output_contract_addendum, status
		) VALUES (
			'%s', '%s', '%s', 1, '{}'::jsonb, '{}'::jsonb, '{}'::jsonb,
			'{}'::jsonb, '{}'::jsonb, '{}'::jsonb, 'draft'
		);

		INSERT INTO digital_employee_effective_configs (
			tenant_id, digital_employee_id,
			tenant_team_config_revision_id, employee_config_revision_id,
			effective_config_snapshot, validation_result, status, approved_at
		) VALUES (
			'%s', '%s', '%s', '%s', '{}'::jsonb, '{}'::jsonb, 'approved', NOW()
		);
	`,
		tenantID,
		teamID, tenantID,
		runtimeNodeID, tenantID, authoritativeNodeID,
		tenantID, runtimeNodeID,
		employeeID, tenantID, teamID, ownerUserID,
		executionInstanceID, tenantID, employeeID, runtimeNodeID,
		teamConfigRevisionID, tenantID, teamID,
		employeeConfigRevisionID, tenantID, employeeID,
		tenantID, employeeID, teamConfigRevisionID, employeeConfigRevisionID,
	)
	_, err := conn.Exec(ctx, sql)
	return err
}
