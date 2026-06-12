package project

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCreateProjectRequiresHumanOwnerAndCreatesEvents(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	ownerID := uuid.New()
	employeeID := uuid.New()

	created, err := service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      ownerID,
		Name:             "支付网关稳定性整改",
		Goal:             "修复超时链路并形成验收报告",
		HumanOwnerUserID: ownerID,
		Members: []ProjectMemberInput{
			{PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID, ProjectRole: ProjectRoleOwner, DisplayNameSnapshot: "王佩"},
			{PrincipalType: PrincipalTypeDigitalEmployee, PrincipalID: employeeID, ProjectRole: ProjectRoleExecutor, DisplayNameSnapshot: "后端执行 A", Settings: map[string]any{"concurrency_slots": float64(2)}},
		},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if created.Project.Status != ProjectStatusRunning {
		t.Fatalf("expected running project, got %s", created.Project.Status)
	}
	if created.Project.CoordinationStatus != "registered" {
		t.Fatalf("expected registered coordination status, got %s", created.Project.CoordinationStatus)
	}
	if !strings.HasPrefix(created.Project.CoordinationWorkflowID, "project-coordinator:") {
		t.Fatalf("expected coordination workflow id, got %q", created.Project.CoordinationWorkflowID)
	}
	if repo.eventTypes[0] != ProjectEventCreated || repo.eventTypes[1] != ProjectEventConfigChanged {
		t.Fatalf("expected create/config events, got %#v", repo.eventTypes)
	}
	for _, member := range created.Members {
		if member.ProjectRole == ProjectRole("coordinator") {
			t.Fatal("coordinator must not be represented as a project member")
		}
	}
}

func TestCreateProjectRequiresMandatoryFields(t *testing.T) {
	service, err := NewService(newMemoryRepository())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         uuid.New(),
		ActorUserID:      uuid.New(),
		Name:             "缺少目标",
		HumanOwnerUserID: uuid.New(),
	})
	if !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("expected invalid project error, got %v", err)
	}
}

func TestCreateProjectRejectsCoordinatorMemberRole(t *testing.T) {
	service, err := NewService(newMemoryRepository())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         uuid.New(),
		ActorUserID:      uuid.New(),
		Name:             "项目",
		Goal:             "目标",
		HumanOwnerUserID: uuid.New(),
		Members: []ProjectMemberInput{{
			PrincipalType: PrincipalTypeDigitalEmployee,
			PrincipalID:   uuid.New(),
			ProjectRole:   ProjectRole("coordinator"),
		}},
	})
	if !errors.Is(err, ErrInvalidProjectMember) {
		t.Fatalf("expected invalid member error, got %v", err)
	}
}

func TestCreateProjectValidatesRolePrincipalTypes(t *testing.T) {
	service, err := NewService(newMemoryRepository())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	for _, tc := range []struct {
		name          string
		principalType PrincipalType
		role          ProjectRole
	}{
		{name: "owner must be human", principalType: PrincipalTypeDigitalEmployee, role: ProjectRoleOwner},
		{name: "leader must be human", principalType: PrincipalTypeDigitalEmployee, role: ProjectRoleLeader},
		{name: "acceptance must be human", principalType: PrincipalTypeDigitalEmployee, role: ProjectRoleAcceptance},
		{name: "executor must be digital employee", principalType: PrincipalTypeHumanUser, role: ProjectRoleExecutor},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.CreateProject(context.Background(), CreateProjectRequest{
				TenantID:         uuid.New(),
				ActorUserID:      uuid.New(),
				Name:             "项目",
				Goal:             "目标",
				HumanOwnerUserID: uuid.New(),
				Members: []ProjectMemberInput{{
					PrincipalType: tc.principalType,
					PrincipalID:   uuid.New(),
					ProjectRole:   tc.role,
				}},
			})
			if !errors.Is(err, ErrInvalidProjectMember) {
				t.Fatalf("expected invalid member error, got %v", err)
			}
		})
	}
}

func TestSubmitDemandRecordsDemandAndEventWithoutAutoCreatingTask(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		Name:             "客户侧 Runtime 接入验收",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	seedHumanOwnerMember(repo, repo.projects[projectID].TenantID, projectID, ownerID)

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID:          repo.projects[projectID].TenantID,
		ProjectID:         projectID,
		SubmittedByUserID: ownerID,
		Title:             "验证 Runtime 连接",
		Content:           "检查心跳和命令回写",
		SourceType:        DemandSourceManual,
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}
	if demand.Status != ProjectDemandStatusPlanningPending {
		t.Fatalf("expected planning pending demand, got %s", demand.Status)
	}
	if len(repo.tasks) != 0 {
		t.Fatalf("service must not create project tasks from demand directly")
	}
	if len(repo.eventTypes) != 1 || repo.eventTypes[0] != ProjectEventDemandSubmitted {
		t.Fatalf("expected demand event only, got %#v", repo.eventTypes)
	}
}

func TestSubmitDemandPersistsDefaultReviewerPreference(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	reviewerID := uuid.New()
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID,
			ProjectRole: ProjectRoleOwner, Status: "active",
		},
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: reviewerID,
			ProjectRole: ProjectRoleReviewer, Status: "active",
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "审查 PR", Content: "统计 PR 并分派审查",
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}

	if demand.ReviewerPreference == nil {
		t.Fatalf("expected reviewer preference on demand: %#v", demand)
	}
	if demand.ReviewerPreference.ReviewerUserID != reviewerID {
		t.Fatalf("expected reviewer %s, got %#v", reviewerID, demand.ReviewerPreference)
	}
	if demand.ReviewerPreference.SelectionReason != ReviewerSelectionProjectReviewerDefault {
		t.Fatalf("unexpected reviewer reason: %#v", demand.ReviewerPreference)
	}
	if demand.SourceRefs["reviewer_user_id"] != reviewerID.String() {
		t.Fatalf("expected reviewer persisted in source refs: %#v", demand.SourceRefs)
	}
}

func TestSubmitDemandPersistsExplicitReviewerSelectionReason(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	reviewerID := uuid.New()
	reviewerName := "审查负责人"
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID,
			ProjectRole: ProjectRoleOwner, Status: "active",
		},
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: reviewerID,
			ProjectRole: ProjectRoleReviewer, DisplayNameSnapshot: &reviewerName, Status: "active",
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "审查 PR", ReviewerUserID: &reviewerID,
		ReviewerSelectionReason: ReviewerSelectionProjectReviewerDefault,
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}

	if demand.ReviewerPreference == nil {
		t.Fatalf("expected reviewer preference on demand: %#v", demand)
	}
	if demand.ReviewerPreference.SelectionReason != ReviewerSelectionProjectReviewerDefault {
		t.Fatalf("expected explicit reason to be preserved, got %#v", demand.ReviewerPreference)
	}
	if demand.SourceRefs["reviewer_selection_reason"] != string(ReviewerSelectionProjectReviewerDefault) {
		t.Fatalf("expected reviewer reason persisted in source refs: %#v", demand.SourceRefs)
	}
	if demand.SourceRefs["reviewer_display_name"] != reviewerName {
		t.Fatalf("expected reviewer display name persisted in source refs: %#v", demand.SourceRefs)
	}
}

func TestSubmitDemandRejectsInvalidReviewerSelectionReason(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	reviewerID := uuid.New()
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID,
			ProjectRole: ProjectRoleOwner, Status: "active",
		},
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: reviewerID,
			ProjectRole: ProjectRoleReviewer, Status: "active",
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "审查 PR", ReviewerUserID: &reviewerID,
		ReviewerSelectionReason: ReviewerSelectionReason("invalid_reason"),
	})
	if !errors.Is(err, ErrInvalidProjectMember) {
		t.Fatalf("expected invalid project member, got %v", err)
	}
}

func TestSubmitDemandFallsBackToHumanOwnerWhenNoReviewer(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
		PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID,
		ProjectRole: ProjectRoleOwner, Status: "active",
	}}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "补充证据",
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}
	if demand.ReviewerPreference == nil || demand.ReviewerPreference.ReviewerUserID != ownerID {
		t.Fatalf("expected owner fallback preference: %#v", demand.ReviewerPreference)
	}
	if demand.ReviewerPreference.SelectionReason != ReviewerSelectionProjectHumanOwnerFallback {
		t.Fatalf("expected owner fallback reason, got %#v", demand.ReviewerPreference)
	}
}

func TestSubmitDemandRequiresActiveHumanOwnerMemberForFallback(t *testing.T) {
	for _, tc := range []struct {
		name    string
		members []ProjectMember
	}{
		{name: "missing owner member"},
		{
			name: "inactive owner member",
			members: []ProjectMember{{
				PrincipalType: PrincipalTypeHumanUser,
				ProjectRole:   ProjectRoleOwner,
				Status:        "inactive",
			}},
		},
		{
			name: "digital owner member",
			members: []ProjectMember{{
				PrincipalType: PrincipalTypeDigitalEmployee,
				ProjectRole:   ProjectRoleOwner,
				Status:        "active",
			}},
		},
		{
			name: "observer owner member",
			members: []ProjectMember{{
				PrincipalType: PrincipalTypeHumanUser,
				ProjectRole:   ProjectRoleObserver,
				Status:        "active",
			}},
		},
		{
			name: "executor owner member",
			members: []ProjectMember{{
				PrincipalType: PrincipalTypeHumanUser,
				ProjectRole:   ProjectRoleExecutor,
				Status:        "active",
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tenantID := uuid.New()
			projectID := uuid.New()
			ownerID := uuid.New()
			repo := newMemoryRepository()
			repo.projects[projectID] = Project{
				ID:               projectID,
				TenantID:         tenantID,
				Status:           ProjectStatusRunning,
				HumanOwnerUserID: ownerID,
			}
			for _, member := range tc.members {
				member.ID = uuid.New()
				member.TenantID = tenantID
				member.ProjectID = projectID
				member.PrincipalID = ownerID
				repo.members[projectID] = append(repo.members[projectID], member)
			}
			service, err := NewService(repo)
			if err != nil {
				t.Fatalf("new service: %v", err)
			}

			_, err = service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
				TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
				Title: "补充证据",
			})
			if !errors.Is(err, ErrInvalidProjectMember) {
				t.Fatalf("expected invalid project member, got %v", err)
			}
		})
	}
}

func TestReviewerPreferenceFromSourceRefsRestoresDisplayName(t *testing.T) {
	reviewerID := uuid.New()
	preference := reviewerPreferenceFromSourceRefs(map[string]any{
		"reviewer_user_id":            reviewerID.String(),
		"reviewer_selection_reason":   string(ReviewerSelectionProjectReviewerDefault),
		"reviewer_project_role":       string(ProjectRoleReviewer),
		"reviewer_resolved_from_rule": true,
		"reviewer_display_name":       "审查负责人",
	})

	if preference == nil {
		t.Fatal("expected reviewer preference")
	}
	if preference.DisplayName == nil || *preference.DisplayName != "审查负责人" {
		t.Fatalf("expected display name restored, got %#v", preference)
	}
	if preference.ReviewerUserID != reviewerID || preference.SelectionReason != ReviewerSelectionProjectReviewerDefault || preference.ProjectRole != ProjectRoleReviewer || !preference.ResolvedFromRule {
		t.Fatalf("unexpected reviewer preference: %#v", preference)
	}
}

func TestSubmitDemandRejectsDigitalEmployeeReviewer(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	digitalEmployeeID := uuid.New()
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID,
			ProjectRole: ProjectRoleOwner, Status: "active",
		},
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeDigitalEmployee, PrincipalID: digitalEmployeeID,
			ProjectRole: ProjectRoleExecutor, Status: "active",
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "需要审核", ReviewerUserID: &digitalEmployeeID,
	})
	if !errors.Is(err, ErrInvalidProjectMember) {
		t.Fatalf("expected invalid project member, got %v", err)
	}
}

func TestSubmitDemandRequiresExplicitReviewerWhenMultipleReviewers(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
		PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID,
		ProjectRole: ProjectRoleOwner, Status: "active",
	}}
	for range 2 {
		repo.members[projectID] = append(repo.members[projectID], ProjectMember{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: uuid.New(),
			ProjectRole: ProjectRoleReviewer, Status: "active",
		})
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "多审核人项目",
	})
	if !errors.Is(err, ErrInvalidProjectMember) {
		t.Fatalf("expected reviewer selection error, got %v", err)
	}
}

func TestProjectGovernanceCreatesEvidenceAndProjectEvent(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning, HumanOwnerUserID: actorID}

	evidence, err := service.CreateEvidenceRef(context.Background(), CreateEvidenceRefServiceRequest{
		TenantID: tenantID, ProjectID: projectID, ActorType: "human_user", ActorID: actorID,
		EvidenceType: "test_result", Title: "回归测试结果", SourceType: "artifact",
		SourceRef: "s3://bucket/reports/regression.json", SubmittedByType: "human_user", SubmittedByID: &actorID,
	})
	if err != nil {
		t.Fatalf("create evidence: %v", err)
	}
	if evidence.VerificationStatus != EvidenceVerificationStatusSubmitted {
		t.Fatalf("expected submitted evidence, got %s", evidence.VerificationStatus)
	}
	if repo.eventTypes[len(repo.eventTypes)-1] != ProjectEventEvidenceLinked {
		t.Fatalf("expected evidence event, got %#v", repo.eventTypes)
	}
}

func TestProjectAcceptanceRequiresHumanOwnerAndFinalReport(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	otherUserID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}

	_, err = service.CreateAcceptanceRecord(context.Background(), CreateAcceptanceServiceRequest{
		TenantID: tenantID, ProjectID: projectID, AcceptedByUserID: otherUserID,
		Status: "accepted", Conclusion: "通过", EvidenceRefIDs: []uuid.UUID{uuid.New()}, ReportRefIDs: []uuid.UUID{uuid.New()},
	})
	if !errors.Is(err, ErrInvalidProjectAcceptance) {
		t.Fatalf("expected invalid acceptance actor, got %v", err)
	}
}

func TestProjectGovernanceEvidenceFailureDoesNotLeaveSuccessEvent(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	repo.createEvidenceRefErr = fmt.Errorf("evidence store unavailable")
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning, HumanOwnerUserID: actorID}

	_, err = service.CreateEvidenceRef(context.Background(), CreateEvidenceRefServiceRequest{
		TenantID: tenantID, ProjectID: projectID, ActorType: "human_user", ActorID: actorID,
		EvidenceType: "test_result", Title: "回归测试结果", SourceType: "artifact",
		SourceRef: "s3://bucket/reports/regression.json", SubmittedByType: "human_user", SubmittedByID: &actorID,
	})
	if err == nil {
		t.Fatal("expected evidence write error")
	}
	if countProjectEvents(repo.eventTypes, ProjectEventEvidenceLinked) != 0 {
		t.Fatalf("expected no success event after evidence failure, got %#v", repo.eventTypes)
	}
}

func TestProjectPatchEvidencePreservesOrClearsMetadata(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning, HumanOwnerUserID: ownerID}
	evidence, err := repo.CreateEvidenceRef(context.Background(), CreateEvidenceRefRequest{
		TenantID: tenantID, ProjectID: projectID, EvidenceType: "test_result", Title: "回归测试结果",
		SourceType: "artifact", SourceRef: "s3://bucket/reports/regression.json",
		SubmittedByType: "human_user", SubmittedByID: &ownerID, VerificationStatus: EvidenceVerificationStatusSubmitted,
		Metadata: map[string]any{"suite": "regression", "passed": true},
	})
	if err != nil {
		t.Fatalf("seed evidence: %v", err)
	}

	updated, err := service.PatchEvidence(context.Background(), PatchEvidenceRequest{
		TenantID: tenantID, ProjectID: projectID, EvidenceID: evidence.ID, ActorUserID: ownerID,
		VerificationStatus: EvidenceVerificationStatusVerified,
	})
	if err != nil {
		t.Fatalf("patch evidence with omitted metadata: %v", err)
	}
	if updated.VerificationStatus != EvidenceVerificationStatusVerified || updated.Metadata["suite"] != "regression" || updated.Metadata["passed"] != true {
		t.Fatalf("expected omitted metadata to keep existing values, got %#v", updated)
	}

	cleared, err := service.PatchEvidence(context.Background(), PatchEvidenceRequest{
		TenantID: tenantID, ProjectID: projectID, EvidenceID: evidence.ID, ActorUserID: ownerID,
		VerificationStatus: EvidenceVerificationStatusRejected,
		Metadata:           map[string]any{},
	})
	if err != nil {
		t.Fatalf("patch evidence with empty metadata: %v", err)
	}
	if cleared.VerificationStatus != EvidenceVerificationStatusRejected || len(cleared.Metadata) != 0 {
		t.Fatalf("expected explicit empty metadata to clear values, got %#v", cleared)
	}
}

func TestProjectPatchEvidenceEventFailureRollsBackStatusAndMetadata(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning, HumanOwnerUserID: ownerID}
	evidence, err := repo.CreateEvidenceRef(context.Background(), CreateEvidenceRefRequest{
		TenantID: tenantID, ProjectID: projectID, EvidenceType: "test_result", Title: "回归测试结果",
		SourceType: "artifact", SourceRef: "s3://bucket/reports/regression.json",
		SubmittedByType: "human_user", SubmittedByID: &ownerID, VerificationStatus: EvidenceVerificationStatusSubmitted,
		Metadata: map[string]any{"suite": "regression"},
	})
	if err != nil {
		t.Fatalf("seed evidence: %v", err)
	}
	repo.appendProjectEventErr = errors.New("event store unavailable")

	_, err = service.PatchEvidence(context.Background(), PatchEvidenceRequest{
		TenantID: tenantID, ProjectID: projectID, EvidenceID: evidence.ID, ActorUserID: ownerID,
		VerificationStatus: EvidenceVerificationStatusVerified,
		Metadata:           map[string]any{"suite": "smoke"},
	})
	if err == nil {
		t.Fatal("expected event write failure")
	}
	if repo.evidenceRefs[0].VerificationStatus != EvidenceVerificationStatusSubmitted || repo.evidenceRefs[0].Metadata["suite"] != "regression" {
		t.Fatalf("expected evidence update rolled back, got %#v", repo.evidenceRefs[0])
	}
	if countProjectEvents(repo.eventTypes, ProjectEventEvidenceVerified) != 0 {
		t.Fatalf("expected no verification event after rollback, got %#v", repo.eventTypes)
	}
}

func TestProjectGovernanceMissingRecordsReturnNotFound(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning, HumanOwnerUserID: ownerID}

	_, err = service.PatchEvidence(context.Background(), PatchEvidenceRequest{
		TenantID: tenantID, ProjectID: projectID, EvidenceID: uuid.New(), ActorUserID: ownerID,
		VerificationStatus: EvidenceVerificationStatusVerified,
	})
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected missing evidence not found, got %v", err)
	}
	_, err = service.GetAcceptance(context.Background(), tenantID, projectID)
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected missing acceptance not found, got %v", err)
	}
	_, err = service.GetConfigRevision(context.Background(), tenantID, projectID, uuid.New())
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected missing config revision not found, got %v", err)
	}
}

func TestProjectBudgetSummaryRequiresExistingProject(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.GetBudgetSummary(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected missing project budget summary to return not found, got %v", err)
	}
}

func TestProjectAcceptanceRejectsMissingEvidenceOrReportRefs(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}

	_, err = service.CreateAcceptanceRecord(context.Background(), CreateAcceptanceServiceRequest{
		TenantID: tenantID, ProjectID: projectID, AcceptedByUserID: ownerID,
		Status: "accepted", Conclusion: "通过", EvidenceRefIDs: []uuid.UUID{uuid.New()}, ReportRefIDs: []uuid.UUID{uuid.New()},
	})
	if !errors.Is(err, ErrInvalidProjectAcceptance) {
		t.Fatalf("expected invalid acceptance refs, got %v", err)
	}
	if countProjectEvents(repo.eventTypes, ProjectEventAcceptanceSubmitted) != 0 {
		t.Fatalf("expected no acceptance event for invalid refs, got %#v", repo.eventTypes)
	}
}

func TestProjectAcceptanceSucceedsWithExistingEvidenceAndReportRefs(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}
	evidence, err := repo.CreateEvidenceRef(context.Background(), CreateEvidenceRefRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		EvidenceType:       "test_result",
		Title:              "回归测试结果",
		SourceType:         "artifact",
		SourceRef:          "s3://bucket/reports/regression.json",
		SubmittedByType:    "human_user",
		SubmittedByID:      &ownerID,
		VerificationStatus: EvidenceVerificationStatusSubmitted,
	})
	if err != nil {
		t.Fatalf("seed evidence: %v", err)
	}
	report, err := repo.CreateReportRef(context.Background(), CreateReportRefRequest{
		TenantID:        tenantID,
		ProjectID:       projectID,
		ReportType:      "final_report",
		Title:           "验收报告",
		ObjectRef:       "s3://bucket/reports/final.md",
		Format:          "markdown",
		GeneratedByType: "human_user",
		GeneratedByID:   &ownerID,
	})
	if err != nil {
		t.Fatalf("seed report: %v", err)
	}

	acceptance, err := service.CreateAcceptanceRecord(context.Background(), CreateAcceptanceServiceRequest{
		TenantID: tenantID, ProjectID: projectID, AcceptedByUserID: ownerID,
		Status: "accepted", Conclusion: "通过", EvidenceRefIDs: []uuid.UUID{evidence.ID}, ReportRefIDs: []uuid.UUID{report.ID},
	})
	if err != nil {
		t.Fatalf("create acceptance: %v", err)
	}
	if acceptance.CreatedEventID == nil || countProjectEvents(repo.eventTypes, ProjectEventAcceptanceSubmitted) != 1 {
		t.Fatalf("expected acceptance event and record link, events=%#v acceptance=%#v", repo.eventTypes, acceptance)
	}
}

func TestProjectArchivePreviewCountsAllPages(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}

	for i := 0; i < 105; i++ {
		_, err := repo.CreateEvidenceRef(context.Background(), CreateEvidenceRefRequest{
			TenantID:           tenantID,
			ProjectID:          projectID,
			EvidenceType:       "test_result",
			Title:              fmt.Sprintf("证据 %d", i),
			SourceType:         "artifact",
			SourceRef:          fmt.Sprintf("s3://bucket/evidence/%d.json", i),
			SubmittedByType:    "human_user",
			SubmittedByID:      &ownerID,
			VerificationStatus: EvidenceVerificationStatusSubmitted,
		})
		if err != nil {
			t.Fatalf("seed evidence: %v", err)
		}
	}
	for i := 0; i < 103; i++ {
		_, err := repo.CreateArtifactRef(context.Background(), CreateArtifactRefRequest{
			TenantID:        tenantID,
			ProjectID:       projectID,
			ArtifactType:    "execution_log",
			Title:           fmt.Sprintf("工件 %d", i),
			ObjectRef:       fmt.Sprintf("s3://bucket/artifacts/%d.log", i),
			RetentionStatus: "locked",
		})
		if err != nil {
			t.Fatalf("seed artifact: %v", err)
		}
	}
	for i := 0; i < 102; i++ {
		_, err := repo.CreateReportRef(context.Background(), CreateReportRefRequest{
			TenantID:        tenantID,
			ProjectID:       projectID,
			ReportType:      "final_report",
			Title:           fmt.Sprintf("报告 %d", i),
			ObjectRef:       fmt.Sprintf("s3://bucket/reports/%d.md", i),
			Format:          "markdown",
			GeneratedByType: "human_user",
			GeneratedByID:   &ownerID,
		})
		if err != nil {
			t.Fatalf("seed report: %v", err)
		}
	}

	preview, err := service.BuildArchivePreview(context.Background(), tenantID, projectID)
	if err != nil {
		t.Fatalf("build archive preview: %v", err)
	}
	if preview.EvidenceCount != 105 || preview.ArtifactCount != 103 || preview.ReportCount != 102 {
		t.Fatalf("expected full counts, got evidence=%d artifact=%d report=%d", preview.EvidenceCount, preview.ArtifactCount, preview.ReportCount)
	}
}

func TestArchiveSnapshotLocksReferencedArtifactsBeforeArchiving(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	lockEventID := uuid.New()
	locker := &fakeArchiveArtifactLocker{eventID: &lockEventID}
	service, err := NewServiceWithArchiveArtifactLocker(repo, locker)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	artifactID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}
	repo.artifactRefs = append(repo.artifactRefs, ProjectArtifactRef{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, ArtifactID: &artifactID,
		ObjectRef: "s3://bucket/report.md", Title: "最终报告",
	})
	snapshot, err := service.CreateArchiveSnapshot(context.Background(), CreateArchiveSnapshotServiceRequest{
		TenantID: tenantID, ProjectID: projectID, CreatedByUserID: ownerID,
		SnapshotType: "final_archive", Summary: "验收通过后归档", ObjectRef: "s3://bucket/archive/project.json",
	})
	if err != nil {
		t.Fatalf("archive snapshot: %v", err)
	}
	if snapshot.Status != "archived" {
		t.Fatalf("expected archived snapshot, got %s", snapshot.Status)
	}
	if len(locker.artifactIDs) != 1 || locker.artifactIDs[0] != artifactID {
		t.Fatalf("expected artifact lock, got %#v", locker.artifactIDs)
	}
	if snapshot.RetentionLockEventID == nil || *snapshot.RetentionLockEventID != lockEventID {
		t.Fatalf("expected retention lock event id %s, got %#v", lockEventID, snapshot.RetentionLockEventID)
	}
	if repo.projects[projectID].Status != ProjectStatusArchived {
		t.Fatalf("expected project archived after retention lock, got %s", repo.projects[projectID].Status)
	}
}

func TestArchiveSnapshotStaysPendingWhenArtifactLockFails(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	locker := &fakeArchiveArtifactLocker{err: errors.New("retention store unavailable")}
	service, err := NewServiceWithArchiveArtifactLocker(repo, locker)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	artifactID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}
	repo.artifactRefs = append(repo.artifactRefs, ProjectArtifactRef{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, ArtifactID: &artifactID,
		ObjectRef: "s3://bucket/report.md", Title: "最终报告",
	})

	snapshot, err := service.CreateArchiveSnapshot(context.Background(), CreateArchiveSnapshotServiceRequest{
		TenantID: tenantID, ProjectID: projectID, CreatedByUserID: ownerID,
		SnapshotType: "final_archive", Summary: "验收通过后归档", ObjectRef: "s3://bucket/archive/project.json",
	})
	if err != nil {
		t.Fatalf("archive snapshot should return pending state without error: %v", err)
	}
	if snapshot.Status != "archive_pending_retention" {
		t.Fatalf("expected retention pending snapshot, got %s", snapshot.Status)
	}
	if repo.projects[projectID].Status == ProjectStatusArchived {
		t.Fatalf("project must not be archived when retention lock fails")
	}
	if len(repo.archiveSnapshots) != 1 || repo.archiveSnapshots[0].Status != "archive_pending_retention" {
		t.Fatalf("expected persisted pending snapshot, got %#v", repo.archiveSnapshots)
	}
}

func TestArchiveSnapshotReturnsArchiveProjectErrorAfterSuccessfulLock(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	repo.archiveProjectErr = errors.New("archive update failed")
	locker := &fakeArchiveArtifactLocker{}
	service, err := NewServiceWithArchiveArtifactLocker(repo, locker)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	artifactID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}
	repo.artifactRefs = append(repo.artifactRefs, ProjectArtifactRef{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, ArtifactID: &artifactID,
		ObjectRef: "s3://bucket/report.md", Title: "最终报告",
	})

	_, err = service.CreateArchiveSnapshot(context.Background(), CreateArchiveSnapshotServiceRequest{
		TenantID: tenantID, ProjectID: projectID, CreatedByUserID: ownerID,
		SnapshotType: "final_archive", Summary: "验收通过后归档", ObjectRef: "s3://bucket/archive/project.json",
	})
	if !errors.Is(err, repo.archiveProjectErr) {
		t.Fatalf("expected archive project error, got %v", err)
	}
	if len(repo.archiveSnapshots) != 0 {
		t.Fatalf("expected archived snapshot to roll back after archive project failure, got %#v", repo.archiveSnapshots)
	}
	if countProjectEvents(repo.eventTypes, ProjectEventArchiveSnapshotCreated) != 0 {
		t.Fatalf("expected archive snapshot event to roll back after archive project failure, got %#v", repo.eventTypes)
	}
	if repo.projects[projectID].Status == ProjectStatusArchived {
		t.Fatalf("project must not be marked archived when repository archive update fails")
	}
}

func TestSubmitDemandSignalsProjectCoordinatorInV1(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "客户侧 Runtime 接入验收",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       ownerID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
		CoordinationStatus:     "registered",
	}
	seedHumanOwnerMember(repo, tenantID, projectID, ownerID)

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		SubmittedByUserID: ownerID,
		Title:             "验证 Runtime 连接",
		Content:           "检查心跳和命令回写",
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}
	if demand.Status != ProjectDemandStatusPlanningPending {
		t.Fatalf("expected planning pending demand, got %s", demand.Status)
	}
	if coordinator.demandSignals != 1 {
		t.Fatalf("expected one DemandSubmitted signal, got %d", coordinator.demandSignals)
	}
	if coordinator.lastDemand.DemandID != demand.ID || coordinator.lastDemand.CreatedEventID == uuid.Nil {
		t.Fatalf("unexpected demand signal: %#v", coordinator.lastDemand)
	}
}

func TestSubmitDemandRecordsRetryableWorkflowSignalFailure(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{demandSignalErr: errors.New("temporal unavailable")}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "客户侧 Runtime 接入验收",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       ownerID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
		CoordinationStatus:     "registered",
	}
	seedHumanOwnerMember(repo, tenantID, projectID, ownerID)

	_, err = service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		SubmittedByUserID: ownerID,
		Title:             "验证 Runtime 连接",
		Content:           "检查心跳和命令回写",
	})
	if err == nil {
		t.Fatal("expected signal error")
	}
	if len(repo.eventTypes) != 2 || repo.eventTypes[1] != ProjectEventWorkflowSignaled {
		t.Fatalf("expected workflow signal failure event, got %#v", repo.eventTypes)
	}
	payload := repo.events[len(repo.events)-1].Payload
	if payload["signal_name"] != "DemandSubmitted" || payload["status"] != "failed" || payload["retryable"] != true {
		t.Fatalf("unexpected workflow signal payload: %#v", payload)
	}
	if payload["demand_id"] == "" || payload["error"] == "" {
		t.Fatalf("expected retry payload to include demand id and error: %#v", payload)
	}
}

func TestRetryWorkflowSignalReplaysFailedDemandSignal(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{demandSignalErr: fmt.Errorf("temporal unavailable")}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "客户侧 Runtime 接入验收",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       ownerID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
		CoordinationStatus:     "registered",
	}
	seedHumanOwnerMember(repo, tenantID, projectID, ownerID)

	_, err = service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		SubmittedByUserID: ownerID,
		Title:             "验证 Runtime 连接",
		Content:           "检查心跳和命令回写",
	})
	if err == nil {
		t.Fatal("expected first signal error")
	}
	failedEvent := repo.events[len(repo.events)-1]
	coordinator.demandSignalErr = nil

	event, err := service.RetryWorkflowSignal(context.Background(), RetryWorkflowSignalRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventID:   failedEvent.ID,
		ActorID:   ownerID,
	})
	if err != nil {
		t.Fatalf("retry workflow signal: %v", err)
	}
	if repo.demands[0].CreatedEventID == nil {
		t.Fatalf("expected demand created event id: %#v", repo.demands[0])
	}
	if coordinator.demandSignals != 2 || coordinator.lastDemand.DemandID != repo.demands[0].ID || coordinator.lastDemand.CreatedEventID != *repo.demands[0].CreatedEventID {
		t.Fatalf("expected demand signal replay, count=%d signal=%#v demand=%#v", coordinator.demandSignals, coordinator.lastDemand, repo.demands[0])
	}
	if event.EventType != ProjectEventWorkflowSignaled || event.Payload["signal_name"] != "DemandSubmitted" || event.Payload["status"] != "sent" || event.Payload["retry_of_event_id"] != failedEvent.ID.String() {
		t.Fatalf("unexpected retry event: %#v", event)
	}
}

func TestCompleteProjectTaskWritesSummaryAndSignalsCoordinator(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	summary, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:              tenantID,
		RuntimeNodeID:         runtimeNodeID,
		ProjectTaskID:         taskID,
		DigitalEmployeeID:     employeeID,
		Conclusion:            "证据充分",
		EvidenceRefs:          []any{"s3://bucket/report.md"},
		ArtifactRefs:          []any{"artifact-1"},
		ConfidenceFactors:     map[string]any{"tests": "passed"},
		RecommendedNextAction: "提交负责人验收",
	})
	if err != nil {
		t.Fatalf("complete project task: %v", err)
	}
	if summary.ProjectTaskID != taskID || summary.DigitalEmployeeID != employeeID {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if summary.CreatedEventID == nil {
		t.Fatalf("expected summary to reference created event: %#v", summary)
	}
	if repo.tasks[0].Status != "completed" {
		t.Fatalf("expected task completed, got %s", repo.tasks[0].Status)
	}
	if coordinator.completedSignals != 1 || coordinator.lastCompleted.ExecutionSummaryID != summary.ID {
		t.Fatalf("expected completed signal for summary, got count=%d signal=%#v", coordinator.completedSignals, coordinator.lastCompleted)
	}
	if repo.eventTypes[len(repo.eventTypes)-1] != ProjectEventTaskCompleted {
		t.Fatalf("expected completed event, got %#v", repo.eventTypes)
	}
}

func TestRetryWorkflowSignalReplaysCompletedTaskWithoutDuplicateWriteback(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{completedSignalErr: fmt.Errorf("temporal unavailable")}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "证据充分",
	})
	if err == nil {
		t.Fatal("expected first completed signal error")
	}
	if len(repo.executionSummaries) != 1 {
		t.Fatalf("expected one summary after failed signal, got %d", len(repo.executionSummaries))
	}
	completedEvents := countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted)
	if completedEvents != 1 {
		t.Fatalf("expected one completed event after failed signal, got %d events=%#v", completedEvents, repo.eventTypes)
	}
	failedSignalEvent := repo.events[len(repo.events)-1]
	coordinator.completedSignalErr = nil

	retryEvent, err := service.RetryWorkflowSignal(context.Background(), RetryWorkflowSignalRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventID:   failedSignalEvent.ID,
		ActorID:   repo.projects[projectID].HumanOwnerUserID,
	})
	if err != nil {
		t.Fatalf("retry completed workflow signal: %v", err)
	}
	if coordinator.completedSignals != 2 || coordinator.lastCompleted.ProjectTaskID != taskID || coordinator.lastCompleted.ExecutionSummaryID != repo.executionSummaries[0].ID {
		t.Fatalf("expected completed signal replay, count=%d signal=%#v summary=%#v", coordinator.completedSignals, coordinator.lastCompleted, repo.executionSummaries[0])
	}
	if len(repo.executionSummaries) != 1 {
		t.Fatalf("expected retry not to create duplicate summary, got %d", len(repo.executionSummaries))
	}
	if countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted) != 1 {
		t.Fatalf("expected retry not to create duplicate completed event, events=%#v", repo.eventTypes)
	}
	if retryEvent.Payload["status"] != "sent" || retryEvent.Payload["retry_of_event_id"] != failedSignalEvent.ID.String() {
		t.Fatalf("unexpected retry event payload: %#v", retryEvent.Payload)
	}
}

func TestProjectCoordinationBackendE2ESimulation(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{demandSignalErr: fmt.Errorf("temporal unavailable")}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "E2E 仿真项目",
		Goal:                   "验证需求、Runtime 写回和 Workflow signal 重试闭环",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       ownerID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
		CoordinationStatus:     "registered",
	}
	seedHumanOwnerMember(repo, tenantID, projectID, ownerID)

	_, err = service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		SubmittedByUserID: ownerID,
		Title:             "验证 Runtime 执行回写",
		Content:           "模拟 Temporal 短暂不可用后的重试恢复",
	})
	if err == nil {
		t.Fatal("expected demand signal failure")
	}
	if len(repo.demands) != 1 || countProjectEvents(repo.eventTypes, ProjectEventDemandSubmitted) != 1 {
		t.Fatalf("expected one persisted demand before retry, demands=%d events=%#v", len(repo.demands), repo.eventTypes)
	}
	failedDemandSignalEvent := repo.events[len(repo.events)-1]
	if failedDemandSignalEvent.EventType != ProjectEventWorkflowSignaled || failedDemandSignalEvent.Payload["signal_name"] != "DemandSubmitted" || failedDemandSignalEvent.Payload["status"] != "failed" {
		t.Fatalf("expected retryable demand signal failure event, got %#v", failedDemandSignalEvent)
	}

	coordinator.demandSignalErr = nil
	retryDemandEvent, err := service.RetryWorkflowSignal(context.Background(), RetryWorkflowSignalRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventID:   failedDemandSignalEvent.ID,
		ActorID:   ownerID,
	})
	if err != nil {
		t.Fatalf("retry demand workflow signal: %v", err)
	}
	if retryDemandEvent.Payload["status"] != "sent" || retryDemandEvent.Payload["retry_of_event_id"] != failedDemandSignalEvent.ID.String() {
		t.Fatalf("unexpected demand retry event payload: %#v", retryDemandEvent.Payload)
	}
	if coordinator.demandSignals != 2 || len(repo.demands) != 1 || countProjectEvents(repo.eventTypes, ProjectEventDemandSubmitted) != 1 {
		t.Fatalf("expected demand retry to only resend signal, signals=%d demands=%d events=%#v", coordinator.demandSignals, len(repo.demands), repo.eventTypes)
	}

	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理执行证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     uuid.New(),
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "错误 Runtime 尝试写回",
	})
	if !errors.Is(err, ErrProjectTaskForbidden) {
		t.Fatalf("expected wrong runtime rejection, got %v", err)
	}
	if repo.tasks[0].Status != "assigned" || len(repo.executionSummaries) != 0 || countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted) != 0 {
		t.Fatalf("expected rejected runtime writeback to have no side effects, task=%#v summaries=%d events=%#v", repo.tasks[0], len(repo.executionSummaries), repo.eventTypes)
	}

	coordinator.completedSignalErr = fmt.Errorf("temporal unavailable")
	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:              tenantID,
		RuntimeNodeID:         runtimeNodeID,
		ProjectTaskID:         taskID,
		DigitalEmployeeID:     employeeID,
		Conclusion:            "证据充分",
		EvidenceRefs:          []any{"s3://bucket/e2e-report.md"},
		ArtifactRefs:          []any{"artifact-runtime-log"},
		ConfidenceFactors:     map[string]any{"tests": "passed"},
		RecommendedNextAction: "提交负责人验收",
	})
	if err == nil {
		t.Fatal("expected completed task signal failure")
	}
	if repo.tasks[0].Status != "completed" || len(repo.executionSummaries) != 1 || countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted) != 1 {
		t.Fatalf("expected successful writeback before signal retry, task=%#v summaries=%d events=%#v", repo.tasks[0], len(repo.executionSummaries), repo.eventTypes)
	}
	failedCompletedSignalEvent := repo.events[len(repo.events)-1]
	if failedCompletedSignalEvent.EventType != ProjectEventWorkflowSignaled || failedCompletedSignalEvent.Payload["signal_name"] != "EmployeeTaskCompleted" || failedCompletedSignalEvent.Payload["status"] != "failed" {
		t.Fatalf("expected retryable completed signal failure event, got %#v", failedCompletedSignalEvent)
	}

	coordinator.completedSignalErr = nil
	retryCompletedEvent, err := service.RetryWorkflowSignal(context.Background(), RetryWorkflowSignalRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventID:   failedCompletedSignalEvent.ID,
		ActorID:   ownerID,
	})
	if err != nil {
		t.Fatalf("retry completed workflow signal: %v", err)
	}
	if retryCompletedEvent.Payload["status"] != "sent" || retryCompletedEvent.Payload["retry_of_event_id"] != failedCompletedSignalEvent.ID.String() {
		t.Fatalf("unexpected completed retry event payload: %#v", retryCompletedEvent.Payload)
	}
	if coordinator.completedSignals != 2 || coordinator.lastCompleted.ExecutionSummaryID != repo.executionSummaries[0].ID {
		t.Fatalf("expected completed signal replay, signals=%d last=%#v summary=%#v", coordinator.completedSignals, coordinator.lastCompleted, repo.executionSummaries[0])
	}
	if len(repo.executionSummaries) != 1 || countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted) != 1 {
		t.Fatalf("expected completed retry not to duplicate facts, summaries=%d events=%#v", len(repo.executionSummaries), repo.eventTypes)
	}

	demands, err := service.ListProjectDemands(context.Background(), tenantID, projectID, 50, 0)
	if err != nil {
		t.Fatalf("list demands: %v", err)
	}
	summaries, err := service.ListExecutionSummaries(context.Background(), tenantID, projectID, 50, 0)
	if err != nil {
		t.Fatalf("list execution summaries: %v", err)
	}
	events, err := service.ListProjectEvents(context.Background(), tenantID, projectID, 50, 0)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(demands) != 1 || len(summaries) != 1 || countProjectEvents(projectEventTypes(events), ProjectEventWorkflowSignaled) != 4 {
		t.Fatalf("unexpected API-facing read model: demands=%d summaries=%d events=%#v", len(demands), len(summaries), projectEventTypes(events))
	}
}

func TestProjectTaskWritebackRequiresRuntimeNodeIdentity(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewServiceWithCoordinator(repo, &fakeCoordinatorSignalClient{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "证据充分",
	})
	if !errors.Is(err, ErrProjectTaskForbidden) {
		t.Fatalf("expected runtime identity rejection, got %v", err)
	}
	if len(repo.executionSummaries) != 0 || len(repo.eventTypes) != 0 {
		t.Fatalf("expected rejection before side effects, summaries=%d events=%#v", len(repo.executionSummaries), repo.eventTypes)
	}
}

func TestProjectTaskWritebackRequiresDigitalEmployeeRunBinding(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewServiceWithCoordinator(repo, &fakeCoordinatorSignalClient{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     uuid.New(),
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "未绑定运行记录",
	})
	if !errors.Is(err, ErrProjectTaskForbidden) {
		t.Fatalf("expected missing run binding rejection, got %v", err)
	}
	if len(repo.executionSummaries) != 0 || len(repo.eventTypes) != 0 {
		t.Fatalf("expected rejection before side effects, summaries=%d events=%#v", len(repo.executionSummaries), repo.eventTypes)
	}
}

func TestCompleteProjectTaskRejectsTerminalReplay(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "completed",
		AssignedDigitalEmployeeID: &employeeID,
	})

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     uuid.New(),
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "重复完成",
	})
	if !errors.Is(err, ErrProjectTaskForbidden) {
		t.Fatalf("expected terminal replay rejection, got %v", err)
	}
	if len(repo.executionSummaries) != 0 || len(repo.eventTypes) != 0 || coordinator.completedSignals != 0 {
		t.Fatalf("expected rejection before side effects, summaries=%d events=%#v signals=%d", len(repo.executionSummaries), repo.eventTypes, coordinator.completedSignals)
	}
}

func TestCompleteProjectTaskRejectsConcurrentTerminalTransitionBeforeSideEffects(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	completed := "completed"
	repo.taskStatusBeforeUpdate = &completed
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "并发完成",
	})
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected conditional status update rejection, got %v", err)
	}
	if len(repo.executionSummaries) != 0 || len(repo.eventTypes) != 0 || coordinator.completedSignals != 0 {
		t.Fatalf("expected rejection before side effects, summaries=%d events=%#v signals=%d", len(repo.executionSummaries), repo.eventTypes, coordinator.completedSignals)
	}
}

func TestCompleteProjectTaskRollsBackStatusWhenSummaryCreationFails(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.createExecutionSummaryErr = fmt.Errorf("summary unavailable")
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "写摘要失败",
	})
	if err == nil {
		t.Fatal("expected summary creation error")
	}
	if repo.tasks[0].Status != "assigned" || len(repo.executionSummaries) != 0 || len(repo.eventTypes) != 0 || coordinator.completedSignals != 0 {
		t.Fatalf("expected rollback before side effects, task=%#v summaries=%d events=%#v signals=%d", repo.tasks[0], len(repo.executionSummaries), repo.eventTypes, coordinator.completedSignals)
	}
}

func TestFailProjectTaskRollsBackStatusWhenEventAppendFails(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.appendProjectEventErr = fmt.Errorf("event store unavailable")
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "running",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	_, err = service.FailProjectTask(context.Background(), FailProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		FailureSummary:    "工具链失败",
	})
	if err == nil {
		t.Fatal("expected event append error")
	}
	if repo.tasks[0].Status != "running" || len(repo.eventTypes) != 0 || coordinator.failedSignals != 0 {
		t.Fatalf("expected rollback before side effects, task=%#v events=%#v signals=%d", repo.tasks[0], repo.eventTypes, coordinator.failedSignals)
	}
}

func TestRequestProjectTaskTransferRollsBackStatusWhenTransferCreationFails(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.createTransferRequestErr = fmt.Errorf("transfer store unavailable")
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	_, err = service.RequestProjectTaskTransfer(context.Background(), RequestProjectTaskTransferRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Reason:            "上下文不足",
	})
	if err == nil {
		t.Fatal("expected transfer creation error")
	}
	if repo.tasks[0].Status != "assigned" || len(repo.transferRequests) != 0 || len(repo.eventTypes) != 0 || coordinator.transferSignals != 0 {
		t.Fatalf("expected rollback before side effects, task=%#v transfers=%d events=%#v signals=%d", repo.tasks[0], len(repo.transferRequests), repo.eventTypes, coordinator.transferSignals)
	}
}

func TestCompleteProjectTaskRejectsWrongRuntimeWhenRunIsBound(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewServiceWithCoordinator(repo, &fakeCoordinatorSignalClient{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runID := uuid.New()
	expectedRuntimeNodeID := uuid.New()
	repo.projectTaskRunRuntimeNodes[taskID] = expectedRuntimeNodeID
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
		DigitalEmployeeRunID:      &runID,
	})

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     uuid.New(),
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "错误 Runtime 写回",
	})
	if !errors.Is(err, ErrProjectTaskForbidden) {
		t.Fatalf("expected wrong runtime rejection, got %v", err)
	}
	if len(repo.executionSummaries) != 0 || len(repo.eventTypes) != 0 {
		t.Fatalf("expected rejection before side effects, summaries=%d events=%#v", len(repo.executionSummaries), repo.eventTypes)
	}
}

func TestRequestProjectTaskTransferRejectsWaitingHumanTask(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "等待负责人确认",
		Status:                    "waiting_human",
		AssignedDigitalEmployeeID: &employeeID,
	})

	_, err = service.RequestProjectTaskTransfer(context.Background(), RequestProjectTaskTransferRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     uuid.New(),
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Reason:            "上下文不足",
	})
	if !errors.Is(err, ErrProjectTaskForbidden) {
		t.Fatalf("expected waiting human transfer rejection, got %v", err)
	}
	if len(repo.transferRequests) != 0 || len(repo.eventTypes) != 0 || coordinator.transferSignals != 0 {
		t.Fatalf("expected rejection before side effects, transfers=%d events=%#v signals=%d", len(repo.transferRequests), repo.eventTypes, coordinator.transferSignals)
	}
}

func TestRequestProjectTaskTransferMovesTaskToWaitingHuman(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	transfer, err := service.RequestProjectTaskTransfer(context.Background(), RequestProjectTaskTransferRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Reason:            "上下文不足",
	})
	if err != nil {
		t.Fatalf("request transfer: %v", err)
	}
	if transfer.Status != "requested" || repo.tasks[0].Status != "waiting_human" {
		t.Fatalf("expected transfer to pause task, transfer=%#v task=%#v", transfer, repo.tasks[0])
	}
	if coordinator.transferSignals != 1 {
		t.Fatalf("expected transfer signal, got %d", coordinator.transferSignals)
	}
}

func TestResolveDecisionUsesApprovalAndSignalsCoordinator(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	approvalID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:                decisionID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: approvalID,
		TargetUserID:      actorID,
		DecisionType:      "route_review",
		TitleSnapshot:     "需要负责人确认",
		StatusSnapshot:    "pending",
	})

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   actorID,
		Decision:          "approved",
		Comment:           "同意",
		Payload:           map[string]any{"source": "console"},
	})
	if err != nil {
		t.Fatalf("resolve decision: %v", err)
	}
	if resolved.StatusSnapshot != "approved" {
		t.Fatalf("expected approved projection, got %s", resolved.StatusSnapshot)
	}
	if approvals.calls != 1 || approvals.last.ApprovalRequestID != approvalID || approvals.last.Decision != "approved" {
		t.Fatalf("expected approval resolver call, got count=%d last=%#v", approvals.calls, approvals.last)
	}
	if coordinator.decisionSignals != 1 || coordinator.lastDecision.DecisionRequestID != decisionID || coordinator.lastDecision.ResolvedEventID == uuid.Nil {
		t.Fatalf("expected decision signal, got count=%d signal=%#v", coordinator.decisionSignals, coordinator.lastDecision)
	}
}

func TestResolveDecisionFindsDecisionBeyondFirstPage(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	targetDecisionID := uuid.New()
	targetApprovalID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	for i := 0; i < 100; i++ {
		repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
			ID:                uuid.New(),
			TenantID:          tenantID,
			ProjectID:         projectID,
			ApprovalRequestID: uuid.New(),
			TargetUserID:      actorID,
			DecisionType:      "route_review",
			TitleSnapshot:     "较新的决策",
			StatusSnapshot:    "pending",
			CreatedAt:         time.Now().UTC().Add(time.Duration(i+1) * time.Minute),
		})
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:                targetDecisionID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: targetApprovalID,
		TargetUserID:      actorID,
		DecisionType:      "route_review",
		TitleSnapshot:     "较早的决策",
		StatusSnapshot:    "pending",
		CreatedAt:         time.Now().UTC().Add(-time.Hour),
	})

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: targetDecisionID,
		DecidedByUserID:   actorID,
		Decision:          "approved",
	})
	if err != nil {
		t.Fatalf("resolve older decision: %v", err)
	}
	if resolved.ID != targetDecisionID || approvals.last.ApprovalRequestID != targetApprovalID {
		t.Fatalf("expected target decision to resolve, decision=%#v approval=%#v", resolved, approvals.last)
	}
}

func TestUpdateConfigRejectsArchivedProject(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         uuid.New(),
		Name:             "已归档项目",
		Status:           ProjectStatusArchived,
		HumanOwnerUserID: uuid.New(),
	}
	_, err = service.UpdateProjectConfig(context.Background(), UpdateProjectConfigRequest{
		TenantID:    repo.projects[projectID].TenantID,
		ProjectID:   projectID,
		ActorUserID: uuid.New(),
		Name:        "新名称",
	})
	if !errors.Is(err, ErrProjectArchived) {
		t.Fatalf("expected archived error, got %v", err)
	}
}

func TestUpdateProjectConfigCreatesRevision(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "旧项目",
		Goal:             "旧目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: uuid.New(),
	}

	updated, err := service.UpdateProjectConfig(context.Background(), UpdateProjectConfigRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ActorUserID: uuid.New(),
		Name:        "新项目",
		Goal:        "新目标",
	})
	if err != nil {
		t.Fatalf("update config: %v", err)
	}
	if updated.Name != "新项目" {
		t.Fatalf("expected updated project name, got %q", updated.Name)
	}
	if len(repo.revisions) != 1 {
		t.Fatalf("expected config revision, got %d", len(repo.revisions))
	}
	if len(repo.eventTypes) != 1 || repo.eventTypes[0] != ProjectEventConfigChanged {
		t.Fatalf("expected config changed event, got %#v", repo.eventTypes)
	}
}

func TestUpdateProjectConfigRecordsRetryableWorkflowSignalFailure(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{policySignalErr: errors.New("temporal unavailable")}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "旧项目",
		Goal:                   "旧目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}

	_, err = service.UpdateProjectConfig(context.Background(), UpdateProjectConfigRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ActorUserID: uuid.New(),
		Name:        "新项目",
		Goal:        "新目标",
	})
	if err == nil {
		t.Fatal("expected signal error")
	}
	if len(repo.eventTypes) != 2 || repo.eventTypes[1] != ProjectEventWorkflowSignaled {
		t.Fatalf("expected workflow signal failure event, got %#v", repo.eventTypes)
	}
	payload := repo.events[len(repo.events)-1].Payload
	if payload["signal_name"] != "ProjectPolicyChanged" || payload["status"] != "failed" || payload["retryable"] != true {
		t.Fatalf("unexpected workflow signal payload: %#v", payload)
	}
	if payload["changed_event_id"] == "" || payload["error"] == "" {
		t.Fatalf("expected retry payload to include event id and error: %#v", payload)
	}
}

func TestUpdateProjectConfigRejectsMissingIDs(t *testing.T) {
	service, err := NewService(newMemoryRepository())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	for _, tc := range []struct {
		name string
		req  UpdateProjectConfigRequest
	}{
		{name: "tenant", req: UpdateProjectConfigRequest{ProjectID: uuid.New(), ActorUserID: uuid.New()}},
		{name: "project", req: UpdateProjectConfigRequest{TenantID: uuid.New(), ActorUserID: uuid.New()}},
		{name: "actor", req: UpdateProjectConfigRequest{TenantID: uuid.New(), ProjectID: uuid.New()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.UpdateProjectConfig(context.Background(), tc.req)
			if !errors.Is(err, ErrInvalidProject) {
				t.Fatalf("expected invalid project error, got %v", err)
			}
		})
	}
}

func TestUpdateProjectConfigWithoutMembersPreservesExistingMembers(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	memberID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "旧项目",
		Goal:             "旧目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: memberID,
	}
	repo.members[projectID] = []ProjectMember{{
		ID:            uuid.New(),
		TenantID:      tenantID,
		ProjectID:     projectID,
		PrincipalType: PrincipalTypeHumanUser,
		PrincipalID:   memberID,
		ProjectRole:   ProjectRoleOwner,
		Status:        "active",
	}}

	_, err = service.UpdateProjectConfig(context.Background(), UpdateProjectConfigRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ActorUserID: uuid.New(),
		Name:        " 新项目 ",
	})
	if err != nil {
		t.Fatalf("update config: %v", err)
	}
	if got := repo.projects[projectID].Name; got != "新项目" {
		t.Fatalf("expected trimmed name, got %q", got)
	}
	if len(repo.members[projectID]) != 1 {
		t.Fatalf("expected members to be preserved, got %d", len(repo.members[projectID]))
	}
}

func TestReplaceProjectMembersRequiresActorAndRecordsEvent(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "项目",
		Goal:             "目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: uuid.New(),
	}

	_, err = service.ReplaceProjectMembers(context.Background(), tenantID, projectID, uuid.Nil, nil)
	if !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("expected invalid project error, got %v", err)
	}

	members, err := service.ReplaceProjectMembers(context.Background(), tenantID, projectID, uuid.New(), []ProjectMemberInput{{
		PrincipalType: PrincipalTypeDigitalEmployee,
		PrincipalID:   uuid.New(),
		ProjectRole:   ProjectRoleExecutor,
	}})
	if err != nil {
		t.Fatalf("replace members: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("expected one member, got %d", len(members))
	}
	if len(repo.eventTypes) != 1 || repo.eventTypes[0] != ProjectEventConfigChanged {
		t.Fatalf("expected config changed event, got %#v", repo.eventTypes)
	}
	if got := repo.events[0].Payload["member_count"]; got != 1 {
		t.Fatalf("expected member_count payload, got %#v", got)
	}
}

func TestListPaginationIsNormalized(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "项目",
		Goal:             "目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: uuid.New(),
	}

	if _, err := service.ListProjects(context.Background(), ListProjectsRequest{TenantID: tenantID, Limit: 200, Offset: -5}); err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if repo.lastListProjects.Limit != 100 || repo.lastListProjects.Offset != 0 {
		t.Fatalf("expected projects pagination 100/0, got %d/%d", repo.lastListProjects.Limit, repo.lastListProjects.Offset)
	}
	if _, err := service.ListProjectEvents(context.Background(), tenantID, projectID, 0, -1); err != nil {
		t.Fatalf("list events: %v", err)
	}
	if repo.lastEventsLimit != 50 || repo.lastEventsOffset != 0 {
		t.Fatalf("expected events pagination 50/0, got %d/%d", repo.lastEventsLimit, repo.lastEventsOffset)
	}
	if _, err := service.ListProjectDemands(context.Background(), tenantID, projectID, 101, -2); err != nil {
		t.Fatalf("list demands: %v", err)
	}
	if repo.lastDemandsLimit != 100 || repo.lastDemandsOffset != 0 {
		t.Fatalf("expected demands pagination 100/0, got %d/%d", repo.lastDemandsLimit, repo.lastDemandsOffset)
	}
	if _, err := service.GetOverview(context.Background(), tenantID, projectID); err != nil {
		t.Fatalf("get overview: %v", err)
	}
	if repo.lastTasksLimit != 20 || repo.lastTasksOffset != 0 || repo.lastEventsLimit != 20 || repo.lastEventsOffset != 0 {
		t.Fatalf("expected overview pagination 20/0, got tasks %d/%d events %d/%d", repo.lastTasksLimit, repo.lastTasksOffset, repo.lastEventsLimit, repo.lastEventsOffset)
	}
}

type memoryRepository struct {
	projects           map[uuid.UUID]Project
	members            map[uuid.UUID][]ProjectMember
	tasks              []ProjectTask
	events             []ProjectEvent
	eventTypes         []ProjectEventType
	demands            []ProjectDemand
	revisions          []ProjectConfigRevision
	coordinationJobs   []CoordinationJob
	routeDecisions     []RouteDecision
	executionSummaries []ExecutionSummary
	transferRequests   []TransferRequest
	decisionRequests   []DecisionRequest
	evidenceRefs       []ProjectEvidenceRef
	artifactRefs       []ProjectArtifactRef
	reportRefs         []ProjectReportRef
	budgetLedger       []ProjectBudgetLedgerEntry
	acceptanceRecords  []ProjectAcceptanceRecord
	archiveSnapshots   []ProjectArchiveSnapshot
	lastListProjects   ListProjectsRequest
	lastTasksLimit     int32
	lastTasksOffset    int32
	lastEventsLimit    int32
	lastEventsOffset   int32
	lastDemandsLimit   int32
	lastDemandsOffset  int32

	taskStatusBeforeUpdate     *string
	appendProjectEventErr      error
	createExecutionSummaryErr  error
	createTransferRequestErr   error
	archiveProjectErr          error
	projectTaskRunRuntimeNodes map[uuid.UUID]uuid.UUID
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		projects:                   map[uuid.UUID]Project{},
		members:                    map[uuid.UUID][]ProjectMember{},
		projectTaskRunRuntimeNodes: map[uuid.UUID]uuid.UUID{},
	}
}

func cloneProjects(projects map[uuid.UUID]Project) map[uuid.UUID]Project {
	cloned := make(map[uuid.UUID]Project, len(projects))
	for id, project := range projects {
		cloned[id] = project
	}
	return cloned
}

func strPtrOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func seedHumanOwnerMember(repo *memoryRepository, tenantID, projectID, ownerID uuid.UUID) {
	repo.members[projectID] = append(repo.members[projectID], ProjectMember{
		ID:            uuid.New(),
		TenantID:      tenantID,
		ProjectID:     projectID,
		PrincipalType: PrincipalTypeHumanUser,
		PrincipalID:   ownerID,
		ProjectRole:   ProjectRoleOwner,
		Status:        "active",
	})
}

func bindTaskToRuntimeRun(repo *memoryRepository, taskIndex int, runtimeNodeID uuid.UUID) uuid.UUID {
	runID := uuid.New()
	repo.tasks[taskIndex].DigitalEmployeeRunID = &runID
	repo.projectTaskRunRuntimeNodes[repo.tasks[taskIndex].ID] = runtimeNodeID
	return runID
}

func (r *memoryRepository) CreateProject(ctx context.Context, req CreateProjectRequest, projectID uuid.UUID, workflowID string) (Project, error) {
	project := Project{
		ID:                     projectID,
		TenantID:               req.TenantID,
		TeamID:                 req.TeamID,
		Name:                   req.Name,
		Description:            strPtrOrNil(req.Description),
		Goal:                   req.Goal,
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       req.HumanOwnerUserID,
		LeaderUserID:           req.LeaderUserID,
		AcceptanceUserID:       req.AcceptanceUserID,
		CoordinationWorkflowID: workflowID,
		CoordinationStatus:     "registered",
		CoordinationPolicy:     req.CoordinationPolicy,
		ApprovalPolicy:         req.ApprovalPolicy,
		EvidencePolicy:         req.EvidencePolicy,
	}
	r.projects[project.ID] = project
	return project, nil
}

func (r *memoryRepository) GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (Project, error) {
	project, ok := r.projects[projectID]
	if !ok || project.TenantID != tenantID {
		return Project{}, ErrProjectNotFound
	}
	return project, nil
}

func (r *memoryRepository) ListProjects(ctx context.Context, req ListProjectsRequest) ([]Project, error) {
	r.lastListProjects = req
	projects := make([]Project, 0, len(r.projects))
	for _, project := range r.projects {
		if project.TenantID != req.TenantID {
			continue
		}
		if req.Status != nil && project.Status != *req.Status {
			continue
		}
		if req.Query != "" && !strings.Contains(project.Name, req.Query) && !strings.Contains(project.Goal, req.Query) {
			continue
		}
		projects = append(projects, project)
	}
	return projects, nil
}

func (r *memoryRepository) UpdateProjectConfig(ctx context.Context, req UpdateProjectConfigRequest) (Project, error) {
	project, ok := r.projects[req.ProjectID]
	if !ok || project.TenantID != req.TenantID {
		return Project{}, ErrProjectNotFound
	}
	if req.Name != "" {
		project.Name = req.Name
	}
	if req.Description != "" {
		project.Description = strPtrOrNil(req.Description)
	}
	if req.Goal != "" {
		project.Goal = req.Goal
	}
	if req.HumanOwnerUserID != uuid.Nil {
		project.HumanOwnerUserID = req.HumanOwnerUserID
	}
	if req.LeaderUserID != nil {
		project.LeaderUserID = req.LeaderUserID
	}
	if req.AcceptanceUserID != nil {
		project.AcceptanceUserID = req.AcceptanceUserID
	}
	if req.CoordinationPolicy != nil {
		project.CoordinationPolicy = req.CoordinationPolicy
	}
	if req.ApprovalPolicy != nil {
		project.ApprovalPolicy = req.ApprovalPolicy
	}
	if req.EvidencePolicy != nil {
		project.EvidencePolicy = req.EvidencePolicy
	}
	r.projects[project.ID] = project
	return project, nil
}

func (r *memoryRepository) ArchiveProject(ctx context.Context, tenantID, projectID uuid.UUID) (Project, error) {
	if r.archiveProjectErr != nil {
		return Project{}, r.archiveProjectErr
	}
	project, ok := r.projects[projectID]
	if !ok || project.TenantID != tenantID {
		return Project{}, ErrProjectNotFound
	}
	now := time.Now()
	project.Status = ProjectStatusArchived
	project.ArchivedAt = &now
	r.projects[projectID] = project
	return project, nil
}

func (r *memoryRepository) ReplaceProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID, members []ProjectMemberInput) ([]ProjectMember, error) {
	project, ok := r.projects[projectID]
	if !ok || project.TenantID != tenantID {
		return nil, ErrProjectNotFound
	}
	mapped := make([]ProjectMember, 0, len(members))
	for _, member := range members {
		mapped = append(mapped, ProjectMember{
			ID:                  uuid.New(),
			TenantID:            tenantID,
			ProjectID:           projectID,
			PrincipalType:       member.PrincipalType,
			PrincipalID:         member.PrincipalID,
			ProjectRole:         member.ProjectRole,
			DisplayNameSnapshot: strPtrOrNil(member.DisplayNameSnapshot),
			Status:              "active",
			Settings:            member.Settings,
		})
	}
	r.members[projectID] = mapped
	return mapped, nil
}

func (r *memoryRepository) ListProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectMember, error) {
	members := r.members[projectID]
	filtered := make([]ProjectMember, 0, len(members))
	for _, member := range members {
		if member.TenantID == tenantID {
			filtered = append(filtered, member)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) ListProjectTasks(ctx context.Context, tenantID, projectID uuid.UUID, status *string, limit, offset int32) ([]ProjectTask, error) {
	r.lastTasksLimit = limit
	r.lastTasksOffset = offset
	filtered := make([]ProjectTask, 0, len(r.tasks))
	for _, task := range r.tasks {
		if task.TenantID == tenantID && task.ProjectID == projectID && (status == nil || task.Status == *status) {
			filtered = append(filtered, task)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) AppendProjectEvent(ctx context.Context, event AppendProjectEventRequest) (ProjectEvent, error) {
	if r.appendProjectEventErr != nil {
		return ProjectEvent{}, r.appendProjectEventErr
	}
	projectEvent := ProjectEvent{
		ID:             uuid.New(),
		TenantID:       event.TenantID,
		ProjectID:      event.ProjectID,
		SequenceNumber: int64(len(r.events) + 1),
		EventType:      event.EventType,
		ActorType:      event.ActorType,
		ActorID:        event.ActorID,
		ResourceType:   event.ResourceType,
		ResourceID:     event.ResourceID,
		Summary:        strPtrOrNil(event.Summary),
		Payload:        event.Payload,
	}
	r.events = append(r.events, projectEvent)
	r.eventTypes = append(r.eventTypes, event.EventType)
	return projectEvent, nil
}

func (r *memoryRepository) GetProjectEvent(ctx context.Context, tenantID, projectID, eventID uuid.UUID) (ProjectEvent, error) {
	for _, event := range r.events {
		if event.ID == eventID && event.TenantID == tenantID && event.ProjectID == projectID {
			return event, nil
		}
	}
	return ProjectEvent{}, ErrProjectNotFound
}

func (r *memoryRepository) ListProjectEvents(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectEvent, error) {
	r.lastEventsLimit = limit
	r.lastEventsOffset = offset
	filtered := make([]ProjectEvent, 0, len(r.events))
	for _, event := range r.events {
		if event.TenantID == tenantID && event.ProjectID == projectID {
			filtered = append(filtered, event)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) CreateProjectDemand(ctx context.Context, req SubmitProjectDemandRequest, status ProjectDemandStatus, createdEventID *uuid.UUID) (ProjectDemand, error) {
	demand := ProjectDemand{
		ID:                 uuid.New(),
		TenantID:           req.TenantID,
		ProjectID:          req.ProjectID,
		SubmittedByUserID:  req.SubmittedByUserID,
		Title:              req.Title,
		Content:            strPtrOrNil(req.Content),
		SourceType:         req.SourceType,
		SourceRefs:         req.SourceRefs,
		Attachments:        req.Attachments,
		ReviewerPreference: reviewerPreferenceFromSourceRefs(req.SourceRefs),
		Status:             status,
		CreatedEventID:     createdEventID,
	}
	r.demands = append(r.demands, demand)
	return demand, nil
}

func (r *memoryRepository) ListProjectDemands(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectDemand, error) {
	r.lastDemandsLimit = limit
	r.lastDemandsOffset = offset
	filtered := make([]ProjectDemand, 0, len(r.demands))
	for _, demand := range r.demands {
		if demand.TenantID == tenantID && demand.ProjectID == projectID {
			filtered = append(filtered, demand)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) CreateConfigRevision(ctx context.Context, req UpdateProjectConfigRequest, project Project, eventID uuid.UUID) (ProjectConfigRevision, error) {
	revision := ProjectConfigRevision{
		ID:              uuid.New(),
		TenantID:        req.TenantID,
		ProjectID:       req.ProjectID,
		RevisionNumber:  int32(len(r.revisions) + 1),
		ConfigSnapshot:  map[string]any{"name": project.Name, "status": string(project.Status)},
		ChangeSummary:   strPtrOrNil("项目配置已更新"),
		CreatedByUserID: req.ActorUserID,
		CreatedEventID:  &eventID,
	}
	r.revisions = append(r.revisions, revision)
	return revision, nil
}

func (r *memoryRepository) GetProjectDemand(ctx context.Context, tenantID, demandID uuid.UUID) (ProjectDemand, error) {
	for _, demand := range r.demands {
		if demand.ID == demandID && demand.TenantID == tenantID {
			return demand, nil
		}
	}
	return ProjectDemand{}, ErrProjectNotFound
}

func (r *memoryRepository) GetProjectTask(ctx context.Context, tenantID, projectTaskID uuid.UUID) (ProjectTask, error) {
	for _, task := range r.tasks {
		if task.ID == projectTaskID && task.TenantID == tenantID {
			return task, nil
		}
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) GetProjectTaskRunRuntimeNodeID(ctx context.Context, tenantID, projectTaskID, runID uuid.UUID) (uuid.UUID, error) {
	runtimeNodeID, ok := r.projectTaskRunRuntimeNodes[projectTaskID]
	if !ok {
		return uuid.Nil, ErrProjectNotFound
	}
	return runtimeNodeID, nil
}

func (r *memoryRepository) CreateCoordinationJob(ctx context.Context, req CreateCoordinationJobRequest) (CoordinationJob, error) {
	job := CoordinationJob{
		ID:               uuid.New(),
		TenantID:         req.TenantID,
		ProjectID:        req.ProjectID,
		WorkflowID:       req.WorkflowID,
		TriggerEventID:   req.TriggerEventID,
		JobType:          req.JobType,
		Status:           req.Status,
		InputSnapshotRef: req.InputSnapshotRef,
		OutputEventIDs:   []any{},
		CreatedAt:        time.Now().UTC(),
	}
	r.coordinationJobs = append(r.coordinationJobs, job)
	return job, nil
}

func (r *memoryRepository) FinishCoordinationJob(ctx context.Context, req FinishCoordinationJobRequest) (CoordinationJob, error) {
	for index, job := range r.coordinationJobs {
		if job.ID == req.ID && job.TenantID == req.TenantID {
			now := time.Now().UTC()
			job.Status = req.Status
			job.OutputEventIDs = req.OutputEventIDs
			job.FinishedAt = &now
			r.coordinationJobs[index] = job
			return job, nil
		}
	}
	return CoordinationJob{}, ErrProjectNotFound
}

func (r *memoryRepository) ListCoordinationJobs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]CoordinationJob, error) {
	filtered := make([]CoordinationJob, 0, len(r.coordinationJobs))
	for _, job := range r.coordinationJobs {
		if job.TenantID == tenantID && job.ProjectID == projectID {
			filtered = append(filtered, job)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) CreateRouteDecision(ctx context.Context, req CreateRouteDecisionRequest) (RouteDecision, error) {
	decision := RouteDecision{
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

func (r *memoryRepository) ListRouteDecisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]RouteDecision, error) {
	filtered := make([]RouteDecision, 0, len(r.routeDecisions))
	for _, decision := range r.routeDecisions {
		if decision.TenantID == tenantID && decision.ProjectID == projectID {
			filtered = append(filtered, decision)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) CreateProjectTask(ctx context.Context, req CreateProjectTaskRequest) (ProjectTask, error) {
	task := ProjectTask{
		ID:                        uuid.New(),
		TenantID:                  req.TenantID,
		ProjectID:                 req.ProjectID,
		DemandID:                  req.DemandID,
		Title:                     req.Title,
		Summary:                   strPtrOrNil(req.Summary),
		Status:                    req.Status,
		AssignedDigitalEmployeeID: req.AssignedDigitalEmployeeID,
		RiskLevel:                 strPtrOrNil(req.RiskLevel),
		RequiresHumanApproval:     req.RequiresHumanApproval,
		CreatedAt:                 time.Now().UTC(),
		UpdatedAt:                 time.Now().UTC(),
	}
	r.tasks = append(r.tasks, task)
	return task, nil
}

func (r *memoryRepository) UpdateProjectTaskStatus(ctx context.Context, tenantID, projectTaskID uuid.UUID, status string, eventID *uuid.UUID, currentStatuses []string) (ProjectTask, error) {
	for index, task := range r.tasks {
		if task.ID == projectTaskID && task.TenantID == tenantID {
			if r.taskStatusBeforeUpdate != nil {
				task.Status = *r.taskStatusBeforeUpdate
				r.tasks[index] = task
				r.taskStatusBeforeUpdate = nil
			}
			if !containsString(currentStatuses, task.Status) {
				return ProjectTask{}, ErrProjectNotFound
			}
			task.Status = status
			task.UpdatedAt = time.Now().UTC()
			r.tasks[index] = task
			return task, nil
		}
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) AssignProjectTask(ctx context.Context, tenantID, projectTaskID uuid.UUID, status string, assignedDigitalEmployeeID, eventID *uuid.UUID) (ProjectTask, error) {
	for index, task := range r.tasks {
		if task.ID == projectTaskID && task.TenantID == tenantID {
			task.Status = status
			task.AssignedDigitalEmployeeID = assignedDigitalEmployeeID
			task.UpdatedAt = time.Now().UTC()
			r.tasks[index] = task
			return task, nil
		}
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) CreateExecutionSummary(ctx context.Context, req CreateExecutionSummaryRequest) (ExecutionSummary, error) {
	if r.createExecutionSummaryErr != nil {
		return ExecutionSummary{}, r.createExecutionSummaryErr
	}
	summary := ExecutionSummary{
		ID:                    uuid.New(),
		TenantID:              req.TenantID,
		ProjectID:             req.ProjectID,
		ProjectTaskID:         req.ProjectTaskID,
		DigitalEmployeeID:     req.DigitalEmployeeID,
		Conclusion:            req.Conclusion,
		EvidenceRefs:          req.EvidenceRefs,
		ArtifactRefs:          req.ArtifactRefs,
		ConfidenceFactors:     req.ConfidenceFactors,
		Uncertainty:           strPtrOrNil(req.Uncertainty),
		MissingInformation:    req.MissingInformation,
		RecommendedNextAction: strPtrOrNil(req.RecommendedNextAction),
		RequiresHumanReview:   req.RequiresHumanReview,
		TransferRequestID:     req.TransferRequestID,
		CreatedEventID:        req.CreatedEventID,
		CreatedAt:             time.Now().UTC(),
	}
	r.executionSummaries = append(r.executionSummaries, summary)
	return summary, nil
}

func (r *memoryRepository) ListExecutionSummaries(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ExecutionSummary, error) {
	filtered := make([]ExecutionSummary, 0, len(r.executionSummaries))
	for _, summary := range r.executionSummaries {
		if summary.TenantID == tenantID && summary.ProjectID == projectID {
			filtered = append(filtered, summary)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) CreateTransferRequest(ctx context.Context, req CreateTransferRequestRequest) (TransferRequest, error) {
	if r.createTransferRequestErr != nil {
		return TransferRequest{}, r.createTransferRequestErr
	}
	transfer := TransferRequest{
		ID:                           uuid.New(),
		TenantID:                     req.TenantID,
		ProjectID:                    req.ProjectID,
		ProjectTaskID:                req.ProjectTaskID,
		RequestedByDigitalEmployeeID: req.RequestedByDigitalEmployeeID,
		Reason:                       req.Reason,
		SuggestedEmployeeType:        strPtrOrNil(req.SuggestedEmployeeType),
		SuggestedDigitalEmployeeIDs:  req.SuggestedDigitalEmployeeIDs,
		MissingContextRefs:           req.MissingContextRefs,
		Status:                       req.Status,
		CreatedEventID:               req.CreatedEventID,
		CreatedAt:                    time.Now().UTC(),
		UpdatedAt:                    time.Now().UTC(),
	}
	r.transferRequests = append(r.transferRequests, transfer)
	return transfer, nil
}

func (r *memoryRepository) CompleteProjectTaskWriteback(ctx context.Context, req CompleteProjectTaskWritebackRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	if _, err := r.UpdateProjectTaskStatus(ctx, req.Task.TenantID, req.Task.ID, "completed", nil, req.AllowedCurrentStatuses); err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	summaryReq := req.Summary
	summaryReq.CreatedEventID = &event.ID
	summary, err := r.CreateExecutionSummary(ctx, summaryReq)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	task, err := r.UpdateProjectTaskStatus(ctx, req.Task.TenantID, req.Task.ID, "completed", &event.ID, []string{"completed"})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	return ProjectTaskWritebackResult{Task: task, Event: event, Summary: summary}, nil
}

func (r *memoryRepository) FailProjectTaskWriteback(ctx context.Context, req FailProjectTaskWritebackRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	if _, err := r.UpdateProjectTaskStatus(ctx, req.Task.TenantID, req.Task.ID, "failed", nil, req.AllowedCurrentStatuses); err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	task, err := r.UpdateProjectTaskStatus(ctx, req.Task.TenantID, req.Task.ID, "failed", &event.ID, []string{"failed"})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	return ProjectTaskWritebackResult{Task: task, Event: event}, nil
}

func (r *memoryRepository) RequestProjectTaskTransferWriteback(ctx context.Context, req RequestProjectTaskTransferWritebackRequest) (ProjectTaskTransferWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	if _, err := r.UpdateProjectTaskStatus(ctx, req.Task.TenantID, req.Task.ID, "waiting_human", nil, req.AllowedCurrentStatuses); err != nil {
		return ProjectTaskTransferWritebackResult{}, err
	}
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskTransferWritebackResult{}, err
	}
	transferReq := req.Transfer
	transferReq.CreatedEventID = &event.ID
	transfer, err := r.CreateTransferRequest(ctx, transferReq)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskTransferWritebackResult{}, err
	}
	task, err := r.UpdateProjectTaskStatus(ctx, req.Task.TenantID, req.Task.ID, "waiting_human", &event.ID, []string{"waiting_human"})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskTransferWritebackResult{}, err
	}
	return ProjectTaskTransferWritebackResult{Task: task, Event: event, Transfer: transfer}, nil
}

type memoryWritebackSnapshot struct {
	tasks              []ProjectTask
	events             []ProjectEvent
	eventTypes         []ProjectEventType
	executionSummaries []ExecutionSummary
	transferRequests   []TransferRequest
}

func (r *memoryRepository) writebackSnapshot() memoryWritebackSnapshot {
	return memoryWritebackSnapshot{
		tasks:              append([]ProjectTask(nil), r.tasks...),
		events:             append([]ProjectEvent(nil), r.events...),
		eventTypes:         append([]ProjectEventType(nil), r.eventTypes...),
		executionSummaries: append([]ExecutionSummary(nil), r.executionSummaries...),
		transferRequests:   append([]TransferRequest(nil), r.transferRequests...),
	}
}

func (r *memoryRepository) restoreWritebackSnapshot(snapshot memoryWritebackSnapshot) {
	r.tasks = snapshot.tasks
	r.events = snapshot.events
	r.eventTypes = snapshot.eventTypes
	r.executionSummaries = snapshot.executionSummaries
	r.transferRequests = snapshot.transferRequests
}

func (r *memoryRepository) ListTransferRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]TransferRequest, error) {
	filtered := make([]TransferRequest, 0, len(r.transferRequests))
	for _, transfer := range r.transferRequests {
		if transfer.TenantID == tenantID && transfer.ProjectID == projectID {
			filtered = append(filtered, transfer)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) CreateDecisionRequest(ctx context.Context, req CreateDecisionRequestRequest) (DecisionRequest, error) {
	decision := DecisionRequest{
		ID:                uuid.New(),
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		ApprovalRequestID: req.ApprovalRequestID,
		CoordinationJobID: req.CoordinationJobID,
		ProjectTaskID:     req.ProjectTaskID,
		TargetUserID:      req.TargetUserID,
		DecisionType:      req.DecisionType,
		TitleSnapshot:     req.TitleSnapshot,
		SummarySnapshot:   strPtrOrNil(req.SummarySnapshot),
		RiskLevelSnapshot: strPtrOrNil(req.RiskLevelSnapshot),
		StatusSnapshot:    req.StatusSnapshot,
		CreatedEventID:    req.CreatedEventID,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	r.decisionRequests = append(r.decisionRequests, decision)
	return decision, nil
}

func (r *memoryRepository) GetDecisionRequest(ctx context.Context, tenantID, projectID, decisionRequestID uuid.UUID) (DecisionRequest, error) {
	for _, decision := range r.decisionRequests {
		if decision.ID == decisionRequestID && decision.TenantID == tenantID && decision.ProjectID == projectID {
			return decision, nil
		}
	}
	return DecisionRequest{}, ErrProjectNotFound
}

func (r *memoryRepository) ResolveDecisionRequest(ctx context.Context, req ResolveDecisionRequestRepositoryRequest) (DecisionRequest, error) {
	for index, decision := range r.decisionRequests {
		if decision.ID == req.ID && decision.TenantID == req.TenantID && decision.ProjectID == req.ProjectID {
			now := time.Now().UTC()
			decision.StatusSnapshot = req.StatusSnapshot
			decision.ResolvedEventID = req.ResolvedEventID
			decision.ResolvedAt = &now
			decision.UpdatedAt = now
			r.decisionRequests[index] = decision
			return decision, nil
		}
	}
	return DecisionRequest{}, ErrProjectNotFound
}

func (r *memoryRepository) ListDecisionRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]DecisionRequest, error) {
	filtered := make([]DecisionRequest, 0, len(r.decisionRequests))
	for _, decision := range r.decisionRequests {
		if decision.TenantID == tenantID && decision.ProjectID == projectID {
			filtered = append(filtered, decision)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	start := int(offset)
	if start > len(filtered) {
		return []DecisionRequest{}, nil
	}
	end := start + int(limit)
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], nil
}

func (r *memoryRepository) CreateEvidenceRefWithEvent(ctx context.Context, req CreateEvidenceRefWithEventRequest) (ProjectEvidenceRefWriteResult, error) {
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		return ProjectEvidenceRefWriteResult{}, err
	}
	evidenceReq := req.Evidence
	evidenceReq.CreatedEventID = &event.ID
	evidence, err := r.CreateEvidenceRef(ctx, evidenceReq)
	if err != nil {
		return ProjectEvidenceRefWriteResult{}, err
	}
	return ProjectEvidenceRefWriteResult{Event: event, Evidence: evidence}, nil
}

func (r *memoryRepository) UpdateEvidenceVerificationStatusWithEvent(ctx context.Context, req UpdateEvidenceVerificationStatusWithEventRequest) (ProjectEvidenceRefWriteResult, error) {
	evidence, err := r.UpdateEvidenceVerificationStatus(ctx, req.Evidence)
	if err != nil {
		return ProjectEvidenceRefWriteResult{}, err
	}
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		return ProjectEvidenceRefWriteResult{}, err
	}
	return ProjectEvidenceRefWriteResult{Event: event, Evidence: evidence}, nil
}

func (r *memoryRepository) CreateAcceptanceRecordWithEvent(ctx context.Context, req CreateAcceptanceRecordWithEventRequest) (ProjectAcceptanceRecordWriteResult, error) {
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		return ProjectAcceptanceRecordWriteResult{}, err
	}
	acceptanceReq := req.Acceptance
	acceptanceReq.CreatedEventID = &event.ID
	acceptance, err := r.CreateAcceptanceRecord(ctx, acceptanceReq)
	if err != nil {
		return ProjectAcceptanceRecordWriteResult{}, err
	}
	return ProjectAcceptanceRecordWriteResult{Event: event, Acceptance: acceptance}, nil
}

func (r *memoryRepository) CreateArchiveSnapshotWithEvent(ctx context.Context, req CreateArchiveSnapshotWithEventRequest) (ProjectArchiveSnapshotWriteResult, error) {
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	snapshotReq := req.Snapshot
	snapshotReq.CreatedEventID = &event.ID
	snapshot, err := r.CreateArchiveSnapshot(ctx, snapshotReq)
	if err != nil {
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	return ProjectArchiveSnapshotWriteResult{Event: event, Snapshot: snapshot}, nil
}

func (r *memoryRepository) CreateArchiveSnapshotWithEventAndArchiveProject(ctx context.Context, req CreateArchiveSnapshotWithEventRequest) (ProjectArchiveSnapshotWriteResult, error) {
	eventSnapshot := append([]ProjectEvent(nil), r.events...)
	eventTypesSnapshot := append([]ProjectEventType(nil), r.eventTypes...)
	archiveSnapshotsSnapshot := append([]ProjectArchiveSnapshot(nil), r.archiveSnapshots...)
	projectsSnapshot := cloneProjects(r.projects)
	result, err := r.CreateArchiveSnapshotWithEvent(ctx, req)
	if err != nil {
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	if _, err := r.ArchiveProject(ctx, req.Snapshot.TenantID, req.Snapshot.ProjectID); err != nil {
		r.events = eventSnapshot
		r.eventTypes = eventTypesSnapshot
		r.archiveSnapshots = archiveSnapshotsSnapshot
		r.projects = projectsSnapshot
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	return result, nil
}

type governanceMemoryRepository struct {
	*memoryRepository
	evidenceRefs         []ProjectEvidenceRef
	artifactRefs         []ProjectArtifactRef
	reportRefs           []ProjectReportRef
	budgetLedger         []ProjectBudgetLedgerEntry
	acceptanceRecords    []ProjectAcceptanceRecord
	archiveSnapshots     []ProjectArchiveSnapshot
	createEvidenceRefErr error
	createAcceptanceErr  error
	createArchiveSnapErr error
}

func newGovernanceMemoryRepository() *governanceMemoryRepository {
	return &governanceMemoryRepository{memoryRepository: newMemoryRepository()}
}

func (r *governanceMemoryRepository) CreateEvidenceRef(ctx context.Context, req CreateEvidenceRefRequest) (ProjectEvidenceRef, error) {
	if r.createEvidenceRefErr != nil {
		return ProjectEvidenceRef{}, r.createEvidenceRefErr
	}
	evidence := ProjectEvidenceRef{
		ID:                 uuid.New(),
		TenantID:           req.TenantID,
		ProjectID:          req.ProjectID,
		ProjectTaskID:      req.ProjectTaskID,
		RouteDecisionID:    req.RouteDecisionID,
		ExecutionSummaryID: req.ExecutionSummaryID,
		EvidenceType:       req.EvidenceType,
		Title:              req.Title,
		Summary:            strPtrOrNil(req.Summary),
		SourceType:         req.SourceType,
		SourceRef:          req.SourceRef,
		ArtifactRefID:      req.ArtifactRefID,
		SubmittedByType:    req.SubmittedByType,
		SubmittedByID:      req.SubmittedByID,
		VerificationStatus: req.VerificationStatus,
		Metadata:           req.Metadata,
		CreatedEventID:     req.CreatedEventID,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	r.evidenceRefs = append(r.evidenceRefs, evidence)
	return evidence, nil
}

func (r *governanceMemoryRepository) CreateEvidenceRefWithEvent(ctx context.Context, req CreateEvidenceRefWithEventRequest) (ProjectEvidenceRefWriteResult, error) {
	snapshot := r.governanceSnapshot()
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		return ProjectEvidenceRefWriteResult{}, err
	}
	evidenceReq := req.Evidence
	evidenceReq.CreatedEventID = &event.ID
	evidence, err := r.CreateEvidenceRef(ctx, evidenceReq)
	if err != nil {
		r.restoreGovernanceSnapshot(snapshot)
		return ProjectEvidenceRefWriteResult{}, err
	}
	return ProjectEvidenceRefWriteResult{Event: event, Evidence: evidence}, nil
}

func (r *governanceMemoryRepository) ListEvidenceRefs(ctx context.Context, tenantID, projectID uuid.UUID, status *EvidenceVerificationStatus, limit, offset int32) ([]ProjectEvidenceRef, error) {
	filtered := make([]ProjectEvidenceRef, 0, len(r.evidenceRefs))
	for _, evidence := range r.evidenceRefs {
		if evidence.TenantID == tenantID && evidence.ProjectID == projectID && (status == nil || evidence.VerificationStatus == *status) {
			filtered = append(filtered, evidence)
		}
	}
	return paginateSlice(filtered, limit, offset), nil
}

func (r *governanceMemoryRepository) UpdateEvidenceVerificationStatus(ctx context.Context, req UpdateEvidenceVerificationStatusRequest) (ProjectEvidenceRef, error) {
	for index, evidence := range r.evidenceRefs {
		if evidence.ID == req.ID && evidence.TenantID == req.TenantID && evidence.ProjectID == req.ProjectID {
			evidence.VerificationStatus = req.VerificationStatus
			if req.Metadata != nil {
				evidence.Metadata = req.Metadata
			}
			evidence.UpdatedAt = time.Now().UTC()
			r.evidenceRefs[index] = evidence
			return evidence, nil
		}
	}
	return ProjectEvidenceRef{}, ErrProjectNotFound
}

func (r *governanceMemoryRepository) UpdateEvidenceVerificationStatusWithEvent(ctx context.Context, req UpdateEvidenceVerificationStatusWithEventRequest) (ProjectEvidenceRefWriteResult, error) {
	snapshot := r.governanceSnapshot()
	evidence, err := r.UpdateEvidenceVerificationStatus(ctx, req.Evidence)
	if err != nil {
		return ProjectEvidenceRefWriteResult{}, err
	}
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		r.restoreGovernanceSnapshot(snapshot)
		return ProjectEvidenceRefWriteResult{}, err
	}
	return ProjectEvidenceRefWriteResult{Event: event, Evidence: evidence}, nil
}

func (r *governanceMemoryRepository) CreateArtifactRef(ctx context.Context, req CreateArtifactRefRequest) (ProjectArtifactRef, error) {
	artifact := ProjectArtifactRef{
		ID:              uuid.New(),
		TenantID:        req.TenantID,
		ProjectID:       req.ProjectID,
		ProjectTaskID:   req.ProjectTaskID,
		ArtifactID:      req.ArtifactID,
		ArtifactType:    req.ArtifactType,
		Title:           req.Title,
		ObjectRef:       req.ObjectRef,
		ContentType:     strPtrOrNil(req.ContentType),
		SizeBytes:       req.SizeBytes,
		Checksum:        strPtrOrNil(req.Checksum),
		RetentionStatus: req.RetentionStatus,
		RetentionHoldID: req.RetentionHoldID,
		Metadata:        req.Metadata,
		CreatedEventID:  req.CreatedEventID,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	r.artifactRefs = append(r.artifactRefs, artifact)
	return artifact, nil
}

func (r *governanceMemoryRepository) ListArtifactRefs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArtifactRef, error) {
	filtered := make([]ProjectArtifactRef, 0, len(r.artifactRefs))
	for _, artifact := range r.artifactRefs {
		if artifact.TenantID == tenantID && artifact.ProjectID == projectID {
			filtered = append(filtered, artifact)
		}
	}
	return paginateSlice(filtered, limit, offset), nil
}

func (r *governanceMemoryRepository) UpdateArtifactRetention(ctx context.Context, req UpdateArtifactRetentionRequest) (ProjectArtifactRef, error) {
	for index, artifact := range r.artifactRefs {
		if artifact.ID == req.ID && artifact.TenantID == req.TenantID && artifact.ProjectID == req.ProjectID {
			artifact.RetentionStatus = req.RetentionStatus
			artifact.RetentionHoldID = req.RetentionHoldID
			artifact.UpdatedAt = time.Now().UTC()
			r.artifactRefs[index] = artifact
			return artifact, nil
		}
	}
	return ProjectArtifactRef{}, ErrProjectNotFound
}

func (r *governanceMemoryRepository) CreateReportRef(ctx context.Context, req CreateReportRefRequest) (ProjectReportRef, error) {
	report := ProjectReportRef{
		ID:              uuid.New(),
		TenantID:        req.TenantID,
		ProjectID:       req.ProjectID,
		ReportType:      req.ReportType,
		Title:           req.Title,
		Summary:         strPtrOrNil(req.Summary),
		ObjectRef:       req.ObjectRef,
		Format:          req.Format,
		GeneratedByType: req.GeneratedByType,
		GeneratedByID:   req.GeneratedByID,
		CreatedEventID:  req.CreatedEventID,
		CreatedAt:       time.Now().UTC(),
	}
	r.reportRefs = append(r.reportRefs, report)
	return report, nil
}

func (r *governanceMemoryRepository) ListReportRefs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectReportRef, error) {
	filtered := make([]ProjectReportRef, 0, len(r.reportRefs))
	for _, report := range r.reportRefs {
		if report.TenantID == tenantID && report.ProjectID == projectID {
			filtered = append(filtered, report)
		}
	}
	return paginateSlice(filtered, limit, offset), nil
}

func (r *governanceMemoryRepository) CreateBudgetLedgerEntry(ctx context.Context, req CreateBudgetLedgerEntryRequest) (ProjectBudgetLedgerEntry, error) {
	entry := ProjectBudgetLedgerEntry{
		ID:                uuid.New(),
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		CoordinationJobID: req.CoordinationJobID,
		ProjectTaskID:     req.ProjectTaskID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		CostType:          req.CostType,
		EstimatedTokens:   req.EstimatedTokens,
		ActualTokens:      req.ActualTokens,
		EstimatedCost:     req.EstimatedCost,
		ActualCost:        req.ActualCost,
		Source:            req.Source,
		Reason:            strPtrOrNil(req.Reason),
		CreatedEventID:    req.CreatedEventID,
		CreatedAt:         time.Now().UTC(),
	}
	r.budgetLedger = append(r.budgetLedger, entry)
	return entry, nil
}

func (r *governanceMemoryRepository) ListBudgetLedger(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectBudgetLedgerEntry, error) {
	filtered := make([]ProjectBudgetLedgerEntry, 0, len(r.budgetLedger))
	for _, entry := range r.budgetLedger {
		if entry.TenantID == tenantID && entry.ProjectID == projectID {
			filtered = append(filtered, entry)
		}
	}
	return paginateSlice(filtered, limit, offset), nil
}

func (r *governanceMemoryRepository) GetBudgetSummary(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectBudgetSummary, error) {
	var summary ProjectBudgetSummary
	for _, entry := range r.budgetLedger {
		if entry.TenantID != tenantID || entry.ProjectID != projectID {
			continue
		}
		summary.LedgerCount++
		if entry.EstimatedTokens != nil {
			summary.EstimatedTokens += *entry.EstimatedTokens
		}
		if entry.ActualTokens != nil {
			summary.ActualTokens += *entry.ActualTokens
		}
		if entry.EstimatedCost != "" {
			summary.EstimatedCost = entry.EstimatedCost
		}
		if entry.ActualCost != "" {
			summary.ActualCost = entry.ActualCost
		}
	}
	return summary, nil
}

func (r *governanceMemoryRepository) CreateAcceptanceRecord(ctx context.Context, req CreateAcceptanceRecordRequest) (ProjectAcceptanceRecord, error) {
	if r.createAcceptanceErr != nil {
		return ProjectAcceptanceRecord{}, r.createAcceptanceErr
	}
	record := ProjectAcceptanceRecord{
		ID:               uuid.New(),
		TenantID:         req.TenantID,
		ProjectID:        req.ProjectID,
		AcceptedByUserID: req.AcceptedByUserID,
		Status:           req.Status,
		Conclusion:       req.Conclusion,
		Summary:          strPtrOrNil(req.Summary),
		EvidenceRefIDs:   req.EvidenceRefIDs,
		ReportRefIDs:     req.ReportRefIDs,
		UnresolvedRisks:  req.UnresolvedRisks,
		CreatedEventID:   req.CreatedEventID,
		CreatedAt:        time.Now().UTC(),
	}
	r.acceptanceRecords = append(r.acceptanceRecords, record)
	return record, nil
}

func (r *governanceMemoryRepository) CreateAcceptanceRecordWithEvent(ctx context.Context, req CreateAcceptanceRecordWithEventRequest) (ProjectAcceptanceRecordWriteResult, error) {
	snapshot := r.governanceSnapshot()
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		return ProjectAcceptanceRecordWriteResult{}, err
	}
	acceptanceReq := req.Acceptance
	acceptanceReq.CreatedEventID = &event.ID
	acceptance, err := r.CreateAcceptanceRecord(ctx, acceptanceReq)
	if err != nil {
		r.restoreGovernanceSnapshot(snapshot)
		return ProjectAcceptanceRecordWriteResult{}, err
	}
	return ProjectAcceptanceRecordWriteResult{Event: event, Acceptance: acceptance}, nil
}

func (r *governanceMemoryRepository) GetLatestAcceptanceRecord(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectAcceptanceRecord, error) {
	for index := len(r.acceptanceRecords) - 1; index >= 0; index-- {
		record := r.acceptanceRecords[index]
		if record.TenantID == tenantID && record.ProjectID == projectID {
			return record, nil
		}
	}
	return ProjectAcceptanceRecord{}, ErrProjectNotFound
}

func (r *governanceMemoryRepository) CreateArchiveSnapshot(ctx context.Context, req CreateArchiveSnapshotRequest) (ProjectArchiveSnapshot, error) {
	if r.createArchiveSnapErr != nil {
		return ProjectArchiveSnapshot{}, r.createArchiveSnapErr
	}
	snapshot := ProjectArchiveSnapshot{
		ID:                   uuid.New(),
		TenantID:             req.TenantID,
		ProjectID:            req.ProjectID,
		SnapshotType:         req.SnapshotType,
		Status:               req.Status,
		ObjectRef:            strPtrOrNil(req.ObjectRef),
		Summary:              strPtrOrNil(req.Summary),
		IncludedCounts:       req.IncludedCounts,
		RetainedArtifactIDs:  req.RetainedArtifactIDs,
		RetentionLockEventID: req.RetentionLockEventID,
		CreatedByUserID:      req.CreatedByUserID,
		CreatedEventID:       req.CreatedEventID,
		CreatedAt:            time.Now().UTC(),
	}
	r.archiveSnapshots = append(r.archiveSnapshots, snapshot)
	return snapshot, nil
}

func (r *governanceMemoryRepository) CreateArchiveSnapshotWithEvent(ctx context.Context, req CreateArchiveSnapshotWithEventRequest) (ProjectArchiveSnapshotWriteResult, error) {
	snapshot := r.governanceSnapshot()
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	snapshotReq := req.Snapshot
	snapshotReq.CreatedEventID = &event.ID
	archiveSnapshot, err := r.CreateArchiveSnapshot(ctx, snapshotReq)
	if err != nil {
		r.restoreGovernanceSnapshot(snapshot)
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	return ProjectArchiveSnapshotWriteResult{Event: event, Snapshot: archiveSnapshot}, nil
}

func (r *governanceMemoryRepository) CreateArchiveSnapshotWithEventAndArchiveProject(ctx context.Context, req CreateArchiveSnapshotWithEventRequest) (ProjectArchiveSnapshotWriteResult, error) {
	snapshot := r.governanceSnapshot()
	result, err := r.CreateArchiveSnapshotWithEvent(ctx, req)
	if err != nil {
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	if _, err := r.ArchiveProject(ctx, req.Snapshot.TenantID, req.Snapshot.ProjectID); err != nil {
		r.restoreGovernanceSnapshot(snapshot)
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	return result, nil
}

func (r *governanceMemoryRepository) ListArchiveSnapshots(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArchiveSnapshot, error) {
	filtered := make([]ProjectArchiveSnapshot, 0, len(r.archiveSnapshots))
	for _, snapshot := range r.archiveSnapshots {
		if snapshot.TenantID == tenantID && snapshot.ProjectID == projectID {
			filtered = append(filtered, snapshot)
		}
	}
	return paginateSlice(filtered, limit, offset), nil
}

func (r *governanceMemoryRepository) ListConfigRevisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectConfigRevision, error) {
	filtered := make([]ProjectConfigRevision, 0, len(r.revisions))
	for _, revision := range r.revisions {
		if revision.TenantID == tenantID && revision.ProjectID == projectID {
			filtered = append(filtered, revision)
		}
	}
	return paginateSlice(filtered, limit, offset), nil
}

func (r *governanceMemoryRepository) GetConfigRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (ProjectConfigRevision, error) {
	for _, revision := range r.revisions {
		if revision.ID == revisionID && revision.TenantID == tenantID && revision.ProjectID == projectID {
			return revision, nil
		}
	}
	return ProjectConfigRevision{}, ErrProjectNotFound
}

type governanceMemorySnapshot struct {
	projects          map[uuid.UUID]Project
	events            []ProjectEvent
	eventTypes        []ProjectEventType
	evidenceRefs      []ProjectEvidenceRef
	acceptanceRecords []ProjectAcceptanceRecord
	archiveSnapshots  []ProjectArchiveSnapshot
}

func (r *governanceMemoryRepository) governanceSnapshot() governanceMemorySnapshot {
	return governanceMemorySnapshot{
		projects:          cloneProjects(r.projects),
		events:            append([]ProjectEvent(nil), r.events...),
		eventTypes:        append([]ProjectEventType(nil), r.eventTypes...),
		evidenceRefs:      append([]ProjectEvidenceRef(nil), r.evidenceRefs...),
		acceptanceRecords: append([]ProjectAcceptanceRecord(nil), r.acceptanceRecords...),
		archiveSnapshots:  append([]ProjectArchiveSnapshot(nil), r.archiveSnapshots...),
	}
}

func (r *governanceMemoryRepository) restoreGovernanceSnapshot(snapshot governanceMemorySnapshot) {
	r.projects = snapshot.projects
	r.events = snapshot.events
	r.eventTypes = snapshot.eventTypes
	r.evidenceRefs = snapshot.evidenceRefs
	r.acceptanceRecords = snapshot.acceptanceRecords
	r.archiveSnapshots = snapshot.archiveSnapshots
}

type fakeArchiveArtifactLocker struct {
	artifactIDs []uuid.UUID
	holdIDs     []uuid.UUID
	eventID     *uuid.UUID
	err         error
}

func (l *fakeArchiveArtifactLocker) LockProjectArtifacts(ctx context.Context, tenantID, projectID uuid.UUID, artifactIDs []uuid.UUID) (ArchiveArtifactLockResult, error) {
	l.artifactIDs = append([]uuid.UUID(nil), artifactIDs...)
	if len(l.holdIDs) == 0 {
		l.holdIDs = make([]uuid.UUID, 0, len(artifactIDs))
		for range artifactIDs {
			l.holdIDs = append(l.holdIDs, uuid.New())
		}
	}
	return ArchiveArtifactLockResult{
		HoldIDs:     append([]uuid.UUID(nil), l.holdIDs...),
		ArtifactIDs: append([]uuid.UUID(nil), artifactIDs...),
		EventID:     l.eventID,
	}, l.err
}

type fakeCoordinatorSignalClient struct {
	ensureSignals      int
	demandSignals      int
	policySignals      int
	memberSignals      int
	completedSignals   int
	failedSignals      int
	transferSignals    int
	decisionSignals    int
	lastDemand         DemandSubmittedSignal
	lastCompleted      EmployeeTaskCompletedSignal
	lastDecision       HumanDecisionSubmittedSignal
	demandSignalErr    error
	policySignalErr    error
	completedSignalErr error
}

func (f *fakeCoordinatorSignalClient) EnsureProjectCoordinator(ctx context.Context, signal ProjectCoordinatorSignal) error {
	f.ensureSignals++
	return nil
}

func (f *fakeCoordinatorSignalClient) SignalDemandSubmitted(ctx context.Context, signal DemandSubmittedSignal) error {
	f.demandSignals++
	f.lastDemand = signal
	return f.demandSignalErr
}

func (f *fakeCoordinatorSignalClient) SignalProjectPolicyChanged(ctx context.Context, signal ProjectPolicyChangedSignal) error {
	f.policySignals++
	return f.policySignalErr
}

func (f *fakeCoordinatorSignalClient) SignalProjectMemberChanged(ctx context.Context, signal ProjectMemberChangedSignal) error {
	f.memberSignals++
	return nil
}

func (f *fakeCoordinatorSignalClient) SignalEmployeeTaskCompleted(ctx context.Context, signal EmployeeTaskCompletedSignal) error {
	f.completedSignals++
	f.lastCompleted = signal
	return f.completedSignalErr
}

func (f *fakeCoordinatorSignalClient) SignalEmployeeTaskFailed(ctx context.Context, signal EmployeeTaskFailedSignal) error {
	f.failedSignals++
	return nil
}

func (f *fakeCoordinatorSignalClient) SignalEmployeeTransferRequested(ctx context.Context, signal EmployeeTransferRequestedSignal) error {
	f.transferSignals++
	return nil
}

func (f *fakeCoordinatorSignalClient) SignalHumanDecisionSubmitted(ctx context.Context, signal HumanDecisionSubmittedSignal) error {
	f.decisionSignals++
	f.lastDecision = signal
	return nil
}

type fakeApprovalResolver struct {
	calls int
	last  ResolveApprovalRequest
}

func (f *fakeApprovalResolver) ResolveApproval(ctx context.Context, req ResolveApprovalRequest) error {
	f.calls++
	f.last = req
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func paginateSlice[T any](values []T, limit, offset int32) []T {
	start := int(offset)
	if start > len(values) {
		return []T{}
	}
	end := start + int(limit)
	if end > len(values) {
		end = len(values)
	}
	return values[start:end]
}

func countProjectEvents(values []ProjectEventType, target ProjectEventType) int {
	count := 0
	for _, value := range values {
		if value == target {
			count++
		}
	}
	return count
}

func projectEventTypes(events []ProjectEvent) []ProjectEventType {
	values := make([]ProjectEventType, 0, len(events))
	for _, event := range events {
		values = append(values, event.EventType)
	}
	return values
}
