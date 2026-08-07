package projectcoordination

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/project"
)

// sessionResumeOutcome 是派发层消费的会话接续结论（来自 employee，不反向依赖）。
type sessionResumeOutcome struct {
	Status     string
	SkipReason string
	SessionID  string
	Summary    string
	Label      string
}

func (o sessionResumeOutcome) shouldEmit() bool {
	return o.Status == "resumed" || o.Status == "skipped"
}

func sessionResumeOutcomeFromRun(run StartProjectTaskRunResult) sessionResumeOutcome {
	return sessionResumeOutcome{
		Status:     strings.TrimSpace(run.SessionResumeStatus),
		SkipReason: strings.TrimSpace(run.SessionResumeSkipReason),
		SessionID:  strings.TrimSpace(run.SessionResumeSessionID),
		Summary:    strings.TrimSpace(run.SessionResumeSummary),
		Label:      strings.TrimSpace(run.SessionResumeLabel),
	}
}

func sessionResumeOutcomeFromPacket(packet map[string]any) sessionResumeOutcome {
	if packet == nil {
		return sessionResumeOutcome{}
	}
	status, _ := packet["session_resume_status"].(string)
	skip, _ := packet["session_resume_skip_reason"].(string)
	if skip == "" {
		skip, _ = packet["session_resume_skipped"].(string)
	}
	sessionID := ""
	switch strings.TrimSpace(status) {
	case "resumed":
		if v, ok := packet["provider_session_id"].(string); ok {
			sessionID = v
		}
		if sessionID == "" {
			if v, ok := packet["session_resume_session_id"].(string); ok {
				sessionID = v
			}
		}
	case "skipped":
		if v, ok := packet["session_resume_skipped_session_id"].(string); ok {
			sessionID = v
		}
		if sessionID == "" {
			if v, ok := packet["session_resume_session_id"].(string); ok {
				sessionID = v
			}
		}
	}
	summary, _ := packet["session_resume_summary"].(string)
	label, _ := packet["session_resume_label"].(string)
	return sessionResumeOutcome{
		Status:     strings.TrimSpace(status),
		SkipReason: strings.TrimSpace(skip),
		SessionID:  strings.TrimSpace(sessionID),
		Summary:    strings.TrimSpace(summary),
		Label:      strings.TrimSpace(label),
	}
}

func attachSessionResumeOutcomeToPacket(packet map[string]any, outcome sessionResumeOutcome) {
	if packet == nil || outcome.Status == "" {
		return
	}
	packet["session_resume_status"] = outcome.Status
	if outcome.SkipReason != "" {
		packet["session_resume_skip_reason"] = outcome.SkipReason
		packet["session_resume_skipped"] = outcome.SkipReason
	}
	if outcome.SessionID != "" {
		packet["session_resume_session_id"] = outcome.SessionID
		if outcome.Status == "resumed" {
			packet["provider_session_id"] = outcome.SessionID
		}
		if outcome.Status == "skipped" {
			packet["session_resume_skipped_session_id"] = outcome.SessionID
		}
	}
	if outcome.Summary != "" {
		packet["session_resume_summary"] = outcome.Summary
	}
	if outcome.Label != "" {
		packet["session_resume_label"] = outcome.Label
	}
}

// ensureSessionContinuityAfterBound 从已绑定 attempt 的 packet 恢复 outcome 并补写事件。
func (s *ProjectStore) ensureSessionContinuityAfterBound(ctx context.Context, input DispatchProjectTaskInput, task project.ProjectTask) error {
	if task.CurrentAttemptID == nil || *task.CurrentAttemptID == uuid.Nil {
		return nil
	}
	attempt, err := s.repository.GetProjectTaskAttempt(ctx, input.TenantID, *task.CurrentAttemptID)
	if err != nil {
		return err
	}
	return s.ensureSessionContinuityEvent(ctx, input, task, attempt.ID, sessionResumeOutcomeFromPacket(attempt.ExecutionContextPacket))
}

// ensureSessionContinuityEvent 在 bind 成功后写入卷宗 continuity 事件（幂等键=attempt id）。
func (s *ProjectStore) ensureSessionContinuityEvent(
	ctx context.Context,
	input DispatchProjectTaskInput,
	task project.ProjectTask,
	attemptID uuid.UUID,
	outcome sessionResumeOutcome,
) error {
	if s.repository == nil || !outcome.shouldEmit() || attemptID == uuid.Nil {
		return nil
	}
	exists, err := s.repository.ProjectTaskEventExists(ctx, input.TenantID, input.ProjectID, project.ProjectEventTaskSessionContinuity, attemptID.String())
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	summary := outcome.Summary
	if summary == "" {
		switch {
		case outcome.Status == "resumed":
			summary = "已接上该员工上次会话继续执行"
		case outcome.SkipReason == "session_stale":
			summary = "原会话超过7天未活跃，已主动开新会话（未沿用旧会话）"
		case outcome.SkipReason == "session_node_mismatch":
			summary = "原会话在其他运行节点，已主动开新会话"
		default:
			summary = "已主动开新会话（未沿用旧会话）"
		}
	}
	demandID := ""
	if task.DemandID != nil {
		demandID = task.DemandID.String()
	}
	employeeID := ""
	if task.AssignedDigitalEmployeeID != nil {
		employeeID = task.AssignedDigitalEmployeeID.String()
	}
	payload := map[string]any{
		"project_task_id":         task.ID.String(),
		"project_task_attempt_id": attemptID.String(),
		"digital_employee_id":     employeeID,
		"demand_id":               demandID,
		"session_resume_status":   outcome.Status,
	}
	if outcome.SkipReason != "" {
		payload["session_resume_skip_reason"] = outcome.SkipReason
	}
	if outcome.SessionID != "" {
		payload["provider_session_id"] = outcome.SessionID
	}
	if task.PlannerMetadata != nil {
		if root, ok := task.PlannerMetadata["revision_root_task_id"].(string); ok && strings.TrimSpace(root) != "" {
			payload["revision_root_task_id"] = strings.TrimSpace(root)
		}
	}
	_, err = s.repository.AppendProjectEvent(ctx, project.AppendProjectEventRequest{
		TenantID:     input.TenantID,
		ProjectID:    input.ProjectID,
		EventType:    project.ProjectEventTaskSessionContinuity,
		ActorType:    "project_coordinator",
		ActorID:      attemptID.String(), // 幂等按 attempt，勿用 task id（spec §5.3）
		ResourceType: stringPtr("project_task"),
		ResourceID:   stringPtr(task.ID.String()),
		Summary:      summary,
		Payload:      payload,
	})
	return err
}
