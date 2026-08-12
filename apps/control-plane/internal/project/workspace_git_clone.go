package project

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
)

const workspaceCloneAttemptPayloadKey = "provision_attempt_id"

// ProjectWorkspaceReceiptLister lists project_workspace command receipts so
// clone writeback can decide when a provision attempt has fully failed.
type ProjectWorkspaceReceiptLister interface {
	ListProjectWorkspaceReceipts(ctx context.Context, tenantID, projectID uuid.UUID, commandType string, limit int32) ([]WorkspaceCommandReceiptSummary, error)
}

type WorkspaceCommandReceiptSummary struct {
	CommandID     string
	CommandType   string
	RuntimeNodeID uuid.UUID
	Status        string
	Payload       map[string]any
	ErrorMessage  *string
}

func (s *Service) SetProjectWorkspaceReceiptLister(lister ProjectWorkspaceReceiptLister) {
	if s != nil {
		s.workspaceReceipts = lister
	}
}

// enqueueProjectGitClone fans out async clone_project_repository to bound nodes.
// Create path keeps project pending until at least one clone succeeds.
func (s *Service) enqueueProjectGitClone(ctx context.Context, tenantID, projectID uuid.UUID, projectName string, runtimeNodeIDs []uuid.UUID) error {
	return s.dispatchProjectGitClones(ctx, tenantID, projectID, projectName, runtimeNodeIDs, false)
}

// cloneProjectRepositoryOnNodeSync waits for clone_project_repository to finish
// on one node. Used by admin provision confirm so provisioned ⇒ disk ready.
func (s *Service) cloneProjectRepositoryOnNodeSync(
	ctx context.Context,
	tenantID, projectID uuid.UUID,
	projectName string,
	runtimeNodeID uuid.UUID,
	force bool,
) error {
	if s.workspaceCommander == nil {
		return fmt.Errorf("%w: workspace commander not configured", ErrProjectWorkspaceProvision)
	}
	project, err := s.repository.GetProject(ctx, tenantID, projectID)
	if err != nil {
		return err
	}
	if project.RepoBinding.Status != ProjectRepoBindingStatusBound {
		return fmt.Errorf("%w: project has no bound repository", ErrInvalidProject)
	}
	repoURL := strings.TrimSpace(project.RepoBinding.URL)
	if repoURL == "" {
		return fmt.Errorf("%w: bound project missing repo_url", ErrInvalidProject)
	}
	payload := map[string]any{
		"project_id":                     projectID.String(),
		"project_name":                   projectName,
		"repo_url":                       repoURL,
		workspaceCloneAttemptPayloadKey: uuid.NewString(),
		"force":                          force,
	}
	if branch := strings.TrimSpace(project.RepoBinding.DefaultBranch); branch != "" {
		payload["default_branch"] = branch
	}
	// Longer timeout than mkdir: clone can pull large repos.
	return s.dispatchProjectWorkspaceCommand(
		ctx,
		tenantID,
		projectID,
		runtimeNodeID,
		runtimeCommandCloneProjectRepository,
		payload,
		defaultWorkspaceProvisionTimeout*4,
		true,
	)
}

func (s *Service) dispatchProjectGitClones(
	ctx context.Context,
	tenantID, projectID uuid.UUID,
	projectName string,
	runtimeNodeIDs []uuid.UUID,
	force bool,
) error {
	if s.workspaceCommander == nil {
		slog.Default().Warn("skip project git clone: workspace commander not configured",
			"project_id", projectID.String(),
		)
		return nil
	}
	if len(runtimeNodeIDs) == 0 {
		return nil
	}
	project, err := s.repository.GetProject(ctx, tenantID, projectID)
	if err != nil {
		return err
	}
	if project.RepoBinding.Status != ProjectRepoBindingStatusBound {
		return nil
	}
	repoURL := strings.TrimSpace(project.RepoBinding.URL)
	if repoURL == "" {
		return fmt.Errorf("%w: bound project missing repo_url", ErrInvalidProject)
	}
	attemptID := uuid.NewString()
	var defaultBranch *string
	if branch := strings.TrimSpace(project.RepoBinding.DefaultBranch); branch != "" {
		defaultBranch = &branch
	}

	var firstErr error
	dispatched := 0
	for _, runtimeNodeID := range runtimeNodeIDs {
		payload := map[string]any{
			"project_id":                     projectID.String(),
			"project_name":                   projectName,
			"repo_url":                       repoURL,
			workspaceCloneAttemptPayloadKey: attemptID,
			"force":                          force,
		}
		if defaultBranch != nil {
			payload["default_branch"] = *defaultBranch
		}
		if err := s.dispatchProjectWorkspaceCommand(
			ctx,
			tenantID,
			projectID,
			runtimeNodeID,
			runtimeCommandCloneProjectRepository,
			payload,
			defaultWorkspaceProvisionTimeout,
			false,
		); err != nil {
			slog.Default().Error("dispatch project git clone failed",
				"project_id", projectID.String(),
				"runtime_node_id", runtimeNodeID.String(),
				"error", err.Error(),
			)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		dispatched++
	}
	if dispatched == 0 {
		if firstErr != nil {
			msg := firstErr.Error()
			if _, setErr := s.repository.SetProjectWorkspaceReady(ctx, tenantID, projectID, WorkspaceReadyStatusError, project.PrimaryRuntimeNodeID, &msg); setErr != nil {
				slog.Default().Error("set workspace ready=error after clone dispatch failure",
					"project_id", projectID.String(),
					"error", setErr.Error(),
				)
			}
			return firstErr
		}
		msg := "no runtime nodes available for git clone"
		if _, setErr := s.repository.SetProjectWorkspaceReady(ctx, tenantID, projectID, WorkspaceReadyStatusError, project.PrimaryRuntimeNodeID, &msg); setErr != nil {
			slog.Default().Error("set workspace ready=error when no clone targets",
				"project_id", projectID.String(),
				"error", setErr.Error(),
			)
		}
		return fmt.Errorf("%w: %s", ErrProjectWorkspaceProvision, msg)
	}
	return nil
}

// HandleCloneCommandTerminal applies async clone writeback to project readiness.
// Success on a pending project → ready + primary. Failure only flips to error when
// the whole provision attempt has no remaining pending clones and no successes.
// Already-ready projects are never downgraded by a later node clone failure.
func (s *Service) HandleCloneCommandTerminal(ctx context.Context, req CloneCommandTerminal) error {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.RuntimeNodeID == uuid.Nil {
		return nil
	}
	project, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return err
	}
	attemptID := payloadString(req.Payload, workspaceCloneAttemptPayloadKey)

	if req.Success {
		switch project.WorkspaceReadyStatus {
		case WorkspaceReadyStatusReady:
			if project.PrimaryRuntimeNodeID == nil || *project.PrimaryRuntimeNodeID == uuid.Nil {
				primary := req.RuntimeNodeID
				_, err = s.repository.SetProjectWorkspaceReady(ctx, req.TenantID, req.ProjectID, WorkspaceReadyStatusReady, &primary, nil)
				return ignoreWorkspaceReadyCASConflict(err)
			}
			return nil
		case WorkspaceReadyStatusPending, WorkspaceReadyStatusError, "":
			// 丢弃过期 attempt 的成功回写:若仍有其它 attempt 的 clone 在飞,说明已 reclone。
			if attemptID != "" && s.workspaceReceipts != nil {
				stale, staleErr := s.cloneSuccessIsStale(ctx, req.TenantID, req.ProjectID, attemptID)
				if staleErr != nil {
					slog.Default().Warn("list receipts for stale clone success check failed; applying success anyway",
						"project_id", req.ProjectID.String(),
						"error", staleErr.Error(),
					)
				} else if stale {
					slog.Default().Info("ignore stale git clone success from older provision attempt",
						"project_id", req.ProjectID.String(),
						"provision_attempt_id", attemptID,
						"runtime_node_id", req.RuntimeNodeID.String(),
					)
					return nil
				}
			}
			primary := req.RuntimeNodeID
			updated, setErr := s.repository.SetProjectWorkspaceReady(ctx, req.TenantID, req.ProjectID, WorkspaceReadyStatusReady, &primary, nil)
			if setErr != nil {
				return ignoreWorkspaceReadyCASConflict(setErr)
			}
			// The clone landed: this node's disk is now a usable workspace, which
			// is exactly what `provisioned` means (spec §5.2). Create/reclone both
			// arrive here, so this is the single place Git supply is confirmed.
			if _, markErr := s.repository.MarkProjectRuntimeNodeProvisioned(ctx, req.TenantID, req.ProjectID, req.RuntimeNodeID, "clone"); markErr != nil {
				slog.Default().Warn("mark node provisioned after clone success failed",
					"project_id", req.ProjectID.String(),
					"runtime_node_id", req.RuntimeNodeID.String(),
					"error", markErr.Error(),
				)
			}
			_, _ = s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
				TenantID:  req.TenantID,
				ProjectID: req.ProjectID,
				EventType: ProjectEventWorkspaceReady,
				ActorType: "system",
				ActorID:   "runtime",
				Summary:   "项目工作区 Git clone 已就绪",
				Payload: map[string]any{
					"primary_runtime_node_id": primary.String(),
					"provision_attempt_id":    attemptID,
					"workspace_ready_status":  string(updated.WorkspaceReadyStatus),
				},
			})
			return nil
		default:
			return nil
		}
	}

	// Failure path.
	if project.WorkspaceReadyStatus == WorkspaceReadyStatusReady {
		slog.Default().Warn("project git clone failed on node after project already ready",
			"project_id", req.ProjectID.String(),
			"runtime_node_id", req.RuntimeNodeID.String(),
			"error", strings.TrimSpace(req.ErrorMessage),
		)
		return nil
	}
	if project.WorkspaceReadyStatus != WorkspaceReadyStatusPending && project.WorkspaceReadyStatus != "" {
		return nil
	}

	// 无 attempt / 无法列 receipt 时不落 error,避免误杀仍在飞的兄弟节点或历史成功 receipt。
	if attemptID == "" {
		slog.Default().Warn("clone failure missing provision_attempt_id; not marking project error",
			"project_id", req.ProjectID.String(),
			"runtime_node_id", req.RuntimeNodeID.String(),
		)
		return nil
	}
	if s.workspaceReceipts == nil {
		slog.Default().Warn("project workspace receipt lister not configured; not marking clone failure terminal",
			"project_id", req.ProjectID.String(),
		)
		return nil
	}
	receipts, listErr := s.workspaceReceipts.ListProjectWorkspaceReceipts(ctx, req.TenantID, req.ProjectID, runtimeCommandCloneProjectRepository, 500)
	if listErr != nil {
		slog.Default().Warn("list project git clone receipts failed; not marking project error yet",
			"project_id", req.ProjectID.String(),
			"error", listErr.Error(),
		)
		return nil
	}
	pending := 0
	completed := 0
	for _, receipt := range receipts {
		if req.CommandID != "" && receipt.CommandID == req.CommandID {
			continue
		}
		if payloadString(receipt.Payload, workspaceCloneAttemptPayloadKey) != attemptID {
			continue
		}
		switch receipt.Status {
		case "pending", "running", "dispatched":
			pending++
		case "completed":
			completed++
		}
	}
	if pending > 0 || completed > 0 {
		return nil
	}
	msg := strings.TrimSpace(req.ErrorMessage)
	if msg == "" {
		msg = "project git clone failed on all runtime nodes"
	}
	updated, setErr := s.repository.SetProjectWorkspaceReady(ctx, req.TenantID, req.ProjectID, WorkspaceReadyStatusError, project.PrimaryRuntimeNodeID, &msg)
	if setErr != nil {
		return ignoreWorkspaceReadyCASConflict(setErr)
	}
	_, _ = s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  req.TenantID,
		ProjectID: req.ProjectID,
		EventType: ProjectEventWorkspaceError,
		ActorType: "system",
		ActorID:   "runtime",
		Summary:   "项目工作区 Git clone 失败",
		Payload: map[string]any{
			"error":                  msg,
			"provision_attempt_id":   attemptID,
			"workspace_ready_status": string(updated.WorkspaceReadyStatus),
		},
	})
	return nil
}

func (s *Service) cloneSuccessIsStale(ctx context.Context, tenantID, projectID uuid.UUID, attemptID string) (bool, error) {
	receipts, err := s.workspaceReceipts.ListProjectWorkspaceReceipts(ctx, tenantID, projectID, runtimeCommandCloneProjectRepository, 500)
	if err != nil {
		return false, err
	}
	for _, receipt := range receipts {
		otherAttempt := payloadString(receipt.Payload, workspaceCloneAttemptPayloadKey)
		if otherAttempt == "" || otherAttempt == attemptID {
			continue
		}
		switch receipt.Status {
		case "pending", "running", "dispatched":
			return true, nil
		}
	}
	return false, nil
}

func ignoreWorkspaceReadyCASConflict(err error) error {
	if err == nil {
		return nil
	}
	// CAS miss (另一节点已推进状态) → 视为成功无操作。
	if err == ErrProjectNotFound || strings.Contains(err.Error(), "no rows") {
		return nil
	}
	return err
}

type CloneCommandTerminal struct {
	TenantID      uuid.UUID
	ProjectID     uuid.UUID
	RuntimeNodeID uuid.UUID
	CommandID     string
	Success       bool
	ErrorMessage  string
	Payload       map[string]any
}

// RecloneProjectWorkspace re-enters git provision from error/pending (force clone).
func (s *Service) RecloneProjectWorkspace(ctx context.Context, req WorkspaceManualActionRequest) (*Project, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ActorUserID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	project, err := s.requireActiveProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	// attached projects never force-delete + reclone (spec 2026-08-12 §5.5 / P2).
	if project.WorkspaceOwnership == WorkspaceOwnershipAttached {
		return nil, fmt.Errorf("%w: 认领目录项目禁止重新 clone（平台不会 force 清空该目录）", ErrInvalidProject)
	}
	if project.RepoBinding.Status != ProjectRepoBindingStatusBound {
		return nil, fmt.Errorf("%w: project has no bound repository", ErrInvalidProject)
	}
	switch project.WorkspaceReadyStatus {
	case WorkspaceReadyStatusError, WorkspaceReadyStatusPending:
	default:
		return nil, fmt.Errorf("%w: reclone only allowed from error or pending (status=%s)", ErrInvalidProject, project.WorkspaceReadyStatus)
	}
	nodes, err := s.repository.ListProjectRuntimeNodes(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, ErrProjectRuntimeNodesRequired
	}
	// Only reclone already-provisioned nodes (unprovisioned have no disk to replace).
	runtimeNodeIDs := make([]uuid.UUID, 0, len(nodes))
	for _, node := range nodes {
		if node.IsProvisioned() {
			runtimeNodeIDs = append(runtimeNodeIDs, node.RuntimeNodeID)
		}
	}
	if len(runtimeNodeIDs) == 0 {
		return nil, fmt.Errorf("%w: no provisioned runtime node to reclone", ErrInvalidProject)
	}
	updated, err := s.repository.SetProjectWorkspaceReady(ctx, req.TenantID, req.ProjectID, WorkspaceReadyStatusPending, project.PrimaryRuntimeNodeID, nil)
	if err != nil {
		return nil, err
	}
	if _, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  req.TenantID,
		ProjectID: req.ProjectID,
		EventType: ProjectEventWorkspaceRecloneRequested,
		ActorType: "human_user",
		ActorID:   req.ActorUserID.String(),
		Summary:   "人工触发项目工作区重新 clone",
		Payload: map[string]any{
			"reason":                 strings.TrimSpace(req.Reason),
			"workspace_ready_status": string(updated.WorkspaceReadyStatus),
		},
	}); err != nil {
		return nil, err
	}
	if err := s.dispatchProjectGitClones(ctx, req.TenantID, req.ProjectID, updated.WorkspaceDirectoryName(), runtimeNodeIDs, true); err != nil {
		slog.Default().Warn("reclone dispatch incomplete",
			"project_id", req.ProjectID.String(),
			"error", err.Error(),
		)
	}
	latest, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return &updated, nil
	}
	return &latest, nil
}

// MarkProjectWorkspaceReadyManually trusts operator that disk is fixed.
func (s *Service) MarkProjectWorkspaceReadyManually(ctx context.Context, req WorkspaceManualActionRequest) (*Project, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ActorUserID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	project, err := s.requireActiveProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	switch project.WorkspaceReadyStatus {
	case WorkspaceReadyStatusReady:
		return &project, nil
	case WorkspaceReadyStatusPending, WorkspaceReadyStatusError:
	default:
		return nil, fmt.Errorf("%w: cannot mark ready from status %s", ErrInvalidProject, project.WorkspaceReadyStatus)
	}
	primary := project.PrimaryRuntimeNodeID
	if primary == nil || *primary == uuid.Nil {
		nodes, listErr := s.repository.ListProjectRuntimeNodes(ctx, req.TenantID, req.ProjectID)
		if listErr != nil {
			return nil, listErr
		}
		if len(nodes) > 0 {
			id := nodes[0].RuntimeNodeID
			primary = &id
		}
	}
	updated, err := s.repository.SetProjectWorkspaceReady(ctx, req.TenantID, req.ProjectID, WorkspaceReadyStatusReady, primary, nil)
	if err != nil {
		return nil, err
	}
	if _, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  req.TenantID,
		ProjectID: req.ProjectID,
		EventType: ProjectEventWorkspaceMarkedReady,
		ActorType: "human_user",
		ActorID:   req.ActorUserID.String(),
		Summary:   "人工标记项目工作区已就绪",
		Payload: map[string]any{
			"reason":                 strings.TrimSpace(req.Reason),
			"workspace_ready_status": string(updated.WorkspaceReadyStatus),
			"primary_runtime_node_id": func() string {
				if primary == nil {
					return ""
				}
				return primary.String()
			}(),
		},
	}); err != nil {
		return nil, err
	}
	return &updated, nil
}

type WorkspaceManualActionRequest struct {
	TenantID    uuid.UUID
	ProjectID   uuid.UUID
	ActorUserID uuid.UUID
	Reason      string
}

func payloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	raw, ok := payload[key]
	if !ok || raw == nil {
		return ""
	}
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}
