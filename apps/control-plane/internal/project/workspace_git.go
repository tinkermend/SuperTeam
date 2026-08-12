package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/employee"
	"github.com/superteam/control-plane/internal/platform"
	runtimepkg "github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/systemconfig"
)

const (
	workspaceGitSampleBatchLimit     = 50
	workspaceGitIdleSampleMultiplier = 6
	workspaceGitInflightStale        = 2 * time.Minute
	workspaceGitRefreshWait          = 8 * time.Second
)

type workspaceGitProbeWriteback struct {
	TenantID      uuid.UUID
	ProjectID     uuid.UUID
	RuntimeNodeID uuid.UUID
	NodeID        string
	CommandID     string
	Success       bool
	ErrorMessage  string
	Result        map[string]any
	SampledAt     time.Time
}

func (s *Service) SetWorkspaceGitStore(store workspaceGitStore) {
	if s != nil {
		s.workspaceGit = store
	}
}

func (s *Service) attachWorkspaceGitStatus(ctx context.Context, project *Project) {
	if s == nil || s.workspaceGit == nil || project == nil || project.ID == uuid.Nil {
		return
	}
	status, err := s.workspaceGit.GetProjectWorkspaceGitSnapshot(ctx, project.TenantID, project.ID)
	if err != nil {
		slog.WarnContext(ctx, "workspace git snapshot attach failed",
			"project_id", project.ID.String(),
			"error", err,
		)
		return
	}
	project.WorkspaceGit = status
}

func (s *Service) attachWorkspaceGitStatusMany(ctx context.Context, tenantID uuid.UUID, projects []Project) {
	if s == nil || s.workspaceGit == nil || tenantID == uuid.Nil || len(projects) == 0 {
		return
	}
	ids := make([]uuid.UUID, 0, len(projects))
	for _, project := range projects {
		if project.ID != uuid.Nil {
			ids = append(ids, project.ID)
		}
	}
	snapshots, err := s.workspaceGit.ListProjectWorkspaceGitSnapshots(ctx, tenantID, ids)
	if err != nil {
		slog.WarnContext(ctx, "workspace git snapshot list attach failed",
			"tenant_id", tenantID.String(),
			"error", err,
		)
		return
	}
	for i := range projects {
		if status, ok := snapshots[projects[i].ID]; ok {
			projects[i].WorkspaceGit = status
		}
	}
}

func (s *Service) attachWorkspaceGitStatusPortfolio(ctx context.Context, tenantID uuid.UUID, items []ProjectPortfolioItem) {
	if s == nil || s.workspaceGit == nil || tenantID == uuid.Nil || len(items) == 0 {
		return
	}
	ids := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
		if item.Project.ID != uuid.Nil {
			ids = append(ids, item.Project.ID)
		}
	}
	snapshots, err := s.workspaceGit.ListProjectWorkspaceGitSnapshots(ctx, tenantID, ids)
	if err != nil {
		slog.WarnContext(ctx, "workspace git snapshot portfolio attach failed",
			"tenant_id", tenantID.String(),
			"error", err,
		)
		return
	}
	for i := range items {
		if status, ok := snapshots[items[i].Project.ID]; ok {
			items[i].WorkspaceGit = leanWorkspaceGitStatus(status)
		}
	}
}

func leanWorkspaceGitStatus(status *ProjectWorkspaceGitStatus) *ProjectWorkspaceGitStatus {
	if status == nil {
		return nil
	}
	lean := *status
	lean.UncommittedEntries = nil
	return &lean
}

// RefreshProjectWorkspaceGitStatus 手动刷新：同一探测链路，绕过 sampled_at，异步命令通道。
// 同项目在飞节流；节点离线时标未采到并保留上次快照。项目读权限即可。
func (s *Service) RefreshProjectWorkspaceGitStatus(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID) (*Project, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	project, err := s.repository.GetProject(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	s.attachWorkspaceGitStatus(ctx, &project)
	if s.workspaceGit == nil || s.workspaceCommander == nil {
		return &project, nil
	}
	if project.PrimaryRuntimeNodeID == nil || *project.PrimaryRuntimeNodeID == uuid.Nil {
		_ = s.workspaceGit.MarkProjectWorkspaceGitSnapshotFailed(ctx, tenantID, projectID, "未绑定主节点", time.Now().UTC())
		s.attachWorkspaceGitStatus(ctx, &project)
		return &project, nil
	}
	if current := project.WorkspaceGit; current != nil && current.RefreshPending {
		if current.InflightAt != nil && time.Since(current.InflightAt.UTC()) < workspaceGitInflightStale {
			return &project, nil
		}
	}
	node, err := s.workspaceCommander.GetNodeByID(ctx, *project.PrimaryRuntimeNodeID)
	if err != nil {
		_ = s.markWorkspaceGitSampleFailed(ctx, tenantID, projectID, fmt.Sprintf("解析主节点失败: %v", err))
		s.attachWorkspaceGitStatus(ctx, &project)
		return &project, nil
	}
	nodeID := strings.TrimSpace(node.NodeID)
	if nodeID == "" || !s.workspaceCommander.IsConnected(nodeID) {
		_ = s.markWorkspaceGitSampleFailed(ctx, tenantID, projectID, workspaceGitOfflineSampleError(project.WorkspaceGit))
		s.attachWorkspaceGitStatus(ctx, &project)
		return &project, nil
	}
	if _, err := s.dispatchWorkspaceGitProbe(ctx, workspaceGitProbeRequest{
		TenantID:       tenantID,
		ProjectID:      projectID,
		DirectoryName:  project.WorkspaceDirectoryName(),
		RuntimeNodeID:  *project.PrimaryRuntimeNodeID,
		PersistReceipt: true,
		Wait:           true,
		Timeout:        workspaceGitRefreshWait,
	}); err != nil {
		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			_ = s.markWorkspaceGitSampleFailed(ctx, tenantID, projectID, err.Error())
		}
	}
	_ = actorUserID
	s.attachWorkspaceGitStatus(ctx, &project)
	return &project, nil
}

func workspaceGitOfflineSampleError(previous *ProjectWorkspaceGitStatus) string {
	if previous != nil && previous.SampledAt != nil && !previous.SampledAt.IsZero() {
		minutes := int(time.Since(previous.SampledAt.UTC()).Minutes())
		if minutes < 1 {
			minutes = 1
		}
		return fmt.Sprintf("节点离线，显示的是 %d 分钟前的现场", minutes)
	}
	return "节点离线，尚未采到现场"
}

func (s *Service) markWorkspaceGitSampleFailed(ctx context.Context, tenantID, projectID uuid.UUID, sampleError string) error {
	if s == nil || s.workspaceGit == nil {
		return nil
	}
	return s.workspaceGit.MarkProjectWorkspaceGitSnapshotFailed(ctx, tenantID, projectID, strings.TrimSpace(sampleError), time.Now().UTC())
}

func projectTaskStatusEligibleForWorkspaceGitSample(status string) bool {
	switch status {
	case ProjectTaskStatusCompleted, ProjectTaskStatusFailed, ProjectTaskStatusCancelled:
		return true
	default:
		return false
	}
}

func (s *Service) maybeSampleWorkspaceGitOnTaskTerminal(ctx context.Context, tenantID, projectID uuid.UUID, task ProjectTask) {
	if s == nil || s.workspaceGit == nil || s.workspaceCommander == nil {
		return
	}
	if !projectTaskStatusEligibleForWorkspaceGitSample(task.Status) {
		reloaded, err := s.repository.GetProjectTask(ctx, tenantID, task.ID)
		if err != nil {
			return
		}
		task = reloaded
		if !projectTaskStatusEligibleForWorkspaceGitSample(task.Status) {
			return
		}
	}
	project, err := s.repository.GetProject(ctx, tenantID, projectID)
	if err != nil {
		slog.WarnContext(ctx, "workspace git terminal sample skipped: load project",
			"project_id", projectID.String(),
			"error", err,
		)
		return
	}
	if project.PrimaryRuntimeNodeID == nil || *project.PrimaryRuntimeNodeID == uuid.Nil {
		return
	}
	if _, err := s.dispatchWorkspaceGitProbe(ctx, workspaceGitProbeRequest{
		TenantID:       tenantID,
		ProjectID:      projectID,
		DirectoryName:  project.WorkspaceDirectoryName(),
		RuntimeNodeID:  *project.PrimaryRuntimeNodeID,
		PersistReceipt: true,
		Wait:           false,
	}); err != nil {
		slog.WarnContext(ctx, "workspace git terminal sample dispatch failed",
			"project_id", projectID.String(),
			"error", err,
		)
	}
}

// SweepWorkspaceGitSamples 看门狗一轮：已就绪 + 主节点在线 + sampled_at 早于阈值，取前 N 个。
// 定时路径不落 command receipt。失败只记日志，下轮重试。
func (s *Service) SweepWorkspaceGitSamples(ctx context.Context, now time.Time, limit int32) (int, error) {
	if s == nil || s.workspaceGit == nil || s.workspaceCommander == nil {
		return 0, nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	interval := s.workspaceGitSampleInterval(ctx)
	staleBefore := now.Add(-interval)
	idleStaleBefore := now.Add(-interval * workspaceGitIdleSampleMultiplier)
	inflightStaleBefore := now.Add(-workspaceGitInflightStale)
	due, err := s.workspaceGit.ListProjectsDueForWorkspaceGitSample(ctx, staleBefore, idleStaleBefore, inflightStaleBefore, limit)
	if err != nil {
		return 0, err
	}
	dispatched := 0
	for _, item := range due {
		if ctx.Err() != nil {
			return dispatched, ctx.Err()
		}
		node, err := s.workspaceCommander.GetNodeByID(ctx, item.PrimaryRuntimeNodeID)
		if err != nil {
			slog.WarnContext(ctx, "workspace git sample skip: resolve node",
				"project_id", item.ID.String(),
				"error", err,
			)
			continue
		}
		nodeID := strings.TrimSpace(node.NodeID)
		if nodeID == "" || !s.workspaceCommander.IsConnected(nodeID) {
			_ = s.markWorkspaceGitSampleFailed(ctx, item.TenantID, item.ID, workspaceGitOfflineSampleError(&ProjectWorkspaceGitStatus{
				SampledAt: item.SampledAt,
			}))
			continue
		}
		if _, err := s.dispatchWorkspaceGitProbe(ctx, workspaceGitProbeRequest{
			TenantID:       item.TenantID,
			ProjectID:      item.ID,
			DirectoryName:  item.DirectoryName,
			RuntimeNodeID:  item.PrimaryRuntimeNodeID,
			PersistReceipt: false,
			Wait:           false,
		}); err != nil {
			slog.WarnContext(ctx, "workspace git sample dispatch failed",
				"project_id", item.ID.String(),
				"error", err,
			)
			continue
		}
		dispatched++
	}
	return dispatched, nil
}

func (s *Service) workspaceGitSampleInterval(ctx context.Context) time.Duration {
	if s.systemConfig != nil {
		interval := s.systemConfig.Duration(ctx, platform.DefaultTenantID, systemconfig.KeyProjectWorkspaceGitSampleIntervalSeconds)
		if interval > 0 {
			return interval
		}
	}
	return systemconfig.DefaultDurationFor(systemconfig.KeyProjectWorkspaceGitSampleIntervalSeconds)
}

type workspaceGitProbeRequest struct {
	TenantID       uuid.UUID
	ProjectID      uuid.UUID
	DirectoryName  string
	RuntimeNodeID  uuid.UUID
	PersistReceipt bool
	Wait           bool
	Timeout        time.Duration
}

func (s *Service) dispatchWorkspaceGitProbe(ctx context.Context, req workspaceGitProbeRequest) (string, error) {
	if s.workspaceCommander == nil {
		return "", fmt.Errorf("%w: workspace commander not configured", ErrProjectWorkspaceProvision)
	}
	directoryName := strings.TrimSpace(req.DirectoryName)
	if err := ValidateProjectDirectoryName(directoryName); err != nil {
		return "", err
	}
	node, err := s.workspaceCommander.GetNodeByID(ctx, req.RuntimeNodeID)
	if err != nil {
		return "", fmt.Errorf("%w: resolve runtime node %s: %v", ErrProjectWorkspaceProvision, req.RuntimeNodeID, err)
	}
	if node.TenantID != req.TenantID {
		return "", fmt.Errorf("%w: runtime node tenant mismatch", ErrProjectWorkspaceProvision)
	}
	nodeID := strings.TrimSpace(node.NodeID)
	if nodeID == "" {
		return "", fmt.Errorf("%w: runtime node %s has empty node_id", ErrProjectWorkspaceProvision, req.RuntimeNodeID)
	}
	if !s.workspaceCommander.IsConnected(nodeID) {
		return "", fmt.Errorf("%w: runtime node %s (%s) is not connected", ErrProjectWorkspaceProvision, node.Name, nodeID)
	}
	commandID := uuid.NewString()
	payload := map[string]any{
		"project_name": directoryName,
		"project_id":   req.ProjectID.String(),
	}
	now := time.Now().UTC()
	if req.PersistReceipt {
		if err := s.workspaceCommander.CreateCommandReceipt(ctx, WorkspaceCommandReceiptRequest{
			TenantID:      req.TenantID,
			CommandID:     commandID,
			CommandType:   runtimeCommandProbeProjectDirectory,
			RuntimeNodeID: req.RuntimeNodeID,
			NodeID:        nodeID,
			ResourceType:  projectWorkspaceResourceType,
			ResourceID:    req.ProjectID,
			Status:        "pending",
			Payload:       payload,
			DispatchedAt:  &now,
		}); err != nil {
			return "", fmt.Errorf("%w: create probe receipt: %v", ErrProjectWorkspaceProvision, err)
		}
	}
	if s.workspaceGit != nil {
		if err := s.workspaceGit.MarkProjectWorkspaceGitProbeInflight(ctx, req.TenantID, req.ProjectID, commandID, now); err != nil {
			slog.WarnContext(ctx, "workspace git mark inflight failed",
				"project_id", req.ProjectID.String(),
				"error", err,
			)
		}
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("%w: encode probe payload: %v", ErrProjectWorkspaceProvision, err)
	}
	if err := s.workspaceCommander.Dispatch(ctx, nodeID, runtimepkg.RuntimeCommand{
		ID:      commandID,
		Type:    runtimeCommandProbeProjectDirectory,
		Payload: rawPayload,
	}); err != nil {
		return "", fmt.Errorf("%w: dispatch probe to %s: %v", ErrProjectWorkspaceProvision, nodeID, err)
	}
	if !req.Wait {
		return commandID, nil
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = workspaceGitRefreshWait
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := s.waitProjectWorkspaceCommand(waitCtx, req.TenantID, commandID, runtimeCommandProbeProjectDirectory, nodeID); err != nil {
		return commandID, err
	}
	return commandID, nil
}

func (s *Service) ApplyWorkspaceGitProbeWriteback(ctx context.Context, req workspaceGitProbeWriteback) error {
	if s == nil || s.workspaceGit == nil {
		return nil
	}
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil {
		return nil
	}
	sampledAt := req.SampledAt
	if sampledAt.IsZero() {
		sampledAt = time.Now().UTC()
	}
	if !req.Success {
		msg := strings.TrimSpace(req.ErrorMessage)
		if msg == "" {
			msg = "探测失败"
		}
		return s.workspaceGit.MarkProjectWorkspaceGitSnapshotFailed(ctx, req.TenantID, req.ProjectID, msg, sampledAt)
	}
	status, err := workspaceGitStatusFromProbeResult(req.Result, sampledAt, req.RuntimeNodeID, req.NodeID)
	if err != nil {
		return s.workspaceGit.MarkProjectWorkspaceGitSnapshotFailed(ctx, req.TenantID, req.ProjectID, err.Error(), sampledAt)
	}
	return s.workspaceGit.UpsertProjectWorkspaceGitSnapshotSuccess(ctx, req.TenantID, req.ProjectID, status)
}

func workspaceGitStatusFromProbeResult(result map[string]any, sampledAt time.Time, runtimeNodeID uuid.UUID, nodeID string) (ProjectWorkspaceGitStatus, error) {
	facts := probeFactsFromResult(result)
	if facts == nil || !facts.Exists || !facts.IsDir {
		return ProjectWorkspaceGitStatus{}, fmt.Errorf("目录不存在或不是目录")
	}
	status := ProjectWorkspaceGitStatus{
		Applicable:           facts.IsGitRepo,
		IsGitRepo:            boolRef(facts.IsGitRepo),
		HeadCommit:           strings.TrimSpace(facts.HeadCommit),
		CurrentBranch:        strings.TrimSpace(facts.CurrentBranch),
		Detached:             facts.Detached,
		SampledAt:            timeRef(sampledAt.UTC()),
		SampledRuntimeNodeID: uuidRef(runtimeNodeID),
		SampledNodeID:        strings.TrimSpace(nodeID),
	}
	if !facts.IsGitRepo {
		status.IsClean = nil
		status.RepoState = ""
		return status, nil
	}
	clean := !facts.Dirty
	status.IsClean = &clean
	status.RepoState = ProjectWorkspaceGitRepoState(strings.TrimSpace(stringFromAny(result["repo_state"])))
	if status.RepoState == "" {
		if facts.Detached {
			status.RepoState = ProjectWorkspaceGitRepoStateDetached
		} else {
			status.RepoState = ProjectWorkspaceGitRepoStateOK
		}
	}
	status.UncommittedCount = intFromAny(result["uncommitted_count"])
	status.UncommittedTruncated, _ = boolFromAny(result["uncommitted_truncated"])
	status.UncommittedOmitted = intFromAny(result["uncommitted_omitted"])
	status.UncommittedEntries = uncommittedEntriesFromAny(result["uncommitted_entries"])
	if status.UncommittedCount == 0 && len(status.UncommittedEntries) > 0 {
		status.UncommittedCount = len(status.UncommittedEntries) + status.UncommittedOmitted
	}
	if facts.Dirty && status.UncommittedCount == 0 {
		status.UncommittedCount = 1
	}
	return status, nil
}

func uncommittedEntriesFromAny(value any) []ProjectWorkspaceGitFileEntry {
	switch typed := value.(type) {
	case []ProjectWorkspaceGitFileEntry:
		return typed
	case []any:
		out := make([]ProjectWorkspaceGitFileEntry, 0, len(typed))
		for _, item := range typed {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			path := strings.TrimSpace(stringFromAny(entry["path"]))
			if path == "" {
				continue
			}
			out = append(out, ProjectWorkspaceGitFileEntry{
				Path:     path,
				Category: ProjectWorkspaceGitFileCategory(strings.TrimSpace(stringFromAny(entry["category"]))),
			})
		}
		return out
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return nil
		}
		var out []ProjectWorkspaceGitFileEntry
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil
		}
		return out
	}
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		n, _ := typed.Int64()
		return int(n)
	default:
		return 0
	}
}

func boolRef(value bool) *bool { return &value }

func timeRef(value time.Time) *time.Time { return &value }

func uuidRef(value uuid.UUID) *uuid.UUID {
	if value == uuid.Nil {
		return nil
	}
	return &value
}

func workspaceGitProjectIDFromResult(result, payload map[string]any) uuid.UUID {
	for _, source := range []map[string]any{result, payload} {
		if source == nil {
			continue
		}
		raw := strings.TrimSpace(stringFromAny(source["project_id"]))
		if raw == "" {
			continue
		}
		id, err := uuid.Parse(raw)
		if err == nil && id != uuid.Nil {
			return id
		}
	}
	return uuid.Nil
}

func (a ProjectWorkspaceCloneWritebackAdapter) OnReceiptlessProbeTerminal(ctx context.Context, identity employee.RuntimeCommandWritebackIdentity, commandID string, terminal employee.RuntimeCommandTerminalWriteback, success bool) error {
	result := terminal.Result
	projectID := workspaceGitProjectIDFromResult(result, nil)
	if projectID == uuid.Nil {
		return nil
	}
	errMsg := ""
	if terminal.ErrorMessage != nil {
		errMsg = *terminal.ErrorMessage
	}
	return a.service.ApplyWorkspaceGitProbeWriteback(ctx, workspaceGitProbeWriteback{
		TenantID:      identity.TenantID,
		ProjectID:     projectID,
		RuntimeNodeID: identity.RuntimeNodeID,
		NodeID:        identity.NodeID,
		CommandID:     commandID,
		Success:       success,
		ErrorMessage:  errMsg,
		Result:        result,
		SampledAt:     time.Now().UTC(),
	})
}

func (a ProjectWorkspaceCloneWritebackAdapter) applyProbeReceipt(ctx context.Context, receipt employee.RuntimeCommandReceipt, success bool) error {
	if strings.TrimSpace(receipt.CommandType) != runtimeCommandProbeProjectDirectory {
		return nil
	}
	projectID := receipt.ResourceID
	if projectID == uuid.Nil {
		projectID = workspaceGitProjectIDFromResult(receipt.Result, receipt.Payload)
	}
	if projectID == uuid.Nil {
		return nil
	}
	errMsg := ""
	if receipt.ErrorMessage != nil {
		errMsg = *receipt.ErrorMessage
	}
	return a.service.ApplyWorkspaceGitProbeWriteback(ctx, workspaceGitProbeWriteback{
		TenantID:      receipt.TenantID,
		ProjectID:     projectID,
		RuntimeNodeID: receipt.RuntimeNodeID,
		NodeID:        receipt.NodeID,
		CommandID:     receipt.CommandID,
		Success:       success,
		ErrorMessage:  errMsg,
		Result:        receipt.Result,
		SampledAt:     time.Now().UTC(),
	})
}
