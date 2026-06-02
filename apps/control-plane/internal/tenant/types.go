package tenant

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput = errors.New("invalid tenant input")
	ErrNotFound     = errors.New("tenant not found")
)

type TeamStatus string

const (
	TeamStatusActive   TeamStatus = "active"
	TeamStatusDisabled TeamStatus = "disabled"
)

func (s TeamStatus) IsValid() bool {
	switch s {
	case TeamStatusActive, TeamStatusDisabled:
		return true
	default:
		return false
	}
}

type TeamConfigRevisionStatus string

const (
	TeamConfigRevisionStatusDraft  TeamConfigRevisionStatus = "draft"
	TeamConfigRevisionStatusActive TeamConfigRevisionStatus = "active"
)

func (s TeamConfigRevisionStatus) IsValid() bool {
	switch s {
	case TeamConfigRevisionStatusDraft, TeamConfigRevisionStatusActive:
		return true
	default:
		return false
	}
}

type Team struct {
	ID               uuid.UUID
	TenantID         uuid.UUID
	Slug             string
	Name             string
	Status           TeamStatus
	HumanOwnerUserID *uuid.UUID
	Metadata         map[string]any
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type TeamConfigRevision struct {
	ID                          uuid.UUID
	TenantID                    uuid.UUID
	TeamID                      uuid.UUID
	RevisionNumber              int32
	Constitution                map[string]any
	CapabilityPolicy            map[string]any
	ContextPolicy               map[string]any
	ApprovalPolicy              map[string]any
	ArtifactContract            map[string]any
	InternalCollaborationPolicy map[string]any
	RuntimeScopePolicy          map[string]any
	HumanOwnerUserID            *uuid.UUID
	Status                      TeamConfigRevisionStatus
	ApprovedBy                  *uuid.UUID
	ApprovedAt                  *time.Time
	CreatedAt                   time.Time
	UpdatedAt                   time.Time
}

type CreateTeamRequest struct {
	TenantID         uuid.UUID
	Slug             string
	Name             string
	Status           TeamStatus
	HumanOwnerUserID *uuid.UUID
	Metadata         map[string]any
}

type ListTeamsRequest struct {
	TenantID uuid.UUID
	Status   TeamStatus
	Offset   int32
	Limit    int32
}

type CreateTeamConfigRevisionRequest struct {
	TenantID                    uuid.UUID
	TeamID                      uuid.UUID
	Constitution                map[string]any
	CapabilityPolicy            map[string]any
	ContextPolicy               map[string]any
	ApprovalPolicy              map[string]any
	ArtifactContract            map[string]any
	InternalCollaborationPolicy map[string]any
	RuntimeScopePolicy          map[string]any
	HumanOwnerUserID            *uuid.UUID
	Status                      TeamConfigRevisionStatus
	ApprovedBy                  *uuid.UUID
}
