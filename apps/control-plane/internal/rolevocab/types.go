package rolevocab

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput = errors.New("invalid role vocabulary input")
	ErrNotFound     = errors.New("role vocabulary entry not found")
	ErrConflict     = errors.New("role vocabulary conflict")
)

const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

type Entry struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	RoleKey     string
	Title       string
	Description string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CreateRequest struct {
	TenantID    uuid.UUID
	ActorUserID uuid.UUID
	RoleKey     string
	Title       string
	Description string
	Status      string
}

type PatchRequest struct {
	TenantID    uuid.UUID
	ActorUserID uuid.UUID
	RoleKey     string
	Title       *string
	Description *string
	Status      *string
}

// TemplateRef is a scenario template that references a role key in its current spec.
type TemplateRef struct {
	Key  string
	Name string
}

// EmployeeRef is a digital employee that currently holds a role key.
type EmployeeRef struct {
	ID   uuid.UUID
	Name string
}

// References is the disable-impact snapshot for a role vocabulary entry.
// Design: { scenario_templates, employee_count, casting_count }; employees[]
// is included so the confirm dialog can list holder names (spec §4.3).
type References struct {
	ScenarioTemplates []TemplateRef
	Employees         []EmployeeRef
	EmployeeCount     int
	CastingCount      int
}
