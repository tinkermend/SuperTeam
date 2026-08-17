package employee

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/superteam/control-plane/internal/storage/queries"
)

// TestListOrphanedActiveRuns（spec 2026-08-17-recovery-mode-and-active-run-
// poisoning-fix §4）：L3 交叉核对 SQL 的三表夹具——活跃 run × 回执 payload 关联
// × attempt 终态与宽限。夹具覆盖：命中（终态+过宽限）、attempt 非终态（不命中）、
// 宽限未到（不命中）、waiting_human 且 finished_at 为 NULL（COALESCE 口径命中）。
func TestListOrphanedActiveRuns(t *testing.T) {
	ctx := context.Background()
	cfg, ok := employeeRepoIntegrationTestConfig()
	if !ok {
		t.Skip("set TEST_DATABASE_URL, or set ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 with DATABASE_URL")
	}
	conn, err := pgx.Connect(ctx, cfg.databaseURL)
	require.NoError(t, err)
	defer conn.Close(ctx)

	schemaName := "orphan_runs_" + strings.ReplaceAll(strings.ToLower(uuid.NewString()), "-", "_")
	_, err = conn.Exec(ctx, `CREATE SCHEMA `+schemaName)
	require.NoError(t, err)
	defer conn.Exec(ctx, `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)
	_, err = conn.Exec(ctx, `SET search_path TO `+schemaName)
	require.NoError(t, err)
	require.NoError(t, runEmployeeRepoTestMigrations(ctx, conn))

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	_, err = conn.Exec(ctx, `INSERT INTO tenants (id, slug, name, status) VALUES ($1, 'orphan-probe', 'orphan-probe', 'active') ON CONFLICT (id) DO NOTHING`, tenantID)
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `INSERT INTO projects (tenant_id, id, name, directory_name, status, human_owner_user_id, coordination_workflow_id, coordination_status, coordination_policy, created_at, updated_at)
		VALUES ($1, $2, 'orphan-probe', 'orphan-probe', 'running', $3, 'wf:probe', 'registered', '{}', now(), now())`,
		tenantID, uuid.MustParse("00000000-0000-0000-0000-0000000000a1"), uuid.New())
	require.NoError(t, err)
	// 每个 attempt 独立 project_task 行：uq_project_task_attempts_active 把
	// waiting_human 计入活跃，同任务多活跃 attempt 会撞唯一约束。
	seedTaskSeq := 0
	// updatedAgo 回拨 updated_at：表有 update_project_task_attempts_updated_at
	// 触发器，INSERT 后再 UPDATE 会被覆盖回 now()，必须写入时指定。
	seedAttempt := func(id, status string, finishedAgo, updatedAgo *time.Duration) uuid.UUID {
		seedTaskSeq++
		taskID := uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", 0xb1+seedTaskSeq))
		_, err = conn.Exec(ctx, `INSERT INTO project_tasks (tenant_id, project_id, id, title, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'running', now(), now())`, tenantID, uuid.MustParse("00000000-0000-0000-0000-0000000000a1"), taskID, "probe-task-"+id)
		require.NoError(t, err)
		var finished, updated any = nil, time.Now()
		if finishedAgo != nil {
			finished = time.Now().Add(-*finishedAgo)
		}
		if updatedAgo != nil {
			updated = time.Now().Add(-*updatedAgo)
		}
		_, err = conn.Exec(ctx, `INSERT INTO project_task_attempts (tenant_id, project_task_id, id, attempt_no, status, lease_token, idempotency_key, created_at, updated_at, finished_at)
			VALUES ($1, $2, $3, 1, $4, 'lease-probe', $5, $6, $6, $7)`,
			tenantID, taskID, id, status, "idem-"+id, updated, finished)
		require.NoError(t, err)
		return taskID
	}
	seedRunAndReceipt := func(runID, commandID, attemptID, runStatus string) {
		_, err = conn.Exec(ctx, `INSERT INTO task_runs (tenant_id, id, task_id, node_id, project_id, status, command_id, created_at, updated_at)
			VALUES ($1, $2, $3, 'node-probe', $4, $5, $6, now(), now())`, tenantID, runID, uuid.New(), uuid.MustParse("00000000-0000-0000-0000-0000000000a1"), runStatus, commandID)
		require.NoError(t, err)
		payload := fmt.Sprintf(`{"metadata": {"project_task_attempt_id": "%s"}}`, attemptID)
		_, err = conn.Exec(ctx, `INSERT INTO runtime_command_receipts (tenant_id, id, command_id, command_type, runtime_node_id, node_id, resource_type, resource_id, status, payload, result, dispatched_at, created_at, updated_at)
			VALUES ($1, $2, $3, 'start_session', $4, 'node-probe', 'task', $5, 'dispatched', $6::jsonb, '{}'::jsonb, now(), now(), now())`,
			tenantID, uuid.New(), commandID, uuid.MustParse("00000000-0000-0000-0000-0000000000f1"), uuid.New(), payload)
		require.NoError(t, err)
	}

	tenMin := 10 * time.Minute
	oneMin := time.Minute
	seedAttempt("00000000-0000-0000-0000-0000000000c1", "failed", &tenMin, nil)        // 命中：终态+过宽限
	seedAttempt("00000000-0000-0000-0000-0000000000c2", "running", &tenMin, nil)       // 不命中：attempt 非终态
	seedAttempt("00000000-0000-0000-0000-0000000000c3", "failed", &oneMin, nil)        // 不命中：宽限未到
	seedAttempt("00000000-0000-0000-0000-0000000000c4", "waiting_human", nil, &tenMin) // 命中：waiting_human 且 finished_at NULL（COALESCE）
	seedRunAndReceipt("00000000-0000-0000-0000-0000000000d1", "cmd-orphan-1", "00000000-0000-0000-0000-0000000000c1", "running")
	seedRunAndReceipt("00000000-0000-0000-0000-0000000000d2", "cmd-orphan-2", "00000000-0000-0000-0000-0000000000c2", "running")
	seedRunAndReceipt("00000000-0000-0000-0000-0000000000d3", "cmd-orphan-3", "00000000-0000-0000-0000-0000000000c3", "running")
	seedRunAndReceipt("00000000-0000-0000-0000-0000000000d4", "cmd-orphan-4", "00000000-0000-0000-0000-0000000000c4", "cancelling")
	// 干扰项：run 终态但 attempt 终态（早已收敛，不该再被扫出）。
	seedRunAndReceipt("00000000-0000-0000-0000-0000000000d5", "cmd-orphan-5", "00000000-0000-0000-0000-0000000000c1", "completed")

	repo, ok := NewPgRunRepository(queries.New(conn), conn).(OrphanedActiveRunLister)
	require.True(t, ok, "PgRunRepository must implement OrphanedActiveRunLister")
	orphans, err := repo.ListOrphanedActiveRuns(ctx, time.Now().Add(-5*time.Minute), 100)
	require.NoError(t, err)
	got := map[string]string{}
	for _, item := range orphans {
		got[item.RunID.String()] = item.AttemptStatus
	}
	require.Len(t, got, 2)
	require.Equal(t, "failed", got["00000000-0000-0000-0000-0000000000d1"])
	require.Equal(t, "waiting_human", got["00000000-0000-0000-0000-0000000000d4"])
}
