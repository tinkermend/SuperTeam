package retention

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/superteam/control-plane/internal/storage/queries"
	"github.com/superteam/control-plane/internal/systemconfig"
)

type recordingQueries struct {
	calls  []string
	rows   map[string]int64 // rule name -> rows deleted per batch
	errFor string
}

func (q *recordingQueries) take(name string, batchSize int32) (int64, error) {
	q.calls = append(q.calls, name)
	if q.errFor == name {
		return 0, errors.New("boom")
	}
	if n, ok := q.rows[name]; ok {
		return n, nil
	}
	return 0, nil
}

func (q *recordingQueries) DeleteExpiredRuntimeEvents(_ context.Context, arg queries.DeleteExpiredRuntimeEventsParams) (int64, error) {
	return q.take("runtime_events", arg.BatchSize)
}

func (q *recordingQueries) DeleteExpiredProviderSessionEvents(_ context.Context, arg queries.DeleteExpiredProviderSessionEventsParams) (int64, error) {
	return q.take("provider_session_events", arg.BatchSize)
}

func (q *recordingQueries) DeleteExpiredRuntimeCommandReceipts(_ context.Context, arg queries.DeleteExpiredRuntimeCommandReceiptsParams) (int64, error) {
	return q.take("runtime_command_receipts", arg.BatchSize)
}

func (q *recordingQueries) DeleteExpiredTaskEvents(_ context.Context, arg queries.DeleteExpiredTaskEventsParams) (int64, error) {
	return q.take("task_events", arg.BatchSize)
}

func (q *recordingQueries) DeleteExpiredAuthzAllowOperationLogs(_ context.Context, arg queries.DeleteExpiredAuthzAllowOperationLogsParams) (int64, error) {
	return q.take("authz_allow_logs", arg.BatchSize)
}

func (q *recordingQueries) DeleteExpiredOperationLogs(_ context.Context, arg queries.DeleteExpiredOperationLogsParams) (int64, error) {
	return q.take("operation_logs", arg.BatchSize)
}

func (q *recordingQueries) DeleteExpiredAuthSessions(_ context.Context, batchSize int32) (int64, error) {
	return q.take("expired_auth_sessions", batchSize)
}

func (q *recordingQueries) DeleteProjectEventsForPurgedProjects(_ context.Context, arg queries.DeleteProjectEventsForPurgedProjectsParams) (int64, error) {
	return q.take("purged_project_events", arg.BatchSize)
}

func (q *recordingQueries) DeleteExecutionLedgerEventsForPurgedProjects(_ context.Context, arg queries.DeleteExecutionLedgerEventsForPurgedProjectsParams) (int64, error) {
	return q.take("purged_project_ledger", arg.BatchSize)
}

type staticConfig map[string]int64

func (c staticConfig) Int64(_ context.Context, _ uuid.UUID, key string) int64 { return c[key] }

type fakeSingleton struct {
	acquired   bool
	acquires   int
	releases   int
	acquireErr error
}

func (s *fakeSingleton) TryAcquire(context.Context) (bool, error) {
	s.acquires++
	return s.acquired, s.acquireErr
}

func (s *fakeSingleton) Release(context.Context) { s.releases++ }

// The retention policy is the whole point of this package: business-fact tables
// must never be swept by age. project_events backs the project timeline (11 read
// paths), execution_ledger_events backs the execution-trace panel, and
// audit_events is the audit baseline. They may only be cleared through the
// purged-project rules, which are gated on the project already being soft-deleted.
func TestRulesNeverSweepBusinessFactTablesByAge(t *testing.T) {
	for _, rule := range Rules {
		if rule.DaysKey == systemconfig.KeyRetentionPurgedProjectDays {
			// Purged-project rules are allowed to touch fact tables; they are
			// scoped to projects that were soft-deleted long enough ago.
			require.True(t, strings.HasPrefix(rule.Name, "purged_project_"),
				"only purged_project_* rules may use the purged-project window, got %q", rule.Name)
			continue
		}
		require.NotContains(t, rule.Name, "project_events")
		require.NotContains(t, rule.Name, "execution_ledger")
		require.NotContains(t, rule.Name, "audit_events")
	}
}

func TestSweepRunsEveryRuleAndReportsCounts(t *testing.T) {
	q := &recordingQueries{rows: map[string]int64{"runtime_events": 3, "expired_auth_sessions": 7}}
	svc := NewService(q, staticConfig{systemconfig.KeyRetentionSweepBatchSize: 100}, nil)

	result, err := svc.Sweep(context.Background())

	require.NoError(t, err)
	require.False(t, result.Skipped)
	require.Len(t, result.Deleted, len(Rules))
	require.Equal(t, int64(3), result.Deleted["runtime_events"])
	require.Equal(t, int64(7), result.Deleted["expired_auth_sessions"])
	require.Contains(t, q.calls, "purged_project_ledger")
}

// A short batch means the window is drained; the rule must stop rather than
// keep issuing deletes.
func TestSweepStopsRuleOnShortBatch(t *testing.T) {
	q := &recordingQueries{rows: map[string]int64{"runtime_events": 5}}
	svc := NewService(q, staticConfig{systemconfig.KeyRetentionSweepBatchSize: 100}, nil)

	_, err := svc.Sweep(context.Background())

	require.NoError(t, err)
	runtimeCalls := 0
	for _, call := range q.calls {
		if call == "runtime_events" {
			runtimeCalls++
		}
	}
	require.Equal(t, 1, runtimeCalls, "a short batch must end the rule")
}

// A full batch means more rows remain, but one rule must not monopolise the
// sweep — maxBatchesPerRule caps it and the rest waits for the next run.
func TestSweepCapsBatchesPerRule(t *testing.T) {
	q := &recordingQueries{rows: map[string]int64{"runtime_events": 100}}
	svc := NewService(q, staticConfig{systemconfig.KeyRetentionSweepBatchSize: 100}, nil)

	result, err := svc.Sweep(context.Background())

	require.NoError(t, err)
	require.Equal(t, int64(100*maxBatchesPerRule), result.Deleted["runtime_events"])
}

// One failing rule must not stop the others: a lock timeout on a busy telemetry
// table should not stop expired sessions from being cleared.
func TestSweepContinuesAfterRuleFailure(t *testing.T) {
	q := &recordingQueries{rows: map[string]int64{"expired_auth_sessions": 4}, errFor: "runtime_events"}
	svc := NewService(q, staticConfig{systemconfig.KeyRetentionSweepBatchSize: 100}, nil)

	result, err := svc.Sweep(context.Background())

	require.Error(t, err)
	require.Contains(t, err.Error(), "runtime_events")
	require.Equal(t, int64(4), result.Deleted["expired_auth_sessions"], "later rules must still run")
}

// Losing the singleton means another Control Plane process is already sweeping;
// this one must do nothing rather than delete concurrently.
func TestSweepSkipsWhenSingletonNotAcquired(t *testing.T) {
	q := &recordingQueries{}
	singleton := &fakeSingleton{acquired: false}
	svc := NewService(q, staticConfig{}, singleton)

	result, err := svc.Sweep(context.Background())

	require.NoError(t, err)
	require.True(t, result.Skipped)
	require.Empty(t, q.calls, "no deletes may run without the singleton")
	require.Equal(t, 0, singleton.releases, "must not release a lock it never held")
}

func TestSweepReleasesSingletonWhenAcquired(t *testing.T) {
	q := &recordingQueries{}
	singleton := &fakeSingleton{acquired: true}
	svc := NewService(q, staticConfig{}, singleton)

	_, err := svc.Sweep(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, singleton.acquires)
	require.Equal(t, 1, singleton.releases)
}

// Unset or nonsensical config must fall back to a sane window rather than
// deleting with days=0, which would wipe the table.
func TestSweepUsesFallbackDaysWhenConfigMissing(t *testing.T) {
	var seenDays int32 = -1
	q := &recordingQueries{}
	svc := NewService(q, staticConfig{}, nil)
	svc.rules = []Rule{{
		Name:    "probe",
		DaysKey: systemconfig.KeyRetentionRuntimeEventsDays,
		Delete: func(_ context.Context, _ Queries, days, _ int32) (int64, error) {
			seenDays = days
			return 0, nil
		},
	}}

	_, err := svc.Sweep(context.Background())

	require.NoError(t, err)
	require.Equal(t, int32(30), seenDays, "missing config must not degrade to days=0")
}
