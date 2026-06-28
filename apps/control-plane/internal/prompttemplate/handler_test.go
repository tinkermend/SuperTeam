package prompttemplate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/superteam/control-plane/internal/api/gen"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/auth"
)

type mockAuthService struct {
	ctx *auth.CurrentUserContext
	err error
}

func (m *mockAuthService) GetCurrentUserContext(ctx context.Context, sessionToken string) (*auth.CurrentUserContext, error) {
	return m.ctx, m.err
}

type mockHandlerService struct {
	listRes   []PromptTemplate
	listErr   error
	createRes PromptTemplate
	createErr error
	applyErr  error
}

func (m *mockHandlerService) ListTemplates(ctx context.Context, authCtx *auth.CurrentUserContext) ([]PromptTemplate, error) {
	return m.listRes, m.listErr
}

func (m *mockHandlerService) CreateTemplate(ctx context.Context, input CreateTemplateInput) (PromptTemplate, error) {
	return m.createRes, m.createErr
}

func (m *mockHandlerService) ApplyTemplate(ctx context.Context, id uuid.UUID, authCtx *auth.CurrentUserContext) error {
	return m.applyErr
}

func TestHandler_AuthFailure(t *testing.T) {
	h := NewHandler(&mockHandlerService{}, &mockAuthService{err: errors.New("unauthorized")})
	req := httptest.NewRequest(http.MethodGet, "/templates", nil)
	w := httptest.NewRecorder()
	h.ListPromptTemplates(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for no context auth, got %d", w.Code)
	}
}

func TestHandler_ListSuccess(t *testing.T) {
	mockID := uuid.New()
	tenantID := uuid.New()
	
	service := &mockHandlerService{
		listRes: []PromptTemplate{
			{
				ID:           mockID,
				TenantID:     tenantID,
				Title:        "test",
				Content:      "hello",
				CategoryCode: "cat",
				Scope:        "TENANT",
			},
		},
	}
	authSvc := &mockAuthService{ctx: &auth.CurrentUserContext{}}
	h := NewHandler(service, authSvc)

	req := httptest.NewRequest(http.MethodGet, "/templates", nil)
	ctx := context.WithValue(req.Context(), middleware.TenantIDKey, tenantID)
	ctx = context.WithValue(ctx, middleware.UserIDKey, uuid.New())
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.ListPromptTemplates(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var res []gen.PromptTemplate
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Id.String() != mockID.String() {
		t.Errorf("unexpected response mapping: %+v", res)
	}
}

func TestHandler_CreateTemplate(t *testing.T) {
	authCtx := &auth.CurrentUserContext{TenantID: uuid.New(), User: &auth.User{ID: uuid.New()}}
	authSvc := &mockAuthService{ctx: authCtx}

	tenantID := uuid.New()
	userID := uuid.New()

	t.Run("invalid json", func(t *testing.T) {
		h := NewHandler(&mockHandlerService{}, authSvc)
		req := httptest.NewRequest(http.MethodPost, "/templates", bytes.NewBufferString("{bad json"))
		ctx := context.WithValue(req.Context(), middleware.TenantIDKey, tenantID)
		ctx = context.WithValue(ctx, middleware.UserIDKey, userID)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()
		h.CreatePromptTemplate(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing fields", func(t *testing.T) {
		h := NewHandler(&mockHandlerService{createErr: errors.New("team_id is required")}, authSvc)
		body := gen.CreatePromptTemplateRequest{
			Title: "New Temp",
			Content: "content",
			Scope: gen.TEAM,
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/templates", bytes.NewBuffer(b))
		ctx := context.WithValue(req.Context(), middleware.TenantIDKey, tenantID)
		ctx = context.WithValue(ctx, middleware.UserIDKey, userID)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()
		h.CreatePromptTemplate(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for missing fields, got %d", w.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		mockID := uuid.New()
		teamID := uuid.New()
		
		teamIDGen := openapi_types.UUID(teamID)
		
		body := gen.CreatePromptTemplateRequest{
			Title: "New Temp",
			Content: "content",
			Scope: gen.TEAM,
			TeamId: &teamIDGen,
			CategoryCode: "code",
		}
		b, _ := json.Marshal(body)
		
		service := &mockHandlerService{
			createRes: PromptTemplate{
				ID: mockID,
				Title: "New Temp",
				Content: "content",
				TeamID: &teamID,
			},
		}
		h := NewHandler(service, authSvc)
		req := httptest.NewRequest(http.MethodPost, "/templates", bytes.NewBuffer(b))
		ctx := context.WithValue(req.Context(), middleware.TenantIDKey, tenantID)
		ctx = context.WithValue(ctx, middleware.UserIDKey, userID)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		h.CreatePromptTemplate(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		var res gen.PromptTemplate
		if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
			t.Fatal(err)
		}
		if res.Id.String() != mockID.String() {
			t.Errorf("expected id %s, got %s", mockID, res.Id)
		}
	})
}

func TestHandler_ApplyTemplate(t *testing.T) {
	authCtx := &auth.CurrentUserContext{TenantID: uuid.New(), User: &auth.User{ID: uuid.New()}}
	authSvc := &mockAuthService{ctx: authCtx}

	t.Run("invalid id", func(t *testing.T) {
		h := NewHandler(&mockHandlerService{}, authSvc)
		req := httptest.NewRequest(http.MethodPost, "/templates/invalid/apply", nil)
		
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "invalid")
		ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
		ctx = context.WithValue(ctx, middleware.TenantIDKey, uuid.New())
		ctx = context.WithValue(ctx, middleware.UserIDKey, uuid.New())
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()
		
		h.ApplyPromptTemplate(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid id, got %d", w.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		h := NewHandler(&mockHandlerService{}, authSvc)
		req := httptest.NewRequest(http.MethodPost, "/templates/uuid/apply", nil)
		
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", uuid.New().String())
		ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
		ctx = context.WithValue(ctx, middleware.TenantIDKey, uuid.New())
		ctx = context.WithValue(ctx, middleware.UserIDKey, uuid.New())
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()
		
		h.ApplyPromptTemplate(w, req)
		if w.Code != http.StatusNoContent {
			t.Errorf("expected 204, got %d: %s", w.Code, w.Body.String())
		}
	})
}
