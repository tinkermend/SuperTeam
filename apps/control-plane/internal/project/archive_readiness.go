package project

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
)

// archiveReadiness is the single source of truth for archive hard gates and warnings.
// Used by assertProjectReadyToArchive, BuildArchivePreview, and tryCloseProjectFromDemandSignOff.
type archiveReadiness struct {
	Blockers []ProjectArchiveBlocker
	Warnings []ProjectArchiveWarning
}

func (r archiveReadiness) CanArchive() bool {
	return len(r.Blockers) == 0
}

func (r archiveReadiness) Message() string {
	if r.CanArchive() {
		if len(r.Warnings) == 0 {
			return "项目可以归档"
		}
		return "项目可以归档，请确认下列提示"
	}
	parts := make([]string, 0, len(r.Blockers))
	for _, b := range r.Blockers {
		parts = append(parts, b.Message)
	}
	return "无法归档：" + strings.Join(parts, "；")
}

func (r archiveReadiness) CompatBlockedReasons() []any {
	out := make([]any, 0, len(r.Blockers)+len(r.Warnings))
	for _, b := range r.Blockers {
		out = append(out, b.Code)
	}
	for _, w := range r.Warnings {
		out = append(out, w.Code)
	}
	return out
}

func (s *Service) evaluateArchiveReadiness(ctx context.Context, tenantID, projectID uuid.UUID, project Project, evidenceCount, reportCount int) (archiveReadiness, error) {
	var ready archiveReadiness

	if projectArchived(project) {
		ready.Blockers = append(ready.Blockers, ProjectArchiveBlocker{
			Code:    "already_archived",
			Message: "项目已归档",
			Count:   1,
		})
	}

	taskSummary, err := s.repository.GetProjectTaskStatusCounts(ctx, tenantID, projectID)
	if err != nil {
		return ready, err
	}
	if taskSummary.ActiveTasks > 0 {
		ready.Blockers = append(ready.Blockers, ProjectArchiveBlocker{
			Code:    "active_tasks",
			Message: fmt.Sprintf("仍有 %d 个未完结任务", taskSummary.ActiveTasks),
			Count:   int(taskSummary.ActiveTasks),
		})
	}

	openDemands, err := s.repository.CountNonTerminalProjectDemands(ctx, tenantID, projectID)
	if err != nil {
		return ready, err
	}
	if openDemands > 0 {
		ready.Blockers = append(ready.Blockers, ProjectArchiveBlocker{
			Code:    "open_demands",
			Message: fmt.Sprintf("仍有 %d 个未结需求", openDemands),
			Count:   int(openDemands),
		})
	}

	deleteCounts, err := s.repository.GetProjectDeletePreviewCounts(ctx, tenantID, projectID)
	if err != nil {
		return ready, err
	}
	if deleteCounts.PendingDecisionCount > 0 {
		ready.Blockers = append(ready.Blockers, ProjectArchiveBlocker{
			Code:    "pending_decisions",
			Message: fmt.Sprintf("仍有 %d 个待决决策", deleteCounts.PendingDecisionCount),
			Count:   int(deleteCounts.PendingDecisionCount),
		})
	}

	// D11: missing_final_report 并入 missing_evidence（材料不全）。
	missingMaterial := 0
	if evidenceCount == 0 {
		missingMaterial++
	}
	if reportCount == 0 {
		missingMaterial++
	}
	if missingMaterial > 0 {
		ready.Warnings = append(ready.Warnings, ProjectArchiveWarning{
			Code:    "missing_evidence",
			Message: "材料不全：缺少证据或最终报告",
			Count:   missingMaterial,
		})
	}

	if deleteCounts.OpenInboxCount > 0 {
		ready.Warnings = append(ready.Warnings, ProjectArchiveWarning{
			Code:    "open_inbox_will_cancel",
			Message: fmt.Sprintf("归档将取消 %d 条待办收件箱", deleteCounts.OpenInboxCount),
			Count:   int(deleteCounts.OpenInboxCount),
		})
	}

	return ready, nil
}

func (s *Service) assertProjectReadyToArchive(ctx context.Context, tenantID, projectID uuid.UUID) error {
	project, err := s.repository.GetProject(ctx, tenantID, projectID)
	if err != nil {
		return err
	}
	// Evidence/report counts are only needed for warnings; hard gates do not depend on them.
	ready, err := s.evaluateArchiveReadiness(ctx, tenantID, projectID, project, 1, 1)
	if err != nil {
		return err
	}
	if !ready.CanArchive() {
		return &ProjectArchiveBlockedError{
			Blockers: append([]ProjectArchiveBlocker(nil), ready.Blockers...),
			Message:  ready.Message(),
		}
	}
	return nil
}

func (s *Service) recordAutoCloseDeferred(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID, blockers []ProjectArchiveBlocker) {
	codes := make([]string, 0, len(blockers))
	payloadBlockers := make([]map[string]any, 0, len(blockers))
	for _, b := range blockers {
		codes = append(codes, fmt.Sprintf("%s(%d)", b.Code, b.Count))
		payloadBlockers = append(payloadBlockers, map[string]any{
			"code":    b.Code,
			"message": b.Message,
			"count":   b.Count,
		})
	}
	slog.Info("project auto close deferred after demand sign-off",
		"tenant_id", tenantID.String(),
		"project_id", projectID.String(),
		"blockers", strings.Join(codes, ","),
	)
	summary := "通过并结项延后：项目未达归档条件"
	if len(codes) > 0 {
		summary = "通过并结项延后：" + strings.Join(codes, "；")
	}
	actorID := ""
	if actorUserID != uuid.Nil {
		actorID = actorUserID.String()
	}
	if _, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: ProjectEventArchiveAutoCloseDeferred,
		ActorType: "human_user",
		ActorID:   actorID,
		Summary:   summary,
		Payload: map[string]any{
			"reason":   "also_close_project_deferred",
			"blockers": payloadBlockers,
		},
	}); err != nil {
		slog.Warn("failed to append auto-close deferred event",
			"tenant_id", tenantID.String(),
			"project_id", projectID.String(),
			"error", err,
		)
	}
}
