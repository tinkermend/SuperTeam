package project

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

// WorkspaceProvisionInboxItem is a package-local DTO for lazy provision todos.
// Defined here (not inbox.*) to avoid project↔inbox import cycles; app wires
// an adapter into inbox.Service.
type WorkspaceProvisionInboxItem struct {
	TenantID        uuid.UUID
	TargetUserID    uuid.UUID
	SourceID        uuid.UUID
	SourceProjectID uuid.UUID
	Title           string
	Summary         string
	DeepLink        map[string]any
	ContextPayload  map[string]any
}

// WorkspaceProvisionInbox is the narrow surface for lazy provision todos.
// Optional; nil skips inbox (tests without inbox wired).
type WorkspaceProvisionInbox interface {
	UpsertWorkspaceProvisionTodo(ctx context.Context, item WorkspaceProvisionInboxItem) error
}

// ProvisionWorkspaceOnNode is the admin-confirmed supply path for a bound but
// unprovisioned runtime node (spec 2026-08-12 §5.3 / P1).
// Non-git → mkdir; git → clone; attached (P2) → probe only.
func (s *Service) ProvisionWorkspaceOnNode(ctx context.Context, req ModifyProjectRuntimeNodeRequest) (*ProjectRuntimeNode, error) {
	req.Reason = strings.TrimSpace(req.Reason)
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.RuntimeNodeID == uuid.Nil || req.ActorUserID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	project, err := s.requireActiveProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	nodes, err := s.repository.ListProjectRuntimeNodes(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	var bound *ProjectRuntimeNode
	for i := range nodes {
		if nodes[i].RuntimeNodeID == req.RuntimeNodeID {
			bound = &nodes[i]
			break
		}
	}
	if bound == nil {
		return nil, fmt.Errorf("%w: runtime node is not bound to project", ErrProjectNotFound)
	}
	if bound.IsProvisioned() {
		return bound, nil // idempotent
	}

	dirName := project.WorkspaceDirectoryName()
	switch {
	case project.WorkspaceOwnership == WorkspaceOwnershipAttached:
		// attached: human must place the directory; platform only probes.
		if _, err := s.probeProjectDirectoryOnNode(ctx, req.TenantID, req.RuntimeNodeID, dirName); err != nil {
			return nil, err
		}
	case project.RepoBinding.Status == ProjectRepoBindingStatusBound:
		// Synchronous clone: do not mark provisioned until disk has a completed clone
		// (FATAL fix vs async create-path fan-out which uses wait=false).
		if err := s.cloneProjectRepositoryOnNodeSync(ctx, req.TenantID, req.ProjectID, dirName, req.RuntimeNodeID, false); err != nil {
			return nil, err
		}
	default:
		if err := s.ensureProjectDirectoriesOnNodes(ctx, req.TenantID, req.ProjectID, dirName, []uuid.UUID{req.RuntimeNodeID}); err != nil {
			return nil, err
		}
	}

	source := "confirm"
	if project.WorkspaceOwnership == WorkspaceOwnershipAttached {
		source = "attach_probe"
	}
	marked, err := s.repository.MarkProjectRuntimeNodeProvisioned(ctx, req.TenantID, req.ProjectID, req.RuntimeNodeID, source)
	if err != nil {
		return nil, err
	}
	if _, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  req.TenantID,
		ProjectID: req.ProjectID,
		EventType: ProjectEventRuntimePlacementUpdated,
		ActorType: "human_user",
		ActorID:   req.ActorUserID.String(),
		Summary:   "项目工作区已供给到节点",
		Payload: map[string]any{
			"runtime_node_id":  req.RuntimeNodeID.String(),
			"provision_status": string(ProvisionStatusProvisioned),
			"reason":           req.Reason,
		},
	}); err != nil {
		return nil, err
	}
	return &marked, nil
}

// maybeOpenWorkspaceProvisionTodos opens one inbox card per unprovisioned bound
// node when dispatch finds no workspace-ready candidate (lazy trigger, idempotent).
func (s *Service) maybeOpenWorkspaceProvisionTodos(ctx context.Context, project Project, actorHint uuid.UUID) {
	if s.workspaceProvisionInbox == nil {
		return
	}
	nodes, err := s.repository.ListProjectRuntimeNodes(ctx, project.TenantID, project.ID)
	if err != nil {
		slog.Default().Warn("workspace provision todo: list nodes failed", "project_id", project.ID.String(), "error", err.Error())
		return
	}
	target := project.HumanOwnerUserID
	if target == uuid.Nil {
		target = actorHint
	}
	if target == uuid.Nil {
		return
	}
	repoHint := ""
	if project.RepoBinding.Status == ProjectRepoBindingStatusBound {
		repoHint = strings.TrimSpace(project.RepoBinding.URL)
	}
	for _, node := range nodes {
		if node.IsProvisioned() {
			continue
		}
		sourceID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(project.ID.String()+"|"+node.RuntimeNodeID.String()+"|workspace_provision"))
		nodeLabel := node.RuntimeNodeID.String()
		if s.workspaceCommander != nil {
			if rec, err := s.workspaceCommander.GetNodeByID(ctx, node.RuntimeNodeID); err == nil && strings.TrimSpace(rec.Name) != "" {
				nodeLabel = strings.TrimSpace(rec.Name)
			}
		}
		summary := fmt.Sprintf(
			"节点「%s」已绑定但未供给工作区。确认供给后才会在该节点创建/克隆目录；主节点上未推送的改动不在其中。",
			nodeLabel,
		)
		if repoHint != "" {
			summary = fmt.Sprintf(
				"节点「%s」上将得到远端 %s 的当前内容；主节点上未推送的改动不在其中。确认后才在该节点执行 clone。",
				nodeLabel, repoHint,
			)
		}
		if err := s.workspaceProvisionInbox.UpsertWorkspaceProvisionTodo(ctx, WorkspaceProvisionInboxItem{
			TenantID:        project.TenantID,
			TargetUserID:    target,
			SourceID:        sourceID,
			SourceProjectID: project.ID,
			Title:           fmt.Sprintf("项目「%s」需供给到节点「%s」", project.Name, nodeLabel),
			Summary:         summary,
			DeepLink: map[string]any{
				"type": "project_workspace_provision_pending",
				"path": fmt.Sprintf("/projects/%s/config", project.ID.String()),
			},
			ContextPayload: map[string]any{
				"project_id":      project.ID.String(),
				"runtime_node_id": node.RuntimeNodeID.String(),
				"directory_name":  project.WorkspaceDirectoryName(),
				"repo_url":        repoHint,
				"opened_at":       time.Now().UTC().Format(time.RFC3339),
			},
		}); err != nil {
			slog.Default().Warn("workspace provision todo upsert failed",
				"project_id", project.ID.String(),
				"runtime_node_id", node.RuntimeNodeID.String(),
				"error", err.Error(),
			)
		}
	}
}
