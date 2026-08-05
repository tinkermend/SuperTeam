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
