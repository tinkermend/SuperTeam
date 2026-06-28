package prompttemplate

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/auth"
)

type mockRepository struct {
	listFn              func(ctx context.Context, tenantID uuid.UUID, teamIDs []uuid.UUID, userID uuid.UUID) ([]PromptTemplate, error)
	createFn            func(ctx context.Context, input CreateTemplateInput) (PromptTemplate, error)
	incrementUseCountFn func(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error
}

func (m *mockRepository) List(ctx context.Context, tenantID uuid.UUID, teamIDs []uuid.UUID, userID uuid.UUID) ([]PromptTemplate, error) {
	return m.listFn(ctx, tenantID, teamIDs, userID)
}

func (m *mockRepository) Create(ctx context.Context, input CreateTemplateInput) (PromptTemplate, error) {
	return m.createFn(ctx, input)
}

func (m *mockRepository) IncrementUseCount(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	return m.incrementUseCountFn(ctx, id, tenantID)
}

type mockResolver struct {
	listUserProjectTeamScopesFn func(ctx context.Context, tenantID, userID uuid.UUID) ([]auth.UserProjectTeamScopeSummary, error)
}

func (m *mockResolver) ListUserProjectTeamScopes(ctx context.Context, tenantID, userID uuid.UUID) ([]auth.UserProjectTeamScopeSummary, error) {
	return m.listUserProjectTeamScopesFn(ctx, tenantID, userID)
}

func TestService_ListTemplates(t *testing.T) {
	repo := &mockRepository{}
	resolver := &mockResolver{}
	svc := NewService(repo, resolver, nil)

	ctx := context.Background()
	tenantID := uuid.New()
	userID := uuid.New()
	teamID1 := uuid.New()
	teamID2 := uuid.New()

	authCtx := &auth.CurrentUserContext{
		User:     &auth.User{ID: userID},
		TenantID: tenantID,
	}

	resolver.listUserProjectTeamScopesFn = func(ctx context.Context, tID, uID uuid.UUID) ([]auth.UserProjectTeamScopeSummary, error) {
		if tID != tenantID || uID != userID {
			return nil, errors.New("wrong args")
		}
		return []auth.UserProjectTeamScopeSummary{
			{TeamID: teamID1},
			{TeamID: teamID2},
		}, nil
	}

	repo.listFn = func(ctx context.Context, tID uuid.UUID, teamIDs []uuid.UUID, uID uuid.UUID) ([]PromptTemplate, error) {
		if tID != tenantID || uID != userID {
			return nil, errors.New("wrong args")
		}
		if len(teamIDs) != 2 || teamIDs[0] != teamID1 || teamIDs[1] != teamID2 {
			return nil, errors.New("wrong teamIDs")
		}
		return []PromptTemplate{{ID: uuid.New(), Title: "Test Template"}}, nil
	}

	templates, err := svc.ListTemplates(ctx, authCtx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(templates) != 1 || templates[0].Title != "Test Template" {
		t.Fatalf("unexpected templates: %+v", templates)
	}
}

func TestService_CreateTemplate(t *testing.T) {
	repo := &mockRepository{}
	resolver := &mockResolver{}
	svc := NewService(repo, resolver, nil)

	ctx := context.Background()
	tenantID := uuid.New()
	userID := uuid.New()
	teamID := uuid.New()

	t.Run("success global scope without tokens", func(t *testing.T) {
		repo.createFn = func(ctx context.Context, input CreateTemplateInput) (PromptTemplate, error) {
			return PromptTemplate{ID: uuid.New(), Title: input.Title}, nil
		}

		input := CreateTemplateInput{
			TenantID:     tenantID,
			Title:        "Title",
			Content:      "No tokens",
			CategoryCode: "TEST",
			Scope:        "GLOBAL",
			CreatorID:    userID,
		}

		res, err := svc.CreateTemplate(ctx, input)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if res.Title != "Title" {
			t.Fatalf("unexpected result: %+v", res)
		}
	})

	t.Run("success team scope with tokens", func(t *testing.T) {
		resolver.listUserProjectTeamScopesFn = func(ctx context.Context, tID, uID uuid.UUID) ([]auth.UserProjectTeamScopeSummary, error) {
			return []auth.UserProjectTeamScopeSummary{{TeamID: teamID}}, nil
		}
		repo.createFn = func(ctx context.Context, input CreateTemplateInput) (PromptTemplate, error) {
			return PromptTemplate{ID: uuid.New(), Title: input.Title}, nil
		}

		input := CreateTemplateInput{
			TenantID:     tenantID,
			Title:        "Title",
			Content:      "Hello {{name}}, you are {{age}}",
			CategoryCode: "TEST",
			Scope:        "TEAM",
			TeamID:       &teamID,
			CreatorID:    userID,
			Variables: []PromptTemplateVariable{
				{Name: "name"},
				{Name: "age"},
			},
		}

		res, err := svc.CreateTemplate(ctx, input)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if res.Title != "Title" {
			t.Fatalf("unexpected result: %+v", res)
		}
	})

	t.Run("fail team scope without team access", func(t *testing.T) {
		resolver.listUserProjectTeamScopesFn = func(ctx context.Context, tID, uID uuid.UUID) ([]auth.UserProjectTeamScopeSummary, error) {
			return []auth.UserProjectTeamScopeSummary{{TeamID: uuid.New()}}, nil // different team
		}

		input := CreateTemplateInput{
			TenantID:     tenantID,
			Title:        "Title",
			Content:      "Content",
			Scope:        "TEAM",
			TeamID:       &teamID,
			CreatorID:    userID,
		}

		_, err := svc.CreateTemplate(ctx, input)
		if err == nil || err.Error() != "forbidden: user does not belong to this team" {
			t.Fatalf("expected unauthorized team_id error, got %v", err)
		}
	})

	t.Run("fail token missing in variables", func(t *testing.T) {
		input := CreateTemplateInput{
			TenantID:     tenantID,
			Title:        "Title",
			Content:      "Hello {{name}}, you are {{age}}",
			Scope:        "USER",
			CreatorID:    userID,
			Variables: []PromptTemplateVariable{
				{Name: "name"},
				// missing age
			},
		}

		_, err := svc.CreateTemplate(ctx, input)
		if err == nil || err.Error() != "bad request: token \"age\" is used in the template content but not defined in variables" {
			t.Fatalf("expected missing variable error, got %v", err)
		}
	})

	t.Run("fail variable not in tokens", func(t *testing.T) {
		input := CreateTemplateInput{
			TenantID:     tenantID,
			Title:        "Title",
			Content:      "Hello {{name}}",
			Scope:        "USER",
			CreatorID:    userID,
			Variables: []PromptTemplateVariable{
				{Name: "name"},
				{Name: "age"}, // unused
			},
		}

		_, err := svc.CreateTemplate(ctx, input)
		if err == nil || err.Error() != "bad request: variable \"age\" is defined but not used in the template content" {
			t.Fatalf("expected unused variable error, got %v", err)
		}
	})

	t.Run("fail team scope missing teamID for admin", func(t *testing.T) {
		input := CreateTemplateInput{
			TenantID:     tenantID,
			Title:        "Title",
			Content:      "Content",
			Scope:        "TEAM",
			CreatorID:    userID,
			IsAdmin:      true,
		}

		_, err := svc.CreateTemplate(ctx, input)
		if err == nil || err.Error() != "bad request: team_id is required for TEAM scope" {
			t.Fatalf("expected missing team_id error, got %v", err)
		}
	})

	t.Run("success team scope for admin without team access", func(t *testing.T) {
		resolver.listUserProjectTeamScopesFn = func(ctx context.Context, tID, uID uuid.UUID) ([]auth.UserProjectTeamScopeSummary, error) {
			return []auth.UserProjectTeamScopeSummary{{TeamID: uuid.New()}}, nil // different team
		}
		repo.createFn = func(ctx context.Context, input CreateTemplateInput) (PromptTemplate, error) {
			return PromptTemplate{ID: uuid.New(), Title: input.Title}, nil
		}

		input := CreateTemplateInput{
			TenantID:     tenantID,
			Title:        "Title",
			Content:      "Content",
			Scope:        "TEAM",
			TeamID:       &teamID,
			CreatorID:    userID,
			IsAdmin:      true,
		}

		res, err := svc.CreateTemplate(ctx, input)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if res.Title != "Title" {
			t.Fatalf("unexpected result: %+v", res)
		}
	})

	t.Run("success clearing TeamID for non-TEAM scope", func(t *testing.T) {
		repo.createFn = func(ctx context.Context, input CreateTemplateInput) (PromptTemplate, error) {
			if input.TeamID != nil {
				t.Fatalf("expected TeamID to be nil, got %v", *input.TeamID)
			}
			return PromptTemplate{ID: uuid.New(), Title: input.Title}, nil
		}

		input := CreateTemplateInput{
			TenantID:     tenantID,
			Title:        "Title",
			Content:      "Content",
			Scope:        "GLOBAL",
			TeamID:       &teamID,
			CreatorID:    userID,
		}

		res, err := svc.CreateTemplate(ctx, input)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if res.Title != "Title" {
			t.Fatalf("unexpected result: %+v", res)
		}
	})
}

func TestService_ApplyTemplate(t *testing.T) {
	repo := &mockRepository{}
	resolver := &mockResolver{}
	svc := NewService(repo, resolver, nil)

	ctx := context.Background()
	tenantID := uuid.New()
	templateID := uuid.New()

	authCtx := &auth.CurrentUserContext{
		User:     &auth.User{ID: uuid.New()},
		TenantID: tenantID,
	}

	repo.incrementUseCountFn = func(ctx context.Context, id uuid.UUID, tID uuid.UUID) error {
		if id != templateID || tID != tenantID {
			return errors.New("wrong args")
		}
		return errors.New("some DB error") // Should not fail the call
	}

	err := svc.ApplyTemplate(ctx, templateID, authCtx)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}
