package projectcoordination

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/approval"
	"github.com/superteam/control-plane/internal/project"
)

type ProjectStore struct {
	repository        project.Repository
	approvals         ApprovalCreator
	inbox             project.DecisionInboxProjector
	runStarter        ProjectTaskRunStarter
	readiness         DigitalEmployeeReadinessChecker
	lending           LendingGatekeeper
	scenarioTemplates ScenarioTemplateSource
	profileSource     DigitalEmployeePlanningProfileSource
	clock             clockFunc
	employeeReader    GateEmployeeRuntimeReader
	capabilityReader  GateCapabilityReader
	nodeResolver      GateProjectTaskNodeResolver
}

type clockFunc func() time.Time

const defaultRevisionMaxAttempts int32 = 3

// defaultMaxPlanIterations bounds graph extension rounds (upstream supplement
// tasks appended to resolve a blocked task's missing inputs) when
// projects.coordination_policy.max_plan_iterations is absent or invalid.
const defaultMaxPlanIterations = 3

func (s *ProjectStore) WithClock(clock clockFunc) *ProjectStore {
	s.clock = clock
	return s
}

func (s *ProjectStore) now() time.Time {
	if s.clock != nil {
		return s.clock()
	}
	return time.Now().UTC()
}

type GateEmployeeRuntimeReader interface {
	GetEmployeeRuntimeSnapshot(ctx context.Context, tenantID, projectID, employeeID uuid.UUID) (project.PreDispatchEmployeeSnapshot, project.PreDispatchRuntimeSnapshot, error)
}

type GateCapabilityReader interface {
	GetEmployeeCapabilitySnapshot(ctx context.Context, tenantID, employeeID uuid.UUID, task project.ProjectTask) (project.PreDispatchCapabilitySnapshot, error)
}

func (s *ProjectStore) WithPreDispatchGateReaders(employeeReader GateEmployeeRuntimeReader, capabilityReader GateCapabilityReader) *ProjectStore {
	s.employeeReader = employeeReader
	s.capabilityReader = capabilityReader
	return s
}

// GateProjectTaskNodeResolver resolves the runtime node the pre-dispatch gate
// should evaluate for a task, using the same three-layer algorithm (task pin >
// employee affinity > lowest load) that dispatch itself uses — implemented as a
// DryRun call so gate evaluation (which may run repeatedly, e.g. on retry
// polling) never mutates employee affinity. This is how the gate distinguishes
// "task hard-pinned to an offline node" from "no eligible node is online at
// all", which the project's runtime eligibility set alone cannot answer.
type GateProjectTaskNodeResolver interface {
	ResolveProjectTaskNodeForGate(ctx context.Context, tenantID, projectID, employeeID, projectTaskID uuid.UUID) (project.NodeResolution, error)
}

// WithProjectTaskNodeResolver attaches the node resolver used to compute the
// gate's runtime Pinned/NodeOnline facts. Optional: when unset, PlacementPresent
// still reflects the eligibility set, but Pinned/NodeOnline fall back to
// whatever GateEmployeeRuntimeReader reports (which cannot distinguish a
// pinned-but-offline node from "no node was ever pinned").
func (s *ProjectStore) WithProjectTaskNodeResolver(resolver GateProjectTaskNodeResolver) *ProjectStore {
	s.nodeResolver = resolver
	return s
}

// WithDigitalEmployeeReadiness attaches the legacy employee-scoped readiness checker.
// ProjectTask planning prefers project-scoped Runtime placement when a gate reader is
// available, so this checker is only a compatibility fallback.
func (s *ProjectStore) WithDigitalEmployeeReadiness(checker DigitalEmployeeReadinessChecker) *ProjectStore {
	s.readiness = checker
	return s
}

// WithLendingGatekeeper attaches a team-lending gate used to exclude borrowed digital
// employees from a foreign team that the project has no effective lending grant for.
func (s *ProjectStore) WithLendingGatekeeper(gatekeeper LendingGatekeeper) *ProjectStore {
	s.lending = gatekeeper
	return s
}

// ScenarioTemplateSource resolves a project's bound scenario template for
// injection into the planning snapshot.
type ScenarioTemplateSource interface {
	GetScenarioTemplateSnapshot(ctx context.Context, tenantID uuid.UUID, key string) (ScenarioTemplateSnapshot, error)
}

func (s *ProjectStore) WithScenarioTemplateSource(source ScenarioTemplateSource) *ProjectStore {
	s.scenarioTemplates = source
	return s
}

func (s *ProjectStore) WithDigitalEmployeePlanningProfiles(source DigitalEmployeePlanningProfileSource) *ProjectStore {
	s.profileSource = source
	return s
}

// runtimeReadyEmployeeIDs returns the set of project-runtime-ready digital-employee
// principal IDs among the given members. A nil result means "do not filter" so
// behavior stays backward-compatible when no readiness source is attached or lookup fails.
func (s *ProjectStore) runtimeReadyEmployeeIDs(ctx context.Context, tenantID, projectID uuid.UUID, members []project.ProjectMember) map[uuid.UUID]bool {
	ids := make([]uuid.UUID, 0, len(members))
	for _, member := range members {
		if member.PrincipalType == project.PrincipalTypeDigitalEmployee && member.PrincipalID != uuid.Nil {
			ids = append(ids, member.PrincipalID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	if s.employeeReader != nil && projectID != uuid.Nil {
		ready := make(map[uuid.UUID]bool, len(ids))
		for _, id := range ids {
			employee, runtimeSnapshot, err := s.employeeReader.GetEmployeeRuntimeSnapshot(ctx, tenantID, projectID, id)
			if err != nil {
				return nil
			}
			ready[id] = employeeRuntimeSnapshotReady(employee, runtimeSnapshot)
		}
		return ready
	}
	if s.readiness == nil {
		return nil
	}
	ready, err := s.readiness.AreRuntimeReady(ctx, tenantID, ids)
	if err != nil {
		// Fail open: a readiness lookup error must not block planning.
		return nil
	}
	return ready
}

func employeeRuntimeSnapshotReady(employee project.PreDispatchEmployeeSnapshot, runtimeSnapshot project.PreDispatchRuntimeSnapshot) bool {
	return employee.PolicyAllowed &&
		runtimeSnapshot.NodeOnline &&
		runtimeSnapshot.ProviderAvailable &&
		runtimeSnapshot.WorkspaceReady &&
		runtimeSnapshot.SlotAvailable &&
		runtimeSnapshot.ContractVersionAccepted
}

func (s *ProjectStore) planningProfileRecords(ctx context.Context, tenantID, projectID uuid.UUID, employeeIDs []uuid.UUID) map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord {
	if s.profileSource == nil || len(employeeIDs) == 0 {
		return nil
	}
	records, err := s.profileSource.PlanningProfileRecords(ctx, tenantID, projectID, employeeIDs)
	if err != nil {
		// Fail open: profile facts enrich planning, but a source outage must not block it.
		return nil
	}
	return records
}

// lendingEligibleEmployeeIDs applies the team-lending gate to candidate digital-employee
// IDs. It returns the set of eligible IDs and a map of skipped employee -> the foreign
// team they were borrowed from without a grant. A nil eligible set means "do not filter"
// (no gatekeeper, no candidates, or a lookup error) so behavior stays backward-compatible.
// ownTeamID is the project's own team (may be nil). Like the readiness check, lending
// lookups fail open: a gate error must not strand planning, since the authoritative
// lending enforcement remains the approval workflow.
func (s *ProjectStore) lendingEligibleEmployeeIDs(ctx context.Context, tenantID, projectID uuid.UUID, ownTeamID *uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]bool, map[uuid.UUID]uuid.UUID) {
	if s.lending == nil || ownTeamID == nil || len(employeeIDs) == 0 {
		return nil, nil
	}
	employeeTeams, err := s.lending.ResolveEmployeeTeams(ctx, tenantID, employeeIDs)
	if err != nil {
		return nil, nil
	}
	grantedTeams, err := s.lending.EffectiveLendingTeams(ctx, tenantID, projectID)
	if err != nil {
		return nil, nil
	}
	eligible := make(map[uuid.UUID]bool, len(employeeIDs))
	skipped := make(map[uuid.UUID]uuid.UUID)
	for _, id := range employeeIDs {
		team, hasTeam := employeeTeams[id]
		switch {
		case !hasTeam || team == uuid.Nil:
			// No owning team → not a borrowed resource → always eligible.
			eligible[id] = true
		case ownTeamID != nil && team == *ownTeamID:
			// Project's own team → eligible without a lending grant.
			eligible[id] = true
		case grantedTeams[team]:
			// Foreign team with an effective lending grant → eligible.
			eligible[id] = true
		default:
			// Foreign team without a grant → gated out of the executor pool.
			skipped[id] = team
		}
	}
	return eligible, skipped
}

// recordLendingSkips emits a best-effort coordination event for each digital employee
// excluded from the pool for lacking an effective team-lending grant. Failures are ignored
// so observability writes never block planning.
func (s *ProjectStore) recordLendingSkips(ctx context.Context, tenantID, projectID, demandID uuid.UUID, skipped map[uuid.UUID]uuid.UUID) {
	if s.repository == nil || len(skipped) == 0 {
		return
	}
	for employeeID, teamID := range skipped {
		_, _ = s.repository.AppendProjectEvent(ctx, coordinatorEvent(
			tenantID,
			projectID,
			project.ProjectEventLendingEmployeeSkipped,
			"project_coordinator",
			"数字员工因缺少有效团队借调授权被排除出可执行池",
			map[string]any{
				"digital_employee_id": employeeID.String(),
				"team_id":             teamID.String(),
				"demand_id":           demandID.String(),
			},
		))
	}
}

func NewProjectStore(repository project.Repository) *ProjectStore {
	return NewProjectStoreWithApprovals(repository, nil)
}

type ApprovalCreator interface {
	CreateRequest(ctx context.Context, input approval.CreateRequestInput) (*approval.ApprovalRequest, error)
	GetRequestByResource(ctx context.Context, tenantID uuid.UUID, resourceType string, resourceID uuid.UUID) (*approval.ApprovalRequest, error)
}

type ProjectTaskRunStarter interface {
	StartProjectTaskRun(ctx context.Context, req StartProjectTaskRunRequest) (StartProjectTaskRunResult, error)
}

// DigitalEmployeeReadinessChecker reports which digital employees are runtime-ready
// (bound to a healthy online runtime with an approved effective config). The coordinator
// uses it to filter its executor pool so the reasoning planner only proposes employees
// that can actually run, instead of stranding tasks on unbound ones.
type DigitalEmployeeReadinessChecker interface {
	AreRuntimeReady(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]bool, error)
}

type DigitalEmployeePlanningProfileSource interface {
	PlanningProfileRecords(ctx context.Context, tenantID, projectID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord, error)
}

// LendingGatekeeper enforces team-lending grants when the coordinator builds its executor
// pool. A digital employee whose owning team differs from the project's own team is only
// eligible if the project holds an effective (approved/auto_approved) lending grant for
// that team; employees with no team, or in the project's own team, are never gated.
type LendingGatekeeper interface {
	// ResolveEmployeeTeams maps the given digital-employee IDs to their owning team.
	// Employees with no owning team are omitted from the result.
	ResolveEmployeeTeams(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]uuid.UUID, error)
	// EffectiveLendingTeams returns the set of teams the project may currently borrow from.
	EffectiveLendingTeams(ctx context.Context, tenantID, projectID uuid.UUID) (map[uuid.UUID]bool, error)
}

type recoveryDependencyRepository interface {
	CreateProjectTaskDependency(ctx context.Context, req project.CreateProjectTaskDependencyRequest) (project.ProjectTaskDependency, error)
	RewireProjectTaskDependencies(ctx context.Context, req project.RewireProjectTaskDependenciesRequest) ([]project.ProjectTaskDependency, error)
}

func NewProjectStoreWithApprovals(repository project.Repository, approvals ApprovalCreator) *ProjectStore {
	return NewProjectStoreWithApprovalsAndInbox(repository, approvals, nil)
}

func NewProjectStoreWithApprovalsAndInbox(repository project.Repository, approvals ApprovalCreator, inbox project.DecisionInboxProjector) *ProjectStore {
	return NewProjectStoreWithApprovalsInboxAndRunStarter(repository, approvals, inbox, nil)
}

func NewProjectStoreWithApprovalsInboxAndRunStarter(repository project.Repository, approvals ApprovalCreator, inbox project.DecisionInboxProjector, runStarter ProjectTaskRunStarter) *ProjectStore {
	return &ProjectStore{repository: repository, approvals: approvals, inbox: inbox, runStarter: runStarter}
}

func (s *ProjectStore) LoadProjectCoordinationSnapshot(ctx context.Context, input LoadSnapshotInput) (CoordinationSnapshot, error) {
	if s.repository == nil {
		return CoordinationSnapshot{}, ErrActivityStoreRequired
	}
	projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return CoordinationSnapshot{}, err
	}
	demand, err := s.repository.GetProjectDemand(ctx, input.TenantID, input.DemandID)
	if err != nil {
		return CoordinationSnapshot{}, err
	}
	members, err := s.repository.ListProjectMembers(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return CoordinationSnapshot{}, err
	}
	readyEmployees := s.runtimeReadyEmployeeIDs(ctx, input.TenantID, input.ProjectID, members)
	candidates := make([]project.ProjectMember, 0, len(members))
	candidateIDs := make([]uuid.UUID, 0, len(members))
	for _, member := range members {
		if member.PrincipalType != project.PrincipalTypeDigitalEmployee || member.Status != "active" || !isRoutableDigitalProjectRole(member.ProjectRole) {
			continue
		}
		candidates = append(candidates, member)
		candidateIDs = append(candidateIDs, member.PrincipalID)
	}
	// Team-lending gate: a borrowed employee from a foreign team is only an eligible executor
	// if the project holds an effective lending grant for that team. Ungranted ones are
	// silently excluded from the pool and recorded as skipped (best-effort audit event).
	lendingEligible, lendingSkipped := s.lendingEligibleEmployeeIDs(ctx, input.TenantID, input.ProjectID, projectRecord.TeamID, candidateIDs)
	s.recordLendingSkips(ctx, input.TenantID, input.ProjectID, input.DemandID, lendingSkipped)
	eligibleCandidates := make([]project.ProjectMember, 0, len(candidates))
	eligibleCandidateIDs := make([]uuid.UUID, 0, len(candidates))
	for _, member := range candidates {
		if lendingEligible != nil && !lendingEligible[member.PrincipalID] {
			continue
		}
		eligibleCandidates = append(eligibleCandidates, member)
		eligibleCandidateIDs = append(eligibleCandidateIDs, member.PrincipalID)
	}
	if len(eligibleCandidates) == 0 {
		_, _ = s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventCoordinationBlocked, input.DemandID.String(), "项目没有可参与规划的数字员工", map[string]any{
			"reason_code": "no_plannable_digital_employee",
			"demand_id":   input.DemandID.String(),
		}))
	}
	profileRecords := s.planningProfileRecords(ctx, input.TenantID, input.ProjectID, eligibleCandidateIDs)
	pool := make([]ProjectMemberSnapshot, 0, len(eligibleCandidates))
	for _, member := range eligibleCandidates {
		displayName := ""
		if member.DisplayNameSnapshot != nil {
			displayName = *member.DisplayNameSnapshot
		}
		sourceRecord := DigitalEmployeePlanningProfileSourceRecord{}
		if profileRecords != nil {
			sourceRecord = profileRecords[member.PrincipalID]
		}
		runtimeReady := readyEmployees == nil || readyEmployees[member.PrincipalID]
		profile := BuildDigitalEmployeePlanningProfile(member, sourceRecord, runtimeReady)
		pool = append(pool, ProjectMemberSnapshot{
			PrincipalID:     member.PrincipalID,
			ProjectRole:     string(member.ProjectRole),
			Status:          member.Status,
			DisplayName:     displayName,
			PlanningProfile: &profile,
		})
	}
	content := ""
	if demand.Content != nil {
		content = *demand.Content
	}
	var scenarioTemplate *ScenarioTemplateSnapshot
	if s.scenarioTemplates != nil && projectRecord.ScenarioTemplateKey != nil {
		if key := strings.TrimSpace(*projectRecord.ScenarioTemplateKey); key != "" {
			template, templateErr := s.scenarioTemplates.GetScenarioTemplateSnapshot(ctx, input.TenantID, key)
			if templateErr != nil {
				// A stale binding degrades to the generic fallback (nil) rather
				// than blocking planning; the defect stays visible in the log.
				log.Printf("scenario template %q unresolved for project %s: %v", key, input.ProjectID, templateErr)
			} else {
				scenarioTemplate = &template
			}
		}
	}
	return CoordinationSnapshot{
		ProjectID:           projectRecord.ID,
		Demand:              DemandSnapshot{ID: demand.ID, Title: demand.Title, Content: content},
		DigitalEmployeePool: pool,
		CoordinationPolicy:  projectRecord.CoordinationPolicy,
		ScenarioTemplate:    scenarioTemplate,
	}, nil
}

func (s *ProjectStore) CreateCoordinationJob(ctx context.Context, input CreateCoordinationJobInput) (CoordinationJobResult, error) {
	if s.repository == nil {
		return CoordinationJobResult{}, ErrActivityStoreRequired
	}
	triggerEventID := input.TriggerEventID
	job, err := s.repository.CreateCoordinationJob(ctx, project.CreateCoordinationJobRequest{
		TenantID:       input.TenantID,
		ProjectID:      input.ProjectID,
		WorkflowID:     input.WorkflowID,
		TriggerEventID: &triggerEventID,
		JobType:        input.JobType,
		Status:         "running",
		InputSnapshotRef: map[string]any{
			"trigger_event_id": input.TriggerEventID.String(),
			"job_type":         input.JobType,
		},
	})
	if err != nil {
		return CoordinationJobResult{}, err
	}
	if _, err := s.ensureCoordinatorProjectEvent(ctx, input.TenantID, input.ProjectID, project.ProjectEventCoordinationJobCreated, job.ID.String(), "协调作业已创建", map[string]any{
		"coordination_job_id": job.ID.String(),
		"workflow_id":         input.WorkflowID,
		"trigger_event_id":    input.TriggerEventID.String(),
		"job_type":            input.JobType,
	}); err != nil {
		return CoordinationJobResult{}, err
	}
	return CoordinationJobResult{ID: job.ID}, nil
}

func (s *ProjectStore) PersistRouteDecision(ctx context.Context, input PersistRouteDecisionInput) (RouteDecisionResult, error) {
	if s.repository == nil {
		return RouteDecisionResult{}, ErrActivityStoreRequired
	}
	existing, err := s.repository.GetRouteDecisionByCoordinationJob(ctx, input.TenantID, input.JobID)
	if err == nil {
		event, eventErr := s.ensureRouteDecisionCreatedEvent(ctx, input)
		if eventErr != nil {
			return RouteDecisionResult{}, eventErr
		}
		return RouteDecisionResult{ID: existing.ID, CreatedEventID: event.ID}, nil
	}
	if !errors.Is(err, project.ErrProjectNotFound) {
		return RouteDecisionResult{}, err
	}
	event, err := s.ensureRouteDecisionCreatedEvent(ctx, input)
	if err != nil {
		return RouteDecisionResult{}, err
	}
	demandID := input.DemandID
	aggregated := aggregateRouteDecisionFields(input.Decision)
	decision, err := s.repository.CreateRouteDecision(ctx, project.CreateRouteDecisionRequest{
		TenantID:                    input.TenantID,
		ProjectID:                   input.ProjectID,
		CoordinationJobID:           input.JobID,
		DemandID:                    &demandID,
		CandidateDigitalEmployeeIDs: aggregated.CandidateDigitalEmployeeIDs,
		SelectedDigitalEmployeeIDs:  aggregated.SelectedDigitalEmployeeIDs,
		Reason:                      input.Decision.Reason,
		InputRequirements:           aggregated.InputRequirements,
		ExpectedOutputs:             stringsToAny(aggregated.ExpectedOutputs),
		BudgetEstimate:              input.Decision.BudgetEstimate,
		RequiresHumanReview:         input.Decision.RequiresHumanReview,
		CreatedEventID:              &event.ID,
	})
	if err != nil {
		existing, existingErr := s.repository.GetRouteDecisionByCoordinationJob(ctx, input.TenantID, input.JobID)
		if existingErr == nil {
			return RouteDecisionResult{ID: existing.ID, CreatedEventID: event.ID}, nil
		}
		return RouteDecisionResult{}, err
	}
	return RouteDecisionResult{ID: decision.ID, CreatedEventID: event.ID}, nil
}

func (s *ProjectStore) ensureRouteDecisionCreatedEvent(ctx context.Context, input PersistRouteDecisionInput) (project.ProjectEvent, error) {
	return s.ensureCoordinatorProjectEvent(ctx, input.TenantID, input.ProjectID, project.ProjectEventRouteDecisionCreated, input.JobID.String(), "路由决策已生成", map[string]any{
		"coordination_job_id": input.JobID.String(),
		"demand_id":           input.DemandID.String(),
	})
}

func (s *ProjectStore) PersistPlanRevision(ctx context.Context, input PersistPlanRevisionInput) (PlanRevisionResult, error) {
	if s.repository == nil {
		return PlanRevisionResult{}, ErrActivityStoreRequired
	}
	payload := BuildPlanRevisionPayload(input.Decision)
	validation := ValidatePlanRevisionPayload(payload)
	status := project.PlanRevisionStatusAccepted
	if !validation.Acceptable {
		status = project.PlanRevisionStatusValidationFailed
	} else if validation.ReviewRequired || input.Decision.RequiresHumanReview {
		status = project.PlanRevisionStatusPendingReview
	}
	event, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventWorkflowSignaled, input.CoordinationJobID.String(), "计划版本已生成", map[string]any{
		"demand_id":        input.DemandID.String(),
		"plan_fingerprint": validation.PlanFingerprint,
		"status":           status,
	}))
	if err != nil {
		return PlanRevisionResult{}, err
	}
	projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return PlanRevisionResult{}, err
	}
	payloadMap, err := planRevisionPayloadMap(payload)
	if err != nil {
		return PlanRevisionResult{}, err
	}
	reviewReasons := append([]string{}, validation.ReviewReasons...)
	if input.Decision.RequiresHumanReview {
		reviewReasons = appendUniqueString(reviewReasons, "plan_requires_human_review")
	}
	// Freeze the demand's coordination_mode onto the plan revision at persist time (spec
	// §8.1). If the demand cannot be read (legacy/missing), leave it nil rather than failing
	// the persist; interpreting a nil mode as "loop" happens downstream, not here.
	var coordinationMode *string
	if demand, demandErr := s.repository.GetProjectDemand(ctx, input.TenantID, input.DemandID); demandErr == nil {
		mode := demand.CoordinationMode
		coordinationMode = &mode
	}
	revision, err := s.repository.CreatePlanRevision(ctx, project.CreatePlanRevisionRequest{
		TenantID:               input.TenantID,
		TeamID:                 projectRecord.TeamID,
		ProjectID:              input.ProjectID,
		DemandID:               input.DemandID,
		CoordinationJobID:      &input.CoordinationJobID,
		RouteDecisionID:        &input.RouteDecisionID,
		Status:                 status,
		Payload:                payloadMap,
		PlanFingerprint:        validation.PlanFingerprint,
		ValidationErrors:       validation.Errors,
		ValidationWarnings:     validation.Warnings,
		ReviewRequired:         validation.ReviewRequired || input.Decision.RequiresHumanReview,
		ReviewReason:           stringPtr(strings.Join(reviewReasons, "; ")),
		SupersedeOpenRevisions: input.SupersedeOpen,
		SupersedeReason:        input.SupersedeReason,
		CreatedEventID:         &event.ID,
		CoordinationMode:       coordinationMode,
	})
	if err != nil {
		return PlanRevisionResult{}, err
	}
	return PlanRevisionResult{
		ID:              revision.ID,
		Status:          revision.Status,
		RevisionNumber:  revision.RevisionNumber,
		PlanFingerprint: revision.PlanFingerprint,
		Payload:         payload,
		ReviewRequired:  revision.ReviewRequired,
		CreatedEventID:  event.ID,
	}, nil
}

func (s *ProjectStore) DecomposeAcceptedPlanRevision(ctx context.Context, input DecomposeAcceptedPlanRevisionInput) ([]ProjectTaskResult, error) {
	if s.repository == nil {
		return nil, ErrActivityStoreRequired
	}
	graphTasks := make([]project.ProjectTaskGraphCreateTask, 0, len(input.Payload.Tasks))
	for _, plannedTask := range input.Payload.Tasks {
		employeeID, err := uuid.Parse(strings.TrimSpace(plannedTask.SelectedEmployeeID))
		if err != nil {
			return nil, project.ErrInvalidProject
		}
		status := project.ProjectTaskStatusPlanned
		if len(plannedTask.DependsOn) > 0 {
			status = "blocked"
		}
		inputRequirements, plannerNotes := plannedTaskInputRequirements(PlannedTask{
			InputRequirements: plannedTask.InputRequirements,
			Produces:          plannedTask.Produces,
		})
		if len(plannedTask.InputContextRefs) > 0 {
			inputRequirements["input_context_refs"] = append([]string(nil), plannedTask.InputContextRefs...)
		}
		handoffContract := cloneAnyMap(plannedTask.HandoffContract)
		if len(plannedTask.AcceptanceCriteria) > 0 {
			handoffContract["acceptance_criteria"] = append([]string(nil), plannedTask.AcceptanceCriteria...)
		}
		if len(plannedTask.VerificationRequirements) > 0 {
			handoffContract["verification_requirements"] = append([]string(nil), plannedTask.VerificationRequirements...)
		}
		metadata := map[string]any{
			"accepted_plan_revision_id": input.PlanRevisionID.String(),
			"plan_fingerprint":          input.PlanFingerprint,
			"employee_selection": map[string]any{
				"reason":                         plannedTask.EmployeeSelectionReason,
				"required_capabilities":          plannedTask.RequiredCapabilities,
				"matched_capabilities":           plannedTask.MatchedCapabilities,
				"missing_capabilities":           plannedTask.MissingCapabilities,
				"selection_score":                plannedTask.SelectionScore,
				"planning_profile_snapshot_hash": plannedTask.PlanningProfileSnapshotHash,
			},
			"acceptance_criteria": plannedTask.AcceptanceCriteria,
			"produces":            stringsToAny(plannedTask.Produces),
			"planner_notes":       plannerNotes,
		}
		graphTasks = append(graphTasks, project.ProjectTaskGraphCreateTask{
			Key:                       plannedTask.PlannedTaskKey,
			Title:                     plannedTask.Title,
			Summary:                   plannedTask.Objective,
			Status:                    status,
			AssignedDigitalEmployeeID: employeeID,
			TaskKind:                  plannedTask.TaskType,
			RiskLevel:                 plannedTask.RiskLevel,
			RequiresHumanApproval:     plannedTask.HumanReviewRequired,
			ExpectedOutputs:           stringsToAny(plannedTask.ExpectedOutputs),
			InputRequirements:         inputRequirements,
			HandoffContract:           handoffContract,
			PlannerMetadata:           metadata,
			BlockedByKeys:             plannedTask.DependsOn,
		})
	}
	decomposition, err := s.repository.DecomposeAcceptedPlanRevision(ctx, project.DecomposeAcceptedPlanRevisionRequest{
		TenantID:               input.TenantID,
		ProjectID:              input.ProjectID,
		DemandID:               input.DemandID,
		CoordinationJobID:      input.CoordinationJobID,
		RouteDecisionID:        input.RouteDecisionID,
		AcceptedPlanRevisionID: input.PlanRevisionID,
		PlanFingerprint:        input.PlanFingerprint,
		DecompositionClaimKey:  input.PlanRevisionID.String(),
		Tasks:                  graphTasks,
	})
	if err != nil {
		return nil, err
	}
	results := make([]ProjectTaskResult, 0, len(decomposition.Tasks))
	for _, task := range decomposition.Tasks {
		results = append(results, ProjectTaskResult{ID: task.ID})
	}
	return results, nil
}

// plannedTaskInputRequirements keeps only schema'd dependency declarations in
// input_requirements and moves planner prose into metadata.
func plannedTaskInputRequirements(task PlannedTask) (stored map[string]any, notes map[string]any) {
	stored = map[string]any{}
	notes = map[string]any{}
	for key, value := range task.InputRequirements {
		if key == "required_inputs" {
			stored[key] = value
			continue
		}
		notes[key] = value
	}
	return stored, notes
}

func (s *ProjectStore) ListDispatchableTasks(ctx context.Context, input ListDispatchableTasksInput) ([]uuid.UUID, error) {
	if s.repository == nil {
		return nil, ErrActivityStoreRequired
	}
	tasks, err := s.repository.ListProjectTasksByCoordinationJob(ctx, input.TenantID, input.ProjectID, input.CoordinationJobID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	candidates := make([]project.ProjectTask, 0, len(tasks))
	candidateIDs := make([]uuid.UUID, 0, len(tasks))
	for _, task := range tasks {
		if !projectTaskDispatchAllowed(task.Status) {
			continue
		}
		if task.RetryNotBefore != nil && task.RetryNotBefore.After(now) {
			continue
		}
		candidates = append(candidates, task)
		candidateIDs = append(candidateIDs, task.ID)
	}
	if len(candidates) == 0 {
		return []uuid.UUID{}, nil
	}
	unresolved, err := s.repository.ListUnresolvedBlockersForTasks(ctx, input.TenantID, input.ProjectID, candidateIDs)
	if err != nil {
		return nil, err
	}
	blockedByTaskID := unresolvedBlockersByDependent(unresolved)
	dispatchable := make([]uuid.UUID, 0, len(candidates))
	for _, task := range candidates {
		if _, blocked := blockedByTaskID[task.ID]; blocked {
			continue
		}
		dispatchable = append(dispatchable, task.ID)
	}
	return dispatchable, nil
}

func (s *ProjectStore) ResolveReadyDownstream(ctx context.Context, input ResolveReadyDownstreamInput) ([]uuid.UUID, error) {
	if s.repository == nil {
		return nil, ErrActivityStoreRequired
	}
	dependentIDs, err := s.repository.ListDependentsOfTask(ctx, input.TenantID, input.ProjectID, input.CompletedTaskID)
	if err != nil {
		return nil, err
	}
	if len(dependentIDs) == 0 {
		return []uuid.UUID{}, nil
	}
	unresolved, err := s.repository.ListUnresolvedBlockersForTasks(ctx, input.TenantID, input.ProjectID, dependentIDs)
	if err != nil {
		return nil, err
	}
	blockedByTaskID := unresolvedBlockersByDependent(unresolved)
	readyIDs := make([]uuid.UUID, 0, len(dependentIDs))
	for _, taskID := range dependentIDs {
		if _, blocked := blockedByTaskID[taskID]; blocked {
			continue
		}
		task, err := s.repository.GetProjectTask(ctx, input.TenantID, taskID)
		if err != nil {
			return nil, err
		}
		if task.ProjectID != input.ProjectID {
			return nil, project.ErrProjectNotFound
		}
		if task.Status != "blocked" {
			continue
		}
		updated, err := s.repository.UpdateProjectTaskStatus(ctx, input.TenantID, taskID, "planned", nil, []string{"blocked"})
		if err != nil {
			if errors.Is(err, project.ErrProjectConflict) {
				continue
			}
			return nil, err
		}
		readyIDs = append(readyIDs, updated.ID)
	}
	return readyIDs, nil
}

func (s *ProjectStore) InspectTaskResultDecision(ctx context.Context, input InspectTaskResultDecisionInput) (InspectTaskResultDecisionResult, error) {
	if s.repository == nil {
		return InspectTaskResultDecisionResult{}, ErrActivityStoreRequired
	}
	task, err := s.repository.GetProjectTask(ctx, input.TenantID, input.ProjectTaskID)
	if err != nil {
		return InspectTaskResultDecisionResult{}, err
	}
	if task.ProjectID != input.ProjectID {
		return InspectTaskResultDecisionResult{}, project.ErrProjectNotFound
	}
	result, err := s.latestTaskResult(ctx, task)
	if err != nil || result == nil {
		return InspectTaskResultDecisionResult{}, err
	}
	return InspectTaskResultDecisionResult{
		ResultID:  result.ID,
		Decision:  string(result.Decision),
		Exhausted: s.revisionBudgetExhausted(ctx, input.TenantID, input.ProjectID, task),
		Blocker:   result.Contract.Blocker,
	}, nil
}

func (s *ProjectStore) CreateRevisionTaskForResult(ctx context.Context, input CreateRevisionTaskForResultInput) (CreateRevisionTaskForResultResult, error) {
	if s.repository == nil {
		return CreateRevisionTaskForResultResult{}, ErrActivityStoreRequired
	}
	source, err := s.repository.GetProjectTask(ctx, input.TenantID, input.SourceTaskID)
	if err != nil {
		return CreateRevisionTaskForResultResult{}, err
	}
	if source.ProjectID != input.ProjectID {
		return CreateRevisionTaskForResultResult{}, project.ErrProjectNotFound
	}
	result, err := s.taskResultByID(ctx, input.TenantID, input.ProjectID, input.SourceTaskID, input.ResultID)
	if err != nil {
		return CreateRevisionTaskForResultResult{}, err
	}
	if result.Decision != project.TaskResultDecisionRevisionAttempt || result.ResultStatus != project.TaskResultStatusRevisionNeeded || result.Contract.RevisionRequest == nil {
		return CreateRevisionTaskForResultResult{}, project.ErrInvalidProject
	}
	if s.revisionBudgetExhausted(ctx, input.TenantID, input.ProjectID, source) {
		return CreateRevisionTaskForResultResult{Exhausted: true}, nil
	}
	revision, err := s.repository.CreateProjectTask(ctx, project.CreateProjectTaskRequest{
		TenantID:                  input.TenantID,
		ProjectID:                 input.ProjectID,
		DemandID:                  source.DemandID,
		Title:                     revisionTaskTitle(source, result),
		Summary:                   revisionTaskSummary(source, result),
		Status:                    project.ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: source.AssignedDigitalEmployeeID,
		RiskLevel:                 stringPtrValue(source.RiskLevel),
		RequiresHumanApproval:     source.RequiresHumanApproval,
		CoordinationJobID:         source.CoordinationJobID,
		RouteDecisionID:           source.RouteDecisionID,
		PlannedTaskKey:            revisionTaskKey(source, result),
		TaskKind:                  source.TaskKind,
		StageIndex:                source.StageIndex,
		RevisionOfTaskID:          &source.ID,
		ExpectedOutputs:           append([]any(nil), source.ExpectedOutputs...),
		InputRequirements:         revisionInputRequirements(source, result),
		HandoffContract:           cloneAnyMap(source.HandoffContract),
		PlannerMetadata:           revisionPlannerMetadata(source, result),
		BlockedByTaskIDs:          append([]uuid.UUID(nil), source.BlockedByTaskIDs...),
	})
	if err != nil {
		return CreateRevisionTaskForResultResult{}, err
	}
	if _, err := s.repository.LinkProjectTaskResultRevisionTask(ctx, input.TenantID, input.ProjectID, result.ID, revision.ID); err != nil {
		return CreateRevisionTaskForResultResult{}, err
	}
	return CreateRevisionTaskForResultResult{TaskID: revision.ID}, nil
}

// CreateUpstreamSupplementTasks appends a task for the owner of each missing
// input. The blocked source is downstream of that owner and is re-run when the
// supplement completes.
func (s *ProjectStore) CreateUpstreamSupplementTasks(ctx context.Context, input CreateUpstreamSupplementInput) (CreateUpstreamSupplementResult, error) {
	if s.repository == nil {
		return CreateUpstreamSupplementResult{}, ErrActivityStoreRequired
	}
	source, err := s.repository.GetProjectTask(ctx, input.TenantID, input.SourceTaskID)
	if err != nil {
		return CreateUpstreamSupplementResult{}, err
	}
	if source.ProjectID != input.ProjectID {
		return CreateUpstreamSupplementResult{}, project.ErrProjectNotFound
	}
	if source.CoordinationJobID == nil || *source.CoordinationJobID == uuid.Nil {
		return CreateUpstreamSupplementResult{}, project.ErrInvalidProject
	}
	siblings, err := s.repository.ListProjectTasksByCoordinationJob(ctx, input.TenantID, input.ProjectID, *source.CoordinationJobID)
	if err != nil {
		return CreateUpstreamSupplementResult{}, err
	}
	projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return CreateUpstreamSupplementResult{}, err
	}
	planIteration := currentPlanIteration(siblings)
	if planIteration >= int32(maxPlanIterations(projectRecord.CoordinationPolicy)) {
		return CreateUpstreamSupplementResult{Exhausted: true}, nil
	}
	planIteration++
	owners := make(map[string]project.ProjectTask)
	for _, task := range siblings {
		for _, produced := range plannerProducesFromMetadata(task.PlannerMetadata) {
			owners[produced] = task
		}
	}

	seen := make(map[uuid.UUID]struct{})
	result := CreateUpstreamSupplementResult{}
	for _, missing := range input.MissingInputs {
		owner, ok := owners[missing]
		if !ok {
			return CreateUpstreamSupplementResult{}, project.ErrInvalidProject
		}
		if _, duplicate := seen[owner.ID]; duplicate {
			continue
		}
		seen[owner.ID] = struct{}{}
		supplement, err := s.repository.CreateProjectTask(ctx, project.CreateProjectTaskRequest{
			TenantID:                  input.TenantID,
			ProjectID:                 input.ProjectID,
			DemandID:                  source.DemandID,
			Title:                     owner.Title,
			Summary:                   "上游补做：" + strings.Join(input.MissingInputs, ", "),
			Status:                    project.ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: owner.AssignedDigitalEmployeeID,
			RiskLevel:                 stringPtrValue(owner.RiskLevel),
			RequiresHumanApproval:     owner.RequiresHumanApproval,
			CoordinationJobID:         source.CoordinationJobID,
			RouteDecisionID:           source.RouteDecisionID,
			PlannedTaskKey:            upstreamSupplementTaskKey(owner, planIteration),
			TaskKind:                  owner.TaskKind,
			StageIndex:                owner.StageIndex,
			RevisionOfTaskID:          &owner.ID,
			AcceptedPlanRevisionID:    source.AcceptedPlanRevisionID,
			ExpectedOutputs:           append([]any(nil), owner.ExpectedOutputs...),
			InputRequirements:         cloneAnyMap(owner.InputRequirements),
			HandoffContract:           cloneAnyMap(owner.HandoffContract),
			PlannerMetadata:           revisionPlannerMetadataForSupplement(owner, input.SourceTaskID, input.MissingInputs),
			PlanIteration:             planIteration,
		})
		if err != nil {
			return CreateUpstreamSupplementResult{}, err
		}
		result.TaskIDs = append(result.TaskIDs, supplement.ID)
	}
	return result, nil
}

func (s *ProjectStore) HoldDownstreamForFailure(ctx context.Context, input HoldDownstreamForFailureInput) (DecisionRequestResult, error) {
	if s.repository == nil || s.approvals == nil {
		return DecisionRequestResult{}, ErrActivityStoreRequired
	}
	projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return DecisionRequestResult{}, err
	}
	failedTask, err := s.repository.GetProjectTask(ctx, input.TenantID, input.FailedTaskID)
	if err != nil {
		return DecisionRequestResult{}, err
	}
	if failedTask.ProjectID != input.ProjectID {
		return DecisionRequestResult{}, project.ErrProjectNotFound
	}
	downstreamIDs, err := s.recursiveDownstreamTaskIDs(ctx, input.TenantID, input.ProjectID, input.FailedTaskID)
	if err != nil {
		return DecisionRequestResult{}, err
	}
	for _, taskID := range downstreamIDs {
		task, err := s.repository.GetProjectTask(ctx, input.TenantID, taskID)
		if err != nil {
			return DecisionRequestResult{}, err
		}
		if task.ProjectID != input.ProjectID {
			return DecisionRequestResult{}, project.ErrProjectNotFound
		}
		if projectTaskTerminalStatus(task.Status) || task.Status == "blocked" {
			continue
		}
		if _, err := s.repository.UpdateProjectTaskStatus(ctx, input.TenantID, taskID, "blocked", nil, failureHoldCurrentStatuses()); err != nil {
			if errors.Is(err, project.ErrProjectConflict) {
				continue
			}
			return DecisionRequestResult{}, err
		}
	}
	approvalRequest, err := s.approvals.CreateRequest(ctx, approval.CreateRequestInput{
		TenantID:       input.TenantID,
		ResourceType:   "project_task",
		ResourceID:     input.FailedTaskID,
		RequesterType:  "project_coordinator",
		TargetUserID:   projectRecord.HumanOwnerUserID,
		DecisionType:   "task_failure_recovery",
		Title:          "处理项目任务失败",
		Summary:        input.FailureSummary,
		RiskLevel:      "high",
		Options:        []any{"approved", "rejected", "needs_more_evidence"},
		ContextPayload: failureRecoveryContext(input, downstreamIDs),
	})
	if err != nil {
		return DecisionRequestResult{}, err
	}
	event, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventDecisionRequested, input.FailedTaskID.String(), "项目任务失败需要恢复决策", map[string]any{
		"approval_request_id": approvalRequest.ID.String(),
		"project_task_id":     input.FailedTaskID.String(),
		"failed_event_id":     input.FailedEventID.String(),
		"target_user_id":      projectRecord.HumanOwnerUserID.String(),
	}))
	if err != nil {
		return DecisionRequestResult{}, err
	}
	failedTaskID := input.FailedTaskID
	decision, err := s.repository.CreateDecisionRequest(ctx, project.CreateDecisionRequestRequest{
		TenantID:          input.TenantID,
		ProjectID:         input.ProjectID,
		ApprovalRequestID: approvalRequest.ID,
		ProjectTaskID:     &failedTaskID,
		TargetUserID:      projectRecord.HumanOwnerUserID,
		DecisionType:      "task_failure_recovery",
		TitleSnapshot:     "处理项目任务失败",
		SummarySnapshot:   input.FailureSummary,
		RiskLevelSnapshot: "high",
		StatusSnapshot:    "pending",
		CreatedEventID:    &event.ID,
	})
	if err != nil {
		return DecisionRequestResult{}, err
	}
	if s.inbox != nil {
		if err := s.inbox.UpsertProjectDecisionRequest(ctx, decision); err != nil {
			return DecisionRequestResult{}, err
		}
	}
	return DecisionRequestResult{ID: decision.ID}, nil
}

func (s *ProjectStore) RequestProjectTaskIterationExhaustedReview(ctx context.Context, input RequestProjectTaskIterationExhaustedReviewInput) (DecisionRequestResult, error) {
	if s.repository == nil || s.approvals == nil {
		return DecisionRequestResult{}, ErrActivityStoreRequired
	}
	projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return DecisionRequestResult{}, err
	}
	task, err := s.repository.GetProjectTask(ctx, input.TenantID, input.ProjectTaskID)
	if err != nil {
		return DecisionRequestResult{}, err
	}
	if task.ProjectID != input.ProjectID {
		return DecisionRequestResult{}, project.ErrProjectNotFound
	}
	downstreamIDs, err := s.recursiveDownstreamTaskIDs(ctx, input.TenantID, input.ProjectID, input.ProjectTaskID)
	if err != nil {
		return DecisionRequestResult{}, err
	}
	for _, taskID := range downstreamIDs {
		downstream, err := s.repository.GetProjectTask(ctx, input.TenantID, taskID)
		if err != nil {
			return DecisionRequestResult{}, err
		}
		if downstream.ProjectID != input.ProjectID {
			return DecisionRequestResult{}, project.ErrProjectNotFound
		}
		if projectTaskTerminalStatus(downstream.Status) || downstream.Status == "blocked" {
			continue
		}
		if _, err := s.repository.UpdateProjectTaskStatus(ctx, input.TenantID, taskID, "blocked", nil, failureHoldCurrentStatuses()); err != nil {
			if errors.Is(err, project.ErrProjectConflict) {
				continue
			}
			return DecisionRequestResult{}, err
		}
	}
	summary := strings.TrimSpace(input.Summary)
	if summary == "" {
		summary = "同一失败重复出现，需要人类判断是否继续"
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		reason = "iteration_exhausted"
	}
	approvalRequest, err := s.approvals.CreateRequest(ctx, approval.CreateRequestInput{
		TenantID:       input.TenantID,
		ResourceType:   "project_task",
		ResourceID:     input.ProjectTaskID,
		RequesterType:  "project_coordinator",
		TargetUserID:   projectRecord.HumanOwnerUserID,
		DecisionType:   "project_task_iteration_exhausted",
		Title:          "处理项目任务修订耗尽",
		Summary:        summary,
		RiskLevel:      "high",
		Options:        []any{"approved", "rejected", "needs_more_evidence"},
		ContextPayload: iterationExhaustedContext(input, reason, summary, downstreamIDs),
	})
	if err != nil {
		return DecisionRequestResult{}, err
	}
	event, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventDecisionRequested, input.ProjectTaskID.String(), summary, map[string]any{
		"approval_request_id": approvalRequest.ID.String(),
		"project_task_id":     input.ProjectTaskID.String(),
		"result_id":           input.ResultID.String(),
		"reason":              reason,
		"completed_event_id":  input.CreatedEventID.String(),
		"target_user_id":      projectRecord.HumanOwnerUserID.String(),
	}))
	if err != nil {
		return DecisionRequestResult{}, err
	}
	taskID := input.ProjectTaskID
	decision, err := s.repository.CreateDecisionRequest(ctx, project.CreateDecisionRequestRequest{
		TenantID:          input.TenantID,
		ProjectID:         input.ProjectID,
		ApprovalRequestID: approvalRequest.ID,
		ProjectTaskID:     &taskID,
		TargetUserID:      projectRecord.HumanOwnerUserID,
		DecisionType:      "project_task_iteration_exhausted",
		TitleSnapshot:     "处理项目任务修订耗尽",
		SummarySnapshot:   summary,
		RiskLevelSnapshot: "high",
		StatusSnapshot:    "pending",
		CreatedEventID:    &event.ID,
	})
	if err != nil {
		return DecisionRequestResult{}, err
	}
	if s.inbox != nil {
		if err := s.inbox.UpsertProjectDecisionRequest(ctx, decision); err != nil {
			return DecisionRequestResult{}, err
		}
	}
	return DecisionRequestResult{ID: decision.ID}, nil
}

func (s *ProjectStore) ApplyFailureRecoveryDecision(ctx context.Context, input ApplyFailureRecoveryDecisionInput) (ApplyFailureRecoveryDecisionResult, error) {
	if s.repository == nil {
		return ApplyFailureRecoveryDecisionResult{}, ErrActivityStoreRequired
	}
	decision, err := s.repository.GetDecisionRequest(ctx, input.TenantID, input.ProjectID, input.DecisionRequestID)
	if err != nil {
		return ApplyFailureRecoveryDecisionResult{}, err
	}
	if decision.DecisionType != "task_failure_recovery" || decision.ProjectTaskID == nil {
		return ApplyFailureRecoveryDecisionResult{}, project.ErrInvalidProject
	}
	action, err := parseFailureRecoveryAction(input.Decision, input.Payload)
	if err != nil {
		return ApplyFailureRecoveryDecisionResult{}, err
	}
	if action.Action == "needs_more_evidence" {
		return ApplyFailureRecoveryDecisionResult{ReadyTaskIDs: []uuid.UUID{}}, nil
	}
	source, err := s.repository.GetProjectTask(ctx, input.TenantID, *decision.ProjectTaskID)
	if err != nil {
		return ApplyFailureRecoveryDecisionResult{}, err
	}
	if source.ProjectID != input.ProjectID {
		return ApplyFailureRecoveryDecisionResult{}, project.ErrProjectNotFound
	}
	switch action.Action {
	case "retry":
		replacement, err := s.createRecoveryReplacementTask(ctx, input, decision, source, action)
		if err != nil {
			return ApplyFailureRecoveryDecisionResult{}, err
		}
		return s.recoveryReplacementReadyResult(ctx, input.TenantID, input.ProjectID, replacement)
	case "reassign":
		if action.NewDigitalEmployeeID == nil {
			return ApplyFailureRecoveryDecisionResult{}, project.ErrInvalidProject
		}
		if err := s.validateActiveDigitalProjectMember(ctx, input.TenantID, input.ProjectID, *action.NewDigitalEmployeeID); err != nil {
			return ApplyFailureRecoveryDecisionResult{}, err
		}
		replacement, err := s.createRecoveryReplacementTask(ctx, input, decision, source, action)
		if err != nil {
			return ApplyFailureRecoveryDecisionResult{}, err
		}
		return s.recoveryReplacementReadyResult(ctx, input.TenantID, input.ProjectID, replacement)
	case "cancel_downstream":
		if err := s.cancelFailureDownstream(ctx, input, source); err != nil {
			return ApplyFailureRecoveryDecisionResult{}, err
		}
		return ApplyFailureRecoveryDecisionResult{ReadyTaskIDs: []uuid.UUID{}}, nil
	default:
		return ApplyFailureRecoveryDecisionResult{}, project.ErrInvalidProject
	}
}

func (s *ProjectStore) LoadHumanDecisionRoute(ctx context.Context, input LoadHumanDecisionRouteInput) (HumanDecisionRouteResult, error) {
	if s.repository == nil {
		return HumanDecisionRouteResult{}, ErrActivityStoreRequired
	}
	decision, err := s.repository.GetDecisionRequest(ctx, input.TenantID, input.ProjectID, input.DecisionRequestID)
	if err != nil {
		if errors.Is(err, project.ErrProjectNotFound) {
			return HumanDecisionRouteResult{}, nil
		}
		return HumanDecisionRouteResult{}, err
	}
	result := HumanDecisionRouteResult{
		Decision: ProjectDecisionSnapshot{
			ID:                   decision.ID,
			ProjectID:            decision.ProjectID,
			DecisionType:         decision.DecisionType,
			StatusSnapshot:       decision.StatusSnapshot,
			CoordinationJobID:    uuidValue(decision.CoordinationJobID),
			ProjectTaskID:        uuidValue(decision.ProjectTaskID),
			PlanRevisionID:       uuidValue(decision.PlanRevisionID),
			DispatchGateResultID: uuidValue(decision.DispatchGateResultID),
			CreatedEventID:       uuidValue(decision.CreatedEventID),
		},
	}
	if decision.DecisionType != "plan_review" {
		return result, nil
	}
	if decision.PlanRevisionID == nil || *decision.PlanRevisionID == uuid.Nil {
		return HumanDecisionRouteResult{}, project.ErrInvalidProject
	}
	revision, err := s.repository.GetPlanRevision(ctx, input.TenantID, input.ProjectID, *decision.PlanRevisionID)
	if err != nil {
		return HumanDecisionRouteResult{}, err
	}
	if revision.CoordinationJobID == nil || *revision.CoordinationJobID == uuid.Nil ||
		revision.RouteDecisionID == nil || *revision.RouteDecisionID == uuid.Nil {
		return HumanDecisionRouteResult{}, project.ErrInvalidProject
	}
	payload, err := planRevisionPayloadFromMap(revision.Payload)
	if err != nil {
		return HumanDecisionRouteResult{}, err
	}
	result.PlanReview = &PlanReviewRoute{
		ProjectID:         revision.ProjectID,
		DemandID:          revision.DemandID,
		CoordinationJobID: *revision.CoordinationJobID,
		RouteDecisionID:   *revision.RouteDecisionID,
		PlanRevisionID:    revision.ID,
		PlanFingerprint:   revision.PlanFingerprint,
		Payload:           payload,
		PlanEventID:       uuidValue(revision.CreatedEventID),
	}
	routeDecision, err := s.repository.GetRouteDecision(ctx, input.TenantID, *revision.RouteDecisionID)
	if err != nil {
		return HumanDecisionRouteResult{}, err
	}
	if routeDecision.ID != *revision.RouteDecisionID {
		return HumanDecisionRouteResult{}, project.ErrInvalidProject
	}
	result.PlanReview.RouteEventID = uuidValue(routeDecision.CreatedEventID)
	outputEventIDs, err := s.planReviewRouteOutputEventIDs(ctx, input.TenantID, input.ProjectID, revision)
	if err != nil {
		return HumanDecisionRouteResult{}, err
	}
	result.PlanReview.OutputEventIDs = outputEventIDs
	return result, nil
}

func (s *ProjectStore) planReviewRouteOutputEventIDs(ctx context.Context, tenantID, projectID uuid.UUID, currentRevision project.PlanRevision) ([]uuid.UUID, error) {
	revisions, err := s.repository.ListPlanRevisionsForDemand(ctx, tenantID, projectID, currentRevision.DemandID)
	if err != nil {
		return nil, err
	}
	outputEventIDs := []uuid.UUID{}
	for _, revision := range revisions {
		if revision.DemandID != currentRevision.DemandID || revision.RevisionNumber > currentRevision.RevisionNumber {
			continue
		}
		if revision.CoordinationJobID == nil || *revision.CoordinationJobID == uuid.Nil ||
			revision.RouteDecisionID == nil || *revision.RouteDecisionID == uuid.Nil {
			return nil, project.ErrInvalidProject
		}
		routeDecision, err := s.repository.GetRouteDecision(ctx, tenantID, *revision.RouteDecisionID)
		if err != nil {
			return nil, err
		}
		if routeDecision.ID != *revision.RouteDecisionID {
			return nil, project.ErrInvalidProject
		}
		outputEventIDs = appendUniqueUUID(outputEventIDs, uuidValue(routeDecision.CreatedEventID))
		outputEventIDs = appendUniqueUUID(outputEventIDs, uuidValue(revision.CreatedEventID))
		if revision.ID == currentRevision.ID {
			return outputEventIDs, nil
		}
		decision, err := s.planReviewDecisionForRevision(ctx, tenantID, projectID, revision.ID)
		if err != nil {
			return nil, err
		}
		outputEventIDs = appendUniqueUUID(outputEventIDs, uuidValue(decision.ResolvedEventID))
	}
	return outputEventIDs, nil
}

func (s *ProjectStore) planReviewDecisionForRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (project.DecisionRequest, error) {
	decision, err := s.repository.GetDecisionRequestByPlanRevision(ctx, tenantID, projectID, revisionID)
	if err != nil {
		if errors.Is(err, project.ErrProjectNotFound) {
			return project.DecisionRequest{}, nil
		}
		return project.DecisionRequest{}, err
	}
	return decision, nil
}

func appendUniqueUUID(values []uuid.UUID, value uuid.UUID) []uuid.UUID {
	if value == uuid.Nil {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func parseFailureRecoveryAction(decision string, payload map[string]any) (FailureRecoveryAction, error) {
	switch decision {
	case "needs_more_evidence":
		return FailureRecoveryAction{Action: "needs_more_evidence"}, nil
	case "rejected":
		return FailureRecoveryAction{Action: "cancel_downstream"}, nil
	case "approved":
		raw, _ := payload["recovery_action"].(string)
		switch strings.TrimSpace(raw) {
		case "retry", "cancel_downstream":
			return FailureRecoveryAction{Action: strings.TrimSpace(raw)}, nil
		case "reassign":
			idText, _ := payload["new_digital_employee_id"].(string)
			id, err := uuid.Parse(strings.TrimSpace(idText))
			if err != nil {
				return FailureRecoveryAction{}, project.ErrInvalidProject
			}
			return FailureRecoveryAction{Action: "reassign", NewDigitalEmployeeID: &id}, nil
		default:
			return FailureRecoveryAction{}, project.ErrInvalidProject
		}
	default:
		return FailureRecoveryAction{}, project.ErrInvalidProject
	}
}

func (s *ProjectStore) createRecoveryReplacementTask(ctx context.Context, input ApplyFailureRecoveryDecisionInput, decision project.DecisionRequest, source project.ProjectTask, action FailureRecoveryAction) (project.ProjectTask, error) {
	assigneeID := source.AssignedDigitalEmployeeID
	if action.NewDigitalEmployeeID != nil {
		assigneeID = action.NewDigitalEmployeeID
	}
	if assigneeID == nil || source.DemandID == nil || source.CoordinationJobID == nil {
		return project.ProjectTask{}, project.ErrInvalidProject
	}
	replacementKey := recoveryReplacementTaskKey(source)
	sourceBlockers, err := s.repository.ListProjectTaskDependencies(ctx, input.TenantID, input.ProjectID, []uuid.UUID{source.ID})
	if err != nil {
		return project.ProjectTask{}, err
	}
	replacement, exists, err := s.findExistingRecoveryReplacement(ctx, input.TenantID, input.ProjectID, source, action, replacementKey)
	if err != nil {
		return project.ProjectTask{}, err
	}
	if !exists {
		status, err := s.recoveryReplacementStatus(ctx, input.TenantID, input.ProjectID, sourceBlockers)
		if err != nil {
			return project.ProjectTask{}, err
		}
		replacement, err = s.repository.CreateProjectTask(ctx, project.CreateProjectTaskRequest{
			TenantID:                  input.TenantID,
			ProjectID:                 input.ProjectID,
			DemandID:                  source.DemandID,
			Title:                     recoveryReplacementTitle(source.Title),
			Summary:                   stringPtrValue(source.Summary),
			Status:                    status,
			AssignedDigitalEmployeeID: assigneeID,
			RiskLevel:                 stringPtrValue(source.RiskLevel),
			RequiresHumanApproval:     source.RequiresHumanApproval,
			CoordinationJobID:         source.CoordinationJobID,
			RouteDecisionID:           source.RouteDecisionID,
			PlannedTaskKey:            &replacementKey,
			TaskKind:                  source.TaskKind,
			StageIndex:                source.StageIndex,
			ExpectedOutputs:           append([]any(nil), source.ExpectedOutputs...),
			InputRequirements:         cloneAnyMap(source.InputRequirements),
			HandoffContract:           cloneAnyMap(source.HandoffContract),
			PlannerMetadata:           recoveryPlannerMetadata(source, decision.ID, action),
		})
		if err != nil {
			existing, ok, findErr := s.findExistingRecoveryReplacement(ctx, input.TenantID, input.ProjectID, source, action, replacementKey)
			if findErr != nil {
				return project.ProjectTask{}, findErr
			}
			if !ok {
				return project.ProjectTask{}, err
			}
			replacement = existing
		}
	}
	if err := s.ensureRecoveryTaskCreatedEvent(ctx, input, decision.ID, source.ID, replacement.ID, action.Action); err != nil {
		return project.ProjectTask{}, err
	}
	if err := s.ensureReplacementBlockerDependencies(ctx, input.TenantID, input.ProjectID, replacement.ID, sourceBlockers); err != nil {
		return project.ProjectTask{}, err
	}
	if err := s.rewireRecoverableDependents(ctx, input.TenantID, input.ProjectID, source.ID, replacement.ID); err != nil {
		return project.ProjectTask{}, err
	}
	return replacement, nil
}

func (s *ProjectStore) ensureRecoveryTaskCreatedEvent(ctx context.Context, input ApplyFailureRecoveryDecisionInput, decisionRequestID, sourceTaskID, replacementTaskID uuid.UUID, action string) error {
	exists, err := s.repository.ProjectTaskEventExists(ctx, input.TenantID, input.ProjectID, project.ProjectEventTaskCreated, replacementTaskID.String())
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventTaskCreated, replacementTaskID.String(), "恢复任务已创建", map[string]any{
		"project_task_id":        replacementTaskID.String(),
		"source_project_task_id": sourceTaskID.String(),
		"decision_request_id":    decisionRequestID.String(),
		"recovery_action":        action,
	}))
	return err
}

func (s *ProjectStore) findExistingRecoveryReplacement(ctx context.Context, tenantID, projectID uuid.UUID, source project.ProjectTask, action FailureRecoveryAction, replacementKey string) (project.ProjectTask, bool, error) {
	if source.CoordinationJobID == nil {
		return project.ProjectTask{}, false, nil
	}
	tasks, err := s.repository.ListProjectTasksByCoordinationJob(ctx, tenantID, projectID, *source.CoordinationJobID)
	if err != nil {
		return project.ProjectTask{}, false, err
	}
	for _, task := range tasks {
		if task.PlannedTaskKey == nil || *task.PlannedTaskKey != replacementKey {
			continue
		}
		if task.PlannerMetadata["source_task_id"] != source.ID.String() || task.PlannerMetadata["recovery_action"] != action.Action {
			return project.ProjectTask{}, false, project.ErrProjectConflict
		}
		if action.NewDigitalEmployeeID != nil && (task.AssignedDigitalEmployeeID == nil || *task.AssignedDigitalEmployeeID != *action.NewDigitalEmployeeID) {
			return project.ProjectTask{}, false, project.ErrProjectConflict
		}
		return task, true, nil
	}
	return project.ProjectTask{}, false, nil
}

func (s *ProjectStore) recoveryReplacementReadyResult(ctx context.Context, tenantID, projectID uuid.UUID, replacement project.ProjectTask) (ApplyFailureRecoveryDecisionResult, error) {
	result := ApplyFailureRecoveryDecisionResult{ReadyTaskIDs: []uuid.UUID{}}
	if !projectTaskDispatchAllowed(replacement.Status) {
		return result, nil
	}
	blockers, err := s.repository.ListUnresolvedBlockersForTasks(ctx, tenantID, projectID, []uuid.UUID{replacement.ID})
	if err != nil {
		return ApplyFailureRecoveryDecisionResult{}, err
	}
	if len(blockers) > 0 {
		return result, nil
	}
	result.ReadyTaskIDs = append(result.ReadyTaskIDs, replacement.ID)
	return result, nil
}

func (s *ProjectStore) recoveryReplacementStatus(ctx context.Context, tenantID, projectID uuid.UUID, sourceBlockers []project.ProjectTaskDependency) (string, error) {
	for _, dependency := range sourceBlockers {
		blocker, err := s.repository.GetProjectTask(ctx, tenantID, dependency.BlockerTaskID)
		if err != nil {
			return "", err
		}
		if blocker.ProjectID != projectID {
			return "", project.ErrProjectNotFound
		}
		if blocker.Status != "completed" {
			return "blocked", nil
		}
	}
	return "planned", nil
}

func (s *ProjectStore) ensureReplacementBlockerDependencies(ctx context.Context, tenantID, projectID, replacementID uuid.UUID, sourceBlockers []project.ProjectTaskDependency) error {
	if len(sourceBlockers) == 0 {
		return nil
	}
	dependencyRepository, ok := s.repository.(recoveryDependencyRepository)
	if !ok {
		return ErrActivityStoreRequired
	}
	existing, err := s.repository.ListProjectTaskDependencies(ctx, tenantID, projectID, []uuid.UUID{replacementID})
	if err != nil {
		return err
	}
	for _, sourceBlocker := range sourceBlockers {
		if dependencyExists(existing, replacementID, sourceBlocker.BlockerTaskID) {
			continue
		}
		if _, err := dependencyRepository.CreateProjectTaskDependency(ctx, project.CreateProjectTaskDependencyRequest{
			TenantID:          tenantID,
			ProjectID:         projectID,
			CoordinationJobID: sourceBlocker.CoordinationJobID,
			DependentTaskID:   replacementID,
			BlockerTaskID:     sourceBlocker.BlockerTaskID,
		}); err != nil {
			refreshed, refreshErr := s.repository.ListProjectTaskDependencies(ctx, tenantID, projectID, []uuid.UUID{replacementID})
			if refreshErr == nil && dependencyExists(refreshed, replacementID, sourceBlocker.BlockerTaskID) {
				continue
			}
			return err
		}
	}
	return nil
}

func (s *ProjectStore) rewireRecoverableDependents(ctx context.Context, tenantID, projectID, oldBlockerID, newBlockerID uuid.UUID) error {
	dependentIDs, err := s.repository.ListDependentsOfTask(ctx, tenantID, projectID, oldBlockerID)
	if err != nil {
		return err
	}
	rewireIDs := make([]uuid.UUID, 0, len(dependentIDs))
	for _, taskID := range dependentIDs {
		task, err := s.repository.GetProjectTask(ctx, tenantID, taskID)
		if err != nil {
			return err
		}
		if task.ProjectID != projectID {
			return project.ErrProjectNotFound
		}
		if projectTaskTerminalStatus(task.Status) {
			continue
		}
		rewireIDs = append(rewireIDs, taskID)
	}
	if len(rewireIDs) == 0 {
		return nil
	}
	dependencyRepository, ok := s.repository.(recoveryDependencyRepository)
	if !ok {
		return ErrActivityStoreRequired
	}
	_, err = dependencyRepository.RewireProjectTaskDependencies(ctx, project.RewireProjectTaskDependenciesRequest{
		TenantID:         tenantID,
		ProjectID:        projectID,
		DependentTaskIDs: rewireIDs,
		OldBlockerTaskID: oldBlockerID,
		NewBlockerTaskID: newBlockerID,
	})
	return err
}

func (s *ProjectStore) cancelFailureDownstream(ctx context.Context, input ApplyFailureRecoveryDecisionInput, source project.ProjectTask) error {
	downstreamIDs, err := s.recursiveDownstreamTaskIDs(ctx, input.TenantID, input.ProjectID, source.ID)
	if err != nil {
		return err
	}
	for _, taskID := range downstreamIDs {
		task, err := s.repository.GetProjectTask(ctx, input.TenantID, taskID)
		if err != nil {
			return err
		}
		if task.ProjectID != input.ProjectID {
			return project.ErrProjectNotFound
		}
		if task.Status == "cancelled" {
			if err := s.ensureProjectTaskCancelledEvent(ctx, input, source.ID, task.ID); err != nil {
				return err
			}
			continue
		}
		if !projectTaskCancellationAllowed(task.Status) {
			continue
		}
		updated, err := s.repository.UpdateProjectTaskStatus(ctx, input.TenantID, taskID, "cancelled", nil, []string{"blocked", "planned", "pending"})
		if err != nil {
			if errors.Is(err, project.ErrProjectConflict) {
				continue
			}
			return err
		}
		if err := s.ensureProjectTaskCancelledEvent(ctx, input, source.ID, updated.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *ProjectStore) ensureProjectTaskCancelledEvent(ctx context.Context, input ApplyFailureRecoveryDecisionInput, sourceTaskID, cancelledTaskID uuid.UUID) error {
	exists, err := s.repository.ProjectTaskEventExists(ctx, input.TenantID, input.ProjectID, project.ProjectEventTaskCancelled, cancelledTaskID.String())
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventTaskCancelled, cancelledTaskID.String(), "项目任务已取消", map[string]any{
		"project_task_id":        cancelledTaskID.String(),
		"source_project_task_id": sourceTaskID.String(),
		"decision_request_id":    input.DecisionRequestID.String(),
		"recovery_action":        "cancel_downstream",
	}))
	return err
}

func (s *ProjectStore) validateActiveDigitalProjectMember(ctx context.Context, tenantID, projectID, digitalEmployeeID uuid.UUID) error {
	members, err := s.repository.ListProjectMembers(ctx, tenantID, projectID)
	if err != nil {
		return err
	}
	for _, member := range members {
		if member.PrincipalType == project.PrincipalTypeDigitalEmployee && member.PrincipalID == digitalEmployeeID && member.Status == "active" {
			return nil
		}
	}
	return project.ErrInvalidProject
}

func (s *ProjectStore) recursiveDownstreamTaskIDs(ctx context.Context, tenantID, projectID, failedTaskID uuid.UUID) ([]uuid.UUID, error) {
	seen := map[uuid.UUID]struct{}{failedTaskID: {}}
	queue := []uuid.UUID{failedTaskID}
	result := make([]uuid.UUID, 0)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		dependents, err := s.repository.ListDependentsOfTask(ctx, tenantID, projectID, current)
		if err != nil {
			return nil, err
		}
		for _, dependentID := range dependents {
			if _, exists := seen[dependentID]; exists {
				continue
			}
			seen[dependentID] = struct{}{}
			result = append(result, dependentID)
			queue = append(queue, dependentID)
		}
	}
	return result, nil
}

func projectTaskTerminalStatus(status string) bool {
	switch status {
	case "completed", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func projectTaskCancellationAllowed(status string) bool {
	return status == "blocked" || status == "planned" || status == "pending"
}

func failureHoldCurrentStatuses() []string {
	return []string{"planned", "pending", "assigned", "running", "waiting_human"}
}

func unresolvedBlockersByDependent(readiness []project.ProjectTaskDependencyReadiness) map[uuid.UUID]struct{} {
	blocked := map[uuid.UUID]struct{}{}
	for _, row := range readiness {
		blocked[row.DependentTaskID] = struct{}{}
	}
	return blocked
}

func (s *ProjectStore) RequestPlanRevisionReview(ctx context.Context, input RequestPlanRevisionReviewInput) (DecisionRequestResult, error) {
	if s.repository == nil || s.approvals == nil {
		return DecisionRequestResult{}, ErrActivityStoreRequired
	}
	projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return DecisionRequestResult{}, err
	}
	targetUserID, err := s.planReviewTargetUserID(ctx, input, projectRecord)
	if err != nil {
		return DecisionRequestResult{}, err
	}
	approvalRequest, err := s.approvals.CreateRequest(ctx, approval.CreateRequestInput{
		TenantID:       input.TenantID,
		ResourceType:   "project_plan_revision",
		ResourceID:     input.PlanRevisionID,
		RequesterType:  "project_coordinator",
		TargetUserID:   targetUserID,
		DecisionType:   "plan_review",
		Title:          "确认项目计划版本",
		Summary:        input.Payload.Summary,
		RiskLevel:      input.Payload.RiskAssessment.HighestRiskLevel,
		Options:        []any{"approved", "rejected", "request_changes", "cancelled"},
		ContextPayload: planRevisionReviewContext(input),
	})
	if err != nil {
		return DecisionRequestResult{}, err
	}
	event, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventDecisionRequested, input.CoordinationJobID.String(), "计划版本需要人类确认", map[string]any{
		"approval_request_id": approvalRequest.ID.String(),
		"plan_revision_id":    input.PlanRevisionID.String(),
		"plan_fingerprint":    input.PlanFingerprint,
		"demand_id":           input.DemandID.String(),
		"target_user_id":      targetUserID.String(),
	}))
	if err != nil {
		return DecisionRequestResult{}, err
	}
	coordinationJobID := input.CoordinationJobID
	planRevisionID := input.PlanRevisionID
	decision, err := s.repository.CreateDecisionRequest(ctx, project.CreateDecisionRequestRequest{
		TenantID:          input.TenantID,
		ProjectID:         input.ProjectID,
		ApprovalRequestID: approvalRequest.ID,
		CoordinationJobID: &coordinationJobID,
		PlanRevisionID:    &planRevisionID,
		TargetUserID:      targetUserID,
		DecisionType:      "plan_review",
		TitleSnapshot:     "确认项目计划版本",
		SummarySnapshot:   input.Payload.Summary,
		RiskLevelSnapshot: nonEmptyString(input.Payload.RiskAssessment.HighestRiskLevel, "medium"),
		StatusSnapshot:    "pending",
		CreatedEventID:    &event.ID,
	})
	if err != nil {
		return DecisionRequestResult{}, err
	}
	if s.inbox != nil {
		if err := s.inbox.UpsertProjectDecisionRequest(ctx, decision); err != nil {
			return DecisionRequestResult{}, err
		}
	}
	return DecisionRequestResult{ID: decision.ID}, nil
}

func (s *ProjectStore) ResolvePlanRevisionReview(ctx context.Context, input ResolvePlanRevisionReviewInput) (PlanRevisionResult, error) {
	if s.repository == nil {
		return PlanRevisionResult{}, ErrActivityStoreRequired
	}
	switch input.Decision {
	case project.PlanReviewDecisionAccept:
		revision, err := s.repository.AcceptPlanRevision(ctx, project.AcceptPlanRevisionRequest{
			TenantID:   input.TenantID,
			ProjectID:  input.ProjectID,
			RevisionID: input.PlanRevisionID,
			AcceptedBy: uuidPtrOrNil(input.ActorUserID),
		})
		if err != nil {
			return PlanRevisionResult{}, err
		}
		return planRevisionResultFromDomain(revision), nil
	case project.PlanReviewDecisionReject, project.PlanReviewDecisionCancel, project.PlanReviewDecisionRequestChanges:
		reason := stringFromMap(input.Payload, "reason")
		revision, err := s.repository.RejectPlanRevision(ctx, project.RejectPlanRevisionRequest{
			TenantID:        input.TenantID,
			ProjectID:       input.ProjectID,
			RevisionID:      input.PlanRevisionID,
			RejectedBy:      uuidPtrOrNil(input.ActorUserID),
			RejectionReason: stringPtr(reason),
		})
		if err != nil {
			return PlanRevisionResult{}, err
		}
		return planRevisionResultFromDomain(revision), nil
	default:
		return PlanRevisionResult{}, project.ErrInvalidProject
	}
}

// IsProjectAcceptanceReady reports whether every demand of the project has reached a
// terminal state, i.e. the project is ready for human acceptance.
func (s *ProjectStore) IsProjectAcceptanceReady(ctx context.Context, input IsProjectAcceptanceReadyInput) (bool, error) {
	if s.repository == nil {
		return false, ErrActivityStoreRequired
	}
	return s.repository.AreAllProjectDemandsTerminal(ctx, input.TenantID, input.ProjectID)
}

// RequestProjectAcceptanceReview moves the project into the acceptance state and opens a
// human-decision item (approval + decision request + inbox) for the human owner. It is
// idempotent: the running→acceptance status transition is the guard — if the project is
// no longer running (already pending/terminal acceptance), it returns a zero result and
// the caller must not record a new pending handle.
func (s *ProjectStore) RequestProjectAcceptanceReview(ctx context.Context, input RequestProjectAcceptanceReviewInput) (DecisionRequestResult, error) {
	if s.repository == nil || s.approvals == nil {
		return DecisionRequestResult{}, ErrActivityStoreRequired
	}
	projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return DecisionRequestResult{}, err
	}
	if err := s.ensureFinalDemandSummariesForAcceptance(ctx, input.TenantID, input.ProjectID); err != nil {
		return DecisionRequestResult{}, err
	}
	if _, err := s.repository.TransitionProjectStatus(ctx, input.TenantID, input.ProjectID, []string{string(project.ProjectStatusRunning)}, string(project.ProjectStatusAcceptance)); err != nil {
		if errors.Is(err, project.ErrProjectNotFound) {
			// Already in acceptance/terminal: a review is already pending or resolved.
			return DecisionRequestResult{}, nil
		}
		return DecisionRequestResult{}, err
	}
	rollbackToRunning := func() {
		_, _ = s.repository.TransitionProjectStatus(ctx, input.TenantID, input.ProjectID, []string{string(project.ProjectStatusAcceptance)}, string(project.ProjectStatusRunning))
	}
	targetUserID := projectRecord.HumanOwnerUserID
	if projectRecord.AcceptanceUserID != nil && *projectRecord.AcceptanceUserID != uuid.Nil {
		targetUserID = *projectRecord.AcceptanceUserID
	}
	approvalRequest, err := s.approvals.CreateRequest(ctx, approval.CreateRequestInput{
		TenantID:      input.TenantID,
		ResourceType:  "project",
		ResourceID:    input.ProjectID,
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
		return DecisionRequestResult{}, err
	}
	event, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventDecisionRequested, input.ProjectID.String(), "项目进入待验收,等待人类确认", map[string]any{
		"approval_request_id": approvalRequest.ID.String(),
		"project_id":          input.ProjectID.String(),
		"target_user_id":      targetUserID.String(),
	}))
	if err != nil {
		rollbackToRunning()
		return DecisionRequestResult{}, err
	}
	decision, err := s.repository.CreateDecisionRequest(ctx, project.CreateDecisionRequestRequest{
		TenantID:          input.TenantID,
		ProjectID:         input.ProjectID,
		ApprovalRequestID: approvalRequest.ID,
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
		return DecisionRequestResult{}, err
	}
	if s.inbox != nil {
		if err := s.inbox.UpsertProjectDecisionRequest(ctx, decision); err != nil {
			return DecisionRequestResult{ID: decision.ID}, nil
		}
	}
	return DecisionRequestResult{ID: decision.ID}, nil
}

func (s *ProjectStore) ensureFinalDemandSummariesForAcceptance(ctx context.Context, tenantID, projectID uuid.UUID) error {
	const demandPageLimit int32 = 100
	for offset := int32(0); ; offset += demandPageLimit {
		demands, err := s.repository.ListProjectDemands(ctx, tenantID, projectID, demandPageLimit, offset)
		if err != nil {
			return err
		}
		for _, demand := range demands {
			if !projectDemandTerminalStatus(demand.Status) {
				continue
			}
			if err := s.ensureFinalDemandSummary(ctx, demand); err != nil {
				return err
			}
		}
		if len(demands) < int(demandPageLimit) {
			return nil
		}
	}
}

func (s *ProjectStore) ensureFinalDemandSummary(ctx context.Context, demand project.ProjectDemand) error {
	idempotencyKey := "final-demand-summary:" + demand.ID.String()
	if latest, err := s.repository.GetLatestProjectDemandSummary(ctx, demand.TenantID, demand.ProjectID, demand.ID); err == nil {
		if latest.IdempotencyKey == idempotencyKey {
			_, err = s.ensureFinalDemandSummaryCreatedEvent(ctx, demand, latest, idempotencyKey)
			return err
		}
		return nil
	} else if !errors.Is(err, project.ErrProjectNotFound) {
		return err
	}
	tasks, err := s.listDemandSummaryTasks(ctx, demand.TenantID, demand.ProjectID, demand.ID)
	if err != nil {
		return err
	}
	taskFacts := make([]demandSummaryTaskFact, 0, len(tasks))
	for _, task := range tasks {
		latestResult, err := s.latestTaskResult(ctx, task)
		if err != nil {
			return err
		}
		taskFacts = append(taskFacts, demandSummaryTaskFact{Task: task, LatestResult: latestResult})
	}
	payload, conclusion := buildFinalDemandSummaryPayload(demand, taskFacts)
	summary, err := s.repository.CreateProjectDemandSummary(ctx, project.CreateProjectDemandSummaryRequest{
		TenantID:           demand.TenantID,
		ProjectID:          demand.ProjectID,
		DemandID:           demand.ID,
		Status:             string(demand.Status),
		Conclusion:         conclusion,
		SummaryPayload:     payload,
		AcceptanceRequired: true,
		IdempotencyKey:     idempotencyKey,
	})
	if err != nil {
		return err
	}
	_, err = s.ensureFinalDemandSummaryCreatedEvent(ctx, demand, summary, idempotencyKey)
	return err
}

func (s *ProjectStore) ensureFinalDemandSummaryCreatedEvent(ctx context.Context, demand project.ProjectDemand, summary project.ProjectDemandSummary, idempotencyKey string) (project.ProjectEvent, error) {
	return s.ensureCoordinatorProjectEvent(ctx, demand.TenantID, demand.ProjectID, project.ProjectEventDemandSummaryCreated, "demand_summary:"+demand.ID.String(), "需求最终总结已生成", map[string]any{
		"demand_id":       demand.ID.String(),
		"demand_status":   string(demand.Status),
		"summary_id":      summary.ID.String(),
		"idempotency_key": idempotencyKey,
	})
}

func (s *ProjectStore) listDemandSummaryTasks(ctx context.Context, tenantID, projectID, demandID uuid.UUID) ([]project.ProjectTask, error) {
	const pageLimit int32 = 100
	tasks := make([]project.ProjectTask, 0)
	for offset := int32(0); ; offset += pageLimit {
		page, err := s.repository.ListProjectTasks(ctx, tenantID, projectID, nil, pageLimit, offset)
		if err != nil {
			return nil, err
		}
		for _, task := range page {
			if task.DemandID != nil && *task.DemandID == demandID {
				tasks = append(tasks, task)
			}
		}
		if len(page) < int(pageLimit) {
			return tasks, nil
		}
	}
}

func (s *ProjectStore) latestTaskResult(ctx context.Context, task project.ProjectTask) (*project.ProjectTaskResult, error) {
	if task.LatestTaskResultID == nil || *task.LatestTaskResultID == uuid.Nil {
		return nil, nil
	}
	const pageLimit int32 = 100
	for offset := int32(0); ; offset += pageLimit {
		results, err := s.repository.ListProjectTaskResults(ctx, project.ListProjectTaskResultsRequest{
			TenantID:      task.TenantID,
			ProjectID:     task.ProjectID,
			ProjectTaskID: task.ID,
			Limit:         pageLimit,
			Offset:        offset,
		})
		if err != nil {
			return nil, err
		}
		for _, result := range results {
			if result.ID == *task.LatestTaskResultID {
				copyResult := result
				return &copyResult, nil
			}
		}
		if len(results) < int(pageLimit) {
			return nil, nil
		}
	}
}

type demandSummaryTaskFact struct {
	Task         project.ProjectTask
	LatestResult *project.ProjectTaskResult
}

func buildFinalDemandSummaryPayload(demand project.ProjectDemand, taskFacts []demandSummaryTaskFact) (map[string]any, string) {
	taskStatuses := make([]map[string]any, 0, len(taskFacts))
	completedTasks := make([]map[string]any, 0)
	unfinishedTasks := make([]map[string]any, 0)
	evidenceRefs := make([]map[string]any, 0)
	artifactRefs := make([]map[string]any, 0)
	humanDecisionRefs := make([]map[string]any, 0)
	validationResults := make([]map[string]any, 0)
	changes := make([]map[string]any, 0)
	actualVerification := make([]map[string]any, 0)
	remainingRisks := make([]map[string]any, 0)
	suggestedNextSteps := make([]map[string]any, 0)
	seenDecisionRefs := map[string]struct{}{}

	for _, fact := range taskFacts {
		taskPayload := demandSummaryTaskPayload(fact.Task, fact.LatestResult)
		taskStatuses = append(taskStatuses, taskPayload)
		if demandSummaryTaskAccepted(fact) {
			completedTasks = append(completedTasks, taskPayload)
		} else {
			unfinishedTasks = append(unfinishedTasks, taskPayload)
		}
		if fact.Task.WaitingRequestID != nil && *fact.Task.WaitingRequestID != uuid.Nil {
			addDecisionRef(&humanDecisionRefs, seenDecisionRefs, *fact.Task.WaitingRequestID, fact.Task.ID, "task_waiting_request")
		}
		if fact.LatestResult == nil {
			continue
		}
		result := *fact.LatestResult
		if result.DecisionRequestID != nil && *result.DecisionRequestID != uuid.Nil {
			addDecisionRef(&humanDecisionRefs, seenDecisionRefs, *result.DecisionRequestID, fact.Task.ID, "task_result_decision")
		}
		for _, ref := range result.Contract.EvidenceRefs {
			evidenceRefs = append(evidenceRefs, taskResultRefPayload(ref, fact.Task.ID))
		}
		for _, ref := range result.Contract.ArtifactRefs {
			artifactRefs = append(artifactRefs, taskResultRefPayload(ref, fact.Task.ID))
		}
		for _, acceptance := range result.Contract.AcceptanceResults {
			validationResults = append(validationResults, taskResultAcceptancePayload(acceptance, fact.Task.ID))
		}
		for _, change := range result.Contract.ChangesMade {
			changes = append(changes, taskResultChangePayload(change, fact.Task.ID))
		}
		for _, verification := range result.Contract.Verification {
			actualVerification = append(actualVerification, taskResultVerificationPayload(verification, fact.Task.ID))
		}
		for _, risk := range result.Contract.Risks {
			remainingRisks = append(remainingRisks, taskResultRiskPayload(risk, fact.Task.ID))
		}
		for _, followUp := range result.Contract.FollowUpRequests {
			suggestedNextSteps = append(suggestedNextSteps, taskResultFollowUpPayload(followUp, fact.Task.ID))
		}
		if result.Contract.Failure != nil && strings.TrimSpace(result.Contract.Failure.RecoveryRecommendation) != "" {
			suggestedNextSteps = append(suggestedNextSteps, map[string]any{
				"task_id": fact.Task.ID.String(),
				"type":    "failure_recovery",
				"summary": result.Contract.Failure.RecoveryRecommendation,
			})
		}
	}

	payload := map[string]any{
		"demand_id":            demand.ID.String(),
		"original_goal":        demand.Title,
		"status":               string(demand.Status),
		"conclusion":           demandSummaryConclusion(demand.Status, len(completedTasks), len(unfinishedTasks)),
		"task_statuses":        taskStatuses,
		"completed_tasks":      completedTasks,
		"unfinished_tasks":     unfinishedTasks,
		"evidence_refs":        evidenceRefs,
		"artifact_refs":        artifactRefs,
		"human_decision_refs":  humanDecisionRefs,
		"validation_results":   validationResults,
		"changes":              changes,
		"actual_verification":  actualVerification,
		"remaining_risks":      remainingRisks,
		"suggested_next_steps": suggestedNextSteps,
	}
	if demand.Content != nil {
		payload["original_goal_content"] = *demand.Content
	}
	if len(demand.SourceRefs) > 0 {
		payload["source_refs"] = demand.SourceRefs
	}
	if len(demand.Attachments) > 0 {
		payload["attachments"] = demand.Attachments
	}
	return payload, payload["conclusion"].(string)
}

func projectDemandTerminalStatus(status project.ProjectDemandStatus) bool {
	switch status {
	case project.ProjectDemandStatusCompleted, project.ProjectDemandStatusFailed, project.ProjectDemandStatusCancelled:
		return true
	default:
		return false
	}
}

func demandSummaryTaskAccepted(fact demandSummaryTaskFact) bool {
	return fact.Task.Status == project.ProjectTaskStatusCompleted &&
		fact.LatestResult != nil &&
		project.ProjectTaskResultAcceptedForDependencyUnlock(*fact.LatestResult)
}

func demandSummaryTaskPayload(task project.ProjectTask, latestResult *project.ProjectTaskResult) map[string]any {
	payload := map[string]any{
		"task_id": task.ID.String(),
		"title":   task.Title,
		"status":  task.Status,
	}
	if task.TaskKind != nil {
		payload["task_kind"] = *task.TaskKind
	}
	if task.LatestTaskResultID != nil && *task.LatestTaskResultID != uuid.Nil {
		payload["latest_task_result_id"] = task.LatestTaskResultID.String()
	}
	if latestResult != nil {
		payload["result_status"] = string(latestResult.ResultStatus)
		payload["result_decision"] = string(latestResult.Decision)
		payload["validation_status"] = latestResult.ValidationStatus
		payload["result_summary"] = latestResult.Contract.Summary
	}
	return payload
}

func demandSummaryConclusion(status project.ProjectDemandStatus, completedCount, unfinishedCount int) string {
	return "demand " + string(status) + " with " + strconv.Itoa(completedCount) + " completed task(s), " + strconv.Itoa(unfinishedCount) + " unfinished task(s)"
}

func addDecisionRef(items *[]map[string]any, seen map[string]struct{}, decisionRequestID, taskID uuid.UUID, source string) {
	key := decisionRequestID.String()
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*items = append(*items, map[string]any{
		"decision_request_id": decisionRequestID.String(),
		"task_id":             taskID.String(),
		"source":              source,
	})
}

func taskResultRefPayload(ref project.TaskResultRef, taskID uuid.UUID) map[string]any {
	payload := map[string]any{"task_id": taskID.String()}
	if ref.ID != "" {
		payload["id"] = ref.ID
	}
	if ref.Kind != "" {
		payload["kind"] = ref.Kind
	}
	if ref.Type != "" {
		payload["type"] = ref.Type
	}
	if ref.Ref != "" {
		payload["ref"] = ref.Ref
	}
	if ref.URI != "" {
		payload["uri"] = ref.URI
	}
	if ref.URL != "" {
		payload["url"] = ref.URL
	}
	if ref.Title != "" {
		payload["title"] = ref.Title
	}
	if ref.Summary != "" {
		payload["summary"] = ref.Summary
	}
	if len(ref.Metadata) > 0 {
		payload["metadata"] = ref.Metadata
	}
	return payload
}

func taskResultAcceptancePayload(result project.TaskResultAcceptanceResult, taskID uuid.UUID) map[string]any {
	payload := map[string]any{
		"task_id": taskID.String(),
		"status":  string(result.Status),
	}
	if result.ID != "" {
		payload["id"] = result.ID
	}
	if result.Criterion != "" {
		payload["criterion"] = result.Criterion
	}
	if result.CriterionID != "" {
		payload["criterion_id"] = result.CriterionID
	}
	if result.Name != "" {
		payload["name"] = result.Name
	}
	if result.Summary != "" {
		payload["summary"] = result.Summary
	}
	if len(result.EvidenceRefs) > 0 {
		payload["evidence_refs"] = append([]string(nil), result.EvidenceRefs...)
	}
	if result.HumanAcceptedReason != "" {
		payload["human_accepted_reason"] = result.HumanAcceptedReason
	}
	return payload
}

func taskResultChangePayload(change project.TaskResultChange, taskID uuid.UUID) map[string]any {
	payload := map[string]any{"task_id": taskID.String()}
	if change.Type != "" {
		payload["type"] = change.Type
	}
	if change.Ref != "" {
		payload["ref"] = change.Ref
	}
	if change.Summary != "" {
		payload["summary"] = change.Summary
	}
	if len(change.Files) > 0 {
		payload["files"] = append([]string(nil), change.Files...)
	}
	if len(change.ArtifactRefs) > 0 {
		refs := make([]map[string]any, 0, len(change.ArtifactRefs))
		for _, ref := range change.ArtifactRefs {
			refs = append(refs, taskResultRefPayload(ref, taskID))
		}
		payload["artifact_refs"] = refs
	}
	return payload
}

func taskResultVerificationPayload(verification project.TaskResultVerification, taskID uuid.UUID) map[string]any {
	payload := map[string]any{
		"task_id": taskID.String(),
		"status":  string(verification.Status),
	}
	if verification.Type != "" {
		payload["type"] = verification.Type
	}
	if verification.Ref != "" {
		payload["ref"] = verification.Ref
	}
	if verification.Summary != "" {
		payload["summary"] = verification.Summary
	}
	if verification.Method != "" {
		payload["method"] = verification.Method
	}
	if len(verification.EvidenceRefs) > 0 {
		refs := make([]map[string]any, 0, len(verification.EvidenceRefs))
		for _, ref := range verification.EvidenceRefs {
			refs = append(refs, taskResultRefPayload(ref, taskID))
		}
		payload["evidence_refs"] = refs
	}
	return payload
}

func taskResultRiskPayload(risk project.TaskResultRisk, taskID uuid.UUID) map[string]any {
	payload := map[string]any{"task_id": taskID.String()}
	if risk.Summary != "" {
		payload["summary"] = risk.Summary
	}
	if risk.Description != "" {
		payload["description"] = risk.Description
	}
	if risk.Severity != "" {
		payload["severity"] = risk.Severity
	}
	if risk.Level != "" {
		payload["level"] = risk.Level
	}
	if risk.Mitigation != "" {
		payload["mitigation"] = risk.Mitigation
	}
	if risk.RequiresHumanReview {
		payload["requires_human_review"] = true
	}
	return payload
}

func taskResultFollowUpPayload(followUp project.TaskResultFollowUpRequest, taskID uuid.UUID) map[string]any {
	payload := map[string]any{"task_id": taskID.String()}
	if followUp.Type != "" {
		payload["type"] = followUp.Type
	}
	if followUp.Summary != "" {
		payload["summary"] = followUp.Summary
	}
	if followUp.RequiredBy != "" {
		payload["required_by"] = followUp.RequiredBy
	}
	if len(followUp.MissingInformation) > 0 {
		refs := make([]map[string]any, 0, len(followUp.MissingInformation))
		for _, ref := range followUp.MissingInformation {
			refs = append(refs, taskResultRefPayload(ref, taskID))
		}
		payload["missing_information"] = refs
	}
	return payload
}

// ApplyProjectAcceptanceDecision closes the acceptance loop: accept archives the project
// (and records an accepted acceptance conclusion); reject / needs_more_evidence reopens it
// to running for rework. The decision's conclusion, if provided in the payload, is recorded.
func (s *ProjectStore) ApplyProjectAcceptanceDecision(ctx context.Context, input ApplyProjectAcceptanceDecisionInput) error {
	if s.repository == nil {
		return ErrActivityStoreRequired
	}
	projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return err
	}
	// Human decisions use the platform vocabulary approved/rejected/needs_more_evidence
	// (see validHumanDecision); an approved decision maps to an accepted acceptance record.
	approved := strings.EqualFold(strings.TrimSpace(input.Decision), "approved")
	status := "rejected"
	if approved {
		status = "accepted"
	} else if strings.EqualFold(strings.TrimSpace(input.Decision), "needs_more_evidence") {
		status = "needs_more_evidence"
	}
	conclusion := acceptanceConclusion(input.Payload, status)
	acceptedBy := projectRecord.HumanOwnerUserID
	if approved {
		if _, err := s.repository.ArchiveProject(ctx, input.TenantID, input.ProjectID); err != nil {
			return err
		}
	} else {
		if _, err := s.repository.TransitionProjectStatus(ctx, input.TenantID, input.ProjectID, []string{string(project.ProjectStatusAcceptance)}, string(project.ProjectStatusRunning)); err != nil && !errors.Is(err, project.ErrProjectNotFound) {
			return err
		}
	}
	_, err = s.repository.CreateAcceptanceRecordWithEvent(ctx, project.CreateAcceptanceRecordWithEventRequest{
		Event: coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventAcceptanceSubmitted, input.DecisionRequestID.String(), acceptanceSummary(status), map[string]any{
			"decision_request_id": input.DecisionRequestID.String(),
			"decision":            status,
		}),
		Acceptance: project.CreateAcceptanceRecordRequest{
			TenantID:         input.TenantID,
			ProjectID:        input.ProjectID,
			AcceptedByUserID: acceptedBy,
			Status:           status,
			Conclusion:       conclusion,
		},
	})
	return err
}

func acceptanceConclusion(payload map[string]any, status string) string {
	if payload != nil {
		if text, ok := payload["conclusion"].(string); ok {
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				return trimmed
			}
		}
	}
	switch status {
	case "accepted":
		return "项目交付已通过验收"
	case "needs_more_evidence":
		return "验收未通过,需要补充证据后重新交付"
	default:
		return "验收未通过,项目退回返工"
	}
}

func acceptanceSummary(status string) string {
	switch status {
	case "accepted":
		return "项目验收通过,已归档"
	case "needs_more_evidence":
		return "项目验收需补充证据,已退回"
	default:
		return "项目验收未通过,已退回返工"
	}
}

func (s *ProjectStore) planReviewTargetUserID(ctx context.Context, input RequestPlanRevisionReviewInput, projectRecord project.Project) (uuid.UUID, error) {
	demand, err := s.repository.GetProjectDemand(ctx, input.TenantID, input.DemandID)
	if err != nil {
		return uuid.Nil, err
	}
	if demand.ProjectID != input.ProjectID {
		return uuid.Nil, project.ErrProjectNotFound
	}
	if demand.ReviewerPreference != nil && demand.ReviewerPreference.ReviewerUserID != uuid.Nil {
		return demand.ReviewerPreference.ReviewerUserID, nil
	}
	return projectRecord.HumanOwnerUserID, nil
}

func (s *ProjectStore) AppendProjectEvent(ctx context.Context, input AppendProjectEventInput) (ProjectEventResult, error) {
	if s.repository == nil {
		return ProjectEventResult{}, ErrActivityStoreRequired
	}
	event, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventType(input.EventType), "project_coordinator", input.Summary, map[string]any{}))
	if err != nil {
		return ProjectEventResult{}, err
	}
	return ProjectEventResult{ID: event.ID}, nil
}

func (s *ProjectStore) ensureCoordinatorProjectEvent(ctx context.Context, tenantID, projectID uuid.UUID, eventType project.ProjectEventType, actorID, summary string, payload map[string]any) (project.ProjectEvent, error) {
	existing, err := s.repository.GetProjectEventByTypeAndActor(ctx, tenantID, projectID, eventType, actorID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, project.ErrProjectNotFound) {
		return project.ProjectEvent{}, err
	}
	return s.repository.AppendProjectEvent(ctx, coordinatorEvent(tenantID, projectID, eventType, actorID, summary, payload))
}

func (s *ProjectStore) DispatchProjectTask(ctx context.Context, input DispatchProjectTaskInput) error {
	if s.repository == nil || s.runStarter == nil {
		return ErrActivityStoreRequired
	}
	task, err := s.repository.GetProjectTask(ctx, input.TenantID, input.TaskID)
	if err != nil {
		return err
	}
	if task.ProjectID != input.ProjectID {
		return s.recordDispatchFailure(ctx, input.TenantID, task.ProjectID, task, project.ErrProjectNotFound)
	}
	if projectTaskQueuedWithoutRunBinding(task) {
		return s.resumeQueuedProjectTaskRunStart(ctx, input, task)
	}
	if task.DigitalEmployeeRunID != nil {
		if task.RuntimeTaskID == nil {
			return s.recordDispatchFailure(ctx, input.TenantID, task.ProjectID, task, project.ErrInvalidProject)
		}
		exists, err := s.repository.ProjectTaskEventExists(ctx, input.TenantID, input.ProjectID, project.ProjectEventTaskDispatched, input.TaskID.String())
		if err != nil {
			return err
		}
		if exists {
			return s.advanceDispatchedTaskDemand(ctx, input, task)
		}
		projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
		if err != nil {
			return err
		}
		if _, err = s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventTaskDispatched, input.TaskID.String(), "项目任务已分派", reemittedDispatchedPayload(task, projectRecord))); err != nil {
			return err
		}
		return s.advanceDispatchedTaskDemand(ctx, input, task)
	}
	if !projectTaskDispatchAllowed(task.Status) || task.AssignedDigitalEmployeeID == nil || task.DemandID == nil {
		return s.recordDispatchFailure(ctx, input.TenantID, task.ProjectID, task, project.ErrInvalidProject)
	}
	input.DispatchReason = defaultDispatchReason(input.DispatchReason)
	gate, err := s.RunPreDispatchGate(ctx, input)
	if err != nil {
		return err
	}
	if !gate.AllowRunStart {
		if err := s.recordDispatchBlocked(ctx, input, task, gate); err != nil {
			return err
		}
		switch {
		case gate.Retryable:
			return ErrProjectTaskDispatchRetryLater
		case gate.Terminal:
			return s.recordDispatchFailure(ctx, input.TenantID, input.ProjectID, task, project.ErrInvalidProject)
		default:
			return nil
		}
	}
	projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return err
	}
	demand, err := s.repository.GetProjectDemand(ctx, input.TenantID, *task.DemandID)
	if err != nil {
		return err
	}
	nextAttemptNo := task.AttemptCount + 1
	attemptID := projectTaskDispatchAttemptID(task.ID, nextAttemptNo)
	leaseToken := projectTaskAttemptLeaseToken(task.ID, nextAttemptNo)
	attemptIdempotencyKey := projectTaskAttemptDispatchIdempotencyKey(task.ID, nextAttemptNo)
	handoffContract := projectTaskDispatchHandoffContract(task.HandoffContract)
	workspaceMode := WorkspaceModeForTaskKind(stringPtrValue(task.TaskKind))
	projectGit := projectGitMetadata(projectRecord.RepoBinding)
	baseRef := ""
	if projectRecord.RepoBinding.Status == project.ProjectRepoBindingStatusBound {
		baseRef, _ = projectGit["default_branch"].(string)
	} else {
		workspaceMode = WorkspaceModeNone
	}
	if workspaceMode == WorkspaceModeBranch || workspaceMode == WorkspaceModeDetachedRun {
		handoffContract["requires_runtime_attestation"] = true
	}
	runMetadata := map[string]any{
		"source":                           "project_task_dispatch",
		"actor_type":                       "project_coordinator",
		"project_id":                       input.ProjectID.String(),
		"demand_id":                        demand.ID.String(),
		"project_task_id":                  task.ID.String(),
		"project_task_attempt_id":          attemptID.String(),
		"project_task_lease_token":         leaseToken,
		"execution_context_packet_version": "v1",
		"expected_outputs":                 append([]any(nil), task.ExpectedOutputs...),
		"input_requirements":               cloneAnyMap(task.InputRequirements),
		"handoff_contract":                 handoffContract,
		"workspace_mode":                   workspaceMode,
		"base_ref":                         baseRef,
	}
	if projectGit != nil {
		runMetadata["project_git"] = projectGit
	}
	addDispatchGateMetadata(runMetadata, gate.Gate)
	upstreamResults := s.collectUpstreamResults(ctx, input.TenantID, input.ProjectID, task)
	executionContextPacket := projectTaskDispatchExecutionContextPacket(input.ProjectID, demand.ID, task, attemptID, leaseToken, handoffContract, gate.Gate, upstreamResults)
	queueResult, err := s.repository.QueueProjectTaskWithAttempt(ctx, project.QueueProjectTaskRequest{
		TenantID:                      input.TenantID,
		ProjectID:                     input.ProjectID,
		ProjectTaskID:                 input.TaskID,
		ProjectTaskAttemptID:          &attemptID,
		DigitalEmployeeID:             *task.AssignedDigitalEmployeeID,
		IdempotencyKey:                attemptIdempotencyKey,
		LeaseToken:                    leaseToken,
		ExecutionContextPacket:        cloneAnyMap(executionContextPacket),
		ExecutionContextPacketVersion: "v1",
		DispatchGateResultID:          &gate.Gate.ID,
	})
	if err != nil {
		if errors.Is(err, project.ErrProjectConflict) {
			latest, latestErr := s.repository.GetProjectTask(ctx, input.TenantID, input.TaskID)
			if latestErr != nil {
				return latestErr
			}
			if latest.CurrentAttemptID != nil && latest.DigitalEmployeeRunID != nil && latest.RuntimeTaskID != nil {
				return s.advanceDispatchedTaskDemand(ctx, input, latest)
			}
		}
		return s.recordDispatchFailure(ctx, input.TenantID, input.ProjectID, task, err)
	}
	if queueResult.Attempt.ID != uuid.Nil {
		if _, err := s.repository.LinkPreDispatchGateAttempt(ctx, project.LinkPreDispatchGateAttemptRequest{
			TenantID:      input.TenantID,
			ProjectID:     input.ProjectID,
			ProjectTaskID: input.TaskID,
			GateResultID:  gate.Gate.ID,
			AttemptID:     queueResult.Attempt.ID,
		}); err != nil {
			return err
		}
	}
	run, err := s.runStarter.StartProjectTaskRun(ctx, StartProjectTaskRunRequest{
		TenantID:             input.TenantID,
		ProjectID:            input.ProjectID,
		DemandID:             demand.ID,
		ProjectTaskID:        task.ID,
		ProjectTaskAttemptID: attemptID,
		DigitalEmployeeID:    *task.AssignedDigitalEmployeeID,
		DispatchUserID:       projectRecord.HumanOwnerUserID,
		Objective:            task.Title,
		Prompt:               projectTaskRunPrompt(projectRecord, demand, task, upstreamResults),
		IdempotencyKey:       attemptIdempotencyKey,
		Metadata:             runMetadata,
		WorkspaceMode:        workspaceMode,
		BaseRef:              baseRef,
		ProjectGit:           projectGit,
	})
	if err != nil {
		return s.recordQueuedAttemptDispatchStartFailure(ctx, input, queueResult.Task, queueResult.Attempt, err)
	}
	if strings.TrimSpace(run.ProviderType) != "" {
		runMetadata["provider_type"] = run.ProviderType
	}
	boundExecutionContextPacket := cloneAnyMap(executionContextPacket)
	projectTaskDispatchAttachRun(boundExecutionContextPacket, run)
	_, err = s.repository.BindProjectTaskAttemptRun(ctx, project.BindProjectTaskAttemptRunRequest{
		TenantID:                      input.TenantID,
		ProjectID:                     input.ProjectID,
		ProjectTaskID:                 input.TaskID,
		AttemptID:                     queueResult.Attempt.ID,
		DigitalEmployeeRunID:          run.RunID,
		RuntimeTaskID:                 run.RuntimeTaskID,
		RuntimeNodeID:                 run.RuntimeNodeID,
		ProviderType:                  run.ProviderType,
		ExecutionContextPacket:        boundExecutionContextPacket,
		ExecutionContextPacketVersion: "v1",
	})
	if err != nil {
		if errors.Is(err, project.ErrProjectConflict) {
			latest, latestErr := s.repository.GetProjectTask(ctx, input.TenantID, input.TaskID)
			if latestErr != nil {
				return latestErr
			}
			if projectTaskBoundToAttemptRun(latest, queueResult.Attempt.ID, run) {
				return s.advanceDispatchedTaskDemand(ctx, input, latest)
			}
		}
		return err
	}
	return s.advanceDispatchedTaskDemand(ctx, input, task)
}

func (s *ProjectStore) advanceDispatchedTaskDemand(ctx context.Context, input DispatchProjectTaskInput, task project.ProjectTask) error {
	if task.DemandID == nil {
		return nil
	}
	return s.repository.AdvanceProjectDemandStatus(ctx, input.TenantID, input.ProjectID, *task.DemandID, project.ProjectDemandStatusExecuting)
}

func (s *ProjectStore) resumeQueuedProjectTaskRunStart(ctx context.Context, input DispatchProjectTaskInput, task project.ProjectTask) error {
	if task.AssignedDigitalEmployeeID == nil || task.DemandID == nil || task.CurrentAttemptID == nil {
		return s.recordDispatchFailure(ctx, input.TenantID, input.ProjectID, task, project.ErrInvalidProject)
	}
	attempt, err := s.repository.GetProjectTaskAttempt(ctx, input.TenantID, *task.CurrentAttemptID)
	if err != nil {
		return err
	}
	if attempt.Status != project.ProjectTaskAttemptStatusQueued || attempt.ProjectTaskID != task.ID {
		return s.recordDispatchFailure(ctx, input.TenantID, input.ProjectID, task, project.ErrInvalidProject)
	}
	projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return err
	}
	demand, err := s.repository.GetProjectDemand(ctx, input.TenantID, *task.DemandID)
	if err != nil {
		return err
	}
	workspaceMode := WorkspaceModeForTaskKind(stringPtrValue(task.TaskKind))
	projectGit := projectGitMetadata(projectRecord.RepoBinding)
	baseRef := ""
	if projectRecord.RepoBinding.Status == project.ProjectRepoBindingStatusBound {
		baseRef, _ = projectGit["default_branch"].(string)
	} else {
		workspaceMode = WorkspaceModeNone
	}
	handoffContract := projectTaskDispatchHandoffContract(task.HandoffContract)
	if workspaceMode == WorkspaceModeBranch || workspaceMode == WorkspaceModeDetachedRun {
		handoffContract["requires_runtime_attestation"] = true
	}
	upstreamResults := s.collectUpstreamResults(ctx, input.TenantID, input.ProjectID, task)
	runMetadata := map[string]any{
		"source":                           "project_task_dispatch",
		"actor_type":                       "project_coordinator",
		"project_id":                       input.ProjectID.String(),
		"demand_id":                        demand.ID.String(),
		"project_task_id":                  task.ID.String(),
		"project_task_attempt_id":          attempt.ID.String(),
		"project_task_lease_token":         attempt.LeaseToken,
		"execution_context_packet_version": nonEmptyString(attempt.ExecutionContextPacketVersion, "v1"),
		"expected_outputs":                 append([]any(nil), task.ExpectedOutputs...),
		"input_requirements":               cloneAnyMap(task.InputRequirements),
		"handoff_contract":                 handoffContract,
		"workspace_mode":                   workspaceMode,
		"base_ref":                         baseRef,
	}
	if projectGit != nil {
		runMetadata["project_git"] = projectGit
	}
	run, err := s.runStarter.StartProjectTaskRun(ctx, StartProjectTaskRunRequest{
		TenantID:             input.TenantID,
		ProjectID:            input.ProjectID,
		DemandID:             demand.ID,
		ProjectTaskID:        task.ID,
		ProjectTaskAttemptID: attempt.ID,
		DigitalEmployeeID:    *task.AssignedDigitalEmployeeID,
		DispatchUserID:       projectRecord.HumanOwnerUserID,
		Objective:            task.Title,
		Prompt:               projectTaskRunPrompt(projectRecord, demand, task, upstreamResults),
		IdempotencyKey:       attempt.IdempotencyKey,
		Metadata:             runMetadata,
		WorkspaceMode:        workspaceMode,
		BaseRef:              baseRef,
		ProjectGit:           projectGit,
	})
	if err != nil {
		return s.recordQueuedAttemptDispatchStartFailure(ctx, input, task, attempt, err)
	}
	boundExecutionContextPacket := cloneAnyMap(attempt.ExecutionContextPacket)
	if len(boundExecutionContextPacket) == 0 {
		boundExecutionContextPacket = projectTaskDispatchExecutionContextPacket(input.ProjectID, demand.ID, task, attempt.ID, attempt.LeaseToken, handoffContract, project.PreDispatchGateResult{}, upstreamResults)
	}
	projectTaskDispatchAttachRun(boundExecutionContextPacket, run)
	_, err = s.repository.BindProjectTaskAttemptRun(ctx, project.BindProjectTaskAttemptRunRequest{
		TenantID:                      input.TenantID,
		ProjectID:                     input.ProjectID,
		ProjectTaskID:                 input.TaskID,
		AttemptID:                     attempt.ID,
		DigitalEmployeeRunID:          run.RunID,
		RuntimeTaskID:                 run.RuntimeTaskID,
		RuntimeNodeID:                 run.RuntimeNodeID,
		ProviderType:                  run.ProviderType,
		ExecutionContextPacket:        boundExecutionContextPacket,
		ExecutionContextPacketVersion: nonEmptyString(attempt.ExecutionContextPacketVersion, "v1"),
	})
	if err != nil {
		if errors.Is(err, project.ErrProjectConflict) {
			latest, latestErr := s.repository.GetProjectTask(ctx, input.TenantID, input.TaskID)
			if latestErr != nil {
				return latestErr
			}
			if projectTaskBoundToAttemptRun(latest, attempt.ID, run) {
				return s.advanceDispatchedTaskDemand(ctx, input, latest)
			}
		}
		return err
	}
	return s.advanceDispatchedTaskDemand(ctx, input, task)
}

func (s *ProjectStore) recordDispatchBlocked(ctx context.Context, input DispatchProjectTaskInput, task project.ProjectTask, gate PreDispatchGateDecision) error {
	reasonCode, recommendedAction := dispatchBlockReasonFromGate(gate.Gate)
	demandID := ""
	if task.DemandID != nil {
		demandID = task.DemandID.String()
	}
	payload := map[string]any{
		"project_task_id":         task.ID.String(),
		"demand_id":               demandID,
		"reason_code":             reasonCode,
		"recommended_action":      recommendedAction,
		"dispatch_gate_result_id": gate.Gate.ID.String(),
	}
	_, err := s.repository.AppendProjectEvent(ctx, project.AppendProjectEventRequest{
		TenantID:     input.TenantID,
		ProjectID:    input.ProjectID,
		EventType:    project.ProjectEventTaskDispatchBlocked,
		ActorType:    "project_coordinator",
		ActorID:      task.ID.String(),
		ResourceType: stringPtr("project_task"),
		ResourceID:   stringPtr(task.ID.String()),
		Summary:      "项目任务派发被阻塞",
		Payload:      payload,
	})
	return err
}

func (s *ProjectStore) recordQueuedAttemptDispatchStartFailure(ctx context.Context, input DispatchProjectTaskInput, task project.ProjectTask, attempt project.ProjectTaskAttempt, dispatchErr error) error {
	event, err := s.appendDispatchFailureEvent(ctx, input.TenantID, input.ProjectID, task, dispatchErr)
	if err != nil {
		return err
	}
	if attempt.ID != uuid.Nil {
		failureFamily := project.FailureFamilyRuntimeStartTimeout
		if dispatchErrorRetryable(dispatchErr) {
			failureFamily = project.FailureFamilyDispatchTransient
		}
		_, releaseErr := s.repository.FailQueuedProjectTaskAttemptDispatchStart(ctx, project.FailQueuedProjectTaskAttemptDispatchStartRequest{
			TenantID:               input.TenantID,
			ProjectID:              input.ProjectID,
			ProjectTaskID:          input.TaskID,
			AttemptID:              attempt.ID,
			DigitalEmployeeID:      selectedEmployeeID(task),
			LeaseToken:             attempt.LeaseToken,
			FailureSummary:         dispatchErr.Error(),
			FailureFamily:          failureFamily,
			Retryable:              dispatchErrorRetryable(dispatchErr),
			RestoreTaskStatus:      project.ProjectTaskStatusPlanned,
			ClearCurrentAttempt:    true,
			DispatchFailureEventID: &event.ID,
		})
		if releaseErr != nil {
			return releaseErr
		}
	}
	return &ProjectTaskDispatchError{FailureRecorded: true, Err: dispatchErr}
}

// RecoverTaskDispatchFailure builds a short-lived project service over the
// store's repository and inbox so recovery reuses the service-level decision
// logic without new worker wiring.
func (s *ProjectStore) RecoverTaskDispatchFailure(ctx context.Context, input RecoverTaskDispatchFailureInput) (RecoverTaskDispatchFailureResult, error) {
	if s.repository == nil {
		return RecoverTaskDispatchFailureResult{}, ErrActivityStoreRequired
	}
	service, err := project.NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(s.repository, project.NoopCoordinatorSignalClient{}, nil, s.inbox, nil)
	if err != nil {
		return RecoverTaskDispatchFailureResult{}, err
	}
	result, err := service.RecoverProjectTaskDispatchFailure(ctx, project.RecoverProjectTaskDispatchFailureRequest{
		TenantID:      input.TenantID,
		ProjectID:     input.ProjectID,
		ProjectTaskID: input.ProjectTaskID,
	})
	if err != nil {
		return RecoverTaskDispatchFailureResult{}, err
	}
	return RecoverTaskDispatchFailureResult{Action: result.Action, RetryNotBefore: result.Task.RetryNotBefore}, nil
}

func (s *ProjectStore) recordDispatchFailure(ctx context.Context, tenantID, projectID uuid.UUID, task project.ProjectTask, dispatchErr error) error {
	if _, err := s.appendDispatchFailureEvent(ctx, tenantID, projectID, task, dispatchErr); err != nil {
		return err
	}
	return &ProjectTaskDispatchError{FailureRecorded: true, Err: dispatchErr}
}

func (s *ProjectStore) appendDispatchFailureEvent(ctx context.Context, tenantID, projectID uuid.UUID, task project.ProjectTask, dispatchErr error) (project.ProjectEvent, error) {
	return s.repository.AppendProjectEvent(ctx, coordinatorEvent(tenantID, projectID, project.ProjectEventTaskDispatchFailed, task.ID.String(), "项目任务分派失败", dispatchFailurePayload(task, dispatchErr, dispatchErrorRetryable(dispatchErr))))
}

func projectTaskDispatchAllowed(status string) bool {
	return status == project.ProjectTaskStatusPlanned || status == project.ProjectTaskStatusWaitingHuman
}

func projectTaskQueuedWithoutRunBinding(task project.ProjectTask) bool {
	return task.Status == project.ProjectTaskStatusQueued && task.CurrentAttemptID != nil && task.DigitalEmployeeRunID == nil && task.RuntimeTaskID == nil
}

func projectTaskBoundToRun(task project.ProjectTask, run StartProjectTaskRunResult) bool {
	return task.DigitalEmployeeRunID != nil &&
		task.RuntimeTaskID != nil &&
		*task.DigitalEmployeeRunID == run.RunID &&
		*task.RuntimeTaskID == run.RuntimeTaskID
}

func projectTaskBoundToAttemptRun(task project.ProjectTask, attemptID uuid.UUID, run StartProjectTaskRunResult) bool {
	return task.CurrentAttemptID != nil &&
		*task.CurrentAttemptID == attemptID &&
		projectTaskBoundToRun(task, run)
}

func projectTaskDispatchIdempotencyKey(taskID uuid.UUID) string {
	return "project-task:" + taskID.String()
}

func projectTaskAttemptDispatchIdempotencyKey(taskID uuid.UUID, attemptNo int32) string {
	return "project-task:" + taskID.String() + ":attempt:" + strconv.FormatInt(int64(attemptNo), 10) + ":dispatch"
}

func projectTaskRunPrompt(projectRecord project.Project, demand project.ProjectDemand, task project.ProjectTask, upstreamResults []map[string]any) string {
	content := ""
	if demand.Content != nil {
		content = *demand.Content
	}
	summary := ""
	if task.Summary != nil {
		summary = *task.Summary
	}
	return "项目任务执行请求\n" +
		"项目ID: " + projectRecord.ID.String() + "\n" +
		"需求ID: " + demand.ID.String() + "\n" +
		"ProjectTask ID: " + task.ID.String() + "\n" +
		"需求标题: " + demand.Title + "\n" +
		"需求内容: " + content + "\n" +
		"任务标题: " + task.Title + "\n" +
		"任务摘要: " + summary + "\n" +
		"expected_outputs: " + taskContractJSON(task.ExpectedOutputs) + "\n" +
		"input_requirements: " + taskContractJSON(task.InputRequirements) + "\n" +
		"handoff_contract: " + taskContractJSON(task.HandoffContract) + "\n" +
		"produces: " + taskContractJSON(plannerProducesFromMetadata(task.PlannerMetadata)) + "\n" +
		"upstream_results: " + taskContractJSON(upstreamResults) + "\n" +
		"结果契约要求: 最终答案必须包含一个 ```json 代码块，顶层字段为 result_contract。" +
		"result_contract.status 使用 completed；summary 填写结论；" +
		"acceptance_results 必须逐条覆盖 handoff_contract.acceptance_criteria，status 使用 passed 并带 evidence_refs；" +
		"evidence_refs/verification 用于说明已读取或验证的证据。" +
		"result_contract 必须含 deliverables 数组，逐项覆盖 produces 列出的每个产出名（每项含 name 与 value 或 ref）；produces 为空时可省略。\n" +
		"upstream_results 是你直接上游任务的真实产出，优先复用其中的值与引用，不要重做上游已完成的工作。\n" +
		"请按项目任务要求执行，并直接输出结论、证据、工件引用、不确定性和 result_contract。" +
		"你只需要给出最终答案；Runtime Agent 会在本轮结束后记录该答案。"
}

// Rotate nothing here: the full upstream summary stays available through the
// blocker's persisted result; the dispatch prompt only carries a bounded slice.
const upstreamSummaryLimitBytes = 4096

func truncateUpstreamSummary(summary string) (string, bool) {
	if len(summary) <= upstreamSummaryLimitBytes {
		return summary, false
	}
	end := upstreamSummaryLimitBytes
	for end > 0 && !utf8.RuneStart(summary[end]) {
		end--
	}
	return summary[:end] + "…[truncated]", true
}

// collectUpstreamResults loads the direct blockers' latest results for
// injection into the dispatch request. One dependency edge = one handoff:
// transitive ancestors are deliberately not walked. Injection is additive
// context, so every lookup failure degrades to less context, never to a
// blocked dispatch.
func (s *ProjectStore) collectUpstreamResults(ctx context.Context, tenantID, projectID uuid.UUID, task project.ProjectTask) []map[string]any {
	deps, err := s.repository.ListProjectTaskDependencies(ctx, tenantID, projectID, []uuid.UUID{task.ID})
	if err != nil || len(deps) == 0 {
		return nil
	}
	results := make([]map[string]any, 0, len(deps))
	for _, dep := range deps {
		blocker, err := s.repository.GetProjectTask(ctx, tenantID, dep.BlockerTaskID)
		if err != nil {
			continue
		}
		entry := map[string]any{
			"task_id":    blocker.ID.String(),
			"task_title": blocker.Title,
			"status":     blocker.Status,
		}
		if blocker.AssignedDigitalEmployeeID != nil {
			entry["digital_employee_id"] = blocker.AssignedDigitalEmployeeID.String()
		}
		if result, err := s.latestTaskResult(ctx, blocker); err == nil && result != nil {
			summary, truncated := truncateUpstreamSummary(result.Contract.Summary)
			entry["summary"] = summary
			if truncated {
				entry["summary_truncated"] = true
			}
			if len(result.Contract.Deliverables) > 0 {
				entry["deliverables"] = result.Contract.Deliverables
			}
			if len(result.Contract.EvidenceRefs) > 0 {
				entry["evidence_refs"] = result.Contract.EvidenceRefs
			}
			if len(result.Contract.ArtifactRefs) > 0 {
				entry["artifact_refs"] = result.Contract.ArtifactRefs
			}
		} else {
			entry["result"] = "unavailable"
		}
		results = append(results, entry)
	}
	return results
}

func projectTaskDispatchExecutionContextPacket(projectID, demandID uuid.UUID, task project.ProjectTask, attemptID uuid.UUID, leaseToken string, handoffContract map[string]any, gate project.PreDispatchGateResult, upstreamResults []map[string]any) map[string]any {
	packet := map[string]any{
		"project_id":               projectID.String(),
		"demand_id":                demandID.String(),
		"project_task_id":          task.ID.String(),
		"project_task_attempt_id":  attemptID.String(),
		"project_task_lease_token": leaseToken,
		"objective":                task.Title,
		"expected_outputs":         append([]any(nil), task.ExpectedOutputs...),
		"input_requirements":       cloneAnyMap(task.InputRequirements),
		"handoff_contract":         handoffContract,
	}
	if len(upstreamResults) > 0 {
		packet["upstream_results"] = upstreamResults
	}
	if task.AssignedDigitalEmployeeID != nil {
		packet["digital_employee_id"] = task.AssignedDigitalEmployeeID.String()
	}
	addDispatchGateMetadata(packet, gate)
	return packet
}

func projectTaskDispatchAttachRun(packet map[string]any, run StartProjectTaskRunResult) {
	if packet == nil {
		return
	}
	packet["digital_employee_run_id"] = run.RunID.String()
	packet["runtime_task_id"] = run.RuntimeTaskID.String()
	packet["runtime_node_id"] = run.RuntimeNodeID.String()
	packet["node_id"] = run.NodeID
	packet["provider_type"] = run.ProviderType
}

func taskContractJSON(value any) string {
	if value == nil {
		return "null"
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func dispatchFailurePayload(task project.ProjectTask, err error, retryable bool) map[string]any {
	digitalEmployeeID := ""
	if task.AssignedDigitalEmployeeID != nil {
		digitalEmployeeID = task.AssignedDigitalEmployeeID.String()
	}
	return map[string]any{
		"project_task_id":     task.ID.String(),
		"digital_employee_id": digitalEmployeeID,
		"error":               err.Error(),
		"error_family":        "project_task_dispatch",
		"retryable":           retryable,
		"dispatch_actor_type": "project_coordinator",
	}
}

func projectGitMetadata(binding project.ProjectRepoBinding) map[string]any {
	if binding.Status != project.ProjectRepoBindingStatusBound {
		return nil
	}
	values := map[string]any{
		"url":            strings.TrimSpace(binding.URL),
		"default_branch": strings.TrimSpace(binding.DefaultBranch),
	}
	if binding.GitCredentialRef != nil {
		if credentialRef := strings.TrimSpace(*binding.GitCredentialRef); credentialRef != "" {
			values["git_credential_ref"] = credentialRef
		}
	}
	scope := make([]any, 0, len(binding.Scope))
	for _, item := range binding.Scope {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			scope = append(scope, trimmed)
		}
	}
	if len(scope) > 0 {
		values["scope"] = scope
	}
	return values
}

func addDispatchGateMetadata(target map[string]any, gate project.PreDispatchGateResult) {
	if target == nil {
		return
	}
	target["dispatch_reason"] = gate.DispatchReason
	if gate.ID != uuid.Nil {
		target["dispatch_gate_result_id"] = gate.ID.String()
	}
	if strings.TrimSpace(gate.Status) != "" {
		target["dispatch_gate_status"] = gate.Status
	}
	if strings.TrimSpace(gate.IdempotencyKey) != "" {
		target["dispatch_gate_idempotency_key"] = gate.IdempotencyKey
	}
	if strings.TrimSpace(gate.DispatchToken) != "" {
		target["dispatch_gate_dispatch_token"] = gate.DispatchToken
	}
}

func dispatchErrorRetryable(err error) bool {
	if errors.Is(err, ErrProjectTaskDispatchRetryLater) {
		return true
	}
	switch {
	case errors.Is(err, project.ErrProjectNotFound),
		errors.Is(err, project.ErrInvalidProject),
		errors.Is(err, project.ErrProjectConflict):
		return false
	}
	var startErr *ProjectTaskRunStartError
	if errors.As(err, &startErr) {
		return startErr.Retryable
	}
	return true
}

func reemittedDispatchedPayload(task project.ProjectTask, projectRecord project.Project) map[string]any {
	payload := map[string]any{
		"project_task_id":     task.ID.String(),
		"project_task_status": task.Status,
		"dispatch_actor_type": "project_coordinator",
		"dispatch_user_id":    projectRecord.HumanOwnerUserID.String(),
		"reemitted":           true,
	}
	if task.CurrentAttemptID != nil {
		payload["project_task_attempt_id"] = task.CurrentAttemptID.String()
	}
	if task.AssignedDigitalEmployeeID != nil {
		payload["digital_employee_id"] = task.AssignedDigitalEmployeeID.String()
	}
	if task.DigitalEmployeeRunID != nil {
		payload["digital_employee_run_id"] = task.DigitalEmployeeRunID.String()
	}
	if task.RuntimeTaskID != nil {
		payload["runtime_task_id"] = task.RuntimeTaskID.String()
	}
	return payload
}

func (s *ProjectStore) FinishCoordinationJob(ctx context.Context, input FinishCoordinationJobInput) error {
	if s.repository == nil {
		return ErrActivityStoreRequired
	}
	outputEventIDs := make([]any, 0, len(input.OutputEventIDs))
	for _, id := range input.OutputEventIDs {
		outputEventIDs = append(outputEventIDs, id.String())
	}
	_, err := s.repository.FinishCoordinationJob(ctx, project.FinishCoordinationJobRequest{
		TenantID:       input.TenantID,
		ID:             input.JobID,
		Status:         input.Status,
		OutputEventIDs: outputEventIDs,
	})
	return err
}

func coordinatorEvent(tenantID, projectID uuid.UUID, eventType project.ProjectEventType, actorID, summary string, payload map[string]any) project.AppendProjectEventRequest {
	if actorID == "" {
		actorID = "project_coordinator"
	}
	return project.AppendProjectEventRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: eventType,
		ActorType: "project_coordinator",
		ActorID:   actorID,
		Summary:   summary,
		Payload:   payload,
	}
}

func stringsToAny(values []string) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

type routeDecisionAggregate struct {
	CandidateDigitalEmployeeIDs []uuid.UUID
	SelectedDigitalEmployeeIDs  []uuid.UUID
	ExpectedOutputs             []string
	InputRequirements           map[string]any
}

func aggregateRouteDecisionFields(plan RouteDecisionPlan) routeDecisionAggregate {
	selected := make([]uuid.UUID, 0, len(plan.Tasks))
	seenEmployees := map[uuid.UUID]struct{}{}
	expectedOutputs := make([]string, 0)
	seenOutputs := map[string]struct{}{}
	taskSummaries := make([]any, 0, len(plan.Tasks))
	for _, task := range plan.Tasks {
		if task.SelectedEmployeeID != uuid.Nil {
			if _, seen := seenEmployees[task.SelectedEmployeeID]; !seen {
				seenEmployees[task.SelectedEmployeeID] = struct{}{}
				selected = append(selected, task.SelectedEmployeeID)
			}
		}
		for _, output := range task.ExpectedOutputs {
			output = strings.TrimSpace(output)
			if output == "" {
				continue
			}
			if _, seen := seenOutputs[output]; seen {
				continue
			}
			seenOutputs[output] = struct{}{}
			expectedOutputs = append(expectedOutputs, output)
		}
		taskSummaries = append(taskSummaries, aggregateTaskInputSummary(task))
	}
	return routeDecisionAggregate{
		CandidateDigitalEmployeeIDs: selected,
		SelectedDigitalEmployeeIDs:  selected,
		ExpectedOutputs:             expectedOutputs,
		InputRequirements:           map[string]any{"tasks": taskSummaries},
	}
}

func aggregateTaskInputSummary(task PlannedTask) map[string]any {
	summary := map[string]any{
		"key":                          task.Key,
		"title":                        task.Title,
		"selected_digital_employee_id": task.SelectedEmployeeID.String(),
		"expected_outputs":             stringsToAny(task.ExpectedOutputs),
		"input_requirement_keys":       stringsToAny(sortedMapKeys(task.InputRequirements)),
	}
	if task.TaskKind != "" {
		summary["task_kind"] = task.TaskKind
	}
	if task.StageIndex != nil {
		summary["stage_index"] = *task.StageIndex
	}
	if task.RiskLevel != "" {
		summary["risk_level"] = task.RiskLevel
	}
	if task.RequiresHumanApproval {
		summary["requires_human_approval"] = true
	}
	if len(task.BlockedByKeys) > 0 {
		summary["blocked_by_keys"] = stringsToAny(task.BlockedByKeys)
	}
	if task.EmployeeSelectionReason != "" {
		summary["employee_selection_reason"] = task.EmployeeSelectionReason
	}
	if len(task.RequiredCapabilities) > 0 {
		summary["required_capabilities"] = stringsToAny(task.RequiredCapabilities)
	}
	if len(task.MatchedCapabilities) > 0 {
		summary["matched_capabilities"] = stringsToAny(task.MatchedCapabilities)
	}
	if len(task.MissingCapabilities) > 0 {
		summary["missing_capabilities"] = stringsToAny(task.MissingCapabilities)
	}
	if len(task.PermissionRequirements) > 0 {
		summary["permission_requirements"] = stringsToAny(task.PermissionRequirements)
	}
	if len(task.ToolRequirements) > 0 {
		summary["tool_requirements"] = stringsToAny(task.ToolRequirements)
	}
	if len(task.RuntimeRequirements) > 0 {
		summary["runtime_requirements"] = stringsToAny(task.RuntimeRequirements)
	}
	if len(task.VerificationRequirements) > 0 {
		summary["verification_requirements"] = stringsToAny(task.VerificationRequirements)
	}
	if task.SelectionScore > 0 {
		summary["selection_score"] = task.SelectionScore
	}
	if task.PlanningProfileSnapshotHash != "" {
		summary["profile_snapshot_hash"] = task.PlanningProfileSnapshotHash
	}
	return summary
}

func sortedMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func uuidStrings(values []uuid.UUID) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value.String())
	}
	return result
}

func planRevisionReviewContext(input RequestPlanRevisionReviewInput) map[string]any {
	return map[string]any{
		"project_id":               input.ProjectID.String(),
		"demand_id":                input.DemandID.String(),
		"coordination_job_id":      input.CoordinationJobID.String(),
		"plan_revision_id":         input.PlanRevisionID.String(),
		"plan_fingerprint":         input.PlanFingerprint,
		"created_event_id":         input.CreatedEventID.String(),
		"tasks":                    input.Payload.Tasks,
		"plan_acceptance_criteria": input.Payload.PlanAcceptanceCriteria,
		"risk_assessment":          input.Payload.RiskAssessment,
		"human_review":             input.Payload.HumanReview,
	}
}

func planRevisionPayloadMap(payload PlanRevisionPayload) (map[string]any, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if value == nil {
		value = map[string]any{}
	}
	return value, nil
}

func planRevisionPayloadFromMap(value map[string]any) (PlanRevisionPayload, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return PlanRevisionPayload{}, err
	}
	var payload PlanRevisionPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return PlanRevisionPayload{}, err
	}
	return payload, nil
}

func planRevisionResultFromDomain(revision project.PlanRevision) PlanRevisionResult {
	return PlanRevisionResult{
		ID:              revision.ID,
		Status:          revision.Status,
		RevisionNumber:  revision.RevisionNumber,
		PlanFingerprint: revision.PlanFingerprint,
		ReviewRequired:  revision.ReviewRequired,
	}
}

func uuidPtrOrNil(value uuid.UUID) *uuid.UUID {
	if value == uuid.Nil {
		return nil
	}
	return &value
}

func uuidValue(value *uuid.UUID) uuid.UUID {
	if value == nil {
		return uuid.Nil
	}
	return *value
}

func stringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func nonEmptyString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func recoveryReplacementTaskKey(source project.ProjectTask) string {
	base := source.ID.String()
	if source.PlannedTaskKey != nil && strings.TrimSpace(*source.PlannedTaskKey) != "" {
		base = strings.TrimSpace(*source.PlannedTaskKey)
	}
	if strings.HasSuffix(base, "#1") {
		base = strings.TrimSuffix(base, "#1")
	}
	key := base + "#2"
	if len(key) <= 100 {
		return key
	}
	return source.ID.String()[:8] + "#2"
}

func recoveryReplacementTitle(title string) string {
	if strings.Contains(title, "重试") {
		return title
	}
	return title + "（重试）"
}

func recoveryPlannerMetadata(source project.ProjectTask, decisionRequestID uuid.UUID, action FailureRecoveryAction) map[string]any {
	metadata := cloneAnyMap(source.PlannerMetadata)
	metadata["source_task_id"] = source.ID.String()
	metadata["decision_request_id"] = decisionRequestID.String()
	metadata["recovery_action"] = action.Action
	if source.CoordinationJobID != nil {
		metadata["parent_coordination_job_id"] = source.CoordinationJobID.String()
	}
	if action.NewDigitalEmployeeID != nil {
		metadata["new_digital_employee_id"] = action.NewDigitalEmployeeID.String()
	}
	return metadata
}

func revisionInputRequirements(source project.ProjectTask, result project.ProjectTaskResult) map[string]any {
	requirements := cloneAnyMap(source.InputRequirements)
	revision := result.Contract.RevisionRequest
	requirements["revision_reason"] = revision.Reason
	requirements["requested_changes"] = append([]string(nil), revision.RequestedChanges...)
	requirements["source_task_id"] = source.ID.String()
	requirements["source_result_id"] = result.ID.String()
	if result.Contract.Summary != "" {
		requirements["source_result_summary"] = result.Contract.Summary
	}
	return requirements
}

func revisionPlannerMetadata(source project.ProjectTask, result project.ProjectTaskResult) map[string]any {
	metadata := cloneAnyMap(source.PlannerMetadata)
	metadata["revision_root_task_id"] = revisionRootTaskID(source)
	metadata["revision_attempt_count"] = revisionAttemptCount(source) + 1
	metadata["revision_max_attempts"] = revisionMaxAttempts(source)
	metadata["source_task_id"] = source.ID.String()
	metadata["source_result_id"] = result.ID.String()
	return metadata
}

func revisionPlannerMetadataForSupplement(owner project.ProjectTask, sourceTaskID uuid.UUID, missingInputs []string) map[string]any {
	metadata := cloneAnyMap(owner.PlannerMetadata)
	metadata["supplement_for"] = sourceTaskID.String()
	metadata["missing_inputs"] = append([]string(nil), missingInputs...)
	return metadata
}

func plannerProducesFromMetadata(metadata map[string]any) []string {
	switch values := metadata["produces"].(type) {
	case []any:
		produces := make([]string, 0, len(values))
		for _, value := range values {
			if produced, ok := value.(string); ok && strings.TrimSpace(produced) != "" {
				produces = append(produces, strings.TrimSpace(produced))
			}
		}
		return produces
	case []string:
		produces := make([]string, 0, len(values))
		for _, produced := range values {
			if strings.TrimSpace(produced) != "" {
				produces = append(produces, strings.TrimSpace(produced))
			}
		}
		return produces
	default:
		return nil
	}
}

func revisionTaskTitle(source project.ProjectTask, result project.ProjectTaskResult) string {
	if result.Contract.RevisionRequest != nil && strings.TrimSpace(result.Contract.RevisionRequest.RecommendedTaskTitle) != "" {
		return strings.TrimSpace(result.Contract.RevisionRequest.RecommendedTaskTitle)
	}
	if strings.Contains(source.Title, "修订") {
		return source.Title
	}
	return source.Title + "（修订）"
}

func revisionTaskSummary(source project.ProjectTask, result project.ProjectTaskResult) string {
	if result.Contract.RevisionRequest != nil && strings.TrimSpace(result.Contract.RevisionRequest.RecommendedTaskSummary) != "" {
		return strings.TrimSpace(result.Contract.RevisionRequest.RecommendedTaskSummary)
	}
	if strings.TrimSpace(result.Contract.Summary) != "" {
		return strings.TrimSpace(result.Contract.Summary)
	}
	return stringPtrValue(source.Summary)
}

func revisionTaskKey(source project.ProjectTask, result project.ProjectTaskResult) *string {
	base := source.ID.String()
	if value, ok := source.PlannerMetadata["iteration_key"].(string); ok && strings.TrimSpace(value) != "" {
		base = strings.TrimSpace(value)
	} else if source.PlannedTaskKey != nil && strings.TrimSpace(*source.PlannedTaskKey) != "" {
		base = strings.TrimSpace(*source.PlannedTaskKey)
	}
	key := base + "#revision-" + result.ID.String()[:8]
	if len(key) > 100 {
		key = source.ID.String()[:8] + "#revision-" + result.ID.String()[:8]
	}
	return &key
}

// upstreamSupplementTaskKey derives a planned_task_key for a supplement task
// that is distinct from the owner's own key. The owner keeps its original key
// within the same coordination job, so copying it verbatim collides with
// uq_project_tasks_coordination_planned_key; each graph-extension round gets
// its own suffix instead.
func upstreamSupplementTaskKey(owner project.ProjectTask, planIteration int32) *string {
	base := owner.ID.String()
	if owner.PlannedTaskKey != nil && strings.TrimSpace(*owner.PlannedTaskKey) != "" {
		base = strings.TrimSpace(*owner.PlannedTaskKey)
	}
	key := base + "#upstream-supplement-" + strconv.Itoa(int(planIteration))
	if len(key) > 100 {
		key = owner.ID.String()[:8] + "#upstream-supplement-" + strconv.Itoa(int(planIteration))
	}
	return &key
}

func revisionBudgetExhausted(task project.ProjectTask) bool {
	return revisionAttemptCount(task) >= revisionMaxAttempts(task)
}

// maxPlanIterations reads projects.coordination_policy.max_plan_iterations,
// falling back to defaultMaxPlanIterations when absent or not a positive
// number. int32FromAny covers both decoded-JSON numeric shapes (float64,
// json.Number) and plain Go numeric literals used in tests.
func maxPlanIterations(policy map[string]any) int {
	if value, ok := int32FromAny(policy["max_plan_iterations"]); ok && value > 0 {
		return int(value)
	}
	return defaultMaxPlanIterations
}

// currentPlanIteration is the highest graph extension round observed among
// siblings created by the same coordination job (0 for the original plan).
func currentPlanIteration(siblings []project.ProjectTask) int32 {
	var max int32
	for _, task := range siblings {
		if task.PlanIteration > max {
			max = task.PlanIteration
		}
	}
	return max
}

func revisionRootTaskID(task project.ProjectTask) string {
	if value, ok := task.PlannerMetadata["revision_root_task_id"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if task.RevisionOfTaskID != nil && *task.RevisionOfTaskID != uuid.Nil {
		return task.RevisionOfTaskID.String()
	}
	return task.ID.String()
}

func revisionAttemptCount(task project.ProjectTask) int32 {
	if value, ok := int32FromAny(task.PlannerMetadata["revision_attempt_count"]); ok && value > 0 {
		return value
	}
	if task.AttemptCount > 0 {
		return task.AttemptCount
	}
	return 1
}

func revisionMaxAttempts(task project.ProjectTask) int32 {
	if value, ok := int32FromAny(task.PlannerMetadata["revision_max_attempts"]); ok && value > 0 {
		return value
	}
	if task.MaxAttempts != nil && *task.MaxAttempts > 0 {
		return *task.MaxAttempts
	}
	return defaultRevisionMaxAttempts
}

func int32FromAny(value any) (int32, bool) {
	switch typed := value.(type) {
	case int:
		return int32(typed), true
	case int32:
		return typed, true
	case int64:
		return int32(typed), true
	case float64:
		return int32(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int32(parsed), true
	default:
		return 0, false
	}
}

func (s *ProjectStore) revisionBudgetExhausted(ctx context.Context, tenantID, projectID uuid.UUID, source project.ProjectTask) bool {
	maxAttempts := revisionMaxAttempts(source)
	maxCount := revisionAttemptCount(source)
	iterationKey, _ := source.PlannerMetadata["iteration_key"].(string)
	for _, task := range s.priorRevisionTasks(ctx, tenantID, projectID, source, iterationKey) {
		if count := revisionAttemptCount(task); count > maxCount {
			maxCount = count
		}
	}
	return maxCount >= maxAttempts
}

func (s *ProjectStore) priorRevisionTasks(ctx context.Context, tenantID, projectID uuid.UUID, source project.ProjectTask, iterationKey string) []project.ProjectTask {
	if source.CoordinationJobID == nil {
		return nil
	}
	tasks, err := s.repository.ListProjectTasksByCoordinationJob(ctx, tenantID, projectID, *source.CoordinationJobID)
	if err != nil {
		return nil
	}
	rootTaskID := revisionRootTaskID(source)
	revisions := make([]project.ProjectTask, 0)
	for _, task := range tasks {
		if task.ID == source.ID {
			continue
		}
		sameSource := task.RevisionOfTaskID != nil && *task.RevisionOfTaskID == source.ID
		sameRoot := rootTaskID != "" && revisionRootTaskID(task) == rootTaskID
		sameIteration := iterationKey != "" && task.PlannerMetadata["iteration_key"] == iterationKey
		if sameSource || sameRoot || sameIteration {
			revisions = append(revisions, task)
		}
	}
	return revisions
}

func (s *ProjectStore) taskResultByID(ctx context.Context, tenantID, projectID, taskID, resultID uuid.UUID) (project.ProjectTaskResult, error) {
	results, err := s.repository.ListProjectTaskResults(ctx, project.ListProjectTaskResultsRequest{
		TenantID:      tenantID,
		ProjectID:     projectID,
		ProjectTaskID: taskID,
		Limit:         100,
	})
	if err != nil {
		return project.ProjectTaskResult{}, err
	}
	for _, result := range results {
		if result.ID == resultID {
			return result, nil
		}
	}
	return project.ProjectTaskResult{}, project.ErrProjectNotFound
}

func dependencyExists(dependencies []project.ProjectTaskDependency, dependentTaskID, blockerTaskID uuid.UUID) bool {
	for _, dependency := range dependencies {
		if dependency.DependentTaskID == dependentTaskID && dependency.BlockerTaskID == blockerTaskID {
			return true
		}
	}
	return false
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func cloneAnyMap(values map[string]any) map[string]any {
	if values == nil {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func projectTaskDispatchAttemptID(projectTaskID uuid.UUID, attemptNo int32) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("superteam:project-task-attempt:"+projectTaskID.String()+":"+strconv.FormatInt(int64(attemptNo), 10)))
}

func projectTaskAttemptLeaseToken(projectTaskID uuid.UUID, attemptNo int32) string {
	return "project-task-" + projectTaskID.String() + "-attempt-" + strconv.FormatInt(int64(attemptNo), 10)
}

// projectTaskDispatchHandoffContract clones a task's handoff contract and forces
// completion_path to "project_task_attempt_writeback". The runtime agent gates
// project-task completion writeback on this exact value; the planner does not
// always emit it, so the control-plane enforces it at dispatch.
func projectTaskDispatchHandoffContract(contract map[string]any) map[string]any {
	cloned := cloneAnyMap(contract)
	cloned["completion_path"] = "project_task_attempt_writeback"
	return cloned
}

func failureRecoveryContext(input HoldDownstreamForFailureInput, downstreamTaskIDs []uuid.UUID) map[string]any {
	return map[string]any{
		"project_id":          input.ProjectID.String(),
		"failed_task_id":      input.FailedTaskID.String(),
		"failed_event_id":     input.FailedEventID.String(),
		"failure_summary":     input.FailureSummary,
		"downstream_task_ids": uuidStrings(downstreamTaskIDs),
	}
}

func iterationExhaustedContext(input RequestProjectTaskIterationExhaustedReviewInput, reason, summary string, downstreamTaskIDs []uuid.UUID) map[string]any {
	return map[string]any{
		"project_id":          input.ProjectID.String(),
		"project_task_id":     input.ProjectTaskID.String(),
		"result_id":           input.ResultID.String(),
		"completed_event_id":  input.CreatedEventID.String(),
		"reason":              reason,
		"summary":             summary,
		"downstream_task_ids": uuidStrings(downstreamTaskIDs),
	}
}

func isRoutableDigitalProjectRole(role project.ProjectRole) bool {
	return role == project.ProjectRoleExecutor || role == project.ProjectRoleReviewer
}
