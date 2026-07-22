package automation

import (
	"context"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/employee"
	"github.com/superteam/control-plane/internal/project"
)

type demandSubmitterAdapter struct {
	projects *project.Service
}

func NewDemandSubmitter(projects *project.Service) DemandSubmitter {
	return demandSubmitterAdapter{projects: projects}
}

func (a demandSubmitterAdapter) SubmitDemand(ctx context.Context, req DemandSubmitRequest) (DemandSubmitResult, error) {
	if a.projects == nil {
		return DemandSubmitResult{}, ErrInvalidInput
	}
	demand, err := a.projects.SubmitDemand(ctx, project.SubmitProjectDemandRequest{
		TenantID:            req.TenantID,
		ProjectID:           req.ProjectID,
		SubmittedByUserID:   req.SubmittedByUserID,
		Title:               req.Title,
		Content:             req.Content,
		SourceType:          project.DemandSourceType(req.SourceType),
		SourceRefs:          req.SourceRefs,
		CoordinationMode:    req.CoordinationMode,
		ScenarioTemplateKey: req.ScenarioTemplateKey,
	})
	if err != nil {
		return DemandSubmitResult{}, err
	}
	return DemandSubmitResult{DemandID: demand.ID}, nil
}

type chatRunnerAdapter struct {
	runs *employee.DigitalEmployeeRunService
}

func NewChatRunner(runs *employee.DigitalEmployeeRunService) ChatRunner {
	return chatRunnerAdapter{runs: runs}
}

func (a chatRunnerAdapter) CreateChatRun(ctx context.Context, tenantID, employeeID, projectID, actorUserID uuid.UUID, objective string, metadata map[string]any) (uuid.UUID, error) {
	if a.runs == nil {
		return uuid.Nil, ErrInvalidInput
	}
	projectRef := projectID
	run, err := a.runs.CreateRun(ctx, employee.CreateDigitalEmployeeRunRequest{
		TenantID:          tenantID,
		UserID:            actorUserID,
		DigitalEmployeeID: employeeID,
		Objective:         objective,
		Prompt:            objective,
		RunKind:           employee.RunKindChat,
		ProjectID:         &projectRef,
		Metadata:          metadata,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return run.ID, nil
}
