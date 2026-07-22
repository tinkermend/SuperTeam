package automation

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput      = errors.New("invalid automation input")
	ErrNotFound          = errors.New("automation rule not found")
	ErrForbidden         = errors.New("automation action forbidden")
	ErrConflict          = errors.New("automation conflict")
	ErrActorNotEligible  = errors.New("actor is not eligible to initiate on project")
	ErrProjectModeLocked = errors.New("project and coordination mode cannot be changed")
)

const (
	ModePlan = "plan"
	ModeLoop = "loop"
	ModeChat = "chat"

	ScheduleCron     = "cron"
	ScheduleInterval = "interval"

	OverlapSkip = "skip"

	DisabledReasonActorRemoved           = "actor_removed_from_project"
	DisabledReasonActorDeactivated       = "actor_deactivated"
	DisabledReasonConsecutiveFireFailures = "consecutive_fire_failures"
	DisabledReasonUserDisabled           = "user_disabled"

	FireStatusPending         = "pending"
	FireStatusSucceeded       = "succeeded"
	FireStatusFailed          = "failed"
	FireStatusSkippedOverlap  = "skipped_overlap"
	FireStatusSkippedDisabled = "skipped_disabled"

	MinIntervalSeconds      = 60
	MaxConsecutiveFailures  = 3
	DefaultTimezone         = "Asia/Shanghai"
	DefaultListLimit        = 50
	MaxListLimit            = 200
)

type Rule struct {
	ID                      uuid.UUID
	TenantID                uuid.UUID
	TeamID                  uuid.UUID
	ProjectID               uuid.UUID
	ProjectName             string
	Name                    string
	Enabled                 bool
	CoordinationMode        string
	DemandTitleTemplate     *string
	DemandBodyTemplate      *string
	ScenarioTemplateKey     *string
	DigitalEmployeeID       *uuid.UUID
	ChatObjectiveTemplate   *string
	ScheduleKind            string
	CronExpr                *string
	IntervalSeconds         *int32
	Timezone                string
	OverlapPolicy           string
	ActorUserID             uuid.UUID
	DisabledReason          *string
	ConsecutiveFailureCount int32
	TemporalScheduleID      *string
	CreatedAt               time.Time
	UpdatedAt               time.Time
	LatestFire              *Fire
}

type Fire struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	RuleID          uuid.UUID
	ScheduledFireAt time.Time
	IdempotencyKey  string
	Status          string
	DemandID        *uuid.UUID
	RunID           *uuid.UUID
	ErrorCode       *string
	ErrorMessage    *string
	CreatedAt       time.Time
}

type CreateRuleRequest struct {
	TenantID              uuid.UUID
	ActorUserID           uuid.UUID
	ProjectID             uuid.UUID
	Name                  string
	CoordinationMode      string
	DemandTitleTemplate   *string
	DemandBodyTemplate    *string
	ScenarioTemplateKey   *string
	DigitalEmployeeID     *uuid.UUID
	ChatObjectiveTemplate *string
	ScheduleKind          string
	CronExpr              *string
	IntervalSeconds       *int32
	Timezone              string
	Enabled               *bool
}

type UpdateRuleRequest struct {
	TenantID              uuid.UUID
	RuleID                uuid.UUID
	ActorUserID           uuid.UUID
	Name                  *string
	DemandTitleTemplate   *string
	DemandBodyTemplate    *string
	ScenarioTemplateKey   *string
	DigitalEmployeeID     *uuid.UUID
	ChatObjectiveTemplate *string
	ScheduleKind          *string
	CronExpr              *string
	IntervalSeconds       *int32
	Timezone              *string
}

type ListRulesRequest struct {
	TenantID  uuid.UUID
	ProjectID *uuid.UUID
	Enabled   *bool
	Limit     int32
	Offset    int32
}

type ListFiresRequest struct {
	TenantID uuid.UUID
	RuleID   uuid.UUID
	Limit    int32
	Offset   int32
}

type TriggerRequest struct {
	TenantID    uuid.UUID
	RuleID      uuid.UUID
	ActorUserID uuid.UUID
}
