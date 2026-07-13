package project

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/employee"
)

// ChatAnchorProjectValidatorAdapter implements employee.ChatAnchorProjectValidator
// by delegating to Service.requireActiveProject: a chat run's project anchor
// (§13 design revision) must exist, belong to the requesting tenant, and not be
// archived before its dispatch touches project-scoped node resolution and
// preflight. The employee package must not import project, so it only ever
// sees an opaque error wrapping employee.ErrInvalidInput (400-mapped by the
// handler layer) for any not-found/archived/cross-tenant outcome.
type ChatAnchorProjectValidatorAdapter struct {
	service *Service
}

// NewChatAnchorProjectValidatorAdapter wires a project Service into the
// employee.ChatAnchorProjectValidator interface for injection into
// DigitalEmployeeRunService.
func NewChatAnchorProjectValidatorAdapter(service *Service) employee.ChatAnchorProjectValidator {
	if service == nil {
		return nil
	}
	return ChatAnchorProjectValidatorAdapter{service: service}
}

func (a ChatAnchorProjectValidatorAdapter) ValidateChatAnchorProject(ctx context.Context, tenantID, projectID uuid.UUID) error {
	if _, err := a.service.requireActiveProject(ctx, tenantID, projectID); err != nil {
		if errors.Is(err, ErrProjectNotFound) || errors.Is(err, ErrInvalidProject) {
			return fmt.Errorf("%w: project not found", employee.ErrInvalidInput)
		}
		if errors.Is(err, ErrProjectArchived) {
			return fmt.Errorf("%w: project is archived", employee.ErrInvalidInput)
		}
		return err
	}
	return nil
}
