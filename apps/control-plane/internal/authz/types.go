package authz

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

const (
	ActorUser        = "user"
	ActorRuntimeNode = "runtime_node"
	ActorEmployee    = "employee"
	ActorService     = "service_account"
)

const (
	ResourceConsole = "console"
	ResourceTenant  = "tenant"
	ResourceTeam    = "team"
	ResourceTask    = "task"
)

const (
	ActionConsoleAccess      = "console.access"
	ActionTenantAccess       = "tenant.access"
	ActionTeamAccess         = "team.access"
	ActionTaskClaim          = "task.claim"
	ActionRuntimeScopeManage = "runtime_scope.manage"
)

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
	RoleViewer = "viewer"
)

const (
	ReasonAllowed             = "allowed"
	ReasonNoMembership        = "no active membership"
	ReasonInvalidActor        = "invalid actor"
	ReasonInvalidResource     = "invalid resource"
	ReasonUnsupportedAction   = "unsupported action"
	ReasonRuntimeScopeMissing = "runtime scope does not cover task"
)

var (
	ErrNoMembership      = errors.New("no active membership")
	ErrUnsupportedAction = errors.New("unsupported action")
)

type ActorRef struct {
	Type string
	ID   string
}

type ResourceRef struct {
	Type string
	ID   string
}

type CheckRequest struct {
	Actor       ActorRef
	Action      string
	Resource    ResourceRef
	TenantID    uuid.UUID
	TeamID      *uuid.UUID
	Context     map[string]any
	AuditReason string
}

type Decision struct {
	Allowed       bool
	Reason        string
	MatchedRule   string
	RequiresAudit bool
	Snapshot      map[string]any
}

type DecisionRecord struct {
	TenantID     uuid.UUID
	TeamID       *uuid.UUID
	ActorType    string
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	Allowed      bool
	Reason       string
	MatchedRule  string
	Engine       string
	Snapshot     map[string]any
}

type DecisionRecorder interface {
	RecordDecision(ctx context.Context, record DecisionRecord) error
}

type Membership struct {
	TenantID      uuid.UUID
	TeamID        *uuid.UUID
	PrincipalType string
	PrincipalID   uuid.UUID
	Role          string
	Status        string
}
