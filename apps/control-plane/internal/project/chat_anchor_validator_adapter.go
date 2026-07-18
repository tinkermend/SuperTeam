package project

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

// ValidateChatParticipant implements employee.ChatParticipantValidator
// (团队归属参与门禁): a chat run may only be driven by an active
// digital_employee member of the anchor project.
func (a ChatAnchorProjectValidatorAdapter) ValidateChatParticipant(ctx context.Context, tenantID, projectID, digitalEmployeeID uuid.UUID) error {
	members, err := a.service.repository.ListProjectMembers(ctx, tenantID, projectID)
	if err != nil {
		return fmt.Errorf("list project members for chat participant gate: %w", err)
	}
	for _, member := range members {
		if member.PrincipalType != PrincipalTypeDigitalEmployee || member.PrincipalID != digitalEmployeeID {
			continue
		}
		if member.Status != "active" {
			return fmt.Errorf("%w: 该数字员工在项目中的成员状态不是 active，无法发起对话", employee.ErrInvalidInput)
		}
		return nil
	}
	return fmt.Errorf("%w: 该数字员工不是该项目的成员，无法发起对话", employee.ErrInvalidInput)
}

// ChatAnchorProjectGit implements employee.ChatAnchorProjectGitResolver
// (目录与能力投影修订 spec §4): chat dispatch seeds a readonly worktree from the
// anchor project's repo binding, so it needs the same project_git metadata
// shape project task dispatch emits. Returns nil (no error) when the project
// has no repo binding.
func (a ChatAnchorProjectValidatorAdapter) ChatAnchorProjectGit(ctx context.Context, tenantID, projectID uuid.UUID) (map[string]any, error) {
	record, err := a.service.requireActiveProject(ctx, tenantID, projectID)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) || errors.Is(err, ErrInvalidProject) {
			return nil, fmt.Errorf("%w: project not found", employee.ErrInvalidInput)
		}
		if errors.Is(err, ErrProjectArchived) {
			return nil, fmt.Errorf("%w: project is archived", employee.ErrInvalidInput)
		}
		return nil, err
	}
	return repoBindingGitMetadata(record.RepoBinding), nil
}

// repoBindingGitMetadata mirrors projectcoordination.projectGitMetadata (that
// package imports project, so the helper cannot be shared without a cycle);
// both must keep emitting the metadata["project_git"] shape the runtime's
// payload.rs::project_git_metadata parses.
func repoBindingGitMetadata(binding ProjectRepoBinding) map[string]any {
	if binding.Status != ProjectRepoBindingStatusBound {
		return nil
	}
	values := map[string]any{
		"url":            strings.TrimSpace(binding.URL),
		"default_branch": strings.TrimSpace(binding.DefaultBranch),
	}
	if binding.GitCredentialRef != nil {
		if credentialRef := strings.TrimSpace(*binding.GitCredentialRef); credentialRef != "" {
			values["git_credential_ref"] = credentialRef
		}
	}
	scope := make([]any, 0, len(binding.Scope))
	for _, item := range binding.Scope {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			scope = append(scope, trimmed)
		}
	}
	if len(scope) > 0 {
		values["scope"] = scope
	}
	return values
}
