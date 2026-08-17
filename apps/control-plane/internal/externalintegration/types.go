// Package externalintegration implements autonomy P5: external API integrations
// (spec docs/superpowers/specs/2026-08-13-autonomy-envelope-policy-playbook-design.md §8/§8.1).
//
// An integration is a pre-authorized execution entry with an envelope: it pins
// project, digital employee, skill set, optional playbook, autonomy tier, and a
// per-hour call budget at grant time. Enforcement is a LIVE reference — the
// effective tier and skill surface are recomputed on every call against current
// project/playbook ceilings (tightening applies immediately; loosening never
// auto-upgrades the binding). Calls never produce approval items: out-of-envelope
// or over-budget requests are rejected explicitly.
package externalintegration

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput = errors.New("invalid external integration input")
	ErrNotFound     = errors.New("external integration not found")
	ErrUnauthorized = errors.New("invalid or revoked integration token")
	ErrForbidden    = errors.New("integration forbidden")
	ErrOverBudget   = errors.New("integration hourly call budget exhausted")
	// ErrPolicyReject: external demand would hit a human gate (effective ≠ full_auto).
	// Spec §2.2 / §4.4 / §10 — sync reject, never park into inbox.
	ErrPolicyReject = errors.New("external call rejected by autonomy policy")
)

const (
	StatusActive   = "active"
	StatusDisabled = "disabled"

	TokenStatusActive  = "active"
	TokenStatusRevoked = "revoked"

	// TokenPrefix marks integration tokens (dedicated lifecycle, decision §8.1-2).
	TokenPrefix = "xi_"

	// DemandSourceType / chat run metadata source stamped by the two verbs.
	SourceType = "external_integration"

	DefaultMaxCallsPerHour = 60
)

type Integration struct {
	ID                  uuid.UUID
	TenantID            uuid.UUID
	ProjectID           uuid.UUID
	DigitalEmployeeID   uuid.UUID
	Name                string
	Description         string
	AllowChatRun        bool
	AllowDemandSubmit   bool
	SkillIDs            []uuid.UUID
	ScenarioTemplateKey *string
	AutonomyTier        string
	PinnedExitDeliverable        *string
	AcknowledgeExitTierSemantics bool
	MaxCallsPerHour     int32
	Status              string
	CreatedByUserID     uuid.UUID
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type Token struct {
	ID            uuid.UUID
	TenantID      uuid.UUID
	IntegrationID uuid.UUID
	Status        string
	CreatedAt     time.Time
	LastUsedAt    *time.Time
	RevokedAt     *time.Time
}

type CreateIntegrationRequest struct {
	TenantID            uuid.UUID
	CreatedByUserID     uuid.UUID
	ProjectID           uuid.UUID
	DigitalEmployeeID   uuid.UUID
	Name                string
	Description         string
	AllowChatRun        bool
	AllowDemandSubmit   bool
	SkillIDs            []uuid.UUID
	ScenarioTemplateKey *string
	AutonomyTier        string
	PinnedExitDeliverable        *string
	AcknowledgeExitTierSemantics bool
	MaxCallsPerHour     *int32
}

type UpdateIntegrationRequest struct {
	TenantID      uuid.UUID
	IntegrationID uuid.UUID
	ActorUserID   uuid.UUID

	Name              *string
	Description       *string
	AllowChatRun      *bool
	AllowDemandSubmit *bool
	SkillIDs          *[]uuid.UUID
	// ScenarioTemplateKeySet=true applies ScenarioTemplateKey (nil clears the binding).
	ScenarioTemplateKeySet bool
	ScenarioTemplateKey    *string
	AutonomyTier           *string
	PinnedExitDeliverable  *string
	AcknowledgeExitTierSemanticsSet bool
	AcknowledgeExitTierSemantics    bool
	MaxCallsPerHour        *int32
	Status                 *string
}

// ExternalChatRunRequest is verb 1: a chat run inside the envelope.
type ExternalChatRunRequest struct {
	Objective     string
	ResumeOfRunID *uuid.UUID
}

type ExternalChatRunResult struct {
	RunID  uuid.UUID
	Status string
}

// ExternalDemandRequest is verb 2: a plan/loop demand through the binding.
type ExternalDemandRequest struct {
	Title            string
	Content          string
	CoordinationMode string
}

type ExternalDemandResult struct {
	DemandID uuid.UUID
	Status   string
}
