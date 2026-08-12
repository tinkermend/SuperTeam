package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/google/uuid"

	runtimepkg "github.com/superteam/control-plane/internal/runtime"
)

// Reason codes surfaced by ResolveProjectTaskNode when a runnable node cannot be
// selected. They map to distinct dispatch/gate outcomes: a missing eligible
// online node is retryable ("bind/start a runtime"), whereas an offline pinned
// node means the task must pause on its existing node rather than drift.
const (
	NodeResolutionReasonNoEligibleOnlineNode = "no_eligible_online_node"
	NodeResolutionReasonPinnedNodeOffline    = "pinned_node_offline"
	NodeResolutionReasonWorkspaceUnavailable = "workspace_unavailable"
)

// NodeResolution is the outcome of three-layer runtime node resolution.
type NodeResolution struct {
	// NodeID is the resolved runtime node (runtime_nodes.id). Nil when Paused or
	// Reason is set.
	NodeID uuid.UUID
	// Pinned is true when NodeID was reused from the task's existing attempt.
	Pinned bool
	// Paused is true when the task's pinned node is offline/at capacity; the task
	// must wait on that node and must NOT be re-selected onto a different one.
	Paused bool
	// Reason is one of the NodeResolutionReason* codes, or "" on success.
	Reason string
}

// ResolveProjectTaskNodeInput carries the identifiers needed to resolve a
// runtime node for a project task dispatch.
type ResolveProjectTaskNodeInput struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DigitalEmployeeID uuid.UUID
	// ProjectTaskID selects the task-pin layer. When Nil (very first dispatch or
	// callers with no task context) the pin layer is skipped.
	ProjectTaskID uuid.UUID
	// RequiredProvider, when non-empty, filters candidate nodes to those that
	// advertise the provider. Empty skips provider filtering (the dispatch
	// preflight applies a provider-health check as a downstream safety net).
	RequiredProvider string
	// DryRun resolves a node for reporting/readiness without persisting affinity.
	DryRun bool
}

// ResolveProjectTaskNode implements the three-layer runtime node resolver:
//
//	layer 3 (task hard-pin): if the task's current attempt is already bound to a
//	  node, reuse it when it is eligible+online+has capacity; if that node is
//	  offline/full, pause (do NOT drift to another node).
//	layer 1 (eligibility ∩ online): otherwise gather the project's eligible nodes
//	  that are online, have capacity, and support the required provider.
//	layer 2 (employee affinity → load): prefer the employee's sticky node when it
//	  is a candidate, else pick the lowest-load candidate; persist the choice.
func (s *Service) ResolveProjectTaskNode(ctx context.Context, in ResolveProjectTaskNodeInput) (NodeResolution, error) {
	if s.runtimeNodes == nil {
		return NodeResolution{}, ErrProjectNotFound
	}

	project, err := s.repository.GetProject(ctx, in.TenantID, in.ProjectID)
	if err != nil {
		return NodeResolution{}, err
	}
	if project.WorkspaceReadyStatus != "" && project.WorkspaceReadyStatus != WorkspaceReadyStatusReady {
		return NodeResolution{}, fmt.Errorf("%w: status=%s", ErrProjectWorkspaceNotReady, project.WorkspaceReadyStatus)
	}

	eligibleNodes, err := s.repository.ListProjectRuntimeNodes(ctx, in.TenantID, in.ProjectID)
	if err != nil {
		return NodeResolution{}, err
	}
	eligible := make(map[uuid.UUID]struct{}, len(eligibleNodes))
	for _, node := range eligibleNodes {
		eligible[node.RuntimeNodeID] = struct{}{}
	}

	nodes, err := s.runtimeNodes.ListRuntimeNodesForTenant(ctx, runtimepkg.ListRuntimeNodesForTenantParams{
		TenantID: in.TenantID,
		Limit:    500,
	})
	if err != nil {
		return NodeResolution{}, err
	}
	byID := make(map[uuid.UUID]runtimepkg.NodeRecord, len(nodes))
	for _, node := range nodes {
		if node.TenantID == in.TenantID {
			byID[node.ID] = node
		}
	}

	dispatchable := func(id uuid.UUID) bool {
		if _, ok := eligible[id]; !ok {
			return false
		}
		node, ok := byID[id]
		if !ok {
			return false
		}
		return runtimeNodeDispatchable(node, in.RequiredProvider)
	}

	// Layer 3: task hard-pin.
	if in.ProjectTaskID != uuid.Nil {
		attempt, err := s.repository.GetCurrentProjectTaskAttempt(ctx, in.TenantID, in.ProjectTaskID)
		switch {
		case err == nil && attempt.RuntimeNodeID != nil && *attempt.RuntimeNodeID != uuid.Nil:
			pinned := *attempt.RuntimeNodeID
			if dispatchable(pinned) {
				if !in.DryRun {
					requireGit := project.RepoBinding.Status == ProjectRepoBindingStatusBound
					if err := s.validateProjectWorkspaceOnNode(ctx, project.TenantID, project.ID, project.WorkspaceDirectoryName(), pinned, requireGit); err != nil {
						return NodeResolution{}, fmt.Errorf("%w: pinned node workspace invalid: %v", ErrProjectWorkspaceUnavailable, err)
					}
				}
				return NodeResolution{NodeID: pinned, Pinned: true}, nil
			}
			return NodeResolution{Paused: true, Reason: NodeResolutionReasonPinnedNodeOffline}, nil
		case err != nil && !errors.Is(err, ErrProjectNotFound):
			return NodeResolution{}, err
		}
	}

	// Layer 1: eligibility ∩ online ∩ capacity ∩ provider.
	candidates := make([]runtimepkg.NodeRecord, 0, len(eligibleNodes))
	inCandidates := make(map[uuid.UUID]struct{}, len(eligibleNodes))
	for _, node := range eligibleNodes {
		record, ok := byID[node.RuntimeNodeID]
		if !ok || !runtimeNodeDispatchable(record, in.RequiredProvider) {
			continue
		}
		if _, seen := inCandidates[record.ID]; seen {
			continue
		}
		inCandidates[record.ID] = struct{}{}
		candidates = append(candidates, record)
	}
	if len(candidates) == 0 {
		return NodeResolution{Reason: NodeResolutionReasonNoEligibleOnlineNode}, nil
	}

	// Only provisioned bindings are dispatch candidates (spec 2026-08-12 §5.2).
	// eligibleNodes already carries ProvisionStatus — no second round-trip.
	provisioned := make(map[uuid.UUID]struct{}, len(eligibleNodes))
	for _, binding := range eligibleNodes {
		if binding.IsProvisioned() {
			provisioned[binding.RuntimeNodeID] = struct{}{}
		}
	}
	filtered := make([]runtimepkg.NodeRecord, 0, len(candidates))
	for _, record := range candidates {
		if _, ok := provisioned[record.ID]; ok {
			filtered = append(filtered, record)
		}
	}
	candidates = filtered
	inCandidates = make(map[uuid.UUID]struct{}, len(candidates))
	for _, record := range candidates {
		inCandidates[record.ID] = struct{}{}
	}
	if len(candidates) == 0 {
		if !in.DryRun {
			s.maybeOpenWorkspaceProvisionTodos(ctx, project, uuid.Nil)
		}
		return NodeResolution{Reason: NodeResolutionReasonWorkspaceUnavailable}, nil
	}

	requireGit := project.RepoBinding.Status == ProjectRepoBindingStatusBound
	// 真实派发:对候选节点做工作区校验。DryRun(就绪探测)跳过,避免轮询拖垮 Runtime。
	if !in.DryRun {
		validated, validateErr := s.filterCandidatesByWorkspace(ctx, project, candidates, requireGit)
		if validateErr != nil || len(validated) == 0 {
			// Primary disk missing / clone incomplete + standby unprovisioned:
			// still open provision todos (spec §5.3 lazy trigger). Do not leak a
			// raw error that skips the todo path.
			s.maybeOpenWorkspaceProvisionTodos(ctx, project, uuid.Nil)
			if validateErr != nil && !errors.Is(validateErr, ErrProjectWorkspaceUnavailable) {
				return NodeResolution{}, validateErr
			}
			return NodeResolution{Reason: NodeResolutionReasonWorkspaceUnavailable}, nil
		}
		candidates = validated
		inCandidates = make(map[uuid.UUID]struct{}, len(candidates))
		for _, record := range candidates {
			inCandidates[record.ID] = struct{}{}
		}
	}

	// Layer 2: primary sticky → employee affinity → lowest load.
	// 主节点存活且校验通过时必选主节点;主挂才落到其它已绑定且校验通过的节点。
	chosen := lowestLoadNodeID(candidates)
	primaryUsable := false
	if project.PrimaryRuntimeNodeID != nil {
		if _, ok := inCandidates[*project.PrimaryRuntimeNodeID]; ok {
			chosen = *project.PrimaryRuntimeNodeID
			primaryUsable = true
		}
	}
	affinity, err := s.repository.GetProjectEmployeeNodeAffinity(ctx, in.TenantID, in.ProjectID, in.DigitalEmployeeID)
	switch {
	case err == nil && affinity.RuntimeNodeID != uuid.Nil:
		// 员工亲和只在主节点不可用时生效;主节点可用时保持粘滞主节点。
		if !primaryUsable {
			if _, ok := inCandidates[affinity.RuntimeNodeID]; ok {
				chosen = affinity.RuntimeNodeID
			}
		}
	case err != nil && !errors.Is(err, ErrProjectNotFound):
		return NodeResolution{}, err
	}

	if !in.DryRun {
		if _, err := s.repository.UpsertProjectEmployeeNodeAffinity(ctx, in.TenantID, in.ProjectID, in.DigitalEmployeeID, chosen); err != nil {
			return NodeResolution{}, err
		}
	}
	return NodeResolution{NodeID: chosen, Pinned: false}, nil
}

func (s *Service) filterCandidatesByWorkspace(
	ctx context.Context,
	project Project,
	candidates []runtimepkg.NodeRecord,
	requireGit bool,
) ([]runtimepkg.NodeRecord, error) {
	if s.workspaceCommander == nil {
		return candidates, nil
	}
	validated := make([]runtimepkg.NodeRecord, 0, len(candidates))
	var lastErr error
	for _, record := range candidates {
		if err := s.validateProjectWorkspaceOnNode(ctx, project.TenantID, project.ID, project.WorkspaceDirectoryName(), record.ID, requireGit); err != nil {
			lastErr = err
			slog.Default().Warn("runtime node failed project workspace validation",
				"project_id", project.ID.String(),
				"runtime_node_id", record.ID.String(),
				"error", err.Error(),
			)
			continue
		}
		validated = append(validated, record)
	}
	if len(validated) == 0 && lastErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrProjectWorkspaceUnavailable, lastErr)
	}
	return validated, nil
}

// runtimeNodeDispatchable reports whether a runtime node can currently accept a
// dispatch: online, with a free slot, and (when required) advertising the
// provider. It mirrors the online/capacity checks used by project runtime
// readiness so resolution and readiness agree on what "usable" means.
func runtimeNodeDispatchable(node runtimepkg.NodeRecord, requiredProvider string) bool {
	if strings.TrimSpace(node.Status) != string(runtimepkg.NodeStatusOnline) {
		return false
	}
	if node.MaxSlots > 0 && node.CurrentLoad >= node.MaxSlots {
		return false
	}
	if !nodeSupportsProvider(node, requiredProvider) {
		return false
	}
	return true
}

func nodeSupportsProvider(node runtimepkg.NodeRecord, providerType string) bool {
	providerType = strings.TrimSpace(providerType)
	if providerType == "" || len(node.SupportedProviders) == 0 {
		return true
	}
	var decoded []string
	if err := json.Unmarshal(node.SupportedProviders, &decoded); err != nil {
		return true
	}
	for _, provider := range decoded {
		if strings.TrimSpace(provider) == providerType {
			return true
		}
	}
	return false
}

// lowestLoadNodeID returns the candidate node id with the smallest CurrentLoad,
// breaking ties by lexicographically smallest node id for deterministic output.
func lowestLoadNodeID(candidates []runtimepkg.NodeRecord) uuid.UUID {
	return lowestLoadNode(candidates).ID
}

// lowestLoadNode is lowestLoadNodeID's full-record counterpart, used where the
// caller needs more than the id (e.g. readiness reporting a representative
// node's display name).
func lowestLoadNode(candidates []runtimepkg.NodeRecord) runtimepkg.NodeRecord {
	sorted := append([]runtimepkg.NodeRecord(nil), candidates...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].CurrentLoad != sorted[j].CurrentLoad {
			return sorted[i].CurrentLoad < sorted[j].CurrentLoad
		}
		return sorted[i].ID.String() < sorted[j].ID.String()
	})
	return sorted[0]
}

// usableEligibleNodes returns the project's eligibility-set nodes that are
// currently online, have capacity, and are connected — the set-level building
// block GetProjectRuntimeReadiness uses ("≥1 eligible node online + connected +
// with capacity"), sharing the online/capacity predicate (runtimeNodeDispatchable)
// with ResolveProjectTaskNode so the two never disagree about what "usable"
// means.
func (s *Service) usableEligibleNodes(ctx context.Context, tenantID uuid.UUID, eligibleNodes []ProjectRuntimeNode) ([]runtimepkg.NodeRecord, error) {
	nodes, err := s.runtimeNodes.ListRuntimeNodesForTenant(ctx, runtimepkg.ListRuntimeNodesForTenantParams{
		TenantID: tenantID,
		Limit:    500,
	})
	if err != nil {
		return nil, err
	}
	byID := make(map[uuid.UUID]runtimepkg.NodeRecord, len(nodes))
	for _, node := range nodes {
		if node.TenantID == tenantID {
			byID[node.ID] = node
		}
	}
	usable := make([]runtimepkg.NodeRecord, 0, len(eligibleNodes))
	seen := make(map[uuid.UUID]struct{}, len(eligibleNodes))
	for _, eligible := range eligibleNodes {
		record, ok := byID[eligible.RuntimeNodeID]
		if !ok || !runtimeNodeDispatchable(record, "") {
			continue
		}
		if !s.runtimeNodes.IsConnected(record.NodeID) {
			continue
		}
		if _, dup := seen[record.ID]; dup {
			continue
		}
		seen[record.ID] = struct{}{}
		usable = append(usable, record)
	}
	return usable, nil
}

// unionProviderCapabilities reports the set of provider types available across
// nodes — "does the eligible, usable set cover every required provider",
// rather than requiring a single node to cover all of them.
func (s *Service) unionProviderCapabilities(ctx context.Context, tenantID uuid.UUID, nodes []runtimepkg.NodeRecord) ([]string, error) {
	union := make([]string, 0, len(nodes))
	for _, node := range nodes {
		capabilities, err := s.runtimeNodes.ListRuntimeCapabilitiesForNode(ctx, tenantID, node.NodeID)
		if err != nil {
			return nil, err
		}
		union = append(union, availableProviderCapabilities(capabilities, node.SupportedProviders)...)
	}
	return normalizeStringSet(union), nil
}
