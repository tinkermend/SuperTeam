package project

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/employee"
)

// RunAttemptStateAdapter 把 project 域的 attempt 终态事实供给 employee 域的
// L2 死亡证据核对（spec 2026-08-17-recovery-mode-and-active-run-poisoning-fix
// §2.3）。依赖方向与 ProjectDispatchFactsAdapter 一致：project → employee。
type RunAttemptStateAdapter struct {
	service *Service
}

// NewRunAttemptStateAdapter 返回 employee.RunAttemptStateChecker 适配器；
// service 为 nil 时返回 nil（L2 层关闭）。
func NewRunAttemptStateAdapter(service *Service) employee.RunAttemptStateChecker {
	if service == nil {
		return nil
	}
	return RunAttemptStateAdapter{service: service}
}

func (a RunAttemptStateAdapter) ProjectTaskAttemptTerminalState(ctx context.Context, tenantID, attemptID uuid.UUID) (string, time.Time, bool) {
	if a.service == nil || a.service.repository == nil {
		return "", time.Time{}, false
	}
	attempt, err := a.service.repository.GetProjectTaskAttempt(ctx, tenantID, attemptID)
	if err != nil || attempt.TenantID != tenantID {
		return "", time.Time{}, false
	}
	// finished_at 为 NULL 时（waiting_human 可为 NULL）回退 updated_at，与 L3 SQL 的
	// COALESCE(finished_at, updated_at) 口径对齐（spec §2.3 拍板点 3）。
	finishedAt := attempt.UpdatedAt
	if attempt.FinishedAt != nil {
		finishedAt = *attempt.FinishedAt
	}
	return attempt.Status, finishedAt, true
}
