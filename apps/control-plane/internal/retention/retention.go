// Package retention sweeps expired rows out of the append-only tables.
//
// Before this package there was no purge path anywhere in the schema: no
// PARTITION, no DELETE-by-age query, and the one cleanup query that did exist
// (DeleteExpiredSessions) had no caller. Telemetry tables grew without bound and
// soft-deleted projects kept every row they ever wrote.
//
// The policy is a registry (see Rules) rather than scattered cleanup calls, so
// what is deleted and what is kept is readable in one place. Business-fact
// tables — project_events, execution_ledger_events, audit_events — are never
// swept by age; they are the evidence chain the platform is built on. They are
// only cleared for projects that have been soft-deleted long enough to be past
// any recovery window.
package retention

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/platform"
	"github.com/superteam/control-plane/internal/storage/queries"
	"github.com/superteam/control-plane/internal/systemconfig"
)

// SweepInterval is how often the job runs. Retention is measured in days, so
// hourly is far more often than correctness needs; it just keeps each run small.
const SweepInterval = time.Hour

// maxBatchesPerRule bounds one rule's work in a single sweep so a table with a
// huge backlog cannot monopolise the run (or hold the advisory lock for hours).
// Whatever is left is picked up by the next sweep.
const maxBatchesPerRule = 20

// Reader is the subset of the system config service this package needs.
//
// Retention is platform-wide, not per tenant: a sweep deletes by age across all
// tenants. Its thresholds are therefore read from the platform default tenant,
// the same convention the stuck-task reconciler and session TTL already use.
type Reader interface {
	Int64(ctx context.Context, tenantID uuid.UUID, key string) int64
}

// Queries is the generated query surface used by the rules.
type Queries interface {
	DeleteExpiredRuntimeEvents(ctx context.Context, arg queries.DeleteExpiredRuntimeEventsParams) (int64, error)
	DeleteExpiredProviderSessionEvents(ctx context.Context, arg queries.DeleteExpiredProviderSessionEventsParams) (int64, error)
	DeleteExpiredRuntimeCommandReceipts(ctx context.Context, arg queries.DeleteExpiredRuntimeCommandReceiptsParams) (int64, error)
	DeleteExpiredTaskEvents(ctx context.Context, arg queries.DeleteExpiredTaskEventsParams) (int64, error)
	DeleteExpiredAuthzAllowOperationLogs(ctx context.Context, arg queries.DeleteExpiredAuthzAllowOperationLogsParams) (int64, error)
	DeleteExpiredOperationLogs(ctx context.Context, arg queries.DeleteExpiredOperationLogsParams) (int64, error)
	DeleteExpiredAuthSessions(ctx context.Context, batchSize int32) (int64, error)
	DeleteProjectEventsForPurgedProjects(ctx context.Context, arg queries.DeleteProjectEventsForPurgedProjectsParams) (int64, error)
	DeleteExecutionLedgerEventsForPurgedProjects(ctx context.Context, arg queries.DeleteExecutionLedgerEventsForPurgedProjectsParams) (int64, error)
}

// Rule is one retention policy: a name for logs, the config key holding its
// retention days (empty when the rule has no day dimension, e.g. expired
// sessions), and the batched delete itself.
type Rule struct {
	Name string
	// DaysKey is the systemconfig key for this rule's retention window. Empty
	// means the rule's predicate carries its own expiry (expired sessions).
	DaysKey string
	// Delete removes at most batchSize rows and reports how many it removed.
	Delete func(ctx context.Context, q Queries, days int32, batchSize int32) (int64, error)
}

// Rules is the retention policy of the platform, in one readable place.
//
// Deliberately absent: project_events, execution_ledger_events and audit_events
// by age. project_events has 11 read paths and backs the project timeline;
// execution_ledger_events is served by ListProjectExecutionLedgerEvents (the
// execution-trace panel); audit_events is the audit baseline. The first two are
// cleared only via the purged-project rules at the end of this list.
var Rules = []Rule{
	{
		Name:    "runtime_events",
		DaysKey: systemconfig.KeyRetentionRuntimeEventsDays,
		Delete: func(ctx context.Context, q Queries, days, batchSize int32) (int64, error) {
			return q.DeleteExpiredRuntimeEvents(ctx, queries.DeleteExpiredRuntimeEventsParams{RetentionDays: days, BatchSize: batchSize})
		},
	},
	{
		Name:    "provider_session_events",
		DaysKey: systemconfig.KeyRetentionProviderSessionEventsDays,
		Delete: func(ctx context.Context, q Queries, days, batchSize int32) (int64, error) {
			return q.DeleteExpiredProviderSessionEvents(ctx, queries.DeleteExpiredProviderSessionEventsParams{RetentionDays: days, BatchSize: batchSize})
		},
	},
	{
		Name:    "runtime_command_receipts",
		DaysKey: systemconfig.KeyRetentionCommandReceiptsDays,
		Delete: func(ctx context.Context, q Queries, days, batchSize int32) (int64, error) {
			return q.DeleteExpiredRuntimeCommandReceipts(ctx, queries.DeleteExpiredRuntimeCommandReceiptsParams{RetentionDays: days, BatchSize: batchSize})
		},
	},
	{
		Name:    "task_events",
		DaysKey: systemconfig.KeyRetentionTaskEventsDays,
		Delete: func(ctx context.Context, q Queries, days, batchSize int32) (int64, error) {
			return q.DeleteExpiredTaskEvents(ctx, queries.DeleteExpiredTaskEventsParams{RetentionDays: days, BatchSize: batchSize})
		},
	},
	{
		Name:    "authz_allow_logs",
		DaysKey: systemconfig.KeyRetentionAuthzAllowLogsDays,
		Delete: func(ctx context.Context, q Queries, days, batchSize int32) (int64, error) {
			return q.DeleteExpiredAuthzAllowOperationLogs(ctx, queries.DeleteExpiredAuthzAllowOperationLogsParams{RetentionDays: days, BatchSize: batchSize})
		},
	},
	{
		Name:    "operation_logs",
		DaysKey: systemconfig.KeyRetentionOperationLogsDays,
		Delete: func(ctx context.Context, q Queries, days, batchSize int32) (int64, error) {
			return q.DeleteExpiredOperationLogs(ctx, queries.DeleteExpiredOperationLogsParams{RetentionDays: days, BatchSize: batchSize})
		},
	},
	{
		Name: "expired_auth_sessions",
		Delete: func(ctx context.Context, q Queries, _ int32, batchSize int32) (int64, error) {
			return q.DeleteExpiredAuthSessions(ctx, batchSize)
		},
	},
	{
		Name:    "purged_project_events",
		DaysKey: systemconfig.KeyRetentionPurgedProjectDays,
		Delete: func(ctx context.Context, q Queries, days, batchSize int32) (int64, error) {
			return q.DeleteProjectEventsForPurgedProjects(ctx, queries.DeleteProjectEventsForPurgedProjectsParams{RetentionDays: days, BatchSize: batchSize})
		},
	},
	{
		Name:    "purged_project_ledger",
		DaysKey: systemconfig.KeyRetentionPurgedProjectDays,
		Delete: func(ctx context.Context, q Queries, days, batchSize int32) (int64, error) {
			return q.DeleteExecutionLedgerEventsForPurgedProjects(ctx, queries.DeleteExecutionLedgerEventsForPurgedProjectsParams{RetentionDays: days, BatchSize: batchSize})
		},
	},
}

// Service runs the retention rules.
type Service struct {
	queries Queries
	config  Reader
	// singleton guards against two Control Plane processes sweeping at once. It
	// may be nil, in which case the sweep runs unguarded (single-process dev).
	singleton Singleton
	rules     []Rule
}

// Singleton is a cross-process mutual exclusion primitive. The Postgres-backed
// implementation uses a session-scoped advisory lock; when leader election lands
// for the Control Plane it can be swapped here without touching the rules.
type Singleton interface {
	// TryAcquire reports whether this process won the lock. Release must only be
	// called when it returned true.
	TryAcquire(ctx context.Context) (bool, error)
	Release(ctx context.Context)
}

func NewService(q Queries, config Reader, singleton Singleton) *Service {
	return &Service{queries: q, config: config, singleton: singleton, rules: Rules}
}

// SweepResult reports how many rows each rule removed, keyed by rule name.
type SweepResult struct {
	Deleted map[string]int64
	Skipped bool // another process held the singleton
}

func (s *Service) Sweep(ctx context.Context) (SweepResult, error) {
	result := SweepResult{Deleted: map[string]int64{}}
	if s == nil || s.queries == nil {
		return result, errors.New("retention service is not configured")
	}
	if s.singleton != nil {
		acquired, err := s.singleton.TryAcquire(ctx)
		if err != nil {
			return result, fmt.Errorf("acquire retention singleton: %w", err)
		}
		if !acquired {
			result.Skipped = true
			return result, nil
		}
		defer s.singleton.Release(ctx)
	}

	batchSize := s.intValue(ctx, systemconfig.KeyRetentionSweepBatchSize, 5000)
	var firstErr error
	for _, rule := range s.rules {
		days := int32(0)
		if rule.DaysKey != "" {
			days = s.intValue(ctx, rule.DaysKey, 30)
		}
		deleted, err := s.runRule(ctx, rule, days, batchSize)
		result.Deleted[rule.Name] = deleted
		if err != nil {
			// One failing rule must not stop the rest: a lock timeout on a busy
			// table should not stop expired sessions from being cleared.
			if firstErr == nil {
				firstErr = fmt.Errorf("retention rule %s: %w", rule.Name, err)
			}
			continue
		}
	}
	return result, firstErr
}

func (s *Service) runRule(ctx context.Context, rule Rule, days, batchSize int32) (int64, error) {
	var total int64
	for batch := 0; batch < maxBatchesPerRule; batch++ {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		deleted, err := rule.Delete(ctx, s.queries, days, batchSize)
		if err != nil {
			return total, err
		}
		total += deleted
		// A short batch means the table is drained for this window.
		if deleted < int64(batchSize) {
			return total, nil
		}
	}
	return total, nil
}

func (s *Service) intValue(ctx context.Context, key string, fallback int32) int32 {
	if s.config == nil {
		return fallback
	}
	value := s.config.Int64(ctx, platform.DefaultTenantID, key)
	if value <= 0 {
		return fallback
	}
	return int32(value)
}

// Start runs the sweep on a ticker until ctx is cancelled.
func (s *Service) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = SweepInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	// Sweep once on start so a deploy catches up on backlog immediately rather
	// than waiting a full interval; the stuck-task reconciler does the same.
	first := true
	for {
		if !first {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
		first = false
		result, err := s.Sweep(ctx)
		if err != nil {
			slog.Error("retention sweep failed", "error", err, "deleted", result.Deleted)
			continue
		}
		if result.Skipped {
			continue
		}
		var total int64
		for _, n := range result.Deleted {
			total += n
		}
		if total > 0 {
			slog.Info("retention sweep removed expired rows", "total", total, "by_rule", result.Deleted)
		}
	}
}
