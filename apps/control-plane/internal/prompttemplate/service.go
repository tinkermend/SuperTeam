package prompttemplate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/auth"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrBadRequest   = errors.New("bad request")
	ErrNotFound     = errors.New("not found")
)

type TeamMembershipResolver interface {
	ListUserProjectTeamScopes(ctx context.Context, tenantID, userID uuid.UUID) ([]auth.UserProjectTeamScopeSummary, error)
}

type Service struct {
	repo     Repository
	resolver TeamMembershipResolver
	logger   *slog.Logger
}

func NewService(repo Repository, resolver TeamMembershipResolver, logger *slog.Logger) *Service {
	return &Service{
		repo:     repo,
		resolver: resolver,
		logger:   logger,
	}
}

func (s *Service) ListTemplates(ctx context.Context, authCtx *auth.CurrentUserContext) ([]PromptTemplate, error) {
	if authCtx == nil || authCtx.User == nil {
		return nil, ErrUnauthorized
	}

	scopes, err := s.resolver.ListUserProjectTeamScopes(ctx, authCtx.TenantID, authCtx.User.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list user team scopes: %w", err)
	}

	teamIDs := make([]uuid.UUID, 0, len(scopes))
	for _, scope := range scopes {
		teamIDs = append(teamIDs, scope.TeamID)
	}

	return s.repo.List(ctx, authCtx.TenantID, teamIDs, authCtx.User.ID)
}

func (s *Service) CreateTemplate(ctx context.Context, input CreateTemplateInput) (PromptTemplate, error) {
	if input.Scope == "TEAM" {
		if input.TeamID == nil || *input.TeamID == uuid.Nil {
			return PromptTemplate{}, fmt.Errorf("%w: team_id is required for TEAM scope", ErrBadRequest)
		}

		if !input.IsAdmin {
			scopes, err := s.resolver.ListUserProjectTeamScopes(ctx, input.TenantID, input.CreatorID)
			if err != nil {
				return PromptTemplate{}, fmt.Errorf("failed to list user team scopes: %w", err)
			}

			found := false
			for _, scope := range scopes {
				if scope.TeamID == *input.TeamID {
					found = true
					break
				}
			}
			if !found {
				return PromptTemplate{}, fmt.Errorf("%w: user does not belong to this team", ErrForbidden)
			}
		}
	} else {
		input.TeamID = nil
	}

	// Validate variables ↔ {{tokens}}
	matches := tokenRegex.FindAllStringSubmatch(input.Content, -1)
	tokensInContent := make(map[string]bool)
	for _, m := range matches {
		if len(m) >= 2 {
			tokensInContent[m[1]] = true
		}
	}

	varsDefined := make(map[string]bool)
	for _, v := range input.Variables {
		varsDefined[v.Name] = true
		if !tokensInContent[v.Name] {
			return PromptTemplate{}, fmt.Errorf("%w: variable %q is defined but not used in the template content", ErrBadRequest, v.Name)
		}
	}

	for token := range tokensInContent {
		if !varsDefined[token] {
			return PromptTemplate{}, fmt.Errorf("%w: token %q is used in the template content but not defined in variables", ErrBadRequest, token)
		}
	}

	return s.repo.Create(ctx, input)
}

func (s *Service) ApplyTemplate(ctx context.Context, id uuid.UUID, authCtx *auth.CurrentUserContext) error {
	if authCtx == nil {
		return ErrUnauthorized
	}
	err := s.repo.IncrementUseCount(ctx, id, authCtx.TenantID)
	if err != nil {
		if s.logger != nil {
			s.logger.ErrorContext(ctx, "failed to increment template use count", "template_id", id, "error", err)
		}
		if strings.Contains(err.Error(), "no rows in result set") || err.Error() == "not found" {
			return ErrNotFound
		}
		return err
	}
	return nil
}
