package automation

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	CreateRule(ctx context.Context, rule Rule) (Rule, error)
	GetRule(ctx context.Context, tenantID, ruleID uuid.UUID) (Rule, error)
	ListRules(ctx context.Context, req ListRulesRequest) ([]Rule, error)
	UpdateRule(ctx context.Context, rule Rule) (Rule, error)
	SetRuleEnabled(ctx context.Context, tenantID, ruleID uuid.UUID, enabled bool, disabledReason *string) (Rule, error)
	SetRuleScheduleID(ctx context.Context, tenantID, ruleID uuid.UUID, scheduleID *string) (Rule, error)
	IncrementFailureCount(ctx context.Context, tenantID, ruleID uuid.UUID) (Rule, error)
	ResetFailureCount(ctx context.Context, tenantID, ruleID uuid.UUID) (Rule, error)
	DisableRuleSystem(ctx context.Context, tenantID, ruleID uuid.UUID, reason string) (Rule, error)
	DeleteRule(ctx context.Context, tenantID, ruleID uuid.UUID) error
	ListRulesByProject(ctx context.Context, tenantID, projectID uuid.UUID) ([]Rule, error)
	DeleteRulesForProject(ctx context.Context, tenantID, projectID uuid.UUID) error
	ListEnabledRulesByActor(ctx context.Context, tenantID, actorUserID uuid.UUID) ([]Rule, error)
	ListEnabledRulesByActorOnProject(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID) ([]Rule, error)

	CreateFire(ctx context.Context, fire Fire) (Fire, error)
	GetFireByIdempotency(ctx context.Context, idempotencyKey string) (Fire, error)
	UpdateFire(ctx context.Context, fire Fire) (Fire, error)
	ListFires(ctx context.Context, req ListFiresRequest) ([]Fire, error)
	GetLatestNonTerminalFire(ctx context.Context, tenantID, ruleID uuid.UUID) (Fire, error)
}

// ProjectGateway resolves project metadata and initiator eligibility for automation rules.
type ProjectGateway interface {
	GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectInfo, error)
	IsEligibleInitiator(ctx context.Context, tenantID, projectID, userID uuid.UUID) (bool, error)
	// MissingCastingRoles returns role keys still uncast (or cast to unavailable
	// employees) for the project×template. Empty = complete. Used at rule save
	// (G7) and fire time (G8). Nil-safe: implementers may return (nil, nil).
	MissingCastingRoles(ctx context.Context, tenantID, projectID uuid.UUID, templateKey string) ([]string, error)
}

type ProjectInfo struct {
	ID     uuid.UUID
	TeamID uuid.UUID
	Name   string
}

// DemandSubmitter submits plan/loop demands into the task hub.
type DemandSubmitter interface {
	SubmitDemand(ctx context.Context, req DemandSubmitRequest) (DemandSubmitResult, error)
}

type DemandSubmitRequest struct {
	TenantID            uuid.UUID
	ProjectID           uuid.UUID
	SubmittedByUserID   uuid.UUID
	Title               string
	Content             string
	CoordinationMode    string
	ScenarioTemplateKey *string
	SourceType          string
	SourceRefs          map[string]any
}

type DemandSubmitResult struct {
	DemandID uuid.UUID
}

// ChatRunner creates a digital-employee chat run.
type ChatRunner interface {
	CreateChatRun(ctx context.Context, tenantID, employeeID, projectID, actorUserID uuid.UUID, objective string, metadata map[string]any) (uuid.UUID, error)
}

// ScheduleSyncer keeps Temporal Schedules aligned with automation rules.
type ScheduleSyncer interface {
	Create(ctx context.Context, rule Rule) (scheduleID string, err error)
	Update(ctx context.Context, rule Rule) error
	Pause(ctx context.Context, scheduleID string, note string) error
	Unpause(ctx context.Context, scheduleID string, note string) error
	Delete(ctx context.Context, scheduleID string) error
}
