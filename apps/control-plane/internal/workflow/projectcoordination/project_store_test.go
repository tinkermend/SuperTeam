package projectcoordination

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/superteam/control-plane/internal/approval"
	"github.com/superteam/control-plane/internal/project"
)

func TestPlannedTaskInputRequirementsKeepOnlyRequiredInputs(t *testing.T) {
	planned := PlannedTask{
		Key:      "b",
		Produces: []string{"summary"},
		InputRequirements: map[string]any{
			"required_inputs": []any{"load_test_report"},
			"repository":      "superteam",
			"scope":           "one host",
		},
	}

	stored, notes := plannedTaskInputRequirements(planned)

	require.Equal(t, map[string]any{"required_inputs": []any{"load_test_report"}}, stored)
	require.Equal(t, map[string]any{"repository": "superteam", "scope": "one host"}, notes)
}

func TestProjectStoreSnapshotIncludesOnlyActiveDigitalExecutorsAndReviewers(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	executorID := uuid.New()
	reviewerID := uuid.New()
	observerID := uuid.New()
	inactiveExecutorID := uuid.New()
	humanID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{
			ID:                 projectID,
			TenantID:           tenantID,
			CoordinationPolicy: map[string]any{"mode": "balanced"},
		},
		demand: project.ProjectDemand{
			ID:        demandID,
			TenantID:  tenantID,
			ProjectID: projectID,
			Title:     "补齐验收证据",
			Content:   strPtr("整理日志并给出结论"),
		},
		members: []project.ProjectMember{
			{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, PrincipalType: project.PrincipalTypeDigitalEmployee, PrincipalID: executorID, ProjectRole: project.ProjectRoleExecutor, Status: "active", DisplayNameSnapshot: strPtr("执行员工")},
			{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, PrincipalType: project.PrincipalTypeDigitalEmployee, PrincipalID: reviewerID, ProjectRole: project.ProjectRoleReviewer, Status: "active", DisplayNameSnapshot: strPtr("复核员工")},
			{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, PrincipalType: project.PrincipalTypeDigitalEmployee, PrincipalID: observerID, ProjectRole: project.ProjectRoleObserver, Status: "active"},
			{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, PrincipalType: project.PrincipalTypeDigitalEmployee, PrincipalID: inactiveExecutorID, ProjectRole: project.ProjectRoleExecutor, Status: "inactive"},
			{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, PrincipalType: project.PrincipalTypeHumanUser, PrincipalID: humanID, ProjectRole: project.ProjectRoleOwner, Status: "active"},
		},
	}
	store := NewProjectStore(repo)

	snapshot, err := store.LoadProjectCoordinationSnapshot(context.Background(), LoadSnapshotInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		DemandID:  demandID,
	})
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}

	if len(snapshot.DigitalEmployeePool) != 2 {
		t.Fatalf("expected executor and reviewer only, got %#v", snapshot.DigitalEmployeePool)
	}
	if snapshot.DigitalEmployeePool[0].PrincipalID != executorID || snapshot.DigitalEmployeePool[1].PrincipalID != reviewerID {
		t.Fatalf("unexpected employee pool: %#v", snapshot.DigitalEmployeePool)
	}
}

func TestLoadProjectCoordinationSnapshotRecordsBlockedEventWhenNoPlannableDigitalEmployee(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	observerID := uuid.New()
	humanID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "无法规划"},
		members: []project.ProjectMember{
			{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, PrincipalType: project.PrincipalTypeDigitalEmployee, PrincipalID: observerID, ProjectRole: project.ProjectRoleObserver, Status: "active"},
			{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, PrincipalType: project.PrincipalTypeHumanUser, PrincipalID: humanID, ProjectRole: project.ProjectRoleOwner, Status: "active"},
		},
	}
	store := NewProjectStore(repo)

	snapshot, err := store.LoadProjectCoordinationSnapshot(context.Background(), LoadSnapshotInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		DemandID:  demandID,
	})

	require.NoError(t, err)
	require.Empty(t, snapshot.DigitalEmployeePool)
	blockedEvents := eventsByType(repo.events, project.ProjectEventCoordinationBlocked)
	require.Len(t, blockedEvents, 1)
	require.Equal(t, "no_plannable_digital_employee", blockedEvents[0].Payload["reason_code"])
	require.Equal(t, demandID.String(), blockedEvents[0].Payload["demand_id"])
}

func TestProjectStoreSnapshotUsesProjectRuntimeReadinessForExecutorPool(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	employeeID := uuid.New()
	runtimeNodeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "验证项目维度运行"},
		members: []project.ProjectMember{{
			ID:            uuid.New(),
			TenantID:      tenantID,
			ProjectID:     projectID,
			PrincipalType: project.PrincipalTypeDigitalEmployee,
			PrincipalID:   employeeID,
			ProjectRole:   project.ProjectRoleExecutor,
			Status:        "active",
		}},
	}
	reader := projectStoreGateRuntimeReader{
		employee: project.PreDispatchEmployeeSnapshot{
			ID:                 employeeID,
			Status:             "ready",
			PolicyAllowed:      true,
			AvailableLoadSlots: 1,
			RequiredLoadSlots:  1,
		},
		runtime: project.PreDispatchRuntimeSnapshot{
			NodeOnline:              true,
			ProviderAvailable:       true,
			WorkspaceReady:          true,
			SlotAvailable:           true,
			ContractVersionAccepted: true,
		},
	}
	source := &fakePlanningProfileSource{records: map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord{
		employeeID: {
			DigitalEmployeeID: employeeID,
			EmployeeType:      "implementation",
			EmployeeStatus:    "ready",
			CapabilityBindings: map[string]any{
				"external_capabilities": []any{"implementation"},
			},
			RuntimeNodeID:   runtimeNodeID,
			ProviderType:    "codex",
			ExecutionStatus: "ready",
		},
	}}
	store := NewProjectStore(repo).
		WithDigitalEmployeeReadiness(fakeLegacyReadiness{ready: map[uuid.UUID]bool{employeeID: false}}).
		WithDigitalEmployeePlanningProfiles(source).
		WithPreDispatchGateReaders(reader, nil)

	snapshot, err := store.LoadProjectCoordinationSnapshot(context.Background(), LoadSnapshotInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		DemandID:  demandID,
	})

	require.NoError(t, err)
	require.Equal(t, projectID, source.projectID)
	require.Len(t, snapshot.DigitalEmployeePool, 1)
	require.Equal(t, employeeID, snapshot.DigitalEmployeePool[0].PrincipalID)
	require.NotNil(t, snapshot.DigitalEmployeePool[0].PlanningProfile)
	profile := snapshot.DigitalEmployeePool[0].PlanningProfile
	require.Equal(t, "ready", profile.RuntimeRequirements.ProviderStatus)
	require.Equal(t, runtimeNodeID.String(), profile.RuntimeRequirements.RuntimeNodeID)
	require.Equal(t, []string{"codex"}, profile.RuntimeRequirements.ProviderTypes)
	score := ScorePlanningProfile(*profile, PlanningTaskRequirements{RequiredCapabilities: []string{"implementation"}})
	require.Empty(t, score.HardFailures)
}

func TestLoadProjectCoordinationSnapshotKeepsPlannableEmployeesWhenRuntimeNotReady(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	employeeID := uuid.New()
	runtimeNodeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "规划运行准备度"},
		members: []project.ProjectMember{{
			ID:                  uuid.New(),
			TenantID:            tenantID,
			ProjectID:           projectID,
			PrincipalType:       project.PrincipalTypeDigitalEmployee,
			PrincipalID:         employeeID,
			ProjectRole:         project.ProjectRoleExecutor,
			Status:              "active",
			DisplayNameSnapshot: strPtr("执行员工"),
		}},
	}
	reader := projectStoreGateRuntimeReader{
		employee: project.PreDispatchEmployeeSnapshot{
			ID:                 employeeID,
			Status:             "ready",
			PolicyAllowed:      true,
			AvailableLoadSlots: 1,
			RequiredLoadSlots:  1,
		},
		runtime: project.PreDispatchRuntimeSnapshot{
			NodeOnline:              false,
			ProviderAvailable:       true,
			WorkspaceReady:          false,
			SlotAvailable:           true,
			ContractVersionAccepted: true,
		},
	}
	source := &fakePlanningProfileSource{records: map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord{
		employeeID: {
			DigitalEmployeeID: employeeID,
			EmployeeType:      "implementation",
			EmployeeStatus:    "ready",
			CapabilityBindings: map[string]any{
				"external_capabilities": []any{"implementation"},
			},
			RuntimeNodeID:   runtimeNodeID,
			ProviderType:    "codex",
			ExecutionStatus: "ready",
		},
	}}
	store := NewProjectStore(repo).
		WithDigitalEmployeePlanningProfiles(source).
		WithPreDispatchGateReaders(reader, nil)

	snapshot, err := store.LoadProjectCoordinationSnapshot(context.Background(), LoadSnapshotInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		DemandID:  demandID,
	})

	require.NoError(t, err)
	require.Len(t, snapshot.DigitalEmployeePool, 1)
	require.Equal(t, employeeID, snapshot.DigitalEmployeePool[0].PrincipalID)
	require.NotNil(t, snapshot.DigitalEmployeePool[0].PlanningProfile)
	profile := snapshot.DigitalEmployeePool[0].PlanningProfile
	require.Equal(t, "not_ready", profile.RuntimeRequirements.DispatchReadinessStatus)
	require.Equal(t, []string{"runtime_not_ready"}, profile.RuntimeRequirements.DispatchBlockingReasons)
	require.Empty(t, profile.HardFailures)
}

func TestProjectStoreSnapshotAttachesPlanningProfilesFromSource(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	employeeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID},
		demand: project.ProjectDemand{
			ID:        demandID,
			TenantID:  tenantID,
			ProjectID: projectID,
			Title:     "分析数据库",
			Content:   strPtr("检查慢查询"),
		},
		members: []project.ProjectMember{{
			ID:                  uuid.New(),
			TenantID:            tenantID,
			ProjectID:           projectID,
			PrincipalType:       project.PrincipalTypeDigitalEmployee,
			PrincipalID:         employeeID,
			ProjectRole:         project.ProjectRoleExecutor,
			Status:              "active",
			DisplayNameSnapshot: strPtr("数据库员工"),
		}},
	}
	source := &fakePlanningProfileSource{
		records: map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord{
			employeeID: {
				DigitalEmployeeID: employeeID,
				EmployeeType:      "database_admin",
				CapabilityBindings: map[string]any{
					"external_capabilities": []any{"database.read"},
					"skills":                []any{"sql.analysis"},
				},
				ExecutionStatus: "ready",
			},
		},
	}

	snapshot, err := NewProjectStore(repo).WithDigitalEmployeePlanningProfiles(source).LoadProjectCoordinationSnapshot(context.Background(), LoadSnapshotInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		DemandID:  demandID,
	})

	require.NoError(t, err)
	require.Len(t, snapshot.DigitalEmployeePool, 1)
	profile := snapshot.DigitalEmployeePool[0].PlanningProfile
	require.NotNil(t, profile)
	require.Equal(t, employeeID, profile.DigitalEmployeeID)
	require.Equal(t, "database_admin", profile.RoleProfile.PrimaryRole)
	require.Equal(t, []PlanningCapability{{Key: "database.read", Level: "strong", Source: "capability_bindings.external_capabilities", Confidence: 0.9}}, profile.Capabilities)
}

func TestProjectStoreSnapshotKeepsUnknownProfileWhenProfileSourceFails(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	employeeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "分析数据库"},
		members: []project.ProjectMember{{
			ID:            uuid.New(),
			TenantID:      tenantID,
			ProjectID:     projectID,
			PrincipalType: project.PrincipalTypeDigitalEmployee,
			PrincipalID:   employeeID,
			ProjectRole:   project.ProjectRoleExecutor,
			Status:        "active",
		}},
	}

	snapshot, err := NewProjectStore(repo).WithDigitalEmployeePlanningProfiles(&fakePlanningProfileSource{err: errors.New("source down")}).LoadProjectCoordinationSnapshot(context.Background(), LoadSnapshotInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		DemandID:  demandID,
	})

	require.NoError(t, err)
	require.Len(t, snapshot.DigitalEmployeePool, 1)
	require.NotNil(t, snapshot.DigitalEmployeePool[0].PlanningProfile)
	require.Equal(t, "unknown", snapshot.DigitalEmployeePool[0].PlanningProfile.ProfileFreshness.SourceState)
	require.Contains(t, snapshot.DigitalEmployeePool[0].PlanningProfile.SelectionWarnings, "profile_source_missing")
}

type fakeLegacyReadiness struct {
	ready map[uuid.UUID]bool
	err   error
}

func (r fakeLegacyReadiness) AreRuntimeReady(_ context.Context, _ uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	if r.err != nil {
		return nil, r.err
	}
	result := make(map[uuid.UUID]bool, len(employeeIDs))
	for _, id := range employeeIDs {
		result[id] = r.ready[id]
	}
	return result, nil
}

type fakePlanningProfileSource struct {
	records   map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord
	projectID uuid.UUID
	err       error
}

func (s *fakePlanningProfileSource) PlanningProfileRecords(_ context.Context, _ uuid.UUID, projectID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord, error) {
	if s.err != nil {
		return nil, s.err
	}
	s.projectID = projectID
	out := map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord{}
	for _, id := range employeeIDs {
		if record, ok := s.records[id]; ok {
			out[id] = record
		}
	}
	return out, nil
}

type fakeLendingGatekeeper struct {
	employeeTeams map[uuid.UUID]uuid.UUID
	grantedTeams  map[uuid.UUID]bool
	resolveErr    error
	grantsErr     error
}

func (g fakeLendingGatekeeper) ResolveEmployeeTeams(_ context.Context, _ uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	if g.resolveErr != nil {
		return nil, g.resolveErr
	}
	out := map[uuid.UUID]uuid.UUID{}
	for _, id := range employeeIDs {
		if team, ok := g.employeeTeams[id]; ok {
			out[id] = team
		}
	}
	return out, nil
}

func (g fakeLendingGatekeeper) EffectiveLendingTeams(_ context.Context, _, _ uuid.UUID) (map[uuid.UUID]bool, error) {
	if g.grantsErr != nil {
		return nil, g.grantsErr
	}
	return g.grantedTeams, nil
}

func TestLoadSnapshotAppliesLendingGate(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	ownTeam := uuid.New()
	grantedTeam := uuid.New()
	foreignTeam := uuid.New()
	ownEmp := uuid.New()
	grantedEmp := uuid.New()
	foreignEmp := uuid.New()
	noTeamEmp := uuid.New()

	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, TeamID: &ownTeam},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "需求"},
		members: []project.ProjectMember{
			{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, PrincipalType: project.PrincipalTypeDigitalEmployee, PrincipalID: ownEmp, ProjectRole: project.ProjectRoleExecutor, Status: "active"},
			{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, PrincipalType: project.PrincipalTypeDigitalEmployee, PrincipalID: grantedEmp, ProjectRole: project.ProjectRoleExecutor, Status: "active"},
			{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, PrincipalType: project.PrincipalTypeDigitalEmployee, PrincipalID: foreignEmp, ProjectRole: project.ProjectRoleExecutor, Status: "active"},
			{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, PrincipalType: project.PrincipalTypeDigitalEmployee, PrincipalID: noTeamEmp, ProjectRole: project.ProjectRoleExecutor, Status: "active"},
		},
	}
	gate := fakeLendingGatekeeper{
		employeeTeams: map[uuid.UUID]uuid.UUID{ownEmp: ownTeam, grantedEmp: grantedTeam, foreignEmp: foreignTeam},
		grantedTeams:  map[uuid.UUID]bool{grantedTeam: true},
	}
	store := NewProjectStore(repo).WithLendingGatekeeper(gate)

	snapshot, err := store.LoadProjectCoordinationSnapshot(context.Background(), LoadSnapshotInput{TenantID: tenantID, ProjectID: projectID, DemandID: demandID})
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	got := map[uuid.UUID]bool{}
	for _, member := range snapshot.DigitalEmployeePool {
		got[member.PrincipalID] = true
	}
	if !got[ownEmp] || !got[grantedEmp] || !got[noTeamEmp] {
		t.Fatalf("own-team, granted-foreign-team and no-team employees must be eligible: %#v", snapshot.DigitalEmployeePool)
	}
	if got[foreignEmp] {
		t.Fatalf("ungranted foreign-team employee must be gated out: %#v", snapshot.DigitalEmployeePool)
	}
	skipEvents := 0
	for _, event := range repo.events {
		if event.EventType == project.ProjectEventLendingEmployeeSkipped {
			skipEvents++
		}
	}
	if skipEvents != 1 {
		t.Fatalf("expected one lending-skip event, got %d", skipEvents)
	}
}

func TestLoadSnapshotLendingGateFailsOpenWhenProjectHasNoTeam(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	employeeID := uuid.New()
	employeeTeam := uuid.New()

	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "需求"},
		members: []project.ProjectMember{
			{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, PrincipalType: project.PrincipalTypeDigitalEmployee, PrincipalID: employeeID, ProjectRole: project.ProjectRoleExecutor, Status: "active"},
		},
	}
	gate := fakeLendingGatekeeper{
		employeeTeams: map[uuid.UUID]uuid.UUID{employeeID: employeeTeam},
		grantedTeams:  map[uuid.UUID]bool{},
	}
	store := NewProjectStore(repo).WithLendingGatekeeper(gate)

	snapshot, err := store.LoadProjectCoordinationSnapshot(context.Background(), LoadSnapshotInput{TenantID: tenantID, ProjectID: projectID, DemandID: demandID})
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if len(snapshot.DigitalEmployeePool) != 1 || snapshot.DigitalEmployeePool[0].PrincipalID != employeeID {
		t.Fatalf("project without own team must not lending-gate executors: %#v", snapshot.DigitalEmployeePool)
	}
	if events := eventsByType(repo.events, project.ProjectEventLendingEmployeeSkipped); len(events) != 0 {
		t.Fatalf("project without own team must not record lending skips: %#v", events)
	}
}

func TestLoadSnapshotLendingGateFailsOpen(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	ownTeam := uuid.New()
	foreignEmp := uuid.New()

	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, TeamID: &ownTeam},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "需求"},
		members: []project.ProjectMember{
			{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, PrincipalType: project.PrincipalTypeDigitalEmployee, PrincipalID: foreignEmp, ProjectRole: project.ProjectRoleExecutor, Status: "active"},
		},
	}
	// A lending lookup error must not strand planning: the gate fails open (no filtering).
	gate := fakeLendingGatekeeper{grantsErr: errLendingGateProbe}
	store := NewProjectStore(repo).WithLendingGatekeeper(gate)

	snapshot, err := store.LoadProjectCoordinationSnapshot(context.Background(), LoadSnapshotInput{TenantID: tenantID, ProjectID: projectID, DemandID: demandID})
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if len(snapshot.DigitalEmployeePool) != 1 {
		t.Fatalf("gate error should fail open and keep the candidate: %#v", snapshot.DigitalEmployeePool)
	}
}

var errLendingGateProbe = errors.New("lending gate probe")

func TestProjectStorePersistRouteDecisionAggregatesGraphFields(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	jobID := uuid.New()
	demandID := uuid.New()
	firstEmployeeID := uuid.New()
	secondEmployeeID := uuid.New()
	repo := &projectStoreMemoryRepository{}
	store := NewProjectStore(repo)

	_, err := store.PersistRouteDecision(context.Background(), PersistRouteDecisionInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		JobID:     jobID,
		DemandID:  demandID,
		Decision: RouteDecisionPlan{
			Reason:              "分派并行调查和修复",
			RequiresHumanReview: true,
			BudgetEstimate:      map[string]any{"mode": "policy_default"},
			Tasks: []PlannedTask{
				{
					Key:                         "investigate",
					Title:                       "调查问题",
					Summary:                     "整理日志和复现路径",
					SelectedEmployeeID:          firstEmployeeID,
					EmployeeSelectionReason:     "具备日志调查能力",
					RequiredCapabilities:        []string{"log.analysis"},
					MatchedCapabilities:         []string{"log.analysis"},
					PermissionRequirements:      []string{"logs.read"},
					ToolRequirements:            []string{"mcp:logstore"},
					RuntimeRequirements:         []string{"provider:codex"},
					VerificationRequirements:    []string{"复现路径已记录"},
					SelectionScore:              92,
					PlanningProfileSnapshotHash: "profile-hash-for-route-summary",
					ExpectedOutputs:             []string{"execution_summary", "evidence_refs"},
					InputRequirements: map[string]any{
						"demand_id": demandID.String(),
						"prompt":    strings.Repeat("long prompt ", 20),
					},
					HandoffContract: map[string]any{"format": "markdown"},
				},
				{
					Key:                "repair",
					Title:              "修复问题",
					Summary:            "根据调查结论实施修复",
					SelectedEmployeeID: secondEmployeeID,
					ExpectedOutputs:    []string{"execution_summary", "recommended_next_action"},
					InputRequirements:  map[string]any{"demand_id": demandID.String()},
					HandoffContract:    map[string]any{"format": "patch"},
					BlockedByKeys:      []string{"investigate"},
				},
				{
					Key:                "verify",
					Title:              "验证修复",
					Summary:            "复跑回归检查",
					SelectedEmployeeID: firstEmployeeID,
					ExpectedOutputs:    []string{"evidence_refs"},
					InputRequirements:  map[string]any{"demand_id": demandID.String()},
					HandoffContract:    map[string]any{"format": "report"},
					BlockedByKeys:      []string{"repair"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("persist route decision: %v", err)
	}
	if len(repo.routeDecisionRequests) != 1 {
		t.Fatalf("expected one route decision request, got %d", len(repo.routeDecisionRequests))
	}
	req := repo.routeDecisionRequests[0]
	assertUUIDs(t, req.SelectedDigitalEmployeeIDs, []uuid.UUID{firstEmployeeID, secondEmployeeID})
	assertUUIDs(t, req.CandidateDigitalEmployeeIDs, []uuid.UUID{firstEmployeeID, secondEmployeeID})
	assertAnyStrings(t, req.ExpectedOutputs, []string{"execution_summary", "evidence_refs", "recommended_next_action"})
	if req.Reason != "分派并行调查和修复" || !req.RequiresHumanReview || req.BudgetEstimate["mode"] != "policy_default" {
		t.Fatalf("unexpected route decision fields: %#v", req)
	}

	taskSummaries, ok := req.InputRequirements["tasks"].([]any)
	if !ok || len(taskSummaries) != 3 {
		t.Fatalf("expected aggregated task summaries, got %#v", req.InputRequirements)
	}
	firstSummary, ok := taskSummaries[0].(map[string]any)
	if !ok {
		t.Fatalf("expected task summary map, got %#v", taskSummaries[0])
	}
	if firstSummary["key"] != "investigate" || firstSummary["selected_digital_employee_id"] != firstEmployeeID.String() {
		t.Fatalf("unexpected first task summary: %#v", firstSummary)
	}
	if _, storesRawInputs := firstSummary["input_requirements"]; storesRawInputs {
		t.Fatalf("route-level input summary must not store raw task input requirements: %#v", firstSummary)
	}
	assertPayloadStrings(t, firstSummary["input_requirement_keys"], []string{"demand_id", "prompt"})
	require.Equal(t, "具备日志调查能力", firstSummary["employee_selection_reason"])
	assertPayloadStrings(t, firstSummary["required_capabilities"], []string{"log.analysis"})
	assertPayloadStrings(t, firstSummary["matched_capabilities"], []string{"log.analysis"})
	assertPayloadStrings(t, firstSummary["permission_requirements"], []string{"logs.read"})
	assertPayloadStrings(t, firstSummary["tool_requirements"], []string{"mcp:logstore"})
	assertPayloadStrings(t, firstSummary["runtime_requirements"], []string{"provider:codex"})
	assertPayloadStrings(t, firstSummary["verification_requirements"], []string{"复现路径已记录"})
	require.Equal(t, 92, firstSummary["selection_score"])
	require.Equal(t, "profile-hash-for-route-summary", firstSummary["profile_snapshot_hash"])
}

func TestProjectStoreCreateCoordinationJobIsIdempotentForSameTrigger(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	workflowID := "project-coordinator:" + projectID.String()
	triggerEventID := uuid.New()
	repo := &projectStoreMemoryRepository{}
	store := NewProjectStore(repo)
	input := CreateCoordinationJobInput{
		TenantID:       tenantID,
		ProjectID:      projectID,
		WorkflowID:     workflowID,
		TriggerEventID: triggerEventID,
		JobType:        "demand_route",
	}

	first, err := store.CreateCoordinationJob(context.Background(), input)
	require.NoError(t, err)
	second, err := store.CreateCoordinationJob(context.Background(), input)
	require.NoError(t, err)

	require.Equal(t, first.ID, second.ID)
	createdEvents := eventsByType(repo.events, project.ProjectEventCoordinationJobCreated)
	require.Len(t, createdEvents, 1)
	require.Equal(t, first.ID.String(), createdEvents[0].ActorID)
}

func TestProjectStorePersistRouteDecisionIsIdempotentForSameJob(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	jobID := uuid.New()
	demandID := uuid.New()
	employeeID := uuid.New()
	repo := &projectStoreMemoryRepository{}
	store := NewProjectStore(repo)
	input := PersistRouteDecisionInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		JobID:     jobID,
		DemandID:  demandID,
		Decision: RouteDecisionPlan{
			Reason: "same job replay",
			Tasks: []PlannedTask{{
				Key:                "t1",
				Title:              "分析",
				Summary:            "分析需求",
				SelectedEmployeeID: employeeID,
				ExpectedOutputs:    []string{"execution_summary"},
				InputRequirements:  map[string]any{},
				HandoffContract:    map[string]any{},
			}},
		},
	}

	first, err := store.PersistRouteDecision(context.Background(), input)
	require.NoError(t, err)
	second, err := store.PersistRouteDecision(context.Background(), input)
	require.NoError(t, err)

	require.Equal(t, first.ID, second.ID)
	require.Equal(t, first.CreatedEventID, second.CreatedEventID)
	require.Len(t, repo.routeDecisionRequests, 1)
	createdEvents := eventsByType(repo.events, project.ProjectEventRouteDecisionCreated)
	require.Len(t, createdEvents, 1)
	require.Equal(t, jobID.String(), createdEvents[0].ActorID)
}

func TestProjectStorePersistsPendingPlanRevisionWithoutCreatingTasks(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	jobID := uuid.New()
	routeID := uuid.New()
	employeeID := uuid.New()
	ownerID := uuid.New()
	repo := &projectStoreMemoryRepository{}
	repo.projectRecord = project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID}
	store := NewProjectStore(repo)

	result, err := store.PersistPlanRevision(context.Background(), PersistPlanRevisionInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DemandID:          demandID,
		CoordinationJobID: jobID,
		RouteDecisionID:   routeID,
		Decision: RouteDecisionPlan{
			Reason:              "需要人工复核计划",
			RequiresHumanReview: true,
			Tasks: []PlannedTask{
				{
					Key:                     "inspect",
					Title:                   "检查",
					Summary:                 "检查输入",
					TaskKind:                "analysis",
					SelectedEmployeeID:      employeeID,
					EmployeeSelectionReason: "具备分析能力",
					RequiredCapabilities:    []string{"codebase.analysis"},
					MatchedCapabilities:     []string{"codebase.analysis"},
					ExpectedOutputs:         []string{"结论"},
					HandoffContract:         map[string]any{"acceptance_criteria": []any{"结论可复核"}},
				},
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, project.PlanRevisionStatusPendingReview, result.Status)
	require.NotEqual(t, uuid.Nil, result.ID)
	require.Empty(t, repo.decomposeAcceptedPlanRevisionRequests)
	require.Len(t, repo.planRevisions, 1)
}

func TestProjectStoreDecomposesOnlyAcceptedPlanRevision(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	jobID := uuid.New()
	routeID := uuid.New()
	revisionID := uuid.New()
	employeeID := uuid.New()
	repo := &projectStoreMemoryRepository{}
	repo.planRevisions = append(repo.planRevisions, project.PlanRevision{
		ID:              revisionID,
		TenantID:        tenantID,
		ProjectID:       projectID,
		DemandID:        demandID,
		Status:          project.PlanRevisionStatusAccepted,
		Payload:         map[string]any{"summary": "accepted"},
		PlanFingerprint: "fingerprint",
	})
	store := NewProjectStore(repo)

	tasks, err := store.DecomposeAcceptedPlanRevision(context.Background(), DecomposeAcceptedPlanRevisionInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DemandID:          demandID,
		CoordinationJobID: jobID,
		RouteDecisionID:   routeID,
		PlanRevisionID:    revisionID,
		PlanFingerprint:   "fingerprint",
		Payload: PlanRevisionPayload{
			Summary: "accepted",
			Tasks: []PlanRevisionTask{
				{
					PlannedTaskKey:          "inspect",
					Title:                   "检查",
					Objective:               "检查输入",
					TaskType:                "analysis",
					SelectedEmployeeID:      employeeID.String(),
					EmployeeSelectionReason: "具备分析能力",
					ExpectedOutputs:         []string{"结论"},
					Produces:                []string{"inspection_summary"},
					InputContextRefs:        []string{"project_context"},
					InputRequirements: map[string]any{
						"required_inputs": []any{"load_test_report"},
						"repository":      "superteam",
						"scope":           "one host",
					},
					AcceptanceCriteria: []string{"结论可复核"},
				},
			},
			FinalSummaryContract: PlanRevisionFinalSummaryContract{RequiredSections: []string{"conclusion", "evidence", "risks", "next_steps"}},
		},
	})

	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Len(t, repo.decomposeAcceptedPlanRevisionRequests, 1)
	req := repo.decomposeAcceptedPlanRevisionRequests[0]
	require.Equal(t, revisionID, req.AcceptedPlanRevisionID)
	require.Equal(t, "fingerprint", req.PlanFingerprint)
	require.Len(t, req.Tasks, 1)
	require.Equal(t, map[string]any{
		"required_inputs":    []any{"load_test_report"},
		"input_context_refs": []string{"project_context"},
	}, req.Tasks[0].InputRequirements)
	require.Equal(t, []any{"inspection_summary"}, req.Tasks[0].PlannerMetadata["produces"])
	require.Equal(t, map[string]any{"repository": "superteam", "scope": "one host"}, req.Tasks[0].PlannerMetadata["planner_notes"])
}

func TestProjectStoreRequestPlanRevisionReviewStoresPlanRevisionID(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	coordinationJobID := uuid.New()
	planRevisionID := uuid.New()
	targetUserID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{
			ID:               projectID,
			TenantID:         tenantID,
			HumanOwnerUserID: targetUserID,
		},
		demand: project.ProjectDemand{
			ID:        demandID,
			TenantID:  tenantID,
			ProjectID: projectID,
		},
	}
	approvals := &projectStoreApprovalCreator{approvalID: uuid.New()}
	store := NewProjectStoreWithApprovals(repo, approvals)

	decision, err := store.RequestPlanRevisionReview(context.Background(), RequestPlanRevisionReviewInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		CoordinationJobID: coordinationJobID,
		DemandID:          demandID,
		PlanRevisionID:    planRevisionID,
		PlanFingerprint:   "fingerprint",
		Payload: PlanRevisionPayload{
			Summary: "需要审核",
			RiskAssessment: PlanRevisionRiskAssessment{
				HighestRiskLevel: "high",
			},
		},
		CreatedEventID: uuid.New(),
	})

	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, decision.ID)
	require.Len(t, repo.decisionRequests, 1)
	require.NotNil(t, repo.decisionRequests[0].PlanRevisionID)
	require.Equal(t, planRevisionID, *repo.decisionRequests[0].PlanRevisionID)
	require.NotNil(t, repo.decisionRequests[0].SummarySnapshot)
	require.Equal(t, "需要审核", *repo.decisionRequests[0].SummarySnapshot)
	require.NotNil(t, repo.decisionRequests[0].RiskLevelSnapshot)
	require.Equal(t, "high", *repo.decisionRequests[0].RiskLevelSnapshot)
}

func TestProjectStoreListDispatchableTasksFiltersBlockedTasksAndUnresolvedBlockers(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	jobID := uuid.New()
	routeID := uuid.New()
	demandID := uuid.New()
	rootID := uuid.New()
	pendingID := uuid.New()
	blockedID := uuid.New()
	plannedButBlockedID := uuid.New()
	completedBlockerID := uuid.New()
	readyDependentID := uuid.New()
	unrelatedJobID := uuid.New()
	repo := &projectStoreMemoryRepository{
		tasks: []project.ProjectTask{
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, rootID, "planned"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, pendingID, "pending"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, blockedID, "blocked"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, plannedButBlockedID, "planned"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, completedBlockerID, "completed"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, readyDependentID, "planned"),
			projectStoreTask(tenantID, projectID, demandID, unrelatedJobID, routeID, uuid.New(), "planned"),
		},
		taskDependencies: []project.ProjectTaskDependency{
			projectStoreDependency(tenantID, projectID, jobID, blockedID, rootID),
			projectStoreDependency(tenantID, projectID, jobID, plannedButBlockedID, pendingID),
			projectStoreDependency(tenantID, projectID, jobID, readyDependentID, completedBlockerID),
		},
	}
	repo.setTaskLatestResult(completedBlockerID, projectStoreTaskResult(tenantID, projectID, completedBlockerID, project.TaskResultDecisionCompleteAccepted, "accepted"))
	store := NewProjectStore(repo)

	ids, err := store.ListDispatchableTasks(context.Background(), ListDispatchableTasksInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		CoordinationJobID: jobID,
	})

	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{rootID, readyDependentID}, ids)
}

func TestProjectStoreListDispatchableTasksSkipsFutureRetry(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	jobID := uuid.New()
	taskID := uuid.New()
	future := time.Now().UTC().Add(time.Hour)
	repo := &projectStoreMemoryRepository{
		tasks: []project.ProjectTask{{
			ID:                taskID,
			TenantID:          tenantID,
			ProjectID:         projectID,
			CoordinationJobID: &jobID,
			Status:            project.ProjectTaskStatusPlanned,
			RetryNotBefore:    &future,
		}},
	}
	store := NewProjectStore(repo).WithClock(func() time.Time { return time.Now().UTC() })

	ready, err := store.ListDispatchableTasks(context.Background(), ListDispatchableTasksInput{TenantID: tenantID, ProjectID: projectID, CoordinationJobID: jobID})

	require.NoError(t, err)
	require.Empty(t, ready)
}

func TestProjectStoreListDispatchableTasksIncludesDueRetry(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	jobID := uuid.New()
	taskID := uuid.New()
	now := time.Now().UTC()
	past := now.Add(-time.Minute)
	repo := &projectStoreMemoryRepository{
		tasks: []project.ProjectTask{{
			ID:                taskID,
			TenantID:          tenantID,
			ProjectID:         projectID,
			CoordinationJobID: &jobID,
			Status:            project.ProjectTaskStatusPlanned,
			RetryNotBefore:    &past,
		}},
	}
	store := NewProjectStore(repo).WithClock(func() time.Time { return now })

	ready, err := store.ListDispatchableTasks(context.Background(), ListDispatchableTasksInput{TenantID: tenantID, ProjectID: projectID, CoordinationJobID: jobID})

	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{taskID}, ready)
}

func TestProjectStoreListDispatchableTasksRequiresAcceptedLatestBlockerResult(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	jobID := uuid.New()
	routeID := uuid.New()
	demandID := uuid.New()
	rootID := uuid.New()
	noResultBlockerID := uuid.New()
	waitingResultBlockerID := uuid.New()
	acceptedBlockerID := uuid.New()
	noResultDependentID := uuid.New()
	waitingResultDependentID := uuid.New()
	acceptedDependentID := uuid.New()
	repo := &projectStoreMemoryRepository{
		tasks: []project.ProjectTask{
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, rootID, "planned"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, noResultBlockerID, "completed"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, waitingResultBlockerID, "completed"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, acceptedBlockerID, "completed"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, noResultDependentID, "planned"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, waitingResultDependentID, "planned"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, acceptedDependentID, "planned"),
		},
		taskDependencies: []project.ProjectTaskDependency{
			projectStoreDependency(tenantID, projectID, jobID, noResultDependentID, noResultBlockerID),
			projectStoreDependency(tenantID, projectID, jobID, waitingResultDependentID, waitingResultBlockerID),
			projectStoreDependency(tenantID, projectID, jobID, acceptedDependentID, acceptedBlockerID),
		},
	}
	repo.setTaskLatestResult(waitingResultBlockerID, projectStoreTaskResult(tenantID, projectID, waitingResultBlockerID, project.TaskResultDecisionWaitingHumanReview, "accepted"))
	repo.setTaskLatestResult(acceptedBlockerID, projectStoreTaskResult(tenantID, projectID, acceptedBlockerID, project.TaskResultDecisionCompleteAccepted, "accepted"))
	store := NewProjectStore(repo)

	ids, err := store.ListDispatchableTasks(context.Background(), ListDispatchableTasksInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		CoordinationJobID: jobID,
	})

	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{rootID, acceptedDependentID}, ids)
}

func TestProjectStoreResolveReadyDownstreamUpdatesOnlyUnblockedDependents(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	jobID := uuid.New()
	routeID := uuid.New()
	demandID := uuid.New()
	completedTaskID := uuid.New()
	readyDownstreamID := uuid.New()
	blockedDownstreamID := uuid.New()
	otherBlockerID := uuid.New()
	alreadyPlannedID := uuid.New()
	repo := &projectStoreMemoryRepository{
		tasks: []project.ProjectTask{
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, completedTaskID, "completed"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, readyDownstreamID, "blocked"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, blockedDownstreamID, "blocked"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, otherBlockerID, "assigned"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, alreadyPlannedID, "planned"),
		},
		taskDependencies: []project.ProjectTaskDependency{
			projectStoreDependency(tenantID, projectID, jobID, readyDownstreamID, completedTaskID),
			projectStoreDependency(tenantID, projectID, jobID, blockedDownstreamID, completedTaskID),
			projectStoreDependency(tenantID, projectID, jobID, blockedDownstreamID, otherBlockerID),
			projectStoreDependency(tenantID, projectID, jobID, alreadyPlannedID, completedTaskID),
		},
	}
	repo.setTaskLatestResult(completedTaskID, projectStoreTaskResult(tenantID, projectID, completedTaskID, project.TaskResultDecisionCompleteAccepted, "accepted"))
	store := NewProjectStore(repo)

	ids, err := store.ResolveReadyDownstream(context.Background(), ResolveReadyDownstreamInput{
		TenantID:        tenantID,
		ProjectID:       projectID,
		CompletedTaskID: completedTaskID,
	})

	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{readyDownstreamID}, ids)
	require.Equal(t, "planned", repo.taskStatus(readyDownstreamID))
	require.Equal(t, "blocked", repo.taskStatus(blockedDownstreamID))
	require.Equal(t, "planned", repo.taskStatus(alreadyPlannedID))
	require.Equal(t, []projectTaskStatusUpdateRecord{
		{TenantID: tenantID, TaskID: readyDownstreamID, Status: "planned", CurrentStatuses: []string{"blocked"}},
	}, repo.statusUpdates)
}

func TestProjectStoreResolveReadyDownstreamRequiresAcceptedLatestBlockerResult(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	jobID := uuid.New()
	routeID := uuid.New()
	demandID := uuid.New()
	completedTaskID := uuid.New()
	downstreamID := uuid.New()
	repo := &projectStoreMemoryRepository{
		tasks: []project.ProjectTask{
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, completedTaskID, "completed"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, downstreamID, "blocked"),
		},
		taskDependencies: []project.ProjectTaskDependency{
			projectStoreDependency(tenantID, projectID, jobID, downstreamID, completedTaskID),
		},
	}
	store := NewProjectStore(repo)

	ids, err := store.ResolveReadyDownstream(context.Background(), ResolveReadyDownstreamInput{
		TenantID:        tenantID,
		ProjectID:       projectID,
		CompletedTaskID: completedTaskID,
	})

	require.NoError(t, err)
	require.Empty(t, ids)
	require.Equal(t, "blocked", repo.taskStatus(downstreamID))
	require.Empty(t, repo.statusUpdates)

	repo.setTaskLatestResult(completedTaskID, projectStoreTaskResult(tenantID, projectID, completedTaskID, project.TaskResultDecisionCompleteAccepted, "accepted"))

	ids, err = store.ResolveReadyDownstream(context.Background(), ResolveReadyDownstreamInput{
		TenantID:        tenantID,
		ProjectID:       projectID,
		CompletedTaskID: completedTaskID,
	})

	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{downstreamID}, ids)
	require.Equal(t, "planned", repo.taskStatus(downstreamID))
	require.Equal(t, []projectTaskStatusUpdateRecord{
		{TenantID: tenantID, TaskID: downstreamID, Status: "planned", CurrentStatuses: []string{"blocked"}},
	}, repo.statusUpdates)
}

func TestApplyTaskResultRevisionCreatesBoundedRevisionTask(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	coordinationJobID := uuid.New()
	routeDecisionID := uuid.New()
	sourceTaskID := uuid.New()
	resultID := uuid.New()
	employeeID := uuid.New()
	maxAttempts := int32(3)
	repo := &projectStoreMemoryRepository{
		tasks: []project.ProjectTask{{
			ID:                        sourceTaskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			CoordinationJobID:         &coordinationJobID,
			RouteDecisionID:           &routeDecisionID,
			Title:                     "Implement login",
			Summary:                   strPtr("Wire redirect flow"),
			Status:                    project.ProjectTaskStatusCompleted,
			AttemptCount:              1,
			MaxAttempts:               &maxAttempts,
			AssignedDigitalEmployeeID: &employeeID,
			TaskKind:                  strPtr("implementation"),
			RiskLevel:                 strPtr("medium"),
			ExpectedOutputs:           []any{"patch", "test_evidence"},
			InputRequirements:         map[string]any{"existing": "context"},
			PlannerMetadata:           map[string]any{"iteration_key": "wi-login"},
			HandoffContract:           map[string]any{"completion_path": "project_task_attempt_writeback"},
		}},
		projectTaskResults: []project.ProjectTaskResult{{
			ID:            resultID,
			TenantID:      tenantID,
			ProjectID:     projectID,
			ProjectTaskID: sourceTaskID,
			ResultStatus:  project.TaskResultStatusRevisionNeeded,
			Decision:      project.TaskResultDecisionRevisionAttempt,
			Contract: project.TaskResultContract{
				Status:  project.TaskResultStatusRevisionNeeded,
				Summary: "tests failed",
				RevisionRequest: &project.TaskResultRevisionRequest{
					Reason:           "login test failed",
					RequestedChanges: []string{"fix redirect"},
				},
			},
		}},
	}
	store := NewProjectStore(repo)

	created, err := store.CreateRevisionTaskForResult(context.Background(), CreateRevisionTaskForResultInput{
		TenantID:     tenantID,
		ProjectID:    projectID,
		SourceTaskID: sourceTaskID,
		ResultID:     resultID,
	})

	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, created.TaskID)
	revision := repo.mustTask(created.TaskID)
	require.Equal(t, project.ProjectTaskStatusPlanned, revision.Status)
	require.Equal(t, &sourceTaskID, revision.RevisionOfTaskID)
	require.Equal(t, "wi-login", revision.PlannerMetadata["iteration_key"])
	require.Equal(t, "login test failed", revision.InputRequirements["revision_reason"])
	require.Equal(t, []string{"fix redirect"}, revision.InputRequirements["requested_changes"])
	require.Equal(t, sourceTaskID.String(), revision.InputRequirements["source_task_id"])
	require.Equal(t, resultID.String(), revision.InputRequirements["source_result_id"])
	require.Equal(t, repo.tasks[0].HandoffContract, revision.HandoffContract)
	require.Equal(t, &employeeID, revision.AssignedDigitalEmployeeID)
}

func TestCreateUpstreamSupplementTasksDispatchesToOwner(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	coordinationJobID := uuid.New()
	routeDecisionID := uuid.New()
	acceptedPlanRevisionID := uuid.New()
	ownerTaskID := uuid.New()
	sourceTaskID := uuid.New()
	ownerEmployeeID := uuid.New()
	sourceEmployeeID := uuid.New()
	ownerTaskKey := "load-test"
	sourceTaskKey := "publish"
	repo := &projectStoreMemoryRepository{
		tasks: []project.ProjectTask{
			{
				ID:                        ownerTaskID,
				TenantID:                  tenantID,
				ProjectID:                 projectID,
				DemandID:                  &demandID,
				CoordinationJobID:         &coordinationJobID,
				RouteDecisionID:           &routeDecisionID,
				AcceptedPlanRevisionID:    &acceptedPlanRevisionID,
				Title:                     "Run load test",
				Status:                    project.ProjectTaskStatusCompleted,
				AssignedDigitalEmployeeID: &ownerEmployeeID,
				PlannedTaskKey:            &ownerTaskKey,
				PlannerMetadata:           map[string]any{"produces": []any{"load_test_report"}},
			},
			{
				ID:                        sourceTaskID,
				TenantID:                  tenantID,
				ProjectID:                 projectID,
				DemandID:                  &demandID,
				CoordinationJobID:         &coordinationJobID,
				RouteDecisionID:           &routeDecisionID,
				AcceptedPlanRevisionID:    &acceptedPlanRevisionID,
				Title:                     "Publish capacity conclusion",
				Status:                    project.ProjectTaskStatusCompleted,
				AssignedDigitalEmployeeID: &sourceEmployeeID,
				PlannedTaskKey:            &sourceTaskKey,
				InputRequirements:         map[string]any{"required_inputs": []any{"load_test_report"}},
			},
		},
	}
	store := NewProjectStore(repo)

	created, err := store.CreateUpstreamSupplementTasks(context.Background(), CreateUpstreamSupplementInput{
		TenantID:      tenantID,
		ProjectID:     projectID,
		SourceTaskID:  sourceTaskID,
		MissingInputs: []string{"load_test_report"},
	})

	require.NoError(t, err)
	require.Len(t, created.TaskIDs, 1)
	supplement := repo.mustTask(created.TaskIDs[0])
	require.Equal(t, &ownerEmployeeID, supplement.AssignedDigitalEmployeeID)
	require.NotEqual(t, &sourceEmployeeID, supplement.AssignedDigitalEmployeeID)
	require.Equal(t, &ownerTaskID, supplement.RevisionOfTaskID)
	require.Equal(t, &acceptedPlanRevisionID, supplement.AcceptedPlanRevisionID)
	require.Equal(t, sourceTaskID.String(), supplement.PlannerMetadata["supplement_for"])
	require.Equal(t, []string{"load_test_report"}, supplement.PlannerMetadata["missing_inputs"])
}

func TestApplyTaskResultRevisionStopsOnRepeatedFailureFingerprint(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	coordinationJobID := uuid.New()
	routeDecisionID := uuid.New()
	sourceTaskID := uuid.New()
	resultID := uuid.New()
	employeeID := uuid.New()
	maxAttempts := int32(5)
	repeatedContract := project.TaskResultContract{
		Status:  project.TaskResultStatusRevisionNeeded,
		Summary: "tests failed",
		RevisionRequest: &project.TaskResultRevisionRequest{
			Reason:           "login test failed",
			RequestedChanges: []string{"fix redirect"},
		},
	}
	fingerprint := revisionFailureFingerprint(repeatedContract)
	firstRevisionID := uuid.New()
	secondRevisionID := uuid.New()
	repo := &projectStoreMemoryRepository{
		tasks: []project.ProjectTask{
			{
				ID:                        sourceTaskID,
				TenantID:                  tenantID,
				ProjectID:                 projectID,
				DemandID:                  &demandID,
				CoordinationJobID:         &coordinationJobID,
				RouteDecisionID:           &routeDecisionID,
				Title:                     "Implement login",
				Status:                    project.ProjectTaskStatusCompleted,
				AttemptCount:              2,
				MaxAttempts:               &maxAttempts,
				AssignedDigitalEmployeeID: &employeeID,
				PlannerMetadata:           map[string]any{"iteration_key": "wi-login"},
			},
			{
				ID:                firstRevisionID,
				TenantID:          tenantID,
				ProjectID:         projectID,
				DemandID:          &demandID,
				CoordinationJobID: &coordinationJobID,
				RevisionOfTaskID:  &sourceTaskID,
				Status:            project.ProjectTaskStatusCompleted,
				PlannerMetadata: map[string]any{
					"iteration_key":                "wi-login",
					"revision_failure_fingerprint": fingerprint,
				},
				CreatedAt: time.Now().UTC().Add(-2 * time.Minute),
			},
			{
				ID:                secondRevisionID,
				TenantID:          tenantID,
				ProjectID:         projectID,
				DemandID:          &demandID,
				CoordinationJobID: &coordinationJobID,
				RevisionOfTaskID:  &sourceTaskID,
				Status:            project.ProjectTaskStatusCompleted,
				PlannerMetadata: map[string]any{
					"iteration_key":                "wi-login",
					"revision_failure_fingerprint": fingerprint,
				},
				CreatedAt: time.Now().UTC().Add(-1 * time.Minute),
			},
		},
		projectTaskResults: []project.ProjectTaskResult{{
			ID:            resultID,
			TenantID:      tenantID,
			ProjectID:     projectID,
			ProjectTaskID: sourceTaskID,
			ResultStatus:  project.TaskResultStatusRevisionNeeded,
			Decision:      project.TaskResultDecisionRevisionAttempt,
			Contract:      repeatedContract,
		}},
	}
	store := NewProjectStore(repo)

	created, err := store.CreateRevisionTaskForResult(context.Background(), CreateRevisionTaskForResultInput{
		TenantID:     tenantID,
		ProjectID:    projectID,
		SourceTaskID: sourceTaskID,
		ResultID:     resultID,
	})

	require.NoError(t, err)
	require.True(t, created.Exhausted)
	require.Equal(t, uuid.Nil, created.TaskID)
	require.Len(t, repo.tasks, 3)
}

func TestRevisionFailureFingerprintIgnoresModelProse(t *testing.T) {
	changes := []string{"add load test report", "attach baseline metrics"}

	first := revisionFailureFingerprint(project.TaskResultContract{
		Status:  project.TaskResultStatusRevisionNeeded,
		Summary: "I could not finish because the report was missing.",
		RevisionRequest: &project.TaskResultRevisionRequest{
			Reason:           "missing report",
			RequestedChanges: changes,
		},
	})

	// Same structural failure, different prose. The circuit breaker must not be
	// defeated by rewording.
	second := revisionFailureFingerprint(project.TaskResultContract{
		Status:  project.TaskResultStatusRevisionNeeded,
		Summary: "Unfortunately I was unable to complete this task at all.",
		RevisionRequest: &project.TaskResultRevisionRequest{
			Reason:           "the report is nowhere to be found",
			RequestedChanges: changes,
		},
	})

	require.Equal(t, first, second)
}

func TestRevisionFailureFingerprintSeparatesDifferentRequestedChanges(t *testing.T) {
	base := project.TaskResultContract{
		Status:          project.TaskResultStatusRevisionNeeded,
		RevisionRequest: &project.TaskResultRevisionRequest{RequestedChanges: []string{"add load test report"}},
	}
	other := project.TaskResultContract{
		Status:          project.TaskResultStatusRevisionNeeded,
		RevisionRequest: &project.TaskResultRevisionRequest{RequestedChanges: []string{"add baseline metrics"}},
	}

	require.NotEqual(t, revisionFailureFingerprint(base), revisionFailureFingerprint(other))
}

func TestRevisionFailureFingerprintIsOrderInsensitive(t *testing.T) {
	a := project.TaskResultContract{
		Status:          project.TaskResultStatusRevisionNeeded,
		RevisionRequest: &project.TaskResultRevisionRequest{RequestedChanges: []string{"b", "a"}},
	}
	b := project.TaskResultContract{
		Status:          project.TaskResultStatusRevisionNeeded,
		RevisionRequest: &project.TaskResultRevisionRequest{RequestedChanges: []string{"a", "b"}},
	}

	require.Equal(t, revisionFailureFingerprint(a), revisionFailureFingerprint(b))
}

func TestRevisionFailureFingerprintCoversMissingInputs(t *testing.T) {
	a := project.TaskResultContract{
		Status:  project.TaskResultStatusBlocked,
		Blocker: &project.TaskResultBlocker{MissingInputs: []string{"x", "y"}},
	}
	b := project.TaskResultContract{
		Status:  project.TaskResultStatusBlocked,
		Blocker: &project.TaskResultBlocker{MissingInputs: []string{"y", "x"}},
	}
	require.Equal(t, revisionFailureFingerprint(a), revisionFailureFingerprint(b))

	// Different inputs -> different fingerprint.
	c := project.TaskResultContract{
		Status:  project.TaskResultStatusBlocked,
		Blocker: &project.TaskResultBlocker{MissingInputs: []string{"z"}},
	}
	require.NotEqual(t, revisionFailureFingerprint(a), revisionFailureFingerprint(c))
}

func TestApplyTaskResultRevisionStopsWhenMaxAttemptsExhausted(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	sourceTaskID := uuid.New()
	resultID := uuid.New()
	maxAttempts := int32(2)
	repo := &projectStoreMemoryRepository{
		tasks: []project.ProjectTask{{
			ID:           sourceTaskID,
			TenantID:     tenantID,
			ProjectID:    projectID,
			Title:        "Implement login",
			Status:       project.ProjectTaskStatusCompleted,
			AttemptCount: 2,
			MaxAttempts:  &maxAttempts,
		}},
		projectTaskResults: []project.ProjectTaskResult{{
			ID:            resultID,
			TenantID:      tenantID,
			ProjectID:     projectID,
			ProjectTaskID: sourceTaskID,
			ResultStatus:  project.TaskResultStatusRevisionNeeded,
			Decision:      project.TaskResultDecisionRevisionAttempt,
			Contract: project.TaskResultContract{
				Status: project.TaskResultStatusRevisionNeeded,
				RevisionRequest: &project.TaskResultRevisionRequest{
					Reason: "attempts exhausted",
				},
			},
		}},
	}
	store := NewProjectStore(repo)

	created, err := store.CreateRevisionTaskForResult(context.Background(), CreateRevisionTaskForResultInput{
		TenantID:     tenantID,
		ProjectID:    projectID,
		SourceTaskID: sourceTaskID,
		ResultID:     resultID,
	})

	require.NoError(t, err)
	require.True(t, created.Exhausted)
	require.Equal(t, uuid.Nil, created.TaskID)
	require.Len(t, repo.tasks, 1)
}

func TestApplyTaskResultRevisionStopsWhenHistoryExhaustsMaxDespiteResetMetadata(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	coordinationJobID := uuid.New()
	sourceTaskID := uuid.New()
	resultID := uuid.New()
	maxAttempts := int32(3)
	revisionRootID := uuid.New()
	firstRevisionID := uuid.New()
	secondRevisionID := uuid.New()
	revisionContract := project.TaskResultContract{
		Status:  project.TaskResultStatusRevisionNeeded,
		Summary: "different failure this time",
		RevisionRequest: &project.TaskResultRevisionRequest{
			Reason:           "new failing assertion",
			RequestedChanges: []string{"fix the new assertion"},
		},
	}
	repo := &projectStoreMemoryRepository{
		tasks: []project.ProjectTask{
			{
				ID:                sourceTaskID,
				TenantID:          tenantID,
				ProjectID:         projectID,
				CoordinationJobID: &coordinationJobID,
				Title:             "Implement login revision",
				Status:            project.ProjectTaskStatusCompleted,
				MaxAttempts:       &maxAttempts,
				RevisionOfTaskID:  &revisionRootID,
				PlannerMetadata: map[string]any{
					"iteration_key":           "wi-login",
					"revision_root_task_id":   revisionRootID.String(),
					"revision_attempt_count":  1,
					"revision_max_attempts":   maxAttempts,
					"revision_failure_marker": "tampered-low-count",
				},
			},
			{
				ID:                firstRevisionID,
				TenantID:          tenantID,
				ProjectID:         projectID,
				CoordinationJobID: &coordinationJobID,
				RevisionOfTaskID:  &revisionRootID,
				Status:            project.ProjectTaskStatusCompleted,
				PlannerMetadata: map[string]any{
					"iteration_key":           "wi-login",
					"revision_root_task_id":   revisionRootID.String(),
					"revision_attempt_count":  2,
					"revision_failure_marker": "old-1",
				},
				CreatedAt: time.Now().UTC().Add(-2 * time.Minute),
			},
			{
				ID:                secondRevisionID,
				TenantID:          tenantID,
				ProjectID:         projectID,
				CoordinationJobID: &coordinationJobID,
				RevisionOfTaskID:  &revisionRootID,
				Status:            project.ProjectTaskStatusCompleted,
				PlannerMetadata: map[string]any{
					"iteration_key":           "wi-login",
					"revision_root_task_id":   revisionRootID.String(),
					"revision_attempt_count":  3,
					"revision_failure_marker": "old-2",
				},
				CreatedAt: time.Now().UTC().Add(-1 * time.Minute),
			},
		},
		projectTaskResults: []project.ProjectTaskResult{{
			ID:            resultID,
			TenantID:      tenantID,
			ProjectID:     projectID,
			ProjectTaskID: sourceTaskID,
			ResultStatus:  project.TaskResultStatusRevisionNeeded,
			Decision:      project.TaskResultDecisionRevisionAttempt,
			Contract:      revisionContract,
		}},
	}
	store := NewProjectStore(repo)

	created, err := store.CreateRevisionTaskForResult(context.Background(), CreateRevisionTaskForResultInput{
		TenantID:     tenantID,
		ProjectID:    projectID,
		SourceTaskID: sourceTaskID,
		ResultID:     resultID,
	})

	require.NoError(t, err)
	require.True(t, created.Exhausted)
	require.Equal(t, uuid.Nil, created.TaskID)
	require.Len(t, repo.tasks, 3)
}

func TestProjectStoreRequestProjectAcceptanceReviewTransitionsAndIsIdempotent(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	approvalID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{
			ID:               projectID,
			TenantID:         tenantID,
			Status:           project.ProjectStatusRunning,
			HumanOwnerUserID: ownerID,
		},
		approvalID: approvalID,
	}
	approvals := &projectStoreApprovalCreator{approvalID: approvalID}
	inbox := &projectStoreDecisionInboxProjector{}
	store := NewProjectStoreWithApprovalsAndInbox(repo, approvals, inbox)

	result, err := store.RequestProjectAcceptanceReview(context.Background(), RequestProjectAcceptanceReviewInput{
		TenantID: tenantID, ProjectID: projectID,
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, result.ID)
	require.Equal(t, project.ProjectStatusAcceptance, repo.projectRecord.Status)
	require.NotEmpty(t, repo.decisionRequests)
	require.Equal(t, "project_acceptance", repo.decisionRequests[0].DecisionType)
	require.Equal(t, ownerID, repo.decisionRequests[0].TargetUserID)
	require.Len(t, inbox.upserts, 1)

	// Second call: project is no longer running (already in acceptance) -> idempotent no-op.
	repo.decisionRequests = nil
	inbox.upserts = nil
	repeat, err := store.RequestProjectAcceptanceReview(context.Background(), RequestProjectAcceptanceReviewInput{
		TenantID: tenantID, ProjectID: projectID,
	})
	require.NoError(t, err)
	require.Equal(t, uuid.Nil, repeat.ID)
	require.Empty(t, repo.decisionRequests, "idempotent review must not create a second decision request")
}

func TestProjectStoreRequestProjectAcceptanceReviewReturnsDecisionWhenInboxProjectionFails(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	approvalID := uuid.New()
	inboxErr := errors.New("inbox projection unavailable")
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{
			ID:               projectID,
			TenantID:         tenantID,
			Status:           project.ProjectStatusRunning,
			HumanOwnerUserID: ownerID,
		},
		approvalID: approvalID,
	}
	approvals := &projectStoreApprovalCreator{approvalID: approvalID}
	inbox := &projectStoreDecisionInboxProjector{upsertErr: inboxErr}
	store := NewProjectStoreWithApprovalsAndInbox(repo, approvals, inbox)

	result, err := store.RequestProjectAcceptanceReview(context.Background(), RequestProjectAcceptanceReviewInput{
		TenantID: tenantID, ProjectID: projectID,
	})

	require.NoError(t, err)
	require.Equal(t, project.ProjectStatusAcceptance, repo.projectRecord.Status)
	require.Len(t, repo.decisionRequests, 1)
	require.Equal(t, repo.decisionRequests[0].ID, result.ID)
	require.Len(t, inbox.upserts, 1)
}

func TestProjectStoreRequestProjectAcceptanceReviewCreatesFinalDemandSummary(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	jobID := uuid.New()
	routeID := uuid.New()
	decisionRequestID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{
			ID:               projectID,
			TenantID:         tenantID,
			Status:           project.ProjectStatusRunning,
			HumanOwnerUserID: ownerID,
		},
		demands: []project.ProjectDemand{{
			ID:        demandID,
			TenantID:  tenantID,
			ProjectID: projectID,
			Title:     "交付可验收结果",
			Content:   strPtr("完成任务并沉淀证据"),
			Status:    project.ProjectDemandStatusCompleted,
		}},
		tasks: []project.ProjectTask{projectStoreTask(tenantID, projectID, demandID, jobID, routeID, taskID, project.ProjectTaskStatusCompleted)},
	}
	staleResult := projectStoreTaskResult(tenantID, projectID, taskID, project.TaskResultDecisionValidationFailed, "rejected")
	staleResult.Contract.ArtifactRefs = []project.TaskResultRef{{ID: "old-artifact", Kind: "log"}}
	repo.projectTaskResults = append(repo.projectTaskResults, staleResult)
	acceptedResult := projectStoreTaskResult(tenantID, projectID, taskID, project.TaskResultDecisionCompleteAccepted, "accepted")
	acceptedResult.DecisionRequestID = &decisionRequestID
	acceptedResult.Contract = project.TaskResultContract{
		Status:  project.TaskResultStatusCompleted,
		Summary: "真实链路验证通过",
		AcceptanceResults: []project.TaskResultAcceptanceResult{{
			ID:           "acceptance-1",
			Criterion:    "API 返回非 5xx",
			Status:       project.TaskResultCriterionStatusPassed,
			Summary:      "curl smoke passed",
			EvidenceRefs: []string{"evidence-1"},
		}},
		EvidenceRefs: []project.TaskResultRef{{ID: "evidence-1", Kind: "log", Ref: "run-123", Title: "运行日志"}},
		ArtifactRefs: []project.TaskResultRef{{ID: "artifact-1", Kind: "report", URI: "artifact://report-1", Title: "交付报告"}},
		ChangesMade:  []project.TaskResultChange{{Type: "code", Summary: "补齐验收链路", Files: []string{"apps/control-plane/internal/project/service.go"}}},
		Verification: []project.TaskResultVerification{{Status: project.TaskResultVerificationStatusPassed, Type: "curl", Summary: "真实接口 smoke 通过"}},
		Risks:        []project.TaskResultRisk{{Summary: "仍需人工验收", Severity: "medium", Mitigation: "由负责人确认"}},
		FollowUpRequests: []project.TaskResultFollowUpRequest{{
			Type:    "manual_acceptance",
			Summary: "负责人完成最终验收",
		}},
	}
	repo.setTaskLatestResult(taskID, acceptedResult)
	approvals := &projectStoreApprovalCreator{}
	store := NewProjectStoreWithApprovals(repo, approvals)

	result, err := store.RequestProjectAcceptanceReview(context.Background(), RequestProjectAcceptanceReviewInput{
		TenantID: tenantID, ProjectID: projectID,
	})

	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, result.ID)
	require.Equal(t, project.ProjectStatusAcceptance, repo.projectRecord.Status)
	require.Len(t, repo.demandSummaries, 1)
	summaryEvents := projectStoreEventsByType(repo.events, project.ProjectEventDemandSummaryCreated)
	require.Len(t, summaryEvents, 1)
	summary := repo.demandSummaries[0]
	require.Equal(t, summary.ID.String(), summaryEvents[0].Payload["summary_id"])
	require.Equal(t, string(project.ProjectDemandStatusCompleted), summary.Status)
	require.Contains(t, summary.Conclusion, "completed")
	payload := summary.SummaryPayload
	require.Equal(t, demandID.String(), payload["demand_id"])
	require.Equal(t, "交付可验收结果", payload["original_goal"])
	require.Equal(t, string(project.ProjectDemandStatusCompleted), payload["status"])
	requirePayloadListContains(t, payload, "task_statuses", "task_id", taskID.String())
	requirePayloadListContains(t, payload, "completed_tasks", "task_id", taskID.String())
	requirePayloadListContains(t, payload, "evidence_refs", "id", "evidence-1")
	requirePayloadListContains(t, payload, "artifact_refs", "id", "artifact-1")
	requirePayloadListNotContains(t, payload, "artifact_refs", "id", "old-artifact")
	requirePayloadListContains(t, payload, "human_decision_refs", "decision_request_id", decisionRequestID.String())
	requirePayloadListContains(t, payload, "validation_results", "id", "acceptance-1")
	requirePayloadListContains(t, payload, "actual_verification", "summary", "真实接口 smoke 通过")
	requirePayloadListContains(t, payload, "changes", "summary", "补齐验收链路")
	requirePayloadListContains(t, payload, "remaining_risks", "summary", "仍需人工验收")
	requirePayloadListContains(t, payload, "suggested_next_steps", "summary", "负责人完成最终验收")
	require.Len(t, repo.decisionRequests, 1, "summary generation must not replace human-owned project acceptance")
}

func TestProjectStoreRequestProjectAcceptanceReviewSummarizesMoreThanOneHundredDemandTasks(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	demandID := uuid.New()
	jobID := uuid.New()
	routeID := uuid.New()
	var lastTaskID uuid.UUID
	tasks := make([]project.ProjectTask, 0, 105)
	for i := 0; i < 105; i++ {
		taskID := uuid.New()
		if i == 104 {
			lastTaskID = taskID
		}
		tasks = append(tasks, projectStoreTask(tenantID, projectID, demandID, jobID, routeID, taskID, project.ProjectTaskStatusCompleted))
	}
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{
			ID:               projectID,
			TenantID:         tenantID,
			Status:           project.ProjectStatusRunning,
			HumanOwnerUserID: ownerID,
		},
		demands: []project.ProjectDemand{{
			ID:        demandID,
			TenantID:  tenantID,
			ProjectID: projectID,
			Title:     "超过一百个任务的需求",
			Status:    project.ProjectDemandStatusCompleted,
		}},
		tasks: tasks,
	}
	store := NewProjectStoreWithApprovals(repo, &projectStoreApprovalCreator{})

	_, err := store.RequestProjectAcceptanceReview(context.Background(), RequestProjectAcceptanceReviewInput{
		TenantID: tenantID, ProjectID: projectID,
	})

	require.NoError(t, err)
	require.Len(t, repo.demandSummaries, 1)
	taskStatuses := payloadListItems(repo.demandSummaries[0].SummaryPayload["task_statuses"])
	require.Len(t, taskStatuses, 105)
	requirePayloadListContains(t, repo.demandSummaries[0].SummaryPayload, "task_statuses", "task_id", lastTaskID.String())
}

func TestProjectStoreRequestProjectAcceptanceReviewSkipsExistingDemandSummary(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	demandID := uuid.New()
	existingSummaryID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{
			ID:               projectID,
			TenantID:         tenantID,
			Status:           project.ProjectStatusRunning,
			HumanOwnerUserID: ownerID,
		},
		demands: []project.ProjectDemand{{
			ID:        demandID,
			TenantID:  tenantID,
			ProjectID: projectID,
			Title:     "已有总结",
			Status:    project.ProjectDemandStatusCompleted,
		}},
		demandSummaries: []project.ProjectDemandSummary{{
			ID:             existingSummaryID,
			TenantID:       tenantID,
			ProjectID:      projectID,
			DemandID:       demandID,
			Status:         string(project.ProjectDemandStatusCompleted),
			Conclusion:     "already summarized",
			SummaryPayload: map[string]any{"existing": true},
		}},
	}
	store := NewProjectStoreWithApprovals(repo, &projectStoreApprovalCreator{})

	_, err := store.RequestProjectAcceptanceReview(context.Background(), RequestProjectAcceptanceReviewInput{
		TenantID: tenantID, ProjectID: projectID,
	})

	require.NoError(t, err)
	require.Len(t, repo.demandSummaries, 1)
	require.Equal(t, existingSummaryID, repo.demandSummaries[0].ID)
	require.Empty(t, projectStoreEventsByType(repo.events, project.ProjectEventDemandSummaryCreated))
}

func TestProjectStoreRequestProjectAcceptanceReviewStopsWhenDemandSummaryCreationFails(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	demandID := uuid.New()
	summaryErr := errors.New("summary store unavailable")
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{
			ID:               projectID,
			TenantID:         tenantID,
			Status:           project.ProjectStatusRunning,
			HumanOwnerUserID: ownerID,
		},
		demands: []project.ProjectDemand{{
			ID:        demandID,
			TenantID:  tenantID,
			ProjectID: projectID,
			Title:     "总结失败",
			Status:    project.ProjectDemandStatusCompleted,
		}},
		createDemandSummaryErr: summaryErr,
	}
	store := NewProjectStoreWithApprovals(repo, &projectStoreApprovalCreator{})

	result, err := store.RequestProjectAcceptanceReview(context.Background(), RequestProjectAcceptanceReviewInput{
		TenantID: tenantID, ProjectID: projectID,
	})

	require.ErrorIs(t, err, summaryErr)
	require.Equal(t, uuid.Nil, result.ID)
	require.Equal(t, project.ProjectStatusRunning, repo.projectRecord.Status)
	require.Empty(t, repo.decisionRequests)
	require.Empty(t, repo.demandSummaries)
	require.Empty(t, projectStoreEventsByType(repo.events, project.ProjectEventDemandSummaryCreated))
}

func TestProjectStoreRequestProjectAcceptanceReviewSummarizesFailedAndCancelledDemandTasks(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	failedDemandID := uuid.New()
	cancelledDemandID := uuid.New()
	failedTaskID := uuid.New()
	cancelledTaskID := uuid.New()
	blockedTaskID := uuid.New()
	jobID := uuid.New()
	routeID := uuid.New()
	waitingRequestID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{
			ID:               projectID,
			TenantID:         tenantID,
			Status:           project.ProjectStatusRunning,
			HumanOwnerUserID: ownerID,
		},
		demands: []project.ProjectDemand{
			{ID: failedDemandID, TenantID: tenantID, ProjectID: projectID, Title: "失败需求", Status: project.ProjectDemandStatusFailed},
			{ID: cancelledDemandID, TenantID: tenantID, ProjectID: projectID, Title: "取消需求", Status: project.ProjectDemandStatusCancelled},
		},
		tasks: []project.ProjectTask{
			projectStoreTask(tenantID, projectID, failedDemandID, jobID, routeID, failedTaskID, project.ProjectTaskStatusFailed),
			projectStoreTask(tenantID, projectID, failedDemandID, jobID, routeID, blockedTaskID, project.ProjectTaskStatusWaitingHuman),
			projectStoreTask(tenantID, projectID, cancelledDemandID, jobID, routeID, cancelledTaskID, project.ProjectTaskStatusCancelled),
		},
	}
	repo.tasks[1].WaitingRequestID = &waitingRequestID
	failedResult := projectStoreTaskResult(tenantID, projectID, failedTaskID, project.TaskResultDecisionFailedRecovery, "failed")
	failedResult.Contract = project.TaskResultContract{
		Status:  project.TaskResultStatusFailed,
		Summary: "执行失败",
		Failure: &project.TaskResultFailure{ErrorFamily: "runtime", Message: "provider exited", RecoveryRecommendation: "人工确认是否重试"},
		Risks:   []project.TaskResultRisk{{Summary: "失败任务未恢复", Severity: "high"}},
		FollowUpRequests: []project.TaskResultFollowUpRequest{{
			Type:    "recovery",
			Summary: "判断是否重新规划",
		}},
	}
	repo.setTaskLatestResult(failedTaskID, failedResult)
	cancelledResult := projectStoreTaskResult(tenantID, projectID, cancelledTaskID, project.TaskResultDecisionCancelledTerminal, "cancelled")
	cancelledResult.Contract = project.TaskResultContract{
		Status:       project.TaskResultStatusCancelled,
		Summary:      "需求已取消",
		Cancellation: &project.TaskResultCancellation{Reason: "human ended demand", CancelledBy: "human_owner"},
	}
	repo.setTaskLatestResult(cancelledTaskID, cancelledResult)
	store := NewProjectStoreWithApprovals(repo, &projectStoreApprovalCreator{})

	_, err := store.RequestProjectAcceptanceReview(context.Background(), RequestProjectAcceptanceReviewInput{
		TenantID: tenantID, ProjectID: projectID,
	})

	require.NoError(t, err)
	require.Len(t, repo.demandSummaries, 2)
	failedSummary := requireProjectStoreDemandSummary(t, repo.demandSummaries, failedDemandID)
	require.Equal(t, string(project.ProjectDemandStatusFailed), failedSummary.Status)
	requirePayloadListContains(t, failedSummary.SummaryPayload, "unfinished_tasks", "task_id", failedTaskID.String())
	requirePayloadListContains(t, failedSummary.SummaryPayload, "unfinished_tasks", "task_id", blockedTaskID.String())
	requirePayloadListContains(t, failedSummary.SummaryPayload, "human_decision_refs", "decision_request_id", waitingRequestID.String())
	requirePayloadListContains(t, failedSummary.SummaryPayload, "remaining_risks", "summary", "失败任务未恢复")
	requirePayloadListContains(t, failedSummary.SummaryPayload, "suggested_next_steps", "summary", "判断是否重新规划")
	cancelledSummary := requireProjectStoreDemandSummary(t, repo.demandSummaries, cancelledDemandID)
	require.Equal(t, string(project.ProjectDemandStatusCancelled), cancelledSummary.Status)
	requirePayloadListContains(t, cancelledSummary.SummaryPayload, "unfinished_tasks", "task_id", cancelledTaskID.String())
	require.Contains(t, cancelledSummary.Conclusion, "cancelled")
}

func TestProjectStoreRequestProjectAcceptanceReviewRollsBackStatusWhenApprovalCreationFails(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	approvalErr := errors.New("approval service unavailable")
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{
			ID:               projectID,
			TenantID:         tenantID,
			Status:           project.ProjectStatusRunning,
			HumanOwnerUserID: ownerID,
		},
	}
	approvals := &projectStoreApprovalCreator{err: approvalErr}
	store := NewProjectStoreWithApprovals(repo, approvals)

	result, err := store.RequestProjectAcceptanceReview(context.Background(), RequestProjectAcceptanceReviewInput{
		TenantID:  tenantID,
		ProjectID: projectID,
	})

	require.ErrorIs(t, err, approvalErr)
	require.Equal(t, uuid.Nil, result.ID)
	require.Equal(t, project.ProjectStatusRunning, repo.projectRecord.Status)
	require.Empty(t, repo.decisionRequests)
}

func TestProjectStoreRequestProjectAcceptanceReviewRollsBackStatusWhenDecisionRequestCreationFails(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	decisionErr := errors.New("decision request store unavailable")
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{
			ID:               projectID,
			TenantID:         tenantID,
			Status:           project.ProjectStatusRunning,
			HumanOwnerUserID: ownerID,
		},
		createDecisionRequestErr: decisionErr,
	}
	approvals := &projectStoreApprovalCreator{}
	store := NewProjectStoreWithApprovals(repo, approvals)

	result, err := store.RequestProjectAcceptanceReview(context.Background(), RequestProjectAcceptanceReviewInput{
		TenantID:  tenantID,
		ProjectID: projectID,
	})

	require.ErrorIs(t, err, decisionErr)
	require.Equal(t, uuid.Nil, result.ID)
	require.Equal(t, project.ProjectStatusRunning, repo.projectRecord.Status)
	require.Len(t, repo.events, 1)
	require.Empty(t, repo.decisionRequests)
}

func TestProjectStoreLoadHumanDecisionRouteForPlanReview(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	planRevisionID := uuid.New()
	coordinationJobID := uuid.New()
	demandID := uuid.New()
	routeDecisionID := uuid.New()
	routeEventID := uuid.New()
	planEventID := uuid.New()
	repo := &projectStoreMemoryRepository{
		routeDecisions: []project.RouteDecision{{
			ID:                routeDecisionID,
			TenantID:          tenantID,
			ProjectID:         projectID,
			CoordinationJobID: coordinationJobID,
			CreatedEventID:    &routeEventID,
		}},
		decisionRequests: []project.DecisionRequest{{
			ID:                decisionID,
			TenantID:          tenantID,
			ProjectID:         projectID,
			CoordinationJobID: &coordinationJobID,
			PlanRevisionID:    &planRevisionID,
			DecisionType:      "plan_review",
			StatusSnapshot:    "resolved",
			CreatedEventID:    &planEventID,
		}},
		planRevisions: []project.PlanRevision{{
			ID:                planRevisionID,
			TenantID:          tenantID,
			ProjectID:         projectID,
			DemandID:          demandID,
			CoordinationJobID: &coordinationJobID,
			RouteDecisionID:   &routeDecisionID,
			Status:            project.PlanRevisionStatusPendingReview,
			PlanFingerprint:   "fingerprint",
			CreatedEventID:    &planEventID,
			Payload: map[string]any{
				"summary": "review me",
				"tasks":   []any{},
			},
		}},
	}
	store := NewProjectStore(repo)

	route, err := store.LoadHumanDecisionRoute(context.Background(), LoadHumanDecisionRouteInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
	})

	require.NoError(t, err)
	require.Equal(t, "plan_review", route.Decision.DecisionType)
	require.NotNil(t, route.PlanReview)
	require.Equal(t, planRevisionID, route.PlanReview.PlanRevisionID)
	require.Equal(t, demandID, route.PlanReview.DemandID)
	require.Equal(t, coordinationJobID, route.PlanReview.CoordinationJobID)
	require.Equal(t, routeDecisionID, route.PlanReview.RouteDecisionID)
	require.Equal(t, planEventID, route.PlanReview.PlanEventID)
	require.Equal(t, []uuid.UUID{routeEventID, planEventID}, route.PlanReview.OutputEventIDs)
}

func TestProjectStoreLoadHumanDecisionRouteForReplanAcceptUsesRevisionOrderAndEvents(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	coordinationJobID := uuid.New()
	firstDecisionID := uuid.New()
	secondDecisionID := uuid.New()
	firstPlanRevisionID := uuid.New()
	secondPlanRevisionID := uuid.New()
	firstRouteDecisionID := uuid.New()
	secondRouteDecisionID := uuid.New()
	firstRouteEventID := uuid.New()
	firstPlanEventID := uuid.New()
	firstResolvedEventID := uuid.New()
	secondRouteEventID := uuid.New()
	secondPlanEventID := uuid.New()
	secondDecisionCreatedEventID := uuid.New()
	repo := &projectStoreMemoryRepository{
		routeDecisions: []project.RouteDecision{
			{ID: firstRouteDecisionID, TenantID: tenantID, ProjectID: projectID, CoordinationJobID: coordinationJobID, CreatedEventID: &firstRouteEventID},
			{ID: secondRouteDecisionID, TenantID: tenantID, ProjectID: projectID, CoordinationJobID: coordinationJobID, CreatedEventID: &secondRouteEventID},
		},
		decisionRequests: []project.DecisionRequest{
			{
				ID:                secondDecisionID,
				TenantID:          tenantID,
				ProjectID:         projectID,
				CoordinationJobID: &coordinationJobID,
				PlanRevisionID:    &secondPlanRevisionID,
				DecisionType:      "plan_review",
				StatusSnapshot:    "resolved",
				CreatedEventID:    &secondDecisionCreatedEventID,
			},
			{
				ID:                firstDecisionID,
				TenantID:          tenantID,
				ProjectID:         projectID,
				CoordinationJobID: &coordinationJobID,
				PlanRevisionID:    &firstPlanRevisionID,
				DecisionType:      "plan_review",
				StatusSnapshot:    "request_changes",
				ResolvedEventID:   &firstResolvedEventID,
			},
		},
		planRevisions: []project.PlanRevision{
			{
				ID:                secondPlanRevisionID,
				TenantID:          tenantID,
				ProjectID:         projectID,
				DemandID:          demandID,
				CoordinationJobID: &coordinationJobID,
				RouteDecisionID:   &secondRouteDecisionID,
				RevisionNumber:    2,
				Status:            project.PlanRevisionStatusPendingReview,
				PlanFingerprint:   "second-fingerprint",
				CreatedEventID:    &secondPlanEventID,
				Payload:           map[string]any{"summary": "second", "tasks": []any{}},
			},
			{
				ID:                firstPlanRevisionID,
				TenantID:          tenantID,
				ProjectID:         projectID,
				DemandID:          demandID,
				CoordinationJobID: &coordinationJobID,
				RouteDecisionID:   &firstRouteDecisionID,
				RevisionNumber:    1,
				Status:            project.PlanRevisionStatusSuperseded,
				PlanFingerprint:   "first-fingerprint",
				CreatedEventID:    &firstPlanEventID,
				Payload:           map[string]any{"summary": "first", "tasks": []any{}},
			},
		},
	}
	store := NewProjectStore(repo)

	route, err := store.LoadHumanDecisionRoute(context.Background(), LoadHumanDecisionRouteInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: secondDecisionID,
	})

	require.NoError(t, err)
	require.NotNil(t, route.PlanReview)
	require.Equal(t, secondPlanEventID, route.PlanReview.PlanEventID)
	require.Equal(t, []uuid.UUID{
		firstRouteEventID,
		firstPlanEventID,
		firstResolvedEventID,
		secondRouteEventID,
		secondPlanEventID,
	}, route.PlanReview.OutputEventIDs)
}

func TestProjectStoreLoadHumanDecisionRouteMissingDecisionReturnsZeroRoute(t *testing.T) {
	store := NewProjectStore(&projectStoreMemoryRepository{})

	route, err := store.LoadHumanDecisionRoute(context.Background(), LoadHumanDecisionRouteInput{
		TenantID:          uuid.New(),
		ProjectID:         uuid.New(),
		DecisionRequestID: uuid.New(),
	})

	require.NoError(t, err)
	require.Equal(t, uuid.Nil, route.Decision.ID)
	require.Nil(t, route.PlanReview)
}

func TestProjectStoreLoadHumanDecisionRouteForNonPlanReviewReturnsDecisionOnly(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	projectTaskID := uuid.New()
	repo := &projectStoreMemoryRepository{
		decisionRequests: []project.DecisionRequest{{
			ID:             decisionID,
			TenantID:       tenantID,
			ProjectID:      projectID,
			ProjectTaskID:  &projectTaskID,
			DecisionType:   "task_failure_recovery",
			StatusSnapshot: "pending",
		}},
	}
	store := NewProjectStore(repo)

	route, err := store.LoadHumanDecisionRoute(context.Background(), LoadHumanDecisionRouteInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
	})

	require.NoError(t, err)
	require.Equal(t, decisionID, route.Decision.ID)
	require.Equal(t, projectID, route.Decision.ProjectID)
	require.Equal(t, "task_failure_recovery", route.Decision.DecisionType)
	require.Equal(t, "pending", route.Decision.StatusSnapshot)
	require.Equal(t, projectTaskID, route.Decision.ProjectTaskID)
	require.Nil(t, route.PlanReview)
}

func TestProjectStoreLoadHumanDecisionRouteForPlanReviewRequiresPlanRevisionID(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	repo := &projectStoreMemoryRepository{
		decisionRequests: []project.DecisionRequest{{
			ID:             decisionID,
			TenantID:       tenantID,
			ProjectID:      projectID,
			DecisionType:   "plan_review",
			StatusSnapshot: "resolved",
		}},
	}
	store := NewProjectStore(repo)

	_, err := store.LoadHumanDecisionRoute(context.Background(), LoadHumanDecisionRouteInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
	})

	require.ErrorIs(t, err, project.ErrInvalidProject)
}

func TestProjectStoreLoadHumanDecisionRouteForPlanReviewMissingRevisionReturnsNotFound(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	planRevisionID := uuid.New()
	repo := &projectStoreMemoryRepository{
		decisionRequests: []project.DecisionRequest{{
			ID:             decisionID,
			TenantID:       tenantID,
			ProjectID:      projectID,
			PlanRevisionID: &planRevisionID,
			DecisionType:   "plan_review",
			StatusSnapshot: "resolved",
		}},
	}
	store := NewProjectStore(repo)

	_, err := store.LoadHumanDecisionRoute(context.Background(), LoadHumanDecisionRouteInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
	})

	require.ErrorIs(t, err, project.ErrProjectNotFound)
}

func TestProjectStoreLoadHumanDecisionRouteForPlanReviewRequiresRevisionRouteIDs(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	planRevisionID := uuid.New()
	coordinationJobID := uuid.New()
	routeDecisionID := uuid.New()
	demandID := uuid.New()

	tests := []struct {
		name              string
		coordinationJobID *uuid.UUID
		routeDecisionID   *uuid.UUID
	}{
		{name: "missing coordination job", coordinationJobID: nil, routeDecisionID: &routeDecisionID},
		{name: "missing route decision", coordinationJobID: &coordinationJobID, routeDecisionID: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &projectStoreMemoryRepository{
				decisionRequests: []project.DecisionRequest{{
					ID:             decisionID,
					TenantID:       tenantID,
					ProjectID:      projectID,
					PlanRevisionID: &planRevisionID,
					DecisionType:   "plan_review",
					StatusSnapshot: "resolved",
				}},
				planRevisions: []project.PlanRevision{{
					ID:                planRevisionID,
					TenantID:          tenantID,
					ProjectID:         projectID,
					DemandID:          demandID,
					CoordinationJobID: tt.coordinationJobID,
					RouteDecisionID:   tt.routeDecisionID,
					Status:            project.PlanRevisionStatusPendingReview,
					PlanFingerprint:   "fingerprint",
					Payload: map[string]any{
						"summary": "review me",
						"tasks":   []any{},
					},
				}},
			}
			store := NewProjectStore(repo)

			_, err := store.LoadHumanDecisionRoute(context.Background(), LoadHumanDecisionRouteInput{
				TenantID:          tenantID,
				ProjectID:         projectID,
				DecisionRequestID: decisionID,
			})

			require.ErrorIs(t, err, project.ErrInvalidProject)
		})
	}
}

func TestProjectStoreLoadHumanDecisionRouteForPlanReviewInvalidPayloadReturnsError(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	planRevisionID := uuid.New()
	coordinationJobID := uuid.New()
	routeDecisionID := uuid.New()
	demandID := uuid.New()
	repo := &projectStoreMemoryRepository{
		decisionRequests: []project.DecisionRequest{{
			ID:             decisionID,
			TenantID:       tenantID,
			ProjectID:      projectID,
			PlanRevisionID: &planRevisionID,
			DecisionType:   "plan_review",
			StatusSnapshot: "resolved",
		}},
		planRevisions: []project.PlanRevision{{
			ID:                planRevisionID,
			TenantID:          tenantID,
			ProjectID:         projectID,
			DemandID:          demandID,
			CoordinationJobID: &coordinationJobID,
			RouteDecisionID:   &routeDecisionID,
			Status:            project.PlanRevisionStatusPendingReview,
			PlanFingerprint:   "fingerprint",
			Payload: map[string]any{
				"bad": func() {},
			},
		}},
	}
	store := NewProjectStore(repo)

	_, err := store.LoadHumanDecisionRoute(context.Background(), LoadHumanDecisionRouteInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
	})

	require.Error(t, err)
}

func TestProjectStoreApplyProjectAcceptanceDecisionAcceptArchivesRejectReopens(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	decisionRequestID := uuid.New()

	t.Run("accepted archives project", func(t *testing.T) {
		repo := &projectStoreMemoryRepository{
			projectRecord: project.Project{ID: projectID, TenantID: tenantID, Status: project.ProjectStatusAcceptance, HumanOwnerUserID: ownerID},
		}
		store := NewProjectStore(repo)
		err := store.ApplyProjectAcceptanceDecision(context.Background(), ApplyProjectAcceptanceDecisionInput{
			TenantID: tenantID, ProjectID: projectID, DecisionRequestID: decisionRequestID, Decision: "approved",
		})
		require.NoError(t, err)
		require.Equal(t, project.ProjectStatusArchived, repo.projectRecord.Status)
		require.Len(t, repo.acceptanceRecords, 1)
		require.Equal(t, "accepted", repo.acceptanceRecords[0].Status)
	})

	t.Run("rejected reopens to running", func(t *testing.T) {
		repo := &projectStoreMemoryRepository{
			projectRecord: project.Project{ID: projectID, TenantID: tenantID, Status: project.ProjectStatusAcceptance, HumanOwnerUserID: ownerID},
		}
		store := NewProjectStore(repo)
		err := store.ApplyProjectAcceptanceDecision(context.Background(), ApplyProjectAcceptanceDecisionInput{
			TenantID: tenantID, ProjectID: projectID, DecisionRequestID: decisionRequestID, Decision: "rejected",
		})
		require.NoError(t, err)
		require.Equal(t, project.ProjectStatusRunning, repo.projectRecord.Status)
		require.Len(t, repo.acceptanceRecords, 1)
		require.Equal(t, "rejected", repo.acceptanceRecords[0].Status)
	})
}

func TestProjectStoreHoldDownstreamForFailureBlocksRecursiveDownstreamAndCreatesDecision(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	jobID := uuid.New()
	routeID := uuid.New()
	demandID := uuid.New()
	failedTaskID := uuid.New()
	firstDownstreamID := uuid.New()
	secondDownstreamID := uuid.New()
	completedDownstreamID := uuid.New()
	cancelledDownstreamID := uuid.New()
	approvalID := uuid.New()
	failedEventID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{
			ID:               projectID,
			TenantID:         tenantID,
			HumanOwnerUserID: ownerID,
		},
		tasks: []project.ProjectTask{
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, failedTaskID, "failed"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, firstDownstreamID, "planned"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, secondDownstreamID, "pending"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, completedDownstreamID, "completed"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, cancelledDownstreamID, "cancelled"),
		},
		taskDependencies: []project.ProjectTaskDependency{
			projectStoreDependency(tenantID, projectID, jobID, firstDownstreamID, failedTaskID),
			projectStoreDependency(tenantID, projectID, jobID, completedDownstreamID, failedTaskID),
			projectStoreDependency(tenantID, projectID, jobID, secondDownstreamID, firstDownstreamID),
			projectStoreDependency(tenantID, projectID, jobID, cancelledDownstreamID, firstDownstreamID),
		},
		approvalID: approvalID,
	}
	approvals := &projectStoreApprovalCreator{approvalID: approvalID}
	inbox := &projectStoreDecisionInboxProjector{}
	store := NewProjectStoreWithApprovalsAndInbox(repo, approvals, inbox)

	result, err := store.HoldDownstreamForFailure(context.Background(), HoldDownstreamForFailureInput{
		TenantID:       tenantID,
		ProjectID:      projectID,
		FailedTaskID:   failedTaskID,
		FailureSummary: "runtime execution failed",
		FailedEventID:  failedEventID,
	})

	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, result.ID)
	require.Equal(t, "blocked", repo.taskStatus(firstDownstreamID))
	require.Equal(t, "blocked", repo.taskStatus(secondDownstreamID))
	require.Equal(t, "completed", repo.taskStatus(completedDownstreamID))
	require.Equal(t, "cancelled", repo.taskStatus(cancelledDownstreamID))
	require.Equal(t, []projectTaskStatusUpdateRecord{
		{TenantID: tenantID, TaskID: firstDownstreamID, Status: "blocked", CurrentStatuses: []string{"planned", "pending", "assigned", "running", "waiting_human"}},
		{TenantID: tenantID, TaskID: secondDownstreamID, Status: "blocked", CurrentStatuses: []string{"planned", "pending", "assigned", "running", "waiting_human"}},
	}, repo.statusUpdates)
	require.Equal(t, ownerID, approvals.last.TargetUserID)
	require.Equal(t, failedTaskID, approvals.last.ResourceID)
	require.Equal(t, "task_failure_recovery", approvals.last.DecisionType)
	require.Equal(t, "runtime execution failed", approvals.last.ContextPayload["failure_summary"])
	require.Equal(t, projectID.String(), approvals.last.ContextPayload["project_id"])
	require.Equal(t, failedTaskID.String(), approvals.last.ContextPayload["failed_task_id"])
	require.Len(t, repo.events, 1)
	require.Equal(t, project.ProjectEventDecisionRequested, repo.events[0].EventType)
	require.Len(t, repo.decisionRequests, 1)
	decision := repo.decisionRequests[0]
	require.Equal(t, approvalID, decision.ApprovalRequestID)
	require.Equal(t, ownerID, decision.TargetUserID)
	require.Equal(t, "task_failure_recovery", decision.DecisionType)
	require.Equal(t, "pending", decision.StatusSnapshot)
	require.NotNil(t, decision.ProjectTaskID)
	require.Equal(t, failedTaskID, *decision.ProjectTaskID)
	require.Len(t, inbox.upserts, 1)
	require.Equal(t, decision.ID, inbox.upserts[0].ID)
	require.Equal(t, failedTaskID, *inbox.upserts[0].ProjectTaskID)
}

func TestProjectStoreRequestProjectTaskIterationExhaustedReviewCreatesDedicatedDecision(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	jobID := uuid.New()
	routeID := uuid.New()
	demandID := uuid.New()
	sourceTaskID := uuid.New()
	downstreamID := uuid.New()
	resultID := uuid.New()
	eventID := uuid.New()
	approvalID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{
			ID:               projectID,
			TenantID:         tenantID,
			HumanOwnerUserID: ownerID,
		},
		tasks: []project.ProjectTask{
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, sourceTaskID, "completed"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, downstreamID, "planned"),
		},
		taskDependencies: []project.ProjectTaskDependency{
			projectStoreDependency(tenantID, projectID, jobID, downstreamID, sourceTaskID),
		},
		approvalID: approvalID,
	}
	approvals := &projectStoreApprovalCreator{approvalID: approvalID}
	inbox := &projectStoreDecisionInboxProjector{}
	store := NewProjectStoreWithApprovalsAndInbox(repo, approvals, inbox)

	result, err := store.RequestProjectTaskIterationExhaustedReview(context.Background(), RequestProjectTaskIterationExhaustedReviewInput{
		TenantID:       tenantID,
		ProjectID:      projectID,
		ProjectTaskID:  sourceTaskID,
		ResultID:       resultID,
		Reason:         "iteration_exhausted",
		Summary:        "同一失败重复出现，需要人类判断是否继续",
		CreatedEventID: eventID,
	})

	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, result.ID)
	require.Equal(t, "blocked", repo.taskStatus(downstreamID))
	require.Equal(t, ownerID, approvals.last.TargetUserID)
	require.Equal(t, sourceTaskID, approvals.last.ResourceID)
	require.Equal(t, "project_task_iteration_exhausted", approvals.last.DecisionType)
	require.Equal(t, "iteration_exhausted", approvals.last.ContextPayload["reason"])
	require.Equal(t, "同一失败重复出现，需要人类判断是否继续", approvals.last.ContextPayload["summary"])
	require.Equal(t, resultID.String(), approvals.last.ContextPayload["result_id"])
	require.Len(t, repo.events, 1)
	require.Equal(t, project.ProjectEventDecisionRequested, repo.events[0].EventType)
	require.Len(t, repo.decisionRequests, 1)
	decision := repo.decisionRequests[0]
	require.Equal(t, approvalID, decision.ApprovalRequestID)
	require.Equal(t, "project_task_iteration_exhausted", decision.DecisionType)
	require.NotNil(t, decision.SummarySnapshot)
	require.Equal(t, "同一失败重复出现，需要人类判断是否继续", *decision.SummarySnapshot)
	require.NotNil(t, decision.ProjectTaskID)
	require.Equal(t, sourceTaskID, *decision.ProjectTaskID)
	require.Len(t, inbox.upserts, 1)
	require.Equal(t, decision.ID, inbox.upserts[0].ID)
}

func TestParseFailureRecoveryAction(t *testing.T) {
	newEmployeeID := uuid.New()
	tests := []struct {
		name      string
		decision  string
		payload   map[string]any
		want      FailureRecoveryAction
		wantError error
	}{
		{
			name:     "needs more evidence does not mutate recovery graph",
			decision: "needs_more_evidence",
			want:     FailureRecoveryAction{Action: "needs_more_evidence"},
		},
		{
			name:     "rejected cancels downstream",
			decision: "rejected",
			want:     FailureRecoveryAction{Action: "cancel_downstream"},
		},
		{
			name:     "approved retry",
			decision: "approved",
			payload:  map[string]any{"recovery_action": "retry"},
			want:     FailureRecoveryAction{Action: "retry"},
		},
		{
			name:     "approved cancel downstream",
			decision: "approved",
			payload:  map[string]any{"recovery_action": "cancel_downstream"},
			want:     FailureRecoveryAction{Action: "cancel_downstream"},
		},
		{
			name:     "approved reassign",
			decision: "approved",
			payload: map[string]any{
				"recovery_action":         "reassign",
				"new_digital_employee_id": newEmployeeID.String(),
			},
			want: FailureRecoveryAction{Action: "reassign", NewDigitalEmployeeID: &newEmployeeID},
		},
		{
			name:      "approved reassign requires employee id",
			decision:  "approved",
			payload:   map[string]any{"recovery_action": "reassign"},
			wantError: project.ErrInvalidProject,
		},
		{
			name:      "approved unknown action rejected",
			decision:  "approved",
			payload:   map[string]any{"recovery_action": "replace_subgraph"},
			wantError: project.ErrInvalidProject,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFailureRecoveryAction(tt.decision, tt.payload)
			if tt.wantError != nil {
				require.ErrorIs(t, err, tt.wantError)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want.Action, got.Action)
			if tt.want.NewDigitalEmployeeID == nil {
				require.Nil(t, got.NewDigitalEmployeeID)
			} else {
				require.NotNil(t, got.NewDigitalEmployeeID)
				require.Equal(t, *tt.want.NewDigitalEmployeeID, *got.NewDigitalEmployeeID)
			}
		})
	}
}

func TestApplyFailureRecoveryRetryCreatesAppendOnlySubgraph(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	jobID := uuid.New()
	routeID := uuid.New()
	employeeID := uuid.New()
	failedTaskID := uuid.New()
	downstreamID := uuid.New()
	decisionID := uuid.New()
	stageIndex := int32(1)
	failedTaskIDPtr := failedTaskID
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		tasks: []project.ProjectTask{
			{
				ID:                        failedTaskID,
				TenantID:                  tenantID,
				ProjectID:                 projectID,
				DemandID:                  &demandID,
				Title:                     "分析问题",
				Summary:                   strPtr("整理失败原因"),
				Status:                    "failed",
				AssignedDigitalEmployeeID: &employeeID,
				RiskLevel:                 strPtr("high"),
				RequiresHumanApproval:     true,
				CoordinationJobID:         &jobID,
				RouteDecisionID:           &routeID,
				PlannedTaskKey:            strPtr("A#1"),
				TaskKind:                  strPtr("analysis"),
				StageIndex:                &stageIndex,
				ExpectedOutputs:           []any{"execution_summary", "evidence_refs"},
				InputRequirements:         map[string]any{"scope": "logs"},
				HandoffContract:           map[string]any{"format": "markdown"},
				PlannerMetadata:           map[string]any{"provider": "deepseek"},
			},
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, downstreamID, "blocked"),
		},
		taskDependencies: []project.ProjectTaskDependency{
			projectStoreDependency(tenantID, projectID, jobID, downstreamID, failedTaskID),
		},
		decisionRequests: []project.DecisionRequest{{
			ID:             decisionID,
			TenantID:       tenantID,
			ProjectID:      projectID,
			ProjectTaskID:  &failedTaskIDPtr,
			DecisionType:   "task_failure_recovery",
			StatusSnapshot: "pending",
		}},
	}
	store := NewProjectStore(repo)

	result, err := store.ApplyFailureRecoveryDecision(context.Background(), ApplyFailureRecoveryDecisionInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		Decision:          "approved",
		Payload:           map[string]any{"recovery_action": "retry"},
	})

	require.NoError(t, err)
	replacement := requireRecoveryReplacementTask(t, repo, failedTaskID)
	require.NotEqual(t, failedTaskID, replacement.ID)
	require.Equal(t, []uuid.UUID{replacement.ID}, result.ReadyTaskIDs)
	require.Contains(t, replacement.Title, "重试")
	require.Equal(t, "planned", replacement.Status)
	require.Equal(t, employeeID, *replacement.AssignedDigitalEmployeeID)
	require.Equal(t, []any{"execution_summary", "evidence_refs"}, replacement.ExpectedOutputs)
	require.Equal(t, map[string]any{"scope": "logs"}, replacement.InputRequirements)
	require.Equal(t, map[string]any{"format": "markdown"}, replacement.HandoffContract)
	require.Equal(t, "retry", replacement.PlannerMetadata["recovery_action"])
	require.Equal(t, failedTaskID.String(), replacement.PlannerMetadata["source_task_id"])
	require.Equal(t, jobID.String(), replacement.PlannerMetadata["parent_coordination_job_id"])
	requireDependency(t, repo.taskDependencies, downstreamID, replacement.ID)
	requireNoDependency(t, repo.taskDependencies, downstreamID, failedTaskID)

	repo.setTaskStatus(replacement.ID, "completed")
	repo.setTaskLatestResult(replacement.ID, projectStoreTaskResult(tenantID, projectID, replacement.ID, project.TaskResultDecisionCompleteAccepted, "accepted"))
	ready, err := store.ResolveReadyDownstream(context.Background(), ResolveReadyDownstreamInput{
		TenantID:        tenantID,
		ProjectID:       projectID,
		CompletedTaskID: replacement.ID,
	})
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{downstreamID}, ready)
	require.Equal(t, "planned", repo.taskStatus(downstreamID))

	secondResult, err := store.ApplyFailureRecoveryDecision(context.Background(), ApplyFailureRecoveryDecisionInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		Decision:          "approved",
		Payload:           map[string]any{"recovery_action": "retry"},
	})
	require.NoError(t, err)
	require.Empty(t, secondResult.ReadyTaskIDs)
	require.Len(t, recoveryReplacementTasks(repo, failedTaskID), 1)
}

func TestApplyFailureRecoveryRetryReturnsNoReadyIDsWhenReplacementBlocked(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	jobID := uuid.New()
	routeID := uuid.New()
	employeeID := uuid.New()
	failedTaskID := uuid.New()
	blockerID := uuid.New()
	decisionID := uuid.New()
	failedTaskIDPtr := failedTaskID
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		tasks: []project.ProjectTask{
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, blockerID, "planned"),
			{
				ID:                        failedTaskID,
				TenantID:                  tenantID,
				ProjectID:                 projectID,
				DemandID:                  &demandID,
				Title:                     "分析问题",
				Status:                    "failed",
				AssignedDigitalEmployeeID: &employeeID,
				CoordinationJobID:         &jobID,
				RouteDecisionID:           &routeID,
				PlannedTaskKey:            strPtr("A#1"),
				ExpectedOutputs:           []any{"execution_summary"},
				InputRequirements:         map[string]any{},
				HandoffContract:           map[string]any{},
				PlannerMetadata:           map[string]any{},
			},
		},
		taskDependencies: []project.ProjectTaskDependency{
			projectStoreDependency(tenantID, projectID, jobID, failedTaskID, blockerID),
		},
		decisionRequests: []project.DecisionRequest{{
			ID:             decisionID,
			TenantID:       tenantID,
			ProjectID:      projectID,
			ProjectTaskID:  &failedTaskIDPtr,
			DecisionType:   "task_failure_recovery",
			StatusSnapshot: "pending",
		}},
	}
	store := NewProjectStore(repo)

	result, err := store.ApplyFailureRecoveryDecision(context.Background(), ApplyFailureRecoveryDecisionInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		Decision:          "approved",
		Payload:           map[string]any{"recovery_action": "retry"},
	})

	require.NoError(t, err)
	replacement := requireRecoveryReplacementTask(t, repo, failedTaskID)
	require.Equal(t, "blocked", replacement.Status)
	require.Empty(t, result.ReadyTaskIDs)
}

func TestApplyFailureRecoveryReassignRequiresNewEmployee(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	jobID := uuid.New()
	routeID := uuid.New()
	originalEmployeeID := uuid.New()
	newEmployeeID := uuid.New()
	failedTaskID := uuid.New()
	decisionID := uuid.New()
	failedTaskIDPtr := failedTaskID
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		members: []project.ProjectMember{{
			ID:            uuid.New(),
			TenantID:      tenantID,
			ProjectID:     projectID,
			PrincipalType: project.PrincipalTypeDigitalEmployee,
			PrincipalID:   newEmployeeID,
			ProjectRole:   project.ProjectRoleExecutor,
			Status:        "active",
		}},
		tasks: []project.ProjectTask{{
			ID:                        failedTaskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "修复问题",
			Status:                    "failed",
			AssignedDigitalEmployeeID: &originalEmployeeID,
			CoordinationJobID:         &jobID,
			RouteDecisionID:           &routeID,
			PlannedTaskKey:            strPtr("repair"),
			ExpectedOutputs:           []any{"execution_summary"},
			InputRequirements:         map[string]any{},
			HandoffContract:           map[string]any{},
			PlannerMetadata:           map[string]any{},
		}},
		decisionRequests: []project.DecisionRequest{{
			ID:             decisionID,
			TenantID:       tenantID,
			ProjectID:      projectID,
			ProjectTaskID:  &failedTaskIDPtr,
			DecisionType:   "task_failure_recovery",
			StatusSnapshot: "pending",
		}},
	}
	store := NewProjectStore(repo)

	_, err := store.ApplyFailureRecoveryDecision(context.Background(), ApplyFailureRecoveryDecisionInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		Decision:          "approved",
		Payload:           map[string]any{"recovery_action": "reassign"},
	})
	require.ErrorIs(t, err, project.ErrInvalidProject)
	require.Len(t, repo.tasks, 1)

	result, err := store.ApplyFailureRecoveryDecision(context.Background(), ApplyFailureRecoveryDecisionInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		Decision:          "approved",
		Payload: map[string]any{
			"recovery_action":         "reassign",
			"new_digital_employee_id": newEmployeeID.String(),
		},
	})
	require.NoError(t, err)
	replacement := requireRecoveryReplacementTask(t, repo, failedTaskID)
	require.Equal(t, []uuid.UUID{replacement.ID}, result.ReadyTaskIDs)
	require.Equal(t, "reassign", replacement.PlannerMetadata["recovery_action"])
	require.Equal(t, newEmployeeID, *replacement.AssignedDigitalEmployeeID)
}

func TestApplyFailureRecoveryRetryRejectsNilCoordinationJob(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	routeID := uuid.New()
	employeeID := uuid.New()
	failedTaskID := uuid.New()
	decisionID := uuid.New()
	failedTaskIDPtr := failedTaskID
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		tasks: []project.ProjectTask{{
			ID:                        failedTaskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "修复问题",
			Status:                    "failed",
			AssignedDigitalEmployeeID: &employeeID,
			RouteDecisionID:           &routeID,
			PlannedTaskKey:            strPtr("repair"),
			ExpectedOutputs:           []any{"execution_summary"},
			InputRequirements:         map[string]any{},
			HandoffContract:           map[string]any{},
			PlannerMetadata:           map[string]any{},
		}},
		decisionRequests: []project.DecisionRequest{{
			ID:             decisionID,
			TenantID:       tenantID,
			ProjectID:      projectID,
			ProjectTaskID:  &failedTaskIDPtr,
			DecisionType:   "task_failure_recovery",
			StatusSnapshot: "pending",
		}},
	}
	store := NewProjectStore(repo)

	_, err := store.ApplyFailureRecoveryDecision(context.Background(), ApplyFailureRecoveryDecisionInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		Decision:          "approved",
		Payload:           map[string]any{"recovery_action": "retry"},
	})

	require.ErrorIs(t, err, project.ErrInvalidProject)
	require.Len(t, repo.tasks, 1)
	require.Empty(t, recoveryReplacementTasks(repo, failedTaskID))
}

func TestApplyFailureRecoveryReassignRejectsNilCoordinationJob(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	routeID := uuid.New()
	employeeID := uuid.New()
	newEmployeeID := uuid.New()
	failedTaskID := uuid.New()
	decisionID := uuid.New()
	failedTaskIDPtr := failedTaskID
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		members: []project.ProjectMember{{
			ID:            uuid.New(),
			TenantID:      tenantID,
			ProjectID:     projectID,
			PrincipalType: project.PrincipalTypeDigitalEmployee,
			PrincipalID:   newEmployeeID,
			ProjectRole:   project.ProjectRoleExecutor,
			Status:        "active",
		}},
		tasks: []project.ProjectTask{{
			ID:                        failedTaskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "修复问题",
			Status:                    "failed",
			AssignedDigitalEmployeeID: &employeeID,
			RouteDecisionID:           &routeID,
			PlannedTaskKey:            strPtr("repair"),
			ExpectedOutputs:           []any{"execution_summary"},
			InputRequirements:         map[string]any{},
			HandoffContract:           map[string]any{},
			PlannerMetadata:           map[string]any{},
		}},
		decisionRequests: []project.DecisionRequest{{
			ID:             decisionID,
			TenantID:       tenantID,
			ProjectID:      projectID,
			ProjectTaskID:  &failedTaskIDPtr,
			DecisionType:   "task_failure_recovery",
			StatusSnapshot: "pending",
		}},
	}
	store := NewProjectStore(repo)

	_, err := store.ApplyFailureRecoveryDecision(context.Background(), ApplyFailureRecoveryDecisionInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		Decision:          "approved",
		Payload: map[string]any{
			"recovery_action":         "reassign",
			"new_digital_employee_id": newEmployeeID.String(),
		},
	})

	require.ErrorIs(t, err, project.ErrInvalidProject)
	require.Len(t, repo.tasks, 1)
	require.Empty(t, recoveryReplacementTasks(repo, failedTaskID))
}

func TestApplyFailureRecoveryCancelDownstreamCancelsBlockedDependents(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	jobID := uuid.New()
	routeID := uuid.New()
	failedTaskID := uuid.New()
	blockedID := uuid.New()
	plannedID := uuid.New()
	pendingID := uuid.New()
	completedID := uuid.New()
	failedDownstreamID := uuid.New()
	cancelledID := uuid.New()
	decisionID := uuid.New()
	failedTaskIDPtr := failedTaskID
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		tasks: []project.ProjectTask{
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, failedTaskID, "failed"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, blockedID, "blocked"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, plannedID, "planned"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, pendingID, "pending"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, completedID, "completed"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, failedDownstreamID, "failed"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, cancelledID, "cancelled"),
		},
		taskDependencies: []project.ProjectTaskDependency{
			projectStoreDependency(tenantID, projectID, jobID, blockedID, failedTaskID),
			projectStoreDependency(tenantID, projectID, jobID, plannedID, failedTaskID),
			projectStoreDependency(tenantID, projectID, jobID, pendingID, blockedID),
			projectStoreDependency(tenantID, projectID, jobID, completedID, failedTaskID),
			projectStoreDependency(tenantID, projectID, jobID, failedDownstreamID, failedTaskID),
			projectStoreDependency(tenantID, projectID, jobID, cancelledID, blockedID),
		},
		decisionRequests: []project.DecisionRequest{{
			ID:             decisionID,
			TenantID:       tenantID,
			ProjectID:      projectID,
			ProjectTaskID:  &failedTaskIDPtr,
			DecisionType:   "task_failure_recovery",
			StatusSnapshot: "pending",
		}},
	}
	store := NewProjectStore(repo)

	_, err := store.ApplyFailureRecoveryDecision(context.Background(), ApplyFailureRecoveryDecisionInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		Decision:          "rejected",
	})

	require.NoError(t, err)
	require.Equal(t, "cancelled", repo.taskStatus(blockedID))
	require.Equal(t, "cancelled", repo.taskStatus(plannedID))
	require.Equal(t, "cancelled", repo.taskStatus(pendingID))
	require.Equal(t, "completed", repo.taskStatus(completedID))
	require.Equal(t, "failed", repo.taskStatus(failedDownstreamID))
	require.Equal(t, "cancelled", repo.taskStatus(cancelledID))
	require.Len(t, eventsByType(repo.events, project.ProjectEventTaskCancelled), 4)

	_, err = store.ApplyFailureRecoveryDecision(context.Background(), ApplyFailureRecoveryDecisionInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		Decision:          "rejected",
	})
	require.NoError(t, err)
	require.Len(t, eventsByType(repo.events, project.ProjectEventTaskCancelled), 4)
}

func TestApplyFailureRecoveryCancelDownstreamRepairsMissingAuditEventOnRetry(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	jobID := uuid.New()
	routeID := uuid.New()
	failedTaskID := uuid.New()
	blockedID := uuid.New()
	decisionID := uuid.New()
	failedTaskIDPtr := failedTaskID
	eventErr := errors.New("event store unavailable")
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		tasks: []project.ProjectTask{
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, failedTaskID, "failed"),
			projectStoreTask(tenantID, projectID, demandID, jobID, routeID, blockedID, "blocked"),
		},
		taskDependencies: []project.ProjectTaskDependency{
			projectStoreDependency(tenantID, projectID, jobID, blockedID, failedTaskID),
		},
		decisionRequests: []project.DecisionRequest{{
			ID:             decisionID,
			TenantID:       tenantID,
			ProjectID:      projectID,
			ProjectTaskID:  &failedTaskIDPtr,
			DecisionType:   "task_failure_recovery",
			StatusSnapshot: "pending",
		}},
		appendProjectEventErr: eventErr,
	}
	store := NewProjectStore(repo)
	input := ApplyFailureRecoveryDecisionInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		Decision:          "rejected",
	}

	_, err := store.ApplyFailureRecoveryDecision(context.Background(), input)
	require.ErrorIs(t, err, eventErr)
	require.Equal(t, "cancelled", repo.taskStatus(blockedID))
	require.Empty(t, eventsByType(repo.events, project.ProjectEventTaskCancelled))

	repo.appendProjectEventErr = nil
	_, err = store.ApplyFailureRecoveryDecision(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, "cancelled", repo.taskStatus(blockedID))
	require.Len(t, eventsByType(repo.events, project.ProjectEventTaskCancelled), 1)

	_, err = store.ApplyFailureRecoveryDecision(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, eventsByType(repo.events, project.ProjectEventTaskCancelled), 1)
}

func TestProjectStoreDispatchProjectTaskStartsRunAndQueuesTask(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	ownerID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID},
		demand: project.ProjectDemand{
			ID:        demandID,
			TenantID:  tenantID,
			ProjectID: projectID,
			Title:     "检查上线证据",
			Content:   strPtr("需要确认测试报告和回滚方案。"),
		},
		members: []project.ProjectMember{projectStoreExecutorMember(tenantID, projectID, employeeID)},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "整理证据",
			Summary:                   strPtr("输出证据清单"),
			Status:                    "planned",
			AssignedDigitalEmployeeID: &employeeID,
			ExpectedOutputs:           []any{"execution_summary", "evidence_refs"},
			InputRequirements:         map[string]any{"required_context": []any{"test_report", "rollback_plan"}},
			HandoffContract:           map[string]any{"required_refs": []any{"test_report"}},
		}},
	}
	starter := &projectTaskRunStarterFake{
		result: StartProjectTaskRunResult{
			RunID:         runID,
			RuntimeTaskID: runtimeTaskID,
			RuntimeNodeID: runtimeNodeID,
			NodeID:        "node-1",
			ProviderType:  "codex",
		},
		onStart: func(req StartProjectTaskRunRequest) {
			require.NotContains(t, req.Metadata, "provider_type")
		},
	}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	if err != nil {
		t.Fatalf("dispatch project task: %v", err)
	}
	if len(starter.requests) != 1 {
		t.Fatalf("expected one run start request, got %d", len(starter.requests))
	}
	req := starter.requests[0]
	attemptID := projectTaskDispatchAttemptID(taskID, 1)
	leaseToken := projectTaskAttemptLeaseToken(taskID, 1)
	attemptIdempotencyKey := projectTaskAttemptDispatchIdempotencyKey(taskID, 1)
	if req.DispatchUserID != ownerID || req.DigitalEmployeeID != employeeID || req.IdempotencyKey != attemptIdempotencyKey {
		t.Fatalf("unexpected run start request: %#v", req)
	}
	require.Equal(t, projectID, req.ProjectID)
	require.Equal(t, demandID, req.DemandID)
	require.Equal(t, taskID, req.ProjectTaskID)
	require.Equal(t, attemptID, req.ProjectTaskAttemptID)
	if !strings.Contains(req.Prompt, "需要确认测试报告") || !strings.Contains(req.Prompt, taskID.String()) {
		t.Fatalf("expected prompt to include demand content and task id, got %q", req.Prompt)
	}
	require.NotContains(t, req.Prompt, "回写")
	require.NotContains(t, strings.ToLower(req.Prompt), "writeback")
	require.Contains(t, req.Prompt, "Runtime Agent")
	require.Contains(t, req.Prompt, "expected_outputs")
	require.Contains(t, req.Prompt, "input_requirements")
	require.Contains(t, req.Prompt, "handoff_contract")
	require.Contains(t, req.Prompt, "result_contract")
	require.Contains(t, req.Prompt, "acceptance_results")
	require.Contains(t, req.Prompt, "test_report")
	require.Equal(t, []any{"execution_summary", "evidence_refs"}, req.Metadata["expected_outputs"])
	require.Equal(t, map[string]any{"required_context": []any{"test_report", "rollback_plan"}}, req.Metadata["input_requirements"])
	require.Equal(t, attemptID.String(), req.Metadata["project_task_attempt_id"])
	require.Equal(t, leaseToken, req.Metadata["project_task_lease_token"])
	require.Equal(t, "v1", req.Metadata["execution_context_packet_version"])
	require.Equal(t, map[string]any{"completion_path": "project_task_attempt_writeback", "required_refs": []any{"test_report"}}, req.Metadata["handoff_contract"])
	require.Empty(t, repo.bindRequests)
	require.Len(t, repo.queueRequests, 1)
	queueReq := repo.queueRequests[0]
	require.Equal(t, attemptIdempotencyKey, queueReq.IdempotencyKey)
	require.NotNil(t, queueReq.ProjectTaskAttemptID)
	require.Equal(t, attemptID, *queueReq.ProjectTaskAttemptID)
	require.Equal(t, leaseToken, queueReq.LeaseToken)
	require.Nil(t, queueReq.DigitalEmployeeRunID)
	require.Nil(t, queueReq.RuntimeTaskID)
	require.Nil(t, queueReq.RuntimeNodeID)
	require.Empty(t, queueReq.ProviderType)
	require.Equal(t, projectID.String(), queueReq.ExecutionContextPacket["project_id"])
	require.Equal(t, demandID.String(), queueReq.ExecutionContextPacket["demand_id"])
	require.Equal(t, taskID.String(), queueReq.ExecutionContextPacket["project_task_id"])
	require.Equal(t, attemptID.String(), queueReq.ExecutionContextPacket["project_task_attempt_id"])
	require.Equal(t, leaseToken, queueReq.ExecutionContextPacket["project_task_lease_token"])
	require.Equal(t, employeeID.String(), queueReq.ExecutionContextPacket["digital_employee_id"])
	require.Equal(t, "整理证据", queueReq.ExecutionContextPacket["objective"])
	require.Equal(t, []any{"execution_summary", "evidence_refs"}, queueReq.ExecutionContextPacket["expected_outputs"])
	require.Equal(t, map[string]any{"required_context": []any{"test_report", "rollback_plan"}}, queueReq.ExecutionContextPacket["input_requirements"])
	require.Equal(t, map[string]any{"completion_path": "project_task_attempt_writeback", "required_refs": []any{"test_report"}}, queueReq.ExecutionContextPacket["handoff_contract"])
	require.NotContains(t, queueReq.ExecutionContextPacket, "digital_employee_run_id")
	require.NotContains(t, queueReq.ExecutionContextPacket, "runtime_task_id")
	require.NotContains(t, queueReq.ExecutionContextPacket, "runtime_node_id")
	require.NotContains(t, queueReq.ExecutionContextPacket, "node_id")
	require.NotContains(t, queueReq.ExecutionContextPacket, "provider_type")
	require.Len(t, repo.bindAttemptRunRequests, 1)
	bindReq := repo.bindAttemptRunRequests[0]
	require.Equal(t, attemptID, bindReq.AttemptID)
	require.Equal(t, runID, bindReq.DigitalEmployeeRunID)
	require.Equal(t, runtimeTaskID, bindReq.RuntimeTaskID)
	require.Equal(t, runtimeNodeID, bindReq.RuntimeNodeID)
	require.Equal(t, "codex", bindReq.ProviderType)
	require.Equal(t, runID.String(), bindReq.ExecutionContextPacket["digital_employee_run_id"])
	require.Equal(t, runtimeTaskID.String(), bindReq.ExecutionContextPacket["runtime_task_id"])
	require.Equal(t, runtimeNodeID.String(), bindReq.ExecutionContextPacket["runtime_node_id"])
	require.Equal(t, "node-1", bindReq.ExecutionContextPacket["node_id"])
	require.Equal(t, "codex", bindReq.ExecutionContextPacket["provider_type"])
	require.Equal(t, "queued", repo.tasks[0].Status)
	require.NotNil(t, repo.tasks[0].CurrentAttemptID)
	require.Equal(t, int32(1), repo.tasks[0].AttemptCount)
	require.Equal(t, runID, *repo.tasks[0].DigitalEmployeeRunID)
	require.Equal(t, runtimeTaskID, *repo.tasks[0].RuntimeTaskID)
	require.Len(t, repo.projectTaskAttempts, 1)
	require.Equal(t, attemptID, *repo.tasks[0].CurrentAttemptID)
	require.Equal(t, attemptID, repo.projectTaskAttempts[0].ID)
	require.NotNil(t, repo.projectTaskAttempts[0].DigitalEmployeeID)
	require.Equal(t, employeeID, *repo.projectTaskAttempts[0].DigitalEmployeeID)
	require.NotNil(t, repo.projectTaskAttempts[0].ProviderType)
	require.Equal(t, "codex", *repo.projectTaskAttempts[0].ProviderType)
	dispatchedEvents := eventsByType(repo.events, project.ProjectEventTaskDispatched)
	if len(dispatchedEvents) != 1 {
		t.Fatalf("expected dispatched event, got %#v", repo.events)
	}
	dispatchedEvent := dispatchedEvents[0]
	require.Equal(t, "project_coordinator", dispatchedEvent.ActorType)
	require.Equal(t, taskID.String(), dispatchedEvent.ActorID)
	if dispatchedEvent.Payload["project_task_attempt_id"] != repo.projectTaskAttempts[0].ID.String() ||
		dispatchedEvent.Payload["project_task_status"] != "queued" ||
		dispatchedEvent.Payload["digital_employee_id"] != employeeID.String() {
		t.Fatalf("expected queued attempt payload, got %#v", dispatchedEvent.Payload)
	}
	require.NotContains(t, dispatchedEvent.Payload, "digital_employee_run_id")
	require.NotContains(t, dispatchedEvent.Payload, "runtime_task_id")
	require.NotContains(t, dispatchedEvent.Payload, "runtime_node_id")
}

func TestDispatchProjectTaskIncludesRepoBindingAndWorkspaceMode(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	attemptID := projectTaskDispatchAttemptID(taskID, 1)
	credentialRef := "  git-credential:primary  "
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{
			ID: projectID, TenantID: tenantID, Name: "Code project", HumanOwnerUserID: uuid.New(),
			RepoBinding: project.ProjectRepoBinding{
				Status:           project.ProjectRepoBindingStatusBound,
				URL:              "https://github.com/acme/app.git",
				DefaultBranch:    "  main  ",
				GitCredentialRef: &credentialRef,
				Scope:            []string{"apps/web", "packages/shared"},
			},
		},
		demand:  project.ProjectDemand{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, Title: "Fix login"},
		members: []project.ProjectMember{projectStoreExecutorMember(tenantID, projectID, employeeID)},
		tasks: []project.ProjectTask{{
			ID: taskID, TenantID: tenantID, ProjectID: projectID, Status: project.ProjectTaskStatusPlanned,
			Title: "Implement fix", AssignedDigitalEmployeeID: &employeeID, TaskKind: stringPtr("feature_development"),
		}},
	}
	repo.tasks[0].DemandID = &repo.demand.ID
	starter := &projectTaskRunStarterFake{result: StartProjectTaskRunResult{
		RunID: uuid.New(), RuntimeTaskID: uuid.New(), RuntimeNodeID: uuid.New(), NodeID: "node-1",
	}}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{
		TenantID: tenantID, ProjectID: projectID, TaskID: taskID, DispatchReason: project.DispatchReasonRootReady,
	})

	require.NoError(t, err)
	require.Len(t, starter.requests, 1)
	require.Equal(t, "branch", starter.requests[0].Metadata["workspace_mode"])
	require.Equal(t, "main", starter.requests[0].Metadata["base_ref"])
	require.Equal(t, attemptID.String(), starter.requests[0].Metadata["project_task_attempt_id"])
	require.Equal(t, map[string]any{
		"url":                "https://github.com/acme/app.git",
		"default_branch":     "main",
		"git_credential_ref": "git-credential:primary",
		"scope":              []any{"apps/web", "packages/shared"},
	}, starter.requests[0].Metadata["project_git"])
}

func TestDispatchProjectTaskRecordsDispatchBlockedEvent(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	demandID := uuid.New()
	employeeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord:          project.Project{ID: projectID, TenantID: tenantID, Name: "Blocked project", HumanOwnerUserID: uuid.New()},
		demand:                 project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "Needs runtime"},
		members:                []project.ProjectMember{projectStoreExecutorMember(tenantID, projectID, employeeID)},
		missingActivePlacement: true,
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Status:                    project.ProjectTaskStatusPlanned,
			Title:                     "Implement with runtime",
			AssignedDigitalEmployeeID: &employeeID,
		}},
	}
	starter := &projectTaskRunStarterFake{result: StartProjectTaskRunResult{
		RunID: uuid.New(), RuntimeTaskID: uuid.New(), RuntimeNodeID: uuid.New(), NodeID: "node-1",
	}}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{
		TenantID: tenantID, ProjectID: projectID, TaskID: taskID, DispatchReason: project.DispatchReasonRootReady,
	})

	require.NoError(t, err)
	require.Empty(t, starter.requests)
	blockedEvents := eventsByType(repo.events, project.ProjectEventTaskDispatchBlocked)
	require.Len(t, blockedEvents, 1)
	event := blockedEvents[0]
	require.Equal(t, "project_coordinator", event.ActorType)
	require.Equal(t, taskID.String(), event.ActorID)
	require.NotNil(t, event.ResourceType)
	require.Equal(t, "project_task", *event.ResourceType)
	require.NotNil(t, event.ResourceID)
	require.Equal(t, taskID.String(), *event.ResourceID)
	require.Equal(t, taskID.String(), event.Payload["project_task_id"])
	require.Equal(t, demandID.String(), event.Payload["demand_id"])
	require.Equal(t, "runtime_placement_missing", event.Payload["reason_code"])
	require.Equal(t, "bind_runtime", event.Payload["recommended_action"])
	require.NotEmpty(t, event.Payload["dispatch_gate_result_id"])
}

func TestDispatchProjectTaskBranchWorkspaceRequiresRuntimeAttestation(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{
			ID: projectID, TenantID: tenantID, Name: "Code project", HumanOwnerUserID: uuid.New(),
			RepoBinding: project.ProjectRepoBinding{
				Status:        project.ProjectRepoBindingStatusBound,
				URL:           "https://github.com/acme/app.git",
				DefaultBranch: "main",
			},
		},
		demand:  project.ProjectDemand{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, Title: "Implement login"},
		members: []project.ProjectMember{projectStoreExecutorMember(tenantID, projectID, employeeID)},
		tasks: []project.ProjectTask{{
			ID: taskID, TenantID: tenantID, ProjectID: projectID, Status: project.ProjectTaskStatusPlanned,
			Title: "Implement login", AssignedDigitalEmployeeID: &employeeID, TaskKind: stringPtr("feature_development"),
			HandoffContract: map[string]any{"acceptance_criteria": []any{"tests pass"}},
		}},
	}
	repo.tasks[0].DemandID = &repo.demand.ID
	starter := &projectTaskRunStarterFake{result: StartProjectTaskRunResult{
		RunID: uuid.New(), RuntimeTaskID: uuid.New(), RuntimeNodeID: uuid.New(), NodeID: "node-1",
	}}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{
		TenantID: tenantID, ProjectID: projectID, TaskID: taskID, DispatchReason: project.DispatchReasonRootReady,
	})

	require.NoError(t, err)
	require.Len(t, starter.requests, 1)
	runContract, ok := starter.requests[0].Metadata["handoff_contract"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, runContract["requires_runtime_attestation"])
	require.Len(t, repo.queueRequests, 1)
	packetContract, ok := repo.queueRequests[0].ExecutionContextPacket["handoff_contract"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, packetContract["requires_runtime_attestation"])
}

func TestDispatchProjectTaskForcesNoWorkspaceModeWithoutRepoBinding(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{
			ID: projectID, TenantID: tenantID, Name: "Unbound code project", HumanOwnerUserID: uuid.New(),
			RepoBinding: project.ProjectRepoBinding{Status: project.ProjectRepoBindingStatusUnbound},
		},
		demand:  project.ProjectDemand{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, Title: "Fix login"},
		members: []project.ProjectMember{projectStoreExecutorMember(tenantID, projectID, employeeID)},
		tasks: []project.ProjectTask{{
			ID: taskID, TenantID: tenantID, ProjectID: projectID, Status: project.ProjectTaskStatusPlanned,
			Title: "Implement fix", AssignedDigitalEmployeeID: &employeeID, TaskKind: stringPtr("feature_development"),
		}},
	}
	repo.tasks[0].DemandID = &repo.demand.ID
	starter := &projectTaskRunStarterFake{result: StartProjectTaskRunResult{
		RunID: uuid.New(), RuntimeTaskID: uuid.New(), RuntimeNodeID: uuid.New(), NodeID: "node-1",
	}}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{
		TenantID: tenantID, ProjectID: projectID, TaskID: taskID, DispatchReason: project.DispatchReasonRootReady,
	})

	require.NoError(t, err)
	require.Len(t, starter.requests, 1)
	require.Equal(t, WorkspaceModeNone, starter.requests[0].Metadata["workspace_mode"])
	require.Equal(t, "", starter.requests[0].Metadata["base_ref"])
	require.NotContains(t, starter.requests[0].Metadata, "project_git")
}

func TestProjectStoreDispatchProjectTaskRunsGateBeforeRunStart(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	ownerID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "需求"},
		members: []project.ProjectMember{{
			TenantID:      tenantID,
			ProjectID:     projectID,
			PrincipalType: project.PrincipalTypeDigitalEmployee,
			PrincipalID:   employeeID,
			ProjectRole:   project.ProjectRoleExecutor,
			Status:        "active",
		}},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    project.ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
		}},
	}
	starter := &projectTaskRunStarterFake{
		result: StartProjectTaskRunResult{
			RunID:         runID,
			RuntimeTaskID: runtimeTaskID,
			RuntimeNodeID: runtimeNodeID,
			NodeID:        "node-1",
		},
		onStart: func(req StartProjectTaskRunRequest) {
			require.Len(t, repo.dispatchGateResults, 1, "gate must be recorded before run start")
			require.Equal(t, project.PreDispatchGateStatusPassed, repo.dispatchGateResults[0].Status)
		},
	}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{
		TenantID:       tenantID,
		ProjectID:      projectID,
		TaskID:         taskID,
		DispatchReason: project.DispatchReasonDependencyUnlocked,
	})
	require.NoError(t, err)
	require.Len(t, repo.dispatchGateResults, 1)
	gate := repo.dispatchGateResults[0]
	require.Equal(t, project.DispatchReasonDependencyUnlocked, gate.DispatchReason)
	require.NotEmpty(t, gate.IdempotencyKey)
	require.NotEmpty(t, gate.DispatchToken)
	require.Len(t, starter.requests, 1)
	require.Equal(t, project.DispatchReasonDependencyUnlocked, starter.requests[0].Metadata["dispatch_reason"])
	require.Equal(t, gate.ID.String(), starter.requests[0].Metadata["dispatch_gate_result_id"])
	require.Equal(t, gate.Status, starter.requests[0].Metadata["dispatch_gate_status"])
	require.Equal(t, gate.IdempotencyKey, starter.requests[0].Metadata["dispatch_gate_idempotency_key"])
	require.Equal(t, gate.DispatchToken, starter.requests[0].Metadata["dispatch_gate_dispatch_token"])
	require.Len(t, repo.queueRequests, 1)
	require.NotNil(t, repo.queueRequests[0].DispatchGateResultID)
	require.Equal(t, gate.ID, *repo.queueRequests[0].DispatchGateResultID)
	require.Equal(t, project.DispatchReasonDependencyUnlocked, repo.queueRequests[0].ExecutionContextPacket["dispatch_reason"])
	require.Equal(t, gate.ID.String(), repo.queueRequests[0].ExecutionContextPacket["dispatch_gate_result_id"])
	require.Equal(t, gate.Status, repo.queueRequests[0].ExecutionContextPacket["dispatch_gate_status"])
	require.Equal(t, gate.IdempotencyKey, repo.queueRequests[0].ExecutionContextPacket["dispatch_gate_idempotency_key"])
	require.Equal(t, gate.DispatchToken, repo.queueRequests[0].ExecutionContextPacket["dispatch_gate_dispatch_token"])
	require.Len(t, repo.linkGateAttemptRequests, 1)
	require.Equal(t, gate.ID, repo.linkGateAttemptRequests[0].GateResultID)
	require.Equal(t, repo.projectTaskAttempts[0].ID, repo.linkGateAttemptRequests[0].AttemptID)
	require.NotNil(t, repo.projectTaskAttempts[0].DispatchGateResultID)
	require.Equal(t, gate.ID, *repo.projectTaskAttempts[0].DispatchGateResultID)
	require.NotNil(t, repo.dispatchGateResults[0].AttemptID)
	require.Equal(t, repo.projectTaskAttempts[0].ID, *repo.dispatchGateResults[0].AttemptID)
	require.Len(t, eventsByType(repo.events, project.ProjectEventTaskDispatchGateChecked), 1)
	require.Len(t, eventsByType(repo.events, project.ProjectEventTaskDispatched), 1)
}

func TestProjectStoreDispatchProjectTaskAllowsRuntimeLessReadyEmployeeWithRuntimeCapability(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, Status: project.ProjectStatusRunning, HumanOwnerUserID: uuid.New()},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "需求"},
		members:       []project.ProjectMember{projectStoreExecutorMember(tenantID, projectID, employeeID)},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    project.ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
			InputRequirements: map[string]any{
				"required_capabilities": []any{"provider:codex"},
			},
			PlannerMetadata: map[string]any{
				"employee_selection": map[string]any{
					"matched_capabilities": []any{"provider:codex"},
				},
			},
		}},
	}
	starter := &projectTaskRunStarterFake{result: StartProjectTaskRunResult{
		RunID:         runID,
		RuntimeTaskID: runtimeTaskID,
		RuntimeNodeID: runtimeNodeID,
		NodeID:        "node-runtime-capability",
		ProviderType:  "codex",
	}}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter).
		WithPreDispatchGateReaders(projectStoreGateRuntimeReader{
			employee: project.PreDispatchEmployeeSnapshot{
				ID:                 employeeID,
				Status:             "ready",
				PolicyAllowed:      true,
				RequiredLoadSlots:  1,
				AvailableLoadSlots: 1,
			},
			runtime: project.PreDispatchRuntimeSnapshot{
				NodeOnline:              true,
				ProviderAvailable:       true,
				WorkspaceReady:          true,
				SlotAvailable:           true,
				ContractVersionAccepted: true,
			},
		}, nil)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{
		TenantID:       tenantID,
		ProjectID:      projectID,
		TaskID:         taskID,
		DispatchReason: project.DispatchReasonRootReady,
	})

	require.NoError(t, err)
	require.Len(t, repo.dispatchGateResults, 1)
	require.Equal(t, project.PreDispatchGateStatusPassed, repo.dispatchGateResults[0].Status)
	require.Empty(t, repo.dispatchGateResults[0].Blockers)
	require.Len(t, starter.requests, 1)
	require.Equal(t, employeeID, starter.requests[0].DigitalEmployeeID)
	require.Len(t, repo.queueRequests, 1)
	require.Nil(t, repo.queueRequests[0].DigitalEmployeeRunID)
	require.Nil(t, repo.queueRequests[0].RuntimeTaskID)
	require.Nil(t, repo.queueRequests[0].RuntimeNodeID)
	require.Empty(t, repo.queueRequests[0].ProviderType)
	require.Len(t, repo.bindAttemptRunRequests, 1)
	require.Equal(t, runID, repo.bindAttemptRunRequests[0].DigitalEmployeeRunID)
	require.Equal(t, runtimeTaskID, repo.bindAttemptRunRequests[0].RuntimeTaskID)
	require.Equal(t, runtimeNodeID, repo.bindAttemptRunRequests[0].RuntimeNodeID)
	require.Equal(t, "codex", repo.bindAttemptRunRequests[0].ProviderType)
	require.Equal(t, project.ProjectTaskStatusQueued, repo.tasks[0].Status)
	require.Empty(t, repo.bindRequests)
}

func TestProjectStoreDispatchProjectTaskWaitingHumanGateDoesNotStartRun(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	ownerID := uuid.New()
	approvalID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "需求"},
		members: []project.ProjectMember{{
			TenantID:      tenantID,
			ProjectID:     projectID,
			PrincipalType: project.PrincipalTypeDigitalEmployee,
			PrincipalID:   employeeID,
			ProjectRole:   project.ProjectRoleExecutor,
			Status:        "active",
		}},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    project.ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
			RequiresHumanApproval:     true,
		}},
	}
	approvals := &projectStoreApprovalCreator{approvalID: approvalID}
	starter := &projectTaskRunStarterFake{}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, approvals, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	require.NoError(t, err)
	require.Empty(t, starter.requests)
	require.Empty(t, repo.queueRequests)
	require.Empty(t, repo.bindRequests)
	require.Len(t, repo.dispatchGateResults, 1)
	gate := repo.dispatchGateResults[0]
	require.Equal(t, project.PreDispatchGateStatusWaitingHuman, gate.Status)
	require.NotNil(t, gate.DecisionRequestID)
	require.NotNil(t, approvals.record)
	require.Equal(t, gate.ID, approvals.record.ResourceID)
	require.Equal(t, project.ProjectTaskStatusWaitingHuman, repo.tasks[0].Status)
	require.NotNil(t, repo.tasks[0].WaitingRequestID)
	require.Equal(t, *gate.DecisionRequestID, *repo.tasks[0].WaitingRequestID)
	require.NotNil(t, repo.tasks[0].LatestDispatchGateResultID)
	require.Equal(t, gate.ID, *repo.tasks[0].LatestDispatchGateResultID)
	require.Len(t, eventsByType(repo.events, project.ProjectEventTaskDispatchGateWaitingHuman), 1)
	require.Len(t, eventsByType(repo.events, project.ProjectEventDecisionRequested), 1)
	require.Empty(t, eventsByType(repo.events, project.ProjectEventTaskDispatched))
	require.Equal(t, 1, repo.getProjectCalls)
	require.Zero(t, repo.getProjectDemandCalls)
}

func TestProjectStoreDispatchProjectTaskRetryLaterGateIsRetryable(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "需求"},
		members: []project.ProjectMember{{
			TenantID:      tenantID,
			ProjectID:     projectID,
			PrincipalType: project.PrincipalTypeDigitalEmployee,
			PrincipalID:   employeeID,
			ProjectRole:   project.ProjectRoleExecutor,
			Status:        "active",
		}},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    project.ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
		}},
	}
	starter := &projectTaskRunStarterFake{}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter).
		WithPreDispatchGateReaders(projectStoreGateRuntimeReader{
			runtime: project.PreDispatchRuntimeSnapshot{
				NodeOnline:              true,
				ProviderAvailable:       true,
				WorkspaceReady:          true,
				SlotAvailable:           false,
				ContractVersionAccepted: true,
			},
		}, nil)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	require.ErrorIs(t, err, ErrProjectTaskDispatchRetryLater)
	require.True(t, dispatchErrorRetryable(err))
	require.Empty(t, starter.requests)
	require.Empty(t, repo.queueRequests)
	require.Empty(t, repo.bindRequests)
	require.Len(t, repo.dispatchGateResults, 1)
	require.Equal(t, project.PreDispatchGateStatusRetryLater, repo.dispatchGateResults[0].Status)
	require.Len(t, eventsByType(repo.events, project.ProjectEventTaskDispatchGateRetryLater), 1)
	require.Empty(t, eventsByType(repo.events, project.ProjectEventTaskDispatched))
	require.Empty(t, eventsByType(repo.events, project.ProjectEventTaskDispatchFailed))
	require.Equal(t, project.ProjectTaskStatusPlanned, repo.tasks[0].Status)
	require.Zero(t, repo.getProjectCalls)
	require.Zero(t, repo.getProjectDemandCalls)
}

func TestProjectStoreDispatchProjectTaskCreatesQueuedAttempt(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "需求"},
		members:       []project.ProjectMember{projectStoreExecutorMember(tenantID, projectID, employeeID)},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    project.ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
		}},
	}
	starter := &projectTaskRunStarterFake{result: StartProjectTaskRunResult{
		RunID:         runID,
		RuntimeTaskID: runtimeTaskID,
		RuntimeNodeID: runtimeNodeID,
		NodeID:        "node-1",
	}}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	require.NoError(t, err)
	require.Len(t, repo.queueRequests, 1)
	require.Empty(t, repo.bindRequests)
	require.Len(t, repo.projectTaskAttempts, 1)
	require.Equal(t, project.ProjectTaskStatusQueued, repo.tasks[0].Status)
	require.Equal(t, repo.projectTaskAttempts[0].ID, *repo.tasks[0].CurrentAttemptID)
	require.Equal(t, runID, *repo.tasks[0].DigitalEmployeeRunID)
	require.Equal(t, runtimeTaskID, *repo.tasks[0].RuntimeTaskID)
	require.Equal(t, runID, *repo.projectTaskAttempts[0].DigitalEmployeeRunID)
	require.Equal(t, runtimeTaskID, *repo.projectTaskAttempts[0].RuntimeTaskID)
	require.Equal(t, runtimeNodeID, *repo.projectTaskAttempts[0].RuntimeNodeID)
	require.Len(t, eventsByType(repo.events, project.ProjectEventTaskDispatched), 1)
}

func TestProjectStoreDispatchProjectTaskQueuedAttemptEventIsIdempotentOnRetry(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "需求"},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    project.ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
		}},
	}
	_, err := repo.QueueProjectTaskWithAttempt(context.Background(), project.QueueProjectTaskRequest{
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        taskID,
		DigitalEmployeeID:    employeeID,
		DigitalEmployeeRunID: &runID,
		RuntimeTaskID:        &runtimeTaskID,
		RuntimeNodeID:        &runtimeNodeID,
		IdempotencyKey:       projectTaskAttemptDispatchIdempotencyKey(taskID, 1),
		LeaseToken:           "project-task-" + taskID.String() + "-attempt-1",
	})
	require.NoError(t, err)
	require.Len(t, repo.events, 1)
	require.Equal(t, "project_coordinator", repo.events[0].ActorType)
	require.Equal(t, taskID.String(), repo.events[0].ActorID)

	starter := &projectTaskRunStarterFake{}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err = store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	require.NoError(t, err)
	require.Empty(t, starter.requests)
	require.Empty(t, repo.bindRequests)
	require.Len(t, repo.queueRequests, 1)
	require.Len(t, repo.events, 1)
	require.Equal(t, 1, repo.advanceDemandCalls)
	require.Equal(t, project.ProjectDemandStatusExecuting, repo.demand.Status)
}

func TestProjectStoreDispatchProjectTaskRetriesQueuedAttemptWithoutRunBinding(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Status: project.ProjectDemandStatusPlanned, Title: "需求"},
		members:       []project.ProjectMember{projectStoreExecutorMember(tenantID, projectID, employeeID)},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    project.ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
		}},
	}
	attemptID := projectTaskDispatchAttemptID(taskID, 1)
	_, err := repo.QueueProjectTaskWithAttempt(context.Background(), project.QueueProjectTaskRequest{
		TenantID:                      tenantID,
		ProjectID:                     projectID,
		ProjectTaskID:                 taskID,
		ProjectTaskAttemptID:          &attemptID,
		DigitalEmployeeID:             employeeID,
		IdempotencyKey:                projectTaskAttemptDispatchIdempotencyKey(taskID, 1),
		LeaseToken:                    projectTaskAttemptLeaseToken(taskID, 1),
		ExecutionContextPacket:        map[string]any{"project_task_id": taskID.String()},
		ExecutionContextPacketVersion: "v1",
	})
	require.NoError(t, err)
	require.Nil(t, repo.tasks[0].DigitalEmployeeRunID)
	require.Nil(t, repo.tasks[0].RuntimeTaskID)

	starter := &projectTaskRunStarterFake{result: StartProjectTaskRunResult{
		RunID:         runID,
		RuntimeTaskID: runtimeTaskID,
		RuntimeNodeID: runtimeNodeID,
		NodeID:        "node-1",
		ProviderType:  "codex",
	}}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err = store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	require.NoError(t, err)
	require.Len(t, starter.requests, 1)
	require.Equal(t, attemptID, starter.requests[0].ProjectTaskAttemptID)
	require.Len(t, repo.queueRequests, 1)
	require.Len(t, repo.bindAttemptRunRequests, 1)
	require.Equal(t, attemptID, repo.bindAttemptRunRequests[0].AttemptID)
	require.Equal(t, runID, *repo.tasks[0].DigitalEmployeeRunID)
	require.Equal(t, runtimeTaskID, *repo.tasks[0].RuntimeTaskID)
	require.Len(t, eventsByType(repo.events, project.ProjectEventTaskDispatched), 1)
	require.Empty(t, eventsByType(repo.events, project.ProjectEventTaskDispatchFailed))
	require.Equal(t, project.ProjectDemandStatusExecuting, repo.demand.Status)
}

func TestProjectStoreDispatchProjectTaskRetriesAfterRunStartFailureWithNewAttempt(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	firstAttemptID := projectTaskDispatchAttemptID(taskID, 1)
	secondAttemptID := projectTaskDispatchAttemptID(taskID, 2)
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Status: project.ProjectDemandStatusPlanned, Title: "需求"},
		members:       []project.ProjectMember{projectStoreExecutorMember(tenantID, projectID, employeeID)},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    project.ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
		}},
	}
	startErr := errors.New("runtime start timeout")
	starter := &projectTaskRunStarterFake{err: startErr}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	require.ErrorIs(t, err, startErr)
	require.True(t, dispatchFailureRecorded(err))
	require.Len(t, starter.requests, 1)
	require.Len(t, repo.queueRequests, 1)
	require.Len(t, repo.projectTaskAttempts, 1)
	require.Equal(t, firstAttemptID, repo.projectTaskAttempts[0].ID)
	require.Equal(t, project.ProjectTaskAttemptStatusLost, repo.projectTaskAttempts[0].Status)
	require.Equal(t, project.ProjectTaskStatusPlanned, repo.tasks[0].Status)
	require.Nil(t, repo.tasks[0].CurrentAttemptID)
	require.Nil(t, repo.tasks[0].DigitalEmployeeRunID)
	require.Nil(t, repo.tasks[0].RuntimeTaskID)

	starter.err = nil
	starter.result = StartProjectTaskRunResult{
		RunID:         runID,
		RuntimeTaskID: runtimeTaskID,
		RuntimeNodeID: runtimeNodeID,
		NodeID:        "node-1",
		ProviderType:  "codex",
	}
	err = store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	require.NoError(t, err)
	require.Len(t, starter.requests, 2)
	require.Equal(t, secondAttemptID, starter.requests[1].ProjectTaskAttemptID)
	require.Equal(t, projectTaskAttemptDispatchIdempotencyKey(taskID, 2), starter.requests[1].IdempotencyKey)
	require.Len(t, repo.queueRequests, 2)
	require.Len(t, repo.projectTaskAttempts, 2)
	require.Equal(t, secondAttemptID, repo.projectTaskAttempts[1].ID)
	require.Equal(t, project.ProjectTaskAttemptStatusQueued, repo.projectTaskAttempts[1].Status)
	require.Equal(t, secondAttemptID, repo.bindAttemptRunRequests[0].AttemptID)
	require.Equal(t, secondAttemptID, *repo.tasks[0].CurrentAttemptID)
	require.Equal(t, runID, *repo.tasks[0].DigitalEmployeeRunID)
	require.Equal(t, runtimeTaskID, *repo.tasks[0].RuntimeTaskID)
	require.Equal(t, project.ProjectDemandStatusExecuting, repo.demand.Status)
}

func TestProjectStoreDispatchProjectTaskBindFailureRetriesWithoutDispatchRecovery(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	bindErr := errors.New("attempt run bind failed")
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Status: project.ProjectDemandStatusPlanned, Title: "需求"},
		members:       []project.ProjectMember{projectStoreExecutorMember(tenantID, projectID, employeeID)},
		bindErr:       bindErr,
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    project.ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
		}},
	}
	starter := &projectTaskRunStarterFake{result: StartProjectTaskRunResult{
		RunID:         uuid.New(),
		RuntimeTaskID: uuid.New(),
		RuntimeNodeID: uuid.New(),
		NodeID:        "node-1",
		ProviderType:  "codex",
	}}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	require.ErrorIs(t, err, bindErr)
	require.False(t, dispatchFailureRecorded(err))
	require.Len(t, starter.requests, 1)
	require.Len(t, repo.queueRequests, 1)
	require.Len(t, repo.bindAttemptRunRequests, 1)
	require.Len(t, repo.projectTaskAttempts, 1)
	require.Equal(t, project.ProjectTaskStatusQueued, repo.tasks[0].Status)
	require.NotNil(t, repo.tasks[0].CurrentAttemptID)
	attemptID := *repo.tasks[0].CurrentAttemptID
	require.Nil(t, repo.tasks[0].DigitalEmployeeRunID)
	require.Nil(t, repo.tasks[0].RuntimeTaskID)
	require.Empty(t, eventsByType(repo.events, project.ProjectEventTaskDispatchFailed))

	repo.bindErr = nil
	err = store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	require.NoError(t, err)
	require.Len(t, starter.requests, 2)
	require.Equal(t, attemptID, starter.requests[1].ProjectTaskAttemptID)
	require.Equal(t, projectTaskAttemptDispatchIdempotencyKey(taskID, 1), starter.requests[1].IdempotencyKey)
	require.Len(t, repo.queueRequests, 1)
	require.Len(t, repo.projectTaskAttempts, 1)
	require.Len(t, repo.bindAttemptRunRequests, 2)
	require.Equal(t, attemptID, repo.bindAttemptRunRequests[1].AttemptID)
	require.NotNil(t, repo.tasks[0].DigitalEmployeeRunID)
	require.NotNil(t, repo.tasks[0].RuntimeTaskID)
	require.Equal(t, project.ProjectDemandStatusExecuting, repo.demand.Status)
}

func TestProjectStoreDispatchProjectTaskRejectsPendingBeforeRunStart(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Status: project.ProjectDemandStatusPlanned, Title: "需求"},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    "pending",
			AssignedDigitalEmployeeID: &employeeID,
		}},
	}
	starter := &projectTaskRunStarterFake{result: StartProjectTaskRunResult{RunID: uuid.New(), RuntimeTaskID: uuid.New(), RuntimeNodeID: uuid.New(), NodeID: "node-1"}}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	require.ErrorIs(t, err, project.ErrInvalidProject)
	require.True(t, dispatchFailureRecorded(err))
	require.Empty(t, starter.requests)
	require.Empty(t, repo.queueRequests)
	require.Empty(t, repo.bindRequests)
	require.Equal(t, 0, repo.advanceDemandCalls)
	require.Equal(t, "pending", repo.tasks[0].Status)
	require.Len(t, eventsByType(repo.events, project.ProjectEventTaskDispatchFailed), 1)
}

func TestProjectStoreDispatchProjectTaskRetriesDemandAdvanceAfterQueuedReplay(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	runtimeNodeID := uuid.New()
	advanceErr := errors.New("demand status write failed")
	repo := &projectStoreMemoryRepository{
		projectRecord:    project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		demand:           project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Status: project.ProjectDemandStatusPlanned, Title: "需求"},
		members:          []project.ProjectMember{projectStoreExecutorMember(tenantID, projectID, employeeID)},
		advanceDemandErr: advanceErr,
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    project.ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
		}},
	}
	starter := &projectTaskRunStarterFake{result: StartProjectTaskRunResult{
		RunID:         runID,
		RuntimeTaskID: runtimeTaskID,
		RuntimeNodeID: runtimeNodeID,
		NodeID:        "node-1",
	}}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	require.ErrorIs(t, err, advanceErr)
	require.Len(t, starter.requests, 1)
	require.Len(t, repo.queueRequests, 1)
	require.Len(t, eventsByType(repo.events, project.ProjectEventTaskDispatchGateChecked), 1)
	require.Len(t, eventsByType(repo.events, project.ProjectEventTaskDispatched), 1)
	require.Equal(t, 1, repo.advanceDemandCalls)
	require.Equal(t, project.ProjectTaskStatusQueued, repo.tasks[0].Status)

	repo.advanceDemandErr = nil
	err = store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	require.NoError(t, err)
	require.Len(t, starter.requests, 1)
	require.Len(t, repo.queueRequests, 1)
	require.Empty(t, repo.bindRequests)
	require.Len(t, eventsByType(repo.events, project.ProjectEventTaskDispatchGateChecked), 1)
	require.Len(t, eventsByType(repo.events, project.ProjectEventTaskDispatched), 1)
	require.Equal(t, 2, repo.advanceDemandCalls)
	require.Equal(t, project.ProjectDemandStatusExecuting, repo.demand.Status)
}

func TestProjectStoreDispatchProjectTaskBindConflictIsIdempotentWhenSameAttemptRunAlreadyBound(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord:                  project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		demand:                         project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Status: project.ProjectDemandStatusPlanned, Title: "需求"},
		members:                        []project.ProjectMember{projectStoreExecutorMember(tenantID, projectID, employeeID)},
		bindConflictAfterApplyingFirst: true,
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    project.ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
		}},
	}
	starter := &projectTaskRunStarterFake{result: StartProjectTaskRunResult{
		RunID:         runID,
		RuntimeTaskID: runtimeTaskID,
		RuntimeNodeID: runtimeNodeID,
		NodeID:        "node-1",
		ProviderType:  "codex",
	}}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	require.NoError(t, err)
	require.Len(t, starter.requests, 1)
	require.Len(t, repo.queueRequests, 1)
	require.Len(t, repo.bindAttemptRunRequests, 1)
	require.Len(t, eventsByType(repo.events, project.ProjectEventTaskDispatched), 1)
	require.Empty(t, eventsByType(repo.events, project.ProjectEventTaskDispatchFailed))
	require.Equal(t, 1, repo.advanceDemandCalls)
	require.Equal(t, project.ProjectDemandStatusExecuting, repo.demand.Status)
	require.NotNil(t, repo.tasks[0].CurrentAttemptID)
	require.Equal(t, runID, *repo.tasks[0].DigitalEmployeeRunID)
	require.Equal(t, runtimeTaskID, *repo.tasks[0].RuntimeTaskID)
}

func TestProjectStoreDispatchProjectTaskQueueConflictIsIdempotentWhenTaskAlreadyDispatchedConcurrently(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord:                        project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		demand:                               project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Status: project.ProjectDemandStatusPlanned, Title: "需求"},
		members:                              []project.ProjectMember{projectStoreExecutorMember(tenantID, projectID, employeeID)},
		queueConflictAfterConcurrentDispatch: true,
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    project.ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
		}},
	}
	starter := &projectTaskRunStarterFake{}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	require.NoError(t, err)
	require.Empty(t, starter.requests)
	require.Len(t, repo.queueRequests, 1)
	require.Empty(t, repo.bindAttemptRunRequests)
	require.Empty(t, eventsByType(repo.events, project.ProjectEventTaskDispatchFailed))
	require.Equal(t, 1, repo.advanceDemandCalls)
	require.Equal(t, project.ProjectDemandStatusExecuting, repo.demand.Status)
	require.NotNil(t, repo.tasks[0].CurrentAttemptID)
	require.NotNil(t, repo.tasks[0].DigitalEmployeeRunID)
	require.NotNil(t, repo.tasks[0].RuntimeTaskID)
}

func TestProjectStoreDispatchProjectTaskBindingEnablesRuntimeWriteback(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "需求"},
		members:       []project.ProjectMember{projectStoreExecutorMember(tenantID, projectID, employeeID)},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    "planned",
			AssignedDigitalEmployeeID: &employeeID,
		}},
	}
	starter := &projectTaskRunStarterFake{result: StartProjectTaskRunResult{
		RunID:         runID,
		RuntimeTaskID: runtimeTaskID,
		RuntimeNodeID: runtimeNodeID,
		NodeID:        "node-1",
	}}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	if err != nil {
		t.Fatalf("dispatch project task: %v", err)
	}
	if repo.tasks[0].Status != "queued" {
		t.Fatalf("expected queued task after dispatch, got %s", repo.tasks[0].Status)
	}
	if repo.tasks[0].DigitalEmployeeRunID == nil || *repo.tasks[0].DigitalEmployeeRunID != runID {
		t.Fatalf("expected digital employee run binding, got %#v", repo.tasks[0].DigitalEmployeeRunID)
	}
	if repo.tasks[0].RuntimeTaskID == nil || *repo.tasks[0].RuntimeTaskID != runtimeTaskID {
		t.Fatalf("expected runtime task binding, got %#v", repo.tasks[0].RuntimeTaskID)
	}
	if repo.demand.Status != project.ProjectDemandStatusExecuting {
		t.Fatalf("expected demand advanced to executing after dispatch, got %s", repo.demand.Status)
	}
	require.Len(t, repo.queueRequests, 1)
	require.Empty(t, repo.bindRequests)
}

func TestProjectStoreDispatchProjectTaskRunStartFailureKeepsTaskPlanned(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "需求"},
		members:       []project.ProjectMember{projectStoreExecutorMember(tenantID, projectID, employeeID)},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    "planned",
			AssignedDigitalEmployeeID: &employeeID,
		}},
	}
	// Plain error => default to retryable.
	starter := &projectTaskRunStarterFake{err: errors.New("runtime node is not connected")}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	if err == nil {
		t.Fatal("expected dispatch error")
	}
	if repo.tasks[0].Status != "planned" || len(repo.bindRequests) != 0 || len(repo.queueRequests) != 1 {
		t.Fatalf("expected planned task after released queued attempt, task=%#v binds=%#v queues=%#v", repo.tasks[0], repo.bindRequests, repo.queueRequests)
	}
	require.Nil(t, repo.tasks[0].CurrentAttemptID)
	require.Len(t, repo.projectTaskAttempts, 1)
	require.Equal(t, project.ProjectTaskAttemptStatusLost, repo.projectTaskAttempts[0].Status)
	require.NotNil(t, repo.projectTaskAttempts[0].Retryable)
	require.True(t, *repo.projectTaskAttempts[0].Retryable)
	require.Len(t, repo.dispatchStartFailures, 1)
	dispatchFailedEvents := eventsByType(repo.events, project.ProjectEventTaskDispatchFailed)
	if len(dispatchFailedEvents) != 1 {
		t.Fatalf("expected dispatch failed event, got %#v", repo.events)
	}
	if dispatchFailedEvents[0].Payload["retryable"] != true {
		t.Fatalf("expected retryable failure payload, got %#v", dispatchFailedEvents[0].Payload)
	}
}

func TestProjectStoreDispatchProjectTaskQueueFailureDoesNotLeaveStartedRuntimeRun(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "需求"},
		members:       []project.ProjectMember{projectStoreExecutorMember(tenantID, projectID, employeeID)},
		queueErr:      errors.New("attempt queue failed"),
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    project.ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
		}},
	}
	starter := &projectTaskRunStarterFake{result: StartProjectTaskRunResult{
		RunID:         uuid.New(),
		RuntimeTaskID: uuid.New(),
		RuntimeNodeID: uuid.New(),
		NodeID:        "node-1",
		ProviderType:  "codex",
	}}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})

	require.Error(t, err)
	require.Empty(t, starter.requests, "dispatch must not leave a started runtime run when attempt queue fails")
	require.Len(t, repo.queueRequests, 1)
	require.Empty(t, repo.projectTaskAttempts)
	require.Equal(t, project.ProjectTaskStatusPlanned, repo.tasks[0].Status)
}

func TestProjectStoreDispatchProjectTaskTerminalRunStartFailureMarksNonRetryable(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "需求"},
		members:       []project.ProjectMember{projectStoreExecutorMember(tenantID, projectID, employeeID)},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    "planned",
			AssignedDigitalEmployeeID: &employeeID,
		}},
	}
	starter := &projectTaskRunStarterFake{err: &ProjectTaskRunStartError{Retryable: false, Err: errors.New("invalid run input")}}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	if err == nil {
		t.Fatal("expected dispatch error")
	}
	dispatchFailedEvents := eventsByType(repo.events, project.ProjectEventTaskDispatchFailed)
	if len(dispatchFailedEvents) != 1 || dispatchFailedEvents[0].Payload["retryable"] != false {
		t.Fatalf("expected non-retryable failure payload, got %#v", repo.events)
	}
	if repo.tasks[0].Status != "planned" || len(repo.bindRequests) != 0 || len(repo.queueRequests) != 1 {
		t.Fatalf("expected planned task after released queued attempt, task=%#v binds=%#v queues=%#v", repo.tasks[0], repo.bindRequests, repo.queueRequests)
	}
	require.Nil(t, repo.tasks[0].CurrentAttemptID)
	require.Len(t, repo.projectTaskAttempts, 1)
	require.Equal(t, project.ProjectTaskAttemptStatusLost, repo.projectTaskAttempts[0].Status)
	require.NotNil(t, repo.projectTaskAttempts[0].Retryable)
	require.False(t, *repo.projectTaskAttempts[0].Retryable)
	require.Len(t, repo.dispatchStartFailures, 1)
}

func TestProjectStoreDispatchProjectTaskAlreadyBoundSameRunIsIdempotent(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "需求"},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    "queued",
			AssignedDigitalEmployeeID: &employeeID,
			DigitalEmployeeRunID:      &runID,
			RuntimeTaskID:             &runtimeTaskID,
		}},
	}
	// The dispatched event already exists, so the idempotent replay must be a pure no-op.
	repo.events = append(repo.events, project.ProjectEvent{TenantID: tenantID, ProjectID: projectID, EventType: project.ProjectEventTaskDispatched, ActorType: "project_coordinator", ActorID: taskID.String()})
	starter := &projectTaskRunStarterFake{}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	if err != nil {
		t.Fatalf("expected idempotent success, got %v", err)
	}
	if len(starter.requests) != 0 || len(repo.bindRequests) != 0 || len(repo.queueRequests) != 0 || len(repo.events) != 1 {
		t.Fatalf("expected no duplicate side effects, starts=%d binds=%d queues=%d events=%d", len(starter.requests), len(repo.bindRequests), len(repo.queueRequests), len(repo.events))
	}
	require.Equal(t, 1, repo.advanceDemandCalls)
}

func TestProjectStoreDispatchProjectTaskReemitsMissingDispatchedEvent(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	ownerID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "需求"},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    "queued",
			AssignedDigitalEmployeeID: &employeeID,
			DigitalEmployeeRunID:      &runID,
			RuntimeTaskID:             &runtimeTaskID,
		}},
	}
	// Task is bound but the dispatched event is missing (e.g. a prior attempt crashed
	// after binding); dispatch must re-emit exactly one event without restarting the run.
	starter := &projectTaskRunStarterFake{}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	if err != nil {
		t.Fatalf("expected idempotent success, got %v", err)
	}
	if len(starter.requests) != 0 || len(repo.bindRequests) != 0 || len(repo.queueRequests) != 0 {
		t.Fatalf("expected no run start, bind, or queue, starts=%d binds=%d queues=%d", len(starter.requests), len(repo.bindRequests), len(repo.queueRequests))
	}
	if len(repo.events) != 1 || repo.events[0].EventType != project.ProjectEventTaskDispatched || repo.events[0].Payload["reemitted"] != true {
		t.Fatalf("expected one re-emitted dispatched event, got %#v", repo.events)
	}
	if repo.events[0].Payload["digital_employee_run_id"] != runID.String() {
		t.Fatalf("expected re-emitted payload to carry run id, got %#v", repo.events[0].Payload)
	}
	require.Equal(t, 1, repo.advanceDemandCalls)
}

func TestProjectStoreDispatchProjectTaskQueuedAttemptAlreadyBoundIsIdempotent(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	ownerID := uuid.New()
	attemptID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Status: project.ProjectDemandStatusCompleted, Title: "需求"},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    project.ProjectTaskStatusQueued,
			AssignedDigitalEmployeeID: &employeeID,
			CurrentAttemptID:          &attemptID,
			AttemptCount:              1,
		}},
		projectTaskAttempts: []project.ProjectTaskAttempt{{
			ID:                   attemptID,
			TenantID:             tenantID,
			ProjectTaskID:        taskID,
			AttemptNo:            1,
			Status:               project.ProjectTaskAttemptStatusQueued,
			DigitalEmployeeID:    &employeeID,
			DigitalEmployeeRunID: &runID,
			RuntimeTaskID:        &runtimeTaskID,
			RuntimeNodeID:        &runtimeNodeID,
			ProviderType:         strPtr("codex"),
			LeaseToken:           "lease-token",
			IdempotencyKey:       projectTaskAttemptDispatchIdempotencyKey(taskID, 1),
		}},
	}
	starter := &projectTaskRunStarterFake{
		result: StartProjectTaskRunResult{
			RunID:         runID,
			RuntimeTaskID: runtimeTaskID,
			RuntimeNodeID: runtimeNodeID,
			ProviderType:  "codex",
		},
		onStart: func(StartProjectTaskRunRequest) {
			repo.tasks[0].Status = project.ProjectTaskStatusCompleted
			repo.tasks[0].DigitalEmployeeRunID = &runID
			repo.tasks[0].RuntimeTaskID = &runtimeTaskID
		},
	}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	require.NoError(t, err)
	require.Len(t, starter.requests, 1)
	require.Len(t, repo.bindAttemptRunRequests, 1)
	require.Equal(t, project.ProjectDemandStatusCompleted, repo.demand.Status)
	require.Equal(t, runID, *repo.tasks[0].DigitalEmployeeRunID)
	require.Equal(t, runtimeTaskID, *repo.tasks[0].RuntimeTaskID)
}

func TestProjectStoreDispatchProjectTaskRejectsBoundRunMissingRuntimeTask(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	runID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		demand:        project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "需求"},
		tasks: []project.ProjectTask{{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "执行任务",
			Status:                    "queued",
			AssignedDigitalEmployeeID: &employeeID,
			DigitalEmployeeRunID:      &runID,
		}},
	}
	starter := &projectTaskRunStarterFake{}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID})
	if !errors.Is(err, project.ErrInvalidProject) {
		t.Fatalf("expected invalid project error, got %v", err)
	}
	if !dispatchFailureRecorded(err) {
		t.Fatalf("expected recorded dispatch failure, got %v", err)
	}
	if len(starter.requests) != 0 || len(repo.bindRequests) != 0 || len(repo.queueRequests) != 0 {
		t.Fatalf("expected no run start, bind, or queue, starts=%d binds=%d queues=%d", len(starter.requests), len(repo.bindRequests), len(repo.queueRequests))
	}
	if len(repo.events) != 1 || repo.events[0].EventType != project.ProjectEventTaskDispatchFailed {
		t.Fatalf("expected dispatch failed event, got %#v", repo.events)
	}
}

func TestDispatchErrorRetryableClassification(t *testing.T) {
	if dispatchErrorRetryable(project.ErrInvalidProject) {
		t.Fatal("expected ErrInvalidProject to be terminal")
	}
	if dispatchErrorRetryable(project.ErrProjectNotFound) {
		t.Fatal("expected ErrProjectNotFound to be terminal")
	}
	if dispatchErrorRetryable(project.ErrProjectConflict) {
		t.Fatal("expected ErrProjectConflict to be terminal")
	}
	if dispatchErrorRetryable(&ProjectTaskRunStartError{Retryable: false, Err: errors.New("x")}) {
		t.Fatal("expected non-retryable start error to be terminal")
	}
	if !dispatchErrorRetryable(&ProjectTaskRunStartError{Retryable: true, Err: errors.New("x")}) {
		t.Fatal("expected retryable start error to be transient")
	}
	if !dispatchErrorRetryable(errors.New("db timeout")) {
		t.Fatal("expected unknown error to default to transient")
	}
}

type projectStoreMemoryRepository struct {
	project.Repository

	projectRecord project.Project
	demand        project.ProjectDemand
	demands       []project.ProjectDemand
	members       []project.ProjectMember
	tasks         []project.ProjectTask
	approvalID    uuid.UUID

	bindRequests                          []project.BindProjectTaskRunRequest
	bindAttemptRunRequests                []project.BindProjectTaskAttemptRunRequest
	dispatchStartFailures                 []project.FailQueuedProjectTaskAttemptDispatchStartRequest
	queueRequests                         []project.QueueProjectTaskRequest
	dispatchGateResults                   []project.PreDispatchGateResult
	linkGateAttemptRequests               []project.LinkPreDispatchGateAttemptRequest
	projectTaskAttempts                   []project.ProjectTaskAttempt
	bindErr                               error
	bindConflictAfterApplyingFirst        bool
	queueErr                              error
	queueConflictAfterConcurrentDispatch  bool
	advanceDemandErr                      error
	advanceDemandCalls                    int
	appendProjectEventErr                 error
	events                                []project.ProjectEvent
	coordinationJobs                      []project.CoordinationJob
	routeDecisions                        []project.RouteDecision
	planRevisions                         []project.PlanRevision
	taskDependencies                      []project.ProjectTaskDependency
	projectTaskResults                    []project.ProjectTaskResult
	demandSummaries                       []project.ProjectDemandSummary
	createDemandSummaryErr                error
	statusUpdates                         []projectTaskStatusUpdateRecord
	routeDecisionRequests                 []project.CreateRouteDecisionRequest
	projectTaskRequests                   []project.CreateProjectTaskRequest
	projectTaskGraphRequests              []project.CreateProjectTaskGraphRequest
	decomposeAcceptedPlanRevisionRequests []project.DecomposeAcceptedPlanRevisionRequest
	decisionRequests                      []project.DecisionRequest
	createDecisionRequestErr              error
	activePlacement                       *project.ProjectRuntimePlacement
	missingActivePlacement                bool

	acceptanceReady   bool
	acceptanceRecords []project.ProjectAcceptanceRecord

	getProjectCalls       int
	getProjectDemandCalls int
}

type projectTaskStatusUpdateRecord struct {
	TenantID        uuid.UUID
	TaskID          uuid.UUID
	Status          string
	CurrentStatuses []string
}

func (r *projectStoreMemoryRepository) GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (project.Project, error) {
	r.getProjectCalls++
	if r.projectRecord.TenantID == tenantID && r.projectRecord.ID == projectID {
		return r.projectRecord, nil
	}
	return project.Project{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) GetActiveProjectPlacement(ctx context.Context, tenantID, projectID uuid.UUID) (project.ProjectRuntimePlacement, error) {
	if r.missingActivePlacement {
		return project.ProjectRuntimePlacement{}, project.ErrProjectNotFound
	}
	if r.activePlacement != nil && r.activePlacement.TenantID == tenantID && r.activePlacement.ProjectID == projectID && r.activePlacement.PlacementStatus == project.ProjectRuntimePlacementStateActive {
		return *r.activePlacement, nil
	}
	if r.projectRecord.TenantID != tenantID || r.projectRecord.ID != projectID {
		return project.ProjectRuntimePlacement{}, project.ErrProjectNotFound
	}
	return project.ProjectRuntimePlacement{
		ID:              uuid.New(),
		TenantID:        tenantID,
		ProjectID:       projectID,
		RuntimeNodeID:   uuid.New(),
		PlacementStatus: project.ProjectRuntimePlacementStateActive,
	}, nil
}

// ListProjectRuntimeNodes mirrors GetActiveProjectPlacement's presence
// semantics (missingActivePlacement gates both) so scenarios that exercised the
// legacy placement-based PlacementPresent check exercise the eligibility-set
// based one the same way.
func (r *projectStoreMemoryRepository) ListProjectRuntimeNodes(ctx context.Context, tenantID, projectID uuid.UUID) ([]project.ProjectRuntimeNode, error) {
	if r.missingActivePlacement {
		return nil, nil
	}
	if r.projectRecord.TenantID != tenantID || r.projectRecord.ID != projectID {
		return nil, nil
	}
	return []project.ProjectRuntimeNode{{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, RuntimeNodeID: uuid.New()}}, nil
}

func (r *projectStoreMemoryRepository) TransitionProjectStatus(ctx context.Context, tenantID, projectID uuid.UUID, fromStatuses []string, toStatus string) (project.Project, error) {
	if r.projectRecord.TenantID != tenantID || r.projectRecord.ID != projectID {
		return project.Project{}, project.ErrProjectNotFound
	}
	for _, from := range fromStatuses {
		if string(r.projectRecord.Status) == from {
			r.projectRecord.Status = project.ProjectStatus(toStatus)
			return r.projectRecord, nil
		}
	}
	return project.Project{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) AreAllProjectDemandsTerminal(ctx context.Context, tenantID, projectID uuid.UUID) (bool, error) {
	return r.acceptanceReady, nil
}

func (r *projectStoreMemoryRepository) ArchiveProject(ctx context.Context, tenantID, projectID uuid.UUID) (project.Project, error) {
	if r.projectRecord.TenantID != tenantID || r.projectRecord.ID != projectID {
		return project.Project{}, project.ErrProjectNotFound
	}
	r.projectRecord.Status = project.ProjectStatusArchived
	return r.projectRecord, nil
}

func (r *projectStoreMemoryRepository) CreateAcceptanceRecordWithEvent(ctx context.Context, req project.CreateAcceptanceRecordWithEventRequest) (project.ProjectAcceptanceRecordWriteResult, error) {
	event := project.ProjectEvent{ID: uuid.New(), TenantID: req.Event.TenantID, ProjectID: req.Event.ProjectID, EventType: req.Event.EventType}
	r.events = append(r.events, event)
	record := project.ProjectAcceptanceRecord{
		ID:               uuid.New(),
		TenantID:         req.Acceptance.TenantID,
		ProjectID:        req.Acceptance.ProjectID,
		AcceptedByUserID: req.Acceptance.AcceptedByUserID,
		Status:           req.Acceptance.Status,
		Conclusion:       req.Acceptance.Conclusion,
		CreatedEventID:   &event.ID,
	}
	r.acceptanceRecords = append(r.acceptanceRecords, record)
	return project.ProjectAcceptanceRecordWriteResult{Event: event, Acceptance: record}, nil
}

func (r *projectStoreMemoryRepository) GetProjectDemand(ctx context.Context, tenantID, demandID uuid.UUID) (project.ProjectDemand, error) {
	r.getProjectDemandCalls++
	if r.demand.TenantID == tenantID && r.demand.ID == demandID {
		return r.demand, nil
	}
	for _, demand := range r.demands {
		if demand.TenantID == tenantID && demand.ID == demandID {
			return demand, nil
		}
	}
	return project.ProjectDemand{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) ListProjectDemands(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]project.ProjectDemand, error) {
	demands := make([]project.ProjectDemand, 0, len(r.demands)+1)
	if r.demand.TenantID == tenantID && r.demand.ProjectID == projectID && r.demand.ID != uuid.Nil {
		demands = append(demands, r.demand)
	}
	for _, demand := range r.demands {
		if demand.TenantID == tenantID && demand.ProjectID == projectID {
			demands = append(demands, demand)
		}
	}
	if offset < 0 {
		offset = 0
	}
	if int(offset) >= len(demands) {
		return []project.ProjectDemand{}, nil
	}
	demands = demands[offset:]
	if limit > 0 && int(limit) < len(demands) {
		demands = demands[:limit]
	}
	return demands, nil
}

func (r *projectStoreMemoryRepository) AdvanceProjectDemandStatus(ctx context.Context, tenantID, projectID, demandID uuid.UUID, target project.ProjectDemandStatus) error {
	r.advanceDemandCalls++
	if r.advanceDemandErr != nil {
		return r.advanceDemandErr
	}
	if r.demand.TenantID == tenantID && r.demand.ID == demandID &&
		project.ProjectDemandStatusCanAdvance(r.demand.Status, target) {
		r.demand.Status = target
	}
	return nil
}

func (r *projectStoreMemoryRepository) ListProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID) ([]project.ProjectMember, error) {
	members := make([]project.ProjectMember, 0, len(r.members))
	for _, member := range r.members {
		if member.TenantID == tenantID && member.ProjectID == projectID {
			members = append(members, member)
		}
	}
	return members, nil
}

func (r *projectStoreMemoryRepository) CreateCoordinationJob(ctx context.Context, req project.CreateCoordinationJobRequest) (project.CoordinationJob, error) {
	if req.TriggerEventID != nil {
		existing, err := r.GetCoordinationJobByTrigger(ctx, req.TenantID, req.WorkflowID, *req.TriggerEventID, req.JobType)
		if err == nil {
			return existing, nil
		}
	}
	job := project.CoordinationJob{
		ID:               uuid.New(),
		TenantID:         req.TenantID,
		ProjectID:        req.ProjectID,
		WorkflowID:       req.WorkflowID,
		TriggerEventID:   req.TriggerEventID,
		JobType:          req.JobType,
		Status:           req.Status,
		InputSnapshotRef: req.InputSnapshotRef,
		CreatedAt:        time.Now().UTC(),
	}
	r.coordinationJobs = append(r.coordinationJobs, job)
	return job, nil
}

func (r *projectStoreMemoryRepository) GetCoordinationJobByTrigger(ctx context.Context, tenantID uuid.UUID, workflowID string, triggerEventID uuid.UUID, jobType string) (project.CoordinationJob, error) {
	for _, job := range r.coordinationJobs {
		if job.TenantID == tenantID && job.WorkflowID == workflowID && job.TriggerEventID != nil && *job.TriggerEventID == triggerEventID && job.JobType == jobType {
			return job, nil
		}
	}
	return project.CoordinationJob{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) AppendProjectEvent(ctx context.Context, req project.AppendProjectEventRequest) (project.ProjectEvent, error) {
	if r.appendProjectEventErr != nil {
		return project.ProjectEvent{}, r.appendProjectEventErr
	}
	event := project.ProjectEvent{
		ID:           uuid.New(),
		TenantID:     req.TenantID,
		ProjectID:    req.ProjectID,
		EventType:    req.EventType,
		ActorType:    req.ActorType,
		ActorID:      req.ActorID,
		ResourceType: req.ResourceType,
		ResourceID:   req.ResourceID,
		Summary:      &req.Summary,
		Payload:      req.Payload,
		CreatedAt:    time.Now().UTC(),
	}
	r.events = append(r.events, event)
	return event, nil
}

func (r *projectStoreMemoryRepository) GetProjectEventByTypeAndActor(ctx context.Context, tenantID, projectID uuid.UUID, eventType project.ProjectEventType, actorID string) (project.ProjectEvent, error) {
	for i := len(r.events) - 1; i >= 0; i-- {
		event := r.events[i]
		if event.TenantID == tenantID && event.ProjectID == projectID && event.EventType == eventType && event.ActorID == actorID {
			return event, nil
		}
	}
	return project.ProjectEvent{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) CreateRouteDecision(ctx context.Context, req project.CreateRouteDecisionRequest) (project.RouteDecision, error) {
	existing, err := r.GetRouteDecisionByCoordinationJob(ctx, req.TenantID, req.CoordinationJobID)
	if err == nil {
		return existing, nil
	}
	r.routeDecisionRequests = append(r.routeDecisionRequests, req)
	decision := project.RouteDecision{
		ID:                          uuid.New(),
		TenantID:                    req.TenantID,
		ProjectID:                   req.ProjectID,
		CoordinationJobID:           req.CoordinationJobID,
		DemandID:                    req.DemandID,
		CandidateDigitalEmployeeIDs: req.CandidateDigitalEmployeeIDs,
		SelectedDigitalEmployeeIDs:  req.SelectedDigitalEmployeeIDs,
		Reason:                      req.Reason,
		InputRequirements:           req.InputRequirements,
		ExpectedOutputs:             req.ExpectedOutputs,
		BudgetEstimate:              req.BudgetEstimate,
		RequiresHumanReview:         req.RequiresHumanReview,
		CreatedEventID:              req.CreatedEventID,
		CreatedAt:                   time.Now().UTC(),
	}
	r.routeDecisions = append(r.routeDecisions, decision)
	return decision, nil
}

func (r *projectStoreMemoryRepository) GetRouteDecisionByCoordinationJob(ctx context.Context, tenantID, coordinationJobID uuid.UUID) (project.RouteDecision, error) {
	for _, decision := range r.routeDecisions {
		if decision.TenantID == tenantID && decision.CoordinationJobID == coordinationJobID {
			return decision, nil
		}
	}
	return project.RouteDecision{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) GetRouteDecision(ctx context.Context, tenantID, routeDecisionID uuid.UUID) (project.RouteDecision, error) {
	for _, decision := range r.routeDecisions {
		if decision.TenantID == tenantID && decision.ID == routeDecisionID {
			return decision, nil
		}
	}
	return project.RouteDecision{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) CreatePlanRevision(ctx context.Context, req project.CreatePlanRevisionRequest) (project.PlanRevision, error) {
	revisionNumber := int32(1)
	for _, revision := range r.planRevisions {
		if revision.TenantID == req.TenantID && revision.ProjectID == req.ProjectID && revision.DemandID == req.DemandID && revision.RevisionNumber >= revisionNumber {
			revisionNumber = revision.RevisionNumber + 1
		}
	}
	revision := project.PlanRevision{
		ID:                 uuid.New(),
		TenantID:           req.TenantID,
		TeamID:             req.TeamID,
		ProjectID:          req.ProjectID,
		DemandID:           req.DemandID,
		CoordinationJobID:  req.CoordinationJobID,
		RouteDecisionID:    req.RouteDecisionID,
		RevisionNumber:     revisionNumber,
		Status:             req.Status,
		Payload:            cloneAnyMap(req.Payload),
		PlannerProvider:    req.PlannerProvider,
		PlannerModel:       req.PlannerModel,
		PlannerInputHash:   req.PlannerInputHash,
		PlanFingerprint:    req.PlanFingerprint,
		ValidationErrors:   append([]string(nil), req.ValidationErrors...),
		ValidationWarnings: append([]string(nil), req.ValidationWarnings...),
		ReviewRequired:     req.ReviewRequired,
		ReviewReason:       req.ReviewReason,
		CreatedEventID:     req.CreatedEventID,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	r.planRevisions = append(r.planRevisions, revision)
	if req.SupersedeOpenRevisions {
		for index := range r.planRevisions {
			if r.planRevisions[index].ID == revision.ID ||
				r.planRevisions[index].TenantID != req.TenantID ||
				r.planRevisions[index].ProjectID != req.ProjectID ||
				r.planRevisions[index].DemandID != req.DemandID ||
				!project.IsMutablePlanRevisionStatus(r.planRevisions[index].Status) {
				continue
			}
			r.planRevisions[index].Status = project.PlanRevisionStatusSuperseded
			r.planRevisions[index].SupersededByRevisionID = &revision.ID
		}
	}
	return revision, nil
}

func (r *projectStoreMemoryRepository) GetPlanRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (project.PlanRevision, error) {
	for _, revision := range r.planRevisions {
		if revision.TenantID == tenantID && revision.ProjectID == projectID && revision.ID == revisionID {
			return revision, nil
		}
	}
	return project.PlanRevision{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) ListPlanRevisions(ctx context.Context, req project.ListPlanRevisionsRequest) ([]project.PlanRevision, error) {
	revisions := make([]project.PlanRevision, 0, len(r.planRevisions))
	for _, revision := range r.planRevisions {
		if revision.TenantID != req.TenantID || revision.ProjectID != req.ProjectID {
			continue
		}
		if req.DemandID != nil && revision.DemandID != *req.DemandID {
			continue
		}
		revisions = append(revisions, revision)
	}
	sort.SliceStable(revisions, func(i, j int) bool {
		if revisions[i].DemandID == revisions[j].DemandID {
			return revisions[i].RevisionNumber < revisions[j].RevisionNumber
		}
		return revisions[i].DemandID.String() < revisions[j].DemandID.String()
	})
	return revisions, nil
}

func (r *projectStoreMemoryRepository) ListPlanRevisionsForDemand(ctx context.Context, tenantID, projectID, demandID uuid.UUID) ([]project.PlanRevision, error) {
	return r.ListPlanRevisions(ctx, project.ListPlanRevisionsRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		DemandID:  &demandID,
	})
}

func (r *projectStoreMemoryRepository) AcceptPlanRevision(ctx context.Context, req project.AcceptPlanRevisionRequest) (project.PlanRevision, error) {
	for index, revision := range r.planRevisions {
		if revision.TenantID == req.TenantID && revision.ProjectID == req.ProjectID && revision.ID == req.RevisionID {
			revision.Status = project.PlanRevisionStatusAccepted
			revision.AcceptedBy = req.AcceptedBy
			now := time.Now().UTC()
			revision.AcceptedAt = &now
			r.planRevisions[index] = revision
			return revision, nil
		}
	}
	return project.PlanRevision{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) RejectPlanRevision(ctx context.Context, req project.RejectPlanRevisionRequest) (project.PlanRevision, error) {
	for index, revision := range r.planRevisions {
		if revision.TenantID == req.TenantID && revision.ProjectID == req.ProjectID && revision.ID == req.RevisionID {
			revision.Status = project.PlanRevisionStatusRejected
			revision.RejectedBy = req.RejectedBy
			revision.RejectionReason = req.RejectionReason
			now := time.Now().UTC()
			revision.RejectedAt = &now
			r.planRevisions[index] = revision
			return revision, nil
		}
	}
	return project.PlanRevision{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) CreateProjectTask(ctx context.Context, req project.CreateProjectTaskRequest) (project.ProjectTask, error) {
	r.projectTaskRequests = append(r.projectTaskRequests, req)
	var summary *string
	if req.Summary != "" {
		summary = strPtr(req.Summary)
	}
	var riskLevel *string
	if req.RiskLevel != "" {
		riskLevel = strPtr(req.RiskLevel)
	}
	task := project.ProjectTask{
		ID:                        uuid.New(),
		TenantID:                  req.TenantID,
		ProjectID:                 req.ProjectID,
		DemandID:                  req.DemandID,
		Title:                     req.Title,
		Summary:                   summary,
		Status:                    req.Status,
		AssignedDigitalEmployeeID: req.AssignedDigitalEmployeeID,
		RuntimeTaskID:             req.RuntimeTaskID,
		DigitalEmployeeRunID:      req.DigitalEmployeeRunID,
		RiskLevel:                 riskLevel,
		RequiresHumanApproval:     req.RequiresHumanApproval,
		CoordinationJobID:         req.CoordinationJobID,
		RouteDecisionID:           req.RouteDecisionID,
		PlannedTaskKey:            req.PlannedTaskKey,
		TaskKind:                  req.TaskKind,
		StageIndex:                req.StageIndex,
		RevisionOfTaskID:          req.RevisionOfTaskID,
		AcceptedPlanRevisionID:    req.AcceptedPlanRevisionID,
		DecompositionClaimKey:     req.DecompositionClaimKey,
		ExpectedOutputs:           req.ExpectedOutputs,
		InputRequirements:         req.InputRequirements,
		HandoffContract:           req.HandoffContract,
		PlannerMetadata:           req.PlannerMetadata,
		BlockedByTaskIDs:          req.BlockedByTaskIDs,
		CreatedAt:                 time.Now().UTC(),
		UpdatedAt:                 time.Now().UTC(),
	}
	r.tasks = append(r.tasks, task)
	return task, nil
}

func (r *projectStoreMemoryRepository) CreateProjectTaskGraph(ctx context.Context, req project.CreateProjectTaskGraphRequest) (project.CreateProjectTaskGraphResult, error) {
	r.projectTaskGraphRequests = append(r.projectTaskGraphRequests, req)
	return r.createProjectTaskGraphInMemory(req, nil, nil)
}

func (r *projectStoreMemoryRepository) DecomposeAcceptedPlanRevision(ctx context.Context, req project.DecomposeAcceptedPlanRevisionRequest) (project.DecomposeAcceptedPlanRevisionResult, error) {
	r.decomposeAcceptedPlanRevisionRequests = append(r.decomposeAcceptedPlanRevisionRequests, req)
	revision, err := r.GetPlanRevision(ctx, req.TenantID, req.ProjectID, req.AcceptedPlanRevisionID)
	if err != nil && len(r.planRevisions) > 0 {
		return project.DecomposeAcceptedPlanRevisionResult{}, err
	}
	if err == nil && (revision.DemandID != req.DemandID || !project.IsAcceptedPlanRevisionStatus(revision.Status) || revision.PlanFingerprint != req.PlanFingerprint) {
		return project.DecomposeAcceptedPlanRevisionResult{}, project.ErrProjectConflict
	}
	existing := make([]project.ProjectTask, 0)
	for _, task := range r.tasks {
		if task.TenantID != req.TenantID || task.ProjectID != req.ProjectID || task.DemandID == nil || *task.DemandID != req.DemandID ||
			task.AcceptedPlanRevisionID == nil || *task.AcceptedPlanRevisionID != req.AcceptedPlanRevisionID {
			continue
		}
		if task.DecompositionClaimKey == nil || *task.DecompositionClaimKey != req.DecompositionClaimKey {
			return project.DecomposeAcceptedPlanRevisionResult{}, project.ErrProjectConflict
		}
		existing = append(existing, task)
	}
	if len(existing) > 0 {
		if len(existing) != len(req.Tasks) {
			return project.DecomposeAcceptedPlanRevisionResult{}, project.ErrProjectConflict
		}
		existingByKey := map[string]project.ProjectTask{}
		existingIDs := map[uuid.UUID]struct{}{}
		for _, task := range existing {
			if task.PlannedTaskKey == nil {
				return project.DecomposeAcceptedPlanRevisionResult{}, project.ErrProjectConflict
			}
			existingByKey[*task.PlannedTaskKey] = task
			existingIDs[task.ID] = struct{}{}
		}
		for _, planned := range req.Tasks {
			task, ok := existingByKey[planned.Key]
			if !ok || task.Title != planned.Title || task.Status != planned.Status {
				return project.DecomposeAcceptedPlanRevisionResult{}, project.ErrProjectConflict
			}
		}
		dependencies := make([]project.ProjectTaskDependency, 0)
		for _, dependency := range r.taskDependencies {
			if dependency.TenantID != req.TenantID || dependency.ProjectID != req.ProjectID {
				continue
			}
			if _, ok := existingIDs[dependency.DependentTaskID]; ok {
				dependencies = append(dependencies, dependency)
			}
		}
		return project.DecomposeAcceptedPlanRevisionResult{Tasks: existing, Dependencies: dependencies, Replayed: true}, nil
	}
	graph, err := r.createProjectTaskGraphInMemory(project.CreateProjectTaskGraphRequest{
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		DemandID:          req.DemandID,
		CoordinationJobID: req.CoordinationJobID,
		RouteDecisionID:   req.RouteDecisionID,
		Tasks:             req.Tasks,
	}, &req.AcceptedPlanRevisionID, &req.DecompositionClaimKey)
	if err != nil {
		return project.DecomposeAcceptedPlanRevisionResult{}, err
	}
	tasks := make([]project.ProjectTask, 0, len(graph.Tasks))
	for _, created := range graph.Tasks {
		for _, task := range r.tasks {
			if task.ID == created.ID {
				tasks = append(tasks, task)
				break
			}
		}
	}
	return project.DecomposeAcceptedPlanRevisionResult{Tasks: tasks, Dependencies: graph.Dependencies}, nil
}

func (r *projectStoreMemoryRepository) createProjectTaskGraphInMemory(req project.CreateProjectTaskGraphRequest, acceptedPlanRevisionID *uuid.UUID, decompositionClaimKey *string) (project.CreateProjectTaskGraphResult, error) {
	result := project.CreateProjectTaskGraphResult{
		Tasks:        make([]project.ProjectTaskGraphTaskResult, 0, len(req.Tasks)),
		Dependencies: []project.ProjectTaskDependency{},
		GraphEventID: uuid.New(),
	}
	keyToID := map[string]uuid.UUID{}
	for _, planned := range req.Tasks {
		id := uuid.New()
		keyToID[planned.Key] = id
		demandID := req.DemandID
		coordinationJobID := req.CoordinationJobID
		routeDecisionID := req.RouteDecisionID
		employeeID := planned.AssignedDigitalEmployeeID
		taskKind := planned.TaskKind
		r.tasks = append(r.tasks, project.ProjectTask{
			ID:                        id,
			TenantID:                  req.TenantID,
			ProjectID:                 req.ProjectID,
			DemandID:                  &demandID,
			Title:                     planned.Title,
			Summary:                   strPtr(planned.Summary),
			Status:                    planned.Status,
			AssignedDigitalEmployeeID: &employeeID,
			RiskLevel:                 strPtr(planned.RiskLevel),
			RequiresHumanApproval:     planned.RequiresHumanApproval,
			CoordinationJobID:         &coordinationJobID,
			RouteDecisionID:           &routeDecisionID,
			PlannedTaskKey:            strPtr(planned.Key),
			TaskKind:                  &taskKind,
			StageIndex:                planned.StageIndex,
			AcceptedPlanRevisionID:    acceptedPlanRevisionID,
			DecompositionClaimKey:     decompositionClaimKey,
			ExpectedOutputs:           planned.ExpectedOutputs,
			InputRequirements:         planned.InputRequirements,
			HandoffContract:           planned.HandoffContract,
			PlannerMetadata:           planned.PlannerMetadata,
			CreatedAt:                 time.Now().UTC(),
			UpdatedAt:                 time.Now().UTC(),
		})
		result.Tasks = append(result.Tasks, project.ProjectTaskGraphTaskResult{
			ID:             id,
			PlannedTaskKey: planned.Key,
			StageIndex:     planned.StageIndex,
			CreatedEventID: uuid.New(),
			IsRoot:         len(planned.BlockedByKeys) == 0,
		})
	}
	for _, planned := range req.Tasks {
		for _, blockerKey := range planned.BlockedByKeys {
			result.Dependencies = append(result.Dependencies, project.ProjectTaskDependency{
				ID:                uuid.New(),
				TenantID:          req.TenantID,
				ProjectID:         req.ProjectID,
				CoordinationJobID: &req.CoordinationJobID,
				DependentTaskID:   keyToID[planned.Key],
				BlockerTaskID:     keyToID[blockerKey],
			})
		}
	}
	r.taskDependencies = append(r.taskDependencies, result.Dependencies...)
	return result, nil
}

func (r *projectStoreMemoryRepository) CreateProjectTaskDependency(ctx context.Context, req project.CreateProjectTaskDependencyRequest) (project.ProjectTaskDependency, error) {
	for _, dependency := range r.taskDependencies {
		if dependency.TenantID == req.TenantID && dependency.DependentTaskID == req.DependentTaskID && dependency.BlockerTaskID == req.BlockerTaskID {
			return dependency, nil
		}
	}
	dependency := project.ProjectTaskDependency{
		ID:                uuid.New(),
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		CoordinationJobID: req.CoordinationJobID,
		DependentTaskID:   req.DependentTaskID,
		BlockerTaskID:     req.BlockerTaskID,
	}
	r.taskDependencies = append(r.taskDependencies, dependency)
	return dependency, nil
}

func (r *projectStoreMemoryRepository) RewireProjectTaskDependencies(ctx context.Context, req project.RewireProjectTaskDependenciesRequest) ([]project.ProjectTaskDependency, error) {
	requested := map[uuid.UUID]struct{}{}
	for _, taskID := range req.DependentTaskIDs {
		requested[taskID] = struct{}{}
	}
	rewired := make([]project.ProjectTaskDependency, 0)
	for index := 0; index < len(r.taskDependencies); {
		dependency := r.taskDependencies[index]
		if dependency.TenantID != req.TenantID || dependency.ProjectID != req.ProjectID || dependency.BlockerTaskID != req.OldBlockerTaskID {
			index++
			continue
		}
		if _, ok := requested[dependency.DependentTaskID]; !ok {
			index++
			continue
		}
		if dependencyExists(r.taskDependencies, dependency.DependentTaskID, req.NewBlockerTaskID) {
			r.taskDependencies = append(r.taskDependencies[:index], r.taskDependencies[index+1:]...)
			continue
		}
		dependency.BlockerTaskID = req.NewBlockerTaskID
		r.taskDependencies[index] = dependency
		rewired = append(rewired, dependency)
		index++
	}
	return rewired, nil
}

func (r *projectStoreMemoryRepository) ListProjectTaskDependencies(ctx context.Context, tenantID, projectID uuid.UUID, dependentTaskIDs []uuid.UUID) ([]project.ProjectTaskDependency, error) {
	requested := map[uuid.UUID]struct{}{}
	for _, taskID := range dependentTaskIDs {
		requested[taskID] = struct{}{}
	}
	dependencies := make([]project.ProjectTaskDependency, 0)
	for _, dependency := range r.taskDependencies {
		if dependency.TenantID != tenantID || dependency.ProjectID != projectID {
			continue
		}
		if _, ok := requested[dependency.DependentTaskID]; !ok {
			continue
		}
		dependencies = append(dependencies, dependency)
	}
	return dependencies, nil
}

func (r *projectStoreMemoryRepository) GetProjectTask(ctx context.Context, tenantID, projectTaskID uuid.UUID) (project.ProjectTask, error) {
	for _, task := range r.tasks {
		if task.TenantID == tenantID && task.ID == projectTaskID {
			return task, nil
		}
	}
	return project.ProjectTask{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) GetCurrentProjectTaskAttempt(ctx context.Context, tenantID, projectTaskID uuid.UUID) (project.ProjectTaskAttempt, error) {
	for _, attempt := range r.projectTaskAttempts {
		if attempt.TenantID == tenantID && attempt.ProjectTaskID == projectTaskID &&
			(attempt.Status == project.ProjectTaskAttemptStatusQueued || attempt.Status == project.ProjectTaskAttemptStatusRunning) {
			return attempt, nil
		}
	}
	return project.ProjectTaskAttempt{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) GetProjectTaskAttempt(ctx context.Context, tenantID, attemptID uuid.UUID) (project.ProjectTaskAttempt, error) {
	for _, attempt := range r.projectTaskAttempts {
		if attempt.TenantID == tenantID && attempt.ID == attemptID {
			return attempt, nil
		}
	}
	return project.ProjectTaskAttempt{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) BindProjectTaskRun(ctx context.Context, req project.BindProjectTaskRunRequest) (project.ProjectTask, error) {
	r.bindRequests = append(r.bindRequests, req)
	if r.bindErr != nil {
		return project.ProjectTask{}, r.bindErr
	}
	for i, task := range r.tasks {
		if task.TenantID != req.TenantID || task.ID != req.ProjectTaskID {
			continue
		}
		if task.DigitalEmployeeRunID != nil {
			if task.RuntimeTaskID != nil && *task.DigitalEmployeeRunID == req.DigitalEmployeeRunID && *task.RuntimeTaskID == req.RuntimeTaskID {
				return task, nil
			}
			return project.ProjectTask{}, project.ErrProjectConflict
		}
		task.Status = "assigned"
		task.DigitalEmployeeRunID = &req.DigitalEmployeeRunID
		task.RuntimeTaskID = &req.RuntimeTaskID
		task.UpdatedAt = time.Now().UTC()
		r.tasks[i] = task
		return task, nil
	}
	return project.ProjectTask{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) BindProjectTaskAttemptRun(ctx context.Context, req project.BindProjectTaskAttemptRunRequest) (project.ProjectTaskAttemptRunBindingResult, error) {
	r.bindAttemptRunRequests = append(r.bindAttemptRunRequests, req)
	if r.bindErr != nil {
		return project.ProjectTaskAttemptRunBindingResult{}, r.bindErr
	}
	attemptIndex := -1
	for index, attempt := range r.projectTaskAttempts {
		if attempt.TenantID == req.TenantID && attempt.ProjectTaskID == req.ProjectTaskID && attempt.ID == req.AttemptID {
			attemptIndex = index
			break
		}
	}
	if attemptIndex == -1 {
		return project.ProjectTaskAttemptRunBindingResult{}, project.ErrProjectNotFound
	}
	attempt := r.projectTaskAttempts[attemptIndex]
	if attempt.Status != project.ProjectTaskAttemptStatusQueued {
		return project.ProjectTaskAttemptRunBindingResult{}, project.ErrProjectConflict
	}
	if attempt.DigitalEmployeeRunID != nil && *attempt.DigitalEmployeeRunID != req.DigitalEmployeeRunID {
		return project.ProjectTaskAttemptRunBindingResult{}, project.ErrProjectConflict
	}
	if attempt.RuntimeTaskID != nil && *attempt.RuntimeTaskID != req.RuntimeTaskID {
		return project.ProjectTaskAttemptRunBindingResult{}, project.ErrProjectConflict
	}
	if attempt.RuntimeNodeID != nil && *attempt.RuntimeNodeID != req.RuntimeNodeID {
		return project.ProjectTaskAttemptRunBindingResult{}, project.ErrProjectConflict
	}
	attempt.DigitalEmployeeRunID = &req.DigitalEmployeeRunID
	attempt.RuntimeTaskID = &req.RuntimeTaskID
	attempt.RuntimeNodeID = &req.RuntimeNodeID
	attempt.ProviderType = strPtrOrNil(req.ProviderType)
	attempt.ExecutionContextPacket = cloneAnyMap(req.ExecutionContextPacket)
	attempt.ExecutionContextPacketVersion = req.ExecutionContextPacketVersion
	attempt.UpdatedAt = time.Now().UTC()
	r.projectTaskAttempts[attemptIndex] = attempt

	for index, task := range r.tasks {
		if task.TenantID != req.TenantID || task.ProjectID != req.ProjectID || task.ID != req.ProjectTaskID {
			continue
		}
		if task.CurrentAttemptID == nil || *task.CurrentAttemptID != req.AttemptID || task.Status != project.ProjectTaskStatusQueued {
			return project.ProjectTaskAttemptRunBindingResult{}, project.ErrProjectConflict
		}
		task.DigitalEmployeeRunID = &req.DigitalEmployeeRunID
		task.RuntimeTaskID = &req.RuntimeTaskID
		task.UpdatedAt = time.Now().UTC()
		r.tasks[index] = task
		if r.bindConflictAfterApplyingFirst && len(r.bindAttemptRunRequests) == 1 {
			return project.ProjectTaskAttemptRunBindingResult{}, project.ErrProjectConflict
		}
		return project.ProjectTaskAttemptRunBindingResult{Task: task, Attempt: attempt}, nil
	}
	return project.ProjectTaskAttemptRunBindingResult{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) FailQueuedProjectTaskAttemptDispatchStart(ctx context.Context, req project.FailQueuedProjectTaskAttemptDispatchStartRequest) (project.ProjectTaskWritebackResult, error) {
	r.dispatchStartFailures = append(r.dispatchStartFailures, req)
	attemptIndex := -1
	for index, attempt := range r.projectTaskAttempts {
		if attempt.TenantID == req.TenantID && attempt.ProjectTaskID == req.ProjectTaskID && attempt.ID == req.AttemptID {
			attemptIndex = index
			break
		}
	}
	if attemptIndex == -1 {
		return project.ProjectTaskWritebackResult{}, project.ErrProjectNotFound
	}
	attempt := r.projectTaskAttempts[attemptIndex]
	if attempt.Status != project.ProjectTaskAttemptStatusQueued || attempt.LeaseToken != req.LeaseToken {
		return project.ProjectTaskWritebackResult{}, project.ErrProjectConflict
	}
	now := time.Now().UTC()
	attempt.Status = project.ProjectTaskAttemptStatusLost
	attempt.FinishedAt = &now
	attempt.Retryable = &req.Retryable
	attempt.FailureFamily = strPtrOrNil(req.FailureFamily)
	attempt.FailureMessage = strPtrOrNil(req.FailureSummary)
	attempt.TerminalEventID = req.DispatchFailureEventID
	attempt.UpdatedAt = now
	r.projectTaskAttempts[attemptIndex] = attempt

	for index, task := range r.tasks {
		if task.TenantID != req.TenantID || task.ProjectID != req.ProjectID || task.ID != req.ProjectTaskID {
			continue
		}
		if task.CurrentAttemptID == nil || *task.CurrentAttemptID != req.AttemptID || task.Status != project.ProjectTaskStatusQueued {
			return project.ProjectTaskWritebackResult{}, project.ErrProjectConflict
		}
		task.Status = req.RestoreTaskStatus
		if task.Status == "" {
			task.Status = project.ProjectTaskStatusPlanned
		}
		if req.ClearCurrentAttempt {
			task.CurrentAttemptID = nil
		}
		task.RetryNotBefore = req.RetryNotBefore
		task.UpdatedAt = now
		r.tasks[index] = task
		var event project.ProjectEvent
		if req.DispatchFailureEventID != nil {
			for _, candidate := range r.events {
				if candidate.TenantID == req.TenantID && candidate.ProjectID == req.ProjectID && candidate.ID == *req.DispatchFailureEventID {
					event = candidate
					break
				}
			}
		}
		return project.ProjectTaskWritebackResult{Task: task, Event: event}, nil
	}
	return project.ProjectTaskWritebackResult{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) RecordPreDispatchGateResult(ctx context.Context, req project.RecordPreDispatchGateResultRequest) (project.PreDispatchGateResult, error) {
	now := time.Now().UTC()
	checkedAt := req.CheckedAt
	if checkedAt.IsZero() {
		checkedAt = now
	}
	for index, task := range r.tasks {
		if task.TenantID != req.TenantID || task.ProjectID != req.ProjectID || task.ID != req.ProjectTaskID {
			continue
		}
		for gateIndex, gate := range r.dispatchGateResults {
			if gate.TenantID == req.TenantID && gate.ProjectID == req.ProjectID && gate.ProjectTaskID == req.ProjectTaskID && gate.IdempotencyKey == req.IdempotencyKey {
				if gate.AttemptID != nil || gate.DecisionRequestID != nil {
					return gate, nil
				}
				gate.Status = req.Status
				gate.CheckedAt = checkedAt
				gate.Checks = append([]project.PreDispatchGateCheck(nil), req.Checks...)
				gate.Blockers = append([]project.PreDispatchGateBlocker(nil), req.Blockers...)
				gate.HumanActionRequest = cloneHumanActionRequest(req.HumanActionRequest)
				gate.RetryAfter = req.RetryAfter
				gate.UpdatedAt = now
				r.dispatchGateResults[gateIndex] = gate
				r.tasks[index].LatestDispatchGateResultID = &gate.ID
				return gate, nil
			}
		}
		gate := project.PreDispatchGateResult{
			ID:                     uuid.New(),
			TenantID:               req.TenantID,
			ProjectID:              req.ProjectID,
			ProjectTaskID:          req.ProjectTaskID,
			AcceptedPlanRevisionID: req.AcceptedPlanRevisionID,
			PlannedTaskKey:         req.PlannedTaskKey,
			SelectedEmployeeID:     req.SelectedEmployeeID,
			AttemptNo:              req.AttemptNo,
			DispatchReason:         req.DispatchReason,
			IdempotencyKey:         req.IdempotencyKey,
			DispatchToken:          req.DispatchToken,
			Status:                 req.Status,
			CheckedAt:              checkedAt,
			Checks:                 append([]project.PreDispatchGateCheck(nil), req.Checks...),
			Blockers:               append([]project.PreDispatchGateBlocker(nil), req.Blockers...),
			HumanActionRequest:     cloneHumanActionRequest(req.HumanActionRequest),
			RetryAfter:             req.RetryAfter,
			CreatedEventID:         req.CreatedEventID,
			CreatedAt:              now,
			UpdatedAt:              now,
		}
		r.dispatchGateResults = append(r.dispatchGateResults, gate)
		r.tasks[index].LatestDispatchGateResultID = &gate.ID
		return gate, nil
	}
	return project.PreDispatchGateResult{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) LinkPreDispatchGateAttempt(ctx context.Context, req project.LinkPreDispatchGateAttemptRequest) (project.PreDispatchGateResult, error) {
	r.linkGateAttemptRequests = append(r.linkGateAttemptRequests, req)
	gateIndex := -1
	for index, gate := range r.dispatchGateResults {
		if gate.TenantID == req.TenantID && gate.ProjectID == req.ProjectID && gate.ProjectTaskID == req.ProjectTaskID && gate.ID == req.GateResultID {
			if gate.AttemptID != nil && *gate.AttemptID != req.AttemptID {
				return project.PreDispatchGateResult{}, project.ErrProjectNotFound
			}
			gateIndex = index
			break
		}
	}
	if gateIndex == -1 {
		return project.PreDispatchGateResult{}, project.ErrProjectNotFound
	}
	attemptIndex := -1
	for index, attempt := range r.projectTaskAttempts {
		if attempt.TenantID == req.TenantID && attempt.ProjectTaskID == req.ProjectTaskID && attempt.ID == req.AttemptID {
			if attempt.DispatchGateResultID != nil && *attempt.DispatchGateResultID != req.GateResultID {
				return project.PreDispatchGateResult{}, project.ErrProjectNotFound
			}
			attemptIndex = index
			break
		}
	}
	if attemptIndex == -1 {
		return project.PreDispatchGateResult{}, project.ErrProjectNotFound
	}
	now := time.Now().UTC()
	gate := r.dispatchGateResults[gateIndex]
	gate.AttemptID = &req.AttemptID
	gate.UpdatedAt = now
	r.dispatchGateResults[gateIndex] = gate
	attempt := r.projectTaskAttempts[attemptIndex]
	attempt.DispatchGateResultID = &req.GateResultID
	attempt.UpdatedAt = now
	r.projectTaskAttempts[attemptIndex] = attempt
	return gate, nil
}

func (r *projectStoreMemoryRepository) LinkPreDispatchGateDecisionRequest(ctx context.Context, req project.LinkPreDispatchGateDecisionRequest) (project.PreDispatchGateResult, error) {
	gateIndex := -1
	for index, gate := range r.dispatchGateResults {
		if gate.TenantID == req.TenantID && gate.ProjectID == req.ProjectID && gate.ProjectTaskID == req.ProjectTaskID && gate.ID == req.GateResultID {
			if gate.DecisionRequestID != nil && *gate.DecisionRequestID != req.DecisionRequestID {
				return project.PreDispatchGateResult{}, project.ErrProjectNotFound
			}
			gateIndex = index
			break
		}
	}
	if gateIndex == -1 {
		return project.PreDispatchGateResult{}, project.ErrProjectNotFound
	}
	decisionIndex := -1
	for index, decision := range r.decisionRequests {
		if decision.TenantID == req.TenantID && decision.ProjectID == req.ProjectID && decision.ID == req.DecisionRequestID {
			if decision.ProjectTaskID == nil || *decision.ProjectTaskID != req.ProjectTaskID {
				return project.PreDispatchGateResult{}, project.ErrProjectNotFound
			}
			if decision.DispatchGateResultID != nil && *decision.DispatchGateResultID != req.GateResultID {
				return project.PreDispatchGateResult{}, project.ErrProjectNotFound
			}
			decisionIndex = index
			break
		}
	}
	if decisionIndex == -1 {
		return project.PreDispatchGateResult{}, project.ErrProjectNotFound
	}
	now := time.Now().UTC()
	gate := r.dispatchGateResults[gateIndex]
	gate.DecisionRequestID = &req.DecisionRequestID
	gate.UpdatedAt = now
	r.dispatchGateResults[gateIndex] = gate
	decision := r.decisionRequests[decisionIndex]
	decision.DispatchGateResultID = &req.GateResultID
	decision.UpdatedAt = now
	r.decisionRequests[decisionIndex] = decision
	return gate, nil
}

func (r *projectStoreMemoryRepository) MoveProjectTaskToWaitingHumanForPreDispatchGate(ctx context.Context, req project.MoveProjectTaskToWaitingHumanForPreDispatchGateRequest) (project.ProjectTask, error) {
	for _, gate := range r.dispatchGateResults {
		if gate.TenantID == req.TenantID && gate.ProjectID == req.ProjectID && gate.ProjectTaskID == req.ProjectTaskID && gate.ID == req.GateResultID {
			for index, task := range r.tasks {
				if task.TenantID != req.TenantID || task.ProjectID != req.ProjectID || task.ID != req.ProjectTaskID {
					continue
				}
				if task.Status != project.ProjectTaskStatusPlanned && task.Status != project.ProjectTaskStatusWaitingHuman {
					return project.ProjectTask{}, project.ErrProjectConflict
				}
				now := time.Now().UTC()
				task.Status = project.ProjectTaskStatusWaitingHuman
				task.WaitingReason = strPtr(req.WaitingReason)
				task.WaitingRequestID = &req.DecisionRequestID
				task.LatestDispatchGateResultID = &req.GateResultID
				task.StatusChangedAt = now
				task.UpdatedAt = now
				r.tasks[index] = task
				return task, nil
			}
		}
	}
	return project.ProjectTask{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) QueueProjectTaskWithAttempt(ctx context.Context, req project.QueueProjectTaskRequest) (project.QueueProjectTaskResult, error) {
	r.queueRequests = append(r.queueRequests, req)
	if r.queueErr != nil {
		return project.QueueProjectTaskResult{}, r.queueErr
	}
	for _, attempt := range r.projectTaskAttempts {
		if attempt.TenantID != req.TenantID || attempt.IdempotencyKey != req.IdempotencyKey {
			continue
		}
		if attempt.ProjectTaskID != req.ProjectTaskID {
			return project.QueueProjectTaskResult{}, project.ErrProjectConflict
		}
		task, err := r.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
		if err != nil {
			return project.QueueProjectTaskResult{}, err
		}
		var event project.ProjectEvent
		if attempt.CreatedEventID != nil {
			for _, candidate := range r.events {
				if candidate.TenantID == req.TenantID && candidate.ProjectID == req.ProjectID && candidate.ID == *attempt.CreatedEventID {
					event = candidate
					break
				}
			}
		}
		return project.QueueProjectTaskResult{Task: task, Attempt: attempt, Event: event}, nil
	}
	for i, task := range r.tasks {
		if task.TenantID != req.TenantID || task.ProjectID != req.ProjectID || task.ID != req.ProjectTaskID {
			continue
		}
		if task.Status != project.ProjectTaskStatusPlanned && task.Status != project.ProjectTaskStatusWaitingHuman {
			return project.QueueProjectTaskResult{}, project.ErrProjectConflict
		}
		if task.AssignedDigitalEmployeeID != nil && *task.AssignedDigitalEmployeeID != req.DigitalEmployeeID {
			return project.QueueProjectTaskResult{}, project.ErrProjectTaskForbidden
		}
		attemptID := uuid.New()
		if req.ProjectTaskAttemptID != nil {
			attemptID = *req.ProjectTaskAttemptID
		}
		attemptNo := task.AttemptCount + 1
		event, err := r.AppendProjectEvent(ctx, project.AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    req.ProjectID,
			EventType:    project.ProjectEventTaskDispatched,
			ActorType:    "project_coordinator",
			ActorID:      req.ProjectTaskID.String(),
			ResourceType: strPtr("project_task"),
			ResourceID:   strPtr(req.ProjectTaskID.String()),
			Summary:      "项目任务已排队",
			Payload:      projectStoreQueueTaskEventPayload(req, attemptID, attemptNo),
		})
		if err != nil {
			return project.QueueProjectTaskResult{}, err
		}
		packet := req.ExecutionContextPacket
		if packet == nil {
			packet = map[string]any{}
		}
		version := strings.TrimSpace(req.ExecutionContextPacketVersion)
		if version == "" {
			version = "v1"
		}
		attempt := project.ProjectTaskAttempt{
			ID:                            attemptID,
			TenantID:                      req.TenantID,
			ProjectTaskID:                 req.ProjectTaskID,
			AttemptNo:                     attemptNo,
			Status:                        project.ProjectTaskAttemptStatusQueued,
			DigitalEmployeeID:             &req.DigitalEmployeeID,
			ProviderType:                  strPtrOrNil(req.ProviderType),
			DigitalEmployeeRunID:          req.DigitalEmployeeRunID,
			RuntimeTaskID:                 req.RuntimeTaskID,
			RuntimeNodeID:                 req.RuntimeNodeID,
			ExecutionContextPacket:        packet,
			ExecutionContextPacketVersion: version,
			LeaseToken:                    req.LeaseToken,
			LeaseExpiresAt:                req.LeaseExpiresAt,
			IdempotencyKey:                req.IdempotencyKey,
			DispatchGateResultID:          req.DispatchGateResultID,
			CreatedEventID:                &event.ID,
			CreatedAt:                     time.Now().UTC(),
			UpdatedAt:                     time.Now().UTC(),
		}
		r.projectTaskAttempts = append(r.projectTaskAttempts, attempt)
		now := time.Now().UTC()
		task.Status = project.ProjectTaskStatusQueued
		task.CurrentAttemptID = &attempt.ID
		task.AttemptCount++
		task.DigitalEmployeeRunID = req.DigitalEmployeeRunID
		task.RuntimeTaskID = req.RuntimeTaskID
		task.RetryNotBefore = nil
		task.WaitingReason = nil
		task.WaitingRequestID = nil
		task.StatusChangedAt = now
		task.UpdatedAt = now
		if r.queueConflictAfterConcurrentDispatch && len(r.queueRequests) == 1 {
			concurrentRunID := uuid.New()
			concurrentRuntimeTaskID := uuid.New()
			concurrentRuntimeNodeID := uuid.New()
			attempt.DigitalEmployeeRunID = &concurrentRunID
			attempt.RuntimeTaskID = &concurrentRuntimeTaskID
			attempt.RuntimeNodeID = &concurrentRuntimeNodeID
			r.projectTaskAttempts[len(r.projectTaskAttempts)-1] = attempt
			task.DigitalEmployeeRunID = &concurrentRunID
			task.RuntimeTaskID = &concurrentRuntimeTaskID
			r.tasks[i] = task
			return project.QueueProjectTaskResult{}, project.ErrProjectConflict
		}
		r.tasks[i] = task
		return project.QueueProjectTaskResult{Task: task, Attempt: attempt, Event: event}, nil
	}
	return project.QueueProjectTaskResult{}, project.ErrProjectNotFound
}

func projectStoreQueueTaskEventPayload(req project.QueueProjectTaskRequest, attemptID uuid.UUID, attemptNo int32) map[string]any {
	payload := map[string]any{
		"project_task_id":         req.ProjectTaskID.String(),
		"project_task_attempt_id": attemptID.String(),
		"project_task_status":     project.ProjectTaskStatusQueued,
		"digital_employee_id":     req.DigitalEmployeeID.String(),
		"attempt_no":              attemptNo,
		"idempotency_key":         req.IdempotencyKey,
		"lease_expires_at_set":    req.LeaseExpiresAt != nil,
	}
	if req.DigitalEmployeeRunID != nil {
		payload["digital_employee_run_id"] = req.DigitalEmployeeRunID.String()
	}
	if req.RuntimeTaskID != nil {
		payload["runtime_task_id"] = req.RuntimeTaskID.String()
	}
	if req.RuntimeNodeID != nil {
		payload["runtime_node_id"] = req.RuntimeNodeID.String()
	}
	if strings.TrimSpace(req.ProviderType) != "" {
		payload["provider_type"] = req.ProviderType
	}
	if req.DispatchGateResultID != nil {
		payload["dispatch_gate_result_id"] = req.DispatchGateResultID.String()
	}
	return payload
}

func (r *projectStoreMemoryRepository) ListProjectTasksByCoordinationJob(ctx context.Context, tenantID, projectID, coordinationJobID uuid.UUID) ([]project.ProjectTask, error) {
	tasks := make([]project.ProjectTask, 0, len(r.tasks))
	for _, task := range r.tasks {
		if task.TenantID == tenantID && task.ProjectID == projectID && task.CoordinationJobID != nil && *task.CoordinationJobID == coordinationJobID {
			tasks = append(tasks, task)
		}
	}
	return tasks, nil
}

func (r *projectStoreMemoryRepository) ListProjectTasks(ctx context.Context, tenantID, projectID uuid.UUID, status *string, limit, offset int32) ([]project.ProjectTask, error) {
	tasks := make([]project.ProjectTask, 0, len(r.tasks))
	for _, task := range r.tasks {
		if task.TenantID != tenantID || task.ProjectID != projectID {
			continue
		}
		if status != nil && task.Status != *status {
			continue
		}
		tasks = append(tasks, task)
	}
	if offset < 0 {
		offset = 0
	}
	if int(offset) >= len(tasks) {
		return []project.ProjectTask{}, nil
	}
	tasks = tasks[offset:]
	if limit > 0 && int(limit) < len(tasks) {
		tasks = tasks[:limit]
	}
	return tasks, nil
}

func (r *projectStoreMemoryRepository) ListDemandLaunchProjectTasks(ctx context.Context, tenantID, projectID, demandID uuid.UUID, limit int32) ([]project.ProjectTask, error) {
	tasks := make([]project.ProjectTask, 0, len(r.tasks))
	for _, task := range r.tasks {
		if task.TenantID != tenantID || task.ProjectID != projectID || task.DemandID == nil || *task.DemandID != demandID {
			continue
		}
		tasks = append(tasks, task)
		if limit > 0 && int32(len(tasks)) >= limit {
			break
		}
	}
	return tasks, nil
}

func (r *projectStoreMemoryRepository) ListDependentsOfTask(ctx context.Context, tenantID, projectID, blockerTaskID uuid.UUID) ([]uuid.UUID, error) {
	dependentIDs := make([]uuid.UUID, 0)
	seen := map[uuid.UUID]struct{}{}
	for _, dependency := range r.taskDependencies {
		if dependency.TenantID != tenantID || dependency.ProjectID != projectID || dependency.BlockerTaskID != blockerTaskID {
			continue
		}
		if _, exists := seen[dependency.DependentTaskID]; exists {
			continue
		}
		seen[dependency.DependentTaskID] = struct{}{}
		dependentIDs = append(dependentIDs, dependency.DependentTaskID)
	}
	return dependentIDs, nil
}

func (r *projectStoreMemoryRepository) ListUnresolvedBlockersForTasks(ctx context.Context, tenantID, projectID uuid.UUID, dependentTaskIDs []uuid.UUID) ([]project.ProjectTaskDependencyReadiness, error) {
	requested := map[uuid.UUID]struct{}{}
	for _, taskID := range dependentTaskIDs {
		requested[taskID] = struct{}{}
	}
	readiness := make([]project.ProjectTaskDependencyReadiness, 0)
	for _, dependency := range r.taskDependencies {
		if dependency.TenantID != tenantID || dependency.ProjectID != projectID {
			continue
		}
		if _, ok := requested[dependency.DependentTaskID]; !ok {
			continue
		}
		blocker, err := r.GetProjectTask(ctx, tenantID, dependency.BlockerTaskID)
		if err != nil {
			return nil, err
		}
		accepted, latestResult := r.blockerAcceptanceSatisfied(blocker)
		if accepted {
			continue
		}
		item := project.ProjectTaskDependencyReadiness{
			DependentTaskID:     dependency.DependentTaskID,
			BlockerTaskID:       dependency.BlockerTaskID,
			BlockerStatus:       blocker.Status,
			AcceptanceSatisfied: accepted,
		}
		if blocker.LatestTaskResultID != nil {
			item.LatestTaskResultID = blocker.LatestTaskResultID
		}
		if latestResult != nil {
			item.LatestResultStatus = latestResult.ResultStatus
			item.LatestResultDecision = latestResult.Decision
			item.LatestResultValidationStatus = latestResult.ValidationStatus
		}
		readiness = append(readiness, item)
	}
	return readiness, nil
}

func (r *projectStoreMemoryRepository) blockerAcceptanceSatisfied(blocker project.ProjectTask) (bool, *project.ProjectTaskResult) {
	if blocker.Status != project.ProjectTaskStatusCompleted || blocker.LatestTaskResultID == nil || *blocker.LatestTaskResultID == uuid.Nil {
		return false, nil
	}
	for _, result := range r.projectTaskResults {
		if result.ID != *blocker.LatestTaskResultID || result.ProjectTaskID != blocker.ID {
			continue
		}
		resultCopy := result
		return project.ProjectTaskResultAcceptedForDependencyUnlock(result), &resultCopy
	}
	return false, nil
}

func (r *projectStoreMemoryRepository) ListProjectTaskResults(ctx context.Context, req project.ListProjectTaskResultsRequest) ([]project.ProjectTaskResult, error) {
	results := make([]project.ProjectTaskResult, 0, len(r.projectTaskResults))
	for _, result := range r.projectTaskResults {
		if result.TenantID == req.TenantID && result.ProjectID == req.ProjectID && result.ProjectTaskID == req.ProjectTaskID {
			results = append(results, result)
		}
	}
	return results, nil
}

func (r *projectStoreMemoryRepository) LinkProjectTaskResultRevisionTask(ctx context.Context, tenantID, projectID, resultID, revisionTaskID uuid.UUID) (project.ProjectTaskResult, error) {
	for index, result := range r.projectTaskResults {
		if result.TenantID == tenantID && result.ProjectID == projectID && result.ID == resultID {
			result.RevisionTaskID = &revisionTaskID
			r.projectTaskResults[index] = result
			return result, nil
		}
	}
	return project.ProjectTaskResult{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) CreateProjectDemandSummary(ctx context.Context, req project.CreateProjectDemandSummaryRequest) (project.ProjectDemandSummary, error) {
	if r.createDemandSummaryErr != nil {
		return project.ProjectDemandSummary{}, r.createDemandSummaryErr
	}
	for _, summary := range r.demandSummaries {
		if summary.TenantID == req.TenantID && summary.IdempotencyKey == req.IdempotencyKey {
			return summary, nil
		}
	}
	summary := project.ProjectDemandSummary{
		ID:                 uuid.New(),
		TenantID:           req.TenantID,
		ProjectID:          req.ProjectID,
		DemandID:           req.DemandID,
		Status:             req.Status,
		Conclusion:         req.Conclusion,
		SummaryPayload:     req.SummaryPayload,
		ReportRefID:        req.ReportRefID,
		AcceptanceRequired: req.AcceptanceRequired,
		IdempotencyKey:     req.IdempotencyKey,
		CreatedEventID:     req.CreatedEventID,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	r.demandSummaries = append(r.demandSummaries, summary)
	return summary, nil
}

func (r *projectStoreMemoryRepository) GetLatestProjectDemandSummary(ctx context.Context, tenantID, projectID, demandID uuid.UUID) (project.ProjectDemandSummary, error) {
	for i := len(r.demandSummaries) - 1; i >= 0; i-- {
		summary := r.demandSummaries[i]
		if summary.TenantID == tenantID && summary.ProjectID == projectID && summary.DemandID == demandID {
			return summary, nil
		}
	}
	return project.ProjectDemandSummary{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) ProjectTaskEventExists(ctx context.Context, tenantID, projectID uuid.UUID, eventType project.ProjectEventType, actorID string) (bool, error) {
	for _, event := range r.events {
		if event.TenantID == tenantID && event.ProjectID == projectID && event.EventType == eventType && event.ActorID == actorID {
			return true, nil
		}
	}
	return false, nil
}

func (r *projectStoreMemoryRepository) UpdateProjectTaskStatus(ctx context.Context, tenantID, projectTaskID uuid.UUID, status string, eventID *uuid.UUID, currentStatuses []string) (project.ProjectTask, error) {
	r.statusUpdates = append(r.statusUpdates, projectTaskStatusUpdateRecord{TenantID: tenantID, TaskID: projectTaskID, Status: status, CurrentStatuses: append([]string(nil), currentStatuses...)})
	allowed := map[string]struct{}{}
	for _, currentStatus := range currentStatuses {
		allowed[currentStatus] = struct{}{}
	}
	for i, task := range r.tasks {
		if task.TenantID != tenantID || task.ID != projectTaskID {
			continue
		}
		if len(allowed) > 0 {
			if _, ok := allowed[task.Status]; !ok {
				return project.ProjectTask{}, project.ErrProjectConflict
			}
		}
		task.Status = status
		task.UpdatedAt = time.Now().UTC()
		r.tasks[i] = task
		return task, nil
	}
	return project.ProjectTask{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) FinishCoordinationJob(ctx context.Context, req project.FinishCoordinationJobRequest) (project.CoordinationJob, error) {
	return project.CoordinationJob{ID: req.ID, TenantID: req.TenantID, Status: req.Status, OutputEventIDs: req.OutputEventIDs, CreatedAt: time.Now().UTC()}, nil
}

func (r *projectStoreMemoryRepository) CreateDecisionRequest(ctx context.Context, req project.CreateDecisionRequestRequest) (project.DecisionRequest, error) {
	if r.createDecisionRequestErr != nil {
		return project.DecisionRequest{}, r.createDecisionRequestErr
	}
	decision := project.DecisionRequest{
		ID:                uuid.New(),
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		ApprovalRequestID: req.ApprovalRequestID,
		CoordinationJobID: req.CoordinationJobID,
		PlanRevisionID:    req.PlanRevisionID,
		ProjectTaskID:     req.ProjectTaskID,
		TargetUserID:      req.TargetUserID,
		DecisionType:      req.DecisionType,
		TitleSnapshot:     req.TitleSnapshot,
		SummarySnapshot:   strPtr(req.SummarySnapshot),
		RiskLevelSnapshot: strPtr(req.RiskLevelSnapshot),
		StatusSnapshot:    req.StatusSnapshot,
		CreatedEventID:    req.CreatedEventID,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	r.decisionRequests = append(r.decisionRequests, decision)
	return decision, nil
}

func (r *projectStoreMemoryRepository) GetDecisionRequest(ctx context.Context, tenantID, projectID, decisionRequestID uuid.UUID) (project.DecisionRequest, error) {
	for _, decision := range r.decisionRequests {
		if decision.ID == decisionRequestID && decision.TenantID == tenantID && decision.ProjectID == projectID {
			return decision, nil
		}
	}
	return project.DecisionRequest{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) GetDecisionRequestByPlanRevision(ctx context.Context, tenantID, projectID, planRevisionID uuid.UUID) (project.DecisionRequest, error) {
	for _, decision := range r.decisionRequests {
		if decision.TenantID == tenantID && decision.ProjectID == projectID &&
			decision.DecisionType == "plan_review" && decision.PlanRevisionID != nil && *decision.PlanRevisionID == planRevisionID {
			return decision, nil
		}
	}
	return project.DecisionRequest{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) ListDecisionRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]project.DecisionRequest, error) {
	results := []project.DecisionRequest{}
	for _, decision := range r.decisionRequests {
		if decision.TenantID == tenantID && decision.ProjectID == projectID {
			results = append(results, decision)
		}
	}
	return results, nil
}

type projectStoreApprovalCreator struct {
	approvalID uuid.UUID
	last       approval.CreateRequestInput
	record     *approval.ApprovalRequest
	err        error
}

func (c *projectStoreApprovalCreator) CreateRequest(ctx context.Context, input approval.CreateRequestInput) (*approval.ApprovalRequest, error) {
	c.last = input
	if c.err != nil {
		return nil, c.err
	}
	id := c.approvalID
	if id == uuid.Nil {
		id = uuid.New()
	}
	request := &approval.ApprovalRequest{
		ID:           id,
		TenantID:     input.TenantID,
		ResourceType: input.ResourceType,
		ResourceID:   input.ResourceID,
		TargetUserID: input.TargetUserID,
		DecisionType: input.DecisionType,
		Title:        input.Title,
		Status:       approval.ApprovalStatusPending,
	}
	c.record = request
	return request, nil
}

func (c *projectStoreApprovalCreator) GetRequestByResource(ctx context.Context, tenantID uuid.UUID, resourceType string, resourceID uuid.UUID) (*approval.ApprovalRequest, error) {
	if c.record != nil &&
		c.record.TenantID == tenantID &&
		c.record.ResourceType == resourceType &&
		c.record.ResourceID == resourceID &&
		c.record.Status == approval.ApprovalStatusPending {
		return c.record, nil
	}
	return nil, approval.ErrApprovalNotFound
}

type projectStoreDecisionInboxProjector struct {
	upserts     []project.DecisionRequest
	resolutions []project.DecisionRequest
	upsertErr   error
}

func (p *projectStoreDecisionInboxProjector) UpsertProjectDecisionRequest(ctx context.Context, decision project.DecisionRequest) error {
	p.upserts = append(p.upserts, decision)
	return p.upsertErr
}

func (p *projectStoreDecisionInboxProjector) ResolveProjectDecisionRequest(ctx context.Context, decision project.DecisionRequest) error {
	p.resolutions = append(p.resolutions, decision)
	return nil
}

type projectTaskRunStarterFake struct {
	requests []StartProjectTaskRunRequest
	result   StartProjectTaskRunResult
	err      error
	onStart  func(StartProjectTaskRunRequest)
}

func (f *projectTaskRunStarterFake) StartProjectTaskRun(ctx context.Context, req StartProjectTaskRunRequest) (StartProjectTaskRunResult, error) {
	f.requests = append(f.requests, req)
	if f.onStart != nil {
		f.onStart(req)
	}
	if f.err != nil {
		return StartProjectTaskRunResult{}, f.err
	}
	return f.result, nil
}

type projectStoreGateRuntimeReader struct {
	employee project.PreDispatchEmployeeSnapshot
	runtime  project.PreDispatchRuntimeSnapshot
	err      error
}

func (r projectStoreGateRuntimeReader) GetEmployeeRuntimeSnapshot(ctx context.Context, tenantID, projectID, employeeID uuid.UUID) (project.PreDispatchEmployeeSnapshot, project.PreDispatchRuntimeSnapshot, error) {
	if r.err != nil {
		return project.PreDispatchEmployeeSnapshot{}, project.PreDispatchRuntimeSnapshot{}, r.err
	}
	return r.employee, r.runtime, nil
}

func projectStoreExecutorMember(tenantID, projectID, employeeID uuid.UUID) project.ProjectMember {
	return project.ProjectMember{
		TenantID:      tenantID,
		ProjectID:     projectID,
		PrincipalType: project.PrincipalTypeDigitalEmployee,
		PrincipalID:   employeeID,
		ProjectRole:   project.ProjectRoleExecutor,
		Status:        "active",
	}
}

func projectStoreTask(tenantID, projectID, demandID, coordinationJobID, routeDecisionID, taskID uuid.UUID, status string) project.ProjectTask {
	return project.ProjectTask{
		ID:                taskID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		DemandID:          &demandID,
		Title:             "任务 " + taskID.String(),
		Status:            status,
		CoordinationJobID: &coordinationJobID,
		RouteDecisionID:   &routeDecisionID,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
}

func projectStoreDependency(tenantID, projectID, coordinationJobID, dependentTaskID, blockerTaskID uuid.UUID) project.ProjectTaskDependency {
	return project.ProjectTaskDependency{
		ID:                uuid.New(),
		TenantID:          tenantID,
		ProjectID:         projectID,
		CoordinationJobID: &coordinationJobID,
		DependentTaskID:   dependentTaskID,
		BlockerTaskID:     blockerTaskID,
	}
}

func projectStoreTaskResult(tenantID, projectID, taskID uuid.UUID, decision project.TaskResultDecision, validationStatus string) project.ProjectTaskResult {
	return project.ProjectTaskResult{
		ID:               uuid.New(),
		TenantID:         tenantID,
		ProjectID:        projectID,
		ProjectTaskID:    taskID,
		ResultStatus:     project.TaskResultStatusCompleted,
		ValidationStatus: validationStatus,
		Decision:         decision,
		Contract: project.TaskResultContract{
			Status:  project.TaskResultStatusCompleted,
			Summary: "dependency result",
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
}

func (r *projectStoreMemoryRepository) taskStatus(taskID uuid.UUID) string {
	for _, task := range r.tasks {
		if task.ID == taskID {
			return task.Status
		}
	}
	return ""
}

func (r *projectStoreMemoryRepository) mustTask(taskID uuid.UUID) project.ProjectTask {
	for _, task := range r.tasks {
		if task.ID == taskID {
			return task
		}
	}
	panic("project task not found")
}

func (r *projectStoreMemoryRepository) setTaskStatus(taskID uuid.UUID, status string) {
	for index, task := range r.tasks {
		if task.ID == taskID {
			r.tasks[index].Status = status
			return
		}
	}
}

func (r *projectStoreMemoryRepository) setTaskLatestResult(taskID uuid.UUID, result project.ProjectTaskResult) {
	r.projectTaskResults = append(r.projectTaskResults, result)
	for index, task := range r.tasks {
		if task.ID == taskID {
			r.tasks[index].LatestTaskResultID = &result.ID
			r.tasks[index].UpdatedAt = result.UpdatedAt
			return
		}
	}
}

func projectStoreEventsByType(events []project.ProjectEvent, eventType project.ProjectEventType) []project.ProjectEvent {
	matches := make([]project.ProjectEvent, 0)
	for _, event := range events {
		if event.EventType == eventType {
			matches = append(matches, event)
		}
	}
	return matches
}

func requireProjectStoreDemandSummary(t *testing.T, summaries []project.ProjectDemandSummary, demandID uuid.UUID) project.ProjectDemandSummary {
	t.Helper()
	for _, summary := range summaries {
		if summary.DemandID == demandID {
			return summary
		}
	}
	t.Fatalf("summary for demand %s not found in %#v", demandID, summaries)
	return project.ProjectDemandSummary{}
}

func requirePayloadListContains(t *testing.T, payload map[string]any, key, field string, value any) {
	t.Helper()
	if payloadListContains(payload[key], field, value) {
		return
	}
	t.Fatalf("payload[%q] does not contain %q=%#v: %#v", key, field, value, payload[key])
}

func requirePayloadListNotContains(t *testing.T, payload map[string]any, key, field string, value any) {
	t.Helper()
	if !payloadListContains(payload[key], field, value) {
		return
	}
	t.Fatalf("payload[%q] unexpectedly contains %q=%#v: %#v", key, field, value, payload[key])
}

func payloadListContains(value any, field string, expected any) bool {
	for _, item := range payloadListItems(value) {
		if reflect.DeepEqual(item[field], expected) {
			return true
		}
	}
	return false
}

func payloadListItems(value any) []map[string]any {
	switch items := value.(type) {
	case []map[string]any:
		return items
	case []any:
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			switch typed := item.(type) {
			case map[string]any:
				out = append(out, typed)
			case map[string]string:
				converted := make(map[string]any, len(typed))
				for key, value := range typed {
					converted[key] = value
				}
				out = append(out, converted)
			}
		}
		return out
	default:
		return nil
	}
}

func recoveryReplacementTasks(repo *projectStoreMemoryRepository, sourceTaskID uuid.UUID) []project.ProjectTask {
	tasks := make([]project.ProjectTask, 0)
	for _, task := range repo.tasks {
		if task.PlannerMetadata["source_task_id"] == sourceTaskID.String() {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

func requireRecoveryReplacementTask(t *testing.T, repo *projectStoreMemoryRepository, sourceTaskID uuid.UUID) project.ProjectTask {
	t.Helper()
	tasks := recoveryReplacementTasks(repo, sourceTaskID)
	require.Len(t, tasks, 1)
	return tasks[0]
}

func requireDependency(t *testing.T, dependencies []project.ProjectTaskDependency, dependentTaskID, blockerTaskID uuid.UUID) {
	t.Helper()
	for _, dependency := range dependencies {
		if dependency.DependentTaskID == dependentTaskID && dependency.BlockerTaskID == blockerTaskID {
			return
		}
	}
	t.Fatalf("expected dependency dependent=%s blocker=%s in %#v", dependentTaskID, blockerTaskID, dependencies)
}

func requireNoDependency(t *testing.T, dependencies []project.ProjectTaskDependency, dependentTaskID, blockerTaskID uuid.UUID) {
	t.Helper()
	for _, dependency := range dependencies {
		if dependency.DependentTaskID == dependentTaskID && dependency.BlockerTaskID == blockerTaskID {
			t.Fatalf("unexpected dependency dependent=%s blocker=%s in %#v", dependentTaskID, blockerTaskID, dependencies)
		}
	}
}

func eventsByType(events []project.ProjectEvent, eventType project.ProjectEventType) []project.ProjectEvent {
	filtered := make([]project.ProjectEvent, 0)
	for _, event := range events {
		if event.EventType == eventType {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func strPtr(value string) *string {
	return &value
}

func strPtrOrNil(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func assertUUIDs(t *testing.T, got []uuid.UUID, want []uuid.UUID) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected uuid list: got %#v want %#v", got, want)
	}
}

func assertAnyStrings(t *testing.T, got []any, want []string) {
	t.Helper()
	if !reflect.DeepEqual(anyStrings(got), want) {
		t.Fatalf("unexpected string list: got %#v want %#v", got, want)
	}
}

func assertPayloadStrings(t *testing.T, value any, want []string) {
	t.Helper()
	got, ok := value.([]any)
	if !ok {
		t.Fatalf("expected []any payload, got %#v", value)
	}
	assertAnyStrings(t, got, want)
}

func anyStrings(values []any) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.(string))
	}
	return result
}
