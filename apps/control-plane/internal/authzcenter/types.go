package authzcenter

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/authz"
)

const (
	ActionRuntimeScopeManage = authz.ActionRuntimeScopeManage
	ActionAuthzCenterRead    = authz.ActionAuthzCenterRead

	OperationModuleAuthz              = "authz"
	OperationResourceRuntimeNodeScope = "runtime_node_scope"
	OperationActionRuntimeScopeCreate = "runtime_scope.create"
	OperationActionRuntimeScopeUpdate = "runtime_scope.update"
	OperationResultSucceeded          = "succeeded"
	OperationResultFailed             = "failed"
)

var (
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("not found")
	ErrInvalidInput = errors.New("invalid input")
)

type Actor struct {
	UserID   uuid.UUID
	Username string
	TenantID uuid.UUID
	TeamID   *uuid.UUID
}

type EngineStatus struct {
	Engine        string
	Status        string
	EngineVersion string
}

type DecisionTotals struct {
	Total   int64
	Allowed int64
	Denied  int64
}

func (t DecisionTotals) DeniedRate() float64 {
	if t.Total == 0 {
		return 0
	}
	return float64(t.Denied) / float64(t.Total)
}

type ActionCount struct {
	Action string
	Count  int64
}

type Overview struct {
	Engine           EngineStatus
	Totals           DecisionTotals
	TopDeniedActions []ActionCount
	RecentEvents     []DecisionRecord
}

type DecisionFilter struct {
	TenantID     uuid.UUID
	Result       string
	Action       string
	ActorType    string
	ActorID      string
	ResourceType string
	ResourceID   string
	RequestID    string
	Limit        int32
	Offset       int32
}

type DecisionRecord struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	UserID       *uuid.UUID
	Username     *string
	Module       string
	ResourceType *string
	ResourceID   *string
	Action       string
	Result       string
	RequestID    *string
	ActorType    *string
	ActorID      *string
	Engine       *string
	Reason       *string
	MatchedRule  *string
	Details      map[string]any
	CreatedAt    time.Time
}

type RuntimeScopeNodeRecord struct {
	RuntimeNodeID      uuid.UUID
	TenantID           uuid.UUID
	NodeID             string
	Name               string
	SupportedProviders []string
	MaxSlots           int32
	CurrentLoad        int32
	Status             string
	LastHeartbeatAt    *time.Time
	RecentDeniedReason *string
	Scopes             []RuntimeScopeRecord
}

type RuntimeScopeRecord struct {
	ID            uuid.UUID
	TenantID      uuid.UUID
	RuntimeNodeID uuid.UUID
	TeamID        *uuid.UUID
	ScopeType     RuntimeScopeScopeType
	ScopeValue    string
	Status        RuntimeScopeStatus
	DisabledAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type RuntimeScopeInput struct {
	TenantID      uuid.UUID
	RuntimeNodeID uuid.UUID
	TeamID        *uuid.UUID
	ScopeType     string
	ScopeValue    string
}

type MemberFilter struct {
	TenantID uuid.UUID
	Limit    int32
	Offset   int32
}

type MemberRecord struct {
	UserID             uuid.UUID
	Username           string
	DisplayName        *string
	Email              *string
	AccountStatus      string
	ConsoleAccess      bool
	RecentDeniedReason *string
	Memberships        []MembershipRecord
}

type MembershipRecord struct {
	TenantID      uuid.UUID
	TeamID        *uuid.UUID
	PrincipalType string
	PrincipalID   uuid.UUID
	Role          string
	Status        string
}

type CheckPermissionInput struct {
	Actor    authz.ActorRef
	Action   string
	Resource authz.ResourceRef
	TenantID uuid.UUID
	TeamID   *uuid.UUID
}

type OperationLogInput struct {
	Actor        Actor
	TenantID     uuid.UUID
	Module       string
	ResourceType string
	ResourceID   string
	Action       string
	Result       string
	RequestID    string
	ClientIP     string
	UserAgent    string
	Details      map[string]any
}
