package externalintegration

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/employee"
	"github.com/superteam/control-plane/internal/project"
	"github.com/superteam/control-plane/internal/skill"
)

// projectServiceGateway adapts project.Service (same equal-status human decider
// set as automation's gateway: owners + active human members) plus skill surface
// and digital-employee membership checks used at grant/call time.
type projectServiceGateway struct {
	projects *project.Service
	skills   *skill.Service
}

func NewProjectServiceGateway(projects *project.Service, skills *skill.Service) ProjectGateway {
	return projectServiceGateway{projects: projects, skills: skills}
}

func (g projectServiceGateway) GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectInfo, error) {
	record, err := g.projects.GetProject(ctx, tenantID, projectID)
	if err != nil {
		return ProjectInfo{}, err
	}
	if record == nil {
		return ProjectInfo{}, ErrNotFound
	}
	return ProjectInfo{
		ID:                 record.ID,
		Name:               record.Name,
		CoordinationPolicy: record.CoordinationPolicy,
	}, nil
}

func (g projectServiceGateway) IsEligibleInitiator(ctx context.Context, tenantID, projectID, userID uuid.UUID) (bool, error) {
	if userID == uuid.Nil {
		return false, nil
	}
	record, err := g.projects.GetProject(ctx, tenantID, projectID)
	if err != nil {
		return false, err
	}
	if record == nil {
		return false, ErrNotFound
	}
	if userID == record.HumanOwnerUserID {
		return true, nil
	}
	for _, ownerID := range record.HumanOwnerUserIDs {
		if ownerID == userID {
			return true, nil
		}
	}
	members, err := g.projects.ListProjectMembers(ctx, tenantID, projectID)
	if err != nil {
		return false, err
	}
	for _, member := range members {
		if member.PrincipalType == project.PrincipalTypeHumanUser && member.Status == "active" && member.PrincipalID == userID {
			return true, nil
		}
	}
	return false, nil
}

func (g projectServiceGateway) ListProjectSkillIDs(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID) ([]uuid.UUID, error) {
	if g.skills == nil {
		return nil, fmt.Errorf("%w: skill binding resolver is not configured", ErrForbidden)
	}
	bindings, err := g.skills.ListProjectSkillBindings(ctx, skill.ListProjectSkillBindingsRequest{
		TenantID:  tenantID,
		UserID:    actorUserID,
		ProjectID: projectID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]uuid.UUID, 0, len(bindings))
	for _, binding := range bindings {
		if binding.SkillID != uuid.Nil {
			out = append(out, binding.SkillID)
		}
	}
	return out, nil
}

func (g projectServiceGateway) IsDigitalEmployeeMember(ctx context.Context, tenantID, projectID, employeeID uuid.UUID) (bool, error) {
	if employeeID == uuid.Nil {
		return false, nil
	}
	members, err := g.projects.ListProjectMembers(ctx, tenantID, projectID)
	if err != nil {
		return false, err
	}
	for _, member := range members {
		if member.PrincipalType == project.PrincipalTypeDigitalEmployee &&
			member.Status == "active" &&
			member.PrincipalID == employeeID {
			return true, nil
		}
	}
	return false, nil
}

type chatRunnerAdapter struct {
	runs *employee.DigitalEmployeeRunService
}

func NewChatRunner(runs *employee.DigitalEmployeeRunService) ChatRunner {
	return chatRunnerAdapter{runs: runs}
}

func (a chatRunnerAdapter) CreateExternalChatRun(ctx context.Context, req ChatRunGatewayRequest) (uuid.UUID, string, error) {
	projectRef := req.ProjectID
	createReq := employee.CreateDigitalEmployeeRunRequest{
		TenantID:          req.TenantID,
		UserID:            req.ActorUserID,
		DigitalEmployeeID: req.EmployeeID,
		Objective:         req.Objective,
		Prompt:            req.Objective,
		RunKind:           employee.RunKindChat,
		ProjectID:         &projectRef,
		SkillIDs:          req.SkillIDs,
		Metadata:          req.Metadata,
	}
	if req.ResumeOfRunID != nil {
		createReq.ResumeOfRunID = req.ResumeOfRunID
	}
	run, err := a.runs.CreateRun(ctx, createReq)
	if err != nil {
		return uuid.Nil, "", err
	}
	return run.ID, string(run.Status), nil
}

type demandSubmitterAdapter struct {
	projects *project.Service
}

func NewDemandSubmitter(projects *project.Service) DemandSubmitter {
	return demandSubmitterAdapter{projects: projects}
}

func (a demandSubmitterAdapter) SubmitExternalDemand(ctx context.Context, req DemandGatewayRequest) (uuid.UUID, string, error) {
	demand, err := a.projects.SubmitDemand(ctx, project.SubmitProjectDemandRequest{
		TenantID:            req.TenantID,
		ProjectID:           req.ProjectID,
		SubmittedByUserID:   req.SubmittedByUserID,
		Title:               req.Title,
		Content:             req.Content,
		SourceType:          project.DemandSourceExternal,
		SourceRefs:          req.SourceRefs,
		CoordinationMode:    req.CoordinationMode,
		ScenarioTemplateKey: req.ScenarioTemplateKey,
	})
	if err != nil {
		return uuid.Nil, "", err
	}
	return demand.ID, string(demand.Status), nil
}
