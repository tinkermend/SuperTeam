package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	runtimepkg "github.com/superteam/control-plane/internal/runtime"
)

type Service struct {
	repository                Repository
	coordinator               CoordinatorSignalClient
	approvals                 ApprovalResolver
	digitalEmployeeIdentities DigitalEmployeeIdentityLookup
	inbox                     DecisionInboxProjector
	archiveArtifactLocker     ArchiveArtifactLocker
	teamScopeAuthorizer       ProjectTeamScopeAuthorizer
	runtimeNodes              ProjectRuntimeNodeReader
	planningProfiles          DigitalEmployeePlanningProfileSource
	scenarioTemplates         ScenarioTemplateResolver
}

// ScenarioTemplateResolver is the narrow view of the scenario template
// registry the project service needs: existence + status at bind time. nil
// resolver skips validation (tests and callers without the registry wired).
type ScenarioTemplateResolver interface {
	ResolveScenarioTemplate(ctx context.Context, tenantID uuid.UUID, key string) (ScenarioTemplateBinding, error)
}

type ScenarioTemplateBinding struct {
	Key    string
	Name   string
	Status string
}

func (s *Service) SetScenarioTemplateResolver(resolver ScenarioTemplateResolver) {
	s.scenarioTemplates = resolver
}

const (
	projectTaskAttemptStartReadinessAttempts = 25
	projectTaskAttemptStartReadinessBackoff  = 200 * time.Millisecond
)

type latestConfigRevisionRepository interface {
	GetLatestConfigRevision(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectConfigRevision, error)
}

type ProjectTeamScopeAuthorizer interface {
	CanUseTeamForProject(ctx context.Context, tenantID, userID, teamID uuid.UUID) (bool, error)
}

type ProjectRuntimeNodeReader interface {
	ListRuntimeNodesForTenant(ctx context.Context, params runtimepkg.ListRuntimeNodesForTenantParams) ([]runtimepkg.NodeRecord, error)
	ListRuntimeCapabilitiesForNode(ctx context.Context, tenantID uuid.UUID, nodeID string) ([]runtimepkg.RuntimeCapability, error)
	IsConnected(nodeID string) bool
}

type DigitalEmployeePlanningProfileSource interface {
	PlanningProfileRecords(ctx context.Context, tenantID, projectID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord, error)
}

type DigitalEmployeePlanningProfileSourceRecord struct {
	DigitalEmployeeID uuid.UUID
	ProviderType      string
	ExecutionStatus   string
}

type ApprovalResolver interface {
	ResolveApproval(ctx context.Context, req ResolveApprovalRequest) error
	// GetRequestContextPayload returns the approval request's ContextPayload as
	// recorded at creation time — used by ResolveDecision to read decision-type
	// vocabulary (e.g. a planning_gap decision's structured gap) that must come
	// from the original record, not from the resolving caller's payload.
	GetRequestContextPayload(ctx context.Context, tenantID, approvalRequestID uuid.UUID) (map[string]any, error)
	// CreateRequest opens a new approval request and returns its ID. The service
	// uses it to open the project acceptance review when a criterion sign-off
	// makes the last demand terminal (maybeOpenProjectAcceptanceReview) — the
	// only path that opens a review outside the coordinator.
	CreateRequest(ctx context.Context, req CreateApprovalRequestInput) (uuid.UUID, error)
}

// CreateApprovalRequestInput mirrors approval.CreateRequestInput's fields the
// project service needs to open a project_acceptance review; the adapter maps it
// to the approval package's own input type.
type CreateApprovalRequestInput struct {
	TenantID      uuid.UUID
	ResourceType  string
	ResourceID    uuid.UUID
	RequesterType string
	TargetUserID  uuid.UUID
	DecisionType  string
	Title         string
	Summary       string
	RiskLevel     string
	Options       []any
}

type DigitalEmployeeIdentity struct {
	Role        string
	AvatarAsset *ProjectTaskGraphEmployeeAvatarAsset
}

type DigitalEmployeeIdentityLookup interface {
	GetDigitalEmployeeIdentity(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (DigitalEmployeeIdentity, error)
}

func NewService(repository Repository) (*Service, error) {
	return NewServiceWithCoordinator(repository, NoopCoordinatorSignalClient{})
}

func NewServiceWithCoordinator(repository Repository, coordinator CoordinatorSignalClient) (*Service, error) {
	return NewServiceWithCoordinatorAndApprovals(repository, coordinator, nil)
}

func NewServiceWithCoordinatorAndApprovals(repository Repository, coordinator CoordinatorSignalClient, approvals ApprovalResolver) (*Service, error) {
	return NewServiceWithCoordinatorApprovalsAndArchiveArtifactLocker(repository, coordinator, approvals, nil)
}

func NewServiceWithArchiveArtifactLocker(repository Repository, locker ArchiveArtifactLocker) (*Service, error) {
	return NewServiceWithCoordinatorApprovalsAndArchiveArtifactLocker(repository, NoopCoordinatorSignalClient{}, nil, locker)
}

func NewServiceWithCoordinatorApprovalsAndArchiveArtifactLocker(repository Repository, coordinator CoordinatorSignalClient, approvals ApprovalResolver, locker ArchiveArtifactLocker) (*Service, error) {
	return NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repository, coordinator, approvals, nil, locker)
}

func NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repository Repository, coordinator CoordinatorSignalClient, approvals ApprovalResolver, inbox DecisionInboxProjector, locker ArchiveArtifactLocker) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("project repository is required")
	}
	if coordinator == nil {
		coordinator = NoopCoordinatorSignalClient{}
	}
	teamScopeAuthorizer, _ := repository.(ProjectTeamScopeAuthorizer)
	return &Service{
		repository:            repository,
		coordinator:           coordinator,
		approvals:             approvals,
		inbox:                 inbox,
		archiveArtifactLocker: locker,
		teamScopeAuthorizer:   teamScopeAuthorizer,
	}, nil
}

func (s *Service) SetTeamScopeAuthorizer(authorizer ProjectTeamScopeAuthorizer) {
	if s != nil {
		s.teamScopeAuthorizer = authorizer
	}
}

func (s *Service) SetDigitalEmployeeIdentityLookup(lookup DigitalEmployeeIdentityLookup) {
	if s != nil {
		s.digitalEmployeeIdentities = lookup
	}
}

func (s *Service) SetProjectRuntimeNodeReader(reader ProjectRuntimeNodeReader) {
	if s != nil {
		s.runtimeNodes = reader
	}
}

func (s *Service) SetDigitalEmployeePlanningProfileSource(source DigitalEmployeePlanningProfileSource) {
	if s != nil {
		s.planningProfiles = source
	}
}

func (s *Service) CreateProject(ctx context.Context, req CreateProjectRequest) (*CreateProjectResult, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Goal = strings.TrimSpace(req.Goal)
	if req.TenantID == uuid.Nil || req.ActorUserID == uuid.Nil || req.HumanOwnerUserID == uuid.Nil || req.Name == "" || req.Goal == "" {
		return nil, ErrInvalidProject
	}
	if err := validateMembers(req.Members); err != nil {
		return nil, err
	}
	if err := s.validateProjectTeamScopes(ctx, req); err != nil {
		return nil, err
	}
	repoBinding, err := normalizeProjectRepoBindingInput(req.RepoBinding)
	if err != nil {
		return nil, err
	}
	req.RepoBinding = projectRepoBindingInputFromBinding(repoBinding)

	runtimeNodeIDs, err := s.validateRuntimeNodeIDs(ctx, req.TenantID, req.RuntimeNodeIDs)
	if err != nil {
		return nil, err
	}
	req.RuntimeNodeIDs = runtimeNodeIDs

	if req.ScenarioTemplateKey != nil {
		key := strings.TrimSpace(*req.ScenarioTemplateKey)
		if key == "" {
			req.ScenarioTemplateKey = nil
		} else if s.scenarioTemplates != nil {
			binding, err := s.scenarioTemplates.ResolveScenarioTemplate(ctx, req.TenantID, key)
			if err != nil {
				return nil, fmt.Errorf("scenario template %q: %w", key, ErrInvalidProject)
			}
			if binding.Status != "active" {
				return nil, fmt.Errorf("scenario template %q is %s: %w", key, binding.Status, ErrInvalidProject)
			}
			req.ScenarioTemplateKey = &key
		}
	}

	projectID := uuid.New()
	workflowID := fmt.Sprintf("project-coordinator:%s", projectID)
	project, err := s.repository.CreateProject(ctx, req, projectID, workflowID)
	if err != nil {
		return nil, err
	}
	for _, runtimeNodeID := range runtimeNodeIDs {
		if _, err := s.repository.InsertProjectRuntimeNode(ctx, req.TenantID, project.ID, runtimeNodeID); err != nil {
			return nil, err
		}
	}
	members, err := s.repository.ReplaceProjectMembers(ctx, req.TenantID, project.ID, ensureOwnerMember(req))
	if err != nil {
		return nil, err
	}
	if _, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  req.TenantID,
		ProjectID: project.ID,
		EventType: ProjectEventCreated,
		ActorType: "human_user",
		ActorID:   req.ActorUserID.String(),
		Summary:   "项目已创建",
		Payload:   map[string]any{"name": project.Name},
	}); err != nil {
		return nil, err
	}
	if _, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  req.TenantID,
		ProjectID: project.ID,
		EventType: ProjectEventConfigChanged,
		ActorType: "human_user",
		ActorID:   req.ActorUserID.String(),
		Summary:   "项目配置已初始化",
		Payload:   map[string]any{"member_count": len(members)},
	}); err != nil {
		return nil, err
	}
	if err := s.coordinator.EnsureProjectCoordinator(ctx, ProjectCoordinatorSignal{
		TenantID:   req.TenantID,
		ProjectID:  project.ID,
		WorkflowID: project.CoordinationWorkflowID,
	}); err != nil {
		return nil, err
	}

	return &CreateProjectResult{Project: project, Members: members}, nil
}

func (s *Service) validateProjectTeamScopes(ctx context.Context, req CreateProjectRequest) error {
	return s.validateProjectTeamScopeAccess(ctx, req.TenantID, req.ActorUserID, req.TeamID, req.Members)
}

func (s *Service) validateProjectTeamScopeAccess(ctx context.Context, tenantID, actorUserID uuid.UUID, projectTeamID *uuid.UUID, members []ProjectMemberInput) error {
	teamIDs := make(map[uuid.UUID]struct{})
	orderedTeamIDs := make([]uuid.UUID, 0)
	addTeamID := func(teamID uuid.UUID) {
		if teamID == uuid.Nil {
			return
		}
		if _, ok := teamIDs[teamID]; ok {
			return
		}
		teamIDs[teamID] = struct{}{}
		orderedTeamIDs = append(orderedTeamIDs, teamID)
	}
	if projectTeamID != nil && *projectTeamID != uuid.Nil {
		addTeamID(*projectTeamID)
	}
	for _, member := range members {
		if member.PrincipalType == PrincipalTypeTeam && member.PrincipalID != uuid.Nil {
			addTeamID(member.PrincipalID)
		}
	}
	if len(teamIDs) == 0 {
		return nil
	}
	if s.teamScopeAuthorizer == nil {
		return ErrUnauthorizedProjectTeamScope
	}
	sort.Slice(orderedTeamIDs, func(i, j int) bool {
		return orderedTeamIDs[i].String() < orderedTeamIDs[j].String()
	})
	for _, teamID := range orderedTeamIDs {
		allowed, err := s.teamScopeAuthorizer.CanUseTeamForProject(ctx, tenantID, actorUserID, teamID)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrUnauthorizedProjectTeamScope
		}
	}
	return nil
}

// validateRuntimeNodeIDs requires at least one runtime node id, dedupes the
// input (preserving first-seen order), and confirms every id resolves to a
// runtime node registered for the tenant.
func (s *Service) validateRuntimeNodeIDs(ctx context.Context, tenantID uuid.UUID, runtimeNodeIDs []uuid.UUID) ([]uuid.UUID, error) {
	if len(runtimeNodeIDs) == 0 {
		return nil, ErrProjectRuntimeNodesRequired
	}
	seen := make(map[uuid.UUID]struct{}, len(runtimeNodeIDs))
	deduped := make([]uuid.UUID, 0, len(runtimeNodeIDs))
	for _, runtimeNodeID := range runtimeNodeIDs {
		if runtimeNodeID == uuid.Nil {
			continue
		}
		if _, ok := seen[runtimeNodeID]; ok {
			continue
		}
		seen[runtimeNodeID] = struct{}{}
		deduped = append(deduped, runtimeNodeID)
	}
	if len(deduped) == 0 {
		return nil, ErrProjectRuntimeNodesRequired
	}
	for _, runtimeNodeID := range deduped {
		if err := s.requireRuntimeNodeForTenant(ctx, tenantID, runtimeNodeID); err != nil {
			return nil, err
		}
	}
	return deduped, nil
}

// ListProjectRuntimeNodes returns the runtime node eligibility set bound to a
// project — the pool of nodes a task under this project may be dispatched to.
func (s *Service) ListProjectRuntimeNodes(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectRuntimeNode, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	return s.repository.ListProjectRuntimeNodes(ctx, tenantID, projectID)
}

func (s *Service) GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (*Project, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	project, err := s.repository.GetProject(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	return &project, nil
}

func (s *Service) requireActiveProject(ctx context.Context, tenantID, projectID uuid.UUID) (Project, error) {
	project, err := s.repository.GetProject(ctx, tenantID, projectID)
	if err != nil {
		return Project{}, err
	}
	if project.Status == ProjectStatusArchived || project.ArchivedAt != nil {
		return Project{}, ErrProjectArchived
	}
	return project, nil
}

func (s *Service) GetProjectRuntimePlacement(ctx context.Context, req GetProjectRuntimePlacementRequest) (*ProjectRuntimePlacement, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	if _, err := s.requireActiveProject(ctx, req.TenantID, req.ProjectID); err != nil {
		return nil, err
	}
	placement, err := s.repository.GetActiveProjectPlacement(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	return &placement, nil
}

func (s *Service) PutProjectRuntimePlacement(ctx context.Context, req PutProjectRuntimePlacementRequest) (*ProjectRuntimePlacement, error) {
	req.Reason = strings.TrimSpace(req.Reason)
	req.ExpectedProviderTypes = normalizeStringSet(req.ExpectedProviderTypes)
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.RuntimeNodeID == uuid.Nil || req.ActorUserID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	if _, err := s.requireActiveProject(ctx, req.TenantID, req.ProjectID); err != nil {
		return nil, err
	}
	if err := s.requireRuntimeNodeForTenant(ctx, req.TenantID, req.RuntimeNodeID); err != nil {
		return nil, err
	}
	placement, err := s.repository.UpsertProjectPlacement(ctx, req)
	if err != nil {
		return nil, err
	}
	if _, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  req.TenantID,
		ProjectID: req.ProjectID,
		EventType: ProjectEventRuntimePlacementUpdated,
		ActorType: "human_user",
		ActorID:   req.ActorUserID.String(),
		Summary:   "项目 Runtime 绑定已更新",
		Payload: map[string]any{
			"runtime_node_id":         placement.RuntimeNodeID.String(),
			"reason":                  placement.PlacementReason,
			"expected_provider_types": append([]string(nil), req.ExpectedProviderTypes...),
		},
	}); err != nil {
		return nil, err
	}
	return &placement, nil
}

func (s *Service) ReleaseProjectRuntimePlacement(ctx context.Context, req ReleaseProjectRuntimePlacementRequest) (*ProjectRuntimePlacement, error) {
	req.Reason = strings.TrimSpace(req.Reason)
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ActorUserID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	if _, err := s.requireActiveProject(ctx, req.TenantID, req.ProjectID); err != nil {
		return nil, err
	}
	placement, err := s.repository.ReleaseProjectPlacement(ctx, req)
	if err != nil {
		return nil, err
	}
	if _, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  req.TenantID,
		ProjectID: req.ProjectID,
		EventType: ProjectEventRuntimePlacementReleased,
		ActorType: "human_user",
		ActorID:   req.ActorUserID.String(),
		Summary:   "项目 Runtime 绑定已释放",
		Payload: map[string]any{
			"runtime_node_id": placement.RuntimeNodeID.String(),
			"reason":          req.Reason,
		},
	}); err != nil {
		return nil, err
	}
	return &placement, nil
}

func (s *Service) GetProjectRuntimeReadiness(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectRuntimePlacementReadiness, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	if _, err := s.requireActiveProject(ctx, tenantID, projectID); err != nil {
		return nil, err
	}
	members, err := s.repository.ListProjectMembers(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	employeeReadiness, requiredProviders, err := s.projectEmployeeReadiness(ctx, tenantID, projectID, members)
	if err != nil {
		return nil, err
	}
	readiness := &ProjectRuntimePlacementReadiness{
		PlacementStatus:       ProjectRuntimePlacementStatusMissing,
		RequiredProviderTypes: requiredProviders,
		EmployeeReadiness:     employeeReadiness,
	}

	// Plan B's node-selection authority is the project's runtime eligibility set
	// (project_runtime_nodes), not the legacy single active project_placement —
	// gating readiness on GetActiveProjectPlacement would permanently report
	// "missing" for every Plan B project regardless of its bound nodes.
	eligibleNodes, err := s.repository.ListProjectRuntimeNodes(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	if len(eligibleNodes) == 0 {
		readiness.BlockingReasons = append(readiness.BlockingReasons, ProjectReadinessReason{Code: "runtime_placement_missing", Message: "project has no active runtime placement"})
		readiness.NextActions = append(readiness.NextActions, ProjectReadinessAction{Code: "bind_runtime", Label: "Bind runtime"})
		return readiness, nil
	}

	if s.runtimeNodes == nil {
		readiness.PlacementStatus = ProjectRuntimePlacementStatusRuntimeOffline
		readiness.BlockingReasons = append(readiness.BlockingReasons, ProjectReadinessReason{Code: "runtime_node_reader_missing", Message: "runtime node reader is not configured"})
		readiness.NextActions = append(readiness.NextActions, ProjectReadinessAction{Code: "configure_runtime_reader", Label: "Configure runtime reader"})
		markEmployeesNotDispatchable(readiness.EmployeeReadiness, "runtime_node_reader_missing", "runtime node reader is not configured")
		return readiness, nil
	}

	// Set-level readiness: ≥1 eligible node online + connected + with capacity.
	// Unlike the old single-placement model, individual nodes in the set can be
	// offline/disconnected/full without blocking dispatch as long as at least
	// one usable node remains — so those per-node conditions collapse into one
	// blocking reason here rather than the old node-by-node cascade.
	usableNodes, err := s.usableEligibleNodes(ctx, tenantID, eligibleNodes)
	if err != nil {
		return nil, err
	}
	if len(usableNodes) == 0 {
		readiness.PlacementStatus = ProjectRuntimePlacementStatusRuntimeOffline
		readiness.BlockingReasons = append(readiness.BlockingReasons, ProjectReadinessReason{Code: "runtime_no_eligible_online_node", Message: "no eligible runtime node is online, connected, and has capacity"})
		readiness.NextActions = append(readiness.NextActions, ProjectReadinessAction{Code: "start_runtime", Label: "Start runtime"})
		markEmployeesNotDispatchable(readiness.EmployeeReadiness, "runtime_no_eligible_online_node", "no eligible runtime node is online, connected, and has capacity")
		return readiness, nil
	}
	representative := lowestLoadNode(usableNodes)
	readiness.RuntimeNodeID = &representative.ID
	readiness.RuntimeNodeName = representative.Name
	readiness.CommandChannelConnected = true

	// providerCapabilities is the UNION across the usable set: the set covers a
	// provider if any node in it does, not just a single placed node.
	providerCapabilities, err := s.unionProviderCapabilities(ctx, tenantID, usableNodes)
	if err != nil {
		return nil, err
	}
	readiness.ProviderCapabilities = providerCapabilities
	missingProviders := missingProviderTypes(requiredProviders, providerCapabilities)
	if len(missingProviders) > 0 {
		readiness.PlacementStatus = ProjectRuntimePlacementStatusProviderUnavailable
		readiness.BlockingReasons = append(readiness.BlockingReasons, ProjectReadinessReason{Code: "provider_unavailable", Message: "eligible runtime set does not cover every required provider"})
		readiness.NextActions = append(readiness.NextActions, ProjectReadinessAction{Code: "bind_provider_capable_runtime", Label: "Bind provider-capable runtime"})
		for i := range readiness.EmployeeReadiness {
			if readiness.EmployeeReadiness[i].ProviderType != "" && stringInSlice(readiness.EmployeeReadiness[i].ProviderType, missingProviders) {
				readiness.EmployeeReadiness[i].CanDispatch = false
				readiness.EmployeeReadiness[i].ReasonCode = "provider_unavailable"
				readiness.EmployeeReadiness[i].ReasonMessage = "required provider is unavailable on eligible runtime set"
			}
		}
		return readiness, nil
	}
	if hasEmployeeDispatchBlock(readiness.EmployeeReadiness) {
		readiness.PlacementStatus = ProjectRuntimePlacementStatusWorkspacePending
		code, action := employeeDispatchBlockProjectReason(readiness.EmployeeReadiness)
		readiness.BlockingReasons = append(readiness.BlockingReasons, ProjectReadinessReason{Code: code, Message: "one or more project employees are not dispatch-ready"})
		readiness.NextActions = append(readiness.NextActions, action)
		return readiness, nil
	}
	readiness.PlacementStatus = ProjectRuntimePlacementStatusReady
	for i := range readiness.EmployeeReadiness {
		readiness.EmployeeReadiness[i].CanDispatch = readiness.EmployeeReadiness[i].CanPlan
	}
	return readiness, nil
}

func (s *Service) CreateProjectTaskAttestation(ctx context.Context, req CreateProjectTaskAttestationRequest) (*ProjectTaskAttestation, error) {
	req.AttestationType = strings.TrimSpace(req.AttestationType)
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	req.CapabilityManifestVersion = strings.TrimSpace(req.CapabilityManifestVersion)
	if req.ProviderAuthMode == "" {
		req.ProviderAuthMode = ProjectTaskAttestationProviderAuthModeHost
	}
	if req.TenantID == uuid.Nil ||
		req.ProjectID == uuid.Nil ||
		req.ProjectTaskID == uuid.Nil ||
		req.AttemptID == uuid.Nil ||
		req.RuntimeNodeID == uuid.Nil ||
		req.DigitalEmployeeID == uuid.Nil {
		return nil, ErrInvalidProjectEvidence
	}
	if req.AttestationType == "" || req.IdempotencyKey == "" {
		return nil, ErrInvalidProjectEvidence
	}
	if !validProjectTaskAttestationStatus(req.Status) || !validProjectTaskAttestationProviderAuthMode(req.ProviderAuthMode) {
		return nil, ErrInvalidProjectEvidence
	}
	req.Metadata = removeRuntimeLocalPathMetadata(req.Metadata)
	attestation, err := s.repository.CreateProjectTaskAttestation(ctx, req)
	if err != nil {
		return nil, err
	}
	return &attestation, nil
}

func validProjectTaskAttestationStatus(status ProjectTaskAttestationStatus) bool {
	switch status {
	case ProjectTaskAttestationStatusSucceeded, ProjectTaskAttestationStatusFailed, ProjectTaskAttestationStatusCancelled, ProjectTaskAttestationStatusTimedOut:
		return true
	default:
		return false
	}
}

func validProjectTaskAttestationProviderAuthMode(mode ProjectTaskAttestationProviderAuthMode) bool {
	switch mode {
	case ProjectTaskAttestationProviderAuthModeHost, ProjectTaskAttestationProviderAuthModeEmployee, ProjectTaskAttestationProviderAuthModeExplicitCredential:
		return true
	default:
		return false
	}
}

func (s *Service) RecordProjectTaskAttemptBudgetHeartbeat(ctx context.Context, req RecordProjectTaskAttemptBudgetHeartbeatRequest) (*ProjectTaskAttemptBudgetHeartbeatResult, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ProjectTaskID == uuid.Nil || req.AttemptID == uuid.Nil || req.ConsumedWallClockSec < 0 || req.ConsumedTokens < 0 {
		return nil, ErrInvalidProject
	}
	task, err := s.repository.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
	if err != nil {
		return nil, err
	}
	if task.ProjectID != req.ProjectID {
		return nil, ErrProjectNotFound
	}
	attempt, err := s.repository.GetProjectTaskAttempt(ctx, req.TenantID, req.AttemptID)
	if err != nil {
		return nil, err
	}
	if attempt.ProjectTaskID != req.ProjectTaskID {
		return nil, ErrProjectNotFound
	}
	tripReason := ""
	if attempt.BudgetWallClockLimitSec != nil && req.ConsumedWallClockSec > *attempt.BudgetWallClockLimitSec {
		tripReason = "wall_clock_exceeded"
	}
	updated, err := s.repository.UpdateProjectTaskAttemptBudgetHeartbeat(ctx, req, tripReason)
	if err != nil {
		return nil, err
	}
	return &ProjectTaskAttemptBudgetHeartbeatResult{Attempt: updated, Tripped: tripReason != "", TripReason: tripReason}, nil
}

func (s *Service) ListProjects(ctx context.Context, req ListProjectsRequest) ([]Project, error) {
	if req.TenantID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	req.Limit, req.Offset = normalizePagination(req.Limit, req.Offset)
	return s.repository.ListProjects(ctx, req)
}

func (s *Service) QueueProjectTask(ctx context.Context, req QueueProjectTaskRequest) (QueueProjectTaskResult, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ProjectTaskID == uuid.Nil || req.DigitalEmployeeID == uuid.Nil {
		return QueueProjectTaskResult{}, ErrInvalidProject
	}
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	req.LeaseToken = strings.TrimSpace(req.LeaseToken)
	if req.IdempotencyKey == "" || req.LeaseToken == "" {
		return QueueProjectTaskResult{}, ErrInvalidProject
	}
	if len(req.ExecutionContextPacket) == 0 {
		task, err := s.repository.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
		if err != nil {
			return QueueProjectTaskResult{}, err
		}
		if task.ProjectID != req.ProjectID {
			return QueueProjectTaskResult{}, ErrProjectNotFound
		}
		packet, err := s.BuildProjectTaskExecutionPacket(ctx, task)
		if err != nil {
			return QueueProjectTaskResult{}, err
		}
		req.ExecutionContextPacket = projectTaskExecutionPacketMap(packet)
		req.ExecutionContextPacketVersion = packet.Version
	}
	if strings.TrimSpace(req.ExecutionContextPacketVersion) == "" {
		req.ExecutionContextPacketVersion = "v1"
	}
	return s.repository.QueueProjectTaskWithAttempt(ctx, req)
}

func (s *Service) BuildProjectTaskExecutionPacket(ctx context.Context, task ProjectTask) (ProjectTaskExecutionPacket, error) {
	if task.TenantID == uuid.Nil || task.ProjectID == uuid.Nil || task.ID == uuid.Nil {
		return ProjectTaskExecutionPacket{}, ErrInvalidProject
	}
	summary := ""
	if task.Summary != nil {
		summary = *task.Summary
	}
	riskLevel := ""
	if task.RiskLevel != nil {
		riskLevel = *task.RiskLevel
	}
	packet := ProjectTaskExecutionPacket{
		Version:              "v1",
		ProjectID:            task.ProjectID.String(),
		ProjectTaskID:        task.ID.String(),
		Title:                task.Title,
		Summary:              summary,
		ExpectedOutputs:      append([]any(nil), task.ExpectedOutputs...),
		InputRequirements:    cloneMap(mapOrEmptyAny(task.InputRequirements)),
		HandoffContract:      cloneMap(mapOrEmptyAny(task.HandoffContract)),
		ForbiddenScopes:      []string{},
		RiskLevel:            riskLevel,
		StopForHumanCriteria: []string{HumanWaitReasonMissingContext, HumanWaitReasonApprovalRequired, HumanWaitReasonPermissionRequired, HumanWaitReasonPlanInvalid},
	}
	if len(task.BlockedByTaskIDs) > 0 {
		summaries, err := s.repository.ListExecutionSummaries(ctx, task.TenantID, task.ProjectID, 200, 0)
		if err != nil {
			return ProjectTaskExecutionPacket{}, err
		}
		blockedBy := make(map[uuid.UUID]struct{}, len(task.BlockedByTaskIDs))
		for _, blockerID := range task.BlockedByTaskIDs {
			blockedBy[blockerID] = struct{}{}
		}
		for _, summary := range summaries {
			if _, ok := blockedBy[summary.ProjectTaskID]; !ok {
				continue
			}
			packet.DependencyOutputs = append(packet.DependencyOutputs, ProjectTaskDependencyOutput{
				ProjectTaskID: summary.ProjectTaskID.String(),
				Conclusion:    summary.Conclusion,
				EvidenceRefs:  append([]any(nil), summary.EvidenceRefs...),
				ArtifactRefs:  append([]any(nil), summary.ArtifactRefs...),
			})
		}
	}
	decisions, err := s.repository.ListDecisionRequests(ctx, task.TenantID, task.ProjectID, 200, 0)
	if err != nil {
		return ProjectTaskExecutionPacket{}, err
	}
	for _, decision := range decisions {
		if decision.ProjectTaskID == nil || *decision.ProjectTaskID != task.ID {
			continue
		}
		packet.HumanDecisionRefs = append(packet.HumanDecisionRefs, ProjectTaskHumanDecisionRef{
			DecisionRequestID: decision.ID.String(),
			DecisionType:      decision.DecisionType,
			StatusSnapshot:    decision.StatusSnapshot,
		})
	}
	return packet, nil
}

func (s *Service) RecordAttemptContextUpdate(ctx context.Context, req RecordAttemptContextUpdateRequest) (ProjectTaskAttemptContextUpdate, error) {
	req.UpdateKind = strings.TrimSpace(req.UpdateKind)
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ProjectTaskID == uuid.Nil || req.UpdateKind == "" || req.Payload == nil {
		return ProjectTaskAttemptContextUpdate{}, ErrInvalidProject
	}
	task, err := s.repository.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
	if err != nil {
		return ProjectTaskAttemptContextUpdate{}, err
	}
	if task.ProjectID != req.ProjectID {
		return ProjectTaskAttemptContextUpdate{}, ErrProjectNotFound
	}
	attemptID := req.AttemptID
	if attemptID == nil {
		attemptID = task.CurrentAttemptID
	}
	deliveryMode := projectTaskContextUpdateDeliveryMode(task, req.UpdateKind)
	return s.repository.RecordProjectTaskAttemptContextUpdate(ctx, RecordProjectTaskAttemptContextUpdateRepositoryRequest{
		TenantID:      req.TenantID,
		ProjectTaskID: req.ProjectTaskID,
		AttemptID:     attemptID,
		UpdateKind:    req.UpdateKind,
		Payload:       cloneMap(req.Payload),
		DeliveryMode:  deliveryMode,
	})
}

func (s *Service) ListWorkflowInstances(ctx context.Context, req ListWorkflowInstancesRequest) ([]WorkflowInstanceSummary, error) {
	if req.TenantID == uuid.Nil || req.ActorUserID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	req.Query = strings.TrimSpace(req.Query)
	req.Limit, req.Offset = normalizeWorkflowInstancePagination(req.Limit, req.Offset)
	items, err := s.repository.ListWorkflowInstances(ctx, req)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Status = normalizeWorkflowInstanceStatus(items[i])
	}
	sort.SliceStable(items, func(i, j int) bool {
		leftRank := workflowInstanceAttentionRank(items[i].Status)
		rightRank := workflowInstanceAttentionRank(items[j].Status)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	if req.Status != nil {
		filtered := make([]WorkflowInstanceSummary, 0, len(items))
		for _, item := range items {
			if item.Status == *req.Status {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	return items, nil
}

func (s *Service) UpdateProjectConfig(ctx context.Context, req UpdateProjectConfigRequest) (*Project, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ActorUserID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	project, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	if project.Status == ProjectStatusArchived || project.ArchivedAt != nil {
		return nil, ErrProjectArchived
	}
	if req.Name != "" {
		req.Name = strings.TrimSpace(req.Name)
	}
	if req.Goal != "" {
		req.Goal = strings.TrimSpace(req.Goal)
	}
	if req.Members != nil {
		if err := validateMembers(*req.Members); err != nil {
			return nil, err
		}
		if err := s.validateProjectTeamScopeAccess(ctx, req.TenantID, req.ActorUserID, nil, *req.Members); err != nil {
			return nil, err
		}
	}
	if req.RepoBinding != nil {
		repoBinding, err := normalizeProjectRepoBindingInput(req.RepoBinding)
		if err != nil {
			return nil, err
		}
		req.RepoBinding = projectRepoBindingInputFromBinding(repoBinding)
	}

	updated, err := s.repository.UpdateProjectConfig(ctx, req)
	if err != nil {
		return nil, err
	}
	if req.Members != nil {
		if _, err := s.repository.ReplaceProjectMembers(ctx, req.TenantID, req.ProjectID, *req.Members); err != nil {
			return nil, err
		}
	}
	event, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  req.TenantID,
		ProjectID: req.ProjectID,
		EventType: ProjectEventConfigChanged,
		ActorType: "human_user",
		ActorID:   req.ActorUserID.String(),
		Summary:   "项目配置已更新",
		Payload:   map[string]any{"name": updated.Name},
	})
	if err != nil {
		return nil, err
	}
	if _, err := s.repository.CreateConfigRevision(ctx, req, updated, event.ID); err != nil {
		return nil, err
	}
	if err := s.coordinator.SignalProjectPolicyChanged(ctx, ProjectPolicyChangedSignal{
		TenantID:       req.TenantID,
		ProjectID:      req.ProjectID,
		ChangedEventID: event.ID,
		WorkflowID:     updated.CoordinationWorkflowID,
	}); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, req.ProjectID, "ProjectPolicyChanged", "failed", err, map[string]any{
			"changed_event_id": event.ID.String(),
		})
		return nil, err
	}
	return &updated, nil
}

func normalizeProjectRepoBindingInput(input *ProjectRepoBindingInput) (ProjectRepoBinding, error) {
	if input == nil {
		return unboundProjectRepoBinding(), nil
	}
	url := strings.TrimSpace(input.URL)
	defaultBranch := strings.TrimSpace(input.DefaultBranch)
	credentialRef := trimmedStringPtr(input.GitCredentialRef)
	scope := normalizeProjectRepoBindingScope(input.Scope)
	if url == "" && defaultBranch == "" && credentialRef == nil && len(scope) == 0 {
		return unboundProjectRepoBinding(), nil
	}
	if url == "" || defaultBranch == "" {
		return ProjectRepoBinding{}, ErrInvalidProject
	}
	return ProjectRepoBinding{
		Status:           ProjectRepoBindingStatusBound,
		URL:              url,
		DefaultBranch:    defaultBranch,
		GitCredentialRef: credentialRef,
		Scope:            scope,
	}, nil
}

func unboundProjectRepoBinding() ProjectRepoBinding {
	return ProjectRepoBinding{Status: ProjectRepoBindingStatusUnbound, Scope: []string{}}
}

func projectRepoBindingInputFromBinding(binding ProjectRepoBinding) *ProjectRepoBindingInput {
	if binding.Status != ProjectRepoBindingStatusBound {
		return &ProjectRepoBindingInput{Scope: []string{}}
	}
	return &ProjectRepoBindingInput{
		URL:              binding.URL,
		DefaultBranch:    binding.DefaultBranch,
		GitCredentialRef: binding.GitCredentialRef,
		Scope:            append([]string(nil), binding.Scope...),
	}
}

func normalizeProjectRepoBindingScope(values []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	return normalized
}

func trimmedStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeStringSet(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func (s *Service) projectEmployeeReadiness(ctx context.Context, tenantID, projectID uuid.UUID, members []ProjectMember) ([]ProjectEmployeeReadiness, []string, error) {
	employeeIDs := make([]uuid.UUID, 0, len(members))
	activeEmployees := make([]ProjectMember, 0, len(members))
	for _, member := range members {
		if member.PrincipalType != PrincipalTypeDigitalEmployee || member.PrincipalID == uuid.Nil || member.Status != "active" {
			continue
		}
		activeEmployees = append(activeEmployees, member)
		employeeIDs = append(employeeIDs, member.PrincipalID)
	}
	profileRecords := map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord{}
	if s.planningProfiles != nil && len(employeeIDs) > 0 {
		records, err := s.planningProfiles.PlanningProfileRecords(ctx, tenantID, projectID, employeeIDs)
		if err != nil {
			return nil, nil, err
		}
		profileRecords = records
	}
	readiness := make([]ProjectEmployeeReadiness, 0, len(activeEmployees))
	requiredProviders := make([]string, 0, len(activeEmployees))
	for _, member := range activeEmployees {
		displayName := ""
		if member.DisplayNameSnapshot != nil {
			displayName = strings.TrimSpace(*member.DisplayNameSnapshot)
		}
		record := profileRecords[member.PrincipalID]
		providerType := strings.TrimSpace(record.ProviderType)
		item := ProjectEmployeeReadiness{
			DigitalEmployeeID: member.PrincipalID,
			DisplayName:       displayName,
			ProviderType:      providerType,
			CanPlan:           true,
			CanDispatch:       false,
		}
		if providerType == "" {
			item.CanPlan = false
			item.ReasonCode = "provider_type_missing"
			item.ReasonMessage = "digital employee provider type is missing"
		} else {
			requiredProviders = append(requiredProviders, providerType)
			if code, message, blocked := employeeDispatchBlockReason(record); blocked {
				item.ReasonCode = code
				item.ReasonMessage = message
			}
		}
		readiness = append(readiness, item)
	}
	return readiness, normalizeStringSet(requiredProviders), nil
}

func (s *Service) findRuntimeNode(ctx context.Context, tenantID, runtimeNodeID uuid.UUID) (runtimepkg.NodeRecord, bool, error) {
	nodes, err := s.runtimeNodes.ListRuntimeNodesForTenant(ctx, runtimepkg.ListRuntimeNodesForTenantParams{
		TenantID: tenantID,
		Limit:    500,
	})
	if err != nil {
		return runtimepkg.NodeRecord{}, false, err
	}
	for _, node := range nodes {
		if node.ID == runtimeNodeID && node.TenantID == tenantID {
			return node, true, nil
		}
	}
	return runtimepkg.NodeRecord{}, false, nil
}

func (s *Service) requireRuntimeNodeForTenant(ctx context.Context, tenantID, runtimeNodeID uuid.UUID) error {
	if s.runtimeNodes == nil {
		return ErrProjectNotFound
	}
	_, found, err := s.findRuntimeNode(ctx, tenantID, runtimeNodeID)
	if err != nil {
		return err
	}
	if !found {
		return ErrProjectNotFound
	}
	return nil
}

func availableProviderCapabilities(capabilities []runtimepkg.RuntimeCapability, supportedProviders []byte) []string {
	providers := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		if strings.TrimSpace(capability.ProviderType) == "" || !capability.Available {
			continue
		}
		if !runtimeCapabilityProviderReady(capability.Status, capability.HealthStatus) {
			continue
		}
		providers = append(providers, capability.ProviderType)
	}
	if len(capabilities) == 0 && len(supportedProviders) > 0 {
		var decoded []string
		if err := json.Unmarshal(supportedProviders, &decoded); err == nil {
			providers = append(providers, decoded...)
		}
	}
	return normalizeStringSet(providers)
}

func runtimeCapabilityProviderReady(status, healthStatus string) bool {
	status = strings.TrimSpace(status)
	healthStatus = strings.TrimSpace(healthStatus)
	if status != "" && status != "available" && status != "healthy" {
		return false
	}
	if healthStatus != "" && healthStatus != "healthy" {
		return false
	}
	return true
}

func missingProviderTypes(required, available []string) []string {
	availableSet := map[string]struct{}{}
	for _, provider := range available {
		availableSet[provider] = struct{}{}
	}
	missing := make([]string, 0)
	for _, provider := range required {
		if _, ok := availableSet[provider]; !ok {
			missing = append(missing, provider)
		}
	}
	return missing
}

func markEmployeesNotDispatchable(employees []ProjectEmployeeReadiness, code, message string) {
	for i := range employees {
		employees[i].CanDispatch = false
		if employees[i].ReasonCode == "" {
			employees[i].ReasonCode = code
			employees[i].ReasonMessage = message
		}
	}
}

func hasEmployeeDispatchBlock(employees []ProjectEmployeeReadiness) bool {
	for _, employee := range employees {
		if !employee.CanDispatch && employee.ReasonCode != "" {
			return true
		}
	}
	return false
}

func employeeDispatchBlockProjectReason(employees []ProjectEmployeeReadiness) (string, ProjectReadinessAction) {
	for _, employee := range employees {
		if !employee.CanDispatch && employee.ReasonCode == "provider_type_missing" {
			return "provider_type_missing", ProjectReadinessAction{Code: "configure_employee_provider", Label: "Configure employee provider"}
		}
	}
	return "employee_workspace_pending", ProjectReadinessAction{Code: "prepare_employee_workspace", Label: "Prepare employee workspace"}
}

func employeeDispatchBlockReason(record DigitalEmployeePlanningProfileSourceRecord) (string, string, bool) {
	executionStatus := strings.TrimSpace(record.ExecutionStatus)
	if executionStatus != "" && executionStatus != "ready" && executionStatus != "active" {
		return "employee_workspace_pending", "employee execution workspace or provider is not ready", true
	}
	return "", "", false
}

func stringInSlice(value string, values []string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func (s *Service) ArchiveProject(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID) (*Project, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil || actorUserID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	project, err := s.repository.ArchiveProject(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	if _, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: ProjectEventArchived,
		ActorType: "human_user",
		ActorID:   actorUserID.String(),
		Summary:   "项目已归档",
		Payload:   map[string]any{"status": string(project.Status)},
	}); err != nil {
		return nil, err
	}
	return &project, nil
}

func (s *Service) GetProjectDeletePreview(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectDeletePreview, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	project, err := s.repository.GetProjectForDelete(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	blockers, err := s.repository.ListProjectDeleteBlockers(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	warnings, err := s.repository.GetProjectDeletePreviewCounts(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	canDelete := len(blockers) == 0
	return &ProjectDeletePreview{
		ProjectID:   projectID,
		ProjectName: project.Name,
		CanDelete:   canDelete,
		Blockers:    append([]ProjectDeleteBlocker(nil), blockers...),
		Warnings:    warnings,
		Message:     projectDeletePreviewMessage(canDelete),
	}, nil
}

func (s *Service) DeleteProject(ctx context.Context, req DeleteProjectRequest) error {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ActorUserID == uuid.Nil {
		return ErrInvalidProject
	}
	project, err := s.repository.GetProjectForDelete(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return err
	}
	blockers, err := s.repository.ListProjectDeleteBlockers(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return err
	}
	if len(blockers) > 0 {
		return &ProjectDeleteBlockedError{Blockers: append([]ProjectDeleteBlocker(nil), blockers...)}
	}
	if err := s.coordinator.TerminateProjectCoordinator(ctx, TerminateProjectCoordinatorSignal{
		TenantID:   req.TenantID,
		ProjectID:  req.ProjectID,
		WorkflowID: project.CoordinationWorkflowID,
		Reason:     "project deleted",
	}); err != nil {
		return err
	}
	deletedAt := time.Now().UTC()
	_, err = s.repository.SoftDeleteProjectCascade(ctx, SoftDeleteProjectCascadeParams{
		TenantID:    req.TenantID,
		ProjectID:   req.ProjectID,
		DeletedAt:   deletedAt,
		ActorUserID: req.ActorUserID,
		Project:     project,
	})
	return err
}

func projectDeletePreviewMessage(canDelete bool) string {
	message := "删除将取消待审批并解除成员与 Runtime 绑定。"
	if !canDelete {
		message += "存在活跃执行时不可删除。"
	}
	return message
}

func (s *Service) ReplaceProjectMembers(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID, members []ProjectMemberInput) ([]ProjectMember, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil || actorUserID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	if err := validateMembers(members); err != nil {
		return nil, err
	}
	replaced, err := s.repository.ReplaceProjectMembers(ctx, tenantID, projectID, members)
	if err != nil {
		return nil, err
	}
	event, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: ProjectEventConfigChanged,
		ActorType: "human_user",
		ActorID:   actorUserID.String(),
		Summary:   "项目成员已更新",
		Payload:   map[string]any{"member_count": len(replaced)},
	})
	if err != nil {
		return nil, err
	}
	project, err := s.repository.GetProject(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	changedMemberIDs := make([]uuid.UUID, 0, len(replaced))
	for _, member := range replaced {
		changedMemberIDs = append(changedMemberIDs, member.ID)
	}
	if err := s.coordinator.SignalProjectMemberChanged(ctx, ProjectMemberChangedSignal{
		TenantID:         tenantID,
		ProjectID:        projectID,
		ChangedMemberIDs: changedMemberIDs,
		ChangedEventID:   event.ID,
		WorkflowID:       project.CoordinationWorkflowID,
	}); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, tenantID, projectID, "ProjectMemberChanged", "failed", err, map[string]any{
			"changed_event_id":     event.ID.String(),
			"changed_member_ids":   uuidStrings(changedMemberIDs),
			"changed_member_count": len(changedMemberIDs),
		})
		return nil, err
	}
	return replaced, nil
}

func (s *Service) ListProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectMember, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	return s.repository.ListProjectMembers(ctx, tenantID, projectID)
}

func (s *Service) ListProjectTasks(ctx context.Context, tenantID, projectID uuid.UUID, status *string, limit, offset int32) ([]ProjectTask, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListProjectTasks(ctx, tenantID, projectID, status, limit, offset)
}

func (s *Service) ListProjectTaskLiveness(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectTaskLiveness, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	tasks, err := s.repository.ListProjectTasks(ctx, tenantID, projectID, nil, 1000, 0)
	if err != nil {
		return nil, err
	}
	taskIDs := make([]uuid.UUID, 0, len(tasks))
	for _, task := range tasks {
		taskIDs = append(taskIDs, task.ID)
	}
	unresolvedByTask := map[uuid.UUID][]uuid.UUID{}
	if len(taskIDs) > 0 {
		readiness, err := s.repository.ListUnresolvedBlockersForTasks(ctx, tenantID, projectID, taskIDs)
		if err != nil {
			return nil, err
		}
		for _, item := range readiness {
			unresolvedByTask[item.DependentTaskID] = append(unresolvedByTask[item.DependentTaskID], item.BlockerTaskID)
		}
	}
	now := time.Now().UTC()
	items := make([]ProjectTaskLiveness, 0, len(tasks))
	for _, task := range tasks {
		item := ProjectTaskLiveness{
			ProjectTaskID:         task.ID,
			BlockingDependencyIDs: append([]uuid.UUID(nil), unresolvedByTask[task.ID]...),
			CurrentAttemptID:      task.CurrentAttemptID,
			WaitingRequestID:      task.WaitingRequestID,
			RetryNotBefore:        task.RetryNotBefore,
		}
		if task.CurrentAttemptID != nil {
			attempt, err := s.repository.GetCurrentProjectTaskAttempt(ctx, tenantID, task.ID)
			if err != nil && !errors.Is(err, ErrProjectNotFound) {
				return nil, err
			}
			if err == nil {
				item.AttemptStatus = attempt.Status
				item.LeaseExpiresAt = attempt.LeaseExpiresAt
			}
		}
		classifyProjectTaskLiveness(&item, task, now)
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) ListProjectEvents(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectEvent, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListProjectEvents(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) CreateEvidenceRef(ctx context.Context, req CreateEvidenceRefServiceRequest) (*ProjectEvidenceRef, error) {
	req.ActorType = strings.TrimSpace(req.ActorType)
	req.EvidenceType = strings.TrimSpace(req.EvidenceType)
	req.Title = strings.TrimSpace(req.Title)
	req.Summary = strings.TrimSpace(req.Summary)
	req.SourceType = strings.TrimSpace(req.SourceType)
	req.SourceRef = strings.TrimSpace(req.SourceRef)
	req.SubmittedByType = strings.TrimSpace(req.SubmittedByType)
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ActorID == uuid.Nil || req.ActorType == "" || req.EvidenceType == "" || req.Title == "" || req.SourceType == "" || req.SourceRef == "" || req.SubmittedByType == "" || req.SubmittedByID == nil || *req.SubmittedByID == uuid.Nil {
		return nil, ErrInvalidProjectEvidence
	}
	project, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	if projectArchived(project) {
		return nil, ErrProjectArchived
	}
	result, err := s.repository.CreateEvidenceRefWithEvent(ctx, CreateEvidenceRefWithEventRequest{
		Event: AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    req.ProjectID,
			EventType:    ProjectEventEvidenceLinked,
			ActorType:    req.ActorType,
			ActorID:      req.ActorID.String(),
			ResourceType: strPtr("project_evidence_ref"),
			Summary:      "项目证据已关联",
			Payload: map[string]any{
				"evidence_type": req.EvidenceType,
				"title":         req.Title,
				"source_type":   req.SourceType,
			},
		},
		Evidence: CreateEvidenceRefRequest{
			TenantID:           req.TenantID,
			ProjectID:          req.ProjectID,
			ProjectTaskID:      req.ProjectTaskID,
			RouteDecisionID:    req.RouteDecisionID,
			ExecutionSummaryID: req.ExecutionSummaryID,
			EvidenceType:       req.EvidenceType,
			Title:              req.Title,
			Summary:            req.Summary,
			SourceType:         req.SourceType,
			SourceRef:          req.SourceRef,
			ArtifactRefID:      req.ArtifactRefID,
			SubmittedByType:    req.SubmittedByType,
			SubmittedByID:      req.SubmittedByID,
			VerificationStatus: EvidenceVerificationStatusSubmitted,
			Metadata:           mapOrEmptyAny(req.Metadata),
		},
	})
	if err != nil {
		return nil, err
	}
	return &result.Evidence, nil
}

func (s *Service) ListEvidenceRefs(ctx context.Context, tenantID, projectID uuid.UUID, status *EvidenceVerificationStatus, limit, offset int32) ([]ProjectEvidenceRef, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListEvidenceRefs(ctx, tenantID, projectID, status, limit, offset)
}

type PatchEvidenceRequest struct {
	TenantID           uuid.UUID
	ProjectID          uuid.UUID
	EvidenceID         uuid.UUID
	ActorUserID        uuid.UUID
	VerificationStatus EvidenceVerificationStatus
	Metadata           map[string]any
}

func (s *Service) ListEvidence(ctx context.Context, tenantID, projectID uuid.UUID, status *EvidenceVerificationStatus, limit, offset int32) ([]ProjectEvidenceRef, error) {
	return s.ListEvidenceRefs(ctx, tenantID, projectID, status, limit, offset)
}

func (s *Service) CreateEvidence(ctx context.Context, req CreateEvidenceRefServiceRequest) (*ProjectEvidenceRef, error) {
	return s.CreateEvidenceRef(ctx, req)
}

func (s *Service) PatchEvidence(ctx context.Context, req PatchEvidenceRequest) (*ProjectEvidenceRef, error) {
	req.VerificationStatus = EvidenceVerificationStatus(strings.TrimSpace(string(req.VerificationStatus)))
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.EvidenceID == uuid.Nil || req.ActorUserID == uuid.Nil || !validEvidenceVerificationStatus(req.VerificationStatus) {
		return nil, ErrInvalidProjectEvidence
	}
	project, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	if projectArchived(project) {
		return nil, ErrProjectArchived
	}
	result, err := s.repository.UpdateEvidenceVerificationStatusWithEvent(ctx, UpdateEvidenceVerificationStatusWithEventRequest{
		Event: AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    req.ProjectID,
			EventType:    ProjectEventEvidenceVerified,
			ActorType:    "human_user",
			ActorID:      req.ActorUserID.String(),
			ResourceType: strPtr("project_evidence_ref"),
			ResourceID:   strPtr(req.EvidenceID.String()),
			Summary:      "项目证据校验状态已更新",
			Payload: map[string]any{
				"verification_status": string(req.VerificationStatus),
			},
		},
		Evidence: UpdateEvidenceVerificationStatusRequest{
			TenantID:           req.TenantID,
			ProjectID:          req.ProjectID,
			ID:                 req.EvidenceID,
			VerificationStatus: req.VerificationStatus,
			Metadata:           req.Metadata,
		},
	})
	if err != nil {
		return nil, err
	}
	return &result.Evidence, nil
}

func (s *Service) ListArtifactRefs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArtifactRef, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListArtifactRefs(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) ListArtifacts(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArtifactRef, error) {
	return s.ListArtifactRefs(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) ListReportRefs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectReportRef, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListReportRefs(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) ListReports(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectReportRef, error) {
	return s.ListReportRefs(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) ListBudgetLedger(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectBudgetLedgerEntry, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListBudgetLedger(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) GetBudgetSummary(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectBudgetSummary, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	if _, err := s.repository.GetProject(ctx, tenantID, projectID); err != nil {
		return nil, err
	}
	summary, err := s.repository.GetBudgetSummary(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func (s *Service) CreateAcceptanceRecord(ctx context.Context, req CreateAcceptanceServiceRequest) (*ProjectAcceptanceRecord, error) {
	req.Status = strings.TrimSpace(req.Status)
	req.Conclusion = strings.TrimSpace(req.Conclusion)
	req.Summary = strings.TrimSpace(req.Summary)
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.AcceptedByUserID == uuid.Nil || req.Conclusion == "" || !validAcceptanceStatus(req.Status) {
		return nil, ErrInvalidProjectAcceptance
	}
	project, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	if projectArchived(project) {
		return nil, ErrProjectArchived
	}
	if req.AcceptedByUserID != project.HumanOwnerUserID && (project.AcceptanceUserID == nil || req.AcceptedByUserID != *project.AcceptanceUserID) {
		return nil, ErrInvalidProjectAcceptance
	}
	if req.Status == "accepted" && (len(req.EvidenceRefIDs) == 0 || len(req.ReportRefIDs) == 0) {
		return nil, ErrInvalidProjectAcceptance
	}
	if req.Status == "accepted" {
		if err := s.validateAcceptanceRefs(ctx, req.TenantID, req.ProjectID, req.EvidenceRefIDs, req.ReportRefIDs); err != nil {
			return nil, err
		}
	}
	result, err := s.repository.CreateAcceptanceRecordWithEvent(ctx, CreateAcceptanceRecordWithEventRequest{
		Event: AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    req.ProjectID,
			EventType:    ProjectEventAcceptanceSubmitted,
			ActorType:    "human_user",
			ActorID:      req.AcceptedByUserID.String(),
			ResourceType: strPtr("project_acceptance_record"),
			Summary:      "项目验收结论已提交",
			Payload: map[string]any{
				"status":             req.Status,
				"evidence_ref_count": len(req.EvidenceRefIDs),
				"report_ref_count":   len(req.ReportRefIDs),
			},
		},
		Acceptance: CreateAcceptanceRecordRequest{
			TenantID:         req.TenantID,
			ProjectID:        req.ProjectID,
			AcceptedByUserID: req.AcceptedByUserID,
			Status:           req.Status,
			Conclusion:       req.Conclusion,
			Summary:          req.Summary,
			EvidenceRefIDs:   req.EvidenceRefIDs,
			ReportRefIDs:     req.ReportRefIDs,
			UnresolvedRisks:  sliceOrEmptyAny(req.UnresolvedRisks),
		},
	})
	if err != nil {
		return nil, err
	}
	return &result.Acceptance, nil
}

func (s *Service) CreateAcceptance(ctx context.Context, req CreateAcceptanceServiceRequest) (*ProjectAcceptanceRecord, error) {
	return s.CreateAcceptanceRecord(ctx, req)
}

func (s *Service) GetAcceptance(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectAcceptanceRecord, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	record, err := s.repository.GetLatestAcceptanceRecord(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *Service) BuildArchivePreview(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectArchivePreview, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	project, err := s.repository.GetProject(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	pageSize, _ := normalizePagination(100, 0)
	evidenceRefs, err := collectArchivePreviewPages(ctx, pageSize, func(limit, offset int32) ([]ProjectEvidenceRef, error) {
		return s.repository.ListEvidenceRefs(ctx, tenantID, projectID, nil, limit, offset)
	})
	if err != nil {
		return nil, err
	}
	artifactRefs, err := collectArchivePreviewPages(ctx, pageSize, func(limit, offset int32) ([]ProjectArtifactRef, error) {
		return s.repository.ListArtifactRefs(ctx, tenantID, projectID, limit, offset)
	})
	if err != nil {
		return nil, err
	}
	reportRefs, err := collectArchivePreviewPages(ctx, pageSize, func(limit, offset int32) ([]ProjectReportRef, error) {
		return s.repository.ListReportRefs(ctx, tenantID, projectID, limit, offset)
	})
	if err != nil {
		return nil, err
	}
	budgetSummary, err := s.repository.GetBudgetSummary(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	blockedReasons := make([]any, 0)
	if len(reportRefs) == 0 {
		blockedReasons = append(blockedReasons, "missing_final_report")
	}
	if len(evidenceRefs) == 0 {
		blockedReasons = append(blockedReasons, "missing_evidence")
	}
	retentionPending := false
	estimatedObjectRefs := make([]any, 0, len(artifactRefs)+len(reportRefs))
	for _, artifact := range artifactRefs {
		if strings.TrimSpace(artifact.ObjectRef) != "" {
			estimatedObjectRefs = append(estimatedObjectRefs, artifact.ObjectRef)
		}
		if artifact.RetentionStatus == "" || artifact.RetentionStatus == "pending" || artifact.RetentionStatus == "retention_pending" {
			retentionPending = true
		}
	}
	for _, report := range reportRefs {
		if strings.TrimSpace(report.ObjectRef) != "" {
			estimatedObjectRefs = append(estimatedObjectRefs, report.ObjectRef)
		}
	}
	if projectArchived(project) {
		blockedReasons = append(blockedReasons, "project_already_archived")
	}
	if budgetSummary.LedgerCount > 0 {
		estimatedObjectRefs = append(estimatedObjectRefs, map[string]any{
			"budget_ledger_count": budgetSummary.LedgerCount,
			"actual_cost":         budgetSummary.ActualCost,
		})
	}
	return &ProjectArchivePreview{
		ProjectID:           projectID,
		EvidenceCount:       int64(len(evidenceRefs)),
		ArtifactCount:       int64(len(artifactRefs)),
		ReportCount:         int64(len(reportRefs)),
		RetentionPending:    retentionPending,
		BlockedReasons:      blockedReasons,
		EstimatedObjectRefs: estimatedObjectRefs,
	}, nil
}

func (s *Service) GetArchivePreview(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectArchivePreview, error) {
	return s.BuildArchivePreview(ctx, tenantID, projectID)
}

func (s *Service) CreateArchiveSnapshot(ctx context.Context, req CreateArchiveSnapshotServiceRequest) (*ProjectArchiveSnapshot, error) {
	req.SnapshotType = strings.TrimSpace(req.SnapshotType)
	req.Summary = strings.TrimSpace(req.Summary)
	req.ObjectRef = strings.TrimSpace(req.ObjectRef)
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.CreatedByUserID == uuid.Nil || req.SnapshotType == "" {
		return nil, ErrInvalidProject
	}
	project, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	if projectArchived(project) {
		return nil, ErrProjectArchived
	}
	preview, err := s.BuildArchivePreview(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	artifactIDs, err := s.collectArchiveArtifactIDs(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}

	status := "archived"
	retainedArtifactIDs := []uuid.UUID(nil)
	var retentionLockEventID *uuid.UUID
	if len(artifactIDs) > 0 {
		if s.archiveArtifactLocker == nil {
			status = "archive_pending_retention"
		} else {
			lockResult, lockErr := s.archiveArtifactLocker.LockProjectArtifacts(ctx, req.TenantID, req.ProjectID, artifactIDs)
			if lockErr != nil {
				status = "archive_pending_retention"
				retainedArtifactIDs = lockResult.ArtifactIDs
				retentionLockEventID = lockResult.EventID
			} else {
				retainedArtifactIDs = lockResult.ArtifactIDs
				retentionLockEventID = lockResult.EventID
			}
		}
	}

	includedCounts := archiveSnapshotIncludedCounts(preview)
	snapshotReq := CreateArchiveSnapshotWithEventRequest{
		Event: AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    req.ProjectID,
			EventType:    ProjectEventArchiveSnapshotCreated,
			ActorType:    "human_user",
			ActorID:      req.CreatedByUserID.String(),
			ResourceType: strPtr("project_archive_snapshot"),
			Summary:      "项目归档快照已创建",
			Payload: map[string]any{
				"snapshot_type": req.SnapshotType,
				"status":        status,
				"included_counts": map[string]any{
					"evidence": preview.EvidenceCount,
					"artifact": preview.ArtifactCount,
					"report":   preview.ReportCount,
				},
			},
		},
		Snapshot: CreateArchiveSnapshotRequest{
			TenantID:             req.TenantID,
			ProjectID:            req.ProjectID,
			SnapshotType:         req.SnapshotType,
			Status:               status,
			ObjectRef:            req.ObjectRef,
			Summary:              req.Summary,
			IncludedCounts:       includedCounts,
			RetainedArtifactIDs:  retainedArtifactIDs,
			RetentionLockEventID: retentionLockEventID,
			CreatedByUserID:      req.CreatedByUserID,
		},
	}
	var result ProjectArchiveSnapshotWriteResult
	if status == "archived" {
		result, err = s.repository.CreateArchiveSnapshotWithEventAndArchiveProject(ctx, snapshotReq)
	} else {
		result, err = s.repository.CreateArchiveSnapshotWithEvent(ctx, snapshotReq)
	}
	if err != nil {
		return nil, err
	}
	return &result.Snapshot, nil
}

func (s *Service) ListArchiveSnapshots(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArchiveSnapshot, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListArchiveSnapshots(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) ListConfigRevisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectConfigRevision, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListConfigRevisions(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) GetConfigRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (*ProjectConfigRevision, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil || revisionID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	revision, err := s.repository.GetConfigRevision(ctx, tenantID, projectID, revisionID)
	if err != nil {
		return nil, err
	}
	return &revision, nil
}

func (s *Service) SubmitDemand(ctx context.Context, req SubmitProjectDemandRequest) (*ProjectDemand, error) {
	req.Title = strings.TrimSpace(req.Title)
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.SubmittedByUserID == uuid.Nil || req.Title == "" {
		return nil, ErrInvalidProject
	}
	if req.CoordinationMode == "" {
		req.CoordinationMode = CoordinationModePlan
	}
	if req.CoordinationMode != CoordinationModePlan && req.CoordinationMode != CoordinationModeLoop {
		return nil, ErrInvalidCoordinationMode
	}
	if req.SourceType == "" {
		req.SourceType = DemandSourceManual
	}
	if req.ScenarioTemplateKey != nil {
		key := strings.TrimSpace(*req.ScenarioTemplateKey)
		if key == "" {
			req.ScenarioTemplateKey = nil
		} else if s.scenarioTemplates != nil {
			binding, err := s.scenarioTemplates.ResolveScenarioTemplate(ctx, req.TenantID, key)
			if err != nil {
				return nil, fmt.Errorf("scenario template %q: %w", key, ErrInvalidProject)
			}
			if binding.Status != "active" {
				return nil, fmt.Errorf("scenario template %q is %s: %w", key, binding.Status, ErrInvalidProject)
			}
			req.ScenarioTemplateKey = &key
		}
	}
	project, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	if project.Status == ProjectStatusArchived || project.ArchivedAt != nil {
		return nil, ErrProjectArchived
	}
	preference, reviewerSourceRefs, err := s.resolveDemandReviewer(ctx, req, project)
	if err != nil {
		return nil, err
	}
	req.SourceRefs = mergeReviewerSourceRefs(req.SourceRefs, reviewerSourceRefs)

	event, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  req.TenantID,
		ProjectID: req.ProjectID,
		EventType: ProjectEventDemandSubmitted,
		ActorType: "human_user",
		ActorID:   req.SubmittedByUserID.String(),
		Summary:   "需求已提交到当前项目",
		Payload: map[string]any{
			"title":                     req.Title,
			"reviewer_user_id":          preference.ReviewerUserID.String(),
			"reviewer_selection_reason": string(preference.SelectionReason),
		},
	})
	if err != nil {
		return nil, err
	}
	demand, err := s.repository.CreateProjectDemand(ctx, req, ProjectDemandStatusPlanningPending, &event.ID)
	if err != nil {
		return nil, err
	}
	demand.ReviewerPreference = preference
	if err := s.ensureProjectCoordinator(ctx, project); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, req.ProjectID, "DemandSubmitted", "failed", err, map[string]any{
			"demand_id":        demand.ID.String(),
			"created_event_id": event.ID.String(),
		})
		return nil, err
	}
	if err := s.coordinator.SignalDemandSubmitted(ctx, DemandSubmittedSignal{
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		DemandID:          demand.ID,
		SubmittedByUserID: req.SubmittedByUserID,
		CreatedEventID:    event.ID,
		WorkflowID:        project.CoordinationWorkflowID,
	}); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, req.ProjectID, "DemandSubmitted", "failed", err, map[string]any{
			"demand_id":        demand.ID.String(),
			"created_event_id": event.ID.String(),
		})
		return nil, err
	}
	return &demand, nil
}

func (s *Service) resolveDemandReviewer(ctx context.Context, req SubmitProjectDemandRequest, project Project) (*ReviewerPreference, map[string]any, error) {
	members, err := s.repository.ListProjectMembers(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, nil, err
	}
	selected, reason, resolvedFromRule, err := selectReviewer(req.ReviewerUserID, req.ReviewerSelectionReason, project, members)
	if err != nil {
		return nil, nil, err
	}
	preference := &ReviewerPreference{
		ReviewerUserID:   selected.PrincipalID,
		SelectionReason:  reason,
		DisplayName:      selected.DisplayNameSnapshot,
		ProjectRole:      selected.ProjectRole,
		ResolvedFromRule: resolvedFromRule,
	}
	sourceRefs := map[string]any{
		"reviewer_user_id":            preference.ReviewerUserID.String(),
		"reviewer_selection_reason":   string(preference.SelectionReason),
		"reviewer_project_role":       string(preference.ProjectRole),
		"reviewer_resolved_from_rule": preference.ResolvedFromRule,
	}
	if preference.DisplayName != nil {
		sourceRefs["reviewer_display_name"] = *preference.DisplayName
	}
	return preference, sourceRefs, nil
}

func selectReviewer(explicit *uuid.UUID, explicitReason ReviewerSelectionReason, project Project, members []ProjectMember) (ProjectMember, ReviewerSelectionReason, bool, error) {
	if explicit != nil {
		reason, err := normalizeReviewerSelectionReason(explicitReason)
		if err != nil {
			return ProjectMember{}, "", false, err
		}
		for _, member := range members {
			if member.PrincipalType == PrincipalTypeHumanUser && member.PrincipalID == *explicit && member.Status == "active" {
				return member, reason, false, nil
			}
		}
		return ProjectMember{}, "", false, ErrInvalidProjectMember
	}
	for _, member := range members {
		if member.PrincipalType == PrincipalTypeHumanUser && member.PrincipalID == project.HumanOwnerUserID && member.ProjectRole == ProjectRoleOwner && member.Status == "active" {
			return member, ReviewerSelectionProjectHumanOwnerFallback, true, nil
		}
	}
	return ProjectMember{}, "", false, ErrInvalidProjectMember
}

func normalizeReviewerSelectionReason(reason ReviewerSelectionReason) (ReviewerSelectionReason, error) {
	if reason == "" {
		return ReviewerSelectionUserSelected, nil
	}
	if isValidReviewerSelectionReason(reason) {
		return reason, nil
	}
	return "", ErrInvalidProjectMember
}

func isValidReviewerSelectionReason(reason ReviewerSelectionReason) bool {
	switch reason {
	case ReviewerSelectionProjectReviewerDefault, ReviewerSelectionProjectHumanOwnerFallback, ReviewerSelectionUserSelected:
		return true
	default:
		return false
	}
}

func mergeReviewerSourceRefs(sourceRefs map[string]any, reviewer map[string]any) map[string]any {
	merged := map[string]any{}
	for key, value := range sourceRefs {
		if strings.HasPrefix(key, "reviewer_") {
			continue
		}
		merged[key] = value
	}
	for key, value := range reviewer {
		merged[key] = value
	}
	return merged
}

func (s *Service) ListProjectDemands(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectDemand, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListProjectDemands(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) GetDemandLaunchDetail(ctx context.Context, tenantID, demandID uuid.UUID) (*DemandLaunchDetail, error) {
	if tenantID == uuid.Nil || demandID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	demand, err := s.repository.GetProjectDemand(ctx, tenantID, demandID)
	if err != nil {
		return nil, err
	}
	project, err := s.repository.GetProject(ctx, tenantID, demand.ProjectID)
	if err != nil {
		return nil, err
	}
	jobs, err := s.repository.ListDemandLaunchCoordinationJobs(ctx, tenantID, demand.ProjectID, demand.ID, demand.CreatedEventID, 100)
	if err != nil {
		return nil, err
	}
	routes, err := s.repository.ListDemandLaunchRouteDecisions(ctx, tenantID, demand.ProjectID, demand.ID, 100)
	if err != nil {
		return nil, err
	}
	tasks, err := s.repository.ListDemandLaunchProjectTasks(ctx, tenantID, demand.ProjectID, demand.ID, 100)
	if err != nil {
		return nil, err
	}
	taskIDs := projectTaskIDs(tasks)
	summaries, err := s.repository.ListExecutionSummariesByTaskIDs(ctx, tenantID, demand.ProjectID, taskIDs)
	if err != nil {
		return nil, err
	}
	decisions, err := s.repository.ListDemandLaunchDecisionRequests(ctx, tenantID, demand.ProjectID, coordinationJobIDs(jobs), taskIDs, 100)
	if err != nil {
		return nil, err
	}
	events, err := s.repository.ListDemandLaunchEvents(ctx, tenantID, demand.ProjectID, demand.ID, demand.CreatedEventID, taskIDs, decisionRequestIDs(decisions), 50)
	if err != nil {
		return nil, err
	}
	return &DemandLaunchDetail{
		Demand:             demand,
		Project:            project,
		Reviewer:           demand.ReviewerPreference,
		CoordinationJobs:   jobs,
		RouteDecisions:     routes,
		ProjectTasks:       tasks,
		ExecutionSummaries: summaries,
		DecisionRequests:   decisions,
		RecentEvents:       events,
	}, nil
}

// ListDemandAcceptanceCriteriaDetail builds the acceptance-panel read model for
// a demand: its snapshotted criteria (snapshot order) with each one's EFFECTIVE
// verdict resolved under the human-over-executor precedence rule, plus the
// result summaries of the tasks that satisfied it (anti-rubber-stamp evidence).
// Legacy demands with no open plan revision snapshot return an empty criteria
// list — the panel simply renders nothing.
func (s *Service) ListDemandAcceptanceCriteriaDetail(ctx context.Context, tenantID, demandID uuid.UUID) (*DemandAcceptanceCriteriaDetail, error) {
	if tenantID == uuid.Nil || demandID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	demand, err := s.repository.GetProjectDemand(ctx, tenantID, demandID)
	if err != nil {
		return nil, err
	}
	revisions, err := s.repository.ListPlanRevisionsForDemand(ctx, tenantID, demand.ProjectID, demandID)
	if err != nil {
		return nil, err
	}
	revisionID := CurrentEffectivePlanRevisionID(revisions)
	if revisionID == uuid.Nil {
		return &DemandAcceptanceCriteriaDetail{
			DemandID:     demandID,
			DemandStatus: demand.Status,
			Criteria:     []DemandAcceptanceCriterionDetail{},
		}, nil
	}
	criteria, err := s.repository.ListDemandAcceptanceCriteria(ctx, tenantID, demandID, revisionID)
	if err != nil {
		return nil, err
	}
	verdicts, err := s.repository.ListDemandCriterionVerdicts(ctx, tenantID, demandID, revisionID)
	if err != nil {
		return nil, err
	}

	// satisfied_by stores each satisfying task's planned_task_key (Task 4
	// decompose-time identity — e.g. "develop"), NOT a task UUID. Map keys to
	// the demand's real task UUIDs so task_summaries can surface each task's
	// conclusion (anti-rubber-stamp evidence); a direct UUID parse is kept as a
	// fallback for any legacy/tooling row that stored a UUID.
	demandTasks, err := s.repository.ListDemandLaunchProjectTasks(ctx, tenantID, demand.ProjectID, demandID, 500)
	if err != nil {
		return nil, err
	}
	taskIDByPlannedKey := make(map[string]uuid.UUID, len(demandTasks))
	for _, t := range demandTasks {
		if t.PlannedTaskKey == nil {
			continue
		}
		key := strings.TrimSpace(*t.PlannedTaskKey)
		if key == "" {
			continue
		}
		if _, exists := taskIDByPlannedKey[key]; !exists {
			taskIDByPlannedKey[key] = t.ID
		}
	}
	resolveSatisfiedByTaskID := func(raw string) (uuid.UUID, bool) {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return uuid.Nil, false
		}
		if id, ok := taskIDByPlannedKey[trimmed]; ok {
			return id, true
		}
		if id, parseErr := uuid.Parse(trimmed); parseErr == nil {
			return id, true
		}
		return uuid.Nil, false
	}

	// Gather the union of satisfied_by task IDs and fetch each one's latest
	// result conclusion in a single batched read.
	taskIDSet := make(map[uuid.UUID]struct{})
	taskIDs := make([]uuid.UUID, 0)
	for _, c := range criteria {
		for _, raw := range c.SatisfiedBy {
			taskID, ok := resolveSatisfiedByTaskID(raw)
			if !ok {
				continue
			}
			if _, seen := taskIDSet[taskID]; seen {
				continue
			}
			taskIDSet[taskID] = struct{}{}
			taskIDs = append(taskIDs, taskID)
		}
	}
	latestSummaryByTask := make(map[uuid.UUID]ExecutionSummary)
	if len(taskIDs) > 0 {
		summaries, summErr := s.repository.ListExecutionSummariesByTaskIDs(ctx, tenantID, demand.ProjectID, taskIDs)
		if summErr != nil {
			return nil, summErr
		}
		for _, summary := range summaries {
			existing, ok := latestSummaryByTask[summary.ProjectTaskID]
			if !ok || summary.CreatedAt.After(existing.CreatedAt) {
				latestSummaryByTask[summary.ProjectTaskID] = summary
			}
		}
	}

	details := make([]DemandAcceptanceCriterionDetail, 0, len(criteria))
	for _, c := range criteria {
		detail := DemandAcceptanceCriterionDetail{
			CriterionID:        c.CriterionID,
			Statement:          c.Statement,
			VerificationMethod: c.VerificationMethod,
			Severity:           c.Severity,
			SatisfiedBy:        append([]string(nil), c.SatisfiedBy...),
			EvidenceRefs:       []string{},
			TaskSummaries:      make([]DemandCriterionTaskSummary, 0, len(c.SatisfiedBy)),
		}
		if verdict, judgeType, evidenceRefs, hasVerdict := criterionEffectiveVerdict(verdicts, c.CriterionID); hasVerdict {
			v := verdict
			j := judgeType
			detail.Verdict = &v
			detail.JudgeType = &j
			if len(evidenceRefs) > 0 {
				detail.EvidenceRefs = append([]string(nil), evidenceRefs...)
			}
		}
		for _, raw := range c.SatisfiedBy {
			taskRef := strings.TrimSpace(raw)
			summaryText := ""
			taskIDText := taskRef
			if taskID, ok := resolveSatisfiedByTaskID(raw); ok {
				taskIDText = taskID.String()
				if summary, ok := latestSummaryByTask[taskID]; ok {
					summaryText = summary.Conclusion
				}
			}
			detail.TaskSummaries = append(detail.TaskSummaries, DemandCriterionTaskSummary{
				TaskID:  taskIDText,
				Summary: summaryText,
			})
		}
		details = append(details, detail)
	}

	return &DemandAcceptanceCriteriaDetail{
		DemandID:     demandID,
		DemandStatus: demand.Status,
		Criteria:     details,
	}, nil
}

func (s *Service) GetProjectTaskGraph(ctx context.Context, req GetProjectTaskGraphRequest) (*ProjectTaskGraph, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || (req.CoordinationJobID == nil && req.DemandID == nil) {
		return nil, ErrInvalidProject
	}
	graph, err := s.repository.GetProjectTaskGraph(ctx, req)
	if err != nil {
		return nil, err
	}
	normalizeProjectTaskGraph(&graph)
	if len(graph.Nodes) == 0 && req.DemandID != nil {
		facts, err := s.projectTaskGraphBlockingFacts(ctx, req)
		if err != nil {
			return nil, err
		}
		graph.BlockingFacts = facts
	}
	if err := s.enrichProjectTaskGraphEmployeeIdentities(ctx, req.TenantID, &graph); err != nil {
		return nil, err
	}
	return &graph, nil
}

func (s *Service) projectTaskGraphBlockingFacts(ctx context.Context, req GetProjectTaskGraphRequest) ([]ProjectTaskGraphBlockingFact, error) {
	if req.DemandID == nil {
		return []ProjectTaskGraphBlockingFact{}, nil
	}
	events, err := s.repository.ListDemandLaunchEvents(ctx, req.TenantID, req.ProjectID, *req.DemandID, nil, nil, nil, 50)
	if err != nil {
		return nil, err
	}
	facts := make([]ProjectTaskGraphBlockingFact, 0)
	for _, event := range events {
		if !taskGraphBlockingEventType(event.EventType) {
			continue
		}
		facts = append(facts, projectTaskGraphBlockingFactFromEvent(event))
	}
	return facts, nil
}

func taskGraphBlockingEventType(eventType ProjectEventType) bool {
	switch eventType {
	case ProjectEventCoordinationBlocked, ProjectEventWorkflowCoordinationFailed, ProjectEventTaskDispatchBlocked:
		return true
	default:
		return false
	}
}

func projectTaskGraphBlockingFactFromEvent(event ProjectEvent) ProjectTaskGraphBlockingFact {
	resourceType := ""
	if event.ResourceType != nil {
		resourceType = *event.ResourceType
	}
	resourceID := ""
	if event.ResourceID != nil {
		resourceID = *event.ResourceID
	}
	message := ""
	if event.Summary != nil {
		message = *event.Summary
	}
	reasonCode := stringPayload(event.Payload, "reason_code")
	if reasonCode == "" {
		reasonCode = "coordination_blocked"
	}
	fact := ProjectTaskGraphBlockingFact{
		ReasonCode:        reasonCode,
		Message:           message,
		ResourceType:      resourceType,
		ResourceID:        resourceID,
		RecommendedAction: stringPayload(event.Payload, "recommended_action"),
		CreatedAt:         event.CreatedAt,
		DecisionRequestID: stringPayload(event.Payload, "decision_request_id"),
	}
	if gapPayload := mapFromPayload(event.Payload, "gap"); len(gapPayload) > 0 {
		fact.Gap = &ProjectTaskGraphBlockingFactGap{
			ConstraintKind:       stringPayload(gapPayload, "constraint_kind"),
			Roles:                stringSlicePayload(gapPayload, "roles"),
			RequiredCapabilities: stringSlicePayload(gapPayload, "required_capabilities"),
			ActiveExecutorCount:  intPayload(gapPayload, "active_executor_count"),
			Options:              stringSlicePayload(gapPayload, "options"),
		}
	}
	return fact
}

// stringSlicePayload extracts a []string from a decoded JSON payload map, accepting
// both []string (set directly by Go callers in tests) and []any of strings (the
// shape after a jsonb column round trip). Non-string entries and blank strings are
// dropped. A missing/wrong-typed key returns an empty (non-nil) slice — not nil —
// so a Gap's Roles/RequiredCapabilities/Options always JSON-marshal as "[]", never
// "null", keeping the web's `gap.roles.join(...)`-style access safe without an
// extra null check.
func stringSlicePayload(payload map[string]any, key string) []string {
	switch raw := payload[key].(type) {
	case []string:
		values := make([]string, 0, len(raw))
		for _, value := range raw {
			if strings.TrimSpace(value) != "" {
				values = append(values, value)
			}
		}
		return values
	case []any:
		values := make([]string, 0, len(raw))
		for _, item := range raw {
			if value, ok := item.(string); ok && strings.TrimSpace(value) != "" {
				values = append(values, value)
			}
		}
		return values
	default:
		return []string{}
	}
}

// intPayload extracts an int from a decoded JSON payload map. Numeric payload
// values decode as float64 after a jsonb round trip, but Go test callers may set
// int/int64 directly; both are accepted. A missing/wrong-typed key returns 0.
func intPayload(payload map[string]any, key string) int {
	switch value := payload[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	default:
		return 0
	}
}

func (s *Service) enrichProjectTaskGraphEmployeeIdentities(ctx context.Context, tenantID uuid.UUID, graph *ProjectTaskGraph) error {
	if s.digitalEmployeeIdentities == nil {
		return nil
	}
	for i := range graph.Employees {
		identity, err := s.digitalEmployeeIdentities.GetDigitalEmployeeIdentity(ctx, tenantID, graph.Employees[i].DigitalEmployeeID)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			continue
		}
		graph.Employees[i].EmployeeRole = identity.Role
		graph.Employees[i].AvatarAsset = identity.AvatarAsset
	}
	return nil
}

func (s *Service) ListRouteDecisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]RouteDecision, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListRouteDecisions(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) ListPlanRevisions(ctx context.Context, req ListPlanRevisionsRequest) ([]PlanRevision, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	req.Limit, req.Offset = normalizePagination(req.Limit, req.Offset)
	return s.repository.ListPlanRevisions(ctx, req)
}

func (s *Service) ListPreDispatchGateResults(ctx context.Context, req ListPreDispatchGateResultsRequest) ([]PreDispatchGateResult, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ProjectTaskID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	req.Limit, req.Offset = normalizePagination(req.Limit, req.Offset)
	return s.repository.ListPreDispatchGateResults(ctx, req)
}

func (s *Service) GetPlanRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (*PlanRevision, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil || revisionID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	revision, err := s.repository.GetPlanRevision(ctx, tenantID, projectID, revisionID)
	if err != nil {
		return nil, err
	}
	return &revision, nil
}

func (s *Service) ListCoordinationJobs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]CoordinationJob, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListCoordinationJobs(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) ListDecisionRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]DecisionRequest, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListDecisionRequests(ctx, tenantID, projectID, limit, offset)
}

func coordinationJobIDs(jobs []CoordinationJob) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.ID)
	}
	return ids
}

func projectTaskIDs(tasks []ProjectTask) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

func decisionRequestIDs(decisions []DecisionRequest) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(decisions))
	for _, decision := range decisions {
		ids = append(ids, decision.ID)
	}
	return ids
}

func filterJobsForDemand(jobs []CoordinationJob, demand ProjectDemand) []CoordinationJob {
	filtered := []CoordinationJob{}
	for _, job := range jobs {
		if demand.CreatedEventID != nil && job.TriggerEventID != nil && *job.TriggerEventID == *demand.CreatedEventID {
			filtered = append(filtered, job)
			continue
		}
		if rawDemandID, ok := job.InputSnapshotRef["demand_id"].(string); ok && rawDemandID == demand.ID.String() {
			filtered = append(filtered, job)
		}
	}
	return filtered
}

func filterRoutesForDemand(routes []RouteDecision, demandID uuid.UUID) []RouteDecision {
	filtered := []RouteDecision{}
	for _, route := range routes {
		if route.DemandID != nil && *route.DemandID == demandID {
			filtered = append(filtered, route)
		}
	}
	return filtered
}

func filterTasksForDemand(tasks []ProjectTask, demandID uuid.UUID) []ProjectTask {
	filtered := []ProjectTask{}
	for _, task := range tasks {
		if task.DemandID != nil && *task.DemandID == demandID {
			filtered = append(filtered, task)
		}
	}
	return filtered
}

func filterDecisionsForDemand(decisions []DecisionRequest, jobs []CoordinationJob, tasks []ProjectTask) []DecisionRequest {
	jobIDs := map[uuid.UUID]struct{}{}
	for _, job := range jobs {
		jobIDs[job.ID] = struct{}{}
	}
	taskIDs := map[uuid.UUID]struct{}{}
	for _, task := range tasks {
		taskIDs[task.ID] = struct{}{}
	}
	filtered := []DecisionRequest{}
	for _, decision := range decisions {
		if decision.CoordinationJobID != nil {
			if _, ok := jobIDs[*decision.CoordinationJobID]; ok {
				filtered = append(filtered, decision)
				continue
			}
		}
		if decision.ProjectTaskID != nil {
			if _, ok := taskIDs[*decision.ProjectTaskID]; ok {
				filtered = append(filtered, decision)
			}
		}
	}
	return filtered
}

func filterEventsForDemand(events []ProjectEvent, demand ProjectDemand, tasks []ProjectTask, decisions []DecisionRequest) []ProjectEvent {
	taskIDs := map[string]struct{}{}
	for _, task := range tasks {
		taskIDs[task.ID.String()] = struct{}{}
	}
	decisionIDs := map[string]struct{}{}
	for _, decision := range decisions {
		decisionIDs[decision.ID.String()] = struct{}{}
	}
	filtered := []ProjectEvent{}
	for _, event := range events {
		if demand.CreatedEventID != nil && event.ID == *demand.CreatedEventID {
			filtered = append(filtered, event)
			continue
		}
		if event.ResourceID != nil {
			if *event.ResourceID == demand.ID.String() {
				filtered = append(filtered, event)
				continue
			}
			if _, ok := taskIDs[*event.ResourceID]; ok {
				filtered = append(filtered, event)
				continue
			}
			if _, ok := decisionIDs[*event.ResourceID]; ok {
				filtered = append(filtered, event)
				continue
			}
		}
		if rawDemandID, ok := event.Payload["demand_id"].(string); ok && rawDemandID == demand.ID.String() {
			filtered = append(filtered, event)
			continue
		}
		if rawProjectTaskID, ok := event.Payload["project_task_id"].(string); ok {
			if _, exists := taskIDs[rawProjectTaskID]; exists {
				filtered = append(filtered, event)
				continue
			}
		}
		if rawDecisionRequestID, ok := event.Payload["decision_request_id"].(string); ok {
			if _, exists := decisionIDs[rawDecisionRequestID]; exists {
				filtered = append(filtered, event)
			}
		}
	}
	return filtered
}

func normalizeProjectTaskGraph(graph *ProjectTaskGraph) {
	if graph.Nodes == nil {
		graph.Nodes = []ProjectTaskGraphNode{}
	}
	if graph.Edges == nil {
		graph.Edges = []ProjectTaskGraphEdge{}
	}
	if graph.Employees == nil {
		graph.Employees = []ProjectTaskGraphEmployee{}
	}
	if graph.Runs == nil {
		graph.Runs = []ProjectTaskGraphRun{}
	}
	if graph.ExecutionSummaries == nil {
		graph.ExecutionSummaries = []ExecutionSummary{}
	}
	if graph.RecentEvents == nil {
		graph.RecentEvents = []ProjectEvent{}
	}
	if graph.DecisionRequests == nil {
		graph.DecisionRequests = []DecisionRequest{}
	}
	if graph.StageSummaries == nil {
		graph.StageSummaries = buildProjectTaskGraphStageSummaries(graph.Nodes)
	}
	if graph.BlockingFacts == nil {
		graph.BlockingFacts = []ProjectTaskGraphBlockingFact{}
	}
}

func buildProjectTaskGraphStageSummaries(nodes []ProjectTaskGraphNode) []ProjectTaskGraphStageSummary {
	type mutableSummary struct {
		summary ProjectTaskGraphStageSummary
	}
	byStage := map[int32]*mutableSummary{}
	for _, node := range nodes {
		stage := int32(-1)
		if node.Task.StageIndex != nil {
			stage = *node.Task.StageIndex
		}
		entry := byStage[stage]
		if entry == nil {
			title := "未分阶段"
			if stage >= 0 {
				title = fmt.Sprintf("第 %d 阶段", stage)
			}
			entry = &mutableSummary{summary: ProjectTaskGraphStageSummary{StageIndex: stage, Title: title}}
			byStage[stage] = entry
		}
		entry.summary.TotalNodes++
		switch normalizeTaskStatusForSummary(node.Task.Status) {
		case "completed":
			entry.summary.CompletedNodes++
		case "running":
			entry.summary.RunningNodes++
		case "waiting_human":
			entry.summary.WaitingHumanNodes++
		case "blocked":
			entry.summary.BlockedNodes++
		}
	}
	stages := make([]int, 0, len(byStage))
	for stage := range byStage {
		stages = append(stages, int(stage))
	}
	sort.Ints(stages)
	result := make([]ProjectTaskGraphStageSummary, 0, len(stages))
	for _, stage := range stages {
		result = append(result, byStage[int32(stage)].summary)
	}
	return result
}

func normalizeTaskStatusForSummary(status string) string {
	switch strings.ToLower(status) {
	case "completed", "done", "success":
		return "completed"
	case "assigned", "running", "in_progress":
		return "running"
	case "waiting_human", "pending_review":
		return "waiting_human"
	case "blocked":
		return "blocked"
	default:
		return "other"
	}
}

func (s *Service) ListExecutionSummaries(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ExecutionSummary, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListExecutionSummaries(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) GetExecutionTrace(ctx context.Context, req GetExecutionTraceRequest) (*ProjectExecutionTrace, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	req.Limit, req.Offset = normalizePagination(req.Limit, req.Offset)
	if req.Limit < 100 {
		req.Limit = 100
	}
	attempts, err := s.repository.ListProjectTaskAttemptsForExecutionTrace(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	attempts = filterExecutionTraceAttempts(attempts, req)
	events, err := s.repository.ListProjectExecutionLedgerEvents(ctx, req)
	if err != nil {
		return nil, err
	}
	summaryEventType := ExecutionLedgerEventSummaryCreated
	summaryMappingReq := req
	summaryMappingReq.EventType = &summaryEventType
	summaryMappingReq.ErrorFamily = nil
	summaryMappingReq.Limit = 1000
	summaryMappingReq.Offset = 0
	summaryMappingEvents, err := s.repository.ListProjectExecutionLedgerEvents(ctx, summaryMappingReq)
	if err != nil {
		return nil, err
	}
	summaries, err := s.repository.ListExecutionSummaries(ctx, req.TenantID, req.ProjectID, 1000, 0)
	if err != nil {
		return nil, err
	}
	trace := buildProjectExecutionTrace(req.ProjectID, attempts, events, summaryMappingEvents, summaries)
	return &trace, nil
}

func filterExecutionTraceAttempts(attempts []ProjectTaskAttempt, req GetExecutionTraceRequest) []ProjectTaskAttempt {
	if req.ProjectTaskID == nil && req.ProjectTaskAttemptID == nil {
		return attempts
	}
	filtered := make([]ProjectTaskAttempt, 0, len(attempts))
	for _, attempt := range attempts {
		if req.ProjectTaskID != nil && attempt.ProjectTaskID != *req.ProjectTaskID {
			continue
		}
		if req.ProjectTaskAttemptID != nil && attempt.ID != *req.ProjectTaskAttemptID {
			continue
		}
		filtered = append(filtered, attempt)
	}
	return filtered
}

func buildProjectExecutionTrace(projectID uuid.UUID, attempts []ProjectTaskAttempt, visibleEvents []ExecutionLedgerEvent, summaryMappingEvents []ExecutionLedgerEvent, summaries []ExecutionSummary) ProjectExecutionTrace {
	trace := ProjectExecutionTrace{
		ProjectID: projectID,
		Attempts:  make([]ProjectExecutionTraceAttempt, 0, len(attempts)),
	}
	attemptIndexes := make(map[uuid.UUID]int, len(attempts))
	latestAttemptIndexByTaskID := make(map[uuid.UUID]int, len(attempts))
	for _, attempt := range attempts {
		attemptIndexes[attempt.ID] = len(trace.Attempts)
		trace.Attempts = append(trace.Attempts, ProjectExecutionTraceAttempt{
			ProjectTaskID:     attempt.ProjectTaskID,
			AttemptID:         attempt.ID,
			AttemptNo:         attempt.AttemptNo,
			Status:            attempt.Status,
			RuntimeNodeID:     attempt.RuntimeNodeID,
			ProviderType:      attempt.ProviderType,
			ProviderSessionID: attempt.ProviderSessionID,
			StartedAt:         attempt.StartedAt,
			FinishedAt:        attempt.FinishedAt,
			FailureFamily:     attempt.FailureFamily,
			Retryable:         attempt.Retryable,
			Events:            []ExecutionLedgerEvent{},
		})
		trace.Summary.AttemptCount++
		if isFailedExecutionTraceAttempt(attempt.Status) {
			trace.Summary.FailedAttemptCount++
		}
		latestIndex, ok := latestAttemptIndexByTaskID[attempt.ProjectTaskID]
		if !ok || executionTraceAttemptAfter(attempt, attempts[latestIndex]) {
			latestAttemptIndexByTaskID[attempt.ProjectTaskID] = attemptIndexes[attempt.ID]
		}
	}

	summaryByID := make(map[string]ExecutionSummary, len(summaries))
	latestSummaryByTaskID := make(map[uuid.UUID]ExecutionSummary, len(summaries))
	tasksWithMatchedSummaryEvent := make(map[uuid.UUID]bool, len(summaries))
	attachedSummaryIDs := make(map[uuid.UUID]bool, len(summaries))
	refCounter := newExecutionTraceRefCounter()
	for _, summary := range summaries {
		summaryByID[summary.ID.String()] = summary
		latest, ok := latestSummaryByTaskID[summary.ProjectTaskID]
		if !ok || summary.CreatedAt.After(latest.CreatedAt) {
			latestSummaryByTaskID[summary.ProjectTaskID] = summary
		}
	}

	for _, event := range summaryMappingEvents {
		if event.EventType != ExecutionLedgerEventSummaryCreated {
			continue
		}
		summary, ok := summaryByID[event.SourceID]
		if !ok {
			continue
		}
		tasksWithMatchedSummaryEvent[summary.ProjectTaskID] = true
		if event.ProjectTaskAttemptID == nil {
			continue
		}
		attemptIndex, attemptOK := attemptIndexes[*event.ProjectTaskAttemptID]
		if attemptOK && !attachedSummaryIDs[summary.ID] && trace.Attempts[attemptIndex].Summary == nil {
			attachExecutionTraceSummary(&trace, attemptIndex, summary, refCounter)
			attachedSummaryIDs[summary.ID] = true
		}
	}

	var latestErrorEvent *ExecutionLedgerEvent
	for _, event := range visibleEvents {
		if event.ErrorFamily != nil && (latestErrorEvent == nil || executionTraceEventAfter(event, *latestErrorEvent)) {
			latestEvent := event
			latestErrorEvent = &latestEvent
			errorFamily := *event.ErrorFamily
			trace.Summary.LatestErrorFamily = &errorFamily
		}
		if event.ProjectTaskAttemptID == nil {
			continue
		}
		attemptIndex, attemptOK := attemptIndexes[*event.ProjectTaskAttemptID]
		if !attemptOK {
			continue
		}
		clonedEvent := cloneExecutionLedgerEvent(event)
		trace.Attempts[attemptIndex].Events = append(trace.Attempts[attemptIndex].Events, clonedEvent)
		trace.Summary.ArtifactRefCount += refCounter.addArtifactRefs(clonedEvent.ArtifactRefs)
		trace.Summary.EvidenceRefCount += refCounter.addEvidenceRefs(clonedEvent.EvidenceRefs)
		if trace.Attempts[attemptIndex].ProviderType == nil && clonedEvent.ProviderType != nil {
			trace.Attempts[attemptIndex].ProviderType = clonedEvent.ProviderType
		}
	}

	for taskID, summary := range latestSummaryByTaskID {
		if tasksWithMatchedSummaryEvent[taskID] {
			continue
		}
		attemptIndex, ok := latestAttemptIndexByTaskID[taskID]
		if !ok || trace.Attempts[attemptIndex].Summary != nil || attachedSummaryIDs[summary.ID] {
			continue
		}
		attachExecutionTraceSummary(&trace, attemptIndex, summary, refCounter)
		attachedSummaryIDs[summary.ID] = true
	}
	return trace
}

func isFailedExecutionTraceAttempt(status string) bool {
	switch status {
	case ProjectTaskAttemptStatusFailed, ProjectTaskAttemptStatusLost, ProjectTaskAttemptStatusTimedOut:
		return true
	default:
		return false
	}
}

func executionTraceEventAfter(left, right ExecutionLedgerEvent) bool {
	if !left.OccurredAt.Equal(right.OccurredAt) {
		return left.OccurredAt.After(right.OccurredAt)
	}
	return left.CreatedAt.After(right.CreatedAt)
}

func executionTraceAttemptAfter(left, right ProjectTaskAttempt) bool {
	leftTime := executionTraceAttemptSortTime(left)
	rightTime := executionTraceAttemptSortTime(right)
	if !leftTime.Equal(rightTime) {
		return leftTime.After(rightTime)
	}
	if left.AttemptNo != right.AttemptNo {
		return left.AttemptNo > right.AttemptNo
	}
	return left.ID.String() > right.ID.String()
}

func executionTraceAttemptSortTime(attempt ProjectTaskAttempt) time.Time {
	if attempt.FinishedAt != nil {
		return *attempt.FinishedAt
	}
	if attempt.StartedAt != nil {
		return *attempt.StartedAt
	}
	return attempt.CreatedAt
}

type executionTraceRefCounter struct {
	artifactRefs map[string]struct{}
	evidenceRefs map[string]struct{}
}

func newExecutionTraceRefCounter() *executionTraceRefCounter {
	return &executionTraceRefCounter{
		artifactRefs: map[string]struct{}{},
		evidenceRefs: map[string]struct{}{},
	}
}

func (c *executionTraceRefCounter) addArtifactRefs(refs []any) int32 {
	return addExecutionTraceRefs(c.artifactRefs, refs)
}

func (c *executionTraceRefCounter) addEvidenceRefs(refs []any) int32 {
	return addExecutionTraceRefs(c.evidenceRefs, refs)
}

func addExecutionTraceRefs(seen map[string]struct{}, refs []any) int32 {
	var added int32
	for _, ref := range sliceOrEmptyAny(refs) {
		key := executionTraceRefKey(ref)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		added++
	}
	return added
}

func executionTraceRefKey(ref any) string {
	encoded, err := json.Marshal(ref)
	if err == nil {
		return "json:" + string(encoded)
	}
	return fmt.Sprintf("fmt:%#v", ref)
}

func attachExecutionTraceSummary(trace *ProjectExecutionTrace, attemptIndex int, summary ExecutionSummary, refCounter *executionTraceRefCounter) {
	if trace.Attempts[attemptIndex].Summary != nil {
		return
	}
	artifactRefs := sliceOrEmptyAny(summary.ArtifactRefs)
	evidenceRefs := sliceOrEmptyAny(summary.EvidenceRefs)
	trace.Attempts[attemptIndex].Summary = &ProjectExecutionTraceAttemptSummary{
		ExecutionSummaryID:  summary.ID,
		Conclusion:          summary.Conclusion,
		RequiresHumanReview: summary.RequiresHumanReview,
		ArtifactRefs:        append([]any(nil), artifactRefs...),
		EvidenceRefs:        append([]any(nil), evidenceRefs...),
		CreatedAt:           summary.CreatedAt,
	}
	if summary.RequiresHumanReview {
		trace.Summary.HumanReviewRequiredCount++
	}
	trace.Summary.ArtifactRefCount += refCounter.addArtifactRefs(artifactRefs)
	trace.Summary.EvidenceRefCount += refCounter.addEvidenceRefs(evidenceRefs)
}

func cloneExecutionLedgerEvent(event ExecutionLedgerEvent) ExecutionLedgerEvent {
	event.ArtifactRefs = append([]any(nil), sliceOrEmptyAny(event.ArtifactRefs)...)
	event.EvidenceRefs = append([]any(nil), sliceOrEmptyAny(event.EvidenceRefs)...)
	event.Metadata = cloneMap(mapOrEmptyAny(event.Metadata))
	return event
}

func (s *Service) ListTransferRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]TransferRequest, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	limit, offset = normalizePagination(limit, offset)
	return s.repository.ListTransferRequests(ctx, tenantID, projectID, limit, offset)
}

func (s *Service) CompleteProjectTask(ctx context.Context, req CompleteProjectTaskRequest) (*ExecutionSummary, error) {
	req.Conclusion = strings.TrimSpace(req.Conclusion)
	if req.TenantID == uuid.Nil || req.ProjectTaskID == uuid.Nil || req.DigitalEmployeeID == uuid.Nil || req.Conclusion == "" {
		return nil, ErrInvalidProject
	}
	task, projectRecord, err := s.taskAndProjectForWriteback(ctx, req.TenantID, req.RuntimeNodeID, req.ProjectTaskID, req.DigitalEmployeeID)
	if err != nil {
		return nil, err
	}
	runWorkProducts, err := s.projectTaskRunWorkProducts(ctx, req.TenantID, task)
	if err != nil {
		return nil, err
	}
	contract := validateProjectTaskCompletionContract(task, req, runWorkProducts)
	if !contract.Satisfied() {
		if err := s.appendProjectTaskContractMissingEvent(ctx, task, req, contract); err != nil {
			return nil, err
		}
		return nil, ErrInvalidProjectEvidence
	}
	writebackRepository, err := s.projectTaskWritebackRepository()
	if err != nil {
		return nil, err
	}
	result, err := writebackRepository.CompleteProjectTaskWriteback(ctx, CompleteProjectTaskWritebackRequest{
		Task: task,
		Summary: CreateExecutionSummaryRequest{
			TenantID:              req.TenantID,
			ProjectID:             task.ProjectID,
			ProjectTaskID:         task.ID,
			DigitalEmployeeID:     req.DigitalEmployeeID,
			Conclusion:            req.Conclusion,
			EvidenceRefs:          sliceOrEmptyAny(req.EvidenceRefs),
			ArtifactRefs:          sliceOrEmptyAny(req.ArtifactRefs),
			ConfidenceFactors:     mapOrEmptyAny(req.ConfidenceFactors),
			Uncertainty:           strings.TrimSpace(req.Uncertainty),
			MissingInformation:    sliceOrEmptyAny(req.MissingInformation),
			RecommendedNextAction: strings.TrimSpace(req.RecommendedNextAction),
			RequiresHumanReview:   req.RequiresHumanReview,
		},
		Event: AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    task.ProjectID,
			EventType:    ProjectEventTaskCompleted,
			ActorType:    "digital_employee",
			ActorID:      req.DigitalEmployeeID.String(),
			ResourceType: strPtr("project_task"),
			ResourceID:   strPtr(task.ID.String()),
			Summary:      "项目任务已完成",
			Payload: map[string]any{
				"project_task_id": task.ID.String(),
			},
		},
		AllowedCurrentStatuses: runtimeWritebackProjectTaskStatuses(),
	})
	if err != nil {
		return nil, err
	}
	// Materialize the task's structured evidence/artifacts into the project read
	// models so /evidence and /artifacts surface them. Best-effort: a materialization
	// failure must not roll back an already-completed task, so it is audited, not returned.
	if err := s.materializeTaskCompletionEvidence(ctx, task, req, result.Summary.ID); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, task.ProjectID, "EvidenceMaterialization", "failed", err, map[string]any{
			"project_task_id":      task.ID.String(),
			"execution_summary_id": result.Summary.ID.String(),
		})
	}
	if err := s.coordinator.SignalEmployeeTaskCompleted(ctx, EmployeeTaskCompletedSignal{
		TenantID:           req.TenantID,
		ProjectID:          task.ProjectID,
		ProjectTaskID:      task.ID,
		ExecutionSummaryID: result.Summary.ID,
		CompletedEventID:   result.Event.ID,
		WorkflowID:         projectRecord.CoordinationWorkflowID,
	}); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, task.ProjectID, "EmployeeTaskCompleted", "failed", err, map[string]any{
			"project_task_id":      task.ID.String(),
			"execution_summary_id": result.Summary.ID.String(),
			"completed_event_id":   result.Event.ID.String(),
		})
		return nil, err
	}
	return &result.Summary, nil
}

func (s *Service) StartProjectTaskAttempt(ctx context.Context, req StartProjectTaskAttemptRequest) (*ProjectTaskAttempt, error) {
	var lastErr error
	for attempt := 0; attempt < projectTaskAttemptStartReadinessAttempts; attempt++ {
		started, err := s.startProjectTaskAttemptOnce(ctx, req)
		if err == nil {
			return started, nil
		}
		lastErr = err
		if !errors.Is(err, ErrProjectConflict) && !errors.Is(err, ErrProjectNotFound) {
			return nil, err
		}
		if !s.projectTaskAttemptStartMayBeAheadOfQueue(ctx, req.ProjectTaskAttemptRuntimeRequest) {
			return nil, err
		}
		if attempt == projectTaskAttemptStartReadinessAttempts-1 {
			break
		}
		timer := time.NewTimer(projectTaskAttemptStartReadinessBackoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastErr
}

func (s *Service) projectTaskAttemptStartMayBeAheadOfQueue(ctx context.Context, req ProjectTaskAttemptRuntimeRequest) bool {
	task, err := s.repository.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
	if err != nil {
		return errors.Is(err, ErrProjectNotFound)
	}
	if task.CurrentAttemptID == nil {
		return task.Status == ProjectTaskStatusPlanned || task.Status == ProjectTaskStatusWaitingHuman
	}
	if *task.CurrentAttemptID != req.AttemptID {
		return task.Status == ProjectTaskStatusPlanned || task.Status == ProjectTaskStatusWaitingHuman
	}
	if !projectTaskAcceptsRuntimeWriteback(task.Status) {
		return false
	}
	attempt, err := s.repository.GetProjectTaskAttempt(ctx, req.TenantID, req.AttemptID)
	if err != nil {
		return errors.Is(err, ErrProjectNotFound)
	}
	if attempt.ProjectTaskID != req.ProjectTaskID || attempt.LeaseToken != req.LeaseToken {
		return false
	}
	if attempt.RuntimeNodeID != nil && *attempt.RuntimeNodeID != req.RuntimeNodeID {
		return false
	}
	return attempt.Status == ProjectTaskAttemptStatusQueued
}

func (s *Service) startProjectTaskAttemptOnce(ctx context.Context, req StartProjectTaskAttemptRequest) (*ProjectTaskAttempt, error) {
	if _, _, err := s.validateAttemptRuntimeRequest(ctx, req.ProjectTaskAttemptRuntimeRequest); err != nil {
		return nil, err
	}
	writebackRepository, err := s.projectTaskAttemptWritebackRepository()
	if err != nil {
		return nil, err
	}
	result, err := writebackRepository.StartProjectTaskAttemptWriteback(ctx, req)
	if err != nil {
		return nil, err
	}
	_, _ = s.repository.CreateExecutionLedgerEvent(ctx, CreateExecutionLedgerEventRequest{
		TenantID:             req.TenantID,
		ProjectID:            result.Task.ProjectID,
		ProjectTaskID:        &req.ProjectTaskID,
		ProjectTaskAttemptID: &req.AttemptID,
		EventType:            ExecutionLedgerEventAttemptStarted,
		SourceType:           "project_task_attempt",
		SourceID:             req.AttemptID.String(),
		ActorType:            "runtime_node",
		ActorID:              strPtr(req.RuntimeNodeID.String()),
		RuntimeNodeID:        &req.RuntimeNodeID,
		ProviderSessionID:    req.ProviderSessionID,
		InputSummary:         "Runtime started project task attempt",
		Metadata: map[string]any{
			"project_task_id": req.ProjectTaskID.String(),
			"idempotency_key": req.IdempotencyKey,
		},
		IdempotencyKey: "project_task_attempt:" + req.AttemptID.String() + ":attempt.started",
	})
	return &result.Attempt, nil
}

func (s *Service) RenewProjectTaskAttemptLease(ctx context.Context, req RenewProjectTaskAttemptLeaseRequest) error {
	if _, _, err := s.validateAttemptRuntimeRequest(ctx, req.ProjectTaskAttemptRuntimeRequest); err != nil {
		return err
	}
	writebackRepository, err := s.projectTaskAttemptWritebackRepository()
	if err != nil {
		return err
	}
	_, err = writebackRepository.RenewProjectTaskAttemptLeaseWriteback(ctx, req)
	return err
}

func (s *Service) CompleteProjectTaskAttempt(ctx context.Context, req CompleteProjectTaskAttemptRequest) (*ExecutionSummary, error) {
	if req.ResultContract != nil && req.ResultContract.Status != TaskResultStatusCompleted {
		return s.SubmitProjectTaskAttemptResult(ctx, SubmitProjectTaskAttemptResultRequest{
			ProjectTaskAttemptRuntimeRequest: req.ProjectTaskAttemptRuntimeRequest,
			ResultContract:                   *req.ResultContract,
		})
	}
	req.Conclusion = strings.TrimSpace(req.Conclusion)
	if req.ResultContract == nil && req.Conclusion == "" {
		return nil, ErrInvalidProject
	}
	task, _, err := s.validateAttemptRuntimeRequest(ctx, req.ProjectTaskAttemptRuntimeRequest)
	if err != nil {
		return nil, err
	}
	digitalEmployeeID, err := digitalEmployeeIDForProjectTask(task)
	if err != nil {
		return nil, err
	}
	req.DigitalEmployeeID = digitalEmployeeID
	resultContract := req.ResultContract
	if resultContract == nil {
		legacyContract := TaskResultContractFromLegacyCompletion(req)
		resultContract = &legacyContract
	} else {
		req.Conclusion = strings.TrimSpace(resultContract.Summary)
	}
	validation, err := s.validateTaskResultContractForAttempt(ctx, task, req.ProjectTaskAttemptRuntimeRequest, *resultContract)
	if err != nil {
		return nil, err
	}
	if !validation.Valid {
		s.recordHandoffUnfulfilledLedgerEvent(ctx, task, req.ProjectTaskAttemptRuntimeRequest, validation)
		return s.recordRejectedProjectTaskAttemptResultAndWaitHuman(ctx, task, req.ProjectTaskAttemptRuntimeRequest, *resultContract, validation)
	}
	enrichedContract := enrichContractWithHandoffVerification(task, *resultContract)
	resultContract = &enrichedContract
	if req.Conclusion == "" {
		return nil, ErrInvalidProject
	}
	projectRecord, err := s.repository.GetProject(ctx, req.TenantID, task.ProjectID)
	if err != nil {
		return nil, err
	}
	runWorkProducts, err := s.projectTaskRunWorkProducts(ctx, req.TenantID, task)
	if err != nil {
		return nil, err
	}
	contract := validateProjectTaskCompletionContract(task, CompleteProjectTaskRequest{
		TenantID:              req.TenantID,
		RuntimeNodeID:         req.RuntimeNodeID,
		ProjectTaskID:         req.ProjectTaskID,
		DigitalEmployeeID:     digitalEmployeeID,
		Conclusion:            req.Conclusion,
		EvidenceRefs:          req.EvidenceRefs,
		ArtifactRefs:          req.ArtifactRefs,
		ConfidenceFactors:     req.ConfidenceFactors,
		Uncertainty:           req.Uncertainty,
		MissingInformation:    req.MissingInformation,
		RecommendedNextAction: req.RecommendedNextAction,
		RequiresHumanReview:   req.RequiresHumanReview,
	}, runWorkProducts)
	if !contract.Satisfied() {
		if err := s.appendProjectTaskContractMissingEvent(ctx, task, CompleteProjectTaskRequest{
			TenantID:              req.TenantID,
			RuntimeNodeID:         req.RuntimeNodeID,
			ProjectTaskID:         req.ProjectTaskID,
			DigitalEmployeeID:     digitalEmployeeID,
			Conclusion:            req.Conclusion,
			EvidenceRefs:          req.EvidenceRefs,
			ArtifactRefs:          req.ArtifactRefs,
			ConfidenceFactors:     req.ConfidenceFactors,
			Uncertainty:           req.Uncertainty,
			MissingInformation:    req.MissingInformation,
			RecommendedNextAction: req.RecommendedNextAction,
			RequiresHumanReview:   req.RequiresHumanReview,
		}, contract); err != nil {
			return nil, err
		}
		return nil, ErrInvalidProjectEvidence
	}
	recordReq := projectTaskAttemptResultRecordRequest(task, req.ProjectTaskAttemptRuntimeRequest, nil, nil, *resultContract, validation)
	if err := s.projectDemandCriterionVerdicts(ctx, task, req.ProjectTaskAttemptRuntimeRequest, *resultContract); err != nil {
		return nil, err
	}
	writebackRepository, err := s.projectTaskAttemptWritebackRepository()
	if err != nil {
		return nil, err
	}
	if projectTaskRequiresAcceptance(task, req) {
		acceptanceReq := CompleteProjectTaskAttemptAcceptanceWritebackRequest{
			Task:     task,
			Complete: req,
			Decision: CreateDecisionRequestRequest{
				TenantID:          req.TenantID,
				ProjectID:         task.ProjectID,
				ApprovalRequestID: uuid.Nil,
				CoordinationJobID: task.CoordinationJobID,
				ProjectTaskID:     &task.ID,
				TargetUserID:      projectRecord.HumanOwnerUserID,
				DecisionType:      projectTaskHumanWaitDecisionType(HumanWaitReasonAcceptanceRequired),
				TitleSnapshot:     task.Title,
				SummarySnapshot:   req.Conclusion,
				RiskLevelSnapshot: stringValue(task.RiskLevel),
				StatusSnapshot:    "pending",
			},
		}
		var result ProjectTaskWritebackResult
		var err error
		result, err = writebackRepository.CompleteProjectTaskAttemptAcceptanceResultWriteback(ctx, CompleteProjectTaskAttemptAcceptanceResultWritebackRequest{
			Acceptance: acceptanceReq,
			Result:     recordReq,
		})
		if err != nil {
			return nil, err
		}
		s.recordHandoffVerifiedLedgerEvent(ctx, task, req.ProjectTaskAttemptRuntimeRequest)
		if s.inbox != nil {
			if err := s.inbox.UpsertProjectDecisionRequest(ctx, result.Decision); err != nil {
				return nil, err
			}
		}
		if err := s.materializeTaskCompletionEvidence(ctx, task, CompleteProjectTaskRequest{
			TenantID:              req.TenantID,
			RuntimeNodeID:         req.RuntimeNodeID,
			ProjectTaskID:         req.ProjectTaskID,
			DigitalEmployeeID:     digitalEmployeeID,
			Conclusion:            req.Conclusion,
			EvidenceRefs:          req.EvidenceRefs,
			ArtifactRefs:          req.ArtifactRefs,
			ConfidenceFactors:     req.ConfidenceFactors,
			Uncertainty:           req.Uncertainty,
			MissingInformation:    req.MissingInformation,
			RecommendedNextAction: req.RecommendedNextAction,
			RequiresHumanReview:   req.RequiresHumanReview,
		}, result.Summary.ID); err != nil {
			_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, task.ProjectID, "EvidenceMaterialization", "failed", err, map[string]any{
				"project_task_id":      task.ID.String(),
				"execution_summary_id": result.Summary.ID.String(),
			})
		}
		return &result.Summary, nil
	}
	var result ProjectTaskWritebackResult
	result, err = writebackRepository.CompleteProjectTaskAttemptResultWriteback(ctx, CompleteProjectTaskAttemptResultWritebackRequest{
		Complete: req,
		Result:   recordReq,
	})
	if err != nil {
		return nil, err
	}
	s.recordHandoffVerifiedLedgerEvent(ctx, task, req.ProjectTaskAttemptRuntimeRequest)
	if err := s.materializeTaskCompletionEvidence(ctx, task, CompleteProjectTaskRequest{
		TenantID:              req.TenantID,
		RuntimeNodeID:         req.RuntimeNodeID,
		ProjectTaskID:         req.ProjectTaskID,
		DigitalEmployeeID:     digitalEmployeeID,
		Conclusion:            req.Conclusion,
		EvidenceRefs:          req.EvidenceRefs,
		ArtifactRefs:          req.ArtifactRefs,
		ConfidenceFactors:     req.ConfidenceFactors,
		Uncertainty:           req.Uncertainty,
		MissingInformation:    req.MissingInformation,
		RecommendedNextAction: req.RecommendedNextAction,
		RequiresHumanReview:   req.RequiresHumanReview,
	}, result.Summary.ID); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, task.ProjectID, "EvidenceMaterialization", "failed", err, map[string]any{
			"project_task_id":      task.ID.String(),
			"execution_summary_id": result.Summary.ID.String(),
		})
	}
	if err := s.coordinator.SignalEmployeeTaskCompleted(ctx, EmployeeTaskCompletedSignal{
		TenantID:           req.TenantID,
		ProjectID:          task.ProjectID,
		ProjectTaskID:      task.ID,
		ExecutionSummaryID: result.Summary.ID,
		CompletedEventID:   result.Event.ID,
		WorkflowID:         projectRecord.CoordinationWorkflowID,
	}); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, task.ProjectID, "EmployeeTaskCompleted", "failed", err, map[string]any{
			"project_task_id":      task.ID.String(),
			"execution_summary_id": result.Summary.ID.String(),
			"completed_event_id":   result.Event.ID.String(),
		})
		return nil, err
	}
	return &result.Summary, nil
}

func (s *Service) SubmitProjectTaskAttemptResult(ctx context.Context, req SubmitProjectTaskAttemptResultRequest) (*ExecutionSummary, error) {
	if req.ResultContract.Status == TaskResultStatusCompleted {
		return s.CompleteProjectTaskAttempt(ctx, CompleteProjectTaskAttemptRequest{
			ProjectTaskAttemptRuntimeRequest: req.ProjectTaskAttemptRuntimeRequest,
			Conclusion:                       req.ResultContract.Summary,
			EvidenceRefs:                     taskResultRefsToAny(req.ResultContract.EvidenceRefs),
			ArtifactRefs:                     taskResultRefsToAny(req.ResultContract.ArtifactRefs),
			RecommendedNextAction:            firstTaskResultFollowUpSummary(req.ResultContract.FollowUpRequests),
			RequiresHumanReview:              req.ResultContract.HumanReviewRequest != nil,
			ResultContract:                   &req.ResultContract,
		})
	}

	task, _, err := s.validateAttemptRuntimeRequest(ctx, req.ProjectTaskAttemptRuntimeRequest)
	if err != nil {
		return nil, err
	}
	validation, err := s.validateTaskResultContractForAttempt(ctx, task, req.ProjectTaskAttemptRuntimeRequest, req.ResultContract)
	if err != nil {
		return nil, err
	}
	if !validation.Valid {
		return s.recordRejectedProjectTaskAttemptResultAndWaitHuman(ctx, task, req.ProjectTaskAttemptRuntimeRequest, req.ResultContract, validation)
	}
	result, err := s.recordProjectTaskAttemptResult(ctx, task, req.ProjectTaskAttemptRuntimeRequest, nil, nil, req.ResultContract)
	if err != nil {
		return nil, err
	}
	if err := s.routeNonCompletedProjectTaskAttemptResult(ctx, req, result); err != nil {
		return nil, err
	}
	return &ExecutionSummary{
		TenantID:      req.TenantID,
		ProjectID:     task.ProjectID,
		ProjectTaskID: req.ProjectTaskID,
		Conclusion:    req.ResultContract.Summary,
	}, nil
}

func (s *Service) recordProjectTaskAttemptResult(ctx context.Context, task ProjectTask, runtimeReq ProjectTaskAttemptRuntimeRequest, summaryID, eventID *uuid.UUID, contract TaskResultContract) (ProjectTaskResult, error) {
	validation, err := s.validateTaskResultContractForAttempt(ctx, task, runtimeReq, contract)
	if err != nil {
		return ProjectTaskResult{}, err
	}
	result, err := s.repository.RecordProjectTaskResult(ctx, projectTaskAttemptResultRecordRequest(task, runtimeReq, summaryID, eventID, contract, validation))
	if err != nil {
		return ProjectTaskResult{}, err
	}
	if _, err := s.repository.LinkProjectTaskLatestResult(ctx, runtimeReq.TenantID, task.ProjectID, runtimeReq.ProjectTaskID, result.ID); err != nil {
		return ProjectTaskResult{}, err
	}
	if !validation.Valid {
		return result, ErrInvalidProjectEvidence
	}
	return result, nil
}

func (s *Service) validateTaskResultContractForAttempt(ctx context.Context, task ProjectTask, runtimeReq ProjectTaskAttemptRuntimeRequest, contract TaskResultContract) (TaskResultValidation, error) {
	validation := ValidateTaskResultContract(task, contract)
	if !validation.Valid {
		return validation, nil
	}
	var validationErrors []TaskResultValidationError
	runtimeErrors, err := s.validateRuntimeAttestationRefs(ctx, task, runtimeReq, contract)
	if err != nil {
		return validation, err
	}
	validationErrors = append(validationErrors, runtimeErrors...)

	acceptanceErrors, err := s.validateAcceptanceCriterionAttestation(ctx, task, runtimeReq, contract)
	if err != nil {
		return validation, err
	}
	validationErrors = append(validationErrors, acceptanceErrors...)

	if len(validationErrors) > 0 {
		validation.Valid = false
		validation.Decision = TaskResultDecisionValidationFailed
		validation.Errors = append(validation.Errors, validationErrors...)
	}
	return validation, nil
}

// demandAcceptanceCriteriaSnapshot reads the plan-level acceptance criteria
// snapshot (Task 4's demand_acceptance_criteria) for the task's own demand and
// accepted plan revision. Tasks predating the snapshot rollout (or whose
// demand/plan-revision linkage is unset) have no rows: callers must treat that
// as "skip entirely", not "no criteria required" — see demandLegacyGuard note
// on validateAcceptanceCriterionAttestation and projectDemandCriterionVerdicts.
func (s *Service) demandAcceptanceCriteriaSnapshot(ctx context.Context, task ProjectTask) ([]DemandAcceptanceCriterion, error) {
	if task.DemandID == nil || task.AcceptedPlanRevisionID == nil {
		return nil, nil
	}
	return s.repository.ListDemandAcceptanceCriteria(ctx, task.TenantID, *task.DemandID, *task.AcceptedPlanRevisionID)
}

// criteriaSatisfiedByTask narrows a demand+revision criteria snapshot to the
// criteria THIS task is on the hook for: those whose SatisfiedBy names the
// task's planned key — the same identity Task 4's decompose-time injection
// keyed on (criterionInjectionsByTaskKey[plannedTask.PlannedTaskKey]).
// Defense-in-depth for both projection and attestation tightening: without
// this scope, the statement-text matching fallback could resolve one task's
// echoed statement against a similarly-worded criterion belonging to a
// different task — rejecting task A for task B's missing attestation, or
// projecting a verdict task B never earned. A task with no planned key (never
// produced by Task-4 decomposition) is on the hook for nothing. human_judgment
// criteria typically carry an empty SatisfiedBy and thus fall out of scope
// here too — consistent with them being a human sign-off matter.
func criteriaSatisfiedByTask(snapshot []DemandAcceptanceCriterion, task ProjectTask) []DemandAcceptanceCriterion {
	if task.PlannedTaskKey == nil {
		return nil
	}
	taskKey := strings.TrimSpace(*task.PlannedTaskKey)
	if taskKey == "" {
		return nil
	}
	scoped := make([]DemandAcceptanceCriterion, 0, len(snapshot))
	for _, criterion := range snapshot {
		for _, satisfiedBy := range criterion.SatisfiedBy {
			if strings.TrimSpace(satisfiedBy) == taskKey {
				scoped = append(scoped, criterion)
				break
			}
		}
	}
	return scoped
}

// matchAcceptanceResultToSnapshotCriterion resolves which of the employee's
// self-reported AcceptanceResults judges a given snapshot criterion. Matching
// resolution (Task 4 review, binding): criterion_id equality first — the
// contract gate that requires an AcceptanceResult per requiredAcceptanceCriteria
// keys on statement text (see stringsFromCriterionMap), so an employee may
// legitimately echo only the statement and never populate CriterionID. Falling
// back to matchesCriterion's statement/id/name candidate set against the
// criterion's Statement covers that case.
func matchAcceptanceResultToSnapshotCriterion(results []TaskResultAcceptanceResult, criterion DemandAcceptanceCriterion) (TaskResultAcceptanceResult, bool) {
	for _, result := range results {
		if strings.TrimSpace(result.CriterionID) != "" && result.CriterionID == criterion.CriterionID {
			return result, true
		}
	}
	for _, result := range results {
		if matchesCriterion(result, criterion.Statement) {
			return result, true
		}
	}
	return TaskResultAcceptanceResult{}, false
}

const (
	demandAcceptanceVerificationMethodAutomatedTest = "automated_test"
	demandAcceptanceVerificationMethodHumanJudgment = "human_judgment"
)

// validateAcceptanceCriterionAttestation tightens automated_test criteria:
// "绿灯必须挂真实执行记录". Correctness of this gate does NOT rely on the
// employee self-reporting an attestation ref — the runtime mints the real
// attestation (command/exit-code/hash) at writeback into the result
// verification[] array and project_task_attestations, and the employee never
// sees it and cannot echo it into acceptance_results. So the SERVER verifies a
// real runtime attestation EXISTS for this attempt (via runtimeVerified
// AttestationRefsForAttempt). A passed/human_overridden automated_test result
// with NO such attestation anywhere is the genuinely-unbacked green-light and is
// rejected. Skips entirely when the task has no criteria snapshot (legacy plans
// / demands not decomposed under Task 4) so pre-existing behavior is untouched
// byte-for-byte.
func (s *Service) validateAcceptanceCriterionAttestation(ctx context.Context, task ProjectTask, runtimeReq ProjectTaskAttemptRuntimeRequest, contract TaskResultContract) ([]TaskResultValidationError, error) {
	if contract.Status != TaskResultStatusCompleted {
		return nil, nil
	}
	snapshot, err := s.demandAcceptanceCriteriaSnapshot(ctx, task)
	if err != nil {
		return nil, err
	}
	if len(snapshot) == 0 {
		return nil, nil
	}
	scoped := criteriaSatisfiedByTask(snapshot, task)
	// Collect the automated_test criteria this task claims green so we only pay
	// for the attestation lookup when one is actually in play.
	var claimed []DemandAcceptanceCriterion
	for _, criterion := range scoped {
		if criterion.VerificationMethod != demandAcceptanceVerificationMethodAutomatedTest {
			continue
		}
		result, ok := matchAcceptanceResultToSnapshotCriterion(contract.AcceptanceResults, criterion)
		if !ok {
			continue
		}
		if result.Status != TaskResultCriterionStatusPassed && result.Status != TaskResultCriterionStatusHumanOverridden {
			continue
		}
		claimed = append(claimed, criterion)
	}
	if len(claimed) == 0 {
		return nil, nil
	}
	serverRefs, err := s.runtimeVerifiedAttestationRefsForAttempt(ctx, task, runtimeReq, contract)
	if err != nil {
		return nil, err
	}
	if len(serverRefs) > 0 {
		return nil, nil
	}
	var errs []TaskResultValidationError
	for _, criterion := range claimed {
		errs = append(errs, "acceptance_result_attestation_required:"+criterion.CriterionID)
	}
	return errs, nil
}

// runtimeVerifiedAttestationRefsForAttempt returns the attestation refs the
// SERVER can independently vouch for this attempt: refs the runtime minted into
// the result verification[] array whose backing record is a succeeded
// ProjectTaskAttestation for THIS attempt, or — when verification[] carries none
// — refs derived directly from succeeded attestation records the runtime wrote
// for this attempt. The employee never sees these refs (they are minted from
// attempt_id+command_id at writeback), so they cannot be forged into
// acceptance_results; their existence is what proves a real execution backed the
// green light. Empty result means no attestation exists for the attempt at all.
func (s *Service) runtimeVerifiedAttestationRefsForAttempt(ctx context.Context, task ProjectTask, runtimeReq ProjectTaskAttemptRuntimeRequest, contract TaskResultContract) ([]string, error) {
	attestations, err := s.repository.ListProjectTaskAttestations(ctx, runtimeReq.TenantID, task.ProjectID, runtimeReq.ProjectTaskID, 1000, 0)
	if err != nil {
		return nil, err
	}
	refs := make([]string, 0)
	seen := map[string]struct{}{}
	add := func(ref string) {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			return
		}
		if _, ok := seen[ref]; ok {
			return
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	// Prefer the runtime-minted refs echoed into verification[] whose backing
	// record is a succeeded attestation for this attempt (authoritative, and it
	// keeps the exact ref string the runtime produced for lineage display).
	for _, ref := range runtimeAttestationRefsFromVerifications(contract.Verification) {
		key, parsedAttempt, hasAttempt := parseRuntimeAttestationRef(ref)
		if hasAttempt && parsedAttempt != runtimeReq.AttemptID {
			continue
		}
		att, ok := findRuntimeAttestationForRef(attestations, key)
		if !ok || att.AttemptID != runtimeReq.AttemptID || att.Status != ProjectTaskAttestationStatusSucceeded {
			continue
		}
		add(ref)
	}
	if len(refs) > 0 {
		return refs, nil
	}
	// Fall back to the attestations table directly: any succeeded attestation
	// minted for this attempt proves a real execution backed the result.
	for _, att := range attestations {
		if att.AttemptID != runtimeReq.AttemptID || att.Status != ProjectTaskAttestationStatusSucceeded {
			continue
		}
		add(attestationRecordRef(att))
	}
	return refs, nil
}

// attestationRecordRef derives a stable, traceable evidence ref from a
// runtime-minted attestation record (its idempotency key carries the
// attempt/command identity; the row UUID is the last-resort anchor).
func attestationRecordRef(att ProjectTaskAttestation) string {
	key := strings.TrimSpace(att.IdempotencyKey)
	if key == "" {
		key = att.ID.String()
	}
	if strings.HasPrefix(key, "attestation:") {
		return key
	}
	return "attestation:" + key
}

// demandCriterionVerdictValueFromResultStatus maps an employee's self-reported
// criterion status to the verdict table's vocabulary. not_applicable projects
// as the third verdict value (the contract layer already requires a
// human_accepted_reason for it, and the gate treats it as non-blocking — see
// demandCriterionVerdictNotApplicable). needs_human genuinely awaits resolution
// and is intentionally not projected.
func demandCriterionVerdictValueFromResultStatus(status TaskResultCriterionStatus) (string, bool) {
	switch status {
	case TaskResultCriterionStatusPassed, TaskResultCriterionStatusHumanOverridden:
		return demandCriterionVerdictSatisfied, true
	case TaskResultCriterionStatusFailed:
		return demandCriterionVerdictUnsatisfied, true
	case TaskResultCriterionStatusNotApplicable:
		return demandCriterionVerdictNotApplicable, true
	default:
		return "", false
	}
}

// projectDemandCriterionVerdicts writes one demand_criterion_verdicts row per
// snapshot criterion the employee's AcceptanceResults judged, on the same
// completed-and-validated path that records the task result. For a satisfied
// automated_test criterion the SERVER-known runtime attestation ref (from the
// result verification[] array or project_task_attestations for this attempt) is
// attached to the verdict's evidence_refs automatically — so the lineage panel
// shows a real, traceable attestation ref even though the employee never echoed
// it. human_judgment criteria are a human sign-off matter (later task): an
// employee self-report against one is intentionally not projected, only logged.
// Criteria with no matching result, or whose result status is needs_human, are
// left unprojected — a later attempt or a human may still resolve them; a
// not_applicable result IS projected as the non-blocking not_applicable verdict.
// No-ops entirely when the task has no criteria snapshot
// (legacy guard, mirrors validateAcceptanceCriterionAttestation).
func (s *Service) projectDemandCriterionVerdicts(ctx context.Context, task ProjectTask, runtimeReq ProjectTaskAttemptRuntimeRequest, contract TaskResultContract) error {
	if contract.Status != TaskResultStatusCompleted || task.AssignedDigitalEmployeeID == nil {
		return nil
	}
	snapshot, err := s.demandAcceptanceCriteriaSnapshot(ctx, task)
	if err != nil {
		return err
	}
	if len(snapshot) == 0 {
		return nil
	}
	var serverAttestationRefs []string
	serverAttestationRefsLoaded := false
	for _, criterion := range criteriaSatisfiedByTask(snapshot, task) {
		result, ok := matchAcceptanceResultToSnapshotCriterion(contract.AcceptanceResults, criterion)
		if !ok {
			continue
		}
		if criterion.VerificationMethod == demandAcceptanceVerificationMethodHumanJudgment {
			slog.Default().Warn("demand acceptance criterion self-reported by executor: ignored, awaiting human sign-off",
				"project_task_id", task.ID,
				"demand_id", criterion.DemandID,
				"plan_revision_id", criterion.PlanRevisionID,
				"criterion_id", criterion.CriterionID,
			)
			continue
		}
		if criterion.VerificationMethod == demandCriterionVerificationMethodAdversarialReview {
			// adversarial_review criteria are decided by the adversarial judge
			// engine (aggregate row, judge_type=adversarial), never by executor
			// self-report — projecting an executor verdict here would let a task
			// self-satisfy a criterion the judges must decide. Skip; only log.
			slog.Default().Warn("demand acceptance criterion self-reported by executor: ignored, awaiting adversarial review",
				"project_task_id", task.ID,
				"demand_id", criterion.DemandID,
				"plan_revision_id", criterion.PlanRevisionID,
				"criterion_id", criterion.CriterionID,
			)
			continue
		}
		verdictValue, ok := demandCriterionVerdictValueFromResultStatus(result.Status)
		if !ok {
			continue
		}
		reason := strings.TrimSpace(result.Summary)
		if reason == "" {
			reason = strings.TrimSpace(result.HumanAcceptedReason)
		}
		evidenceRefs := result.EvidenceRefs
		// A satisfied automated_test verdict carries the server-verified
		// attestation ref (validation already guaranteed one exists for this
		// attempt), attaching real execution lineage the employee couldn't echo.
		if criterion.VerificationMethod == demandAcceptanceVerificationMethodAutomatedTest && verdictValue == "satisfied" {
			if !serverAttestationRefsLoaded {
				serverAttestationRefs, err = s.runtimeVerifiedAttestationRefsForAttempt(ctx, task, runtimeReq, contract)
				if err != nil {
					return err
				}
				serverAttestationRefsLoaded = true
			}
			evidenceRefs = mergeStringRefs(result.EvidenceRefs, serverAttestationRefs)
		}
		if err := s.repository.CreateDemandCriterionVerdict(ctx, CreateDemandCriterionVerdictRequest{
			TenantID:       task.TenantID,
			ProjectID:      task.ProjectID,
			DemandID:       criterion.DemandID,
			PlanRevisionID: criterion.PlanRevisionID,
			CriterionID:    criterion.CriterionID,
			Verdict:        verdictValue,
			JudgeType:      "executor",
			JudgeID:        *task.AssignedDigitalEmployeeID,
			Reason:         reason,
			EvidenceRefs:   evidenceRefs,
			ProjectTaskID:  &task.ID,
		}); err != nil {
			return err
		}
	}
	return nil
}

// mergeStringRefs returns the union of two ref slices preserving order
// (base first, then additions), de-duplicated by trimmed value.
func mergeStringRefs(base, extra []string) []string {
	merged := make([]string, 0, len(base)+len(extra))
	seen := map[string]struct{}{}
	for _, group := range [][]string{base, extra} {
		for _, ref := range group {
			trimmed := strings.TrimSpace(ref)
			if trimmed == "" {
				continue
			}
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			merged = append(merged, trimmed)
		}
	}
	return merged
}

func (s *Service) validateRuntimeAttestationRefs(ctx context.Context, task ProjectTask, runtimeReq ProjectTaskAttemptRuntimeRequest, contract TaskResultContract) ([]TaskResultValidationError, error) {
	if contract.Status != TaskResultStatusCompleted || !boolFromTaskContract(task.HandoffContract, "requires_runtime_attestation") {
		return nil, nil
	}
	refs := runtimeAttestationRefsFromVerifications(contract.Verification)
	if len(refs) == 0 {
		return nil, nil
	}
	attestations, err := s.repository.ListProjectTaskAttestations(ctx, runtimeReq.TenantID, task.ProjectID, runtimeReq.ProjectTaskID, 1000, 0)
	if err != nil {
		return nil, err
	}
	var errors []TaskResultValidationError
	for _, ref := range refs {
		key, parsedAttemptID, hasParsedAttempt := parseRuntimeAttestationRef(ref)
		if hasParsedAttempt && parsedAttemptID != runtimeReq.AttemptID {
			errors = append(errors, "verification_attestation_ref_wrong_attempt")
			continue
		}
		attestation, ok := findRuntimeAttestationForRef(attestations, key)
		if !ok {
			errors = append(errors, "verification_attestation_ref_not_found")
			continue
		}
		if attestation.AttemptID != runtimeReq.AttemptID {
			errors = append(errors, "verification_attestation_ref_wrong_attempt")
			continue
		}
		if attestation.RuntimeNodeID != runtimeReq.RuntimeNodeID {
			errors = append(errors, "verification_attestation_ref_wrong_runtime_node")
			continue
		}
		if task.AssignedDigitalEmployeeID != nil && attestation.DigitalEmployeeID != *task.AssignedDigitalEmployeeID {
			errors = append(errors, "verification_attestation_ref_wrong_digital_employee")
			continue
		}
		if attestation.Status != ProjectTaskAttestationStatusSucceeded {
			errors = append(errors, "verification_attestation_ref_not_succeeded")
		}
	}
	return errors, nil
}

func runtimeAttestationRefsFromVerifications(verifications []TaskResultVerification) []string {
	refs := make([]string, 0)
	seen := map[string]struct{}{}
	for _, verification := range verifications {
		if verification.Status != TaskResultVerificationStatusPassed {
			continue
		}
		for _, ref := range verification.EvidenceRefs {
			if !taskResultRefIsAttestation(ref) {
				continue
			}
			value := strings.TrimSpace(ref.Ref)
			if value == "" {
				value = strings.TrimSpace(ref.ID)
			}
			if value == "" {
				value = strings.TrimSpace(ref.URI)
			}
			if value == "" {
				value = strings.TrimSpace(ref.URL)
			}
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			refs = append(refs, value)
		}
	}
	return refs
}

func taskResultRefIsAttestation(ref TaskResultRef) bool {
	return strings.EqualFold(strings.TrimSpace(ref.Kind), "attestation") ||
		strings.EqualFold(strings.TrimSpace(ref.Type), "attestation") ||
		strings.HasPrefix(strings.TrimSpace(ref.Ref), "attestation:")
}

func parseRuntimeAttestationRef(ref string) (string, uuid.UUID, bool) {
	key := strings.TrimSpace(ref)
	key = strings.TrimPrefix(key, "attestation:")
	parts := strings.Split(key, ":")
	if len(parts) >= 3 && parts[0] == "project-task-attempt" {
		if attemptID, err := uuid.Parse(parts[1]); err == nil {
			return key, attemptID, true
		}
	}
	return key, uuid.Nil, false
}

func findRuntimeAttestationForRef(attestations []ProjectTaskAttestation, key string) (ProjectTaskAttestation, bool) {
	for _, attestation := range attestations {
		if strings.TrimSpace(attestation.IdempotencyKey) == key {
			return attestation, true
		}
		if attestation.ID.String() == key {
			return attestation, true
		}
	}
	return ProjectTaskAttestation{}, false
}

// Best-effort observability: the handoff check outcome must be visible on the
// execution ledger, but failing to record it must never fail the completion.
func (s *Service) recordHandoffVerifiedLedgerEvent(ctx context.Context, task ProjectTask, runtimeReq ProjectTaskAttemptRuntimeRequest) {
	produces := taskPlannerProduces(task)
	if len(produces) == 0 {
		return
	}
	_, _ = s.repository.CreateExecutionLedgerEvent(ctx, CreateExecutionLedgerEventRequest{
		TenantID:             runtimeReq.TenantID,
		ProjectID:            task.ProjectID,
		ProjectTaskID:        &task.ID,
		ProjectTaskAttemptID: &runtimeReq.AttemptID,
		EventType:            ExecutionLedgerEventHandoffVerified,
		SourceType:           "project_task_attempt",
		SourceID:             runtimeReq.AttemptID.String(),
		ActorType:            "control_plane",
		OutputSummary:        "交接产出逐项核对通过: " + strings.Join(produces, ", "),
		Metadata:             map[string]any{"produces": produces},
		IdempotencyKey:       "project_task_attempt:" + runtimeReq.AttemptID.String() + ":handoff.verified",
	})
}

func (s *Service) recordHandoffUnfulfilledLedgerEvent(ctx context.Context, task ProjectTask, runtimeReq ProjectTaskAttemptRuntimeRequest, validation TaskResultValidation) {
	var missing []string
	for _, validationError := range validation.Errors {
		if name, ok := strings.CutPrefix(validationError, "handoff_deliverable_missing:"); ok {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return
	}
	_, _ = s.repository.CreateExecutionLedgerEvent(ctx, CreateExecutionLedgerEventRequest{
		TenantID:             runtimeReq.TenantID,
		ProjectID:            task.ProjectID,
		ProjectTaskID:        &task.ID,
		ProjectTaskAttemptID: &runtimeReq.AttemptID,
		EventType:            ExecutionLedgerEventHandoffUnfulfilled,
		SourceType:           "project_task_attempt",
		SourceID:             runtimeReq.AttemptID.String(),
		ActorType:            "control_plane",
		OutputSummary:        "交接产出缺项: " + strings.Join(missing, ", "),
		Metadata:             map[string]any{"missing": missing},
		IdempotencyKey:       "project_task_attempt:" + runtimeReq.AttemptID.String() + ":handoff.unfulfilled",
	})
}

func (s *Service) recordRejectedProjectTaskAttemptResultAndWaitHuman(ctx context.Context, task ProjectTask, runtimeReq ProjectTaskAttemptRuntimeRequest, contract TaskResultContract, validation TaskResultValidation) (*ExecutionSummary, error) {
	result, err := s.recordProjectTaskAttemptResult(ctx, task, runtimeReq, nil, nil, contract)
	if err != nil && !errors.Is(err, ErrInvalidProjectEvidence) {
		return nil, err
	}
	if err := s.routeRejectedProjectTaskAttemptResult(ctx, runtimeReq, result, contract, validation); err != nil {
		return nil, err
	}
	return &ExecutionSummary{
		TenantID:      runtimeReq.TenantID,
		ProjectID:     task.ProjectID,
		ProjectTaskID: runtimeReq.ProjectTaskID,
		Conclusion:    taskResultValidationFailedSummary(contract, validation),
	}, nil
}

func (s *Service) routeNonCompletedProjectTaskAttemptResult(ctx context.Context, req SubmitProjectTaskAttemptResultRequest, result ProjectTaskResult) error {
	switch result.Decision {
	case TaskResultDecisionRevisionAttempt, TaskResultDecisionRevisionTask:
		_, err := s.WaitHumanProjectTaskAttempt(ctx, WaitHumanProjectTaskAttemptRequest{
			ProjectTaskAttemptRuntimeRequest: req.ProjectTaskAttemptRuntimeRequest,
			Reason:                           humanWaitReasonForTaskResultRevision(req.ResultContract),
			Summary:                          taskResultHumanWaitSummary(req.ResultContract),
			MissingContextRefs:               taskResultHumanWaitContextRefs(result, req.ResultContract),
			SuggestedResolutionOptions:       taskResultHumanWaitResolutionOptions(result.Decision),
			ResultContract:                   &req.ResultContract,
		})
		return err
	case TaskResultDecisionBlockedWaitingHuman:
		_, err := s.WaitHumanProjectTaskAttempt(ctx, WaitHumanProjectTaskAttemptRequest{
			ProjectTaskAttemptRuntimeRequest: req.ProjectTaskAttemptRuntimeRequest,
			Reason:                           humanWaitReasonForTaskResultBlocker(req.ResultContract.Blocker),
			Summary:                          taskResultHumanWaitSummary(req.ResultContract),
			MissingContextRefs:               taskResultHumanWaitContextRefs(result, req.ResultContract),
			SuggestedResolutionOptions:       taskResultHumanWaitResolutionOptions(result.Decision),
			ResultContract:                   &req.ResultContract,
		})
		return err
	case TaskResultDecisionReplanRequested:
		_, err := s.WaitHumanProjectTaskAttempt(ctx, WaitHumanProjectTaskAttemptRequest{
			ProjectTaskAttemptRuntimeRequest: req.ProjectTaskAttemptRuntimeRequest,
			Reason:                           HumanWaitReasonPlanInvalid,
			Summary:                          taskResultHumanWaitSummary(req.ResultContract),
			MissingContextRefs:               taskResultHumanWaitContextRefs(result, req.ResultContract),
			SuggestedResolutionOptions:       taskResultHumanWaitResolutionOptions(result.Decision),
			ResultContract:                   &req.ResultContract,
		})
		return err
	case TaskResultDecisionFailedRetryable, TaskResultDecisionFailedRecovery:
		_, err := s.FailProjectTaskAttempt(ctx, FailProjectTaskAttemptRequest{
			ProjectTaskAttemptRuntimeRequest: req.ProjectTaskAttemptRuntimeRequest,
			FailureSummary:                   taskResultFailureSummary(req.ResultContract),
			FailureFamily:                    taskResultFailureFamily(req.ResultContract),
			Retryable:                        taskResultFailureRetryable(req.ResultContract),
			ResultContract:                   &req.ResultContract,
		})
		return err
	case TaskResultDecisionCancelledTerminal:
		retryable := false
		_, err := s.FailProjectTaskAttempt(ctx, FailProjectTaskAttemptRequest{
			ProjectTaskAttemptRuntimeRequest: req.ProjectTaskAttemptRuntimeRequest,
			FailureSummary:                   taskResultCancellationSummary(req.ResultContract),
			FailureFamily:                    FailureFamilyBusinessCancelled,
			Retryable:                        &retryable,
			ResultContract:                   &req.ResultContract,
		})
		return err
	case TaskResultDecisionBlockedResolvableUpstream:
		return s.signalUpstreamSupplementResolvable(ctx, req)
	default:
		return ErrInvalidProjectEvidence
	}
}

// signalUpstreamSupplementResolvable reacts to a blocked_resolvable_upstream
// result without opening a human decision: the platform, not a human, resolves
// the missing input by locating its producer (see CreateUpstreamSupplementTasks).
// The result contract is already persisted by recordProjectTaskAttemptResult; this
// only needs to append the audit event and signal the coordinator workflow so
// handleEmployeeTaskCompleted can re-derive the decision and append the supplement.
func (s *Service) signalUpstreamSupplementResolvable(ctx context.Context, req SubmitProjectTaskAttemptResultRequest) error {
	task, err := s.repository.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
	if err != nil {
		return err
	}
	projectRecord, err := s.repository.GetProject(ctx, req.TenantID, task.ProjectID)
	if err != nil {
		return err
	}
	event, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:     req.TenantID,
		ProjectID:    task.ProjectID,
		EventType:    ProjectEventTaskResultBlocked,
		ActorType:    "digital_employee",
		ActorID:      digitalEmployeeIDStringForProjectTask(task),
		ResourceType: strPtr("project_task"),
		ResourceID:   strPtr(task.ID.String()),
		Summary:      "项目任务缺少上游产出，需要上游补做",
		Payload: map[string]any{
			"project_task_id": task.ID.String(),
			"missing_inputs":  req.ResultContract.Blocker.MissingInputs,
		},
	})
	if err != nil {
		return err
	}
	if err := s.coordinator.SignalEmployeeTaskCompleted(ctx, EmployeeTaskCompletedSignal{
		TenantID:         req.TenantID,
		ProjectID:        task.ProjectID,
		ProjectTaskID:    task.ID,
		CompletedEventID: event.ID,
		WorkflowID:       projectRecord.CoordinationWorkflowID,
	}); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, task.ProjectID, "EmployeeTaskCompleted", "failed", err, map[string]any{
			"project_task_id":    task.ID.String(),
			"completed_event_id": event.ID.String(),
		})
		return err
	}
	return nil
}

func digitalEmployeeIDStringForProjectTask(task ProjectTask) string {
	if task.AssignedDigitalEmployeeID != nil {
		return task.AssignedDigitalEmployeeID.String()
	}
	return ""
}

func (s *Service) routeRejectedProjectTaskAttemptResult(ctx context.Context, runtimeReq ProjectTaskAttemptRuntimeRequest, result ProjectTaskResult, contract TaskResultContract, validation TaskResultValidation) error {
	_, err := s.WaitHumanProjectTaskAttempt(ctx, WaitHumanProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: runtimeReq,
		Reason:                           HumanWaitReasonClarification,
		Summary:                          taskResultValidationFailedSummary(contract, validation),
		MissingContextRefs:               taskResultValidationHumanWaitContextRefs(result, contract, validation),
		SuggestedResolutionOptions:       taskResultHumanWaitResolutionOptions(TaskResultDecisionValidationFailed),
		ResultContract:                   &contract,
	})
	return err
}

func taskResultValidationFailedSummary(contract TaskResultContract, validation TaskResultValidation) string {
	if summary := strings.TrimSpace(contract.Summary); summary != "" {
		return summary
	}
	if len(validation.Errors) > 0 {
		return "Task result failed validation: " + strings.Join(taskResultValidationErrors(validation.Errors), ", ")
	}
	return "Task result failed validation"
}

func taskResultValidationHumanWaitContextRefs(result ProjectTaskResult, contract TaskResultContract, validation TaskResultValidation) []any {
	refs := taskResultHumanWaitContextRefs(result, contract)
	if len(refs) == 0 {
		return refs
	}
	context, ok := refs[0].(map[string]any)
	if !ok {
		return refs
	}
	context["validation_status"] = result.ValidationStatus
	context["validation_errors"] = taskResultValidationErrors(validation.Errors)
	return refs
}

func humanWaitReasonForTaskResultRevision(contract TaskResultContract) string {
	if contract.RevisionRequest != nil && contract.RevisionRequest.ContractChanged {
		return HumanWaitReasonPlanInvalid
	}
	return HumanWaitReasonClarification
}

func humanWaitReasonForTaskResultBlocker(blocker *TaskResultBlocker) string {
	if blocker == nil {
		return HumanWaitReasonClarification
	}
	value := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		blocker.Reason,
		blocker.ResolutionPrompt,
		blocker.RequiredBy,
	}, " ")))
	switch {
	case strings.Contains(value, HumanWaitReasonPermissionRequired),
		strings.Contains(value, "permission"),
		strings.Contains(value, "authorization"),
		strings.Contains(value, "auth"):
		return HumanWaitReasonPermissionRequired
	case strings.Contains(value, HumanWaitReasonApprovalRequired),
		strings.Contains(value, "approval"),
		strings.Contains(value, "approve"):
		return HumanWaitReasonApprovalRequired
	case strings.Contains(value, HumanWaitReasonPlanInvalid),
		strings.Contains(value, "plan invalid"),
		strings.Contains(value, "replan"),
		strings.Contains(value, "contract_changed"):
		return HumanWaitReasonPlanInvalid
	case strings.Contains(value, HumanWaitReasonMissingContext),
		strings.Contains(value, "missing context"),
		strings.Contains(value, "context"):
		return HumanWaitReasonMissingContext
	default:
		if len(blocker.ContextRefs) > 0 {
			return HumanWaitReasonMissingContext
		}
		return HumanWaitReasonClarification
	}
}

func taskResultHumanWaitSummary(contract TaskResultContract) string {
	if summary := strings.TrimSpace(contract.Summary); summary != "" {
		return summary
	}
	return "Task result requires human recovery decision"
}

func taskResultHumanWaitContextRefs(result ProjectTaskResult, contract TaskResultContract) []any {
	context := map[string]any{
		"kind":           "task_result_contract",
		"task_result_id": result.ID.String(),
		"status":         string(contract.Status),
		"decision":       string(result.Decision),
		"summary":        contract.Summary,
	}
	switch {
	case contract.RevisionRequest != nil:
		context["reason"] = contract.RevisionRequest.Reason
		context["contract_changed"] = contract.RevisionRequest.ContractChanged
		context["requested_changes"] = contract.RevisionRequest.RequestedChanges
		if contract.RevisionRequest.RecommendedTaskTitle != "" {
			context["recommended_task_title"] = contract.RevisionRequest.RecommendedTaskTitle
		}
		if contract.RevisionRequest.RecommendedTaskSummary != "" {
			context["recommended_task_summary"] = contract.RevisionRequest.RecommendedTaskSummary
		}
	case contract.Blocker != nil:
		context["reason"] = contract.Blocker.Reason
		context["required_by"] = contract.Blocker.RequiredBy
		context["resolution_prompt"] = contract.Blocker.ResolutionPrompt
	case contract.ReplanRequest != nil:
		context["reason"] = contract.ReplanRequest.Reason
		context["scope"] = contract.ReplanRequest.Scope
		context["constraints"] = contract.ReplanRequest.Constraints
	}
	refs := []any{context}
	if contract.Blocker != nil && len(contract.Blocker.ContextRefs) > 0 {
		refs = append(refs, taskResultRefsToAny(contract.Blocker.ContextRefs)...)
	}
	return refs
}

func taskResultHumanWaitResolutionOptions(decision TaskResultDecision) []string {
	switch decision {
	case TaskResultDecisionRevisionAttempt, TaskResultDecisionRevisionTask, TaskResultDecisionReplanRequested:
		return []string{HumanWaitResolutionResumeSameTask, HumanWaitResolutionCancelAndReplan, HumanWaitResolutionMarkFailed}
	default:
		return []string{HumanWaitResolutionResumeSameTask, HumanWaitResolutionCancelWithoutPlan, HumanWaitResolutionMarkFailed}
	}
}

func taskResultFailureSummary(contract TaskResultContract) string {
	if summary := strings.TrimSpace(contract.Summary); summary != "" {
		return summary
	}
	if contract.Failure != nil && strings.TrimSpace(contract.Failure.Message) != "" {
		return strings.TrimSpace(contract.Failure.Message)
	}
	return "Task result reported failure"
}

func taskResultFailureFamily(contract TaskResultContract) string {
	if contract.Failure != nil {
		if family := strings.TrimSpace(contract.Failure.ErrorFamily); family != "" {
			return family
		}
	}
	return FailureFamilyNonRetryableExecution
}

func taskResultFailureRetryable(contract TaskResultContract) *bool {
	if contract.Failure != nil && contract.Failure.Retryable != nil {
		return contract.Failure.Retryable
	}
	retryable := retryableFailureFamily(taskResultFailureFamily(contract))
	return &retryable
}

func taskResultCancellationSummary(contract TaskResultContract) string {
	if summary := strings.TrimSpace(contract.Summary); summary != "" {
		return summary
	}
	if contract.Cancellation != nil && strings.TrimSpace(contract.Cancellation.Reason) != "" {
		return strings.TrimSpace(contract.Cancellation.Reason)
	}
	return "Task result reported cancellation"
}

func projectTaskAttemptResultRecordRequest(task ProjectTask, runtimeReq ProjectTaskAttemptRuntimeRequest, summaryID, eventID *uuid.UUID, contract TaskResultContract, validation TaskResultValidation) RecordProjectTaskResultRequest {
	validationStatus := "accepted"
	if !validation.Valid {
		validationStatus = "rejected"
	}
	return RecordProjectTaskResultRequest{
		TenantID:           runtimeReq.TenantID,
		ProjectID:          task.ProjectID,
		ProjectTaskID:      runtimeReq.ProjectTaskID,
		AttemptID:          &runtimeReq.AttemptID,
		ExecutionSummaryID: summaryID,
		ResultStatus:       contract.Status,
		ValidationStatus:   validationStatus,
		Decision:           validation.Decision,
		Contract:           contract,
		ValidationErrors:   taskResultValidationErrors(validation.Errors),
		ValidationWarnings: validation.Warnings,
		IdempotencyKey:     "project_task_attempt:" + runtimeReq.AttemptID.String() + ":result:" + runtimeReq.IdempotencyKey,
		CreatedEventID:     eventID,
	}
}

func taskResultValidationErrors(errors []TaskResultValidationError) []string {
	values := make([]string, 0, len(errors))
	for _, err := range errors {
		values = append(values, string(err))
	}
	return values
}

func taskResultRefsToAny(refs []TaskResultRef) []any {
	values := make([]any, 0, len(refs))
	for _, ref := range refs {
		value := map[string]any{}
		if ref.Type != "" {
			value["type"] = ref.Type
		}
		if ref.Ref != "" {
			value["ref"] = ref.Ref
		}
		if ref.Summary != "" {
			value["summary"] = ref.Summary
		}
		values = append(values, value)
	}
	return values
}

func firstTaskResultFollowUpSummary(requests []TaskResultFollowUpRequest) string {
	for _, request := range requests {
		if summary := strings.TrimSpace(request.Summary); summary != "" {
			return summary
		}
	}
	return ""
}

type parsedEvidenceRef struct {
	EvidenceType string
	Title        string
	Summary      string
	SourceType   string
	SourceRef    string
}

type parsedArtifactRef struct {
	ArtifactType string
	Title        string
	ObjectRef    string
	ContentType  string
	Checksum     string
}

// materializeTaskCompletionEvidence turns the structured evidence_refs/artifact_refs a
// digital employee returns on completion into ProjectEvidenceRef / ProjectArtifactRef
// read-model rows, reusing the same create paths as the manual evidence/artifact APIs.
// Re-completion is blocked by the writeback status guard, so this runs once per task.
// Returns the first error encountered; the caller treats it as best-effort.
func (s *Service) materializeTaskCompletionEvidence(ctx context.Context, task ProjectTask, req CompleteProjectTaskRequest, summaryID uuid.UUID) error {
	var firstErr error
	record := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for _, raw := range req.ArtifactRefs {
		parsed, ok := parseArtifactRefElement(raw)
		if !ok {
			continue
		}
		artifact, err := s.repository.CreateArtifactRef(ctx, CreateArtifactRefRequest{
			TenantID:        req.TenantID,
			ProjectID:       task.ProjectID,
			ProjectTaskID:   &task.ID,
			ArtifactType:    parsed.ArtifactType,
			Title:           parsed.Title,
			ObjectRef:       parsed.ObjectRef,
			ContentType:     parsed.ContentType,
			Checksum:        parsed.Checksum,
			RetentionStatus: "pending",
		})
		if err != nil {
			record(err)
			continue
		}
		_, err = s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    task.ProjectID,
			EventType:    ProjectEventArtifactLinked,
			ActorType:    "digital_employee",
			ActorID:      req.DigitalEmployeeID.String(),
			ResourceType: strPtr("project_artifact_ref"),
			ResourceID:   strPtr(artifact.ID.String()),
			Summary:      "项目工件已关联",
			Payload: map[string]any{
				"artifact_type":   parsed.ArtifactType,
				"title":           parsed.Title,
				"project_task_id": task.ID.String(),
			},
		})
		record(err)
	}
	submittedBy := req.DigitalEmployeeID
	for _, raw := range req.EvidenceRefs {
		parsed, ok := parseEvidenceRefElement(raw)
		if !ok {
			continue
		}
		_, err := s.CreateEvidenceRef(ctx, CreateEvidenceRefServiceRequest{
			TenantID:           req.TenantID,
			ProjectID:          task.ProjectID,
			ActorType:          "digital_employee",
			ActorID:            req.DigitalEmployeeID,
			ProjectTaskID:      &task.ID,
			RouteDecisionID:    task.RouteDecisionID,
			ExecutionSummaryID: &summaryID,
			EvidenceType:       parsed.EvidenceType,
			Title:              parsed.Title,
			Summary:            parsed.Summary,
			SourceType:         parsed.SourceType,
			SourceRef:          parsed.SourceRef,
			SubmittedByType:    "digital_employee",
			SubmittedByID:      &submittedBy,
		})
		record(err)
	}
	return firstErr
}

// parseEvidenceRefElement maps a completion evidence_ref element into a parsedEvidenceRef.
// Elements are either a plain string ref or a map[string]any with ref/id/title/type keys
// (matching addReferenceTokens). ok is false when no usable source ref is present.
func parseEvidenceRefElement(value any) (parsedEvidenceRef, bool) {
	parsed := parsedEvidenceRef{EvidenceType: "execution_evidence", SourceType: "runtime_output"}
	switch typed := value.(type) {
	case string:
		parsed.SourceRef = strings.TrimSpace(typed)
	case map[string]any:
		parsed.SourceRef = firstRefString(typed, "source_ref", "ref", "id")
		parsed.Title = firstRefString(typed, "title")
		parsed.Summary = firstRefString(typed, "summary")
		if t := firstRefString(typed, "evidence_type", "type"); t != "" {
			parsed.EvidenceType = t
		}
		if st := firstRefString(typed, "source_type"); st != "" {
			parsed.SourceType = st
		}
	default:
		return parsedEvidenceRef{}, false
	}
	if parsed.SourceRef == "" {
		return parsedEvidenceRef{}, false
	}
	if parsed.Title == "" {
		parsed.Title = parsed.SourceRef
	}
	return parsed, true
}

// parseArtifactRefElement maps a completion artifact_ref element into a parsedArtifactRef.
func parseArtifactRefElement(value any) (parsedArtifactRef, bool) {
	parsed := parsedArtifactRef{ArtifactType: "execution_artifact"}
	switch typed := value.(type) {
	case string:
		parsed.ObjectRef = strings.TrimSpace(typed)
	case map[string]any:
		parsed.ObjectRef = firstRefString(typed, "object_ref", "ref", "id")
		parsed.Title = firstRefString(typed, "title")
		parsed.ContentType = firstRefString(typed, "content_type")
		parsed.Checksum = firstRefString(typed, "checksum")
		if t := firstRefString(typed, "artifact_type", "type"); t != "" {
			parsed.ArtifactType = t
		}
	default:
		return parsedArtifactRef{}, false
	}
	if parsed.ObjectRef == "" {
		return parsedArtifactRef{}, false
	}
	if parsed.Title == "" {
		parsed.Title = parsed.ObjectRef
	}
	return parsed, true
}

func firstRefString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if text, ok := m[key].(string); ok {
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

type completionContractValidation struct {
	MissingOutputs     []string
	MissingHandoffRefs []string
}

func (v completionContractValidation) Satisfied() bool {
	return len(v.MissingOutputs) == 0 && len(v.MissingHandoffRefs) == 0
}

func (s *Service) projectTaskRunWorkProducts(ctx context.Context, tenantID uuid.UUID, task ProjectTask) ([]any, error) {
	if task.DigitalEmployeeRunID == nil {
		return []any{}, nil
	}
	runRepository, ok := s.repository.(ProjectTaskRunWorkProductRepository)
	if !ok {
		return []any{}, nil
	}
	workProducts, err := runRepository.GetProjectTaskRunWorkProducts(ctx, tenantID, *task.DigitalEmployeeRunID)
	if errors.Is(err, ErrProjectNotFound) {
		return []any{}, nil
	}
	return workProducts, err
}

func validateProjectTaskCompletionContract(task ProjectTask, req CompleteProjectTaskRequest, runWorkProducts []any) completionContractValidation {
	required := stringSetFromAny(task.ExpectedOutputs)
	missingOutputs := make([]string, 0)
	if required["execution_summary"] && strings.TrimSpace(req.Conclusion) == "" {
		missingOutputs = append(missingOutputs, "execution_summary")
	}
	if required["evidence_refs"] && len(req.EvidenceRefs) == 0 {
		missingOutputs = append(missingOutputs, "evidence_refs")
	}
	if required["artifact_refs"] && len(req.ArtifactRefs) == 0 {
		missingOutputs = append(missingOutputs, "artifact_refs")
	}
	if required["recommended_next_action"] && strings.TrimSpace(req.RecommendedNextAction) == "" {
		missingOutputs = append(missingOutputs, "recommended_next_action")
	}
	if required["missing_information"] && req.MissingInformation == nil {
		missingOutputs = append(missingOutputs, "missing_information")
	}
	if required["work_products"] && len(runWorkProducts) == 0 {
		missingOutputs = append(missingOutputs, "work_products")
	}
	return completionContractValidation{
		MissingOutputs:     missingOutputs,
		MissingHandoffRefs: missingRequiredHandoffRefs(task.HandoffContract, req, runWorkProducts),
	}
}

func (s *Service) appendProjectTaskContractMissingEvent(ctx context.Context, task ProjectTask, req CompleteProjectTaskRequest, validation completionContractValidation) error {
	payload := map[string]any{
		"project_task_id": task.ID.String(),
		"missing_outputs": stringsToAny(validation.MissingOutputs),
	}
	if len(validation.MissingHandoffRefs) > 0 {
		payload["missing_handoff_refs"] = stringsToAny(validation.MissingHandoffRefs)
	}
	_, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:     req.TenantID,
		ProjectID:    task.ProjectID,
		EventType:    ProjectEventTaskContractMissing,
		ActorType:    "digital_employee",
		ActorID:      req.DigitalEmployeeID.String(),
		ResourceType: strPtr("project_task"),
		ResourceID:   strPtr(task.ID.String()),
		Summary:      "项目任务完成输出未满足交接契约",
		Payload:      payload,
	})
	return err
}

func stringSetFromAny(values []any) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text != "" {
			result[text] = true
		}
	}
	return result
}

func missingRequiredHandoffRefs(contract map[string]any, req CompleteProjectTaskRequest, runWorkProducts []any) []string {
	requiredRefs := requiredHandoffRefs(contract)
	if len(requiredRefs) == 0 {
		return []string{}
	}
	available := referenceTokenSet(req.EvidenceRefs, req.ArtifactRefs, runWorkProducts)
	missing := make([]string, 0)
	for _, ref := range requiredRefs {
		if !available[ref] {
			missing = append(missing, ref)
		}
	}
	return missing
}

func requiredHandoffRefs(contract map[string]any) []string {
	raw, ok := contract["required_refs"]
	if !ok {
		return []string{}
	}
	switch refs := raw.(type) {
	case []string:
		return normalizedStringRefs(refs)
	case []any:
		result := make([]string, 0, len(refs))
		for _, ref := range refs {
			if text, ok := ref.(string); ok {
				text = strings.TrimSpace(text)
				if text != "" {
					result = append(result, text)
				}
			}
		}
		return result
	default:
		return []string{}
	}
}

func normalizedStringRefs(refs []string) []string {
	result := make([]string, 0, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref != "" {
			result = append(result, ref)
		}
	}
	return result
}

func referenceTokenSet(groups ...[]any) map[string]bool {
	tokens := map[string]bool{}
	for _, group := range groups {
		for _, value := range group {
			addReferenceTokens(tokens, value)
		}
	}
	return tokens
}

func addReferenceTokens(tokens map[string]bool, value any) {
	switch typed := value.(type) {
	case string:
		addReferenceToken(tokens, typed)
	case map[string]any:
		for _, key := range []string{"ref", "id", "title", "type"} {
			if text, ok := typed[key].(string); ok {
				addReferenceToken(tokens, text)
			}
		}
	}
}

func addReferenceToken(tokens map[string]bool, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		tokens[value] = true
	}
}

func stringsToAny(values []string) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func (s *Service) FailProjectTask(ctx context.Context, req FailProjectTaskRequest) (*ProjectTask, error) {
	req.FailureSummary = strings.TrimSpace(req.FailureSummary)
	if req.TenantID == uuid.Nil || req.ProjectTaskID == uuid.Nil || req.DigitalEmployeeID == uuid.Nil || req.FailureSummary == "" {
		return nil, ErrInvalidProject
	}
	task, projectRecord, err := s.taskAndProjectForWriteback(ctx, req.TenantID, req.RuntimeNodeID, req.ProjectTaskID, req.DigitalEmployeeID)
	if err != nil {
		return nil, err
	}
	writebackRepository, err := s.projectTaskWritebackRepository()
	if err != nil {
		return nil, err
	}
	result, err := writebackRepository.FailProjectTaskWriteback(ctx, FailProjectTaskWritebackRequest{
		Task: task,
		Event: AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    task.ProjectID,
			EventType:    ProjectEventTaskFailed,
			ActorType:    "digital_employee",
			ActorID:      req.DigitalEmployeeID.String(),
			ResourceType: strPtr("project_task"),
			ResourceID:   strPtr(task.ID.String()),
			Summary:      "项目任务执行失败",
			Payload: map[string]any{
				"project_task_id": task.ID.String(),
				"failure_summary": req.FailureSummary,
			},
		},
		AllowedCurrentStatuses: runtimeWritebackProjectTaskStatuses(),
	})
	if err != nil {
		return nil, err
	}
	if err := s.coordinator.SignalEmployeeTaskFailed(ctx, EmployeeTaskFailedSignal{
		TenantID:       req.TenantID,
		ProjectID:      task.ProjectID,
		ProjectTaskID:  task.ID,
		FailureSummary: req.FailureSummary,
		FailedEventID:  result.Event.ID,
		WorkflowID:     projectRecord.CoordinationWorkflowID,
	}); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, task.ProjectID, "EmployeeTaskFailed", "failed", err, map[string]any{
			"project_task_id": task.ID.String(),
			"failed_event_id": result.Event.ID.String(),
			"failure_summary": req.FailureSummary,
		})
		return nil, err
	}
	return &result.Task, nil
}

func (s *Service) FailProjectTaskAttempt(ctx context.Context, req FailProjectTaskAttemptRequest) (*ProjectTask, error) {
	req.FailureSummary = strings.TrimSpace(req.FailureSummary)
	req.FailureFamily = strings.TrimSpace(req.FailureFamily)
	if req.FailureSummary == "" || req.FailureFamily == "" {
		return nil, ErrInvalidProject
	}
	task, attempt, err := s.validateAttemptRuntimeRequest(ctx, req.ProjectTaskAttemptRuntimeRequest)
	if err != nil {
		return nil, err
	}
	digitalEmployeeID, err := digitalEmployeeIDForProjectTask(task)
	if err != nil {
		return nil, err
	}
	req.DigitalEmployeeID = digitalEmployeeID
	writebackRepository, err := s.projectTaskAttemptWritebackRepository()
	if err != nil {
		return nil, err
	}
	action := projectTaskFailureAction(task, req.FailureFamily, req.Retryable)
	result, err := writebackRepository.RecoverProjectTaskAttemptFailureWriteback(ctx, RecoverProjectTaskAttemptFailureWritebackRequest{
		Task:                  task,
		Attempt:               attempt,
		Failure:               req,
		AttemptTerminalStatus: projectTaskAttemptFailureStatus(req.FailureFamily),
		TaskTargetStatus:      action,
		WaitingReason:         humanWaitReasonForFailureFamily(req.FailureFamily),
		RetryAttemptID:        uuid.New(),
		RetryLeaseToken:       "retry-" + uuid.NewString(),
		RetryIdempotencyKey:   projectTaskRetryIdempotencyKey(task, req.IdempotencyKey),
	})
	if err != nil {
		return nil, err
	}
	if result.Task.Status != ProjectTaskStatusFailed {
		return &result.Task, nil
	}
	projectRecord, err := s.repository.GetProject(ctx, req.TenantID, task.ProjectID)
	if err != nil {
		return nil, err
	}
	if err := s.coordinator.SignalEmployeeTaskFailed(ctx, EmployeeTaskFailedSignal{
		TenantID:       req.TenantID,
		ProjectID:      task.ProjectID,
		ProjectTaskID:  task.ID,
		FailureSummary: req.FailureSummary,
		FailedEventID:  result.Event.ID,
		WorkflowID:     projectRecord.CoordinationWorkflowID,
	}); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, task.ProjectID, "EmployeeTaskFailed", "failed", err, map[string]any{
			"project_task_id": task.ID.String(),
			"failed_event_id": result.Event.ID.String(),
			"failure_summary": req.FailureSummary,
		})
		return nil, err
	}
	return &result.Task, nil
}

func (s *Service) WaitHumanProjectTaskAttempt(ctx context.Context, req WaitHumanProjectTaskAttemptRequest) (*ProjectTask, error) {
	req.Reason = strings.TrimSpace(req.Reason)
	req.Summary = strings.TrimSpace(req.Summary)
	if req.Reason == "" || req.Summary == "" || !validHumanWaitReason(req.Reason) {
		return nil, ErrInvalidProject
	}
	task, attempt, err := s.validateAttemptRuntimeRequest(ctx, req.ProjectTaskAttemptRuntimeRequest)
	if err != nil {
		return nil, err
	}
	digitalEmployeeID, err := digitalEmployeeIDForProjectTask(task)
	if err != nil {
		return nil, err
	}
	if req.DigitalEmployeeID != uuid.Nil && req.DigitalEmployeeID != digitalEmployeeID {
		return nil, ErrProjectTaskForbidden
	}
	req.DigitalEmployeeID = digitalEmployeeID
	projectRecord, err := s.repository.GetProject(ctx, req.TenantID, task.ProjectID)
	if err != nil {
		return nil, err
	}
	writebackRepository, err := s.projectTaskAttemptWritebackRepository()
	if err != nil {
		return nil, err
	}
	result, err := writebackRepository.WaitHumanProjectTaskAttemptWriteback(ctx, WaitHumanProjectTaskAttemptWritebackRequest{
		Task:    task,
		Attempt: attempt,
		Wait:    req,
		Decision: CreateDecisionRequestRequest{
			TenantID:          req.TenantID,
			ProjectID:         task.ProjectID,
			ApprovalRequestID: uuid.Nil,
			CoordinationJobID: task.CoordinationJobID,
			ProjectTaskID:     &task.ID,
			TargetUserID:      projectRecord.HumanOwnerUserID,
			DecisionType:      projectTaskHumanWaitDecisionType(req.Reason),
			TitleSnapshot:     task.Title,
			SummarySnapshot:   req.Summary,
			RiskLevelSnapshot: stringValue(task.RiskLevel),
			StatusSnapshot:    "pending",
		},
	})
	if err != nil {
		return nil, err
	}
	if s.inbox != nil {
		if err := s.inbox.UpsertProjectDecisionRequest(ctx, result.Decision); err != nil {
			return nil, err
		}
	}
	return &result.Task, nil
}

func (s *Service) ResolveProjectTaskHumanWait(ctx context.Context, req ResolveProjectTaskHumanWaitRequest) (*ProjectTask, error) {
	req.Resolution = strings.TrimSpace(req.Resolution)
	req.ResponseSummary = strings.TrimSpace(req.ResponseSummary)
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ProjectTaskID == uuid.Nil || req.ActorUserID == uuid.Nil || req.ResponseSummary == "" || !validHumanWaitResolution(req.Resolution) {
		return nil, ErrInvalidProject
	}
	task, err := s.repository.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
	if err != nil {
		return nil, err
	}
	if task.ProjectID != req.ProjectID || task.Status != ProjectTaskStatusWaitingHuman {
		return nil, ErrProjectConflict
	}
	if req.Resolution == HumanWaitResolutionApprove && (task.WaitingReason == nil || *task.WaitingReason != HumanWaitReasonAcceptanceRequired) {
		return nil, ErrProjectConflict
	}
	projectRecord, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	if req.ActorUserID != projectRecord.HumanOwnerUserID {
		return nil, ErrProjectTaskForbidden
	}
	var currentAttempt ProjectTaskAttempt
	if task.CurrentAttemptID != nil {
		currentAttempt, _ = s.repository.GetProjectTaskAttempt(ctx, req.TenantID, *task.CurrentAttemptID)
	}
	targetStatus := projectTaskHumanWaitResolutionStatus(req.Resolution)
	resolutionRepository, err := s.projectTaskHumanWaitResolutionRepository()
	if err != nil {
		return nil, err
	}
	result, err := resolutionRepository.ResolveProjectTaskHumanWaitWriteback(ctx, ResolveProjectTaskHumanWaitWritebackRequest{
		Task:                task,
		CurrentAttempt:      currentAttempt,
		Resolve:             req,
		TargetStatus:        targetStatus,
		RetryAttemptID:      uuid.New(),
		RetryLeaseToken:     "human-wait-" + uuid.NewString(),
		RetryIdempotencyKey: fmt.Sprintf("project-task:%s:attempt:%d:human-wait:%s", task.ID, task.AttemptCount+1, req.Resolution),
	})
	if err != nil {
		return nil, err
	}
	if targetStatus == ProjectTaskStatusCompleted && req.Resolution == HumanWaitResolutionApprove {
		acceptedResult, linkedTask, recorded, err := s.recordHumanAcceptedProjectTaskResult(ctx, task, result, req)
		if err != nil {
			return nil, err
		}
		if recorded {
			result.Task = linkedTask
		}
		summaryID, err := s.projectTaskHumanWaitCompletionSummaryID(ctx, req, acceptedResult)
		if err != nil {
			return nil, err
		}
		if err := s.coordinator.SignalEmployeeTaskCompleted(ctx, EmployeeTaskCompletedSignal{
			TenantID:           req.TenantID,
			ProjectID:          task.ProjectID,
			ProjectTaskID:      task.ID,
			ExecutionSummaryID: summaryID,
			CompletedEventID:   result.Event.ID,
			WorkflowID:         projectRecord.CoordinationWorkflowID,
		}); err != nil {
			_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, task.ProjectID, "EmployeeTaskCompleted", "failed", err, map[string]any{
				"project_task_id":       task.ID.String(),
				"execution_summary_id":  summaryID.String(),
				"completed_event_id":    result.Event.ID.String(),
				"human_wait_resolution": req.Resolution,
			})
			return nil, err
		}
	}
	return &result.Task, nil
}

func (s *Service) recordHumanAcceptedProjectTaskResult(ctx context.Context, task ProjectTask, writeback ProjectTaskWritebackResult, req ResolveProjectTaskHumanWaitRequest) (*ProjectTaskResult, ProjectTask, bool, error) {
	if task.LatestTaskResultID == nil {
		return nil, ProjectTask{}, false, nil
	}
	latestResult, ok, err := s.findProjectTaskResult(ctx, req.TenantID, task.ProjectID, task.ID, *task.LatestTaskResultID)
	if err != nil {
		return nil, ProjectTask{}, false, err
	}
	if !ok || latestResult.ResultStatus != TaskResultStatusCompleted || latestResult.Decision != TaskResultDecisionWaitingHumanReview {
		return nil, ProjectTask{}, false, nil
	}
	eventID := writeback.Event.ID
	recorded, err := s.repository.RecordProjectTaskResult(ctx, RecordProjectTaskResultRequest{
		TenantID:           req.TenantID,
		ProjectID:          task.ProjectID,
		ProjectTaskID:      task.ID,
		AttemptID:          latestResult.AttemptID,
		ExecutionSummaryID: latestResult.ExecutionSummaryID,
		ResultStatus:       TaskResultStatusCompleted,
		ValidationStatus:   "accepted",
		Decision:           TaskResultDecisionCompleteAccepted,
		Contract:           taskResultContractWithHumanAcceptance(latestResult.Contract, req.ResponseSummary),
		ValidationWarnings: append([]string(nil), latestResult.ValidationWarnings...),
		IdempotencyKey:     humanAcceptedProjectTaskResultIdempotencyKey(latestResult, task),
		CreatedEventID:     &eventID,
	})
	if err != nil {
		return nil, ProjectTask{}, false, s.rollbackHumanAcceptedProjectTaskResult(ctx, task, latestResult.ID, err)
	}
	if task.WaitingRequestID != nil {
		linkedResult, err := s.repository.LinkProjectTaskResultDecisionRequest(ctx, req.TenantID, task.ProjectID, recorded.ID, *task.WaitingRequestID)
		if err != nil {
			return nil, ProjectTask{}, false, s.rollbackHumanAcceptedProjectTaskResult(ctx, task, latestResult.ID, err)
		}
		recorded = linkedResult
	}
	linkedTask, err := s.repository.LinkProjectTaskLatestResult(ctx, req.TenantID, task.ProjectID, task.ID, recorded.ID)
	if err != nil {
		return nil, ProjectTask{}, false, s.rollbackHumanAcceptedProjectTaskResult(ctx, task, latestResult.ID, err)
	}
	return &recorded, linkedTask, true, nil
}

func (s *Service) rollbackHumanAcceptedProjectTaskResult(ctx context.Context, task ProjectTask, latestResultID uuid.UUID, cause error) error {
	if rollbackErr := s.restoreProjectTaskHumanWaitState(ctx, task, latestResultID); rollbackErr != nil {
		return errors.Join(cause, fmt.Errorf("rollback human accepted task result: %w", rollbackErr))
	}
	return cause
}

func (s *Service) restoreProjectTaskHumanWaitState(ctx context.Context, task ProjectTask, latestResultID uuid.UUID) error {
	rollbackErrors := make([]error, 0, 2)
	if latestResultID != uuid.Nil {
		if _, err := s.repository.LinkProjectTaskLatestResult(ctx, task.TenantID, task.ProjectID, task.ID, latestResultID); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("restore previous latest task result: %w", err))
		}
	}
	if _, err := s.repository.UpdateProjectTaskStatus(ctx, task.TenantID, task.ID, ProjectTaskStatusWaitingHuman, nil, []string{ProjectTaskStatusCompleted}); err != nil {
		current, getErr := s.repository.GetProjectTask(ctx, task.TenantID, task.ID)
		if getErr == nil && current.Status == ProjectTaskStatusWaitingHuman {
			return errors.Join(rollbackErrors...)
		}
		if getErr != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("restore project task waiting status: %w; get project task after rollback failure: %v", err, getErr))
		} else {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("restore project task waiting status: %w", err))
		}
	}
	return errors.Join(rollbackErrors...)
}

func (s *Service) findProjectTaskResult(ctx context.Context, tenantID, projectID, projectTaskID, resultID uuid.UUID) (ProjectTaskResult, bool, error) {
	results, err := s.repository.ListProjectTaskResults(ctx, ListProjectTaskResultsRequest{
		TenantID:      tenantID,
		ProjectID:     projectID,
		ProjectTaskID: projectTaskID,
		Limit:         100,
	})
	if err != nil {
		return ProjectTaskResult{}, false, err
	}
	for _, result := range results {
		if result.ID == resultID {
			return result, true, nil
		}
	}
	return ProjectTaskResult{}, false, nil
}

func taskResultContractWithHumanAcceptance(contract TaskResultContract, responseSummary string) TaskResultContract {
	responseSummary = strings.TrimSpace(responseSummary)
	if responseSummary == "" {
		return contract
	}
	if len(contract.AcceptanceResults) == 0 {
		contract.AcceptanceResults = []TaskResultAcceptanceResult{{
			ID:                  "human_acceptance",
			Criterion:           "human_acceptance",
			Status:              TaskResultCriterionStatusHumanOverridden,
			Summary:             "人工验收通过",
			HumanAcceptedReason: responseSummary,
		}}
		return contract
	}
	for i := range contract.AcceptanceResults {
		contract.AcceptanceResults[i].HumanAcceptedReason = responseSummary
	}
	return contract
}

func humanAcceptedProjectTaskResultIdempotencyKey(latestResult ProjectTaskResult, task ProjectTask) string {
	decisionID := "no-decision-request"
	if task.WaitingRequestID != nil {
		decisionID = task.WaitingRequestID.String()
	}
	return fmt.Sprintf("project_task_result:%s:human_acceptance:%s:%s", latestResult.ID, task.ID, decisionID)
}

func (s *Service) projectTaskHumanWaitCompletionSummaryID(ctx context.Context, req ResolveProjectTaskHumanWaitRequest, acceptedResult *ProjectTaskResult) (uuid.UUID, error) {
	if acceptedResult != nil && acceptedResult.ExecutionSummaryID != nil {
		return *acceptedResult.ExecutionSummaryID, nil
	}
	summaries, err := s.repository.ListExecutionSummaries(ctx, req.TenantID, req.ProjectID, 100, 0)
	if err != nil {
		return uuid.Nil, err
	}
	for _, summary := range summaries {
		if summary.ProjectTaskID == req.ProjectTaskID {
			return summary.ID, nil
		}
	}
	return uuid.Nil, nil
}

func projectTaskRequiresAcceptance(task ProjectTask, req CompleteProjectTaskAttemptRequest) bool {
	if req.ResultContract != nil && req.ResultContract.HumanReviewRequest != nil {
		return true
	}
	if task.RequiresHumanApproval {
		return true
	}
	if task.RiskLevel != nil {
		switch strings.ToLower(strings.TrimSpace(*task.RiskLevel)) {
		case "high", "critical":
			return true
		}
	}
	return req.RequiresHumanReview
}

func projectTaskFailureAction(task ProjectTask, failureFamily string, retryable *bool) string {
	if retryable != nil && !*retryable {
		if failureFamily == FailureFamilyBusinessCancelled || failureFamily == FailureFamilyPlanInvalid || failureFamily == FailureFamilyRequirementChanged {
			return ProjectTaskStatusCancelled
		}
		return ProjectTaskStatusFailed
	}
	switch failureFamily {
	case FailureFamilyTransientRuntime, FailureFamilyTransientProvider, FailureFamilyTimeout:
		maxAttempts := int32(1)
		if task.MaxAttempts != nil {
			maxAttempts = *task.MaxAttempts
		}
		if task.AttemptCount < maxAttempts {
			return ProjectTaskStatusQueued
		}
		return ProjectTaskStatusWaitingHuman
	case FailureFamilyInvalidContract, FailureFamilyApprovalRequired, FailureFamilyPermissionRequired, FailureFamilyAcceptanceRequired:
		return ProjectTaskStatusWaitingHuman
	case FailureFamilyBusinessCancelled, FailureFamilyPlanInvalid, FailureFamilyRequirementChanged:
		return ProjectTaskStatusCancelled
	default:
		return ProjectTaskStatusFailed
	}
}

const defaultDispatchRecoveryBackoff = 2 * time.Minute

// RecoverProjectTaskDispatchFailure turns the latest recorded dispatch failure
// of a task without an active attempt into a bounded retry schedule or a
// waiting-human recovery decision. It is idempotent under Temporal activity
// retries: a pending waiting-human decision is never duplicated and a
// not-yet-due retry is never re-scheduled.
func (s *Service) RecoverProjectTaskDispatchFailure(ctx context.Context, req RecoverProjectTaskDispatchFailureRequest) (*RecoverProjectTaskDispatchFailureResult, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ProjectTaskID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	repository, ok := s.repository.(ProjectTaskDispatchRecoveryRepository)
	if !ok {
		return nil, ErrInvalidProject
	}
	task, err := s.repository.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
	if err != nil {
		return nil, err
	}
	if task.ProjectID != req.ProjectID {
		return nil, ErrProjectNotFound
	}
	if task.CurrentAttemptID != nil {
		return &RecoverProjectTaskDispatchFailureResult{Task: task, Action: ProjectTaskRecoveryActionNoop}, nil
	}
	if task.Status == ProjectTaskStatusWaitingHuman && task.WaitingRequestID != nil {
		return &RecoverProjectTaskDispatchFailureResult{Task: task, Action: ProjectTaskRecoveryActionNoop}, nil
	}
	if task.RetryNotBefore != nil && task.RetryNotBefore.After(time.Now().UTC()) {
		return &RecoverProjectTaskDispatchFailureResult{Task: task, Action: ProjectTaskRecoveryActionNoop}, nil
	}
	event, err := repository.GetProjectTaskLatestDispatchFailureEvent(ctx, req.TenantID, req.ProjectID, req.ProjectTaskID)
	if err != nil {
		return nil, err
	}
	if req.FailureEventID != uuid.Nil && event.ID != req.FailureEventID {
		return &RecoverProjectTaskDispatchFailureResult{Task: task, Event: event, Action: ProjectTaskRecoveryActionNoop}, nil
	}
	dispatchFailureCount, err := repository.CountProjectTaskDispatchFailureEvents(ctx, req.TenantID, req.ProjectID, req.ProjectTaskID)
	if err != nil {
		return nil, err
	}
	action := projectTaskDispatchRecoveryAction(task, event, dispatchFailureCount)
	result, err := repository.RecoverProjectTaskDispatchFailure(ctx, RecoverProjectTaskDispatchFailureWritebackRequest{
		TenantID:       req.TenantID,
		ProjectID:      req.ProjectID,
		ProjectTaskID:  req.ProjectTaskID,
		FailureEventID: event.ID,
		Action:         action,
	})
	if err != nil {
		return nil, err
	}
	if result.Decision.ID != uuid.Nil && s.inbox != nil {
		if err := s.inbox.UpsertProjectDecisionRequest(ctx, result.Decision); err != nil {
			return nil, err
		}
	}
	return &RecoverProjectTaskDispatchFailureResult{
		Task:     result.Task,
		Event:    result.Event,
		Decision: result.Decision,
		Action:   action.Action,
	}, nil
}

func (s *Service) RecoverStaleQueuedProjectTaskAttempt(ctx context.Context, req RecoverProjectTaskAttemptRequest) (*ProjectTask, error) {
	if req.FailureFamily == "" {
		req.FailureFamily = FailureFamilyRuntimeStartTimeout
	}
	return s.recoverProjectTaskAttempt(ctx, req, ProjectTaskAttemptStatusLost)
}

func (s *Service) RecoverLostProjectTaskAttempt(ctx context.Context, req RecoverProjectTaskAttemptRequest) (*ProjectTask, error) {
	if req.FailureFamily == "" {
		req.FailureFamily = FailureFamilyRuntimeLeaseLost
	}
	return s.recoverProjectTaskAttempt(ctx, req, ProjectTaskAttemptStatusLost)
}

func (s *Service) SweepStaleQueuedProjectTaskAttempts(ctx context.Context, req SweepProjectTaskAttemptRecoveryRequest) (SweepProjectTaskAttemptRecoveryResult, error) {
	if req.TenantID == uuid.Nil {
		return SweepProjectTaskAttemptRecoveryResult{}, ErrInvalidProject
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	repository, ok := s.repository.(ProjectTaskDispatchRecoveryRepository)
	if !ok {
		return SweepProjectTaskAttemptRecoveryResult{}, ErrInvalidProject
	}
	startedBefore := now.Add(-5 * time.Minute)
	attempts, err := repository.ListStaleQueuedProjectTaskAttempts(ctx, req.TenantID, startedBefore, limit)
	if err != nil {
		return SweepProjectTaskAttemptRecoveryResult{}, err
	}
	return s.recoverAttemptCandidates(ctx, attempts, FailureFamilyRuntimeStartTimeout, "Runtime did not acknowledge project task attempt start before deadline", now)
}

func (s *Service) SweepExpiredRunningProjectTaskAttempts(ctx context.Context, req SweepProjectTaskAttemptRecoveryRequest) (SweepProjectTaskAttemptRecoveryResult, error) {
	if req.TenantID == uuid.Nil {
		return SweepProjectTaskAttemptRecoveryResult{}, ErrInvalidProject
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	repository, ok := s.repository.(ProjectTaskDispatchRecoveryRepository)
	if !ok {
		return SweepProjectTaskAttemptRecoveryResult{}, ErrInvalidProject
	}
	attempts, err := repository.ListExpiredRunningProjectTaskAttempts(ctx, req.TenantID, now, limit)
	if err != nil {
		return SweepProjectTaskAttemptRecoveryResult{}, err
	}
	return s.recoverAttemptCandidates(ctx, attempts, FailureFamilyRuntimeLeaseLost, "Runtime lease expired before terminal writeback", now)
}

func (s *Service) recoverAttemptCandidates(ctx context.Context, attempts []ProjectTaskAttempt, failureFamily, summary string, now time.Time) (SweepProjectTaskAttemptRecoveryResult, error) {
	result := SweepProjectTaskAttemptRecoveryResult{
		RecoveredAttemptIDs: []uuid.UUID{},
		RecoveredTaskIDs:    []uuid.UUID{},
	}
	for _, attempt := range attempts {
		task, err := s.repository.GetProjectTask(ctx, attempt.TenantID, attempt.ProjectTaskID)
		if err != nil {
			return result, err
		}
		recovered, err := s.recoverProjectTaskAttempt(ctx, RecoverProjectTaskAttemptRequest{
			TenantID:      attempt.TenantID,
			ProjectID:     task.ProjectID,
			ProjectTaskID: attempt.ProjectTaskID,
			AttemptID:     attempt.ID,
			FailureFamily: failureFamily,
			Summary:       summary,
			Now:           now,
		}, ProjectTaskAttemptStatusLost)
		if err != nil {
			return result, err
		}
		result.RecoveredAttemptIDs = append(result.RecoveredAttemptIDs, attempt.ID)
		result.RecoveredTaskIDs = append(result.RecoveredTaskIDs, recovered.ID)
	}
	return result, nil
}

func (s *Service) recoverProjectTaskAttempt(ctx context.Context, req RecoverProjectTaskAttemptRequest, attemptTerminalStatus string) (*ProjectTask, error) {
	req.Summary = strings.TrimSpace(req.Summary)
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ProjectTaskID == uuid.Nil || req.AttemptID == uuid.Nil || req.Summary == "" {
		return nil, ErrInvalidProject
	}
	task, err := s.repository.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
	if err != nil {
		return nil, err
	}
	if task.ProjectID != req.ProjectID {
		return nil, ErrProjectNotFound
	}
	attempt, err := s.repository.GetProjectTaskAttempt(ctx, req.TenantID, req.AttemptID)
	if err != nil {
		return nil, err
	}
	writebackRepository, err := s.projectTaskAttemptWritebackRepository()
	if err != nil {
		return nil, err
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	// The replacement attempt carries an explicit backoff so it does not
	// immediately re-queue against a Runtime that may still be down. Retries
	// stay bounded independently: ScheduleProjectTaskRetry increments
	// attempt_count, so projectTaskFailureAction escalates to waiting-human
	// once max_attempts is reached.
	retryAt := now.Add(defaultDispatchRecoveryBackoff)
	retryable := true
	action := projectTaskFailureAction(task, recoveryFailureFamilyForAction(req.FailureFamily), &retryable)
	result, err := writebackRepository.RecoverProjectTaskAttemptFailureWriteback(ctx, RecoverProjectTaskAttemptFailureWritebackRequest{
		Task:                  task,
		Attempt:               attempt,
		Failure:               FailProjectTaskAttemptRequest{ProjectTaskAttemptRuntimeRequest: ProjectTaskAttemptRuntimeRequest{TenantID: req.TenantID, AttemptID: req.AttemptID, ProjectTaskID: req.ProjectTaskID, RuntimeNodeID: uuidValue(attempt.RuntimeNodeID), LeaseToken: attempt.LeaseToken, IdempotencyKey: "recovery-" + req.AttemptID.String()}, FailureSummary: req.Summary, FailureFamily: recoveryFailureFamilyForAction(req.FailureFamily), Retryable: &retryable},
		AttemptTerminalStatus: attemptTerminalStatus,
		TaskTargetStatus:      action,
		WaitingReason:         HumanWaitReasonRuntimeRecovery,
		RetryAttemptID:        uuid.New(),
		RetryLeaseToken:       "retry-" + uuid.NewString(),
		RetryIdempotencyKey:   projectTaskRetryIdempotencyKey(task, "recovery-"+req.AttemptID.String()),
		RetryNotBefore:        &retryAt,
	})
	if err != nil {
		return nil, err
	}
	// The attempt-failure writeback does not create a decision request; a
	// recovery that parks the task waiting-human still needs a durable human
	// next action, so create and project one here.
	if result.Task.Status == ProjectTaskStatusWaitingHuman {
		projectRecord, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
		if err != nil {
			return nil, err
		}
		decision, err := s.repository.CreateDecisionRequest(ctx, CreateDecisionRequestRequest{
			TenantID:          req.TenantID,
			ProjectID:         req.ProjectID,
			ProjectTaskID:     &req.ProjectTaskID,
			TargetUserID:      projectRecord.HumanOwnerUserID,
			DecisionType:      "project_task_recovery",
			TitleSnapshot:     task.Title,
			SummarySnapshot:   req.Summary,
			RiskLevelSnapshot: stringValue(task.RiskLevel),
			StatusSnapshot:    "pending",
			CreatedEventID:    &result.Event.ID,
		})
		if err != nil {
			return nil, err
		}
		if s.inbox != nil {
			if err := s.inbox.UpsertProjectDecisionRequest(ctx, decision); err != nil {
				return nil, err
			}
		}
	}
	return &result.Task, nil
}

func recoveryFailureFamilyForAction(failureFamily string) string {
	switch failureFamily {
	case FailureFamilyRuntimeStartTimeout, FailureFamilyRuntimeLeaseLost:
		return FailureFamilyTransientRuntime
	case FailureFamilyProviderStart:
		return FailureFamilyTransientProvider
	default:
		if strings.TrimSpace(failureFamily) == "" {
			return FailureFamilyTransientRuntime
		}
		return failureFamily
	}
}

func uuidValue(value *uuid.UUID) uuid.UUID {
	if value == nil {
		return uuid.Nil
	}
	return *value
}

func projectTaskDispatchRecoveryAction(task ProjectTask, event ProjectEvent, dispatchFailureCount int64) ProjectTaskRecoveryAction {
	retryable := boolPayload(event.Payload, "retryable", true)
	failureFamily := dispatchRecoveryFailureFamily(event, retryable)
	waitingReason := humanWaitReasonForFailureFamily(failureFamily)
	switch failureFamily {
	case FailureFamilyDispatchTransient, FailureFamilyRuntimeStartTimeout, FailureFamilyRuntimeLeaseLost, FailureFamilyProviderStart, FailureFamilyTransientRuntime, FailureFamilyTransientProvider:
		waitingReason = HumanWaitReasonRuntimeRecovery
	}
	if !retryable {
		return ProjectTaskRecoveryAction{
			Action:        ProjectTaskRecoveryActionWaitingHuman,
			FailureFamily: failureFamily,
			Retryable:     false,
			WaitingReason: waitingReason,
		}
	}
	if projectTaskDispatchRetryAvailable(task, dispatchFailureCount) {
		retryAt := time.Now().UTC().Add(defaultDispatchRecoveryBackoff)
		return ProjectTaskRecoveryAction{
			Action:         ProjectTaskRecoveryActionRetryScheduled,
			FailureFamily:  failureFamily,
			Retryable:      true,
			RetryNotBefore: &retryAt,
			WaitingReason:  waitingReason,
		}
	}
	return ProjectTaskRecoveryAction{
		Action:        ProjectTaskRecoveryActionWaitingHuman,
		FailureFamily: failureFamily,
		Retryable:     true,
		WaitingReason: waitingReason,
	}
}

func boolPayload(payload map[string]any, key string, fallback bool) bool {
	if payload == nil {
		return fallback
	}
	value, ok := payload[key]
	if !ok {
		return fallback
	}
	if b, ok := value.(bool); ok {
		return b
	}
	if s, ok := value.(string); ok {
		return strings.EqualFold(strings.TrimSpace(s), "true")
	}
	return fallback
}

// dispatchRecoveryFailureFamily classifies a dispatch failure. Current
// dispatch_failed events carry "error_family" fixed to "project_task_dispatch"
// (see dispatchFailurePayload), so the "failure_family" passthrough only
// applies to future writers; classification normally comes from error text.
func dispatchRecoveryFailureFamily(event ProjectEvent, retryable bool) string {
	if payloadFamily, ok := event.Payload["failure_family"].(string); ok {
		payloadFamily = strings.TrimSpace(payloadFamily)
		if payloadFamily != "" {
			return payloadFamily
		}
	}
	errorText := strings.ToLower(strings.TrimSpace(stringPayload(event.Payload, "error")))
	switch {
	case strings.Contains(errorText, "permission"), strings.Contains(errorText, "unauthorized"), strings.Contains(errorText, "forbidden"):
		return FailureFamilyPermissionRequired
	case strings.Contains(errorText, "invalid"), strings.Contains(errorText, "contract"):
		return FailureFamilyInvalidContract
	case strings.Contains(errorText, "provider"):
		if retryable {
			return FailureFamilyProviderStart
		}
		return FailureFamilyProviderConfig
	default:
		if retryable {
			return FailureFamilyTransientRuntime
		}
		return FailureFamilyInvalidContract
	}
}

func stringPayload(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	if value, ok := payload[key].(string); ok {
		return value
	}
	return ""
}

// projectTaskDispatchRetryAvailable bounds dispatch retries by the number of
// recorded dispatch_failed events. attempt_count cannot be used here: it only
// increments when an attempt is queued, so it stays 0 for pure dispatch
// failures and would allow unlimited dispatch retries.
func projectTaskDispatchRetryAvailable(task ProjectTask, dispatchFailureCount int64) bool {
	maxAttempts := int64(1)
	if task.MaxAttempts != nil {
		maxAttempts = int64(*task.MaxAttempts)
	}
	return dispatchFailureCount < maxAttempts
}

func projectTaskAttemptFailureStatus(failureFamily string) string {
	switch failureFamily {
	case FailureFamilyTimeout:
		return ProjectTaskAttemptStatusTimedOut
	case FailureFamilyTransientRuntime:
		return ProjectTaskAttemptStatusLost
	default:
		return ProjectTaskAttemptStatusFailed
	}
}

func humanWaitReasonForFailureFamily(failureFamily string) string {
	switch failureFamily {
	case FailureFamilyApprovalRequired:
		return HumanWaitReasonApprovalRequired
	case FailureFamilyPermissionRequired:
		return HumanWaitReasonPermissionRequired
	case FailureFamilyInvalidContract, FailureFamilyPlanInvalid:
		return HumanWaitReasonPlanInvalid
	case FailureFamilyAcceptanceRequired:
		return HumanWaitReasonAcceptanceRequired
	default:
		return HumanWaitReasonClarification
	}
}

func projectTaskRetryIdempotencyKey(task ProjectTask, failureIdempotencyKey string) string {
	return fmt.Sprintf("project-task:%s:attempt:%d:retry:%s", task.ID, task.AttemptCount+1, failureIdempotencyKey)
}

func validHumanWaitReason(reason string) bool {
	switch reason {
	case HumanWaitReasonMissingContext,
		HumanWaitReasonClarification,
		HumanWaitReasonApprovalRequired,
		HumanWaitReasonPermissionRequired,
		HumanWaitReasonPlanInvalid,
		HumanWaitReasonAcceptanceRequired,
		HumanWaitReasonRuntimeRecovery,
		HumanWaitReasonBudgetApproval:
		return true
	default:
		return false
	}
}

func validHumanWaitResolution(resolution string) bool {
	switch resolution {
	case HumanWaitResolutionApprove,
		HumanWaitResolutionResumeSameTask,
		HumanWaitResolutionCancelAndReplan,
		HumanWaitResolutionCancelWithoutPlan,
		HumanWaitResolutionMarkFailed:
		return true
	default:
		return false
	}
}

func projectTaskHumanWaitResolutionStatus(resolution string) string {
	switch resolution {
	case HumanWaitResolutionApprove:
		return ProjectTaskStatusCompleted
	case HumanWaitResolutionResumeSameTask:
		return ProjectTaskStatusQueued
	case HumanWaitResolutionCancelAndReplan, HumanWaitResolutionCancelWithoutPlan:
		return ProjectTaskStatusCancelled
	case HumanWaitResolutionMarkFailed:
		return ProjectTaskStatusFailed
	default:
		return ""
	}
}

func projectTaskHumanWaitDecisionType(reason string) string {
	switch reason {
	case HumanWaitReasonMissingContext:
		return "project_task_missing_context"
	case HumanWaitReasonClarification:
		return "project_task_clarification"
	case HumanWaitReasonApprovalRequired:
		return "project_task_approval"
	case HumanWaitReasonPermissionRequired:
		return "project_task_permission"
	case HumanWaitReasonPlanInvalid:
		return "project_task_plan_invalid"
	case HumanWaitReasonAcceptanceRequired:
		return "project_task_acceptance"
	case HumanWaitReasonRuntimeRecovery:
		return "project_task_runtime_recovery"
	case HumanWaitReasonBudgetApproval:
		return "project_task_budget_approval"
	default:
		return "project_task_human_wait"
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *Service) RequestProjectTaskTransfer(ctx context.Context, req RequestProjectTaskTransferRequest) (*TransferRequest, error) {
	req.Reason = strings.TrimSpace(req.Reason)
	if req.TenantID == uuid.Nil || req.ProjectTaskID == uuid.Nil || req.DigitalEmployeeID == uuid.Nil || req.Reason == "" {
		return nil, ErrInvalidProject
	}
	task, projectRecord, err := s.taskAndProjectForWriteback(ctx, req.TenantID, req.RuntimeNodeID, req.ProjectTaskID, req.DigitalEmployeeID)
	if err != nil {
		return nil, err
	}
	writebackRepository, err := s.projectTaskWritebackRepository()
	if err != nil {
		return nil, err
	}
	result, err := writebackRepository.RequestProjectTaskTransferWriteback(ctx, RequestProjectTaskTransferWritebackRequest{
		Task: task,
		Event: AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    task.ProjectID,
			EventType:    ProjectEventTransferRequested,
			ActorType:    "digital_employee",
			ActorID:      req.DigitalEmployeeID.String(),
			ResourceType: strPtr("project_task"),
			ResourceID:   strPtr(task.ID.String()),
			Summary:      "数字员工请求转派",
			Payload:      map[string]any{"project_task_id": task.ID.String(), "reason": req.Reason},
		},
		Transfer: CreateTransferRequestRequest{
			TenantID:                     req.TenantID,
			ProjectID:                    task.ProjectID,
			ProjectTaskID:                task.ID,
			RequestedByDigitalEmployeeID: req.DigitalEmployeeID,
			Reason:                       req.Reason,
			SuggestedEmployeeType:        strings.TrimSpace(req.SuggestedEmployeeType),
			SuggestedDigitalEmployeeIDs:  req.SuggestedDigitalEmployeeIDs,
			MissingContextRefs:           sliceOrEmptyAny(req.MissingContextRefs),
			Status:                       "requested",
		},
		AllowedCurrentStatuses: runtimeWritebackProjectTaskStatuses(),
	})
	if err != nil {
		return nil, err
	}
	if err := s.coordinator.SignalEmployeeTransferRequested(ctx, EmployeeTransferRequestedSignal{
		TenantID:          req.TenantID,
		ProjectID:         task.ProjectID,
		ProjectTaskID:     task.ID,
		TransferRequestID: result.Transfer.ID,
		RequestedEventID:  result.Event.ID,
		WorkflowID:        projectRecord.CoordinationWorkflowID,
	}); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, task.ProjectID, "EmployeeTransferRequested", "failed", err, map[string]any{
			"project_task_id":     task.ID.String(),
			"transfer_request_id": result.Transfer.ID.String(),
			"requested_event_id":  result.Event.ID.String(),
		})
		return nil, err
	}
	return &result.Transfer, nil
}

func (s *Service) ResolveDecision(ctx context.Context, req ResolveDecisionRequest) (*DecisionRequest, error) {
	req.Decision = strings.TrimSpace(req.Decision)
	req.Comment = strings.TrimSpace(req.Comment)
	req.TargetExitDeliverable = strings.TrimSpace(req.TargetExitDeliverable)
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.DecisionRequestID == uuid.Nil || req.DecidedByUserID == uuid.Nil || !validHumanDecision(req.Decision) {
		return nil, ErrInvalidProject
	}
	projectRecord, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	decision, err := s.findDecisionRequest(ctx, req.TenantID, req.ProjectID, req.DecisionRequestID)
	if err != nil {
		return nil, err
	}
	// request_changes is plan-review vocabulary only: it means "supersede this
	// plan revision and replan". Other decision types (e.g. project_acceptance)
	// must not coerce it into a rejected resolution.
	if req.Decision == PlanReviewDecisionRequestChanges && decision.DecisionType != "plan_review" {
		return nil, ErrInvalidProject
	}
	// restaffed and exempted are planning_gap vocabulary only: they mean "the
	// pool was supplemented" / "the constraint was waived for this demand" —
	// either way, reopen and replan. Other decision types must not accept them.
	if (req.Decision == PlanningGapDecisionRestaffed || req.Decision == PlanningGapDecisionExempted) && decision.DecisionType != DecisionTypePlanningGap {
		return nil, ErrInvalidProject
	}
	// The inverse also holds: a planning_gap decision's vocabulary is closed to
	// restaffed/exempted (reopen+replan) and rejected (关闭). The generic approved /
	// needs_more_evidence have no planning_gap semantics and would strand the
	// decision in an unactionable snapshot, so they are invalid input here.
	if decision.DecisionType == DecisionTypePlanningGap &&
		req.Decision != PlanningGapDecisionRestaffed && req.Decision != PlanningGapDecisionExempted && req.Decision != "rejected" {
		return nil, ErrInvalidProject
	}
	// A non-empty target_exit_deliverable pins the replan's exit — it must name a
	// member of the reviewed plan revision's available_exits. The web Select only
	// ever offers known members, but an authorized plan_review actor can call this
	// endpoint directly; an unvalidated value would reach the coordinator's replan
	// pin verbatim, burn MaxAttempts real planner calls on an unresolvable
	// validation error, and strand the demand in planning_pending untyped.
	if req.Decision == PlanReviewDecisionRequestChanges && req.TargetExitDeliverable != "" {
		if err := s.validateTargetExitDeliverable(ctx, req.TenantID, req.ProjectID, decision, req.TargetExitDeliverable); err != nil {
			return nil, err
		}
	}
	if decision.StatusSnapshot != "pending" {
		if decision.StatusSnapshot == req.Decision {
			if err := s.resolveProjectTaskAcceptanceDecision(ctx, decision, req); err != nil {
				return nil, err
			}
			return &decision, nil
		}
		return nil, ErrInvalidProject
	}
	// exempted persists a first-class DemandConstraintExemption record before any
	// approval/signal side effect — the constraint_kind/roles come from the
	// decision's own recorded gap (ContextPayload), never from the resolving
	// caller's payload, so a human exempts what the system actually diagnosed.
	// Missing/corrupt gap data leaves zero side effects (ErrInvalidProject).
	if req.Decision == PlanningGapDecisionExempted {
		if err := s.createPlanningGapExemption(ctx, req, decision); err != nil {
			return nil, err
		}
	}
	if s.approvals != nil && decision.ApprovalRequestID != uuid.Nil {
		if err := s.approvals.ResolveApproval(ctx, ResolveApprovalRequest{
			TenantID:          req.TenantID,
			ApprovalRequestID: decision.ApprovalRequestID,
			DecidedByUserID:   req.DecidedByUserID,
			Decision:          req.Decision,
			Comment:           req.Comment,
			Payload:           mapOrEmptyAny(req.Payload),
		}); err != nil {
			return nil, err
		}
	}
	event, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:     req.TenantID,
		ProjectID:    req.ProjectID,
		EventType:    ProjectEventDecisionSubmitted,
		ActorType:    "human_user",
		ActorID:      req.DecidedByUserID.String(),
		ResourceType: strPtr("decision_request"),
		ResourceID:   strPtr(req.DecisionRequestID.String()),
		Summary:      "人类决策已提交",
		Payload:      map[string]any{"decision": req.Decision, "comment": req.Comment, "payload": mapOrEmptyAny(req.Payload)},
	})
	if err != nil {
		return nil, err
	}
	resolved, err := s.repository.ResolveDecisionRequest(ctx, ResolveDecisionRequestRepositoryRequest{
		TenantID:        req.TenantID,
		ProjectID:       req.ProjectID,
		ID:              req.DecisionRequestID,
		StatusSnapshot:  req.Decision,
		ResolvedEventID: &event.ID,
	})
	if err != nil {
		return nil, err
	}
	if s.inbox != nil {
		if err := s.inbox.ResolveProjectDecisionRequest(ctx, resolved); err != nil {
			return nil, err
		}
	}
	if err := s.resolveProjectTaskAcceptanceDecision(ctx, resolved, req); err != nil {
		return nil, err
	}
	if err := s.coordinator.SignalHumanDecisionSubmitted(ctx, HumanDecisionSubmittedSignal{
		TenantID:              req.TenantID,
		ProjectID:             req.ProjectID,
		ApprovalRequestID:     decision.ApprovalRequestID,
		DecisionRequestID:     req.DecisionRequestID,
		Decision:              req.Decision,
		Payload:               mapOrEmptyAny(req.Payload),
		ResolvedEventID:       event.ID,
		WorkflowID:            projectRecord.CoordinationWorkflowID,
		TargetExitDeliverable: req.TargetExitDeliverable,
	}); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, req.ProjectID, "HumanDecisionSubmitted", "failed", err, map[string]any{
			"approval_request_id": decision.ApprovalRequestID.String(),
			"decision_request_id": req.DecisionRequestID.String(),
			"resolved_event_id":   event.ID.String(),
			"decision":            req.Decision,
			"payload":             mapOrEmptyAny(req.Payload),
		})
		return nil, err
	}
	return &resolved, nil
}

// createPlanningGapExemption persists the DemandConstraintExemption for a
// planning_gap decision resolved "exempted". It reads demand_id and the
// structured gap (constraint_kind/roles) from the approval request's
// ContextPayload — the record ensurePlanningGapDecision wrote when the gap was
// first detected — never from req.Payload: a human exempts what the system
// actually diagnosed, not an arbitrary claim typed into the resolve call. If the
// approval resolver is unset, or the payload carries no demand_id or no
// structured gap (e.g. a legacy/non-structural no-suitable-employee diagnosis
// that predates PlanningGap, or any other planning_gap channel with nothing to
// exempt), this returns ErrInvalidProject with zero side effects — the caller
// (ResolveDecision) must not have run any approval/event/signal writes yet.
func (s *Service) createPlanningGapExemption(ctx context.Context, req ResolveDecisionRequest, decision DecisionRequest) error {
	if s.approvals == nil {
		return ErrInvalidProject
	}
	contextPayload, err := s.approvals.GetRequestContextPayload(ctx, req.TenantID, decision.ApprovalRequestID)
	if err != nil {
		return err
	}
	demandID, constraintKind, roles, ok := parsePlanningGapExemptionContext(contextPayload)
	if !ok {
		return ErrInvalidProject
	}
	decisionRequestID := req.DecisionRequestID
	return s.repository.CreateDemandConstraintExemption(ctx, CreateDemandConstraintExemptionRequest{
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		DemandID:          demandID,
		ConstraintKind:    constraintKind,
		Roles:             roles,
		GrantedByUserID:   req.DecidedByUserID,
		DecisionRequestID: &decisionRequestID,
	})
}

// parsePlanningGapExemptionContext extracts demand_id and the structured gap's
// constraint_kind/roles from a planning_gap approval request's ContextPayload
// (shaped by planningGapPayload in workflow/projectcoordination/project_store.go:
// {"demand_id": "...", "diagnosis": "...", "gap": {"constraint_kind": "...",
// "roles": [...], ...}}). ok is false when demand_id is missing/unparseable, or
// when "gap" is absent, not an object, or carries a blank constraint_kind — there
// is nothing to exempt.
func parsePlanningGapExemptionContext(payload map[string]any) (demandID uuid.UUID, constraintKind string, roles []string, ok bool) {
	demandIDRaw, _ := payload["demand_id"].(string)
	demandID, err := uuid.Parse(strings.TrimSpace(demandIDRaw))
	if err != nil {
		return uuid.Nil, "", nil, false
	}
	gapRaw, exists := payload["gap"]
	if !exists {
		return uuid.Nil, "", nil, false
	}
	gapMap, isMap := gapRaw.(map[string]any)
	if !isMap {
		return uuid.Nil, "", nil, false
	}
	kind, _ := gapMap["constraint_kind"].(string)
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return uuid.Nil, "", nil, false
	}
	rolesRaw, _ := gapMap["roles"].([]any)
	parsedRoles := make([]string, 0, len(rolesRaw))
	for _, entry := range rolesRaw {
		if role, isString := entry.(string); isString && strings.TrimSpace(role) != "" {
			parsedRoles = append(parsedRoles, role)
		}
	}
	return demandID, kind, parsedRoles, true
}

// validateTargetExitDeliverable rejects a target_exit_deliverable that is not a
// member of the reviewed plan revision's available_exits (payload.available_exits).
// An unbound/legacy plan revision (no available_exits, or the decision predates
// PlanRevisionID linkage) also rejects any non-empty target — there is nothing to
// validate membership against.
func (s *Service) validateTargetExitDeliverable(ctx context.Context, tenantID, projectID uuid.UUID, decision DecisionRequest, target string) error {
	if decision.PlanRevisionID == nil {
		return fmt.Errorf("改选出口不在该计划的可选出口中: %w", ErrInvalidProject)
	}
	revision, err := s.repository.GetPlanRevision(ctx, tenantID, projectID, *decision.PlanRevisionID)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return fmt.Errorf("改选出口不在该计划的可选出口中: %w", ErrInvalidProject)
		}
		return err
	}
	for _, deliverable := range planRevisionAvailableExitDeliverables(revision.Payload) {
		if deliverable == target {
			return nil
		}
	}
	return fmt.Errorf("改选出口不在该计划的可选出口中: %w", ErrInvalidProject)
}

// planRevisionAvailableExitDeliverables extracts the deliverable keys from a plan
// revision payload's available_exits (see PlanRevisionPayload.AvailableExits /
// PlanExitOption in projectcoordination), as decoded from stored JSON.
func planRevisionAvailableExitDeliverables(payload map[string]any) []string {
	raw, _ := payload["available_exits"].([]any)
	deliverables := make([]string, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		deliverable, _ := entry["deliverable"].(string)
		deliverable = strings.TrimSpace(deliverable)
		if deliverable != "" {
			deliverables = append(deliverables, deliverable)
		}
	}
	return deliverables
}

func (s *Service) resolveProjectTaskAcceptanceDecision(ctx context.Context, decision DecisionRequest, req ResolveDecisionRequest) error {
	if decision.DecisionType != "project_task_acceptance" || req.Decision != "approved" || decision.ProjectTaskID == nil {
		return nil
	}
	_, err := s.ResolveProjectTaskHumanWait(ctx, ResolveProjectTaskHumanWaitRequest{
		TenantID:        req.TenantID,
		ProjectID:       req.ProjectID,
		ProjectTaskID:   *decision.ProjectTaskID,
		ActorUserID:     req.DecidedByUserID,
		Resolution:      HumanWaitResolutionApprove,
		ResponseSummary: projectTaskAcceptanceResponseSummary(req.Comment),
	})
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrProjectConflict) {
		task, getErr := s.repository.GetProjectTask(ctx, req.TenantID, *decision.ProjectTaskID)
		if getErr == nil && task.ProjectID == req.ProjectID && task.Status == ProjectTaskStatusCompleted {
			return nil
		}
	}
	return err
}

// SignDemandCriterionVerdict records one human sign-off against a
// snapshotted blocking human_judgment acceptance criterion for a demand
// parked at acceptance_pending by the convergence gate (see
// gatedCompletionStatus / ensureDemandAcceptanceDecision). Preconditions
// (each a distinct error so the handler can map a specific status code):
//   - demand.Status must be acceptance_pending (else ErrProjectConflict), OR
//     already terminal — see the reconciliation note below.
//   - a pending demand_acceptance decision must exist for the demand's
//     current effective plan revision (else ErrProjectNotFound).
//   - the caller must be the decision's TargetUserID or the project's
//     human_owner (else ErrProjectDecisionForbidden) — mirrors
//     ResolveProjectTaskHumanWait's actor check.
//   - criterion_id must name a blocking, human_judgment criterion in the
//     revision's snapshot (else ErrInvalidProject).
//
// Re-signing the same criterion with the same verdict is idempotent; re-signing
// with a different verdict is ErrProjectConflict — Phase 1 has no re-judgement.
//
// A satisfied verdict recomputes the demand via
// PgRepository.RecomputeProjectDemandStatus (the convergence gate reads ALL
// blocking criteria, human_judgment or not, so completion is driven by the
// gate, not by this criterion's human_judgment subset alone). An unsatisfied
// verdict on a blocking criterion fails the demand immediately regardless of
// any other still-unsigned criteria — the gate itself can only ever produce
// "still pending" or "completed", never "failed", so that transition is forced
// here.
//
// Retry-recoverability: the demand-advance, decision-resolve and
// project-acceptance-review-open are three separate non-atomic writes. If a
// prior attempt died mid-sequence the demand can be left half-converged
// (advanced but decision still pending, or verdict written but demand still
// acceptance_pending). This method heals either partial state on any retry:
//   - demand already terminal + pending decision still open → resolve the
//     decision to match the terminal status (reconcileTerminalDemandSignOff).
//   - verdict already written + demand still acceptance_pending → re-run the
//     recompute+advance+resolve convergence idempotently rather than
//     early-returning (convergeDemandSignOff, gated on the still-pending
//     decision so nothing is double-resolved).
func (s *Service) SignDemandCriterionVerdict(ctx context.Context, req SignDemandCriterionVerdictRequest) (*SignDemandCriterionVerdictResult, error) {
	req.CriterionID = strings.TrimSpace(req.CriterionID)
	req.Verdict = strings.TrimSpace(req.Verdict)
	req.Reason = strings.TrimSpace(req.Reason)
	if req.TenantID == uuid.Nil || req.DemandID == uuid.Nil || req.ActorUserID == uuid.Nil || req.CriterionID == "" || !validDemandCriterionVerdict(req.Verdict) {
		return nil, ErrInvalidProject
	}
	demand, err := s.repository.GetProjectDemand(ctx, req.TenantID, req.DemandID)
	if err != nil {
		return nil, err
	}
	projectRecord, err := s.repository.GetProject(ctx, req.TenantID, demand.ProjectID)
	if err != nil {
		return nil, err
	}
	revisions, err := s.repository.ListPlanRevisionsForDemand(ctx, req.TenantID, demand.ProjectID, req.DemandID)
	if err != nil {
		return nil, err
	}
	revisionID := CurrentEffectivePlanRevisionID(revisions)
	if revisionID == uuid.Nil {
		return nil, ErrProjectConflict
	}
	criteria, err := s.repository.ListDemandAcceptanceCriteria(ctx, req.TenantID, req.DemandID, revisionID)
	if err != nil {
		return nil, err
	}
	verdicts, err := s.repository.ListDemandCriterionVerdicts(ctx, req.TenantID, req.DemandID, revisionID)
	if err != nil {
		return nil, err
	}
	humanJudgmentCriteria := blockingHumanJudgmentCriteria(criteria)

	// Reconciliation: a prior attempt already advanced the demand to a terminal
	// status but may have died before resolving the pending decision / opening
	// the project acceptance review. Heal idempotently.
	if demand.Status == ProjectDemandStatusCompleted || demand.Status == ProjectDemandStatusFailed {
		return s.reconcileTerminalDemandSignOff(ctx, req, demand, projectRecord, revisionID, criteria, humanJudgmentCriteria, verdicts)
	}
	if demand.Status != ProjectDemandStatusAcceptancePending {
		return nil, ErrProjectConflict
	}

	decision, err := s.repository.GetPendingDemandAcceptanceDecisionByPlanRevision(ctx, req.TenantID, demand.ProjectID, revisionID)
	if err != nil {
		return nil, err
	}
	if req.ActorUserID != decision.TargetUserID && req.ActorUserID != projectRecord.HumanOwnerUserID {
		return nil, ErrProjectDecisionForbidden
	}
	criterion := findDemandAcceptanceCriterion(criteria, req.CriterionID)
	if criterion == nil || criterion.Severity != demandAcceptanceCriterionSeverityBlocking || criterion.VerificationMethod != demandCriterionVerificationMethodHumanJudgment {
		return nil, ErrInvalidProject
	}
	if existing := findHumanCriterionVerdict(verdicts, req.CriterionID); existing != nil {
		if existing.Verdict != req.Verdict {
			return nil, ErrProjectConflict // re-judgement — Phase 1 unsupported
		}
		// Same-value replay: do NOT early-return. A prior attempt may have written
		// this verdict then died before advancing/resolving, leaving the demand
		// stuck acceptance_pending. Fall through and re-run convergence — every
		// step below is idempotent.
	} else {
		if err := s.repository.CreateDemandCriterionVerdict(ctx, CreateDemandCriterionVerdictRequest{
			TenantID:       req.TenantID,
			ProjectID:      demand.ProjectID,
			DemandID:       req.DemandID,
			PlanRevisionID: revisionID,
			CriterionID:    req.CriterionID,
			Verdict:        req.Verdict,
			JudgeType:      demandCriterionJudgeTypeHuman,
			JudgeID:        req.ActorUserID,
			Reason:         req.Reason,
		}); err != nil {
			return nil, err
		}
		verdicts = append(verdicts, DemandCriterionVerdict{
			DemandID:       req.DemandID,
			PlanRevisionID: revisionID,
			CriterionID:    req.CriterionID,
			Verdict:        req.Verdict,
			JudgeType:      demandCriterionJudgeTypeHuman,
			JudgeID:        req.ActorUserID,
			Reason:         req.Reason,
		})
	}
	return s.convergeDemandSignOff(ctx, req, demand, revisionID, criterion, humanJudgmentCriteria, verdicts)
}

// convergeDemandSignOff drives the acceptance_pending demand toward its terminal
// status after a verdict is on record, then resolves the pending decision and
// opens the project acceptance review. Ordering is chosen so a crash between any
// two writes heals on retry (see SignDemandCriterionVerdict's reconciliation):
// the demand-advance runs before the decision-resolve, and both are gated so a
// repeat is a no-op. On completion this is also the only place the project-level
// acceptance review is (re)evaluated for a sign-off-driven convergence — the
// coordinator's EmployeeTaskCompleted acceptance re-check does not fire when the
// last demand converges via human sign-off rather than a task event.
func (s *Service) convergeDemandSignOff(ctx context.Context, req SignDemandCriterionVerdictRequest, demand ProjectDemand, revisionID uuid.UUID, criterion *DemandAcceptanceCriterion, humanJudgmentCriteria []DemandAcceptanceCriterion, verdicts []DemandCriterionVerdict) (*SignDemandCriterionVerdictResult, error) {
	targetStatus := demand.Status
	if req.Verdict == demandCriterionVerdictUnsatisfied {
		// A blocking criterion signed unsatisfied fails the demand — the gate
		// never yields "failed", so advance explicitly (forward-only idempotent),
		// then resolve the decision. Reconciliation heals a crash between them.
		if err := s.repository.AdvanceProjectDemandStatus(ctx, req.TenantID, demand.ProjectID, req.DemandID, ProjectDemandStatusFailed); err != nil {
			return nil, err
		}
		if err := s.resolveDemandAcceptanceDecisionIfPending(ctx, req.TenantID, demand.ProjectID, req.DemandID, revisionID, "rejected", req.ActorUserID, req.Reason, criterion); err != nil {
			return nil, err
		}
		targetStatus = ProjectDemandStatusFailed
	} else {
		if err := s.repository.RecomputeProjectDemandStatus(ctx, req.TenantID, demand.ProjectID, req.DemandID); err != nil {
			return nil, err
		}
		updatedDemand, err := s.repository.GetProjectDemand(ctx, req.TenantID, req.DemandID)
		if err != nil {
			return nil, err
		}
		targetStatus = updatedDemand.Status
		if targetStatus == ProjectDemandStatusCompleted {
			if err := s.resolveDemandAcceptanceDecisionIfPending(ctx, req.TenantID, demand.ProjectID, req.DemandID, revisionID, "approved", req.ActorUserID, req.Reason, nil); err != nil {
				return nil, err
			}
		}
	}
	if targetStatus == ProjectDemandStatusCompleted || targetStatus == ProjectDemandStatusFailed {
		if err := s.maybeOpenProjectAcceptanceReview(ctx, req.TenantID, demand.ProjectID); err != nil {
			return nil, err
		}
	}
	signed, total := demandAcceptanceHumanProgress(humanJudgmentCriteria, verdicts)
	return &SignDemandCriterionVerdictResult{
		DemandID:     req.DemandID,
		DemandStatus: targetStatus,
		CriterionID:  req.CriterionID,
		Verdict:      req.Verdict,
		Signed:       signed,
		Total:        total,
		Remaining:    total - signed,
	}, nil
}

// reconcileTerminalDemandSignOff heals a demand that a prior attempt already
// advanced to completed/failed but may have left with an unresolved
// demand_acceptance decision and/or an unopened project acceptance review. It
// resolves any still-pending decision to match the terminal status and (re)opens
// the project review — both idempotent — then returns 200 with current progress.
func (s *Service) reconcileTerminalDemandSignOff(ctx context.Context, req SignDemandCriterionVerdictRequest, demand ProjectDemand, projectRecord Project, revisionID uuid.UUID, criteria, humanJudgmentCriteria []DemandAcceptanceCriterion, verdicts []DemandCriterionVerdict) (*SignDemandCriterionVerdictResult, error) {
	resolution := "approved"
	var rejectedCriterion *DemandAcceptanceCriterion
	if demand.Status == ProjectDemandStatusFailed {
		resolution = "rejected"
		rejectedCriterion = findRejectedHumanCriterion(criteria, verdicts)
	}
	decision, err := s.repository.GetPendingDemandAcceptanceDecisionByPlanRevision(ctx, req.TenantID, demand.ProjectID, revisionID)
	switch {
	case err == nil:
		if req.ActorUserID != decision.TargetUserID && req.ActorUserID != projectRecord.HumanOwnerUserID {
			return nil, ErrProjectDecisionForbidden
		}
		if err := s.resolveDemandAcceptanceDecisionIfPending(ctx, req.TenantID, demand.ProjectID, req.DemandID, revisionID, resolution, req.ActorUserID, req.Reason, rejectedCriterion); err != nil {
			return nil, err
		}
	case errors.Is(err, ErrProjectNotFound):
		// Decision already resolved by a prior attempt — nothing to reconcile there.
	default:
		return nil, err
	}
	if err := s.maybeOpenProjectAcceptanceReview(ctx, req.TenantID, demand.ProjectID); err != nil {
		return nil, err
	}
	signed, total := demandAcceptanceHumanProgress(humanJudgmentCriteria, verdicts)
	return &SignDemandCriterionVerdictResult{
		DemandID:     req.DemandID,
		DemandStatus: demand.Status,
		CriterionID:  req.CriterionID,
		Verdict:      req.Verdict,
		Signed:       signed,
		Total:        total,
		Remaining:    total - signed,
	}, nil
}

// resolveDemandAcceptanceDecisionIfPending resolves the demand's still-pending
// demand_acceptance decision to `resolution` (approved/rejected), appending the
// matching structured event, and is a no-op when no pending decision remains
// (already resolved by a prior attempt). It is the idempotent convergence
// primitive both the fresh sign path (convergeDemandSignOff) and the crash-retry
// reconciliation (reconcileTerminalDemandSignOff) share.
//
// Order within: append event → resolve decision (pending-guarded) → resolve
// inbox → resolve approval. This deliberately resolves the decision projection
// before the upstream approval (the reverse of ResolveDecision) so that once the
// decision is resolved a retry finds no pending decision and skips the whole
// block — the approval is never resolved twice. The residual crash windows are
// benign: a duplicate audit event (append then crash before resolve) or a rare
// stranded-pending approval (resolve decision then crash before approval), never
// a stuck demand/project.
func (s *Service) resolveDemandAcceptanceDecisionIfPending(ctx context.Context, tenantID, projectID, demandID, revisionID uuid.UUID, resolution string, actorID uuid.UUID, reason string, rejectedCriterion *DemandAcceptanceCriterion) error {
	decision, err := s.repository.GetPendingDemandAcceptanceDecisionByPlanRevision(ctx, tenantID, projectID, revisionID)
	if errors.Is(err, ErrProjectNotFound) {
		return nil // already resolved — converged
	}
	if err != nil {
		return err
	}
	var event ProjectEvent
	if resolution == "approved" {
		event, err = s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
			TenantID:     tenantID,
			ProjectID:    projectID,
			EventType:    ProjectEventDemandAcceptanceCompleted,
			ActorType:    "human_user",
			ActorID:      actorID.String(),
			ResourceType: strPtr("project_demand"),
			ResourceID:   strPtr(demandID.String()),
			Summary:      "需求验收判据全部签署通过",
			Payload:      map[string]any{"demand_id": demandID.String(), "decision_request_id": decision.ID.String()},
		})
	} else {
		payload := map[string]any{"demand_id": demandID.String(), "reason": reason}
		if rejectedCriterion != nil {
			payload["criterion_id"] = rejectedCriterion.CriterionID
			payload["statement"] = rejectedCriterion.Statement
		}
		event, err = s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
			TenantID:     tenantID,
			ProjectID:    projectID,
			EventType:    ProjectEventDemandAcceptanceRejected,
			ActorType:    "human_user",
			ActorID:      actorID.String(),
			ResourceType: strPtr("project_demand"),
			ResourceID:   strPtr(demandID.String()),
			Summary:      "需求验收判据签署未通过",
			Payload:      payload,
		})
	}
	if err != nil {
		return err
	}
	resolved, err := s.repository.ResolveDecisionRequest(ctx, ResolveDecisionRequestRepositoryRequest{
		TenantID:        tenantID,
		ProjectID:       projectID,
		ID:              decision.ID,
		StatusSnapshot:  resolution,
		ResolvedEventID: &event.ID,
	})
	if err != nil {
		return err
	}
	if s.inbox != nil {
		if err := s.inbox.ResolveProjectDecisionRequest(ctx, resolved); err != nil {
			return err
		}
	}
	if s.approvals != nil && decision.ApprovalRequestID != uuid.Nil {
		if err := s.approvals.ResolveApproval(ctx, ResolveApprovalRequest{
			TenantID:          tenantID,
			ApprovalRequestID: decision.ApprovalRequestID,
			DecidedByUserID:   actorID,
			Decision:          resolution,
			Comment:           reason,
		}); err != nil {
			return err
		}
	}
	return nil
}

// maybeOpenProjectAcceptanceReview opens the project-level acceptance review
// when a criterion sign-off makes the LAST demand terminal. This closes a
// loop-break introduced by the acceptance-criteria gate: before it, the last
// task completing drove the demand terminal inside the coordinator's
// EmployeeTaskCompleted handler, which then re-checked project acceptance; now
// the last task completing parks the demand at acceptance_pending (no project
// re-check), and the terminal transition happens later via this sign-off
// endpoint — outside any coordinator signal. So the review is opened here
// directly, mirroring ProjectStore.RequestProjectAcceptanceReview's fields.
//
// Idempotent: the running→acceptance transition is the guard. If the project is
// no longer running (already in acceptance/terminal, or a concurrent coordinator
// open won the race), this is a no-op. Bare-repository callers with no approval
// sink wired skip it entirely (nothing can open a review).
//
// This deliberately does NOT reproduce the coordinator's
// ensureFinalDemandSummariesForAcceptance enrichment — final demand summaries
// are a display nicety, not a correctness requirement for un-sticking the
// project; a demand converging via sign-off simply won't have that coordinator-
// generated summary. Tracked as a follow-up, not a blocker.
func (s *Service) maybeOpenProjectAcceptanceReview(ctx context.Context, tenantID, projectID uuid.UUID) error {
	if s.approvals == nil {
		return nil
	}
	ready, err := s.repository.AreAllProjectDemandsTerminal(ctx, tenantID, projectID)
	if err != nil {
		return err
	}
	if !ready {
		return nil
	}
	projectRecord, err := s.repository.GetProject(ctx, tenantID, projectID)
	if err != nil {
		return err
	}
	if _, err := s.repository.TransitionProjectStatus(ctx, tenantID, projectID, []string{string(ProjectStatusRunning)}, string(ProjectStatusAcceptance)); err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return nil // not running → already opened/terminal, nothing to do
		}
		return err
	}
	rollbackToRunning := func() {
		_, _ = s.repository.TransitionProjectStatus(ctx, tenantID, projectID, []string{string(ProjectStatusAcceptance)}, string(ProjectStatusRunning))
	}
	targetUserID := projectRecord.HumanOwnerUserID
	if projectRecord.AcceptanceUserID != nil && *projectRecord.AcceptanceUserID != uuid.Nil {
		targetUserID = *projectRecord.AcceptanceUserID
	}
	approvalRequestID, err := s.approvals.CreateRequest(ctx, CreateApprovalRequestInput{
		TenantID:      tenantID,
		ResourceType:  "project",
		ResourceID:    projectID,
		RequesterType: "project_coordinator",
		TargetUserID:  targetUserID,
		DecisionType:  "project_acceptance",
		Title:         "验收项目交付",
		Summary:       "项目全部需求已完成,请确认验收",
		RiskLevel:     "high",
		Options:       []any{"approved", "rejected", "needs_more_evidence"},
	})
	if err != nil {
		rollbackToRunning()
		return err
	}
	event, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: ProjectEventDecisionRequested,
		ActorType: "project_coordinator",
		ActorID:   "project_coordinator",
		Summary:   "项目进入待验收,等待人类确认",
		Payload: map[string]any{
			"approval_request_id": approvalRequestID.String(),
			"project_id":          projectID.String(),
			"target_user_id":      targetUserID.String(),
		},
	})
	if err != nil {
		rollbackToRunning()
		return err
	}
	decision, err := s.repository.CreateDecisionRequest(ctx, CreateDecisionRequestRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: approvalRequestID,
		TargetUserID:      targetUserID,
		DecisionType:      "project_acceptance",
		TitleSnapshot:     "验收项目交付",
		SummarySnapshot:   "项目全部需求已完成,请确认验收",
		RiskLevelSnapshot: "high",
		StatusSnapshot:    "pending",
		CreatedEventID:    &event.ID,
	})
	if err != nil {
		rollbackToRunning()
		return err
	}
	if s.inbox != nil {
		if err := s.inbox.UpsertProjectDecisionRequest(ctx, decision); err != nil {
			return err
		}
	}
	return nil
}

// findRejectedHumanCriterion returns the criterion a human signed unsatisfied,
// used to populate the demand.acceptance_rejected event during reconciliation of
// an already-failed demand (the fresh reject path carries the criterion directly).
func findRejectedHumanCriterion(criteria []DemandAcceptanceCriterion, verdicts []DemandCriterionVerdict) *DemandAcceptanceCriterion {
	for _, v := range verdicts {
		if v.JudgeType == demandCriterionJudgeTypeHuman && v.ProjectTaskID == nil && v.Verdict == demandCriterionVerdictUnsatisfied {
			if c := findDemandAcceptanceCriterion(criteria, v.CriterionID); c != nil {
				return c
			}
		}
	}
	return nil
}

// findDemandAcceptanceCriterion looks up a snapshotted criterion by its
// payload-side criterion_id (not the snapshot row's own UUID).
func findDemandAcceptanceCriterion(criteria []DemandAcceptanceCriterion, criterionID string) *DemandAcceptanceCriterion {
	for i := range criteria {
		if criteria[i].CriterionID == criterionID {
			return &criteria[i]
		}
	}
	return nil
}

// blockingHumanJudgmentCriteria filters a revision's criterion snapshot down
// to the ones Service.SignDemandCriterionVerdict accepts sign-off against —
// the denominator for sign-off progress (Total/Signed/Remaining).
func blockingHumanJudgmentCriteria(criteria []DemandAcceptanceCriterion) []DemandAcceptanceCriterion {
	result := make([]DemandAcceptanceCriterion, 0, len(criteria))
	for _, c := range criteria {
		if c.Severity == demandAcceptanceCriterionSeverityBlocking && c.VerificationMethod == demandCriterionVerificationMethodHumanJudgment {
			result = append(result, c)
		}
	}
	return result
}

// findHumanCriterionVerdict returns the existing human sign-off (ProjectTaskID
// nil) for a criterion, if any — at most one can exist per
// uq_demand_verdicts_human (migration 064).
func findHumanCriterionVerdict(verdicts []DemandCriterionVerdict, criterionID string) *DemandCriterionVerdict {
	for i := range verdicts {
		if verdicts[i].CriterionID == criterionID && verdicts[i].JudgeType == demandCriterionJudgeTypeHuman && verdicts[i].ProjectTaskID == nil {
			return &verdicts[i]
		}
	}
	return nil
}

// demandAcceptanceHumanProgress reports how many of the demand's blocking
// human_judgment criteria (humanJudgmentCriteria) already carry a satisfied
// human verdict, out of the total.
func demandAcceptanceHumanProgress(humanJudgmentCriteria []DemandAcceptanceCriterion, verdicts []DemandCriterionVerdict) (signed, total int32) {
	satisfied := make(map[string]bool, len(verdicts))
	for _, v := range verdicts {
		if v.JudgeType == demandCriterionJudgeTypeHuman && v.ProjectTaskID == nil && v.Verdict == demandCriterionVerdictSatisfied {
			satisfied[v.CriterionID] = true
		}
	}
	for _, c := range humanJudgmentCriteria {
		if satisfied[c.CriterionID] {
			signed++
		}
	}
	return signed, int32(len(humanJudgmentCriteria))
}

func validDemandCriterionVerdict(verdict string) bool {
	switch verdict {
	case demandCriterionVerdictSatisfied, demandCriterionVerdictUnsatisfied:
		return true
	default:
		return false
	}
}

func projectTaskAcceptanceResponseSummary(comment string) string {
	comment = strings.TrimSpace(comment)
	if comment != "" {
		return comment
	}
	return "任务验收通过"
}

func (s *Service) RetryWorkflowSignal(ctx context.Context, req RetryWorkflowSignalRequest) (*ProjectEvent, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.EventID == uuid.Nil || req.ActorID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	event, err := s.repository.GetProjectEvent(ctx, req.TenantID, req.ProjectID, req.EventID)
	if err != nil {
		return nil, err
	}
	if event.EventType != ProjectEventWorkflowSignaled {
		return nil, ErrInvalidProject
	}
	signalName, _ := event.Payload["signal_name"].(string)
	status, _ := event.Payload["status"].(string)
	retryable, _ := event.Payload["retryable"].(bool)
	if signalName == "" || status != "failed" || !retryable {
		return nil, ErrInvalidProject
	}
	projectRecord, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureProjectCoordinator(ctx, projectRecord); err != nil {
		retryPayload := cloneMap(event.Payload)
		retryPayload["retry_of_event_id"] = req.EventID.String()
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, req.ProjectID, signalName, "failed", err, retryPayload)
		return nil, err
	}
	if err := s.retryWorkflowSignal(ctx, projectRecord, signalName, event.Payload); err != nil {
		retryPayload := cloneMap(event.Payload)
		retryPayload["retry_of_event_id"] = req.EventID.String()
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, req.ProjectID, signalName, "failed", err, retryPayload)
		return nil, err
	}
	retryEvent, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  req.TenantID,
		ProjectID: req.ProjectID,
		EventType: ProjectEventWorkflowSignaled,
		ActorType: "human_user",
		ActorID:   req.ActorID.String(),
		Summary:   "Workflow signal 已重试",
		Payload: map[string]any{
			"signal_name":       signalName,
			"status":            "sent",
			"retryable":         false,
			"retry_of_event_id": req.EventID.String(),
		},
	})
	if err != nil {
		return nil, err
	}
	return &retryEvent, nil
}

func (s *Service) ensureProjectCoordinator(ctx context.Context, projectRecord Project) error {
	return s.coordinator.EnsureProjectCoordinator(ctx, ProjectCoordinatorSignal{
		TenantID:   projectRecord.TenantID,
		ProjectID:  projectRecord.ID,
		WorkflowID: projectRecord.CoordinationWorkflowID,
	})
}

func (s *Service) retryWorkflowSignal(ctx context.Context, projectRecord Project, signalName string, payload map[string]any) error {
	switch signalName {
	case "DemandSubmitted":
		demandID, err := uuidFromPayload(payload, "demand_id")
		if err != nil {
			return err
		}
		demand, err := s.repository.GetProjectDemand(ctx, projectRecord.TenantID, demandID)
		if err != nil {
			return err
		}
		if demand.CreatedEventID == nil {
			return ErrInvalidProject
		}
		return s.coordinator.SignalDemandSubmitted(ctx, DemandSubmittedSignal{
			TenantID:          projectRecord.TenantID,
			ProjectID:         projectRecord.ID,
			DemandID:          demand.ID,
			SubmittedByUserID: demand.SubmittedByUserID,
			CreatedEventID:    *demand.CreatedEventID,
			WorkflowID:        projectRecord.CoordinationWorkflowID,
		})
	case "ProjectPolicyChanged":
		changedEventID, err := uuidFromPayload(payload, "changed_event_id")
		if err != nil {
			return err
		}
		return s.coordinator.SignalProjectPolicyChanged(ctx, ProjectPolicyChangedSignal{
			TenantID:       projectRecord.TenantID,
			ProjectID:      projectRecord.ID,
			ChangedEventID: changedEventID,
			WorkflowID:     projectRecord.CoordinationWorkflowID,
		})
	case "ProjectMemberChanged":
		changedEventID, err := uuidFromPayload(payload, "changed_event_id")
		if err != nil {
			return err
		}
		return s.coordinator.SignalProjectMemberChanged(ctx, ProjectMemberChangedSignal{
			TenantID:         projectRecord.TenantID,
			ProjectID:        projectRecord.ID,
			ChangedMemberIDs: uuidSliceFromPayload(payload, "changed_member_ids"),
			ChangedEventID:   changedEventID,
			WorkflowID:       projectRecord.CoordinationWorkflowID,
		})
	case "EmployeeTaskCompleted":
		projectTaskID, err := uuidFromPayload(payload, "project_task_id")
		if err != nil {
			return err
		}
		executionSummaryID, err := uuidFromPayload(payload, "execution_summary_id")
		if err != nil {
			return err
		}
		completedEventID, err := uuidFromPayload(payload, "completed_event_id")
		if err != nil {
			return err
		}
		return s.coordinator.SignalEmployeeTaskCompleted(ctx, EmployeeTaskCompletedSignal{
			TenantID:           projectRecord.TenantID,
			ProjectID:          projectRecord.ID,
			ProjectTaskID:      projectTaskID,
			ExecutionSummaryID: executionSummaryID,
			CompletedEventID:   completedEventID,
			WorkflowID:         projectRecord.CoordinationWorkflowID,
		})
	case "EmployeeTaskFailed":
		projectTaskID, err := uuidFromPayload(payload, "project_task_id")
		if err != nil {
			return err
		}
		failedEventID, err := uuidFromPayload(payload, "failed_event_id")
		if err != nil {
			return err
		}
		failureSummary, _ := payload["failure_summary"].(string)
		return s.coordinator.SignalEmployeeTaskFailed(ctx, EmployeeTaskFailedSignal{
			TenantID:       projectRecord.TenantID,
			ProjectID:      projectRecord.ID,
			ProjectTaskID:  projectTaskID,
			FailureSummary: failureSummary,
			FailedEventID:  failedEventID,
			WorkflowID:     projectRecord.CoordinationWorkflowID,
		})
	case "EmployeeTransferRequested":
		projectTaskID, err := uuidFromPayload(payload, "project_task_id")
		if err != nil {
			return err
		}
		transferRequestID, err := uuidFromPayload(payload, "transfer_request_id")
		if err != nil {
			return err
		}
		requestedEventID, err := uuidFromPayload(payload, "requested_event_id")
		if err != nil {
			return err
		}
		return s.coordinator.SignalEmployeeTransferRequested(ctx, EmployeeTransferRequestedSignal{
			TenantID:          projectRecord.TenantID,
			ProjectID:         projectRecord.ID,
			ProjectTaskID:     projectTaskID,
			TransferRequestID: transferRequestID,
			RequestedEventID:  requestedEventID,
			WorkflowID:        projectRecord.CoordinationWorkflowID,
		})
	case "HumanDecisionSubmitted":
		approvalRequestID, err := uuidFromPayload(payload, "approval_request_id")
		if err != nil {
			return err
		}
		decisionRequestID, err := uuidFromPayload(payload, "decision_request_id")
		if err != nil {
			return err
		}
		resolvedEventID, err := uuidFromPayload(payload, "resolved_event_id")
		if err != nil {
			return err
		}
		decision, _ := payload["decision"].(string)
		return s.coordinator.SignalHumanDecisionSubmitted(ctx, HumanDecisionSubmittedSignal{
			TenantID:          projectRecord.TenantID,
			ProjectID:         projectRecord.ID,
			ApprovalRequestID: approvalRequestID,
			DecisionRequestID: decisionRequestID,
			Decision:          decision,
			Payload:           mapFromPayload(payload, "payload"),
			ResolvedEventID:   resolvedEventID,
			WorkflowID:        projectRecord.CoordinationWorkflowID,
		})
	default:
		return ErrInvalidProject
	}
}

func (s *Service) GetLatestProjectConfigRevision(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectConfigRevision, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	repository, ok := s.repository.(latestConfigRevisionRepository)
	if !ok {
		return nil, fmt.Errorf("project repository does not support latest config revision")
	}
	revision, err := repository.GetLatestConfigRevision(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	return &revision, nil
}

func (s *Service) GetProjectOverview(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectOverview, error) {
	return s.GetOverview(ctx, tenantID, projectID)
}

func (s *Service) GetOverview(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectOverview, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	project, err := s.repository.GetProject(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	members, err := s.repository.ListProjectMembers(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	limit, offset := normalizePagination(20, 0)
	tasks, err := s.repository.ListProjectTasks(ctx, tenantID, projectID, nil, limit, offset)
	if err != nil {
		return nil, err
	}
	events, err := s.repository.ListProjectEvents(ctx, tenantID, projectID, limit, offset)
	if err != nil {
		return nil, err
	}

	overview := ProjectOverview{
		Project: project,
		StatusSummary: ProjectStatusSummary{
			CurrentPhase: string(project.Status),
			IsArchived:   project.Status == ProjectStatusArchived || project.ArchivedAt != nil,
		},
		ActiveTasks:  tasks,
		RecentEvents: events,
		CoordinationWorkflow: ProjectCoordinationWorkflow{
			WorkflowID: project.CoordinationWorkflowID,
			Status:     project.CoordinationStatus,
		},
	}
	for _, member := range members {
		switch member.PrincipalType {
		case PrincipalTypeHumanUser:
			overview.HumanRoles = append(overview.HumanRoles, member)
		case PrincipalTypeDigitalEmployee:
			overview.DigitalEmployeePool = append(overview.DigitalEmployeePool, member)
		}
	}
	for _, task := range tasks {
		switch task.Status {
		case "completed":
			overview.TaskSummary.CompletedTasks++
		case "failed":
			overview.TaskSummary.FailedTasks++
		case "waiting_human":
			overview.TaskSummary.PendingHumanTasks++
			overview.TaskSummary.ActiveTasks++
		default:
			overview.TaskSummary.ActiveTasks++
		}
	}
	return &overview, nil
}

func (s *Service) taskAndProjectForWriteback(ctx context.Context, tenantID, runtimeNodeID, projectTaskID, digitalEmployeeID uuid.UUID) (ProjectTask, Project, error) {
	if runtimeNodeID == uuid.Nil {
		return ProjectTask{}, Project{}, ErrProjectTaskForbidden
	}
	task, err := s.repository.GetProjectTask(ctx, tenantID, projectTaskID)
	if err != nil {
		return ProjectTask{}, Project{}, err
	}
	if task.AssignedDigitalEmployeeID == nil || *task.AssignedDigitalEmployeeID != digitalEmployeeID {
		return ProjectTask{}, Project{}, ErrProjectTaskForbidden
	}
	if task.DigitalEmployeeRunID == nil {
		return ProjectTask{}, Project{}, ErrProjectTaskForbidden
	}
	runRepository, ok := s.repository.(ProjectTaskRuntimeBindingRepository)
	if !ok {
		return ProjectTask{}, Project{}, ErrProjectTaskForbidden
	}
	taskRuntimeNodeID, err := runRepository.GetProjectTaskRunRuntimeNodeID(ctx, tenantID, task.ID, *task.DigitalEmployeeRunID)
	if err != nil {
		return ProjectTask{}, Project{}, err
	}
	if taskRuntimeNodeID != runtimeNodeID {
		return ProjectTask{}, Project{}, ErrProjectTaskForbidden
	}
	if !projectTaskAcceptsRuntimeWriteback(task.Status) {
		return ProjectTask{}, Project{}, ErrProjectTaskForbidden
	}
	projectRecord, err := s.repository.GetProject(ctx, tenantID, task.ProjectID)
	if err != nil {
		return ProjectTask{}, Project{}, err
	}
	return task, projectRecord, nil
}

func (s *Service) validateAttemptRuntimeRequest(ctx context.Context, req ProjectTaskAttemptRuntimeRequest) (ProjectTask, ProjectTaskAttempt, error) {
	req.LeaseToken = strings.TrimSpace(req.LeaseToken)
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	if req.TenantID == uuid.Nil || req.AttemptID == uuid.Nil || req.ProjectTaskID == uuid.Nil || req.RuntimeNodeID == uuid.Nil {
		return ProjectTask{}, ProjectTaskAttempt{}, ErrInvalidProject
	}
	if req.LeaseToken == "" || req.IdempotencyKey == "" {
		return ProjectTask{}, ProjectTaskAttempt{}, ErrInvalidProject
	}
	task, err := s.repository.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
	if err != nil {
		return ProjectTask{}, ProjectTaskAttempt{}, err
	}
	if task.CurrentAttemptID == nil || *task.CurrentAttemptID != req.AttemptID {
		return ProjectTask{}, ProjectTaskAttempt{}, ErrProjectConflict
	}
	if !projectTaskAcceptsRuntimeWriteback(task.Status) {
		return ProjectTask{}, ProjectTaskAttempt{}, ErrProjectConflict
	}
	attempt, err := s.repository.GetProjectTaskAttempt(ctx, req.TenantID, req.AttemptID)
	if err != nil {
		return ProjectTask{}, ProjectTaskAttempt{}, err
	}
	if attempt.ProjectTaskID != req.ProjectTaskID || attempt.LeaseToken != req.LeaseToken {
		return ProjectTask{}, ProjectTaskAttempt{}, ErrProjectConflict
	}
	if attempt.RuntimeNodeID != nil && *attempt.RuntimeNodeID != req.RuntimeNodeID {
		return ProjectTask{}, ProjectTaskAttempt{}, ErrProjectConflict
	}
	return task, attempt, nil
}

func digitalEmployeeIDForProjectTask(task ProjectTask) (uuid.UUID, error) {
	if task.AssignedDigitalEmployeeID == nil || *task.AssignedDigitalEmployeeID == uuid.Nil {
		return uuid.Nil, ErrProjectTaskForbidden
	}
	return *task.AssignedDigitalEmployeeID, nil
}

func projectTaskAcceptsRuntimeWriteback(status string) bool {
	switch status {
	case "assigned", "queued", "running":
		return true
	default:
		return false
	}
}

func runtimeWritebackProjectTaskStatuses() []string {
	return []string{"assigned", "queued", "running"}
}

func (s *Service) projectTaskWritebackRepository() (ProjectTaskWritebackRepository, error) {
	repository, ok := s.repository.(ProjectTaskWritebackRepository)
	if !ok {
		return nil, fmt.Errorf("project repository does not support atomic project task writeback")
	}
	return repository, nil
}

func (s *Service) projectTaskAttemptWritebackRepository() (ProjectTaskAttemptWritebackRepository, error) {
	repository, ok := s.repository.(ProjectTaskAttemptWritebackRepository)
	if !ok {
		return nil, fmt.Errorf("project repository does not support atomic project task attempt writeback")
	}
	return repository, nil
}

func (s *Service) projectTaskHumanWaitResolutionRepository() (ProjectTaskHumanWaitResolutionRepository, error) {
	repository, ok := s.repository.(ProjectTaskHumanWaitResolutionRepository)
	if !ok {
		return nil, fmt.Errorf("project repository does not support atomic project task human wait resolution")
	}
	return repository, nil
}

func (s *Service) findDecisionRequest(ctx context.Context, tenantID, projectID, decisionID uuid.UUID) (DecisionRequest, error) {
	return s.repository.GetDecisionRequest(ctx, tenantID, projectID, decisionID)
}

func (s *Service) appendWorkflowSignalEvent(ctx context.Context, tenantID, projectID uuid.UUID, signalName, status string, signalErr error, payload map[string]any) error {
	payload = cloneMap(mapOrEmptyAny(payload))
	payload["signal_name"] = signalName
	payload["status"] = status
	payload["retryable"] = signalErr != nil
	if signalErr != nil {
		payload["error"] = signalErr.Error()
	}
	_, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: ProjectEventWorkflowSignaled,
		ActorType: "control_plane",
		ActorID:   "project_service",
		Summary:   "Workflow signal 状态已记录",
		Payload:   payload,
	})
	if err != nil {
		return err
	}
	if status != "failed" || signalErr == nil {
		return nil
	}
	summary, failurePayload := workflowCoordinationFailurePayload(signalName, signalErr, payload)
	_, err = s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: ProjectEventWorkflowCoordinationFailed,
		ActorType: "control_plane",
		ActorID:   "project_service",
		Summary:   summary,
		Payload:   failurePayload,
	})
	return err
}

func workflowCoordinationFailurePayload(signalName string, err error, extra map[string]any) (summary string, payload map[string]any) {
	reasonCode := "workflow_signal_failed"
	recommendedAction := "inspect_workflow_signal_failure"
	errText := ""
	if err != nil {
		errText = err.Error()
		lowerErr := strings.ToLower(errText)
		switch {
		case strings.Contains(lowerErr, "digital_employee_pool is empty"):
			reasonCode = "no_plannable_digital_employee"
			recommendedAction = "assign_digital_employee"
		case strings.Contains(lowerErr, "runtime_placement_missing"):
			reasonCode = "runtime_placement_missing"
			recommendedAction = "bind_runtime"
		case strings.Contains(lowerErr, "provider_unavailable"):
			reasonCode = "provider_unavailable"
			recommendedAction = "check_provider"
		}
	}
	payload = cloneMap(mapOrEmptyAny(extra))
	payload["signal_name"] = signalName
	payload["status"] = "failed"
	payload["reason_code"] = reasonCode
	payload["recommended_action"] = recommendedAction
	if errText != "" {
		payload["error"] = errText
	}
	return "Workflow coordination failure projected: " + reasonCode, payload
}

func uuidFromPayload(payload map[string]any, key string) (uuid.UUID, error) {
	value, ok := payload[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return uuid.Nil, ErrInvalidProject
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, ErrInvalidProject
	}
	return id, nil
}

func uuidSliceFromPayload(payload map[string]any, key string) []uuid.UUID {
	switch raw := payload[key].(type) {
	case []string:
		ids := make([]uuid.UUID, 0, len(raw))
		for _, value := range raw {
			id, err := uuid.Parse(value)
			if err == nil {
				ids = append(ids, id)
			}
		}
		return ids
	case []any:
		ids := make([]uuid.UUID, 0, len(raw))
		for _, item := range raw {
			value, ok := item.(string)
			if !ok {
				continue
			}
			id, err := uuid.Parse(value)
			if err == nil {
				ids = append(ids, id)
			}
		}
		return ids
	default:
		return nil
	}
}

func mapFromPayload(payload map[string]any, key string) map[string]any {
	value, ok := payload[key].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return value
}

func cloneMap(value map[string]any) map[string]any {
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func removeRuntimeLocalPathMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}
	cleaned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		if runtimeLocalPathMetadataKey(key) {
			continue
		}
		cleaned[key] = removeRuntimeLocalPathMetadataValue(value)
	}
	return cleaned
}

func removeRuntimeLocalPathMetadataValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return removeRuntimeLocalPathMetadata(typed)
	case []any:
		cleaned := make([]any, len(typed))
		for index, item := range typed {
			cleaned[index] = removeRuntimeLocalPathMetadataValue(item)
		}
		return cleaned
	case []map[string]any:
		cleaned := make([]map[string]any, len(typed))
		for index, item := range typed {
			cleaned[index] = removeRuntimeLocalPathMetadata(item)
		}
		return cleaned
	default:
		return value
	}
}

func runtimeLocalPathMetadataKey(key string) bool {
	switch strings.ToLower(key) {
	case "agent_home_dir", "workspace_base_dir", "workspace_path", "mcp_config_path", "employee_capability_dir":
		return true
	default:
		return false
	}
}

func projectTaskExecutionPacketMap(packet ProjectTaskExecutionPacket) map[string]any {
	dependencyOutputs := make([]any, 0, len(packet.DependencyOutputs))
	for _, output := range packet.DependencyOutputs {
		dependencyOutputs = append(dependencyOutputs, map[string]any{
			"project_task_id": output.ProjectTaskID,
			"conclusion":      output.Conclusion,
			"evidence_refs":   append([]any(nil), output.EvidenceRefs...),
			"artifact_refs":   append([]any(nil), output.ArtifactRefs...),
		})
	}
	humanDecisionRefs := make([]any, 0, len(packet.HumanDecisionRefs))
	for _, ref := range packet.HumanDecisionRefs {
		humanDecisionRefs = append(humanDecisionRefs, map[string]any{
			"decision_request_id": ref.DecisionRequestID,
			"decision_type":       ref.DecisionType,
			"status_snapshot":     ref.StatusSnapshot,
		})
	}
	forbiddenScopes := make([]any, 0, len(packet.ForbiddenScopes))
	for _, scope := range packet.ForbiddenScopes {
		forbiddenScopes = append(forbiddenScopes, scope)
	}
	stopForHumanCriteria := make([]any, 0, len(packet.StopForHumanCriteria))
	for _, criterion := range packet.StopForHumanCriteria {
		stopForHumanCriteria = append(stopForHumanCriteria, criterion)
	}
	return map[string]any{
		"version":                 packet.Version,
		"project_id":              packet.ProjectID,
		"project_task_id":         packet.ProjectTaskID,
		"title":                   packet.Title,
		"summary":                 packet.Summary,
		"expected_outputs":        append([]any(nil), packet.ExpectedOutputs...),
		"input_requirements":      cloneMap(mapOrEmptyAny(packet.InputRequirements)),
		"handoff_contract":        cloneMap(mapOrEmptyAny(packet.HandoffContract)),
		"dependency_outputs":      dependencyOutputs,
		"human_decision_refs":     humanDecisionRefs,
		"forbidden_scopes":        forbiddenScopes,
		"risk_level":              packet.RiskLevel,
		"stop_for_human_criteria": stopForHumanCriteria,
	}
}

func projectTaskContextUpdateDeliveryMode(task ProjectTask, updateKind string) string {
	switch strings.TrimSpace(updateKind) {
	case "requirement_changed", "plan_invalid", "scope_changed":
		return ContextUpdateDeliveryCancelAndReplan
	case "comment", "additional_context", "evidence_ref":
		if task.Status == ProjectTaskStatusWaitingHuman {
			return ContextUpdateDeliveryWaitingHuman
		}
		return ContextUpdateDeliveryNextAttempt
	default:
		return ContextUpdateDeliveryNextAttempt
	}
}

func classifyProjectTaskLiveness(item *ProjectTaskLiveness, task ProjectTask, now time.Time) {
	switch task.Status {
	case ProjectTaskStatusCompleted, ProjectTaskStatusFailed, ProjectTaskStatusCancelled:
		item.Liveness = ProjectTaskLivenessTerminal
		item.NextAction = "no-op terminal"
		return
	}
	if task.RetryNotBefore != nil && task.RetryNotBefore.After(now) {
		item.Liveness = ProjectTaskLivenessRetryScheduled
		item.NextAction = "retry wakeup"
		return
	}
	if len(item.BlockingDependencyIDs) > 0 {
		item.Liveness = ProjectTaskLivenessBlockedByDependency
		item.NextAction = "dependency completion"
		return
	}
	switch task.Status {
	case ProjectTaskStatusQueued:
		item.Liveness = ProjectTaskLivenessQueued
		item.NextAction = "runtime start"
	case ProjectTaskStatusRunning:
		if item.LeaseExpiresAt != nil && item.LeaseExpiresAt.Before(now) {
			item.Liveness = ProjectTaskLivenessLeaseLost
			item.NextAction = "recovery policy"
			return
		}
		item.Liveness = ProjectTaskLivenessRunning
		item.NextAction = "lease renew"
	case ProjectTaskStatusWaitingHuman:
		item.Liveness = ProjectTaskLivenessWaitingHuman
		item.NextAction = "human response"
		if task.WaitingReason != nil {
			item.Reason = *task.WaitingReason
		}
	default:
		item.Liveness = ProjectTaskLivenessReadyToDispatch
		item.NextAction = "dispatch"
	}
}

func validHumanDecision(decision string) bool {
	switch decision {
	case "approved", "rejected", "needs_more_evidence", PlanReviewDecisionRequestChanges, PlanningGapDecisionRestaffed, PlanningGapDecisionExempted:
		return true
	default:
		return false
	}
}

func validAcceptanceStatus(status string) bool {
	switch status {
	case "accepted", "rejected", "needs_more_evidence", "partially_accepted":
		return true
	default:
		return false
	}
}

func validEvidenceVerificationStatus(status EvidenceVerificationStatus) bool {
	switch status {
	case EvidenceVerificationStatusSubmitted, EvidenceVerificationStatusLinked, EvidenceVerificationStatusVerified, EvidenceVerificationStatusRejected, EvidenceVerificationStatusSuperseded:
		return true
	default:
		return false
	}
}

func projectArchived(project Project) bool {
	return project.Status == ProjectStatusArchived || project.ArchivedAt != nil
}

func collectArchivePreviewPages[T any](ctx context.Context, pageSize int32, list func(limit, offset int32) ([]T, error)) ([]T, error) {
	pageSize, offset := normalizePagination(pageSize, 0)
	values := make([]T, 0)
	for page := 0; page < 10000; page++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rows, err := list(pageSize, offset)
		if err != nil {
			return nil, err
		}
		values = append(values, rows...)
		if int32(len(rows)) < pageSize {
			return values, nil
		}
		nextOffset := offset + int32(len(rows))
		if nextOffset <= offset {
			return nil, ErrInvalidProject
		}
		offset = nextOffset
	}
	return nil, ErrInvalidProject
}

func (s *Service) collectArchiveArtifactIDs(ctx context.Context, tenantID, projectID uuid.UUID) ([]uuid.UUID, error) {
	pageSize, _ := normalizePagination(100, 0)
	artifactRefs, err := collectArchivePreviewPages(ctx, pageSize, func(limit, offset int32) ([]ProjectArtifactRef, error) {
		return s.repository.ListArtifactRefs(ctx, tenantID, projectID, limit, offset)
	})
	if err != nil {
		return nil, err
	}
	seen := make(map[uuid.UUID]struct{}, len(artifactRefs))
	artifactIDs := make([]uuid.UUID, 0, len(artifactRefs))
	for _, artifactRef := range artifactRefs {
		if artifactRef.ArtifactID == nil || *artifactRef.ArtifactID == uuid.Nil {
			continue
		}
		if _, ok := seen[*artifactRef.ArtifactID]; ok {
			continue
		}
		seen[*artifactRef.ArtifactID] = struct{}{}
		artifactIDs = append(artifactIDs, *artifactRef.ArtifactID)
	}
	return artifactIDs, nil
}

func archiveSnapshotIncludedCounts(preview *ProjectArchivePreview) map[string]any {
	if preview == nil {
		return map[string]any{}
	}
	return map[string]any{
		"evidence_ref_count": preview.EvidenceCount,
		"artifact_ref_count": preview.ArtifactCount,
		"report_ref_count":   preview.ReportCount,
	}
}

func (s *Service) validateAcceptanceRefs(ctx context.Context, tenantID, projectID uuid.UUID, evidenceRefIDs, reportRefIDs []uuid.UUID) error {
	for _, id := range evidenceRefIDs {
		if id == uuid.Nil {
			return ErrInvalidProjectAcceptance
		}
	}
	for _, id := range reportRefIDs {
		if id == uuid.Nil {
			return ErrInvalidProjectAcceptance
		}
	}
	pageSize, _ := normalizePagination(100, 0)
	evidenceRefs, err := collectArchivePreviewPages(ctx, pageSize, func(limit, offset int32) ([]ProjectEvidenceRef, error) {
		return s.repository.ListEvidenceRefs(ctx, tenantID, projectID, nil, limit, offset)
	})
	if err != nil {
		return err
	}
	reportRefs, err := collectArchivePreviewPages(ctx, pageSize, func(limit, offset int32) ([]ProjectReportRef, error) {
		return s.repository.ListReportRefs(ctx, tenantID, projectID, limit, offset)
	})
	if err != nil {
		return err
	}
	evidenceIDs := make(map[uuid.UUID]struct{}, len(evidenceRefs))
	for _, evidence := range evidenceRefs {
		evidenceIDs[evidence.ID] = struct{}{}
	}
	for _, id := range evidenceRefIDs {
		if _, ok := evidenceIDs[id]; !ok {
			return ErrInvalidProjectAcceptance
		}
	}
	reportIDs := make(map[uuid.UUID]struct{}, len(reportRefs))
	for _, report := range reportRefs {
		reportIDs[report.ID] = struct{}{}
	}
	for _, id := range reportRefIDs {
		if _, ok := reportIDs[id]; !ok {
			return ErrInvalidProjectAcceptance
		}
	}
	return nil
}

func validateMembers(members []ProjectMemberInput) error {
	for _, member := range members {
		if member.PrincipalID == uuid.Nil {
			return ErrInvalidProjectMember
		}
		if member.ProjectRole == ProjectRole("coordinator") {
			return ErrInvalidProjectMember
		}
		if member.ProjectRole == ProjectRoleExecutor && member.PrincipalType != PrincipalTypeDigitalEmployee {
			return ErrInvalidProjectMember
		}
		if (member.ProjectRole == ProjectRoleOwner || member.ProjectRole == ProjectRoleLeader || member.ProjectRole == ProjectRoleAcceptance) && member.PrincipalType != PrincipalTypeHumanUser {
			return ErrInvalidProjectMember
		}
	}
	return nil
}

func ensureOwnerMember(req CreateProjectRequest) []ProjectMemberInput {
	members := append([]ProjectMemberInput{}, req.Members...)
	for _, member := range members {
		if member.PrincipalType == PrincipalTypeHumanUser && member.PrincipalID == req.HumanOwnerUserID && member.ProjectRole == ProjectRoleOwner {
			return members
		}
	}
	return append(members, ProjectMemberInput{
		PrincipalType: PrincipalTypeHumanUser,
		PrincipalID:   req.HumanOwnerUserID,
		ProjectRole:   ProjectRoleOwner,
	})
}

func normalizePagination(limit, offset int32) (int32, int32) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func normalizeWorkflowInstancePagination(limit, offset int32) (int32, int32) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func normalizeWorkflowInstanceStatus(item WorkflowInstanceSummary) WorkflowInstanceStatus {
	switch item.Status {
	case WorkflowInstanceStatusFailed, WorkflowInstanceStatusCancelled:
		return item.Status
	}
	if item.Progress.WaitingHumanNodes > 0 {
		return WorkflowInstanceStatusWaitingHuman
	}
	if item.Progress.RunningNodes > 0 {
		return WorkflowInstanceStatusRunning
	}
	if item.Progress.TotalNodes == 0 {
		return WorkflowInstanceStatusPlanning
	}
	if item.Progress.CompletedNodes == item.Progress.TotalNodes {
		return WorkflowInstanceStatusCompleted
	}
	if item.Status != "" {
		return item.Status
	}
	return WorkflowInstanceStatusUnknown
}

func workflowInstanceAttentionRank(status WorkflowInstanceStatus) int {
	switch status {
	case WorkflowInstanceStatusWaitingHuman:
		return 0
	case WorkflowInstanceStatusFailed:
		return 1
	case WorkflowInstanceStatusRunning:
		return 2
	case WorkflowInstanceStatusPlanning:
		return 3
	case WorkflowInstanceStatusUnknown:
		return 4
	case WorkflowInstanceStatusCompleted:
		return 5
	case WorkflowInstanceStatusCancelled:
		return 6
	default:
		return 7
	}
}

func strPtr(value string) *string {
	return &value
}

func mapOrEmptyAny(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func sliceOrEmptyAny(value []any) []any {
	if value == nil {
		return []any{}
	}
	return value
}
